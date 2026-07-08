package domain

import "time"

const (
	RegistrationStepConfirmName   = "confirm_name"
	RegistrationStepEnterName     = "enter_name"
	RegistrationStepEnterTeam     = "enter_team"
	RegistrationStepEnterRole     = "enter_role"
	RegistrationStepEnterTimezone = "enter_timezone"
	RegistrationStepEditName      = "edit_name"
	RegistrationStepEditTeam      = "edit_team"
	RegistrationStepEditRole      = "edit_role"
	RegistrationStepEditTimezone  = "edit_timezone"
	RegistrationStepComplete      = "complete"
)

type User struct {
	ID               int64
	TelegramID       int64
	Username         string
	FirstName        string
	LastName         string
	FullName         string
	DisplayName      string
	Team             string
	Role             string
	Timezone         string
	RegistrationStep string
	RegisteredAt     time.Time
	LastSeenAt       time.Time
}

func (u User) IsRegistered() bool {
	return u.RegistrationStep == RegistrationStepComplete
}
