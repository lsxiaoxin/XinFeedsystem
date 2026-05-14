package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/repository"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/response"
)

const CtxUserID = "user_id"

// dbTokenReader is the DB-side dependency for token validation.
type dbTokenReader interface {
	FindTokenByUserID(ctx context.Context, userID int64) (string, error)
}

// cacheTokenStore is the Redis-side dependency for token validation.
type cacheTokenStore interface {
	Get(ctx context.Context, userID int64) (string, error)
	Save(ctx context.Context, userID int64, token string, ttl time.Duration) error
}

// JWTAuth enforces authentication: verifies the JWT signature, then validates
// the token against the active session (Redis → DB fallback).
func JWTAuth(userRepo *repository.UserRepository, tokenCache *repository.TokenCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr == "" {
			response.FailWithErr(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		claims, err := pkgjwt.Parse(tokenStr)
		if err != nil {
			code := errcode.TokenInvalid
			if err == pkgjwt.ErrExpiredToken {
				code = errcode.TokenExpired
			}
			response.FailWithErr(c, code)
			c.Abort()
			return
		}
		if !validateToken(c.Request.Context(), claims, tokenStr, userRepo, tokenCache) {
			response.FailWithErr(c, errcode.TokenInvalid)
			c.Abort()
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Next()
	}
}

// OptionalAuth attempts to authenticate; requests without a valid token are allowed through.
func OptionalAuth(userRepo *repository.UserRepository, tokenCache *repository.TokenCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr != "" {
			if claims, err := pkgjwt.Parse(tokenStr); err == nil {
				if validateToken(c.Request.Context(), claims, tokenStr, userRepo, tokenCache) {
					c.Set(CtxUserID, claims.UserID)
				}
			}
		}
		c.Next()
	}
}

// validateToken checks the token against Redis (fast path) then DB (fallback).
// On a DB hit it back-fills Redis so subsequent requests skip the DB query.
func validateToken(ctx context.Context, claims *pkgjwt.Claims, tokenStr string,
	db dbTokenReader, tc cacheTokenStore) bool {

	// Fast path: Redis hit.
	if cached, err := tc.Get(ctx, claims.UserID); err == nil && cached != "" {
		return cached == tokenStr
	}

	// Redis miss (or error) → fall back to DB.
	stored, err := db.FindTokenByUserID(ctx, claims.UserID)
	if err != nil || stored != tokenStr {
		return false
	}

	// Back-fill Redis: TTL = remaining JWT lifetime so it expires with the token.
	if remaining := time.Until(claims.ExpiresAt.Time); remaining > 0 {
		_ = tc.Save(ctx, claims.UserID, tokenStr, remaining)
	}
	return true
}

func extractToken(c *gin.Context) string {
	if token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
		return token
	}
	return c.Query("token")
}

func GetUserID(c *gin.Context) int64 {
	id, _ := c.Get(CtxUserID)
	uid, _ := id.(int64)
	return uid
}
