package service

import (
	"sync"
	"time"
)

// RateLimiter ограничивает частоту попыток по ключу.
type RateLimiter interface {
	// Allow атомарно проверяет лимит и, если он не исчерпан, записывает попытку.
	// Если лимит исчерпан, ok=false, попытка не записывается, retryAfter > 0 —
	// через сколько освободится слот.
	Allow(key string, limit int, window time.Duration) (retryAfter time.Duration, ok bool)
	// Reset удаляет все попытки ключа.
	Reset(key string)
}

// sweepInterval — как часто Allow удаляет ключи без попыток в окне. Чистка проходит
// по всем ключам, поэтому не чаще раза в интервал, а не на каждый вызов.
const sweepInterval = time.Minute

type rateLimitBucket struct {
	events []time.Time
	window time.Duration
}

// MemoryRateLimiter хранит события в памяти процесса; состояние локально для
// процесса и не является общим при запуске нескольких инстансов.
type MemoryRateLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	buckets   map[string]*rateLimitBucket
	lastSweep time.Time
}

var _ RateLimiter = (*MemoryRateLimiter)(nil)

// NewMemoryRateLimiter создаёт ограничитель с системными часами.
func NewMemoryRateLimiter() *MemoryRateLimiter {
	return NewMemoryRateLimiterWithClock(time.Now)
}

// NewMemoryRateLimiterWithClock создаёт ограничитель с заданными часами.
func NewMemoryRateLimiterWithClock(now func() time.Time) *MemoryRateLimiter {
	if now == nil {
		panic("service: nil rate limiter clock")
	}
	return &MemoryRateLimiter{
		now:       now,
		buckets:   make(map[string]*rateLimitBucket),
		lastSweep: now(),
	}
}

// Allow проверяет число событий в скользящем окне и записывает новое, если лимит не исчерпан.
// limit <= 0 — без ограничения (событие не записывается).
func (l *MemoryRateLimiter) Allow(key string, limit int, window time.Duration) (time.Duration, bool) {
	if limit <= 0 {
		return 0, true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= sweepInterval {
		l.sweep(now)
	}

	bucket, ok := l.buckets[key]
	if !ok {
		bucket = &rateLimitBucket{}
		l.buckets[key] = bucket
	}
	pruneEvents(bucket, now, window)
	bucket.window = window
	if len(bucket.events) >= limit {
		retryAfter := bucket.events[0].Add(window).Sub(now)
		if retryAfter < time.Millisecond {
			retryAfter = time.Millisecond
		}
		return retryAfter, false
	}
	bucket.events = append(bucket.events, now)
	return 0, true
}

// Reset удаляет события ключа.
func (l *MemoryRateLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

// Len возвращает число ключей в памяти; метод предназначен для тестов.
func (l *MemoryRateLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// sweep удаляет ключи, у которых не осталось событий в их окне. Вызывается под l.mu.
func (l *MemoryRateLimiter) sweep(now time.Time) {
	for key, bucket := range l.buckets {
		pruneEvents(bucket, now, bucket.window)
		if len(bucket.events) == 0 {
			delete(l.buckets, key)
		}
	}
	l.lastSweep = now
}

func pruneEvents(bucket *rateLimitBucket, now time.Time, window time.Duration) {
	cutoff := now.Add(-window)
	firstValid := 0
	for firstValid < len(bucket.events) && bucket.events[firstValid].Before(cutoff) {
		firstValid++
	}
	if firstValid > 0 {
		copy(bucket.events, bucket.events[firstValid:])
		bucket.events = bucket.events[:len(bucket.events)-firstValid]
	}
}
