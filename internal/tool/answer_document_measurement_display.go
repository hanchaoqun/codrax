package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceMeasurementTypeDisplayLabel preserves the ruler of this exact typed
// value. IO family membership, completion closure and a peer identity cannot
// substitute for its published caliber. This is vocabulary only: it grants
// neither a root-cause seat nor a blocking relationship.
//
// A bare token cannot say what a blocking row proved: callers that hold the
// row use TraceMeasurementNodeTypeDisplayLabel (a projection node) or
// TraceBlockingRowDisplayLabel (the typed notes of an observation), which
// add the row's typed blocking kind. Without it the blocking_span word is the
// kind-neutral recorded-wait word, never a lock word.
func TraceMeasurementTypeDisplayLabel(token, ioValueCaliber string, zh bool) string {
	if types.TraceUsesIOValueCaliber(token, ioValueCaliber) {
		return types.TraceIOValueCaliberLabel(ioValueCaliber, zh)
	}
	return TraceRootCauseTypeDisplayLabel(token, zh)
}

// TraceMeasurementNodeTypeDisplayLabel is TraceMeasurementTypeDisplayLabel
// for a token read from one projection node: a blocking_span token speaks the
// node's own typed blocking kind (HMC377 D2), every other token is unchanged.
func TraceMeasurementNodeTypeDisplayLabel(token string, node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceCausalProjectionCanonicalNode(token) == "blocking_span" {
		return TraceBlockingRowDisplayLabel(node.BlockingKind, node.BlockingValueBasis, node.BlockingSubjectIsHolder, zh)
	}
	return TraceMeasurementTypeDisplayLabel(token, node.IOValueCaliber, zh)
}

// TraceBlockingRowDisplayLabel is THE reader word of a blocking_span row
// (HMC377 D2). The word comes from what the row's typed evidence proves:
//
//   - a parsed lock or monitor contention payload (typed BlockingKind) speaks
//     lock words — 持锁阻塞 when the row's subject is the resolved holder,
//     锁竞争等待 when the subject is the waiter;
//   - any other blocking row is a recorded wait: 阻塞等待, or the XERR1
//     envelope retreat 「span 包络(含运行)」 when the waiter's wait segments
//     could not be derived (no blocking claim at all).
//
// Lock words never come from the row token, the span name or a waker: a
// kind-less row is a synchronous wait ended by whoever woke it. Exact typed
// enum comparisons only.
func TraceBlockingRowDisplayLabel(blockingKind, valueBasis string, holderSubject, zh bool) string {
	if strings.TrimSpace(blockingKind) != "" {
		switch {
		case holderSubject && zh:
			return "持锁阻塞"
		case holderSubject:
			return "lock-holder blocking"
		case zh:
			return "锁竞争等待"
		default:
			return "lock contention wait"
		}
	}
	kind := "blocking_span"
	if strings.TrimSpace(valueBasis) == tracequery.BlockingValueBasisSpanEnvelope {
		kind = runtimeTraceBlockingSpanKindEnvelope
	}
	return runtimeTraceCausalProjectionPeerRelationShortWord(kind, zh)
}

// runtimeTraceBlockingWakeRelayText (HMC377 D2) is the relation form of a
// payload-less blocking row whose waker carries the engine's typed relay
// verdict: the waker was parked in the same recorded wait and only passed the
// wakeup on, so it is named as a fellow waiter beside the upstream party —
// never as the counterpart this wait depended on. Same wait-segments word
// head as the neutral form; ok=false keeps the neutral wording.
func runtimeTraceBlockingWakeRelayText(node types.TraceCausalProjectionNode, kind, peer string, zh bool) (string, bool) {
	upstream := strings.TrimSpace(node.BlockingWakeRelayUpstream)
	if !node.BlockingWakeRelay || upstream == "" || kind != runtimeTraceBlockingSpanKindWaitSegments {
		return "", false
	}
	upstream = runtimeTraceCausalProjectionDisplayNodeName(upstream, zh)
	head := runtimeTraceCausalProjectionPeerRelationShortWord(kind, zh)
	if zh {
		return head + "(唤醒由同在等待的 " + peer + " 转发;上游 " + upstream + ")", true
	}
	return head + " (wakeup passed on by fellow waiter " + peer + "; upstream " + upstream + ")", true
}
