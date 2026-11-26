package tracefinding

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CompleteRootCausesFromAnswerTable extends a nonempty selection at delivery
// with rows the accepted answer explicitly put in its on-chain cause table.
// The table identifies a candidate. The typed contract supplies every exported
// value, qualifier and evidence reference.
func CompleteRootCausesFromAnswerTable(report *types.TraceRootCauseReportV2, doc *types.AnswerDocumentV2, contract *types.TraceFindingContract) *types.TraceRootCauseReportV2 {
	if report == nil || len(report.RootCauses) == 0 || doc == nil ||
		contract == nil || !contract.RootCauseReportEnabled || contract.MultiArtifact() {
		return report
	}

	selection := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
	seen := make(map[string]bool)
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockTable || block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown ||
			strings.TrimSpace(block.Text) != "" ||
			!strings.HasPrefix(block.Title, "链上根因") || len(block.Columns) < 4 ||
			!slices.Equal(block.Columns[:4], []string{"座位", "线程", "类别", "有效归因"}) {
			continue
		}
		for _, row := range block.Items {
			if row.Label != "" || len(row.Cells) < 4 {
				continue
			}
			var matchedID string
			for _, candidate := range contract.Candidates {
				if !candidate.PrimaryEligible || !answerTableMatchesCause(block.Title, row.Cells, candidate.Decision) {
					continue
				}
				if _, ok := boundRootCauseItem(candidate); !ok {
					continue
				}
				if matchedID != "" {
					matchedID = ""
					break
				}
				matchedID = candidate.Decision.CandidateID
			}
			if matchedID != "" && !seen[matchedID] {
				seen[matchedID] = true
				selection.RootCauses = append(selection.RootCauses, &types.TraceRootCauseItemV2{CandidateID: matchedID})
			}
		}
	}
	if len(selection.RootCauses) == 0 {
		return report
	}
	confirmed, err := BindRootCauseReportSelection(selection, contract)
	if err != nil {
		return report
	}
	completed := *report
	completed.RootCauses = slices.Clone(report.RootCauses)
	for _, cause := range confirmed.RootCauses {
		if slices.ContainsFunc(completed.RootCauses, func(existing *types.TraceRootCauseItemV2) bool {
			return existing.ArtifactLabel == cause.ArtifactLabel && existing.Category == cause.Category &&
				slices.Equal(existing.Evidence, cause.Evidence)
		}) {
			continue
		}
		cause.Rank = len(completed.RootCauses) + 1
		completed.RootCauses = append(completed.RootCauses, cause)
	}
	return &completed
}

func answerTableMatchesCause(title string, cells []string, decision types.TraceCauseDecision) bool {
	facts, magnitude := decision.EvidenceFacts, decision.Magnitude
	if facts == nil || facts.LineStart <= 0 || facts.WindowScope == nil ||
		facts.WindowScope.Role != types.TraceQueryWindowScopeRequestedPrincipal ||
		magnitude == nil || magnitude.Caliber != types.TraceImpactCaliberEffectiveAttribution ||
		decision.Rank <= 0 {
		return false
	}
	window := fmt.Sprintf("窗口 %.6f..%.6f", facts.WindowScope.QueryWindowStartTs, facts.WindowScope.QueryWindowEndTs)
	if !strings.Contains(title, window+"，") ||
		cells[0] != "#"+strconv.Itoa(decision.Rank) ||
		cells[1] != decision.SubjectName || cells[3] != fmt.Sprintf("%.3f ms", magnitude.Value) {
		return false
	}
	if label := types.TraceSemanticWorkLabel(decision.Token.Token, true); label != "" {
		return cells[2] == label
	}
	switch decision.Token.Token {
	case "io_wait":
		return cells[2] == "IO 等待"
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		return cells[2] == "D 状态/IO 等待"
	}
	return false
}
