package mailer

import "context"

type Mailer interface {
	SendVerification(context.Context, string, string) error
}
