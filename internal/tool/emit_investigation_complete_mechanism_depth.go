package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	mechanismSemanticDescentMaxDepth             = 2
	mechanismSemanticDescentMaxDemands           = 4
	mechanismSemanticDescentFullBodyMaxLines     = 96
	mechanismSemanticDescentDeclarationLookahead = 16
	mechanismSemanticDescentCallContextLines     = 8
	mechanismSemanticDescentMaxReturnCallSites   = 4
)

type mechanismSemanticDescentNode struct {
	file  string
	fi    *repotypes.FileInfo
	sym   *repotypes.Symbol
	depth int
	root  string
	// selectionLine is the exact model-selected evidence/declaration row that
	// brought this callable into the bounded closure. It is parser-grounded and
	// never derived from request, reasoning, or answer prose.
	selectionLine int
}

// raiseMechanismSemanticDescentPendingReads closes a narrow but important
// mechanism-explanation gap: a model-owned principal member can cite and read
// a thin wrapper, then close before reading the local callable that actually
// implements the returned behavior.
//
// The descent is deliberately bounded and source-owned:
//   - the request must already be a typed current-source mechanism/explanation;
//   - the model must have emitted an aligned current-source member_set and
//     either a non-empty responsibility note or an exact Explorer-authored
//     mechanism-definition row for the supported member;
//   - every traversed edge must be a direct parser-owned call on a return line;
//   - the direct callee must resolve through the repository graph; a callable
//     argument handoff is not execution and never creates a child read;
//   - only declaration bodies are requested. No EvidenceItem, relation,
//     conclusion, or answer text is synthesized here.
//
// Raw user/model/final prose and heuristic scores never participate. The
// function inspects only typed request/fact fields, parser graph metadata, and
// source lines that read_file already placed in the grounding context.
func raiseMechanismSemanticDescentPendingReads(
	ctx *types.BusContext,
	closure *types.EvidenceClosure,
	aggregateFacts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) int {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || closure == nil || len(aggregateFacts) == 0 {
		return 0
	}
	rm := ctx.AnalysisIR.RequestModel
	if !genericForcedReadBoundaryCanUseModelPrincipalSet(rm) {
		return 0
	}
	// A typed call-chain request has its own exact endpoint, call-edge,
	// selected-body, directed-reachability, and no-path completion gates. It may
	// share the generic forced-read eligibility used by those gates, but it is
	// not a mechanism narrative: recursively following every executable helper
	// selected during chain discovery expands beyond the model-selected path and
	// can continually mint a new pending-read frontier. Keep this exclusion local
	// to semantic descent so the call-chain evidence gates retain their existing
	// authority. No request or answer prose participates.
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqCallChain {
		return 0
	}
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil || len(graph.FileIndex) == 0 {
		return 0
	}
	frontier := make([]mechanismSemanticDescentNode, 0, 4)
	seen := make(map[string]bool)
	behavioralRoster := false
	addSeed := func(node mechanismSemanticDescentNode) {
		key := mechanismSemanticDescentSymbolKey(node.file, node.sym)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		frontier = append(frontier, node)
	}
	for _, fact := range aggregateFacts {
		if !aggregateNarrativeMemberSetRequiresAlignedCurrentSourceSupport(ctx, fact) ||
			len(fact.Members) != len(fact.MemberNotes) || len(fact.Members) != len(fact.SupportRefs) {
			continue
		}
		for idx, member := range fact.Members {
			if strings.TrimSpace(fact.MemberNotes[idx]) == "" {
				continue
			}
			behavioralRoster = true
			file, fi, sym, ok := mechanismNarrativeMemberCallable(graph, member, fact.SupportRefs[idx])
			if !ok {
				continue
			}
			addSeed(mechanismSemanticDescentNode{
				file: file, fi: fi, sym: sym, root: strings.TrimSpace(member), selectionLine: sym.Line,
			})
		}
	}
	// B879c: EvidenceKindMechanism + ClaimDefinitionFact is a stronger typed
	// behavioral selection than an optional member note. When the Explorer's
	// exact definition row and a member/support_ref resolve to the same local
	// callable, use that callable as a seed even across non-flow predicate axes.
	// Free-form evidence summaries and aggregate labels remain ignored.
	for _, node := range mechanismSemanticDescentDefinitionSeeds(ctx, graph, aggregateFacts, evidence) {
		behavioralRoster = true
		addSeed(node)
	}
	// B879d: an exact executable row selected by the Explorer is already a
	// precise statement that this operation participates in the requested
	// mechanism. Seed its enclosing callable even when the final aggregate is
	// an enum/state roster rather than a callable roster. The exact row owns only
	// that enclosing body; sibling calls need their own parser-owned operation
	// row and enter through mechanismSemanticDescentOperationLeafSeeds.
	// Definition/text-reference evidence cannot enter this lane.
	for _, node := range mechanismSemanticDescentExecutionSeeds(ctx, graph, aggregateFacts, evidence) {
		addSeed(node)
	}
	// B879e: a callable definition explicitly classified by the Explorer as
	// mechanism evidence is also a precise model selection. This covers runs
	// whose evidence shape contains no executable guard/return/assignment yet.
	// Keep only the first exact callable per source file so role-description
	// rosters cannot crowd out the bounded closure. System auto-pairs and plain
	// direct definitions are excluded by producer/kind below. A supporting-only
	// roster may authorize the selected callable's own body read, but cannot
	// expand the closure into child callables.
	for _, node := range mechanismSemanticDescentSelectedDefinitionSeeds(ctx, graph, aggregateFacts, evidence) {
		addSeed(node)
	}
	// B879b: a mechanism roster may legitimately consist of enum outcomes or
	// other non-callable identities. In that shape the earlier flow-operation
	// gate can still make the model select a parser-grounded call edge. Continue
	// from the target of that exact Explorer-authored edge rather than requiring
	// the roster member itself to be a callable. This consumes typed endpoints
	// only; Summary/member-note/reason prose never participates.
	if behavioralRoster && len(evidence) > 0 {
		for _, node := range mechanismSemanticDescentOperationLeafSeeds(graph, rm, evidence) {
			addSeed(node)
		}
	}
	if len(frontier) == 0 {
		return 0
	}

	demands := 0
	for len(frontier) > 0 && demands < mechanismSemanticDescentMaxDemands {
		node := frontier[0]
		frontier = frontier[1:]
		if node.sym == nil || node.fi == nil || node.sym.Line <= 0 {
			continue
		}
		bodyRanges := mechanismSemanticDescentReadRanges(node)
		if !mechanismSemanticDescentRangesFullyRead(closure, node.file, bodyRanges) {
			mechanismSemanticDescentAddSelectedBodyPendingRead(closure, node.file, node.sym, node.root, bodyRanges)
			demands++
			continue
		}
		if node.depth >= mechanismSemanticDescentMaxDepth {
			continue
		}

		for _, line := range mechanismSemanticDescentReturnCallLines(node) {
			if demands >= mechanismSemanticDescentMaxDemands || !closure.HasReadLine(node.file, line) {
				break
			}
			for relIdx := range node.fi.Relations {
				rel := &node.fi.Relations[relIdx]
				if !mechanismSemanticDescentCallRelation(rel, line) {
					continue
				}
				children := mechanismReturnCallChildren(graph, node.fi, node.sym, rel)
				for _, child := range children {
					key := mechanismSemanticDescentSymbolKey(child.file, child.sym)
					if key == "" || seen[key] {
						continue
					}
					seen[key] = true
					child.depth = node.depth + 1
					child.root = node.root
					child.selectionLine = child.sym.Line
					childRanges := mechanismSemanticDescentReadRanges(child)
					if !mechanismSemanticDescentRangesFullyRead(closure, child.file, childRanges) {
						mechanismSemanticDescentAddPendingRead(closure, child.file, child.sym, node.root, childRanges)
						demands++
						if demands >= mechanismSemanticDescentMaxDemands {
							break
						}
						continue
					}
					frontier = append(frontier, child)
				}
			}
		}
	}
	if demands > 0 {
		logging.Info("[emit_investigation_complete] queued %d bounded mechanism semantic-descent read(s)", demands)
	}
	return demands
}

func mechanismSemanticDescentExecutionSeeds(
	ctx *types.BusContext,
	graph *repotypes.Graph,
	aggregateFacts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) []mechanismSemanticDescentNode {
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil || len(evidence) == 0 ||
		!mechanismCompletionHasCurrentSourcePrincipalFact(ctx, aggregateFacts, evidence) {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	principalScope := types.PrincipalSourceScope(rm.SourceScopeProfile)
	seeds := make([]mechanismSemanticDescentNode, 0, 4)
	seen := make(map[string]bool)
	for _, item := range evidence {
		if item.Producer != types.EvidenceProducerExplorerEmitEvidence || !item.IsCitable() ||
			!mechanismSemanticDescentExecutableClaim(types.ClaimFormOf(item)) ||
			types.RuntimeArtifactPathKind(item.Source) != "" ||
			!types.SourceScopeAllowsPathRole(principalScope, types.ClassifySourcePathRole(item.Source)) {
			continue
		}
		file, fi := mechanismSemanticDescentGraphFile(graph, item.Source)
		if fi == nil || !mechanismSemanticDescentASTFile(fi) || item.LineStart <= 0 {
			continue
		}
		sym := enclosingCallableSymbol(fi, item.LineStart)
		if !mechanismSemanticDescentCallable(sym) {
			continue
		}
		end := sym.EndLine
		if end < sym.Line {
			end = sym.Line
		}
		if item.LineStart < sym.Line || item.LineStart > end {
			continue
		}
		key := mechanismSemanticDescentSymbolKey(file, sym)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		root := strings.TrimSpace(item.AnchorSymbol)
		if root == "" {
			root = strings.TrimSpace(sym.Name)
		}
		seeds = append(seeds, mechanismSemanticDescentNode{
			file: file, fi: fi, sym: sym, root: root, depth: mechanismSemanticDescentMaxDepth,
			selectionLine: item.LineStart,
		})
	}
	return seeds
}

func mechanismSemanticDescentSelectedDefinitionSeeds(
	ctx *types.BusContext,
	graph *repotypes.Graph,
	aggregateFacts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) []mechanismSemanticDescentNode {
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil || len(evidence) == 0 {
		return nil
	}
	principalScope := types.PrincipalSourceScope(ctx.AnalysisIR.RequestModel.SourceScopeProfile)
	seeds := make([]mechanismSemanticDescentNode, 0, mechanismSemanticDescentMaxDemands)
	seenFiles := make(map[string]bool)
	for _, item := range evidence {
		if item.Producer != types.EvidenceProducerExplorerEmitEvidence || item.Kind != types.EvidenceMechanism ||
			item.AnchorKind != types.AnchorDefinition || types.ClaimFormOf(item) != types.ClaimDefinitionFact ||
			!item.IsCitable() || item.LineStart <= 0 || types.RuntimeArtifactPathKind(item.Source) != "" ||
			!types.SourceScopeAllowsPathRole(principalScope, types.ClassifySourcePathRole(item.Source)) {
			continue
		}
		file, fi := mechanismSemanticDescentGraphFile(graph, item.Source)
		fileKey := strings.ToLower(canonicalRelationSourcePath(file))
		if fi == nil || fileKey == "" || seenFiles[fileKey] || !mechanismSemanticDescentASTFile(fi) {
			continue
		}
		sym := enclosingCallableSymbol(fi, item.LineStart)
		if !mechanismSemanticDescentCallable(sym) {
			continue
		}
		end := sym.EndLine
		if end < sym.Line {
			end = sym.Line
		}
		qualified := qualifiedEvidenceSymbolNameInFile(fi, sym)
		if item.LineStart < sym.Line || item.LineStart > end ||
			!mechanismSemanticDescentIdentityMatches(item.AnchorSymbol, sym.Name, qualified) {
			continue
		}
		seenFiles[fileKey] = true
		node := mechanismSemanticDescentNode{
			file: file, fi: fi, sym: sym, root: strings.TrimSpace(item.AnchorSymbol),
			selectionLine: item.LineStart,
		}
		// A selected definition proves only its own body regardless of whether
		// another principal fact exists elsewhere in the dispatch. Exact call
		// operations, not global principal presence, own child traversal.
		node.depth = mechanismSemanticDescentMaxDepth
		seeds = append(seeds, node)
		if len(seeds) >= mechanismSemanticDescentMaxDemands {
			break
		}
	}
	return seeds
}

func mechanismCompletionHasCurrentSourcePrincipalFact(
	ctx *types.BusContext,
	facts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	if rm == nil || rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	hasPrincipal := false
	for _, fact := range facts {
		if types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer {
			continue
		}
		hasPrincipal = true
		for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
			if origin == types.AnswerEvidenceOriginCurrentSource {
				return true
			}
		}
	}
	if !hasPrincipal {
		return false
	}
	principalScope := types.PrincipalSourceScope(rm.SourceScopeProfile)
	for _, item := range evidence {
		if item.Producer == types.EvidenceProducerExplorerEmitEvidence && item.IsCitable() &&
			strings.TrimSpace(item.Source) != "" && item.LineStart > 0 &&
			types.RuntimeArtifactPathKind(item.Source) == "" &&
			types.SourceScopeAllowsPathRole(principalScope, types.ClassifySourcePathRole(item.Source)) {
			return true
		}
	}
	return false
}

func mechanismSemanticDescentExecutableClaim(form types.ClaimForm) bool {
	switch form {
	case types.ClaimGuardCondition, types.ClaimReturnFact, types.ClaimAssignmentFact:
		return true
	default:
		return false
	}
}

func mechanismSemanticDescentDefinitionSeeds(
	ctx *types.BusContext,
	graph *repotypes.Graph,
	aggregateFacts []types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) []mechanismSemanticDescentNode {
	if graph == nil || len(aggregateFacts) == 0 || len(evidence) == 0 {
		return nil
	}
	seeds := make([]mechanismSemanticDescentNode, 0, 4)
	seen := make(map[string]bool)
	for _, fact := range aggregateFacts {
		if !mechanismDefinitionRosterCanSeed(ctx, fact) {
			continue
		}
		for idx, member := range fact.Members {
			file, fi, sym, ok := mechanismNarrativeMemberCallable(graph, member, fact.SupportRefs[idx])
			if !ok || !mechanismDefinitionEvidenceSelectsCallable(evidence, file, fi, sym, member) {
				continue
			}
			key := mechanismSemanticDescentSymbolKey(file, sym)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			seeds = append(seeds, mechanismSemanticDescentNode{
				file: file, fi: fi, sym: sym, root: strings.TrimSpace(member), selectionLine: sym.Line,
			})
		}
	}
	return seeds
}

func mechanismDefinitionRosterCanSeed(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 ||
		len(fact.Members) != len(fact.SupportRefs) || !aggregateFactCanDefineModelOwnedCompletionBoundary(fact) {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			return true
		}
	}
	return false
}

func mechanismDefinitionEvidenceSelectsCallable(
	evidence []types.EvidenceItem,
	file string,
	fi *repotypes.FileInfo,
	sym *repotypes.Symbol,
	member string,
) bool {
	if fi == nil || !mechanismSemanticDescentCallable(sym) {
		return false
	}
	end := sym.EndLine
	if end < sym.Line {
		end = sym.Line
	}
	qualified := qualifiedEvidenceSymbolNameInFile(fi, sym)
	for _, item := range evidence {
		if item.Producer != types.EvidenceProducerExplorerEmitEvidence ||
			item.Kind != types.EvidenceMechanism || types.ClaimFormOf(item) != types.ClaimDefinitionFact ||
			!item.IsCitable() || item.LineStart < sym.Line || item.LineStart > end ||
			!callChainSourcePathEquivalent(canonicalRelationSourcePath(item.Source), canonicalRelationSourcePath(file)) {
			continue
		}
		if mechanismSemanticDescentIdentityMatches(item.AnchorSymbol, sym.Name, qualified) &&
			mechanismSemanticDescentIdentityMatches(member, sym.Name, qualified) {
			return true
		}
	}
	return false
}

// mechanismSemanticDescentOperationLeafSeeds projects only exact,
// Explorer-selected current-source call operations onto their parser-resolved
// repository target. It is intentionally narrower than a call-graph walk:
// unrelated parser expansions and system-synthesised relation rows cannot
// create a completion obligation, and a mismatched/ambiguous endpoint fails
// open.
func mechanismSemanticDescentOperationLeafSeeds(
	graph *repotypes.Graph,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
) []mechanismSemanticDescentNode {
	if graph == nil || len(evidence) == 0 {
		return nil
	}
	operations := types.ExplorerAuthoredFlowOperationEvidenceForRequest(evidence, rm)
	seeds := make([]mechanismSemanticDescentNode, 0, len(operations))
	seen := make(map[string]bool)
	for _, item := range operations {
		if types.ClaimFormOf(item) != types.ClaimCallEdge || item.LineStart <= 0 {
			continue
		}
		_, fi := mechanismSemanticDescentGraphFile(graph, item.Source)
		if fi == nil || !mechanismSemanticDescentASTFile(fi) {
			continue
		}
		owner := enclosingCallableSymbol(fi, item.LineStart)
		if !mechanismSemanticDescentCallable(owner) {
			continue
		}
		for relIdx := range fi.Relations {
			rel := &fi.Relations[relIdx]
			if !mechanismSemanticDescentCallRelation(rel, item.LineStart) {
				continue
			}
			target := graph.ResolveCallTarget(fi, *rel)
			if !mechanismSemanticDescentCallable(target) || target == owner {
				continue
			}
			targetFile, targetFI := mechanismSemanticDescentGraphFile(graph, target.File)
			if targetFI == nil || !mechanismSemanticDescentASTFile(targetFI) {
				continue
			}
			qualified := qualifiedEvidenceSymbolNameInFile(targetFI, target)
			if !mechanismSemanticDescentIdentityMatches(item.AnchorSymbol, target.Name, qualified) &&
				!mechanismSemanticDescentIdentityMatches(item.Object, target.Name, qualified) {
				continue
			}
			key := mechanismSemanticDescentSymbolKey(targetFile, target)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			seeds = append(seeds, mechanismSemanticDescentNode{
				file: targetFile, fi: targetFI, sym: target, depth: 1,
				root:          strings.TrimSpace(item.Subject) + " -> " + strings.TrimSpace(target.Name),
				selectionLine: target.Line,
			})
		}
	}
	return seeds
}

func mechanismNarrativeMemberCallable(
	graph *repotypes.Graph,
	member, supportRef string,
) (string, *repotypes.FileInfo, *repotypes.Symbol, bool) {
	if graph == nil {
		return "", nil, nil, false
	}
	refLabel, loc, ok := types.ParseAnswerSupportRefMemberLocation(supportRef)
	if !ok || strings.TrimSpace(loc.File) == "" || loc.LineStart <= 0 {
		return "", nil, nil, false
	}
	file, fi := mechanismSemanticDescentGraphFile(graph, loc.File)
	if fi == nil || !mechanismSemanticDescentASTFile(fi) {
		return "", nil, nil, false
	}
	sym := enclosingCallableSymbol(fi, loc.LineStart)
	if !mechanismSemanticDescentCallable(sym) {
		return "", nil, nil, false
	}
	qualified := qualifiedEvidenceSymbolNameInFile(fi, sym)
	memberIdentity := callChainAggregateMemberIdentity(member)
	if !mechanismSemanticDescentIdentityMatches(memberIdentity, sym.Name, qualified) ||
		(strings.TrimSpace(refLabel) != "" && !mechanismSemanticDescentIdentityMatches(refLabel, sym.Name, qualified)) {
		return "", nil, nil, false
	}
	return file, fi, sym, true
}

func mechanismReturnCallLine(fi *repotypes.FileInfo, line int) bool {
	if fi == nil || line <= 0 {
		return false
	}
	hasReturn, hasCall := false, false
	for _, feature := range fi.LineFeatures[line] {
		switch feature {
		case repotypes.LineFeatureReturnStmt:
			hasReturn = true
		case repotypes.LineFeatureCallExpression:
			hasCall = true
		}
	}
	return hasReturn && hasCall
}

func mechanismSemanticDescentCallRelation(rel *repotypes.Relation, line int) bool {
	return rel != nil && rel.Kind == "call" && rel.Line == line &&
		(rel.Provenance == repotypes.ProvenanceTreeSitter || rel.Provenance == repotypes.ProvenanceCangjieParser)
}

func mechanismReturnCallChildren(
	graph *repotypes.Graph,
	fi *repotypes.FileInfo,
	owner *repotypes.Symbol,
	rel *repotypes.Relation,
) []mechanismSemanticDescentNode {
	var children []mechanismSemanticDescentNode
	add := func(sym *repotypes.Symbol) {
		if !mechanismSemanticDescentCallable(sym) || sym == owner {
			return
		}
		childFile, childFI := mechanismSemanticDescentGraphFile(graph, sym.File)
		if childFI == nil || !mechanismSemanticDescentASTFile(childFI) {
			return
		}
		for _, existing := range children {
			if mechanismSemanticDescentSymbolKey(existing.file, existing.sym) == mechanismSemanticDescentSymbolKey(childFile, sym) {
				return
			}
		}
		children = append(children, mechanismSemanticDescentNode{
			file: childFile, fi: childFI, sym: sym, selectionLine: sym.Line,
		})
	}
	if graph != nil && fi != nil && rel != nil {
		add(graph.ResolveCallTarget(fi, *rel))
	}
	return children
}

func mechanismSemanticDescentIdentityMatches(candidate, name, qualified string) bool {
	candidate = strings.Trim(strings.TrimSpace(candidate), "`'\"()&* ")
	if candidate == "" {
		return false
	}
	return types.AnswerCodeIdentitySurfacesCompatible(candidate, name) ||
		types.AnswerCodeIdentitySurfacesCompatible(name, candidate) ||
		types.AnswerCodeIdentitySurfacesCompatible(candidate, qualified) ||
		types.AnswerCodeIdentitySurfacesCompatible(qualified, candidate)
}

func mechanismSemanticDescentCallable(sym *repotypes.Symbol) bool {
	if sym == nil || strings.TrimSpace(sym.Name) == "" || sym.Line <= 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(sym.Kind)) {
	case "function", "method", "ctor", "operator", "foreign-func", "builder",
		"suspend-function", "extension-function":
		return true
	default:
		return false
	}
}

func mechanismSemanticDescentASTFile(fi *repotypes.FileInfo) bool {
	// A precise parser-owned symbol is sufficient authority to demand that
	// symbol's own body. LineFeatures are needed only when following a body
	// line into another callable; mechanismReturnCallLine/mechanismCallLine
	// already fail closed when that optional index is absent. Requiring a
	// non-empty feature map here made cached graphs and legitimate leaf bodies
	// silently suppress the initial semantic read even though the definition
	// identity and extent were exact.
	return fi != nil && fi.ParseTier <= 2 && len(fi.Symbols) > 0
}

// mechanismSemanticDescentReadRanges keeps the thin-wrapper guard exact
// without turning a large orchestration function into a whole-body reading
// obligation. Small callables retain the original complete-body contract. For
// a large callable, the closure owns only the declaration/selected evidence
// window plus every parser-owned direct return-call site when that set itself
// is bounded. A callable with many return-call sites is structurally not a
// thin wrapper, so it does not create recursive semantic-descent work.
func mechanismSemanticDescentReadRanges(node mechanismSemanticDescentNode) []types.LineRange {
	if node.sym == nil || node.fi == nil || node.sym.Line <= 0 {
		return nil
	}
	end := node.sym.EndLine
	if end < node.sym.Line {
		end = node.sym.Line
	}
	if end-node.sym.Line+1 <= mechanismSemanticDescentFullBodyMaxLines {
		return []types.LineRange{{Start: node.sym.Line, End: end}}
	}

	ranges := []types.LineRange{{
		Start: node.sym.Line,
		End:   min(end, node.sym.Line+mechanismSemanticDescentDeclarationLookahead),
	}}
	selectionLine := node.selectionLine
	if selectionLine < node.sym.Line || selectionLine > end {
		selectionLine = node.sym.Line
	}
	ranges = append(ranges, mechanismSemanticDescentContextRange(selectionLine, node.sym.Line, end))
	if node.depth < mechanismSemanticDescentMaxDepth {
		for _, line := range mechanismSemanticDescentReturnCallLines(node) {
			ranges = append(ranges, mechanismSemanticDescentContextRange(line, node.sym.Line, end))
		}
	}
	return mechanismSemanticDescentMergeRanges(ranges)
}

func mechanismSemanticDescentContextRange(line, start, end int) types.LineRange {
	if line < start {
		line = start
	}
	if line > end {
		line = end
	}
	return types.LineRange{
		Start: max(start, line-mechanismSemanticDescentCallContextLines),
		End:   min(end, line+mechanismSemanticDescentCallContextLines),
	}
}

// mechanismSemanticDescentReturnCallLines returns the complete bounded set of
// parser-owned return-call rows for a callable. It deliberately returns no
// rows, rather than an arbitrary first-N subset, when the callable has too many
// such sites to qualify as a thin wrapper.
func mechanismSemanticDescentReturnCallLines(node mechanismSemanticDescentNode) []int {
	if node.sym == nil || node.fi == nil || node.sym.Line <= 0 {
		return nil
	}
	end := node.sym.EndLine
	if end < node.sym.Line {
		end = node.sym.Line
	}
	seen := make(map[int]bool)
	lines := make([]int, 0, mechanismSemanticDescentMaxReturnCallSites)
	for idx := range node.fi.Relations {
		rel := &node.fi.Relations[idx]
		if rel.Line < node.sym.Line || rel.Line > end || seen[rel.Line] ||
			!mechanismSemanticDescentCallRelation(rel, rel.Line) ||
			!mechanismReturnCallLine(node.fi, rel.Line) {
			continue
		}
		seen[rel.Line] = true
		lines = append(lines, rel.Line)
		if len(lines) > mechanismSemanticDescentMaxReturnCallSites {
			return nil
		}
	}
	sort.Ints(lines)
	return lines
}

func mechanismSemanticDescentMergeRanges(ranges []types.LineRange) []types.LineRange {
	valid := make([]types.LineRange, 0, len(ranges))
	for _, r := range ranges {
		if r.Start > 0 && r.End >= r.Start {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].Start == valid[j].Start {
			return valid[i].End < valid[j].End
		}
		return valid[i].Start < valid[j].Start
	})
	out := []types.LineRange{valid[0]}
	for _, r := range valid[1:] {
		last := &out[len(out)-1]
		if r.Start <= last.End+1 {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func mechanismSemanticDescentRangesFullyRead(closure *types.EvidenceClosure, file string, ranges []types.LineRange) bool {
	if len(ranges) == 0 {
		return false
	}
	for _, r := range ranges {
		if !callChainDemandRangeFullyRead(closure, file, r) {
			return false
		}
	}
	return true
}

func mechanismSemanticDescentGraphFile(graph *repotypes.Graph, source string) (string, *repotypes.FileInfo) {
	if graph == nil {
		return "", nil
	}
	want := canonicalRelationSourcePath(source)
	for file, fi := range graph.FileIndex {
		if fi == nil {
			continue
		}
		candidate := canonicalRelationSourcePath(fi.RelPath)
		if candidate == "" {
			candidate = canonicalRelationSourcePath(file)
		}
		if callChainSourcePathEquivalent(candidate, want) {
			return candidate, fi
		}
	}
	return "", nil
}

func mechanismSemanticDescentSymbolKey(file string, sym *repotypes.Symbol) string {
	if sym == nil || strings.TrimSpace(file) == "" || sym.Line <= 0 {
		return ""
	}
	return strings.ToLower(canonicalRelationSourcePath(file)) + ":" + fmt.Sprintf("%d:%s", sym.Line, strings.TrimSpace(sym.Name))
}

func mechanismSemanticDescentAddPendingRead(
	closure *types.EvidenceClosure,
	file string,
	sym *repotypes.Symbol,
	root string,
	bodyRanges []types.LineRange,
) {
	if closure == nil || sym == nil || file == "" || len(bodyRanges) == 0 {
		return
	}
	closure.AddPendingRead(types.PendingRead{
		File: file,
		Rationale: fmt.Sprintf(
			"the explained entry %q returns or delegates through local callable %q; read that exact implementation body before closing so the answer can distinguish wrapper behavior from the delegated behavior",
			strings.TrimSpace(root), qualifiedEvidenceSymbolName(sym),
		),
		Origin:                  fmt.Sprintf("pre_complete.mechanism_semantic_descent.%d", sym.Line),
		LineRanges:              bodyRanges,
		MaterializationRequired: true,
		Stage:                   string(types.StageExplore),
	})
}

func mechanismSemanticDescentAddSelectedBodyPendingRead(
	closure *types.EvidenceClosure,
	file string,
	sym *repotypes.Symbol,
	root string,
	bodyRanges []types.LineRange,
) {
	if closure == nil || sym == nil || file == "" || len(bodyRanges) == 0 {
		return
	}
	closure.AddPendingRead(types.PendingRead{
		File: file,
		Rationale: fmt.Sprintf(
			"the typed mechanism selection %q resolves inside local callable %q; read only the listed parser-owned declaration/selection windows before closing, while sibling calls require their own typed operation evidence",
			strings.TrimSpace(root), qualifiedEvidenceSymbolName(sym),
		),
		Origin:                  fmt.Sprintf("pre_complete.mechanism_semantic_descent.%d", sym.Line),
		LineRanges:              bodyRanges,
		MaterializationRequired: true,
		Stage:                   string(types.StageExplore),
	})
}
