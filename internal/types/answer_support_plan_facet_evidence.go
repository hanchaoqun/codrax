package types

import (
	"sort"
	"strconv"
	"strings"
)

// compileFacetEvidenceSupportPlan compiles the generic per-family
// support lanes used by every non-root-cause / non-call-chain family.
// The plan composes:
//
//   - an observation lane (only when FacetObservedArtifactFact source
//     candidates exist on the active facet coverage),
//   - a principal-evidence lane keyed by the family-specific facet
//     kinds, and
//   - a facet-uncertainty lane for absence / drift / authority caveats.
//
// Returns nil when none of the lanes have entries so downstream
// renderers can skip the section entirely.
func compileFacetEvidenceSupportPlan(family QuestionFamily, rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	out := &AnswerSupportPlan{Family: family}
	if family == QFEnumeration {
		out.PrincipalMemberCoverage = principalMemberCoveragePolicyForEnumeration(rm)
	}
	if len(supportFacetCandidateIDs(plan, FacetObservedArtifactFact)) > 0 {
		if lane := compileObservedArtifactSupportLane(rm, plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	aggregatePrincipal := false
	if family == QFEnumeration {
		if lane := compileAggregateMemberSupportLane(rm, plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
			aggregatePrincipal = true
		}
	}
	if !aggregatePrincipal {
		if lane := compilePrincipalEvidenceSupportLane(family, rm, plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	if family == QFArchitecture {
		if lane := compileArchitectureRoleOutputSupportLane(plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	if family == QFEnumeration {
		if lane := compileEnumerationSupportingContextLane(rm, plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	if lane := compileFacetUncertaintySupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

func compileChangeImpactAggregateMemberSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLanePrincipalEvidence,
		Title:         "Model-emitted affected source member set",
		AllowedBlocks: principalEvidenceAllowedBlocks(QFEnumeration),
		Guidance: "Use this lane as the principal member slate for this change-impact answer. " +
			"Every entry comes from the investigator's structured `emit_investigation_complete.aggregate_facts` member_set payload, not from raw thinking or closure prose. " +
			"For requested_output=files, file paths are the principal item labels and multiple file:line entries for the same file are equivalent supporting anchors. " +
			"For requested_output=sites, the cited source location is the principal item label. " +
			"Descriptive change-type text may enrich the same row, but it is not a call edge, assignment edge, or symbol-definition claim unless another typed evidence lane independently says so. " +
			"Do not add helper symbols, adjacent context, or search hints as extra principal members.",
	}
	if plan == nil || rm.ChangeImpactProfile == nil || !rm.ChangeImpactProfile.Active() {
		return lane
	}
	switch rm.ChangeImpactProfile.RequestedOutput {
	case ImpactOutputFiles, ImpactOutputSites:
	default:
		return lane
	}
	supportEvidence := changeImpactSupportEvidenceByLocation(plan.SurfaceEvidence)
	seen := map[string]bool{}
	requireTargetProof := plan.StableAbsent
	for factIdx, fact := range plan.StableAggregateFacts {
		if fact.Kind != AnswerAggregateMemberSet || len(fact.Members) == 0 {
			continue
		}
		for memberIdx, member := range fact.Members {
			entry, ok := changeImpactAggregateMemberSupportEntry(rm.ChangeImpactProfile, fact, factIdx, memberIdx, member, supportEvidence, requireTargetProof)
			if !ok {
				continue
			}
			key := strings.ToLower(entry.Location) + "\x00" + strings.ToLower(entry.Text)
			if seen[key] {
				continue
			}
			seen[key] = true
			lane.Entries = append(lane.Entries, entry)
			if len(lane.Entries) >= facetSupportEntryLimitDefault {
				return lane
			}
		}
	}
	return lane
}

func compileAggregateMemberSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return compileChangeImpactAggregateMemberSupportLane(rm, plan)
	}
	return compileGenericAggregateMemberSupportLane(rm, plan)
}

func compileGenericAggregateMemberSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLanePrincipalEvidence,
		Title:         "Model-emitted enumeration member set",
		AllowedBlocks: principalEvidenceAllowedBlocks(QFEnumeration),
		Guidance: "Use this lane as the principal member slate for this exhaustive enumeration answer. " +
			"Every entry comes from the investigator's structured `emit_investigation_complete.aggregate_facts` member_set payload, not from raw thinking, closure prose, grep memory, or analyzer search hints. " +
			"Preserve all entries as the user-visible member set. If an entry carries a source location or member-specific support_ref, cite that anchor; otherwise treat the entry as a model-authored aggregate member label and do not invent a citation or helper symbol for it. " +
			"Supporting evidence may enrich why a member qualifies, but it must not add extra principal members.",
	}
	if plan == nil || !enumerationRequestRequiresPrincipalMemberCoverage(rm) {
		return lane
	}
	seen := map[string]bool{}
	for _, set := range CompileEnumerationDisplaySets(&rm, plan) {
		for _, row := range set.Rows {
			entry := enumerationDisplayRowSupportEntry(row)
			key := strings.ToLower(strings.TrimSpace(entry.Text)) + "\x00" + strings.ToLower(strings.TrimSpace(entry.Location))
			if seen[key] {
				continue
			}
			seen[key] = true
			lane.Entries = append(lane.Entries, entry)
		}
	}
	return lane
}

func genericAggregateMemberSupportEntry(fact AnswerAggregateFact, factIdx, memberIdx int, member string, supportEvidence *aggregateMemberEvidenceIndex, origins []AnswerEvidenceOrigin) (AnswerSupportEntry, bool) {
	member = strings.TrimSpace(member)
	if member == "" {
		return AnswerSupportEntry{}, false
	}
	allowCurrentSourceLocations := aggregateMemberFactAllowsCurrentSourceLocations(origins)
	var source string
	var line int
	var location string
	var ev EvidenceItem
	var hasEvidence bool
	if allowCurrentSourceLocations {
		source, line, location = aggregateMemberStructuredLocation(fact, memberIdx, member)
		ev, hasEvidence = aggregateMemberEvidenceByLabel(member, supportEvidence)
		if location == "" {
			if genericEv, genericLocation, ok := aggregateMemberEvidenceByGenericSupportRefs(fact, member, supportEvidence); ok {
				ev = genericEv
				hasEvidence = true
				source = genericLocation.File
				line = genericLocation.LineStart
				location = aggregateMemberStartLocation(genericLocation)
			}
		}
		if location == "" {
			if hasEvidence {
				source = strings.TrimSpace(strings.ReplaceAll(ev.Source, `\`, `/`))
				line = ev.LineStart
				location = aggregateMemberStartLocation(AnswerSourceLocationSurface{File: source, LineStart: line})
			}
		}
		if !hasEvidence && location != "" {
			ev, hasEvidence = aggregateMemberEvidenceByLocationAndText(member, source, line, supportEvidence)
		}
		if hasEvidence && location != "" && line > 0 &&
			ev.LineStart == line &&
			aggregateSupportRefPathCorresponds(ev.Source, source) &&
			aggregateSupportRefMoreSpecific(ev.Source, source) {
			source = strings.TrimSpace(strings.ReplaceAll(ev.Source, `\`, `/`))
			line = ev.LineStart
			location = aggregateMemberStartLocation(AnswerSourceLocationSurface{File: source, LineStart: line})
		}
	}
	displayMember := member
	if label, _, ok := aggregateSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		displayMember = strings.TrimSpace(label)
	}
	surface := aggregateMemberSurface(member, location)
	entry := AnswerSupportEntry{
		Text:          member,
		Detail:        "source=emit_investigation_complete.aggregate_facts; aggregate_kind=member_set; member_surface=" + string(surface),
		Location:      location,
		EvidenceID:    aggregateMemberEvidenceID(factIdx, memberIdx),
		ClaimForm:     ClaimUnknown,
		LabelSurface:  ClaimLabelSurfaceDisplayLabel,
		SurfaceTerms:  dedupeAggregateMemberTerms([]string{displayMember, member, location, source}),
		Source:        source,
		LineStart:     line,
		MemberSurface: surface,
		Producer:      "explorer.emit_investigation_complete.aggregate_facts",
	}
	if location != "" {
		entry.GroundingTier = TierLineText
	}
	if hasEvidence && sameAggregateMemberEvidenceLocation(ev, source, line) {
		applyAggregateMemberEvidenceToSupportEntry(&entry, ev)
		alignAggregateMemberSurfaceWithClaimForm(&entry)
	} else if hasEvidence && aggregateMemberEvidenceCanCoAnchor(member, ev, supportEvidence, source, line) {
		evLocation := aggregateMemberStartLocation(AnswerSourceLocationSurface{File: strings.TrimSpace(strings.ReplaceAll(ev.Source, `\`, `/`)), LineStart: ev.LineStart})
		entry.EquivalentLocations = appendAggregateMemberEquivalentLocation(entry.EquivalentLocations, evLocation)
		entry.SurfaceTerms = dedupeAggregateMemberTerms(append(entry.SurfaceTerms, aggregateEvidenceMemberLabels(ev)...))
		if form := ClaimFormOf(ev); form != ClaimUnknown {
			entry.Detail = strings.TrimSpace(entry.Detail + "; equivalent_anchor=" + evLocation + "; equivalent_claim_form=" + string(form) + "; equivalent_anchor_kind=" + string(ev.AnchorKind))
		}
		if ev.LineEnd > ev.LineStart {
			entry.EquivalentLocations = appendAggregateMemberEquivalentLocation(
				entry.EquivalentLocations,
				aggregateMemberStartLocation(AnswerSourceLocationSurface{File: strings.TrimSpace(strings.ReplaceAll(ev.Source, `\`, `/`)), LineStart: ev.LineEnd}),
			)
		}
	}
	return entry, true
}

func aggregateMemberFactAllowsCurrentSourceLocations(origins []AnswerEvidenceOrigin) bool {
	if len(origins) == 0 {
		return true
	}
	for _, origin := range origins {
		switch origin {
		case AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginUnknown:
			return true
		}
	}
	return false
}

func alignAggregateMemberSurfaceWithClaimForm(entry *AnswerSupportEntry) {
	if entry == nil {
		return
	}
	if entry.MemberSurface == PrincipalMemberSurfaceSourceLocation {
		return
	}
	if entry.ClaimForm.UsesNonSymbolLabelSurface() {
		entry.MemberSurface = PrincipalMemberSurfaceDisplayLabel
	}
}

type aggregateMemberEvidenceIndex struct {
	byKey      map[string]EvidenceItem
	byKeyCount map[string]int
	items      []EvidenceItem
}

func genericAggregateSupportEvidenceByMemberLabel(items []EvidenceItem) *aggregateMemberEvidenceIndex {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]EvidenceItem, len(items))
	counts := make(map[string]int, len(items))
	var indexed []EvidenceItem
	for _, item := range items {
		if item.Source == "" || item.LineStart <= 0 {
			continue
		}
		if item.GroundingStatus == GroundingUngrounded {
			continue
		}
		indexed = append(indexed, item)
		itemKeys := map[string]bool{}
		for _, label := range aggregateEvidenceMemberLabels(item) {
			key := aggregateMemberKey(label)
			if key == "" {
				continue
			}
			itemKeys[key] = true
			if existing, ok := out[key]; ok {
				if aggregateMemberEvidenceSameAnchor(existing, item) {
					out[key] = mergeAggregateMemberEvidenceSameAnchor(existing, item)
				} else if aggregateMemberEvidencePrefersAnchor(item, existing) {
					out[key] = item
				}
				continue
			}
			out[key] = item
		}
		for key := range itemKeys {
			counts[key]++
		}
	}
	if len(indexed) == 0 {
		return nil
	}
	return &aggregateMemberEvidenceIndex{byKey: out, byKeyCount: counts, items: indexed}
}

func aggregateMemberEvidenceByLabel(member string, support *aggregateMemberEvidenceIndex) (EvidenceItem, bool) {
	if support == nil || len(support.items) == 0 {
		return EvidenceItem{}, false
	}
	if label, _, ok := aggregateSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		if ev, found := aggregateMemberEvidenceByLabel(label, support); found {
			return ev, true
		}
	}
	if ev, found := aggregateMemberEvidenceByExactIdentity(member, support.items); found {
		return ev, true
	}
	if left, right, ok := AnswerAggregateMemberRelationParts(member); ok {
		return aggregateRelationMemberEvidence(left, right, support.items)
	}
	for _, candidate := range aggregateMemberDisplayCandidates(member) {
		key := aggregateMemberKey(candidate)
		if key == "" {
			continue
		}
		if support.byKeyCount[key] != 1 {
			continue
		}
		if ev, ok := support.byKey[key]; ok {
			return ev, true
		}
	}
	return EvidenceItem{}, false
}

func aggregateMemberEvidenceByExactIdentity(member string, items []EvidenceItem) (EvidenceItem, bool) {
	memberKeys := aggregateMemberIdentityKeys(member)
	if len(memberKeys) == 0 || len(items) == 0 {
		return EvidenceItem{}, false
	}
	bestRank := -1
	bestIdentityScore := -1
	var best EvidenceItem
	found := false
	for _, item := range items {
		identityScore := aggregateEvidenceIdentityMatchScore(item, memberKeys)
		if identityScore < 0 {
			continue
		}
		if found && identityScore < bestIdentityScore {
			continue
		}
		rank := changeImpactSupportEvidenceRank(item)
		if found && identityScore > bestIdentityScore {
			best = item
			bestRank = rank
			bestIdentityScore = identityScore
			continue
		}
		if found && rank < bestRank {
			continue
		}
		if found && rank == bestRank && aggregateMemberEvidenceSameAnchor(best, item) {
			best = mergeAggregateMemberEvidenceSameAnchor(best, item)
			continue
		}
		if found && rank == bestRank && !aggregateMemberEvidencePrefersAnchor(item, best) {
			continue
		}
		best = item
		bestRank = rank
		bestIdentityScore = identityScore
		found = true
	}
	return best, found
}

func aggregateEvidenceIdentityMatchScore(item EvidenceItem, keys map[string]bool) int {
	if len(keys) == 0 {
		return -1
	}
	scoreFor := func(raw string, score int) int {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return -1
		}
		if keys["literal:"+strings.ToLower(raw)] {
			return score
		}
		if key := AnswerAggregateMemberSurfaceKey(raw); key != "" && keys[strings.ToLower(key)] {
			return score
		}
		if aggregateEvidenceRawPathSuffixMatches(raw, keys) {
			return score - 5
		}
		return -1
	}
	best := -1
	for _, candidate := range []struct {
		value string
		score int
	}{
		{item.AnchorSymbol, 100},
		{item.Object, 90},
		{item.OwnerSymbol, 70},
		{item.Subject, 60},
	} {
		if score := scoreFor(candidate.value, candidate.score); score > best {
			best = score
		}
	}
	for _, term := range item.SurfaceTerms {
		if score := scoreFor(term, 80); score > best {
			best = score
		}
	}
	return best
}

func aggregateMemberEvidencePrefersAnchor(candidate, existing EvidenceItem) bool {
	candidateRank := changeImpactSupportEvidenceRank(candidate)
	existingRank := changeImpactSupportEvidenceRank(existing)
	if candidateRank != existingRank {
		return candidateRank > existingRank
	}
	if candidate.LoadBearingSummary && !existing.LoadBearingSummary {
		return true
	}
	if candidate.Salience.IsSet() && (!existing.Salience.IsSet() || candidate.Salience.Rank() < existing.Salience.Rank()) {
		return true
	}
	return false
}

func aggregateMemberEvidenceSameAnchor(a, b EvidenceItem) bool {
	if a.ID != "" && b.ID != "" && a.ID == b.ID {
		return true
	}
	if strings.TrimSpace(a.Source) == "" || strings.TrimSpace(b.Source) == "" || a.LineStart <= 0 || b.LineStart <= 0 {
		return false
	}
	return normalizeAnswerSupportPath(a.Source) == normalizeAnswerSupportPath(b.Source) && a.LineStart == b.LineStart
}

func mergeAggregateMemberEvidenceSameAnchor(dst, src EvidenceItem) EvidenceItem {
	merged := mergeExactResolutionSurfaceEvidence(dst, src)
	merged.Summary = MergeEvidenceSummaries(dst.Summary, src.Summary)
	return merged
}

func aggregateMemberIdentityKeys(member string) map[string]bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	out := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		out["literal:"+strings.ToLower(raw)] = true
		if key := AnswerAggregateMemberSurfaceKey(raw); key != "" {
			out[strings.ToLower(key)] = true
		}
		if suffix := aggregateMemberPathSuffixIdentity(raw); suffix != "" {
			out["pathsuffix:"+suffix] = true
		}
	}
	for _, candidate := range aggregateMemberDisplayCandidates(member) {
		add(candidate)
	}
	add(member)
	if len(out) == 0 {
		return nil
	}
	return out
}

func aggregateMemberPathSuffixIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`\"'")
	raw = strings.ReplaceAll(raw, `\`, `/`)
	for strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") || !strings.Contains(raw, "/") {
		return ""
	}
	if strings.Contains(raw, "://") {
		return ""
	}
	return strings.ToLower(raw)
}

func aggregateEvidenceRawPathSuffixMatches(raw string, keys map[string]bool) bool {
	candidate := aggregateMemberPathSuffixIdentity(raw)
	if candidate == "" {
		return false
	}
	for key := range keys {
		suffix, ok := strings.CutPrefix(key, "pathsuffix:")
		if !ok || suffix == "" {
			continue
		}
		if candidate == suffix || strings.HasSuffix(candidate, "/"+suffix) {
			return true
		}
	}
	return false
}

func aggregateEvidenceIdentityMatchesAny(item EvidenceItem, keys map[string]bool) bool {
	if len(keys) == 0 {
		return false
	}
	for _, label := range aggregateEvidenceMemberLabels(item) {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if keys["literal:"+strings.ToLower(label)] {
			return true
		}
		if key := AnswerAggregateMemberSurfaceKey(label); key != "" && keys[strings.ToLower(key)] {
			return true
		}
	}
	return false
}

func aggregateMemberEvidenceByGenericSupportRefs(fact AnswerAggregateFact, member string, support *aggregateMemberEvidenceIndex) (EvidenceItem, AnswerSourceLocationSurface, bool) {
	if support == nil || len(support.items) == 0 {
		return EvidenceItem{}, AnswerSourceLocationSurface{}, false
	}
	bestScore := -1
	var best EvidenceItem
	var bestLocation AnswerSourceLocationSurface
	found := false
	for _, ref := range fact.SupportRefs {
		refMember, refLocation, ok := aggregateSupportRefMemberLocation(ref)
		if !ok || !AnswerSupportRefLabelIsGeneric(refMember) {
			continue
		}
		ev, ok := aggregateMemberEvidenceByLocationAndText(member, refLocation.File, refLocation.LineStart, support)
		if !ok {
			continue
		}
		score := aggregateGenericSupportRefEvidenceScore(member, ev, refLocation)
		if score <= bestScore {
			continue
		}
		best = ev
		bestLocation = refLocation
		bestScore = score
		found = true
	}
	return best, bestLocation, found
}

func aggregateGenericSupportRefEvidenceScore(member string, ev EvidenceItem, refLocation AnswerSourceLocationSurface) int {
	if strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 {
		return -1
	}
	score := changeImpactSupportEvidenceRank(ev) * 100
	if normalizeAnswerSupportPath(ev.Source) == normalizeAnswerSupportPath(refLocation.File) &&
		ev.LineStart == refLocation.LineStart {
		score += 1000
	}
	if aggregateEvidenceTextSupportsMember(ev, member) {
		score += 300
	}
	if aggregateEvidenceIdentityMatchesAny(ev, aggregateMemberIdentityKeys(member)) {
		score += 250
	}
	if form := ClaimFormOf(ev); form == ClaimDefinitionFact {
		score += 180
	} else if form != ClaimUnknown {
		score += 60
	}
	if ev.AnchorKind == AnchorDefinition {
		score += 120
	}
	return score
}

func aggregateMemberEvidenceByLocationAndText(member string, source string, line int, support *aggregateMemberEvidenceIndex) (EvidenceItem, bool) {
	if support == nil || len(support.items) == 0 || strings.TrimSpace(source) == "" || line <= 0 {
		return EvidenceItem{}, false
	}
	source = normalizeAnswerSupportPath(source)
	for _, item := range support.items {
		if normalizeAnswerSupportPath(item.Source) != source || item.LineStart != line {
			continue
		}
		if aggregateEvidenceTextSupportsMember(item, member) {
			return item, true
		}
	}
	return EvidenceItem{}, false
}

func aggregateEvidenceTextSupportsMember(item EvidenceItem, member string) bool {
	text := strings.TrimSpace(item.Snippet)
	if item.LoadBearingSummary {
		text = strings.TrimSpace(text + "\n" + item.Summary)
	}
	if text == "" {
		return false
	}
	for _, candidate := range aggregateMemberDisplayCandidates(member) {
		if AnswerSupportRefLabelIsGeneric(candidate) {
			continue
		}
		if AnswerCodeSurfaceAppearsInText(text, candidate) {
			return true
		}
	}
	return false
}

func aggregateRelationMemberEvidence(left, right string, items []EvidenceItem) (EvidenceItem, bool) {
	var best EvidenceItem
	bestRank := -1
	found := false
	for _, item := range items {
		if !aggregateEvidenceSupportsRelationMember(item, left, right) {
			continue
		}
		rank := changeImpactSupportEvidenceRank(item)
		if found && rank <= bestRank {
			continue
		}
		best = item
		bestRank = rank
		found = true
	}
	return best, found
}

func aggregateEvidenceSupportsRelationMember(ev EvidenceItem, left, right string) bool {
	return aggregateEvidenceEndpointSupportsRelationRight(ev, right) &&
		aggregateEvidenceOrSourceSupportsRelationLeft(ev, left)
}

func aggregateEvidenceOrSourceSupportsRelationLeft(ev EvidenceItem, left string) bool {
	if aggregateEvidenceEndpointSupportsRelationLeft(ev, left) {
		return true
	}
	return aggregatePathSegmentsSupportToken(ev.Source, left)
}

func aggregateEvidenceEndpointSupportsRelationLeft(ev EvidenceItem, left string) bool {
	left = trimAggregateMemberSurface(left)
	if left == "" {
		return false
	}
	for _, label := range aggregateEvidenceMemberLabels(ev) {
		label = trimAggregateMemberSurface(label)
		if label == "" {
			continue
		}
		if strings.EqualFold(label, left) {
			return true
		}
		if candidateLeft, _, ok := AnswerAggregateMemberRelationParts(label); ok && strings.EqualFold(candidateLeft, left) {
			return true
		}
	}
	return false
}

func aggregateEvidenceEndpointSupportsRelationRight(ev EvidenceItem, right string) bool {
	candidates := aggregateRelationRightMatchCandidates(right)
	if len(candidates) == 0 {
		return false
	}
	for _, label := range aggregateEvidenceMemberLabels(ev) {
		if aggregateRelationLabelMatchesAny(label, candidates) {
			return true
		}
		if _, candidateRight, ok := AnswerAggregateMemberRelationParts(label); ok &&
			aggregateRelationLabelMatchesAny(candidateRight, candidates) {
			return true
		}
	}
	return false
}

func aggregateRelationRightMatchCandidates(right string) []string {
	right = trimAggregateMemberSurface(right)
	if right == "" {
		return nil
	}
	out := []string{right}
	out = append(out, aggregateRelationPartDisplayForms(right)...)
	if base, _, ok := aggregateRelationPartDecorator(right); ok {
		out = append(out, base)
	}
	if tail := aggregateRelationPartDisplayTail(right); tail != "" {
		out = append(out, tail)
	}
	if tail := aggregateRelationPartTail(right); tail != "" {
		out = append(out, tail)
	}
	return dedupeAggregateMemberTerms(out)
}

func aggregateRelationLabelMatchesAny(label string, candidates []string) bool {
	label = trimAggregateMemberSurface(label)
	if label == "" {
		return false
	}
	for _, candidate := range candidates {
		if strings.EqualFold(label, candidate) {
			return true
		}
	}
	labelTail := NormalizedSurfaceSymbolTail(label)
	if labelTail == "" {
		return false
	}
	for _, candidate := range candidates {
		if candidateTail := NormalizedSurfaceSymbolTail(candidate); candidateTail != "" && labelTail == candidateTail {
			return true
		}
	}
	return false
}

func aggregatePathSegmentsSupportToken(path string, token string) bool {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	token = trimAggregateMemberSurface(token)
	if path == "" || token == "" {
		return false
	}
	want := strings.ToLower(token)
	for _, segment := range strings.Split(path, "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		base := segment
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		for _, candidate := range []string{segment, base} {
			if strings.ToLower(candidate) == want {
				return true
			}
		}
	}
	return false
}

func sameAggregateMemberEvidenceLocation(ev EvidenceItem, source string, line int) bool {
	source = normalizeAnswerSupportPath(source)
	if source == "" || line <= 0 {
		return false
	}
	return normalizeAnswerSupportPath(ev.Source) == source && ev.LineStart == line
}

func applyAggregateMemberEvidenceToSupportEntry(entry *AnswerSupportEntry, ev EvidenceItem) {
	if entry == nil {
		return
	}
	entry.ClaimForm = ClaimFormOf(ev)
	entry.LabelSurface = entry.ClaimForm.LabelSurfaceKind()
	entry.Subject = strings.TrimSpace(ev.Subject)
	entry.Object = strings.TrimSpace(ev.Object)
	entry.AnchorSymbol = strings.TrimSpace(ev.AnchorSymbol)
	entry.OwnerSymbol = strings.TrimSpace(ev.OwnerSymbol)
	entry.AnchorKind = ev.AnchorKind
	entry.LineEnd = ev.LineEnd
	entry.Producer = firstNonEmptySurfaceString(strings.TrimSpace(ev.Producer), entry.Producer)
	entry.GroundingTier = ev.GroundingTier
	entry.SurfaceTerms = dedupeAggregateMemberTerms(append(entry.SurfaceTerms, aggregateEvidenceMemberLabels(ev)...))
	if entry.ClaimForm != ClaimUnknown {
		entry.Detail = strings.TrimSpace(entry.Detail + "; claim_form=" + string(entry.ClaimForm) + "; anchor_kind=" + string(ev.AnchorKind))
		if entry.ClaimForm == ClaimTextReferenceFact {
			entry.Detail += "; text_reference=true; not_definition_call_or_assignment=true"
		}
	}
}

func aggregateMemberEvidenceCanCoAnchor(member string, ev EvidenceItem, support *aggregateMemberEvidenceIndex, source string, line int) bool {
	if strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 {
		return false
	}
	if sameAggregateMemberEvidenceLocation(ev, source, line) {
		return false
	}
	if normalizeAnswerSupportPath(ev.Source) == normalizeAnswerSupportPath(source) {
		return true
	}
	return aggregateMemberEvidenceUniqueForMember(member, ev, support)
}

func aggregateMemberEvidenceUniqueForMember(member string, ev EvidenceItem, support *aggregateMemberEvidenceIndex) bool {
	if support == nil || len(support.byKey) == 0 {
		return false
	}
	for _, candidate := range aggregateMemberDisplayCandidates(member) {
		key := aggregateMemberKey(candidate)
		if key == "" || support.byKeyCount[key] != 1 {
			continue
		}
		best, ok := support.byKey[key]
		if !ok {
			continue
		}
		if sameAggregateEvidenceIdentity(best, ev) {
			return true
		}
	}
	return false
}

func sameAggregateEvidenceIdentity(a, b EvidenceItem) bool {
	if strings.TrimSpace(a.ID) != "" && strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) {
		return true
	}
	return normalizeAnswerSupportPath(a.Source) == normalizeAnswerSupportPath(b.Source) &&
		a.LineStart > 0 &&
		a.LineStart == b.LineStart
}

func appendAggregateMemberEquivalentLocation(in []string, location string) []string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return in
	}
	key := normalizeAnswerSupportLocation(location)
	for _, existing := range in {
		if normalizeAnswerSupportLocation(existing) == key {
			return in
		}
	}
	return append(in, location)
}

func aggregateMemberStructuredLocation(fact AnswerAggregateFact, memberIdx int, member string) (source string, line int, location string) {
	if surface, ok := ParseAnswerSourceLocationSurface(member); ok {
		if precise, ok := aggregateMemberPreciseSupportRefLocation(fact, memberIdx, "", surface); ok {
			surface = precise
		}
		return surface.File, surface.LineStart, aggregateMemberStartLocation(surface)
	}
	if refMember, refLocation, ok := aggregateSupportRefMemberLocation(member); ok && strings.TrimSpace(refMember) != "" {
		if precise, ok := aggregateMemberPreciseSupportRefLocation(fact, memberIdx, refMember, refLocation); ok {
			refLocation = precise
		}
		return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
	}
	memberKey := strings.ToLower(strings.TrimSpace(member))
	memberLabel := aggregateMemberSupportSurfaceLabel(member)
	var bareRefs []AnswerSourceLocationSurface
	var genericRefs []AnswerSourceLocationSurface
	for _, ref := range fact.SupportRefs {
		refMember, refLocation, ok := aggregateSupportRefMemberLocation(ref)
		if !ok {
			continue
		}
		if refMember == "" {
			bareRefs = append(bareRefs, refLocation)
			continue
		}
		if AnswerSupportRefLabelIsGeneric(refMember) {
			genericRefs = append(genericRefs, refLocation)
			continue
		}
		if strings.ToLower(refMember) == memberKey ||
			aggregateSupportRefCanDescribeMember(refMember, memberLabel) {
			return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
		}
	}
	if len(genericRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(genericRefs) {
		refLocation := genericRefs[memberIdx]
		return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
	}
	if len(genericRefs) == 1 && len(fact.Members) == 1 {
		refLocation := genericRefs[0]
		return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
	}
	if len(bareRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(bareRefs) {
		refLocation := bareRefs[memberIdx]
		return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
	}
	if len(bareRefs) == 1 {
		refLocation := bareRefs[0]
		return refLocation.File, refLocation.LineStart, aggregateMemberStartLocation(refLocation)
	}
	return "", 0, ""
}

func aggregateMemberPreciseSupportRefLocation(fact AnswerAggregateFact, memberIdx int, memberLabel string, displayLocation AnswerSourceLocationSurface) (AnswerSourceLocationSurface, bool) {
	if displayLocation.LineStart <= 0 || strings.TrimSpace(displayLocation.File) == "" {
		return AnswerSourceLocationSurface{}, false
	}
	type candidate struct {
		location AnswerSourceLocationSurface
		index    int
	}
	var candidates []candidate
	for refIdx, ref := range fact.SupportRefs {
		refMember, refLocation, ok := aggregateSupportRefMemberLocation(ref)
		if !ok || refLocation.LineStart != displayLocation.LineStart {
			continue
		}
		if !aggregateSupportRefPathCorresponds(refLocation.File, displayLocation.File) ||
			!aggregateSupportRefMoreSpecific(refLocation.File, displayLocation.File) {
			continue
		}
		if !aggregateSupportRefMemberMatchesDisplay(refMember, memberLabel) {
			continue
		}
		candidates = append(candidates, candidate{location: refLocation, index: refIdx})
	}
	if len(candidates) == 1 {
		return candidates[0].location, true
	}
	if memberIdx >= 0 {
		for _, candidate := range candidates {
			if candidate.index == memberIdx {
				return candidate.location, true
			}
		}
	}
	return AnswerSourceLocationSurface{}, false
}

func aggregateSupportRefPathCorresponds(refFile, displayFile string) bool {
	refFile = normalizeAnswerSupportPath(refFile)
	displayFile = normalizeAnswerSupportPath(displayFile)
	if refFile == "" || displayFile == "" {
		return false
	}
	if refFile == displayFile {
		return true
	}
	return strings.HasSuffix(refFile, "/"+displayFile) ||
		strings.HasSuffix(displayFile, "/"+refFile)
}

func aggregateSupportRefMoreSpecific(refFile, displayFile string) bool {
	refFile = normalizeAnswerSupportPath(refFile)
	displayFile = normalizeAnswerSupportPath(displayFile)
	return refFile != "" && displayFile != "" &&
		refFile != displayFile &&
		len(refFile) > len(displayFile) &&
		strings.HasSuffix(refFile, "/"+displayFile)
}

func aggregateSupportRefMemberMatchesDisplay(refMember, displayLabel string) bool {
	refMember = strings.TrimSpace(refMember)
	displayLabel = strings.TrimSpace(displayLabel)
	if refMember == "" || AnswerSupportRefLabelIsGeneric(refMember) {
		return true
	}
	if displayLabel == "" {
		return false
	}
	for _, candidate := range aggregateMemberDisplayCandidates(displayLabel) {
		if strings.EqualFold(strings.TrimSpace(candidate), refMember) {
			return true
		}
	}
	return false
}

func aggregateSupportRefMemberLocation(ref string) (member string, location AnswerSourceLocationSurface, ok bool) {
	return ParseAnswerSupportRefMemberLocation(ref)
}

func aggregateMemberDisplayCandidates(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	out := AnswerAggregateMemberDisplayCandidates(member)
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		out = append(out, AnswerAggregateMemberDisplayCandidates(label)...)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			prefix := strings.TrimSpace(member[:idx])
			if prefix != "" {
				out = append(out, AnswerAggregateMemberDisplayCandidates(prefix)...)
			}
		}
	}
	return dedupeAggregateMemberTerms(out)
}

func aggregateEvidenceMemberLabels(ev EvidenceItem) []string {
	out := []string{ev.AnchorSymbol, ev.Subject, ev.Object, ev.OwnerSymbol}
	out = append(out, ev.SurfaceTerms...)
	return out
}

func aggregateMemberKey(raw string) string {
	return NormalizedSurfaceSymbolTail(raw)
}

func aggregateMemberSurface(member string, location string) AnswerPrincipalMemberSurface {
	if _, ok := ParseAnswerSourceLocationSurface(member); ok {
		return PrincipalMemberSurfaceSourceLocation
	}
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		return PrincipalMemberSurfaceDisplayLabel
	}
	if IsCodeIdentitySurface(member) {
		return PrincipalMemberSurfaceSymbolLike
	}
	return PrincipalMemberSurfaceDisplayLabel
}

func changeImpactAggregateMemberSupportEntry(profile *ChangeImpactProfile, fact AnswerAggregateFact, factIdx, memberIdx int, member string, supportEvidence map[string]EvidenceItem, requireTargetProof bool) (AnswerSupportEntry, bool) {
	if profile == nil || !profile.Active() {
		return AnswerSupportEntry{}, false
	}
	member = strings.TrimSpace(member)
	if member == "" {
		return AnswerSupportEntry{}, false
	}
	var file string
	var line int
	var location string
	var display string
	var terms []string
	if surface, ok := ParseAnswerSourceLocationSurface(member); ok {
		file = surface.File
		line = surface.LineStart
		location = aggregateMemberStartLocation(surface)
		display = strings.TrimSpace(member)
		terms = append(terms, display, file, location)
	} else if profile.RequestedOutput == ImpactOutputFiles {
		if parsedFile, ok := ParseAnswerFilePathSurface(member); ok {
			ref, refOK := changeImpactAggregateSupportLocationForFile(fact.SupportRefs, parsedFile)
			if !refOK {
				return AnswerSupportEntry{}, false
			}
			file = ref.File
			line = ref.LineStart
			location = aggregateMemberStartLocation(ref)
			display = parsedFile
			terms = append(terms, display, location)
		}
	}
	if file == "" || line <= 0 || location == "" {
		return AnswerSupportEntry{}, false
	}
	var supportItem EvidenceItem
	var hasSupportItem bool
	if ev, ok := supportEvidence[changeImpactSupportLocationKey(file, line)]; ok {
		supportItem = ev
		hasSupportItem = true
	}
	if requireTargetProof {
		target := parseChangeImpactPrincipalTarget(profile)
		if strings.TrimSpace(target.path) == "" || !hasSupportItem || !changeImpactEvidenceMatchesTarget(supportItem, nil, target) {
			return AnswerSupportEntry{}, false
		}
	}
	text := display
	if label := strings.TrimSpace(fact.Label); label != "" {
		text = label + ": " + display
	}
	detail := "source=emit_investigation_complete.aggregate_facts; aggregate_kind=member_set; member_surface=source_location"
	entry := AnswerSupportEntry{
		Text:          text,
		Detail:        detail,
		Location:      location,
		EvidenceID:    aggregateMemberEvidenceID(factIdx, memberIdx),
		ClaimForm:     ClaimUnknown,
		LabelSurface:  ClaimLabelSurfaceDisplayLabel,
		SurfaceTerms:  dedupeAggregateMemberTerms(terms),
		Source:        file,
		LineStart:     line,
		MemberSurface: PrincipalMemberSurfaceSourceLocation,
		Producer:      "explorer.emit_investigation_complete.aggregate_facts",
		GroundingTier: TierLineText,
	}
	if hasSupportItem {
		ev := supportItem
		entry.ClaimForm = ClaimFormOf(ev)
		entry.LabelSurface = entry.ClaimForm.LabelSurfaceKind()
		entry.Subject = strings.TrimSpace(ev.Subject)
		entry.Object = strings.TrimSpace(ev.Object)
		entry.AnchorSymbol = strings.TrimSpace(ev.AnchorSymbol)
		entry.OwnerSymbol = strings.TrimSpace(ev.OwnerSymbol)
		entry.AnchorKind = ev.AnchorKind
		entry.LineEnd = ev.LineEnd
		entry.Producer = firstNonEmptySurfaceString(strings.TrimSpace(ev.Producer), entry.Producer)
		entry.GroundingTier = ev.GroundingTier
		entry.SurfaceTerms = dedupeAggregateMemberTerms(append(entry.SurfaceTerms, append(ev.SurfaceTerms, ev.AnchorSymbol, ev.Subject, ev.Object)...))
		if entry.ClaimForm != ClaimUnknown {
			entry.Detail = strings.TrimSpace(entry.Detail + "; claim_form=" + string(entry.ClaimForm) + "; anchor_kind=" + string(ev.AnchorKind))
			if entry.ClaimForm == ClaimTextReferenceFact {
				entry.Detail += "; text_reference=true; not_definition_call_or_assignment=true"
			}
		}
	}
	return entry, true
}

func changeImpactSupportEvidenceByLocation(items []EvidenceItem) map[string]EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]EvidenceItem, len(items))
	for _, item := range items {
		if item.Source == "" || item.LineStart <= 0 {
			continue
		}
		if item.GroundingStatus == GroundingUngrounded {
			continue
		}
		key := changeImpactSupportLocationKey(item.Source, item.LineStart)
		if key == "" {
			continue
		}
		if existing, ok := out[key]; ok && changeImpactSupportEvidenceRank(existing) >= changeImpactSupportEvidenceRank(item) {
			continue
		}
		out[key] = item
	}
	return out
}

func changeImpactSupportEvidenceRank(item EvidenceItem) int {
	rank := 0
	if item.GroundingStatus == GroundingGrounded {
		rank += 4
	} else if item.GroundingStatus == GroundingRecovered {
		rank += 2
	}
	if strings.TrimSpace(item.Producer) != "" && item.Kind.IsLLMEmittable() {
		rank += 2
	}
	if ClaimFormOf(item) == ClaimTextReferenceFact {
		rank += 1
	}
	return rank
}

func changeImpactSupportLocationKey(file string, line int) string {
	file = normalizeAnswerLocationFile(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)))
	if file == "" || line <= 0 {
		return ""
	}
	return file + ":" + intString(line)
}

func aggregateMemberEvidenceID(factIdx, memberIdx int) string {
	return "aggregate_fact:member_set:" + strconv.Itoa(factIdx) + ":" + strconv.Itoa(memberIdx)
}

func aggregateMemberStartLocation(surface AnswerSourceLocationSurface) string {
	if surface.File == "" || surface.LineStart <= 0 {
		return ""
	}
	return surface.File + ":" + intString(surface.LineStart)
}

func changeImpactAggregateSupportLocationForFile(refs []string, wantFile string) (AnswerSourceLocationSurface, bool) {
	wantFile = normalizeAnswerSupportPath(wantFile)
	if wantFile == "" {
		return AnswerSourceLocationSurface{}, false
	}
	for _, ref := range refs {
		surface, ok := ParseAnswerSourceLocationSurface(ref)
		if !ok {
			continue
		}
		if normalizeAnswerSupportPath(surface.File) == wantFile {
			return surface, true
		}
	}
	return AnswerSourceLocationSurface{}, false
}

func dedupeAggregateMemberTerms(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, term := range in {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compilePrincipalEvidenceSupportLane(family QuestionFamily, rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLanePrincipalEvidence,
		Title:         principalEvidenceLaneTitle(family),
		AllowedBlocks: principalEvidenceAllowedBlocksForRequest(family, rm),
		Guidance:      principalEvidenceLaneGuidance(family, rm),
	}
	items := PrincipalSupportEvidenceItemsForFamily(family, plan)
	if family == QFConfigPrecedence {
		items = curateConfigPrecedencePrincipalEvidence(rm, plan, items)
	}
	if len(items) == 0 {
		return lane
	}
	limit := facetSupportEntryLimitForFamily(family, len(items))
	for _, item := range items {
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		entry := answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text))
		entry.MemberSurface = PrincipalMemberSurfaceForRequest(rm, item, items)
		entry.Detail = appendPrincipalMemberSurfaceDetail(entry.Detail, entry.MemberSurface)
		lane.Entries = append(lane.Entries, entry)
		if len(lane.Entries) >= limit {
			break
		}
	}
	return lane
}

func curateConfigPrecedencePrincipalEvidence(rm RequestModel, plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 || plan == nil {
		return items
	}
	contract := plan.ExactResolution
	if contract == nil {
		contract = BuildExactResolutionContract(rm)
	}
	if contract == nil || contract.TargetKind != SubjectConfigKey {
		return items
	}
	if plan.PreferredExactResolution == nil ||
		plan.PreferredExactResolution.Status != AnswerExactResolutionAbsent {
		return items
	}
	requested := ConfigTraceRequestedDiagramRoles(contract)
	if len(requested) == 0 {
		return items
	}
	requestedSet := make(map[EvidenceDiagramRole]bool, len(requested))
	for _, role := range requested {
		requestedSet[role] = true
	}
	bestByRole := make(map[EvidenceDiagramRole]EvidenceItem, len(requestedSet))
	bestRankByRole := make(map[EvidenceDiagramRole]int, len(requestedSet))
	var unscoped []EvidenceItem
	for _, item := range items {
		role := ConfigTraceCoverageRoleInFiles(contract, plan.ExactContextRequiredFiles, item)
		if role == EvidenceDiagramRoleUnknown || !requestedSet[role] {
			// In exact-absence config traces, generic absence proofs and
			// same-family helper anchors belong to the exact-resolution /
			// uncertainty lanes. They are not additional user-requested
			// precedence rows unless they carry a validated requested role.
			continue
		}
		rank := configPrecedencePrincipalRoleRank(role, item)
		if existingRank, ok := bestRankByRole[role]; ok && existingRank <= rank {
			continue
		}
		bestByRole[role] = item
		bestRankByRole[role] = rank
	}
	for _, role := range requested {
		if item, ok := bestByRole[role]; ok {
			unscoped = append(unscoped, item)
		}
	}
	if len(unscoped) == 0 {
		return nil
	}
	return unscoped
}

func configPrecedencePrincipalRoleRank(role EvidenceDiagramRole, item EvidenceItem) int {
	rank := facetSupportEvidencePriority(QFConfigPrecedence, item) * 100
	if item.GroundingStatus == GroundingGrounded {
		rank -= 20
	}
	if item.DiagramRole == role || CanonicalEvidenceDiagramRole(string(item.RequestedDiagramRole)) == role {
		rank -= 16
	}
	if item.ContextRole == EvidenceContextRoleAbsenceSupport || item.Kind == EvidenceAbsent {
		rank += 8
	}
	switch role {
	case EvidenceDiagramRoleConfig:
		if LooksLikeConfigFilePath(item.Source) {
			rank -= 10
		}
		if item.Scope == ScopeFile || item.Scope == ScopeSection || item.Scope == ScopeNegative {
			rank -= 4
		}
	case EvidenceDiagramRoleDefault:
		switch item.AnchorKind {
		case AnchorDefinition, AnchorAssignment, AnchorInitializer:
			rank -= 10
		case AnchorReturn:
			rank -= 4
		}
	case EvidenceDiagramRoleRuntime, EvidenceDiagramRoleOverride:
		switch item.AnchorKind {
		case AnchorAssignment, AnchorInitializer, AnchorCall, AnchorCondition:
			rank -= 8
		}
	}
	if strings.TrimSpace(item.AnchorSymbol) != "" {
		rank -= 2
	}
	return rank
}

func principalEvidenceLaneTitle(family QuestionFamily) string {
	switch family {
	case QFConfigPrecedence:
		return "Grounded config / precedence evidence"
	case QFRoleLookup:
		return "Grounded role lookup evidence"
	case QFEnumeration:
		return "Grounded enumeration evidence"
	case QFArchitecture:
		return "Grounded architecture evidence"
	case QFComparison:
		return "Grounded comparison evidence"
	default:
		return "Grounded principal evidence"
	}
}

func principalEvidenceLaneGuidance(family QuestionFamily, rm RequestModel) string {
	base := "Use this lane for principal user-visible claims in this family. " +
		"Each entry is selected from typed facet source candidates, so it may support the block kinds listed below. " +
		"Evidence notes can enrich the cited fact, but do not add uncited helper names, search hints, prior-turn guesses, or nearby context as new principal claims. " +
		"When entries use different visible syntax or label surfaces (assignment, object/struct literal, import/path, route, macro, table label), preserve each entry's own snippet/operator instead of collapsing them into one generic wording. " +
		"When an entry says member_surface=source_location, the cited file/path/line is the principal member label; do not replace it with a helper symbol or imported endpoint."
	switch family {
	case QFConfigPrecedence:
		return base + " For config answers, keep scalar/table/list content to real default/config/CLI/runtime layer anchors; general precedence rules belong in prose unless this lane cites that layer."
	case QFEnumeration:
		if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
			if rm.ChangeImpactProfile.RequestedOutput == ImpactOutputFiles {
				return base + " For change-impact file enumerations, file paths are the principal members and affected file:line sites are supporting reasons for those files. Direct assignments are only one affected-site role; include files backed by readers, guards, validators, construction sites, serializers, call adapters, import edges, and definitions when this lane contains typed evidence for them. Do not narrow the user's affected-file criterion to one role unless the typed impact profile says the requested output is that one role."
			}
			return base + " For change-impact enumerations, affected sites are the principal members. Direct assignments are only one affected-site role; include readers, guards, validators, construction sites, serializers, call adapters, import edges, and definitions when this lane contains typed evidence for them. Do not narrow the user's affected-site criterion to one role unless the typed impact profile says the requested output is that one role."
		}
		if principalMemberCoveragePolicyForEnumeration(rm) == PrincipalMemberCoveragePolicyEnrichmentOnly {
			return base + " For enumeration-shaped mechanism, architecture, or comparison fallbacks, these entries are grounded enrichment anchors, not a one-row-per-entry member slate. Use them only for the user's requested axes or claims, and keep helper/context entries out of principal lists."
		}
		return base + " For enumerations, each principal item must correspond to a listed entry here or to an extractor-backed symbol; do not invent missing members from context."
	case QFArchitecture:
		return base + " For architecture answers, sections should describe component responsibilities supported by these anchors; include grounded produced artifacts / consumed inputs / handoff outputs when the role-output lane supplies them, and avoid turning unrelated helper calls into architectural layers."
	case QFComparison:
		return base + " For comparisons, preserve the user's bucket labels from the semantic view and use these anchors only for the bucket content they actually support."
	default:
		return base
	}
}

func principalMemberCoveragePolicyForEnumeration(rm RequestModel) PrincipalMemberCoveragePolicy {
	if enumerationRequestRequiresPrincipalMemberCoverage(rm) {
		return PrincipalMemberCoveragePolicyRequired
	}
	return PrincipalMemberCoveragePolicyEnrichmentOnly
}

func enumerationRequestRequiresPrincipalMemberCoverage(rm RequestModel) bool {
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if rm.CompletenessObligation.IsActive() {
		return true
	}
	if rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqEnumeration, ReqRegistration:
		return true
	case ReqMechanism, ReqCallChain, ReqConfigMapping, ReqReturnValue, ReqHistory, ReqConditional:
		return false
	}
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsRoleLocateLookup || rm.Predicates.IsCountQuestion {
		return false
	}
	return rm.Intent == IntentEnumerate
}

func principalEvidenceAllowedBlocks(family QuestionFamily) []string {
	switch family {
	case QFConfigPrecedence:
		return blockKindStrings(BlockSummary, BlockScalar, BlockTable, BlockOrderedList)
	case QFRoleLookup:
		return blockKindStrings(BlockSummary, BlockScalar, BlockSection)
	case QFEnumeration:
		return blockKindStrings(BlockSummary, BlockOrderedList, BlockTable, BlockBulletList, BlockSection)
	case QFArchitecture:
		return blockKindStrings(BlockSummary, BlockSection, BlockOrderedList, BlockBulletList, BlockDiagram)
	case QFComparison:
		return blockKindStrings(BlockSummary, BlockSection, BlockTable, BlockOrderedList, BlockBulletList)
	case QFGeneric:
		return blockKindStrings(BlockSummary, BlockSection, BlockOrderedList, BlockBulletList, BlockDiagram)
	default:
		return blockKindStrings(BlockSummary)
	}
}

func principalEvidenceAllowedBlocksForRequest(family QuestionFamily, rm RequestModel) []string {
	allowed := principalEvidenceAllowedBlocks(family)
	if rm.ErrorGranularityProfile == nil || !rm.ErrorGranularityProfile.Active() {
		return allowed
	}
	for _, kind := range allowed {
		if kind == string(BlockDecision) {
			return allowed
		}
	}
	out := append([]string(nil), allowed...)
	out = append(out, string(BlockDecision))
	return out
}

func principalSupportFacetKinds(family QuestionFamily) []AnswerFacetKind {
	switch family {
	case QFConfigPrecedence:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetConfigPrecedenceRole}
	case QFRoleLookup:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetCurrentCodePath, FacetNearestMechanism}
	case QFEnumeration:
		return []AnswerFacetKind{FacetEnumerationItem}
	case QFArchitecture:
		return []AnswerFacetKind{FacetCurrentCodePath, FacetComponentRelation, FacetDiagramSpine}
	case QFComparison:
		return []AnswerFacetKind{FacetCurrentCodePath, FacetComponentRelation}
	case QFGeneric:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetCurrentCodePath, FacetNearestMechanism, FacetPrincipalPathEdge}
	default:
		return nil
	}
}

func supportFacetCandidateIDs(plan *AnswerSurfacePlan, facets ...AnswerFacetKind) map[string]bool {
	if plan == nil || plan.FacetCoverage == nil || len(facets) == 0 {
		return nil
	}
	want := make(map[AnswerFacetKind]bool, len(facets))
	for _, facet := range facets {
		want[facet] = true
	}
	out := make(map[string]bool)
	collect := func(req FacetRequirement) {
		if !want[req.Kind] {
			return
		}
		for _, id := range req.SourceCandidate {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = true
			}
		}
	}
	for _, req := range plan.FacetCoverage.Required {
		collect(req)
	}
	for _, req := range plan.FacetCoverage.Optional {
		collect(req)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PrincipalSupportEvidenceItemsForFamily returns the same ordered,
// de-duplicated typed evidence pool that the principal support lane
// consumes for a question family. Consumers that need to reason about
// the principal evidence shape should use this helper rather than
// re-walking SurfaceEvidence and facet candidates with local rules.
func PrincipalSupportEvidenceItemsForFamily(family QuestionFamily, plan *AnswerSurfacePlan) []EvidenceItem {
	return principalSupportEvidenceItemsForFacets(family, plan, principalSupportFacetKinds(family)...)
}

// PrincipalSupportEvidenceItemsForFacet returns the same curated
// model-authored / de-duplicated principal evidence pool, scoped to a
// single facet. Prompt renderers use this as the answer-grade evidence
// count so broad raw SourceCandidate pools do not look like hundreds of
// equally principal facts.
func PrincipalSupportEvidenceItemsForFacet(family QuestionFamily, plan *AnswerSurfacePlan, facet AnswerFacetKind) []EvidenceItem {
	return principalSupportEvidenceItemsForFacets(family, plan, facet)
}

func principalSupportEvidenceItemsForFacets(family QuestionFamily, plan *AnswerSurfacePlan, facets ...AnswerFacetKind) []EvidenceItem {
	out := principalSupportEvidenceItemsForFacetsRaw(family, plan, facets...)
	if family == QFEnumeration {
		return curateEnumerationPrincipalEvidence(plan, out)
	}
	return out
}

func principalSupportEvidenceItemsForFacetsRaw(family QuestionFamily, plan *AnswerSurfacePlan, facets ...AnswerFacetKind) []EvidenceItem {
	candidates := supportFacetCandidateIDs(plan, facets...)
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(candidates))
	out := make([]EvidenceItem, 0, len(candidates))
	for _, item := range orderedFacetSupportEvidenceItems(family, plan.SurfaceEvidence) {
		id := strings.TrimSpace(item.ID)
		if id == "" || !candidates[id] || !principalEvidenceItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		location := supportEntryLocation(item)
		key := strings.ToLower(text) + "\x00" + strings.ToLower(location)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	out = preferModelAuthoredPrincipalEvidence(out)
	return dedupePrincipalSupportEvidenceBySurfaceRole(out)
}

func compileArchitectureRoleOutputSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneNearestMechanism,
		Title:         "Grounded architecture role / output enrichment",
		AllowedBlocks: blockKindStrings(BlockSummary, BlockSection, BlockOrderedList, BlockBulletList, BlockDiagram),
		Guidance: "Use this lane to enrich architecture sections with what a component, stage, module, or layer produces, consumes, validates, or hands off. " +
			"Entries here are grounded role/output facts, such as produced artifacts, typed IRs, plans, verdicts, reports, state transitions, or handoff boundaries. " +
			"They may deepen the section that already answers the user's requested architecture, but they are not standalone layers or list members unless the principal architecture lane also carries that component. " +
			"Do not invent stage products from naming conventions; preserve only the model-emitted structured evidence or deterministic source lines shown in the entries.",
	}
	items := PrincipalSupportEvidenceItemsForFacet(QFArchitecture, plan, FacetNearestMechanism)
	if len(items) == 0 {
		return lane
	}
	for _, item := range items {
		text := strings.TrimSpace(EvidenceDeterministicSurfaceText(item, false))
		if text == "" {
			text = strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		}
		if text == "" {
			continue
		}
		entry := answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text))
		entry.MemberSurface = PrincipalMemberSurfaceDisplayLabel
		entry.Detail = appendPrincipalMemberSurfaceDetail(entry.Detail, entry.MemberSurface)
		lane.Entries = append(lane.Entries, entry)
		if len(lane.Entries) >= facetSupportEntryLimitDefault {
			break
		}
	}
	return lane
}

func compileEnumerationSupportingContextLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneNearestMechanism,
		Title:         "Grounded enumeration support context",
		AllowedBlocks: blockKindStrings(BlockSummary, BlockTable, BlockSection, BlockCaveat),
		Guidance: "Use this lane to explain why the principal members satisfy the requested set, " +
			"or to disclose nearby proof boundaries. These entries are supporting mechanism / context: " +
			"do not turn them into additional principal ordered-list or bullet-list members.",
	}
	items := principalSupportEvidenceItemsForFacetsRaw(QFEnumeration, plan, principalSupportFacetKinds(QFEnumeration)...)
	if len(items) == 0 {
		return lane
	}
	for _, item := range enumerationSupportingContextEvidence(plan, items) {
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		entry := answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text))
		entry.MemberSurface = PrincipalMemberSurfaceForRequest(rm, item, items)
		entry.Detail = appendPrincipalMemberSurfaceDetail(entry.Detail, entry.MemberSurface)
		lane.Entries = append(lane.Entries, entry)
		if len(lane.Entries) >= facetSupportEntryLimitDefault {
			break
		}
	}
	return lane
}

func appendPrincipalMemberSurfaceDetail(detail string, surface AnswerPrincipalMemberSurface) string {
	if surface == PrincipalMemberSurfaceUnknown {
		return strings.TrimSpace(detail)
	}
	detail = strings.TrimSpace(detail)
	member := "member_surface=" + string(surface)
	if detail == "" {
		return member
	}
	if strings.Contains(detail, member) {
		return detail
	}
	return detail + "; " + member
}

func curateEnumerationPrincipalEvidence(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	if plan != nil && plan.ChangeImpactProfile != nil && plan.ChangeImpactProfile.Active() {
		if filtered, ok := filterChangeImpactPrincipalEvidence(plan.ChangeImpactProfile, items); ok {
			return filtered
		}
		return items
	}
	if plan != nil && len(plan.StepBackbone) > 0 {
		return enumerationPrincipalEvidenceMatchingBackbone(plan, items)
	}
	return filterDominantEnumerationPrincipalSurface(items)
}

func filterChangeImpactPrincipalEvidence(profile *ChangeImpactProfile, items []EvidenceItem) ([]EvidenceItem, bool) {
	if len(items) == 0 || profile == nil || !profile.Active() {
		return nil, false
	}
	candidates := make([]EvidenceItem, 0, len(items))
	roleFiltered := false
	for _, item := range items {
		if !changeImpactPrincipalEvidenceRoleEligible(profile, item) {
			roleFiltered = true
			continue
		}
		candidates = append(candidates, item)
	}
	if roleFiltered && len(candidates) == 0 {
		return nil, true
	}
	if len(candidates) == 0 {
		return nil, false
	}
	target := parseChangeImpactPrincipalTarget(profile)
	if strings.TrimSpace(target.path) == "" {
		if roleFiltered {
			return candidates, true
		}
		return nil, false
	}
	out := make([]EvidenceItem, 0, len(candidates))
	for _, item := range candidates {
		if changeImpactEvidenceMatchesTarget(item, candidates, target) {
			out = append(out, item)
		}
	}
	if len(out) > 0 {
		return out, true
	}
	return nil, true
}

func changeImpactPrincipalEvidenceRoleEligible(profile *ChangeImpactProfile, item EvidenceItem) bool {
	switch item.ContextRole {
	case EvidenceContextRoleRelatedContext, EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	}
	if item.Kind != EvidenceMechanism {
		return true
	}
	return changeImpactProfileAllowsMechanismPrincipal(profile)
}

func changeImpactProfileAllowsMechanismPrincipal(profile *ChangeImpactProfile) bool {
	if profile == nil {
		return false
	}
	for _, kind := range profile.AffectedSiteKinds {
		switch kind {
		case ImpactSiteDocumentation, ImpactSiteGenerated, ImpactSiteBuild:
			return true
		}
	}
	return false
}

// ChangeImpactPrincipalTargetSurfaceGaps returns model-emitted impact-site
// evidence whose cited source text names the owner-qualified target, but whose
// structured evidence fields do not. Callers use this as a repair signal: the
// answer member must be re-emitted with the target in subject/object/snippet /
// surface_terms rather than recovered from free-form summary prose.
func ChangeImpactPrincipalTargetSurfaceGaps(profile *ChangeImpactProfile, items []EvidenceItem, sourceText func(EvidenceItem) string) []EvidenceItem {
	if len(items) == 0 || profile == nil || !profile.Active() || !profile.RequestedBroadAffectedSites() {
		return nil
	}
	target := parseChangeImpactPrincipalTarget(profile)
	if !target.ownerQualified || strings.TrimSpace(target.path) == "" {
		return nil
	}
	candidates := make([]EvidenceItem, 0, len(items))
	for _, item := range orderedFacetSupportEvidenceItems(QFEnumeration, items) {
		if !principalEvidenceItemEligible(item) ||
			!changeImpactPrincipalEvidenceRoleEligible(profile, item) ||
			!changeImpactPrincipalEvidenceClaimEligible(profile, item) {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(candidates))
	out := make([]EvidenceItem, 0, len(candidates))
	for _, item := range candidates {
		if changeImpactEvidenceMatchesTarget(item, candidates, target) {
			continue
		}
		if sourceText == nil || !changeImpactSourceTextHasTarget(sourceText(item), target.path) {
			continue
		}
		key := principalSupportEvidenceSurfaceRoleKey(item)
		if key == "" {
			key = strings.ToLower(supportEntryLocation(item))
		}
		if key != "" && seen[key] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		out = append(out, item)
	}
	return out
}

func changeImpactPrincipalEvidenceClaimEligible(profile *ChangeImpactProfile, item EvidenceItem) bool {
	switch ClaimFormOf(item) {
	case ClaimDefinitionFact, ClaimAssignmentFact, ClaimGuardCondition,
		ClaimCallEdge, ClaimReturnFact, ClaimImportEdge:
		return true
	case ClaimTextReferenceFact:
		return profile != nil && profile.AllowsTextReferencePrincipal()
	default:
		return false
	}
}

type changeImpactPrincipalTarget struct {
	raw            string
	owner          string
	leaf           string
	path           string
	ownerQualified bool
}

func parseChangeImpactPrincipalTarget(profile *ChangeImpactProfile) changeImpactPrincipalTarget {
	if profile == nil || !profile.Active() {
		return changeImpactPrincipalTarget{}
	}
	raw := strings.TrimSpace(profile.Target)
	normalized := normalizeChangeImpactTargetSurface(raw)
	parts := strings.Split(normalized, ".")
	if len(parts) < 2 {
		return changeImpactPrincipalTarget{raw: raw, path: normalized}
	}
	owner := strings.TrimSpace(parts[len(parts)-2])
	leaf := strings.TrimSpace(parts[len(parts)-1])
	if owner == "" || leaf == "" {
		return changeImpactPrincipalTarget{raw: raw, path: normalized}
	}
	return changeImpactPrincipalTarget{
		raw:            raw,
		owner:          owner,
		leaf:           leaf,
		path:           owner + "." + leaf,
		ownerQualified: true,
	}
}

func changeImpactEvidenceMatchesTarget(item EvidenceItem, peers []EvidenceItem, target changeImpactPrincipalTarget) bool {
	if strings.TrimSpace(target.path) == "" {
		return true
	}
	if !target.ownerQualified {
		return changeImpactEvidenceHasTargetPath(item, target.path)
	}
	if changeImpactEvidenceHasTargetPath(item, target.path) {
		return true
	}
	if changeImpactEvidenceDefinesOwner(item, target.owner) {
		return true
	}
	if changeImpactEvidenceDefinesLeaf(item, target.leaf) &&
		changeImpactHasNearbyOwnerDefinition(item, peers, target.owner) {
		return true
	}
	return false
}

func changeImpactEvidenceHasTargetPath(item EvidenceItem, targetPath string) bool {
	if strings.TrimSpace(targetPath) == "" {
		return false
	}
	for _, surface := range changeImpactEvidenceTargetSurfaces(item) {
		if changeImpactSourceTextHasTarget(surface, targetPath) {
			return true
		}
	}
	return false
}

func changeImpactEvidenceTargetSurfaces(item EvidenceItem) []string {
	out := []string{
		item.AnchorSymbol,
		item.OwnerSymbol,
		item.Subject,
		item.Object,
		item.Predicate,
		item.Condition,
		item.Snippet,
	}
	out = append(out, item.SurfaceTerms...)
	return out
}

func changeImpactEvidenceDefinesOwner(item EvidenceItem, owner string) bool {
	if strings.TrimSpace(owner) == "" || ClaimFormOf(item) != ClaimDefinitionFact {
		return false
	}
	return changeImpactSurfaceEquals(item.AnchorSymbol, owner) ||
		changeImpactSurfaceEquals(item.Subject, owner) ||
		changeImpactSurfaceEquals(item.OwnerSymbol, owner)
}

func changeImpactEvidenceDefinesLeaf(item EvidenceItem, leaf string) bool {
	if strings.TrimSpace(leaf) == "" || ClaimFormOf(item) != ClaimDefinitionFact {
		return false
	}
	return changeImpactSurfaceEquals(item.AnchorSymbol, leaf) ||
		changeImpactSurfaceEquals(item.Subject, leaf) ||
		changeImpactSurfaceEquals(item.Object, leaf)
}

func changeImpactHasNearbyOwnerDefinition(item EvidenceItem, peers []EvidenceItem, owner string) bool {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return false
	}
	source := normalizeAnswerSupportPath(item.Source)
	for _, peer := range peers {
		if normalizeAnswerSupportPath(peer.Source) != source || peer.LineStart <= 0 {
			continue
		}
		if absInt(peer.LineStart-item.LineStart) > definitionSupportMemberCitationWindow {
			continue
		}
		if changeImpactEvidenceDefinesOwner(peer, owner) {
			return true
		}
	}
	return false
}

func changeImpactSurfaceEquals(surface, target string) bool {
	return normalizeChangeImpactTargetSurface(surface) == normalizeChangeImpactTargetSurface(target)
}

func changeImpactSourceTextHasTarget(surface, targetPath string) bool {
	if strings.TrimSpace(targetPath) == "" {
		return false
	}
	normalized := normalizeChangeImpactTargetSurface(surface)
	return normalized != "" && strings.Contains(normalized, targetPath)
}

func normalizeChangeImpactTargetSurface(surface string) string {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		`\\`, `/`,
		"::", ".",
		"->", ".",
		"?.", ".",
		"[", ".",
		"]", "",
		"(", " ",
		")", " ",
		"{", " ",
		"}", " ",
		"`", "",
		"\"", "",
		"'", "",
	)
	surface = replacer.Replace(surface)
	fields := strings.FieldsFunc(surface, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ';', ':', '=', '!', '<', '>', '+', '-', '*', '/', '&', '|':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(fields, "."))
}

func enumerationSupportingContextEvidence(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	principal := curateEnumerationPrincipalEvidence(plan, items)
	if len(principal) == 0 {
		return nil
	}
	principalIDs := make(map[string]bool, len(principal))
	for _, item := range principal {
		if id := strings.TrimSpace(item.ID); id != "" {
			principalIDs[id] = true
		}
	}
	var out []EvidenceItem
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id != "" && principalIDs[id] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterDominantEnumerationPrincipalSurface(items []EvidenceItem) []EvidenceItem {
	if len(items) < 3 {
		return items
	}
	counts := make(map[AnswerPrincipalMemberSurface]int)
	for _, item := range items {
		group := PrincipalMemberSurfaceForEvidenceSet(item, items)
		if group == PrincipalMemberSurfaceUnknown {
			continue
		}
		counts[group]++
	}
	if len(counts) < 2 {
		return items
	}
	var best AnswerPrincipalMemberSurface
	bestCount := 0
	tied := false
	for group, count := range counts {
		switch {
		case count > bestCount:
			best = group
			bestCount = count
			tied = false
		case count == bestCount:
			tied = true
		}
	}
	if tied || bestCount < 2 {
		return items
	}
	out := make([]EvidenceItem, 0, bestCount)
	for _, item := range items {
		group := PrincipalMemberSurfaceForEvidenceSet(item, items)
		if group == best {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return items
	}
	return out
}

func enumerationPrincipalEvidenceMatchingBackbone(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if plan == nil || len(plan.StepBackbone) == 0 || len(items) == 0 {
		return items
	}
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if enumerationEvidenceMatchesStepBackbone(plan.StepBackbone, item) {
			out = append(out, item)
		}
	}
	return out
}

func enumerationEvidenceNotMatchingBackbone(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if plan == nil || len(plan.StepBackbone) == 0 || len(items) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if !enumerationEvidenceMatchesStepBackbone(plan.StepBackbone, item) {
			out = append(out, item)
		}
	}
	return out
}

func enumerationEvidenceMatchesStepBackbone(backbone []StepSurfaceAnchor, item EvidenceItem) bool {
	source := normalizeAnswerSupportPath(item.Source)
	if source == "" || item.LineStart <= 0 {
		return false
	}
	itemNames := normalizedEnumerationEvidenceNames(item)
	for _, anchor := range backbone {
		anchorFile := normalizeAnswerSupportPath(anchor.File)
		if anchorFile == "" || anchorFile != source {
			continue
		}
		if anchor.Line > 0 && anchor.Line == item.LineStart {
			return true
		}
		if normalizedEnumerationName(anchor.Name) == "" {
			continue
		}
		if anchor.Line > 0 && absInt(anchor.Line-item.LineStart) > definitionSupportMemberCitationWindow {
			continue
		}
		if itemNames[normalizedEnumerationName(anchor.Name)] {
			return true
		}
	}
	return false
}

func normalizedEnumerationEvidenceNames(item EvidenceItem) map[string]bool {
	out := make(map[string]bool, 6+len(item.SurfaceTerms))
	for _, raw := range []string{item.AnchorSymbol, item.Subject, item.Object} {
		if name := normalizedEnumerationName(raw); name != "" {
			out[name] = true
		}
	}
	for _, raw := range item.SurfaceTerms {
		if name := normalizedEnumerationName(raw); name != "" {
			out[name] = true
		}
	}
	return out
}

func normalizedEnumerationName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToLower(NormalizedSurfaceSymbolTail(raw))
}

func preferModelAuthoredPrincipalEvidence(items []EvidenceItem) []EvidenceItem {
	modelAuthored := 0
	modelForms := make(map[ClaimForm]bool)
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			modelAuthored++
			if form := ClaimFormOf(item); form != ClaimUnknown {
				modelForms[form] = true
			}
		}
	}
	if modelAuthored == 0 {
		return items
	}
	out := make([]EvidenceItem, 0, modelAuthored)
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			out = append(out, item)
		}
	}
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			continue
		}
		form := ClaimFormOf(item)
		if form != ClaimReturnFact || modelForms[form] {
			continue
		}
		modelForms[form] = true
		out = append(out, item)
	}
	return out
}

func principalEvidenceModelAuthored(item EvidenceItem) bool {
	if strings.TrimSpace(item.Producer) == "explorer.emit_evidence" {
		return true
	}
	return strings.TrimSpace(item.Producer) != "" && item.Kind.IsLLMEmittable()
}

func dedupePrincipalSupportEvidenceBySurfaceRole(items []EvidenceItem) []EvidenceItem {
	if len(items) < 2 {
		return items
	}
	seen := make(map[string]bool, len(items))
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		key := principalSupportEvidenceSurfaceRoleKey(item)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, item)
	}
	return out
}

func principalSupportEvidenceSurfaceRoleKey(item EvidenceItem) string {
	location := strings.TrimSpace(strings.ToLower(supportEntryLocation(item)))
	if location == "" {
		return ""
	}
	form := ClaimFormOf(item)
	if form == ClaimUnknown {
		return ""
	}
	parts := []string{
		location,
		string(form),
		string(item.AnchorKind),
		strings.TrimSpace(item.AnchorSymbol),
		strings.TrimSpace(item.OwnerSymbol),
	}
	if item.AnchorKind != AnchorAssignment && item.AnchorKind != AnchorInitializer {
		parts = append(parts,
			strings.TrimSpace(item.Subject),
			strings.TrimSpace(item.Object),
			strings.TrimSpace(item.Condition),
		)
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return strings.Join(parts, "\x00")
}

func facetSupportEntryLimitForFamily(family QuestionFamily, candidateCount int) int {
	if family == QFEnumeration {
		// Enumeration lane entries are principal answer members, not
		// optional enrichment. Capping them silently drops part of the
		// closed set before the finalizer sees it, so keep the full
		// typed evidence member set and let explicit uncertainty /
		// completeness contracts disclose any real boundary.
		return candidateCount
	}
	return facetSupportEntryLimitDefault
}

func orderedFacetSupportEvidenceItems(family QuestionFamily, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := append([]EvidenceItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		leftPriority := facetSupportEvidencePriority(family, out[i])
		rightPriority := facetSupportEvidencePriority(family, out[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftSource := supportEvidenceSortLocation(out[i])
		rightSource := supportEvidenceSortLocation(out[j])
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		if out[i].LineStart != out[j].LineStart {
			return out[i].LineStart < out[j].LineStart
		}
		return strings.TrimSpace(out[i].AnchorSymbol) < strings.TrimSpace(out[j].AnchorSymbol)
	})
	return out
}

func facetSupportEvidencePriority(family QuestionFamily, item EvidenceItem) int {
	if supportEvidenceIsGroundedAnswerAnchor(item) {
		switch {
		case item.Kind == EvidenceDirect && item.AnchorKind == AnchorDefinition:
			if family == QFEnumeration || family == QFRoleLookup {
				return 0
			}
			return 1
		case item.Kind == EvidenceRegistration:
			return 2
		case item.Kind == EvidenceDirect:
			return 3
		case item.Kind == EvidenceRelationship || item.Kind == EvidenceMechanism:
			return 4
		case item.Kind == EvidenceConditional:
			return 5
		}
	}
	switch {
	case item.Producer == "explorer.emit_evidence":
		return 20
	case item.Kind.IsLLMEmittable() && item.Producer != "":
		return 21
	case item.Kind == EvidenceConcrete || item.Producer == "concrete_values":
		return 22
	case strings.HasPrefix(item.Producer, "dataflow."):
		return 23
	default:
		return 22
	}
}

func supportEvidenceIsGroundedAnswerAnchor(item EvidenceItem) bool {
	return item.GroundingStatus == GroundingGrounded &&
		supportEvidenceHasUsableLocation(item) &&
		item.IsCitable()
}

func principalEvidenceItemEligible(item EvidenceItem) bool {
	if item.GroundingStatus == GroundingUngrounded {
		return false
	}
	return supportEvidenceHasUsableLocation(item) && item.IsCitable()
}

func compileFacetUncertaintySupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneUncertaintyBound,
		Title:         "Evidence boundary disclosures",
		AllowedBlocks: blockKindStrings(BlockCaveat, BlockSummary),
		Guidance: "Use this lane only for scope, absence, drift, or proof-boundary disclosures. " +
			"It can qualify the principal answer but must not become extra principal list items, table rows, diagram nodes, or comparison buckets.",
	}
	if plan == nil {
		return lane
	}
	for _, item := range orderedFacetSupportEvidenceItems(QFGeneric, plan.SurfaceEvidence) {
		if !uncertaintySupportItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			text = strings.TrimSpace(EvidenceDeterministicSurfaceText(item, false))
		}
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
		if len(lane.Entries) >= facetUncertaintySupportEntryLimit {
			break
		}
	}
	return lane
}

func uncertaintySupportItemEligible(item EvidenceItem) bool {
	if !supportEvidenceHasUsableLocation(item) {
		return false
	}
	if !item.IsCitable() {
		return false
	}
	return item.ContextRole == EvidenceContextRoleAbsenceSupport ||
		item.Kind == EvidenceAbsent ||
		item.DriftReason != "" ||
		item.Authority == AuthorityConditional
}
