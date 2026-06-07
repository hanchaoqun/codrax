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

func ActionNodeFor(action dataquery.DataAction, status string) ActionNode {
	kind := NormalizeActionKind(action.Kind)
	capability, _ := Capability(kind)
	return ActionNode{
		ID:               strings.TrimSpace(action.ID),
		Kind:             string(kind),
		Status:           strings.TrimSpace(status),
		DependencyRank:   capability.DependencyRank,
		IdempotencyKey:   ActionIdempotencyKey(action),
		InputAliases:     cleanActionAliases(action.InputPaths),
		OutputAlias:      strings.TrimSpace(action.OutputArtifact),
		CanProduceLedger: ledgerKindStrings(capability.ProducesLedgers),
	}
}

func ActionNodesFor(actions []dataquery.DataAction, status string) []ActionNode {
	out := make([]ActionNode, 0, len(actions))
	for _, action := range actions {
		out = append(out, ActionNodeFor(action, status))
	}
	return out
}

func ActionIdempotencyKey(action dataquery.DataAction) string {
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
