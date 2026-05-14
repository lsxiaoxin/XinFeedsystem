package dto

// ---------- Feed 请求/响应 ----------

type FeedRequest struct {
	// type 决定使用哪种策略：latest | like_count | popularity | following
	Type   string `form:"type" binding:"required"`
	Cursor string `form:"cursor"` // opaque base64 游标，首页不传
	Limit  int    `form:"limit"`
}

type FeedResponse struct {
	Videos     []*VideoVO `json:"videos"`
	NextCursor string     `json:"next_cursor"` // 原样传回下一页请求
	HasMore    bool       `json:"has_more"`
}
