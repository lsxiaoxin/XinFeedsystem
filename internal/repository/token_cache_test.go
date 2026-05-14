package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/repository"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func TestTokenCache_SaveGetDelete(t *testing.T) {
	rdb := newTestRedis(t)
	c := repository.NewTokenCache(rdb)
	ctx := context.Background()

	const (
		userID = int64(42)
		token  = "tok.abc.xyz"
	)

	// Save then Get.
	if err := c.Save(ctx, userID, token, time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := c.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != token {
		t.Fatalf("Get: want %q, got %q", token, got)
	}

	// Delete then Get should return empty.
	if err := c.Delete(ctx, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = c.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty after Delete, got %q", got)
	}
}

func TestTokenCache_Get_Miss_ReturnsEmpty(t *testing.T) {
	rdb := newTestRedis(t)
	c := repository.NewTokenCache(rdb)
	ctx := context.Background()

	got, err := c.Get(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty on miss, got %q", got)
	}
}

func TestTokenCache_TTL_Expires(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	c := repository.NewTokenCache(rdb)
	ctx := context.Background()

	_ = c.Save(ctx, 1, "mytoken", 100*time.Millisecond)

	// Fast-forward miniredis clock past TTL.
	mr.FastForward(200 * time.Millisecond)

	got, err := c.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get after expiry: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty after TTL expiry, got %q", got)
	}
}
