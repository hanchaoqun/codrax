package tracequery

import (
	"sync"
)

// P1 (2026-07-03): sparse per-file anchor index. Every windowed build used
// to re-stream the artifact from byte 0 to find its window — the R5
// recursive drilldown pattern issues 6-10 windows per run, so a GiB-scale
// trace paid 6-10 full prefix scans. Anchors record, every
// traceAnchorLineInterval lines, the byte offset plus the RUNNING MAX
// timestamp seen so far (clock regressions mean a single line's ts is not
// a safe upper bound for its predecessors); a later windowed build seeks to
// the last anchor whose running max is STRICTLY below the padded window
// start (and whose line number is below any line-window start), guaranteed
// to sit before every in-window line. Pure latency optimization: the gate
// logic downstream of the seek is byte-identical, so the parsed event set
// cannot change — pinned by TestAnchorSeekBuildsIdenticalIndex.
//
// The trace flavor is cached alongside: flavor voting depends on the first
// ~200 raw lines, which a seek never sees, so only from-byte-0 scans write
// the flavor and every seek-build reuses it (also making flavor a stable
// per-file property instead of a per-window vote).

// traceAnchorLineInterval is the anchor spacing. 8K lines ≈ 0.8-1.1 MB of
// typical ftrace text — ~1.4K anchors (≈33 KB) for a 1 GiB artifact. The
// spacing directly bounds the seek OVERSHOOT a warm windowed build re-scans
// before its window head; since the correctness batch made every overshoot
// line pay the physical-row audits plus one shared Event parse (perf audit
// #21, §29.25 处置委托 2026-07-10), the old 64K stride made anchor-seek rebuilds
// ~8× more expensive than the anchor resolution itself. Seek safety is
// stride-independent: an anchor is only chosen when its running-max ts is
// strictly below the padded window start.
const traceAnchorLineInterval = 8192

// traceAnchorCacheMaxFiles bounds the anchor cache; entries are a few tens
// of KB for GiB-scale artifacts.
const traceAnchorCacheMaxFiles = 32

type traceAnchor struct {
	LineNo       int     // last line INCLUDED before this anchor point
	ByteOffset   int64   // offset of the first byte of line LineNo+1
	RunningMaxTs float64 // max timestamp observed in lines 1..LineNo
}

type traceAnchorSet struct {
	Anchors []traceAnchor
	// CoveredLines/CoveredOffset mark how far contiguous scanning has
	// recorded anchors; a scan may extend coverage only when it starts at
	// or before this frontier (anchor positions are deterministic line
	// multiples, so extensions align).
	CoveredLines  int
	CoveredOffset int64
	CoveredMaxTs  float64
	// CoveredLastTs/Set and CoveredClockRegressions describe the contiguous
	// prefix through CoveredLines. They let a later contiguous extension keep
	// auditing physical timestamp order without rescanning that prefix.
	CoveredLastTs           float64
	CoveredLastTsSet        bool
	CoveredClockRegressions int
	// TimestampOrder is a COMPLETE-file proof and is written only by
	// finishEOF. A regression-free prefix must remain Unknown: it says nothing
	// about an unread suffix that may move back into a requested window.
	TimestampOrder TraceTimestampOrder
	FlavorSet      bool
	Flavor         TraceFlavor
	FlavorConf     float64
	FlavorSignals  []string
	flavorObserved bool
	// PlatformSurfaces is the per-file platform-detection record (W-1 修根,
	// platform_surfaces.go): written ONCE by a from-0 scan — the same rule
	// as the flavor — and reused by every later scan of the file, making the
	// platform label a stable per-file property instead of a per-query vote.
	PlatformSurfaces platformSurfaceScan
}

type traceAnchorKey struct {
	path     string
	size     int64
	modUnix  int64
	identity string
	version  string
}

type traceAnchorCache struct {
	mu    sync.Mutex
	items map[traceAnchorKey]*traceAnchorSet
	order []traceAnchorKey
}

var anchorCache = &traceAnchorCache{items: map[traceAnchorKey]*traceAnchorSet{}}

func (c *traceAnchorCache) load(key traceAnchorKey) *traceAnchorSet {
	c.mu.Lock()
	defer c.mu.Unlock()
	if set, ok := c.items[key]; ok {
		copied := *set
		copied.Anchors = append([]traceAnchor(nil), set.Anchors...)
		copied.FlavorSignals = append([]string(nil), set.FlavorSignals...)
		copied.PlatformSurfaces = set.PlatformSurfaces.clone()
		return &copied
	}
	return nil
}

func (c *traceAnchorCache) store(key traceAnchorKey, set *traceAnchorSet) {
	if set == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.items[key]; ok {
		// Keep the widest coverage; flavor is written once by a from-0 scan.
		if set.CoveredLines > existing.CoveredLines {
			existing.Anchors = append([]traceAnchor(nil), set.Anchors...)
			existing.CoveredLines = set.CoveredLines
			existing.CoveredOffset = set.CoveredOffset
			existing.CoveredMaxTs = set.CoveredMaxTs
			existing.CoveredLastTs = set.CoveredLastTs
			existing.CoveredLastTsSet = set.CoveredLastTsSet
			existing.CoveredClockRegressions = set.CoveredClockRegressions
		}
		// A complete proof may arrive with the same CoveredLines as the last
		// stride anchor (for a file ending exactly on the stride). Preserve it
		// independently of the wider-coverage comparison.
		if existing.TimestampOrder == TraceTimestampOrderUnknown && set.TimestampOrder != TraceTimestampOrderUnknown {
			existing.TimestampOrder = set.TimestampOrder
			existing.CoveredLastTs = set.CoveredLastTs
			existing.CoveredLastTsSet = set.CoveredLastTsSet
			existing.CoveredClockRegressions = set.CoveredClockRegressions
		}
		if !existing.FlavorSet && set.FlavorSet {
			existing.FlavorSet = true
			existing.Flavor = set.Flavor
			existing.FlavorConf = set.FlavorConf
			existing.FlavorSignals = append([]string(nil), set.FlavorSignals...)
		}
		// W-1 修根: the platform record is write-once like the flavor — the
		// FIRST from-0 determination is THE determination for this file
		// (upgrading it later would re-open the intra-run label flip the
		// record exists to kill).
		if !existing.PlatformSurfaces.Set && set.PlatformSurfaces.Set {
			existing.PlatformSurfaces = set.PlatformSurfaces.clone()
		}
		return
	}
	if len(c.order) >= traceAnchorCacheMaxFiles {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
	copied := *set
	copied.Anchors = append([]traceAnchor(nil), set.Anchors...)
	copied.FlavorSignals = append([]string(nil), set.FlavorSignals...)
	copied.PlatformSurfaces = set.PlatformSurfaces.clone()
	c.items[key] = &copied
	c.order = append(c.order, key)
}

// seekAnchorFor returns the latest anchor safe for the given padded window:
// running max strictly below any time-window start AND line number strictly
// below any line-window start. ok=false means scan from byte 0.
func (s *traceAnchorSet) seekAnchorFor(timeStartSet bool, paddedTimeStart float64, paddedLineStart int) (traceAnchor, bool) {
	if s == nil || len(s.Anchors) == 0 {
		return traceAnchor{}, false
	}
	if !timeStartSet && paddedLineStart <= 0 {
		return traceAnchor{}, false
	}
	best := traceAnchor{}
	ok := false
	for _, a := range s.Anchors {
		if timeStartSet && !(a.RunningMaxTs < paddedTimeStart) {
			break
		}
		if paddedLineStart > 0 && !(a.LineNo < paddedLineStart) {
			break
		}
		best = a
		ok = true
	}
	return best, ok
}

// anchorRecorder accumulates anchors during one scan.
type anchorRecorder struct {
	set              *traceAnchorSet
	runningMaxTs     float64
	lastTs           float64
	lastTsSet        bool
	clockRegressions int
	// recordFrom is the first line this scan may append an anchor for
	// (extension must stay contiguous with prior coverage).
	recordFrom int
	byteOffset int64
	maxLine    int
	contiguous bool
}

func newAnchorRecorder(prior *traceAnchorSet, seek traceAnchor, seeked bool) *anchorRecorder {
	rec := &anchorRecorder{set: &traceAnchorSet{}}
	if prior != nil {
		rec.set = prior
		rec.lastTs = prior.CoveredLastTs
		rec.lastTsSet = prior.CoveredLastTsSet
		rec.clockRegressions = prior.CoveredClockRegressions
	}
	if seeked {
		rec.runningMaxTs = seek.RunningMaxTs
		rec.byteOffset = seek.ByteOffset
	}
	rec.recordFrom = rec.set.CoveredLines + 1
	return rec
}

// observe advances the recorder by one raw line (rawLen includes the
// delimiter). ts/hasTS is the line's timestamp when known.
func (r *anchorRecorder) observe(lineNo int, rawLen int, ts float64, hasTS bool) {
	if hasTS && ts > r.runningMaxTs {
		r.runningMaxTs = ts
	}
	r.byteOffset += int64(rawLen)
	if lineNo > r.maxLine {
		r.maxLine = lineNo
	}
	// Lines below recordFrom are an overlap between the chosen seek anchor and
	// the already-covered frontier. Their order was accounted for by the prior
	// prefix metadata; count only the contiguous extension to avoid duplicates.
	if lineNo >= r.recordFrom && hasTS {
		if r.lastTsSet && ts < r.lastTs {
			r.clockRegressions++
		}
		r.lastTs = ts
		r.lastTsSet = true
	}
	if lineNo%traceAnchorLineInterval != 0 {
		return
	}
	if lineNo < r.recordFrom {
		return
	}
	// Contiguity: appending an anchor for lineNo requires prior coverage
	// through lineNo-1 (either cached or scanned by this recorder).
	r.set.Anchors = append(r.set.Anchors, traceAnchor{
		LineNo:       lineNo,
		ByteOffset:   r.byteOffset,
		RunningMaxTs: r.runningMaxTs,
	})
	r.set.CoveredLines = lineNo
	r.set.CoveredOffset = r.byteOffset
	r.set.CoveredMaxTs = r.runningMaxTs
	r.set.CoveredLastTs = r.lastTs
	r.set.CoveredLastTsSet = r.lastTsSet
	r.set.CoveredClockRegressions = r.clockRegressions
}

// finishEOF promotes the contiguous prefix metadata into a complete-artifact
// timestamp-order proof. Callers must invoke it only after ReadString returned
// io.EOF, never after a line/time gate or event-budget stop.
func (r *anchorRecorder) finishEOF() {
	if r == nil || !r.contiguous {
		return
	}
	if r.maxLine > r.set.CoveredLines {
		r.set.CoveredLines = r.maxLine
		r.set.CoveredOffset = r.byteOffset
		r.set.CoveredMaxTs = r.runningMaxTs
	}
	r.set.CoveredLastTs = r.lastTs
	r.set.CoveredLastTsSet = r.lastTsSet
	r.set.CoveredClockRegressions = r.clockRegressions
	if r.clockRegressions == 0 {
		r.set.TimestampOrder = TraceTimestampOrderMonotonic
	} else {
		r.set.TimestampOrder = TraceTimestampOrderRegressed
	}
}

// canExtend reports whether this scan's starting line keeps anchor
// extension contiguous with prior coverage.
func (r *anchorRecorder) canExtend(startLine int) bool {
	r.contiguous = startLine <= r.set.CoveredLines+1
	return r.contiguous
}
