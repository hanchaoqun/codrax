package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the durable report wire surface rather than synthesizing an assertion.
// The project suite passed while a supplementary probe produced no witness.
func b1702ProbeObservationContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := b1634ControllerProofContext(t, nil)
	var report types.ChangeReport
	if err := json.Unmarshal([]byte(b1702ProbeObservationWire(`{"plan_id":"plan-proof-scope","channel":"post_apply_verify","passed":true,"verification_status":"passed","test_results":[{"assertion_id":"project-suite","passed":true,"observation_scope":"aggregate"}],"verification_diagnostics":[{"category":"probe_unavailable","reason_code":"verification_probe_javascript_syntax_error","runner":"verification_probe","outcome":"parser_error","probe_execution_observations":[{"plan_id":"plan-proof-scope","probe_id":"native-check","execution_id":"exec-observed","definition_sha256":"definition-observed","invocation_sha256":"invocation-observed","output_excerpt":"SyntaxError: binding already declared at native-check:14","output_ref":"/outputs/retained-native-check.txt"}]}]}`)), &report); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.SetChangeReport(&report)
	pack := types.WriteContextPackFromChangeReport(&report).WithScope("batch", "slice")
	for i := 0; i < 110; i++ {
		pack.Items = append(pack.Items, types.WriteContextItem{ID: fmt.Sprint("constraint-", i), Kind: "constraint", Priority: types.WriteContextP0, Text: "preserve the public interface"})
	}
	ctx.Mutable.SetWriteContextPack(&pack)
	ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: report.PlanID, ActiveSliceID: "slice"}}})
	return ctx
}

func b1702ProbeObservationWire(raw string) string {
	const receipt = `"executed_commands":[{"runner":"verification_probe","framework":"javascript","source":"pre_suite_verification_probe","outcome":"parser_error","exit_code":1,"probe_execution":{"version":1,"definition_sha256":"definition-observed","invocation_sha256":"invocation-observed","execution_id":"exec-observed","started_at":"2026-09-15T10:00:00Z","finished_at":"2026-09-15T10:00:01Z","repository_root":"/repo","executable":"/bin/node","args":["-e","native program"],"working_dir":"/repo"}}],`
	raw = strings.Replace(raw, `"verification_diagnostics":`, receipt+`"verification_diagnostics":`, 1)
	raw = strings.ReplaceAll(raw, "definition-observed", strings.Repeat("a", 64))
	return strings.ReplaceAll(raw, "invocation-observed", strings.Repeat("b", 64))
}

func b1702ProbeObservationPrompts(ctx *types.AgentContext) map[string]string {
	return map[string]string{
		"controller": (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil),
		"planner":    (&plannerEvaluator{}).BuildInitialInstruction(ctx, nil),
		"verifier":   (&verifierEvaluator{}).BuildInitialInstruction(ctx, nil),
	}
}

func TestB1702ProbeExecutionObservationReachesAllCurrentConsumers(t *testing.T) {
	ctx := b1702ProbeObservationContext(t)
	before := b1122JSON(t, []any{ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), ctx.Mutable.WriteContextPack(), types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), nil)})
	for consumer, prompt := range b1702ProbeObservationPrompts(ctx) {
		for _, want := range []string{"SyntaxError: binding already declared at native-check:14", "/outputs/retained-native-check.txt", "native-check", "exec-observed"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s lost precise execution context %q", consumer, want)
			}
		}
		if got := strings.Count(prompt, "SyntaxError: binding already declared at native-check:14"); got != 1 {
			t.Errorf("%s execution excerpt count=%d, want one independently bounded section", consumer, got)
		}
		if at := strings.Index(prompt, "SyntaxError: binding already declared"); at < 0 || at > strings.Index(prompt, "## Priority write context pack") {
			t.Errorf("%s observation hidden behind ordinary context cap", consumer)
		}
		if !strings.Contains(prompt, "do not change") || !strings.Contains(prompt, "untrusted data") {
			t.Errorf("%s lost non-authoritative boundary", consumer)
		}
	}
	after := b1122JSON(t, []any{ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), ctx.Mutable.WriteContextPack(), types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), nil)})
	if string(before) != string(after) {
		t.Fatal("display changed model plan, report, historical context or proof")
	}
}

func TestB1702ProbeExecutionObservationHistoricalRecoveryAndScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AgentContext)
		want bool
	}{
		{"report_reset", func(ctx *types.AgentContext) { ctx.Mutable.ResetChangeReport() }, true},
		{"plan_and_report_reset", func(ctx *types.AgentContext) { ctx.Mutable.ResetChangePlan(); ctx.Mutable.ResetChangeReport() }, true},
		{"current_empty_supersedes", func(ctx *types.AgentContext) { ctx.Mutable.ChangeReport().VerificationDiagnostics = nil }, false},
		{"foreign_report", func(ctx *types.AgentContext) { ctx.Mutable.ChangeReport().PlanID = "foreign" }, false},
		{"missing_report_plan", func(ctx *types.AgentContext) { ctx.Mutable.ChangeReport().PlanID = "" }, false},
		{"missing_active_plan", func(ctx *types.AgentContext) { ctx.Mutable.ResetChangePlan(); ctx.Mutable.SetWriteWorkflowRun(nil) }, false},
		{"planner_channel", func(ctx *types.AgentContext) {
			ctx.Mutable.ChangeReport().Channel = types.ChangeReportChannelPlannerProbe
		}, false},
		{"foreign_plan", func(ctx *types.AgentContext) {
			ctx.Mutable.ResetChangeReport()
			ctx.Mutable.ChangePlan().ID = "foreign"
		}, false},
		{"foreign_batch", func(ctx *types.AgentContext) {
			ctx.Mutable.ResetChangeReport()
			ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "other", Batches: []types.WriteWorkflowBatch{{ID: "other", PlanID: "plan-proof-scope", ActiveSliceID: "slice"}}})
		}, false},
		{"foreign_slice", func(ctx *types.AgentContext) {
			ctx.Mutable.ResetChangeReport()
			ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: "plan-proof-scope", ActiveSliceID: "other"}}})
		}, false},
		{"missing_scope", func(ctx *types.AgentContext) { ctx.Mutable.ResetChangeReport(); ctx.Mutable.SetWriteWorkflowRun(nil) }, false},
		{"read", func(ctx *types.AgentContext) { ctx.Mode = types.ModeRead }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1702ProbeObservationContext(t)
			tc.edit(ctx)
			before := b1122JSON(t, ctx.Mutable.WriteContextPack())
			for consumer, prompt := range b1702ProbeObservationPrompts(ctx) {
				if consumer == "verifier" && ctx.Mutable.ChangePlan() == nil {
					continue // The verifier still refuses to run without an installed plan.
				}
				present := strings.Contains(prompt, "SyntaxError: binding already declared at native-check:14")
				if present != tc.want {
					t.Errorf("%s observation presence=%t want=%t", consumer, present, tc.want)
				}
				if tc.want && (!strings.Contains(prompt, "historical") || !strings.Contains(prompt, "superseded")) {
					t.Errorf("%s historical observation presented as current authority", consumer)
				}
			}
			if string(before) != string(b1122JSON(t, ctx.Mutable.WriteContextPack())) {
				t.Fatal("prompt mutated retained context")
			}
		})
	}
}

func TestB1702ProbeExecutionObservationRetainsExcerptEndsAndOutputReference(t *testing.T) {
	for _, historical := range []bool{false, true} {
		ctx := b1702ProbeObservationContext(t)
		observation := &ctx.Mutable.ChangeReport().VerificationDiagnostics[0].ProbeExecutionObservations[0]
		observation.OutputExcerpt = "HEAD-NATIVE " + strings.Repeat("ordinary execution context ", 100) + " TAIL-NATIVE"
		observation.OutputRef = "/outputs/" + strings.Repeat("ordinary-directory/", 12) + "native-check.txt"
		wantRef := observation.OutputRef
		// The normal durable pack round trip and crowded normalization must
		// preserve this atomic item, independently from its ordinary top-N view.
		pack := types.WriteContextPackFromChangeReport(ctx.Mutable.ChangeReport()).WithScope("batch", "slice")
		for i := 0; i < 110; i++ {
			pack.Items = append(pack.Items, types.WriteContextItem{ID: fmt.Sprint("constraint-", i), Kind: "constraint", Priority: types.WriteContextP0, Text: "preserve the interface"})
		}
		var restored types.WriteContextPack
		if err := json.Unmarshal(b1122JSON(t, pack), &restored); err != nil {
			t.Fatal(err)
		}
		ctx.Mutable.SetWriteContextPack(&restored)
		if historical {
			ctx.Mutable.ResetChangeReport()
		}
		before := b1122JSON(t, []any{ctx.Mutable.ChangeReport(), ctx.Mutable.WriteContextPack()})
		for consumer, prompt := range b1702ProbeObservationPrompts(ctx) {
			for _, want := range []string{"HEAD-NATIVE", "TAIL-NATIVE", wantRef} {
				if !strings.Contains(prompt, want) {
					t.Errorf("historical=%t %s lost %q", historical, consumer, want)
				}
			}
		}
		if string(before) != string(b1122JSON(t, []any{ctx.Mutable.ChangeReport(), ctx.Mutable.WriteContextPack()})) {
			t.Fatal("bounded rendering edited the original execution observation")
		}
	}
}
