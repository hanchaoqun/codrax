package types

import (
	"strings"
	"testing"
)

func TestLooksLikeModelAuthoredVisibleSurfaceDraft_StructuralSurfaces(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "markdown table",
			body: "| package | entry |\n| --- | --- |\n| alpha | Run |\n",
		},
		{
			name: "fenced diagram",
			body: "```mermaid\nflowchart TD\nA --> B\n```",
		},
		{
			name: "box drawing",
			body: "┌────┐\n│ A  │\n└────┘",
		},
		{
			name: "long organized list",
			body: "- item one has enough surrounding explanation to be user-visible\n" +
				"- item two\n- item three\n- item four\n- item five\n" +
				strings.Repeat("additional prose that makes this a substantive draft rather than a tiny scratch list. ", 8),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !LooksLikeModelAuthoredVisibleSurfaceDraft(tc.body) {
				t.Fatalf("expected visible surface draft for:\n%s", tc.body)
			}
		})
	}
}

func TestLooksLikeModelAuthoredVisibleSurfaceDraft_DropsPlaceholderAndPlainProse(t *testing.T) {
	for _, body := range []string{
		"{{PLACEHOLDER_REASONING_PLACEHOLDER}}",
		"short plain prose without a table or diagram",
	} {
		if LooksLikeModelAuthoredVisibleSurfaceDraft(body) {
			t.Fatalf("unexpected visible surface draft: %q", body)
		}
	}
}

func TestTrimModelAuthoredVisibleSurfaceDraft_BoundsPromptPayload(t *testing.T) {
	got := TrimModelAuthoredVisibleSurfaceDraft("abcdef", 3)
	if got != "abc\n\n[model-authored draft clipped for prompt budget]" {
		t.Fatalf("trimmed draft = %q", got)
	}
	if got := TrimModelAuthoredVisibleSurfaceDraft("{{PLACEHOLDER_REASONING_PLACEHOLDER}}", 100); got != "" {
		t.Fatalf("placeholder should trim to empty, got %q", got)
	}
}
