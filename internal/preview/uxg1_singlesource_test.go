package preview

// uxg1_singlesource_test.go — UXG-1 M1 grep tripwire (ledger
// docs/design/real_trace_campaign_20260705.md §29.38 M1; matrix spec §④):
// this package's NON-TEST sources must not spell any entry of the five
// tracefence display-token tables as a local literal — every consumer reads
// the internal/tracefence constants. This is the mechanical closure of the
// UXG-0 过程根因 ("typed 权威在 markdown 字节边界死亡 + preview 禁 import
// tool → 五张闭集表手抄,同批 4 改 1 同步 = 机制上限").
//
// Precision discipline: comments are stripped before scanning (a comment may
// legitimately NAME a glyph while explaining the mechanism); everything else
// — string literals, rune literals — trips. The failure direction is
// conservative: a literal hidden behind a same-line string containing "//"
// can be missed (false green), a trip is always a true second copy.

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// uxg1TableLiterals returns every word-face literal of the five UXG-1 M1
// single-source tables, derived FROM the tables (adding a table entry
// automatically extends this tripwire).
func uxg1TableLiterals() []string {
	var out []string
	for _, mark := range tracefence.StateMarks() { // table ① state marks
		out = append(out, mark.Glyph)
	}
	out = append(out, tracefence.ActionTokens()...) // table ② action tokens
	out = append(out,                               // table ③ seat-channel phrases + badges
		tracefence.SeatChannelChainZH, tracefence.SeatChannelChainEN,
		tracefence.SeatChannelAdjacentZH, tracefence.SeatChannelAdjacentEN,
	)
	out = append(out, tracefence.BadgeGlyphs()...)
	out = append(out, tracefence.SectionOptimizationTitles()...) // table ④ headings
	out = append(out, tracefence.SectionDetailTitles()...)
	out = append(out, tracefence.SectionEvidenceTitles()...)
	out = append(out, tracefence.SectionProjectionTitles()...) // UX-ANCHOR 件a lead boundary
	out = append(out, tracefence.AuxRefMarkers()...) // table ⑤ aux markers
	// fence-head table (P0-landed single source, same discipline).
	out = append(out, tracefence.RootGlyph, tracefence.InfoToken)
	out = append(out, tracefence.TargetProvenanceChips()...)
	out = append(out, tracefence.FlatFallbackHeads()...)
	out = append(out, tracefence.ScaleNoteMarkers()...)
	return out
}

// uxg1StripComments removes Go line comments: whole-line comments vanish,
// trailing comments are cut at the first whitespace-preceded "//".
func uxg1StripComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			lines[i] = ""
			continue
		}
		if at := strings.Index(line, "\t//"); at >= 0 {
			lines[i] = line[:at]
		} else if at := strings.Index(line, " //"); at >= 0 {
			lines[i] = line[:at]
		}
	}
	return strings.Join(lines, "\n")
}

func TestUXG1PreviewKeepsNoSecondDisplayTableLiteral(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	literals := uxg1TableLiterals()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := uxg1StripComments(string(raw))
		for _, literal := range literals {
			if strings.Contains(src, literal) {
				line := 0
				for i, l := range strings.Split(src, "\n") {
					if strings.Contains(l, literal) {
						line = i + 1
						break
					}
				}
				t.Errorf("%s:%d spells display-table literal %q locally — consume the internal/tracefence constant instead (UXG-1 M1 single source)", name, line, literal)
			}
		}
	}
}
