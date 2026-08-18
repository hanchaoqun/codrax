package agent

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	answerDocMultiTopicEvidencePerUnitLimit = 16
	answerDocMultiTopicSharedEvidenceLimit  = 12
)

type answerDocInvestigationEvidenceOwner struct {
	producerUnitIDs map[string]bool
	unitIDs         map[string]bool
	association     string
	affinityScore   int
	item            types.EvidenceItem
}

// renderAnswerDocMultiTopicEvidenceOwnership preserves the typed producer
// lineage that already exists between topic evidence nodes and EvidenceItems.
// It is prompt-only guidance: ownership says where a fact was investigated,
// not that two facts form one mechanism or that either fact is a conclusion.
func renderAnswerDocMultiTopicEvidenceOwnership(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	plan := types.CompileInvestigationPlan(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract)
	if len(plan.Units) < 2 {
		return ""
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return ""
	}
	ledger := closure.NodeArtifactLedger()
	if ledger.Empty() {
		return ""
	}

	// Multi-topic partitioning selects only a handful of rows per unit, so it
	// must rank before truncating. Reusing the general enrichment pool's global
	// 1024-row cap let one broad file monopolize the prefix and starve a later,
	// directly relevant row. The pool below remains the already-accepted typed
	// evidence set; only this prompt-only ranker sees its full reconciled shape.
	evidence := answerDocMultiTopicEvidencePool(ctx)
	byID := make(map[string]*answerDocInvestigationEvidenceOwner, len(evidence))
	for _, item := range evidence {
		if item.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		id := types.RuntimeArtifactIDForEvidenceItem(item)
		if id == "" {
			continue
		}
		byID[id] = &answerDocInvestigationEvidenceOwner{
			producerUnitIDs: map[string]bool{},
			unitIDs:         map[string]bool{},
			item:            item,
		}
	}
	for _, record := range ledger.RecordsByKind(types.RuntimeArtifactEvidenceItem) {
		id := strings.TrimSpace(record.EvidenceID)
		if id == "" {
			id = strings.TrimSpace(record.Artifact.ID)
		}
		owned := byID[id]
		if owned == nil {
			continue
		}
		unit, ok := plan.InvestigationUnitForEvidenceNode(record.ProducerNodeID)
		if !ok {
			continue
		}
		owned.producerUnitIDs[unit.ID] = true
	}

	// A topic node is an execution producer, not semantic ownership: one
	// focused Explorer call can legitimately inspect a neighboring topic too.
	// Prefer exact analyzer-authored file/directory scopes. Producer lineage is
	// only a soft fallback when that unit has no exact source match at all.
	unitHasExactSource := make(map[string]bool, len(plan.Units))
	for _, owned := range byID {
		for _, unit := range plan.Units {
			if answerDocEvidenceMatchesInvestigationUnitSource(owned.item, unit) {
				owned.unitIDs[unit.ID] = true
				owned.association = "exact_source_scope"
				unitHasExactSource[unit.ID] = true
			}
		}
	}
	unitAffinityTokens, tokenUnitFrequency := answerDocInvestigationUnitAffinityTokens(plan.Units)
	unitIdentityTokens := answerDocInvestigationUnitIdentityTokens(plan.Units)
	for _, owned := range byID {
		if len(owned.unitIDs) > 0 {
			continue
		}
		bestUnitID, bestScore, bestCount := "", 0, 0
		for _, unit := range plan.Units {
			score := answerDocInvestigationEvidenceAffinityScore(owned.item, unitAffinityTokens[unit.ID], unitIdentityTokens[unit.ID], tokenUnitFrequency)
			if score > bestScore {
				bestUnitID, bestScore, bestCount = unit.ID, score, 1
			} else if score > 0 && score == bestScore {
				bestCount++
			}
		}
		// A unique unit-only token is worth four points. Requiring that floor
		// keeps broad shared words from manufacturing ownership. This remains a
		// soft writing hint and is disclosed as such in the prompt.
		if bestScore >= 4 && bestCount == 1 {
			owned.unitIDs[bestUnitID] = true
			owned.association = "topic_affinity_hint"
			owned.affinityScore = bestScore
		}
		if len(owned.unitIDs) > 0 {
			continue
		}
		if len(owned.producerUnitIDs) == 1 {
			for unitID := range owned.producerUnitIDs {
				if !unitHasExactSource[unitID] {
					owned.unitIDs[unitID] = true
					owned.association = "producer_lane_fallback"
				}
			}
		}
		if len(owned.unitIDs) == 0 {
			owned.association = "shared_or_unassigned"
		}
	}

	unitRows := make(map[string][]answerDocInvestigationEvidenceOwner, len(plan.Units))
	var shared []answerDocInvestigationEvidenceOwner
	for _, owned := range byID {
		switch len(owned.unitIDs) {
		case 0:
			shared = append(shared, *owned)
		case 1:
			for unitID := range owned.unitIDs {
				row := *owned
				if row.affinityScore == 0 {
					row.affinityScore = answerDocInvestigationEvidenceAffinityScore(row.item, unitAffinityTokens[unitID], unitIdentityTokens[unitID], tokenUnitFrequency)
				}
				unitRows[unitID] = append(unitRows[unitID], row)
			}
		default:
			shared = append(shared, *owned)
		}
	}
	hasOwned := false
	for _, unit := range plan.Units {
		if len(unitRows[unit.ID]) > 0 {
			hasOwned = true
			break
		}
	}
	if !hasOwned && len(shared) == 0 {
		return ""
	}

	sortEvidenceOwners := func(rows []answerDocInvestigationEvidenceOwner) {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].affinityScore != rows[j].affinityScore {
				return rows[i].affinityScore > rows[j].affinityScore
			}
			leftScore := answerDocInvestigationEvidenceDisplayScore(rows[i].item)
			rightScore := answerDocInvestigationEvidenceDisplayScore(rows[j].item)
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			return types.RuntimeArtifactIDForEvidenceItem(rows[i].item) < types.RuntimeArtifactIDForEvidenceItem(rows[j].item)
		})
	}

	var b strings.Builder
	b.WriteString("## Evidence partition hints by investigation unit (prompt-only)\n\n")
	b.WriteString("- The grouping below prefers exact file/directory scopes declared on typed investigation units; task-node producer lineage is used only as a fallback when a unit has no exact source-scope match. It is a writing partition, not semantic ownership. Do not echo this heading, unit IDs, evidence IDs, association labels, or connectivity labels in the user-facing answer.\n")
	b.WriteString("- `topic_affinity_hint` and affinity ordering use only typed unit summaries/entities/scopes plus structured evidence fields. They are noisy prompt-selection guidance only: they do not prove ownership, relevance, an edge, or a conclusion and never drive validation or rejection.\n")
	b.WriteString("- Within that soft ordering, a structured evidence row that names a bare typed component/entity receives priority over rows matching only a file/directory candidate. This keeps the named component's mechanism visible before the per-unit display cap; it still does not prove that the row answers the unit.\n")
	b.WriteString("- A unit association is not a call, causal, precedence, fallback, or failure-path edge. Join facts into one mechanism only when an explicit typed relation row supplies the exact endpoints and direction.\n")
	b.WriteString("- `connectivity=standalone_fact` rows may explain a definition, state, option, or local behavior, but they cannot by themselves become a transition in a mechanism chain. `connectivity=explicit_typed_relation` authorizes only the displayed subject-to-object relation, not a broader category or neighboring step.\n")
	b.WriteString("- Evidence assigned to another unit, shared probe evidence, and disconnected standalone facts remain supporting context unless a typed relation independently connects them to the current section.\n\n")

	for _, unit := range plan.Units {
		rows := unitRows[unit.ID]
		if len(rows) == 0 {
			continue
		}
		sortEvidenceOwners(rows)
		fmt.Fprintf(&b, "### Unit %d: %s\n\n", unit.Index, answerDocInlineClip(firstNonEmptyAnswerDocUnitLabel(unit), 180))
		limit := len(rows)
		if limit > answerDocMultiTopicEvidencePerUnitLimit {
			limit = answerDocMultiTopicEvidencePerUnitLimit
		}
		for _, owned := range rows[:limit] {
			b.WriteString(renderAnswerDocInvestigationEvidenceOwnerRow(owned))
		}
		if len(rows) > limit {
			fmt.Fprintf(&b, "- ... %d additional owned evidence rows remain available in the main Evidence Items section.\n", len(rows)-limit)
		}
		b.WriteString("\n")
	}

	if len(shared) > 0 {
		sortEvidenceOwners(shared)
		b.WriteString("### Shared or not uniquely assigned evidence\n\n")
		limit := len(shared)
		if limit > answerDocMultiTopicSharedEvidenceLimit {
			limit = answerDocMultiTopicSharedEvidenceLimit
		}
		for _, owned := range shared[:limit] {
			b.WriteString(renderAnswerDocInvestigationEvidenceOwnerRow(owned))
		}
		if len(shared) > limit {
			fmt.Fprintf(&b, "- ... %d additional shared evidence rows remain available in the main Evidence Items section.\n", len(shared)-limit)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func answerDocMultiTopicEvidencePool(ctx *types.AgentContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	limit := len(ctx.EvidenceItems)
	if ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			limit += len(ta.EvidenceItems)
		}
		limit += len(ctx.Mutable.EmittedEvidence())
	}
	if limit <= 0 {
		return nil
	}
	return answerDocTypedEnrichmentEvidencePool(ctx, limit)
}

func answerDocInvestigationUnitAffinityTokens(units []types.InvestigationUnit) (map[string]map[string]bool, map[string]int) {
	byUnit := make(map[string]map[string]bool, len(units))
	frequency := map[string]int{}
	for _, unit := range units {
		tokens := map[string]bool{}
		for _, value := range append(append([]string{unit.Summary, unit.Label}, unit.Entities...), unit.Scopes...) {
			for token := range answerDocAffinityTokens(value) {
				tokens[token] = true
			}
		}
		byUnit[unit.ID] = tokens
		for token := range tokens {
			frequency[token]++
		}
	}
	return byUnit, frequency
}

// answerDocInvestigationUnitIdentityTokens keeps analyzer-authored business /
// component identities distinct from path-shaped navigation candidates. Both
// remain soft prompt-ranking inputs, but a bare identity such as REPL,
// Scheduler, or ConfigStore is more specific to the requested owner than a
// neighboring directory found during pre-scan. Explicit path authority belongs
// in InvestigationUnit.Scopes; this helper deliberately does not promote it.
func answerDocInvestigationUnitIdentityTokens(units []types.InvestigationUnit) map[string]map[string]bool {
	byUnit := make(map[string]map[string]bool, len(units))
	for _, unit := range units {
		tokens := map[string]bool{}
		for _, value := range unit.Entities {
			value = strings.TrimSpace(value)
			if value == "" || strings.ContainsAny(value, `/\\.`) {
				continue
			}
			for token := range answerDocAffinityTokens(value) {
				tokens[token] = true
			}
		}
		byUnit[unit.ID] = tokens
	}
	return byUnit
}

func answerDocInvestigationEvidenceAffinityScore(item types.EvidenceItem, unitTokens, identityTokens map[string]bool, frequency map[string]int) int {
	if len(unitTokens) == 0 {
		return 0
	}
	evidenceTokens := map[string]bool{}
	for _, value := range []string{
		item.Source, item.Subject, item.Predicate, item.Object, item.Summary,
		item.AnchorSymbol, item.OwnerSymbol, item.DeclaredBinding, item.DeclaredType,
		item.DeclaredOwner, types.EvidenceAuthoritativeSurfaceText(item, false),
	} {
		for token := range answerDocAffinityTokens(value) {
			evidenceTokens[token] = true
		}
	}
	score := 0
	for token := range unitTokens {
		if !evidenceTokens[token] {
			continue
		}
		if frequency[token] == 1 {
			score += 4
		} else {
			score++
		}
		if identityTokens[token] {
			// Prompt-only owner affinity. This is intentionally stronger than a
			// collection of path/summary tokens so one neighboring directory
			// cannot consume the bounded prefix ahead of the named component.
			// It never feeds validation or completion gates.
			score += 32
		}
	}
	return score
}

func answerDocAffinityTokens(value string) map[string]bool {
	out := map[string]bool{}
	var separated strings.Builder
	var previous rune
	for index, current := range value {
		if index > 0 && (unicode.IsLower(previous) || unicode.IsDigit(previous)) && unicode.IsUpper(current) {
			separated.WriteByte(' ')
		}
		separated.WriteRune(current)
		previous = current
	}
	for _, token := range strings.FieldsFunc(strings.ToLower(separated.String()), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		token = strings.TrimSpace(token)
		if utf8.RuneCountInString(token) < 3 {
			continue
		}
		out[token] = true
		// A short ASCII prefix lets ordinary inflections such as render /
		// rendering and config / configuration meet without a language- or
		// product-specific synonym table. It remains soft ranking only.
		if len(token) >= 6 && len(token) == utf8.RuneCountInString(token) {
			out["prefix:"+token[:5]] = true
		}
	}
	return out
}

func firstNonEmptyAnswerDocUnitLabel(unit types.InvestigationUnit) string {
	if label := strings.TrimSpace(unit.Label); label != "" {
		return label
	}
	if summary := strings.TrimSpace(unit.Summary); summary != "" {
		return summary
	}
	return fmt.Sprintf("unit-%d", unit.Index)
}

func renderAnswerDocInvestigationEvidenceOwnerRow(owned answerDocInvestigationEvidenceOwner) string {
	item := owned.item
	id := types.RuntimeArtifactIDForEvidenceItem(item)
	connectivity := "standalone_fact"
	if answerDocEvidenceHasExplicitTypedRelation(item) {
		connectivity = "explicit_typed_relation"
	}
	surface := strings.TrimSpace(types.EvidenceAuthoritativeSurfaceText(item, false))
	if surface == "" {
		surface = firstNonEmptyAnswerDocEvidenceLabel(item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object)
	}
	surface = answerDocInlineClip(surface, 240)
	loc := item.DisplayLocation(true)
	if loc != "" {
		return fmt.Sprintf("- evidence_id=`%s` association=`%s` connectivity=`%s` @ %s: %s\n", id, owned.association, connectivity, loc, surface)
	}
	return fmt.Sprintf("- evidence_id=`%s` association=`%s` connectivity=`%s`: %s\n", id, owned.association, connectivity, surface)
}

func answerDocEvidenceMatchesInvestigationUnitSource(item types.EvidenceItem, unit types.InvestigationUnit) bool {
	source := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(item.Source, "\\", "/")))
	if source == "" {
		return false
	}
	candidates := append(append([]string(nil), unit.Scopes...), unit.Entities...)
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(candidate, "\\", "/")))
		candidate = strings.TrimPrefix(candidate, "./")
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") {
			if source == candidate || strings.HasPrefix(source, strings.TrimSuffix(candidate, "/")+"/") || strings.HasSuffix(source, "/"+candidate) {
				return true
			}
			continue
		}
		// Bare analyzer entities are accepted as source scopes only when they
		// are exact filenames, not broad symbols such as REPL or Config.
		if strings.Contains(candidate, ".") {
			parts := strings.Split(source, "/")
			if parts[len(parts)-1] == candidate {
				return true
			}
		}
	}
	return false
}

func answerDocInvestigationEvidenceDisplayScore(item types.EvidenceItem) int {
	score := 0
	if answerDocEvidenceHasExplicitTypedRelation(item) {
		score += 100
	}
	if strings.TrimSpace(item.Producer) == types.EvidenceProducerExplorerEmitEvidence {
		score += 50
	}
	if item.Salience != "" {
		score += 20
	}
	if item.LoadBearingSummary {
		score += 10
	}
	return score
}

func firstNonEmptyAnswerDocEvidenceLabel(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "grounded evidence"
}

func answerDocEvidenceHasExplicitTypedRelation(item types.EvidenceItem) bool {
	if strings.TrimSpace(item.Subject) == "" || strings.TrimSpace(item.Object) == "" {
		return false
	}
	switch item.AnchorKind {
	case types.AnchorCall, types.AnchorCallback, types.AnchorPrecedence:
		return true
	}
	switch item.Kind {
	case types.EvidenceRelationship, types.EvidenceRegistration, types.EvidenceDataflowPath, types.EvidenceControlFlow:
		return true
	default:
		return false
	}
}
