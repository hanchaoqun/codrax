package hitraceconv

import "math"

// profilerTerminalPublicationCounts is the fixed terminal verdict cell shared
// by every projection below. It is deliberately comparable: the sidecar is
// revalidated several times, and each pass must reproduce the exact same
// ledger without mutating retained sink state.
type profilerTerminalPublicationCounts struct {
	staged    uint64
	published uint64
	withheld  uint64
}

func (counts *profilerTerminalPublicationCounts) observe(disposition profilerSourceOrderDisposition) bool {
	if counts == nil || !disposition.valid() || counts.staged == math.MaxUint64 {
		return false
	}
	counts.staged++
	if disposition.publishable() {
		if counts.published == math.MaxUint64 {
			return false
		}
		counts.published++
		return true
	}
	if counts.withheld == math.MaxUint64 {
		return false
	}
	counts.withheld++
	return true
}

func (counts profilerTerminalPublicationCounts) valid() bool {
	return counts.published <= counts.staged &&
		counts.withheld == counts.staged-counts.published
}

func addProfilerTerminalPublicationCounts(
	target *profilerTerminalPublicationCounts,
	delta profilerTerminalPublicationCounts,
) bool {
	if target == nil || !delta.valid() ||
		!checkedProfilerUint64AddTo(&target.staged, delta.staged) ||
		!checkedProfilerUint64AddTo(&target.published, delta.published) ||
		!checkedProfilerUint64AddTo(&target.withheld, delta.withheld) {
		return false
	}
	return target.valid()
}

type profilerTerminalTextMessageLedger struct {
	staged        uint64
	published     uint64
	fullyWithheld uint64
	pairBearing   uint64
}

func (messages profilerTerminalTextMessageLedger) valid() bool {
	return messages.published <= messages.staged &&
		messages.fullyWithheld == messages.staged-messages.published &&
		messages.pairBearing <= messages.staged
}

// profilerTerminalPublicationLedger is a closed, fixed-cardinality projection
// of authenticated sidecar rows in source ingest order. It intentionally does
// not retain frame or message identities.
type profilerTerminalPublicationLedger struct {
	rows                   profilerTerminalPublicationCounts
	textRows               profilerTerminalPublicationCounts
	structuredRows         profilerTerminalPublicationCounts
	sourceNeutralRows      profilerTerminalPublicationCounts
	pairFamilies           [pairRenderKindCount]profilerTerminalPublicationCounts
	structuredPairFamilies [pairRenderKindCount]profilerTerminalPublicationCounts
	endpoints              [profilerPairEndpointSlotCount]profilerTerminalPublicationCounts
	structuredEndpoints    [profilerPairEndpointSlotCount]profilerTerminalPublicationCounts
	publishers             [profilerPairPublisherSlotCount]profilerTerminalPublicationCounts
	publisherFamilies      [profilerPairPublisherSlotCount][pairRenderKindCount]profilerTerminalPublicationCounts
	textMessages           profilerTerminalTextMessageLedger
}

type profilerTerminalPublicationBuilder struct {
	ledger                 profilerTerminalPublicationLedger
	activeMessage          uint32
	activeMessagePublisher profilerPairPublisherSlot
	activeMessagePublished bool
	activeMessagePair      bool
}

func (builder *profilerTerminalPublicationBuilder) finishMessage() bool {
	if builder == nil || builder.activeMessage == 0 {
		return builder != nil
	}
	messages := &builder.ledger.textMessages
	if messages.staged == math.MaxUint64 {
		return false
	}
	messages.staged++
	if builder.activeMessagePublished {
		if messages.published == math.MaxUint64 {
			return false
		}
		messages.published++
	} else {
		if messages.fullyWithheld == math.MaxUint64 {
			return false
		}
		messages.fullyWithheld++
	}
	if builder.activeMessagePair {
		if messages.pairBearing == math.MaxUint64 {
			return false
		}
		messages.pairBearing++
	}
	builder.activeMessage = 0
	builder.activeMessagePublisher = profilerPairPublisherNone
	builder.activeMessagePublished = false
	builder.activeMessagePair = false
	return messages.valid()
}

func (builder *profilerTerminalPublicationBuilder) beginOrContinueMessage(
	provenance profilerPairRowProvenance,
) bool {
	if builder == nil || provenance.TextMessageOrdinal == 0 ||
		!provenance.PublisherSlot.textCapable() {
		return false
	}
	if builder.activeMessage != 0 && provenance.TextMessageOrdinal != builder.activeMessage {
		if !builder.finishMessage() {
			return false
		}
	}
	if builder.activeMessage == 0 {
		expected := builder.ledger.textMessages.staged + 1
		if expected > math.MaxUint32 || uint64(provenance.TextMessageOrdinal) != expected {
			return false
		}
		builder.activeMessage = provenance.TextMessageOrdinal
		builder.activeMessagePublisher = provenance.PublisherSlot
	} else if builder.activeMessagePublisher != provenance.PublisherSlot {
		return false
	}
	return true
}

func (builder *profilerTerminalPublicationBuilder) observe(
	provenance profilerPairRowProvenance,
	disposition profilerSourceOrderDisposition,
) bool {
	if builder == nil || !provenance.storageValid() || !disposition.valid() ||
		provenance.PublisherSlot == profilerPairPublisherNoncanonicalFtrace {
		return false
	}
	if provenance.TextMessageOrdinal == 0 {
		if !builder.finishMessage() {
			return false
		}
	} else if !builder.beginOrContinueMessage(provenance) {
		return false
	}
	if !builder.ledger.rows.observe(disposition) ||
		!builder.ledger.publishers[provenance.PublisherSlot].observe(disposition) {
		return false
	}
	switch {
	case provenance.TextMessageOrdinal != 0:
		if !builder.ledger.textRows.observe(disposition) {
			return false
		}
		if disposition.publishable() {
			builder.activeMessagePublished = true
		}
	case provenance.PublisherSlot == profilerPairPublisherSession:
		if !builder.ledger.textRows.observe(disposition) {
			return false
		}
	case provenance.PublisherSlot == profilerPairPublisherExactFtrace:
		if !builder.ledger.structuredRows.observe(disposition) {
			return false
		}
	case provenance.PublisherSlot == profilerPairPublisherNone:
		// Low-level authenticated-sorter fixtures are source-neutral. Production
		// Profiler container parity rejects this class; retaining it here keeps the
		// sidecar proof usable as an independently testable primitive.
		if !builder.ledger.sourceNeutralRows.observe(disposition) {
			return false
		}
	default:
		return false
	}
	if provenance.PairKind == pairRenderUnknown {
		return true
	}
	if !profilerPairBudgetKind(provenance.PairKind) {
		return provenance.PublisherSlot == profilerPairPublisherNone
	}
	if provenance.EndpointSlot == profilerPairEndpointNone {
		return false
	}
	if !builder.ledger.pairFamilies[provenance.PairKind].observe(disposition) ||
		!builder.ledger.endpoints[provenance.EndpointSlot].observe(disposition) ||
		!builder.ledger.publisherFamilies[provenance.PublisherSlot][provenance.PairKind].observe(disposition) {
		return false
	}
	if provenance.TextMessageOrdinal != 0 {
		builder.activeMessagePair = true
	}
	if provenance.Flags&profilerPairRowProvenanceStructured != 0 {
		if (provenance.PublisherSlot != profilerPairPublisherExactFtrace &&
			provenance.PublisherSlot != profilerPairPublisherNone) ||
			provenance.TextMessageOrdinal != 0 ||
			!builder.ledger.structuredPairFamilies[provenance.PairKind].observe(disposition) ||
			!builder.ledger.structuredEndpoints[provenance.EndpointSlot].observe(disposition) {
			return false
		}
	}
	return true
}

func (builder *profilerTerminalPublicationBuilder) finish() (profilerTerminalPublicationLedger, bool) {
	if builder == nil || !builder.finishMessage() || !builder.ledger.textMessages.valid() {
		return profilerTerminalPublicationLedger{}, false
	}
	return builder.ledger, true
}

func profilerTerminalCountsMatchFixed(
	counts profilerTerminalPublicationCounts,
	staged int,
	published int,
	withheld int,
) bool {
	return staged >= 0 && published >= 0 && withheld >= 0 &&
		counts.staged == uint64(staged) && counts.published == uint64(published) &&
		counts.withheld == uint64(withheld) && counts.valid()
}

func (s *traceDBRowSink) validateProfilerTerminalPublicationLedger(
	ledger profilerTerminalPublicationLedger,
) error {
	fail := func(reason string) error {
		return &traceDBOutputInvariantError{Reason: reason}
	}
	if s == nil || s.captureLifecycle == profilerCaptureInactive || s.stats.RowsAccepted < 0 ||
		!ledger.rows.valid() || !ledger.textRows.valid() || !ledger.structuredRows.valid() ||
		!ledger.sourceNeutralRows.valid() ||
		!ledger.textMessages.valid() || ledger.rows.staged != uint64(s.stats.RowsAccepted) ||
		ledger.textMessages.staged != uint64(s.nextTextMessage) {
		return fail("profiler_terminal_publication_ledger_global_invalid")
	}
	var rowClasses profilerTerminalPublicationCounts
	if !addProfilerTerminalPublicationCounts(&rowClasses, ledger.textRows) ||
		!addProfilerTerminalPublicationCounts(&rowClasses, ledger.structuredRows) ||
		!addProfilerTerminalPublicationCounts(&rowClasses, ledger.sourceNeutralRows) ||
		rowClasses != ledger.rows {
		return fail("profiler_terminal_publication_ledger_row_class_mismatch")
	}
	if ledger.publishers[profilerPairPublisherNoncanonicalFtrace] != (profilerTerminalPublicationCounts{}) {
		return fail("profiler_terminal_publication_ledger_publisher_invalid")
	}
	var publisherRows profilerTerminalPublicationCounts
	for publisher := profilerPairPublisherSlot(0); publisher < profilerPairPublisherSlotCount; publisher++ {
		if publisher == profilerPairPublisherNoncanonicalFtrace {
			continue
		}
		counts := ledger.publishers[publisher]
		if !counts.valid() || !addProfilerTerminalPublicationCounts(&publisherRows, counts) {
			return fail("profiler_terminal_publication_ledger_publisher_invalid")
		}
	}
	if publisherRows != ledger.rows {
		return fail("profiler_terminal_publication_ledger_publisher_mismatch")
	}
	for kind := pairRenderKind(0); kind < pairRenderKindCount; kind++ {
		familyCounts := ledger.pairFamilies[kind]
		structuredCounts := ledger.structuredPairFamilies[kind]
		if !familyCounts.valid() || !structuredCounts.valid() {
			return fail("profiler_terminal_publication_ledger_family_invalid")
		}
		if !profilerPairBudgetKind(kind) {
			if familyCounts != (profilerTerminalPublicationCounts{}) ||
				structuredCounts != (profilerTerminalPublicationCounts{}) {
				return fail("profiler_terminal_publication_ledger_nonbudget_family")
			}
			for publisher := profilerPairPublisherSlot(0); publisher < profilerPairPublisherSlotCount; publisher++ {
				if ledger.publisherFamilies[publisher][kind] != (profilerTerminalPublicationCounts{}) {
					return fail("profiler_terminal_publication_ledger_nonbudget_family")
				}
			}
			continue
		}
		if ledger.publisherFamilies[profilerPairPublisherNoncanonicalFtrace][kind] !=
			(profilerTerminalPublicationCounts{}) {
			return fail("profiler_terminal_publication_ledger_publisher_family_invalid")
		}
		fixed, ok := s.pairFixedLedger.family(kind)
		if !ok || !profilerTerminalCountsMatchFixed(
			familyCounts, fixed.staged, fixed.staged-fixed.withheld, fixed.withheld,
		) || !profilerTerminalCountsMatchFixed(
			structuredCounts, fixed.structured,
			fixed.structured-fixed.structuredWithheld, fixed.structuredWithheld,
		) {
			return fail("profiler_terminal_publication_ledger_family_mismatch")
		}
		var publisherFamily profilerTerminalPublicationCounts
		for publisher := profilerPairPublisherSlot(0); publisher < profilerPairPublisherSlotCount; publisher++ {
			if publisher == profilerPairPublisherNoncanonicalFtrace {
				continue
			}
			counts := ledger.publisherFamilies[publisher][kind]
			if !counts.valid() || !addProfilerTerminalPublicationCounts(&publisherFamily, counts) {
				return fail("profiler_terminal_publication_ledger_publisher_family_invalid")
			}
		}
		if publisherFamily != familyCounts {
			return fail("profiler_terminal_publication_ledger_publisher_family_mismatch")
		}
	}
	if ledger.endpoints[profilerPairEndpointNone] != (profilerTerminalPublicationCounts{}) ||
		ledger.structuredEndpoints[profilerPairEndpointNone] != (profilerTerminalPublicationCounts{}) {
		return fail("profiler_terminal_publication_ledger_endpoint_invalid")
	}
	var endpointFamilies [pairRenderKindCount]profilerTerminalPublicationCounts
	var structuredEndpointFamilies [pairRenderKindCount]profilerTerminalPublicationCounts
	for slot := profilerPairEndpointSlot(1); slot < profilerPairEndpointSlotCount; slot++ {
		descriptor, ok := slot.descriptor()
		counts := ledger.endpoints[slot]
		structuredCounts := ledger.structuredEndpoints[slot]
		fixed, fixedOK := s.pairFixedLedger.endpoint(slot)
		if !ok || !fixedOK || !counts.valid() || !structuredCounts.valid() ||
			!profilerTerminalCountsMatchFixed(
				counts, fixed.staged, fixed.staged-fixed.withheld, fixed.withheld,
			) || !profilerTerminalCountsMatchFixed(
			structuredCounts, fixed.structured,
			fixed.structured-fixed.structuredWithheld, fixed.structuredWithheld,
		) || !addProfilerTerminalPublicationCounts(&endpointFamilies[descriptor.kind], counts) ||
			!addProfilerTerminalPublicationCounts(&structuredEndpointFamilies[descriptor.kind], structuredCounts) {
			return fail("profiler_terminal_publication_ledger_endpoint_mismatch")
		}
	}
	for _, kind := range profilerCaptureKinds {
		if endpointFamilies[kind] != ledger.pairFamilies[kind] ||
			structuredEndpointFamilies[kind] != ledger.structuredPairFamilies[kind] {
			return fail("profiler_terminal_publication_ledger_endpoint_family_mismatch")
		}
	}
	if s.allRowsFailClosed && ledger.rows.published != 0 {
		return fail("profiler_terminal_publication_ledger_source_fail_close_mismatch")
	}
	return nil
}
