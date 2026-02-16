// Package redis provides a Redis client for caching, user memory,
// agent prompts, and session state. Mirrors survival/nanobot/redis_client.py.
//
// Graceful fallback: if Redis is unavailable, operations silently return
// zero values instead of blocking the business logic.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Key prefixes — keep in sync with Backend (redis.ts REDIS_KEYS).
const (
	KeyMemory      = "mem:"     // User memory cache
	KeyAgentPrompt = "ap:"      // Agent persona cache
	KeySession     = "session:" // Session state
	KeyLock        = "lock:"    // Distributed lock
	KeyCache       = "cache:"   // General cache
)

// Config holds Redis connection settings.
type Config struct {
	URL      string // redis://host:port
	Password string
	DB       int
}

var (
	client    *redis.Client
	connected bool
	mu        sync.RWMutex
)

// Init initializes the Redis connection. Returns true if connected.
func Init(cfg Config) bool {
	if cfg.URL == "" {
		log.Println("[Redis] URL not configured, skipping init")
		return false
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		log.Printf("[Redis] ❌ Invalid URL: %v", err)
		return false
	}

	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	opts.DB = cfg.DB
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second
	opts.MaxRetries = 3

	c := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx).Err(); err != nil {
		log.Printf("[Redis] ❌ Connection failed: %v", err)
		return false
	}

	mu.Lock()
	client = c
	connected = true
	mu.Unlock()

	log.Println("[Redis] ✅ Connected")
	return true
}

// Close closes the Redis connection.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		client.Close()
		client = nil
		connected = false
		log.Println("[Redis] Connection closed")
	}
}

// Client returns the Redis client. Returns nil if not available.
func Client() *redis.Client {
	mu.RLock()
	defer mu.RUnlock()
	if connected {
		return client
	}
	return nil
}

// IsAvailable checks if Redis is connected.
func IsAvailable() bool {
	mu.RLock()
	defer mu.RUnlock()
	return connected && client != nil
}

// --- Cache operations (with graceful fallback) ---

// CacheGet reads a string value. Returns "" if unavailable.
func CacheGet(ctx context.Context, key string) string {
	c := Client()
	if c == nil {
		return ""
	}
	val, err := c.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Printf("[Redis] cache_get failed (%s): %v", key, err)
		}
		return ""
	}
	return val
}

// CacheSet writes a string value with TTL. Returns false on failure.
func CacheSet(ctx context.Context, key, value string, ttl time.Duration) bool {
	c := Client()
	if c == nil {
		return false
	}
	if err := c.Set(ctx, key, value, ttl).Err(); err != nil {
		log.Printf("[Redis] cache_set failed (%s): %v", key, err)
		return false
	}
	return true
}

// CacheDel deletes a key. Returns false on failure.
func CacheDel(ctx context.Context, key string) bool {
	c := Client()
	if c == nil {
		return false
	}
	if err := c.Del(ctx, key).Err(); err != nil {
		log.Printf("[Redis] cache_del failed (%s): %v", key, err)
		return false
	}
	return true
}

// CacheGetJSON reads a JSON value into out. Returns false if not found/error.
func CacheGetJSON(ctx context.Context, key string, out any) bool {
	raw := CacheGet(ctx, key)
	if raw == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		log.Printf("[Redis] cache_get_json parse failed (%s): %v", key, err)
		return false
	}
	return true
}

// CacheSetJSON writes a JSON-serialized value with TTL.
func CacheSetJSON(ctx context.Context, key string, value any, ttl time.Duration) bool {
	data, err := json.Marshal(value)
	if err != nil {
		log.Printf("[Redis] cache_set_json marshal failed (%s): %v", key, err)
		return false
	}
	return CacheSet(ctx, key, string(data), ttl)
}

// MemoryKey returns the Redis key for a user's memory.
func MemoryKey(personID string) string {
	return fmt.Sprintf("%s%s", KeyMemory, personID)
}

// AgentPromptKey returns the Redis key for an agent's prompt.
func AgentPromptKey(agentID string) string {
	return fmt.Sprintf("%s%s", KeyAgentPrompt, agentID)
}
