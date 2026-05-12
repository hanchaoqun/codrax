package types

import (
	"fmt"
	"strings"
)

// augmentCurrentStatusVerdictLane appends a typed verdict-synthesis
// lane to an existing plan when the active answer contract requires a
// bounded current-status verdict. The lane reuses every other lane's
// location anchors so the verdict block can cite historical
// observation + current-code verification + boundary evidence in one
// place. Returns the input plan unchanged when the contract does not
// require a verdict, when a verdict lane already exists, or when no
// candidate location anchors are present.
func augmentCurrentStatusVerdictLane(
	plan *AnswerSupportPlan,
	contract *CurrentStatusDiagnosticContract,
) *AnswerSupportPlan {
	if plan == nil || contract == nil || !contract.Required {
		return plan
	}
	for _, lane := range plan.Lanes {
		if lane.Kind == SupportLaneCurrentVerdict {
			return plan
		}
	}
	lane := compileCurrentStatusVerdictSupportLane(plan)
	if len(lane.Entries) == 0 {
		return plan
	}
	out := *plan
	out.Lanes = append(append([]AnswerSupportLane(nil), plan.Lanes...), lane)
	return &out
}

func compileCurrentStatusVerdictSupportLane(plan *AnswerSupportPlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneCurrentVerdict,
		Title:         "Current status verdict synthesis",
		AllowedBlocks: []string{"decision"},
		Guidance: "Use this lane only for the bounded verdict block. It may cite the historical " +
			"observation, current code verification, and boundary evidence together, but it must not " +
			"be rendered as path steps, diagram nodes, or a standalone mechanism story.",
	}
	if plan == nil {
		return lane
	}
	seen := make(map[string]struct{})
	for _, sourceLane := range plan.Lanes {
		if sourceLane.Kind == SupportLaneCurrentVerdict {
			continue
		}
		title := strings.TrimSpace(sourceLane.Title)
		if title == "" {
			title = string(sourceLane.Kind)
		}
		for _, entry := range sourceLane.Entries {
			location := strings.TrimSpace(entry.Location)
			if location == "" {
				continue
			}
			key := strings.ToLower(strings.ReplaceAll(location, `\`, `/`))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			text := strings.TrimSpace(entry.Text)
			if text == "" {
				text = location
			}
			entry.Text = fmt.Sprintf("%s verdict support: %s", title, text)
			entry.Location = location
			lane.Entries = append(lane.Entries, entry)
			if len(lane.Entries) >= 8 {
				return lane
			}
		}
	}
	return lane
}
