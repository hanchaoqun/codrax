// Package binder attaches hypotheses to TaskGraph nodes by
// relevance. It replaces the pre-refactor round-robin hdp.Bind
// which assigned hypotheses via modulo index.
//
// Relevance metric (see relevance() below):
//
//	α · jaccard(hypothesis term IDs, node search hints)
//	+ β · surface mentions of the hypothesis statement in node objective
//	+ γ · kind-family affinity between the hypothesis falsification
//	      kind and the node type
//
// α=1.0, β=0.5, γ=0.3. Each binding-eligible node picks the top-K
// hypotheses by score. A hypothesis that scores zero for every node
// gets a fallback binding to the first evidence node so gate's
// hypothesis_coverage rule still passes.
package binder

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Options tunes the binder. Zero values yield sensible defaults: K=2
// bindings per node, MinScore=0.01 so anything above trivial wins.
type Options struct {
	K        int
	MinScore float64
}

// BindByRelevance walks the task graph and attaches hypothesis IDs
// to every binding-eligible node (non-probe). Returns an error
// when a binding-eligible node ends up with zero hypotheses — gate
// would reject such an IR anyway, so failing loudly here produces a
// clearer message.
func BindByRelevance(tg *types.TaskGraph, hs []types.Hypothesis, opts Options) error {
	if tg == nil {
		return errors.New("binder: nil TaskGraph")
	}
	if len(hs) == 0 {
		return errors.New("binder: empty hypothesis set")
	}
	k := opts.K
	if k <= 0 {
		k = 2
	}
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = 0.01
	}

	bound := make(map[string]bool, len(hs))
	for nIdx := range tg.Nodes {
		n := &tg.Nodes[nIdx]
		if !requiresHypothesis(n.Type) {
			continue
		}
		if len(n.Hypotheses) > 0 {
			for _, id := range n.Hypotheses {
				bound[id] = true
			}
			continue
		}
		type scored struct {
			idx   int
			score float64
		}
		var list []scored
		for i, h := range hs {
			s := relevance(h, *n)
			if s < minScore {
				continue
			}
			list = append(list, scored{idx: i, score: s})
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].score != list[j].score {
				return list[i].score > list[j].score
			}
			return list[i].idx < list[j].idx
		})
		take := k
		if take > len(list) {
			take = len(list)
		}
		ids := make([]string, 0, take)
		for i := 0; i < take; i++ {
			ids = append(ids, hs[list[i].idx].ID)
			bound[hs[list[i].idx].ID] = true
		}
		n.Hypotheses = ids
	}

	// Fallback: any hypothesis with zero relevance on every node
	// gets pinned to the first evidence node. Same invariant gate
	// enforces; tightening it to "no orphans" means every
	// hypothesis is reachable from at least one runtime verdict.
	var orphans []string
	for _, h := range hs {
		if !bound[h.ID] {
			orphans = append(orphans, h.ID)
		}
	}
	if len(orphans) > 0 {
		// Find first evidence node; if none, fall back to first
		// binding-eligible node.
		var target *types.TaskNode
		for i := range tg.Nodes {
			if tg.Nodes[i].Type == types.NodeEvidence {
				target = &tg.Nodes[i]
				break
			}
		}
		if target == nil {
			for i := range tg.Nodes {
				if requiresHypothesis(tg.Nodes[i].Type) {
					target = &tg.Nodes[i]
					break
				}
			}
		}
		if target == nil {
			return errors.New("binder: no evidence node to bind orphans to")
		}
		target.Hypotheses = append(target.Hypotheses, orphans...)
	}

	// Final sanity: every binding-eligible node must now have at
	// least one hypothesis.
	var unbound []string
	for _, n := range tg.Nodes {
		if !requiresHypothesis(n.Type) {
			continue
		}
		if len(n.Hypotheses) == 0 {
			unbound = append(unbound, n.ID)
		}
	}
	if len(unbound) > 0 {
		return fmt.Errorf("binder: %d node(s) left with zero hypotheses: %v", len(unbound), unbound)
	}
	return nil
}

func requiresHypothesis(t types.TaskNodeType) bool {
	switch t {
	case types.NodeEvidence, types.NodeValidate, types.NodeReconcile, types.NodeFinalize:
		return true
	}
	return false
}

func relevance(h types.Hypothesis, n types.TaskNode) float64 {
	termIDs := termTokens(h.Statement)
	hintIDs := append(append([]string(nil), n.SearchHints.KeywordIDs...), n.SearchHints.EntityIDs...)
	j := jaccard(termIDs, hintIDs)

	surfaces := 0.0
	stmt := strings.ToLower(h.Statement)
	for _, tok := range strings.Fields(strings.ToLower(n.Objective)) {
		if len(tok) > 3 && strings.Contains(stmt, tok) {
			surfaces++
		}
	}
	surfaces /= 5.0 // saturate quickly
	if surfaces > 1 {
		surfaces = 1
	}

	kindFit := kindAffinity(h.FalsificationCondition.Kind, n.Type)

	return 1.0*j + 0.5*surfaces + 0.3*kindFit
}

// termTokens extracts space-separated lowercase tokens ≥4 chars
// from a hypothesis statement for Jaccard comparison.
func termTokens(s string) []string {
	var out []string
	for _, tok := range strings.Fields(strings.ToLower(s)) {
		clean := strings.Trim(tok, ".,;:()[]{}\"'`")
		if len(clean) >= 4 {
			out = append(out, clean)
		}
	}
	return out
}

func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	aset := make(map[string]bool, len(a))
	for _, x := range a {
		aset[strings.ToLower(x)] = true
	}
	inter := 0
	bseen := make(map[string]bool, len(b))
	for _, x := range b {
		lx := strings.ToLower(x)
		if bseen[lx] {
			continue
		}
		bseen[lx] = true
		if aset[lx] {
			inter++
		}
	}
	union := len(aset)
	for k := range bseen {
		if !aset[k] {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// kindAffinity maps a hypothesis falsification kind onto the node
// type where it is most productively evaluated. Returns 1.0 for
// the natural pairing, 0.3 for an adequate pairing, 0 otherwise.
func kindAffinity(kind string, nodeType types.TaskNodeType) float64 {
	fam := kindFamily(kind)
	switch nodeType {
	case types.NodeEvidence:
		if fam == famPresence || fam == famAbsence {
			return 1.0
		}
		return 0.3
	case types.NodeValidate:
		if fam == famInvariant || fam == famFlow || fam == famResolution {
			return 1.0
		}
		return 0.3
	case types.NodeReconcile:
		if fam == famCardinality || fam == famAmbiguity {
			return 1.0
		}
		return 0.3
	}
	return 0.1
}

const (
	famPresence    = "presence"
	famAbsence     = "absence"
	famInvariant   = "invariant"
	famFlow        = "flow"
	famResolution  = "resolution"
	famCardinality = "cardinality"
	famAmbiguity   = "ambiguity"
	famOther       = "other"
)

func kindFamily(kind string) string {
	switch kind {
	case types.CritSymbolPresent:
		return famPresence
	case types.CritNoCallSites, types.CritNoRelevantEvidence:
		return famAbsence
	case types.CritInvariantBroken:
		return famInvariant
	case types.CritUntrustedReachesSink:
		return famFlow
	case types.CritMultipleResolutionChains:
		return famResolution
	case types.CritAnswerSetUnbounded, types.CritAnswerSetBounded:
		return famCardinality
	case types.CritUserClauseUnresolved:
		return famAmbiguity
	}
	return famOther
}
