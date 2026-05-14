package api

import (
	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
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
	resp, err := h.feedSvc.GetFeed(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
