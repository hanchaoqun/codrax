package types

import (
	"path/filepath"
	"strings"
	"time"
)

const (
	maxHandoffFailingTests = 10
	maxHandoffBuildErrors  = 10
	maxHandoffCommands     = 6
	maxHandoffDiagnostics  = 8
	maxHandoffConfidence   = 8
	maxHandoffSignals      = 10
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

	// FailureReasonCode refines FailureKind for planner/controller consumers
	// without requiring them to parse FailureSummary prose.
	FailureReasonCode string `json:"failure_reason_code,omitempty"`

	// Executed are the verification command rows from the failing report
	// (bounded), with cwd / exit code / provenance.
	Executed []ExecutedCommand `json:"executed,omitempty"`

	// Diagnostics are bounded typed verify diagnostics projected from command
	// and probe outcomes. They preserve secondary causes such as probe
	// authoring errors or environment startup failures without forcing
	// consumers to parse FailureSummary.
	Diagnostics []VerificationDiagnostic `json:"diagnostics,omitempty"`

	// Confidence carries bounded typed confidence records such as weak probe
	// coverage, missing changed-symbol coupling, or unavailable local runner
	// evidence. It is separate from pass/fail so replan can improve evidence
	// quality without treating environment gaps as product-code failures.
	Confidence []VerificationConfidenceRecord `json:"confidence,omitempty"`

	// FailingTests are the failed assertion rows (bounded).
	FailingTests []TestResult `json:"failing_tests,omitempty"`

	// FailureSignals are compact, typed projections of failed assertions for
	// the next planning round. They are derived from TestResult fields and
	// runner failure detail; consumers use them as soft repair guidance instead
	// of parsing long traceback prose.
	FailureSignals []TestFailureSignal `json:"failure_signals,omitempty"`

	// BuildErrors are the parsed compile rows (bounded).
	BuildErrors []BuildError `json:"build_errors,omitempty"`

	// BlobRef pages the full runner output via read_file offset/limit.
	BlobRef string `json:"blob_ref,omitempty"`

	// DiffArtifactRef names the persisted attempt patch
	// (<stem>.attempt-N.diff) in the plan directory.
	DiffArtifactRef string `json:"diff_artifact_ref,omitempty"`

	// DiffArtifactPath is the resolved local path to DiffArtifactRef when the
	// orchestrator knows the artifact directory. It is a read-only handoff
	// convenience for planner/verifier consumers; durable workflow state still
	// stores the portable ref above.
	DiffArtifactPath string `json:"diff_artifact_path,omitempty"`

	// SurfaceArtifactRef names the persisted runnable test surface
	// (<stem>.attempt-N.surface.json) in the plan directory.
	SurfaceArtifactRef string `json:"surface_artifact_ref,omitempty"`

	// SurfaceArtifactPath is the resolved local path to SurfaceArtifactRef when
	// the orchestrator knows the artifact directory.
	SurfaceArtifactPath string `json:"surface_artifact_path,omitempty"`

	// NextSurfaceCandidateID names the highest-ranked unexecuted test
	// surface candidate with real test work, when one exists — the typed
	// "what verification to aim at next" suggestion.
	NextSurfaceCandidateID string `json:"next_surface_candidate_id,omitempty"`

	GeneratedAt time.Time `json:"generated_at"`
}

// BuildVerifyFailureHandoff projects a failed post-apply ChangeReport into the
// typed replan carrier. Returns nil for nil/passed reports.
func BuildVerifyFailureHandoff(report *ChangeReport, batchID string, attempt int, diffArtifactRef, surfaceArtifactRef string) *VerifyFailureHandoff {
	if report == nil || report.Passed {
		return nil
	}
	h := &VerifyFailureHandoff{
		PlanID:             strings.TrimSpace(report.PlanID),
		BatchID:            strings.TrimSpace(batchID),
		Attempt:            attempt,
		FailureKind:        report.FailureKind,
		FailureSummary:     strings.TrimSpace(report.FailureSummary),
		FailureReasonCode:  strings.TrimSpace(report.FailureReasonCode),
		BlobRef:            strings.TrimSpace(report.FailureSummaryBlobRef),
		DiffArtifactRef:    strings.TrimSpace(diffArtifactRef),
		SurfaceArtifactRef: strings.TrimSpace(surfaceArtifactRef),
		GeneratedAt:        time.Now(),
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
	for _, diag := range report.VerificationDiagnostics {
		if len(h.Diagnostics) >= maxHandoffDiagnostics {
			break
		}
		h.Diagnostics = append(h.Diagnostics, diag)
	}
	for _, confidence := range report.VerificationConfidence {
		if len(h.Confidence) >= maxHandoffConfidence {
			break
		}
		h.Confidence = append(h.Confidence, confidence)
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
		if len(h.FailureSignals) < maxHandoffSignals {
			if signal := ExtractTestFailureSignal(tr, defaultFailureSignalMaxChars); renderTestFailureSignal(signal) != "" {
				h.FailureSignals = append(h.FailureSignals, signal)
			}
		}
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

// ResolveVerifyFailureHandoffArtifactPaths annotates a handoff with concrete
// local paths for its portable artifact refs. It does not affect hard routing:
// refs remain the durable identity, while paths are a model-facing convenience
// so a retry planner can call read_file on the exact failed-attempt evidence
// instead of guessing where the plan directory lives.
func ResolveVerifyFailureHandoffArtifactPaths(h *VerifyFailureHandoff, artifactDir string) {
	if h == nil {
		return
	}
	artifactDir = strings.TrimSpace(artifactDir)
	if artifactDir == "" {
		return
	}
	if p := resolveHandoffArtifactPath(artifactDir, h.DiffArtifactRef); p != "" {
		h.DiffArtifactPath = p
	}
	if p := resolveHandoffArtifactPath(artifactDir, h.SurfaceArtifactRef); p != "" {
		h.SurfaceArtifactPath = p
	}
}

func resolveHandoffArtifactPath(artifactDir, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	cleanRef := filepath.Clean(ref)
	if cleanRef == "." {
		return ""
	}
	if filepath.IsAbs(cleanRef) {
		return cleanRef
	}
	slash := filepath.ToSlash(cleanRef)
	if slash == ".." || strings.HasPrefix(slash, "../") {
		return ""
	}
	dir := filepath.Clean(artifactDir)
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	return filepath.Join(dir, cleanRef)
}
