// Package catalog owns the interview catalogue: disciplines, roles, shapes
// and personas, and which combinations of them a session can be built from.
//
// Implements CAT-03. The catalogue is content, not code: the document is a
// registry artifact (ADR-0011), authored in git, published through the same
// lifecycle as every other artifact, and resolved from the registry at
// serve time. Adding a profession is publishing a new version - nothing in
// this package knows a single discipline's name. What the package does own
// is coherence and validity: a document that contradicts itself is refused
// at parse, and a selection that combines entries the catalogue does not
// offer together is refused field by field, server-side, because a browser
// filtering options is a convenience and never the rule.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrIncoherent means the catalogue document contradicts itself and was
// refused before anything could serve it.
var ErrIncoherent = errors.New("catalog: the catalogue document is incoherent")

// Discipline is a profession the product serves.
type Discipline struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Shape is an interview format, with the lengths it can honestly run at.
type Shape struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Minutes     []int  `json:"minutes"`
}

// Persona is an interview style - pacing, follow-up pressure, silence -
// never a judgement about the candidate.
type Persona struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Style       string `json:"style"`
	Voice       string `json:"voice"`
	Description string `json:"description"`
	BestFor     string `json:"best_for"`
	// Shapes this persona runs. Empty means unrestricted, so adding a shape
	// does not require touching every persona.
	Shapes []string `json:"shapes"`
}

// Role is a catalogued position, in a discipline, with the shapes that make
// sense for it.
type Role struct {
	ID           string   `json:"id"`
	Discipline   string   `json:"discipline"`
	Title        string   `json:"title"`
	Organisation string   `json:"organisation"`
	Competencies []string `json:"competencies"`
	Shapes       []string `json:"shapes"`
}

// Catalogue is one parsed, coherent catalogue document.
type Catalogue struct {
	Disciplines []Discipline `json:"disciplines"`
	Shapes      []Shape      `json:"shapes"`
	Personas    []Persona    `json:"personas"`
	Roles       []Role       `json:"roles"`

	disciplines map[string]Discipline
	shapes      map[string]Shape
	personas    map[string]Persona
	roles       map[string]Role
}

// Selection is what a wizard submits: one entry from each collection.
type Selection struct {
	Discipline string
	Role       string
	Shape      string
	Minutes    int
	Persona    string
}

// FieldError is one refusal, aimed at the field that has to change.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// Parse decodes and coheres a catalogue document.
//
// Every cross-reference is checked here, once, so everything downstream can
// index without defending: a role in no discipline, a ghost shape, a shape
// with no runnable length or a duplicated identifier is a refusal at the
// door, which is where the registry's validating state runs this.
func Parse(body json.RawMessage) (Catalogue, error) {
	var parsed Catalogue
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Catalogue{}, fmt.Errorf("%w: %v", ErrIncoherent, err)
	}

	parsed.disciplines = map[string]Discipline{}
	for _, discipline := range parsed.Disciplines {
		if discipline.ID == "" || discipline.Name == "" {
			return Catalogue{}, fmt.Errorf("%w: a discipline is missing its id or name", ErrIncoherent)
		}
		if _, seen := parsed.disciplines[discipline.ID]; seen {
			return Catalogue{}, fmt.Errorf("%w: discipline %q appears twice", ErrIncoherent, discipline.ID)
		}
		parsed.disciplines[discipline.ID] = discipline
	}

	parsed.shapes = map[string]Shape{}
	for _, shape := range parsed.Shapes {
		if shape.ID == "" || shape.Name == "" {
			return Catalogue{}, fmt.Errorf("%w: a shape is missing its id or name", ErrIncoherent)
		}
		if _, seen := parsed.shapes[shape.ID]; seen {
			return Catalogue{}, fmt.Errorf("%w: shape %q appears twice", ErrIncoherent, shape.ID)
		}
		if len(shape.Minutes) == 0 {
			return Catalogue{}, fmt.Errorf("%w: shape %q offers no duration", ErrIncoherent, shape.ID)
		}
		parsed.shapes[shape.ID] = shape
	}

	parsed.personas = map[string]Persona{}
	for _, persona := range parsed.Personas {
		if persona.ID == "" || persona.Name == "" {
			return Catalogue{}, fmt.Errorf("%w: a persona is missing its id or name", ErrIncoherent)
		}
		if _, seen := parsed.personas[persona.ID]; seen {
			return Catalogue{}, fmt.Errorf("%w: persona %q appears twice", ErrIncoherent, persona.ID)
		}
		for _, shape := range persona.Shapes {
			if _, known := parsed.shapes[shape]; !known {
				return Catalogue{}, fmt.Errorf("%w: persona %q names shape %q, which does not exist", ErrIncoherent, persona.ID, shape)
			}
		}
		parsed.personas[persona.ID] = persona
	}

	parsed.roles = map[string]Role{}
	for _, role := range parsed.Roles {
		if role.ID == "" || role.Title == "" {
			return Catalogue{}, fmt.Errorf("%w: a role is missing its id or title", ErrIncoherent)
		}
		if _, seen := parsed.roles[role.ID]; seen {
			return Catalogue{}, fmt.Errorf("%w: role %q appears twice", ErrIncoherent, role.ID)
		}
		if _, known := parsed.disciplines[role.Discipline]; !known {
			return Catalogue{}, fmt.Errorf("%w: role %q belongs to discipline %q, which does not exist", ErrIncoherent, role.ID, role.Discipline)
		}
		if len(role.Shapes) == 0 {
			return Catalogue{}, fmt.Errorf("%w: role %q offers no interview shape", ErrIncoherent, role.ID)
		}
		for _, shape := range role.Shapes {
			if _, known := parsed.shapes[shape]; !known {
				return Catalogue{}, fmt.Errorf("%w: role %q names shape %q, which does not exist", ErrIncoherent, role.ID, shape)
			}
		}
		parsed.roles[role.ID] = role
	}

	return parsed, nil
}

// Validate refuses a selection the catalogue does not offer, field by field.
//
// Field by field rather than first-refusal, because the wizard shows one
// step per field and a person should learn everything wrong with a restored
// draft in one answer.
func (c Catalogue) Validate(selection Selection) []FieldError {
	var refusals []FieldError
	refuse := func(field, code, message string) {
		refusals = append(refusals, FieldError{Field: field, Code: code, Message: message})
	}

	if _, known := c.disciplines[selection.Discipline]; !known {
		refuse("discipline", "DISCIPLINE_UNKNOWN", "That discipline is not in the catalogue.")
	}

	role, roleKnown := c.roles[selection.Role]
	if !roleKnown {
		refuse("role", "ROLE_UNKNOWN", "That role is not in the catalogue.")
	} else if role.Discipline != selection.Discipline {
		refuse("role", "ROLE_OUTSIDE_DISCIPLINE", "That role is not part of the chosen discipline.")
	}

	shape, shapeKnown := c.shapes[selection.Shape]
	if !shapeKnown {
		refuse("shape", "SHAPE_UNKNOWN", "That interview shape is not in the catalogue.")
	} else {
		if roleKnown && !contains(role.Shapes, selection.Shape) {
			refuse("shape", "SHAPE_NOT_OFFERED", "The chosen role does not offer that interview shape.")
		}
		if !containsInt(shape.Minutes, selection.Minutes) {
			refuse("minutes", "DURATION_NOT_OFFERED", "That interview shape does not run at that length.")
		}
	}

	persona, personaKnown := c.personas[selection.Persona]
	switch {
	case !personaKnown:
		refuse("persona", "PERSONA_UNKNOWN", "That interviewer is not in the catalogue.")
	case len(persona.Shapes) > 0 && shapeKnown && !contains(persona.Shapes, selection.Shape):
		refuse("persona", "PERSONA_NOT_OFFERED", "That interviewer does not run the chosen interview shape.")
	}

	return refusals
}

func contains(list []string, value string) bool {
	for _, each := range list {
		if each == value {
			return true
		}
	}
	return false
}

func containsInt(list []int, value int) bool {
	for _, each := range list {
		if each == value {
			return true
		}
	}
	return false
}

// CompetencyID derives the stable identifier for a competency name: the
// lowercased, hyphenated slug. In this package because the catalogue owns
// the vocabulary, and two contexts deriving it differently would make the
// same competency two things.
func CompetencyID(name string) string {
	var out []rune
	lastHyphen := true
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out = append(out, r)
			lastHyphen = false
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
			lastHyphen = false
		default:
			if !lastHyphen {
				out = append(out, '-')
				lastHyphen = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}
