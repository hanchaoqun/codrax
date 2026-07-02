package types

import (
	"fmt"
	"sort"
	"strings"
)

const (
	RuntimeArtifactAnalysisPolicyNone                   = ""
	RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact = "trace_only_exact_artifact"
	RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous = "trace_artifact_ambiguous"
	RuntimeArtifactAnalysisPolicySelectionAdvisory      = "runtime_artifact_selection_advisory"

	runtimeArtifactSelectionReasonNoArtifact      = "no_runtime_artifact"
	runtimeArtifactSelectionReasonSingleTraceOnly = "single_trace_with_current_source_excluded"
	runtimeArtifactSelectionReasonAmbiguousTrace  = "multiple_trace_artifacts"
	runtimeArtifactSelectionReasonMixedRuntime    = "mixed_runtime_artifacts"
)

// RuntimeArtifactSelectionView is the lane-neutral typed view over runtime
// artifacts available to the current turn. It is deliberately built from
// attachment/preflight/RequestModel carriers only; raw user text, objective
// prose, model rationale, tool summaries, and localized UI logs are not inputs.
type RuntimeArtifactSelectionView struct {
	Active     bool                           `json:"active,omitempty"`
	Items      []RuntimeArtifactSelectionItem `json:"items,omitempty"`
	TraceCount int                            `json:"trace_count,omitempty"`
	LogCount   int                            `json:"log_count,omitempty"`
	Policy     RuntimeArtifactAnalysisPolicy  `json:"policy,omitempty"`
}

type RuntimeArtifactSelectionItem struct {
	ID         string   `json:"id,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Source     string   `json:"source,omitempty"`
	Carriers   []string `json:"carriers,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Status     string   `json:"status,omitempty"`
}

type RuntimeArtifactAnalysisPolicy struct {
	Kind                    string `json:"kind,omitempty"`
	ReasonCode              string `json:"reason_code,omitempty"`
	CurrentSourceExcluded   bool   `json:"current_source_excluded,omitempty"`
	ActiveArtifactID        string `json:"active_artifact_id,omitempty"`
	ActiveArtifactSource    string `json:"active_artifact_source,omitempty"`
	AmbiguousTraceArtifacts int    `json:"ambiguous_trace_artifacts,omitempty"`
}

func RuntimeArtifactSelectionViewFromAgentContext(ctx *AgentContext) RuntimeArtifactSelectionView {
	if ctx == nil {
		return RuntimeArtifactSelectionView{Policy: RuntimeArtifactAnalysisPolicy{ReasonCode: runtimeArtifactSelectionReasonNoArtifact}}
	}
	builder := runtimeArtifactSelectionBuilder{items: map[string]RuntimeArtifactSelectionItem{}}
	builder.addPreflight(ctx.RuntimeArtifactPreflight)
	if strings.TrimSpace(ctx.AttachedLog) != "" {
		builder.add("log", "attached_log", "attached_log", 1.0)
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" || strings.TrimSpace(ctx.AttachedHitraceSource) != "" {
		source := firstNonEmptyRuntimeArtifactSelectionString(ctx.AttachedHitraceSource, "attached_trace")
		builder.add("trace", source, "attached_trace", 1.0)
	}
	if ctx.PerfTrace != nil {
		builder.add("trace", runtimeArtifactSelectionPerfSource(ctx.PerfTrace), "perf_trace", 1.0)
	}
	if ctx.Mutable != nil {
		if perf := ctx.Mutable.PerfTrace(); perf != nil && ctx.PerfTrace == nil {
			builder.add("trace", runtimeArtifactSelectionPerfSource(perf), "mutable_perf_trace", 1.0)
		}
		if log := ctx.Mutable.LogTriage(); log != nil && strings.TrimSpace(ctx.AttachedLog) == "" {
			builder.add("log", "log_triage", "mutable_log_triage", 1.0)
		}
	}
	if ctx.AnalysisIR != nil {
		builder.addRequestModel(ctx.AnalysisIR.RequestModel)
	}
	view := builder.view()
	rm := (*RequestModel)(nil)
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	view.Policy = deriveRuntimeArtifactAnalysisPolicy(view, rm, ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo())
	return view
}

func (view RuntimeArtifactSelectionView) ShouldRender() bool {
	if !view.Active {
		return false
	}
	return view.TraceCount+view.LogCount > 1 ||
		view.Policy.Kind == RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact ||
		view.Policy.Kind == RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous
}

func (view RuntimeArtifactSelectionView) SingleTraceArtifact() (RuntimeArtifactSelectionItem, bool) {
	var out RuntimeArtifactSelectionItem
	for _, item := range view.Items {
		if item.Kind != "trace" {
			continue
		}
		if out.ID != "" {
			return RuntimeArtifactSelectionItem{}, false
		}
		out = item
	}
	return out, out.ID != ""
}

func deriveRuntimeArtifactAnalysisPolicy(view RuntimeArtifactSelectionView, rm *RequestModel, zeroCurrentSourceRepo bool) RuntimeArtifactAnalysisPolicy {
	// Two precise signals open the current-source-excluded policy family: the
	// analyzer-emitted explicit user boundary (ExcludesCurrentSource), and the
	// deterministic run-entry census proving the checkout holds no current
	// source at all. The census keeps trace-only steering (grep/read_file →
	// trace_query pullback) alive when the analyzer fails to emit the policy —
	// in a zero-source checkout there is no source lane the exclusion could
	// wrongly suppress.
	currentSourceExcluded := zeroCurrentSourceRepo ||
		(rm != nil && rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource())
	if !view.Active {
		return RuntimeArtifactAnalysisPolicy{ReasonCode: runtimeArtifactSelectionReasonNoArtifact, CurrentSourceExcluded: currentSourceExcluded}
	}
	if currentSourceExcluded {
		if item, ok := view.SingleTraceArtifact(); ok {
			return RuntimeArtifactAnalysisPolicy{
				Kind:                  RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact,
				ReasonCode:            runtimeArtifactSelectionReasonSingleTraceOnly,
				CurrentSourceExcluded: true,
				ActiveArtifactID:      item.ID,
				ActiveArtifactSource:  item.Source,
			}
		}
		if view.TraceCount > 1 {
			return RuntimeArtifactAnalysisPolicy{
				Kind:                    RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous,
				ReasonCode:              runtimeArtifactSelectionReasonAmbiguousTrace,
				CurrentSourceExcluded:   true,
				AmbiguousTraceArtifacts: view.TraceCount,
			}
		}
	}
	if view.TraceCount > 1 {
		return RuntimeArtifactAnalysisPolicy{
			Kind:                    RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous,
			ReasonCode:              runtimeArtifactSelectionReasonAmbiguousTrace,
			CurrentSourceExcluded:   currentSourceExcluded,
			AmbiguousTraceArtifacts: view.TraceCount,
		}
	}
	if view.TraceCount > 0 && view.LogCount > 0 {
		return RuntimeArtifactAnalysisPolicy{
			Kind:                  RuntimeArtifactAnalysisPolicySelectionAdvisory,
			ReasonCode:            runtimeArtifactSelectionReasonMixedRuntime,
			CurrentSourceExcluded: currentSourceExcluded,
		}
	}
	return RuntimeArtifactAnalysisPolicy{ReasonCode: runtimeArtifactSelectionReasonNoArtifact, CurrentSourceExcluded: currentSourceExcluded}
}

type runtimeArtifactSelectionBuilder struct {
	items map[string]RuntimeArtifactSelectionItem
}

func (b *runtimeArtifactSelectionBuilder) addPreflight(profile RuntimeArtifactPreflightProfile) {
	profile = NormalizeRuntimeArtifactPreflightProfile(profile)
	for _, artifact := range profile.Artifacts {
		kind := artifact.RuntimeArtifactKind()
		if kind == "" {
			continue
		}
		b.add(kind, firstNonEmptyRuntimeArtifactSelectionString(artifact.Source, artifact.Detail), firstNonEmptyRuntimeArtifactSelectionString(artifact.Carrier, "runtime_preflight"), 0.95)
	}
}

func (b *runtimeArtifactSelectionBuilder) addRequestModel(rm RequestModel) {
	if rm.PerfTrace != nil {
		b.add("trace", runtimeArtifactSelectionPerfSource(rm.PerfTrace), "request_model_perf_trace", 1.0)
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		kind := RuntimeArtifactPathKind(hint.Path)
		if kind == "" {
			kind = RuntimeArtifactPathKindInText(hint.Path)
		}
		if kind == "" {
			continue
		}
		conf := hint.Confidence
		if conf <= 0 {
			conf = 0.8
		}
		b.add(kind, hint.Path, "required_file_hint", conf)
	}
	for _, raw := range rm.runtimeArtifactPathReferenceCandidates() {
		for _, token := range RuntimeArtifactPathTokensInText(raw) {
			if kind := RuntimeArtifactPathKind(token); kind != "" {
				b.add(kind, token, "request_model_runtime_reference", 0.9)
			}
		}
	}
}

func (b *runtimeArtifactSelectionBuilder) add(kind, source, carrier string, confidence float64) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	source = strings.TrimSpace(source)
	carrier = strings.TrimSpace(carrier)
	if kind != "trace" && kind != "log" {
		return
	}
	if source == "" {
		source = kind
	}
	if carrier == "" {
		carrier = "typed_artifact"
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	key := strings.ToLower(kind + "\x00" + source)
	item, ok := b.items[key]
	if !ok {
		item = RuntimeArtifactSelectionItem{
			ID:         "runtime_artifact:" + RuntimeArtifactHashString(kind+"\x00"+source),
			Kind:       kind,
			Source:     source,
			Confidence: confidence,
			Status:     "available",
		}
	}
	item.Carriers = appendRuntimeArtifactSelectionCarrier(item.Carriers, carrier)
	if confidence > item.Confidence {
		item.Confidence = confidence
	}
	b.items[key] = item
}

func (b *runtimeArtifactSelectionBuilder) view() RuntimeArtifactSelectionView {
	items := make([]RuntimeArtifactSelectionItem, 0, len(b.items))
	for _, item := range b.items {
		sort.Strings(item.Carriers)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind > items[j].Kind
		}
		return items[i].Source < items[j].Source
	})
	out := RuntimeArtifactSelectionView{Items: items}
	for _, item := range items {
		switch item.Kind {
		case "trace":
			out.TraceCount++
		case "log":
			out.LogCount++
		}
	}
	out.Active = len(items) > 0
	return out
}

func appendRuntimeArtifactSelectionCarrier(carriers []string, carrier string) []string {
	for _, existing := range carriers {
		if existing == carrier {
			return carriers
		}
	}
	return append(carriers, carrier)
}

func runtimeArtifactSelectionPerfSource(perf *PerfBundle) string {
	if perf == nil {
		return "perf_trace"
	}
	source := strings.TrimSpace(perf.Meta.Source)
	if source == "" {
		return "perf_trace"
	}
	return fmt.Sprintf("perf_trace:%s", source)
}

func firstNonEmptyRuntimeArtifactSelectionString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
