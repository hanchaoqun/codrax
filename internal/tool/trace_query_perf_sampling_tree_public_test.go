package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerfSamplingTreePublicSQLiteDefaultPreparation(t *testing.T) {
	path := transactionSQLitePublicFixture(t, `
INSERT INTO sched_slice VALUES (1000000000,100000000,0,1,'S',120);
INSERT INTO thread_state VALUES (1,1000000000,100000000,0,'Running');
CREATE TABLE perf_thread (thread_id,process_id,thread_name);
INSERT INTO perf_thread VALUES (101,100,'app-main');
CREATE TABLE perf_files (file_id,serial_id,symbol,path);
INSERT INTO perf_files VALUES (7,-1,NULL,'/app/A.so');
CREATE TABLE perf_callchain (id,callchain_id,depth,name,ip,file_id,symbol_id);
INSERT INTO perf_callchain VALUES (2,10,0,'sys;root',16,-1,-1),(3,10,1,'work',32,7,-1);
CREATE TABLE perf_sample (id,callchain_id,timestamp_trace,thread_id,event_count,cpu_id,thread_state);
INSERT INTO perf_sample VALUES (1,10,1050000000,101,9,0,'Running'),(2,-1,1060000000,101,3,0,'Running');
`)
	before, _ := os.ReadFile(path)
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 1.1})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	found := false
	for _, o := range r.Observations {
		if p, ok := DecodeTracePerfSamplingTree(o); ok {
			found = true
			if p.Status != "available" || p.SampleCount != 2 || len(p.Cohorts) != 1 || p.Cohorts[0].TotalWeight != 12 || p.Cohorts[0].UnavailableWeight != 3 || len(p.Cohorts[0].Nodes) != 2 || p.Cohorts[0].Nodes[0].Symbol != "sys;root" || p.Cohorts[0].Nodes[0].References[0].SourceRef != "perf_callchain:2/callchain:10" {
				t.Fatalf("SQLite lost exact frames/denominator: %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("no native SQLite tree: %s", r.Summary)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only tree preparation mutated source")
	}
}

func TestPerfSamplingTreePublicSQLiteUnknownFrames(t *testing.T) {
	path := transactionSQLitePublicFixture(t, `CREATE TABLE perf_thread(thread_id,process_id,thread_name);INSERT INTO perf_thread VALUES(101,100,'app-main');CREATE TABLE perf_callchain(id,callchain_id,depth);INSERT INTO perf_callchain VALUES(1,10,0),(2,20,0);CREATE TABLE perf_sample(id,callchain_id,timestamp_trace,thread_id,event_count,cpu_id,thread_state);INSERT INTO perf_sample VALUES(1,10,1050000000,101,4,0,'Running'),(2,20,1060000000,101,5,0,'Running');`)
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 1.1})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	found := false
	for _, o := range r.Observations {
		if p, ok := DecodeTracePerfSamplingTree(o); ok {
			found = true
			if p.SampleCount != 2 || len(p.Cohorts) != 1 || p.Cohorts[0].TotalWeight != 9 || p.Cohorts[0].UnavailableWeight != 9 || len(p.Cohorts[0].Nodes) != 0 || p.Cohorts[0].UnavailableReasons["frame_budget_or_metadata_unavailable"] != 2 {
				t.Fatalf("unknown native frames merged: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("missing typed census")
	}
}

func TestPerfSamplingTreePublicLegacyBoundary(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "sampling.perftrace")
	if err := os.WriteFile(path, []byte("worker-11 (10) [000] .... 1.000000: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_weight=7 event=cycles callchain=\"root;literal;symbol\" source=raw_perfdata_fallback\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 0.5, "time_end": 1.5})
	r, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !r.Success {
		t.Fatalf("public sampling tree failed: %v %+v", err, r)
	}
	if !strings.Contains(r.Summary, "legacy_frame_boundaries_unavailable") || !strings.Contains(r.Summary, "total_weight=7") {
		t.Fatalf("lost denominator or guessed legacy boundaries: %s", r.Summary)
	}
	if len(r.Observations) == 0 {
		t.Fatal("no native typed observation")
	}
	found := false
	for _, o := range r.Observations {
		if p, ok := DecodeTracePerfSamplingTree(o); ok {
			found = true
			if p.CohortCount != 1 || p.Cohorts[0].TotalWeight != 7 || p.Cohorts[0].NodeCount != 0 {
				t.Fatalf("legacy tree invented: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("typed tree cannot round trip")
	}
}

func TestPerfSamplingTreePublicWithheldAndOverflowText(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "sampling.perftrace")
	body := "worker-7 (70) [000] .... 1.1: perf_sample: pid=70 tid=7 thread_comm=worker thread_identity_known=true sample_weight=9223372036854775807 event=cycles\nworker-7 (71) [000] .... 1.2: perf_sample: pid=71 tid=7 thread_comm=worker thread_identity_known=true sample_weight=1 event=cycles\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	for _, scoped := range []bool{false, true} {
		params := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 2}
		if scoped {
			params["pid"] = 7
		}
		r := hmc17NamedQuery(t, ctx, params)
		if !r.Success {
			t.Fatal(r.Summary)
		}
		if scoped {
			if !strings.Contains(r.Summary, "status=unavailable") || !strings.Contains(r.Summary, "perf_thread_selector_withheld=true") {
				t.Fatalf("public unknown hidden: %s", r.Summary)
			}
		} else if !strings.Contains(r.Summary, "weight_status=aggregate_overflow") || !strings.Contains(r.Summary, "total_weight=unknown") || strings.Contains(r.Summary, "total_weight=0") {
			t.Fatalf("overflow rendered as zero: %s", r.Summary)
		}
	}
}

func TestPerfSamplingTreePublicCohortsAndExpansion(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "sampling.perftrace")
	frames := func(dso, ip string) string {
		return tracewire.EncodePerfFrames([]tracewire.PerfFrame{{Symbol: "sys;root", DSO: "/system/lib.so", IP: "0x1", SourceRef: "frame:0", SourceDepth: 0}, {Symbol: "work", DSO: dso, IP: ip, SourceRef: "frame:1", SourceDepth: 1}}, "available")
	}
	line := func(ts float64, weight int, event, source, frames string) string {
		wire := ""
		if frames != "" {
			wire = " frames_v1=" + tracewire.QuotePerfKVValue(frames)
		}
		return fmt.Sprintf("worker-11 (10) [000] .... %.6f: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_weight=%d event=%s callchain=\"sys;root;work\" source=%s%s\n", ts, weight, event, source, wire)
	}
	text := line(1.1, 10, "cycles", "raw_perfdata_fallback", frames("/app/A.so", "0x2")) + line(1.2, 20, "cycles", "raw_perfdata_fallback", frames("/app/B.so", "0x3")) + line(1.3, 5, "cycles", "raw_perfdata_fallback", "") + line(1.4, 100, "instructions", "raw_perfdata_fallback", frames("/app/A.so", "0x2")) + line(1.5, 7, "cycles", "simpleperf_report_sample", frames("/app/A.so", "0x2")) + line(3, 900, "cycles", "raw_perfdata_fallback", frames("/app/A.so", "0x2"))
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	query := func(start, end float64, extra map[string]any) tracequery.PerfSamplingTreeResult {
		t.Helper()
		args := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": start, "time_end": end}
		for k, v := range extra {
			args[k] = v
		}
		raw, _ := json.Marshal(args)
		r, err := (&TraceQuery{}).Execute(ctx, raw)
		if err != nil || !r.Success {
			t.Fatalf("query: %v %+v", err, r)
		}
		for _, o := range r.Observations {
			if p, ok := DecodeTracePerfSamplingTree(o); ok {
				return p
			}
		}
		t.Fatalf("missing typed tree: %s %+v", r.Summary, r.Observations)
		return tracequery.PerfSamplingTreeResult{}
	}
	p := query(1, 2, nil)
	if p.SampleCount != 5 || p.CohortCount != 3 {
		t.Fatalf("cohorts merged: %+v", p)
	}
	var c tracequery.PerfSamplingCohort
	for _, item := range p.Cohorts {
		if item.Event == "cycles" && item.Producer == "raw_perfdata_fallback" {
			c = item
		}
	}
	if c.TotalWeight != 35 || c.StackWeight != 30 || c.UnavailableWeight != 5 || len(c.Nodes) != 3 {
		t.Fatalf("denominator/names lost: %+v", c)
	}
	var root tracequery.PerfSamplingNode
	for _, n := range c.Nodes {
		if n.Depth == 0 {
			root = n
		}
	}
	if root.Symbol != "sys;root" || root.InclusiveWeight != 30 || root.SelfWeight != 0 || math.Abs(root.Percent-100.0*30/35) > 1e-9 {
		t.Fatalf("semicolon root or share wrong: %+v", root)
	}
	collapsed := query(1, 2, map[string]any{"sampling_depth": 1})
	for _, v := range collapsed.Cohorts {
		if v.ID == c.ID && (v.TotalWeight != 35 || len(v.Nodes) != 1 || v.OmittedNodes != 2 || v.Nodes[0].OmittedChildren != 2) {
			t.Fatalf("collapse changed population: %+v", v)
		}
	}
	expanded := query(1, 2, map[string]any{"sampling_root": root.ID, "sampling_depth": 2})
	for _, v := range expanded.Cohorts {
		if v.ID == c.ID && (v.TotalWeight != 35 || len(v.Nodes) != 3 || v.Nodes[0].ID != root.ID) {
			t.Fatalf("expand changed identity: %+v", v)
		}
	}
	empty := query(2, 2.5, nil)
	if empty.SampleCount != 0 || len(empty.Cohorts) != 0 {
		t.Fatal("wrong-window fallback")
	}
	missing := query(1, 2, map[string]any{"sampling_root": "absent"})
	for _, v := range missing.Cohorts {
		if len(v.Nodes) != 0 || v.OmittedNodes != v.NodeCount {
			t.Fatal("unknown display root fell back")
		}
	}
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Ctx = dead
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 2})
	r, _ := (&TraceQuery{}).Execute(ctx, raw)
	if r.Success || len(r.Observations) > 0 || r.TraceViewCancellation == nil {
		t.Fatalf("cancellation published: %+v", r)
	}
}
