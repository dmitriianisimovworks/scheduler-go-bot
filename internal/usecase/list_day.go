package usecase

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

type ListDay struct {
	repo repository.MeetingRepository
}

func NewListDay(repo repository.MeetingRepository) ListDay {
	return ListDay{repo: repo}
}

func (uc ListDay) Execute(ctx context.Context, day time.Time) ([]domain.Meeting, error) {
	return uc.repo.ListByDay(ctx, day)
}
