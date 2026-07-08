package tool

// answer_document_mutation_runtime_dcs_test.go — DCS 修复批 display-half pins
// (ledger real_trace_campaign_20260705.md §23/§23.1, 2026-07-08):
//
//   - E5: a semantic row carrying a typed SOURCE query window differing from
//     the anchor re-bases its window-share % on that source window and wears
//     the 来自查询窗 tag (cmp_01 E2 witness: "83.893ms · 83%" against an
//     anchor window the span never touched). Control: same/absent source
//     window keeps the legacy anchor share byte-identically.
//   - E1 display words: tier=deterministic_optimization rank rows speak the
//     确定性优化 display family (确定优化 / 确定性优化点), and the tier's
//     root_cause_deterministic_optimization predicate NEVER enters the
//     primary-roots bucket (负向: 不与 primary 选举竞争).
//   - E6/F3b: the comparison overview grows the 确定性优化点 column (typed
//     SemanticSpans data via the shared LEAD-SEM selector), the zero-chain
//     primary cell carries the presence 括注 pointing at that column, and the
//     LEAD-SEM semantic-fallback lane never double-writes the note.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// dcsSemanticSourceWindowProjection is the cmp_01 E2 shape (6.0B138, ledger
// §23 H2): anchor window 8144.608–8144.708 (100ms); one JIT semantic row of
// 83.893ms measured over a DIFFERENT typed query window 8144.358–8145.242
// (884ms) and physically outside the anchor window.
func dcsSemanticSourceWindowProjection() types.TraceCausalProjection {
	out := false
	return types.TraceCausalProjection{
		WindowStartTs:           8144.608,
		WindowEndTs:             8144.708,
		RootCauseFamilyObserved: true,
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "dcs-sem",
			Subject: "线程名未记录(tid 10154)", Predicate: "trace_semantic_span",
			Object: "jit_compile", SpanName: "JIT compiling android.content.pm.Foo",
			SemanticClass: "jit_compile", ImpactMS: 83.893, CumulativeImpactMS: 83.893,
			StartTs: 8144.972, EndTs: 8145.056,
			QueryWindowStartTs: 8144.358, QueryWindowEndTs: 8145.242,
			WithinRequestedWindow: &out,
			LineStart:             769, LineEnd: 799, Confidence: 0.66,
		}},
	}
}

// E5 positive: the % divides by the row's own 884ms source window (9%), the
// 来自查询窗 tag disclosures the base, and the anchor-based 83%/84% form is
// gone. Mutation guard: reverting the source-window lane resurrects "84%".
func TestDCSSemanticSourceWindowShareRebasesOnSourceWindow(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(dcsSemanticSourceWindowProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "9%") {
		t.Fatalf("semantic row must re-base its share on the 884ms source window (83.893/884≈9%%):\n%s", fence)
	}
	if strings.Contains(fence, "83%") || strings.Contains(fence, "84%") {
		t.Fatalf("the anchor-based share must not render for a differing source window:\n%s", fence)
	}
	if !strings.Contains(fence, "来自查询窗 8144.358–8145.242s") {
		t.Fatalf("the re-based share must disclose its source query window inline:\n%s", fence)
	}
	if model.Marks == nil || !model.Marks.has(runtimeTraceProjMarkSemanticSourceWindowShare) {
		t.Fatalf("the source-window share mark must be recorded for the legend")
	}
}

// E5 control (负向): without a typed source window the legacy anchor share
// stays byte-identical — no tag, no mark, 84% (83.893/100ms) against the
// anchor. Absence never guesses a window.
func TestDCSSemanticShareControlWithoutSourceWindowKeepsAnchorBase(t *testing.T) {
	projection := dcsSemanticSourceWindowProjection()
	projection.SemanticSpans[0].QueryWindowStartTs = 0
	projection.SemanticSpans[0].QueryWindowEndTs = 0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "84%") {
		t.Fatalf("without a source window the legacy anchor share must stay:\n%s", fence)
	}
	if strings.Contains(fence, "来自查询窗") {
		t.Fatalf("no typed source window → no source-window tag:\n%s", fence)
	}
	if model.Marks != nil && model.Marks.has(runtimeTraceProjMarkSemanticSourceWindowShare) {
		t.Fatalf("the source-window mark must not record on the legacy lane")
	}
	// Same-window control: a source window equal to the anchor (±1ms) keeps
	// the legacy lane too.
	projection.SemanticSpans[0].QueryWindowStartTs = 8144.608
	projection.SemanticSpans[0].QueryWindowEndTs = 8144.708
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "来自查询窗") {
		t.Fatalf("a source window matching the anchor must keep the legacy share lane:\n%s", fence)
	}
}

// E1 display words: the deterministic_optimization tier speaks the 确定性优化
// display family on both detail-table cells, aligned with the semantic
// observation lane — never the on_chain root-cause words (重点关注/链上).
func TestDCSDeterministicOptimizationTierDisplayWords(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Predicate: "root_cause_deterministic_optimization", Tier: "deterministic_optimization",
		Object: "jit_compile", SemanticClass: "jit_compile", ChainRelevance: "on_chain",
	}
	if got := runtimeTraceCausalProjectionPriorityCell(node, true); got != "确定优化" {
		t.Fatalf("zh priority cell: got %q want 确定优化", got)
	}
	if got := runtimeTraceCausalProjectionPriorityCell(node, false); got != "optimize" {
		t.Fatalf("en priority cell: got %q want optimize", got)
	}
	if got := runtimeTraceCausalProjectionLayerCell(node, true); got != "确定性优化点" {
		t.Fatalf("zh layer cell: got %q want 确定性优化点", got)
	}
	if got := runtimeTraceCausalProjectionLayerCell(node, false); got != "deterministic optimization" {
		t.Fatalf("en layer cell: got %q want deterministic optimization", got)
	}
}

// E1 负向 (不与 primary 选举竞争, display half): a rank record wearing the
// root_cause_deterministic_optimization predicate compiles into the context
// lane, never the primary-roots bucket the conclusion elects from.
func TestDCSDeterministicOptimizationPredicateNeverPrimaryBucket(t *testing.T) {
	record := projV3Obs("dcs-opt", "root_cause_deterministic_optimization",
		"root_cause_deterministic_optimization:jit", "worker-200", "jit_compile",
		"5.600", 5.6, 3, 6,
		"rank=2", "tier=deterministic_optimization", "chain_relevance=on_chain",
		"causality=on_wakeup_chain", "semantic_class=jit_compile",
		"span_name=JIT compiling com.example.Foo")
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	if len(projection.PrimaryRootCauses) != 0 {
		t.Fatalf("deterministic_optimization rows must never enter the primary bucket: %+v", projection.PrimaryRootCauses)
	}
	if len(projection.OnChainCauses)+len(projection.AdjacentCauses)+len(projection.BackgroundCauses) == 0 {
		t.Fatalf("deterministic_optimization rows must still compile as ranked context: %+v", projection)
	}
}

// E6/F3b: the comparison overview grows the 确定性优化点 column with the top
// semantic span per artifact ("—" on the span-less side), and the zero-chain
// side's primary cell carries the presence 括注 pointing at the column.
func TestDCSCompareOverviewOptimizationColumnAndPresenceNote(t *testing.T) {
	withSpans := dcsSemanticSourceWindowProjection()
	withSpans.ArtifactLabel = "6.0B138_3900.sys.systrace"
	// The fixture's primary bucket is EMPTY, so the cell walks the 未定位
	// branch (the demoted-primary shape rides the LEAD-SEM lane instead —
	// pinned by the no-double-write test below).
	withoutSpans := types.TraceCausalProjection{
		ArtifactLabel:           "7.0B30SP22_7315.systrace",
		WindowStartTs:           3680.818,
		WindowEndTs:             3680.919,
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "dcs-a-run",
			Subject: "NetworkStats-9060", Predicate: "root_cause_primary",
			Object: "blocking_span", Rank: 1, ChainRelevance: "on_chain",
			Causality: "on_wakeup_chain", ImpactMS: 47.562, CumulativeImpactMS: 47.562,
			Confidence: 0.9,
		}},
	}
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{withoutSpans, withSpans}, types.ObservationLedger{}, "zh-CN", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("comparison overview must render for two projections")
	}
	joinedColumns := strings.Join(block.Columns, "|")
	if !strings.Contains(joinedColumns, "确定性优化点") {
		t.Fatalf("overview must grow the 确定性优化点 column when a side carries semantic spans: %v", block.Columns)
	}
	var flat []string
	for _, row := range block.Items {
		flat = append(flat, strings.Join(row.Cells, "|"))
	}
	table := strings.Join(flat, "\n")
	if !strings.Contains(table, "JIT compiling android.content.pm.Foo 83.893ms") {
		t.Fatalf("the span side's optimization cell must name the top semantic span:\n%s", table)
	}
	if !strings.Contains(table, "占其查询窗9%") {
		t.Fatalf("the optimization cell share must ride the E5 source-window base:\n%s", table)
	}
	if !strings.Contains(table, "—") {
		t.Fatalf("the span-less side's optimization cell must stay —:\n%s", table)
	}
	if !strings.Contains(table, "存在确定性优化点: JIT compiling android.content.pm.Foo 83.893ms") ||
		!strings.Contains(table, "见优化点列") {
		t.Fatalf("the zero-chain primary cell must carry the presence 括注 pointing at the column:\n%s", table)
	}
}

// F3b 负向 (mutation guard: column 禁用→咬红): with no semantic spans on any
// side the column never renders — an all-“—” column would widen every
// non-compile comparison for nothing.
func TestDCSCompareOverviewOptimizationColumnAbsentWithoutSemanticSpans(t *testing.T) {
	a := types.TraceCausalProjection{
		ArtifactLabel: "a.systrace", WindowStartTs: 1.0, WindowEndTs: 1.1,
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "a-1",
			Subject: "w-1", Predicate: "root_cause_primary", Object: "running",
			Rank: 1, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			ImpactMS: 12, Confidence: 0.9,
		}},
	}
	b := a
	b.ArtifactLabel = "b.systrace"
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{a, b}, types.ObservationLedger{}, "zh-CN", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("comparison overview must render for two projections")
	}
	for _, column := range block.Columns {
		if strings.Contains(column, "确定性优化点") {
			t.Fatalf("no semantic spans anywhere → no optimization column: %v", block.Columns)
		}
	}
}

// E6/F3b LEAD-SEM 协调 (负向, no double-write): when the semantic-fallback
// lane leads the primary cell, the cell already NAMES the span — the presence
// 括注 must not stack a second mention.
func TestDCSCompareOverviewPresenceNoteNeverDoubleWritesSemanticLead(t *testing.T) {
	projection := dcsSemanticSourceWindowProjection()
	// Primary bucket non-empty but demoted (aggregate-metric background shape)
	// → runtimeTraceProjLeadSelect resolves the semantic-fallback lane.
	projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "dcs-bg",
		Subject: "unknown-thread", Predicate: "root_cause_primary",
		Object:      "supply_pressure",
		SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		Rank:        1, ImpactMS: 45375.684, Confidence: 0.58,
	}}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLaneSemanticFallback {
		t.Fatalf("fixture must resolve the semantic-fallback lane, got lane=%d lead=%v", lane, lead)
	}
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "窗口内最大语义优化span") {
		t.Fatalf("the semantic lead must speak the LEAD-SEM single-source wording: %q", cell)
	}
	if strings.Contains(cell, "存在确定性优化点") {
		t.Fatalf("the presence 括注 must never double-write beside the semantic lead: %q", cell)
	}
}
