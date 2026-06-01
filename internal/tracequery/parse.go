package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ftraceLineRE     = regexp.MustCompile(`^\s*(.+)-(\d+)(?:\s+\(\s*([0-9-]+)\))?\s+\[(\d+)\]\s+\S+\s+([0-9]+(?:\.[0-9]+)?):\s+([A-Za-z0-9_./:-]+):?\s*(.*)$`)
	traceTimestampRE = regexp.MustCompile(`\s([0-9]+(?:\.[0-9]+)?):\s+[A-Za-z0-9_./:-]+:?`)
	kvRE             = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=([^ ]+)`)
	blockRequestRE   = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(?:(?:\d+)\s+)?\([^)]*\)\s+(\d+)\s+\+\s+(\d+)`)
	blockRemapRE     = regexp.MustCompile(`^(\S+)\s+(\d+)\s+\+\s+(\d+)\s+<-`)
	blockErrorRE     = regexp.MustCompile(`\[([^\]]+)\]\s*$`)
)

type parseCacheKey struct {
	path      string
	size      int64
	modUnix   int64
	version   string
	windowKey string
}

var indexCache sync.Map

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
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	opts = normalizeBuildOptions(opts)
	windowKey := opts.cacheKey()
	key := parseCacheKey{
		path:      path,
		size:      info.Size(),
		modUnix:   info.ModTime().UnixNano(),
		version:   ParserVersion,
		windowKey: windowKey,
	}
	if cached, ok := indexCache.Load(key); ok {
		if idx, ok := cached.(*Index); ok {
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
		if cached, ok := indexCache.Load(fullKey); ok {
			if idx, ok := cached.(*Index); ok {
				windowed := deriveWindowedIndex(idx, opts)
				indexCache.Store(key, windowed)
				return windowed, nil
			}
		}
	}
	return buildIndexSingleflight(ctx, key, path, info.Size(), info.ModTime().UnixNano(), opts)
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

func buildIndexSingleflight(ctx context.Context, key parseCacheKey, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
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
	if call.err == nil {
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
	}
	firstLine, lastLine := 0, 0
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
	if !opts.AllowWindowedParse {
		return BuildOptions{}
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
	return fmt.Sprintf("ts=%t:%.9f-%t:%.9f+%.6f/%.6f;ln=%d-%d+%d/%d",
		opts.TimeStartSet, opts.TimeStart, opts.TimeEndSet, opts.TimeEnd,
		opts.TimePaddingBefore, opts.TimePaddingAfter,
		opts.LineStart, opts.LineEnd, opts.LinePaddingBefore, opts.LinePaddingAfter)
}

func parseFile(ctx context.Context, path string, size int64, modUnix int64, opts BuildOptions) (*Index, error) {
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
			if ev, ok := ParseLine(lineNo, trimmed, intern); ok {
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
				idx.Events = append(idx.Events, ev)
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
	case EventClockSetRate:
		ev.Frequency = atoi(firstNonEmpty(kv["state"], kv["frequency"], kv["freq"]))
		ev.CPUForField, ev.CPUForFieldValid = atoiMaybe(kv["cpu_id"])
		ev.ClockName = intern.intern(clockNameForEvent(rawType, fields))
	case EventTraceMark:
		ev.SpanAction, ev.SpanName, ev.SpanValue = parseTraceMark(fields)
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
		dev, sector, length := parseBlockRemap(fields)
		ev.BlockDev = intern.intern(dev)
		ev.BlockSector = sector
		ev.BlockLen = length
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
	case EventMemory:
		ev.MemoryKind = intern.intern(classifyMemoryKind(rawType, fields))
		if ev.SubsystemKind == "" {
			ev.SubsystemKind = ev.MemoryKind
		}
		populateResourceFields(&ev, kv, intern)
	case EventStorage, EventFilesystem:
		populateResourceFields(&ev, kv, intern)
	case EventAbilityMonitor, EventXPower, EventHiSystemEvent:
		populatePluginFields(&ev, rawType, kv, intern)
	}
	return ev, true
}

func populateResourceFields(ev *Event, kv map[string]string, intern *stringInterner) {
	if ev == nil {
		return
	}
	ev.ResourcePath = intern.intern(firstNonEmpty(kv["path"], kv["file"], kv["filename"], kv["name"]))
	ev.ResourceOp = intern.intern(firstNonEmpty(kv["op"], kv["operation"], kv["syscall"], kv["type"], kv["rwbs"]))
	ev.ResourceLatencyMs = parseLatencyMs(kv)
	ev.ResourceBytes = atoi64(firstNonEmpty(kv["bytes"], kv["size"], kv["len"], kv["length"]))
	ev.ResourceAddress = intern.intern(firstNonEmpty(kv["addr"], kv["address"], kv["fault_addr"]))
	ev.ResourceCallstack = intern.intern(clampString(firstNonEmpty(kv["callstack"], kv["backtrace"], kv["stack"]), 160))
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
	m := blockRequestRE.FindStringSubmatch(strings.TrimSpace(fields))
	if len(m) != 5 {
		return "", "", 0, 0
	}
	return m[1], m[2], atoi64(m[3]), atoi64(m[4])
}

func parseBlockRemap(fields string) (dev string, sector, length int64) {
	m := blockRemapRE.FindStringSubmatch(strings.TrimSpace(fields))
	if len(m) != 4 {
		return "", 0, 0
	}
	return m[1], atoi64(m[2]), atoi64(m[3])
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
	case raw == "sched_wakeup":
		return EventSchedWakeup
	case raw == "sched_waking":
		return EventSchedWaking
	case raw == "sched_blocked_reason":
		return EventSchedBlockedReason
	case raw == "cpu_idle":
		return EventCPUIdle
	case raw == "cpu_frequency":
		return EventCPUFrequency
	case raw == "cpu_frequency_limits":
		return EventCPUFrequencyLimit
	case raw == "clock_set_rate":
		if isCPUFrequencyClock(fields) {
			return EventCPUFrequency
		}
		return EventClockSetRate
	case strings.Contains(rawLower, "cpu") && strings.Contains(rawLower, "freq") && strings.Contains(rawLower, "limit"):
		return EventCPUFrequencyLimit
	case strings.Contains(rawLower, "cpu") && strings.Contains(rawLower, "freq"):
		return EventCPUFrequency
	case raw == "block_rq_issue":
		return EventBlockIssue
	case raw == "block_bio_remap":
		return EventBlockRemap
	case raw == "block_rq_complete":
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
	case strings.HasPrefix(raw, "irq_"):
		return EventIRQ
	case raw == "print" || raw == "tracing_mark_write":
		if strings.HasPrefix(fields, "B|") || strings.HasPrefix(fields, "E|") || strings.HasPrefix(fields, "C|") {
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
		strings.HasPrefix(raw, "i2c_") ||
		strings.HasPrefix(raw, "smbus_") ||
		(strings.Contains(raw, "bio") && strings.Contains(raw, "latency")) ||
		strings.HasPrefix(raw, "bio_") ||
		strings.HasPrefix(raw, "ebpf_bio")
}

func isFilesystemEvent(raw string) bool {
	return strings.HasPrefix(raw, "ext4_") ||
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
			out[m[1]] = m[2]
		}
	}
	return out
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

func parseTraceMark(fields string) (action, name, value string) {
	parts := strings.Split(fields, "|")
	if len(parts) >= 4 && parts[0] == "C" {
		return parts[0], parts[2], parts[3]
	}
	if len(parts) >= 3 {
		return parts[0], parts[2], ""
	}
	if len(parts) >= 1 {
		return parts[0], fields, ""
	}
	return "", "", ""
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
