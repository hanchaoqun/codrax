package outputdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func withExplicitReport(t *testing.T, r ExplicitReport) {
	t.Helper()
	SetExplicitReport(r)
	t.Cleanup(func() { SetExplicitReport(ExplicitReport{}) })
}

func explicitReportArgs(dir string) Args {
	return Args{
		Dir:     dir,
		Max:     10,
		Request: "why does the loop stall?",
		Answer:  "# 结论\n\nbecause of the mutex.\n",
		Now:     time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC),
		PID:     4242,
	}
}

func readFileOrFatal(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// Pins ②④⑥: with the default dump enabled, explicit targets write
// ADDITIONAL copies — the default dir keeps its canonical .md/.html pair,
// the explicit markdown is byte-identical to the default dump markdown,
// the explicit HTML comes from the same BuildHTML pipeline over the same
// markdown bytes, and missing parent directories are created.
func TestExplicitReportWritesAlongsideDefaultDump(t *testing.T) {
	tmp := t.TempDir()
	defaultDir := filepath.Join(tmp, "default")
	mdPath := filepath.Join(tmp, "extra", "nested", "answer-report.md")
	htmlPath := filepath.Join(tmp, "extra", "deeper", "answer-report.html")
	withExplicitReport(t, ExplicitReport{MarkdownPath: mdPath, HTMLPath: htmlPath})

	result := WriteResult(explicitReportArgs(defaultDir))
	if result.MarkdownPath == "" || filepath.Dir(result.MarkdownPath) != defaultDir {
		t.Fatalf("default dump markdown missing or misplaced: %+v", result)
	}
	if result.HTMLPath == "" || filepath.Dir(result.HTMLPath) != defaultDir {
		t.Fatalf("default dump html missing or misplaced: %+v", result)
	}

	defaultMD := readFileOrFatal(t, result.MarkdownPath)
	explicitMD := readFileOrFatal(t, mdPath)
	if string(defaultMD) != string(explicitMD) {
		t.Fatalf("explicit markdown is not byte-identical to the default dump markdown")
	}

	explicitHTML := readFileOrFatal(t, htmlPath)
	wantHTML, err := BuildHTML(filepath.Base(htmlPath), string(defaultMD))
	if err != nil {
		t.Fatalf("BuildHTML: %v", err)
	}
	if string(explicitHTML) != wantHTML {
		t.Fatalf("explicit html does not match the shared BuildHTML product over the default dump markdown bytes")
	}

	writes := ExplicitReportWrites()
	if len(writes) != 2 {
		t.Fatalf("expected 2 recorded writes, got %d: %+v", len(writes), writes)
	}
	for _, w := range writes {
		if w.Err != nil {
			t.Fatalf("unexpected write error for %s %s: %v", w.Kind, w.Path, w.Err)
		}
	}
}

// Pin ①: the two targets are independent — markdown-only writes only the
// markdown copy, html-only writes only the html copy.
func TestExplicitReportMarkdownOnly(t *testing.T) {
	tmp := t.TempDir()
	mdPath := filepath.Join(tmp, "only.md")
	withExplicitReport(t, ExplicitReport{MarkdownPath: mdPath})

	WriteResult(explicitReportArgs(filepath.Join(tmp, "default")))
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("markdown copy missing: %v", err)
	}
	writes := ExplicitReportWrites()
	if len(writes) != 1 || writes[0].Kind != "markdown" {
		t.Fatalf("expected exactly one markdown write, got %+v", writes)
	}
}

func TestExplicitReportHTMLOnly(t *testing.T) {
	tmp := t.TempDir()
	htmlPath := filepath.Join(tmp, "only.html")
	withExplicitReport(t, ExplicitReport{HTMLPath: htmlPath})

	WriteResult(explicitReportArgs(filepath.Join(tmp, "default")))
	if _, err := os.Stat(htmlPath); err != nil {
		t.Fatalf("html copy missing: %v", err)
	}
	writes := ExplicitReportWrites()
	if len(writes) != 1 || writes[0].Kind != "html" {
		t.Fatalf("expected exactly one html write, got %+v", writes)
	}
}

// Pin ③: SuppressDefaultDir (output_dump_enabled=false + explicit CLI
// paths) writes ONLY the explicit copies — the default dir is never even
// created — and the returned Result stays empty so callers keep treating
// the default dump as disabled.
func TestExplicitReportSuppressDefaultDirWritesOnlyExplicit(t *testing.T) {
	tmp := t.TempDir()
	defaultDir := filepath.Join(tmp, "default")
	mdPath := filepath.Join(tmp, "report.md")
	htmlPath := filepath.Join(tmp, "report.html")
	withExplicitReport(t, ExplicitReport{
		MarkdownPath:       mdPath,
		HTMLPath:           htmlPath,
		SuppressDefaultDir: true,
	})

	result := WriteResult(explicitReportArgs(defaultDir))
	if result.MarkdownPath != "" || result.HTMLPath != "" {
		t.Fatalf("suppressed default dump must return an empty Result, got %+v", result)
	}
	if _, err := os.Stat(defaultDir); !os.IsNotExist(err) {
		t.Fatalf("default dir must stay untouched when suppressed, stat err=%v", err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("explicit markdown missing: %v", err)
	}
	if _, err := os.Stat(htmlPath); err != nil {
		t.Fatalf("explicit html missing: %v", err)
	}
	// Same content source even without a default dump on disk: the
	// markdown copy must equal the composed BuildBody product.
	got := readFileOrFatal(t, mdPath)
	want := BuildBody(explicitReportArgs(defaultDir))
	if string(got) != want {
		t.Fatalf("explicit markdown diverged from BuildBody product")
	}
}

// Pin ⑤: an existing file at the explicit path is overwritten — the
// user-supplied path is explicit intent, no timestamp dance.
func TestExplicitReportOverwritesExistingFile(t *testing.T) {
	tmp := t.TempDir()
	mdPath := filepath.Join(tmp, "report.md")
	if err := os.WriteFile(mdPath, []byte("stale junk"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	withExplicitReport(t, ExplicitReport{MarkdownPath: mdPath, SuppressDefaultDir: true})

	WriteResult(explicitReportArgs(filepath.Join(tmp, "default")))
	got := string(readFileOrFatal(t, mdPath))
	if strings.Contains(got, "stale junk") {
		t.Fatalf("existing file was not overwritten:\n%s", got)
	}
	if !strings.Contains(got, "because of the mutex.") {
		t.Fatalf("overwritten file lacks the answer body:\n%s", got)
	}
}

// Pin ⑦: an unwritable explicit path (parent is a regular file) fails
// loud into the recorded status without blocking the default dump or the
// other explicit target.
func TestExplicitReportBadPathDoesNotBlockDefaultOrSibling(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("i am a file"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	badMD := filepath.Join(blocker, "report.md") // parent is a file → MkdirAll fails
	goodHTML := filepath.Join(tmp, "report.html")
	withExplicitReport(t, ExplicitReport{MarkdownPath: badMD, HTMLPath: goodHTML})

	defaultDir := filepath.Join(tmp, "default")
	result := WriteResult(explicitReportArgs(defaultDir))
	if result.MarkdownPath == "" {
		t.Fatalf("default dump must still write when an explicit path fails")
	}
	if _, err := os.Stat(goodHTML); err != nil {
		t.Fatalf("sibling html target must still write when the markdown target fails: %v", err)
	}
	writes := ExplicitReportWrites()
	if len(writes) != 2 {
		t.Fatalf("expected 2 recorded writes, got %+v", writes)
	}
	var sawMDErr, sawHTMLOK bool
	for _, w := range writes {
		switch w.Kind {
		case "markdown":
			sawMDErr = w.Err != nil && w.Path == badMD
		case "html":
			sawHTMLOK = w.Err == nil && w.Path == goodHTML
		}
	}
	if !sawMDErr || !sawHTMLOK {
		t.Fatalf("status must carry the failed markdown path and the successful html path: %+v", writes)
	}
}

// Regression guard: with no explicit sink registered, WriteResult keeps
// its historical contract — empty Dir writes nothing and returns the
// zero Result.
func TestWriteResultWithoutExplicitSinkKeepsEmptyDirNoop(t *testing.T) {
	withExplicitReport(t, ExplicitReport{})
	a := explicitReportArgs("")
	if got := WriteResult(a); got.MarkdownPath != "" || got.HTMLPath != "" {
		t.Fatalf("empty Dir without explicit sink must be a no-op, got %+v", got)
	}
	if writes := ExplicitReportWrites(); len(writes) != 0 {
		t.Fatalf("no writes should be recorded without targets, got %+v", writes)
	}
}

func TestExplicitRootCauseArtifactAvailable(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "api", "root-causes.json")
	impact := 0.005
	report := &types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses: []*types.TraceRootCauseItemV2{{
			Rank: 1, Category: types.TraceRootCausePriorityInversion,
			ThreadName: "worker-7", ImpactSeconds: &impact,
			Summary: "worker-7线程优先级反转", Evidence: []string{"typed evidence"},
		}},
	}
	withExplicitReport(t, ExplicitReport{RootCauseJSONPath: rootPath, SuppressDefaultDir: true})
	a := explicitReportArgs(filepath.Join(tmp, "default"))
	a.RootCauseReport = report
	result := WriteResult(a)
	if result.RootCauseJSONPath != rootPath {
		t.Fatalf("explicit root-cause path must be returned for programmatic discovery: %+v", result)
	}

	var got ExplicitRootCauseArtifact
	if err := json.Unmarshal(readFileOrFatal(t, rootPath), &got); err != nil {
		t.Fatalf("decode guaranteed artifact: %v", err)
	}
	if got.ArtifactSchemaVersion != ExplicitRootCauseArtifactSchemaVersion ||
		got.Status != ExplicitRootCauseStatusAvailable || got.ReasonCode != "" ||
		got.TraceRootCauses == nil || len(got.TraceRootCauses.RootCauses) != 1 {
		t.Fatalf("available artifact mismatch: %+v", got)
	}
}

func TestExplicitRootCauseArtifactUnavailableIsNotEmptyConclusion(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "root-causes.json")
	withExplicitReport(t, ExplicitReport{RootCauseJSONPath: rootPath, SuppressDefaultDir: true})
	a := explicitReportArgs(filepath.Join(tmp, "default"))
	a.RootCauseUnavailableReason = "no_selectable_typed_on_chain_candidates"
	WriteResult(a)

	var got ExplicitRootCauseArtifact
	if err := json.Unmarshal(readFileOrFatal(t, rootPath), &got); err != nil {
		t.Fatalf("decode guaranteed artifact: %v", err)
	}
	if got.Status != ExplicitRootCauseStatusUnavailable ||
		got.ReasonCode != "no_selectable_typed_on_chain_candidates" || got.TraceRootCauses != nil {
		t.Fatalf("unavailable artifact must not mint an empty model conclusion: %+v", got)
	}
}

func TestEnsureExplicitRootCauseArtifactClosesNoFinalAnswerLane(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "root-causes.json")
	withExplicitReport(t, ExplicitReport{RootCauseJSONPath: rootPath, SuppressDefaultDir: true})

	EnsureExplicitRootCauseArtifact("final_answer_transcript_not_available")
	var got ExplicitRootCauseArtifact
	if err := json.Unmarshal(readFileOrFatal(t, rootPath), &got); err != nil {
		t.Fatalf("decode guaranteed artifact: %v", err)
	}
	if got.Status != ExplicitRootCauseStatusUnavailable || got.ReasonCode != "final_answer_transcript_not_available" {
		t.Fatalf("no-answer artifact mismatch: %+v", got)
	}
	writes := ExplicitReportWrites()
	if len(writes) != 1 || writes[0].Kind != "root-causes" || writes[0].Err != nil {
		t.Fatalf("expected one successful guaranteed write, got %+v", writes)
	}
}
