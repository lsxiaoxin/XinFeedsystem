package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/repository"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/response"
)

const CtxUserID = "user_id"

// JWTAuth 强制鉴权：验签 + 比对 DB 中存储的 token，不一致视为已登出或被顶替。
func JWTAuth(userRepo *repository.UserRepository) gin.HandlerFunc {
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
		stored, err := userRepo.FindTokenByUserID(c.Request.Context(), claims.UserID)
		if err != nil || stored != tokenStr {
			response.FailWithErr(c, errcode.TokenInvalid)
			c.Abort()
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Next()
	}
}

// OptionalAuth 尝试解析 token 并比对 DB；token 缺失或无效时直接放行。
func OptionalAuth(userRepo *repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr != "" {
			if claims, err := pkgjwt.Parse(tokenStr); err == nil {
				if stored, err := userRepo.FindTokenByUserID(c.Request.Context(), claims.UserID); err == nil && stored == tokenStr {
					c.Set(CtxUserID, claims.UserID)
				}
			}
		}
		c.Next()
	}
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
