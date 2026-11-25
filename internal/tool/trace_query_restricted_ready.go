package tool

// trace_query_restricted_ready.go — HMC377 round 16 (batch-owner rulings
// R-377-16 ③④, R-377-17 ②③⑥, 2026-11-24): THE decision of which rows hold the
// restricted ready instants of a CPU-restriction seat, read by every face that
// states it — the reader tree and detail blocks (runtimeTraceProjStampRestrictedHeld),
// the rank summary's preview rows (traceQueryRankRestrictedReadyFields) and,
// exported, the finalizer request (TraceRestrictedReadyHolds).
//
// The decision reads the typed fields an observation record of the seat
// carries and nothing else:
//   - the seat is a CPU-restriction seat (the engine's type token);
//   - it is an engine-absorbed member of a rank family (AbsorbedByRankFamily
//     with its AbsorbedInto key), or the engine typed its restricted ready
//     account as split between rows of the published board
//     (RestrictedReadyAccountMS and ShareCountedElsewhereMS);
//   - each resolved, non-represented anchored or unwalked claim of the seat is
//     one hold: its share is the restricted time inside the holders the claim
//     names (HolderEvidenceIDs), and an absorbed seat's hold sits on its
//     family's row, the row that wears the family key.
// The family key is compared with a row's RankFamilyKey as an opaque identity
// (never parsed). No thread name, no row value and no label decides a hold.
//
// Every face reads the same records: the reader and the finalizer read the
// projection compiled from the observation records; the rank summary is
// written before the records exist, so it reads the seat through the record's
// own encoders (traceQueryRankRestrictedReadySeat) and only the rank rows the
// record transport publishes (traceQueryRankRecordEmitted) — a seat or a
// holder whose record the transport cap drops is stated on no face.
//
// Where a face puts a hold is that face's own placement: the reader on the
// drawn row that draws the holder's record and publishes an account; the
// summary on the preview row of the holder's record.

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceRestrictedReadyHold is one hold of the decision: a part of a
// CPU-restriction seat's restricted ready time that lies inside the rows the
// engine names.
type TraceRestrictedReadyHold struct {
	// Seat is the CPU-restriction seat (its label, its thread and its
	// CPU-constraint judgment).
	Seat types.TraceCausalProjectionNode
	// FamilyKey is the absorbing family's key when Seat is an engine-absorbed
	// member: the hold sits on the row that wears this key (compared as an
	// opaque identity). "" for a seat whose account the engine split.
	FamilyKey string
	// HolderIDs are the observation record ids of the rows the claim names as
	// holders. For an absorbed seat they place the hold where no row wears the
	// family key.
	HolderIDs []string
	// MS is the restricted ready time inside the holders: the claim's share.
	MS float64
	// AccountMS is the seat's typed restricted ready account (0 when the
	// engine typed none). The account is split when it exceeds the time a
	// row states (TraceRestrictedReadySplit).
	AccountMS float64
	// OwnAccount: case A' — the engine's chain-account claim stands beside
	// this walked part; the holder keeps its own diverging account, so the
	// share lies inside the holder's time and is no part of its value.
	OwnAccount bool
	// seat is Seat's position in the roster the decision read.
	seat int
}

// TraceRestrictedReadyHolds is the decision over a compiled projection: the
// absorbed rank rows (AbsorbedChainRows) and the complete seat roster
// (RankedSeats: a seat the context cap folded keeps its typed claims there),
// in that order.
func TraceRestrictedReadyHolds(projection types.TraceCausalProjection) []TraceRestrictedReadyHold {
	seats := make([]types.TraceCausalProjectionNode, 0, len(projection.AbsorbedChainRows)+len(projection.RankedSeats))
	seats = append(seats, projection.AbsorbedChainRows...)
	seats = append(seats, projection.RankedSeats...)
	return traceRestrictedReadyDecide(seats)
}

// traceRestrictedReadyDecide is the decision itself (see the file header).
// A seat met twice in the roster decides once.
func traceRestrictedReadyDecide(seats []types.TraceCausalProjectionNode) []TraceRestrictedReadyHold {
	var out []TraceRestrictedReadyHold
	seen := map[string]bool{}
	for i, seat := range seats {
		if !runtimeTraceProjRestrictionSeatNode(seat) {
			continue
		}
		key := runtimeTraceCausalProjectionNodeKey(seat)
		if seen[key] {
			continue
		}
		seen[key] = true
		family := ""
		if seat.AbsorbedByRankFamily {
			family = strings.TrimSpace(seat.AbsorbedInto)
		}
		if family == "" && (seat.RestrictedReadyAccountMS <= 0 || seat.ShareCountedElsewhereMS <= 0) {
			continue
		}
		chainAccount := false
		for _, c := range seat.ShareClaims {
			chainAccount = chainAccount || c.Part == string(tracequery.RootCauseShareClaimChainAccount)
		}
		for _, c := range seat.ShareClaims {
			if !runtimeTraceProjRestrictedPartClaim(c.Part, c.Resolved, c.Represented, c.ShareMS) {
				continue
			}
			var holders []string
			for _, id := range c.HolderEvidenceIDs {
				if id = strings.TrimSpace(id); id != "" {
					holders = append(holders, id)
				}
			}
			// A split seat's part is placed only by the holders its claim
			// names; an absorbed seat's part sits on its family's row.
			if family == "" && len(holders) == 0 {
				continue
			}
			out = append(out, TraceRestrictedReadyHold{Seat: seat, FamilyKey: family, HolderIDs: holders, MS: c.ShareMS,
				AccountMS: seat.RestrictedReadyAccountMS, OwnAccount: runtimeTraceProjRestrictedPartOwnAccount(chainAccount, c.Part, c.Role), seat: i})
		}
	}
	return out
}

// TraceRestrictedReadySplit reports whether a seat's typed restricted ready
// account is split around the time a row states (R-377-17 ①②): the account
// exceeds it at the precision every face prints. A face then states the
// account beside the time; an unsplit account is the time itself.
func TraceRestrictedReadySplit(accountMS, statedMS float64) bool {
	return math.Round(accountMS*1000) > math.Round(statedMS*1000)
}

// TraceRestrictedReadyThread is the typed identity of a seat's thread on its
// board (R-377-17 ⑤: a thread's CPU-constraint description is stated once per
// face): the member TID the record carries and the board identity. A seat
// without a typed TID is its own identity.
func TraceRestrictedReadyThread(node types.TraceCausalProjectionNode) string {
	if s := node.RankSubjects; s != nil && s.Member.TID > 0 {
		return fmt.Sprintf("tid:%d|board:%s|%s", s.Member.TID, strings.TrimSpace(node.RankBoardTarget), strings.TrimSpace(node.RankBoardParamsFingerprint))
	}
	return "seat:" + runtimeTraceCausalProjectionNodeKey(node)
}

// traceQueryRankRestrictedReadyScope is the record scope the summary's reading
// of the rank rows names records by (its ids never leave this file).
const traceQueryRankRestrictedReadyScope = "rank_preview"

// traceQueryRankRestrictedReadySeat reads a rank row of the board as the
// projection compile reads its observation record, through the record's own
// encoders: type token, absorbed family, the restriction subset's typed
// account fields and the share claims with their holders named by the record
// ids of their board positions.
func traceQueryRankRestrictedReadySeat(item tracequery.RootCauseRankItem, pos int, positions map[string]int) types.TraceCausalProjectionNode {
	id := traceQueryRankRecordID(traceQueryRankRestrictedReadyScope, pos)
	account, elsewhere, _ := traceQueryRestrictedSubsetNoteValues(item)
	parse := func(v string) float64 {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f <= 0 {
			return 0
		}
		return f
	}
	typ := strings.TrimSpace(item.Type)
	return types.TraceCausalProjectionNode{EvidenceID: id, TypeToken: typ, Object: typ,
		AbsorbedByRankFamily:     traceQueryTypedBool(item.AbsorbedByRankFamily) == "true",
		AbsorbedInto:             strings.TrimSpace(item.AbsorbedIntoFamily),
		RestrictedReadyAccountMS: parse(account),
		ShareCountedElsewhereMS:  parse(elsewhere),
		ShareClaims:              types.ParseTraceShareClaimsNote(types.EncodeTraceShareClaimsNote(traceQueryShareClaimNoteEntries(item, positions)), id),
		RankSubjects:             &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: item.Thread.PID, TGID: item.Thread.TGID, Comm: item.Thread.Comm}},
	}
}

// traceQueryRankRestrictedReadyHolds is the decision on a rank board, read
// through the records the transport publishes (Items then AbsorbedItems, the
// record positions).
func traceQueryRankRestrictedReadyHolds(rank *tracequery.RootCauseRankResult) []TraceRestrictedReadyHold {
	if rank == nil {
		return nil
	}
	rows := make([]tracequery.RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	rows = append(rows, rank.Items...)
	rows = append(rows, rank.AbsorbedItems...)
	positions := traceQueryShareHolderPositions(rows)
	var seats []types.TraceCausalProjectionNode
	for i, item := range rows {
		if traceQueryRankRecordEmitted(i, item) {
			seats = append(seats, traceQueryRankRestrictedReadySeat(item, i+1, positions))
		}
	}
	return traceRestrictedReadyDecide(seats)
}

// traceQueryCPUConstraintKVs is the one list of a rank item's typed
// CPU-constraint judgment as note pairs (empty values are dropped by the
// writer): the item's observation record carries it, and the rank preview row
// that holds the seat's time prints it (R-377-16 ④).
func traceQueryCPUConstraintKVs(item tracequery.RootCauseRankItem) [][2]string {
	return [][2]string{
		{types.TraceNoteKeyCPUConstraintKind, sanitizeForBanner(item.CPUConstraintKind)},
		{types.TraceNoteKeyCPUConstraintCPUSet, sanitizeForBanner(item.CPUConstraintCPUSet)},
		{types.TraceNoteKeyCPUConstraintCPUSetIsBinding, traceQueryTypedBool(item.CPUConstraintCPUSetIsBinding)},
		{types.TraceNoteKeyCPUConstraintPolicy, sanitizeForBanner(item.CPUConstraintPolicy)},
		{types.TraceNoteKeyCPUConstraintAllowedCPUs, traceIntList(item.CPUConstraintAllowedCPUs)},
		{types.TraceNoteKeyCPUConstraintExcludedCPUs, traceIntList(item.CPUConstraintExcludedCPUs)},
		{types.TraceNoteKeyCPUConstraintAllowedMaxTierKHz, traceQueryTypedInt64(item.CPUConstraintAllowedMaxTierKHz)},
		{types.TraceNoteKeyCPUConstraintGlobalMaxTierKHz, traceQueryTypedInt64(item.CPUConstraintGlobalMaxTierKHz)},
	}
}

// traceQueryRankRestrictedReadyFields is the rank summary's carrier of the
// decision (R-377-16 ④, R-377-17 ②): the preview row of a holder ends with
// the restricted ready time inside it under one key on both arms; where the
// seat's typed account is split around that time, the account follows under
// the key the seat's own record carries it by; on the first holding row of
// the thread (board order) the seat's typed constraint judgment follows, once
// per thread and summary. This holds whether or not the reader face draws the
// seat as a row of its own. The verbatim policy string stays on the seat's
// record: its inner key=value tokens would read as fields of the holding row.
//
// Placement on the summary: an absorbed seat's hold on the preview row that
// wears its family key (the first in board order), or, where no board row
// wears it, on its claim's holder; a split seat's hold on the preview row of
// the one holder its claim names. A claim that names several rows (the engine
// types no split between them) and a row that would hold the time of two
// seats state nothing here: the key speaks of this row alone, and the seats'
// records carry the relation. Keyed by the row's index in rank.Items (only
// rows below visible); every other row gets "".
func traceQueryRankRestrictedReadyFields(rank *tracequery.RootCauseRankResult, visible int) map[int]string {
	holds := traceQueryRankRestrictedReadyHolds(rank)
	if len(holds) == 0 {
		return nil
	}
	rowOf := map[string]int{}
	familyRow := map[string]int{}
	for i, item := range rank.Items {
		if i < visible {
			rowOf[traceQueryRankRecordID(traceQueryRankRestrictedReadyScope, i+1)] = i
		}
		key := strings.TrimSpace(item.RankFamilyKey)
		if _, seen := familyRow[key]; !seen {
			familyRow[key] = i
		}
	}
	type rowHold struct {
		seat int
		ms   float64
		hold TraceRestrictedReadyHold
		two  bool
	}
	byRow := map[int]*rowHold{}
	for _, h := range holds {
		at := -1
		if row, ok := familyRow[h.FamilyKey]; h.FamilyKey != "" && ok {
			if row < visible {
				at = row
			}
		} else {
			for _, id := range h.HolderIDs {
				row, ok := rowOf[id]
				if !ok || (at >= 0 && row != at) {
					at = -2
					break
				}
				at = row
			}
		}
		if at < 0 {
			continue
		}
		switch got := byRow[at]; {
		case got == nil:
			byRow[at] = &rowHold{seat: h.seat, ms: h.MS, hold: h}
		case got.seat == h.seat:
			// Two parts of one seat inside one row: disjoint instants of one
			// account.
			got.ms += h.MS
		default:
			got.two = true
		}
	}
	rows := make([]tracequery.RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	rows = append(rows, rank.Items...)
	rows = append(rows, rank.AbsorbedItems...)
	seatItem := map[string]tracequery.RootCauseRankItem{}
	for i, item := range rows {
		seatItem[traceQueryRankRecordID(traceQueryRankRestrictedReadyScope, i+1)] = item
	}
	var out map[int]string
	described := map[string]bool{}
	for at := range rank.Items {
		got := byRow[at]
		if got == nil || got.two {
			continue
		}
		var b strings.Builder
		b.WriteString(traceQueryRankRestrictedReadyKey + fmt.Sprintf("%.3fms", got.ms))
		if TraceRestrictedReadySplit(got.hold.AccountMS, got.ms) {
			b.WriteString(" " + types.TraceNoteKeyRestrictedReadyAccount + "=" + traceQueryObservationMSValue(got.hold.AccountMS))
		}
		if thread := TraceRestrictedReadyThread(got.hold.Seat); !described[thread] {
			described[thread] = true
			for _, kv := range traceQueryCPUConstraintKVs(seatItem[strings.TrimSpace(got.hold.Seat.EvidenceID)]) {
				if kv[0] == types.TraceNoteKeyCPUConstraintPolicy || strings.TrimSpace(kv[1]) == "" {
					continue
				}
				b.WriteString(" " + kv[0] + "=" + kv[1])
			}
		}
		if out == nil {
			out = map[int]string{}
		}
		out[at] = b.String()
	}
	return out
}
