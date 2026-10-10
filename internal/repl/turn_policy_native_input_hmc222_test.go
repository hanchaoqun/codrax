package repl

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC222NativeReaderBoundaryInActualClassifierContract(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, lane := range []string{"repl", "single_shot"} {
		t.Run(lane, func(t *testing.T) {
			a := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"optional","operation":"investigate","write_intent":"analysis_only","source":"artifact","confidence":0.95,"reason":"read native observations","requires_diagram":false,"required_outcomes":["measurement"]}`)}}
			classifier := NewChitchatClassifier(a)
			request := "列出 " + path + " 的原始记录和区间"
			ctx := WithNamedInputRoutingContext(context.Background(), request, "", t.TempDir())
			var err error
			if lane == "repl" {
				_, err = classifier.(TurnPolicyClassifier).ClassifyPolicy(ctx, request, "", false)
			} else {
				_, err = classifier.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(ctx, request, "", false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(a.calls) != 1 {
				t.Fatalf("unexpected repair: %d", len(a.calls))
			}
			var system, user string
			for _, m := range a.calls[0].messages {
				if m.Role == "system" {
					system = m.Content
				}
				if m.Role == "user" {
					user = m.Content
				}
			}
			const boundary = "Native runtime records, values, intervals, counts, and statistics are external observations"
			if !strings.Contains(system, boundary) {
				t.Fatal("system lacks non-causal native reader boundary")
			}
			var schema struct {
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(a.calls[0].tools[0].Parameters, &schema); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(schema.Properties["route"].Description, boundary) {
				t.Fatal("route schema lacks the shared native reader boundary")
			}
			if !strings.Contains(schema.Properties["operation"].Description, "diagnosis and current-source access are not prerequisites") ||
				!strings.Contains(system, "diagnosis and current-source access are not prerequisites") {
				t.Fatal("investigate still excludes native non-diagnostic readings")
			}
			if !strings.Contains(schema.Properties["source"].Description, "external observation (including native runtime records and measurements)") ||
				!strings.Contains(system, "derives from an input observation artifact") {
				t.Fatal("source teaching lost input observation artifacts")
			}
			if strings.Count(system, boundary) != 1 || strings.Count(schema.Properties["route"].Description, boundary) != 1 {
				t.Fatal("routing rule repeated within one teaching surface")
			}
			if strings.Contains(user, "Prefer it when") || strings.Contains(user, "trace_query is available") || strings.Contains(user, boundary) {
				t.Fatal("current input data repeats routing instructions")
			}
			if !strings.Contains(user, "candidate_views") || !strings.Contains(user, "## current: "+request) {
				t.Fatal("actual content or original request lost")
			}
		})
	}
}

// Canned decisions exercise preservation, not natural-language routing accuracy:
// content candidates are soft navigation and cannot overwrite an independently
// typed ordinary-data, explicit-operation, or authorized-write decision.
func TestHMC222NativeCandidateKeepsIndependentTypedRoutes(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		policy        TurnPolicy
		hasLastAnswer bool
	}{
		{"ordinary_data", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Operation: "structured_file_transform", DataTaskKind: "structured_file_transform", Source: "data", Confidence: 0.95, Reason: "convert the supported dataset", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}, false},
		{"file_operation", TurnPolicy{Route: RouteOperation, NeedsOperationAccess: true, Operation: "computer_operation", OperationKind: "computer_operation", Source: "current_message", TargetSurface: "file_artifact", SideEffects: []string{"local_file_write"}, RiskLevel: "low", Confidence: 0.95, Reason: "explicit file operation", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}, false},
		{"authorized_write", TurnPolicy{Route: RouteWrite, NeedsRepoAccess: true, Operation: "code_change", WriteIntent: WriteIntentExplicitChange, Source: "repo", Confidence: 0.95, Reason: "authorized source edit", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}, false},
		{"existing_answer", TurnPolicy{Route: RouteLocal, Operation: "transform", Source: "last_answer", Confidence: 0.95, Reason: "reformat existing facts", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			a := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(string(body))}}
			classifier := NewChitchatClassifier(a).(TurnPolicyClassifier)
			ctx := WithNamedInputRoutingContext(context.Background(), path, "", t.TempDir())
			got, err := classifier.ClassifyPolicy(ctx, path, "", tc.hasLastAnswer)
			if err != nil {
				t.Fatal(err)
			}
			got = ApplyTurnPolicyGuards(got, tc.hasLastAnswer, false)
			if got.Route != tc.policy.Route || got.Operation != tc.policy.Operation {
				t.Fatalf("candidate changed independently typed route: got %+v want %+v", got, tc.policy)
			}
		})
	}
}
