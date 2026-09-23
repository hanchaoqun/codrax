package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// planScopeEditingPolicy keeps hard admission and advisory repair on the same
// typed scope. Absence is not authority for a new hard gate, but also must not
// encourage a whole-file overwrite without an explicit wider classification.
func planScopeEditingPolicy(ctx *types.BusContext) (forbidModify, suggestModify bool) {
	if ctx == nil || ctx.Mutable == nil {
		return false, false
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return false, false
	}
	switch ir.Request.Task.Scope {
	case types.ScopeMicro:
		return true, false
	case types.ScopePackage, types.ScopeCross, types.ScopeProject:
		return false, true
	default:
		return false, false
	}
}

// createPathExistsRepair describes only this physical path in AcceptedEnums.
// An alternate new path is a separate choice, not permission to retry create on
// the same existing file. The stat snapshot is shared with the rejecting gate.
func createPathExistsRepair(ctx *types.BusContext, toolName, path string, info os.FileInfo) (string, *types.PlanRepairPack) {
	state := "non-regular path"
	var kinds []string
	guidance := "choose a verified regular-file path before selecting a content edit"
	if info.IsDir() {
		state = "directory"
	} else if info.Mode().IsRegular() {
		state = "file"
		kinds = []string{"patch"}
		_, suggestModify := planScopeEditingPolicy(ctx)
		if suggestModify {
			kinds = append(kinds, "modify")
		}
		guidance = "current-path repair options are kind=" + strings.Join(kinds, "/") + "; prefer a localized patch"
		if info.Size() == 0 {
			guidance += `; this file is empty: use kind=patch with edits=[{"kind":"insert_at_eof","content":"..."}] without inventing a source line`
		}
	}
	rej := fmt.Sprintf("change %q has kind=create but that %s already exists; %s. Separately, kind=create requires a different, verified new-file path within the authorized task", path, state, guidance)
	pack := planRepairPackFromReason(toolName, "create_path_exists", rej, []string{"$.changes[].kind", "$.changes[].path"}, []string{path})
	if len(kinds) > 0 {
		pack.AcceptedEnums = map[string][]string{"$.changes[].kind": kinds}
	}
	pack.RetryInstruction = "Re-emit emit_change_plan with the corrected path/kind and matching content. Accepted kind options apply only to the failing current path, not to other entries or an alternate new path."
	if toolName == "emit_plan_change" {
		pack.RetryInstruction = "Re-emit emit_plan_skeleton for the full plan with the corrected path/kind, then refill all required bodies with emit_plan_change. The retained plan is unchanged; emit_plan_change alone cannot change a slot's path/kind."
	}
	normalized := types.NormalizePlanRepairPack(*pack)
	return rej, &normalized
}

func planPathState(rootAbs, repoRel string) (info os.FileInfo, statErr error, ok bool) {
	if strings.TrimSpace(rootAbs) == "" || strings.TrimSpace(repoRel) == "" {
		return nil, os.ErrNotExist, false
	}
	abs := filepath.Join(rootAbs, filepath.FromSlash(repoRel))
	abs, err := filepath.Abs(abs)
	if err != nil {
		return nil, err, true
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return nil, err, false
	}
	info, err = os.Stat(abs)
	return info, err, true
}
