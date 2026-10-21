package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Preview reuse is not query, population, or completion identity. Require a
// current private physical-source receipt, the complete query coordinate, and
// the entire native publication. Only the result-envelope coordinate differs.
// Keeping sibling tables also prevents a matching preview from hiding a
// changed member population, ruler, omission, or unit elsewhere in the result.
func runtimeMeasurementPreviewPublicationKey(source, predicate string, p types.RuntimeMeasurementPublication) string {
	if source == "" || !types.RuntimeMeasurementPredicateIsRegistered(predicate) {
		return ""
	}
	envelope := filepath.Base(p.Source.PayloadRef)
	query, queryOK := strings.CutPrefix(p.Source.QueryScopeID, envelope+":")
	observation, observationOK := strings.CutPrefix(p.ObservationID, "trace_query:"+envelope)
	if !queryOK || !observationOK || query == "" || observation == "" {
		return ""
	}
	copy := p
	copy.Source.RawRef, copy.Source.PayloadRef = "", ""
	copy.Source.QueryScopeID, copy.ObservationID = query, observation
	copy.Tables = make([]types.RuntimeMeasurementTable, len(p.Tables))
	for i, table := range p.Tables {
		if table.ObservationID != p.ObservationID || !table.IsValid() {
			return ""
		}
		copy.Tables[i] = table.Clone()
		copy.Tables[i].ObservationID = observation
		if population := copy.Tables[i].DisplayPopulation; population != nil {
			if population.QueryScopeID != p.Source.QueryScopeID {
				return ""
			}
			population.QueryScopeID = query
		}
	}
	raw, err := json.Marshal([]any{source, predicate, copy})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return "native-preview:" + hex.EncodeToString(digest[:])
}

func runtimeMeasurementPreviewIdentities(ctx *types.AgentContext, tables []types.RuntimeMeasurementTable, receipt *answerDocNativePresentationReceipt) []string {
	keys := make([]string, len(tables))
	wanted := map[string]bool{}
	for _, table := range tables {
		if !table.HasNativeSamplingFacts() && table.EventData == nil {
			wanted[table.ObservationID] = true
		}
	}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	type witness struct{ key, table string }
	witnesses := map[samplingFactViewKey]witness{}
	conflicts := map[samplingFactViewKey]bool{}
	for _, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			for _, record := range result.Observations {
				if !wanted[record.ID] {
					continue
				}
				p, valid := types.DecodeRuntimeMeasurementPublication(record)
				if !valid {
					continue
				}
				source := types.RuntimeObservationPresentationSourceKey(input, result, record)
				key := runtimeMeasurementPreviewPublicationKey(source, record.Predicate, p)
				for _, table := range p.Tables {
					selector := samplingFactViewKey{table.ObservationID, table.View}
					raw, _ := json.Marshal(table)
					candidate := witness{key, string(raw)}
					if prior, found := witnesses[selector]; found && prior != candidate {
						conflicts[selector] = true
					}
					witnesses[selector] = candidate
				}
			}
		}
	}
	for i, table := range tables {
		if table.HasNativeSamplingFacts() {
			// Retain sampling's established native thread/cohort equivalence.
			keys[i] = receipt.samplingTableKey(table)
			continue
		}
		if table.EventData != nil {
			continue // Its inventory already owns shared rows and notes.
		}
		selector := samplingFactViewKey{table.ObservationID, table.View}
		w := witnesses[selector]
		raw, _ := json.Marshal(table)
		// The fresh contract may add request-local scoping or missing-window
		// notes. Such a projection must not alias the unmodified raw table.
		if !conflicts[selector] && w.key != "" && w.table == string(raw) {
			keys[i] = w.key + "\x00" + string(table.View)
		}
	}
	return keys
}

func runtimeMeasurementDistinctPreviewRows(tables []types.RuntimeMeasurementTable, keys []string) []int {
	copy := append([]types.RuntimeMeasurementTable(nil), tables...)
	seen := map[string]bool{}
	for i, key := range keys {
		if key == "" {
			continue
		}
		if seen[key] {
			copy[i].Rows = nil
		}
		seen[key] = true
	}
	return runtimeMeasurementHandoffPreviewRows(copy)
}

type runtimeMeasurementPublishedPreview struct {
	table    types.RuntimeMeasurementTable
	rows     int
	complete bool
}
