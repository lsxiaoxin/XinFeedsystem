package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"xinfeedsystem/internal/model/entity"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (*entity.User, error) {
	var user entity.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

// FindByIDs 批量查询用户，返回 id→User 映射（避免 N+1）。
func (r *UserRepository) FindByIDs(ctx context.Context, ids []int64) (map[int64]*entity.User, error) {
	if len(ids) == 0 {
		return map[int64]*entity.User{}, nil
	}
	var users []*entity.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]*entity.User, len(users))
	for _, u := range users {
		m[u.ID] = u
	}
	return m, nil
}

// SaveToken 登录时写入 token（覆盖旧值）。
func (r *UserRepository) SaveToken(ctx context.Context, userID int64, token string) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ?", userID).
		UpdateColumn("token", token).Error
}

// ClearToken 登出时清空 token。
func (r *UserRepository) ClearToken(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ?", userID).
		UpdateColumn("token", "").Error
}

// FindTokenByUserID 查询用户当前存储的 token。
func (r *UserRepository) FindTokenByUserID(ctx context.Context, userID int64) (string, error) {
	var u entity.User
	err := r.db.WithContext(ctx).Select("token").First(&u, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return u.Token, err
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
	var user entity.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}
