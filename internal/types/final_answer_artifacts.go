package types

// FinalAnswerArtifactsV1 is the atomic finalizer commit. TraceFinding is a
// sibling of the presentation document, not a field inside it.
type FinalAnswerArtifactsV1 struct {
	Document     AnswerDocumentV2 `json:"document"`
	TraceFinding *TraceFindingV1  `json:"trace_finding,omitempty"`
}

func cloneTraceFindingV1(in *TraceFindingV1) *TraceFindingV1 {
	if in == nil {
		return nil
	}
	out := *in
	out.EvidenceRefs = append([]string(nil), in.EvidenceRefs...)
	out.CounterEvidenceRefs = append([]string(nil), in.CounterEvidenceRefs...)
	out.Coverage.Caveats = append([]string(nil), in.Coverage.Caveats...)
	out.Contributors = append([]TraceCauseDecision(nil), in.Contributors...)
	if in.PrimaryCause != nil {
		primary := *in.PrimaryCause
		primary.EvidenceRefs = append([]string(nil), in.PrimaryCause.EvidenceRefs...)
		if in.PrimaryCause.Magnitude != nil {
			magnitude := *in.PrimaryCause.Magnitude
			primary.Magnitude = &magnitude
		}
		out.PrimaryCause = &primary
	}
	for i := range out.Contributors {
		out.Contributors[i].EvidenceRefs = append([]string(nil), in.Contributors[i].EvidenceRefs...)
		if in.Contributors[i].Magnitude != nil {
			magnitude := *in.Contributors[i].Magnitude
			out.Contributors[i].Magnitude = &magnitude
		}
	}
	if in.Unresolved != nil {
		unresolved := *in.Unresolved
		out.Unresolved = &unresolved
	}
	return &out
}
