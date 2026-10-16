package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc256IdentityPayload(t *testing.T, request string) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"simple","question_kind":"mechanism","keywords":["records"],"entities":[]}`)), &p); err != nil {
		t.Fatal(err)
	}
	p["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": .95}
	p["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "full_artifact", "source_quote": request, "confidence": .95}
	p["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "fact_families": []string{"other_observed_value"}, "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": .95}
	return p
}

func hmc256EmitIdentity(t *testing.T, bus *types.BusContext, p map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(raw)
	out, err := (&EmitAnalysis{}).Execute(bus, raw)
	if err != nil || !bytes.Equal(before, raw) {
		t.Fatalf("emit error or mutation of submitted JSON: %v", err)
	}
	return out
}

func TestHMC256LookupQuoteRepairPreservesSubmittedIdentitiesPublic(t *testing.T) {
	const request = "在1到2秒查看线程 101 和 202 所属进程的调用位置与次数。"
	bus := &types.BusContext{Mutable: types.NewMutableState(request)}
	bus.Mutable.SetRequestModel(types.RequestModel{RawRequest: request, RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77}}})
	before, _ := json.Marshal(bus.Mutable.RequestModel())
	p := hmc256IdentityPayload(t, request)
	p["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 1, "time_end": 2, "source_quote": "1到2秒", "confidence": .95}
	// The time scope is independent of the identity repair, and is itself
	// anchored in this request; do not recover it from a rejected object.
	p["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .95, "dimensions": []any{
		map[string]any{"label": "调用位置", "role": "observed_value", "source_quote": "调用位置", "required": true, "index": 1},
		map[string]any{"label": "次数", "role": "count", "source_quote": "次数", "required": true, "index": 2},
	}}
	lookups := []types.RuntimeThreadLookup{{PID: 101, SourceQuote: "线程 101"}, {PID: 202, SourceQuote: "线程 202"}}
	p["runtime_thread_lookups"] = lookups
	bad := hmc256EmitIdentity(t, bus, p)
	if bad.Success {
		t.Fatal("a noncontiguous quote was accepted")
	}
	for _, want := range []string{"runtime_thread_lookups[1].source_quote", "not a contiguous verbatim", "Keep the submitted pid/thread", "not yet accepted", "shared verbatim phrase", "withdraw the unproven lookup row rather than deleting its identity fields"} {
		if !strings.Contains(bad.Summary, want) {
			t.Errorf("quote-only error hid its field or encouraged identity deletion; missing %q: %s", want, bad.Summary)
		}
	}
	if strings.Contains(bad.Summary, "valid thread identity") || strings.Contains(bad.Summary, "identity is invalid") {
		t.Errorf("quote-only rejection blamed the structurally valid identity: %s", bad.Summary)
	}
	after, _ := json.Marshal(bus.Mutable.RequestModel())
	if !bytes.Equal(before, after) {
		t.Fatal("rejection partially published new identities")
	}
	lookups[0].SourceQuote, lookups[1].SourceQuote = "线程 101 和 202", "线程 101 和 202"
	if good := hmc256EmitIdentity(t, bus, p); !good.Success {
		t.Fatalf("full model-owned quote repair failed: %s", good.Summary)
	}
	rm := bus.Mutable.RequestModel()
	start, end, scoped := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
	if !reflect.DeepEqual(rm.RuntimeThreadLookups, lookups) || len(rm.RuntimeTargets) != 0 || rm.RuntimeTargetProfile.NamedTarget() || !scoped || start != 1 || end != 2 || !rm.RuntimeQuestionProfile.BoundedFactSet() || len(rm.RequestedAnswerDimensions.Dimensions) != 2 {
		t.Fatalf("repair lost identity/scope/dimensions or promoted locators: %+v", rm)
	}
}

func TestHMC256RuntimeSubjectAndSourceRolesPublic(t *testing.T) {
	for _, currentSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "recorded_only", true: "recorded_and_current_source"}[currentSource], func(t *testing.T) {
			request := "列出线程 101 和 202 的采集调用位置。"
			if currentSource {
				request += "也定位当前实现的源码位置。"
			}
			hint := types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			if currentSource {
				hint.Source = "mixed"
				hint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceRequired
			}
			bus := &types.BusContext{Mutable: types.NewMutableState(request), TurnRouteHint: hint}
			p := hmc256IdentityPayload(t, request)
			p["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "线程 101 和 202", "confidence": .95}
			p["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 101, ""), runtimeTargetProvenanceRowHMC237("thread", 202, "")}
			dims := []any{map[string]any{"label": "采集调用位置", "role": "observed_value", "source_quote": "采集调用位置", "required": true, "index": 1}}
			if currentSource {
				dims = append(dims, map[string]any{"label": "源码位置", "role": "source_location", "source_quote": "当前实现的源码位置", "required": true, "index": 2})
			}
			p["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "dimensions": dims, "confidence": .95}
			if out := hmc256EmitIdentity(t, bus, p); !out.Success {
				t.Fatalf("legal typed role rejected: %s", out.Summary)
			}
			rm := bus.Mutable.RequestModel()
			if len(rm.RuntimeTargets) != 2 || rm.RuntimeTargets[0].PID != 101 || rm.RuntimeTargets[1].PID != 202 || !rm.RuntimeTargetProfile.NamedTarget() || len(rm.RuntimeThreadLookups) != 0 || !rm.RuntimeQuestionProfile.BoundedFactSet() {
				t.Fatalf("fact subjects were demoted to lookups or causal subjects: %+v", rm)
			}
			precision := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, hint)
			want := types.RuntimeSourceRequirementNone
			if currentSource {
				want = types.RuntimeSourceRequirementPrecise
			}
			if precision != want || len(rm.RequestedAnswerDimensions.Dimensions) != len(dims) || rm.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionObservedValue {
				t.Fatalf("typed source responsibility or recorded value changed: precision=%s dims=%+v", precision, rm.RequestedAnswerDimensions)
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed {
				t.Fatal("finite fact subject acquired full causal report authority")
			}
		})
	}
}

func TestHMC256LookupRepairRetainsCausalObligationsPublic(t *testing.T) {
	raw, p := mixedRuntimePayload(t, "causal_attribution", true, true)
	raw += "; locate the processes containing threads 101 and 202"
	lookups := []types.RuntimeThreadLookup{{PID: 101, SourceQuote: "threads 101"}, {PID: 202, SourceQuote: "threads 202"}}
	p["runtime_thread_lookups"] = lookups
	bus := &types.BusContext{Mutable: types.NewMutableState(raw)}
	if bad := hmc256EmitIdentity(t, bus, p); bad.Success || bus.Mutable.RequestModel() != nil || !strings.Contains(bad.Summary, "runtime_thread_lookups[1].source_quote") {
		t.Fatalf("quote rejection was lost or published partial authority: %s", bad.Summary)
	}
	lookups[0].SourceQuote, lookups[1].SourceQuote = "threads 101 and 202", "threads 101 and 202"
	if out := hmc256EmitIdentity(t, bus, p); !out.Success {
		t.Fatalf("causal repair failed: %s", out.Summary)
	}
	rm := bus.Mutable.RequestModel()
	if !reflect.DeepEqual(rm.RuntimeThreadLookups, lookups) || len(rm.RuntimeTargets) != 0 || !rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || !rm.RuntimeQuestionProfile.FrameCausalityRequested || len(rm.RequestedAnswerDimensions.Dimensions) != 3 || !rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		t.Fatalf("repair altered independent work/frame/window obligations: %+v", rm)
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || !allowed {
		t.Fatal("quote repair suppressed explicitly requested Trace causal projection")
	}
}

func TestHMC256LookupFieldDiagnosticsPublic(t *testing.T) {
	const request = "Inspect thread 101 and 202, worker-202 and io-worker; process 9 is separate."
	for _, tc := range []struct {
		name, field, reason string
		lookup              types.RuntimeThreadLookup
		valid               bool
	}{
		{"negative_pid", ".pid", "range", types.RuntimeThreadLookup{PID: -1, SourceQuote: "thread 101"}, false},
		{"large_pid", ".pid", "range", types.RuntimeThreadLookup{PID: types.RuntimeTargetMaxPID + 1, SourceQuote: "thread 101"}, false},
		{"missing_identity", ".pid/thread", "missing", types.RuntimeThreadLookup{SourceQuote: "thread 101"}, false},
		{"conflicting_identity", ".thread", "conflicts with", types.RuntimeThreadLookup{PID: 101, Thread: "worker-202", SourceQuote: "thread 101 and 202, worker-202"}, false},
		{"missing_quote", ".source_quote", "missing", types.RuntimeThreadLookup{PID: 101}, false},
		{"nonverbatim_quote", ".source_quote", "not a contiguous verbatim", types.RuntimeThreadLookup{PID: 202, SourceQuote: "thread 202"}, false},
		{"unproven_identity", ".source_quote", "must contain that exact thread name or TID", types.RuntimeThreadLookup{PID: 777, SourceQuote: "thread 101"}, false},
		{"numeric_prefix", ".source_quote", "must contain that exact thread name or TID", types.RuntimeThreadLookup{PID: 10, SourceQuote: "thread 101"}, false},
		{"name_only", "", "", types.RuntimeThreadLookup{Thread: "io-worker", SourceQuote: "io-worker"}, true},
		{"canonical_identity", "", "", types.RuntimeThreadLookup{PID: 202, Thread: "worker-202", SourceQuote: "worker-202"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := &types.BusContext{Mutable: types.NewMutableState(request)}
			p := hmc256IdentityPayload(t, request)
			p["runtime_thread_lookups"] = []types.RuntimeThreadLookup{tc.lookup}
			out := hmc256EmitIdentity(t, bus, p)
			if out.Success != tc.valid {
				t.Fatalf("valid=%t got=%t: %s", tc.valid, out.Success, out.Summary)
			}
			if tc.valid {
				if len(bus.Mutable.RequestModel().RuntimeTargets) != 0 || !reflect.DeepEqual(bus.Mutable.RequestModel().RuntimeThreadLookups, []types.RuntimeThreadLookup{tc.lookup}) {
					t.Fatal("lookup role was changed")
				}
				return
			}
			if bus.Mutable.RequestModel() != nil || !strings.Contains(out.Summary, "runtime_thread_lookups[0]"+tc.field) || !strings.Contains(out.Summary, tc.reason) {
				t.Fatalf("rejected row published or wrong field diagnosed: %s", out.Summary)
			}
		})
	}
}

func TestHMC256IdentityAndLocationTeachingSingleOwners(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	lookupDescription := schema["properties"].(map[string]any)["runtime_thread_lookups"].(map[string]any)["description"].(string)
	for _, surface := range []string{lookupDescription, skill.BuildAnalysisSkill().OutputFormat} {
		for _, want := range []string{"finite observed facts", "only to locate another answer subject", "does not prove causality"} {
			if !strings.Contains(surface, want) {
				t.Errorf("identity role guidance lacks %q", want)
			}
		}
	}
	if strings.Contains(skill.BuildAnalysisSkill().OutputFormat, "that the answer diagnoses or describes") {
		t.Error("old independent identity decision paragraph was left beside shared teaching")
	}
	for _, want := range []string{"runtime caller address", "stored path or line", "not a current-checkout location", "source_location"} {
		if !strings.Contains(skill.AnalysisObservedDimensionTeaching, want) {
			t.Errorf("recorded/current source role distinction lacks %q", want)
		}
	}
}
