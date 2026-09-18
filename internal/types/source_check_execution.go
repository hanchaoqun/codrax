package types

import "strings"

const SourceCheckExecutionReceiptVersion = 1

// SourceCheckExecutionReceipt records one producer-owned source-check process.
// A completed successful process with unknown input scope is still an honest
// execution observation, but an empty CheckedPaths grants no source-path proof.
// Paths are canonical, case-sensitive, repository-relative paths, not inferred
// from command text, diagnostics, or a language-wide directory scope.
type SourceCheckExecutionReceipt struct {
	Version       int      `json:"version"`
	Started       bool     `json:"started"`
	Completed     bool     `json:"completed"`
	ExitCodeKnown bool     `json:"exit_code_known"`
	ExitCode      int      `json:"exit_code"`
	CheckedPaths  []string `json:"checked_paths,omitempty"`
}

// SourceCheckCommandSucceeded grants syntax-only authority for the exact
// recorded paths. A legacy zero exit, skipped tool, unstarted command, or an
// ordinary project/probe command cannot borrow this source-check authority.
// Qualification is local to this command: a later sibling failure/cancellation
// does not erase a successfully completed check, or become green because of it.
func SourceCheckCommandSucceeded(cmd ExecutedCommand) bool {
	if !sourceCheckCommandExecutionCompleted(cmd) || cmd.ExitCode != 0 {
		return false
	}
	r := cmd.SourceCheckExecution
	checked, ok := sourceCheckPathSet(r.CheckedPaths)
	if !ok {
		return false
	}
	covered, ok := sourceCheckPathSet(cmd.CoveredPaths)
	if !ok || len(checked) != len(covered) {
		return false
	}
	for path := range checked {
		if !covered[path] {
			return false
		}
	}
	return true
}

// Execution classification is weaker than successful exact-path authority:
// a real failed check or a completed project check with unknown inputs remains
// a source-check attempt, never an ordinary project-suite execution.
func sourceCheckCommandExecutionCompleted(cmd ExecutedCommand) bool {
	if cmd.Outcome != ExecutedCommandOutcomeSyntaxCheckFallback && cmd.Outcome != ExecutedCommandOutcomeSyntaxPreflight {
		return false
	}
	r := cmd.SourceCheckExecution
	return r != nil && r.Version == SourceCheckExecutionReceiptVersion && r.Started && r.Completed && r.ExitCodeKnown && r.ExitCode == cmd.ExitCode
}

func sourceCheckPathSet(paths []string) (map[string]bool, bool) {
	if len(paths) == 0 {
		return nil, false
	}
	out := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !probeTargetRelativePath(path) || strings.ContainsAny(path, "\\\x00") || (len(path) >= 2 && path[1] == ':') {
			return nil, false
		}
		out[path] = true
	}
	return out, true
}

// effectiveSourceCheckCoverage only revalidates source-check-owned rows and
// legacy syntax-only rows with no caliber. Independent project/probe authority
// remains with its own resolver, even when its capability is only syntax.
func effectiveSourceCheckCoverage(plan *ChangePlan, report *ChangeReport, row ChangedPathVerificationCoverage) ChangedPathVerificationCoverage {
	if row.Status != ChangedPathVerificationCovered ||
		(row.Caliber != ChangedPathVerificationSourceCheck && (row.Caliber != "" || row.Capability != VerificationCapabilitySyntaxOnly)) {
		return row
	}
	if plan == nil || plan.ID == report.PlanID {
		for _, cmd := range report.ExecutedCommands {
			if cmd.Runner != row.Runner || cmd.Source != row.Source || !SourceCheckCommandSucceeded(cmd) {
				continue
			}
			for _, path := range cmd.CoveredPaths {
				if path == row.Path {
					row.Capability = VerificationCapabilitySyntaxOnly
					return row
				}
			}
		}
	}
	row.Status, row.Capability = ChangedPathVerificationUncovered, VerificationCapabilityUnknown
	row.ReasonCode = "source_check_execution_unobserved"
	return row
}

func effectiveSourceCheckConfidence(plan *ChangePlan, report *ChangeReport, rec VerificationConfidenceRecord) VerificationConfidenceRecord {
	if rec.Category != "source_compile" || rec.Status != "satisfied" {
		return rec
	}
	paths := map[string]bool{}
	if plan == nil || plan.ID == report.PlanID {
		for _, cmd := range report.ExecutedCommands {
			source := strings.TrimSpace(cmd.Source)
			if source == "" {
				source = "syntax_preflight"
			}
			if source != rec.Source || !SourceCheckCommandSucceeded(cmd) {
				continue
			}
			for _, path := range cmd.CoveredPaths {
				paths[path] = true
			}
		}
	}
	bound := len(paths) > 0
	for _, ref := range rec.ChangedSymbolRefs {
		if !strings.HasPrefix(ref, "path:") || !paths[strings.TrimPrefix(ref, "path:")] {
			bound = false
		}
	}
	if bound {
		if len(rec.ChangedSymbolRefs) == 0 {
			// A historical generic claim may survive as a real observation,
			// not as proof that the entire plan was checked.
			rec.Detail = "source-check succeeded for the exact paths recorded in matching execution receipts"
		}
		return rec
	}
	rec.Status, rec.Severity, rec.ReasonCode = "unverified", "warning", "source_check_execution_unobserved"
	rec.Detail = "stored source-check claim has no matching successful exact-path execution receipt"
	return rec
}
