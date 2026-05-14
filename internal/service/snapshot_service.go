package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/model/entity"
)

const snapshotSize = 1000

// SnapshotService rebuilds Redis ZSet snapshots for ranked feeds every minute.
//
// Key layout:
//
//	snapshot:{type}:current   → latest epoch (string, no TTL)
//	snapshot:{type}:v{epoch}  → ZSet member=video_id score=heat|like_count (TTL 15min)
type SnapshotService struct {
	videoRepo videoStore
	rdb       *redis.Client
}

func NewSnapshotService(videoRepo videoStore, rdb *redis.Client) *SnapshotService {
	return &SnapshotService{videoRepo: videoRepo, rdb: rdb}
}

// Start launches a background goroutine that rebuilds both snapshots immediately,
// then once per minute until ctx is cancelled.
func (s *SnapshotService) Start(ctx context.Context) {
	go func() {
		s.rebuild(ctx, "popularity")
		s.rebuild(ctx, "like_count")

		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.rebuild(ctx, "popularity")
				s.rebuild(ctx, "like_count")
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *SnapshotService) rebuild(ctx context.Context, snapType string) {
	videos, err := s.topVideos(ctx, snapType)
	if err != nil || len(videos) == 0 {
		return
	}

	epoch := time.Now().Unix()
	snapKey := fmt.Sprintf("snapshot:%s:v%d", snapType, epoch)

	pipe := s.rdb.Pipeline()
	for _, v := range videos {
		pipe.ZAdd(ctx, snapKey, redis.Z{
			Score:  s.scoreOf(v, snapType),
			Member: strconv.FormatInt(v.ID, 10),
		})
	}
	pipe.Expire(ctx, snapKey, 15*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return
	}

	// Update current pointer only after the ZSet is fully written.
	s.rdb.Set(ctx, fmt.Sprintf("snapshot:%s:current", snapType), epoch, 0)
}

func (s *SnapshotService) topVideos(ctx context.Context, snapType string) ([]*entity.Video, error) {
	switch snapType {
	case "popularity":
		return s.videoRepo.ListByHeat(ctx, 0, 0, snapshotSize)
	default:
		return s.videoRepo.ListByLikeCount(ctx, 0, 0, snapshotSize)
	}
}

func (s *SnapshotService) scoreOf(v *entity.Video, snapType string) float64 {
	if snapType == "popularity" {
		return float64(v.Heat)
	}
	return float64(v.LikeCount)
}
