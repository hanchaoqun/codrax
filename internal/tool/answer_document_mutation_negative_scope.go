package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const currentSourceNegativeScopeAuthorityBlockID = "current_source_negative_scope_authority"

type currentSourceNegativeScopeAuthorityRow struct {
	Producer string
	Pattern  string
	Scope    string
}

// materializeCurrentSourceNegativeScopeAuthority publishes the exact target
// and scope of verified current-source no-match queries. It deliberately does
// not inspect the request or answer prose and does not infer a global absence:
// each row authorizes only its own typed query boundary.
func materializeCurrentSourceNegativeScopeAuthority(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace {
		return false
	}
	rows := currentSourceNegativeScopeAuthorityRows(ctx)
	if len(rows) == 0 {
		return false
	}

	const maxRows = 8
	total := len(rows)
	if total > maxRows {
		rows = rows[:maxRows]
	}
	zh := !strings.EqualFold(strings.TrimSpace(ctx.AnalysisIR.AnswerContract.Language), "en") &&
		!strings.EqualFold(strings.TrimSpace(ctx.AnalysisIR.RequestModel.Language), "en")
	block := types.AnswerBlock{
		Kind:                types.BlockCaveat,
		SystemGeneratedKind: types.AnswerSystemGeneratedNegativeSearchAuthority,
	}
	var b strings.Builder
	if zh {
		block.Title = "未命中结果的搜索范围"
		b.WriteString("以下每行只说明指定查询在指定范围内返回 0 个匹配。结果不能外推到其他文件、目录、配置层或仓库，也不能用另一目标的正向结果证明当前目标不存在。\n\n")
	} else {
		block.Title = "Search scope for no-match results"
		b.WriteString("Each row says only that the stated query returned zero matches inside its stated scope. It does not extend to another file, directory, configuration layer, or repository, and positive evidence for another target cannot prove this target absent.\n\n")
	}
	for _, row := range rows {
		if zh {
			fmt.Fprintf(
				&b,
				"- 来源=%s；查询=`%s`；范围=`%s`；匹配数=0\n",
				currentSourceNegativeScopeProducerLabel(row.Producer, true),
				currentSourceNegativeScopeAuthorityCell(row.Pattern),
				currentSourceNegativeScopeAuthorityCell(row.Scope),
			)
		} else {
			fmt.Fprintf(
				&b,
				"- source=%s; query=`%s`; scope=`%s`; matches=0\n",
				currentSourceNegativeScopeProducerLabel(row.Producer, false),
				currentSourceNegativeScopeAuthorityCell(row.Pattern),
				currentSourceNegativeScopeAuthorityCell(row.Scope),
			)
		}
	}
	if total > len(rows) {
		if zh {
			fmt.Fprintf(&b, "\n这里只展示 %d/%d 条查询；未展示的查询不能据此判定存在或不存在。", len(rows), total)
		} else {
			fmt.Fprintf(&b, "\nThis compact view shows %d/%d queries; omitted queries remain neither proven present nor proven absent.", len(rows), total)
		}
	}
	if zh {
		b.WriteString("\n\n未列出的范围仍属未验证，不能跨目标借用证据。")
	} else {
		b.WriteString("\n\nUnlisted scopes remain unproven, and evidence cannot be borrowed across targets.")
	}
	block.Text = b.String()

	if index := currentSourceNegativeScopeAuthorityBlockIndex(doc); index >= 0 {
		block.ID = doc.Blocks[index].ID
		changed := types.AnswerBlockVisibleSurface(doc.Blocks[index]) != types.AnswerBlockVisibleSurface(block) ||
			doc.Blocks[index].SystemGeneratedKind != block.SystemGeneratedKind
		doc.Blocks[index] = block
		return changed
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		return false
	}
	block.ID = currentSourceNegativeScopeAuthorityUniqueBlockID(doc)
	doc.Blocks = append(doc.Blocks, block)
	return true
}

func currentSourceNegativeScopeProducerLabel(producer string, zh bool) string {
	switch strings.TrimSpace(producer) {
	case "verified_negative_evidence":
		if zh {
			return "已验证的精确未命中证据"
		}
		return "verified exact no-match evidence"
	case "typed_grep_no_match":
		if zh {
			return "grep 完整未命中结果"
		}
		return "complete grep no-match result"
	default:
		if zh {
			return "已验证查询"
		}
		return "verified query"
	}
}

func currentSourceNegativeScopeAuthorityRows(ctx *types.BusContext) []currentSourceNegativeScopeAuthorityRow {
	if ctx == nil {
		return nil
	}
	var evidence []types.EvidenceItem
	evidence = append(evidence, ctx.EvidenceItems...)
	if ctx.Mutable != nil {
		evidence = append(evidence, ctx.Mutable.EmittedEvidence()...)
	}
	var rows []currentSourceNegativeScopeAuthorityRow
	for _, item := range evidence {
		if item.Kind != types.EvidenceAbsent ||
			item.Scope != types.ScopeNegative ||
			item.NegativeQuery == nil ||
			!item.NegativeScope.IsValid() ||
			(item.GroundingStatus != types.GroundingGrounded &&
				item.GroundingStatus != types.GroundingRecovered) {
			continue
		}
		pattern := strings.TrimSpace(item.NegativeQuery.Pattern)
		file := strings.TrimSpace(item.NegativeQuery.File)
		if pattern == "" || file == "" {
			continue
		}
		scope := file + " (" + string(item.NegativeScope) + ")"
		if item.NegativeScope == types.NegativeScopeSection &&
			strings.TrimSpace(item.NegativeQuery.Section) != "" {
			scope += " section=" + strings.TrimSpace(item.NegativeQuery.Section)
		}
		rows = append(rows, currentSourceNegativeScopeAuthorityRow{
			Producer: "verified_negative_evidence",
			Pattern:  pattern,
			Scope:    scope,
		})
	}
	// A free-standing navigation miss is not answer authority. Only attach
	// typed grep no-match rows after at least one verified negative evidence
	// anchor has established that this turn contains an intentional absence
	// investigation.
	if len(rows) == 0 {
		return nil
	}
	for _, result := range ctx.ToolResults {
		discovery := result.PathDiscovery
		if !result.Success || discovery == nil ||
			discovery.Kind != types.ToolPathDiscoveryKindGrep ||
			!discovery.NoMatches || discovery.ResultCount != 0 ||
			strings.TrimSpace(discovery.Pattern) == "" ||
			strings.TrimSpace(discovery.Path) == "" ||
			currentSourceNegativeScopePathDiscoveryIncomplete(result) {
			continue
		}
		scope := strings.TrimSpace(discovery.Path)
		if include := strings.TrimSpace(discovery.Include); include != "" {
			scope += "; include=" + include
		}
		if fileType := strings.TrimSpace(discovery.FileType); fileType != "" {
			scope += "; file_type=" + fileType
		}
		if discovery.FilesOnly {
			scope += "; files_only=true"
		}
		rows = append(rows, currentSourceNegativeScopeAuthorityRow{
			Producer: "typed_grep_no_match",
			Pattern:  strings.TrimSpace(discovery.Pattern),
			Scope:    scope,
		})
	}
	return normalizeCurrentSourceNegativeScopeAuthorityRows(rows)
}

func currentSourceNegativeScopePathDiscoveryIncomplete(result types.ToolResult) bool {
	if result.PathDiscovery == nil || result.PathDiscovery.CandidateFilesTruncated {
		return true
	}
	hint := result.Refinement
	return hint != nil &&
		(hint.ResultTruncated ||
			hint.CandidateBudgetTruncated ||
			len(hint.SkippedLargeCandidates) > 0)
}

func normalizeCurrentSourceNegativeScopeAuthorityRows(in []currentSourceNegativeScopeAuthorityRow) []currentSourceNegativeScopeAuthorityRow {
	if len(in) == 0 {
		return nil
	}
	out := make([]currentSourceNegativeScopeAuthorityRow, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, row := range in {
		row.Producer = strings.TrimSpace(row.Producer)
		row.Pattern = strings.TrimSpace(row.Pattern)
		row.Scope = strings.TrimSpace(row.Scope)
		if row.Producer == "" || row.Pattern == "" || row.Scope == "" {
			continue
		}
		key := row.Producer + "\x00" + row.Pattern + "\x00" + row.Scope
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].Producer < out[j].Producer
	})
	return out
}

func currentSourceNegativeScopeAuthorityCell(raw string) string {
	raw = strings.Join(strings.Fields(raw), " ")
	raw = strings.ReplaceAll(raw, "`", "'")
	const max = 220
	runes := []rune(raw)
	if len(runes) <= max {
		return raw
	}
	return string(runes[:max-1]) + "…"
}

func currentSourceNegativeScopeAuthorityBlockIndex(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return -1
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].SystemGeneratedKind == types.AnswerSystemGeneratedNegativeSearchAuthority {
			return i
		}
	}
	return -1
}

func currentSourceNegativeScopeAuthorityUniqueBlockID(doc *types.AnswerDocumentV2) string {
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		used[strings.TrimSpace(block.ID)] = true
	}
	if !used[currentSourceNegativeScopeAuthorityBlockID] {
		return currentSourceNegativeScopeAuthorityBlockID
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_system%d", currentSourceNegativeScopeAuthorityBlockID, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}
