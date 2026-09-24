package tracequery

import "sort"

// Union before subtraction prevents duplicate/overlapping direct children
// from being double charged. Grandchildren never get subtracted a second time.
func traceMarkerTreeSubtractChildren(whole TimeWindow, children []TimeWindow) []TimeWindow {
	if whole.EndTs <= whole.StartTs {
		return nil
	}
	sort.Slice(children, func(i, j int) bool { return children[i].StartTs < children[j].StartTs })
	cursor := whole.StartTs
	var self []TimeWindow
	for _, child := range children {
		start, end := maxFloat(child.StartTs, whole.StartTs), minFloat(child.EndTs, whole.EndTs)
		if end <= start || end <= cursor {
			continue
		}
		if start > cursor {
			self = append(self, TimeWindow{StartTs: cursor, EndTs: start})
		}
		cursor = end
	}
	if cursor < whole.EndTs {
		self = append(self, TimeWindow{StartTs: cursor, EndTs: whole.EndTs})
	}
	return self
}

// Each lane is sorted, disjoint and already validated against the other
// exclusive lanes. Prefix sums permit arbitrary nested marker and self-set
// queries without rescanning the owner's full scheduler timeline per node.
type traceMarkerTreeIntegral struct {
	spans  []TimeWindow
	prefix []float64
}

func (p *traceMarkerTreeIntegral) append(start, end float64) {
	p.spans = append(p.spans, TimeWindow{StartTs: start, EndTs: end})
	total := end - start
	if len(p.prefix) > 0 {
		total += p.prefix[len(p.prefix)-1]
	}
	p.prefix = append(p.prefix, total)
}

func (p *traceMarkerTreeIntegral) at(ts float64) float64 {
	i := sort.Search(len(p.spans), func(i int) bool { return p.spans[i].EndTs > ts })
	total := 0.0
	if i > 0 {
		total = p.prefix[i-1]
	}
	if i < len(p.spans) && ts > p.spans[i].StartTs {
		total += ts - p.spans[i].StartTs
	}
	return total
}

func (p *traceMarkerTreeIntegral) sum(segments []TimeWindow) float64 {
	total := 0.0
	for _, segment := range segments {
		total += p.at(segment.EndTs) - p.at(segment.StartTs)
	}
	return total * 1000
}
