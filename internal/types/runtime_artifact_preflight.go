package types

import "strings"

// RuntimeArtifactPreflightProfile is a deterministic, pre-analyzer carrier for
// runtime artifacts that are already present before emit_analysis runs. It is
// built from attached log/trace payloads and resolved runtime-artifact file
// references, then projected through BusContext/AgentContext like other typed
// signals.
//
// This profile is navigation policy input only. It says "a runtime artifact is
// already available, so analyzer repo pre-scan is optional until emit_analysis
// opens a current-source lane"; it does not decide the final answer and must not
// be derived from model prose.
type RuntimeArtifactPreflightProfile struct {
	Active                   bool                               `json:"active,omitempty"`
	SourceNavigationOptional bool                               `json:"source_navigation_optional,omitempty"`
	ReasonCode               string                             `json:"reason_code,omitempty"`
	Artifacts                []RuntimeArtifactPreflightArtifact `json:"artifacts,omitempty"`
}

type RuntimeArtifactPreflightArtifact struct {
	Kind    string `json:"kind,omitempty"`
	Source  string `json:"source,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Carrier string `json:"carrier,omitempty"`
}

const RuntimeArtifactPreflightReasonDetected = "runtime_artifact_preflight_detected"

func NormalizeRuntimeArtifactPreflightProfile(profile RuntimeArtifactPreflightProfile) RuntimeArtifactPreflightProfile {
	seen := map[string]bool{}
	out := RuntimeArtifactPreflightProfile{
		ReasonCode: strings.TrimSpace(profile.ReasonCode),
	}
	for _, artifact := range profile.Artifacts {
		artifact = NormalizeRuntimeArtifactPreflightArtifact(artifact)
		if artifact.Kind == "" && artifact.Source == "" {
			continue
		}
		key := strings.ToLower(artifact.Kind + "\x00" + artifact.Source + "\x00" + artifact.Carrier)
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Artifacts = append(out.Artifacts, artifact)
	}
	out.Active = profile.Active || len(out.Artifacts) > 0
	out.SourceNavigationOptional = profile.SourceNavigationOptional && out.Active
	if out.Active && out.ReasonCode == "" {
		out.ReasonCode = RuntimeArtifactPreflightReasonDetected
	}
	return out
}

func NormalizeRuntimeArtifactPreflightArtifact(artifact RuntimeArtifactPreflightArtifact) RuntimeArtifactPreflightArtifact {
	artifact.Kind = strings.TrimSpace(strings.ToLower(artifact.Kind))
	artifact.Source = strings.TrimSpace(artifact.Source)
	artifact.Detail = strings.TrimSpace(artifact.Detail)
	artifact.Carrier = strings.TrimSpace(artifact.Carrier)
	if artifact.Bytes < 0 {
		artifact.Bytes = 0
	}
	return artifact
}

func (profile RuntimeArtifactPreflightProfile) HasRuntimeArtifact() bool {
	return NormalizeRuntimeArtifactPreflightProfile(profile).Active
}

func (profile RuntimeArtifactPreflightProfile) SourceNavigationOptionalForAnalyze() bool {
	normalized := NormalizeRuntimeArtifactPreflightProfile(profile)
	return normalized.Active && normalized.SourceNavigationOptional
}
