package types

import (
	"testing"
)

// analyzer_decision_test.go — R8 lock tests for the
// AnalyzerDecisionSignal channel.

// TestMutableState_AppendAnalyzerDecision_RoundTrip pins the
// basic Append + read flow, including dedup-by-composite-key.
func TestMutableState_AppendAnalyzerDecision_RoundTrip(t *testing.T) {
	mut := &MutableState{}
	if got := mut.AnalyzerDecisions(); got != nil {
		t.Errorf("fresh MutableState: got %d signals, want 0", len(got))
	}

	sig1 := AnalyzerDecisionSignal{
		Kind:   "scenario_reconciled",
		Stage:  string(StageAnalyze),
		Before: "architecture_explain",
		After:  "generic",
		Reason: "single-topic structural trace",
	}
	mut.AppendAnalyzerDecision(sig1)
	if got := mut.AnalyzerDecisions(); len(got) != 1 || got[0] != sig1 {
		t.Errorf("after Append: got %+v, want one entry equal to sig1", got)
	}

	// Identical signal — must dedup.
	mut.AppendAnalyzerDecision(sig1)
	if got := mut.AnalyzerDecisions(); len(got) != 1 {
		t.Errorf("dedup broken: len=%d after duplicate Append", len(got))
	}

	// Distinct signal — must record separately.
	sig2 := AnalyzerDecisionSignal{
		Kind:   "completeness_downgraded",
		Stage:  string(StageExtract),
		Before: "complete",
		After:  "lower_bound",
		Reason: "3 items < baseline 9",
	}
	mut.AppendAnalyzerDecision(sig2)
	if got := mut.AnalyzerDecisions(); len(got) != 2 {
		t.Errorf("distinct signal: len=%d, want 2", len(got))
	}
}

// TestMutableState_AppendAnalyzerDecision_NilGuards covers nil
// receiver + empty Kind early-returns.
func TestMutableState_AppendAnalyzerDecision_NilGuards(t *testing.T) {
	if (*MutableState)(nil).AnalyzerDecisions() != nil {
		t.Error("nil receiver must yield nil")
	}
	mut := &MutableState{}
	mut.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: ""}) // empty Kind dropped
	if got := mut.AnalyzerDecisions(); len(got) > 0 {
		t.Errorf("empty Kind must be silently dropped; got %d entries", len(got))
	}
}
