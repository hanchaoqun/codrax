package llm

import (
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// §29.174 RUN2AUDIT-1 F5 件2: startup soft warning when the configured
// stream first-byte ceiling sits below the reasoning-model safety floor
// (the §29.92.1 180s code default) AND the routed model name matches a
// known reasoning family. The runnable_2.txt customer session ran
// MiniMax-M2.7 with a configured 40s cap and spent the whole run
// rendering "已 1m0s / 首字节上限 40s" heartbeats — the request only
// survived because keep-alive bytes kept sliding the liveness watchdog.
//
// Red-line placement (precise signals for hard gates, noisy signals for
// soft guidance): the CAP comparison is a precise typed-duration
// integer check, but the model-name family match is a substring
// heuristic — so the combined signal drives ONLY a WARN log line.
// Nothing is blocked, no value is rewritten, and operators who
// deliberately run a short cap keep exactly the behavior they
// configured.

// reasoningFamilyModelMarkers is the case-insensitive substring roster
// for model families known to hold first output through a long hidden
// thinking phase. Deliberately short and conservative; misses cost only
// a missing advisory line, hits cost only one WARN.
var reasoningFamilyModelMarkers = []string{
	"minimax",
	"deepseek-r",
	"-r1",
	"qwq",
	"thinking",
	"reasoner",
	"glm-z",
	"kimi-k",
}

func reasoningFamilyModelName(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if lower == "" {
		return false
	}
	for _, marker := range reasoningFamilyModelMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// firstByteCapBelowReasoningFloor reports whether the resolved
// first-byte ceiling deserves the reasoning-floor advisory: a positive
// configured cap strictly below the 180s safety floor, routed at a
// reasoning-family model. A cap left at (or raised above) the default
// never fires — ResolveDurationSeconds returns the 180s floor itself
// when the knob is absent.
func firstByteCapBelowReasoningFloor(model string, cap time.Duration) bool {
	return cap > 0 && cap < defaultStreamFirstByteTimeout && reasoningFamilyModelName(model)
}

// firstByteFloorWarnOnce dedupes the advisory per (model, cap) pair —
// adapters are constructed once per agent, and eight agents on one
// provider must not print eight identical lines.
var firstByteFloorWarnOnce sync.Map

func warnFirstByteCapBelowReasoningFloor(model string, cap time.Duration) {
	if !firstByteCapBelowReasoningFloor(model, cap) {
		return
	}
	key := strings.ToLower(strings.TrimSpace(model)) + "|" + cap.String()
	if _, loaded := firstByteFloorWarnOnce.LoadOrStore(key, true); loaded {
		return
	}
	logging.Warning("[llm] providers.yaml stream_first_byte_timeout_seconds=%d is below the reasoning-model safety floor (%ds) while model %q matches a reasoning family: deep-thinking gateways may hold the first byte past this static cap and survive only on keep-alive liveness resets — consider raising it to >=%d",
		int(cap.Seconds()), defaultStreamFirstByteTimeoutSeconds, strings.TrimSpace(model), defaultStreamFirstByteTimeoutSeconds)
}
