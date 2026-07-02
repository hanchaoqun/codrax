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
	RepoSourceCensus         RuntimeArtifactRepoSourceCensus    `json:"repo_source_census,omitempty"`
}

// RuntimeArtifactRepoSourceCensus is a deterministic run-entry census of the
// repository root used to detect the "runtime artifact only" repository shape:
// every regular file is itself a log/trace artifact by path shape, so a
// current-source citation floor is structurally unsatisfiable — no sequence of
// tool calls can ever produce a cite-eligible current-source line. Gates that
// would otherwise demand current-source proof consume ZeroCurrentSourceRepo as
// a precise environment-state signal (never model prose or user wording).
//
// Completed=false (walk aborted at the entry cap or on I/O error) makes the
// census inert: ZeroCurrentSourceRepo never fires on partial knowledge.
type RuntimeArtifactRepoSourceCensus struct {
	Completed     bool `json:"completed,omitempty"`
	SourceFiles   int  `json:"source_files,omitempty"`
	ArtifactFiles int  `json:"artifact_files,omitempty"`
}

func (c RuntimeArtifactRepoSourceCensus) ZeroCurrentSourceRepo() bool {
	return c.Completed && c.SourceFiles == 0 && c.ArtifactFiles > 0
}

// ZeroCurrentSourceRepo reports the runtime-artifact-only repository shape:
// the census completed, found at least one runtime artifact, and found zero
// current-source files, while the preflight itself carries an active runtime
// artifact. This is the single chokepoint gates use to recognize a repository
// where current-source citations cannot exist.
func (profile RuntimeArtifactPreflightProfile) ZeroCurrentSourceRepo() bool {
	return profile.RepoSourceCensus.ZeroCurrentSourceRepo() && profile.HasRuntimeArtifact()
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
		ReasonCode:       strings.TrimSpace(profile.ReasonCode),
		RepoSourceCensus: profile.RepoSourceCensus,
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

func (profile RuntimeArtifactPreflightProfile) HasTraceArtifact() bool {
	return profile.HasArtifactKind("trace")
}

func (profile RuntimeArtifactPreflightProfile) HasLogArtifact() bool {
	return profile.HasArtifactKind("log")
}

func (profile RuntimeArtifactPreflightProfile) HasArtifactKind(kind string) bool {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return false
	}
	normalized := NormalizeRuntimeArtifactPreflightProfile(profile)
	for _, artifact := range normalized.Artifacts {
		if artifact.RuntimeArtifactKind() == kind {
			return true
		}
	}
	return false
}

func (artifact RuntimeArtifactPreflightArtifact) RuntimeArtifactKind() string {
	artifact = NormalizeRuntimeArtifactPreflightArtifact(artifact)
	switch artifact.Kind {
	case "trace", "log":
		return artifact.Kind
	}
	if kind := RuntimeArtifactPathKind(artifact.Source); kind != "" {
		return kind
	}
	return RuntimeArtifactPathKindInText(artifact.Source)
}

func (profile RuntimeArtifactPreflightProfile) SourceNavigationOptionalForAnalyze() bool {
	normalized := NormalizeRuntimeArtifactPreflightProfile(profile)
	return normalized.Active && normalized.SourceNavigationOptional
}
