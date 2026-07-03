package tool

// Presentation v3 (docs/design/trace_projection_presentation_v3_20260702.md):
// the Trace Causal Projection section renders as a 3-block cluster —
//   1. a lead block whose Text carries a fact-only conclusion line, the
//      requested-window anchor + coverage subtraction, one tree-reading
//      sentence, and the MAIN monospace tree inside a ```text fence
//      (byte-identical across HTML / markdown / terminal — zero mermaid);
//   2. one lossless detail table (the full duration quad, per-row relation,
//      impact shape, evidence + confidence);
//   3. a file-grouped evidence index.
// The tree is anchored at the user-focused thread (🎯 root = last wakeup-path
// node); four edge kinds only (下钻 / 唤醒 / 语义 / 成因); bars are scaled to
// the requested window when the precise anchor exists and deterministically
// fall back to the batch max otherwise (never a fabricated window).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

const (
	runtimeTraceProjTreeRowChain      = "chain"
	runtimeTraceProjTreeRowCause      = "cause"
	runtimeTraceProjTreeRowSemantic   = "semantic"
	runtimeTraceProjTreeRowDepthless  = "depthless"
	runtimeTraceProjTreeRowSelf       = "self"
	runtimeTraceProjTreeRowAdjacent   = "adjacent"
	runtimeTraceProjTreeRowBackground = "background"
	runtimeTraceProjTreeRowOmitted    = "omitted"

	runtimeTraceProjTreeEdgeDrill    = "drill"
	runtimeTraceProjTreeEdgeWake     = "wake"
	runtimeTraceProjTreeEdgeSemantic = "semantic"
	runtimeTraceProjTreeEdgeCause    = "cause"

	// runtimeTraceProjTreeLabelWidth is the display-cell budget of the tree
	// label column (prefix + edge + icon + name); bars/ms/tags align after it.
	runtimeTraceProjTreeLabelWidth = 56
	// runtimeTraceProjTreeNameMinWidth is the readability floor for the NAME
	// portion of a tree row label (§7.30.2 C4b B1): a deep prefix/edge/icon may
	// never squeeze the name below this many display cells — the name budget is
	// LabelWidth minus the actual fixed-part width, floored here; deep rows
	// widen the shared label column instead of eating the name.
	runtimeTraceProjTreeNameMinWidth = 20
	// runtimeTraceProjTreeRowMaxWidth caps a rendered tree/stanza row's total
	// display width including the tag segment (§7.30.2 C4b B4). Overflowing
	// secondary tags elide to a "…" marker; the leading state/impact tag and
	// the trailing [E#] evidence reference always survive.
	runtimeTraceProjTreeRowMaxWidth = 120
	// runtimeTraceProjTreeTrunkMaxNodes bounds a long trunk display: deeper
	// middles compress into one omitted marker row (counts + cycle note kept).
	runtimeTraceProjTreeTrunkMaxNodes = 8
	runtimeTraceProjTreeBarWidth      = 10
)

type runtimeTraceProjTreeRow struct {
	Node        types.TraceCausalProjectionNode
	Kind        string
	Edge        string
	Depth       int
	Indent      int
	Last        bool
	Ancestors   []bool // per ancestor level: more siblings follow (renders │)
	Parent      string // display name of the parent node (typed 影响点)
	HasData     bool   // false = bare path transit node without its own row
	Omitted     int
	CyclePeriod int
	CycleCount  int
	EvidenceTag string
	// RecursOnChain marks a trunk row whose canonical subject already appeared
	// earlier on the rendered chain (target root first, then depth 1..K) — the
	// small-cycle shape (A→B→A) the ≥6-node cycle detector cannot see (H11,
	// customer audit 2026-07-03). Display-only annotation; the chain itself is
	// never truncated because of it.
	//
	// V4 (customer revisit 2026-07-03): the former DedupFold row flag moved to
	// the typed Node.DuplicatePublications field — one home shared by the
	// aggregation layer's pre-R2 dedup pass and this layer's H6 fold. The ×N
	// label forks on that typed count, never on guessing from the numbers.
	RecursOnChain bool
}

type runtimeTraceProjTreeModel struct {
	Target     string
	TreeRows   []runtimeTraceProjTreeRow // trunk + attached (flattened, render order)
	SelfRows   []runtimeTraceProjTreeRow // target's own state rows (under root)
	Adjacent   []runtimeTraceProjTreeRow
	Background []runtimeTraceProjTreeRow
	WindowMS   float64 // >0 = window mode; 0 = fallback (BarMaxMS denominator)
	BarMaxMS   float64
	// TrunkLen is the resolved wakeup-path trunk length (0 = flat mode); it is
	// the same value the 裁定1 demotion gate ran against, so lead selection can
	// re-apply exactly that gate instead of a diverging copy.
	TrunkLen int
	// WakeupChainRecommendedNotRun mirrors the typed projection flag (§7.30
	// 裁定3): a chain_required=true state_drilldown recommendation existed but no
	// wakeup_chain-family observation ran, so the flat-fallback header can name
	// the actual coverage cause instead of the opaque "path unresolved".
	WakeupChainRecommendedNotRun bool
	// RootFocusAnchorOnly selects the fence root label lane (R2, customer audit
	// 2026-07-03 C4a): the typed analyzer entity comparison RAN and the 🎯 root
	// matched none of the user's entities, so the label must say ‹分析锚点线程›
	// instead of falsely claiming ‹用户关注线程›. False is the fail-open default:
	// when no typed entity context reached the renderer the legacy label stays.
	RootFocusAnchorOnly bool
	// RootFocusUserEntities lists the user's thread/pid-shaped entities for the
	// anchor-only explanation line (display-only roster; empty = no note line).
	RootFocusUserEntities []string
	// UserWindowStart/UserWindowEnd is the user's originally-requested window in
	// seconds, derived DISPLAY-ONLY from a timestamp-shaped analyzer entity pair
	// (R2 双窗关系行). Zero when absent/ambiguous — the relation line then never
	// renders; nothing else consumes these values.
	UserWindowStart float64
	UserWindowEnd   float64
}

// missingWakeup reports whether any rendered row carries the typed
// missing_wakeup undrillable reason — the OTHER flat-fallback cause (§7.30
// 裁定3): the sleep interval had no sched_wakeup record in the selected window.
func (m runtimeTraceProjTreeModel) missingWakeup() bool {
	for _, rows := range [][]runtimeTraceProjTreeRow{m.TreeRows, m.SelfRows, m.Adjacent, m.Background} {
		for _, row := range rows {
			if strings.TrimSpace(row.Node.UndrillableReason) == "missing_wakeup" {
				return true
			}
		}
	}
	return false
}

type runtimeTraceProjTreeNode struct {
	row      runtimeTraceProjTreeRow
	children []*runtimeTraceProjTreeNode
}

// --- model construction ------------------------------------------------------

func buildRuntimeTraceProjTreeModel(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) runtimeTraceProjTreeModel {
	model := runtimeTraceProjTreeModel{
		WindowMS:                     projection.WindowDurationMS(),
		WakeupChainRecommendedNotRun: projection.WakeupChainRecommendedNotRun,
	}
	path := runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)
	if len(path) >= 2 {
		model.Target = path[len(path)-1]
	}
	targetKey := runtimeTraceCausalProjectionCanonicalNode(model.Target)

	// On-chain node universe: primaries + on-chain bucket + hops, deduped by
	// node key (buckets deliberately overlap; see the aggregation layer).
	// Semantic spans are excluded here — their classified copies also live in
	// OnChainCauses, but they render exclusively through the ✦ 语义 lane (a span
	// consumed as a same-subject "cause" row would appear twice).
	chainUniverse := runtimeTraceProjDedupNodes(
		runtimeTraceProjExcludeSemanticSpans(
			append(append(append([]types.TraceCausalProjectionNode{},
				runtimeTraceCausalProjectionPrimaryRoots(projection)...),
				projection.OnChainCauses...),
				projection.SupportingHops...)))
	// §7.30 裁定1: only relation-resolved rows may enter the on-chain tree.
	// Aggregate-metric rows and unknown-thread rows whose depth cannot attach
	// demote to the background-pressure stanza (merged with the existing
	// background rows) instead of rendering as on-chain placeholders.
	trunkLen := 0
	if len(path) >= 2 {
		trunkLen = len(path) - 1
	}
	model.TrunkLen = trunkLen
	chainNodes := make([]types.TraceCausalProjectionNode, 0, len(chainUniverse))
	var demoted []types.TraceCausalProjectionNode
	for _, node := range chainUniverse {
		if runtimeTraceProjNodeDemotedToBackground(node, trunkLen) {
			demoted = append(demoted, node)
			continue
		}
		chainNodes = append(chainNodes, node)
	}
	bySubject := map[string][]types.TraceCausalProjectionNode{}
	for _, node := range chainNodes {
		key := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		bySubject[key] = append(bySubject[key], node)
	}
	consumed := map[string]bool{}
	consume := func(node types.TraceCausalProjectionNode) {
		consumed[runtimeTraceCausalProjectionNodeKey(node)] = true
	}

	// Target's own rows (state / blocked-wait views of the focused thread).
	if targetKey != "" {
		for _, node := range bySubject[targetKey] {
			consume(node)
			model.SelfRows = append(model.SelfRows, runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			})
		}
	}

	// Trunk: path minus target, ordered depth 1..K (nearest upstream first),
	// bounded for very long chains.
	var trunk []string
	for i := len(path) - 2; i >= 0; i-- {
		trunk = append(trunk, path[i])
	}
	omitStart, omitEnd := -1, -1
	cyclePeriod, cycleCount := 0, 0
	if len(trunk) > runtimeTraceProjTreeTrunkMaxNodes {
		head := runtimeTraceProjTreeTrunkMaxNodes/2 + 1
		tail := runtimeTraceProjTreeTrunkMaxNodes - head
		omitStart, omitEnd = head, len(trunk)-tail
		cyclePeriod, cycleCount = runtimeTraceCausalProjectionRepeatingPath(path)
	}
	// H11: mark trunk rows whose canonical subject already appeared earlier on
	// the rendered chain (root target first, then depth 1..K). This catches the
	// small-cycle shape (VSyncGenerator→tppmgr→VSyncGenerator) that the ≥6-node
	// cycle detector above never sees. Canonical verbatim equality only.
	recurs := make([]bool, len(trunk))
	seenOnChain := map[string]bool{}
	if targetKey != "" {
		seenOnChain[targetKey] = true
	}
	for i, subject := range trunk {
		key := runtimeTraceCausalProjectionCanonicalNode(subject)
		if seenOnChain[key] {
			recurs[i] = true
		}
		seenOnChain[key] = true
	}

	semantics := append([]types.TraceCausalProjectionNode(nil), projection.SemanticSpans...)
	semanticBySubject := map[string][]types.TraceCausalProjectionNode{}
	for _, span := range semantics {
		key := runtimeTraceCausalProjectionCanonicalNode(span.Subject)
		semanticBySubject[key] = append(semanticBySubject[key], span)
	}
	semanticConsumed := map[string]bool{}

	depthAttach := map[int][]types.TraceCausalProjectionNode{}
	trunkKeys := map[string]int{}
	for i, subject := range trunk {
		trunkKeys[runtimeTraceCausalProjectionCanonicalNode(subject)] = i + 1
	}
	for _, node := range chainNodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if consumed[key] {
			continue
		}
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if _, onTrunk := trunkKeys[subjectKey]; onTrunk {
			continue // trunk pass consumes below
		}
		if node.ChainDepth > 0 && node.ChainDepth <= len(trunk) {
			depthAttach[node.ChainDepth] = append(depthAttach[node.ChainDepth], node)
			consume(node)
		}
	}

	// Recursive trunk build (depth d node's child = depth d+1 node).
	var buildTrunk func(idx int, parentName string) []*runtimeTraceProjTreeNode
	buildTrunk = func(idx int, parentName string) []*runtimeTraceProjTreeNode {
		if idx >= len(trunk) {
			return nil
		}
		if idx == omitStart {
			omitted := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Kind: runtimeTraceProjTreeRowOmitted, Omitted: omitEnd - omitStart,
				CyclePeriod: cyclePeriod, CycleCount: cycleCount, Depth: idx + 1,
			}}
			omitted.children = buildTrunk(omitEnd, "…")
			return []*runtimeTraceProjTreeNode{omitted}
		}
		subject := trunk[idx]
		depth := idx + 1
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(subject)
		var main types.TraceCausalProjectionNode
		var extra []types.TraceCausalProjectionNode
		hasData := false
		for _, node := range bySubject[subjectKey] {
			if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
				continue
			}
			if !hasData {
				main, hasData = node, true
				continue
			}
			if runtimeTraceProjNodeDisplayImpact(node) > runtimeTraceProjNodeDisplayImpact(main) {
				extra = append(extra, main)
				main = node
			} else {
				extra = append(extra, node)
			}
		}
		if hasData {
			consume(main)
			for _, node := range extra {
				consume(node)
			}
		} else {
			main = types.TraceCausalProjectionNode{Subject: subject, ChainDepth: depth}
		}
		edge := runtimeTraceProjTreeEdgeWake
		if depth == 1 {
			edge = runtimeTraceProjTreeEdgeDrill
		}
		trunkNode := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: main, Kind: runtimeTraceProjTreeRowChain, Edge: edge, Depth: depth,
			Parent: parentName, HasData: hasData, RecursOnChain: recurs[idx],
			EvidenceTag: runtimeTraceProjEvidenceTag(main, evidence, zh),
		}}
		for _, node := range extra {
			trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowCause, Edge: runtimeTraceProjTreeEdgeCause,
				Depth: depth, Parent: subject, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
		// Semantic spans attach under THEIR subject's wakee — i.e. as this
		// node's children when the span's subject is this node's upstream child
		// subject. Handled after the child trunk is built: spans whose subject
		// is trunk[idx+1] become siblings of that deeper trunk node, faithful
		// to the typed impact point (presentation v3 §5).
		children := buildTrunk(idx+1, subject)
		trunkNode.children = append(trunkNode.children, children...)
		if idx+1 < len(trunk) {
			deeperKey := runtimeTraceCausalProjectionCanonicalNode(trunk[idx+1])
			if !semanticConsumed[deeperKey] {
				for _, span := range semanticBySubject[deeperKey] {
					trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
						Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
						Depth: span.ChainDepth, Parent: subject, HasData: true,
						EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
					}})
				}
				semanticConsumed[deeperKey] = true
			}
		}
		for _, node := range depthAttach[depth+1] {
			trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
				Depth: depth + 1, Parent: subject, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
		return []*runtimeTraceProjTreeNode{trunkNode}
	}

	var roots []*runtimeTraceProjTreeNode
	roots = append(roots, buildTrunk(0, model.Target)...)
	// Semantic spans whose subject is the depth-1 trunk node hang off the root.
	if len(trunk) > 0 {
		d1Key := runtimeTraceCausalProjectionCanonicalNode(trunk[0])
		if !semanticConsumed[d1Key] {
			for _, span := range semanticBySubject[d1Key] {
				roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
					Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
					Depth: span.ChainDepth, Parent: model.Target, HasData: true,
					EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
				}})
			}
			semanticConsumed[d1Key] = true
		}
		for _, node := range depthAttach[1] {
			roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
				Depth: 1, Parent: model.Target, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
	}
	// Remaining on-chain rows (no trunk membership, no resolvable depth) — a
	// typed-faithful "depth unresolved" branch instead of an invented position.
	for _, node := range chainNodes {
		if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
			continue
		}
		consume(node)
		roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowDepthless, Edge: runtimeTraceProjTreeEdgeWake,
			Depth: node.ChainDepth, Parent: model.Target, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		}})
	}
	// Orphan semantic spans (subject not on the trunk at all).
	for _, span := range semantics {
		key := runtimeTraceCausalProjectionCanonicalNode(span.Subject)
		if semanticConsumed[key] {
			continue
		}
		roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
			Depth: span.ChainDepth, Parent: model.Target, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
		}})
	}

	var flatten func(nodes []*runtimeTraceProjTreeNode, indent int, ancestors []bool)
	flatten = func(nodes []*runtimeTraceProjTreeNode, indent int, ancestors []bool) {
		for i, n := range nodes {
			last := i == len(nodes)-1
			row := n.row
			row.Indent = indent
			row.Last = last
			row.Ancestors = append([]bool(nil), ancestors...)
			model.TreeRows = append(model.TreeRows, row)
			flatten(n.children, indent+1, append(append([]bool(nil), ancestors...), !last))
		}
	}
	flatten(roots, 0, nil)

	for _, node := range runtimeTraceProjAdjacentNodesForDisplay(projection.AdjacentCauses) {
		model.Adjacent = append(model.Adjacent, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		})
	}
	backgroundSeen := map[string]bool{}
	for _, node := range projection.BackgroundCauses {
		backgroundSeen[runtimeTraceCausalProjectionNodeKey(node)] = true
		model.Background = append(model.Background, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		})
	}
	// 裁定1 demoted rows merge into the same background stanza. The evidence
	// roster entry is taken from the ORIGINAL node (audit keeps the raw typed
	// provenance); the display copy is then normalized to background semantics —
	// its former on-chain/primary labeling was the pollution being removed. The
	// raw observation record itself is untouched.
	for _, node := range demoted {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if backgroundSeen[key] {
			continue
		}
		backgroundSeen[key] = true
		tag := runtimeTraceProjEvidenceTag(node, evidence, zh)
		node.ChainRelevance = "background"
		if node.Role == types.TraceCausalRolePrimaryRootCause || node.Role == types.TraceCausalRoleCausalHop {
			node.Role = types.TraceCausalRoleRootCauseContext
		}
		if strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary") {
			node.Predicate = "root_cause_context"
		}
		model.Background = append(model.Background, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			EvidenceTag: tag,
		})
	}
	if len(demoted) > 0 {
		sort.SliceStable(model.Background, func(i, j int) bool {
			return runtimeTraceProjNodeDisplayImpact(model.Background[i].Node) >
				runtimeTraceProjNodeDisplayImpact(model.Background[j].Node)
		})
	}

	model.BarMaxMS = runtimeTraceProjModelMaxImpact(model)
	return model
}

// runtimeTraceProjNodeDemotedToBackground implements §7.30 裁定1: a row may sit
// on the on-chain tree only when its relation is resolved. Typed
// aggregate-metric rows (window-scoped cpu/io/irq/ipi/frequency/supply pressure
// with no thread subject) and unknown-thread sentinel rows whose typed chain
// depth cannot attach inside the wakeup trunk demote to the background-pressure
// stanza. Precise typed signals only — never a prose heuristic.
func runtimeTraceProjNodeDemotedToBackground(node types.TraceCausalProjectionNode, trunkLen int) bool {
	if node.IsAggregateMetric() {
		return true
	}
	switch runtimeTraceCausalProjectionCanonicalNode(node.Subject) {
	case "unknown-thread", "unknown":
	default:
		return false
	}
	return node.ChainDepth <= 0 || node.ChainDepth > trunkLen
}

func runtimeTraceProjExcludeSemanticSpans(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
			continue
		}
		out = append(out, node)
	}
	return out
}

func runtimeTraceProjDedupNodes(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

// runtimeTraceProjAdjacentNodesForDisplay prepares the adjacent stanza (H6,
// customer audit 2026-07-03): first the same typed node-key dedupe the
// background stanza already runs, then one strictly-equal duplicate-measurement
// fold — same canonical subject + same canonical object + same canonical
// TypeToken + EXACTLY equal projected ms (pure float equality, never ±ε) AND a
// precise line/time overlap (RF2a, adversarial review 2026-07-03) merge into
// the first row's DuplicatePublications/MergedEvidenceIDs. The real customer
// shape was two irq_activity rows with identical 35.350ms over overlapping
// line ranges (793201-830007 vs 793204-830012) rendering as two stanza rows.
//
// V4 (customer revisit 2026-07-03): the load-bearing fold now lives in the
// aggregation layer (traceCausalProjectionDedupDuplicatePublications, pre-R2,
// all buckets) so ≥3 duplicates can no longer be SUM-grabbed by R2 first. This
// display pass is retained as the safety net for projections that did not go
// through the record compile, and it writes the SAME typed field
// (Node.DuplicatePublications) instead of its former private MergedCount +
// row-flag semantics — one home, one label fork.
func runtimeTraceProjAdjacentNodesForDisplay(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	var out []types.TraceCausalProjectionNode
	for _, node := range nodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		merged := false
		for i := range out {
			// Upstream ×N aggregates carry SUM semantics; only fold single
			// measurements into single measurements (or into a duplicate fold —
			// its publication count accumulates).
			if node.MergedCount > 1 || out[i].MergedCount > 1 {
				continue
			}
			if !runtimeTraceProjSameAdjacentMeasurement(out[i], node) {
				continue
			}
			runtimeTraceProjAbsorbAdjacentDuplicate(&out[i], node)
			merged = true
			break
		}
		if merged {
			continue
		}
		out = append(out, node)
	}
	return out
}

// runtimeTraceProjSameAdjacentMeasurement is the strict duplicate-measurement
// identity: canonical subject + canonical object + canonical TypeToken equal,
// the positive projected ms exactly equal, AND the two rows precisely overlap
// in the artifact — line-range intersection or time-span intersection (RF2a,
// adversarial review 2026-07-03: two REAL irq bursts at different moments can
// quantize to the same %.3f ms; folding them halves the reported contribution).
// When neither location lane is determinate for the pair the fold fails open
// to two rows — value equality alone never merges.
func runtimeTraceProjSameAdjacentMeasurement(a, b types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionCanonicalNode(a.Subject) == runtimeTraceCausalProjectionCanonicalNode(b.Subject) &&
		runtimeTraceCausalProjectionCanonicalNode(a.Object) == runtimeTraceCausalProjectionCanonicalNode(b.Object) &&
		runtimeTraceCausalProjectionCanonicalNode(a.TypeToken) == runtimeTraceCausalProjectionCanonicalNode(b.TypeToken) &&
		a.ImpactMS > 0 && a.ImpactMS == b.ImpactMS &&
		(runtimeTraceProjLineSpansOverlap(a, b) || runtimeTraceProjTimeSpansOverlap(a, b))
}

// runtimeTraceProjLineSpansOverlap is the boolean line-range intersection; both
// nodes must expose a valid range of their own (same guard style as the typed
// peer-alias fold's traceCausalProjectionSpansOverlap in internal/types).
func runtimeTraceProjLineSpansOverlap(a, b types.TraceCausalProjectionNode) bool {
	if a.LineStart <= 0 || a.LineEnd < a.LineStart || b.LineStart <= 0 || b.LineEnd < b.LineStart {
		return false
	}
	return a.LineStart <= b.LineEnd && b.LineStart <= a.LineEnd
}

// runtimeTraceProjTimeSpansOverlap is the boolean time-span intersection; both
// nodes must expose a valid span of their own. Local isomorph of the unexported
// types-layer traceCausalProjectionSpansOverlap
// (internal/types/trace_causal_projection_aggregate.go) — keep the two aligned.
func runtimeTraceProjTimeSpansOverlap(a, b types.TraceCausalProjectionNode) bool {
	if a.StartTs <= 0 || a.EndTs <= a.StartTs || b.StartTs <= 0 || b.EndTs <= b.StartTs {
		return false
	}
	return a.StartTs < b.EndTs && b.StartTs < a.EndTs
}

// runtimeTraceProjAbsorbAdjacentDuplicate folds one duplicate measurement into
// the surviving first-occurrence row: publication count + evidence union only —
// the projected value stays the survivor's (the rows measured the same amount;
// a sum would double-count the wall clock). V4: writes the typed
// DuplicatePublications field shared with the aggregation-layer pass; the
// former MergedCount/MergedMin/Max writes are gone (those carry SUM-aggregate
// semantics), and the former subject-roster append was dead code — the fold
// identity requires equal canonical subjects.
func runtimeTraceProjAbsorbAdjacentDuplicate(survivor *types.TraceCausalProjectionNode, dup types.TraceCausalProjectionNode) {
	if survivor.DuplicatePublications < 1 {
		survivor.DuplicatePublications = 1
	}
	add := dup.DuplicatePublications
	if add < 1 {
		add = 1
	}
	survivor.DuplicatePublications += add
	absorbed := map[string]bool{runtimeTraceCausalProjectionCanonicalNode(survivor.EvidenceID): true}
	for _, id := range survivor.MergedEvidenceIDs {
		absorbed[runtimeTraceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[runtimeTraceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[runtimeTraceCausalProjectionCanonicalNode(raw)] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, raw)
	}
	appendID(dup.EvidenceID)
	for _, id := range dup.MergedEvidenceIDs {
		appendID(id)
	}
}

func runtimeTraceProjNodeDisplayImpact(node types.TraceCausalProjectionNode) float64 {
	if node.ImpactMS > 0 {
		return node.ImpactMS
	}
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return node.ActualImpactMS
}

func runtimeTraceProjModelMaxImpact(model runtimeTraceProjTreeModel) float64 {
	max := 0.0
	consider := func(rows []runtimeTraceProjTreeRow) {
		for _, row := range rows {
			if v := runtimeTraceProjNodeDisplayImpact(row.Node); v > max {
				max = v
			}
		}
	}
	consider(model.TreeRows)
	consider(model.SelfRows)
	consider(model.Adjacent)
	consider(model.Background)
	return max
}

func runtimeTraceProjEvidenceTag(node types.TraceCausalProjectionNode, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) string {
	id := evidence.add(node, zh)
	if id == "" {
		return ""
	}
	if n := len(node.MergedEvidenceIDs); n > 0 {
		return fmt.Sprintf("%s(+%d)", id, n)
	}
	return id
}

// --- user-focus comparison (R2) -------------------------------------------------

// runtimeTraceProjUserFocus carries the typed analyzer entity context the
// projection renderer may compare the 🎯 root against (R2, customer audit
// 2026-07-03 C4a). Entities is AnalyzerHints.Entities ∪ ExactTargets verbatim —
// never RawRequest, never model prose. An empty list means the comparison
// cannot run and every consumer fails open to legacy behavior.
type runtimeTraceProjUserFocus struct {
	Entities []string
}

// runtimeTraceProjApplyUserFocus runs the precise root-vs-entity comparison and
// the display-only user-window derivation on a built model. No typed entity
// context → the model keeps its zero values (legacy label, no relation line).
func runtimeTraceProjApplyUserFocus(model *runtimeTraceProjTreeModel, focus runtimeTraceProjUserFocus) {
	if model == nil || len(focus.Entities) == 0 {
		return
	}
	if start, end, ok := runtimeTraceProjUserWindowFromEntities(focus.Entities); ok {
		model.UserWindowStart, model.UserWindowEnd = start, end
	}
	target := strings.TrimSpace(model.Target)
	if target == "" {
		return
	}
	if runtimeTraceProjTargetMatchesUserEntities(target, focus.Entities) {
		return // 🎯 root really is a user-named thread — keep ‹用户关注线程›
	}
	model.RootFocusAnchorOnly = true
	model.RootFocusUserEntities = runtimeTraceProjThreadOrPidEntities(focus.Entities)
}

// runtimeTraceProjTargetMatchesUserEntities decides whether the 🎯 root names a
// user entity via PRECISE signals only (架构红线: hard-ish display switches read
// integer equality / verbatim equality, never fuzzy containment):
//   - whole target verbatim equal to an entity;
//   - the target's name part (before the trailing -pid) verbatim equal;
//   - the target's pid integer (trailing -pid tail, or the bare "pid=N" handle
//     traceThreadLabel emits when the Comm was never resolved — every
//     wakeup-path label passes through it; RF1b, adversarial review
//     2026-07-03) equal to a pure-digit or "pid=N"-shaped entity's integer
//     (RF1a).
func runtimeTraceProjTargetMatchesUserEntities(target string, entities []string) bool {
	name, pid, hasPid := runtimeTraceProjSplitNamePid(target)
	if !hasPid {
		// RF1b target side: a bare pid handle has no name part to compare.
		pid, hasPid = runtimeTraceProjPidHandleForm(target)
	}
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		if entity == target {
			return true
		}
		if hasPid && name != "" && entity == name {
			return true
		}
		if hasPid {
			if n, ok := runtimeTraceProjPureInt(entity); ok && n == pid {
				return true
			}
			// RF1a entity side: the analyzer may hand the focus as "pid=N".
			if n, ok := runtimeTraceProjPidHandleForm(entity); ok && n == pid {
				return true
			}
		}
	}
	return false
}

// runtimeTraceProjPidHandleForm matches the literal "pid=N" thread handle
// (character-class check: the fixed prefix plus pure digits — "pidx=42591"
// and "pid=42591abc" both fail). Local isomorph of the unexported types-layer
// traceCausalProjectionPidPeerForm
// (internal/types/trace_causal_projection_aggregate.go) — keep the two
// character-class checks aligned.
func runtimeTraceProjPidHandleForm(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "pid=") {
		return 0, false
	}
	return runtimeTraceProjPureInt(strings.TrimPrefix(s, "pid="))
}

// runtimeTraceProjSplitNamePid splits the canonical thread label form name-pid
// (character-class validation: non-empty name, pure-digit tail after the last
// '-'). ok=false when the label does not carry a pid tail.
func runtimeTraceProjSplitNamePid(label string) (string, int, bool) {
	idx := strings.LastIndex(label, "-")
	if idx <= 0 || idx == len(label)-1 {
		return "", 0, false
	}
	pid, ok := runtimeTraceProjPureInt(label[idx+1:])
	if !ok {
		return "", 0, false
	}
	return label[:idx], pid, true
}

// runtimeTraceProjPureInt parses a pure-digit string (character-class check —
// display/comparison lane only, exempted from the no-keyword-matching rule).
func runtimeTraceProjPureInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// runtimeTraceProjThreadOrPidEntities selects the user entities that LOOK like
// a thread/pid handle (pure-digit pid, the "pid=N" handle form — RF1,
// adversarial review 2026-07-03 — or the name-pid label form) for the
// anchor-only note's "用户关注: …" roster. Display-only; capped at 2.
func runtimeTraceProjThreadOrPidEntities(entities []string) []string {
	var out []string
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		if _, ok := runtimeTraceProjPureInt(entity); ok {
			out = append(out, entity)
		} else if _, ok := runtimeTraceProjPidHandleForm(entity); ok {
			out = append(out, entity)
		} else if _, _, ok := runtimeTraceProjSplitNamePid(entity); ok {
			out = append(out, entity)
		}
		if len(out) == 2 {
			break
		}
	}
	return out
}

// runtimeTraceProjUserWindowFromEntities derives the user-requested window from
// the typed entity set: EXACTLY two timestamp-shaped entities (character-class:
// digits '.' digits, optional trailing "s") with start < end. Anything else —
// zero, one, three or more stamps, or a non-increasing pair — is ambiguous and
// yields nothing. Display-only lane (R2 双窗关系行).
func runtimeTraceProjUserWindowFromEntities(entities []string) (float64, float64, bool) {
	var stamps []float64
	for _, entity := range entities {
		v, ok := runtimeTraceProjTimestampShaped(strings.TrimSpace(entity))
		if !ok {
			continue
		}
		stamps = append(stamps, v)
		if len(stamps) > 2 {
			return 0, 0, false
		}
	}
	if len(stamps) != 2 || stamps[0] <= 0 || stamps[1] <= stamps[0] {
		return 0, 0, false
	}
	return stamps[0], stamps[1], true
}

// runtimeTraceProjTimestampShaped validates the timestamp character class:
// digits '.' digits with an optional trailing seconds unit "s".
func runtimeTraceProjTimestampShaped(s string) (float64, bool) {
	s = strings.TrimSuffix(s, "s")
	dot := strings.IndexByte(s, '.')
	if dot <= 0 || dot == len(s)-1 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if i == dot {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// --- tree fence rendering -----------------------------------------------------

func runtimeTraceProjTreeFence(model runtimeTraceProjTreeModel, zh bool) string {
	if len(model.TreeRows) == 0 && len(model.SelfRows) == 0 &&
		len(model.Adjacent) == 0 && len(model.Background) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```text\n")
	windowMode := model.WindowMS > 0
	denom := model.WindowMS
	if !windowMode {
		denom = model.BarMaxMS
	}
	width := runtimeTraceProjTreeLabelColumn(model, zh)
	// Header: target anchor + explicit bar-scale declaration. The root label is
	// honest about provenance (R2, customer audit 2026-07-03 C4a): ‹用户关注线程›
	// only when the typed analyzer entity comparison matched (or never ran —
	// fail-open keeps the legacy label); a mismatch renders ‹分析锚点线程› plus a
	// quiet one-line note naming the user's actual focus entities.
	if strings.TrimSpace(model.Target) != "" {
		header := "🎯 " + model.Target
		switch {
		case model.RootFocusAnchorOnly && zh:
			header += " ‹分析锚点线程›"
		case model.RootFocusAnchorOnly:
			header += " <analysis anchor thread>"
		case zh:
			header += " ‹用户关注线程›"
		default:
			header += " <user-focused thread>"
		}
		b.WriteString(runtimeTraceProjPadDisplay(header, width))
		b.WriteString(" ")
		b.WriteString(runtimeTraceProjScaleNote(model, zh))
		b.WriteString("\n")
		if model.RootFocusAnchorOnly && len(model.RootFocusUserEntities) > 0 {
			if zh {
				b.WriteString("- 根为唤醒链锚点线程,非用户指定关注对象(用户关注: " + strings.Join(model.RootFocusUserEntities, "、") + ")\n")
			} else {
				b.WriteString("- root is the wakeup-chain anchor thread, not the user-specified focus (user focus: " + strings.Join(model.RootFocusUserEntities, ", ") + ")\n")
			}
		}
	} else {
		b.WriteString(runtimeTraceProjFlatFallbackHeader(model, zh) + "  " + runtimeTraceProjScaleNote(model, zh) + "\n")
	}
	for _, row := range model.SelfRows {
		b.WriteString("│     " + runtimeTraceProjSelfRowText(row, zh) + "\n")
	}
	if len(model.TreeRows) > 0 && strings.TrimSpace(model.Target) != "" {
		b.WriteString("│\n")
	}
	for _, row := range model.TreeRows {
		b.WriteString(runtimeTraceProjTreeRowLine(row, width, denom, windowMode, zh))
		b.WriteString("\n")
	}
	if len(model.Adjacent) > 0 {
		b.WriteString("\n")
		if zh {
			b.WriteString("◇ 邻近链 — 与主链时间相邻,不在唤醒路径上\n")
		} else {
			b.WriteString("◇ Adjacent — time-adjacent to the chain, not on the wakeup path\n")
		}
		for _, row := range model.Adjacent {
			b.WriteString(runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	if len(model.Background) > 0 {
		b.WriteString("\n")
		if zh {
			b.WriteString("▒ 背景压力 — 环境证据,不计入链上归因,需结合 on-chain 证据解读\n")
		} else {
			b.WriteString("▒ Background pressure — environmental evidence, not chain attribution; read with on-chain evidence\n")
		}
		for _, row := range model.Background {
			b.WriteString(runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	b.WriteString("```")
	return b.String()
}

// runtimeTraceProjTreeLabelColumn returns the shared label-column width for a
// fence render (§7.30.2 C4b B1): the fixed 56-cell budget, widened only when a
// deep bar row's fixed prefix/edge/icon width plus its floored name budget
// cannot fit. All bar rows pad to ONE column, so bars keep a single aligned
// start column at every level; shallow trees render byte-identically to the
// fixed budget. Rows without metrics (omitted markers, bare transit nodes)
// never widen the column — they carry no bar to align.
func runtimeTraceProjTreeLabelColumn(model runtimeTraceProjTreeModel, zh bool) int {
	width := runtimeTraceProjTreeLabelWidth
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
			continue
		}
		fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
		fixedW := runewidth.StringWidth(fixed)
		budget := runtimeTraceProjTreeLabelWidth - fixedW
		if budget < runtimeTraceProjTreeNameMinWidth {
			budget = runtimeTraceProjTreeNameMinWidth
		}
		nameW := runewidth.StringWidth(name)
		if nameW > budget {
			nameW = budget
		}
		if need := fixedW + nameW; need > width {
			width = need
		}
	}
	return width
}

// runtimeTraceProjTreeLabelParts splits a tree row label into its fixed part
// (prefix + edge + icon + separators) and the name, so truncation can target
// the name alone instead of the composed label (B1).
func runtimeTraceProjTreeLabelParts(row runtimeTraceProjTreeRow, zh bool) (string, string) {
	edge := runtimeTraceProjEdgeLabel(row.Edge, zh)
	if row.Kind == runtimeTraceProjTreeRowDepthless && strings.TrimSpace(row.Parent) == "" {
		// Flat fallback (no resolved target): a hanging "wakes" edge word would
		// claim a wake relation with no wakee — render a bare branch instead.
		edge = ""
	}
	fixed := runtimeTraceProjTreePrefix(row) + edge + " " +
		runtimeTraceProjStateIcon(row.Node, row.Kind) + " "
	return fixed, runtimeTraceProjRowName(row, zh)
}

// runtimeTraceProjTreeLabel composes a row label with name-scoped truncation
// (B1): the name budget is the base label width minus the actual fixed-part
// display width, floored at the readability minimum, then the whole label pads
// (never truncates) to the shared column width.
func runtimeTraceProjTreeLabel(fixed, name string, width int) string {
	budget := runtimeTraceProjTreeLabelWidth - runewidth.StringWidth(fixed)
	if budget < runtimeTraceProjTreeNameMinWidth {
		budget = runtimeTraceProjTreeNameMinWidth
	}
	if runewidth.StringWidth(name) > budget {
		name = runtimeTraceProjPadDisplay(name, budget)
	}
	label := fixed + name
	if pad := width - runewidth.StringWidth(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return label
}

// runtimeTraceProjFlatFallbackHeader names WHY the tree renders flat (§7.30
// 裁定3, two typed causes): a missing_wakeup row means the sleep interval had no
// sched_wakeup record in the selected window; the recommended-not-run flag means
// the wakeup-chain drilldown was never executed this round. Both are precise
// typed signals; the opaque "path unresolved" wording stays only as the last
// fallback.
func runtimeTraceProjFlatFallbackHeader(model runtimeTraceProjTreeModel, zh bool) string {
	switch {
	case model.missingWakeup():
		if zh {
			return "(睡眠区间在所选窗口内无 sched_wakeup 记录,唤醒链无法上溯——按层级平铺展示)"
		}
		return "(the sleep interval has no sched_wakeup record in the selected window — the wakeup chain cannot be traced upstream; layers rendered flat)"
	case model.WakeupChainRecommendedNotRun:
		if zh {
			return "(本轮未执行唤醒链下钻,建议 trace_query view=wakeup_chain——按层级平铺展示)"
		}
		return "(wakeup-chain drilldown was not run this round — recommend trace_query view=wakeup_chain; layers rendered flat)"
	default:
		if zh {
			return "(唤醒链路径未解析——按层级平铺展示)"
		}
		return "(wakeup path unresolved — layers rendered flat)"
	}
}

func runtimeTraceProjScaleNote(model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS > 0 {
		if zh {
			return fmt.Sprintf("满格=窗口%.3fms", model.WindowMS)
		}
		return fmt.Sprintf("bar full = window %.3fms", model.WindowMS)
	}
	if zh {
		return fmt.Sprintf("窗口起止未采集·满格=本批最大%.3fms(回退尺度,不显示占窗%%)", model.BarMaxMS)
	}
	return fmt.Sprintf("window bounds not captured; bar full = batch max %.3fms (fallback scale, no window %%)", model.BarMaxMS)
}

func runtimeTraceProjSelfRowText(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	var parts []string
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if node.IsSleepState() {
		state := strings.TrimSpace(node.StateKind)
		if state == "" {
			state = "sleep"
		}
		parts = append(parts, "💤 "+state)
	} else if name != "" {
		parts = append(parts, name)
	}
	if v := runtimeTraceProjNodeDisplayImpact(node); v > 0 {
		parts = append(parts, fmt.Sprintf("%.3fms", v))
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels.
	if node.DuplicatePublications > 1 {
		parts = append(parts, runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh))
	}
	if node.MergedCount > 1 {
		if zh {
			parts = append(parts, fmt.Sprintf("×%d合并(单次%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		} else {
			parts = append(parts, fmt.Sprintf("×%d merged (each %.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		}
	}
	// 裁定4 applies to the target's own status rows too: the tree legend
	// promises "rows without a dominant state keep their impact-shape
	// value", and every other row family (chain / cause / adjacent /
	// background / merged) already renders it — a lock-contention self
	// row must say 锁竞争·阻塞 at a glance instead of a bare duration
	// (lock_001 customer report, 2026-07-03). Sleep rows keep their
	// dedicated wording below.
	if !node.IsSleepState() {
		stateTag := runtimeTraceProjStateKindLabel(node, zh)
		if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
			runtimeTraceCausalProjectionInversionRow(node) {
			stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		}
		if stateTag != "" {
			parts = append(parts, stateTag)
		}
	}
	if node.IsSleepState() {
		if zh {
			parts = append(parts, "窗口内主要处于等待唤醒")
		} else {
			parts = append(parts, "mostly waiting for wakeup inside the window")
		}
	}
	if node.Undrillable() {
		if zh {
			parts = append(parts, "⛔窗口内无匹配 sched_wakeup("+node.UndrillableReason+"),无法下钻")
		} else {
			parts = append(parts, "⛔no matching sched_wakeup in the window ("+node.UndrillableReason+") — cannot drill")
		}
	}
	if row.EvidenceTag != "" {
		parts = append(parts, "["+row.EvidenceTag+"]")
	}
	return strings.Join(parts, " ")
}

func runtimeTraceProjTreePrefix(row runtimeTraceProjTreeRow) string {
	var b strings.Builder
	for _, more := range row.Ancestors {
		if more {
			b.WriteString("│   ")
		} else {
			b.WriteString("    ")
		}
	}
	if row.Last {
		b.WriteString("└─")
	} else {
		b.WriteString("├─")
	}
	return b.String()
}

func runtimeTraceProjEdgeLabel(edge string, zh bool) string {
	switch edge {
	case runtimeTraceProjTreeEdgeDrill:
		if zh {
			return "下钻─"
		}
		return "drill─"
	case runtimeTraceProjTreeEdgeSemantic:
		if zh {
			return "语义─"
		}
		return "span─"
	case runtimeTraceProjTreeEdgeCause:
		if zh {
			return "成因─"
		}
		return "cause─"
	default:
		if zh {
			return "唤醒─"
		}
		return "wakes─"
	}
}

func runtimeTraceProjStateIcon(node types.TraceCausalProjectionNode, kind string) string {
	if kind == runtimeTraceProjTreeRowSemantic {
		return "✦"
	}
	if node.IsSleepState() {
		return "💤"
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running":
		return "⚙"
	case "runnable":
		return "⏳"
	case "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		return "⛓"
	default:
		return "◦"
	}
}

func runtimeTraceProjRowName(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		name := strings.TrimSpace(node.SpanName)
		if name == "" {
			name = strings.TrimSpace(node.Object)
		}
		return name
	}
	if node.IsAggregateMetric() {
		// The metric IS the subject; the Object type word is already folded into
		// the semantic name (裁定2 rendering half).
		return strings.TrimSpace(runtimeTraceCausalProjectionAggregateMetricName(node, zh))
	}
	// §7.30.3 D1: lock-contention rows render their typed semantics (with the
	// parsed holder) instead of an unresolved-peer label + bare duration.
	if blocking := runtimeTraceCausalProjectionBlockingName(node, zh); blocking != "" {
		if runtimeTraceCausalProjectionKnownSubject(node.Subject) {
			return strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh)) + " · " + blocking
		}
		return blocking
	}
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	// D2: tree and cause rows show the concise zh label for recognized type
	// tokens (the raw token stays on the detail table's 类型 column).
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if row.Kind == runtimeTraceProjTreeRowCause {
		// Same-subject cause decomposition: the subject is already the parent
		// trunk row; show only the cause word.
		if object != "" {
			return object
		}
		return subject
	}
	if node.MergedCount > 1 && node.Subject == "" {
		// The fold line keeps the folded rows' thread names (customer
		// 2026-07-03: a bare "其余 N 项合并" lost every thread identity).
		if zh {
			return fmt.Sprintf("其余 %d 项合并%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
		}
		return fmt.Sprintf("%d more folded%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
	}
	switch {
	case subject != "" && object != "":
		return subject + " · " + object
	case subject != "":
		return subject
	default:
		return object
	}
}

// runtimeTraceProjDedupFoldTagText is the dedupe-exclusive ×N label (RF2b,
// adversarial review 2026-07-03): a duplicate-publication row's ms is ONE
// measurement that was published N times, so it must never share the upstream
// R2 sum-aggregate rendering form "×N合并(单次…)" — a reader could not tell a
// single measurement from a total. Callers fork on the typed
// Node.DuplicatePublications count (V4: one home for both fold producers).
func runtimeTraceProjDedupFoldTagText(count int, zh bool) string {
	if zh {
		return fmt.Sprintf("×%d同值合并(重复发布)", count)
	}
	return fmt.Sprintf("×%d same-value merge (duplicate publication)", count)
}

// runtimeTraceProjSubjectlessFoldRow identifies the R3 background fold row
// (typed identity: a merged row with no thread subject of its own — exactly the
// key the "其余 N 项合并" name lane already forks on). It is the only merged
// row family whose published value is the member MAX (V3, customer revisit
// 2026-07-03): the members are different threads, whose wall clocks never sum.
func runtimeTraceProjSubjectlessFoldRow(node types.TraceCausalProjectionNode) bool {
	return node.MergedCount > 1 && strings.TrimSpace(node.Subject) == ""
}

// runtimeTraceProjMergedSubjectsSuffix names the folded members' thread
// subjects on a merged row: "(A、B 等)" — the trailing 等/… appears only when
// MergedCount exceeds the preserved roster (the typed cap lives at the
// aggregation site). Empty when the row carries no MergedSubjects.
func runtimeTraceProjMergedSubjectsSuffix(node types.TraceCausalProjectionNode, zh bool) string {
	if len(node.MergedSubjects) == 0 {
		return ""
	}
	if zh {
		suffix := strings.Join(node.MergedSubjects, "、")
		if node.MergedCount > len(node.MergedSubjects) {
			suffix += " 等"
		}
		return "(" + suffix + ")"
	}
	suffix := strings.Join(node.MergedSubjects, ", ")
	if node.MergedCount > len(node.MergedSubjects) {
		suffix += ", …"
	}
	return " (" + suffix + ")"
}

func runtimeTraceProjBar(value, denom float64, background bool) string {
	if denom <= 0 || value <= 0 {
		return strings.Repeat("░", runtimeTraceProjTreeBarWidth)
	}
	filled := int(value/denom*float64(runtimeTraceProjTreeBarWidth) + 0.5)
	if filled < 1 {
		filled = 1
	}
	if filled > runtimeTraceProjTreeBarWidth {
		filled = runtimeTraceProjTreeBarWidth
	}
	cell := "█"
	if background {
		cell = "▒"
	}
	return strings.Repeat(cell, filled) + strings.Repeat("░", runtimeTraceProjTreeBarWidth-filled)
}

// runtimeTraceProjPadDisplay pads (or runewise truncates with …) to a display
// width so bars align in monospace surfaces; CJK/emoji measured via runewidth.
func runtimeTraceProjPadDisplay(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	runes := []rune(s)
	for len(runes) > 0 && runewidth.StringWidth(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	out := string(runes) + "…"
	if pad := width - runewidth.StringWidth(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

func runtimeTraceProjTreeRowLine(row runtimeTraceProjTreeRow, width int, denom float64, windowMode, zh bool) string {
	if row.Kind == runtimeTraceProjTreeRowOmitted {
		prefix := runtimeTraceProjTreePrefix(row)
		note := ""
		if zh {
			note = fmt.Sprintf("…省略%d个链路节点…", row.Omitted)
			if row.CyclePeriod > 0 && row.CycleCount > 0 {
				note += fmt.Sprintf("(检测到%d节点循环约%d轮)", row.CyclePeriod, row.CycleCount)
			}
			note += " 完整链路见原始 trace_query 记录"
		} else {
			note = fmt.Sprintf("…%d chain nodes omitted…", row.Omitted)
			if row.CyclePeriod > 0 && row.CycleCount > 0 {
				note += fmt.Sprintf(" (≈%d-node cycle ×%d)", row.CyclePeriod, row.CycleCount)
			}
			note += " full chain remains in the trace_query record"
		}
		return prefix + " " + note
	}
	fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
	left := runtimeTraceProjTreeLabel(fixed, name, width)
	var line string
	if !row.HasData {
		if zh {
			line = left + " (链路中转,本轮无独立影响行)"
		} else {
			line = left + " (chain transit, no standalone impact row this run)"
		}
	} else {
		line = runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
	}
	// H11 small-cycle annotation: the canonical subject already appeared earlier
	// on the rendered chain — say so at end of row instead of letting the repeat
	// read as a distinct thread. Display-only; never truncates the chain.
	if row.RecursOnChain {
		if zh {
			line += " ↺(线程在链上重复出现)"
		} else {
			line += " ↺ (recurs on chain)"
		}
	}
	return line
}

func runtimeTraceProjStanzaRowLine(row runtimeTraceProjTreeRow, width int, denom float64, windowMode, zh bool) string {
	left := runtimeTraceProjTreeLabel("    ", runtimeTraceProjRowName(row, zh), width)
	return runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
}

// runtimeTraceProjRowLineWithMetrics assembles label + bar/ms cells + tags
// under the total row-width cap (§7.30.2 C4b B4): the tag segment gets the
// remaining budget and elides secondary tags when it would overflow.
func runtimeTraceProjRowLineWithMetrics(left string, row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	base, tags := runtimeTraceProjRowMetricParts(row, denom, windowMode, zh)
	if len(tags) == 0 {
		return left + " " + base
	}
	budget := runtimeTraceProjTreeRowMaxWidth -
		runewidth.StringWidth(left) - 1 - runewidth.StringWidth(base) - 2
	return left + " " + base + "  " + runtimeTraceProjFitTags(tags, budget)
}

// runtimeTraceProjTag is one tag cell of a tree/stanza row with its typed
// elision class (§7.30.2 C4b B4). DropOrder is assigned at the tag's build
// site — a precise typed signal, never re-derived from the rendered text.
type runtimeTraceProjTag struct {
	Text string
	// DropOrder: 0 = load-bearing, never dropped (the leading state/impact
	// attribution, ⚠ cross-window and ⛔ missing_wakeup markers, the [E#]
	// evidence reference); 1 = typed action lane (drops last among the
	// droppable); 2 = detail extras the lossless table mirrors (chain cum,
	// merged range, impact points, attribution note, span host); 3 =
	// layer/priority chip (first to go — the detail table's 因果位置·优先级
	// column is the authoritative surface for it).
	DropOrder int
}

const (
	runtimeTraceProjTagKeep      = 0
	runtimeTraceProjTagAction    = 1
	runtimeTraceProjTagExtra     = 2
	runtimeTraceProjTagLayerChip = 3
)

// runtimeTraceProjFitTags joins the tag segment within the row-width budget
// (B4): when the full join overflows, droppable tags elide to a "…" marker in
// typed DropOrder (layer chip first, then table-mirrored extras end-first,
// then the action lane). Load-bearing tags — the leading state attribution
// (裁定4), ⚠/⛔ markers (gaps c/e) and the [E#] evidence reference — always
// survive, even when that leaves the row slightly over the soft cap. The
// detail table stays the lossless surface for everything elided.
func runtimeTraceProjFitTags(tags []runtimeTraceProjTag, budget int) string {
	if len(tags) == 0 {
		return ""
	}
	assemble := func(dropped map[int]bool) string {
		var parts []string
		elided := false
		for i, tag := range tags {
			if dropped[i] {
				if !elided {
					parts = append(parts, "…")
					elided = true
				}
				continue
			}
			parts = append(parts, tag.Text)
		}
		return strings.Join(parts, " · ")
	}
	candidate := assemble(nil)
	if runewidth.StringWidth(candidate) <= budget {
		return candidate
	}
	dropped := map[int]bool{}
	for order := runtimeTraceProjTagLayerChip; order >= runtimeTraceProjTagAction; order-- {
		for i := len(tags) - 1; i >= 0; i-- {
			if tags[i].DropOrder != order {
				continue
			}
			dropped[i] = true
			candidate = assemble(dropped)
			if runewidth.StringWidth(candidate) <= budget {
				return candidate
			}
		}
	}
	// Readability floor: only load-bearing tags remain — keep them even over
	// budget; the state/⚠/⛔/E# references must never be dropped.
	return candidate
}

// runtimeTraceProjStateKindLabel is the bar-row state attribution (§7.30
// 裁定4): a localized label for the node's typed dominant scheduler state.
// Empty when the node exposes no StateKind — callers then fall back to the
// impact-shape cell value instead of fabricating a state.
func runtimeTraceProjStateKindLabel(node types.TraceCausalProjectionNode, zh bool) string {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait":
		if zh {
			return "睡眠等待"
		}
		return "sleep wait"
	case "runnable":
		if zh {
			return "可运行等待"
		}
		return "runnable wait"
	case "running":
		if zh {
			return "运行占用"
		}
		return "running"
	case "io_wait":
		if zh {
			return "IO阻塞"
		}
		return "IO wait"
	case "d_state", "d_sleep", "uninterruptible_sleep":
		if zh {
			return "D状态"
		}
		return "D-state"
	}
	return ""
}

// runtimeTraceProjRowMetricParts renders the fixed metric cells (bar + ms +
// window %) and returns the tag list separately so the caller can fit the tag
// segment into the remaining row-width budget (B4).
func runtimeTraceProjRowMetricParts(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) (string, []runtimeTraceProjTag) {
	node := row.Node
	impact := runtimeTraceProjNodeDisplayImpact(node)
	var b strings.Builder
	b.WriteString(runtimeTraceProjBar(impact, denom, row.Kind == runtimeTraceProjTreeRowBackground))
	b.WriteString(fmt.Sprintf(" %9.3fms", impact))
	if windowMode && denom > 0 && impact > 0 {
		b.WriteString(fmt.Sprintf(" %3.0f%%", impact/denom*100))
		// H8: an over-window share (cross-CPU / multi-span cumulative values can
		// legitimately exceed the wall-clock window) must not run naked — the bar
		// is already capped, so the number carries the explanation inline. The
		// *1.001 tolerance mirrors runtimeTraceProjCrossWindow: WindowMS comes
		// from an anchor float subtraction, so an EXACT whole-window projection
		// (101.000ms vs 100.9999…ms) must not read as "over the window" (V3,
		// customer revisit 2026-07-03).
		if impact > denom*1.001 {
			if zh {
				b.WriteString("(跨CPU/多段累计)")
			} else {
				b.WriteString(" (multi-CPU/multi-span cumulative)")
			}
		}
	}
	var tags []runtimeTraceProjTag
	// 裁定4: every bar row states WHAT the duration was (typed StateKind label;
	// impact-shape value when no state was exposed — never fabricated).
	// §7.30.3 D1/D3: typed lock-contention rows and gated-composite inversion
	// rows always carry their semantic label — the shape cell wins over any
	// single-state claim (an inversion composite is NOT "running").
	stateTag := runtimeTraceProjStateKindLabel(node, zh)
	if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
		runtimeTraceCausalProjectionInversionRow(node) {
		stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
	}
	if stateTag != "" {
		tags = append(tags, runtimeTraceProjTag{Text: stateTag, DropOrder: runtimeTraceProjTagKeep})
	}
	// V3 (customer revisit 2026-07-03): a background row whose projection covers
	// ≥99% of the window — without exceeding it — waited out the whole window:
	// the renderer face of the producer-side Q2 idle whole-window-sleeper fold
	// signal. Over-window values (H8 tolerance: > denom*1.001) are the
	// multi-CPU cumulative shape — an ACTIVE burst, never tagged idle. F3: the
	// full judgment (incl. the typed wait-family StateKind guard) lives in the
	// shared helper; the detail table mirrors the same call.
	if windowMode && runtimeTraceProjWholeWindowIdleRow(row, denom) {
		text := "整窗等待(疑似空闲)"
		if !zh {
			text = "whole-window wait (likely idle)"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep})
	}
	// §7.30.3 D3: the inversion composite shows its gated composition — the
	// split is load-bearing and never elides.
	if runtimeTraceCausalProjectionInversionRow(node) &&
		(node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0) {
		text := fmt.Sprintf("影响构成: 可运行等待 %.3fms + 运行折算 %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
		if !zh {
			text = fmt.Sprintf("composition: runnable %.3fms + discounted running %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep})
	}
	// §7.30.3 D1: the parsed holder site is auditable detail — droppable on
	// width pressure; the raw record keeps it lossless.
	if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
		text := "持有点 " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		if !zh {
			text = "held at " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	layer := runtimeTraceCausalProjectionLayerCell(node, zh)
	priority := runtimeTraceCausalProjectionPriorityCell(node, zh)
	if row.Kind != runtimeTraceProjTreeRowBackground {
		// background stanza header already states the layer; keep those rows lean
		tags = append(tags, runtimeTraceProjTag{Text: "‹" + layer + "›" + priority, DropOrder: runtimeTraceProjTagLayerChip})
	}
	if action := runtimeTraceCausalProjectionActionCell(node, zh); action != "" &&
		row.Kind != runtimeTraceProjTreeRowBackground {
		tags = append(tags, runtimeTraceProjTag{Text: action, DropOrder: runtimeTraceProjTagAction})
	}
	if node.CumulativeImpactMS > 0 && impact > 0 && node.CumulativeImpactMS != impact {
		text := fmt.Sprintf("链上累计%.3fms", node.CumulativeImpactMS)
		if !zh {
			text = fmt.Sprintf("chain cum %.3fms", node.CumulativeImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels.
	if node.DuplicatePublications > 1 {
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh), DropOrder: runtimeTraceProjTagExtra})
	}
	if node.MergedCount > 1 {
		var text string
		if runtimeTraceProjSubjectlessFoldRow(node) {
			// V3: the R3 cross-thread fold publishes the member MAX — say so
			// instead of letting the ×N read as the sum form.
			text = fmt.Sprintf("×%d合并·各%.3f–%.3fms(取最大值,不求和)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			if !zh {
				text = fmt.Sprintf("×%d merged · each %.3f–%.3fms (max shown, not summed)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		} else {
			text = fmt.Sprintf("×%d合并·单次%.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			if !zh {
				text = fmt.Sprintf("×%d merged · each %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if len(node.SecondaryObjects) > 0 {
		joined := strings.Join(node.SecondaryObjects, "/")
		text := "影响点 " + joined
		if !zh {
			text = "impact point " + joined
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if runtimeTraceProjEffectiveInherited(node) {
		text := fmt.Sprintf("有效归因%.3fms(承自等待区间,非本行实测)", node.EffectiveImpactMS)
		if !zh {
			text = fmt.Sprintf("attribution %.3fms (inherited from the wait interval, not this row)", node.EffectiveImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if runtimeTraceProjCrossWindow(node) {
		text := fmt.Sprintf("⚠跨窗(实际%.3fms)", node.ActualImpactMS)
		if !zh {
			text = fmt.Sprintf("⚠crosses window (actual %.3fms)", node.ActualImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep})
	}
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if parent != "" {
			text := "(span 位于 " + parent + " 内)"
			if !zh {
				text = "(span inside " + parent + ")"
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
		}
	}
	if node.Undrillable() {
		text := "⛔无匹配唤醒·链止"
		if !zh {
			text = "⛔no matching wakeup · chain ends"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep})
	}
	if row.EvidenceTag != "" {
		tags = append(tags, runtimeTraceProjTag{Text: "[" + row.EvidenceTag + "]", DropOrder: runtimeTraceProjTagKeep})
	}
	return b.String(), tags
}

// runtimeTraceProjEffectiveInherited flags the contradictory-number shape a
// real customer render exposed: an EffectiveImpactMS an order of magnitude
// above the row's own cumulative means the attribution was inherited from the
// enclosing wait interval, and MUST be annotated instead of shown bare.
func runtimeTraceProjEffectiveInherited(node types.TraceCausalProjectionNode) bool {
	return node.EffectiveImpactMS > 0 && node.CumulativeImpactMS > 0 &&
		node.EffectiveImpactMS > 10*node.CumulativeImpactMS
}

// runtimeTraceProjCrossWindow marks a node whose underlying state extends
// beyond its in-window projection (deterministic comparison; also honors the
// typed WithinRequestedWindow=false drill marker). The baseline is the LARGER
// of per-layer projection and chain total — an actual equal to the chain total
// does not cross anything (comparing actual against the smaller per-layer value
// alone over-flags dual-scope rows).
func runtimeTraceProjCrossWindow(node types.TraceCausalProjectionNode) bool {
	if node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow {
		return true
	}
	baseline := node.ImpactMS
	if node.CumulativeImpactMS > baseline {
		baseline = node.CumulativeImpactMS
	}
	return node.ActualImpactMS > 0 && baseline > 0 && node.ActualImpactMS > baseline*1.001
}

// --- lead text ----------------------------------------------------------------

func runtimeTraceProjLeadText(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, lang string, zh bool) string {
	var sections []string
	if line := runtimeTraceProjConclusionLine(projection, model, zh); line != "" {
		sections = append(sections, line)
	}
	sections = append(sections, runtimeTraceProjWindowLine(projection, model, zh))
	// Customer 2026-07-03: the reading note was one run-on paragraph and
	// unreadable — itemized, one short clause per line, deliberately plain
	// (no bold, no emoji beyond the glyphs it explains).
	if zh {
		sections = append(sections, strings.Join([]string{
			"树读法:",
			"- 自上而下 = 从关注线程向上游追溯。",
			"- `└─唤醒─` = 该行唤醒/依赖其父行。",
			"- 💤 = 症状非根因,其唤醒子行即下钻结果。",
			"- `├─成因─` = 同一线程的成因分解。",
			"- `⛔` = 窗口内无匹配 sched_wakeup,链止于此。",
			"- 时长条后的状态标签(睡眠等待/可运行等待/运行占用/IO阻塞/D状态)来自该行主导调度状态;无主导状态的行沿用影响形态。",
			"- 时长、排序与 E# 均可定位到原始 trace_query 结构化证据,不是额外推测。",
		}, "\n"))
	} else {
		sections = append(sections, strings.Join([]string{
			"Tree reading:",
			"- Top-down = tracing upstream from the focused thread.",
			"- `└─wakes─` = this row wakes/feeds its parent.",
			"- 💤 = a symptom, not a root cause; its wake child IS the drilldown result.",
			"- `├─cause─` = same-thread cause decomposition.",
			"- `⛔` = no matching sched_wakeup in the window; the chain ends there.",
			"- The state tag after each bar (sleep wait / runnable wait / running / IO wait / D-state) is the row's dominant scheduler state; rows without one keep their impact-shape value.",
			"- Durations, ranks and E# tags locate structured trace_query evidence — never extra speculation.",
		}, "\n"))
	}
	if len(model.Background) == 0 {
		if zh {
			sections = append(sections, "背景层: 当前结构化 trace_query 结果没有产出可承重的 off-chain/background 行;这不等于背景没有影响,只表示本轮证据没有给出可审计的背景统计。")
		} else {
			sections = append(sections, "Background layer: the structured trace_query result did not produce load-bearing off-chain/background rows. This does not prove there was no background influence; it only means this run lacks auditable background statistics.")
		}
	}
	return strings.Join(sections, "\n\n")
}

// runtimeTraceProjConclusionLine is FACT-ONLY: subject, cause, magnitudes and
// the typed drilldown target. It never emits advice/should-sentences — the
// system must not ghost-write the user-facing recommendation surface.
func runtimeTraceProjConclusionLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	primary := runtimeTraceProjLeadPrimary(projection, model.TrunkLen)
	if primary == nil {
		if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
			return ""
		}
		// Every primary candidate was demoted to the background stanza (§7.30
		// 裁定1) — the lead must say so instead of naming a demoted row as the
		// primary root cause and contradicting the tree below it.
		if zh {
			return "**主根因:** 窗口内未定位到链上主根因,见背景压力段。"
		}
		return "**Primary root cause:** no on-chain primary root cause was located in the window — see the background-pressure stanza."
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*primary, zh))
	// D4: the narrative lane uses the 中文（english_token） combined format on
	// the zh surface (tree rows stay concise zh; the table keeps raw tokens).
	cause := strings.TrimSpace(runtimeTraceCausalProjectionNarrativeCauseName(primary.Object, zh))
	if primary.IsAggregateMetric() {
		// The metric semantic name already carries the Object type word.
		cause = ""
	}
	ms := primary.CumulativeImpactMS
	if ms <= 0 {
		ms = runtimeTraceProjNodeDisplayImpact(*primary)
	}
	var b strings.Builder
	if zh {
		b.WriteString("**主根因:** ")
	} else {
		b.WriteString("**Primary root cause:** ")
	}
	b.WriteString(name)
	if cause != "" {
		b.WriteString(" " + cause)
	}
	if primary.MergedCount > 1 && primary.MergedMaxMS > 0 {
		// V1 (customer revisit 2026-07-03): a ×N aggregate's SUM never publishes
		// as the headline hard fact — show the per-instance max with the count;
		// the window share follows the same single-instance value.
		if zh {
			b.WriteString(fmt.Sprintf(" 单次最大 %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount))
			if model.WindowMS > 0 {
				b.WriteString(fmt.Sprintf("(占窗%.0f%%)", primary.MergedMaxMS/model.WindowMS*100))
			}
		} else {
			b.WriteString(fmt.Sprintf(" single max %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount))
			if model.WindowMS > 0 {
				b.WriteString(fmt.Sprintf(" (%.0f%% of window)", primary.MergedMaxMS/model.WindowMS*100))
			}
		}
	} else if ms > 0 {
		b.WriteString(fmt.Sprintf(" %.3fms", ms))
		if model.WindowMS > 0 {
			if zh {
				b.WriteString(fmt.Sprintf("(占窗%.0f%%)", ms/model.WindowMS*100))
			} else {
				b.WriteString(fmt.Sprintf(" (%.0f%% of window)", ms/model.WindowMS*100))
			}
		}
	}
	if target := strings.TrimSpace(primary.DrilldownTarget); target != "" && primary.IsSleepState() {
		if zh {
			b.WriteString(",下钻到 " + target)
		} else {
			b.WriteString(", drills down to " + target)
		}
	} else if primary.IsSleepState() && primary.Undrillable() {
		if zh {
			b.WriteString(",⛔窗口内无匹配唤醒、无法继续下钻")
		} else {
			b.WriteString(", ⛔ no matching wakeup in the window — cannot drill further")
		}
	}
	b.WriteString("。")
	if !zh {
		return strings.TrimSuffix(b.String(), "。") + "."
	}
	return b.String()
}

// runtimeTraceProjLeadPrimary picks the lead-line primary from the primary
// roots that SURVIVED the 裁定1 background demotion gate — a row rendered in
// the background stanza must never be named as the primary root cause. Nil when
// no primary exists or all of them were demoted.
//
// Selection order (V1, customer revisit 2026-07-03):
//  1. the engine's typed rank — the lowest positive Rank wins (the audit lane
//     already published rank=1; the conclusion must consume it instead of
//     re-ranking by displayed ms);
//  2. no ranked candidate: the largest single-instance effective attribution
//     (runtimeTraceProjLeadSelectionValue).
//
// A ×N aggregate's SUM (window projection total) never participates in the
// selection — same ruling family as S1 (排序合成分数不得以 ms 硬事实发布):
// the real customer conclusion named a ×7 hmfs_discard sum of 13.324ms over
// the engine's rank=1 running row of 4.115ms and contradicted its own body.
func runtimeTraceProjLeadPrimary(projection types.TraceCausalProjection, trunkLen int) *types.TraceCausalProjectionNode {
	roots := runtimeTraceCausalProjectionPrimaryRoots(projection)
	var ranked *types.TraceCausalProjectionNode
	for i := range roots {
		if runtimeTraceProjNodeDemotedToBackground(roots[i], trunkLen) {
			continue
		}
		if roots[i].Rank <= 0 {
			continue
		}
		// F4: rank TIES break on the single-instance selection value (per-instance
		// max for ×N aggregates), never on bucket order — the primary bucket sorts
		// by cumulative (= the R2 SUM), so "first row wins" re-admitted the merged
		// SUM through the very lane built to keep it out (a ×7 SUM of 13.324 beat
		// a single instance of 4.115 on a rank tie). Still-equal values keep the
		// earlier row (deterministic bucket order).
		if ranked == nil || roots[i].Rank < ranked.Rank ||
			(roots[i].Rank == ranked.Rank &&
				runtimeTraceProjLeadSelectionValue(roots[i]) > runtimeTraceProjLeadSelectionValue(*ranked)) {
			ranked = &roots[i]
		}
	}
	if ranked != nil {
		return ranked
	}
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	for i := range roots {
		if runtimeTraceProjNodeDemotedToBackground(roots[i], trunkLen) {
			continue
		}
		if v := runtimeTraceProjLeadSelectionValue(roots[i]); best == nil || v > bestValue {
			best, bestValue = &roots[i], v
		}
	}
	return best
}

// runtimeTraceProjLeadSelectionValue is the rank-fallback ordering key for the
// conclusion line: the single-instance effective attribution. A ×N aggregate
// contributes its per-instance max — the merged SUM is a window-projection
// total across instances and must never compete against single-instance hard
// facts (V1, customer revisit 2026-07-03).
func runtimeTraceProjLeadSelectionValue(node types.TraceCausalProjectionNode) float64 {
	if node.MergedCount > 1 {
		return node.MergedMaxMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return runtimeTraceProjNodeDisplayImpact(node)
}

func runtimeTraceProjWindowLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS <= 0 {
		if zh {
			return "关注窗口起止未采集: 不显示占窗百分比,树内时长条满格=本批最大投影(回退尺度,系统不估算窗口)。"
		}
		return "Window bounds not captured: no window percentages; tree bars scale to the batch max projection (fallback scale — the system never estimates a window)."
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "关注窗口 %.3fs → %.3fs,共 %.3fms。", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	} else {
		fmt.Fprintf(&b, "Requested window %.3fs → %.3fs, %.3fms total.", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	}
	// Coverage = depth-1 cumulative vs window, by SUBTRACTION only — chain
	// values overlap on the wall clock and must never be summed across layers.
	if attributed := runtimeTraceProjDepth1Cumulative(model); attributed > 0 {
		// V2 (customer revisit 2026-07-03): when the 🎯 target published its own
		// state rows, the coverage denominator is the TARGET SYMPTOM duration,
		// not the whole window — a target that slept 11.7ms of a 101ms window
		// once rendered as "残差 97%". Falls back to the whole window (wording
		// unchanged) when no self-state row exists or the attribution exceeds
		// the symptom duration.
		if symptom := runtimeTraceProjTargetSymptomMS(model); symptom > 0 && attributed <= symptom {
			residual := symptom - attributed
			if zh {
				fmt.Fprintf(&b, " 目标睡眠/阻塞 %.3fms 中 on-chain 已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)。",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			} else {
				fmt.Fprintf(&b, " Of the target's %.3fms sleep/blocked time, on-chain attributed %.3fms (%.0f%%), unattributed %.3fms (%.0f%%).",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			}
		} else if attributed <= model.WindowMS {
			residual := model.WindowMS - attributed
			if zh {
				fmt.Fprintf(&b, " on-chain 已归因 %.3fms/%.0f%%,未归因残差 %.3fms/%.0f%%。",
					attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
			} else {
				fmt.Fprintf(&b, " On-chain attributed %.3fms/%.0f%%, unattributed residual %.3fms/%.0f%%.",
					attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
			}
		} else {
			if zh {
				fmt.Fprintf(&b, " on-chain 已归因 %.3fms(其实际状态跨出窗口,见 ⚠ 标记)。", attributed)
			} else {
				fmt.Fprintf(&b, " On-chain attributed %.3fms (the underlying state crosses the window; see ⚠ marks).", attributed)
			}
		}
	}
	// R2 双窗关系行: when a user-requested window was derivable from the typed
	// entity pair and the projection window is a small sub-window of it (strict
	// numeric comparison: projection < 50% of the user window), say explicitly
	// how the two windows relate — the berlin customer saw a 101ms projection
	// with no mention of the 3.3s window they actually asked about.
	if model.UserWindowEnd > model.UserWindowStart && model.UserWindowStart > 0 {
		userMS := (model.UserWindowEnd - model.UserWindowStart) * 1000
		if model.WindowMS < userMS*0.5 {
			if zh {
				fmt.Fprintf(&b, "\n- 用户请求窗 %.3fs → %.3fs(共 %.1fs);本投影取其中代表性子窗,全窗指标见 Trace 指标快照",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			} else {
				fmt.Fprintf(&b, "\n- User-requested window %.3fs → %.3fs (%.1fs total); this projection covers a representative sub-window — full-window metrics live in the Trace Metric Snapshot",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			}
		}
	}
	return b.String()
}

// runtimeTraceProjTargetSymptomMS is the 🎯 target's own symptom duration: the
// sum of the target's self STATE-view rows' window projections (V2, customer
// revisit 2026-07-03). Summation is legal HERE only — these are one thread's
// own scheduler-state segments inside the window; cross-thread and cross-layer
// values still never add. 0 when the target exposed no qualifying self-state
// row (callers fall back to the whole window).
//
// F1 (adversarial review 2026-07-03): SelfRows deliberately mixes TWO typed
// views of the focused thread — its scheduler-STATE rows and hop-view rows
// (Role=causal_hop: critical_blocking / wakeup_causal_* / root_evidence:*)
// that re-describe wall clock already inside a state segment. Only state-view
// rows whose typed StateKind is in the sleep/D/blocked wait family enter the
// denominator: a 10ms sleep with an 8ms binder wait nested inside it is a
// 10ms symptom (never 18ms), and a running/runnable self row is not
// sleep/blocked time at all. Precise typed signals only (Role enum +
// StateKind enum), never prose.
func runtimeTraceProjTargetSymptomMS(model runtimeTraceProjTreeModel) float64 {
	total := 0.0
	for _, row := range model.SelfRows {
		if row.Node.Role == types.TraceCausalRoleCausalHop {
			continue // blocked-wait/attribution hop view: wall clock already counted by its enclosing state segment
		}
		if !runtimeTraceProjWaitFamilyStateKind(row.Node) {
			continue // running/runnable/stateless rows are not sleep/blocked symptom time
		}
		if row.Node.ImpactMS > 0 {
			total += row.Node.ImpactMS
		}
	}
	return total
}

// runtimeTraceProjWaitFamilyStateKind reports whether the node's typed dominant
// scheduler state belongs to the sleep/D/blocked wait family. Precise typed
// enum check (never a prose substring): running/runnable and rows WITHOUT a
// StateKind (e.g. metric aggregates that never exposed a dominant state) are
// NOT waits. Shared by the target-symptom denominator (F1) and the
// whole-window idle annotation (F3) so the two gates cannot drift.
func runtimeTraceProjWaitFamilyStateKind(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait",
		"d_state", "d_sleep", "uninterruptible_sleep", "io_wait":
		return true
	}
	return false
}

// runtimeTraceProjWholeWindowIdleRow is the SINGLE definition of the V3
// "整窗等待(疑似空闲)" annotation — the tree stanza tag and the detail-table
// mirror both call it (F3: two hand-synced copies were the drift risk). True
// only for a background row whose typed dominant state is in the sleep/D/
// blocked wait family AND whose projection covers ≥99% of the window without
// exceeding it (H8 tolerance: over-window cumulative rows are the multi-CPU
// ACTIVE shape, never idle). A whole-window running CPU hog or a stateless
// cpu·ms aggregate row that happens to ≈ the window never takes the tag.
func runtimeTraceProjWholeWindowIdleRow(row runtimeTraceProjTreeRow, windowMS float64) bool {
	if row.Kind != runtimeTraceProjTreeRowBackground || windowMS <= 0 {
		return false
	}
	if !runtimeTraceProjWaitFamilyStateKind(row.Node) {
		return false
	}
	impact := runtimeTraceProjNodeDisplayImpact(row.Node)
	return impact >= windowMS*0.99 && impact <= windowMS*1.001
}

func runtimeTraceProjDepth1Cumulative(model runtimeTraceProjTreeModel) float64 {
	if v := runtimeTraceProjChainDepthCumulative(model, 1); v > 0 {
		return v
	}
	// H10 fallback (berlin shape): every depth-1 trunk node was a bare transit
	// hop with no data row of its own, which silently dropped the whole
	// attributed/residual coverage line. Fall back to the SHALLOWEST depth that
	// carries a data-bearing chain row: its cumulative already contains every
	// deeper on-chain layer by wall-clock containment, so it is the tightest
	// available lower bound of on-chain attributed time. Max within ONE depth
	// only — values are never summed across layers (墙钟不可加和).
	minDepth := 0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || !row.HasData || row.Depth <= 1 {
			continue
		}
		if minDepth == 0 || row.Depth < minDepth {
			minDepth = row.Depth
		}
	}
	if minDepth == 0 {
		return 0
	}
	return runtimeTraceProjChainDepthCumulative(model, minDepth)
}

// runtimeTraceProjChainDepthCumulative returns the largest cumulative impact
// among data-bearing chain rows at exactly the given depth.
func runtimeTraceProjChainDepthCumulative(model runtimeTraceProjTreeModel, depth int) float64 {
	max := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || row.Depth != depth || !row.HasData {
			continue
		}
		v := row.Node.CumulativeImpactMS
		if v <= 0 {
			v = runtimeTraceProjNodeDisplayImpact(row.Node)
		}
		if v > max {
			max = v
		}
	}
	return max
}

// --- lossless detail table ------------------------------------------------------

func runtimeTraceProjDetailTable(model runtimeTraceProjTreeModel, zh bool) ([]string, []types.AnswerBlockItem) {
	// D2 audit fidelity: the 类型 column keeps the raw English type token that
	// the zh tree label replaced (both language surfaces carry the column).
	columns := []string{"层级", "因果位置·优先级", "节点/原因", "类型", "关系 ▸ 影响点", "影响形态", "窗口投影", "链上累计", "有效归因", "实际状态", "证据·置信"}
	if !zh {
		columns = []string{"Layer", "Causal position · priority", "Node / cause", "Type", "Relation ▸ impact point", "Impact shape", "Window projection", "Chain total", "Attribution", "Actual state", "Evidence · confidence"}
	}
	dash := "—"
	msCell := func(v float64) string {
		if v <= 0 {
			return dash
		}
		return fmt.Sprintf("%.3fms", v)
	}
	// Flat mode (no wakeup-path trunk): EVERY row is depthless, so per-row
	// "depth unresolved" / "impact point unresolved" placeholders carry zero
	// information and spam the table — the header already names the flat cause
	// (§7.30 裁定3) and the causal-position column already says on-chain. The
	// placeholders render only when a trunk exists and a named row cannot attach.
	flat := strings.TrimSpace(model.Target) == ""
	var rows []types.AnswerBlockItem
	addRow := func(row runtimeTraceProjTreeRow) {
		if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
			return
		}
		node := row.Node
		layer := runtimeTraceProjDetailLayerCell(row, zh, flat)
		position := runtimeTraceCausalProjectionLayerCell(node, zh) + " · " + runtimeTraceCausalProjectionPriorityCell(node, zh)
		name := runtimeTraceCausalProjectionNodeSubjectCell(node, zh)
		// RF2b/V4: the duplicate-publication fold (single measurement) and the
		// R2 sum aggregate are independent typed signals with distinct labels.
		if node.DuplicatePublications > 1 {
			name += " " + runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)
		}
		if node.MergedCount > 1 {
			if runtimeTraceProjSubjectlessFoldRow(node) {
				// V3: the cross-thread fold publishes the member MAX, and (V5)
				// the table cell keeps the same member roster the tree fold line
				// already shows — "对端线程未解析 ×6" lost every thread identity.
				if zh {
					name += fmt.Sprintf(" ×%d(各%.3f–%.3fms,取最大值)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				} else {
					name += fmt.Sprintf(" ×%d (each %.3f–%.3fms, max shown)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				}
				name += runtimeTraceProjMergedSubjectsSuffix(node, zh)
			} else {
				name += fmt.Sprintf(" ×%d(%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		}
		if node.Undrillable() {
			name += " ⛔"
		}
		relation := runtimeTraceProjDetailRelationCell(row, zh, flat)
		// §7.30.2 C4b: the typed R1-merge impact points (SecondaryObjects) must
		// stay lossless on the table — the tree's 影响点 tag is width-elidable.
		if len(node.SecondaryObjects) > 0 {
			joined := strings.Join(node.SecondaryObjects, "/")
			if zh {
				relation += " ▸ 影响点 " + joined
			} else {
				relation += " ▸ impact point " + joined
			}
		}
		shape := runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		if shape == "" {
			shape = dash
		}
		// V3: mirror the whole-window-wait annotation on the lossless surface —
		// F3: the SAME shared judgment as the stanza tag (typed wait-family
		// StateKind guard included; over-window cumulative rows are the H8
		// shape and never take it).
		if runtimeTraceProjWholeWindowIdleRow(row, model.WindowMS) {
			idle := "整窗等待(疑似空闲)"
			if !zh {
				idle = "whole-window wait (likely idle)"
			}
			if shape == dash {
				shape = idle
			} else {
				shape += "·" + idle
			}
		}
		effective := msCell(node.EffectiveImpactMS)
		if runtimeTraceProjEffectiveInherited(node) {
			if zh {
				effective += "(承自等待区间)"
			} else {
				effective += " (inherited)"
			}
		}
		actual := msCell(node.ActualImpactMS)
		if runtimeTraceProjCrossWindow(node) && node.ActualImpactMS > 0 {
			actual += " ⚠"
		}
		evidence := row.EvidenceTag
		if evidence == "" {
			evidence = dash
		}
		if tier := runtimeTraceProjConfidenceTier(node.Confidence, zh); tier != "" {
			evidence += " · " + tier
		}
		typeToken := runtimeTraceCausalProjectionRawTypeToken(node)
		if typeToken == "" {
			typeToken = dash
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				layer, position,
				runtimeTraceCausalProjectionMarkdownSafe(name),
				runtimeTraceCausalProjectionMarkdownSafe(typeToken),
				runtimeTraceCausalProjectionMarkdownSafe(relation),
				shape,
				msCell(node.ImpactMS), msCell(node.CumulativeImpactMS),
				effective, actual,
				evidence,
			},
			CitationRef: -1,
		})
	}
	for _, row := range model.SelfRows {
		addRow(row)
	}
	for _, row := range model.TreeRows {
		addRow(row)
	}
	for _, row := range model.Adjacent {
		addRow(row)
	}
	for _, row := range model.Background {
		addRow(row)
	}
	return columns, rows
}

func runtimeTraceProjDetailLayerCell(row runtimeTraceProjTreeRow, zh, flat bool) string {
	switch row.Kind {
	case runtimeTraceProjTreeRowSelf:
		if zh {
			return "目标状态"
		}
		return "target state"
	case runtimeTraceProjTreeRowAdjacent:
		if zh {
			return "◇ 邻近"
		}
		return "◇ adjacent"
	case runtimeTraceProjTreeRowBackground:
		if zh {
			return "▒ 背景"
		}
		return "▒ background"
	case runtimeTraceProjTreeRowDepthless:
		if flat {
			// No trunk exists, so "detached/unresolved" would describe every
			// single row — render the plain typed depth (or a dash) instead.
			if row.Depth > 0 {
				if zh {
					return fmt.Sprintf("深度%d", row.Depth)
				}
				return fmt.Sprintf("depth %d", row.Depth)
			}
			return "—"
		}
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("深度%d(未接入链)", row.Depth)
			}
			return fmt.Sprintf("depth %d (detached)", row.Depth)
		}
		if zh {
			return "链上·深度未解析"
		}
		return "on-chain · depth unresolved"
	case runtimeTraceProjTreeRowCause:
		if zh {
			return fmt.Sprintf("成因·深度%d", row.Depth)
		}
		return fmt.Sprintf("cause · depth %d", row.Depth)
	case runtimeTraceProjTreeRowSemantic:
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("语义·深度%d", row.Depth)
			}
			return fmt.Sprintf("span · depth %d", row.Depth)
		}
		if zh {
			return "语义span"
		}
		return "span"
	default:
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("深度%d", row.Depth)
			}
			return fmt.Sprintf("depth %d", row.Depth)
		}
		if zh {
			return "链上"
		}
		return "on-chain"
	}
}

func runtimeTraceProjDetailRelationCell(row runtimeTraceProjTreeRow, zh, flat bool) string {
	parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Parent, zh))
	switch row.Kind {
	case runtimeTraceProjTreeRowSelf:
		if zh {
			return "自身状态"
		}
		return "own state"
	case runtimeTraceProjTreeRowAdjacent:
		if zh {
			return "邻近支撑(无直接唤醒边)"
		}
		return "adjacent support (no direct wake edge)"
	case runtimeTraceProjTreeRowBackground:
		if zh {
			return "背景支撑"
		}
		return "background support"
	}
	if row.Kind == runtimeTraceProjTreeRowDepthless && parent == "" {
		if flat {
			// Flat mode: every row lacks an attach point by construction; the
			// per-row placeholder is pure spam next to the flat-cause header.
			return "—"
		}
		if zh {
			return "on-chain·影响点未解析"
		}
		return "on-chain · impact point unresolved"
	}
	label := ""
	switch row.Edge {
	case runtimeTraceProjTreeEdgeDrill:
		if zh {
			label = "下钻"
		} else {
			label = "drill"
		}
	case runtimeTraceProjTreeEdgeSemantic:
		if zh {
			label = "语义span"
		} else {
			label = "span"
		}
	case runtimeTraceProjTreeEdgeCause:
		if zh {
			label = "成因"
		} else {
			label = "cause"
		}
	default:
		if zh {
			label = "唤醒"
		} else {
			label = "wakes"
		}
	}
	if parent == "" {
		return label
	}
	return label + " ▸ " + parent
}

func runtimeTraceProjConfidenceTier(confidence float64, zh bool) string {
	switch {
	case confidence >= 0.85:
		if zh {
			return "高"
		}
		return "high"
	case confidence >= 0.6:
		if zh {
			return "中"
		}
		return "mid"
	case confidence > 0:
		if zh {
			return "低"
		}
		return "low"
	default:
		return ""
	}
}

// --- file-grouped evidence index ---------------------------------------------

// runtimeTraceProjEvidenceBlockParts renders the evidence roster grouped by
// artifact file: when every locator shares one file, the file name appears once
// in the block text and each entry keeps only its line-range suffix (a real
// customer render burned 29 lines repeating one truncated file name).
func runtimeTraceProjEvidenceBlockParts(evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) (string, []types.AnswerBlockItem) {
	if evidence == nil || len(evidence.order) == 0 {
		return "", nil
	}
	// H19: an entry whose ref is the naked line=N / lines=N-M form (its source
	// node carried no SupportRefs path) breaks locator uniformity ("lines=…"
	// next to "berlin.systrace:…"). When every path-carrying entry of THIS
	// roster agrees on exactly one artifact, adopt that artifact for the bare
	// entries (display copy only). Ambiguous multi-artifact rosters keep the
	// bare form rather than guessing an artifact.
	entries := append([]runtimeTraceCausalProjectionEvidenceEntry(nil), evidence.order...)
	if shared := runtimeTraceCausalProjectionSoleArtifactPath(entries); shared != "" {
		for i := range entries {
			if lineRange, ok := runtimeTraceCausalProjectionBareLineRef(entries[i].Ref); ok {
				entries[i].Ref = shared + ":" + lineRange
			}
		}
	}
	sharedFile := ""
	uniform := true
	for _, entry := range entries {
		pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref)
		if suffix == "" || strings.TrimSpace(pathPart) == "" {
			uniform = false
			break
		}
		if sharedFile == "" {
			sharedFile = pathPart
			continue
		}
		if pathPart != sharedFile {
			uniform = false
			break
		}
	}
	intro := runtimeTraceCausalProjectionEvidenceText(zh)
	if uniform && sharedFile != "" && len(entries) > 1 {
		if zh {
			intro += " 全部证据位于 `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`,各条只列行号区间。"
		} else {
			intro += " All locators live in `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`; entries list only line ranges."
		}
	}
	items := make([]types.AnswerBlockItem, 0, len(entries))
	for _, entry := range entries {
		locator := runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow(entry.Ref, entry.Window)
		if uniform && sharedFile != "" && len(entries) > 1 {
			// Grouped mode: the shared file name is stated once in the intro; each
			// entry keeps only its own window (preferred, 裁定6) or line range.
			if entry.Window != "" {
				locator = runtimeTraceCausalProjectionMarkdownSafe(entry.Window)
			} else if _, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref); suffix != "" {
				locator = runtimeTraceCausalProjectionMarkdownSafe(suffix)
			}
		}
		if locator == "" {
			locator = "trace_query"
		}
		audit := runtimeTraceCausalProjectionCompactCellText(entry.Details, 72)
		var text string
		if zh {
			text = fmt.Sprintf("定位: %s；审计: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "；完整定位见原始 trace_query 记录"
			}
		} else {
			text = fmt.Sprintf("locator: %s; audit: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "; full locator remains in the original trace_query record"
			}
		}
		items = append(items, types.AnswerBlockItem{
			ID:          strings.ToLower(entry.ID),
			Label:       entry.ID,
			Text:        text,
			CitationRef: -1,
		})
	}
	return intro, items
}
