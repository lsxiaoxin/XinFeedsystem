package redislock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"xinfeedsystem/pkg/redislock"
)

func newTestClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

func TestTryLock_AcquireAndRelease(t *testing.T) {
	rdb, _ := newTestClient(t)
	l := redislock.New(rdb)
	ctx := context.Background()

	token, err := l.TryLock(ctx, "mylock", 5*time.Second)
	if err != nil {
		t.Fatalf("expected lock acquired, got %v", err)
	}

	// Second TryLock on the same key must fail.
	_, err2 := l.TryLock(ctx, "mylock", 5*time.Second)
	if !errors.Is(err2, redislock.ErrNotObtained) {
		t.Fatalf("expected ErrNotObtained, got %v", err2)
	}

	// Release, then a third caller should succeed.
	if err := l.Unlock(ctx, "mylock", token); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	_, err3 := l.TryLock(ctx, "mylock", 5*time.Second)
	if err3 != nil {
		t.Fatalf("expected re-acquire after unlock, got %v", err3)
	}
}

func TestUnlock_WrongTokenIsNoop(t *testing.T) {
	rdb, _ := newTestClient(t)
	l := redislock.New(rdb)
	ctx := context.Background()

	_, err := l.TryLock(ctx, "mylock", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Unlock with a wrong token must not delete the key.
	_ = l.Unlock(ctx, "mylock", "wrong-token")

	_, err2 := l.TryLock(ctx, "mylock", 5*time.Second)
	if !errors.Is(err2, redislock.ErrNotObtained) {
		t.Fatal("lock should still be held after wrong-token unlock")
	}
}

func TestDo_LoaderCalledOnceOnCacheMiss(t *testing.T) {
	rdb, _ := newTestClient(t)
	l := redislock.New(rdb)
	ctx := context.Background()

	calls := 0
	val, err := l.Do(
		ctx,
		"lock:do-test",
		5*time.Second,
		200*time.Millisecond,
		func() (interface{}, error) {
			calls++
			return "db-result", nil
		},
		func() (interface{}, bool, error) {
			return nil, false, nil // simulate cache never populated (only matters for waiters)
		},
	)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if val != "db-result" {
		t.Fatalf("expected db-result, got %v", val)
	}
	if calls != 1 {
		t.Fatalf("loader called %d times, want 1", calls)
	}
}
