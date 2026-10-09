package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

const RenderingCandidatesLimit = 32
const RenderingCandidateSignalsLimit = 16
const RenderingCandidateExamplesLimit = 4

func buildRenderingCandidates(idx *Index, q Query) *RenderingCandidatesResult {
	p := &RenderingCandidatesResult{Status: "available", SourcePath: idx.Path,
		Window:    RenderingCandidatesWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled},
		TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope,
		Caveats: []string{RenderingCandidatesTeaching}}
	// Line-only discovery still reports the enclosing indexed time extent;
	// line bounds filter observations, never manufacture a complete interval.
	if q.LineStart != 0 || q.LineEnd != 0 {
		if !queryExplicitTimeStart(q) {
			p.Window.StartTs = idx.FirstTs
		}
		if !queryExplicitTimeEnd(q) {
			p.Window.EndTs, p.Window.EndInclusive = idx.LastTs, true
		}
	}
	if !isFiniteTraceNumber(p.Window.StartTs) || !isFiniteTraceNumber(p.Window.EndTs) || p.Window.EndTs < p.Window.StartTs {
		p.Status = "unavailable"
		p.Caveats = append(p.Caveats, "No finite ordered query window is available.")
		return p
	}
	if idx.Windowed || idx.RelationScoped {
		p.Caveats = append(p.Caveats, "Only the retained index scope was inspected; omitted trace material cannot support absence claims.")
	}
	sourceCounts := map[string]int{}
	for _, source := range idx.TraceArtifacts {
		sourceCounts[source.SourcePath]++
	}
	ambiguousSource := false
	for _, count := range sourceCounts {
		ambiguousSource = ambiguousSource || count > 1
	}
	if ambiguousSource {
		p.Caveats = append(p.Caveats, "Repeated source paths have ambiguous artifact identity and are excluded; no cross-generation owner grouping is inferred.")
	}
	type candidateKey struct {
		source, scope string
		owner         int
		framework     string
	}
	type candidateAccumulator struct {
		candidate RenderingCandidate
		signals   map[int]*RenderingCandidateSignal
	}
	groups := map[candidateKey]*candidateAccumulator{}
	seen := map[candidateKey]bool{}
	limit := ViewCapacityFor(ViewRenderingCandidates).ClampLimit(q.Limit)
	less := func(a, b candidateKey) bool {
		if a.source != b.source {
			return a.source < b.source
		}
		if a.scope != b.scope {
			return a.scope < b.scope
		}
		if a.owner != b.owner {
			return a.owner < b.owner
		}
		return a.framework < b.framework
	}
	types := map[EventType]bool{}
	for _, typ := range q.EventTypes {
		types[typ] = true
	}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		if !eventInQueryBase(ev, q, types, nil) || ev.Ts < p.Window.StartTs || ev.Ts > p.Window.EndTs || ev.Ts == p.Window.EndTs && !p.Window.EndInclusive {
			continue
		}
		coordinates := ProjectTraceEventInventoryCoordinates(ev)
		if !coordinates.EmitterTIDKnown || coordinates.EmitterTID <= 0 {
			continue
		}
		if q.PID > 0 {
			owner := coordinates.EmitterTID
			if q.TargetScope == TargetScopeProcess {
				owner = coordinates.EmitterTGID
			}
			if owner != q.PID {
				continue
			}
		} else if q.Thread != "" && ev.Comm != q.Thread {
			continue
		}
		if q.SpanName != "" && ev.SpanName != q.SpanName {
			continue
		}
		source, local := idx.Path, ev.Line
		if len(idx.TraceArtifacts) > 0 {
			spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
			if len(spans) != 1 {
				continue
			}
			source, local = spans[0].SourcePath, spans[0].LocalLineStart
		}
		if source == "" || local <= 0 || sourceCounts[source] > 1 {
			continue
		}
		scope, owner := TargetScopeThread, coordinates.EmitterTID
		if coordinates.EmitterTGIDKnown {
			scope, owner = TargetScopeProcess, coordinates.EmitterTGID
		}
		for ruleID, rule := range renderingCandidateRules {
			name := ev.Comm
			if rule.kind == "slice_name" {
				if ev.Type != EventTraceMark || ev.SpanName == "" {
					continue
				}
				name = ev.SpanName
			}
			if !renderingNameMatches(rule.pattern, name) {
				continue
			}
			key := candidateKey{source, scope, owner, rule.framework}
			seen[key] = true
			acc := groups[key]
			if acc == nil {
				// Count all distinct keys, but retain payload only for the
				// canonical first K. An evicted key can never return as the
				// upper bound monotonically decreases, so retained counts and
				// examples are complete without an unbounded payload map.
				if len(groups) >= limit {
					var largest candidateKey
					first := true
					for existing := range groups {
						if first || less(largest, existing) {
							largest, first = existing, false
						}
					}
					if !less(key, largest) {
						continue
					}
					delete(groups, largest)
				}
				status := "available"
				if rule.unsupported {
					status = "unsupported_definition"
				}
				acc = &candidateAccumulator{candidate: RenderingCandidate{Framework: rule.framework, DefinitionStatus: status, SourcePath: source, OwnerScope: scope, OwnerID: owner}, signals: map[int]*RenderingCandidateSignal{}}
				groups[key] = acc
			}
			signal := acc.signals[ruleID]
			if signal == nil {
				signal = &RenderingCandidateSignal{Kind: rule.kind, Pattern: rule.pattern, RoleCandidate: rule.role}
				acc.signals[ruleID] = signal
			}
			signal.Count++
			if len(signal.Examples) < RenderingCandidateExamplesLimit {
				signal.Examples = append(signal.Examples, RenderingCandidateObservation{ev.Line, local, ev.Ts, name, coordinates.EmitterTID, coordinates.EmitterTGID, ev.Comm})
			} else {
				signal.OmittedExamples++
			}
		}
	}
	keys := make([]candidateKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	if q.runCancel.sample() {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) })
	p.TotalCandidates, p.OmittedCandidates = len(seen), len(seen)-len(keys)
	for _, key := range keys {
		acc := groups[key]
		acc.candidate.TotalSignals = len(acc.signals)
		for ruleID := range renderingCandidateRules {
			if signal := acc.signals[ruleID]; signal != nil {
				if len(acc.candidate.Signals) >= RenderingCandidateSignalsLimit {
					acc.candidate.OmittedSignals++
					continue
				}
				acc.candidate.Signals = append(acc.candidate.Signals, *signal)
			}
		}
		p.Candidates = append(p.Candidates, acc.candidate)
	}
	if len(groups) == 0 {
		p.Caveats = append(p.Caveats, "No supported rendering signature was observed in the selected scope; framework remains unknown.")
	}
	return p
}

// ValidRenderingCandidates validates display structure/identity, not truth of
// a framework attribution. Even a valid result remains soft navigation only.
func ValidRenderingCandidates(p RenderingCandidatesResult) bool {
	if p.Status != "available" || p.SourcePath == "" || !isFiniteTraceNumber(p.Window.StartTs) || !isFiniteTraceNumber(p.Window.EndTs) || p.Window.EndTs < p.Window.StartTs || p.TargetPID < 0 || (p.TargetScope != TargetScopeThread && p.TargetScope != TargetScopeProcess) || len(p.Candidates) > RenderingCandidatesLimit || p.OmittedCandidates < 0 || p.TotalCandidates != len(p.Candidates)+p.OmittedCandidates {
		return false
	}
	seen := map[string]bool{}
	for _, c := range p.Candidates {
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s", c.SourcePath, c.OwnerScope, c.OwnerID, c.Framework)
		if seen[key] || c.SourcePath == "" || c.OwnerID <= 0 || (c.OwnerScope != TargetScopeThread && c.OwnerScope != TargetScopeProcess) || len(c.Signals) == 0 || len(c.Signals) > RenderingCandidateSignalsLimit || c.OmittedSignals < 0 || c.TotalSignals != len(c.Signals)+c.OmittedSignals {
			return false
		}
		seen[key] = true
		signals := map[string]bool{}
		for _, s := range c.Signals {
			id := s.Kind + "\x00" + s.Pattern
			if signals[id] || s.Count <= 0 || s.OmittedExamples < 0 || s.Count != len(s.Examples)+s.OmittedExamples || len(s.Examples) == 0 || len(s.Examples) > RenderingCandidateExamplesLimit {
				return false
			}
			signals[id] = true
			known := false
			for _, rule := range renderingCandidateRules {
				status := "available"
				if rule.unsupported {
					status = "unsupported_definition"
				}
				if rule.framework == c.Framework && rule.kind == s.Kind && rule.pattern == s.Pattern && rule.role == s.RoleCandidate && c.DefinitionStatus == status {
					known = true
					break
				}
			}
			if !known {
				return false
			}
			lines := map[int]bool{}
			for _, e := range s.Examples {
				if e.Line <= 0 || e.SourceLine <= 0 || e.TID <= 0 || e.TGID != -1 && e.TGID <= 0 || !isFiniteTraceNumber(e.Ts) || e.Ts < p.Window.StartTs || e.Ts > p.Window.EndTs || e.Ts == p.Window.EndTs && !p.Window.EndInclusive || !renderingNameMatches(s.Pattern, e.Name) {
					return false
				}
				if lines[e.Line] || s.Kind == "thread_name" && e.Name != e.Thread {
					return false
				}
				lines[e.Line] = true
				if c.OwnerScope == TargetScopeProcess && e.TGID != c.OwnerID || c.OwnerScope == TargetScopeThread && (e.TID != c.OwnerID || e.TGID != -1) {
					return false
				}
				if p.TargetPID > 0 && (p.TargetScope == TargetScopeThread && e.TID != p.TargetPID || p.TargetScope == TargetScopeProcess && e.TGID != p.TargetPID) || p.TargetPID == 0 && strings.TrimSpace(p.TargetThread) != "" && e.Thread != p.TargetThread {
					return false
				}
			}
		}
	}
	return true
}
