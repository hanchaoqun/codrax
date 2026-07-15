package tracequery

// full_freq_curves.go — R6 rule 4 (§29.88.9 CLUSTER-DERIVE, user ruling
// 2026-07-14, docs/design/real_trace_campaign_20260705.md): FULL-FILE per-CPU
// frequency curves, collected in the SAME single BuildIndex forward pass that
// parses the trace (禁二次全文件重扫).
//
// Why the Index event set is NOT a sufficient basis: idx.Events is a CARVE of
// the raw file — the windowed ts gate skips out-of-window lines, the
// relation-scope prune drops non-relation events, and the MaxEvents budget
// bounds admission. Every frequency-DERIVED quantity (cluster membership,
// per-cluster fmax, the R5 全域最大核最高频点 conversion basis) is a
// TRACE-GLOBAL attribute per the ruling: deriving it from a cropped sample
// stream both fragments clusters at carve boundaries (the CAP-3 §29.11
// mechanical shape) and systematically under-states fmax when the head/tail
// of the frequency history falls outside the carve (R5 缺口系统性偏小).
//
// Collection discipline:
//
//   - SAME pass: the scan loop hands every parsed event to the collector
//     BEFORE the window-skip / relation-prune / MaxEvents admission gates.
//     Out-of-window lines are parsed for collection only when the O(1) raw
//     prescreen (fullFreqCurveRawCandidate) hits — frequency rows are sparse,
//     so windowed builds pay one strings.Contains per line plus a parse on
//     the sparse hits only (single-pass, never O(cores×events)).
//   - SAME admission predicates as every other frequency basis:
//     isPerCPUFrequencySample / isPerCPULimitSample (cluster_ceilings.go) —
//     no second membership judgment may grow beside them.
//   - COMPLETENESS is a precise flag: curves publish (collected=true) only
//     when the scan covered the whole file — from byte 0 (no anchor seek) to
//     EOF (no early stop, no padding truncation) — and the defensive sample
//     cap did not trip. A partial scan publishes NOTHING and every consumer
//     falls back to the historical idx.Events basis byte-identically
//     (absence never guesses; a partial curve set masquerading as full-file
//     truth would be exactly the cropped-basis disease this file removes).
//   - ORDER INTEGRITY mirrors the frequency fail-close discipline
//     (frequencyOrderIntegrity): a physical same-lane timestamp rollback
//     poisons that CPU's curve for its lane; finalize drops poisoned CPUs
//     entirely. The full-scan sample set is a superset (same physical order)
//     of the idx.Events set, so this audit is at least as strong as the
//     events-basis audit it replaces on the consuming faces.
//   - PER-FILE REUSE (trace attribute, write-once): a complete collection is
//     stamped into the per-file anchor record (traceAnchorSet.FullFreq) under
//     the same write-once rule as the flavor/platform records, so a later
//     anchor-seek or early-stop windowed build of the SAME file consumes the
//     full-file curves instead of falling back to its cropped event set.
//     Bounded by fullFreqCurveAnchorSampleCap (larger collections stay on the
//     building index only).
//
// Consumers (single accessor pair below): indexFreqSampleTimelines
// (cluster_freq_share.go — cluster derivation + every window-face membership)
// and chainQueryCache.buildFreqLimitIndex (supply_fold.go — the fmax ladder's
// limits rung). Window-DISPLAY calibers (in-window residency, the strict
// in-window CPUFrequencyLimits display accumulation) are deliberately NOT
// consumers: rule 4 governs derivation quantities, not window display facts.

import (
	"sort"
	"strings"
)

// fullFreqCurveSampleCap bounds one index's full-file curve collection
// (defensive: DVFS transition rows are sparse in real captures; a
// pathological multi-million-row frequency lane falls back to the historical
// events basis instead of holding an unbounded side allocation).
const fullFreqCurveSampleCap = 1 << 20

// fullFreqCurveAnchorSampleCap bounds the per-file anchor-record stamp (the
// anchor cache holds up to traceAnchorCacheMaxFiles files; 128Ki samples ≈
// 2MB keeps the worst case bounded).
const fullFreqCurveAnchorSampleCap = 1 << 17

// fullFreqCurves is the published full-file curve set. collected=false is the
// precise "no full-file coverage" signal — consumers then use their
// historical idx.Events basis. Maps are READ-ONLY BY CONTRACT once published
// (shared across derived indices and the anchor record).
type fullFreqCurves struct {
	collected  bool
	samples    int
	freqByCPU  map[int][]freqSample
	limitByCPU map[int][]freqSample
}

// fullFreqCurveRawCandidate is the O(1) prescreen for out-of-window lines:
// every raw name that can classify to EventCPUFrequency /
// EventCPUFrequencyLimit contains "freq" (cpu_frequency, cpu_frequency_limits,
// the generalized cpu+freq shapes; the clock_set_rate reclassification is
// excluded from per-CPU curves by isPerCPUFrequencySample regardless).
func fullFreqCurveRawCandidate(line string) bool {
	return strings.Contains(line, "freq")
}

// fullFreqCurveCollector accumulates one scan's collection. Zero value not
// usable — newFullFreqCurveCollector.
type fullFreqCurveCollector struct {
	curves      fullFreqCurves
	freqLastTs  map[int]float64
	limitLastTs map[int]float64
	freqUnsafe  map[int]bool
	limitUnsafe map[int]bool
	overflowed  bool
}

func newFullFreqCurveCollector() *fullFreqCurveCollector {
	return &fullFreqCurveCollector{
		curves: fullFreqCurves{
			freqByCPU:  map[int][]freqSample{},
			limitByCPU: map[int][]freqSample{},
		},
		freqLastTs:  map[int]float64{},
		limitLastTs: map[int]float64{},
		freqUnsafe:  map[int]bool{},
		limitUnsafe: map[int]bool{},
	}
}

// observe feeds one parsed event in physical scan order. Cheap on non-
// frequency events (one type switch).
func (c *fullFreqCurveCollector) observe(ev Event) {
	if c == nil || c.overflowed {
		return
	}
	switch ev.Type {
	case EventCPUFrequency:
		if !isPerCPUFrequencySample(ev) {
			return
		}
		cpu := eventCPUForStats(ev)
		if last, seen := c.freqLastTs[cpu]; seen && ev.Ts < last {
			// Physical same-lane rollback: fail-close this CPU's curve
			// (frequencyOrderIntegrity discipline).
			c.freqUnsafe[cpu] = true
		}
		c.freqLastTs[cpu] = ev.Ts
		c.curves.freqByCPU[cpu] = append(c.curves.freqByCPU[cpu], freqSample{ts: ev.Ts, khz: ev.Frequency})
	case EventCPUFrequencyLimit:
		cpu, ok := isPerCPULimitSample(ev)
		if !ok {
			return
		}
		if last, seen := c.limitLastTs[cpu]; seen && ev.Ts < last {
			c.limitUnsafe[cpu] = true
		}
		c.limitLastTs[cpu] = ev.Ts
		c.curves.limitByCPU[cpu] = append(c.curves.limitByCPU[cpu], freqSample{ts: ev.Ts, khz: ev.FrequencyMax})
	default:
		return
	}
	c.curves.samples++
	if c.curves.samples > fullFreqCurveSampleCap {
		c.overflowed = true
	}
}

// finalize publishes the collection. complete=false (seeked head, early stop,
// padding truncation, no EOF) or an overflow publishes the zero value —
// consumers fall back to the events basis.
func (c *fullFreqCurveCollector) finalize(complete bool) fullFreqCurves {
	if c == nil || !complete || c.overflowed {
		return fullFreqCurves{}
	}
	for cpu := range c.freqUnsafe {
		delete(c.curves.freqByCPU, cpu)
	}
	for cpu := range c.limitUnsafe {
		delete(c.curves.limitByCPU, cpu)
	}
	c.curves.collected = true
	return c.curves
}

// fullFrequencyTimelines returns the full-file cpu_frequency curves when the
// rule-4 collection covered the whole file (poisoned CPUs already dropped).
// ok=false → consumers use their historical basis.
func (idx *Index) fullFrequencyTimelines() (map[int][]freqSample, bool) {
	if idx == nil || !idx.fullFreq.collected {
		return nil, false
	}
	return idx.fullFreq.freqByCPU, true
}

// fullFrequencyLimitTimelines is the cpu_frequency_limits twin.
func (idx *Index) fullFrequencyLimitTimelines() (map[int][]freqSample, bool) {
	if idx == nil || !idx.fullFreq.collected {
		return nil, false
	}
	return idx.fullFreq.limitByCPU, true
}

// mergeCompositeFullFreqCurves maps one child's complete curves into the
// composite's canonical clock domain (bundle path). Any unmappable sample or
// a cap overflow degrades the composite (collected=false, fail-open to the
// events basis) — a partially-merged set must never claim full coverage.
func mergeCompositeFullFreqCurves(dst *fullFreqCurves, child fullFreqCurves, source TraceArtifactSource) {
	mapLane := func(dstLane map[int][]freqSample, childLane map[int][]freqSample) bool {
		for cpu, tl := range childLane {
			for _, s := range tl {
				mapped, ok := source.toCanonicalTsChecked(s.ts)
				if !ok {
					return false
				}
				dstLane[cpu] = append(dstLane[cpu], freqSample{ts: mapped, khz: s.khz})
				dst.samples++
			}
		}
		return true
	}
	if !mapLane(dst.freqByCPU, child.freqByCPU) || !mapLane(dst.limitByCPU, child.limitByCPU) || dst.samples > fullFreqCurveSampleCap {
		dst.collected = false
	}
}

// finalizeCompositeFullFreqCurves canonically orders the merged curves
// (mirrors the composite event sort: ts ascending; per-child order is
// preserved by the stable sort for equal timestamps).
func finalizeCompositeFullFreqCurves(dst *fullFreqCurves) {
	for _, lane := range []map[int][]freqSample{dst.freqByCPU, dst.limitByCPU} {
		for cpu := range lane {
			tl := lane[cpu]
			sort.SliceStable(tl, func(i, j int) bool { return tl[i].ts < tl[j].ts })
		}
	}
}
