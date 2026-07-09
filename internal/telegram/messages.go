package telegram

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"meeting-bot/internal/domain"
)

func formatMainMenu(user domain.User, todaysMeetingCount int) string {
	name := valueOrDash(user.DisplayName)
	team := valueOrDash(user.Team)
	role := valueOrDash(user.Role)
	timezone := valueOrDash(user.Timezone)

	return fmt.Sprintf(
		"👋 %s, рабочее меню\n\nКоманда: %s\nРоль: %s\nЧасовой пояс: %s\nСегодня встреч: %d",
		name,
		team,
		role,
		timezone,
		todaysMeetingCount,
	)
}

func formatProfile(user domain.User) string {
	return fmt.Sprintf(
		"👤 Профиль\n\nИмя: %s\nКоманда: %s\nРоль: %s\nЧасовой пояс: %s",
		valueOrDash(user.DisplayName),
		valueOrDash(user.Team),
		valueOrDash(user.Role),
		valueOrDash(user.Timezone),
	)
}

func valueOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "не указано"
	}
	return value
}

func timeLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("timezone is empty")
	}
	return time.LoadLocation(name)
}
