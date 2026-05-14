package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"xinfeedsystem/pkg/logger"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered", zap.Any("err", err), zap.String("path", c.Request.URL.Path))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code": 50000,
					"msg":  "服务器内部错误",
					"data": nil,
				})
			}
		}()
		c.Next()
	}
}
