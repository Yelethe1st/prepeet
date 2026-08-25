package main

import (
	"context"
	"encoding/json"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
)

// catalogAdapter presents the catalogue as the port the API declared:
// ADR-0005's translation, in the one place allowed to see catalog, content
// and api together.
type catalogAdapter struct {
	service *catalog.Service
}

func newCatalogAdapter(registry *content.Store) catalogAdapter {
	return catalogAdapter{service: catalog.NewService(registrySource{registry: registry})}
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

func (a catalogAdapter) Catalogue(ctx context.Context, tenantID string) (api.CatalogueView, error) {
	parsed, err := a.service.Catalogue(ctx, tenantID)
	if err != nil {
		return api.CatalogueView{}, err
	}

	view := api.CatalogueView{}
	for _, discipline := range parsed.Disciplines {
		view.Disciplines = append(view.Disciplines, api.Discipline(discipline))
	}
	for _, role := range parsed.Roles {
		view.Roles = append(view.Roles, api.CatalogRole{
			ID: role.ID, Discipline: role.Discipline, Title: role.Title,
			Organisation: role.Organisation,
			Competencies: role.Competencies, Shapes: role.Shapes,
		})
	}
	for _, shape := range parsed.Shapes {
		view.Shapes = append(view.Shapes, api.InterviewShape{
			ID: shape.ID, Name: shape.Name, Description: shape.Description,
			Minutes: shape.Minutes,
		})
	}
	for _, persona := range parsed.Personas {
		view.Personas = append(view.Personas, api.CatalogPersona{
			ID: persona.ID, Name: persona.Name, Style: persona.Style,
			Voice: persona.Voice, Description: persona.Description,
			BestFor: persona.BestFor, Shapes: persona.Shapes,
		})
	}
	return view, nil
}
