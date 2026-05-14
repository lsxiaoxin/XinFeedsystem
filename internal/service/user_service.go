package service

import (
	"context"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/hash"
	pkgjwt "xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/snowflake"
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService(userRepo *repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

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
	return token, user, nil
}

func (s *UserService) GetUserInfo(ctx context.Context, id int64) (*entity.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errcode.New(errcode.UserNotFound)
	}
	return user, nil
}
