package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	mechanismSemanticDescentMaxDepth   = 2
	mechanismSemanticDescentMaxDemands = 4
)

type mechanismSemanticDescentNode struct {
	file           string
	fi             *repotypes.FileInfo
	sym            *repotypes.Symbol
	depth          int
	root           string
	followAllCalls bool
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
//   - every traversed edge must be on a parser-owned return+call line;
//   - a direct callee must resolve through the repository graph; a callable
//     argument must resolve to exactly one repository callable and be proved by
//     the existing line-local callback parser;
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
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil || len(graph.FileIndex) == 0 {
		return 0
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
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
			addSeed(mechanismSemanticDescentNode{file: file, fi: fi, sym: sym, root: strings.TrimSpace(member)})
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
	// an enum/state roster rather than a callable roster. Unlike the older
	// wrapper lane, this lane may follow any parser-owned local call in the
	// selected callable; the shared depth/demand cap keeps that closure bounded.
	// Definition/text-reference evidence cannot enter this lane.
	for _, node := range mechanismSemanticDescentExecutionSeeds(ctx, graph, aggregateFacts, evidence) {
		addSeed(node)
	}
	// B879e: a callable definition explicitly classified by the Explorer as
	// mechanism evidence is also a precise model selection. This covers runs
	// whose evidence shape contains no executable guard/return/assignment yet.
	// Keep only the first exact callable per source file so role-description
	// rosters cannot crowd out the bounded closure. System auto-pairs and plain
	// direct definitions are excluded by producer/kind below.
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
		end := node.sym.EndLine
		if end < node.sym.Line {
			end = node.sym.Line
		}
		body := types.LineRange{Start: node.sym.Line, End: end}
		if !callChainDemandRangeFullyRead(closure, node.file, body) {
			mechanismSemanticDescentAddPendingRead(closure, node.file, node.sym, node.root, body)
			demands++
			continue
		}
		if node.depth >= mechanismSemanticDescentMaxDepth {
			continue
		}

		for line := node.sym.Line; line <= end && demands < mechanismSemanticDescentMaxDemands; line++ {
			if !closure.HasReadLine(node.file, line) ||
				(!node.followAllCalls && !mechanismReturnCallLine(node.fi, line)) ||
				(node.followAllCalls && !mechanismCallLine(node.fi, line)) {
				continue
			}
			for relIdx := range node.fi.Relations {
				rel := &node.fi.Relations[relIdx]
				if !mechanismSemanticDescentCallRelation(rel, line) {
					continue
				}
				children := mechanismReturnCallChildren(graph, gc, node.file, node.fi, node.sym, rel)
				for _, child := range children {
					key := mechanismSemanticDescentSymbolKey(child.file, child.sym)
					if key == "" || seen[key] {
						continue
					}
					seen[key] = true
					child.depth = node.depth + 1
					child.root = node.root
					child.followAllCalls = node.followAllCalls
					childEnd := child.sym.EndLine
					if childEnd < child.sym.Line {
						childEnd = child.sym.Line
					}
					childBody := types.LineRange{Start: child.sym.Line, End: childEnd}
					if !callChainDemandRangeFullyRead(closure, child.file, childBody) {
						mechanismSemanticDescentAddPendingRead(closure, child.file, child.sym, node.root, childBody)
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
			file: file, fi: fi, sym: sym, root: root, followAllCalls: true,
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
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil || len(evidence) == 0 ||
		!mechanismCompletionHasCurrentSourcePrincipalFact(ctx, aggregateFacts, evidence) {
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
		seeds = append(seeds, mechanismSemanticDescentNode{
			file: file, fi: fi, sym: sym, root: strings.TrimSpace(item.AnchorSymbol), followAllCalls: true,
		})
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
				file: file, fi: fi, sym: sym, root: strings.TrimSpace(member),
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
				root: strings.TrimSpace(item.Subject) + " -> " + strings.TrimSpace(target.Name),
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

func mechanismCallLine(fi *repotypes.FileInfo, line int) bool {
	if fi == nil || line <= 0 {
		return false
	}
	for _, feature := range fi.LineFeatures[line] {
		if feature == repotypes.LineFeatureCallExpression {
			return true
		}
	}
	return false
}

func mechanismSemanticDescentCallRelation(rel *repotypes.Relation, line int) bool {
	return rel != nil && rel.Kind == "call" && rel.Line == line &&
		(rel.Provenance == repotypes.ProvenanceTreeSitter || rel.Provenance == repotypes.ProvenanceCangjieParser)
}

func mechanismReturnCallChildren(
	graph *repotypes.Graph,
	gc *ground.Context,
	file string,
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
		children = append(children, mechanismSemanticDescentNode{file: childFile, fi: childFI, sym: sym})
	}
	if graph != nil && fi != nil && rel != nil {
		add(graph.ResolveCallTarget(fi, *rel))
	}
	receiver := callRelationTargetName(graph, fi, rel)
	for _, flow := range ground.DetectArgumentFlowsAtLine(gc, file, rel.Line, receiver) {
		callable, ok := mechanismUniqueCallableArgumentSymbol(graph, flow.Argument)
		if !ok {
			continue
		}
		detectedReceiver, detectedCallable, detected := ground.DetectCallbackHandoffAtLine(gc, file, rel.Line, flow.Argument)
		if !detected || !types.AnswerCodeIdentitySurfacesCompatible(receiver, detectedReceiver) ||
			!types.AnswerCodeIdentitySurfacesCompatible(flow.Argument, detectedCallable) {
			continue
		}
		add(callable)
	}
	return children
}

func mechanismUniqueCallableArgumentSymbol(graph *repotypes.Graph, argument string) (*repotypes.Symbol, bool) {
	if graph == nil {
		return nil, false
	}
	argument = strings.Trim(strings.TrimSpace(argument), "()&* ")
	tail := types.NormalizedSurfaceSymbolTail(argument)
	if argument == "" || tail == "" {
		return nil, false
	}
	var match *repotypes.Symbol
	for name, defs := range graph.SymbolDefs {
		if !strings.EqualFold(types.NormalizedSurfaceSymbolTail(name), tail) {
			continue
		}
		for _, sym := range defs {
			if !mechanismSemanticDescentCallable(sym) {
				continue
			}
			_, fi := mechanismSemanticDescentGraphFile(graph, sym.File)
			if fi == nil || !mechanismSemanticDescentASTFile(fi) {
				continue
			}
			qualified := qualifiedEvidenceSymbolNameInFile(fi, sym)
			if !mechanismSemanticDescentIdentityMatches(argument, sym.Name, qualified) {
				continue
			}
			if match != nil && mechanismSemanticDescentSymbolKey(match.File, match) != mechanismSemanticDescentSymbolKey(sym.File, sym) {
				return nil, false
			}
			match = sym
		}
	}
	return match, match != nil
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
	return fi != nil && fi.ParseTier <= 2 && len(fi.LineFeatures) > 0
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
	body types.LineRange,
) {
	if closure == nil || sym == nil || file == "" || body.Start <= 0 || body.End < body.Start {
		return
	}
	closure.AddPendingRead(types.PendingRead{
		File: file,
		Rationale: fmt.Sprintf(
			"the explained entry %q returns or delegates through local callable %q; read that exact implementation body before closing so the answer can distinguish wrapper behavior from the delegated behavior",
			strings.TrimSpace(root), qualifiedEvidenceSymbolName(sym),
		),
		Origin:     fmt.Sprintf("pre_complete.mechanism_semantic_descent.%d", sym.Line),
		LineRanges: []types.LineRange{body},
		Stage:      string(types.StageExplore),
	})
}
