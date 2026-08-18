package mailer

import (
	"context"
	"log/slog"
)

type Log struct {
	Logger *slog.Logger
}

func (m Log) SendVerification(_ context.Context, to, verificationURL string) error {
	m.Logger.Info("development verification email", "to", to, "verification_url", verificationURL)
	return nil
}
