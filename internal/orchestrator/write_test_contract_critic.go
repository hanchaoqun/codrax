package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const writeConstraintPreserveRegressionTest = "preserve_regression_test"
const writeConstraintPreserveFailedTestAssertion = "preserve_failed_test_assertion"

type writeTestContractFinding struct {
	Path     string
	Snippet  string
	Contract types.WriteConstraint
}

func (o *Orchestrator) testContractReplanHint(plan *types.ChangePlan) (string, []string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return "", nil
	}
	var findings []writeTestContractFinding
	ir := plan.WriteAnalysisIR
	if ir == nil {
		ir = o.busCtx.Mutable.WriteAnalysisIR()
	}
	if ir != nil {
		contracts := protectedRegressionTestContracts(ir.Request.Constraints)
		for _, change := range plan.Changes {
			path, ok := normalizeWorkflowScopePath(change.Path)
			if !ok {
				continue
			}
			for _, contract := range contracts {
				target, ok := normalizeWorkflowScopePath(contract.Target)
				if !ok || !workflowPathWithinAnyScope(path, []string{target}) {
					continue
				}
				for _, snippet := range removedProtectedTestSnippets(o.busCtx.RepoRoot, path, change) {
					findings = append(findings, writeTestContractFinding{
						Path:     path,
						Snippet:  snippet,
						Contract: contract,
					})
				}
			}
		}
	}
	findings = append(findings, o.failedVerifyTestContractFindings(plan)...)
	if len(findings) == 0 {
		return "", nil
	}
	return renderTestContractReplanHint(findings), uniqueFindingPaths(findings)
}

func protectedRegressionTestContracts(in []types.WriteConstraint) []types.WriteConstraint {
	out := make([]types.WriteConstraint, 0, len(in))
	for _, c := range in {
		if strings.TrimSpace(c.Kind) != writeConstraintPreserveRegressionTest {
			continue
		}
		if strings.TrimSpace(c.Target) == "" || strings.TrimSpace(c.Target) == "*" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func removedProtectedTestSnippets(repoRoot, relPath string, change types.FileChange) []string {
	switch strings.TrimSpace(change.Kind) {
	case "delete":
		return []string{"<entire protected regression test file deleted>"}
	case "modify":
		return removedLinesFromFullModify(repoRoot, relPath, change.NewContent)
	case "patch":
		if len(change.Edits) > 0 {
			return removedLinesFromStructuredEdits(repoRoot, relPath, change.Edits)
		}
		return removedLinesFromUnifiedDiff(change.Patch)
	default:
		return nil
	}
}

func (o *Orchestrator) failedVerifyTestContractFindings(plan *types.ChangePlan) []writeTestContractFinding {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return nil
	}
	handoff := o.busCtx.Mutable.VerifyFailureHandoff()
	if handoff == nil || len(handoff.FailingTests) == 0 {
		return nil
	}
	protected := failedVerifyProtectedTestLines(o.busCtx.RepoRoot, handoff.FailingTests)
	if len(protected) == 0 {
		return nil
	}
	var findings []writeTestContractFinding
	for _, change := range plan.Changes {
		path, ok := normalizeWorkflowScopePath(change.Path)
		if !ok {
			continue
		}
		wantSnippets := protected[path]
		if len(wantSnippets) == 0 {
			continue
		}
		for _, removed := range removedProtectedTestSnippets(o.busCtx.RepoRoot, path, change) {
			if !containsNormalizedSnippet(wantSnippets, removed) {
				continue
			}
			findings = append(findings, writeTestContractFinding{
				Path:    path,
				Snippet: removed,
				Contract: types.WriteConstraint{
					Kind:   writeConstraintPreserveFailedTestAssertion,
					Target: path,
				},
			})
		}
	}
	return findings
}

func failedVerifyProtectedTestLines(repoRoot string, tests []types.TestResult) map[string][]string {
	out := map[string][]string{}
	for _, result := range tests {
		if result.Passed {
			continue
		}
		signal := types.ExtractTestFailureSignal(result, 900)
		path, line, ok := failureSignalRepoLocation(repoRoot, signal.Location)
		if !ok || line <= 0 || types.ClassifySourcePathRole(path) != types.SourcePathRoleTest {
			continue
		}
		snippet := readRepoLine(repoRoot, path, line)
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		out[path] = appendBoundedUnique(out[path], snippet, 6)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func failureSignalRepoLocation(repoRoot, location string) (string, int, bool) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", 0, false
	}
	idx := strings.LastIndex(location, ":")
	if idx <= 0 || idx == len(location)-1 {
		return "", 0, false
	}
	line, ok := parsePositiveInt(strings.TrimSpace(location[idx+1:]))
	if !ok {
		return "", 0, false
	}
	rawPath := strings.TrimSpace(location[:idx])
	if rawPath == "" {
		return "", 0, false
	}
	if filepath.IsAbs(rawPath) {
		root := strings.TrimSpace(repoRoot)
		if root == "" {
			return "", 0, false
		}
		if absRoot, err := filepath.Abs(root); err == nil {
			root = absRoot
		}
		rel, err := filepath.Rel(root, rawPath)
		if err != nil {
			return "", 0, false
		}
		if rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
			return "", 0, false
		}
		rawPath = stripCodraxWorktreePathPrefix(rel)
	}
	rawPath = stripCodraxWorktreePathPrefix(rawPath)
	path, ok := normalizeWorkflowScopePath(filepath.ToSlash(rawPath))
	if !ok {
		return "", 0, false
	}
	return path, line, true
}

func stripCodraxWorktreePathPrefix(path string) string {
	slash := filepath.ToSlash(strings.TrimSpace(path))
	parts := strings.Split(slash, "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] != ".codrax" || parts[i+1] != "worktrees" || strings.TrimSpace(parts[i+2]) == "" {
			continue
		}
		if i+3 >= len(parts) {
			return slash
		}
		return strings.Join(parts[i+3:], "/")
	}
	return slash
}

func parsePositiveInt(raw string) (int, bool) {
	var n int
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, n > 0
}

func readRepoLine(repoRoot, relPath string, line int) string {
	if line <= 0 {
		return ""
	}
	lines := readWriteCriticFileLines(repoRoot, relPath)
	if line > len(lines) {
		return ""
	}
	return strings.TrimRight(lines[line-1], "\r\n")
}

func containsNormalizedSnippet(snippets []string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	for _, snippet := range snippets {
		if strings.TrimSpace(snippet) == candidate {
			return true
		}
	}
	return false
}

func removedLinesFromUnifiedDiff(patch string) []string {
	var out []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "---") {
			continue
		}
		if !strings.HasPrefix(line, "-") {
			continue
		}
		text := strings.TrimPrefix(line, "-")
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = appendBoundedUnique(out, text, 6)
	}
	return out
}

func removedLinesFromStructuredEdits(repoRoot, relPath string, edits []types.StructuredEdit) []string {
	lines := readWriteCriticFileLines(repoRoot, relPath)
	var out []string
	for _, edit := range edits {
		switch strings.TrimSpace(edit.Kind) {
		case "replace", "delete":
		default:
			continue
		}
		if strings.TrimSpace(edit.OldText) != "" {
			for _, line := range strings.Split(edit.OldText, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				out = appendBoundedUnique(out, line, 6)
			}
			continue
		}
		if len(lines) == 0 || edit.StartLine < 1 {
			continue
		}
		endLine := edit.EndLine
		if endLine == 0 {
			endLine = edit.StartLine
		}
		if endLine < edit.StartLine || endLine > len(lines) {
			continue
		}
		for _, line := range lines[edit.StartLine-1 : endLine] {
			line = strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(line) == "" {
				continue
			}
			out = appendBoundedUnique(out, line, 6)
		}
	}
	return out
}

func removedLinesFromFullModify(repoRoot, relPath, newContent string) []string {
	oldLines := readWriteCriticFileLines(repoRoot, relPath)
	if len(oldLines) == 0 {
		return nil
	}
	newCounts := map[string]int{}
	for _, line := range strings.SplitAfter(newContent, "\n") {
		newCounts[strings.TrimRight(line, "\r\n")]++
	}
	var out []string
	for _, line := range oldLines {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if newCounts[line] > 0 {
			newCounts[line]--
			continue
		}
		out = appendBoundedUnique(out, line, 6)
	}
	return out
}

func readWriteCriticFileLines(repoRoot, relPath string) []string {
	root := strings.TrimSpace(repoRoot)
	path, ok := normalizeWorkflowScopePath(relPath)
	if root == "" || !ok {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return nil
	}
	return strings.SplitAfter(string(raw), "\n")
}

func renderTestContractReplanHint(findings []writeTestContractFinding) string {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Snippet < findings[j].Snippet
		}
		return findings[i].Path < findings[j].Path
	})
	var b strings.Builder
	b.WriteString("## Typed test-contract critique\n\n")
	b.WriteString("The previous ChangePlan removed or replaced content from regression tests protected by typed request constraints or prior failed verification evidence. Replan this same batch preserving the protected regression assertions; production fixes and additional tests are allowed.\n\n")
	b.WriteString("Protected test changes to preserve:\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "- %s: %s\n", f.Path, boundedTestContractSnippet(f.Snippet))
	}
	b.WriteString("\nProtected constraints:\n")
	seen := map[string]bool{}
	for _, f := range findings {
		key := strings.TrimSpace(f.Contract.Kind) + "|" + strings.TrimSpace(f.Contract.Target)
		if seen[key] {
			continue
		}
		seen[key] = true
		fmt.Fprintf(&b, "- kind=%s target=%s\n", strings.TrimSpace(f.Contract.Kind), strings.TrimSpace(f.Contract.Target))
	}
	b.WriteString("\nDo not reduce, remove, or narrow these protected regression inputs unless a later typed exploration result supersedes the constraint.")
	return strings.TrimSpace(b.String())
}

func uniqueFindingPaths(findings []writeTestContractFinding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range findings {
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

func appendBoundedUnique(in []string, s string, max int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return in
	}
	for _, existing := range in {
		if existing == s {
			return in
		}
	}
	if len(in) >= max {
		return in
	}
	return append(in, s)
}

func boundedTestContractSnippet(s string) string {
	s = strings.TrimSpace(s)
	const max = 180
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
