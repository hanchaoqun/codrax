package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const maxAnswerDocCallableCitationRows = 12

type answerDocCallableCitationRow struct {
	identity              string
	roles                 []string
	callsiteRefs          []string
	sources               map[string]bool
	definitionAuthorities []string
	definitionRef         string
	definitionID          string
	definitionState       string
	bodyCallFacts         []string
}

// renderAnswerDocCallChainCitationAuthority pairs call-site and definition
// coordinates through typed support entries. It reduces the model's need to
// infer citation roles from a long evidence list, but remains soft authoring
// context: it neither inspects draft prose nor rewrites answer items.
func renderAnswerDocCallChainCitationAuthority(plan *types.AnswerSupportPlan) string {
	if plan == nil || plan.Family != types.QFCallChain {
		return ""
	}
	entries := answerDocCurrentCodePathEntries(plan)
	rows := answerDocCallableCitationRows(entries)
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Callable role and citation authority (typed advisory)\n\n")
	b.WriteString("- A call-site reference proves only that its exact caller invokes its exact target. A definition reference proves the callable declaration/body at that location. Do not describe a call-site line as where the callee is defined, and do not use a definition line as proof that a caller invoked it.\n")
	b.WriteString("- For an ordered hop, cite `callsite_refs`. Discuss a callable's signature or general body only when `definition_status=proved`, and cite `definition_ref`. An exact `body_call_fact` independently proves only that listed invocation inside the callable and may be described from its own reference even when the declaration line is absent. When neither definition nor body fact is proved, say only that the grounded chain invokes/reaches the endpoint.\n")
	for i, row := range rows {
		fmt.Fprintf(&b, "- callable[%d]: identity=`%s`; observed_roles=`%s`; callsite_refs=`%s`; definition_status=`%s`",
			i+1, answerDocCallChainInline(row.identity), strings.Join(row.roles, "|"), strings.Join(row.callsiteRefs, " | "), row.definitionState)
		if row.definitionRef != "" {
			fmt.Fprintf(&b, "; definition_ref=`%s`; definition_evidence=`%s`", answerDocCallChainInline(row.definitionRef), answerDocCallChainInline(row.definitionID))
		}
		if len(row.bodyCallFacts) > 0 {
			fmt.Fprintf(&b, "; body_call_facts=`%s`", strings.Join(row.bodyCallFacts, " | "))
		}
		b.WriteString(".\n")
	}
	b.WriteString("\n")
	return b.String()
}

func answerDocCurrentCodePathEntries(plan *types.AnswerSupportPlan) []types.AnswerSupportEntry {
	var out []types.AnswerSupportEntry
	for _, lane := range plan.Lanes {
		if lane.Kind == types.SupportLaneCurrentCodePath {
			out = append(out, lane.Entries...)
		}
	}
	return out
}

func answerDocCallableCitationRows(entries []types.AnswerSupportEntry) []answerDocCallableCitationRow {
	rows := make(map[string]*answerDocCallableCitationRow)
	var order []string
	add := func(identity, role, source, location, definitionAuthority string) {
		identity = strings.TrimSpace(identity)
		key := strings.ToLower(answerDocExactChainIdentity(identity))
		if key == "" {
			return
		}
		row := rows[key]
		if row == nil {
			if len(order) >= maxAnswerDocCallableCitationRows {
				return
			}
			row = &answerDocCallableCitationRow{identity: identity, sources: make(map[string]bool)}
			rows[key] = row
			order = append(order, key)
		}
		row.roles = appendAnswerDocUniqueString(row.roles, role)
		row.callsiteRefs = appendAnswerDocUniqueString(row.callsiteRefs, strings.TrimSpace(location))
		row.definitionAuthorities = appendAnswerDocUniqueString(row.definitionAuthorities, definitionAuthority)
		if source = strings.TrimSpace(source); source != "" {
			row.sources[source] = true
		}
	}
	for _, entry := range entries {
		if entry.ClaimForm != types.ClaimCallEdge {
			continue
		}
		ownerAuthority := ""
		if entry.Producer == types.EvidenceProducerExplorerEmitEvidence &&
			types.AnswerCodeIdentitySurfacesCompatible(entry.Subject, entry.OwnerSymbol) {
			ownerAuthority = entry.OwnerSymbol
		}
		add(entry.Subject, "caller_operation", entry.Source, entry.Location, ownerAuthority)
		add(entry.Object, "invoked_target", entry.Source, entry.Location, "")
		key := strings.ToLower(answerDocExactChainIdentity(entry.Subject))
		if row := rows[key]; row != nil && strings.TrimSpace(entry.Object) != "" && strings.TrimSpace(entry.Location) != "" {
			row.bodyCallFacts = appendAnswerDocUniqueString(row.bodyCallFacts,
				fmt.Sprintf("%s -> %s @ %s", strings.TrimSpace(entry.Subject), strings.TrimSpace(entry.Object), strings.TrimSpace(entry.Location)))
		}
	}

	definitions := make([]types.AnswerSupportEntry, 0)
	for _, entry := range entries {
		if entry.ClaimForm == types.ClaimDefinitionFact && answerDocDefinitionEntryIdentity(entry) != "" && strings.TrimSpace(entry.Location) != "" {
			definitions = append(definitions, entry)
		}
	}
	for _, key := range order {
		row := rows[key]
		row.definitionState = "unproven"
		var candidates []types.AnswerSupportEntry
		for _, definition := range definitions {
			if !row.sources[strings.TrimSpace(definition.Source)] ||
				!answerDocDefinitionMatchesCallableRow(definition, row) ||
				answerDocDefinitionCompatibleCallableCount(definition, rows) != 1 {
				continue
			}
			candidates = append(candidates, definition)
		}
		if len(candidates) != 1 {
			continue
		}
		row.definitionState = "proved"
		row.definitionRef = strings.TrimSpace(candidates[0].Location)
		row.definitionID = strings.TrimSpace(candidates[0].EvidenceID)
	}

	out := make([]answerDocCallableCitationRow, 0, len(order))
	for _, key := range order {
		row := rows[key]
		sort.Strings(row.roles)
		out = append(out, *row)
	}
	return out
}

func answerDocDefinitionMatchesCallableRow(definition types.AnswerSupportEntry, row *answerDocCallableCitationRow) bool {
	if row == nil {
		return false
	}
	identity := answerDocDefinitionEntryIdentity(definition)
	if answerDocExactChainIdentity(identity) == answerDocExactChainIdentity(row.identity) {
		return true
	}
	for _, authority := range row.definitionAuthorities {
		if types.AnswerCodeIdentitySurfacesCompatible(authority, identity) {
			return true
		}
	}
	return false
}

func answerDocDefinitionCompatibleCallableCount(definition types.AnswerSupportEntry, rows map[string]*answerDocCallableCitationRow) int {
	source := strings.TrimSpace(definition.Source)
	count := 0
	for _, row := range rows {
		if row.sources[source] && answerDocDefinitionMatchesCallableRow(definition, row) {
			count++
		}
	}
	return count
}

func answerDocDefinitionEntryIdentity(entry types.AnswerSupportEntry) string {
	for _, raw := range []string{entry.AnchorSymbol, entry.Subject, entry.Object} {
		raw = strings.TrimSpace(raw)
		if types.IsCodeIdentitySurface(raw) {
			return raw
		}
	}
	return ""
}

func appendAnswerDocUniqueString(dst []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dst
	}
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}
