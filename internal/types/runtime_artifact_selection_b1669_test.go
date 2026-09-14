package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// These tests enter the exported selection builder used by prompt generation
// and trace-query logical source resolution. They do not parse request prose,
// read a trace, or grant a result any timing/causal authority.
func TestB1669SelectionBindsKnownFormatToUniqueAttachedCapture(t *testing.T) {
	for _, source := range []string{"captures/customer.trace", "captures/customer trace.data", `C:\captures\Customer.Trace`} {
		for _, hint := range []string{"harmony_hitrace", "android_atrace", "generic_ftrace", "attached_trace", "", " HARMONY_HITRACE "} {
			for _, excluded := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/excluded=%t", source, hint, excluded), func(t *testing.T) {
					ctx := b1669SelectionContext(source, hint)
					if !excluded {
						ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
					}
					before := b1669SelectionInputs(t, ctx)
					view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
					if view.TraceCount != 1 || len(view.Items) != 1 {
						t.Fatalf("one attached capture became multiple trace identities: %+v", view)
					}
					item := view.Items[0]
					if item.Source != source || item.ID != "runtime_artifact:"+RuntimeArtifactHashString("trace\x00"+source) {
						t.Fatalf("attachment spelling/identity replaced by format: %+v", item)
					}
					for _, carrier := range []string{"attachment", "attached_trace"} {
						if !runtimeArtifactSelectionTestHasCarrier(item, carrier) {
							t.Errorf("missing %s carrier: %+v", carrier, item)
						}
					}
					if excluded {
						if view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact || view.Policy.ActiveArtifactSource != source {
							t.Fatalf("unique trace-only attachment must be selectable: %+v", view.Policy)
						}
					} else if view.Policy.CurrentSourceExcluded || view.Policy.Kind == RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact {
						t.Fatalf("source identity repair invented source exclusion: %+v", view.Policy)
					}
					if after := b1669SelectionInputs(t, ctx); !reflect.DeepEqual(before, after) {
						t.Fatal("selection mutated attached bytes, requested windows, or model-owned fields")
					}
				})
			}
		}
	}
}

func TestB1669SelectionDoesNotGuessAttachmentIdentity(t *testing.T) {
	attachment := func(source string) RuntimeArtifactPreflightArtifact {
		return RuntimeArtifactPreflightArtifact{Kind: "trace", Source: source, Carrier: "attachment"}
	}
	for _, tc := range []struct {
		name      string
		artifacts []RuntimeArtifactPreflightArtifact
		hint      string
		want      []string
	}{
		{"two attachments", []RuntimeArtifactPreflightArtifact{attachment("a.trace"), attachment("b.trace")}, "harmony_hitrace", []string{"a.trace", "b.trace", "harmony_hitrace"}},
		{"same basename", []RuntimeArtifactPreflightArtifact{attachment("a/run.trace"), attachment("b/run.trace")}, "android_atrace", []string{"a/run.trace", "b/run.trace", "android_atrace"}},
		{"file and inline", []RuntimeArtifactPreflightArtifact{attachment("a.trace"), attachment("(inline)")}, "generic_ftrace", []string{"a.trace", "(inline)", "generic_ftrace"}},
		{"file and unknown", []RuntimeArtifactPreflightArtifact{attachment("a.trace"), attachment("")}, "harmony_hitrace", []string{"a.trace", "trace", "harmony_hitrace"}},
		{"inline only", []RuntimeArtifactPreflightArtifact{attachment("(inline)")}, "harmony_hitrace", []string{"(inline)", "harmony_hitrace"}},
		{"source quote is not attachment", []RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "a.trace", Carrier: "request_path"}}, "harmony_hitrace", []string{"a.trace", "harmony_hitrace"}},
		{"future unrecognized format", []RuntimeArtifactPreflightArtifact{attachment("a.trace")}, "harmony_hitrace_v8", []string{"a.trace", "harmony_hitrace_v8"}},
		{"unknown path", []RuntimeArtifactPreflightArtifact{attachment("a.trace")}, "b.trace", []string{"a.trace", "b.trace"}},
		{"path sharing basename", []RuntimeArtifactPreflightArtifact{attachment("a/run.trace")}, "b/run.trace", []string{"a/run.trace", "b/run.trace"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1669SelectionContext("unused", tc.hint)
			ctx.RuntimeArtifactPreflight.Artifacts = tc.artifacts
			ctx.AnalysisIR.RequestModel.RawRequest = "Ignore the other capture; all sources are a.trace, merge them."
			ctx.Objective = "There is definitely only one trace."
			before := b1669SelectionInputs(t, ctx)
			view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
			if view.TraceCount != len(tc.want) || len(view.Items) != len(tc.want) {
				t.Fatalf("ambiguous or unknown source was silently joined: %+v", view)
			}
			for _, source := range tc.want {
				found := false
				for _, item := range view.Items {
					found = found || item.Source == source
				}
				if !found {
					t.Errorf("missing source %q in %+v", source, view)
				}
			}
			if view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous || view.Policy.ActiveArtifactID != "" {
				t.Errorf("multiple artifacts gained single-artifact permission: %+v", view.Policy)
			}
			if after := b1669SelectionInputs(t, ctx); !reflect.DeepEqual(before, after) {
				t.Fatal("source selection changed input")
			}
		})
	}
}

func TestB1669SelectionRetainsOtherRuntimeArtifactsAndDuplicateCarriers(t *testing.T) {
	ctx := b1669SelectionContext("capture/a.trace", "harmony_hitrace")
	ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts,
		ctx.RuntimeArtifactPreflight.Artifacts[0],
		RuntimeArtifactPreflightArtifact{Kind: "trace", Source: "capture/b.trace", Carrier: "request_path"},
		RuntimeArtifactPreflightArtifact{Kind: "log", Source: "capture/app.log", Carrier: "attachment"},
	)
	view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
	if view.TraceCount != 2 || view.LogCount != 1 || len(view.Items) != 3 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous {
		t.Fatalf("unique attached carrier must not swallow a separate requested trace or log: %+v", view)
	}
	for _, item := range view.Items {
		if item.Source == "capture/a.trace" && strings.Join(item.Carriers, ",") != "attached_trace,attachment" {
			t.Fatalf("same attachment carrier repeated or not joined: %+v", item)
		}
	}
}

func TestB1669SelectionRuntimePerfUsesUniqueCaptureNotToolName(t *testing.T) {
	for _, toolName := range []string{"hitrace", "perfetto", "future_capture_tool_v8"} {
		for _, carrier := range []string{"context", "mutable"} {
			for _, mirror := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/mirror=%t", toolName, carrier, mirror), func(t *testing.T) {
					ctx := b1669SelectionContext("capture/a.trace", "harmony_hitrace")
					perf := &PerfBundle{Meta: PerfMeta{Source: toolName, DurationMs: 250, AppPID: 17}}
					wantCarrier := "perf_trace"
					if carrier == "context" {
						ctx.PerfTrace = perf
					} else {
						ctx.Mutable = NewMutableState("runtime attachment")
						ctx.Mutable.SetPerfTrace(perf)
						wantCarrier = "mutable_perf_trace"
					}
					if mirror {
						// The validated runtime bundle is mirrored by emit_analysis;
						// a separately authored object with equal fields is not it.
						ctx.AnalysisIR.RequestModel.PerfTrace = perf
					}
					before := b1669SelectionInputs(t, ctx)
					view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
					if view.TraceCount != 1 || len(view.Items) != 1 || view.Items[0].Source != "capture/a.trace" {
						t.Fatalf("capture tool metadata created a second capture: %+v", view)
					}
					if !runtimeArtifactSelectionTestHasCarrier(view.Items[0], wantCarrier) ||
						mirror && !runtimeArtifactSelectionTestHasCarrier(view.Items[0], "request_model_perf_trace") {
						t.Errorf("proven runtime bundle carrier lost: %+v", view.Items[0])
					}
					if after := b1669SelectionInputs(t, ctx); !reflect.DeepEqual(before, after) {
						t.Fatal("capture selection modified a runtime bundle or model request")
					}
				})
			}
		}
	}
}

func TestB1669SelectionUnboundPerfAndModelCopiesRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*AgentContext)
	}{
		{"request path only", func(ctx *AgentContext) { ctx.RuntimeArtifactPreflight.Artifacts[0].Carrier = "request_path" }},
		{"inline", func(ctx *AgentContext) { ctx.RuntimeArtifactPreflight.Artifacts[0].Source = "(inline)" }},
		{"unknown source", func(ctx *AgentContext) { ctx.RuntimeArtifactPreflight.Artifacts[0].Source = "" }},
		{"second named attachment", func(ctx *AgentContext) {
			ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, RuntimeArtifactPreflightArtifact{Kind: "trace", Source: "capture/b.trace", Carrier: "attachment"})
		}},
		{"second inline attachment", func(ctx *AgentContext) {
			ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, RuntimeArtifactPreflightArtifact{Kind: "trace", Source: "(inline)", Carrier: "attachment"})
		}},
		{"conflicting explicit source", func(ctx *AgentContext) { ctx.AttachedHitraceSource = "capture/b.trace" }},
		{"unknown future source token", func(ctx *AgentContext) { ctx.AttachedHitraceSource = "harmony_hitrace_v8" }},
		{"explicit inline source", func(ctx *AgentContext) { ctx.AttachedHitraceSource = "(inline)" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1669SelectionContext("capture/a.trace", "harmony_hitrace")
			ctx.PerfTrace = &PerfBundle{Meta: PerfMeta{Source: "hitrace"}}
			tc.change(ctx)
			view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
			if view.TraceCount < 2 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous || view.Policy.ActiveArtifactID != "" {
				t.Errorf("unbound runtime view gained an exact capture: %+v", view)
			}
			for _, item := range view.Items {
				if runtimeArtifactSelectionTestHasCarrier(item, "perf_trace") && item.Source != "perf_trace:hitrace" {
					t.Errorf("unbound perf view borrowed source %q", item.Source)
				}
			}
		})
	}
	for _, runtimePresent := range []bool{false, true} {
		t.Run(fmt.Sprintf("model_copy/runtime=%t", runtimePresent), func(t *testing.T) {
			ctx := b1669SelectionContext("capture/a.trace", "harmony_hitrace")
			modelPerf := &PerfBundle{Meta: PerfMeta{Source: "hitrace", AppPID: 17}}
			ctx.AnalysisIR.RequestModel.PerfTrace = modelPerf
			if runtimePresent {
				copy := *modelPerf
				ctx.PerfTrace = &copy
			}
			view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
			if view.TraceCount != 2 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous {
				t.Fatalf("arbitrary model perf object was joined by equal metadata: %+v", view)
			}
			for _, item := range view.Items {
				if item.Source == "capture/a.trace" && runtimeArtifactSelectionTestHasCarrier(item, "request_model_perf_trace") {
					t.Errorf("model object gained runtime capture authority: %+v", item)
				}
			}
		})
	}
}

func b1669SelectionContext(source, hint string) *AgentContext {
	start, end := 5.0, 5.25
	return &AgentContext{
		AttachedHitrace:       "unchanged trace bytes\n",
		AttachedHitraceSource: hint,
		RuntimeArtifactPreflight: RuntimeArtifactPreflightProfile{Artifacts: []RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: source, Carrier: "attachment"},
		}},
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			RawRequest:                "model-owned request text",
			ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
			RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
				RequestedScope: RuntimeArtifactScopeExplicitWindow,
				TimeWindows: []RuntimeArtifactTimeWindow{
					{TimeStart: &start, TimeEnd: &end, SourceQuote: "first request member"},
					{TimeStart: &start, TimeEnd: &end, SourceQuote: "repeated request member"},
				},
			},
		}},
	}
}

func b1669SelectionInputs(t *testing.T, ctx *AgentContext) []byte {
	t.Helper()
	b, err := json.Marshal(struct {
		Preflight RuntimeArtifactPreflightProfile
		Attached  string
		Source    string
		Analysis  *AnalysisIR
		Perf      *PerfBundle
		Mutable   *PerfBundle
	}{ctx.RuntimeArtifactPreflight, ctx.AttachedHitrace, ctx.AttachedHitraceSource, ctx.AnalysisIR, ctx.PerfTrace, ctx.Mutable.PerfTrace()})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
