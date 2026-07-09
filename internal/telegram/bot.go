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
	config   config.Config
	logger   *logger.Logger
	users    repository.UserRepository
	settings repository.SettingsRepository
	calendar usecase.CalendarClient
	clock    clock.Clock
	bot      *tele.Bot

	createMeeting  usecase.CreateMeeting
	listMyMeetings usecase.ListMyMeetings
	cancelMeeting  usecase.CancelMeeting
	listSchedule   usecase.ListSchedule

	draftsMu sync.Mutex
	drafts   map[int64]*meetingDraft

	adminIDs map[int64]bool

	workdayMu        sync.RWMutex
	workdayStartHour int
	workdayEndHour   int

	adminInputMu sync.Mutex
	adminInput   map[int64]string

	btnUseName           tele.Btn
	btnEnterName         tele.Btn
	btnTZPick            tele.Btn
	btnTZOther           tele.Btn
	btnEditName          tele.Btn
	btnEditTeam          tele.Btn
	btnEditRole          tele.Btn
	btnEditTimezone      tele.Btn
	btnBackToMenu        tele.Btn
	btnDateToday         tele.Btn
	btnDateTomorrow      tele.Btn
	btnDateOther         tele.Btn
	btnDatePick          tele.Btn
	btnMonthPrev         tele.Btn
	btnMonthNext         tele.Btn
	btnDur15             tele.Btn
	btnDur30             tele.Btn
	btnDur60             tele.Btn
	btnDur90             tele.Btn
	btnTimePick          tele.Btn
	btnTimeOther         tele.Btn
	btnParticipantToggle tele.Btn
	btnParticipantsDone  tele.Btn
	btnDraftCancel       tele.Btn

	btnTeamPick  tele.Btn
	btnTeamOther tele.Btn
	btnRolePick  tele.Btn
	btnRoleOther tele.Btn

	btnMyMeetingInfo          tele.Btn
	btnMyMeetingCancelAsk     tele.Btn
	btnMyMeetingCancelConfirm tele.Btn
	btnMyMeetingCancelDecline tele.Btn

	btnAdminCalendar      tele.Btn
	btnAdminCalendarEdit  tele.Btn
	btnAdminWorkHours     tele.Btn
	btnAdminWorkHoursPick tele.Btn
	btnAdminEmployees     tele.Btn
	btnAdminBack          tele.Btn
	btnAdminInputCancel   tele.Btn

	btnScheduleToday       tele.Btn
	btnScheduleTomorrow    tele.Btn
	btnScheduleWeek        tele.Btn
	btnScheduleOtherDay    tele.Btn
	btnSchedulePick        tele.Btn
	btnScheduleCalPrev     tele.Btn
	btnScheduleCalNext     tele.Btn
	btnScheduleDayPrev     tele.Btn
	btnScheduleDayNext     tele.Btn
	btnScheduleMeetingInfo tele.Btn
	btnScheduleBack        tele.Btn
	btnScheduleNoop        tele.Btn

	mainMenuKeyboard  *tele.ReplyMarkup
	adminMenuKeyboard *tele.ReplyMarkup
}

func New(
	cfg config.Config,
	log *logger.Logger,
	clk clock.Clock,
	users repository.UserRepository,
	meetings repository.MeetingRepository,
	settings repository.SettingsRepository,
	_ *sheets.Client,
	calendarClient usecase.CalendarClient,
) *Bot {
	adminIDs := make(map[int64]bool, len(cfg.AdminTelegramIDs))
	for _, id := range cfg.AdminTelegramIDs {
		adminIDs[id] = true
	}

	b := &Bot{
		config:           cfg,
		logger:           log,
		users:            users,
		settings:         settings,
		calendar:         calendarClient,
		clock:            clk,
		adminIDs:         adminIDs,
		workdayStartHour: defaultWorkdayStartHour,
		workdayEndHour:   defaultWorkdayEndHour,
		adminInput:       make(map[int64]string),
		createMeeting:    usecase.NewCreateMeeting(meetings, calendarClient),
		listMyMeetings:   usecase.NewListMyMeetings(meetings),
		cancelMeeting:    usecase.NewCancelMeeting(meetings, calendarClient),
		listSchedule:     usecase.NewListSchedule(meetings),
		drafts:           make(map[int64]*meetingDraft),
	}
	b.initButtons()
	b.loadSettings()
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

// SendReminder pushes a message to a user outside of any incoming update
// (e.g. a scheduled meeting reminder), unlike normal handlers which reply
// within a tele.Context.
func (b *Bot) SendReminder(telegramID int64, text string) error {
	if b.bot == nil {
		return fmt.Errorf("telegram bot is not ready")
	}
	_, err := b.bot.Send(tele.ChatID(telegramID), text)
	return err
}

func (b *Bot) initButtons() {
	b.btnUseName = tele.Btn{Unique: "profile_use_name", Text: "✅ Использовать"}
	b.btnEnterName = tele.Btn{Unique: "profile_enter_name", Text: "✏️ Ввести другое"}
	b.btnTZPick = tele.Btn{Unique: "tz_pick"}
	b.btnTZOther = tele.Btn{Unique: "tz_other", Text: "✏️ Другой"}
	b.btnEditName = tele.Btn{Unique: "edit_name", Text: "✏️ Имя"}
	b.btnEditTeam = tele.Btn{Unique: "edit_team", Text: "🏢 Команда"}
	b.btnEditRole = tele.Btn{Unique: "edit_role", Text: "💼 Роль"}
	b.btnEditTimezone = tele.Btn{Unique: "edit_timezone", Text: "🌍 Часовой пояс"}
	b.btnBackToMenu = tele.Btn{Unique: "back_menu", Text: "⬅️ В меню"}
	b.btnDateToday = tele.Btn{Unique: "meeting_date_today", Text: "Сегодня"}
	b.btnDateTomorrow = tele.Btn{Unique: "meeting_date_tomorrow", Text: "Завтра"}
	b.btnDateOther = tele.Btn{Unique: "meeting_date_other", Text: "✏️ Ввести дату вручную"}
	b.btnDatePick = tele.Btn{Unique: "meeting_date_pick"}
	b.btnMonthPrev = tele.Btn{Unique: "meeting_month_prev", Text: "◀"}
	b.btnMonthNext = tele.Btn{Unique: "meeting_month_next", Text: "▶"}
	b.btnDur15 = tele.Btn{Unique: "meeting_dur_15", Text: "15 мин"}
	b.btnDur30 = tele.Btn{Unique: "meeting_dur_30", Text: "30 мин"}
	b.btnDur60 = tele.Btn{Unique: "meeting_dur_60", Text: "1 час"}
	b.btnDur90 = tele.Btn{Unique: "meeting_dur_90", Text: "1.5 часа"}
	b.btnTimePick = tele.Btn{Unique: "meeting_time_pick"}
	b.btnTimeOther = tele.Btn{Unique: "meeting_time_other", Text: "✏️ Другое время"}
	b.btnParticipantToggle = tele.Btn{Unique: "meeting_participant_toggle"}
	b.btnParticipantsDone = tele.Btn{Unique: "meeting_participants_done", Text: "✅ Готово"}
	b.btnDraftCancel = tele.Btn{Unique: "meeting_draft_cancel", Text: "✖️ Отмена"}
	b.btnMyMeetingInfo = tele.Btn{Unique: "my_meeting_info"}
	b.btnMyMeetingCancelAsk = tele.Btn{Unique: "my_meeting_cancel_ask", Text: "✕"}
	b.btnMyMeetingCancelConfirm = tele.Btn{Unique: "my_meeting_cancel_confirm", Text: "✅ Да, отменить"}
	b.btnMyMeetingCancelDecline = tele.Btn{Unique: "my_meeting_cancel_decline", Text: "Отмена"}

	b.btnScheduleToday = tele.Btn{Unique: "sched_today", Text: "Сегодня"}
	b.btnScheduleTomorrow = tele.Btn{Unique: "sched_tomorrow", Text: "Завтра"}
	b.btnScheduleWeek = tele.Btn{Unique: "sched_week", Text: "Неделя"}
	b.btnScheduleOtherDay = tele.Btn{Unique: "sched_other_day", Text: "📆 Другой день"}
	b.btnSchedulePick = tele.Btn{Unique: "sched_pick"}
	b.btnScheduleCalPrev = tele.Btn{Unique: "sched_cal_prev"}
	b.btnScheduleCalNext = tele.Btn{Unique: "sched_cal_next"}
	b.btnScheduleDayPrev = tele.Btn{Unique: "sched_day_prev"}
	b.btnScheduleDayNext = tele.Btn{Unique: "sched_day_next"}
	b.btnScheduleMeetingInfo = tele.Btn{Unique: "sched_meeting_info"}
	b.btnScheduleBack = tele.Btn{Unique: "sched_back"}
	b.btnScheduleNoop = tele.Btn{Unique: "sched_noop"}

	b.btnTeamPick = tele.Btn{Unique: "team_pick"}
	b.btnTeamOther = tele.Btn{Unique: "team_other", Text: "✏️ Другое"}
	b.btnRolePick = tele.Btn{Unique: "role_pick"}
	b.btnRoleOther = tele.Btn{Unique: "role_other", Text: "✏️ Другое"}

	b.btnAdminCalendar = tele.Btn{Unique: "admin_calendar", Text: "📅 Google-календарь"}
	b.btnAdminCalendarEdit = tele.Btn{Unique: "admin_calendar_edit", Text: "✏️ Изменить"}
	b.btnAdminWorkHours = tele.Btn{Unique: "admin_workhours", Text: "🕐 Рабочие часы"}
	b.btnAdminWorkHoursPick = tele.Btn{Unique: "admin_workhours_pick"}
	b.btnAdminEmployees = tele.Btn{Unique: "admin_employees", Text: "👥 Сотрудники"}
	b.btnAdminBack = tele.Btn{Unique: "admin_back", Text: "◀ Назад"}
	b.btnAdminInputCancel = tele.Btn{Unique: "admin_input_cancel", Text: "✖️ Отмена"}

	menu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
	menu.Reply(
		menu.Row(menu.Text("➕ Создать встречу"), menu.Text("📋 Мои встречи")),
		menu.Row(menu.Text("🗓 Расписание команды"), menu.Text("👤 Профиль")),
	)
	b.mainMenuKeyboard = menu

	adminMenu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
	adminMenu.Reply(
		adminMenu.Row(adminMenu.Text("➕ Создать встречу"), adminMenu.Text("📋 Мои встречи")),
		adminMenu.Row(adminMenu.Text("🗓 Расписание команды"), adminMenu.Text("👤 Профиль")),
		adminMenu.Row(adminMenu.Text("⚙️ Конфигурация")),
	)
	b.adminMenuKeyboard = adminMenu
}

func (b *Bot) registerHandlers() {
	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/profile", b.handleProfile)

	b.bot.Handle("👤 Профиль", b.handleProfile)
	b.bot.Handle("📋 Мои встречи", b.handleMyMeetings)
	b.bot.Handle("➕ Создать встречу", b.handleCreateMeetingStart)
	b.bot.Handle("🗓 Расписание команды", b.handleScheduleStart)
	b.bot.Handle("⚙️ Конфигурация", b.handleAdminConfigStart)

	b.bot.Handle(&b.btnUseName, b.handleUseTelegramName)
	b.bot.Handle(&b.btnEnterName, b.handleEnterName)
	b.bot.Handle(&b.btnTZPick, b.handleTZPick)
	b.bot.Handle(&b.btnTZOther, b.handleTimezoneOther)
	b.bot.Handle(&b.btnEditName, b.handleEditName)
	b.bot.Handle(&b.btnEditTeam, b.handleEditTeam)
	b.bot.Handle(&b.btnEditRole, b.handleEditRole)
	b.bot.Handle(&b.btnEditTimezone, b.handleEditTimezone)
	b.bot.Handle(&b.btnBackToMenu, b.handleBackToMenu)

	b.bot.Handle(&b.btnDateToday, b.handleMeetingDateToday)
	b.bot.Handle(&b.btnDateTomorrow, b.handleMeetingDateTomorrow)
	b.bot.Handle(&b.btnDateOther, b.handleMeetingDateOther)
	b.bot.Handle(&b.btnDatePick, b.handleMeetingDatePick)
	b.bot.Handle(&b.btnMonthPrev, b.handleMeetingMonthPrev)
	b.bot.Handle(&b.btnMonthNext, b.handleMeetingMonthNext)
	b.bot.Handle(&b.btnDur15, b.handleMeetingDuration(15*time.Minute))
	b.bot.Handle(&b.btnDur30, b.handleMeetingDuration(30*time.Minute))
	b.bot.Handle(&b.btnDur60, b.handleMeetingDuration(60*time.Minute))
	b.bot.Handle(&b.btnDur90, b.handleMeetingDuration(90*time.Minute))
	b.bot.Handle(&b.btnTimePick, b.handleMeetingTimePick)
	b.bot.Handle(&b.btnTimeOther, b.handleMeetingTimeOther)
	b.bot.Handle(&b.btnParticipantToggle, b.handleParticipantToggle)
	b.bot.Handle(&b.btnParticipantsDone, b.handleParticipantsDone)
	b.bot.Handle(&b.btnDraftCancel, b.handleDraftCancel)
	b.bot.Handle(&b.btnMyMeetingInfo, b.handleMyMeetingInfo)
	b.bot.Handle(&b.btnMyMeetingCancelAsk, b.handleMyMeetingCancelAsk)
	b.bot.Handle(&b.btnMyMeetingCancelConfirm, b.handleMyMeetingCancelConfirm)
	b.bot.Handle(&b.btnMyMeetingCancelDecline, b.handleMyMeetingCancelDecline)

	b.bot.Handle(&b.btnScheduleToday, b.handleScheduleToday)
	b.bot.Handle(&b.btnScheduleTomorrow, b.handleScheduleTomorrow)
	b.bot.Handle(&b.btnScheduleWeek, b.handleScheduleWeek)
	b.bot.Handle(&b.btnScheduleOtherDay, b.handleScheduleOtherDay)
	b.bot.Handle(&b.btnSchedulePick, b.handleSchedulePick)
	b.bot.Handle(&b.btnScheduleCalPrev, b.handleScheduleCalPrev)
	b.bot.Handle(&b.btnScheduleCalNext, b.handleScheduleCalNext)
	b.bot.Handle(&b.btnScheduleDayPrev, b.handleScheduleDayPrev)
	b.bot.Handle(&b.btnScheduleDayNext, b.handleScheduleDayNext)
	b.bot.Handle(&b.btnScheduleMeetingInfo, b.handleScheduleMeetingInfo)
	b.bot.Handle(&b.btnScheduleBack, b.handleScheduleBack)
	b.bot.Handle(&b.btnScheduleNoop, b.handleScheduleNoop)

	b.bot.Handle(&b.btnTeamPick, b.handleTeamPick)
	b.bot.Handle(&b.btnTeamOther, b.handleTeamOther)
	b.bot.Handle(&b.btnRolePick, b.handleRolePick)
	b.bot.Handle(&b.btnRoleOther, b.handleRoleOther)

	b.bot.Handle(&b.btnAdminCalendar, b.handleAdminCalendar)
	b.bot.Handle(&b.btnAdminCalendarEdit, b.handleAdminCalendarEdit)
	b.bot.Handle(&b.btnAdminWorkHours, b.handleAdminWorkHours)
	b.bot.Handle(&b.btnAdminWorkHoursPick, b.handleAdminWorkHoursPick)
	b.bot.Handle(&b.btnAdminEmployees, b.handleAdminEmployees)
	b.bot.Handle(&b.btnAdminBack, b.handleAdminBack)
	b.bot.Handle(&b.btnAdminInputCancel, b.handleAdminInputCancel)

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
		if b.isAdmin(user.TelegramID) {
			if field, ok := b.getAdminInput(user.TelegramID); ok {
				return b.handleAdminInputText(c, user, field, text)
			}
		}
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
	return b.sendTeamOptions(c, "🏢 Выберите команду или отдел:")
}

func (b *Bot) askRole(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterRole); err != nil {
		return err
	}
	return b.sendRoleOptions(c, "💼 Выберите роль:")
}

func (b *Bot) askTimezone(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEnterTimezone); err != nil {
		return err
	}
	return b.sendTimezoneOptions(c, "🌍 Выберите часовой пояс.")
}

func (b *Bot) handleTimezoneOther(c tele.Context) error {
	defer c.Respond()
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
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEditTeam); err != nil {
		return err
	}
	return b.sendTeamOptions(c, "🏢 Выберите новую команду или отдел:")
}

func (b *Bot) handleEditRole(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEditRole); err != nil {
		return err
	}
	return b.sendRoleOptions(c, "💼 Выберите новую роль:")
}

func (b *Bot) handleEditTimezone(c tele.Context) error {
	defer c.Respond()
	user, err := b.currentUser(c)
	if err != nil {
		return err
	}
	if err := b.users.SetRegistrationStep(context.Background(), user.TelegramID, domain.RegistrationStepEditTimezone); err != nil {
		return err
	}
	return b.sendTimezoneOptions(c, "🌍 Выберите новый часовой пояс.")
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

func (b *Bot) handleMyMeetings(c tele.Context) error {
	user, err := b.currentUser(c)
	if err != nil {
		return b.handleStart(c)
	}
	return b.renderMyMeetings(c, user)
}

func (b *Bot) sendMainMenu(c tele.Context, user domain.User) error {
	count, err := b.todaysMeetingCount(context.Background(), user)
	if err != nil {
		count = 0
	}
	return c.Send(formatMainMenu(user, count), b.menuKeyboardFor(user))
}

func (b *Bot) todaysMeetingCount(ctx context.Context, user domain.User) (int, error) {
	loc := b.userLocation(user)
	now := b.clock.Now().In(loc)
	from := startOfDay(now)
	to := from.AddDate(0, 0, 1)

	meetings, err := b.listSchedule.Execute(ctx, from, to)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, meeting := range meetings {
		if meeting.CreatorID == user.ID || containsID(meeting.ParticipantIDs, user.ID) {
			count++
		}
	}
	return count, nil
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
