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

type PatchReviewFinding struct {
	Code        string              `json:"code"`
	Severity    PatchReviewSeverity `json:"severity"`
	Message     string              `json:"message,omitempty"`
	Path        string              `json:"path,omitempty"`
	EvidenceRef string              `json:"evidence_ref,omitempty"`
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
		finding.EvidenceRef = strings.TrimSpace(finding.EvidenceRef)
		if finding.Severity == "" {
			finding.Severity = PatchReviewSeverityInfo
		}
		if finding.Code == "" && finding.Path == "" && finding.Message == "" {
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
