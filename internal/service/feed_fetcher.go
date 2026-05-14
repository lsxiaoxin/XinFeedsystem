package service

import (
	"context"

	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
)

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
// 扩展占位（实现接口后在 main.go 注册即可）：
//
//   type LikeCountFetcher struct{ videoRepo *repository.VideoRepository }
//   func (f *LikeCountFetcher) Type() string { return "like_count" }
//   func (f *LikeCountFetcher) ScoreOf(v *entity.Video) int64 { return int64(v.LikeCount) }
//
//   type FollowingFetcher struct{ videoRepo *repository.VideoRepository; followRepo *repository.FollowRepository }
//   func (f *FollowingFetcher) Type() string { return "following" }
//   func (f *FollowingFetcher) ScoreOf(v *entity.Video) int64 { return v.CreatedAt }
// ──────────────────────────────────────────────
