package tracequery

import "math"

// Display/handoff validation of typed measurements, never prose or root election.
func ValidProcessProfile(p ProcessProfile) bool {
	valid := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	near := func(a, b float64) bool { return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(a, b)) }
	if !valid(p.Window.StartTs) || !valid(p.Window.EndTs) || p.Window.EndTs <= p.Window.StartTs || !near(p.WindowMs, (p.Window.EndTs-p.Window.StartTs)*1000) {
		return false
	}
	if p.Status == "unavailable" {
		return p.Reason != "" && len(p.Threads) == 0
	}
	if p.Status != "observed_members" || p.TGID <= 0 || p.SourceThread.TGID != p.TGID || p.MembershipBasis != "observed_native_tgid_members" || p.ThreadCount <= 0 || p.EmittedThreads != len(p.Threads) || p.OmittedThreads < 0 || p.ThreadCount != p.EmittedThreads+p.OmittedThreads || p.UnavailableThreads < 0 || p.UnavailableThreads > p.ThreadCount || p.UnknownMembershipThreads < 0 {
		return false
	}
	seen := map[int]bool{}
	for _, row := range p.Threads {
		if row.Thread.PID <= 0 || row.Thread.TGID != p.TGID || seen[row.Thread.PID] || row.States == nil {
			return false
		}
		seen[row.Thread.PID] = true
		s := row.States
		if !valid(s.UnknownMs) || s.UnknownMs > p.WindowMs+1e-6 {
			return false
		}
		if v := s.Values; v != nil {
			for _, ms := range []float64{v.RunningMs, v.RunnableMs, v.SleepMs, v.DStateMs, v.IOWaitMs, v.StoppedMs, v.DeadMs, v.SleepIOWaitMs, v.AccountedMs} {
				if !valid(ms) {
					return false
				}
			}
			if !near(v.AccountedMs, v.RunningMs+v.RunnableMs+v.SleepMs+v.DStateMs+v.IOWaitMs+v.StoppedMs+v.DeadMs) || !near(v.AccountedMs+s.UnknownMs, p.WindowMs) || v.SleepIOWaitMs > v.SleepMs+1e-6 || row.RunningWindowPct == nil || !valid(*row.RunningWindowPct) || !near(*row.RunningWindowPct, v.RunningMs/p.WindowMs*100) {
				return false
			}
		} else if row.RunningWindowPct != nil || !near(s.UnknownMs, p.WindowMs) {
			return false
		}
		if row.OmittedSleepGroups < 0 || row.SleepGroupCount != len(row.SleepGroups)+row.OmittedSleepGroups || row.OmittedBusinessGroups < 0 || row.BusinessGroupCount != len(row.BusinessHotspots)+row.OmittedBusinessGroups {
			return false
		}
		for _, g := range row.SleepGroups {
			switch g.State {
			case StateSSleep, StateDSleep, StateIOWait:
			default:
				return false
			}
			if g.CallerKnown != (g.Caller != "") || g.IntervalCount <= 0 || !valid(g.DurationMs) || !valid(g.IOFlagPositiveMs) || g.IOFlagPositiveMs > g.DurationMs+1e-6 || g.IOFlagKnownIntervals < 0 || g.IOFlagKnownIntervals > g.IntervalCount {
				return false
			}
		}
		for _, h := range row.BusinessHotspots {
			if h.InstanceCount <= 0 || !valid(h.InclusiveMs) || !valid(h.MaxInstanceMs) || h.MaxInstanceMs > h.InclusiveMs+1e-6 || h.LineStart <= 0 || h.LineEnd < h.LineStart {
				return false
			}
		}
	}
	return true
}
