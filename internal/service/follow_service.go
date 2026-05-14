package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
)

type FollowService struct {
	followRepo *repository.FollowRepository
	userRepo   *repository.UserRepository
	rdb        *redis.Client
}

func NewFollowService(followRepo *repository.FollowRepository, userRepo *repository.UserRepository, rdb *redis.Client) *FollowService {
	return &FollowService{followRepo: followRepo, userRepo: userRepo, rdb: rdb}
}

func (s *FollowService) FollowAction(ctx context.Context, followerID, followeeID int64, actionType int) error {
	if followerID == followeeID {
		return errcode.New(errcode.InvalidParam)
	}
	var err error
	if actionType == 1 {
		err = s.followRepo.Follow(ctx, followerID, followeeID)
		if errors.Is(err, repository.ErrAlreadyFollowed) {
			return errcode.New(errcode.AlreadyFollowed)
		}
	} else {
		err = s.followRepo.Unfollow(ctx, followerID, followeeID)
		if errors.Is(err, repository.ErrNotFollowedYet) {
			return errcode.New(errcode.NotFollowedYet)
		}
	}
	if err != nil {
		return err
	}
	// follow_count / follower_count 变了，失效双方缓存
	_ = s.rdb.Del(ctx,
		fmt.Sprintf("user:info:%d", followerID),
		fmt.Sprintf("user:info:%d", followeeID),
	)
	return nil
}

func (s *FollowService) ListFollowing(ctx context.Context, req *dto.FollowListRequest) (*dto.FollowListResponse, error) {
	limit := normalizeLimit(req.Limit)
	users, cursors, err := s.followRepo.ListFollowing(ctx, req.UserID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}
	return buildFollowResponse(users, cursors, limit), nil
}

func (s *FollowService) ListFollower(ctx context.Context, req *dto.FollowListRequest) (*dto.FollowListResponse, error) {
	limit := normalizeLimit(req.Limit)
	users, cursors, err := s.followRepo.ListFollower(ctx, req.UserID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}
	return buildFollowResponse(users, cursors, limit), nil
}

// buildFollowResponse 截断到 limit 条，用 cursors[limit-1]（最后一条返回行）生成游标。
func buildFollowResponse(users []*entity.User, cursors [][2]int64, limit int) *dto.FollowListResponse {
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	vos := make([]*dto.UserVO, len(users))
	for i, u := range users {
		vos[i] = dto.ToUserVO(u)
	}
	resp := &dto.FollowListResponse{Users: vos, HasMore: hasMore}
	if hasMore && limit <= len(cursors) {
		resp.NextCursorTime = cursors[limit-1][0]
		resp.NextCursorID = cursors[limit-1][1]
	}
	return resp
}
