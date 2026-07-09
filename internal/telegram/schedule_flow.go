package telegram

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"meeting-bot/internal/domain"

	tele "gopkg.in/telebot.v3"
)

var ruWeekdaysShort = [...]string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

func weekdayLabel(t time.Time) string {
	return ruWeekdaysShort[(int(t.Weekday())+6)%7]
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func schedulePayload(mode string, anchor time.Time, meetingID int64) string {
	return fmt.Sprintf("%s|%s|%d", mode, anchor.Format("2006-01-02"), meetingID)
}

func scheduleListPayload(mode string, anchor time.Time) string {
	return fmt.Sprintf("%s|%s", mode, anchor.Format("2006-01-02"))
}

func parseSchedulePayload(data string, loc *time.Location) (mode string, anchor time.Time, meetingID int64, err error) {
	parts := strings.Split(data, "|")
	if len(parts) != 3 {
		return "", time.Time{}, 0, fmt.Errorf("bad schedule payload: %s", data)
	}
	anchor, err = time.ParseInLocation("2006-01-02", parts[1], loc)
	if err != nil {
		return "", time.Time{}, 0, err
	}
	meetingID, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", time.Time{}, 0, err
	}
	return parts[0], anchor, meetingID, nil
}

func parseScheduleListPayload(data string, loc *time.Location) (mode string, anchor time.Time, err error) {
	parts := strings.Split(data, "|")
	if len(parts) != 2 {
		return "", time.Time{}, fmt.Errorf("bad schedule list payload: %s", data)
	}
	anchor, err = time.ParseInLocation("2006-01-02", parts[1], loc)
	if err != nil {
		return "", time.Time{}, err
	}
	return parts[0], anchor, nil
}

func scheduleMeetingLabel(meeting domain.Meeting, user domain.User, loc *time.Location) string {
	prefix := ""
	if meeting.CreatorID == user.ID || containsID(meeting.ParticipantIDs, user.ID) {
		prefix = "★ "
	}
	return fmt.Sprintf(
		"%s%s–%s — %s",
		prefix,
		meeting.StartsAt.In(loc).Format("15:04"),
		meeting.EndsAt.In(loc).Format("15:04"),
		meeting.Title,
	)
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "нет"
	}
	return strings.Join(items, ", ")
}

func (b *Bot) handleScheduleStart(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	if !user.IsRegistered() {
		return b.sendMainMenu(c, user)
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnScheduleToday, b.btnScheduleTomorrow),
		markup.Row(b.btnScheduleWeek, b.btnScheduleOtherDay),
	)
	return c.Send("🗓 Расписание команды. За какой период?", markup)
}

func (b *Bot) handleScheduleToday(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	now := b.clock.Now().In(b.userLocation(user))
	return b.renderScheduleDay(c, user, startOfDay(now))
}

func (b *Bot) handleScheduleTomorrow(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	now := b.clock.Now().In(b.userLocation(user))
	return b.renderScheduleDay(c, user, startOfDay(now).AddDate(0, 0, 1))
}

func (b *Bot) handleScheduleWeek(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	now := b.clock.Now().In(b.userLocation(user))
	return b.renderScheduleWeek(c, user, startOfDay(now))
}

func (b *Bot) handleScheduleOtherDay(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	now := b.clock.Now().In(b.userLocation(user))
	return b.askScheduleCalendar(c, user, startOfMonth(now))
}

func (b *Bot) askScheduleCalendar(c tele.Context, user domain.User, month time.Time) error {
	loc := b.userLocation(user)
	now := b.clock.Now().In(loc)

	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, week := range calendarWeeks(month) {
		var buttons []tele.Btn
		for _, day := range week {
			if day == 0 {
				continue
			}
			date := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, loc)
			buttons = append(buttons, markup.Data(strconv.Itoa(day), b.btnSchedulePick.Unique, date.Format("2006-01-02")))
		}
		if len(buttons) > 0 {
			rows = append(rows, markup.Row(buttons...))
		}
	}

	minMonth := startOfMonth(now).AddDate(0, -maxMonthsAhead, 0)
	maxMonth := startOfMonth(now).AddDate(0, maxMonthsAhead, 0)
	var navButtons []tele.Btn
	if startOfMonth(month).After(minMonth) {
		navButtons = append(navButtons, markup.Data("◀", b.btnScheduleCalPrev.Unique, month.Format("2006-01")))
	}
	if startOfMonth(month).Before(maxMonth) {
		navButtons = append(navButtons, markup.Data("▶", b.btnScheduleCalNext.Unique, month.Format("2006-01")))
	}
	if len(navButtons) > 0 {
		rows = append(rows, markup.Row(navButtons...))
	}
	markup.Inline(rows...)

	text := fmt.Sprintf("📆 Выберите день\n\n%s", monthLabel(month))
	return c.EditOrSend(text, markup)
}

func (b *Bot) handleScheduleCalPrev(c tele.Context) error {
	defer c.Respond()
	return b.shiftScheduleCalendar(c, -1)
}

func (b *Bot) handleScheduleCalNext(c tele.Context) error {
	defer c.Respond()
	return b.shiftScheduleCalendar(c, 1)
}

func (b *Bot) shiftScheduleCalendar(c tele.Context, delta int) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	loc := b.userLocation(user)
	month, err := time.ParseInLocation("2006-01", c.Data(), loc)
	if err != nil {
		return c.RespondText("Не понял месяц")
	}
	return b.askScheduleCalendar(c, user, startOfMonth(month).AddDate(0, delta, 0))
}

func (b *Bot) handleSchedulePick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	date, err := time.ParseInLocation("2006-01-02", c.Data(), b.userLocation(user))
	if err != nil {
		return c.RespondText("Не понял дату")
	}
	return b.renderScheduleDay(c, user, date)
}

func (b *Bot) handleScheduleDayPrev(c tele.Context) error {
	defer c.Respond()
	return b.shiftScheduleDay(c, -1)
}

func (b *Bot) handleScheduleDayNext(c tele.Context) error {
	defer c.Respond()
	return b.shiftScheduleDay(c, 1)
}

func (b *Bot) shiftScheduleDay(c tele.Context, delta int) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	date, err := time.ParseInLocation("2006-01-02", c.Data(), b.userLocation(user))
	if err != nil {
		return c.RespondText("Не понял дату")
	}
	return b.renderScheduleDay(c, user, date.AddDate(0, 0, delta))
}

func (b *Bot) renderScheduleDay(c tele.Context, user domain.User, date time.Time) error {
	loc := b.userLocation(user)
	from := startOfDay(date)
	to := from.AddDate(0, 0, 1)

	meetings, err := b.listSchedule.Execute(context.Background(), from, to)
	if err != nil {
		return err
	}
	sort.Slice(meetings, func(i, j int) bool { return meetings[i].StartsAt.Before(meetings[j].StartsAt) })

	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, meeting := range meetings {
		rows = append(rows, markup.Row(markup.Data(
			scheduleMeetingLabel(meeting, user, loc),
			b.btnScheduleMeetingInfo.Unique,
			schedulePayload("day", from, meeting.ID),
		)))
	}
	rows = append(rows, markup.Row(
		markup.Data("◀ Пред. день", b.btnScheduleDayPrev.Unique, from.Format("2006-01-02")),
		markup.Data("След. день ▶", b.btnScheduleDayNext.Unique, from.Format("2006-01-02")),
	))
	rows = append(rows, markup.Row(b.btnBackToMenu))
	markup.Inline(rows...)

	header := fmt.Sprintf("🗓 Расписание на %s (%s)", from.Format("02.01.2006"), weekdayLabel(from))
	if len(meetings) == 0 {
		header += "\n\nВстреч нет."
	} else {
		header += "\n\n★ — ваши встречи"
	}
	return c.EditOrSend(header, markup)
}

func (b *Bot) renderScheduleWeek(c tele.Context, user domain.User, weekStart time.Time) error {
	loc := b.userLocation(user)
	from := startOfDay(weekStart)
	to := from.AddDate(0, 0, 7)

	meetings, err := b.listSchedule.Execute(context.Background(), from, to)
	if err != nil {
		return err
	}
	sort.Slice(meetings, func(i, j int) bool { return meetings[i].StartsAt.Before(meetings[j].StartsAt) })

	byDay := make(map[string][]domain.Meeting)
	for _, meeting := range meetings {
		key := startOfDay(meeting.StartsAt.In(loc)).Format("2006-01-02")
		byDay[key] = append(byDay[key], meeting)
	}

	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for i := 0; i < 7; i++ {
		day := from.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		rows = append(rows, markup.Row(markup.Data(
			fmt.Sprintf("── %s %s ──", weekdayLabel(day), day.Format("02.01")),
			b.btnScheduleNoop.Unique, "noop",
		)))
		for _, meeting := range byDay[key] {
			rows = append(rows, markup.Row(markup.Data(
				scheduleMeetingLabel(meeting, user, loc),
				b.btnScheduleMeetingInfo.Unique,
				schedulePayload("week", from, meeting.ID),
			)))
		}
	}
	rows = append(rows, markup.Row(b.btnBackToMenu))
	markup.Inline(rows...)

	header := fmt.Sprintf("🗓 Расписание на неделю: %s – %s", from.Format("02.01"), from.AddDate(0, 0, 6).Format("02.01.2006"))
	if len(meetings) > 0 {
		header += "\n\n★ — ваши встречи"
	}
	return c.EditOrSend(header, markup)
}

func (b *Bot) handleScheduleNoop(c tele.Context) error {
	return c.Respond()
}

func (b *Bot) handleScheduleMeetingInfo(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	loc := b.userLocation(user)

	mode, anchor, meetingID, err := parseSchedulePayload(c.Data(), loc)
	if err != nil {
		return c.RespondText("Не понял встречу")
	}

	from := anchor
	to := from.AddDate(0, 0, 1)
	if mode == "week" {
		to = from.AddDate(0, 0, 7)
	}

	meetings, err := b.listSchedule.Execute(context.Background(), from, to)
	if err != nil {
		return err
	}

	var found *domain.Meeting
	for i := range meetings {
		if meetings[i].ID == meetingID {
			found = &meetings[i]
			break
		}
	}
	if found == nil {
		return c.RespondText("Встреча не найдена")
	}

	candidates, err := b.users.ListRegistered(context.Background())
	if err != nil {
		return err
	}
	names := make(map[int64]string, len(candidates))
	for _, candidate := range candidates {
		names[candidate.ID] = valueOrDash(candidate.DisplayName)
	}

	participantNames := make([]string, 0, len(found.ParticipantIDs))
	for _, id := range found.ParticipantIDs {
		if name, ok := names[id]; ok {
			participantNames = append(participantNames, name)
		}
	}

	text := fmt.Sprintf(
		"🗓 %s\n%s, %s–%s\n\n👤 Организатор: %s\n👥 Участники: %s",
		found.Title,
		found.StartsAt.In(loc).Format("02.01.2006"),
		found.StartsAt.In(loc).Format("15:04"),
		found.EndsAt.In(loc).Format("15:04"),
		names[found.CreatorID],
		joinOrDash(participantNames),
	)
	if found.MeetLink != "" {
		text += fmt.Sprintf("\n\n🔗 Google Meet: %s", found.MeetLink)
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(markup.Data("◀ Назад к списку", b.btnScheduleBack.Unique, scheduleListPayload(mode, anchor))))

	return c.Edit(text, markup)
}

func (b *Bot) handleScheduleBack(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	mode, anchor, err := parseScheduleListPayload(c.Data(), b.userLocation(user))
	if err != nil {
		return c.RespondText("Не понял")
	}
	if mode == "week" {
		return b.renderScheduleWeek(c, user, anchor)
	}
	return b.renderScheduleDay(c, user, anchor)
}
