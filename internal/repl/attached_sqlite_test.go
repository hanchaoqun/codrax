package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestREPLExistingSQLiteLoadAndFailureKeepExactMaterial(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_existing_sqlite/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capture.data")
	if err := os.WriteFile(path, body, 0o400); err != nil {
		t.Fatal(err)
	}
	runner := &materialAwareRunner{}
	out := &bytes.Buffer{}
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Config{Runner: runner, Store: store, RuntimeAnchor: t.TempDir(), RepoRoot: t.TempDir(), Out: out, Language: "en", Render: renderNothing})
	r.handleHitraceCmd("/htrace " + path)
	m := r.attachedTraceMaterial
	if m == nil || runner.material != m || m.SourcePath() != path {
		t.Fatalf("SQLite not published by real REPL path: %s", out.String())
	}
	idx, err := tracequery.BuildIndex(t.Context(), m.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "tail-business-marker", Limit: 10})
	if len(result.Events) != 1 {
		t.Fatalf("REPL query lost tail: %+v", result)
	}
	r.dispatch("inspect attached trace", "inspect attached trace")
	if len(runner.seenMaterials) != 1 || runner.seenMaterials[0] != m {
		t.Fatal("dispatch lost SQLite material")
	}
	if err := os.WriteFile(path+"-journal", []byte("active database"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.handleHitraceCmd("/htrace " + path)
	if r.attachedTraceMaterial != m || runner.material != m {
		t.Fatal("failed SQLite load overwrote previous attachment")
	}
	r.dispatch("inspect failed new input", "inspect failed new input")
	if len(runner.seenMaterials) != 1 {
		t.Fatal("failed reload silently answered with old attachment")
	}
}
