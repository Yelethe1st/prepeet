package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads an OpenAPI document.
func Load(path string) (*Document, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("apicompat: reading %s: %w", path, err)
	}
	return Parse(body)
}

// Parse reads a document from bytes, which is what the tests use.
func Parse(body []byte) (*Document, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("apicompat: parsing the document: %w", err)
	}
	return &Document{raw: raw}, nil
}

func (d *Document) paths() map[string]map[string]any {
	found := map[string]map[string]any{}
	raw, ok := mapAt(d.raw, "paths")
	if !ok {
		return found
	}
	for path, item := range raw {
		if asMap, ok := item.(map[string]any); ok {
			found[path] = asMap
		}
	}
	return found
}

// resolve follows a local $ref to the schema it names.
//
// Bounded, because a schema defined in terms of itself would otherwise hang
// the gate rather than fail it, and a hang in CI is far harder to diagnose
// than a failure. Only local component references are followed: a remote one
// is not something this document uses, and fetching it would make the gate
// depend on the network.
func (d *Document) resolve(schema map[string]any) map[string]any {
	for depth := 0; depth < 8; depth++ {
		if schema == nil {
			return nil
		}
		ref, ok := schema["$ref"].(string)
		if !ok {
			return schema
		}
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(ref, prefix) {
			return nil
		}
		components, ok := mapAt(d.raw, "components")
		if !ok {
			return nil
		}
		schemas, ok := mapAt(components, "schemas")
		if !ok {
			return nil
		}
		next, ok := mapAt(schemas, strings.TrimPrefix(ref, prefix))
		if !ok {
			return nil
		}
		schema = next
	}
	return nil
}

// bodySchema digs out the application/json schema of a request or a response.
func bodySchema(node map[string]any) map[string]any {
	content, ok := mapAt(node, "content")
	if !ok {
		if body, has := mapAt(node, "requestBody"); has {
			content, ok = mapAt(body, "content")
		}
		if !ok {
			return nil
		}
	}
	media, ok := mapAt(content, "application/json")
	if !ok {
		return nil
	}
	schema, ok := mapAt(media, "schema")
	if !ok {
		return nil
	}
	return schema
}

// properties reads a schema's properties, resolving each one's reference so a
// type change behind a $ref is visible.
func properties(document *Document, schema map[string]any) map[string]map[string]any {
	found := map[string]map[string]any{}
	raw, ok := mapAt(schema, "properties")
	if !ok {
		return found
	}
	for name, property := range raw {
		asMap, ok := property.(map[string]any)
		if !ok {
			continue
		}
		if resolved := document.resolve(asMap); resolved != nil {
			found[name] = resolved
		}
	}
	return found
}

func requiredNames(schema map[string]any) map[string]bool {
	names := map[string]bool{}
	list, ok := schema["required"].([]any)
	if !ok {
		return names
	}
	for _, name := range list {
		if asString, ok := name.(string); ok {
			names[asString] = true
		}
	}
	return names
}

// removedEnumValues are the values the previous version accepted and this one
// does not. A schema that never had an enum accepted anything, and narrowing
// from "anything" to a list is itself a break, reported as the values that
// were implicitly allowed cannot be enumerated: that case shows up instead as
// a type change or is caught in review.
func removedEnumValues(before, after map[string]any) []string {
	beforeValues, ok := before["enum"].([]any)
	if !ok {
		return nil
	}
	afterValues, _ := after["enum"].([]any)
	still := map[string]bool{}
	for _, value := range afterValues {
		still[fmt.Sprint(value)] = true
	}
	if len(afterValues) == 0 {
		// Widened to anything, which accepts everything it used to.
		return nil
	}

	removed := []string{}
	for _, value := range beforeValues {
		if !still[fmt.Sprint(value)] {
			removed = append(removed, fmt.Sprint(value))
		}
	}
	sort.Strings(removed)
	return removed
}

func changedType(before, after map[string]any) (string, string, bool) {
	from, _ := before["type"].(string)
	to, _ := after["type"].(string)
	if from == "" || to == "" || from == to {
		return "", "", false
	}
	return from, to, true
}

func mapAt(node map[string]any, key string) (map[string]any, bool) {
	if node == nil {
		return nil, false
	}
	value, ok := node[key].(map[string]any)
	return value, ok
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
