package notifier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/platform/clock"
	"meeting-bot/internal/platform/logger"
	"meeting-bot/internal/repository"
)

// Sender delivers a reminder message to a user outside of any Telegram
// update (a scheduled push, not a reply).
type Sender interface {
	SendReminder(telegramID int64, text string) error
}

type threshold struct {
	Key    string
	Before time.Duration
	Label  string
}

var thresholds = []threshold{
	{Key: "30m", Before: 30 * time.Minute, Label: "начнётся через 30 минут"},
	{Key: "15m", Before: 15 * time.Minute, Label: "начнётся через 15 минут"},
	{Key: "5m", Before: 5 * time.Minute, Label: "начнётся через 5 минут"},
	{Key: "start", Before: 0, Label: "начинается"},
}

// maxStaleness bounds how far past a threshold's ideal fire time we still
// consider it worth sending — protects against a downtime/restart causing a
// burst of stale reminders for meetings that already started long ago.
const maxStaleness = 3 * time.Minute

const tickInterval = time.Minute

type Notifier struct {
	meetings repository.MeetingRepository
	users    repository.UserRepository
	clock    clock.Clock
	sender   Sender
	logger   *logger.Logger
}

func New(meetings repository.MeetingRepository, users repository.UserRepository, clk clock.Clock, sender Sender, log *logger.Logger) *Notifier {
	return &Notifier{meetings: meetings, users: users, clock: clk, sender: sender, logger: log}
}

func (n *Notifier) Run(ctx context.Context) error {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			n.tick(ctx)
		}
	}
}

func (n *Notifier) tick(ctx context.Context) {
	now := n.clock.Now()
	from := now.Add(-maxStaleness)
	to := now.Add(30*time.Minute + tickInterval)

	meetings, err := n.meetings.ListByDateRange(ctx, from, to)
	if err != nil {
		n.logger.Printf("notifier: list meetings: %v", err)
		return
	}
	if len(meetings) == 0 {
		return
	}

	users, err := n.users.ListRegistered(ctx)
	if err != nil {
		n.logger.Printf("notifier: list users: %v", err)
		return
	}
	byID := make(map[int64]domain.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	for _, meeting := range meetings {
		for _, th := range thresholds {
			target := meeting.StartsAt.Add(-th.Before)
			remaining := now.Sub(target)
			if remaining < 0 || remaining > maxStaleness {
				continue
			}

			sent, err := n.meetings.IsReminderSent(ctx, meeting.ID, th.Key)
			if err != nil {
				n.logger.Printf("notifier: check sent for meeting %d/%s: %v", meeting.ID, th.Key, err)
				continue
			}
			if sent {
				continue
			}

			n.notify(meeting, th, byID)

			if err := n.meetings.MarkReminderSent(ctx, meeting.ID, th.Key); err != nil {
				n.logger.Printf("notifier: mark sent for meeting %d/%s: %v", meeting.ID, th.Key, err)
			}
		}
	}
}

func (n *Notifier) notify(meeting domain.Meeting, th threshold, byID map[int64]domain.User) {
	organizer := displayName(byID[meeting.CreatorID])

	participantNames := make([]string, 0, len(meeting.ParticipantIDs))
	for _, id := range meeting.ParticipantIDs {
		if u, ok := byID[id]; ok {
			participantNames = append(participantNames, displayName(u))
		}
	}

	recipientIDs := append([]int64{meeting.CreatorID}, meeting.ParticipantIDs...)
	seen := make(map[int64]bool, len(recipientIDs))
	for _, id := range recipientIDs {
		if seen[id] {
			continue
		}
		seen[id] = true

		user, ok := byID[id]
		if !ok || user.TelegramID == 0 {
			continue
		}

		loc, err := time.LoadLocation(user.Timezone)
		if err != nil {
			loc = time.UTC
		}

		text := formatReminder(meeting, th, loc, organizer, participantNames)
		if err := n.sender.SendReminder(user.TelegramID, text); err != nil {
			n.logger.Printf("notifier: send to telegram id %d: %v", user.TelegramID, err)
		}
	}
}

func formatReminder(meeting domain.Meeting, th threshold, loc *time.Location, organizer string, participants []string) string {
	header := "⏰ Встреча " + th.Label
	if th.Key == "start" {
		header = "▶️ Встреча " + th.Label
	}

	text := fmt.Sprintf(
		"%s\n\n🗓 %s\n%s–%s\n\n👤 Организатор: %s\n👥 Участники: %s",
		header,
		meeting.Title,
		meeting.StartsAt.In(loc).Format("15:04"),
		meeting.EndsAt.In(loc).Format("15:04"),
		organizer,
		joinNamesOrDash(participants),
	)
	if meeting.MeetLink != "" {
		text += fmt.Sprintf("\n\n🔗 Google Meet: %s", meeting.MeetLink)
	}
	return text
}

func displayName(user domain.User) string {
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		return "не указано"
	}
	return name
}

func joinNamesOrDash(names []string) string {
	if len(names) == 0 {
		return "только организатор"
	}
	return strings.Join(names, ", ")
}
