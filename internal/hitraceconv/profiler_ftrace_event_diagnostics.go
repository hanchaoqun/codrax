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
	// Protobuf field numbers are limited to 29 bits. This representative is
	// used only to validate/label exact issues after distinct unknown IDs have
	// been compacted into the fixed unknown slot; it is never published as an
	// observed field identity.
	profilerFtraceUnknownEventAggregateField = 1<<29 - 1
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
	profilerFtraceEventDegradationBlockDisplay
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
	case profilerFtraceEventDegradationBlockDisplay:
		return "block_display"
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

// The largest closed per-event issue universe is MMC request-start: 25 wire
// duplicates + 25 wrong-wire fields + 23 range fields plus whole-payload and
// semantic failures. Keep one fixed slot for every legal tuple with headroom;
// this is a schema bound, not a runtime budget that valid input may exhaust.
const profilerFtraceEventIssuesPerSlot = 128

type profilerFtraceEventIssueCensus struct {
	Issue          profilerFtraceEventIssue
	Occurrences    uint64
	AffectedFrames uint64
}

type profilerFtraceEventSlotCensus struct {
	RowsRead            uint64
	RowsEmitted         uint64
	IssueCount          uint8
	Issues              [profilerFtraceEventIssuesPerSlot]profilerFtraceEventIssueCensus
	ClassAffectedFrames [profilerFtraceEventDegradationKindCount]uint64
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

func (batch *profilerFtraceEventBatchCensus) observeIssues(field int, publishable bool, issues []profilerFtraceEventIssue) bool {
	if batch == nil || batch.Overflow {
		return false
	}
	slotIndex := profilerFtraceEventSlot(field)
	slot := &batch.Slots[slotIndex]
	if !profilerFtraceEventIssueVerdictValid(field, publishable, issues) {
		batch.Overflow = true
		return false
	}
	var observedClass [profilerFtraceEventDegradationKindCount]bool
	for _, issue := range issues {
		class := issue.sourceClass()
		if class >= profilerFtraceEventDegradationKindCount {
			batch.Overflow = true
			return false
		}
		observedClass[int(class)] = true
		index, found := profilerFtraceEventIssueCensusIndex(slot, issue)
		if !found {
			if int(slot.IssueCount) >= len(slot.Issues) {
				batch.Overflow = true
				return false
			}
			used := int(slot.IssueCount)
			copy(slot.Issues[index+1:used+1], slot.Issues[index:used])
			slot.Issues[index] = profilerFtraceEventIssueCensus{Issue: issue}
			slot.IssueCount++
		}
		census := &slot.Issues[index]
		firstForFrame := census.Occurrences == 0
		if !checkedProfilerUint64AddTo(&census.Occurrences, 1) {
			batch.Overflow = true
			return false
		}
		if firstForFrame && !checkedProfilerUint64AddTo(&census.AffectedFrames, 1) {
			batch.Overflow = true
			return false
		}
	}
	for class, observed := range observedClass {
		if observed && slot.ClassAffectedFrames[class] == 0 &&
			!checkedProfilerUint64AddTo(&slot.ClassAffectedFrames[class], 1) {
			batch.Overflow = true
			return false
		}
	}
	return true
}

func profilerFtraceEventIssueVerdictValid(field int, publishable bool, issues []profilerFtraceEventIssue) bool {
	_, known := profilerFtraceEventDescriptors[field]
	if publishable && !known || !publishable && len(issues) == 0 {
		return false
	}
	unknownSlot := profilerFtraceEventSlot(field) == profilerFtraceUnknownEventSlot
	unmappedSeen := false
	for _, issue := range issues {
		if unknownSlot {
			switch {
			case issue.Kind == profilerFtraceEventIssueUnmappedField:
				unmappedSeen = true
			case issue.sourceClass() != profilerFtraceEventDegradationEnvelope:
				return false
			}
		}
		if !issue.validFor(field) || publishable != (issue.Severity == profilerFtraceEventIssueAdmittedDisplay) {
			return false
		}
	}
	return !unknownSlot || unmappedSeen
}

func profilerFtraceEventIssueCensusIndex(slot *profilerFtraceEventSlotCensus, issue profilerFtraceEventIssue) (int, bool) {
	if slot == nil {
		return 0, false
	}
	used := int(slot.IssueCount)
	if used > len(slot.Issues) {
		return len(slot.Issues), false
	}
	for index := 0; index < used; index++ {
		comparison := slot.Issues[index].Issue.compare(issue)
		if comparison == 0 {
			return index, true
		}
		if comparison > 0 {
			return index, false
		}
	}
	return used, false
}

func (batch profilerFtraceEventBatchCensus) degraded() bool {
	for _, slot := range batch.Slots {
		if slot.IssueCount > 0 {
			return true
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
		if int(source.IssueCount) > len(source.Issues) || int(target.IssueCount) > len(target.Issues) {
			return false
		}
		if !checkedProfilerUint64AddTo(&target.RowsRead, source.RowsRead) ||
			!checkedProfilerUint64AddTo(&target.RowsEmitted, source.RowsEmitted) {
			return false
		}
		for sourceIndex := 0; sourceIndex < int(source.IssueCount); sourceIndex++ {
			item := source.Issues[sourceIndex]
			index, found := profilerFtraceEventIssueCensusIndex(target, item.Issue)
			if !found {
				if int(target.IssueCount) >= len(target.Issues) {
					return false
				}
				used := int(target.IssueCount)
				copy(target.Issues[index+1:used+1], target.Issues[index:used])
				target.Issues[index] = profilerFtraceEventIssueCensus{Issue: item.Issue}
				target.IssueCount++
			}
			if !checkedProfilerUint64AddTo(&target.Issues[index].Occurrences, item.Occurrences) ||
				!checkedProfilerUint64AddTo(&target.Issues[index].AffectedFrames, item.AffectedFrames) {
				return false
			}
		}
		for class := range target.ClassAffectedFrames {
			if !checkedProfilerUint64AddTo(&target.ClassAffectedFrames[class], source.ClassAffectedFrames[class]) {
				return false
			}
		}
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
		issueField := field
		if slotIndex == profilerFtraceUnknownEventSlot {
			issueField = profilerFtraceUnknownEventAggregateField
		}
		classOccurrences, classAffected, reasonSamples, valid := profilerFtraceEventIssueSummaryCensus(issueField, census)
		if !valid {
			return false
		}
		coverage.Skipped = profilerFtraceEventDegradationSummary(issueField, census, classOccurrences, reasonSamples)
		for kind := profilerFtraceEventDegradationKind(0); kind < profilerFtraceEventDegradationKindCount; kind++ {
			index := int(kind)
			if classOccurrences[index] == 0 {
				continue
			}
			prefix := "degraded_" + kind.label()
			coverage.FieldSources[prefix+"_occurrences"] = strconv.FormatUint(classOccurrences[index], 10)
			coverage.FieldSources[prefix+"_affected_frames"] = strconv.FormatUint(classAffected[index], 10)
		}
		if reasonSamples.Used > 0 {
			coverage.FieldSources["degradation_reason_samples"] = reasonSamples.render()
			coverage.FieldSources["degradation_reason_sample_policy"] = "sha256_min_k8_domain_separated_prefix96_bounded_examples"
		}
		for issueIndex := 0; issueIndex < int(census.IssueCount); issueIndex++ {
			item := census.Issues[issueIndex]
			reason, labelOK := item.Issue.label(issueField)
			if !labelOK || item.Occurrences == 0 || item.AffectedFrames == 0 || item.AffectedFrames > item.Occurrences {
				return false
			}
			if traceDBSingleToken(reason) {
				coverage.FieldSources["degraded_"+reason+"_occurrences"] = strconv.FormatUint(item.Occurrences, 10)
				coverage.FieldSources["degraded_"+reason+"_affected_frames"] = strconv.FormatUint(item.AffectedFrames, 10)
			}
		}
		if reason, count, exact := profilerFtraceSingleExactReason(issueField, census); exact && traceDBSingleToken(reason) {
			coverage.FieldSources["degraded_"+reason+"_rows"] = strconv.FormatUint(count, 10)
		}
		out.profilerEventCoverage.Present[slotIndex] = true
		out.profilerEventCoverage.Index[slotIndex] = len(out.TraceCoverage)
		out.TraceCoverage = append(out.TraceCoverage, coverage)
	}
	return true
}

func profilerFtraceEventIssueSummaryCensus(field int, census profilerFtraceEventSlotCensus) (
	[profilerFtraceEventDegradationKindCount]uint64,
	[profilerFtraceEventDegradationKindCount]uint64,
	profilerStableSampleSet,
	bool,
) {
	var occurrences [profilerFtraceEventDegradationKindCount]uint64
	affected := census.ClassAffectedFrames
	var samples profilerStableSampleSet
	if int(census.IssueCount) > len(census.Issues) {
		return occurrences, affected, samples, false
	}
	for index := 0; index < int(census.IssueCount); index++ {
		item := census.Issues[index]
		class := item.Issue.sourceClass()
		label, ok := item.Issue.label(field)
		if !ok || class >= profilerFtraceEventDegradationKindCount || item.Occurrences == 0 ||
			item.AffectedFrames == 0 || item.AffectedFrames > item.Occurrences ||
			!checkedProfilerUint64AddTo(&occurrences[int(class)], item.Occurrences) {
			return occurrences, affected, samples, false
		}
		samples.observe("profiler-ftrace-event-reason:"+strconv.Itoa(profilerFtraceEventSlot(field)), []byte(label))
	}
	for class := range affected {
		if affected[class] > occurrences[class] {
			return occurrences, affected, samples, false
		}
	}
	return occurrences, affected, samples, true
}

func profilerFtraceEventDegradationSummary(field int, census profilerFtraceEventSlotCensus,
	occurrences [profilerFtraceEventDegradationKindCount]uint64, reasonSamples profilerStableSampleSet,
) string {
	parts := make([]string, 0, profilerFtraceEventDegradationKindCount+1)
	for kind := profilerFtraceEventDegradationKind(0); kind < profilerFtraceEventDegradationKindCount; kind++ {
		if count := occurrences[int(kind)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind.label(), count))
		}
	}
	if reason, count, exact := profilerFtraceSingleExactReason(field, census); exact {
		parts = append(parts, fmt.Sprintf("%s=%d", reason, count))
	} else if reasonSamples.Used > 0 {
		parts = append(parts, "reason_samples="+reasonSamples.render())
	}
	return strings.Join(parts, ",")
}

func profilerFtraceSingleExactReason(field int, census profilerFtraceEventSlotCensus) (string, uint64, bool) {
	if census.IssueCount != 1 || int(census.IssueCount) > len(census.Issues) {
		return "", 0, false
	}
	item := census.Issues[0]
	reason, ok := item.Issue.label(field)
	if !ok || item.Occurrences == 0 {
		return "", 0, false
	}
	return reason, item.Occurrences, true
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
