package types

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
)

// AnswerRelationClaim is a model-authored, machine-checkable declaration of
// how a concrete set of typed values may be related. It is metadata on the
// model's own investigation/answer, not system-authored visible prose.
//
// The physical relation and arithmetic permission are deliberately separate:
// unresolved physical overlap does not imply additivity, while a closed
// mutually-exclusive partition may be additive to exactly one published
// subtotal. Validators compare this payload only with typed authorities; they
// never infer a claim from the user's request or the model's prose.
type AnswerRelationClaim struct {
	AuthorityID      string   `json:"authority_id"`
	MemberRefs       []string `json:"member_refs"`
	PhysicalRelation string   `json:"physical_relation"`
	Addition         string   `json:"addition"`
	SubtotalValue    *float64 `json:"subtotal_value,omitempty"`
	SubtotalUnit     string   `json:"subtotal_unit,omitempty"`
}

const (
	AnswerPhysicalRelationUnresolved        = "unresolved"
	AnswerPhysicalRelationMutuallyExclusive = "mutually_exclusive"
	AnswerPhysicalRelationOverlap           = "overlap"
	AnswerPhysicalRelationContains          = "contains"
	AnswerPhysicalRelationContainedBy       = "contained_by"

	AnswerRelationAdditionAuthorized = "authorized_to_published_subtotal"
	AnswerRelationAdditionForbidden  = "forbidden"
)

// AnswerRelationAuthorityKind names the exact typed rule behind an accepted
// AnswerRelationClaim. New producers can add kinds without changing the wire
// claim shape.
type AnswerRelationAuthorityKind string

const (
	AnswerRelationAuthoritySameRulerSubtotal   AnswerRelationAuthorityKind = "same_ruler_subtotal"
	AnswerRelationAuthorityCrossRulerBoundary  AnswerRelationAuthorityKind = "cross_ruler_boundary"
	AnswerRelationAuthorityClosedPartition     AnswerRelationAuthorityKind = "closed_partition"
	AnswerRelationAuthoritySameSourcePartition AnswerRelationAuthorityKind = "same_source_partition"
	AnswerRelationAuthorityOverlappingMembers  AnswerRelationAuthorityKind = "overlapping_members"
)

// AnswerRelationAuthority is system-owned typed input to the model and exact
// validator. RequiredForClosure is set only where omitting the relation would
// make the already-published value roster unsafe to summarize (currently the
// self-runnable two-ruler accounting). It never forces visible wording.
type AnswerRelationAuthority struct {
	ID                 string
	Kind               AnswerRelationAuthorityKind
	MemberRefs         []string
	LeftMemberRefs     []string
	RightMemberRefs    []string
	PhysicalRelation   string
	Addition           string
	SubtotalValue      *float64
	SubtotalUnit       string
	RequiredForClosure bool
	// The fields below calibrate non-additive overlap authorities for the
	// reasoning model. They are typed system inputs, not part of the optional
	// model-authored AnswerRelationClaim carrier: the claim acknowledges the
	// exact pair/relation/addition rule while these values keep the comparison
	// arithmetic out of prose and out of JSON copying work.
	MemberValuesMS    []float64
	MeasuredOverlapMS *float64
	ComparisonValueMS *float64
	ComparisonRule    string
	FixDirection      string
	ChainLane         string
}

// CompileTraceAnswerRelationAuthorities projects only already-validated trace
// relation carriers into stable model-facing authorities. IDs use a compact
// typed-content fingerprint, so the trace_query preview and the later
// partitioned projection compile name the same authority independently.
func CompileTraceAnswerRelationAuthorities(set TraceCausalProjectionSet) []AnswerRelationAuthority {
	var out []AnswerRelationAuthority
	for _, projection := range set.Projections {
		for _, accounting := range projection.SelfRunnableTwoRulerAccountings {
			if !TraceCausalProjectionSelfRunnableTwoRulerValid(accounting) {
				continue
			}
			prefix := "trace:self_runnable_two_ruler:" + answerRelationTwoRulerFingerprint(accounting)
			wall := answerRelationRankRefs(accounting.WallRanks)
			edge := answerRelationRankRefs(accounting.EdgeRanks)
			wallSubtotal := accounting.WallSubtotalMS
			edgeSubtotal := accounting.EdgeSubtotalMS
			out = append(out,
				AnswerRelationAuthority{
					ID: prefix + ":wall", Kind: AnswerRelationAuthoritySameRulerSubtotal,
					MemberRefs: wall, PhysicalRelation: AnswerPhysicalRelationUnresolved,
					Addition: AnswerRelationAdditionAuthorized, SubtotalValue: &wallSubtotal,
					SubtotalUnit: "ms", RequiredForClosure: true,
				},
				AnswerRelationAuthority{
					ID: prefix + ":edge", Kind: AnswerRelationAuthoritySameRulerSubtotal,
					MemberRefs: edge, PhysicalRelation: AnswerPhysicalRelationUnresolved,
					Addition: AnswerRelationAdditionAuthorized, SubtotalValue: &edgeSubtotal,
					SubtotalUnit: "ms", RequiredForClosure: true,
				},
				AnswerRelationAuthority{
					ID: prefix + ":cross", Kind: AnswerRelationAuthorityCrossRulerBoundary,
					LeftMemberRefs: wall, RightMemberRefs: edge,
					PhysicalRelation: AnswerPhysicalRelationUnresolved,
					Addition:         AnswerRelationAdditionForbidden, RequiredForClosure: true,
				},
			)
		}
		if account := projection.TargetStateAccount; answerRelationTargetStatePartitionClosed(projection, account) {
			total := account.TotalMS
			out = append(out, AnswerRelationAuthority{
				ID:               "trace:target_state_partition:" + answerRelationTargetStateFingerprint(*account),
				Kind:             AnswerRelationAuthorityClosedPartition,
				MemberRefs:       []string{"running", "runnable", "sleep", "d_state", "io_wait"},
				PhysicalRelation: AnswerPhysicalRelationMutuallyExclusive,
				Addition:         AnswerRelationAdditionAuthorized,
				SubtotalValue:    &total, SubtotalUnit: "ms",
				RequiredForClosure: true,
			})
		}
		out = append(out, answerRelationSameSourcePartitionAuthorities(projection)...)
	}
	return out
}

// TraceAnswerDecisionEliminableSeats is the single bounded on-chain seat
// selection for both typed relation compilation and the finalizer's axis-B
// roster. RankedSeats deliberately preserves adjacent/background rank boards
// too, but those boards are context rather than target-causal authority. They
// must not be relabelled as an "effective attribution" merely because their
// producer also assigned a positive rank. Keeping this one exact typed-lane
// selector prevents both prompt faces from drifting while leaving every
// off-chain row available through the separate contextual roster.
func TraceAnswerDecisionEliminableSeats(projection TraceCausalProjection, limit int) []TraceCausalProjectionNode {
	pool := append([]TraceCausalProjectionNode(nil), projection.RankedSeats...)
	if len(pool) == 0 {
		pool = append(pool, projection.PrimaryRootCauses...)
		pool = append(pool, projection.OnChainCauses...)
	}
	seats := make([]TraceCausalProjectionNode, 0, len(pool))
	seen := map[string]bool{}
	for _, node := range pool {
		if strings.TrimSpace(node.ChainRelevance) != "on_chain" ||
			node.Rank <= 0 || node.EffectiveImpactMS <= 0 ||
			node.IsTargetSelfStateRow() || node.IsAggregateMetric() || node.OnChainOverflowFold ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		identity := fmt.Sprintf("%d\x00%s", node.Rank, traceAnswerDecisionNodeIdentity(node))
		if seen[identity] {
			continue
		}
		seen[identity] = true
		seats = append(seats, node)
	}
	if TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
		matching := make([]TraceCausalProjectionNode, 0, len(seats))
		for _, node := range seats {
			start, end := node.RankQueryWindowStartTs, node.RankQueryWindowEndTs
			if !TraceCausalProjectionWindowPresent(start, end) {
				start, end = node.QueryWindowStartTs, node.QueryWindowEndTs
			}
			if TraceCausalProjectionWindowPresent(start, end) &&
				math.Abs(start-projection.WindowStartTs) <= TraceCausalProjectionSameWindowToleranceS &&
				math.Abs(end-projection.WindowEndTs) <= TraceCausalProjectionSameWindowToleranceS {
				matching = append(matching, node)
			}
		}
		if len(matching) > 0 {
			seats = matching
		}
	}
	sort.SliceStable(seats, func(i, j int) bool {
		if seats[i].Rank != seats[j].Rank {
			return seats[i].Rank < seats[j].Rank
		}
		if seats[i].EffectiveImpactMS != seats[j].EffectiveImpactMS {
			return seats[i].EffectiveImpactMS > seats[j].EffectiveImpactMS
		}
		return traceAnswerDecisionNodeIdentity(seats[i]) < traceAnswerDecisionNodeIdentity(seats[j])
	})
	if limit > 0 && len(seats) > limit {
		seats = seats[:limit]
	}
	return seats
}

func traceAnswerDecisionNodeIdentity(node TraceCausalProjectionNode) string {
	if id := strings.TrimSpace(node.EvidenceID); id != "" {
		return id
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%.6f\x00%.6f\x00%.6f",
		strings.TrimSpace(node.Subject), strings.TrimSpace(node.Predicate),
		strings.TrimSpace(node.Object), node.ImpactMS, node.StartTs, node.EndTs)
}

func answerRelationRankSeatMemberRef(node TraceCausalProjectionNode) string {
	board, boardKnown := answerRelationRankBoardIdentity(node)
	if node.Rank <= 0 || !node.EffectiveImpactPublished || !boardKnown || strings.TrimSpace(node.Subject) == "" ||
		strings.TrimSpace(node.Object) == "" || node.EffectiveImpactMS <= 0 ||
		strings.TrimSpace(node.FixDirection) == "" || strings.TrimSpace(node.ChainRelevance) == "" {
		return ""
	}
	raw := fmt.Sprintf("%s|#%d|%s|%s|%.6f..%.6f|%.3f|%s|%s|%d..%d",
		board, node.Rank,
		traceCausalProjectionCanonicalNode(node.Subject), strings.TrimSpace(node.Object),
		node.StartTs, node.EndTs, answerRelationPublishedMS(node.EffectiveImpactMS),
		strings.TrimSpace(node.FixDirection), strings.TrimSpace(node.ChainRelevance),
		node.LineStart, node.LineEnd)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("#%d@%x", node.Rank, sum[:4])
}

// TraceAnswerRelationMemberRef returns the stable compact relation identity
// for one exact ranked seat. It is empty when the seat lacks the complete
// typed board/value/relation identity needed by relation authorities.
func TraceAnswerRelationMemberRef(node TraceCausalProjectionNode) string {
	return answerRelationRankSeatMemberRef(node)
}

func answerRelationRankBoardIdentity(node TraceCausalProjectionNode) (string, bool) {
	start, end := node.RankQueryWindowStartTs, node.RankQueryWindowEndTs
	if !TraceCausalProjectionWindowPresent(start, end) {
		start, end = node.QueryWindowStartTs, node.QueryWindowEndTs
	}
	target := strings.TrimSpace(node.RankBoardTarget)
	params := strings.TrimSpace(node.RankBoardParamsFingerprint)
	if target == "" || params == "" || !TraceCausalProjectionWindowPresent(start, end) {
		return "", false
	}
	return fmt.Sprintf("%s\x00%s\x00%.6f\x00%.6f", target, params, start, end), true
}

// answerRelationSameSourcePartitionAuthorities compiles the engine-minted
// RSPA chain-anchor split into a model-visible relation. It deliberately reads
// the complete ranked-seat authority roster rather than renderer rows: tree
// caps, folds and E# assignment must not create a second relation judgment.
//
// A relation is published only when one exact board contains exactly one
// on-chain anchored seat and exactly one adjacent remainder seat with the same
// subject/type/line envelope and the same typed full/anchored pair. Both
// published effective values must reproduce the two partition terms at the
// producer's 3-decimal wire precision. Missing twins, ownership divergence,
// cross-board lookalikes and ambiguous duplicates all fail closed.
func answerRelationSameSourcePartitionAuthorities(projection TraceCausalProjection) []AnswerRelationAuthority {
	type pair struct {
		anchored  []TraceCausalProjectionNode
		remainder []TraceCausalProjectionNode
	}
	groups := map[string]*pair{}
	for _, node := range projection.RankedSeats {
		full := answerRelationPublishedMS(node.ChainAnchorFullMS)
		anchored := answerRelationPublishedMS(node.ChainAnchoredMS)
		if full <= 0 || anchored <= 0 || anchored >= full ||
			node.ChainAnchorOwnershipDivergent || !node.EffectiveImpactPublished ||
			node.LineStart <= 0 || node.LineEnd < node.LineStart ||
			strings.TrimSpace(node.Subject) == "" || strings.TrimSpace(node.Object) == "" {
			continue
		}
		component := anchored
		if node.ChainAnchorRemainderSeat {
			component = answerRelationPublishedMS(full - anchored)
			if strings.TrimSpace(node.ChainRelevance) != "adjacent" {
				continue
			}
		} else if strings.TrimSpace(node.ChainRelevance) != "on_chain" {
			continue
		}
		if answerRelationPublishedMS(node.EffectiveImpactMS) != component {
			continue
		}
		key := strings.Join([]string{
			traceCausalProjectionRankBoardIdentityKey(node),
			traceCausalProjectionCanonicalNode(node.Subject),
			traceCausalProjectionCanonicalNode(node.Object),
			fmt.Sprintf("%d..%d", node.LineStart, node.LineEnd),
			fmt.Sprintf("%.3f|%.3f", full, anchored),
		}, "\x00")
		if groups[key] == nil {
			groups[key] = &pair{}
		}
		if node.ChainAnchorRemainderSeat {
			groups[key].remainder = append(groups[key].remainder, node)
		} else {
			groups[key].anchored = append(groups[key].anchored, node)
		}
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []AnswerRelationAuthority
	for _, key := range keys {
		group := groups[key]
		if len(group.anchored) != 1 || len(group.remainder) != 1 {
			continue
		}
		anchoredNode, remainderNode := group.anchored[0], group.remainder[0]
		full := answerRelationPublishedMS(anchoredNode.ChainAnchorFullMS)
		anchoredRef := answerRelationChainSplitMemberRef("on_chain_anchored", anchoredNode)
		remainderRef := answerRelationChainSplitMemberRef("adjacent_remainder", remainderNode)
		out = append(out, AnswerRelationAuthority{
			ID:                 "trace:same_source_partition:" + answerRelationSameSourcePartitionFingerprint(anchoredNode),
			Kind:               AnswerRelationAuthoritySameSourcePartition,
			MemberRefs:         []string{anchoredRef, remainderRef},
			PhysicalRelation:   AnswerPhysicalRelationMutuallyExclusive,
			Addition:           AnswerRelationAdditionAuthorized,
			SubtotalValue:      &full,
			SubtotalUnit:       "ms",
			RequiredForClosure: true,
		})
	}
	return out
}

func answerRelationPublishedMS(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func answerRelationChainSplitMemberRef(role string, node TraceCausalProjectionNode) string {
	return fmt.Sprintf("%s:%s:%s:lines=%d-%d", role, strings.TrimSpace(node.Subject),
		strings.TrimSpace(node.Object), node.LineStart, node.LineEnd)
}

func answerRelationSameSourcePartitionFingerprint(node TraceCausalProjectionNode) string {
	raw := fmt.Sprintf("%s|%s|%d..%d|%.3f|%.3f",
		traceCausalProjectionCanonicalNode(node.Subject),
		traceCausalProjectionCanonicalNode(node.Object), node.LineStart, node.LineEnd,
		answerRelationPublishedMS(node.ChainAnchorFullMS),
		answerRelationPublishedMS(node.ChainAnchoredMS))
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:8])
}

// answerRelationTargetStatePartitionClosed admits only the engine's exact
// five-lane wall-clock partition. The five raw lanes are mutually exclusive;
// the renderer may merge d_state+io_wait into one human-facing D-state term,
// but that display fold does not change the underlying arithmetic. Requiring
// both the lane identity and the selected-window identity prevents a partial
// or imbalanced state account from becoming closure authority. The account is
// attached to a projection with the shared F-2 same-window tolerance, so this
// authority must use that same endpoint rule. Its subtotal closes the
// account's own typed window; requiring it to equal the anchor's byte-rendered
// duration after tolerant admission would make preview and final validation
// disagree for the same accepted account.
func answerRelationTargetStatePartitionClosed(projection TraceCausalProjection, account *TraceCausalProjectionTargetStateAccount) bool {
	if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 ||
		account.RunningMS < 0 || account.RunnableMS < 0 || account.SleepMS < 0 ||
		account.DStateMS < 0 || account.IOWaitMS < 0 {
		return false
	}
	sum := account.RunningMS + account.RunnableMS + account.SleepMS + account.DStateMS + account.IOWaitMS
	if fmt.Sprintf("%.3f", sum) != fmt.Sprintf("%.3f", account.TotalMS) {
		return false
	}
	// The compact pre-Explore preview has no projection anchor yet; the typed
	// result account still proves its own subtotal. Once an anchor exists, it
	// must match the account window and the total must close that exact window.
	if !TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
		return true
	}
	if !TraceCausalProjectionWindowPresent(account.WindowStartTs, account.WindowEndTs) ||
		math.Abs(account.WindowStartTs-projection.WindowStartTs) > TraceCausalProjectionSameWindowToleranceS ||
		math.Abs(account.WindowEndTs-projection.WindowEndTs) > TraceCausalProjectionSameWindowToleranceS {
		return false
	}
	windowMS := (account.WindowEndTs - account.WindowStartTs) * 1000
	return fmt.Sprintf("%.3f", account.TotalMS) == fmt.Sprintf("%.3f", windowMS)
}

// CompileTraceAnswerRelationAuthoritiesFromLedger is the authority entry for
// completion/final-answer validators. It scans the already-typed observation
// ledger with the projection's strict parser, then merges projection-level
// authorities. This keeps a valid two-ruler side channel alive even when a
// projection has no active causal node (TraceCausalProjection.Active is a
// presentation predicate and intentionally ignores side channels).
func CompileTraceAnswerRelationAuthoritiesFromLedger(ledger ObservationLedger) []AnswerRelationAuthority {
	var out []AnswerRelationAuthority
	for _, record := range ledger.Records {
		if !traceCausalProjectionTraceQueryRecord(record) || strings.TrimSpace(record.Predicate) != "self_runnable_two_ruler" {
			continue
		}
		accounting, ok := traceCausalProjectionSelfRunnableTwoRulerFromRecord(record)
		if !ok {
			continue
		}
		out = append(out, CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{
			Projections: []TraceCausalProjection{{SelfRunnableTwoRulerAccountings: []TraceCausalProjectionSelfRunnableTwoRuler{accounting}}},
		})...)
	}
	out = append(out, CompileTraceAnswerRelationAuthorities(CompileTraceCausalProjectionSet(ledger))...)
	seen := make(map[string]bool, len(out))
	deduped := out[:0]
	for _, authority := range out {
		if authority.ID == "" || seen[authority.ID] {
			continue
		}
		seen[authority.ID] = true
		deduped = append(deduped, authority)
	}
	return deduped
}

func answerRelationTwoRulerFingerprint(accounting TraceCausalProjectionSelfRunnableTwoRuler) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|wall=", strings.TrimSpace(accounting.Subject))
	for i, rank := range accounting.WallRanks {
		if i < len(accounting.WallEffsMS) {
			fmt.Fprintf(&b, "#%d:%.6f,", rank, accounting.WallEffsMS[i])
		}
	}
	fmt.Fprintf(&b, "subtotal:%.6f|edge=", accounting.WallSubtotalMS)
	for i, rank := range accounting.EdgeRanks {
		if i < len(accounting.EdgeEffsMS) {
			fmt.Fprintf(&b, "#%d:%.6f,", rank, accounting.EdgeEffsMS[i])
		}
	}
	fmt.Fprintf(&b, "subtotal:%.6f", accounting.EdgeSubtotalMS)
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", sum[:8])
}

func answerRelationTargetStateFingerprint(account TraceCausalProjectionTargetStateAccount) string {
	raw := fmt.Sprintf("%s|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f",
		strings.TrimSpace(account.Subject), account.RunningMS, account.RunnableMS,
		account.SleepMS, account.DStateMS, account.IOWaitMS, account.TotalMS)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:8])
}

func answerRelationRankRefs(ranks []int) []string {
	out := make([]string, 0, len(ranks))
	for _, rank := range ranks {
		if rank > 0 {
			out = append(out, fmt.Sprintf("#%d", rank))
		}
	}
	return out
}

// ValidateAnswerRelationClaims validates model-authored relation metadata
// against typed authorities. When requireClosureAuthorities is true, every
// RequiredForClosure authority must be acknowledged exactly once. There is no
// prose inspection and no system-side answer repair.
func ValidateAnswerRelationClaims(claims []AnswerRelationClaim, authorities []AnswerRelationAuthority, requireClosureAuthorities bool) error {
	byID := make(map[string]AnswerRelationAuthority, len(authorities))
	for _, authority := range authorities {
		byID[authority.ID] = authority
	}
	seen := make(map[string]bool, len(claims))
	var violations []string
	for index, raw := range claims {
		claim := NormalizeAnswerRelationClaim(raw)
		if claim.AuthorityID == "" {
			violations = append(violations, fmt.Sprintf("relation_claims[%d].authority_id is required", index))
			continue
		}
		if seen[claim.AuthorityID] {
			violations = append(violations, fmt.Sprintf("relation_claims[%d] duplicates authority_id=%q", index, claim.AuthorityID))
			continue
		}
		seen[claim.AuthorityID] = true
		authority, ok := byID[claim.AuthorityID]
		if !ok {
			violations = append(violations, fmt.Sprintf("relation_claims[%d].authority_id=%q has no typed relation authority in this investigation", index, claim.AuthorityID))
			continue
		}
		for _, violation := range answerRelationClaimAuthorityViolations(claim, authority) {
			violations = append(violations, fmt.Sprintf("relation_claims[%d] authority_id=%q: %s", index, claim.AuthorityID, violation))
		}
	}
	if requireClosureAuthorities {
		for _, authority := range authorities {
			if authority.RequiredForClosure && !seen[authority.ID] {
				violations = append(violations, fmt.Sprintf("missing required model-authored relation claim for authority_id=%q", authority.ID))
			}
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("relation_claims validation failed with %d violation(s); fix all listed fields in one re-emit: %s",
			len(violations), formatAnswerRelationClaimViolations(violations))
	}
	return nil
}

// PartitionAnswerRelationClaimsByCurrentAuthorities separates previously
// accepted model claims that are still backed by the final typed authority
// slate from claims superseded by later typed evidence (for example a
// deterministic post-Explore supplement or a replanned window). The match is
// exact across identity, members, relation, addition, and subtotal. Callers
// may keep the current claims as model-authored metadata, but must not require
// superseded claims alongside the final authority slate: that would create an
// impossible hard contract. No model prose is inspected or rewritten here.
func PartitionAnswerRelationClaimsByCurrentAuthorities(claims []AnswerRelationClaim, authorities []AnswerRelationAuthority) (current, superseded []AnswerRelationClaim) {
	byID := make(map[string]AnswerRelationAuthority, len(authorities))
	for _, authority := range authorities {
		byID[authority.ID] = authority
	}
	for _, raw := range claims {
		claim := NormalizeAnswerRelationClaim(raw)
		authority, ok := byID[claim.AuthorityID]
		if !ok || len(answerRelationClaimAuthorityViolations(claim, authority)) > 0 {
			superseded = append(superseded, claim)
			continue
		}
		current = append(current, claim)
	}
	return CloneAnswerRelationClaims(current), CloneAnswerRelationClaims(superseded)
}

// answerRelationClaimAuthorityViolations reports every independently
// actionable field mismatch for one claim. Identity failures are handled by
// the caller and do not enter this function: comparing dependent fields
// without an exact typed authority would manufacture cascade errors.
func answerRelationClaimAuthorityViolations(claim AnswerRelationClaim, authority AnswerRelationAuthority) []string {
	var violations []string
	if claim.PhysicalRelation != authority.PhysicalRelation {
		violations = append(violations, fmt.Sprintf("physical_relation=%q; typed authority requires %q", claim.PhysicalRelation, authority.PhysicalRelation))
	}
	if claim.Addition != authority.Addition {
		violations = append(violations, fmt.Sprintf("addition=%q; typed authority requires %q", claim.Addition, authority.Addition))
	}
	switch authority.Kind {
	case AnswerRelationAuthorityCrossRulerBoundary:
		if claim.SubtotalValue != nil || claim.SubtotalUnit != "" {
			violations = append(violations, fmt.Sprintf(
				"subtotal_value=%s subtotal_unit=%q; typed cross-ruler authority requires subtotal_value=<absent> subtotal_unit=%q",
				answerRelationOptionalFloat(claim.SubtotalValue), claim.SubtotalUnit, ""))
		}
		known := append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
		if len(claim.MemberRefs) < 2 || !answerRelationMembersKnown(claim.MemberRefs, known) ||
			!answerRelationHasAny(claim.MemberRefs, authority.LeftMemberRefs) || !answerRelationHasAny(claim.MemberRefs, authority.RightMemberRefs) {
			violations = append(violations, fmt.Sprintf(
				"member_refs=%v; typed authority requires known members including at least one from each ruler group (left=%v right=%v)",
				claim.MemberRefs, authority.LeftMemberRefs, authority.RightMemberRefs))
		}
	default:
		if !answerRelationSameSet(claim.MemberRefs, authority.MemberRefs) {
			violations = append(violations, fmt.Sprintf("member_refs=%v; typed authority requires the exact member set %v", claim.MemberRefs, authority.MemberRefs))
		}
		switch {
		case authority.SubtotalValue == nil && claim.SubtotalValue != nil:
			violations = append(violations, fmt.Sprintf("subtotal_value=%s; typed authority requires <absent>", answerRelationOptionalFloat(claim.SubtotalValue)))
		case authority.SubtotalValue != nil && claim.SubtotalValue == nil:
			violations = append(violations, fmt.Sprintf("subtotal_value=<absent>; typed published subtotal is %.6f", *authority.SubtotalValue))
		case authority.SubtotalValue != nil && claim.SubtotalValue != nil && math.Abs(*claim.SubtotalValue-*authority.SubtotalValue) > 0.001:
			violations = append(violations, fmt.Sprintf("subtotal_value=%.6f; typed published subtotal is %.6f", *claim.SubtotalValue, *authority.SubtotalValue))
		}
		if claim.SubtotalUnit != authority.SubtotalUnit {
			violations = append(violations, fmt.Sprintf("subtotal_unit=%q; typed authority requires %q", claim.SubtotalUnit, authority.SubtotalUnit))
		}
	}
	return violations
}

func answerRelationOptionalFloat(value *float64) string {
	if value == nil {
		return "<absent>"
	}
	return fmt.Sprintf("%.6f", *value)
}

// formatAnswerRelationClaimViolations keeps retry guidance bounded even when
// a caller bypasses the schema's maxItems constraint. The total count remains
// visible so truncation can never masquerade as a complete census.
func formatAnswerRelationClaimViolations(violations []string) string {
	const maxListed = 12
	shown := violations
	if len(shown) > maxListed {
		shown = shown[:maxListed]
	}
	var b strings.Builder
	for i, violation := range shown {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "[%d] %s", i+1, violation)
	}
	if len(violations) > len(shown) {
		fmt.Fprintf(&b, "; ... and %d more violation(s)", len(violations)-len(shown))
	}
	return b.String()
}

// NormalizeAnswerRelationClaim canonicalizes only structural metadata. It does
// not alter any model-authored visible conclusion.
func NormalizeAnswerRelationClaim(in AnswerRelationClaim) AnswerRelationClaim {
	out := in
	out.AuthorityID = strings.TrimSpace(out.AuthorityID)
	out.PhysicalRelation = strings.TrimSpace(out.PhysicalRelation)
	out.Addition = strings.TrimSpace(out.Addition)
	out.SubtotalUnit = strings.TrimSpace(out.SubtotalUnit)
	out.MemberRefs = make([]string, len(in.MemberRefs))
	for i, member := range in.MemberRefs {
		out.MemberRefs[i] = strings.TrimSpace(member)
	}
	return out
}

// AnswerRelationClaimForAuthority returns the exact model-copyable wire
// projection of a typed relation authority. Diagnostic calibration fields on
// AnswerRelationAuthority (member values, overlap, comparison rules, repair
// direction) deliberately do not belong to the JSON claim schema.
//
// Keeping this projection here prevents prompt renderers from teaching a
// visually adjacent superset that strict tool decoding will later reject.
func AnswerRelationClaimForAuthority(authority AnswerRelationAuthority) AnswerRelationClaim {
	members := append([]string(nil), authority.MemberRefs...)
	if authority.Kind == AnswerRelationAuthorityCrossRulerBoundary {
		members = append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
	}
	claim := AnswerRelationClaim{
		AuthorityID:      authority.ID,
		MemberRefs:       members,
		PhysicalRelation: authority.PhysicalRelation,
		Addition:         authority.Addition,
		SubtotalUnit:     authority.SubtotalUnit,
	}
	if authority.SubtotalValue != nil {
		value := *authority.SubtotalValue
		claim.SubtotalValue = &value
	}
	return NormalizeAnswerRelationClaim(claim)
}

func CloneAnswerRelationClaims(in []AnswerRelationClaim) []AnswerRelationClaim {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerRelationClaim, len(in))
	for i, claim := range in {
		out[i] = claim
		out[i].MemberRefs = append([]string(nil), claim.MemberRefs...)
		if claim.SubtotalValue != nil {
			value := *claim.SubtotalValue
			out[i].SubtotalValue = &value
		}
	}
	return out
}

func AnswerRelationClaimsEqual(left, right []AnswerRelationClaim) bool {
	if len(left) != len(right) {
		return false
	}
	canon := func(in []AnswerRelationClaim) []AnswerRelationClaim {
		out := CloneAnswerRelationClaims(in)
		for i := range out {
			out[i] = NormalizeAnswerRelationClaim(out[i])
			sort.Strings(out[i].MemberRefs)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].AuthorityID < out[j].AuthorityID })
		return out
	}
	a, b := canon(left), canon(right)
	for i := range a {
		if a[i].AuthorityID != b[i].AuthorityID || a[i].PhysicalRelation != b[i].PhysicalRelation ||
			a[i].Addition != b[i].Addition || a[i].SubtotalUnit != b[i].SubtotalUnit ||
			!answerRelationSameSet(a[i].MemberRefs, b[i].MemberRefs) {
			return false
		}
		switch {
		case a[i].SubtotalValue == nil && b[i].SubtotalValue == nil:
		case a[i].SubtotalValue == nil || b[i].SubtotalValue == nil:
			return false
		case math.Abs(*a[i].SubtotalValue-*b[i].SubtotalValue) > 0.001:
			return false
		}
	}
	return true
}

func answerRelationSameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}

func answerRelationMembersKnown(members, allowed []string) bool {
	known := make(map[string]bool, len(allowed))
	for _, member := range allowed {
		known[member] = true
	}
	seen := map[string]bool{}
	for _, member := range members {
		if !known[member] || seen[member] {
			return false
		}
		seen[member] = true
	}
	return true
}

func answerRelationHasAny(members, group []string) bool {
	wanted := make(map[string]bool, len(group))
	for _, member := range group {
		wanted[member] = true
	}
	for _, member := range members {
		if wanted[member] {
			return true
		}
	}
	return false
}
