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
// Which rows hold which restricted time is ONE decision shared by every face
// (TraceRestrictedReadyHolds, trace_query_restricted_ready.go: the reader, the
// rank summary and the finalizer request read it — R-377-17 ③). The relation
// is the engine's, read from typed fields only:
//   - (a) the restriction seat is an engine-absorbed member of a rank family
//     (AbsorbedByRankFamily with its AbsorbedInto key); its hold sits on the
//     row that wears the family key — AbsorbedInto compared with the row's
//     RankFamilyKey as an opaque identity, never parsed (the host rule
//     runtimeTraceProjAbsorbedChainHosts shares with the 「链上并入」 note and
//     the drawn-record index); where no row wears the key on the face (the
//     family's seat is drawn inside its chain hop row), on the row that draws
//     the record its resolved claim names, as in (b);
//   - (b) the engine typed the seat's restricted ready account as split
//     between rows of the published board (RestrictedReadyAccountMS and
//     ShareCountedElsewhereMS) and the resolver's claim for a part names the
//     holder's record.
// The amount is the claim's share on both arms, never a row value. No thread
// name, no value and no label decides a hold.
//
// This file is the reader's placement and wording of the decision, once per
// model, read by both languages and by every section that draws the holding
// row with its notes (tree line, flat board, stanza rows, detail block). A
// restriction seat drawn as a row of its own keeps its description and its
// own 「…另行计入[E#]」 note there; a holding row then states no share of it.
// One CPU-constraint description per (thread, face), the thread read by its
// typed identity (TraceRestrictedReadyThread): where a restriction row of the
// thread is drawn, it describes, and a holding row of that thread adds none;
// otherwise the first holding row of the thread in drawing order describes.
//
// What the line says follows the same typed fields: where the seat's typed
// account is split around the time the row states (TraceRestrictedReadySplit)
// the row states that account and its part in the words of the restriction
// row's own account sentence; otherwise the 「其中」 sentence states the time.

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
	// MS is the restricted time priced on this row: the engine share of the
	// claim this row holds (the decision's TraceRestrictedReadyHold.MS) —
	// never the row's value.
	MS float64
	// Seat is the restriction seat (label and description source). Its
	// RestrictedReadyAccountMS, where it exceeds MS (TraceRestrictedReadySplit),
	// is the whole restricted ready account MS is a part of: the line states
	// both (runtimeTraceProjRestrictedHeldText).
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
	// of a restriction seat of the thread, and no restriction row of the
	// thread is drawn — the seat's CPU-constraint description is printed here
	// and nowhere else on the face (one per thread and face, the thread read
	// by its typed identity, TraceRestrictedReadyThread).
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

// runtimeTraceProjStampRestrictedHeld places the decision's holds on the rows
// of the model (see the file header). It runs once per model, after every row
// lane is populated and the absorbed members are attached, and before the
// micro anchored-seat fold, whose row takes its members' stamps
// (runtimeTraceProjFoldRestrictedHeld). holds is TraceRestrictedReadyHolds of
// the model's projection.
func runtimeTraceProjStampRestrictedHeld(model *runtimeTraceProjTreeModel, holds []TraceRestrictedReadyHold,
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
		row    *runtimeTraceProjTreeRow
		held   runtimeTraceProjRestrictedHeld
		thread string
	}
	// One list per restriction seat, in the decision's seat order.
	var seats [][]share
	seatAt := map[int]int{}
	// publishes reports a row that publishes its own account (a row of
	// several records whose value is the members' maximum prices no member's
	// share).
	publishes := func(row *runtimeTraceProjTreeRow) bool {
		return row != nil && row.HasData && !row.Node.OnChainOverflowFold && !runtimeTraceProjSubjectlessFoldRow(row.Node)
	}
	index := runtimeTraceProjDrawnRecordIndex(model, all)
	// The threads with a restriction row drawn as a row of its own describe
	// there (the analysed thread's 「其中」 line under its ready account,
	// R-377-12 ④, is such a row).
	described := map[string]bool{}
	for _, drawn := range index {
		if drawn.row != nil && drawn.own && runtimeTraceProjRestrictionSeatNode(drawn.row.Node) {
			described[TraceRestrictedReadyThread(drawn.row.Node)] = true
		}
	}
	hosts := runtimeTraceProjAbsorbedChainHosts(model)
	for _, h := range holds {
		if runtimeTraceProjRestrictionSeatDrawn(h.Seat, index) {
			continue
		}
		thread := TraceRestrictedReadyThread(h.Seat)
		var placed *share
		if host := hosts[h.FamilyKey]; h.FamilyKey != "" && host != nil {
			// The absorbed member's 「链上并入」 note cites its [E#] on its
			// family's row: the line points at it.
			if publishes(host) {
				placed = &share{row: host, thread: thread, held: runtimeTraceProjRestrictedHeld{MS: h.MS, Seat: h.Seat,
					Tag: runtimeTraceProjEvidenceTag(h.Seat, evidence, zh), OwnAccount: h.OwnAccount}}
			}
		} else if len(h.HolderIDs) > 0 {
			// Every holder the claim names must be drawn in a row that
			// publishes an account: a share with a holder this face does not
			// draw is stated on no row (the engine types no split between
			// holders). The line sits on the first such row; a holder that is
			// that row's own account reads 「本行账」, every other holder — a
			// seat drawn inside the row under its own [E#], or another row —
			// is named by its [E#]. Where no row wears an absorbed seat's
			// family key it has no [E#] on the face: the line points at none.
			type holderAt struct {
				row *runtimeTraceProjTreeRow
				tag string
				own bool
			}
			var holders []holderAt
			ok := true
			for _, id := range h.HolderIDs {
				drawn, found := index[id]
				if !found || !publishes(drawn.row) {
					ok = false
					break
				}
				at := holderAt{row: drawn.row, tag: strings.TrimSpace(drawn.tag),
					own: drawn.own || strings.TrimSpace(drawn.tag) == strings.TrimSpace(drawn.row.EvidenceTag)}
				known := false
				for _, k := range holders {
					known = known || (k.row == at.row && k.tag == at.tag)
				}
				if !known {
					holders = append(holders, at)
				}
			}
			if ok {
				sort.SliceStable(holders, func(i, j int) bool { return order[holders[i].row] < order[holders[j].row] })
				first := holders[0].row
				held := runtimeTraceProjRestrictedHeld{MS: h.MS, Seat: h.Seat, Elsewhere: true, OwnAccount: h.OwnAccount}
				for _, k := range holders {
					switch {
					case k.row == first && k.own:
						held.Elsewhere = false
					case k.own:
						held.Refs = append(held.Refs, strings.TrimSpace(k.row.EvidenceTag))
					default:
						held.Refs = append(held.Refs, k.tag)
					}
				}
				placed = &share{row: first, thread: thread, held: held}
			}
		}
		if placed == nil {
			continue
		}
		at, known := seatAt[h.seat]
		if !known {
			at = len(seats)
			seatAt[h.seat] = at
			seats = append(seats, nil)
		}
		// Two parts of the seat priced on one row's own account are one line
		// of that row (disjoint instants of one account).
		merged := false
		for i := range seats[at] {
			s := &seats[at][i]
			if s.row == placed.row && s.held.OwnAccount == placed.held.OwnAccount &&
				!s.held.Elsewhere && !placed.held.Elsewhere && len(s.held.Refs) == 0 && len(placed.held.Refs) == 0 {
				s.held.MS += placed.held.MS
				merged = true
				break
			}
		}
		if !merged {
			seats[at] = append(seats[at], *placed)
		}
	}
	// One description per thread and face: the first holding row of the
	// thread in drawing order, unless a restriction row of the thread is drawn.
	first := map[string]*share{}
	for i := range seats {
		for j := range seats[i] {
			s := &seats[i][j]
			if described[s.thread] {
				continue
			}
			if f := first[s.thread]; f == nil || order[s.row] < order[f.row] {
				first[s.thread] = s
			}
		}
	}
	for _, s := range first {
		s.held.Describe = true
	}
	// Each row takes its shares in seat order (a row's lines never depend on
	// another row's).
	for _, shares := range seats {
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

// runtimeTraceProjRestrictedInsideWords says which account a restricted time is
// already inside and that it is never added again — the closing words of every
// sentence that states a CPU-restriction seat's time under the row that counts
// it (the 「其中」 sentence and the holding row's part of a split account).
func runtimeTraceProjRestrictedInsideWords(account string, zh bool) string {
	if zh {
		return "已含在" + account + "内,不另计"
	}
	return "already inside " + account + ", never added again"
}

// runtimeTraceProjOfWhichRestrictedText is the one 「其中」 sentence of a
// CPU-restriction seat's time stated under the row that counts it: the
// restricted time, whose runnable time it is part of, the account it is
// already inside, and that it is never added again. The analysed thread's
// form (R-377-12 ④) and the holding row's form for a seat whose whole
// restricted time it prices (R-377-16 ③) are this one sentence with their own
// subject and account words: ms is the thread's whole restricted time.
func runtimeTraceProjOfWhichRestrictedText(label string, ms float64, whose, account, pointer string, zh bool) string {
	if zh {
		return fmt.Sprintf("其中%s下%.3fms是%srunnable时间里受限的那一部分,%s%s", label, ms, whose, runtimeTraceProjRestrictedInsideWords(account, zh), pointer)
	}
	return fmt.Sprintf("of which %.3fms under the %s is the restricted part of %s runnable time, %s%s", ms, label, whose, runtimeTraceProjRestrictedInsideWords(account, zh), pointer)
}

// runtimeTraceProjRestrictedAccountHead opens every sentence that states a
// CPU-restriction seat's whole restricted ready account before the parts of it
// that rows count: the restriction row's own account sentence
// (runtimeTraceProjRestrictedAccountText) and the holding row's line for a
// part of that account (runtimeTraceProjRestrictedHeldText).
func runtimeTraceProjRestrictedAccountHead(label string, accountMS float64, zh bool) string {
	if zh {
		return fmt.Sprintf("%s下合计%.3fms:其中", label, accountMS)
	}
	return fmt.Sprintf("%.3fms under the %s in all: of which ", accountMS, label)
}

// runtimeTraceProjRestrictedHeldText words one share on its holding row. The
// amount is the share this row prices, so the sentence never says the whole
// row is restricted.
//
// HMC377 round 16 fix-up (review finding, 2026-11-24): the amount is not always
// the thread's whole restricted time either. Where the engine typed the seat's
// restricted ready account as split between rows (Seat.RestrictedReadyAccountMS
// — the field the restriction row's own account sentence reads — exceeds MS,
// TraceRestrictedReadySplit, the one split rule of every face), MS is one part
// of that account, and the 「其中」 sentence's words 「X ms是本线程runnable时间里
// 受限的那一部分」 would give the part as the thread's restricted time (dio w01
// udk-irq-7-76: 11.364 ms stated, 18.308 ms restricted). The line then states
// the whole account and this row's part, in the words of the restriction row's
// own account sentence turned to the holding row: 「…下合计18.308ms:其中
// 11.364ms已含在本行账内,不另计」. On a seat whose account the engine did not
// split (the absorbed member: every restricted instant inside one row), MS is
// the whole restricted time and the 「其中」 sentence stands.
//
// The account the share is already inside is the row's own (「本行账」) or the
// seat's this row draws under its own [E#] (「[E#]账」: the row's headline is
// then the host's account, on which 「本行账」 would be false). Case A' takes
// the restriction row's own A' words turned to the holding row (the share lies
// inside the seat's time; the seat keeps its own account): 「…已含在…账内」
// would be false there.
func runtimeTraceProjRestrictedHeldText(held runtimeTraceProjRestrictedHeld, zh bool) string {
	refs := make([]string, 0, len(held.Refs))
	for _, ref := range held.Refs {
		refs = append(refs, "["+ref+"]")
	}
	return runtimeTraceProjRestrictedHeldWords(held, refs, zh)
}

// TraceRestrictedReadyLineText words one hold of the decision on a face that
// lists every ranked seat on a line of its own under that face's ordinal
// words (the finalizer request, R-377-17 ④), in the reader's sentence
// (runtimeTraceProjRestrictedHeldText): on a holder's line (elsewhere false)
// the restricted time inside this line's account — beside the seat's whole
// account where the engine split it. The request passes no refs there: a
// claim whose holders sit on several lines is stated on the seat's own line
// only (R-377-19 ①), and refs on a holder's line would select the reader's
// co-holder words (registered R16X-6). On the restriction seat's own line
// (elsewhere true) it words the seat's account and the lines its part lies
// inside (refs, at least one). refs are written verbatim.
func TraceRestrictedReadyLineText(hold TraceRestrictedReadyHold, elsewhere bool, refs []string, zh bool) string {
	return runtimeTraceProjRestrictedHeldWords(runtimeTraceProjRestrictedHeld{MS: hold.MS, Seat: hold.Seat,
		Elsewhere: elsewhere, OwnAccount: hold.OwnAccount}, refs, zh)
}

// TraceCPUConstraintDescription is the one description renderer's words
// (runtimeTraceProjCPUConstraintDescriptionText) for a face outside this
// package: "" when the seat carries no typed judgment.
func TraceCPUConstraintDescription(seat types.TraceCausalProjectionNode, zh bool) string {
	return runtimeTraceProjCPUConstraintDescriptionText(seat, zh)
}

// runtimeTraceProjRestrictedHeldWords is runtimeTraceProjRestrictedHeldText
// with the holders' references already written (refs).
func runtimeTraceProjRestrictedHeldWords(held runtimeTraceProjRestrictedHeld, refs []string, zh bool) string {
	label := strings.TrimSpace(TraceRootCauseTypeDisplayLabel(held.Seat.Object, zh))
	pointer := ""
	if tag := strings.TrimSpace(held.Tag); tag != "" {
		pointer = runtimeTraceProjProsePointer(0, tag)
	}
	member := strings.TrimSpace(held.Member)
	// split: the head of the split account's sentence — a row that draws
	// several threads names the member whose account it is.
	split := ""
	if account := held.Seat.RestrictedReadyAccountMS; TraceRestrictedReadySplit(account, held.MS) {
		split = runtimeTraceProjRestrictedAccountHead(label, account, zh)
		switch {
		case member != "" && zh:
			split = "成员 " + member + " " + split
		case member != "":
			split = "member " + member + "'s " + split
		}
	}
	if held.OwnAccount {
		var inside string
		switch {
		case held.Elsewhere && len(refs) > 0 && zh:
			inside = "落在链上席" + refs[0] + "时段内(链席另列自账)"
		case held.Elsewhere && len(refs) > 0:
			inside = " lies inside the time of the chain seat " + refs[0] + " (the chain seat keeps its own separate account)"
		case zh && member != "":
			inside = "落在成员 " + member + " 时段内(该成员另列自账)"
		case zh:
			inside = "落在本行时段内(本行另列自账)"
		case member != "":
			inside = " lies inside the time of member " + member + " (the member keeps its own separate account)"
		default:
			inside = " lies inside the time of this row (this row keeps its own separate account)"
		}
		switch {
		case split != "":
			return fmt.Sprintf("%s%.3fms%s%s", split, held.MS, inside, pointer)
		case zh:
			return fmt.Sprintf("%s下%.3fms%s%s", label, held.MS, inside, pointer)
		default:
			return fmt.Sprintf("%.3fms under the %s%s%s", held.MS, label, inside, pointer)
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
	switch {
	case split != "" && zh:
		return fmt.Sprintf("%s%.3fms%s%s", split, held.MS, runtimeTraceProjRestrictedInsideWords(account, zh), pointer)
	case split != "":
		return fmt.Sprintf("%s%.3fms is %s%s", split, held.MS, runtimeTraceProjRestrictedInsideWords(account, zh), pointer)
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
