package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/config"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// idorExempt — операции с параметром-id в пути, к которым IDOR-строка неприменима.
var idorExempt = map[string]string{
	"/admin/": "администратор видит все организации; чужой роли — 403, это TestAccessOrder",
}

// TestIDOR — чужой id любого ресурса даёт 404 NOT_FOUND, неотличимый от несуществующего id,
// а мутация чужой сущности её не меняет. Граф зависимостей — прод (newRouter), БД — настоящая:
// фейковый сервис не проверил бы условие владения в SQL.
func TestIDOR(t *testing.T) {
	pool := servicetest.NewPool(t)
	router, err := newRouter(pool, config.Config{SessionTTL: time.Hour, DocumentStorageDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	f := newIDORFixture(t, srv, pool)

	type attempt struct {
		method, path, body string // path с %s на месте id
		id                 string
		missingID          string
		foreign            []*actor
		unchanged          func(t *testing.T) // состояние в БД после попыток; nil — GET
	}
	missingUUID := uuid.NewString()
	const missingNotification = "9223372036854775000"

	draftUnchanged := func(t *testing.T) {
		f.wantColumn(t, "SELECT status::text || '|' || purpose FROM applications WHERE id = $1", f.draftApp, "draft|"+idorPurpose)
		f.wantColumn(t, "SELECT count(*)::text FROM offers WHERE application_id = $1", f.draftApp, "0")
	}
	pendingOfferUnchanged := func(t *testing.T) {
		f.wantColumn(t, "SELECT status::text FROM offers WHERE id = $1", f.pendingOffer, "sent")
		f.wantColumn(t, "SELECT status::text FROM applications WHERE id = $1", f.publishedApp, "published")
	}
	messagesUnchanged := func(t *testing.T) {
		f.wantColumn(t, "SELECT count(*)::text FROM messages WHERE offer_id = $1", f.acceptedOffer, "1")
	}

	attempts := []attempt{
		{method: "GET", path: "/applications/%s", id: f.publishedApp, foreign: []*actor{f.b}},
		{method: "PATCH", path: "/applications/%s", body: `{"purpose":"Чужая правка назначения платежа"}`, id: f.draftApp, foreign: []*actor{f.b}, unchanged: draftUnchanged},
		{method: "POST", path: "/applications/%s/submit", id: f.draftApp, foreign: []*actor{f.b}, unchanged: draftUnchanged},
		{method: "POST", path: "/applications/%s/cancel", id: f.draftApp, foreign: []*actor{f.b}, unchanged: draftUnchanged},
		{method: "POST", path: "/applications/%s/reject-offers", id: f.publishedApp, foreign: []*actor{f.b}, unchanged: pendingOfferUnchanged},
		{method: "GET", path: "/applications/%s/history", id: f.publishedApp, foreign: []*actor{f.b}},
		{method: "GET", path: "/applications/%s/offers", id: f.publishedApp, foreign: []*actor{f.b}},
		// Неопубликованная заявка кредитору не видна — оффер на неё невозможен.
		{method: "POST", path: "/applications/%s/offers", body: idorOfferBody, id: f.draftApp, foreign: []*actor{f.l2}, unchanged: draftUnchanged},
		{method: "GET", path: "/offers/%s", id: f.pendingOffer, foreign: []*actor{f.b, f.l2}},
		{method: "POST", path: "/offers/%s/accept", id: f.pendingOffer, foreign: []*actor{f.b}, unchanged: pendingOfferUnchanged},
		{method: "POST", path: "/offers/%s/reject", id: f.pendingOffer, foreign: []*actor{f.b}, unchanged: pendingOfferUnchanged},
		{method: "POST", path: "/offers/%s/withdraw", id: f.pendingOffer, foreign: []*actor{f.l2}, unchanged: pendingOfferUnchanged},
		{method: "GET", path: "/deals/%s", id: f.deal, foreign: []*actor{f.b, f.l2}},
		{method: "GET", path: "/offers/%s/messages", id: f.acceptedOffer, foreign: []*actor{f.b, f.l2}},
		{method: "POST", path: "/offers/%s/messages", body: `{"text":"чужое сообщение"}`, id: f.acceptedOffer, foreign: []*actor{f.b, f.l2}, unchanged: messagesUnchanged},
		{method: "GET", path: "/documents/%s/file", id: f.document, foreign: []*actor{f.b}},
		// C в draft: у verified удаление отбивает статус (409) раньше проверки владения.
		{method: "DELETE", path: "/organizations/me/documents/%s", id: f.document, foreign: []*actor{f.c}, unchanged: func(t *testing.T) {
			f.wantColumn(t, "SELECT count(*)::text FROM kyc_documents WHERE id = $1", f.document, "1")
		}},
		{method: "POST", path: "/notifications/%s/read", id: f.notification, missingID: missingNotification, foreign: []*actor{f.b}, unchanged: func(t *testing.T) {
			f.wantColumn(t, "SELECT is_read::text FROM notifications WHERE id = $1::bigint", f.notification, "false")
		}},
	}

	covered := map[string]bool{}
	for _, a := range attempts {
		covered[a.method+" "+strings.ReplaceAll(a.path, "%s", "{}")] = true
		missing := a.missingID
		if missing == "" {
			missing = missingUUID
		}
		for _, who := range a.foreign {
			t.Run(fmt.Sprintf("%s %s as %s", a.method, a.path, who.name), func(t *testing.T) {
				foreignStatus, foreignErr := who.call(t, a.method, fmt.Sprintf(a.path, a.id), a.body)
				missingStatus, missingErr := who.call(t, a.method, fmt.Sprintf(a.path, missing), a.body)

				if foreignStatus != http.StatusNotFound || foreignErr.Code != "NOT_FOUND" {
					t.Errorf("foreign id: status %d code %q, want 404 NOT_FOUND", foreignStatus, foreignErr.Code)
				}
				// Ответ на чужой id не должен выдавать, что сущность существует.
				if missingStatus != foreignStatus || missingErr != foreignErr {
					t.Errorf("foreign %d %+v differs from missing %d %+v", foreignStatus, foreignErr, missingStatus, missingErr)
				}
				if a.unchanged != nil {
					a.unchanged(t)
				}
			})
		}
	}

	t.Run("every id path is covered", func(t *testing.T) {
		for _, op := range idPathOperations() {
			if covered[op] || isIDORExempt(op) {
				continue
			}
			t.Errorf("%s: нет строки в TestIDOR (или причины в idorExempt)", op)
		}
	})
}

// pathParam — параметр пути спеки: {applicationId}, {offerId}, ...
var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// idPathOperations — «METHOD /path» всех операций спеки с параметром в пути; параметр — {}.
func idPathOperations() []string {
	var ops []string
	for path, item := range openapi.MustDoc().Paths.Map() {
		if !strings.Contains(path, "{") {
			continue
		}
		normalized := pathParam.ReplaceAllString(path, "{}")
		for method := range item.Operations() {
			ops = append(ops, method+" "+normalized)
		}
	}
	slices.Sort(ops)
	return ops
}

func isIDORExempt(op string) bool {
	_, path, _ := strings.Cut(op, " ")
	for prefix := range idorExempt {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

const (
	idorPurpose   = "Оплата поставки оборудования"
	idorOfferBody = `{"amount":"500000.00","interest_rate_annual":"12.50","repayment_schedule":"bullet"}`
	idorAppBody   = `{"amount":"1000000.00","currency":"USD","target_country_code":"DE",` +
		`"purpose":"` + idorPurpose + `","term_months":12,"transfer_method":"swift"}`
)

// actor — пользователь со своей cookie-сессией.
type actor struct {
	name   string
	srv    *httptest.Server
	client *http.Client
	orgID  string
}

type apiError struct{ Code, Message string }

// call шлёт запрос от имени actor с CSRF-заголовком из его cookie и возвращает
// статус и тело ошибки (пустое при 2xx).
func (a *actor) call(t *testing.T, method, path, body string) (int, apiError) {
	t.Helper()
	status, raw := a.send(t, method, path, "application/json", strings.NewReader(body), body != "")
	var e struct{ Error apiError }
	if status >= 400 {
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
	return status, e.Error
}

func (a *actor) send(t *testing.T, method, path, ctype string, body io.Reader, hasBody bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, a.srv.URL+handler.BaseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if hasBody {
		req.Header.Set("Content-Type", ctype)
	}
	u, _ := url.Parse(a.srv.URL)
	for _, c := range a.client.Jar.Cookies(u) {
		if c.Name == middleware.CSRFCookieName {
			req.Header.Set(middleware.CSRFHeaderName, c.Value)
		}
	}
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

// mustJSON — запрос, который обязан пройти со статусом want; тело разбирается в out.
func (a *actor) mustJSON(t *testing.T, method, path, body string, want int, out any) {
	t.Helper()
	status, raw := a.send(t, method, path, "application/json", strings.NewReader(body), body != "")
	if status != want {
		t.Fatalf("%s %s as %s: status %d, want %d; body %s", method, path, a.name, status, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
}

type idorFixture struct {
	pool                   *pgxpool.Pool
	track                  func(orgID uuid.UUID) // servicetest.CleanupCommittedOrgs
	a, b, l1, l2           *actor
	c                      *actor // заёмщик с KYC в draft
	draftApp, publishedApp string
	pendingOffer           string // на publishedApp, статус sent
	acceptedOffer, deal    string // на третьей заявке A; в чате одно сообщение A
	document, notification string
}

// newIDORFixture: заёмщики A, B, C (KYC в draft) и кредиторы L1, L2 из разных организаций. У A — черновик,
// опубликованная заявка с офером L1 в статусе sent, сделка по другой заявке с сообщением в
// чате, KYC-документ и уведомление о новом оффере.
func newIDORFixture(t *testing.T, srv *httptest.Server, pool *pgxpool.Pool) *idorFixture {
	t.Helper()
	// Компании регистрируются через API и коммитятся; удаляет их владелец схемы в t.Cleanup.
	f := &idorFixture{pool: pool, track: servicetest.CleanupCommittedOrgs(t)}
	f.a = f.register(t, srv, "A", "borrower")
	f.b = f.register(t, srv, "B", "borrower")
	f.l1 = f.register(t, srv, "L1", "lender")
	f.l2 = f.register(t, srv, "L2", "lender")
	f.c = f.register(t, srv, "C", "borrower")

	// Документ — до верификации: загрузка разрешена только в draft/rejected.
	f.document = f.uploadDocument(t, f.a)
	for _, who := range []*actor{f.a, f.b, f.l1, f.l2} {
		f.exec(t, "UPDATE organizations SET kyc_status = 'verified', verified_at = now() WHERE id = $1", who.orgID)
	}

	f.draftApp = f.createApplication(t)
	f.publishedApp = f.publishedApplication(t)
	f.pendingOffer = f.makeOffer(t, f.publishedApp)

	agreedApp := f.publishedApplication(t)
	f.acceptedOffer = f.makeOffer(t, agreedApp)
	f.a.mustJSON(t, "POST", "/offers/"+f.acceptedOffer+"/messages", `{"text":"Обсудим условия"}`, http.StatusCreated, nil)
	var deal struct{ ID string }
	f.a.mustJSON(t, "POST", "/offers/"+f.acceptedOffer+"/accept", "", http.StatusOK, &deal)
	f.deal = deal.ID

	var notes struct{ Items []struct{ ID int64 } }
	f.a.mustJSON(t, "GET", "/notifications?unread_only=true", "", http.StatusOK, &notes)
	if len(notes.Items) == 0 {
		t.Fatal("A has no unread notifications after offers")
	}
	f.notification = fmt.Sprint(notes.Items[0].ID)
	return f
}

func (f *idorFixture) register(t *testing.T, srv *httptest.Server, name, role string) *actor {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	who := &actor{name: name, srv: srv, client: &http.Client{Jar: jar}}
	suffix := uuid.NewString()
	body := fmt.Sprintf(`{"email":"idor-%s@example.com","password":"idor-password-1","role":%q,`+
		`"organization":{"name":"IDOR Test %s","country_code":%q,"registration_number":%q,"legal_address":"Limassol"}}`,
		suffix, role, suffix[:8], countryFor(role), servicetest.UniqueRegistrationNumber())
	var me struct{ Organization struct{ ID string } }
	who.mustJSON(t, "POST", "/auth/register", body, http.StatusCreated, &me)
	if me.Organization.ID == "" {
		t.Fatalf("register %s: no organization id", name)
	}
	who.orgID = me.Organization.ID
	f.track(uuid.MustParse(who.orgID))
	return who
}

func (f *idorFixture) uploadDocument(t *testing.T, who *actor) string {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if err := form.WriteField("doc_type", "charter"); err != nil {
		t.Fatal(err)
	}
	file, err := form.CreateFormFile("file", "charter.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("%PDF-1.4\n%idor test\n"))
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	status, raw := who.send(t, "POST", "/organizations/me/documents", form.FormDataContentType(), &buf, true)
	if status != http.StatusCreated {
		t.Fatalf("upload document: status %d; body %s", status, raw)
	}
	var doc struct{ ID string }
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.ID
}

func (f *idorFixture) createApplication(t *testing.T) string {
	t.Helper()
	var app struct{ ID string }
	f.a.mustJSON(t, "POST", "/applications", idorAppBody, http.StatusCreated, &app)
	return app.ID
}

// publishedApplication — заявка A сразу в published: модерация не предмет этого теста.
func (f *idorFixture) publishedApplication(t *testing.T) string {
	t.Helper()
	id := f.createApplication(t)
	f.exec(t, "UPDATE applications SET status = 'published', published_at = now() WHERE id = $1", id)
	return id
}

func (f *idorFixture) makeOffer(t *testing.T, applicationID string) string {
	t.Helper()
	var offer struct{ ID string }
	f.l1.mustJSON(t, "POST", "/applications/"+applicationID+"/offers", idorOfferBody, http.StatusCreated, &offer)
	return offer.ID
}

func (f *idorFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f *idorFixture) wantColumn(t *testing.T, sql, id, want string) {
	t.Helper()
	var got string
	if err := f.pool.QueryRow(context.Background(), sql, id).Scan(&got); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	if got != want {
		t.Errorf("%s [%s] = %q, want %q", sql, id, got, want)
	}
}

// countryFor — заёмщик по правилам платформы из RU, кредитор — иностранный.
func countryFor(role string) string {
	if role == "borrower" {
		return "RU"
	}
	return "CY"
}
