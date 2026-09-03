package recruiting

import (
	"errors"
	"strings"
	"unicode"
)

// SCR-03: the requirements a screening interview draws on, extracted from the
// job context a recruiter provides, each linked to the exact span of that text
// it came from, so EVL-06 can report evidence against a requirement whose
// origin is auditable.
//
// Extraction here is deterministic, the floor PRO-03 set for the CV side: rules
// over text, behind a port, so a model-backed extractor is a later ticket's
// swap rather than a rewrite. What matters at any quality is the contract this
// enforces: a requirement that cannot say where it came from is not a
// requirement, and the recruiter reviews and corrects before anything issues.

// RequirementExtractor turns job-context text into candidate requirements. It
// is a port so the deterministic floor below can be replaced by a model-backed
// one without any caller changing.
type RequirementExtractor interface {
	// Extract reads the job context and returns the requirements it found, each
	// carrying the half-open span of the source it came from. It reports the
	// version of the extractor that produced them, so a requirement is always
	// legible against exactly the rules that made it.
	Extract(sourceText string) (version string, requirements []ExtractedRequirement)
}

// ExtractedRequirement is one requirement as extraction proposes it, before a
// recruiter has reviewed it.
type ExtractedRequirement struct {
	// Text is the requirement as extraction read it.
	Text string
	// SpanStart and SpanEnd are the half-open range [start, end) of the source
	// text this requirement came from, so it can be shown in context and
	// verified against the exact bytes.
	SpanStart int
	SpanEnd   int
}

// RuleExtractorVersion names the deterministic floor, so requirements it
// produced are told apart from a later extractor's the way PRO-03's are.
const RuleExtractorVersion = "requirements-rule-1"

// ErrNoJobContext means extraction was asked to read empty text. A job context
// with nothing in it produces no requirements, which is a caller error rather
// than an empty result to store.
var ErrNoJobContext = errors.New("recruiting: the job context is empty")

// ruleExtractor is the deterministic floor: it reads the job context a line at
// a time and takes each line that states a requirement, skipping the headings
// that organise a job description rather than state one of its needs.
type ruleExtractor struct{}

// NewRuleExtractor builds the deterministic extractor.
func NewRuleExtractor() RequirementExtractor { return ruleExtractor{} }

// Extract walks the source line by line, tracking the offset so every
// requirement carries the exact span it came from. A line is a requirement
// unless it reads as a heading: headings organise the document and stating them
// as requirements would put words nobody wrote into what the interview judges.
func (ruleExtractor) Extract(sourceText string) (string, []ExtractedRequirement) {
	requirements := []ExtractedRequirement{}
	offset := 0
	for _, line := range splitKeepingOffsets(sourceText) {
		trimmed := strings.TrimSpace(line.text)
		start := offset + line.leading
		end := start + len(strings.TrimRightFunc(line.text[line.leading:], unicode.IsSpace))
		offset += len(line.text) + line.sep

		if trimmed == "" || isHeading(trimmed) {
			continue
		}
		requirements = append(requirements, ExtractedRequirement{
			Text: stripBullet(trimmed), SpanStart: start, SpanEnd: end,
		})
	}
	return RuleExtractorVersion, requirements
}

// lineWithOffset is one source line and where its content sits within it.
type lineWithOffset struct {
	text    string
	leading int // bytes of leading whitespace before content
	sep     int // bytes of the newline separator that followed, 0 at end
}

// splitKeepingOffsets breaks text into lines while preserving the byte offsets
// a span must be measured in, which strings.Split alone discards.
func splitKeepingOffsets(text string) []lineWithOffset {
	lines := []lineWithOffset{}
	for len(text) > 0 {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			lines = append(lines, lineWithOffset{text: text, leading: leadingSpaces(text)})
			break
		}
		segment := text[:idx]
		lines = append(lines, lineWithOffset{text: segment, leading: leadingSpaces(segment), sep: 1})
		text = text[idx+1:]
	}
	return lines
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeftFunc(s, unicode.IsSpace))
}

// isHeading reports whether a line organises the document rather than states a
// requirement. A trailing colon marks a section label; an all-caps short line
// is the other common heading shape a job description uses.
func isHeading(line string) bool {
	if strings.HasSuffix(line, ":") {
		return true
	}
	letters, upper := 0, 0
	for _, r := range line {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters > 0 && upper == letters && len(line) <= 40
}

// stripBullet removes a leading list marker so the requirement reads as a
// statement rather than a fragment of formatting, without touching the span,
// which still points at the source including its marker.
func stripBullet(line string) string {
	return strings.TrimSpace(strings.TrimLeft(line, "-*•·—0123456789.() \t"))
}
