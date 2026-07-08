package usecase

import (
	"context"

	"meeting-bot/internal/repository"
)

type CancelMeeting struct {
	repo repository.MeetingRepository
}

func NewCancelMeeting(repo repository.MeetingRepository) CancelMeeting {
	return CancelMeeting{repo: repo}
}

func (uc CancelMeeting) Execute(ctx context.Context, meetingID int64, requesterID int64) error {
	return uc.repo.Cancel(ctx, meetingID, requesterID)
}
