package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/attachment"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

func resetPreparedTraceFlags(t *testing.T) {
	t.Helper()
	oldLog, oldH, oldA := flagAttachLog, flagAttachHitrace, flagAttachAtrace
	oldLogText, oldHText, oldAText := flagAttachLogText, flagAttachHitraceText, flagAttachAtraceText
	oldCap, oldPrepare := maxAttachedTraceBytes, prepareAttachedTraceInput
	t.Cleanup(func() {
		flagAttachLog, flagAttachHitrace, flagAttachAtrace = oldLog, oldH, oldA
		flagAttachLogText, flagAttachHitraceText, flagAttachAtraceText = oldLogText, oldHText, oldAText
		maxAttachedTraceBytes, prepareAttachedTraceInput = oldCap, oldPrepare
	})
	flagAttachLog, flagAttachHitrace, flagAttachAtrace = nil, nil, nil
	flagAttachLogText, flagAttachHitraceText, flagAttachAtraceText = "", "", ""
	maxAttachedTraceBytes = 4096
}

func preparedTraceTestMaterial(t *testing.T, source, query, preview string) *attachment.TraceMaterial {
	t.Helper()
	bindings := make(map[string]filegeneration.Identity)
	for _, path := range []string{source, query} {
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	material, err := attachment.BindTraceMaterial(source, query, preview, bindings)
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func TestLoadPreparedAttachedTraceFileUsesPreparationOnce(t *testing.T) {
	resetPreparedTraceFlags(t)
	dir := t.TempDir()
	source, query := filepath.Join(dir, "capture.sys"), filepath.Join(dir, "converted.systrace")
	if err := os.WriteFile(source, []byte("OHOSPROF\x00capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	row := "worker-12 (12) [000] .... 1.000000: sched_wakeup: comm=tail pid=42 prio=120 target_cpu=000\n"
	if err := os.WriteFile(query, []byte(row), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := "# codrax-source: " + source + "\n" + row
	material := preparedTraceTestMaterial(t, source, query, preview)
	flagAttachAtrace = []string{source}
	calls := 0
	prepareAttachedTraceInput = func(ctx context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
		calls++
		anchor, _ := filepath.Abs(runtimeAnchorDir)
		if opts.InputPath != source || opts.RuntimeAnchor != anchor || opts.PreviewBytes != maxAttachedTraceBytes {
			t.Fatalf("preparation options drifted: %+v", opts)
		}
		if ctx.Done() == nil {
			t.Fatal("file preparation has no signal-cancelable context")
		}
		return material, nil
	}
	loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
	if err != nil || calls != 1 || loaded.material != material || loaded.body != preview || loaded.source != "android_atrace" {
		t.Fatalf("loaded=%+v calls=%d err=%v", loaded, calls, err)
	}
	for _, lang := range []string{"en", "zh"} {
		lines := strings.Join(cliPreparedTraceLoadedLines(lang, material), "\n")
		for _, want := range []string{source, query} {
			if !strings.Contains(lines, want) {
				t.Fatalf("%s disclosure lost path %q: %s", lang, want, lines)
			}
		}
		if strings.Contains(lines, "loaded slice only") || strings.Contains(lines, "仅基于已加载内容") {
			t.Fatalf("complete material disclosed as preview-only: %s", lines)
		}
	}
}

func TestLoadPreparedAttachedTraceRejectsConflictsBeforePreparation(t *testing.T) {
	resetPreparedTraceFlags(t)
	calls := 0
	prepareAttachedTraceInput = func(context.Context, traceinput.Options) (*attachment.TraceMaterial, error) {
		calls++
		return nil, errors.New("must not prepare")
	}
	tests := []struct {
		name  string
		setup func()
	}{
		{"file and inline", func() { flagAttachHitrace, flagAttachHitraceText = []string{"a.sys"}, "text" }},
		{"aliases", func() { flagAttachHitrace, flagAttachAtrace = []string{"a.sys"}, []string{"b.sys"} }},
		{"multiple captures", func() { flagAttachHitrace = []string{"a.sys", "b.sys"} }},
		{"two stdin", func() { flagAttachLog, flagAttachHitrace = []string{"-"}, []string{"-"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flagAttachLog, flagAttachHitrace, flagAttachAtrace = nil, nil, nil
			flagAttachHitraceText, flagAttachAtraceText = "", ""
			test.setup()
			loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
			if err == nil || loaded.body != "" || loaded.material != nil || calls != 0 {
				t.Fatalf("conflict reached preparation: loaded=%+v calls=%d err=%v", loaded, calls, err)
			}
		})
	}
}

func TestLoadPreparedAttachedTraceInlineAndStdinRemainTextOnly(t *testing.T) {
	resetPreparedTraceFlags(t)
	prepareAttachedTraceInput = func(context.Context, traceinput.Options) (*attachment.TraceMaterial, error) {
		t.Fatal("inline/stdin invoked binary preparation")
		return nil, nil
	}
	flagAttachHitraceText = "worker-12 (12) [000] .... 1.000000: sched_switch: prev_pid=12 next_pid=42\n"
	loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
	if err != nil || loaded.body != flagAttachHitraceText || loaded.material != nil {
		t.Fatalf("inline text changed: %+v %v", loaded, err)
	}
	flagAttachHitraceText = "OHOSPROF\x00capture"
	if _, err := loadPreparedAttachedTrace(context.Background(), nil); err == nil {
		t.Fatal("binary inline accepted")
	}
	flagAttachHitraceText, flagAttachHitrace = "", []string{"-"}
	stdinPath := filepath.Join(t.TempDir(), "stdin.bin")
	if err := os.WriteFile(stdinPath, []byte("OHOSPROF\x00capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = oldStdin; _ = stdin.Close() })
	if _, err := loadPreparedAttachedTrace(context.Background(), nil); err == nil {
		t.Fatal("binary stdin accepted")
	}
}

func TestRootRunTracePreparationFailureAndCancellationDoNotPublishOrDispatch(t *testing.T) {
	resetPreparedTraceFlags(t)
	oldApp, oldRequest, oldDiag, oldAudit := app, flagRequest, flagTraceDiag, flagWriteAudit
	t.Cleanup(func() { app, flagRequest, flagTraceDiag, flagWriteAudit = oldApp, oldRequest, oldDiag, oldAudit })
	// Nil orchestrator is intentional: any attachment setter or pipeline
	// dispatch before successful preparation would panic instead of returning.
	app = appContext{}
	flagRequest, flagTraceDiag, flagWriteAudit = "inspect attached trace", "", ""
	flagAttachLogText, flagAttachHitrace = "new log that must not publish", []string{"capture.sys"}
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := errors.New("conversion failed")
			cleanupErr := errors.New("conversion rollback could not remove owned output")
			rollbackFinished := false
			prepareAttachedTraceInput = func(prepCtx context.Context, _ traceinput.Options) (*attachment.TraceMaterial, error) {
				defer func() { rollbackFinished = true }()
				if canceled {
					cancel()
					<-prepCtx.Done()
					return nil, errors.Join(prepCtx.Err(), cleanupErr)
				}
				return nil, want
			}
			command := &cobra.Command{}
			command.SetContext(ctx)
			err := rootRun(command, nil)
			if canceled {
				want = context.Canceled
			}
			if !errors.Is(err, want) || !rollbackFinished {
				t.Fatalf("err=%v want=%v rollbackFinished=%t", err, want, rollbackFinished)
			}
			if canceled && !errors.Is(err, cleanupErr) {
				t.Fatalf("cancellation discarded rollback failure: %v", err)
			}
		})
	}
}

func TestPrepareCLITraceFileSignalAllowsRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-SIGINT subprocess regression is Unix-only")
	}
	const helperKey = "CODRAX_TEST_ATTACHMENT_SIGNAL_HELPER"
	if os.Getenv(helperKey) == "1" {
		worktree.InstallSignalHandler()
		dir := t.TempDir()
		owned := filepath.Join(dir, "conversion-owned.tmp")
		prepareAttachedTraceInput = func(ctx context.Context, _ traceinput.Options) (*attachment.TraceMaterial, error) {
			if err := os.WriteFile(owned, []byte("rollback must remove this"), 0o600); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(owned)
			process, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if err := process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		material, err := prepareCLITraceFile(context.Background(), traceinput.Options{})
		if material != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("signal did not cancel preparation: material=%v err=%v", material, err)
		}
		if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("signal returned before rollback: %v", err)
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestPrepareCLITraceFileSignalAllowsRollback$", "-test.count=1", "-test.timeout=20s")
	command.Env = append(os.Environ(), helperKey+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("signal canceled the process before conversion rollback: %v\n%s", err, output)
	}
}

func TestLoadPreparedAttachedTraceBinaryQueriesTailBeyondPreview(t *testing.T) {
	resetPreparedTraceFlags(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(source, cliBinaryTraceTailFixture(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "missing-trace-streamer"))
	// Keep the runtime-managed conversion outputs inside this test directory.
	prepareAttachedTraceInput = func(ctx context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
		opts.RuntimeAnchor = filepath.Join(dir, ".codrax")
		opts.RuntimeAnchorFallback = ""
		return traceinput.Prepare(ctx, opts)
	}
	// Include the provenance envelope even when the managed directory's
	// canonical path is longer (for example /private/var on macOS).
	flagAttachHitrace, maxAttachedTraceBytes = []string{source}, 1024
	loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.material == nil || len(loaded.body) > maxAttachedTraceBytes || strings.Contains(loaded.body, "tail-target") {
		t.Fatalf("expected bounded head preview with complete material: %+v", loaded)
	}
	if loaded.material.SourcePath() != source || loaded.material.QueryPath() == source {
		t.Fatalf("binary preparation lost derivation: source=%q query=%q", loaded.material.SourcePath(), loaded.material.QueryPath())
	}
	index, err := tracequery.BuildIndex(context.Background(), loaded.material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Events) != 12 || index.Events[len(index.Events)-1].WakeePID != 424242 {
		t.Fatalf("full query lost binary tail event: %+v", index.Events)
	}
	bus := &types.BusContext{
		RepoRoot: dir, WorkDir: dir, AttachedHitrace: loaded.body,
		AttachedTraceMaterial: loaded.material, Mutable: types.NewMutableState("query the binary trace tail"),
	}
	agentContext := promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	projected := types.ToolBusContext(agentContext, types.AgentExplorer)
	if projected.AttachedTraceMaterial != loaded.material {
		t.Fatal("agent-to-tool projection dropped complete binary query material")
	}
	params := json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"tail-target","time_start":1.0105,"time_end":1.012,"limit":10}`)
	result, err := (&tool.TraceQuery{}).Execute(projected, params)
	if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-target") {
		t.Fatalf("production attached_trace query lost binary tail: %+v err=%v", result, err)
	}
}

// A real RMQ binary envelope with twelve physical pages. The unique final
// wakeup is intentionally far beyond the model preview in both source and
// converted text, without relying on an external trace-streamer executable.
func cliBinaryTraceTailFixture() []byte {
	var out bytes.Buffer
	header := make([]byte, 12)
	binary.LittleEndian.PutUint16(header[0:2], 0x0ace)
	header[2] = 1
	binary.LittleEndian.PutUint16(header[4:6], 1)
	binary.LittleEndian.PutUint32(header[8:12], 2)
	out.Write(header)
	segment := func(kind uint32, data []byte) {
		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[:4], kind)
		binary.LittleEndian.PutUint32(hdr[4:], uint32(len(data)))
		out.Write(hdr[:])
		out.Write(data)
	}
	format := "name: sched_wakeup\nID: 10\nformat:\n" +
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;\n" +
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;\n" +
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;\n" +
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;\n" +
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;\n" +
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;\n" +
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;\n" +
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;\n" +
		"print fmt: \"comm=%s pid=%d prio=%d target_cpu=%03d\"\n"
	segment(1, []byte(format))
	segment(2, []byte("12 worker\n"))
	segment(3, []byte("12 12\n"))
	var pages bytes.Buffer
	for i := 0; i < 12; i++ {
		page := make([]byte, 4096)
		binary.LittleEndian.PutUint64(page[0:8], uint64(1_000_000_000+i*1_000_000))
		binary.LittleEndian.PutUint64(page[8:16], 42)
		binary.LittleEndian.PutUint16(page[21:23], 36)
		payload := page[23:59]
		binary.LittleEndian.PutUint16(payload[:2], 10)
		binary.LittleEndian.PutUint32(payload[4:8], 12)
		name, pid := "head-worker", uint32(42)
		if i == 11 {
			name, pid = "tail-target", 424242
		}
		copy(payload[8:24], name)
		binary.LittleEndian.PutUint32(payload[24:28], pid)
		binary.LittleEndian.PutUint32(payload[28:32], 120)
		pages.Write(page)
	}
	segment(4, pages.Bytes())
	return out.Bytes()
}
