package tool

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
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
	seenPaths := make(map[string]int, len(changes))
	for i, c := range changes {
		path := strings.TrimSpace(c.Path)
		if path == "" {
			return "one of the changes has an empty path"
		}
		if !isLegalChangeKind(strings.TrimSpace(c.Kind)) {
			return fmt.Sprintf("change %q has illegal kind %q (must be create|modify|delete|patch|rename)", path, c.Kind)
		}
		if _, dup := seenPaths[path]; dup {
			return fmt.Sprintf("duplicate change for path %q (one-change-per-file constraint; combine into a single FileChange)", path)
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
				return fmt.Sprintf("change %q has kind=rename but new_path is empty", path)
			}
			if newPath == path {
				return fmt.Sprintf("change %q has kind=rename with new_path equal to path; remove the rename or pick a different destination", path)
			}
			if _, collision := seenPaths[newPath]; collision {
				return fmt.Sprintf("change %q rename destination %q collides with another change in this plan (one path per plan, even across rename)", path, newPath)
			}
		default:
			if newPath != "" {
				return fmt.Sprintf("change %q has kind=%s but new_path is set (only kind=rename uses new_path)", path, kind)
			}
		}
	}
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		for _, dep := range c.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				return fmt.Sprintf("change %q has an empty depends_on entry", path)
			}
			if dep == path {
				return fmt.Sprintf("change %q depends_on itself", path)
			}
			if _, ok := seenPaths[dep]; !ok {
				return fmt.Sprintf("change %q depends_on %q but %q is not in changes[]", path, dep, dep)
			}
		}
	}
	return ""
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
// path + the WORKED EXAMPLE pointer so the LLM can fix without
// guessing the unified diff format.
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
			"task.scope=micro forbids kind=modify on %q — micro-scope tasks (one function / one constant / one line in one file, per the analyzer's classification) MUST use kind=patch with a unified diff. "+
				"A whole-file overwrite for a one-line edit collapses the diff the user reviews. "+
				"Re-emit this change with kind=patch and a unified diff that touches only the affected line(s); "+
				"the change-plan-skill prompt's WORKED EXAMPLE shows the exact format (--- / +++ headers, @@ hunk header with line counts, ' '/'-'/'+' line prefixes, byte-for-byte context match including tabs). "+
				"Carve-outs: kind=create (new file) / kind=delete (removal) / kind=rename (pure move) bypass this gate; only kind=modify-on-existing-file is rejected.",
			path)
	}
	return ""
}

func validatePlanFullContent(ctx *types.BusContext, summary string, changes []types.FileChange) string {
	if GitAvailable() && ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		for _, c := range changes {
			if strings.TrimSpace(c.Kind) != "patch" {
				continue
			}
			if strings.TrimSpace(c.Patch) == "" {
				return fmt.Sprintf("change %q has kind=patch but Patch is empty (unified-diff required)", strings.TrimSpace(c.Path))
			}
			if err := CheckUnifiedDiff(ctx.RepoRoot, c.Patch); err != nil {
				return composePatchRejectionReason(ctx.RepoRoot, strings.TrimSpace(c.Path), err.Error(), c.Patch)
			}
		}
	}
	if rej := validatePlanDepsClosure(ctx.RepoRoot, changes); rej != "" {
		return rej
	}
	if rej := validatePlanWiringClosure(changes); rej != "" {
		return rej
	}
	if rej := validatePlanSummaryConsistency(summary, changes); rej != "" {
		return rej
	}
	if rej := validatePlanDryBuild(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePlanLint(ctx, changes); rej != "" {
		return rej
	}
	if rej := validatePlanProjectLint(ctx, changes); rej != "" {
		return rej
	}
	return ""
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
func newChangePlanFromChanges(request, summary string, changes []types.FileChange, acceptanceTests []string) *types.ChangePlan {
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
	return plan
}
