package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"xinfeedsystem/config"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/snowflake"
)

const defaultLimit = 10
const maxLimit = 50

type VideoService struct {
	videoRepo *repository.VideoRepository
	store     config.StorageConfig
}

func NewVideoService(videoRepo *repository.VideoRepository, store config.StorageConfig) *VideoService {
	return &VideoService{videoRepo: videoRepo, store: store}
}

func (s *VideoService) Publish(ctx context.Context, authorID int64, req *dto.VideoPublishRequest, file multipart.File, header *multipart.FileHeader) (*entity.Video, error) {
	if header.Size > int64(s.store.MaxVideoSizeMB)*1024*1024 {
		return nil, errcode.New(errcode.VideoUploadFail)
	}

	id := snowflake.NewID()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".mp4"
	}
	filename := fmt.Sprintf("%d%s", id, ext)
	savePath := filepath.Join(s.store.BaseDir, s.store.VideoDir, filename)

	dst, err := os.Create(savePath)
	if err != nil {
		return nil, errcode.New(errcode.VideoUploadFail)
	}
	defer dst.Close()
	if _, err = io.Copy(dst, file); err != nil {
		os.Remove(savePath)
		return nil, errcode.New(errcode.VideoUploadFail)
	}

	v := &entity.Video{
		ID:       id,
		AuthorID: authorID,
		Title:    req.Title,
		PlayURL:  fmt.Sprintf("%s/%s", s.store.VideoURLPrefix, filename),
		CoverURL: "",
		Duration: req.Duration,
		Status:   1,
	}
	if err := s.videoRepo.Create(ctx, v); err != nil {
		os.Remove(savePath)
		return nil, err
	}
	return v, nil
}

func (s *VideoService) GetDetail(ctx context.Context, id int64) (*entity.Video, error) {
	v, err := s.videoRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errcode.New(errcode.VideoNotFound)
	}
	return v, nil
}

func (s *VideoService) ListByAuthorID(ctx context.Context, req *dto.VideoListByAuthorRequest) (*dto.VideoListResponse, error) {
	limit := normalizeLimit(req.Limit)
	// 多取 1 条判断是否有下一页
	videos, err := s.videoRepo.ListByAuthorID(ctx, req.AuthorID, req.CursorTime, req.CursorID, limit+1)
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

	resp := &dto.VideoListResponse{Videos: vos, HasMore: hasMore}
	if hasMore && len(videos) > 0 {
		last := videos[len(videos)-1]
		resp.NextCursorTime = last.CreatedAt
		resp.NextCursorID = last.ID
	}
	return resp, nil
}

func normalizeLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}
