package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The occupancy table states the same fact as the running sentence: a marker that
// records a wait is not deterministic work.
func TestHMC376OccupancyTableKeepsRecordedWaitsApartFromWork(t *testing.T) {
	row := func(span string, impact float64) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowSemantic, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "ease.cloudmusic-11077", SpanName: span, ImpactMS: impact},
		}
	}
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		row("GC: Wait For Completion Alloc", 123.089),
		row("Background concurrent copying GC", 100),
	}}
	for _, c := range []struct {
		zh                bool
		wait, work, wrong string
	}{
		{true, "已记录的等待（墙钟）", "确定性工作 span（墙钟）", "确定性工作 span 的"},
		{false, "Recorded waits (wall clock)", "Deterministic work spans (wall clock)", "of deterministic work spans"},
	} {
		got := runtimeTraceOccupancySemanticCandidates(model, c.zh)
		if len(got) != 2 {
			t.Fatalf("want both rows, got %+v", got)
		}
		if got[0].group != c.wait || strings.Contains(got[0].caliber, c.wrong) {
			t.Fatalf("recorded wait row: group=%q caliber=%q", got[0].group, got[0].caliber)
		}
		if got[1].group != c.work || !strings.Contains(got[1].caliber, c.wrong) {
			t.Fatalf("executed work row: group=%q caliber=%q", got[1].group, got[1].caliber)
		}
		if got[0].totalMS != 123.089 {
			t.Fatalf("the label must not change the value: %+v", got[0])
		}
	}
}

// A row past the badged seats has no glyph, and the sentence keeps one space.
func TestHMC376SentencePointerPastTheBadgedSeatsKeepsOneSpace(t *testing.T) {
	if got := runtimeTraceProjProsePointer(1, "E1"); got != " ➊ [E1]" {
		t.Fatalf("badged pointer=%q", got)
	}
	if got := runtimeTraceProjProsePointer(0, "E2"); got != " [E2]" {
		t.Fatalf("pointer without a glyph=%q", got)
	}
	const target = "ease.cloudmusic-11077"
	model := runtimeTraceProjTreeModel{Target: target, TreeRows: []runtimeTraceProjTreeRow{{
		HasData: true, EvidenceTag: "E2",
		Node: types.TraceCausalProjectionNode{Subject: target, StateKind: "running", SupplyFoldComputed: true, SupplyFoldDeficitMS: 0.388},
	}}}
	if _, tag, ok := runtimeTraceProjFourStateSupplyPointer(model); !ok || tag != " [E2]" {
		t.Fatalf("supply pointer tag=%q ok=%v", tag, ok)
	}
	semantic := runtimeTraceProjTreeModel{Target: target, TreeRows: []runtimeTraceProjTreeRow{{
		Kind: runtimeTraceProjTreeRowSemantic, HasData: true, EvidenceTag: "E3",
		Node: types.TraceCausalProjectionNode{Subject: target, SpanName: "JIT compiling void a.b()", EffectiveImpactMS: 1.5},
	}}}
	if tag, _, _ := runtimeTraceProjFourStateSemanticPointer(semantic); tag != " [E3]" {
		t.Fatalf("semantic pointer tag=%q", tag)
	}
}
