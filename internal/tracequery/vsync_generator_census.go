package tracequery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// VSync / frame-pacing generator census lane (SA-F2, DISPATCH-IND 批4,
// real_trace_campaign_20260705 §29.69 立案, 2026-07-14).
//
// Witness: on the tieba VSync question the model computed a 124.14ms "period"
// from two Choreographer#onVsync CONSUMER callbacks and never saw the
// generator side, although the raw capture carried 43 VSyncGenerator-1682
// mentions INCLUDING the generator's own authoritative period print
// (`H:GenerateVsyncCount:1, period:16552213, …` ≈ 16.55ms). Consumer callback
// spacing measures frame pacing (frames may be skipped between callbacks);
// the generator's period print states the signal period directly. This lane
// makes the generator-side account a deterministic typed face instead of a
// display-row lottery.
//
// Identification uses PRECISE signals only, both derived from a survey of the
// two real captures in eval/fixtures/real_traces (donghu_tieba_frame.systrace
// + donghu.ftrace, 2026-07-14) — never a ranker or similarity heuristic:
//
//   - thread-name closed set: comm == "VSyncGenerator" (the HarmonyOS
//     render_service software vsync generator thread; both captures) or
//     comm == "hwc_vsync_threa" (the hardware-composer real-vsync thread,
//     15-char kernel comm truncation of hwc_vsync_thread; donghu capture,
//     `realVsync recv` / `C|…|vsync_recv` prints);
//   - period-print format family: a trace_mark whose payload carries the
//     verbatim `GenerateVsyncCount` token — the generator loop's own print.
//     A thread emitting it is a generator even if its comm drifts from the
//     closed set. The authoritative period is the print's `period:<ns>`
//     field (tieba form `…period:16552213, currRefreshRate_:0, vsyncMode_:0`;
//     donghu form `…period:11091679, currRefreshRate:90, vsyncMode:1, now:…`
//     — both parsed by the same two anchored expressions below).
//
// Two calibers, one accumulator (each census says which one it used):
//
//   - window_population (window_stats / rank / bundle): every event inside
//     the query's line+time window, population-wide — the same admission the
//     other window faces use (no pid/pattern predicate).
//   - matched_rows (event_search): the pattern/type/window-matched set,
//     accumulated BEFORE the chronological display cap (CPUFrequencyCensus
//     precedent) and DELIBERATELY WITHOUT the query's pid/thread arms — the
//     generator almost never lives in the queried thread's process (the
//     query names the CONSUMER; that asymmetry IS the SA-F2 disease), and
//     the tool auto-injects the request's runtime target into target-less
//     event_search calls, which on the tieba witness reduced the census to
//     the single consumer-emitted wakeup row (events=0, no period) while the
//     generator's own rows sat outside the thread filter. The stream
//     prefilter is pattern-only, so target-free admission is available on
//     both twins. (A full population caliber is still impossible on the
//     stream path — non-pattern lines are never parsed — hence matched_rows,
//     not window_population.) Caliber note (修复轮 件5, P3-1, 2026-07-14):
//     the matched_rows caliber carries NO thread-incarnation fail-close —
//     the stream lane has no incarnation audit to consult, so a search
//     window spanning a generator-tid reuse would merge both incarnations
//     into one per-PID row. The window_population twin rides the
//     window_stats identityConflict gate; matched_rows is a search-scope
//     face and accepts this bound (generator tids are long-lived system
//     threads; a reuse inside one search window has no observed witness).
//
// Additive only: results with no generator sighting stay byte-identical
// (census nil).

// vsyncGeneratorThreadNames is the survey-derived closed comm set. Extending
// it requires a raw-capture witness (禁猜 — never add names speculatively).
var vsyncGeneratorThreadNames = map[string]bool{
	"VSyncGenerator":  true,
	"hwc_vsync_threa": true,
}

// vsyncGeneratorPeriodPrintToken is the verbatim generator-loop print token
// (survey: present in both real captures, emitted only by the generator
// thread).
const vsyncGeneratorPeriodPrintToken = "GenerateVsyncCount"

var (
	vsyncGeneratorPeriodPattern      = regexp.MustCompile(`\bperiod:([0-9]+)`)
	vsyncGeneratorRefreshRatePattern = regexp.MustCompile(`\bcurrRefreshRate_?:([0-9]+)`)
)

// VsyncGeneratorPeriod is one distinct authoritative period value parsed from
// the generator's own period print, with its row count (a window may
// legitimately carry several values across a refresh-rate switch).
type VsyncGeneratorPeriod struct {
	// PeriodNs is the print's verbatim `period:` field (nanoseconds — the
	// raw generator unit; ms/Hz are display derivations).
	PeriodNs int64 `json:"period_ns"`
	// Rows counts the period prints carrying exactly this value.
	Rows int `json:"rows"`
	// RefreshRate is the print's currRefreshRate/currRefreshRate_ field when
	// present and >0 (0 = the print carried no set rate — absence, never a
	// guess).
	RefreshRate int     `json:"refresh_rate,omitempty"`
	FirstTs     float64 `json:"first_ts,omitempty"`
	LastTs      float64 `json:"last_ts,omitempty"`
	FirstLine   int     `json:"first_line,omitempty"`
	LastLine    int     `json:"last_line,omitempty"`
}

// PeriodMs is the display derivation of the verbatim ns value.
func (p VsyncGeneratorPeriod) PeriodMs() float64 {
	return float64(p.PeriodNs) / 1e6
}

// VsyncGeneratorThreadCensus is one generator thread's account in scope.
type VsyncGeneratorThreadCensus struct {
	Thread ThreadRef `json:"thread"`
	// EventCount counts the rows this thread EMITTED in scope (its own
	// scheduler/mark/irq rows); WokenCount counts sched_wakeup/sched_waking
	// rows TARGETING it (other threads requesting a vsync tick).
	EventCount     int `json:"event_count"`
	TraceMarkCount int `json:"trace_mark_count,omitempty"`
	WokenCount     int `json:"woken_count,omitempty"`
	// Periods are the distinct authoritative period values parsed from this
	// thread's period prints (empty when the thread emitted no parseable
	// period print in scope — e.g. the hwc real-vsync thread);
	// PeriodPrintRows is the total period-print row count.
	Periods         []VsyncGeneratorPeriod `json:"periods,omitempty"`
	PeriodPrintRows int                    `json:"period_print_rows,omitempty"`
	// IdentifiedBy names the precise signal(s) that admitted this thread:
	// "thread_name", "period_print", or "thread_name+period_print".
	IdentifiedBy string  `json:"identified_by"`
	FirstTs      float64 `json:"first_ts,omitempty"`
	LastTs       float64 `json:"last_ts,omitempty"`
	FirstLine    int     `json:"first_line,omitempty"`
	LastLine     int     `json:"last_line,omitempty"`
}

// VsyncGeneratorCensus is the scope-level generator account.
type VsyncGeneratorCensus struct {
	// Caliber says what population the census covered:
	// "window_population" (every event in the query window) or
	// "matched_rows" (the event_search matched set, pre-truncation).
	Caliber string                       `json:"caliber"`
	Threads []VsyncGeneratorThreadCensus `json:"threads"`
	Summary string                       `json:"summary,omitempty"`
}

const (
	VsyncGeneratorCensusCaliberWindow  = "window_population"
	VsyncGeneratorCensusCaliberMatched = "matched_rows"
)

type vsyncGeneratorCensusThreadAcc struct {
	thread          ThreadRef
	eventCount      int
	traceMarkCount  int
	wokenCount      int
	periodPrintRows int
	periods         map[int64]*VsyncGeneratorPeriod
	byName          bool
	byPrint         bool
	firstTs, lastTs float64
	firstLn, lastLn int
}

type vsyncGeneratorCensusAcc struct {
	threads map[int]*vsyncGeneratorCensusThreadAcc
}

func newVsyncGeneratorCensusAcc() *vsyncGeneratorCensusAcc {
	return &vsyncGeneratorCensusAcc{threads: map[int]*vsyncGeneratorCensusThreadAcc{}}
}

func (a *vsyncGeneratorCensusAcc) bucket(pid int, comm string) *vsyncGeneratorCensusThreadAcc {
	b := a.threads[pid]
	if b == nil {
		b = &vsyncGeneratorCensusThreadAcc{periods: map[int64]*VsyncGeneratorPeriod{}}
		b.thread.PID = pid
		a.threads[pid] = b
	}
	if b.thread.Comm == "" && comm != "" {
		b.thread.Comm = comm
	}
	return b
}

func (b *vsyncGeneratorCensusThreadAcc) sight(ts float64, line int) {
	if b.firstTs == 0 || (ts > 0 && ts < b.firstTs) {
		b.firstTs = ts
	}
	if ts > b.lastTs {
		b.lastTs = ts
	}
	if b.firstLn == 0 || (line > 0 && line < b.firstLn) {
		b.firstLn = line
	}
	if line > b.lastLn {
		b.lastLn = line
	}
}

// observe records one in-scope event. The caller decides the scope (window
// admission or matched set); observe applies only the precise identification
// signals.
func (a *vsyncGeneratorCensusAcc) observe(ev Event) {
	if a == nil {
		return
	}
	// Emitter lane: the row's own thread is a generator (closed comm set
	// and/or the period-print format family). Positive pid required — a
	// pid-less row cannot anchor a per-thread account.
	isPeriodPrint := ev.Type == EventTraceMark && strings.Contains(ev.SpanName, vsyncGeneratorPeriodPrintToken)
	if ev.PID > 0 && (vsyncGeneratorThreadNames[ev.Comm] || isPeriodPrint) {
		b := a.bucket(ev.PID, ev.Comm)
		if ev.TGID > 0 && b.thread.TGID == 0 {
			b.thread.TGID = ev.TGID
		}
		b.eventCount++
		if vsyncGeneratorThreadNames[ev.Comm] {
			b.byName = true
		}
		if ev.Type == EventTraceMark {
			b.traceMarkCount++
		}
		if isPeriodPrint {
			b.byPrint = true
			b.periodPrintRows++
			if m := vsyncGeneratorPeriodPattern.FindStringSubmatch(ev.SpanName); m != nil {
				ns, _ := strconv.ParseInt(m[1], 10, 64)
				if ns > 0 {
					p := b.periods[ns]
					if p == nil {
						p = &VsyncGeneratorPeriod{PeriodNs: ns, FirstTs: ev.Ts, FirstLine: ev.Line}
						b.periods[ns] = p
					}
					p.Rows++
					if ev.Ts > 0 && (p.FirstTs == 0 || ev.Ts < p.FirstTs) {
						p.FirstTs = ev.Ts
					}
					if ev.Ts > p.LastTs {
						p.LastTs = ev.Ts
					}
					if ev.Line > 0 && (p.FirstLine == 0 || ev.Line < p.FirstLine) {
						p.FirstLine = ev.Line
					}
					if ev.Line > p.LastLine {
						p.LastLine = ev.Line
					}
					// 修复轮 件3 (P3-3, 2026-07-14): FIRST-SEEN wins — a
					// mid-window rate switch on the same period value must
					// not silently last-wins overwrite the published rate
					// (never last-wins doctrine; FirstTs/FirstLine already
					// anchor the sighting the kept rate belongs to).
					if p.RefreshRate == 0 {
						if rm := vsyncGeneratorRefreshRatePattern.FindStringSubmatch(ev.SpanName); rm != nil {
							if rate, err := strconv.Atoi(rm[1]); err == nil && rate > 0 {
								p.RefreshRate = rate
							}
						}
					}
				}
			}
		}
		b.sight(ev.Ts, ev.Line)
	}
	// Wakeup-target lane: another thread requested a tick from the generator.
	if (ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking) &&
		ev.WakeePID > 0 && vsyncGeneratorThreadNames[ev.WakeeComm] {
		b := a.bucket(ev.WakeePID, ev.WakeeComm)
		b.byName = true
		b.wokenCount++
		b.sight(ev.Ts, ev.Line)
	}
}

func (a *vsyncGeneratorCensusAcc) finalize(caliber string) *VsyncGeneratorCensus {
	if a == nil || len(a.threads) == 0 {
		return nil
	}
	census := &VsyncGeneratorCensus{Caliber: caliber}
	for _, b := range a.threads {
		row := VsyncGeneratorThreadCensus{
			Thread:          b.thread,
			EventCount:      b.eventCount,
			TraceMarkCount:  b.traceMarkCount,
			WokenCount:      b.wokenCount,
			PeriodPrintRows: b.periodPrintRows,
			FirstTs:         b.firstTs,
			LastTs:          b.lastTs,
			FirstLine:       b.firstLn,
			LastLine:        b.lastLn,
		}
		switch {
		case b.byName && b.byPrint:
			row.IdentifiedBy = "thread_name+period_print"
		case b.byPrint:
			row.IdentifiedBy = "period_print"
		default:
			row.IdentifiedBy = "thread_name"
		}
		for _, p := range b.periods {
			row.Periods = append(row.Periods, *p)
		}
		// Deterministic member order: rows descending, then ns ascending
		// (DET-1 discipline: typed constant tie keys, never map order).
		sort.Slice(row.Periods, func(i, j int) bool {
			if row.Periods[i].Rows != row.Periods[j].Rows {
				return row.Periods[i].Rows > row.Periods[j].Rows
			}
			return row.Periods[i].PeriodNs < row.Periods[j].PeriodNs
		})
		census.Threads = append(census.Threads, row)
	}
	sort.Slice(census.Threads, func(i, j int) bool {
		ti, tj := census.Threads[i], census.Threads[j]
		if ti.EventCount != tj.EventCount {
			return ti.EventCount > tj.EventCount
		}
		if ti.Thread.PID != tj.Thread.PID {
			return ti.Thread.PID < tj.Thread.PID
		}
		return ti.Thread.Comm < tj.Thread.Comm
	})
	census.Summary = vsyncGeneratorCensusSummary(census)
	return census
}

// FormatVsyncGeneratorPeriods renders the compact period listing shared by
// the banner row and the observation face: verbatim ns first, derived ms/Hz
// marked as derivations.
func FormatVsyncGeneratorPeriods(periods []VsyncGeneratorPeriod) string {
	if len(periods) == 0 {
		return ""
	}
	parts := make([]string, 0, len(periods))
	for _, p := range periods {
		part := fmt.Sprintf("period:%dns(≈%.3fms≈%.1fHz)×%d", p.PeriodNs, p.PeriodMs(), 1e9/float64(p.PeriodNs), p.Rows)
		if p.RefreshRate > 0 {
			part += fmt.Sprintf(" refresh_rate=%d", p.RefreshRate)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func vsyncGeneratorCensusSummary(census *VsyncGeneratorCensus) string {
	if census == nil || len(census.Threads) == 0 {
		return ""
	}
	scope := "all events in the queried window"
	if census.Caliber == VsyncGeneratorCensusCaliberMatched {
		scope = "all pattern-matched rows in the queried window BEFORE the chronological display truncation, thread/pid filters not applied"
	}
	parts := make([]string, 0, len(census.Threads))
	for _, t := range census.Threads {
		part := fmt.Sprintf("%s-%d events=%d", t.Thread.Comm, t.Thread.PID, t.EventCount)
		if len(t.Periods) > 0 {
			part += " " + FormatVsyncGeneratorPeriods(t.Periods)
		}
		parts = append(parts, part)
	}
	return fmt.Sprintf("vsync/frame-pacing generator census over %s: %s", scope, strings.Join(parts, "; "))
}

// ComputeVsyncGeneratorWindowCensus is the window_population-caliber census:
// one scan over idx.Events with the SAME population admission the other
// window_stats faces use (line window + time window; no pid/pattern
// predicate). Nil when no generator was sighted in the window.
func ComputeVsyncGeneratorWindowCensus(idx *Index, q Query) *VsyncGeneratorCensus {
	if idx == nil {
		return nil
	}
	acc := newVsyncGeneratorCensusAcc()
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		acc.observe(ev)
	}
	return acc.finalize(VsyncGeneratorCensusCaliberWindow)
}

// vsyncGeneratorCensusQuery strips the pid/thread arms off an event_search
// query — the census's target-free admission form (see the caliber note in
// the file header: the generator lives outside the queried thread's process
// by construction, and the tool's auto-injected runtime target must not
// blind the census — tieba run-2 witness, 2026-07-14).
func vsyncGeneratorCensusQuery(q Query) Query {
	q.PID = 0
	q.Thread = ""
	return q
}

// ComputeVsyncGeneratorSearchCensus is the indexed event_search twin of the
// streaming inline accumulation: the SAME admission predicate as EventSearch
// (eventInQuery) minus the display limit and minus the pid/thread arms
// (matched_rows caliber, target-free by design).
func ComputeVsyncGeneratorSearchCensus(idx *Index, q Query) *VsyncGeneratorCensus {
	if idx == nil {
		return nil
	}
	q = vsyncGeneratorCensusQuery(q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	acc := newVsyncGeneratorCensusAcc()
	for _, ev := range idx.Events {
		if !eventInQuery(ev, q, typeSet, actionSet) {
			continue
		}
		acc.observe(ev)
	}
	return acc.finalize(VsyncGeneratorCensusCaliberMatched)
}
