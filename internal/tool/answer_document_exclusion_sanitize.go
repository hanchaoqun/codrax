package tool

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/logging"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeTypedExcludedAnswerSurface removes concrete excluded candidates
// from the user-visible answer surface when the analyzer emitted a typed
// AnswerExclusionPolicy. This is a compatibility/salvage pass, not a hard
// gate: the model's answer is otherwise accepted, and the scope signal remains
// visible without forcing a finalizer rewrite.
func normalizeTypedExcludedAnswerSurface(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return 0
	}
	policy := ctx.AnalysisIR.RequestModel.AnswerExclusionPolicy
	if policy == nil || !policy.Active() {
		return 0
	}
	roles := answerDocumentExcludedRoleSet(policy)
	if len(roles) == 0 {
		return 0
	}
	changed := pruneExcludedPrincipalRows(doc, policy)
	candidates := answerDocumentExcludedCandidateNames(ctx, roles)
	if len(candidates) == 0 {
		return changed
	}
	rewrite := func(s string) string {
		return redactExcludedCandidateTokens(s, candidates)
	}
	for i := range doc.Blocks {
		before := doc.Blocks[i].Title
		doc.Blocks[i].Title = rewrite(doc.Blocks[i].Title)
		if doc.Blocks[i].Title != before {
			changed++
		}
		before = doc.Blocks[i].Text
		doc.Blocks[i].Text = rewrite(doc.Blocks[i].Text)
		if doc.Blocks[i].Text != before {
			changed++
		}
		for j := range doc.Blocks[i].Columns {
			before = doc.Blocks[i].Columns[j]
			doc.Blocks[i].Columns[j] = rewrite(doc.Blocks[i].Columns[j])
			if doc.Blocks[i].Columns[j] != before {
				changed++
			}
		}
		for j := range doc.Blocks[i].Items {
			before = doc.Blocks[i].Items[j].Label
			doc.Blocks[i].Items[j].Label = rewrite(doc.Blocks[i].Items[j].Label)
			if doc.Blocks[i].Items[j].Label != before {
				changed++
			}
			before = doc.Blocks[i].Items[j].Text
			doc.Blocks[i].Items[j].Text = rewrite(doc.Blocks[i].Items[j].Text)
			if doc.Blocks[i].Items[j].Text != before {
				changed++
			}
			for k := range doc.Blocks[i].Items[j].Cells {
				before = doc.Blocks[i].Items[j].Cells[k]
				doc.Blocks[i].Items[j].Cells[k] = rewrite(doc.Blocks[i].Items[j].Cells[k])
				if doc.Blocks[i].Items[j].Cells[k] != before {
					changed++
				}
			}
		}
		if doc.Blocks[i].Diagram != nil {
			before = doc.Blocks[i].Diagram.Body
			doc.Blocks[i].Diagram.Body = rewrite(doc.Blocks[i].Diagram.Body)
			if doc.Blocks[i].Diagram.Body != before {
				changed++
			}
		}
	}
	for i := range doc.Caveats {
		before := doc.Caveats[i]
		doc.Caveats[i] = rewrite(doc.Caveats[i])
		if doc.Caveats[i] != before {
			changed++
		}
	}
	for i := range doc.Citations {
		before := doc.Citations[i].Quote
		doc.Citations[i].Quote = rewrite(doc.Citations[i].Quote)
		if doc.Citations[i].Quote != before {
			changed++
		}
		before = doc.Citations[i].CrossfileSummary
		doc.Citations[i].CrossfileSummary = rewrite(doc.Citations[i].CrossfileSummary)
		if doc.Citations[i].CrossfileSummary != before {
			changed++
		}
		before = doc.Citations[i].NegativePattern
		doc.Citations[i].NegativePattern = rewrite(doc.Citations[i].NegativePattern)
		if doc.Citations[i].NegativePattern != before {
			changed++
		}
	}
	if changed > 0 {
		logging.Warning("[emit_answer_document] redacted %d excluded candidate surface(s) from final answer via typed exclusion policy", changed)
	}
	return changed
}

func normalizeAggregateFactsForTypedExclusion(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if len(facts) == 0 || ctx == nil || ctx.AnalysisIR == nil {
		return facts
	}
	policy := ctx.AnalysisIR.RequestModel.AnswerExclusionPolicy
	if policy == nil || !policy.Active() {
		return facts
	}
	roles := answerDocumentExcludedRoleSet(policy)
	candidates := answerDocumentExcludedCandidateNames(ctx, roles)
	if len(candidates) == 0 {
		return facts
	}
	excluded := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		excluded[candidate] = true
	}
	out := make([]types.AnswerAggregateFact, len(facts))
	copy(out, facts)
	for i := range out {
		if len(out[i].Members) == 0 {
			continue
		}
		keptMembers := out[i].Members[:0]
		keptRefs := out[i].SupportRefs[:0]
		removed := 0
		for idx, member := range out[i].Members {
			if aggregateMemberMatchesExcludedCandidate(member, excluded) {
				removed++
				continue
			}
			keptMembers = append(keptMembers, member)
			if idx < len(out[i].SupportRefs) {
				keptRefs = append(keptRefs, out[i].SupportRefs[idx])
			}
		}
		if removed == 0 {
			continue
		}
		out[i].Members = keptMembers
		out[i].SupportRefs = keptRefs
		if types.AnswerAggregateFactCarriesCompleteMemberSet(facts[i]) {
			out[i].Value = strconv.Itoa(len(keptMembers))
		}
	}
	return out
}

func answerDocumentExcludedRoleSet(policy *types.AnswerExclusionPolicy) map[types.AnswerCandidateRole]bool {
	out := map[types.AnswerCandidateRole]bool{}
	if policy == nil || !policy.Active() {
		return out
	}
	for _, role := range policy.ExcludedCandidateRoles {
		if role == types.AnswerCandidateRoleUnknown {
			continue
		}
		out[role] = true
	}
	return out
}

func aggregateMemberMatchesExcludedCandidate(member string, excluded map[string]bool) bool {
	member = strings.Trim(member, "` \t\r\n")
	if member == "" || len(excluded) == 0 {
		return false
	}
	if excluded[member] {
		return true
	}
	if head := answerDocumentExcludedCandidateHead(member); head != "" && excluded[head] {
		return true
	}
	return false
}

func pruneExcludedPrincipalRows(doc *types.AnswerDocumentV2, policy *types.AnswerExclusionPolicy) int {
	if doc == nil || policy == nil || !policy.Active() {
		return 0
	}
	removed := 0
	for i := range doc.Blocks {
		if doc.Blocks[i].SurfaceRole != types.SurfacePrincipal {
			continue
		}
		items := doc.Blocks[i].Items
		if len(items) == 0 {
			continue
		}
		kept := items[:0]
		for _, item := range items {
			if policy.ExcludesRole(item.CandidateRole) {
				removed++
				continue
			}
			kept = append(kept, item)
		}
		doc.Blocks[i].Items = kept
	}
	return removed
}

func answerDocumentExcludedCandidateNames(ctx *types.BusContext, roles map[types.AnswerCandidateRole]bool) []string {
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
	}
	if ctx != nil && ctx.Mutable != nil {
		addExcludedAggregateCandidates(ctx.Mutable.StableInvestigationAggregateFacts(), add)
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			addExcludedAggregateCandidates(ta.AcceptedAggregateFacts, add)
		}
	}
	for _, graph := range answerDocumentExclusionGraphs(ctx) {
		addExcludedGraphSymbols(graph, roles, add)
	}
	out := make([]string, 0, len(seen))
	for candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func addExcludedAggregateCandidates(facts []types.AnswerAggregateFact, add func(string)) {
	for _, fact := range facts {
		for _, excluded := range fact.Excluded {
			add(excluded)
			if head := answerDocumentExcludedCandidateHead(excluded); head != "" {
				add(head)
			}
		}
	}
}

func answerDocumentExcludedCandidateHead(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, sep := range []string{" @ ", "\t", " ", ":", "：", "("} {
		if idx := strings.Index(raw, sep); idx > 0 {
			return strings.TrimSpace(raw[:idx])
		}
	}
	return raw
}

func answerDocumentExclusionGraphs(ctx *types.BusContext) []*repotypes.Graph {
	if ctx == nil {
		return nil
	}
	seen := map[*repotypes.Graph]bool{}
	var out []*repotypes.Graph
	add := func(g *repotypes.Graph) {
		if g == nil || seen[g] {
			return
		}
		seen[g] = true
		out = append(out, g)
	}
	if ctx.Mutable != nil {
		if graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph); ok {
			add(graph)
		}
	}
	type graphSet interface {
		AllGraphs() map[string]*repotypes.Graph
	}
	if mg, ok := ctx.MultiGraph.(graphSet); ok {
		for _, graph := range mg.AllGraphs() {
			add(graph)
		}
	}
	return out
}

func addExcludedGraphSymbols(graph *repotypes.Graph, roles map[types.AnswerCandidateRole]bool, add func(string)) {
	if graph == nil || len(roles) == 0 {
		return
	}
	for _, defs := range graph.SymbolDefs {
		for _, sym := range defs {
			if answerDocumentSymbolMatchesExcludedRole(sym, roles) {
				add(sym.Name)
			}
		}
	}
}

func answerDocumentSymbolMatchesExcludedRole(sym *repotypes.Symbol, roles map[types.AnswerCandidateRole]bool) bool {
	if sym == nil || strings.TrimSpace(sym.Name) == "" {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(sym.Kind))
	switch {
	case roles[types.AnswerCandidateRoleFunction] && kind == "function":
		return true
	case roles[types.AnswerCandidateRoleMethod] && kind == "method":
		return true
	case roles[types.AnswerCandidateRoleConstant] && (kind == "const" || kind == "constant"):
		return true
	case roles[types.AnswerCandidateRoleVariable] && (kind == "var" || kind == "variable"):
		return true
	case roles[types.AnswerCandidateRoleField] && (kind == "field" || kind == "property"):
		return true
	case roles[types.AnswerCandidateRolePackage] && (kind == "package" || kind == "module" || kind == "namespace"):
		return true
	case roles[types.AnswerCandidateRoleType] && answerDocumentSymbolKindIsType(kind):
		return true
	case roles[types.AnswerCandidateRolePrivate] && !sym.Exported:
		return true
	default:
		return false
	}
}

func answerDocumentSymbolKindIsType(kind string) bool {
	switch kind {
	case "type", "interface", "class", "struct", "enum", "trait":
		return true
	default:
		return false
	}
}

func redactExcludedCandidateTokens(text string, candidates []string) string {
	if strings.TrimSpace(text) == "" || len(candidates) == 0 {
		return text
	}
	out := text
	for _, candidate := range candidates {
		out = replaceIdentifierToken(out, candidate, "[excluded]")
	}
	return out
}

func replaceIdentifierToken(text, target, replacement string) string {
	if target == "" || text == "" || !strings.Contains(text, target) {
		return text
	}
	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(text[start:], target)
		if idx < 0 {
			b.WriteString(text[start:])
			break
		}
		idx += start
		end := idx + len(target)
		if isIdentifierBoundary(text, idx, end) {
			b.WriteString(text[start:idx])
			b.WriteString(replacement)
		} else {
			b.WriteString(text[start:end])
		}
		start = end
	}
	return b.String()
}

func isIdentifierBoundary(text string, start, end int) bool {
	beforeOK := start <= 0
	if !beforeOK {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		beforeOK = !isAnswerIdentifierRune(r)
	}
	afterOK := end >= len(text)
	if !afterOK {
		r, _ := utf8.DecodeRuneInString(text[end:])
		afterOK = !isAnswerIdentifierRune(r)
	}
	return beforeOK && afterOK
}

func isAnswerIdentifierRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
