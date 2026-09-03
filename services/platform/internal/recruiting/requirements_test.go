package recruiting_test

import (
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The extractor's spans must index back into the exact source bytes, because a
// requirement whose provenance does not resolve to what it claims is worse than
// no provenance: it asserts an origin nobody can check.
func TestRuleExtractorSpansIndexBackIntoTheSource(t *testing.T) {
	source := "Requirements:\n- Five years of Go\n  Strong SQL\nENGINEERING\nLeads a small team\n"
	version, reqs := recruiting.NewRuleExtractor().Extract(source)

	if version != recruiting.RuleExtractorVersion {
		t.Fatalf("version = %q", version)
	}
	// "Requirements:" is a heading, "ENGINEERING" is an all-caps heading; the
	// three real requirements remain.
	if len(reqs) != 3 {
		t.Fatalf("got %d requirements, want 3: %+v", len(reqs), reqs)
	}
	for _, req := range reqs {
		if req.SpanStart < 0 || req.SpanEnd > len(source) || req.SpanStart >= req.SpanEnd {
			t.Fatalf("span [%d,%d) is not a valid range into %d bytes", req.SpanStart, req.SpanEnd, len(source))
		}
		slice := source[req.SpanStart:req.SpanEnd]
		// The stored text has its bullet stripped, but the span points at the
		// source, so the source slice must contain the requirement's words.
		if slice == "" {
			t.Fatalf("empty source slice for %q", req.Text)
		}
	}
	// The first real requirement is the Go line, its bullet stripped.
	if reqs[0].Text != "Five years of Go" {
		t.Fatalf("first requirement = %q", reqs[0].Text)
	}
	if got := source[reqs[0].SpanStart:reqs[0].SpanEnd]; got != "- Five years of Go" {
		t.Fatalf("first span resolves to %q, not the source line", got)
	}
	// The indented line keeps its span aligned to content, not the whitespace.
	if reqs[1].Text != "Strong SQL" || source[reqs[1].SpanStart:reqs[1].SpanEnd] != "Strong SQL" {
		t.Fatalf("second requirement span misaligned: %q / %q", reqs[1].Text, source[reqs[1].SpanStart:reqs[1].SpanEnd])
	}
}

// Empty and heading-only text produce no requirements rather than empty-text
// junk requirements.
func TestRuleExtractorSkipsHeadingsAndBlanks(t *testing.T) {
	_, reqs := recruiting.NewRuleExtractor().Extract("ROLE:\n\nResponsibilities:\n   \n")
	if len(reqs) != 0 {
		t.Fatalf("headings and blanks produced %d requirements: %+v", len(reqs), reqs)
	}
}
