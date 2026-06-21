package types

type EvidenceReducerInputClass string

const (
	EvidenceReducerInputToolResultRound            EvidenceReducerInputClass = "tool_result_round"
	EvidenceReducerInputStageEvidenceSnapshot      EvidenceReducerInputClass = "stage_evidence_snapshot"
	EvidenceReducerInputReadCoverageDelta          EvidenceReducerInputClass = "read_coverage_delta"
	EvidenceReducerInputStageCoverageSnapshot      EvidenceReducerInputClass = "stage_coverage_snapshot"
	EvidenceReducerInputTurnAHandoffSnapshot       EvidenceReducerInputClass = "turn_a_handoff_snapshot"
	EvidenceReducerInputSourceInventoryObservation EvidenceReducerInputClass = "source_inventory_observation_snapshot"
	EvidenceReducerInputForkClosureDelta           EvidenceReducerInputClass = "fork_closure_delta"
	EvidenceReducerInputReadRunSnapshotSeed        EvidenceReducerInputClass = "read_run_snapshot_seed"
)

type EvidenceReducerInput struct {
	Class                      EvidenceReducerInputClass
	ToolResults                []ToolResult
	EvidenceItems              []EvidenceItem
	HandoffCarriers            []ToolHandoffCarrier
	ReadSet                    map[string]bool
	ReadRanges                 map[string][]LineRange
	FileTotalLines             map[string]int
	ReplaceReadSet             bool
	ReplaceReadRanges          bool
	ReplaceFileTotalLines      bool
	AcceptedEvidence           []AcceptedEvidenceRef
	NodeStatuses               map[string]NodeExecStatus
	NodeAttempts               map[string]int
	ProgressDecision           ProgressDecision
	HasProgressDecision        bool
	SourceInventoryObservation SourceInventoryObservation
	ForkClosure                *EvidenceClosure
}

// EvidenceRoundDelta is the typed projection derived from one batch of
// reducer input rows. It is used by the production read reducer family so
// scheduler code can compare "what this round proves" without re-reading
// prompt prose.
type EvidenceRoundDelta struct {
	ReadSet                    map[string]bool            `json:"read_set,omitempty"`
	ReadRanges                 map[string][]LineRange     `json:"read_ranges,omitempty"`
	FileTotalLines             map[string]int             `json:"file_total_lines,omitempty"`
	ReplaceReadSet             bool                       `json:"replace_read_set,omitempty"`
	ReplaceReadRanges          bool                       `json:"replace_read_ranges,omitempty"`
	ReplaceFileTotalLines      bool                       `json:"replace_file_total_lines,omitempty"`
	AcceptedEvidence           []AcceptedEvidenceRef      `json:"accepted_evidence,omitempty"`
	NodeStatuses               map[string]NodeExecStatus  `json:"node_statuses,omitempty"`
	NodeAttempts               map[string]int             `json:"node_attempts,omitempty"`
	ProgressDecision           ProgressDecision           `json:"progress_decision,omitempty"`
	HasProgressDecision        bool                       `json:"has_progress_decision,omitempty"`
	SourceInventoryObservation SourceInventoryObservation `json:"source_inventory_observation,omitempty"`
}

func (d EvidenceRoundDelta) Empty() bool {
	return len(d.ReadSet) == 0 &&
		len(d.ReadRanges) == 0 &&
		len(d.FileTotalLines) == 0 &&
		len(d.AcceptedEvidence) == 0 &&
		len(d.NodeStatuses) == 0 &&
		len(d.NodeAttempts) == 0 &&
		!d.HasProgressDecision &&
		!d.SourceInventoryObservation.IsActive()
}

func EvidenceRoundDeltaFromToolResults(results []ToolResult, repoRoot string) EvidenceRoundDelta {
	readSet, readRanges, totals := ExtractReadCoverage(results, repoRoot)
	return EvidenceRoundDelta{
		ReadSet:          readSet,
		ReadRanges:       readRanges,
		FileTotalLines:   totals,
		AcceptedEvidence: acceptedEvidenceRefsFromToolResults(results),
	}
}

func EvidenceRoundDeltaFromReducerInput(input EvidenceReducerInput, repoRoot string) EvidenceRoundDelta {
	switch input.Class {
	case EvidenceReducerInputToolResultRound:
		return EvidenceRoundDeltaFromToolResults(input.ToolResults, repoRoot)
	case EvidenceReducerInputStageEvidenceSnapshot:
		return EvidenceRoundDelta{
			AcceptedEvidence: acceptedEvidenceRefsFromEvidenceItems(input.EvidenceItems),
		}
	case EvidenceReducerInputReadCoverageDelta:
		return EvidenceRoundDelta{
			ReadSet:        cloneBoolMap(input.ReadSet),
			ReadRanges:     cloneLineRangeMap(input.ReadRanges),
			FileTotalLines: cloneIntMap(input.FileTotalLines),
		}
	case EvidenceReducerInputStageCoverageSnapshot:
		return EvidenceRoundDelta{
			ReadSet:               cloneBoolMap(input.ReadSet),
			ReadRanges:            cloneLineRangeMap(input.ReadRanges),
			FileTotalLines:        cloneIntMap(input.FileTotalLines),
			ReplaceReadSet:        input.ReplaceReadSet,
			ReplaceReadRanges:     input.ReplaceReadRanges,
			ReplaceFileTotalLines: input.ReplaceFileTotalLines,
		}
	case EvidenceReducerInputTurnAHandoffSnapshot:
		carriers := NormalizeToolHandoffCarriers(input.HandoffCarriers)
		refs := acceptedEvidenceRefsFromHandoffCarriers(carriers)
		for _, item := range input.EvidenceItems {
			if ref, ok := AcceptedEvidenceRefFromEvidenceItem(item); ok {
				refs = append(refs, ref)
			}
		}
		return EvidenceRoundDelta{
			AcceptedEvidence:           normalizeAcceptedEvidenceRefs(refs),
			SourceInventoryObservation: CloneSourceInventoryObservation(input.SourceInventoryObservation),
		}
	case EvidenceReducerInputSourceInventoryObservation:
		return EvidenceRoundDelta{
			SourceInventoryObservation: CloneSourceInventoryObservation(input.SourceInventoryObservation),
		}
	case EvidenceReducerInputReadRunSnapshotSeed:
		return EvidenceRoundDelta{
			ReadSet:                    cloneBoolMap(input.ReadSet),
			ReadRanges:                 cloneLineRangeMap(input.ReadRanges),
			FileTotalLines:             cloneIntMap(input.FileTotalLines),
			ReplaceReadSet:             input.ReplaceReadSet,
			ReplaceReadRanges:          input.ReplaceReadRanges,
			ReplaceFileTotalLines:      input.ReplaceFileTotalLines,
			AcceptedEvidence:           normalizeAcceptedEvidenceRefs(input.AcceptedEvidence),
			NodeStatuses:               cloneNodeExecStatusMap(input.NodeStatuses),
			NodeAttempts:               cloneNodeExecAttemptMap(input.NodeAttempts),
			ProgressDecision:           input.ProgressDecision,
			HasProgressDecision:        input.HasProgressDecision,
			SourceInventoryObservation: CloneSourceInventoryObservation(input.SourceInventoryObservation),
		}
	default:
		return EvidenceRoundDelta{}
	}
}

// IngestRound merges one batch of typed tool outputs into the closure and
// returns the exact delta it consumed. It is the compatibility entrypoint for
// tool-event ownership; new read/handoff reducers should call
// IngestEvidenceReducerInput with an explicit input class.
func (c *EvidenceClosure) IngestRound(results []ToolResult, repoRoot string) EvidenceRoundDelta {
	return c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:       EvidenceReducerInputToolResultRound,
		ToolResults: results,
	}, repoRoot)
}

func (c *EvidenceClosure) IngestEvidenceReducerInput(input EvidenceReducerInput, repoRoot string) EvidenceRoundDelta {
	if input.Class == EvidenceReducerInputForkClosureDelta {
		if c != nil && input.ForkClosure != nil {
			c.MergeFrom(input.ForkClosure)
		}
		return EvidenceRoundDelta{}
	}
	delta := EvidenceRoundDeltaFromReducerInput(input, repoRoot)
	if c == nil || delta.Empty() {
		return delta
	}
	if len(delta.ReadSet) > 0 {
		if delta.ReplaceReadSet {
			c.SetReadSet(delta.ReadSet)
		} else {
			c.AddReadSet(delta.ReadSet)
		}
	}
	if len(delta.ReadRanges) > 0 {
		if delta.ReplaceReadRanges {
			c.SetReadRanges(delta.ReadRanges)
		} else {
			c.AddReadRanges(delta.ReadRanges)
		}
	}
	if len(delta.FileTotalLines) > 0 {
		if delta.ReplaceFileTotalLines {
			c.SetFileTotalLines(delta.FileTotalLines)
		} else {
			for file, total := range delta.FileTotalLines {
				c.RecordFileTotalLines(file, total)
			}
		}
	}
	if len(delta.AcceptedEvidence) > 0 {
		c.AppendAcceptedEvidenceRefs(delta.AcceptedEvidence)
	}
	for nodeID, status := range delta.NodeStatuses {
		c.SetNodeExecStatus(nodeID, status)
	}
	if len(delta.NodeAttempts) > 0 {
		c.SetNodeExecAttempts(delta.NodeAttempts)
	}
	if delta.HasProgressDecision {
		c.SetLatestProgressDecision(delta.ProgressDecision)
	}
	if delta.SourceInventoryObservation.IsActive() {
		c.RecordSourceInventoryObservation(delta.SourceInventoryObservation)
	}
	return delta
}

func acceptedEvidenceRefsFromToolResults(results []ToolResult) []AcceptedEvidenceRef {
	var refs []AcceptedEvidenceRef
	for _, result := range results {
		if result.Handoff == nil {
			continue
		}
		carrier := NormalizeToolHandoffCarrier(*result.Handoff)
		refs = append(refs, carrier.AcceptedEvidence...)
	}
	return normalizeAcceptedEvidenceRefs(refs)
}

func acceptedEvidenceRefsFromHandoffCarriers(carriers []ToolHandoffCarrier) []AcceptedEvidenceRef {
	var refs []AcceptedEvidenceRef
	for _, carrier := range carriers {
		carrier = NormalizeToolHandoffCarrier(carrier)
		refs = append(refs, carrier.AcceptedEvidence...)
	}
	return normalizeAcceptedEvidenceRefs(refs)
}
