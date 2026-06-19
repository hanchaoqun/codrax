package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

const writeFinalLoopEventRefLimit = 128

func writeFinalLoopSummaryFromRun(run *types.WriteWorkflowRun) *types.WriteFinalLoopSummary {
	if run == nil {
		return nil
	}
	events := loopkernel.EventsFromWriteWorkflowRun(*run)
	if len(events) == 0 {
		return nil
	}
	view := loopkernel.ReduceEvents(events)
	out := &types.WriteFinalLoopSummary{
		RunID:          strings.TrimSpace(view.RunID),
		Status:         string(view.Status),
		ActiveUnitID:   strings.TrimSpace(view.ActiveUnitID),
		EventCount:     view.EventCount,
		LastEventKind:  string(view.LastEventKind),
		LastReasonCode: strings.TrimSpace(view.LastReasonCode),
		EventRefs:      writeFinalLoopEventRefs(events, writeFinalLoopEventRefLimit),
		Truth:          view.Truth,
		Proof: types.WriteFinalAuthoritySummary{
			State:             string(view.Proof.State),
			ReasonCode:        view.Proof.ReasonCode,
			RecommendedAction: string(view.Proof.RecommendedAction),
		},
		Localization: types.WriteFinalAuthoritySummary{
			State:               string(view.Localization.State),
			ReasonCode:          view.Localization.ReasonCode,
			RecommendedAction:   string(view.Localization.RecommendedAction),
			SourcePaths:         append([]string(nil), view.Localization.SourcePaths...),
			OwnerSupportedPaths: append([]string(nil), view.Localization.OwnerSupportedPaths...),
			OwnerMissingPaths:   append([]string(nil), view.Localization.OwnerMissingPaths...),
		},
		Permission: types.WriteFinalAuthoritySummary{
			State:             string(view.Permission.State),
			ReasonCode:        view.Permission.ReasonCode,
			RecommendedAction: string(view.Permission.RecommendedAction),
			Source:            view.Permission.Source,
			RequiresUser:      view.Permission.RequiresUser,
		},
	}
	for _, unit := range view.Units {
		out.RuntimeUnits = append(out.RuntimeUnits, types.WriteFinalRuntimeUnitSummary{
			UnitID:     strings.TrimSpace(unit.UnitID),
			Status:     string(unit.Status),
			ReasonCode: strings.TrimSpace(unit.ReasonCode),
			Truth:      unit.Truth,
		})
	}
	normalized := types.NormalizeWriteFinalReport(types.WriteFinalReport{Loop: out})
	return normalized.Loop
}

func writeFinalLoopEventRefs(events []loopkernel.LoopEvent, limit int) []string {
	if limit <= 0 {
		limit = len(events)
	}
	refs := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		ref := strings.TrimSpace(event.ID)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
		if len(refs) >= limit {
			break
		}
	}
	return refs
}
