package types

// LogOperationalRelationAuthority is the producer-owned permission for
// relating two or more decoded operational events. It is intentionally
// separate from each event's local meaning: several exact rows can coexist
// without proving that one caused the next.
type LogOperationalRelationAuthority string

const (
	LogOperationalRelationNotApplicable         LogOperationalRelationAuthority = "not_applicable"
	LogOperationalRelationObservedLineOrderOnly LogOperationalRelationAuthority = "observed_log_line_order_only"
	LogOperationalRelationTypedTransition       LogOperationalRelationAuthority = "typed_transition_witness"
)

// ResolveLogOperationalRelationAuthority derives cross-event authority only
// from system-decoded protocol rows. Fewer than two events have no cross-event
// question. Any explicit non-local transition authority is a positive typed
// witness; otherwise multiple rows prove only their observed line order.
func ResolveLogOperationalRelationAuthority(rows []LogOperationalSemantic) LogOperationalRelationAuthority {
	if len(rows) < 2 {
		return LogOperationalRelationNotApplicable
	}
	for _, row := range rows {
		if row.TransitionAuthority != "" && row.TransitionAuthority != LogOperationalTransitionEventLocalOnly {
			return LogOperationalRelationTypedTransition
		}
	}
	return LogOperationalRelationObservedLineOrderOnly
}
