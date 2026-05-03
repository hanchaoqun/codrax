package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scanMechanismEvidence (T2.2) is the deterministic evidence producer
// for ERM mechanism requirements. T2.1 added the mechanism Kind to
// extractEvidenceRequirements + checkRequirementSatisfaction, but
// without a producer the satisfaction check could only fire on
// dataflow-derived EvidenceRelationship items or LLM-tagged
// [MECHANISM] notes — both unreliable for "how does X work" questions
// where the canonical answer is the *list of branches* inside a
// function body.
//
// This scanner closes that gap. For each ERM mechanism requirement,
// it locates the entity's primary function in the repo_map graph,
// reads the full body, and reuses extractDecisionBlocks (the
// section-comment block detector added in commit 828939c) to produce
// one EvidenceMechanism item per logical block. The result flows into
// the same evidence pipeline as Concrete Values and dataflow output.
//
// Why this is not over-fitted:
//
//   - Block detection uses cross-language section-comment heuristics
//     that already handle Go/Python/Java/Rust/SQL — same code path as
//     extractDecisionBlocks, no new shape claims.
//   - Entity selection comes from ERM (which derives from the user's
//     question), not from a hardcoded symbol list.
//   - The output is structured per block, not per file or per entity,
//     so questions with N strategies produce N evidence items —
//     proportional to actual structure, not to question text.
//
// Budgets are tight: at most 3 entities per call, at most 1 function
// per entity, at most 8 blocks per function. Mechanism scan is meant
// to be a precision tool, not a coverage one.
const (
	mechanismMaxEntities      = 3
	mechanismMaxFuncsPerEnt   = 1
	mechanismMaxBlocksPerFunc = 8
	mechanismMinBodyLines     = 20 // skip trivial functions
)

// scanMechanismEvidence walks the ERM mechanism requirements, locates
// the candidate functions in the graph, and produces structured
// EvidenceMechanism items. Returns nil when there is nothing to do
// (no mechanism reqs, no graph, or nothing matched).
//
// repoRoot is needed for absolute file path resolution because the
// graph stores forward-slash relative paths but file I/O needs the
// platform-native absolute form.
func scanMechanismEvidence(reqs []EvidenceRequirement, graph *repomap.Graph, repoRoot string) []types.EvidenceItem {
	if graph == nil || len(reqs) == 0 {
		return nil
	}

	// Filter to mechanism reqs only — every other Kind has its own
	// dedicated producer (Concrete Values for return_value/registration,
	// dataflow lowering for call_chain/conditional/config_mapping).
	var mechReqs []EvidenceRequirement
	for _, r := range reqs {
		if r.Kind == types.ReqMechanism {
			mechReqs = append(mechReqs, r)
		}
	}
	if len(mechReqs) == 0 {
		return nil
	}

	// Collect distinct entities across all mechanism reqs.
	seenEntity := make(map[string]bool)
	var entities []string
	for _, r := range mechReqs {
		for _, e := range r.Entities {
			low := strings.ToLower(e)
			if seenEntity[low] {
				continue
			}
			seenEntity[low] = true
			entities = append(entities, e)
			if len(entities) >= mechanismMaxEntities {
				break
			}
		}
		if len(entities) >= mechanismMaxEntities {
			break
		}
	}
	if len(entities) == 0 {
		return nil
	}

	// File-content cache so multiple symbols in the same file pay one
	// I/O cost. Mirror buildConcreteValuesSection's loader.
	fileLines := make(map[string][]string)
	loadFileLines := func(absPath string) []string {
		if lines, ok := fileLines[absPath]; ok {
			return lines
		}
		f, err := os.Open(absPath)
		if err != nil {
			fileLines[absPath] = nil
			return nil
		}
		defer f.Close()
		var lines []string
		scanner := bufio.NewScanner(f)
		// Allow large lines — generated code or single-line minified
		// files would otherwise blow up the default 64KB cap.
		scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		fileLines[absPath] = lines
		return lines
	}

	var out []types.EvidenceItem

	for _, ent := range entities {
		funcs := selectMechanismFunctions(graph, ent)
		if len(funcs) == 0 {
			continue
		}
		for _, sym := range funcs {
			absPath := filepath.Join(repoRoot, sym.File)
			lines := loadFileLines(absPath)
			if len(lines) == 0 {
				continue
			}
			if sym.EndLine == 0 || sym.EndLine-sym.Line < mechanismMinBodyLines {
				continue
			}
			// Phase 6 stage 18 (2026-05-03) — extractDecisionBlocks
			// now consumes the typed AST features map populated by
			// repomap's tree-sitter extractors. Empty/nil features
			// (Tier 3+ regex-only fallback) ⇒ no decision-block
			// surfacing for this symbol.
			fi := graph.FileIndex[sym.File]
			var lineFeatures map[int][]repotypes.LineFeature
			if fi != nil {
				lineFeatures = fi.LineFeatures
			}
			blocks := extractDecisionBlocks(lines, sym.Line, sym.EndLine, lineFeatures)
			if len(blocks) == 0 {
				continue
			}
			if len(blocks) > mechanismMaxBlocksPerFunc {
				blocks = blocks[:mechanismMaxBlocksPerFunc]
			}
			subject := sym.Name
			if sym.Receiver != "" {
				subject = sym.Receiver + "." + sym.Name
			} else if sym.Parent != "" {
				subject = sym.Parent + "." + sym.Name
			}
			source := fmt.Sprintf("mechanism_scan:%s", sym.File)
			for _, b := range blocks {
				out = append(out, types.EvidenceItem{
					Kind:      types.EvidenceMechanism,
					Predicate: "step",
					Subject:   subject,
					Object:    b.label,
					Summary:   fmt.Sprintf("`%s` step at line %d: %s", subject, b.startLine, b.label),
					Source:    source,
					LineStart: b.startLine,
					LineEnd:   b.endLine,
					Confidence: 0.85,
					// Deterministic mechanism scan emits line-shaped
					// evidence (step blocks at known line ranges).
					Scope: types.ScopeLineRange,
				})
			}
		}
	}

	if len(out) > 0 {
		logging.Debug("[explorer] mechanism_scan: %d evidence items from %d entities", len(out), len(entities))
	}
	return out
}

// selectMechanismFunctions resolves an ERM entity name to the function/
// method symbols whose mechanism is most likely the answer. Two
// resolution paths:
//
//  1. Direct: entity matches a function/method symbol name.
//  2. Type-method: entity matches a type/struct/class/interface, and
//     the type has methods. We pick the methods with the longest body
//     (most likely to contain branching).
//
// Returns at most mechanismMaxFuncsPerEnt symbols, ranked by body
// length (longest first — short functions are usually delegating
// helpers, not the answer).
func selectMechanismFunctions(graph *repomap.Graph, entity string) []*repomap.Symbol {
	if graph == nil || entity == "" {
		return nil
	}
	entLower := strings.ToLower(entity)

	type scored struct {
		sym  *repomap.Symbol
		size int
	}
	var candidates []scored

	// Path 1: direct symbol match (function/method whose name contains
	// the entity).
	for symName, defs := range graph.SymbolDefs {
		if !strings.Contains(strings.ToLower(symName), entLower) {
			continue
		}
		for _, sym := range defs {
			if sym.Kind != "function" && sym.Kind != "method" {
				continue
			}
			if sym.EndLine == 0 {
				continue
			}
			candidates = append(candidates, scored{sym: sym, size: sym.EndLine - sym.Line})
		}
	}

	// Path 2: type-method resolution. If the entity is a type/struct/
	// class, find methods whose Receiver/Parent matches.
	for _, fi := range graph.Files {
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.Kind != "method" && sym.Kind != "function" {
				continue
			}
			if sym.EndLine == 0 {
				continue
			}
			match := false
			if sym.Receiver != "" && strings.Contains(strings.ToLower(sym.Receiver), entLower) {
				match = true
			} else if sym.Parent != "" && strings.Contains(strings.ToLower(sym.Parent), entLower) {
				match = true
			}
			if match {
				candidates = append(candidates, scored{sym: sym, size: sym.EndLine - sym.Line})
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by size descending — the largest body is usually where the
	// branching logic lives.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].size > candidates[j-1].size; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	// Dedup by file:line so the two resolution paths above don't
	// double-count the same symbol.
	seen := make(map[string]bool)
	var out []*repomap.Symbol
	for _, c := range candidates {
		key := fmt.Sprintf("%s:%d", c.sym.File, c.sym.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c.sym)
		if len(out) >= mechanismMaxFuncsPerEnt {
			break
		}
	}
	return out
}
