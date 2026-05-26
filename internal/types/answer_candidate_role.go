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
	AnswerCandidateRoleConfigFile     AnswerCandidateRole = "config_file"
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
		AnswerCandidateRoleConfigFile,
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
	role := AnswerCandidateRole(normalizeAnswerCandidateRoleAlias(raw))
	if !role.IsValid() {
		return AnswerCandidateRoleUnknown, false
	}
	return role, true
}

func normalizeAnswerCandidateRoleAlias(raw string) string {
	role := strings.ToLower(strings.TrimSpace(raw))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.Join(strings.Fields(role), "_")
	switch role {
	case "interface", "class", "struct", "enum":
		return string(AnswerCandidateRoleType)
	case "func", "procedure":
		return string(AnswerCandidateRoleFunction)
	case "member_function":
		return string(AnswerCandidateRoleMethod)
	case "const":
		return string(AnswerCandidateRoleConstant)
	case "var", "local", "parameter", "param":
		return string(AnswerCandidateRoleVariable)
	case "property", "prop", "member", "struct_field", "object_field", "field_member":
		return string(AnswerCandidateRoleField)
	case "module", "namespace":
		return string(AnswerCandidateRolePackage)
	case "filepath", "file_path", "path":
		return string(AnswerCandidateRoleFile)
	case "manifest", "manifest_file", "config", "configuration", "configuration_file", "build_config", "ci_config":
		return string(AnswerCandidateRoleConfigFile)
	case "import", "import_spec":
		return string(AnswerCandidateRoleImportPath)
	case "literal", "value":
		return string(AnswerCandidateRoleLiteralValue)
	case "guard":
		return string(AnswerCandidateRoleGuardCondition)
	}
	return role
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

// AnswerRoleProfile is the analyzer-emitted typed lane for positive
// role-binding requests such as "the retry budget parameter", "the attempt
// counter", "the guard condition", "the tool name", or "the commit hash".
// Downstream hard gates consume RequiredCandidateRoles plus answer-row
// candidate_role annotations; they never infer the requested role from
// RawRequest or rendered answer prose.
type AnswerRoleProfile struct {
	IsRoleBindingRequested bool                  `json:"is_role_binding_requested"`
	RequiredCandidateRoles []AnswerCandidateRole `json:"required_candidate_roles,omitempty"`
	SourceQuotes           []string              `json:"source_quotes,omitempty"`
	Confidence             float64               `json:"confidence,omitempty"`
	Rationale              string                `json:"rationale,omitempty"`
}

func (p *AnswerRoleProfile) Active() bool {
	return p != nil && p.IsRoleBindingRequested && len(p.RequiredCandidateRoles) > 0
}

// MissingRequiredCandidateRoles returns the positive role obligations that are
// not represented by any principal answer item. It reads only typed
// AnswerDocumentV2 fields: block.surface_role and items[].candidate_role.
func MissingRequiredCandidateRoles(doc *AnswerDocumentV2, required []AnswerCandidateRole) []AnswerCandidateRole {
	if doc == nil || len(required) == 0 {
		return nil
	}
	present := make(map[AnswerCandidateRole]struct{})
	for _, block := range doc.Blocks {
		if block.SurfaceRole != SurfacePrincipal {
			continue
		}
		for _, item := range block.Items {
			if item.CandidateRole == AnswerCandidateRoleUnknown {
				continue
			}
			present[item.CandidateRole] = struct{}{}
		}
	}
	seenRequired := make(map[AnswerCandidateRole]struct{}, len(required))
	missing := make([]AnswerCandidateRole, 0, len(required))
	for _, role := range required {
		if role == AnswerCandidateRoleUnknown {
			continue
		}
		if _, dup := seenRequired[role]; dup {
			continue
		}
		seenRequired[role] = struct{}{}
		if _, ok := present[role]; ok {
			continue
		}
		missing = append(missing, role)
	}
	return missing
}
