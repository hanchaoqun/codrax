package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

// A request identity is minted only alongside the native source-read receipt.
// It survives in-memory copies, not JSON. Display notes can request isolation;
// they cannot mint physical identity or any causal qualification.
type traceIORequestIdentity struct {
	physical, snapshot string
}

// TraceIORequestEndpoints is the original paired request, independently of
// clipped request time or the issuer's blocked interval. This public fact
// carrier grants no identity permission without the private source receipt.
type TraceIORequestEndpoints struct {
	Family       string  `json:"family"`
	IssueLine    int     `json:"issue_line"`
	CompleteLine int     `json:"complete_line"`
	IssueTs      float64 `json:"issue_ts"`
	CompleteTs   float64 `json:"complete_ts"`
	Device       string  `json:"device,omitempty"`
	Operation    string  `json:"operation,omitempty"`
	Sector       string  `json:"sector,omitempty"`
	Length       string  `json:"length,omitempty"`
}

func (p TraceIORequestEndpoints) valid() bool {
	return (p.Family == "block_rq" || p.Family == "block_bio") && p.IssueLine > 0 && p.CompleteLine > p.IssueLine &&
		p.IssueTs >= 0 && p.CompleteTs >= p.IssueTs && !math.IsNaN(p.IssueTs) && !math.IsNaN(p.CompleteTs) && !math.IsInf(p.IssueTs, 0) && !math.IsInf(p.CompleteTs, 0)
}

func traceIORequestRecord(record ObservationRecord) bool {
	return record.ioRequestIdentity != nil || record.IORequestEndpoints != nil || record.Predicate == "io_latency" ||
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyType) == "io_latency" ||
		NormalizeTraceIOValueCaliber(traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOValueCaliber)) != ""
}

func traceIORequestRecordSnapshot(record ObservationRecord) string {
	// Bind the original publication context without turning query coordinates
	// into physical endpoints. Preserve exact names and clocks.
	data, _ := json.Marshal([]any{record.Producer, record.Origin, record.Negative, record.GroundingPolicy,
		record.SourceRef.Kind, record.SourceRef.ArtifactKind, record.SourceRef.Path,
		record.SourceRef.TimeDomain, record.SourceRef.CanonicalTimeDomain, record.SourceRef.ClockAlignment,
		record.SourceRef.ClockCalibrated, record.SourceRef.ClockOffsetSec, record.SourceRef.ClockSlope,
		record.Subject, record.Object, record.Predicate, record.Role, record.Span, record.RichNotes,
		record.SourceRef.QueryTargetPID, record.SourceRef.QueryTargetThread, record.SourceRef.QueryTargetScope,
		record.SourceRef.QueryWindowKnown, record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs,
		record.SourceRef.QueryLineRangeKnown, record.SourceRef.QueryLineStart, record.SourceRef.QueryLineEnd,
		record.IORequestEndpoints, record.Value, record.Unit})
	return string(data)
}

func stampTraceIORequestIdentities(result *ToolResult, ref TraceQuerySourceReadRef) {
	for i := range result.Observations {
		r := &result.Observations[i]
		r.ioRequestIdentity = nil
		if !traceIORequestRecord(*r) || r.Negative || !RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
			r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.GroundingPolicy != ClaimGroundingHard ||
			r.SourceRef.Kind != ObservationSourceRuntimeArtifact || r.SourceRef.ArtifactKind != "trace" ||
			r.IORequestEndpoints == nil || !r.IORequestEndpoints.valid() {
			continue
		}
		path, generation, ok := traceSourcePhysicalPath(r.SourceRef.Path)
		if !ok || path != ref.path || !ref.identity.SameVersion(generation) {
			continue
		}
		// The original issue/completion lines identify a paired request, even
		// when a view clips its numeric window. Optional device/request fields
		// constrain compatibility but are never required to exist.
		parts := []any{path, ref.identity.CacheToken(), fmt.Sprintf("%p", ref.generation), r.IORequestEndpoints}
		data, _ := json.Marshal(parts)
		sum := sha256.Sum256(data)
		r.ioRequestIdentity = &traceIORequestIdentity{physical: hex.EncodeToString(sum[:]), snapshot: traceIORequestRecordSnapshot(*r)}
	}
}

// TraceObservationIOPublicationKey returns an exact native publication key.
// handled with an empty key means the row is an IO request measurement but
// identity is unknown: keep it, do not group it with other unknown rows.
func TraceObservationIOPublicationKey(record ObservationRecord) (key string, handled bool) {
	if !traceIORequestRecord(record) {
		return "", false
	}
	identity := record.ioRequestIdentity
	if identity == nil || identity.snapshot != traceIORequestRecordSnapshot(record) {
		return "", true
	}
	ref := record.SourceRef
	data, _ := json.Marshal([]any{identity.physical, ref.TimeDomain, ref.CanonicalTimeDomain, ref.ClockAlignment,
		ref.ClockCalibrated, ref.ClockOffsetSec, ref.ClockSlope, record.Subject, record.Object, record.Predicate,
		record.Role, record.Value, record.Unit, record.Span.StartTs, record.Span.EndTs,
		ref.QueryTargetPID, ref.QueryTargetThread, ref.QueryTargetScope, ref.QueryLineRangeKnown, ref.QueryLineStart, ref.QueryLineEnd,
		ref.QueryWindowKnown, ref.QueryWindowStartTs, ref.QueryWindowEndTs,
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIOValueCaliber),
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyIORequestResidenceCaliber),
		traceCausalProjectionChainRelevance(record.RichNotes),
		traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyCausality)})
	return string(data), true
}

// TraceCausalProjectionIOPublicationKey retains request identity AND every
// selected numeric ruler. Equal duration/overlap is never request identity.
func TraceCausalProjectionIOPublicationKey(node TraceCausalProjectionNode) (key string, handled bool) {
	if !node.IORequestMeasurement && node.ioRequestIdentity == "" && node.TypeToken != "io_latency" && node.Predicate != "io_latency" && NormalizeTraceIOValueCaliber(node.IOValueCaliber) == "" {
		return "", false
	}
	if node.ioRequestIdentity == "" {
		return "", true
	}
	data, _ := json.Marshal([]any{node.ioRequestIdentity, node.IOValueCaliber, node.Value, node.Unit,
		node.Role, node.Subject, node.Object, node.Predicate, node.ChainRelevance, node.Causality, node.OnChainBasis,
		node.ChainDepth, node.ChainBranch, node.ResourceCompletionClosure, node.RankBoardTarget, node.RankBoardParamsFingerprint,
		node.StartTs, node.EndTs, node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS,
		node.ActualImpactMS, node.TargetImpactMS, node.ChainAnchorRemainderSeat, node.GatedShareConstituentSeat})
	return string(data), true
}

// TraceCausalProjectionIOPublicationMatch is the single IO identity boundary
// for engine and presentation folds. Non-IO rows retain their existing rules.
func TraceCausalProjectionIOPublicationMatch(a, b TraceCausalProjectionNode) (same, handled bool) {
	ak, ah := TraceCausalProjectionIOPublicationKey(a)
	bk, bh := TraceCausalProjectionIOPublicationKey(b)
	return ah && bh && ak != "" && ak == bk && !TraceSchedulerMeasurementRestrictionsConflict(a.MeasurementOrigins, b.MeasurementOrigins), ah || bh
}

func traceCausalProjectionIOFoldGuard(a, b TraceCausalProjectionNode) bool {
	same, handled := TraceCausalProjectionIOPublicationMatch(a, b)
	return !handled || same
}
