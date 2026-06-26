package service

import (
	"context"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// UserService holds business logic for users.
type UserService interface {
	Create(ctx context.Context, in model.CreateUserInput) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
	List(ctx context.Context, page, pageSize int) ([]model.User, error)
}

type userService struct {
	repo repository.UserRepository
}

// NewUserService wires a UserService to its repository.
func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) Create(ctx context.Context, in model.CreateUserInput) (*model.User, error) {
	return s.repo.Create(ctx, in)
}

func (s *userService) GetByID(ctx context.Context, id int64) (*model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *userService) List(ctx context.Context, page, pageSize int) ([]model.User, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, pageSize, offset)
}
