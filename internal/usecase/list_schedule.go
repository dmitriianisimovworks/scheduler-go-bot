package usecase

import (
	"context"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/repository"
)

type ListSchedule struct {
	repo repository.MeetingRepository
}

func NewListSchedule(repo repository.MeetingRepository) ListSchedule {
	return ListSchedule{repo: repo}
}

func (uc ListSchedule) Execute(ctx context.Context, from time.Time, to time.Time) ([]domain.Meeting, error) {
	return uc.repo.ListByDateRange(ctx, from, to)
}
