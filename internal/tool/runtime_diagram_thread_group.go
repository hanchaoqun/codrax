package tool

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A native lane id is minted per lookup and per recorded precision, so one
// recorded thread can be published under several lanes. This file answers one
// display question only: do two published lanes observe the same recorded
// thread? Lane ids, labels, process fields, event identities, the authority
// pool and the presentation order are untouched. Every input is a
// provider-typed integer or string; no thread name, label or text is read.

// runtimeDiagramThreadSubject is the typed same-recorded-thread key: the
// physical capture file, both clocks and the thread number.
type runtimeDiagramThreadSubject struct {
	path, timeDomain, canonicalTimeDomain string
	tid                                   int
}

const runtimeDiagramThreadGroupPrefix = "thread\x00"

// The lifeline rule is taught in one place: the authoring capsule printed with
// the published rows. Other surfaces point to it instead of restating it.
const runtimeDiagramLifelineTeaching = "One lifeline per recorded thread: for each known thread, reuse the same supplied native thread participant, or one participant id of your own, for all of its arrows and state notes. Rows that show the same thread number in the same capture are that one thread even when their from_node/to_node ids differ (those ids are per lookup), unless that number is recorded with two process numbers or as exited and reused; keep each arrow's own from_identity/to_identity. Two different threads never share a lifeline."

// The repair text for arrows on another thread's lifeline. It names only the
// odd arrows and one patch operation.
const runtimeDiagramLifelineRepairTeaching = "Only the arrows named here sit on the lifeline of another recorded thread (one lifeline per recorded thread, as the runtime diagram anchors state); every other arrow and anchor is correct and stays unchanged. Move each named end to the lifeline of the thread it records and keep that arrow's from_identity/to_identity: in a patch use diagram_edge_edits with action \"replace\" on that arrow's failure_ref; in a full re-emit change only that from_node/to_node. These arrows are recorded events: move them, do not delete them. "

func (s runtimeDiagramThreadSubject) group() string {
	return runtimeDiagramThreadGroupPrefix + s.path + "\x00" + s.timeDomain + "\x00" + s.canonicalTimeDomain + "\x00" + strconv.Itoa(s.tid)
}

// runtimeDiagramThreadLifecycle is what the current ledger records about exit
// and reuse of one thread number in one capture. Any value other than none
// withholds the join of that number's lanes.
type runtimeDiagramThreadLifecycle uint8

const (
	runtimeDiagramThreadLifecycleNone runtimeDiagramThreadLifecycle = iota
	// The capture's lifecycle record is capped: which thread numbers were
	// reused is unknown, so no number of that capture is provably one task.
	runtimeDiagramThreadLifecycleIncomplete
	// An exit and reuse of this thread number is recorded.
	runtimeDiagramThreadLifecycleReused
)

type runtimeDiagramThreadSide struct {
	lane      string
	subject   runtimeDiagramThreadSubject
	process   int
	lifecycle runtimeDiagramThreadLifecycle
}

// runtimeDiagramRowThreadSides returns the typed thread observation on each
// side of one provider-validated wake row, or nothing when the row has no
// typed endpoints or no exact physical point.
func runtimeDiagramRowThreadSides(row RuntimeDiagramRelation) []runtimeDiagramThreadSide {
	e := row.threadEndpoints
	if row.Kind != types.DiagramRelWakeup || e == nil {
		return nil
	}
	_, path, ok := runtimeWakeupPhysicalPoint(types.ObservationRecord{SupportRefs: row.SupportRefs})
	if !ok {
		return nil
	}
	var out []runtimeDiagramThreadSide
	for _, side := range []struct {
		lane      string
		thread    types.RuntimeWakeupThread
		lifecycle runtimeDiagramThreadLifecycle
	}{{row.FromNode, e.from, e.fromLifecycle}, {row.ToNode, e.to, e.toLifecycle}} {
		// Thread number zero is the per-CPU idle task, never one thread.
		if !strings.HasPrefix(side.lane, "rt_thread_") || side.thread.PID <= 0 || side.thread.PID > types.RuntimeTargetMaxPID {
			continue
		}
		out = append(out, runtimeDiagramThreadSide{lane: side.lane, process: side.thread.TGID, lifecycle: side.lifecycle,
			subject: runtimeDiagramThreadSubject{path: path, timeDomain: e.source.TimeDomain, canonicalTimeDomain: e.source.CanonicalTimeDomain, tid: side.thread.PID}})
	}
	return out
}

// runtimeDiagramThreadIndex is the per-call view of which published lanes
// observe one recorded thread and why a join was withheld.
type runtimeDiagramThreadIndex struct {
	groups map[string]string
	// subjects holds the lanes that observe exactly one typed subject.
	subjects     map[string]runtimeDiagramThreadSubject
	twoProcesses map[runtimeDiagramThreadSubject]bool
	lifecycles   map[runtimeDiagramThreadSubject]runtimeDiagramThreadLifecycle
}

// Lanes of one recorded thread share a group. The join is withheld, and the
// lane stays its own group, when the lane has no typed endpoints, observes
// more than one subject, or its subject is contradicted: two different
// recorded process numbers (thread-number reuse across processes), or a
// lifecycle record (exit and reuse) that covers that thread number.
func runtimeDiagramThreadIndexOf(rows []RuntimeDiagramRelation) runtimeDiagramThreadIndex {
	laneSubjects := map[string]map[runtimeDiagramThreadSubject]bool{}
	processes := map[runtimeDiagramThreadSubject]map[int]bool{}
	index := runtimeDiagramThreadIndex{groups: map[string]string{}, subjects: map[string]runtimeDiagramThreadSubject{},
		twoProcesses: map[runtimeDiagramThreadSubject]bool{}, lifecycles: map[runtimeDiagramThreadSubject]runtimeDiagramThreadLifecycle{}}
	for _, row := range rows {
		for _, side := range runtimeDiagramRowThreadSides(row) {
			if laneSubjects[side.lane] == nil {
				laneSubjects[side.lane] = map[runtimeDiagramThreadSubject]bool{}
			}
			laneSubjects[side.lane][side.subject] = true
			if side.process > 0 {
				if processes[side.subject] == nil {
					processes[side.subject] = map[int]bool{}
				}
				processes[side.subject][side.process] = true
				if len(processes[side.subject]) > 1 {
					index.twoProcesses[side.subject] = true
				}
			}
			if side.lifecycle > index.lifecycles[side.subject] {
				index.lifecycles[side.subject] = side.lifecycle
			}
		}
	}
	for lane, subjects := range laneSubjects {
		index.groups[lane] = lane
		if len(subjects) != 1 {
			continue
		}
		for subject := range subjects {
			index.subjects[lane] = subject
			if !index.twoProcesses[subject] && index.lifecycles[subject] == runtimeDiagramThreadLifecycleNone {
				index.groups[lane] = subject.group()
			}
		}
	}
	return index
}

// runtimeDiagramThreadGroups maps every published thread lane to its group.
func runtimeDiagramThreadGroups(rows []RuntimeDiagramRelation) map[string]string {
	return runtimeDiagramThreadIndexOf(rows).groups
}

// A lane the provider did not publish with typed endpoints is its own group.
func runtimeDiagramThreadGroup(groups map[string]string, lane string) string {
	if group := groups[lane]; group != "" {
		return group
	}
	return lane
}

func runtimeDiagramSameThreadGroup(groups map[string]string, a, b string) bool {
	return runtimeDiagramThreadGroup(groups, a) == runtimeDiagramThreadGroup(groups, b)
}

// runtimeDiagramLaneJoined reports that the lane belongs to a proven
// recorded-thread group rather than standing alone.
func runtimeDiagramLaneJoined(groups map[string]string, lane string) bool {
	return strings.HasPrefix(groups[lane], runtimeDiagramThreadGroupPrefix)
}

// markRuntimeDiagramThreadLifecycleBoundaries records, per wake-row side,
// what the current ledger's thread lifecycle records (exit and reuse of one
// thread number) say about that thread number in that capture. A recorded
// reuse means the number may denote two tasks, so its lanes are never joined.
// A lifecycle record that names no thread number (a capped lifecycle audit)
// leaves every thread number of its capture unproven and withholds them all.
func markRuntimeDiagramThreadLifecycleBoundaries(rows []RuntimeDiagramRelation, ledger types.ObservationLedger) []RuntimeDiagramRelation {
	threads := map[string]map[int]bool{}
	whole := map[string]bool{}
	for _, record := range ledger.Records {
		if record.Predicate != "thread_incarnation_suppression" {
			continue
		}
		tid, err := strconv.Atoi(traceSupplementRichNoteValue(record.RichNotes, "conflict_tid"))
		capped := err != nil || tid <= 0 || traceSupplementRichNoteValue(record.RichNotes, "signal") == "lifecycle_audit_truncated"
		for _, capture := range runtimeDiagramLifecycleRecordCaptures(record) {
			if capped {
				whole[capture] = true
				continue
			}
			if threads[capture] == nil {
				threads[capture] = map[int]bool{}
			}
			threads[capture][tid] = true
		}
	}
	if len(threads) == 0 && len(whole) == 0 {
		return rows
	}
	for i := range rows {
		e := rows[i].threadEndpoints
		if e == nil {
			continue
		}
		captures := []string{e.source.Path}
		if _, path, ok := runtimeWakeupPhysicalPoint(types.ObservationRecord{SupportRefs: rows[i].SupportRefs}); ok {
			captures = append(captures, path)
		}
		lifecycle := func(tid int) runtimeDiagramThreadLifecycle {
			state := runtimeDiagramThreadLifecycleNone
			for _, capture := range captures {
				if capture == "" {
					continue
				}
				if threads[capture][tid] {
					return runtimeDiagramThreadLifecycleReused
				}
				if whole[capture] {
					state = runtimeDiagramThreadLifecycleIncomplete
				}
			}
			return state
		}
		from, to := lifecycle(e.from.PID), lifecycle(e.to.PID)
		if from == e.fromLifecycle && to == e.toLifecycle {
			continue
		}
		marked := *e
		marked.fromLifecycle, marked.toLifecycle = from, to
		rows[i].threadEndpoints = &marked
	}
	return rows
}

// The capture a lifecycle record speaks about: its query source and every
// physical file its support refs name.
func runtimeDiagramLifecycleRecordCaptures(record types.ObservationRecord) []string {
	captures := []string{record.SourceRef.Path}
	for _, ref := range record.SupportRefs {
		if colon := strings.LastIndexByte(ref, ':'); colon > 0 {
			captures = append(captures, ref[:colon])
		}
	}
	return captures
}

// runtimeDiagramWakeEventKey identifies one physical wake row: its exact
// physical point, both clocks, the native timestamp and both thread numbers.
func runtimeDiagramWakeEventKey(row RuntimeDiagramRelation) (string, bool) {
	e := row.threadEndpoints
	if row.Kind != types.DiagramRelWakeup || e == nil || row.eventOrder == nil {
		return "", false
	}
	point, _, ok := runtimeWakeupPhysicalPoint(types.ObservationRecord{SupportRefs: row.SupportRefs})
	if !ok {
		return "", false
	}
	return strings.Join([]string{point, e.source.TimeDomain, e.source.CanonicalTimeDomain,
		strconv.FormatFloat(row.eventOrder.timestamp, 'g', -1, 64), strconv.Itoa(e.from.PID), strconv.Itoa(e.to.PID)}, "\x00"), true
}

// A request binding proved on one lookup's copy of a physical wake row also
// describes the other current copies of that same row. Nothing else on a row
// changes: no process number is filled in, no lane is merged and no binding
// is minted where no copy was bound. A side whose copies record two
// different process numbers is left as it is.
func propagateRequestedParticipantsAcrossEventCopies(rows []RuntimeDiagramRelation) []RuntimeDiagramRelation {
	type copies struct {
		from, to                   []string
		fromProcesses, toProcesses map[int]bool
	}
	byEvent := map[string]*copies{}
	for _, row := range rows {
		key, ok := runtimeDiagramWakeEventKey(row)
		if !ok {
			continue
		}
		c := byEvent[key]
		if c == nil {
			c = &copies{fromProcesses: map[int]bool{}, toProcesses: map[int]bool{}}
			byEvent[key] = c
		}
		if tgid := row.threadEndpoints.from.TGID; tgid > 0 {
			c.fromProcesses[tgid] = true
		}
		if tgid := row.threadEndpoints.to.TGID; tgid > 0 {
			c.toProcesses[tgid] = true
		}
		c.from = runtimeDiagramMergeRequested(c.from, row.requestedFrom)
		c.to = runtimeDiagramMergeRequested(c.to, row.requestedTo)
	}
	for i := range rows {
		key, ok := runtimeDiagramWakeEventKey(rows[i])
		if !ok {
			continue
		}
		c := byEvent[key]
		if len(c.from) > 0 && len(c.fromProcesses) <= 1 {
			rows[i].requestedFrom = runtimeDiagramMergeRequested(rows[i].requestedFrom, c.from)
		}
		if len(c.to) > 0 && len(c.toProcesses) <= 1 {
			rows[i].requestedTo = runtimeDiagramMergeRequested(rows[i].requestedTo, c.to)
		}
	}
	return rows
}

func runtimeDiagramMergeRequested(have, add []string) []string {
	for _, value := range add {
		found := false
		for _, existing := range have {
			found = found || existing == value
		}
		if !found {
			have = append(append([]string(nil), have...), value)
		}
	}
	return have
}

// runtimeDiagramLifelineOddSide is one end of one recorded wake arrow that
// sits on a lifeline of another recorded thread.
type runtimeDiagramLifelineOddSide struct {
	anchor            int
	from              bool
	homeLane, oddLane string
	// homeFromFirstArrow: the lifeline's thread was set by its first arrow in
	// document order, not by a published thread id used as the lifeline id.
	homeFromFirstArrow bool
}

// runtimeDiagramLifelineOddSides checks one diagram node as a lifeline. The
// node stands for the recorded thread whose published id it uses, or else for
// the thread its first arrow in document order binds. Every later arrow end
// on that node must record the same thread group; the ones that do not are
// returned. Lanes without the "rt_thread_" prefix never bind a lifeline.
func runtimeDiagramLifelineOddSides(rows []RuntimeDiagramRelation, groups map[string]string, anchors []types.DiagramEdgeAnchor, node string) []runtimeDiagramLifelineOddSide {
	home, first := "", true
	if _, published := groups[node]; published {
		home, first = node, false
	}
	var odd []runtimeDiagramLifelineOddSide
	for i, a := range anchors {
		for _, r := range rows {
			if r.Kind != a.RelationKind || r.FromIdentity != a.FromIdentity || r.ToIdentity != a.ToIdentity {
				continue
			}
			for j, side := range [][2]string{{a.FromNode, r.FromNode}, {a.ToNode, r.ToNode}} {
				if side[0] != node || !strings.HasPrefix(side[1], "rt_thread_") {
					continue
				}
				if home == "" {
					home = side[1]
				} else if !runtimeDiagramSameThreadGroup(groups, home, side[1]) {
					odd = append(odd, runtimeDiagramLifelineOddSide{anchor: i, from: j == 0, homeLane: home, oddLane: side[1], homeFromFirstArrow: first})
				}
			}
		}
	}
	return odd
}

// runtimeDiagramLifelineOddArrows names, per edge_anchors index of one
// diagram block, the recorded wake arrows that sit on a lifeline of another
// recorded thread, in reader words. A sequence diagram checks every lifeline;
// other diagram kinds check only nodes that use a published thread id.
func runtimeDiagramLifelineOddArrows(rows []RuntimeDiagramRelation, block types.AnswerBlock) map[int]string {
	if block.Diagram == nil || len(rows) == 0 || len(block.EdgeAnchors) == 0 {
		return nil
	}
	index := runtimeDiagramThreadIndexOf(rows)
	if len(index.groups) == 0 && block.Diagram.Kind != types.DiagramSequence {
		return nil
	}
	labels := runtimeNativeParticipantLanes(rows)
	sequence := block.Diagram.Kind == types.DiagramSequence
	var nodes []string
	seen := map[string]bool{}
	for _, a := range block.EdgeAnchors {
		for _, node := range []string{a.FromNode, a.ToNode} {
			if _, published := index.groups[node]; !seen[node] && (sequence || published) {
				seen[node] = true
				nodes = append(nodes, node)
			}
		}
	}
	// A lifeline that already carries a thread, by group, for the suggestion.
	hosts := map[string]string{}
	oddByNode := map[string][]runtimeDiagramLifelineOddSide{}
	for _, node := range nodes {
		oddByNode[node] = runtimeDiagramLifelineOddSides(rows, index.groups, block.EdgeAnchors, node)
	}
	for _, node := range nodes {
		home := ""
		if _, published := index.groups[node]; published {
			home = node
		} else {
			home = runtimeDiagramLifelineFirstLane(rows, block.EdgeAnchors, node)
		}
		if home == "" {
			continue
		}
		if group := runtimeDiagramThreadGroup(index.groups, home); hosts[group] == "" {
			hosts[group] = node
		}
	}
	out := map[int]string{}
	for _, node := range nodes {
		for _, odd := range oddByNode[node] {
			a := block.EdgeAnchors[odd.anchor]
			lifeline := fmt.Sprintf("Lifeline %q is thread %s", node, labels[odd.homeLane])
			if !sequence {
				lifeline = fmt.Sprintf("Node %q is thread %s", node, labels[odd.homeLane])
			}
			if _, published := index.groups[node]; published {
				lifeline = fmt.Sprintf("The lifeline declared with the published id of thread %s", labels[odd.homeLane])
				if !sequence {
					lifeline = fmt.Sprintf("The node declared with the published id of thread %s", labels[odd.homeLane])
				}
				lifeline += " is that thread"
			} else if odd.homeFromFirstArrow {
				lifeline += " (its first arrow)"
			}
			end, field := "ends at", "to_node"
			if odd.from {
				end, field = "starts from", "from_node"
			}
			target := "a lifeline of its own"
			if host := hosts[runtimeDiagramThreadGroup(index.groups, odd.oddLane)]; host != "" && host != node {
				target = fmt.Sprintf("lifeline %q", host)
				if _, published := index.groups[host]; published {
					target = "the lifeline already declared for that thread"
				}
			}
			from, to := runtimeDiagramArrowThreads(rows, a)
			detail := fmt.Sprintf("%s. Arrow %q (%s → %s) %s thread %s: set its %s to %s", lifeline,
				runtimeDiagramShortLabel(a.VisibleLabel), from, to, end, labels[odd.oddLane], field, target)
			if reason := index.withheldReason(odd.homeLane, odd.oddLane); reason != "" {
				detail += "; " + reason
			}
			if out[odd.anchor] != "" {
				detail = out[odd.anchor] + ". " + detail
			}
			out[odd.anchor] = detail
		}
	}
	return out
}

func runtimeDiagramLifelineFirstLane(rows []RuntimeDiagramRelation, anchors []types.DiagramEdgeAnchor, node string) string {
	for _, a := range anchors {
		for _, r := range rows {
			if r.Kind != a.RelationKind || r.FromIdentity != a.FromIdentity || r.ToIdentity != a.ToIdentity {
				continue
			}
			for _, side := range [][2]string{{a.FromNode, r.FromNode}, {a.ToNode, r.ToNode}} {
				if side[0] == node && strings.HasPrefix(side[1], "rt_thread_") {
					return side[1]
				}
			}
		}
	}
	return ""
}

// The recorded endpoint labels of the row an anchor selects.
func runtimeDiagramArrowThreads(rows []RuntimeDiagramRelation, a types.DiagramEdgeAnchor) (from, to string) {
	for _, r := range rows {
		if r.Kind == a.RelationKind && r.FromIdentity == a.FromIdentity && r.ToIdentity == a.ToIdentity {
			return r.FromLabel, r.ToLabel
		}
	}
	return "", ""
}

func runtimeDiagramShortLabel(label string) string {
	runes := []rune(strings.TrimSpace(label))
	if len(runes) > 48 {
		return string(runes[:48]) + "…"
	}
	return string(runes)
}

// withheldReason explains, from typed data only, why two recordings of the
// same thread number are not one lifeline. Different thread numbers need no
// explanation.
func (index runtimeDiagramThreadIndex) withheldReason(a, b string) string {
	left, okLeft := index.subjects[a]
	right, okRight := index.subjects[b]
	if !okLeft || !okRight || left.tid != right.tid {
		return ""
	}
	why := "it is recorded in another capture file or on another clock"
	if left == right {
		switch {
		case index.twoProcesses[left]:
			why = "two different process numbers are recorded for it"
		case index.lifecycles[left] == runtimeDiagramThreadLifecycleReused:
			why = "the capture records an exit and reuse of this thread number"
		case index.lifecycles[left] == runtimeDiagramThreadLifecycleIncomplete:
			why = "the capture's record of thread exits and reuses is incomplete, so one number is not proven to be one thread"
		default:
			return ""
		}
	}
	return fmt.Sprintf("the recordings of thread number %d are kept apart because %s", left.tid, why)
}
