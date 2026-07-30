package dataquery

import (
	"fmt"
	"strings"
)

// Group-key lineage lanes for runner-minted contributions (EVALFIX-2D 类4).
// The GroupKey of every contribution minted by runComputeContributions lands
// on exactly one typed lineage lane:
//
//   - field values      — recordCompositeGroupKey over observed rows;
//   - declared literal  — the §15.12 group_key_literal channel (always a
//     constant label, never reinterpreted as a field name);
//   - plain constant    — a label colliding with no field and no ledger
//     schema name (e.g. "all_active");
//   - synthetic "all"   — the implicit default.
//
// There is NO fifth lane: the historical per-input convenience that silently
// demoted a group field name to a constant group label on inputs lacking the
// field minted phantom ledger groups with no data lineage — both sides of
// reconcile carried the same ghost key and agreed with themselves (eval gap
// F11). The interpretation is therefore decided ONCE per action, globally
// across all inputs, at compute time (the earliest honest point, records in
// hand).

// ContributionGroupKeyLineageRole marks the EVALFIX-2D 类4 hard violation: a
// compute_contributions constant group_key that is a member of the canonical
// ledger schema-name closed set (canonicalLedgerFieldNameSet) while NO input
// carries a field with that name. Minting that constant would produce a
// phantom ledger group with no data lineage that self-consistently passes
// reconcile. The typed escape is the §15.12 group_key_literal channel; the
// deterministic repair is the enrich/join missing-field family (dataworkflow
// missing_field_recovery, which recognizes this role next to
// "contribution_group_key").
const ContributionGroupKeyLineageRole = "contribution_group_key_lineage"

// computeContributionInput is one fully-read compute_contributions input
// (phase 1 census): every input is read BEFORE any contribution is minted so
// the group-key interpretation is decided once for the whole action.
type computeContributionInput struct {
	records []actionRecord
	headers []string
	total   int
	rel     string
}

// markKnownActionFields, actionRecordFieldExistsInRecords, and
// recordCompositeGroupKey are the lineage lanes' precise predicates: the
// field census, the verbatim field-key existence check the global
// interpretation decision reads, and the field-values lane constructor whose
// hand-built composition from row values IS the lineage guarantee.

func markKnownActionFields(out map[string]bool, headers []string, records []actionRecord) {
	mark := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	for _, header := range headers {
		mark(header)
	}
	for _, record := range records {
		for key := range record.Fields {
			mark(key)
		}
	}
}

func recordCompositeGroupKey(fields map[string]string, names []string) string {
	names = cleanStringList(names)
	if len(names) == 0 {
		return ""
	}
	values := make([]string, 0, len(names))
	for _, name := range names {
		value := recordField(fields, name)
		if strings.TrimSpace(value) == "" {
			return ""
		}
		values = append(values, value)
	}
	return strings.Join(values, "/")
}

func actionRecordFieldExistsInRecords(headers []string, records []actionRecord, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if len(records) > 0 {
		for _, record := range records {
			for key := range record.Fields {
				if strings.ToLower(strings.TrimSpace(key)) == name {
					return true
				}
			}
		}
		return false
	}
	for _, header := range headers {
		if strings.ToLower(strings.TrimSpace(header)) == name {
			return true
		}
	}
	return false
}

func (r ActionRunner) readComputeContributionInputs(paths []string, maxRecords int) ([]computeContributionInput, map[string]bool, error) {
	inputs := make([]computeContributionInput, 0, len(paths))
	globalKnownFields := map[string]bool{}
	for _, path := range paths {
		records, headers, total, rel, err := r.readActionRecords(path, maxRecords)
		if err != nil {
			return nil, nil, err
		}
		markKnownActionFields(globalKnownFields, headers, records)
		inputs = append(inputs, computeContributionInput{records: records, headers: headers, total: total, rel: rel})
	}
	return inputs, globalKnownFields, nil
}

// resolveContributionGroupKeyInterpretation performs the single global
// (per-action, not per-input) group-key interpretation:
//
//  1. group_key_literal declared → declared-literal lane, untouched.
//  2. group_key constant matching a field in ≥1 input → GLOBAL field
//     interpretation for every input; inputs lacking the field take the
//     existing typed contribution_source_skipped lane in the minting loop.
//  3. group_key constant matching no field anywhere but a member of the
//     canonical ledger schema-name closed set → typed hard violation
//     (ContributionGroupKeyLineageRole): the decidable degenerate case that
//     would mint a phantom ledger group. Both repair arms are stated and the
//     group_key_literal escape stays open.
//  4. otherwise → plain constant lane, byte-identical behavior.
//
// Both hard-gate predicates are PRECISE (verbatim field-key existence via
// actionRecordFieldExistsInRecords, verbatim closed-set membership); noisy
// signals never reach this gate (红线: 精确信号才配硬门).
func resolveContributionGroupKeyInterpretation(groupKeyLiteral string, groupKeyFields []string, groupKeyConst string, inputs []computeContributionInput, globalKnownFields map[string]bool) (fields []string, konst string, inferredFromConst bool, diagnostic string, err error) {
	if groupKeyLiteral != "" || len(groupKeyFields) > 0 || groupKeyConst == "" {
		return groupKeyFields, groupKeyConst, false, "", nil
	}
	for _, input := range inputs {
		if actionRecordFieldExistsInRecords(input.headers, input.records, groupKeyConst) {
			return []string{groupKeyConst}, "", true,
				fmt.Sprintf("group_key %q matched an input field; treated as group_key_field for all inputs", groupKeyConst), nil
		}
	}
	if canonicalLedgerFieldNameSet()[strings.ToLower(groupKeyConst)] {
		rels := make([]string, 0, len(inputs))
		for _, input := range inputs {
			rels = append(rels, input.rel)
		}
		return nil, "", false, "", DataFieldContractError{
			ActionKind:           DataActionComputeContribs,
			Role:                 ContributionGroupKeyLineageRole,
			Field:                groupKeyConst,
			Fields:               []string{groupKeyConst},
			InputAlias:           firstNonEmptyString(rels...),
			InputAliases:         rels,
			AvailableFieldSample: knownActionFieldNames(globalKnownFields),
			ActualSnippet:        fmt.Sprintf("group_key=%q", groupKeyConst),
			RepairHint:           "materialize the group field on the contribution rows first with enrich_records, join_records, or apply_entity_resolutions and retry compute_contributions; or, if a constant group label is truly intended, declare it explicitly with group_key_literal",
			Message: fmt.Sprintf("compute_contributions group_key %q is a ledger schema field name and no input record carries a field with that name; a constant group label equal to a schema name would mint a ledger group with no data lineage. If the intent is grouping by that field, materialize it with enrich_records or join_records (or apply_entity_resolutions) before retrying; if the intent is a constant group label, declare it explicitly with group_key_literal. inputs=[%s]; fields=[%s]",
				groupKeyConst,
				strings.Join(rels, ", "),
				strings.Join(clampStringSliceForError(knownActionFieldNames(globalKnownFields), 32), ", "),
			),
		}
	}
	return groupKeyFields, groupKeyConst, false, "", nil
}
