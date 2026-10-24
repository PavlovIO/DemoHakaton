package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{apperr.Validation("bad", apperr.FieldError{Field: "amount", Message: "must be > 0"}), 400, "VALIDATION_ERROR"},
		{apperr.Unauthorized("no session"), 401, "UNAUTHORIZED"},
		{apperr.Forbidden("no"), 403, "FORBIDDEN"},
		{apperr.CSRFInvalid(), 403, "CSRF_INVALID"},
		{apperr.KYCNotVerified(), 403, "KYC_NOT_VERIFIED"},
		{apperr.NotFound("nope"), 404, "NOT_FOUND"},
		{apperr.Conflict("email taken"), 409, "CONFLICT"},
		{apperr.InvalidTransition("published", "draft"), 409, "INVALID_STATUS_TRANSITION"},
		{apperr.FileTooLarge(), 413, "FILE_TOO_LARGE"},
		{apperr.RateLimited(), 429, "RATE_LIMITED"},
		{apperr.NotImplemented("listDeals"), 501, "NOT_IMPLEMENTED"},
		{apperr.Unavailable("db down", errors.New("dial")), 503, "UNAVAILABLE"},
		{apperr.UpstreamUnavailable("registry down", errors.New("timeout")), 502, "UPSTREAM_UNAVAILABLE"},
		{errors.New("boom"), 500, "INTERNAL"},
		// Доменная ошибка, обёрнутая через fmt.Errorf("...: %w"), всё равно распознаётся.
		{wrap(apperr.NotFound("nope")), 404, "NOT_FOUND"},
	}

	for _, tt := range tests {
		t.Run(tt.wantCode, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			handler.WriteError(rec, req, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			e := decodeError(t, rec.Body.Bytes())
			if e.Error.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s", e.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestWriteErrorRetryAfter(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantRetryAfter string
	}{
		{name: "90 seconds", err: apperr.RateLimitedAfter(90 * time.Second), wantStatus: http.StatusTooManyRequests, wantRetryAfter: "90"},
		{name: "round up milliseconds", err: apperr.RateLimitedAfter(1500 * time.Millisecond), wantStatus: http.StatusTooManyRequests, wantRetryAfter: "2"},
		{name: "minimum one second", err: apperr.RateLimitedAfter(100 * time.Millisecond), wantStatus: http.StatusTooManyRequests, wantRetryAfter: "1"},
		{name: "wrapped rate limit", err: fmt.Errorf("x: %w", apperr.RateLimitedAfter(30*time.Second)), wantStatus: http.StatusTooManyRequests, wantRetryAfter: "30"},
		{name: "rate limit without retry duration", err: apperr.RateLimited(), wantStatus: http.StatusTooManyRequests},
		{name: "not found", err: apperr.NotFound("nope"), wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), tt.err)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Retry-After"); got != tt.wantRetryAfter {
				t.Fatalf("Retry-After = %q, want %q", got, tt.wantRetryAfter)
			}
		})
	}
}

func TestErrorMappingDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	handler.WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil),
		apperr.Validation("bad", apperr.FieldError{Field: "amount", Message: "must be > 0"}))

	e := decodeError(t, rec.Body.Bytes())
	if e.Error.Details == nil || len(e.Error.Details) != 1 || e.Error.Details[0].Field != "amount" {
		t.Fatalf("details = %+v", e.Error.Details)
	}
}

// Каждый код из apperr должен иметь явный статус: новый код без строки в statusByCode
// молча превратился бы в 500.
func TestEveryCodeHasStatus(t *testing.T) {
	codes := []apperr.Code{
		apperr.CodeValidation, apperr.CodeUnauthorized, apperr.CodeForbidden, apperr.CodeCSRFInvalid,
		apperr.CodeKYCNotVerified, apperr.CodeNotFound, apperr.CodeConflict, apperr.CodeInvalidTransition,
		apperr.CodeRateLimited, apperr.CodeInternal, apperr.CodeNotImplemented, apperr.CodeUnavailable,
		apperr.CodeFileTooLarge, apperr.CodeUpstreamUnavailable,
	}
	for _, c := range codes {
		if s := handler.StatusOf(c); s == http.StatusInternalServerError && c != apperr.CodeInternal {
			t.Errorf("code %s has no status mapping", c)
		}
	}
}

// Ошибки, которые возникают до вызова хендлера, тоже приходят в формате Error.
func TestRouterErrors(t *testing.T) {
	srv := newTestServer(t, handler.Deps{Dicts: fakeDicts{}, DB: okDB{}, Auth: testAuthService()})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{"invalid uuid in path", "/api/v1/applications/not-a-uuid", 400, "VALIDATION_ERROR", "applicationId"},
		{"invalid query param", "/api/v1/applications?page=abc", 400, "VALIDATION_ERROR", "page"},
		{"unknown route", "/api/v1/nope", 404, "NOT_FOUND", ""},
		{"not implemented yet", "/api/v1/notifications", 501, "NOT_IMPLEMENTED", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := get(t, srv, tt.path, true)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, tt.wantStatus, body)
			}
			e := decodeError(t, body)
			if e.Error.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s", e.Error.Code, tt.wantCode)
			}
			if tt.wantField != "" {
				if e.Error.Details == nil || e.Error.Details[0].Field != tt.wantField {
					t.Fatalf("details = %+v, want field %s", e.Error.Details, tt.wantField)
				}
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	t.Run("db ok", func(t *testing.T) {
		srv := newTestServer(t, handler.Deps{DB: okDB{}})
		resp, body := get(t, srv, "/api/v1/healthz")
		if resp.StatusCode != http.StatusOK || string(body) != "{\"status\":\"ok\"}\n" {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
	})
	t.Run("db down", func(t *testing.T) {
		srv := newTestServer(t, handler.Deps{DB: downDB{}})
		resp, body := get(t, srv, "/api/v1/healthz")
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", resp.StatusCode)
		}
		if e := decodeError(t, body); e.Error.Code != "UNAVAILABLE" {
			t.Fatalf("code = %s, want UNAVAILABLE", e.Error.Code)
		}
	})
}

type okDB struct{}

func (okDB) Ping(context.Context) error { return nil }

type downDB struct{}

func (downDB) Ping(context.Context) error { return errors.New("connection refused") }

func wrap(err error) error { return errors.Join(errors.New("context"), err) }
