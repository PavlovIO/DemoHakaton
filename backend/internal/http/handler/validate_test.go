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
// корректный запрос доходит до заглушки (501).
func TestRequestValidation(t *testing.T) {
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, Auth: testAuthService()})

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
	}{
		// Позитивные: путь / query / тело — доходят до хендлера или заглушки.
		{name: "ok путь", method: "GET", path: "/api/v1/dictionaries/countries", wantStatus: 200},
		{name: "ok query -> заглушка", method: "GET", path: "/api/v1/applications?page=1&page_size=20", wantStatus: 501, wantCode: "NOT_IMPLEMENTED"},
		{name: "ok тело -> заглушка", method: "PATCH", path: "/api/v1/organizations/me", body: `{"name":"Acme GmbH"}`, ctype: "application/json", wantStatus: 501, wantCode: "NOT_IMPLEMENTED"},
		{name: "ok uuid v7 в пути", method: "GET", path: "/api/v1/applications/019245a0-1b2c-7def-8000-000000000001", wantStatus: 501},
		// HEAD: ServeMux сопоставляет его с GET-паттернами, валидатор (gorillamux) — нет; раньше было 200.
		{name: "HEAD как GET", method: "HEAD", path: "/api/v1/dictionaries/countries", wantStatus: 200},

		// Query.
		{name: "page=0 (minimum)", method: "GET", path: "/api/v1/applications?page=0", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "page"},
		{name: "status вне enum", method: "GET", path: "/api/v1/applications?status=bogus", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "status"},
		{name: "amount_min не по паттерну", method: "GET", path: "/api/v1/applications?amount_min=abc", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "amount_min"},
		{name: "page_size > maximum", method: "GET", path: "/api/v1/applications?page_size=1000", wantStatus: 400, wantCode: "VALIDATION_ERROR", wantField: "page_size"},

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
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.ctype != "" {
				req.Header.Set("Content-Type", tt.ctype)
			}
			if needsTestAuthentication(tt.method, tt.path) {
				addValidTestSession(req)
			}
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
		// мусорное тело доходит до заглушки — 501, а не 400.
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/organizations/me/documents", strings.NewReader("garbage"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		addValidTestSession(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 501; body %s", resp.StatusCode, raw)
		}
	})
}

func needsTestAuthentication(method, path string) bool {
	switch {
	case method == http.MethodGet && path == "/api/v1/healthz":
		return false
	case method == http.MethodGet && (path == "/api/v1/dictionaries/countries" ||
		path == "/api/v1/dictionaries/currencies" ||
		path == "/api/v1/dictionaries/transfer-methods"):
		return false
	case method == http.MethodPost && (path == "/api/v1/auth/register" ||
		path == "/api/v1/auth/login" ||
		path == "/api/v1/registry/lookup"):
		return false
	default:
		return true
	}
}
