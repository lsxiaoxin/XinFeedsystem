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

func (r *CommentRepository) FindByID(ctx context.Context, id int64) (*entity.Comment, error) {
	var c entity.Comment
	err := r.db.WithContext(ctx).First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &c, err
}

// Create 插入评论，仅写 comments 表，不修改计数字段。
// 计数字段（comment_count / heat）由 Kafka consumer 异步更新。
// 返回 delta=+1 供 service 层发 Kafka 事件。
func (r *CommentRepository) Create(ctx context.Context, c *entity.Comment) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// Delete 软删除评论（仅限评论者本人），不修改计数字段。
// 返回被删评论的 videoID，供 service 层发 Kafka 事件；delta=-1。
func (r *CommentRepository) Delete(ctx context.Context, commentID, userID int64) (videoID int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		videoID = c.VideoID
		return tx.Delete(&c).Error
	})
	return videoID, err
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
