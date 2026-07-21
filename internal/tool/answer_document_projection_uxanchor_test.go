package tool

// answer_document_projection_uxanchor_test.go — UX-ANCHOR engine-minted pins
// (§29.61.7 customer feedback, 2026-07-14): the projection LEAD segment's E#
// references and TOP-5 badges decorate on the HTML face through the REAL
// product path — engine cluster → render.RenderAnswerDocument markdown →
// preview.RenderMarkdownHTML — never a handwritten lookalike. The fixture is
// the cov4 running-dominant shape whose four-state running line mints the
// 「确定性工作 … 见 ➌[E#]」 and 「供给折算影响 … 见 ➍[E#]」 badge+ref pairs.
//
// Synthetic boundary/negative lanes (fail-closed count identity, unclaimed
// ordinals, foreign-heading scope, bare-form grammar) live in
// internal/preview/markdown_trace_lead_test.go.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

var uxAnchorBadgeRefPattern = regexp.MustCompile(`(➊|➋|➌|➍|➎)\[(E\d+)(\(\+\d+\))?\]`)

func TestUXAnchorEngineLeadRefsDecorateOnHTMLFace(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(cov4RunningDominantProjection(), "zh", runtimeTraceProjUserFocus{})
	markRuntimeTraceSystemBlocks(blocks)
	md := render.RenderAnswerDocument(&types.AnswerDocumentV2{Blocks: blocks}, "zh")

	// Fixture guards: the real renderer emits the H2 title from the table-④
	// single source, and the lead prose carries at least one badge+[E#] pair
	// BEFORE the tree fence (the §29.27.1 running attribution pointers).
	if !strings.Contains(md, "## "+tracefence.SectionProjectionZH+"\n") {
		t.Fatalf("engine markdown lost the projection section H2:\n%s", md)
	}
	fenceAt := strings.Index(md, tracefence.Opener)
	if fenceAt < 0 {
		t.Fatalf("engine markdown lost the projection fence:\n%s", md)
	}
	leadPairs := uxAnchorBadgeRefPattern.FindAllStringSubmatch(md[:fenceAt], -1)
	if len(leadPairs) == 0 {
		t.Fatalf("cov4 lead no longer mints a badge+[E#] pair before the fence:\n%s", md[:fenceAt])
	}

	html, err := preview.RenderMarkdownHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	badgeRank := map[string]int{"➊": 1, "➋": 2, "➌": 3, "➍": 4, "➎": 5}
	for _, pair := range leadPairs {
		glyph, ordinal, merge := pair[1], pair[2], pair[3]
		badge := `<span class="trace-lead-badge trace-rank-` +
			string(rune('0'+badgeRank[glyph])) + `">` + glyph + `</span>`
		link := `<a class="trace-eref-lead" href="#trace-` +
			strings.ToLower(ordinal) + `">[` + ordinal + merge + `]</a>`
		if !strings.Contains(html, badge+link) {
			t.Errorf("engine lead pair %s[%s%s] missing its compact badge + anchor link (want %q):\n%s",
				glyph, ordinal, merge, badge+link, html)
		}
	}
	// The linked ordinals resolve to real in-document targets (no dangling
	// href): each claimed id renders exactly once.
	for _, pair := range leadPairs {
		id := `id="trace-` + strings.ToLower(pair[2]) + `"`
		if strings.Count(html, id) != 1 {
			t.Errorf("lead anchor target %s must exist exactly once:\n%s", id, html)
		}
	}
	// The fence's own [E#] links (pre-existing lane) stay live beside the
	// lead lane.
	if !strings.Contains(html, `<a class="trace-eref" `) {
		t.Fatalf("fence anchor links must stay live:\n%s", html)
	}
}
