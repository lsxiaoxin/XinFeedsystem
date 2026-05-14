package service

import (
	"context"
	"errors"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/repository"
)

type FollowService struct {
	followRepo *repository.FollowRepository
	userRepo   *repository.UserRepository
}

func NewFollowService(followRepo *repository.FollowRepository, userRepo *repository.UserRepository) *FollowService {
	return &FollowService{followRepo: followRepo, userRepo: userRepo}
}

func (s *FollowService) FollowAction(ctx context.Context, followerID, followeeID int64, actionType int) error {
	if followerID == followeeID {
		return errcode.New(errcode.InvalidParam)
	}
	if actionType == 1 {
		err := s.followRepo.Follow(ctx, followerID, followeeID)
		if errors.Is(err, repository.ErrAlreadyFollowed) {
			return errcode.New(errcode.AlreadyFollowed)
		}
		return err
	}
	err := s.followRepo.Unfollow(ctx, followerID, followeeID)
	if errors.Is(err, repository.ErrNotFollowedYet) {
		return errcode.New(errcode.NotFollowedYet)
	}
	return err
}

func (s *FollowService) ListFollowing(ctx context.Context, req *dto.FollowListRequest) (*dto.FollowListResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	users, last, err := s.followRepo.ListFollowing(ctx, req.UserID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	vos := make([]*dto.UserVO, len(users))
	for i, u := range users {
		vos[i] = dto.ToUserVO(u)
	}
	resp := &dto.FollowListResponse{
		Users:   vos,
		HasMore: hasMore,
	}
	if hasMore {
		resp.NextCursorTime = last[0]
		resp.NextCursorID = last[1]
	}
	return resp, nil
}

func (s *FollowService) ListFollower(ctx context.Context, req *dto.FollowListRequest) (*dto.FollowListResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	users, last, err := s.followRepo.ListFollower(ctx, req.UserID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	vos := make([]*dto.UserVO, len(users))
	for i, u := range users {
		vos[i] = dto.ToUserVO(u)
	}
	resp := &dto.FollowListResponse{
		Users:   vos,
		HasMore: hasMore,
	}
	if hasMore {
		resp.NextCursorTime = last[0]
		resp.NextCursorID = last[1]
	}
	return resp, nil
}
