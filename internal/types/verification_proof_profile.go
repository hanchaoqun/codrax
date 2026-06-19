package types

import (
	"sort"
	"strings"
)

const verificationProofMaxReasonCodes = 32
const verificationProofLedgerMaxItems = 64

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

type VerificationProofLedgerState string

const (
	VerificationProofLedgerUnknown       VerificationProofLedgerState = "unknown"
	VerificationProofLedgerVerified      VerificationProofLedgerState = "verified"
	VerificationProofLedgerLowConfidence VerificationProofLedgerState = "low_confidence"
	VerificationProofLedgerUnavailable   VerificationProofLedgerState = "unavailable"
	VerificationProofLedgerFailed        VerificationProofLedgerState = "failed"
)

type VerificationProofLedgerItemStatus string

const (
	VerificationProofLedgerItemUnknown     VerificationProofLedgerItemStatus = "unknown"
	VerificationProofLedgerItemCovered     VerificationProofLedgerItemStatus = "covered"
	VerificationProofLedgerItemMissing     VerificationProofLedgerItemStatus = "missing"
	VerificationProofLedgerItemUnverified  VerificationProofLedgerItemStatus = "unverified"
	VerificationProofLedgerItemUnavailable VerificationProofLedgerItemStatus = "unavailable"
	VerificationProofLedgerItemFailed      VerificationProofLedgerItemStatus = "failed"
	VerificationProofLedgerItemAdvisory    VerificationProofLedgerItemStatus = "advisory"
)

// VerificationProofLedger is the auditable typed proof ledger behind a final
// local-verification confidence verdict. It is a projection from existing
// structured artifacts and never consumes user prose, model rationale, terminal
// narrative, or prompt text.
type VerificationProofLedger struct {
	State                      VerificationProofLedgerState    `json:"state,omitempty"`
	ProfileStatus              VerificationProofStatus         `json:"profile_status,omitempty"`
	VerificationStatus         VerificationStatus              `json:"verification_status,omitempty"`
	RunnerEvidence             VerificationProofRunnerEvidence `json:"runner_evidence,omitempty"`
	Cumulative                 bool                            `json:"cumulative,omitempty"`
	ReasonCodes                []string                        `json:"reason_codes,omitempty"`
	CoverageCounts             map[string]int                  `json:"coverage_counts,omitempty"`
	ObligationCount            int                             `json:"obligation_count,omitempty"`
	CoveredCount               int                             `json:"covered_count,omitempty"`
	UncoveredCount             int                             `json:"uncovered_count,omitempty"`
	UnavailableCount           int                             `json:"unavailable_count,omitempty"`
	FailedCount                int                             `json:"failed_count,omitempty"`
	CapabilityCount            int                             `json:"capability_count,omitempty"`
	CapabilityUnavailableCount int                             `json:"capability_unavailable_count,omitempty"`
	CapabilityFailedCount      int                             `json:"capability_failed_count,omitempty"`
	Obligations                []VerificationProofLedgerItem   `json:"obligations,omitempty"`
	Capabilities               []VerificationProofLedgerItem   `json:"capabilities,omitempty"`
}

type VerificationProofLedgerItem struct {
	ID           string                            `json:"id,omitempty"`
	Kind         string                            `json:"kind,omitempty"`
	Status       VerificationProofLedgerItemStatus `json:"status,omitempty"`
	Source       string                            `json:"source,omitempty"`
	PlanID       string                            `json:"plan_id,omitempty"`
	ReportPlanID string                            `json:"report_plan_id,omitempty"`
	Category     string                            `json:"category,omitempty"`
	Severity     string                            `json:"severity,omitempty"`
	ReasonCode   string                            `json:"reason_code,omitempty"`
	Path         string                            `json:"path,omitempty"`
	RelatedPath  string                            `json:"related_path,omitempty"`
	Symbol       string                            `json:"symbol,omitempty"`
	ContractRef  string                            `json:"contract_ref,omitempty"`
	EvidenceRef  string                            `json:"evidence_ref,omitempty"`
	Detail       string                            `json:"detail,omitempty"`
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

func BuildVerificationProofLedger(primaryPlan *ChangePlan, primaryReport *ChangeReport, artifacts []VerificationProofArtifact) VerificationProofLedger {
	profile := BuildCumulativeVerificationProofProfile(primaryPlan, primaryReport, artifacts)
	out := VerificationProofLedger{
		ProfileStatus:      profile.Status,
		VerificationStatus: profile.VerificationStatus,
		RunnerEvidence:     profile.RunnerEvidence,
		Cumulative:         profile.Cumulative,
		ReasonCodes:        append([]string(nil), profile.ReasonCodes...),
		CoverageCounts:     map[string]int{},
	}
	unique := verificationProofUniqueArtifacts(primaryPlan, primaryReport, artifacts)
	if len(unique) == 0 {
		out.State = verificationProofLedgerStateFromProfile(profile)
		return NormalizeVerificationProofLedger(out)
	}
	for _, artifact := range unique {
		out.addVerificationReportLedgerItems(artifact.Report)
		out.addVerificationConfidenceLedgerItems(artifact.Report)
		out.addPatchReviewLedgerItems(artifact.Plan)
		out.addImpactLedgerItems(artifact.Plan)
	}
	out.State = verificationProofLedgerStateFromProfile(profile)
	return NormalizeVerificationProofLedger(out)
}

func NormalizeVerificationProofLedger(in VerificationProofLedger) VerificationProofLedger {
	switch in.State {
	case VerificationProofLedgerVerified,
		VerificationProofLedgerLowConfidence,
		VerificationProofLedgerUnavailable,
		VerificationProofLedgerFailed:
	default:
		in.State = VerificationProofLedgerUnknown
	}
	in.ProfileStatus = NormalizeVerificationProofProfile(VerificationProofProfile{Status: in.ProfileStatus}).Status
	in.RunnerEvidence = NormalizeVerificationProofProfile(VerificationProofProfile{RunnerEvidence: in.RunnerEvidence}).RunnerEvidence
	in.ReasonCodes = dedupTrimWriteWorkflowRunStringsBounded(in.ReasonCodes, verificationProofMaxReasonCodes)
	in.Obligations = normalizeVerificationProofLedgerItems(in.Obligations, verificationProofLedgerMaxItems)
	in.Obligations = resolveVerificationProofLedgerObligations(in.Obligations)
	in.Capabilities = normalizeVerificationProofLedgerItems(in.Capabilities, verificationProofLedgerMaxItems)
	in.CoverageCounts = map[string]int{}
	for _, item := range in.Obligations {
		status := string(item.Status)
		if status == "" {
			status = string(VerificationProofLedgerItemUnknown)
		}
		in.CoverageCounts[status]++
	}
	if len(in.CoverageCounts) == 0 {
		in.CoverageCounts = nil
	}
	in.ObligationCount = len(in.Obligations)
	in.CapabilityCount = len(in.Capabilities)
	in.CoveredCount = 0
	in.UncoveredCount = 0
	in.UnavailableCount = 0
	in.FailedCount = 0
	in.CapabilityUnavailableCount = 0
	in.CapabilityFailedCount = 0
	for _, item := range in.Obligations {
		switch item.Status {
		case VerificationProofLedgerItemCovered, VerificationProofLedgerItemAdvisory:
			in.CoveredCount++
		case VerificationProofLedgerItemUnavailable:
			in.UnavailableCount++
			in.UncoveredCount++
		case VerificationProofLedgerItemFailed:
			in.FailedCount++
			in.UncoveredCount++
		case VerificationProofLedgerItemMissing,
			VerificationProofLedgerItemUnverified,
			VerificationProofLedgerItemUnknown:
			in.UncoveredCount++
		}
	}
	for _, item := range in.Capabilities {
		switch item.Status {
		case VerificationProofLedgerItemUnavailable:
			in.CapabilityUnavailableCount++
		case VerificationProofLedgerItemFailed:
			in.CapabilityFailedCount++
		}
	}
	if in.State == VerificationProofLedgerVerified && (in.FailedCount > 0 || in.UnavailableCount > 0 || in.UncoveredCount > 0) {
		in.State = VerificationProofLedgerLowConfidence
	}
	if (in.FailedCount > 0 || in.CapabilityFailedCount > 0) && in.State != VerificationProofLedgerUnavailable {
		in.State = VerificationProofLedgerFailed
	}
	return in
}

func verificationProofLedgerStateFromProfile(profile VerificationProofProfile) VerificationProofLedgerState {
	switch NormalizeVerificationProofProfile(profile).Status {
	case VerificationProofFailed:
		return VerificationProofLedgerFailed
	case VerificationProofUnavailable:
		return VerificationProofLedgerUnavailable
	case VerificationProofWeak:
		return VerificationProofLedgerLowConfidence
	case VerificationProofAdequate, VerificationProofStrong:
		return VerificationProofLedgerVerified
	default:
		return VerificationProofLedgerUnknown
	}
}

func (ledger *VerificationProofLedger) addVerificationReportLedgerItems(report *ChangeReport) {
	if ledger == nil || report == nil {
		return
	}
	status := report.NormalizeVerificationStatus()
	itemStatus := VerificationProofLedgerItemUnknown
	switch status {
	case VerificationStatusPassed:
		itemStatus = VerificationProofLedgerItemCovered
	case VerificationStatusUnavailable:
		itemStatus = VerificationProofLedgerItemUnavailable
	case VerificationStatusFailed:
		itemStatus = VerificationProofLedgerItemFailed
	}
	ledger.Capabilities = append(ledger.Capabilities, VerificationProofLedgerItem{
		ID:           verificationProofLedgerStableID("capability", report.PlanID, string(status), report.FailureReasonCode),
		Kind:         "local_verification",
		Status:       itemStatus,
		Source:       "change_report",
		ReportPlanID: strings.TrimSpace(report.PlanID),
		ReasonCode:   firstNonEmptyVerificationProof(report.FailureReasonCode, string(report.FailureKind)),
		Detail:       strings.TrimSpace(report.FailureSummary),
	})
	for _, cmd := range report.ExecutedCommands {
		class := verificationProofCommandClass(cmd)
		if class == VerificationProofRunnerNone {
			continue
		}
		unavailableReasonCode := verificationProofCommandUnavailableReasonCode(cmd, class)
		cmdStatus := VerificationProofLedgerItemCovered
		if unavailableReasonCode != "" {
			cmdStatus = VerificationProofLedgerItemUnavailable
		}
		if executedCommandFailed(cmd) && unavailableReasonCode == "" {
			cmdStatus = VerificationProofLedgerItemFailed
		}
		ledger.Capabilities = append(ledger.Capabilities, VerificationProofLedgerItem{
			ID:           verificationProofLedgerStableID("command", report.PlanID, cmd.Source, cmd.Suite, cmd.Outcome, cmd.Command),
			Kind:         "executed_command",
			Status:       cmdStatus,
			Source:       firstNonEmptyVerificationProof(cmd.Source, "run_tests"),
			ReportPlanID: strings.TrimSpace(report.PlanID),
			Category:     string(class),
			ReasonCode:   firstNonEmptyVerificationProof(unavailableReasonCode, cmd.ReasonCode, cmd.Outcome),
			Path:         strings.TrimSpace(cmd.WorkingDir),
			RelatedPath:  strings.TrimSpace(cmd.Suite),
			Detail:       strings.TrimSpace(cmd.Command),
		})
	}
}

func verificationProofCommandUnavailableReasonCode(cmd ExecutedCommand, class VerificationProofRunnerEvidence) string {
	if code := executedCommandUnavailableReasonCode(cmd); code != "" {
		return code
	}
	outcome := strings.TrimSpace(cmd.Outcome)
	if class == VerificationProofRunnerUnavailable {
		return firstNonEmptyVerificationProof(outcome, string(FailureKindParserError))
	}
	switch outcome {
	case string(FailureKindParserError):
		return string(FailureKindParserError)
	case string(FailureKindRunnerMissing):
		return string(FailureKindRunnerMissing)
	case string(FailureKindNoTests), "synthetic_no_tests", "zero_tests":
		return string(FailureKindNoTests)
	case "not_configured":
		return string(FailureKindRunnerMissing)
	default:
		return ""
	}
}

func (ledger *VerificationProofLedger) addVerificationConfidenceLedgerItems(report *ChangeReport) {
	if ledger == nil || report == nil {
		return
	}
	for _, rec := range report.VerificationConfidence {
		status := verificationProofLedgerStatusFromConfidence(rec.Status)
		kind := verificationProofLedgerKindFromConfidence(rec.Category)
		source := firstNonEmptyVerificationProof(rec.Source, "verification_confidence")
		base := VerificationProofLedgerItem{
			Kind:         kind,
			Status:       status,
			Source:       source,
			ReportPlanID: strings.TrimSpace(report.PlanID),
			Category:     strings.TrimSpace(rec.Category),
			Severity:     strings.TrimSpace(rec.Severity),
			ReasonCode:   strings.TrimSpace(rec.ReasonCode),
			Detail:       strings.TrimSpace(rec.Detail),
		}
		add := func(item VerificationProofLedgerItem) {
			if item.ID == "" {
				item.ID = verificationProofLedgerItemKey(item)
			}
			ledger.Obligations = append(ledger.Obligations, item)
		}
		switch strings.TrimSpace(rec.Category) {
		case "probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs":
			refs := dedupTrimWriteWorkflowRunStrings(rec.ContractRefs)
			if len(refs) == 0 {
				add(base)
				continue
			}
			for _, ref := range refs {
				item := base
				item.ContractRef = ref
				add(item)
			}
		case "probe_changed_symbol":
			refs := dedupTrimWriteWorkflowRunStrings(rec.ChangedSymbolRefs)
			if len(refs) == 0 {
				add(base)
				continue
			}
			for _, ref := range refs {
				item := base
				item.Symbol = ref
				add(item)
			}
		case "probe_execution":
			base.Kind = "verification_probe_execution"
			ledger.Capabilities = append(ledger.Capabilities, base)
		default:
			add(base)
		}
	}
}

func (ledger *VerificationProofLedger) addPatchReviewLedgerItems(plan *ChangePlan) {
	if ledger == nil || plan == nil || plan.PatchReview == nil {
		return
	}
	review := NormalizePatchReviewRecord(*plan.PatchReview)
	for _, finding := range review.Findings {
		if finding.Category != PatchReviewCategorySemanticCoverage &&
			finding.CoverageStatus == "" &&
			!review.HardBlock {
			continue
		}
		kind := firstNonEmptyVerificationProof(patchReviewImpactKindForFinding(finding), string(finding.Category), "patch_review")
		status := verificationProofLedgerStatusFromPatchCoverage(finding.CoverageStatus)
		if review.HardBlock && status != VerificationProofLedgerItemCovered {
			status = VerificationProofLedgerItemFailed
		}
		ledger.Obligations = append(ledger.Obligations, VerificationProofLedgerItem{
			ID:          verificationProofLedgerStableID("patch_review", plan.ID, finding.Code, string(finding.CoverageStatus), finding.Path, finding.SubjectSymbol, finding.EvidenceRef),
			Kind:        kind,
			Status:      status,
			Source:      firstNonEmptyVerificationProof(finding.SourceStage, "patch_review"),
			PlanID:      strings.TrimSpace(plan.ID),
			Category:    string(finding.Category),
			Severity:    string(finding.Severity),
			ReasonCode:  strings.TrimSpace(finding.Code),
			Path:        strings.TrimSpace(finding.Path),
			RelatedPath: strings.TrimSpace(finding.RelatedPath),
			Symbol:      strings.TrimSpace(finding.SubjectSymbol),
			EvidenceRef: strings.TrimSpace(finding.EvidenceRef),
			Detail:      firstNonEmptyVerificationProof(finding.Message, finding.ContextSummary),
		})
	}
}

func (ledger *VerificationProofLedger) addImpactLedgerItems(plan *ChangePlan) {
	if ledger == nil || plan == nil || plan.ImpactAnalysis == nil {
		return
	}
	analysis := NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
	for _, target := range analysis.VerificationTargets {
		ledger.Obligations = append(ledger.Obligations, VerificationProofLedgerItem{
			ID:          verificationProofLedgerStableID("impact", plan.ID, target.ID, target.Kind, target.CoverageStatus, target.Path, target.RelatedPath, target.Symbol, target.ContractRef),
			Kind:        firstNonEmptyVerificationProof(target.Kind, "impact_target"),
			Status:      verificationProofLedgerStatusFromImpactCoverage(target.CoverageStatus),
			Source:      firstNonEmptyVerificationProof(target.Source, "impact_analysis"),
			PlanID:      strings.TrimSpace(plan.ID),
			ReasonCode:  firstNonEmptyVerificationProof(target.CoverageStatus, "coverage_unknown"),
			Path:        strings.TrimSpace(target.Path),
			RelatedPath: strings.TrimSpace(target.RelatedPath),
			Symbol:      strings.TrimSpace(target.Symbol),
			ContractRef: strings.TrimSpace(target.ContractRef),
			EvidenceRef: strings.TrimSpace(target.EvidenceRef),
			Detail:      strings.TrimSpace(target.ProbeID),
		})
	}
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

func resolveVerificationProofLedgerObligations(in []VerificationProofLedgerItem) []VerificationProofLedgerItem {
	coveredContracts := map[string]bool{}
	coveredSymbols := map[string]bool{}
	for _, item := range in {
		if item.Status != VerificationProofLedgerItemCovered {
			continue
		}
		if item.Kind == "behavior_contract" && item.ContractRef != "" {
			coveredContracts[item.ContractRef] = true
		}
		if item.Kind == "changed_symbol" && item.Symbol != "" {
			coveredSymbols[item.Symbol] = true
		}
	}
	out := make([]VerificationProofLedgerItem, 0, len(in))
	for _, item := range in {
		if item.Status == VerificationProofLedgerItemMissing {
			switch item.Kind {
			case "behavior_contract":
				if item.ContractRef != "" && coveredContracts[item.ContractRef] {
					item.Status = VerificationProofLedgerItemCovered
					if item.Detail == "" {
						item.Detail = "resolved_by_cumulative_proof"
					}
				}
			case "changed_symbol":
				if item.Symbol != "" && coveredSymbols[item.Symbol] {
					item.Status = VerificationProofLedgerItemCovered
					if item.Detail == "" {
						item.Detail = "resolved_by_cumulative_proof"
					}
				}
			}
		}
		out = append(out, item)
	}
	return normalizeVerificationProofLedgerItems(out, verificationProofLedgerMaxItems)
}

func normalizeVerificationProofLedgerItems(in []VerificationProofLedgerItem, limit int) []VerificationProofLedgerItem {
	seen := map[string]int{}
	var out []VerificationProofLedgerItem
	for _, item := range in {
		item.ID = strings.TrimSpace(item.ID)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Source = strings.TrimSpace(item.Source)
		item.PlanID = strings.TrimSpace(item.PlanID)
		item.ReportPlanID = strings.TrimSpace(item.ReportPlanID)
		item.Category = strings.TrimSpace(item.Category)
		item.Severity = strings.TrimSpace(item.Severity)
		item.ReasonCode = strings.TrimSpace(item.ReasonCode)
		item.Path = strings.TrimSpace(item.Path)
		item.RelatedPath = strings.TrimSpace(item.RelatedPath)
		item.Symbol = strings.TrimSpace(item.Symbol)
		item.ContractRef = strings.TrimSpace(item.ContractRef)
		item.EvidenceRef = strings.TrimSpace(item.EvidenceRef)
		item.Detail = strings.TrimSpace(item.Detail)
		item.Status = normalizeVerificationProofLedgerItemStatus(item.Status)
		if item.Kind == "" {
			continue
		}
		if item.ID == "" {
			item.ID = verificationProofLedgerItemKey(item)
		}
		if idx, ok := seen[item.ID]; ok {
			out[idx] = mergeVerificationProofLedgerItem(out[idx], item)
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if verificationProofLedgerStatusRank(out[i].Status) != verificationProofLedgerStatusRank(out[j].Status) {
			return verificationProofLedgerStatusRank(out[i].Status) < verificationProofLedgerStatusRank(out[j].Status)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func mergeVerificationProofLedgerItem(a, b VerificationProofLedgerItem) VerificationProofLedgerItem {
	if verificationProofLedgerStatusRank(b.Status) < verificationProofLedgerStatusRank(a.Status) {
		a.Status = b.Status
	}
	if a.Source == "" {
		a.Source = b.Source
	}
	if a.PlanID == "" {
		a.PlanID = b.PlanID
	}
	if a.ReportPlanID == "" {
		a.ReportPlanID = b.ReportPlanID
	}
	if a.Category == "" {
		a.Category = b.Category
	}
	if a.Severity == "" {
		a.Severity = b.Severity
	}
	if a.ReasonCode == "" {
		a.ReasonCode = b.ReasonCode
	}
	if a.Path == "" {
		a.Path = b.Path
	}
	if a.RelatedPath == "" {
		a.RelatedPath = b.RelatedPath
	}
	if a.Symbol == "" {
		a.Symbol = b.Symbol
	}
	if a.ContractRef == "" {
		a.ContractRef = b.ContractRef
	}
	if a.EvidenceRef == "" {
		a.EvidenceRef = b.EvidenceRef
	}
	if a.Detail == "" {
		a.Detail = b.Detail
	}
	return a
}

func normalizeVerificationProofLedgerItemStatus(in VerificationProofLedgerItemStatus) VerificationProofLedgerItemStatus {
	switch in {
	case VerificationProofLedgerItemCovered,
		VerificationProofLedgerItemMissing,
		VerificationProofLedgerItemUnverified,
		VerificationProofLedgerItemUnavailable,
		VerificationProofLedgerItemFailed,
		VerificationProofLedgerItemAdvisory:
		return in
	default:
		return VerificationProofLedgerItemUnknown
	}
}

func verificationProofLedgerStatusRank(status VerificationProofLedgerItemStatus) int {
	switch normalizeVerificationProofLedgerItemStatus(status) {
	case VerificationProofLedgerItemFailed:
		return 0
	case VerificationProofLedgerItemUnavailable:
		return 1
	case VerificationProofLedgerItemMissing:
		return 2
	case VerificationProofLedgerItemUnverified:
		return 3
	case VerificationProofLedgerItemUnknown:
		return 4
	case VerificationProofLedgerItemCovered:
		return 5
	case VerificationProofLedgerItemAdvisory:
		return 6
	default:
		return 7
	}
}

func verificationProofLedgerStatusFromConfidence(status string) VerificationProofLedgerItemStatus {
	switch strings.TrimSpace(status) {
	case "satisfied", "covered", "passed", "verified":
		return VerificationProofLedgerItemCovered
	case "missing":
		return VerificationProofLedgerItemMissing
	case "unavailable":
		return VerificationProofLedgerItemUnavailable
	case "failed", "error":
		return VerificationProofLedgerItemFailed
	case "unverified":
		return VerificationProofLedgerItemUnverified
	default:
		return VerificationProofLedgerItemUnknown
	}
}

func verificationProofLedgerStatusFromPatchCoverage(status PatchReviewCoverageStatus) VerificationProofLedgerItemStatus {
	switch status {
	case PatchReviewCoverageVerified:
		return VerificationProofLedgerItemCovered
	case PatchReviewCoverageUnavailable:
		return VerificationProofLedgerItemUnavailable
	case PatchReviewCoverageUnverified:
		return VerificationProofLedgerItemUnverified
	case PatchReviewCoverageAdvisory:
		return VerificationProofLedgerItemAdvisory
	default:
		return VerificationProofLedgerItemUnknown
	}
}

func verificationProofLedgerStatusFromImpactCoverage(status string) VerificationProofLedgerItemStatus {
	switch strings.TrimSpace(status) {
	case "verified", "covered", "passed":
		return VerificationProofLedgerItemCovered
	case "unavailable":
		return VerificationProofLedgerItemUnavailable
	case "unverified":
		return VerificationProofLedgerItemUnverified
	case "failed", "error":
		return VerificationProofLedgerItemFailed
	case "missing":
		return VerificationProofLedgerItemMissing
	default:
		return VerificationProofLedgerItemUnknown
	}
}

func verificationProofLedgerKindFromConfidence(category string) string {
	switch strings.TrimSpace(category) {
	case "probe_contract_refs", "probe_soft_contract_refs":
		return "behavior_contract"
	case "probe_placement_refs":
		return "rendered_text_placement_contract"
	case "probe_changed_symbol":
		return "changed_symbol"
	case "probe_execution":
		return "verification_probe_execution"
	default:
		return firstNonEmptyVerificationProof(strings.TrimSpace(category), "verification_confidence")
	}
}

func verificationProofLedgerItemKey(item VerificationProofLedgerItem) string {
	return verificationProofLedgerStableID(
		item.Kind,
		string(item.Status),
		item.Source,
		item.PlanID,
		item.ReportPlanID,
		item.Category,
		item.ReasonCode,
		item.Path,
		item.RelatedPath,
		item.Symbol,
		item.ContractRef,
		item.EvidenceRef,
	)
}

func verificationProofLedgerStableID(parts ...string) string {
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			trimmed = append(trimmed, part)
		}
	}
	return strings.Join(trimmed, "\x00")
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
	placementCovered := map[string]bool{}
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
			case "probe_placement_refs":
				for _, ref := range rec.ContractRefs {
					if ref = strings.TrimSpace(ref); ref != "" {
						placementCovered[ref] = true
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
			case "probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs":
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
		case "probe_placement_refs":
			if !allVerificationProofRefsCovered(miss.refs, placementCovered) {
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
		"verification_probe_missing_required_placement_ref",
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
