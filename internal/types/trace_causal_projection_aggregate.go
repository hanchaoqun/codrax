package types

// Deterministic pre-render aggregation for the Trace Causal Projection
// (presentation v3 §6, docs/design/trace_projection_presentation_v3_20260702.md).
//
// Real customer traces surfaced three presentation-poisoning duplication shapes
// that no renderer can fix row-by-row:
//   - the SAME wall-clock fact emitted under two predicates (an io_latency
//     primary row and its critical_blocking twin with identical ms + identical
//     evidence line range) rendering as two rows;
//   - the same (subject, cause) repeated many times with tiny values (six
//     sub-ms io_latency rows flooding the first screen);
//   - background rows whose impact point is the unknown-thread sentinel
//     flooding half the report.
//
// All rules here are STRICT, pure comparisons (user-adjudicated tolerance):
// R1 merges only on subject + projected ms equal at 3 decimals + identical
// evidence line range; R2 groups only on exactly-equal (subject, object); R3
// keys only on the unknown-thread sentinel. No ±ε approximation, no prose.
// Every merged row's observation id is retained (MergedEvidenceIDs), so the
// aggregation is lossless for auditability.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	// traceCausalProjectionSameKindAggregateMin is the R2 threshold: only ≥3
	// repeats of the same (subject, object) collapse into one ×N row — merging
	// two rows saves nothing and hides the repetition count.
	traceCausalProjectionSameKindAggregateMin = 3
	// traceCausalProjectionUnknownBackgroundKeep / Min: R3 keeps the top-K
	// unknown-impact-point background rows and folds the rest, but only when at
	// least two rows would fold (N ≥ Keep+2) — folding a single row is noise.
	traceCausalProjectionUnknownBackgroundKeep = 2
	traceCausalProjectionUnknownBackgroundMin  = traceCausalProjectionUnknownBackgroundKeep + 2
	// traceCausalProjectionSecondaryObjectCap bounds the 影响点 note list an R1
	// survivor accumulates; further merged views keep their evidence ids but do
	// not grow the display note.
	traceCausalProjectionSecondaryObjectCap = 3
	// traceCausalProjectionMergedSubjectCap bounds MergedSubjects on a merged
	// row: up to 4 distinct member thread names are preserved for display;
	// anything beyond is expressed through MergedCount (the evidence ids stay
	// lossless in MergedEvidenceIDs).
	traceCausalProjectionMergedSubjectCap = 4
)

func traceCausalProjectionAggregateForPresentation(out *TraceCausalProjection) {
	if out == nil {
		return
	}
	traceCausalProjectionMergeSameFacts(out)
	// R4 peer-alias fold runs between R1 (same-fact) and R2 (×N): the two alias
	// rows carry slightly different ms, so R1's strict identity never catches
	// them, and letting them reach R2 would risk a double-counting ×2 sum.
	out.PrimaryRootCauses = traceCausalProjectionMergePeerAliases(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionMergePeerAliases(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionMergePeerAliases(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionMergePeerAliases(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionMergePeerAliases(out.SupportingHops)
	// V4 duplicate-publication dedup MUST run before R2: three same-value
	// overlapping publications would otherwise reach the ≥3 threshold and SUM
	// into a 3× phantom total (customer revisit 2026-07-03: three 35.350ms
	// irq_activity rows over overlapping spans published as 106.05ms). After the
	// fold the survivor count is what R2 legitimately sees.
	out.PrimaryRootCauses = traceCausalProjectionDedupDuplicatePublications(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionDedupDuplicatePublications(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionDedupDuplicatePublications(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionDedupDuplicatePublications(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionDedupDuplicatePublications(out.SupportingHops)
	out.PrimaryRootCauses = traceCausalProjectionAggregateSameKind(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionAggregateSameKind(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionAggregateSameKind(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionAggregateSameKind(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionAggregateSameKind(out.SupportingHops)
	out.BackgroundCauses = traceCausalProjectionFoldUnknownBackground(out.BackgroundCauses)
	traceCausalProjectionResortAfterAggregation(out)
}

// --- R1: cross-predicate same-fact merge -----------------------------------

// traceCausalProjectionSameFactKey returns the strict identity of one observed
// wall-clock fact: subject + projected ms at 3 decimals + the exact evidence
// line range. Empty when the node lacks a line span or a positive projected
// value — such rows are never merged.
func traceCausalProjectionSameFactKey(node TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	impact := node.ImpactMS
	if impact <= 0 {
		impact = node.CumulativeImpactMS
	}
	if impact <= 0 {
		return ""
	}
	subject := traceCausalProjectionCanonicalNode(node.Subject)
	if subject == "" {
		return ""
	}
	return fmt.Sprintf("%s\x00%.3f\x00%d\x00%d", subject, impact, node.LineStart, node.LineEnd)
}

// traceCausalProjectionMergeSameFacts merges R1 duplicates across the
// projection buckets in priority order (primary → on-chain → hops → adjacent →
// background; semantic spans are deliberately excluded — a span is a different
// kind of fact than a state/cause row even at identical coordinates). The
// first occurrence in scan order survives; later occurrences with a DIFFERENT
// EvidenceID fold into it (same-EvidenceID hits are the survivor's own
// cross-bucket copy, which bucket-overlap semantics require keeping).
func traceCausalProjectionMergeSameFacts(out *TraceCausalProjection) {
	type survivorRef struct {
		bucket int
		index  int
	}
	buckets := []*[]TraceCausalProjectionNode{
		&out.PrimaryRootCauses,
		&out.OnChainCauses,
		&out.SupportingHops,
		&out.AdjacentCauses,
		&out.BackgroundCauses,
	}
	survivors := map[string]survivorRef{}
	merged := map[string]map[string]bool{} // fact key -> evidence ids already absorbed
	for b, bucket := range buckets {
		kept := (*bucket)[:0]
		for _, node := range *bucket {
			key := traceCausalProjectionSameFactKey(node)
			if key == "" {
				kept = append(kept, node)
				continue
			}
			ref, seen := survivors[key]
			if !seen {
				survivors[key] = survivorRef{bucket: b, index: len(kept)}
				merged[key] = map[string]bool{traceCausalProjectionCanonicalNode(node.EvidenceID): true}
				kept = append(kept, node)
				continue
			}
			survivor := &(*buckets[ref.bucket])[ref.index]
			if traceCausalProjectionCanonicalNode(node.EvidenceID) != "" &&
				traceCausalProjectionCanonicalNode(node.EvidenceID) == traceCausalProjectionCanonicalNode(survivor.EvidenceID) {
				// The survivor's own copy in another bucket — keep it so bucket
				// overlap semantics (and their consumers) stay intact.
				kept = append(kept, node)
				continue
			}
			traceCausalProjectionAbsorbSameFact(survivor, node, merged[key])
		}
		*bucket = kept
	}
}

func traceCausalProjectionAbsorbSameFact(survivor *TraceCausalProjectionNode, loser TraceCausalProjectionNode, absorbed map[string]bool) {
	appendEvidence := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		key := traceCausalProjectionCanonicalNode(id)
		if absorbed[key] {
			return
		}
		absorbed[key] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, id)
	}
	appendEvidence(loser.EvidenceID)
	for _, id := range loser.MergedEvidenceIDs {
		appendEvidence(id)
	}
	if object := strings.TrimSpace(loser.Object); object != "" &&
		traceCausalProjectionCanonicalNode(object) != traceCausalProjectionCanonicalNode(survivor.Object) {
		traceCausalProjectionAppendSecondaryObject(survivor, object)
	}
	for _, object := range loser.SecondaryObjects {
		traceCausalProjectionAppendSecondaryObject(survivor, object)
	}
	// Field back-fill: the merged views describe ONE fact, so empty typed slots
	// on the survivor take the loser's value; conflicting non-empty values keep
	// the survivor's (it won the priority scan).
	if survivor.StateKind == "" {
		survivor.StateKind = loser.StateKind
	}
	if survivor.SubjectKind == "" {
		survivor.SubjectKind = loser.SubjectKind
	}
	// VS-1 F6(b) (adversarial review 2026-07-04): a periodic-source survivor's
	// EffectiveImpactMS is the AUTHORITATIVE discounted attribution even at
	// exactly 0 (pure in-period cadence) — the merged twin is the raw-lane
	// view of the same fact, and backfilling its positive value would
	// resurrect the very sleep the discount removed. The survivor's periodic
	// triple (PeriodicSource/DetectedPeriodMS/PeriodicLatenessMS) likewise
	// stays exactly as the survivor published it (it won the priority scan);
	// a loser's periodic fields are never copied over. Precise boolean gate.
	if !survivor.PeriodicSource && survivor.EffectiveImpactMS <= 0 {
		survivor.EffectiveImpactMS = loser.EffectiveImpactMS
	}
	if survivor.ActualImpactMS <= 0 {
		survivor.ActualImpactMS = loser.ActualImpactMS
	}
	if survivor.UndrillableReason == "" {
		survivor.UndrillableReason = loser.UndrillableReason
	}
	if survivor.ChainDepth <= 0 {
		survivor.ChainDepth = loser.ChainDepth
	}
	// The dual-view case (a root_cause_primary row carrying the chain-cumulative
	// value merged with its per-hop twin): cumulative is the larger scope, keep
	// the max — both describe the same fact, max never invents a number.
	if loser.CumulativeImpactMS > survivor.CumulativeImpactMS {
		survivor.CumulativeImpactMS = loser.CumulativeImpactMS
	}
	if survivor.Confidence <= 0 {
		survivor.Confidence = loser.Confidence
	}
	if survivor.TypeToken == "" {
		survivor.TypeToken = loser.TypeToken
	}
}

// traceCausalProjectionAppendMergedSubject records one merged member's thread
// subject on the aggregate row (display roster for the fold/×N line): distinct
// by canonical key, real thread names only (the unknown sentinel and empty
// subjects carry no display value), capped at traceCausalProjectionMergedSubjectCap.
func traceCausalProjectionAppendMergedSubject(aggregate *TraceCausalProjectionNode, subject string) {
	subject = strings.TrimSpace(subject)
	if subject == "" || !traceCausalProjectionKnownSubject(subject) {
		return
	}
	if len(aggregate.MergedSubjects) >= traceCausalProjectionMergedSubjectCap {
		return
	}
	key := traceCausalProjectionCanonicalNode(subject)
	for _, existing := range aggregate.MergedSubjects {
		if traceCausalProjectionCanonicalNode(existing) == key {
			return
		}
	}
	aggregate.MergedSubjects = append(aggregate.MergedSubjects, subject)
}

func traceCausalProjectionAppendSecondaryObject(survivor *TraceCausalProjectionNode, object string) {
	object = strings.TrimSpace(object)
	if object == "" || len(survivor.SecondaryObjects) >= traceCausalProjectionSecondaryObjectCap {
		return
	}
	key := traceCausalProjectionCanonicalNode(object)
	if key == traceCausalProjectionCanonicalNode(survivor.Object) {
		return
	}
	for _, existing := range survivor.SecondaryObjects {
		if traceCausalProjectionCanonicalNode(existing) == key {
			return
		}
	}
	survivor.SecondaryObjects = append(survivor.SecondaryObjects, object)
}

// --- R4: peer-alias merge (customer audit 2026-07-03, H18) --------------------

// traceCausalProjectionMergePeerAliases folds the readfile_de E1/E2 shape: the
// SAME contention observed twice with the lock owner written two ways — once as
// a resolved thread label ("NetworkKit_AssetsUtil_Operate_0-42067") and once as
// the raw "pid=42067" handle. Two rows in one bucket merge when ALL of:
//   - canonical subject equal (same blocked thread),
//   - canonical TypeToken equal (same producer kind token),
//   - one row's BlockingPeer is the literal pid=N form (character-class check)
//     and the other's peer name carries the SAME integer N as its -pid tail
//     (integer equality, never substring),
//   - the two rows' own time spans overlap (boolean intersection; both spans
//     must be valid).
//
// The NAMED variant survives; the projected impact keeps the LARGER of the two
// measurements (both describe one wait — max never invents a number); evidence
// ids union losslessly.
func traceCausalProjectionMergePeerAliases(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	dropped := map[int]bool{}
	for i := 0; i < len(nodes); i++ {
		if dropped[i] {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if dropped[j] {
				continue
			}
			named, pidVariant, ok := traceCausalProjectionPeerAliasPair(&nodes[i], &nodes[j])
			if !ok {
				continue
			}
			traceCausalProjectionAbsorbPeerAlias(named, *pidVariant)
			if pidVariant == &nodes[i] {
				dropped[i] = true
			} else {
				dropped[j] = true
			}
			if dropped[i] {
				break
			}
		}
	}
	if len(dropped) == 0 {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(dropped))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		out = append(out, node)
	}
	return out
}

func traceCausalProjectionPeerAliasPair(a, b *TraceCausalProjectionNode) (named, pidVariant *TraceCausalProjectionNode, ok bool) {
	if traceCausalProjectionCanonicalNode(a.Subject) != traceCausalProjectionCanonicalNode(b.Subject) {
		return nil, nil, false
	}
	if traceCausalProjectionCanonicalNode(a.TypeToken) != traceCausalProjectionCanonicalNode(b.TypeToken) {
		return nil, nil, false
	}
	if !traceCausalProjectionSpansOverlap(*a, *b) {
		return nil, nil, false
	}
	if pid, isPid := traceCausalProjectionPidPeerForm(b.BlockingPeer); isPid {
		if n, hasTail := traceCausalProjectionNamePidTail(a.BlockingPeer); hasTail && n == pid {
			return a, b, true
		}
	}
	if pid, isPid := traceCausalProjectionPidPeerForm(a.BlockingPeer); isPid {
		if n, hasTail := traceCausalProjectionNamePidTail(b.BlockingPeer); hasTail && n == pid {
			return b, a, true
		}
	}
	return nil, nil, false
}

// traceCausalProjectionSpansOverlap is the boolean time-span intersection; both
// nodes must expose a valid span of their own.
func traceCausalProjectionSpansOverlap(a, b TraceCausalProjectionNode) bool {
	if a.StartTs <= 0 || a.EndTs <= a.StartTs || b.StartTs <= 0 || b.EndTs <= b.StartTs {
		return false
	}
	return a.StartTs < b.EndTs && b.StartTs < a.EndTs
}

// traceCausalProjectionPidPeerForm matches the literal "pid=N" peer handle
// (character-class check: the fixed prefix plus pure digits).
func traceCausalProjectionPidPeerForm(peer string) (int, bool) {
	peer = strings.TrimSpace(peer)
	if !strings.HasPrefix(peer, "pid=") {
		return 0, false
	}
	return traceCausalProjectionPureInt(strings.TrimPrefix(peer, "pid="))
}

// traceCausalProjectionNamePidTail extracts the integer -pid tail of a named
// thread label (non-empty name part, pure-digit tail after the last '-').
func traceCausalProjectionNamePidTail(peer string) (int, bool) {
	peer = strings.TrimSpace(peer)
	idx := strings.LastIndex(peer, "-")
	if idx <= 0 || idx == len(peer)-1 {
		return 0, false
	}
	return traceCausalProjectionPureInt(peer[idx+1:])
}

func traceCausalProjectionPureInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func traceCausalProjectionAbsorbPeerAlias(named *TraceCausalProjectionNode, pidVariant TraceCausalProjectionNode) {
	if pidVariant.ImpactMS > named.ImpactMS {
		named.ImpactMS = pidVariant.ImpactMS
	}
	if pidVariant.CumulativeImpactMS > named.CumulativeImpactMS {
		named.CumulativeImpactMS = pidVariant.CumulativeImpactMS
	}
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(named.EvidenceID): true}
	for _, id := range named.MergedEvidenceIDs {
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(raw)] = true
		named.MergedEvidenceIDs = append(named.MergedEvidenceIDs, raw)
	}
	appendID(pidVariant.EvidenceID)
	for _, id := range pidVariant.MergedEvidenceIDs {
		appendID(id)
	}
}

// --- V4: duplicate-publication dedup (pre-R2) ---------------------------------

// traceCausalProjectionDedupDuplicatePublications folds duplicate publications
// of ONE measurement inside a bucket (V4, customer revisit 2026-07-03): rows
// with the same canonical subject + object + TypeToken, EXACTLY equal positive
// projected ms (pure float equality, never ±ε) AND a precise line-range or
// time-span overlap describe one wall-clock fact republished N times. The
// first occurrence survives with the value UNCHANGED; DuplicatePublications
// counts the publications and evidence ids union losslessly. MergedCount is
// never touched — its ×N carries SUM semantics for genuinely distinct
// instances (three same-value NON-overlapping bursts stay separate and may
// legitimately R2-SUM). Value equality alone never folds; upstream ×N sum
// aggregates and same-EvidenceID copies are never folded.
func traceCausalProjectionDedupDuplicatePublications(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	dropped := map[int]bool{}
	folded := false
	for i := 0; i < len(nodes); i++ {
		if dropped[i] || nodes[i].MergedCount > 1 {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if dropped[j] || nodes[j].MergedCount > 1 {
				continue
			}
			if !traceCausalProjectionSameDuplicatePublication(nodes[i], nodes[j]) {
				continue
			}
			traceCausalProjectionAbsorbDuplicatePublication(&nodes[i], nodes[j])
			dropped[j] = true
			folded = true
		}
	}
	if !folded {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(dropped))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		out = append(out, node)
	}
	return out
}

// traceCausalProjectionSameDuplicatePublication is the strict identity of one
// republished measurement — the types-layer home of the identity the renderer's
// H6 display fold pioneered (kept aligned with the tool-layer isomorph
// runtimeTraceProjSameAdjacentMeasurement).
func traceCausalProjectionSameDuplicatePublication(a, b TraceCausalProjectionNode) bool {
	if traceCausalProjectionCanonicalNode(a.EvidenceID) != "" &&
		traceCausalProjectionCanonicalNode(a.EvidenceID) == traceCausalProjectionCanonicalNode(b.EvidenceID) {
		// The same observation's own copy — renderers dedupe by node key; a fold
		// here would fabricate a publication count.
		return false
	}
	return traceCausalProjectionCanonicalNode(a.Subject) == traceCausalProjectionCanonicalNode(b.Subject) &&
		traceCausalProjectionCanonicalNode(a.Object) == traceCausalProjectionCanonicalNode(b.Object) &&
		traceCausalProjectionCanonicalNode(a.TypeToken) == traceCausalProjectionCanonicalNode(b.TypeToken) &&
		a.ImpactMS > 0 && a.ImpactMS == b.ImpactMS &&
		(traceCausalProjectionLineSpansOverlap(a, b) || traceCausalProjectionSpansOverlap(a, b))
}

// traceCausalProjectionLineSpansOverlap is the boolean line-range intersection;
// both nodes must expose a valid range of their own (same guard style as the
// time-span twin traceCausalProjectionSpansOverlap).
func traceCausalProjectionLineSpansOverlap(a, b TraceCausalProjectionNode) bool {
	if a.LineStart <= 0 || a.LineEnd < a.LineStart || b.LineStart <= 0 || b.LineEnd < b.LineStart {
		return false
	}
	return a.LineStart <= b.LineEnd && b.LineStart <= a.LineEnd
}

func traceCausalProjectionAbsorbDuplicatePublication(survivor *TraceCausalProjectionNode, dup TraceCausalProjectionNode) {
	if survivor.DuplicatePublications < 1 {
		survivor.DuplicatePublications = 1
	}
	add := dup.DuplicatePublications
	if add < 1 {
		add = 1
	}
	survivor.DuplicatePublications += add
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(survivor.EvidenceID): true}
	for _, id := range survivor.MergedEvidenceIDs {
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(raw)] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, raw)
	}
	appendID(dup.EvidenceID)
	for _, id := range dup.MergedEvidenceIDs {
		appendID(id)
	}
}

// --- R2: same-kind ×N aggregation -------------------------------------------

// traceCausalProjectionAggregateSameKind collapses ≥3 rows with exactly the
// same (subject, object) inside ONE bucket into a single ×N row carrying the
// SUM, the per-instance min–max range, and every instance's evidence id.
// Cross-bucket copies stay consistent because each bucket aggregates the same
// member set to the same lead EvidenceID (renderers dedupe by node key).
func traceCausalProjectionAggregateSameKind(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < traceCausalProjectionSameKindAggregateMin {
		return nodes
	}
	type group struct {
		first   int
		members []int
	}
	groups := map[string]*group{}
	order := make([]string, 0, len(nodes))
	for i, node := range nodes {
		subject := traceCausalProjectionCanonicalNode(node.Subject)
		object := traceCausalProjectionCanonicalNode(node.Object)
		if subject == "" || object == "" {
			continue
		}
		key := subject + "\x00" + object
		g, ok := groups[key]
		if !ok {
			g = &group{first: i}
			groups[key] = g
			order = append(order, key)
		}
		g.members = append(g.members, i)
	}
	replaced := map[int]TraceCausalProjectionNode{}
	dropped := map[int]bool{}
	for _, key := range order {
		g := groups[key]
		if len(g.members) < traceCausalProjectionSameKindAggregateMin {
			continue
		}
		aggregate := nodes[g.first]
		var sum, minMS, maxMS float64
		absorbed := map[string]bool{traceCausalProjectionCanonicalNode(aggregate.EvidenceID): true}
		for _, idx := range g.members {
			member := nodes[idx]
			traceCausalProjectionAppendMergedSubject(&aggregate, member.Subject)
			display := member.ImpactMS
			if display <= 0 {
				display = member.CumulativeImpactMS
			}
			sum += display
			if minMS == 0 || (display > 0 && display < minMS) {
				minMS = display
			}
			if display > maxMS {
				maxMS = display
			}
			if idx != g.first {
				dropped[idx] = true
				id := strings.TrimSpace(member.EvidenceID)
				if id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
					absorbed[traceCausalProjectionCanonicalNode(id)] = true
					aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, id)
				}
				for _, id := range member.MergedEvidenceIDs {
					if id = strings.TrimSpace(id); id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
						absorbed[traceCausalProjectionCanonicalNode(id)] = true
						aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, id)
					}
				}
				for _, object := range member.SecondaryObjects {
					traceCausalProjectionAppendSecondaryObject(&aggregate, object)
				}
				if member.LineStart > 0 && (aggregate.LineStart <= 0 || member.LineStart < aggregate.LineStart) {
					aggregate.LineStart = member.LineStart
				}
				if member.LineEnd > aggregate.LineEnd {
					aggregate.LineEnd = member.LineEnd
				}
				if member.Rank > 0 && (aggregate.Rank <= 0 || member.Rank < aggregate.Rank) {
					aggregate.Rank = member.Rank
				}
				if member.Confidence > 0 && (aggregate.Confidence <= 0 || member.Confidence < aggregate.Confidence) {
					aggregate.Confidence = member.Confidence
				}
				if member.StartTs > 0 && (aggregate.StartTs <= 0 || member.StartTs < aggregate.StartTs) {
					aggregate.StartTs = member.StartTs
				}
				if member.EndTs > aggregate.EndTs {
					aggregate.EndTs = member.EndTs
				}
			}
		}
		aggregate.MergedCount = len(g.members)
		aggregate.MergedMinMS = minMS
		aggregate.MergedMaxMS = maxMS
		aggregate.ImpactMS = sum
		aggregate.CumulativeImpactMS = sum
		// F2 (adversarial review 2026-07-03): the ×N row carries a SUM, so the
		// DuplicatePublications contract ("dup>0 ⇒ the value is ONE republished
		// measurement") can never hold on it — a dup count inherited from the
		// group-first survivor (or silently lost from a non-first member) once
		// rendered the mutually-exclusive ×2同值合并 and ×3合并 labels on one row.
		// Cleared unconditionally: member provenance stays lossless through
		// MergedEvidenceIDs; no second counter is introduced.
		aggregate.DuplicatePublications = 0
		// VS-1 F6(a) (adversarial review 2026-07-04): the ×N SUM row re-derives
		// its periodic accounting from the MEMBERS instead of inheriting the
		// group-first copy. All members periodic → the fold keeps the flag with
		// the summed discount (Σ effective / Σ lateness are legal sums here:
		// per-member discounts are disjoint per-occurrence amounts, never
		// overlapping wall clock) and the group head's DetectedPeriodMS (already
		// on the aggregate). ANY non-periodic member → the SUM row is back to
		// raw semantics: flag, cadence fields and the inherited (periodic-only)
		// discount are cleared — a part-cadence sum labelled periodic would
		// discount real waits it never measured, and a stale group-first
		// effective would understate the ×N total.
		allPeriodic := true
		for _, idx := range g.members {
			if !nodes[idx].PeriodicSource {
				allPeriodic = false
				break
			}
		}
		if allPeriodic {
			effective, lateness := 0.0, 0.0
			for _, idx := range g.members {
				effective += nodes[idx].EffectiveImpactMS
				lateness += nodes[idx].PeriodicLatenessMS
			}
			aggregate.EffectiveImpactMS = effective
			aggregate.PeriodicLatenessMS = lateness
		} else if aggregate.PeriodicSource {
			aggregate.PeriodicSource = false
			aggregate.DetectedPeriodMS = 0
			aggregate.PeriodicLatenessMS = 0
			aggregate.EffectiveImpactMS = 0
		}
		replaced[g.first] = aggregate
	}
	if len(replaced) == 0 && len(dropped) == 0 {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		if aggregate, ok := replaced[i]; ok {
			out = append(out, aggregate)
			continue
		}
		out = append(out, node)
	}
	return out
}

// --- R3: unknown-impact-point background folding -----------------------------

// traceCausalProjectionFoldUnknownBackground keeps the top-K background rows
// whose impact point is the unknown-thread sentinel and folds the rest into a
// single subjectless aggregate row (rendered as “其余 N 项合并”). Background
// rows with a REAL object (a cause word or resolved peer) are never folded.
//
// V3 (customer revisit 2026-07-03): the fold members are DIFFERENT threads, so
// their wall-clock projections must never be summed — six whole-window 101ms
// background threads once published as a 606ms/600% fold row. The fold's
// ImpactMS/CumulativeImpactMS carry the member MAX; MergedMinMS/MergedMaxMS
// keep the lossless per-member range and MergedCount the member count.
func traceCausalProjectionFoldUnknownBackground(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	var unknown []int
	for i, node := range nodes {
		if node.MergedCount > 1 {
			continue
		}
		if !traceCausalProjectionKnownSubject(node.Object) && strings.TrimSpace(node.Object) != "" {
			unknown = append(unknown, i)
		}
	}
	if len(unknown) < traceCausalProjectionUnknownBackgroundMin {
		return nodes
	}
	// Bucket order is already impact-major (classifiedLess); keep the first K.
	fold := unknown[traceCausalProjectionUnknownBackgroundKeep:]
	foldSet := make(map[int]bool, len(fold))
	for _, idx := range fold {
		foldSet[idx] = true
	}
	aggregate := TraceCausalProjectionNode{
		Role:           nodes[fold[0]].Role,
		Predicate:      nodes[fold[0]].Predicate,
		Object:         nodes[fold[0]].Object,
		ChainRelevance: "background",
		Causality:      nodes[fold[0]].Causality,
	}
	// F3 support: the fold row keeps the members' typed dominant state ONLY when
	// every member carries the same canonical StateKind (strict unanimity — any
	// divergence or an empty member leaves the fold stateless). The renderer's
	// whole-window idle annotation is gated on the wait-family StateKind, so a
	// fold of uniform whole-window sleepers legitimately keeps the tag while a
	// mixed or stateless fold never fabricates one.
	foldState := nodes[fold[0]].StateKind
	for _, idx := range fold {
		if traceCausalProjectionCanonicalNode(nodes[idx].StateKind) !=
			traceCausalProjectionCanonicalNode(foldState) {
			foldState = ""
			break
		}
	}
	aggregate.StateKind = strings.TrimSpace(foldState)
	var minMS, maxMS float64
	absorbed := map[string]bool{}
	for _, idx := range fold {
		member := nodes[idx]
		// Keep the folded rows' thread names visible on the subjectless fold
		// row — the renderer's "其余 N 项合并" line names them from here.
		traceCausalProjectionAppendMergedSubject(&aggregate, member.Subject)
		display := member.ImpactMS
		if display <= 0 {
			display = member.CumulativeImpactMS
		}
		if minMS == 0 || (display > 0 && display < minMS) {
			minMS = display
		}
		if display > maxMS {
			maxMS = display
		}
		appendID := func(raw string) {
			raw = strings.TrimSpace(raw)
			if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
				return
			}
			absorbed[traceCausalProjectionCanonicalNode(raw)] = true
			if aggregate.EvidenceID == "" {
				aggregate.EvidenceID = raw
				return
			}
			aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, raw)
		}
		appendID(member.EvidenceID)
		for _, id := range member.MergedEvidenceIDs {
			appendID(id)
		}
		if member.LineStart > 0 && (aggregate.LineStart <= 0 || member.LineStart < aggregate.LineStart) {
			aggregate.LineStart = member.LineStart
		}
		if member.LineEnd > aggregate.LineEnd {
			aggregate.LineEnd = member.LineEnd
		}
		if member.Confidence > 0 && (aggregate.Confidence <= 0 || member.Confidence < aggregate.Confidence) {
			aggregate.Confidence = member.Confidence
		}
	}
	aggregate.MergedCount = len(fold)
	aggregate.MergedMinMS = minMS
	aggregate.MergedMaxMS = maxMS
	// V3: member MAX, never a cross-thread wall-clock sum (see the fold doc).
	aggregate.ImpactMS = maxMS
	aggregate.CumulativeImpactMS = maxMS
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(fold)+1)
	for i, node := range nodes {
		if foldSet[i] {
			continue
		}
		out = append(out, node)
	}
	return append(out, aggregate)
}

// --- post-aggregation ordering ----------------------------------------------

// traceCausalProjectionResortAfterAggregation restores impact-major order
// inside each bucket after R2 sums may have changed magnitudes. It reuses the
// build-time comparators; the R3 fold row is subjectless and deliberately sorts
// by its published magnitude (the member max since V3) like any other row.
func traceCausalProjectionResortAfterAggregation(out *TraceCausalProjection) {
	pathIndex := traceCausalProjectionPathIndex(out.WakeupPath)
	sort.SliceStable(out.PrimaryRootCauses, func(i, j int) bool {
		return traceCausalProjectionPrimaryLess(out.PrimaryRootCauses[i], out.PrimaryRootCauses[j], pathIndex)
	})
	sort.SliceStable(out.SupportingHops, func(i, j int) bool {
		return traceCausalProjectionHopLess(out.SupportingHops[i], out.SupportingHops[j], pathIndex)
	})
	for _, bucket := range []*[]TraceCausalProjectionNode{&out.OnChainCauses, &out.AdjacentCauses, &out.BackgroundCauses} {
		nodes := *bucket
		sort.SliceStable(nodes, func(i, j int) bool {
			return traceCausalProjectionClassifiedLess(nodes[i], nodes[j], pathIndex)
		})
	}
}
