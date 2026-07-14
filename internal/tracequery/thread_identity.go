package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// threadResolution is the deterministic result of turning a user-facing
// thread selector into one scheduler thread identity.
//
// A PID-bearing query is already precise and always wins. A name-only query
// is usable only when the selected window identifies exactly one positive
// scheduler PID (exact comm matches win over substring matches). Returning an
// empty Thread on ambiguity is intentional: silently choosing the first row
// makes results depend on trace order, while falling back to comm would merge
// distinct threads that happen to share a name.
//
// CandidatePIDs is sorted and populated only for an ambiguous name selector.
// It is diagnostic output, never a heuristic input to a hard gate.
type threadResolution struct {
	Thread        ThreadRef
	CandidatePIDs []int
	Ambiguous     bool
}

type resolvedThreadCandidate struct {
	ref ThreadRef
}

// resolveThreadSelection resolves q to one scheduler identity. Candidate
// discovery is window-local first: a duplicate comm elsewhere in a full trace
// must not make an otherwise precise bounded query ambiguous. If the selected
// window contains no candidate, the full parsed index is used as a metadata
// fallback so a caller can still receive the correct PID plus an honest
// "no scheduler interval in this window" result.
func resolveThreadSelection(idx *Index, q Query) threadResolution {
	if q.PID > 0 {
		return threadResolution{Thread: resolvePIDThread(idx, q.PID, q)}
	}

	sel := parseThreadSelector(firstNonEmpty(q.ThreadInput, q.Thread))
	if sel.HasPID {
		if sel.HyphenPID {
			if resolution, handled := resolveHyphenThreadSelector(idx, q, sel); handled {
				return resolution
			}
		}
		return threadResolution{Thread: resolvePIDThread(idx, sel.PID, q)}
	}
	if strings.TrimSpace(sel.Raw) == "" && strings.TrimSpace(sel.Name) == "" {
		return threadResolution{}
	}

	exact, partial := collectThreadResolutionCandidates(idx, q, sel, true)
	bounded := q.TimeStartSet || q.TimeEndSet || q.TimeStart > 0 || q.TimeEnd > 0 || q.LineStart > 0 || q.LineEnd > 0
	if len(exact) == 0 && len(partial) == 0 && !bounded {
		exact, partial = collectThreadResolutionCandidates(idx, q, sel, false)
	}
	candidates := partial
	if len(exact) > 0 {
		// An exact scheduler comm is a precise signal. Broader substring hits
		// are only a convenience fallback and must not manufacture ambiguity
		// when an exact candidate exists.
		for pid := range exact {
			if metadata, ok := partial[pid]; ok {
				mergeThreadResolutionCandidate(exact, metadata)
			}
		}
		candidates = exact
	}
	if len(candidates) == 1 {
		for _, candidate := range candidates {
			return threadResolution{Thread: candidate.ref}
		}
	}
	if len(candidates) > 1 {
		pids := make([]int, 0, len(candidates))
		for pid := range candidates {
			pids = append(pids, pid)
		}
		sort.Ints(pids)
		return threadResolution{CandidatePIDs: pids, Ambiguous: true}
	}

	// Preserve the historical comm-only fallback for scheduler rows whose
	// format genuinely carries no positive PID (for example pid-0/swapper
	// forms). Once a positive PID candidate exists, every path above either
	// selects exactly one PID or fails closed on ambiguity.
	name := strings.TrimSpace(sel.Name)
	if name == "" {
		name = strings.TrimSpace(sel.Raw)
	}
	return threadResolution{Thread: ThreadRef{Comm: name}}
}

// resolveHyphenThreadSelector disambiguates the convenient "comm-123" form.
// A numeric suffix is commonly a rendered TID, but it can also be part of a
// literal comm. When both interpretations are present and point at different
// numeric TIDs, choosing either would be a hard identity guess, so fail closed.
// If only the literal comm is observed, preserve it; otherwise retain the
// historical rendered-label compatibility.
func resolveHyphenThreadSelector(idx *Index, q Query, sel threadSelector) (threadResolution, bool) {
	literal := threadSelector{Raw: sel.Raw, Name: sel.Raw}
	exact, _ := collectThreadResolutionCandidates(idx, q, literal, true)
	if len(exact) == 0 {
		return threadResolution{}, false
	}
	pids := make(map[int]bool, len(exact)+1)
	for pid := range exact {
		pids[pid] = true
	}
	if threadPIDObservedInQuery(idx, q, sel.PID) {
		pids[sel.PID] = true
	}
	if len(pids) > 1 {
		candidates := make([]int, 0, len(pids))
		for pid := range pids {
			candidates = append(candidates, pid)
		}
		sort.Ints(candidates)
		return threadResolution{CandidatePIDs: candidates, Ambiguous: true}, true
	}
	for pid, candidate := range exact {
		if pid == sel.PID {
			return threadResolution{Thread: resolvePIDThread(idx, pid, q)}, true
		}
		return threadResolution{Thread: candidate.ref}, true
	}
	return threadResolution{}, false
}

func threadPIDObservedInQuery(idx *Index, q Query, pid int) bool {
	if idx == nil || pid <= 0 {
		return false
	}
	if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
			if _, ok := head.Threads[pid]; ok {
				return true
			}
		}
	}
	for _, ev := range idx.Events {
		if !eventInQueryWindow(ev, q) {
			continue
		}
		if ev.PID == pid || ev.PrevPID == pid || ev.NextPID == pid || ev.WakeePID == pid {
			return true
		}
	}
	return false
}

func resolvePIDThread(idx *Index, pid int, q Query) ThreadRef {
	ref := ThreadRef{PID: pid}
	if idx == nil || pid <= 0 {
		return ref
	}
	dead := false
	apply := func(ev Event) bool {
		if schedWakeupStartsNewIncarnation(ev) && ev.WakeePID == pid {
			// Exact generation boundary: never inherit the prior occupant's comm
			// or TGID into the newly created task.
			ref = ThreadRef{PID: pid, Comm: ev.WakeeComm}
			dead = false
			return true
		}
		matched := ev.PID == pid || ev.NextPID == pid || ev.PrevPID == pid || ev.WakeePID == pid
		if !matched {
			return false
		}
		if dead {
			// Exact X/Z termination followed by another appearance without a
			// visible wakeup_new still starts a new occupant. Clear stale process
			// metadata before applying the reappearance row.
			ref = ThreadRef{PID: pid}
			dead = false
		}
		if ev.PID == pid {
			if strings.TrimSpace(ev.Comm) != "" {
				ref.Comm = ev.Comm
			}
			if ev.TGID > 0 {
				ref.TGID = ev.TGID
			}
		}
		var roleComm string
		switch {
		case pid == ev.NextPID:
			roleComm = ev.NextComm
		case pid == ev.PrevPID:
			roleComm = ev.PrevComm
		case pid == ev.WakeePID:
			roleComm = ev.WakeeComm
		}
		if strings.TrimSpace(roleComm) != "" {
			ref.Comm = roleComm
		}
		if ev.Type == EventSchedSwitch && ev.PrevPID == pid && stateFromPrevState(ev.PrevState) == StateDead {
			dead = true
		}
		return true
	}

	// Prefer the complete window-head checkpoint as the governing generation;
	// then consume only selected-window metadata. This is essential for bounded
	// indexes whose old creation/exit rows were intentionally not retained.
	headSeeded := false
	if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
			if state, ok := head.Threads[pid]; ok {
				state.Thread.PID = pid
				ref = state.Thread
				headSeeded = true
			}
		}
	}

	// Without a usable head, scan the lifecycle prefix through the query's upper
	// bound. With a head, scan only the inclusive selected window so pre-window
	// metadata cannot overwrite it.
	for _, ev := range idx.Events {
		// SUPP-HYG P3-C (§29.81 立案, 2026-07-14): cooperative sampling point.
		// This resolver runs NESTED per blocked-reason row inside the
		// ComputeWindowStats main sweep (donghu profile: resolvePIDThread was
		// >50% of the whole stats build), so without a tick each nested scan
		// was one opaque scan unit and the cancellation-observation gap grew
		// with O(rows × events). On fire the partial ref is returned and the
		// enclosing face is discarded whole by its attach gate (禁半账).
		if q.runCancel.tick() {
			return ref
		}
		if q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			continue
		}
		if headSeeded && !eventInQueryWindow(ev, q) {
			continue
		}
		apply(ev)
	}
	return ref
}

func collectThreadResolutionCandidates(idx *Index, q Query, sel threadSelector, windowOnly bool) (map[int]resolvedThreadCandidate, map[int]resolvedThreadCandidate) {
	exact := map[int]resolvedThreadCandidate{}
	partial := map[int]resolvedThreadCandidate{}
	if idx == nil {
		return exact, partial
	}
	selectorName := normalizedThreadText(sel.Name)
	if selectorName == "" {
		selectorName = normalizedThreadText(sel.Raw)
	}
	add := func(pid int, comm string, tgid int) {
		if pid <= 0 || !threadSelectorMatchesName(sel, comm) {
			return
		}
		candidate := resolvedThreadCandidate{ref: ThreadRef{Comm: comm, PID: pid, TGID: tgid}}
		mergeThreadResolutionCandidate(partial, candidate)
		if selectorName != "" && normalizedThreadText(comm) == selectorName {
			mergeThreadResolutionCandidate(exact, candidate)
		}
	}
	if windowOnly && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
			for pid, state := range head.Threads {
				add(pid, state.Thread.Comm, state.Thread.TGID)
			}
		}
	}
	for _, ev := range idx.Events {
		if windowOnly && !eventInQueryWindow(ev, q) {
			continue
		}
		add(ev.PID, ev.Comm, ev.TGID)
		add(ev.PrevPID, ev.PrevComm, 0)
		add(ev.NextPID, ev.NextComm, 0)
		add(ev.WakeePID, ev.WakeeComm, 0)
	}
	return exact, partial
}

func mergeThreadResolutionCandidate(dst map[int]resolvedThreadCandidate, candidate resolvedThreadCandidate) {
	current, ok := dst[candidate.ref.PID]
	if !ok {
		dst[candidate.ref.PID] = candidate
		return
	}
	if current.ref.TGID == 0 && candidate.ref.TGID > 0 {
		current.ref.TGID = candidate.ref.TGID
	}
	if strings.TrimSpace(current.ref.Comm) == "" && strings.TrimSpace(candidate.ref.Comm) != "" {
		current.ref.Comm = candidate.ref.Comm
	}
	dst[candidate.ref.PID] = current
}

// resolveThreadRefs applies the same exact-name-before-substring and
// unique-positive-PID policy to a pre-aggregated thread roster.  Streaming
// consumers use it after their one pass so a name-only selector cannot merge
// two same-name TIDs or depend on physical trace order.
func resolveThreadRefs(selector string, refs []ThreadRef) threadResolution {
	sel := parseThreadSelector(selector)
	if sel.HasPID {
		if sel.HyphenPID {
			literalPIDs := map[int]ThreadRef{}
			pidObserved := false
			for _, ref := range refs {
				if ref.PID == sel.PID {
					pidObserved = true
				}
				if normalizedThreadText(ref.Comm) == normalizedThreadText(sel.Raw) && ref.PID > 0 {
					literalPIDs[ref.PID] = ref
				}
			}
			if len(literalPIDs) > 0 {
				pids := map[int]bool{}
				for pid := range literalPIDs {
					pids[pid] = true
				}
				if pidObserved {
					pids[sel.PID] = true
				}
				if len(pids) > 1 {
					candidates := make([]int, 0, len(pids))
					for pid := range pids {
						candidates = append(candidates, pid)
					}
					sort.Ints(candidates)
					return threadResolution{CandidatePIDs: candidates, Ambiguous: true}
				}
				for _, ref := range literalPIDs {
					return threadResolution{Thread: ref}
				}
			}
		}
		for _, ref := range refs {
			if ref.PID == sel.PID {
				return threadResolution{Thread: ref}
			}
		}
		return threadResolution{Thread: ThreadRef{PID: sel.PID, Comm: strings.TrimSpace(sel.Name)}}
	}
	if strings.TrimSpace(sel.Raw) == "" && strings.TrimSpace(sel.Name) == "" {
		return threadResolution{}
	}
	exact := map[int]resolvedThreadCandidate{}
	partial := map[int]resolvedThreadCandidate{}
	selectorName := normalizedThreadText(sel.Name)
	if selectorName == "" {
		selectorName = normalizedThreadText(sel.Raw)
	}
	for _, ref := range refs {
		if ref.PID <= 0 || !threadSelectorMatchesName(sel, ref.Comm) {
			continue
		}
		candidate := resolvedThreadCandidate{ref: ref}
		mergeThreadResolutionCandidate(partial, candidate)
		if selectorName != "" && normalizedThreadText(ref.Comm) == selectorName {
			mergeThreadResolutionCandidate(exact, candidate)
		}
	}
	candidates := partial
	if len(exact) > 0 {
		candidates = exact
	}
	if len(candidates) == 1 {
		for _, candidate := range candidates {
			return threadResolution{Thread: candidate.ref}
		}
	}
	if len(candidates) > 1 {
		pids := make([]int, 0, len(candidates))
		for pid := range candidates {
			pids = append(pids, pid)
		}
		sort.Ints(pids)
		return threadResolution{CandidatePIDs: pids, Ambiguous: true}
	}
	return threadResolution{}
}

func threadResolutionCaveat(idx *Index, q Query) string {
	resolution := resolveThreadSelection(idx, q)
	if !resolution.Ambiguous {
		return ""
	}
	selector := strings.TrimSpace(firstNonEmpty(q.ThreadInput, q.Thread))
	return fmt.Sprintf("thread_selector_ambiguous selector=%q candidate_pids=%s; this name-only selector does not identify exactly one scheduler PID, so target-scoped views refuse to choose a thread by trace order — rerun with pid=<tid> or a pid-bearing thread selector", selector, joinThreadResolutionPIDs(resolution.CandidatePIDs))
}

func joinThreadResolutionPIDs(pids []int) string {
	if len(pids) == 0 {
		return ""
	}
	const maxItems = 8
	parts := make([]string, 0, min(len(pids), maxItems))
	for i, pid := range pids {
		if i == maxItems {
			break
		}
		parts = append(parts, strconv.Itoa(pid))
	}
	if len(pids) > maxItems {
		parts = append(parts, fmt.Sprintf("...(+%d)", len(pids)-maxItems))
	}
	return strings.Join(parts, ",")
}
