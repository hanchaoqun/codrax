package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDiscoverTraceBatchUnitsSameDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.systrace": "trace",
		"b.trace":    "trace",
		"c.txt":      "worker-1 [000] 1.000: sched_switch: prev_pid=1 next_pid=2",
		"d":          "worker-2 [001] 1.100: sched_wakeup: pid=3",
		"ignore.log": "ordinary application log",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	units, err := discoverTraceBatchUnits(dir, defaultTraceBatchPatterns(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 4 || units[0].UnitID != "a" || units[1].UnitID != "b" || units[2].UnitID != "c" || units[3].UnitID != "d" {
		t.Fatalf("unexpected units: %+v", units)
	}
}

func TestAppendTraceBatchDetailLinksPreservesOriginalReportPaths(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "batch")
	reports := map[string]string{
		"trace-b": filepath.Join(outputDir, "reports", "trace-b.md"),
		"trace-a": filepath.Join(outputDir, "reports", "trace-a.md"),
	}
	got := appendTraceBatchDetailLinks("# Trace 根因聚类报告\n", outputDir, reports, "zh-CN")
	for _, want := range []string{"每份 Trace 的完整分析", "没有覆盖原始结果", "reports/trace-a.md", "reports/trace-b.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch report missing %q:\n%s", want, got)
		}
	}
}

func TestTraceBatchChildReceivesPhysicalTraceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raw.trace")
	args := traceBatchChildArgs(traceBatchUnit{UnitID: "raw", Path: path}, "finding.json", "分析根因")
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--htrace" && args[i+1] == path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("batch child did not receive the physical trace path: %v", args)
	}
}

func TestRenderTraceClusterMarkdownShowsShortChineseCause(t *testing.T) {
	set := types.TraceRootCauseClusterSetV1{
		BatchID: "b1", InputUnitCount: 2, SuccessfulCount: 2, ResolvedCount: 2,
		Clusters: []types.TraceRootCauseCluster{{
			ClusterID: "rc-1", PrimaryCount: 2, ShareOfResolved: 1,
			Fingerprint:    types.TraceCauseFingerprintV1{Token: "scheduler_latency"},
			PrimaryMembers: []types.TraceClusterMember{{UnitID: "a"}, {UnitID: "b"}},
		}},
	}
	got := renderTraceClusterMarkdown(set, "zh-CN")
	for _, want := range []string{"Trace 根因聚类报告", "调度延迟", "100.0%", "`a`", "`b`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q:\n%s", want, got)
		}
	}
}

func TestTraceBatchAndClusterHelpExposeLocalFlags(t *testing.T) {
	batch := commandHelpText(t, traceBatchCmd)
	for _, want := range []string{"Input:", "Batch:", "--input-dir", "--pattern", "--concurrency", "--output-dir"} {
		if !strings.Contains(batch, want) {
			t.Fatalf("batch help missing %q:\n%s", want, batch)
		}
	}
	cluster := commandHelpText(t, traceClusterCmd)
	for _, want := range []string{"Input and output:", "--input-dir", "--format", "--output"} {
		if !strings.Contains(cluster, want) {
			t.Fatalf("cluster help missing %q:\n%s", want, cluster)
		}
	}
}
