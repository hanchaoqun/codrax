package types

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ClaimCitationRoleIdentityKind classifies the precise identity shape
// that can be recovered from an answer-visible item surface for
// citation-role alignment. This is deliberately separate from
// ClaimLabelSurfaceKind: label-surface gates decide whether a label
// should be treated as a source symbol, while citation-role alignment
// decides whether a visible sentence names a typed evidence claim
// strongly enough to require a matching citation.
type ClaimCitationRoleIdentityKind string

const (
	ClaimCitationRoleIdentityNone ClaimCitationRoleIdentityKind = ""

	// ClaimCitationRoleDirectedEdge means the evidence identity is a
	// directed subject -> object edge. The hard gate only fires when
	// both endpoints appear as exact code-token surfaces in the item.
	ClaimCitationRoleDirectedEdge ClaimCitationRoleIdentityKind = "directed_edge"

	// ClaimCitationRoleDisplaySurface means the claim identity is a
	// typed display surface carried by structured evidence fields
	// such as SurfaceTerms, DiagramRole, or FileRoleLabel. This covers
	// labels that are not source declarations: imports, config layers,
	// trace/log labels, routes, spans, and similar surfaces.
	ClaimCitationRoleDisplaySurface ClaimCitationRoleIdentityKind = "display_surface"
)

func (k ClaimCitationRoleIdentityKind) IsValid() bool {
	switch k {
	case ClaimCitationRoleDirectedEdge, ClaimCitationRoleDisplaySurface:
		return true
	default:
		return false
	}
}

// CitationRoleIdentityKind returns the item-surface identity shape
// that may be used for hard citation-role alignment for this claim
// form. Forms that return ClaimCitationRoleIdentityNone are still
// checked by the existing ClaimFormSupport / symbol-label gates; they
// simply do not have a precise enough item-surface role identity for
// this specific hard gate.
func (c ClaimForm) CitationRoleIdentityKind() ClaimCitationRoleIdentityKind {
	switch c {
	case ClaimCallEdge, ClaimImportEdge:
		return ClaimCitationRoleDirectedEdge
	case ClaimPrecedenceRole, ClaimExternalObservation, ClaimLiteralValueFact,
		ClaimTextReferenceFact:
		return ClaimCitationRoleDisplaySurface
	case ClaimDefinitionFact, ClaimGuardCondition, ClaimAssignmentFact,
		ClaimReturnFact, ClaimAbsenceFact:
		return ClaimCitationRoleIdentityNone
	default:
		return ClaimCitationRoleIdentityNone
	}
}

// SupportsCitationRoleAlignment reports whether an answer item that
// visibly names this claim form's typed role can be required to cite
// evidence for the same role. The predicate is exhaustive over the
// ClaimForm enum through CitationRoleIdentityKind, so future forms
// must choose a role identity shape before the tests pass.
func (c ClaimForm) SupportsCitationRoleAlignment() bool {
	return c.CitationRoleIdentityKind().IsValid()
}

// ClaimFormsSupportingCitationRoleAlignment filters and de-duplicates
// forms that can participate in typed item/citation role alignment.
func ClaimFormsSupportingCitationRoleAlignment(forms []ClaimForm) []ClaimForm {
	seen := make(map[ClaimForm]bool, len(forms))
	out := make([]ClaimForm, 0, len(forms))
	for _, f := range forms {
		if !f.SupportsCitationRoleAlignment() || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// EvidenceClaimRoleMentionedBySurface reports whether a visible
// surface asserts ev's claim role strongly enough for a hard
// citation-role alignment check. It consumes only typed evidence fields
// plus exact structural carriers; it never scans user prose for
// heuristic keywords.
func EvidenceClaimRoleMentionedBySurface(ev EvidenceItem, allowed []ClaimForm, surface string) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" || ev.GroundingStatus == GroundingUngrounded {
		return false
	}
	form := ClaimFormOf(ev)
	if !claimFormAllowed(form, allowed) || !form.SupportsCitationRoleAlignment() {
		return false
	}
	switch form.CitationRoleIdentityKind() {
	case ClaimCitationRoleDirectedEdge:
		return evidenceDirectedEdgeMentioned(surface, ev)
	case ClaimCitationRoleDisplaySurface:
		return evidenceDisplaySurfaceMentioned(surface, ev)
	default:
		return false
	}
}

// EvidenceClaimRoleAssertedByAnswerSurface is the answer-item specific
// projection used by emit-time and post-emit citation-role validators.
// Directed relations may be asserted by label or text only when the
// endpoints are connected by an explicit edge surface (for example
// `A -> B` or Mermaid-style arrows). Display-surface roles are limited
// to the item label, which is the principal row/member surface; body
// prose may mention comparison, caveat, or boundary context without
// becoming a hard claim-role assertion.
func EvidenceClaimRoleAssertedByAnswerSurface(ev EvidenceItem, allowed []ClaimForm, label, text string) bool {
	if ev.GroundingStatus == GroundingUngrounded {
		return false
	}
	form := ClaimFormOf(ev)
	if !claimFormAllowed(form, allowed) || !form.SupportsCitationRoleAlignment() {
		return false
	}
	switch form.CitationRoleIdentityKind() {
	case ClaimCitationRoleDirectedEdge:
		return evidenceDirectedEdgeMentioned(strings.TrimSpace(label+"\n"+text), ev)
	case ClaimCitationRoleDisplaySurface:
		return evidenceDisplaySurfaceMentioned(strings.TrimSpace(label), ev)
	default:
		return false
	}
}

// EvidenceSetContainsSameClaimRole checks whether a citation's
// evidence pool contains the same typed claim role as expected. ID
// equality wins; otherwise the comparison falls back to the form's
// typed identity fields.
func EvidenceSetContainsSameClaimRole(items []EvidenceItem, expected EvidenceItem) bool {
	for _, ev := range items {
		if SameEvidenceClaimRole(ev, expected) {
			return true
		}
	}
	return false
}

// SameEvidenceClaimRole compares two evidence items at the typed claim
// role level. It is stricter than "same line" and weaker than "same
// evidence ID": two independently emitted items may describe the same
// edge or display role, but a definition line for the same symbol must
// not satisfy a call/import edge claim.
func SameEvidenceClaimRole(a, b EvidenceItem) bool {
	if strings.TrimSpace(a.ID) != "" && strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) {
		return true
	}
	form := ClaimFormOf(a)
	if form == ClaimUnknown || form != ClaimFormOf(b) || !form.SupportsCitationRoleAlignment() {
		return false
	}
	switch form.CitationRoleIdentityKind() {
	case ClaimCitationRoleDirectedEdge:
		return codeSurfaceMatches(a.Subject, b.Subject) && codeSurfaceMatches(a.Object, b.Object)
	case ClaimCitationRoleDisplaySurface:
		if sameEvidenceLocation(a, b) {
			return true
		}
		return displaySurfaceTermsOverlap(a, b)
	default:
		return false
	}
}

func EvidenceClaimRoleName(ev EvidenceItem) string {
	form := ClaimFormOf(ev)
	switch form.CitationRoleIdentityKind() {
	case ClaimCitationRoleDirectedEdge:
		subject := strings.TrimSpace(ev.Subject)
		object := strings.TrimSpace(ev.Object)
		if subject != "" && object != "" {
			return fmt.Sprintf("%s -> %s", subject, object)
		}
	case ClaimCitationRoleDisplaySurface:
		if terms := evidenceDisplayRoleTerms(ev); len(terms) > 0 {
			return fmt.Sprintf("%s %q", form, terms[0])
		}
	}
	if form != ClaimUnknown {
		return string(form)
	}
	return "typed claim role"
}

func EvidenceClaimRoleLocation(ev EvidenceItem) string {
	source := strings.TrimSpace(ev.Source)
	if source == "" || ev.LineStart <= 0 {
		return "the matching typed evidence"
	}
	return fmt.Sprintf("%s:%d", source, ev.LineStart)
}

func claimFormAllowed(form ClaimForm, allowed []ClaimForm) bool {
	if form == ClaimUnknown {
		return false
	}
	if len(allowed) == 0 {
		return form.SupportsCitationRoleAlignment()
	}
	for _, f := range allowed {
		if f == form {
			return true
		}
	}
	return false
}

func evidenceDirectedEdgeMentioned(surface string, ev EvidenceItem) bool {
	subject := strings.TrimSpace(ev.Subject)
	object := strings.TrimSpace(ev.Object)
	if subject == "" || object == "" {
		return false
	}
	return explicitDirectedEdgeSurfaceConnects(subject, object, surface)
}

func evidenceDisplaySurfaceMentioned(surface string, ev EvidenceItem) bool {
	for _, term := range evidenceDisplayRoleTerms(ev) {
		if displaySurfaceAppears(term, surface) {
			return true
		}
	}
	return false
}

func displaySurfaceTermsOverlap(a, b EvidenceItem) bool {
	aTerms := evidenceDisplayRoleTerms(a)
	bTerms := evidenceDisplayRoleTerms(b)
	if len(aTerms) == 0 || len(bTerms) == 0 {
		return false
	}
	seen := make(map[string]bool, len(aTerms))
	for _, term := range aTerms {
		if key := normalizedDisplaySurface(term); key != "" {
			seen[key] = true
		}
	}
	for _, term := range bTerms {
		if seen[normalizedDisplaySurface(term)] {
			return true
		}
	}
	return false
}

func evidenceDisplayRoleTerms(ev EvidenceItem) []string {
	var out []string
	form := ClaimFormOf(ev)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if normalizedDisplaySurface(existing) == normalizedDisplaySurface(s) {
				return
			}
		}
		out = append(out, s)
	}
	for _, term := range ev.SurfaceTerms {
		if form == ClaimPrecedenceRole && isGenericPrecedenceRoleSurfaceTerm(term) {
			continue
		}
		add(term)
	}
	add(ev.Subject)
	add(ev.Object)
	add(ev.AnchorSymbol)
	add(ev.OwnerSymbol)
	if form == ClaimPrecedenceRole {
		add(ev.Source)
	} else if ev.DiagramRole != EvidenceDiagramRoleUnknown && ev.DiagramRole != EvidenceDiagramRoleDefault {
		add(string(ev.DiagramRole))
	}
	if form != ClaimPrecedenceRole &&
		ev.RequestedDiagramRole != EvidenceDiagramRoleUnknown &&
		ev.RequestedDiagramRole != EvidenceDiagramRoleDefault {
		add(string(ev.RequestedDiagramRole))
	}
	if ev.FileRoleLabel != "" {
		add(string(ev.FileRoleLabel))
	}
	if ev.LogPerfSubKind != "" {
		add(string(ev.LogPerfSubKind))
	}
	return out
}

func isGenericPrecedenceRoleSurfaceTerm(term string) bool {
	switch normalizedDisplaySurface(term) {
	case "config",
		"configcanonical",
		"configuration",
		"default",
		"defaults",
		"runtime",
		"override",
		"overrides",
		"yaml":
		return true
	default:
		return false
	}
}

func displaySurfaceAppears(term, surface string) bool {
	term = normalizedDisplaySurface(term)
	surface = normalizedDisplaySurface(surface)
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

func normalizedDisplaySurface(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func displaySurfaceBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

func sameEvidenceLocation(a, b EvidenceItem) bool {
	return strings.TrimSpace(a.Source) != "" &&
		strings.TrimSpace(a.Source) == strings.TrimSpace(b.Source) &&
		a.LineStart > 0 && a.LineStart == b.LineStart
}

func codeSurfaceMatches(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
}

type codeSurfaceSpan struct {
	start int
	end   int
}

func explicitDirectedEdgeSurfaceConnects(subject, object, surface string) bool {
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(object) == "" || strings.TrimSpace(surface) == "" {
		return false
	}
	subjectSpans := codeSurfaceTokenSpans(subject, surface)
	objectSpans := codeSurfaceTokenSpans(object, surface)
	if len(subjectSpans) == 0 || len(objectSpans) == 0 {
		return false
	}
	for _, from := range subjectSpans {
		for _, to := range objectSpans {
			if from.end > to.start {
				continue
			}
			if isExplicitDirectedEdgeConnector(surface[from.end:to.start]) {
				return true
			}
		}
	}
	return false
}

func codeSurfaceTokenSpans(needle, haystack string) []codeSurfaceSpan {
	needle = strings.TrimSpace(needle)
	haystack = strings.TrimSpace(haystack)
	if needle == "" || haystack == "" {
		return nil
	}
	var out []codeSurfaceSpan
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return out
		}
		pos := start + idx
		end := pos + len(needle)
		if codeSurfaceBoundary(haystack, pos-1) && codeSurfaceBoundary(haystack, end) {
			out = append(out, codeSurfaceSpan{start: pos, end: end})
		}
		start = end
		if start >= len(haystack) {
			return out
		}
	}
}

func isExplicitDirectedEdgeConnector(raw string) bool {
	conn := strings.TrimSpace(raw)
	conn = strings.Trim(conn, "`'\"“”‘’")
	conn = strings.TrimSpace(conn)
	conn = strings.Join(strings.Fields(conn), "")
	switch conn {
	case "->", "=>", "→", "-->", "==>", "-.->":
		return true
	}
	if strings.HasPrefix(conn, "-->") {
		return true
	}
	if strings.HasPrefix(conn, "--") && strings.HasSuffix(conn, "-->") {
		return true
	}
	if strings.HasPrefix(conn, "==") && strings.HasSuffix(conn, "==>") {
		return true
	}
	if strings.HasPrefix(conn, "-.") && strings.HasSuffix(conn, ".->") {
		return true
	}
	return false
}

// CodeSurfaceAppearsAsToken is shared by answer validators that need
// exact code-identity occurrence checks. It treats identifier
// characters plus dot, slash, colon, and dash as token continuations
// so package paths, C++ qualifiers, ArkTS paths, routes, and Cangjie
// qualified identifiers do not spuriously match substrings.
func CodeSurfaceAppearsAsToken(needle, haystack string) bool {
	needle = strings.TrimSpace(needle)
	haystack = strings.TrimSpace(haystack)
	if needle == "" || haystack == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if codeSurfaceBoundary(haystack, pos-1) && codeSurfaceBoundary(haystack, pos+len(needle)) {
			return true
		}
		start = pos + len(needle)
		if start >= len(haystack) {
			return false
		}
	}
}

func codeSurfaceBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '/' || r == ':' || r == '-')
}
