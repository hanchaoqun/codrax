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

type PatchReviewRecord struct {
	ReviewID        string               `json:"review_id,omitempty"`
	PlanID          string               `json:"plan_id,omitempty"`
	SliceID         string               `json:"slice_id,omitempty"`
	Source          string               `json:"source,omitempty"`
	Status          string               `json:"status,omitempty"`
	HardBlock       bool                 `json:"hard_block,omitempty"`
	ReasonCodes     []string             `json:"reason_codes,omitempty"`
	PatchEffectID   string               `json:"patch_effect_id,omitempty"`
	DiffFingerprint string               `json:"diff_fingerprint,omitempty"`
	TargetPaths     []string             `json:"target_paths,omitempty"`
	AllowedPaths    []string             `json:"allowed_paths,omitempty"`
	AppliedPaths    []string             `json:"applied_paths,omitempty"`
	Findings        []PatchReviewFinding `json:"findings,omitempty"`
	CreatedAt       time.Time            `json:"created_at,omitempty"`
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
	return in
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
