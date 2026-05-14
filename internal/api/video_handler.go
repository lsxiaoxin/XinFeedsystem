package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/middleware"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/response"
)

type VideoHandler struct {
	videoSvc *service.VideoService
}

func NewVideoHandler(videoSvc *service.VideoService) *VideoHandler {
	return &VideoHandler{videoSvc: videoSvc}
}

// Publish godoc
// @Summary      上传视频
// @Tags         video
// @Security     BearerAuth
// @Accept       mpfd
// @Produce      json
// @Param        title    formData  string  true  "标题"
// @Param        duration formData  int     false "时长（秒）"
// @Param        video    formData  file    true  "视频文件"
// @Success      200  {object}  response.Response{data=dto.VideoVO}
// @Router       /api/v1/video/publish [post]
func (h *VideoHandler) Publish(c *gin.Context) {
	// multipart 文件上传，必须用 ShouldBind（非 JSON 的必要例外）
	var req dto.VideoPublishRequest
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}

	file, header, err := c.Request.FormFile("video")
	if err != nil {
		response.Fail(c, errcode.InvalidParam, "missing video file")
		return
	}
	defer file.Close()

	uid := middleware.GetUserID(c)
	v, err := h.videoSvc.Publish(c.Request.Context(), uid, &req, file, header)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, dto.ToVideoVO(v))
}

// GetDetail godoc
// @Summary      视频详情
// @Tags         video
// @Produce      json
// @Param        id  path  int  true  "视频 ID"
// @Success      200 {object} response.Response{data=dto.VideoVO}
// @Router       /api/v1/video/{id} [get]
func (h *VideoHandler) GetDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam, "invalid video id")
		return
	}
	v, err := h.videoSvc.GetDetail(c.Request.Context(), id)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, dto.ToVideoVO(v))
}

// ListByAuthorID godoc
// @Summary      作者视频列表（游标分页）
// @Tags         video
// @Produce      json
// @Param        author_id    query  int  true  "作者 ID"
// @Param        cursor_time  query  int  false "游标 created_at(ms)，首页不传"
// @Param        cursor_id    query  int  false "游标 ID，首页不传"
// @Param        limit        query  int  false "每页条数，默认 10，最大 50"
// @Success      200 {object} response.Response{data=dto.VideoListResponse}
// @Router       /api/v1/video/list [get]
func (h *VideoHandler) ListByAuthorID(c *gin.Context) {
	var req dto.VideoListByAuthorRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	resp, err := h.videoSvc.ListByAuthorID(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, resp)
}
