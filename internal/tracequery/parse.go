package tracequery

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
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
	"unicode/utf8"
	"unsafe"

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

const (
	maxPerfSampleTextFieldLen = 512
	maxPerfCallchainFieldLen  = 2048
)

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
	return int64(len(idx.Events))*eventSizeBytes + idx.RetainedStringBytes + idx.RetainedSideTableBytes + headBytes + durationAuditBytes + cpuInputAuditBytes + traceMarkAuditBytes
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

func BuildIndexWithOptions(ctx context.Context, path string, opts BuildOptions) (*Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	path = promoteSiblingTraceBundlePath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	sourceBytes, sourceKey, err := traceIndexSourceIdentity(path, info)
	if err != nil {
		return nil, err
	}
	opts = normalizeBuildOptions(opts)
	windowKey := opts.cacheKey()
	cacheable := shouldCacheTraceIndex(sourceBytes, opts)
	key := parseCacheKey{
		path:      path,
		size:      info.Size(),
		modUnix:   info.ModTime().UnixNano(),
		version:   ParserVersion,
		windowKey: windowKey,
		sourceKey: sourceKey,
	}
	if cacheable {
		if idx, ok := indexCache.Load(key); ok {
			return idx, nil
		}
	}
	// A relation-scoped index must be built by pruning during the streamed
	// parse; deriving it from a cached FULL index would hand back the unpruned
	// window (correct data, but defeats the memory goal and misses the point).
	// Only non-scoped windowed indices reuse the full-cache derive fast path.
	if opts.windowed() && !opts.relationScoped() {
		fullKey := parseCacheKey{
			path:      path,
			size:      info.Size(),
			modUnix:   info.ModTime().UnixNano(),
			version:   ParserVersion,
			sourceKey: sourceKey,
		}
		if idx, ok := indexCache.Load(fullKey); ok {
			derived := deriveWindowedIndex(idx, opts)
			auditQ := Query{TimeStart: derived.IndexTimeStart, TimeEnd: derived.IndexTimeEnd, LineStart: derived.IndexLineStart, LineEnd: derived.IndexLineEnd}
			derived.schedulerOrderFailures = nil
			derived.durationOrderFailures = nil
			derived.durationOrderFailuresCapped = nil
			derived.schedulerRowIntegrityFailures = nil
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
			return derived, nil
		}
	}
	idx, err := buildIndexSingleflight(ctx, key, path, info.Size(), info.ModTime().UnixNano(), opts, cacheable)
	if err != nil {
		return idx, err
	}
	currentEntry, statErr := os.Stat(path)
	if statErr != nil {
		return nil, fmt.Errorf("revalidate trace source identity after parse: %w", statErr)
	}
	_, currentSourceKey, identityErr := traceIndexSourceIdentity(path, currentEntry)
	if identityErr != nil {
		return nil, identityErr
	}
	if currentSourceKey != sourceKey {
		return nil, fmt.Errorf("trace source artifacts changed while the index was being built; retry with a stable capture")
	}
	if opts.windowed() && opts.TimeStartSet && opts.TimeStart > 0 {
		if err := populateWindowSchedulerHead(ctx, idx, opts.TimeStart); err != nil {
			return nil, err
		}
		// The checkpoint scan is a second read phase. Revalidate the complete
		// artifact universe again so an atomic replacement between the index and
		// head phases cannot publish a mixed-version result.
		finalEntry, finalStatErr := os.Stat(path)
		if finalStatErr != nil {
			return nil, fmt.Errorf("revalidate trace source identity after scheduler head scan: %w", finalStatErr)
		}
		_, finalSourceKey, finalIdentityErr := traceIndexSourceIdentity(path, finalEntry)
		if finalIdentityErr != nil {
			return nil, finalIdentityErr
		}
		if finalSourceKey != sourceKey {
			return nil, fmt.Errorf("trace source artifacts changed while the scheduler head checkpoint was being built; retry with a stable capture")
		}
	}
	return idx, nil
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
// sibling artifact.  The same value keys the cache and in-flight singleflight;
// total bytes (not the small manifest/primary size) decide cacheability.
func traceIndexSourceIdentity(path string, entry os.FileInfo) (int64, string, error) {
	paths := []string{canonicalTraceIndexPath(path)}
	if traceBundlePath(path) {
		body, err := os.ReadFile(path)
		if err != nil {
			return 0, "", err
		}
		var bundle traceBundleFile
		if err := json.Unmarshal(body, &bundle); err != nil {
			return 0, "", fmt.Errorf("parse trace bundle %s for source identity: %w", path, err)
		}
		paths = append(paths, traceBundleIndexPaths(path, bundle)...)
	} else {
		paths = append(paths, siblingTraceArtifactPaths(path)...)
	}
	seen := map[string]bool{}
	var total int64
	var key strings.Builder
	for _, candidate := range paths {
		candidate = canonicalTraceIndexPath(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		info := entry
		if candidate != canonicalTraceIndexPath(path) || info == nil {
			var err error
			info, err = os.Stat(candidate)
			if err != nil {
				return 0, "", fmt.Errorf("stat trace source artifact %s: %w", candidate, err)
			}
		}
		total += info.Size()
		identity := traceFileIdentityFromInfo(info)
		fmt.Fprintf(&key, "%d:%s:%s|", len(candidate), candidate, identity.cacheToken())
	}
	return total, key.String(), nil
}

func canonicalTraceIndexPath(path string) string {
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		abs = filepath.Clean(abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			return filepath.Clean(real)
		}
		return abs
	}
	return path
}

func buildIndexSingleflight(ctx context.Context, key parseCacheKey, path string, size int64, modUnix int64, opts BuildOptions, cacheable bool) (*Index, error) {
	indexBuildMu.Lock()
	if call := indexBuilds[key]; call != nil {
		indexBuildMu.Unlock()
		select {
		case <-call.done:
			return call.idx, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &indexBuildCall{done: make(chan struct{})}
	indexBuilds[key] = call
	indexBuildMu.Unlock()

	call.idx, call.err = parseFile(ctx, path, size, modUnix, opts)
	if call.err == nil && cacheable {
		indexCache.Store(key, call.idx)
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
		Path:                                  full.Path,
		Size:                                  full.Size,
		ModTime:                               full.ModTime,
		TraceArtifacts:                        append([]TraceArtifactSource(nil), full.TraceArtifacts...),
		LineCount:                             full.LineCount,
		ScannedLineCount:                      full.ScannedLineCount,
		Windowed:                              true,
		IndexTimeStart:                        paddedTimeStart(opts),
		IndexTimeEnd:                          paddedTimeEnd(opts),
		IndexLineStart:                        paddedLineStart(opts),
		IndexLineEnd:                          paddedLineEnd(opts),
		TraceFlavor:                           full.TraceFlavor,
		FlavorConfidence:                      full.FlavorConfidence,
		FlavorSignals:                         append([]string(nil), full.FlavorSignals...),
		Caveats:                               append([]string(nil), full.Caveats...),
		ClockRegressions:                      full.ClockRegressions,
		TimestampOrder:                        full.TimestampOrder,
		schedulerOrderFailures:                append([]schedulerOrderViolation(nil), full.schedulerOrderFailures...),
		durationOrderFailures:                 append([]durationOrderViolation(nil), full.durationOrderFailures...),
		durationOrderFailuresCapped:           cloneDurationOrderCapped(full.durationOrderFailuresCapped),
		schedulerRowIntegrityFailures:         append([]schedulerRowIntegrityFailure(nil), full.schedulerRowIntegrityFailures...),
		cpuInputIntegrityFailures:             append([]cpuInputIntegrityFailure(nil), full.cpuInputIntegrityFailures...),
		cpuInputIntegrityFailuresCapped:       full.cpuInputIntegrityFailuresCapped,
		traceMarkIntegrityFailures:            append([]traceMarkIntegrityFailure(nil), full.traceMarkIntegrityFailures...),
		traceMarkIntegrityFailuresCapped:      full.traceMarkIntegrityFailuresCapped,
		traceMarkIntegrityDroppedGlobalPoison: full.traceMarkIntegrityDroppedGlobalPoison,
		traceTrackIntegrityDroppedPoison:      full.traceTrackIntegrityDroppedPoison,
		threadIncarnationFailures:             append([]threadIncarnationConflict(nil), full.threadIncarnationFailures...),
		threadIncarnationFailuresCapped:       full.threadIncarnationFailuresCapped,
		schedulerOrderFailuresCapped:          full.schedulerOrderFailuresCapped,
		schedulerRowIntegrityFailuresCapped:   full.schedulerRowIntegrityFailuresCapped,
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
		return false
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
	ts                   float64
	tsOK                 bool
	tsTried              bool
}

func (s *lineScan) reset(lineNo int, line string) {
	s.lineNo, s.line = lineNo, line
	s.mTried, s.kvTried, s.tsTried = false, false, false
	s.schedSwitchKVFailure = ""
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
			if rawType == "sched_switch" {
				// Do not fall back to the generic token scanner on structural
				// failure. Its context-free matches cannot distinguish a comm's
				// text from typed scheduler fields, so the scheduler integrity gate
				// must see an empty map and reject the malformed row instead.
				s.kv, s.schedSwitchKVFailure = parseSchedSwitchKV(fields)
			} else {
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
		if m := s.match(); len(m) != 0 {
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
		line, readErr := r.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			ts, hasTS := parseLineTimestamp(trimmed)
			recorder.observe(lineNo, len(line), ts, hasTS)
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
	if traceBundlePath(path) {
		return parseTraceBundleFile(ctx, path, size, modUnix, opts)
	}
	if companions := siblingTraceArtifactPaths(path); len(companions) > 0 {
		return parseTraceArtifactPathList(ctx, path, size, modUnix, opts, append([]string{path}, companions...))
	}
	idx, err := parseSingleTraceFile(ctx, path, size, modUnix, opts)
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

func parseSingleTraceFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return nil, err
	}
	openedIdentity := traceFileIdentityFromInfo(openedInfo)
	if openedInfo.Size() != size || openedInfo.ModTime().UnixNano() != modUnix {
		return nil, fmt.Errorf("trace source identity changed before its parser opened the artifact")
	}

	idx := &Index{Path: path, Size: size, ModTime: time.Unix(0, modUnix)}
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
		s, caveats, derr := discoverRelationScope(ctx, path, opts)
		if derr != nil {
			return nil, derr
		}
		relScope = s
		relScopeCaveats = caveats
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

	r := bufio.NewReaderSize(f, 256*1024)
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
		line, err := r.ReadString('\n')
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
			if retained || (idx.Windowed && durationCandidate && durationEndpointRowFailure == nil) || schedulerHeadCandidate {
				if retained {
					ev, evOK = safeParseLineScan(&scan, intern, idx)
				} else {
					ev, evOK = safeParseLineScan(&scan, auditIntern, auditScratch)
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
					recorder.observe(lineNo, len(line), lineTs, lineHasTS)
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
				recorder.observe(lineNo, len(line), lineTs, lineHasTS)
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
							return nil, proofErr
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
			return nil, err
		}
	}
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
		idx.relationScopeTIDs = make(map[int]bool, len(relScope.relevantTids))
		for tid := range relScope.relevantTids {
			idx.relationScopeTIDs[tid] = true
		}
	}
	if len(idx.TraceArtifacts) == 0 {
		finalInfo, statErr := f.Stat()
		if statErr != nil {
			return nil, statErr
		}
		if !openedIdentity.matchesInfo(finalInfo) {
			return nil, fmt.Errorf("trace source changed while it was being parsed")
		}
		idx.TraceArtifacts = []TraceArtifactSource{singleTraceArtifactSourceWithIdentity(path, openedIdentity, idx.LineCount, len(idx.Events))}
	}
	return idx, nil
}

type traceBundleFile struct {
	Version             string                          `json:"version"`
	InputPath           string                          `json:"input_path"`
	Systrace            string                          `json:"systrace"`
	Artifacts           []traceBundleArtifact           `json:"artifacts"`
	ProviderDecisions   []traceBundleProviderDecision   `json:"provider_decisions"`
	TraceDecisions      []traceBundleTraceDecision      `json:"trace_provider_decisions"`
	TraceDBCoverage     []traceBundleCoverage           `json:"trace_db_coverage"`
	TraceCoverage       []traceBundleCoverage           `json:"trace_coverage"`
	TraceToolGates      []traceBundleTraceToolGate      `json:"trace_tool_gates"`
	PerfClockAlignments []traceBundlePerfClockAlignment `json:"perf_clock_alignments"`
	Caveats             []string                        `json:"caveats"`
}

const traceBundleCoverageCaveatLimit = 24

const traceBundleCoveragePriorityCaveatLimit = 1

const traceBundleTraceToolGateCaveatLimit = 8

type traceBundleArtifact struct {
	Type      string                     `json:"type"`
	Path      string                     `json:"path"`
	Converter string                     `json:"converter,omitempty"`
	Perf      *traceBundlePerfCapability `json:"perf_capability,omitempty"`
	Caveats   []string                   `json:"caveats,omitempty"`
}

type traceBundlePerfCapability struct {
	ProviderKind    string   `json:"provider_kind,omitempty"`
	ProviderName    string   `json:"provider_name,omitempty"`
	InputFormat     string   `json:"input_format,omitempty"`
	OutputFormat    string   `json:"output_format,omitempty"`
	TimeDomain      string   `json:"time_domain,omitempty"`
	TimeAlignment   string   `json:"time_alignment,omitempty"`
	ThreadIdentity  string   `json:"thread_identity,omitempty"`
	CPUIdentity     string   `json:"cpu_identity,omitempty"`
	EventWeight     string   `json:"event_weight,omitempty"`
	Symbolization   string   `json:"symbolization,omitempty"`
	Callchain       string   `json:"callchain,omitempty"`
	DSOLabel        string   `json:"dso_label,omitempty"`
	BuildID         string   `json:"build_id,omitempty"`
	OffCPU          string   `json:"off_cpu,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	TraceQueryReady bool     `json:"trace_query_ready,omitempty"`
	Degraded        bool     `json:"degraded,omitempty"`
	Caveats         []string `json:"caveats,omitempty"`
}

type traceBundleProviderDecision struct {
	Stage           string `json:"stage,omitempty"`
	ProviderKind    string `json:"provider_kind,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	InputFormat     string `json:"input_format,omitempty"`
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
	Family         string   `json:"family,omitempty"`
	Table          string   `json:"table,omitempty"`
	Role           string   `json:"role,omitempty"`
	Found          bool     `json:"found"`
	ColumnsPresent []string `json:"columns_present,omitempty"`
	ColumnsMissing []string `json:"columns_missing,omitempty"`
	RowsRead       int      `json:"rows_read,omitempty"`
	RowsEmitted    int      `json:"rows_emitted,omitempty"`
	PeakBuffered   int      `json:"peak_buffered_rows,omitempty"`
	SpillChunks    int      `json:"spill_chunks,omitempty"`
	TempBytes      int64    `json:"temp_bytes,omitempty"`
	ElapsedUS      int64    `json:"elapsed_us,omitempty"`
	Skipped        string   `json:"skipped,omitempty"`
	Error          string   `json:"error,omitempty"`
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
	if bundle := siblingTraceBundlePath(path); bundle != "" {
		return bundle
	}
	return path
}

func siblingTraceBundlePath(path string) string {
	requested := canonicalTraceIndexPath(path)
	base := traceArtifactBase(requested)
	if base == "" {
		return ""
	}
	candidate := canonicalTraceIndexPath(base + ".tracebundle.json")
	body, err := os.ReadFile(candidate)
	if err != nil {
		// A sibling manifest is optional metadata. An unreadable candidate must
		// not make the explicitly requested physical trace unusable.
		return ""
	}
	var bundle traceBundleFile
	if err := json.Unmarshal(body, &bundle); err != nil {
		// Likewise, a partial/stale invalid manifest cannot hijack a direct
		// systrace request. BuildIndex will continue with requested itself.
		return ""
	}
	for _, spec := range traceBundleArtifactSpecs(candidate, bundle) {
		if spec.source.Kind == "systrace" && canonicalTraceIndexPath(spec.source.SourcePath) == requested {
			return candidate
		}
	}
	// Basename proximity is not capture provenance. Only an explicit,
	// canonically resolved systrace declaration authorizes auto-promotion.
	return ""
}

func siblingTraceArtifactPaths(path string) []string {
	if traceBundlePath(path) {
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
		if _, err := os.Stat(candidate); err == nil {
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
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bundle traceBundleFile
	if err := json.Unmarshal(body, &bundle); err != nil {
		return nil, fmt.Errorf("parse trace bundle %s: %w", path, err)
	}
	artifactSpecs := traceBundleArtifactSpecs(path, bundle)
	if len(artifactSpecs) == 0 {
		return nil, fmt.Errorf("trace bundle %s has no systrace or perftrace artifacts", path)
	}
	idx, err := parseTraceArtifactSpecs(ctx, path, size, modUnix, opts, artifactSpecs)
	if err != nil {
		return nil, err
	}
	idx.Caveats = append(idx.Caveats, traceBundleCaveats(bundle)...)
	return idx, nil
}

func traceBundleCaveats(bundle traceBundleFile) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if existing == s {
				return
			}
		}
		out = append(out, s)
	}
	for _, caveat := range bundle.Caveats {
		add("tracebundle_caveat: " + caveat)
	}
	for _, artifact := range bundle.Artifacts {
		kind := traceBundleLabel(artifact.Type, artifact.Path)
		for _, caveat := range artifact.Caveats {
			add(fmt.Sprintf("tracebundle_artifact %s caveat: %s", kind, caveat))
		}
		if artifact.Perf != nil {
			add(traceBundlePerfCapabilityCaveat(kind, *artifact.Perf))
			for _, caveat := range artifact.Perf.Caveats {
				add(fmt.Sprintf("tracebundle_artifact %s perf_capability_caveat: %s", kind, caveat))
			}
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
	appendKV("time_alignment", perf.TimeAlignment)
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
	if len(rows) == 0 {
		return nil
	}
	limit := traceBundleCoverageCaveatLimit
	if len(rows) < limit {
		limit = len(rows)
	}
	out := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		out = append(out, traceBundleCoverageCaveat(prefix, rows[i]))
	}
	priorityEmitted := 0
	for i := limit; i < len(rows) && priorityEmitted < traceBundleCoveragePriorityCaveatLimit; i++ {
		if !traceBundleCoveragePriority(rows[i]) {
			continue
		}
		out = append(out, traceBundleCoverageCaveat(prefix, rows[i]))
		priorityEmitted++
	}
	if len(rows) > limit {
		out = append(out, fmt.Sprintf("%s_compacted total=%d emitted=%d priority_emitted=%d", prefix, len(rows), limit, priorityEmitted))
	}
	return out
}

func traceBundleCoveragePriority(coverage traceBundleCoverage) bool {
	return coverage.Family == "resolver.lifecycle" && coverage.Table == "__authority__"
}

func traceBundleCoverageCaveat(prefix string, coverage traceBundleCoverage) string {
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
	appendKV("family", coverage.Family)
	appendKV("table", coverage.Table)
	appendKV("role", coverage.Role)
	parts = append(parts, fmt.Sprintf("found=%t", coverage.Found))
	appendInt("rows_read", coverage.RowsRead)
	appendInt("rows_emitted", coverage.RowsEmitted)
	appendInt("peak_buffered_rows", coverage.PeakBuffered)
	appendInt("spill_chunks", coverage.SpillChunks)
	appendInt64("temp_bytes", coverage.TempBytes)
	appendInt64("elapsed_us", coverage.ElapsedUS)
	appendKV("columns_missing", traceBundleCompactList(coverage.ColumnsMissing, 8))
	appendKV("columns_present", traceBundleCompactList(coverage.ColumnsPresent, 8))
	appendKV("skipped", coverage.Skipped)
	appendKV("error", coverage.Error)
	return strings.Join(parts, " ")
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
	return parseTraceArtifactSpecs(ctx, path, size, modUnix, opts, traceArtifactSpecsForPaths(artifactPaths))
}

func parseTraceArtifactSpecs(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions, artifactSpecs []traceArtifactSpec) (*Index, error) {
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
	for _, spec := range artifactSpecs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		artifactPath := spec.source.SourcePath
		info, err := os.Stat(artifactPath)
		if err != nil {
			return nil, fmt.Errorf("stat trace bundle artifact %s: %w", artifactPath, err)
		}
		source := spec.source
		source.SourceBytes = info.Size()
		source.SourceModUnixNano = info.ModTime().UnixNano()
		source.sourceIdentity = traceFileIdentityFromInfo(info)
		source.VirtualLineBase = virtualLineBase
		reserve, reserveErr := traceArtifactVirtualLineReserve(info.Size(), virtualLineBase)
		if reserveErr != nil {
			return nil, fmt.Errorf("trace bundle artifact %s: %w", artifactPath, reserveErr)
		}
		virtualLineBase += reserve
		idx.Size += info.Size()
		if info.ModTime().After(idx.ModTime) {
			idx.ModTime = info.ModTime()
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
		child, err := parseSingleTraceFile(ctx, artifactPath, info.Size(), info.ModTime().UnixNano(), childOpts)
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
			idx.schedulerRowIntegrityFailuresCapped = true
		}
		for _, childFailure := range childRowIntegrityFailures {
			failure := childFailure
			failure.LocalLine = childFailure.Line
			failure.Line += source.VirtualLineBase
			failure.SourcePath = source.SourcePath
			mapped, ok := source.toCanonicalTsChecked(failure.Ts)
			if !ok {
				return nil, fmt.Errorf("trace bundle artifact %s scheduler incomplete-row timestamp is not safely representable in the canonical clock", artifactPath)
			}
			failure.Ts = mapped
			appendSchedulerRowIntegrityFailure(idx, failure)
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
		idx.Events = append(idx.Events, child.Events...)
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

func resolveTraceBundleArtifactPath(baseDir, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return canonicalTraceIndexPath(p)
	}
	// Bundle-relative is the primary contract. Prefer it over an accidental
	// same-named file in the process CWD; otherwise provenance could point at
	// and parse a different capture than the manifest names. Retain the old
	// CWD-relative form only as a compatibility fallback when the bundle-local
	// target does not exist.
	bundleRelative := filepath.Clean(filepath.Join(baseDir, p))
	if _, err := os.Stat(bundleRelative); err == nil {
		return canonicalTraceIndexPath(bundleRelative)
	}
	if _, err := os.Stat(p); err == nil {
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

// PhysicalFtraceHeaderProbe is a source-neutral observation of the elected
// outer physical ftrace header. HeaderKnown is represented by the boolean return;
// TimestampKnown is separate because malformed timestamp/CPU/PID scalars must
// still stop body text from being reinterpreted as a second top-level header.
// This probe grants no endpoint or pairing authority.
type PhysicalFtraceHeaderProbe struct {
	EventName      string
	TimestampNS    uint64
	TimestampKnown bool
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
	return PhysicalFtraceHeaderProbe{
		EventName: name, TimestampNS: ts, TimestampKnown: timestampKnown,
	}, true
}

// parseLineScan is ParseLine over the shared per-line memo: the header match
// and parseKV computed by the window gate or any physical-row audit are reused
// here instead of being recomputed (perf audit #21).
func parseLineScan(s *lineScan, intern *stringInterner) (Event, bool) {
	lineNo := s.lineNo
	m := s.match()
	if len(m) == 0 {
		return Event{}, false
	}
	pid, pidOK := parseUnsignedTraceInt(m[2])
	if !pidOK {
		// Header PID is an identity, not a magnitude. Overflow/non-decimal input
		// must never collapse to the valid idle identity 0.
		return Event{}, false
	}
	tgid := atoi(m[3])
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
	if schedulerFieldsValidationFailure(lineNo, rawType, ts, cpu, kv) != nil {
		// Critical scheduler identities are presence-sensitive. Returning false
		// keeps an absent/invalid PID from silently materializing as the valid
		// idle PID 0; the physical scan records the typed fail-closed witness.
		return Event{}, false
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
		ev.WakeeComm = intern.intern(schedCommValue(fields, "comm", kv["comm"]))
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
		if ev.WakeePrioUnknown && !ev.WakeePrioInferred {
			ev.WakeePrio = 0
		}
		setEventTargetCPU(&ev, kv["target_cpu"])
	case EventSchedBlockedReason:
		ev.WakeePID = atoi(kv["pid"])
		ev.IOWait = atoi(kv["iowait"])
		ev.Reason = intern.intern(blockedReasonSemanticCaller(kv))
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
		// §7.11 A-1: INT-declared fields tolerate the hmtrace float-string
		// shape ("state=2200000.0") — Atoi fast path, ParseFloat truncation
		// fallback. Applies to cpu_idle/cpu_frequency/limits/clock_set_rate.
		ev.State = atoiFloatTolerant(kv["state"])
		setEventCPUForField(&ev, kv["cpu_id"])
	case EventCPUFrequency:
		freqRaw := firstNonEmpty(kv["state"], kv["frequency"], kv["freq"])
		if freqRaw == "" && rawType == "clock_set_rate" {
			// §7.11 B-2: a cpu-freq-named clock lane reclassified here keeps
			// the keyless positional payload shape.
			freqRaw = clockSetRatePositionalValue(fields)
		}
		ev.Frequency = atoiFloatTolerant(freqRaw)
		setEventCPUForField(&ev, kv["cpu_id"])
		ev.ClockName = intern.intern(clockNameForEvent(rawType, fields))
	case EventCPUFrequencyLimit:
		ev.FrequencyMin = atoiFloatTolerant(firstNonEmpty(kv["min"], kv["min_freq"]))
		ev.FrequencyMax = atoiFloatTolerant(firstNonEmpty(kv["max"], kv["max_freq"]))
		setEventCPUForField(&ev, kv["cpu_id"])
		ev.ClockName = intern.intern(rawType)
	case EventCPUConstraint:
		ev.ConstraintFields = &ConstraintFields{}
		populateCPUConstraintFields(&ev, rawType, kv, intern)
	case EventClockSetRate:
		rateRaw := firstNonEmpty(kv["state"], kv["frequency"], kv["freq"])
		if rateRaw == "" {
			// §7.11 B-2: keyless positional shape "clock_set_rate: <name>
			// <rate>" — precise key-absence gate, keyed shapes unchanged.
			rateRaw = clockSetRatePositionalValue(fields)
		}
		ev.Frequency = atoiFloatTolerant(rateRaw)
		setEventCPUForField(&ev, kv["cpu_id"])
		ev.ClockName = intern.intern(clockNameForEvent(rawType, fields))
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
		ev.BinderFields = &BinderFields{
			TransactionID: strictBinderTransactionID(fields, false),
			DestProc:      atoi(kv["dest_proc"]),
			DestThread:    atoi(kv["dest_thread"]),
			Reply:         atoi(kv["reply"]),
			Flags:         intern.intern(kv["flags"]),
			Code:          intern.intern(kv["code"]),
		}
	case EventBinderReceived:
		ev.BinderFields = &BinderFields{
			TransactionID: strictBinderTransactionID(fields, false),
			DebugID:       atoi(kv["debug_id"]),
		}
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
		populatePerfSampleFields(&ev, kv, intern)
	}
	return ev, true
}

func populatePerfSampleFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil || ev.PerfFields == nil {
		return
	}
	pf := ev.PerfFields
	if perfSampleCPUIsExplicitNoClaim(kv) {
		ev.CPU = -1
	} else if cpu, present, valid, _ := parseTraceCPUScalar(kv["cpu"]); present {
		if valid {
			ev.CPU = cpu
		} else {
			ev.CPU = -1
			ev.CPUInputInvalid = true
		}
	}
	pf.PID = atoi(firstNonEmpty(kv["pid"], kv["process_pid"], kv["tgid"]))
	pf.TID = atoi(firstNonEmpty(kv["tid"], kv["thread_pid"]))
	if pf.PID == 0 && ev.TGID > 0 {
		pf.PID = ev.TGID
	}
	if pf.TID == 0 && ev.PID > 0 {
		pf.TID = ev.PID
	}
	pf.Comm = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["thread_comm"], kv["comm"], kv["name"], ev.Comm), maxPerfSampleTextFieldLen))
	pf.Period = atoi64(firstNonEmpty(kv["sample_weight"], kv["period_weight"], kv["period"], kv["sample_period"], kv["event_count"], kv["count"]))
	pf.EventName = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["event"], kv["type"]), maxPerfSampleTextFieldLen))
	pf.Symbol = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["symbol"], kv["func"], kv["function"]), maxPerfSampleTextFieldLen))
	pf.DSO = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["dso"], kv["file"], kv["path"]), maxPerfSampleTextFieldLen))
	pf.IP = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["ip"], kv["addr"], kv["address"]), maxPerfSampleTextFieldLen))
	pf.Addr = intern.intern(cleanTraceValueBounded(kv["addr"], maxPerfSampleTextFieldLen))
	pf.SampleID = intern.intern(cleanTraceValueBounded(kv["sample_id"], maxPerfSampleTextFieldLen))
	pf.StreamID = intern.intern(cleanTraceValueBounded(kv["stream_id"], maxPerfSampleTextFieldLen))
	pf.RawWeight = atoi64Auto(kv["perf_weight"])
	pf.DataSrc = intern.intern(cleanTraceValueBounded(kv["data_src"], maxPerfSampleTextFieldLen))
	pf.Transaction = intern.intern(cleanTraceValueBounded(kv["transaction"], maxPerfSampleTextFieldLen))
	pf.PhysAddr = intern.intern(cleanTraceValueBounded(kv["phys_addr"], maxPerfSampleTextFieldLen))
	pf.CGroupID = intern.intern(cleanTraceValueBounded(kv["cgroup_id"], maxPerfSampleTextFieldLen))
	pf.DataPageSize = atoi64Auto(kv["data_page_size"])
	pf.CodePageSize = atoi64Auto(kv["code_page_size"])
	pf.RawSize = atoi64Auto(kv["raw_size"])
	pf.BranchCount = atoi64Auto(kv["branch_count"])
	pf.UserRegsABI = intern.intern(cleanTraceValueBounded(kv["user_regs_abi"], maxPerfSampleTextFieldLen))
	pf.UserRegsCount = atoi64Auto(kv["user_regs_count"])
	pf.UserStackSize = atoi64Auto(kv["user_stack_size"])
	pf.AuxSize = atoi64Auto(kv["aux_size"])
	pf.Callchain = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["callchain"], kv["call_stack"], kv["stack"]), maxPerfCallchainFieldLen))
	pf.Source = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["source"], kv["producer"]), maxPerfSampleTextFieldLen))
	if known, ok := perfWireBool(kv["thread_identity_known"]); ok {
		pf.ThreadIdentityKnown = boolPtr(known)
	}
	pf.Resolution = intern.intern(cleanTraceValueBounded(kv["resolution"], maxPerfSampleTextFieldLen))
	if unverified, ok := perfWireBool(kv["lifecycle_unverified"]); ok {
		pf.LifecycleUnverified = boolPtr(unverified)
	}
	if sourcePID, ok := parseUnsignedTraceInt(kv["perf_source_pid"]); ok {
		pf.SourcePID = sourcePID
	}
	if sourceTID, ok := parseUnsignedTraceInt(kv["perf_source_tid"]); ok {
		pf.SourceTID = sourceTID
	}
	pf.SourceComm = intern.intern(cleanTraceValueBounded(kv["perf_source_comm"], maxPerfSampleTextFieldLen))
	pf.SampleKind = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["sample_kind"], kv["sample_type"], kv["perf_sample_kind"]), maxPerfSampleTextFieldLen))
	pf.SampleKindSource = intern.intern(cleanTraceValueBounded(kv["sample_kind_source"], maxPerfSampleTextFieldLen))
	pf.SymbolizationStatus = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["symbolization_status"], kv["symbol_status"], kv["symbols"]), maxPerfSampleTextFieldLen))
	pf.Clock = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["clock"], kv["clockid"]), maxPerfSampleTextFieldLen))
	if known, ok := boolMaybe(firstNonEmpty(kv["cpu_known"], kv["cpu_valid"], kv["cpu_available"])); ok {
		pf.CPUKnown = boolPtr(known)
	}
	if pf.CPUKnown == nil {
		pf.CPUKnown = boolPtr(validTraceCPUIndex(ev.CPU))
	} else if *pf.CPUKnown && !validTraceCPUIndex(ev.CPU) {
		// An explicit truthy cpu_known flag cannot resurrect a malformed CPU
		// identity token into perf attribution.
		pf.CPUKnown = boolPtr(false)
	}
	if pf.SymbolizationStatus == "" {
		pf.SymbolizationStatus = intern.intern(defaultPerfSymbolizationStatus(pf))
	}
	pf.ClockConfidence = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["clock_confidence"], kv["time_alignment"], kv["time_alignment_confidence"]), maxPerfSampleTextFieldLen))
	if pf.ClockConfidence == "" {
		pf.ClockConfidence = intern.intern(defaultPerfClockConfidence(pf))
	}
	pf.CallchainStatus = intern.intern(cleanTraceValueBounded(firstNonEmpty(kv["callchain_status"], kv["stack_status"], kv["call_stack_status"]), maxPerfSampleTextFieldLen))
	if pf.CallchainStatus == "" {
		pf.CallchainStatus = intern.intern(defaultPerfCallchainStatus(pf))
	}
	normalizePerfSampleClaims(ev)
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
	ev.NextInfoLoad = info.load
	ev.NextInfoGroup = info.group
	ev.NextInfoRestricted = info.restricted
	ev.NextInfoExpel = info.expel
	ev.NextInfoCGID = info.cgid
}

type harmonyNextInfoFields struct {
	affinity    string
	allowedCPUs []int
	load        int
	group       int
	restricted  bool
	expel       int
	cgid        int
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
		load:        atoi(parts[1]),
		group:       atoi(parts[2]),
		restricted:  atoi(parts[3]) != 0,
		expel:       atoi(parts[4]),
	}
	if len(parts) >= 6 {
		out.cgid = atoi(parts[5])
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
	case raw == "sched_switch":
		return EventSchedSwitch
	case raw == "sched_wakeup" || raw == "sched_wakeup_new":
		return EventSchedWakeup
	case raw == "sched_waking":
		return EventSchedWaking
	case raw == "sched_blocked_reason":
		return EventSchedBlockedReason
	case strings.HasPrefix(raw, "sched_stat_"):
		return EventSchedStat
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
		// types below) before falling back to Unknown. Plain print prose
		// stays EventUnknown — there is deliberately NO standalone hilog
		// event family (§7.11 B-4 ruling), and the Unknown path keeps its
		// index slot / event_search reachability byte-identical.
		if typ, ok := classifyPrintPluginPayload(comm, rawLower, fields); ok {
			return typ
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
		case strings.Contains(text, "erofs"):
			return "fs_erofs"
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
		strings.HasPrefix(raw, "erofs_") ||
		strings.HasPrefix(raw, "z_erofs_") ||
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

func (i *stringInterner) intern(s string) string {
	if s == "" || i == nil {
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
