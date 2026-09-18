package traceinput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func beginBinaryTest(t *testing.T, ctx context.Context) (*Preparation, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "raw capture.sys")
	body := realRMQFixture(40)
	writeTestFile(t, input, body)
	p, err := Begin(ctx, Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 768})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Discard() })
	return p, input, body
}

func TestPreparationCommitCompleteBinaryAndDiscardDoesNotDeletePublished(t *testing.T) {
	p, input, original := beginBinaryTest(t, context.Background())
	material, err := p.Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Discard(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Commit(context.Background()); err == nil {
		t.Fatal("repeated commit reissued authority")
	}
	idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
	if err != nil || len(idx.Events) != 40 || idx.Events[39].WakeePID != 139 {
		t.Fatalf("published complete binary material unavailable: %v %v", idx, err)
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("commit modified original: %v", err)
	}
}

func TestPreparationCancelBetweenBeginAndCommitRollsBack(t *testing.T) {
	for _, cancelOriginal := range []bool{true, false} {
		t.Run(map[bool]string{true: "begin_context", false: "commit_context"}[cancelOriginal], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			beginCtx := context.Background()
			if cancelOriginal {
				beginCtx = ctx
			}
			p, input, original := beginBinaryTest(t, beginCtx)
			output := p.owned.path
			cancel()
			commitCtx := ctx
			if cancelOriginal {
				commitCtx = context.Background()
			}
			if material, err := p.Commit(commitCtx); material != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("late cancellation published: %v %v", material, err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unpublished conversion not rolled back: %v", err)
			}
			if err := p.Discard(); !errors.Is(err, context.Canceled) {
				t.Fatalf("terminal outcome disappeared: %v", err)
			}
			if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("cancel modified original: %v", err)
			}
		})
	}
}

func TestPreparationChangedSourceOrMemberCannotCommit(t *testing.T) {
	for _, source := range []bool{true, false} {
		t.Run(map[bool]string{true: "source", false: "derived"}[source], func(t *testing.T) {
			p, input, _ := beginBinaryTest(t, context.Background())
			path := p.material.QueryPath()
			if source {
				path = input
			}
			writeTestFile(t, path, []byte("changed generation"))
			output := p.owned.path
			if material, err := p.Commit(context.Background()); material != nil || err == nil {
				t.Fatal("changed generation committed")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failed commit kept owned conversion: %v", err)
			}
			if source {
				if got, err := os.ReadFile(input); err != nil || string(got) != "changed generation" {
					t.Fatal("cleanup deleted caller's changed source")
				}
			}
		})
	}
}

func TestPreparationPlainTextDiscardNeverDeletesSource(t *testing.T) {
	input := filepath.Join(t.TempDir(), "capture.systrace")
	writeTestFile(t, input, []byte("worker-42 (42) [000] .... 1.000000: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=0\n"))
	p, err := Begin(nil, Options{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Discard(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatal("discard deleted original text", err)
	}
	if m, err := p.Commit(nil); m != nil || err == nil {
		t.Fatal("discarded text can be committed")
	}
	var zero Preparation
	if m, err := zero.Commit(nil); m != nil || err == nil {
		t.Fatal("zero preparation can commit")
	}
}

func TestPreparationConcurrentCommitDiscardHaveOneTerminalOutcome(t *testing.T) {
	p, _, _ := beginBinaryTest(t, context.Background())
	output := p.owned.path
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var committed bool
	go func() {
		defer wg.Done()
		<-start
		material, err := p.Commit(context.Background())
		committed = material != nil && err == nil
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := p.Discard(); err != nil {
			t.Error(err)
		}
	}()
	close(start)
	wg.Wait()
	_, err := os.Stat(output)
	if committed && err != nil || !committed && !os.IsNotExist(err) {
		t.Fatalf("published=%v output status=%v", committed, err)
	}
}

func TestPreparationSwappedOutputKeepsReplacementAndTerminalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows held directory denies this rename; shared authority covers its native guard")
	}
	p, _, _ := beginBinaryTest(t, context.Background())
	output := p.owned.path
	moved := output + ".moved"
	if err := os.Rename(output, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(output, "not-ours")
	writeTestFile(t, sentinel, []byte("replacement"))
	if material, err := p.Commit(context.Background()); err == nil || material != nil {
		t.Fatal("swapped output published")
	}
	first := p.Discard()
	if first == nil {
		t.Fatal("cleanup failure hidden")
	}
	if err := os.Rename(output, output+".other"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, output); err != nil {
		t.Fatal(err)
	}
	if err := p.Discard(); err == nil || err.Error() != first.Error() {
		t.Fatal("ABA rearmed failed transaction")
	}
	if got, err := os.ReadFile(filepath.Join(output+".other", "not-ours")); err != nil || string(got) != "replacement" {
		t.Fatal("replacement data deleted")
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal("ABA retry deleted original output")
	}
}
