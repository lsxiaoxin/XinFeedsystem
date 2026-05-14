package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/pkg/snowflake"
)

type LikeRepository struct {
	db *gorm.DB
}

func NewLikeRepository(db *gorm.DB) *LikeRepository {
	return &LikeRepository{db: db}
}

// FindVideoLike 查询点赞记录，Unscoped 包含软删除行（判断是否曾经点赞过）。
func (r *LikeRepository) FindVideoLike(ctx context.Context, userID, videoID int64) (*entity.VideoLike, error) {
	var like entity.VideoLike
	err := r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&like).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &like, err
}

// Like 在事务内完成点赞 + 视频计数 +1。
// 若已点赞（deleted_at IS NULL）则返回 ErrAlreadyLiked。
func (r *LikeRepository) Like(ctx context.Context, userID, videoID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var like entity.VideoLike
		err := tx.Unscoped().
			Where("user_id = ? AND video_id = ?", userID, videoID).
			First(&like).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 首次点赞：INSERT
			like = entity.VideoLike{
				ID:      snowflake.NewID(),
				UserID:  userID,
				VideoID: videoID,
			}
			if err := tx.Create(&like).Error; err != nil {
				return err
			}
		case err == nil && like.DeletedAt.Valid:
			// 曾经取消过：恢复软删
			if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		case err == nil && !like.DeletedAt.Valid:
			return ErrAlreadyLiked
		default:
			return err
		}

		if err := tx.Model(&entity.Video{}).Where("id = ?", videoID).
			UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&entity.Video{}).Where("id = ?", videoID).
			UpdateColumn("heat", gorm.Expr("heat + 1")).Error
	})
}

// Unlike 在事务内完成取消点赞 + 视频计数 -1（计数最小为 0）。
func (r *LikeRepository) Unlike(ctx context.Context, userID, videoID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var like entity.VideoLike
		err := tx.Where("user_id = ? AND video_id = ?", userID, videoID).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotLikedYet
		}
		if err != nil {
			return err
		}

		// 软删除
		now := time.Now()
		if err := tx.Unscoped().Model(&like).Update("deleted_at", now).Error; err != nil {
			return err
		}

		return tx.Model(&entity.Video{}).Where("id = ? AND like_count > 0", videoID).
			UpdateColumn("like_count", gorm.Expr("like_count - 1")).Error
	})
}

// ListLikedVideos 查询用户点赞过的视频（游标分页，按点赞时间倒序）。
func (r *LikeRepository) ListLikedVideos(ctx context.Context, userID, cursorTime, cursorID int64, limit int) ([]*entity.Video, error) {
	q := r.db.WithContext(ctx).
		Table("video_likes vl").
		Select("v.*").
		Joins("JOIN videos v ON v.id = vl.video_id AND v.deleted_at IS NULL").
		Where("vl.user_id = ? AND vl.deleted_at IS NULL AND v.status = 1", userID).
		Order("vl.created_at DESC, vl.id DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("vl.created_at < ? OR (vl.created_at = ? AND vl.id < ?)",
			cursorTime, cursorTime, cursorID)
	}

	var list []*entity.Video
	return list, q.Scan(&list).Error
}

// sentinel errors（不跨包暴露，由 service 层转换为 errcode）
var (
	ErrAlreadyLiked = errors.New("already liked")
	ErrNotLikedYet  = errors.New("not liked yet")
)
