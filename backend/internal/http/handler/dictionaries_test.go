package handler_test

// Образец теста операции. Тест поднимает НАСТОЯЩИЙ роутер (mux → strict-обёртка → Handler →
// маппинг ошибок), а сервис подменяет фейком. Так проверяется весь HTTP-контракт операции:
// путь, статус, JSON-форма ответа и формат ошибок. Логика сервиса тестируется отдельно в service.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

type fakeDicts struct {
	countries []repository.ListCountriesRow
	err       error
}

func (f fakeDicts) Countries(context.Context) ([]repository.ListCountriesRow, error) {
	return f.countries, f.err
}

func (f fakeDicts) Currencies(context.Context) ([]repository.ListCurrenciesRow, error) {
	return []repository.ListCurrenciesRow{{Code: "EUR", Name: "Евро"}}, f.err
}

func (f fakeDicts) TransferMethods(context.Context) ([]repository.TransferMethod, error) {
	return []repository.TransferMethod{{Code: "swift", Name: "SWIFT"}}, f.err
}

// newTestServer — общий хелпер: роутер с фейковыми зависимостями.
func newTestServer(t *testing.T, deps handler.Deps) *httptest.Server {
	t.Helper()
	if deps.Auth == nil { // без Auth NewRouter паникует
		deps.Auth = testAuthService()
	}
	srv := httptest.NewServer(handler.NewRouter(handler.New(deps)))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, authenticated ...bool) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if len(authenticated) > 0 && authenticated[0] {
		addValidTestSession(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

func TestListCountries(t *testing.T) {
	tests := []struct {
		name       string
		dicts      fakeDicts
		wantStatus int
		check      func(t *testing.T, body []byte)
	}{
		{
			name: "ok",
			dicts: fakeDicts{countries: []repository.ListCountriesRow{
				{Code: "DE", NameRu: "Германия", NameEn: "Germany", FatfHighRisk: false},
				{Code: "IR", NameRu: "Иран", NameEn: "Iran", FatfHighRisk: true},
			}},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				var got []api.Country
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				want := []api.Country{
					{Code: "DE", NameRu: "Германия", NameEn: "Germany", FATFHighRisk: false},
					{Code: "IR", NameRu: "Иран", NameEn: "Iran", FATFHighRisk: true},
				}
				if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
					t.Fatalf("got %+v, want %+v", got, want)
				}
				// Поле обязано называться ровно так, как в спеке.
				if !strings.Contains(string(body), `"fatf_high_risk":true`) {
					t.Fatalf("body has no fatf_high_risk: %s", body)
				}
			},
		},
		{
			name:       "empty list is [] not null",
			dicts:      fakeDicts{countries: []repository.ListCountriesRow{}},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				if strings.TrimSpace(string(body)) != "[]" {
					t.Fatalf("body = %s, want []", body)
				}
			},
		},
		{
			name:       "service error is hidden behind INTERNAL",
			dicts:      fakeDicts{err: errors.New("pq: connection refused to 10.0.0.5")},
			wantStatus: http.StatusInternalServerError,
			check: func(t *testing.T, body []byte) {
				e := decodeError(t, body)
				if e.Error.Code != "INTERNAL" {
					t.Fatalf("code = %s, want INTERNAL", e.Error.Code)
				}
				if strings.Contains(string(body), "10.0.0.5") {
					t.Fatalf("internal error details leaked: %s", body)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, handler.Deps{Dicts: tt.dicts})
			resp, body := get(t, srv, "/api/v1/dictionaries/countries")
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, tt.wantStatus, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q", ct)
			}
			tt.check(t, body)
		})
	}
}

func TestListCurrenciesAndTransferMethods(t *testing.T) {
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}})

	_, body := get(t, srv, "/api/v1/dictionaries/currencies")
	if strings.TrimSpace(string(body)) != `[{"code":"EUR","name":"Евро"}]` {
		t.Fatalf("currencies body = %s", body)
	}

	_, body = get(t, srv, "/api/v1/dictionaries/transfer-methods")
	if strings.TrimSpace(string(body)) != `[{"code":"swift","name":"SWIFT"}]` {
		t.Fatalf("transfer-methods body = %s", body)
	}
}

func decodeError(t *testing.T, body []byte) api.Error {
	t.Helper()
	var e api.Error
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode Error: %v; body: %s", err, body)
	}
	if e.Error.Code == "" || e.Error.Message == "" {
		t.Fatalf("Error without code/message: %s", body)
	}
	return e
}
