package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/response"
)

const CtxUserID = "user_id"

// JWTAuth 强制鉴权，token 无效直接 abort。
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.FailWithErr(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		claims, err := pkgjwt.Parse(token)
		if err != nil {
			code := errcode.TokenInvalid
			if err == pkgjwt.ErrExpiredToken {
				code = errcode.TokenExpired
			}
			response.FailWithErr(c, code)
			c.Abort()
			return
		}
		c.Set(CtxUserID, claims.UserID)
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
