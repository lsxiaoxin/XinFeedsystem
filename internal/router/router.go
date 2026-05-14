package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/api"
	"xinfeedsystem/internal/middleware"
)

// Handlers 聚合所有 handler，随业务增长追加字段即可。
type Handlers struct {
	User    *api.UserHandler
	Video   *api.VideoHandler
	Feed    *api.FeedHandler
	Like    *api.LikeHandler
	Comment *api.CommentHandler
	Follow  *api.FollowHandler
}

func New(h *Handlers, staticDir string) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(gin.Logger())

	// 静态文件服务：视频/封面直链播放
	r.Static("/static/videos", staticDir+"/videos")
	r.Static("/static/covers", staticDir+"/covers")

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// ── 公开路由 ──────────────────────────────────
	public := v1.Group("")
	{
		// 用户
		public.POST("/user/register", h.User.Register)
		public.POST("/user/login", h.User.Login)
		public.GET("/user/:id", h.User.GetUserInfo)

		// 视频（公开可见）
		public.GET("/video/:id", h.Video.GetDetail)
		public.GET("/video/list", h.Video.ListByAuthorID)

		// Feed（公开可见，following 类型需要 token；OptionalAuth 软解析不阻断）
		public.GET("/feed", middleware.OptionalAuth(), h.Feed.GetFeed)

		// 评论列表（公开可见）
		public.GET("/comment/list", h.Comment.List)
	}

	// ── 需要鉴权 ──────────────────────────────────
	auth := v1.Group("", middleware.JWTAuth())
	{
		// 用户
		auth.GET("/user/me", h.User.GetMe)

		// 视频
		auth.POST("/video/publish", h.Video.Publish)

		// 点赞
		auth.POST("/like/action", h.Like.Action)
		auth.GET("/like/list", h.Like.List)

		// 评论（发/删需鉴权，列表公开）
		auth.POST("/comment/action", h.Comment.Action)

		// 关注（关注/取关需鉴权，列表公开）
		auth.POST("/follow/action", h.Follow.Action)
	}

	// ── 关注列表（公开）───────────────────────────
	v1.GET("/follow/following", h.Follow.Following)
	v1.GET("/follow/follower", h.Follow.Follower)

	return r
}
