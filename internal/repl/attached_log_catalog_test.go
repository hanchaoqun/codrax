package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

type catalogAwareRunner struct {
	logAwareRunner
	catalog *loginput.Catalog
	seen    []*loginput.Catalog
}

func (r *catalogAwareRunner) AttachedLog() string                       { return r.curLog }
func (r *catalogAwareRunner) AttachedLogCatalog() *loginput.Catalog     { return r.catalog }
func (r *catalogAwareRunner) SetAttachedLogCatalog(c *loginput.Catalog) { r.catalog = c }
func (r *catalogAwareRunner) SetAttachedLog(body string) {
	r.logAwareRunner.SetAttachedLog(body)
	r.catalog = nil
}
func (r *catalogAwareRunner) Run(request, repo, branch string) (*types.BusContext, error) {
	r.seen = append(r.seen, r.catalog)
	return r.logAwareRunner.Run(request, repo, branch)
}

func newCatalogREPL(t *testing.T, input string) (*REPL, *catalogAwareRunner, *bytes.Buffer) {
	t.Helper()
	out, runner := &bytes.Buffer{}, &catalogAwareRunner{}
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Config{Runner: runner, Store: store, In: strings.NewReader(input), Out: out, Language: "en", Render: renderNothing, AttachedLogMaxBytes: 80, RuntimeAnchor: t.TempDir()})
	return r, runner, out
}

func TestPreparedLogREPLFileAppendDispatchAndClear(t *testing.T) {
	r, runner, out := newCatalogREPL(t, "")
	dir := t.TempDir()
	first, second := filepath.Join(dir, "one.log"), filepath.Join(dir, "two.log")
	if err := os.WriteFile(first, []byte(strings.Repeat("header line\n", 40)+"first-tail\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second-tail\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r.handleLogLoad(first)
	firstCatalog := r.attachedLogCatalog
	if firstCatalog == nil || strings.Contains(r.attachedLog, "first-tail") {
		t.Fatalf("missing bounded catalog: %s", out.String())
	}
	r.handleLogAppend(second)
	if r.attachedLogCatalog == firstCatalog || len(r.attachedLogCatalog.Sources()) != 2 {
		t.Fatal("append lost catalog")
	}
	result, err := r.attachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "tail"})
	if err != nil || result.Matched != 2 {
		t.Fatalf("tail=%+v %v", result, err)
	}
	r.dispatch("inspect attached logs", "inspect attached logs")
	if len(runner.seen) != 1 || runner.seen[0] != r.attachedLogCatalog {
		t.Fatalf("dispatch lost authority: %s", out.String())
	}
	r.handleLogCmd("/log clear")
	if r.attachedLog != "" || r.attachedLogCatalog != nil || runner.catalog != nil || runner.curLog != "" {
		t.Fatal("clear retained old authority")
	}
	r.handleLogLoad(first)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.prepareAttachedLogFileContext(ctx, second, false)
	if r.attachedLog != "" || r.attachedLogCatalog != nil || runner.catalog != nil {
		t.Fatal("canceled replacement borrowed old source")
	}
	r.handleLogLoad(first)
	r.handleLogLoad(filepath.Join(dir, "missing"))
	if r.attachedLog != "" || r.attachedLogCatalog != nil || runner.catalog != nil {
		t.Fatal("failed replacement borrowed old source")
	}
}

func TestPreparedLogREPLPasteBeyondPreviewKeepsInputOwner(t *testing.T) {
	payload := strings.Repeat("line\n", 100) + strings.Repeat("wide-", 100) + "paste-tail\n"
	r, _, out := newCatalogREPL(t, payload+"/end\nnext user command\n")
	r.handleLogPaste()
	if r.attachedLogCatalog == nil || len(r.attachedLog) > 80 {
		t.Fatalf("paste: %s", out.String())
	}
	result, err := r.attachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "paste-tail"})
	if err != nil || result.Matched != 1 {
		t.Fatalf("tail=%+v %v", result, err)
	}
	line, err := r.readInputLines("")
	if err != nil || line != "next user command" {
		t.Fatalf("input ownership: %q %v", line, err)
	}
}

func TestPreparedLogREPLSavedPreviewCannotRestoreOrImport(t *testing.T) {
	r, _, out := newCatalogREPL(t, "")
	if !r.prepareAttachedLogText(strings.Repeat("record\n", 50)+"tail\n", "paste") {
		t.Fatal(out.String())
	}
	snapshot := r.persistCurrentRuntimeArtifactSnapshot()
	if !snapshot.Log.LogRequiresReattach {
		t.Fatal("preview missing restore restriction")
	}
	if _, err := r.runtimeArtifactStore.Load(snapshot.Log, 80); err == nil {
		t.Fatal("preview recovered as fullsource")
	}
	if _, err := r.runtimeArtifactStore.LoadLatest(); err != nil {
		t.Fatal(err)
	}
	r.handleExportCmd("/export")
	dirs, err := filepath.Glob(filepath.Join(r.runtimeAnchor, "exports", "*-bundle"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("exports %v %v", dirs, err)
	}
	importer, runner, importOut := newCatalogREPL(t, "")
	importer.replaceAttachedLogText("existing")
	importer.handleImportCmd("/import " + dirs[0])
	if importer.attachedLog != "existing" || runner.curLog != "existing" || !strings.Contains(importOut.String(), "Reattach the original") {
		t.Fatalf("imported preview: %s", importOut.String())
	}
}

func TestPreparedLogREPLPlainReplacementAndCLISeed(t *testing.T) {
	r, runner, _ := newCatalogREPL(t, "")
	if !r.prepareAttachedLogText("original\n", "inline") {
		t.Fatal("prepare")
	}
	seeded := New(Config{Runner: runner, In: strings.NewReader(""), Out: &bytes.Buffer{}})
	if seeded.attachedLogCatalog != r.attachedLogCatalog {
		t.Fatal("CLI seed lost catalog")
	}
	r.replaceAttachedLogText("replacement")
	if r.attachedLogCatalog != nil || runner.catalog != nil || runner.curLog != "replacement" {
		t.Fatal("plain replacement inherited authority")
	}
}

func TestPreparedLogREPLLegacyPreviewAppendRequiresCompleteInput(t *testing.T) {
	full := "first-record\n" + strings.Repeat("background\n", 100) + "missing-tail\n"
	original, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "original", Data: []byte(full)}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(t.TempDir(), "next.log")
	if err := os.WriteFile(next, []byte("next-source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, preview := range []string{original.Preview(32), "ordinary text without any truncation marker\n"} {
		runner, out := &catalogAwareRunner{}, &bytes.Buffer{}
		runner.SetAttachedLog(preview) // Public host setter has no complete-source receipt.
		r := New(Config{Runner: runner, In: strings.NewReader(""), Out: out, Language: "en"})
		r.handleLogCmd("/log append " + next)
		if r.attachedLogCatalog != nil || runner.catalog != nil || r.attachedLog != preview || runner.curLog != preview {
			t.Fatalf("unproven preview was promoted or mutated: %s", out.String())
		}
		if !strings.Contains(out.String(), "Reattach the original") || !strings.Contains(out.String(), "preview is unchanged") {
			t.Fatalf("append refusal did not explain recovery: %s", out.String())
		}
		if !r.prepareAttachedLogText(full, "complete-paste") {
			t.Fatal(out.String())
		}
		r.handleLogCmd("/log append " + next)
		if r.attachedLogCatalog == nil || len(r.attachedLogCatalog.Sources()) != 2 {
			t.Fatalf("fresh complete input did not recover append: %s", out.String())
		}
		result, err := r.attachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "missing-tail"})
		if err != nil || result.Matched != 1 || !result.Complete {
			t.Fatalf("full original input not retained after reattachment: %+v %v", result, err)
		}
	}
}
