package tool

// answer_document_projection_rn12_13_test.go — RN-C batch pins (§7.9 runnable
// 主导场景审计 2026-07-04, docs/design/customer_dead_session_audit_20260703.md,
// cust_runnable.txt 7.0 段).
//   RN-12 — the chain/flat runnable row carries the full-window coverage
//           cross-reference tail note when the same ledger holds a
//           same-subject full-window state total beyond the exact ×1.2
//           threshold ("窗内 runnable 合计 2528.721ms(state_drilldown),链上仅
//           覆盖 top 片段 635.981ms(25%)"); no observation / near-equal total
//           → no note; the sleep class is isomorphic (per-class mechanism).
//   RN-13 — flat-fallback header explains the analysis anchor when it is not
//           the user's focused thread (customer: anchor FFRT-49706, user focus
//           pid 6565) and the next-step list gains the wakeup_chain recovery
//           row; anchor==user entity keeps the render byte-identical; no
//           typed entity → nothing appears.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// rncWindow is the shared same-window selected_window note of these pins
// (the customer's 3000ms query window) — F-2: the carrier total must name
// its own window, and the anchor-family primary row anchors the projection
// to the same window, so the 同窗 wording ("窗内") stays pinned.
const rncWindow = "selected_window=100.000000..103.000000"

// rncRunnableRecords is the customer 7.0 flat shape: one runnable-dominant
// primary row (chain top fragment 635.981ms, no wakeup edge) plus, when
// fullMS is non-empty, the same-thread full-window state_drilldown total.
func rncRunnableRecords(fullMS string) []types.ObservationRecord {
	return rncRunnableRecordsWindowed(fullMS, rncWindow)
}

// rncRunnableRecordsWindowed lets the F-2 cross-window pin vary the carrier's
// own selected_window note ("" = no note at all, the 禁猜 lane).
func rncRunnableRecordsWindowed(fullMS, carrierWindowNote string) []types.ObservationRecord {
	runnable := projV3Obs("root-ffrt", "root_cause_primary", "root_cause_primary:ffrt",
		"OS_FFRT_2_3-49706", "runnable_wait", "635.981", 635.981, 1200, 9800,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=runnable", rncWindow)
	out := []types.ObservationRecord{runnable}
	if fullMS != "" {
		notes := []string{"rank=1", "state=runnable", "impact=" + fullMS, "total=" + fullMS,
			"source=top_runnable", "chain_required=false", "recursive=true", "significant=true"}
		if carrierWindowNote != "" {
			notes = append(notes, carrierWindowNote)
		}
		out = append(out, types.ObservationRecord{
			ID: "trace_query:w1#state_drilldown:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "state_drilldown", ClaimKey: "state_drilldown:OS_FFRT_2_3-49706:runnable",
			Subject: "OS_FFRT_2_3-49706", Object: "runnable", Value: fullMS, Unit: "ms",
			Span:      types.ObservationSpan{LineStart: 1200, LineEnd: 9800},
			RichNotes: notes,
		})
	}
	return out
}

// --- RN-12: coverage cross-reference tail note --------------------------------

func TestRN12CoverageTailNoteZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), rncRunnableRecords("2528.721"), "")
	collapsed := rn1CollapseContinuations(md)
	// Customer pin: 635.981 vs 2528.721 → 25% (precise division, %.0f).
	if !strings.Contains(collapsed, "窗内 runnable 合计 2528.721ms(state_drilldown),链上仅覆盖 top 片段 635.981ms(25%)") {
		t.Fatalf("runnable chain row must carry the full-window coverage tail note:\n%s", md)
	}
}

func TestRN12CoverageTailNoteEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), rncRunnableRecords("2528.721"), "en")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "full-window runnable total 2528.721ms (state_drilldown); the chain covers only the top fragment 635.981ms (25%)") {
		t.Fatalf("EN chain row must carry the coverage tail note:\n%s", md)
	}
	if strings.Contains(md, "窗内 runnable 合计") {
		t.Fatalf("EN surface must not carry zh labels:\n%s", md)
	}
}

// Pins: no full-window observation → no note; a near-equal total (≤×1.2) →
// no note (对等值不加注).
func TestRN12CoverageNoteOnlyBeyondThreshold(t *testing.T) {
	for _, fullMS := range []string{"", "700.000", "635.981"} {
		md := audit730Render(t, audit730Bus(""), rncRunnableRecords(fullMS), "")
		for _, banned := range []string{"窗内 runnable 合计", "full-window runnable total"} {
			if strings.Contains(md, banned) {
				t.Fatalf("fullMS=%q must not render the coverage note (%q leaked):\n%s", fullMS, banned, md)
			}
		}
	}
}

// Sleep-class isomorph (same per-class mechanism, no runnable special case):
// a sleep chain row + a top_sleep family total renders the sleep wording.
func TestRN12SleepIsomorphTailNoteZH(t *testing.T) {
	sleepRow := projV3Obs("root-rs", "root_cause_primary", "root_cause_primary:rs",
		"render_service-411", "sleep_wait", "120.000", 120.0, 300, 400,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep", rncWindow)
	topSleep := types.ObservationRecord{
		ID: "trace_query:w1#top_sleep:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		Predicate: "sleep_wait", ClaimKey: "sleep_wait:render_service-411",
		Subject: "render_service-411", Object: "sleep", Value: "800.000", Unit: "ms",
		Span:      types.ObservationSpan{LineStart: 300, LineEnd: 400},
		RichNotes: []string{"state=sleep", "duration=800.000", rncWindow},
	}
	md := audit730Render(t, audit730Bus(""), []types.ObservationRecord{sleepRow, topSleep}, "")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "窗内 sleep 合计 800.000ms(top_sleep),链上仅覆盖 top 片段 120.000ms(15%)") {
		t.Fatalf("sleep chain row must carry the isomorphic coverage note:\n%s", md)
	}
}

// F-2 display pin (异窗标注): the recovery dual-window shape — carrier total
// measured in query window 831–834s, projection anchored at 100–103s — must
// label the carrier window explicitly instead of claiming "窗内" (the old
// wording put a 2528.721ms total inside a 3000ms-anchored render regardless
// of source window; with a short anchor that claim is arithmetically
// impossible).
func TestRN12CrossWindowTailNoteLabelsSourceWindowZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""),
		rncRunnableRecordsWindowed("2528.721", "selected_window=831.000000..834.000000"), "")
	collapsed := rn1CollapseContinuations(md)
	if !strings.Contains(collapsed, "另一查询窗(831.000s–834.000s)内 runnable 合计 2528.721ms(state_drilldown),链上仅覆盖 top 片段 635.981ms(25%)") {
		t.Fatalf("cross-window total must render the labeled wording:\n%s", md)
	}
	if strings.Contains(collapsed, "窗内 runnable 合计") {
		t.Fatalf("cross-window total must never claim 窗内:\n%s", md)
	}
}

// F-2 display pin (无 note 不注 — 禁猜): a carrier without a selected_window
// note renders NO coverage note in either wording.
func TestRN12NoWindowNoteRendersNothing(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), rncRunnableRecordsWindowed("2528.721", ""), "")
	for _, banned := range []string{"窗内 runnable 合计", "另一查询窗", "full-window runnable total", "another query window"} {
		if strings.Contains(md, banned) {
			t.Fatalf("carrier without selected_window must not render any coverage note (%q leaked):\n%s", banned, md)
		}
	}
}

// Unit pin: the tag builder consumes only the typed field set + the shared
// class table, and is Keep + NoTruncate + ContinuationLane.
func TestRN12CoverageTagUnit(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "OS_FFRT_2_3-49706", StateKind: "runnable", ImpactMS: 635.981,
		FullWindowStateMS: 2528.721, FullWindowStateSource: "state_drilldown",
		FullWindowStateWindowStart: 100.0, FullWindowStateWindowEnd: 103.0,
		FullWindowStateSameWindow: true,
	}
	tag, ok := runtimeTraceProjFullWindowCoverageTag(node, true)
	if !ok || tag.Text != "窗内 runnable 合计 2528.721ms(state_drilldown),链上仅覆盖 top 片段 635.981ms(25%)" {
		t.Fatalf("zh coverage tag wrong: ok=%t %q", ok, tag.Text)
	}
	if tag.DropOrder != runtimeTraceProjTagKeep || !tag.NoTruncate || !tag.ContinuationLane {
		t.Fatalf("coverage tag must be Keep + NoTruncate + ContinuationLane: %+v", tag)
	}
	if en, ok := runtimeTraceProjFullWindowCoverageTag(node, false); !ok ||
		en.Text != "full-window runnable total 2528.721ms (state_drilldown); the chain covers only the top fragment 635.981ms (25%)" {
		t.Fatalf("en coverage tag wrong: %q", en.Text)
	}
	// F-2: SameWindow=false + endpoints → the labeled wording, zh + en.
	cross := node
	cross.FullWindowStateSameWindow = false
	cross.FullWindowStateWindowStart, cross.FullWindowStateWindowEnd = 831.0, 834.0
	if tag, ok := runtimeTraceProjFullWindowCoverageTag(cross, true); !ok ||
		tag.Text != "另一查询窗(831.000s–834.000s)内 runnable 合计 2528.721ms(state_drilldown),链上仅覆盖 top 片段 635.981ms(25%)" {
		t.Fatalf("zh cross-window tag wrong: %q", tag.Text)
	}
	if tag, ok := runtimeTraceProjFullWindowCoverageTag(cross, false); !ok ||
		tag.Text != "runnable total 2528.721ms in another query window (831.000s–834.000s) (state_drilldown); the chain covers only the top fragment 635.981ms (25%)" {
		t.Fatalf("en cross-window tag wrong: %q", tag.Text)
	}
	// F-2 defensive: SameWindow=false without labelable endpoints → no tag
	// (a window claim would be a guess).
	windowless := cross
	windowless.FullWindowStateWindowStart, windowless.FullWindowStateWindowEnd = 0, 0
	if _, ok := runtimeTraceProjFullWindowCoverageTag(windowless, true); ok {
		t.Fatalf("labeled wording without endpoints must not build a tag")
	}
	node.FullWindowStateMS = 0
	if _, ok := runtimeTraceProjFullWindowCoverageTag(node, true); ok {
		t.Fatalf("zero typed field must not build a tag")
	}
	sleep := types.TraceCausalProjectionNode{
		Subject: "render_service-411", StateKind: "s_sleep", ImpactMS: 120.0,
		FullWindowStateMS: 800.0, FullWindowStateSource: "top_sleep",
		FullWindowStateWindowStart: 100.0, FullWindowStateWindowEnd: 103.0,
		FullWindowStateSameWindow: true,
	}
	if tag, ok := runtimeTraceProjFullWindowCoverageTag(sleep, true); !ok ||
		tag.Text != "窗内 sleep 合计 800.000ms(top_sleep),链上仅覆盖 top 片段 120.000ms(15%)" {
		t.Fatalf("sleep-class tag wrong: %q", tag.Text)
	}
}

// --- RN-13: flat header anchor note + recovery next-step ----------------------

func rncBusWithEntities(lang string, entities ...string) *types.BusContext {
	bus := audit730Bus(lang)
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = entities
	return bus
}

func TestRN13FlatAnchorMismatchHeaderAndNextStepZH(t *testing.T) {
	// Customer shape: anchor FFRT-49706, user focus pid 6565.
	md := audit730Render(t, rncBusWithEntities("", "6565"), rncRunnableRecords(""), "")
	if !strings.Contains(md, "(唤醒链路径未解析——按层级平铺展示)") {
		t.Fatalf("flat header must stay:\n%s", md)
	}
	if !strings.Contains(md, "- 分析锚=OS_FFRT_2_3-49706(非用户关注对象;用户关注 6565 的唤醒链未在本轮查询)") {
		t.Fatalf("customer pin: flat header must explain the analysis anchor:\n%s", md)
	}
	if !strings.Contains(md, "对用户关注线程(6565)补跑 wakeup_chain 以恢复因果树") {
		t.Fatalf("customer pin: the recovery next-step must appear:\n%s", md)
	}
}

func TestRN13FlatAnchorMismatchHeaderAndNextStepEN(t *testing.T) {
	md := audit730Render(t, rncBusWithEntities("en", "6565"), rncRunnableRecords(""), "en")
	if !strings.Contains(md, "- analysis anchor = OS_FFRT_2_3-49706 (not the user-specified focus; the wakeup chain for 6565 was not queried this round)") {
		t.Fatalf("EN flat header must explain the analysis anchor:\n%s", md)
	}
	if !strings.Contains(md, "Re-run wakeup_chain for the user-focused thread (6565) to restore the causal tree") {
		t.Fatalf("EN recovery next-step must appear:\n%s", md)
	}
	if strings.Contains(md, "分析锚=") {
		t.Fatalf("EN surface must not carry zh labels:\n%s", md)
	}
}

// Pin: anchor == user entity → byte-identical to the no-entity render; no
// typed entity → neither the note nor the next-step appears.
func TestRN13MatchedOrAbsentEntitiesKeepBytes(t *testing.T) {
	base := audit730Render(t, audit730Bus(""), rncRunnableRecords(""), "")
	for _, banned := range []string{"分析锚=", "补跑 wakeup_chain", "analysis anchor ="} {
		if strings.Contains(base, banned) {
			t.Fatalf("no-entity render must not carry the RN-13 surfaces (%q):\n%s", banned, base)
		}
	}
	for _, entities := range [][]string{{"49706"}, {"OS_FFRT_2_3"}, {"pid=49706"}} {
		matched := audit730Render(t, rncBusWithEntities("", entities...), rncRunnableRecords(""), "")
		if matched != base {
			t.Fatalf("anchor==user entity (%v) must keep the render byte-identical", entities)
		}
	}
	// Non-thread-shaped entities never build the note (roster empty).
	prose := audit730Render(t, rncBusWithEntities("", "aweme"), rncRunnableRecords(""), "")
	if prose != base {
		t.Fatalf("non-thread-shaped entities must keep the render byte-identical")
	}
}

// Unit pin: the flat-anchor lane is typed — LeadKey subject vs entities; the
// tree-mode (🎯) lane never sets the flat fields.
func TestRN13FlatAnchorFocusUnit(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		EvidenceID: "E-run", Subject: "OS_FFRT_2_3-49706", Object: "runnable",
		Predicate: "root_cause_primary", Role: types.TraceCausalRolePrimaryRootCause,
		Rank: 1, StateKind: "runnable", ChainRelevance: "on_chain",
		ImpactMS: 635.981, CumulativeImpactMS: 635.981,
	}
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{node},
		OnChainCauses:     []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.LeadKey != "E-run" || strings.TrimSpace(model.Target) != "" {
		t.Fatalf("test precondition: flat model with the runnable lead (lead=%q target=%q)", model.LeadKey, model.Target)
	}
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"6565"}})
	if !model.FlatAnchorMismatch || model.FlatAnchorThread != "OS_FFRT_2_3-49706" ||
		len(model.RootFocusUserEntities) != 1 || model.RootFocusUserEntities[0] != "6565" {
		t.Fatalf("mismatched pid entity must set the flat-anchor lane: %+v", model)
	}
	for _, entities := range [][]string{{"49706"}, {"OS_FFRT_2_3"}, {"OS_FFRT_2_3-49706"}, {"pid=49706"}, {"aweme"}, nil} {
		fresh := buildRuntimeTraceProjTreeModel(projection, nil, true)
		runtimeTraceProjApplyUserFocus(&fresh, runtimeTraceProjUserFocus{Entities: entities})
		if fresh.FlatAnchorMismatch || fresh.FlatAnchorThread != "" {
			t.Fatalf("entities %v must not set the flat-anchor lane: %+v", entities, fresh)
		}
	}
	// Tree mode keeps the R2 lane and never the flat one.
	tree := projection
	tree.WakeupPath = []string{"waker-1", "OS_FFRT_2_3-49706"}
	treeModel := buildRuntimeTraceProjTreeModel(tree, nil, true)
	runtimeTraceProjApplyUserFocus(&treeModel, runtimeTraceProjUserFocus{Entities: []string{"6565"}})
	if treeModel.FlatAnchorMismatch || treeModel.FlatAnchorThread != "" {
		t.Fatalf("tree-mode model must never set the flat-anchor fields: %+v", treeModel)
	}
	if !treeModel.RootFocusAnchorOnly {
		t.Fatalf("tree-mode mismatch must keep the existing R2 anchor-only lane: %+v", treeModel)
	}
}
