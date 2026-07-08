package usecase

import (
	"context"

	"meeting-bot/internal/repository"
)

type CancelMeeting struct {
	repo     repository.MeetingRepository
	calendar CalendarClient
}

func NewCancelMeeting(repo repository.MeetingRepository, calendar CalendarClient) CancelMeeting {
	return CancelMeeting{repo: repo, calendar: calendar}
}

func (uc CancelMeeting) Execute(ctx context.Context, meetingID int64, requesterID int64) error {
	meeting, err := uc.repo.Cancel(ctx, meetingID, requesterID)
	if err != nil {
		return err
	}

	if uc.calendar != nil && uc.calendar.Enabled() && meeting.GoogleEventID != "" {
		_ = uc.calendar.DeleteEvent(ctx, meeting.GoogleEventID)
	}

	return nil
}
