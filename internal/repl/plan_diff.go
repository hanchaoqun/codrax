package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderPlanDiff produces a human-readable per-change preview of a
// ChangePlan suitable for /plan show. Output shape is unified-diff-
// ish so a user can /approve with full knowledge of what will change:
//
//   - create: "(new file)" header + full new body
//   - modify: real unified diff between current on-disk bytes and
//     the plan's NewContent
//   - delete: "(deleting existing file)" header + head of current
//     bytes so the user knows what's about to disappear
//   - patch:  the plan's Patch field (already a unified diff)
//     rendered verbatim
//
// For modify, repoRoot is used to read the current file contents.
// An unreadable file (missing / permission) surfaces a note rather
// than an error — the planner may legitimately propose to modify a
// file that only exists in a downstream directory we can't stat.
//
// The total output is bounded by maxTotalBytes; per-change by
// maxPerChangeBytes. When either cap is hit, a truncation marker is
// appended and remaining changes are summarised by path only.
//
// Generalisation: uses only FileChange fields every plan carries
// (Path / Kind / NewContent / Patch / Rationale). No language-specific
// logic; works for any code kind=modify touches.
func renderPlanDiff(plan *types.ChangePlan, repoRoot string, maxTotalBytes, maxPerChangeBytes int) string {
	if plan == nil || len(plan.Changes) == 0 {
		return "(no changes)"
	}
	var b strings.Builder
	truncatedAt := -1
	for i, c := range plan.Changes {
		if b.Len() >= maxTotalBytes {
			truncatedAt = i
			break
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "─── %s (kind=%s) ───\n", c.Path, c.Kind)
		if strings.TrimSpace(c.Rationale) != "" {
			fmt.Fprintf(&b, "rationale: %s\n", oneLine(c.Rationale))
		}
		if len(c.DependsOn) > 0 {
			fmt.Fprintf(&b, "depends_on: %s\n", strings.Join(c.DependsOn, ", "))
		}
		body := renderOneChange(c, repoRoot)
		if len(body) > maxPerChangeBytes {
			body = body[:maxPerChangeBytes] + "\n… (change body truncated; inspect the plan JSON for full detail)\n"
		}
		b.WriteString(body)
	}
	if truncatedAt >= 0 {
		remaining := plan.Changes[truncatedAt:]
		b.WriteString("\n… (diff output truncated; remaining change(s):\n")
		for _, c := range remaining {
			fmt.Fprintf(&b, "  - %s (kind=%s)\n", c.Path, c.Kind)
		}
		b.WriteString("inspect the plan JSON for full detail)\n")
	}
	return b.String()
}

// renderOneChange builds the diff block for a single FileChange.
// Called from renderPlanDiff per entry; kept separate so per-kind
// logic stays readable.
func renderOneChange(c types.FileChange, repoRoot string) string {
	switch c.Kind {
	case "create":
		var b strings.Builder
		b.WriteString("(new file)\n")
		// A real "diff against empty" would just add "+ " to every
		// line; go-udiff does exactly that when called with old="".
		// But rendering raw content is more readable for a new file.
		body := c.NewContent
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		// Prefix every line with "+" so the output looks like a diff
		// and copy-paste into email / PR review works.
		for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
			b.WriteString("+ ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		return b.String()
	case "modify":
		current := readFileForDiff(c.Path, repoRoot)
		// current == "" when file isn't readable; udiff.Unified
		// produces a "new file" shape diff in that case, which is
		// honest — the planner is writing a full body against what
		// we can see.
		diff := udiff.Unified("a/"+c.Path, "b/"+c.Path, current, c.NewContent)
		if diff == "" {
			return "(no textual change — NewContent matches current file bytes)\n"
		}
		return diff
	case "delete":
		var b strings.Builder
		b.WriteString("(deleting existing file)\n")
		current := readFileForDiff(c.Path, repoRoot)
		if current == "" {
			b.WriteString("(could not read current file; apply is idempotent for absent files)\n")
			return b.String()
		}
		for _, line := range strings.Split(strings.TrimSuffix(current, "\n"), "\n") {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		return b.String()
	case "patch":
		// Patch field carries a unified diff already — render verbatim.
		if strings.TrimSpace(c.Patch) == "" {
			return "(empty patch body — plan likely malformed)\n"
		}
		if !strings.HasSuffix(c.Patch, "\n") {
			return c.Patch + "\n"
		}
		return c.Patch
	}
	return fmt.Sprintf("(unknown kind %q — cannot render preview)\n", c.Kind)
}

// readFileForDiff reads a plan-relative path from repoRoot, returning
// the content as a string. Empty string on any error — callers treat
// that as "file absent" which produces a create-style diff (honest).
func readFileForDiff(relPath, repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, relPath))
	if err != nil {
		return ""
	}
	return string(data)
}
