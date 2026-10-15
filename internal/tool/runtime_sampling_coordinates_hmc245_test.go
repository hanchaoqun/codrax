package tool

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Public query and current native receipt, rather than model prose, own both
// ratios and the recorded CPU census. CPU coordinates do not prove execution.
func TestHMC245SamplingRecordedCPUPublicSurfaces(t *testing.T) {
	body := strings.Join([]string{
		`worker-11 (10) [000] .... 1.100000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=0 cpu_known=true sample_weight=3 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`worker-11 (10) [001] .... 1.200000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=1 cpu_known=true sample_weight=9 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`worker-11 (10) [000] .... 1.300000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=0 cpu_known=false sample_weight=2 event=cycles symbol=other dso=/app/B.so source=raw sample_kind=unknown`,
		`worker-11 (10) [002] .... 1.400000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=2 cpu_known=true sample_weight=100 event=instructions symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`worker-11 (10) [003] .... 1.500000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=3 cpu_known=true sample_weight=7 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=off_cpu`,
		`other-12 (10) [008] .... 1.600000: perf_sample: pid=10 tid=12 thread_comm=other thread_identity_known=true cpu=8 cpu_known=true sample_weight=800 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`worker-11 (10) [009] .... 2.000000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=true cpu=9 cpu_known=true sample_weight=900 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "recorded-cpu.perftrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	result, payload := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "limit": 8})
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	var summary, distribution *types.RuntimeMeasurementTable
	for _, table := range contract.Choices() {
		if !strings.Contains(table.Label, "/app/A.so") {
			continue
		}
		copy := table
		if table.View == types.RuntimeMeasurementSummary {
			summary = &copy
		}
		if table.View == types.RuntimeMeasurementDistribution {
			distribution = &copy
		}
	}
	if summary == nil || distribution == nil {
		t.Fatalf("public query did not supply native sampling facts: payload=%+v observations=%+v", payload.PerfStats, result.Observations)
	}
	schema, _ := json.Marshal(BuildAnswerDocumentParametersFor(&types.AnswerSemanticView{RuntimeMeasurementContract: contract}))
	for _, teaching := range []string{"record-share and weight-share denominators", "recorded CPU is not execution state"} {
		if !strings.Contains(string(schema), teaching) {
			t.Errorf("current schema omitted %q", teaching)
		}
	}
	column := func(table *types.RuntimeMeasurementTable, name string) int {
		for i, c := range table.Columns {
			if c == name {
				return i
			}
		}
		t.Errorf("native table omitted column %q: %v", name, table.Columns)
		return -1
	}
	countShare := column(summary, "记录占比")
	var cyclesRow []string
	for _, row := range summary.Rows {
		if row[0] == "cycles" && row[2] == "未知" {
			cyclesRow = row
		}
	}
	if cyclesRow == nil || cyclesRow[4] != "2" || cyclesRow[5] != "12" || cyclesRow[6] != "3" || cyclesRow[7] != "14" || cyclesRow[8] != "12/14" {
		t.Fatalf("native independent counts/weights changed: %v", summary.Rows)
	}
	if countShare >= 0 && cyclesRow[countShare] != "2/3" {
		t.Errorf("count share borrowed weighted denominator: %v", cyclesRow)
	}
	seen := map[string]string{}
	for _, row := range distribution.Rows {
		if row[0] == "cycles" && row[10] == "未知" && row[3] == "记录CPU（不代表执行状态）" {
			seen[row[4]] = row[5] + "/" + row[6]
		}
	}
	for name, want := range map[string]string{"CPU 0": "1/3", "CPU 1": "1/9", "未知": "1/2"} {
		if seen[name] != want {
			t.Errorf("full query CPU census %s=%q want %q; got %v", name, seen[name], want, distribution.Rows)
		}
	}
	if t.Failed() {
		return
	}
	// The same private native receipt renders all three answer surfaces. No
	// model-supplied rows, edge-bearing payload or wall-time conversion exists.
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		for _, table := range []*types.RuntimeMeasurementTable{summary, distribution} {
			ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			if !types.BindRuntimeMeasurementReceipt(ref, contract) {
				t.Fatal("current native binding failed")
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: ref}}}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, want := range []string{"未知", "查询总体权重", "不等于耗时"} {
				if !strings.Contains(visible, want) {
					t.Errorf("%s lost %q", kind, want)
				}
			}
			if table.View == types.RuntimeMeasurementSummary && (!strings.Contains(visible, "2/3") || !strings.Contains(visible, "12/14")) {
				t.Errorf("%s lost separate ratios", kind)
			}
			if table.View == types.RuntimeMeasurementDistribution && (!strings.Contains(visible, "CPU 0") || !strings.Contains(visible, "CPU 1")) {
				t.Errorf("%s lost recorded CPU census", kind)
			}
			if strings.Contains(visible, "-->") || strings.Contains(visible, "CPU 9") || strings.Contains(visible, "CPU 8") {
				t.Errorf("%s invented relation or leaked outside population", kind)
			}
			wire, _ := json.Marshal(doc)
			if strings.Contains(string(wire), "CPU 1") {
				t.Fatal("native CPU facts became model JSON payload")
			}
		}
	}
}

func TestHMC245SamplingRecordedCPUPublicBoundedAndFiltered(t *testing.T) {
	for _, mode := range []string{"full", "filtered", "outside", "source_only", "cpu_zero"} {
		t.Run(mode, func(t *testing.T) {
			var body strings.Builder
			for cpu := 0; cpu < 20; cpu++ {
				known := "true"
				if mode == "source_only" {
					known = "false"
				}
				fmt.Fprintf(&body, "worker-11 (10) [%03d] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=%s cpu=%d cpu_known=true sample_weight=%d event=cycles symbol=cpu%d dso=/app/A.so source=raw sample_kind=unknown\n", cpu, 100+cpu, known, cpu, cpu+1, cpu)
				if mode == "cpu_zero" {
					break
				}
			}
			if mode != "cpu_zero" {
				body.WriteString("worker-11 (10) [000] .... 1.200000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=0 cpu_known=false sample_weight=1000 event=cycles symbol=unassigned dso=/app/A.so source=raw sample_kind=unknown\n")
			}
			path := filepath.Join(t.TempDir(), "many-cpu.perftrace")
			if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
				t.Fatal(err)
			}
			params := map[string]any{"pid": 11, "limit": 1}
			if mode == "filtered" {
				params["pattern"] = "symbol=cpu19"
			}
			if mode == "outside" {
				params["time_start"], params["time_end"] = 3, 4
			}
			r, p := samplingIdentityQueryHMC239(t, path, "perf_stats", params)
			if mode == "outside" || mode == "source_only" {
				for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}}).Choices() {
					if table.HasNativeSamplingFacts() {
						t.Fatal("unavailable population minted recorded CPU display")
					}
				}
				return
			}
			if p.PerfStats == nil || len(p.PerfStats.Cohorts) != 1 || p.PerfStats.Cohorts[0].Quality == nil {
				t.Fatalf("missing cohort: %+v", p.PerfStats)
			}
			c := p.PerfStats.Cohorts[0]
			q := c.Quality
			wantCount, wantWeight, wantOmitted, wantRows := 21, int64(1210), 4, 18
			if mode == "filtered" {
				wantCount, wantWeight, wantOmitted, wantRows = 1, 20, 0, 1
			}
			if mode == "cpu_zero" {
				wantCount, wantWeight, wantOmitted, wantRows = 1, 1, 0, 1
			}
			if q.RecordedCPUOmittedCount != wantOmitted || len(q.RecordedCPUs) != wantRows {
				t.Fatalf("wrong bounded census: %+v", q)
			}
			count, weight := 0, int64(0)
			for _, item := range q.RecordedCPUs {
				count += item.SampleCount
				weight += item.Period
			}
			if count != wantCount || weight != wantWeight || c.SampleCount != wantCount || c.TotalPeriod != wantWeight {
				t.Fatalf("Top-N/filter changed full census: count=%d weight=%d cohort=%+v", count, weight, c)
			}
			for _, hot := range c.TopSymbols {
				if len(hot.CPUs) != 0 {
					t.Fatal("unknown-kind coordinate became execution CPU")
				}
			}
			found := false
			for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}}).Choices() {
				if table.View != types.RuntimeMeasurementDistribution {
					continue
				}
				for _, row := range table.Rows {
					if row[3] != "记录CPU（不代表执行状态）" {
						continue
					}
					if mode == "full" && row[4] == "其他已知CPU（合并4个）" {
						found = row[5] == "4" && row[6] == "10"
					}
					if mode == "filtered" && row[4] == "CPU 19" {
						found = row[5] == "1" && row[6] == "20"
					}
					if mode == "cpu_zero" && row[4] == "CPU 0" {
						found = row[5] == "1" && row[6] == "1"
					}
					if row[7] != strconv.Itoa(wantCount) || row[8] != strconv.FormatInt(wantWeight, 10) {
						t.Fatal("display recalculated population denominator")
					}
				}
			}
			if !found {
				t.Fatal("public native display dropped bounded CPU facts")
			}
		})
	}
}
