package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfQualityScopedSupplementUsesNativeProjectionHMC240(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.perftrace")
	body := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=7 event=cycles symbol=work dso=one.so source=raw symbolization_status=unsymbolized clock=trace\n" +
		"worker-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=3 event=cycles symbol=work dso=two.so source=raw symbolization_status=symbolized clock=trace\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{WorkDir: dir, RepoRoot: dir, Mutable: types.NewMutableState("inspect trace"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
	r, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	ctx := &types.AgentContext{Mutable: bus.Mutable}
	items := tool.RuntimeTracePerfQualityScopedItems(r.Observations)
	rows := runtimeTracePerfQualityMetricRows(ctx, nil)
	if len(items) != 3 || len(rows) != 3 {
		t.Fatalf("scoped summary lost members or count: items=%+v rows=%+v", items, rows)
	}
	for i, item := range items {
		if rows[i].Value != item.Label+": "+item.Text {
			t.Fatal("finalizer supplement copied a second quality formatter")
		}
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "quality", Kind: types.BlockBulletList, Items: items}}}
	if got := runtimeAggregateMetricCompactRows(ctx, doc); len(got) != 0 {
		t.Fatalf("already visible native quality duplicated: %+v", got)
	}
	if strings.Contains(rows[2].Value, "cpu_known=") || !strings.Contains(rows[2].Value, "symbolization_status=symbolized") {
		t.Fatalf("member inherited cohort: %+v", rows[2])
	}
}
