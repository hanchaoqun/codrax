package tracequery

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const cpuConstraintEpochDisplayCap = 16

type cpuConstraintEpochAccounting struct {
	epochs                   []CPUConstraintEpoch
	total                    int
	restrictionEpochCount    int
	restrictedRunnableWaitMs float64
	sourceAuthority          string
	allowedUniform           bool
	// V-2 (§15.12 批甲 verify): merge results computed on the FULL roster
	// BEFORE the display cap (the cap never caps accounting) — the overlay
	// must never re-derive them from the capped epochs slice.
	mergedProof string
	maskOwner   *CPUConstraintEpoch
}

// computeCPUConstraintEpochAccounting builds consecutive, lossless
// next_info/constraint snapshots. It deliberately does not union allowed
// masks across time: each epoch gets its own CPU-universe proof and exact
// runnable overlap before the public epoch roster is capped.
func computeCPUConstraintEpochAccounting(
	events []Event,
	eventIndexes []int,
	q Query,
	traceLastTs float64,
	runnableSegments []runnableWaitSegment,
	cpuUniverse map[int]bool,
	coreByCPU map[int]string,
	tierCapability coreCapabilityMap,
	identity *queryPIDIdentityFilter,
) map[int]cpuConstraintEpochAccounting {
	if len(events) == 0 || len(eventIndexes) == 0 {
		return nil
	}
	active := map[int]int{}
	full := map[int][]CPUConstraintEpoch{}
	signatures := map[int]string{}
	appendObservedCPU := func(epoch *CPUConstraintEpoch, cpu int) {
		if !validTraceCPUIndex(cpu) {
			return
		}
		for _, existing := range epoch.ObservedCPUs {
			if existing == cpu {
				return
			}
		}
		epoch.ObservedCPUs = append(epoch.ObservedCPUs, cpu)
		sort.Ints(epoch.ObservedCPUs)
	}
	closeActive := func(pid int, endTs float64) {
		index, ok := active[pid]
		epochs := full[pid]
		if !ok || index < 0 || index >= len(epochs) {
			return
		}
		if endTs > epochs[index].StartTs {
			epochs[index].EndTs = endTs
			full[pid] = epochs
		}
	}
	for _, eventIndex := range eventIndexes {
		if eventIndex < 0 || eventIndex >= len(events) {
			continue
		}
		ev := events[eventIndex]
		var pid int
		var epoch CPUConstraintEpoch
		var signature string
		switch {
		case ev.Type == EventCPUConstraint:
			cf := ev.ConstraintFields
			if cf == nil {
				cf = &ConstraintFields{}
			}
			pid = cf.PID
			if !identity.allows(pid) || pid <= 0 {
				continue
			}
			epoch.SourceAuthority = CPUConstraintAllowedCPUsAuthorityConstraintEvent
			epoch.AllowedCPUs = append([]int(nil), cf.Allowed...)
			sort.Ints(epoch.AllowedCPUs)
			epoch.CPUSet = cf.CPUSetName
			epoch.CPUSetIsBinding = strings.TrimSpace(cf.CPUSetName) != ""
			if cf.DestCPUSet {
				appendObservedCPU(&epoch, cf.DestCPU)
			} else if cf.CPUValid {
				appendObservedCPU(&epoch, cf.CPU)
			}
			signature = fmt.Sprintf("constraint|allowed:%v|cpuset:%s|binding:%t|policy:%s",
				epoch.AllowedCPUs, epoch.CPUSet, epoch.CPUSetIsBinding, cf.Policy)
		case ev.Type == EventSchedSwitch && strings.TrimSpace(ev.NextInfoAffinity) != "":
			pid = ev.NextPID
			if !identity.allows(pid) || pid <= 0 {
				continue
			}
			epoch.SourceAuthority = CPUConstraintAllowedCPUsAuthorityKernelNextInfo
			epoch.RawNextInfo = strings.TrimSpace(ev.NextInfo)
			epoch.SnapshotCount = 1
			epoch.snapshotTs = []float64{ev.Ts}
			epoch.Affinity = ev.NextInfoAffinity
			epoch.AllowedCPUs = append([]int(nil), ev.NextInfoAllowedCPUs...)
			sort.Ints(epoch.AllowedCPUs)
			info, ok := parseHarmonyNextInfo(ev.NextInfo)
			if ok {
				epoch.FieldCount = info.fieldCount
				epoch.ExtensionFields = append([]string(nil), info.extra...)
			}
			rich := ev.NextInfoRich()
			epoch.Load, epoch.LoadKnown = ev.NextInfoLoad, rich.NextInfoLoadKnown
			epoch.SchedGroup, epoch.SchedGroupKnown = ev.NextInfoGroup, rich.NextInfoGroupKnown
			epoch.ICESBoost, epoch.ICESBoostKnown = rich.NextInfoBoost, rich.NextInfoBoostKnown
			epoch.SMTExpel, epoch.SMTExpelKnown = ev.NextInfoExpel, rich.NextInfoExpelKnown
			epoch.CGID, epoch.CGIDKnown = ev.NextInfoCGID, rich.NextInfoCGIDKnown
			appendObservedCPU(&epoch, ev.CPU)
			// Raw next_info is part of the signature: load/group/boost/expel/
			// cgid and future append-only tail changes each get their own
			// versioned snapshot instead of being overwritten by the last
			// Policy string.
			signature = "next_info|" + epoch.RawNextInfo
		default:
			continue
		}
		epoch.StartTs = ev.Ts
		epoch.LineStart = ev.Line
		epoch.LineEnd = ev.Line
		if previous, ok := active[pid]; ok && signatures[pid] == signature {
			epochs := full[pid]
			current := &epochs[previous]
			if ev.Line > current.LineEnd {
				current.LineEnd = ev.Line
			}
			for _, cpu := range epoch.ObservedCPUs {
				appendObservedCPU(current, cpu)
			}
			current.SnapshotCount++
			current.snapshotTs = append(current.snapshotTs, ev.Ts)
			full[pid] = epochs
			continue
		}
		closeActive(pid, ev.Ts)
		epoch.Ordinal = len(full[pid]) + 1
		full[pid] = append(full[pid], epoch)
		active[pid] = len(full[pid]) - 1
		signatures[pid] = signature
	}
	for pid := range active {
		// SEAM-3 (§15.12 批甲): a legal time-unbounded query (line window
		// only) must still close the trailing epoch — a persistent binding
		// configuration extends to the trace's last timestamp when no query
		// end bounds it. Closing at the epoch's own StartTs left EndTs=0 and
		// zeroed the binding lane's runnable attribution (root seat lost).
		endTs := q.TimeEnd
		if endTs <= 0 {
			endTs = traceLastTs
		}
		closeActive(pid, endTs)
	}

	segmentsByPID := make(map[int][]runnableWaitSegment)
	segmentsEndingByPID := make(map[int]map[int64][]runnableWaitSegment)
	for _, segment := range runnableSegments {
		if segment.thread.PID <= 0 || !segment.cpuKnown || segment.durationMs <= 0 {
			continue
		}
		pid := segment.thread.PID
		segmentsByPID[pid] = append(segmentsByPID[pid], segment)
		if segmentsEndingByPID[pid] == nil {
			segmentsEndingByPID[pid] = make(map[int64][]runnableWaitSegment)
		}
		key := int64(math.Round(segment.endTs * 1e6))
		segmentsEndingByPID[pid][key] = append(segmentsEndingByPID[pid][key], segment)
	}
	out := make(map[int]cpuConstraintEpochAccounting, len(full))
	for pid, epochs := range full {
		account := cpuConstraintEpochAccounting{total: len(epochs), allowedUniform: cpuConstraintEpochAllowedSetsUniform(epochs)}
		var restrictedIntervals []foldInterval
		for i := range epochs {
			epoch := &epochs[i]
			var epochRunnableIntervals []foldInterval
			// SEAM-4: AllowedCPUsAuthority answers "who published the
			// MASK" — a mask-silent witness (name-only binding) contributes
			// binding provenance, never mask authority.
			if len(epoch.AllowedCPUs) > 0 {
				account.sourceAuthority = mergeCPUConstraintAllowedCPUsAuthority(account.sourceAuthority, epoch.SourceAuthority)
			}
			epoch.ExcludedCPUs = cpuConstraintExcludedCPUsFromUniverse(epoch.AllowedCPUs, cpuUniverse)
			switch {
			case len(epoch.ExcludedCPUs) > 0 && epoch.SourceAuthority != "":
				epoch.RestrictionProof = CPUConstraintRestrictionProofAllowedMaskExcludesUniverse
			case epoch.CPUSetIsBinding && strings.TrimSpace(epoch.CPUSet) != "":
				epoch.RestrictionProof = CPUConstraintRestrictionProofBindingEvent
			}
			epoch.AllowedCoreClasses = coreClassesForCPUs(epoch.AllowedCPUs, coreByCPU)
			if allowedKHz, globalKHz, ok := cpuConstraintTierExclusion(tierCapability, epoch.AllowedCPUs); ok {
				epoch.AllowedMaxTierKHz, epoch.GlobalMaxTierKHz = allowedKHz, globalKHz
			}
			if epoch.RawNextInfo != "" {
				// next_info is emitted on the sched_switch that selects this
				// thread. It authoritatively describes the cpuset at the END
				// of the just-finished runnable wait. Use the per-PID end-time
				// index so a hot thread with N snapshots and N segments stays
				// O(N), not O(N²).
				for _, snapshotTs := range epoch.snapshotTs {
					key := int64(math.Round(snapshotTs * 1e6))
					for candidateKey := key - 1; candidateKey <= key+1; candidateKey++ {
						for _, segment := range segmentsEndingByPID[pid][candidateKey] {
							if math.Abs(segment.endTs-snapshotTs) > 0.000001 {
								continue
							}
							epoch.RunnableWaitMs += segment.durationMs
							epochRunnableIntervals = append(epochRunnableIntervals, foldInterval{start: segment.startTs, end: segment.endTs})
						}
					}
				}
			} else if epoch.EndTs > epoch.StartTs {
				// Explicit binding events are persistent configuration
				// actions; their epoch intersects forward until the next
				// configuration witness.
				for _, segment := range segmentsByPID[pid] {
					start := maxFloat(segment.startTs, epoch.StartTs)
					end := minFloat(segment.endTs, epoch.EndTs)
					if end > start {
						epoch.RunnableWaitMs += (end - start) * 1000
						epochRunnableIntervals = append(epochRunnableIntervals, foldInterval{start: start, end: end})
					}
				}
			}
			if epoch.RestrictionProof != "" {
				account.restrictionEpochCount++
				restrictedIntervals = append(restrictedIntervals, epochRunnableIntervals...)
			}
		}
		// One physical runnable interval may carry both an explicit binding
		// witness and the next_info snapshot on its sched-in boundary. Epochs
		// retain both evidence views, but the causal value owns the interval
		// once: union before summing.
		account.restrictedRunnableWaitMs, _ = foldIntervalUnionMs(restrictedIntervals)
		account.mergedProof = cpuConstraintEpochMergedRestrictionProof(epochs, account.allowedUniform, account.restrictedRunnableWaitMs)
		if owner := cpuConstraintEpochFirstMaskBearing(epochs); owner != nil {
			ownerCopy := *owner
			account.maskOwner = &ownerCopy
		}
		emitted := len(epochs)
		if emitted > cpuConstraintEpochDisplayCap {
			emitted = cpuConstraintEpochDisplayCap
		}
		account.epochs = append([]CPUConstraintEpoch(nil), epochs[:emitted]...)
		out[pid] = account
	}
	return out
}

// cpuConstraintEpochAllowedSetsUniform — SEAM-1 (§15.12 批甲): mask
// consistency is judged over mask-BEARING epochs only. A witness that
// published no mask (a name-only cpuset/cgroup attach — the common HarmonyOS
// shape) is mask-SILENT, not a mask conflict: adding it must never wipe the
// kernel next_info mask payload (R-AUTH: two sources together are never
// weaker than one). The §15.10 withdrawal ruling applies to mask CHANGES.
func cpuConstraintEpochAllowedSetsUniform(epochs []CPUConstraintEpoch) bool {
	first := ""
	seen := false
	for i := range epochs {
		if len(epochs[i].AllowedCPUs) == 0 {
			continue
		}
		signature := fmt.Sprint(epochs[i].AllowedCPUs)
		if !seen {
			first, seen = signature, true
			continue
		}
		if signature != first {
			return false
		}
	}
	return true
}

// cpuConstraintEpochFirstMaskBearing returns the first epoch that actually
// published a CPU mask — the top-level mask payload owner under a uniform
// roster (positional epochs[0] could be a mask-silent binding witness).
func cpuConstraintEpochFirstMaskBearing(epochs []CPUConstraintEpoch) *CPUConstraintEpoch {
	for i := range epochs {
		if len(epochs[i].AllowedCPUs) > 0 {
			return &epochs[i]
		}
	}
	return nil
}

// cpuConstraintEpochMergedRestrictionProof — SEAM-2 (§15.12 批甲): the
// top-level proof is a deterministic merge over the epoch EVIDENCE SET,
// never a positional copy (in-window event order must not decide the hard
// gate). A uniform mask-exclusion proof outranks (it carries the exclusion
// payload); an explicit binding event's proof survives mask history (a
// binding event claims no mask, so changing masks cannot withdraw it); mask
// proofs under changing masks degrade to the epoch-scoped proof only with
// runnable overlap (§15.10).
func cpuConstraintEpochMergedRestrictionProof(epochs []CPUConstraintEpoch, maskUniform bool, restrictedRunnableMs float64) string {
	hasMaskProof, hasBindingProof := false, false
	for i := range epochs {
		if epochs[i].RestrictionProof == CPUConstraintRestrictionProofAllowedMaskExcludesUniverse {
			hasMaskProof = true
		}
		// V-1 (§15.12 批甲 verify): binding-ness is censused from the TYPED
		// bit, never the per-epoch proof LABEL — a mask-bearing binding
		// event legitimately mints mask_excludes as its (stronger) per-epoch
		// proof, but its binding evidence must still survive mask history.
		if epochs[i].CPUSetIsBinding && strings.TrimSpace(epochs[i].CPUSet) != "" {
			hasBindingProof = true
		}
	}
	if maskUniform && hasMaskProof {
		return CPUConstraintRestrictionProofAllowedMaskExcludesUniverse
	}
	if hasBindingProof {
		return CPUConstraintRestrictionProofBindingEvent
	}
	if hasMaskProof && restrictedRunnableMs > 0 {
		return CPUConstraintRestrictionProofEpochScoped
	}
	return ""
}

func cpuConstraintAttributedRunnableMs(item CPUConstraintSummary) float64 {
	if item.EpochTotal > 0 {
		return item.RestrictedRunnableWaitMs
	}
	return item.RunnableWaitMs
}

// CPUConstraintEpochDigest is the stable compact handoff form used by the
// text/observation faces. The typed JSON epochs remain authoritative; this
// digest only keeps the same facts visible to model-facing text.
func CPUConstraintEpochDigest(epochs []CPUConstraintEpoch, total int) string {
	if total <= 0 {
		return ""
	}
	parts := make([]string, 0, len(epochs))
	for i, epoch := range epochs {
		if i >= 4 {
			break
		}
		fields := []string{
			fmt.Sprintf("#%d@%.6f..%.6f", epoch.Ordinal, epoch.StartTs, epoch.EndTs),
			"src=" + cpuConstraintEpochSourceWord(epoch.SourceAuthority),
			fmt.Sprintf("f=%d", epoch.FieldCount),
		}
		if len(epoch.ExtensionFields) > 0 {
			fields = append(fields, "tail="+strings.Join(epoch.ExtensionFields, ","))
		}
		if epoch.Affinity != "" {
			fields = append(fields, "a="+epoch.Affinity)
		}
		if len(epoch.AllowedCPUs) > 0 {
			fields = append(fields, "cpus="+intListString(epoch.AllowedCPUs))
		}
		if epoch.RestrictionProof != "" {
			fields = append(fields, "p="+cpuConstraintEpochProofWord(epoch.RestrictionProof))
		}
		fields = append(fields, fmt.Sprintf("run=%.3fms", epoch.RunnableWaitMs))
		if epoch.LoadKnown {
			fields = append(fields, fmt.Sprintf("load=%d", epoch.Load))
		}
		if epoch.SchedGroupKnown {
			fields = append(fields, fmt.Sprintf("grp=%d", epoch.SchedGroup))
		}
		if epoch.ICESBoostKnown {
			fields = append(fields, fmt.Sprintf("boost=%t", epoch.ICESBoost))
		}
		if epoch.SMTExpelKnown {
			fields = append(fields, fmt.Sprintf("expel=%d", epoch.SMTExpel))
		}
		if epoch.CGIDKnown {
			fields = append(fields, fmt.Sprintf("cgid=%d", epoch.CGID))
		}
		parts = append(parts, strings.Join(fields, "/"))
	}
	if total > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d_more", total-len(parts)))
	}
	return strings.Join(parts, "|")
}

func cpuConstraintEpochSourceWord(source string) string {
	switch source {
	case CPUConstraintAllowedCPUsAuthorityKernelNextInfo:
		return "ni"
	case CPUConstraintAllowedCPUsAuthorityConstraintEvent:
		return "event"
	default:
		return source
	}
}

func cpuConstraintEpochProofWord(proof string) string {
	switch proof {
	case CPUConstraintRestrictionProofAllowedMaskExcludesUniverse:
		return "mask_excludes"
	case CPUConstraintRestrictionProofBindingEvent:
		return "binding"
	default:
		return proof
	}
}
