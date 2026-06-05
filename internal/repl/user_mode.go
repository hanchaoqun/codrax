package repl

import (
	"fmt"
	"strings"
)

// UserMode is the user-facing task lane selector. It is deliberately
// separate from types.PipelineMode, which remains the internal write
// phase enum used by the orchestrator.
type UserMode string

const (
	UserModeAuto      UserMode = "auto"
	UserModeCode      UserMode = "code"
	UserModeOperation UserMode = "operation"
	UserModeData      UserMode = "data"
	UserModeWrite     UserMode = "write"
)

func ParseUserMode(value string) (UserMode, error) {
	mode := UserMode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		mode = UserModeAuto
	}
	if !mode.IsValid() {
		return "", fmt.Errorf("unknown mode %q (must be one of: auto, code, operation, data, write)", value)
	}
	return mode, nil
}

func (m UserMode) Normalize() UserMode {
	if strings.TrimSpace(string(m)) == "" {
		return UserModeAuto
	}
	return UserMode(strings.ToLower(strings.TrimSpace(string(m))))
}

func (m UserMode) IsValid() bool {
	switch m.Normalize() {
	case UserModeAuto, UserModeCode, UserModeOperation, UserModeData, UserModeWrite:
		return true
	default:
		return false
	}
}

func (m UserMode) UsesClassifier() bool {
	return m.Normalize() == UserModeAuto
}

func (m UserMode) TurnPolicy() (TurnPolicy, bool) {
	switch m.Normalize() {
	case UserModeCode:
		return TurnPolicy{
			Route:           RouteRepo,
			NeedsRepoAccess: true,
			Operation:       "investigate",
			Source:          "user_override",
			Confidence:      1,
			Reason:          "user explicitly selected code mode",
		}, true
	case UserModeOperation:
		return TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			Operation:            "computer_operation",
			OperationKind:        "computer_operation",
			RiskLevel:            "medium",
			TargetSurface:        "unknown",
			Source:               "user_override",
			Confidence:           1,
			Reason:               "user explicitly selected operation mode",
		}, true
	case UserModeData:
		return TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_task",
			DataTaskKind:    "data_task",
			RiskLevel:       "low",
			Source:          "user_override",
			Confidence:      1,
			Reason:          "user explicitly selected data mode",
		}, true
	default:
		return TurnPolicy{}, false
	}
}
