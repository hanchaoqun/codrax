package types

import (
	"crypto/sha256"
	"encoding/json"
)

// This ephemeral index proves only that a complete set of projected rows came
// from one current successful query. It does not prove that the query enumerated
// the complete physical source, nor grant completion or causal authority.
type runtimeCurrentQueryPopulation struct {
	result    ToolResult
	rows      map[string]map[[32]byte]struct{}
	ambiguous bool
}

type runtimeCurrentQueryCensus struct {
	populations map[runtimeDisplayQueryKey]runtimeCurrentQueryPopulation
}

const runtimeCurrentQueryPopulationLimit = 4096

func runtimeCurrentQueryRowDigest(record ObservationRecord) ([32]byte, bool) {
	// These are exactly the source decorations excluded by runtimeDisplayKey:
	// compiler ToolCallID/RawRef and capture aliases are not producer facts.
	var source ObservationSourceRef
	if json.Unmarshal([]byte(runtimeDisplayKey(record.SourceRef)), &source) != nil {
		return [32]byte{}, false
	}
	source.Kind = record.SourceRef.Kind
	record.SourceRef = source
	// CompileObservationLedger stamps supplement provenance and fills an absent
	// timestamp/claim authority. They do not change the native row's contents.
	record.SystemSupplement = false
	record.ObservedAt = ""
	if !record.ClaimAuthority.IsValid() {
		record.ClaimAuthority = inferObservationClaimAuthority(record)
	}
	raw, err := json.Marshal(record)
	return sha256.Sum256(raw), err == nil
}

// StampRuntimeCurrentQueryPopulation is a one-shot producer boundary after
// final source decoration. It cannot repair an existing census after public
// rows/selectors changed. JSON carries neither this seal nor source authority.
func (m *MutableState) StampRuntimeCurrentQueryPopulation(result *ToolResult) {
	if result == nil || result.TraceQueryWindowReplay.currentRows != nil {
		return
	}
	census := &runtimeCurrentQueryCensus{populations: map[runtimeDisplayQueryKey]runtimeCurrentQueryPopulation{}}
	result.TraceQueryWindowReplay.currentRows = census
	if m == nil || !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil {
		return
	}
	input := ObservationLedgerInput{runtimeMeasurementPairConsumer: m}
	for _, record := range result.Observations {
		ref := record.SourceRef
		if ref.Path == "" || ref.PayloadRef == "" || ref.QueryScopeID == "" || record.Predicate == "" {
			continue
		}
		key := runtimeDisplayKey(ref)
		population, exists := census.populations[key]
		if !exists {
			replay := result.TraceQueryWindowReplay
			replay.currentRows = nil // Never retain a self-referential result.
			population = runtimeCurrentQueryPopulation{result: ToolResult{TraceQueryWindowReplay: replay, TraceStatistics: result.TraceStatistics}, rows: map[string]map[[32]byte]struct{}{}, ambiguous: runtimeMeasurementOriginalSource(input, *result, ref) == ""}
		}
		if len(result.Observations) > runtimeCurrentQueryPopulationLimit {
			population.ambiguous = true
		} else if digest, ok := runtimeCurrentQueryRowDigest(record); ok {
			if population.rows[record.Predicate] == nil {
				population.rows[record.Predicate] = map[[32]byte]struct{}{}
			}
			population.rows[record.Predicate][digest] = struct{}{}
		} else {
			population.ambiguous = true
		}
		census.populations[key] = population
	}
}

func buildRuntimeCurrentQueryPopulations(input ObservationLedgerInput) map[runtimeDisplayQueryKey]runtimeCurrentQueryPopulation {
	out := map[runtimeDisplayQueryKey]runtimeCurrentQueryPopulation{}
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			census := result.TraceQueryWindowReplay.currentRows
			if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil || census == nil {
				continue
			}
			for key, population := range census.populations {
				if previous, exists := out[key]; exists {
					if !sameRuntimeCurrentQueryRows(previous.rows, population.rows) {
						population.ambiguous = true
					}
					population.ambiguous = population.ambiguous || previous.ambiguous
				}
				out[key] = population
			}
		}
	}
	return out
}

func sameRuntimeCurrentQueryRows(a, b map[string]map[[32]byte]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for predicate, rows := range a {
		other := b[predicate]
		if len(rows) != len(other) {
			return false
		}
		for digest := range rows {
			if _, found := other[digest]; !found {
				return false
			}
		}
	}
	return true
}

// CurrentRuntimeQueryPopulation checks a whole producer-projected predicate
// population before any target/display-budget cropping. A copied live ledger
// keeps this private index, but changed rows, JSON, stale sources and another
// run cannot manufacture it. Call once per scope; source validation is shared
// with the existing prepared/native replay path and does not replay the query.
func (ledger ObservationLedger) CurrentRuntimeQueryPopulation(source ObservationSourceRef, predicate string, records []ObservationRecord) bool {
	index := ledger.runtimeDisplayScopes
	if index == nil || len(records) == 0 {
		return false
	}
	key := runtimeDisplayKey(source)
	population, found := index.currentRows[key]
	if !found || population.ambiguous || runtimeMeasurementOriginalSource(index.input, population.result, source) == "" {
		return false
	}
	expected := population.rows[predicate]
	if len(expected) == 0 {
		return false
	}
	seen := map[[32]byte]struct{}{}
	for _, record := range records {
		if record.Predicate != predicate || runtimeDisplayKey(record.SourceRef) != key {
			return false
		}
		digest, ok := runtimeCurrentQueryRowDigest(record)
		if !ok {
			return false
		}
		if _, found := expected[digest]; !found {
			return false
		}
		seen[digest] = struct{}{}
	}
	return len(seen) == len(expected)
}
