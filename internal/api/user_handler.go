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

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// Register godoc
// @Summary      注册
// @Tags         user
// @Accept       json
// @Produce      json
// @Param        body body dto.RegisterRequest true "注册信息"
// @Success      200  {object}  response.Response
// @Router       /api/v1/user/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	if err := h.userSvc.Register(c.Request.Context(), &req); err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, nil)
}

// Login godoc
// @Summary      登录
// @Tags         user
// @Accept       json
// @Produce      json
// @Param        body body dto.LoginRequest true "登录信息"
// @Success      200  {object}  response.Response{data=dto.LoginResponse}
// @Router       /api/v1/user/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam, err.Error())
		return
	}
	token, user, err := h.userSvc.Login(c.Request.Context(), &req)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, dto.LoginResponse{
		Token: token,
		User:  dto.ToUserVO(user),
	})
}

// GetUserInfo godoc
// @Summary      获取用户信息
// @Tags         user
// @Produce      json
// @Param        id   path      int true "用户 ID"
// @Success      200  {object}  response.Response{data=dto.UserVO}
// @Router       /api/v1/user/{id} [get]
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.InvalidParam, "invalid user id")
		return
	}
	user, err := h.userSvc.GetUserInfo(c.Request.Context(), id)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, dto.ToUserVO(user))
}

// GetMe godoc
// @Summary      获取当前登录用户信息
// @Tags         user
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  response.Response{data=dto.UserVO}
// @Router       /api/v1/user/me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	uid := middleware.GetUserID(c)
	user, err := h.userSvc.GetUserInfo(c.Request.Context(), uid)
	if err != nil {
		handleSvcError(c, err)
		return
	}
	response.OK(c, dto.ToUserVO(user))
}
