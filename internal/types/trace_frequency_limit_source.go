package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CloneTraceFrequencyLimitAuthority detaches the receipt, including optional
// clock values. Consumers never re-stamp a published or memoized witness.
func CloneTraceFrequencyLimitAuthority(w TraceFrequencyLimitAuthority) TraceFrequencyLimitAuthority {
	if w.SourceRef != nil {
		source := *w.SourceRef
		if source.ClockOffsetSec != nil {
			v := *source.ClockOffsetSec
			source.ClockOffsetSec = &v
		}
		if source.ClockSlope != nil {
			v := *source.ClockSlope
			source.ClockSlope = &v
		}
		w.SourceRef = &source
	}
	return w
}

// TraceFrequencyLimitSourceRecord is only a source descriptor. CPU policy is
// not a target-thread observation; in particular this must not mint Subject,
// a causal relation, or a new evidence ID.
func TraceFrequencyLimitSourceRecord(w TraceFrequencyLimitAuthority) ObservationRecord {
	w = CloneTraceFrequencyLimitAuthority(w)
	r := ObservationRecord{Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", ObservedAt: w.ObservedAt}
	if w.SourceRef != nil {
		r.SourceRef = *w.SourceRef
	}
	if traceQueryScopeWindowPresent(w.WindowStartTs, w.WindowEndTs) {
		r.RichNotes = []string{fmt.Sprintf("selected_window=%.9f..%.9f", w.WindowStartTs, w.WindowEndTs)}
	}
	return r
}

func traceFrequencyLimitSourceKnown(w TraceFrequencyLimitAuthority) bool {
	if w.SourceRef == nil || w.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		strings.TrimSpace(w.SourceRef.QueryScopeID) == "" || strings.TrimSpace(w.ObservedAt) == "" ||
		(strings.TrimSpace(w.SourceRef.PayloadRef) == "" && strings.TrimSpace(w.SourceRef.RawRef) == "") ||
		!traceQueryScopeWindowPresent(w.WindowStartTs, w.WindowEndTs) {
		return false
	}
	// A display alias or a canonical identity alone is not an addressable
	// result carrier. Require the exact producer path as well as its receipt.
	path := strings.TrimSpace(w.SourceRef.Path)
	return path != "" && !traceCausalProjectionNonArtifactIDTokens[path]
}

// TraceFrequencyLimitSourceKey groups only fully addressed query results.
// Unknown legacy receipts return an empty key and must remain independent.
func TraceFrequencyLimitSourceKey(w TraceFrequencyLimitAuthority) string {
	if !traceFrequencyLimitSourceKnown(w) {
		return ""
	}
	s := w.SourceRef
	key, _ := json.Marshal(struct {
		Path, Payload, Raw, Query, Call, Observed, Capture string
		Start, End                                         float64
	}{s.Path, s.PayloadRef, s.RawRef, s.QueryScopeID, s.ToolCallID, w.ObservedAt, s.CaptureIdentityPath, w.WindowStartTs, w.WindowEndTs})
	return string(key)
}

// TraceFrequencyLimitMatchesRecord pairs a CPU-policy fact with observations
// of the very same producer result. Time proximity, CPU equality, and target
// names never substitute for source identity. Query window comparison follows
// receipt equality, so its serialization tolerance cannot merge queries.
func TraceFrequencyLimitMatchesRecord(w TraceFrequencyLimitAuthority, r ObservationRecord) bool {
	if !traceFrequencyLimitSourceKnown(w) || r.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.SourceRef.Kind != ObservationSourceRuntimeArtifact {
		return false
	}
	x, y := w.SourceRef, r.SourceRef
	if x.Path != y.Path || x.PayloadRef != y.PayloadRef || x.RawRef != y.RawRef ||
		x.QueryScopeID != y.QueryScopeID || w.ObservedAt != r.ObservedAt {
		return false
	}
	if x.ToolCallID != "" && y.ToolCallID != "" && x.ToolCallID != y.ToolCallID {
		return false
	}
	// Ledger preflight may add canonical capture identity while preserving
	// the exact Path. One absent enrichment is not a contradiction; two
	// conflicting proven identities are. No basename or suffix alias join.
	if x.CaptureIdentityPath != "" && y.CaptureIdentityPath != "" && x.CaptureIdentityPath != y.CaptureIdentityPath {
		return false
	}
	start, end, known := TraceCausalProjectionSelectedWindowNote(r.RichNotes)
	return known && traceQueryScopeWindowPresent(start, end) &&
		TraceCausalProjectionPrincipalValueSameWindow(w.WindowStartTs, w.WindowEndTs, start, end)
}

// DedupTraceFrequencyLimitAuthorities preserves input order and the caller's
// display budget. Only equal values with a complete equal source receipt may
// collapse. A missing source does not establish that two facts are duplicates.
func DedupTraceFrequencyLimitAuthorities(in []TraceFrequencyLimitAuthority, limit int) []TraceFrequencyLimitAuthority {
	var out []TraceFrequencyLimitAuthority
	seen := make(map[string]bool)
	for _, w := range in {
		if TraceFrequencyLimitSourceKey(w) != "" {
			encoded, err := json.Marshal(w)
			if err == nil {
				key := string(encoded)
				if seen[key] {
					continue
				}
				seen[key] = true
			}
		}
		out = append(out, CloneTraceFrequencyLimitAuthority(w))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
