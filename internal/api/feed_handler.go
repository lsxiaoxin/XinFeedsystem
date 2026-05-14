package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/middleware"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/response"
)

type FeedHandler struct {
	feedSvc *service.FeedService
}

func NewFeedHandler(feedSvc *service.FeedService) *FeedHandler {
	return &FeedHandler{feedSvc: feedSvc}
}

// GetFeed godoc
// @Summary      Feed 流（支持多种策略）
// @Tags         feed
// @Produce      json
// @Param        type    query  string  true  "策略类型：latest | like_count | popularity | following"
// @Param        cursor  query  string  false "不透明游标，首页不传"
// @Param        limit   query  int     false "每页条数，默认 10，最大 50"
// @Success      200 {object} response.Response{data=dto.FeedResponse}
// @Router       /api/v1/feed [get]
func (h *FeedHandler) GetFeed(c *gin.Context) {
	var req dto.FeedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}

	ctx := c.Request.Context()
	if req.Type == "following" {
		uid := middleware.GetUserID(c)
		if uid == 0 {
			response.FailWithErr(c, errcode.Unauthorized)
			return
		}
		ctx = context.WithValue(ctx, service.FeedUserIDKey, uid)
	}

	resp, err := h.feedSvc.GetFeed(ctx, &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
