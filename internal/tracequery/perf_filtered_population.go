package tracequery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// PerfPopulationFilters records the actual native row predicate, not a tree
// display option. These filters define a different denominator from an
// unfiltered time/thread query. They never imply invocation or causal scope.
type PerfPopulationFilters struct {
	Pattern           string             `json:"pattern,omitempty"`
	Patterns          []string           `json:"patterns,omitempty"`
	EventTypes        []EventType        `json:"event_types,omitempty"`
	EventNames        []string           `json:"event_names,omitempty"`
	EventFieldFilters []EventFieldFilter `json:"event_field_filters,omitempty"`
	TraceMarkActions  []TraceMarkAction  `json:"trace_mark_actions,omitempty"`
	LineStart         int                `json:"line_start,omitempty"`
	LineEnd           int                `json:"line_end,omitempty"`
}

func PerfPopulationQueryFilters(q Query) PerfPopulationFilters {
	return PerfPopulationFilters{q.Pattern, q.Patterns, q.EventTypes, q.EventNames, q.EventFieldFilters, q.TraceMarkActions, q.LineStart, q.LineEnd}
}

func (f PerfPopulationFilters) Active() bool {
	return f.Pattern != "" || len(f.Patterns) > 0 || len(f.EventTypes) > 0 || len(f.EventNames) > 0 || len(f.EventFieldFilters) > 0 || len(f.TraceMarkActions) > 0 || f.LineStart != 0 || f.LineEnd != 0
}

// This witness is not serialized and is issued only after a native Run. A
// copied result JSON cannot use its public provenance as preparation authority.
type perfFilteredPopulationWitness struct {
	source, digest, sources string
	query                   Query
}

func filteredPerfContext(r Result) *PerfContext {
	if r.PerfStats != nil {
		return r.PerfStats
	}
	if r.WindowStats != nil {
		return r.WindowStats.PerfSamples
	}
	return nil
}

func filteredPerfDigest(p *PerfContext) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func sealFilteredPerfPopulation(r Result, q Query) Result {
	if r.ViewCancellation != nil || !perfLegacyViewCarriesSampling(q.View) {
		return r
	}
	p := filteredPerfContext(r)
	if p == nil || p.SampleCount <= 0 || p.ThreadIdentityCountExact == nil || !*p.ThreadIdentityCountExact || p.ThreadIdentityUnknownSampleCount != 0 {
		return r
	}
	// Freeze the effective query's slices; callers retain their own Query.
	b, err := json.Marshal(q)
	if err != nil {
		return r
	}
	var frozen Query
	if json.Unmarshal(b, &frozen) != nil {
		return r
	}
	sources, err := json.Marshal(r.TraceArtifacts)
	if err != nil {
		return r
	}
	r.filteredPerfPopulation = &perfFilteredPopulationWitness{r.SourcePath, filteredPerfDigest(p), string(sources), frozen}
	return r
}

// FilteredPerfPopulation returns only a native, unchanged filtered population.
// Every cohort must keep its own frozen physical source generation. This is
// display selection, not complete Top-N publication or causal admission.
func FilteredPerfPopulation(r Result) (*PerfContext, Query, bool) {
	p, q, ok := NativePerfPopulation(r)
	return p, q, ok && PerfPopulationQueryFilters(q).Active()
}

// NativePerfPopulation also serves fixed producer-owned fact displays. It
// does not relax the filtered member fallback or certify a sampling tree.
func NativePerfPopulation(r Result) (*PerfContext, Query, bool) {
	w, p := r.filteredPerfPopulation, filteredPerfContext(r)
	if w == nil || r.ViewCancellation != nil || r.SourcePath != w.source || p == nil || filteredPerfDigest(p) != w.digest {
		return nil, Query{}, false
	}
	snapshot, err := json.Marshal(r.TraceArtifacts)
	if err != nil || string(snapshot) != w.sources || r.View != w.query.View || r.TimeStart != w.query.TimeStart || r.TimeEnd != w.query.TimeEnd || r.TargetScope != w.query.TargetScope {
		return nil, Query{}, false
	}
	sources := map[string]TraceArtifactSource{}
	for _, s := range r.TraceArtifacts {
		if _, exists := sources[s.SourcePath]; exists || s.SourcePath == "" || !s.sourceIdentity.Strong() || s.SourceBytes != s.sourceIdentity.Size() || s.SourceModUnixNano != s.sourceIdentity.ModUnixNano() {
			return nil, Query{}, false
		}
		if s.SourcePath != r.SourcePath && (s.BundleSchema != tracebundle.SchemaV2 || tracebundle.ValidateCaptureID(s.CaptureID) != nil || tracebundle.ValidateSHA256(s.SourceSHA256) != nil) {
			return nil, Query{}, false
		}
		sources[s.SourcePath] = s
	}
	for _, c := range p.Cohorts {
		if c.Scope == nil || c.Scope.ID == "" {
			return nil, Query{}, false
		}
		s, ok := sources[c.Scope.SourcePath]
		if !ok || c.Scope.SourceGeneration != perfSamplingSourceGeneration(s) {
			return nil, Query{}, false
		}
	}
	// Do not let a caller modify the private witness through returned slices.
	raw, _ := json.Marshal(w.query)
	var query Query
	if json.Unmarshal(raw, &query) != nil {
		return nil, Query{}, false
	}
	return p, query, len(p.Cohorts) > 0
}
