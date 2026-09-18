package tracequery

import "fmt"

// publishIndexedEventSearch keeps the search rows and their accounting in one
// publication path for both a direct view and a composed recipe step. A
// composition must not silently drop match totals, truncation, identity
// caveats, or cancellation boundaries while retaining the displayed rows.
func publishIndexedEventSearch(res *Result, idx *Index, q Query, explicitTimeStart, explicitTimeEnd bool, faceCanceled func(string) bool) {
	searchEvents, perfIdentityCaveat := eventSearchIndexed(idx, q)
	if faceCanceled("event_search") {
		return
	}
	res.Events = searchEvents
	if perfIdentityCaveat != "" {
		res.Caveats = append(res.Caveats, perfIdentityCaveat)
	}
	res.EvidencePack = evidenceFromEvents(res.Events)
	// Keep the complete bounded display face if a later exhaustive census is
	// canceled. Test the discard gate before testing a nil census, so cancellation
	// cannot masquerade as proof that there was no census to publish.
	if census := ComputeCPUFrequencyCensus(idx, q, res.Events); !faceCanceled("cpu_frequency_census") && census != nil {
		res.CPUFrequencyCensus = census
		res.EvidencePack = append([]EvidenceFact{census.EvidenceFact()}, res.EvidencePack...)
	}
	if census := ComputeVsyncGeneratorSearchCensus(idx, q); !faceCanceled("vsync_generator_census") && census != nil {
		res.VsyncGeneratorCensus = census
	}
	if q.runCancel.fired() {
		return
	}
	matchedEvents, matchedTimeStart, matchedTimeEnd, invalidJankFields := eventSearchMatchAccounting(idx, q)
	if faceCanceled("event_search_accounting") {
		return
	}
	if invalidJankFields > 0 {
		res.Caveats = append(res.Caveats, jankEventIntegrityCaveat(invalidJankFields))
	}
	scopeKind, scopeTimeStart, scopeTimeEnd := eventSearchScopeAccounting(idx, q, explicitTimeStart, explicitTimeEnd)
	res.EventSearchCoverage = &EventSearchCoverage{
		ScopeKind: scopeKind, ScopeTimeStart: scopeTimeStart, ScopeTimeEnd: scopeTimeEnd,
		ScopeComplete: true, MatchedTimeStart: matchedTimeStart, MatchedTimeEnd: matchedTimeEnd,
		MatchedTotal: matchedEvents, Emitted: len(searchEvents), EnumerationComplete: true,
	}
	if matchedEvents > len(res.Events) {
		last := res.Events[len(res.Events)-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View: FallbackViewEventSearch, Dimension: CompactionDimensionEvents,
			Total: matchedEvents, Emitted: len(res.Events),
			LastEmittedTs: last.Ts, LastEmittedLine: last.Line,
		})
		res.Caveats = append(res.Caveats,
			fmt.Sprintf("event_search_index_compacted=true; matched %d row(s) but returned the first %d chronological match(es) only; omitted rows may contain later trace-mark actions, so do not infer absence without narrowing the query", matchedEvents, len(res.Events)))
	}
}
