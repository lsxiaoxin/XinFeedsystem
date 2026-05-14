package api

import (
	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/middleware"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/response"
)

type FollowHandler struct {
	followSvc *service.FollowService
}

func NewFollowHandler(followSvc *service.FollowService) *FollowHandler {
	return &FollowHandler{followSvc: followSvc}
}

// Action godoc
// @Summary      关注/取关
// @Tags         follow
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body dto.FollowActionRequest true "action_type: 1=关注 2=取关"
// @Success      200  {object} response.Response
// @Router       /api/v1/follow/action [post]
func (h *FollowHandler) Action(c *gin.Context) {
	var req dto.FollowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	uid := middleware.GetUserID(c)
	if err := h.followSvc.FollowAction(c.Request.Context(), uid, req.FolloweeID, req.ActionType); err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, nil)
}

// Following godoc
// @Summary      关注列表
// @Tags         follow
// @Produce      json
// @Param        user_id     query int true  "用户 ID"
// @Param        cursor_time query int false "游标 created_at(ms)"
// @Param        cursor_id   query int false "游标 ID"
// @Param        limit       query int false "每页条数，默认 10"
// @Success      200 {object} response.Response{data=dto.FollowListResponse}
// @Router       /api/v1/follow/following [get]
func (h *FollowHandler) Following(c *gin.Context) {
	var req dto.FollowListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	resp, err := h.followSvc.ListFollowing(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}

// Follower godoc
// @Summary      粉丝列表
// @Tags         follow
// @Produce      json
// @Param        user_id     query int true  "用户 ID"
// @Param        cursor_time query int false "游标 created_at(ms)"
// @Param        cursor_id   query int false "游标 ID"
// @Param        limit       query int false "每页条数，默认 10"
// @Success      200 {object} response.Response{data=dto.FollowListResponse}
// @Router       /api/v1/follow/follower [get]
func (h *FollowHandler) Follower(c *gin.Context) {
	var req dto.FollowListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	resp, err := h.followSvc.ListFollower(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
