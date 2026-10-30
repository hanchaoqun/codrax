package tool

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC319BusinessAccountTeachingPublicNativePublication(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		start, end float64
	}{
		{"complete", 5, 5.012},
		{"clipped", 5.002, 5.007},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business account teaching")}
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": tc.start, "time_end": tc.end})
			payload := businessSpanSchedulerPublicPayload(t, result)
			before, _ := json.Marshal(result.Observations)
			found, async, unknown := 0, 0, false
			for _, record := range result.Observations {
				if record.Predicate != types.TraceBusinessSpanPredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(record)
				isAsync := false
				for _, span := range payload.WindowStats.TraceSpans {
					if span.Name == record.Object && traceThreadLabel(span.Thread) == record.Subject && span.StartTs == record.Span.StartTs && span.EndTs == record.Span.EndTs && span.StartLine == record.Span.LineStart && span.Kind == "async" {
						isAsync = true
					}
				}
				if isAsync {
					async++
					if ok {
						t.Fatal("async marker acquired synchronous self/state accounting")
					}
					continue
				}
				if !ok || len(p.Tables) != 1 {
					t.Fatalf("missing complete native business table: %s", record.Object)
				}
				found++
				table := p.Tables[0]
				if p.Source.Path != path || p.Source.QueryWindowStartTs != tc.start || p.Source.QueryWindowEndTs != tc.end || table.MemberSet != nil || table.DisplayPopulation != nil {
					t.Fatal("teaching changed query provenance or population authority")
				}
				notes := strings.Join(table.Notes, " ")
				for _, want := range []string{"本表为含子层的总区间量测", "不能跨嵌套业务相加", "不适用于已扣除子区间的自身分段", "调度状态不证明等待机制、唤醒依赖或响应根因"} {
					if !strings.Contains(notes, want) {
						t.Errorf("native inclusive table lost scoped teaching %q: %s", want, notes)
					}
				}
				rows := map[string]string{}
				for _, row := range table.Rows {
					rows[row[0]] = row[1]
				}
				displayMS, displayErr := strconv.ParseFloat(rows["窗口内业务耗时（ms）"], 64)
				observationMS, observationErr := strconv.ParseFloat(record.Value, 64)
				if rows["业务区间开始（秒，含）"] != traceQueryDisplaySeconds(record.Span.StartTs) || rows["业务区间结束（秒，不含）"] != traceQueryDisplaySeconds(record.Span.EndTs) || displayErr != nil || observationErr != nil || math.Abs(displayMS-observationMS) > 1e-6 {
					t.Fatal("inclusive display no longer preserves native ruler")
				}
				if record.Subject == "background-900" {
					unknown = true
					if rows["运行（ms）"] != "未知" || rows["已分类状态合计（ms）"] != "未知" {
						t.Fatal("unavailable state became zero through arithmetic guidance")
					}
				}
			}
			if found != 4 || async != 1 || !unknown {
				t.Fatalf("missing native business inventory: count=%d async=%d unknown=%v", found, async, unknown)
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("reading teaching mutated native observations")
			}
		})
	}
}

func TestHMC319BusinessAccountTeachingPublicRelationRecipes(t *testing.T) {
	ctx, _, original := runtimeNestingFixture(t)
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	before, _ := json.Marshal(ledger)
	text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	for _, want := range []string{"overlapping inclusive (child-including) durations", "mutually disjoint self segments", "same source, thread, query range and unit", "not sample weights or a new causal permission"} {
		if !strings.Contains(text, want) {
			t.Errorf("runtime authoring handoff lost caliber boundary %q", want)
		}
	}
	if strings.Contains(text, "Do not sum parent and child durations.") {
		t.Error("unqualified duration ban still forbids valid self partitions")
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("relation teaching changed ledger")
	}
	current := RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel)
	if len(current) != len(original) {
		t.Fatal("teaching changed native relation count")
	}
	for i := range original {
		if current[i].Kind != types.DiagramRelContain || current[i].FromIdentity != original[i].FromIdentity || current[i].ToIdentity != original[i].ToIdentity {
			t.Fatal("teaching changed exact containment authority")
		}
	}
	if text := RenderRuntimeDiagramRelationRecipes(types.ObservationLedger{}, nil); text != "" {
		t.Fatal("no native relation acquired business arithmetic teaching")
	}
}
