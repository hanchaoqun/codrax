package types

import (
	"sort"
	"strings"
	"time"
)

type PatchReviewSeverity string

const (
	PatchReviewSeverityInfo    PatchReviewSeverity = "info"
	PatchReviewSeverityWarning PatchReviewSeverity = "warning"
	PatchReviewSeverityError   PatchReviewSeverity = "error"
)

type PatchReviewFindingCategory string

const (
	PatchReviewCategoryScope            PatchReviewFindingCategory = "scope"
	PatchReviewCategoryStructural       PatchReviewFindingCategory = "structural"
	PatchReviewCategorySemanticCoverage PatchReviewFindingCategory = "semantic_coverage"
	PatchReviewCategoryConvention       PatchReviewFindingCategory = "convention"
)

type PatchReviewCoverageStatus string

const (
	PatchReviewCoverageUnknown     PatchReviewCoverageStatus = "unknown"
	PatchReviewCoverageVerified    PatchReviewCoverageStatus = "verified"
	PatchReviewCoverageUnverified  PatchReviewCoverageStatus = "unverified"
	PatchReviewCoverageUnavailable PatchReviewCoverageStatus = "unavailable"
	PatchReviewCoverageAdvisory    PatchReviewCoverageStatus = "advisory"
)

type PatchReviewCoverageVerdict string

const (
	PatchReviewCoverageVerdictClean      PatchReviewCoverageVerdict = "clean"
	PatchReviewCoverageVerdictVerified   PatchReviewCoverageVerdict = "verified"
	PatchReviewCoverageVerdictUnverified PatchReviewCoverageVerdict = "unverified"
	PatchReviewCoverageVerdictFailed     PatchReviewCoverageVerdict = "failed"
	PatchReviewCoverageVerdictAdvisory   PatchReviewCoverageVerdict = "advisory"
)

const (
	PatchReviewImpactKindBehaviorContract = "behavior_contract"
	PatchReviewImpactKindChangedSymbol    = "changed_symbol"
	PatchReviewImpactKindDependent        = "dependent"
	PatchReviewImpactKindTestSurface      = "test_surface"
	PatchReviewImpactKindEffectFollowup   = "effect_followup"
	PatchReviewImpactKindChangedFile      = "changed_file"
	PatchReviewImpactKindDependency       = "dependency"
	PatchReviewImpactKindSemanticCoverage = "semantic_coverage"
)

type PatchReviewFinding struct {
	Code           string                     `json:"code"`
	Severity       PatchReviewSeverity        `json:"severity"`
	Category       PatchReviewFindingCategory `json:"category,omitempty"`
	ImpactKind     string                     `json:"impact_kind,omitempty"`
	Relation       string                     `json:"relation,omitempty"`
	Message        string                     `json:"message,omitempty"`
	Path           string                     `json:"path,omitempty"`
	RelatedPath    string                     `json:"related_path,omitempty"`
	SubjectSymbol  string                     `json:"subject_symbol,omitempty"`
	Strength       string                     `json:"strength,omitempty"`
	CoverageStatus PatchReviewCoverageStatus  `json:"coverage_status,omitempty"`
	EvidenceRef    string                     `json:"evidence_ref,omitempty"`
}

type PatchReviewImpactKindCoverage struct {
	Kind        string   `json:"kind"`
	Total       int      `json:"total,omitempty"`
	Verified    int      `json:"verified,omitempty"`
	Unverified  int      `json:"unverified,omitempty"`
	Unavailable int      `json:"unavailable,omitempty"`
	Unknown     int      `json:"unknown,omitempty"`
	Advisory    int      `json:"advisory,omitempty"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

// PatchReviewCoverageSummary is the compact typed root-cause coverage view for
// an actual-diff patch review. Consumers should use this instead of reparsing
// finding prose. It is telemetry/control guidance only; hard gates still decide
// from explicit booleans/enums such as HardBlock and CoverageStatus.
type PatchReviewCoverageSummary struct {
	Verdict                    PatchReviewCoverageVerdict      `json:"verdict,omitempty"`
	HardBlock                  bool                            `json:"hard_block,omitempty"`
	HasUncoveredSemantic       bool                            `json:"has_uncovered_semantic,omitempty"`
	TotalFindings              int                             `json:"total_findings,omitempty"`
	ErrorFindings              int                             `json:"error_findings,omitempty"`
	SemanticFindings           int                             `json:"semantic_findings,omitempty"`
	VerifiedSemantic           int                             `json:"verified_semantic,omitempty"`
	UnverifiedSemantic         int                             `json:"unverified_semantic,omitempty"`
	UnavailableSemantic        int                             `json:"unavailable_semantic,omitempty"`
	UnknownSemantic            int                             `json:"unknown_semantic,omitempty"`
	AdvisoryFindings           int                             `json:"advisory_findings,omitempty"`
	ReasonCodes                []string                        `json:"reason_codes,omitempty"`
	BlockReason                string                          `json:"block_reason,omitempty"`
	PrimaryUncoveredImpactKind string                          `json:"primary_uncovered_impact_kind,omitempty"`
	ImpactKindCoverage         []PatchReviewImpactKindCoverage `json:"impact_kind_coverage,omitempty"`
}

type PatchReviewRecord struct {
	ReviewID        string                      `json:"review_id,omitempty"`
	PlanID          string                      `json:"plan_id,omitempty"`
	SliceID         string                      `json:"slice_id,omitempty"`
	Source          string                      `json:"source,omitempty"`
	Status          string                      `json:"status,omitempty"`
	HardBlock       bool                        `json:"hard_block,omitempty"`
	ReasonCodes     []string                    `json:"reason_codes,omitempty"`
	PatchEffectID   string                      `json:"patch_effect_id,omitempty"`
	DiffFingerprint string                      `json:"diff_fingerprint,omitempty"`
	TargetPaths     []string                    `json:"target_paths,omitempty"`
	AllowedPaths    []string                    `json:"allowed_paths,omitempty"`
	AppliedPaths    []string                    `json:"applied_paths,omitempty"`
	Findings        []PatchReviewFinding        `json:"findings,omitempty"`
	CoverageSummary *PatchReviewCoverageSummary `json:"coverage_summary,omitempty"`
	CreatedAt       time.Time                   `json:"created_at,omitempty"`
}

func NormalizePatchReviewRecord(in PatchReviewRecord) PatchReviewRecord {
	in.ReviewID = strings.TrimSpace(in.ReviewID)
	in.PlanID = strings.TrimSpace(in.PlanID)
	in.SliceID = strings.TrimSpace(in.SliceID)
	in.Source = strings.TrimSpace(in.Source)
	in.Status = strings.TrimSpace(in.Status)
	in.PatchEffectID = strings.TrimSpace(in.PatchEffectID)
	in.DiffFingerprint = strings.TrimSpace(in.DiffFingerprint)
	in.ReasonCodes = dedupTrimWriteWorkflowRunStrings(in.ReasonCodes)
	in.TargetPaths = dedupTrimWriteWorkflowRunStrings(in.TargetPaths)
	in.AllowedPaths = dedupTrimWriteWorkflowRunStrings(in.AllowedPaths)
	in.AppliedPaths = dedupTrimWriteWorkflowRunStrings(in.AppliedPaths)
	out := make([]PatchReviewFinding, 0, len(in.Findings))
	for _, finding := range in.Findings {
		finding.Code = strings.TrimSpace(finding.Code)
		finding.Message = strings.TrimSpace(finding.Message)
		finding.Path = strings.TrimSpace(finding.Path)
		finding.RelatedPath = strings.TrimSpace(finding.RelatedPath)
		finding.SubjectSymbol = strings.TrimSpace(finding.SubjectSymbol)
		finding.Relation = strings.TrimSpace(finding.Relation)
		finding.ImpactKind = normalizePatchReviewImpactKind(finding.ImpactKind)
		finding.Strength = strings.TrimSpace(finding.Strength)
		finding.EvidenceRef = strings.TrimSpace(finding.EvidenceRef)
		finding.Category = normalizePatchReviewFindingCategory(finding.Category)
		finding.CoverageStatus = normalizePatchReviewCoverageStatus(finding.CoverageStatus)
		if finding.Severity == "" {
			finding.Severity = PatchReviewSeverityInfo
		}
		if finding.Code == "" && finding.Path == "" && finding.Message == "" && finding.SubjectSymbol == "" && finding.RelatedPath == "" {
			continue
		}
		if finding.Severity == PatchReviewSeverityError {
			in.HardBlock = true
		}
		out = append(out, finding)
	}
	in.Findings = out
	if in.Status == "" {
		if in.HardBlock {
			in.Status = "failed"
		} else {
			in.Status = "passed"
		}
	}
	summary := summarizeNormalizedPatchReviewCoverage(in)
	if summary.TotalFindings > 0 || summary.HardBlock {
		in.CoverageSummary = &summary
	} else {
		in.CoverageSummary = nil
	}
	return in
}

func SummarizePatchReviewCoverage(review PatchReviewRecord) PatchReviewCoverageSummary {
	review = NormalizePatchReviewRecord(review)
	if review.CoverageSummary == nil {
		return PatchReviewCoverageSummary{Verdict: PatchReviewCoverageVerdictClean}
	}
	return *review.CoverageSummary
}

func PatchReviewHasUncoveredSemanticCoverage(review *PatchReviewRecord) bool {
	if review == nil {
		return false
	}
	summary := SummarizePatchReviewCoverage(*review)
	return summary.HasUncoveredSemantic
}

func summarizeNormalizedPatchReviewCoverage(review PatchReviewRecord) PatchReviewCoverageSummary {
	summary := PatchReviewCoverageSummary{
		Verdict:   PatchReviewCoverageVerdictClean,
		HardBlock: review.HardBlock,
	}
	reasonSeen := map[string]bool{}
	kindSummaries := map[string]*PatchReviewImpactKindCoverage{}
	addReason := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" || reasonSeen[code] {
			return
		}
		reasonSeen[code] = true
		summary.ReasonCodes = append(summary.ReasonCodes, code)
	}
	for _, code := range review.ReasonCodes {
		addReason(code)
	}
	for _, finding := range review.Findings {
		code := strings.TrimSpace(finding.Code)
		if code != "" {
			addReason(code)
		}
		summary.TotalFindings++
		if finding.Severity == PatchReviewSeverityError {
			summary.ErrorFindings++
			if summary.BlockReason == "" {
				summary.BlockReason = "patch_review_error"
				if code != "" {
					summary.BlockReason += ":" + code
				}
			}
		}
		if finding.Category == PatchReviewCategoryConvention ||
			finding.CoverageStatus == PatchReviewCoverageAdvisory {
			summary.AdvisoryFindings++
		}
		if finding.Category != PatchReviewCategorySemanticCoverage {
			continue
		}
		summary.SemanticFindings++
		kindSummary := patchReviewImpactKindCoverageForFinding(kindSummaries, finding, code)
		switch finding.CoverageStatus {
		case PatchReviewCoverageVerified:
			summary.VerifiedSemantic++
			if kindSummary != nil {
				kindSummary.Verified++
			}
		case PatchReviewCoverageUnavailable:
			summary.UnavailableSemantic++
			summary.HasUncoveredSemantic = true
			if kindSummary != nil {
				kindSummary.Unavailable++
			}
		case PatchReviewCoverageUnknown:
			summary.UnknownSemantic++
			summary.HasUncoveredSemantic = true
			if kindSummary != nil {
				kindSummary.Unknown++
			}
		case PatchReviewCoverageUnverified:
			summary.UnverifiedSemantic++
			summary.HasUncoveredSemantic = true
			if kindSummary != nil {
				kindSummary.Unverified++
			}
		case PatchReviewCoverageAdvisory:
			if kindSummary != nil {
				kindSummary.Advisory++
			}
		}
		if summary.HasUncoveredSemantic && summary.PrimaryUncoveredImpactKind == "" && kindSummary != nil {
			summary.PrimaryUncoveredImpactKind = kindSummary.Kind
		}
		if summary.HasUncoveredSemantic && summary.BlockReason == "" {
			summary.BlockReason = "patch_review_semantic_uncovered"
			if code != "" {
				summary.BlockReason += ":" + code
			}
		}
	}
	if summary.HardBlock && summary.BlockReason == "" {
		summary.BlockReason = "patch_review_hard_block"
	}
	switch {
	case summary.HardBlock || summary.ErrorFindings > 0:
		summary.Verdict = PatchReviewCoverageVerdictFailed
	case summary.HasUncoveredSemantic:
		summary.Verdict = PatchReviewCoverageVerdictUnverified
	case summary.SemanticFindings > 0 && summary.VerifiedSemantic == summary.SemanticFindings:
		summary.Verdict = PatchReviewCoverageVerdictVerified
	case summary.AdvisoryFindings > 0 || summary.TotalFindings > 0:
		summary.Verdict = PatchReviewCoverageVerdictAdvisory
	default:
		summary.Verdict = PatchReviewCoverageVerdictClean
	}
	summary.ImpactKindCoverage = patchReviewImpactKindCoverageList(kindSummaries)
	return summary
}

func patchReviewImpactKindCoverageForFinding(byKind map[string]*PatchReviewImpactKindCoverage, finding PatchReviewFinding, code string) *PatchReviewImpactKindCoverage {
	if byKind == nil {
		return nil
	}
	kind := patchReviewImpactKindForFinding(finding)
	if kind == "" {
		return nil
	}
	entry := byKind[kind]
	if entry == nil {
		entry = &PatchReviewImpactKindCoverage{Kind: kind}
		byKind[kind] = entry
	}
	entry.Total++
	code = strings.TrimSpace(code)
	if code != "" && !patchReviewImpactKindCoverageHasReason(entry.ReasonCodes, code) {
		entry.ReasonCodes = append(entry.ReasonCodes, code)
	}
	return entry
}

func patchReviewImpactKindCoverageHasReason(reasons []string, code string) bool {
	for _, reason := range reasons {
		if reason == code {
			return true
		}
	}
	return false
}

func patchReviewImpactKindCoverageList(byKind map[string]*PatchReviewImpactKindCoverage) []PatchReviewImpactKindCoverage {
	if len(byKind) == 0 {
		return nil
	}
	out := make([]PatchReviewImpactKindCoverage, 0, len(byKind))
	for _, entry := range byKind {
		if entry == nil || entry.Kind == "" || entry.Total <= 0 {
			continue
		}
		entry.ReasonCodes = dedupTrimWriteWorkflowRunStrings(entry.ReasonCodes)
		out = append(out, *entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := patchReviewImpactKindRank(out[i].Kind), patchReviewImpactKindRank(out[j].Kind)
		if ri != rj {
			return ri < rj
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func patchReviewImpactKindForFinding(finding PatchReviewFinding) string {
	if kind := normalizePatchReviewImpactKind(finding.ImpactKind); kind != "" {
		return kind
	}
	switch strings.TrimSpace(finding.Code) {
	case "behavior_contract_without_verify_coverage":
		return PatchReviewImpactKindBehaviorContract
	case "changed_symbol_without_probe_coverage":
		return PatchReviewImpactKindChangedSymbol
	case "dependent_surface_without_verify_coverage":
		return PatchReviewImpactKindDependent
	case "related_test_surface_unverified":
		return PatchReviewImpactKindTestSurface
	case "go_nested_string_map_assignment_added",
		"java_chained_string_map_get_added",
		"javascript_nested_string_key_direct_access_added",
		"kotlin_chained_string_map_get_added",
		"production_test_scaffold_added",
		"python_nested_string_key_direct_access_added",
		"ruby_nested_key_direct_access_added",
		"typescript_nested_string_key_direct_access_added":
		return PatchReviewImpactKindEffectFollowup
	default:
		return ""
	}
}

func normalizePatchReviewImpactKind(in string) string {
	switch strings.TrimSpace(in) {
	case PatchReviewImpactKindBehaviorContract,
		PatchReviewImpactKindChangedSymbol,
		PatchReviewImpactKindDependent,
		PatchReviewImpactKindTestSurface,
		PatchReviewImpactKindEffectFollowup,
		PatchReviewImpactKindChangedFile,
		PatchReviewImpactKindDependency,
		PatchReviewImpactKindSemanticCoverage:
		return strings.TrimSpace(in)
	default:
		return ""
	}
}

func patchReviewImpactKindRank(kind string) int {
	switch strings.TrimSpace(kind) {
	case PatchReviewImpactKindBehaviorContract:
		return 10
	case PatchReviewImpactKindChangedSymbol:
		return 20
	case PatchReviewImpactKindDependent:
		return 30
	case PatchReviewImpactKindTestSurface:
		return 40
	case PatchReviewImpactKindEffectFollowup:
		return 50
	case PatchReviewImpactKindChangedFile:
		return 60
	case PatchReviewImpactKindDependency:
		return 70
	case PatchReviewImpactKindSemanticCoverage:
		return 80
	default:
		return 90
	}
}

func normalizePatchReviewFindingCategory(in PatchReviewFindingCategory) PatchReviewFindingCategory {
	switch in {
	case PatchReviewCategoryScope,
		PatchReviewCategoryStructural,
		PatchReviewCategorySemanticCoverage,
		PatchReviewCategoryConvention:
		return in
	default:
		return ""
	}
}

func normalizePatchReviewCoverageStatus(in PatchReviewCoverageStatus) PatchReviewCoverageStatus {
	switch in {
	case PatchReviewCoverageUnknown,
		PatchReviewCoverageVerified,
		PatchReviewCoverageUnverified,
		PatchReviewCoverageUnavailable,
		PatchReviewCoverageAdvisory:
		return in
	default:
		return ""
	}
}
