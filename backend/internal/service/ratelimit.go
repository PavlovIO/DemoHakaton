package service

import (
	"sync"
	"time"
)

// RateLimiter ограничивает частоту попыток по ключу.
type RateLimiter interface {
	// Check сообщает, можно ли выполнить попытку по ключу. Состояние не меняет.
	// Если лимит исчерпан, allowed=false и retryAfter > 0 — через сколько освободится слот.
	Check(key string, limit int, window time.Duration) (retryAfter time.Duration, allowed bool)
	// Hit записывает событие по ключу.
	Hit(key string, window time.Duration)
	// Reset удаляет все события ключа.
	Reset(key string)
}

type rateLimitBucket struct {
	events []time.Time
	window time.Duration
}

// MemoryRateLimiter хранит события в памяти процесса; состояние локально для
// процесса и не является общим при запуске нескольких инстансов.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets map[string]*rateLimitBucket
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
		now:     now,
		buckets: make(map[string]*rateLimitBucket),
	}
}

// Check проверяет количество событий в скользящем окне, не записывая новое.
func (l *MemoryRateLimiter) Check(key string, limit int, window time.Duration) (time.Duration, bool) {
	if limit <= 0 {
		return 0, true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	bucket, ok := l.buckets[key]
	if !ok {
		return 0, true
	}
	l.pruneBucket(key, bucket, now, window)
	if len(bucket.events) < limit {
		return 0, true
	}

	retryAfter := bucket.events[0].Add(window).Sub(now)
	if retryAfter < time.Millisecond {
		retryAfter = time.Millisecond
	}
	return retryAfter, false
}

// Hit записывает событие и удаляет устаревшие события этого ключа.
func (l *MemoryRateLimiter) Hit(key string, window time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	bucket, ok := l.buckets[key]
	if !ok {
		bucket = &rateLimitBucket{}
		l.buckets[key] = bucket
	}
	l.pruneEvents(bucket, now, window)
	bucket.events = append(bucket.events, now)
	bucket.window = window

	if len(l.buckets) > 10000 {
		for existingKey, existingBucket := range l.buckets {
			l.pruneBucket(existingKey, existingBucket, now, existingBucket.window)
		}
	}
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

func (l *MemoryRateLimiter) pruneBucket(key string, bucket *rateLimitBucket, now time.Time, window time.Duration) {
	l.pruneEvents(bucket, now, window)
	if len(bucket.events) == 0 {
		delete(l.buckets, key)
	}
}

func (l *MemoryRateLimiter) pruneEvents(bucket *rateLimitBucket, now time.Time, window time.Duration) {
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
