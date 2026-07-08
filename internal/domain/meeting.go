package domain

import "time"

type Meeting struct {
	ID             int64
	Title          string
	CreatorID      int64
	ParticipantIDs []int64
	StartsAt       time.Time
	EndsAt         time.Time
	GoogleEventID  string
	MeetLink       string
}

func (m Meeting) Overlaps(other Meeting) bool {
	return m.StartsAt.Before(other.EndsAt) && m.EndsAt.After(other.StartsAt)
}
