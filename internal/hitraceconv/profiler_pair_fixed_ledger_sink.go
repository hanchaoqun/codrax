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
		_, ok := next.planStageRow(
			row.pairKind, row.profilerEndpointSlot, row.structuredPair, lanePoisoned,
		)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
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
	if profilerPairBudgetFailurePossible(s, *row, &next) {
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
		_, ok := next.planPoisonLane(pairRenderBlock, laneState)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_plan_invalid"}
		}
	}
	return nil
}

func profilerPairBudgetFailurePossible(
	s *traceDBRowSink,
	row renderedRow,
	next *profilerPairFixedLedger,
) bool {
	if s == nil || next == nil || !profilerPairBudgetKind(row.pairKind) ||
		next.families[row.pairKind].poisoned || s.pairAuthorityFailure != "" {
		return false
	}
	domain := s.profilerPairProofDomain(row.pairKind)
	if domain == nil || domain.failureReason != "" {
		return false
	}
	if domain.observations >= domain.maxObservations {
		return true
	}
	if row.pairLane == "" {
		return false
	}
	if _, found := s.pairLaneRegistries[row.pairKind].idFor(row.pairLane); found {
		return false
	}
	return domain.laneKeys >= domain.maxLaneKeys
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
	if !state.blockClockSeen && (state.lastBlockSeq != 0 || state.lastBlockTSNS != 0) {
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
	if !state.blockClockSeen && (state.lastBlockSeq != 0 || state.lastBlockTSNS != 0) {
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
// the capture and must not let a reset exact-lane registry regrow.
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
		family := s.pairFixedLedger.families[kind]
		if family.staged != s.pairRows[kind] || family.structured != s.structuredPairRows[kind] ||
			family.poisoned != s.poisoned[kind] || family.opaque != s.opaque[kind] {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_family_mismatch"}
		}
	}
	return nil
}
