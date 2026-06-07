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

func TestBuildProgressWindowProjectsGenericDeltas(t *testing.T) {
	events := []ProgressEvent{{
		Round:         1,
		ResultPresent: true,
		ArtifactCount: 1,
		ArtifactRows:  2,
		ArtifactFields: []string{
			"id",
			"amount",
		},
		RuleCoverageRecords: 1,
		Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionExtractRecords,
		}},
	}, {
		Round:         2,
		ResultPresent: true,
		ArtifactCount: 2,
		ArtifactRows:  5,
		ArtifactFields: []string{
			"id",
			"amount",
			"status",
		},
		RuleCoverageRecords: 1,
		ContributionRecords: 3,
		Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionComputeContribs,
		}},
	}}
	window := BuildProgressWindow(events, 4)
	if window.Latest.Round != 2 || window.RepeatedSignatureCount != 1 {
		t.Fatalf("window=%+v, want latest round 2 with unique signature", window)
	}
	if window.LatestDelta.ArtifactRowsDelta != 3 || window.LatestDelta.ArtifactCountDelta != 1 {
		t.Fatalf("delta=%+v, want row/artifact deltas", window.LatestDelta)
	}
	if window.LatestDelta.ContributionDelta != 3 || !containsString(window.LatestDelta.AddedFields, "status") {
		t.Fatalf("delta=%+v, want contribution and field delta", window.LatestDelta)
	}
	if len(window.Recent) != 2 || !containsString(window.Latest.ActionKinds, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("window recent/actions=%+v/%v", window.Recent, window.Latest.ActionKinds)
	}
}

func TestBuildProgressWindowCountsRepeatedSignatures(t *testing.T) {
	events := []ProgressEvent{{
		Round:          1,
		ResultPresent:  true,
		ArtifactCount:  1,
		ArtifactRows:   5,
		ArtifactFields: []string{"id"},
		Actions:        []dataquery.DataAction{{Kind: dataquery.DataActionJoinRecords}},
	}, {
		Round:          2,
		ResultPresent:  true,
		ArtifactCount:  1,
		ArtifactRows:   5,
		ArtifactFields: []string{"id"},
		Actions:        []dataquery.DataAction{{Kind: dataquery.DataActionJoinRecords}},
	}, {
		Round:          3,
		ResultPresent:  true,
		ArtifactCount:  1,
		ArtifactRows:   5,
		ArtifactFields: []string{"id"},
		Actions:        []dataquery.DataAction{{Kind: dataquery.DataActionJoinRecords}},
	}}
	window := BuildProgressWindow(events, 2)
	if window.RepeatedSignatureCount != 3 || len(window.Recent) != 2 {
		t.Fatalf("window=%+v, want three repeated signatures and two recent frames", window)
	}
	if !window.LatestDelta.SignatureChanged && window.LatestDelta.ArtifactRowsDelta != 0 {
		t.Fatalf("delta=%+v, want unchanged signature and zero row delta", window.LatestDelta)
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
