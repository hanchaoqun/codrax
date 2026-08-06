package types

func cloneTraceFindingV1(in *TraceFindingV1) *TraceFindingV1 {
	if in == nil {
		return nil
	}
	out := *in
	if in.PrimaryCause != nil {
		cp := *in.PrimaryCause
		if in.PrimaryCause.Magnitude != nil {
			mag := *in.PrimaryCause.Magnitude
			cp.Magnitude = &mag
		}
		if len(in.PrimaryCause.EvidenceRefs) > 0 {
			cp.EvidenceRefs = append([]string(nil), in.PrimaryCause.EvidenceRefs...)
		}
		out.PrimaryCause = &cp
	}
	if len(in.Contributors) > 0 {
		out.Contributors = make([]TraceCauseDecision, len(in.Contributors))
		for i, c := range in.Contributors {
			out.Contributors[i] = c
			if c.Magnitude != nil {
				mag := *c.Magnitude
				out.Contributors[i].Magnitude = &mag
			}
			if len(c.EvidenceRefs) > 0 {
				out.Contributors[i].EvidenceRefs = append([]string(nil), c.EvidenceRefs...)
			}
		}
	}
	if in.Unresolved != nil {
		cp := *in.Unresolved
		out.Unresolved = &cp
	}
	if len(in.EvidenceRefs) > 0 {
		out.EvidenceRefs = append([]string(nil), in.EvidenceRefs...)
	}
	if len(in.CounterEvidenceRefs) > 0 {
		out.CounterEvidenceRefs = append([]string(nil), in.CounterEvidenceRefs...)
	}
	if len(in.Coverage.Caveats) > 0 {
		out.Coverage.Caveats = append([]string(nil), in.Coverage.Caveats...)
	}
	return &out
}
