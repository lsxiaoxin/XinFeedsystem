package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/config"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/cache"
	"xinfeedsystem/pkg/redislock"
	"xinfeedsystem/pkg/snowflake"
)

const defaultLimit = 10
const maxLimit = 50

type VideoService struct {
	videoRepo *repository.VideoRepository
	store     config.StorageConfig
	rdb       *redis.Client
	lock      *redislock.Lock
}

func NewVideoService(videoRepo *repository.VideoRepository, store config.StorageConfig, rdb *redis.Client) *VideoService {
	return &VideoService{videoRepo: videoRepo, store: store, rdb: rdb, lock: redislock.New(rdb)}
}

func videoDetailKey(id int64) string { return fmt.Sprintf("video:detail:%d", id) }

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
	key := videoDetailKey(id)

	// Fast path: cache already populated, no lock needed.
	var v entity.Video
	if hit, isNil, err := cache.GetJSON(ctx, s.rdb, key, &v); err == nil && hit {
		if isNil {
			return nil, errcode.New(errcode.VideoNotFound)
		}
		return &v, nil
	}

	// Cache miss: only one goroutine queries the DB; the rest poll until it writes back.
	result, err := s.lock.Do(ctx,
		fmt.Sprintf("lock:video:detail:%d", id),
		3*time.Second,   // lock TTL: loader must finish in < 3s
		500*time.Millisecond, // max wait for losers
		func() (interface{}, error) { // loader (lock winner)
			video, err := s.videoRepo.FindByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if video == nil {
				_ = cache.SetNil(ctx, s.rdb, key, 30*time.Second)
				return nil, errcode.New(errcode.VideoNotFound)
			}
			_ = cache.SetJSON(ctx, s.rdb, key, video, cache.RandomizedTTL(5*time.Minute, 30*time.Second))
			return video, nil
		},
		func() (interface{}, bool, error) { // waiter (lock losers polling cache)
			var cached entity.Video
			hit, isNil, err := cache.GetJSON(ctx, s.rdb, key, &cached)
			if err != nil || !hit {
				return nil, false, nil // not ready yet, keep polling
			}
			if isNil {
				return nil, true, errcode.New(errcode.VideoNotFound)
			}
			return &cached, true, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return result.(*entity.Video), nil
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
