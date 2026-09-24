package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The real emitter accepts model extraction for audit/navigation. Its numeric
// fields are not a validated query window, even when they look plausible.
func TestPerfObservationPublicNavigationPreservesRowsWithoutUnverifiedTimes(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedHitrace: string(raw), Mutable: types.NewMutableState("inspect the attached business work")}
	params := json.RawMessage(`{"meta":{"source":"systrace"},"observations":[{"kind":"duration","subject":"wrongly paired business","summary":"guessed self cost","evidence":"unvalidated arithmetic","line_start":3,"line_end":6,"start_ts_ms":5.001,"end_ts_ms":5.005,"duration_ms":987.654,"confidence":0.9}]}`)
	r, err := (&tool.EmitPerfTrace{}).Execute(bus, params)
	if err != nil || !r.Success {
		t.Fatalf("emit: %v %+v", err, r)
	}
	before, _ := json.Marshal(bus.Mutable.PerfTrace())
	if !strings.Contains(string(before), `"duration_ms":987.654`) || !strings.Contains(string(before), `"start_ts_ms":5.001`) {
		t.Fatal("emitter did not preserve the original audit record")
	}
	for _, stage := range []struct {
		name  string
		agent types.AgentName
		stage types.PipelineStage
		skill string
	}{
		{"analyze", types.AgentAnalyzer, types.StageAnalyze, "analysis-skill"},
		{"explore", types.AgentExplorer, types.StageExplore, "explore-skill"},
	} {
		t.Run(stage.name, func(t *testing.T) {
			ac := ctxbuilder.BuildAgentContext(bus, stage.agent, stage.stage)
			pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: stage.skill, ToolSuggestions: []string{"trace_query"}})
			var section string
			for _, s := range pc.UserSections {
				if s.Title == ctxbuilder.SectionPerfTriageExtraction {
					section = s.Content
				}
			}
			if !strings.Contains(section, "candidate_trace_lines=3-6") {
				t.Fatalf("lost source-local navigation: %s", section)
			}
			for _, forbidden := range []string{"candidate_start_ts_ms=", "candidate_end_ts_ms=", "candidate_duration_ms=", "987.654", "wrongly paired business", "guessed self cost", "unvalidated arithmetic"} {
				if strings.Contains(section, forbidden) {
					t.Errorf("unvalidated projection leaked %q", forbidden)
				}
			}
		})
	}
	after, _ := json.Marshal(bus.Mutable.PerfTrace())
	if string(before) != string(after) {
		t.Fatal("prompt rendering mutated the audit bundle")
	}
}

func TestPerfObservationPublicNavigationKeepsExcerptCoordinates(t *testing.T) {
	raw, err := os.ReadFile("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	parent := string(raw)
	pos := strings.Index(parent, "5.001000: tracing_mark_write")
	if pos < 0 {
		t.Fatal("fixture marker missing")
	}
	start := strings.LastIndex(parent[:pos], "\n") + 1
	excerpt, err := attachment.NewTraceExcerpt(context.Background(), parent, nil, start, len(parent))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := excerpt.Resolve(context.Background(), parent, nil)
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedHitrace: parent, AttachedTraceExcerpt: excerpt, Mutable: types.NewMutableState("inspect this trace excerpt")}
	r, err := (&tool.EmitPerfTrace{}).Execute(bus, json.RawMessage(`{"meta":{"source":"systrace"},"observations":[{"subject":"candidate","summary":"unverified","line_start":1,"line_end":2,"start_ts_ms":5.001,"duration_ms":987.654}]}`))
	if err != nil || !r.Success {
		t.Fatalf("scoped emit: %v %+v", err, r)
	}
	before, _ := json.Marshal(bus.Mutable.PerfTrace())
	for _, a := range []struct {
		agent types.AgentName
		stage types.PipelineStage
		skill string
	}{
		{types.AgentAnalyzer, types.StageAnalyze, "analysis-skill"},
		{types.AgentExplorer, types.StageExplore, "explore-skill"},
	} {
		ac := ctxbuilder.BuildAgentContext(bus, a.agent, a.stage)
		pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: a.skill, ToolSuggestions: []string{"trace_query"}})
		var section string
		for _, s := range pc.UserSections {
			if s.Title == ctxbuilder.SectionPerfTriageExtraction {
				section = s.Content
			}
		}
		if !strings.Contains(section, scope.Description()) || !strings.Contains(section, "candidate_trace_lines=3-4") {
			t.Fatalf("%s lost fragment coordinates: %s", a.agent, section)
		}
		if strings.Contains(section, "candidate_start_ts_ms=") || strings.Contains(section, "987.654") {
			t.Fatal("scoped guessed timings leaked")
		}
	}
	after, _ := json.Marshal(bus.Mutable.PerfTrace())
	if string(before) != string(after) {
		t.Fatal("projection mutated scoped audit records")
	}
}
