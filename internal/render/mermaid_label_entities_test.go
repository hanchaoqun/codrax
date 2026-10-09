package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

func TestRenderMermaidBlocks_QuotedEntitiesPreserveVisibleText(t *testing.T) {
	for _, tc := range []struct{ name, label, want string }{
		{"quote and symbols", `first<br/>second &quot;owner&quot; &amp; a &lt; b &gt; c`, `first second "owner" & a < b > c`},
		{"literal entities", `literal &amp;quot; and &amp;lt; and &amp;gt; and &amp;amp;`, `literal &quot; and &lt; and &gt; and &amp;`},
		{"literal slash then quote", `slash \&quot;owner\&quot; tail`, `slash \"owner\" tail`},
		{"empty quotes", `before &quot;&quot; after`, `before "" after`},
	} {
		for _, family := range []string{"flow", "sequence"} {
			t.Run(family+"/"+tc.name, func(t *testing.T) {
				body := "flowchart LR\n A[\"" + tc.label + "\"]\n B[\"sink\"]\n A --> B"
				if family == "sequence" {
					body = "sequenceDiagram\n participant A as \"" + tc.label + "\"\n participant B as sink\n A->>B: sends"
				}
				out := RenderMermaidBlocks("```mermaid\n" + body + "\n```")
				if !strings.Contains(out, "```text\n") || strings.Contains(out, "# ⚠") || strings.Contains(out, "# ·") || !strings.Contains(out, tc.want) {
					t.Fatalf("render must preserve %q without syntax truncation or entity re-decoding:\n%s", tc.want, out)
				}
			})
		}
	}
}

func TestNormalizeMermaidLabels_OneSourcePassAndQuotedEscapes(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`A["x &quot;y&quot; &amp; &lt; &gt;"]`, `A["x \"y\" & < >"]`},
		{`A["&amp;quot; &amp;lt; &amp;gt; &amp;amp;"]`, `A["&quot; &lt; &gt; &amp;"]`},
		{`A["slash \&quot;x\&quot;"]`, `A["slash \\\"x\\\""]`},
		{`A["already \"x\" &amp; more"]`, `A["already \"x\" & more"]`},
		{"%% unmatched \"\n A[\"&quot;kept&quot;\"]", "%% unmatched \"\n A[\"\\\"kept\\\"\"]"},
	} {
		if got := normalizeMermaidLabels(tc.input); got != tc.want {
			t.Fatalf("input %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}

func TestRenderMermaidBlocks_EntityShimFailurePreservesOriginalSource(t *testing.T) {
	for _, body := range []string{
		"classDiagram\n class A[\"line1<br/>line2 &quot;quote&quot; &amp;literal\"]\n A --> B",
	} {
		out := RenderMermaidBlocks("```mermaid\n" + body + "\n```")
		if !strings.Contains(out, "```text\n") || !strings.Contains(out, "# ·") || !strings.Contains(out, body) {
			t.Fatalf("render failure must retain original source rather than the terminal-adapted bytes:\n%s", out)
		}
	}
}

func TestRenderMermaidBlocks_AuthoredLiteralBackslashesPreserveLabelsAndEdges(t *testing.T) {
	for _, label := range []string{`C:\temp\new\`, `literal \\ pair`, `literal \n not newline`, `prefix\"quote`, `prefix\\"quote`} {
		for _, family := range []string{"flowchart LR", "sequenceDiagram"} {
			t.Run(family+"/"+label, func(t *testing.T) {
				body, err := mermaidcompat.AddExplicitNodeDeclarationChecked(family+"\n", "A", label)
				if err != nil {
					t.Fatal(err)
				}
				if family == "sequenceDiagram" {
					body += "\nparticipant B as sink\nA->>B: sends"
				} else {
					body += "\nB[\"sink\"]\nA -->|sends| B"
				}
				edges := mermaidcompat.ParseEdges(body)
				if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" {
					t.Fatalf("backslash escaped the declaration and swallowed an edge: %+v\n%s", edges, body)
				}
				out := RenderMermaidBlocks("```mermaid\n" + body + "\n```")
				for _, want := range []string{label, "sink", "sends"} {
					if !strings.Contains(out, want) {
						t.Errorf("literal display text or following graph was lost (%q):\n%s", want, out)
					}
				}
				if strings.Contains(out, "# ·") || strings.Contains(out, "# ⚠") || !strings.Contains(out, "```text") {
					t.Fatalf("encoded backslashes must reach the renderer, not a fallback:\n%s", out)
				}
			})
		}
	}
}

func TestMermaidLiteralLabelNewlineAdapterCollisionAndCapacity(t *testing.T) {
	const body = "flowchart LR\n A[\"Qa Qb Qc \\\\new\"]\n B[\"中文\"]\n A --> B"
	adapter := newCJKAdapter(body)
	prepared, _, err := adapter.substitute(body)
	if err != nil {
		t.Fatal(err)
	}
	prepared, restore, err := protectMermaidLiteralLabelNewlines(prepared, adapter)
	if err != nil || len(restore) != 1 {
		t.Fatalf("literal label adapter failed: %+v %v", restore, err)
	}
	for placeholder := range restore {
		if strings.Contains(body, placeholder) || adapter.placeholderToRune[placeholder] != 0 {
			t.Fatalf("literal placeholder collided with source or an allocated CJK cell: %q", placeholder)
		}
	}
	restored := adapter.restore(prepared)
	for placeholder, literal := range restore {
		restored = strings.ReplaceAll(restored, placeholder, literal)
	}
	if !strings.Contains(restored, `Qa Qb Qc \new`) || !strings.Contains(restored, "中文") {
		t.Fatalf("restoration damaged existing labels: %q", restored)
	}
	exhausted := newCJKAdapter(body)
	exhausted.poolCursor = len(placeholderPool)
	if got, placeholders, err := protectMermaidLiteralLabelNewlines(body, exhausted); err != errPlaceholderExhausted || got != body || placeholders != nil {
		t.Fatalf("capacity failure must preserve input and fail loudly: body=%q placeholders=%v err=%v", got, placeholders, err)
	}
	// Exercise the real failure path too: every safe pair already occurs in
	// this ASCII label, so the shared adapter cannot allocate a literal cell.
	source := "flowchart LR\n A[\"" + strings.Join(placeholderPool, " ") + " &#92;new\"]\n B[\"sink\"]\n A --> B"
	for _, fence := range []string{
		"```mermaid\n" + source + "\n```",
		"```mermaid flowchart LR\n" + strings.TrimPrefix(source, "flowchart LR\n") + "\n```",
		"```flowchart LR\n" + strings.TrimPrefix(source, "flowchart LR\n") + "\n```",
	} {
		out := RenderMermaidBlocks(fence)
		if !strings.Contains(out, "```text\n# ⚠ ") || !strings.Contains(out, source) {
			t.Fatalf("exhausted terminal shim must preserve complete original source with a failure warning across fence spellings:\n%s", out)
		}
	}
}

func TestRenderMermaidBlocks_ParseFailureWarnsAcrossFenceSpellings(t *testing.T) {
	const body = "sequenceDiagram\n participant"
	for _, fence := range []string{
		"```mermaid\n" + body + "\n```",
		"```mermaid sequenceDiagram\n participant\n```",
		"```sequenceDiagram\n participant\n```",
	} {
		out := RenderMermaidBlocks(fence)
		if !strings.Contains(out, "```text\n# ⚠ ") || !strings.Contains(out, body) {
			t.Fatalf("genuine parse failure must retain its source with a warning rather than unsupported-kind notice:\n%s", out)
		}
	}
}
