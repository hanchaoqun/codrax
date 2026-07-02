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
)

func traceCausalProjectionAggregateForPresentation(out *TraceCausalProjection) {
	if out == nil {
		return
	}
	traceCausalProjectionMergeSameFacts(out)
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
	if survivor.EffectiveImpactMS <= 0 {
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
	var sum, minMS, maxMS float64
	absorbed := map[string]bool{}
	for _, idx := range fold {
		member := nodes[idx]
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
	aggregate.ImpactMS = sum
	aggregate.CumulativeImpactMS = sum
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
// by its summed magnitude like any other row.
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
