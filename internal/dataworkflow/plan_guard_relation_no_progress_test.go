package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func relationNoProgressGuardFixtureState() WorkflowStateView {
	return WorkflowStateView{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	}
}

func relationNoProgressGuardFixtureEvents(n int) []ProgressEvent {
	events := make([]ProgressEvent, 0, n)
	for i := 0; i < n; i++ {
		events = append(events, ProgressEvent{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionMappingCandidate,
			}},
		})
	}
	return events
}

// A plan that repeats relation materialization while the workflow is
// stalled at the contribution stage (zero contribution records, recent
// relation results with no ledger progress) must be rejected with the
// productive action vocabulary, not accepted into another zero-progress
// round.
func TestRelationNoProgressGuard_RejectsRepeatedMappingDuringStall(t *testing.T) {
	guard := RelationNoProgressGuardResult(RelationNoProgressGuardInput{
		State: relationNoProgressGuardFixtureState(),
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionMappingCandidate,
		}}},
		ProgressEvents: relationNoProgressGuardFixtureEvents(2),
		NoProgressStop: 2,
	})
	if guard.Empty() {
		t.Fatal("stalled repeated relation materialization must be rejected")
	}
	if !strings.Contains(guard.Message, "repeats relation materialization") {
		t.Fatalf("guard message should explain the stall, got %q", guard.Message)
	}
	if !strings.Contains(guard.Message, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("guard message should offer productive action kinds, got %q", guard.Message)
	}
}

// Productive (non-relation) actions pass through even during a stall —
// the gate narrows one redundant action class, never the whole space.
func TestRelationNoProgressGuard_AllowsProductiveActionDuringStall(t *testing.T) {
	guard := RelationNoProgressGuardResult(RelationNoProgressGuardInput{
		State: relationNoProgressGuardFixtureState(),
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionFilterRecords,
		}}},
		ProgressEvents: relationNoProgressGuardFixtureEvents(2),
		NoProgressStop: 2,
	})
	if !guard.Empty() {
		t.Fatalf("productive action must not be gated, got %q", guard.Message)
	}
}

// Relation materialization stays legal while real ledger progress
// exists or before the stall threshold — the gate reads only the
// precise structural stall predicate.
func TestRelationNoProgressGuard_SilentWithProgressOrBelowThreshold(t *testing.T) {
	withProgress := relationNoProgressGuardFixtureState()
	withProgress.ContributionRecords = 1
	guard := RelationNoProgressGuardResult(RelationNoProgressGuardInput{
		State: withProgress,
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionMappingCandidate,
		}}},
		ProgressEvents: relationNoProgressGuardFixtureEvents(3),
		NoProgressStop: 2,
	})
	if !guard.Empty() {
		t.Fatalf("contribution progress must clear the gate, got %q", guard.Message)
	}

	guard = RelationNoProgressGuardResult(RelationNoProgressGuardInput{
		State: relationNoProgressGuardFixtureState(),
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionMappingCandidate,
		}}},
		ProgressEvents: relationNoProgressGuardFixtureEvents(1),
		NoProgressStop: 2,
	})
	if !guard.Empty() {
		t.Fatalf("below-threshold relation work must not be gated, got %q", guard.Message)
	}
}
