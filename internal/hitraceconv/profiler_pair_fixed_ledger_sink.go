package hitraceconv

// preflightProfilerPairFixedMutation proves every fixed-ledger branch that the
// pending delta/current row may take before the event's final Context poll.
// The actual commit may still choose a narrower branch after proof-budget and
// Block-clock decisions, but every checked integer assignment has already
// been shown representable; the post-poll tail therefore never returns an
// accounting error.
func (s *traceDBRowSink) preflightProfilerPairFixedMutation(
	row *renderedRow,
	delta *traceDBProfilerEventDelta,
) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
	}
	noPairRow := row == nil || row.pairKind == pairRenderUnknown
	noDelta := delta == nil || *delta == (traceDBProfilerEventDelta{})
	if noPairRow && noDelta {
		return nil
	}
	next := s.pairFixedLedger
	if delta != nil {
		for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
			if !delta.opaqueKinds[kind] {
				continue
			}
			plan, ok := next.planMarkOpaque(kind)
			if !ok {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
			}
			plan.apply(&next)
		}
		for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
			if delta.poisonKinds[kind] {
				plan, ok := next.planPoisonFamily(kind)
				if !ok {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
				}
				plan.apply(&next)
				continue
			}
			lane := delta.poisonLanes[kind]
			if lane == "" || next.families[kind].poisoned {
				continue
			}
			if !profilerPairBudgetKind(kind) {
				plan, ok := next.planPoisonFamily(kind)
				if !ok {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
				}
				plan.apply(&next)
				continue
			}
			laneState := profilerPairLaneState{}
			if id, found := s.pairLaneRegistries[kind].idFor(lane); found {
				state, stateOK := s.pairLaneRegistries[kind].state(id)
				if !stateOK {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_lane_missing"}
				}
				laneState = *state
			}
			plan, ok := next.planPoisonLane(kind, laneState)
			if !ok {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
			}
			plan.apply(&next, &laneState)
		}
	}

	if row == nil || row.pairKind == pairRenderUnknown {
		return nil
	}
	if !profilerPairKindValid(row.pairKind) {
		return &traceDBOutputInvariantError{Reason: "invalid_pair_render_kind"}
	}
	for _, lanePoisoned := range []bool{false, true} {
		if !profilerPairBudgetKind(row.pairKind) && lanePoisoned {
			continue
		}
		candidate := next
		plan, ok := candidate.planStageRow(
			row.pairKind, row.profilerEndpointSlot, row.structuredPair, lanePoisoned,
		)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
		plan.apply(&candidate)
	}
	if profilerPairBudgetKind(row.pairKind) && row.pairLane != "" && !next.families[row.pairKind].poisoned {
		laneState := profilerPairLaneState{}
		if id, found := s.pairLaneRegistries[row.pairKind].idFor(row.pairLane); found {
			state, stateOK := s.pairLaneRegistries[row.pairKind].state(id)
			if !stateOK {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_lane_missing"}
			}
			laneState = *state
		}
		structured := uint32(0)
		if row.structuredPair {
			structured = 1
		}
		if _, ok := laneState.stageEndpointRows(
			row.pairKind, row.profilerEndpointSlot, 1, structured,
		); !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_plan_invalid"}
		}
	}

	// Proof-budget exhaustion and Block authority failure can conservatively
	// close a whole family after the ordinary row path was selected. Prove that
	// alternate fixed branch too without mutating the real ledger.
	if profilerPairBudgetKind(row.pairKind) {
		candidate := next
		if row.pairKind == pairRenderBlock {
			plan, ok := candidate.planPoisonFamily(pairRenderBlock)
			if !ok {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
			}
			plan.apply(&candidate)
		} else {
			for _, kind := range []pairRenderKind{pairRenderMMC, pairRenderF2FS} {
				plan, ok := candidate.planPoisonFamily(kind)
				if !ok {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
				}
				plan.apply(&candidate)
			}
		}
		plan, ok := candidate.planStageRow(
			row.pairKind, row.profilerEndpointSlot, row.structuredPair, false,
		)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
		plan.apply(&candidate)
	}
	if profilerBlockAuthorityResetPossible(s, *row) {
		candidate := next
		for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
			plan, ok := candidate.planPoisonFamily(kind)
			if !ok {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
			}
			plan.apply(&candidate)
		}
		plan, ok := candidate.planStageRow(
			row.pairKind, row.profilerEndpointSlot, row.structuredPair, false,
		)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
		plan.apply(&candidate)
	} else if laneState, possible := profilerBlockLanePoisonPossible(s, *row); possible && !next.families[pairRenderBlock].poisoned {
		// The event delta commits before the Block clock. If it already poisons
		// this exact lane, the timestamp rollback below is an idempotent no-op;
		// mirror that bit while retaining the same checked lane account.
		if delta != nil && !delta.poisonKinds[pairRenderBlock] &&
			delta.poisonLanes[pairRenderBlock] == row.pairLane {
			laneState.poisoned = true
		}
		candidate := next
		plan, ok := candidate.planPoisonLane(pairRenderBlock, laneState)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
		plan.apply(&candidate, &laneState)
	}
	return nil
}

func profilerBlockAuthorityResetPossible(s *traceDBRowSink, row renderedRow) bool {
	if s == nil || row.pairKind != pairRenderBlock || row.pairLane == "" ||
		s.poisoned[pairRenderBlock] || s.pairAuthorityFailure != "" {
		return false
	}
	registry := &s.pairLaneRegistries[pairRenderBlock]
	id, found := registry.idFor(row.pairLane)
	if !found {
		_, corruptExistingKey := registry.byKey[row.pairLane]
		return corruptExistingKey
	}
	state, stateOK := registry.state(id)
	if !stateOK {
		return true
	}
	legacy, legacyFound := s.blockLaneClocks[row.pairLane]
	if legacyFound != state.blockClockSeen || legacyFound &&
		(legacy.seq != state.lastBlockSeq || legacy.tsNS != state.lastBlockTSNS) {
		return true
	}
	return state.blockClockSeen && row.seq <= state.lastBlockSeq
}

func profilerBlockLanePoisonPossible(
	s *traceDBRowSink,
	row renderedRow,
) (profilerPairLaneState, bool) {
	if s == nil || row.pairKind != pairRenderBlock || row.pairLane == "" ||
		s.poisoned[pairRenderBlock] || s.pairAuthorityFailure != "" {
		return profilerPairLaneState{}, false
	}
	registry := &s.pairLaneRegistries[pairRenderBlock]
	id, found := registry.idFor(row.pairLane)
	if !found {
		return profilerPairLaneState{}, false
	}
	state, stateOK := registry.state(id)
	if !stateOK {
		return profilerPairLaneState{}, false
	}
	legacy, legacyFound := s.blockLaneClocks[row.pairLane]
	if legacyFound != state.blockClockSeen || legacyFound &&
		(legacy.seq != state.lastBlockSeq || legacy.tsNS != state.lastBlockTSNS) {
		return profilerPairLaneState{}, false
	}
	if !state.blockClockSeen || row.seq <= state.lastBlockSeq || row.tsNS >= state.lastBlockTSNS {
		return profilerPairLaneState{}, false
	}
	return *state, true
}

// commitProfilerPairFixedRow is the no-error tail selected after delta,
// registry-budget and Block-clock decisions. It returns the final keyed-lane
// decision because an unexpected fixed-state breach source-wide fail-closes
// the capture and must not let legacy parity maps regrow.
func (s *traceDBRowSink) commitProfilerPairFixedRow(row renderedRow, trackLane bool) bool {
	if s == nil || row.pairKind == pairRenderUnknown {
		return trackLane
	}
	lanePoisoned := false
	var laneState *profilerPairLaneState
	var nextLane profilerPairLaneState
	if profilerPairBudgetKind(row.pairKind) && trackLane && row.pairLane != "" &&
		!s.poisoned[row.pairKind] && s.pairAuthorityFailure == "" {
		var ok bool
		laneState, ok = s.pairLaneRegistries[row.pairKind].state(row.profilerLaneID)
		if !ok {
			s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_lane_missing")
			trackLane = false
		} else {
			structured := uint32(0)
			if row.structuredPair {
				structured = 1
			}
			nextLane, ok = laneState.stageEndpointRows(
				row.pairKind, row.profilerEndpointSlot, 1, structured,
			)
			if !ok {
				s.failProfilerPairFixedLedger("profiler_pair_fixed_lane_commit_invalid")
				trackLane = false
			} else {
				lanePoisoned = laneState.poisoned
			}
		}
	}
	plan, ok := s.pairFixedLedger.planStageRow(
		row.pairKind, row.profilerEndpointSlot, row.structuredPair, lanePoisoned,
	)
	if !ok {
		s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_commit_invalid")
		return false
	}
	plan.apply(&s.pairFixedLedger)
	if laneState != nil && trackLane {
		*laneState = nextLane
	}
	return trackLane && !s.poisoned[row.pairKind] && s.pairAuthorityFailure == ""
}

func (s *traceDBRowSink) failProfilerPairFixedLedger(reason string) {
	if s == nil {
		return
	}
	if s.pairAuthorityFailure == "" {
		s.pairAuthorityFailure = reason
	}
	// A broken parity ledger cannot safely calculate a narrower terminal
	// verdict. Keep the malformed fixed state visible to validation, but close
	// every legacy publication lane and the whole output before any customer
	// byte can be opened.
	s.allRowsFailClosed = true
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		s.poisonPairKindLegacyRaw(kind)
	}
}

func (s *traceDBRowSink) validateProfilerPairFixedLedgerParity() error {
	if s == nil || !s.pairFixedLedger.valid() {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		withheld, err := s.withheldPairRowsForKindChecked(kind)
		if err != nil {
			return err
		}
		structuredWithheld, err := s.withheldStructuredPairRowsForKindChecked(kind)
		if err != nil {
			return err
		}
		wantFamily := profilerPairFixedFamilyLedger{
			profilerPairFixedCounts: profilerPairFixedCounts{
				staged: s.pairRows[kind], structured: s.structuredPairRows[kind],
				withheld: withheld, structuredWithheld: structuredWithheld,
			},
			poisoned: s.poisoned[kind], opaque: s.opaque[kind],
		}
		if got := s.pairFixedLedger.families[kind]; got != wantFamily {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_family_mismatch"}
		}
		if !profilerPairBudgetKind(kind) {
			continue
		}
		registry := &s.pairLaneRegistries[kind]
		for slot := profilerPairEndpointSlot(1); slot < profilerPairEndpointSlotCount; slot++ {
			descriptor, ok := slot.descriptor()
			if !ok || descriptor.kind != kind {
				continue
			}
			structured := 0
			structuredWithheld := 0
			if descriptor.structuredField != 0 {
				structured = s.structuredEventRows[kind][descriptor.structuredField]
				structuredWithheld, err = s.withheldStructuredPairRowsForEventFieldChecked(
					kind, descriptor.structuredField,
				)
				if err != nil {
					return err
				}
			}
			want := profilerPairFixedCounts{
				staged: s.pairTableTotals[kind][descriptor.name], structured: structured,
				withheld:           s.withheldPairRowsForTable(kind, descriptor.name),
				structuredWithheld: structuredWithheld,
			}
			if got := s.pairFixedLedger.endpoints[slot]; got != want {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_endpoint_mismatch"}
			}
		}
		if s.poisoned[kind] {
			continue
		}
		for index, lane := range registry.keys {
			state := registry.states[index]
			if !state.endpointCountsValid(kind) {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_state_invalid"}
			}
			rows, structured, ok := state.endpointTotals(kind)
			if !ok || int(rows) != s.pairLaneRows[kind][lane] ||
				int(structured) != s.structuredLaneRows[kind][lane] {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_total_mismatch"}
			}
			for ordinal := profilerPairFamilyEndpointOrdinal(0); ; ordinal++ {
				slot, slotOK := profilerPairEndpointForFamilyOrdinal(kind, ordinal)
				if !slotOK {
					break
				}
				descriptor, _ := slot.descriptor()
				counts, countsOK := state.endpointCountsFor(kind, slot)
				if !countsOK || int(counts.rows) != s.pairTableRows[kind][descriptor.name][lane] {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_endpoint_mismatch"}
				}
				wantStructured := 0
				if descriptor.structuredField != 0 {
					wantStructured = s.structuredEventLanes[kind][descriptor.structuredField][lane]
				}
				if int(counts.structuredRows) != wantStructured {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_structured_mismatch"}
				}
			}
		}
	}
	return nil
}
