package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/event"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/repository"
)

type LikeService struct {
	likeRepo *repository.LikeRepository
	rdb      *redis.Client
	producer *event.Producer
}

func NewLikeService(likeRepo *repository.LikeRepository, rdb *redis.Client, producer *event.Producer) *LikeService {
	return &LikeService{likeRepo: likeRepo, rdb: rdb, producer: producer}
}

func (s *LikeService) LikeAction(ctx context.Context, userID, videoID int64, actionType int) error {
	delta, err := s.likeRepo.Toggle(ctx, userID, videoID, actionType)
	if err != nil {
		return err
	}
	if delta == 0 {
		return nil // 状态未变，幂等返回
	}

	_ = s.rdb.Del(ctx, fmt.Sprintf("video:detail:%d", videoID))

	s.producer.EmitLike(ctx, event.LikeEvent{
		EventID: uuid.NewString(),
		VideoID: videoID,
		UserID:  userID,
		Delta:   delta,
		TS:      time.Now().UnixMilli(),
	})
	return nil
}

func (s *LikeService) ListLikedVideos(ctx context.Context, userID int64, req *dto.LikeListRequest) (*dto.LikeListResponse, error) {
	limit := normalizeLimit(req.Limit)
	videos, err := s.likeRepo.ListLikedVideos(ctx, userID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(videos) > limit
	if hasMore {
		videos = videos[:limit]
	}

	vos := make([]*dto.VideoVO, len(videos))
	for i, v := range videos {
		vos[i] = dto.ToVideoVO(v)
	}

	resp := &dto.LikeListResponse{Videos: vos, HasMore: hasMore}
	if hasMore && len(videos) > 0 {
		last := videos[len(videos)-1]
		resp.NextCursorTime = last.CreatedAt
		resp.NextCursorID = last.ID
	}
	return resp, nil
}

