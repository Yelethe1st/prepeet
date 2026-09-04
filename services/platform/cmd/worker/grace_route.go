package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
	"github.com/Yelethe1st/prepeet/services/platform/platform/realtime"
)

// startGraceTimer turns a session_interrupted announcement into a running
// grace workflow: SES-06's expiry half.
//
// One timer per drop. The workflow id is the session and the attempt the
// announcement names, so a redelivered event joins the running timer and a
// later drop gets its own - each drop restarts the window, and only the
// timer whose deadline actually stands finalizes anything. An announcement
// that is not resumable carries no window to time and is handled by doing
// nothing, which for screening is exactly the human-decision boundary
// SCR-08 draws: the platform never re-invites on its own.
func startGraceTimer(workflows sdkclient.Client) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
			Resumable bool   `json:"resumable"`
			Attempt   int    `json:"attempt"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}
		if !payload.Resumable {
			return nil
		}

		_, err := workflows.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
			ID:                    fmt.Sprintf("grace-%s-%d", payload.SessionID, payload.Attempt),
			TaskQueue:             interview.TaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		}, interview.GraceWorkflow, interview.GraceInput{
			SessionID:   payload.SessionID,
			Mode:        event.Purpose,
			CandidateID: event.Actor.ID,
			TenantID:    event.TenantID,
			ActorID:     event.Actor.ID,
		})

		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return nil
		}
		return err
	}
}

// graceCompleter builds the completion path expiry runs: the same sealing
// the api wires for a candidate's own complete, so an expired session's
// evaluation input and media reconciliation are no lesser than a completed
// one's. Degrades exactly as the api does when storage or egress is not
// configured, because a seal recorded with an honest MEDIA_MISSING warning
// beats a worker that refuses to finalize anything.
func graceCompleter(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *interview.Completer {
	completer := interview.NewCompleter(interview.NewStore(pool))
	if cfg.S3Bucket == "" {
		return completer
	}
	documents, err := objectstore.NewS3Store(ctx, objectstore.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		log.Error("object storage is not usable for grace expiry", slog.String("error", err.Error()))
		os.Exit(1)
	}
	completer = completer.WithEvaluationInput(
		sealedInputWriter{store: documents},
		competencySource(catalog.NewService(registrySource{registry: content.NewStore(pool)})),
	)
	if cfg.LiveKitAPIURL != "" {
		grants, err := realtime.NewGrants(realtime.Config{
			URL: cfg.LiveKitURL, APIKey: cfg.LiveKitAPIKey, APISecret: cfg.LiveKitAPISecret,
		})
		if err != nil {
			log.Error("the egress signer is not usable for grace expiry", slog.String("error", err.Error()))
			os.Exit(1)
		}
		egress := realtime.NewEgress(grants, realtime.EgressConfig{
			APIURL: cfg.LiveKitAPIURL,
			S3: realtime.EgressS3{
				AccessKey: cfg.S3AccessKey, Secret: cfg.S3SecretKey,
				Region: cfg.S3Region, Endpoint: cfg.S3Endpoint,
				Bucket: cfg.S3Bucket, ForcePathStyle: cfg.S3UsePathStyle,
			},
		})
		completer = completer.WithMedia(egress, mediaProber{store: documents})
	}
	return completer
}

// sealedInputWriter stores the evaluation-input document at the seal, under
// the one shared key derivation - the worker's copy of the api's adapter,
// because each binary owns its own translation (ADR-0005).
type sealedInputWriter struct {
	store *objectstore.S3Store
}

func (w sealedInputWriter) PutSealedInput(ctx context.Context, session interview.Session, body []byte) (string, error) {
	key, err := objectstore.SealedInputKey(session.Mode, session.TenantID, session.CandidateID, session.ID)
	if err != nil {
		return "", err
	}
	if err := w.store.Put(ctx, key, body, "application/json"); err != nil {
		return "", err
	}
	return key.String(), nil
}

// competencySource resolves a session's competencies from its configured
// role through the catalogue: the one place allowed to see both contexts.
func competencySource(catalogue *catalog.Service) interview.CompetencySource {
	return func(ctx context.Context, session interview.Session) ([]interview.Competency, error) {
		var config struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(session.Config, &config); err != nil || config.Role == "" {
			// A session with no configured role has nothing to look for;
			// evaluation degrades to zero competencies rather than guessing.
			return []interview.Competency{}, nil
		}

		document, err := catalogue.Catalogue(ctx, session.TenantID)
		if err != nil {
			return nil, fmt.Errorf("resolving the catalogue: %w", err)
		}
		for _, role := range document.Roles {
			if role.ID != config.Role {
				continue
			}
			competencies := make([]interview.Competency, 0, len(role.Competencies))
			for _, name := range role.Competencies {
				competencies = append(competencies, interview.Competency{
					ID: catalog.CompetencyID(name), Name: name,
				})
			}
			return competencies, nil
		}
		return []interview.Competency{}, nil
	}
}

// registrySource narrows the content store to what the catalogue reads.
type registrySource struct {
	registry *content.Store
}

func (s registrySource) ResolveBody(ctx context.Context, reference, tenantID string) (json.RawMessage, string, error) {
	artifact, err := s.registry.Resolve(ctx, reference, tenantID)
	if err != nil {
		return nil, "", err
	}
	return artifact.Body, artifact.Digest, nil
}

// mediaProber reads artifacts back for reconciliation: the sealing path
// trusts what the bucket holds, never what any recorder claimed.
type mediaProber struct {
	store *objectstore.S3Store
}

func (p mediaProber) Stat(ctx context.Context, storageKey string) (int64, string, error) {
	key, err := objectstore.ParseKey(storageKey)
	if err != nil {
		return 0, "", err
	}
	return p.store.StatDigest(ctx, key)
}
