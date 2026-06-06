package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// -------------------------------------------------------------------
// Worker Pool — distributes STT/TTS requests across multiple backends
//
// Instead of a single Whisper or Piper container, the gateway can
// round-robin requests across a pool of workers for horizontal scaling.
// -------------------------------------------------------------------

type WorkerPool struct {
	name    string
	urls    []string
	index   atomic.Int64
	healthy []atomic.Bool
	mu      sync.RWMutex
}

func NewWorkerPool(name string, urls []string) *WorkerPool {
	wp := &WorkerPool{
		name:    name,
		urls:    urls,
		healthy: make([]atomic.Bool, len(urls)),
	}
	for i := range wp.healthy {
		wp.healthy[i].Store(true)
	}
	return wp
}

// Next returns the next healthy worker URL using round-robin.
func (wp *WorkerPool) Next() (string, error) {
	n := len(wp.urls)
	if n == 0 {
		return "", fmt.Errorf("no %s workers configured", wp.name)
	}

	// Round-robin with skip for unhealthy workers
	start := int(wp.index.Add(1)) % n
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if wp.healthy[idx].Load() {
			return wp.urls[idx], nil
		}
	}

	// All unhealthy — return random one and hope
	return wp.urls[rand.Intn(n)], nil
}

// MarkUnhealthy marks a worker as unhealthy. Auto-recovers after cooldown.
func (wp *WorkerPool) MarkUnhealthy(url string, cooldown time.Duration) {
	for i, u := range wp.urls {
		if u == url {
			wp.healthy[i].Store(false)
			slog.Warn("worker marked unhealthy", "pool", wp.name, "url", url)
			go func(idx int) {
				time.Sleep(cooldown)
				wp.healthy[idx].Store(true)
				slog.Info("worker recovered", "pool", wp.name, "url", wp.urls[idx])
			}(i)
			return
		}
	}
}

// HealthyCount returns the number of healthy workers.
func (wp *WorkerPool) HealthyCount() int {
	count := 0
	for i := range wp.healthy {
		if wp.healthy[i].Load() {
			count++
		}
	}
	return count
}

// Status returns pool health summary.
func (wp *WorkerPool) Status() map[string]any {
	workers := make([]map[string]any, len(wp.urls))
	for i, url := range wp.urls {
		workers[i] = map[string]any{
			"url":     url,
			"healthy": wp.healthy[i].Load(),
		}
	}
	return map[string]any{
		"name":    wp.name,
		"total":   len(wp.urls),
		"healthy": wp.HealthyCount(),
		"workers": workers,
	}
}

// StartHealthCheck periodically pings all workers.
func (wp *WorkerPool) StartHealthCheck(ctx context.Context, interval time.Duration, healthPath string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		client := &http.Client{Timeout: 5 * time.Second}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for i, url := range wp.urls {
					resp, err := client.Get(url + healthPath)
					if err != nil || resp.StatusCode >= 500 {
						wp.healthy[i].Store(false)
					} else {
						wp.healthy[i].Store(true)
						resp.Body.Close()
					}
				}
			}
		}
	}()
}

// -------------------------------------------------------------------
// Token Bucket Rate Limiter
//
// Limits requests per caller, per agent, or per API key.
// Prevents overload during traffic spikes. Returns SIP 503 when full.
// -------------------------------------------------------------------

type RateLimiter struct {
	buckets sync.Map // key → *tokenBucket
	rate    float64  // tokens per second
	burst   int      // max tokens
}

type tokenBucket struct {
	tokens    float64
	lastFill  time.Time
	rate      float64
	burst     int
	mu        sync.Mutex
}

func NewRateLimiter(ratePerSecond float64, burst int) *RateLimiter {
	return &RateLimiter{rate: ratePerSecond, burst: burst}
}

// Allow checks if a request is allowed for the given key.
func (rl *RateLimiter) Allow(key string) bool {
	val, _ := rl.buckets.LoadOrStore(key, &tokenBucket{
		tokens:   float64(rl.burst),
		lastFill: time.Now(),
		rate:     rl.rate,
		burst:    rl.burst,
	})
	bucket := val.(*tokenBucket)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bucket.lastFill).Seconds()
	bucket.tokens += elapsed * bucket.rate
	if bucket.tokens > float64(bucket.burst) {
		bucket.tokens = float64(bucket.burst)
	}
	bucket.lastFill = now

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

// Remaining returns tokens remaining for a key.
func (rl *RateLimiter) Remaining(key string) int {
	val, ok := rl.buckets.Load(key)
	if !ok {
		return rl.burst
	}
	bucket := val.(*tokenBucket)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	return int(bucket.tokens)
}

// Middleware wraps an HTTP handler with rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			key = fwd
		}

		if !rl.Allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// -------------------------------------------------------------------
// Admission Controller — limits concurrent call sessions
// -------------------------------------------------------------------

type AdmissionController struct {
	maxSessions int64
	current     atomic.Int64
}

func NewAdmissionController(maxSessions int64) *AdmissionController {
	return &AdmissionController{maxSessions: maxSessions}
}

// Admit tries to admit a new session. Returns false if at capacity.
func (ac *AdmissionController) Admit() bool {
	for {
		current := ac.current.Load()
		if current >= ac.maxSessions {
			return false
		}
		if ac.current.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// Release releases a session slot.
func (ac *AdmissionController) Release() {
	ac.current.Add(-1)
}

// Status returns admission state.
func (ac *AdmissionController) Status() map[string]any {
	return map[string]any{
		"max_sessions": ac.maxSessions,
		"current":      ac.current.Load(),
		"available":    ac.maxSessions - ac.current.Load(),
	}
}
