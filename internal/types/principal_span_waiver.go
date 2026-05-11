// Package types — principal_span_waiver.go (2026-05-12).
//
// PrincipalSpanWaiver is the typed escape lane for the
// callChainPrincipalSpanDowngrade hard gate: when the analyzer
// emitted both a source and a sink endpoint in the same file but no
// model-emitted intermediate evidence sits between them, the system
// blocks emit_investigation_complete and asks for the missing
// intermediate facts. That hard gate is correct for ordinary call
// chains, but a small typed set of legitimate scenarios produce
// source / sink endpoints with no separately-citable user code in
// between — see the reason constants below. The waiver is the
// model's escape valve for those cases: it MUST name one of the
// finite typed reasons, MUST carry a one-sentence rationale, and is
// logged for post-hoc audit so misuse is reviewable.
//
// Architectural alignment: this file mirrors evidence_floor_waiver.go
// (the prior §1.6 typed-escape implementation). Both gates pair a
// precise system-side check with a model-declarable typed exception
// so the LLM never has to hallucinate intermediate evidence to pass
// a gate.
package types

import "strings"

// PrincipalSpanWaiverReason enumerates the typed reasons a model may
// declare to escape the principal-span coverage gate. Six reasons,
// each backed by a concrete cross-language scenario. Add new values
// only when a fundamentally different scenario emerges so the enum
// stays a precise signal rather than a wildcard.
type PrincipalSpanWaiverReason string

const (
	// PrincipalSpanWaiverEndpointsDirectlyAdjacent declares the source
	// and sink endpoints sit on the same statement / line with no
	// intermediate code between them — a single dispatch / wrapper /
	// trampoline like `func dispatch(req) { return handle(req) }`. Use
	// when the cited source line literally calls the cited sink.
	PrincipalSpanWaiverEndpointsDirectlyAdjacent PrincipalSpanWaiverReason = "endpoints_directly_adjacent"

	// PrincipalSpanWaiverNoIntermediateUserCode declares that the lines
	// between the source and sink contain only plumbing the user is
	// not asking about — typically nil guards, accessor pass-throughs,
	// logger calls, defer setup, or boilerplate that cannot stand on
	// its own as a separately-citable mechanism hop.
	PrincipalSpanWaiverNoIntermediateUserCode PrincipalSpanWaiverReason = "no_intermediate_user_code"

	// PrincipalSpanWaiverPlatformBridgeIntermediates declares the
	// intermediate frames cross a cross-language / cross-runtime
	// bridge (FFI, JNI, cgo, ArkTS↔Cangjie native bridge,
	// JavaScript↔native binding) where the intermediate hop has no
	// readable source site inside this repository — the bridge frames
	// belong to the runtime, not the user-authored code.
	PrincipalSpanWaiverPlatformBridgeIntermediates PrincipalSpanWaiverReason = "platform_bridge_intermediates"

	// PrincipalSpanWaiverInlinedCall declares the compiler / JIT
	// inlined the intermediate call so the source line directly
	// invokes the sink with no separate call frame at the source
	// site. Common in Go's mid-stack inliner, Rust monomorphization,
	// C++ template specialization, Java HotSpot inlined-callee.
	PrincipalSpanWaiverInlinedCall PrincipalSpanWaiverReason = "inlined_call"

	// PrincipalSpanWaiverRuntimeDispatchedCall declares the
	// intermediate dispatch is resolved at runtime (interface method
	// table, virtual call, function pointer, closure invocation,
	// reflection-based dispatch) so there is no static call edge to
	// cite between the source and sink frames.
	PrincipalSpanWaiverRuntimeDispatchedCall PrincipalSpanWaiverReason = "runtime_dispatched_call"

	// PrincipalSpanWaiverExternalModuleContinuation declares the
	// chain crosses into an external library / SDK / third-party
	// module whose intermediate frames are not present in the current
	// repository. The source and sink are visible (e.g. user code
	// calls an SDK entry, the SDK reenters user code via a callback)
	// but the intermediate hops live outside the repo.
	PrincipalSpanWaiverExternalModuleContinuation PrincipalSpanWaiverReason = "external_module_continuation"
)

// validPrincipalSpanWaiverReasons enumerates every accepted value so
// JSON decoders / schema builders / error messages share one
// canonical list.
var validPrincipalSpanWaiverReasons = []PrincipalSpanWaiverReason{
	PrincipalSpanWaiverEndpointsDirectlyAdjacent,
	PrincipalSpanWaiverNoIntermediateUserCode,
	PrincipalSpanWaiverPlatformBridgeIntermediates,
	PrincipalSpanWaiverInlinedCall,
	PrincipalSpanWaiverRuntimeDispatchedCall,
	PrincipalSpanWaiverExternalModuleContinuation,
}

// PrincipalSpanWaiverReasonValues returns the enum in declaration
// order. Used by the tool schema's enum list, by validator error
// messages, and by skill-prompt teaching documentation so each
// surface stays byte-canonical.
func PrincipalSpanWaiverReasonValues() []PrincipalSpanWaiverReason {
	out := make([]PrincipalSpanWaiverReason, len(validPrincipalSpanWaiverReasons))
	copy(out, validPrincipalSpanWaiverReasons)
	return out
}

// IsValid reports whether the receiver names one of the accepted
// reason values. False for the zero value so an uninitialised
// PrincipalSpanWaiver does not silently apply.
func (r PrincipalSpanWaiverReason) IsValid() bool {
	for _, v := range validPrincipalSpanWaiverReasons {
		if v == r {
			return true
		}
	}
	return false
}

// PrincipalSpanWaiver is the typed escape envelope. Both fields are
// required when the waiver is set — Reason selects which legitimate
// scenario applies, Rationale is the model's audit-trail
// justification recorded for post-hoc review. Empty / zero value
// means "no waiver claimed".
type PrincipalSpanWaiver struct {
	Reason    PrincipalSpanWaiverReason `json:"reason"`
	Rationale string                    `json:"rationale"`
}

// IsActive reports whether the waiver carries a usable declaration.
// Gate consumers call this as the one-line "should I relax?" check.
// Rejects waivers with empty rationale because an undocumented
// waiver is unreviewable; better to surface the gate error than
// silently honor a blank.
func (w *PrincipalSpanWaiver) IsActive() bool {
	if w == nil {
		return false
	}
	if !w.Reason.IsValid() {
		return false
	}
	return strings.TrimSpace(w.Rationale) != ""
}
