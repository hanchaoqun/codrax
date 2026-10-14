package tracequery

import "fmt"

// PerfCohortScope binds a legacy aggregate to the same native sampling domain
// as a prefix tree. ID identifies the full query-window cohort, not a function
// or invocation. A timeline cohort uses the same ID with its bucket quantities.
// Explicit role projections leave ID empty: their narrower predicate does not
// own the complete tree denominator even when source/thread/window match.
type PerfCohortScope struct {
	ID               string              `json:"id"`
	SourcePath       string              `json:"source_path"`
	SourceGeneration string              `json:"source_generation"`
	Producer         string              `json:"producer"`
	SampleKind       string              `json:"sample_kind"`
	Clock            string              `json:"clock"`
	ThreadIdentity   *PerfThreadIdentity `json:"thread_identity,omitempty"`
	Window           ResourceStackWindow `json:"window"`
}

// Comparable raw metadata is the aggregation key. No per-sample JSON/hash or
// copied identity aliases are retained; IDs are minted once per cohort.
type perfPopulationScopeKey struct {
	sourcePath, generation, producer, sampleKind, clock string
	thread                                              perfThreadKey
	identityKnown                                       bool
	unresolvedSourceLine                                int
}

type perfSamplePopulation struct {
	idx                      *Index
	q                        Query
	ledger                   *perfIdentityLedger
	selector                 threadSelector
	identityAddressed        bool
	selectorWithheld         bool
	selectorValid            bool
	generations              map[string]string
	identitySources          map[string]bool
	identitySourcesRead      bool
	unassignedOtherSource    bool
	unassignedSelectedSource bool
}

func newPerfSamplePopulation(idx *Index, q Query) *perfSamplePopulation {
	selector, addressed := perfTimelineThreadSelector(q)
	p := &perfSamplePopulation{idx: idx, q: q, ledger: ensurePerfIdentityLedger(idx), selector: selector, identityAddressed: addressed, generations: map[string]string{}}
	p.selectorValid = true
	if perfSamplingSelectionView(q.View) {
		_, _, p.selectorValid = NormalizePerfSamplingSelector(q.PID, perfSamplingSelectorInput(q))
	}
	return p
}

// admit is shared by flat statistics and the exact tree. Explicit role filters
// own their target; only the ordinary query lane applies the caller's selector.
// Identity-withheld rows never become a selected target's zero denominator.
func (p *perfSamplePopulation) admit(ordinal int, filter perfSampleFilter) (perfThreadKey, PerfThreadIdentity, bool, bool) {
	ev := p.idx.Events[ordinal]
	if ev.Type != EventPerfSample || !eventLineInWindow(ev, p.q) || !timeInWindow(ev.Ts, p.q) {
		return perfThreadKey{}, PerfThreadIdentity{}, false, false
	}
	key, identity, known := p.ledger.identityForEventOrdinalBorrowed(ordinal)
	if filter != nil {
		return key, identity, known, filter(ev, key, identity, known)
	}
	if !p.selectorValid {
		return key, identity, known, false
	}
	if !perfSampleQueryBase(ev, p.q) {
		return key, identity, known, false
	}
	if p.identityAddressed && (p.idx.threadIncarnationFailuresCapped || p.ledger.selectorVerdictForEventOrdinal(ordinal, p.selector) == perfIdentitySelectorWithheld) {
		p.selectorWithheld = true
		return key, identity, known, false
	}
	p.observeUnassignedIdentity(ordinal, known)
	return key, identity, known, perfThreadIdentityMatchesQuery(p.ledger, key, identity, known, p.q)
}

// observeUnassignedIdentity checks population completeness, not row identity.
// A converter-owned anonymous row cannot prove either membership or absence.
// Restrict this uncertainty to physical sources that actually prove the chosen
// selector; if none do, a nonempty anonymous population is not measured zero.
// Full-capture bindings locate those sources even when the selected window is
// empty. Header/comm/perf_source_* coordinates never enter this decision.
func (p *perfSamplePopulation) observeUnassignedIdentity(ordinal int, known bool) {
	if !p.identityAddressed || !p.selectorValid || known {
		return
	}
	ev := p.idx.Events[ordinal]
	if _, candidate := perfIdentityCandidateTIDFromEvent(ev); candidate {
		// A proved different numeric candidate (including a locally withheld
		// one) is handled by the ledger's precise selector verdict, not by
		// anonymous source coverage.
		return
	}
	if !p.identitySourcesRead {
		p.identitySourcesRead = true
		p.identitySources = map[string]bool{}
		for _, binding := range p.ledger.bindings {
			if p.q.runCancel.tick() {
				return
			}
			record := p.ledger.records[binding.record]
			if !perfThreadIdentityMatchesQuery(p.ledger, record.key, record.identity, true, p.q) {
				continue
			}
			if source, found := samplingSourceForEvent(p.idx, p.idx.Events[binding.ordinal]); found {
				p.identitySources[source.SourcePath] = true
			}
		}
	}
	source, found := samplingSourceForEvent(p.idx, ev)
	if len(p.identitySources) == 0 || !found || p.identitySources[source.SourcePath] {
		p.selectorWithheld = true
		p.unassignedSelectedSource = true
		return
	}
	p.unassignedOtherSource = true
}

func (p *perfSamplePopulation) identityCoverageCaveats() []string {
	var caveats []string
	if p.unassignedSelectedSource {
		caveats = append(caveats, "sampling_identity_coverage=incomplete; selected-window samples have unassigned thread identity in a relevant physical source; the target population is unavailable, not measured zero; source-wide sample inventory remains queryable")
	}
	if p.unassignedOtherSource {
		caveats = append(caveats, "sampling_identity_scope=proven_physical_sources; anonymous samples in other physical sources remain source-wide inventory, not selected-thread membership or proof of complete target coverage across the capture")
	}
	return caveats
}

func (p *perfSamplePopulation) cohortKey(ev Event, thread perfThreadKey, known bool) perfCohortKey {
	key := perfCohortKeyForSample(ev)
	if pf := ev.PerfFields; pf != nil {
		key.scope.producer, key.scope.sampleKind, key.scope.clock = pf.Source, pf.SampleKind, pf.Clock
	}
	key.scope.thread, key.scope.identityKnown = thread, known
	source, found := samplingSourceForEvent(p.idx, ev)
	if !found {
		// Keep unlocated rows independent; missing provenance is not evidence
		// that two rows have a common physical source.
		key.scope.unresolvedSourceLine = ev.Line
		return key
	}
	key.scope.sourcePath = source.SourcePath
	generation, cached := p.generations[source.SourcePath]
	if !cached {
		generation = perfSamplingSourceGeneration(source)
		p.generations[source.SourcePath] = generation
	}
	key.scope.generation = generation
	return key
}

func (p *perfSamplePopulation) scope(key perfCohortKey, identity PerfThreadIdentity) *PerfCohortScope {
	k := key.scope
	thread := "unknown"
	var witness *PerfThreadIdentity
	if k.identityKnown {
		thread = fmt.Sprintf("%s/%d/%d", k.thread.scope, identity.TID, identity.Generation)
		copy := identity
		copy.CommAliases = append([]string(nil), identity.CommAliases...)
		witness = &copy
	}
	if k.unresolvedSourceLine != 0 {
		thread += fmt.Sprintf("/unlocated-line:%d", k.unresolvedSourceLine)
	}
	id := samplingID(k.sourcePath, k.generation, k.producer, key.event, key.weightUnit, k.sampleKind, k.clock, thread, fmt.Sprintf("%.17g/%.17g/%t/%d/%s", p.q.TimeStart, p.q.TimeEnd, p.q.timeEndBackfilled, p.q.PID, p.q.Thread))
	if p.q.LineStart != 0 || p.q.LineEnd != 0 || p.q.Pattern != "" || len(p.q.Patterns) > 0 || len(p.q.EventTypes) > 0 || len(p.q.EventNames) > 0 || len(p.q.EventFieldFilters) > 0 || len(p.q.TraceMarkActions) > 0 {
		// A filtered flat population must not share a selector ID with the
		// complete same-window tree. Do not silently remove these filters.
		id = samplingID(id, fmt.Sprintf("%d/%d/%q/%q/%q/%q/%v/%q", p.q.LineStart, p.q.LineEnd, p.q.Pattern, p.q.Patterns, p.q.EventTypes, p.q.EventNames, p.q.EventFieldFilters, p.q.TraceMarkActions))
	}
	return &PerfCohortScope{ID: id, SourcePath: k.sourcePath, SourceGeneration: k.generation, Producer: k.producer, SampleKind: k.sampleKind, Clock: k.clock, ThreadIdentity: witness, Window: ResourceStackWindow{StartTs: p.q.TimeStart, EndTs: p.q.TimeEnd, EndInclusive: p.q.timeEndBackfilled}}
}

func perfSampleQueryBase(ev Event, q Query) bool {
	if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || len(q.TraceMarkActions) > 0 {
		return false
	}
	// Native sampling windows are half-open unless normalization backfilled
	// the complete capture's final timestamp. This is the tree/finalizer scope
	// contract; leave historical non-perf timeInWindow consumers untouched.
	if (q.TimeStart > 0 || q.TimeStartSet || q.timeStartBackfilled) && ev.Ts < q.TimeStart {
		return false
	}
	if (q.TimeEnd > 0 || q.TimeEndSet || q.timeEndBackfilled) && (ev.Ts > q.TimeEnd || !q.timeEndBackfilled && ev.Ts == q.TimeEnd) {
		return false
	}
	if len(q.EventTypes) > 0 {
		found := false
		for _, typ := range q.EventTypes {
			if typ == EventPerfSample {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return eventMatchesNames(ev, q.EventNames) && (!eventSearchHasLiteralPatterns(q) || eventMatchesQueryPatterns(ev, q)) && eventMatchesFieldFilters(ev, q.EventFieldFilters)
}

func perfCohortKeyLess(a, b perfCohortKey) bool {
	if a.event != b.event {
		return a.event < b.event
	}
	if a.weightUnit != b.weightUnit {
		return a.weightUnit < b.weightUnit
	}
	x, y := a.scope, b.scope
	if x.sourcePath != y.sourcePath {
		return x.sourcePath < y.sourcePath
	}
	if x.generation != y.generation {
		return x.generation < y.generation
	}
	if x.producer != y.producer {
		return x.producer < y.producer
	}
	if x.sampleKind != y.sampleKind {
		return x.sampleKind < y.sampleKind
	}
	if x.clock != y.clock {
		return x.clock < y.clock
	}
	if x.identityKnown != y.identityKnown {
		return !x.identityKnown
	}
	if x.thread.scope != y.thread.scope {
		return x.thread.scope < y.thread.scope
	}
	if x.thread.TID != y.thread.TID {
		return x.thread.TID < y.thread.TID
	}
	if x.thread.Generation != y.thread.Generation {
		return x.thread.Generation < y.thread.Generation
	}
	return x.unresolvedSourceLine < y.unresolvedSourceLine
}

func perfLegacyViewCarriesSampling(view string) bool {
	switch view {
	case "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline":
		return true
	}
	return false
}

func indexHasPerfSamples(idx *Index, q Query) bool {
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return false
		}
		if ev.Type == EventPerfSample {
			return true
		}
	}
	return false
}

func perfCohortScopeSummary(scope *PerfCohortScope) string {
	if scope == nil {
		return ""
	}
	return fmt.Sprintf(" cohort_id=%q source_path=%q source_generation=%q producer=%q sample_kind=%q clock=%q window=%.9f..%.9f end_inclusive=%t", scope.ID, scope.SourcePath, scope.SourceGeneration, scope.Producer, scope.SampleKind, scope.Clock, scope.Window.StartTs, scope.Window.EndTs, scope.Window.EndInclusive)
}

func perfHotspotEvidenceSummary(cohort PerfCohort, hot PerfHotspot, callchain bool) string {
	producer := hot.Source
	if cohort.Scope != nil {
		producer = cohort.Scope.Producer
	}
	// The compact observation face preserves only a short prefix. Put the
	// independent denominator/producer domain before potentially long labels.
	caliber := "value_basis=" + PerfRecordedFieldsValueBasis
	if callchain {
		caliber = "callchain_form=opaque_flat_label"
		if hot.CallchainIdentity != nil {
			caliber = "callchain_form=" + hot.CallchainIdentity.Form
		}
	}
	summary := fmt.Sprintf("perf sample_weight=%d denominator=%d %s producer=%s event=%s weight_unit=%s samples=%d percent=%.2f%%", hot.Period, cohort.TotalPeriod, caliber, firstNonEmpty(producer, "unknown"), cohort.Event, cohort.WeightUnit, hot.SampleCount, hot.Percent)
	if callchain {
		reason, id := "frame_identity_unavailable", ""
		if hot.CallchainIdentity != nil {
			reason, id = hot.CallchainIdentity.Reason, hot.CallchainIdentity.ID
		}
		summary += " frame_reason=" + reason + " path_id=" + id + " raw_callchain=" + firstNonEmpty(hot.Callchain, "unknown")
	} else {
		summary += " symbol=" + firstNonEmpty(hot.Symbol, "unknown") + " dso=" + firstNonEmpty(hot.DSO, "unknown")
	}
	return summary + perfCohortScopeSummary(cohort.Scope)
}
