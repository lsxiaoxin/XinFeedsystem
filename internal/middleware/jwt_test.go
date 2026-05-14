package middleware

import (
	"context"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/internal/repository"
)

// ─── stubs ────────────────────────────────────────────────────────────────────

type stubDB struct {
	token  string  // token to return
	calls  int     // incremented on each FindTokenByUserID call
}

func (s *stubDB) FindTokenByUserID(_ context.Context, _ int64) (string, error) {
	s.calls++
	return s.token, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestTokenCache(t *testing.T) (*repository.TokenCache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return repository.NewTokenCache(rdb), mr
}

// makeClaims builds a Claims with the given userID and an ExpiresAt 1 hour ahead.
func makeClaims(userID int64) *pkgjwt.Claims {
	return &pkgjwt.Claims{
		UserID: userID,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

// ─── validateToken tests ──────────────────────────────────────────────────────

func TestValidateToken_CacheHit_ValidToken(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(1)
	const tok = "valid.token.here"

	_ = tc.Save(ctx, 1, tok, time.Hour)

	db := &stubDB{token: tok}
	if !validateToken(ctx, claims, tok, db, tc) {
		t.Fatal("expected valid token to be accepted")
	}
	// DB must NOT be called when Redis has the token.
	if db.calls != 0 {
		t.Fatalf("DB called %d times, want 0 (Redis fast path)", db.calls)
	}
}

func TestValidateToken_CacheHit_Mismatch_Rejected(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(2)

	// Redis has the old token (e.g. user logged in on another device).
	_ = tc.Save(ctx, 2, "old.token", time.Hour)

	db := &stubDB{token: "new.token"}
	// Request carries the new token, but Redis still has the old one → reject.
	if validateToken(ctx, claims, "new.token", db, tc) {
		t.Fatal("expected mismatched token to be rejected")
	}
	if db.calls != 0 {
		t.Fatal("DB should not be called when Redis has a definitive answer")
	}
}

func TestValidateToken_CacheMiss_DBFallback_Valid(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(3)
	const tok = "fresh.token"

	// Redis is empty (e.g. after a restart).
	db := &stubDB{token: tok}
	if !validateToken(ctx, claims, tok, db, tc) {
		t.Fatal("expected DB-fallback to accept valid token")
	}
	if db.calls != 1 {
		t.Fatalf("expected 1 DB call, got %d", db.calls)
	}
}

func TestValidateToken_CacheMiss_DBFallback_BackfillsRedis(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(4)
	const tok = "backfill.token"

	db := &stubDB{token: tok}
	validateToken(ctx, claims, tok, db, tc)

	// After DB fallback, Redis should now hold the token.
	cached, err := tc.Get(ctx, 4)
	if err != nil {
		t.Fatalf("Get after backfill: %v", err)
	}
	if cached != tok {
		t.Fatalf("Redis not back-filled: want %q, got %q", tok, cached)
	}
}

func TestValidateToken_CacheMiss_DBFallback_Invalid(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(5)

	// DB has a different token (user logged out then in again on another device).
	db := &stubDB{token: "current.token"}
	if validateToken(ctx, claims, "stale.token", db, tc) {
		t.Fatal("expected stale token to be rejected")
	}
}

func TestValidateToken_CacheMiss_NoDBRecord_Rejected(t *testing.T) {
	tc, _ := newTestTokenCache(t)
	ctx := context.Background()
	claims := makeClaims(6)

	// DB returns empty string (user never logged in or token was cleared).
	db := &stubDB{token: ""}
	if validateToken(ctx, claims, "any.token", db, tc) {
		t.Fatal("expected empty DB token to be rejected")
	}
}
