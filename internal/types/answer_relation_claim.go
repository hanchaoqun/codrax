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
	AnswerRelationAuthoritySameRulerSubtotal  AnswerRelationAuthorityKind = "same_ruler_subtotal"
	AnswerRelationAuthorityCrossRulerBoundary AnswerRelationAuthorityKind = "cross_ruler_boundary"
	AnswerRelationAuthorityClosedPartition    AnswerRelationAuthorityKind = "closed_partition"
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
		if account := projection.TargetStateAccount; account != nil && strings.TrimSpace(account.Subject) != "" && account.TotalMS > 0 {
			total := account.TotalMS
			out = append(out, AnswerRelationAuthority{
				ID:               "trace:target_state_partition:" + answerRelationTargetStateFingerprint(*account),
				Kind:             AnswerRelationAuthorityClosedPartition,
				MemberRefs:       []string{"running", "runnable", "sleep", "d_state", "io_wait"},
				PhysicalRelation: AnswerPhysicalRelationMutuallyExclusive,
				Addition:         AnswerRelationAdditionAuthorized,
				SubtotalValue:    &total, SubtotalUnit: "ms",
			})
		}
	}
	return out
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
	for index, raw := range claims {
		claim := NormalizeAnswerRelationClaim(raw)
		if claim.AuthorityID == "" {
			return fmt.Errorf("relation_claims[%d].authority_id is required", index)
		}
		if seen[claim.AuthorityID] {
			return fmt.Errorf("relation_claims[%d] duplicates authority_id=%q", index, claim.AuthorityID)
		}
		seen[claim.AuthorityID] = true
		authority, ok := byID[claim.AuthorityID]
		if !ok {
			return fmt.Errorf("relation_claims[%d].authority_id=%q has no typed relation authority in this investigation", index, claim.AuthorityID)
		}
		if err := validateAnswerRelationClaimAgainstAuthority(claim, authority); err != nil {
			return fmt.Errorf("relation_claims[%d] authority_id=%q: %w", index, claim.AuthorityID, err)
		}
	}
	if requireClosureAuthorities {
		for _, authority := range authorities {
			if authority.RequiredForClosure && !seen[authority.ID] {
				return fmt.Errorf("missing required model-authored relation claim for authority_id=%q", authority.ID)
			}
		}
	}
	return nil
}

func validateAnswerRelationClaimAgainstAuthority(claim AnswerRelationClaim, authority AnswerRelationAuthority) error {
	if claim.PhysicalRelation != authority.PhysicalRelation {
		return fmt.Errorf("physical_relation=%q; typed authority requires %q", claim.PhysicalRelation, authority.PhysicalRelation)
	}
	if claim.Addition != authority.Addition {
		return fmt.Errorf("addition=%q; typed authority requires %q", claim.Addition, authority.Addition)
	}
	switch authority.Kind {
	case AnswerRelationAuthorityCrossRulerBoundary:
		if claim.SubtotalValue != nil || claim.SubtotalUnit != "" {
			return fmt.Errorf("cross-ruler authority publishes no subtotal; omit subtotal_value and subtotal_unit")
		}
		if len(claim.MemberRefs) < 2 || !answerRelationMembersKnown(claim.MemberRefs, append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)) {
			return fmt.Errorf("member_refs must select known members from both ruler groups (left=%v right=%v)", authority.LeftMemberRefs, authority.RightMemberRefs)
		}
		if !answerRelationHasAny(claim.MemberRefs, authority.LeftMemberRefs) || !answerRelationHasAny(claim.MemberRefs, authority.RightMemberRefs) {
			return fmt.Errorf("member_refs must include at least one member from each ruler group (left=%v right=%v)", authority.LeftMemberRefs, authority.RightMemberRefs)
		}
	default:
		if !answerRelationSameSet(claim.MemberRefs, authority.MemberRefs) {
			return fmt.Errorf("member_refs=%v; typed authority requires the exact member set %v", claim.MemberRefs, authority.MemberRefs)
		}
		if authority.SubtotalValue == nil || claim.SubtotalValue == nil {
			return fmt.Errorf("subtotal_value must reproduce the typed published subtotal")
		}
		if math.Abs(*claim.SubtotalValue-*authority.SubtotalValue) > 0.001 {
			return fmt.Errorf("subtotal_value=%.6f; typed published subtotal is %.6f", *claim.SubtotalValue, *authority.SubtotalValue)
		}
		if claim.SubtotalUnit != authority.SubtotalUnit {
			return fmt.Errorf("subtotal_unit=%q; typed authority requires %q", claim.SubtotalUnit, authority.SubtotalUnit)
		}
	}
	return nil
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
