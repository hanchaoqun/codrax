package tool

// answer_document_mutation_runtime_berlin_gaps_test.go — Lane R (2026-07-03),
// docs/design/customer_dead_session_audit_20260703.md batch R2+R4+R5a:
//   R2: 🎯 root label vs typed analyzer entities (C4a), user-window ↔
//       projection-window relation line (C4b/H12), depth-1 residual fallback
//       (H10).
//   R4: adjacent stanza dedupe/fold (H6), >100% window-share annotation (H8),
//       typed aggregate impact shapes (H20), next-step final verbatim dedupe
//       (H9).
//   R5a: small-cycle ↺ annotation (H11), evidence locator artifact fallback
//       (H19).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- R2-1: root label entity comparison (C4a) --------------------------------

func berlinRootLabelModel(zh bool) runtimeTraceProjTreeModel {
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"tppmgr-300", "VSyncGenerator-2270"},
	}
	return buildRuntimeTraceProjTreeModel(projection, nil, zh)
}

func TestRuntimeTraceProjRootLabelAnchorOnlyWhenEntitiesMismatch(t *testing.T) {
	model := berlinRootLabelModel(true)
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{
		Entities: []string{"42591", "3.300", "6.600"},
	})
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "🎯 VSyncGenerator-2270 ‹分析锚点线程›") {
		t.Fatalf("mismatched root must render the anchor label:\n%s", fence)
	}
	if strings.Contains(fence, "用户关注线程") {
		t.Fatalf("mismatched root must not claim user focus:\n%s", fence)
	}
	if !strings.Contains(fence, "- 根为唤醒链锚点线程,非用户指定关注对象(用户关注: 42591)") {
		t.Fatalf("anchor-only header must carry the quiet provenance note:\n%s", fence)
	}

	en := berlinRootLabelModel(false)
	runtimeTraceProjApplyUserFocus(&en, runtimeTraceProjUserFocus{
		Entities: []string{"42591"},
	})
	enFence := runtimeTraceProjTreeFence(en, false)
	for _, want := range []string{"<analysis anchor thread>", "user focus: 42591"} {
		if !strings.Contains(enFence, want) {
			t.Fatalf("EN anchor label missing %q:\n%s", want, enFence)
		}
	}
}

func TestRuntimeTraceProjRootLabelKeptOnPreciseEntityMatch(t *testing.T) {
	cases := map[string][]string{
		"pid integer equality":  {"2270"},
		"name verbatim":         {"VSyncGenerator"},
		"whole target verbatim": {"VSyncGenerator-2270"},
	}
	for name, entities := range cases {
		model := berlinRootLabelModel(true)
		runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: entities})
		fence := runtimeTraceProjTreeFence(model, true)
		if !strings.Contains(fence, "‹用户关注线程›") || strings.Contains(fence, "分析锚点线程") {
			t.Fatalf("%s: matching root must keep the user-focus label:\n%s", name, fence)
		}
	}
	// Near-miss must NOT match: a different pid, and a substring-shaped entity.
	for name, entities := range map[string][]string{
		"different pid": {"2271"},
		"substring":     {"SyncGenerator"},
	} {
		model := berlinRootLabelModel(true)
		runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: entities})
		if !model.RootFocusAnchorOnly {
			t.Fatalf("%s: near-miss entity must not count as a user-focus match", name)
		}
	}
}

// RF1 (adversarial review 2026-07-03): the root comparison must recognize the
// "pid=N" handle form on BOTH sides — analyzer entities may arrive as
// "pid=42591", and traceThreadLabel emits the target itself as "pid=N" when
// the Comm was never resolved (every wakeup-path label passes through it).
func TestRuntimeTraceProjRootLabelPidHandleForms(t *testing.T) {
	// (a) entity side: pid=N entity vs name-pid target.
	if !runtimeTraceProjTargetMatchesUserEntities("VSyncGenerator-2270", []string{"pid=2270"}) {
		t.Fatalf("pid=N entity must match the target's -pid tail")
	}
	// (b) target side: bare pid=N target vs pure-digit and pid=N entities.
	for _, entity := range []string{"42591", "pid=42591"} {
		if !runtimeTraceProjTargetMatchesUserEntities("pid=42591", []string{entity}) {
			t.Fatalf("pid=N target must match entity %q", entity)
		}
	}
	// Character-class negatives: broken prefix / non-digit tail / other pid.
	for _, entity := range []string{"pidx=42591", "pid=42591abc", "pid=42592"} {
		if runtimeTraceProjTargetMatchesUserEntities("pid=42591", []string{entity}) {
			t.Fatalf("malformed or mismatched handle %q must not match", entity)
		}
		if runtimeTraceProjTargetMatchesUserEntities("VSyncGenerator-42591", []string{entity}) {
			t.Fatalf("malformed or mismatched handle %q must not match the name-pid target", entity)
		}
	}
	// Fence-level: a pid=N target keeps the user-focus label on a match…
	model := runtimeTraceProjTreeModel{Target: "pid=42591"}
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"pid=42591"}})
	if model.RootFocusAnchorOnly {
		t.Fatalf("matching pid=N target must keep the user-focus label")
	}
	// …and the anchor-only roster accepts pid=N-shaped entities.
	mismatch := runtimeTraceProjTreeModel{Target: "VSyncGenerator-2270"}
	runtimeTraceProjApplyUserFocus(&mismatch, runtimeTraceProjUserFocus{Entities: []string{"pid=42591"}})
	if !mismatch.RootFocusAnchorOnly {
		t.Fatalf("different pid handle must demote to anchor-only")
	}
	if len(mismatch.RootFocusUserEntities) != 1 || mismatch.RootFocusUserEntities[0] != "pid=42591" {
		t.Fatalf("roster must accept the pid=N form: %+v", mismatch.RootFocusUserEntities)
	}
	if got := runtimeTraceProjThreadOrPidEntities([]string{"pidx=42591", "pid=42591abc"}); len(got) != 0 {
		t.Fatalf("roster must reject malformed handles: %+v", got)
	}
}

func TestRuntimeTraceProjRootLabelFailsOpenWithoutEntityContext(t *testing.T) {
	model := berlinRootLabelModel(true)
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{})
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "‹用户关注线程›") || strings.Contains(fence, "分析锚点") {
		t.Fatalf("no typed entity context must keep the legacy label (fail-open):\n%s", fence)
	}
}

// --- R2-2: user-window ↔ projection-window relation line ----------------------

func TestRuntimeTraceProjUserWindowRelationLine(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"tppmgr-300", "VSyncGenerator-2270"},
		WindowStartTs: 3.300,
		WindowEndTs:   3.401, // 101ms projection sub-window
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{
		Entities: []string{"42591", "3.300", "6.600"},
	})
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "- 用户请求窗 3.300s → 6.600s(共 3.3s);本投影取其中代表性子窗,全窗指标见 Trace 指标快照") {
		t.Fatalf("small sub-window must state the user-window relation:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "User-requested window 3.300s → 6.600s (3.3s total)") {
		t.Fatalf("EN relation line missing:\n%s", en)
	}

	// Trailing seconds unit on the timestamp entities works the same.
	suffixed := buildRuntimeTraceProjTreeModel(projection, nil, true)
	runtimeTraceProjApplyUserFocus(&suffixed, runtimeTraceProjUserFocus{
		Entities: []string{"3.300s", "6.600s"},
	})
	if !strings.Contains(runtimeTraceProjWindowLine(projection, suffixed, true), "用户请求窗 3.300s → 6.600s") {
		t.Fatalf("timestamp entities with the s unit must be accepted")
	}
}

func TestRuntimeTraceProjUserWindowRelationLineSuppressed(t *testing.T) {
	base := types.TraceCausalProjection{WakeupPath: []string{"tppmgr-300", "VSyncGenerator-2270"}}
	cases := []struct {
		name     string
		startTs  float64
		endTs    float64
		entities []string
	}{
		// 1.8s projection ≥ 50% of the 3.3s user window → no relation line.
		{"projection not a small sub-window", 3.300, 5.100, []string{"3.300", "6.600"}},
		// Three timestamp-shaped entities → ambiguous pair → no line.
		{"ambiguous timestamp triple", 3.300, 3.401, []string{"3.300", "6.600", "9.900"}},
		// Non-increasing pair → no line.
		{"non-increasing pair", 3.300, 3.401, []string{"6.600", "3.300"}},
		// No projection window at all → no line.
		{"no projection window", 0, 0, []string{"3.300", "6.600"}},
	}
	for _, tc := range cases {
		projection := base
		projection.WindowStartTs, projection.WindowEndTs = tc.startTs, tc.endTs
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: tc.entities})
		if line := runtimeTraceProjWindowLine(projection, model, true); strings.Contains(line, "用户请求窗") {
			t.Fatalf("%s: relation line must not render:\n%s", tc.name, line)
		}
	}
}

// --- R2-3: depth-1 residual fallback (H10) ------------------------------------

func TestRuntimeTraceProjDepth1CumulativeFallsBackToShallowestDataDepth(t *testing.T) {
	// Berlin shape: every depth-1 trunk node is a bare transit hop; the first
	// data-carrying chain layer sits deeper. The coverage subtraction must fall
	// back to that shallowest data depth instead of silently dropping the
	// attributed/residual line.
	model := runtimeTraceProjTreeModel{
		Target:   "VSyncGenerator-2270",
		WindowMS: 101.0,
		TreeRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: false},
			{Kind: runtimeTraceProjTreeRowChain, Depth: 2, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "tppmgr-300", CumulativeImpactMS: 2.891}},
			{Kind: runtimeTraceProjTreeRowChain, Depth: 2, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "other-1", CumulativeImpactMS: 1.100}},
			// A deeper, larger layer must NOT win over the shallowest data depth.
			{Kind: runtimeTraceProjTreeRowChain, Depth: 3, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "deep-9", CumulativeImpactMS: 50.0}},
		},
	}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 2.891 {
		t.Fatalf("fallback must take the shallowest data depth max, got %v", got)
	}
	projection := types.TraceCausalProjection{WindowStartTs: 3.300, WindowEndTs: 3.401}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "on-chain 已归因 2.891ms") || !strings.Contains(line, "未归因残差") {
		t.Fatalf("berlin shape must still render the attributed/residual coverage line:\n%s", line)
	}
	// Depth-1 data still wins when present.
	model.TreeRows[0].HasData = true
	model.TreeRows[0].Node = types.TraceCausalProjectionNode{Subject: "d1", CumulativeImpactMS: 7.7}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 7.7 {
		t.Fatalf("depth-1 data must keep priority, got %v", got)
	}
}

// --- R4-1: adjacent stanza dedupe + duplicate-measurement fold (H6) -----------

// Pin updated 2026-07-03 (adversarial review RF2a): the strict equal-value
// fold now additionally requires a precise line/time overlap. The old fixture
// used disjoint line ranges (100-103 vs 105-108) and no longer represents a
// foldable pair — the REAL customer rows E11/E12 spanned 793201-830007 vs
// 793204-830012 (overlapping), so the fixture now carries the true shape.
func TestRuntimeTraceProjAdjacentStanzaDedupesAndFoldsDuplicates(t *testing.T) {
	irq := func(id string, impact float64, lineStart, lineEnd int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: id, Subject: "irq_handler-100", Object: "irq_activity",
			ImpactMS: impact, CumulativeImpactMS: impact,
			LineStart: lineStart, LineEnd: lineEnd, ChainRelevance: "adjacent",
		}
	}
	projection := types.TraceCausalProjection{
		AdjacentCauses: []types.TraceCausalProjectionNode{
			irq("E11", 35.350, 793201, 830007),
			irq("E12", 35.350, 793204, 830012), // customer E11/E12: identical ms, overlapping ranges
			irq("E11", 35.350, 793201, 830007), // exact node-key duplicate
			irq("E13", 12.000, 300, 303),       // different ms → stays its own row
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if len(model.Adjacent) != 2 {
		t.Fatalf("adjacent stanza must dedupe+fold to 2 rows, got %d: %+v", len(model.Adjacent), model.Adjacent)
	}
	folded := model.Adjacent[0].Node
	// Pin updated 2026-07-03 (V4): the fold provenance moved from the row-local
	// DedupFold flag + MergedCount to the typed Node.DuplicatePublications field
	// (one home shared with the aggregation layer's pre-R2 dedup pass);
	// MergedCount stays reserved for SUM aggregates and must remain untouched.
	if folded.EvidenceID != "E11" || folded.DuplicatePublications != 2 {
		t.Fatalf("first occurrence must survive with DuplicatePublications=2: %+v", folded)
	}
	if folded.MergedCount != 0 {
		t.Fatalf("duplicate fold must not claim SUM-aggregate MergedCount semantics: %+v", folded)
	}
	if len(folded.MergedEvidenceIDs) != 1 || folded.MergedEvidenceIDs[0] != "E12" {
		t.Fatalf("duplicate's evidence id must fold in: %+v", folded.MergedEvidenceIDs)
	}
	if folded.ImpactMS != 35.350 {
		t.Fatalf("duplicate fold must never sum the wall clock: %v", folded.ImpactMS)
	}
	if model.Adjacent[1].Node.EvidenceID != "E13" {
		t.Fatalf("distinct measurement must stay a separate row: %+v", model.Adjacent[1].Node)
	}
	if model.Adjacent[1].Node.DuplicatePublications > 1 {
		t.Fatalf("unfolded row must not carry DuplicatePublications")
	}
}

// RF2a negative pins: strictly equal values must NOT fold without a precise
// overlap — two real irq bursts at different moments can quantize to the same
// %.3f ms, and folding them would halve the reported contribution.
func TestRuntimeTraceProjAdjacentSameValueWithoutOverlapStaysSeparate(t *testing.T) {
	base := func(id string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: id, Subject: "irq_handler-100", Object: "irq_activity",
			ImpactMS: 35.350, CumulativeImpactMS: 35.350, ChainRelevance: "adjacent",
		}
	}
	cases := []struct {
		name string
		mut  func(a, b *types.TraceCausalProjectionNode)
	}{
		{"disjoint line ranges", func(a, b *types.TraceCausalProjectionNode) {
			a.LineStart, a.LineEnd = 100, 103
			b.LineStart, b.LineEnd = 105, 108
		}},
		{"disjoint time spans", func(a, b *types.TraceCausalProjectionNode) {
			a.StartTs, a.EndTs = 3.100, 3.135
			b.StartTs, b.EndTs = 3.200, 3.235
		}},
		// Neither lane determinate → fail open to two rows, never fold on
		// value equality alone.
		{"no location info at all", func(a, b *types.TraceCausalProjectionNode) {}},
	}
	for _, tc := range cases {
		a, b := base("E11"), base("E12")
		tc.mut(&a, &b)
		projection := types.TraceCausalProjection{
			AdjacentCauses: []types.TraceCausalProjectionNode{a, b},
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		if len(model.Adjacent) != 2 {
			t.Fatalf("%s: same-value rows without precise overlap must stay 2 rows, got %d: %+v",
				tc.name, len(model.Adjacent), model.Adjacent)
		}
		for _, row := range model.Adjacent {
			if row.Node.MergedCount > 1 || row.Node.DuplicatePublications > 1 {
				t.Fatalf("%s: no row may claim a fold: %+v", tc.name, row)
			}
		}
	}
	// The time-span lane folds on its own when line info is absent.
	// (Pin updated 2026-07-03, V4: fold count reads Node.DuplicatePublications.)
	a, b := base("E11"), base("E12")
	a.StartTs, a.EndTs = 3.100, 3.135
	b.StartTs, b.EndTs = 3.120, 3.155
	model := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		AdjacentCauses: []types.TraceCausalProjectionNode{a, b},
	}, nil, true)
	if len(model.Adjacent) != 1 || model.Adjacent[0].Node.DuplicatePublications != 2 {
		t.Fatalf("overlapping time spans must fold: %+v", model.Adjacent)
	}
}

// RF2b: the H6 dedupe fold and the upstream R2 sum aggregate must never share
// one ×N rendering form — a dedupe row's ms is a single measurement, an R2
// row's ms is a total. Pinned on the stanza tag AND the detail-table cell.
func TestRuntimeTraceProjDedupFoldLabelDistinctFromSumAggregateLabel(t *testing.T) {
	dedupe := types.TraceCausalProjectionNode{
		EvidenceID: "E11", Subject: "irq_handler-100", Object: "irq_activity",
		ImpactMS: 35.350, CumulativeImpactMS: 35.350, ChainRelevance: "adjacent",
		LineStart: 793201, LineEnd: 830007,
	}
	dup := dedupe
	dup.EvidenceID = "E12"
	dup.LineStart, dup.LineEnd = 793204, 830012
	sum := types.TraceCausalProjectionNode{
		EvidenceID: "E20", Subject: "binder-7", Object: "state_churn",
		ImpactMS: 90.000, CumulativeImpactMS: 90.000, ChainRelevance: "adjacent",
		MergedCount: 3, MergedMinMS: 20.000, MergedMaxMS: 40.000,
	}
	model := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		AdjacentCauses: []types.TraceCausalProjectionNode{dedupe, dup, sum},
	}, nil, true)
	if len(model.Adjacent) != 2 {
		t.Fatalf("expected folded row + upstream aggregate, got %+v", model.Adjacent)
	}
	// The tag build point is the shared format site (the stanza line may
	// legitimately width-elide Extra tags; the detail table below stays the
	// lossless surface).
	joinTags := func(row runtimeTraceProjTreeRow, zh bool) string {
		_, tags := runtimeTraceProjRowMetricParts(row, 0, false, zh)
		var parts []string
		for _, tag := range tags {
			parts = append(parts, tag.Text)
		}
		return strings.Join(parts, " · ")
	}
	foldTags := joinTags(model.Adjacent[0], true)
	if !strings.Contains(foldTags, "×2同值合并(重复发布)") {
		t.Fatalf("dedupe row must carry the dedupe-exclusive label:\n%s", foldTags)
	}
	if strings.Contains(foldTags, "合并·单次") {
		t.Fatalf("dedupe row must not reuse the R2 sum form:\n%s", foldTags)
	}
	sumTags := joinTags(model.Adjacent[1], true)
	if !strings.Contains(sumTags, "×3合并·单次20.000–40.000ms") {
		t.Fatalf("upstream R2 aggregate must keep the sum form:\n%s", sumTags)
	}
	if strings.Contains(sumTags, "同值合并") {
		t.Fatalf("upstream R2 aggregate must not claim the dedupe label:\n%s", sumTags)
	}
	// EN surfaces fork the same way.
	enFold := joinTags(model.Adjacent[0], false)
	if !strings.Contains(enFold, "×2 same-value merge (duplicate publication)") || strings.Contains(enFold, "merged · each") {
		t.Fatalf("EN dedupe label wrong:\n%s", enFold)
	}
	// Detail table mirrors the fork on the node cell.
	_, rows := runtimeTraceProjDetailTable(model, true)
	var foldCell, sumCell string
	for _, item := range rows {
		switch {
		case strings.Contains(item.Cells[2], "irq_handler"):
			foldCell = item.Cells[2]
		case strings.Contains(item.Cells[2], "binder-7"):
			sumCell = item.Cells[2]
		}
	}
	if !strings.Contains(foldCell, "×2同值合并(重复发布)") || strings.Contains(foldCell, "×2(") {
		t.Fatalf("detail table dedupe cell must use the dedupe label: %q", foldCell)
	}
	if !strings.Contains(sumCell, "×3(20.000–40.000ms)") || strings.Contains(sumCell, "同值合并") {
		t.Fatalf("detail table sum cell must keep the range form: %q", sumCell)
	}
}

// --- R4-3: >100% window share annotation (H8) ----------------------------------

func TestRuntimeTraceProjOverWindowPercentAnnotated(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowBackground, HasData: true,
		Node: types.TraceCausalProjectionNode{
			Subject: "irq/151-dpu", Object: "irq_burst", ImpactMS: 204.382,
			ChainRelevance: "background",
		},
	}
	line := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 101.0, true, true)
	if !strings.Contains(line, "202%(跨CPU/多段累计)") {
		t.Fatalf("over-window share must carry the cumulative annotation:\n%s", line)
	}
	en := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 101.0, true, false)
	if !strings.Contains(en, "202% (multi-CPU/multi-span cumulative)") {
		t.Fatalf("EN over-window share must carry the annotation:\n%s", en)
	}
	// In-window shares stay unannotated.
	row.Node.ImpactMS = 50.0
	if line := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 101.0, true, true); strings.Contains(line, "跨CPU") {
		t.Fatalf("in-window share must not be annotated:\n%s", line)
	}
}

// --- R4-4: typed aggregate impact shapes (H20) ---------------------------------

func TestRuntimeTraceCausalProjectionImpactShapeTypedAggregates(t *testing.T) {
	cases := []struct {
		node types.TraceCausalProjectionNode
		zh   bool
		want string
	}{
		{types.TraceCausalProjectionNode{Object: "irq_burst"}, true, "IRQ突发"},
		{types.TraceCausalProjectionNode{Object: "irq_activity"}, true, "IRQ活动"},
		{types.TraceCausalProjectionNode{Object: "page_cache_churn"}, true, "页缓存抖动"},
		{types.TraceCausalProjectionNode{TypeToken: "irq_burst"}, true, "IRQ突发"},
		{types.TraceCausalProjectionNode{Object: "irq_burst"}, false, "IRQ burst"},
		{types.TraceCausalProjectionNode{Object: "irq_activity"}, false, "IRQ activity"},
		{types.TraceCausalProjectionNode{Object: "page_cache_churn"}, false, "page-cache churn"},
	}
	for _, tc := range cases {
		if got := runtimeTraceCausalProjectionImpactShapeCell(tc.node, tc.zh); got != tc.want {
			t.Fatalf("impact shape for %+v zh=%v: got %q want %q", tc.node, tc.zh, got, tc.want)
		}
	}
	// A typed dominant state still wins over the aggregate token (the H20 lane
	// only replaces the generic candidate fallback).
	withState := types.TraceCausalProjectionNode{Object: "irq_burst", StateKind: "running"}
	if got := runtimeTraceCausalProjectionImpactShapeCell(withState, true); got != "running / CPU执行" {
		t.Fatalf("dominant state must keep priority over the aggregate token: %q", got)
	}
	// Unmapped tokens keep the generic fallback.
	if got := runtimeTraceCausalProjectionImpactShapeCell(types.TraceCausalProjectionNode{Object: "workqueue_activity"}, true); got != "候选影响" {
		t.Fatalf("unmapped tokens must keep the generic fallback: %q", got)
	}
}

// --- R4-5: next-step final verbatim dedupe (H9) --------------------------------

func TestRuntimeTraceNextStepItemsFinalVerbatimDedupe(t *testing.T) {
	mk := func(id, prose string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "state_churn",
			Subject: "app-100", Value: "5.000", Unit: "ms",
			RichNotes: []string{"next_step=" + prose, "next_step_kind=s_sleep"},
		}
	}
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			mk("ns-1", "inspect peer A waking it repeatedly"),
			mk("ns-2", "inspect peer B waking it repeatedly"),
		},
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	// ZH: both typed payloads localize to the same fixed s_sleep guidance — the
	// typed keys differ (裁定5 keeps them apart) but the rendered lines are
	// byte-identical, so the final verbatim layer must drop the duplicate.
	items := runtimeTraceNextStepItems(doc, bus)
	if len(items) != 1 {
		t.Fatalf("verbatim-identical rendered steps must dedupe to one item: %+v", items)
	}
	// EN keeps the system prose verbatim — the two lines differ, so both stay
	// (proof the final layer never over-merges distinct rendered text).
	enBus := newBusForMutationTest()
	enBus.Language = "en"
	enBus.AnalysisIR = &types.AnalysisIR{AnswerContract: types.AnswerContract{Language: "en"}}
	enBus.ToolResults = bus.ToolResults
	if enItems := runtimeTraceNextStepItems(doc, enBus); len(enItems) != 2 {
		t.Fatalf("distinct rendered EN steps must both survive: %+v", enItems)
	}
}

// --- R5a-1: small-cycle ↺ annotation (H11) --------------------------------------

func TestRuntimeTraceProjSmallCycleRecursAnnotation(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"VSyncGenerator-2270", "tppmgr-300", "VSyncGenerator-2270"},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	fence := runtimeTraceProjTreeFence(model, true)
	if got := strings.Count(fence, "↺(线程在链上重复出现)"); got != 1 {
		t.Fatalf("exactly the repeated trunk occurrence must carry the ↺ marker, got %d:\n%s", got, fence)
	}
	// The chain itself is never truncated by the marker: both trunk rows render.
	for _, want := range []string{"tppmgr-300", "VSyncGenerator-2270"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("chain node %q must stay rendered:\n%s", want, fence)
		}
	}
	en := buildRuntimeTraceProjTreeModel(projection, nil, false)
	if !strings.Contains(runtimeTraceProjTreeFence(en, false), "↺ (recurs on chain)") {
		t.Fatalf("EN recurs marker missing")
	}
	// No repeat → no marker.
	clean := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		WakeupPath: []string{"worker-1", "app-2"},
	}, nil, true)
	if strings.Contains(runtimeTraceProjTreeFence(clean, true), "↺") {
		t.Fatalf("clean chain must not carry recurs markers")
	}
}

// --- R5a-3: evidence locator artifact fallback (H19) ----------------------------

func TestRuntimeTraceProjEvidenceLocatorAdoptsSoleArtifactForBareRefs(t *testing.T) {
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	evidence.add(types.TraceCausalProjectionNode{
		EvidenceID: "obs-named", Subject: "binder-1",
		SupportRefs: []string{"berlin.systrace:104233-104391"},
	}, true)
	evidence.add(types.TraceCausalProjectionNode{
		EvidenceID: "obs-bare", Subject: "irq-2",
		LineStart: 794198, LineEnd: 827402,
	}, true)
	intro, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
	flat := intro
	for _, item := range items {
		flat += "\n" + item.Text
	}
	if strings.Contains(flat, "lines=") {
		t.Fatalf("sole-artifact roster must not show naked lines= locators:\n%s", flat)
	}
	if !strings.Contains(flat, "全部证据位于 `berlin.systrace`") || !strings.Contains(flat, ":794198-827402") {
		t.Fatalf("bare ref must adopt the sole artifact and keep its range:\n%s", flat)
	}
}

func TestRuntimeTraceProjEvidenceLocatorKeepsBareRefWhenArtifactsAmbiguous(t *testing.T) {
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	evidence.add(types.TraceCausalProjectionNode{
		EvidenceID: "obs-a", Subject: "binder-1",
		SupportRefs: []string{"berlin.systrace:104233-104391"},
	}, true)
	evidence.add(types.TraceCausalProjectionNode{
		EvidenceID: "obs-b", Subject: "net-3",
		SupportRefs: []string{"other.trace:5-9"},
	}, true)
	evidence.add(types.TraceCausalProjectionNode{
		EvidenceID: "obs-bare", Subject: "irq-2",
		LineStart: 794198, LineEnd: 827402,
	}, true)
	_, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
	flat := ""
	for _, item := range items {
		flat += item.Text + "\n"
	}
	if !strings.Contains(flat, "lines=794198-827402") {
		t.Fatalf("ambiguous artifacts must keep the bare form instead of guessing:\n%s", flat)
	}
}
