package dataquery

import (
	"strings"
	"testing"
)

// A structurally non-aggregating plan (pure extraction/projection, no
// compute_contributions action) whose contributions do not participate
// in numeric reconciliation must reconcile at the ANSWER level (pass)
// instead of failing on the inapplicable numeric path — the capability
// the skill documents via scope="answer".
func TestRunReconcileArtifacts_NonAggregatingAnswerLevelPass(t *testing.T) {
	r := ActionRunner{}
	// non-empty material-role contributions never participate in numeric
	// reconcile (the extraction-pipeline shape: records exist, none are
	// numeric aggregations).
	contribs := []ContributionRecord{
		{Role: LooseText("material"), GroupKey: LooseText("users.json")},
		{Role: LooseText("material"), GroupKey: LooseText("u1")},
	}
	_, report, err := r.runReconcileArtifacts(DataAction{ID: "rec"}, contribs, false /* not aggregating */)
	if err != nil {
		t.Fatalf("pure-extraction reconcile must pass at answer level, got: %v", err)
	}
	if strings.ToLower(report.Status.String()) != "pass" {
		t.Fatalf("answer-level report must be pass, got %q", report.Status.String())
	}
}

// A genuinely-aggregating plan that produced no numeric groups still
// fails loudly — the answer-level lane must not mask a broken aggregation.
func TestRunReconcileArtifacts_AggregatingNoGroupsStillFails(t *testing.T) {
	r := ActionRunner{}
	contribs := []ContributionRecord{
		{Role: LooseText("contribution"), GroupKey: LooseText("g1")}, // participates, but no numeric value/metric → no sums
	}
	_, _, err := r.runReconcileArtifacts(DataAction{ID: "rec"}, contribs, true /* aggregating */)
	if err == nil {
		t.Fatal("aggregating plan with no numeric groups must still fail loudly")
	}
}
