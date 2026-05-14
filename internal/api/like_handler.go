package api

import (
	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/middleware"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/response"
)

type LikeHandler struct {
	likeSvc *service.LikeService
}

func NewLikeHandler(likeSvc *service.LikeService) *LikeHandler {
	return &LikeHandler{likeSvc: likeSvc}
}

// Action godoc
// @Summary      点赞/取消点赞
// @Tags         like
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body dto.LikeActionRequest true "action_type: 1=点赞 2=取消"
// @Success      200  {object} response.Response
// @Router       /api/v1/like/action [post]
func (h *LikeHandler) Action(c *gin.Context) {
	var req dto.LikeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	uid := middleware.GetUserID(c)
	if err := h.likeSvc.LikeAction(c.Request.Context(), uid, req.VideoID, req.ActionType); err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, nil)
}

// List godoc
// @Summary      我点赞过的视频列表
// @Tags         like
// @Security     BearerAuth
// @Produce      json
// @Param        cursor_time  query  int  false "游标 created_at(ms)"
// @Param        cursor_id    query  int  false "游标 ID"
// @Param        limit        query  int  false "每页条数，默认 10"
// @Success      200 {object} response.Response{data=dto.LikeListResponse}
// @Router       /api/v1/like/list [get]
func (h *LikeHandler) List(c *gin.Context) {
	var req dto.LikeListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	uid := middleware.GetUserID(c)
	resp, err := h.likeSvc.ListLikedVideos(c.Request.Context(), uid, &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
