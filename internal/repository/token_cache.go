package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenCache stores the single active token per user in Redis.
// Key: user:token:{userID} → token string, TTL = JWT expire.
type TokenCache struct {
	rdb *redis.Client
}

func NewTokenCache(rdb *redis.Client) *TokenCache {
	return &TokenCache{rdb: rdb}
}

func tokenKey(userID int64) string {
	return fmt.Sprintf("user:token:%d", userID)
}

// Save writes the token to Redis with the given TTL.
func (c *TokenCache) Save(ctx context.Context, userID int64, token string, ttl time.Duration) error {
	return c.rdb.Set(ctx, tokenKey(userID), token, ttl).Err()
}

// Get returns the stored token, or ("", nil) on a cache miss.
func (c *TokenCache) Get(ctx context.Context, userID int64) (string, error) {
	val, err := c.rdb.Get(ctx, tokenKey(userID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Delete removes the token key (used on logout).
func (c *TokenCache) Delete(ctx context.Context, userID int64) error {
	return c.rdb.Del(ctx, tokenKey(userID)).Err()
}
