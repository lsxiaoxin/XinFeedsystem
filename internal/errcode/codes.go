package errcode

const (
	OK = 0

	// 通用
	InvalidParam    = 40001
	Unauthorized    = 40100
	Forbidden       = 40300
	NotFound        = 40400
	TooManyRequests = 42900
	InternalError   = 50000

	// 用户模块
	UserNotFound       = 10001
	UserAlreadyExists  = 10002
	WrongPassword      = 10003
	TokenExpired       = 10004
	TokenInvalid       = 10005

	// 视频模块
	VideoNotFound   = 20001
	VideoUploadFail = 20002

	// 点赞模块
	AlreadyLiked  = 30001
	NotLikedYet   = 30002

	// 评论模块
	CommentNotFound = 60001

	// 关注模块
	AlreadyFollowed  = 50001
	NotFollowedYet   = 50002
	CannotFollowSelf = 50003
)

var msgs = map[int]string{
	OK:               "ok",
	InvalidParam:     "参数错误",
	Unauthorized:     "未登录",
	Forbidden:        "无权限",
	NotFound:         "资源不存在",
	TooManyRequests:  "请求过于频繁",
	InternalError:    "服务器内部错误",
	UserNotFound:     "用户不存在",
	UserAlreadyExists: "用户名已存在",
	WrongPassword:    "密码错误",
	TokenExpired:     "token 已过期",
	TokenInvalid:     "token 无效",
	VideoNotFound:    "视频不存在",
	VideoUploadFail:  "视频上传失败",
	AlreadyLiked:     "已点赞",
	NotLikedYet:      "未点赞",
	CommentNotFound:  "评论不存在",
	AlreadyFollowed:  "已关注",
	NotFollowedYet:   "未关注",
	CannotFollowSelf: "不能关注自己",
}

func Msg(code int) string {
	if m, ok := msgs[code]; ok {
		return m
	}
	return "未知错误"
}
