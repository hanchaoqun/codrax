package types

import "strings"

// EffectiveChangedPathVerificationCoverage is a read-only projection. Stored
// Python plain-probe labels cannot stand in for a current target receipt, and
// even a valid receipt grants execution, never behavior. Other typed runners
// keep their existing authority; this is not a migration of stored reports.
func EffectiveChangedPathVerificationCoverage(plan *ChangePlan, report *ChangeReport) []ChangedPathVerificationCoverage {
	if report == nil {
		return nil
	}
	out := append([]ChangedPathVerificationCoverage(nil), report.ChangedPathCoverage...)
	probes := pythonReportProbes(plan, report)
	for i := range out {
		row := &out[i]
		row.LanguageFamilies = append([]VerificationLanguageFamily(nil), row.LanguageFamilies...)
		if row.Caliber != ChangedPathVerificationProbe && row.Runner != "verification_probe" {
			continue
		}
		probe, python := probes[row.Source]
		for _, family := range row.LanguageFamilies {
			python = python || family == VerificationLanguagePython
		}
		if !python {
			continue
		}
		if probe.ID == "" {
			probe = VerificationProbe{ID: row.Source, Language: "python"}
		}
		resolution := ResolveVerificationProbeTargetExecution(plan, probe, report)
		if row.Status != ChangedPathVerificationCovered {
			continue
		}
		row.Capability, row.Status = VerificationCapabilityUnknown, ChangedPathVerificationUncovered
		row.ReasonCode = "python_target_execution_unobserved"
		for _, path := range resolution.Paths {
			if path == row.Path {
				row.Capability, row.Status = VerificationCapabilityTargetExecution, ChangedPathVerificationCovered
				row.ReasonCode = "python_changed_owners_executed"
			}
		}
	}
	return out
}

// EffectiveVerificationConfidence retains observations but withdraws satisfied
// Python plain-probe contract/placement claims. Ref declarations are not
// assertion receipts. Exact native project-test and source-text witnesses are
// deliberately outside this lane. Mixed legacy records retain only refs also
// backed by an explicitly declared passing non-Python probe in this plan.
func EffectiveVerificationConfidence(plan *ChangePlan, report *ChangeReport) []VerificationConfidenceRecord {
	if report == nil {
		return nil
	}
	out := append([]VerificationConfidenceRecord(nil), report.VerificationConfidence...)
	for i := range out {
		out[i].ContractRefs = append([]string(nil), out[i].ContractRefs...)
		out[i].ChangedSymbolRefs = append([]string(nil), out[i].ChangedSymbolRefs...)
	}
	python := pythonReportProbes(plan, report)
	if len(python) == 0 {
		return out
	}
	contracts, placements, symbols := map[string]bool{}, map[string]bool{}, map[string]bool{}
	if plan == nil {
		for _, probe := range python {
			for _, path := range ResolveVerificationProbeTargetExecution(nil, probe, report).Paths {
				symbols["path:"+path] = true
			}
		}
	}
	if plan != nil && plan.ID == report.PlanID {
		for _, probe := range ChangePlanVerificationProbes(plan) {
			passed := false
			language := probe.Language
			if VerificationProbeLanguageIsPython(language) {
				language = "python"
			}
			for _, result := range report.TestResults {
				if result.AssertionID == probe.ID && result.Suite == "verification_probe/"+language && result.Passed {
					passed = true
				}
			}
			if !passed {
				continue
			}
			if VerificationProbeLanguageIsPython(probe.Language) {
				resolution := ResolveVerificationProbeTargetExecution(plan, probe, report)
				for _, ref := range probe.ChangedSymbolRefs {
					for _, path := range resolution.Paths {
						if ref == "path:"+path {
							symbols[ref] = true
						}
					}
				}
				continue
			}
			for _, ref := range probe.ContractRefs {
				contracts[ref] = true
			}
			for _, ref := range probe.PlacementRefs {
				placements[ref] = true
			}
			for _, ref := range probe.ChangedSymbolRefs {
				symbols[ref] = true
			}
		}
	}
	for i := range out {
		rec := &out[i]
		if rec.Status != "satisfied" {
			continue
		}
		witness, _ := VerificationConfidenceRecordWitnessKind(*rec)
		if witness == WriteBehaviorWitnessProjectTest || witness == WriteBehaviorWitnessSourceText {
			continue
		}
		var allowed map[string]bool
		switch rec.Category {
		case "probe_contract_refs", "probe_soft_contract_refs":
			allowed = contracts
		case "probe_placement_refs":
			allowed = placements
		case "probe_changed_symbol":
			allowed = symbols
		default:
			continue
		}
		refs := rec.ContractRefs
		if rec.Category == "probe_changed_symbol" {
			refs = rec.ChangedSymbolRefs
		}
		kept := make([]string, 0, len(refs))
		for _, ref := range refs {
			if allowed[ref] {
				kept = append(kept, ref)
			}
		}
		if len(kept) == len(refs) && len(refs) > 0 {
			continue
		}
		if len(kept) == 0 {
			rec.Status, rec.Severity, rec.ReasonCode = "unverified", "warning", "python_plain_probe_assertion_witness_missing"
			if rec.Category == "probe_changed_symbol" {
				rec.ReasonCode = "python_target_execution_unobserved"
			}
		} else if rec.Category == "probe_changed_symbol" {
			rec.ChangedSymbolRefs = kept
		} else {
			rec.ContractRefs = kept
		}
	}
	return out
}

// EffectiveVerificationProbeReport owns only the two authority projections it
// changes. All other report values remain untouched, including pass/score and
// the original command/test receipts. It never persists the projected view.
func EffectiveVerificationProbeReport(plan *ChangePlan, report *ChangeReport) *ChangeReport {
	if report == nil {
		return nil
	}
	out := *report
	out.ChangedPathCoverage = EffectiveChangedPathVerificationCoverage(plan, report)
	out.VerificationConfidence = EffectiveVerificationConfidence(plan, report)
	return &out
}

func pythonReportProbes(plan *ChangePlan, report *ChangeReport) map[string]VerificationProbe {
	out := map[string]VerificationProbe{}
	if plan != nil {
		for _, probe := range ChangePlanVerificationProbes(plan) {
			if VerificationProbeLanguageIsPython(probe.Language) {
				out[probe.ID] = probe
			}
		}
	}
	if report == nil {
		return out
	}
	for _, result := range report.TestResults {
		if result.Suite == "verification_probe/python" && result.AssertionID != "" {
			if _, found := out[result.AssertionID]; !found {
				out[result.AssertionID] = VerificationProbe{ID: result.AssertionID, Language: "python"}
			}
		}
	}
	for _, command := range report.ExecutedCommands {
		if command.Runner == "verification_probe" && ((command.Framework != "" && VerificationProbeLanguageIsPython(command.Framework)) || command.Suite == "verification_probe/python") {
			id := ""
			if command.ProbeExecution != nil && command.ProbeExecution.TargetExecution != nil {
				id = command.ProbeExecution.TargetExecution.ProbeID
			}
			if _, found := out[id]; !found {
				out[id] = VerificationProbe{ID: id, Language: "python"}
			}
		}
	}
	for _, row := range report.ChangedPathCoverage {
		if row.Caliber != ChangedPathVerificationProbe && row.Runner != "verification_probe" {
			continue
		}
		for _, family := range row.LanguageFamilies {
			if family == VerificationLanguagePython {
				id := strings.TrimSpace(row.Source)
				if _, found := out[id]; !found {
					out[id] = VerificationProbe{ID: id, Language: "python"}
				}
			}
		}
	}
	return out
}

func effectiveVerificationProofArtifacts(artifacts []VerificationProofArtifact) []VerificationProofArtifact {
	out := append([]VerificationProofArtifact(nil), artifacts...)
	for i := range out {
		out[i].Report = EffectiveVerificationProbeReport(out[i].Plan, out[i].Report)
	}
	return out
}
