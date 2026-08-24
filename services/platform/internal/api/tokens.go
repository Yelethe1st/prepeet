package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The token flow endpoints: IAM-02 at the HTTP boundary.
//
// The request half answers 202 whatever the address, because the response must
// not say which addresses hold accounts. The consume half does the opposite
// and names the exact outcome, because everyone the difference is visible to
// is already holding the link, and the prototype gives each outcome its own
// screen.

// CooldownError refuses a resend at the API boundary, carrying the wait.
type CooldownError struct {
	RetryAfter time.Duration
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("api: another email was sent moments ago; wait %s", e.RetryAfter)
}

// tokenEmailKinds is the contract's enum, enforced here because the generated
// decoder types the field as a string and checks nothing. Discovered by a test
// requesting a carrier_pigeon email and receiving 202.
var tokenEmailKinds = map[prepeetapi.TokenEmailKind]bool{
	prepeetapi.VerifyEmail:   true,
	prepeetapi.PasswordReset: true,
	prepeetapi.MagicLink:     true,
	prepeetapi.Otp:           true,
}

// RequestTokenEmail sends one of the four token emails.
func (a *authentication) RequestTokenEmail(ctx context.Context, request prepeetapi.RequestTokenEmailRequestObject) (prepeetapi.RequestTokenEmailResponseObject, error) {
	if !tokenEmailKinds[request.Body.Kind] {
		return a.failed(ctx, Invalid("kind", "KIND_INVALID", "that is not a token email kind")), nil
	}

	if err := a.identity.RequestTokenEmail(ctx, string(request.Body.Kind), string(request.Body.Email)); err != nil {
		return a.failed(ctx, err), nil
	}

	return prepeetapi.RequestTokenEmail202JSONResponse{
		Body:    prepeetapi.TokenEmailAccepted{Status: prepeetapi.EmailSent},
		Headers: prepeetapi.RequestTokenEmail202ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ConfirmEmailVerification consumes a verification link.
func (a *authentication) ConfirmEmailVerification(ctx context.Context, request prepeetapi.ConfirmEmailVerificationRequestObject) (prepeetapi.ConfirmEmailVerificationResponseObject, error) {
	if err := a.identity.ConfirmEmailVerification(ctx, request.Body.Token); err != nil {
		return a.failed(ctx, err), nil
	}
	return prepeetapi.ConfirmEmailVerification204Response{
		Headers: prepeetapi.ConfirmEmailVerification204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ConfirmPasswordReset consumes a recovery link and sets the new password.
func (a *authentication) ConfirmPasswordReset(ctx context.Context, request prepeetapi.ConfirmPasswordResetRequestObject) (prepeetapi.ConfirmPasswordResetResponseObject, error) {
	if err := a.identity.ConfirmPasswordReset(ctx, request.Body.Token, request.Body.Password); err != nil {
		return a.failed(ctx, err), nil
	}
	return prepeetapi.ConfirmPasswordReset204Response{
		Headers: prepeetapi.ConfirmPasswordReset204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ConsumeMagicLink signs the holder of a sign-in link in.
func (a *authentication) ConsumeMagicLink(ctx context.Context, request prepeetapi.ConsumeMagicLinkRequestObject) (prepeetapi.ConsumeMagicLinkResponseObject, error) {
	session, err := a.identity.ConsumeMagicLink(ctx, request.Body.Token)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	issued, err := a.issued(session)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return issued, nil
}

// ConsumeOTP exchanges an emailed code for a session.
func (a *authentication) ConsumeOTP(ctx context.Context, request prepeetapi.ConsumeOTPRequestObject) (prepeetapi.ConsumeOTPResponseObject, error) {
	session, err := a.identity.ConsumeOTP(ctx, string(request.Body.Email), request.Body.Code)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	issued, err := a.issued(session)
	if err != nil {
		return a.failed(ctx, err), nil
	}
	return issued, nil
}

// tokenFailure maps a token outcome onto its response, or reports that the
// error is not a token outcome.
//
// 422 rather than 400 or 404: the request was well formed and the resource
// exists as an endpoint; it is the presented token that cannot be processed,
// and each code says exactly why because each has its own screen.
func tokenFailure(err error) (status int, code string, message string, ok bool) {
	switch {
	case errors.Is(err, ErrTokenInvalid):
		return http.StatusUnprocessableEntity, string(prepeetapi.TOKENINVALID),
			"That link is not valid. Check it was copied completely, or request a new one.", true
	case errors.Is(err, ErrTokenExpired):
		return http.StatusUnprocessableEntity, string(prepeetapi.TOKENEXPIRED),
			"That link has expired. Request a new one.", true
	case errors.Is(err, ErrTokenUsed):
		return http.StatusUnprocessableEntity, string(prepeetapi.TOKENUSED),
			"That link has already been used. Nothing further is needed.", true
	case errors.Is(err, ErrTokenSuperseded):
		return http.StatusUnprocessableEntity, string(prepeetapi.TOKENSUPERSEDED),
			"A newer email has replaced that link. Use the most recent one.", true
	case errors.Is(err, ErrCodeIncorrect):
		return http.StatusUnprocessableEntity, string(prepeetapi.CODEINCORRECT),
			"That code is not right.", true
	case errors.Is(err, ErrCodeExhausted):
		return http.StatusUnprocessableEntity, string(prepeetapi.CODEATTEMPTSEXHAUSTED),
			"Too many wrong codes. Request a new one.", true
	}
	return 0, "", "", false
}
