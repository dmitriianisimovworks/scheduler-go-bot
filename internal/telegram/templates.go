package telegram

import (
	"context"

	"meeting-bot/internal/domain"

	tele "gopkg.in/telebot.v3"
)

var teamOptions = []string{
	"Маркетинг", "Продажи", "Продукт", "Разработка", "Дизайн",
	"HR", "Финансы", "Операции", "Поддержка", "Руководство",
}

var roleOptions = []string{
	"Руководитель", "Менеджер", "Маркетолог", "Дизайнер", "Разработчик",
	"Аналитик", "HR-менеджер", "Специалист поддержки", "Продакт-менеджер", "Финансист",
}

type timezoneOption struct {
	Label string
	Zone  string
}

var timezoneOptions = []timezoneOption{
	{"Москва (UTC+3)", "Europe/Moscow"},
	{"Калининград (UTC+2)", "Europe/Kaliningrad"},
	{"Минск (UTC+3)", "Europe/Minsk"},
	{"Киев (UTC+2)", "Europe/Kyiv"},
	{"Екатеринбург (UTC+5)", "Asia/Yekaterinburg"},
	{"Алматы (UTC+6)", "Asia/Almaty"},
	{"Ташкент (UTC+5)", "Asia/Tashkent"},
	{"Ереван (UTC+4)", "Asia/Yerevan"},
	{"Баку (UTC+4)", "Asia/Baku"},
	{"Владивосток (UTC+10)", "Asia/Vladivostok"},
}

func (b *Bot) sendTeamOptions(c tele.Context, prompt string) error {
	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for i := 0; i < len(teamOptions); i += 2 {
		end := min(i+2, len(teamOptions))
		var buttons []tele.Btn
		for _, opt := range teamOptions[i:end] {
			buttons = append(buttons, markup.Data(opt, b.btnTeamPick.Unique, opt))
		}
		rows = append(rows, markup.Row(buttons...))
	}
	rows = append(rows, markup.Row(b.btnTeamOther))
	markup.Inline(rows...)
	return c.Send(prompt, markup)
}

func (b *Bot) sendRoleOptions(c tele.Context, prompt string) error {
	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for i := 0; i < len(roleOptions); i += 2 {
		end := min(i+2, len(roleOptions))
		var buttons []tele.Btn
		for _, opt := range roleOptions[i:end] {
			buttons = append(buttons, markup.Data(opt, b.btnRolePick.Unique, opt))
		}
		rows = append(rows, markup.Row(buttons...))
	}
	rows = append(rows, markup.Row(b.btnRoleOther))
	markup.Inline(rows...)
	return c.Send(prompt, markup)
}

func (b *Bot) sendTimezoneOptions(c tele.Context, prompt string) error {
	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for i := 0; i < len(timezoneOptions); i += 2 {
		end := min(i+2, len(timezoneOptions))
		var buttons []tele.Btn
		for _, opt := range timezoneOptions[i:end] {
			buttons = append(buttons, markup.Data(opt.Label, b.btnTZPick.Unique, opt.Zone))
		}
		rows = append(rows, markup.Row(buttons...))
	}
	rows = append(rows, markup.Row(b.btnTZOther))
	markup.Inline(rows...)
	return c.Send(prompt, markup)
}

func (b *Bot) handleTeamPick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.UpdateTeam(context.Background(), user.TelegramID, c.Data()); err != nil {
		return err
	}
	if user.RegistrationStep == domain.RegistrationStepEditTeam {
		return b.finishProfileEdit(c, user.TelegramID)
	}
	return b.askRole(c)
}

func (b *Bot) handleTeamOther(c tele.Context) error {
	defer c.Respond()
	return c.Send("🏢 Напишите свою команду или отдел вручную.")
}

func (b *Bot) handleRolePick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.UpdateRole(context.Background(), user.TelegramID, c.Data()); err != nil {
		return err
	}
	if user.RegistrationStep == domain.RegistrationStepEditRole {
		return b.finishProfileEdit(c, user.TelegramID)
	}
	return b.askTimezone(c)
}

func (b *Bot) handleRoleOther(c tele.Context) error {
	defer c.Respond()
	return c.Send("💼 Напишите свою роль вручную.")
}

func (b *Bot) handleTZPick(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	zone := c.Data()
	if user.RegistrationStep == domain.RegistrationStepEditTimezone {
		if _, err := timeLocation(zone); err != nil {
			return c.RespondText("Некорректный часовой пояс")
		}
		if err := b.users.UpdateTimezone(context.Background(), user.TelegramID, zone); err != nil {
			return err
		}
		return b.finishProfileEdit(c, user.TelegramID)
	}
	return b.saveTimezoneAndComplete(c, user, zone)
}
