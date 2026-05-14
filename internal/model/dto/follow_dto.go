package dto

// FollowActionRequest action_type: 1=关注 2=取关
type FollowActionRequest struct {
	FolloweeID int64 `json:"followee_id,string" binding:"required"`
	ActionType int   `json:"action_type"       binding:"required,oneof=1 2"`
}

type FollowListRequest struct {
	UserID     int64 `form:"user_id"     binding:"required"`
	CursorTime int64 `form:"cursor_time"`
	CursorID   int64 `form:"cursor_id"`
	Limit      int   `form:"limit"`
}

type FollowListResponse struct {
	Users          []*UserVO `json:"users"`
	HasMore        bool      `json:"has_more"`
	NextCursorTime int64     `json:"next_cursor_time"`
	NextCursorID   int64     `json:"next_cursor_id"`
}
