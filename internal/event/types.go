package event

// LikeEvent 由点赞/取消点赞操作触发，写入 like_topic。
type LikeEvent struct {
	EventID string `json:"event_id"` // UUID，consumer 端用于幂等去重
	VideoID int64  `json:"video_id"`
	UserID  int64  `json:"user_id"`
	Delta   int8   `json:"delta"` // +1 点赞，-1 取消
	TS      int64  `json:"ts"`    // 毫秒时间戳
}

// CommentEvent 由发评论/删评论操作触发，写入 comment_topic。
type CommentEvent struct {
	EventID string `json:"event_id"`
	VideoID int64  `json:"video_id"`
	UserID  int64  `json:"user_id"`
	Delta   int8   `json:"delta"` // +1 发评论，-1 删评论
	TS      int64  `json:"ts"`
}
