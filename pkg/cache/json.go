package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

const nilSentinel = "__nil__"

// GetJSON 从 Redis 读取 JSON，反序列化到 v。
// hit=true 且 isNil=true 表示 DB 曾查过是空（防穿透占位）。
// err != nil 时应降级到 DB，不应阻断请求。
func GetJSON(ctx context.Context, rdb *redis.Client, key string, v interface{}) (hit bool, isNil bool, err error) {
	val, err := rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if val == nilSentinel {
		return true, true, nil
	}
	if err = json.Unmarshal([]byte(val), v); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// SetJSON 将 v 序列化为 JSON 写入 Redis，TTL 由调用方传入（建议使用 RandomizedTTL 防雪崩）。
func SetJSON(ctx context.Context, rdb *redis.Client, key string, v interface{}, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	return rdb.Set(ctx, key, b, ttl).Err()
}

// SetNil 写入空占位，防止缓存穿透（DB 查无此记录时调用）。
func SetNil(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) error {
	return rdb.Set(ctx, key, nilSentinel, ttl).Err()
}

// RandomizedTTL 在 base ± jitter 范围内随机选取 TTL，防止大批 key 同时过期（缓存雪崩）。
func RandomizedTTL(base, jitter time.Duration) time.Duration {
	delta := time.Duration(rand.Int63n(int64(jitter)*2+1)) - jitter
	return base + delta
}
