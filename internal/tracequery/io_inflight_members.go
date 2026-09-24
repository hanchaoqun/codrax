package tracequery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Source resolution is a witness-publication check, not a pairing check.
// Unverifiable provenance withholds only the member; accepted-pair numeric
// statistics remain governed by their existing matcher and sweep.
func ioInFlightMemberSourceLine(idx *Index, source string, line int) (int, bool) {
	if idx == nil || source == "" || line <= 0 {
		return 0, false
	}
	if len(idx.TraceArtifacts) == 0 {
		// Compatibility indexes without a ledger have physical line numbers,
		// but still need a concrete source and a proven physical line extent.
		return line, idx.Path == source && line <= idx.LineCount
	}
	i, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, line)
	if !ok || idx.TraceArtifacts[i].SourcePath != source {
		return 0, false
	}
	return line - idx.TraceArtifacts[i].VirtualLineBase, true
}

func ioInFlightMemberForPair(idx *Index, pair ioInFlightInterval, window *IOInFlightWindow) (IOInFlightMember, bool) {
	issueLocal, issueOK := ioInFlightMemberSourceLine(idx, pair.key.source, pair.endpoints.issueLine)
	completeLocal, completeOK := ioInFlightMemberSourceLine(idx, pair.key.source, pair.endpoints.completeLine)
	if !issueOK || !completeOK {
		return IOInFlightMember{}, false
	}
	// JSON tuple encoding avoids source/family delimiter collisions. The ID
	// excludes virtual line bases so it survives placement in another index.
	tuple, _ := json.Marshal([3]string{pair.key.source, pair.key.layer, pair.key.family})
	member := IOInFlightMember{
		ID:         fmt.Sprintf("io:%x:%d:%d:%016x:%016x", sha256.Sum256(tuple), issueLocal, completeLocal, math.Float64bits(pair.start), math.Float64bits(pair.end)),
		SourcePath: pair.key.source, IssueThread: pair.endpoints.issue, CompleteThread: pair.endpoints.complete,
		IssueLine: pair.endpoints.issueLine, CompleteLine: pair.endpoints.completeLine,
		IssueLocalLine: issueLocal, CompleteLocalLine: completeLocal,
		ActualStartTs: pair.start, ActualEndTs: pair.end,
	}
	if window != nil {
		ms := 0.0
		start, end := math.Max(pair.start, window.StartTs), math.Min(pair.end, window.EndTs)
		if end >= start {
			member.WindowContribution = &IOInFlightWindow{StartTs: start, EndTs: end}
			ms = (end - start) * 1000
		}
		member.WindowContributionMs = &ms
	}
	return member, true
}

func ioInFlightMemberLess(a, b IOInFlightMember) bool {
	if a.IssueLocalLine != b.IssueLocalLine {
		return a.IssueLocalLine < b.IssueLocalLine
	}
	if a.CompleteLocalLine != b.CompleteLocalLine {
		return a.CompleteLocalLine < b.CompleteLocalLine
	}
	return a.ID < b.ID
}

// Retain a stable physical-line prefix with bounded memory. Every accepted
// pair is still processed, including those beyond the display budget. A
// displaced witness is a capacity omission, never an unavailable witness.
func retainIOInFlightMember(group *IOInFlightGroup, member IOInFlightMember) {
	i := sort.Search(len(group.Members), func(i int) bool { return !ioInFlightMemberLess(group.Members[i], member) })
	if len(group.Members) == IOInFlightMemberLimit {
		group.OmittedMembers++
		if i == len(group.Members) {
			return
		}
	} else {
		group.Members = append(group.Members, IOInFlightMember{})
	}
	copy(group.Members[i+1:], group.Members[i:len(group.Members)-1])
	group.Members[i] = member
}
