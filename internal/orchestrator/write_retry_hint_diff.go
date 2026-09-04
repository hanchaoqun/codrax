package orchestrator

// write_retry_hint_diff.go — the bounded per-path plan content diff that
// backs the "current vs best" regression delta in write_retry_hint.go
// (buildRetryHintWithBest is the single production caller; the tests in
// verify_retry_test.go exercise it directly). Extracted from
// write_retry_hint.go under the IR-delivery LOC ratchet (§40.55 收编复核
// 再收编, b6f2 #10): the concern file had absorbed buildRetryHint's godoc
// and its 300-line row may not be raised, so this concern moved out with a
// fresh row of its own rather than the row moving up. Write path only; the
// read scheduler loop never reaches this file (L1).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retryHintDiffMaxBytes caps the unified-diff section appended to
// retry hints on regression. 4 KB fits typical small-file
// edits (an exercism-shape stub diff is usually <500 B; a complex
// refactor diff is usually <2 KB) while leaving headroom for the
// reflector critique + heuristic hint above it. Diffs that exceed
// the cap are truncated mid-hunk with an explicit "(truncated …)"
// marker; the planner still sees the first hunks, which usually
// carry the regression signal.
const retryHintDiffMaxBytes = 4096

// buildPlanContentDiff returns a unified diff between best and current
// plan contents, keyed by overlapping FileChange.Path. Empty string
// when either plan is nil, no paths overlap, or all overlapping paths
// have identical content.
//
// Per FileChange.Kind:
//   - "create" / "modify": diff the two NewContent blobs. Most common
//     case for exercism-shape tasks where the LLM rewrites a stub.
//   - "patch": diff the two Patch payloads (each is itself a unified
//     diff; the resulting "diff of diffs" is admittedly noisy but
//     still informative — you can see which hunks the planner removed
//     or added between iterations).
//   - "delete": no diff produced (kind change between best and current
//     is rare; if it happens, the path-list section above flags it).
//
// Output order: alphabetical by path so the prompt is deterministic
// across runs (otherwise prompt cache invalidates on every regenerate).
//
// The total diff is capped at maxBytes; once the cap is reached, the
// remaining paths are summarized as "(N more files truncated)" so the
// hint stays bounded.
func buildPlanContentDiff(best *types.ChangePlan, current *types.ChangePlan, maxBytes int) string {
	if best == nil || current == nil {
		return ""
	}
	bestByPath := make(map[string]types.FileChange, len(best.Changes))
	for _, c := range best.Changes {
		bestByPath[c.Path] = c
	}
	overlapping := make([]string, 0, len(current.Changes))
	for _, c := range current.Changes {
		if _, ok := bestByPath[c.Path]; ok {
			overlapping = append(overlapping, c.Path)
		}
	}
	if len(overlapping) == 0 {
		return ""
	}
	sort.Strings(overlapping)
	curByPath := make(map[string]types.FileChange, len(current.Changes))
	for _, c := range current.Changes {
		curByPath[c.Path] = c
	}
	var b strings.Builder
	truncatedAt := -1
	for i, p := range overlapping {
		if b.Len() >= maxBytes {
			truncatedAt = i
			break
		}
		bc := bestByPath[p]
		cc := curByPath[p]
		var bestText, curText string
		switch {
		case bc.Kind == "patch" || cc.Kind == "patch":
			// Both kind=patch (or kind transitioned) — diff the patch
			// payloads themselves so the planner sees how the patch
			// content shifted. If exactly one side has Patch and the
			// other has NewContent, fall through to NewContent diff
			// against empty for the patch side (rough but informative).
			bestText = bc.Patch
			curText = cc.Patch
			if bestText == "" {
				bestText = bc.NewContent
			}
			if curText == "" {
				curText = cc.NewContent
			}
		default:
			bestText = bc.NewContent
			curText = cc.NewContent
		}
		if bestText == curText {
			continue
		}
		d := udiff.Unified("best/"+p, "current/"+p, bestText, curText)
		if d == "" {
			continue
		}
		// Keep the diff bounded per file too — a single 10K-line file
		// rewrite shouldn't crowd out other paths.
		const perFileCap = 2048
		if len(d) > perFileCap {
			d = types.CutPrefixRuneSafe(d, perFileCap) + "\n… (per-file diff truncated)\n"
		}
		// If appending d would overflow the total cap, truncate to
		// what fits + a marker.
		if b.Len()+len(d) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 64 {
				b.WriteString(types.CutPrefixRuneSafe(d, remaining))
				b.WriteString("\n… (truncated)\n")
			}
			truncatedAt = i + 1
			break
		}
		b.WriteString(d)
	}
	if truncatedAt > 0 && truncatedAt < len(overlapping) {
		fmt.Fprintf(&b, "\n(+%d more files omitted)\n", len(overlapping)-truncatedAt)
	}
	return b.String()
}
