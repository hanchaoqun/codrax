package types

import (
	"fmt"
	"sort"
	"strings"
)

// HMC377 R-377-11 ④ (R10P-4, 2026-11-21): the ×N merge of one thread's
// same-kind cause-partition rows keeps the causes.
//
// The engine mints one formal D/IO seat per (thread, proven blocked_reason
// caller) and one honest-remainder seat for the fragments that proved no
// caller (§29.50.5). When the projection's ×N merge folds several of those
// seats into one row, the merged row used to keep a caller only when every
// member named the same one; any disagreement cleared it. On 105 元宝
// 后台销毁冷启动 #2 (TID 58533 bindApplication [10834.099821,10834.620005))
// the analysed thread's nine partition seats merged into one 4.256 ms D row
// and vfs_path_open_ng, the proven caller of 3.435 ms of it, left the tree,
// the finalizer handoff and the side file. A merged row now keeps every
// proven caller with the ms its members hold, and the ms of members that
// proved none; the faces name them through the one wording below.

// TraceBlockedReasonCallerShare is one proven blocked_reason caller of a
// merged row and the ms its members hold.
type TraceBlockedReasonCallerShare struct {
	Caller string  `json:"caller"`
	MS     float64 `json:"ms"`
}

// TraceBlockedReasonCauseShownMax is the small fixed count of causes a face
// names before it closes the list with 「等N类」 / "and N more".
const TraceBlockedReasonCauseShownMax = 3

// CloneTraceBlockedReasonCallerShares returns an independent copy.
func CloneTraceBlockedReasonCallerShares(in []TraceBlockedReasonCallerShare) []TraceBlockedReasonCallerShare {
	if len(in) == 0 {
		return nil
	}
	return append([]TraceBlockedReasonCallerShare(nil), in...)
}

// HasBlockedReasonCause reports whether the row names a proven
// blocked_reason caller (one unanimous caller or a merged cause list).
func (n TraceCausalProjectionNode) HasBlockedReasonCause() bool {
	return strings.TrimSpace(n.BlockedReasonCaller) != "" || len(n.BlockedReasonCallerShares) > 0
}

// BlockedReasonCauseText is the row's cause wording (see
// TraceBlockedReasonCauseText).
func (n TraceCausalProjectionNode) BlockedReasonCauseText(zh bool) string {
	return TraceBlockedReasonCauseText(n.BlockedReasonCaller, n.BlockedReasonCallerShares, zh)
}

// TraceBlockedReasonCauseText is the one wording of a seat's proven
// blocked_reason causes, shared by the tree, the finalizer handoff and the
// side file. A unanimous caller keeps the single-cause word verbatim (the
// caller symbol). A merged cause list names each cause with its ms, largest
// first, and closes after TraceBlockedReasonCauseShownMax causes with the
// existing count idioms: zh 「等N类」 counts every cause (as 「等N席」 /
// 「等 N 线程」 do), en "and N more" counts the causes not named (as
// "and N more seats" does). "" when the row names no cause.
func TraceBlockedReasonCauseText(caller string, shares []TraceBlockedReasonCallerShare, zh bool) string {
	if caller = strings.TrimSpace(caller); caller != "" {
		return caller
	}
	if len(shares) == 0 {
		return ""
	}
	shown := shares
	if len(shown) > TraceBlockedReasonCauseShownMax {
		shown = shown[:TraceBlockedReasonCauseShownMax]
	}
	parts := make([]string, 0, len(shown))
	for _, share := range shown {
		if share.MS > 0 {
			parts = append(parts, fmt.Sprintf("%s %.3fms", share.Caller, share.MS))
		} else {
			// A valueless member still names its cause; it never prints a
			// zero it did not measure.
			parts = append(parts, share.Caller)
		}
	}
	more := len(shares) - len(shown)
	if zh {
		text := strings.Join(parts, "、")
		if more > 0 {
			text += fmt.Sprintf(" 等%d类", len(shares))
		}
		return text
	}
	text := strings.Join(parts, ", ")
	if more > 0 {
		text += fmt.Sprintf(" and %d more", more)
	}
	return text
}

// TraceBlockedReasonUnprovenText words the ms of a merged cause row that no
// proven caller holds, with the existing 「原因未证」 / "cause unproven"
// qualifier. "" when there is none.
func TraceBlockedReasonUnprovenText(ms float64, zh bool) string {
	if ms <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("原因未证 %.3fms", ms)
	}
	return fmt.Sprintf("cause unproven %.3fms", ms)
}

// TraceCausalProjectionAdoptBlockedReasonCause fills an empty cause slot on
// dst from src as one unit (caller, cause list, unproven ms). Copies of ONE
// fact (the R1 same-fact absorb, same-segment twins, a folded rank twin) use
// it; a row that already names a cause keeps its own.
func TraceCausalProjectionAdoptBlockedReasonCause(dst *TraceCausalProjectionNode, src TraceCausalProjectionNode) {
	if dst == nil || dst.HasBlockedReasonCause() {
		return
	}
	dst.BlockedReasonCaller = src.BlockedReasonCaller
	dst.BlockedReasonCallerShares = CloneTraceBlockedReasonCallerShares(src.BlockedReasonCallerShares)
	dst.BlockedReasonUnprovenMS = src.BlockedReasonUnprovenMS
}

// TraceCausalProjectionFoldBlockedReasonCauses tallies the causes of a
// display fold whose value is the account Σ of its member rows (the micro
// anchored-seat fold): the same tally as the ×N merge body, every member
// counted, Σ caliber. A fold row always names its causes as a list with ms,
// even when every member names the same caller, since it holds several seats.
func TraceCausalProjectionFoldBlockedReasonCauses(members []TraceCausalProjectionNode) ([]TraceBlockedReasonCallerShare, float64) {
	idx := make([]int, len(members))
	for i := range idx {
		idx[i] = i
	}
	return traceCausalProjectionMergedBlockedReasonCauses(members, idx, traceCausalProjectionDisplayValue, true)
}

// traceCausalProjectionMergedBlockedReasonCauses tallies the causes of a ×N
// merged group: each member contributes its own cause list, else its
// unanimous caller with its display value, else (an honest-remainder seat)
// its display value as unproven. sumCaliber reports whether the merged row's
// value is the plain Σ of its members; when it is a union, a MAX or a
// same-segment mirror instead, the members may re-measure the same instants,
// so each cause keeps its largest member value (a lower bound that never
// invents) rather than a Σ. Order: ms descending, ties by first appearance.
func traceCausalProjectionMergedBlockedReasonCauses(nodes []TraceCausalProjectionNode, members []int, display func(TraceCausalProjectionNode) float64, sumCaliber bool) ([]TraceBlockedReasonCallerShare, float64) {
	type tally struct {
		first    int
		sum, max float64
	}
	byCaller := map[string]*tally{}
	add := func(caller string, ms float64) {
		caller = strings.TrimSpace(caller)
		if caller == "" {
			return
		}
		if ms < 0 {
			ms = 0
		}
		t, ok := byCaller[caller]
		if !ok {
			t = &tally{first: len(byCaller)}
			byCaller[caller] = t
		}
		t.sum += ms
		if ms > t.max {
			t.max = ms
		}
	}
	unprovenSum, unprovenMax := 0.0, 0.0
	addUnproven := func(ms float64) {
		if ms <= 0 {
			return
		}
		unprovenSum += ms
		if ms > unprovenMax {
			unprovenMax = ms
		}
	}
	for _, idx := range members {
		member := nodes[idx]
		switch {
		case len(member.BlockedReasonCallerShares) > 0:
			for _, share := range member.BlockedReasonCallerShares {
				add(share.Caller, share.MS)
			}
			addUnproven(member.BlockedReasonUnprovenMS)
		case strings.TrimSpace(member.BlockedReasonCaller) != "":
			add(member.BlockedReasonCaller, display(member))
		case member.DStateCauseUnprovenRemainder:
			addUnproven(display(member))
		}
	}
	if len(byCaller) == 0 {
		return nil, 0
	}
	shares := make([]TraceBlockedReasonCallerShare, 0, len(byCaller))
	firsts := make(map[string]int, len(byCaller))
	for caller, t := range byCaller {
		ms := t.max
		if sumCaliber {
			ms = t.sum
		}
		shares = append(shares, TraceBlockedReasonCallerShare{Caller: caller, MS: ms})
		firsts[caller] = t.first
	}
	sort.SliceStable(shares, func(i, j int) bool {
		if shares[i].MS != shares[j].MS {
			return shares[i].MS > shares[j].MS
		}
		return firsts[shares[i].Caller] < firsts[shares[j].Caller]
	})
	if sumCaliber {
		return shares, unprovenSum
	}
	return shares, unprovenMax
}
