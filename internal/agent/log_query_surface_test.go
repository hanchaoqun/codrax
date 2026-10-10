package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogQuerySurfaceUsesLiveSourceNotProseOrPreview(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "empty", Data: []byte{}}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	sk := &skill.Config{ToolSuggestions: []string{"log_query"}}
	for _, tt := range []struct {
		name string
		ctx  *types.AgentContext
		want bool
	}{
		{"no attachment", &types.AgentContext{Stage: types.StageExplore, Objective: "read app.log with log_query"}, false},
		{"preview only", &types.AgentContext{Stage: types.StageExplore, AttachedLog: "some original text"}, false},
		{"complete empty source", &types.AgentContext{Stage: types.StageExplore, AttachedLogCatalog: catalog}, true},
		{"triage coordinate isolation", &types.AgentContext{Stage: types.StageLogTriage, AttachedLogCatalog: catalog}, false},
		{"extractor does not explore", &types.AgentContext{Stage: types.StageExtract, AttachedLogCatalog: catalog}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasToolSchema(base.buildToolSchemas(sk, tt.ctx), "log_query"); got != tt.want {
				t.Fatalf("schema visibility %v want %v", got, tt.want)
			}
		})
	}
	live := &types.AgentContext{Stage: types.StageExplore, AttachedLogCatalog: catalog, ExploreToolSurface: types.ExploreToolSurfaceSourceInventoryLens}
	if !sourceInventoryLensToolSurface(live)["log_query"] || !sourceInventoryFollowupToolSurface(live)["log_query"] {
		t.Fatal("narrow probe stranded attached logs")
	}
	if rejected := validateExplorerSourceInventoryLensToolCall(live, nil, llm.ToolCall{Name: "log_query", Params: json.RawMessage(`{}`)}); rejected != nil {
		t.Fatalf("call surface disagrees: %+v", rejected)
	}
	encoded, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.AgentContext
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if logQueryToolVisible(&restored) {
		t.Fatal("persisted JSON restored live log access")
	}
}
