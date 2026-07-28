package tracequery

import (
	"bufio"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	ftraceLineRE = regexp.MustCompile(`^\s*(.+)-(\d+)(?:\s+\(\s*([0-9-]+)\))?\s+\[(\d+)\]\s+\S+\s+([0-9]+(?:\.[0-9]+)?):\s+([A-Za-z0-9_./:-]+):?\s*(.*)$`)
	// Ambiguous rows are resolved by enumerating only -PID delimiters inside
	// the canonical 15-rune comm prefix. Once the delimiter is fixed, these
	// tail grammars cannot cross the outer event column into body text.
	ftraceStrictCanonicalPIDTailRE = regexp.MustCompile(`^-(\d+)(?:\s+\(\s*([0-9-]+)\))?\s+\[(\d+)\]\s+\S+\s+([0-9]+(?:\.[0-9]+)?):\s+([A-Za-z0-9_./:-]+):?\s*(.*)$`)
	ftraceLooseCanonicalPIDTailRE  = regexp.MustCompile(`^-(\d+)(?:\s+\(\s*([^\s()]+)\s*\))?\s+\[([^\[\]\r\n]+?)\]?\s+\S+\s+([^\s:]+):\s+([^\s:]+):\s*(.*)$`)
	ftraceLooseAnyPIDTailRE        = regexp.MustCompile(`^-([^\s]+)(?:\s+\(\s*([^\s()]+)\s*\))?\s+\[([^\[\]\r\n]+?)\]?\s+\S+\s+([^\s:]+):\s+([^\s:]+):\s*(.*)$`)
	// Missing PID keeps only family/header provenance. The bracket candidate
	// must still follow a non-empty canonical-length comm.
	ftraceLooseMissingPIDTailRE = regexp.MustCompile(`^\[([^\[\]\r\n]+?)\]?\s+\S+\s+([^\s:]+):\s+([^\s:]+):\s*(.*)$`)
	kvRE                        = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*"|'[^']*'|[^ ]+)`)
	blockRQIssueRE              = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\([^\r\n)]*\)\s+(\d+)\s+\+\s+(\d+)\s+\[[^\r\n\]]*\]\s*$`)
	blockRQCompleteRE           = regexp.MustCompile(`^(\S+)\s+(\S+)\s+\([^\r\n)]*\)\s+(\d+)\s+\+\s+(\d+)\s+\[(-?\d+)\]\s*$`)
	blockBioQueueRE             = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)\s+\[[^\r\n\]]*\]\s*$`)
	blockBioCompleteRE          = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)\s+\[(-?\d+)\]\s*$`)
	blockSimpleLegacyRE         = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)\s+\[[^\r\n\]]*\]\s*$`)
	blockRQRemapRE              = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)\s+<-\s+\(([^\r\n)]+)\)\s+(\d+)\s+(\d+)\s*$`)
	blockBioRemapRE             = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)\s+<-\s+\(([^\r\n)]+)\)\s+(\d+)\s*$`)
	blockRemapLegacyRE          = regexp.MustCompile(`^(\S+)\s+(\d+)\s+\+\s+(\d+)\s+<-\s+\(([^\r\n)]+)\)\s+(\d+)\s*$`)
	blockErrorRE                = regexp.MustCompile(`\[([^\]]+)\]\s*$`)
)

var spaceKVKeys = map[string]struct{}{
	"addr": {}, "address": {}, "affinity": {}, "allowed_cpus": {}, "aux_size": {}, "branch_count": {}, "bytes": {}, "callchain": {}, "cg": {}, "cgroup": {}, "cgroup_id": {}, "clock": {}, "cmdline": {}, "code_page_size": {}, "comm": {}, "comm_source": {}, "cpu": {}, "cpumask": {}, "cpus": {}, "cpus_allowed": {}, "cpuset": {}, "data_page_size": {}, "data_src": {}, "dev": {}, "dest_cpu": {}, "dso": {}, "entry_name": {}, "event": {},
	"file": {}, "filename": {}, "i_blocks": {}, "i_mode": {}, "i_nlink": {}, "i_size": {}, "event_period": {},
	"duration": {}, "duration_ms": {}, "duration_ns": {}, "duration_us": {},
	"ino": {}, "inode": {}, "ip": {}, "latency": {}, "latency_ms": {}, "latency_ns": {}, "latency_us": {}, "len": {}, "length": {}, "name": {}, "offset": {}, "ofs": {},
	"mask": {}, "operation": {}, "op": {}, "orig_cpu": {}, "parent": {}, "parent_ino": {}, "parent_inode": {}, "path": {}, "perf_thread_comm": {}, "perf_weight": {}, "period": {}, "period_weight": {}, "phys_addr": {}, "pid": {}, "pino": {},
	"policy": {}, "pos": {}, "raw_size": {}, "reason": {}, "ret": {}, "rw": {}, "rwbs": {}, "sample_id": {}, "sample_period": {}, "sample_weight": {}, "size": {}, "source": {}, "stream_id": {}, "symbol": {}, "target_comm": {}, "target_cpu": {}, "target_pid": {}, "task": {}, "task_pid": {}, "thread_comm": {}, "tid": {}, "transaction": {}, "type": {}, "user_regs_abi": {}, "user_regs_count": {}, "user_stack_size": {},
}

type parseCacheKey struct {
	path      string
	size      int64
	modUnix   int64
	version   string
	windowKey string
	// sourceKey fingerprints every physical artifact in a composite source.
	// Entry-path stat alone is insufficient: a sibling perftrace or a bundle
	// child can change while the systrace/manifest remains byte-identical.
	sourceKey string
}

const maxCachedTraceIndexBytes int64 = 64 << 20

// defaultTraceIndexMaxEvents bounds a single in-memory query index. Event is
// intentionally rich and large; dense trace windows must be split before the
// append backing array grows into process-wide OOM territory, especially when
// the LLM issues multiple trace_query calls in parallel.
const defaultTraceIndexMaxEvents = 250000

// traceIndexCacheBudgetBytes bounds the total Event bytes retained by the
// index cache. Fixed package constant by design — no configuration knob:
// eviction is pure latency (a miss re-parses through the indexBuilds
// singleflight), never a correctness or gating signal.
const traceIndexCacheBudgetBytes int64 = 512 << 20

// eventSizeBytes is the in-memory cost of one parsed Event, computed once at
// compile time and used only for cache accounting.
const eventSizeBytes = int64(unsafe.Sizeof(Event{}))

// eventSideTableBytes is the struct cost of the kind-specific side tables
// (P4) hanging off one event — charged into Index.RetainedSideTableBytes so
// the cache keeps honest accounting after the Event core shrank. Struct
// bytes only; side-table strings are interned and already counted by
// RetainedStringBytes.
func eventSideTableBytes(ev *Event) int64 {
	var n int64
	if ev.ConstraintFields != nil {
		n += int64(unsafe.Sizeof(ConstraintFields{}))
	}
	if ev.SchedStatFields != nil {
		n += int64(unsafe.Sizeof(SchedStatFields{}))
	}
	if ev.BinderFields != nil {
		n += int64(unsafe.Sizeof(BinderFields{}))
	}
	if ev.BlockIOFields != nil {
		n += int64(unsafe.Sizeof(BlockIOFields{}))
	}
	if ev.ResourceFields != nil {
		n += int64(unsafe.Sizeof(ResourceFields{}))
		if ev.ResourceFields.mmcPairing != nil {
			n += int64(unsafe.Sizeof(mmcPairingAdmission{}))
		}
		if ev.ResourceFields.f2fsPairing != nil {
			n += int64(unsafe.Sizeof(f2fsPairingAdmission{}))
		}
	}
	if ev.FileFields != nil {
		n += int64(unsafe.Sizeof(FileFields{}))
	}
	if ev.PluginFields != nil {
		n += int64(unsafe.Sizeof(PluginFields{}))
		if ev.PluginFields.Counter != nil {
			n += int64(unsafe.Sizeof(TraceCounterFields{}))
		}
		if ev.PluginFields.FrameMap != nil {
			n += int64(unsafe.Sizeof(FrameMapFields{}))
		}
	}
	if ev.PerfFields != nil {
		n += int64(unsafe.Sizeof(PerfFields{}))
	}
	return n
}

type traceIndexCacheEntry struct {
	key  parseCacheKey
	idx  *Index
	cost int64
}

// traceIndexCache is a byte-budgeted LRU keyed by parseCacheKey. Each entry
// is charged len(Events)*eventSizeBytes. Inserting past the budget evicts
// least-recently-used entries; loads refresh recency. An entry larger than
// the entire budget is not cached at all.
type traceIndexCache struct {
	mu     sync.Mutex
	budget int64
	used   int64
	order  *list.List // front = most recently used
	items  map[parseCacheKey]*list.Element
}

var indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)

func newTraceIndexCache(budget int64) *traceIndexCache {
	return &traceIndexCache{
		budget: budget,
		order:  list.New(),
		items:  map[parseCacheKey]*list.Element{},
	}
}

func traceIndexCacheCost(idx *Index) int64 {
	if idx == nil {
		return 0
	}
	idx.schedulerHeadMu.Lock()
	headBytes := idx.schedulerHeadBytes
	idx.schedulerHeadMu.Unlock()
	// Struct cost + the ACTUAL retained string bytes (P2, 2026-07-03):
	// unsafe.Sizeof counts only string headers, so payload-heavy traces
	// used to under-charge the LRU by up to 2x and hold more real
	// memory than the 512 MiB budget promised. Side-table struct bytes
	// (P4) ride the same lane.
	// A bounded index also owns its one correctness-critical window-head
	// scheduler checkpoint; charge it to the same global LRU budget.
	durationAuditBytes := int64(len(idx.durationOrderFailures)) * int64(unsafe.Sizeof(durationOrderViolation{}))
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		durationAuditBytes += int64(len(failure.Lane) + len(failure.LaneKey) + len(failure.Issue) + len(failure.EventName) + len(failure.SourcePath))
		for _, field := range failure.Fields {
			durationAuditBytes += int64(len(field))
		}
	}
	cpuInputAuditBytes := int64(len(idx.cpuInputIntegrityFailures)) * int64(unsafe.Sizeof(cpuInputIntegrityFailure{}))
	for i := range idx.cpuInputIntegrityFailures {
		failure := &idx.cpuInputIntegrityFailures[i]
		cpuInputAuditBytes += int64(len(failure.EventName) + len(failure.Field) + len(failure.Raw) + len(failure.ReasonCode) + len(failure.SourcePath))
	}
	traceMarkAuditBytes := int64(len(idx.traceMarkIntegrityFailures)) * int64(unsafe.Sizeof(traceMarkIntegrityFailure{}))
	for i := range idx.traceMarkIntegrityFailures {
		failure := &idx.traceMarkIntegrityFailures[i]
		traceMarkAuditBytes += int64(len(failure.Action) + len(failure.Reason) + len(failure.SourcePath))
	}
	schedulerRowAuditBytes := int64(len(idx.schedulerRowIntegrityFailures)) * int64(unsafe.Sizeof(schedulerRowIntegrityFailure{}))
	for i := range idx.schedulerRowIntegrityFailures {
		failure := &idx.schedulerRowIntegrityFailures[i]
		schedulerRowAuditBytes += int64(len(failure.EventName)+len(failure.SourcePath)) +
			int64(len(failure.PIDs))*int64(unsafe.Sizeof(int(0)))
		for _, field := range failure.Fields {
			schedulerRowAuditBytes += int64(len(field))
		}
	}
	schedulerRowAuditBytes += int64(len(idx.schedulerRowIntegrityOverflowSources)) * int64(unsafe.Sizeof(""))
	for _, sourcePath := range idx.schedulerRowIntegrityOverflowSources {
		schedulerRowAuditBytes += int64(len(sourcePath))
	}
	schedulerRowAuditBytes += int64(len(idx.priorityMutationIntegrityOverflowSources)) * int64(unsafe.Sizeof(""))
	for _, sourcePath := range idx.priorityMutationIntegrityOverflowSources {
		schedulerRowAuditBytes += int64(len(sourcePath))
	}
	blockedReasonAuditBytes := int64(len(idx.blockedReasonIntegrityFailures)) * int64(unsafe.Sizeof(blockedReasonIntegrityFailure{}))
	for i := range idx.blockedReasonIntegrityFailures {
		failure := &idx.blockedReasonIntegrityFailures[i]
		blockedReasonAuditBytes += int64(len(failure.SourcePath)) +
			int64(len(failure.PIDs))*int64(unsafe.Sizeof(int(0)))
		for _, field := range failure.Fields {
			blockedReasonAuditBytes += int64(len(field))
		}
	}
	blockedReasonAuditBytes += int64(len(idx.blockedReasonIntegrityOverflow.PIDs)) * int64(unsafe.Sizeof(int(0)))
	blockedReasonAuditBytes += int64(len(idx.blockedReasonIdentityOverflow.PIDs)) * int64(unsafe.Sizeof(int(0)))
	blockedReasonAuditBytes += int64(len(idx.blockedReasonIntegrityOverflow.PIDDomains)+len(idx.blockedReasonIdentityOverflow.PIDDomains)) * int64(unsafe.Sizeof(blockedReasonIntegrityPIDDomain{}))
	var caveatBytes int64
	for _, caveat := range idx.Caveats {
		caveatBytes += int64(len(caveat))
	}
	// The perf identity ledger is lazy, but its worst-case retained cohort /
	// binding / selector authority must already be charged while this Index is
	// admitted to the global LRU. Otherwise a cached dense profile can allocate
	// a second unbudgeted index-sized structure on its first query.
	perfIdentityReserve := int64(0)
	for i := range idx.Events {
		if idx.Events[i].Type == EventPerfSample {
			perfIdentityReserve += perfIdentityLedgerReservedBytesPerSample
		}
	}
	return int64(len(idx.Events))*eventSizeBytes + idx.RetainedStringBytes + idx.RetainedSideTableBytes + headBytes + durationAuditBytes + cpuInputAuditBytes + traceMarkAuditBytes + schedulerRowAuditBytes + blockedReasonAuditBytes + caveatBytes + perfIdentityReserve
}

func (c *traceIndexCache) Load(key parseCacheKey) (*Index, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*traceIndexCacheEntry).idx, true
}

func (c *traceIndexCache) Store(key parseCacheKey, idx *Index) {
	cost := traceIndexCacheCost(idx)
	if c == nil || cost <= 0 || cost > c.budget {
		// An entry larger than the entire cache budget is served to its caller
		// but never made process-lifetime resident.
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		entry := el.Value.(*traceIndexCacheEntry)
		c.used += cost - entry.cost
		entry.idx = idx
		entry.cost = cost
		c.order.MoveToFront(el)
	} else {
		c.items[key] = c.order.PushFront(&traceIndexCacheEntry{key: key, idx: idx, cost: cost})
		c.used += cost
	}
	for c.used > c.budget && c.order.Len() > 1 {
		oldest := c.order.Back()
		entry := oldest.Value.(*traceIndexCacheEntry)
		c.order.Remove(oldest)
		delete(c.items, entry.key)
		c.used -= entry.cost
	}
}

func (c *traceIndexCache) Delete(key parseCacheKey) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return
	}
	entry := el.Value.(*traceIndexCacheEntry)
	c.order.Remove(el)
	delete(c.items, key)
	c.used -= entry.cost
}

type indexBuildCall struct {
	done chan struct{}
	idx  *Index
	err  error
}

var (
	indexBuildMu sync.Mutex
	indexBuilds  = map[parseCacheKey]*indexBuildCall{}
)

type BuildOptions struct {
	TimeStart          float64
	TimeEnd            float64
	TimeStartSet       bool
	TimeEndSet         bool
	TimePaddingBefore  float64
	TimePaddingAfter   float64
	LineStart          int
	LineEnd            int
	LinePaddingBefore  int
	LinePaddingAfter   int
	AllowWindowedParse bool
	MaxEvents          int
	// ScopePID / ScopeThread record that this windowed index was built for an
	// explicitly pid/thread-scoped heavy-view query. They do NOT prune the
	// index (Step 1 keeps a full, unpruned window so every view stays
	// correct); they only (a) let the caller raise MaxEvents from a byte
	// budget for a single deliberate scoped call, and (b) let
	// IndexEventLimitError tailor its next-step guidance — a request that
	// already pinned pid/thread cannot "narrow with pid/thread" further, so
	// the density message must steer toward splitting the window instead.
	ScopePID    int
	ScopeThread string
	// RelationScoped (Gap 3 Step 2) opts this windowed index into
	// pid-relation pruning: only events touching the target pid, its
	// scheduling wakers (transitive, to ScopeMaxDepth), and all binder rows
	// are retained. This is provably complete ONLY for the causal-chain views
	// (thread_timeline / wakeup_chain) — it MUST NOT be set for
	// root_cause_rank / frame_root_cause_bundle / window_stats, which consume
	// whole-window × all-thread aggregates (CPU pressure from unrelated
	// same-CPU threads, all-pid trace-mark frame spans, global IO/clock/power)
	// that pruning would silently drop. The pruned index is a strict subset of
	// the full window (only removes events), so it can never make a view worse
	// than the Step-1 byte-budget path; it just lets those two views run on a
	// far larger window. PID scopes seed the closure directly. Thread-only
	// scopes are allowed only after discoverRelationScope resolves the typed
	// thread selector to a single pid/tgid inside the selected window; ambiguous
	// selectors degrade to an unpruned index plus caveat.
	RelationScoped bool
	// ScopeMaxDepth bounds the transitive waker-closure BFS in pass-1. It must
	// be >= the query's wakeup-chain MaxDepth (default 10) plus a buffer so the
	// discovered waker set is a superset of what expandChain actually
	// traverses; a short closure would prune a deep waker's events and break
	// the chain. Zero falls back to defaultRelationScopeMaxDepth.
	ScopeMaxDepth int
}

// indexPaddingTruncatedNoteFmt renders the display note for a windowed build
// whose event budget tripped only inside the safety padding tail — the
// requested window itself is fully parsed and the result stays consumable.
// The single %.6f verb carries the real parse boundary (idx.LastTs at the
// trigger point) so the note states HOW FAR parsing actually got instead of a
// fixed claim (QF5); Index.PaddingTruncatedNote stores the formatted string
// and display surfaces fold it verbatim. The same boundary is published as
// the typed Index.PaddingTruncatedLastTs for query-layer caveats.
const indexPaddingTruncatedNoteFmt = "index budget hit after request window fully parsed (parsed through ts=%.6f); padding tail truncated"

type IndexEventLimitError struct {
	Path           string
	MaxEvents      int
	Events         int
	Line           int
	ScannedLines   int
	Windowed       bool
	IndexTimeStart float64
	IndexTimeEnd   float64
	IndexLineStart int
	IndexLineEnd   int
	FirstTs        float64
	LastTs         float64
	ScopePID       int
	ScopeThread    string
	// RequestTimeStart/RequestTimeEnd carry the caller's UN-padded request
	// window (BuildOptions.TimeStart/TimeEnd); RequestWindowSet is true only
	// when BOTH bounds were explicitly set. Precise signals for the
	// window_sweep-first recovery steer (§4.7): only an explicit request
	// window strictly longer than WindowSweepRecoveryMinWindowSeconds gets
	// it — the padded IndexTimeStart/End must NOT be used here, padding
	// alone (±0.5s) would push sub-second requests over the 1s threshold.
	RequestTimeStart float64
	RequestTimeEnd   float64
	RequestWindowSet bool
}

func (e *IndexEventLimitError) Error() string {
	if e == nil {
		return "trace index event limit reached"
	}
	// A request that already pinned pid/thread cannot "narrow with pid/thread"
	// any further — repeating that suggestion sends the model in circles (and,
	// per Gap 2, tempts it to DROP the pinned pid and scan the whole trace).
	// When the index was built scoped, steer purely toward splitting the window
	// / adding line bounds; keep the pid/thread hint only for unscoped calls.
	nextStep := "split the time window, add line_start/line_end, or narrow with pid/thread/event_types/event_search before running heavy views"
	if e.ScopePID > 0 || strings.TrimSpace(e.ScopeThread) != "" {
		nextStep = "the pinned pid/thread scope is already applied and cannot narrow this further; split the time window into sub-windows (e.g. 80-150ms coverage windows) or add line_start/line_end before rerunning heavy views — do NOT drop the pinned pid/thread to widen the scan"
	}
	return fmt.Sprintf(
		"trace index event limit reached: path=%s parsed_events=%d max_events=%d line=%d scanned_lines=%d windowed=%t index_time=%.6f..%.6f index_lines=%d..%d parsed_time=%.6f..%.6f scope_pid=%d scope_thread=%s; selected trace scope is too dense for a single in-memory index; %s%s",
		e.Path,
		e.Events,
		e.MaxEvents,
		e.Line,
		e.ScannedLines,
		e.Windowed,
		e.IndexTimeStart,
		e.IndexTimeEnd,
		e.IndexLineStart,
		e.IndexLineEnd,
		e.FirstTs,
		e.LastTs,
		e.ScopePID,
		strings.TrimSpace(e.ScopeThread),
		nextStep,
		e.RecoveryParams(),
	)
}

// RecoveryParams renders concrete, copy-pastable retry parameters (§7.30.2 C3):
// the budget-capped index already covered a first segment (FirstTs..LastTs), so
// the message names that exact narrower window, plus the streaming event_search
// escape hatch that does not depend on the in-memory index budget at all. A
// budget hit must never strand the model without an executable next call.
func (e *IndexEventLimitError) RecoveryParams() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	// The escape-hatch view names are interpolated from the capacity table's
	// shared tokens so this pinned sentence and the tool-side recovery
	// surfaces can never drift apart (rendered text stays byte-identical).
	// §4.7 W3: an explicit request window strictly longer than
	// WindowSweepRecoveryMinWindowSeconds steers to window_sweep FIRST — one
	// streaming coverage pass over the SAME window replaces blind window
	// bisection. Shorter/unset windows keep the historical text unchanged.
	if e.RequestWindowSet && e.RequestTimeEnd-e.RequestTimeStart > WindowSweepRecoveryMinWindowSeconds {
		fmt.Fprintf(&b, "; recovery_params: the requested window spans %.3fs; run view=%s FIRST with the SAME time_start/time_end (streaming per-bucket coverage scan, NOT subject to this index event budget) to rank dense sub-windows before narrowing, then use view=%s (streaming scan, NOT subject to this index event budget) to locate exact tokens/lines",
			e.RequestTimeEnd-e.RequestTimeStart, ViewWindowSweep, FallbackViewEventSearch)
	} else {
		fmt.Fprintf(&b, "; recovery_params: use view=%s (streaming scan, NOT subject to this index event budget) to locate exact tokens/lines first", FallbackViewEventSearch)
	}
	if e.LastTs > e.FirstTs && e.FirstTs > 0 {
		fmt.Fprintf(&b, ", or rerun with time_start=%.6f time_end=%.6f (the first window segment this index already covered before hitting the budget)", e.FirstTs, e.LastTs)
	}
	if e.ScopePID <= 0 && strings.TrimSpace(e.ScopeThread) == "" {
		b.WriteString(", or add pid=<target pid> to scope the index")
	}
	return b.String()
}

func newIndexEventLimitError(path string, idx *Index, opts BuildOptions, line, events int) *IndexEventLimitError {
	err := &IndexEventLimitError{
		Path:             path,
		MaxEvents:        opts.MaxEvents,
		Events:           events,
		Line:             line,
		ScopePID:         opts.ScopePID,
		ScopeThread:      opts.ScopeThread,
		RequestTimeStart: opts.TimeStart,
		RequestTimeEnd:   opts.TimeEnd,
		RequestWindowSet: opts.TimeStartSet && opts.TimeEndSet,
	}
	if idx != nil {
		err.ScannedLines = idx.ScannedLineCount
		err.Windowed = idx.Windowed
		err.IndexTimeStart = idx.IndexTimeStart
		err.IndexTimeEnd = idx.IndexTimeEnd
		err.IndexLineStart = idx.IndexLineStart
		err.IndexLineEnd = idx.IndexLineEnd
		err.FirstTs = idx.FirstTs
		err.LastTs = idx.LastTs
	}
	return err
}

func BuildIndex(ctx context.Context, path string) (*Index, error) {
	return BuildIndexWithOptions(ctx, path, BuildOptions{})
}

type traceIndexBuildPhase uint8

const (
	traceIndexPhaseSelectionFrozen traceIndexBuildPhase = iota + 1
	traceIndexPhaseBeforeSchedulerHead
	traceIndexPhaseBeforeUniverseValidation
	traceIndexPhaseSingleflightJoined
)

type traceIndexBuildObserver func(traceIndexBuildPhase, parseCacheKey)

func BuildIndexWithOptions(ctx context.Context, path string, opts BuildOptions) (*Index, error) {
	return buildIndexWithObserver(ctx, path, opts, nil)
}

func buildIndexWithObserver(ctx context.Context, path string, opts BuildOptions, observer traceIndexBuildObserver) (idx *Index, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trace path is empty")
	}
	preCanceled := ctx.Err()
	selectionCtx := ctx
	if preCanceled != nil {
		// A warm cached index is still useful to the tool layer: it mints the
		// typed canceled-partial result without publishing any query faces. Do
		// the bounded source-generation lookup even for an already-dead context,
		// but never parse a cache miss below.
		selectionCtx = context.WithoutCancel(ctx)
	}
	selection, err := resolveTraceIndexSelectionWithPolicy(selectionCtx, path, preCanceled == nil)
	if err != nil {
		if preCanceled != nil {
			return nil, preCanceled
		}
		return nil, err
	}
	defer func() {
		if closeErr := selection.close(); closeErr != nil {
			idx = nil
			err = errors.Join(err, closeErr)
		}
	}()
	path = selection.indexPath
	sourceBytes := selection.universe.totalBytes
	sourceKey := selection.universe.cacheToken
	size := selection.indexIdentity.Size()
	modUnix := selection.indexIdentity.ModUnixNano()
	opts = normalizeBuildOptions(opts)
	windowKey := opts.cacheKey()
	cacheable := shouldCacheTraceIndex(sourceBytes, opts)
	key := parseCacheKey{
		path:      path,
		size:      size,
		modUnix:   modUnix,
		version:   ParserVersion,
		windowKey: windowKey,
		sourceKey: sourceKey,
	}
	if observer != nil {
		observer(traceIndexPhaseSelectionFrozen, key)
	}
	if cacheable {
		if idx, ok := indexCache.Load(key); ok {
			if err := selection.finish(context.WithoutCancel(ctx), idx); err != nil {
				indexCache.Delete(key)
				return nil, err
			}
			return idx, nil
		}
	}
	// Exact cache hits above remain usable for typed canceled-partial results,
	// but a cache miss must not derive or build any new index after the caller
	// has canceled. In particular, a warm full index is not authority to run a
	// fresh window perf-ledger build under context.WithoutCancel.
	if err := ctx.Err(); err != nil {
		return nil, selection.closeAfter(err)
	}
	// A relation-scoped index must be built by pruning during the streamed
	// parse; deriving it from a cached FULL index would hand back the unpruned
	// window (correct data, but defeats the memory goal and misses the point).
	// Only non-scoped windowed indices reuse the full-cache derive fast path.
	if opts.windowed() && !opts.relationScoped() {
		fullKey := parseCacheKey{
			path:      path,
			size:      size,
			modUnix:   modUnix,
			version:   ParserVersion,
			sourceKey: sourceKey,
		}
		if idx, ok := indexCache.Load(fullKey); ok {
			if err := selection.validate(context.WithoutCancel(ctx)); err != nil {
				indexCache.Delete(fullKey)
				return nil, selection.closeAfter(err)
			}
			derived := deriveWindowedIndex(idx, opts)
			auditQ := Query{TimeStart: derived.IndexTimeStart, TimeEnd: derived.IndexTimeEnd, LineStart: derived.IndexLineStart, LineEnd: derived.IndexLineEnd}
			derived.schedulerOrderFailures = nil
			derived.durationOrderFailures = nil
			derived.durationOrderFailuresCapped = nil
			derived.schedulerRowIntegrityFailures = nil
			derived.priorityMutationIntegrityFailuresCapped = false
			derived.priorityMutationIntegrityOverflowSources = nil
			derived.priorityMutationIntegrityOverflowGlobal = false
			derived.blockedReasonIntegrityFailures = nil
			derived.blockedReasonIntegrityFailuresCapped = false
			derived.blockedReasonIntegrityOverflow = blockedReasonIntegrityOverflowScope{}
			derived.blockedReasonIdentityOverflow = blockedReasonIntegrityOverflowScope{}
			derived.cpuInputIntegrityFailures = nil
			derived.traceMarkIntegrityFailures = nil
			derived.traceMarkIntegrityDroppedGlobalPoison = idx.traceMarkIntegrityDroppedGlobalPoison
			derived.traceTrackIntegrityDroppedPoison = idx.traceTrackIntegrityDroppedPoison
			derived.threadIncarnationFailures = nil
			for _, failure := range idx.schedulerRowIntegrityFailures {
				if schedulerRowIntegrityFailureRelevantToQuery(&failure, auditQ, 0) {
					appendSchedulerRowIntegrityFailure(derived, failure)
				}
			}
			derived.schedulerRowIntegrityFailuresCapped = derived.schedulerRowIntegrityFailuresCapped || idx.schedulerRowIntegrityFailuresCapped
			derived.priorityMutationIntegrityFailuresCapped = derived.priorityMutationIntegrityFailuresCapped || idx.priorityMutationIntegrityFailuresCapped
			derived.priorityMutationIntegrityOverflowSources = append([]string(nil), idx.priorityMutationIntegrityOverflowSources...)
			derived.priorityMutationIntegrityOverflowGlobal = idx.priorityMutationIntegrityOverflowGlobal
			for _, failure := range idx.blockedReasonIntegrityFailures {
				if blockedReasonIntegrityFailureRelevantToQuery(&failure, auditQ, 0) {
					appendBlockedReasonIntegrityFailure(derived, failure)
				}
			}
			if idx.blockedReasonIntegrityFailuresCapped && blockedReasonIntegrityOverflowRelevantToQuery(idx.blockedReasonIntegrityOverflow, auditQ, 0) {
				derived.blockedReasonIntegrityFailuresCapped = true
				derived.blockedReasonIntegrityOverflow = idx.blockedReasonIntegrityOverflow.clone()
				if blockedReasonIntegrityOverflowRelevantToQuery(idx.blockedReasonIdentityOverflow, auditQ, 0) {
					derived.blockedReasonIdentityOverflow = idx.blockedReasonIdentityOverflow.clone()
				}
			}
			for _, failure := range idx.cpuInputIntegrityFailures {
				if cpuInputIntegrityFailureRelevantToQuery(failure, auditQ) {
					appendCPUInputIntegrityFailure(derived, failure)
				}
			}
			derived.cpuInputIntegrityFailuresCapped = derived.cpuInputIntegrityFailuresCapped || idx.cpuInputIntegrityFailuresCapped
			for _, failure := range idx.traceMarkIntegrityFailures {
				if traceMarkIntegrityFailureRelevantToQuery(failure, auditQ) {
					appendTraceMarkIntegrityFailure(derived, failure)
				}
			}
			derived.traceMarkIntegrityFailuresCapped = derived.traceMarkIntegrityFailuresCapped || idx.traceMarkIntegrityFailuresCapped
			// parseTraceArtifactSpecs canonically sorts a bundle/composite after
			// preserving each child's physical-order poison. Re-auditing that
			// sorted slice would erase the exact rollback/lifecycle proof. Filter
			// the preserved records instead; only a physical single-file index may
			// derive fresh audit records from idx.Events.
			canonicalComposite := traceBundlePath(idx.Path) || len(idx.TraceArtifacts) > 1
			if canonicalComposite {
				for _, failure := range idx.schedulerOrderFailures {
					if schedulerOrderViolationRelevantToQuery(&failure, auditQ, 0) {
						derived.schedulerOrderFailures = append(derived.schedulerOrderFailures, failure)
					}
				}
				for _, failure := range idx.threadIncarnationFailures {
					if incarnationBoundaryInsideQuery(&failure, auditQ) {
						derived.threadIncarnationFailures, derived.threadIncarnationFailuresCapped = mergeThreadIncarnationFailures(
							derived.threadIncarnationFailures, derived.threadIncarnationFailuresCapped,
							[]threadIncarnationConflict{failure}, false, threadIncarnationFailureCap)
					}
				}
				derived.schedulerOrderFailuresCapped = idx.schedulerOrderFailuresCapped
				for _, failure := range idx.durationOrderFailures {
					if durationOrderViolationRelevantToQuery(&failure, auditQ) {
						appendDurationOrderFailure(derived, failure)
					}
				}
				for family, capped := range idx.durationOrderFailuresCapped {
					if capped {
						if derived.durationOrderFailuresCapped == nil {
							derived.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
						}
						derived.durationOrderFailuresCapped[family] = true
					}
				}
				derived.threadIncarnationFailuresCapped = derived.threadIncarnationFailuresCapped || idx.threadIncarnationFailuresCapped
				// A child-local audit cannot see an old TID in artifact A followed by
				// sched_wakeup_new in artifact B. Re-audit the canonical full-event
				// timeline and union that cross-artifact proof with the preserved
				// physical-order poison above. Never replace the physical proofs:
				// canonical sorting can legitimately erase a child rollback/reuse.
				mergedFailures, mergedCapped := threadIncarnationConflictsFromEvents(idx.Events, auditQ, threadIncarnationFailureCap)
				derived.threadIncarnationFailures, derived.threadIncarnationFailuresCapped = mergeThreadIncarnationFailures(
					derived.threadIncarnationFailures, derived.threadIncarnationFailuresCapped,
					mergedFailures, mergedCapped, threadIncarnationFailureCap)
			} else {
				derived.schedulerOrderFailures, derived.schedulerOrderFailuresCapped = schedulerOrderFailuresFromEvents(idx.Events, auditQ, 0, schedulerOrderFailureCap)
				derived.durationOrderFailures, derived.durationOrderFailuresCapped = durationOrderFailuresFromEvents(idx.Events, auditQ, durationOrderFailureCap)
				derived.threadIncarnationFailures, derived.threadIncarnationFailuresCapped = threadIncarnationConflictsFromEvents(idx.Events, auditQ, threadIncarnationFailureCap)
			}
			if opts.TimeStartSet && opts.TimeStart > 0 {
				derived.setSchedulerHead(schedulerHeadFromEvents(idx, opts.TimeStart))
			}
			if err := selection.finish(ctx, derived); err != nil {
				indexCache.Delete(fullKey)
				return nil, err
			}
			return derived, nil
		}
	}
	return buildIndexSingleflight(ctx, key, selection, opts, cacheable, observer)
}

func shouldCacheTraceIndex(size int64, opts BuildOptions) bool {
	if opts.windowed() {
		return false
	}
	return size <= maxCachedTraceIndexBytes
}

// traceIndexSourceIdentity returns a deterministic stat fingerprint and total
// physical byte size for the complete source universe selected by BuildIndex.
// It covers the bundle manifest plus every child, or a systrace plus every
// V2-bound child. The same value keys the cache and in-flight singleflight;
// total bytes (not the small manifest/primary size) decide cacheability.
func traceIndexSourceIdentity(path string, entry os.FileInfo) (int64, string, error) {
	_ = entry // retained for package-local compatibility with existing callers.
	selection, err := resolveTraceIndexSelection(context.Background(), path)
	if err != nil {
		return 0, "", err
	}
	if err := selection.validate(context.Background()); err != nil {
		return 0, "", selection.closeAfter(err)
	}
	if err := selection.close(); err != nil {
		return 0, "", err
	}
	return selection.universe.totalBytes, selection.universe.cacheToken, nil
}

func canonicalTraceIndexPath(path string) string {
	path = filepath.Clean(path)
	// Windows pipe namespaces are not filesystem paths: filepath.Abs or
	// EvalSymlinks may reach CreateFile/Lstat and wait for a pipe server before
	// the regular-file opener gets a chance to reject them. Keep the lexical
	// spelling untouched until the shared admission guard returns the error.
	if traceSourcePathIsBlockingNamespace(path) {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		abs = filepath.Clean(abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			return filepath.Clean(real)
		}
		return abs
	}
	return path
}

func buildIndexSingleflight(ctx context.Context, key parseCacheKey, selection *traceIndexSelection, opts BuildOptions, cacheable bool, observer traceIndexBuildObserver) (*Index, error) {
	indexBuildMu.Lock()
	if call := indexBuilds[key]; call != nil {
		indexBuildMu.Unlock()
		if observer != nil {
			observer(traceIndexPhaseSingleflightJoined, key)
		}
		select {
		case <-call.done:
			if call.err == nil {
				if err := selection.finish(ctx, call.idx); err != nil {
					indexCache.Delete(key)
					return nil, err
				}
			} else if err := selection.closeAfter(call.err); err != nil {
				return nil, err
			}
			return call.idx, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &indexBuildCall{done: make(chan struct{})}
	indexBuilds[key] = call
	indexBuildMu.Unlock()

	call.idx, call.err = parseSelectedFile(ctx, selection, opts)
	if call.err == nil && opts.windowed() {
		if call.err = populateWindowPerfGenerationHeads(ctx, call.idx); call.err != nil {
			call.idx = nil
		}
	}
	if call.err == nil && opts.windowed() && opts.TimeStartSet && opts.TimeStart > 0 {
		if observer != nil {
			observer(traceIndexPhaseBeforeSchedulerHead, key)
		}
		if call.err = populateWindowSchedulerHead(ctx, call.idx, opts.TimeStart); call.err != nil {
			call.idx = nil
		}
	}
	if call.err == nil {
		if call.err = prebuildPerfIdentityLedger(ctx, call.idx); call.err != nil {
			call.idx = nil
		}
	}
	if call.err == nil {
		if observer != nil {
			observer(traceIndexPhaseBeforeUniverseValidation, key)
		}
		if call.err = selection.finish(ctx, call.idx); call.err != nil {
			call.idx = nil
		}
	}
	if call.err == nil && cacheable {
		indexCache.Store(key, call.idx)
	} else if call.err != nil {
		call.err = selection.closeAfter(call.err)
		indexCache.Delete(key)
	}
	indexBuildMu.Lock()
	delete(indexBuilds, key)
	close(call.done)
	indexBuildMu.Unlock()
	return call.idx, call.err
}

func deriveWindowedIndex(full *Index, opts BuildOptions) *Index {
	if full == nil {
		return nil
	}
	out := &Index{
		Path:                                     full.Path,
		Size:                                     full.Size,
		ModTime:                                  full.ModTime,
		TraceArtifacts:                           append([]TraceArtifactSource(nil), full.TraceArtifacts...),
		LineCount:                                full.LineCount,
		ScannedLineCount:                         full.ScannedLineCount,
		Windowed:                                 true,
		IndexTimeStart:                           paddedTimeStart(opts),
		IndexTimeEnd:                             paddedTimeEnd(opts),
		IndexLineStart:                           paddedLineStart(opts),
		IndexLineEnd:                             paddedLineEnd(opts),
		TraceFlavor:                              full.TraceFlavor,
		FlavorConfidence:                         full.FlavorConfidence,
		FlavorSignals:                            append([]string(nil), full.FlavorSignals...),
		Caveats:                                  append([]string(nil), full.Caveats...),
		ClockRegressions:                         full.ClockRegressions,
		TimestampOrder:                           full.TimestampOrder,
		schedulerOrderFailures:                   append([]schedulerOrderViolation(nil), full.schedulerOrderFailures...),
		durationOrderFailures:                    append([]durationOrderViolation(nil), full.durationOrderFailures...),
		durationOrderFailuresCapped:              cloneDurationOrderCapped(full.durationOrderFailuresCapped),
		schedulerRowIntegrityFailures:            append([]schedulerRowIntegrityFailure(nil), full.schedulerRowIntegrityFailures...),
		blockedReasonIntegrityFailures:           append([]blockedReasonIntegrityFailure(nil), full.blockedReasonIntegrityFailures...),
		blockedReasonIntegrityOverflow:           full.blockedReasonIntegrityOverflow.clone(),
		blockedReasonIdentityOverflow:            full.blockedReasonIdentityOverflow.clone(),
		cpuInputIntegrityFailures:                append([]cpuInputIntegrityFailure(nil), full.cpuInputIntegrityFailures...),
		cpuInputIntegrityFailuresCapped:          full.cpuInputIntegrityFailuresCapped,
		traceMarkIntegrityFailures:               append([]traceMarkIntegrityFailure(nil), full.traceMarkIntegrityFailures...),
		traceMarkIntegrityFailuresCapped:         full.traceMarkIntegrityFailuresCapped,
		traceMarkIntegrityDroppedGlobalPoison:    full.traceMarkIntegrityDroppedGlobalPoison,
		traceTrackIntegrityDroppedPoison:         full.traceTrackIntegrityDroppedPoison,
		threadIncarnationFailures:                append([]threadIncarnationConflict(nil), full.threadIncarnationFailures...),
		threadIncarnationFailuresCapped:          full.threadIncarnationFailuresCapped,
		schedulerOrderFailuresCapped:             full.schedulerOrderFailuresCapped,
		schedulerRowIntegrityFailuresCapped:      full.schedulerRowIntegrityFailuresCapped,
		schedulerRowIntegrityOverflowSources:     append([]string(nil), full.schedulerRowIntegrityOverflowSources...),
		schedulerRowIntegrityOverflowGlobal:      full.schedulerRowIntegrityOverflowGlobal,
		priorityMutationIntegrityFailuresCapped:  full.priorityMutationIntegrityFailuresCapped,
		priorityMutationIntegrityOverflowSources: append([]string(nil), full.priorityMutationIntegrityOverflowSources...),
		priorityMutationIntegrityOverflowGlobal:  full.priorityMutationIntegrityOverflowGlobal,
		blockedReasonIntegrityFailuresCapped:     full.blockedReasonIntegrityFailuresCapped,
		// R6 rule 4: the full-file frequency curves are a trace attribute —
		// a window derived from a complete parent keeps the full-file basis
		// (maps shared read-only).
		fullFreq: full.fullFreq,
	}
	firstLine, lastLine := 0, 0
	// Window selection prefers a ZERO-COPY view: Event is ~1KB, so
	// copying a large window allocates window×1KB per derived query
	// (165MB for a 170k-event window measured on the baseline bench).
	// Events append in line order and trace timestamps are normally
	// monotonic, so the in-window rows usually form one contiguous run
	// [firstIdx..lastIdx]; when a single verification pass confirms
	// every row inside that run matches the window, the derived index
	// shares the parent's backing array (indices are immutable after
	// build; all consumers read events by value or via read-only
	// pointers). Clock regressions or holes fall back to the copying
	// path so correctness never depends on monotonicity.
	firstIdx, lastIdx := -1, -1
	for i := range full.Events {
		if eventInBuildWindow(full.Events[i], out) {
			firstIdx = i
			break
		}
	}
	if firstIdx >= 0 {
		for i := len(full.Events) - 1; i >= firstIdx; i-- {
			if eventInBuildWindow(full.Events[i], out) {
				lastIdx = i
				break
			}
		}
	}
	contiguous := firstIdx >= 0
	if contiguous {
		for i := firstIdx; i <= lastIdx; i++ {
			ev := &full.Events[i]
			if !eventInBuildWindow(*ev, out) {
				contiguous = false
				break
			}
			if out.FirstTs == 0 || ev.Ts < out.FirstTs {
				out.FirstTs = ev.Ts
			}
			if ev.Ts > out.LastTs {
				out.LastTs = ev.Ts
			}
			if ev.Type != EventUnknown {
				out.ParsedKnown++
			}
			if firstLine == 0 || ev.Line < firstLine {
				firstLine = ev.Line
			}
			if ev.Line > lastLine {
				lastLine = ev.Line
			}
		}
	}
	if contiguous && firstIdx >= 0 {
		out.Events = full.Events[firstIdx : lastIdx+1 : lastIdx+1]
	} else if firstIdx >= 0 {
		// Non-contiguous window (clock regression inside the span):
		// reset the stats accumulated during the failed verification
		// pass and rebuild via the copying path.
		out.FirstTs, out.LastTs, out.ParsedKnown = 0, 0, 0
		firstLine, lastLine = 0, 0
		for _, ev := range full.Events {
			if !eventInBuildWindow(ev, out) {
				continue
			}
			if out.FirstTs == 0 || ev.Ts < out.FirstTs {
				out.FirstTs = ev.Ts
			}
			if ev.Ts > out.LastTs {
				out.LastTs = ev.Ts
			}
			if ev.Type != EventUnknown {
				out.ParsedKnown++
			}
			if firstLine == 0 || ev.Line < firstLine {
				firstLine = ev.Line
			}
			if ev.Line > lastLine {
				lastLine = ev.Line
			}
			out.Events = append(out.Events, ev)
		}
	}
	if out.IndexLineStart > 0 && out.IndexLineEnd > 0 {
		out.ScannedLineCount = out.IndexLineEnd - out.IndexLineStart + 1
	} else if firstLine > 0 && lastLine >= firstLine {
		out.ScannedLineCount = lastLine - firstLine + 1
	}
	if out.ScannedLineCount < 0 {
		out.ScannedLineCount = 0
	}
	seedPerfGenerationHeadsFromFull(out, full)
	return out
}

func eventInBuildWindow(ev Event, idx *Index) bool {
	if idx == nil {
		return true
	}
	if idx.IndexLineStart > 0 && ev.Line < idx.IndexLineStart {
		return false
	}
	if idx.IndexLineEnd > 0 && ev.Line > idx.IndexLineEnd {
		return false
	}
	if idx.IndexTimeStart > 0 && ev.Ts < idx.IndexTimeStart {
		if ev.Type != EventTraceAsyncInterval || ev.PluginFields == nil ||
			ev.PluginFields.AsyncInterval == nil ||
			float64(ev.PluginFields.AsyncInterval.EndTimestampNS)/1e9 <= idx.IndexTimeStart {
			return false
		}
	}
	if idx.IndexTimeEnd > 0 && ev.Ts > idx.IndexTimeEnd {
		return false
	}
	return true
}

func normalizeBuildOptions(opts BuildOptions) BuildOptions {
	if opts.MaxEvents <= 0 {
		opts.MaxEvents = defaultTraceIndexMaxEvents
	}
	if !opts.AllowWindowedParse {
		return BuildOptions{MaxEvents: opts.MaxEvents}
	}
	if opts.TimePaddingBefore < 0 {
		opts.TimePaddingBefore = 0
	}
	if opts.TimePaddingAfter < 0 {
		opts.TimePaddingAfter = 0
	}
	if opts.LinePaddingBefore < 0 {
		opts.LinePaddingBefore = 0
	}
	if opts.LinePaddingAfter < 0 {
		opts.LinePaddingAfter = 0
	}
	return opts
}

func (opts BuildOptions) windowed() bool {
	return opts.AllowWindowedParse && (opts.TimeStartSet || opts.TimeEndSet || opts.LineStart > 0 || opts.LineEnd > 0)
}

// relationScoped reports whether this build should relation-prune the windowed
// index. PID scopes seed the closure directly; thread-only scopes are resolved
// by the discovery pass from structured trace rows and may degrade to unpruned
// with a caveat when ambiguous.
func (opts BuildOptions) relationScoped() bool {
	return opts.RelationScoped && opts.windowed() && (opts.ScopePID > 0 || strings.TrimSpace(opts.ScopeThread) != "")
}

func (opts BuildOptions) scopeMaxDepth() int {
	if opts.ScopeMaxDepth > 0 {
		return opts.ScopeMaxDepth
	}
	return defaultRelationScopeMaxDepth
}

func (opts BuildOptions) cacheKey() string {
	if !opts.windowed() {
		return ""
	}
	key := fmt.Sprintf("ts=%t:%.9f-%t:%.9f+%.6f/%.6f;ln=%d-%d+%d/%d;max_events=%d",
		opts.TimeStartSet, opts.TimeStart, opts.TimeEndSet, opts.TimeEnd,
		opts.TimePaddingBefore, opts.TimePaddingAfter,
		opts.LineStart, opts.LineEnd, opts.LinePaddingBefore, opts.LinePaddingAfter,
		opts.MaxEvents)
	// Isolate pid-relation-scoped indices in their own cache slot. A pruned
	// index is NOT a valid answer for a different pid or for an unscoped query,
	// so it must never collide with them. Non-scoped keys are byte-identical to
	// before (the scope segment is appended only when relation-scoped).
	if opts.relationScoped() {
		if opts.ScopePID > 0 {
			key += fmt.Sprintf(";scope=rel/pid:%d/depth:%d", opts.ScopePID, opts.scopeMaxDepth())
		} else {
			key += fmt.Sprintf(";scope=rel/thread:%s/depth:%d", normalizedThreadText(opts.ScopeThread), opts.scopeMaxDepth())
		}
	}
	return key
}

// matchFtraceLine keeps the established fast greedy grammar for a common
// one-header row. Ambiguous rows enumerate -PID candidates only inside the
// canonical comm domain and elect its rightmost valid delimiter; this finds a
// real outer header between comm-internal pseudo headers and body-nested
// headers. A loose twin locates the same delimiter without admitting malformed
// scalars. Legal ']' and header-like bytes in comm therefore remain data.
func matchFtraceLine(line string) []string {
	indexes := matchFtraceLineIndex(line)
	if len(indexes) == 0 {
		return nil
	}
	match := make([]string, len(indexes)/2)
	for group := range match {
		start, end := indexes[group*2], indexes[group*2+1]
		if start >= 0 && end >= start {
			match[group] = line[start:end]
		}
	}
	return match
}

func matchFtraceLineIndex(line string) []int {
	match := ftraceLineRE.FindStringSubmatchIndex(line)
	if len(match) < 4 {
		return match
	}
	comm := line[match[2]:match[3]]
	commAmbiguous := ftraceCommMayContainHeader(comm)
	headerAmbiguous := hasMultipleWhitespaceBracketCandidates(line)
	if !commAmbiguous && !headerAmbiguous {
		return match
	}
	if !strings.ContainsAny(comm, "[]") && !headerAmbiguous {
		// A colon may legitimately occur in comm, including compatibility
		// producers that exceed TASK_COMM_LEN. Without another whitespace-[CPU]
		// candidate, no complete earlier/nested header was swallowed; preserve
		// the greedy/rightmost PID split.
		return match
	}
	strict := boundedPIDFtraceLineIndex(line, ftraceStrictCanonicalPIDTailRE)
	if len(strict) < 14 {
		return nil
	}
	physical := loosePhysicalFtraceLineIndex(line)
	if len(physical) >= 14 && physical[12] != strict[12] {
		// The rightmost canonical comm delimiter leads to a malformed physical
		// header, while an earlier header-like substring happened to satisfy the
		// strict grammar. Reject that pseudo event; body candidates are outside
		// the bounded comm prefix and cannot participate in this election.
		return nil
	}
	return strict
}

func ftraceCommMayContainHeader(comm string) bool {
	return strings.ContainsAny(comm, "[]:")
}

func hasMultipleWhitespaceBracketCandidates(line string) bool {
	rawCount := 0
	for index := 1; index < len(line); index++ {
		if line[index] != '[' || line[index-1] != ' ' && line[index-1] != '\t' {
			continue
		}
		rawCount++
		if rawCount > 1 {
			break
		}
	}
	if rawCount <= 1 {
		return false
	}
	structuralCount := 0
	for index := 1; index < len(line); index++ {
		if line[index] != '[' || line[index-1] != ' ' && line[index-1] != '\t' {
			continue
		}
		tail := ftraceLooseMissingPIDTailRE.FindStringSubmatchIndex(line[index:])
		if len(tail) != 10 || tail[0] != 0 || tail[1] != len(line)-index {
			continue
		}
		structuralCount++
		if structuralCount > 1 {
			return true
		}
	}
	return false
}

const ftraceCanonicalCommMaxRunes = 15

func ftraceLineTrimStart(line string) int {
	start := 0
	for start < len(line) {
		switch line[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			return start
		}
	}
	return start
}

func ftraceCommByteLimit(line string, start int) int {
	limit := start
	for runes := 0; runes < ftraceCanonicalCommMaxRunes && limit < len(line); runes++ {
		_, width := utf8.DecodeRuneInString(line[limit:])
		if width <= 0 {
			width = 1
		}
		limit += width
	}
	return limit
}

// boundedPIDFtraceLineIndex enumerates candidate -PID delimiters from the
// right edge of the canonical comm domain. Fixing the delimiter before
// parsing the tail is the key invariant: a header-like prefix inside comm is
// earlier, while an embedded header in body is necessarily outside the 15
// rune domain. The returned group indexes mirror ftraceLineRE.
func boundedPIDFtraceLineIndex(line string, tailRE *regexp.Regexp) []int {
	start := ftraceLineTrimStart(line)
	limit := ftraceCommByteLimit(line, start)
	if limit >= len(line) {
		limit = len(line) - 1
	}
	for delimiter := limit; delimiter > start; delimiter-- {
		if line[delimiter] != '-' {
			continue
		}
		tail := tailRE.FindStringSubmatchIndex(line[delimiter:])
		if len(tail) != 14 || tail[0] != 0 || tail[1] != len(line)-delimiter {
			continue
		}
		return assemblePIDFtraceLineIndexes(line, start, delimiter, tail)
	}
	return nil
}

// unboundedSafePIDFtraceLineIndex preserves compatibility with producers that
// publish a comm longer than TASK_COMM_LEN. It is intentionally narrower than
// the canonical election: only a comm with no header delimiters can qualify.
// A nested/body candidate necessarily carries the earlier header's brackets
// or event colon in its comm prefix and is skipped while the scan continues
// toward the real outer delimiter.
func unboundedSafePIDFtraceLineIndex(line string, tailRE *regexp.Regexp) []int {
	start := ftraceLineTrimStart(line)
	for delimiter := len(line) - 1; delimiter > start; delimiter-- {
		if line[delimiter] != '-' {
			continue
		}
		tail := tailRE.FindStringSubmatchIndex(line[delimiter:])
		if len(tail) != 14 || tail[0] != 0 || tail[1] != len(line)-delimiter ||
			ftraceCommMayContainHeader(line[start:delimiter]) {
			continue
		}
		return assemblePIDFtraceLineIndexes(line, start, delimiter, tail)
	}
	return nil
}

func assemblePIDFtraceLineIndexes(line string, commStart, delimiter int, tail []int) []int {
	indexes := make([]int, 16)
	for index := range indexes {
		indexes[index] = -1
	}
	indexes[0], indexes[1] = 0, len(line)
	indexes[2], indexes[3] = commStart, delimiter
	for tailGroup := 1; tailGroup <= 6; tailGroup++ {
		source := tailGroup * 2
		destination := (tailGroup + 1) * 2
		if tail[source] >= 0 {
			indexes[destination] = delimiter + tail[source]
			indexes[destination+1] = delimiter + tail[source+1]
		}
	}
	return indexes
}

// loosePhysicalFtraceLineIndex returns the rightmost structurally complete
// header inside the canonical comm domain without granting scalar or endpoint
// authority. Event-column position wins globally; canonical PID wins only
// when candidates reach the same physical column. The permissive and
// missing-PID arms can only retain family/header provenance when owner identity
// is unavailable.
func loosePhysicalFtraceLineIndex(line string) []int {
	type candidate struct {
		indexes  []int
		priority int
	}
	candidates := []candidate{
		{boundedPIDFtraceLineIndex(line, ftraceLooseCanonicalPIDTailRE), 3},
		{unboundedSafePIDFtraceLineIndex(line, ftraceLooseCanonicalPIDTailRE), 3},
		{boundedPIDFtraceLineIndex(line, ftraceLooseAnyPIDTailRE), 2},
		{unboundedSafePIDFtraceLineIndex(line, ftraceLooseAnyPIDTailRE), 2},
		{boundedMissingPIDFtraceLineIndex(line), 1},
	}
	var best candidate
	for _, current := range candidates {
		if len(current.indexes) < 14 {
			continue
		}
		if len(best.indexes) < 14 || current.indexes[12] > best.indexes[12] ||
			current.indexes[12] == best.indexes[12] && current.priority > best.priority {
			best = current
		}
	}
	return best.indexes
}

func boundedMissingPIDFtraceLineIndex(line string) []int {
	start := ftraceLineTrimStart(line)
	for bracket := len(line) - 1; bracket > start; bracket-- {
		if line[bracket] != '[' || bracket == 0 || line[bracket-1] != ' ' && line[bracket-1] != '\t' {
			continue
		}
		commEnd := bracket
		for commEnd > start && (line[commEnd-1] == ' ' || line[commEnd-1] == '\t') {
			commEnd--
		}
		if commEnd <= start || utf8.RuneCountInString(line[start:commEnd]) > ftraceCanonicalCommMaxRunes {
			continue
		}
		tail := ftraceLooseMissingPIDTailRE.FindStringSubmatchIndex(line[bracket:])
		if len(tail) != 10 || tail[0] != 0 || tail[1] != len(line)-bracket {
			continue
		}
		indexes := make([]int, 16)
		for index := range indexes {
			indexes[index] = -1
		}
		indexes[0], indexes[1] = 0, len(line)
		indexes[2], indexes[3] = start, commEnd
		for tailGroup := 1; tailGroup <= 4; tailGroup++ {
			source := tailGroup * 2
			destination := (tailGroup + 3) * 2
			indexes[destination] = bracket + tail[source]
			indexes[destination+1] = bracket + tail[source+1]
		}
		return indexes
	}
	return nil
}

func loosePhysicalFtraceLine(line string) []string {
	indexes := loosePhysicalFtraceLineIndex(line)
	if len(indexes) == 0 {
		return nil
	}
	match := make([]string, len(indexes)/2)
	for group := range match {
		start, end := indexes[group*2], indexes[group*2+1]
		if start >= 0 && end >= start {
			match[group] = line[start:end]
		}
	}
	return match
}

// lineScan memoizes the per-line ftrace header analysis shared by the window
// gate, the anchor recorder, every physical-row integrity audit and the event
// parse itself. One physical line pays for one primary ftraceLineRE match (and
// only an embedded-header ambiguity pays bounded comm-tail probes), one parseKV
// and one Event parse no matter how many consumers inspect it (perf audit #21,
// §29.25 处置委托 2026-07-10: the five per-line audits used to re-run the full
// header regex up to 4× and re-parse the same line 2-3×, a measured 29×
// windowed-build regression on GiB traces). The struct is reused across loop
// iterations; reset() only clears the memo flags.
type lineScan struct {
	lineNo               int
	line                 string
	m                    []string
	mTried               bool
	kv                   map[string]string
	kvTried              bool
	schedSwitchKVFailure string
	schedulerTyped       schedulerTypedFields
	perfTextTyped        perfTextTypedFields
	binderTyped          binderTransactionTypedFields
	cpuScalarTyped       cpuScalarTypedFields
	ts                   float64
	tsOK                 bool
	tsTried              bool
}

func (s *lineScan) reset(lineNo int, line string) {
	s.lineNo, s.line = lineNo, line
	s.mTried, s.kvTried, s.tsTried = false, false, false
	s.schedSwitchKVFailure = ""
	s.schedulerTyped = schedulerTypedFields{}
	s.perfTextTyped = perfTextTypedFields{}
	s.binderTyped = binderTransactionTypedFields{}
	s.cpuScalarTyped = cpuScalarTypedFields{}
}

func (s *lineScan) match() []string {
	if !s.mTried {
		s.mTried = true
		s.m = matchFtraceLine(s.line)
	}
	return s.m
}

// keyValues returns the event-specific typed field view of the matched fields
// column (the strict sched_switch parser, generic parseKV otherwise).
// Consumers must treat the map as read-only: it is shared by every audit on
// this line and by the event parse.
func (s *lineScan) keyValues() map[string]string {
	if !s.kvTried {
		s.kvTried = true
		s.kv = nil
		if m := s.match(); len(m) != 0 {
			fields := strings.TrimSpace(m[7])
			rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
			if cpuScalarProfileForName(rawType) != cpuScalarProfileNone {
				// CPU state/control values govern carry-in, frequency curves and
				// supply attribution. One occurrence-aware fixed-width receipt owns
				// both Event construction and every integrity consumer.
				s.kv, s.cpuScalarTyped = parseCPUScalarTypedFields(rawType, fields)
				return s.kv
			}
			switch rawType {
			case "sched_switch":
				// Do not fall back to the generic token scanner on structural
				// failure. Its context-free matches cannot distinguish a comm's
				// text from typed scheduler fields, so the scheduler integrity gate
				// must see an empty map and reject the malformed row instead.
				s.kv, s.schedSwitchKVFailure = parseSchedSwitchKV(fields)
			case "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task", "sched_blocked_reason", "sched_pi_setprio", "binder_set_priority":
				// These scheduler rows carry hard identities. Their producer grammar
				// is unquoted, so occurrence evidence must be retained before any
				// normalized map can erase it. Event construction and both integrity
				// audits consume this same cached verdict; generic parseKV is never a
				// fallback for these four families.
				s.kv, s.schedulerTyped = parseSchedulerTypedFields(rawType, fields)
			case "perf_sample":
				// Converter-owned perf text has quoted metadata and hard
				// thread/CPU/weight scalars. The generic regex cannot understand
				// Go escapes and may reopen key-looking metadata, so success and
				// failure both stay on this single cached authority.
				s.kv, s.perfTextTyped = parsePerfTextKV(fields)
			case "binder_transaction", "binder_transaction_received":
				// Binder endpoint fields govern pairing, receiver fallback and
				// blocking semantics. Preserve physical occurrences and fixed-width
				// validity in one cached verdict; generic parseKV would erase both.
				s.kv, s.binderTyped = parseBinderTransactionTypedFields(rawType, fields)
			default:
				s.kv = parseKV(fields)
			}
		}
	}
	return s.kv
}

// timestamp is byte-identical to parseLineTimestamp over the same line: the
// exact anchored header grammar plus the finite-seconds parse. A
// timestamp-looking token in comm/field text must never be promoted into
// trace time.
func (s *lineScan) timestamp() (float64, bool) {
	if !s.tsTried {
		s.tsTried = true
		s.ts, s.tsOK = 0, false
		if mark, ok := parseExactTraceMark(s.line); ok {
			s.ts, s.tsOK = float64(mark.TimestampNS)/1e9, true
		} else if mark, ok := parseCPUUnavailableTraceMark(s.line); ok {
			s.ts, s.tsOK = float64(mark.TimestampNS)/1e9, true
		} else if wakeup, ok := parseCPUUnavailableWakeup(s.line); ok {
			s.ts, s.tsOK = float64(wakeup.TimestampNS)/1e9, true
		} else if interval, ok := parseCompletedAsyncInterval(s.line); ok {
			s.ts, s.tsOK = float64(interval.StartTimestampNS)/1e9, true
		} else if relation, ok := parseFrameMapRelation(s.line); ok {
			s.ts, s.tsOK = float64(relation.TimestampNS)/1e9, true
		} else if m := s.match(); len(m) != 0 {
			s.ts, s.tsOK = parseTraceTimestampSeconds(m[5])
		}
	}
	return s.ts, s.tsOK
}

// windowGate holds the padded line/time bounds of a windowed parse and decides,
// for each scanned line, whether it falls before/outside the window (skip), is
// provably past it (stop), or is inside it (retain). A time-end stop is legal
// only with a complete-file monotonic timestamp proof. Both the main parse loop
// and the relation-scope discovery pass route through this single method so
// they observe a byte-identical window line set — any drift between the two
// would corrupt pruning. Timestamp extraction flows through the shared
// lineScan memo, so the gate never pays a second header match on a line whose
// audits or parse already matched it.
type windowGate struct {
	lineStart, lineEnd       int
	timeStart, timeEnd       float64
	timeStartSet, timeEndSet bool
	allowTimeEndStop         bool
}

func (w windowGate) decide(lineNo int, scan *lineScan, seenTimeWindow *bool) (skip, stop bool) {
	if w.lineEnd > 0 && lineNo > w.lineEnd {
		return false, true
	}
	if w.lineStart > 0 && lineNo < w.lineStart {
		return true, false
	}
	if w.timeStartSet || w.timeEndSet {
		ts, hasTS := scan.timestamp()
		if hasTS {
			if w.timeStartSet && ts < w.timeStart {
				// A completed typed interval is a single physical source row.
				// Retain it when its exact end overlaps the requested window;
				// treating only its start timestamp as row admission would
				// erase carry-in/carry-through async work.
				if interval, ok := parseCompletedAsyncInterval(scan.line); ok {
					end := float64(interval.EndTimestampNS) / 1e9
					if end > w.timeStart && (!w.timeEndSet || ts <= w.timeEnd) {
						*seenTimeWindow = true
						return false, false
					}
				}
				return true, false
			}
			if w.timeEndSet && ts > w.timeEnd {
				// A regression-free PREFIX is not a proof about the unread
				// suffix. Without the typed complete-file proof, skip this
				// out-of-window row and keep scanning: a later physical line may
				// regress back into the requested window.
				return true, w.allowTimeEndStop
			}
			*seenTimeWindow = true
		} else if w.timeStartSet && !*seenTimeWindow {
			return true, false
		}
	}
	return false, false
}

// completeTimestampOrderProof consumes the unread physical suffix using only
// timestamp extraction and anchor bookkeeping. It is used when the event
// budget trips in padding: the retained event slice cannot grow, but a cheap
// O(1)-memory suffix pass can still establish (or reject) the complete-file
// monotonic proof required to degrade safely. No event or parse-quality count
// is fabricated for this timestamp-only suffix.
func completeTimestampOrderProof(ctx context.Context, r *bufio.Reader, nextLine int, currentReadErr error, idx *Index, recorder *anchorRecorder, recording bool) error {
	if !recording || recorder == nil {
		return nil
	}
	if currentReadErr != nil {
		if currentReadErr == io.EOF {
			recorder.finishEOF()
			return nil
		}
		return currentReadErr
	}
	for lineNo := nextLine; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := readStreamScanPhysicalLine(r, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			ts, hasTS := parseLineTimestamp(trimmed)
			recorder.observe(lineNo, len(line), ts, hasTS, trimmed)
		}
		if readErr != nil {
			if readErr == io.EOF {
				recorder.finishEOF()
				return nil
			}
			return readErr
		}
	}
}

func parseFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
	selection, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return nil, err
	}
	if selection.indexIdentity.Size() != size || selection.indexIdentity.ModUnixNano() != modUnix {
		return nil, selection.closeAfter(fmt.Errorf("trace source identity changed before parsing began"))
	}
	idx, err := parseSelectedFile(ctx, selection, opts)
	if err == nil {
		err = selection.finish(ctx, idx)
	} else {
		err = selection.closeAfter(err)
	}
	return idx, err
}

func parseSelectedFile(ctx context.Context, selection *traceIndexSelection, opts BuildOptions) (*Index, error) {
	if selection == nil {
		return nil, fmt.Errorf("trace index selection is nil")
	}
	path := selection.indexPath
	size := selection.indexIdentity.Size()
	modUnix := selection.indexIdentity.ModUnixNano()
	if selection.bundleSet {
		return parseTraceBundleSelection(ctx, selection, opts)
	}
	if len(selection.artifactSpecs) > 0 {
		return parseTraceArtifactSpecs(ctx, path, size, modUnix, opts, selection.artifactSpecs, &selection.universe)
	}
	idx, err := parseSingleTraceFile(ctx, path, size, modUnix, opts, selection.indexIdentity)
	if err == nil && idx != nil && strings.HasSuffix(strings.ToLower(path), ".perftrace") {
		// Audit #35 (§29.25 处置委托 2026-07-10): a directly queried .perftrace is
		// the intentional per-clock-domain escape hatch and deliberately never
		// consults a sibling bundle manifest (promoteSiblingTraceBundlePath pin).
		// That also means the bundle lane's capability admission gate
		// (trace_query_ready / thread-identity enums / scheduler-row exclusion /
		// identity scrub) has NOT vouched for these rows — disclose it instead
		// of silently publishing identity/scheduler lanes at full authority.
		// Bundle children take the composite lane and never see this caveat;
		// their rows pass applyPerfBundleAdmission instead.
		idx.Caveats = append(idx.Caveats, "perftrace_capability_unattested=true; this .perftrace was queried directly (per-clock-domain escape hatch), so no bundle capability manifest attested its thread/CPU identity or scheduler-row provenance; treat identity-derived and scheduler-derived lanes as unattested, or query the owning tracebundle for capability-gated admission")
	}
	return idx, err
}

func parseSingleTraceFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions, expected traceFileIdentity) (idx *Index, err error) {
	f, openedIdentity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			idx = nil
			err = errors.Join(err, fmt.Errorf("close trace source %s after parsing: %w", path, closeErr))
		}
	}()
	if expected.Initialized() && !expected.SameVersion(openedIdentity) {
		return nil, fmt.Errorf("trace source generation differs from the selected artifact ledger")
	}
	if openedIdentity.Size() != size || openedIdentity.ModUnixNano() != modUnix {
		return nil, fmt.Errorf("trace source identity changed before its parser opened the artifact")
	}

	idx = &Index{Path: path, Size: size, ModTime: time.Unix(0, modUnix)}
	if opts.windowed() {
		idx.Windowed = true
		idx.IndexTimeStart = paddedTimeStart(opts)
		idx.IndexTimeEnd = paddedTimeEnd(opts)
		idx.IndexLineStart = paddedLineStart(opts)
		idx.IndexLineEnd = paddedLineEnd(opts)
	}
	// The per-file anchor entry is keyed by immutable file identity. Its
	// TimestampOrder value is accepted only when a prior scan reached EOF;
	// prefix anchors alone never authorize a time-end stop.
	anchorKey := traceAnchorKeyForIdentity(path, openedIdentity)
	anchorSet := anchorCache.load(anchorKey)
	if anchorSet != nil {
		idx.TimestampOrder = anchorSet.TimestampOrder
		applyAnchorPriorityMutationAudit(idx, anchorSet, path)
	}
	gate := windowGate{
		lineStart:        idx.IndexLineStart,
		lineEnd:          idx.IndexLineEnd,
		timeStart:        idx.IndexTimeStart,
		timeEnd:          idx.IndexTimeEnd,
		timeStartSet:     opts.TimeStartSet,
		timeEndSet:       opts.TimeEndSet,
		allowTimeEndStop: idx.TimestampOrder.AllowsTimeEndEarlyStop(),
	}
	// Gap 3 Step 2: for a pid-relation-scoped build, first stream the window
	// once to discover the target pid's thread set and its transitive scheduler
	// wakers; the main pass below then retains only events touching that set (plus
	// all binder rows). All idx metadata (FirstTs/LastTs/flavor/…) is still
	// computed over the FULL window, so only idx.Events is pruned — query results
	// on the causal-chain views stay identical to the full index.
	var relScope *relationScope
	var relScopeCaveats []string
	if opts.relationScoped() {
		s, caveats, derr := discoverRelationScope(ctx, f, path, openedIdentity, opts)
		if derr != nil {
			return nil, derr
		}
		relScope = s
		relScopeCaveats = caveats
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return nil, fmt.Errorf("rewind trace source after relation discovery: %w", seekErr)
		}
	}
	// P1 anchor seek: a windowed build over an already-anchored file jumps
	// to the last anchor guaranteed to precede every in-window line instead
	// of re-streaming the whole prefix. Gate logic after the seek point is
	// byte-identical, so the parsed event set cannot change.
	seekAnchor, seeked := anchorSet.seekAnchorFor(opts.TimeStartSet, idx.IndexTimeStart, idx.IndexLineStart)
	// A non-monotonic/unknown source must be audited from physical line 1 for
	// same-lane rollback and lifecycle reuse. Seeking past the prefix would
	// erase the very predecessor needed by those hard gates.
	if idx.TimestampOrder != TraceTimestampOrderMonotonic {
		seeked = false
		seekAnchor = traceAnchor{}
	}
	incarnationAudit := newThreadIncarnationTracker()
	incarnationAuditSeedBoundary := float64(0)
	if seeked {
		// A time anchor skips physical rows that establish whether an in-window
		// sched_wakeup_new reuses an existing TID. Reuse the scheduler-head prefix
		// scan as an immutable lifecycle checkpoint, then audit only rows at/after
		// that boundary to avoid replaying the seeded prefix. Line-only seeks have
		// no timestamp boundary for this checkpoint and therefore stay unseeked.
		if idx.IndexTimeStart <= 0 {
			seeked = false
			seekAnchor = traceAnchor{}
		} else {
			source := singleTraceArtifactSourceWithIdentity(path, openedIdentity, 0, 0)
			prefix, prefixErr := sourceSchedulerHeadSnapshot(ctx, source, idx.IndexTimeStart)
			if prefixErr != nil {
				return nil, fmt.Errorf("build lifecycle checkpoint for anchored trace window: %w", prefixErr)
			}
			if !prefix.Complete || prefix.lifecycle == nil {
				seeked = false
				seekAnchor = traceAnchor{}
			} else {
				incarnationAudit = prefix.lifecycle.clone()
				incarnationAuditSeedBoundary = idx.IndexTimeStart
			}
		}
	}
	startLine := 1
	if seeked && anchorSet != nil && anchorSet.FlavorSet {
		if _, serr := f.Seek(seekAnchor.ByteOffset, io.SeekStart); serr != nil {
			seeked = false
		} else {
			startLine = seekAnchor.LineNo + 1
		}
	} else {
		seeked = false
	}
	if anchorSet == nil {
		anchorSet = &traceAnchorSet{}
	}
	recorder := newAnchorRecorder(anchorSet, seekAnchor, seeked)
	recording := recorder.canExtend(startLine)
	// R6 rule 4 (full_freq_curves.go): arm the full-file frequency-curve
	// collector, unless the per-file anchor record already carries a complete
	// collection (write-once trace attribute — a frequency curve is a property
	// of the FILE, not of one build's window), or this scan seeked past the
	// head and therefore can never publish a complete set.
	var fullFreqCollector *fullFreqCurveCollector
	if anchorSet.FullFreqSet {
		idx.fullFreq = anchorSet.FullFreq
	} else if !seeked {
		fullFreqCollector = newFullFreqCurveCollector(path)
	}

	frozenSource, err := frozenTraceSectionAtCurrentOffset(f, openedIdentity)
	if err != nil {
		return nil, err
	}
	r := bufio.NewReaderSize(frozenSource, 256*1024)
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	// W-1 修根 (platform_surfaces.go): all parsed events feed the platform
	// surface vote; the per-file anchor record (from-0 write-once) is the
	// single query-time authority for every view of this trace.
	platformVote := newPlatformSurfaceVote()
	buildReachedEOF := false
	seenTimeWindow := false
	lastParsedTs := float64(0)
	schedulerAuditCPU := newSchedulerOrderTracker()
	schedulerAuditPID := newSchedulerOrderTracker()
	durationAudit := newDurationOrderTracker()
	auditIntern := newStringInterner()
	auditScratch := &Index{}
	auditQ := Query{TimeStart: idx.IndexTimeStart, TimeEnd: idx.IndexTimeEnd, LineStart: idx.IndexLineStart, LineEnd: idx.IndexLineEnd}
	var scan lineScan
	for lineNo := startLine; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := readStreamScanPhysicalLine(r, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			scan.reset(lineNo, trimmed)
			// The window verdict is computed first because it decides which
			// interner/ledger the single shared parse below feeds. The gate is
			// pure over (lineNo, ts), so evaluating it before the audits cannot
			// change which lines the audits observe; every audit still runs on
			// skipped prefix rows (carry-in state) and on the stop row, exactly
			// as before.
			skip, stop := false, false
			if idx.Windowed {
				skip, stop = gate.decide(lineNo, &scan, &seenTimeWindow)
			}
			retained := !skip && !stop
			var durationEndpointRowFailure *durationOrderViolation
			if idx.Windowed && cpuInputRawCandidate(trimmed) {
				for _, failure := range cpuInputValidationFailuresScan(&scan) {
					if cpuInputIntegrityFailureRelevantToQuery(failure, auditQ) {
						failure.SourcePath = path
						appendCPUInputIntegrityFailure(idx, failure)
					}
				}
			}
			if failure := traceMarkValidationFailureScan(&scan); failure != nil && traceMarkIntegrityFailureRelevantToQuery(*failure, auditQ) {
				failure.SourcePath = path
				appendTraceMarkIntegrityFailure(idx, *failure)
			}
			if failure := blockedReasonValidationFailureScan(&scan); failure != nil && blockedReasonIntegrityFailureRelevantToQuery(failure, auditQ, 0) {
				failure.SourcePath = path
				appendBlockedReasonIntegrityFailure(idx, *failure)
			}
			durationCandidate := durationOrderRawCandidate(trimmed)
			if durationCandidate {
				if failure := durationEndpointRawValidationFailureScan(&scan); failure != nil && durationOrderViolationRelevantToQuery(failure, auditQ) {
					failure.SourcePath = path
					appendDurationOrderFailure(idx, *failure)
					durationEndpointRowFailure = failure
					if idx.Windowed {
						if failure.LaneKey != "" {
							durationAudit.clearLane(durationOrderLane{family: failure.Family, key: failure.LaneKey})
						} else {
							durationAudit.resetFamily(failure.Family)
						}
					}
				}
				// Perf audit #22: the interrupt endpoint validator only ever
				// returns a violation for the six irq/softirq/ipi entry/exit
				// names, so lines without those tokens skip its regex outright.
				if interruptEndpointRawCandidate(trimmed) {
					if failure := interruptEndpointValidationFailureScan(&scan); failure != nil && durationOrderViolationRelevantToQuery(failure, auditQ) {
						failure.SourcePath = path
						appendDurationOrderFailure(idx, *failure)
					}
				}
			}
			var rowIntegrityFailure *schedulerRowIntegrityFailure
			if idx.Windowed && schedulerIntegrityRawCandidate(trimmed) {
				rowIntegrityFailure = schedulerRowValidationFailureScan(&scan)
				if rowIntegrityFailure != nil && schedulerRowIntegrityFailureRelevantToQuery(rowIntegrityFailure, auditQ, 0) {
					rowIntegrityFailure.SourcePath = path
					appendSchedulerRowIntegrityFailure(idx, *rowIntegrityFailure)
				}
			}
			schedulerHeadCandidate := idx.Windowed && rowIntegrityFailure == nil && schedulerHeadRawCandidate(trimmed)
			// ONE Event parse per physical line: retained rows parse into the
			// main interner/ledger (exactly the rows the admission block below
			// consumes), while gate-skipped rows that a stateful audit needs
			// parse into the throwaway audit interner, as before. Both the
			// duration tracker and the scheduler-head audits consume this same
			// ev instead of re-parsing.
			panicsBefore := idx.ParseLinePanics
			var ev Event
			evOK := false
			// R6 rule 4 (full_freq_curves.go): a gate-skipped line still parses
			// for the full-file frequency-curve collection when the O(1)
			// prescreen hits — frequency rows are sparse, and the curve basis
			// must not be cropped by the window gate.
			fullFreqCandidate := fullFreqCollector != nil && !retained && fullFreqCurveRawCandidate(trimmed)
			if retained || (idx.Windowed && durationCandidate && durationEndpointRowFailure == nil) || schedulerHeadCandidate || fullFreqCandidate {
				if retained {
					ev, evOK = safeParseLineScan(&scan, intern, idx)
				} else {
					ev, evOK = safeParseLineScan(&scan, auditIntern, auditScratch)
				}
			}
			if evOK {
				// R6 rule 4: full-file curve collection sits BEFORE the window
				// skip, the relation-scope prune and the MaxEvents admission —
				// those gates crop idx.Events, never this side collection.
				fullFreqCollector.observe(ev)
				if !idx.Windowed {
					if failure := durationEndpointValidationFailureFromEvent(ev); failure != nil &&
						(failure.Family == durationOrderCPUFrequency || failure.Family == durationOrderCPUFreqLimit) {
						failure.SourcePath = path
						appendDurationOrderFailure(idx, *failure)
					}
				}
			}
			// A rejected outer ftrace envelope can still carry a uniquely typed
			// cpu_frequency / cpu_frequency_limits payload. The full-file curve
			// authority must see that barrier before ANY window skip/stop: otherwise
			// a cold scan can reach EOF, publish a clean-looking anchor, and let an
			// older value bridge across the malformed transition on a warm query.
			// Query-local disclosure remains independently window-scoped.
			if !evOK && trimmed != "" && durationCandidate {
				if failure := cpuScalarRejectedRowFailureScan(&scan); failure != nil {
					failure.SourcePath = path
					fullFreqCollector.observeFailure(failure)
					if durationOrderViolationRelevantToQuery(failure, auditQ) {
						appendDurationOrderFailure(idx, *failure)
					}
				}
			}
			if idx.Windowed && durationCandidate && durationEndpointRowFailure == nil && evOK {
				for _, failure := range durationAudit.observeAll(ev) {
					failure.SourcePath = path
					if durationOrderViolationRelevantToQuery(&failure, auditQ) {
						appendDurationOrderFailure(idx, failure)
					}
				}
			}
			if schedulerHeadCandidate {
				if evOK {
					if incarnationAuditSeedBoundary == 0 || ev.Ts >= incarnationAuditSeedBoundary {
						for _, conflict := range incarnationAudit.observeAll(ev, 0) {
							if incarnationBoundaryInsideQuery(&conflict, auditQ) {
								if len(idx.threadIncarnationFailures) < threadIncarnationFailureCap {
									idx.threadIncarnationFailures = append(idx.threadIncarnationFailures, conflict)
								} else {
									idx.threadIncarnationFailuresCapped = true
								}
							}
						}
					}
					lineInOrderDomain := (idx.IndexLineStart <= 0 || lineNo >= idx.IndexLineStart) && (idx.IndexLineEnd <= 0 || lineNo <= idx.IndexLineEnd)
					if lineInOrderDomain {
						for _, failure := range auditSchedulerOrderEvent(schedulerAuditCPU, schedulerAuditPID, ev) {
							failure.SourcePath = path
							if schedulerOrderViolationRelevantToQuery(&failure, auditQ, 0) {
								if len(idx.schedulerOrderFailures) < schedulerOrderFailureCap {
									idx.schedulerOrderFailures = append(idx.schedulerOrderFailures, failure)
								} else {
									idx.schedulerOrderFailuresCapped = true
								}
							}
						}
					}
				} else if rejected := schedulerRejectedRowFailureScan(&scan); rejected != nil && schedulerRowIntegrityFailureRelevantToQuery(rejected, auditQ, 0) {
					rejected.SourcePath = path
					appendSchedulerRowIntegrityFailure(idx, *rejected)
				}
			}
			if idx.Windowed {
				if recording {
					// The recorder's running max must still see EVERY line's
					// ts (memoized: at most one header match per line) or a
					// future time-window seek could jump past an in-window
					// line.
					lineTs, lineHasTS := scan.timestamp()
					recorder.observe(lineNo, len(line), lineTs, lineHasTS, trimmed)
				}
				if stop {
					break
				}
				if skip {
					if !seeked && lineNo <= 200 {
						flavor.observeRawLine(trimmed)
					}
					goto nextLine
				}
			} else if recording {
				lineTs, lineHasTS := scan.timestamp()
				recorder.observe(lineNo, len(line), lineTs, lineHasTS, trimmed)
			}
			flavor.observeRawLine(trimmed)
			if evOK {
				if prev := lastParsedTs; prev > 0 && ev.Ts > 0 && ev.Ts < prev {
					idx.ClockRegressions++
				}
				if ev.Ts > 0 {
					lastParsedTs = ev.Ts
				}
				if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
					idx.FirstTs = ev.Ts
				}
				if ev.Ts > idx.LastTs {
					idx.LastTs = ev.Ts
				}
				if ev.Type != EventUnknown {
					idx.ParsedKnown++
				}
				// Exact-storage SQLite rows are known converter syntax but
				// preservation-only. Count them before every query admission
				// gate, then discard so they cannot consume MaxEvents or gain
				// scheduler/span/causal authority merely by being present.
				if countTraceDBTextRecord(idx, ev) {
					goto nextLine
				}
				flavor.observeEvent(ev)
				platformVote.observe(ev)
				// Relation-scope pruning: keep only events the causal-chain
				// views actually consume for the target pid + its wakers (plus
				// all binder rows). Runs before the MaxEvents check so the cap
				// counts retained events, letting a dense window fit.
				if relScope != nil && !relScope.keep(&ev) {
					goto nextLine
				}
				if opts.MaxEvents > 0 && len(idx.Events) >= opts.MaxEvents {
					// Padding-tail graceful degrade (berlin.systrace, 2026-07-03):
					// the index window is the caller's request window plus safety
					// padding. When the budget trips only inside the padding
					// tail, a denial would throw away a fully usable core window
					// over an unparsed padding remainder — the customer's 101ms
					// request was completely parsed, yet the ±0.5s padding tail
					// tripped the cap and the model permanently lost the view.
					//
					// Degrade criterion — precise signals only:
					//   1. opts.TimeStartSet && opts.TimeEndSet — a half-open
					//      window has no TimeEnd to prove tail coverage against.
					//   2. ev.Ts > opts.TimeEnd — STRICTLY greater: the trigger
					//      event itself lies outside the requested window.
					//      ev.Ts == TimeEnd
					//      must NOT degrade — timeInWindow includes both
					//      endpoints, so that trigger is a real in-window match
					//      whose loss would silently drop an endpoint event
					//      (QF1); it stays on the hard-denial path.
					//   3. TimestampOrder == complete monotonic — a zero
					//      regression count in the parsed prefix is NOT proof
					//      about the unread suffix. If no cached EOF proof exists,
					//      completeTimestampOrderProof streams the suffix without
					//      retaining events; only an EOF-complete monotonic result
					//      authorizes truncating padding.
					//
					// The old idx.FirstTs <= opts.TimeStart conjunct was removed
					// as structurally redundant for head coverage: the window
					// gate skips every line below the padded start and the
					// anchor seek only jumps to anchors guaranteed to precede
					// the padded window, so parsing always begins at or before
					// the window head and the budget cannot eat head events.
					// Worse, the conjunct was ALWAYS false when no event exists
					// at or before TimeStart (trace starting mid-window, or
					// time_start=0), demoting a perfectly degradable shape to a
					// hard denial (QF3). The old idx.LastTs >= opts.TimeEnd
					// conjunct was unsound: LastTs already includes the trigger
					// event, so a trigger at exactly TimeEnd (or an early
					// out-of-order tail event pushing LastTs past TimeEnd)
					// claimed full coverage while in-window events were being
					// dropped (QF1).
					//
					// The relation-scope discovery pass shares windowGate but
					// never reaches this branch — the degrade lives solely on
					// this main-loop budget guard and gate semantics are
					// untouched. Requests failing any conjunct keep the
					// existing hard IndexEventLimitError.
					paddingCandidate := opts.TimeStartSet && opts.TimeEndSet && ev.Ts > opts.TimeEnd
					if paddingCandidate && !idx.TimestampOrder.AllowsTimeEndEarlyStop() {
						if proofErr := completeTimestampOrderProof(ctx, r, lineNo+1, err, idx, recorder, recording); proofErr != nil {
							if ctx.Err() != nil {
								return nil, proofErr
							}
							return nil, traceReadErrorAfterIdentity(f, openedIdentity, "timestamp proof physical read", proofErr)
						}
						idx.TimestampOrder = recorder.set.TimestampOrder
						if idx.TimestampOrder != TraceTimestampOrderUnknown {
							idx.ClockRegressions = recorder.set.CoveredClockRegressions
						}
					}
					if paddingCandidate && idx.TimestampOrder.AllowsTimeEndEarlyStop() {
						idx.PaddingTruncated = true
						idx.PaddingTruncatedLastTs = idx.LastTs
						idx.PaddingTruncatedNote = fmt.Sprintf(indexPaddingTruncatedNoteFmt, idx.LastTs)
						break
					}
					// A budget-denied build still scanned a valid contiguous
					// prefix: the anchors recorded so far are position facts
					// independent of the denial and remain valid for the
					// follow-up window_sweep / narrower-window retries the
					// denial itself recommends, so persist them instead of
					// discarding the scan work with the error. Flavor follows
					// the same rule as the success path: only a from-0 scan
					// publishes it (a seek-build's cache entry already has it),
					// and store() keeps the widest coverage / writes flavor
					// once, so this cannot regress an existing richer entry.
					if recording {
						if !seeked && !recorder.set.FlavorSet {
							recorder.set.Flavor, recorder.set.FlavorConf, recorder.set.FlavorSignals = flavor.result()
							recorder.set.FlavorSet = true
						}
						// 复核 F2: a budget-denied scan never reached EOF — its
						// parsed coverage is incomplete, so it mints NO platform
						// record (the next eligible complete scan will).
						anchorCache.store(anchorKey, recorder.set)
					}
					return nil, newIndexEventLimitError(path, idx, opts, lineNo, len(idx.Events))
				}
				idx.RetainedSideTableBytes += eventSideTableBytes(&ev)
				idx.Events = append(idx.Events, ev)
			} else if trimmed != "" {
				if durationEndpointRowFailure == nil {
					if rejected := durationEndpointRejectedRowFailureScan(&scan); rejected != nil && durationOrderViolationRelevantToQuery(rejected, auditQ) {
						rejected.SourcePath = path
						appendDurationOrderFailure(idx, *rejected)
					}
				}
				if rowIntegrityFailure == nil {
					rowIntegrityFailure = schedulerRowValidationFailureScan(&scan)
					if rowIntegrityFailure != nil && schedulerRowIntegrityFailureRelevantToQuery(rowIntegrityFailure, auditQ, 0) {
						rowIntegrityFailure.SourcePath = path
						appendSchedulerRowIntegrityFailure(idx, *rowIntegrityFailure)
					} else if rejected := schedulerRejectedRowFailureScan(&scan); rejected != nil && schedulerRowIntegrityFailureRelevantToQuery(rejected, auditQ, 0) {
						rejected.SourcePath = path
						appendSchedulerRowIntegrityFailure(idx, *rejected)
					}
				}
				if idx.ParseLinePanics == panicsBefore {
					idx.UnparsedLines++
				}
				// TDIAG B4 (§28.13): typed sample face — covers the no-format
				// arm above AND the panic arm (both are "cannot parse" to the
				// census); counters are untouched.
				idx.recordUnparsedSample(lineNo, trimmed)
			}
		}
	nextLine:
		if err != nil {
			if err == io.EOF {
				buildReachedEOF = true
				if recording {
					recorder.finishEOF()
				}
				break
			}
			return nil, traceReadErrorAfterIdentity(f, openedIdentity, "trace parsing physical read", err)
		}
	}
	// A cold window may establish the complete mutation audit only while
	// consuming its suffix (normal EOF or completeTimestampOrderProof). Import
	// that just-completed source-local ledger into THIS index as well as the
	// cache; otherwise only the next warm query would fail closed.
	applyAnchorPriorityMutationAudit(idx, recorder.set, path)
	// A duration-audit lane-budget overflow (durationOrderTrackerLaneBudget)
	// means some lanes of the family were never audited; fail-close the family
	// on the index exactly like a witness-ledger overflow.
	for family, capped := range durationAudit.capped {
		if capped {
			if idx.durationOrderFailuresCapped == nil {
				idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
			}
			idx.durationOrderFailuresCapped[family] = true
		}
	}
	// R6 rule 4: publish the full-file curves only when this scan provably
	// covered the whole file (from byte 0 to EOF); stamp the per-file anchor
	// record write-once so later seek/early-stop builds of the same file reuse
	// the full-file basis instead of falling back to their cropped event set.
	if fullFreqCollector != nil {
		idx.fullFreq = fullFreqCollector.finalize(!seeked && buildReachedEOF)
		if idx.fullFreq.collected && idx.fullFreq.samples <= fullFreqCurveAnchorSampleCap && !recorder.set.FullFreqSet {
			recorder.set.FullFreq = idx.fullFreq
			recorder.set.FullFreqSet = true
		}
	}
	if seeked && anchorSet.FlavorSet {
		// Seek-builds never see the file head, so the flavor vote would be
		// starved — reuse the from-0 scan's cached result (also making
		// flavor a stable per-file property across windows).
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = anchorSet.Flavor, anchorSet.FlavorConf, append([]string(nil), anchorSet.FlavorSignals...)
	} else {
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	}
	if recording {
		if !seeked && !recorder.set.FlavorSet {
			recorder.set.FlavorSet = true
			recorder.set.Flavor = idx.TraceFlavor
			recorder.set.FlavorConf = idx.FlavorConfidence
			recorder.set.FlavorSignals = append([]string(nil), idx.FlavorSignals...)
		}
		// W-1 修根 + 复核 F2: only a from-0 build with COMPLETE parsed-event
		// coverage (unwindowed, reached EOF) may mint the per-file record —
		// once, flavor discipline.
		if !seeked && !recorder.set.PlatformSurfaces.Set && buildReachedEOF && !opts.windowed() {
			recorder.set.PlatformSurfaces = platformVote.result(false)
		}
		anchorCache.store(anchorKey, recorder.set)
	}
	// W-1 修根: stamp the per-trace platform-detection record on the index —
	// the per-file anchor record is THE authority when present (window/view
	// independent); a build without one falls back to its own vote, marked
	// Scoped when the coverage was incomplete (复核 F3 discloses it).
	if anchorSet != nil && anchorSet.PlatformSurfaces.Set {
		idx.platformSurfaces = anchorSet.PlatformSurfaces.clone()
	} else if recorder.set.PlatformSurfaces.Set {
		idx.platformSurfaces = recorder.set.PlatformSurfaces.clone()
	} else {
		idx.platformSurfaces = platformVote.result(!(buildReachedEOF && !opts.windowed() && !seeked))
	}
	// Publish only the complete proof. When the cold window scan had no proof,
	// it continued to EOF and finishEOF filled this value. A warm monotonic
	// scan may stop early, but inherits the already-complete cached proof.
	idx.TimestampOrder = recorder.set.TimestampOrder
	if idx.TimestampOrder != TraceTimestampOrderUnknown {
		idx.ClockRegressions = recorder.set.CoveredClockRegressions
	}
	// Closed-lane history is needed only when the complete physical source did
	// not prove global timestamp monotonicity. A high-cardinality but monotonic
	// I/O trace must not lose every Block/Storage result merely because it saw
	// more than the bounded history cardinality.
	if idx.TimestampOrder != TraceTimestampOrderMonotonic {
		for family, capped := range durationAudit.pairingHistoryCapped {
			if !capped {
				continue
			}
			if idx.durationOrderFailuresCapped == nil {
				idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
			}
			idx.durationOrderFailuresCapped[family] = true
		}
	}
	idx.RetainedStringBytes = intern.retainedBytes
	if len(relScopeCaveats) > 0 {
		idx.Caveats = append(idx.Caveats, relScopeCaveats...)
	}
	idx.RelationScoped = relScope != nil
	if relScope != nil {
		idx.relationScopePriorityComplete = true
		idx.relationScopeTIDs = make(map[int]bool, len(relScope.relevantTids))
		for tid := range relScope.relevantTids {
			idx.relationScopeTIDs[tid] = true
		}
	}
	if len(idx.TraceArtifacts) == 0 {
		if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "trace parsing"); err != nil {
			return nil, err
		}
		source := singleTraceArtifactSourceWithIdentity(path, openedIdentity, idx.LineCount, len(idx.Events))
		source.timestampOrder = idx.TimestampOrder
		source.clockRegressions = idx.ClockRegressions
		idx.TraceArtifacts = []TraceArtifactSource{source}
	}
	return idx, nil
}

type traceBundleFile struct {
	Schema              string                          `json:"schema"`
	CaptureID           string                          `json:"capture_id"`
	Version             string                          `json:"version"`
	InputPath           string                          `json:"input_path"`
	ArchiveProvenance   *traceBundleArchiveProvenance   `json:"archive_provenance,omitempty"`
	Systrace            string                          `json:"systrace"`
	Artifacts           []traceBundleArtifact           `json:"artifacts"`
	ProviderDecisions   []traceBundleProviderDecision   `json:"provider_decisions"`
	TraceDecisions      []traceBundleTraceDecision      `json:"trace_provider_decisions"`
	TraceDBCoverage     []traceBundleCoverage           `json:"trace_db_coverage"`
	TraceCoverage       []traceBundleCoverage           `json:"trace_coverage"`
	TraceToolGates      []traceBundleTraceToolGate      `json:"trace_tool_gates"`
	PerfClockAlignments []traceBundlePerfClockAlignment `json:"perf_clock_alignments"`
	Caveats             []string                        `json:"caveats"`

	// schemaMode is derived exactly once from the held manifest bytes. It is
	// never decoded from JSON and prevents later path/spec helpers from
	// reclassifying an unknown or malformed schema as legacy.
	schemaMode traceBundleSchemaMode `json:"-"`
}

type traceBundleArchiveProvenance struct {
	Format        string `json:"format"`
	ArchiveBytes  int64  `json:"archive_bytes"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Member        string `json:"member"`
	MemberBytes   int64  `json:"member_bytes"`
	MemberSHA256  string `json:"member_sha256"`
	Selection     string `json:"selection"`
}

const traceBundleCoverageCaveatLimit = 24

// Keep one deterministic extra seat for each closed priority class when the
// source-order prefix is compacted. Trace coverage has one additional exact
// class (systrace receipt); DB coverage remains capped at the three classes it
// can legitimately carry. Fuzzy family/table matches never gain a seat.
const traceBundleCoveragePriorityCaveatLimit = 3
const traceBundleTraceCoveragePriorityCaveatLimit = 4

const traceBundleTraceToolGateCaveatLimit = 8
const traceBundleCapabilityDisclosureValueMaxBytes = 96

type traceBundleArtifact struct {
	Type          string                         `json:"type"`
	Path          string                         `json:"path"`
	Bytes         *int64                         `json:"bytes"`
	SHA256        string                         `json:"sha256"`
	Converter     string                         `json:"converter,omitempty"`
	Trace         *traceBundleTraceCapability    `json:"trace_capability,omitempty"`
	Perf          *traceBundlePerfCapability     `json:"perf_capability,omitempty"`
	PerfTransform *traceBundlePerfInputTransform `json:"perf_input_transform,omitempty"`
	Caveats       []string                       `json:"caveats,omitempty"`
}

type traceBundlePerfInputTransform struct {
	Profile            string `json:"profile"`
	SourceArtifactPath string `json:"source_artifact_path"`
	SourceFormat       string `json:"source_format"`
	SourceBytes        int64  `json:"source_bytes"`
	SourceSHA256       string `json:"source_sha256"`
	DecodedFormat      string `json:"decoded_format"`
	DecodedBytes       int64  `json:"decoded_bytes"`
	DecodedSHA256      string `json:"decoded_sha256"`
}

// traceBundleTraceCapability mirrors converter-owned manifest metadata for
// disclosure only. Unlike perfCapability on traceArtifactSpec, this value is
// never forwarded into artifact admission: the held child generation and its
// full parser result remain the sole query/causal authority.
type traceBundleTraceCapability struct {
	ProviderKind          string `json:"provider_kind"`
	ProviderName          string `json:"provider_name"`
	OutputFormat          string `json:"output_format"`
	ValidationProfile     string `json:"validation_profile"`
	Rows                  int    `json:"rows"`
	Known                 int    `json:"known"`
	AuthoritativeKnown    int    `json:"authoritative_known"`
	AdvisoryRows          int    `json:"advisory_rows"`
	IntentionalUnknown    int    `json:"intentional_unknown"`
	IntentionalHeaderOnly int    `json:"intentional_header_only"`
	TraceQueryReady       bool   `json:"trace_query_ready"`
}

type traceBundlePerfCapability struct {
	ProviderKind           string                                 `json:"provider_kind,omitempty"`
	ProviderName           string                                 `json:"provider_name,omitempty"`
	InputFormat            string                                 `json:"input_format,omitempty"`
	OutputFormat           string                                 `json:"output_format,omitempty"`
	TimeDomain             string                                 `json:"time_domain,omitempty"`
	TimeAlignment          string                                 `json:"time_alignment,omitempty"`
	ThreadIdentity         string                                 `json:"thread_identity,omitempty"`
	CPUIdentity            string                                 `json:"cpu_identity,omitempty"`
	EventWeight            string                                 `json:"event_weight,omitempty"`
	Symbolization          string                                 `json:"symbolization,omitempty"`
	Callchain              string                                 `json:"callchain,omitempty"`
	DSOLabel               string                                 `json:"dso_label,omitempty"`
	BuildID                string                                 `json:"build_id,omitempty"`
	OffCPU                 string                                 `json:"off_cpu,omitempty"`
	Confidence             string                                 `json:"confidence,omitempty"`
	TraceQueryReady        bool                                   `json:"trace_query_ready,omitempty"`
	Degraded               bool                                   `json:"degraded,omitempty"`
	RawCaptureCompleteness *traceBundleRawPerfCaptureCompleteness `json:"raw_perf_capture_completeness,omitempty"`
	RawCaptureResidual     *traceBundleRawPerfCaptureResidual     `json:"raw_perf_capture_residual,omitempty"`
	RawSampleAdmission     *traceBundleRawPerfSampleAdmission     `json:"raw_perf_sample_admission,omitempty"`
	Caveats                []string                               `json:"caveats,omitempty"`
}

type traceBundleProviderDecision struct {
	Stage           string `json:"stage,omitempty"`
	ProviderKind    string `json:"provider_kind,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	InputPath       string `json:"input_path,omitempty"`
	InputFormat     string `json:"input_format,omitempty"`
	OutputPath      string `json:"output_path,omitempty"`
	ParserMode      string `json:"parser_mode,omitempty"`
	Selected        bool   `json:"selected"`
	Attempted       bool   `json:"attempted"`
	Succeeded       bool   `json:"succeeded"`
	Fallback        bool   `json:"fallback"`
	TraceQueryReady bool   `json:"trace_query_ready"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Caveat          string `json:"caveat,omitempty"`
}

type traceBundleTraceDecision struct {
	Stage           string `json:"stage,omitempty"`
	ProviderKind    string `json:"provider_kind,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	InputPath       string `json:"input_path,omitempty"`
	OutputPath      string `json:"output_path,omitempty"`
	DBPath          string `json:"db_path,omitempty"`
	EngineMode      string `json:"engine_mode,omitempty"`
	Selected        bool   `json:"selected"`
	Attempted       bool   `json:"attempted"`
	Succeeded       bool   `json:"succeeded"`
	Fallback        bool   `json:"fallback"`
	TraceQueryReady bool   `json:"trace_query_ready"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Caveat          string `json:"caveat,omitempty"`
}

type traceBundleCoverage struct {
	Family                 string                                 `json:"family,omitempty"`
	ArtifactPath           string                                 `json:"artifact_path,omitempty"`
	Table                  string                                 `json:"table,omitempty"`
	Role                   string                                 `json:"role,omitempty"`
	Found                  bool                                   `json:"found"`
	FieldSources           map[string]string                      `json:"field_sources,omitempty"`
	ColumnsPresent         []string                               `json:"columns_present,omitempty"`
	ColumnsMissing         []string                               `json:"columns_missing,omitempty"`
	RowsRead               int                                    `json:"rows_read,omitempty"`
	RowsEmitted            int                                    `json:"rows_emitted,omitempty"`
	PeakBuffered           int                                    `json:"peak_buffered_rows,omitempty"`
	PeakBufferedBytes      uint64                                 `json:"peak_buffered_bytes,omitempty"`
	SpillChunks            int                                    `json:"spill_chunks,omitempty"`
	TempBytes              int64                                  `json:"temp_bytes,omitempty"`
	CurrentLiveTempBytes   uint64                                 `json:"current_live_temp_bytes,omitempty"`
	PeakLiveTempBytes      uint64                                 `json:"peak_live_temp_bytes,omitempty"`
	PeakOpenRunFDs         int                                    `json:"peak_open_run_fds,omitempty"`
	MergePasses            int                                    `json:"merge_passes,omitempty"`
	ElapsedUS              int64                                  `json:"elapsed_us,omitempty"`
	Skipped                string                                 `json:"skipped,omitempty"`
	Error                  string                                 `json:"error,omitempty"`
	CaptureCompleteness    *traceBundleCaptureCompleteness        `json:"capture_completeness,omitempty"`
	RawCaptureCompleteness *traceBundleRawPerfCaptureCompleteness `json:"raw_perf_capture_completeness,omitempty"`
	RawCaptureResidual     *traceBundleRawPerfCaptureResidual     `json:"raw_perf_capture_residual,omitempty"`
	RawSampleAdmission     *traceBundleRawPerfSampleAdmission     `json:"raw_perf_sample_admission,omitempty"`
}

type traceBundleCaptureCompleteness struct {
	State            string                                `json:"state"`
	RowsAccepted     int                                   `json:"rows_accepted,omitempty"`
	Received         uint64                                `json:"received,omitempty"`
	DataLost         uint64                                `json:"data_lost,omitempty"`
	NotMatch         uint64                                `json:"not_match,omitempty"`
	NotSupported     uint64                                `json:"not_supported,omitempty"`
	InvalidData      uint64                                `json:"invalid_data,omitempty"`
	InfoIssues       uint64                                `json:"info_issues,omitempty"`
	WarnIssues       uint64                                `json:"warn_issues,omitempty"`
	ErrorIssues      uint64                                `json:"error_issues,omitempty"`
	FatalIssues      uint64                                `json:"fatal_issues,omitempty"`
	NonzeroIssueRows int                                   `json:"nonzero_issue_rows,omitempty"`
	Issues           []traceBundleCaptureCompletenessIssue `json:"issues,omitempty"`
	IssuesCompacted  int                                   `json:"issues_compacted,omitempty"`
	IntegrityIssues  []string                              `json:"integrity_issues,omitempty"`
}

type traceBundleCaptureCompletenessIssue struct {
	EventName string `json:"event_name"`
	StatType  string `json:"stat_type"`
	Count     uint64 `json:"count"`
	Source    string `json:"source"`
	Severity  string `json:"severity"`
}

type traceBundleTraceToolGate struct {
	Name                 string   `json:"name,omitempty"`
	State                string   `json:"state,omitempty"`
	Proven               bool     `json:"proven"`
	FixtureManifestCount int      `json:"fixture_manifest_count,omitempty"`
	RequiredEvidence     string   `json:"required_evidence,omitempty"`
	Evidence             []string `json:"evidence,omitempty"`
	Caveats              []string `json:"caveats,omitempty"`
}

type traceBundlePerfClockAlignment struct {
	ArtifactPath    string   `json:"artifact_path,omitempty"`
	PerfTimeDomain  string   `json:"perf_time_domain,omitempty"`
	TraceTimeDomain string   `json:"trace_time_domain,omitempty"`
	OffsetSec       *float64 `json:"offset_sec,omitempty"`
	Slope           *float64 `json:"slope,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Calibrated      bool     `json:"calibrated"`
	Source          string   `json:"source,omitempty"`
	Caveats         []string `json:"caveats,omitempty"`
}

func traceBundlePath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json") && strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".tracebundle.json")
}

func promoteSiblingTraceBundlePath(path string) string {
	path = canonicalTraceIndexPath(path)
	if traceBundlePath(path) {
		return path
	}
	// A perftrace path is the explicit per-domain escape hatch. Promoting it
	// back to a sibling bundle would make an honestly isolated perf clock
	// impossible to query on its own (and contradict the isolation caveat).
	if strings.HasSuffix(strings.ToLower(path), ".perftrace") {
		return path
	}
	selection, err := resolveTraceIndexSelection(context.Background(), path)
	if err == nil {
		if selection.promoted {
			promoted := selection.indexPath
			if selection.close() == nil {
				return promoted
			}
			return path
		}
		_ = selection.close()
	}
	return path
}

func siblingTraceBundlePath(path string) string {
	selection, err := resolveTraceIndexSelection(context.Background(), path)
	if err == nil {
		if selection.promoted {
			promoted := selection.indexPath
			if selection.close() == nil {
				return promoted
			}
			return ""
		}
		_ = selection.close()
	}
	return ""
}

func siblingTraceArtifactPaths(path string) []string {
	if traceBundlePath(path) {
		return nil
	}
	if traceSourcePathIsBlockingNamespace(path) {
		return nil
	}
	base := traceArtifactBase(path)
	if base == "" {
		return nil
	}
	var suffixes []string
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".systrace"):
		suffixes = []string{".perftrace"}
	default:
		return nil
	}
	var out []string
	for _, suffix := range suffixes {
		candidate := base + suffix
		if filepath.Clean(candidate) == filepath.Clean(path) {
			continue
		}
		if identity, err := filegeneration.FromPath(candidate); err == nil && identity.Mode().IsRegular() {
			out = append(out, candidate)
		}
	}
	return out
}

func traceArtifactBase(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".systrace"):
		return path[:len(path)-len(".systrace")]
	case strings.HasSuffix(lower, ".perftrace"):
		return path[:len(path)-len(".perftrace")]
	default:
		return ""
	}
}

func parseTraceBundleFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
	selection, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return nil, err
	}
	if !selection.bundleSet || selection.indexPath != canonicalTraceIndexPath(path) {
		return nil, selection.closeAfter(fmt.Errorf("trace bundle selection did not preserve the explicit manifest"))
	}
	if selection.indexIdentity.Size() != size || selection.indexIdentity.ModUnixNano() != modUnix {
		return nil, selection.closeAfter(fmt.Errorf("trace bundle generation changed before parsing began"))
	}
	idx, err := parseTraceBundleSelection(ctx, selection, opts)
	if err == nil {
		err = selection.finish(ctx, idx)
	} else {
		err = selection.closeAfter(err)
	}
	return idx, err
}

func parseTraceBundleSelection(ctx context.Context, selection *traceIndexSelection, opts BuildOptions) (*Index, error) {
	if selection == nil || !selection.bundleSet {
		return nil, fmt.Errorf("trace bundle selection is missing its decoded manifest")
	}
	if len(selection.artifactSpecs) == 0 {
		return nil, fmt.Errorf("trace bundle %s has no systrace or perftrace artifacts", selection.indexPath)
	}
	idx, err := parseTraceArtifactSpecs(
		ctx,
		selection.indexPath,
		selection.indexIdentity.Size(),
		selection.indexIdentity.ModUnixNano(),
		opts,
		selection.artifactSpecs,
		&selection.universe,
	)
	if err != nil {
		return nil, err
	}
	idx.Caveats = append(idx.Caveats, traceBundleCaveats(selection.bundle)...)
	idx.Caveats = append(idx.Caveats, selection.caveats...)
	return idx, nil
}

func traceBundleCaveats(bundle traceBundleFile) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.Contains(s, traceBundleRawPerfResidualCaveatToken) ||
			strings.Contains(s, traceBundleRawPerfAdmissionCaveatToken) {
			return
		}
		if _, exists := seen[s]; exists {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	// R2a: the raw perf record census is one manifest-global quality advisory,
	// independent of the selected query window. Render it before free-form
	// artifact/provider metadata so a bounded consumer cannot hide inventory
	// status behind lower-priority provenance. The helper fail-closes the whole
	// raw projection on any Artifact/receipt mismatch and never affects sample
	// admission.
	for _, caveat := range traceBundleRawPerfCaptureCompletenessCaveats(bundle) {
		add(caveat)
	}
	for _, caveat := range bundle.Caveats {
		add("tracebundle_caveat: " + caveat)
	}
	for _, artifact := range bundle.Artifacts {
		kind := traceBundleLabel(artifact.Type, artifact.Path)
		for _, caveat := range artifact.Caveats {
			// R2c/a1a: these namespaces are compatibility mirrors of typed
			// raw-perf receipts. The typed projector above either emits one
			// bounded advisory or fail-closes it; publishing a free-form twin
			// here would duplicate valid values and leak forged values on invalid
			// bundles.
			if traceBundleRawPerfResidualCaveatReserved(caveat) ||
				traceBundleRawPerfAdmissionCaveatReserved(caveat) {
				continue
			}
			add(fmt.Sprintf("tracebundle_artifact %s caveat: %s", kind, caveat))
		}
		if artifact.Perf != nil {
			add(traceBundlePerfCapabilityCaveat(kind, *artifact.Perf))
			for _, caveat := range artifact.Perf.Caveats {
				// Only Artifact.Caveats may carry receipt-derived compatibility
				// mirrors. Filter both reserved namespaces from this wrong lane even
				// after the typed projector has failed closed.
				if traceBundleRawPerfResidualCaveatReserved(caveat) ||
					traceBundleRawPerfAdmissionCaveatReserved(caveat) {
					continue
				}
				add(fmt.Sprintf("tracebundle_artifact %s perf_capability_caveat: %s", kind, caveat))
			}
		}
		if artifact.Trace != nil {
			add(traceBundleTraceCapabilityCaveat(artifact))
		}
	}
	for _, decision := range bundle.ProviderDecisions {
		if !decision.Selected && !decision.Attempted && decision.Caveat == "" && decision.Reason == "" {
			continue
		}
		add(traceBundleProviderDecisionCaveat(decision))
	}
	for _, decision := range bundle.TraceDecisions {
		if !decision.Selected && !decision.Attempted && decision.Caveat == "" && decision.Reason == "" {
			continue
		}
		add(traceBundleTraceDecisionCaveat(decision))
	}
	for _, caveat := range traceBundleCoverageCaveats("tracebundle_trace_db_coverage", bundle.TraceDBCoverage) {
		add(caveat)
	}
	for _, caveat := range traceBundleCoverageCaveats("tracebundle_trace_coverage", bundle.TraceCoverage) {
		add(caveat)
	}
	for _, caveat := range traceBundleTraceToolGateCaveats("tracebundle_trace_tool_gate", bundle.TraceToolGates) {
		add(caveat)
	}
	for _, alignment := range bundle.PerfClockAlignments {
		if alignment.Confidence == "" && len(alignment.Caveats) == 0 {
			continue
		}
		add(traceBundleClockAlignmentCaveat(alignment))
		for _, caveat := range alignment.Caveats {
			add(fmt.Sprintf("tracebundle_perf_clock_alignment artifact=%s caveat: %s", traceBundlePathBase(alignment.ArtifactPath), caveat))
		}
	}
	return out
}

func traceBundleTraceCapabilityCaveat(artifact traceBundleArtifact) string {
	capability := artifact.Trace
	if capability == nil {
		return ""
	}
	effectiveKind := strings.ToLower(strings.TrimSpace(artifact.Type))
	if effectiveKind == "" {
		effectiveKind = inferTraceArtifactKind(artifact.Path)
	}
	// Keep the same precise suffix override as traceBundleArtifactSpecs. A
	// manifest cannot relabel a .perftrace child as systrace merely to make a
	// trace capability appear applicable.
	if inferTraceArtifactKind(artifact.Path) == "perftrace" {
		effectiveKind = "perftrace"
	}
	applicability := "systrace_advisory"
	if effectiveKind != "systrace" {
		applicability = "ignored_type_mismatch"
	}
	// Put the load-bearing authority clauses before every manifest-controlled
	// label. Tool banners intentionally bound long caveats, so an oversized
	// provider/path must never truncate away the fact that this declaration is
	// advisory and cannot mint query readiness.
	parts := []string{
		"tracebundle_trace_capability",
		"authority=manifest_advisory",
		"manifest_capability_hard_gate=false",
		"child_parse_authority=authoritative",
		fmt.Sprintf("declared_trace_query_ready=%t", capability.TraceQueryReady),
		"applicability=" + applicability,
		traceBundleCapabilityDisclosureValue(traceBundleLabel(artifact.Type, artifact.Path)),
	}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCapabilityDisclosureValue(value))
		}
	}
	appendKV("declared_provider", firstNonEmpty(capability.ProviderName, capability.ProviderKind))
	appendKV("declared_provider_kind", capability.ProviderKind)
	appendKV("declared_output", capability.OutputFormat)
	appendKV("declared_validation_profile", capability.ValidationProfile)
	parts = append(parts,
		fmt.Sprintf("declared_rows=%d", capability.Rows),
		fmt.Sprintf("declared_known=%d", capability.Known),
		fmt.Sprintf("declared_authoritative_known=%d", capability.AuthoritativeKnown),
		fmt.Sprintf("declared_advisory_rows=%d", capability.AdvisoryRows),
		fmt.Sprintf("declared_intentional_unknown=%d", capability.IntentionalUnknown),
		fmt.Sprintf("declared_intentional_header_only=%d", capability.IntentionalHeaderOnly),
	)
	return strings.Join(parts, " ")
}

// traceBundleCapabilityDisclosureValue bounds one manifest-controlled token
// before it reaches a caveat. The manifest itself has an intake budget, but a
// single large/control-bearing JSON value must not dominate a user-facing
// diagnostic or inject terminal control bytes. Truncation is byte-bounded and
// rune-safe so the caveat remains valid UTF-8 on every platform.
func traceBundleCapabilityDisclosureValue(value string) string {
	value = traceBundleCompactValue(value)
	if value == "" {
		return ""
	}
	var sanitized strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			r = '_'
		}
		sanitized.WriteRune(r)
	}
	value = sanitized.String()
	if len(value) <= traceBundleCapabilityDisclosureValueMaxBytes {
		return value
	}
	const suffix = "_truncated"
	cut := traceBundleCapabilityDisclosureValueMaxBytes - len(suffix)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + suffix
}

func traceBundlePerfCapabilityCaveat(kind string, perf traceBundlePerfCapability) string {
	parts := []string{"tracebundle_perf_capability", kind}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendKV("provider", firstNonEmpty(perf.ProviderName, perf.ProviderKind))
	appendKV("input", perf.InputFormat)
	appendKV("output", perf.OutputFormat)
	appendKV("time_domain", perf.TimeDomain)
	if perf.TraceQueryReady {
		appendKV("time_alignment", perf.TimeAlignment)
	} else {
		// Inventory-only capability metadata describes the producer declaration;
		// it is not effective clock evidence for any admitted sample.
		appendKV("declared_time_alignment", perf.TimeAlignment)
	}
	appendKV("thread_identity", perf.ThreadIdentity)
	appendKV("cpu_identity", perf.CPUIdentity)
	appendKV("event_weight", perf.EventWeight)
	appendKV("symbolization", perf.Symbolization)
	appendKV("callchain", perf.Callchain)
	appendKV("dso_label", perf.DSOLabel)
	appendKV("build_id", perf.BuildID)
	appendKV("off_cpu", perf.OffCPU)
	appendKV("confidence", perf.Confidence)
	parts = append(parts, fmt.Sprintf("trace_query_ready=%t", perf.TraceQueryReady))
	parts = append(parts, fmt.Sprintf("degraded=%t", perf.Degraded))
	return strings.Join(parts, " ")
}

func traceBundleProviderDecisionCaveat(decision traceBundleProviderDecision) string {
	parts := []string{"tracebundle_perf_provider"}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendKV("stage", decision.Stage)
	appendKV("provider", firstNonEmpty(decision.ProviderName, decision.ProviderKind))
	appendKV("input", decision.InputFormat)
	appendKV("parser", decision.ParserMode)
	appendKV("artifact", traceBundlePathBase(decision.ArtifactPath))
	parts = append(parts, fmt.Sprintf("selected=%t", decision.Selected))
	parts = append(parts, fmt.Sprintf("attempted=%t", decision.Attempted))
	parts = append(parts, fmt.Sprintf("succeeded=%t", decision.Succeeded))
	parts = append(parts, fmt.Sprintf("fallback=%t", decision.Fallback))
	parts = append(parts, fmt.Sprintf("trace_query_ready=%t", decision.TraceQueryReady))
	appendKV("reason", decision.Reason)
	appendKV("caveat", decision.Caveat)
	return strings.Join(parts, " ")
}

func traceBundleTraceDecisionCaveat(decision traceBundleTraceDecision) string {
	parts := []string{"tracebundle_trace_provider"}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendKV("stage", decision.Stage)
	appendKV("provider", firstNonEmpty(decision.ProviderName, decision.ProviderKind))
	appendKV("input", traceBundlePathBase(decision.InputPath))
	appendKV("output", traceBundlePathBase(decision.OutputPath))
	appendKV("db", traceBundlePathBase(decision.DBPath))
	appendKV("engine", decision.EngineMode)
	appendKV("artifact", traceBundlePathBase(decision.ArtifactPath))
	parts = append(parts, fmt.Sprintf("selected=%t", decision.Selected))
	parts = append(parts, fmt.Sprintf("attempted=%t", decision.Attempted))
	parts = append(parts, fmt.Sprintf("succeeded=%t", decision.Succeeded))
	parts = append(parts, fmt.Sprintf("fallback=%t", decision.Fallback))
	parts = append(parts, fmt.Sprintf("trace_query_ready=%t", decision.TraceQueryReady))
	appendKV("reason", decision.Reason)
	appendKV("caveat", decision.Caveat)
	return strings.Join(parts, " ")
}

func traceBundleTraceToolGateCaveats(prefix string, rows []traceBundleTraceToolGate) []string {
	if len(rows) == 0 {
		return nil
	}
	limit := traceBundleTraceToolGateCaveatLimit
	if len(rows) < limit {
		limit = len(rows)
	}
	out := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		out = append(out, traceBundleTraceToolGateCaveat(prefix, rows[i]))
	}
	if len(rows) > limit {
		out = append(out, fmt.Sprintf("%s_compacted total=%d emitted=%d", prefix, len(rows), limit))
	}
	return out
}

func traceBundleTraceToolGateCaveat(prefix string, gate traceBundleTraceToolGate) string {
	parts := []string{prefix}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendKV("name", gate.Name)
	appendKV("state", gate.State)
	parts = append(parts, fmt.Sprintf("proven=%t", gate.Proven))
	parts = append(parts, fmt.Sprintf("fixture_manifest_count=%d", gate.FixtureManifestCount))
	appendKV("required_evidence", gate.RequiredEvidence)
	appendKV("evidence", traceBundleCompactTextList(gate.Evidence, 8))
	appendKV("caveats", traceBundleCompactTextList(gate.Caveats, 8))
	return strings.Join(parts, " ")
}

func traceBundleCoverageCaveats(prefix string, rows []traceBundleCoverage) []string {
	if prefix == "tracebundle_trace_db_coverage" {
		rows = traceBundleNormalizeCaptureCoverage(rows)
	} else {
		rows = traceBundleDropWrongLaneCaptureCoverage(rows)
	}
	if len(rows) == 0 {
		return nil
	}
	limit := traceBundleCoverageCaveatLimit
	if len(rows) < limit {
		limit = len(rows)
	}
	priorityLimit := traceBundleCoveragePriorityCaveatLimitForPrefix(prefix)
	out := make([]string, 0, limit+priorityLimit+1)
	prioritySeen := make(map[string]struct{}, priorityLimit)
	for i := 0; i < limit; i++ {
		out = append(out, traceBundleCoverageCaveat(prefix, rows[i]))
		if class := traceBundleCoveragePriorityClassForPrefix(prefix, rows[i]); class != "" {
			prioritySeen[class] = struct{}{}
		}
	}
	priorityEmitted := 0
	for i := limit; i < len(rows) && priorityEmitted < priorityLimit; i++ {
		class := traceBundleCoveragePriorityClassForPrefix(prefix, rows[i])
		if class == "" {
			continue
		}
		if _, exists := prioritySeen[class]; exists {
			continue
		}
		out = append(out, traceBundleCoverageCaveat(prefix, rows[i]))
		prioritySeen[class] = struct{}{}
		priorityEmitted++
	}
	if len(rows) > limit {
		out = append(out, fmt.Sprintf("%s_compacted total=%d emitted=%d priority_emitted=%d", prefix, len(rows), limit, priorityEmitted))
	}
	return out
}

func traceBundleCoveragePriorityCaveatLimitForPrefix(prefix string) int {
	if prefix == "tracebundle_trace_coverage" {
		return traceBundleTraceCoveragePriorityCaveatLimit
	}
	return traceBundleCoveragePriorityCaveatLimit
}

func traceBundleDropWrongLaneCaptureCoverage(rows []traceBundleCoverage) []traceBundleCoverage {
	if len(rows) == 0 {
		return nil
	}
	out := make([]traceBundleCoverage, 0, len(rows))
	for _, row := range rows {
		if traceBundleCoveragePriorityClass(row) == "capture_completeness" {
			continue
		}
		out = append(out, row)
	}
	return out
}

func traceBundleCoveragePriority(coverage traceBundleCoverage) bool {
	return traceBundleCoveragePriorityClass(coverage) != ""
}

func traceBundleCoveragePriorityClassForPrefix(prefix string, coverage traceBundleCoverage) string {
	class := traceBundleCoveragePriorityClass(coverage)
	if class == "capture_completeness" && prefix != "tracebundle_trace_db_coverage" {
		return ""
	}
	if class == "perf_receipt" && prefix != "tracebundle_trace_coverage" {
		return ""
	}
	if class == "systrace_receipt" && prefix != "tracebundle_trace_coverage" {
		return ""
	}
	return class
}

func traceBundleCoveragePriorityClass(coverage traceBundleCoverage) string {
	if coverage.Family == "capture_completeness" && coverage.Table == "stat" && coverage.Role == "capture_completeness" {
		return "capture_completeness"
	}
	if coverage.Family == "resolver.lifecycle" && coverage.Table == "__authority__" {
		return "resolver_lifecycle_authority"
	}
	if coverage.Table == "__systrace_rows__" && coverage.Role == "systrace_text_output" &&
		(coverage.Family == "sorter" || coverage.Family == "builtin_modern_profiler") {
		return "systrace_row_sorter"
	}
	if tracebundle.IsPerfReceiptCoverage(coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath) {
		return "perf_receipt"
	}
	if tracebundle.IsSystraceReceiptCoverage(coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath) {
		return "systrace_receipt"
	}
	return ""
}

func traceBundleCoverageCaveat(prefix string, coverage traceBundleCoverage) string {
	if prefix == "tracebundle_trace_db_coverage" && traceBundleCoveragePriorityClass(coverage) == "capture_completeness" {
		return traceBundleCaptureCoverageCaveat(prefix, coverage)
	}
	parts := []string{prefix}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendInt := func(key string, value int) {
		if value != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendInt64 := func(key string, value int64) {
		if value != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendUint64 := func(key string, value uint64, includeZero bool) {
		if value != 0 || includeZero {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	rowSorter := traceBundleCoveragePriorityClass(coverage) == "systrace_row_sorter"
	appendKV("family", coverage.Family)
	appendKV("table", coverage.Table)
	appendKV("artifact", coverage.ArtifactPath)
	appendKV("role", coverage.Role)
	parts = append(parts, fmt.Sprintf("found=%t", coverage.Found))
	appendInt("rows_read", coverage.RowsRead)
	appendInt("rows_emitted", coverage.RowsEmitted)
	appendInt("peak_buffered_rows", coverage.PeakBuffered)
	appendUint64("peak_buffered_bytes", coverage.PeakBufferedBytes, rowSorter)
	appendInt("spill_chunks", coverage.SpillChunks)
	appendInt64("temp_bytes", coverage.TempBytes)
	appendUint64("current_live_temp_bytes", coverage.CurrentLiveTempBytes, rowSorter)
	appendUint64("peak_live_temp_bytes", coverage.PeakLiveTempBytes, rowSorter)
	if coverage.PeakOpenRunFDs != 0 || rowSorter {
		parts = append(parts, fmt.Sprintf("peak_open_run_fds=%d", coverage.PeakOpenRunFDs))
	}
	if coverage.MergePasses != 0 || rowSorter {
		parts = append(parts, fmt.Sprintf("merge_passes=%d", coverage.MergePasses))
	}
	appendInt64("elapsed_us", coverage.ElapsedUS)
	appendKV("field_sources", traceBundleCompactFieldSources(coverage.FieldSources, 8))
	appendKV("columns_missing", traceBundleCompactList(coverage.ColumnsMissing, 8))
	appendKV("columns_present", traceBundleCompactList(coverage.ColumnsPresent, 8))
	appendKV("skipped", coverage.Skipped)
	appendKV("error", coverage.Error)
	return strings.Join(parts, " ")
}

func traceBundleCaptureCoverageCaveat(prefix string, coverage traceBundleCoverage) string {
	parts := []string{prefix, "family=capture_completeness", "table=stat", "role=capture_completeness"}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleControlSafeToken(value))
		}
	}
	appendInt := func(key string, value int) {
		if value != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendUint64 := func(key string, value uint64) {
		if value != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendEncodedList := func(key, value string) {
		if value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	completeness := coverage.CaptureCompleteness
	if completeness == nil {
		completeness = traceBundleCaptureUnknown("invalid_bundle_capture_payload")
	}
	// These fixed tokens make the advisory/trust boundary unambiguous on the
	// exact line that the model sees. A tracebundle is mutable input, so even a
	// structurally clean self-audit never becomes a deterministic hard gate.
	appendKV("capture_state", completeness.State)
	appendKV("capture_scope", "trace_streamer_parser_self_audit")
	appendKV("capture_trust", "manifest_advisory")
	appendKV("capture_hard_gate", "false")
	appendKV("capture_absence_policy", "require_quality_caveat")
	appendKV("capture_positive_evidence", "preserve")
	appendKV("capture_received_proves_source_complete", "false")
	appendKV("capture_loss_scope", "global_absence_quality")
	appendKV("capture_not_match_scope", "context_dependent_absence_quality")
	appendKV("capture_other_scope", "global_absence_quality")
	parts = append(parts, fmt.Sprintf("found=%t", coverage.Found))
	appendInt("rows_read", coverage.RowsRead)
	appendInt("capture_rows_accepted", completeness.RowsAccepted)
	appendUint64("capture_received", completeness.Received)
	appendUint64("capture_data_lost", completeness.DataLost)
	appendUint64("capture_not_match", completeness.NotMatch)
	appendUint64("capture_not_supported", completeness.NotSupported)
	appendUint64("capture_invalid_data", completeness.InvalidData)
	appendUint64("capture_info_issues", completeness.InfoIssues)
	appendUint64("capture_warn_issues", completeness.WarnIssues)
	appendUint64("capture_error_issues", completeness.ErrorIssues)
	appendUint64("capture_fatal_issues", completeness.FatalIssues)
	appendInt("capture_nonzero_issue_rows", completeness.NonzeroIssueRows)
	appendInt("capture_issues_compacted", completeness.IssuesCompacted)
	appendEncodedList("capture_integrity_issues", traceBundleCaptureIntegrityIssues(completeness.IntegrityIssues, 8))
	appendEncodedList("capture_issue_examples", traceBundleCaptureIssueExamples(completeness.Issues, 8))
	return strings.Join(parts, " ")
}

const (
	traceBundleCaptureMaxRows       = 4096
	traceBundleCaptureMaxIssueRows  = 32
	traceBundleCaptureMaxEventBytes = 256
)

func traceBundleNormalizeCaptureCoverage(rows []traceBundleCoverage) []traceBundleCoverage {
	if len(rows) == 0 {
		return nil
	}
	captureCount := 0
	for _, row := range rows {
		if traceBundleCoveragePriorityClass(row) == "capture_completeness" {
			captureCount++
		}
	}
	out := make([]traceBundleCoverage, 0, len(rows)-maxInt(0, captureCount-1))
	captureEmitted := false
	for _, source := range rows {
		row := source
		if traceBundleCoveragePriorityClass(row) != "capture_completeness" {
			// A nested payload on any non-authority row is untrusted baggage,
			// never an alternate completeness authority.
			row.CaptureCompleteness = nil
			out = append(out, row)
			continue
		}
		if captureEmitted {
			continue
		}
		captureEmitted = true
		if captureCount > 1 {
			out = append(out, traceBundleCanonicalCaptureCoverage(false, 0, traceBundleCaptureUnknown("duplicate_capture_authority")))
			continue
		}
		if !traceBundleCaptureCompletenessValid(row) {
			out = append(out, traceBundleCanonicalCaptureCoverage(false, 0, traceBundleCaptureUnknown("invalid_bundle_capture_payload")))
			continue
		}
		out = append(out, traceBundleCanonicalCaptureCoverage(row.Found, row.RowsRead, row.CaptureCompleteness))
	}
	return out
}

func traceBundleCanonicalCaptureCoverage(found bool, rowsRead int, completeness *traceBundleCaptureCompleteness) traceBundleCoverage {
	return traceBundleCoverage{
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness",
		Found: found, RowsRead: rowsRead, CaptureCompleteness: completeness,
	}
}

func traceBundleCaptureUnknown(reason string) *traceBundleCaptureCompleteness {
	return &traceBundleCaptureCompleteness{State: "unknown", IntegrityIssues: []string{reason}}
}

func traceBundleCaptureCompletenessValid(coverage traceBundleCoverage) bool {
	capture := coverage.CaptureCompleteness
	if capture == nil || coverage.RowsRead < 0 || coverage.RowsRead > traceBundleCaptureMaxRows+1 ||
		coverage.RowsEmitted != 0 || coverage.Skipped != "" || coverage.Error != "" ||
		capture.RowsAccepted < 0 || capture.RowsAccepted > traceBundleCaptureMaxRows ||
		capture.NonzeroIssueRows < 0 || capture.IssuesCompacted < 0 || len(capture.Issues) > traceBundleCaptureMaxIssueRows {
		return false
	}
	switch capture.State {
	case "unknown":
		return traceBundleCaptureUnknownValid(capture)
	case "parser_self_audit_clean", "parser_self_audit_degraded":
		return traceBundleCaptureKnownValid(coverage, capture)
	default:
		return false
	}
}

func traceBundleCaptureUnknownValid(capture *traceBundleCaptureCompleteness) bool {
	if capture == nil || capture.RowsAccepted != 0 || capture.Received != 0 || capture.DataLost != 0 ||
		capture.NotMatch != 0 || capture.NotSupported != 0 || capture.InvalidData != 0 ||
		capture.InfoIssues != 0 || capture.WarnIssues != 0 || capture.ErrorIssues != 0 || capture.FatalIssues != 0 ||
		capture.NonzeroIssueRows != 0 || len(capture.Issues) != 0 || capture.IssuesCompacted != 0 ||
		len(capture.IntegrityIssues) == 0 || len(capture.IntegrityIssues) > 8 {
		return false
	}
	seen := make(map[string]struct{}, len(capture.IntegrityIssues))
	for _, reason := range capture.IntegrityIssues {
		if !traceBundleCaptureIntegrityReasonKnown(reason) {
			return false
		}
		if _, duplicate := seen[reason]; duplicate {
			return false
		}
		seen[reason] = struct{}{}
	}
	return true
}

func traceBundleCaptureKnownValid(coverage traceBundleCoverage, capture *traceBundleCaptureCompleteness) bool {
	if capture == nil || !coverage.Found || capture.RowsAccepted == 0 || capture.RowsAccepted%5 != 0 ||
		coverage.RowsRead != capture.RowsAccepted || len(capture.IntegrityIssues) != 0 ||
		capture.NonzeroIssueRows > (capture.RowsAccepted/5)*4 {
		return false
	}
	events := uint64(capture.RowsAccepted / 5)
	perTypeMax := events * math.MaxUint32
	for _, total := range []uint64{
		capture.Received, capture.DataLost, capture.NotMatch, capture.NotSupported, capture.InvalidData,
		capture.InfoIssues, capture.WarnIssues, capture.ErrorIssues, capture.FatalIssues,
	} {
		if total > perTypeMax*4 {
			return false
		}
	}
	if capture.Received > perTypeMax || capture.DataLost > perTypeMax || capture.NotMatch > perTypeMax ||
		capture.NotSupported > perTypeMax || capture.InvalidData > perTypeMax {
		return false
	}
	issueTotal, issueOK := traceBundleCheckedSum(capture.DataLost, capture.NotMatch, capture.NotSupported, capture.InvalidData)
	severityTotal, severityOK := traceBundleCheckedSum(capture.InfoIssues, capture.WarnIssues, capture.ErrorIssues, capture.FatalIssues)
	if !issueOK || !severityOK || issueTotal != severityTotal {
		return false
	}
	if capture.State == "parser_self_audit_clean" {
		return issueTotal == 0 && capture.NonzeroIssueRows == 0 && len(capture.Issues) == 0 && capture.IssuesCompacted == 0
	}
	if issueTotal == 0 || capture.NonzeroIssueRows == 0 {
		return false
	}
	wantVisible := minInt(capture.NonzeroIssueRows, traceBundleCaptureMaxIssueRows)
	if len(capture.Issues) != wantVisible || capture.IssuesCompacted != capture.NonzeroIssueRows-wantVisible {
		return false
	}
	exampleTypes := make(map[string]uint64, 4)
	exampleSeverities := make(map[string]uint64, 4)
	exampleKeys := make(map[string]struct{}, len(capture.Issues))
	uniqueEvents := make(map[string]struct{}, len(capture.Issues))
	for i, issue := range capture.Issues {
		if !traceBundleCaptureIssueValid(issue) || i > 0 && !traceBundleCaptureIssueLess(capture.Issues[i-1], issue) {
			return false
		}
		key := issue.EventName + "\x00" + issue.StatType
		if _, duplicate := exampleKeys[key]; duplicate {
			return false
		}
		exampleKeys[key] = struct{}{}
		uniqueEvents[issue.EventName] = struct{}{}
		if !traceBundleAddCaptureExampleTotal(exampleTypes, issue.StatType, issue.Count) ||
			!traceBundleAddCaptureExampleTotal(exampleSeverities, issue.Severity, issue.Count) {
			return false
		}
	}
	if len(uniqueEvents) > capture.RowsAccepted/5 {
		return false
	}
	wantTypes := map[string]uint64{
		"data_lost": capture.DataLost, "not_match": capture.NotMatch,
		"not_supported": capture.NotSupported, "invalid_data": capture.InvalidData,
	}
	wantSeverities := map[string]uint64{
		"info": capture.InfoIssues, "warn": capture.WarnIssues,
		"error": capture.ErrorIssues, "fatal": capture.FatalIssues,
	}
	for key, want := range wantTypes {
		if exampleTypes[key] > want || capture.IssuesCompacted == 0 && exampleTypes[key] != want {
			return false
		}
	}
	for key, want := range wantSeverities {
		if exampleSeverities[key] > want || capture.IssuesCompacted == 0 && exampleSeverities[key] != want {
			return false
		}
	}
	return true
}

func traceBundleAddCaptureExampleTotal(totals map[string]uint64, key string, value uint64) bool {
	if totals == nil || value > math.MaxUint64-totals[key] {
		return false
	}
	totals[key] += value
	return true
}

func traceBundleCaptureIssueValid(issue traceBundleCaptureCompletenessIssue) bool {
	if issue.EventName == "" || len(issue.EventName) > traceBundleCaptureMaxEventBytes ||
		issue.EventName != strings.TrimSpace(issue.EventName) || !utf8.ValidString(issue.EventName) ||
		issue.Count == 0 || issue.Count > math.MaxUint32 || issue.Source != "trace" {
		return false
	}
	for _, r := range issue.EventName {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	switch issue.StatType {
	case "data_lost", "not_match", "not_supported", "invalid_data":
	default:
		return false
	}
	switch issue.Severity {
	case "info", "warn", "error", "fatal":
		return true
	default:
		return false
	}
}

func traceBundleCaptureIssueLess(left, right traceBundleCaptureCompletenessIssue) bool {
	leftRank, rightRank := traceBundleCaptureSeverityRank(left.Severity), traceBundleCaptureSeverityRank(right.Severity)
	if leftRank != rightRank {
		return leftRank > rightRank
	}
	if left.Count != right.Count {
		return left.Count > right.Count
	}
	if left.EventName != right.EventName {
		return left.EventName < right.EventName
	}
	return left.StatType < right.StatType
}

func traceBundleCaptureSeverityRank(severity string) int {
	switch severity {
	case "fatal":
		return 4
	case "error":
		return 3
	case "warn":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func traceBundleCaptureIntegrityReasonKnown(reason string) bool {
	switch reason {
	case "missing_table", "missing_columns", "malformed_row", "duplicate_event_stat", "aggregate_overflow",
		"empty_table", "incomplete_event_status_set", "row_limit_exceeded",
		"duplicate_capture_authority", "invalid_bundle_capture_payload":
		return true
	default:
		return false
	}
}

func traceBundleCheckedSum(values ...uint64) (uint64, bool) {
	var total uint64
	for _, value := range values {
		if value > math.MaxUint64-total {
			return 0, false
		}
		total += value
	}
	return total, true
}

func traceBundleCaptureIssueExamples(issues []traceBundleCaptureCompletenessIssue, limit int) string {
	if len(issues) == 0 || limit <= 0 {
		return ""
	}
	if len(issues) < limit {
		limit = len(issues)
	}
	parts := make([]string, 0, limit+1)
	for _, issue := range issues[:limit] {
		parts = append(parts, fmt.Sprintf("%s:%s:%d:%s:%s",
			traceBundleControlSafeToken(issue.EventName), traceBundleControlSafeToken(issue.StatType), issue.Count,
			traceBundleControlSafeToken(issue.Severity), traceBundleControlSafeToken(issue.Source)))
	}
	if len(issues) > limit {
		parts = append(parts, fmt.Sprintf("+%d", len(issues)-limit))
	}
	return strings.Join(parts, ",")
}

func traceBundleCaptureIntegrityIssues(issues []string, limit int) string {
	if len(issues) == 0 || limit <= 0 {
		return ""
	}
	if len(issues) < limit {
		limit = len(issues)
	}
	parts := make([]string, 0, limit+1)
	for _, issue := range issues[:limit] {
		parts = append(parts, traceBundleControlSafeToken(issue))
	}
	if len(issues) > limit {
		parts = append(parts, fmt.Sprintf("+%d", len(issues)-limit))
	}
	return strings.Join(parts, ",")
}

func traceBundleControlSafeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	const hex = "0123456789ABCDEF"
	const encodedByteLimit = 253
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		b := value[i]
		safe := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-' || b == '.'
		width := 3
		if safe {
			width = 1
		}
		if out.Len()+width > encodedByteLimit {
			out.WriteString("...")
			break
		}
		if safe {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hex[b>>4])
		out.WriteByte(hex[b&0x0f])
	}
	if out.Len() == 0 {
		return "unknown"
	}
	return out.String()
}

func traceBundleCompactFieldSources(values map[string]string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	compacted := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		compactKey := traceBundleCompactValue(key)
		compactValue := traceBundleCompactValue(values[key])
		if compactKey == "" || compactValue == "" {
			continue
		}
		compacted = append(compacted, compactKey+":"+compactValue)
	}
	if limit > 0 && len(values) > limit {
		compacted = append(compacted, fmt.Sprintf("+%d", len(values)-limit))
	}
	return strings.Join(compacted, ",")
}

func traceBundleClockAlignmentCaveat(alignment traceBundlePerfClockAlignment) string {
	parts := []string{"tracebundle_perf_clock_alignment"}
	appendKV := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, key+"="+traceBundleCompactValue(value))
		}
	}
	appendKV("artifact", traceBundlePathBase(alignment.ArtifactPath))
	appendKV("perf_time_domain", alignment.PerfTimeDomain)
	appendKV("trace_time_domain", alignment.TraceTimeDomain)
	if alignment.OffsetSec != nil {
		parts = append(parts, fmt.Sprintf("offset_sec=%.9g", *alignment.OffsetSec))
	}
	if alignment.Slope != nil {
		parts = append(parts, fmt.Sprintf("slope=%.12g", *alignment.Slope))
	}
	appendKV("confidence", alignment.Confidence)
	parts = append(parts, fmt.Sprintf("calibrated=%t", alignment.Calibrated))
	appendKV("source", alignment.Source)
	return strings.Join(parts, " ")
}

func traceBundleCompactList(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if limit <= 0 || len(values) <= limit {
		return strings.Join(values, ",")
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, fmt.Sprintf("+%d", len(values)-limit))
	return strings.Join(out, ",")
}

func traceBundleCompactTextList(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = traceBundleCompactValue(value)
		if value != "" {
			compacted = append(compacted, value)
		}
	}
	return traceBundleCompactList(compacted, limit)
}

func traceBundleLabel(kind, path string) string {
	kind = traceBundleCompactValue(kind)
	base := traceBundlePathBase(path)
	if base == "" {
		return "type=" + kind
	}
	return "type=" + kind + " path=" + traceBundleCompactValue(base)
}

func traceBundlePathBase(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func traceBundleCompactValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.Join(strings.Fields(value), "_")
}

func parseTraceArtifactPathList(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions, artifactPaths []string) (*Index, error) {
	return parseTraceArtifactSpecs(ctx, path, size, modUnix, opts, traceArtifactSpecsForPaths(artifactPaths), nil)
}

// relationScopePriorityMerge records only children that actually contributed
// an admitted scheduler stream to the composite relation-scoped index. A
// causally isolated child was not parsed into Events, and a perftrace child is
// admitted under a sample-only capability after relation pruning is disabled;
// neither can vote on scheduler-priority closure. The composite proof is
// positive only when at least one eligible child voted and every such child
// published its parser-owned closure token.
type relationScopePriorityMerge struct {
	voters      int
	allComplete bool
}

func (m *relationScopePriorityMerge) observeAdmitted(source TraceArtifactSource, child *Index) {
	if m == nil || child == nil || !source.CausalCompatible ||
		strings.EqualFold(source.Kind, "perftrace") || !child.RelationScoped {
		return
	}
	if m.voters == 0 {
		m.allComplete = true
	}
	m.voters++
	m.allComplete = m.allComplete && child.relationScopePriorityComplete
}

func (m relationScopePriorityMerge) complete() bool {
	return m.voters > 0 && m.allComplete
}

func parseTraceArtifactSpecs(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions, artifactSpecs []traceArtifactSpec, universe *traceSourceUniverse) (*Index, error) {
	// A bundle owns manifest bytes in addition to its children.  A sibling
	// systrace/perftrace universe does not: the primary path is one of the
	// children and must not be counted twice.
	baseSize := int64(0)
	if traceBundlePath(path) {
		baseSize = size
	}
	idx := &Index{Path: path, Size: baseSize, ModTime: time.Unix(0, modUnix)}
	if opts.windowed() {
		idx.Windowed = true
		idx.IndexTimeStart = paddedTimeStart(opts)
		idx.IndexTimeEnd = paddedTimeEnd(opts)
		idx.IndexLineStart = paddedLineStart(opts)
		idx.IndexLineEnd = paddedLineEnd(opts)
	}
	var flavorSet bool
	virtualLineBase := 0
	// R6 rule 4: composite full-file curves — merged from the admitted
	// children's own complete collections, mapped into the canonical clock
	// domain (bundle provenance gate already vetted the mapping). Any child
	// without a complete collection, any unmappable sample, or a perf-kind
	// child (its event set passes a typed admission the side curves cannot)
	// degrades the composite to the historical events basis (fail-open).
	compositeFullFreq := fullFreqCurves{
		collected:        true,
		freqByCPU:        map[int][]freqSample{},
		limitByCPU:       map[int][]freqSample{},
		freqUnsafe:       map[int]bool{},
		limitUnsafe:      map[int]bool{},
		freqPoisonByCPU:  map[int]durationOrderViolation{},
		limitPoisonByCPU: map[int]durationOrderViolation{},
	}
	compositeFullFreqChildren := 0
	var relationScopePriority relationScopePriorityMerge
	for _, spec := range artifactSpecs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		artifactPath := spec.source.SourcePath
		var artifactIdentity traceFileIdentity
		if universe != nil {
			entry, ok := universe.entry(artifactPath)
			if !ok {
				return nil, fmt.Errorf("trace source universe omitted artifact %s", artifactPath)
			}
			artifactIdentity = entry.identity
		} else {
			var err error
			artifactIdentity, err = filegeneration.FromPath(artifactPath)
			if err != nil {
				return nil, fmt.Errorf("inspect trace bundle artifact %s: %w", artifactPath, err)
			}
		}
		source := spec.source
		source.SourceBytes = artifactIdentity.Size()
		source.SourceModUnixNano = artifactIdentity.ModUnixNano()
		source.sourceIdentity = artifactIdentity
		source.VirtualLineBase = virtualLineBase
		reserve, reserveErr := traceArtifactVirtualLineReserve(artifactIdentity.Size(), virtualLineBase)
		if reserveErr != nil {
			return nil, fmt.Errorf("trace bundle artifact %s: %w", artifactPath, reserveErr)
		}
		virtualLineBase += reserve
		if artifactIdentity.Size() > math.MaxInt64-idx.Size {
			return nil, fmt.Errorf("trace bundle artifact bytes overflow the composite size")
		}
		idx.Size += artifactIdentity.Size()
		artifactModTime := time.Unix(0, artifactIdentity.ModUnixNano())
		if artifactModTime.After(idx.ModTime) {
			idx.ModTime = artifactModTime
		}
		if !source.CausalCompatible {
			idx.TraceArtifacts = append(idx.TraceArtifacts, source)
			idx.Caveats = append(idx.Caveats, fmt.Sprintf(
				"tracebundle_clock_domain_isolated artifact=%s time_domain=%s canonical_time_domain=%s reason=%s; query the artifact directly for per-domain analysis",
				filepath.Base(source.SourcePath), source.TimeDomain, source.CanonicalTimeDomain, source.IsolationReason))
			continue
		}

		childOpts, intersects, mapErr := traceArtifactBuildOptions(opts, source)
		if mapErr != nil {
			return nil, fmt.Errorf("map canonical window into trace bundle artifact %s: %w", artifactPath, mapErr)
		}
		if !intersects {
			idx.TraceArtifacts = append(idx.TraceArtifacts, source)
			// R6 rule 4: a causally-compatible artifact skipped by the window
			// intersection was never read — the composite cannot claim
			// full-file curve coverage (incompatible artifacts above stay
			// excluded by the provenance gate and do NOT degrade).
			compositeFullFreq.collected = false
			continue
		}
		if strings.EqualFold(source.Kind, "perftrace") {
			// Relation discovery consumes scheduler rows to build a waker
			// closure. A perftrace capability does not attest scheduler-row
			// provenance, so an injected sched_wakeup must not expand that
			// closure before the post-parse perf-only admission gate can remove
			// it. Parse the bounded perf window without relation pruning; typed
			// sample identity is scrubbed/admitted immediately below.
			childOpts.RelationScoped = false
		}
		child, err := parseSingleTraceFile(ctx, artifactPath, artifactIdentity.Size(), artifactIdentity.ModUnixNano(), childOpts, artifactIdentity)
		if err != nil {
			return nil, fmt.Errorf("parse trace bundle artifact %s: %w", artifactPath, err)
		}
		if childSource, ok := traceArtifactSourceForPath(child.TraceArtifacts, artifactPath); ok {
			// Bind the composite ledger to the descriptor actually parsed, rather
			// than the pathname stat that merely preceded Open.
			source.SourceBytes = childSource.SourceBytes
			source.SourceModUnixNano = childSource.SourceModUnixNano
			source.sourceIdentity = childSource.sourceIdentity
		}
		observedEventCount := len(child.Events)
		if strings.EqualFold(source.Kind, "perftrace") {
			var admission perfBundleAdmissionSummary
			child.Events, admission = applyPerfBundleAdmission(child.Events, spec.perfCapability)
			idx.Caveats = append(idx.Caveats, admission.caveat(source.SourcePath))
			// The discarded rows have no authority to poison scheduler,
			// duration, or task-incarnation lanes through their child-local
			// audit side records. Re-audit only the admitted perf_sample set
			// below (which has no scheduler state transitions).
			child.schedulerRowIntegrityFailures = nil
			child.schedulerRowIntegrityFailuresCapped = false
			child.schedulerRowIntegrityOverflowSources = nil
			child.schedulerRowIntegrityOverflowGlobal = false
			child.priorityMutationIntegrityFailuresCapped = false
			child.priorityMutationIntegrityOverflowSources = nil
			child.priorityMutationIntegrityOverflowGlobal = false
			child.blockedReasonIntegrityFailures = nil
			child.blockedReasonIntegrityFailuresCapped = false
			child.blockedReasonIntegrityOverflow = blockedReasonIntegrityOverflowScope{}
			child.blockedReasonIdentityOverflow = blockedReasonIntegrityOverflowScope{}
			child.cpuInputIntegrityFailures = nil
			child.cpuInputIntegrityFailuresCapped = false
			child.traceMarkIntegrityFailures = nil
			child.traceMarkIntegrityFailuresCapped = false
			child.traceMarkIntegrityDroppedGlobalPoison = false
			child.traceTrackIntegrityDroppedPoison = false
			child.schedulerOrderFailures = nil
			child.schedulerOrderFailuresCapped = false
			child.durationOrderFailures = nil
			child.durationOrderFailuresCapped = nil
			child.threadIncarnationFailures = nil
			child.threadIncarnationFailuresCapped = false
		}
		childAuditQ := Query{
			TimeStart: child.IndexTimeStart, TimeEnd: child.IndexTimeEnd,
			LineStart: child.IndexLineStart, LineEnd: child.IndexLineEnd,
		}
		childRowIntegrityFailures := append([]schedulerRowIntegrityFailure(nil), child.schedulerRowIntegrityFailures...)
		childRowIntegrityCapped := child.schedulerRowIntegrityFailuresCapped
		childPriorityMutationIntegrityCapped := child.priorityMutationIntegrityFailuresCapped
		childBlockedReasonFailures := append([]blockedReasonIntegrityFailure(nil), child.blockedReasonIntegrityFailures...)
		childBlockedReasonCapped := child.blockedReasonIntegrityFailuresCapped
		childBlockedReasonOverflow := child.blockedReasonIntegrityOverflow.clone()
		childBlockedReasonIdentityOverflow := child.blockedReasonIdentityOverflow.clone()
		childCPUInputFailures := append([]cpuInputIntegrityFailure(nil), child.cpuInputIntegrityFailures...)
		childCPUInputCapped := child.cpuInputIntegrityFailuresCapped
		childTraceMarkFailures := append([]traceMarkIntegrityFailure(nil), child.traceMarkIntegrityFailures...)
		childTraceMarkCapped := child.traceMarkIntegrityFailuresCapped
		childTraceMarkDroppedGlobalPoison := child.traceMarkIntegrityDroppedGlobalPoison
		childTraceTrackDroppedPoison := child.traceTrackIntegrityDroppedPoison
		childSchedulerFailures := append([]schedulerOrderViolation(nil), child.schedulerOrderFailures...)
		childSchedulerCapped := child.schedulerOrderFailuresCapped
		reauditedSchedulerFailures, reauditSchedulerCapped := schedulerOrderFailuresFromEvents(child.Events, childAuditQ, 0, schedulerOrderFailureCap)
		childSchedulerCapped = childSchedulerCapped || reauditSchedulerCapped
		for _, failure := range reauditedSchedulerFailures {
			duplicate := false
			for _, existing := range childSchedulerFailures {
				if existing.Lane == failure.Lane && existing.ID == failure.ID && existing.Line == failure.Line && existing.PreviousTs == failure.PreviousTs && existing.CurrentTs == failure.CurrentTs {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			if len(childSchedulerFailures) >= schedulerOrderFailureCap {
				childSchedulerCapped = true
				continue
			}
			childSchedulerFailures = append(childSchedulerFailures, failure)
		}
		childDurationFailures := append([]durationOrderViolation(nil), child.durationOrderFailures...)
		childDurationCapped := cloneDurationOrderCapped(child.durationOrderFailuresCapped)
		reauditedDurationFailures, reauditDurationCapped := durationOrderFailuresFromEvents(child.Events, childAuditQ, durationOrderFailureCap)
		durationAuditHolder := &Index{
			durationOrderFailures:       childDurationFailures,
			durationOrderFailuresCapped: childDurationCapped,
		}
		mergeDurationOrderFailures(durationAuditHolder, reauditedDurationFailures, reauditDurationCapped)
		childDurationFailures = durationAuditHolder.durationOrderFailures
		childDurationCapped = durationAuditHolder.durationOrderFailuresCapped
		childIdentityFailures := append([]threadIncarnationConflict(nil), child.threadIncarnationFailures...)
		childIdentityCapped := child.threadIncarnationFailuresCapped
		reauditedIdentityFailures, reauditIdentityCapped := threadIncarnationConflictsFromEvents(child.Events, childAuditQ, threadIncarnationFailureCap)
		childIdentityFailures, childIdentityCapped = mergeThreadIncarnationFailures(
			childIdentityFailures, childIdentityCapped,
			reauditedIdentityFailures, reauditIdentityCapped, threadIncarnationFailureCap)
		source.LocalLineCount = child.LineCount
		source.timestampOrder = child.TimestampOrder
		source.clockRegressions = child.ClockRegressions
		// EventCount is physical parsed-event inventory for this artifact.
		// The index/result EventCount remains len(idx.Events), i.e. only rows
		// admitted to the shared stream. tracebundle_perf_admission discloses
		// both counts when they differ.
		source.EventCount = observedEventCount
		idx.TraceArtifacts = append(idx.TraceArtifacts, source)
		idx.LineCount += child.LineCount
		idx.ScannedLineCount += child.ScannedLineCount
		idx.ParseLinePanics += child.ParseLinePanics
		idx.ClockRegressions += child.ClockRegressions
		idx.UnparsedLines += child.UnparsedLines
		// TDIAG B4: merged bundles keep the first-cap samples in artifact
		// order. Rebase their line to the same virtual coordinate as Events;
		// ResolveArtifactSpans remains the one path back to local lines.
		for _, sample := range child.UnparsedSamples {
			if len(idx.UnparsedSamples) >= IndexUnparsedSampleCap {
				break
			}
			sample.Line += source.VirtualLineBase
			idx.UnparsedSamples = append(idx.UnparsedSamples, sample)
		}
		// Audit #39 (§29.25 处置委托 2026-07-10): child parse-time disclosures
		// (today: the relation-scope seed-resolution failure) must survive the
		// composite merge — dropping them silently erased the only record that
		// a thread selector failed to resolve at parse time. Exact-duplicate
		// strings from sibling children collapse.
		for _, caveat := range child.Caveats {
			duplicate := false
			for _, existing := range idx.Caveats {
				if existing == caveat {
					duplicate = true
					break
				}
			}
			if !duplicate {
				idx.Caveats = append(idx.Caveats, caveat)
			}
		}
		idx.ParsedKnown += child.ParsedKnown
		idx.RelationScoped = idx.RelationScoped || child.RelationScoped
		relationScopePriority.observeAdmitted(source, child)
		if len(child.relationScopeTIDs) > 0 {
			if idx.relationScopeTIDs == nil {
				idx.relationScopeTIDs = map[int]bool{}
			}
			for tid := range child.relationScopeTIDs {
				idx.relationScopeTIDs[tid] = true
			}
		}
		// Honest cache accounting on the merged index: the children carry
		// the retained string/side-table bytes for the events merged below.
		// (String bytes were dropped here before P4; that under-charged
		// bundle indexes in the LRU.)
		idx.RetainedStringBytes += child.RetainedStringBytes
		idx.RetainedSideTableBytes += child.RetainedSideTableBytes
		if !strings.EqualFold(source.Kind, "perftrace") && (!flavorSet || child.FlavorConfidence > idx.FlavorConfidence) {
			idx.TraceFlavor = child.TraceFlavor
			idx.FlavorConfidence = child.FlavorConfidence
			idx.FlavorSignals = append([]string(nil), child.FlavorSignals...)
			flavorSet = true
		}
		var affineCanonicalSources map[uint64]uint64
		if source.ClockAlignment == TraceClockAlignmentAffine {
			affineCanonicalSources = make(map[uint64]uint64, len(child.Events))
		}
		for i := range child.Events {
			sourceTs := child.Events[i].Ts
			child.Events[i].Line += source.VirtualLineBase
			mapped, ok := source.toCanonicalTsChecked(sourceTs)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s clock mapping cannot safely and reversibly represent timestamp at local line %d", artifactPath, child.Events[i].Line-source.VirtualLineBase)
			}
			if source.ClockAlignment == TraceClockAlignmentAffine {
				// The inverse scan bounds above are conservative. Canonicalize a
				// mapped event that lands within the checked-map ULP allowance of an
				// explicit inclusive query boundary so the later query gate cannot
				// drop a mathematically exact boundary event due only to rounding.
				if opts.TimeStartSet && traceClockRoundTripWithinULPs(opts.TimeStart, mapped) {
					mapped = opts.TimeStart
				} else if opts.TimeEndSet && traceClockRoundTripWithinULPs(opts.TimeEnd, mapped) {
					mapped = opts.TimeEnd
				}
			}
			if affineCanonicalSources != nil {
				canonicalBits := math.Float64bits(mapped)
				sourceBits := math.Float64bits(sourceTs)
				if previousSourceBits, exists := affineCanonicalSources[canonicalBits]; exists && previousSourceBits != sourceBits {
					return nil, fmt.Errorf("trace bundle artifact %s affine clock mapping collapses distinct source timestamps at local line %d", artifactPath, child.Events[i].Line-source.VirtualLineBase)
				}
				affineCanonicalSources[canonicalBits] = sourceBits
			}
			child.Events[i].Ts = mapped
		}
		if childRowIntegrityCapped {
			markSchedulerRowIntegrityOverflow(idx, source.SourcePath)
		}
		if childPriorityMutationIntegrityCapped {
			markPriorityMutationIntegrityOverflow(idx, source.SourcePath)
		}
		for _, childFailure := range childRowIntegrityFailures {
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			mapped, ok := source.toCanonicalTsChecked(failure.Ts)
			if !ok && !schedulerPriorityMutationEventName(failure.EventName) {
				return nil, fmt.Errorf("trace bundle artifact %s scheduler incomplete-row timestamp is not safely representable in the canonical clock", artifactPath)
			}
			if ok {
				failure.Ts = mapped
			} else {
				// Exact malformed priority mutation with no usable timestamp is
				// source-global range poison; it is not a scheduler-state row and
				// therefore must survive the bundle without fabricating a time.
				failure.Ts = math.NaN()
			}
			appendSchedulerRowIntegrityFailure(idx, failure)
		}
		if childBlockedReasonCapped {
			mappedOverflow, ok := mapBlockedReasonIntegrityOverflowScope(childBlockedReasonOverflow, source)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s blocked-reason overflow scope is not safely representable in the canonical clock", artifactPath)
			}
			idx.blockedReasonIntegrityFailuresCapped = true
			idx.blockedReasonIntegrityOverflow.merge(mappedOverflow)
			mappedIdentityOverflow, ok := mapBlockedReasonIntegrityOverflowScope(childBlockedReasonIdentityOverflow, source)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s blocked-reason identity overflow scope is not safely representable in the canonical clock", artifactPath)
			}
			idx.blockedReasonIdentityOverflow.merge(mappedIdentityOverflow)
		}
		for _, childFailure := range childBlockedReasonFailures {
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			mapped, ok := source.toCanonicalTsChecked(failure.Ts)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s blocked-reason integrity timestamp is not safely representable in the canonical clock", artifactPath)
			}
			failure.Ts = mapped
			appendBlockedReasonIntegrityFailure(idx, failure)
		}
		if childCPUInputCapped {
			idx.cpuInputIntegrityFailuresCapped = true
		}
		for _, childFailure := range childCPUInputFailures {
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			mapped, ok := source.toCanonicalTsChecked(failure.Ts)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s invalid-CPU witness timestamp is not safely representable in the canonical clock", artifactPath)
			}
			failure.Ts = mapped
			appendCPUInputIntegrityFailure(idx, failure)
		}
		if childTraceMarkCapped {
			idx.traceMarkIntegrityFailuresCapped = true
		}
		idx.traceMarkIntegrityDroppedGlobalPoison = idx.traceMarkIntegrityDroppedGlobalPoison || childTraceMarkDroppedGlobalPoison
		idx.traceTrackIntegrityDroppedPoison = idx.traceTrackIntegrityDroppedPoison || childTraceTrackDroppedPoison
		for _, childFailure := range childTraceMarkFailures {
			failure := childFailure
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			if failure.TimestampKnown {
				mapped, ok := source.toCanonicalTsChecked(failure.Ts)
				if !ok {
					return nil, fmt.Errorf("trace bundle artifact %s malformed trace-mark witness timestamp is not safely representable in the canonical clock", artifactPath)
				}
				failure.Ts = mapped
			}
			appendTraceMarkIntegrityFailure(idx, failure)
		}
		if childSchedulerCapped {
			idx.schedulerOrderFailuresCapped = true
		}
		for _, childFailure := range childSchedulerFailures {
			if len(idx.schedulerOrderFailures) >= schedulerOrderFailureCap {
				idx.schedulerOrderFailuresCapped = true
				continue
			}
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			previous, previousOK := source.toCanonicalTsChecked(failure.PreviousTs)
			current, currentOK := source.toCanonicalTsChecked(failure.CurrentTs)
			if !previousOK || !currentOK {
				return nil, fmt.Errorf("trace bundle artifact %s scheduler rollback boundary is not safely representable in the canonical clock", artifactPath)
			}
			failure.PreviousTs, failure.CurrentTs = previous, current
			idx.schedulerOrderFailures = append(idx.schedulerOrderFailures, failure)
		}
		for family, capped := range childDurationCapped {
			if capped {
				if idx.durationOrderFailuresCapped == nil {
					idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
				}
				idx.durationOrderFailuresCapped[family] = true
			}
		}
		for _, childFailure := range childDurationFailures {
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			if !failure.TsUnknown {
				previous, previousOK := source.toCanonicalTsChecked(failure.PreviousTs)
				current, currentOK := source.toCanonicalTsChecked(failure.CurrentTs)
				if !previousOK || !currentOK {
					return nil, fmt.Errorf("trace bundle artifact %s duration rollback boundary is not safely representable in the canonical clock", artifactPath)
				}
				failure.PreviousTs, failure.CurrentTs = previous, current
			}
			appendDurationOrderFailure(idx, failure)
		}
		if childIdentityCapped {
			idx.threadIncarnationFailuresCapped = true
		}
		for _, childFailure := range childIdentityFailures {
			if len(idx.threadIncarnationFailures) >= threadIncarnationFailureCap {
				idx.threadIncarnationFailuresCapped = true
				continue
			}
			failure := childFailure
			failure.LocalPreviousLine = childFailure.PreviousLine
			failure.LocalBoundaryLine = childFailure.BoundaryLine
			failure.PreviousLine += source.VirtualLineBase
			failure.BoundaryLine += source.VirtualLineBase
			if failure.PriorDeadLine > 0 {
				failure.PriorDeadLine += source.VirtualLineBase
			}
			failure.SourcePath = source.SourcePath
			previous, previousOK := source.toCanonicalTsChecked(failure.PreviousTs)
			boundary, boundaryOK := source.toCanonicalTsChecked(failure.BoundaryTs)
			if !previousOK || !boundaryOK {
				return nil, fmt.Errorf("trace bundle artifact %s lifecycle predecessor or boundary is not safely representable in the canonical clock", artifactPath)
			}
			failure.PreviousTs = previous
			failure.BoundaryTs = boundary
			if failure.PriorDead {
				priorDead, priorDeadOK := source.toCanonicalTsChecked(failure.PriorDeadTs)
				if !priorDeadOK {
					return nil, fmt.Errorf("trace bundle artifact %s lifecycle termination is not safely representable in the canonical clock", artifactPath)
				}
				failure.PriorDeadTs = priorDead
			}
			idx.threadIncarnationFailures = append(idx.threadIncarnationFailures, failure)
		}
		if opts.MaxEvents > 0 && len(child.Events) > 0 && len(idx.Events)+len(child.Events) > opts.MaxEvents {
			return nil, newIndexEventLimitError(path, idx, opts, child.Events[0].Line, len(idx.Events)+len(child.Events))
		}
		// A padding-tail-degraded child must keep its typed marker on the
		// merged index, or the multi-artifact path would silently present a
		// truncated build as complete.
		if child.PaddingTruncated {
			idx.PaddingTruncated = true
			// Child note timestamps are source-domain values.  Re-render from
			// the canonical typed boundary so text and JSON cannot disagree on
			// an affine-mapped artifact.
			mapped, ok := source.toCanonicalTsChecked(child.PaddingTruncatedLastTs)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s clock mapping cannot safely and reversibly represent padding boundary", artifactPath)
			}
			idx.PaddingTruncatedLastTs = mapped
			idx.PaddingTruncatedNote = fmt.Sprintf(indexPaddingTruncatedNoteFmt, idx.PaddingTruncatedLastTs)
		}
		if source.ClockAlignment == TraceClockAlignmentAffine {
			offset, slope := traceClockMapValues(source.ClockOffsetSec, source.ClockSlope)
			idx.Caveats = append(idx.Caveats, fmt.Sprintf(
				"tracebundle_clock_alignment_applied artifact=%s source_domain=%s canonical_domain=%s formula=canonical_ts=source_ts*%.12g%+.12g",
				filepath.Base(source.SourcePath), source.TimeDomain, source.CanonicalTimeDomain, slope, offset))
		}
		// W-1 修根: the composite's platform-detection record is the union of
		// its children's per-file records (each a write-once from-0 authority)
		// — every view of the bundle then answers with one label.
		idx.platformSurfaces = mergePlatformSurfaceScans(idx.platformSurfaces, child.platformSurfaces)
		// R6 rule 4: merge this child's complete full-file curves into the
		// canonical domain; any gap degrades the composite set (fail-open).
		compositeFullFreqChildren++
		if compositeFullFreq.collected {
			if !child.fullFreq.collected || strings.EqualFold(source.Kind, "perftrace") {
				compositeFullFreq.collected = false
			} else {
				mergeCompositeFullFreqCurves(&compositeFullFreq, child.fullFreq, source)
			}
		}
		idx.Events = append(idx.Events, child.Events...)
	}
	// RelationScoped is a view marker; it is not itself a closure proof. Only
	// the positive AND of admitted scheduler-child votes may authorize closed
	// priority ranges on a composite index.
	idx.relationScopePriorityComplete = relationScopePriority.complete()
	// R6 rule 4: publish the composite curves only when every admitted child
	// contributed a complete, cleanly-mapped collection.
	if compositeFullFreqChildren > 0 && compositeFullFreq.collected {
		finalizeCompositeFullFreqCurves(&compositeFullFreq)
		idx.fullFreq = compositeFullFreq
	}
	sort.SliceStable(idx.Events, func(i, j int) bool {
		if idx.Events[i].Ts == idx.Events[j].Ts {
			return idx.Events[i].Line < idx.Events[j].Line
		}
		return idx.Events[i].Ts < idx.Events[j].Ts
	})
	// Preserve the child-local physical audits above, then add the lifecycle
	// conflicts visible only after causally compatible artifacts are merged.
	// In particular, an old occupant may appear only in the primary systrace
	// while its sched_wakeup_new creation edge is carried by a companion trace.
	mergedAuditQ := Query{TimeStart: idx.IndexTimeStart, TimeEnd: idx.IndexTimeEnd, LineStart: idx.IndexLineStart, LineEnd: idx.IndexLineEnd}
	mergedIdentityFailures, mergedIdentityCapped := threadIncarnationConflictsFromEvents(idx.Events, mergedAuditQ, threadIncarnationFailureCap)
	idx.threadIncarnationFailures, idx.threadIncarnationFailuresCapped = mergeThreadIncarnationFailures(
		idx.threadIncarnationFailures, idx.threadIncarnationFailuresCapped,
		mergedIdentityFailures, mergedIdentityCapped, threadIncarnationFailureCap)
	// Included artifacts are mapped into the canonical clock domain and then
	// deterministically sorted, so query-side time_end stops over this merged
	// event slice are safe even if a child's physical line order regressed.
	idx.TimestampOrder = TraceTimestampOrderMonotonic
	for _, ev := range idx.Events {
		if ev.Ts > 0 && (idx.FirstTs == 0 || ev.Ts < idx.FirstTs) {
			idx.FirstTs = ev.Ts
		}
		if ev.Ts > idx.LastTs {
			idx.LastTs = ev.Ts
		}
	}
	if len(idx.TraceArtifacts) > 1 {
		idx.Caveats = append(idx.Caveats, "tracebundle_virtual_line_coordinates=true; use trace_artifacts/source_spans to resolve every global line to an artifact-local line")
	}
	return idx, nil
}

func traceBundleIndexPaths(bundlePath string, bundle traceBundleFile) []string {
	specs := traceBundleArtifactSpecs(bundlePath, bundle)
	paths := make([]string, 0, len(specs))
	for _, spec := range specs {
		paths = append(paths, spec.source.SourcePath)
	}
	return paths
}

type traceBundleArtifactPathResolver struct {
	baseDir  string
	resolved map[string]string
}

func newTraceBundleArtifactPathResolver(baseDir string) *traceBundleArtifactPathResolver {
	return &traceBundleArtifactPathResolver{
		baseDir:  baseDir,
		resolved: make(map[string]string),
	}
}

func (r *traceBundleArtifactPathResolver) resolve(rawPath string) string {
	p := strings.TrimSpace(rawPath)
	if r == nil || p == "" {
		return ""
	}
	if resolved, ok := r.resolved[p]; ok {
		return resolved
	}
	resolved := resolveTraceBundleArtifactPathUncached(r.baseDir, p)
	r.resolved[p] = resolved
	return resolved
}

func resolveTraceBundleArtifactPath(baseDir, p string) string {
	return newTraceBundleArtifactPathResolver(baseDir).resolve(p)
}

func resolveTraceBundleArtifactPathUncached(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return canonicalTraceIndexPath(p)
	}
	// Bundle-relative is the primary contract. Prefer it over an accidental
	// same-named file in the process CWD; otherwise provenance could point at
	// and parse a different capture than the manifest names. Retain the old
	// CWD-relative form only as a compatibility fallback when the bundle-local
	// target does not exist.
	// The chosen absolute path is frozen in traceIndexSelection. If a later
	// call observes a newly created bundle-local target, that different path is
	// part of a different source-universe cache token and cannot reuse the
	// earlier CWD-fallback index.
	bundleRelative := filepath.Clean(filepath.Join(baseDir, p))
	if identity, err := filegeneration.FromPath(bundleRelative); err == nil && identity.Mode().IsRegular() {
		return canonicalTraceIndexPath(bundleRelative)
	}
	if identity, err := filegeneration.FromPath(p); err == nil && identity.Mode().IsRegular() {
		return canonicalTraceIndexPath(p)
	}
	return canonicalTraceIndexPath(bundleRelative)
}

func paddedTimeStart(opts BuildOptions) float64 {
	if !opts.TimeStartSet {
		return 0
	}
	start := opts.TimeStart - opts.TimePaddingBefore
	if start < 0 {
		return 0
	}
	return start
}

func paddedTimeEnd(opts BuildOptions) float64 {
	if !opts.TimeEndSet {
		return 0
	}
	return opts.TimeEnd + opts.TimePaddingAfter
}

func paddedLineStart(opts BuildOptions) int {
	if opts.LineStart <= 0 {
		return 0
	}
	start := opts.LineStart - opts.LinePaddingBefore
	if start < 1 {
		return 1
	}
	return start
}

func paddedLineEnd(opts BuildOptions) int {
	if opts.LineEnd <= 0 {
		return 0
	}
	return opts.LineEnd + opts.LinePaddingAfter
}

func parseLineTimestamp(line string) (float64, bool) {
	// Timestamp extraction is a hard gate for window admission, EOF-complete
	// monotonicity proof and anchor seeking.  It therefore uses the exact same
	// anchored ftrace header grammar as ParseLine.  A timestamp-looking token
	// in comm/field text must never be promoted into trace time.
	if mark, ok := parseExactTraceMark(line); ok {
		return float64(mark.TimestampNS) / 1e9, true
	}
	if mark, ok := parseCPUUnavailableTraceMark(line); ok {
		return float64(mark.TimestampNS) / 1e9, true
	}
	if wakeup, ok := parseCPUUnavailableWakeup(line); ok {
		return float64(wakeup.TimestampNS) / 1e9, true
	}
	if interval, ok := parseCompletedAsyncInterval(line); ok {
		return float64(interval.StartTimestampNS) / 1e9, true
	}
	if relation, ok := parseFrameMapRelation(line); ok {
		return float64(relation.TimestampNS) / 1e9, true
	}
	m := matchFtraceLine(line)
	if len(m) == 0 {
		return 0, false
	}
	ts, ok := parseTraceTimestampSeconds(m[5])
	if !ok {
		return 0, false
	}
	return ts, true
}

func ParseLine(lineNo int, line string, intern *stringInterner) (Event, bool) {
	var scan lineScan
	scan.reset(lineNo, line)
	return parseLineScan(&scan, intern)
}

// ProbeEventNamePrefix returns the event token only when the supplied prefix
// contains a complete, structurally anchored ftrace header and a terminated
// event name. It intentionally does not parse the body. Callers may therefore
// census an oversized line from a bounded prefix without allocating or
// scanning the entire physical row.
func ProbeEventNamePrefix(prefix string) (string, bool) {
	match := matchFtraceLineIndex(prefix)
	// Full match plus seven capture groups => 16 indexes. Group 6 is the
	// event token and occupies indexes 12/13.
	if len(match) < 16 || match[12] < 0 || match[13] <= match[12] {
		return "", false
	}
	raw := prefix[match[12]:match[13]]
	terminated := strings.HasSuffix(raw, ":")
	if !terminated && match[13] < len(prefix) {
		switch prefix[match[13]] {
		case ' ', '\t', '\r', '\n':
			terminated = true
		}
	}
	if !terminated {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimSpace(raw), ":")
	return name, name != ""
}

// ProbeExactEventNamePrefix is the hard-endpoint variant of
// ProbeEventNamePrefix. It requires the event column's physical trailing ':';
// whitespace termination or a missing delimiter remains useful inventory but
// cannot open a pairing lane.
func ProbeExactEventNamePrefix(prefix string) (string, bool) {
	match := matchFtraceLineIndex(prefix)
	if len(match) == 0 {
		match = loosePhysicalFtraceLineIndex(prefix)
	}
	name, _, ok := exactEventNameFromPrefixMatch(prefix, match)
	return name, ok
}

// ProbeLeadingExactEventNamePrefix is the physical-origin variant used when a
// bounded prefix must retain an exact endpoint despite an unsafe/oversized body
// suffix. The header itself must begin at byte zero after ASCII spaces only and
// contain no UTF-8 control/Zl/Zp bytes. This rejects a loose rightmost header
// embedded behind protobuf/NUL metadata without requiring the body tail to be
// publishable.
func ProbeLeadingExactEventNamePrefix(prefix string) (string, bool) {
	match := matchFtraceLineIndex(prefix)
	if len(match) == 0 {
		match = loosePhysicalFtraceLineIndex(prefix)
	}
	name, headerEnd, ok := exactEventNameFromPrefixMatch(prefix, match)
	if !ok || len(match) < 4 || match[2] < 0 || headerEnd <= match[2] {
		return "", false
	}
	commStart := 0
	for commStart < len(prefix) && prefix[commStart] == ' ' {
		commStart++
	}
	if match[2] != commStart || !utf8.ValidString(prefix[:headerEnd]) {
		return "", false
	}
	for _, r := range prefix[:headerEnd] {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", false
		}
	}
	return name, true
}

func exactEventNameFromPrefixMatch(prefix string, match []int) (name string, headerEnd int, ok bool) {
	if len(match) < 16 || match[12] < 0 || match[13] <= match[12] {
		return "", 0, false
	}
	raw := prefix[match[12]:match[13]]
	if strings.HasSuffix(raw, ":") {
		name = strings.TrimSuffix(raw, ":")
		headerEnd = match[13]
	} else if match[13] < len(prefix) && prefix[match[13]] == ':' {
		// The loose physical grammar keeps the delimiter outside group 6 so
		// malformed scalar rows retain only header/family provenance.
		name = raw
		headerEnd = match[13] + 1
	} else {
		return "", 0, false
	}
	if name == "" || name != strings.TrimSpace(name) {
		return "", 0, false
	}
	return name, headerEnd, true
}

// PhysicalFtraceHeaderProbe is a source-neutral observation of the elected
// outer physical ftrace header. HeaderKnown is represented by the boolean return;
// TimestampKnown and OwnerKnown are separate because malformed timestamp/CPU/PID scalars must
// still stop body text from being reinterpreted as a second top-level header.
// This probe grants no endpoint or pairing authority.
type PhysicalFtraceHeaderProbe struct {
	EventName      string
	TimestampNS    uint64
	TimestampKnown bool
	HeaderTID      int64
	OwnerKnown     bool
}

// ProbePhysicalFtraceHeader locates one complete outer header shape using the
// same bounded comm authority as ParseLine, while tolerating malformed scalar
// values for quarantine/census adapters. Arbitrary "timestamp:event" prose is
// rejected because the full comm-PID/TGID/CPU/flags envelope is mandatory.
func ProbePhysicalFtraceHeader(line string) (PhysicalFtraceHeaderProbe, bool) {
	match := matchFtraceLine(line)
	if len(match) == 0 {
		match = loosePhysicalFtraceLine(line)
	}
	if len(match) < 8 {
		return PhysicalFtraceHeaderProbe{}, false
	}
	name := strings.TrimSuffix(strings.TrimSpace(match[6]), ":")
	if name == "" {
		return PhysicalFtraceHeaderProbe{}, false
	}
	ts, timestampKnown := parseTraceTimestampNanoseconds(match[5])
	pid, ownerKnown := parseFtraceHeaderTID(match[2])
	if !ownerKnown {
		pid = 0
	}
	return PhysicalFtraceHeaderProbe{
		EventName: name, TimestampNS: ts, TimestampKnown: timestampKnown,
		HeaderTID: int64(pid), OwnerKnown: ownerKnown,
	}, true
}

// parseLineScan is ParseLine over the shared per-line memo: the header match
// and parseKV computed by the window gate or any physical-row audit are reused
// here instead of being recomputed (perf audit #21).
func parseLineScan(s *lineScan, intern *stringInterner) (Event, bool) {
	lineNo := s.lineNo
	if mark, ok := parseExactTraceMark(s.line); ok {
		return exactTraceMarkEvent(lineNo, mark, intern), true
	}
	if mark, ok := parseCPUUnavailableTraceMark(s.line); ok {
		return cpuUnavailableTraceMarkEvent(lineNo, mark, intern), true
	}
	if wakeup, ok := parseCPUUnavailableWakeup(s.line); ok {
		return cpuUnavailableWakeupEvent(lineNo, wakeup, intern), true
	}
	if interval, ok := parseCompletedAsyncInterval(s.line); ok {
		return completedAsyncIntervalEvent(lineNo, interval, intern), true
	}
	if relation, ok := parseFrameMapRelation(s.line); ok {
		return frameMapRelationEvent(lineNo, relation, intern), true
	}
	if record, ok := parseTraceDBTextRecord(s.line); ok {
		return traceDBTextRecordEvent(lineNo, record, intern), true
	}
	m := s.match()
	if len(m) == 0 {
		return Event{}, false
	}
	pid, pidOK := parseFtraceHeaderTID(m[2])
	if !pidOK {
		// Header PID is an identity, not a magnitude. Overflow/non-decimal input
		// must never collapse to the valid idle identity 0.
		return Event{}, false
	}
	// TGID is optional grouping metadata, not the physical row owner. Invalid,
	// negative, placeholder, or out-of-int32 values degrade to unknown instead
	// of depending on the host's native int width.
	tgid, _ := parseFtraceHeaderTID(m[3])
	cpu, cpuPresent, cpuValid, _ := parseTraceCPUScalar(m[4])
	if !cpuPresent || !cpuValid {
		// The row-header CPU participates in scheduler, IRQ, resource and perf
		// attribution. Parse it as an identity scalar, not through atoi: a
		// decimal overflow must not collapse to the valid CPU 0. A globally
		// invalid identity cannot be safely narrowed to one family, so reject
		// the event; the physical scan retains a bounded cpu_input_invalid
		// witness for the query caveat.
		return Event{}, false
	}
	ts, tsOK := s.timestamp()
	if !tsOK {
		// ParseLine and parseLineTimestamp are both admission gates. Keeping
		// them on one finite parser prevents a range-overflow timestamp from
		// entering the event index while the window/ordering scan rejects it.
		return Event{}, false
	}
	comm := strings.TrimSpace(m[1])
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	rawFields := m[7]
	fields := strings.TrimSpace(rawFields)
	classificationFields := fields
	if isPrintFamilyRaw(strings.ToLower(rawType)) {
		// Marker classification and typed admission consume the same full-right
		// carrier. Generic key/value families retain their historical TrimSpace
		// view; only the print family owns opaque edge bytes.
		classificationFields = trimTraceMarkEnvelopeLeft(rawFields)
	}
	ev := Event{
		Line:      lineNo,
		Ts:        ts,
		CPU:       cpu,
		Comm:      intern.intern(comm),
		PID:       pid,
		TGID:      tgid,
		Type:      classifyEventType(comm, rawType, classificationFields),
		Name:      intern.intern(rawType),
		FieldText: intern.intern(clampString(fields, 300)),
	}
	ev.SubsystemKind = intern.intern(classifySubsystemKind(rawType, classificationFields, ev.Type))
	kv := s.keyValues()
	if schedulerRowValidationFailureScan(s) != nil {
		// Critical scheduler identities are presence-sensitive. Returning false
		// keeps an absent/invalid PID from silently materializing as the valid
		// idle PID 0; the physical scan records the typed fail-closed witness.
		return Event{}, false
	}
	if s.schedulerTyped.SuppressWakeEdge {
		// A legacy success=0 row is a valid scheduler observation of a no-op
		// wake attempt, not a successful wake transition. Preserve it for raw
		// event search/census without granting wake-chain or generation authority.
		ev.Type = EventUnknown
		ev.CPUInputInvalid = len(s.schedulerTyped.CPUIssues) != 0
		return ev, true
	}
	switch ev.Type {
	case EventSchedSwitch:
		// The sched_switch-specific structural parser owns both comm values and
		// the optional suffix. Re-reading them through schedCommValue/parseKV
		// would reopen key-looking comm text as a second authority.
		ev.PrevComm = intern.intern(kv["prev_comm"])
		ev.PrevPID = atoi(kv["prev_pid"])
		ev.PrevPrio = atoi(kv["prev_prio"])
		ev.PrevState = intern.intern(kv["prev_state"])
		ev.NextComm = intern.intern(kv["next_comm"])
		ev.NextPID = atoi(kv["next_pid"])
		ev.NextPrio = atoi(kv["next_prio"])
		ev.NextInfo = intern.intern(kv["next_info"])
		populateHarmonyNextInfoFields(&ev)
		ev.CGroup = intern.intern(firstNonEmpty(kv["cg"], kv["cgroup"]))
	case EventSchedWakeup, EventSchedWaking:
		ev.WakeeComm = intern.intern(kv["comm"])
		ev.WakeePID = atoi(kv["pid"])
		ev.WakeePrio = atoi(kv["prio"])
		switch source := strings.TrimSpace(kv["codrax_prio_source"]); source {
		case "":
		case WakeePrioritySourceInferredNextSchedSlice:
			ev.WakeePrioInferred = true
		case WakeePrioritySourceUnknown:
			ev.WakeePrioUnknown = true
		default:
			// Unknown provenance must never regain exact-priority authority.
			ev.WakeePrioInferred = true
			ev.WakeePrioUnknown = true
		}
		if s.schedulerTyped.WakePriorityUnknown {
			ev.WakeePrioUnknown = true
		}
		if ev.WakeePrioUnknown && !ev.WakeePrioInferred {
			ev.WakeePrio = 0
		}
		setEventTargetCPU(&ev, kv["target_cpu"])
		if len(s.schedulerTyped.CPUIssues) != 0 {
			ev.CPUInputInvalid = true
		}
	case EventPriorityMutation:
		// A unique canonical subject narrows the poison to that TID. Zero is
		// the deliberate artifact-global poison for an unscoped mutation; this
		// does not invalidate the independent scheduler state ledger.
		ev.WakeePID = s.schedulerTyped.PriorityMutationPID
	case EventSchedBlockedReason:
		// sched_blocked_reason is occurrence-aware: a malformed subject cannot
		// bind to a thread, while iowait and delay degrade independently instead
		// of collapsing to the valid zero values.
		if !s.schedulerTyped.BlockedPIDKnown {
			ev.Type = EventUnknown
			return ev, true
		}
		ev.WakeePID = atoi(kv["pid"])
		ev.BlockedReasonIOWaitKnown = s.schedulerTyped.BlockedIOWaitKnown
		if ev.BlockedReasonIOWaitKnown {
			ev.IOWait = int32(atoi(kv["iowait"]))
		}
		ev.Reason = intern.intern(blockedReasonSemanticCaller(kv))
		// 件1 census 根修 (2026-07-13): the vendor delay field, RAW as
		// printed. The occurrence-aware parser distinguishes a supported absent
		// field from an invalid explicit zero and rejects noncanonical/duplicate values;
		// the int32 slot is a core-size ratchet measure (see Event's comment).
		ev.BlockedDelayKnown = s.schedulerTyped.BlockedDelayKnown
		if ev.BlockedDelayKnown {
			ev.BlockedDelay = int32(atoi(kv["delay"]))
		}
	case EventSchedStat:
		ev.SchedStatFields = &SchedStatFields{
			Kind:    intern.intern(strings.TrimPrefix(strings.ToLower(rawType), "sched_stat_")),
			Comm:    intern.intern(kv["comm"]),
			PID:     atoi(kv["pid"]),
			DelayNs: atoi64(kv["delay"]),
			RunNs:   atoi64(kv["runtime"]),
			VRunNs:  atoi64(kv["vruntime"]),
		}
	case EventCPUIdle:
		s.cpuScalarTyped.apply(&ev, intern)
	case EventCPUFrequency:
		s.cpuScalarTyped.apply(&ev, intern)
		if rawType != "clock_set_rate" {
			ev.ClockName = intern.intern(rawType)
		}
	case EventCPUFrequencyLimit:
		s.cpuScalarTyped.apply(&ev, intern)
		ev.ClockName = intern.intern(rawType)
	case EventCPUConstraint:
		ev.ConstraintFields = &ConstraintFields{}
		populateCPUConstraintFields(&ev, rawType, kv, intern)
	case EventClockSetRate:
		s.cpuScalarTyped.apply(&ev, intern)
	case EventTraceMark:
		// The ftrace envelope consumes delimiter whitespace on the left, but the
		// payload's right edge remains producer data. Parse the complete marker
		// before FieldText is bounded, and keep the bounded inventory copy from
		// the same untrimmed-right bytes so endpoint identities cannot collapse.
		markerFields := trimTraceMarkEnvelopeLeft(rawFields)
		ev.FieldText = intern.intern(clampString(markerFields, 300))
		parsed := parseTraceMarkValidated(markerFields)
		if parsed.counterParsed && traceCounterRawPayloadAtCap(rawFields, normalizeTraceMarkPayload(markerFields)) {
			parsed.counter = traceCounterSample{issueReason: "counter_payload_too_long"}
			parsed.spanPID, parsed.name, parsed.value = 0, "", ""
		}
		ev.SpanAction, ev.SpanPID, ev.SpanName, ev.SpanValue = parsed.action, parsed.spanPID, parsed.name, parsed.value
		ev.SpanAction = intern.intern(ev.SpanAction)
		ev.SpanName = intern.intern(ev.SpanName)
		ev.SpanValue = intern.intern(ev.SpanValue)
		if parsed.track != "" || parsed.counterParsed {
			ev.PluginFields = &PluginFields{SpanTrack: intern.intern(parsed.track)}
		}
		if parsed.counterParsed {
			counter := parsed.counter
			ev.PluginFields.Counter = &TraceCounterFields{
				OwnerRaw: intern.intern(counter.ownerRaw), OwnerScope: intern.intern(counter.ownerScope),
				Metadata: intern.intern(counter.metadataRaw), OutputLevel: intern.intern(counter.outputLevel),
				TagBits: intern.intern(counter.tagBits), IssueReason: intern.intern(counter.issueReason),
				NumericValue: counter.numericValue, Parsed: true,
				NumericValid: counter.numericValid, IdentityValid: counter.identityOK,
			}
		}
	case EventBlockIssue, EventBlockComplete:
		dev, op, sector, length, identityValid := parseBlockRequestValidated(rawType, fields)
		bf := &BlockIOFields{
			Dev:            intern.intern(dev),
			Op:             intern.intern(op),
			Sector:         sector,
			Len:            length,
			IdentityParsed: true,
			IdentityValid:  identityValid,
		}
		if ev.Type == EventBlockComplete {
			bf.Error = intern.intern(parseBlockError(fields))
		}
		ev.BlockIOFields = bf
	case EventBlockRemap:
		remap := parseBlockRemapValidated(rawType, fields)
		ev.BlockIOFields = &BlockIOFields{
			Dev:              intern.intern(remap.Dev),
			Op:               intern.intern(remap.Op),
			Sector:           remap.Sector,
			Len:              remap.Len,
			SrcDev:           intern.intern(remap.SrcDev),
			SrcSector:        remap.SrcSector,
			RemapBios:        remap.NrBios,
			RemapBiosPresent: remap.NrBiosPresent,
			IdentityParsed:   true,
			IdentityValid:    remap.Valid,
		}
	case EventBinderTransaction:
		ev.BinderFields = s.binderTyped.binderFields(intern)
	case EventBinderReceived:
		ev.BinderFields = s.binderTyped.binderFields(intern)
	case EventBinderAllocBuf:
		ev.BinderFields = &BinderFields{
			TransactionID: strictBinderTransactionID(fields, true),
			DebugID:       atoi(kv["debug_id"]),
			DataSize:      atoi64(kv["data_size"]),
			OffsetsSize:   atoi64(kv["offsets_size"]),
			ExtraSize:     atoi64(firstNonEmpty(kv["extra_buffers_size"], kv["extra_size"])),
		}
	case EventBinderLock, EventBinderLocked, EventBinderUnlock, EventBinderReply:
		ev.BinderFields = &BinderFields{
			TransactionID: strictBinderTransactionID(fields, true),
			DebugID:       atoi(kv["debug_id"]),
			LockTag:       intern.intern(firstNonEmpty(kv["tag"], kv["lock"], kv["name"], fields)),
		}
	case EventIRQ, EventSoftIRQ:
		ev.IRQID = atoi(firstNonEmpty(kv["irq"], kv["vec"]))
		ev.IRQName = intern.intern(firstNonEmpty(kv["name"], strings.TrimSuffix(kv["action"], "]"), kv["vec"]))
	case EventIPI:
		ev.IRQName = intern.intern(parseIPIReason(fields))
		ev.IPITargetMask = intern.intern(firstNonEmpty(kv["target_mask"], kv["target_cpus"]))
		ev.IPITargetCPUs = parseIPITargetCPUs(ev.IPITargetMask)
	case EventMemory:
		ev.MemoryKind = intern.intern(classifyMemoryKind(rawType, fields))
		if ev.SubsystemKind == "" {
			ev.SubsystemKind = ev.MemoryKind
		}
		ev.ResourceFields = &ResourceFields{}
		ev.FileFields = &FileFields{}
		if payload, ok := parsePageCacheMutationPayload(rawType, fields); ok {
			// The complete source-pinned tuple is the only authority for this
			// mutation lane.  Bypass the generic key projector so page/pfn,
			// injected bytes/path aliases and malformed partial tuples cannot
			// leak into unrelated resource semantics.
			ev.FileFields.Dev = intern.intern(payload.dev)
			ev.FileFields.Ino = intern.intern(payload.inode)
			ev.FileFields.Offset = payload.offset
			ev.FileFields.pageCacheMutation = payload.kind
			break
		}
		if rawType == pageCacheAddEventName || rawType == pageCacheDeleteEventName {
			// Exact-but-malformed rows remain EventMemory inventory/search
			// records with empty typed projections and no mutation authority.
			break
		}
		if !populateResourceFields(&ev, kv, intern) {
			return Event{}, false
		}
		populateFileIOFields(&ev, kv, intern)
	case EventStorage, EventFilesystem:
		ev.ResourceFields = &ResourceFields{}
		ev.FileFields = &FileFields{}
		if admission, governed := mmcStorageWireAdmission(rawType, fields); governed {
			ev.ResourceFields.mmcPairing = &mmcPairingAdmission{
				identityKnown: admission.identityKnown, payloadAdmitted: admission.payloadAdmitted,
				device: intern.intern(admission.dev), opcode: intern.intern(admission.op),
			}
		}
		if admission, governed := f2fsStorageWireAdmission(rawType, fields); governed {
			ev.ResourceFields.f2fsPairing = &f2fsPairingAdmission{
				identityKnown: admission.identityKnown, payloadAdmitted: admission.payloadAdmitted,
				device: intern.intern(admission.dev), inode: intern.intern(admission.inode), operation: intern.intern(admission.op),
			}
		}
		if exactWritebackObservationName(rawType) {
			populateWritebackObservationFields(&ev, fields, intern)
			break
		}
		if !populateResourceFields(&ev, kv, intern) {
			return Event{}, false
		}
		populateFileIOFields(&ev, kv, intern)
	case EventAbilityMonitor, EventXPower, EventHiSystemEvent:
		ev.PluginFields = &PluginFields{}
		populatePluginFields(&ev, rawType, kv, intern)
		if isPrintFamilyRaw(rawType) {
			// §7.11 B-4: the converter HiSysEvent print shape carries
			// domain/ename positionally ("{domain}/{ename}: {contents}",
			// db2systrace.py:751-764), not as k=v. Inside this print-family
			// gate there are NO native plugin bytes to protect — anything the
			// generic kv pass scraped came from the {contents} tail (noise
			// like name=/bundle= inside the serialized payload), so the
			// positional head wins UNCONDITIONALLY when the comm-anchored
			// probe hits. Native (non-print) plugin raw types never enter
			// this branch and keep byte-identical fields.
			if domain, ename, ok := parseHiSysEventPrintPayload(ev.Comm, fields); ok {
				ev.PluginFields.Domain = intern.intern(domain)
				ev.PluginFields.EventName = intern.intern(ename)
			}
		}
	case EventPerfSample:
		ev.PerfFields = &PerfFields{}
		populatePerfSampleFields(&ev, kv, s.perfTextTyped, intern)
	}
	return ev, true
}

func populatePerfSampleFields(ev *Event, kv map[string]string, typed perfTextTypedFields, intern *stringInterner) {
	if ev == nil || ev.PerfFields == nil {
		return
	}
	pf := ev.PerfFields
	pf.PerfWeightInvalid = typed.WeightInvalid
	if typed.CPUInvalid {
		ev.CPU = -1
		ev.CPUInputInvalid = true
		pf.CPUKnown = boolPtr(false)
	} else if perfSampleCPUIsExplicitNoClaim(kv) {
		ev.CPU = -1
	} else if cpu, present, valid, _ := parseTraceCPUScalar(kv["cpu"]); present {
		if valid {
			ev.CPU = cpu
		} else {
			ev.CPU = -1
			ev.CPUInputInvalid = true
		}
	}
	pf.PID = atoi(kv["pid"])
	pf.TID = atoi(kv["tid"])
	if !typed.ThreadIdentityInvalid && typed.PIDPresent && typed.TIDPresent {
		// The converter-owned body is the perf identity authority, but a
		// contradictory positive ftrace envelope is a precise integrity
		// failure rather than a second valid identity for later OR matching.
		if ev.PID != pf.TID || ev.TGID > 0 && ev.TGID != pf.PID {
			typed.addIssue("thread_identity", "envelope_body_mismatch", false)
			typed.ThreadIdentityInvalid = true
		} else {
			ev.TGID = pf.PID
			ev.PID = pf.TID
		}
	}
	if !typed.PIDPresent && !typed.ThreadIdentityInvalid && pf.PID == 0 && ev.TGID > 0 {
		pf.PID = ev.TGID
	}
	if !typed.TIDPresent && !typed.ThreadIdentityInvalid && pf.TID == 0 && ev.PID > 0 {
		pf.TID = ev.PID
	}
	pf.Comm = intern.intern(perfTextValueBounded(firstNonEmpty(kv["thread_comm"], kv["comm"], kv["name"], ev.Comm), tracewire.MaxPerfMetadataBytes))
	pf.Period = atoi64(firstNonEmpty(kv["sample_weight"], kv["period_weight"], kv["period"], kv["sample_period"], kv["event_count"], kv["count"]))
	pf.EventName = intern.intern(perfTextValueBounded(firstNonEmpty(kv["event"], kv["type"]), tracewire.MaxPerfMetadataBytes))
	pf.Symbol = intern.intern(perfTextValueBounded(firstNonEmpty(kv["symbol"], kv["func"], kv["function"]), tracewire.MaxPerfMetadataBytes))
	pf.DSO = intern.intern(perfTextValueBounded(firstNonEmpty(kv["dso"], kv["file"], kv["path"]), tracewire.MaxPerfMetadataBytes))
	pf.IP = intern.intern(perfTextValueBounded(firstNonEmpty(kv["ip"], kv["addr"], kv["address"]), tracewire.MaxPerfMetadataBytes))
	pf.Addr = intern.intern(perfTextValueBounded(kv["addr"], tracewire.MaxPerfMetadataBytes))
	pf.SampleID = intern.intern(perfTextValueBounded(kv["sample_id"], tracewire.MaxPerfMetadataBytes))
	pf.StreamID = intern.intern(perfTextValueBounded(kv["stream_id"], tracewire.MaxPerfMetadataBytes))
	pf.RawWeight = perfOptionalInt64(kv, "perf_weight", &typed)
	pf.DataSrc = intern.intern(perfTextValueBounded(kv["data_src"], tracewire.MaxPerfMetadataBytes))
	pf.Transaction = intern.intern(perfTextValueBounded(kv["transaction"], tracewire.MaxPerfMetadataBytes))
	pf.PhysAddr = intern.intern(perfTextValueBounded(kv["phys_addr"], tracewire.MaxPerfMetadataBytes))
	pf.CGroupID = intern.intern(perfTextValueBounded(kv["cgroup_id"], tracewire.MaxPerfMetadataBytes))
	pf.DataPageSize = perfOptionalInt64(kv, "data_page_size", &typed)
	pf.CodePageSize = perfOptionalInt64(kv, "code_page_size", &typed)
	pf.RawSize = perfOptionalInt64(kv, "raw_size", &typed)
	pf.BranchCount = perfOptionalInt64(kv, "branch_count", &typed)
	pf.UserRegsABI = intern.intern(perfTextValueBounded(kv["user_regs_abi"], tracewire.MaxPerfMetadataBytes))
	pf.UserRegsCount = perfOptionalInt64(kv, "user_regs_count", &typed)
	pf.UserStackSize = perfOptionalInt64(kv, "user_stack_size", &typed)
	pf.AuxSize = perfOptionalInt64(kv, "aux_size", &typed)
	pf.Callchain = intern.intern(perfTextValueBounded(firstNonEmpty(kv["callchain"], kv["call_stack"], kv["stack"]), tracewire.MaxPerfCallchainBytes))
	pf.Source = intern.intern(perfTextValueBounded(firstNonEmpty(kv["source"], kv["producer"]), tracewire.MaxPerfMetadataBytes))
	pf.ParserCaveats = intern.intern(perfTextValueBounded(kv["parser_caveats"], tracewire.MaxPerfParserCaveatsBytes))
	if known, ok := perfWireBool(kv["thread_identity_known"]); ok {
		pf.ThreadIdentityKnown = boolPtr(known)
	}
	pf.Resolution = intern.intern(perfTextValueBounded(kv["resolution"], tracewire.MaxPerfMetadataBytes))
	if unverified, ok := perfWireBool(kv["lifecycle_unverified"]); ok {
		pf.LifecycleUnverified = boolPtr(unverified)
	}
	if typed.ThreadIdentityInvalid {
		pf.ThreadIdentityKnown = boolPtr(false)
		pf.LifecycleUnverified = boolPtr(true)
		if pf.Resolution == "" {
			pf.Resolution = intern.intern("perf_text_identity_degraded")
		}
	}
	pf.PerfTextIntegrity = intern.intern(typed.integritySummary())
	if sourcePID, ok := parseUnsignedTraceInt(kv["perf_source_pid"]); ok {
		pf.SourcePID = sourcePID
	}
	if sourceTID, ok := parseUnsignedTraceInt(kv["perf_source_tid"]); ok {
		pf.SourceTID = sourceTID
	}
	pf.SourceComm = intern.intern(perfTextValueBounded(kv["perf_source_comm"], tracewire.MaxPerfMetadataBytes))
	pf.SampleKind = intern.intern(perfTextValueBounded(firstNonEmpty(kv["sample_kind"], kv["sample_type"], kv["perf_sample_kind"]), tracewire.MaxPerfMetadataBytes))
	pf.SampleKindSource = intern.intern(perfTextValueBounded(kv["sample_kind_source"], tracewire.MaxPerfMetadataBytes))
	pf.SymbolizationStatus = intern.intern(perfTextValueBounded(firstNonEmpty(kv["symbolization_status"], kv["symbol_status"], kv["symbols"]), tracewire.MaxPerfMetadataBytes))
	pf.Clock = intern.intern(perfTextValueBounded(firstNonEmpty(kv["clock"], kv["clockid"]), tracewire.MaxPerfMetadataBytes))
	if !typed.CPUInvalid {
		if known, ok := perfWireBool(firstNonEmpty(kv["cpu_known"], kv["cpu_valid"], kv["cpu_available"])); ok {
			pf.CPUKnown = boolPtr(known)
		}
	}
	if typed.CPUInvalid {
		pf.CPUKnown = boolPtr(false)
	} else if pf.CPUKnown == nil {
		pf.CPUKnown = boolPtr(validTraceCPUIndex(ev.CPU))
	} else if *pf.CPUKnown && !validTraceCPUIndex(ev.CPU) {
		// An explicit truthy cpu_known flag cannot resurrect a malformed CPU
		// identity token into perf attribution.
		pf.CPUKnown = boolPtr(false)
	}
	if pf.SymbolizationStatus == "" {
		pf.SymbolizationStatus = intern.intern(defaultPerfSymbolizationStatus(pf))
	}
	pf.ClockConfidence = intern.intern(perfTextValueBounded(firstNonEmpty(kv["clock_confidence"], kv["time_alignment"], kv["time_alignment_confidence"]), tracewire.MaxPerfMetadataBytes))
	if pf.ClockConfidence == "" {
		pf.ClockConfidence = intern.intern(defaultPerfClockConfidence(pf))
	}
	pf.CallchainStatus = intern.intern(perfTextValueBounded(firstNonEmpty(kv["callchain_status"], kv["stack_status"], kv["call_stack_status"]), tracewire.MaxPerfMetadataBytes))
	if pf.CallchainStatus == "" {
		pf.CallchainStatus = intern.intern(defaultPerfCallchainStatus(pf))
	}
	normalizePerfSampleClaims(ev)
}

func perfTextValueBounded(raw string, maxLen int) string {
	// parsePerfTextKV already decoded and delimited this value. Re-running the
	// generic quote trimmer would corrupt legitimate leading/trailing quotes.
	return clampString(raw, maxLen)
}

func perfOptionalInt64(kv map[string]string, key string, typed *perfTextTypedFields) int64 {
	raw, present := kv[key]
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if !present || raw == "" {
		return 0
	}
	base, value := 10, raw
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		base, value = 16, value[2:]
	}
	parsed, err := strconv.ParseUint(value, base, 64)
	if err != nil || parsed > math.MaxInt64 {
		typed.addIssue(key, "out_of_range", false)
		return 0
	}
	return int64(parsed)
}

func defaultPerfSymbolizationStatus(pf *PerfFields) string {
	source := strings.ToLower(strings.TrimSpace(pf.Source))
	switch {
	case strings.Contains(source, "raw_perfdata"):
		return "unsymbolized"
	case pf.Symbol != "" && !perfLabelLooksLikeIP(pf.Symbol):
		return "symbolized"
	case pf.DSO != "" || pf.IP != "":
		return "partial"
	default:
		return "unknown"
	}
}

func defaultPerfClockConfidence(pf *PerfFields) string {
	if strings.TrimSpace(pf.Clock) == "" {
		return "unknown"
	}
	return "assumed"
}

func defaultPerfCallchainStatus(pf *PerfFields) string {
	callchain := strings.TrimSpace(pf.Callchain)
	source := strings.ToLower(strings.TrimSpace(pf.Source))
	switch {
	case callchain == "":
		return "missing"
	case strings.Contains(source, "raw_perfdata") || perfCallchainLooksIPOnly(callchain):
		return "ip_only"
	default:
		return "symbolized"
	}
}

func perfCallchainLooksIPOnly(callchain string) bool {
	parts := strings.FieldsFunc(callchain, func(r rune) bool {
		return r == ';' || r == ',' || r == '>' || r == '|'
	})
	seen := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		seen = true
		if !perfLabelLooksLikeIP(part) {
			return false
		}
	}
	return seen
}

func perfLabelLooksLikeIP(raw string) bool {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimPrefix(strings.ToLower(raw), "0x")
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func populateCPUConstraintFields(ev *Event, rawType string, kv map[string]string, intern *stringInterner) {
	if ev == nil || ev.ConstraintFields == nil {
		return
	}
	cf := ev.ConstraintFields
	cf.Kind = intern.intern(strings.TrimSpace(rawType))
	cf.Comm = intern.intern(cleanTraceValue(firstNonEmpty(kv["target_comm"], kv["comm"], kv["task"], kv["name"])))
	cf.PID = atoi(firstNonEmpty(kv["target_pid"], kv["task_pid"], kv["pid"], kv["tid"]))
	cf.Policy = intern.intern(firstNonEmpty(kv["policy"], kv["reason"], kv["type"]))
	cf.CPUSetName = intern.intern(cleanTraceValue(firstNonEmpty(kv["cpuset"], kv["cgroup"], kv["cg"], kv["path"])))
	if cf.CPUSetName != "" && ev.CGroup == "" {
		ev.CGroup = cf.CPUSetName
	}
	cpuRaw := firstNonEmpty(kv["cpu"], kv["target_cpu"])
	if cpu, present, valid, _ := parseTraceCPUScalar(cpuRaw); present {
		cf.CPUPresent = true
		if valid {
			cf.CPU = cpu
			cf.CPUValid = true
		} else {
			ev.CPUInputInvalid = true
		}
	}
	if cpu, present, valid, _ := parseTraceCPUScalar(kv["orig_cpu"]); present {
		cf.OrigCPUPresent = true
		if valid {
			cf.OrigCPU = cpu
			cf.OrigCPUSet = true
		} else {
			ev.CPUInputInvalid = true
		}
	}
	if cpu, present, valid, _ := parseTraceCPUScalar(kv["dest_cpu"]); present {
		cf.DestCPUPresent = true
		if valid {
			cf.DestCPU = cpu
			cf.DestCPUSet = true
			ev.TargetCPU = cpu
			ev.TargetCPUValid = true
		} else {
			ev.CPUInputInvalid = true
		}
	}
	allowedText := cleanTraceValue(firstNonEmpty(kv["allowed_cpus"], kv["cpus_allowed"], kv["cpumask"], kv["cpus"], kv["affinity"], kv["mask"]))
	cf.AllowedText = intern.intern(allowedText)
	if allowedText != "" {
		cf.AllowedPresent = true
		cf.Allowed, cf.AllowedValid, _ = parseCPUSetListStrict(allowedText)
		if !cf.AllowedValid {
			ev.CPUInputInvalid = true
		}
	}
}

func populateHarmonyNextInfoFields(ev *Event) {
	if ev == nil || strings.TrimSpace(ev.NextInfo) == "" {
		return
	}
	info, ok := parseHarmonyNextInfo(ev.NextInfo)
	if !ok {
		return
	}
	ev.NextInfoAffinity = info.affinity
	ev.NextInfoAllowedCPUs = info.allowedCPUs
	ev.NextInfoLoad = int32(info.load)
	ev.NextInfoGroup = int32(info.group)
	ev.NextInfoExpel = int32(info.expel)
	ev.NextInfoCGID = int32(info.cgid)
	// AUD-06 (§14.7, 2026-07-25): inline stamps — zero heap objects on the
	// hot sched_switch path (the former per-event *NextInfoRichFields
	// allocation is gone; extra/fieldCount stay derivable from ev.NextInfo).
	ev.NextInfoBoost = info.boost
	ev.NextInfoLoadKnown = info.loadKnown
	ev.NextInfoGroupKnown = info.groupKnown
	ev.NextInfoBoostKnown = info.boostKnown
	ev.NextInfoExpelKnown = info.expelKnown
	ev.NextInfoCGIDKnown = info.cgidKnown
}

type harmonyNextInfoFields struct {
	affinity    string
	allowedCPUs []int
	load        int
	loadKnown   bool
	group       int
	groupKnown  bool
	boost       bool
	boostKnown  bool
	expel       int
	expelKnown  bool
	cgid        int
	cgidKnown   bool
	extra       []string
	fieldCount  int
}

// nextInfoCheckedField — NEXTINFO P1: each field parses independently and a
// malformed token fails OPEN to known=false instead of silently collapsing
// into 0 (every 0 has closed-set meaning per the customer semantics doc).
func nextInfoCheckedField(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 7 {
		return 0, false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	return atoi(raw), true
}

func parseHarmonyNextInfo(raw string) (harmonyNextInfoFields, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) < 5 {
		return harmonyNextInfoFields{}, false
	}
	affinity := strings.TrimSpace(parts[0])
	if affinity == "" {
		return harmonyNextInfoFields{}, false
	}
	out := harmonyNextInfoFields{
		affinity:    affinity,
		allowedCPUs: parseCPUMaskHex(affinity),
		fieldCount:  len(parts),
	}
	out.load, out.loadKnown = nextInfoCheckedField(parts[1])
	out.group, out.groupKnown = nextInfoCheckedField(parts[2])
	// Field 3 = ices_boost (customer doc). AUD-05(3) (§14.6, 2026-07-25)
	// semantic-known split: the doc's boost closed set is {0,1} — an
	// out-of-range value (e.g. 2) WITHDRAWS the semantic ices_boost claim,
	// so the faces never assert 前台加速 on an undocumented value.
	// NEXTINFO-V1 (2026-07-26): the legacy bug-compat restricted fill
	// (lexically-valid non-zero → "restricted") retired with its consumers.
	boostRaw, boostLexical := nextInfoCheckedField(parts[3])
	out.boostKnown = boostLexical && boostRaw <= 1
	out.boost = out.boostKnown && boostRaw == 1
	out.expel, out.expelKnown = nextInfoCheckedField(parts[4])
	if len(parts) >= 6 {
		out.cgid, out.cgidKnown = nextInfoCheckedField(parts[5])
	}
	if len(parts) > 6 {
		for _, part := range parts[6:] {
			out.extra = append(out.extra, strings.TrimSpace(part))
		}
	}
	return out, true
}

func parseCPUSetList(raw string) []int {
	cpus, valid, _ := parseCPUSetListStrict(raw)
	if !valid {
		return nil
	}
	return cpus
}

func parseCPUMaskHex(raw string) []int {
	raw = strings.TrimSpace(strings.Trim(raw, "{}[](),"))
	raw = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(raw), "0x"), "0X")
	if raw == "" {
		return nil
	}
	mask, err := strconv.ParseUint(raw, 16, 64)
	if err != nil || mask == 0 {
		return nil
	}
	var cpus []int
	for cpu := 0; cpu < 64; cpu++ {
		if mask&(uint64(1)<<cpu) != 0 {
			cpus = append(cpus, cpu)
		}
	}
	return cpus
}

func containsHexAlpha(raw string) bool {
	for _, r := range raw {
		if r >= 'a' && r <= 'f' {
			return true
		}
	}
	return false
}

func uniqueSortedInts(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, v := range in {
		if v < 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func populateResourceFields(ev *Event, kv map[string]string, intern *stringInterner) bool {
	if ev == nil || ev.ResourceFields == nil {
		return true
	}
	rf := ev.ResourceFields
	rf.Path = intern.intern(cleanTraceValue(firstNonEmpty(kv["path"], kv["file"], kv["filename"], kv["entry_name"], kv["name"])))
	rf.Op = intern.intern(firstNonEmpty(kv["op"], kv["operation"], kv["syscall"], kv["type"], kv["rw"], kv["rwbs"]))
	latencyMs, latencyOK := parseLatencyMsChecked(kv)
	if !latencyOK {
		// A resource row that explicitly claims a malformed/non-finite
		// latency is not safe count or duration evidence. Reject the row at
		// admission instead of keeping a zero-latency shell that can still
		// acquire an advisory rank through event-count fallbacks.
		return false
	}
	rf.LatencyMs = latencyMs
	rf.Bytes = atoi64(firstNonEmpty(kv["bytes"], kv["size"], kv["len"], kv["length"]))
	rf.Address = intern.intern(firstNonEmpty(kv["addr"], kv["address"], kv["fault_addr"]))
	rf.Callstack = intern.intern(clampString(firstNonEmpty(kv["callstack"], kv["backtrace"], kv["stack"]), 160))
	return true
}

func populateFileIOFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil || ev.FileFields == nil {
		return
	}
	ff := ev.FileFields
	ff.Dev = intern.intern(firstNonEmpty(kv["fs_dev"], kv["dev"]))
	ff.Ino = intern.intern(cleanTraceValue(firstNonEmpty(kv["ino"], kv["inode"])))
	ff.ParentIno = intern.intern(cleanTraceValue(firstNonEmpty(kv["pino"], kv["parent_ino"], kv["parent_inode"], kv["parent"])))
	ff.Entry = intern.intern(cleanTraceValue(firstNonEmpty(kv["entry_name"], kv["path"], kv["file"], kv["filename"], kv["name"])))
	ff.Offset = atoi64Auto(firstNonEmpty(kv["offset"], kv["ofs"], kv["pos"]))
	ff.Len = atoi64Auto(firstNonEmpty(kv["bytes"], kv["len"], kv["length"]))
	ff.RW = intern.intern(normalizeFileRW(firstNonEmpty(kv["rw"], kv["rwbs"], kv["op"], kv["operation"], kv["type"], fileOperationFromEventName(ev.Name))))
	ff.Ret = atoi64Auto(kv["ret"])
	ff.Size = atoi64Auto(firstNonEmpty(kv["i_size"], kv["file_size"]))
	if rf := ev.ResourceFields; rf != nil {
		if rf.Path == "" && ff.Entry != "" {
			rf.Path = ff.Entry
		}
		if rf.Op == "" && ff.RW != "" {
			rf.Op = ff.RW
		}
		if rf.Bytes == 0 && ff.Len > 0 {
			rf.Bytes = ff.Len
		}
	}
}

func populatePluginFields(ev *Event, rawType string, kv map[string]string, intern *stringInterner) {
	if ev == nil || ev.PluginFields == nil {
		return
	}
	pl := ev.PluginFields
	pl.Domain = intern.intern(firstNonEmpty(kv["domain"], kv["module"], kv["bundle"], kv["process"], kv["package"]))
	pl.EventName = intern.intern(firstNonEmpty(kv["event_name"], kv["eventname"], kv["event"], kv["name"], rawType))
	pl.Metric = intern.intern(firstNonEmpty(kv["metric"], kv["key"], kv["item"], kv["counter"], kv["component"], kv["type"]))
	pl.Value = intern.intern(firstNonEmpty(kv["value"], kv["val"], kv["state"], kv["usage"], kv["energy"], kv["count"], kv["duration_ms"], kv["latency_ms"]))
	pl.Category = intern.intern(firstNonEmpty(kv["category"], kv["level"], kv["tag"], kv["scene"]))
}

type parsedBlockRemap struct {
	Dev, Op, SrcDev string
	Sector, Len     int64
	SrcSector       int64
	NrBios          int64
	NrBiosPresent   bool
	Valid           bool
}

func parseBlockRemapValidated(rawType, fields string) parsedBlockRemap {
	trimmed := strings.TrimSpace(fields)
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "block_rq_remap":
		if m := blockRQRemapRE.FindStringSubmatch(trimmed); len(m) == 8 {
			return buildParsedBlockRemap(m[1], m[2], m[3], m[4], m[5], m[6], m[7])
		}
	case "block_bio_remap":
		if m := blockBioRemapRE.FindStringSubmatch(trimmed); len(m) == 7 {
			return buildParsedBlockRemap(m[1], m[2], m[3], m[4], m[5], m[6], "")
		}
		if m := blockRemapLegacyRE.FindStringSubmatch(trimmed); len(m) == 6 {
			return buildParsedBlockRemap(m[1], "", m[2], m[3], m[4], m[5], "")
		}
	}
	return parsedBlockRemap{}
}

func buildParsedBlockRemap(devRaw, op, sectorRaw, lenRaw, srcDevRaw, srcSectorRaw, nrBiosRaw string) parsedBlockRemap {
	dev, devOK := canonicalBlockDevice(devRaw)
	srcDev, srcDevOK := canonicalBlockDevice(srcDevRaw)
	sector, sectorErr := strconv.ParseInt(sectorRaw, 10, 64)
	length, lengthErr := strconv.ParseInt(lenRaw, 10, 64)
	srcSector, srcSectorErr := strconv.ParseInt(srcSectorRaw, 10, 64)
	parsed := parsedBlockRemap{
		Dev: dev, Op: strings.TrimSpace(op), SrcDev: srcDev,
		Sector: sector, Len: length, SrcSector: srcSector,
	}
	if nrBiosRaw != "" {
		parsed.NrBiosPresent = true
		parsed.NrBios, _ = strconv.ParseInt(nrBiosRaw, 10, 64)
	}
	parsed.Valid = devOK && srcDevOK && sectorErr == nil && lengthErr == nil && srcSectorErr == nil &&
		sector >= 0 && length >= 0 && length <= maxBlockSectorCount && srcSector >= 0 &&
		(parsed.Op == "" || validBlockOperationToken(parsed.Op))
	if parsed.NrBiosPresent {
		_, nrErr := strconv.ParseUint(nrBiosRaw, 10, 32)
		parsed.Valid = parsed.Valid && nrErr == nil
	}
	if !parsed.Valid {
		// IdentityValid is deliberately not part of the public JSON surface.
		// Therefore an invalid row must not publish partial typed values or a
		// presence bit that could be mistaken for an exact encoded zero.
		return parsedBlockRemap{}
	}
	return parsed
}

func parseBlockError(fields string) string {
	m := blockErrorRE.FindStringSubmatch(strings.TrimSpace(fields))
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func classifyMemoryKind(raw, fields string) string {
	text := strings.ToLower(strings.TrimSpace(raw + " " + fields))
	switch {
	case strings.Contains(text, "reclaim") || strings.Contains(text, "vmscan"):
		return "reclaim"
	case strings.Contains(text, "fault"):
		return "page_fault"
	case strings.Contains(text, "filemap") || strings.Contains(text, "page_cache"):
		return "page_cache"
	case strings.Contains(text, "gc"):
		return "gc"
	default:
		return "memory"
	}
}

func classifyEventType(comm, raw, fields string) EventType {
	raw = strings.TrimSpace(raw)
	rawLower := strings.ToLower(raw)
	pairingProfile, exactPairingEndpoint := pairingEndpointProfileForName(raw)
	switch {
	case EROFSCoverageOnlyNameCandidate(raw):
		// EROFS compatibility names have no producer-matched binary descriptor
		// authority yet. Keep the physical row searchable as EventUnknown; do
		// not infer filesystem semantics from the prefix or payload shape.
		return EventUnknown
	case raw == "sched_switch":
		return EventSchedSwitch
	case raw == "sched_wakeup" || raw == "sched_wakeup_new":
		return EventSchedWakeup
	case raw == "sched_waking":
		return EventSchedWaking
	case raw == "sched_blocked_reason":
		return EventSchedBlockedReason
	case raw == "sched_pi_setprio" || raw == "binder_set_priority":
		// Exact priority-mutation tracepoints are range poison until their
		// producer-specific old/new priority domain is mechanically proven.
		return EventPriorityMutation
	case strings.HasPrefix(raw, "sched_stat_"):
		return EventSchedStat
	case raw == "task_rename":
		// Standard ftrace registration metadata. It remains context-only: the
		// physical header TID/lifecycle is the identity authority and rename
		// text never enters scheduler or causal hard gates.
		return EventTaskRename
	case raw == "rss_stat":
		// OpenHarmony's exact rss_stat row is inventory context only. In
		// particular, do not fold it into EventMemory: no typed resource or
		// memory-pressure authority is established by this row.
		return EventRSSStat
	case raw == "phase_task_delta":
		// OpenHarmony per-task accounting inventory. The payload is not a
		// sched_switch/running interval and therefore grants no scheduler or
		// compute-supply authority.
		return EventPhaseTaskDelta
	case raw == "perf_sample":
		return EventPerfSample
	case raw == "cpu_idle":
		return EventCPUIdle
	case raw == "cpu_frequency":
		return EventCPUFrequency
	case raw == "cpu_frequency_limits":
		return EventCPUFrequencyLimit
	case isCPUConstraintEvent(rawLower):
		return EventCPUConstraint
	case raw == "clock_set_rate":
		if isCPUFrequencyClock(fields) {
			return EventCPUFrequency
		}
		return EventClockSetRate
	case strings.Contains(rawLower, "cpu") && strings.Contains(rawLower, "freq") && strings.Contains(rawLower, "limit"):
		return EventCPUFrequencyLimit
	case strings.Contains(rawLower, "cpu") && strings.Contains(rawLower, "freq"):
		return EventCPUFrequency
	case exactPairingEndpoint && pairingProfile.Family == PairingEndpointBlock && pairingProfile.Phase == PairingEndpointStart:
		return EventBlockIssue
	case raw == "block_rq_insert" || raw == "block_getrq":
		return EventBlockIssue
	case raw == "block_bio_remap" || raw == "block_rq_remap":
		return EventBlockRemap
	case exactPairingEndpoint && pairingProfile.Family == PairingEndpointBlock && pairingProfile.Phase == PairingEndpointDone:
		return EventBlockComplete
	case exactPairingEndpoint && pairingProfile.Family == PairingEndpointBinder && pairingProfile.Phase == PairingEndpointStart:
		return EventBinderTransaction
	case exactPairingEndpoint && pairingProfile.Family == PairingEndpointBinder && pairingProfile.Phase == PairingEndpointDone:
		return EventBinderReceived
	case raw == "binder_transaction_alloc_buf" || raw == "binder_alloc_buf":
		return EventBinderAllocBuf
	case raw == "binder_transaction_lock" || raw == "binder_lock":
		return EventBinderLock
	case raw == "binder_transaction_locked" || raw == "binder_locked":
		return EventBinderLocked
	case raw == "binder_transaction_unlock" || raw == "binder_unlock":
		return EventBinderUnlock
	case raw == "binder_transaction_reply" || raw == "binder_reply":
		return EventBinderReply
	case strings.Contains(rawLower, "softirq"):
		return EventSoftIRQ
	case raw == "ipi_entry" || raw == "ipi_exit" || raw == "ipi_raise":
		return EventIPI
	case strings.HasPrefix(raw, "irq_"):
		return EventIRQ
	case isPrintFamilyRaw(raw):
		if isTraceMarkPayload(fields) {
			return EventTraceMark
		}
		// §7.11 B-4: non-mark print payloads chain through the existing
		// plugin detectors (the same trio consumed for native plugin raw
		// types below) first. The converter-only HiLog context inventory is
		// the final precise fallback, so it cannot shadow an existing plugin
		// observation. Ordinary plain print prose stays EventUnknown.
		if typ, ok := classifyPrintPluginPayload(comm, rawLower, fields); ok {
			return typ
		}
		if isConverterHiLogPrintPayload(comm, fields) {
			return EventHiLog
		}
		return EventUnknown
	case isStorageEvent(rawLower):
		return EventStorage
	case isFilesystemEvent(raw):
		return EventFilesystem
	case isPowerEvent(rawLower):
		return EventPower
	case isAbilityEvent(rawLower, fields):
		return EventAbilityMonitor
	case isXPowerEvent(rawLower, fields):
		return EventXPower
	case isHiSystemEvent(rawLower, fields):
		return EventHiSystemEvent
	case strings.HasPrefix(rawLower, "workqueue_"):
		return EventWorkqueue
	case strings.HasPrefix(rawLower, "dma_fence"):
		return EventDMAFence
	case strings.HasPrefix(rawLower, "mm_") || strings.Contains(rawLower, "reclaim") || strings.Contains(rawLower, "fault") || strings.Contains(fields, "GC"):
		return EventMemory
	default:
		return EventUnknown
	}
}

func isCPUConstraintEvent(raw string) bool {
	switch raw {
	case "sched_setaffinity", "sched_migrate_task", "cpuset_attach", "cgroup_attach_task":
		return true
	default:
		return false
	}
}

func classifySubsystemKind(raw, fields string, typ EventType) string {
	if EROFSCoverageOnlyNameCandidate(raw) {
		return ""
	}
	text := strings.ToLower(strings.TrimSpace(raw + " " + fields))
	switch typ {
	case EventCPUFrequencyLimit:
		return "cpu_frequency_limits"
	case EventSoftIRQ:
		return "softirq"
	case EventStorage:
		switch {
		case strings.Contains(text, "bio") && strings.Contains(text, "latency"):
			return "ebpf_bio"
		case strings.Contains(text, "ufshcd"):
			return "storage_ufs"
		case strings.Contains(text, "mmc"):
			return "storage_mmc"
		case strings.Contains(text, "scsi"):
			return "storage_scsi"
		case strings.Contains(text, "i2c"):
			return "storage_i2c"
		case strings.Contains(text, "smbus"):
			return "storage_smbus"
		default:
			return "storage"
		}
	case EventFilesystem:
		switch {
		case strings.Contains(text, "file_system") || strings.Contains(text, "filesystem") || strings.Contains(text, "ebpf_file"):
			return "ebpf_filesystem"
		case strings.Contains(text, "android_fs"):
			return "fs_android"
		case strings.Contains(text, "f2fs"):
			return "fs_f2fs"
		case strings.Contains(text, "hmfs"):
			// §28.6 ④: HarmonyOS hmfs (f2fs-derivative) keeps its own honest
			// subsystem word — never silently relabelled fs_f2fs.
			return "fs_hmfs"
		case strings.Contains(text, "ext4"):
			return "fs_ext4"
		case strings.Contains(text, "writeback") || strings.Contains(text, "wb_err"):
			return "writeback"
		case strings.Contains(text, "filemap") || strings.Contains(text, "page_cache"):
			return "page_cache"
		default:
			return "filesystem"
		}
	case EventPower:
		switch {
		case strings.Contains(text, "thermal"):
			return "thermal"
		case strings.Contains(text, "regulator"):
			return "regulator"
		default:
			return "power"
		}
	case EventAbilityMonitor:
		return "ability_monitor"
	case EventXPower:
		return "xpower"
	case EventHiSystemEvent:
		return "hi_sysevent"
	case EventWorkqueue:
		return "workqueue"
	case EventDMAFence:
		return "dma_fence"
	case EventMemory:
		return classifyMemoryKind(raw, fields)
	default:
		return ""
	}
}

func isStorageEvent(raw string) bool {
	return strings.HasPrefix(raw, "ufshcd_") ||
		strings.HasPrefix(raw, "mmc_") ||
		strings.HasPrefix(raw, "scsi_") ||
		strings.HasPrefix(raw, "i2c_") ||
		strings.HasPrefix(raw, "smbus_") ||
		(strings.Contains(raw, "bio") && strings.Contains(raw, "latency")) ||
		strings.HasPrefix(raw, "bio_") ||
		strings.HasPrefix(raw, "ebpf_bio")
}

func isFilesystemEvent(raw string) bool {
	// writeback admission is byte-exact.  Lower-casing belongs only to the
	// established broad filesystem families below; applying it to these two
	// names would turn case drift into typed filesystem authority.
	if exactWritebackObservationName(raw) {
		return true
	}
	if EROFSCoverageOnlyNameCandidate(raw) {
		return false
	}
	raw = strings.ToLower(raw)
	return strings.HasPrefix(raw, "ext4_") ||
		strings.HasPrefix(raw, "f2fs_") ||
		// hmfs_ (§28.6 ④, 2026-07-09): the HarmonyOS FS layer (an f2fs
		// derivative) emits f2fs-isomorphic tracepoints under the hmfs_
		// prefix; without this arm the customer platform's FS events were
		// wholesale unclassified (the 东湖 inode story leaked in only via
		// mm_filemap). The kv field shapes reuse populateFileIOFields
		// unchanged (generic key-driven parse, same keys as f2fs_*).
		strings.HasPrefix(raw, "hmfs_") ||
		strings.HasPrefix(raw, "android_fs_") ||
		strings.HasPrefix(raw, "filesystem") ||
		strings.HasPrefix(raw, "file_system") ||
		strings.HasPrefix(raw, "ebpf_file")
}

func isPowerEvent(raw string) bool {
	return strings.HasPrefix(raw, "thermal_") || strings.HasPrefix(raw, "regulator_")
}

func isAbilityEvent(raw, fields string) bool {
	text := strings.ToLower(raw + " " + fields)
	return strings.Contains(text, "ability_monitor") ||
		strings.HasPrefix(raw, "ability_") ||
		strings.Contains(text, "abilitymanager") ||
		strings.Contains(text, "ability_manager")
}

func isXPowerEvent(raw, fields string) bool {
	text := strings.ToLower(raw + " " + fields)
	return strings.Contains(text, "xpower")
}

func isHiSystemEvent(raw, fields string) bool {
	text := strings.ToLower(raw + " " + fields)
	return strings.Contains(text, "hisysevent") ||
		strings.Contains(text, "hi_sysevent") ||
		strings.Contains(text, "hi_sys_event")
}

// isPrintFamilyRaw reports whether the raw ftrace event name is the
// print/tracing_mark_write family that carries userspace payloads.
func isPrintFamilyRaw(raw string) bool {
	switch raw {
	case "print", "tracing_mark_write", "tracing_mark_write_xacct", "xacct_tracing_mark_write":
		return true
	default:
		return false
	}
}

// classifyPrintPluginPayload types a print-family row whose payload is NOT a
// B|/E|/C|/S|/F|/G|/H|/N|/I| trace mark (§7.11 B-4). Converters such as hmtrace
// db2systrace.py re-emit HiLog rows as "print: [{level}][{tag}] {msg}"
// (db2systrace.py:626-634) and HiSysEvent rows as
// "print: {domain}/{ename}: {contents}" (db2systrace.py:751-764); before this
// hook they early-exited as EventUnknown, keeping their index slot and
// event_search hits but losing the plugin type. The chain reuses the EXACT
// detectors already applied to native plugin raw types (isAbilityEvent /
// isXPowerEvent / isHiSystemEvent, same relative order as the main switch)
// plus one comm-anchored structural probe for the converter HiSysEvent shape
// (comm must be the converter machine token, see isHiSysEventPrintPayload).
// Typing feeds observation surfaces only (stats counters, plugin summaries,
// subsystem labels, event-type/pattern query matching) — no hard gate reads
// these types, and a payload with no detector hit stays EventUnknown.
func classifyPrintPluginPayload(comm, rawLower, fields string) (EventType, bool) {
	switch {
	case isHiSysEventPrintPayload(comm, fields):
		return EventHiSystemEvent, true
	case isAbilityEvent(rawLower, fields):
		return EventAbilityMonitor, true
	case isXPowerEvent(rawLower, fields):
		return EventXPower, true
	case isHiSystemEvent(rawLower, fields):
		return EventHiSystemEvent, true
	default:
		return EventUnknown, false
	}
}

// converterHiSysEventComm is the machine comm token converters pin on every
// re-emitted HiSysEvent print row: codrax's own trace-db exporter writes it
// verbatim (hitraceconv/streamerdb_export_extended.go:940,
// `addTraceDBInstantRow(sink, ts, "<hisysevent>", …)`), and hmtrace
// db2systrace.py:751-764 emits the identical comm. Real threads never carry
// this angle-bracketed synthetic task name.
const converterHiSysEventComm = "<hisysevent>"

// converterHiLogComm is the exact synthetic comm used by the SQL writer for
// re-emitted log rows. It opens only a context inventory type: no scheduler,
// plugin or causal consumer treats EventHiLog as hard evidence. Ordinary
// userspace print rows remain EventUnknown.
const converterHiLogComm = "<hilog>"

func isConverterHiLogPrintPayload(comm, fields string) bool {
	if comm != converterHiLogComm || len(fields) < 6 || fields[0] != '[' {
		return false
	}
	levelEnd := strings.IndexByte(fields, ']')
	if levelEnd <= 1 || levelEnd+2 >= len(fields) || fields[levelEnd+1] != '[' {
		return false
	}
	tagTail := fields[levelEnd+2:]
	tagEnd := strings.IndexByte(tagTail, ']')
	if tagEnd <= 0 {
		return false
	}
	remainder := tagTail[tagEnd+1:]
	return remainder == "" || remainder[0] == ' '
}

// hisysEventPrintRE matches the converter-emitted HiSysEvent print payload
// "{domain}/{ename}: {contents}" (db2systrace.py:751-764;
// hitraceconv/streamerdb_export_extended.go:939): DOMAIN and ENAME are
// HiSysEvent uppercase identifiers of length ≥2, exactly one '/', then ':'
// followed by end-of-payload or a space. The ≥2 floor kills short prose
// heads ("I/O: read done", "A/B: on") that would otherwise fit the shape.
var hisysEventPrintRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]+/[A-Z][A-Z0-9_]+:( |$)`)

// isHiSysEventPrintPayload reports whether a print-family payload is the
// converter HiSysEvent re-emission. The payload shape alone is NOT precise
// enough to type on — real userspace print prose uses the same head
// ("UI/UX: jank", "GC/HEAP: freed", "NET/DNS: resolve") — so the PRIMARY
// criterion is the comm anchor: converters pin comm to the machine token
// converterHiSysEventComm on these rows and nothing else does. Any other
// comm falls through to the Contains-detector chain and then EventUnknown —
// the pre-B-4 behavior (fail-open).
func isHiSysEventPrintPayload(comm, fields string) bool {
	return comm == converterHiSysEventComm && hisysEventPrintRE.MatchString(fields)
}

// parseHiSysEventPrintPayload extracts the positional domain/ename pair from
// the converter HiSysEvent print shape (db2systrace.py:751-764). ok is false
// for anything that does not match the exact machine shape, including any
// row whose comm is not the converter machine token.
func parseHiSysEventPrintPayload(comm, fields string) (domain, ename string, ok bool) {
	if !isHiSysEventPrintPayload(comm, fields) {
		return "", "", false
	}
	head, _, _ := strings.Cut(fields, ":")
	domain, ename, _ = strings.Cut(head, "/")
	return domain, ename, true
}

// parseSchedSwitchKV parses the scheduler core in its producer-defined order.
// Generic key scanning is intentionally not a fallback: prev_comm/next_comm are
// unquoted strings and may legally contain spaces plus text such as
// "next_info=..." or "cg=...". Each REQUIRED core key must have exactly one
// ASCII-boundary declaration across the whole row; this prevents a valid-
// looking key sequence in a comm or suffix from hiding the producer's malformed
// real core. The unique declarations must then form the exact ordered grammar.
func parseSchedSwitchKV(fields string) (map[string]string, string) {
	fields = trimASCIIHorizontalSpace(fields)
	const prevCommPrefix = "prev_comm="
	const (
		corePrevComm = iota
		corePrevPID
		corePrevPrio
		corePrevState
		coreNextComm
		coreNextPID
		coreNextPrio
	)
	coreKeys := [...]string{"prev_comm", "prev_pid", "prev_prio", "prev_state", "next_comm", "next_pid", "next_prio"}
	var positions [len(coreKeys)]int
	for i, key := range coreKeys {
		position, count := asciiKeyDeclarationPosition(fields, key, 0)
		if count == 0 {
			return nil, key + "_missing"
		}
		if count > 1 {
			return nil, key + "_ambiguous"
		}
		positions[i] = position
	}
	if positions[corePrevComm] != 0 || !strings.HasPrefix(fields, prevCommPrefix) {
		return nil, "prev_comm_misordered"
	}
	if positions[corePrevPID] < len(prevCommPrefix) {
		return nil, "prev_pid_misordered"
	}

	prevComm := trimASCIIHorizontalSpace(fields[len(prevCommPrefix):positions[corePrevPID]])
	prevPIDRaw, next, ok := consumeSchedToken(fields, positions[corePrevPID], "prev_pid=")
	prevPID, valid := parseCanonicalSchedPID(prevPIDRaw)
	if !ok || !valid {
		return nil, "prev_pid_invalid"
	}
	if !skipRequiredASCIIHorizontalSpace(fields, &next) || next != positions[corePrevPrio] {
		return nil, "prev_prio_misordered"
	}
	prevPrioRaw, next, ok := consumeSchedToken(fields, positions[corePrevPrio], "prev_prio=")
	prevPrio, valid := parseCanonicalSchedPriority(prevPrioRaw)
	if !ok || !valid {
		return nil, "prev_prio_invalid"
	}
	if !skipRequiredASCIIHorizontalSpace(fields, &next) || next != positions[corePrevState] {
		return nil, "prev_state_misordered"
	}
	prevState, next, ok := consumeSchedToken(fields, positions[corePrevState], "prev_state=")
	if !ok || prevState == "" {
		return nil, "prev_state_invalid"
	}
	if !skipRequiredASCIIHorizontalSpace(fields, &next) || !strings.HasPrefix(fields[next:], "==>") {
		return nil, "switch_delimiter_missing_or_misordered"
	}
	next += len("==>")
	if next < len(fields) && !isASCIIHorizontalSpace(fields[next]) {
		return nil, "switch_delimiter_invalid"
	}
	if !skipRequiredASCIIHorizontalSpace(fields, &next) || next != positions[coreNextComm] {
		return nil, "next_comm_misordered"
	}
	nextCommFrom := positions[coreNextComm] + len("next_comm=")
	if positions[coreNextPID] < nextCommFrom {
		return nil, "next_pid_misordered"
	}
	nextComm := trimASCIIHorizontalSpace(fields[nextCommFrom:positions[coreNextPID]])

	nextPIDRaw, next, ok := consumeSchedToken(fields, positions[coreNextPID], "next_pid=")
	nextPID, valid := parseCanonicalSchedPID(nextPIDRaw)
	if !ok || !valid {
		return nil, "next_pid_invalid"
	}
	if !skipRequiredASCIIHorizontalSpace(fields, &next) || next != positions[coreNextPrio] {
		return nil, "next_prio_misordered"
	}
	nextPrioRaw, suffixFrom, ok := consumeSchedToken(fields, positions[coreNextPrio], "next_prio=")
	nextPrio, valid := parseCanonicalSchedPriority(nextPrioRaw)
	if !ok || !valid {
		return nil, "next_prio_invalid"
	}
	out := map[string]string{
		"prev_comm":  prevComm,
		"prev_pid":   strconv.Itoa(prevPID),
		"prev_prio":  strconv.Itoa(prevPrio),
		"prev_state": prevState,
		"next_comm":  nextComm,
		"next_pid":   strconv.Itoa(nextPID),
		"next_prio":  strconv.Itoa(nextPrio),
	}
	suffixKV, optionalOK, fatalReason := parseSchedSwitchSuffixKV(fields[suffixFrom:])
	if fatalReason != "" {
		return nil, fatalReason
	}
	if optionalOK {
		for key, value := range suffixKV {
			out[key] = value
		}
	}
	return out, ""
}

// asciiKeyDeclarationPosition counts both the producer's canonical "key=" and
// malformed legacy-compatible "key <hspace>=" spellings. Only the canonical
// spelling can pass consumeSchedToken later, but every declaration participates
// in the uniqueness census so a spaced malformed sibling cannot be hidden in a
// comm, quoted unknown token, or suffix and leave a false core authoritative.
func asciiKeyDeclarationPosition(s, key string, from int) (first, count int) {
	first = -1
	if from < 0 {
		from = 0
	}
	for cursor := from; cursor <= len(s)-len(key); {
		rel := strings.Index(s[cursor:], key)
		if rel < 0 {
			break
		}
		start := cursor + rel
		after := start + len(key)
		for after < len(s) && isASCIIHorizontalSpace(s[after]) {
			after++
		}
		if (start == 0 || isASCIIHorizontalSpace(s[start-1])) && after < len(s) && s[after] == '=' {
			if count == 0 {
				first = start
			}
			count++
			if count > 1 {
				return first, count
			}
		}
		cursor = start + 1
	}
	return first, count
}

func consumeSchedToken(s string, from int, prefix string) (string, int, bool) {
	if from < 0 || from > len(s) || !strings.HasPrefix(s[from:], prefix) {
		return "", from, false
	}
	valueFrom := from + len(prefix)
	end := valueFrom
	for end < len(s) && !isASCIIHorizontalSpace(s[end]) {
		end++
	}
	if end == valueFrom {
		return "", from, false
	}
	return s[valueFrom:end], end, true
}

func skipRequiredASCIIHorizontalSpace(s string, pos *int) bool {
	if pos == nil || *pos < 0 || *pos >= len(s) || !isASCIIHorizontalSpace(s[*pos]) {
		return false
	}
	for *pos < len(s) && isASCIIHorizontalSpace(s[*pos]) {
		(*pos)++
	}
	return true
}

func isASCIIHorizontalSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func trimASCIIHorizontalSpace(s string) string {
	return strings.Trim(s, " \t")
}

func parseCanonicalSchedPID(raw string) (int, bool) {
	if raw == "" || !isAllDigits(raw) {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 31)
	if err != nil || raw != strconv.FormatUint(value, 10) {
		return 0, false
	}
	return int(value), true
}

func parseCanonicalSchedPriority(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || raw != strconv.FormatInt(value, 10) {
		return 0, false
	}
	return int(value), true
}

// parseSchedSwitchSuffixKV keeps next_info and the cg/cgroup alias family as
// independent optional dimensions. It has a dedicated quote-aware lexer: the
// generic kvRE is context-free and can reopen key-looking text from an
// unterminated quote. A syntax-bad suffix contributes no optional metadata,
// while the independently validated scheduler core remains usable. Duplicate
// authorities omit only their own family, even when their values agree.
func parseSchedSwitchSuffixKV(suffix string) (map[string]string, bool, string) {
	tokens, ok := tokenizeSchedSwitchSuffix(suffix)
	if !ok {
		return nil, false, ""
	}
	var nextInfo, cgroup string
	nextInfoCount, cgroupCount := 0, 0
	for _, token := range tokens {
		key, rawValue, found := strings.Cut(token, "=")
		if !found || !isTraceKVKey(key) {
			return nil, false, ""
		}
		value := unwrapSchedSuffixValue(rawValue)
		switch key {
		case "next_info":
			nextInfoCount++
			if value != "" {
				nextInfo = value
			}
		case "cg", "cgroup":
			cgroupCount++
			if value != "" {
				cgroup = value
			}
		case "prev_comm", "prev_pid", "prev_prio", "prev_state", "next_comm", "next_pid", "next_prio":
			// Core-shaped declarations are not optional suffix fields. If one
			// appears here, suppress every optional value rather than allowing a
			// forged next_info/cgroup beside it to acquire typed authority.
			return nil, false, "core_key_in_suffix"
		default:
			if value == "" {
				return nil, false, ""
			}
		}
	}
	out := make(map[string]string, 2)
	if nextInfoCount == 1 && nextInfo != "" {
		out["next_info"] = nextInfo
	}
	if cgroupCount == 1 && cgroup != "" {
		// Normalize both producer spellings to the pre-existing cg-preferred
		// consumer key; Event.CGroup remains the sole public typed field.
		out["cg"] = cgroup
	}
	return out, true, ""
}

func tokenizeSchedSwitchSuffix(s string) ([]string, bool) {
	s = trimASCIIHorizontalSpace(s)
	if s == "" {
		return nil, true
	}
	var tokens []string
	for pos := 0; pos < len(s); {
		for pos < len(s) && isASCIIHorizontalSpace(s[pos]) {
			pos++
		}
		if pos == len(s) {
			break
		}
		start := pos
		var quote byte
		for pos < len(s) {
			c := s[pos]
			if quote != 0 {
				if c == '\\' {
					if pos+1 >= len(s) {
						return nil, false
					}
					pos += 2
					continue
				}
				if c == quote {
					quote = 0
				}
				pos++
				continue
			}
			switch c {
			case '\'', '"':
				quote = c
				pos++
			case ' ', '\t':
				goto tokenDone
			default:
				pos++
			}
		}
	tokenDone:
		if quote != 0 || pos == start {
			return nil, false
		}
		tokens = append(tokens, s[start:pos])
	}
	return tokens, true
}

func isTraceKVKey(key string) bool {
	if key == "" || !((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= 'a' && key[0] <= 'z') || key[0] == '_') {
		return false
	}
	for i := 1; i < len(key); i++ {
		c := key[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func unwrapSchedSuffixValue(raw string) string {
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		raw = raw[1 : len(raw)-1]
	}
	return strings.TrimSpace(raw)
}

func parseKV(fields string) map[string]string {
	out := make(map[string]string)
	for _, m := range kvRE.FindAllStringSubmatch(fields, -1) {
		if len(m) == 3 {
			out[m[1]] = cleanTraceValue(m[2])
		}
	}
	parseColonKV(fields, out)
	parseSpaceKV(fields, out)
	return out
}

func parseColonKV(fields string, out map[string]string) {
	tokens := strings.Fields(fields)
	for _, token := range tokens {
		token = strings.Trim(strings.TrimSpace(token), ",")
		if strings.Contains(token, "=") {
			continue
		}
		idx := strings.IndexByte(token, ':')
		if idx <= 0 || idx >= len(token)-1 {
			continue
		}
		key := strings.ToLower(strings.Trim(strings.TrimSpace(token[:idx]), ":,"))
		if _, ok := spaceKVKeys[key]; !ok {
			continue
		}
		value := cleanTraceValue(token[idx+1:])
		if value == "" || value == "=" || strings.Contains(value, "==>") {
			continue
		}
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
}

func parseSpaceKV(fields string, out map[string]string) {
	tokens := strings.Fields(fields)
	for i := 0; i < len(tokens)-1; i++ {
		key := strings.ToLower(strings.Trim(strings.TrimSpace(tokens[i]), ":,"))
		if _, ok := spaceKVKeys[key]; !ok {
			continue
		}
		if strings.Contains(key, "=") {
			continue
		}
		valueIdx := i + 1
		if tokens[valueIdx] == "=" && valueIdx+1 < len(tokens) {
			valueIdx++
		}
		value := cleanTraceValue(tokens[valueIdx])
		if value == "" || value == "=" || strings.Contains(value, "==>") {
			continue
		}
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
}

// schedCommBoundaryKeys is the CLOSED key set that can follow a comm value
// inside sched_wakeup / sched_waking field text. sched_switch is deliberately
// absent: its core and comm values use the strict unique-key parser above.
var schedCommBoundaryKeys = []string{
	"prev_comm", "prev_pid", "prev_prio", "prev_state",
	"next_comm", "next_pid", "next_prio", "next_info",
	"cg", "cgroup",
	"comm", "pid", "prio", "target_cpu", "dest_cpu", "success",
}

// schedCommValue preserves the legacy wakeup/waking comm compatibility path.
// It is intentionally not reused by sched_switch, whose structural parser is
// the sole authority for both comm values and every typed suffix field.
func schedCommValue(fields, key, fallback string) string {
	marker := key + "="
	start := -1
	if strings.HasPrefix(fields, marker) {
		start = len(marker)
	} else if i := strings.Index(fields, " "+marker); i >= 0 {
		start = i + 1 + len(marker)
	}
	if start < 0 || start >= len(fields) {
		return fallback
	}
	rest := fields[start:]
	end := len(rest)
	for _, boundary := range schedCommBoundaryKeys {
		if boundary == key {
			continue
		}
		if i := strings.Index(rest, " "+boundary+"="); i >= 0 && i < end {
			end = i
		}
	}
	value := cleanTraceValue(rest[:end])
	if value == "" {
		return fallback
	}
	return value
}

func clockNameForEvent(raw, fields string) string {
	raw = strings.TrimSpace(raw)
	if raw != "clock_set_rate" {
		return raw
	}
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return raw
	}
	name := strings.Fields(fields)[0]
	if strings.Contains(name, "=") {
		return raw
	}
	return name
}

func isCPUFrequencyClock(fields string) bool {
	return isCPUFrequencyClockName(clockNameForEvent("clock_set_rate", fields))
}

// isCPUFrequencyClockName is a NAME HEURISTIC over clock_set_rate lane names.
// VS-2c 终局裁定 (§7.10, customer_dead_session_audit_20260703.md): clock lane
// names are vendor DTS free vocabulary (trace_streamer passes them through
// verbatim; no reference implementation binds them to a cluster or CPU), so
// this heuristic is a NOISY signal — soft guidance and corroboration caveats
// only. It MUST NOT feed the supply-fold fmax basis (structural pin in
// semantic_ruling_pins_test.go).
func isCPUFrequencyClockName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "pid_freq", "cpu_freq", "cpu_frequency", "cpufreq", "scaling_cur_freq":
		return true
	default:
		return strings.Contains(name, "cpu") && strings.Contains(name, "freq") && !strings.Contains(name, "ddr")
	}
}

// clockSetRatePositionalValue extracts the rate from the keyless
// clock_set_rate payload shape "<name> <rate> ..." (§7.11 B-2; hmtrace flavor
// emits e.g. "clock_set_rate: cpu-cluster.0 2200000.0" and
// "clock_set_rate: ddr_freq 1866000000"). The value is the token immediately
// after the first non-k=v token (the clock name, same anchor as
// clockNameForEvent); a k=v or absent successor yields "" so the keyed
// path's zero semantics hold. Callers gate on KEY ABSENCE (no state/
// frequency/freq key), a precise condition — keyed shapes never reach here.
func clockSetRatePositionalValue(fields string) string {
	tokens := strings.Fields(fields)
	for i, token := range tokens {
		if strings.Contains(token, "=") {
			continue
		}
		if i+1 >= len(tokens) || strings.Contains(tokens[i+1], "=") {
			return ""
		}
		return tokens[i+1]
	}
	return ""
}

func isTraceMarkPayload(fields string) bool {
	fields = normalizeTraceMarkPayload(fields)
	if fields == "" {
		return false
	}
	if fields == "E" {
		return true
	}
	switch {
	case strings.HasPrefix(fields, "B|"):
		return true
	case strings.HasPrefix(fields, "E|"):
		return true
	case strings.HasPrefix(fields, "C|"):
		return true
	case strings.HasPrefix(fields, "S|"):
		return true
	case strings.HasPrefix(fields, "F|"):
		return true
	case strings.HasPrefix(fields, "G|"):
		return true
	case strings.HasPrefix(fields, "H|"):
		return true
	case strings.HasPrefix(fields, "N|"):
		return true
	case strings.HasPrefix(fields, "I|"):
		return true
	default:
		return false
	}
}

// IsTraceMarkPayload exposes the parser's single trace-mark payload grammar to
// converter provenance code. It intentionally delegates to the unexported
// classifier instead of maintaining a second B/E/C/S/F/G/H/N/I grammar.
func IsTraceMarkPayload(fields string) bool {
	return isTraceMarkPayload(fields)
}

func parseTraceMark(fields string) (action string, spanPID int, name, value string) {
	parsed := parseTraceMarkValidated(fields)
	return parsed.action, parsed.spanPID, parsed.name, parsed.value
}

func normalizeTraceMarkPayload(fields string) string {
	fields = trimTraceMarkEnvelopeLeft(fields)
	if strings.TrimSpace(fields) == "" {
		return ""
	}
	if fields == "E" {
		// Bare E is an action scalar, not an opaque name. Canonicalize only this
		// scalar form; pipe-delimited payloads keep their complete right edge.
		return "E"
	}
	if isDirectTraceMarkPayload(fields) {
		return fields
	}
	if idx := strings.Index(fields, ":"); idx > 0 && idx+1 < len(fields) {
		prefix := strings.TrimSpace(fields[:idx])
		payload := trimTraceMarkEnvelopeLeft(fields[idx+1:])
		if tracePrintPrefixLooksLikeAddress(prefix) {
			// Standard address-carved mark: "0x<addr>: B|pid|name" — the payload
			// still leads with a supported trace-mark action letter; pass it through
			// byte-identical.
			if isDirectTraceMarkPayload(payload) {
				return payload
			}
			// §16 T-span variant: a HarmonyOS print-mark converter (trace_streamer
			// suspect) ate the leading action letter+pipe and left a "0x0:" address
			// residue, so the carved payload leads with the container-ns pid instead
			// of B|/E|/C|. Restore the action from structure; the canonical form then
			// re-enters the SAME parser/gate as a native mark.
			//
			// F1 gate (tighter than the pass-through above): the RESTORATION arm
			// additionally requires the prefix to LITERALLY begin with 0x/0X.
			// tracePrintPrefixLooksLikeAddress alone accepts any all-hex word
			// ("cafe", "1248") — fine for the pass-through, whose payload still
			// carries the action letter (a second precise factor), but fatal for
			// restoration: "print: 1248: 15" would forge E|15 and a forged E pops a
			// REAL open B off the per-thread (ev.PID-keyed) span stack, cascading
			// corruption down the whole thread. The full §16 signature is therefore:
			// literal 0x prefix ∧ all-hex residue ∧ first payload field pure digits.
			if strings.HasPrefix(strings.ToLower(prefix), "0x") {
				if restored, ok := restoreCarvedTraceMarkPayload(payload); ok {
					return restored
				}
			}
		}
	}
	return fields
}

// restoreCarvedTraceMarkPayload rebuilds the canonical B|/E|/C| trace-mark form
// from the §16 customer variant, where the converter dropped the leading action
// character (B|/E|/C|/S|/F|) and the payload — after the "0x0:" address residue
// is carved — begins with the container-ns pid.
//
// Discriminator (all precise structural signals, no heuristics):
//   - first field MUST be pure digits (the pid) and at most 8 digits — Linux
//     pid_max caps at 2^22 (7 digits); longer digit runs are data, not pids,
//     and are NOT restored (fail-closed);
//   - "<pid>"                      → End   (bare pid, no field after it)
//   - "<pid>|<tag>"                → End   (only a <UPPER><digits> tag after the
//     pid, no name; standard analogue: "E|1252|I39", "E|1727|M0538")
//   - "<pid>|<name>[|<tag>]"       → Begin (a name field follows the pid; the
//     optional trailing tag rides along as in "B|8091|H:…|I39")
//   - "<pid>|<name>|<numeric>"     → Counter (4-field form, e.g. carved
//     "C|60194|Heap size (KB)|71214")
//   - "<pid>|<name>|<numeric>|<tag>" → Counter (5-field form, e.g. carved
//     "C|1252|H:VSync-rs|0|I38"): real HarmonyOS counters put the VALUE BETWEEN
//     the name and the trailing tag, so the counter test is "does any
//     independent field after the name and before the tag parse as a plain
//     number", NOT "is the last field numeric". Counters map to C| so they can
//     never forge a sync span (only B/E enter the per-thread span stack).
//
// Pairing note: findSpanWindowsCompacted pairs B/E on ev.PID — the ftrace ROW
// pid — not on this payload pid. The restored SpanPID (container-ns pid N)
// identifies the span subject only; pairing works because a thread's B and E
// rows share the same row pid.
//
// Disclosed irreducible ambiguities (converter eats B|/E|/C|/S|/F| uniformly):
//   - VALUE-LESS counters: a carved "C|60194|Heap" arrives as "60194|Heap",
//     byte-identical to a carved Begin "B|15|setCoreSettings" → "15|setCoreSettings".
//     No precise signal separates them; default is Begin (real corpora:
//     donghu has 60/60 counters WITH a value field and zero value-less ones,
//     while 2-field Begins are the dominant span form). A misjudged value-less
//     counter becomes a dangling B: alone it produces NO span (unclosed B is
//     inert), but if the same thread later carries an unmatched E the dangling
//     B can consume it and forge a short span — documented residual risk,
//     see ledger §16.1.
//   - TAG-SHAPED SHORT NAMES: a 2-field carved Begin whose name itself matches
//     ^[A-Z][0-9]+$ (e.g. a span literally named "V8" or "T1") is judged End,
//     because "<pid>|<tag>" End rows are pervasive in real captures while
//     spans named exactly like a tag are not. Same-family tradeoff, disclosed.
//   - ASYNC S/F cannot be reconstructed: a carved S/F loses the action letter
//     AND its cookie is indistinguishable from a counter value; real corpora
//     (donghu) contain zero S/F print marks, so this stays out-of-distribution
//     and unrestored rather than guessed.
func restoreCarvedTraceMarkPayload(payload string) (string, bool) {
	// Carved restoration is an explicitly lossy compatibility lane: the old
	// converter already removed the action. Canonicalize only the inferred
	// action/PID/value/tag scalars; retain the still-present opaque name edge.
	payload = trimTraceMarkEnvelopeLeft(payload)
	if strings.TrimSpace(payload) == "" {
		return "", false
	}
	parts := strings.Split(payload, "|")
	pid := strings.TrimSpace(parts[0])
	if !isAllDigits(pid) {
		// First field is an action letter or prose, not a carved pid — not the
		// variant. (A standard "0x<addr>: B|…" already returned above.)
		return "", false
	}
	if len(pid) > 8 {
		// Longer than any Linux pid (pid_max ≤ 2^22, 7 digits; 8 is generous)
		// — a timestamp/hash datum, not a carved pid. Not restored.
		return "", false
	}
	// The canonical restored form always keeps the pid as the field right after
	// the action letter, exactly as native "B|<pid>|…" / "E|<pid>…" — the join
	// below therefore reattaches the whole original payload (pid + tail).
	switch len(parts) {
	case 1:
		// "<pid>" — bare End.
		return "E|" + pid, true
	case 2:
		f1 := strings.TrimSpace(parts[1])
		if isInstanceTag(f1) {
			// "<pid>|<tag>" — End carrying only an instance tag, no name
			// (isomorphic to the native "E|1252|I39" shape). Disclosed tradeoff:
			// this also captures a Begin whose literal name is tag-shaped ("V8").
			return "E|" + pid + "|" + f1, true
		}
		// "<pid>|<name>" — Begin with a name and no tag. Disclosed tradeoff:
		// this also captures a value-less counter ("C|60194|Heap" carved) —
		// irreducible, see the function comment.
		return "B|" + pid + "|" + parts[1], true
	default:
		// Three or more fields: "<pid>|<name>|<tail…>".
		last := strings.TrimSpace(parts[len(parts)-1])
		if isInstanceTag(last) {
			// Trailing tag: counter vs Begin is decided by whether an
			// INDEPENDENT plain-numeric field sits between the name and the
			// tag — the real 5-field counter shape ("C|1252|H:VSync-rs|0|I38"
			// carves to "1252|H:VSync-rs|0|I38"). Checking only the last field
			// would misjudge every such counter as Begin and forge a fake sync
			// span out of the counter row plus a later unmatched End.
			for _, mid := range parts[2 : len(parts)-1] {
				if isAllNumeric(strings.TrimSpace(mid)) {
					canonical := append([]string{pid}, parts[1:len(parts)-1]...)
					canonical = append(canonical, last)
					return "C|" + strings.Join(canonical, "|"), true
				}
			}
			// "<pid>|<name>|…|<tag>" with no numeric mid-field — Begin; the
			// trailing tag rides along exactly as native "B|pid|name|I<n>".
			canonical := append([]string{pid}, parts[1:len(parts)-1]...)
			canonical = append(canonical, last)
			return "B|" + strings.Join(canonical, "|"), true
		}
		if isAllNumeric(last) {
			// "<pid>|<name>|<numeric>" — 4-field counter/cookie form, NOT a
			// tag. Map to C so the counter-delta path can read the value and
			// the sync-span stack (B/E only) can never forge a span window.
			canonical := append([]string{pid}, parts[1:len(parts)-1]...)
			canonical = append(canonical, last)
			return "C|" + strings.Join(canonical, "|"), true
		}
		// Any other trailing field: treat the whole tail as a Begin name/payload
		// (the native parser keeps parts[3:] in value/raw for literal search).
		return "B|" + pid + "|" + strings.Join(parts[1:], "|"), true
	}
}

// isInstanceTag reports whether a trace-mark field is a HarmonyOS instance /
// marker tag of the form "<UPPER><digits>" — a single uppercase letter followed
// by pure digits (e.g. "I38", "I39", "M0538"). These ride on both End rows
// ("E|1252|I39", "E|1727|M0538") and the tail of Begin rows
// ("B|8091|H:…|I39", "B|1727|H:…|M0538") and are never a span name. The letter
// family is NOT closed to I: undamaged reference captures (both tracing_mark_write
// and print, container-ns pid) show M-tags on the same Render()/frame spans, so
// the gate is structural (^[A-Z][0-9]+$), not an I-literal — still precise, a
// real span name never has this exact shape.
func isInstanceTag(f string) bool {
	f = strings.TrimSpace(f)
	if len(f) < 2 {
		return false
	}
	if f[0] < 'A' || f[0] > 'Z' {
		return false
	}
	return isAllDigits(f[1:])
}

// isAllDigits reports whether s is non-empty and every rune is an ASCII digit.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isAllNumeric reports whether s is a PLAIN integer/decimal literal (a counter
// reading or async cookie): optional sign, digits, at most one dot. The value
// domain is deliberately tighter than strconv.ParseFloat, which also accepts
// "Inf"/"NaN"/hex floats/exponents — those are names, not counter values, and
// must not flip a Begin into a Counter.
func isAllNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	digits, dots := 0, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] >= '0' && s[i] <= '9':
			digits++
		case s[i] == '.':
			dots++
		default:
			return false
		}
	}
	return digits > 0 && dots <= 1
}

func isDirectTraceMarkPayload(fields string) bool {
	fields = trimTraceMarkEnvelopeLeft(fields)
	if fields == "E" {
		return true
	}
	return strings.HasPrefix(fields, "B|") ||
		strings.HasPrefix(fields, "E|") ||
		strings.HasPrefix(fields, "C|") ||
		strings.HasPrefix(fields, "S|") ||
		strings.HasPrefix(fields, "F|") ||
		strings.HasPrefix(fields, "G|") ||
		strings.HasPrefix(fields, "H|") ||
		strings.HasPrefix(fields, "N|") ||
		strings.HasPrefix(fields, "I|")
}

func trimTraceMarkEnvelopeLeft(raw string) string {
	// Systrace/ftrace separators are ASCII spaces or tabs. Do not use
	// TrimSpace: CR/LF, Unicode separators and the payload's right edge are
	// integrity-relevant bytes and must remain visible to the typed parser.
	return strings.TrimLeft(raw, " \t")
}

func tracePrintPrefixLooksLikeAddress(prefix string) bool {
	prefix = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(prefix), "0x"))
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func parseIPIReason(fields string) string {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return ""
	}
	if start := strings.Index(fields, "("); start >= 0 {
		if end := strings.LastIndex(fields, ")"); end > start {
			return strings.TrimSpace(fields[start+1 : end])
		}
	}
	return fields
}

func parseIPITargetCPUs(mask string) []int {
	mask = strings.Trim(strings.TrimSpace(mask), ",")
	if mask == "" {
		return nil
	}
	value := uint64(atoi64Auto(mask))
	if value == 0 {
		return nil
	}
	var out []int
	for cpu := 0; cpu < 64; cpu++ {
		if value&(uint64(1)<<uint(cpu)) != 0 {
			out = append(out, cpu)
		}
	}
	return out
}

// strictBinderTransactionID keeps the retained typed payload aligned with the
// single pairing authority. Endpoint rows require one unambiguous alias;
// auxiliary metadata may repeat distinct aliases only when their canonical
// positive transaction values agree. Invalid wire remains inventory with a
// zero typed ID and can never be made pairable by parseKV's last-value wins.
func strictBinderTransactionID(fields string, coherentAliases bool) int {
	var raw string
	var ok bool
	if coherentAliases {
		raw, ok = strictCoherentPairingAlias(fields, "transaction", "debug_id", "transaction_id")
	} else {
		raw, ok = strictUniquePairingAlias(fields, "transaction", "debug_id", "transaction_id")
		if ok {
			raw, ok = canonicalPositiveDecimalIdentity(raw)
		}
	}
	if !ok {
		return 0
	}
	return atoi(raw)
}

func atoi(raw string) int {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func atoi64(raw string) int64 {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

func atoi64Auto(raw string) int64 {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		n, _ := strconv.ParseInt(raw[2:], 16, 64)
		return n
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

// atoiFloatTolerantMax bounds the float-fallback path of atoiFloatTolerant
// (§7.11 A-1 sequel, 2026-07-04 review). Every consumption point is a
// frequency/idle-state magnitude: real CPU frequency points top out well
// under 1e10 whether the vendor lane emits kHz or Hz, and the u32 cpu_idle
// exit sentinel (4294967295) sits below it too. Values ABOVE it are sentinel
// semantics, not magnitudes — u64 max ("18446744073709551615.0"), 1e19,
// 1e300 — and must not truncate into a fake positive frequency.
const atoiFloatTolerantMax = 1e10

// atoiFloatTolerant parses an INT-declared numeric trace field that some
// flavors emit as a float string (§7.11 A-1, customer_dead_session_audit_
// 20260703.md): hmtrace REAL columns render "2200000.0" while
// export_format.rs declares the field INT — both shapes coexist in the wild,
// and a plain Atoi silently zeroed the float shape, fail-quietly emptying the
// whole frequency timeline (fmax/residency). Integer strings keep the Atoi
// fast path byte-for-byte; float strings truncate; non-numeric input stays 0
// (same fail-quiet contract as atoi). This helper is for the INT-declared
// consumption points ONLY (cpu_idle state, cpu_frequency, limits min/max,
// clock_set_rate) — genuinely-float fields keep parseFloat untouched.
//
// A-1 sequel (2026-07-04 review): the float fallback range-checks BEFORE the
// int conversion. int(f) on a beyond-int64 float saturates to a POSITIVE
// math.MaxInt64 on arm64/amd64, which sails through every <=0 consumer
// filter and pollutes fmax / fabricates supply gaps (u64 sentinel and 1e19
// shapes observed in review). Non-finite, negative, or >atoiFloatTolerantMax
// floats collapse to 0 — the same fail-quiet contract as non-numeric input.
func atoiFloatTolerant(raw string) int {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	if math.IsNaN(f) || f < 0 || f > atoiFloatTolerantMax {
		return 0
	}
	return int(f)
}

func parseLatencyMs(kv map[string]string) float64 {
	value, _ := parseLatencyMsChecked(kv)
	return value
}

// parseLatencyMsChecked distinguishes a missing/zero latency (valid inventory
// input) from an explicitly malformed numeric claim (invalid event input).
func parseLatencyMsChecked(kv map[string]string) (float64, bool) {
	if len(kv) == 0 {
		return 0, true
	}
	groups := []struct {
		keys    []string
		divisor float64
	}{
		{keys: []string{"latency_ms", "duration_ms", "dur_ms"}, divisor: 1},
		{keys: []string{"latency_us", "duration_us", "dur_us"}, divisor: 1000},
		{keys: []string{"latency_ns", "duration_ns", "dur_ns"}, divisor: 1000000},
		{keys: []string{"latency", "duration", "dur"}, divisor: 1},
	}
	for _, group := range groups {
		for _, key := range group.keys {
			raw := strings.TrimSpace(kv[key])
			if raw == "" {
				continue
			}
			value, ok := parseFiniteTraceNumber(raw)
			if !ok {
				// A malformed higher-precedence field poisons the latency
				// claim. Do not fall through to a second unit spelling and
				// accidentally mint a usable duration from conflicting input.
				return 0, false
			}
			if value < 0 {
				return 0, false
			}
			if value == 0 {
				continue
			}
			latencyMs := value / group.divisor
			if !isFiniteTraceNumber(latencyMs) || latencyMs <= 0 {
				return 0, false
			}
			return latencyMs, true
		}
	}
	return 0, true
}

func parseFloat(raw string) float64 {
	v, ok := parseFiniteTraceNumber(raw)
	if !ok {
		return 0
	}
	return v
}

func atoiMaybe(raw string) (int, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

func boolMaybe(raw string) (bool, bool) {
	raw = strings.ToLower(strings.Trim(strings.TrimSpace(raw), ":,"))
	switch raw {
	case "1", "t", "true", "y", "yes", "known", "valid", "available":
		return true, true
	case "0", "f", "false", "n", "no", "unknown", "invalid", "unavailable":
		return false, true
	default:
		return false, false
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// clampString caps s at a TOTAL byte budget of n (the legacy contract: the
// "..." decoration is counted INSIDE the budget, hence the n-3 cut). HYG
// (§28.2 顺手项 b, 2026-07-09): the byte cuts go through the shared rune-safe
// primitive so a budget landing inside a multibyte rune (CJK span names,
// interned field text) can no longer emit a broken tail — pure-ASCII behavior
// is byte-identical to the old s[:n-3]+"..." shape. The ASCII "..." decoration
// is deliberately kept (not "…"): the 3-byte ellipsis is part of the budget
// arithmetic and of existing output pins.
func clampString(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 3 {
		return types.CutPrefixRuneSafe(s, n)
	}
	return types.CutPrefixRuneSafe(s, n-3) + "..."
}

func cleanTraceValue(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimRight(raw, ",")
	return raw
}

func cleanTraceValueBounded(raw string, maxLen int) string {
	return clampString(cleanTraceValue(raw), maxLen)
}

func normalizeFileRW(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.Trim(raw, `"'`)
	switch {
	case raw == "":
		return ""
	case raw == "r" || raw == "read" || strings.Contains(raw, "dataread"):
		return "read"
	case raw == "w" || raw == "write" || strings.Contains(raw, "datawrite"):
		return "write"
	case strings.Contains(raw, "read") && strings.Contains(raw, "bio"):
		return "read_bio"
	case strings.Contains(raw, "write") && strings.Contains(raw, "bio"):
		return "write_bio"
	case strings.Contains(raw, "sync"):
		return "sync"
	default:
		return raw
	}
}

func fileOperationFromEventName(name string) string {
	if profile, exact := exactF2FSPairingProfile(name); exact {
		switch profile.SemanticBase {
		case "f2fs_sync_file":
			return "sync"
		case "f2fs_write":
			return "write"
		default:
			return ""
		}
	}
	if F2FSClosedEndpointNameCandidate(name) {
		return ""
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "dataread"):
		return "read"
	case strings.Contains(name, "datawrite"):
		return "write"
	case strings.Contains(name, "submit_read_bio"):
		return "read_bio"
	case strings.Contains(name, "submit_write_bio"):
		return "write_bio"
	case strings.Contains(name, "direct_io"):
		return "direct_io"
	case strings.Contains(name, "sync_file"):
		return "sync"
	case name == pageCacheAddEventName:
		return "page_cache_add"
	case name == pageCacheDeleteEventName:
		return "page_cache_delete"
	default:
		return ""
	}
}

type stringInterner struct {
	values map[string]string
	// retainedBytes accumulates the byte length of every DISTINCT string
	// this interner (and thus the index) retains — the real memory cost
	// the cache accounting charges (unsafe.Sizeof sees only headers).
	retainedBytes int64
}

// maxInternerEntries bounds the interner map (P3, 2026-07-03): interning is
// a memory optimization, not a semantic requirement, and a pathological
// trace with millions of distinct clamped payloads must not grow the map
// without bound. Past the cap, strings pass through un-interned but their
// bytes still count toward retainedBytes.
const maxInternerEntries = 512 << 10

func newStringInterner() *stringInterner {
	return &stringInterner{values: make(map[string]string)}
}

// newNonRetainingStringInterner is for streaming consumers which never retain
// Event values after the synchronous callback. A nil values map is a precise
// no-retention mode: parsed strings pass through for the callback lifetime and
// no distinct-string census accumulates across physical lines.
func newNonRetainingStringInterner() *stringInterner {
	return &stringInterner{}
}

func (i *stringInterner) intern(s string) string {
	if s == "" || i == nil {
		return s
	}
	if i.values == nil {
		return s
	}
	if existing, ok := i.values[s]; ok {
		return existing
	}
	i.retainedBytes += int64(len(s))
	if len(i.values) < maxInternerEntries {
		i.values[s] = s
	}
	return s
}

// safeParseLine isolates per-line parse panics: trace artifacts are
// untrusted input, and a single pathological line must degrade to a
// typed counter instead of killing the whole query. The recover is
// function-scoped so the hot loop pays only the call overhead.
func safeParseLine(lineNo int, line string, intern *stringInterner, idx *Index) (ev Event, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if idx != nil {
				idx.ParseLinePanics++
				for _, failure := range cpuInputValidationFailures(lineNo, line) {
					failure.SourcePath = idx.Path
					appendCPUInputIntegrityFailure(idx, failure)
				}
			}
			ev, ok = Event{}, false
		}
	}()
	ev, ok = parseLineFn(lineNo, line, intern)
	if idx != nil && (!ok || ev.CPUInputInvalid) {
		for _, failure := range cpuInputValidationFailures(lineNo, line) {
			failure.SourcePath = idx.Path
			appendCPUInputIntegrityFailure(idx, failure)
		}
	}
	return ev, ok
}

// safeParseLineScan is safeParseLine over the shared per-line memo. Same
// panic isolation and typed-counter degrade contract; the cpu_input witness
// paths consume the memoized header match instead of re-running the regex.
func safeParseLineScan(s *lineScan, intern *stringInterner, idx *Index) (ev Event, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if idx != nil {
				idx.ParseLinePanics++
				if cpuInputRawCandidate(s.line) {
					for _, failure := range cpuInputValidationFailuresScan(s) {
						failure.SourcePath = idx.Path
						appendCPUInputIntegrityFailure(idx, failure)
					}
				}
			}
			ev, ok = Event{}, false
		}
	}()
	ev, ok = parseLineScanFn(s, intern)
	if idx != nil && (!ok || ev.CPUInputInvalid) {
		if cpuInputRawCandidate(s.line) {
			for _, failure := range cpuInputValidationFailuresScan(s) {
				failure.SourcePath = idx.Path
				appendCPUInputIntegrityFailure(idx, failure)
			}
		}
	}
	return ev, ok
}

// parseLineFn indirects ParseLine so the recover seam is testable with
// an injected panic; production always points at ParseLine.
var parseLineFn = ParseLine

// parseLineScanFn is the same testable recover seam for the memoized parse
// used by the main windowed/full parse loop.
var parseLineScanFn = parseLineScan

// recordUnparsedSample retains one unparseable-line witness on the typed
// Index face (TDIAG B4, §28.13): first IndexUnparsedSampleCap lines only,
// text rune-safe byte-capped (types.CutPrefixRuneSafe — the shared truncation
// authority; a 2MiB pathological line retains ≤480 bytes). The cap check
// comes first so the hot parse loop pays one length compare and zero
// allocations once the sample face is full.
func (idx *Index) recordUnparsedSample(line int, text string) {
	if idx == nil || len(idx.UnparsedSamples) >= IndexUnparsedSampleCap {
		return
	}
	idx.UnparsedSamples = append(idx.UnparsedSamples, UnparsedLineSample{
		Line: line,
		Text: types.CutPrefixRuneSafe(text, indexUnparsedSampleTextBytes),
	})
}
