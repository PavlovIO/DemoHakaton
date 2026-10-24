package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeChat запоминает аргументы и отдаёт заданные сообщения или ошибку.
type fakeChat struct {
	messages []service.Message
	err      error
	gotOffer uuid.UUID
	gotAfter int64
	gotLimit int32
	gotText  *string
}

func (f *fakeChat) List(_ context.Context, _ service.Principal, offerID uuid.UUID, afterID int64, limit int32) ([]service.Message, error) {
	f.gotOffer, f.gotAfter, f.gotLimit = offerID, afterID, limit
	return f.messages, f.err
}

func (f *fakeChat) Send(_ context.Context, _ service.Principal, offerID uuid.UUID, text string) (service.Message, error) {
	f.gotOffer, f.gotText = offerID, &text
	if f.err != nil {
		return service.Message{}, f.err
	}
	m := testMessage()
	m.Text, m.OfferID = text, offerID
	return m, nil
}

func testMessage() service.Message {
	return service.Message{
		ID: 7, OfferID: uuid.New(), AuthorUserID: uuid.New(), AuthorRole: repository.UserRoleBorrower,
		AuthorOrg: service.OrgShort{ID: uuid.New(), Name: "ООО «Альфа»", CountryCode: "RU"},
		Text:      "Здравствуйте", IsMine: true, CreatedAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
	}
}

func newChatServer(t *testing.T, chat *fakeChat) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Chat = fakeDicts{}, okDB{}, chat
	return newTestServer(t, deps)
}

func messagesPath(offerID uuid.UUID) string {
	return "/api/v1/offers/" + offerID.String() + "/messages"
}

func TestListMessages(t *testing.T) {
	t.Run("без параметров — after_id 0, limit 50", func(t *testing.T) {
		chat := &fakeChat{messages: []service.Message{testMessage()}}
		srv := newChatServer(t, chat)
		offerID := uuid.New()
		resp, body := sessionRequest(t, srv, "lender-verified", "GET", messagesPath(offerID), "", nil)
		if resp.StatusCode != http.StatusOK || chat.gotOffer != offerID || chat.gotAfter != 0 || chat.gotLimit != 50 {
			t.Fatalf("status = %d, offer %s after %d limit %d; body %s", resp.StatusCode, chat.gotOffer, chat.gotAfter, chat.gotLimit, body)
		}
		var raw []map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatal(err)
		}
		org, _ := raw[0]["author_organization"].(map[string]any)
		if len(raw) != 1 || raw[0]["id"] != float64(7) || raw[0]["author_role"] != "borrower" || raw[0]["is_mine"] != true ||
			raw[0]["text"] != "Здравствуйте" || org["name"] != "ООО «Альфа»" || org["country_code"] != "RU" {
			t.Fatalf("body = %s", body)
		}
	})

	t.Run("after_id=10&limit=200 — переданы как есть", func(t *testing.T) {
		chat := &fakeChat{}
		srv := newChatServer(t, chat)
		resp, body := sessionRequest(t, srv, "lender-verified", "GET", messagesPath(uuid.New())+"?after_id=10&limit=200", "", nil)
		if resp.StatusCode != http.StatusOK || chat.gotAfter != 10 || chat.gotLimit != 200 {
			t.Fatalf("status = %d, after %d limit %d; body %s", resp.StatusCode, chat.gotAfter, chat.gotLimit, body)
		}
		if strings.TrimSpace(string(body)) != "[]" {
			t.Fatalf("body = %q, want []", body)
		}
	})

	t.Run("limit=201 — 400 VALIDATION_ERROR limit", func(t *testing.T) {
		chat := &fakeChat{}
		srv := newChatServer(t, chat)
		resp, body := sessionRequest(t, srv, "lender-verified", "GET", messagesPath(uuid.New())+"?limit=201", "", nil)
		wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "limit")
	})
}

func TestSendMessage(t *testing.T) {
	t.Run("HTML в тексте — 201, текст байт в байт", func(t *testing.T) {
		chat := &fakeChat{}
		srv := newChatServer(t, chat)
		offerID := uuid.New()
		const text = "<script>alert(1)</script>"
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", messagesPath(offerID), "application/json",
			strings.NewReader(`{"text":"<script>alert(1)</script>"}`))
		if resp.StatusCode != http.StatusCreated || chat.gotOffer != offerID || chat.gotText == nil || *chat.gotText != text {
			t.Fatalf("status = %d, got text %v; body %s", resp.StatusCode, chat.gotText, body)
		}
		var got api.Message
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Text != text || got.OfferID != offerID {
			t.Fatalf("message = %+v", got)
		}
	})

	for name, text := range map[string]string{"пустой": "", "4001 символ": strings.Repeat("я", 4001)} {
		t.Run(name+" текст — 400, сервис не вызван", func(t *testing.T) {
			chat := &fakeChat{}
			srv := newChatServer(t, chat)
			payload, _ := json.Marshal(api.MessageCreate{Text: text})
			resp, body := sessionRequest(t, srv, "borrower-verified", "POST", messagesPath(uuid.New()), "application/json", bytes.NewReader(payload))
			wantError(t, resp, body, http.StatusBadRequest, "VALIDATION_ERROR", "text")
			if chat.gotText != nil {
				t.Fatalf("сервис вызван с %q", *chat.gotText)
			}
		})
	}

	t.Run("RateLimitedAfter(1500ms) — 429, Retry-After: 2", func(t *testing.T) {
		srv := newChatServer(t, &fakeChat{err: apperr.RateLimitedAfter(1500 * time.Millisecond)})
		resp, body := sessionRequest(t, srv, "borrower-verified", "POST", messagesPath(uuid.New()), "application/json", strings.NewReader(`{"text":"x"}`))
		wantError(t, resp, body, http.StatusTooManyRequests, "RATE_LIMITED", "")
		if got := resp.Header.Get("Retry-After"); got != "2" {
			t.Fatalf("Retry-After = %q, want 2", got)
		}
	})

	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"Forbidden — 403", apperr.Forbidden("Администратор не пишет в чат"), http.StatusForbidden, "FORBIDDEN"},
		{"InvalidTransition — 409", apperr.New(apperr.CodeInvalidTransition, "Чат закрыт: оффер в статусе rejected"), http.StatusConflict, "INVALID_STATUS_TRANSITION"},
		{"NotFound — 404", apperr.NotFound("Оффер не найден"), http.StatusNotFound, "NOT_FOUND"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newChatServer(t, &fakeChat{err: tt.err})
			resp, body := sessionRequest(t, srv, "lender-verified", "POST", messagesPath(uuid.New()), "application/json", strings.NewReader(`{"text":"x"}`))
			wantError(t, resp, body, tt.status, tt.code, "")
		})
	}
}
