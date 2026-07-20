package tracequery

// rank_self_runnable_two_ruler.go — RULER2-1 (§29.150② user ruling, R-19-b,
// 2026-07-19): the cross-row TWO-RULER accounting record for the analysis
// target's own runnable seats.
//
// Background (§29.136 CHAIN-BUDGET, cb_rework P3③ 备案): the donghu17267
// flagship board's former single self runnable seat (5.604ms, mem=8) split
// under the CHAIN-BUDGET default tier into THREE published seats riding TWO
// different closed rulers — 3.956 [self_wall_clock] + 1.193 [self_wall_clock,
// the chain RootEvidence producer side] on the self wall-clock ruler and
// 1.648 [on_wakeup_chain, edge-anchored] on the wakeup-edge ruler. Each seat's
// value and caliber word were already honest ((a)/(b) answered); the missing
// (c) face is the CROSS-ROW disclosure sentence explaining the split. This
// file mints the typed record the display sentence consumes.
//
// M3 禁混尺 red line (SELF-ALL §29.61.2): the two rulers measure the SAME
// runnable account under different proof lanes — a cross-ruler sum is a
// mixed-ruler number and MUST NEVER be computed or published anywhere
// (Σ6.797 is banned; the record deliberately has no cross-ruler total field).
// Same-ruler subtotals are honest additive facts (3.956+1.193=5.149, µs
// identity by construction) and ride the record per ruler.
//
// Typed admission (PRECISE signals only, 宁漏勿假指 — any ambiguity mints
// nothing):
//   - published board scan (rank.Items), family = Type "runnable_wait" ∧
//     SubjectIsAnalysisTarget ∧ ChainRelevance "on_chain";
//   - every family seat must wear one of the two CLOSED ruler causality
//     tokens (self_wall_clock / on_wakeup_chain), publish a positive
//     effective value, carry a candidate rank ordinal, and share ONE thread —
//     any family seat breaking any condition silences the whole record;
//   - BOTH rulers occupied (≥1 seat each). A single-ruler multi-seat board
//     (the §29.136 single-ruler fold precedent,
//     TestSelfAllChainBudgetDefaultTierSingleRulerFold) mints nothing — the
//     existing same-ruler fold faces own that shape.
//
// The record is display wording input ONLY: no gate, score, sort, seat or
// value lane reads it (值通道/席位/排序零动 — 纯披露句批).

// SelfRunnableTwoRulerSeat is one published seat's (ordinal, value) pair —
// verbatim from the published board, never re-priced.
type SelfRunnableTwoRulerSeat struct {
	Rank  int     `json:"rank"`
	EffMs float64 `json:"eff_ms"`
}

// SelfRunnableTwoRulerAccounting is the result-level side-channel record: the
// target's own runnable seats split across the two closed rulers, with the
// per-ruler subtotals (same-ruler additive facts; NO cross-ruler total field
// exists by design — M3 禁混尺).
type SelfRunnableTwoRulerAccounting struct {
	Thread ThreadRef `json:"thread"`
	// WallSeats — the self wall-clock ruler's seats (causality
	// self_wall_clock), board order (rank asc). EdgeSeats — the wakeup-edge
	// ruler's seats (causality on_wakeup_chain), board order.
	WallSeats []SelfRunnableTwoRulerSeat `json:"wall_seats"`
	EdgeSeats []SelfRunnableTwoRulerSeat `json:"edge_seats"`
	// WallSubtotalMs / EdgeSubtotalMs — Σ of that ruler's seat values (µs
	// identity by construction; the display re-validates before rendering).
	// Same-ruler only: values on ONE ruler share a measure and may add.
	WallSubtotalMs float64 `json:"wall_subtotal_ms"`
	EdgeSubtotalMs float64 `json:"edge_subtotal_ms"`
	// LineStart / LineEnd — the lead (lowest-ordinal) seat's own trace-line
	// evidence range (the wire record's grounding span).
	LineStart int `json:"line_start,omitempty"`
	LineEnd   int `json:"line_end,omitempty"`
}

// rootCauseCausalityOnWakeupChain is the wakeup-edge ruler's causality token
// (the literal every edge-proven on-chain row publishes; see the enrich
// stamps in query.go / rank_family_fold.go).
const rootCauseCausalityOnWakeupChain = "on_wakeup_chain"

// harvestSelfRunnableTwoRulerAccounting builds the record from the PUBLISHED
// board (each seat's published value must be in place — the pool never
// speaks). nil = silent (single ruler, no family seat, or any 宁漏勿假指
// abort). Runs at both rank-pass tails (idempotent overwrite — one value
// source, publishedness reflects the final board).
func harvestSelfRunnableTwoRulerAccounting(published []RootCauseRankItem) *SelfRunnableTwoRulerAccounting {
	var record SelfRunnableTwoRulerAccounting
	seenThread := false
	for i := range published {
		item := &published[i]
		if item.Type != "runnable_wait" || !item.SubjectIsAnalysisTarget || item.ChainRelevance != "on_chain" {
			continue
		}
		// Family seat. Every admission condition below is load-bearing: one
		// broken seat silences the WHOLE record (the sentence claims a
		// complete per-ruler accounting; a silently skipped family seat
		// would make it lie by omission).
		eff := rootCauseEffectiveImpactMs(*item)
		if eff <= 0 || item.Rank <= 0 {
			return nil
		}
		if seenThread && item.Thread.PID != record.Thread.PID {
			return nil
		}
		seat := SelfRunnableTwoRulerSeat{Rank: item.Rank, EffMs: eff}
		switch item.Causality {
		case RootCauseCausalitySelfWallClock:
			record.WallSeats = insertSelfRunnableTwoRulerSeat(record.WallSeats, seat)
			record.WallSubtotalMs += eff
		case rootCauseCausalityOnWakeupChain:
			record.EdgeSeats = insertSelfRunnableTwoRulerSeat(record.EdgeSeats, seat)
			record.EdgeSubtotalMs += eff
		default:
			// A family seat outside the two closed rulers — the accounting
			// vocabulary cannot describe it honestly; the whole record stays
			// silent (closed set, absence never guesses).
			return nil
		}
		if !seenThread {
			record.Thread = item.Thread
			seenThread = true
		}
		// Lead seat = lowest candidate ordinal across both rulers; its line
		// span grounds the wire record.
		if lead := selfRunnableTwoRulerLeadRank(record); lead == item.Rank {
			record.LineStart, record.LineEnd = item.LineStart, item.LineEnd
		}
	}
	if len(record.WallSeats) == 0 || len(record.EdgeSeats) == 0 {
		return nil
	}
	return &record
}

// insertSelfRunnableTwoRulerSeat keeps a ruler's seat list ordered by rank
// asc (board order — the published ordinals are unique per candidate space).
func insertSelfRunnableTwoRulerSeat(seats []SelfRunnableTwoRulerSeat, seat SelfRunnableTwoRulerSeat) []SelfRunnableTwoRulerSeat {
	at := len(seats)
	for i := range seats {
		if seat.Rank < seats[i].Rank {
			at = i
			break
		}
	}
	seats = append(seats, SelfRunnableTwoRulerSeat{})
	copy(seats[at+1:], seats[at:])
	seats[at] = seat
	return seats
}

// selfRunnableTwoRulerLeadRank returns the lowest seat ordinal across both
// rulers (0 = no seat yet).
func selfRunnableTwoRulerLeadRank(record SelfRunnableTwoRulerAccounting) int {
	lead := 0
	for _, list := range [][]SelfRunnableTwoRulerSeat{record.WallSeats, record.EdgeSeats} {
		for _, seat := range list {
			if lead == 0 || seat.Rank < lead {
				lead = seat.Rank
			}
		}
	}
	return lead
}
