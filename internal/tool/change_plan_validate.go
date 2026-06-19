package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// change_plan_validate.go bundles the change-plan validators and
// constructors so BOTH the single-shot emit_change_plan path AND the
// structural emit_plan_skeleton + emit_plan_change path can run them
// against the same canonical types.FileChange shape. This file is the
// load-bearing seam that prevents the two emit paths from drifting:
// every validator below is the SOLE producer of its rejection text;
// every caller routes through these helpers, never re-implements the
// rule in-line. If a validation contract changes, it changes here,
// and both emit paths inherit the new behaviour for free.

// emitChangesToFileChanges normalizes the wire-shape coming out of
// emit_change_plan's JSON decoder into the canonical types.FileChange
// shape used by every validator below. Trims whitespace on every
// string field; copies DependsOn into a fresh slice (so later
// in-place edits cannot leak into the JSON-decoded struct).
func emitChangesToFileChanges(changes []emitChangePlanChange) []types.FileChange {
	out := make([]types.FileChange, 0, len(changes))
	for _, c := range changes {
		var deps []string
		if len(c.DependsOn) > 0 {
			deps = make([]string, 0, len(c.DependsOn))
			for _, d := range c.DependsOn {
				deps = append(deps, strings.TrimSpace(d))
			}
		}
		out = append(out, types.FileChange{
			Path:       strings.TrimSpace(c.Path),
			Kind:       strings.TrimSpace(c.Kind),
			NewContent: c.NewContent,
			Patch:      c.Patch,
			Edits:      append([]types.StructuredEdit(nil), c.Edits...),
			NewPath:    strings.TrimSpace(c.NewPath),
			Rationale:  strings.TrimSpace(c.Rationale),
			DependsOn:  deps,
		})
	}
	return out
}

// validatePlanGraphIntegrity bundles the four content-independent
// structural checks: every change has a path; every kind is legal;
// no duplicate paths; no dangling / self / empty depends_on. Cycle
// detection is its own pass (detectDepsCycle) because the caller
// wants the cycle path string in the error message.
//
// Returns "" on success or a short rejection reason. Callers prefix
// the tool name + "rejected: " before surfacing.
//
// Content-independent: this can run on the skeleton path BEFORE any
// new_content arrives, so emit_plan_skeleton catches structural
// problems early without waiting for per-file emits.
func validatePlanGraphIntegrity(changes []types.FileChange) string {
	rej, _ := validatePlanGraphIntegrityWithRepair("", changes)
	return rej
}

func validatePlanGraphIntegrityWithRepair(toolName string, changes []types.FileChange) (string, *types.PlanRepairPack) {
	seenPaths := make(map[string]int, len(changes))
	for i, c := range changes {
		path := strings.TrimSpace(c.Path)
		if path == "" {
			rej := "one of the changes has an empty path"
			return rej, planRepairPackFromReason(toolName, "change_path_empty", rej, []string{"$.changes[].path"}, nil)
		}
		if !isLegalChangeKind(strings.TrimSpace(c.Kind)) {
			rej := fmt.Sprintf("change %q has illegal kind %q (must be create|modify|delete|patch|rename)", path, c.Kind)
			return rej, planRepairPackWithEnums(toolName, "change_kind_invalid", rej, []string{"$.changes[].kind"}, map[string][]string{
				"$.changes[].kind": {"create", "modify", "delete", "patch", "rename"},
			})
		}
		if _, dup := seenPaths[path]; dup {
			rej := fmt.Sprintf("duplicate change for path %q (one-change-per-file constraint; combine into a single FileChange)", path)
			return rej, planRepairPackFromReason(toolName, "duplicate_change_path", rej, []string{"$.changes[].path"}, []string{path})
		}
		seenPaths[path] = i
	}
	// Per-kind shape checks. rename requires NewPath; non-rename
	// kinds must NOT carry NewPath (catches LLM emits that confuse
	// rename with modify+ move).
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		kind := strings.TrimSpace(c.Kind)
		newPath := strings.TrimSpace(c.NewPath)
		switch kind {
		case "rename":
			if newPath == "" {
				rej := fmt.Sprintf("change %q has kind=rename but new_path is empty", path)
				return rej, planRepairPackFromReason(toolName, "rename_new_path_empty", rej, []string{"$.changes[].new_path"}, []string{path})
			}
			if newPath == path {
				rej := fmt.Sprintf("change %q has kind=rename with new_path equal to path; remove the rename or pick a different destination", path)
				return rej, planRepairPackFromReason(toolName, "rename_new_path_same_as_path", rej, []string{"$.changes[].new_path"}, []string{path})
			}
			if _, collision := seenPaths[newPath]; collision {
				rej := fmt.Sprintf("change %q rename destination %q collides with another change in this plan (one path per plan, even across rename)", path, newPath)
				return rej, planRepairPackFromReason(toolName, "rename_new_path_collision", rej, []string{"$.changes[].new_path"}, []string{path, newPath})
			}
		default:
			if newPath != "" {
				rej := fmt.Sprintf("change %q has kind=%s but new_path is set (only kind=rename uses new_path)", path, kind)
				return rej, planRepairPackFromReason(toolName, "new_path_only_for_rename", rej, []string{"$.changes[].new_path", "$.changes[].kind"}, []string{path})
			}
		}
		if len(c.Edits) > 0 && kind != "patch" {
			rej := fmt.Sprintf("change %q has edits[] but kind=%s (structured edits are only valid for kind=patch)", path, kind)
			return rej, planRepairPackWithEnums(toolName, "edits_require_patch_kind", rej, []string{"$.changes[].kind", "$.changes[].edits"}, map[string][]string{
				"$.changes[].kind": {"patch"},
			})
		}
		if len(c.Edits) > 0 && strings.TrimSpace(c.Patch) != "" {
			rej := fmt.Sprintf("change %q has both patch and edits[]; choose exactly one patch source", path)
			return rej, planRepairPackFromReason(toolName, "patch_and_edits_mutually_exclusive", rej, []string{"$.changes[].patch", "$.changes[].edits"}, []string{path})
		}
	}
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		for _, dep := range c.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				rej := fmt.Sprintf("change %q has an empty depends_on entry", path)
				return rej, planRepairPackFromReason(toolName, "depends_on_empty", rej, []string{"$.changes[].depends_on"}, []string{path})
			}
			if dep == path {
				rej := fmt.Sprintf("change %q depends_on itself", path)
				return rej, planRepairPackFromReason(toolName, "depends_on_self", rej, []string{"$.changes[].depends_on"}, []string{path})
			}
			if _, ok := seenPaths[dep]; !ok {
				rej := fmt.Sprintf("change %q depends_on %q but %q is not in changes[]", path, dep, dep)
				return rej, planRepairPackFromReason(toolName, "depends_on_missing_target", rej, []string{"$.changes[].depends_on"}, []string{path, dep})
			}
		}
	}
	return "", nil
}

// validatePlanFullContent runs every validator that requires the
// new_content / patch payload to be filled in:
//   - patch pre-flight (`git apply --check`)
//   - V1 deps closure (Go imports vs go.mod)
//   - V3 wiring closure (subsystem fan-in fileset)
//   - V4 summary fidelity (paths + imports referenced in summary)
//   - V2 dry-build (`go vet` on overlay)
//
// Returns "" on success or the first failing validator's reason.
//
// Content-dependent: must NOT run from emit_plan_skeleton (NewContent
// is empty there). emit_plan_change calls this exactly once — when
// the last placeholder slot is filled — and only then promotes the
// PartialChangePlan to ChangePlan.
// validatePlanScopeKindAlignment (Method M, 2026-05-07
// patch-vs-modify forensic) enforces the typed-signal hard gate
// between WriteAnalysisIR.Request.Task.Scope and FileChange.Kind:
//
//	task.scope == "micro"  →  every existing-file change MUST be
//	                          kind=patch (kind=modify rejected)
//	other scopes           →  unconstrained (kind=patch / modify
//	                          decision left to the planner)
//
// "micro" is defined in WriteScope as "change is contained to one
// function or one constant in one file" — exactly the shape where
// kind=patch's surgical-diff guarantee is most valuable. Allowing
// kind=modify on micro-scope tasks routinely produced full-file
// overwrites for one-line typo fixes (eval forensic 2026-05-07
// patch_go_typo r1) — observably worse for the user (line-level
// review collapsed to whole-file diff) and structurally redundant
// (the same content can be expressed as a 1-hunk patch).
//
// Carve-outs (kind != "modify" → no enforcement):
//
//   - kind=create: new file; cannot patch what doesn't exist.
//   - kind=delete: file removal; no content edit needed.
//   - kind=rename: pure rename; the diff happens at git level,
//     not via patch. A rename + content edit needs TWO entries
//     (rename for the move, modify/patch for the content change),
//     so the modify/patch entry is independently scope-checked.
//   - kind=patch: ✓ what the gate enforces.
//
// Returns "" on success or a rejection reason naming the offending
// path and the structured patch alternatives so the LLM can fix without
// guessing whole-file content.
//
// Skip when ctx == nil OR ctx.Mutable == nil OR no WriteAnalysisIR
// has been emitted yet (defensive: the gate is content-dependent
// and pre-IR emits should not crash on a nil dereference). Skip
// when task.scope is empty / unknown (gate fires only on an
// authoritative LLM scope decision, never on absence).
func validatePlanScopeKindAlignment(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	scope := ir.Request.Task.Scope
	if scope != types.ScopeMicro {
		return ""
	}
	for i, c := range changes {
		if strings.TrimSpace(c.Kind) != "modify" {
			continue
		}
		path := strings.TrimSpace(c.Path)
		if path == "" {
			path = fmt.Sprintf("changes[%d]", i)
		}
		return fmt.Sprintf(
			"task.scope=micro forbids kind=modify on %q — micro-scope tasks (one function / one constant / one line in one file, per the prior classification) MUST use kind=patch with a unified diff. "+
				"A whole-file overwrite for a one-line edit collapses the diff the user reviews. "+
				"Re-emit this change with kind=patch and prefer edits[] for localized line changes; use raw patch only for complex diffs. "+
				"Carve-outs: kind=create (new file) / kind=delete (removal) / kind=rename (pure move) bypass this gate; only kind=modify-on-existing-file is rejected.",
			path)
	}
	return ""
}

func validatePlanFullContent(ctx *types.BusContext, summary string, changes []types.FileChange, verificationProbes []types.VerificationProbe) string {
	rej, _ := validatePlanFullContentWithRepair(ctx, "", summary, changes, verificationProbes)
	return rej
}

func validatePlanFullContentWithRepair(ctx *types.BusContext, toolName, summary string, changes []types.FileChange, verificationProbes []types.VerificationProbe) (string, *types.PlanRepairPack) {
	if rej, pack := validatePlanContentCarriersWithRepair(toolName, changes); rej != "" {
		return rej, pack
	}
	if rej, pack := validatePlanPathStateWithRepair(ctx, toolName, changes); rej != "" {
		return rej, pack
	}
	if rej, pack := validateFullModifyCompletenessWithRepair(ctx, toolName, changes); rej != "" {
		return rej, pack
	}
	if rej, pack := compileStructuredEditPatchesWithRepair(ctx, toolName, changes); rej != "" {
		return rej, pack
	}
	if rej := validatePythonDuplicateDefinitionStutter(ctx, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "python_duplicate_definition_stutter", rej, []string{"$.changes[].edits", "$.changes[].patch", "$.changes[].new_content"}, nil)
	}
	if rej := validatePythonUnreachableAddedStatements(ctx, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "python_unreachable_added_statement", rej, []string{"$.changes[].edits", "$.changes[].patch", "$.changes[].new_content"}, nil)
	}
	if rej := validatePlanPatchDuplicateInsertions(changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "patch_duplicate_insertions", rej, []string{"$.changes[].patch", "$.changes[].edits"}, nil)
	}
	if GitAvailable() && ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		for _, c := range changes {
			if strings.TrimSpace(c.Kind) != "patch" {
				continue
			}
			if strings.TrimSpace(c.Patch) == "" {
				rej := fmt.Sprintf("change %q has kind=patch but Patch is empty (unified-diff required)", strings.TrimSpace(c.Path))
				return rej, planRepairPackFromReason(toolName, "patch_empty", rej, []string{"$.changes[].patch", "$.changes[].edits"}, []string{strings.TrimSpace(c.Path)})
			}
			if err := CheckUnifiedDiff(ctx.RepoRoot, c.Patch); err != nil {
				rej := composePatchRejectionReason(ctx.RepoRoot, strings.TrimSpace(c.Path), err.Error(), c.Patch)
				return rej, planRepairPackFromReason(toolName, "patch_apply_check_failed", rej, []string{"$.changes[].patch", "$.changes[].edits"}, []string{strings.TrimSpace(c.Path)})
			}
		}
	}
	if rej := validatePlanDepsClosure(ctx.RepoRoot, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "deps_closure_failed", rej, []string{"$.changes[].new_content", "$.changes[].path"}, nil)
	}
	if rej := validatePlanWiringClosure(changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "wiring_closure_failed", rej, []string{"$.changes[].path"}, nil)
	}
	if rej := validatePlanSummaryConsistency(summary, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "summary_consistency_failed", rej, []string{"$.summary", "$.changes[].path", "$.changes[].new_content"}, nil)
	}
	if rej := validatePlanDryBuild(ctx, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "dry_build_failed", rej, []string{"$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, nil)
	}
	if rej := validatePlanLint(ctx, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "plan_lint_failed", rej, []string{"$.changes"}, nil)
	}
	if rej := validatePlanProjectLint(ctx, changes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "project_lint_failed", rej, []string{"$.changes"}, nil)
	}
	if rej := validateVerificationProbeContractRefs(ctx, verificationProbes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "verification_probe_contract_refs_failed", rej, []string{"$.verification_probes[].contract_refs", "$.changes[].verification_probes[].contract_refs"}, nil)
	}
	if rej := validateVerificationProbeCoupling(ctx, changes, verificationProbes); rej != "" {
		return rej, planRepairPackFromReason(toolName, "verification_probe_coupling_failed", rej, []string{"$.verification_probes[].code", "$.verification_probes[].changed_symbol_refs"}, nil)
	}
	return "", nil
}

func validatePlanPathStateWithRepair(ctx *types.BusContext, toolName string, changes []types.FileChange) (string, *types.PlanRepairPack) {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return "", nil
	}
	rootAbs, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		return "", nil
	}
	for _, change := range changes {
		path := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(change.Path)), "./")
		if path == "" {
			continue
		}
		kind := strings.TrimSpace(change.Kind)
		exists, isDir, statErr, ok := planPathState(rootAbs, path)
		if !ok {
			rej := fmt.Sprintf("change %q path escapes RepoRoot; use a repo-relative path inside the checkout", path)
			return rej, planRepairPackFromReason(toolName, "path_state_outside_repo", rej, []string{"$.changes[].path"}, []string{path})
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			rej := fmt.Sprintf("change %q cannot be statted before planning: %v", path, statErr)
			return rej, planRepairPackFromReason(toolName, "path_state_stat_failed", rej, []string{"$.changes[].path"}, []string{path})
		}
		switch kind {
		case "create":
			if exists {
				state := "file"
				if isDir {
					state = "directory"
				}
				rej := fmt.Sprintf("change %q has kind=create but that %s already exists; use kind=patch/modify for an existing file or choose a new path", path, state)
				return rej, planRepairPackWithEnums(toolName, "create_path_exists", rej, []string{"$.changes[].kind", "$.changes[].path"}, map[string][]string{
					"$.changes[].kind": {"patch", "modify", "create"},
				})
			}
		case "modify", "patch":
			if !exists {
				rej := fmt.Sprintf("change %q has kind=%s but the file does not exist; use kind=create for a new file", path, kind)
				return rej, planRepairPackWithEnums(toolName, kind+"_path_missing", rej, []string{"$.changes[].kind", "$.changes[].path"}, map[string][]string{
					"$.changes[].kind": {"create", kind},
				})
			}
			if isDir {
				rej := fmt.Sprintf("change %q has kind=%s but the path is a directory; choose a regular file path", path, kind)
				return rej, planRepairPackFromReason(toolName, kind+"_path_is_directory", rej, []string{"$.changes[].path"}, []string{path})
			}
		case "delete":
			if exists && isDir {
				rej := fmt.Sprintf("change %q has kind=delete but the path is a directory; delete changes must target files, not directories", path)
				return rej, planRepairPackFromReason(toolName, "delete_path_is_directory", rej, []string{"$.changes[].path"}, []string{path})
			}
		case "rename":
			if !exists {
				rej := fmt.Sprintf("change %q has kind=rename but the source file does not exist", path)
				return rej, planRepairPackFromReason(toolName, "rename_source_missing", rej, []string{"$.changes[].path"}, []string{path})
			}
			if isDir {
				rej := fmt.Sprintf("change %q has kind=rename but the source path is a directory; rename changes must target files", path)
				return rej, planRepairPackFromReason(toolName, "rename_source_is_directory", rej, []string{"$.changes[].path"}, []string{path})
			}
			newPath := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(change.NewPath)), "./")
			if newPath == "" {
				continue
			}
			destExists, destIsDir, destErr, destOK := planPathState(rootAbs, newPath)
			if !destOK {
				rej := fmt.Sprintf("change %q new_path %q escapes RepoRoot; use a repo-relative destination inside the checkout", path, newPath)
				return rej, planRepairPackFromReason(toolName, "rename_destination_outside_repo", rej, []string{"$.changes[].new_path"}, []string{path, newPath})
			}
			if destErr != nil && !os.IsNotExist(destErr) {
				rej := fmt.Sprintf("change %q new_path %q cannot be statted before planning: %v", path, newPath, destErr)
				return rej, planRepairPackFromReason(toolName, "rename_destination_stat_failed", rej, []string{"$.changes[].new_path"}, []string{path, newPath})
			}
			if destExists {
				state := "file"
				if destIsDir {
					state = "directory"
				}
				rej := fmt.Sprintf("change %q has kind=rename but destination %q already exists as a %s", path, newPath, state)
				return rej, planRepairPackFromReason(toolName, "rename_destination_exists", rej, []string{"$.changes[].new_path"}, []string{path, newPath})
			}
		}
	}
	return "", nil
}

func planPathState(rootAbs, repoRel string) (exists bool, isDir bool, statErr error, ok bool) {
	if strings.TrimSpace(rootAbs) == "" || strings.TrimSpace(repoRel) == "" {
		return false, false, os.ErrNotExist, false
	}
	abs := filepath.Join(rootAbs, filepath.FromSlash(repoRel))
	abs, err := filepath.Abs(abs)
	if err != nil {
		return false, false, err, true
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return false, false, err, false
	}
	info, err := os.Stat(abs)
	if err != nil {
		return false, false, err, true
	}
	return true, info.IsDir(), nil, true
}

func validatePlanContentCarriers(changes []types.FileChange) string {
	rej, _ := validatePlanContentCarriersWithRepair("", changes)
	return rej
}

func validatePlanContentCarriersWithRepair(toolName string, changes []types.FileChange) (string, *types.PlanRepairPack) {
	for _, change := range changes {
		path := strings.TrimSpace(change.Path)
		kind := strings.TrimSpace(change.Kind)
		hasNewContent := strings.TrimSpace(change.NewContent) != ""
		hasPatch := strings.TrimSpace(change.Patch) != ""
		hasEdits := len(change.Edits) > 0
		switch kind {
		case "create", "modify":
			if hasPatch || hasEdits {
				rej := fmt.Sprintf("change %q has kind=%s but also carries patch/edits content; create/modify require new_content as the only content carrier", path, kind)
				return rej, planRepairPackFromReason(toolName, "content_carrier_conflict", rej, []string{"$.changes[].kind", "$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
			}
			if !hasNewContent {
				rej := fmt.Sprintf("change %q has kind=%s but new_content is empty; provide the full file body or choose kind=delete/patch as appropriate", path, kind)
				return rej, planRepairPackFromReason(toolName, "new_content_required", rej, []string{"$.changes[].new_content", "$.changes[].kind"}, []string{path})
			}
		case "patch":
			if hasNewContent {
				rej := fmt.Sprintf("change %q has kind=patch but also carries new_content; patch changes must use only patch or edits[] so full-file overwrite bytes cannot leak into a surgical plan", path)
				return rej, planRepairPackFromReason(toolName, "patch_new_content_conflict", rej, []string{"$.changes[].kind", "$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
			}
			if !hasPatch && !hasEdits {
				rej := fmt.Sprintf("change %q has kind=patch but Patch is empty (unified-diff required)", path)
				return rej, planRepairPackFromReason(toolName, "patch_empty", rej, []string{"$.changes[].patch", "$.changes[].edits"}, []string{path})
			}
		case "delete":
			if hasNewContent || hasPatch || hasEdits {
				rej := fmt.Sprintf("change %q has kind=delete but carries content fields; delete changes must not include new_content, patch, or edits[]", path)
				return rej, planRepairPackFromReason(toolName, "delete_content_forbidden", rej, []string{"$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
			}
		case "rename":
			if hasNewContent || hasPatch || hasEdits {
				rej := fmt.Sprintf("change %q has kind=rename but carries content fields; use a separate modify/patch change for content edits", path)
				return rej, planRepairPackFromReason(toolName, "rename_content_forbidden", rej, []string{"$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
			}
		}
	}
	return "", nil
}

const (
	fullModifyLargeFileLineThreshold     = 400
	fullModifyLargeFileByteThreshold     = 20 * 1024
	fullModifyPrefixTruncationPct        = 80
	fullModifyDrasticLineShrinkPct       = 50
	fullModifyDrasticByteShrinkPct       = 50
	fullModifyMinDeletedLinesForHardGate = 300
	fullModifyMinDeletedBytesForHardGate = 20 * 1024
)

func validateFullModifyCompleteness(ctx *types.BusContext, changes []types.FileChange) string {
	rej, _ := validateFullModifyCompletenessWithRepair(ctx, "", changes)
	return rej
}

func validateFullModifyCompletenessWithRepair(ctx *types.BusContext, toolName string, changes []types.FileChange) (string, *types.PlanRepairPack) {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return "", nil
	}
	repoRootAbs, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		return "", nil
	}
	for _, change := range changes {
		if strings.TrimSpace(change.Kind) != "modify" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" {
			continue
		}
		abs := filepath.Clean(filepath.Join(repoRootAbs, filepath.FromSlash(path)))
		absCanonical, err := filepath.Abs(abs)
		if err != nil {
			continue
		}
		if absCanonical != repoRootAbs && !strings.HasPrefix(absCanonical, repoRootAbs+string(filepath.Separator)) {
			continue
		}
		oldBytes, err := os.ReadFile(absCanonical)
		if err != nil || len(oldBytes) == 0 {
			continue
		}
		newBytes := []byte(change.NewContent)
		if bytes.Equal(oldBytes, newBytes) {
			continue
		}
		oldLines := countPlanContentLines(string(oldBytes))
		newLines := countPlanContentLines(change.NewContent)
		largeByLines := oldLines >= fullModifyLargeFileLineThreshold
		largeByBytes := len(oldBytes) >= fullModifyLargeFileByteThreshold
		if !largeByLines && !largeByBytes {
			continue
		}
		deletedLines := oldLines - newLines
		deletedBytes := len(oldBytes) - len(newBytes)
		if looksLikePrefixTruncation(oldBytes, newBytes, deletedLines, deletedBytes) {
			rej := fmt.Sprintf("change %q uses kind=modify on an existing large file but new_content is a strict prefix/truncated subset of the current file (%d→%d lines, %d→%d bytes). Use kind=patch for localized edits or provide the complete full file body for an intentional rewrite.",
				path, oldLines, newLines, len(oldBytes), len(newBytes))
			return rej, planRepairPackFromReason(toolName, "full_modify_prefix_truncation", rej, []string{"$.changes[].kind", "$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
		}
		if looksLikeCatastrophicFullModifyShrink(oldLines, newLines, len(oldBytes), len(newBytes), deletedLines, deletedBytes) {
			rej := fmt.Sprintf("change %q uses kind=modify on an existing large file and removes most of the file (%d→%d lines, %d→%d bytes). This is likely a partial generated file body. Use kind=patch for surgical edits, kind=delete for intentional removal, or provide a complete full-file rewrite with comparable scope.",
				path, oldLines, newLines, len(oldBytes), len(newBytes))
			return rej, planRepairPackFromReason(toolName, "full_modify_catastrophic_shrink", rej, []string{"$.changes[].kind", "$.changes[].new_content", "$.changes[].patch", "$.changes[].edits"}, []string{path})
		}
	}
	return "", nil
}

func countPlanContentLines(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

func looksLikePrefixTruncation(oldBytes, newBytes []byte, deletedLines, deletedBytes int) bool {
	if len(newBytes) == 0 || len(newBytes) >= len(oldBytes) {
		return false
	}
	if !bytes.HasPrefix(oldBytes, newBytes) {
		return false
	}
	if len(newBytes)*100 >= len(oldBytes)*fullModifyPrefixTruncationPct {
		return false
	}
	return deletedLines >= 100 || deletedBytes >= 4096
}

func looksLikeCatastrophicFullModifyShrink(oldLines, newLines, oldByteLen, newByteLen, deletedLines, deletedBytes int) bool {
	lineShrink := oldLines > 0 && newLines*100 < oldLines*fullModifyDrasticLineShrinkPct && deletedLines >= fullModifyMinDeletedLinesForHardGate
	byteShrink := oldByteLen > 0 && newByteLen*100 < oldByteLen*fullModifyDrasticByteShrinkPct && deletedBytes >= fullModifyMinDeletedBytesForHardGate
	return lineShrink || byteShrink
}

type verificationProbeCouplingProvider struct {
	Language       string
	DisplayName    string
	TargetProducer func(repoRoot string, changes []types.FileChange) map[string]struct{}
	ProbeRefs      func(probe types.VerificationProbe) map[string]struct{}
	Covers         func(ref, target string) bool
}

var verificationProbeCouplingProviders = []verificationProbeCouplingProvider{
	{
		Language:       "python",
		DisplayName:    "Python",
		TargetProducer: pythonProductionModuleCandidatesWithRepo,
		ProbeRefs:      func(probe types.VerificationProbe) map[string]struct{} { return pythonImportDeclarations(probe.Code) },
		Covers:         pythonImportCoversTarget,
	},
	{
		Language:       "javascript",
		DisplayName:    "JavaScript/TypeScript",
		TargetProducer: javascriptProductionModuleCandidates,
		ProbeRefs:      javascriptImportDeclarations,
		Covers:         slashModuleRefCoversTarget,
	},
	{
		Language:       "ruby",
		DisplayName:    "Ruby",
		TargetProducer: rubyProductionModuleCandidates,
		ProbeRefs:      rubyRequireDeclarations,
		Covers:         rubyRequireCoversTarget,
	},
	{
		Language:       "java",
		DisplayName:    "Java",
		TargetProducer: javaProductionClassCandidates,
		ProbeRefs:      javaImportDeclarations,
		Covers:         javaRefCoversTarget,
	},
	{
		Language:       "go",
		DisplayName:    "Go",
		TargetProducer: goProductionPackageCandidates,
		ProbeRefs:      goImportDeclarations,
		Covers:         goImportCoversTarget,
	},
}

func validateVerificationProbeCoupling(ctx *types.BusContext, changes []types.FileChange, probes []types.VerificationProbe) string {
	if len(probes) == 0 {
		return ""
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	for _, provider := range verificationProbeCouplingProviders {
		if rej := validateVerificationProbeCouplingForProvider(repoRoot, changes, probes, provider); rej != "" {
			return rej
		}
	}
	return ""
}

func validateVerificationProbeCouplingForProvider(repoRoot string, changes []types.FileChange, probes []types.VerificationProbe, provider verificationProbeCouplingProvider) string {
	if provider.Language == "" || provider.TargetProducer == nil || provider.ProbeRefs == nil || provider.Covers == nil {
		return ""
	}
	targets := provider.TargetProducer(repoRoot, changes)
	if len(targets) == 0 {
		return ""
	}
	probeCount := 0
	refsSeen := map[string]struct{}{}
	for _, probe := range probes {
		language, ok := normalizeVerificationProbeLanguage(probe.Language)
		if !ok || language != provider.Language {
			continue
		}
		probeCount++
		refs := provider.ProbeRefs(probe)
		if len(refs) == 0 {
			continue
		}
		for ref := range refs {
			refsSeen[ref] = struct{}{}
		}
		if probeRefsCoverAnyTarget(refs, targets, provider.Covers) {
			return ""
		}
	}
	if probeCount == 0 {
		return ""
	}
	if len(refsSeen) == 0 {
		return fmt.Sprintf("verification_probes do not include any %s import/require declarations for changed production modules %s; at least one %s probe must import or require the changed module under test instead of checking an isolated copy", provider.DisplayName, formatStringSet(targets), provider.DisplayName)
	}
	return fmt.Sprintf("verification_probes reference %s but do not reference any changed %s production module %s; at least one %s probe must exercise the changed code, not a copied implementation fragment", formatStringSet(refsSeen), provider.DisplayName, formatStringSet(targets), provider.DisplayName)
}

func probeRefsCoverAnyTarget(refs, targets map[string]struct{}, covers func(ref, target string) bool) bool {
	for target := range targets {
		for ref := range refs {
			if covers(ref, target) {
				return true
			}
		}
	}
	return false
}

const pythonDuplicateDefinitionStutterMaxGap = 6

type pythonDefinitionStutter struct {
	Kind       string
	Name       string
	Scope      string
	FirstLine  int
	SecondLine int
}

type pythonDefinitionSeen struct {
	Kind       string
	Line       int
	Indent     int
	Decorators []string
}

type pythonScopeFrame struct {
	Indent  int
	Segment string
}

func validatePythonDuplicateDefinitionStutter(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil {
		return ""
	}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") {
			continue
		}
		content, ok, rej := plannedPythonContent(ctx.RepoRoot, change)
		if rej != "" {
			return rej
		}
		if !ok {
			continue
		}
		dups := findPythonDuplicateDefinitionStutters(content)
		if len(dups) == 0 {
			continue
		}
		originalCounts := map[string]int{}
		if original, ok := readOriginalPythonContent(ctx.RepoRoot, change); ok {
			for _, dup := range findPythonDuplicateDefinitionStutters(original) {
				originalCounts[pythonDefinitionStutterKey(dup)]++
			}
		}
		for _, dup := range dups {
			key := pythonDefinitionStutterKey(dup)
			if originalCounts[key] > 0 {
				originalCounts[key]--
				continue
			}
			scope := "module"
			if strings.TrimSpace(dup.Scope) != "" {
				scope = dup.Scope
			}
			return fmt.Sprintf("change %q would create duplicate Python %s %q in scope %s at lines %d and %d. This usually means a structured insert landed next to the existing definition; replace the existing definition/body instead of inserting a second one.",
				path, dup.Kind, dup.Name, scope, dup.FirstLine, dup.SecondLine)
		}
	}
	return ""
}

func validatePythonUnreachableAddedStatements(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil {
		return ""
	}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") {
			continue
		}
		planned, ok, rej := plannedPythonContent(ctx.RepoRoot, change)
		if rej != "" {
			return rej
		}
		if !ok {
			continue
		}
		original, _ := readOriginalPythonContent(ctx.RepoRoot, change)
		added := pythonAddedLineNumbers(original, planned)
		if len(added) == 0 {
			continue
		}
		if lineNo, terminalLine, statement, ok := firstPythonUnreachableAddedStatement(planned, added); ok {
			return fmt.Sprintf(
				"change %q adds Python statement %q at line %d after a terminal statement at line %d in the same block. "+
					"Newly added unreachable code usually means the patch landed in the wrong function or after an early return; move the edit before the terminal statement or into the intended target block.",
				path, statement, lineNo, terminalLine)
		}
	}
	return ""
}

func pythonAddedLineNumbers(original, planned string) map[int]bool {
	counts := map[string]int{}
	for _, line := range strings.Split(original, "\n") {
		counts[line]++
	}
	added := map[int]bool{}
	for i, line := range strings.Split(planned, "\n") {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		added[i+1] = true
	}
	return added
}

type pythonTerminalBlock struct {
	Indent int
	Line   int
}

func firstPythonUnreachableAddedStatement(content string, added map[int]bool) (lineNo int, terminalLine int, statement string, ok bool) {
	if len(added) == 0 {
		return 0, 0, "", false
	}
	var terminal *pythonTerminalBlock
	inTriple := ""
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		currentLine := i + 1
		if inTriple != "" {
			if pythonTripleQuoteCount(line, inTriple)%2 == 1 {
				inTriple = ""
			}
			continue
		}
		code := stripPythonLineComment(line)
		trimmed := strings.TrimSpace(code)
		if trimmed == "" {
			continue
		}
		if delim, ok := pythonOpeningTripleQuote(trimmed); ok {
			if pythonTripleQuoteCount(trimmed, delim)%2 == 1 {
				inTriple = delim
			}
			continue
		}
		indent := pythonLeadingIndent(line)
		if terminal != nil && indent < terminal.Indent {
			terminal = nil
		}
		if terminal != nil && indent == terminal.Indent && currentLine > terminal.Line && added[currentLine] {
			return currentLine, terminal.Line, trimmed, true
		}
		if pythonSimpleTerminalStatement(trimmed) {
			terminal = &pythonTerminalBlock{Indent: indent, Line: currentLine}
			continue
		}
		if terminal != nil && indent == terminal.Indent {
			// Pre-existing or non-added same-block code after a terminal statement
			// means the file already has a shape this gate cannot safely judge.
			terminal = nil
		}
	}
	return 0, 0, "", false
}

func pythonSimpleTerminalStatement(trimmed string) bool {
	if trimmed == "return" || trimmed == "raise" || trimmed == "break" || trimmed == "continue" {
		return true
	}
	for _, prefix := range []string{"return ", "raise "} {
		if strings.HasPrefix(trimmed, prefix) {
			return pythonStatementLooksSingleLine(trimmed)
		}
	}
	return false
}

func pythonStatementLooksSingleLine(trimmed string) bool {
	if strings.HasSuffix(trimmed, "\\") {
		return false
	}
	balance := 0
	var quote rune
	escaped := false
	for _, r := range trimmed {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		switch r {
		case '(', '[', '{':
			balance++
		case ')', ']', '}':
			if balance > 0 {
				balance--
			}
		}
	}
	return quote == 0 && balance == 0
}

func plannedPythonContent(repoRoot string, change types.FileChange) (string, bool, string) {
	kind := strings.TrimSpace(change.Kind)
	switch kind {
	case "create", "modify":
		if strings.TrimSpace(change.NewContent) == "" {
			return "", false, ""
		}
		return change.NewContent, true, ""
	case "patch":
		if len(change.Edits) > 0 {
			_, newContent, err := compileStructuredEditsToContent(repoRoot, &change)
			if err != nil {
				return "", false, enrichStructuredEditReplanDiagnostic(nil, err.Error())
			}
			return newContent, true, ""
		}
		if strings.TrimSpace(change.Patch) != "" {
			if strings.TrimSpace(repoRoot) == "" {
				return "", false, ""
			}
			newContent, err := applyUnifiedDiffToTempAndRead(repoRoot, change)
			if err != nil {
				return "", false, err.Error()
			}
			return newContent, true, ""
		}
		return "", false, ""
	default:
		return "", false, ""
	}
}

func applyUnifiedDiffToTempAndRead(repoRoot string, change types.FileChange) (string, error) {
	root := strings.TrimSpace(repoRoot)
	path := filepath.ToSlash(strings.TrimSpace(change.Path))
	patch := strings.TrimSpace(change.Patch)
	if root == "" || path == "" || patch == "" {
		return "", fmt.Errorf("change %q cannot be materialized from patch without repo root, path, and patch bytes", path)
	}
	original, ok := readOriginalPythonContent(root, change)
	if !ok {
		return "", fmt.Errorf("change %q cannot be materialized because the current file bytes are unavailable", path)
	}
	tmp, err := os.MkdirTemp("", "codrax-plan-patch-*")
	if err != nil {
		return "", fmt.Errorf("create temp patch workspace: %w", err)
	}
	defer os.RemoveAll(tmp)
	target := filepath.Join(tmp, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create temp patch parent: %w", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		return "", fmt.Errorf("write temp patch source: %w", err)
	}
	if err := applyUnifiedDiff(tmp, change.Patch); err != nil {
		return "", fmt.Errorf("change %q patch could not be materialized for validation: %v", path, err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read temp patch result: %w", err)
	}
	return string(data), nil
}

func findPythonDuplicateDefinitionStutter(content string) (pythonDefinitionStutter, bool) {
	dups := findPythonDuplicateDefinitionStutters(content)
	if len(dups) == 0 {
		return pythonDefinitionStutter{}, false
	}
	return dups[0], true
}

func findPythonDuplicateDefinitionStutters(content string) []pythonDefinitionStutter {
	seen := map[string]pythonDefinitionSeen{}
	var stack []pythonScopeFrame
	var dups []pythonDefinitionStutter
	inTriple := ""
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lineNo := i + 1
		if inTriple != "" {
			if pythonTripleQuoteCount(line, inTriple)%2 == 1 {
				inTriple = ""
			}
			continue
		}
		code := stripPythonLineComment(line)
		trimmed := strings.TrimSpace(code)
		if trimmed == "" {
			continue
		}
		if delim, ok := pythonOpeningTripleQuote(trimmed); ok {
			if pythonTripleQuoteCount(trimmed, delim)%2 == 1 {
				inTriple = delim
			}
			continue
		}
		kind, name, ok := pythonDefinitionHeader(trimmed)
		if !ok {
			continue
		}
		indent := pythonLeadingIndent(line)
		decorators := pythonDecoratorsForDefinition(lines, i, indent)
		for len(stack) > 0 && indent <= stack[len(stack)-1].Indent {
			stack = stack[:len(stack)-1]
		}
		scope := pythonScopePath(stack)
		key := scope + "\x00" + name
		if prev, exists := seen[key]; exists &&
			lineNo-prev.Line <= pythonDuplicateDefinitionStutterMaxGap &&
			!pythonDuplicateDefinitionHasInterveningControlFlow(lines, prev.Line, lineNo, indent) &&
			!pythonDuplicateDefinitionAccessorPair(name, prev.Decorators, decorators) &&
			!pythonDuplicateDefinitionOverloadPair(prev.Decorators, decorators) {
			dups = append(dups, pythonDefinitionStutter{
				Kind:       kind,
				Name:       name,
				Scope:      scope,
				FirstLine:  prev.Line,
				SecondLine: lineNo,
			})
		}
		seen[key] = pythonDefinitionSeen{Kind: kind, Line: lineNo, Indent: indent, Decorators: decorators}
		stack = append(stack, pythonScopeFrame{
			Indent:  indent,
			Segment: kind + " " + name,
		})
	}
	return dups
}

func pythonDefinitionStutterKey(dup pythonDefinitionStutter) string {
	return dup.Kind + "\x00" + dup.Name + "\x00" + dup.Scope
}

func readOriginalPythonContent(repoRoot string, change types.FileChange) (string, bool) {
	root := strings.TrimSpace(repoRoot)
	path := filepath.ToSlash(strings.TrimSpace(change.Path))
	if root == "" || path == "" {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPath := filepath.Join(absRoot, filepath.FromSlash(path))
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func pythonDecoratorsForDefinition(lines []string, defIndex int, defIndent int) []string {
	if defIndex <= 0 || defIndex > len(lines) {
		return nil
	}
	var decorators []string
	for i := defIndex - 1; i >= 0; i-- {
		line := stripPythonLineComment(lines[i])
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if pythonLeadingIndent(line) != defIndent || !strings.HasPrefix(trimmed, "@") {
			break
		}
		decorators = append(decorators, trimmed)
	}
	for i, j := 0, len(decorators)-1; i < j; i, j = i+1, j-1 {
		decorators[i], decorators[j] = decorators[j], decorators[i]
	}
	return decorators
}

func pythonDuplicateDefinitionAccessorPair(name string, prevDecorators, currentDecorators []string) bool {
	prevKind := pythonPropertyAccessorKind(name, prevDecorators)
	currentKind := pythonPropertyAccessorKind(name, currentDecorators)
	if currentKind == "setter" || currentKind == "deleter" {
		return prevKind == "property" || prevKind == "setter" || prevKind == "deleter"
	}
	return false
}

func pythonPropertyAccessorKind(name string, decorators []string) string {
	if !isPythonIdentifier(name) {
		return ""
	}
	for _, raw := range decorators {
		dec := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		dec = strings.TrimSpace(strings.TrimSuffix(dec, "()"))
		switch dec {
		case "property":
			return "property"
		case name + ".setter":
			return "setter"
		case name + ".deleter":
			return "deleter"
		}
	}
	return ""
}

func pythonDuplicateDefinitionOverloadPair(prevDecorators, currentDecorators []string) bool {
	return pythonDecoratorsContainTypingOverload(prevDecorators) || pythonDecoratorsContainTypingOverload(currentDecorators)
}

func pythonDecoratorsContainTypingOverload(decorators []string) bool {
	for _, raw := range decorators {
		dec := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		dec = strings.TrimSpace(strings.TrimSuffix(dec, "()"))
		if dec == "overload" || dec == "typing.overload" {
			return true
		}
	}
	return false
}

func pythonDuplicateDefinitionHasInterveningControlFlow(lines []string, firstLine, secondLine, definitionIndent int) bool {
	if firstLine < 1 || secondLine <= firstLine || secondLine > len(lines) {
		return false
	}
	for i := firstLine; i < secondLine-1; i++ {
		line := stripPythonLineComment(lines[i])
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := pythonLeadingIndent(line)
		if indent > definitionIndent {
			continue
		}
		if pythonControlFlowHeader(trimmed) {
			return true
		}
	}
	return false
}

func pythonControlFlowHeader(trimmed string) bool {
	if !strings.HasSuffix(trimmed, ":") {
		return false
	}
	for _, prefix := range []string{
		"if ", "elif ", "else", "try", "except", "finally", "for ", "while ", "with ", "match ", "case ",
	} {
		if trimmed == strings.TrimSuffix(prefix, " ") || strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func pythonDefinitionHeader(trimmed string) (kind string, name string, ok bool) {
	switch {
	case strings.HasPrefix(trimmed, "async def "):
		name = pythonIdentifierAfterPrefix(trimmed, "async def ")
		kind = "function"
	case strings.HasPrefix(trimmed, "def "):
		name = pythonIdentifierAfterPrefix(trimmed, "def ")
		kind = "function"
	case strings.HasPrefix(trimmed, "class "):
		name = pythonIdentifierAfterPrefix(trimmed, "class ")
		kind = "class"
	default:
		return "", "", false
	}
	if !isPythonIdentifier(name) {
		return "", "", false
	}
	return kind, name, true
}

func pythonIdentifierAfterPrefix(line, prefix string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	for i, r := range rest {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return rest[:i]
		}
	}
	return rest
}

func pythonLeadingIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 8
		default:
			return indent
		}
	}
	return indent
}

func pythonScopePath(stack []pythonScopeFrame) string {
	if len(stack) == 0 {
		return ""
	}
	parts := make([]string, 0, len(stack))
	for _, frame := range stack {
		parts = append(parts, frame.Segment)
	}
	return strings.Join(parts, ".")
}

func pythonOpeningTripleQuote(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, `"""`) {
		return `"""`, true
	}
	if strings.HasPrefix(trimmed, `'''`) {
		return `'''`, true
	}
	return "", false
}

func pythonTripleQuoteCount(line, delim string) int {
	if delim == "" {
		return 0
	}
	count := 0
	for {
		idx := strings.Index(line, delim)
		if idx < 0 {
			return count
		}
		count++
		line = line[idx+len(delim):]
	}
}

func pythonProductionModuleCandidates(changes []types.FileChange) map[string]struct{} {
	out := map[string]struct{}{}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") || types.LooksLikeTestFilePath(path) {
			continue
		}
		for _, mod := range pythonModuleCandidatesForPath(path) {
			out[mod] = struct{}{}
		}
	}
	return out
}

func pythonProductionModuleCandidatesWithRepo(repoRoot string, changes []types.FileChange) map[string]struct{} {
	out := pythonProductionModuleCandidates(changes)
	for publicPackage := range pythonRepoLocalPublicPackagesForChanges(repoRoot, changes) {
		out[publicPackage] = struct{}{}
	}
	return out
}

func javascriptProductionModuleCandidates(repoRoot string, changes []types.FileChange) map[string]struct{} {
	out := slashProductionModuleCandidates(changes, map[string]bool{
		".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
		".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	})
	if len(out) == 0 {
		return out
	}
	if name := packageJSONName(repoRoot); name != "" {
		out[normalizeSlashModuleRef(name)] = struct{}{}
	}
	return out
}

func rubyProductionModuleCandidates(_ string, changes []types.FileChange) map[string]struct{} {
	return slashProductionModuleCandidates(changes, map[string]bool{".rb": true})
}

func slashProductionModuleCandidates(changes []types.FileChange, exts map[string]bool) map[string]struct{} {
	out := map[string]struct{}{}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || types.LooksLikeTestFilePath(path) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !exts[ext] {
			continue
		}
		for _, candidate := range slashModuleCandidatesForPath(path) {
			out[candidate] = struct{}{}
		}
	}
	return out
}

func slashModuleCandidatesForPath(relPath string) []string {
	relPath = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relPath)), "./")
	ext := filepath.Ext(relPath)
	if ext != "" {
		relPath = strings.TrimSuffix(relPath, ext)
	}
	relPath = strings.TrimSuffix(relPath, "/index")
	relPath = strings.TrimSuffix(relPath, "/__init__")
	relPath = strings.Trim(relPath, "/")
	if relPath == "" {
		return nil
	}
	candidates := []string{normalizeSlashModuleRef(relPath)}
	for _, root := range []string{"src/", "lib/", "app/"} {
		if strings.HasPrefix(relPath, root) && len(relPath) > len(root) {
			candidates = append(candidates, normalizeSlashModuleRef(strings.TrimPrefix(relPath, root)))
		}
	}
	return uniqueNonEmptyStrings(candidates)
}

func packageJSONName(repoRoot string) string {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "package.json"))
	if err != nil {
		return ""
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Name)
}

func pythonModuleCandidatesForPath(relPath string) []string {
	relPath = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relPath)), "./")
	if !strings.HasSuffix(relPath, ".py") {
		return nil
	}
	stem := strings.TrimSuffix(relPath, ".py")
	if strings.HasSuffix(stem, "/__init__") {
		stem = strings.TrimSuffix(stem, "/__init__")
	}
	stem = strings.Trim(stem, "/")
	if stem == "" {
		return nil
	}
	raw := strings.ReplaceAll(stem, "/", ".")
	candidates := []string{raw}
	for _, root := range []string{"src.", "lib."} {
		if strings.HasPrefix(raw, root) && len(raw) > len(root) {
			candidates = append(candidates, strings.TrimPrefix(raw, root))
		}
	}
	return uniqueNonEmptyStrings(candidates)
}

func pythonImportDeclarations(code string) map[string]struct{} {
	out := map[string]struct{}{}
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		parsePythonImportDeclaration(line, out)
	}
	return out
}

func parsePythonImportDeclaration(line string, out map[string]struct{}) {
	line = strings.TrimSpace(stripPythonLineComment(line))
	if line == "" || strings.HasPrefix(line, ".") {
		return
	}
	if strings.HasPrefix(line, "import ") {
		body := strings.TrimSpace(strings.TrimPrefix(line, "import "))
		for _, part := range strings.Split(body, ",") {
			mod := firstPythonImportToken(part)
			if isPythonModuleName(mod) {
				out[mod] = struct{}{}
			}
		}
		return
	}
	if strings.HasPrefix(line, "from ") {
		body := strings.TrimSpace(strings.TrimPrefix(line, "from "))
		idx := strings.Index(body, " import ")
		if idx <= 0 {
			return
		}
		base := strings.TrimSpace(body[:idx])
		if !isPythonModuleName(base) {
			return
		}
		out[base] = struct{}{}
		imports := strings.TrimSpace(body[idx+len(" import "):])
		imports = strings.Trim(imports, "()")
		for _, part := range strings.Split(imports, ",") {
			name := firstPythonImportToken(part)
			if isPythonIdentifier(name) {
				out[base+"."+name] = struct{}{}
			}
		}
	}
}

func javascriptImportDeclarations(probe types.VerificationProbe) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(probe.Code, "\n") {
		line = strings.TrimSpace(stripCLikeLineComment(line))
		if line == "" {
			continue
		}
		collectQuotedCallArgs(line, "require(", out, normalizeSlashModuleRef)
		collectQuotedCallArgs(line, "import(", out, normalizeSlashModuleRef)
		if strings.HasPrefix(line, "import ") {
			if idx := strings.LastIndex(line, " from "); idx >= 0 {
				addNormalizedQuotedString(line[idx+len(" from "):], out, normalizeSlashModuleRef)
				continue
			}
			addNormalizedQuotedString(line, out, normalizeSlashModuleRef)
		}
	}
	return out
}

func rubyRequireDeclarations(probe types.VerificationProbe) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(probe.Code, "\n") {
		line = strings.TrimSpace(stripPythonLineComment(line))
		for _, keyword := range []string{"require_relative", "require", "load"} {
			if !strings.HasPrefix(line, keyword) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(line, keyword))
			addNormalizedQuotedString(rest, out, normalizeSlashModuleRef)
		}
	}
	return out
}

func javaProductionClassCandidates(repoRoot string, changes []types.FileChange) map[string]struct{} {
	out := map[string]struct{}{}
	for _, change := range changes {
		rel := filepath.ToSlash(strings.TrimSpace(change.Path))
		if rel == "" || !strings.HasSuffix(rel, ".java") || types.LooksLikeTestFilePath(rel) {
			continue
		}
		className := strings.TrimSuffix(pathBaseSlash(rel), ".java")
		if isJavaIdentifier(className) {
			out[className] = struct{}{}
		}
		content := strings.TrimSpace(change.NewContent)
		if content == "" && strings.TrimSpace(repoRoot) != "" {
			if data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel))); err == nil {
				content = string(data)
			}
		}
		if pkg := javaPackageDeclaration(content); pkg != "" && className != "" {
			out[pkg+"."+className] = struct{}{}
		}
		for _, candidate := range javaClassCandidatesFromPath(rel) {
			out[candidate] = struct{}{}
		}
	}
	return out
}

func javaImportDeclarations(probe types.VerificationProbe) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(probe.Code, "\n") {
		line = strings.TrimSpace(stripCLikeLineComment(line))
		if strings.HasPrefix(line, "import ") {
			ref := strings.TrimSpace(strings.TrimPrefix(line, "import "))
			ref = strings.TrimSpace(strings.TrimPrefix(ref, "static "))
			ref = strings.TrimSuffix(ref, ";")
			ref = strings.TrimSpace(ref)
			if isJavaDottedRef(ref) || strings.HasSuffix(ref, ".*") {
				out[ref] = struct{}{}
			}
			continue
		}
		for _, token := range javaTypeTokens(line) {
			out[token] = struct{}{}
		}
	}
	return out
}

func javaRefCoversTarget(ref, target string) bool {
	ref = strings.TrimSpace(ref)
	target = strings.TrimSpace(target)
	if ref == "" || target == "" {
		return false
	}
	if ref == target {
		return true
	}
	if strings.HasSuffix(ref, ".*") {
		prefix := strings.TrimSuffix(ref, "*")
		return strings.HasPrefix(target, prefix)
	}
	if strings.Contains(ref, ".") && strings.HasPrefix(ref, target+".") {
		return true
	}
	if !strings.Contains(target, ".") && strings.HasSuffix(ref, "."+target) {
		return true
	}
	if strings.Contains(target, ".") && strings.HasSuffix(target, "."+ref) {
		return true
	}
	return false
}

func javaPackageDeclaration(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(stripCLikeLineComment(line))
		if strings.HasPrefix(line, "package ") {
			pkg := strings.TrimSpace(strings.TrimPrefix(line, "package "))
			pkg = strings.TrimSuffix(pkg, ";")
			pkg = strings.TrimSpace(pkg)
			if isJavaDottedRef(pkg) {
				return pkg
			}
			return ""
		}
	}
	return ""
}

func javaClassCandidatesFromPath(rel string) []string {
	rel = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(rel), "./"))
	if !strings.HasSuffix(rel, ".java") {
		return nil
	}
	noExt := strings.TrimSuffix(rel, ".java")
	className := pathBaseSlash(rel)
	className = strings.TrimSuffix(className, ".java")
	var out []string
	for _, marker := range []string{
		"src/main/java/",
		"src/test/java/",
		"app/src/main/java/",
		"java/",
	} {
		if strings.Contains(noExt, marker) {
			idx := strings.LastIndex(noExt, marker)
			candidate := strings.Trim(strings.ReplaceAll(noExt[idx+len(marker):], "/", "."), ".")
			if isJavaDottedRef(candidate) {
				out = append(out, candidate)
			}
		}
	}
	if isJavaIdentifier(className) {
		out = append(out, className)
	}
	return uniqueNonEmptyStrings(out)
}

func javaTypeTokens(line string) []string {
	line = stripCLikeLineComment(line)
	var out []string
	var cur strings.Builder
	flush := func() {
		token := cur.String()
		cur.Reset()
		if token == "" {
			return
		}
		if isJavaIdentifier(token) && token[0] >= 'A' && token[0] <= 'Z' {
			out = append(out, token)
		}
	}
	for _, r := range line {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '$' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return uniqueNonEmptyStrings(out)
}

func isJavaDottedRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if strings.HasSuffix(ref, ".*") {
		ref = strings.TrimSuffix(ref, ".*")
	}
	if ref == "" {
		return false
	}
	for _, part := range strings.Split(ref, ".") {
		if !isJavaIdentifier(part) {
			return false
		}
	}
	return true
}

func isJavaIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || r == '$') {
				return false
			}
			continue
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '$') {
			return false
		}
	}
	return true
}

func pathBaseSlash(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return ""
	}
	if idx := strings.LastIndex(rel, "/"); idx >= 0 {
		return rel[idx+1:]
	}
	return rel
}

func goProductionPackageCandidates(repoRoot string, changes []types.FileChange) map[string]struct{} {
	out := map[string]struct{}{}
	module := goModulePath(repoRoot)
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".go") || types.LooksLikeTestFilePath(path) {
			continue
		}
		dir := strings.Trim(filepath.ToSlash(filepath.Dir(path)), "/")
		if dir == "." {
			dir = ""
		}
		if dir != "" {
			out[dir] = struct{}{}
		}
		if module != "" {
			if dir == "" {
				out[module] = struct{}{}
			} else {
				out[module+"/"+dir] = struct{}{}
			}
		}
	}
	return out
}

func goImportDeclarations(probe types.VerificationProbe) map[string]struct{} {
	out := map[string]struct{}{}
	inBlock := false
	for _, line := range strings.Split(probe.Code, "\n") {
		line = strings.TrimSpace(stripCLikeLineComment(line))
		if line == "" {
			continue
		}
		if inBlock {
			if strings.HasPrefix(line, ")") {
				inBlock = false
				continue
			}
			addNormalizedQuotedString(line, out, strings.TrimSpace)
			continue
		}
		if !strings.HasPrefix(line, "import") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "import"))
		if strings.HasPrefix(rest, "(") {
			inBlock = true
			addNormalizedQuotedString(strings.TrimSpace(strings.TrimPrefix(rest, "(")), out, strings.TrimSpace)
			continue
		}
		addNormalizedQuotedString(rest, out, strings.TrimSpace)
	}
	return out
}

func goModulePath(repoRoot string) string {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func stripPythonLineComment(line string) string {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' {
			return line[:i]
		}
	}
	return line
}

func stripCLikeLineComment(line string) string {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			continue
		}
		if r == '/' && i+1 < len(line) && line[i+1] == '/' {
			return line[:i]
		}
	}
	return line
}

func collectQuotedCallArgs(line, token string, out map[string]struct{}, normalize func(string) string) {
	for {
		idx := strings.Index(line, token)
		if idx < 0 {
			return
		}
		line = line[idx+len(token):]
		addNormalizedQuotedString(line, out, normalize)
	}
}

func addNormalizedQuotedString(s string, out map[string]struct{}, normalize func(string) string) {
	value, ok := firstQuotedString(s)
	if !ok {
		return
	}
	if normalize != nil {
		value = normalize(value)
	}
	value = strings.TrimSpace(value)
	if value != "" {
		out[value] = struct{}{}
	}
}

func firstQuotedString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		quote := s[i]
		if quote != '\'' && quote != '"' && quote != '`' {
			continue
		}
		var b strings.Builder
		escaped := false
		for j := i + 1; j < len(s); j++ {
			ch := s[j]
			if escaped {
				b.WriteByte(ch)
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				return b.String(), true
			}
			b.WriteByte(ch)
		}
		return "", false
	}
	return "", false
}

func normalizeSlashModuleRef(raw string) string {
	ref := filepath.ToSlash(strings.TrimSpace(raw))
	ref = strings.TrimPrefix(ref, "./")
	for strings.HasPrefix(ref, "../") {
		ref = strings.TrimPrefix(ref, "../")
	}
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".rb"} {
		ref = strings.TrimSuffix(ref, ext)
	}
	ref = strings.TrimSuffix(ref, "/index")
	ref = strings.TrimSuffix(ref, "/__init__")
	return strings.Trim(ref, "/")
}

func firstPythonImportToken(part string) string {
	fields := strings.Fields(strings.TrimSpace(part))
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0])
}

func isPythonModuleName(s string) bool {
	if s == "" || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if !isPythonIdentifier(part) {
			return false
		}
	}
	return true
}

func isPythonIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func pythonImportsCoverAnyTarget(imports, targets map[string]struct{}) bool {
	for target := range targets {
		for imported := range imports {
			if pythonImportCoversTarget(imported, target) {
				return true
			}
		}
	}
	return false
}

func pythonImportCoversTarget(imported, target string) bool {
	imported = strings.TrimSpace(imported)
	target = strings.TrimSpace(target)
	if imported == "" || target == "" {
		return false
	}
	if imported == target || strings.HasPrefix(target, imported+".") {
		return true
	}
	return pythonTopLevelModule(imported) == pythonTopLevelModule(target)
}

func slashModuleRefCoversTarget(ref, target string) bool {
	ref = normalizeSlashModuleRef(ref)
	target = normalizeSlashModuleRef(target)
	if ref == "" || target == "" {
		return false
	}
	return ref == target || strings.HasPrefix(target, ref+"/")
}

func rubyRequireCoversTarget(ref, target string) bool {
	ref = normalizeSlashModuleRef(ref)
	target = normalizeSlashModuleRef(target)
	if ref == "" || target == "" {
		return false
	}
	if ref == target || strings.HasPrefix(target, ref+"/") {
		return true
	}
	return slashTopLevel(ref) == slashTopLevel(target)
}

func goImportCoversTarget(ref, target string) bool {
	ref = strings.TrimSpace(ref)
	target = strings.TrimSpace(target)
	return ref != "" && target != "" && ref == target
}

func slashTopLevel(name string) string {
	name = normalizeSlashModuleRef(name)
	if idx := strings.Index(name, "/"); idx >= 0 {
		return name[:idx]
	}
	return name
}

func pythonImportsCoverRepoLocalPublicPackage(repoRoot string, changes []types.FileChange, imports map[string]struct{}) bool {
	publicPackages := pythonRepoLocalPublicPackagesForChanges(repoRoot, changes)
	if len(publicPackages) == 0 {
		return false
	}
	for imported := range imports {
		top := pythonTopLevelModule(imported)
		if top == "" {
			continue
		}
		if _, ok := publicPackages[top]; ok {
			return true
		}
	}
	return false
}

func pythonRepoLocalPublicPackagesForChanges(repoRoot string, changes []types.FileChange) map[string]struct{} {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	out := map[string]struct{}{}
	seenRoots := map[string]struct{}{}
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") || types.LooksLikeTestFilePath(path) {
			continue
		}
		sourceRootRel := pythonSourceRootForPath(path)
		if _, seen := seenRoots[sourceRootRel]; seen {
			continue
		}
		seenRoots[sourceRootRel] = struct{}{}
		sourceRoot := filepath.Join(repoRoot, filepath.FromSlash(sourceRootRel))
		entries, err := os.ReadDir(sourceRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			stem := strings.TrimSuffix(name, ".py")
			if !isPythonIdentifier(stem) || strings.HasPrefix(stem, "_") {
				continue
			}
			full := filepath.Join(sourceRoot, name)
			switch {
			case entry.IsDir():
				if pythonDirectoryLooksImportable(full) {
					out[name] = struct{}{}
				}
			case strings.HasSuffix(name, ".py"):
				out[stem] = struct{}{}
			}
		}
	}
	return out
}

func pythonSourceRootForPath(relPath string) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relPath)), "./")
	for _, root := range []string{"src", "lib"} {
		if relPath == root || strings.HasPrefix(relPath, root+"/") {
			return root
		}
	}
	return "."
}

func pythonDirectoryLooksImportable(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "__init__.py")); err == nil {
		return true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if strings.HasPrefix(name, "_") {
				continue
			}
			if _, err := os.Stat(filepath.Join(path, name, "__init__.py")); err == nil {
				return true
			}
			continue
		}
		if strings.HasSuffix(name, ".py") && !strings.HasPrefix(name, "_") {
			return true
		}
	}
	return false
}

func pythonTopLevelModule(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, ".") {
		return ""
	}
	if idx := strings.Index(name, "."); idx >= 0 {
		name = name[:idx]
	}
	if !isPythonIdentifier(name) {
		return ""
	}
	return name
}

func formatStringSet(values map[string]struct{}) string {
	if len(values) == 0 {
		return "[]"
	}
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return "[" + strings.Join(items, ", ") + "]"
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validatePlanPatchDuplicateInsertions(changes []types.FileChange) string {
	for _, c := range changes {
		if strings.TrimSpace(c.Kind) != "patch" || strings.TrimSpace(c.Patch) == "" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !duplicateInsertionPathEligible(path) {
			continue
		}
		if block, ok := firstAdjacentDuplicateAddedBlock(c.Patch); ok {
			return fmt.Sprintf(
				"change %q repeats the same added block twice in a row (%d lines, first line %q). "+
					"Adjacent exact duplicate insertion blocks are treated as a planner stutter; remove the duplicate block and re-emit the plan.",
				path, len(block), strings.TrimSpace(block[0]))
		}
	}
	return ""
}

func duplicateInsertionPathEligible(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".kt", ".kts",
		".rs", ".rb", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp",
		".cs", ".swift", ".php", ".scala", ".m", ".mm", ".sh", ".bash",
		".zsh", ".fish":
		return true
	default:
		return false
	}
}

func firstAdjacentDuplicateAddedBlock(patch string) ([]string, bool) {
	var run []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			run = append(run, line[1:])
			continue
		}
		if block, ok := duplicateBlockInAddedRun(run); ok {
			return block, true
		}
		run = run[:0]
	}
	return duplicateBlockInAddedRun(run)
}

func duplicateBlockInAddedRun(run []string) ([]string, bool) {
	if len(run) < 6 {
		return nil, false
	}
	for start := 0; start < len(run); start++ {
		max := (len(run) - start) / 2
		for size := max; size >= 3; size-- {
			left := run[start : start+size]
			right := run[start+size : start+2*size]
			if !sameDuplicateAddedBlockLines(left, right) || !duplicateAddedBlockIsSourceLike(left) {
				continue
			}
			return append([]string(nil), left...), true
		}
	}
	return nil, false
}

func sameDuplicateAddedBlockLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func duplicateAddedBlockIsSourceLike(lines []string) bool {
	nonEmpty := 0
	sourceLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if duplicateInsertedLineIsSourceLike(trimmed) {
			sourceLines++
		}
	}
	return nonEmpty >= 3 && sourceLines >= 2
}

func duplicateInsertedLineIsSourceLike(line string) bool {
	if line == "{" || line == "}" || line == "};" || line == ");" || line == ")," || line == "]" || line == "];" {
		return false
	}
	commentPrefixes := []string{"#", "//", "/*", "*", "--", "<!--"}
	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(line, prefix) {
			return false
		}
	}
	return true
}

func compileStructuredEditPatches(ctx *types.BusContext, changes []types.FileChange) string {
	rej, _ := compileStructuredEditPatchesWithRepair(ctx, "", changes)
	return rej
}

func compileStructuredEditPatchesWithRepair(ctx *types.BusContext, toolName string, changes []types.FileChange) (string, *types.PlanRepairPack) {
	for i := range changes {
		if len(changes[i].Edits) == 0 {
			continue
		}
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			rej := fmt.Sprintf("change %q uses edits[] but no repo root is available to compile them", strings.TrimSpace(changes[i].Path))
			return rej, planRepairPackFromReason(toolName, "structured_edit_repo_root_missing", rej, []string{"$.changes[].edits"}, []string{strings.TrimSpace(changes[i].Path)})
		}
		patch, err := compileStructuredEditsToPatch(ctx.RepoRoot, &changes[i])
		if err != nil {
			msg := enrichStructuredEditReplanDiagnostic(ctx, err.Error())
			var diagErr *structuredEditDiagnosticError
			if errors.As(err, &diagErr) && diagErr != nil {
				annotateStructuredEditRelocationCandidates(ctx, changes, &diagErr.diagnostic)
				return msg, planRepairPackFromStructuredEditDiagnostic(toolName, msg, diagErr.diagnostic)
			}
			return msg, planRepairPackFromReason(toolName, "structured_edit_compile_failed", msg, []string{"$.changes[].edits"}, []string{strings.TrimSpace(changes[i].Path)})
		}
		changes[i].Patch = patch
	}
	return "", nil
}

func enrichStructuredEditReplanDiagnostic(ctx *types.BusContext, msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return msg
	}
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.PipelineStage != types.StagePlan {
		return msg
	}
	hasNoOp := strings.Contains(msg, " is a no-op")
	hasOldTextMismatch := strings.Contains(msg, "old_text mismatch")
	if ctx.Mutable.VerifyFailureHandoff() != nil {
		if !hasNoOp && !(hasOldTextMismatch && structuredEditReplanProbePassed(ctx)) {
			return msg
		}
		return msg + ". In a verify-failure replan, a no-op edit means the applied worktree may already contain the intended code. Run a typed planner probe with run_tests(dry_run=true, verification_probe={...}) against the scoped failure; if it passes, emit changes: [] to record the no_change_required sentinel. If it fails, re-read the current bytes and emit a real non-no-op edit."
	}
	if _, ok := activeProofFollowupWorkflowBatch(ctx.Mutable.WriteWorkflowRun()); ok && hasNoOp {
		return msg + ". In a proof-follow-up batch, a no-op edit means the already-applied worktree may already satisfy the proof target. Emit changes: [] with verification_probes[] that import or execute the current code and bind the typed proof criteria; do not add comments, whitespace, or full-file rewrites merely to satisfy changes[]."
	}
	if !hasNoOp && !(hasOldTextMismatch && structuredEditReplanProbePassed(ctx)) {
		return msg
	}
	return msg
}

func structuredEditReplanProbePassed(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	return writeflow.QualifyNoChangeReplanSentinel(writeflow.NoChangeReplanQualificationInput{
		VerifyFailureHandoff: ctx.Mutable.VerifyFailureHandoff(),
		PlannerProbeReports:  ctx.Mutable.PlanStageProbeReports(),
	}).Allowed
}

// composePatchRejectionReason mirrors composePatchRejection but
// returns the bare reason string (without the "emit_change_plan
// rejected:" prefix) so the caller can prepend whichever tool name
// applies. Originally composePatchRejection always emitted the
// emit_change_plan prefix; for the structural path we want
// emit_plan_change to surface the rejection under its own name.
func composePatchRejectionReason(repoRoot, path, gitErr, patchPayload string) string {
	full := composePatchRejection(repoRoot, path, gitErr, patchPayload)
	const prefix = "emit_change_plan rejected: "
	if strings.HasPrefix(full, prefix) {
		return full[len(prefix):]
	}
	return full
}

// newChangePlanFromChanges assembles the canonical ChangePlan struct
// from a request, summary, validated changes slice, and acceptance
// tests. ID format and timestamp logic match the legacy inline
// implementation in emit_change_plan.Execute exactly so disk-side
// plan files stay byte-shape compatible.
//
// Reused by emit_plan_change when promoting PartialChangePlan to
// ChangePlan — same factory, same shape.
func newChangePlanFromChanges(request, summary string, changes []types.FileChange, acceptanceTests []string, verificationProbes []types.VerificationProbe) *types.ChangePlan {
	now := time.Now()
	plan := &types.ChangePlan{
		ID:        fmt.Sprintf("plan-%d-%d", now.UnixNano(), os.Getpid()),
		Request:   request,
		Summary:   summary,
		Status:    types.PlanStatusPending,
		CreatedAt: now,
		Changes:   append([]types.FileChange(nil), changes...),
	}
	paths := make(map[string]struct{}, len(changes))
	for _, c := range changes {
		paths[c.Path] = struct{}{}
	}
	for p := range paths {
		plan.TargetPaths = append(plan.TargetPaths, p)
	}
	if len(acceptanceTests) > 0 {
		plan.AcceptanceTests = append([]string(nil), acceptanceTests...)
	}
	if len(verificationProbes) > 0 {
		plan.VerificationProbes = append([]types.VerificationProbe(nil), verificationProbes...)
	}
	return plan
}

func attachWriteBehaviorContracts(ctx *types.BusContext, plan *types.ChangePlan) {
	if ctx == nil || ctx.Mutable == nil || plan == nil {
		return
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil || len(ir.Request.BehaviorContracts) == 0 {
		return
	}
	plan.BehaviorContracts = append([]types.WriteBehaviorContract(nil), ir.Request.BehaviorContracts...)
}

func enrichVerificationProbeRefs(plan *types.ChangePlan) {
	if plan == nil || len(plan.VerificationProbes) != 1 {
		return
	}
	probe := &plan.VerificationProbes[0]
	contracts := probeCoverageContractRefs(plan.BehaviorContracts)
	if len(probe.ContractRefs) == 0 && len(contracts) == 1 {
		probe.ContractRefs = []string{contracts[0].ID}
	}
	if len(probe.ChangedSymbolRefs) == 0 {
		if refs := probeCoverageChangedSymbolRefs(plan, contracts); len(refs) > 0 {
			probe.ChangedSymbolRefs = refs
		}
	}
}

func probeCoverageContractRefs(contracts []types.WriteBehaviorContract) []types.WriteBehaviorContract {
	var out []types.WriteBehaviorContract
	for _, contract := range contracts {
		if !contract.Required || strings.TrimSpace(contract.ID) == "" {
			continue
		}
		if contract.Polarity == types.WriteBehaviorPolarityObserved {
			continue
		}
		out = append(out, contract)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.TrimSpace(out[i].ID) < strings.TrimSpace(out[j].ID)
	})
	return out
}

func probeCoverageChangedSymbolRefs(plan *types.ChangePlan, contracts []types.WriteBehaviorContract) []string {
	if len(contracts) != 1 {
		return nil
	}
	if subject := strings.TrimSpace(contracts[0].Subject); subject != "" {
		return []string{subject}
	}
	if len(plan.TargetPaths) != 1 {
		return nil
	}
	path := strings.TrimSpace(plan.TargetPaths[0])
	if path == "" || types.LooksLikeTestFilePath(path) {
		return nil
	}
	return []string{"path:" + filepath.ToSlash(path)}
}

func normalizeVerificationProbes(in []types.VerificationProbe) ([]types.VerificationProbe, string) {
	return normalizeVerificationProbesWithOptions(in, true)
}

func normalizePlannerDryRunVerificationProbe(in []types.VerificationProbe) ([]types.VerificationProbe, string) {
	return normalizeVerificationProbesWithOptions(in, false)
}

func normalizeVerificationProbesWithOptions(in []types.VerificationProbe, requireFailureSignal bool) ([]types.VerificationProbe, string) {
	const (
		maxProbes            = 5
		maxProbeCodeBytes    = 8 * 1024
		maxExpectedFragments = 10
		defaultProbeTimeout  = 10
		maxProbeTimeout      = 30
	)
	if len(in) == 0 {
		return nil, ""
	}
	if len(in) > maxProbes {
		return nil, fmt.Sprintf("verification_probes has %d entries; maximum is %d", len(in), maxProbes)
	}
	out := make([]types.VerificationProbe, 0, len(in))
	seen := map[string]struct{}{}
	for i, probe := range in {
		language, ok := normalizeVerificationProbeLanguage(probe.Language)
		if !ok {
			return nil, fmt.Sprintf("verification_probes[%d].language=%q is unsupported; supported values: %s", i, probe.Language, supportedVerificationProbeLanguageList())
		}
		code := strings.TrimSpace(probe.Code)
		if code == "" {
			return nil, fmt.Sprintf("verification_probes[%d].code is required", i)
		}
		if len(code) > maxProbeCodeBytes {
			return nil, fmt.Sprintf("verification_probes[%d].code is %d bytes; maximum is %d", i, len(code), maxProbeCodeBytes)
		}
		id := strings.TrimSpace(probe.ID)
		if id == "" {
			id = fmt.Sprintf("probe-%d", i+1)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Sprintf("verification_probes has duplicate id %q", id)
		}
		seen[id] = struct{}{}
		workingDir := strings.TrimSpace(probe.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		workingDir = filepath.Clean(workingDir)
		if filepath.IsAbs(workingDir) {
			return nil, fmt.Sprintf("verification_probes[%d].working_dir=%q must be repo-relative", i, probe.WorkingDir)
		}
		for _, seg := range strings.Split(filepath.ToSlash(workingDir), "/") {
			if seg == ".." {
				return nil, fmt.Sprintf("verification_probes[%d].working_dir=%q escapes the repository", i, probe.WorkingDir)
			}
		}
		workingDir = filepath.ToSlash(workingDir)
		timeout := probe.TimeoutSeconds
		if timeout <= 0 {
			timeout = defaultProbeTimeout
		}
		if timeout > maxProbeTimeout {
			timeout = maxProbeTimeout
		}
		expected := make([]string, 0, len(probe.ExpectedStdout))
		for _, fragment := range probe.ExpectedStdout {
			fragment = strings.TrimSpace(fragment)
			if fragment != "" {
				expected = append(expected, fragment)
			}
			if len(expected) > maxExpectedFragments {
				return nil, fmt.Sprintf("verification_probes[%d].expected_stdout has more than %d non-empty entries", i, maxExpectedFragments)
			}
		}
		if requireFailureSignal && len(expected) == 0 && !verificationProbeHasExecutableFailureSignal(language, code) {
			return nil, fmt.Sprintf("verification_probes[%d].code must include a %s executable failure signal or expected_stdout; printing failure text without a non-zero exit path can falsely pass verification", i, language)
		}
		contractRefs, rej := normalizeProbeStringRefs(probe.ContractRefs, maxExpectedFragments, fmt.Sprintf("verification_probes[%d].contract_refs", i))
		if rej != "" {
			return nil, rej
		}
		placementRefs, rej := normalizeProbeStringRefs(probe.PlacementRefs, maxExpectedFragments, fmt.Sprintf("verification_probes[%d].placement_refs", i))
		if rej != "" {
			return nil, rej
		}
		changedSymbolRefs, rej := normalizeProbeStringRefs(probe.ChangedSymbolRefs, maxExpectedFragments, fmt.Sprintf("verification_probes[%d].changed_symbol_refs", i))
		if rej != "" {
			return nil, rej
		}
		out = append(out, types.VerificationProbe{
			ID:                     id,
			Language:               language,
			WorkingDir:             workingDir,
			Code:                   code,
			TimeoutSeconds:         timeout,
			ExpectedStdout:         expected,
			ContractRefs:           contractRefs,
			PlacementRefs:          placementRefs,
			ChangedSymbolRefs:      changedSymbolRefs,
			ExpectsBaselineFailure: probe.ExpectsBaselineFailure,
		})
	}
	return out, ""
}

func normalizeProbeStringRefs(in []string, max int, field string) ([]string, string) {
	if len(in) == 0 {
		return nil, ""
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, ref := range in {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
		if len(out) > max {
			return nil, fmt.Sprintf("%s has more than %d non-empty entries", field, max)
		}
	}
	return out, ""
}

func validateVerificationProbeContractRefs(ctx *types.BusContext, probes []types.VerificationProbe) string {
	if len(probes) == 0 || ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	contracts := ir.Request.BehaviorContracts
	if len(contracts) == 0 {
		return ""
	}
	ids := types.WriteBehaviorContractIDs(contracts)
	placementIDs := types.PlacementRequiredWriteBehaviorContractIDs(contracts)
	for i, probe := range probes {
		for _, ref := range probe.ContractRefs {
			if _, ok := ids[ref]; !ok {
				return fmt.Sprintf("verification_probes[%d].contract_refs contains unknown behavior_contract id %q; use one of %s", i, ref, formatStringSet(ids))
			}
		}
		for _, ref := range probe.PlacementRefs {
			if _, ok := ids[ref]; !ok {
				return fmt.Sprintf("verification_probes[%d].placement_refs contains unknown behavior_contract id %q; use one of %s", i, ref, formatStringSet(ids))
			}
			if _, ok := placementIDs[ref]; !ok {
				return fmt.Sprintf("verification_probes[%d].placement_refs contains behavior_contract id %q without placement{}; use one of %s", i, ref, formatStringSet(placementIDs))
			}
		}
	}
	return ""
}

func subtractStringSet(want, got map[string]struct{}) map[string]struct{} {
	if len(want) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for key := range want {
		if _, ok := got[key]; ok {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func pythonVerificationProbeHasExecutableFailureSignal(code string) bool {
	stripped := stripPythonProbeStringsAndComments(code)
	for _, ident := range pythonProbeIdentifiers(stripped) {
		switch ident {
		case "assert", "raise":
			return true
		}
	}
	compact := compactPythonProbeSignalSurface(stripped)
	for _, signal := range []string{
		"sys.exit(",
		"exit(",
		"pytest.fail(",
	} {
		if strings.Contains(compact, signal) {
			return true
		}
	}
	return false
}

func stripPythonProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	triple := false
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if !triple {
					inString = false
				}
				escaped = false
				continue
			}
			b.WriteByte(' ')
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				if triple {
					if i+2 < len(src) && src[i+1] == quote && src[i+2] == quote {
						b.WriteString("  ")
						i += 2
						inString = false
						triple = false
					}
				} else {
					inString = false
				}
			}
			continue
		}
		if ch == '#' {
			for i < len(src) && src[i] != '\n' {
				b.WriteByte(' ')
				i++
			}
			if i < len(src) && src[i] == '\n' {
				b.WriteByte('\n')
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inString = true
			quote = ch
			triple = i+2 < len(src) && src[i+1] == ch && src[i+2] == ch
			b.WriteByte(' ')
			if triple {
				b.WriteString("  ")
				i += 2
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func pythonProbeIdentifiers(src string) []string {
	var out []string
	for i := 0; i < len(src); {
		r := rune(src[i])
		if !isPythonIdentifierStart(r) {
			i++
			continue
		}
		start := i
		i++
		for i < len(src) && isPythonIdentifierPart(rune(src[i])) {
			i++
		}
		out = append(out, src[start:i])
	}
	return out
}

func compactPythonProbeSignalSurface(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if isPythonProbeASCIILetter(ch) || isPythonProbeASCIIDigit(ch) || ch == '_' || ch == '.' || ch == '(' {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func isPythonIdentifierStart(r rune) bool {
	return r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func isPythonIdentifierPart(r rune) bool {
	return isPythonIdentifierStart(r) || ('0' <= r && r <= '9')
}

func isPythonProbeASCIILetter(ch byte) bool {
	return ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isPythonProbeASCIIDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
