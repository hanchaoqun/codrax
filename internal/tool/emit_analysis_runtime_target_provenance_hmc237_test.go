package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func executeRuntimeTargetProvenanceHMC237(t *testing.T, request, quote string, targets []any, lookups []types.RuntimeThreadLookup) (types.ToolResult, *types.BusContext) {
	t.Helper()
	payload := b1793TargetRosterPayload(t, true)
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": quote, "confidence": .95}
	payload["runtime_targets"], payload["runtime_thread_lookups"] = targets, lookups
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(data)
	ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
	out, err := (&EmitAnalysis{}).Execute(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Fatal("identity validation rewrote submitted JSON")
	}
	return out, ctx
}

func runtimeTargetProvenanceRowHMC237(kind string, pid int, name string) map[string]any {
	row := map[string]any{"kind": kind, "source": "user_explicit", "confidence": .95}
	if pid > 0 {
		row["pid"] = pid
	}
	if name != "" {
		row["thread"] = name
	}
	return row
}

func TestRuntimeTargetProvenanceRejectsParentPIDPublicHMC237(t *testing.T) {
	const request = "请看附件中1到2秒文档工作线程（tid 11）的采样调用路径。"
	lookup := []types.RuntimeThreadLookup{{PID: 11, Thread: "document-worker", SourceQuote: "（tid 11）"}}
	row := runtimeTargetProvenanceRowHMC237("thread", 10, "document-worker")
	out, ctx := executeRuntimeTargetProvenanceHMC237(t, request, "tid 11", []any{row}, lookup)
	if out.Success || ctx.Mutable.RequestModel() != nil || !strings.Contains(out.Summary, "runtime_targets[0]") || !strings.Contains(out.Summary, "source_quote") {
		t.Fatalf("parent PID acquired authority from another TID's provenance: success=%t model=%+v summary=%s", out.Success, ctx.Mutable.RequestModel(), out.Summary)
	}
	row["pid"] = 11
	out, ctx = executeRuntimeTargetProvenanceHMC237(t, request, "tid 11", []any{row}, lookup)
	rm := ctx.Mutable.RequestModel()
	if !out.Success || rm == nil || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 11 || len(rm.RuntimeThreadLookups) != 1 || rm.RuntimeThreadLookups[0].PID != 11 {
		t.Fatalf("model-owned identity-only repair did not converge: %+v %s", rm, out.Summary)
	}
}

func TestRuntimeTargetProvenancePublicMatrixHMC237(t *testing.T) {
	row := runtimeTargetProvenanceRowHMC237
	for _, tc := range []struct {
		name, quote string
		targets     []any
		valid       bool
	}{
		{"exact_tid", "tid 11", []any{row("thread", 11, "worker")}, true},
		{"exact_process", "process 10", []any{row("process", 10, "")}, true},
		{"process_not_child_tid", "tid 11", []any{row("process", 10, "worker")}, false},
		{"process_display_not_reparsed", "process 10", []any{row("process", 10, "worker-11")}, true},
		{"name_cannot_prove_pid", "worker", []any{row("thread", 11, "worker")}, false},
		{"name_only", "worker", []any{row("thread", 0, "worker")}, true},
		{"longer_name", "background_worker", []any{row("thread", 0, "worker")}, false},
		{"numeric_substring", "tid 310", []any{row("thread", 10, "")}, false},
		{"numeric_prefix", "tid 100", []any{row("thread", 10, "")}, false},
		{"numeric_name_not_id", "Worker10", []any{row("thread", 10, "")}, false},
		{"qualified_number_not_id", "pkg.10", []any{row("thread", 10, "")}, false},
		{"lookup_quote_cannot_authorize_subject", "tid 11", []any{row("thread", 99, "worker")}, false},
		{"canonical_selector", "worker-11", []any{row("thread", 11, "worker-11")}, true},
		{"decorated_selector", "worker-11 (tid=11)", []any{row("thread", 11, "worker-11 (tid=11)")}, true},
		{"same_name_two_ids", "worker tid 10 and worker tid 11", []any{row("thread", 10, "worker"), row("thread", 11, "worker")}, true},
		{"second_id_unproved", "worker tid 10", []any{row("thread", 10, "worker"), row("thread", 11, "worker")}, false},
		{"process_and_thread_independent", "process 10 and thread 11", []any{row("process", 10, ""), row("thread", 11, "")}, true},
		{"same_numeric_distinct_kinds", "process 11 and thread 11", []any{row("process", 11, ""), row("thread", 11, "")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A separately mentioned lookup is never the answer-subject identity
			// or evidence that another subject with the same name is incorrect.
			request := "Analyze " + tc.quote + "; also inspect worker-99's process."
			lookup := []types.RuntimeThreadLookup{{PID: 99, Thread: "worker", SourceQuote: "worker-99"}}
			out, ctx := executeRuntimeTargetProvenanceHMC237(t, request, tc.quote, tc.targets, lookup)
			rm := ctx.Mutable.RequestModel()
			if out.Success != tc.valid || (!tc.valid && rm != nil) {
				t.Fatalf("valid=%t result=%t model=%+v summary=%s", tc.valid, out.Success, rm, out.Summary)
			}
			if !tc.valid {
				if !strings.Contains(out.Summary, "identity is not supported by runtime_target_profile.source_quote") {
					t.Fatalf("did not reach exact identity provenance: %s", out.Summary)
				}
				return
			}
			if len(rm.RuntimeTargets) != len(tc.targets) || !reflect.DeepEqual(rm.RuntimeThreadLookups, lookup) {
				t.Fatalf("subjects merged or lookup authority changed: %+v", rm)
			}
			for i, target := range rm.RuntimeTargets {
				if string(target.Kind) != tc.targets[i].(map[string]any)["kind"] {
					t.Fatalf("target kind was rewritten: %+v", target)
				}
			}
		})
	}
}

func TestRuntimeTargetProvenanceDoesNotReparseNormalizedNameHMC237(t *testing.T) {
	// This helper consumes normalized identities; it must not reparse the comm
	// as a second coordinate. The analyzer's separate strict selector grammar
	// is unchanged; this is not a claim that every query spelling is accepted
	// as analyzer input.
	target := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 11, Thread: "worker-3"}
	if !runtimeTargetIdentityInQuote(target, "tid 11") || runtimeTargetIdentityInQuote(target, "worker-3") {
		t.Fatal("display name replaced normalized positive identity provenance")
	}
}
