package hitraceconv

// P1-a2.2-B2: fixed structured-event diagnostics. Direct renderer callers
// retain their per-call compatibility coverage; profiler containers merge one
// typed batch per TracePluginResult into this 39-slot terminal ledger.

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	profilerFtraceEventEnvelopeSlot = len(profilerFtraceEventDescriptorList) + iota
	profilerFtraceCPUDetailEnvelopeSlot
	profilerFtraceUnknownEventSlot
	profilerFtraceEventSlotCount
)

type profilerFtraceEventDegradationKind uint8

const (
	profilerFtraceEventDegradationEnvelope profilerFtraceEventDegradationKind = iota
	profilerFtraceEventDegradationCorePayload
	profilerFtraceEventDegradationCoreDisplay
	profilerFtraceEventDegradationAuxPayload
	profilerFtraceEventDegradationAuxDisplay
	profilerFtraceEventDegradationFilemapPayload
	profilerFtraceEventDegradationBlockPayload
	profilerFtraceEventDegradationWireAudit
	profilerFtraceEventDegradationFieldAudit
	profilerFtraceEventDegradationUnmappedField
	profilerFtraceEventDegradationKindCount
)

func (kind profilerFtraceEventDegradationKind) label() string {
	switch kind {
	case profilerFtraceEventDegradationEnvelope:
		return "envelope"
	case profilerFtraceEventDegradationCorePayload:
		return "core_payload"
	case profilerFtraceEventDegradationCoreDisplay:
		return "core_display"
	case profilerFtraceEventDegradationAuxPayload:
		return "aux_payload"
	case profilerFtraceEventDegradationAuxDisplay:
		return "aux_display"
	case profilerFtraceEventDegradationFilemapPayload:
		return "filemap_payload"
	case profilerFtraceEventDegradationBlockPayload:
		return "block_payload"
	case profilerFtraceEventDegradationWireAudit:
		return "wire_audit"
	case profilerFtraceEventDegradationFieldAudit:
		return "field_audit"
	case profilerFtraceEventDegradationUnmappedField:
		return "unmapped_field"
	default:
		return "event_degradation_invalid"
	}
}

type profilerFtraceEventDegradation struct {
	Kind   profilerFtraceEventDegradationKind
	Reason string
}

func profilerFtraceEventDegradations(kind profilerFtraceEventDegradationKind, reasons []string) []profilerFtraceEventDegradation {
	out := make([]profilerFtraceEventDegradation, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, profilerFtraceEventDegradation{Kind: kind, Reason: reason})
	}
	return out
}

type profilerFtraceEventSlotCensus struct {
	RowsRead       uint64
	RowsEmitted    uint64
	Occurrences    [profilerFtraceEventDegradationKindCount]uint64
	AffectedFrames [profilerFtraceEventDegradationKindCount]uint64
	ReasonSamples  profilerStableSampleSet
}

type profilerFtraceEventBatchCensus struct {
	Slots               [profilerFtraceEventSlotCount]profilerFtraceEventSlotCensus
	UnknownFieldSamples profilerStableSampleSet
	Overflow            bool
}

func (batch *profilerFtraceEventBatchCensus) observeRead(field int) bool {
	if batch == nil || batch.Overflow {
		return false
	}
	slot := profilerFtraceEventSlot(field)
	if !checkedProfilerUint64AddTo(&batch.Slots[slot].RowsRead, 1) {
		batch.Overflow = true
		return false
	}
	if slot == profilerFtraceUnknownEventSlot {
		batch.UnknownFieldSamples.observe("profiler-ftrace-unknown-event-field", []byte(strconv.Itoa(field)))
	}
	return true
}

func (batch *profilerFtraceEventBatchCensus) observeEmitted(field int) bool {
	if batch == nil || batch.Overflow {
		return false
	}
	if !checkedProfilerUint64AddTo(&batch.Slots[profilerFtraceEventSlot(field)].RowsEmitted, 1) {
		batch.Overflow = true
		return false
	}
	return true
}

func (batch *profilerFtraceEventBatchCensus) observeDegradations(field int, degradations []profilerFtraceEventDegradation) bool {
	if batch == nil || batch.Overflow {
		return false
	}
	slotIndex := profilerFtraceEventSlot(field)
	slot := &batch.Slots[slotIndex]
	for _, degradation := range degradations {
		if degradation.Kind >= profilerFtraceEventDegradationKindCount || strings.TrimSpace(degradation.Reason) == "" {
			batch.Overflow = true
			return false
		}
		kindIndex := int(degradation.Kind)
		firstForFrame := slot.Occurrences[kindIndex] == 0
		if !checkedProfilerUint64AddTo(&slot.Occurrences[kindIndex], 1) {
			batch.Overflow = true
			return false
		}
		if firstForFrame && !checkedProfilerUint64AddTo(&slot.AffectedFrames[kindIndex], 1) {
			batch.Overflow = true
			return false
		}
		slot.ReasonSamples.observe("profiler-ftrace-event-reason:"+strconv.Itoa(slotIndex), []byte(degradation.Reason))
	}
	return true
}

func (batch profilerFtraceEventBatchCensus) degraded() bool {
	for _, slot := range batch.Slots {
		for _, count := range slot.Occurrences {
			if count > 0 {
				return true
			}
		}
	}
	return false
}

type profilerFtraceEventDiagnosticLedger struct {
	Slots               [profilerFtraceEventSlotCount]profilerFtraceEventSlotCensus
	UnknownFieldSamples profilerStableSampleSet
}

func (ledger *profilerFtraceEventDiagnosticLedger) merge(batch profilerFtraceEventBatchCensus) bool {
	if ledger == nil || batch.Overflow {
		return false
	}
	for slotIndex := range ledger.Slots {
		target := &ledger.Slots[slotIndex]
		source := batch.Slots[slotIndex]
		if !checkedProfilerUint64AddTo(&target.RowsRead, source.RowsRead) ||
			!checkedProfilerUint64AddTo(&target.RowsEmitted, source.RowsEmitted) {
			return false
		}
		for kindIndex := range target.Occurrences {
			if !checkedProfilerUint64AddTo(&target.Occurrences[kindIndex], source.Occurrences[kindIndex]) ||
				!checkedProfilerUint64AddTo(&target.AffectedFrames[kindIndex], source.AffectedFrames[kindIndex]) {
				return false
			}
		}
		mergeProfilerStableSampleSet(&target.ReasonSamples, source.ReasonSamples)
	}
	mergeProfilerStableSampleSet(&ledger.UnknownFieldSamples, batch.UnknownFieldSamples)
	return true
}

func mergeProfilerStableSampleSet(target *profilerStableSampleSet, source profilerStableSampleSet) {
	if target == nil {
		return
	}
	for index := 0; index < int(source.Used); index++ {
		insertProfilerDiagnosticSample(target, source.Items[index])
	}
}

func insertProfilerDiagnosticSample(samples *profilerStableSampleSet, item profilerDiagnosticSample) {
	used := int(samples.Used)
	insert := used
	for index := 0; index < used; index++ {
		comparison := strings.Compare(string(item.Digest[:]), string(samples.Items[index].Digest[:]))
		if comparison == 0 {
			return
		}
		if comparison < 0 {
			insert = index
			break
		}
	}
	if used == profilerDiagnosticSampleLimit && insert == used {
		return
	}
	if used < profilerDiagnosticSampleLimit {
		used++
		samples.Used = uint8(used)
	}
	for index := used - 1; index > insert; index-- {
		samples.Items[index] = samples.Items[index-1]
	}
	samples.Items[insert] = item
}

type profilerFtraceEventCoverageIndexes struct {
	Present [profilerFtraceEventSlotCount]bool
	Index   [profilerFtraceEventSlotCount]int
}

func (indexes profilerFtraceEventCoverageIndexes) coverageIndexForField(field int) (int, bool) {
	slot := profilerFtraceEventSlot(field)
	return indexes.Index[slot], indexes.Present[slot]
}

func (ledger *profilerFtraceEventDiagnosticLedger) materialize(out *profilerContainerExtraction) bool {
	if ledger == nil || out == nil {
		return false
	}
	for slotIndex, census := range ledger.Slots {
		if census.RowsRead == 0 {
			continue
		}
		rowsRead, ok := profilerContainerCountToInt(census.RowsRead)
		if !ok {
			return false
		}
		rowsEmitted, ok := profilerContainerCountToInt(census.RowsEmitted)
		if !ok || rowsEmitted > rowsRead {
			return false
		}
		field, known := profilerFtraceEventFieldForSlot(slotIndex)
		var coverage TraceDBCoverage
		if slotIndex == profilerFtraceUnknownEventSlot {
			coverage = TraceDBCoverage{
				Family: "builtin_modern_ftrace:unknown", Table: "__unknown_event_field__",
				Role: "unsupported_input", Found: true,
				FieldSources: map[string]string{
					"aggregation_policy": "fixed_unknown_event_field_bucket",
					"sample_policy":      "sha256_min_k8_domain_separated_prefix96_bounded_examples",
				},
			}
			if ledger.UnknownFieldSamples.Used > 0 {
				coverage.FieldSources["event_field_samples"] = ledger.UnknownFieldSamples.render()
			}
		} else if known {
			coverage = *profilerFtraceEventRenderCoverage(map[int]*TraceDBCoverage{}, field)
			if coverage.FieldSources == nil {
				coverage.FieldSources = map[string]string{}
			}
			coverage.FieldSources["event_field_id"] = strconv.Itoa(field)
			coverage.FieldSources["aggregation_policy"] = "fixed_typed_event_field_slot"
		} else {
			return false
		}
		coverage.RowsRead = rowsRead
		coverage.RowsEmitted = rowsEmitted
		coverage.Skipped = profilerFtraceEventDegradationSummary(census)
		for kind := profilerFtraceEventDegradationKind(0); kind < profilerFtraceEventDegradationKindCount; kind++ {
			index := int(kind)
			if census.Occurrences[index] == 0 {
				continue
			}
			prefix := "degraded_" + kind.label()
			coverage.FieldSources[prefix+"_occurrences"] = strconv.FormatUint(census.Occurrences[index], 10)
			coverage.FieldSources[prefix+"_affected_frames"] = strconv.FormatUint(census.AffectedFrames[index], 10)
		}
		if census.ReasonSamples.Used > 0 {
			coverage.FieldSources["degradation_reason_samples"] = census.ReasonSamples.render()
			coverage.FieldSources["degradation_reason_sample_policy"] = "sha256_min_k8_domain_separated_prefix96_bounded_examples"
		}
		if reason, count, exact := profilerFtraceSingleExactReason(census); exact && traceDBSingleToken(reason) {
			coverage.FieldSources["degraded_"+reason+"_rows"] = strconv.FormatUint(count, 10)
		}
		out.profilerEventCoverage.Present[slotIndex] = true
		out.profilerEventCoverage.Index[slotIndex] = len(out.TraceCoverage)
		out.TraceCoverage = append(out.TraceCoverage, coverage)
	}
	return true
}

func profilerFtraceEventDegradationSummary(census profilerFtraceEventSlotCensus) string {
	parts := make([]string, 0, profilerFtraceEventDegradationKindCount+1)
	for kind := profilerFtraceEventDegradationKind(0); kind < profilerFtraceEventDegradationKindCount; kind++ {
		if count := census.Occurrences[int(kind)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind.label(), count))
		}
	}
	if reason, count, exact := profilerFtraceSingleExactReason(census); exact {
		parts = append(parts, fmt.Sprintf("%s=%d", reason, count))
	} else if census.ReasonSamples.Used > 0 {
		parts = append(parts, "reason_samples="+census.ReasonSamples.render())
	}
	return strings.Join(parts, ",")
}

func profilerFtraceSingleExactReason(census profilerFtraceEventSlotCensus) (string, uint64, bool) {
	if census.ReasonSamples.Used != 1 {
		return "", 0, false
	}
	item := census.ReasonSamples.Items[0]
	if item.InputLen != uint64(item.PrefixLen) {
		return "", 0, false
	}
	var total uint64
	for _, count := range census.Occurrences {
		if !checkedProfilerUint64AddTo(&total, count) {
			return "", 0, false
		}
	}
	return strings.ToValidUTF8(string(item.Prefix[:item.PrefixLen]), "�"), total, total > 0
}

func profilerFtraceEventSlot(field int) int {
	if field == 0 {
		return profilerFtraceEventEnvelopeSlot
	}
	if field == profilerFtraceCPUDetailEnvelopeField {
		return profilerFtraceCPUDetailEnvelopeSlot
	}
	if slot, ok := profilerFtraceEventDescriptorSlot(field); ok {
		return slot
	}
	return profilerFtraceUnknownEventSlot
}

func profilerFtraceEventFieldForSlot(slot int) (int, bool) {
	switch slot {
	case profilerFtraceEventEnvelopeSlot:
		return 0, true
	case profilerFtraceCPUDetailEnvelopeSlot:
		return profilerFtraceCPUDetailEnvelopeField, true
	case profilerFtraceUnknownEventSlot:
		return 0, false
	default:
		if slot >= 0 && slot < len(profilerFtraceEventDescriptorList) {
			return profilerFtraceEventDescriptorList[slot].Field, true
		}
		return 0, false
	}
}
