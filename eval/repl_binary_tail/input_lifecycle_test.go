package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type eofReader struct {
	io.Reader
	eof  chan struct{}
	once sync.Once
}

func (r *eofReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.once.Do(func() { close(r.eof) })
	}
	return n, err
}

type slowRunner struct {
	entered, release, canceled chan struct{}
	cancelOnce                 sync.Once
	material                   *attachment.TraceMaterial
	body, source               string
	request                    string
}

func (r *slowRunner) Run(request, _, _ string) (*types.BusContext, error) {
	r.request = request
	close(r.entered)
	<-r.release
	return &types.BusContext{Mutable: types.NewMutableState(request)}, nil
}
func (r *slowRunner) Cancel(string) { r.cancelOnce.Do(func() { close(r.canceled) }) }
func (r *slowRunner) IsCanceled() bool {
	select {
	case <-r.canceled:
		return true
	default:
		return false
	}
}
func (r *slowRunner) SetAttachedTraceMaterial(material *attachment.TraceMaterial) {
	r.material = material
}
func (r *slowRunner) SetAttachedHitrace(body string)                   { r.body, r.material = body, nil }
func (r *slowRunner) SetAttachedHitraceSource(source string)           { r.source = source }
func (r *slowRunner) AttachedHitrace() string                          { return r.body }
func (r *slowRunner) AttachedHitraceSource() string                    { return r.source }
func (r *slowRunner) AttachedTraceMaterial() *attachment.TraceMaterial { return r.material }
func (r *slowRunner) SetReadRunSnapshotSeed(*types.ReadRunSnapshot)    {}

type noModelSummarizer struct{}

func (noModelSummarizer) Summarize(context.Context, memory.Turn) (memory.IndexEntry, error) {
	return memory.IndexEntry{}, nil
}

// A deterministic input-lifecycle test, NOT the live-eval Runner. The live
// entry always execs codrax. Here a deliberately blocked public Loop proves
// queued /exit and physical EOF cannot cancel the pending user answer.
func TestPublicREPLQueuedExitWaitsForSlowAnswer(t *testing.T) {
	out, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(out, "workspace")
	for _, name := range []string{"workspace", "input", "logs"} {
		if err := os.Mkdir(filepath.Join(out, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	input := filepath.Join(out, "input", "capture.sys")
	if err := os.WriteFile(input, rmqFixture(), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := memory.NewStore(filepath.Join(dir, "memory"), noModelSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	in := &eofReader{Reader: strings.NewReader("/htrace " + input + "\n" + question + "\n/exit\n"), eof: make(chan struct{})}
	runner := &slowRunner{entered: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(runner.release) }) }
	t.Cleanup(release)
	rendered := make(chan struct{}, 1)
	output, err := os.Create(filepath.Join(dir, "repl.out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	r := repl.New(repl.Config{Runner: runner, Store: store, RepoRoot: dir, In: in, Out: output,
		RuntimeAnchor: filepath.Join(dir, ".codrax"), AttachedTraceMaxBytes: previewBytes,
		Render: func(*types.BusContext) string { rendered <- struct{}{}; return "deterministic lifecycle answer" },
	})
	done := make(chan error, 1)
	go func() { done <- r.Loop() }()
	for name, ch := range map[string]<-chan struct{}{"slow Run entered": runner.entered, "exit consumed through EOF": in.eof} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			body, _ := os.ReadFile(output.Name())
			t.Fatalf("%s did not occur: %s", name, body)
		}
	}
	if runner.IsCanceled() || runner.request != question || runner.material == nil || strings.Contains(runner.material.Preview(), "tail-target") {
		t.Fatal("queued exit canceled Run or attachment/request handoff was lost")
	}
	select {
	case err := <-done:
		t.Fatalf("Loop exited before answer was released: %v", err)
	default:
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued exit did not end completed Loop")
	}
	select {
	case <-rendered:
	default:
		t.Fatal("exit lost the pending answer")
	}
	// Verify the audit's exact snapshot JSON keys against public Loop output,
	// and its preparation/query joins against a real native tool publication.
	// This still is deterministic, not evidence of a model choosing the tool.
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: runner.body, AttachedHitraceSource: runner.source,
		AttachedTraceMaterial: runner.material, Mutable: types.NewMutableState(question)}
	result, err := (&tool.TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"attached_trace","view":"event_search","time_start":1.0105,"time_end":1.012,"limit":20}`))
	if err != nil || !result.Success {
		t.Fatalf("native complete-tail query failed: result=%+v err=%v", result, err)
	}
	receipt := auditRun(out, fixtureOracle{InputSHA256: bytesSHA(rmqFixture()), ExpectedTime: 1.011,
		ExpectedName: "tail-target", ExpectedPID: 424242})
	for _, check := range []string{"input_unchanged", "child_preparation_bound_to_original", "repl_snapshot_preview_excludes_tail", "native_query_contains_exact_tail"} {
		if !receipt.Checks[check] {
			t.Fatalf("public artifact schema/join drift: %s: %+v", check, receipt)
		}
	}
	receipt.finish()
	if receipt.Status != "machine_checks_failed" || receipt.Checks["model_and_tool_telemetry"] || receipt.Checks["answer_mentions_expected_tuple"] {
		t.Fatalf("deterministic public-loop test forged live acceptance: %+v", receipt)
	}
}
