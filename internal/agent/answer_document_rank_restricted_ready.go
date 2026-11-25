package agent

// answer_document_rank_restricted_ready.go — HMC377 round 16 (batch-owner
// rulings R-377-16 ④, R-377-17 ③④, 2026-11-24): the finalizer request's
// placement of the one restricted-ready decision (tool.TraceRestrictedReadyHolds,
// read by the reader face and the rank summary too) on the lines of the
// reader-ready rank section.
//
// Since da5593e65 (R-377-11 ②) the restricted ready instants of a thread that
// lie inside its chain object are priced on the thread's chain seat: the
// request listed that seat as a plain runnable line and the CPU-restriction
// seat with only its remainder (Douyin reboot, dio w45: 「#8 udk-irq-7-76：
// runnable」 3.913 ms and 「原始排序 #9 udk-irq-7-76：CPU亲和/cpuset限制」
// 2.693 ms, while the seat's typed account is 6.606 ms restricted under
// allowed CPU 7), and nothing related the two lines.
//
// The section lists every ranked seat of a board on a line of its own under
// its ordinal, so a hold is placed on the seats' own lines: the holder's line
// is the line whose record the claim names (an absorbed member's: the first
// line wearing its family key, an opaque identity), the restriction seat's
// line is the seat's own (an engine-absorbed member has none).
//   - The holder's line says how much restricted time lies inside its account
//     (beside the seat's whole account where the engine split it) and, on the
//     thread's first holding line, the seat's CPU-constraint description —
//     one per thread and request, the thread read by its typed identity
//     (tool.TraceRestrictedReadyThread).
//   - The restriction seat's own line (the split arm) says its account and
//     that the part lies inside the holder's ordinal.
//
// The words are the reader's (tool.TraceRestrictedReadyLineText,
// tool.TraceCPUConstraintDescription); a holder is named by the request's
// own ordinal words, with its list's name when it sits on another list. A
// hold with a holder on no line of the request states nothing: the engine
// types no split between the holders a claim names.

import (
	"fmt"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// traceRankRestrictedAt is a seat's line of the rank section: the authority's
// index and the seat's index on it.
type traceRankRestrictedAt struct{ authority, seat int }

func traceRankRestrictedBefore(a, b traceRankRestrictedAt) bool {
	if a.authority != b.authority {
		return a.authority < b.authority
	}
	return a.seat < b.seat
}

// traceRankRestrictedReadyLines places the decision of every projection on
// the section's lines and words each placement: the sub-lines of each seat's
// line, in the decision's seat order. lists are the section's list numbers.
func traceRankRestrictedReadyLines(set types.TraceCausalProjectionSet, authorities []traceReaderRankDisplayAuthority, lists []int, zh bool) map[traceRankRestrictedAt][]string {
	type entry struct {
		at         traceRankRestrictedAt
		projection int
		hold       tool.TraceRestrictedReadyHold
		elsewhere  bool
		refs       []string
		describe   bool
	}
	var entries []entry
	for p, projection := range set.Projections {
		holds := tool.TraceRestrictedReadyHolds(projection)
		// The lines of this projection by the record ids they stand for and
		// by the family key they wear. A record is one line of the roster (a
		// merged record leaves its own seat), and one family row wears a key.
		byID := map[string]traceRankRestrictedAt{}
		byFamily := map[string]traceRankRestrictedAt{}
		for a, authority := range authorities {
			if authority.projection != p {
				continue
			}
			for s, seat := range authority.Seats {
				at := traceRankRestrictedAt{a, s}
				for _, id := range append([]string{seat.EvidenceID}, seat.MergedEvidenceIDs...) {
					byID[id] = at
				}
				if seat.RankFamilyKey != "" {
					byFamily[seat.RankFamilyKey] = at
				}
			}
		}
		for _, h := range holds {
			// The holders' lines: the family's line for an absorbed member
			// where a line wears the key, otherwise the lines of every record
			// the claim names — all of them, or the hold is stated nowhere.
			var holders []traceRankRestrictedAt
			if host, ok := byFamily[h.FamilyKey]; ok {
				holders = append(holders, host)
			} else {
				for _, id := range h.HolderIDs {
					at, ok := byID[id]
					if !ok {
						holders = nil
						break
					}
					known := false
					for _, k := range holders {
						known = known || k == at
					}
					if !known {
						holders = append(holders, at)
					}
				}
			}
			if len(holders) == 0 {
				continue
			}
			sort.SliceStable(holders, func(x, y int) bool { return traceRankRestrictedBefore(holders[x], holders[y]) })
			// The statement sits on the first holder's line; the other
			// holders are named by their ordinals.
			first := holders[0]
			held := entry{at: first, projection: p, hold: h}
			for _, k := range holders[1:] {
				held.refs = append(held.refs, traceRankRestrictedRef(authorities, lists, first, k, zh))
			}
			// Two parts of one seat inside one line's own account are one
			// statement of that line (disjoint instants of one account), as
			// on the reader face.
			merged := false
			for j := range entries {
				e := &entries[j]
				if e.at == held.at && e.hold.Seat.EvidenceID == h.Seat.EvidenceID && len(e.refs) == 0 && len(held.refs) == 0 &&
					e.hold.OwnAccount == h.OwnAccount {
					e.hold.MS += h.MS
					merged = true
					break
				}
			}
			if !merged {
				entries = append(entries, held)
			}
			// The restriction seat's own line, where the section draws it,
			// says its account and the holders its part lies inside.
			if seatAt, ok := byID[h.Seat.EvidenceID]; ok {
				part := entry{at: seatAt, projection: p, hold: h, elsewhere: true}
				for _, k := range holders {
					part.refs = append(part.refs, traceRankRestrictedRef(authorities, lists, seatAt, k, zh))
				}
				entries = append(entries, part)
			}
		}
	}
	// One CPU-constraint description per thread and request: the thread's
	// first holding line in the section's order. The thread is read per
	// capture (its typed identity names the board, not the capture).
	first := map[string]int{}
	for j, e := range entries {
		if e.elsewhere {
			continue
		}
		thread := fmt.Sprintf("%d|%s", e.projection, tool.TraceRestrictedReadyThread(e.hold.Seat))
		if k, seen := first[thread]; !seen || traceRankRestrictedBefore(e.at, entries[k].at) {
			first[thread] = j
		}
	}
	for _, j := range first {
		entries[j].describe = true
	}
	out := map[traceRankRestrictedAt][]string{}
	for _, e := range entries {
		out[e.at] = append(out[e.at], tool.TraceRestrictedReadyLineText(e.hold, e.elsewhere, e.refs, zh))
		if !e.describe {
			continue
		}
		if description := tool.TraceCPUConstraintDescription(e.hold.Seat, zh); description != "" {
			out[e.at] = append(out[e.at], description)
		}
	}
	return out
}

// traceRankRestrictedRef names the line to in the words the section prints it
// with, seen from the line from: its ordinal, and its list's name when it is
// on another list.
func traceRankRestrictedRef(authorities []traceReaderRankDisplayAuthority, lists []int, from, to traceRankRestrictedAt, zh bool) string {
	authority := authorities[to.authority]
	rank := authority.Seats[to.seat].Rank
	chain := authority.BoardChannel == "on_chain"
	var ref string
	switch {
	case chain:
		ref = fmt.Sprintf("#%d", rank)
	case zh:
		ref = fmt.Sprintf("原始排序 #%d", rank)
	default:
		ref = fmt.Sprintf("published order #%d", rank)
	}
	if from.authority == to.authority {
		return ref
	}
	switch {
	case chain && zh:
		return fmt.Sprintf("排名清单 %d %s", lists[to.authority], ref)
	case chain:
		return fmt.Sprintf("ranking list %d %s", lists[to.authority], ref)
	case zh:
		return fmt.Sprintf("支撑观测清单 %d %s", lists[to.authority], ref)
	default:
		return fmt.Sprintf("supporting observation list %d %s", lists[to.authority], ref)
	}
}
