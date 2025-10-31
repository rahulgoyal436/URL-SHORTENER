package handler

import (
	"sync"
	"time"
)

// Simple in-memory token bucket per key (IP).
// Not perfect for multi-instance — Redis backed recommended for production.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

type SimpleRateLimiter struct {
	buckets map[string]*tokenBucket
	mu      sync.Mutex
	rate    float64
	burst   float64
}

func NewSimpleRateLimiter() *SimpleRateLimiter {
	return &SimpleRateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    1.0, // tokens per second
		burst:   10,  // burst capacity
	}
}

func (s *SimpleRateLimiter) Allow(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buckets[key]
	now := time.Now()
	if !ok {
		s.buckets[key] = &tokenBucket{tokens: s.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * s.rate
	if b.tokens > s.burst {
		b.tokens = s.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}
