package cmd

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedLogCLICompleteSourcesSurvivePreviewAndProjection(t *testing.T) {
	resetPreparedTraceFlags(t)
	oldCap := maxAttachedLogBytes
	t.Cleanup(func() { maxAttachedLogBytes = oldCap })
	maxAttachedLogBytes = 128
	dir := t.TempDir()
	first, second := filepath.Join(dir, "hilog.txt"), filepath.Join(dir, "kernel.gz")
	body := strings.Repeat("ordinary line\n", 100) + "10-09 12:34:56.123 41 42 E Worker: tail-target\n  continued detail\n"
	if err := os.WriteFile(first, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(second)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err := io.WriteString(gz, "10-09 12:34:56.124 <4>[25175.823383662] -;[2] pid=41 tid=42 comm=worker kernel-tail\n"); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	flagAttachLog = []string{first, filepath.Join(dir, "missing.log"), second}
	loaded, err := loadPreparedAttachedLog(context.Background())
	if err != nil || loaded.catalog == nil {
		t.Fatalf("prepare=%v %#v", err, loaded)
	}
	if len(loaded.body) > maxAttachedLogBytes || strings.Contains(loaded.body, "tail-target") {
		t.Fatal("preview changed source scope")
	}
	bus := &types.BusContext{AttachedLog: loaded.body, AttachedLogCatalog: loaded.catalog, Mutable: types.NewMutableState("inspect logs")}
	ac := promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	projected := types.ToolBusContext(ac, types.AgentExplorer)
	sub := types.SubAgentContext(bus, &types.SubAgentRequest{SubAgent: string(types.AgentExplorer), Objective: "inspect logs"})
	if projected.AttachedLogCatalog != loaded.catalog || sub.AttachedLogCatalog != loaded.catalog || sub.AttachedLog != loaded.body {
		t.Fatal("catalog lost across projection")
	}
	result, err := projected.AttachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "tail", Limit: 20})
	if err != nil || result.Matched != 2 || result.Complete || len(result.SourceErrors) != 1 {
		t.Fatalf("query=%+v err=%v", result, err)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.BusContext
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.AttachedLogCatalog != nil {
		t.Fatal("JSON recreated query authority")
	}
	if err := os.WriteFile(first, []byte("replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := projected.AttachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "tail", Limit: 20})
	if err != nil || changed.Matched != 1 || len(changed.SourceErrors) != 2 {
		t.Fatalf("stale source reused: %+v %v", changed, err)
	}
}

func TestPreparedLogCLIRejectsConflictsBeforeStdinAndFailsAllBad(t *testing.T) {
	resetPreparedTraceFlags(t)
	flagAttachLog, flagAttachHitrace = []string{"-"}, []string{"-"}
	if loaded, err := loadPreparedAttachedLog(context.Background()); err == nil || loaded.catalog != nil {
		t.Fatal("duplicate stdin accepted")
	}
	flagAttachHitrace = nil
	flagAttachLog = []string{filepath.Join(t.TempDir(), "missing")}
	if loaded, err := loadPreparedAttachedLog(context.Background()); err == nil || loaded.catalog != nil || loaded.body != "" {
		t.Fatal("all-bad source published")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if loaded, err := loadPreparedAttachedLog(ctx); !errors.Is(err, context.Canceled) || loaded.catalog != nil {
		t.Fatal("cancellation published source")
	}
}

func TestPreparedLogStreamEOFAndCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := readCompleteLogStream(ctx, reader, 1024); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked stream not canceled")
	}
	_ = writer.Close()
	data, err := readCompleteLogStream(context.Background(), io.NopCloser(strings.NewReader("complete EOF")), 32)
	if err != nil || string(data) != "complete EOF" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if data, err := readCompleteLogStream(context.Background(), io.NopCloser(strings.NewReader("oversized")), 3); err == nil || data != nil {
		t.Fatal("input cap sealed partial stream")
	}
}
