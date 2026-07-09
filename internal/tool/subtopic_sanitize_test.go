package tool

import (
	"strings"
	"testing"
	"unicode/utf8"

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

// TestSanitizeSubTopicSummary_CJKTruncationRuneSafe pins the G18
// mojibake fix on the original customer witness: the live task row
// rendered "…找出导致帧延迟的事件��…" (garbled glyphs after 事件)
// because the byte-offset cut s[:subTopicSummaryMaxChars] split a CJK
// rune into U+FFFD garbage.
// The truncated summary MUST be valid UTF-8, MUST NOT contain a
// replacement character, and MUST end with a complete character
// followed by the ellipsis.
func TestSanitizeSubTopicSummary_CJKTruncationRuneSafe(t *testing.T) {
	// Customer request from the 2026-07 revisit session. The bare
	// sentence is 118 bytes; the closing full-width "。" (3 bytes,
	// spanning offsets 118-120) is what straddled the 120-byte cap at
	// runtime — the old byte cut kept 2 of its 3 bytes and rendered
	// garbage right after "事件", exactly as witnessed.
	const sentence = "分析线程 16547 在 33872.289161s 至 33872.408222s 时间范围内的执行情况，找出导致帧延迟的事件"
	const witness = sentence + "。"
	if len(witness) <= subTopicSummaryMaxChars {
		t.Fatalf("witness is %d bytes; must exceed subTopicSummaryMaxChars=%d for this pin to bite",
			len(witness), subTopicSummaryMaxChars)
	}
	got := sanitizeSubTopicSummary(witness, nil)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated summary is not valid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("truncated summary contains U+FFFD mojibake: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated summary must end with ellipsis: %q", got)
	}
	body := strings.TrimSuffix(got, "…")
	if body == "" {
		t.Fatalf("truncation kept nothing of the witness: %q", got)
	}
	// The kept prefix must be a prefix of the original text ending on
	// a complete character (byte-budget cut backed off to a rune
	// boundary, then whitespace-trimmed) — never a shredded rune.
	if !strings.HasPrefix(witness, body) {
		t.Errorf("kept text %q is not a clean prefix of the witness", body)
	}
	if len(body) > subTopicSummaryMaxChars {
		t.Errorf("kept text is %d bytes, budget %d", len(body), subTopicSummaryMaxChars)
	}
	last, _ := utf8.DecodeLastRuneInString(body)
	if last == utf8.RuneError {
		t.Errorf("kept text ends mid-rune: %q", body)
	}
	// Exact form: the straddling "。" is dropped whole; the row reads
	// "…找出导致帧延迟的事件…" instead of the mojibake witness.
	if want := sentence + "…"; got != want {
		t.Errorf("truncated summary = %q, want %q", got, want)
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
