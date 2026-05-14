package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/middleware"
)

func New() *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(gin.Logger())

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// 公开路由（无需鉴权）
	public := v1.Group("")
	{
		_ = public // 用户注册/登录 handler 在 phase-1 业务实现时注册
	}

	// 需要鉴权的路由
	auth := v1.Group("", middleware.JWTAuth())
	{
		_ = auth // 视频/Feed/点赞/评论/关注 handler 在 phase-1 业务实现时注册
	}

	return r
}
