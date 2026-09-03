package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

/*
 * Reading a transcript is an event, not just a query.
 *
 * authorization-model.md is the one unambiguous requirement in this area:
 * "Reading transcript/audio is independently authorized and audited." The other
 * three mentions across it and data-classification.md say "auditable", "where
 * required" and "may be", which is why this covers transcript and audio and
 * claims nothing about the rest.
 *
 * The declaration lives in the contract for the reason CTR-01 put capabilities
 * there: a handler that decided for itself whether it was sensitive would be a
 * second source of truth, and the one that drifts is always the one nobody
 * re-reads. Recording it in the middleware rather than in each handler means a
 * new read path is audited by declaring itself, not by its author remembering.
 */

const sensitiveSessionID = "00000000-0000-7000-8000-0000000000e1"

// recordedRead is one audit row the middleware asked for.
type recordedRead struct {
	Actor   string
	Subject string
	Action  string
	Outcome string
}

type recordingAuditor struct {
	reads []recordedRead
	err   error
}

func (r *recordingAuditor) RecordSensitiveRead(_ context.Context, read api.SensitiveRead) error {
	r.reads = append(r.reads, recordedRead{
		Actor: read.ActorID, Subject: read.SubjectID,
		Action: read.Action, Outcome: read.Outcome,
	})
	return r.err
}

func serveWithAuditor(t *testing.T, auditor api.SensitiveReadAuditor) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:   &fakeIdentity{principal: api.Principal{UserID: progressionUser}},
		Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		Settings:    &stubSettings{},
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		SensitiveReads: auditor, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func TestReadingATranscriptIsAudited(t *testing.T) {
	auditor := &recordingAuditor{}
	handler := serveWithAuditor(t, auditor)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/"+sensitiveSessionID+"/transcript", nil)
	request.AddCookie(sessionCookie())
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if len(auditor.reads) != 1 {
		t.Fatalf("a transcript read produced %d audit rows", len(auditor.reads))
	}
	recorded := auditor.reads[0]
	if recorded.Actor != progressionUser {
		t.Fatalf("the row names %q rather than the reader", recorded.Actor)
	}
	if recorded.Subject != sensitiveSessionID {
		t.Fatalf("the row names %q rather than what was read", recorded.Subject)
	}
}

func TestAnOrdinaryReadIsNotAudited(t *testing.T) {
	// Auditing everything is the same as auditing nothing: a table where every
	// read appears cannot be scanned for the ones that matter, and the cost is
	// paid on every request.
	auditor := &recordingAuditor{}
	handler := serveWithAuditor(t, auditor)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/personas", nil)
	request.AddCookie(sessionCookie())
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if len(auditor.reads) != 0 {
		t.Fatalf("an ordinary read was audited: %+v", auditor.reads)
	}
}

func TestAnActorRefusedByTheHandlerIsAuditedAsDenied(t *testing.T) {
	// The interesting refusal: somebody who is signed in reaching for content
	// they cannot have. A session that is not theirs is not found rather than
	// forbidden, which is deliberate elsewhere in this API and is exactly the
	// attempt a reviewer searches this table for. It always has an actor.
	auditor := &recordingAuditor{}
	handler, err := api.NewServer(api.ServerConfig{
		Identity:   &fakeIdentity{principal: api.Principal{UserID: progressionUser}},
		Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{err: api.ErrSessionMissing},
		Members: &fakeMembers{}, Billing: &fakeBilling{}, Progression: &stubProgression{},
		Settings:    &stubSettings{},
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		SensitiveReads: auditor, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/"+sensitiveSessionID+"/transcript", nil)
	request.AddCookie(sessionCookie())
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if len(auditor.reads) != 1 {
		t.Fatalf("a refused transcript read produced %d audit rows", len(auditor.reads))
	}
	if auditor.reads[0].Outcome != "denied" {
		t.Fatalf("a refused read was recorded as %q", auditor.reads[0].Outcome)
	}
	if auditor.reads[0].Actor != progressionUser {
		t.Fatalf("the refusal does not name who was refused: %q", auditor.reads[0].Actor)
	}
}

func TestARequestWithNoSessionIsNotAudited(t *testing.T) {
	// It never reached the content. Unauthenticated traffic is described by the
	// request log and the rate limiter; putting it here would fill the audit
	// trail with probes while telling a reviewer nothing about who did what.
	//
	// It also keeps audit.events keyed: a policy admitting a row with no actor
	// decides nothing about who is asking, and PostgreSQL ORs permissive
	// policies, so one such policy re-opens the table. internal/isolation
	// caught exactly that when this was tried the other way.
	auditor := &recordingAuditor{}
	handler := serveWithAuditor(t, auditor)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/"+sensitiveSessionID+"/transcript", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", recorder.Code)
	}
	if len(auditor.reads) != 0 {
		t.Fatalf("an unauthenticated probe was audited: %+v", auditor.reads)
	}
}

func TestAFailureToAuditRefusesTheRead(t *testing.T) {
	// The whole point of an audit obligation is that the read does not happen
	// unrecorded. Serving the transcript anyway would make the audit advisory,
	// and an advisory audit is one that is missing exactly when somebody is
	// doing something they should not.
	auditor := &recordingAuditor{err: context.DeadlineExceeded}
	handler := serveWithAuditor(t, auditor)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/interviews/"+sensitiveSessionID+"/transcript", nil)
	request.AddCookie(sessionCookie())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatalf("the transcript was served though the audit failed: %s", recorder.Body.String())
	}
}

func TestTheServerRefusesToStartWithoutAnAuditor(t *testing.T) {
	// A declaration nothing can honour is worse than no declaration: the
	// contract would promise an audit the deployment cannot write. Refused at
	// construction rather than at the first read.
	_, err := api.NewServer(api.ServerConfig{
		Identity:   &fakeIdentity{},
		Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		Settings:    &stubSettings{},
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		Environment: config.EnvironmentLocal,
	})

	if err == nil {
		t.Fatal("a server serving sensitive reads was built with nowhere to record them")
	}
}
