package types

import (
	"strings"
)

const verificationProofMaxReasonCodes = 32

type VerificationProofStatus string

const (
	VerificationProofUnknown     VerificationProofStatus = "unknown"
	VerificationProofUnavailable VerificationProofStatus = "unavailable"
	VerificationProofFailed      VerificationProofStatus = "failed"
	VerificationProofWeak        VerificationProofStatus = "weak"
	VerificationProofAdequate    VerificationProofStatus = "adequate"
	VerificationProofStrong      VerificationProofStatus = "strong"
)

type VerificationProofRunnerEvidence string

const (
	VerificationProofRunnerNone              VerificationProofRunnerEvidence = "none"
	VerificationProofRunnerUnavailable       VerificationProofRunnerEvidence = "unavailable"
	VerificationProofRunnerProject           VerificationProofRunnerEvidence = "project_runner"
	VerificationProofRunnerVerificationProbe VerificationProofRunnerEvidence = "verification_probe"
	VerificationProofRunnerSyntaxFallback    VerificationProofRunnerEvidence = "syntax_fallback"
	VerificationProofRunnerMixed             VerificationProofRunnerEvidence = "mixed"
)

// VerificationProofProfile is the compact typed answer to "how strong was the
// local proof?". It is a projection from existing typed artifacts; it never
// reads terminal logs, model prose, issue text, or prompt content.
type VerificationProofProfile struct {
	Status                 VerificationProofStatus         `json:"status,omitempty"`
	VerificationStatus     VerificationStatus              `json:"verification_status,omitempty"`
	RunnerEvidence         VerificationProofRunnerEvidence `json:"runner_evidence,omitempty"`
	Cumulative             bool                            `json:"cumulative,omitempty"`
	ContributingReports    int                             `json:"contributing_reports,omitempty"`
	ReasonCodes            []string                        `json:"reason_codes,omitempty"`
	ConfidenceReasonCodes  []string                        `json:"confidence_reason_codes,omitempty"`
	TestCount              int                             `json:"test_count,omitempty"`
	CommandCount           int                             `json:"command_count,omitempty"`
	ProbeCount             int                             `json:"probe_count,omitempty"`
	ProjectRunnerCommands  int                             `json:"project_runner_commands,omitempty"`
	ProbeCommands          int                             `json:"probe_commands,omitempty"`
	SyntaxFallbackCommands int                             `json:"syntax_fallback_commands,omitempty"`
	ImpactTargetCount      int                             `json:"impact_target_count,omitempty"`
	ImpactVerifiedCount    int                             `json:"impact_verified_count,omitempty"`
	ImpactUnverifiedCount  int                             `json:"impact_unverified_count,omitempty"`
	PatchReviewVerdict     PatchReviewCoverageVerdict      `json:"patch_review_verdict,omitempty"`
	LocalizationStatus     SourceLocalizationStatus        `json:"localization_status,omitempty"`
}

// VerificationProofArtifact is one typed plan/report pair that may contribute
// to a workflow-level proof profile. Plan is optional because probe-only proof
// plans can have changes: [] and may not pass the stricter persisted-plan load
// path, while their ChangeReport confidence records remain authoritative.
type VerificationProofArtifact struct {
	Plan   *ChangePlan
	Report *ChangeReport
}

func BuildVerificationProofProfile(plan *ChangePlan, report *ChangeReport) VerificationProofProfile {
	out := VerificationProofProfile{RunnerEvidence: VerificationProofRunnerNone}
	addReason := func(code string) {
		code = strings.TrimSpace(code)
		if code != "" {
			out.ReasonCodes = append(out.ReasonCodes, code)
		}
	}
	if report == nil {
		out.Status = VerificationProofUnknown
		addReason("change_report_missing")
		return NormalizeVerificationProofProfile(out)
	}
	status := report.NormalizeVerificationStatus()
	out.VerificationStatus = status
	out.TestCount = verificationProofUnitTestCount(report)
	out.CommandCount = len(report.ExecutedCommands)
	out.ProbeCount = len(report.VerificationConfidence)
	out.RunnerEvidence = verificationProofRunnerEvidence(report)
	for _, rec := range report.VerificationConfidence {
		if code := strings.TrimSpace(rec.ReasonCode); code != "" {
			out.ConfidenceReasonCodes = append(out.ConfidenceReasonCodes, code)
			if verificationConfidenceRecordWeakensProof(rec) {
				addReason(code)
			}
		}
	}
	for _, cmd := range report.ExecutedCommands {
		switch verificationProofCommandClass(cmd) {
		case VerificationProofRunnerProject:
			out.ProjectRunnerCommands++
		case VerificationProofRunnerVerificationProbe:
			out.ProbeCommands++
		case VerificationProofRunnerSyntaxFallback:
			out.SyntaxFallbackCommands++
		}
	}
	if plan != nil {
		if plan.PatchReview != nil {
			coverage := SummarizePatchReviewCoverage(*plan.PatchReview)
			out.PatchReviewVerdict = coverage.Verdict
			switch coverage.Verdict {
			case PatchReviewCoverageVerdictFailed:
				addReason("patch_review_failed")
			case PatchReviewCoverageVerdictUnverified:
				addReason("patch_review_semantic_unverified")
			}
			if coverage.HardBlock {
				addReason("patch_review_hard_block")
			}
		}
		if plan.ImpactAnalysis != nil {
			analysis := NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
			out.ImpactTargetCount = len(analysis.VerificationTargets)
			for _, target := range analysis.VerificationTargets {
				switch strings.TrimSpace(target.CoverageStatus) {
				case "verified":
					out.ImpactVerifiedCount++
				case "", "unknown", "unverified", "unavailable":
					out.ImpactUnverifiedCount++
				default:
					out.ImpactUnverifiedCount++
				}
			}
			if out.ImpactUnverifiedCount > 0 {
				addReason("impact_targets_unverified")
			}
		}
		if plan.LocalizationReview != nil {
			localization := NormalizeSourceLocalizationReview(*plan.LocalizationReview)
			out.LocalizationStatus = localization.Status
			switch localization.Status {
			case SourceLocalizationMissing:
				addReason("source_localization_missing")
			case SourceLocalizationWeak:
				addReason("source_localization_weak")
			}
		}
	}
	switch status {
	case VerificationStatusFailed:
		out.Status = VerificationProofFailed
		addReason(firstNonEmptyVerificationProof(string(report.FailureKind), "local_verification_failed"))
	case VerificationStatusUnavailable:
		out.Status = VerificationProofUnavailable
		addReason(firstNonEmptyVerificationProof(report.FailureReasonCode, string(report.FailureKind), "local_verification_unavailable"))
	case VerificationStatusPassed:
		if verificationProofHasHardFailure(out) {
			out.Status = VerificationProofFailed
		} else if len(out.ReasonCodes) > 0 {
			out.Status = VerificationProofWeak
		} else if out.RunnerEvidence == VerificationProofRunnerVerificationProbe ||
			out.RunnerEvidence == VerificationProofRunnerSyntaxFallback {
			out.Status = VerificationProofAdequate
		} else {
			out.Status = VerificationProofStrong
		}
	default:
		out.Status = VerificationProofUnknown
		addReason("verification_status_unknown")
	}
	return NormalizeVerificationProofProfile(out)
}

// BuildCumulativeVerificationProofProfile projects the proof strength of a
// coherent terminal workflow delivery chain. It keeps the primary report's
// failed/unavailable authority conservative, but lets later/earlier typed
// satisfied probe records cover missing contract/symbol proof records from a
// sibling batch in the same workflow. It never reads logs, issue prose, model
// rationale, or user text.
func BuildCumulativeVerificationProofProfile(primaryPlan *ChangePlan, primaryReport *ChangeReport, artifacts []VerificationProofArtifact) VerificationProofProfile {
	base := BuildVerificationProofProfile(primaryPlan, primaryReport)
	unique := verificationProofUniqueArtifacts(primaryPlan, primaryReport, artifacts)
	if len(unique) <= 1 {
		return base
	}
	out := base
	out.ContributingReports = 0
	out.TestCount = 0
	out.CommandCount = 0
	out.ProbeCount = 0
	out.ProjectRunnerCommands = 0
	out.ProbeCommands = 0
	out.SyntaxFallbackCommands = 0
	out.RunnerEvidence = VerificationProofRunnerNone
	var confidence []VerificationConfidenceRecord
	var confidenceReasonCodes []string
	for _, artifact := range unique {
		if artifact.Report == nil {
			continue
		}
		out.ContributingReports++
		profile := BuildVerificationProofProfile(artifact.Plan, artifact.Report)
		out.TestCount += profile.TestCount
		out.CommandCount += profile.CommandCount
		out.ProbeCount += profile.ProbeCount
		out.ProjectRunnerCommands += profile.ProjectRunnerCommands
		out.ProbeCommands += profile.ProbeCommands
		out.SyntaxFallbackCommands += profile.SyntaxFallbackCommands
		out.RunnerEvidence = mergeVerificationProofRunnerEvidence(out.RunnerEvidence, profile.RunnerEvidence)
		confidence = append(confidence, artifact.Report.VerificationConfidence...)
		confidenceReasonCodes = append(confidenceReasonCodes, profile.ConfidenceReasonCodes...)
	}
	if out.ContributingReports == 0 {
		return base
	}
	if out.ContributingReports <= 1 {
		return base
	}
	out.Cumulative = true
	out.ConfidenceReasonCodes = append(out.ConfidenceReasonCodes, confidenceReasonCodes...)
	out.ReasonCodes = cumulativeVerificationProofReasonCodes(out.ReasonCodes, confidence)
	switch out.Status {
	case VerificationProofFailed, VerificationProofUnavailable, VerificationProofUnknown:
		// Preserve primary terminal authority for failed/unavailable/unknown.
	case VerificationProofWeak, VerificationProofAdequate, VerificationProofStrong:
		if verificationProofHasHardFailure(out) {
			out.Status = VerificationProofFailed
		} else if len(out.ReasonCodes) > 0 {
			out.Status = VerificationProofWeak
		} else if out.RunnerEvidence == VerificationProofRunnerVerificationProbe ||
			out.RunnerEvidence == VerificationProofRunnerSyntaxFallback {
			out.Status = VerificationProofAdequate
		} else {
			out.Status = VerificationProofStrong
		}
	}
	return NormalizeVerificationProofProfile(out)
}

func NormalizeVerificationProofProfile(in VerificationProofProfile) VerificationProofProfile {
	switch in.Status {
	case VerificationProofUnavailable, VerificationProofFailed, VerificationProofWeak, VerificationProofAdequate, VerificationProofStrong:
	default:
		in.Status = VerificationProofUnknown
	}
	switch in.RunnerEvidence {
	case VerificationProofRunnerUnavailable,
		VerificationProofRunnerProject,
		VerificationProofRunnerVerificationProbe,
		VerificationProofRunnerSyntaxFallback,
		VerificationProofRunnerMixed:
	default:
		in.RunnerEvidence = VerificationProofRunnerNone
	}
	in.ReasonCodes = dedupTrimWriteWorkflowRunStringsBounded(in.ReasonCodes, verificationProofMaxReasonCodes)
	in.ConfidenceReasonCodes = dedupTrimWriteWorkflowRunStringsBounded(in.ConfidenceReasonCodes, verificationProofMaxReasonCodes)
	if in.TestCount < 0 {
		in.TestCount = 0
	}
	if in.CommandCount < 0 {
		in.CommandCount = 0
	}
	if in.ProbeCount < 0 {
		in.ProbeCount = 0
	}
	if in.ProjectRunnerCommands < 0 {
		in.ProjectRunnerCommands = 0
	}
	if in.ProbeCommands < 0 {
		in.ProbeCommands = 0
	}
	if in.SyntaxFallbackCommands < 0 {
		in.SyntaxFallbackCommands = 0
	}
	if in.ContributingReports < 0 {
		in.ContributingReports = 0
	}
	if in.ImpactTargetCount < 0 {
		in.ImpactTargetCount = 0
	}
	if in.ImpactVerifiedCount < 0 {
		in.ImpactVerifiedCount = 0
	}
	if in.ImpactUnverifiedCount < 0 {
		in.ImpactUnverifiedCount = 0
	}
	return in
}

func verificationProofUniqueArtifacts(primaryPlan *ChangePlan, primaryReport *ChangeReport, artifacts []VerificationProofArtifact) []VerificationProofArtifact {
	raw := make([]VerificationProofArtifact, 0, len(artifacts)+1)
	raw = append(raw, VerificationProofArtifact{Plan: primaryPlan, Report: primaryReport})
	raw = append(raw, artifacts...)
	seen := map[string]bool{}
	var out []VerificationProofArtifact
	for _, artifact := range raw {
		if artifact.Plan == nil && artifact.Report == nil {
			continue
		}
		key := verificationProofArtifactKey(artifact)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, artifact)
	}
	return out
}

func verificationProofArtifactKey(artifact VerificationProofArtifact) string {
	if artifact.Report != nil {
		if planID := strings.TrimSpace(artifact.Report.PlanID); planID != "" {
			return "report:" + planID
		}
	}
	if artifact.Plan != nil {
		if planID := strings.TrimSpace(artifact.Plan.ID); planID != "" {
			return "plan:" + planID
		}
	}
	return ""
}

func mergeVerificationProofRunnerEvidence(a, b VerificationProofRunnerEvidence) VerificationProofRunnerEvidence {
	a = NormalizeVerificationProofProfile(VerificationProofProfile{RunnerEvidence: a}).RunnerEvidence
	b = NormalizeVerificationProofProfile(VerificationProofProfile{RunnerEvidence: b}).RunnerEvidence
	switch {
	case a == VerificationProofRunnerNone:
		return b
	case b == VerificationProofRunnerNone:
		return a
	case a == b:
		return a
	case a == VerificationProofRunnerMixed || b == VerificationProofRunnerMixed:
		return VerificationProofRunnerMixed
	default:
		return VerificationProofRunnerMixed
	}
}

func cumulativeVerificationProofReasonCodes(current []string, records []VerificationConfidenceRecord) []string {
	unresolved := unresolvedVerificationProofConfidenceReasons(records)
	var out []string
	seen := map[string]bool{}
	for _, code := range current {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if verificationProofReasonCanBeResolvedByConfidence(code) && !unresolved[code] {
			continue
		}
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	for code := range unresolved {
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

func unresolvedVerificationProofConfidenceReasons(records []VerificationConfidenceRecord) map[string]bool {
	type missingRecord struct {
		code     string
		category string
		refs     []string
	}
	var missing []missingRecord
	hardCovered := map[string]bool{}
	softCovered := map[string]bool{}
	changedCovered := false
	for _, rec := range records {
		status := strings.TrimSpace(rec.Status)
		category := strings.TrimSpace(rec.Category)
		code := strings.TrimSpace(rec.ReasonCode)
		switch status {
		case "satisfied":
			switch category {
			case "probe_contract_refs":
				for _, ref := range rec.ContractRefs {
					if ref = strings.TrimSpace(ref); ref != "" {
						hardCovered[ref] = true
					}
				}
			case "probe_soft_contract_refs":
				for _, ref := range rec.ContractRefs {
					if ref = strings.TrimSpace(ref); ref != "" {
						softCovered[ref] = true
					}
				}
			case "probe_changed_symbol":
				for _, ref := range rec.ChangedSymbolRefs {
					if strings.TrimSpace(ref) != "" {
						changedCovered = true
						break
					}
				}
			}
		case "missing":
			if code == "" {
				continue
			}
			switch category {
			case "probe_contract_refs", "probe_soft_contract_refs":
				missing = append(missing, missingRecord{code: code, category: category, refs: append([]string(nil), rec.ContractRefs...)})
			case "probe_changed_symbol":
				missing = append(missing, missingRecord{code: code, category: category, refs: append([]string(nil), rec.ChangedSymbolRefs...)})
			default:
				missing = append(missing, missingRecord{code: code, category: category})
			}
		}
	}
	unresolved := map[string]bool{}
	for _, miss := range missing {
		switch miss.category {
		case "probe_contract_refs":
			if !allVerificationProofRefsCovered(miss.refs, hardCovered) {
				unresolved[miss.code] = true
			}
		case "probe_soft_contract_refs":
			if !allVerificationProofRefsCovered(miss.refs, softCovered) {
				unresolved[miss.code] = true
			}
		case "probe_changed_symbol":
			if len(dedupTrimWriteWorkflowRunStrings(miss.refs)) == 0 {
				if !changedCovered {
					unresolved[miss.code] = true
				}
			} else if !allVerificationProofRefsCovered(miss.refs, verificationProofChangedSymbolCoveredMap(records)) {
				unresolved[miss.code] = true
			}
		default:
			unresolved[miss.code] = true
		}
	}
	return unresolved
}

func verificationProofChangedSymbolCoveredMap(records []VerificationConfidenceRecord) map[string]bool {
	out := map[string]bool{}
	for _, rec := range records {
		if strings.TrimSpace(rec.Status) != "satisfied" || strings.TrimSpace(rec.Category) != "probe_changed_symbol" {
			continue
		}
		for _, ref := range rec.ChangedSymbolRefs {
			if ref = strings.TrimSpace(ref); ref != "" {
				out[ref] = true
			}
		}
	}
	return out
}

func allVerificationProofRefsCovered(refs []string, covered map[string]bool) bool {
	refs = dedupTrimWriteWorkflowRunStrings(refs)
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		if !covered[ref] {
			return false
		}
	}
	return true
}

func verificationProofReasonCanBeResolvedByConfidence(code string) bool {
	switch strings.TrimSpace(code) {
	case "verification_probe_missing_required_contract_ref",
		"verification_probe_missing_soft_contract_ref",
		"verification_probe_missing_changed_symbol_ref":
		return true
	default:
		return false
	}
}

func verificationProofRunnerEvidence(report *ChangeReport) VerificationProofRunnerEvidence {
	if report == nil {
		return VerificationProofRunnerNone
	}
	if report.NormalizeVerificationStatus() == VerificationStatusUnavailable {
		return VerificationProofRunnerUnavailable
	}
	classes := map[VerificationProofRunnerEvidence]struct{}{}
	for _, cmd := range report.ExecutedCommands {
		class := verificationProofCommandClass(cmd)
		if class != VerificationProofRunnerNone && class != VerificationProofRunnerUnavailable {
			classes[class] = struct{}{}
		}
	}
	if len(classes) == 0 {
		for _, result := range report.TestResults {
			if strings.HasPrefix(strings.TrimSpace(result.Suite), "verification_probe/") {
				classes[VerificationProofRunnerVerificationProbe] = struct{}{}
			} else if result.Kind == TestResultKindUnit {
				classes[VerificationProofRunnerProject] = struct{}{}
			}
		}
	}
	if len(classes) == 0 {
		return VerificationProofRunnerNone
	}
	if len(classes) > 1 {
		return VerificationProofRunnerMixed
	}
	for class := range classes {
		return class
	}
	return VerificationProofRunnerNone
}

func verificationProofCommandClass(cmd ExecutedCommand) VerificationProofRunnerEvidence {
	source := strings.TrimSpace(cmd.Source)
	suite := strings.TrimSpace(cmd.Suite)
	outcome := strings.TrimSpace(cmd.Outcome)
	if outcome == "syntax_check_fallback" {
		return VerificationProofRunnerSyntaxFallback
	}
	if strings.Contains(source, "verification_probe") ||
		strings.HasPrefix(suite, "verification_probe/") {
		return VerificationProofRunnerVerificationProbe
	}
	switch outcome {
	case "runner_missing", "parser_error", "not_configured", "suite_skipped":
		return VerificationProofRunnerUnavailable
	}
	if strings.TrimSpace(cmd.Runner) != "" || strings.TrimSpace(cmd.Command) != "" {
		return VerificationProofRunnerProject
	}
	return VerificationProofRunnerNone
}

func verificationProofUnitTestCount(report *ChangeReport) int {
	if report == nil {
		return 0
	}
	var n int
	for _, result := range report.TestResults {
		kind := result.Kind
		if kind == "" {
			kind = TestResultKindUnit
		}
		if kind == TestResultKindUnit {
			n++
		}
	}
	return n
}

func verificationConfidenceRecordWeakensProof(rec VerificationConfidenceRecord) bool {
	switch strings.TrimSpace(rec.Status) {
	case "missing", "unavailable", "failed", "error":
		return true
	default:
		return false
	}
}

func verificationProofHasHardFailure(profile VerificationProofProfile) bool {
	for _, code := range profile.ReasonCodes {
		switch code {
		case "patch_review_failed", "patch_review_hard_block":
			return true
		}
	}
	return false
}

func firstNonEmptyVerificationProof(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func dedupTrimWriteWorkflowRunStringsBounded(in []string, limit int) []string {
	out := dedupTrimWriteWorkflowRunStrings(in)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
