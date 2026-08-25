package api

import (
	"context"
	"net/http"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The catalogue surface: CAT-03 at the HTTP boundary. The collections come
// from the artifact registry through the port - a hardcoded list here would
// quietly restrict the product to whatever profession the deploy knew about,
// which is exactly what the ticket forbids.

// Catalog is what the API needs from the catalogue, declared here per
// ADR-0005 and wired in cmd.
type Catalog interface {
	// Catalogue answers the whole document for one tenant's view: the
	// platform's catalogue, unless the tenant has published an override.
	Catalogue(ctx context.Context, tenantID string) (CatalogueView, error)
}

// CatalogueView mirrors the catalogue at the port.
type CatalogueView struct {
	Disciplines []Discipline
	Roles       []CatalogRole
	Shapes      []InterviewShape
	Personas    []CatalogPersona
}

// Discipline is a profession the product serves.
type Discipline struct {
	ID   string
	Name string
}

// CatalogRole carries a role and its combination rules.
type CatalogRole struct {
	ID           string
	Discipline   string
	Title        string
	Organisation string
	Competencies []string
	Shapes       []string
}

// InterviewShape carries a format and its runnable lengths.
type InterviewShape struct {
	ID          string
	Name        string
	Description string
	Minutes     []int
}

// CatalogPersona carries an interviewer style.
type CatalogPersona struct {
	ID          string
	Name        string
	Style       string
	Voice       string
	Description string
	BestFor     string
	Shapes      []string
}

// catalog handles the /catalog operations.
type catalog struct {
	authentication *authentication
	source         Catalog
}

// catalogue authenticates and resolves the caller's view.
func (c *catalog) catalogue(ctx context.Context) (CatalogueView, *failure) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refused := c.authentication.rejectedSession(ctx)
		return CatalogueView{}, &refused
	}
	principal, err := c.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		refused := c.authentication.failed(ctx, err)
		return CatalogueView{}, &refused
	}

	view, err := c.source.Catalogue(ctx, principal.ActiveTenantID)
	if err != nil {
		refused := c.authentication.failed(ctx, err)
		return CatalogueView{}, &refused
	}
	return view, nil
}

// ListDisciplines answers the professions.
func (c *catalog) ListDisciplines(ctx context.Context, _ prepeetapi.ListDisciplinesRequestObject) (prepeetapi.ListDisciplinesResponseObject, error) {
	view, refused := c.catalogue(ctx)
	if refused != nil {
		return *refused, nil
	}

	body := prepeetapi.DisciplineList{Disciplines: make([]prepeetapi.Discipline, 0, len(view.Disciplines))}
	for _, discipline := range view.Disciplines {
		body.Disciplines = append(body.Disciplines, prepeetapi.Discipline{
			ID: discipline.ID, Name: discipline.Name,
		})
	}
	return prepeetapi.ListDisciplines200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListDisciplines200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListRoles answers the roles with their combination rules.
func (c *catalog) ListRoles(ctx context.Context, _ prepeetapi.ListRolesRequestObject) (prepeetapi.ListRolesResponseObject, error) {
	view, refused := c.catalogue(ctx)
	if refused != nil {
		return *refused, nil
	}

	body := prepeetapi.RoleList{Roles: make([]prepeetapi.CatalogRole, 0, len(view.Roles))}
	for _, role := range view.Roles {
		body.Roles = append(body.Roles, prepeetapi.CatalogRole{
			ID: role.ID, Discipline: role.Discipline, Title: role.Title,
			Organisation: role.Organisation,
			Competencies: role.Competencies, Shapes: role.Shapes,
		})
	}
	return prepeetapi.ListRoles200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListRoles200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListInterviewShapes answers the formats and their lengths.
func (c *catalog) ListInterviewShapes(ctx context.Context, _ prepeetapi.ListInterviewShapesRequestObject) (prepeetapi.ListInterviewShapesResponseObject, error) {
	view, refused := c.catalogue(ctx)
	if refused != nil {
		return *refused, nil
	}

	body := prepeetapi.ShapeList{Shapes: make([]prepeetapi.InterviewShape, 0, len(view.Shapes))}
	for _, shape := range view.Shapes {
		body.Shapes = append(body.Shapes, prepeetapi.InterviewShape{
			ID: shape.ID, Name: shape.Name, Description: shape.Description,
			Minutes: shape.Minutes,
		})
	}
	return prepeetapi.ListInterviewShapes200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListInterviewShapes200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListPersonas answers the interviewer styles.
func (c *catalog) ListPersonas(ctx context.Context, _ prepeetapi.ListPersonasRequestObject) (prepeetapi.ListPersonasResponseObject, error) {
	view, refused := c.catalogue(ctx)
	if refused != nil {
		return *refused, nil
	}

	body := prepeetapi.PersonaList{Personas: make([]prepeetapi.Persona, 0, len(view.Personas))}
	for _, persona := range view.Personas {
		body.Personas = append(body.Personas, prepeetapi.Persona{
			ID: persona.ID, Name: persona.Name, Style: persona.Style,
			Voice: persona.Voice, Description: persona.Description,
			BestFor: persona.BestFor, Shapes: persona.Shapes,
		})
	}
	return prepeetapi.ListPersonas200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListPersonas200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.ListDisciplinesResponseObject     = failure{}
	_ prepeetapi.ListRolesResponseObject           = failure{}
	_ prepeetapi.ListInterviewShapesResponseObject = failure{}
	_ prepeetapi.ListPersonasResponseObject        = failure{}
)

func (f failure) VisitListDisciplinesResponse(w http.ResponseWriter) error     { return f.write(w) }
func (f failure) VisitListRolesResponse(w http.ResponseWriter) error           { return f.write(w) }
func (f failure) VisitListInterviewShapesResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitListPersonasResponse(w http.ResponseWriter) error        { return f.write(w) }
