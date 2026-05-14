package orchestrator

import (
	"path"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

type selfConsistencyRowBlock struct {
	block types.AnswerBlock
}

// filterDeterministicRowOrderContradictions keeps the LLM reviewer in
// its intended advisory lane for row-order disputes. The reviewer may
// notice a claimed "alphabetic / lexicographic order" mismatch, but
// the hard rewrite decision must consume typed AnswerDocument rows and
// citation metadata when they are available.
func filterDeterministicRowOrderContradictions(doc *types.AnswerDocumentV2, verdict *SelfConsistencyResult) ([]SelfConsistencyContradiction, int) {
	if doc == nil || verdict == nil || len(verdict.Contradictions) == 0 {
		if verdict == nil {
			return nil, 0
		}
		return append([]SelfConsistencyContradiction(nil), verdict.Contradictions...), 0
	}
	remaining := make([]SelfConsistencyContradiction, 0, len(verdict.Contradictions))
	suppressed := 0
	for _, c := range verdict.Contradictions {
		if selfConsistencyContradictionClaimsSortedness(c, verdict.Reasoning) &&
			answerDocumentOrderingClaimDeterministicallySatisfied(doc, c, verdict.Reasoning) {
			suppressed++
			continue
		}
		remaining = append(remaining, c)
	}
	return remaining, suppressed
}

func selfConsistencyContradictionClaimsSortedness(c SelfConsistencyContradiction, _ string) bool {
	hay := strings.ToLower(strings.Join([]string{
		c.Topic,
		c.SummaryClaim,
		c.BodyClaim,
	}, " "))
	for _, token := range []string{
		"alphabetic",
		"alphabetical",
		"alphabetically",
		"alphabetized",
		"alphabetised",
		"lexicographic",
		"lexicographical",
		"lexically",
		"sorted",
		"sorting",
		"sort order",
	} {
		if strings.Contains(hay, token) {
			return true
		}
	}
	if strings.Contains(hay, "字母") {
		return true
	}
	if strings.Contains(hay, "排序") || strings.Contains(hay, "升序") || strings.Contains(hay, "降序") {
		return true
	}
	return false
}

func answerDocumentOrderingClaimDeterministicallySatisfied(doc *types.AnswerDocumentV2, c SelfConsistencyContradiction, reasoning string) bool {
	candidates := selfConsistencySortableRowBlocks(doc)
	if len(candidates) == 0 {
		return false
	}
	selected := selectSelfConsistencyRowBlocksForClaim(doc, candidates, c, reasoning)
	if len(selected) == 0 {
		return false
	}
	for _, candidate := range selected {
		if !selfConsistencyRowBlockHasSortedAxis(doc, candidate.block) {
			return false
		}
	}
	return true
}

func selfConsistencySortableRowBlocks(doc *types.AnswerDocumentV2) []selfConsistencyRowBlock {
	if doc == nil {
		return nil
	}
	out := make([]selfConsistencyRowBlock, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if len(block.Items) < 3 {
			continue
		}
		out = append(out, selfConsistencyRowBlock{block: block})
	}
	return out
}

func selectSelfConsistencyRowBlocksForClaim(
	doc *types.AnswerDocumentV2,
	candidates []selfConsistencyRowBlock,
	c SelfConsistencyContradiction,
	reasoning string,
) []selfConsistencyRowBlock {
	hay := strings.ToLower(strings.Join([]string{c.Topic, c.SummaryClaim, c.BodyClaim, reasoning}, " "))
	mentioned := make([]selfConsistencyRowBlock, 0, len(candidates))
	for _, candidate := range candidates {
		if selfConsistencyRowBlockMentioned(doc, candidate.block, hay) {
			mentioned = append(mentioned, candidate)
		}
	}
	if len(mentioned) > 0 {
		return mentioned
	}

	principal := make([]selfConsistencyRowBlock, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.block.SurfaceRole == types.SurfacePrincipal {
			principal = append(principal, candidate)
		}
	}
	if len(principal) > 0 {
		return principal
	}

	maxItems := 0
	for _, candidate := range candidates {
		if len(candidate.block.Items) > maxItems {
			maxItems = len(candidate.block.Items)
		}
	}
	largest := make([]selfConsistencyRowBlock, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.block.Items) == maxItems {
			largest = append(largest, candidate)
		}
	}
	return largest
}

func selfConsistencyRowBlockMentioned(doc *types.AnswerDocumentV2, block types.AnswerBlock, hay string) bool {
	for _, token := range []string{block.ID, block.Title} {
		if selfConsistencyClaimContainsToken(hay, token) {
			return true
		}
	}
	for _, item := range block.Items {
		if selfConsistencyClaimContainsToken(hay, item.Label) {
			return true
		}
		for _, key := range selfConsistencyItemCitationPathKeys(doc, item) {
			if selfConsistencyClaimContainsToken(hay, key) {
				return true
			}
		}
	}
	return false
}

func selfConsistencyClaimContainsToken(hay, token string) bool {
	token = normalizeSelfConsistencyRowSortKey(token)
	if len([]rune(token)) < 3 {
		return false
	}
	return strings.Contains(hay, strings.ToLower(token))
}

func selfConsistencyRowBlockHasSortedAxis(doc *types.AnswerDocumentV2, block types.AnswerBlock) bool {
	for _, axis := range selfConsistencyRowBlockSortableAxes(doc, block) {
		if selfConsistencyAxisIsAscending(axis) {
			return true
		}
	}
	return false
}

func selfConsistencyRowBlockSortableAxes(doc *types.AnswerDocumentV2, block types.AnswerBlock) [][]string {
	axes := make([][]string, 0, 2)
	if labels := selfConsistencyVisibleLabelAxis(block); len(labels) >= 3 {
		axes = append(axes, labels)
	}
	if citationKeys := selfConsistencyCitationPathAxis(doc, block); len(citationKeys) >= 3 {
		axes = append(axes, citationKeys)
	}
	return axes
}

func selfConsistencyVisibleLabelAxis(block types.AnswerBlock) []string {
	axis := make([]string, 0, len(block.Items))
	for _, item := range block.Items {
		label := strings.TrimSpace(item.Label)
		if label == "" {
			return nil
		}
		axis = append(axis, label)
	}
	return axis
}

func selfConsistencyCitationPathAxis(doc *types.AnswerDocumentV2, block types.AnswerBlock) []string {
	if doc == nil || len(doc.Citations) == 0 || len(block.Items) == 0 {
		return nil
	}
	files := make([]string, 0, len(block.Items))
	for _, item := range block.Items {
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
			return nil
		}
		file := normalizeSelfConsistencyCitationPath(doc.Citations[item.CitationRef].File)
		if file == "" {
			return nil
		}
		files = append(files, file)
	}
	keys := selfConsistencyPathAxisKeys(files)
	if len(keys) < 3 || selfConsistencyUniqueKeyCount(keys) != len(keys) {
		return nil
	}
	return keys
}

func selfConsistencyItemCitationPathKeys(doc *types.AnswerDocumentV2, item types.AnswerBlockItem) []string {
	if doc == nil || item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
		return nil
	}
	file := normalizeSelfConsistencyCitationPath(doc.Citations[item.CitationRef].File)
	if file == "" {
		return nil
	}
	return selfConsistencyPathAxisKeys([]string{file})
}

func selfConsistencyPathAxisKeys(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	parts := make([][]string, 0, len(files))
	for _, file := range files {
		p := splitSelfConsistencyPath(file)
		if len(p) == 0 {
			return nil
		}
		parts = append(parts, p)
	}
	common := commonSelfConsistencyDirPrefix(parts)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if common >= len(p) {
			return nil
		}
		rest := p[common:]
		key := rest[0]
		if strings.HasPrefix(key, "@") && len(rest) >= 2 {
			key = key + "/" + rest[1]
		}
		out = append(out, strings.TrimSuffix(key, path.Ext(key)))
	}
	return out
}

func commonSelfConsistencyDirPrefix(parts [][]string) int {
	if len(parts) == 0 {
		return 0
	}
	limit := len(parts[0]) - 1
	if limit < 0 {
		return 0
	}
	for _, p := range parts[1:] {
		if len(p)-1 < limit {
			limit = len(p) - 1
		}
	}
	common := 0
	for common < limit {
		want := parts[0][common]
		for _, p := range parts[1:] {
			if common >= len(p)-1 || p[common] != want {
				return common
			}
		}
		common++
	}
	return common
}

func splitSelfConsistencyPath(file string) []string {
	file = normalizeSelfConsistencyCitationPath(file)
	if file == "" {
		return nil
	}
	raw := strings.Split(file, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func normalizeSelfConsistencyCitationPath(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	file = strings.TrimPrefix(file, "./")
	for strings.Contains(file, "//") {
		file = strings.ReplaceAll(file, "//", "/")
	}
	return strings.Trim(file, "/")
}

func selfConsistencyAxisIsAscending(axis []string) bool {
	if len(axis) < 3 {
		return false
	}
	normalized := make([]string, 0, len(axis))
	for _, raw := range axis {
		key := normalizeSelfConsistencyRowSortKey(raw)
		if key == "" {
			return false
		}
		normalized = append(normalized, key)
	}
	if selfConsistencyUniqueKeyCount(normalized) < 2 {
		return false
	}
	for i := 1; i < len(normalized); i++ {
		if normalized[i-1] > normalized[i] {
			return false
		}
	}
	return true
}

func selfConsistencyUniqueKeyCount(axis []string) int {
	seen := make(map[string]struct{}, len(axis))
	for _, raw := range axis {
		key := normalizeSelfConsistencyRowSortKey(raw)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

func normalizeSelfConsistencyRowSortKey(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	s = strings.Trim(s, "`'\"“”‘’[](){}<>")
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsDigit(r) || r == '.' || r == ')' || r == '-' || r == '#'
	})
	s = strings.TrimSpace(s)
	return strings.Trim(s, "`'\"“”‘’[](){}<>")
}
