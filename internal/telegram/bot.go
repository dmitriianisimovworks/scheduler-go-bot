package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"meeting-bot/internal/config"
	"meeting-bot/internal/domain"
	"meeting-bot/internal/integrations/sheets"
	"meeting-bot/internal/platform/clock"
	"meeting-bot/internal/platform/logger"
	"meeting-bot/internal/repository"
	"meeting-bot/internal/usecase"

	tele "gopkg.in/telebot.v3"
)

type Bot struct {
	config config.Config
	logger *logger.Logger
	users  repository.UserRepository
	clock  clock.Clock
	bot    *tele.Bot

	createMeeting usecase.CreateMeeting
	listDay       usecase.ListDay
	listWeek      usecase.ListWeek
	listUpcoming  usecase.ListUpcoming
	cancelMeeting usecase.CancelMeeting

	draftsMu sync.Mutex
	drafts   map[int64]*meetingDraft

	btnUseName       tele.Btn
	btnEnterName     tele.Btn
	btnTZMoscow      tele.Btn
	btnTZAlmaty      tele.Btn
	btnTZOther       tele.Btn
	btnEditName      tele.Btn
	btnEditTeam      tele.Btn
	btnEditRole      tele.Btn
	btnEditTimezone  tele.Btn
	btnBackToMenu    tele.Btn
	btnDateToday     tele.Btn
	btnDateTomorrow  tele.Btn
	btnDateOther     tele.Btn
	btnDur15         tele.Btn
	btnDur30         tele.Btn
	btnDur60         tele.Btn
	btnDur90         tele.Btn
	btnCancelPick    tele.Btn
	mainMenuKeyboard *tele.ReplyMarkup
}

func New(
	cfg config.Config,
	log *logger.Logger,
	clk clock.Clock,
	users repository.UserRepository,
	meetings repository.MeetingRepository,
	_ *sheets.Client,
) *Bot {
	b := &Bot{
		config:        cfg,
		logger:        log,
		users:         users,
		clock:         clk,
		createMeeting: usecase.NewCreateMeeting(meetings),
		listDay:       usecase.NewListDay(meetings),
		listWeek:      usecase.NewListWeek(meetings),
		listUpcoming:  usecase.NewListUpcoming(meetings),
		cancelMeeting: usecase.NewCancelMeeting(meetings),
		drafts:        make(map[int64]*meetingDraft),
	}
	b.initButtons()
	return b
}

func (b *Bot) Run(ctx context.Context) error {
	if b.config.TelegramBotToken == "" {
		b.logger.Printf("telegram token is empty; bot is disabled")
		<-ctx.Done()
		return nil
	}

	bot, err := tele.NewBot(tele.Settings{
		Token:       b.config.TelegramBotToken,
		Synchronous: true,
		OnError: func(err error, _ tele.Context) {
			b.logger.Printf("telegram handler error: %v", err)
		},
	})
	if err != nil {
		return err
	}

	b.bot = bot
	b.registerHandlers()

	if b.config.TelegramWebhookURL != "" {
		err := b.bot.SetWebhook(&tele.Webhook{
			Endpoint:    &tele.WebhookEndpoint{PublicURL: b.config.TelegramWebhookURL},
			SecretToken: b.config.TelegramWebhookSecret,
			DropUpdates: true,
		})
		if err != nil {
			return fmt.Errorf("set telegram webhook: %w", err)
		}
		b.logger.Printf("telegram webhook configured: %s", b.config.TelegramWebhookURL)
	}

	<-ctx.Done()
	return nil
}

func (b *Bot) ProcessUpdate(update tele.Update) {
	if b.bot == nil {
		b.logger.Printf("telegram update skipped: bot is not ready")
		return
	}
	b.bot.ProcessUpdate(update)
}

func (b *Bot) initButtons() {
	b.btnUseName = tele.Btn{Unique: "profile_use_name", Text: "✅ Использовать"}
	b.btnEnterName = tele.Btn{Unique: "profile_enter_name", Text: "✏️ Ввести другое"}
	b.btnTZMoscow = tele.Btn{Unique: "tz_moscow", Text: "Москва"}
	b.btnTZAlmaty = tele.Btn{Unique: "tz_almaty", Text: "Алматы"}
	b.btnTZOther = tele.Btn{Unique: "tz_other", Text: "Другой"}
	b.btnEditName = tele.Btn{Unique: "edit_name", Text: "✏️ Имя"}
	b.btnEditTeam = tele.Btn{Unique: "edit_team", Text: "🏢 Команда"}
	b.btnEditRole = tele.Btn{Unique: "edit_role", Text: "💼 Роль"}
	b.btnEditTimezone = tele.Btn{Unique: "edit_timezone", Text: "🌍 Часовой пояс"}
	b.btnBackToMenu = tele.Btn{Unique: "back_menu", Text: "⬅️ В меню"}
	b.btnDateToday = tele.Btn{Unique: "meeting_date_today", Text: "Сегодня"}
	b.btnDateTomorrow = tele.Btn{Unique: "meeting_date_tomorrow", Text: "Завтра"}
	b.btnDateOther = tele.Btn{Unique: "meeting_date_other", Text: "Другая дата"}
	b.btnDur15 = tele.Btn{Unique: "meeting_dur_15", Text: "15 мин"}
	b.btnDur30 = tele.Btn{Unique: "meeting_dur_30", Text: "30 мин"}
	b.btnDur60 = tele.Btn{Unique: "meeting_dur_60", Text: "1 час"}
	b.btnDur90 = tele.Btn{Unique: "meeting_dur_90", Text: "1.5 часа"}
	b.btnCancelPick = tele.Btn{Unique: "cancel_meeting_btn"}

	menu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
	menu.Reply(
		menu.Row(menu.Text("➕ Создать встречу"), menu.Text("📅 Сегодня")),
		menu.Row(menu.Text("🗓 Неделя"), menu.Text("👤 Профиль")),
		menu.Row(menu.Text("🗑 Отменить встречу"), menu.Text("❓ Помощь")),
	)
	b.mainMenuKeyboard = menu
}

func (b *Bot) registerHandlers() {
	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/profile", b.handleProfile)
	b.bot.Handle("/help", b.handleHelp)

	b.bot.Handle("👤 Профиль", b.handleProfile)
	b.bot.Handle("❓ Помощь", b.handleHelp)
	b.bot.Handle("📅 Сегодня", b.handleToday)
	b.bot.Handle("🗓 Неделя", b.handleWeek)
	b.bot.Handle("➕ Создать встречу", b.handleCreateMeetingStart)
	b.bot.Handle("🗑 Отменить встречу", b.handleCancelMeetingStart)

	b.bot.Handle(&b.btnUseName, b.handleUseTelegramName)
	b.bot.Handle(&b.btnEnterName, b.handleEnterName)
	b.bot.Handle(&b.btnTZMoscow, b.handleTimezoneMoscow)
	b.bot.Handle(&b.btnTZAlmaty, b.handleTimezoneAlmaty)
	b.bot.Handle(&b.btnTZOther, b.handleTimezoneOther)
	b.bot.Handle(&b.btnEditName, b.handleEditName)
	b.bot.Handle(&b.btnEditTeam, b.handleEditTeam)
	b.bot.Handle(&b.btnEditRole, b.handleEditRole)
	b.bot.Handle(&b.btnEditTimezone, b.handleEditTimezone)
	b.bot.Handle(&b.btnBackToMenu, b.handleBackToMenu)

	b.bot.Handle(&b.btnDateToday, b.handleMeetingDateToday)
	b.bot.Handle(&b.btnDateTomorrow, b.handleMeetingDateTomorrow)
	b.bot.Handle(&b.btnDateOther, b.handleMeetingDateOther)
	b.bot.Handle(&b.btnDur15, b.handleMeetingDuration(15*time.Minute))
	b.bot.Handle(&b.btnDur30, b.handleMeetingDuration(30*time.Minute))
	b.bot.Handle(&b.btnDur60, b.handleMeetingDuration(60*time.Minute))
	b.bot.Handle(&b.btnDur90, b.handleMeetingDuration(90*time.Minute))
	b.bot.Handle(&b.btnCancelPick, b.handleCancelMeetingPick)

	b.bot.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleStart(c tele.Context) error {
	sender := c.Sender()
	if sender == nil {
		return c.Send("Не удалось определить пользователя.")
	}

	user, err := b.users.Upsert(context.Background(), userFromTelegram(sender, b.config.AppTimezone))
	if err != nil {
		return err
	}

	if user.IsRegistered() {
		return b.sendMainMenu(c, user)
	}

	return b.askNameConfirmation(c, user)
}

func (b *Bot) handleUseTelegramName(c tele.Context) error {
	defer c.Respond()

	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.UpdateDisplayName(context.Background(), user.TelegramID, user.FullName); err != nil {
		return err
	}
	return b.askTeam(c)
}

func (b *Bot) handleEnterName(c tele.Context) error {
	defer c.Respond()

	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterName); err != nil {
		return err
	}
	return c.Send("✏️ Напишите имя, которое будут видеть коллеги.")
}

func (b *Bot) handleText(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}

	text := strings.TrimSpace(c.Text())
	if text == "" {
		return nil
	}

	switch user.RegistrationStep {
	case domain.RegistrationStepEnterName:
		if err := b.users.UpdateDisplayName(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.askTeam(c)
	case domain.RegistrationStepEnterTeam:
		if err := b.users.UpdateTeam(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.askRole(c)
	case domain.RegistrationStepEnterRole:
		if err := b.users.UpdateRole(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.askTimezone(c)
	case domain.RegistrationStepEnterTimezone:
		return b.saveTimezoneAndComplete(c, user, text)
	case domain.RegistrationStepEditName:
		if err := b.users.UpdateDisplayName(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.finishProfileEdit(c, user.TelegramID)
	case domain.RegistrationStepEditTeam:
		if err := b.users.UpdateTeam(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.finishProfileEdit(c, user.TelegramID)
	case domain.RegistrationStepEditRole:
		if err := b.users.UpdateRole(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.finishProfileEdit(c, user.TelegramID)
	case domain.RegistrationStepEditTimezone:
		if _, err := timeLocation(text); err != nil {
			return c.Send("Не узнал такой часовой пояс. Пример: Europe/Moscow")
		}
		if err := b.users.UpdateTimezone(context.Background(), user.TelegramID, text); err != nil {
			return err
		}
		return b.finishProfileEdit(c, user.TelegramID)
	default:
		if draft, ok := b.getDraft(user.TelegramID); ok {
			return b.handleMeetingDraftText(c, user, draft, text)
		}
		return b.sendMainMenu(c, user)
	}
}

func (b *Bot) askNameConfirmation(c tele.Context, user domain.User) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnUseName),
		markup.Row(b.btnEnterName),
	)

	name := user.FullName
	if name == "" {
		name = user.Username
	}

	return c.Send(
		fmt.Sprintf("👋 Привет. Это календарь встреч команды.\n\nИспользовать имя: %s?", name),
		markup,
	)
}

func (b *Bot) askTeam(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterTeam); err != nil {
		return err
	}
	return c.Send("🏢 Укажите команду или отдел. Например: Маркетинг, Закупки, Контент.")
}

func (b *Bot) askRole(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterRole); err != nil {
		return err
	}
	return c.Send("💼 Укажите роль. Например: Руководитель, Менеджер, Дизайнер.")
}

func (b *Bot) askTimezone(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterTimezone); err != nil {
		return err
	}

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnTZMoscow, b.btnTZAlmaty),
		markup.Row(b.btnTZOther),
	)

	return c.Send("🌍 Выберите часовой пояс.", markup)
}

func (b *Bot) handleTimezoneMoscow(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	return b.saveTimezoneAndComplete(c, user, "Europe/Moscow")
}

func (b *Bot) handleTimezoneAlmaty(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	return b.saveTimezoneAndComplete(c, user, "Asia/Almaty")
}

func (b *Bot) handleTimezoneOther(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterTimezone); err != nil {
		return err
	}
	return c.Send("Напишите часовой пояс в формате IANA. Например: Europe/Moscow")
}

func (b *Bot) saveTimezoneAndComplete(c tele.Context, user domain.User, timezone string) error {
	if _, err := timeLocation(timezone); err != nil {
		return c.Send("Не узнал такой часовой пояс. Пример: Europe/Moscow")
	}
	if err := b.users.UpdateTimezone(context.Background(), user.TelegramID, timezone); err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepComplete); err != nil {
		return err
	}

	updated, err := b.users.GetByTelegramID(context.Background(), user.TelegramID)
	if err != nil {
		return err
	}

	if err := c.Send("✅ Профиль создан."); err != nil {
		return err
	}
	return b.sendMainMenu(c, updated)
}

func (b *Bot) handleProfile(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	return b.sendProfile(c, user)
}

func (b *Bot) sendProfile(c tele.Context, user domain.User) error {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(b.btnEditName, b.btnEditTeam),
		markup.Row(b.btnEditRole, b.btnEditTimezone),
		markup.Row(b.btnBackToMenu),
	)

	return c.Send(formatProfile(user), markup)
}

func (b *Bot) handleEditName(c tele.Context) error {
	return b.setEditStep(c, domain.RegistrationStepEditName, "✏️ Напишите новое имя для коллег.")
}

func (b *Bot) handleEditTeam(c tele.Context) error {
	return b.setEditStep(c, domain.RegistrationStepEditTeam, "🏢 Напишите новую команду или отдел.")
}

func (b *Bot) handleEditRole(c tele.Context) error {
	return b.setEditStep(c, domain.RegistrationStepEditRole, "💼 Напишите новую роль.")
}

func (b *Bot) handleEditTimezone(c tele.Context) error {
	return b.setEditStep(c, domain.RegistrationStepEditTimezone, "🌍 Напишите часовой пояс. Например: Europe/Moscow")
}

func (b *Bot) setEditStep(c tele.Context, step string, prompt string) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, step); err != nil {
		return err
	}
	return c.Send(prompt)
}

func (b *Bot) finishProfileEdit(c tele.Context, telegramID int64) error {
	if err := b.users.SetRegistrationStep(context.Background(), telegramID, domain.RegistrationStepComplete); err != nil {
		return err
	}
	user, err := b.users.GetByTelegramID(context.Background(), telegramID)
	if err != nil {
		return err
	}
	return b.sendProfile(c, user)
}

func (b *Bot) handleBackToMenu(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepComplete); err != nil {
		return err
	}
	return b.sendMainMenu(c, user)
}

func (b *Bot) handleToday(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	loc := b.userLocation(user)
	day := startOfDay(b.clock.Now().In(loc))

	meetings, err := b.listDay.Execute(context.Background(), day)
	if err != nil {
		return err
	}
	return c.Send(formatMeetingsList("📅 Встречи сегодня", meetings, loc, "На сегодня встреч пока нет."), b.mainMenuKeyboard)
}

func (b *Bot) handleWeek(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	loc := b.userLocation(user)
	now := b.clock.Now().In(loc)
	offset := (int(now.Weekday()) + 6) % 7
	weekStart := startOfDay(now).AddDate(0, 0, -offset)

	meetings, err := b.listWeek.Execute(context.Background(), weekStart)
	if err != nil {
		return err
	}
	return c.Send(formatMeetingsList("🗓 Встречи на неделе", meetings, loc, "На этой неделе встреч пока нет."), b.mainMenuKeyboard)
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send("❓ Я помогу вести календарь встреч команды.\n\nСейчас доступно: регистрация, профиль, создание встреч с проверкой конфликтов, просмотр расписания на сегодня и на неделю, отмена своих встреч.", b.mainMenuKeyboard)
}

func (b *Bot) sendMainMenu(c tele.Context, user domain.User) error {
	return c.Send(formatMainMenu(user), b.mainMenuKeyboard)
}

func (b *Bot) currentUser(c tele.Context) (domain.User, error) {
	sender := c.Sender()
	if sender == nil {
		return domain.User{}, fmt.Errorf("telegram sender is empty")
	}
	return b.users.GetByTelegramID(context.Background(), sender.ID)
}

func userFromTelegram(user *tele.User, timezone string) domain.User {
	fullName := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if fullName == "" {
		fullName = user.Username
	}
	return domain.User{
		TelegramID:  user.ID,
		Username:    user.Username,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		FullName:    fullName,
		DisplayName: fullName,
		Timezone:    timezone,
	}
}
