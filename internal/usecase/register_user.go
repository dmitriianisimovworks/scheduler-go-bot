package usecase

import "meeting-bot/internal/repository"

type RegisterUser struct {
	repo repository.UserRepository
}

func NewRegisterUser(repo repository.UserRepository) RegisterUser {
	return RegisterUser{repo: repo}
}
