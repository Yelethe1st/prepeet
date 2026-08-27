package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The egress client, from the outside: what it actually puts on the wire.
//
// A recorder is authority to write a room's audio into our bucket, so the
// assertions here are the ones a reviewer would want: the method it calls,
// the claim it carries, the participant it names, and where the artifact
// is told to land. A failure is surfaced with the server's own words
// rather than swallowed, because a silent egress failure is a session
// that believes it is being recorded and is not.

func testGrants(t *testing.T) *Grants {
	t.Helper()
	grants, err := NewGrants(Config{
		URL: "wss://rtc.example", APIKey: "devkey", APISecret: "devsecret-devsecret-devsecret",
	})
	if err != nil {
		t.Fatalf("grants: %v", err)
	}
	return grants
}

// claimsOf decodes the payload of the JWT the client sent.
func claimsOf(t *testing.T, header string) map[string]any {
	t.Helper()
	token := strings.TrimPrefix(header, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("authorization is not a JWT: %q", header)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("claims: %v", err)
	}
	return claims
}

func TestStartTrackAsksForOneParticipantsAudioIntoItsOwnKey(t *testing.T) {
	var path string
	var body map[string]any
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		authorization = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"egress_id":"eg-1"}`))
	}))
	defer server.Close()

	egress := NewEgress(testGrants(t), EgressConfig{
		APIURL: server.URL,
		S3: EgressS3{
			AccessKey: "ak", Secret: "sk", Region: "eu-west-2",
			Endpoint: "http://minio:9000", Bucket: "prepeet", ForcePathStyle: true,
		},
	})

	id, err := egress.StartTrack(context.Background(), "ses-1", "interviewer", "candidate/u/session/s/media/interviewer.webm")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if id != "eg-1" {
		t.Fatalf("egress id = %q", id)
	}
	if path != "/twirp/livekit.Egress/StartParticipantEgress" {
		t.Fatalf("path = %q", path)
	}
	if body["room_name"] != "ses-1" || body["identity"] != "interviewer" || body["audio_only"] != true {
		t.Fatalf("body = %+v", body)
	}
	outputs, ok := body["file_outputs"].([]any)
	if !ok || len(outputs) != 1 {
		t.Fatalf("file outputs = %+v", body["file_outputs"])
	}
	output, _ := outputs[0].(map[string]any)
	if output["filepath"] != "candidate/u/session/s/media/interviewer.webm" {
		t.Fatalf("filepath = %v", output["filepath"])
	}
	s3, _ := output["s3"].(map[string]any)
	if s3["bucket"] != "prepeet" || s3["region"] != "eu-west-2" || s3["force_path_style"] != true {
		t.Fatalf("s3 = %+v", s3)
	}

	// The credential is narrow and short: the recorder's own claim, and
	// nothing that would let it join or publish.
	claims := claimsOf(t, authorization)
	video, _ := claims["video"].(map[string]any)
	if video["roomRecord"] != true {
		t.Fatalf("video claims = %+v", video)
	}
	for _, wider := range []string{"roomJoin", "canPublish", "roomAdmin"} {
		if _, present := video[wider]; present {
			t.Fatalf("the recorder's token carries %q", wider)
		}
	}
	expiry, _ := claims["exp"].(float64)
	issued, _ := claims["nbf"].(float64)
	if window := time.Duration(expiry-issued) * time.Second; window > 2*time.Minute {
		t.Fatalf("the recorder's token lives %s", window)
	}
}

func TestStopTrackNamesTheEgress(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	egress := NewEgress(testGrants(t), EgressConfig{APIURL: server.URL})
	if err := egress.StopTrack(context.Background(), "eg-1"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if path != "/twirp/livekit.Egress/StopEgress" || body["egress_id"] != "eg-1" {
		t.Fatalf("path %q body %+v", path, body)
	}
}

func TestAServerRefusalIsSurfacedWithItsOwnWords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"not_found","msg":"room does not exist"}`))
	}))
	defer server.Close()

	egress := NewEgress(testGrants(t), EgressConfig{APIURL: server.URL})
	_, err := egress.StartTrack(context.Background(), "ses-1", "candidate", "k")
	if err == nil {
		t.Fatal("a refused egress reported success")
	}
	if !strings.Contains(err.Error(), "room does not exist") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("the refusal lost its words: %v", err)
	}
}

func TestAnEgressWithoutAnIdIsRefusedRatherThanRecorded(t *testing.T) {
	// A response the client cannot act on must fail here: a track row
	// carrying an empty egress id could never be stopped.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	egress := NewEgress(testGrants(t), EgressConfig{APIURL: server.URL})
	if _, err := egress.StartTrack(context.Background(), "ses-1", "candidate", "k"); err == nil {
		t.Fatal("an egress with no id was accepted")
	}
}

func TestAnUnreachableServerFailsRatherThanPretending(t *testing.T) {
	egress := NewEgress(testGrants(t), EgressConfig{APIURL: "http://127.0.0.1:1"})
	if err := egress.StopTrack(context.Background(), "eg-1"); err == nil {
		t.Fatal("an unreachable egress server reported success")
	}
}
