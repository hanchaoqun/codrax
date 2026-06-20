package types

import "strings"

// SourceScope is the analyzer-emitted answer scope for repo paths. It answers
// "which path roles may be principal evidence for this question?" separately
// from the deterministic SourcePathRole classifier.
type SourceScope string

const (
	SourceScopeUnknown       SourceScope = "unknown"
	SourceScopeProduction    SourceScope = "production"
	SourceScopeTest          SourceScope = "test"
	SourceScopeDocumentation SourceScope = "documentation"
	SourceScopeAuxiliary     SourceScope = "auxiliary"
	SourceScopeAll           SourceScope = "all"
)

func AllSourceScopes() []SourceScope {
	return []SourceScope{
		SourceScopeUnknown,
		SourceScopeProduction,
		SourceScopeTest,
		SourceScopeDocumentation,
		SourceScopeAuxiliary,
		SourceScopeAll,
	}
}

func (s SourceScope) IsValid() bool {
	for _, declared := range AllSourceScopes() {
		if s == declared {
			return true
		}
	}
	return false
}

// SourceScopeProfile preserves the analyzer's typed judgement of whether
// tests/docs/fixtures/examples are principal answer scope for the current
// request. It prevents deterministic code from keyword-scanning prose like
// "test" while still giving downstream rankers a stable opt-in when the user
// really asked about auxiliary material.
type SourceScopeProfile struct {
	RequestedScope              SourceScope `json:"requested_scope"`
	IncludeAuxiliaryAsPrincipal bool        `json:"include_auxiliary_as_principal,omitempty"`
	SourceQuotes                []string    `json:"source_quotes,omitempty"`
	Confidence                  float64     `json:"confidence,omitempty"`
	Rationale                   string      `json:"rationale,omitempty"`
}

func (p *SourceScopeProfile) AllowsAuxiliaryPrincipal() bool {
	if p == nil {
		return false
	}
	if p.IncludeAuxiliaryAsPrincipal {
		return true
	}
	switch p.RequestedScope {
	case SourceScopeTest, SourceScopeDocumentation, SourceScopeAuxiliary, SourceScopeAll:
		return true
	default:
		return false
	}
}

func (p *SourceScopeProfile) Fingerprint() string {
	if p == nil {
		return ""
	}
	parts := []string{
		string(p.RequestedScope),
	}
	if p.IncludeAuxiliaryAsPrincipal {
		parts = append(parts, "aux-principal")
	}
	if len(p.SourceQuotes) > 0 {
		parts = append(parts, strings.Join(p.SourceQuotes, "|"))
	}
	return strings.Join(parts, ":")
}

// SourceScopeAllowsPathRole reports whether a classified path role can be a
// principal candidate under the requested scope.
func SourceScopeAllowsPathRole(scope SourceScope, role SourcePathRole) bool {
	if role == SourcePathRoleUnknown {
		return false
	}
	switch scope {
	case SourceScopeAll:
		return true
	case SourceScopeTest:
		return role == SourcePathRoleTest
	case SourceScopeDocumentation:
		return role == SourcePathRoleDocumentation
	case SourceScopeAuxiliary:
		return SourcePathRoleIsAuxiliary(role)
	case SourceScopeProduction, SourceScopeUnknown, "":
		return role == SourcePathRoleProduction
	default:
		return role == SourcePathRoleProduction
	}
}
