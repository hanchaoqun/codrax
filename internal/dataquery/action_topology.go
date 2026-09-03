package dataquery

import "sort"

// DataActionTopology is the row-derivation topology an action kind declares
// (V9-1, colleague_merge_audit §40.15). Row identity follows this topology:
//
//   - 1:1 — every output row derives from exactly one input row and INHERITS
//     its `_row_identity` unchanged (filter, qualify, derive, enrich, ...).
//   - 1:N — one input row fans out into several derived rows; each derived
//     row is minted `<parent identity>#<ordinal>` by stampDerivedRowIdentity
//     while `_source_locator` keeps the parent lineage (expand, join, mapping
//     candidates).
//   - N:1 — several input rows fold into one output row; the row gets an
//     artifact-local group identity (`<input>#group#<ordinal>`) and carries
//     `_source_locators` as its lineage list (group_records).
//   - none — the action does not emit identity-bearing record rows.
//
// The table is the single source consulted by the runner; dataworkflow's
// action registry (actionCapabilities) is pinned as a bijection with it, so
// registering a new action kind without declaring its topology is a red test,
// not a customer ledger surprise.
type DataActionTopology string

const (
	ActionTopologyOneToOne  DataActionTopology = "1:1"
	ActionTopologyOneToMany DataActionTopology = "1:N"
	ActionTopologyManyToOne DataActionTopology = "N:1"
	ActionTopologyNone      DataActionTopology = "none"
)

var dataActionTopologies = map[DataActionKind]DataActionTopology{
	DataActionMaterialInventory: ActionTopologyNone,
	DataActionInspectMaterial:   ActionTopologyNone,
	DataActionExtractRecords:    ActionTopologyOneToOne,
	DataActionDeriveRules:       ActionTopologyNone,
	DataActionDeriveFields:      ActionTopologyOneToOne,
	DataActionExtractFields:     ActionTopologyOneToOne,
	DataActionGroupRecords:      ActionTopologyManyToOne,
	DataActionExpandRecords:     ActionTopologyOneToMany,
	DataActionFilterRecords:     ActionTopologyOneToOne,
	DataActionValueDistribution: ActionTopologyNone,
	DataActionQualifyRecords:    ActionTopologyOneToOne,
	// mapping_candidate fans one source value into N candidate rows, but
	// those rows are match diagnostics (source_index/candidate_rank), not
	// ledger-identity-bearing records — no identity stamp.
	DataActionMappingCandidate:  ActionTopologyOneToMany,
	DataActionNormalizeEntities: ActionTopologyNone,
	DataActionApplyResolutions:  ActionTopologyOneToOne,
	DataActionEnrichRecords:     ActionTopologyOneToOne,
	DataActionJoinRecords:       ActionTopologyOneToMany,
	DataActionComputeContribs:   ActionTopologyOneToOne,
	DataActionReconcile:         ActionTopologyNone,
	DataActionAssembleAnswer:    ActionTopologyNone,
	// The script lane mints its own rows/ledgers; the typed runner stamps no
	// identity for it, so it declares no row topology.
	DataActionCustomTransform: ActionTopologyNone,
}

// DataActionTopologyOf resolves the declared derivation topology of an action
// kind through the same spelling normalization the runner dispatch uses.
func DataActionTopologyOf(kind DataActionKind) (DataActionTopology, bool) {
	topology, ok := dataActionTopologies[normalizeDataActionKind(kind)]
	return topology, ok
}

// DeclaredTopologyActionKinds lists every kind the topology table names, in
// stable order, for the registry-bijection census.
func DeclaredTopologyActionKinds() []DataActionKind {
	kinds := make([]DataActionKind, 0, len(dataActionTopologies))
	for kind := range dataActionTopologies {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}
