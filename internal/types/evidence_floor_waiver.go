// Package types — evidence_floor_waiver.go (2026-05-10).
//
// EvidenceFloorWaiver is the typed escape lane the model uses to
// declare that ordinary repo-grounding requirements do not apply
// to this investigation. The system honors the declaration on the
// hard-gate side (forced-read pending check, citation-floor) but
// records every fire for telemetry / audit so a misuse pattern
// can be caught by post-hoc analysis.
//
// Architectural rationale (2026-05-10 customer-reported scenarios):
// system-side hard gates exist for good reason — the model is
// fallible and fresh-from-the-prompt LLMs sometimes cut corners.
// But there are scenarios where the MODEL has stronger judgment
// than any system-side heuristic: a customer-pasted external log
// (no repo intersection), a synthetic-looking log whose frames
// reference current-repo paths but represent a different build,
// an informational runtime trace with no failure component. The
// pre-existing system-derived `LogBundle.IsExternalSource()`
// catches some of these (zero ResolvedFiles + ≥1 Errors) but
// can't cover (a) custom log shapes that don't parse into Errors
// at all and (b) self-referential synthetic logs whose frames DO
// resolve to current paths but the resolution is misleading.
//
// The waiver is the model's escape valve for those cases. Reasons
// are a finite typed enum (not a free-text catch-all) so the
// system can reason precisely about which gate to relax. Rationale
// is the model's one-sentence justification — recorded for audit,
// not parsed for hard logic.
package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// EvidenceFloorWaiverReason enumerates the typed reasons a model
// may declare to escape repo-grounding requirements. Add new values
// only when a fundamentally different scenario emerges — the enum
// is the precise typed signal the system reasons over, so growing
// it cheapens existing values. Each reason maps to a specific gate
// relaxation (see the consumers in
// internal/tool/emit_investigation_complete.go).
type EvidenceFloorWaiverReason string

const (
	// EvidenceFloorWaiverExternalLog declares the attached log is
	// from a system / service / runtime that has no intersection
	// with the current repository — the canonical "customer pastes
	// a panic from another service" case. Use only when frame paths,
	// service names, and identifiers in the log are NOT defined in
	// the current repo and the user is asking about the LOG itself,
	// not about repo code that could explain it.
	EvidenceFloorWaiverExternalLog EvidenceFloorWaiverReason = "external_only_log"

	// EvidenceFloorWaiverExternalTrace is the trace analogue of
	// EvidenceFloorWaiverExternalLog — a hi-trace / atrace / opentelemetry
	// stream from a different system whose spans / tags do not resolve
	// to the current repo's code.
	EvidenceFloorWaiverExternalTrace EvidenceFloorWaiverReason = "external_only_trace"

	// EvidenceFloorWaiverNoRepoIntersection is a stronger general
	// case: the user's question is structurally answerable from the
	// log / trace content alone and any apparent path intersection
	// with the current repo is coincidental (e.g. a synthetic log
	// where frame paths look like current-repo paths but represent
	// a different build / version / deployment). Use when the
	// model has confidently determined that current-repo reading
	// would be misleading, even if some frames superficially
	// resolve.
	EvidenceFloorWaiverNoRepoIntersection EvidenceFloorWaiverReason = "no_repo_intersection"

	// EvidenceFloorWaiverInformationalRuntime declares the input is
	// informational only (clean baseline, performance debug
	// breadcrumb, expected debug output) with no failure component
	// the user is asking about. Citation against repo code is not
	// applicable because there is no anomaly to ground against.
	EvidenceFloorWaiverInformationalRuntime EvidenceFloorWaiverReason = "informational_runtime_only"
)

// validEvidenceFloorWaiverReasons enumerates every accepted value
// so the JSON decoder can validate strictly without having to
// hand-maintain a parallel list. Used by IsValid and by the
// emit-side parameter validator.
var validEvidenceFloorWaiverReasons = []EvidenceFloorWaiverReason{
	EvidenceFloorWaiverExternalLog,
	EvidenceFloorWaiverExternalTrace,
	EvidenceFloorWaiverNoRepoIntersection,
	EvidenceFloorWaiverInformationalRuntime,
}

// EvidenceFloorWaiverReasonValues returns every accepted enum value
// in declaration order. Schema-builders / strict-decode error
// messages call this so the wire-side enum list stays canonical
// (no risk of the schema description listing values the validator
// rejects, or vice versa).
func EvidenceFloorWaiverReasonValues() []EvidenceFloorWaiverReason {
	out := make([]EvidenceFloorWaiverReason, len(validEvidenceFloorWaiverReasons))
	copy(out, validEvidenceFloorWaiverReasons)
	return out
}

// IsValid reports whether the receiver names one of the accepted
// reason values. False for the zero value so an uninitialised
// EvidenceFloorWaiver does not silently apply.
func (r EvidenceFloorWaiverReason) IsValid() bool {
	for _, v := range validEvidenceFloorWaiverReasons {
		if v == r {
			return true
		}
	}
	return false
}

// EvidenceFloorWaiver is the typed escape envelope. Both fields are
// required when the waiver is set — `Reason` selects the gate
// relaxation and `Rationale` is the model's audit-trail
// justification. Empty / zero value means "no waiver claimed".
type EvidenceFloorWaiver struct {
	Reason    EvidenceFloorWaiverReason `json:"reason"`
	Rationale string                    `json:"rationale"`
}

// IsActive reports whether the waiver carries a usable declaration.
// Used by gate consumers as the one-line "should I relax?" check.
// Defensively rejects waivers with empty rationale (the audit trail
// is part of the contract — a waiver without explanation is useless
// for post-hoc review and we'd rather make the model state its
// reasoning than silently honor a blank).
func (w *EvidenceFloorWaiver) IsActive() bool {
	if w == nil {
		return false
	}
	if !w.Reason.IsValid() {
		return false
	}
	return strings.TrimSpace(w.Rationale) != ""
}

// RuntimeGroundingDispositionSource records where the runtime-grounding
// disposition came from. The source is telemetry / prompt context; hard
// logic still keys off the bounded Reason + CitationPolicy.
type RuntimeGroundingDispositionSource string

const (
	RuntimeGroundingModelDeclared  RuntimeGroundingDispositionSource = "model_declared"
	RuntimeGroundingSystemDetected RuntimeGroundingDispositionSource = "system_detected"
)

// RuntimeGroundingCitationPolicy describes how final answer citations
// should treat runtime-artifact facts once repo grounding has been
// declared inapplicable.
type RuntimeGroundingCitationPolicy string

const (
	RuntimeGroundingCitationRuntimeObservation RuntimeGroundingCitationPolicy = "runtime_observation_only"
)

// RuntimeGroundingDisposition is the answer-surface projection of a
// model-declared waiver or precise system-derived external-artifact
// signal. It deliberately carries the same bounded reason enum as
// EvidenceFloorWaiver so downstream stages do not parse free text to
// decide citation policy.
type RuntimeGroundingDisposition struct {
	Source         RuntimeGroundingDispositionSource `json:"source"`
	Reason         EvidenceFloorWaiverReason         `json:"reason"`
	Rationale      string                            `json:"rationale"`
	CitationPolicy RuntimeGroundingCitationPolicy    `json:"citation_policy"`
}

// IsActive reports whether the disposition is complete enough for
// downstream prompt / citation-policy consumers.
func (d *RuntimeGroundingDisposition) IsActive() bool {
	if d == nil {
		return false
	}
	switch d.Source {
	case RuntimeGroundingModelDeclared, RuntimeGroundingSystemDetected:
	default:
		return false
	}
	if !d.Reason.IsValid() {
		return false
	}
	if strings.TrimSpace(d.Rationale) == "" {
		return false
	}
	return d.CitationPolicy == RuntimeGroundingCitationRuntimeObservation
}

// RuntimeGroundingDispositionFromWaiver projects a model-declared
// EvidenceFloorWaiver into the answer-surface citation policy consumed
// by finalization. All four waiver reasons (`external_only_log`,
// `external_only_trace`, `informational_runtime_only`,
// `no_repo_intersection`) describe a property of an attached runtime
// artifact, so the projection requires that at least one of logBundle /
// perfBundle is actually attached. Without an artifact, the waiver may
// still relax other floors (e.g. forced-read / citation-floor in
// emit_investigation_complete) but it MUST NOT activate the
// observation-only citation policy — that lane assumes downstream
// readers can refer to "the attached log / trace", which would be a
// false premise.
func RuntimeGroundingDispositionFromWaiver(
	w *EvidenceFloorWaiver,
	logBundle *LogBundle,
	perfBundle *PerfBundle,
) *RuntimeGroundingDisposition {
	if !w.IsActive() {
		return nil
	}
	if logBundle == nil && perfBundle == nil {
		// Audit: an active model-declared waiver still relaxes other
		// floors (forced-read / citation-floor) but MUST NOT activate
		// the observation-only citation policy without an artifact —
		// log the drop so a waiver-without-artifact pattern is visible.
		logging.Debug("[runtime_grounding] waiver active (reason=%q) but no log/perf bundle attached: disposition projection dropped", w.Reason)
		return nil
	}
	return &RuntimeGroundingDisposition{
		Source:         RuntimeGroundingModelDeclared,
		Reason:         w.Reason,
		Rationale:      strings.TrimSpace(w.Rationale),
		CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
	}
}

// SystemRuntimeGroundingDisposition projects precise system-derived
// runtime-artifact signals into the same citation-policy shape as a
// model-declared waiver. Model-declared waivers should be preferred by
// callers when both are available.
func SystemRuntimeGroundingDisposition(logBundle *LogBundle, perfBundle *PerfBundle) *RuntimeGroundingDisposition {
	if logBundle != nil && logBundle.IsExternalSource() {
		return &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalLog,
			Rationale:      "structured log errors were extracted, but no stack frame resolved to a current repository file",
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		}
	}
	if perfBundle != nil && perfBundle.IsExternalSource() {
		return &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalTrace,
			Rationale:      "structured trace observations were extracted, but no trace frame resolved to a current repository file",
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		}
	}
	return nil
}
