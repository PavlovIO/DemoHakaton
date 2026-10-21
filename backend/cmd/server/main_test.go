package main

// Интеграционный тест жизненного цикла: старт с реальной БД → /healthz 200 → отмена
// контекста (как Ctrl+C) → run возвращает nil, порт освобождён.

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestRunGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL is not set")
	}
	const addr = "127.0.0.1:18089"
	t.Setenv("HTTP_ADDR", addr)
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()

	url := "http://" + addr + "/api/v1/healthz"
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(url) //nolint:noctx // тестовый опрос
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("healthz status = %d", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}

	if _, err := http.Get(url); err == nil { //nolint:noctx // тестовый запрос
		t.Fatal("server still accepts connections after shutdown")
	}
}
