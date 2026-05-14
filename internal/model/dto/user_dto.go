package dto

import "xinfeedsystem/internal/model/entity"

// ---------- 请求 ----------

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
	Nickname string `json:"nickname" binding:"required,max=32"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ---------- 响应 ----------

// UserVO 用户视图对象。ID 序列化为字符串，防止 JS 精度丢失（雪花 ID 超 2^53）。
type UserVO struct {
	ID            int64  `json:"id,string"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar"`
	Signature     string `json:"signature"`
	FollowCount   int    `json:"follow_count"`
	FollowerCount int    `json:"follower_count"`
}

type LoginResponse struct {
	Token string  `json:"token"`
	User  *UserVO `json:"user"`
}

func ToUserVO(u *entity.User) *UserVO {
	return &UserVO{
		ID:            u.ID,
		Username:      u.Username,
		Nickname:      u.Nickname,
		Avatar:        u.Avatar,
		Signature:     u.Signature,
		FollowCount:   u.FollowCount,
		FollowerCount: u.FollowerCount,
	}
}
