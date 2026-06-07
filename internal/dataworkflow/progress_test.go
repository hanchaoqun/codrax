package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestRecentRelationNoProgressCountStopsAtLedgerProgress(t *testing.T) {
	events := []ProgressEvent{
		{
			ResultPresent:       true,
			ContributionRecords: 1,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionComputeContribs,
			}},
		},
		{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionApplyResolutions,
			}},
		},
		{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionJoinRecords,
			}},
		},
	}
	count, kinds := RecentRelationNoProgressCount(events)
	if count != 2 {
		t.Fatalf("count=%d kinds=%v, want 2 relation events after ledger progress", count, kinds)
	}
}

func TestRecentRelationNoProgressCountStopsAtProgressSignatureChange(t *testing.T) {
	events := []ProgressEvent{
		{
			ResultPresent: true,
			ArtifactCount: 1,
			ArtifactRows:  5,
			ArtifactFields: []string{
				"id",
			},
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionJoinRecords,
			}},
		},
		{
			ResultPresent: true,
			ArtifactCount: 1,
			ArtifactRows:  5,
			ArtifactFields: []string{
				"id",
				"canonical_id",
			},
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionJoinRecords,
			}},
		},
	}
	count, kinds := RecentRelationNoProgressCount(events)
	if count != 1 || len(kinds) != 1 || kinds[0] != string(dataquery.DataActionJoinRecords) {
		t.Fatalf("count=%d kinds=%v, want only latest relation event because schema changed", count, kinds)
	}
}

func TestRelationNoProgressViolationUsesTypedFacts(t *testing.T) {
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
	events := []ProgressEvent{
		{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionApplyResolutions,
			}},
		},
		{
			ResultPresent: true,
			Actions: []dataquery.DataAction{{
				Kind: dataquery.DataActionApplyResolutions,
			}},
		},
	}
	violation, ok := RelationNoProgressViolation(facts, events, 2)
	if !ok {
		t.Fatalf("RelationNoProgressViolation ok=false, want typed violation")
	}
	if violation.Code != ViolationStageNoProgress {
		t.Fatalf("Code=%q, want %q", violation.Code, ViolationStageNoProgress)
	}
	if violation.ActionKind != string(dataquery.DataActionApplyResolutions) {
		t.Fatalf("ActionKind=%q, want apply_entity_resolutions", violation.ActionKind)
	}
	if !WouldRepeatRelationNoProgress(facts, events, dataquery.DataActionJoinRecords, 2) {
		t.Fatalf("WouldRepeatRelationNoProgress=false, want relation fallback blocked")
	}
	if WouldRepeatRelationNoProgress(facts, events, dataquery.DataActionComputeContribs, 2) {
		t.Fatalf("compute_contributions should not be treated as relation materialization")
	}
}
