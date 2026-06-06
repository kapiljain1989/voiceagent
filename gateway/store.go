package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// -------------------------------------------------------------------
// Distributed Session Store — Redis-backed state for horizontal scaling
//
// Replaces in-memory maps with Redis. Gateway replicas become stateless.
// Any replica can serve any request. SSE events routed via Redis pub/sub.
//
// When REDIS_URL is not set, falls back to local in-memory store.
// -------------------------------------------------------------------

type SessionStore interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Get(ctx context.Context, key string, dest any) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) bool
	Publish(ctx context.Context, channel string, message any) error
	Subscribe(ctx context.Context, channel string) (<-chan []byte, func())
	Incr(ctx context.Context, key string) int64
	Decr(ctx context.Context, key string) int64
}

// -------------------------------------------------------------------
// Redis Store — production-grade distributed store
// -------------------------------------------------------------------

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(url string) (*RedisStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis parse url: %w", err)
	}
	opts.PoolSize = 50
	opts.MinIdleConns = 10
	opts.MaxRetries = 3
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	slog.Info("redis connected", "addr", opts.Addr)
	return &RedisStore{client: client}, nil
}

func (r *RedisStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisStore) Get(ctx context.Context, key string, dest any) error {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStore) Exists(ctx context.Context, key string) bool {
	n, _ := r.client.Exists(ctx, key).Result()
	return n > 0
}

func (r *RedisStore) Publish(ctx context.Context, channel string, message any) error {
	data, _ := json.Marshal(message)
	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisStore) Subscribe(ctx context.Context, channel string) (<-chan []byte, func()) {
	sub := r.client.Subscribe(ctx, channel)
	ch := make(chan []byte, 50)

	go func() {
		defer close(ch)
		for msg := range sub.Channel() {
			select {
			case ch <- []byte(msg.Payload):
			default:
			}
		}
	}()

	cleanup := func() { sub.Close() }
	return ch, cleanup
}

func (r *RedisStore) Incr(ctx context.Context, key string) int64 {
	v, _ := r.client.Incr(ctx, key).Result()
	return v
}

func (r *RedisStore) Decr(ctx context.Context, key string) int64 {
	v, _ := r.client.Decr(ctx, key).Result()
	return v
}

func (r *RedisStore) Close() { r.client.Close() }

// -------------------------------------------------------------------
// Local In-Memory Store — fallback when Redis is not configured
// -------------------------------------------------------------------

type LocalStore struct {
	data    map[string][]byte
	mu      sync.RWMutex
	pubsub  map[string][]chan []byte
	pubmu   sync.RWMutex
	counters map[string]int64
	cmu     sync.Mutex
}

func NewLocalStore() *LocalStore {
	return &LocalStore{
		data:     make(map[string][]byte),
		pubsub:   make(map[string][]chan []byte),
		counters: make(map[string]int64),
	}
}

func (l *LocalStore) Set(_ context.Context, key string, value any, _ time.Duration) error {
	data, _ := json.Marshal(value)
	l.mu.Lock()
	l.data[key] = data
	l.mu.Unlock()
	return nil
}

func (l *LocalStore) Get(_ context.Context, key string, dest any) error {
	l.mu.RLock()
	data, ok := l.data[key]
	l.mu.RUnlock()
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}
	return json.Unmarshal(data, dest)
}

func (l *LocalStore) Delete(_ context.Context, key string) error {
	l.mu.Lock()
	delete(l.data, key)
	l.mu.Unlock()
	return nil
}

func (l *LocalStore) Exists(_ context.Context, key string) bool {
	l.mu.RLock()
	_, ok := l.data[key]
	l.mu.RUnlock()
	return ok
}

func (l *LocalStore) Publish(_ context.Context, channel string, message any) error {
	data, _ := json.Marshal(message)
	l.pubmu.RLock()
	subs := l.pubsub[channel]
	l.pubmu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- data:
		default:
		}
	}
	return nil
}

func (l *LocalStore) Subscribe(_ context.Context, channel string) (<-chan []byte, func()) {
	ch := make(chan []byte, 50)
	l.pubmu.Lock()
	l.pubsub[channel] = append(l.pubsub[channel], ch)
	l.pubmu.Unlock()

	cleanup := func() {
		l.pubmu.Lock()
		subs := l.pubsub[channel]
		for i, s := range subs {
			if s == ch {
				l.pubsub[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		l.pubmu.Unlock()
		close(ch)
	}
	return ch, cleanup
}

func (l *LocalStore) Incr(_ context.Context, key string) int64 {
	l.cmu.Lock()
	l.counters[key]++
	v := l.counters[key]
	l.cmu.Unlock()
	return v
}

func (l *LocalStore) Decr(_ context.Context, key string) int64 {
	l.cmu.Lock()
	l.counters[key]--
	v := l.counters[key]
	l.cmu.Unlock()
	return v
}

// -------------------------------------------------------------------
// Factory — creates the appropriate store based on config
// -------------------------------------------------------------------

func NewSessionStore(redisURL string) SessionStore {
	if redisURL == "" {
		slog.Info("using local in-memory session store")
		return NewLocalStore()
	}

	store, err := NewRedisStore(redisURL)
	if err != nil {
		slog.Warn("redis unavailable, falling back to local store", "err", err)
		return NewLocalStore()
	}
	return store
}
