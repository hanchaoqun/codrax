package tracequery

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

type traceMarkerTreeEntry struct {
	node     TraceMarkerTreeNode
	children []int
	resetTs  *float64
}

// The collector observes the existing validated B/E state machine. It never
// reparses bounded marker text or infers nesting from temporal containment.
type traceMarkerTreeCollector struct {
	entries       []traceMarkerTreeEntry
	byID          map[string]int
	unknownOwners map[string]bool
	lastLine      map[string]int
	unknownPrefix bool
	invalid       bool
	orderConflict bool
}

func newTraceMarkerTreeCollector(idx *Index) *traceMarkerTreeCollector {
	return &traceMarkerTreeCollector{byID: map[string]int{}, unknownOwners: map[string]bool{}, lastLine: map[string]int{}, unknownPrefix: idx.Windowed || idx.RelationScoped}
}

// Composite indexes sort by timestamp. Even independently valid closed
// cohorts can physically roll back between cohorts, so their sorted order
// must not manufacture a nested stack. This proof is tree-only and does not
// modify existing span or causal lanes.
func (c *traceMarkerTreeCollector) observeOrder(source string, pid, line int) {
	if c == nil {
		return
	}
	owner := traceMarkSyncPairingKey(source, pid)
	if previous := c.lastLine[owner]; previous > 0 && line <= previous {
		c.invalid = true
		c.orderConflict = true
	}
	c.lastLine[owner] = line
}

func (c *traceMarkerTreeCollector) failureCaveats() []string {
	if c != nil && c.orderConflict {
		return []string{"business_tree_unavailable=true; physical_marker_order_conflict; timestamp sorting changed the physical order of one source/emitter stack, so no business parent edges or self costs are published"}
	}
	return nil
}

func traceMarkerTreeID(source string, tid, line int) string {
	return fmt.Sprintf("marker:%x:%d:%d", sha256.Sum256([]byte(source)), tid, line)
}

func (c *traceMarkerTreeCollector) begin(source string, ev Event, stack []Event) {
	if c == nil {
		return
	}
	n := TraceMarkerTreeNode{ID: traceMarkerTreeID(source, ev.PID, ev.Line), SourcePath: source,
		Thread: threadRefFromEvent(ev), Name: ev.SpanName, StartLine: ev.Line, ActualStartTs: ev.Ts,
		Closure: "open", ParentStatus: "observed_root"}
	owner := traceMarkSyncPairingKey(source, ev.PID)
	if c.unknownPrefix || c.unknownOwners[owner] {
		n.ParentStatus = "unknown_prefix"
	}
	if len(stack) > 0 {
		parent := stack[len(stack)-1]
		n.ParentID, n.ParentStatus = traceMarkerTreeID(source, parent.PID, parent.Line), "observed_parent"
		if i, ok := c.byID[n.ParentID]; ok {
			c.entries[i].children = append(c.entries[i].children, len(c.entries))
			c.entries[i].node.DirectChildCount++
		}
	}
	c.byID[n.ID] = len(c.entries)
	c.entries = append(c.entries, traceMarkerTreeEntry{node: n})
}

func (c *traceMarkerTreeCollector) close(source string, start, end Event) {
	if c == nil {
		return
	}
	if i, ok := c.byID[traceMarkerTreeID(source, start.PID, start.Line)]; ok {
		n := &c.entries[i].node
		n.Closure, n.EndLine = "closed", end.Line
		n.ActualEndTs = new(float64)
		*n.ActualEndTs = end.Ts
	}
}

func (c *traceMarkerTreeCollector) reset(source string, pid int, stack []Event, ev Event) {
	if c == nil {
		return
	}
	c.observeOrder(source, pid, ev.Line)
	c.unknownOwners[traceMarkSyncPairingKey(source, pid)] = true
	for _, start := range stack {
		if i, ok := c.byID[traceMarkerTreeID(source, start.PID, start.Line)]; ok {
			c.entries[i].node.Closure = "invalidated"
			boundary := ev.Ts
			c.entries[i].resetTs = &boundary
		}
	}
}

func (c *traceMarkerTreeCollector) invalidate() {
	if c != nil {
		c.invalid = true
	}
}

func (c *traceMarkerTreeCollector) build(idx *Index, q Query, schedulerSafe bool) *TraceMarkerTreeStats {
	if c == nil || c.invalid || len(c.entries) == 0 || q.runCancel.sample() {
		return nil
	}
	out := &TraceMarkerTreeStats{Window: TraceMarkerTreeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}, Coverage: "observed_stream"}
	if !queryResultWindowStartSet(q) || q.TimeEnd == 0 && !q.TimeEndSet {
		out.WindowUnavailableReason = "line_selected_time_window_undetermined"
	}
	if c.unknownPrefix {
		out.Coverage = "partial_topology"
	}
	states := newTraceMarkerTreeStateCache(idx, q, schedulerSafe)
	for i := range c.entries {
		if q.runCancel.tick() {
			return nil
		}
		e := &c.entries[i]
		n := e.node
		if !traceMarkerTreeEntryVisible(e, q) {
			continue
		}
		if n.Closure == "closed" {
			whole, _ := traceMarkerTreeClip(n, q)
			children := make([]TimeWindow, 0, len(e.children))
			for _, child := range e.children {
				if q.runCancel.tick() {
					return nil
				}
				cn := &c.entries[child].node
				if cn.Closure == "closed" {
					children = append(children, TimeWindow{StartTs: cn.ActualStartTs, EndTs: *cn.ActualEndTs})
				}
			}
			self := traceMarkerTreeSubtractChildren(whole, children)
			n.Inclusive = states.account(n, []TimeWindow{whole})
			n.Self = states.account(n, self)
		}
		if n.ParentStatus == "unknown_prefix" || n.Closure != "closed" {
			out.Coverage = "partial_topology"
		}
		out.Nodes = append(out.Nodes, n)
	}
	if q.runCancel.sample() || len(out.Nodes) == 0 {
		return nil
	}
	sort.SliceStable(out.Nodes, func(i, j int) bool { return out.Nodes[i].StartLine < out.Nodes[j].StartLine })
	out.NodeCount = len(out.Nodes)
	if len(out.Nodes) > TraceMarkerTreeNodeLimit {
		out.OmittedNodes = len(out.Nodes) - TraceMarkerTreeNodeLimit
		out.Nodes = out.Nodes[:TraceMarkerTreeNodeLimit]
	}
	out.Caveats = []string{"Synchronous marker nesting is observed same-source, same-emitter stack structure, not a causal dependency or proof of a program root. Inclusive and self costs use all paired children before display truncation; missing parent rows are not roots. Direct child counts cover the full physical instance, including children outside the query. Unclosed starts are possible window context, not proof of active overlap; unclosed or invalidated instances have no invented elapsed cost. Scheduler values describe only the owner's evidenced states; sleep IO is a subset of sleep, not additional time."}
	return out
}

func traceMarkerTreeEntryVisible(e *traceMarkerTreeEntry, q Query) bool {
	n := &e.node
	if n.Closure == "closed" {
		_, ok := traceMarkerTreeClip(*n, q)
		return ok
	}
	if q.LineEnd > 0 && n.StartLine > q.LineEnd || (q.TimeEnd > 0 || q.TimeEndSet) && n.ActualStartTs > q.TimeEnd {
		return false
	}
	return e.resetTs == nil || *e.resetTs >= q.TimeStart
}

// Unlike legacy TimeWindow's zero sentinel, this new face preserves explicit
// zero endpoints. Line admission still uses overlap and never clips elapsed
// durations to an invented continuous time range.
func traceMarkerTreeClip(n TraceMarkerTreeNode, q Query) (TimeWindow, bool) {
	span := TimeWindow{StartTs: n.ActualStartTs, EndTs: *n.ActualEndTs}
	if q.LineStart > 0 && n.EndLine < q.LineStart || q.LineEnd > 0 && n.StartLine > q.LineEnd {
		return span, false
	}
	hasStart, hasEnd := queryResultWindowStartSet(q), q.TimeEnd > 0 || q.TimeEndSet
	if span.StartTs == span.EndTs {
		return span, (!hasStart || span.StartTs >= q.TimeStart) && (!hasEnd || span.EndTs <= q.TimeEnd)
	}
	if hasStart {
		span.StartTs = maxFloat(span.StartTs, q.TimeStart)
	}
	if hasEnd {
		span.EndTs = minFloat(span.EndTs, q.TimeEnd)
	}
	return span, span.EndTs > span.StartTs
}
