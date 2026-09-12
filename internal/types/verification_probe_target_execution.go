package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const VerificationProbeTargetExecutionReceiptVersion = 1

// VerificationProbeTargetExecutionReceipt is produced by the Python executor,
// nested in one terminal invocation. Complete describes observation integrity,
// not behavior correctness or execution of every mapped owner. It is not a
// planner-authored field and does not participate in probe definition identity.
type VerificationProbeTargetExecutionReceipt struct {
	Version                 int                                      `json:"version"`
	PlanID                  string                                   `json:"plan_id"`
	ProbeID                 string                                   `json:"probe_id"`
	ExecutionRoot           string                                   `json:"execution_root"`
	ManifestSHA256          string                                   `json:"manifest_sha256"`
	InvocationBindingSHA256 string                                   `json:"invocation_binding_sha256"`
	PatchEffectID           string                                   `json:"patch_effect_id"`
	DiffFingerprint         string                                   `json:"diff_fingerprint"`
	HeadRef                 string                                   `json:"head_ref"`
	SourceCommitSHA         string                                   `json:"source_commit_sha"`
	Status                  string                                   `json:"status"`
	ReasonCode              string                                   `json:"reason_code,omitempty"`
	Targets                 []VerificationProbeTargetExecutionTarget `json:"targets,omitempty"`
}

type VerificationProbeTargetExecutionTarget struct {
	Path            string                                  `json:"path"`
	SourceSHA256    string                                  `json:"source_sha256"`
	MappingComplete bool                                    `json:"mapping_complete"`
	Owners          []VerificationProbeTargetExecutionOwner `json:"owners,omitempty"`
}

type VerificationProbeTargetExecutionOwner struct {
	CodeSHA256    string `json:"code_sha256"`
	Kind          string `json:"kind"`
	FirstLine     int    `json:"first_line"`
	LastLine      int    `json:"last_line"`
	ChangedLines  []int  `json:"changed_lines,omitempty"`
	ExecutedLines []int  `json:"executed_lines,omitempty"`
}

type VerificationProbeTargetExecutionResolution struct {
	Applies    bool
	Paths      []string
	ReasonCode string
}

// VerificationProbeLanguageIsPython shares the existing executor's closed
// Python spelling/default domain. Unknown language names are not aliases.
func VerificationProbeLanguageIsPython(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "py", "python":
		return true
	default:
		return false
	}
}

// VerificationProbeTargetManifestSHA256 binds the static mapping, preserving
// ordering and case. Runtime observations are deliberately excluded so that
// adding an observed line cannot silently replace the mapping being observed.
func VerificationProbeTargetManifestSHA256(receipt *VerificationProbeTargetExecutionReceipt) string {
	if receipt == nil {
		return ""
	}
	manifest := *receipt
	manifest.ManifestSHA256, manifest.Status, manifest.ReasonCode = "", "", ""
	manifest.Targets = append([]VerificationProbeTargetExecutionTarget(nil), receipt.Targets...)
	for i := range manifest.Targets {
		manifest.Targets[i].Owners = append([]VerificationProbeTargetExecutionOwner(nil), receipt.Targets[i].Owners...)
		for j := range manifest.Targets[i].Owners {
			manifest.Targets[i].Owners[j].ExecutedLines = nil
		}
	}
	data, _ := json.Marshal(manifest)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerificationProbeTargetInvocationSHA256 binds the nested observation to the
// exact outer terminal receipt, including both its repository identity domain
// and actual working directory. Those roots need not be equal for a worktree.
// This instance binding does not change B1616's stable rerun identity.
func VerificationProbeTargetInvocationSHA256(receipt *VerificationProbeExecutionReceipt) string {
	if verificationProbeExecutionIdentity(receipt) == "" {
		return ""
	}
	outer := *receipt
	outer.TargetExecution = nil
	data, _ := json.Marshal(outer)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ResolveVerificationProbeTargetExecution is the shared authority boundary for
// Python plain probes. A missing/legacy receipt is unknown, never a fallback to
// target_behavior. With no plan it validates report-local provenance only;
// callers transferring proof to a plan must provide that plan.
func ResolveVerificationProbeTargetExecution(plan *ChangePlan, probe VerificationProbe, report *ChangeReport) VerificationProbeTargetExecutionResolution {
	out := VerificationProbeTargetExecutionResolution{Applies: VerificationProbeLanguageIsPython(probe.Language)}
	if !out.Applies {
		return out
	}
	out.ReasonCode = "python_target_execution_unobserved"
	if report == nil || report.PlanID == "" || probe.ID == "" {
		return out
	}
	if plan != nil && (plan.ID != report.PlanID || plan.PatchEffect == nil) {
		return out
	}
	if plan != nil {
		declared := false
		for _, candidate := range ChangePlanVerificationProbes(plan) {
			if candidate.ID == probe.ID && reflect.DeepEqual(candidate, probe) {
				declared = true
			}
		}
		if !declared {
			return out
		}
	}
	passed := false
	for _, result := range report.TestResults {
		if result.Suite == "verification_probe/python" && result.AssertionID == probe.ID {
			if !result.Passed {
				return out
			}
			passed = true
		}
	}
	if !passed {
		return out
	}
	definition := ""
	if plan != nil || probe.Code != "" {
		data, _ := json.Marshal(probe)
		sum := sha256.Sum256(data)
		definition = hex.EncodeToString(sum[:])
	}
	var matched *VerificationProbeTargetExecutionReceipt
	for _, command := range report.ExecutedCommands {
		// The producer projects main-snapshot runs into these three closed
		// outcomes. They are observations of a different tree, not competing
		// current executions, and cannot grant current target capability.
		if verificationProbeTargetBaselineCommand(command) {
			continue
		}
		outer := command.ProbeExecution
		if command.Runner != "verification_probe" || command.Framework != "python" || outer == nil {
			continue
		}
		receipt := outer.TargetExecution
		if receipt == nil || receipt.ProbeID != probe.ID {
			continue
		}
		if matched != nil {
			return out
		} // ambiguous current invocation, not pool-first
		if command.Outcome != ExecutedCommandOutcomeExecuted || command.ExitCode != 0 || verificationProbeExecutionIdentity(outer) == "" ||
			receipt.InvocationBindingSHA256 == "" || receipt.InvocationBindingSHA256 != VerificationProbeTargetInvocationSHA256(outer) ||
			(definition != "" && definition != outer.DefinitionSHA256) || receipt.Version != VerificationProbeTargetExecutionReceiptVersion ||
			receipt.Status != "complete" || receipt.PlanID != report.PlanID || receipt.ExecutionRoot == "" || !filepath.IsAbs(receipt.ExecutionRoot) ||
			receipt.PatchEffectID == "" || receipt.DiffFingerprint == "" || receipt.HeadRef == "" || !probeTargetCommit(receipt.SourceCommitSHA) ||
			receipt.ManifestSHA256 != VerificationProbeTargetManifestSHA256(receipt) {
			return out
		}
		relativeWorkingDir, err := filepath.Rel(receipt.ExecutionRoot, outer.WorkingDir)
		if err != nil || relativeWorkingDir == ".." || strings.HasPrefix(relativeWorkingDir, ".."+string(filepath.Separator)) {
			return out
		}
		if plan != nil {
			effect := plan.PatchEffect
			if effect.PlanID != plan.ID || receipt.PatchEffectID != effect.RecordID || receipt.DiffFingerprint != effect.DiffFingerprint || receipt.HeadRef != effect.HeadRef ||
				(probeTargetCommit(effect.HeadRef) && receipt.SourceCommitSHA != effect.HeadRef) ||
				(plan.WorktreePath != "" && filepath.Clean(plan.WorktreePath) != filepath.Clean(receipt.ExecutionRoot)) {
				return out
			}
		}
		matched = receipt
	}
	if matched == nil {
		return out
	}
	paths := map[string]bool{}
	for _, target := range matched.Targets {
		path := target.Path
		if paths[path] {
			out.Paths = nil
			return out
		}
		paths[path] = true
		if !probeTargetRelativePath(path) || !target.MappingComplete || !probeTargetDigest(target.SourceSHA256) || len(target.Owners) == 0 {
			continue
		}
		complete, allExecuted := true, true
		changed, owners := map[int]bool{}, map[string]bool{}
		for _, owner := range target.Owners {
			if !probeTargetDigest(owner.CodeSHA256) || owners[owner.CodeSHA256] || owner.FirstLine <= 0 || owner.LastLine < owner.FirstLine || len(owner.ChangedLines) == 0 {
				complete = false
			}
			owners[owner.CodeSHA256] = true
			switch owner.Kind {
			case "module", "class", "function", "async_function":
			default:
				complete = false
			}
			for _, line := range owner.ChangedLines {
				if line < owner.FirstLine || line > owner.LastLine || changed[line] {
					complete = false
				}
				changed[line] = true
			}
			if len(owner.ExecutedLines) == 0 {
				allExecuted = false
			}
			for _, line := range owner.ExecutedLines {
				if line < owner.FirstLine || line > owner.LastLine {
					complete = false
				}
			}
		}
		if plan != nil && !probeTargetMatchesAddedLines(plan.PatchEffect, path, changed) {
			complete = false
		}
		if complete && allExecuted {
			out.Paths = append(out.Paths, path)
		}
	}
	sort.Strings(out.Paths)
	if len(out.Paths) > 0 {
		out.ReasonCode = "python_changed_owners_executed"
	}
	return out
}

func probeTargetDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func verificationProbeTargetBaselineCommand(command ExecutedCommand) bool {
	if command.Source != "verification_probe_main_snapshot_baseline" {
		return false
	}
	switch command.Outcome {
	case ExecutedCommandOutcomeExpectedFailureObserved, ExecutedCommandOutcomeExpectedFailureNotObserved, ExecutedCommandOutcomeBaselineUnavailable:
		return true
	case "", ExecutedCommandOutcomeExecuted, ExecutedCommandOutcomeSyntheticNoTests, ExecutedCommandOutcomeSyntaxCheckFallback,
		ExecutedCommandOutcomeSyntaxPreflight, ExecutedCommandOutcomeSuiteContinued, ExecutedCommandOutcomeTimeout,
		ExecutedCommandOutcomeOOM, ExecutedCommandOutcomeCPULimit, ExecutedCommandOutcomeZeroTests,
		ExecutedCommandOutcomeProbeConfigError, ExecutedCommandOutcomeExpectedStdoutMissing,
		ExecutedCommandOutcomeFailed, ExecutedCommandOutcomeRunnerMissing, ExecutedCommandOutcomeParserError,
		ExecutedCommandOutcomeNotConfigured, ExecutedCommandOutcomeSuiteSkipped:
		return false
	default:
		return false
	}
}

func probeTargetCommit(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (len(decoded) == 20 || len(decoded) == 32)
}

func probeTargetRelativePath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.ToSlash(filepath.Clean(path)) == path && path != "." && path != ".." && !strings.HasPrefix(path, "../")
}

func probeTargetMatchesAddedLines(effect *PatchEffectRecord, path string, got map[int]bool) bool {
	var found *PatchEffectFile
	for i := range effect.Files {
		if effect.Files[i].Path == path {
			if found != nil {
				return false
			}
			found = &effect.Files[i]
		}
	}
	if found == nil || found.Binary || len(found.Hunks) == 0 {
		return false
	}
	want := map[int]bool{}
	for _, hunk := range found.Hunks {
		// Pure deletion has no exact post-edit owner in this protocol version.
		if hunk.RemovedLines > 0 && len(hunk.AddedLineNumbers) == 0 {
			return false
		}
		if hunk.AddedLines != len(hunk.AddedLineNumbers) {
			return false
		}
		if len(hunk.AddedLineTexts) != len(hunk.AddedLineNumbers) {
			return false
		}
		texts := map[int]bool{}
		for _, line := range hunk.AddedLineTexts {
			if line.Line <= 0 || texts[line.Line] {
				return false
			}
			texts[line.Line] = true
		}
		for _, line := range hunk.AddedLineNumbers {
			if line <= 0 || want[line] || !texts[line] {
				return false
			}
			want[line] = true
		}
	}
	if len(want) == 0 || len(want) != len(got) {
		return false
	}
	for line := range want {
		if !got[line] {
			return false
		}
	}
	return true
}
