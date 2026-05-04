package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyPatch is the apply-stage's structured write tool. Supports
// all four change kinds: create / modify (full body), delete, and
// patch (unified diff piped through `git apply` inside the worktree).
//
// Control flow per successful apply:
//  1. Decode params (strict: DisallowUnknownFields).
//  2. Look up ChangeUnit in Mutable.ChangePlan by path.
//  3. Enforce W1: params.Path must appear in plan.TargetPaths.
//  4. Enforce W1b: every unit.DependsOn must already be in
//     WriteClosure.AppliedSet — rejects out-of-order apply calls.
//  5. Idempotency: if path is already in AppliedSet, return success
//     without touching the filesystem (LLM re-emission is a no-op).
//  6. File I/O against ctx.RepoRoot (which the apply stage hook swapped to
//     the worktree directory). create / modify write a full body;
//     delete removes the file; patch feeds `git apply -` a unified
//     diff.
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
//
// Content fields (`new_content` / `patch`) were dropped in the session-
// 35 hardening. The plan's canonical copy lives on
// Mutable.ChangePlan().Changes[i] and the tool now hydrates from it
// directly — this eliminates LLM transcription errors (the sentinel
// failure was a 1-byte `\n` loss on `func main() {` that crashed
// `git apply` with "corrupt patch at line 11" even though the plan
// contained a byte-perfect diff). Schema is now {path, kind} only.
type applyPatchParams struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// Name returns the tool's stable identifier.
func (t *ApplyPatch) Name() string { return "apply_patch" }

// Description — one sentence scope line that matches the current
// four-way dispatch (create / modify / delete / patch).
func (t *ApplyPatch) Description() string {
	return "Apply a single ChangeUnit from the active ChangePlan to the worktree. " +
		"Supports kind=create|modify|delete|patch. For patch, a unified-diff payload is piped to `git apply`."
}

// Parameters returns the tool's JSON schema. Only {path, kind} —
// content (new_content / patch) is sourced from the authoritative
// Mutable.ChangePlan() slot so the LLM cannot introduce
// transcription errors. See applyPatchParams docblock for the
// session-35 failure that motivated the cut.
func (t *ApplyPatch) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "path": {
      "type": "string",
      "description": "Repo-relative file path to apply. Must be declared in plan.target_paths; the tool hydrates the corresponding ChangeUnit (new_content / patch / delete flag) from Mutable.ChangePlan() — do NOT emit content fields, the schema rejects them."
    },
    "kind": {
      "type": "string",
      "enum": ["create", "modify", "delete", "patch"],
      "description": "Change type; must match the ChangeUnit's declared Kind (sanity check — the tool compares against plan.Changes[path].Kind)."
    }
  },
  "required": ["path", "kind"]
}`)
}

// Execute applies one ChangeUnit. See the package doc above for the
// full control flow.
func (t *ApplyPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "apply_patch requires a writable run context (the orchestrator did not provide one)"), nil
	}

	// Strict decode so any schema drift (LLM emits a new field)
	// fails loud instead of silently losing data.
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p applyPatchParams
	err := dec.Decode(&p)
	if err != nil {
		err = RemapStrictDecodeError(err, nil)
		return errResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
	}

	path := strings.TrimSpace(p.Path)
	if path == "" {
		return errResult(t.Name(), "apply_patch rejected: empty path"), nil
	}
	kind := strings.TrimSpace(p.Kind)
	if !isLegalChangeKind(kind) {
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: illegal kind %q (must be create|modify|delete|patch|rename)", kind)), nil
	}

	// Plan must be installed on Mutable before apply stage runs.
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return errResult(t.Name(),
			"apply_patch rejected: no ChangePlan on Mutable — orchestrator's the apply stage hook did not load one. "+
				"This tool is apply-stage only; calling it from another stage is a skill-configuration bug."), nil
	}

	// W1: path must be in plan.TargetPaths. Message enumerates the
	// valid TargetPaths so the LLM doesn't have to re-read the plan
	// or guess — the plan's declared set is exhausively listed and
	// the corrective action ("pick one of these") is mechanical.
	if !containsPath(plan.TargetPaths, path) {
		valid := "none"
		if len(plan.TargetPaths) > 0 {
			valid = strings.Join(plan.TargetPaths, ", ")
		}
		// Bump the per-path consecutive-rejection counter. Coder
		// ShouldStop reads SaturatedRejectionPath() to detect when
		// the LLM is fixated on a path the planner didn't include —
		// at that point the right escalation is "tell the planner
		// to widen TargetPaths" (verify→plan retry hint), not "let
		// coder churn through more iterations".
		ctx.Mutable.WriteClosure().RecordRejection(path)
		return errResult(t.Name(),
			fmt.Sprintf("apply_patch rejected: path %q is not in plan.TargetPaths (W1 violation). "+
				"Valid target paths for this plan: [%s]. Retry apply_patch with one of the listed paths. Do not retry the same %q — the planner did not include it; if a path you need is missing, stop calling apply_patch and let the verify→plan retry surface widen TargetPaths.",
				path, valid, path)), nil
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
	// broken build. Message enumerates the applied set so the LLM
	// sees exactly what's unblocked — no guessing which call to
	// retry first.
	applied := ctx.Mutable.WriteClosure().AppliedSet()
	for _, dep := range unit.DependsOn {
		if !applied[dep] {
			var appliedList []string
			for p := range applied {
				appliedList = append(appliedList, p)
			}
			appliedStr := "none"
			if len(appliedList) > 0 {
				appliedStr = strings.Join(appliedList, ", ")
			}
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: %q depends_on %q but %q has not been applied yet (W1b ordering violation). "+
					"Currently-applied paths: [%s]. Apply %q first, then retry %q.",
					path, dep, dep, appliedStr, dep, path)), nil
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

	// Patch kind — B2: pipe the unified diff into `git apply -` inside
	// the worktree. Path-level W1/W1b/idempotency already passed; git
	// apply additionally validates the hunks match the current file
	// content and refuses to partially-apply on conflict.
	//
	// Source of truth is unit.Patch (the planner's emitted diff),
	// NOT an LLM-supplied parameter. The LLM cannot corrupt content
	// it never touches (session-35 regression was a lost trailing
	// `\n` during re-emission that `git apply` reports as "corrupt
	// patch at line N").
	if kind == "patch" {
		if strings.TrimSpace(unit.Patch) == "" {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: plan ChangeUnit for %q has empty Patch (planner bug — kind=patch must carry a unified diff)", path)), nil
		}
		if err := applyUnifiedDiff(ctx.RepoRoot, unit.Patch); err != nil {
			return errResult(t.Name(), composeApplyRejection(ctx.RepoRoot, path, err.Error(), unit.Patch)), nil
		}
		ctx.Mutable.WriteClosure().MarkApplied(path)
		logging.Info("[apply_patch] patch %s (worktree=%s)", path, ctx.RepoRoot)
		return okResult(t.Name(),
			fmt.Sprintf("apply_patch: patch %q applied to worktree via git apply", path)), nil
	}

	// Resolve absolute path against the swapped RepoRoot. Day-2
	// audit confirmed the apply stage hook has set ctx.RepoRoot to the
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
		if err := os.WriteFile(absPathCanonical, []byte(unit.NewContent), 0o644); err != nil {
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
		if err := os.WriteFile(absPathCanonical, []byte(unit.NewContent), 0o644); err != nil {
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
	case "rename":
		// rename: move Path → NewPath. Old path must exist; new path
		// must not. Both are validated by emit_change_plan at plan
		// time, but we re-check here in case the worktree drifted
		// between plan emit and apply (warm-rewind retry can reset
		// disk state).
		newPathRaw := strings.TrimSpace(unit.NewPath)
		if newPathRaw == "" {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: rename %q failed — new_path is empty", path)), nil
		}
		newPath := filepath.ToSlash(newPathRaw)
		newPath = strings.TrimPrefix(newPath, "./")
		newAbs := filepath.Join(ctx.RepoRoot, filepath.FromSlash(newPath))
		newAbsCanonical, err := filepath.Abs(newAbs)
		if err != nil {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch: resolve new_path %s: %v", newPath, err)), err
		}
		if rel, err := filepath.Rel(absRoot, newAbsCanonical); err != nil || strings.HasPrefix(rel, "..") {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: new_path %q escapes RepoRoot via traversal", newPath)), nil
		}
		if _, err := os.Stat(absPathCanonical); err != nil {
			if os.IsNotExist(err) {
				return errResult(t.Name(),
					fmt.Sprintf("apply_patch rejected: rename %q failed — source file does not exist. "+
						"Use kind=create to add a new file.", path)), nil
			}
			return errResult(t.Name(), fmt.Sprintf("apply_patch: stat %s: %v", path, err)), err
		}
		if _, err := os.Stat(newAbsCanonical); err == nil {
			return errResult(t.Name(),
				fmt.Sprintf("apply_patch rejected: rename %q → %q failed — destination already exists in worktree. "+
					"Use kind=delete on the destination first if you mean to overwrite.", path, newPath)), nil
		} else if !os.IsNotExist(err) {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: stat %s: %v", newPath, err)), err
		}
		if err := os.MkdirAll(filepath.Dir(newAbsCanonical), 0o755); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: mkdir parent of %s: %v", newPath, err)), err
		}
		if err := os.Rename(absPathCanonical, newAbsCanonical); err != nil {
			return errResult(t.Name(), fmt.Sprintf("apply_patch: rename %s → %s: %v", path, newPath, err)), err
		}
		// Mark BOTH paths applied so downstream W1b checks see
		// dependents on either name as satisfied: a follow-on
		// modify(NewPath) sees AppliedSet[NewPath]=true; an early
		// downstream entry that named the old path also short-
		// circuits the W1b check.
		ctx.Mutable.WriteClosure().MarkApplied(path)
		ctx.Mutable.WriteClosure().MarkApplied(newPath)
		logging.Info("[apply_patch] rename %s → %s (worktree=%s)", path, newPath, ctx.RepoRoot)
		return okResult(t.Name(),
			fmt.Sprintf("apply_patch: rename %q → %q applied to worktree", path, newPath)), nil
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

// applyUnifiedDiff pipes a unified-diff text into `git apply -`
// running inside worktreeRoot, returning a combined-output error
// when git rejects the patch. The caller guarantees worktreeRoot
// is a valid git checkout (the orchestrator's worktree session).
//
// Tolerance chain (session-35): try `git apply --recount` first, and
// if git rejects with a "patch failed" / "corrupt patch" / "while
// searching for" shape, fall back to GNU `patch(1) -p1`. git apply
// is preferred because it understands git-specific metadata (a/ b/
// prefixes, mode changes, rename hints); patch(1) is the safety net
// because its fuzz-matching tolerates the shapes LLMs reliably
// produce and git rejects — miscounted hunk headers with truncated
// body, missing trailing context lines, and small whitespace drift.
// Session-34 rejected patch(1) outright for metadata-loss reasons;
// session-35 walks that back to "git first, patch fallback" because
// the real-LLM eval showed 1/3 Python dispatches dropping trailing
// context, and no amount of --recount / --ignore-whitespace / header
// re-normalization recovers that in git.
//
// Binary patches, mode changes, and renames — none of which LLMs
// emit today — keep working because git handles them on the
// primary path. The fallback only fires when git rejects; at that
// point the diff is plain-text content (no git-specific features
// to preserve).
func applyUnifiedDiff(worktreeRoot, patchText string) error {
	return runUnifiedDiff(worktreeRoot, patchText, false)
}

// CheckUnifiedDiff runs an apply-in-dry-run against repoRoot without
// mutating the worktree. Returns nil if the patch would apply cleanly
// via either git apply or the patch(1) fallback, else an error
// carrying the primary (git) stderr so the planner retry sees the
// strictest diagnostic available.
//
// Used by emit_change_plan as a pre-flight gate on kind=patch units:
// rejecting a malformed diff at emission time lets the planner retry
// within the same dispatch (fail-loud, LLM sees git's error verbatim
// and can regenerate), instead of the malformed diff being persisted
// to plan.json and exploding at apply time after user approval — the
// session-35 failure mode where 2/3 of patches were rejected by git
// apply AFTER a successful-looking plan emission.
func CheckUnifiedDiff(repoRoot, patchText string) error {
	return runUnifiedDiff(repoRoot, patchText, true)
}

// runUnifiedDiff is the chained applier: git apply first, patch(1)
// fallback. Both honor checkOnly (git uses --check, patch uses
// --dry-run). On success returns nil with no side-effect info; on
// failure returns the git error (strictest, most-diagnostic) rather
// than patch(1)'s error — surfacing the git message gives the LLM
// precise guidance for regeneration, and we only landed in the
// fallback because git rejected.
func runUnifiedDiff(repoRoot, patchText string, checkOnly bool) error {
	gitErr := runGitApply(repoRoot, patchText, checkOnly)
	if gitErr == nil {
		return nil
	}
	// Fallback to patch(1). If patch(1) succeeds, the content is
	// semantically fine — git's rejection was an LLM-diff-artifact
	// we can tolerate. Log the fallback so operators see the signal.
	if err := runPatchTool(repoRoot, patchText, checkOnly); err == nil {
		logging.Info("[apply_patch] git rejected but patch(1) accepted; applied via fallback")
		return nil
	}
	// Both rejected — report the git error as primary. It's the
	// strictest validator and its stderr ("patch failed: file:line",
	// "corrupt patch at line N") is what the planner-retry pipeline
	// is wired to consume in composePatchRejection.
	return gitErr
}

// runPatchTool runs GNU `patch(1) -p1` against repoRoot with the
// given patch text. checkOnly=true adds --dry-run. Returns nil on
// success, combined-output error on failure. Not exported: callers
// should use runUnifiedDiff which chains this after git apply.
func runPatchTool(repoRoot, patchText string, checkOnly bool) error {
	if patchText != "" && !strings.HasSuffix(patchText, "\n") {
		patchText += "\n"
	}
	// -p1 strips the a/ b/ prefix that unified diffs carry.
	// --no-backup-if-mismatch keeps the worktree clean on failure.
	// --silent suppresses verbose progress; we want the stderr only
	// on rejection.
	// --force avoids interactive prompts when patching non-existent
	// files or already-patched hunks.
	args := []string{"-p1", "--no-backup-if-mismatch", "--silent", "--force"}
	if checkOnly {
		args = append(args, "--dry-run")
	}
	cmd := exec.Command("patch", args...)
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patchText)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		outTrim := strings.TrimSpace(string(out))
		if outTrim == "" {
			outTrim = runErr.Error()
		}
		return fmt.Errorf("%s", outTrim)
	}
	return nil
}

// runGitApply shared implementation. checkOnly=true adds --check.
// The deadlock-safe stdin feed pattern mirrors the existing
// apply_patch apply path.
//
// Tolerance surface — targets the full class of LLM-generated
// unified-diff imperfections, not one sampled bug. The session-35
// e2e run surfaced two failure modes across N=3:
//
//   (1) hunk header line-count off by one (@@ -22,6 +22,6 @@ over
//       5 actual body lines). The body was byte-correct; only the
//       redundant metadata was wrong.
//
//   (2) patch body missing its final `\n` terminator. Git's parser
//       treats a non-terminated last line as EOF rather than a
//       content line, which surfaced as "corrupt patch at line N"
//       in the trace where N was the body length.
//
// Fix (1) via git's own `--recount` flag — git's man page frames
// this exactly as the workaround for "hand-edited patches that
// didn't have the header updated". Fix (2) by normalising the
// patch text before piping: append `\n` when the caller omitted
// one. We intentionally do NOT pass --inaccurate-eof; that flag
// alters git's tolerance for actual missing-EOL-at-end-of-file
// bytes in the target, which breaks well-formed patches that
// legitimately declare the file DOES end with a newline.
// Normalising only the patch's outer terminator is enough to
// close the session-35 failure mode without widening the
// acceptance surface to corner cases we don't understand.
func runGitApply(repoRoot, patchText string, checkOnly bool) error {
	if patchText != "" && !strings.HasSuffix(patchText, "\n") {
		patchText += "\n"
	}
	args := []string{"apply", "--recount"}
	if checkOnly {
		args = append(args, "--check")
	}
	args = append(args, "-")
	cmd, cancel, stdin, err := NewGitCommandWithStdin(nil, args...)
	if err != nil {
		return fmt.Errorf("setup stdin pipe: %w", err)
	}
	defer cancel()
	cmd.Dir = repoRoot
	var feedErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, werr := io.WriteString(stdin, patchText)
		if werr != nil {
			feedErr = werr
		}
		if cerr := stdin.Close(); cerr != nil && feedErr == nil {
			feedErr = cerr
		}
	}()
	out, runErr := cmd.CombinedOutput()
	<-done
	if feedErr != nil {
		return fmt.Errorf("pipe patch to git apply: %w", feedErr)
	}
	if runErr != nil {
		outTrim := strings.TrimSpace(string(out))
		if outTrim == "" {
			outTrim = runErr.Error()
		}
		return fmt.Errorf("%s", outTrim)
	}
	return nil
}
