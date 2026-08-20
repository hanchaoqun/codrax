package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// completionStageRoleAuthorityGap describes one model-authored stage roster
// whose row-level support is narrower than the checkout-verified current-run
// stage authority. The member text and member_notes remain model-owned; this
// carrier contains only typed member indexes and provider-owned source rows.
type completionStageRoleAuthorityGap struct {
	FactIndex int
	Rows      []stageauthority.StageRow
	Missing   []completionStageRoleSupportGap
}

type completionStageRoleSupportGap struct {
	MemberIndex int
	Row         stageauthority.StageRow
	Actual      string
}

// completionReadModeStageRoleAuthorityGaps binds an exact model-authored
// current-stage roster to the same checkout-verified provider used by the
// finalizer prompt and diagram precedence validators. It never inspects the
// request text, completion reason, model reasoning, answer prose, or Mermaid
// labels. A partial/mixed roster is outside this exclusive authority and keeps
// the ordinary support-ref contract.
func completionReadModeStageRoleAuthorityGaps(
	ctx *types.BusContext,
	facts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) []completionStageRoleAuthorityGap {
	rows := completionSelectedReadModeStageRows(ctx, evidence)
	if len(rows) < 2 || len(facts) == 0 {
		return nil
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	var gaps []completionStageRoleAuthorityGap
	for factIndex, fact := range facts {
		mapped, ok := completionStageRoleFactRows(fact, rows)
		if !ok {
			continue
		}
		gap := completionStageRoleAuthorityGap{FactIndex: factIndex, Rows: mapped}
		for memberIndex, row := range mapped {
			actual := ""
			if memberIndex < len(fact.SupportRefs) {
				actual = completionSupportRefLocation(fact.SupportRefs[memberIndex], support)
			}
			expected := aggregateSupportLocationKey(row.File, row.Line)
			if completionStageAuthorityLocationsEqual(actual, expected) {
				continue
			}
			gap.Missing = append(gap.Missing, completionStageRoleSupportGap{
				MemberIndex: memberIndex,
				Row:         row,
				Actual:      actual,
			})
		}
		if len(gap.Missing) > 0 {
			gaps = append(gaps, gap)
		}
	}
	return gaps
}

func completionSelectedReadModeStageRows(ctx *types.BusContext, evidence []types.EvidenceItem) []stageauthority.StageRow {
	if ctx == nil || ctx.AnalysisIR == nil || (ctx.Mode != "" && ctx.Mode != types.ModeRead) {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	if !ok {
		return nil
	}
	selection := stageauthority.SelectRequiredReadModeWorkflow(ctx.AnalysisIR.RequestModel, evidence, authority)
	return append([]stageauthority.StageRow(nil), selection.Main...)
}

// completionStageRoleFactRows recognizes only an exact one-to-one selected
// stage slate. Candidate extraction is deliberately narrow: the raw member,
// its typed aggregate decorator base, and its explicit source-suffix prefix.
// It never applies a short-symbol tail to a qualified identity, so a helper
// named dataflow.Analyze cannot become the canonical analyze stage.
func completionStageRoleFactRows(fact types.AnswerAggregateFact, rows []stageauthority.StageRow) ([]stageauthority.StageRow, bool) {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) != len(rows) || len(rows) < 2 {
		return nil, false
	}
	mapped := make([]stageauthority.StageRow, len(fact.Members))
	used := make(map[int]bool, len(rows))
	for memberIndex, member := range fact.Members {
		matches := make([]int, 0, 1)
		for rowIndex, row := range rows {
			if completionStageMemberMatchesRow(member, row) {
				matches = append(matches, rowIndex)
			}
		}
		if len(matches) != 1 || used[matches[0]] {
			return nil, false
		}
		used[matches[0]] = true
		mapped[memberIndex] = rows[matches[0]]
	}
	if len(used) != len(rows) {
		return nil, false
	}
	return mapped, true
}

func completionStageMemberMatchesRow(member string, row stageauthority.StageRow) bool {
	for _, candidate := range completionStageMemberIdentityCandidates(member) {
		for _, alias := range row.IdentityAliases() {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(alias)) {
				return true
			}
		}
	}
	return false
}

func completionStageMemberIdentityCandidates(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	out := []string{member}
	if base, _, ok := types.AnswerAggregateDecoratedLabelParts(member); ok {
		if base = strings.TrimSpace(base); base != "" {
			out = append(out, base)
		}
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			if prefix := strings.TrimSpace(member[:idx]); prefix != "" {
				out = append(out, prefix)
			}
		}
	}
	return dedupStringsPreserveOrder(out)
}

func completionSupportRefLocation(ref string, support aggregateMemberSupportIndex) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	for _, candidate := range aggregateSupportRefLookupCandidates(ref) {
		if item, ok := support.byID[candidate]; ok {
			return aggregateSupportLocationKey(item.Source, item.LineStart)
		}
	}
	_, location, ok := aggregateMemberSupportRefPartsTolerant(ref)
	if !ok {
		return ""
	}
	return location
}

func completionStageAuthorityLocationsEqual(actual, expected string) bool {
	aFile, aLine := completionSplitSupportLocation(actual)
	eFile, eLine := completionSplitSupportLocation(expected)
	return aLine > 0 && aLine == eLine && canonicalRelationSourcePath(aFile) == canonicalRelationSourcePath(eFile)
}

func completionSplitSupportLocation(location string) (string, int) {
	surface, ok := types.ParseAnswerSourceLocationSurface(strings.TrimSpace(location))
	if !ok {
		return "", 0
	}
	return surface.File, surface.LineStart
}

func completionStageRoleAuthorityGapSummary(gaps []completionStageRoleAuthorityGap) string {
	var rows []string
	for _, gap := range gaps {
		for _, missing := range gap.Missing {
			actual := strings.TrimSpace(missing.Actual)
			if actual == "" {
				actual = "missing"
			}
			rows = append(rows, fmt.Sprintf(
				"aggregate_facts[%d].members[%d] stage=%s expected_support=%s:%d actual_support=%s",
				gap.FactIndex, missing.MemberIndex, missing.Row.StageIdent,
				missing.Row.File, missing.Row.Line, actual,
			))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "; ")
}

func completionStageRoleAuthorityBlockerKey(gaps []completionStageRoleAuthorityGap) uint32 {
	var identifiers []string
	for _, gap := range gaps {
		for _, missing := range gap.Missing {
			identifiers = append(identifiers, fmt.Sprintf(
				"fact=%d|member=%d|stage=%s|expected=%s:%d|actual=%s",
				gap.FactIndex, missing.MemberIndex, missing.Row.StageIdent,
				missing.Row.File, missing.Row.Line, strings.TrimSpace(missing.Actual),
			))
		}
	}
	return types.ComputeDowngradeTypedIdentifierSetKey(string(types.DowngradeLaneStageRoleAuthority), identifiers)
}

func queueCompletionStageRoleAuthorityRead(ctx *types.BusContext, gaps []completionStageRoleAuthorityGap) {
	if ctx == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil || len(gaps) == 0 {
		return
	}
	file := ""
	start, end := 0, 0
	for _, gap := range gaps {
		for _, row := range gap.Rows {
			if file == "" {
				file = row.File
			}
			if canonicalRelationSourcePath(file) != canonicalRelationSourcePath(row.File) || row.Line <= 0 {
				continue
			}
			if start == 0 || row.Line < start {
				start = row.Line
			}
			if row.Line > end {
				end = row.Line
			}
		}
	}
	if file == "" || start <= 0 || end <= 0 {
		return
	}
	end += 16
	ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:          types.RepairReadFile,
		Files:         []string{file},
		LineRanges:    []types.LineRange{{Start: start, End: end}},
		Tools:         []string{"read_file", "emit_investigation_complete"},
		Subject:       "current_read_stage_role_authority",
		Rationale:     "Read the checkout-verified current-run stage binding rows, then keep each model-authored stage responsibility row aligned to its exact binding support; unrelated homonymous helpers remain separate supporting facts.",
		Origin:        "emit_investigation_complete.stage_role_authority",
		DowngradeLane: types.DowngradeLaneStageRoleAuthority,
		Stage:         string(types.StageExplore),
	})
}

func completionStageRoleAuthorityRepair(gaps []completionStageRoleAuthorityGap) *types.ToolRepair {
	if len(gaps) == 0 {
		return nil
	}
	file := ""
	var lines []int
	seen := map[int]bool{}
	for _, gap := range gaps {
		for _, row := range gap.Rows {
			if file == "" {
				file = row.File
			}
			if canonicalRelationSourcePath(file) != canonicalRelationSourcePath(row.File) || row.Line <= 0 || seen[row.Line] {
				continue
			}
			seen[row.Line] = true
			lines = append(lines, row.Line)
		}
	}
	sort.Ints(lines)
	repair := &types.ToolRepair{
		Code: "current_read_stage_role_authority",
		Hint: "Use the exact checkout-verified stage_binding source row shown for each selected current-read stage as that member's aligned support_ref. Keep a homonymous helper/subsystem in a separate supporting fact unless an independent typed current-read relation connects it; do not keep searching for a same-name producer to redefine the stage.",
		Fields: []string{
			"emit_investigation_complete.aggregate_facts[].members",
			"emit_investigation_complete.aggregate_facts[].member_notes",
			"emit_investigation_complete.aggregate_facts[].support_refs",
		},
		Metadata: map[string]string{
			"repair_origin": "emit_investigation_complete.stage_role_authority",
			"lane":          string(types.DowngradeLaneStageRoleAuthority),
		},
	}
	if file != "" {
		repair.Targets = []types.ToolRepairTarget{{File: file, Lines: lines, Action: string(types.RepairReadFile)}}
	}
	return repair
}

func dropCompletionAggregateFactsForStageRoleGaps(facts []types.AnswerAggregateFact, gaps []completionStageRoleAuthorityGap) []types.AnswerAggregateFact {
	if len(facts) == 0 || len(gaps) == 0 {
		return facts
	}
	drop := make(map[int]bool, len(gaps))
	for _, gap := range gaps {
		drop[gap.FactIndex] = true
	}
	out := make([]types.AnswerAggregateFact, 0, len(facts)-len(drop))
	for i, fact := range facts {
		if !drop[i] {
			out = append(out, fact)
		}
	}
	return out
}
