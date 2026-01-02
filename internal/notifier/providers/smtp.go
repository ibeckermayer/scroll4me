package providers

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPSender sends emails via SMTP
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
}

// NewSMTPSender creates a new SMTP sender
func NewSMTPSender(host string, port int, username, password, from string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

// Send sends an email via SMTP
func (s *SMTPSender) Send(to, subject, htmlBody, plainBody string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Build MIME message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", s.from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/alternative; boundary=\"boundary42\"\r\n")
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString("--boundary42\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainBody)
	msg.WriteString("\r\n")

	// HTML part
	msg.WriteString("--boundary42\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")

	msg.WriteString("--boundary42--\r\n")

	// Authenticate and send
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	err := smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
