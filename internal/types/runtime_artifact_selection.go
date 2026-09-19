package types

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
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
	builder := runtimeArtifactSelectionBuilder{
		items:           map[string]RuntimeArtifactSelectionItem{},
		preparedAliases: runtimeArtifactSelectionPreparedAliases(ctx),
	}
	builder.addPreflight(ctx.RuntimeArtifactPreflight)
	attachedCapture := runtimeArtifactSelectionUniqueAttachedSource(ctx.RuntimeArtifactPreflight)
	attachedHint := firstNonEmptyRuntimeArtifactSelectionString(ctx.AttachedHitraceSource, "attached_trace")
	attachedCapture = builder.traceSource(attachedCapture)
	attachedHint = builder.traceSource(attachedHint)
	if ctx.AttachedTraceMaterial != nil && ctx.AttachedTraceMaterial.Validate(context.Background(), ctx.AttachedHitrace) == nil && attachedCapture == "" && runtimeArtifactSelectionAttachedFormat(attachedHint) {
		// A valid in-process receipt proves the current attachment's exact
		// complete material even when no run-entry census was supplied.
		attachedCapture = ctx.AttachedTraceMaterial.QueryPath()
	}
	if !runtimeArtifactSelectionAttachedFormat(attachedHint) && !runtimeArtifactSelectionSameSource(attachedHint, attachedCapture) {
		// An explicit conflicting path/inline marker or an unrecognized union
		// value cannot identify the one preflight attachment, including for
		// a structured perf view produced through that attached channel.
		attachedCapture = ""
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" {
		builder.add("log", "attached_log", "attached_log", 1.0)
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" || strings.TrimSpace(ctx.AttachedHitraceSource) != "" {
		source := attachedHint
		if runtimeArtifactSelectionAttachedFormat(source) && attachedCapture != "" {
			source = attachedCapture
		}
		builder.add("trace", source, "attached_trace", 1.0)
	}
	runtimePerf := ctx.PerfTrace
	if ctx.PerfTrace != nil {
		builder.add("trace", runtimeArtifactSelectionBoundPerfSource(ctx.PerfTrace, attachedCapture), "perf_trace", 1.0)
	}
	if ctx.Mutable != nil {
		if perf := ctx.Mutable.PerfTrace(); perf != nil && ctx.PerfTrace == nil {
			runtimePerf = perf
			builder.add("trace", runtimeArtifactSelectionBoundPerfSource(perf, attachedCapture), "mutable_perf_trace", 1.0)
		}
		if log := ctx.Mutable.LogTriage(); log != nil && strings.TrimSpace(ctx.AttachedLog) == "" {
			builder.add("log", "log_triage", "mutable_log_triage", 1.0)
		}
	}
	if ctx.AnalysisIR != nil {
		builder.addRequestModel(ctx.AnalysisIR.RequestModel, runtimePerf, attachedCapture)
	}
	view := builder.view()
	rm := (*RequestModel)(nil)
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	view.Policy = deriveRuntimeArtifactAnalysisPolicy(view, rm, ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo())
	return view
}

// AttachedHitraceSource is a legacy union of a capture path and a format hint.
// Only established format/attached-channel tokens may borrow the run-entry
// attachment identity. An explicit path, inline marker, or future unknown
// token must not be guessed to name that capture.
func runtimeArtifactSelectionAttachedFormat(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "attached_trace", "harmony_hitrace", "android_atrace", "generic_ftrace":
		return true
	default:
		return false
	}
}

// This is a channel binding, not a path detector or same-basename join. Keep
// the original preflight spelling so the selection ID and all existing path
// consumers remain unchanged. Unknown/inline attachments participate in the
// census and prevent binding; ignoring them would falsely make a named file
// the unique attachment. Merely request-referenced files provide no binding.
func runtimeArtifactSelectionUniqueAttachedSource(profile RuntimeArtifactPreflightProfile) string {
	var source string
	for _, artifact := range NormalizeRuntimeArtifactPreflightProfile(profile).Artifacts {
		if !strings.EqualFold(artifact.Carrier, "attachment") || artifact.RuntimeArtifactKind() != "trace" {
			continue
		}
		if !runtimeArtifactAttachmentSourceIsAddressable(artifact.Source) {
			return ""
		}
		if source != "" && !runtimeArtifactSelectionSameSource(source, artifact.Source) {
			return ""
		}
		if source == "" {
			source = artifact.Source
		}
	}
	return source
}

func runtimeArtifactSelectionSameSource(a, b string) bool {
	return a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b)
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
	items           map[string]RuntimeArtifactSelectionItem
	preparedAliases map[string]string
}

// Alias only the two physical roles named by a currently valid preparation
// receipt. Other files, bundle children, basenames and request prose do not
// become this capture. Canonical spellings of those exact files are included
// because run-entry inventory may already have resolved a user symlink.
func runtimeArtifactSelectionPreparedAliases(ctx *AgentContext) map[string]string {
	if ctx == nil {
		return nil
	}
	aliases := runtimeArtifactSelectionMaterialAliases(ctx.AttachedTraceMaterial, ctx.AttachedHitrace)
	if aliases == nil {
		aliases = make(map[string]string)
	}
	conflicts := make(map[string]bool)
	if ctx.TraceInputPreparer != nil {
		for _, material := range ctx.TraceInputPreparer.PreparedMaterials() {
			if material == nil {
				continue
			}
			for source, query := range runtimeArtifactSelectionMaterialAliases(material, material.Preview()) {
				if conflicts[source] {
					continue
				}
				if prior, exists := aliases[source]; exists && !runtimeArtifactSelectionSameSource(prior, query) {
					// Two valid receipts disagree about the same physical role.
					// Do not pick a winner or turn a conflict into false single-
					// capture certainty. The independent query paths stay visible.
					delete(aliases, source)
					conflicts[source] = true
					continue
				}
				aliases[source] = query
			}
		}
	}
	return aliases
}

func runtimeArtifactSelectionMaterialAliases(material *attachment.TraceMaterial, preview string) map[string]string {
	if material == nil || material.Validate(context.Background(), preview) != nil {
		return nil
	}
	aliases := make(map[string]string, 4)
	for _, path := range []string{material.SourcePath(), material.QueryPath()} {
		for _, candidate := range []string{path, runtimeArtifactSelectionCanonicalPath(path)} {
			if !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate {
				continue
			}
			if runtime.GOOS == "windows" {
				candidate = strings.ToLower(candidate)
			}
			aliases[candidate] = material.QueryPath()
		}
	}
	if material.Validate(context.Background(), preview) != nil {
		return nil
	}
	return aliases
}

func runtimeArtifactSelectionCanonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(resolved)
}

func (b *runtimeArtifactSelectionBuilder) traceSource(source string) string {
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return source
	}
	key := source
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	if query, ok := b.preparedAliases[key]; ok {
		return query
	}
	return source
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

func (b *runtimeArtifactSelectionBuilder) addRequestModel(rm RequestModel, runtimePerf *PerfBundle, attachedCapture string) {
	if rm.PerfTrace != nil {
		source := runtimeArtifactSelectionPerfSource(rm.PerfTrace)
		// emit_analysis mirrors the validated runtime bundle by pointer. Only
		// that exact object retains the channel binding; equal metadata or a
		// model-authored copy cannot establish shared capture provenance.
		if rm.PerfTrace == runtimePerf {
			source = runtimeArtifactSelectionBoundPerfSource(rm.PerfTrace, attachedCapture)
		}
		b.add("trace", source, "request_model_perf_trace", 1.0)
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
	if kind == "trace" {
		source = b.traceSource(source)
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
	sourceKey := source
	if runtime.GOOS == "windows" {
		sourceKey = strings.ToLower(sourceKey)
	}
	key := kind + "\x00" + sourceKey
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

func runtimeArtifactSelectionBoundPerfSource(perf *PerfBundle, attachedCapture string) string {
	// PerfMeta.Source is the capture tool's name, not a second file. The
	// binding comes from the runtime channel and unique attachment census,
	// never from the spelling (or a future extension) of that tool name.
	if attachedCapture != "" {
		return attachedCapture
	}
	return runtimeArtifactSelectionPerfSource(perf)
}

func firstNonEmptyRuntimeArtifactSelectionString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
