package glossarylint

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestScanPromptSurfaces_ExactResolvedConstIsNeverAlsoAPart pins that the
// exact text and the flow-bound parts of a system surface are disjoint by
// literal position: a package-level const the resolver folded into the
// exact text is reached again by the walker when a same-package builder in
// the same Content names it, and must NOT be reported a second time at
// the const's own line. Both concatenation orders are pinned because the
// resolver visits operands left to right (the walker may run before or
// after the exact fold). EVOLUTION RECORD (batch six fold-in, review
// round three #6): the lane appended every walker part, so this shape
// yielded two TaskGraph hits under one owner (p.go:10 and p.go:5) — red on
// 480939385 for both orders.
func TestScanPromptSurfaces_ExactResolvedConstIsNeverAlsoAPart(t *testing.T) {
	for name, content := range map[string]string{
		"const then builder": `toolUseTail + "\n" + tail()`,
		"builder then const": `tail() + "\n" + toolUseTail`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeScratchPackage(t, map[string]string{"p.go": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

const toolUseTail = "Use tools when the TaskGraph wants them."

func tail() string { return toolUseTail + " again naming the EvidencePlan" }

func build() []llm.Message {
	return []llm.Message{{Role: "system", Content: ` + content + `}}
}
`})
			hits, surfaces, err := ScanPromptSurfaces(dir)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(surfaces) != 1 || !strings.HasSuffix(surfaces[0].Label, "p.go:10 SystemMessage.Content") {
				t.Fatalf("surfaces = %+v, want the one system message at p.go:10", surfaces)
			}
			var got []string
			for _, h := range hits {
				pos := h.Label[:strings.Index(h.Label, " ")]
				got = append(got, filepath.Base(pos)+"/"+h.Term)
			}
			sort.Strings(got)
			want := []string{
				"p.go:10/TaskGraph",   // the const, resolved exactly, reported ONCE at the message site
				"p.go:7/EvidencePlan", // the builder's own literal, bound by flow, reported at its line
			}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Fatalf("hits = %v\nwant %v (the const's literal must not be reported again as a walker part)", got, want)
			}
			for _, p := range surfaces[0].parts {
				if strings.Contains(p.text, "TaskGraph") {
					t.Fatalf("the exactly-resolved const literal leaked into parts at %s: %q", p.pos, p.text)
				}
			}
			if n := strings.Count(surfaces[0].Text, "TaskGraph"); n != 1 {
				t.Fatalf("surface Text carries the const text %d times, want exactly once:\n%s", n, surfaces[0].Text)
			}
		})
	}
}
