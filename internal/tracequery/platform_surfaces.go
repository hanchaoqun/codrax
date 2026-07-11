package tracequery

import "sort"

// platform_surfaces.go — W-1 platform 标签同报告翻转修根 (ledger
// real_trace_campaign_20260705.md §29.28 ② W-1, CLOSE-1 批 2026-07-11).
//
// Witness (customlogs/cust710/open_gap_witness_02.txt): one tracediag run,
// same trace, same window — steps 6/7 (event_types=[workqueue,dma_fence] /
// [trace_mark]) printed platform=harmony while step 8 (event_types=[unknown])
// printed platform=donghu. Root cause: every view resolved the platform from
// detectFrameworkSurfaces over ITS OWN event set — the streaming event_search
// lane rebuilds idx.Events from the QUERY-MATCHED rows only (event_types
// filter + display cap), so the detection input drifted with the query and
// the mixed_harmony_base→donghu candidate fired only when the matched subset
// happened to contain android-framework comms (surfaceflinger / android.ui).
//
// Fix (platform 判定单点化): the platform-detection input is now a per-trace
// record —
//   - during every scan, ALL parsed events feed a platformSurfaceVote
//     (filter-independent within the scan; the same lane as the flavor vote);
//   - a from-0 scan MINTS the record into the per-file anchor set write-once,
//     exactly like the trace flavor ("a stable per-file property instead of a
//     per-window vote" — parse_anchor_index.go); every later scan (any view,
//     seeked or not) consumes the SAME record, so one trace answers with one
//     label per process by construction (an anchor seek requires FlavorSet,
//     which only from-0 scans write, so the record always exists before any
//     seeked scan);
//   - indexed builds stamp the record on the Index (anchor record preferred,
//     else the build's own vote); composite bundles OR-merge their children.
// The published FrameworkSurfaces list stays the per-query window-scoped
// diagnostic enumeration — only the platform INFERENCE input is trace-scoped.
// The flavor lane is untouched (负对照 pin: flavor/conf faces undisturbed).

// platformSurfaceScan is the typed platform-detection input record: which
// framework surfaces exist among the observed comms, plus the audit signals.
// Scoped==true discloses that the minting scan was query-scoped (window /
// line / pattern / type filter) rather than a full unfiltered pass — the
// label is still per-trace stable (write-once), but its evidence basis was
// narrower than the whole file.
type platformSurfaceScan struct {
	Set     bool
	Android bool
	Harmony bool
	Scoped  bool
	Signals []string
}

func (s platformSurfaceScan) clone() platformSurfaceScan {
	copied := s
	copied.Signals = append([]string(nil), s.Signals...)
	return copied
}

// platformSurfaceVote accumulates framework-surface markers from every
// parsed event of one scan. Hot-loop discipline (perf audit #21/#22): once
// BOTH surfaces are found the vote is settled and observe returns on two
// boolean reads; before that, each distinct comm is classified exactly once
// (comm strings are interned by the scan, so the dedupe map stays small and
// allocation-free on the lookup path).
type platformSurfaceVote struct {
	android bool
	harmony bool
	seen    map[string]struct{}
}

func newPlatformSurfaceVote() *platformSurfaceVote {
	return &platformSurfaceVote{seen: make(map[string]struct{}, 64)}
}

func (v *platformSurfaceVote) observe(ev Event) {
	if v == nil || (v.android && v.harmony) {
		return
	}
	v.observeComm(ev.Comm)
	v.observeComm(ev.PrevComm)
	v.observeComm(ev.NextComm)
	v.observeComm(ev.WakeeComm)
}

func (v *platformSurfaceVote) observeComm(comm string) {
	if comm == "" || (v.android && v.harmony) {
		return
	}
	if _, ok := v.seen[comm]; ok {
		return
	}
	v.seen[comm] = struct{}{}
	switch surface, _ := classifyFrameworkSurface(comm, ""); surface {
	case "android_framework":
		v.android = true
	case "harmony_framework":
		v.harmony = true
	}
}

// result mints the typed record. scoped discloses a query-scoped input basis
// (see platformSurfaceScanScoped).
func (v *platformSurfaceVote) result(scoped bool) platformSurfaceScan {
	record := platformSurfaceScan{Set: true, Scoped: scoped}
	if v != nil {
		record.Android = v.android
		record.Harmony = v.harmony
	}
	var signals []string
	if record.Android {
		signals = append(signals, "surface_android_framework", "android_process_marker")
	}
	if record.Harmony {
		signals = append(signals, "surface_harmony_framework", "harmony_process_marker")
	}
	if scoped {
		signals = append(signals, "platform_surfaces_scan_scoped")
	}
	sort.Strings(signals)
	record.Signals = signals
	return record
}

// platformSurfaceMintEligible is the CLOSE-1 复核 F2 minting-eligibility
// gate: only a scan whose parsed-event coverage is provably the COMPLETE
// artifact may mint the per-file record —
//   - no pattern prefilter (pattern-gated lines are never parsed, so the
//     vote would be blind to their comms; event_types / trace-mark action
//     filters apply AFTER the parse and do not narrow the vote),
//   - no time/line window (out-of-window lines are skipped before the
//     parse; early stops end the scan before EOF),
//   - the scan reached EOF (reachedEOF — a budget denial / early stop /
//     LineEnd break never qualifies).
// An ineligible scan mints nothing (the record waits for the next eligible
// scan) and consumes an existing record normally; with no record it falls
// back to its own scan vote, marked Scoped=true so resolveTracePlatform can
// disclose the partial basis (复核 F3). This closes the first-query-anchored
// basis hole: a pattern/window-narrowed first scan can no longer freeze a
// blind label for the whole file, and an LRU-evicted record re-mints from a
// complete basis only (跨驱逐同 run 翻转形 closed).
func platformSurfaceMintEligible(q Query, reachedEOF bool) bool {
	return reachedEOF && q.Pattern == "" &&
		q.TimeStart == 0 && q.TimeEnd == 0 && q.LineStart == 0 && q.LineEnd == 0
}

// platformDetectionSurfaces resolves the per-trace platform-detection input
// for the INDEXED lane: the build-time stamped record when present, else a
// deterministic on-the-fly vote over the index's full event set (synthetic
// test indexes; same inputs → same record).
func (idx *Index) platformDetectionSurfaces() platformSurfaceScan {
	if idx == nil {
		return platformSurfaceScan{}
	}
	if idx.platformSurfaces.Set {
		return idx.platformSurfaces
	}
	vote := newPlatformSurfaceVote()
	for i := range idx.Events {
		vote.observe(idx.Events[i])
	}
	return vote.result(idx.Windowed)
}

// mergePlatformSurfaceScans unions two records (composite bundles: every
// child's per-file authority contributes; the bundle's label reflects all
// member artifacts). An unset side contributes nothing.
func mergePlatformSurfaceScans(a, b platformSurfaceScan) platformSurfaceScan {
	if !a.Set {
		return b.clone()
	}
	if !b.Set {
		return a.clone()
	}
	merged := platformSurfaceScan{
		Set:     true,
		Android: a.Android || b.Android,
		Harmony: a.Harmony || b.Harmony,
		Scoped:  a.Scoped || b.Scoped,
	}
	set := map[string]bool{}
	for _, s := range a.Signals {
		set[s] = true
	}
	for _, s := range b.Signals {
		set[s] = true
	}
	for s := range set {
		merged.Signals = append(merged.Signals, s)
	}
	sort.Strings(merged.Signals)
	return merged
}
