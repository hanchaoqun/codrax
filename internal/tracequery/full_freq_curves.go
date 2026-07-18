package tracequery

// full_freq_curves.go — R6 rule 4 (§29.88.9 CLUSTER-DERIVE, user ruling
// 2026-07-14, docs/design/real_trace_campaign_20260705.md): FULL-FILE per-CPU
// frequency curves, collected in the SAME single BuildIndex forward pass that
// parses the trace. (The original "禁二次全文件重扫" clause here is SUPERSEDED
// by the user ruling 2026-07-18, CLUSTER-FIX-1: when this in-pass collection
// cannot cover the file, a SECOND bounded streaming side-scan is allowed —
// cost-capped and cached per artifact generation so nothing is repeatedly
// re-scanned; see freq_side_scan.go. The in-pass lane below remains the
// preferred zero-extra-IO collection.)
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
//   - SAME typed transition decoders as every governed frequency basis:
//     perCPUFrequencyTransitionValues / perCPULimitSampleValues
//     (cluster_ceilings.go). Canonical zero transitions stay in the state
//     curve only to cut carry-in; hard sample/fmax roles apply their positive
//     gate separately. No second membership judgment may grow beside them.
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
	"strconv"
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
	// Poison is part of the full-file authority, not a transient collector
	// detail. A later causally admitted child must not resurrect a CPU lane
	// that an earlier child proved malformed. These maps are read-only after
	// publication, like the sample maps above.
	freqUnsafe  map[int]bool
	limitUnsafe map[int]bool
	freqAll     bool
	limitAll    bool
	// The first physical witness for every poison verdict travels with the
	// receipt through anchor reuse, derived windows and composite rebasing.
	// Boolean-only poison is sufficient to suppress a lane, but not to explain
	// which child/file/line caused the global derivation to fail closed.
	freqPoisonByCPU   map[int]durationOrderViolation
	limitPoisonByCPU  map[int]durationOrderViolation
	freqAllPoison     durationOrderViolation
	limitAllPoison    durationOrderViolation
	freqAllPoisonSet  bool
	limitAllPoisonSet bool
	// droppedFreqCPUs (CLUSTER-FIX-1 件3, S4 收披露): sorted cpu_frequency
	// lanes the poison application REMOVED from freqByCPU. The drop itself is
	// the long-standing fail-close judgment (unchanged); the roster exists so
	// the disclosure lane can say a cluster count may be understated when a
	// dropped lane was a cluster's only sampled member. Disclosure input
	// only, never a gate. Recorded in applyFullFreqCurvePoison so the in-pass
	// finalize and the composite finalize share one recording point.
	droppedFreqCPUs []int
}

// fullFreqCurveRawCandidate is the O(1) prescreen for out-of-window lines:
// every raw name that can classify to EventCPUFrequency /
// EventCPUFrequencyLimit contains "freq" (cpu_frequency, cpu_frequency_limits,
// the generalized cpu+freq shapes; the clock_set_rate reclassification is
// excluded from per-CPU curves by perCPUFrequencyTransitionValues regardless).
func fullFreqCurveRawCandidate(line string) bool {
	return containsASCIIFold(line, "freq")
}

func containsASCIIFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// fullFreqCurveCollector accumulates one scan's collection. Zero value not
// usable — newFullFreqCurveCollector.
type fullFreqCurveCollector struct {
	curves      fullFreqCurves
	freqLastTs  map[int]float64
	limitLastTs map[int]float64
	sourcePath  string
	overflowed  bool
}

func newFullFreqCurveCollector(sourcePath string) *fullFreqCurveCollector {
	return &fullFreqCurveCollector{
		curves: fullFreqCurves{
			freqByCPU:        map[int][]freqSample{},
			limitByCPU:       map[int][]freqSample{},
			freqUnsafe:       map[int]bool{},
			limitUnsafe:      map[int]bool{},
			freqPoisonByCPU:  map[int]durationOrderViolation{},
			limitPoisonByCPU: map[int]durationOrderViolation{},
		},
		freqLastTs:  map[int]float64{},
		limitLastTs: map[int]float64{},
		sourcePath:  sourcePath,
	}
}

// observe feeds one parsed event in physical scan order. Cheap on non-
// frequency events (one type switch).
func (c *fullFreqCurveCollector) observe(ev Event) {
	if c == nil || c.overflowed {
		return
	}
	// A malformed state transition is a carry-in barrier even though it is
	// intentionally excluded from the sample timeline. Preserve its precise
	// payload CPU when known; without one, fail-close only the affected family.
	if failure := durationEndpointValidationFailureFromEvent(ev); failure != nil {
		if failure.SourcePath == "" {
			failure.SourcePath = c.sourcePath
		}
		if c.observeFailure(failure) {
			return
		}
	}
	switch ev.Type {
	case EventCPUFrequency:
		cpu, khz, ok := perCPUFrequencyTransitionValues(ev)
		if !ok {
			return
		}
		if last, seen := c.freqLastTs[cpu]; seen && ev.Ts < last {
			// Physical same-lane rollback: fail-close this CPU's curve and retain
			// its exact source coordinate in the reusable receipt.
			c.observeFailure(&durationOrderViolation{
				Family:  durationOrderCPUFrequency,
				Lane:    durationOrderLaneLabel(durationOrderLane{family: durationOrderCPUFrequency, key: strconv.Itoa(cpu)}),
				LaneKey: strconv.Itoa(cpu), EventName: ev.Name,
				PreviousTs: last, CurrentTs: ev.Ts, Line: ev.Line, SourcePath: c.sourcePath,
			})
		}
		c.freqLastTs[cpu] = ev.Ts
		c.curves.freqByCPU[cpu] = append(c.curves.freqByCPU[cpu], freqSample{ts: ev.Ts, khz: khz})
	case EventCPUFrequencyLimit:
		cpu, _, maxKHz, ok := perCPULimitSampleValues(ev)
		if !ok {
			return
		}
		if last, seen := c.limitLastTs[cpu]; seen && ev.Ts < last {
			c.observeFailure(&durationOrderViolation{
				Family:  durationOrderCPUFreqLimit,
				Lane:    durationOrderLaneLabel(durationOrderLane{family: durationOrderCPUFreqLimit, key: strconv.Itoa(cpu)}),
				LaneKey: strconv.Itoa(cpu), EventName: ev.Name,
				PreviousTs: last, CurrentTs: ev.Ts, Line: ev.Line, SourcePath: c.sourcePath,
			})
		}
		c.limitLastTs[cpu] = ev.Ts
		c.curves.limitByCPU[cpu] = append(c.curves.limitByCPU[cpu], freqSample{ts: ev.Ts, khz: maxKHz})
	default:
		return
	}
	c.curves.samples++
	if c.curves.samples > fullFreqCurveSampleCap {
		c.overflowed = true
	}
}

func (c *fullFreqCurveCollector) observeFailure(failure *durationOrderViolation) bool {
	if c == nil || failure == nil {
		return false
	}
	copy := *failure
	copy.Fields = append([]string(nil), failure.Fields...)
	if copy.SourcePath == "" {
		copy.SourcePath = c.sourcePath
	}
	if !recordFullFreqCurvePoison(&c.curves, copy) {
		return false
	}
	return true
}

func recordFullFreqCurvePoison(curves *fullFreqCurves, failure durationOrderViolation) bool {
	if curves == nil {
		return false
	}
	var unsafe map[int]bool
	var witnesses map[int]durationOrderViolation
	var all *bool
	var allWitness *durationOrderViolation
	var allWitnessSet *bool
	switch failure.Family {
	case durationOrderCPUFrequency:
		unsafe, witnesses = curves.freqUnsafe, curves.freqPoisonByCPU
		all, allWitness, allWitnessSet = &curves.freqAll, &curves.freqAllPoison, &curves.freqAllPoisonSet
	case durationOrderCPUFreqLimit:
		unsafe, witnesses = curves.limitUnsafe, curves.limitPoisonByCPU
		all, allWitness, allWitnessSet = &curves.limitAll, &curves.limitAllPoison, &curves.limitAllPoisonSet
	default:
		return false
	}
	if unsafe == nil {
		unsafe = map[int]bool{}
		if failure.Family == durationOrderCPUFrequency {
			curves.freqUnsafe = unsafe
		} else {
			curves.limitUnsafe = unsafe
		}
	}
	if witnesses == nil {
		witnesses = map[int]durationOrderViolation{}
		if failure.Family == durationOrderCPUFrequency {
			curves.freqPoisonByCPU = witnesses
		} else {
			curves.limitPoisonByCPU = witnesses
		}
	}
	failure.Fields = append([]string(nil), failure.Fields...)
	cpu, err := strconv.Atoi(failure.LaneKey)
	if err == nil && cpu >= 0 {
		unsafe[cpu] = true
		if _, exists := witnesses[cpu]; !exists {
			witnesses[cpu] = failure
		}
		return true
	}
	*all = true
	if !*allWitnessSet {
		*allWitness = failure
		*allWitnessSet = true
	}
	return true
}

// finalize publishes the collection. complete=false (seeked head, early stop,
// padding truncation, no EOF) or an overflow publishes the zero value —
// consumers fall back to the events basis.
func (c *fullFreqCurveCollector) finalize(complete bool) fullFreqCurves {
	if c == nil || !complete || c.overflowed {
		return fullFreqCurves{}
	}
	applyFullFreqCurvePoison(&c.curves)
	c.curves.collected = true
	return c.curves
}

func applyFullFreqCurvePoison(curves *fullFreqCurves) {
	if curves == nil {
		return
	}
	for cpu := range curves.freqUnsafe {
		if _, present := curves.freqByCPU[cpu]; present {
			// CLUSTER-FIX-1 件3 (S4 收披露): record the dropped lane so the
			// possible cluster-count understatement is disclosed downstream
			// (judgment unchanged — the drop stays exactly the fail-close
			// poison suppression). limits lanes deliberately unrecorded
			// (§29.129 备案: limits do not join cluster-membership judgment).
			if !containsInt(curves.droppedFreqCPUs, cpu) {
				curves.droppedFreqCPUs = append(curves.droppedFreqCPUs, cpu)
			}
		}
		delete(curves.freqByCPU, cpu)
	}
	for cpu := range curves.limitUnsafe {
		delete(curves.limitByCPU, cpu)
	}
	if curves.freqAll {
		for cpu := range curves.freqByCPU {
			if !containsInt(curves.droppedFreqCPUs, cpu) {
				curves.droppedFreqCPUs = append(curves.droppedFreqCPUs, cpu)
			}
		}
		curves.freqByCPU = map[int][]freqSample{}
	}
	if curves.limitAll {
		curves.limitByCPU = map[int][]freqSample{}
	}
	sort.Ints(curves.droppedFreqCPUs)
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
	if dst.freqUnsafe == nil {
		dst.freqUnsafe = map[int]bool{}
	}
	if dst.limitUnsafe == nil {
		dst.limitUnsafe = map[int]bool{}
	}
	if dst.freqPoisonByCPU == nil {
		dst.freqPoisonByCPU = map[int]durationOrderViolation{}
	}
	if dst.limitPoisonByCPU == nil {
		dst.limitPoisonByCPU = map[int]durationOrderViolation{}
	}
	for cpu := range child.freqUnsafe {
		dst.freqUnsafe[cpu] = true
	}
	for cpu := range child.limitUnsafe {
		dst.limitUnsafe[cpu] = true
	}
	dst.freqAll = dst.freqAll || child.freqAll
	dst.limitAll = dst.limitAll || child.limitAll
	mapWitness := func(failure durationOrderViolation) durationOrderViolation {
		failure.Fields = append([]string(nil), failure.Fields...)
		localLine := failure.Line
		if failure.LocalLine > 0 {
			localLine = failure.LocalLine
		}
		failure.LocalLine = localLine
		failure.Line = localLine + source.VirtualLineBase
		failure.SourcePath = source.SourcePath
		if failure.TsUnknown {
			return failure
		}
		if failure.Issue == "endpoint_parse_incomplete" {
			if mapped, ok := source.toCanonicalTsChecked(failure.CurrentTs); ok {
				failure.CurrentTs = mapped
			} else {
				failure.CurrentTs = 0
				failure.TsUnknown = true
			}
			return failure
		}
		previous, previousOK := source.toCanonicalTsChecked(failure.PreviousTs)
		current, currentOK := source.toCanonicalTsChecked(failure.CurrentTs)
		if previousOK && currentOK {
			failure.PreviousTs, failure.CurrentTs = previous, current
			return failure
		}
		// The poison verdict remains safe when a child timestamp cannot be
		// represented in the canonical domain; only its time coordinate is
		// unavailable. Preserve physical source/line provenance and render the
		// receipt form rather than fabricating a mapped rollback boundary.
		failure.Issue = "full_file_curve_poison_receipt"
		failure.Lane = "full_file_curve_poison_receipt"
		failure.PreviousTs, failure.CurrentTs, failure.TsUnknown = 0, 0, true
		return failure
	}
	for cpu, witness := range child.freqPoisonByCPU {
		if _, exists := dst.freqPoisonByCPU[cpu]; !exists {
			dst.freqPoisonByCPU[cpu] = mapWitness(witness)
		}
	}
	for cpu, witness := range child.limitPoisonByCPU {
		if _, exists := dst.limitPoisonByCPU[cpu]; !exists {
			dst.limitPoisonByCPU[cpu] = mapWitness(witness)
		}
	}
	if child.freqAllPoisonSet && !dst.freqAllPoisonSet {
		dst.freqAllPoison = mapWitness(child.freqAllPoison)
		dst.freqAllPoisonSet = true
	}
	if child.limitAllPoisonSet && !dst.limitAllPoisonSet {
		dst.limitAllPoison = mapWitness(child.limitAllPoison)
		dst.limitAllPoisonSet = true
	}

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
	// CLUSTER-FIX-1 件3: carry the children's integrity-dropped lanes into the
	// composite roster (union; disclosure input only — the caller sorts after
	// the final merge).
	for _, cpu := range child.droppedFreqCPUs {
		if !containsInt(dst.droppedFreqCPUs, cpu) {
			dst.droppedFreqCPUs = append(dst.droppedFreqCPUs, cpu)
		}
	}
}

func containsInt(in []int, v int) bool {
	for _, x := range in {
		if x == v {
			return true
		}
	}
	return false
}

// finalizeCompositeFullFreqCurves canonically orders the merged curves
// (mirrors the composite event sort: ts ascending; per-child order is
// preserved by the stable sort for equal timestamps).
func finalizeCompositeFullFreqCurves(dst *fullFreqCurves) {
	applyFullFreqCurvePoison(dst)
	for _, lane := range []map[int][]freqSample{dst.freqByCPU, dst.limitByCPU} {
		for cpu := range lane {
			tl := lane[cpu]
			sort.SliceStable(tl, func(i, j int) bool { return tl[i].ts < tl[j].ts })
		}
	}
	sort.Ints(dst.droppedFreqCPUs)
}
