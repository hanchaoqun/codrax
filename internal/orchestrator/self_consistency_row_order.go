package orchestrator

import (
	"path"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

type selfConsistencyRowOrderBlock struct {
	block types.AnswerBlock
}

// filterDeterministicRowOrderContradictions keeps the reviewer in its advisory
// lane for row-order disputes. It consumes only the typed contradiction_kind
// enum plus AnswerDocumentV2 rows; it does not inspect reviewer prose for
// keywords such as "sorted" or "alphabetic".
func filterDeterministicRowOrderContradictions(
	doc *types.AnswerDocumentV2,
	contradictions []SelfConsistencyContradiction,
) ([]SelfConsistencyContradiction, int) {
	if doc == nil || len(contradictions) == 0 {
		return append([]SelfConsistencyContradiction(nil), contradictions...), 0
	}
	suppressRowOrder := answerDocumentPrincipalRowsDeterministicallyOrdered(doc)
	remaining := make([]SelfConsistencyContradiction, 0, len(contradictions))
	suppressed := 0
	for _, c := range contradictions {
		if c.Kind == SelfConsistencyContradictionRowOrder && suppressRowOrder {
			suppressed++
			continue
		}
		remaining = append(remaining, c)
	}
	return remaining, suppressed
}

func answerDocumentPrincipalRowsDeterministicallyOrdered(doc *types.AnswerDocumentV2) bool {
	candidates := selfConsistencyPrincipalRowOrderBlocks(doc)
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if !selfConsistencyRowBlockHasSortedAxis(doc, candidate.block) {
			return false
		}
	}
	return true
}

func selfConsistencyPrincipalRowOrderBlocks(doc *types.AnswerDocumentV2) []selfConsistencyRowOrderBlock {
	if doc == nil {
		return nil
	}
	out := make([]selfConsistencyRowOrderBlock, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if block.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if len(block.Items) < 3 {
			continue
		}
		out = append(out, selfConsistencyRowOrderBlock{block: block})
	}
	return out
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
	s = strings.Trim(s, "`'\"[](){}<>")
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsDigit(r) || r == '.' || r == ')' || r == '-' || r == '#'
	})
	s = strings.TrimSpace(s)
	return strings.Trim(s, "`'\"[](){}<>")
}
