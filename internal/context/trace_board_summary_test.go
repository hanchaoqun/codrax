package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_board_summary_test.go — CR-1 件③ pins (§29.42.2① 单源摘要喂入,
// 2026-07-12): the typed board summary carries the seated rows in authority
// order with value + caliber + channel + confidence, excludes seatless rows,
// and stays silent on non-trace runs.

func traceBoardTestLedger() types.ObservationLedger {
	rankRecord := func(id, subject, typ string, notes []string, conf float64) types.ObservationRecord {
		return types.ObservationRecord{
			ID:         id,
			Origin:     types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:   "trace_query",
			Subject:    subject,
			Predicate:  "root_cause_primary",
			Object:     typ,
			RichNotes:  notes,
			Confidence: conf,
		}
	}
	return types.ObservationLedger{Records: []types.ObservationRecord{
		rankRecord("trace_query:t#root_cause_rank:1", "CompThread_0-2955", "d_state_or_io_wait",
			[]string{"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757", "member_fold_caliber=sum_disjoint"}, 0.80),
		rankRecord("trace_query:t#root_cause_rank:2", "keva-1-17437", "sleep_wait",
			[]string{"rank=2", "tier=secondary", "chain_relevance=on_chain", "effective_impact_ms=3.399"}, 0.74),
		// Seatless rows never enter the board summary: the target's own
		// symptom row (rank=0) and a context-only pacing row.
		rankRecord("trace_query:t#root_cause_rank:3", ".ugc.aweme.lite-17267", "binder_wait",
			[]string{"tier=target_self_state", "effective_impact_ms=1.409"}, 0.92),
		rankRecord("trace_query:t#root_cause_rank:4", ".ugc.aweme.lite-17267", "pacing_idle",
			[]string{"tier=context_only", "effective_impact_ms=0.000"}, 0.85),
		// An adjacent-channel seat rides its own ordinal space.
		rankRecord("trace_query:t#root_cause_rank:5", "adj-5", "running",
			[]string{"rank=1", "tier=secondary", "chain_relevance=adjacent", "effective_impact_ms=4.596"}, 0.70),
	}}
}

func TestTraceRootCauseBoardSummaryAuthoritativeOrder(t *testing.T) {
	summary := formatTraceRootCauseBoardFromLedger(traceBoardTestLedger())
	if summary == "" {
		t.Fatalf("seated rank rows must render a board summary")
	}
	first := strings.Index(summary, "CompThread_0-2955")
	second := strings.Index(summary, "keva-1-17437")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("board summary must keep the engine seat order:\n%s", summary)
	}
	for _, want := range []string{
		"#1 root-cause seat — CompThread_0-2955 · d_state_or_io_wait",
		"36.757ms (effective attribution)",
		"fold=sum_disjoint",
		"channel=chain",
		"confidence=0.80",
		"Adjacent-impact seats",
		"#1 adjacent seat — adj-5 · running",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("board summary missing %q:\n%s", want, summary)
		}
	}
	// Seatless rows (symptom / context lanes) never take board lines.
	if strings.Contains(summary, "binder_wait") || strings.Contains(summary, "pacing_idle") {
		t.Fatalf("seatless rows must stay off the board summary:\n%s", summary)
	}
	// The teaching preamble states the disclosure duty, never a gate.
	if !strings.Contains(summary, "never reorder silently") || !strings.Contains(summary, "never sum rows together") {
		t.Fatalf("board summary preamble must carry the ordering/no-sum teaching:\n%s", summary)
	}
}

// CR-2 组③ P7 / F-4 (冷读 F-4, 2026-07-12; witness tieba 20260712-135155
// prose: 「runnable_wait 窗口 — 34579.568118s–34579.572194s(25.847ms)」 — the
// whole-window total paired with ONE occurrence window). The board summary
// labels a row's representative window as one occurrence and teaches the
// value-window pairing rule (soft data-feeding lane, never a gate).
func TestTraceRootCauseBoardSummaryLabelsRepresentativeWindow(t *testing.T) {
	ledger := traceBoardTestLedger()
	ledger.Records[0].RichNotes = append(ledger.Records[0].RichNotes,
		"occurrence_windows=34579.568118..34579.572194;34579.579001..34579.581200")
	summary := formatTraceRootCauseBoardFromLedger(ledger)
	if !strings.Contains(summary, "representative_window=34579.568118..34579.572194") {
		t.Fatalf("a row with occurrence windows must label its representative window:\n%s", summary)
	}
	if !strings.Contains(summary, "ONE occurrence among several") {
		t.Fatalf("the preamble must teach the value-window pairing rule:\n%s", summary)
	}
	// Rows without occurrence windows stay byte-identical (no fabricated window).
	if strings.Contains(summary, "keva-1-17437 · sleep_wait · channel=chain · confidence=0.74 · representative_window") {
		t.Fatalf("windowless rows must not gain a representative window:\n%s", summary)
	}
}

func TestTraceRootCauseBoardSummarySilentWithoutTraceObservations(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{
		ID:       "current",
		Origin:   types.AnswerEvidenceOriginCurrentSource,
		Producer: "read_file",
		Subject:  "someSymbol",
	}}}
	if got := formatTraceRootCauseBoardFromLedger(ledger); got != "" {
		t.Fatalf("non-trace runs must not render a board summary: %q", got)
	}
}
