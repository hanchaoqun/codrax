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
		Guidance: "Use this lane for the principal ordered path and any sequence diagram. " +
			"Preserve the hop order already established by these entries. Do not add nearby helpers, " +
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

func selectCallChainSupportEntries(rm RequestModel, plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil {
		return nil
	}
	endpoints := callChainRequestedEndpointHints(rm)
	stepEntries := callChainStepBackboneEntries(plan)
	evidenceEntries := callChainSurfaceEvidenceEntries(rm, plan)
	if callChainPreferSurfaceEvidence(rm, stepEntries, evidenceEntries) {
		return callChainCondenseSupportEntries(evidenceEntries, endpoints, callChainSupportEntryLimit)
	}
	if len(stepEntries) > 0 {
		return callChainCondenseSupportEntries(stepEntries, endpoints, callChainSupportEntryLimit)
	}
	return callChainCondenseSupportEntries(evidenceEntries, endpoints, callChainSupportEntryLimit)
}

func callChainStepBackboneEntries(plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil || len(plan.StepBackbone) == 0 {
		return nil
	}
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
		out = append(out, entry)
	}
	return out
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
	detail := strings.TrimSpace(item.Summary)
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
		raw = strings.TrimSpace(raw[:max]) + "..."
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

func callChainEndpointCompatible(candidate, endpoint string) bool {
	candidate = strings.TrimSpace(candidate)
	endpoint = strings.TrimSpace(endpoint)
	if candidate == "" || endpoint == "" {
		return false
	}
	cLower := strings.ToLower(candidate)
	eLower := strings.ToLower(endpoint)
	if strings.Contains(cLower, eLower) {
		return true
	}
	cTail := normalizedSurfaceSymbolTail(candidate)
	eTail := normalizedSurfaceSymbolTail(endpoint)
	if cTail == "" || eTail == "" {
		return false
	}
	return cTail == eTail ||
		strings.HasPrefix(cTail, eTail) ||
		strings.HasPrefix(eTail, cTail)
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
	for _, entity := range rm.AnalyzerHints.MentionedEntities {
		add(entity)
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
	case AnchorCall, AnchorDefinition, AnchorCondition, AnchorAssignment, AnchorInitializer, AnchorReturn:
		return item.GroundingStatus != GroundingUngrounded
	default:
		return false
	}
}
