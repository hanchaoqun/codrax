package tool

import (
	"fmt"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const requestedSubTopicCallableBodyMaxDemands = 4

type requestedSubTopicCallableBodyDebt struct {
	topic  string
	entity string
	file   string
	sym    *repotypes.Symbol
}

// requestedSubTopicCallableBodyDowngrade closes the gap between a typed,
// independently-answerable sub-topic and a call-site-only evidence row. A
// call proves that a local callable is selected; it does not prove what that
// callable does. When the analyzer's provenance lane resolves the sub-topic
// entity to one unique parser-owned repository callable, require the model to
// inspect and emit evidence from that callable's body before closing.
//
// This is deliberately narrower than a generic "read every mentioned
// function" rule:
//   - only typed multi-topic mechanism/call-chain explanations participate;
//   - the sub-topic entity must have exact symbol provenance and resolve to one
//     AST-backed callable;
//   - an Explorer-authored grounded call row must already select the callable;
//   - ambiguous, external, conceptual, file, and scope entities fail open;
//   - the system queues a bounded read/evidence repair only. It never creates
//     an answer claim or relation on the model's behalf. An exact parser-owned
//     body-call companion that was derived from a model-selected definition
//     may close the body-inspection debt because its producer already proves
//     both model selection and read coverage.
//
// Raw request text, model prose, completion rationale, and final-answer text
// are not inspected.
func requestedSubTopicCallableBodyDowngrade(
	ctx *types.BusContext,
	closure *types.EvidenceClosure,
	evidence []types.EvidenceItem,
) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || closure == nil || len(evidence) == 0 {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if len(rm.SubTopics) < 2 || !genericForcedReadBoundaryCanUseModelPrincipalSet(rm) {
		return ""
	}
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil || len(graph.FileIndex) == 0 {
		return ""
	}

	debts := requestedSubTopicCallableBodyDebts(rm.SubTopics, graph, evidence)
	if len(debts) == 0 {
		return ""
	}
	if len(debts) > requestedSubTopicCallableBodyMaxDemands {
		debts = debts[:requestedSubTopicCallableBodyMaxDemands]
	}

	var emitDebts []requestedSubTopicCallableBodyDebt
	for _, debt := range debts {
		end := debt.sym.EndLine
		if end < debt.sym.Line {
			end = debt.sym.Line
		}
		body := types.LineRange{Start: debt.sym.Line, End: end}
		if !callChainDemandRangeFullyRead(closure, debt.file, body) {
			closure.AddPendingRead(types.PendingRead{
				File: debt.file,
				Rationale: fmt.Sprintf(
					"requested sub-topic %q names local callable %q, but current evidence reaches only its call site; read the exact implementation body before closing",
					debt.topic, qualifiedEvidenceSymbolName(debt.sym),
				),
				Origin:                  fmt.Sprintf("pre_complete.requested_subtopic_callable_body.%d", debt.sym.Line),
				LineRanges:              []types.LineRange{body},
				MaterializationRequired: true,
				Stage:                   string(types.StageExplore),
			})
			continue
		}
		emitDebts = append(emitDebts, debt)
	}
	if len(emitDebts) == 0 {
		return ""
	}

	for _, debt := range emitDebts {
		end := debt.sym.EndLine
		if end < debt.sym.Line {
			end = debt.sym.Line
		}
		closure.AddRepair(types.RepairDirective{
			Kind:       types.RepairEmitEvidence,
			Files:      []string{debt.file},
			Keywords:   []string{debt.entity},
			Tools:      []string{"read_file", "emit_evidence"},
			Rationale:  "an explicitly requested callable sub-topic has only call-site evidence even though its exact local implementation body is already read; emit grounded body evidence before closing",
			Origin:     fmt.Sprintf("pre_complete.requested_subtopic_callable_body_evidence.%d", debt.sym.Line),
			Stage:      string(types.StageExplore),
			LineRanges: []types.LineRange{{Start: debt.sym.Line, End: end}},
		})
	}

	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — a requested callable sub-topic has call-site evidence but no implementation-body evidence.\n\n")
	b.WriteString("The exact local implementation body is already in read coverage. Stay on these bounded source ranges and emit grounded evidence for what the callable actually does before closing:\n")
	for _, debt := range emitDebts {
		end := debt.sym.EndLine
		if end < debt.sym.Line {
			end = debt.sym.Line
		}
		fmt.Fprintf(&b, "  - %s — `%s` at %s:%d-%d\n", debt.topic, debt.entity, debt.file, debt.sym.Line, end)
	}
	b.WriteString("\nA call edge proves selection of the callable, not its implementation behavior. Emit the body evidence or keep that sub-topic's behavior explicitly unproven; do not infer absence from a call-site-only row.")
	return b.String()
}

func requestedSubTopicCallableBodyDebts(
	topics []types.SubTopic,
	graph *repotypes.Graph,
	evidence []types.EvidenceItem,
) []requestedSubTopicCallableBodyDebt {
	var debts []requestedSubTopicCallableBodyDebt
	for _, candidate := range requestedSubTopicCallableBodyCandidates(topics, graph, evidence) {
		if candidate.sym.HasParserOwnedBody() && !requestedSubTopicCallableHasBodyEvidence(evidence, candidate.file, candidate.sym) {
			debts = append(debts, candidate)
		}
	}
	return debts
}

// Resolve identity before inspecting body presence. Removing declarations
// from the identity census would silently choose an implementation/overload.
func requestedSubTopicCallableBodyCandidates(
	topics []types.SubTopic,
	graph *repotypes.Graph,
	evidence []types.EvidenceItem,
) []requestedSubTopicCallableBodyDebt {
	if graph == nil || len(topics) == 0 || len(evidence) == 0 {
		return nil
	}
	debts := make([]requestedSubTopicCallableBodyDebt, 0, requestedSubTopicCallableBodyMaxDemands)
	seen := make(map[string]bool)
	for _, topic := range topics {
		for idx, entity := range topic.Entities {
			identity, ok := requestedSubTopicResolvedSymbolIdentity(topic, idx, entity)
			if !ok {
				continue
			}
			file, fi, sym, ok := requestedSubTopicUniqueCallable(graph, identity)
			if !ok || !requestedSubTopicCallableHasCallEvidence(evidence, sym, fi) {
				continue
			}
			key := mechanismSemanticDescentSymbolKey(file, sym)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			debts = append(debts, requestedSubTopicCallableBodyDebt{
				topic: strings.TrimSpace(topic.Summary), entity: strings.TrimSpace(entity),
				file: file, sym: sym,
			})
		}
	}
	return debts
}

func requestedSubTopicResolvedSymbolIdentity(topic types.SubTopic, idx int, entity string) (string, bool) {
	if idx < 0 || idx >= len(topic.EntityProvenance) {
		return "", false
	}
	prov := topic.EntityProvenance[idx]
	if prov.Resolution != types.EntityResolutionSymbol || !prov.Resolved || !prov.UseForShape ||
		!strings.EqualFold(strings.TrimSpace(prov.Surface), strings.TrimSpace(entity)) {
		return "", false
	}
	identity := strings.TrimSpace(prov.ResolvedAs)
	if identity == "" {
		identity = strings.TrimSpace(prov.Surface)
	}
	return identity, identity != ""
}

func requestedSubTopicUniqueCallable(graph *repotypes.Graph, identity string) (string, *repotypes.FileInfo, *repotypes.Symbol, bool) {
	if graph == nil || strings.TrimSpace(identity) == "" {
		return "", nil, nil, false
	}
	var matchFile string
	var matchFI *repotypes.FileInfo
	var match *repotypes.Symbol
	for file, fi := range graph.FileIndex {
		if fi == nil || !mechanismSemanticDescentASTFile(fi) {
			continue
		}
		for idx := range fi.Symbols {
			sym := &fi.Symbols[idx]
			if !mechanismSemanticDescentCallable(sym) ||
				!types.AnswerCodeIdentitySurfacesCompatible(identity, qualifiedEvidenceSymbolNameInFile(fi, sym)) {
				continue
			}
			candidateFile := canonicalRelationSourcePath(fi.RelPath)
			if candidateFile == "" {
				candidateFile = canonicalRelationSourcePath(file)
			}
			if match != nil && mechanismSemanticDescentSymbolKey(matchFile, match) != mechanismSemanticDescentSymbolKey(candidateFile, sym) {
				return "", nil, nil, false
			}
			matchFile, matchFI, match = candidateFile, fi, sym
		}
	}
	return matchFile, matchFI, match, match != nil && matchFI != nil && matchFile != ""
}

func requestedSubTopicCallableHasBodyEvidence(evidence []types.EvidenceItem, file string, sym *repotypes.Symbol) bool {
	if !sym.HasParserOwnedBody() || sym.Line <= 0 {
		return false
	}
	end := sym.EndLine
	if end < sym.Line {
		end = sym.Line
	}
	for _, item := range evidence {
		if !requestedSubTopicCallableBodyEvidenceProducer(item) || !item.IsCitable() ||
			item.LineStart < sym.Line || item.LineStart > end ||
			!callChainSourcePathEquivalent(canonicalRelationSourcePath(item.Source), canonicalRelationSourcePath(file)) {
			continue
		}
		// Declaration extent includes multi-line parameters and return types.
		// Those lines are not body inspection. Intersect the parser-owned body
		// while retaining the original declaration-first-line boundary. A Python
		// or Ruby body starts at its first statement, not at an opening brace.
		itemEnd := max(item.LineStart, item.LineEnd)
		if item.LineStart <= sym.BodyEndLine && itemEnd >= sym.BodyStartLine &&
			(end == sym.Line || item.LineStart > sym.Line || itemEnd > sym.Line) {
			return true
		}
	}
	return false
}

// callableBodyInspectionAdvisory is a current-snapshot, bounded soft exit for
// selected declarations and unclassified callables. It neither creates a
// completion obligation nor stores a sticky session-wide missing-body fact.
// The accepted tool summary already carries authoring notes to later stages.
func callableBodyInspectionAdvisory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || !genericForcedReadBoundaryCanUseModelPrincipalSet(ctx.AnalysisIR.RequestModel) {
		return ""
	}
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil {
		return ""
	}
	var candidates []requestedSubTopicCallableBodyDebt
	if len(ctx.AnalysisIR.RequestModel.SubTopics) >= 2 {
		candidates = requestedSubTopicCallableBodyCandidates(ctx.AnalysisIR.RequestModel.SubTopics, graph, evidence)
	}
	for _, seed := range mechanismSemanticDescentSelectedDefinitionSeeds(ctx, graph, facts, evidence) {
		candidates = append(candidates, requestedSubTopicCallableBodyDebt{file: seed.file, sym: seed.sym})
	}
	seen := make(map[string]bool)
	var notes []string
	for _, candidate := range candidates {
		if candidate.sym == nil || candidate.sym.HasParserOwnedBody() {
			continue
		}
		key := mechanismSemanticDescentSymbolKey(candidate.file, candidate.sym)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if len(notes) >= requestedSubTopicCallableBodyMaxDemands {
			continue
		}
		boundary := "body presence is not established; inspect the source if its implementation behavior is needed, and do not infer absence"
		if candidate.sym.BodyPresence == repotypes.CallableBodyAbsent {
			boundary = "the parser identifies a declaration without a local body; keep its declaration semantics, and use separately grounded implementation evidence for behavior"
		}
		notes = append(notes, fmt.Sprintf("%q at %s:%d: %s", qualifiedEvidenceSymbolName(candidate.sym), candidate.file, candidate.sym.Line, boundary))
	}
	if len(notes) == 0 {
		return ""
	}
	return fmt.Sprintf("Callable inspection advisory (non-blocking; showing %d of %d selected declarations/unknown bodies): %s. No implementation is selected or behavior established by this note.", len(notes), len(seen), strings.Join(notes, "; "))
}

// requestedSubTopicCallableBodyEvidenceProducer admits exactly two ownership
// lanes. Explorer evidence keeps the model-authored path. The parser companion
// is narrower: autoPairSelectedDefinitionBodyCallEvidence can stamp it only
// for a definition the Explorer selected and only for an invocation line that
// is already in the read closure. Broad repo-map/navigation rows remain
// ineligible, so repository-wide graph discovery cannot silently satisfy a
// requested implementation inspection.
func requestedSubTopicCallableBodyEvidenceProducer(item types.EvidenceItem) bool {
	switch item.Producer {
	case types.EvidenceProducerExplorerEmitEvidence:
		return true
	case types.EvidenceProducerRepoMapSelectedCallableBodyCall:
		return item.AnchorKind == types.AnchorCall && len(item.DerivedFrom) > 0
	default:
		return false
	}
}

func requestedSubTopicCallableHasCallEvidence(evidence []types.EvidenceItem, sym *repotypes.Symbol, fi *repotypes.FileInfo) bool {
	if sym == nil || fi == nil {
		return false
	}
	qualified := qualifiedEvidenceSymbolNameInFile(fi, sym)
	for _, item := range evidence {
		if item.Producer != types.EvidenceProducerExplorerEmitEvidence || !item.IsCitable() ||
			types.ClaimFormOf(item) != types.ClaimCallEdge {
			continue
		}
		if mechanismSemanticDescentIdentityMatches(item.Object, sym.Name, qualified) ||
			mechanismSemanticDescentIdentityMatches(item.AnchorSymbol, sym.Name, qualified) {
			return true
		}
	}
	return false
}
