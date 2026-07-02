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
	// runtimeTraceProjTreeTrunkMaxNodes bounds a long trunk display: deeper
	// middles compress into one omitted marker row (counts + cycle note kept).
	runtimeTraceProjTreeTrunkMaxNodes = 8
	runtimeTraceProjTreeBarWidth      = 10
)

type runtimeTraceProjTreeRow struct {
	Node      types.TraceCausalProjectionNode
	Kind      string
	Edge      string
	Depth     int
	Indent    int
	Last      bool
	Ancestors []bool // per ancestor level: more siblings follow (renders │)
	Parent    string // display name of the parent node (typed 影响点)
	HasData   bool   // false = bare path transit node without its own row
	Omitted   int
	CyclePeriod int
	CycleCount  int
	EvidenceTag string
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
			Parent: parentName, HasData: hasData,
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

	for _, node := range projection.AdjacentCauses {
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
	// Header: target anchor + explicit bar-scale declaration.
	if strings.TrimSpace(model.Target) != "" {
		header := "🎯 " + model.Target
		if zh {
			header += " ‹用户关注线程›"
		} else {
			header += " <user-focused thread>"
		}
		b.WriteString(runtimeTraceProjPadDisplay(header, runtimeTraceProjTreeLabelWidth))
		b.WriteString(" ")
		b.WriteString(runtimeTraceProjScaleNote(model, zh))
		b.WriteString("\n")
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
		b.WriteString(runtimeTraceProjTreeRowLine(row, denom, windowMode, zh))
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
			b.WriteString(runtimeTraceProjStanzaRowLine(row, denom, windowMode, zh))
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
			b.WriteString(runtimeTraceProjStanzaRowLine(row, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	b.WriteString("```")
	return b.String()
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
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.Object, zh))
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
	if node.MergedCount > 1 {
		if zh {
			parts = append(parts, fmt.Sprintf("×%d合并(单次%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		} else {
			parts = append(parts, fmt.Sprintf("×%d merged (each %.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
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
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.Object, zh))
	if row.Kind == runtimeTraceProjTreeRowCause {
		// Same-subject cause decomposition: the subject is already the parent
		// trunk row; show only the cause word.
		if object != "" {
			return object
		}
		return subject
	}
	if node.MergedCount > 1 && node.Subject == "" {
		if zh {
			return fmt.Sprintf("其余 %d 项合并", node.MergedCount)
		}
		return fmt.Sprintf("%d more folded", node.MergedCount)
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

func runtimeTraceProjTreeRowLine(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
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
	node := row.Node
	edge := runtimeTraceProjEdgeLabel(row.Edge, zh)
	if row.Kind == runtimeTraceProjTreeRowDepthless && strings.TrimSpace(row.Parent) == "" {
		// Flat fallback (no resolved target): a hanging "wakes" edge word would
		// claim a wake relation with no wakee — render a bare branch instead.
		edge = ""
	}
	label := runtimeTraceProjTreePrefix(row) + edge + " " +
		runtimeTraceProjStateIcon(node, row.Kind) + " " + runtimeTraceProjRowName(row, zh)
	left := runtimeTraceProjPadDisplay(label, runtimeTraceProjTreeLabelWidth)
	if !row.HasData {
		if zh {
			return left + " (链路中转,本轮无独立影响行)"
		}
		return left + " (chain transit, no standalone impact row this run)"
	}
	return left + " " + runtimeTraceProjRowMetrics(row, denom, windowMode, zh)
}

func runtimeTraceProjStanzaRowLine(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	label := "    " + runtimeTraceProjRowName(row, zh)
	left := runtimeTraceProjPadDisplay(label, runtimeTraceProjTreeLabelWidth)
	return left + " " + runtimeTraceProjRowMetrics(row, denom, windowMode, zh)
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

func runtimeTraceProjRowMetrics(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	node := row.Node
	impact := runtimeTraceProjNodeDisplayImpact(node)
	var b strings.Builder
	b.WriteString(runtimeTraceProjBar(impact, denom, row.Kind == runtimeTraceProjTreeRowBackground))
	b.WriteString(fmt.Sprintf(" %9.3fms", impact))
	if windowMode && denom > 0 && impact > 0 {
		b.WriteString(fmt.Sprintf(" %3.0f%%", impact/denom*100))
	}
	var tags []string
	// 裁定4: every bar row states WHAT the duration was (typed StateKind label;
	// impact-shape value when no state was exposed — never fabricated).
	stateTag := runtimeTraceProjStateKindLabel(node, zh)
	if stateTag == "" {
		stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
	}
	if stateTag != "" {
		tags = append(tags, stateTag)
	}
	layer := runtimeTraceCausalProjectionLayerCell(node, zh)
	priority := runtimeTraceCausalProjectionPriorityCell(node, zh)
	if row.Kind != runtimeTraceProjTreeRowBackground {
		// background stanza header already states the layer; keep those rows lean
		tags = append(tags, "‹"+layer+"›"+priority)
	}
	if action := runtimeTraceCausalProjectionActionCell(node, zh); action != "" &&
		row.Kind != runtimeTraceProjTreeRowBackground {
		tags = append(tags, action)
	}
	if node.CumulativeImpactMS > 0 && impact > 0 && node.CumulativeImpactMS != impact {
		if zh {
			tags = append(tags, fmt.Sprintf("链上累计%.3fms", node.CumulativeImpactMS))
		} else {
			tags = append(tags, fmt.Sprintf("chain cum %.3fms", node.CumulativeImpactMS))
		}
	}
	if node.MergedCount > 1 {
		if zh {
			tags = append(tags, fmt.Sprintf("×%d合并·单次%.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		} else {
			tags = append(tags, fmt.Sprintf("×%d merged · each %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		}
	}
	if len(node.SecondaryObjects) > 0 {
		joined := strings.Join(node.SecondaryObjects, "/")
		if zh {
			tags = append(tags, "影响点 "+joined)
		} else {
			tags = append(tags, "impact point "+joined)
		}
	}
	if runtimeTraceProjEffectiveInherited(node) {
		if zh {
			tags = append(tags, fmt.Sprintf("有效归因%.3fms(承自等待区间,非本行实测)", node.EffectiveImpactMS))
		} else {
			tags = append(tags, fmt.Sprintf("attribution %.3fms (inherited from the wait interval, not this row)", node.EffectiveImpactMS))
		}
	}
	if runtimeTraceProjCrossWindow(node) {
		if zh {
			tags = append(tags, fmt.Sprintf("⚠跨窗(实际%.3fms)", node.ActualImpactMS))
		} else {
			tags = append(tags, fmt.Sprintf("⚠crosses window (actual %.3fms)", node.ActualImpactMS))
		}
	}
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if parent != "" {
			if zh {
				tags = append(tags, "(span 位于 "+parent+" 内)")
			} else {
				tags = append(tags, "(span inside "+parent+")")
			}
		}
	}
	if node.Undrillable() {
		if zh {
			tags = append(tags, "⛔无匹配唤醒·链止")
		} else {
			tags = append(tags, "⛔no matching wakeup · chain ends")
		}
	}
	if row.EvidenceTag != "" {
		tags = append(tags, "["+row.EvidenceTag+"]")
	}
	if len(tags) > 0 {
		b.WriteString("  ")
		b.WriteString(strings.Join(tags, " · "))
	}
	return b.String()
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
	if zh {
		sections = append(sections, "树读法: 自上而下=从关注线程向上游追溯;`└─唤醒─`=该行唤醒/依赖其父行;💤 是症状非根因,其唤醒子行即下钻结果;`├─成因─`=同一线程的成因分解;`⛔`=窗口内无匹配 sched_wakeup,链止于此。时长条后的状态标签(睡眠等待/可运行等待/运行占用/IO阻塞/D状态)来自该行主导调度状态,无主导状态的行沿用影响形态。时长、排序与 E# 均可定位到原始 trace_query 结构化证据,不是额外推测。")
	} else {
		sections = append(sections, "Tree reading: top-down = tracing upstream from the focused thread; `└─wakes─` = this row wakes/feeds its parent; 💤 is a symptom (its wake child IS the drilldown result); `├─cause─` = same-thread cause decomposition; `⛔` = no matching sched_wakeup in the window, the chain ends there. The state tag after each bar (sleep wait / runnable wait / running / IO wait / D-state) is the row's dominant scheduler state; rows without one keep their impact-shape value. Durations, ranks and E# tags locate structured trace_query evidence — never extra speculation.")
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
			return "主根因: 窗口内未定位到链上主根因,见背景压力段。"
		}
		return "Primary root cause: no on-chain primary root cause was located in the window — see the background-pressure stanza."
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*primary, zh))
	cause := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(primary.Object, zh))
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
		b.WriteString("主根因: ")
	} else {
		b.WriteString("Primary root cause: ")
	}
	b.WriteString(name)
	if cause != "" {
		b.WriteString(" " + cause)
	}
	if ms > 0 {
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
func runtimeTraceProjLeadPrimary(projection types.TraceCausalProjection, trunkLen int) *types.TraceCausalProjectionNode {
	roots := runtimeTraceCausalProjectionPrimaryRoots(projection)
	for i := range roots {
		if runtimeTraceProjNodeDemotedToBackground(roots[i], trunkLen) {
			continue
		}
		return &roots[i]
	}
	return nil
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
		if attributed <= model.WindowMS {
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
	return b.String()
}

func runtimeTraceProjDepth1Cumulative(model runtimeTraceProjTreeModel) float64 {
	max := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || row.Depth != 1 || !row.HasData {
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
	columns := []string{"层级", "因果位置·优先级", "节点/原因", "关系 ▸ 影响点", "影响形态", "窗口投影", "链上累计", "有效归因", "实际状态", "证据·置信"}
	if !zh {
		columns = []string{"Layer", "Causal position · priority", "Node / cause", "Relation ▸ impact point", "Impact shape", "Window projection", "Chain total", "Attribution", "Actual state", "Evidence · confidence"}
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
		if node.MergedCount > 1 {
			suffix := fmt.Sprintf(" ×%d(%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			name += suffix
		}
		if node.Undrillable() {
			name += " ⛔"
		}
		relation := runtimeTraceProjDetailRelationCell(row, zh, flat)
		shape := runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		if shape == "" {
			shape = dash
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
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				layer, position,
				runtimeTraceCausalProjectionMarkdownSafe(name),
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
	sharedFile := ""
	uniform := true
	for _, entry := range evidence.order {
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
	if uniform && sharedFile != "" && len(evidence.order) > 1 {
		if zh {
			intro += " 全部证据位于 `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`,各条只列行号区间。"
		} else {
			intro += " All locators live in `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`; entries list only line ranges."
		}
	}
	items := make([]types.AnswerBlockItem, 0, len(evidence.order))
	for _, entry := range evidence.order {
		locator := runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow(entry.Ref, entry.Window)
		if uniform && sharedFile != "" && len(evidence.order) > 1 {
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
