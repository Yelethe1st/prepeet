package realtime

// The egress client: RTC-05's recorder, to ADR-0013.
//
// LiveKit's Egress service speaks Twirp over HTTP: a POST per method with
// a JSON body, authorized by the same HS256 JWT shape the room grants use,
// carrying the recorder's own claim (roomRecord) and nothing wider. Built
// on the standard library for the same reason the grants are: the shape is
// fixed by LiveKit's contract, and the value of the request is that it
// says exactly what we wrote.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// egressTokenTTL bounds the API call's credential: minted per request,
// good for the request.
const egressTokenTTL = time.Minute

// EgressConfig is where the egress API lives and where artifacts land.
type EgressConfig struct {
	// APIURL is the LiveKit server's HTTP address, such as
	// http://livekit:7880.
	APIURL string
	// S3 is the upload target egress writes into: the same bucket the
	// platform's object store reads back at finalization.
	S3 EgressS3
}

// EgressS3 mirrors LiveKit's S3 upload config.
type EgressS3 struct {
	AccessKey      string
	Secret         string
	Region         string
	Endpoint       string
	Bucket         string
	ForcePathStyle bool
}

// Egress starts and stops recordings.
type Egress struct {
	grants *Grants
	config EgressConfig
	client *http.Client
}

// NewEgress wires the client. The Grants' signing credentials authorize
// the API calls.
func NewEgress(grants *Grants, config EgressConfig) *Egress {
	return &Egress{grants: grants, config: config, client: &http.Client{Timeout: 15 * time.Second}}
}

// StartTrack begins an audio-only egress of one participant's published
// audio into the given storage key, answering LiveKit's egress id. The
// participant identity is the track name by our own convention: the agent
// publishes as "interviewer", the browser joins as the candidate id but
// publishes the track named "candidate".
func (e *Egress) StartTrack(ctx context.Context, roomName, track, storageKey string) (string, error) {
	request := map[string]any{
		"room_name":  roomName,
		"identity":   track,
		"audio_only": true,
		"file_outputs": []map[string]any{{
			"filepath": storageKey,
			"s3": map[string]any{
				"access_key":       e.config.S3.AccessKey,
				"secret":           e.config.S3.Secret,
				"region":           e.config.S3.Region,
				"endpoint":         e.config.S3.Endpoint,
				"bucket":           e.config.S3.Bucket,
				"force_path_style": e.config.S3.ForcePathStyle,
			},
		}},
	}
	var response struct {
		EgressID string `json:"egress_id"`
	}
	if err := e.call(ctx, "StartParticipantEgress", request, &response); err != nil {
		return "", err
	}
	if response.EgressID == "" {
		return "", fmt.Errorf("realtime: egress started without an id")
	}
	return response.EgressID, nil
}

// StopTrack ends one egress. An egress that already ended answers an
// error here; the caller's probe of the artifact is what decides, so the
// caller may ignore it.
func (e *Egress) StopTrack(ctx context.Context, egressID string) error {
	var response struct{}
	return e.call(ctx, "StopEgress", map[string]any{"egress_id": egressID}, &response)
}

// call POSTs one Twirp method with a fresh, narrowly-scoped token.
func (e *Egress) call(ctx context.Context, method string, body any, into any) error {
	token, err := e.grants.mintService(map[string]any{"roomRecord": true}, egressTokenTTL)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("realtime: encoding %s: %w", method, err)
	}
	url := strings.TrimRight(e.config.APIURL, "/") + "/twirp/livekit.Egress/" + method
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("realtime: building %s: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)

	response, err := e.client.Do(request)
	if err != nil {
		return fmt.Errorf("realtime: calling %s: %w", method, err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("realtime: %s answered %d: %s", method, response.StatusCode, payload)
	}
	if err := json.Unmarshal(payload, into); err != nil {
		return fmt.Errorf("realtime: decoding %s: %w", method, err)
	}
	return nil
}
