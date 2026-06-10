package safety

import "strings"

type PermissionAction string

const (
	PermissionAllow PermissionAction = "allow"
	PermissionAsk   PermissionAction = "ask"
	PermissionDeny  PermissionAction = "deny"
)

type PermissionDecision struct {
	Action     PermissionAction `json:"action"`
	ReasonCode string           `json:"reason_code,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Source     string           `json:"source,omitempty"`
}

func NormalizePermissionAction(action PermissionAction) PermissionAction {
	switch PermissionAction(strings.ToLower(strings.TrimSpace(string(action)))) {
	case PermissionAllow:
		return PermissionAllow
	case PermissionAsk:
		return PermissionAsk
	case PermissionDeny:
		return PermissionDeny
	default:
		return ""
	}
}

func NormalizePermissionDecision(decision PermissionDecision) PermissionDecision {
	decision.Action = NormalizePermissionAction(decision.Action)
	decision.ReasonCode = strings.TrimSpace(decision.ReasonCode)
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.Source = strings.TrimSpace(decision.Source)
	return decision
}

func AllowPermission(source, reasonCode, reason string) PermissionDecision {
	return NormalizePermissionDecision(PermissionDecision{
		Action:     PermissionAllow,
		ReasonCode: reasonCode,
		Reason:     reason,
		Source:     source,
	})
}

func AskPermission(source, reasonCode, reason string) PermissionDecision {
	return NormalizePermissionDecision(PermissionDecision{
		Action:     PermissionAsk,
		ReasonCode: reasonCode,
		Reason:     reason,
		Source:     source,
	})
}

func DenyPermission(source, reasonCode, reason string) PermissionDecision {
	return NormalizePermissionDecision(PermissionDecision{
		Action:     PermissionDeny,
		ReasonCode: reasonCode,
		Reason:     reason,
		Source:     source,
	})
}

// FoldPermissionDecisions combines deterministic policy observations. Deny
// wins over ask, ask wins over allow. Reasons are not interpreted; they remain
// audit text attached to the typed action.
func FoldPermissionDecisions(source string, decisions ...PermissionDecision) PermissionDecision {
	out := AllowPermission(source, "all_rules_allow", "all permission rules allow the action")
	for _, decision := range decisions {
		decision = NormalizePermissionDecision(decision)
		switch decision.Action {
		case PermissionDeny:
			return decision
		case PermissionAsk:
			if out.Action != PermissionAsk {
				out = decision
			}
		case PermissionAllow:
			if out.Action == "" {
				out = decision
			}
		}
	}
	return NormalizePermissionDecision(out)
}
