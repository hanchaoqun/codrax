package agent

import "strings"

// renderTraceOccurrenceMeasurements is a last-mile display of the existing
// occurrence_windows note, not a new measurement or a model input. Keep its
// supplied occurrence/field order and values; absent lanes are not zero. The
// producer already bounds the occurrence list. Its original-state window may
// enclose multiple state segments and does not imply continuous occupancy.
func renderTraceOccurrenceMeasurements(value string, zh bool) string {
	clauses := strings.Split(value, ";")
	out := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		parts := strings.Split(clause, ",")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		window := strings.TrimSpace(parts[0])
		var details []string
		for _, part := range parts[1:] {
			key, raw, ok := strings.Cut(strings.TrimSpace(part), "=")
			raw = strings.TrimSpace(raw)
			if !ok || raw == "" {
				continue
			}
			key = strings.TrimSpace(key)
			label := traceOccurrenceMeasurementLabel(key, zh)
			if label == "" {
				// In particular, compact lines do not carry the source identity
				// qualification required to present them as physical coordinates.
				continue
			}
			if key == "state" {
				raw = traceOccurrenceStateLabel(raw, zh)
				if raw == "" {
					continue
				}
			}
			separator := ": "
			if zh {
				separator = "："
			}
			details = append(details, label+separator+raw)
		}
		if len(details) > 0 {
			if zh {
				window += "（" + strings.Join(details, "，") + "）"
			} else {
				window += " (" + strings.Join(details, ", ") + ")"
			}
		}
		out = append(out, window)
	}
	if zh {
		return strings.Join(out, "；")
	}
	return strings.Join(out, "; ")
}

func traceOccurrenceMeasurementLabel(key string, zh bool) string {
	var chinese, english string
	switch key {
	case "state":
		chinese, english = "主导状态", "dominant state"
	case "impact":
		chinese, english = "主导状态耗时", "dominant-state time"
	case "total":
		chinese, english = "本段总占时", "occurrence total"
	case "projected_impact":
		// For a D/IO-dominant occurrence, impact can include both lanes;
		// neither this nor actual_impact is the single dominant-state time
		// or an estimate of eliminable time.
		chinese, english = "查询窗内影响时长", "query-window impact"
	case "projected_total":
		chinese, english = "查询窗内总占时", "query-window total"
	case "actual_impact":
		chinese, english = "原始窗影响时长", "original-window impact"
	case "actual_total":
		chinese, english = "原始窗总占时", "original-window total"
	case "actual_window":
		chinese, english = "原始状态窗", "original state window"
	case "target":
		chinese, english = "目标阻塞时长", "target blocked time"
	case "running":
		chinese, english = "运行", "running"
	case "runnable":
		chinese, english = "调度等待", "scheduling wait"
	case "sleep":
		chinese, english = "睡眠等待", "sleep wait"
	case "d_state":
		chinese, english = "不可中断等待", "uninterruptible wait"
	case "io_wait":
		chinese, english = "IO等待", "IO wait"
	}
	if zh {
		return chinese
	}
	return english
}

// The occurrence's state names an observed lane, not a root-cause verdict.
// Do not pass unknown internal state tokens through the entity-name fallback.
func traceOccurrenceStateLabel(state string, zh bool) string {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return ""
	}
	var chinese, english string
	switch state {
	case "running", "fragmented_running":
		chinese, english = "运行", "running"
	case "runnable", "runnable_wait", "fragmented_runnable_wait", "scheduler_latency", "r":
		chinese, english = "可运行等待", "runnable wait"
	case "s_sleep", "sleep", "sleep_wait", "fragmented_sleep_wait", "s":
		chinese, english = "睡眠等待", "sleep wait"
	case "d_sleep", "d_state", "d":
		chinese, english = "不可中断等待", "uninterruptible wait"
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		chinese, english = "不可中断或IO等待", "uninterruptible or IO wait"
	case "io_wait", "io_latency":
		chinese, english = "IO等待", "IO wait"
	default:
		// Preserve the presence of an unfamiliar state without publishing an
		// internal token or assigning it an unsupported causal interpretation.
		chinese, english = "未识别状态", "unrecognized state"
	}
	if zh {
		return chinese
	}
	return english
}
