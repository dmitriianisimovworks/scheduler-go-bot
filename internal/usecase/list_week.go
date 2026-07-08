package usecase

import "meeting-bot/internal/repository"

type ListWeek struct {
	repo repository.MeetingRepository
}

func NewListWeek(repo repository.MeetingRepository) ListWeek {
	return ListWeek{repo: repo}
}
