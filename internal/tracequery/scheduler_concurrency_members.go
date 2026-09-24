package tracequery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// This is publication provenance only. It never changes interval admission,
// the same-TID union, the source/identity gates, or any causal qualification.
type schedulerConcurrencyEndpoint struct {
	thread          ThreadRef
	cpu             int
	cpuKnown, known bool
	start           float64
	closure         string
}

func schedulerConcurrencyMemberForInterval(idx *Index, source, state string, endpoint schedulerConcurrencyEndpoint, end float64, startLine, endLine int, window *SchedulerConcurrencyWindow) (SchedulerConcurrencyMember, bool) {
	startLocal, startOK := ioInFlightMemberSourceLine(idx, source, startLine)
	endLocal, endOK := ioInFlightMemberSourceLine(idx, source, endLine)
	if !endpoint.known || !startOK || !endOK || !schedulerConcurrencyFinite(endpoint.start) || endpoint.start > end {
		return SchedulerConcurrencyMember{}, false
	}
	// Physical source and exact endpoint instances are the identity. Neither
	// names nor query clipping nor a composite's virtual line base enter it.
	tuple, _ := json.Marshal([2]string{source, state})
	m := SchedulerConcurrencyMember{
		ID:         fmt.Sprintf("sched:%x:%d:%d:%d:%016x:%016x", sha256.Sum256(tuple), endpoint.thread.PID, startLocal, endLocal, math.Float64bits(endpoint.start), math.Float64bits(end)),
		SourcePath: source, Thread: endpoint.thread, Closure: endpoint.closure,
		StartLine: startLine, EndLine: endLine, StartLocalLine: startLocal, EndLocalLine: endLocal,
		ActualStartTs: endpoint.start, ActualEndTs: end,
	}
	if endpoint.cpuKnown {
		cpu := endpoint.cpu
		m.CPU = &cpu
	}
	if window != nil {
		start, stop := math.Max(endpoint.start, window.StartTs), math.Min(end, window.EndTs)
		if stop < start {
			return SchedulerConcurrencyMember{}, false
		}
		ms := (stop - start) * 1000
		if !schedulerConcurrencyFinite(ms) {
			return SchedulerConcurrencyMember{}, false
		}
		m.WindowContribution = &SchedulerConcurrencyWindow{StartTs: start, EndTs: stop}
		m.WindowContributionMs = &ms
	}
	return m, true
}

func schedulerConcurrencyMemberLess(a, b SchedulerConcurrencyMember) bool {
	if a.StartLocalLine != b.StartLocalLine {
		return a.StartLocalLine < b.StartLocalLine
	}
	if a.EndLocalLine != b.EndLocalLine {
		return a.EndLocalLine < b.EndLocalLine
	}
	return a.ID < b.ID
}

func retainSchedulerConcurrencyMember(g *SchedulerConcurrencyGroup, member SchedulerConcurrencyMember) {
	i := sort.Search(len(g.Members), func(i int) bool { return !schedulerConcurrencyMemberLess(g.Members[i], member) })
	if len(g.Members) == SchedulerConcurrencyMemberLimit {
		g.OmittedMembers++
		if i == len(g.Members) {
			return
		}
	} else {
		g.Members = append(g.Members, SchedulerConcurrencyMember{})
	}
	copy(g.Members[i+1:], g.Members[i:len(g.Members)-1])
	g.Members[i] = member
}
