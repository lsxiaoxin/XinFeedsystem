package service

import (
	"context"
	"errors"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/repository"
)

type LikeService struct {
	likeRepo *repository.LikeRepository
}

func NewLikeService(likeRepo *repository.LikeRepository) *LikeService {
	return &LikeService{likeRepo: likeRepo}
}

func (s *LikeService) LikeAction(ctx context.Context, userID, videoID int64, actionType int) error {
	var err error
	if actionType == 1 {
		err = s.likeRepo.Like(ctx, userID, videoID)
	} else {
		err = s.likeRepo.Unlike(ctx, userID, videoID)
	}

	if errors.Is(err, repository.ErrAlreadyLiked) {
		return errcode.New(errcode.AlreadyLiked)
	}
	if errors.Is(err, repository.ErrNotLikedYet) {
		return errcode.New(errcode.NotLikedYet)
	}
	return err
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
