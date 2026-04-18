package hint

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G3 HintComposer tests — lock the 6-field contract and the
// structured render path. Non-strict rendering always produces
// SOME output; strict mode fails loud on missing fields.

func TestCompose_EmptyViolations_NonStrictReturnsEmptyHint(t *testing.T) {
	c := New(DefaultConfig())
	h, err := c.Compose(Context{}, nil)
	if err != nil {
		t.Fatalf("non-strict should tolerate empty input, got err=%v", err)
	}
	if h == nil {
		t.Fatalf("non-strict should return non-nil Hint for empty input")
	}
	if got := c.Render(h); !strings.Contains(got, "Retry Directive") {
		t.Errorf("even empty hint should still render header; got %q", got)
	}
}

func TestCompose_EmptyViolations_StrictErrors(t *testing.T) {
	c := New(Config{StrictMode: true})
	if _, err := c.Compose(Context{}, nil); err == nil {
		t.Fatal("strict mode should reject empty violations")
	}
}

func TestCompose_ShapeViolation_BuildsAllowedAndForbidden(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind:   types.ViolShape,
		Detail: "LLM chose shape=boolean but contract requires value",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_shape",
			Reason:     "finalizer picked boolean",
			Confidence: 0.85,
		},
	}}
	h, err := c.Compose(Context{TargetShape: types.ShapeValue}, v)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if h.WhatFailed == "" {
		t.Errorf("WhatFailed empty")
	}
	if !strings.Contains(h.WhyItFailed, "answer_shape") {
		t.Errorf("WhyItFailed must mention SuspectedRoot.IRField, got %q", h.WhyItFailed)
	}
	if len(h.AllowedSet) != 1 || h.AllowedSet[0].Value != "value" {
		t.Errorf("AllowedSet should contain the target shape, got %v", h.AllowedSet)
	}
	if len(h.ForbiddenPatterns) == 0 {
		t.Errorf("ForbiddenPatterns must contain the negative example")
	}
	out := c.Render(h)
	if !strings.Contains(out, "**Why it failed**") || !strings.Contains(out, "**How to fix now**") {
		t.Errorf("Render missing mandatory section headers; got %q", out)
	}
}

func TestCompose_CitationViolation_AllowedFromReadSet(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind:          types.ViolCitation,
		Detail:        "0/3 citations in ReadSet",
		SuspectedRoot: types.SuspectedRoot{IRField: "ScannedSet", Confidence: 0.9},
	}}
	readSet := []string{"a.go", "b.go", "c.go"}
	h, _ := c.Compose(Context{ReadSet: readSet}, v)
	if len(h.AllowedSet) != 3 {
		t.Errorf("AllowedSet len=%d, want 3 (one per ReadSet file)", len(h.AllowedSet))
	}
	out := c.Render(h)
	if !strings.Contains(out, "a.go") {
		t.Errorf("Render must include ReadSet files, got %q", out)
	}
}

func TestCompose_SelfRefLiteral_ForbidsPrimaryEntity(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind: types.ViolSelfRefLiteral,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_subject.kind", Confidence: 0.75,
		},
	}}
	h, _ := c.Compose(Context{PrimaryEntity: "explorer"}, v)
	var found bool
	for _, p := range h.ForbiddenPatterns {
		if strings.Contains(p, "explorer") {
			found = true
		}
	}
	if !found {
		t.Errorf("ForbiddenPatterns must call out self-reference to primary entity; got %v", h.ForbiddenPatterns)
	}
}

func TestCompose_StrictModeRequiresAllFields(t *testing.T) {
	c := New(Config{StrictMode: true, MaxAllowedSet: 5})
	// Missing TargetShape → AllowedSet empty for a shape violation
	// → Validate fails.
	v := []types.Violation{{
		Kind:          types.ViolShape,
		Detail:        "boolean vs value",
		SuspectedRoot: types.SuspectedRoot{IRField: "answer_shape", Confidence: 0.85},
	}}
	if _, err := c.Compose(Context{ /* no TargetShape */ }, v); err == nil {
		t.Fatal("strict mode must fail when AllowedSet is empty")
	}
}

func TestRenderCompact_PreservesLegacySemicolonFormat(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{
		{Kind: types.ViolShape, Detail: "boolean vs value"},
		{Kind: types.ViolCitation, Detail: "missing line", Repair: "add :N"},
	}
	h, _ := c.Compose(Context{}, v)
	got := c.RenderCompact(h)
	if got != "boolean vs value; missing line (add :N)" {
		t.Errorf("RenderCompact format drift: got %q", got)
	}
}

func TestLimitAllowedAndForbidden_TrimsWithEllipsis(t *testing.T) {
	c := New(Config{MaxAllowedSet: 2, MaxForbiddenPatterns: 1})
	v := []types.Violation{{
		Kind:          types.ViolCitation,
		SuspectedRoot: types.SuspectedRoot{IRField: "ScannedSet", Confidence: 0.9},
	}}
	ctx := Context{ReadSet: []string{"a.go", "b.go", "c.go", "d.go"}}
	h, _ := c.Compose(ctx, v)
	if len(h.AllowedSet) != 3 { // 2 kept + 1 "...and N more"
		t.Errorf("AllowedSet len=%d, want 3 (cap 2 + ellipsis)", len(h.AllowedSet))
	}
	if !strings.Contains(h.AllowedSet[2].Value, "more") {
		t.Errorf("ellipsis entry missing 'more' marker, got %v", h.AllowedSet[2])
	}
}
