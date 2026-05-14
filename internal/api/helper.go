package api

import (
	"errors"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/pkg/response"
)

// handleSvcError 将 service 层错误转为统一 HTTP 响应。
func handleSvcError(c *gin.Context, err error) {
	var svcErr *errcode.ServiceError
	if errors.As(err, &svcErr) {
		response.Fail(c, svcErr.Code, svcErr.Error())
		return
	}
	response.FailWithErr(c, errcode.InternalError)
}
