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
	for _, name := range []string{"a.systrace", "b.trace", "ignore.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("trace"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	units, err := discoverTraceBatchUnits(dir, defaultTraceBatchPatterns(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 || units[0].UnitID != "a" || units[1].UnitID != "b" {
		t.Fatalf("unexpected units: %+v", units)
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
