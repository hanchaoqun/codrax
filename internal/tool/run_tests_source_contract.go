package tool

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const postApplySourceContractMaxLineBytes = 1 << 20

// postApplySourceContractConfidenceRecords observes plan-owned post-apply
// source lines for behavior contracts whose typed evidence_ref identifies one
// exact line in a plan-owned repository file. It is deliberately
// language-neutral and does not parse request text, model prose, runtime
// output, or symbol names.
//
// V5-1 (colleague_merge_audit §40.10 / §40.35): a source reading is a
// `source_text` witness. It may SATISFY a contract only when the contract's
// kind admits that witness under the types-level matrix (file_layout), and
// only at a line bound to the post-apply file. The binding base is the
// worktree's base commit (`baseSHA`) — the tree the write analysis took every
// evidence_ref line from — and the binding walks the CUMULATIVE diff from
// that base to the WORKING TREE (the bytes the witness reads), so earlier
// slices, earlier units, earlier applied plans and even an apply whose
// checkpoint commit failed are all accounted for; a tracked file absent from
// that diff is byte-identical to the base and keeps its numbering exactly.
// Files that were dirty in the main checkout when the analysis ran
// (`baseDirtyPaths`) have no base tree at all. Every other outcome is a typed
// disclosure on the `source_text_presence` advisory lane — never a satisfied
// observation:
//   - the base is unknown or the file had no base tree, the line was
//     removed, lies in a rewritten (or binary) region, the file was
//     created/deleted, or the bound line cannot be read →
//     post_apply_source_contract_line_unresolved;
//   - a runtime-kind contract whose value is present / absent on the bound
//     line → post_apply_source_text_present / post_apply_source_text_absent
//     (replan hint only; an executed probe or project test must observe it).
func postApplySourceContractConfidenceRecords(repoRoot, baseSHA string, baseDirtyPaths []string, plan *types.ChangePlan) []types.VerificationConfidenceRecord {
	if plan == nil || strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	owned := postApplySourceContractOwnedPaths(plan)
	if len(owned) == 0 {
		return nil
	}
	binder := newPostApplySourceContractBinder(repoRoot, baseSHA, baseDirtyPaths)
	var out []types.VerificationConfidenceRecord
	for _, contract := range types.ChangePlanVerificationBehaviorContracts(plan) {
		if !postApplySourceContractEligible(contract) {
			continue
		}
		surface, ok := types.ParseAnswerSourceLocationSurface(contract.EvidenceRef)
		if !ok || surface.LineStart <= 0 || surface.LineStart != surface.LineEnd {
			continue
		}
		rel, ok := postApplySourceContractRepoPath(surface.File)
		if !ok || !owned[rel] {
			continue
		}
		id := strings.TrimSpace(contract.ID)
		unresolved := func(why string) {
			out = append(out, types.VerificationConfidenceRecord{
				Source:       "post_apply_source_observation",
				Category:     "source_text_presence",
				Status:       "advisory",
				Severity:     "info",
				ReasonCode:   "post_apply_source_contract_line_unresolved",
				ContractRefs: []string{id},
				WitnessKind:  types.WriteBehaviorWitnessSourceText,
				Detail:       why + "; the post-apply source was not used and cannot witness this contract",
			})
		}
		binding := binder.bind(rel, surface.LineStart)
		if !binding.Mapped() {
			unresolved(postApplySourceContractUnresolvedDetail(binding.Status))
			continue
		}
		line, ok := readPostApplySourceContractLine(repoRoot, binding.Path, binding.Line)
		if !ok {
			unresolved("the bound post-apply line could not be read")
			continue
		}
		matched := postApplySourceContractMatches(contract.Operator, line, contract.Expected)
		if types.WriteBehaviorWitnessSatisfies(contract.Kind, types.WriteBehaviorWitnessSourceText) {
			status := "satisfied"
			severity := "info"
			reasonCode := "post_apply_source_contract_observed"
			detail := "the plan-owned post-apply source line satisfied the exact typed source-value contract"
			if !matched {
				status = "missing"
				severity = "warning"
				reasonCode = "post_apply_source_contract_value_mismatch"
				detail = "the plan-owned post-apply source line did not satisfy the exact typed source-value contract"
			}
			out = append(out, types.VerificationConfidenceRecord{
				Source:       "post_apply_source_observation",
				Category:     "source_contract_refs",
				Status:       status,
				Severity:     severity,
				ReasonCode:   reasonCode,
				ContractRefs: []string{id},
				WitnessKind:  types.WriteBehaviorWitnessSourceText,
				Detail:       detail,
			})
			continue
		}
		reasonCode := "post_apply_source_text_present"
		detail := "the contract value is present on the bound post-apply source line, but source presence is not a behavior observation; an executed verification probe or project test must observe this contract"
		if !matched {
			reasonCode = "post_apply_source_text_absent"
			detail = "the contract value is absent from the bound post-apply source line; source absence is not a behavior observation and an executed verification probe or project test must observe this contract"
		}
		out = append(out, types.VerificationConfidenceRecord{
			Source:       "post_apply_source_observation",
			Category:     "source_text_presence",
			Status:       "advisory",
			Severity:     "info",
			ReasonCode:   reasonCode,
			ContractRefs: []string{id},
			WitnessKind:  types.WriteBehaviorWitnessSourceText,
			Detail:       detail,
		})
	}
	return out
}

// postApplySourceContractUnresolvedDetail renders a remap outcome in plain
// words (the typed status itself is an internal artifact and must not reach
// planner/verifier prompts through the record detail).
func postApplySourceContractUnresolvedDetail(status types.PatchEffectLineRemapStatus) string {
	switch status {
	case types.PatchEffectLineRemovedByPatch:
		return "the change removed the referenced source line"
	case types.PatchEffectLineFileCreated:
		return "the referenced file did not exist before the change"
	case types.PatchEffectLineFileDeleted:
		return "the referenced file was deleted by the change"
	case types.PatchEffectLineHunkUnmapped:
		return "the referenced line lies inside a rewritten region and cannot be located precisely"
	case types.PatchEffectLineInvalid:
		return "the evidence reference does not name a valid line"
	case types.PatchEffectLineNoPatchEffect:
		return "the base tree of the evidence reference is unknown, so the referenced line cannot be located after the change"
	case types.PatchEffectLineBaseDivergent:
		return "the referenced file had uncommitted changes in the main checkout when the analysis ran, so its line numbers cannot be located after the change"
	default:
		return "the referenced line could not be located after the change"
	}
}

// postApplySourceContractBinder binds pre-apply evidence lines through the
// cumulative diff from the analysis base to the working tree of the git
// checkout containing repoRoot. The diff is captured ONCE for the whole
// tree (only the applied delta is ever present) with the git configuration
// pinned to the canonical unified shape — external diff drivers, textconv,
// prefix options and blank-line suppression are all overridden — so the
// parser never sees a shape it cannot bind; renames are paired (-M) so a
// moved file maps to its new path instead of reading as deleted.
type postApplySourceContractBinder struct {
	repoRoot   string
	gitRoot    string
	baseSHA    string
	ok         bool
	dirty      map[string]bool // git-root relative paths dirty at analysis time
	effect     *types.PatchEffectRecord
	effectDone bool
	inBase     map[string]bool
}

func newPostApplySourceContractBinder(repoRoot, baseSHA string, baseDirtyPaths []string) *postApplySourceContractBinder {
	b := &postApplySourceContractBinder{repoRoot: strings.TrimSpace(repoRoot), baseSHA: strings.TrimSpace(baseSHA), dirty: map[string]bool{}, inBase: map[string]bool{}}
	for _, raw := range baseDirtyPaths {
		if rel := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(raw)), "./"); rel != "" {
			b.dirty[rel] = true
		}
	}
	if b.baseSHA == "" {
		return b
	}
	gitRoot, ok := verificationWorktreeGitRootCandidate(b.repoRoot)
	if !ok || !b.git("cat-file", "-e", b.baseSHA+"^{commit}").ok {
		return b
	}
	b.gitRoot = gitRoot
	b.ok = true
	return b
}

type postApplyGitResult struct {
	out string
	ok  bool
}

// git runs a read-only git command in the git root with the diff/quoting
// configuration pinned to the canonical shape the parser understands.
func (b *postApplySourceContractBinder) git(args ...string) postApplyGitResult {
	root := b.gitRoot
	if root == "" {
		root, _ = verificationWorktreeGitRootCandidate(b.repoRoot)
		if root == "" {
			return postApplyGitResult{}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pinned := []string{
		"-c", "core.quotePath=false",
		"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false",
		"-c", "diff.srcPrefix=a/", "-c", "diff.dstPrefix=b/",
		"-c", "diff.suppressBlankEmpty=false", "-c", "diff.renames=true",
		"-c", "diff.external=", "-c", "diff.algorithm=myers",
	}
	cmd := exec.CommandContext(ctx, "git", append(pinned, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_EXTERNAL_DIFF=", "GIT_DIFF_OPTS=")
	out, err := cmd.Output()
	if err != nil {
		return postApplyGitResult{}
	}
	return postApplyGitResult{out: string(out), ok: true}
}

// bind returns the post-apply position of `line` in `rel` (repoRoot-relative
// in, repoRoot-relative out).
func (b *postApplySourceContractBinder) bind(rel string, line int) types.PatchEffectLineRemap {
	if line <= 0 {
		return types.PatchEffectLineRemap{Status: types.PatchEffectLineInvalid}
	}
	if b == nil || !b.ok {
		return types.PatchEffectLineRemap{Status: types.PatchEffectLineNoPatchEffect}
	}
	gitRel, ok := b.gitRelative(rel)
	if !ok {
		return types.PatchEffectLineRemap{Status: types.PatchEffectLineInvalid}
	}
	if b.dirty[gitRel] {
		return types.PatchEffectLineRemap{Status: types.PatchEffectLineBaseDivergent}
	}
	effect, ok := b.cumulativeEffect()
	if !ok {
		return types.PatchEffectLineRemap{Status: types.PatchEffectLineNoPatchEffect}
	}
	remap := types.RemapPatchEffectOldLine(effect, gitRel, line)
	switch remap.Status {
	case types.PatchEffectLineFileNotInPatch:
		// Absent from the base→working-tree diff: either byte-identical to
		// the base (numbering exact) or never in the base at all (untracked
		// creation — no evidence line can refer to it).
		if !b.existsInBase(gitRel) {
			return types.PatchEffectLineRemap{Path: rel, Status: types.PatchEffectLineFileCreated}
		}
		return types.PatchEffectLineRemap{Path: rel, Line: line, Status: types.PatchEffectLineMapped}
	case types.PatchEffectLineMapped:
		back, ok := b.repoRelative(remap.Path)
		if !ok {
			return types.PatchEffectLineRemap{Status: types.PatchEffectLineInvalid}
		}
		remap.Path = back
		return remap
	default:
		return remap
	}
}

func (b *postApplySourceContractBinder) existsInBase(gitRel string) bool {
	if cached, ok := b.inBase[gitRel]; ok {
		return cached
	}
	exists := b.git("cat-file", "-e", b.baseSHA+":"+gitRel).ok
	b.inBase[gitRel] = exists
	return exists
}

func (b *postApplySourceContractBinder) gitRelative(rel string) (string, bool) {
	abs := filepath.Join(b.repoRoot, filepath.FromSlash(rel))
	out, err := filepath.Rel(b.gitRoot, abs)
	if err != nil {
		return "", false
	}
	out = filepath.ToSlash(out)
	if out == "." || out == ".." || strings.HasPrefix(out, "../") {
		return "", false
	}
	return out, true
}

func (b *postApplySourceContractBinder) repoRelative(gitRel string) (string, bool) {
	abs := filepath.Join(b.gitRoot, filepath.FromSlash(gitRel))
	out, err := filepath.Rel(b.repoRoot, abs)
	if err != nil {
		return "", false
	}
	return postApplySourceContractRepoPath(filepath.ToSlash(out))
}

// cumulativeEffect captures `git diff <base>` (base tree vs working tree,
// whole checkout, renames paired) once. An empty diff is a valid record with
// no files.
func (b *postApplySourceContractBinder) cumulativeEffect() (*types.PatchEffectRecord, bool) {
	if b.effectDone {
		return b.effect, b.effect != nil
	}
	b.effectDone = true
	res := b.git("diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--src-prefix=a/", "--dst-prefix=b/", b.baseSHA)
	if !res.ok {
		return nil, false
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff("", "", "worktree_base_range", b.baseSHA, "worktree", res.out)
	b.effect = &effect
	return b.effect, true
}

// postApplySourceContractEligible keeps the operator/shape gate. Kind is NOT
// decided here: every kind may produce a disclosure record; whether the
// source witness can SATISFY the contract is the matrix's decision.
func postApplySourceContractEligible(contract types.WriteBehaviorContract) bool {
	if !types.IsHardRequiredWriteBehaviorContract(contract) ||
		contract.Placement != nil || contract.Transition != nil || contract.Comparator != nil ||
		strings.TrimSpace(contract.Expected) == "" {
		return false
	}
	switch contract.Operator {
	case types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals,
		types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains:
		return true
	default:
		return false
	}
}

func postApplySourceContractOwnedPaths(plan *types.ChangePlan) map[string]bool {
	out := map[string]bool{}
	for _, raw := range types.ChangePlanVerificationTargetPaths(plan, nil) {
		if rel, ok := postApplySourceContractRepoPath(raw); ok {
			out[rel] = true
		}
	}
	return out
}

func postApplySourceContractRepoPath(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	if raw == "" || strings.HasPrefix(raw, "/") || filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", false
	}
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}
	return rel, true
}

func readPostApplySourceContractLine(repoRoot, rel string, lineNumber int) (string, bool) {
	if lineNumber <= 0 {
		return "", false
	}
	rootAbs, err := filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", false
	}
	resolvedTarget, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filepath.FromSlash(rel)))
	if err != nil || !pathWithinRoot(resolvedRoot, resolvedTarget) || samePath(resolvedRoot, resolvedTarget) {
		return "", false
	}
	file, err := os.Open(resolvedTarget)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), postApplySourceContractMaxLineBytes)
	for current := 1; scanner.Scan(); current++ {
		if current == lineNumber {
			return scanner.Text(), true
		}
	}
	return "", false
}

func postApplySourceContractMatches(operator types.WriteBehaviorOperator, actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	switch operator {
	case types.WriteBehaviorOpEquals:
		return actual == expected
	case types.WriteBehaviorOpNotEquals:
		return actual != expected
	case types.WriteBehaviorOpContains:
		return strings.Contains(actual, expected)
	case types.WriteBehaviorOpNotContains:
		return !strings.Contains(actual, expected)
	default:
		return false
	}
}
