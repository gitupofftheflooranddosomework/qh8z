package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Address  string
	Host     string
	Username string
	Password string
	From     string
}

type SMTP struct {
	Config SMTPConfig
}

func (m SMTP) SendVerification(ctx context.Context, to, verificationURL string) error {
	cfg := m.Config
	if cfg.Address == "" || cfg.Host == "" || cfg.From == "" {
		return errors.New("SMTP address, host, and from address are required")
	}
	if strings.ContainsAny(to+cfg.From, "\r\n") {
		return errors.New("invalid email address")
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("dial SMTP: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not advertise STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("start SMTP TLS: %w", err)
	}
	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	body := "Verify your qh8z email by opening this link:\r\n\r\n" + verificationURL + "\r\n\r\nIf you did not create a qh8z account, you can ignore this email.\r\n"
	msg := "From: qh8z <" + cfg.From + ">\r\nTo: " + to + "\r\nSubject: Verify your qh8z email\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP quit: %w", err)
	}
	return nil
}
