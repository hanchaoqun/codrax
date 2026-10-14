package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerfSamplingSourceBindingNativeOnlyHMC241(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), "tid=11 ", "tid=11 thread_identity_known=true "))
	if err := os.WriteFile(filepath.Join(dir, "samples.perftrace"), body, 0600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "prepared.tracebundle.json")
	writeTraceBundleV2ForTest(t, bundle, []byte(`{"artifacts":[{"type":"perftrace","path":"samples.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}]}`))
	result, err := StreamPerfSamplingTree(context.Background(), bundle, Query{View: ViewPerfSamplingTree, PID: 11, TimeStart: 1, TimeEnd: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := PerfSamplingTreeBoundSource(result); !ok {
		t.Fatal("real frozen manifest/member generation did not bind")
	}
	encoded, _ := json.Marshal(result)
	var decoded Result
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := PerfSamplingTreeBoundSource(decoded); ok {
		t.Fatal("copyable JSON provenance minted a private source binding")
	}
	for _, name := range []string{"missing_member", "duplicate_member", "unbound_schema", "capture_identity", "digest", "bytes", "mtime", "generation", "different_member", "different_cohort_source", "different_cohort_generation"} {
		t.Run(name, func(t *testing.T) {
			changed := result
			changed.TraceArtifacts = append([]TraceArtifactSource(nil), result.TraceArtifacts...)
			var p PerfSamplingTreeResult
			raw, _ := json.Marshal(result.PerfSamplingTree)
			_ = json.Unmarshal(raw, &p)
			changed.PerfSamplingTree = &p
			switch name {
			case "missing_member":
				changed.TraceArtifacts = nil
			case "duplicate_member":
				changed.TraceArtifacts = append(changed.TraceArtifacts, changed.TraceArtifacts[0])
			case "unbound_schema":
				changed.TraceArtifacts[0].BundleSchema = ""
			case "capture_identity":
				changed.TraceArtifacts[0].CaptureID = "unknown"
			case "digest":
				changed.TraceArtifacts[0].SourceSHA256 = strings.Repeat("0", 64)
			case "bytes":
				changed.TraceArtifacts[0].SourceBytes++
			case "mtime":
				changed.TraceArtifacts[0].SourceModUnixNano++
			case "generation":
				for i := range p.Cohorts {
					p.Cohorts[i].SourceGeneration += ".other"
				}
			case "different_member":
				for i := range p.Cohorts {
					p.Cohorts[i].SourcePath += ".other"
				}
			case "different_cohort_source":
				p.Cohorts[1].SourcePath += ".other"
			case "different_cohort_generation":
				p.Cohorts[1].SourceGeneration += ".other"
			}
			if _, ok := PerfSamplingTreeBoundSource(changed); ok {
				t.Fatal("changed/unproven source universe minted a display binding")
			}
		})
	}
}
