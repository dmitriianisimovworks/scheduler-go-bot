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
	maxMonthsAhead   = 12
)

var ruMonths = [...]string{
	"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
}

type meetingDraft struct {
	step          string
	title         string
	date          time.Time
	calendarMonth time.Time
	startsAt      time.Time
	duration      time.Duration
	selected      map[int64]bool
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

func startOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

func monthLabel(t time.Time) string {
	return fmt.Sprintf("%s %d", ruMonths[t.Month()-1], t.Year())
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
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnDraftCancel))
	return c.Send("➕ Название встречи? Напишите коротко, например: Синк по продукту.", markup)
}

func (b *Bot) handleDraftCancel(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	b.clearDraft(user.TelegramID)
	return c.Send("Создание встречи отменено.", b.mainMenuKeyboard)
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
		draft.calendarMonth = startOfMonth(b.clock.Now().In(loc))
		return b.askMeetingDate(c, user, draft)
	case draftStepDate:
		date, err := parseMeetingDate(text, loc, b.clock.Now().In(loc))
		if err != nil {
			return c.Send("Не понял дату. Выберите день в календаре или напишите в формате ДД.ММ или ДД.ММ.ГГГГ.")
		}
		if date.Before(startOfDay(b.clock.Now().In(loc))) {
			return c.Send("Эта дата уже прошла. Напишите дату в будущем.")
		}
		draft.date = date
		draft.step = draftStepTime
		return b.askMeetingTime(c, user, draft)
	case draftStepTime:
		hour, minute, err := parseMeetingTime(text)
		if err != nil {
			return c.Send("Не понял время. Формат ЧЧ:ММ, например 14:30.")
		}
		startsAt := time.Date(draft.date.Year(), draft.date.Month(), draft.date.Day(), hour, minute, 0, 0, loc)
		if !startsAt.After(b.clock.Now()) {
			return c.Send("⚠️ Это время уже прошло. Напишите время в будущем.")
		}
		draft.startsAt = startsAt
		draft.step = draftStepDuration
		return b.askMeetingDuration(c)
	case draftStepDuration:
		return b.askMeetingDuration(c)
	case draftStepParticipants:
		return b.askMeetingParticipants(c, user, draft)
	default:
		b.clearDraft(user.TelegramID)
		return b.sendMainMenu(c, user)
	}
}

func (b *Bot) askMeetingDate(c tele.Context, user domain.User, draft *meetingDraft) error {
	loc := b.userLocation(user)
	now := b.clock.Now().In(loc)
	if draft.calendarMonth.IsZero() {
		draft.calendarMonth = startOfMonth(now)
	}

	markup := &tele.ReplyMarkup{}
	rows := []tele.Row{markup.Row(b.btnDateToday, b.btnDateTomorrow)}

	today := startOfDay(now)
	for _, week := range calendarWeeks(draft.calendarMonth) {
		var buttons []tele.Btn
		for _, day := range week {
			if day == 0 {
				continue
			}
			date := time.Date(draft.calendarMonth.Year(), draft.calendarMonth.Month(), day, 0, 0, 0, 0, loc)
			if date.Before(today) {
				continue
			}
			buttons = append(buttons, markup.Data(strconv.Itoa(day), b.btnDatePick.Unique, date.Format("2006-01-02")))
		}
		if len(buttons) > 0 {
			rows = append(rows, markup.Row(buttons...))
		}
	}

	var navButtons []tele.Btn
	if startOfMonth(draft.calendarMonth).After(startOfMonth(now)) {
		navButtons = append(navButtons, b.btnMonthPrev)
	}
	if startOfMonth(draft.calendarMonth).Before(startOfMonth(now).AddDate(0, maxMonthsAhead, 0)) {
		navButtons = append(navButtons, b.btnMonthNext)
	}
	if len(navButtons) > 0 {
		rows = append(rows, markup.Row(navButtons...))
	}
	rows = append(rows, markup.Row(b.btnDateOther))

	markup.Inline(rows...)

	text := fmt.Sprintf("📆 На какой день?\n\n%s", monthLabel(draft.calendarMonth))
	return c.EditOrSend(text, markup)
}

func calendarWeeks(month time.Time) [][]int {
	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	daysInMonth := first.AddDate(0, 1, -1).Day()
	leading := (int(first.Weekday()) + 6) % 7

	var weeks [][]int
	week := make([]int, 0, 7)
	for i := 0; i < leading; i++ {
		week = append(week, 0)
	}
	for day := 1; day <= daysInMonth; day++ {
		week = append(week, day)
		if len(week) == 7 {
			weeks = append(weeks, week)
			week = make([]int, 0, 7)
		}
	}
	if len(week) > 0 {
		weeks = append(weeks, week)
	}
	return weeks
}

func (b *Bot) askMeetingDuration(c tele.Context) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnDur15, b.btnDur30),
		markup.Row(b.btnDur60, b.btnDur90),
	)
	return c.Send("⏱ Сколько по времени?", markup)
}

func (b *Bot) askMeetingTime(c tele.Context, user domain.User, draft *meetingDraft) error {
	loc := b.userLocation(user)
	now := b.clock.Now().In(loc)
	slots := timeSlots(draft.date, now)

	if len(slots) == 0 {
		draft.step = draftStepDate
		if err := c.Send("На выбранный день рабочих слотов больше не осталось. Выберите другой день."); err != nil {
			return err
		}
		return b.askMeetingDate(c, user, draft)
	}

	markup := &tele.ReplyMarkup{}
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

func timeSlots(date time.Time, now time.Time) []string {
	isToday := startOfDay(date).Equal(startOfDay(now))
	var slots []string
	for h := workdayStartHour; h <= workdayEndHour; h++ {
		for m := 0; m < 60; m += int(timeSlotStep / time.Minute) {
			if h == workdayEndHour && m > 0 {
				break
			}
			if isToday && (h < now.Hour() || (h == now.Hour() && m <= now.Minute())) {
				continue
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
	return b.askMeetingTime(c, user, draft)
}

func (b *Bot) handleMeetingDateOther(c tele.Context) error {
	defer c.Respond()
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnDraftCancel))
	return c.Send("Напишите дату в формате ДД.ММ или ДД.ММ.ГГГГ.", markup)
}

func (b *Bot) handleMeetingDatePick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepDate {
		return nil
	}
	loc := b.userLocation(user)
	parsed, err := time.ParseInLocation("2006-01-02", c.Data(), loc)
	if err != nil {
		return c.RespondText("Не понял дату")
	}
	if parsed.Before(startOfDay(b.clock.Now().In(loc))) {
		return c.RespondText("Эта дата уже прошла")
	}
	draft.date = parsed
	draft.step = draftStepTime
	return b.askMeetingTime(c, user, draft)
}

func (b *Bot) handleMeetingMonthPrev(c tele.Context) error {
	defer c.Respond()
	return b.shiftCalendarMonth(c, -1)
}

func (b *Bot) handleMeetingMonthNext(c tele.Context) error {
	defer c.Respond()
	return b.shiftCalendarMonth(c, 1)
}

func (b *Bot) shiftCalendarMonth(c tele.Context, delta int) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepDate {
		return nil
	}
	loc := b.userLocation(user)
	next := draft.calendarMonth.AddDate(0, delta, 0)
	minMonth := startOfMonth(b.clock.Now().In(loc))
	maxMonth := minMonth.AddDate(0, maxMonthsAhead, 0)
	if next.Before(minMonth) || next.After(maxMonth) {
		return nil
	}
	draft.calendarMonth = next
	return b.askMeetingDate(c, user, draft)
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
	startsAt := time.Date(draft.date.Year(), draft.date.Month(), draft.date.Day(), hour, minute, 0, 0, loc)
	if !startsAt.After(b.clock.Now()) {
		return c.Send("⚠️ Это время уже прошло. Выберите другое.")
	}
	draft.startsAt = startsAt
	draft.step = draftStepDuration
	return b.askMeetingDuration(c)
}

func (b *Bot) handleMeetingTimeOther(c tele.Context) error {
	defer c.Respond()
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnDraftCancel))
	return c.Send("Напишите время в формате ЧЧ:ММ, например 14:30.", markup)
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
		return b.askMeetingParticipants(c, user, draft)
	}
}

func participantLabel(user domain.User) string {
	return fmt.Sprintf("%s: %s; %s", valueOrDash(user.DisplayName), valueOrDash(user.Team), valueOrDash(user.Role))
}

func (b *Bot) askMeetingParticipants(c tele.Context, user domain.User, draft *meetingDraft) error {
	candidates, err := b.users.ListRegistered(context.Background())
	if err != nil {
		return err
	}

	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(candidates)+2)
	for _, candidate := range candidates {
		label := participantLabel(candidate)
		if candidate.TelegramID == user.TelegramID {
			label += " (вы)"
		}
		checkbox := "⬜"
		if draft.selected[candidate.ID] {
			checkbox = "✅"
		}
		btn := markup.Data(checkbox+" "+label, b.btnParticipantToggle.Unique, strconv.FormatInt(candidate.ID, 10))
		rows = append(rows, markup.Row(btn))
	}
	rows = append(rows, markup.Row(b.btnParticipantsDone))
	rows = append(rows, markup.Row(b.btnDraftCancel))
	markup.Inline(rows...)

	return c.EditOrSend("👥 Кто участвует? Отметьте участников и нажмите «Готово».", markup)
}

func (b *Bot) handleParticipantToggle(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepParticipants {
		return nil
	}
	id, err := strconv.ParseInt(c.Data(), 10, 64)
	if err != nil {
		return nil
	}
	if draft.selected == nil {
		draft.selected = make(map[int64]bool)
	}
	draft.selected[id] = !draft.selected[id]
	return b.askMeetingParticipants(c, user, draft)
}

func (b *Bot) handleParticipantsDone(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	draft, ok := b.getDraft(user.TelegramID)
	if !ok || draft.step != draftStepParticipants {
		return nil
	}
	return b.finishMeetingDraft(c, user, draft)
}

func (b *Bot) finishMeetingDraft(c tele.Context, user domain.User, draft *meetingDraft) error {
	candidates, err := b.users.ListRegistered(context.Background())
	if err != nil {
		return err
	}

	var participantIDs []int64
	var labels []string
	for _, candidate := range candidates {
		if candidate.TelegramID == user.TelegramID || !draft.selected[candidate.ID] {
			continue
		}
		participantIDs = append(participantIDs, candidate.ID)
		labels = append(labels, valueOrDash(candidate.DisplayName))
	}

	endsAt := draft.startsAt.Add(draft.duration)

	meeting, err := b.createMeeting.Execute(context.Background(), usecase.CreateMeetingInput{
		Title:          draft.title,
		CreatorID:      user.ID,
		ParticipantIDs: participantIDs,
		StartsAt:       draft.startsAt,
		EndsAt:         endsAt,
		Now:            b.clock.Now(),
	})
	if err != nil {
		if errors.Is(err, usecase.ErrConflict) {
			draft.step = draftStepTime
			if sendErr := c.Send("⚠️ В это время уже есть встреча у вас или у участника. Выберите другое время."); sendErr != nil {
				return sendErr
			}
			return b.askMeetingTime(c, user, draft)
		}
		if errors.Is(err, usecase.ErrPastTime) {
			draft.step = draftStepTime
			if sendErr := c.Send("⚠️ Это время уже прошло. Выберите другое."); sendErr != nil {
				return sendErr
			}
			return b.askMeetingTime(c, user, draft)
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
	if len(labels) > 0 {
		summary += fmt.Sprintf("\n\n👥 Участники: %s", strings.Join(labels, ", "))
	} else {
		summary += "\n\n👥 Участники: только вы"
	}

	if err := c.Send(summary); err != nil {
		return err
	}
	return b.sendMainMenu(c, user)
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

func (b *Bot) renderMyMeetings(c tele.Context, user domain.User) error {
	loc := b.userLocation(user)
	meetings, err := b.listMyMeetings.Execute(context.Background(), user.ID, b.clock.Now())
	if err != nil {
		return err
	}
	if len(meetings) == 0 {
		return c.Send("📋 Предстоящих встреч нет.", b.mainMenuKeyboard)
	}

	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].StartsAt.Before(meetings[j].StartsAt)
	})

	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(meetings))
	for _, meeting := range meetings {
		label := fmt.Sprintf(
			"%s–%s — %s",
			meeting.StartsAt.In(loc).Format("02.01 15:04"),
			meeting.EndsAt.In(loc).Format("15:04"),
			meeting.Title,
		)
		infoBtn := markup.Data(label, b.btnMyMeetingInfo.Unique, strconv.FormatInt(meeting.ID, 10))
		if meeting.CreatorID == user.ID {
			cancelBtn := markup.Data("✕", b.btnMyMeetingCancelAsk.Unique, strconv.FormatInt(meeting.ID, 10))
			rows = append(rows, markup.Row(infoBtn, cancelBtn))
		} else {
			rows = append(rows, markup.Row(infoBtn))
		}
	}
	markup.Inline(rows...)

	return c.EditOrSend("📋 Мои ближайшие встречи:", markup)
}

func (b *Bot) handleMyMeetingInfo(c tele.Context) error {
	return c.Respond()
}

func (b *Bot) handleMyMeetingCancelAsk(c tele.Context) error {
	defer c.Respond()

	if _, err := strconv.ParseInt(c.Data(), 10, 64); err != nil {
		return c.RespondText("Не понял, какую встречу отменить")
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			markup.Data("✅ Да, отменить", b.btnMyMeetingCancelConfirm.Unique, c.Data()),
			markup.Data("Отмена", b.btnMyMeetingCancelDecline.Unique, c.Data()),
		),
	)
	return c.Edit("❓ Точно отменить эту встречу?", markup)
}

func (b *Bot) handleMyMeetingCancelConfirm(c tele.Context) error {
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
			if editErr := c.Edit("Эту встречу уже нельзя отменить: не найдена или вы не создатель."); editErr != nil {
				return editErr
			}
			return b.sendMainMenu(c, user)
		}
		return err
	}

	if err := c.Edit("✅ Встреча отменена."); err != nil {
		return err
	}
	return b.sendMainMenu(c, user)
}

func (b *Bot) handleMyMeetingCancelDecline(c tele.Context) error {
	defer c.Respond()

	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	return b.renderMyMeetings(c, user)
}
