package types

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AnswerAggregateKind names a model-emitted aggregate fact that the
// finalizer should preserve as a structured exploration result. These
// facts are authored by the investigating model through
// emit_investigation_complete; the system stores and renders them but
// does not synthesize answer values from raw evidence on its own.
type AnswerAggregateKind string

const (
	AnswerAggregateUnknown             AnswerAggregateKind = ""
	AnswerAggregateTotalCount          AnswerAggregateKind = "total_count"
	AnswerAggregateUniqueCount         AnswerAggregateKind = "unique_count"
	AnswerAggregateGroupedCount        AnswerAggregateKind = "grouped_count"
	AnswerAggregateBucketCount         AnswerAggregateKind = "bucket_count"
	AnswerAggregateExcluded            AnswerAggregateKind = "excluded_count"
	AnswerAggregateScalar              AnswerAggregateKind = "scalar_value"
	AnswerAggregateMemberSet           AnswerAggregateKind = "member_set"
	AnswerAggregateNegativeSearch      AnswerAggregateKind = "negative_search"
	AnswerAggregateNegativeObservation AnswerAggregateKind = "negative_observation"
	AnswerAggregateBehaviorOutcome     AnswerAggregateKind = "behavior_outcome"
	AnswerAggregateErrorGranularity    AnswerAggregateKind = "error_granularity_verdict"
)

var allAnswerAggregateKinds = []AnswerAggregateKind{
	AnswerAggregateTotalCount,
	AnswerAggregateUniqueCount,
	AnswerAggregateGroupedCount,
	AnswerAggregateBucketCount,
	AnswerAggregateExcluded,
	AnswerAggregateScalar,
	AnswerAggregateMemberSet,
	AnswerAggregateNegativeSearch,
	AnswerAggregateNegativeObservation,
	AnswerAggregateBehaviorOutcome,
	AnswerAggregateErrorGranularity,
}

// AllAnswerAggregateKinds returns the canonical non-empty aggregate
// kind list. Returned values are safe for callers to mutate.
func AllAnswerAggregateKinds() []AnswerAggregateKind {
	return append([]AnswerAggregateKind(nil), allAnswerAggregateKinds...)
}

func (k AnswerAggregateKind) IsValid() bool {
	for _, declared := range allAnswerAggregateKinds {
		if k == declared {
			return true
		}
	}
	return false
}

// AnswerAggregateRole classifies how a model-emitted aggregate fact should be
// consumed downstream. It is structured metadata on the aggregate payload, not
// a natural-language guess over labels or answer prose.
type AnswerAggregateRole string

const (
	AnswerAggregateRoleUnknown            AnswerAggregateRole = ""
	AnswerAggregateRolePrincipalAnswer    AnswerAggregateRole = "principal_answer"
	AnswerAggregateRoleSupportingCoverage AnswerAggregateRole = "supporting_coverage"
	AnswerAggregateRoleAuditLedger        AnswerAggregateRole = "audit_ledger"
)

func (r AnswerAggregateRole) IsValid() bool {
	switch r {
	case AnswerAggregateRoleUnknown,
		AnswerAggregateRolePrincipalAnswer,
		AnswerAggregateRoleSupportingCoverage,
		AnswerAggregateRoleAuditLedger:
		return true
	}
	return false
}

func NormalizeAnswerAggregateRole(raw AnswerAggregateRole) AnswerAggregateRole {
	s := strings.ToLower(strings.TrimSpace(string(raw)))
	switch s {
	case "", "unknown":
		return AnswerAggregateRoleUnknown
	case "principal_answer", "principal", "primary_answer", "primary":
		return AnswerAggregateRolePrincipalAnswer
	case "supporting_coverage", "supporting", "support", "coverage", "context":
		return AnswerAggregateRoleSupportingCoverage
	case "audit_ledger", "audit", "ledger", "telemetry":
		return AnswerAggregateRoleAuditLedger
	default:
		return AnswerAggregateRoleUnknown
	}
}

func (r AnswerAggregateRole) IsPrincipal() bool {
	return r == AnswerAggregateRolePrincipalAnswer
}

func AnswerAggregateRolePriority(role AnswerAggregateRole) int {
	switch NormalizeAnswerAggregateRole(role) {
	case AnswerAggregateRolePrincipalAnswer:
		return 3
	case AnswerAggregateRoleSupportingCoverage:
		return 2
	case AnswerAggregateRoleAuditLedger:
		return 1
	default:
		return 0
	}
}

// AnswerAggregateDimension is one typed axis for an aggregate fact:
// scope=production, syntax=struct_literal, bucket=runtime, language=cpp,
// or any other model-authored dimension that was explicitly verified.
type AnswerAggregateDimension struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AnswerAggregateFact is the structured, model-emitted handoff for
// derived counts and grouped scalar facts. It intentionally keeps Value
// as a string so version numbers, ratios, hashes, and "4" all travel
// through the same exact-preservation path.
type AnswerAggregateFact struct {
	Kind        AnswerAggregateKind        `json:"kind"`
	Label       string                     `json:"label"`
	Value       string                     `json:"value"`
	Role        AnswerAggregateRole        `json:"role,omitempty"`
	Provenance  string                     `json:"provenance,omitempty"`
	Unit        string                     `json:"unit,omitempty"`
	Dimensions  []AnswerAggregateDimension `json:"dimensions,omitempty"`
	Members     []string                   `json:"members,omitempty"`
	MemberNotes []string                   `json:"member_notes,omitempty"`
	Excluded    []string                   `json:"excluded,omitempty"`
	SupportRefs []string                   `json:"support_refs,omitempty"`
}

// AnswerAggregateFactRef preserves the original aggregate_facts index while
// passing a curated view downstream. Support-lane evidence IDs and finalizer
// repair hints must continue to point at the model-emitted structured payload,
// not at a re-numbered system projection.
type AnswerAggregateFactRef struct {
	Index      int
	Fact       AnswerAggregateFact
	Role       AnswerAggregateRole
	Provenance string
}

type aggregateRelationMemberSetFact struct {
	index           int
	fact            AnswerAggregateFact
	left            map[string]bool
	leftKey         string
	supportCoverage int
}

const (
	maxAnswerAggregateFacts      = 16
	maxAnswerAggregateDimensions = 8
	maxAnswerAggregateMembers    = 200
	maxAnswerAggregateTextLen    = 240
)

// NormalizeAnswerAggregateFacts validates and canonicalizes aggregate
// facts emitted by the model. The checks are structural only: closed
// kind enum, required label/value, bounded list sizes, and whitespace
// trimming. They do not infer or repair values from evidence. The derived
// cardinality canonicalization is limited to structured exact-member carriers:
// member_set.value is derived from members, and grouped_count / bucket_count
// may derive a missing value from their own members when the model clearly used
// them as a per-group/per-bucket exact-member carrier. total_count and
// unique_count still require explicit values because their members can be
// samples rather than the full measured set.
func NormalizeAnswerAggregateFacts(in []AnswerAggregateFact) ([]AnswerAggregateFact, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxAnswerAggregateFacts {
		return nil, fmt.Errorf("aggregate_facts has %d entries; max %d", len(in), maxAnswerAggregateFacts)
	}
	out := make([]AnswerAggregateFact, 0, len(in))
	seen := map[string]bool{}
	for i, raw := range in {
		fact, err := normalizeAnswerAggregateFact(raw)
		if err != nil {
			return nil, fmt.Errorf("aggregate_facts[%d]: %w", i, err)
		}
		key := AnswerAggregateFactIdentity(fact)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fact)
	}
	if len(out) == 0 {
		return nil, nil
	}
	out = DropPartialAggregateExcludedLists(out)
	out = DropPartialAggregateCountMemberLists(out)
	if err := validateAggregateCountCardinality(out); err != nil {
		return nil, err
	}
	if err := validateAggregateFileLineMemberCompanions(out); err != nil {
		return nil, err
	}
	out = PruneAggregateMemberSetsByStructuredExclusions(out)
	out = reconcilePrincipalAggregateMemberSetSupersets(out)
	return out, nil
}

// DropPartialAggregateCountMemberLists keeps count facts as scalar aggregates
// when the model supplied examples or a partial member list. Count facts are
// not exact member-set carriers; if their members are complete, len(members)
// must equal value. When they are not complete, preserving the numeric value and
// omitting the partial members is safer than forcing another model rewrite or
// pretending the sample is the full set. Exact principal sets must use
// member_set, which this helper intentionally does not touch.
func DropPartialAggregateCountMemberLists(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) == 0 {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		switch out[i].Kind {
		case AnswerAggregateTotalCount, AnswerAggregateUniqueCount, AnswerAggregateGroupedCount, AnswerAggregateBucketCount:
		default:
			continue
		}
		if len(out[i].Members) == 0 {
			continue
		}
		want, ok, err := parseAggregateCountValue(out[i])
		if err != nil || !ok || want == len(out[i].Members) || want > maxAnswerAggregateMembers {
			continue
		}
		out[i].Members = nil
		out[i].MemberNotes = nil
		out[i].Provenance = appendAggregateFactProvenance(
			out[i].Provenance,
			"normalized:partial_count_members_omitted",
		)
		changed = true
	}
	if !changed {
		return cloneAnswerAggregateFacts(facts)
	}
	return out
}

// DropPartialAggregateExcludedLists keeps excluded_count as a count fact when
// the model supplied only examples or a prose placeholder in excluded[]. The
// numeric value is still useful support; treating the partial list as exact is
// what creates retries and misleading downstream tables. Exact excluded lists
// (len(excluded)==value) are preserved.
func DropPartialAggregateExcludedLists(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) == 0 {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		if out[i].Kind != AnswerAggregateExcluded || len(out[i].Excluded) == 0 {
			continue
		}
		want, ok, err := parseAggregateCountValue(out[i])
		if err != nil || !ok || want == len(out[i].Excluded) || want > maxAnswerAggregateMembers {
			continue
		}
		out[i].Excluded = nil
		out[i].Provenance = appendAggregateFactProvenance(
			out[i].Provenance,
			"normalized:partial_excluded_list_omitted",
		)
		changed = true
	}
	if !changed {
		return cloneAnswerAggregateFacts(facts)
	}
	return out
}

// MergeAnswerAggregateFacts folds multiple explorer/fork aggregate handoffs
// into one stable typed payload. It preserves exact facts and unions compatible
// member_set rows that share the same explicit structured bucket (kind, label,
// role, dimensions, unit). Near-duplicate bucket labels are reconciled by the
// typed superset arbiter below instead of being silently folded here, so
// support/audit context remains inspectable. This intentionally never derives
// members from raw evidence or prose; it only combines model-emitted structured
// member_set payloads from sibling explore dispatches.
func MergeAnswerAggregateFacts(groups ...[]AnswerAggregateFact) []AnswerAggregateFact {
	type memberSlot struct {
		index int
	}
	var out []AnswerAggregateFact
	identitySeen := map[string]bool{}
	memberSlots := map[string]memberSlot{}
	for _, group := range groups {
		for _, raw := range group {
			fact, err := normalizeAnswerAggregateFact(raw)
			if err != nil {
				continue
			}
			identity := AnswerAggregateFactIdentity(fact)
			if AnswerAggregateFactCarriesCompleteMemberSet(fact) {
				if identitySeen[identity] {
					continue
				}
				key := answerAggregateMemberSetMergeKey(fact)
				if slot, ok := memberSlots[key]; ok {
					out[slot.index] = mergeAnswerAggregateMemberSet(out[slot.index], fact)
					identitySeen[AnswerAggregateFactIdentity(out[slot.index])] = true
					continue
				}
				memberSlots[key] = memberSlot{index: len(out)}
				identitySeen[identity] = true
			} else {
				if identitySeen[identity] {
					continue
				}
				identitySeen[identity] = true
			}
			out = append(out, fact)
		}
	}
	if len(out) == 0 {
		return nil
	}
	out = PruneAggregateMemberSetsByStructuredExclusions(out)
	out = reconcilePrincipalAggregateMemberSetSupersets(out)
	out = demoteImplicitAggregateMemberSetsShadowedByExplicitPrincipal(out)
	return cloneAnswerAggregateFacts(out)
}

// ReconcilePrincipalAggregateMemberSetSupersets reruns the typed member-set
// arbiter after downstream source-of-truth filters have changed the visible
// member sets, for example after a typed public/exported visibility policy
// prunes private helpers. This is deliberately exported for pipeline layers
// that own request-aware filters; the arbiter itself remains prompt-agnostic.
func ReconcilePrincipalAggregateMemberSetSupersets(facts []AnswerAggregateFact) []AnswerAggregateFact {
	facts = NormalizeAnswerAggregateMemberSetSurfaces(facts)
	out := reconcilePrincipalAggregateMemberSetSupersets(facts)
	out = demoteImplicitAggregateMemberSetsShadowedByExplicitPrincipal(out)
	return cloneAnswerAggregateFacts(out)
}

// NormalizeAnswerAggregateMemberSetSurfaces separates answer-member identity
// from citation/support location surfaces for already-accepted aggregate facts.
// It is safe to run at any later pipeline boundary: it consumes only
// structured member_set.members/support_refs fields, never raw prose, user
// text, or model thoughts.
func NormalizeAnswerAggregateMemberSetSurfaces(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) == 0 {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	for i := range out {
		if !answerAggregateFactCarriesCompleteMemberSet(out[i]) || len(out[i].Members) == 0 {
			continue
		}
		out[i].Members, out[i].SupportRefs, out[i].MemberNotes = normalizeAggregateMemberSetMemberSupportNoteSurfaces(out[i].Members, out[i].SupportRefs, out[i].MemberNotes)
		if out[i].Kind == AnswerAggregateMemberSet {
			out[i].Value = strconv.Itoa(len(out[i].Members))
			out[i].Label = normalizeAggregateMemberSetLabelCardinality(out[i].Label, len(out[i].Members))
		}
	}
	return out
}

// PruneAggregateMemberSetsByStructuredExclusions reconciles two structured
// aggregate lanes emitted by exploration: an exclusion ledger
// (`excluded_count.excluded[]` or `excluded_count.members[]`) and a concrete
// member_set. When the same excluded member also appears inside a principal
// member_set, the exclusion ledger is the narrower typed signal and the member
// set is repaired at source. This consumes only model-emitted structured
// arrays; it never scans raw user text, thoughts, or rendered answer prose.
func PruneAggregateMemberSetsByStructuredExclusions(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) == 0 {
		return cloneAnswerAggregateFacts(facts)
	}
	excluded := aggregateStructuredExcludedMemberKeys(facts)
	if len(excluded) == 0 {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		if out[i].Kind == AnswerAggregateExcluded ||
			!answerAggregateFactCarriesCompleteMemberSet(out[i]) ||
			len(out[i].Members) == 0 {
			continue
		}
		keptMembers := make([]string, 0, len(out[i].Members))
		keptRefs := make([]string, 0, len(out[i].SupportRefs))
		keptNotes := make([]string, 0, len(out[i].MemberNotes))
		removed := 0
		for memberIdx, member := range out[i].Members {
			if aggregateMemberMatchesStructuredExclusion(member, excluded) {
				removed++
				continue
			}
			keptMembers = append(keptMembers, member)
			if memberIdx < len(out[i].SupportRefs) {
				keptRefs = append(keptRefs, out[i].SupportRefs[memberIdx])
			}
			if memberIdx < len(out[i].MemberNotes) {
				keptNotes = append(keptNotes, out[i].MemberNotes[memberIdx])
			}
		}
		if removed == 0 {
			continue
		}
		changed = true
		out[i].Members = keptMembers
		out[i].SupportRefs = keptRefs
		out[i].MemberNotes = trimTrailingEmptyAggregateStrings(keptNotes)
		if len(keptMembers) == 0 {
			out[i].Role = AnswerAggregateRoleSupportingCoverage
			out[i].Value = "0"
			continue
		}
		out[i].Value = strconv.Itoa(len(keptMembers))
	}
	if !changed {
		return cloneAnswerAggregateFacts(facts)
	}
	return out
}

func aggregateStructuredExcludedMemberKeys(facts []AnswerAggregateFact) map[string]bool {
	keys := map[string]bool{}
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateExcluded {
			continue
		}
		for _, member := range fact.Excluded {
			addAggregateStructuredExclusionKeys(keys, member)
		}
		for _, member := range fact.Members {
			addAggregateStructuredExclusionKeys(keys, member)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

func addAggregateStructuredExclusionKeys(keys map[string]bool, raw string) {
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(raw) {
		addAggregateStructuredExclusionKey(keys, candidate)
		if base, _, ok := AnswerAggregateDecoratedLabelParts(candidate); ok {
			addAggregateStructuredExclusionKey(keys, base)
		}
		if label, _, ok := ParseAnswerSupportRefMemberLocation(candidate); ok {
			addAggregateStructuredExclusionKey(keys, label)
		}
	}
	addAggregateStructuredExclusionKey(keys, raw)
	addAggregateStructuredExclusionKey(keys, aggregateMemberKey(raw))
	addAggregateStructuredExclusionKey(keys, NormalizedSurfaceSymbolTail(raw))
}

func addAggregateStructuredExclusionKey(keys map[string]bool, raw string) {
	key := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "`"))
	if key != "" {
		keys[key] = true
	}
}

func aggregateMemberMatchesStructuredExclusion(member string, excluded map[string]bool) bool {
	if len(excluded) == 0 {
		return false
	}
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(member) {
		if aggregateStructuredExclusionKeyExists(candidate, excluded) {
			return true
		}
		if base, _, ok := AnswerAggregateDecoratedLabelParts(candidate); ok &&
			aggregateStructuredExclusionKeyExists(base, excluded) {
			return true
		}
		if label, _, ok := ParseAnswerSupportRefMemberLocation(candidate); ok &&
			aggregateStructuredExclusionKeyExists(label, excluded) {
			return true
		}
	}
	return aggregateStructuredExclusionKeyExists(member, excluded)
}

func aggregateStructuredExclusionKeyExists(raw string, excluded map[string]bool) bool {
	for _, candidate := range []string{raw, aggregateMemberKey(raw), NormalizedSurfaceSymbolTail(raw)} {
		key := strings.ToLower(strings.Trim(strings.TrimSpace(candidate), "`"))
		if key != "" && excluded[key] {
			return true
		}
	}
	return false
}

func demoteImplicitAggregateMemberSetsShadowedByExplicitPrincipal(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) < 2 {
		return facts
	}
	explicit := explicitPrincipalAggregateMemberSetKeys(facts)
	if len(explicit) == 0 {
		return facts
	}
	out := cloneAnswerAggregateFacts(facts)
	for i := range out {
		if NormalizeAnswerAggregateRole(out[i].Role) != AnswerAggregateRoleUnknown ||
			!answerAggregateFactCarriesCompleteMemberSet(out[i]) {
			continue
		}
		candidate := aggregateMemberSetProjectionKeySet(out[i].Members)
		if len(candidate) == 0 {
			continue
		}
		for _, principal := range explicit {
			if aggregateMemberProjectionSetSubsumes(candidate, principal) {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				break
			}
		}
	}
	return out
}

// reconcilePrincipalAggregateMemberSetSupersets resolves a typed handoff
// conflict that can happen when parallel explorers or retry repairs emit two
// complete principal member sets for the same structured bucket, and a later
// set is a strict superset of an earlier stale set. The larger set is the
// higher-fidelity accepted handoff; the smaller one remains available as
// support/audit context but must not compete as a second principal slate.
//
// The compatibility check is deliberately narrow: it consumes only structured
// aggregate labels, dimensions, units, and member arrays. It never reads the
// user prompt, model thinking, closure prose, or rendered answer text.
func reconcilePrincipalAggregateMemberSetSupersets(facts []AnswerAggregateFact) []AnswerAggregateFact {
	if len(facts) < 2 {
		return facts
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		if !answerAggregateFactCanYieldToSuperset(out[i]) {
			continue
		}
		candidateSet := aggregateMemberSetProjectionKeySet(out[i].Members)
		if len(candidateSet) == 0 {
			continue
		}
		for j := range out {
			if i == j || !answerAggregateFactCanActAsMemberSuperset(out[j]) {
				continue
			}
			if !aggregateMemberSetYieldAllowed(out[i], out[j]) {
				continue
			}
			if !aggregateMemberSetFactsSameBucketForSuperset(out[i], out[j]) {
				continue
			}
			superset := aggregateMemberSetProjectionKeySet(out[j].Members)
			if !aggregateMemberProjectionStrictSubsetOf(candidateSet, superset) {
				continue
			}
			if !aggregateMemberSetSupportCompatibleForSuperset(out[i], out[j]) {
				continue
			}
			out[i].Role = AnswerAggregateRoleSupportingCoverage
			out[i].Provenance = appendAggregateFactProvenance(
				out[i].Provenance,
				"demoted:shadowed_by_principal_member_set_superset",
			)
			if AnswerAggregateFactRoleForRequest(out[j], nil) != AnswerAggregateRolePrincipalAnswer {
				out[j].Role = AnswerAggregateRolePrincipalAnswer
			}
			changed = true
			break
		}
	}
	if !changed {
		return facts
	}
	return out
}

func answerAggregateFactCanYieldToSuperset(fact AnswerAggregateFact) bool {
	return answerAggregateFactCarriesCompleteMemberSet(fact) &&
		AnswerAggregateFactRoleForRequest(fact, nil) == AnswerAggregateRolePrincipalAnswer &&
		len(fact.Members) > 0
}

func answerAggregateFactCanActAsMemberSuperset(fact AnswerAggregateFact) bool {
	return answerAggregateFactCarriesCompleteMemberSet(fact) &&
		len(fact.Members) > 0 &&
		AnswerAggregateFactRoleForRequest(fact, nil) != AnswerAggregateRoleAuditLedger
}

func aggregateMemberSetYieldAllowed(candidate, superset AnswerAggregateFact) bool {
	candidateRole := NormalizeAnswerAggregateRole(candidate.Role)
	supersetRole := NormalizeAnswerAggregateRole(superset.Role)
	if candidateRole == AnswerAggregateRolePrincipalAnswer &&
		supersetRole != AnswerAggregateRolePrincipalAnswer &&
		supersetRole != AnswerAggregateRoleSupportingCoverage {
		return false
	}
	return true
}

func aggregateMemberSetFactsSameBucketForSuperset(a, b AnswerAggregateFact) bool {
	if !aggregateFactDimensionsCompatible(a.Dimensions, b.Dimensions) {
		return false
	}
	if strings.TrimSpace(a.Unit) != "" && strings.TrimSpace(b.Unit) != "" &&
		!strings.EqualFold(strings.TrimSpace(a.Unit), strings.TrimSpace(b.Unit)) {
		return false
	}
	return aggregateMemberSetLabelsCompatibleForSuperset(a.Label, b.Label)
}

func aggregateMemberSetLabelsCompatibleForSuperset(a, b string) bool {
	ac := canonicalAggregateMemberSetLabelCore(a)
	bc := canonicalAggregateMemberSetLabelCore(b)
	if ac == "" || bc == "" {
		return false
	}
	if ac == bc {
		return true
	}
	// Allow narrow display drift such as "Kind 常量" versus "Kind const"
	// only when one canonical label fully contains the other after generic
	// list/member suffixes have been removed. A shared broad prefix like
	// "public" is intentionally insufficient, so category subsets (for
	// example public functions inside public symbols) remain independent.
	return len([]rune(ac)) >= 4 && len([]rune(bc)) >= 4 &&
		(strings.Contains(ac, bc) || strings.Contains(bc, ac))
}

func canonicalAggregateMemberSetLabelCore(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return ""
	}
	suffixes := []string{
		"members", "member", "items", "item", "inventory", "list", "set", "complete", "full",
		"成员", "清单", "列表", "集合", "完整", "全部", "所有",
	}
	prefixes := []string{"complete", "full", "all", "全部", "所有"}
	changed := true
	for changed {
		changed = false
		for _, suffix := range suffixes {
			if suffix != "" && strings.HasSuffix(s, suffix) && len(s) > len(suffix) {
				s = strings.TrimSuffix(s, suffix)
				changed = true
			}
		}
		for _, prefix := range prefixes {
			if prefix != "" && strings.HasPrefix(s, prefix) && len(s) > len(prefix) {
				s = strings.TrimPrefix(s, prefix)
				changed = true
			}
		}
	}
	return s
}

func aggregateMemberProjectionStrictSubsetOf(candidate, superset map[string]bool) bool {
	if len(candidate) == 0 || len(superset) == 0 || len(candidate) >= len(superset) {
		return false
	}
	for member := range candidate {
		if !superset[member] {
			return false
		}
	}
	return true
}

func aggregateMemberSetSupportCompatibleForSuperset(candidate, superset AnswerAggregateFact) bool {
	candidateCoverage := aggregateMemberSetSupportCoverage(candidate)
	supersetCoverage := aggregateMemberSetSupportCoverage(superset)
	if candidateCoverage == 0 {
		return true
	}
	if supersetCoverage == 0 {
		return false
	}
	if supersetCoverage >= candidateCoverage {
		return true
	}
	return supersetCoverage == len(superset.Members)
}

func appendAggregateFactProvenance(current, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return current
	}
	if current == "" {
		return addition
	}
	for _, part := range strings.Split(current, ";") {
		if strings.TrimSpace(part) == addition {
			return current
		}
	}
	return current + "; " + addition
}

func explicitPrincipalAggregateMemberSetKeys(facts []AnswerAggregateFact) []map[string]bool {
	var out []map[string]bool
	for _, fact := range facts {
		if NormalizeAnswerAggregateRole(fact.Role) != AnswerAggregateRolePrincipalAnswer ||
			!answerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		keys := aggregateMemberSetProjectionKeySet(fact.Members)
		if len(keys) >= 2 {
			out = append(out, keys)
		}
	}
	return out
}

func aggregateMemberProjectionSetSubsumes(candidate, principal map[string]bool) bool {
	if len(candidate) == 0 || len(principal) == 0 || len(candidate) < len(principal) {
		return false
	}
	for member := range principal {
		if !candidate[member] {
			return false
		}
	}
	return true
}

func aggregateMemberSetProjectionKeySet(members []string) map[string]bool {
	keys := make(map[string]bool, len(members))
	for _, member := range members {
		if key := aggregateMemberSetProjectionMemberKey(member); key != "" {
			keys[key] = true
		}
	}
	return keys
}

func answerAggregateMemberSetMergeKey(fact AnswerAggregateFact) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(string(fact.Kind))))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Label)))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(string(NormalizeAnswerAggregateRole(fact.Role)))))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Unit)))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(renderAggregateDimensionsKey(fact.Dimensions)))
	return b.String()
}

func mergeAnswerAggregateMemberSet(dst, src AnswerAggregateFact) AnswerAggregateFact {
	dst = cloneAnswerAggregateFacts([]AnswerAggregateFact{dst})[0]
	memberKeys := make(map[string]bool, len(dst.Members)+len(src.Members))
	for _, member := range dst.Members {
		key := AnswerAggregateMemberSurfaceKey(member)
		if key != "" {
			memberKeys[key] = true
		}
	}
	for i, member := range src.Members {
		member = strings.TrimSpace(member)
		if member == "" {
			continue
		}
		key := AnswerAggregateMemberSurfaceKey(member)
		if key == "" || memberKeys[key] {
			continue
		}
		memberKeys[key] = true
		dst.Members = append(dst.Members, member)
		if len(src.SupportRefs) > i {
			dst.SupportRefs = appendAggregateSupportRefAtMemberIndex(dst.SupportRefs, len(dst.Members)-1, src.SupportRefs[i])
		}
		if len(src.MemberNotes) > i {
			dst.MemberNotes = appendAggregateStringAtMemberIndex(dst.MemberNotes, len(dst.Members)-1, src.MemberNotes[i])
		}
	}
	if AnswerAggregateRolePriority(src.Role) > AnswerAggregateRolePriority(dst.Role) {
		dst.Role = NormalizeAnswerAggregateRole(src.Role)
	}
	if dst.Provenance == "" {
		dst.Provenance = src.Provenance
	}
	if dst.Unit == "" {
		dst.Unit = src.Unit
	}
	dst.Value = strconv.Itoa(len(dst.Members))
	dst.Label = normalizeAggregateMemberSetLabelCardinality(dst.Label, len(dst.Members))
	return dst
}

func normalizeAggregateMemberSetLabelCardinality(label string, memberCount int) string {
	label = strings.TrimSpace(label)
	if label == "" || memberCount <= 0 {
		return label
	}
	out := removeConflictingAggregateLabelCountSegments(label, memberCount)
	out = strings.Join(strings.Fields(strings.TrimSpace(out)), " ")
	if out == "" {
		return label
	}
	return out
}

type aggregateLabelBracketPair struct {
	open  rune
	close rune
}

func removeConflictingAggregateLabelCountSegments(label string, memberCount int) string {
	pairs := []aggregateLabelBracketPair{{'（', '）'}, {'(', ')'}, {'[', ']'}, {'【', '】'}}
	var b strings.Builder
	runes := []rune(label)
	for i := 0; i < len(runes); i++ {
		pair, ok := aggregateLabelOpeningBracket(runes[i], pairs)
		if !ok {
			b.WriteRune(runes[i])
			continue
		}
		closeIdx := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == pair.close {
				closeIdx = j
				break
			}
		}
		if closeIdx < 0 {
			b.WriteRune(runes[i])
			continue
		}
		segment := string(runes[i+1 : closeIdx])
		if aggregateLabelCountSegmentConflicts(segment, memberCount) {
			i = closeIdx
			continue
		}
		b.WriteRune(runes[i])
		b.WriteString(segment)
		b.WriteRune(pair.close)
		i = closeIdx
	}
	return b.String()
}

func aggregateLabelOpeningBracket(r rune, pairs []aggregateLabelBracketPair) (aggregateLabelBracketPair, bool) {
	for _, pair := range pairs {
		if r == pair.open {
			return pair, true
		}
	}
	return aggregateLabelBracketPair{}, false
}

func aggregateLabelCountSegmentConflicts(segment string, memberCount int) bool {
	values := aggregateLabelSegmentIntegers(segment)
	if len(values) == 0 || !aggregateLabelSegmentLooksLikeCount(segment) {
		return false
	}
	for _, value := range values {
		if value == memberCount {
			return false
		}
	}
	return true
}

func aggregateLabelSegmentIntegers(segment string) []int {
	var out []int
	var digits strings.Builder
	flush := func() {
		if digits.Len() == 0 {
			return
		}
		if n, err := strconv.Atoi(digits.String()); err == nil {
			out = append(out, n)
		}
		digits.Reset()
	}
	for _, r := range segment {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func aggregateLabelSegmentLooksLikeCount(segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" {
		return false
	}
	if _, err := strconv.Atoi(segment); err == nil {
		return true
	}
	for _, marker := range []string{
		"个", "项", "条", "种",
		"member", "members", "item", "items", "entry", "entries", "row", "rows",
		"symbol", "symbols", "const", "consts", "constant", "constants",
		"function", "functions", "func", "funcs", "type", "types",
	} {
		if strings.Contains(segment, marker) {
			return true
		}
	}
	return false
}

func appendAggregateSupportRefAtMemberIndex(refs []string, memberIndex int, ref string) []string {
	ref = strings.TrimSpace(ref)
	for len(refs) < memberIndex {
		refs = append(refs, "")
	}
	if len(refs) == memberIndex {
		refs = append(refs, ref)
		return refs
	}
	if refs[memberIndex] == "" {
		refs[memberIndex] = ref
	}
	return refs
}

func appendAggregateStringAtMemberIndex(values []string, memberIndex int, value string) []string {
	value = trimAggregateText(value)
	for len(values) < memberIndex {
		values = append(values, "")
	}
	if len(values) == memberIndex {
		values = append(values, value)
		return values
	}
	if values[memberIndex] == "" {
		values[memberIndex] = value
	} else {
		values[memberIndex] = MergeEvidenceSummaries(values[memberIndex], value)
	}
	return values
}

// PrincipalAggregateMemberSetFactRefs returns the member_set facts that should
// act as the user-visible principal slate. When the model emits both a left-axis
// set ("packages") and a richer relation set ("package -> entry"), the relation
// set subsumes the left-axis set: the axis is useful coverage/audit context, but
// requiring it as a second principal lane makes finalization duplicate rows or
// cite package labels as if they were function definitions.
//
// This is intentionally language-neutral. It consumes only model-authored
// aggregate member surfaces plus the shared compact relation parser; it does not
// inspect raw prompts, thinking text, or repo-language-specific syntax.
func PrincipalAggregateMemberSetFactRefs(facts []AnswerAggregateFact) []AnswerAggregateFactRef {
	if len(facts) == 0 {
		return nil
	}
	var relationFacts []aggregateRelationMemberSetFact
	for idx, fact := range facts {
		if !answerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if AnswerAggregateFactRoleForRequest(fact, nil) != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		left, ok := aggregateRelationLeftAxisSet(fact)
		if !ok {
			continue
		}
		relationFacts = append(relationFacts, aggregateRelationMemberSetFact{
			index:           idx,
			fact:            fact,
			left:            left,
			leftKey:         aggregateAxisSetKey(left),
			supportCoverage: aggregateMemberSetSupportCoverage(fact),
		})
	}
	skipRelationAlternate := aggregateRelationAlternateMemberSetSkips(relationFacts)
	out := make([]AnswerAggregateFactRef, 0, len(facts))
	for idx, fact := range facts {
		if !answerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if role := AnswerAggregateFactRoleForRequest(fact, nil); role != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		if aggregateMemberSetIsSubsumedLeftAxis(fact, relationFacts) {
			continue
		}
		if skipRelationAlternate[idx] {
			continue
		}
		out = append(out, answerAggregateFactRef(idx, fact, AnswerAggregateRolePrincipalAnswer))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PrincipalAggregateMemberSetFactRefsForRequest returns the principal
// member_set rows for a concrete request shape. The base helper treats every
// complete member_set as principal unless another relation member_set clearly
// subsumes it. That is correct for enumeration/relation answer shapes, but a
// model can also use member_set as a coverage ledger in explanation questions
// (for example, "files I inspected"). For prose/mechanism answer shapes, a pure
// source-path set is context evidence rather than the answer slate, so this
// request-aware projection keeps it out of hard finalizer gates.
//
// The filter is structural: typed RequestModel traits plus source/config path
// surfaces. It does not inspect the raw user prompt, model prose, labels such
// as "core files", or repository-language-specific keywords.
func PrincipalAggregateMemberSetFactRefsForRequest(facts []AnswerAggregateFact, rm *RequestModel) []AnswerAggregateFactRef {
	refs := PrincipalAggregateMemberSetFactRefs(facts)
	if len(refs) == 0 || rm == nil {
		return refs
	}
	hasRicherPrincipalMemberSet := aggregateFactRefsContainPrincipalMemberSet(refs)
	if !hasRicherPrincipalMemberSet && aggregateRequestRequiresPathMemberSetAsPrincipal(*rm) {
		return refs
	}
	out := make([]AnswerAggregateFactRef, 0, len(refs))
	for _, ref := range refs {
		if hasRicherPrincipalMemberSet && aggregateFactIsCountCompleteMemberCarrier(ref.Fact) {
			continue
		}
		if aggregateFactOutsideRequestedSourceInventoryScope(ref.Fact, rm) {
			continue
		}
		if AggregateCountFactMembersAreSupportOnlyForRequest(rm, ref.Fact) {
			continue
		}
		if AggregateMemberSetIsScalarCountSupport(rm, ref.Fact) {
			continue
		}
		if AggregateMemberSetIsScalarRoleLookupSupport(rm, ref.Fact) {
			continue
		}
		if AggregateMemberSetIsNoHitWindowSupport(rm, facts, ref.Index) {
			continue
		}
		if AggregateMemberSetIsMechanismNarrativeSupport(rm, ref.Fact) {
			continue
		}
		if AggregateFactIsNarrativeHistorySupport(rm, ref.Fact) {
			continue
		}
		explicitRole := NormalizeAnswerAggregateRole(ref.Fact.Role)
		role := AnswerAggregateFactRoleForRequest(ref.Fact, rm)
		if role != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		ref.Role = role
		if explicitRole == AnswerAggregateRoleUnknown {
			if aggregateCountMemberSetIsNarrativeCoverageOnly(ref.Fact, *rm) {
				continue
			}
			if aggregateMemberSetIsSourcePathCoverageOnly(ref.Fact, *rm) {
				continue
			}
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EffectiveCitationFloorForPrincipalMemberSets caps a generic citation_count_ge
// floor by the exact principal member-set cardinality when the investigation has
// already emitted a structured answer-grade member_set.
//
// This is intentionally a cap, not a replacement for citations. A one-member
// exact registry answer with one exact citation should not be forced to invent
// two unrelated citations just because the scenario template defaults to a
// broad architecture floor of three. Conversely, a ten-member inventory still
// keeps the template's citation breadth because the principal-member count is
// above the base floor.
//
// The signal is typed-only: model-emitted aggregate_facts members + their
// request-aware principal role. It never scans the user request or answer prose,
// and demoted narrative/support member_sets are filtered out by
// PrincipalAggregateMemberSetFactRefsForRequest before they can affect gates.
func EffectiveCitationFloorForPrincipalMemberSets(base int, facts []AnswerAggregateFact, rm *RequestModel) (int, bool) {
	if base <= 1 || rm == nil || len(facts) == 0 {
		return base, false
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(facts, rm)
	if len(refs) == 0 {
		return base, false
	}
	keys := make(map[string]bool)
	for _, ref := range refs {
		if AnswerAggregateFactRoleForRequest(ref.Fact, rm) != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		labelKey := canonicalAggregateMemberSetLabelCore(ref.Fact.Label)
		for _, member := range ref.Fact.Members {
			memberKey := aggregateMemberSetProjectionMemberKey(member)
			if memberKey == "" {
				memberKey = strings.ToLower(strings.TrimSpace(member))
			}
			if memberKey == "" {
				continue
			}
			keys[labelKey+"\x00"+memberKey] = true
		}
	}
	if len(keys) == 0 || len(keys) >= base {
		return base, false
	}
	return len(keys), true
}

// ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols resolves a typed
// authority conflict before finalization: exploration may emit broad count-like
// aggregate rows with members, while extraction later emits a complete
// emit_answer_symbol slate for the same enumeration. Projection is allowed for
// implicit count/group ledgers; a model-emitted member_set is the
// investigator's accepted handoff and must not be narrowed by a later
// answer-symbol slate, even when the older payload omitted an explicit role
// field. The one exception is a singleton member_set that has zero overlap with
// a complete answer-symbol slate while sibling principal sets do overlap: that
// shape is metadata / count-basis bookkeeping, not a principal enumeration row.
// Answer symbols can provide anchors and presentation structure, but they
// cannot subtract verified principal members. The projection consumes only
// structured member strings and AnswerSymbol names; it never scans user
// wording or model prose.
func ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
	facts []AnswerAggregateFact,
	rm *RequestModel,
	symbols []AnswerSymbol,
	claim CompletenessClaim,
) []AnswerAggregateFact {
	facts = PruneAggregateMemberSetsByStructuredExclusions(facts)
	if len(facts) == 0 || len(symbols) == 0 || claim != CompletenessComplete {
		return facts
	}
	symbolKeys := answerSymbolSurfaceKeySet(symbols)
	if len(symbolKeys) == 0 {
		return facts
	}
	hasPrincipalSymbolOverlap := principalAggregateFactsHaveMemberLevelAnswerSymbolOverlap(facts, rm, symbolKeys)
	if !hasPrincipalSymbolOverlap {
		return facts
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	seenPrincipalSets := map[string]int{}
	for i := range out {
		if !answerAggregateFactCarriesCompleteMemberSet(out[i]) ||
			AnswerAggregateFactRoleForRequest(out[i], rm) != AnswerAggregateRolePrincipalAnswer ||
			len(out[i].Members) == 0 {
			continue
		}
		wasCompleteMemberSetCarrier := answerAggregateFactCarriesCompleteMemberSet(out[i])
		if out[i].Kind == AnswerAggregateMemberSet ||
			NormalizeAnswerAggregateRole(out[i].Role) == AnswerAggregateRolePrincipalAnswer {
			if aggregateMemberSetIsSingletonMetadataOutsideAnswerSymbols(out[i], rm, symbolKeys, hasPrincipalSymbolOverlap) {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:singleton_metadata_outside_complete_answer_symbol_slate",
				)
				changed = true
				continue
			}
			setKey := aggregateMemberSetSymbolProjectionKey(out[i].Members)
			if setKey == "" {
				continue
			}
			if prev, ok := seenPrincipalSets[setKey]; ok {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				if NormalizeAnswerAggregateRole(out[prev].Role) != AnswerAggregateRolePrincipalAnswer {
					out[prev].Role = AnswerAggregateRolePrincipalAnswer
				}
				changed = true
				continue
			}
			seenPrincipalSets[setKey] = i
			continue
		}
		keptMembers := make([]string, 0, len(out[i].Members))
		keptRefs := make([]string, 0, len(out[i].SupportRefs))
		for memberIdx, member := range out[i].Members {
			if !aggregateMemberCoveredByAnswerSymbolSet(member, symbolKeys) {
				continue
			}
			keptMembers = append(keptMembers, member)
			if memberIdx < len(out[i].SupportRefs) {
				keptRefs = append(keptRefs, out[i].SupportRefs[memberIdx])
			}
		}
		switch {
		case len(keptMembers) == 0:
			out[i].Role = AnswerAggregateRoleSupportingCoverage
			changed = true
			continue
		case len(keptMembers) != len(out[i].Members):
			out[i].Members = keptMembers
			out[i].SupportRefs = keptRefs
			if out[i].Kind == AnswerAggregateMemberSet || wasCompleteMemberSetCarrier {
				out[i].Value = strconv.Itoa(len(keptMembers))
			}
			changed = true
		}
		setKey := aggregateMemberSetSymbolProjectionKey(out[i].Members)
		if setKey == "" {
			continue
		}
		if prev, ok := seenPrincipalSets[setKey]; ok {
			out[i].Role = AnswerAggregateRoleSupportingCoverage
			if NormalizeAnswerAggregateRole(out[prev].Role) != AnswerAggregateRolePrincipalAnswer {
				out[prev].Role = AnswerAggregateRolePrincipalAnswer
			}
			changed = true
			continue
		}
		seenPrincipalSets[setKey] = i
	}
	if demoteConflictingPrincipalMemberSetsByCompleteAnswerSymbols(out, rm, symbolKeys, claim) {
		changed = true
	}
	if !changed {
		return facts
	}
	return out
}

func aggregateMemberSetIsSingletonMetadataOutsideAnswerSymbols(
	fact AnswerAggregateFact,
	rm *RequestModel,
	symbolKeys map[string]bool,
	hasPrincipalSymbolOverlap bool,
) bool {
	if !hasPrincipalSymbolOverlap || rm == nil || len(symbolKeys) == 0 {
		return false
	}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(*rm) || len(fact.Members) != 1 {
		return false
	}
	return aggregateMemberSetAnswerSymbolCoverage(fact, symbolKeys) == 0
}

func demoteConflictingPrincipalMemberSetsByCompleteAnswerSymbols(
	facts []AnswerAggregateFact,
	rm *RequestModel,
	symbolKeys map[string]bool,
	claim CompletenessClaim,
) bool {
	if len(facts) < 2 || len(symbolKeys) == 0 || claim != CompletenessComplete {
		return false
	}
	changed := false
	for i := range facts {
		if !aggregateMemberSetEligibleForAnswerSymbolTieBreak(facts[i], rm) {
			continue
		}
		iCoverage := aggregateMemberSetAnswerSymbolCoverage(facts[i], symbolKeys)
		for j := range facts {
			if i == j || !aggregateMemberSetEligibleForAnswerSymbolTieBreak(facts[j], rm) {
				continue
			}
			if len(facts[i].Members) != len(facts[j].Members) ||
				len(facts[i].Members) == 0 ||
				!aggregateMemberSetFactsSameBucketForSuperset(facts[i], facts[j]) {
				continue
			}
			jCoverage := aggregateMemberSetAnswerSymbolCoverage(facts[j], symbolKeys)
			if iCoverage == len(facts[i].Members) && jCoverage < len(facts[j].Members) {
				facts[j].Role = AnswerAggregateRoleSupportingCoverage
				facts[j].Provenance = appendAggregateFactProvenance(
					facts[j].Provenance,
					"demoted:conflicts_with_complete_answer_symbol_slate",
				)
				changed = true
			}
		}
	}
	return changed
}

func aggregateMemberSetEligibleForAnswerSymbolTieBreak(fact AnswerAggregateFact, rm *RequestModel) bool {
	return fact.Kind == AnswerAggregateMemberSet &&
		answerAggregateFactCarriesCompleteMemberSet(fact) &&
		AnswerAggregateFactRoleForRequest(fact, rm) == AnswerAggregateRolePrincipalAnswer
}

func aggregateMemberSetAnswerSymbolCoverage(fact AnswerAggregateFact, symbolKeys map[string]bool) int {
	if len(symbolKeys) == 0 || len(fact.Members) == 0 {
		return 0
	}
	covered := 0
	for _, member := range fact.Members {
		if aggregateMemberCoveredByAnswerSymbolSet(member, symbolKeys) {
			covered++
		}
	}
	return covered
}

// DemoteAggregateCountFactsConflictingWithPrincipalMemberSets keeps stale
// count/scalar handoffs from competing with the authoritative principal
// member_set slate on exhaustive category enumerations. Parallel explorers can
// legitimately finish with different intermediate counts; once at least one
// principal member_set is available, a count-like fact whose numeric value does
// not match any principal set cardinality is support/audit context, not a hard
// visible-answer obligation.
func DemoteAggregateCountFactsConflictingWithPrincipalMemberSets(facts []AnswerAggregateFact, rm *RequestModel) []AnswerAggregateFact {
	if len(facts) == 0 || rm == nil {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	for i := range out {
		if !AggregateFactConflictsWithPrincipalMemberSetCounts(out, rm, i) {
			continue
		}
		out[i].Role = AnswerAggregateRoleSupportingCoverage
		if strings.TrimSpace(out[i].Provenance) == "" {
			out[i].Provenance = "demoted:conflicts_with_principal_member_set_cardinality"
		}
		changed = true
	}
	if !changed {
		return cloneAnswerAggregateFacts(facts)
	}
	return out
}

// NormalizeAggregateFactRolesForRequest applies request-aware role repair to
// aggregate facts before they enter prompt/render surfaces. It only consumes
// typed RequestModel predicates plus structured aggregate kind/role fields; it
// never scans user text, model prose, labels, or answer markdown.
//
// Scalar count questions are the important edge: explorers often emit a
// member_set as the verified coverage ledger behind a scalar command result
// (for example, the files included in a line-count pipeline). If that ledger
// is explicitly tagged principal_answer, downstream finalizer gates interpret
// it as an exhaustive visible-answer enumeration and force noisy "missing
// member" repairs. The scalar value is the principal answer; the member_set is
// support.
func NormalizeAggregateFactRolesForRequest(facts []AnswerAggregateFact, rm *RequestModel) []AnswerAggregateFact {
	if len(facts) == 0 || rm == nil {
		return cloneAnswerAggregateFacts(facts)
	}
	out := cloneAnswerAggregateFacts(facts)
	changed := false
	hasRicherPrincipalMemberSet := aggregateFactsContainPrincipalMemberSetCarrier(out, rm)
	for i := range out {
		role := NormalizeAnswerAggregateRole(out[i].Role)
		if role == AnswerAggregateRoleAuditLedger {
			continue
		}
		if aggregateFactOutsideRequestedSourceInventoryScope(out[i], rm) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:outside_requested_source_inventory_scope",
				)
				changed = true
			}
			continue
		}
		if hasRicherPrincipalMemberSet && aggregateFactIsCountCompleteMemberCarrier(out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:subordinate_to_principal_member_set",
				)
				changed = true
			}
			continue
		}
		if AggregateFactIsRuntimeObservationAdvisory(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:runtime_observation_advisory_aggregate",
				)
				changed = true
			}
			continue
		}
		if AggregateMemberSetIsScalarCountSupport(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:scalar_count_support_member_set",
				)
				changed = true
			}
			continue
		}
		if AggregateMemberSetIsScalarRoleLookupSupport(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:scalar_role_lookup_support_member_set",
				)
				changed = true
			}
			continue
		}
		if AggregateMemberSetIsNoHitWindowSupport(rm, out, i) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:negative_result_window_support",
				)
				changed = true
			}
			continue
		}
		if AggregateMemberSetIsArchitectureNarrativeSupport(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:architecture_narrative_support_member_set",
				)
				changed = true
			}
			continue
		}
		if AggregateMemberSetIsMechanismNarrativeSupport(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:mechanism_narrative_support_member_set",
				)
				changed = true
			}
			continue
		}
		if AggregateFactIsNarrativeHistorySupport(rm, out[i]) {
			if role != AnswerAggregateRoleSupportingCoverage {
				out[i].Role = AnswerAggregateRoleSupportingCoverage
				out[i].Provenance = appendAggregateFactProvenance(
					out[i].Provenance,
					"demoted:history_metadata_support",
				)
				changed = true
			}
			continue
		}
		out[i].Role = role
	}
	if !changed {
		return cloneAnswerAggregateFacts(facts)
	}
	return out
}

// AggregateMemberSetIsNoHitWindowSupport reports whether a complete member set
// is the searched-window ledger behind a zero-result proof rather than the
// visible answer set itself.
//
// This keeps no-hit answers stable across VCS history, diff, log/trace,
// command-output, connector, and repo-search observations: `result_count=0`
// remains the principal fact, while searched commits/files/rows/spans stay as
// support context unless the typed request explicitly asks to enumerate that
// inventory. The predicate is intentionally structural-only. It consumes
// aggregate kind/role, numeric zero-result fields, typed dimensions, evidence
// origins, and RequestModel traits; it never inspects user prose, model prose,
// markdown table titles, or language-specific source syntax.
func AggregateMemberSetIsNoHitWindowSupport(rm *RequestModel, facts []AnswerAggregateFact, idx int) bool {
	if rm == nil || idx < 0 || idx >= len(facts) {
		return false
	}
	if aggregateRequestRequiresNoHitWindowAsPrincipal(*rm) {
		return false
	}
	fact := facts[idx]
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if !aggregateFactsContainPrincipalZeroResult(facts, rm) {
		return false
	}
	return aggregateFactLooksLikeSearchedWindowLedger(fact)
}

func aggregateRequestRequiresNoHitWindowAsPrincipal(rm RequestModel) bool {
	if aggregateRequestRequiresPathMemberSetAsPrincipal(rm) ||
		RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		RequiresRelationMemberSetHandoff(rm) {
		return true
	}
	if rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.QuestionStructure().HasAnyObligation() {
		return true
	}
	return false
}

func aggregateFactsContainPrincipalZeroResult(facts []AnswerAggregateFact, rm *RequestModel) bool {
	for _, fact := range facts {
		switch fact.Kind {
		case AnswerAggregateNegativeSearch, AnswerAggregateNegativeObservation:
		default:
			continue
		}
		if !aggregateFactHasZeroResult(fact) {
			continue
		}
		if AnswerAggregateFactRoleForRequest(fact, rm) == AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

func aggregateFactHasZeroResult(fact AnswerAggregateFact) bool {
	for _, raw := range []string{fact.Value, aggregateFactDimensionValue(fact, "result_count", "count", "matches", "match_count", "observed_count")} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		return err == nil && n == 0
	}
	return false
}

func aggregateFactLooksLikeSearchedWindowLedger(fact AnswerAggregateFact) bool {
	if aggregateFactHasSearchedWindowCoordinate(fact) {
		return true
	}
	for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
		if origin == AnswerEvidenceOriginCurrentSource || origin == AnswerEvidenceOriginSystemInference {
			continue
		}
		if AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return true
		}
	}
	return false
}

func aggregateFactHasSearchedWindowCoordinate(fact AnswerAggregateFact) bool {
	for _, dim := range fact.Dimensions {
		name := normalizeAggregateFactDimensionKey(dim.Name)
		value := strings.TrimSpace(dim.Value)
		if name == "" || value == "" {
			continue
		}
		switch name {
		case "window_count",
			"window_size",
			"unmatched",
			"unmatched_count",
			"commit_range",
			"git_range",
			"revision_range",
			"history_window",
			"tool_result",
			"tool_result_id",
			"command_id",
			"command_result",
			"payload_ref",
			"row_set_ref",
			"artifact_id",
			"trace_window",
			"span_window",
			"log_window",
			"diff_path",
			"window_path",
			"files_searched",
			"searched_files",
			"rows_searched",
			"searched_rows",
			"order":
			return true
		}
	}
	return false
}

func aggregateFactDimensionValue(fact AnswerAggregateFact, names ...string) string {
	if len(names) == 0 || len(fact.Dimensions) == 0 {
		return ""
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		if key := normalizeAggregateFactDimensionKey(name); key != "" {
			wanted[key] = true
		}
	}
	for _, dim := range fact.Dimensions {
		if !wanted[normalizeAggregateFactDimensionKey(dim.Name)] {
			continue
		}
		if value := strings.TrimSpace(dim.Value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeAggregateFactDimensionKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	return key
}

// AggregateMemberSetIsArchitectureNarrativeSupport reports whether a
// model-emitted member_set is a structured decomposition aid for an architecture
// / logical-view answer rather than a closed user-visible member slate.
//
// Architecture answers often need component, agent, stage, function, and data
// flow sets to keep evidence organized. Those sets should enrich the finalizer
// context, not force deterministic補表 or hard member-surface gates, unless the
// request carries an explicit closed-set obligation such as a declared count,
// bucketed comparison, exhaustive enumeration, or relation lookup.
//
// The predicate is intentionally typed-only: RequestModel traits plus
// aggregate kind/role. It never scans raw user text, model prose, labels, or
// repository-language syntax.
func AggregateMemberSetIsArchitectureNarrativeSupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet {
		return false
	}
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if aggregateRequestRequiresPathMemberSetAsPrincipal(*rm) ||
		RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		RequiresRelationMemberSetHandoff(*rm) {
		return false
	}
	if rm.DiagramHint == nil && len(rm.SubTopics) <= 1 && !rm.Predicates.IsCrossComponent {
		return false
	}
	return IsArchitectureNarrativeExplanation(*rm)
}

// AggregateScalarIsNonScalarHistorySupport reports the legacy scalar-only
// subset of AggregateFactIsNarrativeHistorySupport. Kept as a small exported
// helper for callers/tests that specifically care about scalar_value facts.
func AggregateScalarIsNonScalarHistorySupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	return fact.Kind == AnswerAggregateScalar && AggregateFactIsNarrativeHistorySupport(rm, fact)
}

// AggregateMemberSetIsMechanismNarrativeSupport reports whether an exact
// member_set is a structured outline for a single mechanism explanation rather
// than a principal enumeration answer. Explorers often use member_set to keep
// discovered branches (for example retry exits, guard paths, or state
// transitions) precise for downstream prose. In this request shape the user did
// not ask for a closed answer-member set, so the set must enrich the finalizer
// context without creating Principal Enumeration Rows, deterministic补表, or
// hard member-surface gates.
//
// The predicate is intentionally typed-only: it consumes RequestModel traits and
// aggregate kind/role, never raw user text, model prose, labels, or repository
// language syntax.
func AggregateMemberSetIsMechanismNarrativeSupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet {
		return false
	}
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if aggregateRequestRequiresPathMemberSetAsPrincipal(*rm) ||
		RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		RequiresRelationMemberSetHandoff(*rm) {
		return false
	}
	return IsSingleTopicMechanismExplanation(*rm)
}

// AggregateFactIsNarrativeHistorySupport reports whether a history aggregate
// fact is metadata for a narrative / diagnostic answer rather than a principal
// value/list by itself. The distinction is typed-only:
//
//   - history source
//   - not scalar/count
//   - not a user-structured enumeration/list or bucketed comparison
//
// This prevents VCS metadata such as merge commit, source branch, subject
// line, changed-file count, or one-item commit member_set from forcing generic
// scalar blocks or system-generated member tables when the user asked for a
// feature summary or a history-backed diagnostic. It deliberately preserves
// principal aggregates for history counts, one-literal commit lookups, explicit
// recent-N lists, and bucketed / cross-component comparisons.
func AggregateFactIsNarrativeHistorySupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil {
		return false
	}
	switch fact.Kind {
	case AnswerAggregateScalar,
		AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateMemberSet:
		// eligible for demotion below
	default:
		return false
	}
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	if HistoryMemberSetIsPrincipalList(rm, fact) {
		return false
	}
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsCountQuestion {
		return false
	}
	if IsHistoryBackedCurrentCodeExplanation(*rm) {
		return true
	}
	if rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCrossComponent ||
		rm.QuestionStructure().HasAnyObligation() {
		return false
	}
	return true
}

// OriginSpecificMemberSetIsPrincipalList reports whether a model-authored
// member_set is a principal visible slate backed by a non-current-source
// observation lane such as VCS metadata/diff, an attached runtime artifact,
// command output, MCP/connector/web resources, or an external document.
//
// The predicate is intentionally conservative:
//
//   - an explicit support/audit role always stays support/audit;
//   - an explicit principal role is trusted only when the aggregate carries an
//     origin-specific non-current-source evidence origin;
//   - the only implicit promotion is the historical recent-N VCS list shape
//     that predated the explicit role lane and is still emitted by models.
//
// It never scans user prose, model free-form text, markdown table content, or
// language syntax. Current-source member sets still use the normal
// file:line/support_refs contract.
func OriginSpecificMemberSetIsPrincipalList(rm *RequestModel, fact AnswerAggregateFact) bool {
	if fact.Kind != AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) || len(fact.Members) == 0 {
		return false
	}
	role := NormalizeAnswerAggregateRole(fact.Role)
	if role == AnswerAggregateRoleSupportingCoverage || role == AnswerAggregateRoleAuditLedger {
		return false
	}
	if !aggregateFactHasOriginSpecificSupport(fact, rm) {
		return false
	}
	if rm != nil && IsHistoryBackedCurrentCodeExplanation(*rm) {
		return false
	}
	if rm != nil && rm.Predicates.IsHistoryLookup &&
		(rm.Predicates.IsDiagnosticQuestion ||
			rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
			rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() ||
			(rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active()) ||
			(rm.FieldValueProfile != nil && rm.FieldValueProfile.Active())) {
		return false
	}
	if rm != nil && rm.Predicates.IsHistoryLookup && historyMemberSetHasVCSOrigin(rm, fact) &&
		len(fact.Members) < 2 &&
		rm.Intent != IntentEnumerate &&
		!rm.Predicates.IsCategoryEnumeration &&
		!rm.Predicates.IsRelationalLookup &&
		!rm.QuestionStructure().HasAnyObligation() {
		return false
	}
	if role == AnswerAggregateRolePrincipalAnswer {
		if AggregateMemberSetIsScalarCountSupport(rm, fact) {
			return false
		}
		if rm != nil && AggregateMemberSetIsNoHitWindowSupport(rm, []AnswerAggregateFact{fact}, 0) {
			return false
		}
		if AggregateMemberSetIsArchitectureNarrativeSupport(rm, fact) ||
			AggregateMemberSetIsMechanismNarrativeSupport(rm, fact) {
			return false
		}
		return true
	}
	return historyMemberSetIsImplicitPrincipalList(rm, fact)
}

// HistoryMemberSetIsPrincipalList reports whether a model-authored
// member_set is the principal visible slate for a pure repository-history
// list/rollup answer.
//
// This protects recent-N commit summaries and similar VCS-list questions from
// being collapsed into a one-paragraph overview. The rule is typed-only: it
// consumes the analyzer's history/source-shape flags plus the structured
// aggregate fact's origin and role. It does not inspect user prose, model
// thoughts, markdown table text, or commit-message wording.
func HistoryMemberSetIsPrincipalList(rm *RequestModel, fact AnswerAggregateFact) bool {
	if fact.Kind != AnswerAggregateMemberSet {
		return false
	}
	return historyMemberSetIsImplicitPrincipalList(rm, fact) ||
		(historyMemberSetHasVCSOrigin(rm, fact) && OriginSpecificMemberSetIsPrincipalList(rm, fact))
}

func historyMemberSetIsImplicitPrincipalList(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if len(fact.Members) < 2 {
		return false
	}
	if !rm.Predicates.IsHistoryLookup || rm.Predicates.IsScalarAnswer || rm.Predicates.IsCountQuestion {
		return false
	}
	if IsHistoryBackedCurrentCodeExplanation(*rm) ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() ||
		(rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active()) ||
		(rm.FieldValueProfile != nil && rm.FieldValueProfile.Active()) {
		return false
	}
	role := NormalizeAnswerAggregateRole(fact.Role)
	if role == AnswerAggregateRoleSupportingCoverage || role == AnswerAggregateRoleAuditLedger {
		return false
	}
	return historyMemberSetHasVCSOrigin(rm, fact)
}

func historyMemberSetHasVCSOrigin(rm *RequestModel, fact AnswerAggregateFact) bool {
	for _, origin := range AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if origin == AnswerEvidenceOriginVCSMetadata || origin == AnswerEvidenceOriginVCSDiff {
			return true
		}
	}
	return false
}

// HasPrincipalHistoryMemberSetForRequest reports whether accepted aggregate
// facts contain a principal VCS history list that the final answer must not
// omit. It is used by prompt and pre-emit layers so both read the same typed
// contract.
func HasPrincipalHistoryMemberSetForRequest(rm *RequestModel, facts []AnswerAggregateFact) bool {
	if rm == nil || len(facts) == 0 {
		return false
	}
	for _, fact := range facts {
		if HistoryMemberSetIsPrincipalList(rm, fact) &&
			AnswerAggregateFactRoleForRequest(fact, rm) == AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

// HasPrincipalOriginSpecificMemberSetForRequest reports whether accepted
// aggregate facts contain a principal list backed by an origin-specific
// non-current-source observation. Finalization must preserve these rows, but
// they do not create current-source citation pressure.
func HasPrincipalOriginSpecificMemberSetForRequest(rm *RequestModel, facts []AnswerAggregateFact) bool {
	if len(facts) == 0 {
		return false
	}
	for _, fact := range facts {
		if OriginSpecificMemberSetIsPrincipalList(rm, fact) &&
			AnswerAggregateFactRoleForRequest(fact, rm) == AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

func aggregateFactHasOriginSpecificSupport(fact AnswerAggregateFact, rm *RequestModel) bool {
	for _, origin := range AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return true
		}
	}
	return false
}

func AggregateFactConflictsWithPrincipalMemberSetCounts(facts []AnswerAggregateFact, rm *RequestModel, idx int) bool {
	if rm == nil || idx < 0 || idx >= len(facts) {
		return false
	}
	if !rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
		return false
	}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(*rm) && !rm.QuestionStructure().HasAnyObligation() {
		return false
	}
	fact := facts[idx]
	if !aggregateFactKindCanRepresentVisibleCount(fact.Kind) {
		return false
	}
	value, ok := aggregateFactNumericValue(fact)
	if !ok || value <= 1 {
		return false
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(facts, rm)
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		if len(ref.Fact.Members) == value {
			return false
		}
	}
	return true
}

func aggregateFactKindCanRepresentVisibleCount(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateScalar:
		return true
	default:
		return false
	}
}

func aggregateFactNumericValue(fact AnswerAggregateFact) (int, bool) {
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func principalAggregateFactsHaveMemberLevelAnswerSymbolOverlap(facts []AnswerAggregateFact, rm *RequestModel, symbolKeys map[string]bool) bool {
	if len(facts) == 0 || len(symbolKeys) == 0 {
		return false
	}
	matchedMembers := map[string]bool{}
	for _, fact := range facts {
		if !answerAggregateFactCarriesCompleteMemberSet(fact) ||
			AnswerAggregateFactRoleForRequest(fact, rm) != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		for _, member := range fact.Members {
			if aggregateMemberCoveredByAnswerSymbolSet(member, symbolKeys) {
				if key := aggregateMemberSetProjectionMemberKey(member); key != "" {
					matchedMembers[key] = true
				}
				if len(matchedMembers) >= 2 {
					return true
				}
			}
		}
	}
	return false
}

func answerSymbolSurfaceKeySet(symbols []AnswerSymbol) map[string]bool {
	out := map[string]bool{}
	for _, sym := range symbols {
		addAggregateSymbolProjectionKey(out, sym.Name)
	}
	return out
}

func aggregateMemberCoveredByAnswerSymbolSet(member string, symbolKeys map[string]bool) bool {
	if len(symbolKeys) == 0 {
		return false
	}
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(member) {
		if aggregateSymbolProjectionKeyExists(candidate, symbolKeys) {
			return true
		}
		if base, _, ok := AnswerAggregateDecoratedLabelParts(candidate); ok &&
			aggregateSymbolProjectionKeyExists(base, symbolKeys) {
			return true
		}
		if label, _, ok := ParseAnswerSupportRefMemberLocation(candidate); ok &&
			aggregateSymbolProjectionKeyExists(label, symbolKeys) {
			return true
		}
	}
	if base, _, ok := AnswerAggregateDecoratedLabelParts(member); ok {
		return aggregateSymbolProjectionKeyExists(base, symbolKeys)
	}
	return aggregateSymbolProjectionKeyExists(member, symbolKeys)
}

func aggregateSymbolProjectionKeyExists(raw string, keys map[string]bool) bool {
	for _, candidate := range []string{raw, aggregateMemberKey(raw), NormalizedSurfaceSymbolTail(raw)} {
		if key := strings.ToLower(strings.TrimSpace(candidate)); key != "" && keys[key] {
			return true
		}
	}
	return false
}

func addAggregateSymbolProjectionKey(keys map[string]bool, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	for _, candidate := range []string{raw, aggregateMemberKey(raw), NormalizedSurfaceSymbolTail(raw)} {
		if key := strings.ToLower(strings.TrimSpace(candidate)); key != "" {
			keys[key] = true
		}
	}
}

func aggregateMemberSetSymbolProjectionKey(members []string) string {
	keys := make([]string, 0, len(members))
	seen := map[string]bool{}
	for _, member := range members {
		key := aggregateMemberSetProjectionMemberKey(member)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f")
}

func aggregateMemberSetProjectionMemberKey(member string) string {
	key := ""
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(member) {
		if candidateKey := AnswerAggregateMemberSurfaceKey(candidate); candidateKey != "" {
			key = strings.ToLower(candidateKey)
			break
		}
	}
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(member))
	}
	return strings.TrimSpace(key)
}

func answerAggregateFactRef(index int, fact AnswerAggregateFact, role AnswerAggregateRole) AnswerAggregateFactRef {
	provenance := strings.TrimSpace(fact.Provenance)
	if provenance == "" {
		provenance = "model_emitted"
	}
	return AnswerAggregateFactRef{
		Index:      index,
		Fact:       fact,
		Role:       role,
		Provenance: provenance,
	}
}

func AnswerAggregateFactRoleForRequest(fact AnswerAggregateFact, rm *RequestModel) AnswerAggregateRole {
	role := NormalizeAnswerAggregateRole(fact.Role)
	if role == AnswerAggregateRoleAuditLedger {
		return role
	}
	if AggregateFactIsRuntimeObservationAdvisory(rm, fact) {
		return AnswerAggregateRoleSupportingCoverage
	}
	if role != AnswerAggregateRoleUnknown {
		if role == AnswerAggregateRolePrincipalAnswer && AggregateCountFactMembersAreSupportOnlyForRequest(rm, fact) {
			return AnswerAggregateRoleSupportingCoverage
		}
		if role == AnswerAggregateRolePrincipalAnswer && AggregateMemberSetIsScalarRoleLookupSupport(rm, fact) {
			return AnswerAggregateRoleSupportingCoverage
		}
		return role
	}
	if fact.Kind == AnswerAggregateMemberSet {
		if AggregateMemberSetIsScalarCountSupport(rm, fact) {
			return AnswerAggregateRoleSupportingCoverage
		}
		if AggregateMemberSetIsScalarRoleLookupSupport(rm, fact) {
			return AnswerAggregateRoleSupportingCoverage
		}
		if rm != nil && !aggregateRequestRequiresPathMemberSetAsPrincipal(*rm) &&
			aggregateMemberSetIsSourcePathCoverageOnly(fact, *rm) {
			return AnswerAggregateRoleSupportingCoverage
		}
		return AnswerAggregateRolePrincipalAnswer
	}
	if answerAggregateFactCarriesCompleteMemberSet(fact) {
		if AggregateCountFactMembersAreSupportOnlyForRequest(rm, fact) {
			return AnswerAggregateRoleSupportingCoverage
		}
		if rm != nil && aggregateCountMemberSetIsNarrativeCoverageOnly(fact, *rm) {
			return AnswerAggregateRoleSupportingCoverage
		}
		return AnswerAggregateRolePrincipalAnswer
	}
	return AnswerAggregateRolePrincipalAnswer
}

// AggregateFactIsRuntimeObservationAdvisory reports whether a model-authored
// runtime/log/trace aggregate is useful context but not direct proof of caller
// ownership, upstream data construction, or root-cause provenance. External
// artifacts can directly prove observed error classes, frames, spans, timing,
// and literal log content; broader "behavior outcome" conclusions need either
// explicit support refs or current-source evidence before they can become the
// principal answer lane.
func AggregateFactIsRuntimeObservationAdvisory(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || !rm.HasExternalOnlyRuntimeArtifact() {
		return false
	}
	if len(fact.SupportRefs) > 0 {
		return false
	}
	switch fact.Kind {
	case AnswerAggregateBehaviorOutcome, AnswerAggregateErrorGranularity:
		return true
	case AnswerAggregateScalar:
		if aggregateScalarRestatesDirectRuntimeObservation(rm, fact) {
			return false
		}
		return !rm.Predicates.IsScalarAnswer && !rm.Predicates.IsCountQuestion
	case AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount:
		return !rm.Predicates.IsScalarAnswer && !rm.Predicates.IsCountQuestion
	case AnswerAggregateMemberSet:
		return runtimeObservationMemberSetIsAdvisory(rm, fact)
	default:
		return false
	}
}

func runtimeObservationMemberSetIsAdvisory(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet {
		return false
	}
	if runtimeRequestRequiresPrincipalMemberSet(rm) {
		return false
	}
	return rm.Intent == IntentRootCause ||
		rm.Intent == IntentTrace ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() ||
		rm.Scenario == ScenarioPerformanceBottleneck ||
		rm.Scenario == ScenarioRootCause
}

func runtimeRequestRequiresPrincipalMemberSet(rm *RequestModel) bool {
	if rm == nil {
		return false
	}
	if rm.Intent == IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.QuestionStructure().HasAnyObligation() {
		return true
	}
	return rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active()
}

func aggregateScalarRestatesDirectRuntimeObservation(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateScalar {
		return false
	}
	factText := normalizeRuntimeObservationFactText(firstNonEmptyString(fact.Value, fact.Label))
	if factText == "" {
		return false
	}
	if rm.LogTriage != nil && logBundleDirectObservationContains(rm.LogTriage, factText) {
		return true
	}
	if rm.PerfTrace != nil && perfBundleDirectObservationContains(rm.PerfTrace, factText) {
		return true
	}
	return false
}

func logBundleDirectObservationContains(bundle *LogBundle, factText string) bool {
	if bundle == nil || factText == "" {
		return false
	}
	var walk func(LogError) bool
	walk = func(err LogError) bool {
		if runtimeObservationTextMatches(factText, err.Type) ||
			runtimeObservationTextMatches(factText, err.Message) {
			return true
		}
		for _, frame := range err.Frames {
			if runtimeObservationTextMatches(factText, frame.Func) ||
				runtimeObservationTextMatches(factText, frame.File) {
				return true
			}
		}
		return err.Cause != nil && walk(*err.Cause)
	}
	for _, err := range bundle.Errors {
		if walk(err) {
			return true
		}
	}
	for _, obs := range bundle.Observations {
		if !obs.Diagnostic {
			continue
		}
		if runtimeObservationTextMatches(factText, obs.Subject) ||
			runtimeObservationTextMatches(factText, obs.Summary) ||
			runtimeObservationTextMatches(factText, obs.Evidence) {
			return true
		}
	}
	return false
}

func perfBundleDirectObservationContains(bundle *PerfBundle, factText string) bool {
	if bundle == nil || factText == "" {
		return false
	}
	for _, obs := range bundle.Observations {
		if runtimeObservationTextMatches(factText, obs.Subject) ||
			runtimeObservationTextMatches(factText, obs.Summary) ||
			runtimeObservationTextMatches(factText, obs.Evidence) {
			return true
		}
	}
	return false
}

func runtimeObservationTextMatches(factText, direct string) bool {
	direct = normalizeRuntimeObservationFactText(direct)
	if direct == "" {
		return false
	}
	return strings.Contains(factText, direct) || strings.Contains(direct, factText)
}

func normalizeRuntimeObservationFactText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

// AggregateMemberSetIsScalarCountSupport reports whether a model-emitted
// member_set fact is acting as support evidence for a scalar count answer
// (e.g. "how many X are there?") rather than as the principal member slate
// itself. When this returns true, downstream consumers (pre-emit oracle,
// prompt renderers) MUST NOT demand that every member of the fact appear
// verbatim in the visible answer: the scalar count IS the answer, and the
// member list is corroborating evidence.
//
// Single source of truth for both
//   - internal/tool/answer_document_pre_emit_check.go
//     (preCheckAggregateMemberSetCoverage skips these facts when filtering
//     missing members)
//   - internal/agent/answer_document_evaluator.go
//     (renderAnswerDocPrincipalMemberSetContract skips these facts when
//     surfacing the must-verbatim prompt section)
//
// Keeping the predicate here ensures the two surfaces — emit-time reject
// message and pre-emit prompt hint — read the same shape.
func AggregateMemberSetIsScalarCountSupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet || len(fact.Members) == 0 {
		return false
	}
	if !rm.Predicates.IsCountQuestion || !rm.Predicates.IsScalarAnswer {
		return false
	}
	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	return rm.Predicates.IsHistoryLookup ||
		rm.Intent == IntentReturnValue ||
		rm.AnswerSubject.Kind == SubjectNumeric
}

// AggregateMemberSetIsScalarRoleLookupSupport reports whether a model-emitted
// member_set is a context ledger behind a scalar role-location answer rather
// than the visible principal answer itself.
//
// Role lookup questions ask for the component that satisfies a role, not a
// bag of every helper, tool, dispatcher, or runtime touched while explaining
// the route to that component. If the analyzer classified the answer as
// scalar/role-location and a model emits multiple members anyway, keep that
// set as supporting coverage so downstream stages must still materialize the
// scalar answer from typed answer symbols, evidence, or a single-member
// aggregate. The predicate is schema-only: it consumes RequestModel booleans,
// aggregate kind, and member count; it never reads user prose, aggregate
// labels, closure rationale, or model thinking.
func AggregateMemberSetIsScalarRoleLookupSupport(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind != AnswerAggregateMemberSet || len(fact.Members) <= 1 {
		return false
	}
	if rm.Intent != IntentExplain {
		return false
	}
	if !rm.Predicates.IsScalarAnswer && !rm.Predicates.IsRoleLocateLookup {
		return false
	}
	if rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.QuestionStructure().HasAnyObligation() ||
		RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		RequiresRelationMemberSetHandoff(*rm) {
		return false
	}
	return true
}

// AggregateCountFactMembersAreSupportOnlyForRequest reports whether a count
// aggregate's members are proof/support rows rather than the user-visible
// member set. Count-shaped facts (`total_count`, `unique_count`,
// `grouped_count`, `bucket_count`) often carry members as the measured
// partitions or proof samples; treating `value == len(members)` as a visible
// list creates bogus system补表 for count, comparison, mechanism, runtime, and
// VCS answers. Only typed complete-member request shapes may promote these
// members to a principal enumeration surface.
func AggregateCountFactMembersAreSupportOnlyForRequest(rm *RequestModel, fact AnswerAggregateFact) bool {
	if rm == nil || fact.Kind == AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if aggregateRequestRequiresPathMemberSetAsPrincipal(*rm) ||
		RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		RequiresRelationMemberSetHandoff(*rm) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() &&
		(rm.Intent == IntentEnumerate || rm.Predicates.IsCategoryEnumeration || rm.QuestionStructure().HasAnyObligation()) {
		return false
	}
	if rm.Intent == IntentEnumerate || rm.Predicates.IsCategoryEnumeration {
		if rm.Predicates.IsCountQuestion ||
			rm.Predicates.IsScalarAnswer ||
			rm.Predicates.IsCrossComponent ||
			len(rm.QuestionStructure().Buckets) >= 2 {
			return true
		}
		return false
	}
	if rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Predicates.IsCrossComponent ||
		len(rm.QuestionStructure().Buckets) >= 2 ||
		IsArchitectureNarrativeExplanation(*rm) ||
		IsSingleTopicMechanismExplanation(*rm) {
		return true
	}
	contract := CompileAnswerIntentContract(*rm, nil)
	if contract.HasOutput(AnswerRequestedOutputComparison) ||
		contract.HasOutput(AnswerRequestedOutputCount) ||
		contract.HasOutput(AnswerRequestedOutputScalar) ||
		contract.HasOutput(AnswerRequestedOutputMechanism) ||
		contract.HasOutput(AnswerRequestedOutputDiagnostic) ||
		contract.HasOutput(AnswerRequestedOutputTrace) ||
		contract.HasOutput(AnswerRequestedOutputChangeImpact) {
		return true
	}
	return false
}

func aggregateFactIsCountCompleteMemberCarrier(fact AnswerAggregateFact) bool {
	switch fact.Kind {
	case AnswerAggregateTotalCount, AnswerAggregateUniqueCount, AnswerAggregateGroupedCount, AnswerAggregateBucketCount:
		return answerAggregateFactCarriesCompleteMemberSet(fact)
	default:
		return false
	}
}

func aggregateFactRefsContainPrincipalMemberSet(refs []AnswerAggregateFactRef) bool {
	for _, ref := range refs {
		if ref.Fact.Kind == AnswerAggregateMemberSet && answerAggregateFactCarriesCompleteMemberSet(ref.Fact) &&
			NormalizeAnswerAggregateRole(ref.Role) == AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

func aggregateFactsContainPrincipalMemberSetCarrier(facts []AnswerAggregateFact, rm *RequestModel) bool {
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if aggregateFactOutsideRequestedSourceInventoryScope(fact, rm) {
			continue
		}
		if AnswerAggregateFactRoleForRequest(fact, rm) == AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

func aggregateFactOutsideRequestedSourceInventoryScope(fact AnswerAggregateFact, rm *RequestModel) bool {
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if HasTypedRelationMemberSetShape(*rm) {
		return false
	}
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if (RequiresExhaustiveEnumerationMemberSetHandoff(*rm) || RequiresRelationMemberSetHandoff(*rm)) &&
		AnswerAggregateFactRoleForRequest(fact, rm).IsPrincipal() {
		return false
	}
	scopes := aggregateRequestedSourceInventoryScopes(rm)
	if len(scopes) == 0 {
		return false
	}
	files := aggregateFactSourceFiles(fact)
	if len(files) == 0 {
		return false
	}
	for _, file := range files {
		if !aggregateSourceFileInAnyScope(file, scopes) {
			return true
		}
	}
	return false
}

// PrincipalSourceInventoryMemberSetScopeFiles returns the source files carried
// by accepted principal source-inventory member_set facts. It is a scheduling
// helper for read-loop scope pruning: the files come from typed member/support
// carriers, not from final-answer prose, model rationale, or request-keyword
// matching.
func PrincipalSourceInventoryMemberSetScopeFiles(facts []AnswerAggregateFact, rm *RequestModel) []string {
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return nil
	}
	if HasTypedRelationMemberSetShape(*rm) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		for _, file := range answerAggregateFactSourceFilesPreserveCase(raw) {
			if file == "" || seen[strings.ToLower(file)] || !boundedSourceEnumerationPathAllowed(*rm, file) {
				continue
			}
			seen[strings.ToLower(file)] = true
			out = append(out, file)
		}
	}
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateMemberSet || !answerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if aggregateFactOutsideRequestedSourceInventoryScope(fact, rm) {
			continue
		}
		if AnswerAggregateFactRoleForRequest(fact, rm) != AnswerAggregateRolePrincipalAnswer {
			continue
		}
		for _, ref := range fact.SupportRefs {
			add(ref)
		}
		for _, member := range fact.Members {
			add(member)
		}
	}
	sort.Strings(out)
	return out
}

func answerAggregateFactSourceFilesPreserveCase(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(file string) {
		file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
		if file == "" || seen[strings.ToLower(file)] {
			return
		}
		seen[strings.ToLower(file)] = true
		out = append(out, file)
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
		add(loc.File)
	}
	if loc, ok := ParseAnswerSourceLocationSurface(raw); ok {
		add(loc.File)
	}
	if file, ok := ParseAnswerFilePathSurface(raw); ok {
		add(file)
	}
	if _, qualifier, ok := AnswerAggregateDecoratedLabelParts(raw); ok {
		if loc, parsed := ParseAnswerSourceLocationSurface(qualifier); parsed {
			add(loc.File)
		} else if file, parsed := ParseAnswerFilePathSurface(qualifier); parsed {
			add(file)
		}
	}
	return out
}

func aggregateRequestedSourceInventoryScopes(rm *RequestModel) []string {
	if rm == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		scope := aggregateRequestedSourceInventoryScope(raw)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	for _, group := range [][]string{
		rm.AnalyzerHints.ExactTargets,
		rm.AnalyzerHints.MentionedEntities,
		rm.AnalyzerHints.PrimaryEntities,
		rm.AnalyzerHints.Entities,
	} {
		for _, raw := range group {
			add(raw)
		}
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	if SourceInventoryRequiresRepoWideLens(*rm) {
		return nil
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		add(hint.Path)
	}
	sort.Strings(out)
	return out
}

func aggregateRequestedSourceInventoryScope(raw string) string {
	raw = strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "`'\" ")
	if raw == "" || strings.ContainsAny(raw, "\n\r") {
		return ""
	}
	if label, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
		_ = label
		raw = loc.File
	} else if loc, ok := ParseAnswerSourceLocationSurface(raw); ok {
		raw = loc.File
	} else if file, ok := ParseAnswerFilePathSurface(raw); ok {
		raw = file
	}
	raw = strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "/")
	if raw == "" || strings.Contains(raw, " ") {
		return ""
	}
	if !strings.Contains(raw, "/") {
		return ""
	}
	return strings.ToLower(raw)
}

func aggregateFactSourceFiles(fact AnswerAggregateFact) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		for _, file := range aggregateSourceFilesFromSurface(raw) {
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, file)
		}
	}
	for _, ref := range fact.SupportRefs {
		add(ref)
	}
	for _, member := range fact.Members {
		add(member)
	}
	sort.Strings(out)
	return out
}

func aggregateSourceFilesFromSurface(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	add := func(file string) {
		file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
		if file == "" {
			return
		}
		out = append(out, strings.ToLower(file))
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
		add(loc.File)
	}
	if loc, ok := ParseAnswerSourceLocationSurface(raw); ok {
		add(loc.File)
	}
	if file, ok := ParseAnswerFilePathSurface(raw); ok {
		add(file)
	}
	if _, qualifier, ok := AnswerAggregateDecoratedLabelParts(raw); ok {
		if loc, parsed := ParseAnswerSourceLocationSurface(qualifier); parsed {
			add(loc.File)
		} else if file, parsed := ParseAnswerFilePathSurface(qualifier); parsed {
			add(file)
		}
	}
	return aggregateDedupSourceFiles(out)
}

func aggregateDedupSourceFiles(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func aggregateSourceFileInAnyScope(file string, scopes []string) bool {
	file = strings.ToLower(strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/"))
	if file == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.ToLower(strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/"))
		if scope == "" {
			continue
		}
		if file == scope || strings.HasPrefix(file, scope+"/") || strings.HasSuffix(file, "/"+scope) {
			return true
		}
	}
	return false
}

func PrincipalRelationMemberSetFactRefsForRequest(facts []AnswerAggregateFact, rm *RequestModel) []AnswerAggregateFactRef {
	refs := PrincipalAggregateMemberSetFactRefsForRequest(facts, rm)
	if len(refs) == 0 {
		return nil
	}
	out := make([]AnswerAggregateFactRef, 0, len(refs))
	for _, ref := range refs {
		if AnswerAggregateFactHasRelationMembers(ref.Fact) {
			out = append(out, ref)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PrincipalMemberSetHasRelationEvidence reports whether a principal
// aggregate member_set is already backed by model-emitted relation evidence
// such as calls, assignments, imports, guards, returns, or other non-definition
// line-shaped anchors. This is the shared "do not synthesize a second
// answer-symbol slate" predicate for extractor and emit_answer_symbol.
//
// The check is deliberately structural: it reads aggregate_facts members and
// EvidenceItem fields only. It does not inspect user text, labels, closure
// prose, localized words, or final-answer prose.
func PrincipalMemberSetHasRelationEvidence(facts []AnswerAggregateFact, evidence []EvidenceItem, rm *RequestModel) bool {
	if len(facts) == 0 || len(evidence) == 0 {
		return false
	}
	if rm != nil {
		facts = NormalizeAggregateFactRolesForRequest(facts, rm)
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(facts, rm)
	if len(refs) == 0 {
		return false
	}
	members := map[string]bool{}
	for _, ref := range refs {
		for _, member := range ref.Fact.Members {
			if key := aggregateAxisMemberKey(member); key != "" {
				members[key] = true
			}
		}
	}
	if len(members) == 0 {
		return false
	}
	for _, item := range evidence {
		if !item.IsCitable() || !item.Scope.IsLineShaped() || item.LineStart <= 0 {
			continue
		}
		if item.AnchorKind == "" || item.AnchorKind == AnchorDefinition {
			continue
		}
		if evidenceItemMatchesAggregateMember(item, members) {
			return true
		}
	}
	return false
}

func evidenceItemMatchesAggregateMember(item EvidenceItem, members map[string]bool) bool {
	for _, surface := range []string{item.Subject, item.Object, item.AnchorSymbol} {
		if aggregateSurfaceMatchesMember(surface, members) {
			return true
		}
	}
	return false
}

func aggregateSurfaceMatchesMember(surface string, members map[string]bool) bool {
	key := aggregateAxisMemberKey(surface)
	if key == "" {
		return false
	}
	if members[key] {
		return true
	}
	for member := range members {
		if strings.HasPrefix(member, key+" ") ||
			strings.HasPrefix(member, key+"(") ||
			strings.Contains(member, " "+key+" ") {
			return true
		}
	}
	return false
}

func aggregateRequestRequiresPathMemberSetAsPrincipal(rm RequestModel) bool {
	if RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		RequiresRelationMemberSetHandoff(rm) ||
		RequiresSourceOperationSiteMemberSetHandoff(rm) {
		return true
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	return false
}

func aggregateCountMemberSetIsNarrativeCoverageOnly(fact AnswerAggregateFact, rm RequestModel) bool {
	if fact.Kind == AnswerAggregateMemberSet {
		return false
	}
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if aggregateRequestRequiresPathMemberSetAsPrincipal(rm) {
		return false
	}
	if IsArchitectureNarrativeExplanation(rm) || IsSingleTopicMechanismExplanation(rm) {
		return true
	}
	if rm.Intent != IntentExplain {
		return false
	}
	if IsCategoryEnumerationAnswerShape(rm) ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup {
		return false
	}
	return true
}

func aggregateMemberSetIsSourcePathCoverageOnly(fact AnswerAggregateFact, rm RequestModel) bool {
	if !answerAggregateMemberSetAllSourcePathSurfaces(fact) {
		return false
	}
	if IsArchitectureNarrativeExplanation(rm) || IsSingleTopicMechanismExplanation(rm) {
		return true
	}
	if rm.Intent != IntentExplain {
		return false
	}
	if IsCategoryEnumerationAnswerShape(rm) ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup {
		return false
	}
	return true
}

func answerAggregateMemberSetAllSourcePathSurfaces(fact AnswerAggregateFact) bool {
	if !answerAggregateFactCarriesCompleteMemberSet(fact) || len(fact.Members) == 0 {
		return false
	}
	for _, member := range fact.Members {
		member = strings.TrimSpace(member)
		if member == "" {
			return false
		}
		if _, ok := ParseAnswerFilePathSurface(member); ok {
			continue
		}
		if _, ok := ParseAnswerSourceLocationSurface(member); ok {
			continue
		}
		return false
	}
	return true
}

func aggregateRelationAlternateMemberSetSkips(relationFacts []aggregateRelationMemberSetFact) map[int]bool {
	if len(relationFacts) < 2 {
		return nil
	}
	groups := make(map[string][]aggregateRelationMemberSetFact)
	for _, fact := range relationFacts {
		if fact.leftKey == "" {
			continue
		}
		groups[fact.leftKey] = append(groups[fact.leftKey], fact)
	}
	skip := make(map[int]bool)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		best := group[0]
		for _, candidate := range group[1:] {
			if aggregateRelationMemberSetPreferred(candidate, best) {
				best = candidate
			}
		}
		if best.supportCoverage < len(best.fact.Members) {
			continue
		}
		for _, candidate := range group {
			if candidate.index == best.index {
				continue
			}
			if candidate.supportCoverage >= len(candidate.fact.Members) {
				continue
			}
			skip[candidate.index] = true
		}
	}
	if len(skip) == 0 {
		return nil
	}
	return skip
}

func aggregateRelationMemberSetPreferred(a, b aggregateRelationMemberSetFact) bool {
	if a.supportCoverage != b.supportCoverage {
		return a.supportCoverage > b.supportCoverage
	}
	if len(a.fact.SupportRefs) != len(b.fact.SupportRefs) {
		return len(a.fact.SupportRefs) > len(b.fact.SupportRefs)
	}
	return a.index < b.index
}

// PrincipalRelationMemberSetFactRefs returns principal member_set facts whose
// members carry an explicit compact relation surface such as `caller → callee`,
// `pkg: Entry`, `pkg/Entry`, or `Type::Member`.
//
// This is a typed aggregate helper, not a natural-language classifier. It only
// consumes model-authored member_set rows plus the same language-neutral
// relation parser used by member-set canonicalization. Callers use it to apply
// relation-answer shape rules without coupling those rules to Go symbols,
// English/Chinese keywords, or a specific question family.
func PrincipalRelationMemberSetFactRefs(facts []AnswerAggregateFact) []AnswerAggregateFactRef {
	return PrincipalRelationMemberSetFactRefsForRequest(facts, nil)
}

// AnswerAggregateFactHasRelationMembers reports whether at least one principal
// member is an explicit two-part relation display surface. It deliberately does
// not inspect labels, dimensions, raw prompts, or closure prose.
func AnswerAggregateFactHasRelationMembers(fact AnswerAggregateFact) bool {
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	for _, member := range fact.Members {
		if _, _, ok := AnswerAggregateMemberRelationParts(member); ok {
			return true
		}
	}
	return false
}

func aggregateRelationLeftAxisSet(fact AnswerAggregateFact) (map[string]bool, bool) {
	if !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return nil, false
	}
	left := make(map[string]bool, len(fact.Members))
	parsed := 0
	for _, member := range fact.Members {
		l, _, ok := AnswerAggregateMemberRelationParts(member)
		if !ok {
			continue
		}
		key := aggregateAxisMemberKey(l)
		if key == "" {
			continue
		}
		left[key] = true
		parsed++
	}
	return left, parsed > 0 && len(left) > 0
}

func answerAggregateFactCarriesCompleteMemberSet(fact AnswerAggregateFact) bool {
	if fact.Kind == AnswerAggregateMemberSet {
		if len(fact.Members) == 0 {
			return AnswerAggregateFactIsExactEmptyMemberSet(fact)
		}
		return true
	}
	if len(fact.Members) == 0 {
		return false
	}
	switch fact.Kind {
	case AnswerAggregateTotalCount, AnswerAggregateUniqueCount, AnswerAggregateGroupedCount, AnswerAggregateBucketCount:
	default:
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(fact.Value))
	return err == nil && n == len(fact.Members)
}

// AnswerAggregateFactIsExactEmptyMemberSet reports whether a model emitted a
// first-class exact empty set. It is intentionally limited to the dedicated
// member_set kind so zero counts and prose scalars cannot impersonate an
// exhaustive empty answer slate.
func AnswerAggregateFactIsExactEmptyMemberSet(fact AnswerAggregateFact) bool {
	if fact.Kind != AnswerAggregateMemberSet || len(fact.Members) != 0 {
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(fact.Value))
	return err == nil && n == 0
}

// AnswerAggregateFactCarriesCompleteMemberSet reports whether an aggregate fact
// carries an exact principal member list, regardless of whether the model used
// the dedicated member_set kind or a count kind with members whose cardinality
// equals value. Callers use this to consume the schema's typed member carrier
// uniformly instead of hard-coding only one aggregate kind.
func AnswerAggregateFactCarriesCompleteMemberSet(fact AnswerAggregateFact) bool {
	return answerAggregateFactCarriesCompleteMemberSet(fact)
}

func aggregateMemberSetIsSubsumedLeftAxis(fact AnswerAggregateFact, relationFacts []aggregateRelationMemberSetFact) bool {
	if len(relationFacts) == 0 || !answerAggregateFactCarriesCompleteMemberSet(fact) {
		return false
	}
	if _, isRelation := aggregateRelationLeftAxisSet(fact); isRelation {
		return false
	}
	axis := make(map[string]bool, len(fact.Members))
	for _, member := range fact.Members {
		key := aggregateAxisMemberKey(member)
		if key == "" {
			return false
		}
		axis[key] = true
	}
	if len(axis) == 0 {
		return false
	}
	for _, relation := range relationFacts {
		if !aggregateFactDimensionsCompatible(fact.Dimensions, relation.fact.Dimensions) {
			continue
		}
		if aggregateAxisSetSubsetOf(axis, relation.left) {
			return true
		}
	}
	return false
}

func aggregateFactDimensionsCompatible(a, b []AnswerAggregateDimension) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	ak := make([]string, 0, len(a))
	bk := make([]string, 0, len(b))
	for _, dim := range a {
		ak = append(ak, strings.ToLower(strings.TrimSpace(dim.Name))+"\x00"+strings.ToLower(strings.TrimSpace(dim.Value)))
	}
	for _, dim := range b {
		bk = append(bk, strings.ToLower(strings.TrimSpace(dim.Name))+"\x00"+strings.ToLower(strings.TrimSpace(dim.Value)))
	}
	sort.Strings(ak)
	sort.Strings(bk)
	for i := range ak {
		if ak[i] != bk[i] {
			return false
		}
	}
	return true
}

func aggregateAxisSetSubsetOf(axis, relationLeft map[string]bool) bool {
	if len(axis) == 0 || len(relationLeft) == 0 {
		return false
	}
	for key := range axis {
		if !relationLeft[key] {
			return false
		}
	}
	return true
}

func aggregateAxisSetKey(axis map[string]bool) string {
	if len(axis) == 0 {
		return ""
	}
	keys := make([]string, 0, len(axis))
	for key := range axis {
		key = strings.TrimSpace(strings.ToLower(key))
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f")
}

func aggregateMemberSetSupportCoverage(fact AnswerAggregateFact) int {
	if len(fact.Members) == 0 {
		return 0
	}
	selfLocated := 0
	for _, member := range fact.Members {
		if _, _, ok := ParseAnswerSupportRefMemberLocation(member); ok {
			selfLocated++
		}
	}
	parsedRefs := 0
	for _, ref := range fact.SupportRefs {
		if _, _, ok := ParseAnswerSupportRefMemberLocation(ref); ok {
			parsedRefs++
		}
	}
	coverage := selfLocated
	if parsedRefs == len(fact.Members) {
		coverage = len(fact.Members)
	} else if parsedRefs > coverage {
		coverage = parsedRefs
	}
	if coverage > len(fact.Members) {
		return len(fact.Members)
	}
	return coverage
}

func aggregateAxisMemberKey(member string) string {
	member = trimAggregateMemberSurface(member)
	if member == "" {
		return ""
	}
	return strings.ToLower(member)
}

func normalizeAnswerAggregateFact(raw AnswerAggregateFact) (AnswerAggregateFact, error) {
	fact := AnswerAggregateFact{
		Kind:       raw.Kind,
		Label:      trimAggregateText(raw.Label),
		Value:      trimAggregateText(raw.Value),
		Role:       NormalizeAnswerAggregateRole(raw.Role),
		Provenance: trimAggregateText(raw.Provenance),
		Unit:       trimAggregateText(raw.Unit),
	}
	if !fact.Kind.IsValid() {
		return AnswerAggregateFact{}, fmt.Errorf("kind %q is not accepted", raw.Kind)
	}
	if fact.Label == "" {
		return AnswerAggregateFact{}, fmt.Errorf("label is required")
	}
	dims, err := normalizeAnswerAggregateDimensions(raw.Dimensions)
	if err != nil {
		return AnswerAggregateFact{}, err
	}
	fact.Dimensions = dims
	fact.Dimensions = normalizeAggregateFactOriginDimensions(fact)
	fact.Members = normalizeAggregateStrings(raw.Members, maxAnswerAggregateMembers)
	fact.Excluded = normalizeAggregateStrings(raw.Excluded, maxAnswerAggregateMembers)
	fact.SupportRefs = normalizeAggregateStrings(raw.SupportRefs, maxAnswerAggregateMembers)
	fact.MemberNotes = normalizeAggregateStrings(raw.MemberNotes, maxAnswerAggregateMembers)
	if fact.Kind == AnswerAggregateMemberSet && len(fact.Members) > 0 {
		fact.Members, fact.SupportRefs, fact.MemberNotes = normalizeAggregateMemberSetMemberSupportNoteSurfaces(fact.Members, fact.SupportRefs, fact.MemberNotes)
	}
	fact = normalizeLegacyNegativeSearchByNonRepoOrigin(fact)
	fact, err = normalizeNegativeSearchAggregateFact(fact)
	if err != nil {
		return AnswerAggregateFact{}, err
	}
	fact, err = normalizeNegativeObservationAggregateFact(fact)
	if err != nil {
		return AnswerAggregateFact{}, err
	}
	if fact.Kind == AnswerAggregateMemberSet && len(fact.Members) > 0 {
		if fact.Value == "" {
			fact.Value = strconv.Itoa(len(fact.Members))
		} else if n, err := strconv.Atoi(fact.Value); err == nil && n >= 0 && n != len(fact.Members) {
			fact.Value = strconv.Itoa(len(fact.Members))
		}
		fact.Label = normalizeAggregateMemberSetLabelCardinality(fact.Label, len(fact.Members))
	}
	if aggregateCountCanDeriveValueFromMembers(fact.Kind) && fact.Value == "" && len(fact.Members) > 0 {
		fact.Value = strconv.Itoa(len(fact.Members))
	}
	if fact.Value == "" {
		return AnswerAggregateFact{}, fmt.Errorf("value is required")
	}
	return fact, nil
}

func aggregateCountCanDeriveValueFromMembers(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateGroupedCount, AnswerAggregateBucketCount:
		return true
	default:
		return false
	}
}

func normalizeAggregateFactOriginDimensions(fact AnswerAggregateFact) []AnswerAggregateDimension {
	if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
		return fact.Dimensions
	}
	origins := answerAggregateFactExplicitEvidenceOrigins(fact)
	if len(origins) == 0 {
		return fact.Dimensions
	}
	out := append([]AnswerAggregateDimension(nil), fact.Dimensions...)
	dims := aggregateDimensionMap(out)
	for _, origin := range origins {
		if origin == AnswerEvidenceOriginUnknown || origin == AnswerEvidenceOriginCurrentSource {
			continue
		}
		name := aggregateFactOriginDimensionName(origin, dims)
		if name == "" {
			continue
		}
		if len(out) >= maxAnswerAggregateDimensions {
			break
		}
		out = append(out, AnswerAggregateDimension{Name: name, Value: string(origin)})
		dims[name] = string(origin)
	}
	return out
}

func aggregateFactOriginDimensionName(origin AnswerEvidenceOrigin, dims map[string]string) string {
	switch origin {
	case AnswerEvidenceOriginVCSDiff:
		if dims["origin"] == "" && dims["evidence_origin"] == "" {
			return "origin"
		}
		if dims["diff_origin"] == "" {
			return "diff_origin"
		}
	case AnswerEvidenceOriginCommandMeasurement:
		if dims["origin"] == "" && dims["evidence_origin"] == "" {
			return "origin"
		}
		if dims["measurement_origin"] == "" {
			return "measurement_origin"
		}
	default:
		if dims["origin"] == "" {
			return "origin"
		}
		if dims["evidence_origin"] == "" {
			return "evidence_origin"
		}
		if dims["secondary_origin"] == "" {
			return "secondary_origin"
		}
	}
	return ""
}

type aggregateMemberSupportSurface struct {
	member string
	ref    string
	note   string
	label  string
	loc    AnswerSourceLocationSurface
	hasLoc bool
}

func normalizeAggregateMemberSetMemberSupportSurfaces(members []string, refs []string) ([]string, []string) {
	outMembers, outRefs, _ := normalizeAggregateMemberSetMemberSupportNoteSurfaces(members, refs, nil)
	return outMembers, outRefs
}

func normalizeAggregateMemberSetMemberSupportNoteSurfaces(members []string, refs []string, notes []string) ([]string, []string, []string) {
	if len(members) == 0 {
		return nil, nil, nil
	}
	out := make([]aggregateMemberSupportSurface, 0, len(members))
	for i, member := range members {
		ref := ""
		if i < len(refs) {
			ref = strings.TrimSpace(refs[i])
		}
		note := ""
		if i < len(notes) {
			note = trimAggregateText(notes[i])
		}
		surface := normalizeAggregateMemberSupportSurface(member, ref)
		surface.note = note
		if surface.member == "" {
			continue
		}
		merged := false
		for j := range out {
			if !aggregateMemberSupportSurfacesEquivalent(out[j], surface) {
				continue
			}
			out[j] = mergeAggregateMemberSupportSurface(out[j], surface)
			merged = true
			break
		}
		if !merged {
			out = append(out, surface)
		}
	}
	if len(out) == 0 {
		return nil, nil, nil
	}
	outMembers := make([]string, 0, len(out))
	outRefs := make([]string, len(out))
	outNotes := make([]string, len(out))
	seenRefs := make(map[string]bool, len(out)+len(refs))
	lastRef := -1
	lastNote := -1
	for i, surface := range out {
		outMembers = append(outMembers, surface.member)
		if ref := strings.TrimSpace(surface.ref); ref != "" {
			outRefs[i] = ref
			seenRefs[strings.ToLower(ref)] = true
			lastRef = i
		}
		if note := trimAggregateText(surface.note); note != "" {
			outNotes[i] = note
			lastNote = i
		}
	}
	if len(refs) > len(members) {
		for _, ref := range refs[len(members):] {
			ref = normalizeAggregateMemberExtraSupportRef(ref, outMembers)
			if ref == "" {
				continue
			}
			key := strings.ToLower(ref)
			if seenRefs[key] {
				continue
			}
			seenRefs[key] = true
			outRefs = append(outRefs, ref)
			lastRef = len(outRefs) - 1
		}
	}
	if lastNote < 0 {
		outNotes = nil
	} else {
		outNotes = outNotes[:lastNote+1]
	}
	if lastRef < 0 {
		return outMembers, nil, outNotes
	}
	return outMembers, outRefs[:lastRef+1], outNotes
}

func normalizeAggregateMemberExtraSupportRef(ref string, members []string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	refMember, _, ok := ParseAnswerSupportRefMemberLocation(ref)
	if !ok {
		return ""
	}
	if len(members) == 0 {
		return ""
	}
	refMember = strings.TrimSpace(refMember)
	if refMember == "" || AnswerSupportRefLabelIsGeneric(refMember) {
		return ref
	}
	for _, member := range members {
		if aggregateSupportRefCanDescribeMember(refMember, aggregateMemberSupportSurfaceLabel(member)) {
			return ref
		}
	}
	return ""
}

func normalizeAggregateMemberSupportSurface(member string, ref string) aggregateMemberSupportSurface {
	member = strings.TrimSpace(member)
	ref = strings.TrimSpace(ref)
	surface := aggregateMemberSupportSurface{member: member, ref: ref}
	if label, loc, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		surface.member = strings.TrimSpace(label)
		surface.label = strings.TrimSpace(label)
		surface.loc = loc
		surface.hasLoc = loc.File != "" && loc.LineStart > 0
		surface.ref = chooseAggregateMemberSupportRef(ref, formatAggregateMemberSupportRef(surface.label, loc))
		return surface
	}
	surface.label = aggregateMemberSupportSurfaceLabel(member)
	if refLabel, refLoc, ok := ParseAnswerSupportRefMemberLocation(ref); ok && aggregateSupportRefCanDescribeMember(refLabel, surface.label) {
		surface.loc = refLoc
		surface.hasLoc = refLoc.File != "" && refLoc.LineStart > 0
	}
	if surface.label == "" {
		surface.label = member
	}
	return surface
}

func aggregateMemberSupportSurfaceLabel(member string) string {
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	if surface, ok := ParseAnswerSourceLocationSurface(member); ok {
		return aggregateMemberStartLocation(surface)
	}
	if base, _, ok := AnswerAggregateDecoratedLabelParts(member); ok && strings.TrimSpace(base) != "" {
		return strings.TrimSpace(base)
	}
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(member) {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return strings.TrimSpace(member)
}

func aggregateSupportRefCanDescribeMember(refLabel string, memberLabel string) bool {
	refLabel = strings.TrimSpace(refLabel)
	memberLabel = strings.TrimSpace(memberLabel)
	if refLabel == "" || AnswerSupportRefLabelIsGeneric(refLabel) {
		return true
	}
	if memberLabel == "" {
		return false
	}
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(memberLabel) {
		if strings.EqualFold(strings.TrimSpace(candidate), refLabel) {
			return true
		}
	}
	return false
}

func formatAggregateMemberSupportRef(label string, loc AnswerSourceLocationSurface) string {
	label = strings.TrimSpace(label)
	location := aggregateMemberStartLocation(loc)
	if label == "" || location == "" {
		return ""
	}
	return label + " @ " + location
}

func chooseAggregateMemberSupportRef(existing string, candidate string) string {
	existing = strings.TrimSpace(existing)
	candidate = strings.TrimSpace(candidate)
	if existing == "" {
		return candidate
	}
	if candidate == "" {
		return existing
	}
	_, existingLoc, existingOK := ParseAnswerSupportRefMemberLocation(existing)
	_, candidateLoc, candidateOK := ParseAnswerSupportRefMemberLocation(candidate)
	if !existingOK || !candidateOK {
		return existing
	}
	if candidateLoc.LineStart == existingLoc.LineStart &&
		aggregateSupportRefPathCorresponds(candidateLoc.File, existingLoc.File) &&
		aggregateSupportRefMoreSpecific(candidateLoc.File, existingLoc.File) {
		return candidate
	}
	return existing
}

func aggregateMemberSupportSurfacesEquivalent(a, b aggregateMemberSupportSurface) bool {
	aLabel := strings.ToLower(strings.TrimSpace(aggregateMemberSetProjectionMemberKey(a.label)))
	bLabel := strings.ToLower(strings.TrimSpace(aggregateMemberSetProjectionMemberKey(b.label)))
	if aLabel == "" || bLabel == "" || aLabel != bLabel {
		return false
	}
	if !a.hasLoc || !b.hasLoc {
		return true
	}
	return a.loc.LineStart == b.loc.LineStart &&
		aggregateSupportRefPathCorresponds(a.loc.File, b.loc.File)
}

func mergeAggregateMemberSupportSurface(existing, candidate aggregateMemberSupportSurface) aggregateMemberSupportSurface {
	out := existing
	if aggregateMemberSupportSurfacePrefersMember(candidate, existing) {
		out.member = candidate.member
		out.label = candidate.label
	}
	if !out.hasLoc && candidate.hasLoc {
		out.loc = candidate.loc
		out.hasLoc = true
	}
	out.ref = chooseAggregateMemberSupportRef(out.ref, candidate.ref)
	out.note = MergeEvidenceSummaries(out.note, candidate.note)
	return out
}

func aggregateMemberSupportSurfacePrefersMember(candidate, existing aggregateMemberSupportSurface) bool {
	if strings.TrimSpace(candidate.member) == "" {
		return false
	}
	if _, _, ok := ParseAnswerSupportRefMemberLocation(existing.member); ok {
		return true
	}
	if _, ok := ParseAnswerSourceLocationSurface(existing.member); ok {
		return true
	}
	if _, _, ok := ParseAnswerSupportRefMemberLocation(candidate.member); ok {
		return false
	}
	if _, ok := ParseAnswerSourceLocationSurface(candidate.member); ok {
		return false
	}
	return len([]rune(candidate.member)) < len([]rune(existing.member))
}

func normalizeNegativeSearchAggregateFact(fact AnswerAggregateFact) (AnswerAggregateFact, error) {
	if fact.Kind != AnswerAggregateNegativeSearch {
		return fact, nil
	}
	fact.Dimensions = canonicalNegativeSearchDimensions(fact.Dimensions)
	dims := aggregateDimensionMap(fact.Dimensions)
	if fact.Value == "" {
		fact.Value = dims["result_count"]
	}
	if fact.Value == "" {
		fact.Value = "0"
	}
	n, err := strconv.Atoi(strings.TrimSpace(fact.Value))
	if err != nil || n != 0 {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q must use value \"0\" for a verified zero-result search",
			fact.Kind, fact.Label)
	}
	fact.Value = "0"
	if dims["result_count"] == "" {
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension result_count=0 for a verified zero-result search",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "result_count", Value: "0"})
		dims["result_count"] = "0"
	}
	if fact.Unit == "" {
		fact.Unit = "matches"
	}
	if dims["repo"] == "" {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension repo=<active repository or sub-repo>",
			fact.Kind, fact.Label)
	}
	if dims["query"] == "" && dims["pattern"] == "" {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension query=<search query> or pattern=<search regex>",
			fact.Kind, fact.Label)
	}
	if dims["scope"] == "" {
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension scope=<searched repository path or bounded search surface>",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "scope", Value: dims["repo"]})
		dims["scope"] = dims["repo"]
	}
	if dims["searched_at"] == "" {
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension searched_at=<search timestamp or current_investigation>",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "searched_at", Value: "current_investigation"})
	}
	return fact, nil
}

func normalizeLegacyNegativeSearchByNonRepoOrigin(fact AnswerAggregateFact) AnswerAggregateFact {
	if fact.Kind != AnswerAggregateNegativeSearch {
		return fact
	}
	dims := aggregateDimensionMap(canonicalNegativeSearchDimensions(fact.Dimensions))
	if dims["repo"] != "" {
		return fact
	}
	if len(negativeObservationNonRepoOrigins(fact)) == 0 {
		return fact
	}
	fact.Kind = AnswerAggregateNegativeObservation
	return fact
}

func normalizeNegativeObservationAggregateFact(fact AnswerAggregateFact) (AnswerAggregateFact, error) {
	if fact.Kind != AnswerAggregateNegativeObservation {
		return fact, nil
	}
	fact.Dimensions = canonicalNegativeObservationDimensions(fact.Dimensions)
	dims := aggregateDimensionMap(fact.Dimensions)
	if fact.Value == "" {
		fact.Value = dims["result_count"]
	}
	if fact.Value == "" {
		fact.Value = "0"
	}
	n, err := strconv.Atoi(strings.TrimSpace(fact.Value))
	if err != nil || n != 0 {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q must use value \"0\" for a verified zero-result observation",
			fact.Kind, fact.Label)
	}
	fact.Value = "0"
	if dims["result_count"] == "" {
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension result_count=0 for a verified zero-result observation",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "result_count", Value: "0"})
		dims["result_count"] = "0"
	}
	if fact.Unit == "" {
		fact.Unit = "matches"
	}
	origins := negativeObservationNonRepoOrigins(fact)
	if len(origins) == 0 {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q requires a non-repo origin dimension such as origin=vcs_metadata, origin=vcs_diff, origin=runtime_artifact, origin=command_measurement, origin=cross_repo_index, origin=web_page, origin=mcp_resource, origin=external_document, or origin=connector_resource",
			fact.Kind, fact.Label)
	}
	if dims["target"] == "" && dims["query"] == "" && dims["pattern"] == "" && dims["predicate"] == "" {
		return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension target, query, pattern, or predicate for the absent thing",
			fact.Kind, fact.Label)
	}
	if dims["scope"] == "" {
		scope := firstNonEmptyAggregateDim(dims, "artifact_id", "trace_window", "commit_range", "tool_result", "source_ref")
		if scope == "" {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension scope=<bounded observed surface>",
				fact.Kind, fact.Label)
		}
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension scope=<bounded observed surface>",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "scope", Value: scope})
		dims["scope"] = scope
	}
	if dims["searched_at"] == "" {
		if len(fact.Dimensions) >= maxAnswerAggregateDimensions {
			return AnswerAggregateFact{}, fmt.Errorf("%s %q requires dimension searched_at=<search timestamp or current_investigation>",
				fact.Kind, fact.Label)
		}
		fact.Dimensions = append(fact.Dimensions, AnswerAggregateDimension{Name: "searched_at", Value: "current_investigation"})
	}
	return fact, nil
}

func negativeObservationNonRepoOrigins(fact AnswerAggregateFact) []AnswerEvidenceOrigin {
	var out []AnswerEvidenceOrigin
	seen := map[AnswerEvidenceOrigin]bool{}
	for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
		switch origin {
		case AnswerEvidenceOriginVCSMetadata,
			AnswerEvidenceOriginVCSDiff,
			AnswerEvidenceOriginRuntimeArtifact,
			AnswerEvidenceOriginCommandMeasurement,
			AnswerEvidenceOriginCrossRepoIndex,
			AnswerEvidenceOriginExternalDocument,
			AnswerEvidenceOriginWebPage,
			AnswerEvidenceOriginMCPResource,
			AnswerEvidenceOriginConnectorResource,
			AnswerEvidenceOriginSystemInference:
			if !seen[origin] {
				seen[origin] = true
				out = append(out, origin)
			}
		}
	}
	return out
}

func firstNonEmptyAggregateDim(dims map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(dims[name]); value != "" {
			return value
		}
	}
	return ""
}

func canonicalNegativeSearchDimensions(dims []AnswerAggregateDimension) []AnswerAggregateDimension {
	if len(dims) == 0 {
		return nil
	}
	out := make([]AnswerAggregateDimension, 0, len(dims))
	seen := map[string]bool{}
	for _, dim := range dims {
		name := canonicalNegativeSearchDimensionName(dim.Name)
		value := trimAggregateText(dim.Value)
		if name == "" || value == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, AnswerAggregateDimension{Name: name, Value: value})
	}
	return out
}

func canonicalNegativeObservationDimensions(dims []AnswerAggregateDimension) []AnswerAggregateDimension {
	if len(dims) == 0 {
		return nil
	}
	out := make([]AnswerAggregateDimension, 0, len(dims))
	seen := map[string]bool{}
	for _, dim := range dims {
		name := canonicalNegativeObservationDimensionName(dim.Name)
		value := trimAggregateText(dim.Value)
		if name == "" || value == "" {
			continue
		}
		if name == "origin" {
			if origin := AnswerEvidenceOriginFromStructuredToken(value); origin != AnswerEvidenceOriginUnknown && origin.IsValid() {
				value = string(origin)
			}
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, AnswerAggregateDimension{Name: name, Value: value})
	}
	return out
}

func canonicalNegativeSearchDimensionName(raw string) string {
	name := strings.ToLower(trimAggregateText(raw))
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	switch name {
	case "repo", "repository", "sub_repo", "subrepository", "active_repo", "active_repository":
		return "repo"
	case "query", "search_query", "term", "terms":
		return "query"
	case "pattern", "regex", "regexp", "search_pattern":
		return "pattern"
	case "scope", "search_scope", "path", "search_path", "root", "root_rel":
		return "scope"
	case "searched_at", "search_time", "searched_in", "tool_time", "time":
		return "searched_at"
	case "result_count", "count", "matches", "match_count":
		return "result_count"
	case "tool", "file_type", "files_only", "ignore_case":
		return name
	default:
		return trimAggregateText(raw)
	}
}

func canonicalNegativeObservationDimensionName(raw string) string {
	name := strings.ToLower(trimAggregateText(raw))
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	switch name {
	case "origin", "evidence_origin", "source_origin":
		return "origin"
	case "query", "search_query", "term", "terms":
		return "query"
	case "pattern", "regex", "regexp", "search_pattern":
		return "pattern"
	case "target", "subject", "symbol", "member", "entity":
		return "target"
	case "checked_types", "checked_type",
		"absent_types", "absent_type",
		"checked_events", "event_types", "observed_types",
		"missing_signal", "missing_signals",
		"absent_signal", "absent_signals",
		"missing_field", "missing_fields",
		"absent_field", "absent_fields",
		"missing_event", "missing_events",
		"absent_event", "absent_events",
		"missing_type", "missing_types":
		return "target"
	case "predicate", "condition", "absence", "negative_predicate":
		return "predicate"
	case "scope", "search_scope", "observed_scope", "window", "range":
		return "scope"
	case "artifact", "artifact_id", "log", "log_id", "trace", "trace_id":
		return "artifact_id"
	case "trace_window", "span_window", "time_window":
		return "trace_window"
	case "commit_range", "git_range", "revision_range", "history_window":
		return "commit_range"
	case "tool_result", "tool_result_id", "command_id", "command_result":
		return "tool_result"
	case "source", "source_ref", "tool", "producer":
		return "source_ref"
	case "searched_at", "search_time", "searched_in", "tool_time", "time":
		return "searched_at"
	case "result_count", "count", "matches", "match_count", "observed_count":
		return "result_count"
	default:
		return trimAggregateText(raw)
	}
}

func aggregateDimensionMap(dims []AnswerAggregateDimension) map[string]string {
	out := make(map[string]string, len(dims))
	for _, dim := range dims {
		name := strings.ToLower(strings.TrimSpace(dim.Name))
		if name == "" || out[name] != "" {
			continue
		}
		out[name] = strings.TrimSpace(dim.Value)
	}
	return out
}

// AnswerAggregateFactIdentity returns the stable semantic identity for a
// model-authored aggregate fact. For ordinary scalar/count facts, the label is
// part of the identity because it names the measured quantity. For member_set
// facts, the exact member set is the principal answer payload, while label is
// display metadata that may drift across explore/reconcile retries. Treating
// equivalent member sets as one handoff prevents downstream hard gates from
// requiring duplicate exhaustive lists in different surface formats.
func AnswerAggregateFactIdentity(fact AnswerAggregateFact) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(string(fact.Kind))))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Value)))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(renderAggregateDimensionsKey(fact.Dimensions)))
	b.WriteByte('\x00')
	if fact.Kind == AnswerAggregateMemberSet && len(fact.Members) > 0 {
		b.WriteString(canonicalAggregateMemberSetKey(fact.Members))
		return b.String()
	}
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Label)))
	return b.String()
}

func canonicalAggregateMemberSetKey(members []string) string {
	if len(members) == 0 {
		return ""
	}
	if relaxed, ok := canonicalAggregateMemberSetRelaxedKey(members); ok {
		return relaxed
	}
	keys := make([]string, 0, len(members))
	seen := map[string]bool{}
	for _, member := range members {
		key := AnswerAggregateMemberSurfaceKey(member)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f")
}

func canonicalAggregateMemberSetRelaxedKey(members []string) (string, bool) {
	if len(members) < 2 {
		return "", false
	}
	keys := make([]string, 0, len(members))
	seen := map[string]bool{}
	relaxedAny := false
	for _, member := range members {
		key, ok, relaxed := aggregateRelationSurfaceRelaxedKey(member)
		if !ok {
			key = AnswerAggregateMemberSurfaceKey(member)
		}
		if key == "" || seen[key] {
			return "", false
		}
		if relaxed {
			relaxedAny = true
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if !relaxedAny {
		return "", false
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f"), true
}

// AnswerAggregateMemberSurfaceKey canonicalizes a model-emitted member_set
// member for equivalence checks. It is deliberately conservative: only compact
// two-part identifier relations are normalized across separators such as
// "pkg/Fn", "pkg → Fn", "pkg -> Fn", and "Type::Member". Paths, routes, and
// source locations remain literal surfaces so hard gates do not collapse
// unrelated answer members.
func AnswerAggregateMemberSurfaceKey(member string) string {
	member = trimAggregateMemberSurface(member)
	if member == "" {
		return ""
	}
	if key, ok := aggregateRelationSurfaceKey(member); ok {
		return key
	}
	return "literal:" + strings.ToLower(member)
}

// AnswerAggregateMemberDisplayCandidates returns exact visible spellings that
// should satisfy a member_set hard gate for the same model-authored member.
// These are display variants of the structured member, not inferred answer
// facts.
func AnswerAggregateMemberDisplayCandidates(member string) []string {
	member = trimAggregateMemberSurface(member)
	if member == "" {
		return nil
	}
	out := []string{member}
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		out = append(out, AnswerAggregateMemberDisplayCandidates(label)...)
	}
	if left, right, ok := aggregateRelationSurfaceParts(member); ok {
		for _, rightDisplay := range aggregateRelationPartDisplayForms(right) {
			out = append(out, aggregateRelationDisplayCandidates(left, rightDisplay)...)
		}
		if tail := aggregateRelationPartDisplayTail(right); tail != "" && !strings.EqualFold(tail, right) {
			out = append(out, aggregateRelationDisplayCandidates(left, tail)...)
		}
		if tail := aggregateRelationPartTail(right); tail != "" && !strings.EqualFold(tail, right) {
			out = append(out, aggregateRelationDisplayCandidates(left, tail)...)
		}
	}
	return dedupAggregateMemberCandidates(out)
}

func aggregateRelationDisplayCandidates(left, right string) []string {
	if left == "" || right == "" {
		return nil
	}
	return []string{
		left + " → " + right,
		left + " -> " + right,
		left + ": " + right,
		left + "/" + right,
		left + "::" + right,
	}
}

func aggregateRelationPartDisplayForms(part string) []string {
	part = trimAggregateMemberSurface(part)
	if part == "" {
		return nil
	}
	out := []string{part}
	if base, qualifier, ok := aggregateRelationPartDecorator(part); ok {
		out = append(out, base+"("+qualifier+")")
		out = append(out, base+" ("+qualifier+")")
		if aggregateRelationDecoratorIsSourceLine(qualifier) {
			out = append(out, base)
		} else if aggregateRelationDecoratorLooksCallable(part, base) {
			out = append(out, base)
		}
	} else if base, ok := aggregateRelationCallableSignatureBase(part); ok {
		out = append(out, base)
		if tail := aggregateRelationPartDisplayTail(base); tail != "" && !strings.EqualFold(tail, base) {
			out = append(out, tail)
		}
	}
	return dedupAggregateMemberCandidates(out)
}

func AnswerAggregateMemberRelationParts(member string) (left string, right string, ok bool) {
	return aggregateRelationSurfaceParts(member)
}

// AnswerAggregateDecoratedLabelParts parses display labels such as
// "Score (subject)" or "New (Classifier)". These labels are not standalone
// relation members, but the qualifier is still load-bearing when selecting a
// citation among same-named symbols.
func AnswerAggregateDecoratedLabelParts(label string) (base string, qualifier string, ok bool) {
	return aggregateRelationPartDecorator(label)
}

func aggregateRelationSurfaceKey(member string) (string, bool) {
	left, right, ok := aggregateRelationSurfaceParts(member)
	if !ok {
		return "", false
	}
	return "relation:" + aggregateRelationPartKey(left) + "\x00" + aggregateRelationPartKey(right), true
}

func aggregateRelationSurfaceRelaxedKey(member string) (key string, ok bool, relaxed bool) {
	left, right, ok := aggregateRelationSurfaceParts(member)
	if !ok {
		return "", false, false
	}
	leftKey := aggregateRelationPartKey(left)
	rightKey := aggregateRelationPartKey(right)
	tail := aggregateRelationPartTail(right)
	if tail == "" || strings.EqualFold(tail, rightKey) {
		return "relation:" + leftKey + "\x00" + rightKey, true, false
	}
	return "relation:" + leftKey + "\x00" + tail, true, true
}

func aggregateRelationSurfaceParts(member string) (string, string, bool) {
	member = trimAggregateMemberSurface(member)
	if member == "" {
		return "", "", false
	}
	for _, sep := range []string{"→", "->", "=>", "::"} {
		if strings.Count(member, sep) != 1 {
			if (sep == "->" || sep == "=>") && strings.Count(member, sep) > 1 {
				left, right := aggregateRelationSurfaceSplit(member, sep)
				if aggregateRelationPartOK(left) && aggregateRelationPartOK(right) {
					return left, right, true
				}
			}
			continue
		}
		left, right := aggregateRelationSurfaceSplit(member, sep)
		if aggregateRelationPartOK(left) && aggregateRelationPartOK(right) {
			return left, right, true
		}
	}
	if strings.Count(member, ":") == 1 && aggregateColonLooksLikeDisplayRelation(member) {
		parts := strings.Split(member, ":")
		left, right := trimAggregateMemberSurface(parts[0]), trimAggregateMemberSurface(parts[1])
		if aggregateRelationPartOK(left) && aggregateRelationPartOK(right) {
			return left, right, true
		}
	}
	if strings.Count(member, "/") == 1 && !strings.ContainsAny(member, " \t\n\r") {
		parts := strings.Split(member, "/")
		left, right := trimAggregateMemberSurface(parts[0]), trimAggregateMemberSurface(parts[1])
		if aggregateRelationPartOK(left) && aggregateRelationPartOK(right) {
			return left, right, true
		}
	}
	if left, right, ok := aggregateCompactDotRelationParts(member); ok {
		return left, right, true
	}
	return "", "", false
}

func aggregateRelationSurfaceSplit(member, sep string) (left string, right string) {
	idx := strings.Index(member, sep)
	if idx < 0 {
		return "", ""
	}
	return trimAggregateMemberSurface(member[:idx]), trimAggregateMemberSurface(member[idx+len(sep):])
}

func aggregateCompactDotRelationParts(member string) (string, string, bool) {
	if strings.Count(member, ".") != 1 || strings.ContainsAny(member, `/\`) || HasCodeOrConfigPathSuffix(member) {
		return "", "", false
	}
	parts := strings.Split(member, ".")
	left, right := trimAggregateMemberSurface(parts[0]), trimAggregateMemberSurface(parts[1])
	if !aggregateRelationAtomOK(left) || !aggregateRelationPartOK(right) {
		return "", "", false
	}
	return left, right, true
}

func aggregateColonLooksLikeDisplayRelation(member string) bool {
	idx := strings.Index(member, ":")
	if idx <= 0 || idx >= len(member)-1 {
		return false
	}
	before := member[idx-1]
	after := member[idx+1]
	// Keep colon support to display labels such as "package: Entry".
	// Source locations, URLs, drive letters, and dense key:value strings
	// remain literal member surfaces.
	return unicode.IsSpace(rune(before)) || unicode.IsSpace(rune(after))
}

func aggregateRelationPartOK(part string) bool {
	part = trimAggregateMemberSurface(part)
	if part == "" || strings.Contains(part, `\`) {
		return false
	}
	if base, qualifier, ok := aggregateRelationPartDecorator(part); ok {
		if aggregateRelationDecoratorIsSourceLine(qualifier) {
			return aggregateRelationCorePartOK(base)
		}
		if aggregateRelationCorePartOK(base) && aggregateRelationQualifierOK(qualifier) {
			return true
		}
		return aggregateRelationDecoratorLooksCallable(part, base) && aggregateRelationCorePartOK(base)
	}
	if _, ok := aggregateRelationCallableSignatureBase(part); ok {
		return true
	}
	if strings.Contains(part, "/") {
		return false
	}
	return aggregateRelationCorePartOK(part)
}

func aggregateRelationCallableSignatureBase(part string) (string, bool) {
	part = trimAggregateMemberSurface(part)
	if part == "" || strings.ContainsAny(part, "\n\r\\") || HasCodeOrConfigPathSuffix(part) {
		return "", false
	}
	open := strings.Index(part, "(")
	if open <= 0 {
		return "", false
	}
	if !aggregateRelationSignatureParensBalanced(part[open:]) {
		return "", false
	}
	base := aggregateRelationCallableNameFromPrefix(part[:open])
	if base == "" || !aggregateRelationCorePartOK(base) {
		return "", false
	}
	return base, true
}

func aggregateRelationSignatureParensBalanced(s string) bool {
	depth := 0
	seenClose := false
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
			seenClose = true
		}
	}
	return seenClose && depth == 0
}

func aggregateRelationCallableNameFromPrefix(prefix string) string {
	prefix = aggregateRelationTrimTrailingGenericSuffix(trimAggregateMemberSurface(prefix))
	if prefix == "" {
		return ""
	}
	end := len(prefix)
	for end > 0 {
		r, size := utf8LastRune(prefix[:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	start := end
	for start > 0 {
		r, size := utf8LastRune(prefix[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '$' || r == '.' || r == ':' {
			start -= size
			continue
		}
		break
	}
	return trimAggregateMemberSurface(prefix[start:end])
}

func aggregateRelationTrimTrailingGenericSuffix(prefix string) string {
	for {
		prefix = trimAggregateMemberSurface(prefix)
		if len(prefix) < 3 {
			return prefix
		}
		close := prefix[len(prefix)-1]
		var open byte
		switch close {
		case ']':
			open = '['
		case '>':
			open = '<'
		default:
			return prefix
		}
		idx := strings.LastIndexByte(prefix, open)
		if idx <= 0 {
			return prefix
		}
		before := trimAggregateMemberSurface(prefix[:idx])
		if before == "" || strings.ContainsAny(prefix[idx+1:len(prefix)-1], "\n\r") {
			return prefix
		}
		prefix = before
	}
}

func utf8LastRune(s string) (rune, int) {
	for i := len(s); i > 0; {
		r, size := rune(s[i-1]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeLastRuneInString(s[:i])
		}
		return r, size
	}
	return 0, 0
}

func aggregateRelationQualifierOK(qualifier string) bool {
	qualifier = trimAggregateMemberSurface(qualifier)
	if qualifier == "" || strings.Contains(qualifier, `\`) {
		return false
	}
	parts := strings.FieldsFunc(qualifier, func(r rune) bool {
		switch r {
		case '/', '+', ',', '，', '、', '&', '|':
			return true
		default:
			return false
		}
	})
	seenPart := false
	for _, part := range parts {
		part = trimAggregateMemberSurface(part)
		if part == "" {
			continue
		}
		seenPart = true
		if !aggregateRelationCorePartOK(part) {
			return false
		}
	}
	return seenPart
}

func aggregateRelationCorePartOK(part string) bool {
	part = trimAggregateMemberSurface(part)
	if part == "" || strings.ContainsAny(part, `/\`) {
		return false
	}
	if strings.Contains(part, "::") {
		return aggregateNamespaceQualifiedRelationPartOK(part)
	}
	if strings.Contains(part, ".") {
		return aggregateQualifiedRelationPartOK(part)
	}
	return aggregateRelationAtomOK(part)
}

func aggregateNamespaceQualifiedRelationPartOK(part string) bool {
	if HasCodeOrConfigPathSuffix(part) {
		return false
	}
	if strings.HasPrefix(part, "::") || strings.HasSuffix(part, "::") || strings.Contains(part, "::::") {
		return false
	}
	segments := strings.Split(part, "::")
	if len(segments) < 2 {
		return false
	}
	for _, segment := range segments {
		segment = trimAggregateMemberSurface(segment)
		if segment == "" {
			return false
		}
		if strings.Contains(segment, ".") {
			if !aggregateQualifiedRelationPartOK(segment) {
				return false
			}
			continue
		}
		if !aggregateRelationAtomOK(segment) {
			return false
		}
	}
	return true
}

func aggregateQualifiedRelationPartOK(part string) bool {
	if HasCodeOrConfigPathSuffix(part) {
		return false
	}
	if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") || strings.Contains(part, "..") {
		return false
	}
	segments := strings.Split(part, ".")
	if len(segments) < 2 {
		return false
	}
	for _, segment := range segments {
		if !aggregateRelationAtomOK(segment) {
			return false
		}
	}
	return true
}

func aggregateRelationAtomOK(part string) bool {
	hasAlphaNum := false
	for _, r := range part {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			hasAlphaNum = true
		case r == '_' || r == '-' || r == '$':
		default:
			return false
		}
	}
	return hasAlphaNum
}

func aggregateRelationPartKey(part string) string {
	part = trimAggregateMemberSurface(part)
	if base, qualifier, ok := aggregateRelationPartDecorator(part); ok {
		return strings.ToLower(base + "(" + qualifier + ")")
	}
	return strings.ToLower(part)
}

func aggregateRelationPartTail(part string) string {
	if !aggregateQualifiedRelationPartOK(part) && !aggregateNamespaceQualifiedRelationPartOK(part) {
		return ""
	}
	return NormalizedSurfaceSymbolTail(part)
}

func aggregateRelationPartDisplayTail(part string) string {
	part = trimAggregateMemberSurface(part)
	if !aggregateQualifiedRelationPartOK(part) && !aggregateNamespaceQualifiedRelationPartOK(part) {
		return ""
	}
	idx := strings.LastIndex(part, "::")
	sepLen := len("::")
	if idx < 0 {
		idx = strings.LastIndex(part, ".")
		sepLen = len(".")
	}
	if idx < 0 || idx >= len(part)-1 {
		return ""
	}
	return strings.TrimSpace(part[idx+sepLen:])
}

func aggregateRelationPartDecorator(part string) (base string, qualifier string, ok bool) {
	part = trimAggregateMemberSurface(part)
	if part == "" || !strings.HasSuffix(part, ")") {
		return "", "", false
	}
	idx := strings.LastIndex(part, "(")
	if idx <= 0 || idx >= len(part)-1 {
		return "", "", false
	}
	base = trimAggregateMemberSurface(part[:idx])
	qualifier = trimAggregateMemberSurface(strings.TrimSuffix(part[idx+1:], ")"))
	if base == "" || qualifier == "" ||
		strings.ContainsAny(base, "()") ||
		strings.ContainsAny(qualifier, "()") {
		return "", "", false
	}
	return base, qualifier, true
}

func aggregateRelationDecoratorLooksCallable(part, base string) bool {
	part = trimAggregateMemberSurface(part)
	base = trimAggregateMemberSurface(base)
	if part == "" || base == "" {
		return false
	}
	idx := strings.Index(part, "(")
	return idx == len(base)
}

func aggregateRelationDecoratorIsSourceLine(qualifier string) bool {
	qualifier = strings.ToLower(trimAggregateMemberSurface(qualifier))
	if qualifier == "" {
		return false
	}
	qualifier = strings.TrimPrefix(qualifier, "#")
	qualifier = strings.TrimSpace(qualifier)
	for _, prefix := range []string{"lines", "line", "ln", "l", "行", "第"} {
		if !strings.HasPrefix(qualifier, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(qualifier, prefix))
		if prefix == "第" {
			rest = strings.TrimSuffix(rest, "行")
			rest = strings.TrimSpace(rest)
		}
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimPrefix(rest, "#")
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return false
		}
		return aggregateRelationLineRefOK(rest)
	}
	return false
}

func aggregateRelationLineRefOK(rest string) bool {
	seenDigit := false
	for _, r := range rest {
		switch {
		case unicode.IsDigit(r):
			seenDigit = true
		case unicode.IsSpace(r):
		case r == ':' || r == '-' || r == ',' || r == '.' || r == '\u2013' || r == '\u2014':
		default:
			return false
		}
	}
	return seenDigit
}

func trimAggregateMemberSurface(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`'\"")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func dedupAggregateMemberCandidates(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = trimAggregateMemberSurface(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func normalizeAnswerAggregateDimensions(in []AnswerAggregateDimension) ([]AnswerAggregateDimension, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxAnswerAggregateDimensions {
		return nil, fmt.Errorf("dimensions has %d entries; max %d", len(in), maxAnswerAggregateDimensions)
	}
	out := make([]AnswerAggregateDimension, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		name := trimAggregateText(raw.Name)
		value := trimAggregateText(raw.Value)
		if name == "" || value == "" {
			continue
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, AnswerAggregateDimension{Name: name, Value: value})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeAggregateStrings(in []string, limit int) []string {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		item := trimAggregateText(raw)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimTrailingEmptyAggregateStrings(in []string) []string {
	last := -1
	for i, value := range in {
		if strings.TrimSpace(value) != "" {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return append([]string(nil), in[:last+1]...)
}

func trimAggregateText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxAnswerAggregateTextLen {
		s = strings.TrimSpace(s[:maxAnswerAggregateTextLen])
	}
	return s
}

func renderAggregateDimensionsKey(dims []AnswerAggregateDimension) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dims))
	for _, d := range dims {
		if d.Name == "" || d.Value == "" {
			continue
		}
		parts = append(parts, d.Name+"="+d.Value)
	}
	return strings.Join(parts, ";")
}

func validateAggregateCountCardinality(facts []AnswerAggregateFact) error {
	for _, fact := range facts {
		want, ok, err := parseAggregateCountValue(fact)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		switch fact.Kind {
		case AnswerAggregateTotalCount, AnswerAggregateUniqueCount, AnswerAggregateGroupedCount, AnswerAggregateBucketCount:
			if len(fact.Members) == 0 || want > maxAnswerAggregateMembers {
				continue
			}
			if len(fact.Members) != want {
				return fmt.Errorf("%s %q has value %d but %d member(s); omit partial members or provide the exact counted member set",
					fact.Kind, fact.Label, want, len(fact.Members))
			}
		case AnswerAggregateExcluded:
			if len(fact.Excluded) == 0 || want > maxAnswerAggregateMembers {
				continue
			}
			if len(fact.Excluded) != want {
				return fmt.Errorf("%s %q has value %d but %d excluded item(s); omit partial exclusions or provide the exact excluded set",
					fact.Kind, fact.Label, want, len(fact.Excluded))
			}
		case AnswerAggregateMemberSet:
			if len(fact.Members) == 0 {
				if want == 0 {
					continue
				}
				return fmt.Errorf("%s %q requires exact members; use scalar_value for prose-only summaries",
					fact.Kind, fact.Label)
			}
			if len(fact.Members) != want {
				return fmt.Errorf("%s %q has value %d but %d member(s); provide the exact member set or omit the fact",
					fact.Kind, fact.Label, want, len(fact.Members))
			}
		}
	}
	return nil
}

func parseAggregateCountValue(fact AnswerAggregateFact) (int, bool, error) {
	switch fact.Kind {
	case AnswerAggregateTotalCount, AnswerAggregateUniqueCount, AnswerAggregateGroupedCount, AnswerAggregateBucketCount, AnswerAggregateExcluded, AnswerAggregateMemberSet:
	default:
		return 0, false, nil
	}
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return 0, false, fmt.Errorf("%s %q requires a non-negative integer value", fact.Kind, fact.Label)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false, fmt.Errorf("%s %q has non-integer count value %q; put units in unit and keep value numeric",
			fact.Kind, fact.Label, fact.Value)
	}
	return n, true, nil
}

func validateAggregateFileLineMemberCompanions(facts []AnswerAggregateFact) error {
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateTotalCount && fact.Kind != AnswerAggregateGroupedCount && fact.Kind != AnswerAggregateBucketCount {
			continue
		}
		files := aggregateFileLineMemberFiles(fact.Members)
		if len(files) < 2 {
			continue
		}
		if aggregateFactsContainUniqueFileSet(facts, files) {
			continue
		}
		return fmt.Errorf("%s %q lists %d file:line member(s) across %d distinct file(s) but aggregate_facts does not include a matching unique_count fact with the distinct file members",
			fact.Kind, fact.Label, aggregateFileLineMemberCount(fact.Members), len(files))
	}
	return nil
}

func aggregateFileLineMemberFiles(members []string) []string {
	if len(members) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, member := range members {
		surface, ok := ParseAnswerSourceLocationSurface(member)
		if !ok || surface.File == "" {
			continue
		}
		if seen[surface.File] {
			continue
		}
		seen[surface.File] = true
		out = append(out, surface.File)
	}
	return out
}

func aggregateFileLineMemberCount(members []string) int {
	count := 0
	for _, member := range members {
		if _, ok := ParseAnswerSourceLocationSurface(member); ok {
			count++
		}
	}
	return count
}

func aggregateFactsContainUniqueFileSet(facts []AnswerAggregateFact, want []string) bool {
	if len(want) == 0 {
		return false
	}
	wantSet := map[string]bool{}
	for _, file := range want {
		wantSet[file] = true
	}
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateUniqueCount || strings.TrimSpace(fact.Value) != fmt.Sprintf("%d", len(wantSet)) {
			continue
		}
		gotFiles := aggregatePathMemberFiles(fact.Members)
		if len(gotFiles) != len(wantSet) {
			continue
		}
		matched := true
		for _, file := range gotFiles {
			if !wantSet[file] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func aggregatePathMemberFiles(members []string) []string {
	if len(members) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range members {
		file := normalizeAnswerLocationFile(strings.Trim(raw, "`'\" "))
		if file == "" || !HasCodeOrConfigPathSuffix(file) || strings.Contains(file, "\n") {
			continue
		}
		if seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func cloneAnswerAggregateFacts(in []AnswerAggregateFact) []AnswerAggregateFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerAggregateFact, len(in))
	for i, fact := range in {
		out[i] = fact
		if fact.Dimensions != nil {
			out[i].Dimensions = append([]AnswerAggregateDimension(nil), fact.Dimensions...)
		}
		if fact.Members != nil {
			out[i].Members = append([]string(nil), fact.Members...)
		}
		if fact.MemberNotes != nil {
			out[i].MemberNotes = append([]string(nil), fact.MemberNotes...)
		}
		if fact.Excluded != nil {
			out[i].Excluded = append([]string(nil), fact.Excluded...)
		}
		if fact.SupportRefs != nil {
			out[i].SupportRefs = append([]string(nil), fact.SupportRefs...)
		}
	}
	return out
}
