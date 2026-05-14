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
