package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// These are labels for individual scheduler records, not the published D+IO
// fold or evidence about the underlying resource. CanonicalLine and the raw
// observation fields remain untouched.
func runtimeTraceWaitOccurrenceStateLabel(state string, zh bool) string {
	switch strings.TrimSpace(state) {
	case "d_sleep", "d_state", "uninterruptible_sleep":
		if zh {
			return "D 状态（不可中断等待）"
		}
		return "D state (uninterruptible wait)"
	case "s_sleep", "sleep":
		word, _ := tracefence.StateLaneWord(tracefence.StateLaneSleep, zh)
		return word
	case "io_wait":
		word, _ := tracefence.StateLaneWord(tracefence.StateLaneIOWait, zh)
		if zh {
			return tracefence.StateSchedulerMarkedQualifierZH + " " + word
		}
		return tracefence.StateSchedulerMarkedQualifierEN + " " + word
	case "running", "runnable":
		word, _ := tracefence.StateLaneWord(state, zh)
		return word
	default:
		if zh {
			return "其他等待状态"
		}
		return "other wait state"
	}
}

func runtimeTraceWaitIOMarkerLabel(marker string, zh bool) string {
	if zh {
		switch marker {
		case "0":
			return "未标记"
		case "1":
			return "已标记"
		default:
			return "未提供"
		}
	}
	switch marker {
	case "0":
		return "not marked"
	case "1":
		return "marked"
	default:
		return "not provided"
	}
}

func runtimeTraceWaitCallsiteLabel(caller string, zh bool) string {
	if caller == "" || caller == "unknown" {
		if zh {
			return "未解析"
		}
		return "unresolved"
	}
	return caller // Kernel symbol bytes are data, never translated or classified.
}
