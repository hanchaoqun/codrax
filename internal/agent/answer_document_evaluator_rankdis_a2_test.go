package agent

// answer_document_evaluator_rankdis_a2_test.go — RANKDIS-EXT A2
// (§29.104.16.1 M2, 2026-07-16; witness cust_span_vs_prio.txt): the
// per-branch wakeup_chain ClaimKey wears branch= (same word as the typed
// branch= rich note) instead of the retired path#N form whose #N ordinal
// glyph a customer model read as a second Rank#1. Read-side consumers keep a
// fail-open arm for pre-rename artifacts.
//
// MUTATION self-check: dropping the branch= arm in
// traceQueryObservationSupplementOrder reds the new-key order pin; dropping
// the path prefix arm reds the two legacy-key pins.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func rankdisA2SupplementRecord(claimKey string) types.ObservationRecord {
	return types.ObservationRecord{
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey:        claimKey,
		Subject:         "app-100",
		Predicate:       "wakeup_chain",
		Object:          "worker-200 -> app-100",
	}
}

func TestRankdisA2SupplementOrderServesNewAndLegacyBranchKeys(t *testing.T) {
	for _, tc := range []struct {
		label    string
		claimKey string
	}{
		{"new branch= key", "wakeup_chain:branch=3"},
		{"legacy per-branch path#N key (pre-rename artifact)", "wakeup_chain:path#3"},
		{"identity-less bare key", "wakeup_chain:path"},
	} {
		if got := traceQueryObservationSupplementOrder(rankdisA2SupplementRecord(tc.claimKey)); got != 20 {
			t.Fatalf("%s must keep supplement order 20, got %d", tc.label, got)
		}
	}
}

func TestRankdisA2SupplementTextRendersBranchKeyVerbatim(t *testing.T) {
	record := rankdisA2SupplementRecord("wakeup_chain:branch=1")
	for _, tc := range []struct {
		zh   bool
		want string
	}{
		{zh: true, want: "wakeup_chain:branch=1：worker-200 -> app-100"},
		{zh: false, want: "wakeup_chain:branch=1: worker-200 -> app-100"},
	} {
		got := traceQueryObservationSupplementText(record, tc.zh)
		if !strings.HasPrefix(got, tc.want) {
			t.Fatalf("branch-key wakeup path render, zh=%t got=%q want prefix %q", tc.zh, got, tc.want)
		}
		if strings.Contains(got, "path#") {
			t.Fatalf("the supplement face must not resurrect the retired path#N glyph, zh=%t got=%q", tc.zh, got)
		}
	}
}
