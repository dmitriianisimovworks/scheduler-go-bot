package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"meeting-bot/internal/domain"

	tele "gopkg.in/telebot.v3"
)

const (
	defaultWorkdayStartHour = 9
	defaultWorkdayEndHour   = 19

	settingCalendarID   = "calendar_id"
	settingWorkdayStart = "workday_start_hour"
	settingWorkdayEnd   = "workday_end_hour"

	adminInputCalendarID = "calendar_id"
)

var workdayPresets = [][2]int{{8, 17}, {9, 18}, {9, 19}, {10, 20}}

func (b *Bot) isAdmin(telegramID int64) bool {
	return b.adminIDs[telegramID]
}

func (b *Bot) menuKeyboardFor(user domain.User) *tele.ReplyMarkup {
	if b.isAdmin(user.TelegramID) {
		return b.adminMenuKeyboard
	}
	return b.mainMenuKeyboard
}

func (b *Bot) loadSettings() {
	if b.settings == nil {
		return
	}
	ctx := context.Background()

	if calendarID, err := b.settings.GetSetting(ctx, settingCalendarID); err == nil && calendarID != "" && b.calendar != nil {
		b.calendar.SetCalendarID(calendarID)
	}
	if raw, err := b.settings.GetSetting(ctx, settingWorkdayStart); err == nil && raw != "" {
		if v, convErr := strconv.Atoi(raw); convErr == nil {
			b.workdayStartHour = v
		}
	}
	if raw, err := b.settings.GetSetting(ctx, settingWorkdayEnd); err == nil && raw != "" {
		if v, convErr := strconv.Atoi(raw); convErr == nil {
			b.workdayEndHour = v
		}
	}
}

func (b *Bot) workdayWindow() (int, int) {
	b.workdayMu.RLock()
	defer b.workdayMu.RUnlock()
	return b.workdayStartHour, b.workdayEndHour
}

func (b *Bot) setWorkdayWindow(start int, end int) {
	b.workdayMu.Lock()
	defer b.workdayMu.Unlock()
	b.workdayStartHour = start
	b.workdayEndHour = end
}

func (b *Bot) setAdminInput(telegramID int64, field string) {
	b.adminInputMu.Lock()
	defer b.adminInputMu.Unlock()
	b.adminInput[telegramID] = field
}

func (b *Bot) getAdminInput(telegramID int64) (string, bool) {
	b.adminInputMu.Lock()
	defer b.adminInputMu.Unlock()
	field, ok := b.adminInput[telegramID]
	return field, ok
}

func (b *Bot) clearAdminInput(telegramID int64) {
	b.adminInputMu.Lock()
	defer b.adminInputMu.Unlock()
	delete(b.adminInput, telegramID)
}

func (b *Bot) handleAdminInputText(c tele.Context, user domain.User, field string, text string) error {
	b.clearAdminInput(user.TelegramID)
	value := strings.TrimSpace(text)
	if value == "" {
		return c.Send("Пустое значение, отменено.", b.menuKeyboardFor(user))
	}

	switch field {
	case adminInputCalendarID:
		if err := b.settings.SetSetting(context.Background(), settingCalendarID, value); err != nil {
			return err
		}
		b.calendar.SetCalendarID(value)
		return c.Send("✅ Календарь обновлён для всех.", b.menuKeyboardFor(user))
	default:
		return b.sendMainMenu(c, user)
	}
}

func (b *Bot) handleAdminConfigStart(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	return b.renderAdminConfig(c, user)
}

func (b *Bot) renderAdminConfig(c tele.Context, user domain.User) error {
	if !b.isAdmin(user.TelegramID) {
		return b.sendMainMenu(c, user)
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnAdminCalendar),
		markup.Row(b.btnAdminWorkHours),
		markup.Row(b.btnAdminEmployees),
	)
	return c.EditOrSend("⚙️ Конфигурация", markup)
}

func (b *Bot) handleAdminBack(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	return b.renderAdminConfig(c, user)
}

func (b *Bot) handleAdminCalendar(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if !b.isAdmin(user.TelegramID) {
		return nil
	}

	current := b.calendar.CalendarID()
	if current == "" {
		current = "не задан"
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnAdminCalendarEdit),
		markup.Row(b.btnAdminBack),
	)
	return c.Edit(fmt.Sprintf("📅 Текущий Google-календарь:\n%s", current), markup)
}

func (b *Bot) handleAdminCalendarEdit(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if !b.isAdmin(user.TelegramID) {
		return nil
	}

	b.setAdminInput(user.TelegramID, adminInputCalendarID)
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnAdminInputCancel))
	return c.Send("Пришлите новый Calendar ID (например xxx@group.calendar.google.com).", markup)
}

func (b *Bot) handleAdminInputCancel(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	b.clearAdminInput(user.TelegramID)
	return b.renderAdminConfig(c, user)
}

func (b *Bot) handleAdminWorkHours(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if !b.isAdmin(user.TelegramID) {
		return nil
	}

	start, end := b.workdayWindow()

	markup := &tele.ReplyMarkup{}
	buttons := make([]tele.Btn, 0, len(workdayPresets))
	for _, preset := range workdayPresets {
		label := fmt.Sprintf("%02d:00–%02d:00", preset[0], preset[1])
		buttons = append(buttons, markup.Data(label, b.btnAdminWorkHoursPick.Unique, fmt.Sprintf("%d-%d", preset[0], preset[1])))
	}

	rows := []tele.Row{
		markup.Row(buttons[0], buttons[1]),
		markup.Row(buttons[2], buttons[3]),
		markup.Row(b.btnAdminBack),
	}
	markup.Inline(rows...)

	text := fmt.Sprintf("🕐 Текущие рабочие часы: %02d:00–%02d:00\n\nВыберите новое окно:", start, end)
	return c.Edit(text, markup)
}

func (b *Bot) handleAdminWorkHoursPick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if !b.isAdmin(user.TelegramID) {
		return nil
	}

	parts := strings.Split(c.Data(), "-")
	if len(parts) != 2 {
		return c.RespondText("Не понял")
	}
	start, errS := strconv.Atoi(parts[0])
	end, errE := strconv.Atoi(parts[1])
	if errS != nil || errE != nil || start < 0 || end > 23 || start >= end {
		return c.RespondText("Не понял")
	}

	b.setWorkdayWindow(start, end)
	if err := b.settings.SetSetting(context.Background(), settingWorkdayStart, strconv.Itoa(start)); err != nil {
		return err
	}
	if err := b.settings.SetSetting(context.Background(), settingWorkdayEnd, strconv.Itoa(end)); err != nil {
		return err
	}

	return b.renderAdminConfig(c, user)
}

func (b *Bot) handleAdminEmployees(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if !b.isAdmin(user.TelegramID) {
		return nil
	}

	candidates, err := b.users.ListRegistered(context.Background())
	if err != nil {
		return err
	}

	text := "👥 Сотрудники:\n\nНикого нет."
	if len(candidates) > 0 {
		lines := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			lines = append(lines, "• "+participantLabel(candidate))
		}
		text = "👥 Сотрудники:\n\n" + strings.Join(lines, "\n")
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(b.btnAdminBack))
	return c.Edit(text, markup)
}
