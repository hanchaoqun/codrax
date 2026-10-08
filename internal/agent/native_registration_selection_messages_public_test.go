package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use real native producer identities and capture the actual planner adapter
// request. The controller authorization is an explicit protocol fixture seam;
// this test neither registers a test nor claims a new verification execution.
func TestNativeRegistrationSelectionActualPlannerRequest(t *testing.T) {
	sourceBus, _ := nativeIdentityPublicRun(t, "test_other", false)
	report := sourceBus.Mutable.ChangeReport()
	root, err := filepath.EvalSymlinks(sourceBus.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	delivery := types.VerificationDeliverySnapshot{SourcePlanID: report.PlanID, AppliedCommitSHA: sha,
		PatchEffect: &types.PatchEffectRecord{PlanID: report.PlanID, RecordID: "dispatch-fixture", Source: "applied_commit", HeadRef: sha, DiffFingerprint: strings.Repeat("b", 64)}}
	for _, mode := range []string{"authorized", "ordinary", "revoked", "foreign_root", "restored_history"} {
		t.Run(mode, func(t *testing.T) {
			mu := currentBatchPromptFixture()
			if err := mu.AuthorizeNativeTestRegistration(root, delivery, []types.WriteBehaviorContract{{ID: "criterion-current"}}, []string{"packages/widget/widget.py"}); err != nil {
				t.Fatal(err)
			}
			mu.InstallNativeTestRegistrationIdentity(mu.NativeTestRegistrationAuthorization().ID, report)
			choices := mu.NativeTestRegistrationIdentityChoices(root)
			if len(choices) != 1 || choices[0].AssertionID != "python/unittest@packages/widget::test_increment" {
				t.Fatalf("real producer identity not offered: %+v", choices)
			}
			bus := &types.BusContext{Mutable: mu, RepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply}
			switch mode {
			case "ordinary":
				bus.Mutable = types.NewMutableState("ordinary source change")
			case "revoked":
				mu.RevokeNativeTestRegistrationAuthorization()
			case "foreign_root":
				bus.RepoRoot = t.TempDir()
			case "restored_history":
				body := b1122JSON(t, mu.WriteWorkflowRun())
				var restored types.WriteWorkflowRun
				if err := json.Unmarshal(body, &restored); err != nil {
					t.Fatal(err)
				}
				bus.Mutable = types.NewMutableState("restored history")
				bus.Mutable.SetWriteWorkflowRun(&restored)
				bus.Mutable.SetChangePlan(&types.ChangePlan{ID: report.PlanID})
				bus.Mutable.SetChangeReport(report)
			}
			before := b1122JSON(t, []any{report, bus.Mutable.ChangePlan(), bus.Mutable.WriteWorkflowRun()})
			registry := tool.NewRegistry()
			tool.RegisterDefaults(registry)
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured actual native registration choices")}
			planner := NewPlannerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := planner.Execute(ctxbuilder.BuildAgentContext(bus, types.AgentPlanner, types.StagePlan), traceTeachingSkill(t, "change-plan-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("missing actual planner request: calls=%d err=%v", capture.calls, err)
			}
			found := 0
			for _, offered := range capture.tools {
				if offered.Name != "emit_change_plan" && offered.Name != "emit_plan_skeleton" {
					continue
				}
				found++
				if mode == "authorized" {
					if !bytes.Contains(offered.Parameters, []byte(choices[0].Ref)) || !bytes.Contains(offered.Parameters, []byte(`"anyOf"`)) {
						t.Fatalf("actual %s schema omitted current exact selector: %s", offered.Name, offered.Parameters)
					}
				} else {
					original, _ := registry.Get(offered.Name)
					if !bytes.Equal(offered.Parameters, original.Parameters()) || bytes.Contains(offered.Parameters, []byte(`"assertion_ref"`)) {
						t.Fatalf("inactive request acquired selection schema: %s", offered.Name)
					}
				}
			}
			if found != 2 {
				t.Fatalf("actual planner did not expose both plan entry points: %d", found)
			}
			var message strings.Builder
			for _, row := range capture.messages {
				message.WriteString(row.Content)
			}
			if strings.Contains(message.String(), choices[0].Ref) != (mode == "authorized") {
				t.Fatal("visible identity list and actual schema authorization disagree")
			}
			if !bytes.Equal(before, b1122JSON(t, []any{report, bus.Mutable.ChangePlan(), bus.Mutable.WriteWorkflowRun()})) {
				t.Fatal("schema projection changed source report, plan, or workflow")
			}
		})
	}
}
