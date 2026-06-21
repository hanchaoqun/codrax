package types

import "strings"

const (
	ReadDispatchPolicyActionAddProof = "add_proof"
	ReadDispatchPolicySurfaceVerify  = "verification"
)

// ReadDispatchPolicy is a scheduler-owned, one-dispatch execution
// contract for read-mode retry actions. It is deliberately stringly at
// this package boundary so shared context types do not import loopkernel;
// producers must still fill it from typed loopkernel actions/routes.
type ReadDispatchPolicy struct {
	Active                    bool     `json:"active,omitempty"`
	Action                    string   `json:"action,omitempty"`
	ReasonCode                string   `json:"reason_code,omitempty"`
	RouteSurface              string   `json:"route_surface,omitempty"`
	AllowedTools              []string `json:"allowed_tools,omitempty"`
	DeniedTools               []string `json:"denied_tools,omitempty"`
	PreferredTools            []string `json:"preferred_tools,omitempty"`
	UnavailablePreferredTools []string `json:"unavailable_preferred_tools,omitempty"`
	ScopePaths                []string `json:"scope_paths,omitempty"`
	MaxToolCalls              int      `json:"max_tool_calls,omitempty"`
	OneShot                   bool     `json:"one_shot,omitempty"`
}

func NormalizeReadDispatchPolicy(p ReadDispatchPolicy) ReadDispatchPolicy {
	p.Action = strings.TrimSpace(p.Action)
	p.ReasonCode = strings.TrimSpace(p.ReasonCode)
	p.RouteSurface = strings.TrimSpace(p.RouteSurface)
	p.AllowedTools = normalizePolicyTools(p.AllowedTools)
	p.DeniedTools = normalizePolicyTools(p.DeniedTools)
	p.PreferredTools = normalizePolicyTools(p.PreferredTools)
	p.UnavailablePreferredTools = normalizePolicyTools(p.UnavailablePreferredTools)
	p.ScopePaths = normalizePolicyScopePaths(p.ScopePaths)
	if p.MaxToolCalls < 0 {
		p.MaxToolCalls = 0
	}
	p.Active = p.Active && p.Action != ""
	if !p.Active {
		p.OneShot = false
	}
	return p
}

func (p ReadDispatchPolicy) IsActive() bool {
	return NormalizeReadDispatchPolicy(p).Active
}

func (p ReadDispatchPolicy) AllowsTool(name string) bool {
	p = NormalizeReadDispatchPolicy(p)
	if !p.Active {
		return true
	}
	tool := CanonicalToolName(name)
	if containsPolicyString(p.DeniedTools, tool) {
		return false
	}
	if len(p.AllowedTools) == 0 {
		return true
	}
	return containsPolicyString(p.AllowedTools, tool)
}

func normalizePolicyTools(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		name := CanonicalToolName(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func normalizePolicyScopePaths(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		path := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		path = strings.TrimPrefix(path, "./")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func containsPolicyString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
