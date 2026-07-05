package mail

import (
	"context"
	"log"
)

// LogMailer is a dev Mailer that logs the email link instead of sending.
type LogMailer struct {
	logger *log.Logger
}

func NewLogMailer(logger *log.Logger) *LogMailer {
	return &LogMailer{logger: logger}
}

func (m *LogMailer) SendVerification(_ context.Context, to, link string) error {
	m.logger.Printf("[mail] verification to=%s link=%s", to, link)
	return nil
}

func (m *LogMailer) SendPasswordReset(_ context.Context, to, link string) error {
	m.logger.Printf("[mail] password-reset to=%s link=%s", to, link)
	return nil
}
