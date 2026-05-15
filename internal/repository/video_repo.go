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

	if cursorID > 0 {
		q = q.Where("heat < ? OR (heat = ? AND id < ?)", cursorHeat, cursorHeat, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}

// ListByLikeCount 按点赞数倒序游标分页，供 LikeCountFetcher 使用。
func (r *VideoRepository) ListByLikeCount(ctx context.Context, cursorLikes, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Where("status = 1").
		Order("like_count DESC, id DESC").
		Limit(limit)

	if cursorID > 0 {
		q = q.Where("like_count < ? OR (like_count = ? AND id < ?)", cursorLikes, cursorLikes, cursorID)
	}

	var list []*entity.Video
	return list, q.Find(&list).Error
}

// FindByIDs 按 ID 列表批量查询，返回找到的全部视频（顺序不保证，由调用方排序）。
func (r *VideoRepository) FindByIDs(ctx context.Context, ids []int64) ([]*entity.Video, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []*entity.Video
	return list, r.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error
}

// CounterDelta 记录单个视频需要原子增加的各计数字段增量。
type CounterDelta struct {
	LikeDelta    int64
	CommentDelta int64
	HeatDelta    int64
}

// ApplyCounterDeltas 在单个事务内对每个视频执行一条原子 UPDATE，
// 批量应用来自 Kafka consumer 聚合后的计数增量。
func (r *VideoRepository) ApplyCounterDeltas(ctx context.Context, deltas map[int64]CounterDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for videoID, d := range deltas {
			updates := map[string]any{
				"like_count":    gorm.Expr("GREATEST(0, like_count + ?)", d.LikeDelta),
				"comment_count": gorm.Expr("GREATEST(0, comment_count + ?)", d.CommentDelta),
				"heat":          gorm.Expr("GREATEST(0, heat + ?)", d.HeatDelta),
			}
			if err := tx.Model(&entity.Video{}).Where("id = ?", videoID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
