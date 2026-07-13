package hitraceconv

import (
	"context"
	"strconv"
)

func (delta *traceDBProfilerEventDelta) markOpaque(kind pairRenderKind) {
	if delta == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	delta.opaqueKinds[kind] = true
}

func (delta *traceDBProfilerEventDelta) poisonKind(kind pairRenderKind) {
	if delta == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	delta.poisonKinds[kind] = true
	delta.poisonLanes[kind] = ""
}

func (delta *traceDBProfilerEventDelta) poisonAdmission(admission profilerPairAdmission) {
	if delta == nil || !admission.Governed || admission.Kind == pairRenderUnknown ||
		!profilerPairKindValid(admission.Kind) {
		return
	}
	if (admission.Kind == pairRenderF2FS || admission.Kind == pairRenderBlock) &&
		admission.LaneKnown && admission.Lane != "" {
		if !delta.poisonKinds[admission.Kind] {
			delta.poisonLanes[admission.Kind] = admission.Lane
		}
		return
	}
	delta.poisonKind(admission.Kind)
}

// profilerFtraceEventBatchDelta copies exactly one fixed event slot plus the
// K8 unknown-field sample set. Copying the complete batch for every event
// would turn a bounded transaction into prohibitive memory bandwidth on
// million-event frames.
type profilerFtraceEventBatchDelta struct {
	batch          *profilerFtraceEventBatchCensus
	slotIndex      int
	nextSlot       profilerFtraceEventSlotCensus
	nextUnknown    profilerStableSampleSet
	unknownChanged bool
}

func stageProfilerFtraceEventBatchDeltaContext(ctx context.Context, batch *profilerFtraceEventBatchCensus,
	field int, publishable bool, issues []profilerFtraceEventIssue, emitted bool,
) (profilerFtraceEventBatchDelta, error) {
	var delta profilerFtraceEventBatchDelta
	if batch == nil {
		return delta, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return delta, err
	}
	if batch.Overflow {
		return delta, &traceDBOutputInvariantError{Reason: "profiler_event_batch_counter_overflow"}
	}
	slotIndex := profilerFtraceEventSlot(field)
	nextSlot := batch.Slots[slotIndex]
	if !checkedProfilerUint64AddTo(&nextSlot.RowsRead, 1) {
		return delta, &traceDBOutputInvariantError{Reason: "profiler_event_batch_counter_overflow"}
	}
	nextUnknown := batch.UnknownFieldSamples
	unknownChanged := slotIndex == profilerFtraceUnknownEventSlot
	if unknownChanged {
		if err := nextUnknown.observeContext(ctx, "profiler-ftrace-unknown-event-field", []byte(strconv.Itoa(field))); err != nil {
			return delta, err
		}
	}
	if !profilerFtraceEventIssueVerdictValid(field, publishable, issues) {
		return delta, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
	}
	var observedClass [profilerFtraceEventDegradationKindCount]bool
	for _, issue := range issues {
		class := issue.sourceClass()
		if class >= profilerFtraceEventDegradationKindCount {
			return delta, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
		}
		observedClass[int(class)] = true
		index, found := profilerFtraceEventIssueCensusIndex(&nextSlot, issue)
		if !found {
			if int(nextSlot.IssueCount) >= len(nextSlot.Issues) {
				return delta, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
			}
			used := int(nextSlot.IssueCount)
			copy(nextSlot.Issues[index+1:used+1], nextSlot.Issues[index:used])
			nextSlot.Issues[index] = profilerFtraceEventIssueCensus{Issue: issue}
			nextSlot.IssueCount++
		}
		census := &nextSlot.Issues[index]
		firstForFrame := census.Occurrences == 0
		if !checkedProfilerUint64AddTo(&census.Occurrences, 1) ||
			firstForFrame && !checkedProfilerUint64AddTo(&census.AffectedFrames, 1) {
			return delta, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
		}
	}
	for class, observed := range observedClass {
		if observed && nextSlot.ClassAffectedFrames[class] == 0 &&
			!checkedProfilerUint64AddTo(&nextSlot.ClassAffectedFrames[class], 1) {
			return delta, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
		}
	}
	if emitted && !checkedProfilerUint64AddTo(&nextSlot.RowsEmitted, 1) {
		return delta, &traceDBOutputInvariantError{Reason: "profiler_event_batch_emitted_counter_overflow"}
	}
	if err := ctx.Err(); err != nil {
		return delta, err
	}
	return profilerFtraceEventBatchDelta{
		batch: batch, slotIndex: slotIndex, nextSlot: nextSlot,
		nextUnknown: nextUnknown, unknownChanged: unknownChanged,
	}, nil
}

func (delta profilerFtraceEventBatchDelta) commit() {
	if delta.batch == nil {
		return
	}
	delta.batch.Slots[delta.slotIndex] = delta.nextSlot
	if delta.unknownChanged {
		delta.batch.UnknownFieldSamples = delta.nextUnknown
	}
}
