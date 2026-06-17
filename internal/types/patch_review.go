package types

import (
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

type PatchReviewFinding struct {
	Code           string                     `json:"code"`
	Severity       PatchReviewSeverity        `json:"severity"`
	Category       PatchReviewFindingCategory `json:"category,omitempty"`
	Relation       string                     `json:"relation,omitempty"`
	Message        string                     `json:"message,omitempty"`
	Path           string                     `json:"path,omitempty"`
	RelatedPath    string                     `json:"related_path,omitempty"`
	SubjectSymbol  string                     `json:"subject_symbol,omitempty"`
	Strength       string                     `json:"strength,omitempty"`
	CoverageStatus PatchReviewCoverageStatus  `json:"coverage_status,omitempty"`
	EvidenceRef    string                     `json:"evidence_ref,omitempty"`
}

// PatchReviewCoverageSummary is the compact typed root-cause coverage view for
// an actual-diff patch review. Consumers should use this instead of reparsing
// finding prose. It is telemetry/control guidance only; hard gates still decide
// from explicit booleans/enums such as HardBlock and CoverageStatus.
type PatchReviewCoverageSummary struct {
	Verdict              PatchReviewCoverageVerdict `json:"verdict,omitempty"`
	HardBlock            bool                       `json:"hard_block,omitempty"`
	HasUncoveredSemantic bool                       `json:"has_uncovered_semantic,omitempty"`
	TotalFindings        int                        `json:"total_findings,omitempty"`
	ErrorFindings        int                        `json:"error_findings,omitempty"`
	SemanticFindings     int                        `json:"semantic_findings,omitempty"`
	VerifiedSemantic     int                        `json:"verified_semantic,omitempty"`
	UnverifiedSemantic   int                        `json:"unverified_semantic,omitempty"`
	UnavailableSemantic  int                        `json:"unavailable_semantic,omitempty"`
	UnknownSemantic      int                        `json:"unknown_semantic,omitempty"`
	AdvisoryFindings     int                        `json:"advisory_findings,omitempty"`
	ReasonCodes          []string                   `json:"reason_codes,omitempty"`
	BlockReason          string                     `json:"block_reason,omitempty"`
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
		switch finding.CoverageStatus {
		case PatchReviewCoverageVerified:
			summary.VerifiedSemantic++
		case PatchReviewCoverageUnavailable:
			summary.UnavailableSemantic++
			summary.HasUncoveredSemantic = true
		case PatchReviewCoverageUnknown:
			summary.UnknownSemantic++
			summary.HasUncoveredSemantic = true
		case PatchReviewCoverageUnverified:
			summary.UnverifiedSemantic++
			summary.HasUncoveredSemantic = true
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
	return summary
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
