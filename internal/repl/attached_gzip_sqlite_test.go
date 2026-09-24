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

func TestREPLGzipSQLiteLoadAndBadReloadKeepsExactMaterial(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_gzip_sqlite/capture.transport")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "compressed 数据.capture")
	if err := os.WriteFile(path, body, 0o400); err != nil {
		t.Fatal(err)
	}
	runner, out := &materialAwareRunner{}, &bytes.Buffer{}
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Config{Runner: runner, Store: store, RuntimeAnchor: t.TempDir(), RepoRoot: t.TempDir(), Out: out, Language: "en", Render: renderNothing})
	r.handleSlash(types.NormalizeREPLCommandAlias("/atrace " + path))
	m := r.attachedTraceMaterial
	if m == nil || runner.material != m || m.SourcePath() != path {
		t.Fatalf("gzip SQLite not published by REPL: %s", out.String())
	}
	idx, err := tracequery.BuildIndex(t.Context(), m.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "tail-business-marker", TimeStart: 2.98, TimeEnd: 3, Limit: 10})
	if len(result.Events) != 1 || result.TimeStart != 2.98 || result.TimeEnd != 3 {
		t.Fatalf("REPL lost full-file query/window: %+v", result)
	}
	r.dispatch("inspect attached trace", "inspect attached trace")
	if len(runner.seenMaterials) != 1 || runner.seenMaterials[0] != m {
		t.Fatal("dispatch lost compressed material")
	}
	bad := append([]byte(nil), body...)
	bad[len(bad)-8] ^= 1 // The envelope still sniffs as gzip; its CRC is invalid.
	badPath := filepath.Join(t.TempDir(), "bad.capture")
	if err := os.WriteFile(badPath, bad, 0o400); err != nil {
		t.Fatal(err)
	}
	r.handleSlash(types.NormalizeREPLCommandAlias("/atrace " + badPath))
	if r.attachedTraceMaterial != m || runner.material != m {
		t.Fatal("failed compressed load overwrote previous attachment")
	}
	r.dispatch("inspect failed new input", "inspect failed new input")
	if len(runner.seenMaterials) != 1 {
		t.Fatal("failed reload silently answered with old attachment")
	}
}
