package types

import "strings"

// CapabilitySurfaceHint is the compiled authority lane for questions
// about whether a pipeline stage / agent / skill exposes a tool.
//
// The analyzer may recommend this lane, but once present it becomes
// the shared authority record consumed by explorer prompts,
// pre-complete closure gates, and final-answer surface builders.
type CapabilitySurfaceHint struct {
	Binding        StageBinding `json:"binding"`
	Tool           string       `json:"tool,omitempty"`
	AuthorityFiles []string     `json:"authority_files,omitempty"`
}

// NormalizeCapabilitySurfaceHint validates and canonicalizes the
// compiled hint. Invalid or incomplete hints return nil so downstream
// consumers can degrade cleanly without re-deriving policy.
func NormalizeCapabilitySurfaceHint(in *CapabilitySurfaceHint) *CapabilitySurfaceHint {
	if in == nil {
		return nil
	}
	if in.Binding.Stage == "" || in.Binding.Agent == "" || strings.TrimSpace(in.Binding.Skill) == "" {
		return nil
	}
	tool := strings.TrimSpace(in.Tool)
	if tool == "" {
		return nil
	}
	out := &CapabilitySurfaceHint{
		Binding: in.Binding,
		Tool:    tool,
	}
	seen := make(map[string]bool, len(in.AuthorityFiles))
	for _, raw := range in.AuthorityFiles {
		path := strings.TrimSpace(raw)
		path = strings.ReplaceAll(path, "\\", "/")
		path = strings.TrimPrefix(path, "./")
		if path == "" || path == "." || seen[path] {
			continue
		}
		seen[path] = true
		out.AuthorityFiles = append(out.AuthorityFiles, path)
	}
	return out
}

func HasCapabilitySurfaceHint(rm RequestModel) bool {
	return NormalizeCapabilitySurfaceHint(rm.AnalyzerHints.CapabilitySurface) != nil
}
