package tracequery

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	ftraceLineRE     = regexp.MustCompile(`^\s*(.+)-(\d+)(?:\s+\(\s*([0-9-]+)\))?\s+\[(\d+)\]\s+\S+\s+([0-9]+(?:\.[0-9]+)?):\s+([A-Za-z0-9_./:-]+):?\s*(.*)$`)
	traceTimestampRE = regexp.MustCompile(`\s([0-9]+(?:\.[0-9]+)?):\s+[A-Za-z0-9_./:-]+:?`)
	kvRE             = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*"|'[^']*'|[^ ]+)`)
	blockRequestRE   = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(?:(?:\d+)\s+)?\([^)]*\)\s+(\d+)\s+\+\s+(\d+)`)
	blockSimpleRE    = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+\+\s+(\d+)`)
	blockRemapRE     = regexp.MustCompile(`^(\S+)\s+(\d+)\s+\+\s+(\d+)\s+<-(?:\s+\(([^)]+)\)\s+(\d+))?`)
	blockErrorRE     = regexp.MustCompile(`\[([^\]]+)\]\s*$`)
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

type traceIndexCacheEntry struct {
	key  parseCacheKey
	idx  *Index
	cost int64
}

// traceIndexCache is a byte-budgeted LRU keyed by parseCacheKey. Each entry
// is charged len(Events)*eventSizeBytes. Inserting past the budget evicts
// least-recently-used entries; loads refresh recency. The most recent entry
// is never evicted, so a just-built index always serves at least one repeat
// call even when it alone exceeds the budget.
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
	return int64(len(idx.Events)) * eventSizeBytes
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
}

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
}

func (e *IndexEventLimitError) Error() string {
	if e == nil {
		return "trace index event limit reached"
	}
	return fmt.Sprintf(
		"trace index event limit reached: path=%s parsed_events=%d max_events=%d line=%d scanned_lines=%d windowed=%t index_time=%.6f..%.6f index_lines=%d..%d parsed_time=%.6f..%.6f; selected trace scope is too dense for a single in-memory index; split the time window, add line_start/line_end, or narrow with pid/thread/event_types/event_search before running heavy views",
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
	)
}

func newIndexEventLimitError(path string, idx *Index, opts BuildOptions, line, events int) *IndexEventLimitError {
	err := &IndexEventLimitError{
		Path:      path,
		MaxEvents: opts.MaxEvents,
		Events:    events,
		Line:      line,
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
	opts = normalizeBuildOptions(opts)
	windowKey := opts.cacheKey()
	cacheable := shouldCacheTraceIndex(info.Size(), opts)
	key := parseCacheKey{
		path:      path,
		size:      info.Size(),
		modUnix:   info.ModTime().UnixNano(),
		version:   ParserVersion,
		windowKey: windowKey,
	}
	if cacheable {
		if idx, ok := indexCache.Load(key); ok {
			return idx, nil
		}
	}
	if opts.windowed() {
		fullKey := parseCacheKey{
			path:    path,
			size:    info.Size(),
			modUnix: info.ModTime().UnixNano(),
			version: ParserVersion,
		}
		if idx, ok := indexCache.Load(fullKey); ok {
			return deriveWindowedIndex(idx, opts), nil
		}
	}
	return buildIndexSingleflight(ctx, key, path, info.Size(), info.ModTime().UnixNano(), opts, cacheable)
}

func shouldCacheTraceIndex(size int64, opts BuildOptions) bool {
	if opts.windowed() {
		return false
	}
	return size <= maxCachedTraceIndexBytes
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
		Path:             full.Path,
		Size:             full.Size,
		ModTime:          full.ModTime,
		LineCount:        full.LineCount,
		ScannedLineCount: full.ScannedLineCount,
		Windowed:         true,
		IndexTimeStart:   paddedTimeStart(opts),
		IndexTimeEnd:     paddedTimeEnd(opts),
		IndexLineStart:   paddedLineStart(opts),
		IndexLineEnd:     paddedLineEnd(opts),
		TraceFlavor:      full.TraceFlavor,
		FlavorConfidence: full.FlavorConfidence,
		FlavorSignals:    append([]string(nil), full.FlavorSignals...),
		Caveats:          append([]string(nil), full.Caveats...),
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

func (opts BuildOptions) cacheKey() string {
	if !opts.windowed() {
		return ""
	}
	return fmt.Sprintf("ts=%t:%.9f-%t:%.9f+%.6f/%.6f;ln=%d-%d+%d/%d;max_events=%d",
		opts.TimeStartSet, opts.TimeStart, opts.TimeEndSet, opts.TimeEnd,
		opts.TimePaddingBefore, opts.TimePaddingAfter,
		opts.LineStart, opts.LineEnd, opts.LinePaddingBefore, opts.LinePaddingAfter,
		opts.MaxEvents)
}

func parseFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
	if traceBundlePath(path) {
		return parseTraceBundleFile(ctx, path, size, modUnix, opts)
	}
	if companions := siblingTraceArtifactPaths(path); len(companions) > 0 {
		return parseTraceArtifactPathList(ctx, path, size, modUnix, opts, append([]string{path}, companions...))
	}
	return parseSingleTraceFile(ctx, path, size, modUnix, opts)
}

func parseSingleTraceFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	idx := &Index{Path: path, Size: size, ModTime: time.Unix(0, modUnix)}
	if opts.windowed() {
		idx.Windowed = true
		idx.IndexTimeStart = paddedTimeStart(opts)
		idx.IndexTimeEnd = paddedTimeEnd(opts)
		idx.IndexLineStart = paddedLineStart(opts)
		idx.IndexLineEnd = paddedLineEnd(opts)
	}
	r := bufio.NewReaderSize(f, 256*1024)
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	seenTimeWindow := false
	lastParsedTs := float64(0)
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			if idx.Windowed {
				if idx.IndexLineEnd > 0 && lineNo > idx.IndexLineEnd {
					break
				}
				if idx.IndexLineStart > 0 && lineNo < idx.IndexLineStart {
					if lineNo <= 200 {
						flavor.observeRawLine(trimmed)
					}
					goto nextLine
				}
				if opts.TimeStartSet || opts.TimeEndSet {
					ts, hasTS := parseLineTimestamp(trimmed)
					if hasTS {
						if opts.TimeStartSet && ts < idx.IndexTimeStart {
							if lineNo <= 200 {
								flavor.observeRawLine(trimmed)
							}
							goto nextLine
						}
						if opts.TimeEndSet && ts > idx.IndexTimeEnd {
							break
						}
						seenTimeWindow = true
					} else if opts.TimeStartSet && !seenTimeWindow {
						if lineNo <= 200 {
							flavor.observeRawLine(trimmed)
						}
						goto nextLine
					}
				}
			}
			flavor.observeRawLine(trimmed)
			panicsBefore := idx.ParseLinePanics
			if ev, ok := safeParseLine(lineNo, trimmed, intern, idx); ok {
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
				if opts.MaxEvents > 0 && len(idx.Events) >= opts.MaxEvents {
					return nil, newIndexEventLimitError(path, idx, opts, lineNo, len(idx.Events))
				}
				idx.Events = append(idx.Events, ev)
			} else if trimmed != "" && idx.ParseLinePanics == panicsBefore {
				idx.UnparsedLines++
			}
		}
	nextLine:
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
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
	Confidence      string   `json:"confidence,omitempty"`
	Calibrated      bool     `json:"calibrated"`
	Source          string   `json:"source,omitempty"`
	Caveats         []string `json:"caveats,omitempty"`
}

func traceBundlePath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json") && strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".tracebundle.json")
}

func promoteSiblingTraceBundlePath(path string) string {
	if traceBundlePath(path) {
		return path
	}
	if bundle := siblingTraceBundlePath(path); bundle != "" {
		return bundle
	}
	return path
}

func siblingTraceBundlePath(path string) string {
	base := traceArtifactBase(path)
	if base == "" {
		return ""
	}
	candidate := base + ".tracebundle.json"
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
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
	case strings.HasSuffix(lower, ".perftrace"):
		suffixes = []string{".systrace"}
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
	artifactPaths := traceBundleIndexPaths(path, bundle)
	if len(artifactPaths) == 0 {
		return nil, fmt.Errorf("trace bundle %s has no systrace or perftrace artifacts", path)
	}
	idx, err := parseTraceArtifactPathList(ctx, path, size, modUnix, opts, artifactPaths)
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
	if len(rows) > limit {
		out = append(out, fmt.Sprintf("%s_compacted total=%d emitted=%d", prefix, len(rows), limit))
	}
	return out
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
	idx := &Index{Path: path, Size: size, ModTime: time.Unix(0, modUnix)}
	if opts.windowed() {
		idx.Windowed = true
		idx.IndexTimeStart = paddedTimeStart(opts)
		idx.IndexTimeEnd = paddedTimeEnd(opts)
		idx.IndexLineStart = paddedLineStart(opts)
		idx.IndexLineEnd = paddedLineEnd(opts)
	}
	var flavorSet bool
	for _, artifactPath := range artifactPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Stat(artifactPath)
		if err != nil {
			return nil, fmt.Errorf("stat trace bundle artifact %s: %w", artifactPath, err)
		}
		child, err := parseSingleTraceFile(ctx, artifactPath, info.Size(), info.ModTime().UnixNano(), opts)
		if err != nil {
			return nil, fmt.Errorf("parse trace bundle artifact %s: %w", artifactPath, err)
		}
		idx.Size += child.Size
		if child.ModTime.After(idx.ModTime) {
			idx.ModTime = child.ModTime
		}
		idx.LineCount += child.LineCount
		idx.ScannedLineCount += child.ScannedLineCount
		idx.ParseLinePanics += child.ParseLinePanics
		idx.ClockRegressions += child.ClockRegressions
		idx.UnparsedLines += child.UnparsedLines
		idx.ParsedKnown += child.ParsedKnown
		if child.FirstTs > 0 && (idx.FirstTs == 0 || child.FirstTs < idx.FirstTs) {
			idx.FirstTs = child.FirstTs
		}
		if child.LastTs > idx.LastTs {
			idx.LastTs = child.LastTs
		}
		if !flavorSet || child.FlavorConfidence > idx.FlavorConfidence {
			idx.TraceFlavor = child.TraceFlavor
			idx.FlavorConfidence = child.FlavorConfidence
			idx.FlavorSignals = append([]string(nil), child.FlavorSignals...)
			flavorSet = true
		}
		if opts.MaxEvents > 0 && len(child.Events) > 0 && len(idx.Events)+len(child.Events) > opts.MaxEvents {
			return nil, newIndexEventLimitError(path, idx, opts, child.Events[0].Line, len(idx.Events)+len(child.Events))
		}
		idx.Events = append(idx.Events, child.Events...)
	}
	sort.SliceStable(idx.Events, func(i, j int) bool {
		if idx.Events[i].Ts == idx.Events[j].Ts {
			return idx.Events[i].Line < idx.Events[j].Line
		}
		return idx.Events[i].Ts < idx.Events[j].Ts
	})
	return idx, nil
}

func traceBundleIndexPaths(bundlePath string, bundle traceBundleFile) []string {
	baseDir := filepath.Dir(bundlePath)
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		p = resolveTraceBundleArtifactPath(baseDir, strings.TrimSpace(p))
		if p == "" || seen[p] || traceBundlePath(p) {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	add(bundle.Systrace)
	for _, artifact := range bundle.Artifacts {
		switch strings.ToLower(strings.TrimSpace(artifact.Type)) {
		case "systrace", "perftrace":
			add(artifact.Path)
		}
	}
	return paths
}

func resolveTraceBundleArtifactPath(baseDir, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if _, err := os.Stat(p); err == nil {
		if abs, absErr := filepath.Abs(p); absErr == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(baseDir, p))
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
	m := traceTimestampRE.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0, false
	}
	ts, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return ts, true
}

func ParseLine(lineNo int, line string, intern *stringInterner) (Event, bool) {
	m := ftraceLineRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return Event{}, false
	}
	pid := atoi(m[2])
	tgid := atoi(m[3])
	cpu := atoi(m[4])
	ts, _ := strconv.ParseFloat(m[5], 64)
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	fields := strings.TrimSpace(m[7])
	ev := Event{
		Line:      lineNo,
		Ts:        ts,
		CPU:       cpu,
		Comm:      intern.intern(strings.TrimSpace(m[1])),
		PID:       pid,
		TGID:      tgid,
		Type:      classifyEventType(rawType, fields),
		Name:      intern.intern(rawType),
		FieldText: intern.intern(clampString(fields, 300)),
	}
	ev.SubsystemKind = intern.intern(classifySubsystemKind(rawType, fields, ev.Type))
	kv := parseKV(fields)
	switch ev.Type {
	case EventSchedSwitch:
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
		ev.TargetCPU = atoi(kv["target_cpu"])
	case EventSchedBlockedReason:
		ev.WakeePID = atoi(firstNonEmpty(kv["pid"], kv["caller"]))
		ev.IOWait = atoi(kv["iowait"])
		ev.Reason = intern.intern(firstNonEmpty(kv["caller"], fields))
	case EventSchedStat:
		ev.SchedStatKind = intern.intern(strings.TrimPrefix(strings.ToLower(rawType), "sched_stat_"))
		ev.SchedStatComm = intern.intern(kv["comm"])
		ev.SchedStatPID = atoi(kv["pid"])
		ev.SchedStatDelayNs = atoi64(kv["delay"])
		ev.SchedStatRunNs = atoi64(kv["runtime"])
		ev.SchedStatVRunNs = atoi64(kv["vruntime"])
	case EventCPUIdle:
		ev.State = atoi(kv["state"])
		ev.CPUForField, ev.CPUForFieldValid = atoiMaybe(kv["cpu_id"])
	case EventCPUFrequency:
		ev.Frequency = atoi(firstNonEmpty(kv["state"], kv["frequency"], kv["freq"]))
		ev.CPUForField, ev.CPUForFieldValid = atoiMaybe(kv["cpu_id"])
		ev.ClockName = intern.intern(clockNameForEvent(rawType, fields))
	case EventCPUFrequencyLimit:
		ev.FrequencyMin = atoi(firstNonEmpty(kv["min"], kv["min_freq"]))
		ev.FrequencyMax = atoi(firstNonEmpty(kv["max"], kv["max_freq"]))
		ev.CPUForField, ev.CPUForFieldValid = atoiMaybe(kv["cpu_id"])
		ev.ClockName = intern.intern(rawType)
	case EventCPUConstraint:
		populateCPUConstraintFields(&ev, rawType, kv, intern)
	case EventClockSetRate:
		ev.Frequency = atoi(firstNonEmpty(kv["state"], kv["frequency"], kv["freq"]))
		ev.CPUForField, ev.CPUForFieldValid = atoiMaybe(kv["cpu_id"])
		ev.ClockName = intern.intern(clockNameForEvent(rawType, fields))
	case EventTraceMark:
		ev.SpanAction, ev.SpanPID, ev.SpanName, ev.SpanValue = parseTraceMark(fields)
		ev.SpanAction = intern.intern(ev.SpanAction)
		ev.SpanName = intern.intern(ev.SpanName)
		ev.SpanValue = intern.intern(ev.SpanValue)
	case EventBlockIssue, EventBlockComplete:
		dev, op, sector, length := parseBlockRequest(fields)
		ev.BlockDev = intern.intern(dev)
		ev.BlockOp = intern.intern(op)
		ev.BlockSector = sector
		ev.BlockLen = length
		if ev.Type == EventBlockComplete {
			ev.BlockError = intern.intern(parseBlockError(fields))
		}
	case EventBlockRemap:
		dev, sector, length, srcDev, srcSector := parseBlockRemap(fields)
		ev.BlockDev = intern.intern(dev)
		ev.BlockSector = sector
		ev.BlockLen = length
		ev.BlockSrcDev = intern.intern(srcDev)
		ev.BlockSrcSector = srcSector
	case EventBinderTransaction:
		ev.BinderTransactionID = atoi(kv["transaction"])
		ev.BinderDestProc = atoi(kv["dest_proc"])
		ev.BinderDestThread = atoi(kv["dest_thread"])
		ev.BinderReply = atoi(kv["reply"])
		ev.BinderFlags = intern.intern(kv["flags"])
		ev.BinderCode = intern.intern(kv["code"])
	case EventBinderReceived:
		ev.BinderTransactionID = atoi(firstNonEmpty(kv["transaction"], kv["debug_id"]))
		ev.BinderDebugID = atoi(kv["debug_id"])
	case EventBinderAllocBuf:
		ev.BinderTransactionID = atoi(firstNonEmpty(kv["transaction"], kv["debug_id"]))
		ev.BinderDebugID = atoi(kv["debug_id"])
		ev.BinderDataSize = atoi64(kv["data_size"])
		ev.BinderOffsetsSize = atoi64(kv["offsets_size"])
		ev.BinderExtraSize = atoi64(firstNonEmpty(kv["extra_buffers_size"], kv["extra_size"]))
	case EventBinderLock, EventBinderLocked, EventBinderUnlock, EventBinderReply:
		ev.BinderTransactionID = atoi(firstNonEmpty(kv["transaction"], kv["debug_id"]))
		ev.BinderDebugID = atoi(kv["debug_id"])
		ev.BinderLockTag = intern.intern(firstNonEmpty(kv["tag"], kv["lock"], kv["name"], fields))
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
		populateResourceFields(&ev, kv, intern)
		populateFileIOFields(&ev, kv, intern)
	case EventStorage, EventFilesystem:
		populateResourceFields(&ev, kv, intern)
		populateFileIOFields(&ev, kv, intern)
	case EventAbilityMonitor, EventXPower, EventHiSystemEvent:
		populatePluginFields(&ev, rawType, kv, intern)
	case EventPerfSample:
		populatePerfSampleFields(&ev, kv, intern)
	}
	return ev, true
}

func populatePerfSampleFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil {
		return
	}
	if cpu, ok := atoiMaybe(kv["cpu"]); ok {
		ev.CPU = cpu
	}
	ev.PerfPID = atoi(firstNonEmpty(kv["pid"], kv["process_pid"], kv["tgid"]))
	ev.PerfTID = atoi(firstNonEmpty(kv["tid"], kv["thread_pid"]))
	if ev.PerfPID == 0 && ev.TGID > 0 {
		ev.PerfPID = ev.TGID
	}
	if ev.PerfTID == 0 && ev.PID > 0 {
		ev.PerfTID = ev.PID
	}
	ev.PerfComm = intern.intern(cleanTraceValue(firstNonEmpty(kv["thread_comm"], kv["comm"], kv["name"], ev.Comm)))
	ev.PerfPeriod = atoi64(firstNonEmpty(kv["sample_weight"], kv["period_weight"], kv["period"], kv["sample_period"], kv["event_count"], kv["count"]))
	ev.PerfEvent = intern.intern(cleanTraceValue(firstNonEmpty(kv["event"], kv["type"])))
	ev.PerfSymbol = intern.intern(cleanTraceValue(firstNonEmpty(kv["symbol"], kv["func"], kv["function"])))
	ev.PerfDSO = intern.intern(cleanTraceValue(firstNonEmpty(kv["dso"], kv["file"], kv["path"])))
	ev.PerfIP = intern.intern(cleanTraceValue(firstNonEmpty(kv["ip"], kv["addr"], kv["address"])))
	ev.PerfAddr = intern.intern(cleanTraceValue(kv["addr"]))
	ev.PerfSampleID = intern.intern(cleanTraceValue(kv["sample_id"]))
	ev.PerfStreamID = intern.intern(cleanTraceValue(kv["stream_id"]))
	ev.PerfRawWeight = atoi64Auto(kv["perf_weight"])
	ev.PerfDataSrc = intern.intern(cleanTraceValue(kv["data_src"]))
	ev.PerfTransaction = intern.intern(cleanTraceValue(kv["transaction"]))
	ev.PerfPhysAddr = intern.intern(cleanTraceValue(kv["phys_addr"]))
	ev.PerfCGroupID = intern.intern(cleanTraceValue(kv["cgroup_id"]))
	ev.PerfDataPageSize = atoi64Auto(kv["data_page_size"])
	ev.PerfCodePageSize = atoi64Auto(kv["code_page_size"])
	ev.PerfRawSize = atoi64Auto(kv["raw_size"])
	ev.PerfBranchCount = atoi64Auto(kv["branch_count"])
	ev.PerfUserRegsABI = intern.intern(cleanTraceValue(kv["user_regs_abi"]))
	ev.PerfUserRegsCount = atoi64Auto(kv["user_regs_count"])
	ev.PerfUserStackSize = atoi64Auto(kv["user_stack_size"])
	ev.PerfAuxSize = atoi64Auto(kv["aux_size"])
	ev.PerfCallchain = intern.intern(cleanTraceValue(firstNonEmpty(kv["callchain"], kv["call_stack"], kv["stack"])))
	ev.PerfSource = intern.intern(cleanTraceValue(firstNonEmpty(kv["source"], kv["producer"])))
	ev.PerfSampleKind = intern.intern(cleanTraceValue(firstNonEmpty(kv["sample_kind"], kv["sample_type"], kv["perf_sample_kind"])))
	ev.PerfSymbolizationStatus = intern.intern(cleanTraceValue(firstNonEmpty(kv["symbolization_status"], kv["symbol_status"], kv["symbols"])))
	ev.PerfClock = intern.intern(cleanTraceValue(firstNonEmpty(kv["clock"], kv["clockid"])))
	if known, ok := boolMaybe(firstNonEmpty(kv["cpu_known"], kv["cpu_valid"], kv["cpu_available"])); ok {
		ev.PerfCPUKnown = boolPtr(known)
	}
	if ev.PerfCPUKnown == nil {
		ev.PerfCPUKnown = boolPtr(ev.CPU >= 0)
	}
	if ev.PerfSymbolizationStatus == "" {
		ev.PerfSymbolizationStatus = intern.intern(defaultPerfSymbolizationStatus(*ev))
	}
	ev.PerfClockConfidence = intern.intern(cleanTraceValue(firstNonEmpty(kv["clock_confidence"], kv["time_alignment"], kv["time_alignment_confidence"])))
	if ev.PerfClockConfidence == "" {
		ev.PerfClockConfidence = intern.intern(defaultPerfClockConfidence(*ev))
	}
	ev.PerfCallchainStatus = intern.intern(cleanTraceValue(firstNonEmpty(kv["callchain_status"], kv["stack_status"], kv["call_stack_status"])))
	if ev.PerfCallchainStatus == "" {
		ev.PerfCallchainStatus = intern.intern(defaultPerfCallchainStatus(*ev))
	}
}

func defaultPerfSymbolizationStatus(ev Event) string {
	source := strings.ToLower(strings.TrimSpace(ev.PerfSource))
	switch {
	case strings.Contains(source, "raw_perfdata"):
		return "unsymbolized"
	case ev.PerfSymbol != "" && !perfLabelLooksLikeIP(ev.PerfSymbol):
		return "symbolized"
	case ev.PerfDSO != "" || ev.PerfIP != "":
		return "partial"
	default:
		return "unknown"
	}
}

func defaultPerfClockConfidence(ev Event) string {
	if strings.TrimSpace(ev.PerfClock) == "" {
		return "unknown"
	}
	return "assumed"
}

func defaultPerfCallchainStatus(ev Event) string {
	callchain := strings.TrimSpace(ev.PerfCallchain)
	source := strings.ToLower(strings.TrimSpace(ev.PerfSource))
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
	if ev == nil {
		return
	}
	ev.ConstraintKind = intern.intern(strings.TrimSpace(rawType))
	ev.ConstraintComm = intern.intern(cleanTraceValue(firstNonEmpty(kv["target_comm"], kv["comm"], kv["task"], kv["name"])))
	ev.ConstraintPID = atoi(firstNonEmpty(kv["target_pid"], kv["task_pid"], kv["pid"], kv["tid"]))
	ev.ConstraintPolicy = intern.intern(firstNonEmpty(kv["policy"], kv["reason"], kv["type"]))
	ev.CPUSet = intern.intern(cleanTraceValue(firstNonEmpty(kv["cpuset"], kv["cgroup"], kv["cg"], kv["path"])))
	if ev.CPUSet != "" && ev.CGroup == "" {
		ev.CGroup = ev.CPUSet
	}
	if cpu, ok := atoiMaybe(firstNonEmpty(kv["cpu"], kv["target_cpu"])); ok {
		ev.ConstraintCPU = cpu
		ev.ConstraintCPUValid = true
	}
	if cpu, ok := atoiMaybe(kv["orig_cpu"]); ok {
		ev.ConstraintOrigCPU = cpu
		ev.ConstraintOrigCPUSet = true
	}
	if cpu, ok := atoiMaybe(kv["dest_cpu"]); ok {
		ev.ConstraintDestCPU = cpu
		ev.ConstraintDestCPUSet = true
		ev.TargetCPU = cpu
	}
	allowedText := cleanTraceValue(firstNonEmpty(kv["allowed_cpus"], kv["cpus_allowed"], kv["cpumask"], kv["cpus"], kv["affinity"], kv["mask"]))
	ev.AllowedCPUsText = intern.intern(allowedText)
	ev.AllowedCPUs = parseCPUSetList(allowedText)
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
	raw = strings.TrimSpace(strings.Trim(raw, "{}[]()"))
	if raw == "" {
		return nil
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "0x") || containsHexAlpha(lower) {
		return parseCPUMaskHex(raw)
	}
	return uniqueSortedInts(parseCPURangeList(raw))
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

func populateResourceFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil {
		return
	}
	ev.ResourcePath = intern.intern(cleanTraceValue(firstNonEmpty(kv["path"], kv["file"], kv["filename"], kv["entry_name"], kv["name"])))
	ev.ResourceOp = intern.intern(firstNonEmpty(kv["op"], kv["operation"], kv["syscall"], kv["type"], kv["rw"], kv["rwbs"]))
	ev.ResourceLatencyMs = parseLatencyMs(kv)
	ev.ResourceBytes = atoi64(firstNonEmpty(kv["bytes"], kv["size"], kv["len"], kv["length"]))
	ev.ResourceAddress = intern.intern(firstNonEmpty(kv["addr"], kv["address"], kv["fault_addr"]))
	ev.ResourceCallstack = intern.intern(clampString(firstNonEmpty(kv["callstack"], kv["backtrace"], kv["stack"]), 160))
}

func populateFileIOFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil {
		return
	}
	ev.FSDev = intern.intern(firstNonEmpty(kv["fs_dev"], kv["dev"]))
	ev.Inode = intern.intern(cleanTraceValue(firstNonEmpty(kv["ino"], kv["inode"])))
	ev.ParentInode = intern.intern(cleanTraceValue(firstNonEmpty(kv["pino"], kv["parent_ino"], kv["parent_inode"], kv["parent"])))
	ev.EntryName = intern.intern(cleanTraceValue(firstNonEmpty(kv["entry_name"], kv["path"], kv["file"], kv["filename"], kv["name"])))
	ev.FileOffset = atoi64Auto(firstNonEmpty(kv["offset"], kv["ofs"], kv["pos"]))
	ev.FileLen = atoi64Auto(firstNonEmpty(kv["bytes"], kv["len"], kv["length"]))
	ev.FileRW = intern.intern(normalizeFileRW(firstNonEmpty(kv["rw"], kv["rwbs"], kv["op"], kv["operation"], kv["type"], fileOperationFromEventName(ev.Name))))
	ev.FileRet = atoi64Auto(kv["ret"])
	ev.FileSize = atoi64Auto(firstNonEmpty(kv["i_size"], kv["file_size"]))
	if ev.ResourcePath == "" && ev.EntryName != "" {
		ev.ResourcePath = ev.EntryName
	}
	if ev.ResourceOp == "" && ev.FileRW != "" {
		ev.ResourceOp = ev.FileRW
	}
	if ev.ResourceBytes == 0 && ev.FileLen > 0 {
		ev.ResourceBytes = ev.FileLen
	}
}

func populatePluginFields(ev *Event, rawType string, kv map[string]string, intern *stringInterner) {
	if ev == nil {
		return
	}
	ev.PluginDomain = intern.intern(firstNonEmpty(kv["domain"], kv["module"], kv["bundle"], kv["process"], kv["package"]))
	ev.PluginEventName = intern.intern(firstNonEmpty(kv["event_name"], kv["eventname"], kv["event"], kv["name"], rawType))
	ev.PluginMetric = intern.intern(firstNonEmpty(kv["metric"], kv["key"], kv["item"], kv["counter"], kv["component"], kv["type"]))
	ev.PluginValue = intern.intern(firstNonEmpty(kv["value"], kv["val"], kv["state"], kv["usage"], kv["energy"], kv["count"], kv["duration_ms"], kv["latency_ms"]))
	ev.PluginCategory = intern.intern(firstNonEmpty(kv["category"], kv["level"], kv["tag"], kv["scene"]))
}

func parseBlockRequest(fields string) (dev, op string, sector, length int64) {
	trimmed := strings.TrimSpace(fields)
	m := blockRequestRE.FindStringSubmatch(trimmed)
	if len(m) != 5 {
		m = blockSimpleRE.FindStringSubmatch(trimmed)
	}
	if len(m) != 5 {
		return "", "", 0, 0
	}
	return m[1], m[2], atoi64(m[3]), atoi64(m[4])
}

func parseBlockRemap(fields string) (dev string, sector, length int64, srcDev string, srcSector int64) {
	m := blockRemapRE.FindStringSubmatch(strings.TrimSpace(fields))
	if len(m) != 6 {
		return "", 0, 0, "", 0
	}
	return m[1], atoi64(m[2]), atoi64(m[3]), strings.TrimSpace(m[4]), atoi64(m[5])
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

func classifyEventType(raw, fields string) EventType {
	raw = strings.TrimSpace(raw)
	rawLower := strings.ToLower(raw)
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
	case raw == "block_rq_issue" || raw == "block_rq_insert" || raw == "block_getrq" || raw == "block_bio_queue":
		return EventBlockIssue
	case raw == "block_bio_remap":
		return EventBlockRemap
	case raw == "block_rq_complete" || raw == "block_bio_complete":
		return EventBlockComplete
	case raw == "binder_transaction":
		return EventBinderTransaction
	case raw == "binder_transaction_received":
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
	case raw == "print" || raw == "tracing_mark_write" || raw == "tracing_mark_write_xacct" || raw == "xacct_tracing_mark_write":
		if isTraceMarkPayload(fields) {
			return EventTraceMark
		}
		return EventUnknown
	case isStorageEvent(rawLower):
		return EventStorage
	case isFilesystemEvent(rawLower):
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
	return strings.HasPrefix(raw, "ext4_") ||
		strings.HasPrefix(raw, "f2fs_") ||
		strings.HasPrefix(raw, "android_fs_") ||
		strings.HasPrefix(raw, "erofs_") ||
		strings.HasPrefix(raw, "z_erofs_") ||
		strings.HasPrefix(raw, "filesystem") ||
		strings.HasPrefix(raw, "file_system") ||
		strings.HasPrefix(raw, "ebpf_file") ||
		strings.HasPrefix(raw, "file_check_and_advance_wb_err") ||
		strings.HasPrefix(raw, "filemap_set_wb_err")
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
	name := strings.ToLower(clockNameForEvent("clock_set_rate", fields))
	switch name {
	case "pid_freq", "cpu_freq", "cpu_frequency", "cpufreq", "scaling_cur_freq":
		return true
	default:
		return strings.Contains(name, "cpu") && strings.Contains(name, "freq") && !strings.Contains(name, "ddr")
	}
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
	default:
		return false
	}
}

func parseTraceMark(fields string) (action string, spanPID int, name, value string) {
	fields = normalizeTraceMarkPayload(fields)
	if fields == "" {
		return "", 0, "", ""
	}
	parts := strings.Split(fields, "|")
	action = strings.TrimSpace(parts[0])
	if len(parts) >= 2 {
		spanPID = atoi(parts[1])
	}
	if len(parts) >= 3 {
		name = strings.TrimSpace(parts[2])
	}
	if len(parts) >= 4 {
		value = strings.TrimSpace(parts[3])
	}
	switch action {
	case "B":
		if value == "" && len(parts) > 3 {
			value = strings.TrimSpace(parts[3])
		}
	case "E":
		// Synchronous atrace/ftrace spans close with E|pid or bare E; the
		// closing row intentionally does not repeat the begin span name.
		if name == "" {
			name = fields
		}
	case "C":
		// Counter value is the first payload after the counter name.
	case "S", "F":
		// Async spans use name+cookie; extra payload after the cookie remains
		// in FieldText/Raw for literal event_search matching.
	default:
		if name == "" {
			name = fields
		}
	}
	return action, spanPID, name, value
}

func normalizeTraceMarkPayload(fields string) string {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return ""
	}
	if isDirectTraceMarkPayload(fields) {
		return fields
	}
	if idx := strings.Index(fields, ":"); idx > 0 && idx+1 < len(fields) {
		prefix := strings.TrimSpace(fields[:idx])
		payload := strings.TrimSpace(fields[idx+1:])
		if tracePrintPrefixLooksLikeAddress(prefix) && isDirectTraceMarkPayload(payload) {
			return payload
		}
	}
	return fields
}

func isDirectTraceMarkPayload(fields string) bool {
	fields = strings.TrimSpace(fields)
	if fields == "E" {
		return true
	}
	return strings.HasPrefix(fields, "B|") ||
		strings.HasPrefix(fields, "E|") ||
		strings.HasPrefix(fields, "C|") ||
		strings.HasPrefix(fields, "S|") ||
		strings.HasPrefix(fields, "F|")
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

func parseLatencyMs(kv map[string]string) float64 {
	if len(kv) == 0 {
		return 0
	}
	for _, key := range []string{"latency_ms", "duration_ms", "dur_ms"} {
		if v := parseFloat(kv[key]); v > 0 {
			return v
		}
	}
	for _, key := range []string{"latency_us", "duration_us", "dur_us"} {
		if v := parseFloat(kv[key]); v > 0 {
			return v / 1000
		}
	}
	for _, key := range []string{"latency_ns", "duration_ns", "dur_ns"} {
		if v := parseFloat(kv[key]); v > 0 {
			return v / 1000000
		}
	}
	for _, key := range []string{"latency", "duration", "dur"} {
		if v := parseFloat(kv[key]); v > 0 {
			return v
		}
	}
	return 0
}

func parseFloat(raw string) float64 {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(raw, 64)
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

func clampString(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func cleanTraceValue(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimRight(raw, ",")
	return raw
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
	case strings.Contains(name, "filemap_add_to_page_cache"):
		return "page_cache_add"
	case strings.Contains(name, "filemap_delete_from_page_cache"):
		return "page_cache_delete"
	default:
		return ""
	}
}

type stringInterner struct {
	values map[string]string
}

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
	i.values[s] = s
	return s
}

// safeParseLine isolates per-line parse panics: trace artifacts are
// untrusted input, and a single pathological line must degrade to a
// typed counter instead of killing the whole query. The recover is
// function-scoped so the hot loop pays only the call overhead.
func safeParseLine(lineNo int, line string, intern *stringInterner, idx *Index) (ev Event, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			idx.ParseLinePanics++
			ev, ok = Event{}, false
		}
	}()
	return parseLineFn(lineNo, line, intern)
}

// parseLineFn indirects ParseLine so the recover seam is testable with
// an injected panic; production always points at ParseLine.
var parseLineFn = ParseLine
