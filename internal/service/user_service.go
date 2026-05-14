package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/cache"
	"xinfeedsystem/pkg/hash"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/snowflake"
)

type UserService struct {
	userRepo *repository.UserRepository
	rdb      *redis.Client
}

func NewUserService(userRepo *repository.UserRepository, rdb *redis.Client) *UserService {
	return &UserService{userRepo: userRepo, rdb: rdb}
}

func userInfoKey(id int64) string { return fmt.Sprintf("user:info:%d", id) }

func (s *UserService) Register(ctx context.Context, req *dto.RegisterRequest) error {
	existing, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return err
	}
	if existing != nil {
		return errcode.New(errcode.UserAlreadyExists)
	}

	hashed, err := hash.Password(req.Password)
	if err != nil {
		return err
	}

	user := &entity.User{
		ID:           snowflake.NewID(),
		Username:     req.Username,
		PasswordHash: hashed,
		Nickname:     req.Nickname,
	}
	return s.userRepo.Create(ctx, user)
}

func (s *UserService) Login(ctx context.Context, req *dto.LoginRequest) (string, *entity.User, error) {
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return "", nil, err
	}
	if user == nil {
		return "", nil, errcode.New(errcode.UserNotFound)
	}
	if !hash.CheckPassword(req.Password, user.PasswordHash) {
		return "", nil, errcode.New(errcode.WrongPassword)
	}

	token, err := pkgjwt.Sign(user.ID)
	if err != nil {
		return "", nil, err
	}
	if err := s.userRepo.SaveToken(ctx, user.ID, token); err != nil {
		return "", nil, err
	}
	return token, user, nil
}

func (s *UserService) Logout(ctx context.Context, userID int64) error {
	return s.userRepo.ClearToken(ctx, userID)
}

func (s *UserService) GetUserInfo(ctx context.Context, id int64) (*entity.User, error) {
	key := userInfoKey(id)

	var u entity.User
	hit, isNil, err := cache.GetJSON(ctx, s.rdb, key, &u)
	if err == nil && hit {
		if isNil {
			return nil, errcode.New(errcode.UserNotFound)
		}
		return &u, nil
	}

	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		_ = cache.SetNil(ctx, s.rdb, key, 30*time.Second)
		return nil, errcode.New(errcode.UserNotFound)
	}
	_ = cache.SetJSON(ctx, s.rdb, key, user, cache.RandomizedTTL(10*time.Minute, 30*time.Second))
	return user, nil
}
