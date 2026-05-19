package types

import (
	"fmt"
	"strconv"
	"strings"
)

const definitionSupportMemberCitationWindow = 5

// AnswerSupportMemberObligation is a principal support-lane member
// that must survive into an enumeration answer's principal list/table
// surface. It is derived from typed support entries, not from raw
// request text, so the finalizer cannot silently drop answer-grade
// evidence that the explorer already structured.
type AnswerSupportMemberObligation struct {
	EvidenceID   string
	Location     string
	Label        string
	ClaimForm    ClaimForm
	SurfaceTerms []string
	Source       string
	LineStart    int
	LineEnd      int

	// EquivalentLocations holds other file:line anchors that describe
	// the same typed principal member. Definition facts commonly have
	// both declaration-line and field/body-line evidence, especially
	// across Go, C/C++, ArkTS, Cangjie, and generated bindings. These
	// anchors are equivalent for member-preservation purposes; keeping
	// them typed prevents the finalizer from bouncing between adjacent
	// valid citations.
	EquivalentLocations []string
}

// ChangeImpactNarrowingDiagnostic reports a typed impact-answer failure:
// the support lane contains non-assignment affected-site evidence, but the
// rendered principal answer only carries assignment-shaped members. The
// diagnostic is derived from support-lane claim forms and citation coverage,
// not from raw wording in the user's question or the answer prose.
type ChangeImpactNarrowingDiagnostic struct {
	Target         string
	CoveredForms   []ClaimForm
	MissingForms   []ClaimForm
	MissingMembers []AnswerSupportMemberObligation
}

// ChangeImpactFileOutputLabelDiagnostic reports a typed rendering failure for
// change-impact questions whose requested principal surface is files: the
// answer cites the right support member but renders the principal item label as
// a site (`file:line`) or another non-file surface. The validator is driven by
// ChangeImpactProfile + support-lane obligations + citation refs, not by raw
// request wording.
type ChangeImpactFileOutputLabelDiagnostic struct {
	ExpectedCount int
	MissingLabels []AnswerSupportMemberObligation
}

// LocationHint returns the citation shape a repair prompt should ask
// for. It deliberately exposes equivalent definition anchors instead
// of pretending there is only one exact line when the typed evidence
// says several adjacent lines describe the same member.
func (ob AnswerSupportMemberObligation) LocationHint() string {
	locations := supportMemberCoverageLocations(ob)
	if len(locations) == 0 {
		return strings.TrimSpace(ob.Location)
	}
	if len(locations) == 1 {
		return locations[0]
	}
	const maxShown = 4
	if len(locations) <= maxShown {
		return "one of " + strings.Join(locations, ", ")
	}
	return fmt.Sprintf("one of %s, ... (%d equivalent typed anchors)", strings.Join(locations[:maxShown], ", "), len(locations))
}

// StableItemID is a deterministic item-id suggestion for the finalizer's
// principal list/table row. It is a handoff aid, not an answer fact: the
// model still owns the visible prose, while validators and normalizers can
// share one stable id surface instead of re-deriving different ids from the
// same member.
func (ob AnswerSupportMemberObligation) StableItemID() string {
	label := strings.TrimSpace(ob.Label)
	if label == "" && len(ob.SurfaceTerms) > 0 {
		label = strings.TrimSpace(ob.SurfaceTerms[0])
	}
	if label == "" {
		label = strings.TrimSpace(ob.Location)
	}
	base := sanitizeAnswerSupportStableID(label)
	if base == "" {
		base = "member"
	}
	return "support-" + base
}

// StableCitationKey returns the preferred file:line key for the member.
// When several equivalent anchors exist, the key is the first typed
// coverage location; LocationHint() remains the user-facing "one of..."
// rendering for repair text.
func (ob AnswerSupportMemberObligation) StableCitationKey() string {
	locations := supportMemberCoverageLocations(ob)
	if len(locations) == 0 {
		return strings.TrimSpace(ob.Location)
	}
	return locations[0]
}

func sanitizeAnswerSupportStableID(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range label {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// PrincipalSupportMemberObligations returns the answer-grade
// principal evidence entries whose locations should appear as
// cited list/table items in QFEnumeration answers when the support plan
// explicitly treats the lane as a member slate. Other families, and
// enumeration-shaped fallbacks with enrichment-only policy, can use
// principal support lanes for grounded context; forcing all entries there
// to render as one item each would be too strict.
func PrincipalSupportMemberObligations(plan *AnswerSupportPlan) []AnswerSupportMemberObligation {
	if plan == nil || plan.Family != QFEnumeration {
		return nil
	}
	if !plan.PrincipalMemberCoverage.RequiresMemberCoverage() {
		return nil
	}
	seen := make(map[string]int)
	var out []AnswerSupportMemberObligation
	fileLevel := plan.ChangeImpactProfile != nil &&
		plan.ChangeImpactProfile.Active() &&
		plan.ChangeImpactProfile.RequestedOutput == ImpactOutputFiles
	for _, lane := range plan.Lanes {
		if lane.Kind != SupportLanePrincipalEvidence {
			continue
		}
		for _, entry := range lane.Entries {
			if !principalSupportEntryRequiresMemberCoverage(entry) {
				continue
			}
			ob := principalSupportMemberObligation(entry)
			if fileLevel {
				ob = principalSupportFileMemberObligation(ob)
			}
			key := principalSupportMemberDedupKey(ob)
			if fileLevel {
				key = principalSupportFileMemberDedupKey(ob)
			}
			if key == "" {
				continue
			}
			if idx, ok := seen[key]; ok {
				mergePrincipalSupportMemberObligation(&out[idx], ob)
				continue
			}
			seen[key] = len(out)
			out = append(out, ob)
		}
	}
	return out
}

func principalSupportFileMemberObligation(ob AnswerSupportMemberObligation) AnswerSupportMemberObligation {
	source := strings.TrimSpace(ob.Source)
	if source == "" {
		return ob
	}
	source = normalizeAnswerSupportPath(source)
	if source == "" {
		return ob
	}
	if ob.Location != "" {
		appendEquivalentSupportMemberLocation(&ob, ob.Location)
	}
	ob.Label = source
	ob.Location = ""
	if ob.LineStart > 0 {
		ob.Location = normalizeAnswerSupportLocation(fmt.Sprintf("%s:%d", source, ob.LineStart))
	}
	if ob.Location == "" && len(ob.EquivalentLocations) > 0 {
		ob.Location = ob.EquivalentLocations[0]
		ob.EquivalentLocations = ob.EquivalentLocations[1:]
	}
	ob.SurfaceTerms = appendSupportMemberSurfaceTerm(ob.SurfaceTerms, source)
	return ob
}

func principalSupportFileMemberDedupKey(ob AnswerSupportMemberObligation) string {
	source := normalizeAnswerSupportPath(ob.Source)
	if source == "" {
		return ""
	}
	return "file\x00" + source
}

func appendSupportMemberSurfaceTerm(in []string, term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return in
	}
	key := strings.ToLower(term)
	for _, existing := range in {
		if strings.ToLower(strings.TrimSpace(existing)) == key {
			return in
		}
	}
	return append(in, term)
}

func principalSupportEntryRequiresMemberCoverage(entry AnswerSupportEntry) bool {
	if strings.TrimSpace(entry.Location) == "" {
		return false
	}
	switch entry.ClaimForm {
	case ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition,
		ClaimAssignmentFact, ClaimReturnFact, ClaimPrecedenceRole,
		ClaimExternalObservation, ClaimImportEdge:
		return true
	case ClaimUnknown:
		surface := principalMemberSurfaceForSupportEntry(entry)
		return surface == PrincipalMemberSurfaceSourceLocation ||
			(surface == PrincipalMemberSurfaceDisplayLabel &&
				supportEntryIsAggregateMemberSet(entry))
	case ClaimAbsenceFact:
		return false
	default:
		return false
	}
}

func supportEntryIsAggregateMemberSet(entry AnswerSupportEntry) bool {
	if strings.HasPrefix(strings.TrimSpace(entry.EvidenceID), "aggregate_fact:member_set:") {
		return true
	}
	return strings.TrimSpace(entry.Producer) == "explorer.emit_investigation_complete.aggregate_facts"
}

func principalSupportMemberObligation(entry AnswerSupportEntry) AnswerSupportMemberObligation {
	source, line := supportMemberSourceLine(entry.Source, entry.LineStart, entry.Location)
	ob := AnswerSupportMemberObligation{
		EvidenceID: strings.TrimSpace(entry.EvidenceID),
		Location:   normalizeAnswerSupportLocation(entry.Location),
		Label:      principalSupportMemberLabel(entry),
		ClaimForm:  entry.ClaimForm,
		Source:     source,
		LineStart:  line,
		LineEnd:    supportMemberLineEnd(line, entry.LineEnd),
	}
	for _, location := range entry.EquivalentLocations {
		appendEquivalentSupportMemberLocation(&ob, location)
	}
	seen := make(map[string]bool)
	addTerm := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		ob.SurfaceTerms = append(ob.SurfaceTerms, s)
	}
	for _, term := range entry.SurfaceTerms {
		addTerm(term)
	}
	addTerm(entry.Subject)
	addTerm(entry.Object)
	addTerm(entry.AnchorSymbol)
	addTerm(entry.OwnerSymbol)
	if principalMemberSurfaceForSupportEntry(entry) == PrincipalMemberSurfaceSourceLocation {
		if entry.ClaimForm == ClaimImportEdge {
			addTerm(source)
		} else {
			addTerm(normalizeAnswerSupportLocation(entry.Location))
		}
	}
	if ob.Label == "" && len(ob.SurfaceTerms) > 0 {
		ob.Label = ob.SurfaceTerms[0]
	}
	if ob.Label == "" {
		ob.Label = strings.TrimSpace(entry.Text)
	}
	return ob
}

func principalSupportMemberDedupKey(ob AnswerSupportMemberObligation) string {
	label := normalizedSupportMemberIdentity(ob.Label)
	form := string(ob.ClaimForm)
	if form == "" {
		form = string(ClaimUnknown)
	}
	if ob.ClaimForm == ClaimDefinitionFact && strings.TrimSpace(ob.Source) != "" && label != "" {
		return form + "\x00" + normalizeAnswerSupportPath(ob.Source) + "\x00" + label
	}
	location := strings.TrimSpace(ob.Location)
	if location == "" && label == "" {
		return ""
	}
	return form + "\x00" + location + "\x00" + label
}

func mergePrincipalSupportMemberObligation(dst *AnswerSupportMemberObligation, src AnswerSupportMemberObligation) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.EvidenceID) == "" {
		dst.EvidenceID = src.EvidenceID
	}
	if strings.TrimSpace(dst.Location) == "" {
		dst.Location = src.Location
	}
	for _, location := range supportMemberCoverageLocations(src) {
		appendEquivalentSupportMemberLocation(dst, location)
	}
	if strings.TrimSpace(dst.Label) == "" {
		dst.Label = src.Label
	}
	if strings.TrimSpace(dst.Source) == "" {
		dst.Source = src.Source
	}
	if dst.LineStart <= 0 {
		dst.LineStart = src.LineStart
	}
	if dst.LineEnd <= 0 || (src.LineEnd > 0 && src.LineEnd > dst.LineEnd) {
		dst.LineEnd = src.LineEnd
	}
	seen := make(map[string]bool, len(dst.SurfaceTerms)+len(src.SurfaceTerms))
	for _, term := range dst.SurfaceTerms {
		if key := strings.ToLower(strings.TrimSpace(term)); key != "" {
			seen[key] = true
		}
	}
	for _, term := range src.SurfaceTerms {
		term = strings.TrimSpace(term)
		key := strings.ToLower(term)
		if term == "" || seen[key] {
			continue
		}
		seen[key] = true
		dst.SurfaceTerms = append(dst.SurfaceTerms, term)
	}
}

func appendEquivalentSupportMemberLocation(ob *AnswerSupportMemberObligation, location string) {
	if ob == nil {
		return
	}
	location = normalizeAnswerSupportLocation(location)
	if location == "" || location == ob.Location {
		return
	}
	for _, existing := range ob.EquivalentLocations {
		if normalizeAnswerSupportLocation(existing) == location {
			return
		}
	}
	ob.EquivalentLocations = append(ob.EquivalentLocations, location)
}

func principalSupportMemberLabel(entry AnswerSupportEntry) string {
	if principalMemberSurfaceForSupportEntry(entry) == PrincipalMemberSurfaceSourceLocation {
		source, _ := supportMemberSourceLine(entry.Source, entry.LineStart, entry.Location)
		if entry.ClaimForm == ClaimImportEdge && strings.TrimSpace(source) != "" {
			return strings.TrimSpace(source)
		}
		if location := normalizeAnswerSupportLocation(entry.Location); location != "" {
			return location
		}
		if strings.TrimSpace(source) != "" {
			return strings.TrimSpace(source)
		}
	}
	if entry.ClaimForm == ClaimImportEdge && strings.TrimSpace(entry.Subject) != "" && strings.TrimSpace(entry.Object) != "" {
		return fmt.Sprintf("%s -> %s", strings.TrimSpace(entry.Subject), strings.TrimSpace(entry.Object))
	}
	for _, s := range []string{entry.AnchorSymbol, entry.Subject, entry.Object} {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	for _, s := range entry.SurfaceTerms {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func principalMemberSurfaceForSupportEntry(entry AnswerSupportEntry) AnswerPrincipalMemberSurface {
	if entry.MemberSurface != PrincipalMemberSurfaceUnknown {
		return entry.MemberSurface
	}
	if entry.ClaimForm.UsesNonSymbolLabelSurface() {
		return PrincipalMemberSurfaceDisplayLabel
	}
	if entry.ClaimForm.LabelSurfaceKind() == ClaimLabelSurfaceSymbolLike {
		return PrincipalMemberSurfaceSymbolLike
	}
	return PrincipalMemberSurfaceUnknown
}

// MissingPrincipalSupportMembers returns the member obligations that
// are not represented by any principal list/table row item with a
// citation to the same typed support-entry location.
func MissingPrincipalSupportMembers(doc *AnswerDocumentV2, plan *AnswerSupportPlan) []AnswerSupportMemberObligation {
	if doc == nil {
		return nil
	}
	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) == 0 {
		return nil
	}
	index := newAnswerSupportDocumentIndex(doc)
	return missingPrincipalSupportMembersIndexed(index, obligations)
}

func missingPrincipalSupportMembersIndexed(index *answerSupportDocumentIndex, obligations []AnswerSupportMemberObligation) []AnswerSupportMemberObligation {
	var out []AnswerSupportMemberObligation
	for _, ob := range obligations {
		if index.coversSupportMember(ob) {
			continue
		}
		out = append(out, ob)
	}
	return out
}

func ChangeImpactPrincipalNarrowing(doc *AnswerDocumentV2, plan *AnswerSupportPlan) *ChangeImpactNarrowingDiagnostic {
	if doc == nil || plan == nil || plan.ChangeImpactProfile == nil ||
		!plan.ChangeImpactProfile.RequestedBroadAffectedSites() {
		return nil
	}
	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) == 0 {
		return nil
	}
	index := newAnswerSupportDocumentIndex(doc)
	var supportHasNonAssignment bool
	var coveredForms []ClaimForm
	var missingForms []ClaimForm
	var missingMembers []AnswerSupportMemberObligation
	for _, ob := range obligations {
		if ob.ClaimForm != ClaimAssignmentFact {
			supportHasNonAssignment = true
		}
		if index.coversSupportMember(ob) {
			coveredForms = appendUniqueClaimForm(coveredForms, ob.ClaimForm)
			continue
		}
		if ob.ClaimForm != ClaimAssignmentFact {
			missingForms = appendUniqueClaimForm(missingForms, ob.ClaimForm)
			missingMembers = append(missingMembers, ob)
		}
	}
	if !supportHasNonAssignment || len(missingMembers) == 0 {
		return nil
	}
	for _, form := range coveredForms {
		if form != ClaimAssignmentFact && form != ClaimUnknown {
			return nil
		}
	}
	if len(coveredForms) == 0 {
		return nil
	}
	return &ChangeImpactNarrowingDiagnostic{
		Target:         strings.TrimSpace(plan.ChangeImpactProfile.Target),
		CoveredForms:   coveredForms,
		MissingForms:   missingForms,
		MissingMembers: missingMembers,
	}
}

func ChangeImpactFileOutputLabelDrift(doc *AnswerDocumentV2, plan *AnswerSupportPlan) *ChangeImpactFileOutputLabelDiagnostic {
	if doc == nil || plan == nil || plan.ChangeImpactProfile == nil ||
		!plan.ChangeImpactProfile.Active() ||
		plan.ChangeImpactProfile.RequestedOutput != ImpactOutputFiles {
		return nil
	}
	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) == 0 {
		return nil
	}
	index := newAnswerSupportDocumentIndex(doc)
	// Label-surface drift is a finalizer-local rewrite only when the
	// typed file members are otherwise present. If members are missing,
	// the broader coverage/narrowing gates should explain the upstream
	// principal-set failure first.
	if len(missingPrincipalSupportMembersIndexed(index, obligations)) > 0 {
		return nil
	}
	var missing []AnswerSupportMemberObligation
	for _, ob := range obligations {
		if strings.TrimSpace(ob.Source) == "" {
			continue
		}
		if index.labelsFileOutputMember(ob) {
			continue
		}
		if index.coversSupportMember(ob) {
			missing = append(missing, ob)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &ChangeImpactFileOutputLabelDiagnostic{
		ExpectedCount: len(obligations),
		MissingLabels: missing,
	}
}

func answerDocumentLabelsFileOutputMember(doc *AnswerDocumentV2, ob AnswerSupportMemberObligation) bool {
	return newAnswerSupportDocumentIndex(doc).labelsFileOutputMember(ob)
}

type answerSupportDocumentIndex struct {
	principalByLocation map[string][]answerSupportIndexedItem
	principalByFile     map[string][]answerSupportIndexedItem
	caveatsByLocation   map[string][]answerSupportIndexedItem
	caveatsByFile       map[string][]answerSupportIndexedItem
}

type answerSupportIndexedItem struct {
	item      AnswerBlockItem
	blockText string
	citation  Citation
}

func newAnswerSupportDocumentIndex(doc *AnswerDocumentV2) *answerSupportDocumentIndex {
	index := &answerSupportDocumentIndex{
		principalByLocation: make(map[string][]answerSupportIndexedItem),
		principalByFile:     make(map[string][]answerSupportIndexedItem),
		caveatsByLocation:   make(map[string][]answerSupportIndexedItem),
		caveatsByFile:       make(map[string][]answerSupportIndexedItem),
	}
	if doc == nil {
		return index
	}
	for _, block := range doc.Blocks {
		principal := answerBlockCanCarryPrincipalMember(block)
		caveat := block.Kind == BlockCaveat
		if !principal && !caveat {
			continue
		}
		for _, item := range block.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			entry := answerSupportIndexedItem{
				item:      item,
				blockText: block.Text,
				citation:  doc.Citations[item.CitationRef],
			}
			if principal {
				index.addPrincipal(entry)
			}
			if caveat {
				index.addCaveat(entry)
			}
		}
	}
	return index
}

func (index *answerSupportDocumentIndex) addPrincipal(entry answerSupportIndexedItem) {
	if index == nil {
		return
	}
	index.addEntry(index.principalByLocation, index.principalByFile, entry)
}

func (index *answerSupportDocumentIndex) addCaveat(entry answerSupportIndexedItem) {
	if index == nil {
		return
	}
	index.addEntry(index.caveatsByLocation, index.caveatsByFile, entry)
}

func (index *answerSupportDocumentIndex) addEntry(byLocation, byFile map[string][]answerSupportIndexedItem, entry answerSupportIndexedItem) {
	location := citationLocationKeyForSupportMember(entry.citation)
	if location != "" {
		byLocation[location] = append(byLocation[location], entry)
	}
	file := normalizeAnswerSupportPath(entry.citation.File)
	if file != "" {
		byFile[file] = append(byFile[file], entry)
	}
}

func (index *answerSupportDocumentIndex) labelsFileOutputMember(ob AnswerSupportMemberObligation) bool {
	if index == nil {
		return false
	}
	want := normalizeAnswerSupportPath(ob.Source)
	if want == "" {
		return false
	}
	for _, entry := range index.principalCandidates(ob) {
		if !citationCoversSupportMember(entry.citation, ob) {
			continue
		}
		for _, surface := range answerSupportItemIdentitySurfaces(entry.item) {
			labelFile, ok := ParseAnswerFilePathSurface(surface)
			if !ok {
				continue
			}
			if answerLocationFileMatches(labelFile, want) {
				return true
			}
		}
	}
	return false
}

func answerSupportItemIdentitySurfaces(item AnswerBlockItem) []string {
	var out []string
	if label := strings.TrimSpace(item.Label); label != "" {
		out = append(out, label)
	}
	if len(out) == 0 {
		for _, cell := range item.Cells {
			cell = strings.TrimSpace(cell)
			if cell != "" {
				out = append(out, cell)
				break
			}
		}
	}
	return out
}

func appendUniqueClaimForm(in []ClaimForm, form ClaimForm) []ClaimForm {
	if form == "" {
		form = ClaimUnknown
	}
	for _, existing := range in {
		if existing == form {
			return in
		}
	}
	return append(in, form)
}

func answerDocumentCoversSupportMember(doc *AnswerDocumentV2, ob AnswerSupportMemberObligation) bool {
	return newAnswerSupportDocumentIndex(doc).coversSupportMember(ob)
}

func (index *answerSupportDocumentIndex) coversSupportMember(ob AnswerSupportMemberObligation) bool {
	if strings.TrimSpace(ob.Location) == "" {
		return false
	}
	if index == nil {
		return false
	}
	for _, entry := range index.principalCandidates(ob) {
		if !citationCoversSupportMember(entry.citation, ob) {
			continue
		}
		if len(ob.SurfaceTerms) == 0 || answerItemSurfaceMentionsSupportMember(entry.item, ob) {
			return true
		}
	}
	if index.caveatsSupportMember(ob) {
		return true
	}
	return false
}

func answerDocumentCaveatsSupportMember(doc *AnswerDocumentV2, ob AnswerSupportMemberObligation) bool {
	return newAnswerSupportDocumentIndex(doc).caveatsSupportMember(ob)
}

func (index *answerSupportDocumentIndex) caveatsSupportMember(ob AnswerSupportMemberObligation) bool {
	if index == nil {
		return false
	}
	for _, entry := range index.caveatCandidates(ob) {
		if !citationCoversSupportMember(entry.citation, ob) {
			continue
		}
		if len(ob.SurfaceTerms) == 0 ||
			answerItemSurfaceMentionsSupportMember(entry.item, ob) ||
			answerTextMentionsSupportMember(entry.blockText, ob) {
			return true
		}
	}
	return false
}

func (index *answerSupportDocumentIndex) principalCandidates(ob AnswerSupportMemberObligation) []answerSupportIndexedItem {
	if index == nil {
		return nil
	}
	return index.candidates(ob, index.principalByLocation, index.principalByFile)
}

func (index *answerSupportDocumentIndex) caveatCandidates(ob AnswerSupportMemberObligation) []answerSupportIndexedItem {
	if index == nil {
		return nil
	}
	return index.candidates(ob, index.caveatsByLocation, index.caveatsByFile)
}

func (index *answerSupportDocumentIndex) candidates(
	ob AnswerSupportMemberObligation,
	byLocation map[string][]answerSupportIndexedItem,
	byFile map[string][]answerSupportIndexedItem,
) []answerSupportIndexedItem {
	seen := make(map[string]struct{})
	var out []answerSupportIndexedItem
	add := func(items []answerSupportIndexedItem) {
		for _, entry := range items {
			key := supportIndexedItemKey(entry)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, entry)
		}
	}
	for _, location := range supportMemberCoverageLocations(ob) {
		add(byLocation[location])
	}
	if source := normalizeAnswerSupportPath(ob.Source); source != "" {
		add(byFile[source])
	}
	return out
}

func supportIndexedItemKey(entry answerSupportIndexedItem) string {
	key := citationLocationKeyForSupportMember(entry.citation)
	if key == "" {
		key = normalizeAnswerSupportPath(entry.citation.File)
	}
	return key + "\x00" + strings.TrimSpace(entry.item.ID) + "\x00" + strings.TrimSpace(entry.item.Label)
}

func answerBlockCanCarryPrincipalMember(block AnswerBlock) bool {
	switch block.Kind {
	case BlockOrderedList, BlockBulletList, BlockTable:
	default:
		return false
	}
	if block.SurfaceRole == SurfacePrincipal {
		return true
	}
	for _, facet := range block.FacetIDs {
		if strings.TrimSpace(facet) == string(FacetEnumerationItem) {
			return true
		}
	}
	for _, cu := range block.ClaimUses {
		if strings.TrimSpace(cu.FacetID) == string(FacetEnumerationItem) {
			return true
		}
	}
	// Enumeration answers frequently use the principal list/table
	// block before all annotations are filled in. Treat the block
	// shape itself as a carrier; other validators still enforce
	// missing facet/claim annotations.
	return true
}

func answerItemSurfaceMentionsSupportMember(item AnswerBlockItem, ob AnswerSupportMemberObligation) bool {
	return answerTextMentionsSupportMember(AnswerBlockItemVisibleSurface(item), ob)
}

// AnswerTextMentionsSupportMember reports whether a user-visible text surface
// preserves a typed principal support member. It is exported for emit-time
// schema normalizers that need to add/repair citation sidecars only when the
// model has already rendered the member visibly.
func AnswerTextMentionsSupportMember(text string, ob AnswerSupportMemberObligation) bool {
	return answerTextMentionsSupportMember(text, ob)
}

func answerTextMentionsSupportMember(text string, ob AnswerSupportMemberObligation) bool {
	surface := strings.TrimSpace(text)
	if surface == "" {
		return false
	}
	for _, term := range ob.SurfaceTerms {
		if supportMemberTermAppears(term, surface) {
			return true
		}
	}
	if supportMemberTermAppears(ob.Label, surface) {
		return true
	}
	return false
}

func supportMemberTermAppears(term, surface string) bool {
	term = strings.TrimSpace(term)
	surface = strings.TrimSpace(surface)
	if term == "" || surface == "" {
		return false
	}
	if supportMemberDecoratedTermAppears(term, surface) {
		return true
	}
	for _, candidate := range supportMemberDisplayCandidates(term) {
		if supportMemberDisplaySurfaceAppears(candidate, surface) ||
			CodeSurfaceAppearsAsToken(candidate, surface) ||
			supportMemberRelationPartsAppear(candidate, surface) {
			return true
		}
	}
	return false
}

func supportMemberDecoratedTermAppears(term, surface string) bool {
	base, qualifier, ok := AnswerAggregateDecoratedLabelParts(term)
	if !ok {
		return false
	}
	if !supportMemberDisplaySurfaceAppears(base, surface) && !CodeSurfaceAppearsAsToken(base, surface) {
		return false
	}
	return supportMemberQualifierAppears(qualifier, surface)
}

func supportMemberQualifierAppears(qualifier, surface string) bool {
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		return true
	}
	if supportMemberDisplaySurfaceAppears(qualifier, surface) || CodeSurfaceAppearsAsToken(qualifier, surface) {
		return true
	}
	parts := splitSupportMemberQualifierParts(qualifier)
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if !supportMemberDisplaySurfaceAppears(part, surface) && !CodeSurfaceAppearsAsToken(part, surface) {
			return false
		}
	}
	return true
}

func splitSupportMemberQualifierParts(qualifier string) []string {
	fields := strings.FieldsFunc(qualifier, func(r rune) bool {
		switch r {
		case '/', '|', ',', '，', ';', '；':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func supportMemberDisplaySurfaceAppears(term, surface string) bool {
	if displaySurfaceAppears(term, surface) {
		return true
	}
	term = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(term)), " "))
	surface = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(surface)), " "))
	if term == "" || surface == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(surface[start:], term)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if displaySurfaceBoundary(surface, pos-1) && displaySurfaceBoundary(surface, pos+len(term)) {
			return true
		}
		start = pos + len(term)
		if start >= len(surface) {
			return false
		}
	}
}

func supportMemberRelationPartsAppear(candidate, surface string) bool {
	for _, relationSurface := range supportMemberRelationSurfacePrefixes(candidate) {
		left, right, ok := AnswerAggregateMemberRelationParts(relationSurface)
		if !ok {
			continue
		}
		if supportMemberDisplaySurfaceAppears(left, surface) &&
			supportMemberDisplaySurfaceAppears(right, surface) {
			return true
		}
	}
	return false
}

func supportMemberRelationSurfacePrefixes(term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	out := []string{term}
	if label, _, ok := ParseAnswerSupportRefMemberLocation(term); ok && strings.TrimSpace(label) != "" {
		out = append(out, label)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(term, sep); idx > 0 {
			prefix := strings.TrimSpace(term[:idx])
			if prefix != "" {
				out = append(out, prefix)
			}
		}
	}
	return out
}

func supportMemberDisplayCandidates(term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	var add func(string)
	add = func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, candidate)
		if tail := supportMemberSurfaceSymbolTail(candidate); tail != "" && strings.ToLower(tail) != key {
			add(tail)
		}
		if normalizedTail := NormalizedSurfaceSymbolTail(candidate); normalizedTail != "" && normalizedTail != strings.ToLower(candidate) {
			add(normalizedTail)
		}
	}
	add(term)
	if label, _, ok := ParseAnswerSupportRefMemberLocation(term); ok && strings.TrimSpace(label) != "" {
		for _, candidate := range AnswerAggregateMemberDisplayCandidates(label) {
			add(candidate)
		}
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(term, sep); idx > 0 {
			prefix := strings.TrimSpace(term[:idx])
			if prefix == "" {
				continue
			}
			for _, candidate := range AnswerAggregateMemberDisplayCandidates(prefix) {
				add(candidate)
			}
		}
	}
	for _, candidate := range AnswerAggregateMemberDisplayCandidates(term) {
		add(candidate)
	}
	return out
}

func supportMemberSurfaceSymbolTail(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if fields := strings.Fields(raw); len(fields) > 0 {
		raw = fields[0]
	}
	raw = strings.TrimSuffix(raw, "()")
	raw = strings.Trim(raw, "`")
	if idx := strings.LastIndex(raw, "::"); idx >= 0 {
		raw = raw[idx+2:]
	}
	if idx := strings.LastIndexAny(raw, `/\`); idx >= 0 {
		raw = raw[idx+1:]
	}
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		raw = raw[idx+1:]
	}
	raw = strings.Trim(raw, "()")
	raw = strings.TrimLeft(raw, "*&")
	return strings.TrimSpace(raw)
}

func citationLocationKeyForSupportMember(cit Citation) string {
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return ""
	}
	return normalizeAnswerSupportLocation(fmt.Sprintf("%s:%d", file, cit.Line))
}

func citationCoversSupportMember(cit Citation, ob AnswerSupportMemberObligation) bool {
	key := citationLocationKeyForSupportMember(cit)
	if key == "" {
		return false
	}
	for _, location := range supportMemberCoverageLocations(ob) {
		if key == location {
			return true
		}
	}
	if citationWithinSupportMemberRange(cit, ob) {
		return true
	}
	return citationWithinDefinitionSupportMemberWindow(cit, ob)
}

func citationWithinDefinitionSupportMemberWindow(cit Citation, ob AnswerSupportMemberObligation) bool {
	if ob.ClaimForm != ClaimDefinitionFact {
		return false
	}
	if strings.TrimSpace(ob.Source) == "" || ob.LineStart <= 0 {
		return false
	}
	if cit.Line <= 0 || normalizeAnswerSupportPath(cit.File) != normalizeAnswerSupportPath(ob.Source) {
		return false
	}
	return absInt(cit.Line-ob.LineStart) <= definitionSupportMemberCitationWindow
}

func citationWithinSupportMemberRange(cit Citation, ob AnswerSupportMemberObligation) bool {
	if strings.TrimSpace(ob.Source) == "" || ob.LineStart <= 0 || ob.LineEnd <= ob.LineStart {
		return false
	}
	if cit.Line <= 0 || normalizeAnswerSupportPath(cit.File) != normalizeAnswerSupportPath(ob.Source) {
		return false
	}
	return cit.Line >= ob.LineStart && cit.Line <= ob.LineEnd
}

func supportMemberCoverageLocations(ob AnswerSupportMemberObligation) []string {
	var out []string
	add := func(location string) {
		location = normalizeAnswerSupportLocation(location)
		if location == "" {
			return
		}
		for _, existing := range out {
			if existing == location {
				return
			}
		}
		out = append(out, location)
	}
	add(ob.Location)
	if ob.LineEnd > ob.LineStart && strings.TrimSpace(ob.Source) != "" {
		add(fmt.Sprintf("%s:%d", ob.Source, ob.LineEnd))
	}
	for _, location := range ob.EquivalentLocations {
		add(location)
	}
	return out
}

func supportMemberLineEnd(lineStart, lineEnd int) int {
	if lineStart <= 0 || lineEnd <= lineStart {
		return 0
	}
	return lineEnd
}

func normalizeAnswerSupportLocation(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	location = strings.ReplaceAll(location, `\`, `/`)
	return strings.ToLower(location)
}

func normalizeAnswerSupportPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, `\`, `/`)
	return strings.ToLower(path)
}

func supportMemberSourceLine(source string, line int, location string) (string, int) {
	source = strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
	if line > 0 && source != "" {
		return normalizeAnswerSupportPath(source), line
	}
	locSource, locLine := splitAnswerSupportLocation(location)
	if source == "" {
		source = locSource
	}
	if line <= 0 {
		line = locLine
	}
	return normalizeAnswerSupportPath(source), line
}

func splitAnswerSupportLocation(location string) (string, int) {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return "", 0
	}
	idx := strings.LastIndex(location, ":")
	if idx < 0 || idx == len(location)-1 {
		return normalizeAnswerSupportPath(location), 0
	}
	line, err := strconv.Atoi(location[idx+1:])
	if err != nil || line <= 0 {
		return normalizeAnswerSupportPath(location), 0
	}
	return normalizeAnswerSupportPath(location[:idx]), line
}

func normalizedSupportMemberIdentity(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if tail := NormalizedSurfaceSymbolTail(label); tail != "" {
		return tail
	}
	return strings.ToLower(label)
}
