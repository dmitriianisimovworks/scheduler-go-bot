package postgres

import (
	"context"
	"time"

	"meeting-bot/internal/config"
	"meeting-bot/internal/domain"
)

type Repository struct {
	config config.Config
}

func New(cfg config.Config) *Repository {
	return &Repository{config: cfg}
}

func (r *Repository) Upsert(_ context.Context, user domain.User) (domain.User, error) {
	return user, nil
}

func (r *Repository) Create(_ context.Context, meeting domain.Meeting) (domain.Meeting, error) {
	return meeting, nil
}

func (r *Repository) ListByDay(_ context.Context, _ time.Time) ([]domain.Meeting, error) {
	return nil, nil
}

func (r *Repository) ListByWeek(_ context.Context, _ time.Time) ([]domain.Meeting, error) {
	return nil, nil
}

func (r *Repository) Cancel(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (r *Repository) HasConflict(_ context.Context, _ []int64, _ time.Time, _ time.Time) (bool, error) {
	return false, nil
}
