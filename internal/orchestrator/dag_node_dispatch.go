package orchestrator

import (
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// E' (2026-05-17 architecture §1.5/§1.6) — DAG node-level dispatch.
//
// Forensic anchor: prior to this change, when the analyzer's
// expandEvidenceNodes (compiler/templates.go) produced N independent
// evidence sibling nodes (`{prefix}_t0` / `_t1` / `_tN`, one per
// SubTopic), the scheduler's readyExplorerWindowContext correctly
// returned all N nodes in a single slice — but the explore dispatch
// loop then COALESCED the entire window into one explorer LLM
// dispatch (renderWindowHintContext emits "Cover every node objective
// below in this dispatch:\n\n" listing all N objectives). One explorer
// instance had to serially chew through every sub-topic, producing
// the 5-minute Ctrl+C scenario in the 08:14 forensic run.
//
// Architecture red line §1.5: the DAG produced by the analyzer is
// typed authoritative data. The scheduler is code. Code must respect
// what data says — including the *freedom* (no edge between siblings
// = no ordering constraint = independent dispatch). Coalescing
// independent sibling nodes into one dispatch is the scheduler
// IGNORING that typed freedom. This file restores the alignment.
//
// Design (intentionally minimal):
//
//   - When the ready window contains ≥2 NodeEvidence siblings that
//     share a common ID prefix (the `{prefix}_t{i}` pattern emitted
//     by expandEvidenceNodes), trim the window down to its FIRST
//     element. The outer scheduler loop will pick up the remaining
//     siblings on subsequent iterations: each iteration re-runs
//     readyExplorerWindowContext, which filters out nodes whose
//     status is already nodeRunning / nodeDone, so the next loop
//     naturally sees only the un-dispatched siblings.
//
//   - This produces N sequential dispatches, each focused on ONE
//     sub-topic. Per-node EventTaskNodeStart / EventTaskNodeEnd
//     already fire correctly (orchestrator.go:4064-4066, 4172-4174).
//     The /dag UI shows N rows, each transitioning
//     pending → running → done independently.
//
//   - Multi-topic step budget is already scaled at
//     orchestrator.go:3833 (`nSub * agentCfg.SubTopicPipelineStepsExtra`)
//     so N sequential dispatches fit the budget the analyzer already
//     reserved for multi-topic questions.
//
//   - Phase 2B builds on this foundation with splitExploreWindowForDispatch:
//     the same typed sibling detection now produces declaration-ordered
//     focused windows that the scheduler may run concurrently under
//     pipeline_max_parallelism. The serial trim helper remains the exact
//     fallback when the cap is 1.
//
// Single-sub_topic / non-evidence-window paths are byte-identical to
// the prior behavior: the helper returns the same window slice with
// no mutation.

// shouldDispatchExploreNodesIndividually reports whether the ready
// explore window represents multiple INDEPENDENT evidence sibling
// nodes that should each receive a focused explorer dispatch instead
// of being merged into one explorer LLM call.
//
// Returns true iff the window contains AT LEAST TWO types.NodeEvidence
// entries. Other node kinds in the same window (probe, validate,
// reconcile) are allowed — they are handled by the companion trim
// helper, which strips the sibling evidence nodes but preserves
// non-evidence companions so the explorer dispatch still receives
// probe-scope + validate-context.
//
// Real-world DAG shape (from internal/analysis/compiler/templates.go
// expandEvidenceNodes when len(SubTopics) > 1): one probe node, N
// evidence sibling nodes (`{prefix}_t0` / `_t1` / `_tN`), one
// validate node. readyExplorerWindowContext currently has no
// EntryConditions to gate them apart, so all 1+N+1 nodes appear
// simultaneously on the first ready window. Before E', the explorer
// got a single dispatch covering all of them. After E', this
// predicate fires on (N≥2 evidence) and the trim helper retains
// probe + first evidence + validate; the outer scheduler loop
// dispatches the remaining (N-1) evidence siblings one by one on
// subsequent iterations.
//
// Sibling-edge check is implicit: expandEvidenceNodes does NOT emit
// hard/soft edges between sibling evidence nodes (templates.go builds
// edges from each evidence node to validate, never between
// siblings), so any window with ≥2 evidence siblings has no
// inter-sibling edges by construction. If a future analyzer template
// adds sibling-sibling edges, this predicate will need an explicit
// edge scan; today's expansion makes that scan vacuous.
func shouldDispatchExploreNodesIndividually(window []*types.TaskNode) bool {
	if len(window) < 2 {
		return false
	}
	evidenceCount := 0
	for _, n := range window {
		if n == nil {
			continue
		}
		if n.Type == types.NodeEvidence {
			evidenceCount++
			if evidenceCount >= 2 {
				return true
			}
		}
	}
	return false
}

// shouldKeepSourceInventoryExploreWindowUnified keeps bounded source-inventory
// dimensions in one explorer dispatch. A typed source_inventory_profile is the
// strongest signal, but local models sometimes omit that optional field while
// still emitting a structured category-enumeration IR plus a bounded
// RequiredFiles set under one source directory. In that fallback shape we also
// keep the window unified, because splitting each evidence dimension into a
// separate lane makes every lane rediscover the same directory/member set.
//
// This is scheduling only: it does not decide the answer, does not synthesize
// rows, does not weaken downstream evidence validation, and does not inspect
// raw user/model prose. It consumes typed IR fields and deterministic path
// roles so the model remains authoritative over the actual answer.
func shouldKeepSourceInventoryExploreWindowUnified(ctx *types.BusContext, window []*types.TaskNode) bool {
	if exploreEvidenceNodeCount(window) < 2 {
		return false
	}
	if sourceInventoryProfileActive(ctx) {
		return true
	}
	return broadSourceEnumerationScopeActive(ctx)
}

func sourceInventoryProfileActive(ctx *types.BusContext) bool {
	return ctx != nil &&
		ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile != nil &&
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active()
}

func exploreEvidenceNodeCount(window []*types.TaskNode) int {
	evidenceCount := 0
	for _, n := range window {
		if n != nil && n.Type == types.NodeEvidence {
			evidenceCount++
		}
	}
	return evidenceCount
}

const broadSourceEnumerationMinFiles = 6

// broadSourceEnumerationScopeActive recognizes the structural "one bounded
// source inventory, multiple dimensions" shape even when the analyzer omitted
// source_inventory_profile. The guard is intentionally conservative:
//
//   - category enumeration must be typed in the IR;
//   - a sizeable set of source/config paths must be present in EvidencePlan or
//     high-confidence RequiredFileHints;
//   - those paths must share a non-root directory scope;
//   - path-role filtering follows SourceScopeProfile rather than prose cues.
//
// The fallback can only reduce redundant sibling dispatches. It never makes a
// membership/completeness claim and never authorizes final-answer supplements.
func broadSourceEnumerationScopeActive(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentEnumerate || !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	files := broadSourceEnumerationFiles(ctx)
	if len(files) < broadSourceEnumerationMinFiles {
		return false
	}
	return commonNonRootDir(files) != ""
}

func broadSourceEnumerationFiles(ctx *types.BusContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		canon := types.CanonicalRequiredFileHintPath(raw, ctx.RepoRoot)
		if canon == "" || seen[canon] || !broadSourceEnumerationPathAllowed(rm, canon) {
			return
		}
		seen[canon] = true
		out = append(out, canon)
	}
	for _, file := range ctx.AnalysisIR.EvidencePlan.RequiredFiles {
		add(file)
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.5 {
			continue
		}
		add(hint.Path)
	}
	return out
}

func broadSourceEnumerationPathAllowed(rm types.RequestModel, rel string) bool {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, `\`, `/`))
	if rel == "" || rel == "." || strings.HasSuffix(rel, "/") {
		return false
	}
	if !types.HasCodeOrConfigPathSuffix(rel) {
		return false
	}
	scope := types.SourceScopeProduction
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope != "" {
		scope = rm.SourceScopeProfile.RequestedScope
	}
	return types.SourceScopeAllowsPathRole(scope, types.ClassifySourcePathRole(rel))
}

func commonNonRootDir(files []string) string {
	var common []string
	for _, file := range files {
		dir := strings.Trim(path.Dir(strings.Trim(strings.ReplaceAll(file, `\`, `/`), "/")), "/")
		if dir == "" || dir == "." {
			return ""
		}
		parts := strings.Split(dir, "/")
		if len(common) == 0 {
			common = append([]string(nil), parts...)
			continue
		}
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
		if len(common) == 0 {
			return ""
		}
	}
	if len(common) == 0 {
		return ""
	}
	return strings.Join(common, "/")
}

// trimExploreWindowToFirstEvidence drops all but the first
// NodeEvidence entry while preserving every non-evidence companion
// (probe / validate / reconcile / counterfactual). The first
// evidence node — by readyExplorerWindowContext's declaration-order
// sort — is the lowest-index sibling, so successive scheduler
// iterations pick up t1, t2, ... in order.
//
// Example transformation for the forensic 08:14 case:
//
//	IN:  [probe, evidence_t0, evidence_t1, validate]
//	OUT: [probe, evidence_t0, validate]
//
// Next scheduler iteration after t0 markDone:
//
//	IN:  [evidence_t1, validate]   (probe + t0 already done)
//	OUT: same (only 1 evidence node, predicate does not fire)
//
// The returned slice is a freshly-allocated copy — it does NOT
// alias the input. The scheduler treats window slices as read-only
// after readyExplorerWindow returns, so the allocation matters only
// for downstream sequential modification (none today).
func trimExploreWindowToFirstEvidence(window []*types.TaskNode) []*types.TaskNode {
	if len(window) == 0 {
		return window
	}
	out := make([]*types.TaskNode, 0, len(window))
	keptFirstEvidence := false
	for _, n := range window {
		if n == nil {
			out = append(out, n)
			continue
		}
		if n.Type == types.NodeEvidence {
			if keptFirstEvidence {
				continue
			}
			keptFirstEvidence = true
		}
		out = append(out, n)
	}
	return out
}

// splitExploreWindowForDispatch turns a multi-evidence ready window into
// declaration-ordered focused windows. It preserves the E' Phase 1 companion
// semantics: non-evidence companion nodes travel with the first evidence
// window, and later sibling windows contain only their evidence node.
//
// This is the raw splitter. Production scheduling should call
// exploreWindowDispatchGroups so source-inventory and bounded source-enumeration
// guards cannot be bypassed accidentally.
func splitExploreWindowForDispatch(window []*types.TaskNode) [][]*types.TaskNode {
	if len(window) == 0 {
		return nil
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		cp := append([]*types.TaskNode(nil), window...)
		return [][]*types.TaskNode{cp}
	}
	first := make([]*types.TaskNode, 0, len(window))
	var rest [][]*types.TaskNode
	keptFirstEvidence := false
	for _, n := range window {
		if n == nil || n.Type != types.NodeEvidence {
			first = append(first, n)
			continue
		}
		if !keptFirstEvidence {
			first = append(first, n)
			keptFirstEvidence = true
			continue
		}
		rest = append(rest, []*types.TaskNode{n})
	}
	out := make([][]*types.TaskNode, 0, 1+len(rest))
	out = append(out, first)
	out = append(out, rest...)
	return out
}

// exploreWindowDispatchGroups is the production explore-window dispatch
// decision. It centralizes the raw sibling split plus the typed structural
// guards that keep bounded source inventories unified. Keep this wrapper as the
// only orchestrator call-site so future scheduling changes cannot accidentally
// make optional source_inventory_profile omissions reopen duplicate lanes.
func exploreWindowDispatchGroups(ctx *types.BusContext, window []*types.TaskNode) [][]*types.TaskNode {
	if len(window) == 0 {
		return nil
	}
	if shouldDispatchExploreNodesIndividually(window) && !shouldKeepSourceInventoryExploreWindowUnified(ctx, window) {
		return splitExploreWindowForDispatch(window)
	}
	cp := append([]*types.TaskNode(nil), window...)
	return [][]*types.TaskNode{cp}
}
