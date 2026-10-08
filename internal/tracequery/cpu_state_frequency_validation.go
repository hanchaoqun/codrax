package tracequery

import "math"

// ValidCPUStateFrequency validates typed quantities and ownership before a
// handoff. It never examines user prose, labels, or root-cause vocabulary.
func ValidCPUStateFrequency(p CPUStateFrequencyResult) bool {
	near := func(a, b float64) bool {
		return finiteCPUStateFrequency(a) && finiteCPUStateFrequency(b) && math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(a, b))
	}
	if p.SourcePath == "" || !finiteCPUStateFrequency(p.Window.StartTs) || !finiteCPUStateFrequency(p.Window.EndTs) || p.Window.EndTs <= p.Window.StartTs || !near(p.WindowWallMs, (p.Window.EndTs-p.Window.StartTs)*1000) {
		return false
	}
	if p.Status == "unavailable" {
		return p.Reason != "" && len(p.CPUs) == 0 && p.CPUCount == 0 && p.CPUTimeMs == 0 && p.KnownJointMs == 0 && p.UnknownJointMs == 0 && p.OmittedCPUs == 0
	}
	if p.Status != "available" || p.Reason != "" || p.CPUCount <= 0 || p.OmittedCPUs < 0 || p.CPUCount != len(p.CPUs)+p.OmittedCPUs || !near(p.CPUTimeMs, float64(p.CPUCount)*p.WindowWallMs) || !finiteCPUStateFrequency(p.KnownJointMs) || !finiteCPUStateFrequency(p.UnknownJointMs) || !near(p.CPUTimeMs, p.KnownJointMs+p.UnknownJointMs) {
		return false
	}
	seen := map[int]bool{}
	known, unknown := 0.0, 0.0
	for _, c := range p.CPUs {
		if !validTraceCPUIndex(c.CPU) || seen[c.CPU] || !near(c.WindowWallMs, p.WindowWallMs) {
			return false
		}
		seen[c.CPU] = true
		if c.TopologySource == "unknown" {
			if c.CoreClass != "unknown" {
				return false
			}
		} else if c.TopologySource != "explicit" || c.CoreClass == "" || normalizeCoreClass(c.CoreClass) != c.CoreClass {
			return false
		}
		for _, v := range []float64{c.IdleKnownMs, c.FrequencyKnownMs, c.JointKnownMs, c.UnknownJointMs} {
			if !finiteCPUStateFrequency(v) || v > p.WindowWallMs+1e-6 {
				return false
			}
		}
		if !near(c.JointKnownMs+c.UnknownJointMs, p.WindowWallMs) || c.JointKnownMs > c.IdleKnownMs+1e-6 || c.JointKnownMs > c.FrequencyKnownMs+1e-6 {
			return false
		}
		if c.IdleUnavailable != "" && c.IdleKnownMs != 0 || c.FrequencyUnavailable != "" && c.FrequencyKnownMs != 0 {
			return false
		}
		if c.OmittedGroups < 0 || c.GroupCount != len(c.Groups)+c.OmittedGroups || c.GroupCount <= 0 || c.OmittedIntervals < 0 || c.TotalIntervals != len(c.Intervals)+c.OmittedIntervals || c.TotalIntervals <= 0 {
			return false
		}
		groupTotal, groupKnown, groupIdle, groupFreq := 0.0, 0.0, 0.0, 0.0
		keys := map[cpuStateFrequencyKey]bool{}
		groupDurations := map[cpuStateFrequencyKey]float64{}
		for _, g := range c.Groups {
			k := cpuStateFrequencyValueKey(g.CPUStateFrequencyValue)
			if keys[k] || !validCPUStateFrequencyValue(g.CPUStateFrequencyValue) || !finiteCPUStateFrequency(g.DurationMs) || g.DurationMs <= 0 || !finiteCPUStateFrequency(g.WindowPct) || !near(g.WindowPct, g.DurationMs/p.WindowWallMs*100) {
				return false
			}
			keys[k] = true
			groupDurations[k] = g.DurationMs
			groupTotal += g.DurationMs
			if g.StateKnown {
				groupIdle += g.DurationMs
			}
			if g.FrequencyKnown {
				groupFreq += g.DurationMs
			}
			if g.StateKnown && g.FrequencyKnown {
				groupKnown += g.DurationMs
			}
		}
		if groupTotal > p.WindowWallMs+1e-6 || groupIdle > c.IdleKnownMs+1e-6 || groupFreq > c.FrequencyKnownMs+1e-6 || groupKnown > c.JointKnownMs+1e-6 {
			return false
		}
		if c.OmittedGroups == 0 && (!near(groupTotal, p.WindowWallMs) || !near(groupIdle, c.IdleKnownMs) || !near(groupFreq, c.FrequencyKnownMs) || !near(groupKnown, c.JointKnownMs)) {
			return false
		}
		end := p.Window.StartTs
		intervalDurations := map[cpuStateFrequencyKey]float64{}
		intervalKnown, intervalIdle, intervalFreq := 0.0, 0.0, 0.0
		for _, iv := range c.Intervals {
			if !validCPUStateFrequencyValue(iv.CPUStateFrequencyValue) || !finiteCPUStateFrequency(iv.StartTs) || !finiteCPUStateFrequency(iv.EndTs) || iv.StartTs != end || iv.EndTs <= iv.StartTs || iv.EndTs > p.Window.EndTs || !near(iv.DurationMs, (iv.EndTs-iv.StartTs)*1000) || iv.IdleLine < 0 || iv.FrequencyLine < 0 || iv.StateKnown != (iv.IdleLine > 0) || iv.FrequencyKnown != (iv.FrequencyLine > 0) {
				return false
			}
			end = iv.EndTs
			intervalDurations[cpuStateFrequencyValueKey(iv.CPUStateFrequencyValue)] += iv.DurationMs
			if iv.StateKnown {
				intervalIdle += iv.DurationMs
			}
			if iv.FrequencyKnown {
				intervalFreq += iv.DurationMs
			}
			if iv.StateKnown && iv.FrequencyKnown {
				intervalKnown += iv.DurationMs
			}
		}
		if c.OmittedIntervals == 0 && end != p.Window.EndTs {
			return false
		}
		if intervalIdle > c.IdleKnownMs+1e-6 || intervalFreq > c.FrequencyKnownMs+1e-6 || intervalKnown > c.JointKnownMs+1e-6 {
			return false
		}
		if c.OmittedIntervals == 0 && (!near(intervalIdle, c.IdleKnownMs) || !near(intervalFreq, c.FrequencyKnownMs) || !near(intervalKnown, c.JointKnownMs)) {
			return false
		}
		for key, ms := range intervalDurations {
			groupMS, present := groupDurations[key]
			if !present && c.OmittedGroups == 0 || present && ms > groupMS+1e-6 {
				return false
			}
		}
		if c.OmittedIntervals == 0 {
			for key, ms := range groupDurations {
				if !near(ms, intervalDurations[key]) {
					return false
				}
			}
		}
		known += c.JointKnownMs
		unknown += c.UnknownJointMs
	}
	if known > p.KnownJointMs+1e-6 || unknown > p.UnknownJointMs+1e-6 {
		return false
	}
	return p.OmittedCPUs != 0 || near(known, p.KnownJointMs) && near(unknown, p.UnknownJointMs)
}

func validCPUStateFrequencyValue(v CPUStateFrequencyValue) bool {
	if v.FrequencyKnown != (v.FrequencyKHz != nil) || v.FrequencyKHz != nil && (*v.FrequencyKHz <= 0 || *v.FrequencyKHz > math.MaxUint32) {
		return false
	}
	switch v.State {
	case "unknown":
		return !v.StateKnown && v.IdleState == nil
	case "active":
		return v.StateKnown && v.IdleState == nil
	case "idle":
		return v.StateKnown && v.IdleState != nil && *v.IdleState < math.MaxUint32
	default:
		return false
	}
}
