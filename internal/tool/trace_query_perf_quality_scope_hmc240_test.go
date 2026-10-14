package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfQualityPublicMemberAndPopulationHMC240(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.perftrace")
	body := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=174 event=cycles symbol=work dso=kernel.so source=raw_perfdata_fallback symbolization_status=unsymbolized callchain_status=ip_only clock=trace clock_confidence=assumed\n" +
		"worker-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=85 event=cycles symbol=work dso=app.so source=raw_perfdata_fallback symbolization_status=symbolized callchain_status=symbolized clock=trace clock_confidence=assumed\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	ctx.ToolResults = []types.ToolResult{r}
	result, err := (&EmitAnswerDocument{}).Execute(ctx, json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"窗口内有两类采样热点。"}]}`))
	if err != nil || !result.Success {
		t.Fatalf("emit: %v %+v", err, result)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	var items []types.AnswerBlockItem
	for _, block := range doc.Blocks {
		if block.ID == "runtime_trace_perf_quality" {
			items = block.Items
		}
	}
	memberCount, populationCount := 0, 0
	for _, item := range items {
		if strings.Contains(item.Label, "采样总体") {
			populationCount++
			for _, want := range []string{"cpu_known=2", "symbolized", "unsymbolized", "259", path, "1.000000000", "2.000000000"} {
				if !strings.Contains(item.Text, want) {
					t.Errorf("population missing %q: %+v", want, item)
				}
			}
			continue
		}
		if strings.Contains(item.Label, "热点字段") {
			memberCount++
			want := "symbolization_status=symbolized"
			if strings.Contains(item.Text, "kernel.so") {
				want = "symbolization_status=unsymbolized"
			}
			if !strings.Contains(item.Text, want) || strings.Contains(item.Text, "cpu_known=") || strings.Contains(item.Text, "callchain_status=") {
				t.Errorf("member inherited population: %+v", item)
			}
		}
	}
	if memberCount != 2 || populationCount != 1 {
		t.Fatalf("want one scoped population plus two true members; got %+v", items)
	}
	for _, o := range r.Observations {
		if o.Predicate != "perf_sample_top_symbol" || strings.Contains(o.ID, "#evidence_fact:") {
			continue
		}
		notes := strings.Join(o.RichNotes, "\n")
		if !strings.Contains(notes, "perf_quality_scope=") || !strings.Contains(notes, "perf_quality=scope=cohort,") {
			t.Errorf("model handoff lost quality scope: %s", notes)
		}
	}
}

func TestPerfQualityPublicIndependentScopesHMC240(t *testing.T) {
	rows := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=7 event=cycles symbol=work dso=one.so source=raw symbolization_status=unsymbolized clock=trace\n" +
		"worker-11 (10) [000] .... 1.3: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=3 event=cycles symbol=work dso=two.so source=raw symbolization_status=symbolized clock=trace\n" +
		"worker-11 (10) [000] .... 1.4: perf_sample: pid=10 tid=11 cpu_known=false sample_kind=unknown sample_weight=19 event=instructions symbol=work dso=two.so source=raw symbolization_status=unknown clock=trace\n"
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.perftrace"), filepath.Join(dir, "b.perftrace")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(rows), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	var records []types.ObservationRecord
	for _, query := range []struct {
		path string
		end  float64
	}{{paths[0], 2}, {paths[0], 1.2}, {paths[1], 2}} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": query.path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": query.end})
		if !r.Success {
			t.Fatal(r.Summary)
		}
		payload := hmc17NamedPayload(t, r)
		if payload.PerfStats == nil {
			t.Fatal("no native population")
		}
		records = append(records, r.Observations...)
	}
	before, _ := json.Marshal(records)
	items := RuntimeTracePerfQualityScopedItems(records)
	populations, members, unknown := 0, 0, 0
	for _, item := range items {
		if strings.Contains(item.Label, "采样总体") {
			populations++
			continue
		}
		members++
		if strings.Contains(item.Text, "symbolization_status=unknown") {
			unknown++
		}
		if strings.Contains(item.Text, "cpu_known=") || strings.Contains(item.Text, "callchain_status=") {
			t.Errorf("inherited aggregate quality: %+v", item)
		}
	}
	if populations != 5 || members != 7 || unknown != 2 {
		t.Fatalf("source/window/event scopes merged or missing-status invented: pop=%d members=%d unknown=%d %+v", populations, members, unknown, items)
	}
	// Duplicate publication and record order cannot change scope/quantity.
	duplicate := append(append([]types.ObservationRecord(nil), records...), records...)
	slices.Reverse(duplicate)
	if got := RuntimeTracePerfQualityScopedItems(duplicate); !reflect.DeepEqual(items, got) {
		t.Fatalf("duplicate/order changed scoped display: before=%+v after=%+v", items, got)
	}
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Fatal("display changed raw observations")
	}
	var original types.ObservationRecord
	for _, record := range records {
		if scope, _ := runtimeTracePerfQualityScope(record); scope != nil {
			original = record
			break
		}
	}
	if original.ID == "" {
		t.Fatal("no scoped publication")
	}
	for _, field := range []string{"query_source", "query_id", "payload"} {
		bad := original
		switch field {
		case "query_source":
			bad.SourceRef.Path += "-other"
		case "query_id":
			bad.SourceRef.QueryScopeID += "-other"
		case "payload":
			bad.SourceRef.PayloadRef += "-other"
		}
		if scope, _ := runtimeTracePerfQualityScope(bad); scope != nil {
			t.Fatalf("%s borrowed another query's quality", field)
		}
	}
	for _, field := range []string{"source", "generation", "window", "cohort"} {
		changed := original
		changed.RichNotes = append([]string(nil), original.RichNotes...)
		scope, _ := runtimeTracePerfQualityScope(original)
		switch field {
		case "source":
			scope.Scope.SourcePath += "-other"
		case "generation":
			scope.Scope.SourceGeneration += "-other"
		case "window":
			scope.Scope.Window.EndTs++
		case "cohort":
			scope.Scope.ID += "-other"
		}
		raw, _ := json.Marshal(scope)
		for i, note := range changed.RichNotes {
			if strings.HasPrefix(note, "perf_quality_scope=") {
				changed.RichNotes[i] = "perf_quality_scope=" + string(raw)
			}
		}
		got := RuntimeTracePerfQualityScopedItems([]types.ObservationRecord{original, changed})
		count := 0
		for _, item := range got {
			if strings.Contains(item.Label, "采样总体") {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("%s omitted from cohort identity", field)
		}
	}
}

func TestPerfQualityCompactDistributionOrderHMC240(t *testing.T) {
	for _, values := range [][]tracequery.PerfValueCount{
		{{Value: "symbolized", SampleCount: 2, Period: 5}},
		{{Value: "unknown", SampleCount: 2, Period: 5}},
		{{Value: "unsymbolized", SampleCount: 1, Period: 3}, {Value: "symbolized", SampleCount: 1, Period: 2}},
	} {
		q := &tracequery.PerfQualitySummary{SymbolizationStatuses: values}
		before := traceQueryPerfQualityCompact(q)
		slices.Reverse(values)
		if after := traceQueryPerfQualityCompact(q); after != before {
			t.Fatalf("first-item order selected a scalar: %s != %s", before, after)
		}
		if len(values) > 1 && !strings.Contains(before, "symbolization=mixed") {
			t.Fatalf("lost mixed distribution: %s", before)
		}
	}
	entry, ok := types.TraceNoteKeyLookup("perf_quality_scope")
	if !ok || entry.Carrier != types.TraceNoteCarrierDisplayOnly {
		t.Fatal("quality scope must carry no hard authority")
	}
}

func TestPerfQualityPublicSameHotspotMixedOrderHMC240(t *testing.T) {
	for _, order := range [][]string{{"symbolized", "unsymbolized"}, {"unsymbolized", "symbolized"}, {"unknown", "symbolized"}, {"symbolized", "unknown"}} {
		path := filepath.Join(t.TempDir(), "same-hotspot.perftrace")
		var body strings.Builder
		for i, status := range order {
			body.WriteString(fmt.Sprintf("worker-11 (10) [000] .... 1.%d: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=unknown sample_weight=10 event=cycles symbol=same dso=same.so source=raw symbolization_status=%s clock=trace\n", i+1, status))
		}
		if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, _, _ := hmc17NamedPathContext(t)
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
		payload := hmc17NamedPayload(t, r)
		hot := payload.PerfStats.Cohorts[0].TopSymbols
		if len(hot) != 1 || hot[0].SampleCount != 2 || hot[0].Period != 20 || hot[0].SymbolizationStatus != "mixed" {
			t.Fatalf("native member quality chose first row: %+v", hot)
		}
		items := RuntimeTracePerfQualityScopedItems(r.Observations)
		if len(items) != 2 || !strings.Contains(items[1].Text, "symbolization_status=mixed") {
			t.Fatalf("display reinterpreted mixed member: %+v", items)
		}
	}
}
