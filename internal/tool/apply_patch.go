package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyPatch is the apply-stage's structured write tool. B1 ships
// real implementations for kind=create / modify / delete; kind=patch
// remains a fail-loud stub pending B2's unified-diff apply path
// (git-apply integration).
//
// Control flow per successful apply:
//  1. Decode params (strict: DisallowUnknownFields).
//  2. Look up ChangeUnit in Mutable.ChangePlan by path.
//  3. Enforce W1: params.Path must appear in plan.TargetPaths.
//  4. Enforce W1b: every unit.DependsOn must already be in
//     WriteClosure.AppliedSet — rejects out-of-order apply calls.
//  5. Idempotency: if path is already in AppliedSet, return success
//     without touching the filesystem (LLM re-emission is a no-op).
//  6. File I/O against ctx.RepoRoot (which runApplyPhase swapped to
//     the worktree directory). create / modify write a full body;
//     delete removes the file; patch returns "not implemented in B1".
//  7. On success, WriteClosure.MarkApplied(path) so subsequent W1b
//     checks on units that depend on this one pass.
//
// Red-line discipline (L3): Execute MUST NOT invoke ground.BuildContext
// or ground.GroundItem — apply is a write path, not a citation path.
// Enforced by internal/tool/write_mode_red_lines_test.go.
//
// Classified WriteCapable + NonEvidenceTool: IsWrite() returns true
// so read-mode skills that gate on it never expose apply_patch to
// the LLM.
type ApplyPatch struct {
	WriteCapable
	NonEvidenceTool
}

// applyPatchParams mirrors the tool's JSON schema. DisallowUnknownFields
// in Execute prevents schema drift.
type applyPatchParams struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	NewContent string `json:"new_content,omitempty"`
	Patch      string `json:"patch,omitempty"`
}

// Name returns the tool's stable identifier.
func (t *ApplyPatch) Name() string { return "apply_patch" }

// Description — one sentence + B1 scope note so operators reading
// tool logs know the kind=patch stub boundary.
func (t *ApplyPatch) Description() string {
	return "Apply a single ChangeUnit from the active ChangePlan to the worktree. " +
		"Supports kind=create|modify|delete in B1; kind=patch returns 'not yet implemented' until B2."
}

// Parameters returns the tool's JSON schema. Shape unchanged from
// the B0 stub so the coder agent's skill can declare it without
// tracking B1-vs-B0 differences.
func (t *ApplyPatch) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "path": {
      "type": "string",
      "description": "Repo-relative file path to modify (must be declared in plan.target_paths; W1 invariant)."
    },
    "kind": {
      "type": "string",
      "enum": ["create", "modify", "delete", "patch"],
      "description": "Change type; must match the ChangeUnit's declared Kind."
    },
    "new_content": {
      "type": "string",
      "description": "Full file body for kind=create|modify. Ignored for kind=delete; rejected for kind=patch."
    },
    "patch": {
      "type": "string",
      "description": "Unified-diff payload for kind=patch. Not yet implemented in B1 — use kind=create|modify|delete for the apply path instead."
    }
  },
  "required": ["path", "kind"]
}`)
}

// Execute applies one ChangeUnit. See the package doc above for the
// full control flow.
func (t *ApplyPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "apply_patch requires BusContext.Mutable"), nil
	}

	// Strict decode so any schema drift (LLM emits a new field)
	// fails loud instead of silently losing data.
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p applyPatchParams
	if err := dec.Decode(&p); err != nil {
		return errResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
	}

	path := strings.TrimSpace(p.Path)
	if path == "" {
		return errResult(t.Name(), "apply_patch rejected: empty path"), nil
	}
	kind := strings.TrimSpace(p.Kind)
	if !isLegalChangeKind(kind) {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: illegal kind %q (must be create|modify|delete|patch)", kind)), nil
	}

	// Plan must be installed on Mutable before apply stage runs.
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return errResult(t.Name(),
			"apply_patch rejected: no ChangePlan on Mutable — orchestrator's runApplyPhase did not load one. "+
				"This tool is apply-stage only; calling it from another stage is a skill-configuration bug."), nil
	}

	// W1: path must be in plan.TargetPaths.
	if !containsPath(plan.TargetPaths, path) {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: path %q is not in plan.TargetPaths (W1 violation; "+
				"the planner must have emitted a ChangeUnit for this path before the coder may apply it)", path)), nil
	}

	// Find the matching ChangeUnit for the path. Constant lookup
	// is fine for B1 plans (≤20 changes typical).
	var unit *types.FileChange
	for i := range plan.Changes {
		if plan.Changes[i].Path == path {
			unit = &plan.Changes[i]
			break
		}
	}
	if unit == nil {
		// Should be impossible given W1 passed (TargetPaths is
		// derived from Changes) but belt-and-suspenders.
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch internal error: path %q in TargetPaths but missing from Changes[]", path)), nil
	}

	// Kind consistency: the LLM's emitted kind must match the
	// plan's declared kind. Drift here would let a delete hide
	// as a modify (or vice versa).
	if unit.Kind != kind {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: kind mismatch for %q — plan declares %q but tool call sent %q",
				path, unit.Kind, kind)), nil
	}

	// W1b: every DependsOn must already be applied. The apply
	// stage's coder agent is responsible for emitting calls in
	// topological order, but the tool re-enforces so a bug in the
	// ordering logic surfaces as a clean error rather than a
	// broken build.
	applied := ctx.Mutable.WriteClosure().AppliedSet()
	for _, dep := range unit.DependsOn {
		if !applied[dep] {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: %q depends_on %q but %q has not been applied yet "+
					"(W1b ordering violation; apply %q first)", path, dep, dep, dep)), nil
		}
	}

	// Idempotency: a second call for the same path returns success
	// without touching the filesystem. The LLM may re-emit the
	// same apply_patch call (e.g. after a prior tool error forced
	// a retry prompt); we want that to be a no-op rather than
	// compounding state.
	if applied[path] {
		return okResult(t.Name(),
			fmt.Sprintf("apply_patch: %q already applied (idempotent no-op)", path)), nil
	}

	// Patch kind — B1 stub, B2 implements via `git apply`.
	if kind == "patch" {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: kind=patch not yet implemented in B1 (path %q). "+
				"Use kind=modify with full file body as a temporary workaround, or wait for B2.",
				path)), nil
	}

	// Resolve absolute path against the swapped RepoRoot. Day-2
	// audit confirmed runApplyPhase has set ctx.RepoRoot to the
	// worktree checkout, so every write below lands inside the
	// dry-run copy, not the main repo.
	if ctx.RepoRoot == "" {
		return errResult(t.Name(),
			"apply_patch rejected: ctx.RepoRoot is empty — the orchestrator must swap to a worktree before apply"), nil
	}
	absPath := filepath.Join(ctx.RepoRoot, path)
	// Bolt-and-suspenders: refuse any path that escapes RepoRoot
	// via .. traversal. filepath.Clean neutralises redundant
	// segments; we then check the result is still within RepoRoot.
	absPath = filepath.Clean(absPath)
	absRoot, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		return errResult(t.Name(), fmt.Sprintf("apply_patch: abs(RepoRoot): %v", err)), err
	}
	absPathCanonical, err := filepath.Abs(absPath)
	if err != nil {
		return errResult(t.Name(), fmt.Sprintf("apply_patch: abs(path): %v", err)), err
	}
	if !strings.HasPrefix(absPathCanonical, absRoot+string(filepath.Separator)) && absPathCanonical != absRoot {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: path %q escapes RepoRoot via traversal", path)), nil
	}

	// Kind-specific file I/O.
	switch kind {
	case "create":
		// create: the file must not already exist. Rejecting
		// clobber here surfaces planner bugs (planner emitted
		// create for a path that already exists) cleanly.
		if _, err := os.Stat(absPathCanonical); err == nil {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: create %q failed — file already exists in worktree", path)), nil
		} else if !os.IsNotExist(err) {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: stat %s: %v", path, err)), err
		}
		if err := os.MkdirAll(filepath.Dir(absPathCanonical), 0o755); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: mkdir parent of %s: %v", path, err)), err
		}
		if err := os.WriteFile(absPathCanonical, []byte(p.NewContent), 0o644); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: write %s: %v", path, err)), err
		}
	case "modify":
		// modify: file should exist. A plan that emits modify for
		// a missing file is likely confused (meant create) —
		// reject instead of silently doing create's job.
		if _, err := os.Stat(absPathCanonical); err != nil {
			if os.IsNotExist(err) {
				return errResult(t.Name(),
					fmt.Sprintf("apply_patch rejected: modify %q failed — file does not exist. "+
						"Use kind=create for new files.", path)), nil
			}
			return errResult(t.Name(), fmt.Sprintf("apply_patch: stat %s: %v", path, err)), err
		}
		if err := os.WriteFile(absPathCanonical, []byte(p.NewContent), 0o644); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: write %s: %v", path, err)), err
		}
	case "delete":
		// delete: file should exist. Missing-file delete is legal
		// (idempotency) but we log so a planner bug doesn't hide.
		if _, err := os.Stat(absPathCanonical); err != nil {
			if os.IsNotExist(err) {
				logging.Warning("[apply_patch] delete %s: file already absent (idempotent)", path)
			} else {
				return errResult(t.Name(), fmt.Sprintf("apply_patch: stat %s: %v", path, err)), err
			}
		} else if err := os.Remove(absPathCanonical); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: remove %s: %v", path, err)), err
		}
	default:
		// Unreachable: isLegalChangeKind + patch branch above cover
		// every enum value.
		return errResult(t.Name(), fmt.Sprintf("apply_patch: internal unhandled kind %q", kind)), nil
	}

	// Success: mark the path applied so subsequent units that
	// depend on this one can proceed.
	ctx.Mutable.WriteClosure().MarkApplied(path)
	logging.Info("[apply_patch] %s %s (worktree=%s)", kind, path, ctx.RepoRoot)
	return okResult(t.Name(),
		fmt.Sprintf("apply_patch: %s %q applied to worktree", kind, path)), nil
}

// containsPath returns true when needle appears in haystack.
// O(N) linear scan — B1 plans are small (≤20 paths typical).
func containsPath(haystack []string, needle string) bool {
	for _, p := range haystack {
		if p == needle {
			return true
		}
	}
	return false
}

// errResult is the tool-package-standard failure result builder.
// Stays private here because other write-mode tools have their
// own shape conventions (emit_change_plan / run_tests / ...).
func errResult(name, summary string) types.ToolResult {
	return types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

// okResult mirrors errResult for the happy path.
func okResult(name, summary string) types.ToolResult {
	return types.ToolResult{
		ToolName:  name,
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}
