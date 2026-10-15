package agent

import (
	"encoding/json"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/types"
)

type samplingFactViewKey struct {
	observation string
	view        types.RuntimeMeasurementView
}

// The roster records actual complete rows and every note it wrote, after its
// existing group/row budgets. Merely making a selector available is not a
// publication receipt. Each fixed group must publish all its own views.
type samplingFactRosterPublication struct {
	tables map[samplingFactViewKey]types.RuntimeMeasurementTable
	notes  map[string]map[string]bool
}

func (p *samplingFactRosterPublication) table(table types.RuntimeMeasurementTable, rows int) {
	if !table.HasNativeSamplingFacts() || rows != len(table.Rows) {
		return
	}
	if p.tables == nil {
		p.tables = make(map[samplingFactViewKey]types.RuntimeMeasurementTable)
	}
	p.tables[samplingFactViewKey{table.ObservationID, table.View}] = table
}

func (p *samplingFactRosterPublication) note(table types.RuntimeMeasurementTable, note string) {
	if !table.HasNativeSamplingFacts() {
		return
	}
	if p.notes == nil {
		p.notes = make(map[string]map[string]bool)
	}
	if p.notes[table.ObservationID] == nil {
		p.notes[table.ObservationID] = make(map[string]bool)
	}
	p.notes[table.ObservationID][note] = true
}

func (p *samplingFactRosterPublication) complete(expected types.RuntimeMeasurementPublication) bool {
	if len(expected.Tables) == 0 {
		return false
	}
	for _, table := range expected.Tables {
		actual, found := p.tables[samplingFactViewKey{table.ObservationID, table.View}]
		// Private qualifiers may be added by the current contract. Compare the
		// entire public table, never only its counts, label or observation ID.
		want, _ := json.Marshal(table)
		got, _ := json.Marshal(actual)
		if !found || !actual.HasNativeSamplingFacts() || string(want) != string(got) {
			return false
		}
		for _, note := range table.Notes {
			if !p.notes[table.ObservationID][note] {
				return false
			}
		}
	}
	return true
}

func (p *samplingFactRosterPublication) publish(ctx *types.AgentContext, receipt *answerDocNativePresentationReceipt) {
	if receipt == nil || ctx == nil || ctx.Mutable == nil {
		return
	}
	bridge := samplingFactPublicationBridge(ctx)
	for _, record := range answerDocObservationLedger(ctx).Records {
		if record.Predicate != types.RuntimeSamplingMeasurementPredicate {
			continue
		}
		expected, verified := bridge[traceEventPublicationRecordKey(record)]
		receipt.observe(record, verified && p.complete(expected))
	}
}

// Reuse the pure ledger compiler to preserve its exact source enrichment and
// tool/supplement positions. A copied publication JSON cannot create the raw
// native witness, and no source field is erased to make a match succeed.
func samplingFactPublicationBridge(ctx *types.AgentContext) map[string]types.RuntimeMeasurementPublication {
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	publications := map[string]types.RuntimeMeasurementPublication{}
	originals := map[string]string{}
	ambiguous := map[string]bool{}
	replay := func(results []types.ToolResult) []types.ToolResult {
		out := make([]types.ToolResult, len(results))
		for i, result := range results {
			if !result.Success || result.ToolName != "trace_query" {
				continue
			}
			var records []types.ObservationRecord
			for _, record := range result.Observations {
				if record.Predicate != types.RuntimeSamplingMeasurementPredicate {
					continue
				}
				records = append(records, record)
				publication, valid := types.DecodeRuntimeMeasurementPublication(record)
				prior, found := publications[record.ID]
				key := traceEventPublicationRecordKey(record)
				if !valid || key == "" || found && (!reflect.DeepEqual(prior, publication) || originals[record.ID] != key) {
					ambiguous[record.ID] = true
					continue
				}
				publications[record.ID], originals[record.ID] = publication, key
			}
			if len(records) > 0 {
				out[i] = types.ToolResult{ToolName: result.ToolName, Success: true, Timestamp: result.Timestamp, RawRef: result.RawRef, Observations: records}
			}
		}
		return out
	}
	results, supplement := replay(input.ToolResults), replay(input.SystemTraceSupplementResults)
	bridge := map[string]types.RuntimeMeasurementPublication{}
	if len(publications) == 0 {
		return bridge
	}
	compiled := types.CompileObservationLedger(types.ObservationLedgerInput{
		RepoRoot: input.RepoRoot, RuntimeArtifactPreflight: input.RuntimeArtifactPreflight,
		RequestModel: input.RequestModel, ToolResults: results, SystemTraceSupplementResults: supplement,
	})
	for _, record := range compiled.Records {
		publication, found := publications[record.ID]
		if found && !ambiguous[record.ID] && record.Predicate == types.RuntimeSamplingMeasurementPredicate {
			bridge[traceEventPublicationRecordKey(record)] = publication
		}
	}
	return bridge
}
