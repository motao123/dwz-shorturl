package service

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"dwz-admin/internal/config"

	"go.uber.org/zap"
)

// EmailService sends outbound mail (used by expiry reminders) over SMTP. Both
// implicit TLS (port 465) and STARTTLS (587) are supported.
type EmailService struct {
	cfg    config.SMTPConfig
	logger *zap.Logger
}

func NewEmailService(cfg config.SMTPConfig, logger *zap.Logger) *EmailService {
	return &EmailService{cfg: cfg, logger: logger}
}

// Enabled reports whether SMTP is configured.
func (s *EmailService) Enabled() bool {
	return s.cfg.Host != "" && s.cfg.User != "" && s.cfg.Password != ""
}

// Send delivers a UTF-8 HTML/text email to one recipient.
func (s *EmailService) Send(to, subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("smtp not configured")
	}
	host := s.cfg.Host
	port := s.cfg.Port
	if port == 0 {
		port = 465
	}
	fromName := s.cfg.FromName
	if fromName == "" {
		fromName = s.cfg.From
	}
	from := fmt.Sprintf("%s <%s>", fromName, s.cfg.From)

	header := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n",
		from, to, subject)
	msg := []byte(header + body)

	addr := fmt.Sprintf("%s:%d", host, port)
	var conn *tls.Conn
	var err error
	if s.cfg.SSL || port == 465 {
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: false})
		if err != nil {
			return err
		}
	} else {
		// STARTTLS path via net/smtp.SendMail (handles STARTTLS).
		return smtp.SendMail(addr, smtp.PlainAuth("", s.cfg.User, s.cfg.Password, host), s.cfg.From, []string{to}, msg)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Password, host)); err != nil {
		return err
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// maskEmail hides the middle of an address for logs.
func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at <= 1 {
		return e
	}
	return e[:1] + "***" + e[at:]
}
