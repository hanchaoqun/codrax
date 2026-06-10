package types

import (
	"strings"
	"time"
)

const (
	maxHandoffFailingTests = 10
	maxHandoffBuildErrors  = 10
	maxHandoffCommands     = 6
)

// VerifyFailureHandoff is the typed carrier that brings a failed post-apply
// verification to the next planning round. It survives the controller's
// per-round planning-state reset on purpose: replan must open with the
// failure-local evidence (failing rows, executed commands, artifact refs)
// instead of re-deriving the problem from prose or re-exploring from scratch.
// All fields are projected from the typed ChangeReport — never from
// narrative text.
type VerifyFailureHandoff struct {
	PlanID  string `json:"plan_id,omitempty"`
	BatchID string `json:"batch_id,omitempty"`
	// Attempt is the 1-based failed post-apply verify ordinal for the batch.
	Attempt int `json:"attempt,omitempty"`

	FailureKind    FailureKind `json:"failure_kind,omitempty"`
	FailureSummary string      `json:"failure_summary,omitempty"`

	// Executed are the verification command rows from the failing report
	// (bounded), with cwd / exit code / provenance.
	Executed []ExecutedCommand `json:"executed,omitempty"`

	// FailingTests are the failed assertion rows (bounded).
	FailingTests []TestResult `json:"failing_tests,omitempty"`

	// BuildErrors are the parsed compile rows (bounded).
	BuildErrors []BuildError `json:"build_errors,omitempty"`

	// BlobRef pages the full runner output via read_file offset/limit.
	BlobRef string `json:"blob_ref,omitempty"`

	// DiffArtifactRef names the persisted attempt patch
	// (<stem>.attempt-N.diff) in the plan directory.
	DiffArtifactRef string `json:"diff_artifact_ref,omitempty"`

	// NextSurfaceCandidateID names the highest-ranked unexecuted test
	// surface candidate with real test work, when one exists — the typed
	// "what verification to aim at next" suggestion.
	NextSurfaceCandidateID string `json:"next_surface_candidate_id,omitempty"`

	GeneratedAt time.Time `json:"generated_at"`
}

// BuildVerifyFailureHandoff projects a failed post-apply ChangeReport into
// the typed replan carrier. Returns nil for nil/passed reports.
func BuildVerifyFailureHandoff(report *ChangeReport, batchID string, attempt int, diffArtifactRef string) *VerifyFailureHandoff {
	if report == nil || report.Passed {
		return nil
	}
	h := &VerifyFailureHandoff{
		PlanID:          strings.TrimSpace(report.PlanID),
		BatchID:         strings.TrimSpace(batchID),
		Attempt:         attempt,
		FailureKind:     report.FailureKind,
		FailureSummary:  strings.TrimSpace(report.FailureSummary),
		BlobRef:         strings.TrimSpace(report.FailureSummaryBlobRef),
		DiffArtifactRef: strings.TrimSpace(diffArtifactRef),
		GeneratedAt:     time.Now(),
	}
	if h.Attempt < 1 {
		h.Attempt = 1
	}
	for _, cmd := range report.ExecutedCommands {
		if len(h.Executed) >= maxHandoffCommands {
			break
		}
		h.Executed = append(h.Executed, cmd)
	}
	for _, tr := range report.TestResults {
		if tr.Passed {
			continue
		}
		if tr.Kind == TestResultKindBuildError {
			for _, be := range tr.BuildErrors {
				if len(h.BuildErrors) >= maxHandoffBuildErrors {
					break
				}
				h.BuildErrors = append(h.BuildErrors, be)
			}
			continue
		}
		if len(h.FailingTests) >= maxHandoffFailingTests {
			continue
		}
		h.FailingTests = append(h.FailingTests, tr)
	}
	if report.TestSurface != nil {
		executed := map[string]bool{}
		for _, cmd := range report.ExecutedCommands {
			wd := strings.TrimSpace(cmd.WorkingDir)
			if wd == "" {
				wd = "."
			}
			executed[cmd.Runner+"@"+wd] = true
		}
		for _, cand := range report.TestSurface.Candidates {
			if !cand.HasTestSignal {
				continue
			}
			wd := strings.TrimSpace(cand.WorkingDir)
			if wd == "" {
				wd = "."
			}
			if executed[cand.Runner+"@"+wd] {
				continue
			}
			h.NextSurfaceCandidateID = cand.ID
			break
		}
	}
	return h
}
