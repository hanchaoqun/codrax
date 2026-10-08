package tracequery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResourceStackStreamFullSourceParity(t *testing.T) {
	idx := resourceStackTestIndex(t, resourceStackTestRows(t, 150))
	for _, q := range []Query{
		{View: ViewResourceStack},
		{View: ViewResourceStack, PID: 11, TimeStart: 1, TimeEnd: 2},
		{View: ViewResourceStack, PID: 10, TargetScope: TargetScopeProcess, TimeStart: 1, TimeEnd: 2},
		{View: ViewResourceStack, Thread: "missing", TimeStart: 1, TimeEnd: 2},
		{View: ViewResourceStack, TimeStartSet: true, TimeEnd: 1},
	} {
		got, err := StreamResourceStack(context.Background(), idx.Path, q)
		if err != nil {
			t.Fatal(err)
		}
		want := Run(idx, q)
		if !reflect.DeepEqual(got.ResourceStack, want.ResourceStack) || got.ScannedLineCount != idx.LineCount || got.EventCount != len(idx.Events) || got.RootCauseRank != nil {
			t.Fatalf("stream/index disagreement q=%+v\ngot=%+v\nwant=%+v", q, got.ResourceStack, want.ResourceStack)
		}
	}
}

func TestResourceStackStreamNoPrefixEvidence(t *testing.T) {
	base := resourceStackTestRows(t, 3)
	for name, suffix := range map[string]string{
		"missing_last_frame":     "",
		"malformed_tail":         "# codrax_resource_stack/v2 record=bad\n",
		"duplicate_after_window": strings.Split(base, "\n")[0] + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			body := base + strings.Repeat("# padding\n", 1024) + suffix
			if name == "missing_last_frame" {
				rows := strings.Split(strings.TrimSpace(base), "\n")
				body = strings.Join(rows[:len(rows)-1], "\n") + "\n"
			}
			idx := resourceStackTestIndex(t, body)
			q := Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 1.001}
			got, err := StreamResourceStack(context.Background(), idx.Path, q)
			if err != nil || got.ResourceStack.Status != "unavailable" || len(got.ResourceStack.Events) != 0 || got.ScannedLineCount != idx.LineCount {
				t.Fatalf("incomplete source published: %v %+v", err, got)
			}
		})
	}
	idx := resourceStackTestIndex(t, base+strings.Repeat("# padding\n", 1024))
	for _, limits := range [][2]int{{2, 1 << 20}, {100, 10}} {
		got, err := streamResourceStack(context.Background(), idx.Path, Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2}, limits[0], limits[1])
		if err != nil || got.ResourceStack.Reason != "resource_stack_complete_scan_retention_limit" || got.ScannedLineCount != idx.LineCount || got.EventCount != len(idx.Events) || len(got.ResourceStack.Events) != 0 || got.ResourceStack.MatchedEvents != 0 {
			t.Fatalf("cap published prefix or stopped before EOF: %v %+v", err, got)
		}
	}
}

func TestResourceStackStreamVerifiedBundleAndIsolation(t *testing.T) {
	for _, shape := range []string{"identity", "multi", "affine", "stale"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			child, bundle := filepath.Join(dir, "events.systrace"), filepath.Join(dir, "capture.tracebundle.json")
			body := resourceStackTestRows(t, 2)
			if err := os.WriteFile(child, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			manifest := `{"systrace":"events.systrace"}`
			if shape == "multi" {
				if err := os.WriteFile(filepath.Join(dir, "other.systrace"), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				manifest = `{"systrace":"events.systrace","artifacts":[{"type":"systrace","path":"other.systrace"}]}`
			}
			if shape == "affine" {
				manifest = `{"systrace":"events.systrace","perf_clock_alignments":[{"artifact_path":"events.systrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":1,"slope":1,"calibrated":true}]}`
			}
			writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
			if shape == "stale" {
				if err := os.WriteFile(child, []byte(body+"# changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			q := Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2}
			got, err := StreamResourceStack(context.Background(), bundle, q)
			if shape != "identity" {
				if err == nil || got.ResourceStack != nil {
					t.Fatalf("unproven source admitted: %v %+v", err, got)
				}
				return
			}
			idx, buildErr := BuildIndex(context.Background(), bundle)
			if err != nil || buildErr != nil || !reflect.DeepEqual(got.ResourceStack, Run(idx, q).ResourceStack) || got.SourcePath != canonicalTraceIndexPath(bundle) || len(got.TraceArtifacts) != 1 || got.TraceArtifacts[0].SourcePath != canonicalTraceIndexPath(child) || got.TraceArtifacts[0].CaptureID == "" {
				t.Fatalf("verified bundle lost identity or source mapping: %v %v %+v", err, buildErr, got)
			}
		})
	}
}

func TestResourceStackStreamCancellationAndGeneration(t *testing.T) {
	idx := resourceStackTestIndex(t, resourceStackTestRows(t, 150))
	q := Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2}
	full, err := StreamResourceStack(context.Background(), idx.Path, q)
	if err != nil {
		t.Fatal(err)
	}
	canceled, completed := false, false
	for _, k := range []int{0, 1, 5, 16, 64, 128, 512, 1024} {
		got, err := StreamResourceStack(newRunCancelAfterN(k), idx.Path, q)
		if err != nil {
			if !errors.Is(err, context.Canceled) || got.ResourceStack != nil {
				t.Fatalf("cancel became unavailable success: %v %+v", err, got)
			}
			canceled = true
		} else {
			completed = true
			if !reflect.DeepEqual(got.ResourceStack, full.ResourceStack) {
				t.Fatal("canceled scan published a partial face")
			}
		}
	}
	if !canceled || !completed {
		t.Fatalf("cancel sweep did not cover both outcomes: %t %t", canceled, completed)
	}
	for _, direct := range []Query{q.WithRunContext(newRunCancelAfterN(0)), q.WithRunContext(newRunCancelAfterN(1))} {
		direct.runCancel.units = runCancelSampleMask
		got := Run(idx, direct)
		if got.ResourceStack != nil || got.ViewCancellation == nil {
			t.Fatalf("indexed cancellation published an unavailable face: %+v", got)
		}
	}
	selection, err := resolveTraceIndexSelection(context.Background(), idx.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer selection.close()
	changed := false
	_, err = resourceStackScanSource(context.Background(), selection, idx.Path, "", func(Event) bool {
		if !changed {
			changed = true
			if err := os.WriteFile(idx.Path, []byte("# replaced\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return true
	})
	if err == nil || !changed {
		t.Fatal("in-scan generation mutation accepted")
	}
}
