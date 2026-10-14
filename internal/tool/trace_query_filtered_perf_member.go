package tool

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only a verified native filtered population may issue this display receipt.
// Each member binds its own cohort and exact fields; a valid sibling cannot
// qualify other records in the same query. It does not certify a prefix tree.
type traceQueryFilteredPerfMemberReceipt struct {
	Version         int                              `json:"version"`
	SourcePath      string                           `json:"source_path"`
	QueryScopeID    string                           `json:"query_scope_id"`
	PayloadRef      string                           `json:"payload_ref"`
	TargetPID       int                              `json:"target_pid"`
	TargetThread    string                           `json:"target_thread"`
	Filters         tracequery.PerfPopulationFilters `json:"filters"`
	CohortID        string                           `json:"cohort_id"`
	MembersComplete bool                             `json:"members_complete"`
	RecordSHA256    string                           `json:"record_sha256"`
}

func traceQueryFilteredPerfMemberDigest(r types.ObservationRecord, receipt traceQueryFilteredPerfMemberReceipt) string {
	notes := make([]string, 0, len(r.RichNotes))
	for _, n := range r.RichNotes {
		if !strings.HasPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=") {
			notes = append(notes, n)
		}
	}
	receipt.RecordSHA256 = ""
	ref := r.SourceRef
	// CompileObservationLedger fills ToolCallID; its exact preflight requalify
	// (observation_ledger_runtime_artifact_identity.go) decorates the three
	// artifact labels. These do not grant this receipt's selection. Bind every
	// other source field, including the full clock mapping and physical path.
	ref.ToolCallID, ref.ArtifactID, ref.ArtifactKind, ref.CaptureIdentityPath = "", "", "", ""
	b, err := json.Marshal([]any{r.ClaimKey, r.Subject, r.Predicate, r.Object, r.Value, r.Unit, r.Span, ref, notes, receipt})
	if err != nil {
		return ""
	}
	return traceQuerySamplingTreeDigest(b)
}

func traceQueryFilteredPerfMemberReceipts(result tracequery.Result, records []types.ObservationRecord) []types.ObservationRecord {
	p, q, ok := tracequery.FilteredPerfPopulation(result)
	if !ok || q.TargetScope != "thread" {
		return records
	}
	pid, thread, valid := tracequery.NormalizePerfSamplingSelector(q.PID, q.Thread)
	if !valid {
		return records
	}
	for i := range records {
		r := &records[i]
		if r.Predicate != "perf_sample_top_symbol" || !strings.HasPrefix(r.ClaimKey, "perf_sample_top_symbol:cohort=") {
			continue
		}
		ref := r.SourceRef
		if ref.Path != result.SourcePath || ref.PayloadRef == "" || ref.QueryScopeID == "" || !ref.QueryWindowKnown || ref.QueryWindowStartTs != result.TimeStart || ref.QueryWindowEndTs != result.TimeEnd || ref.QueryTargetScope != "thread" || !ref.QueryLineRangeKnown || ref.QueryLineStart != q.LineStart || ref.QueryLineEnd != q.LineEnd {
			continue
		}
		for _, c := range p.Cohorts {
			if c.WeightStatus != "exact" || c.Scope == nil || c.Scope.ThreadIdentity == nil || c.ThreadIdentityCountExact == nil || !*c.ThreadIdentityCountExact || c.ThreadIdentityUnknownSampleCount != 0 || c.Scope.Window.StartTs != ref.QueryWindowStartTs || c.Scope.Window.EndTs != ref.QueryWindowEndTs {
				continue
			}
			for _, hot := range c.TopSymbols {
				if r.ClaimKey != traceQueryPerfCohortClaimKey(c, hot) || r.Subject != firstNonEmptyTraceString(hot.Symbol, "perf_sample") || r.Object != hot.DSO || r.Value != strconv.FormatInt(hot.Period, 10) || r.Span.LineStart != hot.LineStart || r.Span.LineEnd != hot.LineEnd {
					continue
				}
				population, _ := runtimeTracePerfQualityScope(*r)
				if population == nil {
					continue
				}
				actual, _ := json.Marshal(population)
				if string(actual) != traceQueryPerfQualityScopeNote(c, ref) {
					continue
				}
				complete := 0
				for _, member := range c.TopSymbols {
					complete += member.SampleCount
				}
				r.SourceRef.QueryTargetPID, r.SourceRef.QueryTargetThread = pid, thread
				receipt := traceQueryFilteredPerfMemberReceipt{Version: 1, SourcePath: ref.Path, QueryScopeID: ref.QueryScopeID, PayloadRef: ref.PayloadRef, TargetPID: pid, TargetThread: thread, Filters: tracequery.PerfPopulationQueryFilters(q), CohortID: c.Scope.ID, MembersComplete: complete == c.SampleCount && p.CohortCount == len(p.Cohorts)}
				receipt.RecordSHA256 = traceQueryFilteredPerfMemberDigest(*r, receipt)
				data, err := json.Marshal(receipt)
				if err == nil {
					r.RichNotes = append(r.RichNotes, types.TraceNoteKeyPerfFilteredMember+"="+string(data))
				}
			}
		}
	}
	return records
}

// TraceQueryFilteredPerfMemberSelection validates a member's own native
// filtered population. The returned complete bit covers only retained Top-N
// members in that cohort, not complete tree/stack/source/causal coverage.
func TraceQueryFilteredPerfMemberSelection(r types.ObservationRecord) (string, bool, bool) {
	ref := r.SourceRef
	if r.Negative || r.Predicate != "perf_sample_top_symbol" || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingHard || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact || !ref.QueryWindowKnown || !ref.QueryLineRangeKnown || ref.QueryTargetScope != "thread" {
		return "", false, false
	}
	var receipt traceQueryFilteredPerfMemberReceipt
	count := 0
	for _, n := range r.RichNotes {
		if !strings.HasPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=") {
			continue
		}
		count++
		raw := strings.TrimPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=")
		d := json.NewDecoder(strings.NewReader(raw))
		d.DisallowUnknownFields()
		if !traceRootCauseReportHasUniqueNestedKeys(json.RawMessage(raw)) || d.Decode(&receipt) != nil || d.Decode(new(any)) != io.EOF {
			return "", false, false
		}
	}
	if count != 1 || receipt.Version != 1 || !receipt.Filters.Active() || receipt.SourcePath == "" || receipt.SourcePath != ref.Path || receipt.QueryScopeID == "" || receipt.QueryScopeID != ref.QueryScopeID || receipt.PayloadRef == "" || receipt.PayloadRef != ref.PayloadRef || receipt.TargetPID != ref.QueryTargetPID || receipt.TargetThread != ref.QueryTargetThread || receipt.Filters.LineStart != ref.QueryLineStart || receipt.Filters.LineEnd != ref.QueryLineEnd || receipt.RecordSHA256 != traceQueryFilteredPerfMemberDigest(r, receipt) {
		return "", false, false
	}
	p, _ := runtimeTracePerfQualityScope(r)
	if p == nil || p.Scope == nil || p.Scope.ID != receipt.CohortID || p.Scope.ThreadIdentity == nil || p.Samples <= 0 || p.Weight < 0 || p.Scope.Window.StartTs != ref.QueryWindowStartTs || p.Scope.Window.EndTs != ref.QueryWindowEndTs {
		return "", false, false
	}
	id := p.Scope.ThreadIdentity
	if id.Generation <= 0 || id.TGID < 0 || id.TGID > types.RuntimeTargetMaxPID {
		return "", false, false
	}
	// Line predicates remain in the receipt and source ref. The shared thread
	// comparator does not need to reject this independently verified row filter.
	selectorRef := ref
	selectorRef.QueryLineStart, selectorRef.QueryLineEnd = 0, 0
	subject, ok := traceQuerySamplingSelectedThread(selectorRef, id.TID, id.DisplayComm)
	return subject, ok, receipt.MembersComplete
}
