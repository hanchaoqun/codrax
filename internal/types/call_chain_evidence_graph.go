package types

import (
	"sort"
	"strings"
)

// CallChainEvidenceEdge is a grounded, direction-preserving source call edge.
// It is deliberately presentation-neutral: consumers may show the edge to the
// model, but the edge does not author an answer conclusion.
type CallChainEvidenceEdge struct {
	From       string
	To         string
	EvidenceID string
	Source     string
	LineStart  int
	LineEnd    int
}

// CallChainEvidenceGraphAnalysis is the shared exact-endpoint graph result used
// by investigation closure and finalizer context. All paths preserve the
// direction of accepted ClaimCallEdge evidence. Empty fields mean unavailable,
// never an inferred relationship.
type CallChainEvidenceGraphAnalysis struct {
	StartResolved  bool
	EndResolved    bool
	StartAmbiguous bool
	EndAmbiguous   bool
	EdgeCount      int

	DirectedPath      []CallChainEvidenceEdge
	ReversePath       []CallChainEvidenceEdge
	SharedFrontier    string
	SourcePath        []CallChainEvidenceEdge
	SinkPath          []CallChainEvidenceEdge
	SourceFrontier    []CallChainEvidenceEdge
	RequestedBoundary []CallChainEvidenceEdge
}

type callChainGraphEdge struct {
	CallChainEvidenceEdge
	fromKey string
	toKey   string
}

// AnalyzeCallChainEvidenceGraph builds a language-neutral graph from citable
// current-source call edges. Definitions, recovered rows, runtime artifacts,
// prose, source order and prefix siblings cannot mint reachability.
func AnalyzeCallChainEvidenceGraph(evidence []EvidenceItem, startHint, endHint string) CallChainEvidenceGraphAnalysis {
	publicEdges := callChainEvidenceEdges(evidence)
	analysis := CallChainEvidenceGraphAnalysis{EdgeCount: len(publicEdges)}
	if len(publicEdges) == 0 {
		return analysis
	}
	canonical := newSharedCallChainCanonicalizer(publicEdges)
	edges := make([]callChainGraphEdge, 0, len(publicEdges))
	nodes := make(map[string]string)
	adjacency := make(map[string][]callChainGraphEdge)
	for _, edge := range publicEdges {
		from, to := canonical.key(edge.From), canonical.key(edge.To)
		if from == "" || to == "" {
			continue
		}
		graphEdge := callChainGraphEdge{CallChainEvidenceEdge: edge, fromKey: from, toKey: to}
		edges = append(edges, graphEdge)
		adjacency[from] = append(adjacency[from], graphEdge)
		if nodes[from] == "" {
			nodes[from] = edge.From
		}
		if nodes[to] == "" {
			nodes[to] = edge.To
		}
	}
	starts := canonical.resolveEndpoint(startHint, nodes)
	ends := canonical.resolveEndpoint(endHint, nodes)
	analysis.StartResolved = len(starts.candidates) > 0
	analysis.EndResolved = len(ends.candidates) > 0
	analysis.StartAmbiguous = starts.ambiguous
	analysis.EndAmbiguous = ends.ambiguous
	analysis.DirectedPath = callChainCoveredPath(adjacency, starts, ends)
	if len(analysis.DirectedPath) > 0 {
		return analysis
	}
	analysis.ReversePath = callChainCoveredPath(adjacency, ends, starts)
	if len(analysis.ReversePath) > 0 {
		return analysis
	}
	if len(starts.candidates) == 1 && len(ends.candidates) == 1 && !starts.ambiguous && !ends.ambiguous {
		analysis.SharedFrontier, analysis.SourcePath, analysis.SinkPath = callChainSharedDescendant(
			adjacency, nodes, starts.candidates[0], ends.candidates[0],
		)
	}
	analysis.SourceFrontier = callChainBoundedEdges(edges, func(edge callChainGraphEdge) bool {
		return callChainContainsKey(starts.candidates, edge.fromKey)
	}, 3)
	analysis.RequestedBoundary = callChainBoundedEdges(edges, func(edge callChainGraphEdge) bool {
		return callChainContainsKey(ends.candidates, edge.fromKey) || callChainContainsKey(ends.candidates, edge.toKey)
	}, 3)
	return analysis
}

func callChainEvidenceEdges(evidence []EvidenceItem) []CallChainEvidenceEdge {
	out := make([]CallChainEvidenceEdge, 0)
	seen := make(map[string]bool)
	for _, item := range evidence {
		if !item.IsCitable() || ClaimFormOf(item) != ClaimCallEdge ||
			RuntimeArtifactPathKind(item.Source) != "" ||
			!HasCodeOrConfigPathSuffix(strings.ToLower(item.Source)) {
			continue
		}
		// OwnerSymbol is parser-stamped after grounding and preserves the
		// receiver/package/module identity of the enclosing callable. Prefer it
		// over the model-authored short Subject so class methods and same-named
		// wrapper/core functions remain distinct graph nodes. Subject stays the
		// compatibility fallback for older or externally constructed evidence.
		from := strings.TrimSpace(item.OwnerSymbol)
		if from == "" {
			from = strings.TrimSpace(item.Subject)
		}
		to := strings.TrimSpace(item.Object)
		if to == "" {
			to = strings.TrimSpace(item.AnchorSymbol)
		}
		if from == "" || to == "" {
			continue
		}
		key := strings.ToLower(from) + "\x00" + strings.ToLower(to)
		if seen[key] {
			continue
		}
		seen[key] = true
		evidenceID := strings.TrimSpace(item.ID)
		if evidenceID == "" {
			evidenceID = strings.TrimSpace(item.EvidenceRef)
		}
		out = append(out, CallChainEvidenceEdge{
			From: from, To: to, EvidenceID: evidenceID,
			Source: strings.TrimSpace(item.Source), LineStart: item.LineStart, LineEnd: item.LineEnd,
		})
	}
	return out
}

type sharedCallChainCanonicalizer struct {
	qualifiedByTail map[string]map[string]bool
}

type sharedCallChainEndpointResolution struct {
	candidates []string
	ambiguous  bool
}

func newSharedCallChainCanonicalizer(edges []CallChainEvidenceEdge) sharedCallChainCanonicalizer {
	c := sharedCallChainCanonicalizer{qualifiedByTail: make(map[string]map[string]bool)}
	for _, edge := range edges {
		for _, raw := range []string{edge.From, edge.To} {
			identity := sharedCallChainIdentity(raw)
			if sharedCallChainQualifiedOwner(identity) == "" {
				continue
			}
			tail := strings.ToLower(sharedCallChainQualifiedOperation(identity))
			if tail == "" {
				continue
			}
			if c.qualifiedByTail[tail] == nil {
				c.qualifiedByTail[tail] = make(map[string]bool)
			}
			c.qualifiedByTail[tail][strings.ToLower(identity)] = true
		}
	}
	return c
}

func (c sharedCallChainCanonicalizer) key(raw string) string {
	identity := sharedCallChainIdentity(raw)
	if identity == "" {
		return ""
	}
	if sharedCallChainQualifiedOwner(identity) != "" {
		return strings.ToLower(identity)
	}
	tail := strings.ToLower(sharedCallChainQualifiedOperation(identity))
	qualified := c.qualifiedByTail[tail]
	if len(qualified) == 1 {
		for candidate := range qualified {
			return candidate
		}
	}
	return strings.ToLower(identity)
}

func (c sharedCallChainCanonicalizer) resolveEndpoint(endpoint string, nodes map[string]string) sharedCallChainEndpointResolution {
	want := strings.ToLower(sharedCallChainIdentity(endpoint))
	if want == "" {
		return sharedCallChainEndpointResolution{}
	}
	if _, ok := nodes[want]; ok {
		return sharedCallChainEndpointResolution{candidates: []string{want}}
	}
	if sharedCallChainQualifiedOwner(want) == "" {
		var owners []string
		for key := range nodes {
			owner := strings.ToLower(sharedCallChainQualifiedOwner(key))
			if owner == want || strings.ToLower(NormalizedSurfaceSymbolTail(owner)) == want {
				owners = append(owners, key)
			}
		}
		if len(owners) > 0 {
			sort.Strings(owners)
			return sharedCallChainEndpointResolution{candidates: owners}
		}
	}
	tail := strings.ToLower(sharedCallChainQualifiedOperation(want))
	var matches []string
	for key := range nodes {
		if strings.ToLower(sharedCallChainQualifiedOperation(key)) == tail {
			matches = append(matches, key)
		}
	}
	sort.Strings(matches)
	return sharedCallChainEndpointResolution{candidates: matches, ambiguous: len(matches) > 1}
}

func callChainCoveredPath(adjacency map[string][]callChainGraphEdge, starts, ends sharedCallChainEndpointResolution) []CallChainEvidenceEdge {
	if len(starts.candidates) == 0 || len(ends.candidates) == 0 {
		return nil
	}
	startCovered := make(map[string]bool)
	endCovered := make(map[string]bool)
	var first []CallChainEvidenceEdge
	for _, start := range starts.candidates {
		for _, end := range ends.candidates {
			path := callChainEvidencePathBetween(adjacency, start, end)
			if len(path) == 0 {
				continue
			}
			startCovered[start], endCovered[end] = true, true
			if len(first) == 0 {
				first = path
			}
		}
	}
	if (starts.ambiguous && len(startCovered) != len(starts.candidates)) ||
		(ends.ambiguous && len(endCovered) != len(ends.candidates)) {
		return nil
	}
	return first
}

func callChainEvidencePathBetween(adjacency map[string][]callChainGraphEdge, start, end string) []CallChainEvidenceEdge {
	queue := []string{start}
	seen := map[string]bool{start: true}
	parent := make(map[string]callChainGraphEdge)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == end {
			var reversed []CallChainEvidenceEdge
			for node := current; node != start; {
				edge, ok := parent[node]
				if !ok {
					return nil
				}
				reversed = append(reversed, edge.CallChainEvidenceEdge)
				node = edge.fromKey
			}
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			return reversed
		}
		for _, edge := range adjacency[current] {
			if seen[edge.toKey] {
				continue
			}
			seen[edge.toKey] = true
			parent[edge.toKey] = edge
			queue = append(queue, edge.toKey)
		}
	}
	return nil
}

func callChainSharedDescendant(adjacency map[string][]callChainGraphEdge, nodes map[string]string, start, end string) (string, []CallChainEvidenceEdge, []CallChainEvidenceEdge) {
	startDistance := callChainGraphDistances(adjacency, start)
	endDistance := callChainGraphDistances(adjacency, end)
	best, bestDistance := "", int(^uint(0)>>1)
	for node, left := range startDistance {
		right, ok := endDistance[node]
		if !ok || node == start || node == end {
			continue
		}
		if distance := left + right; distance < bestDistance || (distance == bestDistance && node < best) {
			best, bestDistance = node, distance
		}
	}
	if best == "" {
		return "", nil, nil
	}
	return nodes[best], callChainEvidencePathBetween(adjacency, start, best), callChainEvidencePathBetween(adjacency, end, best)
}

func callChainGraphDistances(adjacency map[string][]callChainGraphEdge, start string) map[string]int {
	distance := map[string]int{start: 0}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range adjacency[current] {
			if _, seen := distance[edge.toKey]; seen {
				continue
			}
			distance[edge.toKey] = distance[current] + 1
			queue = append(queue, edge.toKey)
		}
	}
	return distance
}

func callChainBoundedEdges(edges []callChainGraphEdge, match func(callChainGraphEdge) bool, limit int) []CallChainEvidenceEdge {
	var out []CallChainEvidenceEdge
	for _, edge := range edges {
		if !match(edge) {
			continue
		}
		out = append(out, edge.CallChainEvidenceEdge)
		if len(out) == limit {
			break
		}
	}
	return out
}

func callChainContainsKey(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sharedCallChainIdentity(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), "`'\"")
	raw = strings.TrimSuffix(raw, "()")
	raw = strings.ReplaceAll(raw, "::", ".")
	raw = strings.ReplaceAll(raw, "#", ".")
	return strings.TrimSpace(raw)
}

func sharedCallChainQualifiedOwner(symbol string) string {
	symbol, cut, width := sharedCallChainQualifiedSplit(symbol)
	if cut <= 0 || cut+width >= len(symbol) {
		return ""
	}
	return strings.TrimSpace(symbol[:cut])
}

func sharedCallChainQualifiedOperation(symbol string) string {
	symbol, cut, width := sharedCallChainQualifiedSplit(symbol)
	if cut < 0 {
		return symbol
	}
	if cut+width >= len(symbol) {
		return ""
	}
	return strings.TrimSpace(symbol[cut+width:])
}

func sharedCallChainQualifiedSplit(symbol string) (string, int, int) {
	symbol = strings.TrimSpace(symbol)
	cut, width := -1, 0
	for _, separator := range []string{"::", ".", "#"} {
		if idx := strings.LastIndex(symbol, separator); idx > cut {
			cut, width = idx, len(separator)
		}
	}
	return symbol, cut, width
}
