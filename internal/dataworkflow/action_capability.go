package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type LedgerKind string

const (
	LedgerRuleCoverage      LedgerKind = "rule_coverage"
	LedgerDecisions         LedgerKind = "decisions"
	LedgerEntityResolutions LedgerKind = "entity_resolutions"
	LedgerContributions     LedgerKind = "contributions"
	LedgerReconcile         LedgerKind = "reconcile"
	LedgerFinalProjection   LedgerKind = "final_projection"
)

type ActionCapability struct {
	Kind            dataquery.DataActionKind `json:"kind"`
	DependencyRank  int                      `json:"dependency_rank,omitempty"`
	ProducesLedgers []LedgerKind             `json:"produces_ledgers,omitempty"`
	ImpliesLedgers  []LedgerKind             `json:"implies_ledgers,omitempty"`
	LeafFallback    bool                     `json:"leaf_fallback,omitempty"`
	InputPaths      ActionInputPathContract  `json:"input_paths,omitempty"`
}

type ActionInputPathContract struct {
	Min             int  `json:"min,omitempty"`
	Max             int  `json:"max,omitempty"`
	SingleRecordSet bool `json:"single_record_set,omitempty"`
}

var actionCapabilities = map[dataquery.DataActionKind]ActionCapability{
	dataquery.DataActionMaterialInventory: {Kind: dataquery.DataActionMaterialInventory},
	dataquery.DataActionInspectMaterial: {
		Kind:       dataquery.DataActionInspectMaterial,
		InputPaths: ActionInputPathContract{Min: 1},
	},
	dataquery.DataActionExtractRecords: {
		Kind:       dataquery.DataActionExtractRecords,
		InputPaths: ActionInputPathContract{Min: 1},
	},
	dataquery.DataActionDeriveRules: {
		Kind:            dataquery.DataActionDeriveRules,
		DependencyRank:  1,
		ProducesLedgers: []LedgerKind{LedgerRuleCoverage},
		ImpliesLedgers:  []LedgerKind{LedgerRuleCoverage},
	},
	dataquery.DataActionDeriveFields: {
		Kind:           dataquery.DataActionDeriveFields,
		DependencyRank: 2,
		InputPaths:     ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionExtractFields: {
		Kind:           dataquery.DataActionExtractFields,
		DependencyRank: 2,
		InputPaths:     ActionInputPathContract{Min: 1},
	},
	dataquery.DataActionGroupRecords: {
		Kind:           dataquery.DataActionGroupRecords,
		DependencyRank: 2,
		InputPaths:     ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionExpandRecords: {
		Kind:           dataquery.DataActionExpandRecords,
		DependencyRank: 2,
		InputPaths:     ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionNormalizeEntities: {
		Kind:            dataquery.DataActionNormalizeEntities,
		DependencyRank:  2,
		ProducesLedgers: []LedgerKind{LedgerEntityResolutions},
		ImpliesLedgers:  []LedgerKind{LedgerEntityResolutions},
	},
	dataquery.DataActionApplyResolutions: {
		Kind:           dataquery.DataActionApplyResolutions,
		DependencyRank: 3,
	},
	dataquery.DataActionEnrichRecords: {
		Kind:            dataquery.DataActionEnrichRecords,
		DependencyRank:  3,
		ProducesLedgers: []LedgerKind{LedgerEntityResolutions},
		ImpliesLedgers:  []LedgerKind{LedgerEntityResolutions},
	},
	dataquery.DataActionJoinRecords: {
		Kind:           dataquery.DataActionJoinRecords,
		DependencyRank: 3,
		InputPaths:     ActionInputPathContract{Min: 2, Max: 2},
	},
	dataquery.DataActionFilterRecords: {
		Kind:            dataquery.DataActionFilterRecords,
		DependencyRank:  3,
		ProducesLedgers: []LedgerKind{LedgerDecisions},
		ImpliesLedgers:  []LedgerKind{LedgerDecisions},
		InputPaths:      ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionValueDistribution: {
		Kind:           dataquery.DataActionValueDistribution,
		DependencyRank: 3,
		InputPaths:     ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionQualifyRecords: {
		Kind:            dataquery.DataActionQualifyRecords,
		DependencyRank:  3,
		ProducesLedgers: []LedgerKind{LedgerDecisions},
		ImpliesLedgers:  []LedgerKind{LedgerDecisions},
		InputPaths:      ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionMappingCandidate: {
		Kind:           dataquery.DataActionMappingCandidate,
		DependencyRank: 2,
		InputPaths:     ActionInputPathContract{Min: 2, Max: 2},
	},
	dataquery.DataActionComputeContribs: {
		Kind:            dataquery.DataActionComputeContribs,
		DependencyRank:  4,
		ProducesLedgers: []LedgerKind{LedgerDecisions, LedgerContributions},
		// The runner derives decision audit rows from contribution rows, but
		// choosing compute does not by itself make an explicit decision ledger
		// a precondition. Conflating products with implied obligations made
		// normalization add decision_records_required=true and admission then
		// reject the very compute action that had just been advertised.
		ImpliesLedgers: []LedgerKind{LedgerContributions, LedgerReconcile},
		InputPaths:     ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	},
	dataquery.DataActionReconcile: {
		Kind:            dataquery.DataActionReconcile,
		DependencyRank:  5,
		ProducesLedgers: []LedgerKind{LedgerReconcile},
		ImpliesLedgers:  []LedgerKind{LedgerReconcile},
	},
	dataquery.DataActionAssembleAnswer: {
		Kind:            dataquery.DataActionAssembleAnswer,
		DependencyRank:  6,
		ProducesLedgers: []LedgerKind{LedgerFinalProjection},
	},
	dataquery.DataActionCustomTransform: {
		Kind:            dataquery.DataActionCustomTransform,
		ProducesLedgers: []LedgerKind{LedgerRuleCoverage, LedgerDecisions, LedgerEntityResolutions, LedgerContributions, LedgerReconcile, LedgerFinalProjection},
		LeafFallback:    true,
	},
}

func NormalizeActionKind(kind dataquery.DataActionKind) dataquery.DataActionKind {
	trimmed := strings.TrimSpace(string(kind))
	if trimmed == "" || strings.EqualFold(trimmed, string(dataquery.DataActionCustomTransform)) {
		return dataquery.DataActionCustomTransform
	}
	normalized := dataquery.DataActionKind(trimmed)
	if _, ok := actionCapabilities[normalized]; ok {
		return normalized
	}
	return normalized
}

func Capability(kind dataquery.DataActionKind) (ActionCapability, bool) {
	capability, ok := actionCapabilities[NormalizeActionKind(kind)]
	return capability, ok
}

// ActionTopology resolves the row-derivation topology (1:1 / 1:N / N:1 /
// none) an action kind declares. The table lives in dataquery beside the
// runner (V9-1 §40.15); this accessor delegates so the registry never grows a
// second copy, and TestActionTopologyDeclaredForEveryActionKind pins the
// bijection with actionCapabilities.
func ActionTopology(kind dataquery.DataActionKind) (dataquery.DataActionTopology, bool) {
	return dataquery.DataActionTopologyOf(NormalizeActionKind(kind))
}

// SupportedActionKinds returns the schema vocabulary accepted by the typed
// data-action runner. Keep this derived from actionCapabilities so admission,
// repair guidance, and execution cannot grow separate action-kind lists.
func SupportedActionKinds() []string {
	kinds := make([]string, 0, len(actionCapabilities))
	for kind := range actionCapabilities {
		kinds = append(kinds, string(kind))
	}
	sort.Strings(kinds)
	return kinds
}

func DependencyRank(kind dataquery.DataActionKind) int {
	capability, ok := Capability(kind)
	if !ok {
		return 0
	}
	return capability.DependencyRank
}

func ProducesLedger(kind dataquery.DataActionKind, ledger LedgerKind) bool {
	capability, ok := Capability(kind)
	if !ok {
		return false
	}
	for _, produced := range capability.ProducesLedgers {
		if produced == ledger {
			return true
		}
	}
	return false
}

// ImpliesLedgerObligation reports which validation ledgers become required
// merely by selecting an action. This is intentionally distinct from
// ProducesLedger: an action may emit an audit ledger as a by-product without
// making that ledger a prerequisite of the same action.
func ImpliesLedgerObligation(kind dataquery.DataActionKind, ledger LedgerKind) bool {
	capability, ok := Capability(kind)
	if !ok {
		return false
	}
	for _, implied := range capability.ImpliesLedgers {
		if implied == ledger {
			return true
		}
	}
	return false
}

func IsLeafFallback(kind dataquery.DataActionKind) bool {
	capability, ok := Capability(kind)
	return ok && capability.LeafFallback
}

func InputPathContract(kind dataquery.DataActionKind) (ActionInputPathContract, bool) {
	capability, ok := Capability(kind)
	if !ok {
		return ActionInputPathContract{}, false
	}
	if capability.InputPaths.Min == 0 && capability.InputPaths.Max == 0 && !capability.InputPaths.SingleRecordSet {
		return ActionInputPathContract{}, false
	}
	return capability.InputPaths, true
}

func RequiresInputPaths(kind dataquery.DataActionKind) bool {
	contract, ok := InputPathContract(kind)
	return ok && contract.Min > 0
}

func SingleRecordSetInput(kind dataquery.DataActionKind) bool {
	contract, ok := InputPathContract(kind)
	return ok && contract.SingleRecordSet
}

func SplitFirstDependencyRank(actions []dataquery.DataAction) (prefix []dataquery.DataAction, remainder []dataquery.DataAction, split bool) {
	if len(actions) < 2 {
		return append([]dataquery.DataAction(nil), actions...), nil, false
	}
	firstRank := 0
	for i, action := range actions {
		rank := DependencyRank(action.Kind)
		if rank == 0 {
			continue
		}
		if firstRank == 0 {
			firstRank = rank
			continue
		}
		if rank != firstRank {
			return append([]dataquery.DataAction(nil), actions[:i]...),
				append([]dataquery.DataAction(nil), actions[i:]...),
				true
		}
	}
	return append([]dataquery.DataAction(nil), actions...), nil, false
}

func CrossesDependencyRanks(actions []dataquery.DataAction) bool {
	_, _, split := SplitFirstDependencyRank(actions)
	return split
}

func SplitInitialDiscoveryDependencyRank(actions []dataquery.DataAction) (prefix []dataquery.DataAction, remainder []dataquery.DataAction, split bool) {
	prefix, remainder, split = SplitFirstDependencyRank(actions)
	if !split {
		return prefix, remainder, false
	}
	hasDiscovery := false
	for _, action := range prefix {
		if DependencyRank(action.Kind) == 0 && !IsLeafFallback(action.Kind) {
			hasDiscovery = true
			break
		}
	}
	if !hasDiscovery {
		return append([]dataquery.DataAction(nil), actions...), nil, false
	}
	return prefix, remainder, true
}
