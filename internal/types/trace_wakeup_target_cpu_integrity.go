package types

import (
	"strconv"
	"strings"
)

type TraceWakeupTargetCPUIntegrityStatus string

const TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero TraceWakeupTargetCPUIntegrityStatus = "suspected_degraded_all_zero"

// TraceWakeupTargetCPUIntegrity is the compiled, query-window-scoped advisory
// that qualifies wakeup target_cpu placement.  It does not withdraw the raw
// scheduler rows, alter wakeup relations, rank a cause, or create a hard
// answer gate.  The private scopes retain the exact artifact/window ownership
// needed to avoid a stale or different-artifact probe poisoning healthy rows.
type TraceWakeupTargetCPUIntegrity struct {
	Status          TraceWakeupTargetCPUIntegrityStatus
	ObservedCount   int
	ZeroCount       int
	EmitterCPUCount int
	scopes          []traceWakeupTargetCPUIntegrityScope
}

type traceWakeupTargetCPUIntegrityScope struct {
	SourceRef ObservationSourceRef
	StartTs   float64
	EndTs     float64
}

func (i TraceWakeupTargetCPUIntegrity) SuspectedDegraded() bool {
	return i.Status == TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero &&
		i.ObservedCount > 0 && i.ZeroCount == i.ObservedCount && i.EmitterCPUCount >= 2 && len(i.scopes) > 0
}

// BuildTraceWakeupTargetCPUIntegrity consumes only producer-owned typed
// observations.  It never parses a tool banner, request, model draft, final
// prose, or Mermaid payload.  Repeated publications use MAX, never SUM.
func BuildTraceWakeupTargetCPUIntegrity(ledger ObservationLedger) TraceWakeupTargetCPUIntegrity {
	var out TraceWakeupTargetCPUIntegrity
	seenScope := make(map[string]bool)
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != ClaimGroundingHard ||
			strings.TrimSpace(record.Predicate) != "wakeup_target_cpu_integrity" ||
			strings.TrimSpace(record.Object) != string(TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero) {
			continue
		}
		notes := traceWakeupTargetCPUIntegrityNotes(record.RichNotes)
		observed, observedOK := traceWakeupTargetCPUIntegrityPositiveInt(notes[TraceNoteKeyWakeupTargetCPUObservedCount])
		zero, zeroOK := traceWakeupTargetCPUIntegrityPositiveInt(notes[TraceNoteKeyWakeupTargetCPUZeroCount])
		emitters, emittersOK := traceWakeupTargetCPUIntegrityPositiveInt(notes[TraceNoteKeyWakeupTargetCPUEmitterCPUCount])
		if !observedOK || !zeroOK || !emittersOK || zero != observed || emitters < 2 {
			continue
		}
		if observed > out.ObservedCount {
			out.ObservedCount = observed
			out.ZeroCount = zero
		}
		if emitters > out.EmitterCPUCount {
			out.EmitterCPUCount = emitters
		}
		out.Status = TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero
		key := traceWakeupTargetCPUIntegrityScopeKey(record.SourceRef, record.Span.StartTs, record.Span.EndTs)
		if !seenScope[key] {
			seenScope[key] = true
			out.scopes = append(out.scopes, traceWakeupTargetCPUIntegrityScope{
				SourceRef: record.SourceRef,
				StartTs:   record.Span.StartTs,
				EndTs:     record.Span.EndTs,
			})
		}
	}
	return out
}

// AffectsWakeupRecord reports whether this advisory owns the artifact/window
// of one raw wakeup edge.  It is an authority-display decision only.
func (i TraceWakeupTargetCPUIntegrity) AffectsWakeupRecord(record ObservationRecord) bool {
	if !i.SuspectedDegraded() || strings.TrimSpace(record.Predicate) != "wakeup_chain_edge" {
		return false
	}
	ts, tsKnown := traceWakeupTargetCPUIntegrityRecordTimestamp(record)
	for _, scope := range i.scopes {
		if !traceWakeupTargetCPUIntegritySameSource(scope.SourceRef, record.SourceRef) {
			continue
		}
		if scope.EndTs > scope.StartTs && tsKnown && (ts < scope.StartTs || ts > scope.EndTs) {
			continue
		}
		return true
	}
	return false
}

func (i TraceWakeupTargetCPUIntegrity) AffectsAnyWakeupRecord(ledger ObservationLedger) bool {
	for _, record := range ledger.Records {
		if i.AffectsWakeupRecord(record) {
			return true
		}
	}
	return false
}

func traceWakeupTargetCPUIntegrityNotes(notes []string) map[string]string {
	out := make(map[string]string)
	for _, note := range notes {
		key, value, ok := strings.Cut(strings.TrimSpace(note), "=")
		if ok && strings.TrimSpace(key) != "" {
			out[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return out
}

func traceWakeupTargetCPUIntegrityPositiveInt(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	return n, err == nil && n > 0
}

func traceWakeupTargetCPUIntegrityRecordTimestamp(record ObservationRecord) (float64, bool) {
	if raw := traceWakeupCPUTopologyNoteValue(record.RichNotes, TraceNoteKeyWakeupTs); raw != "" {
		ts, err := strconv.ParseFloat(raw, 64)
		return ts, err == nil
	}
	if record.Span.StartTs != 0 || record.Span.EndTs != 0 {
		return record.Span.StartTs, true
	}
	return 0, false
}

func traceWakeupTargetCPUIntegritySameSource(left, right ObservationSourceRef) bool {
	leftKeys := traceWakeupTargetCPUIntegritySourceKeys(left)
	rightKeys := traceWakeupTargetCPUIntegritySourceKeys(right)
	for key := range leftKeys {
		if rightKeys[key] {
			return true
		}
	}
	return false
}

func traceWakeupTargetCPUIntegritySourceKeys(ref ObservationSourceRef) map[string]bool {
	out := make(map[string]bool)
	for prefix, value := range map[string]string{
		"capture:":  ref.CaptureIdentityPath,
		"path:":     ref.Path,
		"artifact:": ref.ArtifactID,
	} {
		if value = strings.TrimSpace(value); value != "" {
			out[prefix+value] = true
		}
	}
	return out
}

func traceWakeupTargetCPUIntegrityScopeKey(ref ObservationSourceRef, start, end float64) string {
	keys := traceWakeupTargetCPUIntegritySourceKeys(ref)
	parts := make([]string, 0, len(keys)+2)
	for key := range keys {
		parts = append(parts, key)
	}
	// Ordering of a map is irrelevant to correctness, but a duplicate scope
	// must collapse deterministically even when several identity axes exist.
	sortStrings(parts)
	parts = append(parts, strconv.FormatFloat(start, 'f', 6, 64), strconv.FormatFloat(end, 'f', 6, 64))
	return strings.Join(parts, "\x00")
}
