package logger

import "log"

type Logger struct {
	env string
}

func New(env string) *Logger {
	return &Logger{env: env}
}

func (l *Logger) Printf(format string, args ...any) {
	log.Printf(format, args...)
}
