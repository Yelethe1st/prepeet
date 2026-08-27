package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// bundleRubricSource answers the rubric exactly as the session's bundle
// pinned it: the pin's coordinates from the bundle document, the body from
// the registry BY DIGEST, so whatever is currently published cannot change
// what an old session is judged by. The cross-context read lives here in
// cmd, the one place allowed to see interview, content and evaluation
// together.
type bundleRubricSource struct {
	sessions *interview.Store
	registry *content.Store
}

// PinnedPolicy answers the model policy the same bundle pinned (EVL-07).
func (r bundleRubricSource) PinnedPolicy(ctx context.Context, ref evaluation.SessionRef) (evaluation.PolicyPin, error) {
	pin, err := r.pinned(ctx, ref, "model_policy")
	if err != nil {
		return evaluation.PolicyPin{}, err
	}
	return evaluation.PolicyPin{
		Reference: pin.Reference, Version: pin.Version,
		Digest: pin.Digest, Body: pin.Body,
	}, nil
}

func (r bundleRubricSource) PinnedRubric(ctx context.Context, ref evaluation.SessionRef) (evaluation.RubricPin, error) {
	pin, err := r.pinned(ctx, ref, "rubric")
	if err != nil {
		return evaluation.RubricPin{}, err
	}
	return evaluation.RubricPin{
		Reference: pin.Reference, Version: pin.Version,
		Digest: pin.Digest, Body: pin.Body,
	}, nil
}

// pinned reads one artifact type's pin from the session's own bundle and
// resolves its body by digest, so the published head is never consulted.
func (r bundleRubricSource) pinned(ctx context.Context, ref evaluation.SessionRef, artifactType string) (evaluation.RubricPin, error) {
	body, err := r.sessions.Bundle(ctx, ref.SessionID, ref.Mode, ref.CandidateID, ref.TenantID)
	if err != nil {
		return evaluation.RubricPin{}, fmt.Errorf("reading the session bundle: %w", err)
	}

	var bundle struct {
		PinnedInputs []struct {
			ArtifactType string `json:"artifact_type"`
			Reference    string `json:"reference"`
			Version      string `json:"version"`
			Digest       string `json:"digest"`
		} `json:"pinned_inputs"`
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
		return evaluation.RubricPin{}, fmt.Errorf("decoding the bundle: %w", err)
	}

	for _, pin := range bundle.PinnedInputs {
		if pin.ArtifactType != artifactType {
			continue
		}
		artifact, err := r.registry.GetByDigest(ctx, pin.Digest, ref.TenantID)
		if err != nil {
			return evaluation.RubricPin{}, fmt.Errorf("resolving the pinned rubric by digest: %w", err)
		}
		return evaluation.RubricPin{
			Reference: pin.Reference, Version: pin.Version,
			Digest: pin.Digest, Body: artifact.Body,
		}, nil
	}
	return evaluation.RubricPin{}, &evaluation.ExtractFailure{
		Code: "FAILURE_CODE_ARTIFACT_NOT_FOUND", Retryable: false,
		Message: "the session's bundle pins no " + artifactType,
	}
}
