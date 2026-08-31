package main

import (
	"context"
	"fmt"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// The catalogue behind progression's targeting port.
//
// PRG-05 needs to know what a session for a role could ask about at all,
// which is the catalogue's answer and not progression's. ADR-0005 forbids
// progression importing the catalogue, so the port is declared there and
// satisfied here: cmd is the one place allowed to see both contexts, and
// this file is nine lines of mapping precisely because everything else
// stays on its own side of the boundary.

// roleCompetencies resolves a role's competencies through the catalogue.
//
// Practice targeting only, which is why no tenant is passed: a practice
// candidate composes against the platform catalogue, and a recommendation
// built from an employer's overridden role list would be an employer
// shaping a candidate's private practice. The identifiers are derived with
// catalog.CompetencyID, the same derivation evaluation uses, because a
// recommendation naming competencies the observations do not is a
// recommendation that can never be shown to have been covered.
func roleCompetencies(catalogue *catalog.Service) progression.RoleCompetencies {
	return roleCompetencySource{catalogue: catalogue}
}

type roleCompetencySource struct {
	catalogue *catalog.Service
}

func (s roleCompetencySource) Competencies(ctx context.Context, roleID string) ([]string, error) {
	document, err := s.catalogue.Catalogue(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("resolving the catalogue: %w", err)
	}
	for _, role := range document.Roles {
		if role.ID != roleID {
			continue
		}
		competencies := make([]string, 0, len(role.Competencies))
		for _, name := range role.Competencies {
			competencies = append(competencies, catalog.CompetencyID(name))
		}
		return competencies, nil
	}
	// An unknown role answers nothing rather than an error, and targeting
	// refuses the empty list itself. A role that has been retired since a
	// session was configured is an ordinary state, not a fault.
	return nil, nil
}
