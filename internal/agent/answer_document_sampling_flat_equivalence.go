package agent

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the existing producer's display envelope, not a new wire contract.
// Unknown fields or non-canonical JSON retain the original independent display.
type samplingQualityDisplayEnvelope struct {
	Version         int                            `json:"version"`
	QuerySourcePath string                         `json:"query_source_path"`
	QueryScopeID    string                         `json:"query_scope_id"`
	PayloadRef      string                         `json:"payload_ref"`
	Scope           *tracequery.PerfCohortScope    `json:"scope"`
	Event           string                         `json:"event"`
	WeightUnit      string                         `json:"weight_unit"`
	Samples         int                            `json:"samples"`
	Weight          int64                          `json:"weight"`
	Quality         *tracequery.PerfQualitySummary `json:"quality"`
}

func (p *answerDocNativePresentationReceipt) samplingFlatKey(r types.ObservationRecord) string {
	if p == nil || (r.Predicate != "perf_sample_top_symbol" && r.Predicate != "perf_sample_top_callchain") || p.nativeSources[traceEventPublicationRecordKey(r)] == "" {
		return ""
	}
	query := nativePresentationQueryKey(r)
	population := p.samplingIdentities[query]
	if population == "" {
		return ""
	}
	if r.Predicate == "perf_sample_top_callchain" || r.Value == "" && len(r.RichNotes) == 0 {
		// Opaque legacy summaries are never parsed or rewritten. Only exact
		// complete content can share its already-proven physical population.
		data, _ := json.Marshal([]any{population, r.Predicate, r.Span, r.Subject, r.Object, r.Value, r.Unit, r.Summary, r.RawExcerpt, r.RichNotes})
		return string(data)
	}
	notes := append([]string(nil), r.RichNotes...)
	found := false
	for i, note := range notes {
		raw, matched := strings.CutPrefix(note, types.TraceNoteKeyPerfQualityScope+"=")
		if !matched {
			continue
		}
		if found {
			return ""
		}
		found = true
		var scope samplingQualityDisplayEnvelope
		if json.Unmarshal([]byte(raw), &scope) != nil {
			return ""
		}
		canonical, _ := json.Marshal(scope)
		if string(canonical) != raw || scope.Version != 1 || scope.Scope == nil || scope.Quality == nil ||
			scope.QuerySourcePath != r.SourceRef.Path || scope.QueryScopeID != r.SourceRef.QueryScopeID || scope.PayloadRef != r.SourceRef.PayloadRef || !p.samplingCohortIDs[query][scope.Scope.ID] {
			return ""
		}
		// All quantities, quality dimensions and source/thread/window fields
		// survive. Only already-validated query and cohort reference IDs differ.
		scope.QueryScopeID, scope.PayloadRef, scope.Scope.ID = "", "", ""
		data, _ := json.Marshal(scope)
		notes[i] = types.TraceNoteKeyPerfQualityScope + "=" + string(data)
	}
	if !found {
		return ""
	}
	data, _ := json.Marshal([]any{population, r.Predicate, r.Span, r.Subject, r.Object, r.Value, r.Unit, notes})
	return string(data)
}
