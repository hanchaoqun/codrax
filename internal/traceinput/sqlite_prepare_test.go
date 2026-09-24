package traceinput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func existingSQLiteFixture(t *testing.T) (string, []byte) {
	t.Helper()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_existing_sqlite/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "existing capture.data")
	writeTestFile(t, path, body)
	return path, body
}

func TestPrepareExistingSQLiteReadOnlyCompleteMaterial(t *testing.T) {
	path, original := existingSQLiteFixture(t)
	if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })
	anchor := t.TempDir()
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(anchor, "absent-converter"))
	material, err := prepare(t.Context(), Options{InputPath: path, RuntimeAnchor: anchor, PreviewBytes: 640}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
		t.Fatal("existing SQLite invoked a binary converter")
		return hitraceconv.Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || len(material.Preview()) > 640 || strings.Contains(material.Preview(), "tail-business-marker") {
		t.Fatalf("source/query/preview contract lost: %v", material)
	}
	idx, err := tracequery.BuildIndex(t.Context(), material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	zero := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "zero-time-marker", Limit: 10})
	if len(zero.Events) != 1 || zero.Events[0].Ts != 0 {
		t.Fatalf("zero time lost: %+v", zero.Events)
	}
	tail := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "tail-business-marker", TimeStart: 2.98, TimeEnd: 3.0, Limit: 10})
	if len(tail.Events) != 1 {
		t.Fatalf("full DB tail lost: %+v", tail.Events)
	}
	spans, _ := tracequery.FindSpanWindows(idx, tracequery.Query{SpanName: "tail-business-marker"}, 10)
	if len(spans) != 1 || spans[0].EndTs < 2.994999 {
		t.Fatalf("duration tail lost: %+v", spans)
	}
	for _, e := range idx.Events {
		if e.Type == "sched_switch" || e.Type == "sched_wakeup" {
			t.Fatalf("thread-state inventory invented scheduler edge: %+v", e)
		}
	}
	receiptBytes, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt preparationReceipt
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Conversion == nil || receipt.Conversion.ExistingTraceDBSource == nil || receipt.SourceKind != "sqlite" || receipt.SourceSHA256 == "" {
		t.Fatalf("missing database source receipt: %+v", receipt)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("modified original: %v", err)
	}
	if files, err := os.ReadDir(filepath.Dir(path)); err != nil || len(files) != 1 {
		t.Fatalf("created source sidecars: %v %v", files, err)
	}
}

func TestPrepareExistingSQLiteSidecarInvalidatesCommitAndCachedMaterial(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			path, _ := existingSQLiteFixture(t)
			anchor := t.TempDir()
			pending, err := Begin(t.Context(), Options{InputPath: path, RuntimeAnchor: anchor})
			if err != nil {
				t.Fatal(err)
			}
			defer pending.Discard()
			writeTestFile(t, path+suffix, []byte("new state without modifying the main DB"))
			if m, err := pending.Commit(t.Context()); err == nil || m != nil {
				t.Fatalf("new sidecar passed commit: %v %v", m, err)
			}
			if dirs, _ := filepath.Glob(filepath.Join(anchor, "trace-input-*")); len(dirs) != 0 {
				t.Fatalf("failed commit leaked: %v", dirs)
			}
			if _, err := os.Stat(path + suffix); err != nil {
				t.Fatal("rollback removed caller sidecar", err)
			}
		})
	}
	path, _ := existingSQLiteFixture(t)
	c := NewCoordinator(Options{RuntimeAnchor: t.TempDir()})
	m, err := c.Prepare(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path+"-wal", []byte("new WAL"))
	if err := m.Validate(t.Context(), m.Preview()); err == nil {
		t.Fatal("source sidecar retained prepared authority")
	}
	if next, err := c.Prepare(t.Context(), path); err == nil || next != nil {
		t.Fatal("coordinator ignored newly active DB")
	}
}

func TestPrepareExistingSQLiteRejectsSameSizeSameMtimeReplacement(t *testing.T) {
	path, original := existingSQLiteFixture(t)
	c := NewCoordinator(Options{RuntimeAnchor: t.TempDir()})
	m, err := c.Prepare(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	writeTestFile(t, replacement, original)
	if err := os.Chtimes(replacement, stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(t.Context(), m.Preview()); err == nil {
		t.Fatal("same-size/mtime replacement retained authority")
	}
	if _, err := c.Prepare(t.Context(), path); err == nil {
		t.Fatal("same Run silently rebuilt replaced DB")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Prepare(ctx, Options{InputPath: path, RuntimeAnchor: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
