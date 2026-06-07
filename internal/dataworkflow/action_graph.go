package dataworkflow

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	ActionStatusReady    = "ready"
	ActionStatusExecuted = "executed"
	ActionStatusFailed   = "failed"
	ActionStatusDeferred = "deferred"
	ActionStatusBlocked  = "blocked"
)

type ActionEvent struct {
	Actions []dataquery.DataAction
	Status  string
}

func ReduceActionGraph(events []ActionEvent, current []dataquery.DataAction, limit int) ActionGraph {
	graph := ActionGraph{}
	for _, event := range events {
		status := cleanActionStatus(event.Status, ActionStatusExecuted)
		graph.Executed = append(graph.Executed, ActionNodesFor(event.Actions, status)...)
	}
	if len(current) > 0 {
		graph.Ready = ActionNodesFor(current, ActionStatusReady)
	}
	if limit > 0 && len(graph.Executed) > limit {
		graph.Executed = append([]ActionNode(nil), graph.Executed[len(graph.Executed)-limit:]...)
	}
	return graph
}

func BlockedActionNodesFromViolations(violations []WorkflowViolation) []ActionNode {
	out := make([]ActionNode, 0, len(violations))
	for _, violation := range violations {
		kindText := strings.TrimSpace(violation.ActionKind)
		kind := dataquery.DataActionKind(kindText)
		var capability ActionCapability
		if kindText != "" {
			kind = NormalizeActionKind(kind)
			capability, _ = Capability(kind)
		}
		inputs := cleanActionAliases(violation.InputAliases)
		if len(inputs) == 0 && strings.TrimSpace(violation.InputAlias) != "" {
			inputs = cleanActionAliases([]string{violation.InputAlias})
		}
		key := strings.TrimSpace(violation.IdempotencyKey)
		if key == "" {
			key = blockedViolationKey(violation, inputs)
		}
		out = append(out, ActionNode{
			ID:               strings.TrimSpace(violation.ActionID),
			Kind:             string(kind),
			Status:           ActionStatusBlocked,
			DependencyRank:   capability.DependencyRank,
			IdempotencyKey:   key,
			InputAliases:     inputs,
			OutputAlias:      strings.TrimSpace(violation.OutputAlias),
			BlockedReason:    blockedViolationReason(violation),
			CanProduceLedger: ledgerKindStrings(capability.ProducesLedgers),
		})
	}
	return out
}

func ActionNodeFor(action dataquery.DataAction, status string) ActionNode {
	action, _ = NormalizeRolePathAction(action)
	kind := NormalizeActionKind(action.Kind)
	capability, _ := Capability(kind)
	return ActionNode{
		ID:               strings.TrimSpace(action.ID),
		Kind:             string(kind),
		Status:           cleanActionStatus(status, ActionStatusReady),
		DependencyRank:   capability.DependencyRank,
		IdempotencyKey:   ActionIdempotencyKey(action),
		InputAliases:     cleanActionAliases(action.InputPaths),
		OutputAlias:      strings.TrimSpace(action.OutputArtifact),
		CanProduceLedger: ledgerKindStrings(capability.ProducesLedgers),
	}
}

func blockedViolationKey(violation WorkflowViolation, inputs []string) string {
	payload, _ := json.Marshal([]any{
		strings.TrimSpace(violation.Code),
		strings.TrimSpace(violation.ActionID),
		strings.TrimSpace(violation.ActionKind),
		inputs,
		strings.TrimSpace(violation.OutputAlias),
		cleanStrings(violation.MissingFields),
	})
	return fmt.Sprintf("blocked:%s", hashActionText(string(payload)))
}

func blockedViolationReason(violation WorkflowViolation) string {
	code := strings.TrimSpace(violation.Code)
	reason := strings.TrimSpace(violation.Reason)
	if code == "" {
		return reason
	}
	if reason == "" {
		return code
	}
	return code + ": " + reason
}

func cleanActionStatus(status, fallback string) string {
	status = strings.TrimSpace(status)
	if status != "" {
		return status
	}
	return fallback
}

func ActionNodesFor(actions []dataquery.DataAction, status string) []ActionNode {
	out := make([]ActionNode, 0, len(actions))
	for _, action := range actions {
		out = append(out, ActionNodeFor(action, status))
	}
	return out
}

func ActionIdempotencyKey(action dataquery.DataAction) string {
	action, _ = NormalizeRolePathAction(action)
	kind := string(NormalizeActionKind(action.Kind))
	inputs := cleanActionAliases(action.InputPaths)
	output := strings.TrimSpace(action.OutputArtifact)
	params := stableActionParams(action.Params)
	scriptHash := ""
	if strings.TrimSpace(action.Script) != "" {
		scriptHash = hashActionText(action.Script)
	}
	payload, _ := json.Marshal([]any{kind, inputs, output, params, scriptHash})
	return fmt.Sprintf("%s:%s", kind, hashActionText(string(payload)))
}

func stableActionParams(params map[string]string) [][2]string {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, [2]string{strings.TrimSpace(key), strings.TrimSpace(params[key])})
	}
	return out
}

func cleanActionAliases(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func ledgerKindStrings(values []LedgerKind) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			out = append(out, string(value))
		}
	}
	return out
}

func hashActionText(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}
