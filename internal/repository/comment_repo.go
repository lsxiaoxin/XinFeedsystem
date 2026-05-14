package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"xinfeedsystem/internal/model/entity"
)

type CommentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Create 在事务内插入评论并更新视频评论数和热度。
func (r *CommentRepository) Create(ctx context.Context, c *entity.Comment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", c.VideoID).
			UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&entity.Video{}).Where("id = ?", c.VideoID).
			UpdateColumn("heat", gorm.Expr("heat + 1")).Error
	})
}

// Delete 软删除评论（仅限评论者本人），并更新视频评论数。
func (r *CommentRepository) Delete(ctx context.Context, commentID, userID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c entity.Comment
		if err := tx.First(&c, commentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommentNotFound
			}
			return err
		}
		if c.UserID != userID {
			return ErrCommentForbidden
		}
		if err := tx.Delete(&c).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.Video{}).Where("id = ? AND comment_count > 0", c.VideoID).
			UpdateColumn("comment_count", gorm.Expr("comment_count - 1")).Error; err != nil {
			return err
		}
		return tx.Model(&entity.Video{}).Where("id = ? AND heat > 0", c.VideoID).
			UpdateColumn("heat", gorm.Expr("heat - 1")).Error
	})
}

// ListByVideoID 查询视频评论，游标分页，按时间倒序。
func (r *CommentRepository) ListByVideoID(ctx context.Context, videoID, cursorTime, cursorID int64, limit int) ([]*entity.Comment, error) {
	q := r.db.WithContext(ctx).
		Where("video_id = ?", videoID).
		Order("created_at DESC, id DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("created_at < ? OR (created_at = ? AND id < ?)", cursorTime, cursorTime, cursorID)
	}

	var list []*entity.Comment
	return list, q.Find(&list).Error
}

var (
	ErrCommentNotFound  = errors.New("comment not found")
	ErrCommentForbidden = errors.New("no permission to delete this comment")
)
