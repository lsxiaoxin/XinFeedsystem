package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/pkg/snowflake"
)

type FollowRepository struct {
	db *gorm.DB
}

func NewFollowRepository(db *gorm.DB) *FollowRepository {
	return &FollowRepository{db: db}
}

// Follow 关注：事务内 upsert follows + 更新双方计数。
func (r *FollowRepository) Follow(ctx context.Context, followerID, followeeID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var f entity.Follow
		err := tx.Unscoped().
			Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
			First(&f).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			f = entity.Follow{
				ID:         snowflake.NewID(),
				FollowerID: followerID,
				FolloweeID: followeeID,
			}
			if err := tx.Create(&f).Error; err != nil {
				return err
			}
		case err == nil && f.DeletedAt.Valid:
			if err := tx.Unscoped().Model(&f).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		case err == nil && !f.DeletedAt.Valid:
			return ErrAlreadyFollowed
		default:
			return err
		}

		// 关注方 follow_count +1
		if err := tx.Model(&entity.User{}).Where("id = ?", followerID).
			UpdateColumn("follow_count", gorm.Expr("follow_count + 1")).Error; err != nil {
			return err
		}
		// 被关注方 follower_count +1
		return tx.Model(&entity.User{}).Where("id = ?", followeeID).
			UpdateColumn("follower_count", gorm.Expr("follower_count + 1")).Error
	})
}

// Unfollow 取关：事务内软删除 + 更新双方计数。
func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followeeID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var f entity.Follow
		if err := tx.Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
			First(&f).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFollowedYet
			}
			return err
		}

		now := time.Now()
		if err := tx.Unscoped().Model(&f).Update("deleted_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.User{}).Where("id = ? AND follow_count > 0", followerID).
			UpdateColumn("follow_count", gorm.Expr("follow_count - 1")).Error; err != nil {
			return err
		}
		return tx.Model(&entity.User{}).Where("id = ? AND follower_count > 0", followeeID).
			UpdateColumn("follower_count", gorm.Expr("follower_count - 1")).Error
	})
}

// ListFollowing 查询 followerID 关注了哪些人（JOIN users），游标分页。
func (r *FollowRepository) ListFollowing(ctx context.Context, followerID, cursorTime, cursorID int64, limit int) ([]*entity.User, [2]int64, error) {
	return r.listWithJoin(ctx, "f.follower_id = ?", "f.followee_id", followerID, cursorTime, cursorID, limit)
}

// ListFollower 查询谁关注了 followeeID（JOIN users），游标分页。
func (r *FollowRepository) ListFollower(ctx context.Context, followeeID, cursorTime, cursorID int64, limit int) ([]*entity.User, [2]int64, error) {
	return r.listWithJoin(ctx, "f.followee_id = ?", "f.follower_id", followeeID, cursorTime, cursorID, limit)
}

// listWithJoin 通用 follow 列表查询，返回 User 列表 + 最后一条的 [created_at, user_id]。
func (r *FollowRepository) listWithJoin(ctx context.Context, whereClause, userIDCol string, id, cursorTime, cursorID int64, limit int) ([]*entity.User, [2]int64, error) {
	q := r.db.WithContext(ctx).
		Table("follows f").
		Select("u.*, f.created_at AS follow_time, "+userIDCol+" AS fuid").
		Joins("JOIN users u ON u.id = "+userIDCol+" AND u.deleted_at IS NULL").
		Where(whereClause+" AND f.deleted_at IS NULL", id).
		Order("f.created_at DESC, "+userIDCol+" DESC").
		Limit(limit)

	if cursorTime > 0 {
		q = q.Where("f.created_at < ? OR (f.created_at = ? AND "+userIDCol+" < ?)",
			cursorTime, cursorTime, cursorID)
	}

	// 用辅助结构体接收额外字段
	type row struct {
		entity.User
		FollowTime int64 `gorm:"column:follow_time"`
		Fuid       int64 `gorm:"column:fuid"`
	}
	var rows []row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, [2]int64{}, err
	}

	users := make([]*entity.User, len(rows))
	var last [2]int64
	for i, r := range rows {
		u := r.User
		users[i] = &u
		if i == len(rows)-1 {
			last = [2]int64{r.FollowTime, r.Fuid}
		}
	}
	return users, last, nil
}

var (
	ErrAlreadyFollowed = errors.New("already followed")
	ErrNotFollowedYet  = errors.New("not followed yet")
)
