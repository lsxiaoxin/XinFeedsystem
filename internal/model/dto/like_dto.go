package dto

// LikeActionRequest action_type: 1=点赞 2=取消点赞
type LikeActionRequest struct {
	VideoID    int64 `json:"video_id,string" binding:"required"`
	ActionType int   `json:"action_type"     binding:"required,oneof=1 2"`
}

type LikeListRequest struct {
	CursorTime int64 `form:"cursor_time"`
	CursorID   int64 `form:"cursor_id"`
	Limit      int   `form:"limit"`
}

type LikeListResponse struct {
	Videos         []*VideoVO `json:"videos"`
	HasMore        bool       `json:"has_more"`
	NextCursorTime int64      `json:"next_cursor_time"`
	NextCursorID   int64      `json:"next_cursor_id"`
}
