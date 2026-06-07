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

type ActionGraphInput struct {
	Events       []ActionEvent
	Ready        []dataquery.DataAction
	Deferred     []dataquery.DataAction
	DeferredPlan dataquery.TaskPlan
	Blocked      []WorkflowViolation
	EventLimit   int
}

func ReduceActionGraph(events []ActionEvent, current []dataquery.DataAction, limit int) ActionGraph {
	return ReduceActionGraphState(ActionGraphInput{
		Events:     events,
		Ready:      current,
		EventLimit: limit,
	})
}

func ReduceActionGraphState(input ActionGraphInput) ActionGraph {
	graph := ActionGraph{}
	suppressed := map[string]bool{}
	for _, event := range input.Events {
		status := cleanActionStatus(event.Status, ActionStatusExecuted)
		nodes := ActionNodesFor(event.Actions, status)
		graph.Executed = append(graph.Executed, nodes...)
		if status == ActionStatusFailed || status == ActionStatusBlocked {
			for _, node := range nodes {
				if node.IdempotencyKey != "" {
					suppressed[node.IdempotencyKey] = true
				}
			}
		}
	}
	if len(input.Blocked) > 0 {
		graph.Blocked = BlockedActionNodesFromViolations(input.Blocked)
		for _, node := range graph.Blocked {
			if node.IdempotencyKey != "" {
				suppressed[node.IdempotencyKey] = true
			}
		}
	}
	if len(input.Ready) > 0 {
		graph.Ready = filterSuppressedActionNodes(ActionNodesFor(input.Ready, ActionStatusReady), suppressed)
	}
	deferredActions := input.Deferred
	if len(deferredActions) == 0 && len(input.DeferredPlan.Actions) > 0 {
		deferredActions = input.DeferredPlan.Actions
	}
	deferredActions = filterSuppressedDataActions(deferredActions, suppressed)
	if len(deferredActions) > 0 {
		graph.Deferred = ActionNodesFor(deferredActions, ActionStatusDeferred)
	}
	deferredPlan := input.DeferredPlan
	if len(deferredPlan.Actions) == 0 && len(deferredActions) > 0 {
		deferredPlan.Actions = deferredActions
	} else if len(deferredPlan.Actions) > 0 {
		deferredPlan.Actions = filterSuppressedDataActions(deferredPlan.Actions, suppressed)
	}
	if len(deferredPlan.Actions) > 0 {
		graph.DeferredPlan = cloneTaskPlan(deferredPlan)
	}
	if input.EventLimit > 0 && len(graph.Executed) > input.EventLimit {
		graph.Executed = append([]ActionNode(nil), graph.Executed[len(graph.Executed)-input.EventLimit:]...)
	}
	return graph
}

func filterSuppressedDataActions(actions []dataquery.DataAction, suppressed map[string]bool) []dataquery.DataAction {
	if len(actions) == 0 || len(suppressed) == 0 {
		return append([]dataquery.DataAction(nil), actions...)
	}
	out := make([]dataquery.DataAction, 0, len(actions))
	for _, action := range actions {
		key := ActionIdempotencyKey(action)
		if key != "" && suppressed[key] {
			continue
		}
		out = append(out, action)
	}
	return out
}

func cloneTaskPlan(plan dataquery.TaskPlan) *dataquery.TaskPlan {
	if len(plan.Actions) == 0 {
		return nil
	}
	out := plan
	out.InputPaths = append([]string(nil), plan.InputPaths...)
	out.Questions = append([]dataquery.Question(nil), plan.Questions...)
	out.KnownConstraints = append([]string(nil), plan.KnownConstraints...)
	out.MissingObservations = append([]string(nil), plan.MissingObservations...)
	out.SuccessCriteria = append([]string(nil), plan.SuccessCriteria...)
	out.Actions = make([]dataquery.DataAction, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		copied := action
		copied.InputPaths = append([]string(nil), action.InputPaths...)
		copied.SuccessCriteria = append([]string(nil), action.SuccessCriteria...)
		if action.Params != nil {
			copied.Params = make(map[string]string, len(action.Params))
			for key, value := range action.Params {
				copied.Params[key] = value
			}
		}
		out.Actions = append(out.Actions, copied)
	}
	return &out
}

func filterSuppressedActionNodes(nodes []ActionNode, suppressed map[string]bool) []ActionNode {
	if len(nodes) == 0 || len(suppressed) == 0 {
		return nodes
	}
	out := nodes[:0]
	for _, node := range nodes {
		if node.IdempotencyKey != "" && suppressed[node.IdempotencyKey] {
			continue
		}
		out = append(out, node)
	}
	return out
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
