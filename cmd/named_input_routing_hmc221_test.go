package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/repl"
)

func TestHMC221NamedNativeContentReachesActualCLIClassifier(t *testing.T) {
	oldApp, oldRepo := app, flagRepo
	t.Cleanup(func() { app, flagRepo = oldApp, oldRepo })
	fixture, err := filepath.Abs("../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &summarizerStubAdapter{resp: llm.Response{ToolCalls: []llm.ToolCall{{Name: "emit_turn_policy", Params: []byte(`{"route":"data","needs_data_access":true,"operation":"transform","data_task_kind":"data_aggregation","source":"current_message","confidence":0.93,"reason":"explicit data transformation","requires_diagram":false}`)}}}}
	app.chitchatClassifier = repl.NewChitchatClassifier(adapter)
	app.dataTaskPlanner = repl.NewDataTaskPlanner(adapter)
	app.userMode = repl.UserModeAuto
	flagRepo = filepath.Dir(fixture)
	request := "Convert selected readings from " + fixture + " into another dataset"
	policy, ok := classifySingleShotRoutePolicy(request)
	if !ok || policy.Route != repl.RouteData {
		t.Fatalf("content capability must not force override coherent typed data route: %+v %t", policy, ok)
	}
	var content string
	for _, m := range adapter.lastMessages {
		if m.Role == "user" {
			content = m.Content
		}
	}
	for _, required := range []string{"## current_named_inputs", "trace_query", "measurements", "## current: " + request} {
		if !strings.Contains(content, required) {
			t.Fatalf("missing %q in actual CLI classifier: %s", required, content)
		}
	}
	for _, mode := range []repl.UserMode{repl.UserModeCode, repl.UserModeWrite, repl.UserModeData} {
		app.userMode = mode
		adapter.lastMessages = nil
		if _, ok := classifySingleShotRoutePolicy(request); ok || len(adapter.lastMessages) != 0 {
			t.Fatalf("explicit mode %s must bypass auto classification", mode)
		}
	}
}
