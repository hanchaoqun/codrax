package tracediag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Format census (§28.12 补充裁定, 2026-07-09): proactively collect the trace's
// FORMAT observation faces so later engine extensions never start from a
// blind spot (the hmfs_ silent-miss / cluster-track discoveries motivated
// this step class). Everything here is computed from the engine's EXPORTED
// streaming face (tracequery.StreamScan events + the shell's parse-quality
// counters and typed UnparsedSamples — TDIAG B2/B4, §28.13, 2026-07-09;
// formerly the Index face, which put whole-file censuses under the 250K
// index event budget) — classification is the engine's own (Event.Type from
// classifyEventType, TraceSpanSemanticClass/TraceSpanNearMissesSemanticWork
// for face ⑦), never a reimplemented parser (anti-parallel-subsystem red
// line). Output is aggregate statistics plus bounded samples only: every
// list carries a top-N cap and a total disclosure, same honesty family as
// the report line caps.
const (
	censusEventNameCap   = 40
	censusUnknownNameCap = 30
	censusClockTrackCap  = 30
	censusPrefixCap      = 30
	censusPrioCap        = 20
	censusPrevStateCap   = 20
	censusSpanNameCap    = 20
	// censusUnparsedSamples mirrors the engine's typed sample cap (B4: the
	// parse site retains tracequery.IndexUnparsedSampleCap samples; the
	// census renders exactly that face).
	censusUnparsedSamples = tracequery.IndexUnparsedSampleCap
)

type nameCount struct {
	Name  string
	Type  string
	Count int
}

type clockTrack struct {
	Name     string
	Count    int
	CPUs     map[int]bool
	MinState int
	MaxState int
}

// censusScope bounds one census step to the step's own window / line range.
// The index alone cannot carry this: a small trace always gets a FULL index
// (buildStepIndex), and a windowed large-trace index still includes padding
// events — the census loop filters precisely so the reported counts match
// the step parameters the report echoes.
type censusScope struct {
	timeStart, timeEnd float64
	timeSet            bool
	lineStart, lineEnd int
}

func censusScopeFromStep(step *Step) censusScope {
	scope := censusScope{lineStart: step.LineStart, lineEnd: step.LineEnd}
	scope.timeStart, scope.timeEnd, scope.timeSet = step.WindowBounds()
	return scope
}

func (s censusScope) bounded() bool {
	return s.timeSet || s.lineStart > 0 || s.lineEnd > 0
}

func (s censusScope) admits(ev *tracequery.Event) bool {
	if s.timeSet && (ev.Ts < s.timeStart || ev.Ts > s.timeEnd) {
		return false
	}
	if s.lineStart > 0 && ev.Line < s.lineStart {
		return false
	}
	if s.lineEnd > 0 && ev.Line > s.lineEnd {
		return false
	}
	return true
}

func (s censusScope) describe() string {
	parts := []string{}
	if s.timeSet {
		parts = append(parts, fmt.Sprintf("window=%s..%s", formatSecondsToken(s.timeStart), formatSecondsToken(s.timeEnd)))
	}
	if s.lineStart > 0 || s.lineEnd > 0 {
		parts = append(parts, fmt.Sprintf("lines=%d..%d", s.lineStart, s.lineEnd))
	}
	if len(parts) == 0 {
		return "全文件"
	}
	return strings.Join(parts, " ")
}

type formatCensus struct {
	// line-level quality (⑧) — filled by finalize from the StreamScan shell
	path             string
	scope            censusScope
	scopedEventCount int
	lineCount        int
	scannedLineCount int
	eventCount       int
	parsedKnown      int
	unparsedLines    int
	parsePanics      int
	clockRegressions int
	firstTs          float64
	lastTs           float64
	unparsedSamples  []tracequery.UnparsedLineSample

	// ① event-name spectrum + unknown blind-spot list
	names        []nameCount
	namesTotal   int
	unknown      []nameCount
	unknownTotal int

	// ② marker forms
	markTotal                    int
	markActions                  map[string]int // engine SpanAction classification
	rawB, rawE, rawS, rawF, rawC int
	rawHPrefix                   int // "…|H:name" raw field forms
	rawETailI                    int // "E|pid|I<digits>" tail form
	rawEWithLoad                 int // "E|pid|<payload>" (payload beyond pid)

	// ③ clock_set_rate tracks
	clockTracks      []clockTrack
	clockTracksTotal int

	// ④ scheduling domain
	schedSwitchCount  int
	prioCounts        map[int]int
	prioMicrokernelRT int
	prioOver159       int
	prevStates        map[string]int
	nextInfoCount     int

	// ⑤ FS / IO
	namePrefixes      []nameCount
	namePrefixesTotal int
	fileKVEvents      int
	fileKVIno         int
	fileKVDev         int
	fileKVEntry       int
	blockIssue        int
	blockRemap        int
	blockComplete     int

	// ⑥ power events
	freqCount     int
	freqCPUs      map[int]bool
	freqLimit     int
	freqLimitCPUs map[int]bool
	idleCount     int
	idleCPUs      map[int]bool

	// ⑦ span census (annotated with the engine's exported semantic
	// classification — TDIAG B3)
	spanNames      []nameCount
	spanNamesTotal int
	spanHPrefix    int

	// streaming accumulators (live between observe calls; folded by finalize)
	nameCounts   map[string]*nameCount
	prefixCounts map[string]*nameCount
	clocks       map[string]*clockTrack
	spanCounts   map[string]int

	// faces this build cannot compute from exported engine API (honest list;
	// empty since the §28.13 exports landed — the mechanism stays for future
	// gaps)
	unsupportedFaces []string
}

// newFormatCensusAcc builds the streaming census accumulator for one step:
// observe() consumes every StreamScan event (scope filters inside), then
// finalize() folds the top-N faces and adopts the shell's line-level quality
// counters + typed unparsed samples.
func newFormatCensusAcc(scope censusScope) *formatCensus {
	return &formatCensus{
		scope:         scope,
		markActions:   map[string]int{},
		prioCounts:    map[int]int{},
		prevStates:    map[string]int{},
		freqCPUs:      map[int]bool{},
		freqLimitCPUs: map[int]bool{},
		idleCPUs:      map[int]bool{},
		nameCounts:    map[string]*nameCount{},
		prefixCounts:  map[string]*nameCount{},
		clocks:        map[string]*clockTrack{},
		spanCounts:    map[string]int{},
	}
}

func (c *formatCensus) observe(ev *tracequery.Event) {
	c.eventCount++
	if !c.scope.admits(ev) {
		return
	}
	c.scopedEventCount++
	key := ev.Name + "\x00" + string(ev.Type)
	if nc := c.nameCounts[key]; nc != nil {
		nc.Count++
	} else {
		c.nameCounts[key] = &nameCount{Name: ev.Name, Type: string(ev.Type), Count: 1}
	}
	if p := namePrefix(ev.Name); p != "" {
		pkey := p + "\x00" + string(ev.Type)
		if nc := c.prefixCounts[pkey]; nc != nil {
			nc.Count++
		} else {
			c.prefixCounts[pkey] = &nameCount{Name: p, Type: string(ev.Type), Count: 1}
		}
	}
	switch ev.Type {
	case tracequery.EventTraceMark:
		c.markTotal++
		action := ev.SpanAction
		if action == "" {
			action = "(none)"
		}
		c.markActions[action]++
		c.countRawMarkForms(ev.FieldText)
		if ev.SpanAction == "B" || ev.SpanAction == "S" {
			c.spanCounts[ev.SpanName]++
			if strings.HasPrefix(ev.SpanName, "H:") {
				c.spanHPrefix++
			}
		}
	case tracequery.EventClockSetRate:
		track := c.clocks[ev.ClockName]
		if track == nil {
			track = &clockTrack{Name: ev.ClockName, CPUs: map[int]bool{}, MinState: ev.Frequency, MaxState: ev.Frequency}
			c.clocks[ev.ClockName] = track
		}
		track.Count++
		if ev.CPUForFieldValid {
			track.CPUs[ev.CPUForField] = true
		}
		if ev.Frequency < track.MinState {
			track.MinState = ev.Frequency
		}
		if ev.Frequency > track.MaxState {
			track.MaxState = ev.Frequency
		}
	case tracequery.EventSchedSwitch:
		c.schedSwitchCount++
		c.countPrio(ev.PrevPrio)
		c.countPrio(ev.NextPrio)
		if ev.PrevState != "" {
			c.prevStates[ev.PrevState]++
		}
		if ev.NextInfo != "" {
			c.nextInfoCount++
		}
	case tracequery.EventSchedWakeup, tracequery.EventSchedWaking:
		c.countPrio(ev.WakeePrio)
	case tracequery.EventCPUFrequency:
		c.freqCount++
		if ev.CPUForFieldValid {
			c.freqCPUs[ev.CPUForField] = true
		}
	case tracequery.EventCPUFrequencyLimit:
		c.freqLimit++
		if ev.CPUForFieldValid {
			c.freqLimitCPUs[ev.CPUForField] = true
		}
	case tracequery.EventCPUIdle:
		c.idleCount++
		if ev.CPUForFieldValid {
			c.idleCPUs[ev.CPUForField] = true
		} else {
			c.idleCPUs[ev.CPU] = true
		}
	case tracequery.EventBlockIssue:
		c.blockIssue++
	case tracequery.EventBlockRemap:
		c.blockRemap++
	case tracequery.EventBlockComplete:
		c.blockComplete++
	}
	if ff := ev.FileFields; ff != nil {
		c.fileKVEvents++
		if ff.Ino != "" {
			c.fileKVIno++
		}
		if ff.Dev != "" {
			c.fileKVDev++
		}
		if ff.Entry != "" {
			c.fileKVEntry++
		}
	}
}

// finalize folds the streamed accumulators into the rendered top-N faces and
// adopts the StreamScan shell's whole-file quality counters + typed samples
// (TDIAG B4: the samples are the parse site's own — the former bounded
// second read, including its scanner.Err abort arm, is deleted, and windowed
// steps get samples exactly like full-file ones).
func (c *formatCensus) finalize(shell *tracequery.Index) {
	c.path = shell.Path
	c.lineCount = shell.LineCount
	c.scannedLineCount = shell.ScannedLineCount
	c.parsedKnown = shell.ParsedKnown
	c.unparsedLines = shell.UnparsedLines
	c.parsePanics = shell.ParseLinePanics
	c.clockRegressions = shell.ClockRegressions
	c.firstTs = shell.FirstTs
	c.lastTs = shell.LastTs
	c.unparsedSamples = shell.UnparsedSamples

	c.names, c.namesTotal = sortedNameCounts(c.nameCounts, censusEventNameCap)
	unknownOnly := map[string]*nameCount{}
	for k, nc := range c.nameCounts {
		if nc.Type == string(tracequery.EventUnknown) {
			unknownOnly[k] = nc
		}
	}
	c.unknown, c.unknownTotal = sortedNameCounts(unknownOnly, censusUnknownNameCap)
	c.namePrefixes, c.namePrefixesTotal = sortedNameCounts(c.prefixCounts, censusPrefixCap)
	c.clockTracks, c.clockTracksTotal = sortedClockTracks(c.clocks, censusClockTrackCap)
	c.spanNames, c.spanNamesTotal = sortedSpanCounts(c.spanCounts, censusSpanNameCap)

	// 缺 API 清单 (§28.13) fully consumed: view enumerator / StreamScan /
	// semantic near-miss exports / typed unparsed samples all landed — the
	// honest-disclosure list is empty in this build; the mechanism stays for
	// the next engine gap.
	c.unsupportedFaces = nil
}

func (c *formatCensus) countPrio(prio int) {
	if prio == 0 {
		return
	}
	c.prioCounts[prio]++
	if prio >= 140 && prio <= 159 {
		c.prioMicrokernelRT++
	}
	if prio > 159 {
		c.prioOver159++
	}
}

// countRawMarkForms censuses the RAW trace_mark field shapes (B|/E|/S|/F|/C|
// heads, |H: name prefix, E-with-payload, E|pid|I<digits> tails) from the
// engine-retained FieldText — the raw-form face beside the engine's
// SpanAction classification, so form/classification drift is visible.
func (c *formatCensus) countRawMarkForms(fieldText string) {
	ft := strings.TrimSpace(fieldText)
	if len(ft) < 2 || ft[1] != '|' {
		return
	}
	switch ft[0] {
	case 'B':
		c.rawB++
	case 'E':
		c.rawE++
	case 'S':
		c.rawS++
	case 'F':
		c.rawF++
	case 'C':
		c.rawC++
	default:
		return
	}
	if strings.Contains(ft, "|H:") {
		c.rawHPrefix++
	}
	if ft[0] == 'E' {
		rest := ft[2:]
		if idx := strings.IndexByte(rest, '|'); idx >= 0 && allDigits(rest[:idx]) {
			tail := rest[idx+1:]
			if tail != "" {
				c.rawEWithLoad++
				if tail[0] == 'I' && len(tail) > 1 && allDigits(tail[1:]) {
					c.rawETailI++
				}
			}
		}
	}
}

func allDigits(s string) bool {
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

// namePrefix extracts the first-underscore-segment prefix ("hmfs_", "f2fs_",
// "sched_", …) used for the FS/IO family-form spectrum: a prefix whose events
// classify as unknown IS the blind-spot signal this census exists for.
func namePrefix(name string) string {
	idx := strings.IndexByte(name, '_')
	if idx <= 0 {
		return ""
	}
	return name[:idx+1]
}

// EVOLUTION RECORD (TDIAG B4, §28.13, 2026-07-09): collectUnparsedSamples —
// the bounded second read that reconstructed samples by event-line absence
// (and had to honestly SKIP windowed indexes, plus disclose bufio.ErrTooLong
// aborts, 复核 P3-3) — is DELETED. Samples now come typed from the parse
// site itself (tracequery Index.UnparsedSamples / the StreamScan shell):
// windowed steps carry samples too, over-long lines are rune-safe
// byte-capped at collection, and there is no second file read to abort.

func sortedNameCounts(m map[string]*nameCount, cap int) ([]nameCount, int) {
	all := make([]nameCount, 0, len(m))
	for _, nc := range m {
		all = append(all, *nc)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return all[i].Type < all[j].Type
	})
	total := len(all)
	if len(all) > cap {
		all = all[:cap]
	}
	return all, total
}

func sortedClockTracks(m map[string]*clockTrack, cap int) ([]clockTrack, int) {
	all := make([]clockTrack, 0, len(m))
	for _, ct := range m {
		all = append(all, *ct)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Name < all[j].Name
	})
	total := len(all)
	if len(all) > cap {
		all = all[:cap]
	}
	return all, total
}

func sortedSpanCounts(m map[string]int, cap int) ([]nameCount, int) {
	all := make([]nameCount, 0, len(m))
	for name, count := range m {
		all = append(all, nameCount{Name: name, Count: count})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Name < all[j].Name
	})
	total := len(all)
	if len(all) > cap {
		all = all[:cap]
	}
	return all, total
}

func renderFormatCensus(c *formatCensus, emit func(string)) {
	// TDIAG B2 (§28.13): the census streams the WHOLE file (tracequery
	// StreamScan — no event index, no 250K budget); 流式解析事件 is the full
	// parsed-event count, and the scope filters faces ①-⑦ only.
	emit(fmt.Sprintf("格式普查(format_census): 普查范围=%s 范围内事件=%d (流式解析事件=%d 已识别=%d 行=%d 扫描行=%d)",
		c.scope.describe(), c.scopedEventCount, c.eventCount, c.parsedKnown, c.lineCount, c.scannedLineCount))
	emit(fmt.Sprintf("时间轴: first_ts=%s last_ts=%s clock_regressions=%d", formatSecondsToken(c.firstTs), formatSecondsToken(c.lastTs), c.clockRegressions))
	emit(fmt.Sprintf("行级质量: unparsed_lines=%d parse_panics=%d 扫描=流式全文件", c.unparsedLines, c.parsePanics))
	if c.scope.bounded() {
		emit("  (行级质量计数与不可解析样本为全文件流式口径;①-⑦ 统计面按普查范围过滤)")
	}
	for _, s := range c.unparsedSamples {
		emit(fmt.Sprintf("- 不可解析行样本 line=%d | %s", s.Line, clampToken(s.Text)))
	}
	if n := c.unparsedLines + c.parsePanics; n > len(c.unparsedSamples) && len(c.unparsedSamples) > 0 {
		emit(fmt.Sprintf("  (不可解析共 %d 行,样本帽 %d,仅列前 %d)", n, censusUnparsedSamples, len(c.unparsedSamples)))
	}

	emit(fmt.Sprintf("① 事件名全谱(共 %d 名,按计数列前 %d):", c.namesTotal, len(c.names)))
	for _, nc := range c.names {
		emit(fmt.Sprintf("- name=%s 引擎分类=%s count=%d", nc.Name, nc.Type, nc.Count))
	}
	emit(fmt.Sprintf("①a 未识别事件名单(引擎分类=unknown;共 %d 名,列前 %d)——格式盲点清单:", c.unknownTotal, len(c.unknown)))
	if len(c.unknown) == 0 {
		emit("- (无:全部事件名均被引擎分类)")
	}
	for _, nc := range c.unknown {
		emit(fmt.Sprintf("- name=%s count=%d ⚠ 引擎未分类", nc.Name, nc.Count))
	}

	emit(fmt.Sprintf("② 标记形普查: trace_mark 总数=%d", c.markTotal))
	if c.markTotal > 0 {
		actions := make([]string, 0, len(c.markActions))
		for a, n := range c.markActions {
			actions = append(actions, fmt.Sprintf("%s=%d", a, n))
		}
		sort.Strings(actions)
		emit("- 引擎动作分类: " + strings.Join(actions, " "))
		emit(fmt.Sprintf("- 原始字段形: B|=%d E|=%d S|=%d F|=%d C|=%d", c.rawB, c.rawE, c.rawS, c.rawF, c.rawC))
		emit(fmt.Sprintf("- H: 前缀形=%d E带载荷形=%d E|pid|I尾形=%d", c.rawHPrefix, c.rawEWithLoad, c.rawETailI))
	}

	emit(fmt.Sprintf("③ clock_set_rate 轨谱(共 %d 轨,按计数列前 %d):", c.clockTracksTotal, len(c.clockTracks)))
	for _, ct := range c.clockTracks {
		emit(fmt.Sprintf("- track=%s count=%d distinct_cpu_id=%d cpu_ids=%s 值域=[%d..%d]", ct.Name, ct.Count, len(ct.CPUs), formatCPUSet(ct.CPUs), ct.MinState, ct.MaxState))
	}

	emit(fmt.Sprintf("④ 调度域: sched_switch=%d next_info出现=%d/%d", c.schedSwitchCount, c.nextInfoCount, c.schedSwitchCount))
	emit("- prio 直方(按计数列前 " + fmt.Sprintf("%d", censusPrioCap) + "):" + formatIntHistogram(c.prioCounts, censusPrioCap))
	emit(fmt.Sprintf("- prio=140..159 (Harmony microkernel RT) 计数=%d", c.prioMicrokernelRT))
	emit(fmt.Sprintf("- prio>159 计数=%d", c.prioOver159))
	emit("- prev_state token 集: " + formatStringHistogram(c.prevStates, censusPrevStateCap))

	emit(fmt.Sprintf("⑤ FS/IO: 事件名前缀谱(共 %d 前缀,列前 %d):", c.namePrefixesTotal, len(c.namePrefixes)))
	for _, nc := range c.namePrefixes {
		emit(fmt.Sprintf("- prefix=%s 引擎分类=%s count=%d", nc.Name, nc.Type, nc.Count))
	}
	emit(fmt.Sprintf("- 文件 kv 覆盖率: 携文件字段事件=%d ino=%d dev=%d entry_name=%d", c.fileKVEvents, c.fileKVIno, c.fileKVDev, c.fileKVEntry))
	emit(fmt.Sprintf("- block 事件: issue=%d remap=%d complete=%d", c.blockIssue, c.blockRemap, c.blockComplete))

	emit(fmt.Sprintf("⑥ 电源事件: cpu_frequency=%d 覆盖CPU=%s cpu_frequency_limits=%d 覆盖CPU=%s cpu_idle=%d 覆盖CPU=%s",
		c.freqCount, formatCPUSet(c.freqCPUs), c.freqLimit, formatCPUSet(c.freqLimitCPUs), c.idleCount, formatCPUSet(c.idleCPUs)))

	emit(fmt.Sprintf("⑦ span 普查(B/S 起始记号;共 %d 名,按计数列前 %d;H: 前缀 span=%d):", c.spanNamesTotal, len(c.spanNames), c.spanHPrefix))
	for _, nc := range c.spanNames {
		// TDIAG B3 (§28.13): each top span is annotated with the engine's OWN
		// semantic classification — semantic_class=X (classified) or the
		// advisory near-miss flag (semantic vocabulary, no known pattern —
		// the naming-drift blind-spot list). One classifier, exported.
		line := fmt.Sprintf("- span=%s count=%d", clampToken(nc.Name), nc.Count)
		if class := tracequery.TraceSpanSemanticClass(nc.Name); class != "" {
			line += " semantic_class=" + class
		} else if tracequery.TraceSpanNearMissesSemanticWork(nc.Name) {
			line += " ⚠ near_miss(语义词汇命中但未匹配已知模式)"
		}
		emit(line)
	}

	if len(c.unsupportedFaces) > 0 {
		emit("本构建不支持的普查面(如实披露):")
		for _, face := range c.unsupportedFaces {
			emit("- " + face)
		}
	}
}

func formatCPUSet(set map[int]bool) string {
	if len(set) == 0 {
		return "{}"
	}
	cpus := make([]int, 0, len(set))
	for cpu := range set {
		cpus = append(cpus, cpu)
	}
	sort.Ints(cpus)
	parts := make([]string, len(cpus))
	for i, cpu := range cpus {
		parts[i] = fmt.Sprintf("%d", cpu)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatIntHistogram(m map[int]int, cap int) string {
	type kv struct{ k, v int }
	all := make([]kv, 0, len(m))
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	total := len(all)
	if len(all) > cap {
		all = all[:cap]
	}
	parts := make([]string, len(all))
	for i, e := range all {
		parts[i] = fmt.Sprintf("%d×%d", e.k, e.v)
	}
	out := " " + strings.Join(parts, " ")
	if total > len(all) {
		out += fmt.Sprintf(" (共 %d 值,列前 %d)", total, len(all))
	}
	return out
}

func formatStringHistogram(m map[string]int, cap int) string {
	type kv struct {
		k string
		v int
	}
	all := make([]kv, 0, len(m))
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	total := len(all)
	if len(all) > cap {
		all = all[:cap]
	}
	parts := make([]string, len(all))
	for i, e := range all {
		parts[i] = fmt.Sprintf("%s×%d", e.k, e.v)
	}
	out := strings.Join(parts, " ")
	if total > len(all) {
		out += fmt.Sprintf(" (共 %d token,列前 %d)", total, len(all))
	}
	if out == "" {
		out = "(无)"
	}
	return out
}
