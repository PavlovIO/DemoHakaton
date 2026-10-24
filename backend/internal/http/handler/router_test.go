package handler_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
)

type panicDB struct{}

func (panicDB) Ping(context.Context) error { panic("boom") }

// Паника в хендлере -> 500, и запрос всё равно попадает в access log:
// RequestLog стоит снаружи Recover.
func TestRouterPanicIsLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv := newTestServer(t, handler.Deps{DB: panicDB{}})
	resp, body := get(t, srv, "/api/v1/healthz")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(logs.String(), `msg="http request"`) || !strings.Contains(logs.String(), "status=500") {
		t.Fatalf("access log has no 500 line:\n%s", logs.String())
	}
}

// HEAD к публичной операции не требует сессии, как и GET.
func TestRouterHeadPublicWithoutSession(t *testing.T) {
	srv := newTestServer(t, handler.Deps{DB: okDB{}})
	req, err := http.NewRequest(http.MethodHead, srv.URL+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD /healthz status = %d, want 200", resp.StatusCode)
	}
}

func TestNewRouterPanicsWithoutAuth(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewRouter without Deps.Auth did not panic")
		}
	}()
	handler.NewRouter(handler.New(handler.Deps{DB: okDB{}}))
}
