package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"xinfeedsystem/internal/model/entity"
)

type VideoRepository struct {
	db *gorm.DB
}

func NewVideoRepository(db *gorm.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Create(ctx context.Context, v *entity.Video) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *VideoRepository) FindByID(ctx context.Context, id int64) (*entity.Video, error) {
	var v entity.Video
	err := r.db.WithContext(ctx).First(&v, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

// ListByAuthorID 按作者 ID 游标分页，cursorTime/cursorID 为 0 时查第一页。
func (r *VideoRepository) ListByAuthorID(ctx context.Context, authorID, cursorTime, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Where("author_id = ? AND status = 1", authorID).
		Order("created_at DESC, id DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("created_at < ? OR (created_at = ? AND id < ?)", cursorTime, cursorTime, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}

// ListByFollowing 拉取 followerID 关注的人发布的视频，游标分页按发布时间倒序。
func (r *VideoRepository) ListByFollowing(ctx context.Context, followerID, cursorTime, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Where("author_id IN (SELECT followee_id FROM follows WHERE follower_id = ? AND deleted_at IS NULL)", followerID).
		Where("status = 1").
		Order("created_at DESC, id DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("created_at < ? OR (created_at = ? AND id < ?)", cursorTime, cursorTime, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}

// ListByHeat 按热度倒序游标分页，供 PopularityFetcher 使用。
func (r *VideoRepository) ListByHeat(ctx context.Context, cursorHeat, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Where("status = 1").
		Order("heat DESC, id DESC").
		Limit(limit)

	if cursorHeat > 0 {
		q = q.Where("heat < ? OR (heat = ? AND id < ?)", cursorHeat, cursorHeat, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}

// ListLatest 全站最新视频游标分页，供 LatestFeedFetcher 使用。
func (r *VideoRepository) ListLatest(ctx context.Context, cursorTime, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Where("status = 1").
		Order("created_at DESC, id DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("created_at < ? OR (created_at = ? AND id < ?)", cursorTime, cursorTime, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}
