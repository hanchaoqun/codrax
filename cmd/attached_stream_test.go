package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestCLITraceStdinAutomaticallyPreparesCompleteSQLite(t *testing.T) {
	resetPreparedTraceFlags(t)
	flagAttachHitrace = []string{"-"}
	maxAttachedTraceBytes = 640
	path, err := filepath.Abs(filepath.Join("..", "eval", "fixtures", "hmosperf_referenced_dictionary", "capture.data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	prior := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = prior; _ = input.Close() })
	loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
	if err != nil || loaded.material == nil {
		t.Fatalf("stdin not prepared: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), loaded.material.QueryPath())
	if err != nil || len(idx.Events) != 20 {
		t.Fatalf("stdin query lost full dictionary events: %d / %v", len(idx.Events), err)
	}
	if len(loaded.body) > 640 {
		t.Fatal("full input leaked into model preview")
	}
}

func TestCLITraceStdinPipeCancellationDoesNotPublish(t *testing.T) {
	resetPreparedTraceFlags(t)
	flagAttachAtrace = []string{"-"}
	t.Chdir(t.TempDir())
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	old := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = old; _ = reader.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		loaded, err := loadPreparedAttachedTrace(ctx, nil)
		if loaded.material != nil {
			done <- errors.New("cancelled stdin published")
			return
		}
		done <- err
	}()
	<-started
	if _, err := writer.Write([]byte("prefix without EOF")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel not preserved: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stdin cancellation did not unblock")
	}
}
