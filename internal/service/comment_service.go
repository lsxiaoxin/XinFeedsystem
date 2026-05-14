package service

import (
	"context"
	"errors"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/snowflake"
)

type CommentService struct {
	commentRepo *repository.CommentRepository
	userRepo    *repository.UserRepository
}

func NewCommentService(commentRepo *repository.CommentRepository, userRepo *repository.UserRepository) *CommentService {
	return &CommentService{commentRepo: commentRepo, userRepo: userRepo}
}

func (s *CommentService) Post(ctx context.Context, userID int64, req *dto.CommentActionRequest) (*dto.CommentVO, error) {
	if req.VideoID == 0 || req.Content == "" {
		return nil, errcode.New(errcode.InvalidParam)
	}
	if len([]rune(req.Content)) > 1000 {
		return nil, errcode.New(errcode.InvalidParam)
	}

	c := &entity.Comment{
		ID:      snowflake.NewID(),
		VideoID: req.VideoID,
		UserID:  userID,
		Content: req.Content,
	}
	if err := s.commentRepo.Create(ctx, c); err != nil {
		return nil, err
	}

	user, _ := s.userRepo.FindByID(ctx, userID)
	var userVO *dto.UserVO
	if user != nil {
		userVO = dto.ToUserVO(user)
	}
	return dto.ToCommentVO(c, userVO), nil
}

func (s *CommentService) Delete(ctx context.Context, userID int64, req *dto.CommentActionRequest) error {
	if req.CommentID == 0 {
		return errcode.New(errcode.InvalidParam)
	}
	err := s.commentRepo.Delete(ctx, req.CommentID, userID)
	if errors.Is(err, repository.ErrCommentNotFound) {
		return errcode.New(errcode.CommentNotFound)
	}
	if errors.Is(err, repository.ErrCommentForbidden) {
		return errcode.New(errcode.Forbidden)
	}
	return err
}

func (s *CommentService) List(ctx context.Context, req *dto.CommentListRequest) (*dto.CommentListResponse, error) {
	limit := normalizeLimit(req.Limit)
	comments, err := s.commentRepo.ListByVideoID(ctx, req.VideoID, req.CursorTime, req.CursorID, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}

	// 批量取用户信息，避免 N+1
	userIDs := make([]int64, 0, len(comments))
	for _, c := range comments {
		userIDs = append(userIDs, c.UserID)
	}
	userMap, err := s.userRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	vos := make([]*dto.CommentVO, len(comments))
	for i, c := range comments {
		var userVO *dto.UserVO
		if u, ok := userMap[c.UserID]; ok {
			userVO = dto.ToUserVO(u)
		}
		vos[i] = dto.ToCommentVO(c, userVO)
	}

	resp := &dto.CommentListResponse{Comments: vos, HasMore: hasMore}
	if hasMore && len(comments) > 0 {
		last := comments[len(comments)-1]
		resp.NextCursorTime = last.CreatedAt
		resp.NextCursorID = last.ID
	}
	return resp, nil
}
