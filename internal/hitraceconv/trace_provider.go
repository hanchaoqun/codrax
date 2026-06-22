package hitraceconv

import (
	"strconv"
	"strings"
)

const (
	traceEngineAuto          = "auto"
	traceEngineTraceStreamer = "trace_streamer"
	traceEngineBuiltin       = "builtin"

	traceProviderStageTraceBody = "trace_body"

	traceProviderKindOfficialDB = "official_trace_db"
	traceProviderKindBuiltin    = "builtin_modern"
	traceProviderKindLegacy     = "legacy_event_segment_archival"

	traceProviderNameTraceStreamer = "trace_streamer_db"
	traceProviderNameBuiltinModern = "codrax_builtin_modern_profiler"
	traceProviderNameLegacySegment = "codrax_legacy_event_segment_archival"
)

type traceProviderSpec struct {
	Kind        string
	Name        string
	Fallback    bool
	Implemented bool
}

var traceProviderRegistry = []traceProviderSpec{
	{
		Kind:        traceProviderKindOfficialDB,
		Name:        traceProviderNameTraceStreamer,
		Implemented: false,
	},
	{
		Kind:        traceProviderKindBuiltin,
		Name:        traceProviderNameBuiltinModern,
		Fallback:    true,
		Implemented: true,
	},
	{
		Kind:        traceProviderKindLegacy,
		Name:        traceProviderNameLegacySegment,
		Fallback:    true,
		Implemented: true,
	},
}

func traceProviderByName(name string) traceProviderSpec {
	for _, provider := range traceProviderRegistry {
		if provider.Name == name {
			return provider
		}
	}
	return traceProviderSpec{Name: strings.TrimSpace(name)}
}

func normalizeTraceEngineMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

func validateTraceEngineMode(mode string) error {
	switch normalizeTraceEngineMode(mode) {
	case "", traceEngineAuto, traceEngineTraceStreamer, traceEngineBuiltin:
		return nil
	default:
		return errUnsupportedTraceEngine(mode)
	}
}

func errUnsupportedTraceEngine(mode string) error {
	return &traceEngineError{mode: mode}
}

type traceEngineError struct {
	mode string
}

func (e *traceEngineError) Error() string {
	return "unsupported trace engine mode " + strconv.Quote(e.mode) + "; use auto, trace_streamer, or builtin"
}

func newTraceProviderDecision(stage string, provider traceProviderSpec, opts Options, inputPath, outputPath string) TraceProviderDecision {
	mode := normalizeTraceEngineMode(opts.TraceEngine)
	if mode == "" {
		mode = traceEngineAuto
	}
	return TraceProviderDecision{
		Stage:        stage,
		ProviderKind: provider.Kind,
		ProviderName: provider.Name,
		InputPath:    inputPath,
		OutputPath:   outputPath,
		EngineMode:   mode,
		Fallback:     provider.Fallback,
	}
}

func traceProviderSuccess(decision TraceProviderDecision, artifact Artifact) TraceProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Succeeded = true
	decision.ArtifactPath = artifact.Path
	decision.TraceQueryReady = artifact.Type == ArtifactSystrace || artifact.Type == ArtifactTraceBundle
	if artifact.Type == ArtifactTraceDB {
		decision.DBPath = artifact.Path
	}
	return decision
}

func traceProviderSkipped(decision TraceProviderDecision, selected bool, reason, caveat string) TraceProviderDecision {
	decision.Selected = selected
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}

func traceProviderFailure(decision TraceProviderDecision, reason, caveat string) TraceProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}
