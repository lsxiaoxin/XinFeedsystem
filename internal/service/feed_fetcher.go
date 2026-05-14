package service

import (
	"context"

	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
)

// feedCtxKey 避免与其他包的 string key 冲突。
type feedCtxKey string

// FeedUserIDKey 供 handler 注入当前登录用户 ID，FollowingFetcher 读取。
const FeedUserIDKey feedCtxKey = "feed_user_id"

// FeedFetcher 是所有 Feed 策略的公共接口。
// 新增一种 Feed 类型只需实现此接口并注册到 FeedService，无需改动已有代码。
//
// Fetch 负责查询数据，调用方传入 (score, cursorID, limit)，返回视频列表。
// ScoreOf 返回某条视频在该策略下的游标得分，FeedService 用它生成 next_cursor。
//   - LatestFetcher    → ScoreOf = v.CreatedAt  (ms)
//   - LikeCountFetcher → ScoreOf = int64(v.LikeCount)
//   - PopularityFetcher → ScoreOf = 综合分
type FeedFetcher interface {
	// Type 返回策略唯一标识，用于路由分发。
	Type() string
	// Fetch 返回最多 limit 条视频。
	Fetch(ctx context.Context, score, cursorID int64, limit int) ([]*entity.Video, error)
	// ScoreOf 返回该条视频在此策略下的排序得分，用于生成游标。
	ScoreOf(v *entity.Video) int64
}

// ──────────────────────────────────────────────
// LatestFetcher  按发布时间倒序
// ──────────────────────────────────────────────

type LatestFetcher struct {
	videoRepo *repository.VideoRepository
}

func NewLatestFetcher(videoRepo *repository.VideoRepository) *LatestFetcher {
	return &LatestFetcher{videoRepo: videoRepo}
}

func (f *LatestFetcher) Type() string { return "latest" }

func (f *LatestFetcher) Fetch(ctx context.Context, score, cursorID int64, limit int) ([]*entity.Video, error) {
	return f.videoRepo.ListLatest(ctx, score, cursorID, limit)
}

// ScoreOf 对 LatestFetcher 来说，排序得分就是 created_at (ms)。
func (f *LatestFetcher) ScoreOf(v *entity.Video) int64 { return v.CreatedAt }

// ──────────────────────────────────────────────
// FollowingFetcher  关注流（拉模式，按发布时间倒序）
// ──────────────────────────────────────────────

type FollowingFetcher struct {
	videoRepo *repository.VideoRepository
}

func NewFollowingFetcher(videoRepo *repository.VideoRepository) *FollowingFetcher {
	return &FollowingFetcher{videoRepo: videoRepo}
}

func (f *FollowingFetcher) Type() string { return "following" }

// Fetch 从 context 取登录用户 ID，查其关注的人发布的视频。
func (f *FollowingFetcher) Fetch(ctx context.Context, score, cursorID int64, limit int) ([]*entity.Video, error) {
	followerID, _ := ctx.Value(FeedUserIDKey).(int64)
	return f.videoRepo.ListByFollowing(ctx, followerID, score, cursorID, limit)
}

func (f *FollowingFetcher) ScoreOf(v *entity.Video) int64 { return v.CreatedAt }
