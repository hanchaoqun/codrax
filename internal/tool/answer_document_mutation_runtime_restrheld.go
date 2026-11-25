package tool

// answer_document_mutation_runtime_restrheld.go — HMC377 round 16 (batch-owner
// ruling R-377-16 ③, 2026-11-24): a drawn row that prices ready instants of a
// CPU-restriction seat says so beneath itself when that seat is not a row of
// its own on the face.
//
// Since da5593e65 (R-377-11 ②) the restricted ready instants that lie inside a
// thread's chain object are priced on the thread's on-chain row; the
// restriction seat is then an absorbed member of that row (every restricted
// instant inside one row) or keeps only the instants no on-chain seat holds
// and may sit in a context fold row. The chain row printed no restriction
// word, and the engine's judgment — allowed cores, excluded observed cores,
// basis — was on no reader face (dio w01 udk-irq-7-76: main drew
// 「⧖ udk-irq-7-76 · CPU亲和/cpuset限制 18.308ms」 with 「CPU约束描述:允许核 7 ·
// 排除全域观测核 0-6 · 判定依据 sched_switch_next_info」; merge 14 drew
// 「├─链上─ ➋ ⧖ udk-irq-7-76 · runnable 11.364ms」 and nothing else; 31
// thread-windows of the 315: ~/hmc-work/hmc377/fix15/scout/res/onchain_class_m14.txt).
// Supply-side facts are must-not-lose on the chain.
//
// One predicate, evaluated once per (restriction seat, model) and read by both
// languages and by every section that draws the holding row with its notes
// (tree line, flat board, stanza rows, detail block). The relation is the
// engine's, read from typed fields only:
//   - (a) the restriction seat is an engine-absorbed member of the row
//     (AbsorbedByRankFamily, AbsorbedInto == the row's RankFamilyKey — the
//     host rule runtimeTraceProjAbsorbedChainHosts shares with the 「链上并入」
//     note and the drawn-record index); where the family's seat is no row of
//     its own on the face (its record is drawn inside its chain hop row), the
//     absorbed seat's own resolved claim names that seat's record, as in (b);
//   - (b) the restriction seat left part of its restricted ready time to rows
//     of the published board (RestrictedReadyAccountMS and
//     ShareCountedElsewhereMS) and the resolver's claim for that part names
//     this row's record as the holder.
//
// No pair-key string, no thread name and no value decides it. A restriction
// seat drawn as a row of its own keeps its description and its own
// 「…另行计入[E#]」 note there; the holding row then adds nothing.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjRestrictedHeld is one share of a CPU-restriction seat's
// restricted ready time that the row prices while the seat is not drawn as a
// row of its own.
type runtimeTraceProjRestrictedHeld struct {
	// MS is the restricted time priced on this row: the absorbed member's own
	// value, or the engine share of the claim this row holds — never the
	// row's value.
	MS float64
	// Seat is the restriction seat (label and description source).
	Seat types.TraceCausalProjectionNode
	// Tag is the seat's own [E#] when it is this row's absorbed member (the
	// number its 「链上并入」 note cites); "" for a seat without an [E#] of its
	// own on this face.
	Tag string
	// Elsewhere: no holder of the share is this row's own account — every
	// holder is a seat drawn inside this row under its own [E#] (a rank seat in
	// its hop row's bracket, whose host publishes the host's account).
	Elsewhere bool
	// Refs are the [E#] of the holders that are not this row's own account:
	// the seats drawn inside this row under their own [E#] and the other rows
	// the engine names as holders of the same claim (one share, several
	// holders: the engine types no split between them).
	Refs []string
	// OwnAccount: the engine's chain-account claim stands beside this walked
	// part — the row keeps its own, diverging account (case A'), so the share
	// lies inside the row's time and is no part of the row's value.
	OwnAccount bool
	// Member is the seat's thread when the row draws several threads (the
	// micro anchored-seat fold): the sentence then names the member, since
	// 「本线程」 has no referent on such a row. "" on the thread's own row.
	Member string
	// Describe: this is the first row, in drawing order, that prices a share
	// of the seat — the seat's CPU-constraint description is printed here and
	// nowhere else on the face.
	Describe bool
}

const runtimeTraceProjRestrictionSeatType = "cpu_affinity_or_cpuset"

// runtimeTraceProjRestrictionSeatNode reads the engine's type token.
func runtimeTraceProjRestrictionSeatNode(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionCanonicalNode(firstNonEmptyAnswerString(node.TypeToken, node.Object)) == runtimeTraceProjRestrictionSeatType
}

// runtimeTraceProjRestrictedPartClaim is THE selection of the claims by which
// a CPU-restriction seat hands part of its restricted ready time to rows of
// the published board: a resolved, positive claim of a walked or unwalked
// part that is no "represented by" sentence. The restriction row's own
// 「…另行计入[E#]」 note (runtimeTraceProjStampRestrictedParts) and the holding
// row's 「其中」 line (runtimeTraceProjStampRestrictedHeld) read this one
// selection.
func runtimeTraceProjRestrictedPartClaim(part string, resolved, represented bool, shareMS float64) bool {
	return !represented && resolved && shareMS > 0 &&
		(part == string(tracequery.RootCauseShareClaimAnchored) || part == string(tracequery.RootCauseShareClaimUnwalked))
}

// runtimeTraceProjRestrictedPartOwnAccount: the walked part of a seat whose
// chain seat keeps its own account (case A' — the seat's chain-account claim).
func runtimeTraceProjRestrictedPartOwnAccount(chainAccount bool, part, role string) bool {
	return chainAccount && part == string(tracequery.RootCauseShareClaimAnchored) && role == string(tracequery.RootCauseShareHolderChainSeat)
}

// runtimeTraceProjRestrictionSeatDrawn: one of the seat's records is the own
// account of a drawn CPU-restriction row. A context fold row that seats the
// record carries no type of its own, so it never counts; the analysed
// thread's restriction row drawn as a 「其中」 line under its ready account
// (R-377-12 ④) stays a row of the model and counts.
func runtimeTraceProjRestrictionSeatDrawn(seat types.TraceCausalProjectionNode, index map[string]runtimeTraceProjDrawnRecord) bool {
	for _, id := range runtimeTraceProjNodeRecordIDs(seat) {
		drawn, ok := index[id]
		if !ok || drawn.row == nil || !drawn.own {
			continue
		}
		if runtimeTraceProjRestrictionSeatNode(drawn.row.Node) {
			return true
		}
	}
	return false
}

// runtimeTraceProjStampRestrictedHeld stamps every row that prices a share of
// a CPU-restriction seat not drawn as a row of its own (see the file header).
// It runs once per model, after every row lane is populated and the absorbed
// members are attached, and before the micro anchored-seat fold, whose row
// takes its members' stamps (runtimeTraceProjFoldRestrictedHeld). ranked is
// the projection's complete seat roster (RankedSeats: a seat the context cap
// folded keeps its typed claims there).
func runtimeTraceProjStampRestrictedHeld(model *runtimeTraceProjTreeModel, ranked []types.TraceCausalProjectionNode,
	evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) {
	if model == nil {
		return
	}
	all := runtimeTraceProjSMR1AllRows(model)
	order := map[*runtimeTraceProjTreeRow]int{}
	for i, row := range all {
		row.RestrictedHeld = nil
		order[row] = i
	}
	type share struct {
		row  *runtimeTraceProjTreeRow
		held runtimeTraceProjRestrictedHeld
	}
	// One list per restriction seat, in seat order (absorbed members first,
	// then the roster): the first holding row in drawing order describes it.
	var seats [][]share
	// holds reports a row that publishes its own account (a row of several
	// records whose value is the members' maximum prices no member's share).
	holds := func(row *runtimeTraceProjTreeRow) bool {
		return row != nil && row.HasData && !row.Node.OnChainOverflowFold && !runtimeTraceProjSubjectlessFoldRow(row.Node)
	}
	index := runtimeTraceProjDrawnRecordIndex(model, all)
	// claimed places the parts of the seat the resolver's claims hand to rows
	// of the published board on the rows that draw the holders' records.
	claimed := func(seat types.TraceCausalProjectionNode) []share {
		chainAccount := false
		for _, c := range seat.ShareClaims {
			chainAccount = chainAccount || c.Part == string(tracequery.RootCauseShareClaimChainAccount)
		}
		var shares []share
		for _, c := range seat.ShareClaims {
			if !runtimeTraceProjRestrictedPartClaim(c.Part, c.Resolved, c.Represented, c.ShareMS) {
				continue
			}
			// Every holder the engine names must be drawn in a row that
			// publishes an account: a share with a holder this face does
			// not draw is stated on no row (the engine types no split
			// between holders). The line sits on the first such row; a
			// holder that is that row's own account reads 「本行账」, every
			// other holder — a seat drawn inside the row under its own
			// [E#], or another row — is named by its [E#].
			type holderAt struct {
				row *runtimeTraceProjTreeRow
				tag string
				own bool
			}
			var holders []holderAt
			placed := len(c.HolderEvidenceIDs) > 0
			for _, id := range c.HolderEvidenceIDs {
				drawn, ok := index[strings.TrimSpace(id)]
				if !ok || !holds(drawn.row) {
					placed = false
					break
				}
				at := holderAt{row: drawn.row, tag: strings.TrimSpace(drawn.tag),
					own: drawn.own || strings.TrimSpace(drawn.tag) == strings.TrimSpace(drawn.row.EvidenceTag)}
				known := false
				for _, h := range holders {
					known = known || (h.row == at.row && h.tag == at.tag)
				}
				if !known {
					holders = append(holders, at)
				}
			}
			if !placed {
				continue
			}
			sort.SliceStable(holders, func(i, j int) bool { return order[holders[i].row] < order[holders[j].row] })
			first := holders[0].row
			held := runtimeTraceProjRestrictedHeld{MS: c.ShareMS, Seat: seat, Elsewhere: true,
				OwnAccount: runtimeTraceProjRestrictedPartOwnAccount(chainAccount, c.Part, c.Role)}
			for _, h := range holders {
				switch {
				case h.row == first && h.own:
					held.Elsewhere = false
				case h.own:
					held.Refs = append(held.Refs, strings.TrimSpace(h.row.EvidenceTag))
				default:
					held.Refs = append(held.Refs, h.tag)
				}
			}
			// Two parts of the seat priced on one row's own account are one
			// line of that row (disjoint instants of one account).
			merged := false
			for i := range shares {
				if shares[i].row == first && shares[i].held.OwnAccount == held.OwnAccount &&
					!shares[i].held.Elsewhere && !held.Elsewhere && len(shares[i].held.Refs) == 0 && len(held.Refs) == 0 {
					shares[i].held.MS += held.MS
					merged = true
					break
				}
			}
			if !merged {
				shares = append(shares, share{row: first, held: held})
			}
		}
		return shares
	}
	hosts := runtimeTraceProjAbsorbedChainHosts(model)
	for _, seat := range model.absorbedChainRows {
		if !runtimeTraceProjRestrictionSeatNode(seat) {
			continue
		}
		if host := hosts[strings.TrimSpace(seat.AbsorbedInto)]; host != nil {
			if ms := runtimeTraceProjNodeDisplayImpact(seat); holds(host) && ms > 0 {
				seats = append(seats, []share{{row: host, held: runtimeTraceProjRestrictedHeld{
					MS: ms, Seat: seat, Tag: runtimeTraceProjEvidenceTag(seat, evidence, zh)}}})
			}
			continue
		}
		// The family's seat is no row of its own on this face: the absorbed
		// seat's resolved claim names its record, drawn inside another row.
		// The absorbed seat has no [E#] here (no 「链上并入」 note cites it),
		// so the line points at none.
		if shares := claimed(seat); len(shares) > 0 {
			seats = append(seats, shares)
		}
	}
	seen := map[string]bool{}
	for _, seat := range ranked {
		if !runtimeTraceProjRestrictionSeatNode(seat) || seat.RestrictedReadyAccountMS <= 0 || seat.ShareCountedElsewhereMS <= 0 {
			continue
		}
		key := runtimeTraceCausalProjectionNodeKey(seat)
		if seen[key] || runtimeTraceProjRestrictionSeatDrawn(seat, index) {
			continue
		}
		seen[key] = true
		if shares := claimed(seat); len(shares) > 0 {
			seats = append(seats, shares)
		}
	}
	for _, shares := range seats {
		first := 0
		for i := range shares {
			if order[shares[i].row] < order[shares[first].row] {
				first = i
			}
		}
		shares[first].held.Describe = true
		sort.SliceStable(shares, func(i, j int) bool { return order[shares[i].row] < order[shares[j].row] })
		for _, s := range shares {
			s.row.RestrictedHeld = append(s.row.RestrictedHeld, s.held)
		}
	}
}

// runtimeTraceProjFoldRestrictedHeld hands the members' stamps to the row that
// draws them as one (the micro anchored-seat fold publishes the members'
// account sum): each share names its member, since the fold row draws several
// threads.
func runtimeTraceProjFoldRestrictedHeld(members []runtimeTraceProjTreeRow) []runtimeTraceProjRestrictedHeld {
	var out []runtimeTraceProjRestrictedHeld
	for _, member := range members {
		for _, held := range member.RestrictedHeld {
			if held.Member == "" {
				held.Member = strings.TrimSpace(member.Node.Subject)
			}
			out = append(out, held)
		}
	}
	return out
}

// runtimeTraceProjOfWhichRestrictedText is the one 「其中」 sentence of a
// CPU-restriction seat's time stated under the row that counts it: the
// restricted time, whose runnable time it is part of, the account it is
// already inside, and that it is never added again. The analysed thread's
// form (R-377-12 ④) and the holding row's form (R-377-16 ③) are this one
// sentence with their own subject and account words.
func runtimeTraceProjOfWhichRestrictedText(label string, ms float64, whose, account, pointer string, zh bool) string {
	if zh {
		return fmt.Sprintf("其中%s下%.3fms是%srunnable时间里受限的那一部分,已含在%s内,不另计%s", label, ms, whose, account, pointer)
	}
	return fmt.Sprintf("of which %.3fms under the %s is the restricted part of %s runnable time, already inside %s, never added again%s", ms, label, whose, account, pointer)
}

// runtimeTraceProjRestrictedHeldText words one share on its holding row. The
// amount is the share this row prices, so the sentence never says the whole
// row is restricted. The account the share is already inside is the row's own
// (「本行账」) or the seat's this row draws under its own [E#] (「[E#]账」: the
// row's headline is then the host's account, on which 「本行账」 would be
// false). Case A' takes the restriction row's own A' words turned to the
// holding row (the share lies inside the seat's time; the seat keeps its own
// account): 「其中…已含在…账内」 would be false there.
func runtimeTraceProjRestrictedHeldText(held runtimeTraceProjRestrictedHeld, zh bool) string {
	label := strings.TrimSpace(TraceRootCauseTypeDisplayLabel(held.Seat.Object, zh))
	pointer := ""
	if tag := strings.TrimSpace(held.Tag); tag != "" {
		pointer = runtimeTraceProjProsePointer(0, tag)
	}
	member := strings.TrimSpace(held.Member)
	refs := make([]string, 0, len(held.Refs))
	for _, ref := range held.Refs {
		refs = append(refs, "["+ref+"]")
	}
	if held.OwnAccount {
		switch {
		case held.Elsewhere && len(refs) > 0 && zh:
			return fmt.Sprintf("%s下%.3fms落在链上席%s时段内(链席另列自账)%s", label, held.MS, refs[0], pointer)
		case held.Elsewhere && len(refs) > 0:
			return fmt.Sprintf("%.3fms under the %s lies inside the time of the chain seat %s (the chain seat keeps its own separate account)%s", held.MS, label, refs[0], pointer)
		case zh && member != "":
			return fmt.Sprintf("%s下%.3fms落在成员 %s 时段内(该成员另列自账)%s", label, held.MS, member, pointer)
		case zh:
			return fmt.Sprintf("%s下%.3fms落在本行时段内(本行另列自账)%s", label, held.MS, pointer)
		case member != "":
			return fmt.Sprintf("%.3fms under the %s lies inside the time of member %s (the member keeps its own separate account)%s", held.MS, label, member, pointer)
		default:
			return fmt.Sprintf("%.3fms under the %s lies inside the time of this row (this row keeps its own separate account)%s", held.MS, label, pointer)
		}
	}
	whose, account := "本线程", "本行账"
	if !zh {
		whose, account = "this thread's", "this row's account"
	}
	if member != "" {
		whose = "成员 " + member + " "
		if !zh {
			whose = "member " + member + "'s"
		}
	}
	switch {
	case len(refs) == 0:
	case held.Elsewhere && zh:
		account = strings.Join(refs, "、") + "账"
	case held.Elsewhere && len(refs) == 1:
		account = "the account of " + refs[0]
	case held.Elsewhere:
		account = "the accounts of " + strings.Join(refs, ", ")
	case zh:
		account = "本行与" + strings.Join(refs, "、") + "账"
	default:
		account = "the accounts of this row and " + strings.Join(refs, ", ")
	}
	return runtimeTraceProjOfWhichRestrictedText(label, held.MS, whose, account, pointer, zh)
}

// runtimeTraceProjRestrictedHeldLines renders a row's stamped shares as
// subordinate lines under the given rails: the 「其中」 line of each share and,
// on the seat's first holding row, the seat's CPU-constraint description in
// the one description renderer's words.
func runtimeTraceProjRestrictedHeldLines(row runtimeTraceProjTreeRow, indent string, zh bool) []string {
	var out []string
	for _, held := range row.RestrictedHeld {
		out = append(out, runtimeTraceProjSubordinateLines(indent, runtimeTraceProjRestrictedHeldText(held, zh))...)
		if !held.Describe {
			continue
		}
		if description := runtimeTraceProjCPUConstraintDescriptionText(held.Seat, zh); description != "" {
			out = append(out, runtimeTraceProjSubordinateLines(indent, description)...)
		}
	}
	return out
}

// runtimeTraceProjRestrictedHeldDetail is the holding row's detail-block
// mirror of those lines: the same sentence under the restriction's own type
// label, and the description under its own head word.
func runtimeTraceProjRestrictedHeldDetail(row runtimeTraceProjTreeRow, zh bool) [][2]string {
	var out [][2]string
	for _, held := range row.RestrictedHeld {
		out = append(out, [2]string{strings.TrimSpace(TraceRootCauseTypeDisplayLabel(held.Seat.Object, zh)), runtimeTraceProjRestrictedHeldText(held, zh)})
		if !held.Describe {
			continue
		}
		if description := runtimeTraceProjCPUConstraintDescriptionText(held.Seat, zh); description != "" {
			head := runtimeTraceProjCPUConstraintDescriptionHead(zh)
			out = append(out, [2]string{strings.TrimRight(strings.TrimSpace(head), ":"), strings.TrimPrefix(description, head)})
		}
	}
	return out
}

// traceQueryCPUConstraintKVs is the one list of a rank item's typed
// CPU-constraint judgment as note pairs (empty values are dropped by the
// writer): the item's observation record carries it, and the rank preview row
// of a row that holds the seat's time prints it (R-377-16 ④).
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

// traceQueryRankRestrictedReadyFields is the rank preview's carrier of the
// same fact (R-377-16 ④), under the same predicate read on the published
// board: a board row whose absorbed member is a CPU-restriction seat (the
// engine's AbsorbedByRankFamily / AbsorbedIntoFamily == the row's
// RankFamilyKey) states the restricted time inside it and the engine's
// constraint judgment under the note keys the seat's own record carries. An
// absorbed seat is no row of the board, so nothing else on the preview names
// it. A restriction seat that keeps a board row of its own (its restricted
// time split between rows) speaks there — its typed account, its claims and
// its judgment ride its own row and record — and the holding row adds
// nothing. The verbatim policy string stays on the seat's record: its inner
// key=value tokens would read as fields of the holding row.
// Keyed by the row's index in rank.Items; every other row gets "".
func traceQueryRankRestrictedReadyFields(rank *tracequery.RootCauseRankResult) map[int]string {
	if rank == nil {
		return nil
	}
	var out map[int]string
	for _, seat := range rank.AbsorbedItems {
		key := strings.TrimSpace(seat.AbsorbedIntoFamily)
		if runtimeTraceCausalProjectionCanonicalNode(seat.Type) != runtimeTraceProjRestrictionSeatType || !seat.AbsorbedByRankFamily || key == "" {
			continue
		}
		ms := seat.ImpactMs
		if ms <= 0 {
			ms = seat.CumulativeImpactMs
		}
		if ms <= 0 {
			continue
		}
		for at, row := range rank.Items {
			if strings.TrimSpace(row.RankFamilyKey) != key {
				continue
			}
			var b strings.Builder
			fmt.Fprintf(&b, " cpu_restricted_ready_inside_this_row=%.3fms", ms)
			for _, kv := range traceQueryCPUConstraintKVs(seat) {
				if kv[0] == types.TraceNoteKeyCPUConstraintPolicy || strings.TrimSpace(kv[1]) == "" {
					continue
				}
				b.WriteString(" " + kv[0] + "=" + kv[1])
			}
			if out == nil {
				out = map[int]string{}
			}
			out[at] += b.String()
			break
		}
	}
	return out
}
