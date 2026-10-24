package handler_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
)

// lockedBuffer — буфер логов: сервер пишет из своей горутины, тест читает из своей.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// lines разбирает JSON-строки лога и сбрасывает буфер.
func (b *lockedBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	scanner := bufio.NewScanner(&b.buf)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("log line %q: %v", scanner.Text(), err)
		}
		out = append(out, line)
	}
	b.buf.Reset()
	return out
}

// captureLogs подменяет slog.Default на JSON-логгер с ContextHandler, как в cmd/server.
func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	logs := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(middleware.ContextHandler{
		Handler: slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return logs
}

func findLine(lines []map[string]any, msg string) map[string]any {
	for _, line := range lines {
		if line["msg"] == msg {
			return line
		}
	}
	return nil
}

func requestWithID(t *testing.T, url, requestID string, withSession bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	if withSession {
		addValidTestSession(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp
}

// Строку RequestLog и строку ошибки WriteError одного запроса связывает request_id,
// и он же уходит клиенту в X-Request-ID.
func TestRequestIDCorrelatesLogLines(t *testing.T) {
	logs := captureLogs(t)
	srv := newTestServer(t, handler.Deps{DB: okDB{}})

	t.Run("генерируется и попадает в обе строки", func(t *testing.T) {
		resp := requestWithID(t, srv.URL+"/api/v1/nope", "", false)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		id := resp.Header.Get("X-Request-ID")
		if id == "" {
			t.Fatal("X-Request-ID is empty")
		}
		lines := logs.lines(t)
		rejected, access := findLine(lines, "request rejected"), findLine(lines, "http request")
		if rejected == nil || access == nil {
			t.Fatalf("want both log lines, got %v", lines)
		}
		if rejected["request_id"] != id || access["request_id"] != id {
			t.Fatalf("request_id = %v / %v, want %s", rejected["request_id"], access["request_id"], id)
		}
	})

	t.Run("валидный id клиента сохраняется", func(t *testing.T) {
		resp := requestWithID(t, srv.URL+"/api/v1/healthz", "abc-123", false)
		if got := resp.Header.Get("X-Request-ID"); got != "abc-123" {
			t.Fatalf("X-Request-ID = %q, want abc-123", got)
		}
		if access := findLine(logs.lines(t), "http request"); access == nil || access["request_id"] != "abc-123" {
			t.Fatalf("access line = %v, want request_id abc-123", access)
		}
	})

	// Перевод строки в id позволил бы подделать строку лога в текстовом формате.
	t.Run("невалидный id заменяется", func(t *testing.T) {
		for _, bad := range []string{"a\tb", "x y", string(bytes.Repeat([]byte("a"), 65))} {
			resp := requestWithID(t, srv.URL+"/api/v1/healthz", bad, false)
			if got := resp.Header.Get("X-Request-ID"); got == "" || got == bad {
				t.Fatalf("X-Request-ID for %q = %q, want generated", bad, got)
			}
		}
		logs.lines(t)
	})
}

// С сессией строка access log несёт user_id и org_id, без сессии — нет.
func TestRequestLogHasPrincipal(t *testing.T) {
	logs := captureLogs(t)
	fixture, deps := authFixture()
	deps.DB = okDB{}
	srv := newTestServer(t, deps)

	requestWithID(t, srv.URL+"/api/v1/healthz", "", true)
	access := findLine(logs.lines(t), "http request")
	if access == nil || access["user_id"] != fixture.principal.UserID.String() || access["org_id"] != fixture.principal.OrgID.String() {
		t.Fatalf("access line = %v, want user_id %s and org_id %s", access, fixture.principal.UserID, *fixture.principal.OrgID)
	}

	requestWithID(t, srv.URL+"/api/v1/healthz", "", false)
	access = findLine(logs.lines(t), "http request")
	if _, ok := access["user_id"]; ok || access == nil {
		t.Fatalf("anonymous access line = %v, want no user_id", access)
	}
}
