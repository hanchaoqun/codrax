package tracequery

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// One physical thread has one identity. The hard key of a scheduler thread is
// its positive TID inside one capture generation; the generation advances only
// on the exact lifecycle records the incarnation guard already owns (a
// creation record for a TID that has already appeared, or a reappearance after
// an X/Z exit). A creation record for a never-seen TID starts its FIRST
// generation and is not a boundary. comm is display metadata: a thread that
// renames itself (an app main thread "main" -> package name, a new thread
// that keeps its creator's name until it names itself, a pool worker that
// takes the name of each task it runs) is still one thread.
//
// Producers fill ThreadRef.Comm from whichever row they happen to read (the
// creation row, the first runnable row, the last switch-out of a window), so
// one TID used to reach the reader under several names. This file owns the
// single display rule and its publication.
//
// Name runs. The scheduler payload names of a TID (sched_switch prev/next
// comm, sched_wakeup / sched_wakeup_new / sched_waking comm) form runs of
// equal consecutive values in physical row order. A run lasts from its first
// record to the first record of the next run; the last run lasts on. The run
// that starts at the TID's own creation record carries the creator's name
// (the kernel copies the creator's name into a new task), so while the thread
// records a name of its own later, that first run is an inherited name, never
// the thread's display name. The row-header column (the kernel's saved
// command-line cache, read when the capture was written out) has no event
// time and only names threads that have no scheduler payload name.
//
// Display rule, for the result's own time window: the own name whose runs
// cover the most of the window; when no own run touches the window, the
// nearest own name (the first after the window, else the last before it).
// Every typed thread field and every system label of the result uses it; the
// result lists every other recorded name with its rows once per thread.
// Unification is withheld (names stay as produced) for a TID with more than
// one generation, more than one recorded process number, rows in more than one
// capture artifact, or more runs than the private cap. No input is user or
// model text; the choice is display only and no gate reads it.

const (
	threadDisplayRunCap           = 1024 // private: reaching it withholds unification
	threadDisplayPublishedNameCap = 8
	threadDisplayIdentityCap      = 32
	threadDisplayPlaceholderComm  = "<...>"
)

// ThreadDisplayIdentity is the published identity of one physical thread whose
// recorded names differ. DisplayComm is the name every typed thread field of
// the same result carries; Names lists every recorded name with its rows (the
// display name included).
type ThreadDisplayIdentity struct {
	TID            int                     `json:"tid"`
	TGID           int                     `json:"tgid,omitempty"`
	DisplayComm    string                  `json:"display_comm"`
	DisplayBasis   string                  `json:"display_basis"`
	Names          []ThreadNameObservation `json:"names"`
	NamesTruncated bool                    `json:"names_truncated,omitempty"`
}

// ThreadNameObservation is one name a thread carried in the rows a lookup read.
type ThreadNameObservation struct {
	Comm      string  `json:"comm"`
	FirstLine int     `json:"first_line,omitempty"`
	FirstTs   float64 `json:"first_ts,omitempty"`
	LastLine  int     `json:"last_line,omitempty"`
	LastTs    float64 `json:"last_ts,omitempty"`
	// InheritedAtCreation: the name the thread carried from its creation
	// record until it named itself (its creator's name), never its own.
	InheritedAtCreation bool `json:"inherited_at_creation,omitempty"`
	// HeaderOnly: seen only in the row-header column, never in a scheduler
	// payload; its rows are the rows whose header showed it.
	HeaderOnly bool `json:"header_only,omitempty"`
}

const (
	// ThreadDisplayBasisLongestInWindow: the own name covering most of the
	// result's window.
	ThreadDisplayBasisLongestInWindow = "longest_own_name_in_window"
	// ThreadDisplayBasisNearestOwnName: no own name touched the window; the
	// nearest own name was used.
	ThreadDisplayBasisNearestOwnName = "nearest_own_name"
	// ThreadDisplayBasisRowHeader: no scheduler payload name; the last
	// row-header name was used.
	ThreadDisplayBasisRowHeader = "row_header"
)

type threadNameRun struct {
	comm                string
	firstLine, lastLine int
	firstTs, lastTs     float64
	inherited           bool
}

type threadDisplayRecord struct {
	tid        int
	tgid       int
	runs       []threadNameRun
	headerOnly []ThreadNameObservation
	known      map[string]bool
	// byLength holds every recorded name longest first for label matching.
	byLength []string
}

type threadDisplayLedger struct {
	records map[int]*threadDisplayRecord
	// reuseCreations are the creation records that reused an already-seen
	// TID in the complete index this ledger was built from; lifecycleCapped
	// means that audit could not name every boundary owner.
	reuseCreations  map[threadCreationLine]bool
	lifecycleCapped bool
}

// threadCreationLine names one creation record: the created TID and the
// physical line of its sched_wakeup_new row.
type threadCreationLine struct {
	tid, line int
}

const threadCreationReuseCap = 4096

// recordThreadCreationReuse keeps a creation-record conflict of the streamed
// lifecycle audit (see Index.threadCreationReuses).
func recordThreadCreationReuse(idx *Index, c threadIncarnationConflict) {
	if idx == nil || c.Signal != "sched_wakeup_new" || c.PID <= 0 || c.BoundaryLine <= 0 {
		return
	}
	key := threadCreationLine{tid: c.PID, line: c.BoundaryLine}
	if idx.threadCreationReuses[key] {
		return
	}
	if len(idx.threadCreationReuses) >= threadCreationReuseCap {
		idx.threadCreationReusesCapped = true
		return
	}
	if idx.threadCreationReuses == nil {
		idx.threadCreationReuses = map[threadCreationLine]bool{}
	}
	idx.threadCreationReuses[key] = true
}

type threadDisplayAccumulator struct {
	runs         []threadNameRun
	headerNames  []ThreadNameObservation
	headerPos    map[string]int
	tgids        map[int]bool
	sources      map[int]bool
	creationLine int
	overflow     bool
}

func threadDisplayUsableComm(comm string) (string, bool) {
	comm = strings.TrimSpace(comm)
	return comm, comm != "" && comm != threadDisplayPlaceholderComm
}

func (a *threadDisplayAccumulator) payload(comm string, line int, ts float64) {
	comm, ok := threadDisplayUsableComm(comm)
	if !ok {
		return
	}
	if n := len(a.runs); n > 0 && a.runs[n-1].comm == comm {
		a.runs[n-1].lastLine, a.runs[n-1].lastTs = line, ts
		return
	}
	if len(a.runs) >= threadDisplayRunCap {
		a.overflow = true
		return
	}
	a.runs = append(a.runs, threadNameRun{comm: comm, firstLine: line, lastLine: line, firstTs: ts, lastTs: ts})
}

func (a *threadDisplayAccumulator) header(comm string, line int, ts float64) {
	comm, ok := threadDisplayUsableComm(comm)
	if !ok {
		return
	}
	if a.headerPos == nil {
		a.headerPos = map[string]int{}
	}
	if i, seen := a.headerPos[comm]; seen {
		a.headerNames[i].LastLine, a.headerNames[i].LastTs = line, ts
		return
	}
	if len(a.headerNames) >= threadDisplayRunCap {
		a.overflow = true
		return
	}
	a.headerPos[comm] = len(a.headerNames)
	a.headerNames = append(a.headerNames, ThreadNameObservation{Comm: comm, HeaderOnly: true, FirstLine: line, FirstTs: ts, LastLine: line, LastTs: ts})
}

// ensureThreadDisplayLedger returns the capture-level display ledger. A
// windowed index inherits its parent's ledger (set at derivation).
func ensureThreadDisplayLedger(idx *Index) *threadDisplayLedger {
	if idx == nil {
		return nil
	}
	if idx.threadDisplayInherited != nil {
		return idx.threadDisplayInherited
	}
	idx.threadDisplayOnce.Do(func() {
		idx.threadDisplay = buildThreadDisplayLedger(idx)
	})
	return idx.threadDisplay
}

func buildThreadDisplayLedger(idx *Index) *threadDisplayLedger {
	ledger := &threadDisplayLedger{records: map[int]*threadDisplayRecord{}}
	if idx == nil || len(idx.Events) == 0 {
		return ledger
	}
	boundaries, capped := ensureThreadGenerationMetadata(idx)
	if capped {
		// The lifecycle audit cannot name every reused TID: no name may be
		// unified, because any of them could join two tasks.
		ledger.lifecycleCapped = true
		return ledger
	}
	for pid, items := range boundaries {
		for _, boundary := range items {
			if boundary.Signal == "sched_wakeup_new" && boundary.BoundaryLine > 0 {
				if ledger.reuseCreations == nil {
					ledger.reuseCreations = map[threadCreationLine]bool{}
				}
				ledger.reuseCreations[threadCreationLine{tid: pid, line: boundary.BoundaryLine}] = true
			}
		}
	}
	multiSource := len(idx.TraceArtifacts) > 1
	acc := map[int]*threadDisplayAccumulator{}
	get := func(tid int) *threadDisplayAccumulator {
		a := acc[tid]
		if a == nil {
			a = &threadDisplayAccumulator{}
			acc[tid] = a
		}
		return a
	}
	for i := range idx.Events {
		ev := &idx.Events[i]
		source := 0
		if multiSource {
			if s, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, ev.Line); ok {
				source = s
			} else {
				source = -1
			}
		}
		touch := func(tid int) *threadDisplayAccumulator {
			a := get(tid)
			if multiSource {
				if a.sources == nil {
					a.sources = map[int]bool{}
				}
				a.sources[source] = true
			}
			return a
		}
		if ev.PID > 0 {
			a := touch(ev.PID)
			a.header(ev.Comm, ev.Line, ev.Ts)
			if ev.TGID > 0 {
				if a.tgids == nil {
					a.tgids = map[int]bool{}
				}
				a.tgids[ev.TGID] = true
			}
		}
		switch ev.Type {
		case EventSchedSwitch:
			if ev.PrevPID > 0 {
				touch(ev.PrevPID).payload(ev.PrevComm, ev.Line, ev.Ts)
			}
			if ev.NextPID > 0 {
				touch(ev.NextPID).payload(ev.NextComm, ev.Line, ev.Ts)
			}
		case EventSchedWakeup, EventSchedWaking:
			if ev.WakeePID > 0 {
				a := touch(ev.WakeePID)
				if schedWakeupStartsNewIncarnation(*ev) && a.creationLine == 0 {
					a.creationLine = ev.Line
				}
				a.payload(ev.WakeeComm, ev.Line, ev.Ts)
			}
		}
	}
	for tid, a := range acc {
		if a.overflow || len(boundaries[tid]) > 0 || len(a.tgids) > 1 || len(a.sources) > 1 || a.sources[-1] {
			continue
		}
		record := &threadDisplayRecord{tid: tid, runs: a.runs, known: map[string]bool{}}
		for tgid := range a.tgids {
			record.tgid = tgid
		}
		for _, run := range record.runs {
			record.known[run.comm] = true
		}
		for _, name := range a.headerNames {
			if !record.known[name.Comm] {
				record.headerOnly = append(record.headerOnly, name)
			}
			record.known[name.Comm] = true
		}
		if len(record.known) < 2 {
			continue
		}
		if len(record.runs) > 1 && a.creationLine > 0 && record.runs[0].firstLine == a.creationLine {
			for _, later := range record.runs[1:] {
				if later.comm != record.runs[0].comm {
					record.runs[0].inherited = true
					break
				}
			}
		}
		for comm := range record.known {
			record.byLength = append(record.byLength, comm)
		}
		sort.Slice(record.byLength, func(i, j int) bool {
			if len(record.byLength[i]) != len(record.byLength[j]) {
				return len(record.byLength[i]) > len(record.byLength[j])
			}
			return record.byLength[i] < record.byLength[j]
		})
		ledger.records[tid] = record
	}
	return ledger
}

// displayForWindow applies the display rule to one time window. start/end of
// zero mean an open side.
func (r *threadDisplayRecord) displayForWindow(start, end float64) (string, string) {
	if len(r.runs) == 0 {
		if n := len(r.headerOnly); n > 0 {
			return r.headerOnly[n-1].Comm, ThreadDisplayBasisRowHeader
		}
		return "", ""
	}
	lo, hi := start, end
	if lo <= 0 {
		lo = math.Inf(-1)
	}
	if hi <= 0 || hi < lo {
		hi = math.Inf(1)
	}
	covered := map[string]float64{}
	lastSeen := map[string]int{}
	for i, run := range r.runs {
		if run.inherited {
			continue
		}
		runStart, runEnd := run.firstTs, math.Inf(1)
		if i == 0 {
			runStart = math.Inf(-1)
		}
		if i+1 < len(r.runs) {
			runEnd = r.runs[i+1].firstTs
		}
		overlap := math.Min(runEnd, hi) - math.Max(runStart, lo)
		if math.IsInf(lo, -1) && math.IsInf(hi, 1) {
			// Open window: weigh by recorded extent so the choice stays finite.
			overlap = run.lastTs - run.firstTs
			if i+1 < len(r.runs) {
				overlap = r.runs[i+1].firstTs - run.firstTs
			}
		}
		if overlap > 0 || (overlap == 0 && runStart <= hi && runEnd > lo) {
			covered[run.comm] += math.Max(overlap, 0)
			lastSeen[run.comm] = i
		}
	}
	best, bestCover, bestLast := "", -1.0, -1
	for comm, cover := range covered {
		last := lastSeen[comm]
		if cover > bestCover || (cover == bestCover && last > bestLast) {
			best, bestCover, bestLast = comm, cover, last
		}
	}
	if best != "" {
		return best, ThreadDisplayBasisLongestInWindow
	}
	// No own run touches the window: nearest own name, after first.
	for _, run := range r.runs {
		if !run.inherited && run.firstTs >= hi {
			return run.comm, ThreadDisplayBasisNearestOwnName
		}
	}
	for i := len(r.runs) - 1; i >= 0; i-- {
		if !r.runs[i].inherited {
			return r.runs[i].comm, ThreadDisplayBasisNearestOwnName
		}
	}
	return r.runs[len(r.runs)-1].comm, ThreadDisplayBasisNearestOwnName
}

// names projects every recorded name with its rows, in first-seen order.
func (r *threadDisplayRecord) names() []ThreadNameObservation {
	var out []ThreadNameObservation
	pos := map[string]int{}
	for _, run := range r.runs {
		if i, ok := pos[run.comm]; ok {
			out[i].LastLine, out[i].LastTs = run.lastLine, run.lastTs
			out[i].InheritedAtCreation = out[i].InheritedAtCreation && run.inherited
			continue
		}
		pos[run.comm] = len(out)
		out = append(out, ThreadNameObservation{Comm: run.comm, FirstLine: run.firstLine, FirstTs: run.firstTs,
			LastLine: run.lastLine, LastTs: run.lastTs, InheritedAtCreation: run.inherited})
	}
	return append(out, r.headerOnly...)
}

// SameThread is the exported thread-identity predicate for consumers outside
// this package: a positive thread number is the thread; the name decides only
// when neither side carries one.
func SameThread(a, b ThreadRef) bool {
	if a.PID > 0 || b.PID > 0 {
		return a.PID > 0 && a.PID == b.PID
	}
	return strings.TrimSpace(a.Comm) != "" && strings.TrimSpace(a.Comm) == strings.TrimSpace(b.Comm)
}

// record returns the unified record of tid, or nil (nil-safe).
func (l *threadDisplayLedger) record(tid int) *threadDisplayRecord {
	if l == nil || tid <= 0 {
		return nil
	}
	return l.records[tid]
}

// threadDisplayView is the ledger seen through one result window.
type threadDisplayView struct {
	ledger     *threadDisplayLedger
	start, end float64
	chosen     map[int]string
	basis      map[int]string
}

func newThreadDisplayView(ledger *threadDisplayLedger, start, end float64) *threadDisplayView {
	return &threadDisplayView{ledger: ledger, start: start, end: end, chosen: map[int]string{}, basis: map[int]string{}}
}

func (v *threadDisplayView) display(tid int) (string, bool) {
	record := v.ledger.record(tid)
	if record == nil {
		return "", false
	}
	if comm, ok := v.chosen[tid]; ok {
		return comm, comm != ""
	}
	comm, basis := record.displayForWindow(v.start, v.end)
	v.chosen[tid], v.basis[tid] = comm, basis
	return comm, comm != ""
}

// displayComm renames only a comm that is one of the TID's recorded names: a
// name the capture never recorded for that TID (a caller's selector text) is
// left as is.
func (v *threadDisplayView) displayComm(tid int, comm string) (string, bool) {
	record := v.ledger.record(tid)
	if record == nil {
		return comm, false
	}
	trimmed := strings.TrimSpace(comm)
	if trimmed == "" || !record.known[trimmed] {
		return comm, false
	}
	display, ok := v.display(tid)
	if !ok {
		return comm, false
	}
	return display, display != comm
}

func (v *threadDisplayView) published(tid int) (ThreadDisplayIdentity, bool) {
	record := v.ledger.record(tid)
	display, ok := v.display(tid)
	if record == nil || !ok {
		return ThreadDisplayIdentity{}, false
	}
	out := ThreadDisplayIdentity{TID: tid, TGID: record.tgid, DisplayComm: display, DisplayBasis: v.basis[tid]}
	names := record.names()
	if len(names) > threadDisplayPublishedNameCap {
		// Keep the display name inside the bounded projection.
		kept := append([]ThreadNameObservation(nil), names[:threadDisplayPublishedNameCap-1]...)
		hasDisplay := false
		for _, name := range kept {
			hasDisplay = hasDisplay || name.Comm == display
		}
		for _, name := range names[threadDisplayPublishedNameCap-1:] {
			if !hasDisplay && name.Comm == display {
				kept = append(kept, name)
				hasDisplay = true
				break
			}
		}
		if len(kept) < threadDisplayPublishedNameCap {
			kept = append(kept, names[threadDisplayPublishedNameCap-1])
		}
		names = kept
		out.NamesTruncated = true
	}
	out.Names = names
	return out, true
}

// threadLabelWordByte is a byte that can extend a thread name to its left in
// a system label (`<comm>-<tid>`); a recorded name preceded by one of these is
// part of a longer, different token and is never rewritten.
func threadLabelWordByte(c byte) bool {
	return c == '_' || c == '.' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// rewriteLabels re-renders system thread labels (`<comm>-<tid>`, the
// threadLabel grammar) so a recorded name of a unified thread prints as its
// display name. Only exact recorded names of that TID match; the longest
// recorded name that ends at the '-' wins, so a display name that contains an
// alias as its tail is never touched.
func (v *threadDisplayView) rewriteLabels(s string, note func(int)) (string, bool) {
	if v == nil || len(v.ledger.records) == 0 || len(s) < 3 || strings.IndexByte(s, '-') < 0 {
		return s, false
	}
	var b strings.Builder
	last := 0
	changed := false
	for i := 0; i < len(s); i++ {
		if s[i] != '-' || i+1 >= len(s) || s[i+1] < '0' || s[i+1] > '9' {
			continue
		}
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j < len(s) && (s[j] == '.' && j+1 < len(s) && s[j+1] >= '0' && s[j+1] <= '9') {
			i = j - 1
			continue // a decimal number, not a thread number
		}
		tid, err := strconv.Atoi(s[i+1 : j])
		record := v.ledger.record(tid)
		if err != nil || record == nil {
			i = j - 1
			continue
		}
		for _, name := range record.byLength {
			start := i - len(name)
			if start < last || start < 0 || s[start:i] != name {
				continue
			}
			if start > 0 && threadLabelWordByte(s[start-1]) && threadLabelWordByte(name[0]) {
				continue
			}
			display, ok := v.display(tid)
			if !ok {
				break
			}
			if note != nil {
				note(tid)
			}
			if name != display {
				b.WriteString(s[last:start])
				b.WriteString(display)
				last = i
				changed = true
			}
			break
		}
		i = j - 1
	}
	if !changed {
		return s, false
	}
	b.WriteString(s[last:])
	return b.String(), true
}

// threadDisplayRewriter applies the view to one published result without
// mutating any value it may share with an index cache: every changed struct,
// slice, map or pointer target is copied (copy-on-write).
type threadDisplayRewriter struct {
	view *threadDisplayView
	// referenced maps every unified TID the result names to the most
	// prominent face naming it (see threadDisplayFacePriority).
	referenced map[int]int
	priority   int
	skip       map[reflect.Type]bool
}

// threadDisplayFacePriority orders the identity disclosure: the analysed
// thread first, then threads on the causal faces, then the rest, so the
// bounded list never drops a chain member for a background thread.
func threadDisplayFacePriority(field string) int {
	switch field {
	case "Timeline", "TargetWindowStates", "ThreadSelection":
		return 0
	case "RootCauseRank", "WakeupChain", "CriticalBlocking", "FrameRootCauseBundle", "IPCGraph":
		return 1
	default:
		return 2
	}
}

var (
	threadDisplayThreadRefType  = reflect.TypeOf(ThreadRef{})
	threadDisplayCoordinateType = reflect.TypeOf(types.TraceThreadCoordinate{})
	threadDisplayEventType      = reflect.TypeOf(Event{})
	threadDisplayEventViewType  = reflect.TypeOf(EventView{})
	threadDisplayIdentityType   = reflect.TypeOf(ThreadDisplayIdentity{})
)

// threadDisplayLabelFields are the system-minted prose fields that render
// thread labels. Raw record fields (Event / EventView, raw lines) are never
// touched: they quote the capture verbatim.
var threadDisplayLabelFields = map[string]bool{"Summary": true, "Subject": true, "Object": true}

func (w *threadDisplayRewriter) mayContain(t reflect.Type) bool {
	if skip, ok := w.skip[t]; ok {
		return !skip
	}
	// Cycle guard: while a type is being classified it may contain a thread
	// field (over-approximation only costs a walk; it can never skip one).
	w.skip[t] = false
	contains := false
	switch t {
	case threadDisplayThreadRefType, threadDisplayCoordinateType:
		contains = true
	case threadDisplayEventType, threadDisplayEventViewType, threadDisplayIdentityType:
		contains = false
	default:
		switch t.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Map:
			contains = w.mayContain(t.Elem())
		case reflect.Interface:
			contains = true
		case reflect.Struct:
			for i := 0; i < t.NumField() && !contains; i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				if f.Type.Kind() == reflect.String {
					contains = threadDisplayLabelFields[f.Name] || f.Name == "ProcessComm"
					continue
				}
				contains = w.mayContain(f.Type)
			}
		}
	}
	w.skip[t] = !contains
	return contains
}

func (w *threadDisplayRewriter) note(tid int) {
	if w.view.ledger.record(tid) == nil {
		return
	}
	if _, ok := w.view.display(tid); !ok {
		return
	}
	if prior, seen := w.referenced[tid]; !seen || w.priority < prior {
		w.referenced[tid] = w.priority
	}
}

func (w *threadDisplayRewriter) rewrite(v reflect.Value) (reflect.Value, bool) {
	t := v.Type()
	if !w.mayContain(t) {
		return v, false
	}
	switch t {
	case threadDisplayThreadRefType:
		ref := v.Interface().(ThreadRef)
		w.note(ref.PID)
		if comm, ok := w.view.displayComm(ref.PID, ref.Comm); ok {
			ref.Comm = comm
			return reflect.ValueOf(ref), true
		}
		return v, false
	case threadDisplayCoordinateType:
		c := v.Interface().(types.TraceThreadCoordinate)
		w.note(c.TID)
		if comm, ok := w.view.displayComm(c.TID, c.Comm); ok {
			c.Comm = comm
			return reflect.ValueOf(c), true
		}
		return v, false
	}
	switch t.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return v, false
		}
		elem, changed := w.rewrite(v.Elem())
		if !changed {
			return v, false
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(elem)
		return p, true
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		elem, changed := w.rewrite(v.Elem())
		if !changed {
			return v, false
		}
		out := reflect.New(t).Elem()
		out.Set(elem)
		return out, true
	case reflect.Slice:
		var out reflect.Value
		for i := 0; i < v.Len(); i++ {
			elem, changed := w.rewrite(v.Index(i))
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeSlice(t, v.Len(), v.Len())
				reflect.Copy(out, v)
			}
			out.Index(i).Set(elem)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	case reflect.Array:
		out := reflect.New(t).Elem()
		out.Set(v)
		changed := false
		for i := 0; i < v.Len(); i++ {
			if elem, ch := w.rewrite(v.Index(i)); ch {
				out.Index(i).Set(elem)
				changed = true
			}
		}
		return out, changed
	case reflect.Map:
		if v.IsNil() {
			return v, false
		}
		keys := v.MapKeys()
		sort.SliceStable(keys, func(i, j int) bool { return reflectKeyLess(keys[i], keys[j]) })
		var out reflect.Value
		for _, key := range keys {
			elem, changed := w.rewrite(v.MapIndex(key))
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeMapWithSize(t, v.Len())
				iter := v.MapRange()
				for iter.Next() {
					out.SetMapIndex(iter.Key(), iter.Value())
				}
			}
			out.SetMapIndex(key, elem)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	case reflect.Struct:
		var out reflect.Value
		ensure := func() {
			if !out.IsValid() {
				out = reflect.New(t).Elem()
				out.Set(v)
			}
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			field := v.Field(i)
			if f.Type.Kind() == reflect.String {
				switch {
				case threadDisplayLabelFields[f.Name]:
					if s, changed := w.view.rewriteLabels(field.String(), w.note); changed {
						ensure()
						out.Field(i).SetString(s)
					}
				case f.Name == "ProcessComm":
					if s, changed := w.processComm(v, field.String()); changed {
						ensure()
						out.Field(i).SetString(s)
					}
				}
				continue
			}
			if elem, changed := w.rewrite(field); changed {
				ensure()
				out.Field(i).Set(elem)
			}
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	}
	return v, false
}

// processComm renames a process comm published beside a thread: a process is
// named by its main thread (TID == TGID), so it takes that thread's display.
func (w *threadDisplayRewriter) processComm(owner reflect.Value, comm string) (string, bool) {
	thread := owner.FieldByName("Thread")
	if !thread.IsValid() || thread.Type() != threadDisplayThreadRefType {
		return comm, false
	}
	tgid := thread.Interface().(ThreadRef).TGID
	if tgid <= 0 {
		return comm, false
	}
	return w.view.displayComm(tgid, comm)
}

func reflectKeyLess(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.String:
		return a.String() < b.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() < b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return a.Uint() < b.Uint()
	default:
		return false
	}
}

// publishThreadDisplayIdentity is the single publication point: every typed
// thread field and every system label of the result names a unified thread by
// its display name for the result's window, and the result lists the identity
// of each such thread it references once (analysed thread first, then the
// causal faces, then the rest; TID ascending inside a class; bounded).
func publishThreadDisplayIdentity(idx *Index, q Query, res Result) Result {
	ledger := ensureThreadDisplayLedger(idx)
	if ledger == nil || len(ledger.records) == 0 {
		return res
	}
	view := newThreadDisplayView(ledger, q.TimeStart, q.TimeEnd)
	w := &threadDisplayRewriter{view: view, referenced: map[int]int{}, skip: map[reflect.Type]bool{}}
	// Walk the result's own fields one face at a time (copy-on-write per
	// field) so each named thread records the most prominent face naming it.
	out := reflect.New(reflect.TypeOf(res)).Elem()
	out.Set(reflect.ValueOf(res))
	t := out.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" || f.Type.Kind() == reflect.String {
			continue
		}
		w.priority = threadDisplayFacePriority(f.Name)
		if elem, changed := w.rewrite(out.Field(i)); changed {
			out.Field(i).Set(elem)
		}
	}
	res = out.Interface().(Result)
	if native := res.nativeTargetTimeline; native != nil {
		w.priority = 0
		if rewritten, changed := w.rewrite(reflect.ValueOf(native)); changed {
			res.nativeTargetTimeline = rewritten.Interface().(*TimelineResult)
		}
	}
	if len(w.referenced) == 0 {
		return res
	}
	tids := make([]int, 0, len(w.referenced))
	for tid := range w.referenced {
		tids = append(tids, tid)
	}
	sort.Slice(tids, func(i, j int) bool {
		if w.referenced[tids[i]] != w.referenced[tids[j]] {
			return w.referenced[tids[i]] < w.referenced[tids[j]]
		}
		return tids[i] < tids[j]
	})
	if len(tids) > threadDisplayIdentityCap {
		tids = tids[:threadDisplayIdentityCap]
	}
	res.ThreadIdentities = nil
	for _, tid := range tids {
		if identity, ok := view.published(tid); ok {
			res.ThreadIdentities = append(res.ThreadIdentities, identity)
		}
	}
	return res
}

// threadCreationCaveat describes one creation record of the analysed thread.
// Only a creation record the lifecycle audit recorded as a reuse of an
// already-seen TID is a generation boundary; the first creation record of a
// TID starts its first lifetime and is reported as a creation, never as a
// reset or a second identity.
func threadCreationCaveat(idx *Index, ev Event) string {
	if threadCreationIsRecordedReuse(idx, ev) {
		return fmt.Sprintf("thread_generation_boundary=true; sched_wakeup_new reset tid=%d at line=%d", ev.WakeePID, ev.Line)
	}
	return fmt.Sprintf("thread created at %.6f s (line %d): TID %d starts its first lifetime here; the wait from creation to its first run is measured ready time", ev.Ts, ev.Line, ev.WakeePID)
}

// threadCreationIsRecordedReuse decides from lifecycle records only. A
// creation is proven to be a first lifetime only where the deciding audit saw
// every earlier row of the capture (or its checkpoint): a complete index's
// lifecycle scan, the complete parent of a derived window, or the streamed
// audit of a windowed parse. Anything else keeps the boundary disclosure.
func threadCreationIsRecordedReuse(idx *Index, ev Event) bool {
	if idx == nil || ev.WakeePID <= 0 {
		return true
	}
	key := threadCreationLine{tid: ev.WakeePID, line: ev.Line}
	if idx.threadCreationReuses[key] || idx.threadCreationReusesCapped {
		return true
	}
	boundaries, capped := ensureThreadGenerationMetadata(idx)
	if capped {
		return true
	}
	for _, boundary := range boundaries[ev.WakeePID] {
		if boundary.Signal == "sched_wakeup_new" && boundary.BoundaryLine == ev.Line {
			return true
		}
	}
	if inherited := idx.threadDisplayInherited; inherited != nil {
		return inherited.lifecycleCapped || inherited.reuseCreations[key]
	}
	if len(idx.TraceArtifacts) > 1 || idx.RelationScoped && !idx.Windowed {
		return true
	}
	return false
}
