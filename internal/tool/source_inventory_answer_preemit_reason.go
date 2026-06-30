package tool

import "strings"

func sourceInventoryAnswerPreEmitReasonCodes(a SourceInventoryAnswerPreEmitAuthority) []string {
	var out []string
	add := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		for _, existing := range out {
			if existing == code {
				return
			}
		}
		out = append(out, code)
	}
	for _, code := range a.Snapshot.ReasonCodes {
		add("snapshot:" + code)
	}
	for _, code := range a.View.ReasonCodes {
		add("view:" + code)
	}
	if a.View.CanBlockCompletion {
		add("view:block_completion")
	}
	if a.View.CanOnlyCaveat {
		add("view:caveat_only")
	}
	if len(a.View.CitationObligations) > 0 {
		add("view:citation_obligations")
	}
	if a.CandidateUniverseGap.Blocking {
		add("candidate_universe_blocking")
	}
	if a.DuplicateLocationGap.Blocking {
		add("duplicate_location_blocking")
	}
	if a.SurfaceFamilyGap.Blocking {
		add("surface_family_blocking")
	}
	if a.ExactAbsenceBlocking {
		add("exact_absence_requires_inventory_proof")
	}
	if a.AcceptedExactUniverse {
		add("accepted_exact_universe")
	}
	if a.AcceptedRequestedUniverse {
		add("accepted_requested_universe")
	}
	if a.EnumerationRowCount > 0 {
		add("accepted_enumeration_rows")
	}
	if a.EnumerationCoverage.Complete() {
		add("accepted_enumeration_rows_visible")
	}
	return out
}
