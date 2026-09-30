package tracequery

import "sort"

func processProfileBusinessHotspots(idx *Index, q Query, members []ThreadRef) map[int][]ProcessBusinessHotspot {
	groups := map[int]map[string]*ProcessBusinessHotspot{}
	for _, member := range members {
		if threadIncarnationConflictForQuery(idx, q, member.PID) == nil {
			groups[member.PID] = map[string]*ProcessBusinessHotspot{}
		}
	}
	// Clear source-TID selectors; membership is applied to the full admitted
	// inventory, never the already truncated top-span display.
	q.PID, q.Thread, q.ThreadInput, q.TargetScope = 0, "", "", TargetScopeThread
	_, spans, _, _ := computeTraceMarksWithInventory(idx, q, 1)
	for _, span := range spans {
		if q.runCancel.tick() {
			break
		}
		byName := groups[span.Thread.PID]
		if byName == nil || span.Kind != "sync" || span.DurationMs <= 0 {
			continue
		}
		h := byName[span.Name]
		if h == nil {
			h = &ProcessBusinessHotspot{Name: span.Name, LineStart: span.StartLine, LineEnd: span.EndLine}
			byName[span.Name] = h
		}
		h.InstanceCount++
		h.InclusiveMs += span.DurationMs
		if span.DurationMs > h.MaxInstanceMs {
			h.MaxInstanceMs = span.DurationMs
		}
		if span.StartLine < h.LineStart {
			h.LineStart = span.StartLine
		}
		if span.EndLine > h.LineEnd {
			h.LineEnd = span.EndLine
		}
	}
	rows := map[int][]ProcessBusinessHotspot{}
	for pid, byName := range groups {
		for _, h := range byName {
			rows[pid] = append(rows[pid], *h)
		}
		sort.Slice(rows[pid], func(i, j int) bool {
			a, b := rows[pid][i], rows[pid][j]
			if a.InclusiveMs != b.InclusiveMs {
				return a.InclusiveMs > b.InclusiveMs
			}
			return a.Name < b.Name
		})
	}
	return rows
}
