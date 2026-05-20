package orchestrator

import (
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var selfConsistencyCommitLineRE = regexp.MustCompile(`(?m)^commit\s+([0-9a-fA-F]{7,40})\s*$`)

// filterVCSHistoryRowOrderContradictions keeps VCS-history row-order review in
// the precise-signal lane. If the principal visible commit rows follow the
// exact order emitted by git_log, an LLM reviewer must not force a rewrite by
// inferring chronology from unrelated attributes such as patch size.
func filterVCSHistoryRowOrderContradictions(
	doc *types.AnswerDocumentV2,
	mut *types.MutableState,
	contradictions []SelfConsistencyContradiction,
) ([]SelfConsistencyContradiction, int) {
	if doc == nil || mut == nil || len(contradictions) == 0 {
		return append([]SelfConsistencyContradiction(nil), contradictions...), 0
	}
	labels := selfConsistencyPrincipalCommitLabels(doc)
	if len(labels) < 3 {
		return append([]SelfConsistencyContradiction(nil), contradictions...), 0
	}
	order := selfConsistencyGitLogCommitOrder(mut)
	if len(order) < len(labels) || !selfConsistencyCommitLabelsFollowOrder(labels, order) {
		return append([]SelfConsistencyContradiction(nil), contradictions...), 0
	}
	remaining := make([]SelfConsistencyContradiction, 0, len(contradictions))
	suppressed := 0
	for _, c := range contradictions {
		if c.Kind == SelfConsistencyContradictionRowOrder {
			suppressed++
			continue
		}
		remaining = append(remaining, c)
	}
	return remaining, suppressed
}

func selfConsistencyPrincipalCommitLabels(doc *types.AnswerDocumentV2) []string {
	if doc == nil {
		return nil
	}
	for _, block := range doc.Blocks {
		if block.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		labels := make([]string, 0, len(block.Items))
		for _, item := range block.Items {
			hash := normalizeSelfConsistencyCommitHash(item.Label)
			if hash == "" {
				return nil
			}
			labels = append(labels, hash)
		}
		if len(labels) >= 3 {
			return labels
		}
	}
	return nil
}

func selfConsistencyGitLogCommitOrder(mut *types.MutableState) []string {
	if mut == nil {
		return nil
	}
	var results []types.ToolResult
	if ta := mut.TurnAArtifacts(); ta != nil {
		results = append(results, ta.ToolResults...)
	}
	results = append(results, mut.DispatchToolResults()...)
	seen := make(map[string]struct{})
	var order []string
	for _, result := range results {
		if result.ToolName != "git_log" || !result.Success {
			continue
		}
		for _, match := range selfConsistencyCommitLineRE.FindAllStringSubmatch(result.Summary, -1) {
			hash := normalizeSelfConsistencyCommitHash(match[1])
			if hash == "" {
				continue
			}
			if _, ok := seen[hash]; ok {
				continue
			}
			seen[hash] = struct{}{}
			order = append(order, hash)
		}
	}
	return order
}

func selfConsistencyCommitLabelsFollowOrder(labels, order []string) bool {
	nextMin := -1
	for _, label := range labels {
		idx := selfConsistencyCommitPrefixIndex(label, order)
		if idx < 0 || idx <= nextMin {
			return false
		}
		nextMin = idx
	}
	return true
}

func selfConsistencyCommitPrefixIndex(prefix string, order []string) int {
	prefix = normalizeSelfConsistencyCommitHash(prefix)
	if len(prefix) < 7 {
		return -1
	}
	found := -1
	for i, hash := range order {
		if strings.HasPrefix(hash, prefix) {
			if found >= 0 {
				return -1
			}
			found = i
		}
	}
	return found
}

func normalizeSelfConsistencyCommitHash(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Trim(s, "`*_[](){}")
	if len(s) < 7 || len(s) > 40 {
		return ""
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return ""
	}
	return s
}
