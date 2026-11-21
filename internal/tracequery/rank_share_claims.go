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
	"strings"
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
)

// RootCauseShareHolderRole names the kind of seat a claim says holds a share.
type RootCauseShareHolderRole string

const (
	RootCauseShareHolderChainSeat     RootCauseShareHolderRole = "chain_seat"
	RootCauseShareHolderClippedSeat   RootCauseShareHolderRole = "clipped_seat"
	RootCauseShareHolderUnwalkedSeat  RootCauseShareHolderRole = "unwalked_seat"
	RootCauseShareHolderResidualSeat  RootCauseShareHolderRole = "residual_seat"
	RootCauseShareHolderRemainderSeat RootCauseShareHolderRole = "remainder_seat"
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

// rspaClaimSpan is one replaceable span of a row's Summary: its held form,
// its compaction-disclosure form, the claims that govern it and the form the
// Summary carries now.
type rspaClaimSpan struct {
	claims  []int
	held    string
	compact string
	// context (HMC377 round 10, R-377-10 ④): the form taken when every
	// unresolved claim of the span lost its holder to the publication gate —
	// the holding row is published only as context. The cap-cut compaction
	// form (compact) is unchanged.
	context     string
	current     string
	represented bool
	// inside / full: a represented span's account figures, from which the
	// resolver writes its partial form (some claims resolve, some do not).
	inside, full float64
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
	var idx []int
	for _, c := range claims {
		if c.Part == "" {
			continue
		}
		idx = append(idx, len(s.claims))
		s.claims = append(s.claims, c)
	}
	if len(idx) == 0 {
		return held
	}
	s.spans = append(s.spans, rspaClaimSpan{claims: idx, held: held, compact: compact, context: context, current: held})
	return held
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
	before := len(s.spans)
	out := s.spanContext(held, rspaRepresentedCompact(inside, full), rspaRepresentedContext(inside, full), marked...)
	if len(s.spans) > before {
		sp := &s.spans[len(s.spans)-1]
		sp.represented, sp.inside, sp.full = true, inside, full
	}
	return out
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
	return role != RootCauseShareHolderRemainderSeat
}

// resolveRankShareClaims is THE resolution pass: it runs once, after the
// final published board is known (board = the published rows; detail = the
// absorbed rows published as lossless detail, whose claims resolve against
// the same board). Every claim resolves by typed identity against the board's
// held shares; every span is written in the form its claims allow; holder
// keys are assigned to the seats that resolve a claim. Re-running it on the
// same board is a no-op; running it on a later board re-resolves from the
// claims (never from the text).
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
			resolveShareClaim(&claims[ci], board)
		}
		item.ShareClaims = claims
		rspaWriteClaimSpans(item)
	}
	for i := range board {
		resolveRow(&board[i])
	}
	for i := range detail {
		resolveRow(&detail[i])
	}
	// Holder keys: the board position of every seat that resolves a claim.
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

// rspaWriteClaimSpans writes every claim span of a row's Summary in the form
// its claims allow (the held form only while every claim it governs
// resolves) and keeps ChainAnchorRepresentedByChainSeat only while every
// claim of its "represented by" sentence resolves.
//
// HMC377 round 10 (R-377-10 ③④): an unresolved span whose every unresolved
// claim lost its holder to the publication gate takes its context form (the
// holding row is published only as context); a represented sentence that
// resolves only in part takes its partial form, and the row publishes its own
// account less the shares its resolved claims hand to holders on the board
// (ChainAnchorRepresentedHeldMs) — exactly one carrier per share. The row's
// minted value channels are kept aside while it is partial, so every run of
// the resolver writes from the same account.
func rspaWriteClaimSpans(item *RootCauseRankItem) {
	claims := item.ShareClaims
	spans := append([]rspaClaimSpan(nil), item.rspaClaimSpans...)
	cursor := 0
	for si := range spans {
		sp := &spans[si]
		resolved, anyResolved, withdrawnOnly := true, false, true
		resolvedMs, restMs := 0.0, 0.0
		for _, ci := range sp.claims {
			if ci < 0 || ci >= len(claims) {
				resolved, withdrawnOnly = false, false
				continue
			}
			if claims[ci].Resolved {
				anyResolved = true
				resolvedMs += claims[ci].ShareMs
				continue
			}
			resolved = false
			restMs += claims[ci].ShareMs
			withdrawnOnly = withdrawnOnly && claims[ci].holderWithdrawn
		}
		var want string
		switch {
		case resolved:
			want = sp.held
		case sp.represented && anyResolved:
			want = rspaRepresentedPartial(sp.inside, sp.full, resolvedMs, restMs, withdrawnOnly)
		case withdrawnOnly && sp.context != "":
			want = sp.context
		default:
			want = sp.compact
		}
		if at := strings.Index(item.Summary[cursor:], sp.current); at >= 0 {
			at += cursor
			item.Summary = item.Summary[:at] + want + item.Summary[at+len(sp.current):]
			sp.current = want
			cursor = at + len(want)
		}
	}
	item.rspaClaimSpans = spans
	// The represented state reads the typed claims (never the text): the row
	// is represented whole only while every claim of its "represented by"
	// sentence resolves, and in part while some do.
	represented, sawRepresented, anyResolved := true, false, false
	heldElsewhere := 0.0
	for _, c := range claims {
		if !c.Represented {
			continue
		}
		sawRepresented = true
		if c.Resolved {
			anyResolved = true
			heldElsewhere += c.ShareMs
		} else {
			represented = false
		}
	}
	if sawRepresented {
		item.ChainAnchorRepresentedByChainSeat = represented
		if represented || !anyResolved {
			heldElsewhere = 0
		}
		rspaWriteRepresentedHeld(item, heldElsewhere)
	}
}

// rspaValueChannels are a row's published value channels.
type rspaValueChannels struct {
	runnable, impact, projected, cumulative, effective, score float64
}

func rspaValueChannelsOf(item RootCauseRankItem) rspaValueChannels {
	return rspaValueChannels{runnable: item.RunnableMs, impact: item.ImpactMs, projected: item.ProjectedImpactMs,
		cumulative: item.CumulativeImpactMs, effective: item.EffectiveImpactMs, score: item.Score}
}

// write publishes v less heldMs on every value channel (the score scales with
// the effective account).
func (v rspaValueChannels) write(item *RootCauseRankItem, heldMs float64) {
	less := func(x float64) float64 {
		if x <= 0 {
			return x
		}
		return rspaClampNonNegative(x - heldMs)
	}
	item.RunnableMs, item.ImpactMs, item.ProjectedImpactMs = less(v.runnable), less(v.impact), less(v.projected)
	item.CumulativeImpactMs, item.EffectiveImpactMs = less(v.cumulative), less(v.effective)
	item.Score = v.score
	if v.effective > 0 {
		item.Score = v.score * item.EffectiveImpactMs / v.effective
	}
}

// rspaWriteRepresentedHeld applies the partial-represented account (HMC377
// round 10, R-377-10 ③): heldMs > 0 only while the row's represented sentence
// resolves in part.
func rspaWriteRepresentedHeld(item *RootCauseRankItem, heldMs float64) {
	if heldMs > rspaAnchorIdentityTolMs {
		if item.rspaRepresentedAccount == nil {
			minted := rspaValueChannelsOf(*item)
			item.rspaRepresentedAccount = &minted
		}
		item.rspaRepresentedAccount.write(item, heldMs)
		item.ChainAnchorRepresentedHeldMs = heldMs
		return
	}
	if item.rspaRepresentedAccount != nil {
		item.rspaRepresentedAccount.write(item, 0)
		item.rspaRepresentedAccount = nil
	}
	item.ChainAnchorRepresentedHeldMs = 0
}

// WithdrawRankShareHolders (HMC377 round 10, batch-owner ruling R-377-10 ①)
// applies the trace_query publication gate's typed withdrawal decision to a
// published rank copy. The gate (the tool's priority proof boundary) can
// publish a seat the engine resolved claims against as context only — no rank
// seat, no inversion type, no gated share. withdrawnBoard[i] /
// withdrawnDetail[i] is that gate's own decision for each published row (the
// precise typed signal; never a value, name or text match).
//
// A withdrawn row holds no share and makes no claim: its holds, claims, claim
// spans and split fields go with its seat. Then the engine's one resolver
// (resolveRankShareClaims) re-runs on the published copy — no second
// resolution rule exists: a claim another published seat still covers stays
// resolved (one carrier), a claim nobody covers is unresolved, and a claim
// whose engine-resolved holder was withdrawn says the holding row is published
// only as context. A LEVELMERGE-1 split whose every claim seat was withdrawn
// is marked GatedShareClaimWithdrawn (its constituent carries its own value).
// A board with no withdrawn row is untouched (byte-identical). The rows are
// published copies: claims and spans are replaced, never mutated in place, so
// the engine result is unchanged.
func WithdrawRankShareHolders(board, detail []RootCauseRankItem, withdrawnBoard, withdrawnDetail []bool) {
	gone := map[string]bool{}
	claimSeats := map[int]map[string]bool{}
	withdrewAny := false
	for i := range board {
		if i >= len(withdrawnBoard) || !withdrawnBoard[i] {
			continue
		}
		withdrewAny = true
		if key := strings.TrimSpace(board[i].ShareHolderKey); key != "" {
			gone[key] = true
		}
		if claimSeats[board[i].Thread.PID] == nil {
			claimSeats[board[i].Thread.PID] = map[string]bool{}
		}
		claimSeats[board[i].Thread.PID][fmt.Sprintf("%d..%d", board[i].LineStart, board[i].LineEnd)] = true
		rspaWithdrawSeat(&board[i])
	}
	for i := range detail {
		if i < len(withdrawnDetail) && withdrawnDetail[i] {
			withdrewAny = true
			rspaWithdrawSeat(&detail[i])
		}
	}
	if !withdrewAny {
		return
	}
	mark := func(item *RootCauseRankItem) {
		named := false
		for _, c := range item.ShareClaims {
			for _, key := range c.HolderKeys {
				named = named || gone[strings.TrimSpace(key)]
			}
		}
		if named {
			claims := append([]RootCauseShareClaim(nil), item.ShareClaims...)
			for ci := range claims {
				for _, key := range claims[ci].HolderKeys {
					claims[ci].holderWithdrawn = claims[ci].holderWithdrawn || gone[strings.TrimSpace(key)]
				}
			}
			item.ShareClaims = claims
		}
		levelMergeWithdrawClaimSeats(item, claimSeats[item.Thread.PID])
	}
	for i := range board {
		mark(&board[i])
	}
	for i := range detail {
		mark(&detail[i])
	}
	resolveRankShareClaims(board, detail)
}

// rspaWithdrawSeat drops what a gate-withdrawn row held and claimed: its
// holds, claims and spans, its holder key, and the split fields that described
// its seat's share of another seat's account (two-way / three-way / divergent
// split, represented form, gated-share split). The row's own measures stay —
// in particular a gated composite seat's refusal record
// (GatedCompositeEdge*), which splits the row's own ready time at the
// analysed thread's waits for disclosure and names no holder.
func rspaWithdrawSeat(item *RootCauseRankItem) {
	if item.rspaRepresentedAccount != nil {
		// A row that held its own account less a share counted elsewhere
		// makes no claim now: it publishes its own account again.
		item.rspaRepresentedAccount.write(item, 0)
	}
	item.ShareClaims, item.rspaClaimSpans, item.rspaHeld = nil, nil, nil
	item.ShareHolderKey = ""
	item.ChainAnchoredMs, item.ChainAnchorFullMs, item.ChainAnchorRemainderSeat = 0, 0, false
	item.ChainAnchorUnwalkedMs, item.ChainAnchorOutsideMs, item.ChainAnchorUnwalkedSeat = 0, 0, false
	item.ChainAnchorOwnershipDivergent, item.ChainAnchorChainLaneMs, item.ChainAnchorCensusMs = false, 0, 0
	item.ChainAnchorRepresentedByChainSeat, item.ChainAnchorRepresentedHeldMs, item.rspaRepresentedAccount = false, 0, nil
	item.GatedShareClaimedMs, item.GatedShareFullMs, item.GatedShareConstituentSeat = 0, 0, false
	item.GatedShareClaimSeats, item.GatedShareOverlapDisclosureMs, item.GatedShareClaimWithdrawn = nil, 0, false
}

// holders re-derives the board indexes a resolved claim named (kept on the
// claim as keys only; the index list is transient).
func (c RootCauseShareClaim) holders() []int { return c.holderIdx }

// resolveShareClaim resolves one claim against the board: the seats of the
// claim's thread holding a share of the claim's role, family and D/IO cause
// partition must hold the claim's instants (a seat without a segment
// inventory contributes its value), or — for a claim without instants — its
// value.
func resolveShareClaim(c *RootCauseShareClaim, board []RootCauseRankItem) {
	c.Resolved, c.Exact, c.holderIdx = false, false, nil
	if c.pid <= 0 || c.ShareMs <= rspaAnchorIdentityTolMs {
		return
	}
	var windowed []foldInterval
	windowlessMs, valueMs := 0.0, 0.0
	var holders []int
	for i := range board {
		row := board[i]
		if row.Thread.PID != c.pid || row.AbsorbedByRankFamily {
			continue
		}
		if rspaHolderOnChainRole(c.Role) && !rootCauseItemIsOnChain(row) {
			continue
		}
		// What this seat holds in the claim's role and family (a merged
		// seat holds what each member held).
		var heldWindows []foldInterval
		heldMs, windowless := 0.0, false
		for _, h := range row.rspaHeld {
			if h.role != c.Role || h.dio != c.dio || h.semantic != c.semantic || h.ms <= rspaAnchorIdentityTolMs {
				continue
			}
			// HMC377 fix round 6: a D/IO cause partition's share is held only
			// in that partition (or by a seat holding for the whole thread)
			// — never by another partition's seat of equal value.
			if !c.partition.admits(h) {
				continue
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
		default:
			windowlessMs += heldMs
		}
		valueMs += heldMs
		holders = append(holders, i)
	}
	if len(holders) == 0 {
		return
	}
	need, covered := c.ShareMs, valueMs
	tol := rspaAnchorIdentityTolMs
	if len(c.windows) > 0 {
		need = rspaFoldLengthMs(c.windows)
		covered = foldIntervalIntersectionMs(rspaFoldCopy(windowed), c.windows) + windowlessMs
		tol *= float64(1 + len(c.windows))
	}
	if covered < need-tol {
		return
	}
	c.Resolved, c.holderIdx = true, holders
	c.Exact = len(holders) == 1 && rspaWithinTol(rspaFamilyValueMs(board[holders[0]], c.dio), c.ShareMs)
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
