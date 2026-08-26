// Command authzgen generates the capability catalogue from its contract.
//
// The contract at packages/contracts/authz/capabilities.yaml is the source, per
// ADR-0004, and this writes the Go and TypeScript that must agree with it. A
// capability therefore cannot exist in one language and not another, and a
// requirement cannot be changed in code without changing the document that
// describes it to whoever has to review it.
//
// Run through `make generate`, which runs from the repository root, because
// output paths resolve against the working directory rather than against this
// file.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// contract is the shape of capabilities.yaml.
type contract struct {
	Version      int               `yaml:"version"`
	Requirements map[string]string `yaml:"requirements"`
	Scopes       []string          `yaml:"scopes"`
	Capabilities []capabilityEntry `yaml:"capabilities"`
	Roles        []roleEntry       `yaml:"roles"`
	Unbundled    []string          `yaml:"unbundled"`
}

// roleEntry is one bundle of capabilities.
type roleEntry struct {
	Name string `yaml:"name"`
	// Membership is false for the bundle somebody holds with no tenant at all.
	Membership   bool     `yaml:"membership"`
	Reason       string   `yaml:"reason"`
	Capabilities []string `yaml:"capabilities"`
}

type capabilityEntry struct {
	Name       string `yaml:"name"`
	Tenant     bool   `yaml:"tenant"`
	Scope      string `yaml:"scope"`
	Owner      bool   `yaml:"owner"`
	Platform   bool   `yaml:"platform"`
	Privileged bool   `yaml:"privileged"`
	StepUp     bool   `yaml:"step_up"`
	Reason     string `yaml:"reason"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "authzgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	const (
		source = "packages/contracts/authz/capabilities.yaml"
		goOut  = "services/platform/platform/authz/catalogue.gen.go"
		tsOut  = "packages/generated/typescript/capabilities.gen.ts"
	)

	raw, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}

	var doc contract
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", source, err)
	}
	if len(doc.Capabilities) == 0 {
		// A contract that parsed to nothing would generate an empty catalogue,
		// and an empty catalogue denies everything, which looks like a policy
		// change rather than a broken build.
		return fmt.Errorf("%s declares no capabilities", source)
	}

	// Sorted, so the generated files do not change when the document is
	// reordered and a diff shows only what actually changed.
	sort.Slice(doc.Capabilities, func(i, j int) bool {
		return doc.Capabilities[i].Name < doc.Capabilities[j].Name
	})

	for _, entry := range doc.Capabilities {
		if strings.TrimSpace(entry.Reason) == "" {
			// Enforced here rather than left to review. A requirement without a
			// reason is a rule nobody can argue against later.
			return fmt.Errorf("capability %q has no reason", entry.Name)
		}
	}

	if err := checkRoles(doc); err != nil {
		return err
	}

	sort.Slice(doc.Roles, func(i, j int) bool { return doc.Roles[i].Name < doc.Roles[j].Name })

	if err := os.WriteFile(goOut, []byte(generateGo(doc)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", goOut, err)
	}
	if err := os.WriteFile(tsOut, []byte(generateTypeScript(doc)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tsOut, err)
	}
	return nil
}

// checkRoles refuses a role model that cannot be correct.
//
// These fail the build rather than a test, because a generated catalogue is
// what every other check runs against: a bundle naming a capability that does
// not exist would generate code that does not compile, and one granting
// platform authority through a tenant role would generate code that compiles
// and is wrong.
func checkRoles(doc contract) error {
	known := map[string]capabilityEntry{}
	for _, entry := range doc.Capabilities {
		known[entry.Name] = entry
	}

	bundled := map[string]bool{}

	for _, role := range doc.Roles {
		if strings.TrimSpace(role.Reason) == "" {
			return fmt.Errorf("role %q has no reason", role.Name)
		}
		if len(role.Capabilities) == 0 {
			// A role granting nothing is either a mistake or a role that should
			// not exist, and both are worth stopping for.
			return fmt.Errorf("role %q grants no capabilities", role.Name)
		}

		for _, name := range role.Capabilities {
			entry, exists := known[name]
			if !exists {
				return fmt.Errorf("role %q names capability %q, which is not in the catalogue", role.Name, name)
			}
			bundled[name] = true

			if entry.Platform {
				// Platform authority is separate from tenant authority rather
				// than a senior form of it. A tenant role granting one would
				// make the owner of any workspace a member of platform staff.
				return fmt.Errorf("role %q grants platform capability %q", role.Name, name)
			}

			// A role with no membership is what somebody holds outside any
			// tenant, so it can only grant capabilities that reach their own
			// data.
			if !role.Membership && !entry.Owner {
				return fmt.Errorf("role %q has no membership and grants %q, which is not owner-scoped",
					role.Name, name)
			}
			// And the reverse: a membership role granting an owner capability
			// would be tenant authority reaching a candidate's own data, which
			// is the failure this product cannot have.
			if role.Membership && entry.Owner {
				return fmt.Errorf("role %q is a membership role and grants owner capability %q",
					role.Name, name)
			}
		}
	}

	// Everything is either in a bundle or listed as deliberately not, so a new
	// capability cannot be added and quietly reach nobody.
	unbundled := map[string]bool{}
	for _, name := range doc.Unbundled {
		if _, exists := known[name]; !exists {
			return fmt.Errorf("unbundled names %q, which is not in the catalogue", name)
		}
		unbundled[name] = true
	}

	for name := range known {
		if !bundled[name] && !unbundled[name] {
			return fmt.Errorf("capability %q is in no role and is not listed as unbundled, "+
				"so nobody can hold it and nobody decided that", name)
		}
		if bundled[name] && unbundled[name] {
			return fmt.Errorf("capability %q is both bundled and listed as unbundled", name)
		}
	}

	return nil
}

// identifier turns a capability name into a Go identifier.
//
// candidate.practice.read_own becomes CandidatePracticeReadOwn, which is what
// the hand-written catalogue used, so existing call sites keep compiling.
func identifier(name string) string {
	var out strings.Builder
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '_' }) {
		out.WriteString(strings.ToUpper(part[:1]))
		out.WriteString(part[1:])
	}
	return out.String()
}

// comment wraps a reason as a Go or TypeScript comment body.
func comment(reason, indent string) string {
	var lines []string
	var current string
	for _, word := range strings.Fields(reason) {
		if len(current)+len(word)+1 > 68 {
			lines = append(lines, current)
			current = word
			continue
		}
		if current == "" {
			current = word
		} else {
			current += " " + word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	for i, line := range lines {
		lines[i] = indent + "// " + line
	}
	return strings.Join(lines, "\n")
}

func generateGo(doc contract) string {
	var out strings.Builder

	fmt.Fprintf(&out, `// Code generated by tools/authzgen. DO NOT EDIT.
//
// Source: packages/contracts/authz/capabilities.yaml (version %d)
//
// The contract is the source and this file is derived from it. Editing here is
// undone by the next %s generate, and the drift gate in CI fails first.

package authz

// The catalogue, version %d.
const CatalogueVersion = %d

const (
`, doc.Version, "`make`", doc.Version, doc.Version)

	for _, entry := range doc.Capabilities {
		fmt.Fprintf(&out, "%s\n\t%s Capability = %q\n\n", comment(entry.Reason, "\t"), identifier(entry.Name), entry.Name)
	}

	out.WriteString(")\n\n// catalogue maps every capability to what it requires.\nvar catalogue = map[Capability]Requirement{\n")
	for _, entry := range doc.Capabilities {
		var parts []string
		if entry.Tenant {
			parts = append(parts, "Tenant: true")
		}
		if entry.Scope != "" {
			parts = append(parts, fmt.Sprintf("Scope: Scope%s", strings.ToUpper(entry.Scope[:1])+entry.Scope[1:]))
		}
		if entry.Owner {
			parts = append(parts, "Owner: true")
		}
		if entry.Platform {
			parts = append(parts, "Platform: true")
		}
		if entry.Privileged {
			parts = append(parts, "Privileged: true")
		}
		if entry.StepUp {
			parts = append(parts, "StepUp: true")
		}
		fmt.Fprintf(&out, "\t%s: {%s},\n", identifier(entry.Name), strings.Join(parts, ", "))
	}
	out.WriteString("}\n\n")

	// Roles.
	out.WriteString(`// Role is a bundle of capabilities.
//
// Never a check of its own. Nothing asks whether somebody is an owner; it asks
// whether they hold a capability, and a role is only how they came to hold it.
type Role string

const (
`)
	for _, role := range doc.Roles {
		fmt.Fprintf(&out, "%s\n\tRole%s Role = %q\n\n",
			comment(role.Reason, "\t"), identifier(role.Name), role.Name)
	}
	out.WriteString(")\n\n")

	out.WriteString(`// bundles maps a role to what it grants.
var bundles = map[Role][]Capability{
`)
	for _, role := range doc.Roles {
		fmt.Fprintf(&out, "\tRole%s: {\n", identifier(role.Name))
		granted := append([]string(nil), role.Capabilities...)
		sort.Strings(granted)
		for _, name := range granted {
			fmt.Fprintf(&out, "\t\t%s,\n", identifier(name))
		}
		out.WriteString("\t},\n")
	}
	out.WriteString("}\n")

	return out.String()
}

func generateTypeScript(doc contract) string {
	var out strings.Builder

	fmt.Fprintf(&out, `/**
 * Code generated by tools/authzgen. DO NOT EDIT.
 *
 * Source: packages/contracts/authz/capabilities.yaml (version %d)
 *
 * The browser uses these to decide what to render. Rendering is never the thing
 * that stops access: the server decides, and hiding a control that somebody
 * could still reach by typing the URL would be a decoration rather than a
 * protection. What this is for is not offering somebody a button that will
 * refuse them.
 */

export const CATALOGUE_VERSION = %d;

export const CAPABILITIES = [
`, doc.Version, doc.Version)

	for _, entry := range doc.Capabilities {
		fmt.Fprintf(&out, "%s\n  %q,\n\n", comment(entry.Reason, "  "), entry.Name)
	}

	out.WriteString(`] as const;

/** Every capability name the catalogue defines. */
export type Capability = (typeof CAPABILITIES)[number];

/**
 * The roles the server bundles capabilities into.
 *
 * Exported so the browser can name one, never so it can decide from one. What
 * somebody may do arrives as a capability list on the session; a role is how
 * the server built that list.
 */
export const ROLES = [
`)
	for _, role := range doc.Roles {
		fmt.Fprintf(&out, "  %q,\n", role.Name)
	}
	out.WriteString(`] as const;

export type Role = (typeof ROLES)[number];

/**
 * What each role's bundle grants, for DISPLAY: the permission matrix a
 * tenant administrator reads is generated from this, so the screen and the
 * server cannot disagree about what a role does. Never used to decide -
 * authority arrives as the session's own capability list.
 */
export const BUNDLES = {
`)
	for _, role := range doc.Roles {
		fmt.Fprintf(&out, "  %s: [\n", role.Name)
		for _, name := range role.Capabilities {
			fmt.Fprintf(&out, "    %q,\n", name)
		}
		out.WriteString("  ],\n")
	}
	out.WriteString(`} as const satisfies Record<Role, readonly Capability[]>;

/**
 * Why each role exists, in the contract's own words - what an administrator
 * reads before granting one.
 */
export const ROLE_REASONS = {
`)
	for _, role := range doc.Roles {
		fmt.Fprintf(&out, "  %s: %q,\n", role.Name, strings.TrimSpace(role.Reason))
	}
	out.WriteString(`} as const satisfies Record<Role, string>;

/**
 * Each capability's reason, in the contract's words: what the permission
 * matrix shows beside a row, so the explanation on screen is the one legal
 * and security reviewed.
 */
export const CAPABILITY_REASONS = {
`)
	for _, entry := range doc.Capabilities {
		fmt.Fprintf(&out, "  %q: %q,\n", entry.Name, strings.TrimSpace(entry.Reason))
	}
	out.WriteString(`} as const satisfies Record<Capability, string>;

/**
 * Capabilities that additionally require an explicit assignment covering the
 * resource: holding one grants nothing outside the campaigns or roles the
 * person is scoped to. The matrix renders these as "scoped", because that is
 * the truth of what the bundle grants.
 */
export const SCOPED_CAPABILITIES = [
`)
	for _, entry := range doc.Capabilities {
		if entry.Scope != "" {
			fmt.Fprintf(&out, "  %q,\n", entry.Name)
		}
	}
	out.WriteString(`] as const;
`)

	return out.String()
}
