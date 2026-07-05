package mail

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPMailer sends real email via SMTP. Used in production.
type SMTPMailer struct {
	addr string // host:port
	auth smtp.Auth
	from string
}

func NewSMTPMailer(host, port, user, password, from string) *SMTPMailer {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}
	return &SMTPMailer{addr: host + ":" + port, auth: auth, from: from}
}

func (m *SMTPMailer) send(to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", m.from, to, subject, body)
	if err := smtp.SendMail(m.addr, m.auth, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("mail: smtp send: %w", err)
	}
	return nil
}

func (m *SMTPMailer) SendVerification(_ context.Context, to, link string) error {
	return m.send(to, "Verify your email", "Confirm your email: "+link)
}

func (m *SMTPMailer) SendPasswordReset(_ context.Context, to, link string) error {
	return m.send(to, "Reset your password", "Reset your password: "+link)
}

func (m *SMTPMailer) SendWorkspaceInvite(_ context.Context, to, workspaceName, link string) error {
	return m.send(to, "You're invited to "+workspaceName, "Join "+workspaceName+": "+link)
}
