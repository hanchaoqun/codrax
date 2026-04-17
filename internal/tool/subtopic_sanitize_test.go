package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSanitizeSubTopicSummary covers every cleanup tier:
//   T1 — well-formed REPL prefix stripped to the current request
//   T2 — stray pollution marker falls back to entity list
//   T3 — long paragraph truncated with ellipsis
func TestSanitizeSubTopicSummary(t *testing.T) {
	cases := []struct {
		name     string
		summary  string
		entities []string
		want     string
	}{
		{
			name:    "clean short summary passes through",
			summary: "analyzer 的功能和作用是什么？",
			want:    "analyzer 的功能和作用是什么？",
		},
		{
			name:    "well-formed REPL prefix stripped",
			summary: "## Prior conversation\n### Recent conversation\n- You: hi\n\n## Current request\nwhat does X do",
			want:    "what does X do",
		},
		{
			name:     "partial REPL fragment falls back to entities",
			summary:  "## Prior conversation\n- You: hi\n- Codrax: (previous attempt ended in error)",
			entities: []string{"analyzer", "explorer"},
			want:     "analyzer / explorer",
		},
		{
			name:    "stray Recent-conversation marker also falls back",
			summary: "### Recent conversation snippet (partial)",
			want:    "",
		},
		{
			name:    "error-placeholder marker triggers fallback",
			summary: "analyzer details: (previous attempt ended in error — details omitted from memory)",
			want:    "",
		},
		{
			name:    "whitespace trimmed",
			summary: "   hello world   ",
			want:    "hello world",
		},
		{
			name:    "very long summary truncated at cap",
			summary: strings.Repeat("x", subTopicSummaryMaxChars+50),
			want:    strings.Repeat("x", subTopicSummaryMaxChars) + "…",
		},
		{
			name:    "empty input returns empty",
			summary: "",
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeSubTopicSummary(c.summary, c.entities)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestSanitizeSubTopics_SliceShapePreserved confirms the helper
// returns a fresh slice of the same length and preserves entity
// lists verbatim while cleaning summaries.
func TestSanitizeSubTopics_SliceShapePreserved(t *testing.T) {
	in := []types.SubTopic{
		{Summary: "topic one", Entities: []string{"A"}},
		{Summary: "## Prior conversation\n polluted", Entities: []string{"B", "C"}},
	}
	out := sanitizeSubTopics(in)
	if len(out) != len(in) {
		t.Fatalf("length: got %d, want %d", len(out), len(in))
	}
	if out[0].Summary != "topic one" {
		t.Errorf("clean summary mutated: %q", out[0].Summary)
	}
	if out[1].Summary != "B / C" {
		t.Errorf("polluted summary not replaced: %q", out[1].Summary)
	}
	if len(out[1].Entities) != 2 || out[1].Entities[0] != "B" {
		t.Errorf("entities not preserved: %+v", out[1].Entities)
	}
	// Originals must not be mutated.
	if in[1].Summary != "## Prior conversation\n polluted" {
		t.Errorf("input mutated: %q", in[1].Summary)
	}
}
