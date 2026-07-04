package types

// trace_causal_projection_fullwindow_rn12_test.go — RN-12 (§7.9 runnable
// 主导场景审计 2026-07-04, docs/design/customer_dead_session_audit_20260703.md,
// cust_runnable.txt) compile-side pins. Customer shape: the flat tree showed
// one runnable chain row 635.981ms (the wakeup-chain view's top fragment)
// while a state_drilldown observation of the SAME thread in the SAME ledger
// held the full-window runnable total 2528.721ms — with no cross-reference,
// the reader concluded "the tree is truncated". The compile pass attaches the
// typed FullWindowStateMS/FullWindowStateSource pair onto chain-universe
// nodes when: same canonical subject, same state class (runnable / sleep —
// per-class table, no runnable special case), and total STRICTLY greater than
// the node's window projection ×1.2 (exact comparison — a near-equal total
// must not grow annotation noise).
//
// F-2 (统一复核 2026-07-04) three-state window ruling: a carrier total only
// collects when it names its own typed selected_window (禁猜 — no note, no
// attach); the attach records the carrier window and the precise ±1ms
// same-window verdict against the projection anchor, so the display layer
// says "窗内" only when it is arithmetically the same window and labels any
// other query window explicitly.

import (
	"fmt"
	"testing"
)

// rn12Window is the shared same-window note of these pins (the customer's
// 3000ms query window).
const rn12Window = "selected_window=100.000000..103.000000"

func rn12Obs(id, predicate, claimKey, subject, object, value string, impact float64, lineStart, lineEnd int, notes ...string) ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: predicate, ClaimKey: claimKey,
		Subject: subject, Object: object, Value: value, Unit: "ms", Confidence: 0.85,
		SupportRefs: []string{fmt.Sprintf("cust_runnable.txt:%d-%d", lineStart, lineEnd)},
		Span:        ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:   append(base, notes...),
	}
}

func rn12RunnablePrimary() ObservationRecord {
	// The root_cause_ ClaimKey family is a CMP-2 anchor family: its
	// selected_window note anchors the projection window (100.0..103.0).
	return rn12Obs("root-ffrt", "root_cause_primary", "root_cause_primary:ffrt",
		"OS_FFRT_2_3-49706", "runnable_wait", "635.981", 635.981, 1200, 9800,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=runnable", rn12Window)
}

func rn12StateDrilldownWindowed(subject, state, value, windowNote string) ObservationRecord {
	notes := []string{"rank=1", "state=" + state, "impact=" + value, "total=" + value,
		"source=top_runnable", "chain_required=false", "recursive=true", "significant=true"}
	if windowNote != "" {
		notes = append(notes, windowNote)
	}
	return ObservationRecord{
		ID: "trace_query:w1#state_drilldown:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "state_drilldown", ClaimKey: "state_drilldown:" + subject + ":" + state,
		Subject: subject, Object: state, Value: value, Unit: "ms",
		Span:      ObservationSpan{LineStart: 1200, LineEnd: 9800},
		RichNotes: notes,
	}
}

func rn12StateDrilldown(subject, state, value string) ObservationRecord {
	return rn12StateDrilldownWindowed(subject, state, value, rn12Window)
}

// Customer pin: 635.981ms chain fragment + 2528.721ms full-window
// state_drilldown total → the typed cross-reference attaches.
func TestRN12FullWindowRunnableTotalAttaches(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldown("OS_FFRT_2_3-49706", "runnable", "2528.721"),
	})
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil {
		t.Fatalf("runnable primary must compile: %+v", projection.PrimaryRootCauses)
	}
	if node.FullWindowStateMS != 2528.721 || node.FullWindowStateSource != "state_drilldown" {
		t.Fatalf("customer pin: full-window total must attach (ms=%v source=%q)",
			node.FullWindowStateMS, node.FullWindowStateSource)
	}
	// F-2 state 1 (同窗): carrier selected_window == anchor window → the
	// typed same-window verdict is true and the endpoints travel with it.
	if !node.FullWindowStateSameWindow ||
		node.FullWindowStateWindowStart != 100.0 || node.FullWindowStateWindowEnd != 103.0 {
		t.Fatalf("same-window carrier must attach with SameWindow=true + endpoints: %+v", node)
	}
	if projection.PrimaryRootCause == nil || !projection.PrimaryRootCause.FullWindowStateSameWindow ||
		projection.PrimaryRootCause.FullWindowStateMS != 2528.721 {
		t.Fatalf("the PrimaryRootCause pointer copy must carry the attach too: %+v", projection.PrimaryRootCause)
	}
	// The carrier is a side-channel, never a node of its own.
	for _, nodes := range [][]TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses, projection.AdjacentCauses,
		projection.BackgroundCauses, projection.SupportingHops,
	} {
		for _, n := range nodes {
			if n.Predicate == "state_drilldown" {
				t.Fatalf("state_drilldown must never compile into a node: %+v", n)
			}
		}
	}
}

// Pin: no full-window observation → no annotation (fields stay zero).
func TestRN12NoTotalObservationAttachesNothing(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{rn12RunnablePrimary()})
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 0 || node.FullWindowStateSource != "" {
		t.Fatalf("no observation → no attach: %+v", node)
	}
}

// Pin: totals within ×1.2 of the row (including the exact-equal republication
// shape) never attach — a near-equal total is not a coverage gap.
func TestRN12BelowThresholdAttachesNothing(t *testing.T) {
	for _, value := range []string{"700.000", "635.981", "763.177"} { // 635.981×1.2 = 763.1772
		projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
			rn12RunnablePrimary(),
			rn12StateDrilldown("OS_FFRT_2_3-49706", "runnable", value),
		})
		node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
		if node == nil || node.FullWindowStateMS != 0 {
			t.Fatalf("total %s ≤ ×1.2 must not attach: %+v", value, node)
		}
	}
}

// Pin: subject and state-class matching are exact — a different thread's
// total or a different class's total never attaches to the runnable row.
func TestRN12SubjectAndClassMatchingIsExact(t *testing.T) {
	otherSubject := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldown("some-other-thread-1", "runnable", "2528.721"),
	})
	node := traceProjectionFindNodeBySubject(otherSubject.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 0 {
		t.Fatalf("subject-mismatched total must not attach: %+v", node)
	}
	otherClass := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldown("OS_FFRT_2_3-49706", "s_sleep", "2528.721"),
	})
	node = traceProjectionFindNodeBySubject(otherClass.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 0 {
		t.Fatalf("class-mismatched total must not attach: %+v", node)
	}
}

// Pin (sleep isomorph, same mechanism per-class): a sleep chain row + a
// top_sleep family total (Predicate sleep_wait, Object sleep) attaches with
// the top_sleep source token.
func TestRN12SleepClassIsomorphAttachesFromTopSleepFamily(t *testing.T) {
	sleepRow := rn12Obs("root-rs", "root_cause_primary", "root_cause_primary:rs",
		"render_service-411", "sleep_wait", "120.000", 120.0, 300, 400,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep", rn12Window)
	topSleep := ObservationRecord{
		ID: "trace_query:w1#top_sleep:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "sleep_wait", ClaimKey: "sleep_wait:render_service-411",
		Subject: "render_service-411", Object: "sleep", Value: "800.000", Unit: "ms",
		Span:      ObservationSpan{LineStart: 300, LineEnd: 400},
		RichNotes: []string{"state=sleep", "duration=800.000", rn12Window},
	}
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{sleepRow, topSleep})
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "render_service-411")
	if node == nil || node.FullWindowStateMS != 800.0 || node.FullWindowStateSource != "top_sleep" {
		t.Fatalf("sleep isomorph must attach from the top_sleep family: %+v", node)
	}
	// The top-family carrier never becomes a node either.
	for _, nodes := range [][]TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses, projection.SupportingHops,
	} {
		for _, n := range nodes {
			if n.Predicate == "sleep_wait" && n.EvidenceID == "trace_query:w1#top_sleep:1" {
				t.Fatalf("top-family carrier must never compile into a node: %+v", n)
			}
		}
	}
}

// Pin: the ×1.2 comparison runs against the ×N merged SUM (含 ×N 合并) —
// three 300ms-class rows merge to a 901.500ms sum; a 1000ms total stays
// inside ×1.2 of the SUM (no attach), a 1200ms total attaches.
func TestRN12ThresholdComparesAgainstMergedSum(t *testing.T) {
	hop := func(id, value string, impact float64, ls, le int) ObservationRecord {
		// wakeup_causal_impact rows carry the selected_window note in
		// production (NEW-8) and are a CMP-2 anchor family — same window as
		// the carrier, so this stays the 同窗 lane.
		return rn12Obs(id, "wakeup_causal_impact", "wakeup_causal_impact:"+id,
			"worker-7", "runnable", value, impact, ls, le,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=runnable", rn12Window)
	}
	hops := []ObservationRecord{
		hop("hop-1", "300.000", 300.0, 100, 110),
		hop("hop-2", "300.500", 300.5, 200, 210),
		hop("hop-3", "301.000", 301.0, 300, 310),
	}
	within := TraceCausalProjectionFromObservationRecords(append(
		append([]ObservationRecord(nil), hops...),
		rn12StateDrilldown("worker-7", "runnable", "1000.000"))) // 901.5×1.2 = 1081.8
	node := traceProjectionFindNodeBySubject(within.OnChainCauses, "worker-7")
	if node == nil || node.MergedCount != 3 || node.ImpactMS != 901.5 {
		t.Fatalf("test precondition: the three hops must R2-merge to the 901.500 sum: %+v", node)
	}
	if node.FullWindowStateMS != 0 {
		t.Fatalf("1000.000 ≤ merged-sum ×1.2 must not attach: %+v", node)
	}
	beyond := TraceCausalProjectionFromObservationRecords(append(
		append([]ObservationRecord(nil), hops...),
		rn12StateDrilldown("worker-7", "runnable", "1200.000")))
	node = traceProjectionFindNodeBySubject(beyond.OnChainCauses, "worker-7")
	if node == nil || node.FullWindowStateMS != 1200.0 || node.FullWindowStateSource != "state_drilldown" {
		t.Fatalf("1200.000 > merged-sum ×1.2 must attach: %+v", node)
	}
}

// F-2 state 2 (异窗标注): a total measured in a DIFFERENT query window than
// the projection anchor still attaches, but with SameWindow=false and its own
// endpoints — the recovery dual-window shape that used to render a 2528.721ms
// "窗内" total inside a 300ms 关注窗 (total = 8.4× the window length, an
// arithmetically impossible claim).
func TestRN12CrossWindowTotalAttachesLabeled(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(), // anchor 100.0..103.0
		rn12StateDrilldownWindowed("OS_FFRT_2_3-49706", "runnable", "2528.721",
			"selected_window=831.000000..834.000000"),
	})
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 2528.721 {
		t.Fatalf("cross-window total must still attach: %+v", node)
	}
	if node.FullWindowStateSameWindow {
		t.Fatalf("a different query window must never claim 同窗: %+v", node)
	}
	if node.FullWindowStateWindowStart != 831.0 || node.FullWindowStateWindowEnd != 834.0 {
		t.Fatalf("the carrier's own window endpoints must travel for honest labeling: %+v", node)
	}
}

// F-2 state 3 (无 note 不注 — 禁猜): a carrier without a typed
// selected_window note never collects, regardless of subject/class/threshold.
func TestRN12NoSelectedWindowNoteNeverAttaches(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldownWindowed("OS_FFRT_2_3-49706", "runnable", "2528.721", ""),
	})
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 0 || node.FullWindowStateSource != "" {
		t.Fatalf("no selected_window note → no attach (禁猜): %+v", node)
	}
}

// F-2 tolerance pin: the same-window verdict is a precise float comparison
// with ±1ms per endpoint — endpoint drift of 0.5ms/0.9ms stays 同窗, 2ms is a
// different window.
func TestRN12SameWindowToleranceIsOneMillisecondPerEndpoint(t *testing.T) {
	within := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldownWindowed("OS_FFRT_2_3-49706", "runnable", "2528.721",
			"selected_window=100.000500..103.000900"),
	})
	node := traceProjectionFindNodeBySubject(within.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 2528.721 || !node.FullWindowStateSameWindow {
		t.Fatalf("±1ms endpoint drift must stay 同窗: %+v", node)
	}
	beyond := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rn12RunnablePrimary(),
		rn12StateDrilldownWindowed("OS_FFRT_2_3-49706", "runnable", "2528.721",
			"selected_window=100.002000..103.000000"),
	})
	node = traceProjectionFindNodeBySubject(beyond.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 2528.721 || node.FullWindowStateSameWindow {
		t.Fatalf("2ms endpoint drift is a different window: %+v", node)
	}
}

// F-2: a projection WITHOUT an anchor window has nothing to verify "窗内"
// against — the total attaches with SameWindow=false (labeled wording), never
// with an unverifiable 同窗 claim.
func TestRN12NoAnchorWindowMeansLabeledAttach(t *testing.T) {
	primary := rn12Obs("root-ffrt", "root_cause_primary", "root_cause_primary:ffrt",
		"OS_FFRT_2_3-49706", "runnable_wait", "635.981", 635.981, 1200, 9800,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=runnable") // no selected_window → no anchor
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		primary,
		rn12StateDrilldown("OS_FFRT_2_3-49706", "runnable", "2528.721"),
	})
	if projection.WindowStartTs != 0 || projection.WindowEndTs != 0 {
		t.Fatalf("test precondition: no anchor window expected: %+v", projection)
	}
	node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, "OS_FFRT_2_3-49706")
	if node == nil || node.FullWindowStateMS != 2528.721 || node.FullWindowStateSameWindow {
		t.Fatalf("anchorless projection must attach labeled (SameWindow=false): %+v", node)
	}
	if node.FullWindowStateWindowStart != 100.0 || node.FullWindowStateWindowEnd != 103.0 {
		t.Fatalf("anchorless attach must still carry the carrier endpoints: %+v", node)
	}
}

// Pin: the shared class table is the single source — exact tokens only.
func TestRN12StateClassTable(t *testing.T) {
	for raw, want := range map[string]string{
		"runnable": "runnable", "Runnable": "runnable",
		"s_sleep": "sleep", "sleep": "sleep", "sleep_wait": "sleep",
		"running": "", "d_state": "", "io_wait": "", "d_sleep": "", "": "",
		"runnable_wait": "", // object word of cause rows, not a state token
	} {
		if got := TraceCausalProjectionStateClass(raw); got != want {
			t.Fatalf("TraceCausalProjectionStateClass(%q) = %q, want %q", raw, got, want)
		}
	}
}
