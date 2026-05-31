package tracequery

import (
	"path/filepath"
	"sort"
	"strings"
)

type TraceFlavor string

const (
	TraceFlavorAuto           TraceFlavor = "auto"
	TraceFlavorHarmonyHitrace TraceFlavor = "harmony_hitrace"
	TraceFlavorAndroidAtrace  TraceFlavor = "android_atrace"
	TraceFlavorGenericFtrace  TraceFlavor = "generic_ftrace"
)

type flavorVote struct {
	harmony float64
	android float64
	signals map[string]bool
	lines   int
}

func newFlavorVote(path string) *flavorVote {
	v := &flavorVote{signals: map[string]bool{}}
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))
	switch ext {
	case ".htrace":
		v.add(TraceFlavorHarmonyHitrace, 3, "path_ext_htrace")
	case ".atrace":
		v.add(TraceFlavorAndroidAtrace, 3, "path_ext_atrace")
	case ".systrace":
		v.signals["path_ext_systrace_ambiguous"] = true
	case ".trace", ".ftrace":
		v.signals["path_ext_generic_trace"] = true
	}
	if strings.Contains(base, "hitrace") {
		v.add(TraceFlavorHarmonyHitrace, 2, "path_name_hitrace")
	}
	if strings.Contains(base, "atrace") {
		v.add(TraceFlavorAndroidAtrace, 2, "path_name_atrace")
	}
	return v
}

func (v *flavorVote) observeRawLine(line string) {
	if v == nil {
		return
	}
	v.lines++
	if v.lines > 1024 {
		return
	}
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "codrax-source:"):
		switch {
		case strings.Contains(lower, ".htrace") || strings.Contains(lower, "hitrace"):
			v.add(TraceFlavorHarmonyHitrace, 3, "source_header_hitrace")
		case strings.Contains(lower, ".atrace") || strings.Contains(lower, "atrace"):
			v.add(TraceFlavorAndroidAtrace, 3, "source_header_atrace")
		case strings.Contains(lower, ".systrace"):
			v.signals["source_header_systrace_ambiguous"] = true
		}
	case strings.Contains(lower, "harmonyos") || strings.Contains(lower, "openharmony") || strings.Contains(lower, "ohos"):
		v.add(TraceFlavorHarmonyHitrace, 2, "raw_harmony_marker")
	case strings.Contains(lower, "android systrace") || strings.Contains(lower, "atrace"):
		v.add(TraceFlavorAndroidAtrace, 2, "raw_android_marker")
	}
}

func (v *flavorVote) observeEvent(ev Event) {
	if v == nil {
		return
	}
	text := strings.ToLower(ev.Comm + " " + ev.PrevComm + " " + ev.NextComm + " " + ev.WakeeComm + " " + ev.Name + " " + ev.FieldText + " " + ev.ClockName)
	if strings.Contains(text, "os_ffrt") || strings.Contains(text, "ffrt") {
		v.add(TraceFlavorHarmonyHitrace, 2, "event_ffrt")
	}
	if strings.Contains(text, "rsunirender") || strings.Contains(text, "render_service") || strings.Contains(text, "h:renderframe") {
		v.add(TraceFlavorHarmonyHitrace, 2, "event_harmony_render")
	}
	if strings.Contains(text, "udk-irq") || strings.Contains(text, "kirq") {
		v.add(TraceFlavorHarmonyHitrace, 1.5, "event_harmony_irq")
	}
	if strings.Contains(text, "heca_") {
		v.add(TraceFlavorHarmonyHitrace, 2, "event_harmony_clock")
	}
	if strings.Contains(text, "surfaceflinger") || strings.Contains(text, "system_server") {
		v.add(TraceFlavorAndroidAtrace, 1.5, "event_android_system")
	}
	if strings.Contains(text, "binder:") && strings.Contains(text, "_") {
		v.add(TraceFlavorAndroidAtrace, 0.5, "event_binder_label")
	}
}

func (v *flavorVote) add(flavor TraceFlavor, weight float64, signal string) {
	if v == nil || weight <= 0 {
		return
	}
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		v.harmony += weight
	case TraceFlavorAndroidAtrace:
		v.android += weight
	default:
		return
	}
	if signal != "" {
		v.signals[signal] = true
	}
}

func (v *flavorVote) result() (TraceFlavor, float64, []string) {
	if v == nil {
		return TraceFlavorGenericFtrace, 0.50, nil
	}
	signals := make([]string, 0, len(v.signals))
	for s := range v.signals {
		signals = append(signals, s)
	}
	sort.Strings(signals)
	if v.harmony >= 2 && v.harmony >= v.android+1.5 {
		return TraceFlavorHarmonyHitrace, confidenceFromScore(v.harmony, v.android), signals
	}
	if v.android >= 2 && v.android >= v.harmony+1.5 {
		return TraceFlavorAndroidAtrace, confidenceFromScore(v.android, v.harmony), signals
	}
	if v.harmony > 0 || v.android > 0 {
		signals = append(signals, "ambiguous_trace_flavor")
		return TraceFlavorGenericFtrace, 0.55, signals
	}
	return TraceFlavorGenericFtrace, 0.50, signals
}

func confidenceFromScore(winner, loser float64) float64 {
	margin := winner - loser
	conf := 0.55 + winner*0.04 + margin*0.06
	if conf > 0.98 {
		return 0.98
	}
	if conf < 0.55 {
		return 0.55
	}
	return conf
}

func NormalizeTraceFlavor(raw string) TraceFlavor {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "-", "_")
	switch s {
	case "", "auto":
		return TraceFlavorAuto
	case "harmony", "harmonyos", "ohos", "hitrace", "harmony_hitrace":
		return TraceFlavorHarmonyHitrace
	case "android", "atrace", "android_atrace":
		return TraceFlavorAndroidAtrace
	case "generic", "ftrace", "generic_ftrace", "systrace":
		return TraceFlavorGenericFtrace
	default:
		return TraceFlavorAuto
	}
}

func PrioritySemanticsForFlavor(flavor TraceFlavor) string {
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		return "HarmonyOS/hitrace user-space priority: larger numeric value means higher priority; 1-40=CFS, 41-139=RT; values outside that range are reported as system_or_kernel/raw."
	case TraceFlavorAndroidAtrace:
		return "Android/atrace ftrace priority is reported as raw scheduler priority; do not apply HarmonyOS 1-40/41-139 mapping. Lower raw values often indicate higher scheduling priority, but exact meaning depends on scheduler class and kernel policy."
	default:
		return "Generic ftrace priority is reported as raw scheduler priority; no platform-specific CFS/RT mapping was applied. Treat priority-derived pressure as advisory unless the trace producer's priority semantics are known."
	}
}

func classifyTracePriority(flavor TraceFlavor, prio int) string {
	if prio <= 0 {
		return ""
	}
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		switch {
		case prio >= 1 && prio <= 40:
			return "ohos_cfs"
		case prio >= 41 && prio <= 139:
			return "ohos_rt"
		case prio > 139:
			return "system_or_kernel"
		}
	case TraceFlavorAndroidAtrace:
		return "android_raw_scheduler_prio"
	default:
		return "raw_scheduler_prio"
	}
	return ""
}

func isHighPriorityForPressure(flavor TraceFlavor, prio int, class string) bool {
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		return class == "ohos_rt" || class == "system_or_kernel" || prio >= 41
	default:
		return false
	}
}

func applyPriorityFlavor(ev Event, flavor TraceFlavor) Event {
	if ev.PrevPrio > 0 {
		ev.PrevPrioClass = classifyTracePriority(flavor, ev.PrevPrio)
	}
	if ev.NextPrio > 0 {
		ev.NextPrioClass = classifyTracePriority(flavor, ev.NextPrio)
	}
	if ev.WakeePrio > 0 {
		ev.WakeePrioClass = classifyTracePriority(flavor, ev.WakeePrio)
	}
	return ev
}
