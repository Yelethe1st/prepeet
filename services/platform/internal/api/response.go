package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// The responses the generator cannot express.
//
// oapi-codegen models a response header as one field, and its visitor writes it
// with Header().Set, which replaces rather than appends. Login and refresh set
// two cookies with different paths, so there is no value of that single string
// that produces the right result: whichever cookie is written second is the only
// one the browser receives.
//
// The generated interfaces are still satisfied. A response object is anything
// with the right Visit method, so these types plug into the generated router
// exactly as the generated ones do, and the strict interface still fails to
// compile if a handler returns the wrong shape. Only the writing is ours.
//
// The alternative was to change the contract so it did not describe two
// cookies. That would have made the document lie about the wire in order to
// suit a generator, which is backwards: ADR-0004 makes the contract the source.

// sessionIssued is the 200 for both login and refresh: a session description in
// the body and the two cookies alongside it.
//
// One type serves both operations because the responses are identical. Two
// types would be two places to get the cookie flags right.
type sessionIssued struct {
	body        prepeetapi.Session
	environment config.Environment
	session     Session
}

func (r sessionIssued) write(w http.ResponseWriter) error {
	SetSessionCookies(w, r.environment,
		r.session.SessionToken, r.session.RefreshToken,
		r.session.ExpiresAt, r.session.RefreshExpires)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// A session description is about the person reading it and must never be
	// stored by an intermediary. The value comes from the same constant the
	// contract-comparison test reads, so this cannot drift from what the
	// document declares.
	w.Header().Set("Cache-Control", NoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(r.body)
}

func (r sessionIssued) VisitLoginResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r sessionIssued) VisitRefreshResponse(w http.ResponseWriter) error { return r.write(w) }

// A magic link and a one-time code end in exactly the session login issues,
// same cookies and same body, which is the point: the flows differ in proof,
// never in what a session is.
func (r sessionIssued) VisitConsumeMagicLinkResponse(w http.ResponseWriter) error { return r.write(w) }
func (r sessionIssued) VisitConsumeOTPResponse(w http.ResponseWriter) error       { return r.write(w) }

// oauthSessionIssued is the callback's answer: the same session, plus where
// the sign-in was started from.
//
// It embeds sessionIssued rather than repeating it, which is the point of
// IAM-08's third criterion: one place writes the cookies, so a session held by
// somebody who signed in with Google is indistinguishable from one held by
// somebody who typed a password, including to logout and to revocation. The
// destination rides alongside because the server cannot navigate a browser and
// a redirect from a fetch call would not be followed.
type oauthSessionIssued struct {
	sessionIssued
	redirectTo string
}

func (r oauthSessionIssued) VisitCompleteOAuthResponse(w http.ResponseWriter) error {
	SetSessionCookies(w, r.environment,
		r.session.SessionToken, r.session.RefreshToken,
		r.session.ExpiresAt, r.session.RefreshExpires)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", NoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(prepeetapi.OAuthSession{
		Session: r.body, RedirectTo: r.redirectTo,
	})
}

// sessionCleared is the 204 for logout and the cookie-clearing half of a
// rejected refresh.
type sessionCleared struct {
	environment config.Environment
}

func (r sessionCleared) VisitLogoutResponse(w http.ResponseWriter) error {
	ClearSessionCookies(w, r.environment)
	w.Header().Set("Cache-Control", NoStore)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// failure is the single error envelope, returned from any operation.
//
// It implements every response interface in the contract, so a handler can
// return one failure value whatever operation it is serving. Without this each
// operation needs its own error type per status, and the envelope gets built in
// a dozen places.
type failure struct {
	status    int
	code      string
	message   string
	retryable bool
	fields    []FieldError
	// clearCookies is set when the failure means the browser is holding a token
	// the server has already refused. Leaving it there makes every subsequent
	// request fail the same way, which the person experiences as the product
	// being broken rather than as being logged out.
	clearCookies bool
	environment  config.Environment
	// retryAfter, when set, becomes the Retry-After header. The same number
	// the body carries in the message, so the interface can show a countdown.
	retryAfter time.Duration
	// requestID is carried on the value rather than read from the writer,
	// because a ResponseWriter has no way back to the request that produced it.
	// The handler has both, so it puts the identifier here.
	requestID string
}

func (f failure) write(w http.ResponseWriter) error {
	if f.clearCookies {
		ClearSessionCookies(w, f.environment)
	}

	type fieldError struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type envelope struct {
		Error struct {
			Code        string       `json:"code"`
			Message     string       `json:"message"`
			Retryable   bool         `json:"retryable"`
			FieldErrors []fieldError `json:"field_errors"`
			RequestID   string       `json:"request_id"`
		} `json:"error"`
	}

	var body envelope
	body.Error.Code = f.code
	// Scrubbed on the way out. These messages are written by hand and are meant
	// to be safe, but this is the last point before text reaches a client and a
	// log, and the cost of the check is nothing.
	body.Error.Message = telemetry.Scrub(f.message)
	body.Error.Retryable = f.retryable
	body.Error.FieldErrors = make([]fieldError, 0, len(f.fields))
	for _, field := range f.fields {
		body.Error.FieldErrors = append(body.Error.FieldErrors, fieldError{
			Field:   field.Field,
			Code:    field.Code,
			Message: telemetry.Scrub(field.Message),
		})
	}
	body.Error.RequestID = f.requestID

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", NoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(f.retryAfter.Seconds()))))
	}
	w.WriteHeader(f.status)
	return json.NewEncoder(w).Encode(body)
}

func (f failure) VisitRegisterResponse(w http.ResponseWriter) error          { return f.write(w) }
func (f failure) VisitLoginResponse(w http.ResponseWriter) error             { return f.write(w) }
func (f failure) VisitLogoutResponse(w http.ResponseWriter) error            { return f.write(w) }
func (f failure) VisitRefreshResponse(w http.ResponseWriter) error           { return f.write(w) }
func (f failure) VisitGetCurrentUserResponse(w http.ResponseWriter) error    { return f.write(w) }
func (f failure) VisitListMembershipsResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitSetActiveTenantResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitRequestTokenEmailResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitConfirmEmailVerificationResponse(w http.ResponseWriter) error {
	return f.write(w)
}
func (f failure) VisitConfirmPasswordResetResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitConsumeMagicLinkResponse(w http.ResponseWriter) error     { return f.write(w) }
func (f failure) VisitConsumeOTPResponse(w http.ResponseWriter) error           { return f.write(w) }
func (f failure) VisitGetMySkillsResponse(w http.ResponseWriter) error          { return f.write(w) }
func (f failure) VisitGetMyReadinessResponse(w http.ResponseWriter) error       { return f.write(w) }
func (f failure) VisitGetTenantSettingsResponse(w http.ResponseWriter) error    { return f.write(w) }
func (f failure) VisitSaveTenantSettingsResponse(w http.ResponseWriter) error   { return f.write(w) }

// Compile-time proof that the hand-written responses satisfy the generated
// interfaces. Without these, a contract change that altered a Visit signature
// would surface as a confusing error at the return statement rather than here.
var (
	_ prepeetapi.LoginResponseObject           = sessionIssued{}
	_ prepeetapi.RefreshResponseObject         = sessionIssued{}
	_ prepeetapi.LogoutResponseObject          = sessionCleared{}
	_ prepeetapi.RegisterResponseObject        = failure{}
	_ prepeetapi.LoginResponseObject           = failure{}
	_ prepeetapi.LogoutResponseObject          = failure{}
	_ prepeetapi.RefreshResponseObject         = failure{}
	_ prepeetapi.GetCurrentUserResponseObject  = failure{}
	_ prepeetapi.ListMembershipsResponseObject = failure{}
	_ prepeetapi.SetActiveTenantResponseObject = failure{}
)
