package tracequery

// Inventory scope is independent of display priority. A numeric thread
// selector does not filter the all-emitter tree, nor does a displayed prefix
// enumerate that thread. Nil metadata is an older/unknown census, not zero.
type TraceMarkerTreePopulation struct {
	Scope          string                           `json:"scope"`
	DisplayedNodes int                              `json:"displayed_nodes"`
	Target         *TraceMarkerTreeTargetPopulation `json:"target,omitempty"`
}

type TraceMarkerTreeTargetPopulation struct {
	TID            int `json:"tid"`
	NodeCount      int `json:"node_count"`
	DisplayedNodes int `json:"displayed_nodes"`
	OmittedNodes   int `json:"omitted_nodes"`
}

const TraceMarkerTreePopulationAllEmitters = "all_observed_emitters"

// Counts are measured before truncation. Open/unclosed inventory context
// retains the existing visibility rule; it is not established active work or
// a closed-span count. No target is guessed from a name, TGID or marker PID.
func traceMarkerTreePopulation(nodes []TraceMarkerTreeNode, targetTID, displayLimit int) *TraceMarkerTreePopulation {
	p := &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: min(displayLimit, len(nodes))}
	if targetTID <= 0 {
		return p
	}
	p.Target = &TraceMarkerTreeTargetPopulation{TID: targetTID}
	for i, node := range nodes {
		if node.Thread.PID == targetTID {
			p.Target.NodeCount++
			if i < p.DisplayedNodes {
				p.Target.DisplayedNodes++
			}
		}
	}
	p.Target.OmittedNodes = p.Target.NodeCount - p.Target.DisplayedNodes
	return p
}

// Validate only precise census arithmetic. This metadata carries no query
// completion, member-set, thread-generation or causal qualification.
func (p *TraceMarkerTreePopulation) ValidCounts(total, omitted int) bool {
	if p == nil {
		return true // compatibility: never manufacture a missing target count
	}
	if p.Scope != TraceMarkerTreePopulationAllEmitters || total < 0 || omitted < 0 || omitted > total || p.DisplayedNodes != total-omitted {
		return false
	}
	t := p.Target
	return t == nil || t.TID > 0 && t.NodeCount >= 0 && t.NodeCount <= total && t.DisplayedNodes >= 0 &&
		t.DisplayedNodes <= p.DisplayedNodes && t.DisplayedNodes <= t.NodeCount && t.OmittedNodes >= 0 &&
		t.OmittedNodes <= omitted && t.OmittedNodes == t.NodeCount-t.DisplayedNodes &&
		total-t.NodeCount >= p.DisplayedNodes-t.DisplayedNodes
}
