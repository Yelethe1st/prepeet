package main

import (
	"strings"
	"testing"
)

// CTR-04's second criterion: a change that breaks a deployed client fails the
// build. Each break is proven by making it, and each thing that is safe is
// proven by making that too, because a gate that fires on safe changes is a
// gate people route around.

func parse(t *testing.T, body string) *Document {
	t.Helper()
	document, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return document
}

const base = `
openapi: 3.1.0
paths:
  /sessions:
    get:
      responses:
        "200":
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/SessionList"
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/NewSession"
      responses:
        "201":
          content:
            application/json:
              schema:
                type: object
                required: [id]
                properties:
                  id: { type: string }
components:
  schemas:
    SessionList:
      type: object
      required: [items, total]
      properties:
        items: { type: array }
        total: { type: integer }
    NewSession:
      type: object
      required: [mode]
      properties:
        mode:
          type: string
          enum: [practice, screening]
        note: { type: string }
`

func compare(t *testing.T, previous, current string) []Break {
	t.Helper()
	return Compare(parse(t, previous), parse(t, current))
}

func onlyBreak(t *testing.T, breaks []Break) Break {
	t.Helper()
	if len(breaks) != 1 {
		t.Fatalf("want exactly one break, got %d: %v", len(breaks), breaks)
	}
	return breaks[0]
}

func TestAnUnchangedDocumentBreaksNothing(t *testing.T) {
	if breaks := compare(t, base, base); len(breaks) != 0 {
		t.Fatalf("an unchanged document reported %d breaks: %v", len(breaks), breaks)
	}
}

func TestRemovingAPathIsABreak(t *testing.T) {
	current := strings.Replace(base, "  /sessions:", "  /other:", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, "404") {
		t.Fatalf("the reason does not say what a client sees: %s", got)
	}
	if got.Remedy == "" {
		t.Fatal("a break with no remedy is a gate people route around")
	}
}

func TestRemovingAnOperationIsABreak(t *testing.T) {
	current := strings.Replace(base, "    post:\n", "    x-removed:\n", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Where, "POST /sessions") {
		t.Fatalf("the break does not name the operation: %s", got)
	}
}

func TestMakingARequestPropertyRequiredIsABreak(t *testing.T) {
	current := strings.Replace(base, "      required: [mode]", "      required: [mode, note]", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, `"note" is now required`) {
		t.Fatalf("the break does not name the property: %s", got)
	}
}

func TestRemovingARequestEnumValueIsABreak(t *testing.T) {
	current := strings.Replace(base, "enum: [practice, screening]", "enum: [practice]", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, "screening") {
		t.Fatalf("the break does not name the value: %s", got)
	}
}

func TestRemovingAGuaranteedResponsePropertyIsABreak(t *testing.T) {
	current := strings.Replace(base, "        total: { type: integer }\n", "", 1)
	current = strings.Replace(current, "required: [items, total]", "required: [items]", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, `"total" was removed`) {
		t.Fatalf("the break does not name the property: %s", got)
	}
}

// Still sent, but no longer promised. A client that reads it will one day find
// nothing, which is the same break arriving later.
func TestDroppingAResponsePropertysGuaranteeIsABreak(t *testing.T) {
	current := strings.Replace(base, "required: [items, total]", "required: [items]", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, "no longer guaranteed") {
		t.Fatalf("the break does not say what changed: %s", got)
	}
}

func TestChangingAPropertysTypeIsABreak(t *testing.T) {
	current := strings.Replace(base, "        total: { type: integer }", "        total: { type: string }", 1)

	got := onlyBreak(t, compare(t, base, current))

	if !strings.Contains(got.Reason, "integer to string") {
		t.Fatalf("the break does not say what it changed to: %s", got)
	}
}

func TestRemovingASuccessResponseIsABreak(t *testing.T) {
	current := strings.Replace(base, `        "201":`, `        "202":`, 1)

	breaks := compare(t, base, current)

	if len(breaks) == 0 {
		t.Fatal("a removed success response was not reported")
	}
	if !strings.Contains(breaks[0].Reason, "201") {
		t.Fatalf("the break does not name the status: %v", breaks)
	}
}

// The safe half. A gate that fires on these is one people learn to ignore, so
// each is asserted rather than assumed.
func TestAdditiveChangesAreNotBreaks(t *testing.T) {
	for name, current := range map[string]string{
		"a new path": base + `
  /extra:
    get:
      responses:
        "200":
          content:
            application/json:
              schema: { type: object }
`,
		"a new optional request property": strings.Replace(base,
			"        note: { type: string }",
			"        note: { type: string }\n        extra: { type: string }", 1),
		"a new response property": strings.Replace(base,
			"        total: { type: integer }",
			"        total: { type: integer }\n        cursor: { type: string }", 1),
		"a new request enum value": strings.Replace(base,
			"enum: [practice, screening]", "enum: [practice, screening, rehearsal]", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if breaks := compare(t, base, current); len(breaks) != 0 {
				t.Fatalf("%s was reported as breaking: %v", name, breaks)
			}
		})
	}
}

// Two runs over the same pair report in the same order, which matters when the
// output is read in a CI log next to a diff.
func TestTheReportIsStablyOrdered(t *testing.T) {
	current := strings.Replace(base, "required: [items, total]", "required: [items]", 1)
	current = strings.Replace(current, "      required: [mode]", "      required: [mode, note]", 1)

	first := Compare(parse(t, base), parse(t, current))
	second := Compare(parse(t, base), parse(t, current))

	if len(first) != 2 {
		t.Fatalf("want both breaks, got %v", first)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("the order changed between runs: %v then %v", first, second)
		}
	}
}
