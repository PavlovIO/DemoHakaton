package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
)

// Валидация запросов по спеке (nethttp-middleware) стоит перед роутингом: мусор
// получает 400 с полем в details ещё до хендлера, а для нереализованных операций
// корректный запрос доходит до хендлера.
func TestRequestValidation(t *testing.T) {
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Auth: testAuthService(), Applications: &fakeApplications{app: testApplication()}})
	// Админские операции Authorize пускает только admin: иначе 403 раньше валидатора.
	adminAuth := testAuthService()
	adminAuth.principal.Role = "admin"
	adminSrv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Auth: adminAuth})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		ctype      string
		wantStatus int
		wantCode   string
		wantField  string // первое поле в details; "" — не проверять
		wantFields int    // число записей в details; 0 — не проверять
		admin      bool   // запрос от admin
	}{
		// Позитивные: путь / query / тело — доходят до хендлера или заглушки.
		{name: "ok путь", method: "GET", path: "/api/v1/dictionaries/countries", wantStatus: 200},
		{name: "registry lookup без сессии", method: "GET", path: "/api/v1/registry/lookup", wantStatus: 400, wantCode: "VALIDATION_ERROR"},
		{name: "ok query -> хендлер", method: "GET", path: "/api/v1/applications?page=1&page_size=20", wantStatus: 200},
		{name: "ok тело -> хендлер", method: "PATCH", path: "/api/v1/applications/019245a0-1b2c-7def-8000-000000000001", body: `{"term_months":12}`, ctype: "application/json", wantStatus: 200},
		{name: "ok uuid v7 в пути", method: "GET", path: "/api/v1/applications/019245a0-1b2c-7def-8000-000000000001", wantStatus: 200},
		{name: "ok несколько target_country", method: "GET", path: "/api/v1/applications?target_country=DE&target_country=TR", wantStatus: 200},
		{name: "ok витрина и архив откликов", method: "GET", path: "/api/v1/applications?status=published&sort=published_at_desc&has_my_offer=true", wantStatus: 200},
		// HEAD: ServeMux сопоставляет его с GET-паттернами, валидатор (gorillamux) — нет; раньше было 200.
		{name: "HEAD как GET", method: "HEAD", path: "/api/v1/dictionaries/countries", wantStatus: 200},

		// Query.
		{name: "page=0 (minimum)", method: "GET", path: "/api/v1/applications?page=0", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "page"},
		{name: "status вне enum", method: "GET", path: "/api/v1/applications?status=bogus", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "status"},
		{name: "amount_min не по паттерну", method: "GET", path: "/api/v1/applications?amount_min=abc", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "amount_min"},
		{name: "page_size > maximum", method: "GET", path: "/api/v1/applications?page_size=1000", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "page_size"},
		{name: "target_country не ISO-код", method: "GET", path: "/api/v1/applications?target_country=DE&target_country=Germany", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "target_country"},
		{name: "has_my_offer не boolean", method: "GET", path: "/api/v1/applications?has_my_offer=yes", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "has_my_offer"},
		{name: "deals: организация не uuid", method: "GET", path: "/api/v1/deals?lender_organization_id=acme", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "lender_organization_id"},
		{name: "aml: заявка не uuid", method: "GET", path: "/api/v1/admin/aml/flags?application_id=APP-1", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "application_id", admin: true},

		// Путь.
		{name: "не uuid в пути", method: "GET", path: "/api/v1/applications/not-a-uuid", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "applicationId"},

		// Тело.
		{name: "поле короче minLength", method: "PATCH", path: "/api/v1/organizations/me", body: `{"name":"x"}`, ctype: "application/json", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "name"},
		{name: "нет обязательного поля", method: "POST", path: "/api/v1/auth/login", body: `{"email":"a@b.co"}`, ctype: "application/json", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "password"},
		// MultiError: по записи на каждое поле тела, а не первая ошибка.
		{name: "несколько полей тела", method: "PATCH", path: "/api/v1/organizations/me", body: `{"name":"x","lei":"bad","legal_address":5}`, ctype: "application/json", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantFields: 3},
		{name: "несколько query-параметров", method: "GET", path: "/api/v1/applications?page=0&page_size=1000&status=zz", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantFields: 3},
		{name: "нет тела", method: "PATCH", path: "/api/v1/organizations/me", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "body"},
		{name: "битый JSON", method: "PATCH", path: "/api/v1/organizations/me", body: `{`, ctype: "application/json", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "body"},
		{name: "тело больше лимита", method: "PATCH", path: "/api/v1/organizations/me", body: `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, ctype: "application/json", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "body"},

		// Роутинг: неизвестный путь и метод — 404 в формате Error, как и до валидатора.
		{name: "неизвестный путь", method: "GET", path: "/api/v1/nope", wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "неизвестный метод", method: "DELETE", path: "/api/v1/dictionaries/countries", wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "вне префикса", method: "GET", path: "/healthz", wantStatus: 404, wantCode: "NOT_FOUND"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			baseURL := srv.URL
			if tt.admin {
				baseURL = adminSrv.URL
			}
			req, err := http.NewRequest(tt.method, baseURL+tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.ctype != "" {
				req.Header.Set("Content-Type", tt.ctype)
			}
			// Сессия и CSRF — на каждом запросе: публичные операции их игнорируют.
			addValidTestSession(req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tt.wantStatus, raw)
			}
			if tt.wantCode == "" {
				return
			}
			var e api.Error
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatalf("decode %s: %v", raw, err)
			}
			if e.Error.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s; body %s", e.Error.Code, tt.wantCode, raw)
			}
			if tt.wantField != "" {
				if len(e.Error.Details) == 0 || e.Error.Details[0].Field != tt.wantField {
					t.Fatalf("details = %+v, want field %q", e.Error.Details, tt.wantField)
				}
				if e.Error.Details[0].Message == "" {
					t.Fatal("пустое сообщение в details")
				}
			}
			if tt.wantFields != 0 && len(e.Error.Details) != tt.wantFields {
				t.Fatalf("details = %+v, want %d записей", e.Error.Details, tt.wantFields)
			}
		})
	}

	t.Run("multipart пропускается", func(t *testing.T) {
		// Загрузку файла валидатор не трогает (читал бы multipart целиком в память):
		// мусорное тело доходит до хендлера, и форму проверяет он сам — нет части file.
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/organizations/me/documents", strings.NewReader("garbage"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		addValidTestSession(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var e api.Error
		_ = json.Unmarshal(raw, &e)
		if resp.StatusCode != http.StatusBadRequest || len(e.Error.Details) == 0 || e.Error.Details[0].Field != "file" {
			t.Fatalf("status = %d, want 400 с field file; body %s", resp.StatusCode, raw)
		}
	})
}
