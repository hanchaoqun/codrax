package types

// EvidenceRoundDelta is the typed projection derived from one batch of
// ToolResult rows. It is used by the M1c shadow reducer so scheduler code can
// compare "what this round proves" without re-reading prompt prose.
type EvidenceRoundDelta struct {
	ReadSet          map[string]bool        `json:"read_set,omitempty"`
	ReadRanges       map[string][]LineRange `json:"read_ranges,omitempty"`
	FileTotalLines   map[string]int         `json:"file_total_lines,omitempty"`
	AcceptedEvidence []AcceptedEvidenceRef  `json:"accepted_evidence,omitempty"`
}

func (d EvidenceRoundDelta) Empty() bool {
	return len(d.ReadSet) == 0 &&
		len(d.ReadRanges) == 0 &&
		len(d.FileTotalLines) == 0 &&
		len(d.AcceptedEvidence) == 0
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

// IngestRound merges one batch of typed tool outputs into the closure and
// returns the exact delta it consumed. The method accepts only ToolResult rows:
// callers that want to project StageOutput.EvidenceItems must keep using
// MutableState.AppendEvidence / AppendAcceptedEvidenceItems.
func (c *EvidenceClosure) IngestRound(results []ToolResult, repoRoot string) EvidenceRoundDelta {
	delta := EvidenceRoundDeltaFromToolResults(results, repoRoot)
	if c == nil || delta.Empty() {
		return delta
	}
	if len(delta.ReadSet) > 0 {
		c.AddReadSet(delta.ReadSet)
	}
	if len(delta.ReadRanges) > 0 {
		c.AddReadRanges(delta.ReadRanges)
	}
	for file, total := range delta.FileTotalLines {
		c.RecordFileTotalLines(file, total)
	}
	if len(delta.AcceptedEvidence) > 0 {
		c.AppendAcceptedEvidenceRefs(delta.AcceptedEvidence)
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
