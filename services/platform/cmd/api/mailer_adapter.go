package main

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
)

// queueMailer presents the notification queue as the Mailer identity declared.
//
// The translation ADR-0005 costs, in the one place allowed to see both
// contexts. Each method maps plain arguments onto the template's typed
// variables, which is also where a new field in a template becomes a visible
// change to identity's contract rather than a silent one.
type queueMailer struct {
	queue *notification.Queue
}

var _ identity.Mailer = queueMailer{}

func (m queueMailer) SendEmailVerification(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient,
		notification.VerifyEmail{Link: link, ExpiresMinutes: expiresMinutes})
	return err
}

func (m queueMailer) SendPasswordReset(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient,
		notification.PasswordReset{Link: link, ExpiresMinutes: expiresMinutes})
	return err
}

func (m queueMailer) SendMagicLink(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient,
		notification.MagicLink{Link: link, ExpiresMinutes: expiresMinutes})
	return err
}

func (m queueMailer) SendOTP(ctx context.Context, tx pgx.Tx, recipient, code string, expiresMinutes int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient,
		notification.OTP{Code: code, ExpiresMinutes: expiresMinutes})
	return err
}
