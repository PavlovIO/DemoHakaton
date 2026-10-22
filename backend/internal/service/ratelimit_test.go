package service_test

import (
	"sync"
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
		if retryAfter, allowed := limiter.Check("login", 3, window); !allowed || retryAfter != 0 {
			t.Fatalf("Check before hit = (%s, %v), want (0, true)", retryAfter, allowed)
		}
		limiter.Hit("login", window)
	}

	now = start.Add(3 * time.Minute)
	if retryAfter, allowed := limiter.Check("login", 3, window); allowed || retryAfter != 12*time.Minute {
		t.Fatalf("Check at limit = (%s, %v), want (12m, false)", retryAfter, allowed)
	}

	now = start.Add(15 * time.Minute)
	if retryAfter, allowed := limiter.Check("login", 3, window); allowed || retryAfter != time.Millisecond {
		t.Fatalf("Check at expiration boundary = (%s, %v), want (1ms, false)", retryAfter, allowed)
	}
	now = start.Add(15*time.Minute + time.Millisecond)
	if retryAfter, allowed := limiter.Check("login", 3, window); !allowed || retryAfter != 0 {
		t.Fatalf("Check after expiration = (%s, %v), want (0, true)", retryAfter, allowed)
	}
}

func TestMemoryRateLimiterResetAndIndependentKeys(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	window := time.Minute
	limiter.Hit("first", window)
	limiter.Hit("second", window)

	if _, allowed := limiter.Check("first", 1, window); allowed {
		t.Fatal("first key allowed after reaching its limit")
	}
	if _, allowed := limiter.Check("second", 2, window); !allowed {
		t.Fatal("second key was affected by the first key")
	}

	limiter.Reset("first")
	if _, allowed := limiter.Check("first", 1, window); !allowed {
		t.Fatal("first key remains limited after Reset")
	}
	if _, allowed := limiter.Check("second", 1, window); allowed {
		t.Fatal("Reset of first key affected second key")
	}
}

func TestMemoryRateLimiterDisabledLimit(t *testing.T) {
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time {
		return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	})
	for _, limit := range []int{0, -1} {
		if retryAfter, allowed := limiter.Check("key", limit, time.Minute); !allowed || retryAfter != 0 {
			t.Errorf("Check(limit=%d) = (%s, %v), want (0, true)", limit, retryAfter, allowed)
		}
	}
}

func TestMemoryRateLimiterConcurrentHits(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	const hits = 100
	var wg sync.WaitGroup
	wg.Add(hits)
	for range hits {
		go func() {
			defer wg.Done()
			limiter.Hit("shared", time.Hour)
		}()
	}
	wg.Wait()

	if _, allowed := limiter.Check("shared", hits, time.Hour); allowed {
		t.Fatal("limit 100 allowed after 100 concurrent hits")
	}
	if _, allowed := limiter.Check("shared", hits+1, time.Hour); !allowed {
		t.Fatal("limit 101 denied after 100 concurrent hits")
	}
}

func TestMemoryRateLimiterCleansExpiredKeys(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
	window := time.Minute
	for i := 0; i <= 10000; i++ {
		limiter.Hit(string(rune(i+1)), window)
	}
	if got := limiter.Len(); got != 10001 {
		t.Fatalf("Len() before expiration = %d, want 10001", got)
	}

	now = now.Add(window + time.Millisecond)
	limiter.Hit("fresh", window)
	if got := limiter.Len(); got != 1 {
		t.Fatalf("Len() after expired-key cleanup = %d, want 1", got)
	}
}
