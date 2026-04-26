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
			return fmt.Sprintf("change %q has illegal kind %q (must be create|modify|delete|patch)", path, c.Kind)
		}
		if _, dup := seenPaths[path]; dup {
			return fmt.Sprintf("duplicate change for path %q (one-change-per-file constraint; combine into a single FileChange)", path)
		}
		seenPaths[path] = i
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
				return composePatchRejectionReason(ctx.RepoRoot, strings.TrimSpace(c.Path), err.Error())
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
func composePatchRejectionReason(repoRoot, path, gitErr string) string {
	full := composePatchRejection(repoRoot, path, gitErr)
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
