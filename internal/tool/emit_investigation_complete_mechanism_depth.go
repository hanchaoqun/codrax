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
	file  string
	fi    *repotypes.FileInfo
	sym   *repotypes.Symbol
	depth int
	root  string
}

// raiseMechanismSemanticDescentPendingReads closes a narrow but important
// mechanism-explanation gap: a model-owned principal member can cite and read
// a thin wrapper, then close before reading the local callable that actually
// implements the returned behavior.
//
// The descent is deliberately bounded and source-owned:
//   - the request must already be a typed current-source mechanism/explanation;
//   - the model must have emitted an index-aligned principal member_set with a
//     non-empty responsibility note and grounded support ref for the member;
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
	for _, fact := range aggregateFacts {
		if !aggregateNarrativeMemberSetRequiresAlignedCurrentSourceSupport(ctx, fact) ||
			len(fact.Members) != len(fact.MemberNotes) || len(fact.Members) != len(fact.SupportRefs) {
			continue
		}
		for idx, member := range fact.Members {
			if strings.TrimSpace(fact.MemberNotes[idx]) == "" {
				continue
			}
			file, fi, sym, ok := mechanismNarrativeMemberCallable(graph, member, fact.SupportRefs[idx])
			if !ok {
				continue
			}
			key := mechanismSemanticDescentSymbolKey(file, sym)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			frontier = append(frontier, mechanismSemanticDescentNode{file: file, fi: fi, sym: sym, root: strings.TrimSpace(member)})
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
			if !mechanismReturnCallLine(node.fi, line) || !closure.HasReadLine(node.file, line) {
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
