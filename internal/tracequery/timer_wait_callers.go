package tracequery

import "strings"

// timer_wait_callers.go — GAP-B2 (§13.3(b)/§13.7, 2026-07-25): the
// survey-derived closed set of sched_blocked_reason semantic callers that
// prove a D-state wait is a PERIODIC TIMER wait (the customer's
// timerfd_read shape is the first witness). Membership is the credential
// the VS-1 D∧timer periodic arm and the pacing sleeper admission read —
// a D-family wait whose caller is NOT in this set keeps every byte of its
// io_blocking treatment (D fail-close red line; SYM-2's self-cause
// decomposable D candidates keep their seats). Extending the set requires
// survey evidence (vsyncGeneratorThreadNames precedent) — never guessed
// name patterns.
var timerWaitCallerClosedSet = map[string]bool{
	"timerfd_read": true,
}

// TimerWaitCallerSymbol reduces a blocked-reason semantic caller to its
// bare symbol: parse already collapsed opaque/hex forms to "unknown"
// (blockedReasonSemanticCaller), but a symbolized caller still carries the
// kernel offset suffix ("timerfd_read+0x74/0x120") — the closed set speaks
// bare symbols only.
func TimerWaitCallerSymbol(reason string) string {
	reason = strings.TrimSpace(reason)
	if i := strings.IndexAny(reason, "+ /"); i >= 0 {
		reason = reason[:i]
	}
	return reason
}

// isTimerWaitCaller reports whether the typed caller proves a periodic
// timer wait. Empty and "unknown" fail closed by construction.
func isTimerWaitCaller(reason string) bool {
	return timerWaitCallerClosedSet[TimerWaitCallerSymbol(reason)]
}
