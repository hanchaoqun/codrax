package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

// stream_window_sweep.go — §4.7 W3: view=window_sweep, the streaming coverage
// scan for second-scale-or-longer dense windows. When the request window is
// too dense for a single in-memory index (index_event_limit), the model used
// to blind-bisect the window over many denial rounds; window_sweep instead
// makes ONE streaming pass over the requested window (same channel family as
// StreamEventSearch — never subject to the index event budget), aggregates
// pure per-bucket counts, and returns advisory top-K dense sub-windows plus a
// compact coverage table so the next drill-down window is an informed pick.
//
// Red-line compliance (precise-signals-for-hard-gates): every number this
// view emits is an exact integer count, but ALL of its outputs — hotspot
// ranking, suggested follow-up views, the coverage table — are soft guidance
// for the model's next call. Nothing here feeds a hard gate, denial, or
// emit-time reject.

const (
	// ViewWindowSweep is the canonical view name, shared by the schema enum,
	// the capacity table, and the denial recovery surfaces (same
	// no-drift-token pattern as FallbackViewEventSearch).
	ViewWindowSweep = "window_sweep"

	// Bucket width bounds (milliseconds). The default targets the C3 coverage
	// window amplitude (80-150ms): one bucket ≈ one candidate drill window.
	WindowSweepDefaultBucketMs = 100.0
	WindowSweepMinBucketMs     = 50.0
	WindowSweepMaxBucketMs     = 500.0

	// WindowSweepDefaultTopK is the default hotspot row count (capacity-table
	// DefaultLimit for the view; an explicit limit overrides it).
	WindowSweepDefaultTopK = 8

	// WindowSweepCoverageMaxRows caps the rendered coverage table; longer
	// sweeps fold equidistant bucket groups into one row each (annotated).
	WindowSweepCoverageMaxRows = 40

	// WindowSweepRecoveryMinWindowSeconds gates the "run window_sweep first"
	// sentence on the index_event_limit recovery surfaces: only requests whose
	// explicit time window spans STRICTLY more than this many seconds get the
	// window_sweep-first steer (precise signal: two typed request-window
	// floats). Shorter windows keep the historical event_search-first text.
	WindowSweepRecoveryMinWindowSeconds = 1.0
)

// ClampWindowSweepBucketMs normalizes the caller's bucket_ms: non-positive /
// unset (and NaN) collapse to the default, everything else clamps into
// [WindowSweepMinBucketMs, WindowSweepMaxBucketMs]. Exact integer and float
// inputs clamp identically.
func ClampWindowSweepBucketMs(bucketMs float64) float64 {
	// !(x > 0) is deliberately used over x <= 0: it also catches NaN.
	if !(bucketMs > 0) {
		return WindowSweepDefaultBucketMs
	}
	if bucketMs < WindowSweepMinBucketMs {
		return WindowSweepMinBucketMs
	}
	if bucketMs > WindowSweepMaxBucketMs {
		return WindowSweepMaxBucketMs
	}
	return bucketMs
}

// WindowSweepBucketCounts are the pure per-bucket counting signals. All exact
// integers; consumed only for advisory ordering and display.
type WindowSweepBucketCounts struct {
	// SchedSwitches counts sched_switch events in the bucket.
	SchedSwitches int `json:"sched_switches"`
	// SchedWakeups counts sched_wakeup + sched_waking events (flavors differ
	// in which of the two they emit for the same semantic signal).
	SchedWakeups int `json:"sched_wakeups"`
	// DStateEntries counts sched_switch events whose prev_state contains "D"
	// (uninterruptible-sleep entry).
	DStateEntries int `json:"d_state_entries"`
	// IRQEntries counts irq handler entry events.
	IRQEntries int `json:"irq_entries"`
	// TraceMarks counts trace_mark (B/E/C/S/F) rows.
	TraceMarks int `json:"trace_marks"`
	// TargetPIDSwitches counts sched_switch events where the query's target
	// pid participates (switched in OR switched out). Zero when no pid given.
	TargetPIDSwitches int `json:"target_pid_switches,omitempty"`
}

// WindowSweepBucket is one coverage-table row: a single bucket, or — on
// folded tables — an equidistant group of consecutive buckets.
type WindowSweepBucket struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
	// Buckets is the number of grid buckets this row spans (1 unfolded).
	Buckets int `json:"buckets"`
	WindowSweepBucketCounts
}

// WindowSweepHotspot is one advisory top-K dense sub-window.
type WindowSweepHotspot struct {
	Rank    int     `json:"rank"`
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
	WindowSweepBucketCounts
	// SuggestedViews are advisory follow-up views for this sub-window
	// (window_stats/frame_window always; wakeup_chain on above-average wakeup
	// density; critical_blocking_calls on above-average D-state entries).
	SuggestedViews []string `json:"suggested_views,omitempty"`
	Summary        string   `json:"summary,omitempty"`
}

// Rank-basis labels (advisory provenance, not typed gate inputs).
const (
	WindowSweepRankBasisTargetPID = "target_pid_sched_switch_participation"
	WindowSweepRankBasisGlobal    = "global_sched_switch_density"
)

// WindowSweepResult is the typed result section for view=window_sweep.
type WindowSweepResult struct {
	Window            TimeWindow           `json:"window"`
	BucketMs          float64              `json:"bucket_ms"`
	RequestedBucketMs float64              `json:"requested_bucket_ms,omitempty"`
	BucketCount       int                  `json:"bucket_count"`
	TargetPID         int                  `json:"target_pid,omitempty"`
	RankBasis         string               `json:"rank_basis,omitempty"`
	Hotspots          []WindowSweepHotspot `json:"hotspots,omitempty"`
	Coverage          []WindowSweepBucket  `json:"coverage,omitempty"`
	CoverageFolded    bool                 `json:"coverage_folded,omitempty"`
	// CoverageFoldSpan is the bucket count folded into one coverage row.
	CoverageFoldSpan int      `json:"coverage_fold_span,omitempty"`
	Caveats          []string `json:"caveats,omitempty"`
}

// StreamWindowSweep scans the requested window once, streaming, and
// aggregates per-bucket counting signals. It never materializes the in-memory
// event index, so it is NOT subject to the index event budget; memory stays
// O(observed buckets). The sparse per-file anchor index (P1) is reused to
// seek past the pre-window prefix when available.
func StreamWindowSweep(ctx context.Context, path string, q Query) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	requestedBucketMs := q.BucketMs
	bucketMs := ClampWindowSweepBucketMs(q.BucketMs)
	bucketSec := bucketMs / 1000
	// Top-K comes from the capacity table BEFORE normalizeQuery substitutes
	// the shared 40-row default: window_sweep's default is its own
	// DefaultLimit (8), an explicit positive limit is honored as requested.
	topK := ViewCapacityFor(ViewWindowSweep).ClampLimit(q.Limit)
	q.View = ViewWindowSweep
	q = normalizeQuery(nil, q)

	// idx.Windowed is deliberately NOT set here (StreamEventSearch precedent):
	// the sweep is a streaming scan, not a bounded INDEX parse. Setting it made
	// resultCaveats attach windowed_index_parse ("... rerun with a wider
	// time/line window or omit the window to build the full index"), which both
	// misdescribes the channel and steers the model back into the
	// index_event_limit denial loop this view exists to break. The
	// streamed_window_sweep caveat below is the authoritative scope statement.
	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}

	// P1 anchor-seek reuse: jump to the last cached anchor guaranteed to
	// precede every in-window line instead of re-streaming the whole prefix.
	// Counting needs no pre-window state (pure counts, no open-interval
	// reconstruction), so the seek can only skip guaranteed-out-of-window
	// lines. Seeking requires the cached flavor (a seek never sees the first
	// ~200 raw lines the flavor vote depends on) — same rule as parseFile.
	anchorKey := traceAnchorKey{path: path, size: info.Size(), modUnix: info.ModTime().UnixNano(), version: ParserVersion}
	anchorSet := anchorCache.load(anchorKey)
	startLine := 1
	seeked := false
	var seekAnchor traceAnchor
	if anchorSet != nil && anchorSet.FlavorSet {
		timeStartSet := q.TimeStartSet && q.TimeStart > 0
		if a, ok := anchorSet.seekAnchorFor(timeStartSet, q.TimeStart, q.LineStart); ok {
			if _, serr := f.Seek(a.ByteOffset, io.SeekStart); serr == nil {
				seekAnchor = a
				startLine = a.LineNo + 1
				seeked = true
			}
		}
	}
	// Anchor recording (same rules as parseFile): a sweep that scans a not yet
	// anchored region extends the per-file anchor coverage so later windowed
	// builds/sweeps can seek instead of re-streaming the prefix. Pure latency
	// reuse — the recorder never influences counting.
	recorder := newAnchorRecorder(anchorSet, seekAnchor, seeked)
	recording := recorder.canExtend(startLine)

	intern := newStringInterner()
	flavor := newFlavorVote(path)
	reader := bufio.NewReaderSize(f, 256*1024)
	buckets := map[int64]*WindowSweepBucketCounts{}
	seenTimeWindow := false
	lastParsedTs := 0.0
	targetPID := q.PID
	// Deliberately NO raw-line prefilter: every in-window line goes through
	// the shared classifier so bucket counts stay exactly consistent with the
	// indexed views across all trace flavors/spellings. Cost is bounded by
	// the request window (the anchor seek skips the prefix) and nothing is
	// retained per line, so memory stays O(buckets).
	for lineNo := startLine; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			if recording {
				// The recorder's running max must see EVERY line's ts
				// (including skipped/stop lines) or a future time-window seek
				// could jump past an in-window line — same rule as parseFile.
				ts, hasTS := parseLineTimestamp(trimmed)
				recorder.observe(lineNo, len(line), ts, hasTS)
			}
			if q.LineEnd > 0 && lineNo > q.LineEnd {
				break
			}
			if q.LineStart > 0 && lineNo < q.LineStart {
				if !seeked && lineNo <= 200 {
					flavor.observeRawLine(trimmed)
				}
				goto nextLine
			}
			if q.TimeStart > 0 || q.TimeEnd > 0 {
				ts, hasTS := parseLineTimestamp(trimmed)
				if hasTS {
					if q.TimeEnd > 0 && ts > q.TimeEnd {
						break
					}
					if q.TimeStart > 0 && ts < q.TimeStart {
						if !seeked && lineNo <= 200 {
							flavor.observeRawLine(trimmed)
						}
						goto nextLine
					}
					seenTimeWindow = true
				} else if q.TimeStart > 0 && !seenTimeWindow {
					if !seeked && lineNo <= 200 {
						flavor.observeRawLine(trimmed)
					}
					goto nextLine
				}
			}
			flavor.observeRawLine(trimmed)
			{
				// Parse-quality counters mirror the streamed event_search
				// path so coverage/quality caveats keep surfacing.
				panicsBefore := idx.ParseLinePanics
				ev, ok := safeParseLine(lineNo, trimmed, intern, idx)
				if !ok {
					if trimmed != "" && idx.ParseLinePanics == panicsBefore {
						idx.UnparsedLines++
					}
					goto nextLine
				}
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
				if ev.Ts <= 0 {
					goto nextLine
				}
				switch ev.Type {
				case EventSchedSwitch:
					acc := windowSweepBucketFor(buckets, ev.Ts, bucketSec)
					acc.SchedSwitches++
					if strings.Contains(ev.PrevState, "D") {
						acc.DStateEntries++
					}
					if targetPID > 0 && (ev.PrevPID == targetPID || ev.NextPID == targetPID) {
						acc.TargetPIDSwitches++
					}
				case EventSchedWakeup, EventSchedWaking:
					windowSweepBucketFor(buckets, ev.Ts, bucketSec).SchedWakeups++
				case EventIRQ:
					if strings.Contains(strings.ToLower(ev.Name), "entry") {
						windowSweepBucketFor(buckets, ev.Ts, bucketSec).IRQEntries++
					}
				case EventTraceMark:
					windowSweepBucketFor(buckets, ev.Ts, bucketSec).TraceMarks++
				}
			}
		}
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return Result{}, readErr
		}
	}

	if seeked && anchorSet != nil && anchorSet.FlavorSet {
		// Seek-scans never see the file head; reuse the cached from-0 vote.
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = anchorSet.Flavor, anchorSet.FlavorConf, append([]string(nil), anchorSet.FlavorSignals...)
	} else {
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	}
	if recording {
		// Persist extended anchor coverage for later builds/sweeps. Only a
		// from-0 scan may publish the flavor (the vote depends on the file
		// head); store() keeps the widest coverage and writes flavor once, so
		// repeated sweeps cannot regress an existing richer entry.
		if !seeked && !recorder.set.FlavorSet {
			recorder.set.FlavorSet = true
			recorder.set.Flavor = idx.TraceFlavor
			recorder.set.FlavorConf = idx.FlavorConfidence
			recorder.set.FlavorSignals = append([]string(nil), idx.FlavorSignals...)
		}
		anchorCache.store(anchorKey, recorder.set)
	}
	flavorValue, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavorValue
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavorValue, frameworkSurfaces, signals)

	sweep := buildWindowSweepResult(q, buckets, bucketMs, requestedBucketMs, topK, targetPID)
	start, end := q.TimeStart, q.TimeEnd
	if start == 0 && sweep.Window.StartTs > 0 {
		start = sweep.Window.StartTs
	}
	if end == 0 && sweep.Window.EndTs > 0 {
		end = sweep.Window.EndTs
	}

	res := Result{
		View:                        ViewWindowSweep,
		SourcePath:                  idx.Path,
		TraceFlavor:                 string(flavorValue),
		Platform:                    string(platform),
		PlatformCandidate:           platformCandidate,
		PlatformCandidateConfidence: platformCandidateConfidence,
		PlatformCandidateSignals:    platformCandidateSignals,
		FlavorConfidence:            confidence,
		FlavorSignals:               signals,
		FrameworkMode:               FrameworkModeForPlatform(platform),
		FrameworkSurfaces:           frameworkSurfaces,
		TimeUnit:                    "seconds",
		PrioritySemantics:           PrioritySemanticsForFlavor(flavorValue),
		LineCount:                   idx.LineCount,
		ScannedLineCount:            idx.ScannedLineCount,
		UnparsedLineCount:           idx.UnparsedLines,
		ParseLinePanics:             idx.ParseLinePanics,
		ClockRegressions:            idx.ClockRegressions,
		EventCount:                  idx.ParsedKnown,
		TimeStart:                   start,
		TimeEnd:                     end,
		WindowSweep:                 &sweep,
	}
	res.Caveats = append(res.Caveats,
		fmt.Sprintf("streamed_window_sweep=true; scanned %d line(s) without building or caching a full trace index; per-bucket counts are exact, but hotspot ranking, suggested views, and the coverage table are advisory drill-down guidance only", idx.ScannedLineCount))
	if strings.TrimSpace(q.Pattern) != "" {
		res.Caveats = append(res.Caveats, "pattern is not consumed by view=window_sweep; use view=event_search to locate exact tokens/lines")
	}
	// A thread selector that carries no pid (pure name) cannot drive the
	// pid-participation rank basis; never let it silently fall back to global
	// density — same "not consumed" transparency as the pattern caveat above.
	if strings.TrimSpace(q.Thread) != "" && targetPID <= 0 {
		res.Caveats = append(res.Caveats, "thread name without an embedded pid is not consumed by view=window_sweep ranking; hotspots are ranked by global sched_switch density — pass pid=<pid> to rank by target-pid sched_switch participation")
	}
	// Same parse-quality caveat wording as the streamed event_search path.
	if idx.ParseLinePanics > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d trace line(s) could not be parsed and were skipped; results may undercount events near those lines", idx.ParseLinePanics))
	}
	if idx.ClockRegressions > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d timestamp regression(s) detected in the trace (clock moved backwards); duration and ordering metrics around those points are unreliable", idx.ClockRegressions))
	}
	if idx.UnparsedLines > 0 && idx.ScannedLineCount > 0 && float64(idx.UnparsedLines) > unparsedLineCaveatRatio*float64(idx.ScannedLineCount) {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d of %d scanned lines did not match any known trace format; coverage may be incomplete", idx.UnparsedLines, idx.ScannedLineCount))
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	res.Caveats = dedupStrings(res.Caveats)
	return res, nil
}

func windowSweepBucketFor(buckets map[int64]*WindowSweepBucketCounts, ts, bucketSec float64) *WindowSweepBucketCounts {
	key := int64(math.Floor(ts / bucketSec))
	// Grid-boundary correction: float division can land one ulp off on exact
	// boundaries (10.3/0.1 -> 102.99999999999999 -> floor 102). Correct with
	// the SAME multiplication formula that publishes hotspot/coverage windows
	// (float64(key)*bucketSec), so an event's bucket attribution always agrees
	// with the [StartTs, EndTs) window the result advertises for that bucket.
	if ts >= float64(key+1)*bucketSec {
		key++
	} else if ts < float64(key)*bucketSec {
		key--
	}
	acc := buckets[key]
	if acc == nil {
		acc = &WindowSweepBucketCounts{}
		buckets[key] = acc
	}
	return acc
}

// buildWindowSweepResult turns the sparse bucket map into the typed result:
// ranked top-K hotspots plus the ≤WindowSweepCoverageMaxRows coverage table.
// Pure so tests can pin ranking/folding without a file scan.
func buildWindowSweepResult(q Query, buckets map[int64]*WindowSweepBucketCounts, bucketMs, requestedBucketMs float64, topK, targetPID int) WindowSweepResult {
	bucketSec := bucketMs / 1000
	sweep := WindowSweepResult{
		BucketMs:  bucketMs,
		TargetPID: targetPID,
	}
	if requestedBucketMs > 0 && requestedBucketMs != bucketMs {
		sweep.RequestedBucketMs = requestedBucketMs
		sweep.Caveats = append(sweep.Caveats,
			fmt.Sprintf("bucket_ms=%g clamped to %g (allowed %g..%g)", requestedBucketMs, bucketMs, WindowSweepMinBucketMs, WindowSweepMaxBucketMs))
	}
	if len(buckets) == 0 {
		sweep.Window = TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
		sweep.Caveats = append(sweep.Caveats,
			"window_sweep observed zero counted scheduler/irq/trace-mark events in the selected scope; verify time/line/pid filters before treating this as absence")
		return sweep
	}

	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	firstKey, lastKey := keys[0], keys[len(keys)-1]
	bucketCount := int(lastKey-firstKey) + 1
	sweep.BucketCount = bucketCount
	sweep.Window = TimeWindow{
		StartTs: float64(firstKey) * bucketSec,
		EndTs:   float64(lastKey+1) * bucketSec,
	}
	if q.TimeStart > 0 && q.TimeStart > sweep.Window.StartTs {
		sweep.Window.StartTs = q.TimeStart
	}
	if q.TimeEnd > 0 && q.TimeEnd < sweep.Window.EndTs {
		sweep.Window.EndTs = q.TimeEnd
	}

	// Rank basis: target-pid participation when a pid is given AND observed;
	// otherwise global sched_switch density. Falling back (rather than
	// returning an empty hotspot list) keeps the result actionable — the
	// fallback is named in a caveat, never silently.
	rankBasis := WindowSweepRankBasisGlobal
	if targetPID > 0 {
		totalTarget := 0
		for _, acc := range buckets {
			totalTarget += acc.TargetPIDSwitches
		}
		if totalTarget > 0 {
			rankBasis = WindowSweepRankBasisTargetPID
		} else {
			sweep.Caveats = append(sweep.Caveats,
				fmt.Sprintf("target pid %d did not participate in any sched_switch inside the swept scope; hotspot ranking fell back to global sched_switch density", targetPID))
		}
	}
	sweep.RankBasis = rankBasis

	// Bucket averages drive the advisory follow-up view picks.
	var totalWakeups, totalDState int
	for _, acc := range buckets {
		totalWakeups += acc.SchedWakeups
		totalDState += acc.DStateEntries
	}
	avgWakeups := float64(totalWakeups) / float64(bucketCount)
	avgDState := float64(totalDState) / float64(bucketCount)

	ranked := append([]int64(nil), keys...)
	metric := func(key int64) int {
		if rankBasis == WindowSweepRankBasisTargetPID {
			return buckets[key].TargetPIDSwitches
		}
		return buckets[key].SchedSwitches
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		mi, mj := metric(ranked[i]), metric(ranked[j])
		if mi != mj {
			return mi > mj
		}
		si, sj := buckets[ranked[i]].SchedSwitches, buckets[ranked[j]].SchedSwitches
		if si != sj {
			return si > sj
		}
		return ranked[i] < ranked[j]
	})
	for _, key := range ranked {
		if len(sweep.Hotspots) >= topK {
			break
		}
		if metric(key) <= 0 {
			break
		}
		acc := buckets[key]
		hotspot := WindowSweepHotspot{
			Rank:                    len(sweep.Hotspots) + 1,
			StartTs:                 float64(key) * bucketSec,
			EndTs:                   float64(key+1) * bucketSec,
			WindowSweepBucketCounts: *acc,
			SuggestedViews:          windowSweepSuggestedViews(*acc, avgWakeups, avgDState),
		}
		hotspot.Summary = fmt.Sprintf(
			"window_sweep hotspot %.6f..%.6f sched_switches=%d wakeups=%d d_state_entries=%d irq_entries=%d trace_marks=%d target_pid_switches=%d; advisory next views: %s with this exact time window",
			hotspot.StartTs, hotspot.EndTs, acc.SchedSwitches, acc.SchedWakeups, acc.DStateEntries, acc.IRQEntries, acc.TraceMarks, acc.TargetPIDSwitches,
			strings.Join(hotspot.SuggestedViews, "/"))
		sweep.Hotspots = append(sweep.Hotspots, hotspot)
	}

	sweep.Coverage, sweep.CoverageFolded, sweep.CoverageFoldSpan = windowSweepCoverageRows(buckets, firstKey, lastKey, bucketSec)
	if sweep.CoverageFolded {
		sweep.Caveats = append(sweep.Caveats,
			fmt.Sprintf("coverage table folded %d bucket(s) into %d row(s) (%d bucket(s) per row, equidistant); per-row counts are sums over the folded buckets", bucketCount, len(sweep.Coverage), sweep.CoverageFoldSpan))
	}
	return sweep
}

// windowSweepCoverageRows materializes the coverage table over the contiguous
// bucket grid [firstKey..lastKey], zero-filling quiet buckets. When the grid
// exceeds WindowSweepCoverageMaxRows rows, consecutive buckets fold
// equidistantly into ceil(n/max)-bucket groups. Memory is O(rows), never
// O(grid): group slots are at most WindowSweepCoverageMaxRows.
func windowSweepCoverageRows(buckets map[int64]*WindowSweepBucketCounts, firstKey, lastKey int64, bucketSec float64) ([]WindowSweepBucket, bool, int) {
	total := int(lastKey-firstKey) + 1
	foldSpan := 1
	if total > WindowSweepCoverageMaxRows {
		foldSpan = (total + WindowSweepCoverageMaxRows - 1) / WindowSweepCoverageMaxRows
	}
	rowCount := (total + foldSpan - 1) / foldSpan
	rows := make([]WindowSweepBucket, rowCount)
	for i := range rows {
		startKey := firstKey + int64(i*foldSpan)
		endKey := startKey + int64(foldSpan)
		if endKey > lastKey+1 {
			endKey = lastKey + 1
		}
		rows[i].StartTs = float64(startKey) * bucketSec
		rows[i].EndTs = float64(endKey) * bucketSec
		rows[i].Buckets = int(endKey - startKey)
	}
	for key, acc := range buckets {
		i := int((key - firstKey) / int64(foldSpan))
		rows[i].SchedSwitches += acc.SchedSwitches
		rows[i].SchedWakeups += acc.SchedWakeups
		rows[i].DStateEntries += acc.DStateEntries
		rows[i].IRQEntries += acc.IRQEntries
		rows[i].TraceMarks += acc.TraceMarks
		rows[i].TargetPIDSwitches += acc.TargetPIDSwitches
	}
	return rows, foldSpan > 1, foldSpan
}

// windowSweepSuggestedViews maps a hotspot's count profile to advisory
// follow-up views (§4.7): the bucket ranked hot on switch/participation
// density always gets window_stats/frame_window; above-average wakeup density
// adds wakeup_chain; above-average D-state entries add
// critical_blocking_calls. Soft guidance only.
func windowSweepSuggestedViews(c WindowSweepBucketCounts, avgWakeups, avgDState float64) []string {
	views := []string{"window_stats", "frame_window"}
	if c.SchedWakeups > 0 && float64(c.SchedWakeups) >= avgWakeups {
		views = append(views, "wakeup_chain")
	}
	if c.DStateEntries > 0 && float64(c.DStateEntries) >= avgDState {
		views = append(views, "critical_blocking_calls")
	}
	return views
}
