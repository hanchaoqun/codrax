package dataworkflow

import (
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
	LeafFallback    bool                     `json:"leaf_fallback,omitempty"`
}

var actionCapabilities = map[dataquery.DataActionKind]ActionCapability{
	dataquery.DataActionMaterialInventory: {Kind: dataquery.DataActionMaterialInventory},
	dataquery.DataActionInspectMaterial:   {Kind: dataquery.DataActionInspectMaterial},
	dataquery.DataActionExtractRecords:    {Kind: dataquery.DataActionExtractRecords},
	dataquery.DataActionDeriveRules: {
		Kind:            dataquery.DataActionDeriveRules,
		DependencyRank:  1,
		ProducesLedgers: []LedgerKind{LedgerRuleCoverage},
	},
	dataquery.DataActionDeriveFields: {
		Kind:           dataquery.DataActionDeriveFields,
		DependencyRank: 2,
	},
	dataquery.DataActionExpandRecords: {
		Kind:           dataquery.DataActionExpandRecords,
		DependencyRank: 2,
	},
	dataquery.DataActionNormalizeEntities: {
		Kind:            dataquery.DataActionNormalizeEntities,
		DependencyRank:  2,
		ProducesLedgers: []LedgerKind{LedgerEntityResolutions},
	},
	dataquery.DataActionApplyResolutions: {
		Kind:           dataquery.DataActionApplyResolutions,
		DependencyRank: 3,
	},
	dataquery.DataActionEnrichRecords: {
		Kind:            dataquery.DataActionEnrichRecords,
		DependencyRank:  3,
		ProducesLedgers: []LedgerKind{LedgerEntityResolutions},
	},
	dataquery.DataActionJoinRecords: {
		Kind:           dataquery.DataActionJoinRecords,
		DependencyRank: 3,
	},
	dataquery.DataActionFilterRecords: {
		Kind:           dataquery.DataActionFilterRecords,
		DependencyRank: 3,
	},
	dataquery.DataActionQualifyRecords: {
		Kind:            dataquery.DataActionQualifyRecords,
		DependencyRank:  3,
		ProducesLedgers: []LedgerKind{LedgerDecisions},
	},
	dataquery.DataActionComputeContribs: {
		Kind:            dataquery.DataActionComputeContribs,
		DependencyRank:  4,
		ProducesLedgers: []LedgerKind{LedgerDecisions, LedgerContributions},
	},
	dataquery.DataActionReconcile: {
		Kind:            dataquery.DataActionReconcile,
		DependencyRank:  5,
		ProducesLedgers: []LedgerKind{LedgerReconcile},
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

func IsLeafFallback(kind dataquery.DataActionKind) bool {
	capability, ok := Capability(kind)
	return ok && capability.LeafFallback
}
