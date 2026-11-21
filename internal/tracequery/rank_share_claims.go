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
	claims      []int
	held        string
	compact     string
	current     string
	represented bool
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
	s.spans = append(s.spans, rspaClaimSpan{claims: idx, held: held, compact: compact, current: held})
	return held
}

// representedSpan registers a whole "represented by" sentence: the row wears
// ChainAnchorRepresentedByChainSeat only while every claim resolves.
func (s *rspaSentence) representedSpan(held, compact string, claims ...RootCauseShareClaim) string {
	marked := make([]RootCauseShareClaim, len(claims))
	for i, c := range claims {
		c.Represented = c.Part != ""
		marked[i] = c
	}
	before := len(s.spans)
	out := s.span(held, compact, marked...)
	if len(s.spans) > before {
		s.spans[len(s.spans)-1].represented = true
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
		spans := append([]rspaClaimSpan(nil), item.rspaClaimSpans...)
		cursor := 0
		represented, sawRepresented := true, false
		for si := range spans {
			sp := &spans[si]
			resolved := true
			for _, ci := range sp.claims {
				if ci < 0 || ci >= len(claims) || !claims[ci].Resolved {
					resolved = false
				}
			}
			want := sp.compact
			if resolved {
				want = sp.held
			}
			if at := strings.Index(item.Summary[cursor:], sp.current); at >= 0 {
				at += cursor
				item.Summary = item.Summary[:at] + want + item.Summary[at+len(sp.current):]
				sp.current = want
				cursor = at + len(want)
			}
			if sp.represented {
				sawRepresented = true
				represented = represented && resolved
			}
		}
		item.rspaClaimSpans = spans
		if sawRepresented {
			item.ChainAnchorRepresentedByChainSeat = represented
		}
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
