package redislock

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrNotObtained is returned when TryLock fails to acquire the lock.
var ErrNotObtained = errors.New("redislock: lock not obtained")

// unlockScript atomically deletes the key only if its value matches the token.
// This prevents a slow holder from accidentally releasing a lock acquired by a later caller.
var unlockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`)

type Lock struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Lock {
	return &Lock{rdb: rdb}
}

// TryLock attempts a single SETNX. Returns (token, nil) on success,
// ("", ErrNotObtained) if the key already exists, or ("", err) on Redis error.
func (l *Lock) TryLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token := uuid.NewString()
	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotObtained
	}
	return token, nil
}

// Unlock releases the lock only when the stored token matches, preventing
// a holder from releasing a lock it no longer owns after TTL expiry.
func (l *Lock) Unlock(ctx context.Context, key, token string) error {
	return unlockScript.Run(ctx, l.rdb, []string{key}, token).Err()
}

// Do is a Cache-Aside helper that serializes cache misses through the lock.
//
//   - The first caller to arrive on a miss wins the lock and runs loader(),
//     which should read from DB and write the result back to the cache.
//   - Concurrent callers that lose the lock spin-poll waiter() (typically a
//     Redis GET) until it returns a hit or maxWait is exceeded.
//   - If loader itself returns an error the lock is released immediately.
func (l *Lock) Do(
	ctx context.Context,
	lockKey string,
	ttl time.Duration,
	maxWait time.Duration,
	loader func() (interface{}, error),
	waiter func() (interface{}, bool, error),
) (interface{}, error) {
	token, err := l.TryLock(ctx, lockKey, ttl)
	if err == nil {
		// This goroutine holds the lock — load from DB and write to cache.
		defer l.Unlock(ctx, lockKey, token) //nolint:errcheck
		return loader()
	}
	if !errors.Is(err, ErrNotObtained) {
		return nil, err
	}

	// Another goroutine is loading. Poll until cache is populated or we time out.
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		val, hit, werr := waiter()
		if werr != nil {
			return nil, werr
		}
		if hit {
			return val, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil, errors.New("redislock: timed out waiting for cache population")
}
