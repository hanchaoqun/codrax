package types

import (
	"encoding/json"
	"path"
	"path/filepath"
	"strings"
)

const (
	ReadDispatchPathScopeAllowed   = "path_scope_allowed"
	ReadDispatchPathScopeInactive  = "path_scope_inactive"
	ReadDispatchPathScopeNoPaths   = "path_scope_no_paths"
	ReadDispatchPathScopeMalformed = "path_scope_malformed"
	ReadDispatchPathScopeOutside   = "path_scope_outside"
)

// ReadDispatchPathScopeDecision is the closed typed result for enforcing a
// scheduler-owned read dispatch policy against structural repo path arguments.
// It deliberately consumes JSON tool params and repo-relative path structure,
// not prompt text, retry hints, model rationale, or user wording.
type ReadDispatchPathScopeDecision struct {
	Allowed        bool     `json:"allowed"`
	ReasonCode     string   `json:"reason_code,omitempty"`
	ToolName       string   `json:"tool_name,omitempty"`
	Field          string   `json:"field,omitempty"`
	Value          string   `json:"value,omitempty"`
	NormalizedPath string   `json:"normalized_path,omitempty"`
	ScopePaths     []string `json:"scope_paths,omitempty"`
}

func (d ReadDispatchPathScopeDecision) Blocked() bool {
	return !d.Allowed && d.ReasonCode != "" && d.ReasonCode != ReadDispatchPathScopeInactive && d.ReasonCode != ReadDispatchPathScopeNoPaths
}

// ValidateReadDispatchPolicyPathScope enforces ReadDispatchPolicy.ScopePaths
// for tools with known structural repo path fields. Empty scope paths mean the
// policy adds no path cap. Virtual references such as blob://... are not repo
// path exploration and are ignored by this gate.
func ValidateReadDispatchPolicyPathScope(policy ReadDispatchPolicy, toolName string, params json.RawMessage, repoRoot string) ReadDispatchPathScopeDecision {
	toolName = CanonicalToolName(toolName)
	policy = NormalizeReadDispatchPolicy(policy)
	if !policy.Active {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeInactive, ToolName: toolName}
	}
	if len(policy.ScopePaths) == 0 {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeNoPaths, ToolName: toolName}
	}
	scopes := normalizedReadDispatchStructuralScopes(policy.ScopePaths, repoRoot)
	if len(scopes) == 0 {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeNoPaths, ToolName: toolName}
	}
	fields := readDispatchPolicyPathFields(toolName, params)
	if len(fields) == 0 {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeNoPaths, ToolName: toolName, ScopePaths: scopes}
	}
	for _, field := range fields {
		normalized, structural, malformed := normalizeReadDispatchStructuralPath(field.Value, repoRoot)
		if !structural {
			continue
		}
		if malformed {
			return ReadDispatchPathScopeDecision{
				Allowed:        false,
				ReasonCode:     ReadDispatchPathScopeMalformed,
				ToolName:       toolName,
				Field:          field.Field,
				Value:          field.Value,
				NormalizedPath: normalized,
				ScopePaths:     scopes,
			}
		}
		if !readDispatchPathWithinAnyScope(normalized, scopes) {
			return ReadDispatchPathScopeDecision{
				Allowed:        false,
				ReasonCode:     ReadDispatchPathScopeOutside,
				ToolName:       toolName,
				Field:          field.Field,
				Value:          field.Value,
				NormalizedPath: normalized,
				ScopePaths:     scopes,
			}
		}
	}
	return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeAllowed, ToolName: toolName, ScopePaths: scopes}
}

// ValidateReadDispatchPolicyScopePaths validates the policy's own scope list.
// Consumers use this before persisting or replaying a policy so malformed scope
// paths do not silently collapse into an unconstrained no-path policy.
func ValidateReadDispatchPolicyScopePaths(policy ReadDispatchPolicy, repoRoot string) ReadDispatchPathScopeDecision {
	policy = NormalizeReadDispatchPolicy(policy)
	if !policy.Active {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeInactive}
	}
	if len(policy.ScopePaths) == 0 {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeNoPaths}
	}
	var scopes []string
	seen := map[string]bool{}
	for _, raw := range policy.ScopePaths {
		normalized, structural, malformed := normalizeReadDispatchStructuralPath(raw, repoRoot)
		if !structural {
			continue
		}
		if malformed {
			return ReadDispatchPathScopeDecision{
				Allowed:        false,
				ReasonCode:     ReadDispatchPathScopeMalformed,
				Field:          "scope_paths",
				Value:          raw,
				NormalizedPath: normalized,
				ScopePaths:     policy.ScopePaths,
			}
		}
		if !seen[normalized] {
			seen[normalized] = true
			scopes = append(scopes, normalized)
		}
	}
	if len(scopes) == 0 {
		return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeNoPaths}
	}
	return ReadDispatchPathScopeDecision{Allowed: true, ReasonCode: ReadDispatchPathScopeAllowed, ScopePaths: scopes}
}

type readDispatchPathField struct {
	Field string
	Value string
}

func readDispatchPolicyPathFields(toolName string, params json.RawMessage) []readDispatchPathField {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil
	}
	switch toolName {
	case "read_file", "list_files":
		if value, ok := readDispatchJSONFieldString(raw, "path"); ok {
			return []readDispatchPathField{{Field: "path", Value: value}}
		}
	case "grep":
		if value, ok := readDispatchJSONFieldString(raw, "path"); ok {
			return []readDispatchPathField{{Field: "path", Value: value}}
		}
		return []readDispatchPathField{{Field: "path", Value: "."}}
	case "repo_map":
		return readDispatchRepoMapPathFields(raw)
	}
	return nil
}

func readDispatchRepoMapPathFields(raw map[string]json.RawMessage) []readDispatchPathField {
	base, _ := readDispatchJSONFieldString(raw, "path")
	var out []readDispatchPathField
	if value, ok := readDispatchJSONFieldString(raw, "target_file"); ok {
		out = append(out, readDispatchPathField{Field: "target_file", Value: readDispatchJoinRepoMapPath(base, value)})
	}
	if value, ok := readDispatchJSONFieldString(raw, "entry_point"); ok {
		out = append(out, readDispatchPathField{Field: "entry_point", Value: readDispatchJoinRepoMapPath(base, value)})
	}
	if value, ok := readDispatchJSONFieldString(raw, "scope"); ok {
		out = append(out, readDispatchPathField{Field: "scope", Value: readDispatchJoinRepoMapPath(base, value)})
	}
	for _, value := range readDispatchJSONFieldStringArray(raw, "scopes") {
		out = append(out, readDispatchPathField{Field: "scopes", Value: readDispatchJoinRepoMapPath(base, value)})
	}
	if len(out) > 0 {
		return out
	}
	if base == "" {
		base = "."
	}
	return []readDispatchPathField{{Field: "path", Value: base}}
}

func readDispatchJSONFieldString(raw map[string]json.RawMessage, field string) (string, bool) {
	value, ok := raw[field]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return "", false
	}
	return strings.TrimSpace(s), strings.TrimSpace(s) != ""
}

func readDispatchJSONFieldStringArray(raw map[string]json.RawMessage, field string) []string {
	value, ok := raw[field]
	if !ok {
		return nil
	}
	var values []string
	if err := json.Unmarshal(value, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func readDispatchJoinRepoMapPath(base, value string) string {
	base = strings.TrimSpace(strings.ReplaceAll(base, "\\", "/"))
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || readDispatchPathIsVirtual(value) || path.IsAbs(value) || readDispatchPathHasWindowsDrive(value) {
		return value
	}
	if base == "" || base == "." || readDispatchPathIsVirtual(base) {
		return value
	}
	return path.Join(base, value)
}

func normalizedReadDispatchStructuralScopes(raw []string, repoRoot string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, scope := range raw {
		normalized, structural, malformed := normalizeReadDispatchStructuralPath(scope, repoRoot)
		if !structural || malformed || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func normalizeReadDispatchStructuralPath(raw, repoRoot string) (normalized string, structural bool, malformed bool) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if value == "" {
		value = "."
	}
	if readDispatchPathIsVirtual(value) {
		return "", false, false
	}
	if path.IsAbs(value) || readDispatchPathHasWindowsDrive(value) {
		if repoRoot == "" {
			return value, true, true
		}
		rel, ok := readDispatchAbsolutePathRel(value, repoRoot)
		if !ok {
			return value, true, true
		}
		value = rel
	}
	cleaned := path.Clean(value)
	if cleaned == "" {
		cleaned = "."
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) || readDispatchPathHasWindowsDrive(cleaned) {
		return cleaned, true, true
	}
	return cleaned, true, false
}

func readDispatchAbsolutePathRel(value, repoRoot string) (string, bool) {
	root := filepath.Clean(repoRoot)
	candidate := filepath.Clean(filepath.FromSlash(value))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ".", true
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return path.Clean(rel), true
}

func readDispatchPathWithinAnyScope(candidate string, scopes []string) bool {
	for _, scope := range scopes {
		if scope == "." {
			return true
		}
		if candidate == scope || strings.HasPrefix(candidate, scope+"/") {
			return true
		}
	}
	return false
}

func readDispatchPathIsVirtual(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "://") || strings.HasPrefix(lower, "blob:")
}

func readDispatchPathHasWindowsDrive(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/'
}
