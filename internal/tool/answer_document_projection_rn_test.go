package tool

// answer_document_projection_rn_test.go — RN-A batch pins (§7.9 runnable
// 主导场景审计 2026-07-04, docs/design/customer_dead_session_audit_20260703.md,
// cust_runnable.txt). Customer 7.0 shape: the model anchored on
// OS_FFRT_2_3-49706 (runnable-dominant, sleep=0, no wakeup edge) → a flat
// single-row projection (runnable 635.981ms / 42% of the window) while the
// engine's tier=primary/rank=1 supply_pressure aggregate sat in the
// background stanza labeled 主根因 · 主要关注 and the conclusion said no
// on-chain primary was located.
//   RN-3(a) — lead falls back to the largest data-bearing on-chain row when
//             every primary-bucket candidate demoted (short note appended);
//   RN-3(b) — the 主根因 · 主要关注 position label only follows the node the
//             conclusion actually consumed; an unconsumed primary-tier
//             background row demotes to 背景 · 支撑参考(rank=N);
//   RN-2b  — no anchor window → no ⚠跨窗 anywhere (tree tag, detail mirror,
//             legend via the typed mark);
//   RN-6   — the symptom family includes runnable (running stays out);
//   RN-8   — the whole-window idle annotation accepts an exact typed wait
//             token when the row exposed no StateKind;
//   RN-4   — the systrace comm-truncation placeholder "<...>-N" renders as
//             线程名未记录(tid N), display-only.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// rnCustomerRunnableProjection mirrors the customer 7.0 flat shape: the
// rank=1 supply_pressure aggregate primary (direct-published into the
// background bucket with its raw primary role — chain_relevance=background
// bypasses the 裁定1 demotion normalization) plus the flat on-chain runnable
// row. Window 1514.24ms → 635.981ms = 42%.
func rnCustomerRunnableProjection() types.TraceCausalProjection {
	supply := types.TraceCausalProjectionNode{
		EvidenceID: "E-supply", Subject: "unknown-thread", Object: "supply_pressure",
		Predicate: "root_cause_primary", Role: types.TraceCausalRolePrimaryRootCause,
		Rank: 1, SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		ChainRelevance: "background", ImpactMS: 12000.0, CumulativeImpactMS: 12000.0,
	}
	runnable := types.TraceCausalProjectionNode{
		EvidenceID: "E-run", Subject: "OS_FFRT_2_3-49706", Object: "runnable",
		Role: types.TraceCausalRoleCausalHop, StateKind: "runnable",
		ChainRelevance: "on_chain", ImpactMS: 635.981, CumulativeImpactMS: 635.981,
	}
	return types.TraceCausalProjection{
		WindowStartTs:     100.0,
		WindowEndTs:       101.51424,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{supply},
		OnChainCauses:     []types.TraceCausalProjectionNode{runnable},
		BackgroundCauses:  []types.TraceCausalProjectionNode{supply},
	}
}

// --- RN-3(a): conclusion falls back to the largest on-chain row -------------------

func TestRNLeadFallsBackToFlatOnChainRow(t *testing.T) {
	projection := rnCustomerRunnableProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	for _, want := range []string{
		"**主根因:** OS_FFRT_2_3-49706", "635.981ms", "(占窗42%)",
		"(链不可上溯,按窗口内最大 on-chain 等待)",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("customer 7.0 pin: the flat runnable row must lead with the short note (%q):\n%s", want, line)
		}
	}
	if strings.Contains(line, "未定位到链上主根因") {
		t.Fatalf("the 未定位 branch must not fire while an on-chain data row exists:\n%s", line)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, nil, false)
	en := runtimeTraceProjConclusionLine(projection, enModel, false)
	if !strings.Contains(en, "OS_FFRT_2_3-49706") ||
		!strings.Contains(en, "(chain not traceable upstream; largest on-chain wait in the window)") {
		t.Fatalf("EN conclusion must fork the same way:\n%s", en)
	}
	// The comparison-overview primary cell consumes the SAME selection + note.
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "OS_FFRT_2_3-49706") || !strings.Contains(cell, "635.981ms") ||
		!strings.Contains(cell, "(链不可上溯,按窗口内最大 on-chain 等待)") {
		t.Fatalf("compare primary cell must consume the fallback lead with its note: %q", cell)
	}
	if strings.Contains(cell, "未定位到链上主根因") {
		t.Fatalf("compare primary cell must not claim 未定位 while the fallback lead exists: %q", cell)
	}
}

func TestRNLeadFallbackStillPointsAtBackgroundWithoutOnChainRows(t *testing.T) {
	// Negative pin: primaries all demoted AND no data-bearing on-chain row →
	// the existing 未定位/背景压力段 wording stays (the fallback never invents
	// a lead from adjacent/background rows).
	projection := rnCustomerRunnableProjection()
	projection.OnChainCauses = nil
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "窗口内未定位到链上主根因,见背景压力段") {
		t.Fatalf("no on-chain data row → the 未定位 branch stays:\n%s", line)
	}
}

// --- RN-3(b): 主根因 label follows the consumed node only --------------------------

func TestRNUnconsumedPrimaryTierBackgroundRowDemotesPositionLabel(t *testing.T) {
	projection := rnCustomerRunnableProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.LeadKey != "E-run" {
		t.Fatalf("model must pin the consumed lead's node key: %q", model.LeadKey)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	var background, flat string
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "背景") {
			background = row.Cells[1]
		} else {
			flat = row.Cells[1]
		}
	}
	// Customer pin: the direct-published rank=1 supply_pressure background row
	// must not read 主根因 while the conclusion names another node — it demotes
	// to 背景 · 支撑参考 with the rank kept for audit.
	if background != "背景 · 支撑参考(rank=1)" {
		t.Fatalf("unconsumed primary-tier background row must demote with its rank note: %q", background)
	}
	// The consumed flat row keeps the CMP-7a flat position (the conclusion
	// note, not the position column, carries the fallback semantics).
	if flat != "平铺(链不可上溯) · 重点关注" {
		t.Fatalf("consumed flat row keeps the CMP-7a position cell: %q", flat)
	}
	for _, row := range rows {
		if strings.Contains(row.Cells[1], "主根因") || strings.Contains(row.Cells[1], "主要关注") {
			t.Fatalf("no row of this shape may carry the primary label: %+v", row.Cells)
		}
	}
	// EN mirror of the demoted cell.
	enModel := buildRuntimeTraceProjTreeModel(projection, nil, false)
	_, enRows := runtimeTraceProjDetailTable(enModel, false)
	found := false
	for _, row := range enRows {
		if strings.Contains(row.Cells[1], "background · supporting context (rank=1)") {
			found = true
		}
		if strings.Contains(row.Cells[1], "primary focus") {
			t.Fatalf("EN surface must not carry the primary label either: %+v", row.Cells)
		}
	}
	if !found {
		t.Fatalf("EN demoted cell missing: %+v", enRows)
	}
}

func TestRNConsumedRankLeadKeepsPrimaryLabel(t *testing.T) {
	// Golden-stability pin: the normal single-artifact rank-lead scenario is
	// label-stable — the consumed rank=1 primary (typed root_cause_primary
	// role, thread subject) keeps 主根因 · 主要关注; the berlin v3 golden pins
	// the full end-to-end rows, this guards the direct lane.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"RSUniRenderThre-1963", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.101,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			EvidenceID: "E4", Subject: "RSUniRenderThre-1963", Object: "running",
			Predicate: "root_cause_primary", Role: types.TraceCausalRolePrimaryRootCause,
			Rank: 1, ImpactMS: 4.115, CumulativeImpactMS: 4.115, EffectiveImpactMS: 4.115,
			StateKind: "running", ChainRelevance: "on_chain", ChainDepth: 1,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.LeadKey != "E4" {
		t.Fatalf("rank=1 running row must be the consumed lead: %q", model.LeadKey)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	found := false
	for _, row := range rows {
		if strings.Contains(row.Cells[2], "RSUniRenderThre-1963") &&
			row.Cells[1] == "主根因 · 主要关注" {
			found = true
		}
	}
	if !found {
		t.Fatalf("consumed rank lead must keep the primary position label: %+v", rows)
	}
}

// --- RN-2b: no anchor window → no ⚠ ----------------------------------------------

func TestRNCrossWindowMarkerSuppressedWithoutAnchorWindow(t *testing.T) {
	mkProjection := func(withWindow bool) types.TraceCausalProjection {
		node := types.TraceCausalProjectionNode{
			EvidenceID: "E-x", Subject: "worker-3", Object: "sleep_wait",
			StateKind: "s_sleep", ChainRelevance: "on_chain",
			ImpactMS: 30.0, CumulativeImpactMS: 30.0,
			ActualImpactMS: 80.0, // > baseline*1.001 → numeric cross shape
		}
		p := types.TraceCausalProjection{OnChainCauses: []types.TraceCausalProjectionNode{node}}
		if withWindow {
			p.WindowStartTs, p.WindowEndTs = 100.0, 100.2
		}
		return p
	}
	// 6.0 shape: no anchor → the marker (and its legend entry, via the typed
	// mark) must not render on any surface.
	model := buildRuntimeTraceProjTreeModel(mkProjection(false), nil, true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "⚠") {
		t.Fatalf("no anchor window → no ⚠ in the fence:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkCrossWindow) {
		t.Fatalf("no anchor window → the cross-window mark must not be emitted (legend follows it)")
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	for _, row := range rows {
		for _, cell := range row.Cells {
			if strings.Contains(cell, "⚠") {
				t.Fatalf("no anchor window → no ⚠ in the detail table: %+v", row.Cells)
			}
		}
	}
	// Anchored control: the exact same node keeps its ⚠ on both surfaces.
	anchored := buildRuntimeTraceProjTreeModel(mkProjection(true), nil, true)
	if fence := runtimeTraceProjTreeFence(anchored, true); !strings.Contains(fence, "⚠跨窗") {
		t.Fatalf("anchored render keeps the ⚠跨窗 tag:\n%s", fence)
	}
	_, anchoredRows := runtimeTraceProjDetailTable(anchored, true)
	found := false
	for _, row := range anchoredRows {
		for _, cell := range row.Cells {
			if strings.Contains(cell, "80.000ms ⚠") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("anchored detail table keeps the actual-state ⚠: %+v", anchoredRows)
	}
}

// --- RN-6: the symptom family includes runnable ------------------------------------

func TestRNRunnableDominantTargetSymptomAndComparisonCell(t *testing.T) {
	// Customer pin: a runnable-dominant target (sleep=0) publishes its runnable
	// total as the symptom — the comparison table's 目标症状 column (same
	// function) no longer renders "—" for that artifact.
	model := runtimeTraceProjTreeModel{
		Target:   "OS_FFRT_2_3-49706",
		WindowMS: 1514.24,
		SelfRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "OS_FFRT_2_3-49706", ImpactMS: 635.981, StateKind: "runnable"}},
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "OS_FFRT_2_3-49706", ImpactMS: 90.0, StateKind: "running"}},
		},
	}
	if got := runtimeTraceProjTargetSymptomMS(model); got != 635.981 {
		t.Fatalf("runnable-dominant symptom must be the runnable total (running excluded): got %.3f", got)
	}
	// The coverage line renders the RN-6 wording over the runnable denominator.
	model.TreeRows = []runtimeTraceProjTreeRow{
		{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "waker-1", CumulativeImpactMS: 100.0}},
	}
	projection := types.TraceCausalProjection{WindowStartTs: 100.0, WindowEndTs: 101.51424}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "目标等待(睡眠/阻塞/就绪) 635.981ms 中") {
		t.Fatalf("runnable-dominant coverage must use the wait wording + runnable denominator:\n%s", line)
	}
}

// --- RN-8: whole-window idle annotation via exact typed wait token ------------------

func TestRNWholeWindowIdleAcceptsStatelessWaitTypeToken(t *testing.T) {
	mkRow := func(state, typeToken, predicate string) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			Node: types.TraceCausalProjectionNode{
				Subject: "jbd2/loop3-8-390", Object: "unknown-thread",
				ImpactMS: 101.0, ChainRelevance: "background",
				StateKind: state, TypeToken: typeToken, Predicate: predicate,
			},
		}
	}
	// Customer pin: the 8×101ms d_state_or_io_wait background rows carried the
	// typed token but no dominant state — they must take the idle annotation
	// instead of reading as an active IO burst.
	if line := runtimeTraceProjStanzaRowLine(mkRow("", "d_state_or_io_wait", ""), runtimeTraceProjTreeLabelWidth, 101.0, true, true); !strings.Contains(line, "整窗等待") {
		t.Fatalf("stateless d_state_or_io_wait whole-window row must be annotated idle:\n%s", line)
	}
	if line := runtimeTraceProjStanzaRowLine(mkRow("", "", "blocking_span"), runtimeTraceProjTreeLabelWidth, 101.0, true, true); !strings.Contains(line, "整窗等待") {
		t.Fatalf("stateless blocking_span whole-window row must be annotated idle:\n%s", line)
	}
	// A present StateKind always wins: running/runnable rows never take the tag
	// even with a wait token (F3 pins kept).
	for _, state := range []string{"running", "runnable"} {
		if line := runtimeTraceProjStanzaRowLine(mkRow(state, "d_state_or_io_wait", ""), runtimeTraceProjTreeLabelWidth, 101.0, true, true); strings.Contains(line, "疑似空闲") {
			t.Fatalf("a %s row must not be annotated idle regardless of its token:\n%s", state, line)
		}
	}
	// Non-wait tokens stay out (no Object prose lane either).
	if line := runtimeTraceProjStanzaRowLine(mkRow("", "io_burst_episode", ""), runtimeTraceProjTreeLabelWidth, 101.0, true, true); strings.Contains(line, "疑似空闲") {
		t.Fatalf("a non-wait token must not fabricate the idle claim:\n%s", line)
	}
	// The detail table consumes the SAME shared judgment.
	model := runtimeTraceProjTreeModel{
		WindowMS:   101.0,
		Background: []runtimeTraceProjTreeRow{mkRow("", "d_state_or_io_wait", "")},
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 1 || !strings.Contains(rows[0].Cells[5], "整窗等待(疑似空闲)") {
		t.Fatalf("detail table must mirror the token-lane idle annotation: %+v", rows)
	}
}

// --- RN-4: comm-truncation placeholder --------------------------------------------

func TestRNCommTruncatedSubjectDisplaysUnrecordedThreadName(t *testing.T) {
	// Customer pin: "<...>-21577" is the systrace comm-truncation marker (a
	// system token, not a name) — display it as 线程名未记录(tid N).
	if got := runtimeTraceCausalProjectionDisplayNodeName("<...>-21577", true); got != "线程名未记录(tid 21577)" {
		t.Fatalf("zh display: %q", got)
	}
	if got := runtimeTraceCausalProjectionDisplayNodeName("<...>-21577", false); got != "unnamed thread (tid 21577)" {
		t.Fatalf("en display: %q", got)
	}
	node := types.TraceCausalProjectionNode{Subject: "<...>-21577", Object: "runnable"}
	if got := runtimeTraceCausalProjectionDisplaySubjectName(node, true); got != "线程名未记录(tid 21577)" {
		t.Fatalf("subject lane must route through the same helper: %q", got)
	}
	// Exact-shape gate: real names — even odd ones — render verbatim.
	for _, verbatim := range []string{
		"OS_FFRT_2_3-49706", "worker<...>-21577", "<...>-21a77", "<...>-", "<...>", "<..>-123",
	} {
		if got := runtimeTraceCausalProjectionDisplayNodeName(verbatim, true); got != verbatim {
			t.Fatalf("non-placeholder subject must stay verbatim: %q -> %q", verbatim, got)
		}
	}
	// Display-only: canonical/grouping keys keep the raw subject.
	if got := runtimeTraceCausalProjectionCanonicalNode("<...>-21577"); got != "<...>-21577" {
		t.Fatalf("canonical key must stay raw: %q", got)
	}
}
