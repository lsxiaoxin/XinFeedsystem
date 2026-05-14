package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"xinfeedsystem/internal/errcode"
)

type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code: errcode.OK,
		Msg:  "ok",
		Data: data,
	})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Msg:  msg,
		Data: nil,
	})
}

func FailWithErr(c *gin.Context, code int) {
	Fail(c, code, errcode.Msg(code))
}
