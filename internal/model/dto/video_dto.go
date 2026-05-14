package dto

import "xinfeedsystem/internal/model/entity"

// ---------- 请求 ----------

// VideoPublishRequest multipart/form-data，用 ShouldBind（文件上传不支持 JSON，此处为必要例外）。
type VideoPublishRequest struct {
	Title    string `form:"title"    binding:"required,max=128"`
	Duration int    `form:"duration"`
}

type VideoListByAuthorRequest struct {
	AuthorID   int64 `form:"author_id"   binding:"required"`
	CursorTime int64 `form:"cursor_time"` // 上一页最后一条 created_at(ms)，首页传 0
	CursorID   int64 `form:"cursor_id"`   // 上一页最后一条 id，首页传 0
	Limit      int   `form:"limit"`
}

// ---------- 响应 ----------

type VideoVO struct {
	ID           int64   `json:"id,string"`
	AuthorID     int64   `json:"author_id,string"`
	Title        string  `json:"title"`
	PlayURL      string  `json:"play_url"`
	CoverURL     string  `json:"cover_url"`
	Duration     int     `json:"duration"`
	LikeCount    int     `json:"like_count"`
	CommentCount int     `json:"comment_count"`
	PlayCount    int64   `json:"play_count"`
	Heat         int64   `json:"heat"`
	Author       *UserVO `json:"author,omitempty"`
	CreatedAt    int64   `json:"created_at"`
}

type VideoListResponse struct {
	Videos         []*VideoVO `json:"videos"`
	HasMore        bool       `json:"has_more"`
	NextCursorTime int64      `json:"next_cursor_time"`
	NextCursorID   int64      `json:"next_cursor_id"`
}

func ToVideoVO(v *entity.Video) *VideoVO {
	return &VideoVO{
		ID:           v.ID,
		AuthorID:     v.AuthorID,
		Title:        v.Title,
		PlayURL:      v.PlayURL,
		CoverURL:     v.CoverURL,
		Duration:     v.Duration,
		LikeCount:    v.LikeCount,
		CommentCount: v.CommentCount,
		PlayCount:    v.PlayCount,
		Heat:         v.Heat,
		CreatedAt:    v.CreatedAt,
	}
}
