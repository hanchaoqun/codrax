package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// This is an encoded native-core budget, not a Go heap limit. It is shared by
// all explorer branches in one source epoch; visible handles remain local to
// each branch until an explicit merge. Session history keeps its own budgets.
const (
	nativeFactRetentionQueryLimit = 128
	nativeFactRetentionByteLimit  = 16 << 20
)

// NativeFactRetentionStatus describes retention, never query execution or
// completeness. A bounded last-omission receipt is deliberately not a roster
// or a count of unique omitted queries.
type NativeFactRetentionStatus struct {
	RetainedQueries  int    `json:"retained_queries"`
	EncodedBytes     int    `json:"encoded_bytes"`
	QueryLimit       int    `json:"query_limit"`
	EncodedByteLimit int    `json:"encoded_byte_limit"`
	CapacityOmitted  bool   `json:"capacity_omitted"`
	LastSource       string `json:"last_source,omitempty"`
	LastQuery        string `json:"last_query,omitempty"`
}

type nativeFactRetentionPool struct {
	mu      sync.Mutex
	entries map[string]*retainedNativeFact
	queries int
	bytes   int
}

// Only this package creates a frozen native core. The JSON string is not a
// persistence/import format: unexported producer witnesses travel separately
// and can neither be decoded from JSON nor signed by a consumer.
type retainedNativeFact struct {
	key        string
	encoded    string
	queries    int
	witness    nativeFactWitness
	generation *traceSourceReadGeneration
}

type nativeFactWire struct {
	Result                 ToolResult
	IOEndpoints            []*TraceIORequestEndpoints
	IOTargets              []traceIOProjectionTarget
	BusinessSpans          []TraceBusinessSpanCandidate
	BusinessRefData        []TraceBusinessSpanCandidate
	BusinessSources        []nativeFactSourceVersion
	PairReport             string
	Source                 nativeFactSourceVersion
	Replay                 nativeFactReplayVersion
	Statistics             nativeFactReplayVersion
	StatisticsAvailability string
	StatisticsSource       string
	PairReplay             [2]nativeFactReplayVersion
	PairMaterial           [2]string
}

type nativeFactSourceVersion struct{ Path, PhysicalSource, Version string }
type nativeFactNavigationVersion struct {
	QueryPath, Views, Params, Material string
	Source                             nativeFactSourceVersion
}
type nativeFactReplayVersion struct {
	Source           nativeFactSourceVersion
	Params           string
	Native, Prepared *nativeFactNavigationVersion
}

func nativeFactSourceVersionOf(source TraceQuerySourceReadRef) nativeFactSourceVersion {
	return nativeFactSourceVersion{source.path, source.physicalSource, source.identity.CacheToken()}
}

func nativeFactNavigationVersionOf(nav *traceIntervalNavigation) *nativeFactNavigationVersion {
	if nav == nil {
		return nil
	}
	return &nativeFactNavigationVersion{QueryPath: nav.queryPath, Views: nav.views, Params: nav.params,
		Material: nav.material.GenerationCacheKey(), Source: nativeFactSourceVersionOf(nav.source)}
}

func nativeFactReplayVersionOf(replay TraceQueryWindowReplayRef) nativeFactReplayVersion {
	return nativeFactReplayVersion{Source: nativeFactSourceVersionOf(replay.source), Params: replay.params,
		Native: nativeFactNavigationVersionOf(replay.nativeNavigation), Prepared: nativeFactNavigationVersionOf(replay.preparedSource)}
}

type nativeFactWitness struct {
	sourceRead   TraceQuerySourceReadRef
	replay       TraceQueryWindowReplayRef
	selection    TraceEventSelectionAcquisitionRef
	statistics   TraceStatisticsRef
	pair         RuntimeMeasurementPairRef
	businessRefs []TraceBusinessSpanRef
	rows         []nativeFactRowWitness
}

type nativeFactRowWitness struct {
	raw         *runtimeRawMeasurementWitness
	measurement *runtimeMeasurementFactWitness
	aggregate   AnswerAggregateKind
	sampling    *runtimeSamplingPublicationWitness
	resource    *runtimeResourcePublicationWitness
	ioIdentity  *traceIORequestIdentity
	ioTarget    traceIOProjectionTarget
}

func nativeFactWireValue(result ToolResult) nativeFactWire {
	// Do not retain the tool's explanatory summary, generic handoff, repair
	// advice or model-owned investigation closure. These are not native facts.
	wire := nativeFactWire{Result: ToolResult{
		ToolName: result.ToolName, RawRef: result.RawRef, Observations: result.Observations,
		TraceEvidenceAuthority: result.TraceEvidenceAuthority, EnumerationAuthority: result.EnumerationAuthority,
		Success: result.Success, Timestamp: result.Timestamp,
	}, BusinessSpans: result.TraceBusinessSpanCandidates, PairReport: result.RuntimeMeasurementPair.report}
	wire.Source, wire.Replay = nativeFactSourceVersionOf(result.TraceQuerySourceRead), nativeFactReplayVersionOf(result.TraceQueryWindowReplay)
	wire.Statistics = nativeFactReplayVersionOf(result.TraceStatistics.replay)
	wire.StatisticsAvailability, wire.StatisticsSource = result.TraceStatistics.availability, result.TraceStatistics.physicalSource
	for i := range result.RuntimeMeasurementPair.replay {
		wire.PairReplay[i] = nativeFactReplayVersionOf(result.RuntimeMeasurementPair.replay[i])
		wire.PairMaterial[i] = result.RuntimeMeasurementPair.material[i].GenerationCacheKey()
	}
	for _, ref := range result.TraceBusinessSpanRefs {
		wire.BusinessRefData = append(wire.BusinessRefData, ref.data)
		wire.BusinessSources = append(wire.BusinessSources, nativeFactSourceVersionOf(ref.source))
	}
	for _, row := range result.Observations {
		wire.IOEndpoints = append(wire.IOEndpoints, row.IORequestEndpoints)
		target := traceIOProjectionTarget{}
		if row.IORequestEndpoints != nil {
			target = row.IORequestEndpoints.projectionTarget
		}
		wire.IOTargets = append(wire.IOTargets, target)
	}
	return wire
}

func nativeFactCoreDigest(result ToolResult) ([32]byte, bool) {
	data, err := json.Marshal(nativeFactWireValue(result))
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(data), true
}

func nativeFactCurrentPublication(m *MutableState, result ToolResult) (*runtimeCurrentQueryCensus, bool) {
	census := result.TraceQueryWindowReplay.currentRows
	if m == nil || census == nil || census.generation == nil || census.generation.nativeFactsRetired.Load() ||
		!result.Success || CanonicalToolName(result.ToolName) != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil || len(census.populations) == 0 {
		return nil, false
	}
	m.mu.RLock()
	current := m.traceSourceReadGeneration == census.generation
	m.mu.RUnlock()
	digest, ok := nativeFactCoreDigest(result)
	if !current || !ok || digest != census.nativeCoreDigest {
		return nil, false
	}
	input := ObservationLedgerInput{runtimeMeasurementPairConsumer: m}
	checked := make(map[runtimeDisplayQueryKey]bool)
	for _, population := range census.populations {
		if population.ambiguous || len(population.rows) == 0 {
			return nil, false
		}
	}
	for _, row := range result.Observations {
		key := runtimeDisplayKey(row.SourceRef)
		population, exists := census.populations[key]
		if key == "" || !exists {
			return nil, false
		}
		if !checked[key] {
			if runtimeMeasurementOriginalSource(input, population.result, row.SourceRef) == "" {
				return nil, false
			}
			checked[key] = true
		}
	}
	return census, true
}

func freezeNativeFact(result ToolResult, census *runtimeCurrentQueryCensus) *retainedNativeFact {
	wire := nativeFactWireValue(result)
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil
	}
	// Identity excludes only observation capture time. The complete rows,
	// sources, query/payload identities, windows and native authority remain.
	// A repeated physical query does not consume another retention slot.
	identity := wire
	identity.Result.Timestamp = time.Time{}
	identity.Result.Observations = append([]ObservationRecord(nil), wire.Result.Observations...)
	for i := range identity.Result.Observations {
		identity.Result.Observations[i].ObservedAt = ""
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		return nil
	}
	digest := sha256.Sum256(identityBytes)
	witness := nativeFactWitness{
		sourceRead: result.TraceQuerySourceRead, replay: result.TraceQueryWindowReplay,
		selection: result.TraceEventSelectionAcquisition, statistics: result.TraceStatistics,
		pair:         result.RuntimeMeasurementPair,
		businessRefs: append([]TraceBusinessSpanRef(nil), result.TraceBusinessSpanRefs...),
	}
	for _, row := range result.Observations {
		private := nativeFactRowWitness{
			raw: row.runtimeRawMeasurementPublication, measurement: row.runtimeMeasurementFactIdentity,
			aggregate: row.aggregateKind, sampling: row.runtimeSamplingPublication,
			resource: row.runtimeResourcePublication, ioIdentity: row.ioRequestIdentity,
		}
		if row.IORequestEndpoints != nil {
			private.ioTarget = row.IORequestEndpoints.projectionTarget
		}
		witness.rows = append(witness.rows, private)
	}
	return &retainedNativeFact{key: hex.EncodeToString(digest[:]), encoded: string(encoded),
		queries: len(census.populations), witness: witness, generation: census.generation}
}

func (f *retainedNativeFact) result() (ToolResult, bool) {
	var wire nativeFactWire
	if f == nil || json.Unmarshal([]byte(f.encoded), &wire) != nil || len(wire.Result.Observations) != len(f.witness.rows) || len(wire.IOEndpoints) != len(f.witness.rows) {
		return ToolResult{}, false
	}
	r := wire.Result
	r.TraceQuerySourceRead, r.TraceQueryWindowReplay = f.witness.sourceRead, f.witness.replay
	r.TraceEventSelectionAcquisition, r.TraceStatistics = f.witness.selection, f.witness.statistics
	r.RuntimeMeasurementPair = f.witness.pair
	r.TraceBusinessSpanCandidates = wire.BusinessSpans
	r.TraceBusinessSpanRefs = append([]TraceBusinessSpanRef(nil), f.witness.businessRefs...)
	for i := range r.Observations {
		row, private := &r.Observations[i], f.witness.rows[i]
		row.runtimeRawMeasurementPublication, row.runtimeMeasurementFactIdentity = private.raw, private.measurement
		row.aggregateKind, row.runtimeSamplingPublication, row.runtimeResourcePublication = private.aggregate, private.sampling, private.resource
		row.ioRequestIdentity, row.IORequestEndpoints = private.ioIdentity, wire.IOEndpoints[i]
		if row.IORequestEndpoints != nil {
			row.IORequestEndpoints.projectionTarget = private.ioTarget
		}
	}
	return r, true
}

// Called only by AppendDispatchToolResult, while no MutableState lock is held.
func (m *MutableState) retainNativeFact(result ToolResult, replacement *turnAReplacementToken) {
	census, valid := nativeFactCurrentPublication(m, result)
	if !valid {
		return
	}
	frozen := freezeNativeFact(result, census)
	if frozen == nil {
		return
	}
	m.completeNativeFactAdmission(census.generation, replacement, frozen,
		result.Observations[0].SourceRef.Path, result.Observations[0].SourceRef.QueryScopeID)
}

// Finish an already validated admission. The collection token is captured
// before validation/encoding. Check it before touching shared capacity, then
// publish capacity and visibility in one critical section. Lock order is
// m.mu -> pool.mu; pool.admit has no callbacks and never acquires m.mu.
func (m *MutableState) completeNativeFactAdmission(generation *traceSourceReadGeneration, replacement *turnAReplacementToken, frozen *retainedNativeFact, source, query string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation == nil || m.turnAReplacementToken != replacement || m.traceSourceReadGeneration != generation || generation.nativeFactsRetired.Load() {
		return
	}
	if frozen == nil || frozen.generation != generation {
		return
	}
	stored, omitted := generation.nativeFacts.admit(frozen)
	if omitted {
		m.nativeFactRetentionOmission = NativeFactRetentionStatus{CapacityOmitted: true,
			LastSource: source, LastQuery: query}
		return
	}
	if stored == nil {
		return
	}
	if m.nativeFactRetentionHandles == nil {
		m.nativeFactRetentionHandles = make(map[string]*retainedNativeFact)
	}
	if _, exists := m.nativeFactRetentionHandles[stored.key]; !exists {
		m.nativeFactRetentionHandles[stored.key] = stored
		m.nativeFactRetentionOrder = append(m.nativeFactRetentionOrder, stored.key)
	}
}

// Replacement only intersects existing registrations. A complete private
// producer census and its exact core digest are necessary, but never create a
// new retention admission here. Current physical-source checks still run when
// the retained publication is consumed.
func nativeFactReplacementKeys(results []ToolResult) map[string]*traceSourceReadGeneration {
	keys := make(map[string]*traceSourceReadGeneration)
	for _, result := range results {
		census := result.TraceQueryWindowReplay.currentRows
		if census == nil || census.generation == nil || census.generation.nativeFactsRetired.Load() ||
			!result.Success || CanonicalToolName(result.ToolName) != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil {
			continue
		}
		digest, ok := nativeFactCoreDigest(result)
		if !ok || digest != census.nativeCoreDigest {
			continue
		}
		if frozen := freezeNativeFact(result, census); frozen != nil {
			keys[frozen.key] = census.generation
		}
	}
	return keys
}

func (m *MutableState) reconcileNativeFactHandlesLocked(keys map[string]*traceSourceReadGeneration) {
	kept := make(map[string]*retainedNativeFact)
	var order []string
	for _, key := range m.nativeFactRetentionOrder {
		fact := m.nativeFactRetentionHandles[key]
		if fact != nil && keys[key] == fact.generation && fact.generation == m.traceSourceReadGeneration && !fact.generation.nativeFactsRetired.Load() {
			kept[key] = fact
			order = append(order, key)
		}
	}
	m.nativeFactRetentionHandles, m.nativeFactRetentionOrder = kept, order
	// An old omission is not an omission from the new explicit collection.
	// Shared pool capacity remains unchanged and is not reclaimed by replacement.
	m.nativeFactRetentionOmission = NativeFactRetentionStatus{}
}

// Admission is all-or-nothing; no row or table is clipped to fit the budget.
func (pool *nativeFactRetentionPool) admit(frozen *retainedNativeFact) (*retainedNativeFact, bool) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	stored := pool.entries[frozen.key]
	omitted := false
	if stored == nil {
		if pool.queries+frozen.queries > nativeFactRetentionQueryLimit || pool.bytes+len(frozen.encoded) > nativeFactRetentionByteLimit {
			omitted = true
		} else {
			if pool.entries == nil {
				pool.entries = make(map[string]*retainedNativeFact)
			}
			pool.entries[frozen.key] = frozen
			pool.queries += frozen.queries
			pool.bytes += len(frozen.encoded)
			stored = frozen
		}
	}
	return stored, omitted
}

func cloneNativeFactHandles(in map[string]*retainedNativeFact) map[string]*retainedNativeFact {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*retainedNativeFact, len(in))
	for key, fact := range in {
		out[key] = fact // frozen value, never mutated after insertion
	}
	return out
}

// Caller holds m.mu; snapshot was obtained under the fork's RLock. The merge
// is independent of historical ToolResult positions and their truncation.
func (m *MutableState) mergeNativeFactHandlesLocked(generation *traceSourceReadGeneration, handles map[string]*retainedNativeFact, order []string, omission NativeFactRetentionStatus) bool {
	if generation == nil || generation != m.traceSourceReadGeneration || generation.nativeFactsRetired.Load() {
		return false
	}
	changed := false
	for _, key := range order {
		fact := handles[key]
		if fact == nil || fact.generation != generation || m.nativeFactRetentionHandles[key] != nil {
			continue
		}
		if m.nativeFactRetentionHandles == nil {
			m.nativeFactRetentionHandles = make(map[string]*retainedNativeFact)
		}
		m.nativeFactRetentionHandles[key] = fact
		m.nativeFactRetentionOrder = append(m.nativeFactRetentionOrder, key)
		if result, ok := fact.result(); ok {
			for _, ref := range traceQueryPublishedBlobRefsFromToolResult(result) {
				m.registerTraceQueryBlobRefLocked(ref)
			}
			m.registerTraceQuerySourceReadLocked(result)
			m.registerTraceBusinessSpanRefsLocked(result)
		}
		changed = true
	}
	if omission.CapacityOmitted && m.nativeFactRetentionOmission != omission {
		m.nativeFactRetentionOmission = omission
		changed = true
	}
	return changed
}

func (m *MutableState) NativeFactRetentionStatus() NativeFactRetentionStatus {
	status := NativeFactRetentionStatus{QueryLimit: nativeFactRetentionQueryLimit, EncodedByteLimit: nativeFactRetentionByteLimit}
	if m == nil {
		return status
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.traceSourceReadGeneration == nil || m.traceSourceReadGeneration.nativeFactsRetired.Load() {
		return status
	}
	status.CapacityOmitted, status.LastSource, status.LastQuery = m.nativeFactRetentionOmission.CapacityOmitted, m.nativeFactRetentionOmission.LastSource, m.nativeFactRetentionOmission.LastQuery
	for _, fact := range m.nativeFactRetentionHandles {
		status.RetainedQueries += fact.queries
		status.EncodedBytes += len(fact.encoded)
	}
	return status
}

func (m *MutableState) observationLedgerNativeToolResults(base []ToolResult) []ToolResult {
	if m == nil {
		return base
	}
	m.mu.RLock()
	order := append([]string(nil), m.nativeFactRetentionOrder...)
	handles := cloneNativeFactHandles(m.nativeFactRetentionHandles)
	m.mu.RUnlock()
	// This repairs absent whole results, not damaged current results. A copied
	// producer census suppresses recovery even when its current rows changed:
	// the original validators must still reject that current representation.
	// Different producer incarnations remain independent; a broad query scope
	// must not hide another window or turn a conflicting row into a valid one.
	seen := make(map[*runtimeCurrentQueryCensus]bool)
	exact := make(map[string]bool)
	for _, result := range base {
		if census := result.TraceQueryWindowReplay.currentRows; census != nil && result.Success && !result.ReusedFromRunMemo && result.TraceViewCancellation == nil {
			seen[census] = true
		}
		if census, ok := nativeFactCurrentPublication(m, result); ok {
			if frozen := freezeNativeFact(result, census); frozen != nil {
				exact[frozen.key] = true
			}
		}
	}
	out := append([]ToolResult(nil), base...)
	for _, key := range order {
		result, ok := handles[key].result()
		if !ok {
			continue
		}
		census, valid := nativeFactCurrentPublication(m, result)
		if !valid {
			continue
		}
		if seen[census] || exact[key] {
			continue // indivisible native query population, never partial injection
		}
		out = append(out, result)
		seen[census], exact[key] = true, true
	}
	return out
}
