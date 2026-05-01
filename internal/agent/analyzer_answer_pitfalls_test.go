package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPrependAnswerPitfalls_EmptyIsByteIdentical pins the
// commit-51 contract: when ctx.ActiveAnswerPitfalls is empty (the
// common case — no prior Run learned anything for this repo, or
// the feature is disabled), the function returns base unchanged.
// This is the byte-identity backstop that keeps the analyzer
// prompt unchanged for any Run that doesn't have a populated
// taxonomy, so existing eval traces stay reproducible.
func TestPrependAnswerPitfalls_EmptyIsByteIdentical(t *testing.T) {
	ctx := &types.AgentContext{}
	got := prependAnswerPitfalls(ctx, "ORIGINAL")
	if got != "ORIGINAL" {
		t.Errorf("empty pitfalls should return base unchanged; got %q", got)
	}
	got = prependAnswerPitfalls(ctx, "")
	if got != "" {
		t.Errorf("empty pitfalls + empty base should be empty; got %q", got)
	}
}

// TestPrependAnswerPitfalls_RendersHeader pins the rendered
// shape: a "## Known answer pitfalls in this repo" header, the
// descriptive (non-prescriptive) framing, and per-pattern
// bullets carrying name + description + trigger.
func TestPrependAnswerPitfalls_RendersHeader(t *testing.T) {
	ctx := &types.AgentContext{
		ActiveAnswerPitfalls: []types.AnswerPattern{
			{
				Name:        "enumeration counts interfaces",
				Description: "Past Runs mis-counted Y when the user wanted Z; the trap recurs.",
				Trigger:     "when the user asks how many of an -er type",
				Consequence: "shape-subject mismatch fail",
				Confidence:  0.8,
				HitCount:    1,
				FirstSeen:   time.Now(),
				LastSeen:    time.Now(),
			},
		},
	}
	got := prependAnswerPitfalls(ctx, "ORIGINAL_BASE")
	if !strings.Contains(got, "## Known answer pitfalls in this repo") {
		t.Error("expected header in rendered prompt")
	}
	if !strings.Contains(got, "enumeration counts interfaces") {
		t.Error("expected pattern name in rendered prompt")
	}
	if !strings.Contains(got, "Trigger: when the user asks how many of an -er type") {
		t.Error("expected trigger sub-bullet")
	}
	if !strings.Contains(got, "Typical consequence: shape-subject mismatch fail") {
		t.Error("expected consequence sub-bullet")
	}
	if !strings.HasSuffix(got, "ORIGINAL_BASE") {
		t.Errorf("base should be appended after pitfalls section; got tail %q", got[len(got)-30:])
	}
	// Descriptive framing must NOT be prescriptive.
	if strings.Contains(got, "you must") || strings.Contains(got, "MUST") {
		t.Error("framing should be descriptive (\"these are observations\"), not prescriptive (\"you must\"); commit 40 red line carried over to commit 51")
	}
}
