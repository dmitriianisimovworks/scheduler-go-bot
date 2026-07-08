package usecase

import "meeting-bot/internal/repository"

type CancelMeeting struct {
	repo repository.MeetingRepository
}

func NewCancelMeeting(repo repository.MeetingRepository) CancelMeeting {
	return CancelMeeting{repo: repo}
}
