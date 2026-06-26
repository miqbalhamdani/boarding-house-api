package service

import (
	"context"
	"testing"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// stubRepo is an in-memory UserRepository used to test the service layer
// in isolation from the database.
type stubRepo struct {
	lastLimit  int
	lastOffset int
}

func (s *stubRepo) Create(_ context.Context, in model.CreateUserInput) (*model.User, error) {
	return &model.User{ID: 1, Name: in.Name, Email: in.Email}, nil
}

func (s *stubRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	return &model.User{ID: id}, nil
}

func (s *stubRepo) List(_ context.Context, limit, offset int) ([]model.User, error) {
	s.lastLimit = limit
	s.lastOffset = offset
	return []model.User{}, nil
}

func TestUserService_List_Pagination(t *testing.T) {
	tests := []struct {
		name           string
		page, pageSize int
		wantLimit      int
		wantOffset     int
	}{
		{"defaults applied", 0, 0, 20, 0},
		{"second page", 2, 10, 10, 10},
		{"oversized clamped", 1, 500, 20, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubRepo{}
			svc := NewUserService(repo)

			if _, err := svc.List(context.Background(), tc.page, tc.pageSize); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.lastLimit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", repo.lastLimit, tc.wantLimit)
			}
			if repo.lastOffset != tc.wantOffset {
				t.Errorf("offset = %d, want %d", repo.lastOffset, tc.wantOffset)
			}
		})
	}
}
