package types

import (
	"fmt"
	"sort"
	"strings"
)

// compileCallChainSupportPlan compiles the per-family support lanes
// for QFCallChain answers: observation lane (if attached log/trace
// gave grounded frames), the principal current-grounded-call-chain
// lane, and the call-chain uncertainty lane for drift / scope
// boundary disclosures. Returns nil when no lane has entries so the
// renderer can skip the section entirely.
func compileCallChainSupportPlan(rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	out := &AnswerSupportPlan{Family: QFCallChain}
	if lane := compileObservedArtifactSupportLane(rm, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCallChainCurrentPathSupportLane(rm, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCallChainUncertaintySupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

func compileCallChainCurrentPathSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneCurrentCodePath,
		Title:         "Current grounded call chain",
		AllowedBlocks: []string{"summary", "ordered_list", "diagram"},
		Guidance: "Use this lane for the principal directed relations and any sequence diagram. " +
			"Preserve every proved edge direction. Treat two entries as consecutive hops only when the first edge's callee is the second edge's caller; " +
			"multiple calls owned by the same caller are sibling call sites, not a callee-to-callee path. Source-line order may organize those call sites for reading, " +
			"but does not by itself prove that every branch executed or that values flowed between the callees. Do not add nearby helpers, " +
			"search-hint subjects, prior-turn subjects, or runtime frames as additional principal hops " +
			"unless they also appear in this lane or are separately cited as part of the same requested chain.",
	}
	if plan == nil {
		return lane
	}
	seen := make(map[string]bool)
	add := func(entry AnswerSupportEntry) {
		entry.Text = strings.TrimSpace(entry.Text)
		entry.Location = strings.TrimSpace(strings.ReplaceAll(entry.Location, `\`, `/`))
		if entry.Text == "" || len(lane.Entries) >= callChainSupportEntryLimit {
			return
		}
		key := strings.ToLower(entry.Text) + "\x00" + strings.ToLower(entry.Location)
		if seen[key] {
			return
		}
		seen[key] = true
		lane.Entries = append(lane.Entries, entry)
	}
	for _, entry := range selectCallChainSupportEntries(rm, plan) {
		add(entry)
	}
	return lane
}

// projectCallChainEndpointBoundarySupportPlan keeps an accepted exact
// no-directed-path boundary from publishing every nearby call edge as a
// principal path member. The endpoint evidence capsule already contains the
// bounded, direction-preserving edges that explain the boundary. Only those
// edges remain in the principal current-path lane; unrelated sibling calls stay
// available in the raw evidence pool for model-authored supporting discussion.
//
// This projection consumes only typed endpoint disposition and typed call-edge
// identities. It does not inspect request prose, model prose, or answer text,
// and it never writes an answer or chooses a conclusion.
func projectCallChainEndpointBoundarySupportPlan(plan *AnswerSupportPlan, boundary *CallChainEndpointBoundary) *AnswerSupportPlan {
	if plan == nil || plan.Family != QFCallChain || boundary == nil ||
		!boundary.Active() || boundary.Disposition != CallChainEndpointNoDirectedPath ||
		boundary.EvidenceCapsule == nil ||
		boundary.EvidenceCapsule.Status == CallChainEndpointEvidenceDirectedPathPresent {
		return plan
	}
	allowed := callChainEndpointBoundaryEvidenceEdges(boundary.EvidenceCapsule)
	lanes := make([]AnswerSupportLane, 0, len(plan.Lanes))
	for _, lane := range plan.Lanes {
		if lane.Kind != SupportLaneCurrentCodePath {
			lanes = append(lanes, lane)
			continue
		}
		filtered := make([]AnswerSupportEntry, 0, len(lane.Entries))
		for _, entry := range lane.Entries {
			if entry.ClaimForm != ClaimCallEdge ||
				!callChainSupportEntryMatchesBoundaryEdge(entry, allowed) {
				continue
			}
			filtered = append(filtered, entry)
		}
		if len(filtered) == 0 {
			continue
		}
		lane.Entries = filtered
		lane.Guidance = "This exact endpoint investigation established a no-directed-path boundary. " +
			"This lane contains only the grounded call edges that explain that boundary. " +
			"Keep each edge in its real direction and keep the requested sink separate; do not promote other same-caller calls into intermediate hops."
		lanes = append(lanes, lane)
	}
	plan.Lanes = lanes
	return plan
}

func callChainEndpointBoundaryEvidenceEdges(capsule *CallChainEndpointEvidenceCapsule) []CallChainEvidenceEdge {
	return CallChainEndpointBoundaryPrincipalEdges(capsule)
}

func callChainSupportEntryMatchesBoundaryEdge(entry AnswerSupportEntry, allowed []CallChainEvidenceEdge) bool {
	for _, edge := range allowed {
		if AnswerCodeIdentitySurfacesEquivalent(entry.Subject, edge.From) &&
			AnswerCodeIdentitySurfacesEquivalent(entry.Object, edge.To) {
			return true
		}
	}
	return false
}

func selectCallChainSupportEntries(rm RequestModel, plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil {
		return nil
	}
	endpoints := callChainRequestedEndpointHints(rm)
	stepEntries := callChainStepBackboneEntries(plan)
	evidenceEntries := callChainSurfaceEvidenceEntries(rm, plan)
	var base []AnswerSupportEntry
	if callChainPreferSurfaceEvidence(rm, stepEntries, evidenceEntries) {
		base = evidenceEntries
	} else if len(stepEntries) > 0 {
		base = stepEntries
	} else {
		base = evidenceEntries
	}

	// StepBackbone is intentionally path-shaped, so exact non-hop facts such as
	// a caller-local guard or connected factory return may not have their own
	// step. They are nevertheless principal support for explaining the selected
	// path. Add only typed, citable rows connected to an already-selected path
	// member; never infer control from line adjacency or free-form summaries.
	extras := callChainPrincipalControlSelectionEntries(plan, base)
	if len(extras) >= callChainSupportEntryLimit {
		return callChainCondenseSupportEntries(extras, endpoints, callChainSupportEntryLimit)
	}
	base = callChainCondenseSupportEntries(base, endpoints, callChainSupportEntryLimit-len(extras))
	return appendUniqueCallChainSupportEntries(base, extras...)
}

func callChainPrincipalControlSelectionEntries(plan *AnswerSurfacePlan, base []AnswerSupportEntry) []AnswerSupportEntry {
	if plan == nil || len(base) == 0 {
		return nil
	}
	evidence := callChainTypedEvidenceItemsForStepEnrichment(plan)
	principal := callChainEvidenceOwnedBySupportEntries(base, evidence)
	if len(principal) == 0 {
		return nil
	}

	type principalOwner struct {
		name   string
		source string
	}
	var owners []principalOwner
	var selectionPool []EvidenceItem
	var endpoints []string
	for _, item := range principal {
		switch ClaimFormOf(item) {
		case ClaimCallEdge:
			selectionPool = append(selectionPool, item)
			endpoints = append(endpoints, item.Subject, item.Object)
			if owner := strings.TrimSpace(item.Subject); owner != "" {
				owners = append(owners, principalOwner{name: owner, source: normalizeAnswerSupportPath(item.Source)})
			}
		case ClaimRegistrationEdge:
			selectionPool = append(selectionPool, item)
			endpoints = append(endpoints, item.Subject, item.Object)
		}
	}

	for _, item := range evidence {
		if !item.IsCitable() {
			continue
		}
		switch ClaimFormOf(item) {
		case ClaimAssignmentFact, ClaimReturnFact:
			selectionPool = append(selectionPool, item)
		case ClaimRegistrationEdge:
			if callChainEvidenceTouchesAnyEndpoint(item, endpoints) {
				selectionPool = append(selectionPool, item)
			}
		}
	}
	selection := CallChainDiscoverySelectionEvidence(selectionPool)
	for _, item := range selection {
		if subject := strings.TrimSpace(item.Subject); subject != "" {
			owners = append(owners, principalOwner{name: subject, source: normalizeAnswerSupportPath(item.Source)})
		}
	}

	var out []AnswerSupportEntry
	guardStateSymbols := make(map[string]map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimGuardCondition || strings.TrimSpace(item.OwnerSymbol) == "" {
			continue
		}
		itemSource := normalizeAnswerSupportPath(item.Source)
		for _, owner := range owners {
			if owner.source != "" && itemSource != owner.source {
				continue
			}
			if !CallChainEndpointCompatible(item.OwnerSymbol, owner.name) {
				continue
			}
			if entry, ok := callChainPrincipalSupportEntryForEvidence(item); ok {
				out = appendUniqueCallChainSupportEntries(out, entry)
				if symbol := callChainExactSemanticIdentity(item.AnchorSymbol); symbol != "" && itemSource != "" {
					if guardStateSymbols[itemSource] == nil {
						guardStateSymbols[itemSource] = make(map[string]bool)
					}
					guardStateSymbols[itemSource][symbol] = true
				}
			}
			break
		}
	}
	// A guard's exact input-state assignments explain fallback/feature-switch
	// alternatives without becoming invocation hops.  Require the same source
	// file, an exact typed guard symbol, and a snippet-verified scalar assignment;
	// never infer branch ownership from line adjacency.
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimAssignmentFact || !AssignmentEvidenceStateMatches(item) {
			continue
		}
		source := normalizeAnswerSupportPath(item.Source)
		symbol := callChainExactSemanticIdentity(item.Subject)
		if source == "" || symbol == "" || !guardStateSymbols[source][symbol] {
			continue
		}
		if entry, ok := callChainPrincipalSupportEntryForEvidence(item); ok {
			out = appendUniqueCallChainSupportEntries(out, entry)
		}
	}
	for _, item := range selection {
		if entry, ok := callChainPrincipalSupportEntryForEvidence(item); ok {
			out = appendUniqueCallChainSupportEntries(out, entry)
		}
	}
	// base can carry repeated display entries even when they refer to the same
	// typed evidence identity.  Slice against the deduplicated prefix, not the
	// raw base cardinality: otherwise a duplicate in base makes the combined
	// result shorter than len(base) and turns a harmless support-plan rebuild
	// into a process panic.
	uniqueBase := appendUniqueCallChainSupportEntries(nil, base...)
	combined := appendUniqueCallChainSupportEntries(append([]AnswerSupportEntry(nil), uniqueBase...), out...)
	return combined[len(uniqueBase):]
}

func callChainExactSemanticIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "->", ".")
	raw = strings.ReplaceAll(raw, "::", ".")
	raw = strings.ReplaceAll(raw, "#", ".")
	return strings.Trim(raw, ".")
}

func callChainEvidenceOwnedBySupportEntries(base []AnswerSupportEntry, evidence []EvidenceItem) []EvidenceItem {
	ids := make(map[string]bool)
	locations := make(map[string]bool)
	for _, entry := range base {
		if id := strings.TrimSpace(entry.EvidenceID); id != "" {
			ids[id] = true
			continue
		}
		if source := normalizeAnswerSupportPath(entry.Source); source != "" && entry.LineStart > 0 {
			locations[fmt.Sprintf("%s:%d", source, entry.LineStart)] = true
		}
	}
	var out []EvidenceItem
	for _, item := range evidence {
		id := strings.TrimSpace(item.ID)
		location := fmt.Sprintf("%s:%d", normalizeAnswerSupportPath(item.Source), item.LineStart)
		if (id != "" && ids[id]) || locations[location] {
			out = append(out, item)
		}
	}
	return out
}

func callChainEvidenceTouchesAnyEndpoint(item EvidenceItem, endpoints []string) bool {
	for _, surface := range []string{item.Subject, item.Object} {
		for _, endpoint := range endpoints {
			if CallChainEndpointCompatible(surface, endpoint) {
				return true
			}
		}
	}
	return false
}

func callChainPrincipalSupportEntryForEvidence(item EvidenceItem) (AnswerSupportEntry, bool) {
	text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
	if text == "" {
		return AnswerSupportEntry{}, false
	}
	return answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)), true
}

func appendUniqueCallChainSupportEntries(dst []AnswerSupportEntry, entries ...AnswerSupportEntry) []AnswerSupportEntry {
	seen := make(map[string]bool, len(dst)+len(entries))
	key := func(entry AnswerSupportEntry) string {
		if id := strings.TrimSpace(entry.EvidenceID); id != "" {
			return "id:" + id
		}
		return fmt.Sprintf("surface:%s:%d:%s:%s:%s",
			normalizeAnswerSupportPath(entry.Source), entry.LineStart, entry.ClaimForm,
			strings.TrimSpace(entry.Subject), strings.TrimSpace(entry.Object))
	}
	for _, entry := range dst {
		seen[key(entry)] = true
	}
	for _, entry := range entries {
		k := key(entry)
		if seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, entry)
	}
	return dst
}

func callChainStepBackboneEntries(plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil || len(plan.StepBackbone) == 0 {
		return nil
	}
	typedItems := callChainTypedEvidenceItemsForStepEnrichment(plan)
	out := make([]AnswerSupportEntry, 0, len(plan.StepBackbone))
	for _, anchor := range plan.StepBackbone {
		text := callChainStepSupportText(anchor)
		if text == "" {
			continue
		}
		entry := AnswerSupportEntry{
			Text:      text,
			Detail:    callChainStepSupportDetail(anchor, text),
			Location:  stepSurfaceAnchorLocation(anchor),
			Source:    strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`)),
			LineStart: anchor.Line,
			LineEnd:   anchor.LineEnd,
		}
		if entry.Source != "" && entry.LineEnd > entry.LineStart {
			entry.EquivalentLocations = appendAnswerSupportEquivalentLocation(
				entry.EquivalentLocations,
				fmt.Sprintf("%s:%d", entry.Source, entry.LineEnd),
			)
		}
		if item, ok := callChainUniqueTypedEvidenceForStepAnchor(anchor, typedItems); ok {
			// Keep the step backbone's ordered display text, but do not discard
			// the exact ClaimForm/endpoint identity already available at the
			// same source coordinate. Without this join, assignments and guards
			// become indistinguishable from invocation hops downstream.
			orderedText := entry.Text
			orderedDetail := entry.Detail
			orderedLocation := entry.Location
			orderedEquivalents := append([]string(nil), entry.EquivalentLocations...)
			entry = answerSupportEntryForEvidence(item, orderedText, orderedDetail)
			entry.Location = orderedLocation
			for _, loc := range orderedEquivalents {
				entry.EquivalentLocations = appendAnswerSupportEquivalentLocation(entry.EquivalentLocations, loc)
			}
		}
		out = append(out, entry)
	}
	return out
}

func callChainTypedEvidenceItemsForStepEnrichment(plan *AnswerSurfacePlan) []EvidenceItem {
	if plan == nil {
		return nil
	}
	if len(plan.DriftBoundedSurfaceItems) > 0 {
		return DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems)
	}
	// StepBackbone already selected the visible row. Read the complete typed
	// surface pool only to recover metadata at that same exact coordinate;
	// facet candidate filtering is for membership selection and must not erase
	// the ClaimForm of a member that is already in the backbone.
	return plan.SurfaceEvidence
}

func callChainUniqueTypedEvidenceForStepAnchor(anchor StepSurfaceAnchor, items []EvidenceItem) (EvidenceItem, bool) {
	file := normalizeAnswerSupportPath(anchor.File)
	if file == "" || anchor.Line <= 0 || len(items) == 0 {
		return EvidenceItem{}, false
	}
	var atLocation []EvidenceItem
	for _, item := range items {
		if normalizeAnswerSupportPath(item.Source) != file || item.LineStart != anchor.Line {
			continue
		}
		atLocation = append(atLocation, item)
	}
	if len(atLocation) == 1 {
		return atLocation[0], true
	}
	name := strings.TrimSpace(anchor.Name)
	if name == "" {
		return EvidenceItem{}, false
	}
	var matched []EvidenceItem
	for _, item := range atLocation {
		for _, surface := range []string{item.Subject, item.Object, item.AnchorSymbol, item.OwnerSymbol} {
			if callChainEndpointCompatible(surface, name) {
				matched = append(matched, item)
				break
			}
		}
	}
	if len(matched) != 1 {
		return EvidenceItem{}, false
	}
	return matched[0], true
}

func callChainSurfaceEvidenceEntries(rm RequestModel, plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil {
		return nil
	}
	var out []AnswerSupportEntry
	for _, item := range orderedCallChainSupportEvidenceItems(rm, plan) {
		if !callChainPathItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		out = append(out,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
	}
	return out
}

func orderedCallChainSupportEvidenceItems(rm RequestModel, plan *AnswerSurfacePlan) []EvidenceItem {
	items := callChainSupportEvidenceItems(plan)
	if len(items) == 0 {
		return nil
	}
	out := append([]EvidenceItem(nil), items...)
	if callChainShouldSortSurfaceEvidenceByLine(rm, out) {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].LineStart == out[j].LineStart {
				return strings.TrimSpace(out[i].AnchorSymbol) < strings.TrimSpace(out[j].AnchorSymbol)
			}
			return out[i].LineStart < out[j].LineStart
		})
	}
	return out
}

func callChainShouldSortSurfaceEvidenceByLine(rm RequestModel, items []EvidenceItem) bool {
	if len(items) < 3 {
		return false
	}
	if rm.Intent != IntentTrace && NormalizeRequirementKind(rm.AnalyzerHints.Kind) != ReqCallChain {
		return false
	}
	var source string
	count := 0
	for _, item := range items {
		if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
			continue
		}
		canonical := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" {
			source = canonical
		}
		if canonical != source {
			return false
		}
		count++
	}
	return source != "" && count >= 3
}

func callChainStepSupportDetail(anchor StepSurfaceAnchor, text string) string {
	for _, raw := range []string{anchor.Chain, anchor.Rationale} {
		if detail := answerSupportEntryDetail(raw, text); detail != "" {
			return detail
		}
	}
	return ""
}

func callChainEvidenceSupportDetail(item EvidenceItem, text string) string {
	// Summary is model-authored prose, while the support lane is an
	// answer-authority surface. A grounded line/range proves only its typed
	// anchor carrier (call, condition, return, assignment, ...); grounding the
	// location does not semantically validate an arbitrary Summary that may
	// describe sibling functions or lines outside the cited span. EvidenceItem
	// already has the explicit LoadBearingSummary opt-in for the narrow cases
	// where dropping a model-authored scalar would drop the answer. Honour that
	// authority boundary here as EvidenceAuthoritativeSurfaceText does, instead
	// of silently re-attaching every Summary as an "Evidence note".
	detail := ""
	if item.LoadBearingSummary {
		detail = strings.TrimSpace(item.Summary)
	}
	if cond := strings.TrimSpace(item.Condition); cond != "" && !strings.Contains(strings.ToLower(detail), strings.ToLower(cond)) {
		if detail == "" {
			detail = "condition: " + cond
		} else {
			detail += "; condition: " + cond
		}
	}
	if surface := answerSupportEvidenceSurfaceMetadata(item, text); surface != "" {
		if detail == "" {
			detail = surface
		} else {
			detail += "; " + surface
		}
	}
	return answerSupportEntryDetail(detail, text)
}

func answerSupportEvidenceSurfaceMetadata(item EvidenceItem, text string) string {
	form := ClaimFormOf(item)
	parts := make([]string, 0, 5)
	if form != ClaimUnknown {
		parts = append(parts, "claim_form="+string(form))
		if label := form.LabelSurfaceKind(); label != ClaimLabelSurfaceUnknown {
			parts = append(parts, "label_surface="+string(label))
		}
	}
	if item.AnchorKind != "" {
		parts = append(parts, "anchor_kind="+string(item.AnchorKind))
	}
	if terms := answerSupportSurfaceTermsForDetail(item, text); len(terms) > 0 {
		parts = append(parts, "surface_terms="+strings.Join(terms, ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return "typed_surface: " + strings.Join(parts, "; ")
}

func answerSupportSurfaceTermsForDetail(item EvidenceItem, text string) []string {
	terms := evidenceDisplayRoleTerms(item)
	if len(terms) == 0 {
		return nil
	}
	textLower := strings.ToLower(text)
	out := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, raw := range terms {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		if !SurfaceTermShouldBeRequiredForEvidence(term, item) {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		if key != "" && strings.Contains(textLower, key) {
			continue
		}
		out = append(out, term)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func answerSupportEntryDetail(raw, text string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(text), strings.ToLower(raw)) {
		return ""
	}
	const max = 260
	if len(raw) > max {
		raw = strings.TrimSpace(CutPrefixRuneSafe(raw, max)) + "..."
	}
	return raw
}

func callChainPreferSurfaceEvidence(
	rm RequestModel,
	stepEntries []AnswerSupportEntry,
	evidenceEntries []AnswerSupportEntry,
) bool {
	if len(evidenceEntries) == 0 {
		return false
	}
	if len(stepEntries) == 0 {
		return true
	}
	endpoints := callChainRequestedEndpointHints(rm)
	if len(endpoints) == 0 {
		return false
	}
	stepCoverage := callChainEndpointCoverage(stepEntries, endpoints)
	evidenceCoverage := callChainEndpointCoverage(evidenceEntries, endpoints)
	if evidenceCoverage > stepCoverage {
		return true
	}
	if evidenceCoverage == stepCoverage && evidenceCoverage > 0 && len(evidenceEntries) > len(stepEntries)+1 {
		return true
	}
	return false
}

func callChainCondenseSupportEntries(entries []AnswerSupportEntry, endpoints []string, limit int) []AnswerSupportEntry {
	if limit <= 0 || len(entries) <= limit {
		return append([]AnswerSupportEntry(nil), entries...)
	}
	selected := make(map[int]bool, limit)
	add := func(idx int) {
		if idx < 0 || idx >= len(entries) || selected[idx] {
			return
		}
		if len(selected) >= limit {
			return
		}
		selected[idx] = true
	}
	add(0)
	for i := 1; i <= 6; i++ {
		add(len(entries) - i)
	}
	terminalEndpoints := callChainTerminalEndpointHints(endpoints)
	for i, entry := range entries {
		for _, endpoint := range terminalEndpoints {
			if callChainEntryMentionsEndpoint(entry, endpoint) {
				add(i)
				break
			}
		}
	}
	for slot := 0; slot < limit && len(selected) < limit; slot++ {
		idx := 0
		if limit > 1 {
			idx = slot * (len(entries) - 1) / (limit - 1)
		}
		add(idx)
	}
	if len(selected) < limit {
		for i := range entries {
			add(i)
			if len(selected) >= limit {
				break
			}
		}
	}
	indices := make([]int, 0, len(selected))
	for idx := range selected {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	out := make([]AnswerSupportEntry, 0, len(indices))
	for _, idx := range indices {
		out = append(out, entries[idx])
	}
	return out
}

func callChainTerminalEndpointHints(endpoints []string) []string {
	if len(endpoints) == 0 {
		return nil
	}
	last := strings.TrimSpace(endpoints[len(endpoints)-1])
	if last == "" {
		return nil
	}
	return []string{last}
}

// CallChainTerminalEndpointHints exposes the support-lane endpoint contract to
// runtime loop controllers. It intentionally delegates to the same private
// helper used by answer support lanes so explorer/finalizer guidance cannot
// drift into separate endpoint semantics.
func CallChainTerminalEndpointHints(endpoints []string) []string {
	return callChainTerminalEndpointHints(endpoints)
}

func callChainEndpointCoverage(entries []AnswerSupportEntry, endpoints []string) int {
	if len(entries) == 0 || len(endpoints) == 0 {
		return 0
	}
	covered := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		for _, entry := range entries {
			if callChainEntryMentionsEndpoint(entry, endpoint) {
				covered[strings.ToLower(endpoint)] = true
				break
			}
		}
	}
	return len(covered)
}

// CallChainEndpointCoverage counts how many requested endpoints are mentioned by
// support-lane entries using the same compatibility rule as the call-chain
// support renderer.
func CallChainEndpointCoverage(entries []AnswerSupportEntry, endpoints []string) int {
	return callChainEndpointCoverage(entries, endpoints)
}

func callChainEntryMentionsEndpoint(entry AnswerSupportEntry, endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false
	}
	haystacks := []string{entry.Text, entry.Location}
	for _, haystack := range haystacks {
		if callChainEndpointCompatible(haystack, endpoint) {
			return true
		}
	}
	return false
}

// CallChainEntryMentionsEndpoint is the exported runtime-safe form of
// callChainEntryMentionsEndpoint. It exists so loop controllers can consume the
// same typed support-lane endpoint semantics without duplicating matcher logic.
func CallChainEntryMentionsEndpoint(entry AnswerSupportEntry, endpoint string) bool {
	return callChainEntryMentionsEndpoint(entry, endpoint)
}

func callChainEndpointCompatible(candidate, endpoint string) bool {
	candidate = strings.TrimSpace(candidate)
	endpoint = strings.TrimSpace(endpoint)
	if candidate == "" || endpoint == "" {
		return false
	}
	if AnswerCodeIdentitySurfacesCompatible(candidate, endpoint) || AnswerCodeSurfaceAppearsInText(candidate, endpoint) {
		return true
	}
	return false
}

// CallChainEndpointCompatible reports whether a symbol/location surface can
// stand for a requested call-chain endpoint. It is intentionally broad enough to
// handle exact qualified/short names from multiple languages, but it never
// accepts prefix siblings (for example RunWith for Run) and never inspects user
// prose; callers pass typed endpoint and evidence surfaces.
func CallChainEndpointCompatible(candidate, endpoint string) bool {
	return callChainEndpointCompatible(candidate, endpoint)
}

func callChainRequestedEndpointHints(rm RequestModel) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || callChainEndpointHintLooksLikePath(raw) {
			return
		}
		key := strings.ToLower(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, raw)
	}
	// The ordered profile is also the strongest identity pair. Its order is
	// preserved here for presentation, while directional hard gates call
	// CallChainOrderedEndpointHints explicitly.
	if source, sink, ok := CallChainOrderedEndpointHints(rm); ok {
		add(source)
		add(sink)
		return out
	}
	if rm.CallChainEndpointProfile.DiscoverSinkActive() || rm.CallChainEndpointProfile.DiscoverTerminalActive() {
		add(rm.CallChainEndpointProfile.Source)
		return out
	}
	// ExactTargets is the analyzer's strongest endpoint identity carrier.
	// Prefer it when present so contextual MentionedEntities cannot displace a
	// model-declared source or sink. Path-like exact targets are discarded by
	// add; if no symbol target remains, fall back to the progressively broader
	// typed entity lanes below.
	for _, entity := range rm.AnalyzerHints.ExactTargets {
		add(entity)
	}
	if len(out) == 0 {
		for _, entity := range rm.AnalyzerHints.MentionedEntities {
			add(entity)
		}
	}
	if len(out) == 0 {
		for _, entity := range rm.AnalyzerHints.PrimaryEntities {
			add(entity)
		}
	}
	if len(out) == 0 {
		for _, entity := range rm.AnalyzerHints.Entities {
			add(entity)
		}
	}
	return out
}

// CallChainRequestedEndpointHints extracts endpoint-like identifiers from the
// analyzer RequestModel using the same priority order as the final answer
// support-lane compiler.
func CallChainRequestedEndpointHints(rm RequestModel) []string {
	return callChainRequestedEndpointHints(rm)
}

func callChainEndpointHintLooksLikePath(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)))
	if lower == "" || strings.Contains(lower, "/") {
		return true
	}
	return HasCodeOrConfigPathSuffix(lower)
}

func callChainStepSupportText(anchor StepSurfaceAnchor) string {
	name := strings.TrimSpace(anchor.Name)
	text := strings.TrimSpace(anchor.SurfaceText)
	if text == "" {
		text = strings.TrimSpace(anchor.Rationale)
	}
	switch {
	case name == "" && text == "":
		return ""
	case name == "":
		return text
	case text == "":
		return fmt.Sprintf("`%s` is one grounded hop in the resolved sequence.", name)
	default:
		return fmt.Sprintf("`%s` — %s", name, text)
	}
}

func callChainSupportEvidenceItems(plan *AnswerSurfacePlan) []EvidenceItem {
	if plan == nil {
		return nil
	}
	if len(plan.DriftBoundedSurfaceItems) > 0 {
		return DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems)
	}
	if len(plan.SurfaceEvidence) == 0 {
		return nil
	}
	candidateIDs := callChainPrincipalCandidateIDs(plan.FacetCoverage)
	out := make([]EvidenceItem, 0, len(plan.SurfaceEvidence))
	for _, item := range plan.SurfaceEvidence {
		if len(candidateIDs) > 0 {
			id := strings.TrimSpace(item.ID)
			if id == "" || !candidateIDs[id] {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func callChainPrincipalCandidateIDs(facets *FacetCoverageContract) map[string]bool {
	if facets == nil {
		return nil
	}
	out := make(map[string]bool)
	collect := func(req FacetRequirement) {
		switch req.Kind {
		case FacetPrincipalPathEdge, FacetCurrentCodePath:
		default:
			return
		}
		for _, id := range req.SourceCandidate {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = true
			}
		}
	}
	for _, req := range facets.Required {
		collect(req)
	}
	for _, req := range facets.Optional {
		collect(req)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compileCallChainUncertaintySupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneUncertaintyBound,
		Title:         "Call-chain boundary disclosures",
		AllowedBlocks: []string{"summary", "caveat"},
		Guidance: "Use this lane only to disclose runtime/current-code drift, incomplete chain proof, " +
			"or scope limits. Do not turn these entries into ordered-list hops or diagram edges.",
	}
	if plan == nil {
		return lane
	}
	for _, anchor := range plan.LogSourceDriftAnchors {
		file := strings.TrimSpace(anchor.File)
		if file == "" || anchor.ObservedLine <= 0 || anchor.AnchoredLine <= 0 {
			continue
		}
		funcLabel := strings.TrimSpace(firstNonEmptySurfaceString(anchor.Func, anchor.OriginalFunc))
		text := fmt.Sprintf("observed chain frame %s:%d", file, anchor.ObservedLine)
		if funcLabel != "" {
			text += fmt.Sprintf(" in %s", funcLabel)
		}
		text += fmt.Sprintf(" maps to current grounded anchor %s:%d", file, anchor.AnchoredLine)
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: fmt.Sprintf("%s:%d", file, anchor.AnchoredLine),
		})
		if len(lane.Entries) >= 3 {
			break
		}
	}
	return lane
}

func callChainPathItemEligible(item EvidenceItem) bool {
	switch item.Kind {
	case EvidenceDirect, EvidenceConditional, EvidenceRegistration, EvidenceMechanism, EvidenceRelationship:
	default:
		return false
	}
	switch item.AnchorKind {
	case AnchorCall, AnchorCallback, AnchorArgument, AnchorDefinition, AnchorCondition, AnchorAssignment, AnchorInitializer, AnchorReturn:
		return item.GroundingStatus != GroundingUngrounded
	default:
		return false
	}
}
