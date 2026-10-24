package service_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	service "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func TestMemoryRateLimiterSlidingWindow(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	now := start
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	window := 15 * time.Minute

	for _, elapsed := range []time.Duration{0, time.Minute, 2 * time.Minute} {
		now = start.Add(elapsed)
		if retryAfter, ok := limiter.Allow("login", 3, window); !ok || retryAfter != 0 {
			t.Fatalf("Allow under limit = (%s, %v), want (0, true)", retryAfter, ok)
		}
	}

	now = start.Add(3 * time.Minute)
	if retryAfter, ok := limiter.Allow("login", 3, window); ok || retryAfter != 12*time.Minute {
		t.Fatalf("Allow at limit = (%s, %v), want (12m, false)", retryAfter, ok)
	}

	now = start.Add(15 * time.Minute)
	if retryAfter, ok := limiter.Allow("login", 3, window); ok || retryAfter != time.Millisecond {
		t.Fatalf("Allow at expiration boundary = (%s, %v), want (1ms, false)", retryAfter, ok)
	}
	now = start.Add(15*time.Minute + time.Millisecond)
	if retryAfter, ok := limiter.Allow("login", 3, window); !ok || retryAfter != 0 {
		t.Fatalf("Allow after expiration = (%s, %v), want (0, true)", retryAfter, ok)
	}
}

func TestMemoryRateLimiterDeniedAttemptIsNotRecorded(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	now := start
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })

	limiter.Allow("key", 1, time.Minute)
	for range 5 {
		now = now.Add(10 * time.Second)
		if _, ok := limiter.Allow("key", 1, time.Minute); ok {
			t.Fatal("Allow over limit returned ok")
		}
	}
	// Отказы не продлевают блокировку: слот освобождается через окно от первой попытки.
	now = start.Add(time.Minute + time.Millisecond)
	if _, ok := limiter.Allow("key", 1, time.Minute); !ok {
		t.Fatal("denied attempts extended the window")
	}
}

func TestMemoryRateLimiterResetAndIndependentKeys(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	window := time.Minute
	limiter.Allow("first", 1, window)
	limiter.Allow("second", 2, window)

	if _, ok := limiter.Allow("first", 1, window); ok {
		t.Fatal("first key allowed after reaching its limit")
	}
	if _, ok := limiter.Allow("second", 2, window); !ok {
		t.Fatal("second key was affected by the first key")
	}

	limiter.Reset("first")
	if _, ok := limiter.Allow("first", 1, window); !ok {
		t.Fatal("first key remains limited after Reset")
	}
	if _, ok := limiter.Allow("second", 2, window); ok {
		t.Fatal("Reset of first key affected second key")
	}
}

func TestMemoryRateLimiterDisabledLimit(t *testing.T) {
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time {
		return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	})
	for _, limit := range []int{0, -1} {
		if retryAfter, ok := limiter.Allow("key", limit, time.Minute); !ok || retryAfter != 0 {
			t.Errorf("Allow(limit=%d) = (%s, %v), want (0, true)", limit, retryAfter, ok)
		}
	}
	if got := limiter.Len(); got != 0 {
		t.Fatalf("Len() after disabled-limit calls = %d, want 0", got)
	}
}

// Проверка и запись атомарны: из 100 одновременных попыток с limit=10 проходят ровно 10.
func TestMemoryRateLimiterConcurrentAllow(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	const attempts, limit = 100, 10
	var allowed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if _, ok := limiter.Allow("shared", limit, time.Hour); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != limit {
		t.Fatalf("allowed = %d, want %d", got, limit)
	}
}

func TestMemoryRateLimiterSweepsExpiredKeys(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	window := 30 * time.Second
	for i := range 100 {
		limiter.Allow(string(rune(i+1)), 1, window)
	}
	if got := limiter.Len(); got != 100 {
		t.Fatalf("Len() before expiration = %d, want 100", got)
	}

	// Окна истекли, но интервал чистки ещё нет — ключи лежат.
	now = now.Add(window + time.Second)
	limiter.Allow("fresh", 1, window)
	if got := limiter.Len(); got != 101 {
		t.Fatalf("Len() before sweep interval = %d, want 101", got)
	}

	now = now.Add(time.Minute)
	limiter.Allow("fresh2", 1, window)
	if got := limiter.Len(); got != 1 {
		t.Fatalf("Len() after sweep = %d, want 1", got)
	}
}
