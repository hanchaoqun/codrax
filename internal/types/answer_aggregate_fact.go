package types

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// AnswerAggregateKind names a model-emitted aggregate fact that the
// finalizer should preserve as a structured exploration result. These
// facts are authored by the investigating model through
// emit_investigation_complete; the system stores and renders them but
// does not synthesize answer values from raw evidence on its own.
type AnswerAggregateKind string

const (
	AnswerAggregateUnknown      AnswerAggregateKind = ""
	AnswerAggregateTotalCount   AnswerAggregateKind = "total_count"
	AnswerAggregateUniqueCount  AnswerAggregateKind = "unique_count"
	AnswerAggregateGroupedCount AnswerAggregateKind = "grouped_count"
	AnswerAggregateBucketCount  AnswerAggregateKind = "bucket_count"
	AnswerAggregateExcluded     AnswerAggregateKind = "excluded_count"
	AnswerAggregateScalar       AnswerAggregateKind = "scalar_value"
	AnswerAggregateMemberSet    AnswerAggregateKind = "member_set"
)

var allAnswerAggregateKinds = []AnswerAggregateKind{
	AnswerAggregateTotalCount,
	AnswerAggregateUniqueCount,
	AnswerAggregateGroupedCount,
	AnswerAggregateBucketCount,
	AnswerAggregateExcluded,
	AnswerAggregateScalar,
	AnswerAggregateMemberSet,
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
	Unit        string                     `json:"unit,omitempty"`
	Dimensions  []AnswerAggregateDimension `json:"dimensions,omitempty"`
	Members     []string                   `json:"members,omitempty"`
	Excluded    []string                   `json:"excluded,omitempty"`
	SupportRefs []string                   `json:"support_refs,omitempty"`
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
// trimming. They do not infer or repair values from evidence. The one
// derived canonicalization is member_set.value: when the model emitted
// the complete members array but omitted value, value is set to
// len(members) from that same model-authored structured payload.
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
	if err := validateAggregateCountCardinality(out); err != nil {
		return nil, err
	}
	if err := validateAggregateFileLineMemberCompanions(out); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeAnswerAggregateFact(raw AnswerAggregateFact) (AnswerAggregateFact, error) {
	fact := AnswerAggregateFact{
		Kind:  raw.Kind,
		Label: trimAggregateText(raw.Label),
		Value: trimAggregateText(raw.Value),
		Unit:  trimAggregateText(raw.Unit),
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
	fact.Members = normalizeAggregateStrings(raw.Members, maxAnswerAggregateMembers)
	fact.Excluded = normalizeAggregateStrings(raw.Excluded, maxAnswerAggregateMembers)
	fact.SupportRefs = normalizeAggregateStrings(raw.SupportRefs, maxAnswerAggregateMembers)
	if fact.Kind == AnswerAggregateMemberSet && fact.Value == "" && len(fact.Members) > 0 {
		fact.Value = strconv.Itoa(len(fact.Members))
	}
	if fact.Value == "" {
		return AnswerAggregateFact{}, fmt.Errorf("value is required")
	}
	return fact, nil
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
	}
	return dedupAggregateMemberCandidates(out)
}

func AnswerAggregateMemberRelationParts(member string) (left string, right string, ok bool) {
	return aggregateRelationSurfaceParts(member)
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
			continue
		}
		parts := strings.Split(member, sep)
		left, right := trimAggregateMemberSurface(parts[0]), trimAggregateMemberSurface(parts[1])
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
	return "", "", false
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
	if part == "" || strings.ContainsAny(part, `/\`) {
		return false
	}
	if base, qualifier, ok := aggregateRelationPartDecorator(part); ok {
		return aggregateRelationCorePartOK(base) && aggregateRelationCorePartOK(qualifier)
	}
	return aggregateRelationCorePartOK(part)
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
		if fact.Excluded != nil {
			out[i].Excluded = append([]string(nil), fact.Excluded...)
		}
		if fact.SupportRefs != nil {
			out[i].SupportRefs = append([]string(nil), fact.SupportRefs...)
		}
	}
	return out
}
