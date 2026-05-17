package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateInactiveScopeDisclosure records the post-emit disclosure signal for
// multi-repo answers bounded by an active sub-repo subset. The violation is
// soft by default; the user-facing disclosure is materialized by
// appendInactiveScopeSystemCaveat so the finalizer is not forced to rewrite
// model-authored content for a system/runtime boundary.
//
// Activation conditions and the disclosure obligation itself live
// in internal/types/inactive_scope_disclosure.go so the same predicate
// can be reused by future pre-emit / repair surfaces without
// duplicating activation logic.
func validateInactiveScopeDisclosure(doc *types.AnswerDocumentV2, busCtx *types.BusContext) []types.Violation {
	obligation := types.BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if !obligation.Active() {
		return nil
	}
	if types.AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		return nil
	}
	subject := obligation.RequestedSubject
	if subject == "" {
		subject = "the requested target"
	}
	pendingHint := strings.Join(obligation.PendingRootRels, ", ")
	if pendingHint == "" {
		pendingHint = "an inactive sub-repo"
	}
	return []types.Violation{{
		Kind: types.ViolInactiveScopeDisclosureMissing,
		Detail: fmt.Sprintf(
			"answer is bounded by the active sub-repo set (reason=%s) but does not disclose the inactive scope. The active set excluded: %s. The user-mentioned subject %q has no resolution in the active scope and may live in an inactive sub-repo.",
			obligation.Reason, pendingHint, subject,
		),
		Repair: fmt.Sprintf(
			"re-emit the answer so it either (a) names an inactive sub-repo by its RootRel (one of: %s) directly in a principal block / caveat, or (b) sets `scope_disclosure` on a block to inactive_scope_named / out_of_active_scope / requires_workspace_adjust. Without this disclosure the user cannot tell whether %q is truly absent or simply outside the current active scope.",
			pendingHint, subject,
		),
		Stage: string(types.StageFinalize),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.scope_disclosure",
			Reason:     "multi-repo bounded answer omitted inactive-scope disclosure",
			Confidence: 0.9,
		},
		ClusterKey: types.IdentityClusterKey(
			fmt.Sprintf("inactive_scope_disclosure:%s", strings.Join(obligation.PendingRootRels, ",")),
			"missing",
		),
	}}
}

func appendInactiveScopeSystemCaveat(answer string, doc *types.AnswerDocumentV2, busCtx *types.BusContext) string {
	note := inactiveScopeSystemCaveat(doc, busCtx)
	if note == "" {
		return answer
	}
	lang := ""
	if busCtx != nil {
		lang = busCtx.Language
	}
	return AppendSystemCaveatString(answer, note, lang)
}

func inactiveScopeSystemCaveat(doc *types.AnswerDocumentV2, busCtx *types.BusContext) string {
	obligation := types.BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if !obligation.Active() {
		return ""
	}
	if types.AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		return ""
	}
	pending := formatInactiveScopeRootList(obligation.PendingRootRels)
	if pending == "" {
		return ""
	}
	if busCtx != nil && isChineseLang(busCtx.Language) {
		return fmt.Sprintf("本回答仅覆盖当前已启用的子仓范围；还有未启用的子仓：%s。若目标可能在这些子仓中，请调整仓库关注范围后再查询。", pending)
	}
	return fmt.Sprintf("This answer only covers the currently active sub-repositories; inactive sub-repositories also exist: %s. If the target may live there, adjust the repository focus and rerun the request.", pending)
}

func formatInactiveScopeRootList(roots []string) string {
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		out = append(out, inlineCode(root))
	}
	return strings.Join(out, ", ")
}
