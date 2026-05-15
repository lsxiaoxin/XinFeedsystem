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

// Toggle 在事务内完成点赞 / 取消点赞，仅操作 video_likes 表，不修改计数字段。
// 返回 delta：+1 表示新增点赞，-1 表示取消点赞，0 表示状态未变（幂等）。
// 计数字段（like_count / heat）由 Kafka consumer 异步更新。
func (r *LikeRepository) Toggle(ctx context.Context, userID, videoID int64, action int) (delta int8, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var like entity.VideoLike
		findErr := tx.Unscoped().
			Where("user_id = ? AND video_id = ?", userID, videoID).
			First(&like).Error

		switch action {
		case 1: // 点赞
			switch {
			case errors.Is(findErr, gorm.ErrRecordNotFound):
				like = entity.VideoLike{
					ID:      snowflake.NewID(),
					UserID:  userID,
					VideoID: videoID,
				}
				if err := tx.Create(&like).Error; err != nil {
					return err
				}
				delta = +1
			case findErr == nil && like.DeletedAt.Valid:
				if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
					return err
				}
				delta = +1
			case findErr == nil && !like.DeletedAt.Valid:
				delta = 0 // 已是点赞状态，幂等
			default:
				return findErr
			}
		case 2: // 取消点赞
			switch {
			case errors.Is(findErr, gorm.ErrRecordNotFound):
				delta = 0 // 未点赞，幂等
			case findErr == nil && !like.DeletedAt.Valid:
				now := time.Now()
				if err := tx.Unscoped().Model(&like).Update("deleted_at", now).Error; err != nil {
					return err
				}
				delta = -1
			case findErr == nil && like.DeletedAt.Valid:
				delta = 0 // 已取消，幂等
			default:
				return findErr
			}
		default:
			return ErrInvalidLikeAction
		}
		return nil
	})
	return delta, err
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
	ErrAlreadyLiked    = errors.New("already liked")
	ErrNotLikedYet     = errors.New("not liked yet")
	ErrInvalidLikeAction = errors.New("invalid like action")
)
