package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// scratchRepo creates a tmp dir layout the test can stat against.
func scratchRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

// fsHitPaths returns just the paths from a FsHit slice for
// equality checks where MentionCount isn't under test.
func fsHitPaths(hits []FsHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}

func TestMentionedFileEntities_DirectStat(t *testing.T) {
	root := scratchRepo(t, map[string]string{
		"Dockerfile":       "FROM scratch\n",
		"src/main.go":      "package main\n",
		"data/users.proto": "syntax = \"proto3\";\n",
	})
	got := fsHitPaths(mentionedFileEntities(root, []string{"Dockerfile", "data/users.proto"}, nil))
	want := []string{"Dockerfile", "data/users.proto"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_SiblingSuffix(t *testing.T) {
	// Repo has codrax.yaml.example but NOT codrax.yaml — the
	// canonical-form mention must still find the example via the
	// sibling-suffix probe. This is the s3a regression case.
	root := scratchRepo(t, map[string]string{
		"codrax.yaml.example": "# example\n",
		"unrelated.go":        "package main\n",
	})
	got := fsHitPaths(mentionedFileEntities(root, []string{"codrax.yaml"}, nil))
	want := []string{"codrax.yaml.example"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_AllDefaultSuffixes(t *testing.T) {
	// Drop one canonical + each default suffix variant; helper must
	// surface every existing variant.
	root := scratchRepo(t, map[string]string{
		"app.conf":         "ok",
		"app.conf.sample":  "ok",
		"app.conf.dist":    "ok",
		"app.conf.tmpl":    "ok",
		"app.conf.tpl":     "ok",
		"app.conf.example": "ok",
	})
	got := fsHitPaths(mentionedFileEntities(root, []string{"app.conf"}, nil))
	want := []string{
		"app.conf",
		"app.conf.example",
		"app.conf.sample",
		"app.conf.dist",
		"app.conf.tmpl",
		"app.conf.tpl",
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_SiblingSuffixOverride(t *testing.T) {
	// Operator overrides the default list with `.in` (autoconf
	// convention). The default suffixes must NOT fire after override.
	original := CurrentMentionedFileSiblingSuffixes()
	t.Cleanup(func() { SetMentionedFileSiblingSuffixes(original) })
	SetMentionedFileSiblingSuffixes([]string{".in"})

	root := scratchRepo(t, map[string]string{
		"config.h.in":      "ok",
		"config.h.example": "should not match",
	})
	got := fsHitPaths(mentionedFileEntities(root, []string{"config.h"}, nil))
	want := []string{"config.h.in"} // .example is no longer a probed suffix
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_FileIndexBasenameMatch(t *testing.T) {
	// User says bare basename "Dockerfile"; the repo has
	// services/api/Dockerfile (deep). os.Stat at repoRoot misses;
	// FileIndex catches.
	root := scratchRepo(t, map[string]string{
		"services/api/Dockerfile": "FROM scratch\n",
	})
	graph := &types.Graph{
		FileIndex: map[string]*types.FileInfo{
			"services/api/Dockerfile": {RelPath: "services/api/Dockerfile"},
			"src/main.go":             {RelPath: "src/main.go"},
		},
	}
	got := fsHitPaths(mentionedFileEntities(root, []string{"Dockerfile"}, graph))
	want := []string{"services/api/Dockerfile"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_NonExistentDropped(t *testing.T) {
	// "config.toml" is mentioned but does not exist in the repo.
	// No layer hits → empty result. Zero false-positive guarantee.
	root := scratchRepo(t, map[string]string{
		"unrelated.go": "package main\n",
	})
	got := mentionedFileEntities(root, []string{"config.toml"}, nil)
	if len(got) != 0 {
		t.Errorf("non-existent file must be dropped; got %v", got)
	}
}

func TestMentionedFileEntities_AbsolutePathRejected(t *testing.T) {
	// A "mentioned entity" that is an absolute path or has ../
	// must be silently dropped — no file outside the repo can be
	// seeded into RequiredFiles even if filesystem access succeeds.
	root := scratchRepo(t, map[string]string{
		"safe.yaml": "ok",
	})
	got := fsHitPaths(mentionedFileEntities(root, []string{"/etc/passwd", "../escape.txt", "safe.yaml"}, nil))
	want := []string{"safe.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMentionedFileEntities_EmptyInputsAreNoop(t *testing.T) {
	if got := mentionedFileEntities("", []string{"x"}, nil); got != nil {
		t.Errorf("empty repo root must return nil; got %v", got)
	}
	if got := mentionedFileEntities("/tmp", nil, nil); got != nil {
		t.Errorf("empty mentioned must return nil; got %v", got)
	}
}

// ── Layer D — MentionCount + tier-split priority ──

func TestMentionedFileEntities_MentionCountSortsCriticalFirst(t *testing.T) {
	// Three files: critical.conf is mentioned in 4 other files;
	// noise.conf in 0 (zero references besides itself).
	// MentionCount sort puts critical.conf first.
	root := scratchRepo(t, map[string]string{
		"critical.conf":   "k1=v\n",
		"noise.conf":      "k2=v\n",
		"src/a.go":        "// see critical.conf for details\npackage a\n",
		"src/b.go":        "// reads critical.conf\npackage b\n",
		"docs/intro.md":   "Refer to critical.conf in the runtime config layer.\n",
		"tests/c_test.go": "// fixture: critical.conf\npackage c\n",
	})
	hits := mentionedFileEntities(root, []string{"critical.conf", "noise.conf"}, nil)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d (%+v)", len(hits), hits)
	}
	if hits[0].Path != "critical.conf" {
		t.Errorf("critical.conf must sort first by MentionCount; got %s first", hits[0].Path)
	}
	if hits[0].MentionCount <= hits[1].MentionCount {
		t.Errorf("critical.conf MentionCount=%d must exceed noise.conf MentionCount=%d",
			hits[0].MentionCount, hits[1].MentionCount)
	}
}

func TestSetRequiredFileMentionCountFloor_DefaultAndOverride(t *testing.T) {
	original := CurrentRequiredFileMentionCountFloor()
	t.Cleanup(func() { SetRequiredFileMentionCountFloor(original) })

	if got := CurrentRequiredFileMentionCountFloor(); got != defaultRequiredFileMentionCountFloor {
		t.Fatalf("default floor: got %d want %d", got, defaultRequiredFileMentionCountFloor)
	}
	SetRequiredFileMentionCountFloor(7)
	if got := CurrentRequiredFileMentionCountFloor(); got != 7 {
		t.Errorf("override failed; got %d want 7", got)
	}
	SetRequiredFileMentionCountFloor(0)
	if got := CurrentRequiredFileMentionCountFloor(); got != defaultRequiredFileMentionCountFloor {
		t.Errorf("zero must restore default; got %d", got)
	}
	SetRequiredFileMentionCountFloor(-5)
	if got := CurrentRequiredFileMentionCountFloor(); got != defaultRequiredFileMentionCountFloor {
		t.Errorf("negative must restore default; got %d", got)
	}
}

func TestSortFsHitsByMention_StableOnTies(t *testing.T) {
	// Three hits with same MentionCount=2. Stable sort preserves
	// insertion order so downstream consumers stay deterministic.
	hits := []FsHit{
		{Path: "first.yaml", MentionCount: 2},
		{Path: "second.yaml", MentionCount: 2},
		{Path: "third.yaml", MentionCount: 2},
	}
	sortFsHitsByMention(hits)
	want := []string{"first.yaml", "second.yaml", "third.yaml"}
	got := []string{hits[0].Path, hits[1].Path, hits[2].Path}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stable sort on ties: got %v want %v", got, want)
	}
}

func TestSetMentionCountMaxGrep_DefaultAndOverride(t *testing.T) {
	original := CurrentMentionCountMaxGrep()
	t.Cleanup(func() { SetMentionCountMaxGrep(original) })

	if got := CurrentMentionCountMaxGrep(); got != defaultMentionCountMaxGrep {
		t.Fatalf("default cap: got %d want %d", got, defaultMentionCountMaxGrep)
	}
	SetMentionCountMaxGrep(7)
	if got := CurrentMentionCountMaxGrep(); got != 7 {
		t.Errorf("override failed; got %d want 7", got)
	}
	SetMentionCountMaxGrep(0)
	if got := CurrentMentionCountMaxGrep(); got != defaultMentionCountMaxGrep {
		t.Errorf("zero must restore default; got %d", got)
	}
	SetMentionCountMaxGrep(-5)
	if got := CurrentMentionCountMaxGrep(); got != defaultMentionCountMaxGrep {
		t.Errorf("negative must restore default; got %d", got)
	}
}

func TestAnnotateMentionCounts_GrepBudgetCap(t *testing.T) {
	// Cap=2 budget; 4 unique basenames → first 2 get real
	// MentionCount, last 2 get MentionCount=0 (budget-exhausted).
	original := CurrentMentionCountMaxGrep()
	t.Cleanup(func() { SetMentionCountMaxGrep(original) })
	SetMentionCountMaxGrep(2)

	root := scratchRepo(t, map[string]string{
		"a.conf":     "k1=v\n",
		"b.conf":     "k2=v\n",
		"c.conf":     "k3=v\n",
		"d.conf":     "k4=v\n",
		"src/all.go": "// references a.conf b.conf c.conf d.conf\npackage all\n",
	})
	hits := []FsHit{
		{Path: "a.conf"}, {Path: "b.conf"},
		{Path: "c.conf"}, {Path: "d.conf"},
	}
	annotateMentionCounts(root, hits)

	// First 2 should have non-zero count (each basename appears in
	// at least the all.go reference).
	if hits[0].MentionCount == 0 {
		t.Errorf("hits[0] should get a real count; got 0")
	}
	if hits[1].MentionCount == 0 {
		t.Errorf("hits[1] should get a real count; got 0")
	}
	// Last 2 should be 0 (budget exhausted).
	if hits[2].MentionCount != 0 {
		t.Errorf("hits[2] over budget cap; got %d want 0", hits[2].MentionCount)
	}
	if hits[3].MentionCount != 0 {
		t.Errorf("hits[3] over budget cap; got %d want 0", hits[3].MentionCount)
	}
}

func TestAnnotateMentionCounts_CacheReusesAcrossDuplicateBasenames(t *testing.T) {
	// Layer C basename match can produce multiple hits sharing a
	// basename (services/api/Dockerfile + services/web/Dockerfile).
	// Cache must collapse to ONE grep — both hits get the same count.
	original := CurrentMentionCountMaxGrep()
	t.Cleanup(func() { SetMentionCountMaxGrep(original) })
	SetMentionCountMaxGrep(1) // single grep budget

	root := scratchRepo(t, map[string]string{
		"services/api/Dockerfile": "FROM scratch\n",
		"services/web/Dockerfile": "FROM scratch\n",
		"src/refs.go":             "// see services/*/Dockerfile\npackage src\n",
	})
	hits := []FsHit{
		{Path: "services/api/Dockerfile"},
		{Path: "services/web/Dockerfile"},
	}
	annotateMentionCounts(root, hits)

	if hits[0].MentionCount == 0 {
		t.Errorf("hits[0] should be annotated; got 0")
	}
	if hits[1].MentionCount != hits[0].MentionCount {
		t.Errorf("duplicate basename should share cached count; %d vs %d",
			hits[1].MentionCount, hits[0].MentionCount)
	}
}

func TestSortFsHitsByMention_DescendingOrder(t *testing.T) {
	hits := []FsHit{
		{Path: "low.conf", MentionCount: 1},
		{Path: "high.conf", MentionCount: 10},
		{Path: "mid.conf", MentionCount: 5},
	}
	sortFsHitsByMention(hits)
	want := []string{"high.conf", "mid.conf", "low.conf"}
	got := []string{hits[0].Path, hits[1].Path, hits[2].Path}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("desc sort: got %v want %v", got, want)
	}
}

func TestSetMentionedFileSiblingSuffixes_NormalisesAndDedups(t *testing.T) {
	original := CurrentMentionedFileSiblingSuffixes()
	t.Cleanup(func() { SetMentionedFileSiblingSuffixes(original) })

	SetMentionedFileSiblingSuffixes([]string{
		"in",        // missing leading dot — auto-prepended
		".dist",     // already canonical
		"  .tmpl  ", // whitespace trim
		"in",        // dup of normalised entry
		"",          // empty drop
	})
	got := CurrentMentionedFileSiblingSuffixes()
	want := []string{".in", ".dist", ".tmpl"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestSetMentionedFileSiblingSuffixes_NilRestoresDefault(t *testing.T) {
	original := CurrentMentionedFileSiblingSuffixes()
	t.Cleanup(func() { SetMentionedFileSiblingSuffixes(original) })

	SetMentionedFileSiblingSuffixes([]string{".x"})
	if got := CurrentMentionedFileSiblingSuffixes(); !reflect.DeepEqual(got, []string{".x"}) {
		t.Fatalf("override failed; got %v", got)
	}
	SetMentionedFileSiblingSuffixes(nil)
	if got := CurrentMentionedFileSiblingSuffixes(); !reflect.DeepEqual(got, defaultMentionedFileSiblingSuffixes) {
		t.Errorf("nil should restore default; got %v", got)
	}
}

func TestMergeRequiredFilePathLists_HeadFirstWithDedup(t *testing.T) {
	got := mergeRequiredFilePathLists(
		[]string{"a", "b", "a", ""}, // includes a dup + an empty
		[]string{"b", "c", "d"},
		3,
	)
	want := []string{"a", "b", "c"} // a, b from head; c clipped to cap=3
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMergeRequiredFilePathLists_CapZeroReturnsAll(t *testing.T) {
	got := mergeRequiredFilePathLists([]string{"a"}, []string{"b", "c"}, 0)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
