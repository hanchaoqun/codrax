package types

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// EnumerationDisplaySet is the deterministic row contract for an accepted
// principal enumeration aggregate. It is compiled from structured investigation
// payloads and grounded evidence; it never derives members from user text,
// model thoughts, closure prose, or raw prompt snippets.
type EnumerationDisplaySet struct {
	ID              string
	FactIndex       int
	Label           string
	Value           string
	Unit            string
	Role            AnswerAggregateRole
	EvidenceOrigins []AnswerEvidenceOrigin
	Rows            []EnumerationDisplayRow
}

// EnumerationDisplayRow is one stable user-visible enumeration row. The row
// keeps identity, display, citation, and rich explanation together so the
// finalizer, renderer repair, and validators can consume the same contract.
type EnumerationDisplayRow struct {
	RowID        string
	SetID        string
	SetLabel     string
	FactIndex    int
	MemberIndex  int
	Member       string
	DisplayLabel string
	Category     string

	Source      string
	LineStart   int
	LineEnd     int
	Location    string
	CitationKey string
	HasCitation bool

	EvidenceID          string
	ClaimForm           ClaimForm
	LabelSurface        ClaimLabelSurfaceKind
	Subject             string
	Object              string
	AnchorSymbol        string
	OwnerSymbol         string
	MemberSurface       AnswerPrincipalMemberSurface
	AnchorKind          AnchorKind
	SurfaceTerms        []string
	EquivalentLocations []string
	Producer            string
	GroundingTier       GroundingTier
	EvidenceOrigins     []AnswerEvidenceOrigin
	Attributes          []EnumerationDisplayRowAttribute
	Note                string
	Detail              string
}

// EnumerationDisplayRowAttribute is a typed row-local dimension carried from
// source-inventory observation, for example a language package/module scope.
// It is intentionally separate from prose notes so finalizer prompts and
// validators can preserve requested dimensions without inferring them from
// member labels or file paths.
type EnumerationDisplayRowAttribute struct {
	Role     AnswerCandidateRole
	Name     string
	Source   string
	Line     int
	Location string
}

// ShouldCompileEnumerationDisplaySetsForRequest reports whether accepted
// aggregate member rows may become deterministic visible principal rows for
// this request shape.
//
// The compiler is answer-shape load-bearing: it can add facets, citations, and
// system supplement rows. Most established principal row lanes remain
// admissible here, including VCS/resource/runtime rows. The explicit exclusion
// is diagnostic current-source mechanism explanation: those turns often carry
// member_set aggregates as support ledgers for mechanisms, but unless a typed
// source-operation/exhaustive/relation/change-impact lane exists, the ledger
// must not rewrite or harden the visible answer surface.
func ShouldCompileEnumerationDisplaySetsForRequest(rm RequestModel) bool {
	if enumerationDisplayConflictsWithDiagnosticCurrentSourceMechanism(rm) {
		return false
	}
	return true
}

func enumerationDisplayConflictsWithDiagnosticCurrentSourceMechanism(rm RequestModel) bool {
	if aggregateRequestRequiresPathMemberSetAsPrincipal(rm) {
		return false
	}
	if !enumerationDisplayHasCurrentSourceDiagnosticShape(rm) {
		return false
	}
	return rm.Predicates.IsDiagnosticQuestion ||
		rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() ||
		rm.Intent == IntentRootCause ||
		rm.Scenario == ScenarioRootCause ||
		rm.Scenario == ScenarioPerformanceBottleneck
}

func enumerationDisplayHasCurrentSourceDiagnosticShape(rm RequestModel) bool {
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	return rm.DiagnosticProfile.CurrentVersionCheck
}

// CompileEnumerationDisplaySets compiles accepted complete principal
// aggregate facts into deterministic rows. It is intentionally conservative:
// diagnostic current-source support ledgers are skipped, complete member
// carriers are compiled, and per-row evidence enriches an existing member
// rather than adding or removing members.
func CompileEnumerationDisplaySets(rm *RequestModel, plan *AnswerSurfacePlan) []EnumerationDisplaySet {
	if plan == nil || len(plan.StableAggregateFacts) == 0 {
		return nil
	}
	if rm == nil || !ShouldCompileEnumerationDisplaySetsForRequest(*rm) {
		return nil
	}
	stableFacts := NormalizeAnswerAggregateMemberSetSurfaces(plan.StableAggregateFacts)
	refs := PrincipalAggregateMemberSetFactRefsForRequest(stableFacts, rm)
	if len(refs) == 0 {
		return nil
	}
	refs = dedupeEnumerationDisplayFactRefs(refs)
	support := genericAggregateSupportEvidenceByMemberLabel(plan.SurfaceEvidence)
	anchorSummaries := enumerationDisplayAnchorSummaryIndexForEvidence(plan.SurfaceEvidence)
	stepSupport := enumerationDisplayStepBackboneIndex(plan.StepBackbone)
	sourceInventoryAttributes := sourceInventoryRowAttributeIndex(plan.SourceInventoryObservation)
	out := make([]EnumerationDisplaySet, 0, len(refs))
	for _, ref := range refs {
		fact := ref.Fact
		if !AnswerAggregateFactCarriesCompleteMemberSet(fact) || len(fact.Members) == 0 {
			continue
		}
		set := EnumerationDisplaySet{
			ID:              enumerationDisplaySetID(ref.Index, fact),
			FactIndex:       ref.Index,
			Label:           strings.TrimSpace(fact.Label),
			Value:           strings.TrimSpace(fact.Value),
			Unit:            strings.TrimSpace(fact.Unit),
			Role:            AnswerAggregateFactRoleForRequest(fact, rm),
			EvidenceOrigins: cloneEnumerationDisplayOrigins(AnswerAggregateFactEvidenceOrigins(fact, rm)),
		}
		for memberIdx, member := range fact.Members {
			row, ok := compileEnumerationDisplayRow(rm, set, fact, ref.Index, memberIdx, member, support, anchorSummaries, stepSupport, sourceInventoryAttributes)
			if !ok {
				continue
			}
			set.Rows = append(set.Rows, row)
		}
		if len(set.Rows) > 0 {
			set.Rows = dedupeEnumerationDisplayRowIDs(set.Rows)
			out = append(out, set)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dedupeEnumerationDisplayFactRefs(refs []AnswerAggregateFactRef) []AnswerAggregateFactRef {
	if len(refs) < 2 {
		return refs
	}
	out := make([]AnswerAggregateFactRef, 0, len(refs))
	byMembers := map[string]int{}
	for _, ref := range refs {
		key := enumerationDisplayFactMemberSetKey(ref.Fact)
		if key == "" {
			out = append(out, ref)
			continue
		}
		if idx, ok := byMembers[key]; ok {
			if enumerationDisplayFactRefPreferred(ref, out[idx]) {
				out[idx] = ref
			}
			continue
		}
		byMembers[key] = len(out)
		out = append(out, ref)
	}
	return out
}

func enumerationDisplayFactMemberSetKey(fact AnswerAggregateFact) string {
	if !AnswerAggregateFactCarriesCompleteMemberSet(fact) || len(fact.Members) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fact.Members))
	for _, member := range fact.Members {
		key := AnswerAggregateMemberSurfaceKey(member)
		if key == "" {
			return ""
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

func enumerationDisplayFactRefPreferred(candidate, existing AnswerAggregateFactRef) bool {
	candidateKind := candidate.Fact.Kind == AnswerAggregateMemberSet
	existingKind := existing.Fact.Kind == AnswerAggregateMemberSet
	if candidateKind != existingKind {
		return candidateKind
	}
	if candidate.Role != existing.Role {
		return AnswerAggregateRolePriority(candidate.Role) > AnswerAggregateRolePriority(existing.Role)
	}
	if len(candidate.Fact.SupportRefs) != len(existing.Fact.SupportRefs) {
		return len(candidate.Fact.SupportRefs) > len(existing.Fact.SupportRefs)
	}
	if candidate.Fact.Provenance != "" && existing.Fact.Provenance == "" {
		return true
	}
	return false
}

func compileEnumerationDisplayRow(
	rm *RequestModel,
	set EnumerationDisplaySet,
	fact AnswerAggregateFact,
	factIdx int,
	memberIdx int,
	member string,
	support *aggregateMemberEvidenceIndex,
	anchorSummaries *enumerationDisplayAnchorSummaryIndex,
	stepSupport *enumerationDisplayStepBackbone,
	sourceInventoryAttributes *enumerationDisplaySourceInventoryAttributeIndex,
) (EnumerationDisplayRow, bool) {
	entry, ok := genericAggregateMemberSupportEntry(fact, factIdx, memberIdx, member, support, set.EvidenceOrigins)
	if !ok {
		return EnumerationDisplayRow{}, false
	}
	member = strings.TrimSpace(member)
	display := enumerationDisplayLabel(member)
	if display == "" {
		display = strings.TrimSpace(entry.Text)
	}
	note := ""
	if ev, found := enumerationDisplayEvidenceForEntry(member, entry, support); found {
		note = enumerationDisplayEvidenceNote(ev)
	}
	location := strings.TrimSpace(entry.Location)
	row := EnumerationDisplayRow{
		RowID:               enumerationDisplayRowID(set.ID, memberIdx, display, member),
		SetID:               set.ID,
		SetLabel:            set.Label,
		FactIndex:           factIdx,
		MemberIndex:         memberIdx,
		Member:              member,
		DisplayLabel:        display,
		Category:            enumerationDisplayCategory(fact),
		Source:              strings.TrimSpace(entry.Source),
		LineStart:           entry.LineStart,
		LineEnd:             entry.LineEnd,
		Location:            location,
		CitationKey:         location,
		HasCitation:         location != "" && entry.LineStart > 0,
		EvidenceID:          strings.TrimSpace(entry.EvidenceID),
		ClaimForm:           entry.ClaimForm,
		LabelSurface:        entry.LabelSurface,
		Subject:             entry.Subject,
		Object:              entry.Object,
		AnchorSymbol:        entry.AnchorSymbol,
		OwnerSymbol:         entry.OwnerSymbol,
		MemberSurface:       entry.MemberSurface,
		AnchorKind:          entry.AnchorKind,
		SurfaceTerms:        append([]string(nil), entry.SurfaceTerms...),
		EquivalentLocations: append([]string(nil), entry.EquivalentLocations...),
		Producer:            entry.Producer,
		GroundingTier:       entry.GroundingTier,
		EvidenceOrigins:     cloneEnumerationDisplayOrigins(set.EvidenceOrigins),
		Attributes: mergeEnumerationDisplayRowAttributes(
			sourceInventoryAttributes.attributesFor(member, entry),
			enumerationDisplayAggregateMemberAttributes(member, answerAggregateMemberNoteAt(fact.MemberNotes, memberIdx), entry),
		),
		Note:   note,
		Detail: enumerationDisplayDetail(entry.Detail, note),
	}
	if anchorNote := enumerationDisplayAnchorSummaryNoteForRow(row, anchorSummaries); anchorNote != "" {
		row.Note = MergeEvidenceSummaries(row.Note, anchorNote)
		row.Detail = enumerationDisplayDetail(entry.Detail, row.Note)
	}
	if stepNote := enumerationDisplayStepBackboneNoteForRow(row, stepSupport); stepNote != "" {
		row.Note = MergeEvidenceSummaries(row.Note, stepNote)
		row.Detail = enumerationDisplayDetail(entry.Detail, row.Note)
	}
	if memberNote := answerAggregateMemberNoteAt(fact.MemberNotes, memberIdx); memberNote != "" {
		row.Note = MergeEvidenceSummaries(row.Note, memberNote)
		row.Detail = enumerationDisplayDetail(entry.Detail, row.Note)
	}
	row.Note = SanitizeSourceInventoryNoteForRequest(rm, row.Note)
	row.Detail = enumerationDisplayDetail(entry.Detail, row.Note)
	return row, true
}

func enumerationDisplayAggregateMemberAttributes(member string, memberNote string, entry AnswerSupportEntry) []EnumerationDisplayRowAttribute {
	_, qualifier, ok := AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		qualifier = strings.TrimSpace(memberNote)
	}
	role, name, ok := aggregateMemberAttributeQualifierParts(qualifier)
	if !ok {
		return nil
	}
	attr := EnumerationDisplayRowAttribute{
		Role:   role,
		Name:   name,
		Source: strings.TrimSpace(entry.Source),
		Line:   entry.LineStart,
	}
	if attr.Source != "" {
		attr.Location = aggregateSupportLocationKeyForDisplay(attr.Source, attr.Line)
	}
	return []EnumerationDisplayRowAttribute{attr}
}

func dedupeEnumerationDisplayRowIDs(rows []EnumerationDisplayRow) []EnumerationDisplayRow {
	if len(rows) < 2 {
		return rows
	}
	counts := map[string]int{}
	for _, row := range rows {
		id := strings.TrimSpace(row.RowID)
		if id == "" {
			continue
		}
		counts[id]++
	}
	out := rows
	seen := map[string]int{}
	for i := range out {
		id := strings.TrimSpace(out[i].RowID)
		if id == "" || counts[id] <= 1 {
			continue
		}
		seen[id]++
		suffix := sanitizeEnumerationDisplayID(out[i].Location)
		if suffix == "" {
			suffix = strconv.Itoa(seen[id])
		}
		out[i].RowID = id + "-" + suffix
	}
	return out
}

type enumerationDisplaySourceInventoryAttributeIndex struct {
	byLocation map[string][]EnumerationDisplayRowAttribute
	byMember   map[string][]EnumerationDisplayRowAttribute
}

func sourceInventoryRowAttributeIndex(observation SourceInventoryObservation) *enumerationDisplaySourceInventoryAttributeIndex {
	if !observation.IsActive() {
		return nil
	}
	idx := &enumerationDisplaySourceInventoryAttributeIndex{
		byLocation: map[string][]EnumerationDisplayRowAttribute{},
		byMember:   map[string][]EnumerationDisplayRowAttribute{},
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			attrs := sourceInventoryRowContextAttributes(member.Attributes)
			if len(attrs) == 0 {
				continue
			}
			idx.addSourceInventoryMember(member, attrs)
		}
	}
	if len(idx.byLocation) == 0 && len(idx.byMember) == 0 {
		return nil
	}
	return idx
}

func sourceInventoryRowContextAttributes(attrs []SourceInventoryObservationAttribute) []EnumerationDisplayRowAttribute {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]EnumerationDisplayRowAttribute, 0, len(attrs))
	seen := map[string]bool{}
	for _, attr := range attrs {
		role := attr.Role
		if role != AnswerCandidateRolePackage {
			continue
		}
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		item := EnumerationDisplayRowAttribute{
			Role:   role,
			Name:   name,
			Source: strings.TrimSpace(attr.File),
			Line:   attr.Line,
		}
		if item.Source != "" {
			item.Location = aggregateSupportLocationKeyForDisplay(item.Source, item.Line)
		}
		key := enumerationDisplayRowAttributeKey(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return string(out[i].Role) < string(out[j].Role)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (idx *enumerationDisplaySourceInventoryAttributeIndex) addSourceInventoryMember(member SourceInventoryObservationMember, attrs []EnumerationDisplayRowAttribute) {
	if idx == nil || len(attrs) == 0 {
		return
	}
	source := strings.TrimSpace(member.File)
	line := member.Line
	if key := enumerationDisplayLocationKey(source, line); key != "" {
		idx.byLocation[key] = mergeEnumerationDisplayRowAttributes(idx.byLocation[key], attrs)
	}
	if label := aggregateMemberKey(member.Name); label != "" {
		if source != "" {
			idx.byMember[label+"\x00"+normalizeAnswerSupportPath(source)] = mergeEnumerationDisplayRowAttributes(idx.byMember[label+"\x00"+normalizeAnswerSupportPath(source)], attrs)
		}
		idx.byMember[label] = mergeEnumerationDisplayRowAttributes(idx.byMember[label], attrs)
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(member.SupportRef); ok {
		if key := enumerationDisplayLocationKey(loc.File, loc.LineStart); key != "" {
			idx.byLocation[key] = mergeEnumerationDisplayRowAttributes(idx.byLocation[key], attrs)
		}
	}
}

func (idx *enumerationDisplaySourceInventoryAttributeIndex) attributesFor(member string, entry AnswerSupportEntry) []EnumerationDisplayRowAttribute {
	if idx == nil {
		return nil
	}
	var out []EnumerationDisplayRowAttribute
	if key := enumerationDisplayLocationKey(entry.Source, entry.LineStart); key != "" {
		out = mergeEnumerationDisplayRowAttributes(out, idx.byLocation[key])
	}
	if loc, ok := ParseAnswerSourceLocationSurface(entry.Location); ok {
		if key := enumerationDisplayLocationKey(loc.File, loc.LineStart); key != "" {
			out = mergeEnumerationDisplayRowAttributes(out, idx.byLocation[key])
		}
	}
	if label := aggregateMemberKey(member); label != "" {
		if entry.Source != "" {
			out = mergeEnumerationDisplayRowAttributes(out, idx.byMember[label+"\x00"+normalizeAnswerSupportPath(entry.Source)])
		}
		if entry.Source == "" && len(out) == 0 {
			out = mergeEnumerationDisplayRowAttributes(out, idx.byMember[label])
		}
	}
	if label := aggregateMemberKey(entry.Text); label != "" && !strings.EqualFold(label, aggregateMemberKey(member)) {
		if entry.Source != "" {
			out = mergeEnumerationDisplayRowAttributes(out, idx.byMember[label+"\x00"+normalizeAnswerSupportPath(entry.Source)])
		}
		if entry.Source == "" && len(out) == 0 {
			out = mergeEnumerationDisplayRowAttributes(out, idx.byMember[label])
		}
	}
	return out
}

func mergeEnumerationDisplayRowAttributes(existing, incoming []EnumerationDisplayRowAttribute) []EnumerationDisplayRowAttribute {
	if len(incoming) == 0 {
		return existing
	}
	seen := map[string]bool{}
	out := make([]EnumerationDisplayRowAttribute, 0, len(existing)+len(incoming))
	for _, attr := range existing {
		key := enumerationDisplayRowAttributeKey(attr)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attr)
	}
	for _, attr := range incoming {
		key := enumerationDisplayRowAttributeKey(attr)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attr)
	}
	return out
}

func enumerationDisplayRowAttributeKey(attr EnumerationDisplayRowAttribute) string {
	name := strings.ToLower(strings.TrimSpace(attr.Name))
	if name == "" {
		return ""
	}
	return string(attr.Role) + "\x00" + name
}

func enumerationDisplayLocationKey(source string, line int) string {
	source = normalizeAnswerSupportPath(source)
	if source == "" || line <= 0 {
		return ""
	}
	return source + ":" + strconv.Itoa(line)
}

func aggregateSupportLocationKeyForDisplay(file string, line int) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if line > 0 {
		return file + ":" + strconv.Itoa(line)
	}
	return file
}

func answerAggregateMemberNoteAt(notes []string, idx int) string {
	if idx < 0 || idx >= len(notes) {
		return ""
	}
	return strings.Join(strings.Fields(strings.TrimSpace(notes[idx])), " ")
}

func SanitizeSourceInventoryNoteForRequest(rm *RequestModel, note string) string {
	note = strings.Join(strings.Fields(strings.TrimSpace(note)), " ")
	if note == "" || rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return note
	}
	profile := rm.SourceInventoryProfile
	if !profile.RequestsField(SourceInventoryFieldValues) {
		note = stripEnumerationValueExamplesFromNote(note)
		note = stripEnumerationValueClausesFromNote(note)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rm.Language)), "zh") {
			note = localizeEnumerationVisibilityNote(note)
		}
	}
	return strings.Join(strings.Fields(strings.TrimSpace(note)), " ")
}

func stripEnumerationValueExamplesFromNote(note string) string {
	runes := []rune(note)
	if len(runes) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		open := runes[i]
		close := rune(0)
		switch open {
		case '(':
			close = ')'
		case '（':
			close = '）'
		default:
			b.WriteRune(open)
			continue
		}
		end := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == close {
				end = j
				break
			}
		}
		if end > i {
			body := string(runes[i+1 : end])
			if enumerationValueExampleListLike(body) {
				i = end
				continue
			}
		}
		b.WriteRune(open)
	}
	return strings.TrimSpace(b.String())
}

func stripEnumerationValueClausesFromNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	clauses := splitEnumerationNoteClauses(note)
	if len(clauses) <= 1 {
		if enumerationValueClauseLike(note) {
			return ""
		}
		return note
	}
	kept := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" || enumerationValueClauseLike(clause) {
			continue
		}
		kept = append(kept, clause)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(kept, "，"))
}

func splitEnumerationNoteClauses(note string) []string {
	return strings.FieldsFunc(note, func(r rune) bool {
		switch r {
		case '，', '。', '；', ';':
			return true
		default:
			return false
		}
	})
}

func enumerationValueClauseLike(clause string) bool {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return false
	}
	lower := strings.ToLower(clause)
	hasTrigger := strings.Contains(lower, "const") ||
		strings.Contains(lower, "value") ||
		strings.Contains(lower, "literal") ||
		strings.Contains(clause, "常量") ||
		strings.Contains(clause, "取值") ||
		strings.Contains(clause, "枚举值") ||
		strings.Contains(clause, "成员值") ||
		strings.Contains(clause, "定义了") ||
		strings.Contains(clause, "包含")
	if !hasTrigger || !strings.ContainsAny(clause, "/,，、|") {
		return false
	}
	return enumerationValueCodeTokenCount(clause) >= 2
}

func enumerationValueCodeTokenCount(text string) int {
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '/', ',', '，', '、', '|', ' ', '\t', '\n', '\r', '`', '\'', '"', '“', '”', '‘', '’', '(', ')', '（', '）':
			return true
		default:
			return false
		}
	})
	count := 0
	for _, token := range tokens {
		token = strings.Trim(token, "`'\"“”‘’…:：;；.。")
		if token == "" {
			continue
		}
		if enumerationValueExampleTokenLike(token) {
			count++
		}
	}
	return count
}

func enumerationValueExampleListLike(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	hasListSeparator := strings.ContainsAny(body, "/,，、|")
	if !hasListSeparator {
		return false
	}
	for _, r := range body {
		if unicode.Is(unicode.Han, r) {
			return false
		}
	}
	tokens := strings.FieldsFunc(body, func(r rune) bool {
		switch r {
		case '/', ',', '，', '、', '|', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	codeTokens := 0
	for _, token := range tokens {
		token = strings.Trim(token, "`'\"…")
		if token == "" {
			continue
		}
		if enumerationValueExampleTokenLike(token) {
			codeTokens++
		}
	}
	return codeTokens >= 2
}

func enumerationValueExampleTokenLike(token string) bool {
	if token == "" {
		return false
	}
	hasAlphaNum := false
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			hasAlphaNum = true
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return hasAlphaNum
}

func localizeEnumerationVisibilityNote(note string) string {
	replacer := strings.NewReplacer(
		"private/internal", "非公开/内部",
		"private / internal", "非公开/内部",
		"private or internal", "非公开或内部",
		"private/internal", "非公开/内部",
		"private", "非公开",
	)
	return replacer.Replace(note)
}

func cloneEnumerationDisplayOrigins(in []AnswerEvidenceOrigin) []AnswerEvidenceOrigin {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerEvidenceOrigin, len(in))
	copy(out, in)
	return out
}

type enumerationDisplayAnchorSummaryIndex struct {
	byKey map[string]string
}

func enumerationDisplayAnchorSummaryIndexForEvidence(items []EvidenceItem) *enumerationDisplayAnchorSummaryIndex {
	if len(items) == 0 {
		return nil
	}
	out := &enumerationDisplayAnchorSummaryIndex{byKey: map[string]string{}}
	for _, item := range items {
		if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 || item.GroundingStatus == GroundingUngrounded {
			continue
		}
		note := enumerationDisplayEvidenceNote(item)
		if note == "" {
			continue
		}
		for _, key := range enumerationDisplayEvidenceAnchorSummaryKeys(item) {
			out.byKey[key] = MergeEvidenceSummaries(out.byKey[key], note)
		}
	}
	if len(out.byKey) == 0 {
		return nil
	}
	return out
}

func enumerationDisplayEvidenceAnchorSummaryKeys(item EvidenceItem) []string {
	loc := enumerationDisplayAnchorLocationKey(item.Source, item.LineStart)
	if loc == "" {
		return nil
	}
	var out []string
	for _, raw := range aggregateEvidenceMemberLabels(item) {
		for _, candidate := range AnswerAggregateMemberDisplayCandidates(raw) {
			if key := aggregateMemberKey(candidate); key != "" {
				out = append(out, loc+"\x00"+key)
			}
		}
		if key := aggregateMemberKey(raw); key != "" {
			out = append(out, loc+"\x00"+key)
		}
	}
	return dedupeAggregateMemberTerms(out)
}

func enumerationDisplayAnchorSummaryNoteForRow(row EnumerationDisplayRow, index *enumerationDisplayAnchorSummaryIndex) string {
	if index == nil || len(index.byKey) == 0 {
		return ""
	}
	var note string
	for _, loc := range enumerationDisplayRowLocationKeys(row) {
		for _, identity := range enumerationDisplayRowMatchKeys(row) {
			if identity == "" {
				continue
			}
			note = MergeEvidenceSummaries(note, index.byKey[loc+"\x00"+identity])
		}
	}
	return note
}

func enumerationDisplayRowLocationKeys(row EnumerationDisplayRow) []string {
	var out []string
	addLocation := func(raw string) {
		if surface, ok := ParseAnswerSourceLocationSurface(raw); ok {
			if key := enumerationDisplayAnchorLocationKey(surface.File, surface.LineStart); key != "" {
				out = append(out, key)
			}
		}
	}
	if key := enumerationDisplayAnchorLocationKey(row.Source, row.LineStart); key != "" {
		out = append(out, key)
	}
	addLocation(row.Location)
	for _, loc := range row.EquivalentLocations {
		addLocation(loc)
	}
	return dedupeAggregateMemberTerms(out)
}

func enumerationDisplayAnchorLocationKey(source string, line int) string {
	source = normalizeAnswerSupportPath(source)
	if source == "" || line <= 0 {
		return ""
	}
	return source + ":" + strconv.Itoa(line)
}

type enumerationDisplayStepBackbone struct {
	byKey map[string][]StepSurfaceAnchor
}

func enumerationDisplayStepBackboneIndex(anchors []StepSurfaceAnchor) *enumerationDisplayStepBackbone {
	if len(anchors) == 0 {
		return nil
	}
	out := &enumerationDisplayStepBackbone{byKey: map[string][]StepSurfaceAnchor{}}
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.Name) == "" {
			continue
		}
		for _, key := range enumerationDisplayStepAnchorKeys(anchor) {
			out.byKey[key] = append(out.byKey[key], anchor)
		}
	}
	if len(out.byKey) == 0 {
		return nil
	}
	return out
}

func enumerationDisplayStepAnchorKeys(anchor StepSurfaceAnchor) []string {
	var out []string
	for _, raw := range []string{anchor.Name} {
		if key := aggregateMemberKey(raw); key != "" {
			out = append(out, key)
		}
	}
	return dedupeAggregateMemberTerms(out)
}

func enumerationDisplayStepBackboneNoteForRow(row EnumerationDisplayRow, index *enumerationDisplayStepBackbone) string {
	if index == nil || len(index.byKey) == 0 {
		return ""
	}
	keys := enumerationDisplayRowMatchKeys(row)
	for _, key := range keys {
		for _, anchor := range index.byKey[key] {
			if !enumerationDisplayStepAnchorLocationCompatible(row, anchor) {
				continue
			}
			if note := enumerationDisplayStepAnchorNote(anchor); note != "" {
				return note
			}
		}
	}
	return ""
}

func enumerationDisplayRowMatchKeys(row EnumerationDisplayRow) []string {
	var out []string
	for _, raw := range []string{
		row.DisplayLabel,
		row.Member,
		row.AnchorSymbol,
		row.Subject,
		row.Object,
	} {
		for _, candidate := range AnswerAggregateMemberDisplayCandidates(raw) {
			if key := aggregateMemberKey(candidate); key != "" {
				out = append(out, key)
			}
		}
		if key := aggregateMemberKey(raw); key != "" {
			out = append(out, key)
		}
	}
	return dedupeAggregateMemberTerms(out)
}

func enumerationDisplayStepAnchorLocationCompatible(row EnumerationDisplayRow, anchor StepSurfaceAnchor) bool {
	file := normalizeAnswerSupportPath(anchor.File)
	if file == "" || anchor.Line <= 0 || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
		return true
	}
	rowFile := normalizeAnswerSupportPath(row.Source)
	if file != rowFile && !strings.HasSuffix(rowFile, "/"+file) && !strings.HasSuffix(file, "/"+rowFile) {
		return false
	}
	if row.LineEnd > 0 {
		return anchor.Line >= row.LineStart && anchor.Line <= row.LineEnd
	}
	return anchor.Line == row.LineStart
}

func enumerationDisplayStepAnchorNote(anchor StepSurfaceAnchor) string {
	for _, raw := range []string{anchor.SurfaceText, anchor.Rationale, anchor.Chain} {
		note := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
		if note != "" {
			return note
		}
	}
	return ""
}

func enumerationDisplaySetID(index int, fact AnswerAggregateFact) string {
	base := "enum-set"
	if label := sanitizeEnumerationDisplayID(fact.Label); label != "" {
		base += "-" + label
	} else {
		base += "-" + strconv.Itoa(index+1)
	}
	return base
}

func enumerationDisplayRowID(setID string, idx int, display string, member string) string {
	base := strings.TrimSpace(setID)
	if base == "" {
		base = "enum-set"
	}
	suffix := sanitizeEnumerationDisplayID(display)
	if suffix == "" {
		suffix = sanitizeEnumerationDisplayID(member)
	}
	if suffix == "" {
		suffix = strconv.Itoa(idx + 1)
	}
	return base + "-row-" + suffix
}

func sanitizeEnumerationDisplayID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 80 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func enumerationDisplayLabel(member string) string {
	member = strings.TrimSpace(member)
	if member == "" {
		return ""
	}
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	if surface, ok := ParseAnswerSourceLocationSurface(member); ok {
		return aggregateMemberStartLocation(surface)
	}
	candidates := aggregateMemberDisplayCandidates(member)
	if len(candidates) > 0 && strings.TrimSpace(candidates[0]) != "" {
		return strings.TrimSpace(candidates[0])
	}
	return member
}

func enumerationDisplayCategory(fact AnswerAggregateFact) string {
	if unit := strings.TrimSpace(fact.Unit); unit != "" {
		return unit
	}
	if label := strings.TrimSpace(fact.Label); label != "" {
		return label
	}
	return string(fact.Kind)
}

func enumerationDisplayEvidenceForEntry(member string, entry AnswerSupportEntry, support *aggregateMemberEvidenceIndex) (EvidenceItem, bool) {
	if support == nil || len(support.items) == 0 {
		return EvidenceItem{}, false
	}
	var best EvidenceItem
	bestScore := -1
	consider := func(ev EvidenceItem) {
		score := enumerationDisplayEvidenceCandidateScore(member, entry, ev)
		if score <= bestScore {
			return
		}
		best = ev
		bestScore = score
	}
	if ev, ok := aggregateMemberEvidenceByLabel(member, support); ok {
		consider(ev)
	}
	source := normalizeAnswerSupportPath(entry.Source)
	line := entry.LineStart
	if source == "" || line <= 0 {
		if bestScore > 0 {
			return best, true
		}
		return EvidenceItem{}, false
	}
	for i := range support.items {
		ev := support.items[i]
		if normalizeAnswerSupportPath(ev.Source) != source || ev.LineStart != line {
			continue
		}
		consider(ev)
	}
	if bestScore > 0 {
		return best, true
	}
	return EvidenceItem{}, false
}

func enumerationDisplayEvidenceCompatibleWithEntry(ev EvidenceItem, entry AnswerSupportEntry) bool {
	if strings.TrimSpace(entry.Source) == "" || entry.LineStart <= 0 {
		return true
	}
	return normalizeAnswerSupportPath(ev.Source) == normalizeAnswerSupportPath(entry.Source) &&
		ev.LineStart == entry.LineStart
}

func enumerationDisplayEvidenceCandidateScore(member string, entry AnswerSupportEntry, ev EvidenceItem) int {
	if strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 || ev.GroundingStatus == GroundingUngrounded {
		return -1
	}
	compatible := enumerationDisplayEvidenceCompatibleWithEntry(ev, entry)
	supportsMember := aggregateEvidenceTextSupportsMember(ev, member)
	if !compatible && !supportsMember && strings.TrimSpace(entry.Location) != "" {
		return -1
	}
	score := 0
	if compatible {
		score += 1000
	}
	if supportsMember {
		score += 300
	}
	if strings.TrimSpace(ev.AnchorSymbol) != "" && strings.EqualFold(strings.TrimSpace(ev.AnchorSymbol), strings.TrimSpace(enumerationDisplayLabel(member))) {
		score += 180
	}
	if form := ClaimFormOf(ev); form == ClaimDefinitionFact {
		score += 120
	} else if form != ClaimUnknown {
		score += 40
	}
	switch ev.GroundingStatus {
	case GroundingGrounded:
		score += 80
	case GroundingRecovered:
		score += 40
	}
	note := enumerationDisplayEvidenceNote(ev)
	if note != "" {
		score += minEnumerationDisplayInt(120, len([]rune(note))/2)
	}
	if ev.LoadBearingSummary {
		score += 20
	}
	return score
}

func minEnumerationDisplayInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func enumerationDisplayEvidenceNote(ev EvidenceItem) string {
	summary := strings.Join(strings.Fields(strings.TrimSpace(ev.Summary)), " ")
	if summary != "" {
		return summary
	}
	surface := strings.Join(strings.Fields(strings.TrimSpace(ev.Snippet)), " ")
	if surface == "" {
		surface = strings.Join(strings.Fields(strings.TrimSpace(EvidenceDeterministicSurfaceText(ev, false))), " ")
	}
	if surface != "" {
		return surface
	}
	return strings.TrimSpace(EvidenceAuthoritativeSurfaceText(ev, false))
}

func enumerationDisplayDetail(detail string, note string) string {
	detail = strings.TrimSpace(detail)
	note = strings.TrimSpace(note)
	if note == "" {
		return detail
	}
	if detail == "" {
		return note
	}
	if strings.Contains(detail, note) {
		return detail
	}
	return note + "; " + detail
}

func enumerationDisplayRowSupportEntry(row EnumerationDisplayRow) AnswerSupportEntry {
	return AnswerSupportEntry{
		Text:                row.Member,
		Detail:              row.Detail,
		Location:            row.Location,
		EvidenceID:          row.EvidenceID,
		ClaimForm:           row.ClaimForm,
		LabelSurface:        row.LabelSurface,
		Subject:             row.Subject,
		Object:              row.Object,
		AnchorSymbol:        row.AnchorSymbol,
		OwnerSymbol:         row.OwnerSymbol,
		SurfaceTerms:        append([]string(nil), row.SurfaceTerms...),
		EquivalentLocations: append([]string(nil), row.EquivalentLocations...),
		Source:              row.Source,
		LineStart:           row.LineStart,
		LineEnd:             row.LineEnd,
		AnchorKind:          row.AnchorKind,
		MemberSurface:       row.MemberSurface,
		Producer:            row.Producer,
		GroundingTier:       row.GroundingTier,
	}
}
