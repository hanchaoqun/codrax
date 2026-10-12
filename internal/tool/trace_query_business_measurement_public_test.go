package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC230BusinessMeasurementPublicOwnWindow(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range [][2]float64{{.999, 1.051}, {1.01, 1.02}} {
		ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business states")}
		result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": window[0], "time_end": window[1]})
		found := 0
		for _, record := range result.Observations {
			if record.Predicate != types.TraceBusinessSpanPredicate {
				continue
			}
			p, ok := types.DecodeRuntimeMeasurementPublication(record)
			if !ok {
				t.Errorf("business fact has no native display: %s", record.Object)
				continue
			}
			found++
			if len(p.Tables) != 1 || p.Tables[0].View != types.RuntimeMeasurementSummary || p.Tables[0].MemberSet != nil || p.Tables[0].DisplayPopulation != nil || p.Source.QueryWindowStartTs != window[0] || p.Source.QueryWindowEndTs != window[1] {
				t.Fatalf("display changed scope or acquired population authority: %+v", p)
			}
			rows := map[string]string{}
			for _, row := range p.Tables[0].Rows {
				rows[row[0]] = row[1]
			}
			want := "5"
			if record.Object == "LoadDocumentIndex" {
				want = "8"
			}
			if window[0] == 1.01 {
				want = "0"
			}
			if rows["运行（ms）"] != want {
				t.Errorf("%s running=%q want %q", record.Object, rows["运行（ms）"], want)
			}
			if rows["业务区间开始（秒，含）"] != traceQueryDisplaySeconds(record.Span.StartTs) || rows["业务区间结束（秒，不含）"] != traceQueryDisplaySeconds(record.Span.EndTs) {
				t.Fatal("business display borrowed broad query endpoints")
			}
			if !strings.Contains(strings.Join(p.Tables[0].Notes, " "), "不能跨嵌套业务相加") {
				t.Fatal("nested accounts lost no-addition boundary")
			}
			for _, mutate := range []func(*types.RuntimeMeasurementTable){
				func(table *types.RuntimeMeasurementTable) { table.View = types.RuntimeMeasurementTimeline },
				func(table *types.RuntimeMeasurementTable) { table.MemberSet = &types.RuntimeMeasurementMemberSet{} },
				func(table *types.RuntimeMeasurementTable) {
					table.DisplayPopulation = &types.RuntimeMeasurementDisplayPopulation{}
				},
			} {
				copy := p
				copy.Tables = append([]types.RuntimeMeasurementTable(nil), p.Tables...)
				mutate(&copy.Tables[0])
				raw, _ := json.Marshal(copy)
				bad := record
				bad.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
				if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
					t.Fatal("business summary acquired another view or population authority")
				}
			}
		}
		if found != 2 {
			t.Errorf("native business account count=%d want2", found)
		}
	}
}

func TestHMC230BusinessMeasurementPublicUnavailableIsNotZero(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
task-700 (600) [003] .... 9.000000: tracing_mark_write: B|600|OtherWork
task-700 (600) [003] .... 9.010000: tracing_mark_write: E|600
`)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 9, "time_end": 9.01})
	for _, record := range result.Observations {
		if record.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(record)
		if !ok {
			t.Fatal("missing business display")
		}
		for _, row := range p.Tables[0].Rows {
			if row[0] == "运行（ms）" && row[1] != "未知" {
				t.Fatal("missing scheduler coverage became zero")
			}
		}
		return
	}
	t.Fatal("missing business observation")
}
