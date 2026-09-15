package dataflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real parser output is the only semantic input here. Source code is not
// accompanied by hand-authored return rows or callable ownership metadata.
func b1692ParsedReturnFile(t *testing.T, path, source string) (string, *repomap.FileInfo, *repomap.Graph) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := repomap.ScanFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	graph := repomap.BuildGraph(root, repomap.ParseFiles(entries, root))
	file := graph.FileIndex[path]
	if file == nil || len(file.Symbols) == 0 {
		t.Fatal("real parser did not produce the callable prerequisite")
	}
	return root, file, graph
}

func TestB1692DataflowReturnCacheBindsSourceAndParserFacts(t *testing.T) {
	const source = "package sample\nfunc value() string {\n// body\nreturn \"ready\"\n}\n"
	root, file, _ := b1692ParsedReturnFile(t, "sample.go", source)
	lines := strings.Split(source, "\n")
	opts := Options{RepoRoot: root, WorkDir: t.TempDir(), MaxNodesPerFunc: 100}
	lowerer := genericLowerer{lang: file.Language}
	// The previous generation deliberately contains an invalid return. It
	// must remain untouched and must not be consumed after the version change.
	oldHash := sha256.Sum256([]byte(file.RelPath + ":" + file.Hash + ":v2"))
	oldPath := filepath.Join(opts.WorkDir, "dataflow-cache", "lowered", hex.EncodeToString(oldHash[:8])+".json")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o700); err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal(LoweredFile{File: file.RelPath, Hash: file.Hash, Summaries: []FunctionSummary{{SymbolName: "value", Returns: []string{"poison"}}}})
	if err := os.WriteFile(oldPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	cold := loadOrLowerFile(opts, file, lines, lowerer)
	if len(cold.Summaries) != 1 || !contains(cold.Summaries[0].Returns, `"ready"`) || contains(cold.Summaries[0].Returns, "poison") {
		t.Fatalf("cold parser return prerequisite failed: %+v", cold.Summaries)
	}
	warm := loadOrLowerFile(opts, file, lines, lowerer)
	if !bytes.Equal(marshalJSON(cold), marshalJSON(warm)) {
		t.Fatal("warm return facts differ from cold lowering")
	}
	withoutReceipts := *file
	withoutReceipts.CallableReturnExpressions = nil
	for _, tc := range []struct {
		name  string
		file  *repomap.FileInfo
		lines []string
		opts  Options
	}{
		{"same_hash_missing_receipts", &withoutReceipts, lines, opts},
		{"source_changed_after_parse", file, strings.Split(strings.ReplaceAll(source, "ready", "other"), "\n"), opts},
		{"bounded_before_return", file, lines, Options{RepoRoot: root, WorkDir: opts.WorkDir, MaxNodesPerFunc: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := loadOrLowerFile(tc.opts, tc.file, tc.lines, lowerer)
			for _, summary := range got.Summaries {
				if len(summary.Returns) != 0 || len(summary.Literals) != 0 {
					t.Fatalf("cache revived out-of-scope return authority: %+v", summary)
				}
			}
		})
	}
	if got := loadOrLowerFile(opts, file, lines, lowerer); !bytes.Equal(marshalJSON(cold), marshalJSON(got)) {
		t.Fatal("limited generation polluted the original full cache")
	}
	if after, err := os.ReadFile(oldPath); err != nil || string(after) != string(old) {
		t.Fatal("retired cache was overwritten instead of bypassed")
	}
}

type b1692CancellingLowerer struct{ cancel context.CancelFunc }

func (b1692CancellingLowerer) Language() string { return repomap.LangGo }
func (l b1692CancellingLowerer) LowerFile(_ string, file *repomap.FileInfo, _ []string, _ Options) LoweredFile {
	l.cancel()
	return LoweredFile{File: file.RelPath, Hash: file.Hash}
}

func TestB1692CancelledLoweringDoesNotCachePartialFacts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	file := &repomap.FileInfo{RelPath: "sample.go", Language: repomap.LangGo, Hash: "generation"}
	opts := Options{Context: ctx, WorkDir: t.TempDir()}
	lines := []string{"package sample"}
	loadOrLowerFile(opts, file, lines, b1692CancellingLowerer{cancel: cancel})
	if _, err := os.Stat(loweredCacheFile(opts, file, lines)); !os.IsNotExist(err) {
		t.Fatalf("cancelled partial lowering was persisted: %v", err)
	}
}

func TestB1692DataflowRejectsPartiallyReadReturnExpression(t *testing.T) {
	const source = "package sample\nfunc value() string {\nreturn build(\n\"ready\",\n)\n}\n"
	root, file, _ := b1692ParsedReturnFile(t, "sample.go", source)
	lines := strings.Split(source, "\n")
	full := (genericLowerer{lang: file.Language}).LowerFile(root, file, lines, Options{MaxNodesPerFunc: 100})
	if len(full.Summaries) != 1 || !contains(full.Summaries[0].Returns, "build(\n\"ready\",\n)") {
		t.Fatalf("complete multi-line return prerequisite missing: %+v", full.Summaries)
	}
	// The opening return line is included, but its closing expression is not.
	partial := (genericLowerer{lang: file.Language}).LowerFile(root, file, lines, Options{MaxNodesPerFunc: 1})
	if len(partial.Summaries) != 1 || len(partial.Summaries[0].Returns) != 0 {
		t.Fatalf("partial expression received full return authority: %+v", partial.Summaries)
	}
}

func TestB1692DataflowReturnsStayWithParserCallable(t *testing.T) {
	for _, tc := range []struct {
		name, path, source, owner, want string
		forbidden                       []string
	}{
		{"rust_discarded_match", "sample.rs", `fn inspect(flag: bool) -> bool {
    let label = match flag {
        true => "inner",
        false => "other",
    };
    true
}
`, "inspect", "true", []string{`"inner",`, `"other",`, `"inner"`, `"other"`}},
		{"javascript_nested_arrow", "sample.js", `function outer() {
    const inner = () => "inner";
    return "outer";
}
`, "outer", `"outer"`, []string{`"inner"`}},
		{"go_explicit_return", "sample.go", "package sample\nfunc value() string {\nreturn \"ready\"\n}\n", "value", `"ready"`, nil},
		{"go_crlf_return", "sample.go", "package sample\r\nfunc value() string {\r\nreturn \"ready\"\r\n}\r\n", "value", `"ready"`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, file, graph := b1692ParsedReturnFile(t, tc.path, tc.source)
			lowered := (genericLowerer{lang: file.Language}).LowerFile(root, file, strings.Split(tc.source, "\n"), Options{MaxNodesPerFunc: 100})
			foundOwner := false
			for _, summary := range lowered.Summaries {
				if summary.SymbolName != tc.owner {
					continue
				}
				foundOwner = true
				for _, invalid := range tc.forbidden {
					if contains(summary.Returns, invalid) || contains(summary.Literals, invalid) {
						t.Errorf("non-return/other callable value became proved summary of %s: %+v", tc.owner, summary)
					}
				}
				if !contains(summary.Returns, tc.want) {
					t.Errorf("actual return missing from %s: got %q want %q", tc.owner, summary.Returns, tc.want)
				}
			}
			if !foundOwner {
				t.Fatal("real parser/lowerer did not retain target callable")
			}
			result := Analyze(graph, Options{RepoRoot: root, CandidateFiles: []string{tc.path}, WorkDir: t.TempDir()})
			for _, row := range result.Evidence {
				if row.Predicate != "returns" || !strings.Contains(row.Subject, tc.owner) || types.EvidenceIsDerivationCandidate(row) {
					continue
				}
				for _, invalid := range tc.forbidden {
					if row.Object == invalid {
						t.Errorf("public dataflow published non-return as independent evidence: %+v", row)
					}
				}
			}
		})
	}
}
