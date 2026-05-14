package dto

import "xinfeedsystem/internal/model/entity"

// ---------- 请求 ----------

// CommentActionRequest action_type: 1=发评论 2=删评论
type CommentActionRequest struct {
	ActionType int `json:"action_type" binding:"required,oneof=1 2"`

	// action_type=1 必填
	VideoID int64  `json:"video_id,string"`
	Content string `json:"content"`

	// action_type=2 必填
	CommentID int64 `json:"comment_id,string"`
}

type CommentListRequest struct {
	VideoID    int64 `form:"video_id" binding:"required"`
	CursorTime int64 `form:"cursor_time"`
	CursorID   int64 `form:"cursor_id"`
	Limit      int   `form:"limit"`
}

// ---------- 响应 ----------

type CommentVO struct {
	ID        int64   `json:"id,string"`
	VideoID   int64   `json:"video_id,string"`
	User      *UserVO `json:"user"`
	Content   string  `json:"content"`
	LikeCount int     `json:"like_count"`
	CreatedAt int64   `json:"created_at"`
}

type CommentListResponse struct {
	Comments       []*CommentVO `json:"comments"`
	HasMore        bool         `json:"has_more"`
	NextCursorTime int64        `json:"next_cursor_time"`
	NextCursorID   int64        `json:"next_cursor_id"`
}

func ToCommentVO(c *entity.Comment, user *UserVO) *CommentVO {
	return &CommentVO{
		ID:        c.ID,
		VideoID:   c.VideoID,
		User:      user,
		Content:   c.Content,
		LikeCount: c.LikeCount,
		CreatedAt: c.CreatedAt,
	}
}
