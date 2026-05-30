package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ftraceLineRE = regexp.MustCompile(`^\s*(.+)-(\d+)\s+\(\s*(\d+)\)\s+\[(\d+)\]\s+\S+\s+([0-9]+(?:\.[0-9]+)?):\s+([A-Za-z0-9_./:-]+):?\s*(.*)$`)
	kvRE         = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=([^ ]+)`)
)

type parseCacheKey struct {
	path    string
	size    int64
	modUnix int64
	version string
}

var indexCache sync.Map

func BuildIndex(ctx context.Context, path string) (*Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trace path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := parseCacheKey{
		path:    path,
		size:    info.Size(),
		modUnix: info.ModTime().UnixNano(),
		version: ParserVersion,
	}
	if cached, ok := indexCache.Load(key); ok {
		if idx, ok := cached.(*Index); ok {
			return idx, nil
		}
	}
	idx, err := parseFile(ctx, path, info.Size(), info.ModTime().UnixNano())
	if err != nil {
		return nil, err
	}
	indexCache.Store(key, idx)
	return idx, nil
}

func parseFile(ctx context.Context, path string, size int64, modUnix int64) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	idx := &Index{Path: path, Size: size, ModTime: time.Unix(0, modUnix)}
	r := bufio.NewReaderSize(f, 256*1024)
	intern := newStringInterner()
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			if ev, ok := ParseLine(lineNo, strings.TrimRight(line, "\r\n"), intern); ok {
				if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
					idx.FirstTs = ev.Ts
				}
				if ev.Ts > idx.LastTs {
					idx.LastTs = ev.Ts
				}
				if ev.Type != EventUnknown {
					idx.ParsedKnown++
				}
				idx.Events = append(idx.Events, ev)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return idx, nil
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
		FieldText: intern.intern(clampString(fields, 300)),
	}
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
	case EventSchedWakeup, EventSchedWaking:
		ev.WakeeComm = intern.intern(kv["comm"])
		ev.WakeePID = atoi(kv["pid"])
		ev.WakeePrio = atoi(kv["prio"])
		ev.TargetCPU = atoi(kv["target_cpu"])
	case EventSchedBlockedReason:
		ev.WakeePID = atoi(firstNonEmpty(kv["pid"], kv["caller"]))
		ev.Reason = intern.intern(firstNonEmpty(kv["caller"], kv["io_wait"], fields))
	case EventCPUIdle:
		ev.State = atoi(kv["state"])
		ev.CPUForField = atoi(kv["cpu_id"])
	case EventCPUFrequency:
		ev.Frequency = atoi(firstNonEmpty(kv["state"], kv["frequency"], kv["freq"]))
		ev.CPUForField = atoi(kv["cpu_id"])
	case EventTraceMark:
		ev.SpanAction, ev.SpanName = parseTraceMark(fields)
		ev.SpanAction = intern.intern(ev.SpanAction)
		ev.SpanName = intern.intern(ev.SpanName)
	}
	if ev.Type == EventUnknown {
		ev.FieldText = ""
	}
	return ev, true
}

func classifyEventType(raw, fields string) EventType {
	raw = strings.TrimSpace(raw)
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
	case raw == "clock_set_rate" || raw == "cpu_frequency" || strings.Contains(raw, "freq"):
		return EventCPUFrequency
	case raw == "block_rq_issue":
		return EventBlockIssue
	case raw == "block_rq_complete":
		return EventBlockComplete
	case raw == "binder_transaction":
		return EventBinderTransaction
	case strings.HasPrefix(raw, "irq_") || strings.Contains(raw, "softirq"):
		return EventIRQ
	case raw == "print" || raw == "tracing_mark_write":
		if strings.HasPrefix(fields, "B|") || strings.HasPrefix(fields, "E|") || strings.HasPrefix(fields, "C|") {
			return EventTraceMark
		}
		return EventUnknown
	case strings.HasPrefix(raw, "mm_") || strings.Contains(raw, "reclaim") || strings.Contains(raw, "fault") || strings.Contains(fields, "GC"):
		return EventMemory
	default:
		return EventUnknown
	}
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

func parseTraceMark(fields string) (action, name string) {
	parts := strings.Split(fields, "|")
	if len(parts) >= 3 {
		return parts[0], parts[2]
	}
	if len(parts) >= 1 {
		return parts[0], fields
	}
	return "", ""
}

func atoi(raw string) int {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
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
