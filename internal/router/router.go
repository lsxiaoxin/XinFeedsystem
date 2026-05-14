package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/api"
	"xinfeedsystem/internal/middleware"
)

// Handlers 聚合所有 handler，随业务增长追加字段即可。
type Handlers struct {
	User *api.UserHandler
}

func New(h *Handlers) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(gin.Logger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// 公开路由（无需鉴权）
	public := v1.Group("")
	{
		public.POST("/user/register", h.User.Register)
		public.POST("/user/login", h.User.Login)
		public.GET("/user/:id", h.User.GetUserInfo)
	}

	// 需要鉴权的路由
	auth := v1.Group("", middleware.JWTAuth())
	{
		auth.GET("/user/me", h.User.GetMe)
	}

	return r
}
