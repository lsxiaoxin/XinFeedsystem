package api

import (
	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/middleware"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/response"
)

type CommentHandler struct {
	commentSvc *service.CommentService
}

func NewCommentHandler(commentSvc *service.CommentService) *CommentHandler {
	return &CommentHandler{commentSvc: commentSvc}
}

// Action godoc
// @Summary      发评论/删评论
// @Tags         comment
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body dto.CommentActionRequest true "action_type: 1=发 2=删"
// @Success      200  {object} response.Response{data=dto.CommentVO}
// @Router       /api/v1/comment/action [post]
func (h *CommentHandler) Action(c *gin.Context) {
	var req dto.CommentActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}

	uid := middleware.GetUserID(c)

	if req.ActionType == 1 {
		vo, err := h.commentSvc.Post(c.Request.Context(), uid, &req)
		if err != nil {
			handleSvcError(c, err)
			return
		}
		response.OK(c, vo)
	} else {
		if err := h.commentSvc.Delete(c.Request.Context(), uid, &req); err != nil {
			handleSvcError(c, err)
			return
		}
		response.OK(c, nil)
	}
}

// List godoc
// @Summary      评论列表
// @Tags         comment
// @Produce      json
// @Param        video_id    query  int  true  "视频 ID"
// @Param        cursor_time query  int  false "游标 created_at(ms)"
// @Param        cursor_id   query  int  false "游标 ID"
// @Param        limit       query  int  false "每页条数，默认 10"
// @Success      200 {object} response.Response{data=dto.CommentListResponse}
// @Router       /api/v1/comment/list [get]
func (h *CommentHandler) List(c *gin.Context) {
	var req dto.CommentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	resp, err := h.commentSvc.List(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
