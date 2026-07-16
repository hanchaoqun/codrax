package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiblingBundlePromotionRequiresExactSystraceMembership(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n")
	writeBundleMembershipFixture(t, bundlePath, `{"version":"test","systrace":"capture.systrace"}`)

	wantBundle := canonicalTraceIndexPath(bundlePath)
	if got := promoteSiblingTraceBundlePath(requested); got != wantBundle {
		t.Fatalf("member bundle was not promoted: got %q want %q", got, wantBundle)
	}
	idx, err := BuildIndex(context.Background(), requested)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != wantBundle {
		t.Fatalf("BuildIndex did not bind the member request to its bundle: got %q want %q", idx.Path, wantBundle)
	}
}

func TestStaleSiblingBundleCannotHijackDirectSystrace(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	other := filepath.Join(dir, "other.systrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, "requested-20 (20) [001] .... 10.000000: sched_wakeup: comm=requested pid=20 prio=20 target_cpu=001\n")
	writeBundleMembershipFixture(t, other, "other-99 (99) [001] .... 20.000000: sched_wakeup: comm=other pid=99 prio=20 target_cpu=001\n")
	writeBundleMembershipFixture(t, bundlePath, `{"version":"stale","systrace":"other.systrace"}`)

	wantPath := canonicalTraceIndexPath(requested)
	if got := promoteSiblingTraceBundlePath(requested); got != wantPath {
		t.Fatalf("stale non-member manifest hijacked request: got %q want %q", got, wantPath)
	}
	if tracePathRequiresCompositeIndex(requested) {
		t.Fatal("stale non-member manifest incorrectly forced the requested trace into a composite universe")
	}
	idx, err := BuildIndex(context.Background(), requested)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != wantPath || len(idx.Events) != 1 || idx.Events[0].PID != 20 {
		t.Fatalf("direct requested trace was not preserved: path=%q events=%+v", idx.Path, idx.Events)
	}
}

func TestSiblingBundleArtifactMembershipUsesCanonicalRelativePath(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n")
	// V2 paths are wire identities, not cleanup hints. A lexical alias is an
	// invalid optional sibling and must leave the explicit physical trace alone.
	if err := os.WriteFile(bundlePath, []byte(`{
  "schema":"codrax.tracebundle/v2",
  "capture_id":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
  "version":"test",
  "artifacts":[{"type":"systrace","path":"./missing/../capture.systrace"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, want := promoteSiblingTraceBundlePath(requested), canonicalTraceIndexPath(requested); got != want {
		t.Fatalf("noncanonical optional artifact declaration was promoted: got %q want %q", got, want)
	}
	idx, err := BuildIndex(context.Background(), requested)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != canonicalTraceIndexPath(requested) || len(idx.TraceArtifacts) != 1 || idx.TraceArtifacts[0].SourcePath != canonicalTraceIndexPath(requested) {
		t.Fatalf("invalid optional bundle changed the direct physical member: %+v", idx.TraceArtifacts)
	}
}

func TestInvalidOrUnreadableSiblingBundleFallsBackToDirectTrace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "invalid_json", setup: func(t *testing.T, path string) {
			writeBundleMembershipFixture(t, path, `{"systrace":`)
		}},
		{name: "unreadable_candidate", setup: func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			requested := filepath.Join(dir, "capture.systrace")
			bundlePath := filepath.Join(dir, "capture.tracebundle.json")
			writeBundleMembershipFixture(t, requested, "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n")
			tc.setup(t, bundlePath)

			wantPath := canonicalTraceIndexPath(requested)
			if got := promoteSiblingTraceBundlePath(requested); got != wantPath {
				t.Fatalf("invalid/unreadable optional manifest was promoted: got %q want %q", got, wantPath)
			}
			idx, err := BuildIndex(context.Background(), requested)
			if err != nil {
				t.Fatalf("direct trace became unusable because of optional manifest: %v", err)
			}
			if idx.Path != wantPath || len(idx.Events) != 1 {
				t.Fatalf("direct fallback mismatch: path=%q events=%+v", idx.Path, idx.Events)
			}
		})
	}
}

func TestExplicitPerftraceNeverPromotesToSiblingBundle(t *testing.T) {
	dir := t.TempDir()
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, perftrace, "app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=App dso=lib.so source=test\n")
	writeBundleMembershipFixture(t, bundlePath, `{"version":"test","artifacts":[{"type":"perftrace","path":"capture.perftrace"}]}`)

	if got, want := promoteSiblingTraceBundlePath(perftrace), canonicalTraceIndexPath(perftrace); got != want {
		t.Fatalf("explicit perftrace was promoted: got %q want %q", got, want)
	}
}

func writeBundleMembershipFixture(t *testing.T, path, body string) {
	t.Helper()
	data := []byte(body)
	if strings.HasSuffix(path, ".tracebundle.json") && json.Valid(data) {
		data = traceBundleV2JSONForTest(t, path, data)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
