package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// callChainReachability is compiled only from typed endpoint obligations and
// accepted citable call-edge evidence. It never reads the raw request, model
// prose, diagram labels, or rendered answer text.
type callChainReachability struct {
	Source string
	Target string
	Proven bool
}

// compileCallChainReachability deliberately handles only the unambiguous
// exactly-two-endpoint shape. Mentioned context symbols in wider requests do
// not have source/sink roles today, so guessing first/last there would turn a
// noisy order into a hard semantic decision.
func compileCallChainReachability(view *types.AnswerSemanticView, evidence []types.EvidenceItem) (callChainReachability, bool) {
	if view == nil || view.Family != types.QFCallChain || len(view.RequiredMechanismAnchors) != 2 {
		return callChainReachability{}, false
	}
	source := strings.TrimSpace(view.RequiredMechanismAnchors[0].Text)
	target := strings.TrimSpace(view.RequiredMechanismAnchors[1].Text)
	if source == "" || target == "" {
		return callChainReachability{}, false
	}
	result := callChainReachability{Source: source, Target: target}
	sourceKey := callChainReachabilityKey(source)
	targetKey := callChainReachabilityKey(target)
	if sourceKey == "" || targetKey == "" {
		return callChainReachability{}, false
	}
	if sourceKey == targetKey {
		result.Proven = true
		return result, true
	}

	adj := make(map[string][]string)
	for _, ev := range evidence {
		if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		from := callChainReachabilityKey(ev.Subject)
		if from == "" {
			continue
		}
		// Object is the exact fully-qualified callee surface and
		// AnchorSymbol is the exact callee token from the same grounded call
		// record. They are aliases authorized by that record, not fuzzy
		// prefix/suffix matches.
		for _, rawTo := range []string{ev.Object, ev.AnchorSymbol} {
			to := callChainReachabilityKey(rawTo)
			if to == "" || to == from {
				continue
			}
			adj[from] = appendUniqueCallChainReachabilityNode(adj[from], to)
		}
	}

	seen := map[string]bool{sourceKey: true}
	queue := []string{sourceKey}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adj[current] {
			if next == targetKey {
				result.Proven = true
				return result, true
			}
			if seen[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return result, true
}

func callChainReachabilityKey(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "()")
	text = strings.TrimSpace(text)
	return strings.ToLower(text)
}

func appendUniqueCallChainReachabilityNode(nodes []string, node string) []string {
	for _, existing := range nodes {
		if existing == node {
			return nodes
		}
	}
	return append(nodes, node)
}

// normalizeCallChainReachabilityAuthority prevents endpoint presence from
// being rendered as endpoint reachability. When the typed graph does not prove
// source→sink, it replaces only the typed summary/principal-path carriers with
// an honest boundary. Verified diagram edges and auxiliary inventories remain
// intact. QFRootCauseTrace (including explicit time-window causal projection)
// never enters this normalizer.
func normalizeCallChainReachabilityAuthority(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext, evidence []types.EvidenceItem) int {
	if doc == nil {
		return 0
	}
	reachability, active := compileCallChainReachability(view, evidence)
	if !active || reachability.Proven {
		return 0
	}
	zh := answerDocumentRequiresChinese(requestedAnswerDocumentLanguage(ctx))
	summary, title, pathText := callChainUnprovenReachabilityText(reachability, zh)
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockSummary {
			continue
		}
		if block.Text != summary || len(block.Items) != 0 || len(block.ClaimUses) != 0 {
			block.Text = summary
			block.Items = nil
			block.ClaimUses = nil
			fixed++
		}
		break
	}

	pathBlockFound := false
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if !containsBlockFacet(*block, types.FacetPrincipalPathEdge) || block.Kind == types.BlockDiagram {
			continue
		}
		pathBlockFound = true
		if normalizeCallChainUnprovenPathBlock(block, reachability, title, pathText, zh) {
			fixed++
		}
	}
	if !pathBlockFound && len(doc.Blocks) < maxBlocksPerDoc {
		block := types.AnswerBlock{
			ID:   uniqueCallChainReachabilityBlockID(doc),
			Kind: types.BlockOrderedList,
		}
		normalizeCallChainUnprovenPathBlock(&block, reachability, title, pathText, zh)
		doc.Blocks = append(doc.Blocks, block)
		fixed++
	}
	return fixed
}

func callChainUnprovenReachabilityText(reachability callChainReachability, zh bool) (summary, title, pathText string) {
	if zh {
		return fmt.Sprintf("本轮已接受的 typed 调用边未证明 `%s` 到 `%s` 的有向调用路径；两个端点都出现，并不等于二者可达。下方图仅保留逐边已验证的关系。", reachability.Source, reachability.Target),
			"调用链可达性判定",
			"未证明起点到目标端点的有向路径。这里只保留请求端点身份；已验证的局部调用边见图和证据清单，不能拼接成完整链。"
	}
	return fmt.Sprintf("Accepted typed call-edge evidence did not prove a directed path from `%s` to `%s`; the presence of both endpoints does not establish reachability. The diagram below retains only individually verified edges.", reachability.Source, reachability.Target),
		"Call-chain reachability",
		"No directed path from the source to the requested target was proven. This carrier preserves endpoint identity only; verified local edges in the diagram and evidence inventory must not be concatenated into a complete chain."
}

func normalizeCallChainUnprovenPathBlock(block *types.AnswerBlock, reachability callChainReachability, title, text string, zh bool) bool {
	if block == nil {
		return false
	}
	sourceText := "source endpoint"
	targetText := "target endpoint; directed reachability unproven"
	if zh {
		sourceText = "请求的起点端点"
		targetText = "请求的目标端点；有向可达性未获证明"
	}
	wantItems := []types.AnswerBlockItem{
		{ID: "source_endpoint", Label: reachability.Source, Text: sourceText, CitationRef: -1},
		{ID: "target_endpoint", Label: reachability.Target, Text: targetText, CitationRef: -1},
	}
	block.Kind = types.BlockOrderedList
	block.Title = title
	block.Text = text
	block.Items = wantItems
	block.SurfaceRole = types.SurfacePrincipal
	block.FacetIDs = []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)}
	block.ClaimUses = nil
	block.EdgeAnchors = nil
	return true
}

func uniqueCallChainReachabilityBlockID(doc *types.AnswerDocumentV2) string {
	const base = "call_chain_reachability"
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		used[strings.TrimSpace(block.ID)] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}
