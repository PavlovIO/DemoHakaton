package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// Атрибуты запроса — в корне строки, даже если логгер открыл группу: по request_id
// ищут строки одного запроса.
func TestContextHandlerAttrsOutsideGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(ContextHandler{Handler: slog.NewJSONHandler(&buf, nil)})
	ctx := context.WithValue(context.Background(), requestIDContextKey{}, "req-1")
	ctx = context.WithValue(ctx, logAttrsContextKey{}, &logAttrs{})
	WithLogAttrs(ctx, slog.String("user_id", "u-1"))

	logger.With("svc", "admin").WithGroup("db").With("table", "audit_log").InfoContext(ctx, "query", "rows", 3)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	if line["request_id"] != "req-1" || line["user_id"] != "u-1" || line["svc"] != "admin" {
		t.Errorf("корневые атрибуты: %v", line)
	}
	db, _ := line["db"].(map[string]any)
	if db["table"] != "audit_log" || db["rows"] != float64(3) {
		t.Errorf("группа db = %v, want table и rows", line["db"])
	}
	if _, ok := db["request_id"]; ok {
		t.Errorf("request_id попал в группу: %v", db)
	}
}
