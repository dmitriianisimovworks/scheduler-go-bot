package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"meeting-bot/internal/domain"
	"meeting-bot/internal/usecase"

	tele "gopkg.in/telebot.v3"
)

const (
	draftStepTitle        = "title"
	draftStepDate         = "date"
	draftStepTime         = "time"
	draftStepDuration     = "duration"
	draftStepParticipants = "participants"
)

const (
	workdayStartHour = 9
	workdayEndHour   = 19
	timeSlotStep     = 30 * time.Minute
)

type meetingDraft struct {
	step     string
	title    string
	date     time.Time
	startsAt time.Time
	duration time.Duration
}

func (b *Bot) setDraft(telegramID int64, draft *meetingDraft) {
	b.draftsMu.Lock()
	defer b.draftsMu.Unlock()
	b.drafts[telegramID] = draft
}

func (b *Bot) getDraft(telegramID int64) (*meetingDraft, bool) {
	b.draftsMu.Lock()
	defer b.draftsMu.Unlock()
	draft, ok := b.drafts[telegramID]
	return draft, ok
}

func (b *Bot) clearDraft(telegramID int64) {
	b.draftsMu.Lock()
	defer b.draftsMu.Unlock()
	delete(b.drafts, telegramID)
}

func (b *Bot) userLocation(user domain.User) *time.Location {
	loc, err := timeLocation(user.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func (b *Bot) handleCreateMeetingStart(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	if !user.IsRegistered() {
		return b.sendMainMenu(c, user)
	}

	b.setDraft(user.TelegramID, &meetingDraft{step: draftStepTitle})
	return c.Send("➕ Название встречи? Напишите коротко, например: Синк по продукту.\n\nЧтобы отменить создание, напишите «отмена».")
}

func (b *Bot) handleMeetingDraftText(c tele.Context, user domain.User, draft *meetingDraft, text string) error {
	if strings.EqualFold(text, "отмена") || strings.EqualFold(text, "cancel") {
		b.clearDraft(user.TelegramID)
		return c.Send("Создание встречи отменено.", b.mainMenuKeyboard)
	}

	loc := b.userLocation(user)

	switch draft.step {
	case draftStepTitle:
		draft.title = text
		draft.step = draftStepDate
		return b.askMeetingDate(c)
	case draftStepDate:
		date, err := parseMeetingDate(text, loc, b.clock.Now().In(loc))
		if err != nil {
			return c.Send("Не понял дату. Выберите кнопку или напишите в формате ДД.ММ или ДД.ММ.ГГГГ.")
		}
		draft.date = date
		draft.step = draftStepTime
		return b.askMeetingTime(c)
	case draftStepTime:
		hour, minute, err := parseMeetingTime(text)
		if err != nil {
			return c.Send("Не понял время. Формат ЧЧ:ММ, например 14:30.")
		}
		draft.startsAt = time.Date(draft.date.Year(), draft.date.Month(), draft.date.Day(), hour, minute, 0, 0, loc)
		draft.step = draftStepDuration
		return b.askMeetingDuration(c)
	case draftStepDuration:
		return b.askMeetingDuration(c)
	case draftStepParticipants:
		return b.finishMeetingDraft(c, user, draft, text)
	default:
		b.clearDraft(user.TelegramID)
		return b.sendMainMenu(c, user)
	}
}

func (b *Bot) askMeetingDate(c tele.Context) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnDateToday, b.btnDateTomorrow),
		markup.Row(b.btnDateOther),
	)
	return c.Send("📆 На какой день? Выберите кнопку или напишите дату (ДД.ММ или ДД.ММ.ГГГГ).", markup)
}

func (b *Bot) askMeetingDuration(c tele.Context) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnDur15, b.btnDur30),
		markup.Row(b.btnDur60, b.btnDur90),
	)
	return c.Send("⏱ Сколько по времени?", markup)
}

func (b *Bot) askMeetingTime(c tele.Context) error {
	markup := &tele.ReplyMarkup{}
	slots := timeSlots()

	rows := make([]tele.Row, 0, len(slots)/3+2)
	for i := 0; i < len(slots); i += 3 {
		end := min(i+3, len(slots))
		var buttons []tele.Btn
		for _, slot := range slots[i:end] {
			buttons = append(buttons, markup.Data(slot, b.btnTimePick.Unique, slot))
		}
		rows = append(rows, markup.Row(buttons...))
	}
	rows = append(rows, markup.Row(b.btnTimeOther))
	markup.Inline(rows...)

	return c.Send("🕐 Время начала?", markup)
}

func timeSlots() []string {
	var slots []string
	for h := workdayStartHour; h <= workdayEndHour; h++ {
		for m := 0; m < 60; m += int(timeSlotStep / time.Minute) {
			if h == workdayEndHour && m > 0 {
				break
			}
			slots = append(slots, fmt.Sprintf("%02d:%02d", h, m))
		}
	}
	return slots
}

func (b *Bot) handleMeetingDateToday(c tele.Context) error {
	defer c.Respond()
	return b.setMeetingDate(c, "сегодня")
}

func (b *Bot) handleMeetingDateTomorrow(c tele.Context) error {
	defer c.Respond()
	return b.setMeetingDate(c, "завтра")
}

func (b *Bot) setMeetingDate(c tele.Context, text string) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepDate {
		return nil
	}
	loc := b.userLocation(user)
	date, err := parseMeetingDate(text, loc, b.clock.Now().In(loc))
	if err != nil {
		return err
	}
	draft.date = date
	draft.step = draftStepTime
	return b.askMeetingTime(c)
}

func (b *Bot) handleMeetingDateOther(c tele.Context) error {
	defer c.Respond()
	return c.Send("Напишите дату в формате ДД.ММ или ДД.ММ.ГГГГ.")
}

func (b *Bot) handleMeetingTimePick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepTime {
		return nil
	}
	hour, minute, err := parseMeetingTime(c.Data())
	if err != nil {
		return c.Send("Не понял время.")
	}
	loc := b.userLocation(user)
	draft.startsAt = time.Date(draft.date.Year(), draft.date.Month(), draft.date.Day(), hour, minute, 0, 0, loc)
	draft.step = draftStepDuration
	return b.askMeetingDuration(c)
}

func (b *Bot) handleMeetingTimeOther(c tele.Context) error {
	defer c.Respond()
	return c.Send("Напишите время в формате ЧЧ:ММ, например 14:30.")
}

func (b *Bot) handleMeetingDuration(duration time.Duration) tele.HandlerFunc {
	return func(c tele.Context) error {
		defer c.Respond()
		user, err := b.currentUser(c)
		if err != nil {
			return err
		}
		draft, ok := b.getDraft(user.TelegramID)
		if !ok || draft.step != draftStepDuration {
			return nil
		}
		draft.duration = duration
		draft.step = draftStepParticipants
		return b.askMeetingParticipants(c)
	}
}

func (b *Bot) askMeetingParticipants(c tele.Context) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnNoParticipant))
	return c.Send("👥 Кто участвует? Напишите Telegram-юзернеймы через запятую (без @), например: ivan, maria.\n\nИли нажмите кнопку, если участников кроме вас нет.", markup)
}

func (b *Bot) handleNoParticipants(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepParticipants {
		return nil
	}
	return b.finishMeetingDraft(c, user, draft, "-")
}

func (b *Bot) finishMeetingDraft(c tele.Context, user domain.User, draft *meetingDraft, participantsText string) error {
	participantIDs, unknown, err := b.resolveParticipants(user, participantsText)
	if err != nil {
		return err
	}

	endsAt := draft.startsAt.Add(draft.duration)

	meeting, err := b.createMeeting.Execute(context.Background(), usecase.CreateMeetingInput{
		Title:          draft.title,
		CreatorID:      user.ID,
		ParticipantIDs: participantIDs,
		StartsAt:       draft.startsAt,
		EndsAt:         endsAt,
	})
	if err != nil {
		if errors.Is(err, usecase.ErrConflict) {
			draft.step = draftStepTime
			return c.Send("⚠️ В это время уже есть встреча у вас или у участника. Напишите другое время (ЧЧ:ММ).")
		}
		return err
	}

	b.clearDraft(user.TelegramID)

	loc := b.userLocation(user)
	summary := fmt.Sprintf(
		"✅ Встреча создана.\n\n%s\n%s – %s",
		meeting.Title,
		meeting.StartsAt.In(loc).Format("02.01.2006 15:04"),
		meeting.EndsAt.In(loc).Format("15:04"),
	)
	if len(unknown) > 0 {
		summary += fmt.Sprintf("\n\n⚠️ Не найдены пользователи: %s", strings.Join(unknown, ", "))
	}

	if err := c.Send(summary); err != nil {
		return err
	}
	return b.sendMainMenu(c, user)
}

func (b *Bot) resolveParticipants(creator domain.User, text string) ([]int64, []string, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "-" {
		return nil, nil, nil
	}

	var participantIDs []int64
	var unknown []string
	seen := map[int64]bool{}

	for _, raw := range strings.Split(text, ",") {
		username := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		if username == "" {
			continue
		}
		participant, err := b.users.GetByUsername(context.Background(), username)
		if err != nil {
			unknown = append(unknown, username)
			continue
		}
		if participant.TelegramID == creator.TelegramID || seen[participant.ID] {
			continue
		}
		seen[participant.ID] = true
		participantIDs = append(participantIDs, participant.ID)
	}

	return participantIDs, unknown, nil
}

func parseMeetingDate(text string, loc *time.Location, now time.Time) (time.Time, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	today := startOfDay(now)

	switch text {
	case "сегодня", "today":
		return today, nil
	case "завтра", "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}

	if parsed, err := time.ParseInLocation("02.01.2006", text, loc); err == nil {
		return startOfDay(parsed), nil
	}

	if parsed, err := time.ParseInLocation("02.01", text, loc); err == nil {
		date := time.Date(today.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
		if date.Before(today) {
			date = date.AddDate(1, 0, 0)
		}
		return date, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse date: %s", text)
}

func parseMeetingTime(text string) (hour int, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad time format")
	}
	hour, errH := strconv.Atoi(parts[0])
	minute, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("bad time value")
	}
	return hour, minute, nil
}

func (b *Bot) handleCancelMeetingStart(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	if !user.IsRegistered() {
		return b.sendMainMenu(c, user)
	}

	loc := b.userLocation(user)
	meetings, err := b.listUpcoming.Execute(context.Background(), user.ID, b.clock.Now())
	if err != nil {
		return err
	}
	if len(meetings) == 0 {
		return c.Send("У вас нет предстоящих встреч для отмены.", b.mainMenuKeyboard)
	}

	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(meetings))
	for _, meeting := range meetings {
		label := fmt.Sprintf(
			"%s — %s",
			meeting.StartsAt.In(loc).Format("02.01 15:04"),
			meeting.Title,
		)
		btn := markup.Data(label, b.btnCancelPick.Unique, strconv.FormatInt(meeting.ID, 10))
		rows = append(rows, markup.Row(btn))
	}
	markup.Inline(rows...)

	return c.Send("🗑 Выберите встречу для отмены:", markup)
}

func (b *Bot) handleCancelMeetingPick(c tele.Context) error {
	defer c.Respond()

	user, err := b.currentUser(c)
	if err != nil {
		return err
	}

	meetingID, err := strconv.ParseInt(c.Data(), 10, 64)
	if err != nil {
		return c.Edit("Не понял, какую встречу отменить.")
	}

	if err := b.cancelMeeting.Execute(context.Background(), meetingID, user.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Edit("Эту встречу уже нельзя отменить: не найдена или вы не создатель.")
		}
		return err
	}

	return c.Edit("✅ Встреча отменена.")
}

func formatMeetingsList(header string, meetings []domain.Meeting, loc *time.Location, emptyText string) string {
	if len(meetings) == 0 {
		return fmt.Sprintf("%s\n\n%s", header, emptyText)
	}

	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].StartsAt.Before(meetings[j].StartsAt)
	})

	var lines []string
	for _, meeting := range meetings {
		line := fmt.Sprintf(
			"%s–%s — %s",
			meeting.StartsAt.In(loc).Format("02.01 15:04"),
			meeting.EndsAt.In(loc).Format("15:04"),
			meeting.Title,
		)
		if participants := len(meeting.ParticipantIDs); participants > 0 {
			line += fmt.Sprintf(" (участников: %d)", participants+1)
		}
		lines = append(lines, line)
	}

	return fmt.Sprintf("%s\n\n%s", header, strings.Join(lines, "\n"))
}
