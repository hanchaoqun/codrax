package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeResourceStackFinalizerFixture(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	start, end := 10.0, 10.05
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("展开资源事件调用栈"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000到10.050秒"}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101, "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("public native query: %v %+v", err, r)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("default native preparation modified the source capture")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	return ctx, r
}

func TestResourceStackFinalizerAggregateBudget(t *testing.T) {
	ctx, result := nativeResourceStackFinalizerFixture(t)
	var record types.ObservationRecord
	for _, candidate := range result.Observations {
		if _, ok := tool.DecodeTraceResourceStack(candidate); ok {
			record = candidate
			break
		}
	}
	p, ok := tool.DecodeTraceResourceStack(record)
	if !ok {
		t.Fatal("missing resource stack")
	}
	for i := range p.Events {
		for j := range p.Events[i].Frames {
			f := &p.Events[i].Frames[j]
			if f.Symbol.Status == "known" {
				f.Symbol.Value = strings.Repeat("s", 2048) + "::whole_symbol_end"
			}
			if f.Library.Status == "known" {
				f.Library.Value = strings.Repeat("l", 2048) + "/whole_library_end"
			}
		}
	}
	var ledger types.ObservationLedger
	for i := 0; i < 4; i++ {
		copy := record
		p.SourcePath = fmt.Sprintf("/capture-%d.systrace", i)
		copy.SourceRef.Path = p.SourcePath
		copy.ID = fmt.Sprintf("resource-query-%d", i)
		body, _ := json.Marshal(p)
		copy.RichNotes = []string{types.TraceNoteKeyResourceStack + "=" + string(body)}
		if _, ok := tool.DecodeTraceResourceStack(copy); !ok {
			t.Fatal("large receipt must remain structurally valid")
		}
		ledger.Records = append(ledger.Records, copy)
	}
	got := renderAnswerDocResourceStacks(ctx, ledger)
	shown := strings.Count(got, "observation_id=")
	if len(got) > 64<<10 || shown == 0 || shown >= 4 || !strings.Contains(got, fmt.Sprintf("另省略查询=%d", 4-shown)) {
		t.Fatalf("aggregate budget not enforced honestly: bytes=%d shown=%d", len(got), shown)
	}
	if !strings.Contains(got, strings.Repeat("s", 2048)+"::whole_symbol_end") || !strings.Contains(got, strings.Repeat("l", 2048)+"/whole_library_end") {
		t.Fatal("display truncated a source name")
	}
}

func TestResourceStackFinalizerInclusiveEnvelope(t *testing.T) {
	start, end := 10.0, 10.05
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10..10.05"}
	for _, tc := range []struct {
		name   string
		window tracequery.ResourceStackWindow
		want   bool
	}{
		{"explicit right-open", tracequery.ResourceStackWindow{StartTs: 10, EndTs: 10.05}, true},
		{"inclusive right edge", tracequery.ResourceStackWindow{StartTs: 10, EndTs: 10.05, EndInclusive: true}, false},
		{"interior inclusive envelope", tracequery.ResourceStackWindow{StartTs: 10, EndTs: 10.04, EndInclusive: true}, true},
		{"interior single point", tracequery.ResourceStackWindow{StartTs: 10.02, EndTs: 10.02, EndInclusive: true}, true},
		{"right-edge single point", tracequery.ResourceStackWindow{StartTs: 10.05, EndTs: 10.05, EndInclusive: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceStackInsideRequestedWindow(profile, tc.window); got != tc.want {
				t.Fatalf("eligibility=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestNativeResourceStackReachesActualFinalizer(t *testing.T) {
	ctx, _ := nativeResourceStackFinalizerFixture(t)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 已观测资源事件与调用栈", "ImageCache::reserve", "UIFrame::render", "free", "不固定某个深度为业务叶子", "0xffffffffffffffff"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	if strings.Contains(prompt, "ambiguous_A") || strings.Contains(prompt, "ambiguous_B") {
		t.Fatal("ambiguous dictionary selected a symbol")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) {
		t.Fatal("factual display mutated source observations")
	}
}

func TestNativeResourceStackHandoffIdentityAndScope(t *testing.T) {
	ctx, r := nativeResourceStackFinalizerFixture(t)
	var record types.ObservationRecord
	for _, candidate := range r.Observations {
		if _, ok := tool.DecodeTraceResourceStack(candidate); ok {
			record = candidate
			break
		}
	}
	if record.ID == "" {
		t.Fatal("missing typed resource stack receipt")
	}
	p, _ := tool.DecodeTraceResourceStack(record)
	if p.MatchedEvents != 3 || len(p.Events) != 3 || p.OmittedEvents != 0 {
		t.Fatalf("owner or right boundary changed: %+v", p)
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{record, record}}
	if got := renderAnswerDocResourceStacks(ctx, ledger); strings.Count(got, "### 已观测资源事件与调用栈") != 1 {
		t.Fatal("identical observations duplicated or disappeared")
	}
	negative := record
	negative.Negative = true
	if got := renderAnswerDocResourceStacks(ctx, types.ObservationLedger{Records: []types.ObservationRecord{negative}}); got != "" {
		t.Fatal("negative receipt gained factual display")
	}
	malformed := record
	malformed.RichNotes = append(append([]string(nil), record.RichNotes...), types.TraceNoteKeyResourceStack+"={}")
	if got := renderAnswerDocResourceStacks(ctx, types.ObservationLedger{Records: []types.ObservationRecord{malformed}}); got != "" {
		t.Fatal("duplicate carrier accepted")
	}
	// Individually valid but conflicting publications for the same receipt
	// cannot be resolved by record order or by choosing the first one.
	p.Events[0].Source.Size.Value = "1024"
	body, _ := json.Marshal(p)
	conflict := record
	conflict.RichNotes = []string{types.TraceNoteKeyResourceStack + "=" + string(body)}
	if _, ok := tool.DecodeTraceResourceStack(conflict); !ok {
		t.Fatal("conflict fixture must be independently structurally valid")
	}
	if got := renderAnswerDocResourceStacks(ctx, types.ObservationLedger{Records: []types.ObservationRecord{record, conflict}}); got != "" {
		t.Fatal("same-credential conflict selected one publication")
	}
	start, end := 10.01, 10.04
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.01..10.04"}
	if got := renderAnswerDocResourceStacks(ctx, ledger); got != "" {
		t.Fatal("wider resource query borrowed for a smaller explicit request")
	}
}
