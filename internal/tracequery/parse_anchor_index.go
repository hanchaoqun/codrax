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

// traceAnchorLineInterval is the anchor spacing. 64K lines ≈ 6-9 MB of
// typical ftrace text — ~170 anchors for a 1 GiB artifact.
const traceAnchorLineInterval = 65536

// traceAnchorCacheMaxFiles bounds the anchor cache; entries are a few KB.
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
	CoveredLines   int
	CoveredOffset  int64
	CoveredMaxTs   float64
	FlavorSet      bool
	Flavor         TraceFlavor
	FlavorConf     float64
	FlavorSignals  []string
	flavorObserved bool
}

type traceAnchorKey struct {
	path    string
	size    int64
	modUnix int64
	version string
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
		}
		if !existing.FlavorSet && set.FlavorSet {
			existing.FlavorSet = true
			existing.Flavor = set.Flavor
			existing.FlavorConf = set.FlavorConf
			existing.FlavorSignals = append([]string(nil), set.FlavorSignals...)
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
	set          *traceAnchorSet
	runningMaxTs float64
	// recordFrom is the first line this scan may append an anchor for
	// (extension must stay contiguous with prior coverage).
	recordFrom int
	byteOffset int64
	maxLine    int
}

func newAnchorRecorder(prior *traceAnchorSet, seek traceAnchor, seeked bool) *anchorRecorder {
	rec := &anchorRecorder{set: &traceAnchorSet{}}
	if prior != nil {
		rec.set = prior
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
}

// canExtend reports whether this scan's starting line keeps anchor
// extension contiguous with prior coverage.
func (r *anchorRecorder) canExtend(startLine int) bool {
	return startLine <= r.set.CoveredLines+1
}
