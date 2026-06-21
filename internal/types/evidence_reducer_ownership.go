package types

import "sort"

type EvidenceReducerArtifactClass string

const (
	EvidenceReducerArtifactReadSet                    EvidenceReducerArtifactClass = "read_set"
	EvidenceReducerArtifactReadRanges                 EvidenceReducerArtifactClass = "read_ranges"
	EvidenceReducerArtifactFileTotalLines             EvidenceReducerArtifactClass = "file_total_lines"
	EvidenceReducerArtifactAcceptedEvidence           EvidenceReducerArtifactClass = "accepted_evidence"
	EvidenceReducerArtifactSourceInventoryObservation EvidenceReducerArtifactClass = "source_inventory_observation"
	EvidenceReducerArtifactProgressDecision           EvidenceReducerArtifactClass = "progress_decision"
	EvidenceReducerArtifactNodeStatus                 EvidenceReducerArtifactClass = "node_status"
	EvidenceReducerArtifactNodeAttempts               EvidenceReducerArtifactClass = "node_attempts"
	EvidenceReducerArtifactNodeArtifacts              EvidenceReducerArtifactClass = "node_artifacts"
	EvidenceReducerArtifactForkClosure                EvidenceReducerArtifactClass = "fork_closure"
)

type EvidenceReducerMergeSemantics string

const (
	EvidenceReducerMergeToolResultProjection EvidenceReducerMergeSemantics = "tool_result_projection"
	EvidenceReducerMergeSnapshotReplace      EvidenceReducerMergeSemantics = "snapshot_replace"
	EvidenceReducerMergeAdditiveUnion        EvidenceReducerMergeSemantics = "additive_union"
	EvidenceReducerMergeStableDedup          EvidenceReducerMergeSemantics = "stable_dedup"
	EvidenceReducerMergeLatestAuthority      EvidenceReducerMergeSemantics = "latest_authority"
	EvidenceReducerMergeNormalizedIdentity   EvidenceReducerMergeSemantics = "normalized_identity"
	EvidenceReducerMergeForkClosureDelta     EvidenceReducerMergeSemantics = "fork_closure_delta"
)

// EvidenceReducerOwnershipRow is the machine-checkable ownership contract for
// a reducer input class. It describes which closure artifact families a class
// may mutate and how those mutations merge. It is intentionally typed data,
// not prompt prose or an implementation comment, so replay/resume/status-card
// consumers can share the same authority.
type EvidenceReducerOwnershipRow struct {
	InputClass EvidenceReducerInputClass                                      `json:"input_class"`
	Artifacts  []EvidenceReducerArtifactClass                                 `json:"artifacts,omitempty"`
	Semantics  map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics `json:"semantics,omitempty"`
}

func KnownEvidenceReducerInputClasses() []EvidenceReducerInputClass {
	return []EvidenceReducerInputClass{
		EvidenceReducerInputToolResultRound,
		EvidenceReducerInputStageEvidenceSnapshot,
		EvidenceReducerInputReadCoverageDelta,
		EvidenceReducerInputStageCoverageSnapshot,
		EvidenceReducerInputTurnAHandoffSnapshot,
		EvidenceReducerInputSourceInventoryObservation,
		EvidenceReducerInputForkClosureDelta,
		EvidenceReducerInputReadRunSnapshotSeed,
		EvidenceReducerInputNodeArtifactDelta,
	}
}

func EvidenceReducerOwnershipMatrix() []EvidenceReducerOwnershipRow {
	rows := []EvidenceReducerOwnershipRow{
		{
			InputClass: EvidenceReducerInputToolResultRound,
			Artifacts: []EvidenceReducerArtifactClass{
				EvidenceReducerArtifactReadSet,
				EvidenceReducerArtifactReadRanges,
				EvidenceReducerArtifactFileTotalLines,
				EvidenceReducerArtifactAcceptedEvidence,
			},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactReadSet:          EvidenceReducerMergeToolResultProjection,
				EvidenceReducerArtifactReadRanges:       EvidenceReducerMergeToolResultProjection,
				EvidenceReducerArtifactFileTotalLines:   EvidenceReducerMergeToolResultProjection,
				EvidenceReducerArtifactAcceptedEvidence: EvidenceReducerMergeStableDedup,
			},
		},
		{
			InputClass: EvidenceReducerInputStageEvidenceSnapshot,
			Artifacts:  []EvidenceReducerArtifactClass{EvidenceReducerArtifactAcceptedEvidence},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactAcceptedEvidence: EvidenceReducerMergeStableDedup,
			},
		},
		{
			InputClass: EvidenceReducerInputReadCoverageDelta,
			Artifacts: []EvidenceReducerArtifactClass{
				EvidenceReducerArtifactReadSet,
				EvidenceReducerArtifactReadRanges,
				EvidenceReducerArtifactFileTotalLines,
			},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactReadSet:        EvidenceReducerMergeAdditiveUnion,
				EvidenceReducerArtifactReadRanges:     EvidenceReducerMergeAdditiveUnion,
				EvidenceReducerArtifactFileTotalLines: EvidenceReducerMergeAdditiveUnion,
			},
		},
		{
			InputClass: EvidenceReducerInputStageCoverageSnapshot,
			Artifacts: []EvidenceReducerArtifactClass{
				EvidenceReducerArtifactReadSet,
				EvidenceReducerArtifactReadRanges,
				EvidenceReducerArtifactFileTotalLines,
			},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactReadSet:        EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactReadRanges:     EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactFileTotalLines: EvidenceReducerMergeSnapshotReplace,
			},
		},
		{
			InputClass: EvidenceReducerInputTurnAHandoffSnapshot,
			Artifacts: []EvidenceReducerArtifactClass{
				EvidenceReducerArtifactAcceptedEvidence,
				EvidenceReducerArtifactSourceInventoryObservation,
			},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactAcceptedEvidence:           EvidenceReducerMergeStableDedup,
				EvidenceReducerArtifactSourceInventoryObservation: EvidenceReducerMergeLatestAuthority,
			},
		},
		{
			InputClass: EvidenceReducerInputSourceInventoryObservation,
			Artifacts:  []EvidenceReducerArtifactClass{EvidenceReducerArtifactSourceInventoryObservation},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactSourceInventoryObservation: EvidenceReducerMergeLatestAuthority,
			},
		},
		{
			InputClass: EvidenceReducerInputForkClosureDelta,
			Artifacts:  []EvidenceReducerArtifactClass{EvidenceReducerArtifactForkClosure},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactForkClosure: EvidenceReducerMergeForkClosureDelta,
			},
		},
		{
			InputClass: EvidenceReducerInputReadRunSnapshotSeed,
			Artifacts: []EvidenceReducerArtifactClass{
				EvidenceReducerArtifactReadSet,
				EvidenceReducerArtifactReadRanges,
				EvidenceReducerArtifactFileTotalLines,
				EvidenceReducerArtifactAcceptedEvidence,
				EvidenceReducerArtifactNodeStatus,
				EvidenceReducerArtifactNodeAttempts,
				EvidenceReducerArtifactNodeArtifacts,
				EvidenceReducerArtifactProgressDecision,
				EvidenceReducerArtifactSourceInventoryObservation,
			},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactReadSet:                    EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactReadRanges:                 EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactFileTotalLines:             EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactAcceptedEvidence:           EvidenceReducerMergeStableDedup,
				EvidenceReducerArtifactNodeStatus:                 EvidenceReducerMergeLatestAuthority,
				EvidenceReducerArtifactNodeAttempts:               EvidenceReducerMergeSnapshotReplace,
				EvidenceReducerArtifactNodeArtifacts:              EvidenceReducerMergeNormalizedIdentity,
				EvidenceReducerArtifactProgressDecision:           EvidenceReducerMergeLatestAuthority,
				EvidenceReducerArtifactSourceInventoryObservation: EvidenceReducerMergeLatestAuthority,
			},
		},
		{
			InputClass: EvidenceReducerInputNodeArtifactDelta,
			Artifacts:  []EvidenceReducerArtifactClass{EvidenceReducerArtifactNodeArtifacts},
			Semantics: map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics{
				EvidenceReducerArtifactNodeArtifacts: EvidenceReducerMergeNormalizedIdentity,
			},
		},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].InputClass < rows[j].InputClass
	})
	return rows
}

func EvidenceReducerOwnershipForInputClass(class EvidenceReducerInputClass) (EvidenceReducerOwnershipRow, bool) {
	for _, row := range EvidenceReducerOwnershipMatrix() {
		if row.InputClass == class {
			return cloneEvidenceReducerOwnershipRow(row), true
		}
	}
	return EvidenceReducerOwnershipRow{}, false
}

func ValidEvidenceReducerArtifactClass(class EvidenceReducerArtifactClass) bool {
	switch class {
	case EvidenceReducerArtifactReadSet,
		EvidenceReducerArtifactReadRanges,
		EvidenceReducerArtifactFileTotalLines,
		EvidenceReducerArtifactAcceptedEvidence,
		EvidenceReducerArtifactSourceInventoryObservation,
		EvidenceReducerArtifactProgressDecision,
		EvidenceReducerArtifactNodeStatus,
		EvidenceReducerArtifactNodeAttempts,
		EvidenceReducerArtifactNodeArtifacts,
		EvidenceReducerArtifactForkClosure:
		return true
	default:
		return false
	}
}

func ValidEvidenceReducerMergeSemantics(semantics EvidenceReducerMergeSemantics) bool {
	switch semantics {
	case EvidenceReducerMergeToolResultProjection,
		EvidenceReducerMergeSnapshotReplace,
		EvidenceReducerMergeAdditiveUnion,
		EvidenceReducerMergeStableDedup,
		EvidenceReducerMergeLatestAuthority,
		EvidenceReducerMergeNormalizedIdentity,
		EvidenceReducerMergeForkClosureDelta:
		return true
	default:
		return false
	}
}

func cloneEvidenceReducerOwnershipRow(row EvidenceReducerOwnershipRow) EvidenceReducerOwnershipRow {
	row.Artifacts = append([]EvidenceReducerArtifactClass(nil), row.Artifacts...)
	if row.Semantics != nil {
		semantics := make(map[EvidenceReducerArtifactClass]EvidenceReducerMergeSemantics, len(row.Semantics))
		for artifact, merge := range row.Semantics {
			semantics[artifact] = merge
		}
		row.Semantics = semantics
	}
	return row
}
