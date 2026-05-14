package types

import "strings"

// AnswerCandidateRole is a language-neutral category/role label for a visible
// answer row. It lets the final answer carrier preserve whether a row is a
// function, variable, tool name, import path, budget cap, attempt counter, etc.
// without forcing validators to infer that category from prose.
type AnswerCandidateRole string

const (
	AnswerCandidateRoleUnknown        AnswerCandidateRole = ""
	AnswerCandidateRoleFunction       AnswerCandidateRole = "function"
	AnswerCandidateRoleMethod         AnswerCandidateRole = "method"
	AnswerCandidateRoleType           AnswerCandidateRole = "type"
	AnswerCandidateRoleConstant       AnswerCandidateRole = "constant"
	AnswerCandidateRoleVariable       AnswerCandidateRole = "variable"
	AnswerCandidateRoleField          AnswerCandidateRole = "field"
	AnswerCandidateRolePackage        AnswerCandidateRole = "package"
	AnswerCandidateRoleFile           AnswerCandidateRole = "file"
	AnswerCandidateRoleTest           AnswerCandidateRole = "test"
	AnswerCandidateRoleGenerated      AnswerCandidateRole = "generated"
	AnswerCandidateRolePrivate        AnswerCandidateRole = "private"
	AnswerCandidateRoleDocumentation  AnswerCandidateRole = "documentation"
	AnswerCandidateRoleExample        AnswerCandidateRole = "example"
	AnswerCandidateRoleFixture        AnswerCandidateRole = "fixture"
	AnswerCandidateRoleHelper         AnswerCandidateRole = "helper"
	AnswerCandidateRoleToolName       AnswerCandidateRole = "tool_name"
	AnswerCandidateRoleConfigKey      AnswerCandidateRole = "config_key"
	AnswerCandidateRoleRoute          AnswerCandidateRole = "route"
	AnswerCandidateRoleImportPath     AnswerCandidateRole = "import_path"
	AnswerCandidateRoleLiteralValue   AnswerCandidateRole = "literal_value"
	AnswerCandidateRoleCommitHash     AnswerCandidateRole = "commit_hash"
	AnswerCandidateRoleBudgetCap      AnswerCandidateRole = "budget_cap"
	AnswerCandidateRoleAttemptCounter AnswerCandidateRole = "attempt_counter"
	AnswerCandidateRoleGuardCondition AnswerCandidateRole = "guard_condition"
	AnswerCandidateRoleOther          AnswerCandidateRole = "other"
)

func AllAnswerCandidateRoles() []AnswerCandidateRole {
	return []AnswerCandidateRole{
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleMethod,
		AnswerCandidateRoleType,
		AnswerCandidateRoleConstant,
		AnswerCandidateRoleVariable,
		AnswerCandidateRoleField,
		AnswerCandidateRolePackage,
		AnswerCandidateRoleFile,
		AnswerCandidateRoleTest,
		AnswerCandidateRoleGenerated,
		AnswerCandidateRolePrivate,
		AnswerCandidateRoleDocumentation,
		AnswerCandidateRoleExample,
		AnswerCandidateRoleFixture,
		AnswerCandidateRoleHelper,
		AnswerCandidateRoleToolName,
		AnswerCandidateRoleConfigKey,
		AnswerCandidateRoleRoute,
		AnswerCandidateRoleImportPath,
		AnswerCandidateRoleLiteralValue,
		AnswerCandidateRoleCommitHash,
		AnswerCandidateRoleBudgetCap,
		AnswerCandidateRoleAttemptCounter,
		AnswerCandidateRoleGuardCondition,
		AnswerCandidateRoleOther,
	}
}

func (r AnswerCandidateRole) IsValid() bool {
	if r == AnswerCandidateRoleUnknown {
		return true
	}
	for _, declared := range AllAnswerCandidateRoles() {
		if r == declared {
			return true
		}
	}
	return false
}

func NormalizeAnswerCandidateRole(raw string) (AnswerCandidateRole, bool) {
	role := AnswerCandidateRole(strings.TrimSpace(raw))
	if !role.IsValid() {
		return AnswerCandidateRoleUnknown, false
	}
	return role, true
}

// AnswerExclusionPolicy is the analyzer-emitted typed lane for current-request
// exclusions such as "do not list variables" or "exclude generated files".
// Downstream hard gates consume only ExcludedCandidateRoles, never RawRequest
// keyword scans.
type AnswerExclusionPolicy struct {
	IsExclusionRequested   bool                  `json:"is_exclusion_requested"`
	ExcludedCandidateRoles []AnswerCandidateRole `json:"excluded_candidate_roles,omitempty"`
	SourceQuotes           []string              `json:"source_quotes,omitempty"`
	Confidence             float64               `json:"confidence,omitempty"`
	Rationale              string                `json:"rationale,omitempty"`
}

func (p *AnswerExclusionPolicy) Active() bool {
	return p != nil && p.IsExclusionRequested && len(p.ExcludedCandidateRoles) > 0
}

func (p *AnswerExclusionPolicy) ExcludesRole(role AnswerCandidateRole) bool {
	if !p.Active() || role == AnswerCandidateRoleUnknown {
		return false
	}
	for _, excluded := range p.ExcludedCandidateRoles {
		if excluded == role {
			return true
		}
	}
	return false
}
