package tracequery

// rank_share_claims.go — HMC377 fix round 5: one mechanism for every sentence
// that says a share of a row's account is held or represented by another row.
//
// Rounds 2-4 checked those sentences after truncation by family-level
// presence (any clipped or chain seat of the thread and family), by verbatim
// string anchors rewritten in place, by the ledger's owner support, and — in
// the renderer — by a role marker or a value match. Each check answered a
// different question than the sentence asked ("is THIS share held by a seat on
// the board?"), and each round found a board where the answers differed: a
// partition seat that died at the cap while another partition's seat
// survived, a satellite represented by a case-B seat the build cap had
// removed, a share owned by the residual-lane seat that carries no marker.
//
// Mechanism:
//   - a claim is typed where the sentence is written: the part of the account
//     it hands over (anchored / unwalked / remainder / the chain seat's own
//     account), the role of the seat that holds it (walked chain seat, case-B
//     clipped seat, the thread's own unwalked seat, the residual-lane seat,
//     the ◇ remainder seat), the thread and state family, and the share's
//     value and instants;
//   - a seat that holds a share records what it holds where it is minted
//     (rspaHeldShare: role, family, instants, value);
//   - one resolution pass runs after the final published board is known
//     (resolveRankShareClaims, after the enrich-pass truncation — both rank
//     passes and every truncation are behind it): a claim resolves iff the
//     seats on that board with the claim's thread, family and role hold the
//     claim's instants (or, for an account without a segment inventory, its
//     value); a sentence span whose claims do not all resolve is written in
//     its compaction-disclosure form, and a row whose "represented by"
//     sentence does not resolve stops wearing ChainAnchorRepresentedByChainSeat
//     (it keeps its own value on the adjacent lane — it never returns to the
//     chain tier, so the share keeps its one owner);
//   - the resolved claims ride typed to the projection (holder keys → record
//     ids) and the renderer names the holder's [E#] or says it is compacted;
//     it never searches the board for a holder.
//
// HMC377 fix round 6: a D/IO family claim also carries the speaking seat's
// cause partition (§29.50.5) and every D/IO hold the holding seat's — a seat
// minted from proof slices has no whole-td segment inventory, so its claims
// were windowless and resolved by value against ANY holder of the thread and
// family (another partition's seat). Now a claim resolves only to holders of
// its own partition (or seats that hold for the whole thread: chain seats,
// the thread's own unwalked seat), and a partition whose proof slices carried
// a complete segment list gives its claims instants
// (dioPartitionInstants).
//
// HMC377 round 10 (batch-owner ruling R-377-10): the trace_query publication
// gate can still publish a seat as context only after this resolution. Its
// typed decision is applied once, on the published copy
// (WithdrawRankShareHolders): the withdrawn row holds no share and makes no
// claim, and this same resolver re-runs — no second resolution rule. A
// represented sentence that resolves only in part publishes the row's own
// account less the shares its resolved claims hand to holders on the board
// (ChainAnchorRepresentedHeldMs), so every share keeps exactly one carrier;
// that holds alike when the engine cap, rather than the gate, cut a holder. A
// claim whose engine-resolved holder the gate withdrew says the holding row is
// published only as context; the cap-cut compaction wording is unchanged.

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RootCauseShareClaimPart names the part of a row's account a claim hands to
// another seat.
type RootCauseShareClaimPart string

const (
	// RootCauseShareClaimAnchored — the share inside the dependency windows
	// that a ◇ remainder (or a satellite / inversion detail) hands to its ⛓
	// holder: the walked share (chain seat), the case-B one object (clipped
	// seat), or the share inside the waits a host's wakeup ended (edge seat).
	RootCauseShareClaimAnchored RootCauseShareClaimPart = "anchored"
	// RootCauseShareClaimUnwalked — the share inside the analysed thread's
	// waits that this thread's own wakeup ended and the chain did not walk.
	RootCauseShareClaimUnwalked RootCauseShareClaimPart = "unwalked"
	// RootCauseShareClaimRemainder — the ◇ remainder a ⛓ seat says is
	// published as a separate adjacent seat.
	RootCauseShareClaimRemainder RootCauseShareClaimPart = "remainder"
	// RootCauseShareClaimChainAccount — the chain seat's own (diverging)
	// account a case-A' remainder says the chain seat keeps.
	RootCauseShareClaimChainAccount RootCauseShareClaimPart = "chain_account"
	// RootCauseShareClaimGatedShare (HMC377 round 11, R-377-11 ①) — the
	// LEVELMERGE-1 split's carved share: the runnable time of a chain
	// aggregate account inside the same thread's priority-inversion seat
	// branch windows, which that seat's gated composite counts. The demoted
	// constituent row and the residual seat both claim it (one resolver
	// decides both); the fail-open overlap disclosure claims its measured
	// overlap without moving a value.
	RootCauseShareClaimGatedShare RootCauseShareClaimPart = types.TraceShareClaimPartGatedShare
)

// RootCauseShareHolderRole names the kind of seat a claim says holds a share.
type RootCauseShareHolderRole string

const (
	RootCauseShareHolderChainSeat     RootCauseShareHolderRole = "chain_seat"
	RootCauseShareHolderClippedSeat   RootCauseShareHolderRole = "clipped_seat"
	RootCauseShareHolderUnwalkedSeat  RootCauseShareHolderRole = "unwalked_seat"
	RootCauseShareHolderResidualSeat  RootCauseShareHolderRole = "residual_seat"
	RootCauseShareHolderRemainderSeat RootCauseShareHolderRole = "remainder_seat"
	// RootCauseShareHolderGatedShareSeat (HMC377 round 11, R-377-11 ①): a
	// priority-inversion chain seat holding the runnable time inside its own
	// branch windows that its gated composite counts — stamped where the
	// LEVELMERGE-1 split names it as a claim seat.
	RootCauseShareHolderGatedShareSeat RootCauseShareHolderRole = "gated_share_seat"
)

// RootCauseShareClaim is one claim a row's sentences make that a share of its
// account is held by another seat of the board, as resolved against the
// published board. Consumers read Resolved / Exact / HolderKeys only (the
// holders' RootCauseRankItem.ShareHolderKey values); the instants ride
// unexported.
type RootCauseShareClaim struct {
	Part    RootCauseShareClaimPart
	Role    RootCauseShareHolderRole
	ShareMs float64
	// Resolved: the seats on the published board with this thread, family and
	// role hold the share.
	Resolved bool
	// Exact: one seat resolves the claim and publishes exactly this share.
	Exact bool
	// Represented: the claim is made by a "represented by" sentence (the row
	// wears ChainAnchorRepresentedByChainSeat only while it resolves).
	Represented bool
	HolderKeys  []string
	// ResolvedMs (HMC377 round 11, R-377-11 ①): the part of the share the
	// seats on the published board hold, measured per held segment — the
	// claim's instants those holders cover. ShareMs when Resolved. A claim
	// without instants resolves by value, whole or not at all. HolderKeys
	// name the seats that hold the resolved part.
	ResolvedMs float64
	// ReturnedMs (R-377-11 ①): the part of the share whose holder the
	// trace_query publication gate withdrew and that passed to this row
	// itself as its published successor — this row counts it with its own
	// value. Written only on a published copy, never by the engine run.
	ReturnedMs float64
	// Successor (R-377-11 ①): a seat holding part of the resolved share is
	// the published successor of a holder the gate withdrew (it holds those
	// instants because it carries them, not as a seat of the claim's role).
	Successor bool

	pid int
	dio bool
	// semantic: the share is a semantic span's in-window projection (the
	// host-edge semantic seat and its ◇ remainder), not a state family.
	semantic bool
	// partition (HMC377 fix round 6): the D/IO cause partition whose share
	// the claim names — the speaking seat's own partition, stamped where the
	// sentence is attached to the seat (rspaSetSentence / rspaAppendSentence).
	partition rspaDIOCausePartition
	windows   []foldInterval
	holderIdx []int
	// holderWithdrawn (HMC377 round 10, R-377-10 ④): the engine resolved this
	// claim to a seat that the trace_query publication gate then published as
	// context only. Stamped once where the gate's decision is applied
	// (WithdrawRankShareHolders); an unresolved claim with it names that the
	// holding row is published only as context, never that it is absent.
	holderWithdrawn bool
	// disclosure (HMC377 round 11): a value-free claim — the LEVELMERGE-1
	// fail-open overlap disclosure names the seat that counts a measured
	// overlap but moves no value, so it never takes a withdrawn holder's
	// segments.
	disclosure bool
	// covered (transient, HMC377 round 11): the claim's instants the seats of
	// its role on the published board hold (successor holds excluded) — the
	// segments the publication's successor pass leaves where they are.
	covered []foldInterval
	// carved (HMC377 round 11, R-377-11 ②): the claiming row's value is its
	// account less this share while the share resolves to a holder on the
	// published board; unresolved, the share returns to the row
	// (rspaWriteCarvedShares).
	carved bool
	// withdrawn / withdrawnWhole (HMC377 round 12, R-377-12 ②): the claim's
	// instants a seat the publication gate withdrew held (withdrawnWhole: a
	// claim without instants whose holder the gate withdrew). Stamped where
	// the gate's decision is applied (WithdrawRankShareHolders), never by the
	// engine run.
	withdrawn      []foldInterval
	withdrawnWhole bool
	// heldOwnMs / heldSuccMs / restWithdrawnMs (transient, HMC377 round 12,
	// R-377-12 ②): the claim's share per disposition after resolution — held
	// by seats of the claim's role on the published board, held by the
	// published successor of a withdrawn seat, and left by a withdrawn seat
	// with no successor. With ReturnedMs and the rest a seat the engine cap
	// cut held, they partition ShareMs; a span's words read them
	// (rspaSpanStateOf) so a mixed disposition is never worded as one.
	heldOwnMs, heldSuccMs, restWithdrawnMs float64
}

// rspaHeldShare is what one seat holds: recorded where the seat is minted,
// read only by the resolution pass.
type rspaHeldShare struct {
	role     RootCauseShareHolderRole
	dio      bool
	semantic bool
	// partition (HMC377 fix round 6): the holding seat's D/IO cause
	// partition, unset on every seat that is not one partition: the seats
	// that hold for the whole thread (a chain seat, the thread's own unwalked
	// seat) and pid-level accounts such as the state-churn twin — which
	// round 7 keeps from holding a partition's share (admits).
	partition rspaDIOCausePartition
	windows   []foldInterval
	ms        float64
	// successor / successorRoles (HMC377 round 11, R-377-11 ①): a hold the
	// publication's successor pass gave this row — instants a seat the gate
	// withdrew held and that this row now carries. It holds them for every
	// claim of the thread and family in one of the withdrawn seat's roles
	// (one carrier per instant across every share relation); it is never
	// stamped by the engine run.
	successor      bool
	successorRoles []RootCauseShareHolderRole
}

// holdsRole reports whether a successor hold carries the instants of claims
// in role.
func (h rspaHeldShare) holdsRole(role RootCauseShareHolderRole) bool {
	for _, r := range h.successorRoles {
		if r == role {
			return true
		}
	}
	return false
}

// rspaDIOCausePartition is the typed identity of one D/IO proof partition of a
// thread's account (§29.50.5): the formal window seat minted for one proven
// wait object (cause), or for the unproven fragments (cause ""). HMC377 fix
// round 6: a claim of one partition seat must never resolve to a seat of
// another partition of the same thread by value — the partitions are disjoint
// accounts. Unset (the zero value) on every seat that is not such a
// partition: it holds or claims for the whole thread family.
type rspaDIOCausePartition struct {
	set   bool
	cause string
}

// admits reports whether hold h can hold a share a claim of partition c
// names: a whole-thread claim is held by any partition's holds; a partition's
// claim is held by a hold of the same partition, or by a hold of a seat that
// holds for the whole thread — the thread-level chain seat or the thread's
// own unwalked seat. HMC377 fix round 7 (R7-3): any other hold without a
// partition (the pid-level state-churn ◇ twin's remainder) is another
// account, never the partition's own twin — a partition's remainder claim
// resolves only within its partition.
func (c rspaDIOCausePartition) admits(h rspaHeldShare) bool {
	switch {
	case !c.set:
		return true
	case h.partition.set:
		return c.cause == h.partition.cause
	default:
		return h.role == RootCauseShareHolderChainSeat || h.role == RootCauseShareHolderUnwalkedSeat
	}
}

// rspaStampClaimPartitions types the D/IO claims of a sentence attached to
// item with the seat's own cause partition.
func rspaStampClaimPartitions(item *RootCauseRankItem, claims []RootCauseShareClaim) {
	for i := range claims {
		if claims[i].dio && !claims[i].semantic {
			claims[i].partition = item.dioCausePartition
		}
	}
}

// rspaClaimSpan is one replaceable span of a row's Summary: the form for
// each disposition its claims' share can have, the claims that govern it and
// the form the Summary carries now.
//
// HMC377 round 12 (R-377-12 ②): a span takes a specific holder form only when
// its whole share has one disposition — all held by seats on the published
// board (held), all cut by the engine cap (compact; a represented sentence
// held in part keeps its partial form, register N17), all held by the
// published successor of a seat the gate withdrew (successor), all passed
// back to this row (returned), all left by a withdrawn seat with no successor
// (context). Every mixed disposition the gate leaves takes the see-detail
// form (detail), which names no holder. released is the form of a pairing
// phrase ("rides as that seat's lossless detail") once the row no longer
// rides as detail (rspaReleaseDetails).
type rspaClaimSpan struct {
	claims                                              []int
	held, compact, context, successor, returned, detail string
	released                                            string
	current                                             string
	represented                                         bool
	// inside / full: a represented span's account figures, from which the
	// resolver writes its partial and see-detail forms.
	inside, full float64
}

// rspaSpanForms are every form one claim phrase can take.
type rspaSpanForms struct {
	held, compact, context, successor, returned, detail, released string
	represented                                                   bool
	inside, full                                                  float64
}

func (f rspaSpanForms) span(claims []int) rspaClaimSpan {
	return rspaClaimSpan{claims: claims, held: f.held, compact: f.compact, context: f.context, successor: f.successor,
		returned: f.returned, detail: f.detail, released: f.released, current: f.held, represented: f.represented,
		inside: f.inside, full: f.full}
}

// rspaSentence is one engine sentence with the claims it makes, every claim
// span written in its held form (the resolution pass rewrites it when the
// holder is not on the published board).
type rspaSentence struct {
	text   string
	claims []RootCauseShareClaim
	spans  []rspaClaimSpan
}

// span registers one claim phrase of the sentence being built and returns
// its held form. Spans must be registered in the order their phrases appear
// in the text. A phrase without a claim is not a holder claim (returned as
// is, never rewritten).
func (s *rspaSentence) span(held, compact string, claims ...RootCauseShareClaim) string {
	return s.spanContext(held, compact, compact, claims...)
}

// spanContext is span with the form the phrase takes when its holder was
// withdrawn by the publication gate (published only as context). Every phrase
// whose claim can name a chain seat registers through it: the gate withdraws
// priority-inversion seats only, and those hold the chain-seat role.
func (s *rspaSentence) spanContext(held, compact, context string, claims ...RootCauseShareClaim) string {
	return s.spanForms(rspaSpanForms{held: held, compact: compact, context: context}, claims...)
}

// spanForms registers a claim phrase with every one of its forms.
func (s *rspaSentence) spanForms(f rspaSpanForms, claims ...RootCauseShareClaim) string {
	var idx []int
	for _, c := range claims {
		if c.Part == "" {
			continue
		}
		idx = append(idx, len(s.claims))
		s.claims = append(s.claims, c)
	}
	if len(idx) == 0 {
		return f.held
	}
	s.spans = append(s.spans, f.span(idx))
	return f.held
}

// spanOnClaim registers one more phrase governed by a claim the sentence
// already registered (claimIdx, from lastClaim) — the claim is not repeated,
// so the row's claims (and its share_claims note) are unchanged. Phrases must
// still be registered in the order they appear in the text.
func (s *rspaSentence) spanOnClaim(f rspaSpanForms, claimIdx int) string {
	if claimIdx < 0 || claimIdx >= len(s.claims) {
		return f.held
	}
	s.spans = append(s.spans, f.span([]int{claimIdx}))
	return f.held
}

// lastClaim is the index of the claim the last registration added (-1 when it
// added none: a phrase without a claim).
func (s *rspaSentence) lastClaim(before int) int {
	if len(s.claims) > before {
		return len(s.claims) - 1
	}
	return -1
}

// ownerSpan registers the anchored-share owner phrase of a ◇ remainder
// sentence (HMC377 round 11: with its successor and returned forms — a ◇ row
// whose anchored holder the gate withdraws carries that share; round 12: a
// share the gate left split takes the see-detail form).
func (s *rspaSentence) ownerSpan(owner RootCauseShareClaim) string {
	return s.spanForms(rspaSpanForms{held: rspaSummaryOwnedByChainSeat, compact: rspaSummaryOwnedByChainSeatUnpublished,
		context: rspaSummaryOwnedByChainSeatContext, successor: rspaSummaryOwnedBySuccessor, returned: rspaSummaryOwnedReturned,
		detail: rspaSummaryOwnedSeeDetail}, owner)
}

// representedSpan registers a whole "represented by" sentence over a
// satellite account of inside ms (of full): the row wears
// ChainAnchorRepresentedByChainSeat only while every claim resolves.
func (s *rspaSentence) representedSpan(held string, inside, full float64, claims ...RootCauseShareClaim) string {
	marked := make([]RootCauseShareClaim, len(claims))
	for i, c := range claims {
		c.Represented = c.Part != ""
		marked[i] = c
	}
	return s.spanForms(rspaSpanForms{held: held, compact: rspaRepresentedCompact(inside, full), context: rspaRepresentedContext(inside, full),
		successor: rspaRepresentedSuccessor(inside, full), represented: true, inside: inside, full: full}, marked...)
}

// plain wraps a sentence that makes no holder claim.
func rspaPlainSentence(text string) rspaSentence { return rspaSentence{text: text} }

// rspaSetSentence replaces a row's Summary (and the claims its former
// sentence made) with s.
func rspaSetSentence(item *RootCauseRankItem, s rspaSentence) {
	item.Summary = s.text
	item.ShareClaims = append([]RootCauseShareClaim(nil), s.claims...)
	rspaStampClaimPartitions(item, item.ShareClaims)
	item.rspaClaimSpans = append([]rspaClaimSpan(nil), s.spans...)
}

// rspaAppendSentence appends s to a row's Summary (and its claims).
func rspaAppendSentence(item *RootCauseRankItem, s rspaSentence) {
	item.Summary += s.text
	base := len(item.ShareClaims)
	added := append([]RootCauseShareClaim(nil), s.claims...)
	rspaStampClaimPartitions(item, added)
	item.ShareClaims = append(append([]RootCauseShareClaim(nil), item.ShareClaims...), added...)
	spans := append([]rspaClaimSpan(nil), item.rspaClaimSpans...)
	for _, sp := range s.spans {
		shifted := sp
		shifted.claims = make([]int, len(sp.claims))
		for i, c := range sp.claims {
			shifted.claims[i] = c + base
		}
		spans = append(spans, shifted)
	}
	item.rspaClaimSpans = spans
}

// rspaClearClaims drops the claims and held shares a copied row inherited
// from the row it was copied from (a copy that becomes another seat speaks
// and holds for itself only).
func rspaClearClaims(item *RootCauseRankItem) {
	item.ShareClaims, item.rspaClaimSpans, item.rspaHeld = nil, nil, nil
	item.ShareHolderKey = ""
	item.rspaCarvedAppliedMs, item.rspaCarvedEffective, item.rspaCarvedAccountMs = 0, false, 0
	item.RestrictedReadyAccountMs, item.ShareCountedElsewhereMs, item.ShareCountedHereMs = 0, 0, 0
}

// rspaHold records that a seat holds a share (one entry per role). A D/IO
// share is held in the seat's own cause partition (HMC377 fix round 6).
func rspaHold(item *RootCauseRankItem, role RootCauseShareHolderRole, dio bool, windows []foldInterval, ms float64) {
	held := make([]rspaHeldShare, 0, len(item.rspaHeld)+1)
	for _, h := range item.rspaHeld {
		if h.role != role || h.dio != dio {
			held = append(held, h)
		}
	}
	share := rspaHeldShare{role: role, dio: dio, windows: rspaFoldCopy(windows), ms: ms}
	if dio {
		share.partition = item.dioCausePartition
	}
	item.rspaHeld = append(held, share)
}

func rspaFoldCopy(windows []foldInterval) []foldInterval {
	if len(windows) == 0 {
		return nil
	}
	merged, _ := foldIntervalUnionWithDisjoint(windows)
	return merged
}

// rspaClaim builds one typed claim.
func rspaClaim(part RootCauseShareClaimPart, role RootCauseShareHolderRole, pid int, dio bool, ms float64, windows []foldInterval) RootCauseShareClaim {
	return RootCauseShareClaim{Part: part, Role: role, ShareMs: ms, pid: pid, dio: dio, windows: rspaFoldCopy(windows)}
}

// rspaFoldIntersect returns a ∩ b (both merged and sorted).
func rspaFoldIntersect(a, b []foldInterval) []foldInterval {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	return foldIntervalSubtract(a, foldIntervalSubtract(a, b))
}

func rspaFoldUnion(parts ...[]foldInterval) []foldInterval {
	var all []foldInterval
	for _, p := range parts {
		all = append(all, p...)
	}
	return rspaFoldCopy(all)
}

func rspaFoldLengthMs(fold []foldInterval) float64 {
	total := 0.0
	for _, iv := range fold {
		if iv.end > iv.start {
			total += (iv.end - iv.start) * 1000
		}
	}
	return total
}

// rspaRowInstants partitions one row's segment inventory into the instants
// inside the walked windows, the instants outside them that the unwalked
// share's owner holds (owned=false: none) and the rest. ok=false when the
// inventory does not reproduce the row's account (the row's claims then
// carry values only).
type rspaRowInstants struct {
	ok                  bool
	walked, owned, rest []foldInterval
}

func rspaInstantsOf(inventory []foldInterval, fullMs float64, walkedFold, ownerSupport []foldInterval) rspaRowInstants {
	if len(inventory) == 0 {
		return rspaRowInstants{}
	}
	merged, _ := foldIntervalUnionWithDisjoint(inventory)
	if length, ok := foldIntervalsLengthMs(merged); !ok || !rspaWithinTol(length, fullMs) {
		return rspaRowInstants{}
	}
	walked := rspaFoldIntersect(merged, walkedFold)
	outside := foldIntervalSubtract(merged, walkedFold)
	owned := rspaFoldIntersect(outside, rspaFoldCopy(ownerSupport))
	return rspaRowInstants{ok: true, walked: walked, owned: owned, rest: foldIntervalSubtract(outside, owned)}
}

// inside is the row's walked + owned instants (a case-B one object).
func (r rspaRowInstants) inside() []foldInterval {
	if !r.ok {
		return nil
	}
	return rspaFoldUnion(r.walked, r.owned)
}

func (r rspaRowInstants) walkedOrNil() []foldInterval {
	if !r.ok {
		return nil
	}
	return r.walked
}

func (r rspaRowInstants) ownedOrNil() []foldInterval {
	if !r.ok {
		return nil
	}
	return r.owned
}

func (r rspaRowInstants) restOrNil() []foldInterval {
	if !r.ok {
		return nil
	}
	return r.rest
}

// rspaFamilyValueMs is the value a seat publishes for one state family.
func rspaFamilyValueMs(item RootCauseRankItem, dio bool) float64 {
	if dio {
		return item.DStateMs + item.IOWaitMs
	}
	if strings.TrimSpace(item.Type) == "priority_inversion_candidate" {
		return item.GatedRunnableMs
	}
	return item.RunnableMs
}

// rspaChainSeatFamily classifies a chain-lane seat by the family whose
// account it publishes (the rspaChainSeatPresenceByPID roles).
func rspaChainSeatFamily(item RootCauseRankItem) (dio, ok bool) {
	if !strings.HasPrefix(strings.TrimSpace(item.Source), "wakeup_chain") || item.Thread.PID <= 0 || item.AbsorbedByRankFamily {
		return false, false
	}
	if strings.TrimSpace(item.Type) == "priority_inversion_candidate" {
		return false, item.GatedRunnableMs > 0
	}
	switch item.DominantState {
	case string(StateRunnable):
		return false, true
	case string(StateDSleep), string(StateIOWait):
		return true, true
	default:
		// running / sleep / stopped / dead / unknown chain seats publish no
		// runnable or D/IO account a share claim could name.
		return false, false
	}
}

// rspaChainSeatOwnWindows is a chain-lane seat's own published segment
// inventory (the XLANE-1 precise inventory: member segments, occurrence
// windows, or the single occurrence's own pair — never an envelope).
func rspaChainSeatOwnWindows(item RootCauseRankItem) []foldInterval {
	var out []foldInterval
	switch {
	case len(item.familyMemberIntervals) > 0:
		out = append(out, item.familyMemberIntervals...)
	case len(item.OccurrenceWindows) > 0:
		for _, occ := range item.OccurrenceWindows {
			out = append(out, foldInterval{start: occ.Window.StartTs, end: occ.Window.EndTs})
		}
	case item.MemberCount <= 1 && item.EndTs > item.StartTs:
		out = append(out, foldInterval{start: item.StartTs, end: item.EndTs})
	}
	return rspaFoldCopy(out)
}

// rspaStampChainSeatHolds records what each chain-lane seat of a re-anchored
// (thread, family) holds: the family's walked windows when it is the family's
// only chain seat, otherwise its own segment windows inside them. Recorded
// once per board (the build pass, before any truncation): a seat that
// survives a cap keeps what it held, it never inherits a dead seat's share.
func rspaStampChainSeatHolds(items []RootCauseRankItem, stats WindowStats, runnable, dio map[int]rspaFamilyDecision) {
	type key struct {
		pid int
		dio bool
	}
	count := map[key]int{}
	for i := range items {
		if d, ok := rspaChainSeatFamily(items[i]); ok {
			count[key{items[i].Thread.PID, d}]++
		}
	}
	for i := range items {
		item := &items[i]
		d, ok := rspaChainSeatFamily(*item)
		if !ok {
			continue
		}
		decisions := runnable
		if d {
			decisions = dio
		}
		if decision, has := decisions[item.Thread.PID]; !has || !decision.migrate {
			continue
		}
		if rspaHolds(*item, RootCauseShareHolderChainSeat, d) {
			continue
		}
		walked := rspaWindowsFold(stats.chainAnchorsByPID[item.Thread.PID])
		windows := walked
		if count[key{item.Thread.PID, d}] > 1 {
			windows = rspaFoldIntersect(rspaChainSeatOwnWindows(*item), walked)
		}
		rspaHold(item, RootCauseShareHolderChainSeat, d, windows, rspaFamilyValueMs(*item, d))
	}
}

func rspaHolds(item RootCauseRankItem, role RootCauseShareHolderRole, dio bool) bool {
	for _, h := range item.rspaHeld {
		if h.role == role && h.dio == dio {
			return true
		}
	}
	return false
}

// rspaHolderOnChainRole: every holder role but the ◇ remainder seat holds its
// share on the chain tier.
func rspaHolderOnChainRole(role RootCauseShareHolderRole) bool {
	return role != RootCauseShareHolderRemainderSeat && role != RootCauseShareHolderSubsetSeat
}

// resolveRankShareClaims is THE resolution pass: it runs once, after the
// final published board is known (board = the published rows; detail = the
// absorbed rows published as lossless detail, whose claims resolve against
// the same board). Every claim resolves by typed identity against the board's
// held shares; every span is written in the form its claims allow; holder
// keys are assigned to the seats that resolve a claim. Re-running it on the
// same board is a no-op; running it on a later board re-resolves from the
// claims (never from the text).
//
// HMC377 round 11 (batch-owner ruling R-377-11 ①): a claim resolves per held
// segment. Its resolved part (ResolvedMs) is the measure of its instants the
// seats on the board hold; a represented row publishes its own account less
// that part, so the rest returns to it. Every share relation of the board —
// the RSPA bipartitions, the represented satellites, the R-377-4 inversion
// details and the LEVELMERGE-1 constituent/residual pair — is decided here and
// nowhere else.
func resolveRankShareClaims(board []RootCauseRankItem, detail []RootCauseRankItem) {
	for i := range board {
		board[i].ShareHolderKey = ""
	}
	resolveRow := func(item *RootCauseRankItem) {
		if len(item.ShareClaims) == 0 {
			return
		}
		claims := append([]RootCauseShareClaim(nil), item.ShareClaims...)
		for ci := range claims {
			resolveShareClaim(&claims[ci], board, item)
		}
		item.ShareClaims = claims
		rspaWriteClaimSpans(item)
		rspaWriteCarvedShares(item)
	}
	for i := range board {
		resolveRow(&board[i])
	}
	for i := range detail {
		resolveRow(&detail[i])
	}
	// Holder keys: the board position of every seat that holds a claim's
	// resolved part.
	keyOf := func(i int) string {
		if board[i].ShareHolderKey == "" {
			board[i].ShareHolderKey = fmt.Sprintf("seat-%d", i+1)
		}
		return board[i].ShareHolderKey
	}
	assign := func(item *RootCauseRankItem) {
		for ci := range item.ShareClaims {
			c := &item.ShareClaims[ci]
			keys := make([]string, 0, len(c.holders()))
			for _, h := range c.holders() {
				keys = append(keys, keyOf(h))
			}
			c.HolderKeys = keys
		}
	}
	for i := range board {
		assign(&board[i])
	}
	for i := range detail {
		assign(&detail[i])
	}
}

// rspaSpanState is what a span's claims say after resolution (HMC377 rounds
// 11–12): whether every claim resolves, and the span's share per disposition
// — held by seats of the claims' roles on the published board (ownMs), held
// by the published successor of a seat the gate withdrew (succMs), passed
// back to this row (returnedMs), left by a withdrawn seat with no successor
// (withdrawnMs) and held by a seat the engine cap cut (capMs).
type rspaSpanState struct {
	resolved                                      bool
	ownMs, succMs, returnedMs, withdrawnMs, capMs float64
}

func rspaSpanStateOf(sp rspaClaimSpan, claims []RootCauseShareClaim) rspaSpanState {
	st := rspaSpanState{resolved: true}
	for _, ci := range sp.claims {
		if ci < 0 || ci >= len(claims) {
			st.resolved = false
			continue
		}
		c := claims[ci]
		st.resolved = st.resolved && c.Resolved
		st.ownMs += c.heldOwnMs
		st.succMs += c.heldSuccMs
		st.returnedMs += c.ReturnedMs
		st.withdrawnMs += c.restWithdrawnMs
		st.capMs += rspaClampNonNegative(c.ShareMs - c.heldOwnMs - c.heldSuccMs - c.ReturnedMs - c.restWithdrawnMs)
	}
	return st
}

// disposition is the span's one disposition when its whole share has one
// (R-377-12 ②); mixed=true otherwise. A span whose share no gate decision
// touched reads the engine run's two dispositions only (held / cap).
func (st rspaSpanState) gateTouched() bool {
	tol := rspaAnchorIdentityTolMs
	return st.succMs > tol || st.returnedMs > tol || st.withdrawnMs > tol
}

func (st rspaSpanState) single() (part string, ok bool) {
	tol := rspaAnchorIdentityTolMs
	n := 0
	for _, kv := range []struct {
		name string
		ms   float64
	}{{"own", st.ownMs}, {"succ", st.succMs}, {"returned", st.returnedMs}, {"withdrawn", st.withdrawnMs}, {"cap", st.capMs}} {
		if kv.ms > tol {
			n++
			part = kv.name
		}
	}
	return part, n == 1
}

// form is the text a span takes for its claims' state (HMC377 round 12,
// R-377-12 ②: a specific holder form only for one disposition; every mixed
// disposition the gate leaves takes the see-detail form).
func (sp rspaClaimSpan) form(st rspaSpanState) string {
	tol := rspaAnchorIdentityTolMs
	or := func(text, fallback string) string {
		if text != "" {
			return text
		}
		return fallback
	}
	if !st.gateTouched() {
		// The engine run's forms: held while every claim resolves; a
		// represented sentence held in part publishes its partial form (its
		// value counts only the rest, R-377-10 ③); otherwise the compaction
		// disclosure (register N17: the cap-cut wording is unchanged).
		switch {
		case st.resolved:
			return sp.held
		case sp.represented && st.ownMs > tol:
			return rspaRepresentedPartial(sp.inside, sp.full, st.ownMs, st.capMs)
		default:
			return sp.compact
		}
	}
	if part, ok := st.single(); ok {
		switch part {
		case "succ":
			return or(sp.successor, sp.context)
		case "returned":
			return or(sp.returned, sp.context)
		case "withdrawn":
			return sp.context
		}
	}
	if sp.represented {
		return rspaRepresentedSeeDetail(sp.inside, sp.full, st.ownMs+st.succMs)
	}
	return or(sp.detail, sp.context)
}

// rspaWriteClaimSpans writes every claim span of a row's Summary in the form
// its claims allow and writes the row's share accounts
// (rspaWriteShareAccounts). A pairing phrase of a row that no longer rides as
// lossless detail takes its released form (HMC377 round 12, R-377-12 ①).
func rspaWriteClaimSpans(item *RootCauseRankItem) {
	claims := item.ShareClaims
	spans := append([]rspaClaimSpan(nil), item.rspaClaimSpans...)
	cursor := 0
	for si := range spans {
		sp := &spans[si]
		want := sp.form(rspaSpanStateOf(*sp, claims))
		if item.rspaReleasedDetail && sp.released != "" {
			want = sp.released
		}
		if at := strings.Index(item.Summary[cursor:], sp.current); at >= 0 {
			at += cursor
			item.Summary = item.Summary[:at] + want + item.Summary[at+len(sp.current):]
			sp.current = want
			cursor = at + len(want)
		}
	}
	item.rspaClaimSpans = spans
	rspaWriteShareAccounts(item, claims)
}

// rspaWriteShareAccounts writes, from the typed claims alone (never the
// text), what the claims' resolution means for the row's own account — one
// carrier per instant (HMC377 rounds 10–11, R-377-10 ③, R-377-11 ①):
//   - a represented row is represented whole only while every claim of its
//     "represented by" sentence resolves; while some of its share is held on
//     the board and some is not, it publishes its own account less the held
//     part (ChainAnchorRepresentedHeldMs) — the rest returns to it;
//   - a ◇ remainder or a LEVELMERGE-1 residual seat that took back a share
//     whose holder the gate withdrew publishes its own account plus that
//     share (ShareReturnedMs);
//   - a LEVELMERGE-1 constituent row that took back its share carries it
//     with its own value (GatedShareClaimWithdrawn; its value is the share it
//     took back), and a residual seat whose carved share no published claim
//     seat counts says so (GatedShareClaimWithdrawn).
//
// The row's minted value channels are kept aside while any adjustment
// applies, so every run of the resolver writes from the same account.
//
// A carved claim (R-377-11 ②: a CPU-restriction seat's instants inside the
// chain object's windows, a ◇ remainder's instants under the restriction) is
// value-written by the carve accounting alone (rspaWriteCarvedShares): a
// share that passed back to its claimant does not resolve, so the carve
// writer already returns it. ShareReturnedMs still names it (the face's
// "this row counts it" clause), but this writer never adds it a second time
// (HMC377 round-11 merge: the naive merge published a restriction seat at its
// account plus the returned share).
func rspaWriteShareAccounts(item *RootCauseRankItem, claims []RootCauseShareClaim) {
	tol := rspaAnchorIdentityTolMs
	represented, sawRepresented, anyResolved := true, false, false
	heldElsewhere, returned, carvedReturned := 0.0, 0.0, 0.0
	var gated *RootCauseShareClaim
	for i := range claims {
		c := claims[i]
		returned += c.ReturnedMs
		if c.carved {
			carvedReturned += c.ReturnedMs
		}
		if c.Part == RootCauseShareClaimGatedShare && gated == nil {
			gated = &claims[i]
		}
		if !c.Represented {
			continue
		}
		sawRepresented = true
		switch {
		case c.Resolved:
			anyResolved = true
			heldElsewhere += c.ShareMs
		case c.ResolvedMs > tol:
			anyResolved = true
			represented = false
			heldElsewhere += c.ResolvedMs
		default:
			represented = false
		}
	}
	if returned <= tol {
		returned = 0
	}
	item.ShareReturnedMs = returned
	// valueReturned: the returned share this writer adds to the row's value.
	valueReturned := rspaClampNonNegative(returned - carvedReturned)
	if valueReturned <= tol {
		valueReturned = 0
	}
	less := 0.0
	switch {
	case sawRepresented:
		item.ChainAnchorRepresentedByChainSeat = represented
		if represented || !anyResolved {
			heldElsewhere = 0
		}
		if heldElsewhere <= tol {
			heldElsewhere = 0
		}
		item.ChainAnchorRepresentedHeldMs = heldElsewhere
		less = heldElsewhere
	case gated != nil && item.GatedShareConstituentSeat:
		item.GatedShareClaimWithdrawn = gated.ReturnedMs > tol
		if item.GatedShareClaimWithdrawn {
			minted := rspaValueChannelsOf(*item)
			if item.rspaMintedAccount != nil {
				minted = *item.rspaMintedAccount
			}
			less = minted.effective - gated.ReturnedMs
		}
	case gated != nil && gated.disclosure:
		item.GatedShareClaimWithdrawn = !gated.Resolved && gated.holderWithdrawn
	case gated != nil:
		// The residual seat: its carved share is counted by the published
		// successor (the constituent row) or taken back by this seat. The
		// split carved the share off the residual's published channels only —
		// its cumulative channel keeps the pre-split full account — so the
		// share taken back returns to those channels alone.
		item.GatedShareClaimWithdrawn = gated.Successor || gated.ReturnedMs > tol
		rspaWriteMintedLess(item, -valueReturned, false)
		return
	case item.ChainAnchorRemainderSeat:
		less = -valueReturned
	case item.rspaReleasedDetail:
		// HMC377 round 12 (R-377-12 ①): a whole-account row that no longer
		// rides as lossless detail (an R-377-4 inversion detail, a
		// restriction seat all inside one chain-object row) publishes its
		// account less the shares its claims still resolve to other rows on
		// the published board — the rest is counted here, once.
		for _, c := range claims {
			if c.disclosure || c.carved || c.Part == RootCauseShareClaimChainAccount {
				continue
			}
			less += c.heldOwnMs + c.heldSuccMs
		}
	}
	rspaWriteMintedLess(item, less, true)
}

// rspaValueChannels are a row's published value channels.
type rspaValueChannels struct {
	runnable, impact, projected, cumulative, effective, score float64
}

func rspaValueChannelsOf(item RootCauseRankItem) rspaValueChannels {
	return rspaValueChannels{runnable: item.RunnableMs, impact: item.ImpactMs, projected: item.ProjectedImpactMs,
		cumulative: item.CumulativeImpactMs, effective: item.EffectiveImpactMs, score: item.Score}
}

// write publishes v less heldMs on every value channel (a negative heldMs
// adds a share the row took back; the score scales with the effective
// account). cumulative=false leaves the cumulative channel as minted (a
// LEVELMERGE-1 residual's cumulative channel already is its full account).
func (v rspaValueChannels) write(item *RootCauseRankItem, heldMs float64, cumulative bool) {
	less := func(x float64) float64 {
		if x <= 0 {
			return x
		}
		return rspaClampNonNegative(x - heldMs)
	}
	item.RunnableMs, item.ImpactMs, item.ProjectedImpactMs = less(v.runnable), less(v.impact), less(v.projected)
	item.CumulativeImpactMs, item.EffectiveImpactMs = v.cumulative, less(v.effective)
	if cumulative {
		item.CumulativeImpactMs = less(v.cumulative)
	}
	item.Score = v.score
	if v.effective > 0 {
		item.Score = v.score * item.EffectiveImpactMs / v.effective
	}
}

// rspaWriteMintedLess publishes the row's minted account less lessMs (HMC377
// rounds 10–11): the minted channels are kept aside while lessMs ≠ 0 and
// restored when no adjustment applies.
func rspaWriteMintedLess(item *RootCauseRankItem, lessMs float64, cumulative bool) {
	if lessMs > rspaAnchorIdentityTolMs || lessMs < -rspaAnchorIdentityTolMs {
		if item.rspaMintedAccount == nil {
			minted := rspaValueChannelsOf(*item)
			item.rspaMintedAccount = &minted
		}
		item.rspaMintedAccount.write(item, lessMs, cumulative)
		return
	}
	if item.rspaMintedAccount != nil {
		item.rspaMintedAccount.write(item, 0, true)
		item.rspaMintedAccount = nil
	}
}

// WithdrawRankShareHolders (HMC377 round 10, batch-owner ruling R-377-10 ①)
// applies the trace_query publication gate's typed withdrawal decision to a
// published rank copy and returns the published board and lossless-detail
// rows. The gate (the tool's priority proof boundary) can publish a seat the
// engine resolved claims against as context only — no rank seat, no
// inversion type, no gated share. withdrawnBoard[i] / withdrawnDetail[i] is
// that gate's own decision for each published row (the precise typed signal;
// never a value, name or text match).
//
// A withdrawn row holds no share and makes no claim: its holds, claims, claim
// spans and split fields go with its seat. HMC377 round 11 (R-377-11 ①): each
// segment the withdrawn row held for a claim no other published seat covers
// passes to exactly one published successor (rspaPassWithdrawnSegments). Then
// the engine's one resolver (resolveRankShareClaims) re-runs on the published
// copy — no second resolution rule exists: the successor counts the segments
// it took, every other claim on them resolves to it, a claim another
// published seat still covers stays resolved, and a claim whose holder was
// withdrawn without a successor says the holding row is published only as
// context.
//
// HMC377 round 12 (batch-owner ruling R-377-12 ①): every "rides as lossless
// detail" relation is a share relation of the same resolver and is decided
// again here. A detail row rides as detail only while its absorber is
// published as a seat and its claims resolve to no instants of its own; any
// other detail returns to the board with its resolved account and its own
// words (rspaReleaseDetail), and the successor rule runs again with it on the
// board. A detail row is never a successor. A board with no withdrawn row is
// untouched (byte-identical, same slices). The rows are published copies:
// claims, spans and holds are replaced, never mutated in place, so the engine
// result is unchanged.
func WithdrawRankShareHolders(board, detail []RootCauseRankItem, withdrawnBoard, withdrawnDetail []bool) ([]RootCauseRankItem, []RootCauseRankItem) {
	gone := map[string]bool{}
	goneFamily := map[string]bool{}
	var holds []rspaWithdrawnHold
	withdrewAny := false
	for i := range board {
		if i >= len(withdrawnBoard) || !withdrawnBoard[i] {
			continue
		}
		withdrewAny = true
		if key := strings.TrimSpace(board[i].ShareHolderKey); key != "" {
			gone[key] = true
		}
		if key := strings.TrimSpace(board[i].RankFamilyKey); key != "" && board[i].AbsorbedRankRows > 0 {
			goneFamily[key] = true
		}
		for _, h := range board[i].rspaHeld {
			if !h.successor {
				holds = append(holds, rspaWithdrawnHold{pid: board[i].Thread.PID, hold: h})
			}
		}
		rspaWithdrawSeat(&board[i])
	}
	for i := range detail {
		if i < len(withdrawnDetail) && withdrawnDetail[i] {
			withdrewAny = true
			rspaWithdrawSeat(&detail[i])
		}
	}
	if !withdrewAny {
		return board, detail
	}
	mark := func(item *RootCauseRankItem) {
		named := false
		for _, c := range item.ShareClaims {
			for _, key := range c.HolderKeys {
				named = named || gone[strings.TrimSpace(key)]
			}
		}
		if !named {
			return
		}
		claims := append([]RootCauseShareClaim(nil), item.ShareClaims...)
		for ci := range claims {
			c := &claims[ci]
			for _, key := range c.HolderKeys {
				c.holderWithdrawn = c.holderWithdrawn || gone[strings.TrimSpace(key)]
			}
			if !c.holderWithdrawn {
				continue
			}
			// The claim's instants the withdrawn seats held: what the
			// successor rule may pass on, and what no successor took.
			var held []foldInterval
			for _, wh := range holds {
				if wh.pid != c.pid || !c.heldBy(wh.hold) {
					continue
				}
				if len(wh.hold.windows) == 0 {
					c.withdrawnWhole = true
				}
				held = append(held, wh.hold.windows...)
			}
			if len(c.windows) == 0 {
				c.withdrawnWhole = true
			}
			if c.withdrawnWhole {
				c.withdrawn = rspaFoldCopy(c.windows)
			} else {
				c.withdrawn = rspaFoldIntersect(c.windows, rspaFoldCopy(held))
			}
		}
		item.ShareClaims = claims
	}
	for i := range board {
		mark(&board[i])
	}
	for i := range detail {
		mark(&detail[i])
	}
	// The one resolver first runs on the published board without the
	// withdrawn holds (what every claim's other holders still cover), then
	// the withdrawn segments pass to their successors and it runs again; a
	// detail row the run leaves with instants of its own (or whose absorber
	// the gate withdrew) returns to the board and the rule runs again.
	nb := len(board)
	withdrawn := append([]bool(nil), withdrawnBoard...)
	for len(withdrawn) < nb {
		withdrawn = append(withdrawn, false)
	}
	// Each run of the successor rule starts from the holds the rows carry on
	// entry (a hold an earlier publication of this copy passed stays).
	base := make([][]rspaHeldShare, len(board))
	for i := range board {
		base[i] = board[i].rspaHeld
	}
	// The detail rows keep their published order (a row that rides as detail
	// again returns to its own position — the transport row cap reads it).
	detail = append([]RootCauseRankItem(nil), detail...)
	for i := range detail {
		detail[i].rspaDetailPos = i + 1
	}
	var keep, moved []RootCauseRankItem
	for _, d := range detail {
		if len(d.ShareClaims) > 0 && goneFamily[strings.TrimSpace(d.AbsorbedIntoFamily)] {
			moved = append(moved, d)
			continue
		}
		keep = append(keep, d)
	}
	for round := 0; ; round++ {
		if len(moved) > 0 {
			board = append(append([]RootCauseRankItem(nil), board...), moved...)
			for _, m := range moved {
				withdrawn = append(withdrawn, false)
				base = append(base, m.rspaHeld)
			}
			for i := len(board) - len(moved); i < len(board); i++ {
				rspaReleaseDetail(board, i)
			}
			detail, moved = keep, nil
		}
		for i := range board {
			board[i].rspaHeld = base[i]
		}
		resolveRankShareClaims(board, detail)
		if rspaPassWithdrawnSegments(board, holds, withdrawn) {
			resolveRankShareClaims(board, detail)
		}
		// A released row whose claims all resolve to one other published
		// row and that carries nothing of its own rides as that row's
		// lossless detail again — its absorber is on the published board and
		// it owns no instant (never a zero-valued second object).
		reattached := false
		for i := len(board) - 1; i >= nb; i-- {
			holder, ok := rspaReleasedDetailAbsorber(board, i, withdrawn)
			if !ok {
				continue
			}
			item := board[i]
			board = append(append([]RootCauseRankItem(nil), board[:i]...), board[i+1:]...)
			withdrawn = append(append([]bool(nil), withdrawn[:i]...), withdrawn[i+1:]...)
			base = append(append([][]rspaHeldShare(nil), base[:i]...), base[i+1:]...)
			if holder > i {
				holder--
			}
			rspaReattachDetail(&item, &board[holder])
			detail = append(append([]RootCauseRankItem(nil), detail...), item)
			reattached = true
			break // board positions moved: the next round resolves afresh
		}
		keep = nil
		for _, d := range detail {
			if rspaDetailOwnsInstants(d) {
				moved = append(moved, d)
				continue
			}
			keep = append(keep, d)
		}
		if (len(moved) == 0 && !reattached) || round > len(detail)+len(board) {
			detail = append(keep, moved...)
			break
		}
	}
	sort.SliceStable(detail, func(a, b int) bool { return detail[a].rspaDetailPos < detail[b].rspaDetailPos })
	for i := nb; i < len(board); i++ {
		item := &board[i]
		item.Rank, item.BackgroundRank, item.Tier = 0, 0, "tertiary"
		if rootCauseEffectiveImpactMs(*item) <= rspaAnchorIdentityTolMs {
			item.Tier = RootCauseTierContextOnly
		}
	}
	return board, detail
}

// rspaReleasedDetailAbsorber (HMC377 round 12, R-377-12 ①): board[i], a
// released detail row, owns no instant after the re-run (nothing returned to
// it, nothing a withdrawn seat left unheld, its resolved account zero) and
// every claim it makes resolves to the same one published row — the row it
// rides as lossless detail of. ok=false otherwise.
func rspaReleasedDetailAbsorber(board []RootCauseRankItem, i int, withdrawn []bool) (int, bool) {
	item := board[i]
	if !item.rspaReleasedDetail || rspaDetailOwnsInstants(item) || len(item.ShareClaims) == 0 ||
		rootCauseEffectiveImpactMs(item) > rspaAnchorIdentityTolMs || item.RunnableMs > rspaAnchorIdentityTolMs {
		return 0, false
	}
	holder := -1
	for _, c := range item.ShareClaims {
		if c.disclosure || !c.Resolved {
			return 0, false
		}
		for _, h := range c.holders() {
			if h == i || h < 0 || h >= len(board) || (h < len(withdrawn) && withdrawn[h]) || (holder >= 0 && h != holder) {
				return 0, false
			}
			holder = h
		}
	}
	return holder, holder >= 0
}

// rspaReattachDetail makes item the lossless detail of absorber: it keeps
// every value it was minted with and rides under the absorber's family key
// (the B4 absorbed carrier the projection folds by).
func rspaReattachDetail(item, absorber *RootCauseRankItem) {
	if item.rspaMintedAccount != nil {
		item.rspaMintedAccount.write(item, 0, true)
		item.rspaMintedAccount = nil
	}
	key := strings.TrimSpace(absorber.RankFamilyKey)
	if key == "" {
		key = item.rspaReleasedFrom
		absorber.RankFamilyKey = key
	}
	absorber.AbsorbedRankRows++
	item.Rank, item.BackgroundRank, item.Tier = 0, 0, RootCauseTierAbsorbed
	item.AbsorbedByRankFamily, item.AbsorbedIntoFamily, item.rspaReleasedDetail = true, key, false
}

// rspaReleaseDetail (HMC377 round 12, R-377-12 ①) returns board[i], a former
// lossless-detail row, to the published board: it no longer wears the absorbed
// markers, its absorber counts one detail row less, and its pairing phrases
// take their released form (rspaWriteClaimSpans) — its value is its resolved
// account (rspaWriteShareAccounts).
func rspaReleaseDetail(board []RootCauseRankItem, i int) {
	item := &board[i]
	key := strings.TrimSpace(item.AbsorbedIntoFamily)
	item.rspaReleasedFrom = key
	item.AbsorbedByRankFamily, item.AbsorbedIntoFamily, item.rspaReleasedDetail = false, "", true
	if key == "" {
		return
	}
	for j := range board {
		if j != i && board[j].AbsorbedRankRows > 0 && strings.TrimSpace(board[j].RankFamilyKey) == key {
			board[j].AbsorbedRankRows--
			if board[j].AbsorbedRankRows == 0 {
				board[j].RankFamilyKey = ""
			}
			return
		}
	}
}

// rspaDetailOwnsInstants reports whether a detail row's claims resolve to
// instants of its own after the successor rule: a share a withdrawn seat held
// that no published row took.
func rspaDetailOwnsInstants(item RootCauseRankItem) bool {
	own := 0.0
	for _, c := range item.ShareClaims {
		if !c.disclosure {
			own += c.ReturnedMs + c.restWithdrawnMs
		}
	}
	return own > rspaAnchorIdentityTolMs
}

// rspaWithdrawnHold is one share a gate-withdrawn seat held.
type rspaWithdrawnHold struct {
	pid  int
	hold rspaHeldShare
}

// rspaSuccessorTier is the typed order in which the published claimants of a
// withdrawn seat's segments take them (HMC377 round 11, R-377-11 ①): first a
// claimant whose claim covers those instants as its own carved share — the
// LEVELMERGE-1 constituent row (1), a cause-specific subset seat's claim on
// its own restricted instants (2), the ◇ remainder's anchored claim (3), the
// residual seat taking back its carved share (4) — else the claimant itself:
// a represented row (5). 0: never a successor (a value-free disclosure, a
// chain seat's own account, a part no withdrawn seat holds, a whole-account
// row whose value already carries the share).
//
// Tier 2 (HMC377 round-11 merge, R-377-11 ②: the cause-specific seat keeps
// its instants): a CPU-restriction seat's claim on its own restricted
// instants inside the withdrawn row's windows comes before the thread's ◇
// remainder, whose anchored claim covers the same instants — so the seat
// carries them whatever the two rows' board order (a value order), and the
// remainder takes the rest of the withdrawn row's segments. HMC377 round 12:
// the seat is typed by its restricted instants (the carve's own inventory),
// whichever split fields the carve writes.
//
// HMC377 round 12 (R-377-12 ①): a lossless-detail row is never a successor —
// the former tier 6 handed withdrawn segments to an absorbed row that holds
// no ◎ seat. A detail the rule leaves with instants of its own returns to the
// board instead, and takes part there at its own tier.
func rspaSuccessorTier(row RootCauseRankItem, c RootCauseShareClaim) int {
	switch {
	case c.disclosure || c.Part == RootCauseShareClaimChainAccount:
		return 0
	case c.Part == RootCauseShareClaimGatedShare && row.GatedShareConstituentSeat:
		return 1
	case (c.Part == RootCauseShareClaimAnchored || c.Part == RootCauseShareClaimUnwalked) && len(row.cpuConstraintRestrictedIntervals) > 0:
		return 2
	case c.Part == RootCauseShareClaimAnchored && row.ChainAnchorRemainderSeat:
		return 3
	case c.Part == RootCauseShareClaimGatedShare:
		return 4
	case c.Represented:
		return 5
	}
	return 0
}

// rspaPassWithdrawnSegments is the successor rule of R-377-11 ①: every
// segment a gate-withdrawn seat held for a published claim that no other
// published seat of the claim's role covers passes to exactly one published
// successor on the board, in rspaSuccessorTier order (board row, then claim,
// order within a tier). The successor holds the segments it took for every
// claim of the thread and family in one of the withdrawn seat's roles, so
// each such claim then resolves to it; its own claim on them counts them as
// returned. A value-only claim (no instants) that no published holder covers
// any more passes whole the same way. Reports whether any segment passed.
func rspaPassWithdrawnSegments(board []RootCauseRankItem, holds []rspaWithdrawnHold, withdrawnBoard []bool) bool {
	type family struct {
		pid           int
		dio, semantic bool
	}
	roles := map[family][]RootCauseShareHolderRole{}
	for _, wh := range holds {
		f := family{wh.pid, wh.hold.dio, wh.hold.semantic}
		seen := false
		for _, r := range roles[f] {
			seen = seen || r == wh.hold.role
		}
		if !seen {
			roles[f] = append(roles[f], wh.hold.role)
		}
	}
	type candidate struct {
		f          family
		tier       int
		row, claim int
		segs       []foldInterval
		valueOnly  bool
		partition  rspaDIOCausePartition
		shareMs    float64
	}
	var cands []candidate
	{
		rows, withdrawn := board, withdrawnBoard
		for ri := range rows {
			if ri < len(withdrawn) && withdrawn[ri] {
				continue
			}
			for ci, c := range rows[ri].ShareClaims {
				if !c.holderWithdrawn || c.Resolved {
					continue
				}
				tier := rspaSuccessorTier(rows[ri], c)
				if tier == 0 {
					continue
				}
				var held []foldInterval
				matched, windowless := false, false
				for _, wh := range holds {
					h := wh.hold
					if wh.pid != c.pid || h.role != c.Role || h.dio != c.dio || h.semantic != c.semantic ||
						h.ms <= rspaAnchorIdentityTolMs || !c.partition.admits(h) {
						continue
					}
					matched = true
					if len(h.windows) == 0 {
						windowless = true
					}
					held = append(held, h.windows...)
				}
				if !matched {
					continue
				}
				cand := candidate{f: family{c.pid, c.dio, c.semantic}, tier: tier, row: ri, claim: ci,
					partition: c.partition, shareMs: c.ShareMs}
				if len(c.windows) == 0 {
					// A claim without instants resolves whole or not at all
					// (resolveShareClaim): an unresolved one passes whole.
					cand.valueOnly = true
				} else {
					// HMC377 round 12 (register R11T-2, mutant P06): the
					// instants a published seat of the claim's role still
					// holds are not subtracted here — the claimant's returned
					// share is clamped to the part no published holder holds
					// (resolveShareClaim), so no reachable board told the
					// two apart and the subtraction is removed.
					cand.segs = c.windows
					if !windowless {
						cand.segs = rspaFoldIntersect(c.windows, rspaFoldCopy(held))
					}
					if rspaFoldLengthMs(cand.segs) <= rspaAnchorIdentityTolMs {
						continue
					}
				}
				cands = append(cands, cand)
			}
		}
	}
	if len(cands) == 0 {
		return false
	}
	sort.SliceStable(cands, func(a, b int) bool {
		x, y := cands[a], cands[b]
		if x.tier != y.tier {
			return x.tier < y.tier
		}
		if x.row != y.row {
			return x.row < y.row
		}
		return x.claim < y.claim
	})
	remaining := map[family][]foldInterval{}
	for _, c := range cands {
		if !c.valueOnly {
			remaining[c.f] = rspaFoldUnion(remaining[c.f], c.segs)
		}
	}
	valueTaken := map[family]bool{}
	passed := false
	for _, c := range cands {
		share := rspaHeldShare{dio: c.f.dio, semantic: c.f.semantic, partition: c.partition, successor: true, successorRoles: roles[c.f]}
		if c.valueOnly {
			if valueTaken[c.f] {
				continue
			}
			valueTaken[c.f] = true
			share.ms = c.shareMs
		} else {
			take := rspaFoldIntersect(remaining[c.f], c.segs)
			if rspaFoldLengthMs(take) <= rspaAnchorIdentityTolMs {
				continue
			}
			remaining[c.f] = foldIntervalSubtract(remaining[c.f], take)
			share.windows, share.ms = take, rspaFoldLengthMs(take)
		}
		row := &board[c.row]
		row.rspaHeld = append(append([]rspaHeldShare(nil), row.rspaHeld...), share)
		passed = true
	}
	return passed
}

// rspaWithdrawSeat drops what a gate-withdrawn row held and claimed: its
// holds, claims and spans, its holder key, and the split fields that described
// its seat's share of another seat's account (two-way / three-way / divergent
// split, represented form, gated-share split). The row's own measures stay —
// in particular a gated composite seat's refusal record
// (GatedCompositeEdge*), which splits the row's own ready time at the
// analysed thread's waits for disclosure and names no holder.
func rspaWithdrawSeat(item *RootCauseRankItem) {
	if item.rspaMintedAccount != nil {
		// A row that published its own account less a share counted
		// elsewhere (or plus a share it took back) makes no claim now: it
		// publishes its own account again.
		item.rspaMintedAccount.write(item, 0, true)
	}
	// Likewise a row that handed carved shares to other seats.
	rspaApplyCarvedShares(item, 0)
	item.ShareClaims, item.rspaClaimSpans, item.rspaHeld = nil, nil, nil
	item.ShareHolderKey = ""
	item.ChainAnchoredMs, item.ChainAnchorFullMs, item.ChainAnchorRemainderSeat = 0, 0, false
	item.ChainAnchorUnwalkedMs, item.ChainAnchorOutsideMs, item.ChainAnchorUnwalkedSeat = 0, 0, false
	item.ChainAnchorOwnershipDivergent, item.ChainAnchorChainLaneMs, item.ChainAnchorCensusMs = false, 0, 0
	item.ChainAnchorRepresentedByChainSeat, item.ChainAnchorRepresentedHeldMs, item.rspaMintedAccount = false, 0, nil
	item.GatedShareClaimedMs, item.GatedShareFullMs, item.GatedShareConstituentSeat = 0, 0, false
	item.GatedShareClaimSeats, item.GatedShareOverlapDisclosureMs, item.GatedShareClaimWithdrawn = nil, 0, false
	item.ShareReturnedMs = 0
	// HMC377 round 12 (R-377-12 ③): the restriction subset's typed fields go
	// with its seat too.
	item.RestrictedReadyAccountMs, item.ShareCountedElsewhereMs, item.ShareCountedHereMs = 0, 0, 0
	item.rspaCarvedAccountMs = 0
}

// holders re-derives the board indexes a resolved claim named (kept on the
// claim as keys only; the index list is transient).
func (c RootCauseShareClaim) holders() []int { return c.holderIdx }

// heldBy reports whether hold h holds a share of claim c's thread family:
// the claim's role (a successor hold: one of the withdrawn seat's roles),
// D/IO and semantic family and cause partition.
func (c RootCauseShareClaim) heldBy(h rspaHeldShare) bool {
	if h.dio != c.dio || h.semantic != c.semantic || h.ms <= rspaAnchorIdentityTolMs || !c.partition.admits(h) {
		return false
	}
	if h.successor {
		return h.holdsRole(c.Role)
	}
	return h.role == c.Role
}

// resolveShareClaim resolves one claim (made by row self) against the board:
// the seats of the claim's thread holding a share of the claim's role, family
// and D/IO cause partition must hold the claim's instants (a seat without a
// segment inventory contributes its value), or — for a claim without instants
// — its value.
//
// HMC377 round 11 (R-377-11 ①): per held segment. ResolvedMs is the measure
// of the claim's instants the holders cover (the share whole when it
// resolves); the holders of that part are named even when the claim does not
// resolve whole (the represented arm and the successor rule read them; a
// value-only claim resolves whole or not at all). A successor hold — instants
// a gate-withdrawn seat held that passed to a published row — holds the
// claim on any row but the claimant's own, where it is the share returned to
// the claimant (ReturnedMs).
func resolveShareClaim(c *RootCauseShareClaim, board []RootCauseRankItem, self *RootCauseRankItem) {
	c.Resolved, c.Exact, c.Successor, c.holderIdx = false, false, false, nil
	c.ResolvedMs, c.ReturnedMs, c.covered = 0, 0, nil
	c.heldOwnMs, c.heldSuccMs, c.restWithdrawnMs = 0, 0, 0
	if c.pid <= 0 || c.ShareMs <= rspaAnchorIdentityTolMs {
		return
	}
	need := c.ShareMs
	if len(c.windows) > 0 {
		need = rspaFoldLengthMs(c.windows)
	}
	scale := func(ms float64) float64 {
		if need <= 0 {
			return 0
		}
		return c.ShareMs * math.Min(1, ms/need)
	}
	// What passed back to the claimant itself.
	var own []foldInterval
	if self != nil && !c.disclosure {
		ownValue := 0.0
		for _, h := range self.rspaHeld {
			if !h.successor || !c.heldBy(h) {
				continue
			}
			ownValue += h.ms
			own = append(own, h.windows...)
		}
		own = rspaFoldCopy(own)
		switch {
		case len(c.windows) > 0 && len(own) > 0:
			c.ReturnedMs = scale(foldIntervalIntersectionMs(own, c.windows))
		case len(c.windows) == 0 && ownValue > rspaAnchorIdentityTolMs:
			c.ReturnedMs = math.Min(ownValue, c.ShareMs)
		}
	}
	// restWithdrawn writes the part of the share a withdrawn seat held that
	// neither a holder on the board nor this row carries (R-377-12 ②: the rest
	// is worded per segment, never by the claim-level withdrawal flag).
	restWithdrawn := func(held []foldInterval) {
		switch {
		case len(c.windows) > 0 && !c.withdrawnWhole:
			rest := foldIntervalSubtract(foldIntervalSubtract(rspaFoldCopy(c.withdrawn), rspaFoldCopy(held)), own)
			c.restWithdrawnMs = scale(rspaFoldLengthMs(rest))
		case c.withdrawnWhole:
			c.restWithdrawnMs = rspaClampNonNegative(c.ShareMs - c.ResolvedMs - c.ReturnedMs)
		}
	}
	var windowed, windowedOwn, windowedSucc []foldInterval
	windowlessMs, windowlessSuccMs, valueMs := 0.0, 0.0, 0.0
	var holders []int
	viaSuccessor, placed := map[int]bool{}, map[int]bool{}
	for i := range board {
		row := &board[i]
		if row.Thread.PID != c.pid || row.AbsorbedByRankFamily {
			continue
		}
		onChain := rootCauseItemIsOnChain(*row)
		// What this seat holds in the claim's role and family (a merged
		// seat holds what each member held).
		var heldWindows, succWindows []foldInterval
		heldMs, windowless, successor, ownRole := 0.0, false, false, false
		for _, h := range row.rspaHeld {
			if !c.heldBy(h) {
				continue
			}
			if h.successor {
				if row == self {
					continue
				}
				successor = true
				succWindows = append(succWindows, h.windows...)
			} else {
				// HMC377 fix round 6: a D/IO cause partition's share is
				// held only in that partition (or by a seat holding for the
				// whole thread) — never by another partition's seat of equal
				// value (admits, inside heldBy).
				if rspaHolderOnChainRole(c.Role) && !onChain {
					continue
				}
				ownRole = true
			}
			heldMs += h.ms
			if len(h.windows) == 0 {
				windowless = true
			}
			heldWindows = append(heldWindows, h.windows...)
		}
		if heldMs <= rspaAnchorIdentityTolMs {
			continue
		}
		switch {
		case len(c.windows) > 0 && !windowless:
			heldWindows = rspaFoldCopy(heldWindows)
			if foldIntervalIntersectionMs(heldWindows, c.windows) <= rspaAnchorIdentityTolMs {
				continue
			}
			windowed = append(windowed, heldWindows...)
			windowedSucc = append(windowedSucc, succWindows...)
			if ownRole && !successor {
				windowedOwn = append(windowedOwn, heldWindows...)
			}
		default:
			windowlessMs += heldMs
			if successor {
				windowlessSuccMs += heldMs
			}
		}
		valueMs += heldMs
		holders = append(holders, i)
		viaSuccessor[i] = successor
		placed[i] = len(c.windows) > 0 && !windowless
	}
	if len(holders) == 0 {
		restWithdrawn(nil)
		return
	}
	covered := valueMs
	tol := rspaAnchorIdentityTolMs
	windowedCover := 0.0
	if len(c.windows) > 0 {
		windowedCover = foldIntervalIntersectionMs(rspaFoldCopy(windowed), c.windows)
		covered = windowedCover + windowlessMs
		tol *= float64(1 + len(c.windows))
		c.covered = rspaFoldIntersect(rspaFoldCopy(windowedOwn), c.windows)
	}
	switch {
	case covered >= need-tol:
		c.Resolved, c.ResolvedMs = true, c.ShareMs
		c.Exact = len(holders) == 1 && rspaWithinTol(rspaFamilyValueMs(board[holders[0]], c.dio), c.ShareMs)
	case len(c.windows) > 0 && windowedCover > rspaAnchorIdentityTolMs:
		// Resolved in part: only the holders that cover instants name the
		// held part (a value never places instants).
		c.ResolvedMs = scale(windowedCover)
		kept := holders[:0:0]
		for _, h := range holders {
			if placed[h] {
				kept = append(kept, h)
			}
		}
		holders = kept
		windowlessMs, windowlessSuccMs = 0, 0
	default:
		restWithdrawn(nil)
		return
	}
	if c.ResolvedMs+c.ReturnedMs > c.ShareMs {
		c.ReturnedMs = rspaClampNonNegative(c.ShareMs - c.ResolvedMs)
	}
	c.holderIdx = holders
	for _, h := range holders {
		c.Successor = c.Successor || viaSuccessor[h]
	}
	// The held part per disposition: the instants only a withdrawn seat's
	// successor holds (successor holds are written by the publication's
	// successor rule alone — zero on every engine run), the rest held by
	// seats of the claim's role.
	switch {
	case len(c.windows) > 0:
		succOnly := foldIntervalSubtract(rspaFoldIntersect(rspaFoldCopy(windowedSucc), c.windows), rspaFoldCopy(windowedOwn))
		c.heldSuccMs = scale(rspaFoldLengthMs(succOnly)) + windowlessSuccMs
	case valueMs > 0:
		c.heldSuccMs = c.ResolvedMs * windowlessSuccMs / valueMs
	}
	if c.heldSuccMs <= rspaAnchorIdentityTolMs {
		c.heldSuccMs = 0
	}
	c.heldSuccMs = math.Min(c.heldSuccMs, c.ResolvedMs)
	c.heldOwnMs = rspaClampNonNegative(c.ResolvedMs - c.heldSuccMs)
	restWithdrawn(windowed)
}

// semanticEdgeAnchorInstants are a host-edge semantic seat's instants inside
// the waits its host's wakeup ended and outside them (HMC377 fix round 5).
func semanticEdgeAnchorInstants(item RootCauseRankItem) (inside, outside []foldInterval) {
	inside = item.semanticMemberIntervals
	if len(inside) == 0 && item.EndTs > item.StartTs {
		inside = []foldInterval{{start: item.StartTs, end: item.EndTs}}
	}
	outside = item.hostEdgeRemainderIntervals
	if len(outside) == 0 && item.hostEdgeRemainderEndTs > item.hostEdgeRemainderStartTs {
		outside = []foldInterval{{start: item.hostEdgeRemainderStartTs, end: item.hostEdgeRemainderEndTs}}
	}
	return rspaFoldCopy(inside), rspaFoldCopy(outside)
}

// semanticEdgeAnchorSeatClaims records what the ⛓ semantic seat holds (the
// span's projection inside those waits) and types its claim on the ◇
// remainder minted from it (rem).
func semanticEdgeAnchorSeatClaims(seat *RootCauseRankItem, rem RootCauseRankItem) {
	inside, outside := semanticEdgeAnchorInstants(*seat)
	seat.rspaHeld = []rspaHeldShare{{role: RootCauseShareHolderResidualSeat, semantic: true, windows: inside, ms: seat.ChainAnchoredMs}}
	claim := rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, seat.Thread.PID, false, rem.CumulativeImpactMs, outside)
	claim.semantic = true
	seat.ShareClaims = append(append([]RootCauseShareClaim(nil), seat.ShareClaims...), claim)
}

// rspaPatchSummariesForTwinVisibility is the board-only form of the
// resolution pass (no absorbed detail rows).
func rspaPatchSummariesForTwinVisibility(items []RootCauseRankItem) {
	resolveRankShareClaims(items, nil)
}
