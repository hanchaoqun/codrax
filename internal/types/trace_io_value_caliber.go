package types

import "strings"

// IO value calibers describe the producer-selected number, not causal eligibility.
// In particular, completion closure alone does not select a ruler: an off-chain
// request can retain request residence in one view and issuer blocking in another.
const (
	TraceIOValueCaliberRQResidence   = "block_rq_issue_to_complete"
	TraceIOValueCaliberBIOResidence  = "block_bio_queue_to_complete"
	TraceIOValueCaliberIssuerBlocked = "completion_closed_issuer_blocked"
	TraceIOValueCaliberMixed         = "mixed"
)

func NormalizeTraceIOValueCaliber(value string) string {
	switch value = strings.TrimSpace(value); value {
	case TraceIOValueCaliberRQResidence, TraceIOValueCaliberBIOResidence,
		TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberMixed:
		return value
	default:
		return ""
	}
}

// TraceUsesIOValueCaliber includes IO-facet unions whose representative has a
// scheduler-wait or burst token. Ordinary wait/burst rows keep their existing
// labels unless the producer published a recognized measurement carrier.
// This is a value-description selector, never a causal admission decision.
func TraceUsesIOValueCaliber(token, caliber string) bool {
	switch strings.TrimSpace(token) {
	case "io_latency":
		return true
	case "io_wait", "io_burst_episode":
		return NormalizeTraceIOValueCaliber(caliber) != ""
	default:
		return false
	}
}

// MergeTraceIOValueCalibers never lends one member's measurement to another.
// Call with the first real member as the seed, not an empty accumulator.
func MergeTraceIOValueCalibers(a, b string) string {
	a, b = NormalizeTraceIOValueCaliber(a), NormalizeTraceIOValueCaliber(b)
	if a == b {
		return a
	}
	return TraceIOValueCaliberMixed
}

// TraceIOValueCaliberLabel is shared by system-owned charts and sidecar prose.
// Missing legacy metadata is not evidence that the entire request was a wait.
func TraceIOValueCaliberLabel(value string, zh bool) string {
	switch NormalizeTraceIOValueCaliber(value) {
	case TraceIOValueCaliberRQResidence:
		if zh {
			return "块设备请求耗时"
		}
		return "block request residence"
	case TraceIOValueCaliberBIOResidence:
		if zh {
			return "BIO请求耗时"
		}
		return "BIO request residence"
	case TraceIOValueCaliberIssuerBlocked:
		if zh {
			return "提交线程IO等待"
		}
		return "issuer I/O wait"
	case TraceIOValueCaliberMixed:
		if zh {
			return "IO观测时长（混合口径）"
		}
		return "I/O duration (mixed measurements)"
	default:
		if zh {
			return "IO观测时长（口径未明确）"
		}
		return "I/O duration (measurement unspecified)"
	}
}
