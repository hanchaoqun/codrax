package tracequery

// freq_side_scan.go — CLUSTER-FIX-1 (user ruling 2026-07-18): the STREAMING
// full-file frequency side-scan. When an Index cannot present the R6 rule-4
// full-file curves (line-window early stop, padding truncation, anchor-seek
// build whose per-file record was never stamped — the >fullFreqCurveAnchor-
// SampleCap large-trace shape, MaxEvents-bounded windows of a >250k-event
// trace, composite children skipped by the window intersection), the cluster
// derivation used to fall back to the window/budget CARVE of idx.Events —
// re-opening the exact CAP-3 (§29.11) disease family: cluster count drifting
// per window, the middle cluster silently crowned big (donghu L1..5000 real
// shape: 2 domains, cap 2.53 minted for the [4-11] middle cluster, global
// fmax 2.15× understated, ZERO degrade words), 10ms-window judgment failure
// measured at 98%.
//
// The ruling (2026-07-18, verbatim intent): a SECOND full-file streaming scan
// IS allowed — the former R6 "禁二次全文件重扫" comment is superseded — as
// long as (a) cost is controlled and (b) scanned results are REUSED, never
// repeatedly re-scanned. This file implements exactly that contract:
//
//   - SINGLE streaming pass per artifact generation
//     (completeTimestampOrderProof's O(1)-memory streaming family): every
//     line pays one O(1) raw prescreen (fullFreqCurveRawCandidate — the same
//     invariant the in-pass windowed collection already relies on), and only
//     the sparse frequency-candidate lines pay a parse. Admission and the
//     rollback order audit are the SAME fullFreqCurveCollector the in-pass
//     rule-4 collection uses, so a side-scanned curve set is sample-for-
//     sample the set a complete from-0 build would have collected.
//   - COST CAP (precise signal): freqSideScanSampleCap bounds one artifact's
//     collection (freqSample is 16 bytes — the cap bounds the retained set to
//     ≈16MB; a GB-scale real capture extrapolates to ≈570k samples, well
//     inside). Overflow is an HONEST typed degrade (the verdict itself is
//     cached so a pathological file is never re-scanned): consumers stay on
//     the events basis and the caveat lane discloses the cap degrade —
//     never a silently truncated curve set masquerading as full-file truth.
//   - REUSE (the ruling's emphasis): outcomes are cached in-process per
//     artifact GENERATION (traceAnchorKey: canonical path + size + mtime +
//     strong identity + parser version — the anchor-cache key family), true
//     LRU (hits refresh recency) under a total sample budget plus a per-lane
//     entry count cap, with an in-flight singleflight so concurrent queries
//     of one artifact share a single scan. Post-open scan FAILURES are also
//     per-generation verdicts and are cached (bounded) so a persistently
//     failing artifact is not re-streamed per Index. Cache hits and scans
//     are counted (typed observability; TestSideScanReuse pins the
//     zero-rescan contract). In-process only by design — a disk cache would
//     need multi-instance write fencing (多实例安全) for a pure latency win.
//   - IDENTITY DISCIPLINE: the scan binds to the queried Index's recorded
//     artifact generation (SameVersion before streaming) and re-validates the
//     held descriptor + pathname binding after EOF
//     (validateTraceFileIdentityAfterRead) — mixed-generation curves are
//     structurally impossible.
//   - CAP-3 red line: the side-scan basis IS the Index-global basis — it
//     reads the whole physical file and is never cropped by the query window,
//     the relation prune, or the MaxEvents budget.
//
// Consumption sits in indexFreqSampleTimelines (cluster_freq_share.go) and
// chainQueryCache.buildFreqLimitIndex (supply_fold.go): full-file curves
// first, side-scan second, events carve last — the choice is a chain of
// precise collected/degrade flags, and the active basis is disclosed via the
// typed ClusterSampleBasis token.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// Cluster-derivation sample-basis tokens (CLUSTER-FIX-1 件2 typed disclosure
// lane): WHERE the Index-global cluster-derivation / fmax sample stream came
// from. full_index is the healthy R6 norm and is disclosed BY ABSENCE
// (pre-batch wire bytes preserved — the CoreCapabilityTopology* precedent);
// the two exceptional bases mint their token on SupplyFoldBasis.
const (
	// ClusterSampleBasisFullIndex: the build's own single-pass rule-4
	// full-file curves (or the per-file anchor record's stamped copy).
	ClusterSampleBasisFullIndex = "full_index"
	// ClusterSampleBasisSideScan: the streaming full-file side-scan recovered
	// the basis (this file) — sample-identical to a complete in-pass
	// collection, Index-global by construction.
	ClusterSampleBasisSideScan = "side_scan"
	// ClusterSampleBasisWindowCarve: both full-file lanes unavailable — the
	// historical idx.Events carve basis (window faces keep their existing
	// freq_only degrade words; the caveat lane discloses WHY the side-scan
	// could not serve when one was attempted).
	ClusterSampleBasisWindowCarve = "window_carve"
)

// freqSideScanSampleCap bounds one artifact's side-scan collection — the
// ruling's 成本帽. Deliberately the same value as the in-pass defensive cap
// (fullFreqCurveSampleCap): the side-scan exists to REPLACE a missing in-pass
// collection, so it must never retain more than the in-pass lane would have.
// freqSample = 16 bytes → ≈16MB ceiling per artifact; overflow degrades
// honestly (typed verdict, cached, caveat-disclosed) — never silent.
const freqSideScanSampleCap = fullFreqCurveSampleCap

// freqSideScanCacheBudgetSamples bounds the TOTAL samples retained across all
// cached side-scan outcomes (true LRU eviction — a cache hit moves the entry
// to the back of the order, so the hottest artifacts are evicted last (裁定
// 「尽量复用」); eviction is pure latency — a miss re-scans through the
// singleflight). 4Mi samples ≈ 64MB worst case, the same budget class the
// anchor cache documents (32 files × ≈2MB).
const freqSideScanCacheBudgetSamples = 4 << 20

// freqSideScanCacheMaxEntries bounds the NUMBER of resident verdicts per cache
// lane (curve outcomes and failure verdicts each). The sample budget above
// cannot bound zero-cost entries (an overflow verdict retains no samples, a
// failure verdict retains only an error), so without a count cap a stream of
// distinct pathological artifacts would grow the maps without bound. Same
// sizing family as traceAnchorCacheMaxFiles; over the cap the least recently
// used (oldest, for the failure lane) entry is evicted.
const freqSideScanCacheMaxEntries = 32

// Side-scan degrade tokens (precise signals; "" = side-scan served). They
// feed the caveat disclosure lane only — never a gate.
const (
	// freqSideScanDegradeOverflow: the sample cap tripped — the artifact's
	// frequency lanes are pathologically dense; the verdict is cached so the
	// file is not re-scanned per query.
	freqSideScanDegradeOverflow = "sample_cap_overflow"
	// freqSideScanDegradeScanFailed: the physical scan failed (open/read
	// error, or the artifact generation changed since indexing).
	freqSideScanDegradeScanFailed = "scan_failed"
	// freqSideScanDegradePerfChild: a causally-admitted perf-kind artifact is
	// part of the queried source universe — a composite child, or the
	// directly queried .perftrace itself — parity with the in-pass composite
	// merge, whose event set passes a typed admission the side curves cannot
	// (the side-scan must never be MORE permissive than the in-pass
	// collection).
	freqSideScanDegradePerfChild = "perf_artifact_present"
	// freqSideScanDegradeUnmappable: a composite child's samples could not be
	// mapped into the canonical clock domain (an over-cap merged set is the
	// separate typed overflow verdict — see collectIndexSideScanFreqCurves).
	freqSideScanDegradeUnmappable = "clock_unmappable"
	// freqSideScanDegradeNoArtifact: the Index records no causally-compatible
	// physical artifact to scan (synthetic/in-memory indices).
	freqSideScanDegradeNoArtifact = "no_artifact"
)

// freqSideScanDegradeZH maps a degrade token to its zh display label (token +
// label ride side by side in the caveat so the line stays greppable AND
// readable — the freqCoMoveSplitArmZH house pattern).
func freqSideScanDegradeZH(token string) string {
	switch token {
	case freqSideScanDegradeOverflow:
		return "频点样本数超成本帽"
	case freqSideScanDegradeScanFailed:
		return "物理扫描失败或文件代际已变更"
	case freqSideScanDegradePerfChild:
		return "含 perf 类工件(组合体或直查)"
	case freqSideScanDegradeUnmappable:
		return "子工件时钟无法映射到公共钟域"
	case freqSideScanDegradeNoArtifact:
		return "无可扫描的物理工件"
	default:
		return ""
	}
}

// freqSideScanArtifact is one artifact generation's cached outcome, in the
// artifact's OWN clock domain (composite consumers map at merge time, so one
// cache entry serves both direct-file and bundle queries of the artifact).
// curves maps are READ-ONLY BY CONTRACT once cached.
//
// CACHE-CONTENT BOUNDARY (user ruling 2026-07-18, pinned by
// TestClusterFix1ArtifactCacheStoresRawScanContentOnly): this cache stores the
// RAW scanned content only — the collected curve set plus the typed cost-cap
// verdict, exactly what the physical scan observed. It must NEVER grow a
// derived-conclusion field (cluster domains, fmax, class labels, R5 bases…):
// cross-question derived recomputation is deliberately per-query local
// (chainQueryCache, newChainQueryCache — one cache per query), so a stale
// derived verdict can never outlive the question that minted it.
type freqSideScanArtifact struct {
	curves     fullFreqCurves
	overflowed bool
}

type freqSideScanFlight struct {
	done  chan struct{}
	entry *freqSideScanArtifact
	err   error
}

// freqSideScanGenerationError marks a scan failure observed AFTER the
// artifact generation was successfully opened and identity-bound — a
// per-generation verdict eligible for the failure-verdict cache lane (件7;
// see freqSideScanCache.errItems). Failures to even open the path stay
// unwrapped and uncached.
type freqSideScanGenerationError struct{ err error }

func (e freqSideScanGenerationError) Error() string { return e.err.Error() }
func (e freqSideScanGenerationError) Unwrap() error { return e.err }

type freqSideScanCache struct {
	mu       sync.Mutex
	items    map[traceAnchorKey]*freqSideScanArtifact
	order    []traceAnchorKey
	samples  int
	inflight map[traceAnchorKey]*freqSideScanFlight
	// errItems/errOrder: the failure-verdict lane (件7). A scan failure
	// observed AFTER the artifact generation was opened (generation mismatch,
	// mid-read I/O error, post-EOF identity change) is a verdict ABOUT that
	// generation: the strong identity ledger (size/mtime/dev/inode/ctime)
	// cannot be restored once it moved (ctime is kernel-owned), so the cached
	// refusal stays correct and a persistently failing large artifact is not
	// re-streamed by every later Index of the same generation. A transient
	// mid-read error therefore also sticks for the generation (in-process
	// only) — an HONEST scan_failed degrade, never a wrong answer. Plain open
	// failures are deliberately NOT cached: they are one cheap syscall to
	// retry and include transient environmental classes (EMFILE). FIFO under
	// freqSideScanCacheMaxEntries (verdicts are zero-cost; only their count
	// needs bounding).
	errItems map[traceAnchorKey]error
	errOrder []traceAnchorKey
	// hits/scans: reuse observability (件1 复用命中可观测; test-pinned).
	hits  int
	scans int
}

var sideScanCache = &freqSideScanCache{
	items:    map[traceAnchorKey]*freqSideScanArtifact{},
	inflight: map[traceAnchorKey]*freqSideScanFlight{},
	errItems: map[traceAnchorKey]error{},
}

func (c *freqSideScanCache) counters() (scans, hits int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scans, c.hits
}

// touchLocked moves key to the back of the LRU order (most recently used).
// Called on every cache hit so the budget/count eviction below always evicts
// the COLDEST verdict first — the ruling's 「尽量复用」 kept honest: the
// hottest artifacts survive eviction pressure longest.
func (c *freqSideScanCache) touchLocked(key traceAnchorKey) {
	for i, k := range c.order {
		if k == key {
			c.order = append(append(c.order[:i], c.order[i+1:]...), key)
			return
		}
	}
}

// storeLocked inserts at the back of the LRU order under both bounds: the
// sample budget (an entry above the whole budget is still cached as a single
// resident so the SAME artifact is not re-scanned back-to-back; the next
// insert evicts it normally) and the entry count cap (zero-cost verdicts are
// invisible to the sample budget). Eviction always takes the LRU front.
func (c *freqSideScanCache) storeLocked(key traceAnchorKey, entry *freqSideScanArtifact) {
	if _, exists := c.items[key]; exists {
		c.touchLocked(key)
		return
	}
	evictOldest := func() {
		oldest := c.order[0]
		c.order = c.order[1:]
		if old, ok := c.items[oldest]; ok {
			c.samples -= old.curves.samples
			delete(c.items, oldest)
		}
	}
	cost := entry.curves.samples
	for len(c.order) > 0 && c.samples+cost > freqSideScanCacheBudgetSamples {
		evictOldest()
	}
	c.items[key] = entry
	c.order = append(c.order, key)
	c.samples += cost
	for len(c.order) > freqSideScanCacheMaxEntries {
		evictOldest()
	}
}

// storeErrLocked records a per-generation failure verdict (bounded FIFO; see
// the errItems field contract for what qualifies).
func (c *freqSideScanCache) storeErrLocked(key traceAnchorKey, err error) {
	if _, exists := c.errItems[key]; exists {
		return
	}
	c.errItems[key] = err
	c.errOrder = append(c.errOrder, key)
	for len(c.errOrder) > freqSideScanCacheMaxEntries {
		oldest := c.errOrder[0]
		c.errOrder = c.errOrder[1:]
		delete(c.errItems, oldest)
	}
}

// sideScanArtifactFreqCurves returns the artifact generation's side-scan
// outcome: cache hit, singleflight join, or one streaming scan. expected is
// the queried Index's recorded generation — a mismatched physical file fails
// the scan (never mixed-generation curves).
func sideScanArtifactFreqCurves(path string, expected traceFileIdentity) (*freqSideScanArtifact, error) {
	if strings.TrimSpace(path) == "" || !expected.Initialized() {
		return nil, fmt.Errorf("frequency side-scan: artifact path/identity unavailable")
	}
	key := traceAnchorKeyForIdentity(path, expected)
	c := sideScanCache
	c.mu.Lock()
	if entry, ok := c.items[key]; ok {
		c.hits++
		c.touchLocked(key)
		c.mu.Unlock()
		return entry, nil
	}
	if cachedErr, ok := c.errItems[key]; ok {
		// 件7: the generation's failure verdict is cached — do not re-stream a
		// persistently failing artifact per Index (reuse observability counts
		// this as a hit like any other served verdict).
		c.hits++
		c.mu.Unlock()
		return nil, cachedErr
	}
	if fl, ok := c.inflight[key]; ok {
		c.hits++
		c.mu.Unlock()
		<-fl.done
		return fl.entry, fl.err
	}
	fl := &freqSideScanFlight{done: make(chan struct{})}
	c.inflight[key] = fl
	c.scans++
	c.mu.Unlock()
	entry, err := streamFreqSideScan(path, expected, freqSideScanSampleCap)
	fl.entry, fl.err = entry, err
	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil {
		c.storeLocked(key, entry)
	} else {
		var generationErr freqSideScanGenerationError
		if errors.As(err, &generationErr) {
			c.storeErrLocked(key, err)
		}
	}
	c.mu.Unlock()
	close(fl.done)
	return entry, err
}

// streamFreqSideScan is the single streaming pass: O(1) raw prescreen per
// line, parse + collect on the sparse frequency candidates only, full
// identity discipline before and after the read. sampleCap is the 成本帽
// (production: freqSideScanSampleCap; tests drive the overflow arm with a
// small cap).
func streamFreqSideScan(path string, expected traceFileIdentity, sampleCap int) (entry *freqSideScanArtifact, err error) {
	// The scan is bounded by the physical file size and runs at most once per
	// artifact generation (singleflight + cache above); the deep query-lane
	// call sites carry no context, so the stream binds to the background one.
	// The artifact was already text-admitted when its Index was built, and the
	// SameVersion check below binds this scan to that exact generation.
	f, openedIdentity, err := openTraceSourceRegularContext(context.Background(), path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			entry, err = nil, fmt.Errorf("close frequency side-scan source %s: %w", path, closeErr)
		}
	}()
	if !expected.SameVersion(openedIdentity) {
		// Generation-final (件7): the queried generation's strong identity
		// ledger (ctime included) can never be restored, so this verdict is
		// cacheable under the generation key.
		return nil, freqSideScanGenerationError{fmt.Errorf("frequency side-scan: artifact generation differs from the queried index")}
	}
	frozen, err := frozenTraceSectionAtCurrentOffset(f, openedIdentity)
	if err != nil {
		return nil, freqSideScanGenerationError{err}
	}
	r := bufio.NewReaderSize(frozen, 256*1024)
	collector := newFullFreqCurveCollector(path)
	intern := newStringInterner()
	scratch := &Index{}
	var scan lineScan
	for lineNo := 1; ; lineNo++ {
		line, readErr := readStreamScanPhysicalLine(r, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if fullFreqCurveRawCandidate(trimmed) {
				scan.reset(lineNo, trimmed)
				if ev, ok := safeParseLineScan(&scan, intern, scratch); ok {
					collector.observe(ev)
					if collector.overflowed || collector.curves.samples > sampleCap {
						// 成本帽: the typed overflow verdict is final for this
						// generation — stop reading (identity is re-validated
						// by stat below, no need to stream the remainder).
						collector.overflowed = true
						break
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, freqSideScanGenerationError{readErr}
		}
	}
	if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "frequency side-scan"); err != nil {
		return nil, freqSideScanGenerationError{err}
	}
	if collector.overflowed {
		return &freqSideScanArtifact{overflowed: true}, nil
	}
	return &freqSideScanArtifact{curves: collector.finalize(true)}, nil
}

// sideScanFreqTimelines assembles (memoized once per Index) the side-scan
// full-file curve set for this Index's artifact universe. degrade is "" on
// success; otherwise one of the freqSideScanDegrade* tokens (curves zero).
func (idx *Index) sideScanFreqTimelines() (fullFreqCurves, string) {
	if idx == nil {
		return fullFreqCurves{}, freqSideScanDegradeNoArtifact
	}
	idx.sideFreqOnce.Do(func() {
		idx.sideFreq, idx.sideFreqDegrade = collectIndexSideScanFreqCurves(idx)
	})
	return idx.sideFreq, idx.sideFreqDegrade
}

func collectIndexSideScanFreqCurves(idx *Index) (fullFreqCurves, string) {
	// Roster mirrors the in-pass composite merge admission exactly:
	// causally-incompatible artifacts are excluded by the provenance gate
	// (their samples must not enter the shared causal timeline — no degrade),
	// while an ADMITTED perf-kind child refuses the whole side collection
	// (parity: parse.go composite merge).
	var eligible []TraceArtifactSource
	for _, source := range idx.TraceArtifacts {
		if !source.CausalCompatible {
			continue
		}
		if strings.EqualFold(source.Kind, "perftrace") {
			return fullFreqCurves{}, freqSideScanDegradePerfChild
		}
		eligible = append(eligible, source)
	}
	if len(eligible) == 0 {
		return fullFreqCurves{}, freqSideScanDegradeNoArtifact
	}
	if len(eligible) == 1 && eligible[0].ClockAlignment != TraceClockAlignmentAffine {
		// Single identity-aligned artifact (the common single-file form):
		// share the cached curve set directly (READ-ONLY BY CONTRACT).
		entry, err := sideScanArtifactFreqCurves(eligible[0].SourcePath, eligible[0].sourceIdentity)
		if err != nil {
			return fullFreqCurves{}, freqSideScanDegradeScanFailed
		}
		if entry.overflowed {
			return fullFreqCurves{}, freqSideScanDegradeOverflow
		}
		return entry.curves, ""
	}
	dst := fullFreqCurves{
		collected:  true,
		freqByCPU:  map[int][]freqSample{},
		limitByCPU: map[int][]freqSample{},
	}
	for _, source := range eligible {
		entry, err := sideScanArtifactFreqCurves(source.SourcePath, source.sourceIdentity)
		if err != nil {
			return fullFreqCurves{}, freqSideScanDegradeScanFailed
		}
		if entry.overflowed {
			return fullFreqCurves{}, freqSideScanDegradeOverflow
		}
		mergeCompositeFullFreqCurves(&dst, entry.curves, source)
		// Post-merge 成本帽 re-check (件4): the per-artifact cap bounds each
		// child, but the MERGED set is what this Index retains — an over-cap
		// union is the typed overflow verdict, checked BEFORE the collected
		// flag because the merge folds its own over-cap detection into
		// collected=false and would otherwise misreport it as a clock issue.
		if dst.samples > freqSideScanSampleCap {
			return fullFreqCurves{}, freqSideScanDegradeOverflow
		}
		if !dst.collected {
			return fullFreqCurves{}, freqSideScanDegradeUnmappable
		}
	}
	finalizeCompositeFullFreqCurves(&dst)
	return dst, ""
}
