package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRepoLanguageCensus_OutOfScopeHint pins the RNE-C53 corner fix: when a
// source-inventory lens ran with narrow (grep-derived) scopes that contain no
// files of a language the repo actually carries, the census must surface that
// language AND the exact files to open, while a language that DOES have files
// in scope (InScope) must NOT be flagged — otherwise a repo-dominant language
// floods the hint and buries the real target.
func TestRepoLanguageCensus_OutOfScopeHint(t *testing.T) {
	obs := types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{
			"internal/tool/repomap/index/cangjie_parser.go",
			"internal/tool/repomap/index/extract_cangjie.go",
		},
		RepoLanguages: []types.SourceInventoryLanguageCount{
			// go has a file in scope -> InScope true -> must NOT be flagged.
			{Language: "go", Count: 1766, InScope: true, Samples: []string{"cmd/main_test.go"}},
			// cangjie has no file in scope -> flagged with openable paths.
			{Language: "cangjie", Count: 8, InScope: false, Samples: []string{
				"internal/thirdparty/tree-sitter-cangjie/corpus/sources/01_package_main.cj",
				"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
			}},
		},
	}

	out := renderSourceInventoryRepoLanguageCensus(obs)
	if !strings.Contains(out, "repo_languages") || !strings.Contains(out, "cangjie:8") {
		t.Fatalf("census line missing:\n%s", out)
	}
	if strings.Contains(out, "  - go ") {
		t.Fatalf("in-scope language go must not be flagged:\n%s", out)
	}
	if !strings.Contains(out, "  - cangjie (8 file(s))") {
		t.Fatalf("out-of-scope cangjie hint missing:\n%s", out)
	}
	if !strings.Contains(out, "corpus/sources/01_package_main.cj") {
		t.Fatalf("cangjie sample path missing from hint:\n%s", out)
	}
}

// TestRepoLanguageCensus_AllInScopeNoHint confirms that when every language has
// files in scope (e.g. a whole-repo lens), the census is informational only and
// no out-of-scope hint is emitted.
func TestRepoLanguageCensus_AllInScopeNoHint(t *testing.T) {
	obs := types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"."},
		RepoLanguages: []types.SourceInventoryLanguageCount{
			{Language: "go", Count: 1766, InScope: true, Samples: nil},
			{Language: "cangjie", Count: 8, InScope: true, Samples: nil},
		},
	}
	out := renderSourceInventoryRepoLanguageCensus(obs)
	if !strings.Contains(out, "repo_languages") || !strings.Contains(out, "cangjie:8") {
		t.Fatalf("census line missing:\n%s", out)
	}
	if strings.Contains(out, "open these files before concluding absence") {
		t.Fatalf("all-in-scope census must not emit out-of-scope hint:\n%s", out)
	}
}

// TestRepoLanguageCensus_Empty confirms the census renders nothing when there
// is no repo-wide language data, so it can never make an empty observation look
// active.
func TestRepoLanguageCensus_Empty(t *testing.T) {
	if out := renderSourceInventoryRepoLanguageCensus(types.SourceInventoryObservation{Active: true}); out != "" {
		t.Fatalf("expected empty render, got:\n%s", out)
	}
}

// TestRepoLanguageCensus_BuilderInScope pins the precise per-language in-scope
// computation against the real repo: with a Go-file scope, go must read as
// InScope (so it is never flagged) while cangjie must read as scope-absent with
// real .cj sample paths to open.
func TestRepoLanguageCensus_BuilderInScope(t *testing.T) {
	ctx := &types.BusContext{RepoRoot: "../.."}
	wantCJ := sourceInventoryTrackedLanguageCount(t, ctx.RepoRoot, repotypes.LangCangjie)
	if wantCJ == 0 {
		t.Fatal("expected repo fixture to carry tracked Cangjie source files")
	}
	census := sourceInventoryRepoLanguageCensus(ctx, []string{
		"internal/tool/repomap/index/cangjie_parser.go",
	})
	var goLC, cjLC *types.SourceInventoryLanguageCount
	for i := range census {
		switch census[i].Language {
		case "go":
			goLC = &census[i]
		case "cangjie":
			cjLC = &census[i]
		}
	}
	if goLC == nil || cjLC == nil {
		t.Fatalf("expected go and cangjie in census, got %+v", census)
	}
	if !goLC.InScope {
		t.Fatalf("go should be InScope when a go file is in scope: %+v", goLC)
	}
	if cjLC.InScope {
		t.Fatalf("cangjie should be scope-absent under a go-only scope: %+v", cjLC)
	}
	if cjLC.Count != wantCJ {
		t.Fatalf("expected cangjie count %d from tracked source universe, got %d", wantCJ, cjLC.Count)
	}
	if len(cjLC.Samples) == 0 || !strings.HasSuffix(cjLC.Samples[0], ".cj") {
		t.Fatalf("expected .cj sample paths for cangjie, got %v", cjLC.Samples)
	}
}

func sourceInventoryTrackedLanguageCount(t *testing.T, repoRoot, lang string) int {
	t.Helper()
	files, complete := sourceInventoryTrackedRepoFiles(repoRoot)
	if !complete {
		t.Fatalf("expected complete tracked source universe for %s", repoRoot)
	}
	count := 0
	for _, rel := range files {
		if repotypes.DetectLanguage(rel) == lang {
			count++
		}
	}
	return count
}

// TestRepoLanguageCountsCloneMergeRoundTrip exercises the typed clone/merge so
// the census survives MutableState round-trips without losing samples or the
// InScope flag.
func TestRepoLanguageCountsCloneMergeRoundTrip(t *testing.T) {
	prior := types.SourceInventoryObservation{
		Active:        true,
		SourceClasses: []types.SourceInventorySourceClassCount{{Role: types.SourcePathRoleProduction, Count: 1}},
		RepoLanguages: []types.SourceInventoryLanguageCount{
			{Language: "cangjie", Count: 8, InScope: false, Samples: []string{"a.cj"}},
		},
	}
	current := types.SourceInventoryObservation{
		Active:        true,
		SourceClasses: []types.SourceInventorySourceClassCount{{Role: types.SourcePathRoleProduction, Count: 1}},
		RepoLanguages: []types.SourceInventoryLanguageCount{
			{Language: "cangjie", Count: 8, InScope: true, Samples: []string{"b.cj"}},
			{Language: "arkts", Count: 6, InScope: false, Samples: []string{"c.ets"}},
		},
	}
	merged := types.MergeSourceInventoryObservation(prior, current)
	got := map[string]types.SourceInventoryLanguageCount{}
	for _, lc := range merged.RepoLanguages {
		got[lc.Language] = lc
	}
	if got["cangjie"].Count != 8 || got["arkts"].Count != 6 {
		t.Fatalf("merged counts wrong: %#v", got)
	}
	if !got["cangjie"].InScope {
		t.Fatalf("InScope should OR to true across calls, got %+v", got["cangjie"])
	}
	if len(got["cangjie"].Samples) != 2 {
		t.Fatalf("expected unioned cangjie samples a.cj+b.cj, got %v", got["cangjie"].Samples)
	}
}
