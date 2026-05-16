package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocInactiveScopeDisclosure adds a typed "Typed
// Inactive Scope Disclosure Contract" section to the finalizer
// prompt when the multi-repo workspace has at least one inactive
// sub-repo. The section names the inactive RootRels by their
// user-visible path and explains how the model can satisfy the
// disclosure obligation (typed scope_disclosure field or inline
// RootRel mention).
//
// The section is emitted regardless of whether the obligation will
// activate post-emit: when PendingSubRepos is non-empty, the model
// always has the option to disclose the boundary. Pre-emit
// activation is conservative (typed BusContext + RequestModel +
// AnswerDocumentV2 state), but the LLM should plan ahead when its
// answer is heading toward a bounded conclusion.
func renderAnswerDocInactiveScopeDisclosure(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	pending := canonicalNonEmpty(ctx.PendingSubRepos)
	if len(pending) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Typed Inactive Scope Disclosure Contract\n\n")
	b.WriteString("- Active set: ")
	if len(ctx.SubRepos) > 0 {
		active := activeSubRepoRootRels(ctx)
		if len(active) > 0 {
			fmt.Fprintf(&b, "%s\n", strings.Join(active, ", "))
		} else {
			b.WriteString("(all current sub-repos)\n")
		}
	} else {
		b.WriteString("(all current sub-repos)\n")
	}
	fmt.Fprintf(&b, "- Inactive sub-repos (not searched this turn): %s\n", strings.Join(pending, ", "))
	b.WriteString("\n")
	b.WriteString("This workspace has at least one sub-repo OUTSIDE the active set above. ")
	b.WriteString("If your principal answer is bounded — exact target absent in active scope, role-locate finds no match, or enumeration scoped to active — the answer MUST disclose the boundary so the operator can adjust the workspace and re-ask.\n\n")
	b.WriteString("Pick ONE of these structured channels to satisfy the disclosure obligation:\n")
	b.WriteString("1. Name an inactive sub-repo by its RootRel directly in a principal block / caveat (the RootRel string must appear verbatim — see list above).\n")
	b.WriteString("2. Set `scope_disclosure` on a caveat / decision block to one of:\n")
	b.WriteString("   - `inactive_scope_named` — the block cites a specific inactive RootRel.\n")
	b.WriteString("   - `out_of_active_scope` — the block asserts the target is outside the active scope without naming a specific RootRel.\n")
	b.WriteString("   - `requires_workspace_adjust` — the block recommends the operator adjust the workspace (e.g. via `/repos focus`) before retrying.\n\n")
	b.WriteString("Bounded answers without either form will be rejected: silently treating a missing target as global absence misleads the user when the target may live in an inactive sub-repo.\n\n")
	return b.String()
}

func canonicalNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(strings.ReplaceAll(s, "\\", "/"))
		if s == "" || s == "." || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func activeSubRepoRootRels(ctx *types.AgentContext) []string {
	if ctx == nil {
		return nil
	}
	pending := make(map[string]bool, len(ctx.PendingSubRepos))
	for _, p := range canonicalNonEmpty(ctx.PendingSubRepos) {
		pending[p] = true
	}
	out := make([]string, 0, len(ctx.SubRepos))
	for _, snap := range ctx.SubRepos {
		root := strings.TrimSpace(snap.RootRel)
		if root == "" || pending[root] {
			continue
		}
		out = append(out, root)
	}
	return out
}
