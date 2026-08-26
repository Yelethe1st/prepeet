package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// sealedInputWriter stores the evaluation-input document at completion,
// under the one shared key derivation.
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
