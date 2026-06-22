package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryTrackedSourceFilesForLensUsesGitTrackedUniverse(t *testing.T) {
	repo := t.TempDir()
	writeTrackedSourceFilesTestFile(t, repo, "src/app/main.go", "package app\nfunc Run() {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", "package demo.cart\npublic class Cart {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj", "package demo.stringext\nextend String {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "vendor/example/native.cpp", "int native_value() { return 1; }\n")
	writeTrackedSourceFilesTestFile(t, repo, "generated/service.pb.go", "package generated\n")
	writeTrackedSourceFilesTestFile(t, repo, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/untracked_noise.cj", "package demo.noise\npublic class Noise {}\n")

	runGitForTrackedSourceFilesTest(t, repo, "init")
	runGitForTrackedSourceFilesTest(t, repo, "add",
		"src/app/main.go",
		"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
		"vendor/example/native.cpp",
		"generated/service.pb.go",
	)

	files, complete := SourceInventoryTrackedSourceFilesForLens(repo, types.SourceInventoryLensQuery{Path: ".", Scopes: []string{"."}})
	if !complete {
		t.Fatal("git-tracked source universe should be complete")
	}
	byPath := map[string]SourceInventoryTrackedSourceFile{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for path, wantRole := range map[string]types.SourcePathRole{
		"src/app/main.go": types.SourcePathRoleProduction,
		"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj":                          types.SourcePathRoleFixture,
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj": types.SourcePathRoleThirdParty,
		"vendor/example/native.cpp":                                                    types.SourcePathRoleVendor,
		"generated/service.pb.go":                                                      types.SourcePathRoleGenerated,
	} {
		got, ok := byPath[path]
		if !ok {
			t.Fatalf("tracked source file %s missing from universe: %+v", path, files)
		}
		if got.Role != wantRole {
			t.Fatalf("role for %s = %q, want %q", path, got.Role, wantRole)
		}
		if got.Language == "" {
			t.Fatalf("language for %s should be detected", path)
		}
	}
	if _, ok := byPath["internal/thirdparty/tree-sitter-cangjie/corpus/sources/untracked_noise.cj"]; ok {
		t.Fatalf("untracked workspace file entered git-tracked source universe: %+v", files)
	}
}

func TestSourceInventorySourceClassUniverseCarriesClassLanguageMatrix(t *testing.T) {
	repo := t.TempDir()
	writeTrackedSourceFilesTestFile(t, repo, "src/app/main.go", "package app\nfunc Run() {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", "package demo.cart\npublic class Cart {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj", "package demo.stringext\nextend String {}\n")
	writeTrackedSourceFilesTestFile(t, repo, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", "package demo.ffi\nforeign func native_add(a: Int64, b: Int64): Int64\n")

	runGitForTrackedSourceFilesTest(t, repo, "init")
	runGitForTrackedSourceFilesTest(t, repo, "add",
		"src/app/main.go",
		"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
	)

	classes := sourceInventorySourceClassUniverseForLens(&types.BusContext{RepoRoot: repo}, types.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"."},
	})
	byRole := map[types.SourcePathRole]types.SourceInventorySourceClassCount{}
	for _, class := range classes {
		byRole[class.Role] = class
	}
	if got := sourceInventoryTestClassLanguageCount(byRole[types.SourcePathRoleFixture], "cangjie"); got != 1 {
		t.Fatalf("fixture cangjie count = %d, want 1; classes=%+v", got, classes)
	}
	if got := sourceInventoryTestClassLanguageCount(byRole[types.SourcePathRoleThirdParty], "cangjie"); got != 2 {
		t.Fatalf("thirdparty cangjie count = %d, want 2; classes=%+v", got, classes)
	}
	if got := sourceInventoryTestClassLanguageCount(byRole[types.SourcePathRoleProduction], "go"); got != 1 {
		t.Fatalf("production go count = %d, want 1; classes=%+v", got, classes)
	}
}

func sourceInventoryTestClassLanguageCount(class types.SourceInventorySourceClassCount, language string) int {
	for _, item := range class.Languages {
		if item.Language == language {
			return item.Count
		}
	}
	return 0
}

func TestSourceInventoryTrackedSourceFilesForLensFilesystemFallback(t *testing.T) {
	repo := t.TempDir()
	writeTrackedSourceFilesTestFile(t, repo, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "@Entry\n@Component\nstruct Index {}\n")

	files, complete := SourceInventoryTrackedSourceFilesForLens(repo, types.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"internal/thirdparty/tree-sitter-arkts/corpus"},
	})
	if !complete {
		t.Fatal("small filesystem fallback universe should be complete")
	}
	if len(files) != 1 {
		t.Fatalf("fallback files = %+v, want one scoped ArkTS corpus file", files)
	}
	if files[0].Role != types.SourcePathRoleThirdParty || files[0].Language == "" {
		t.Fatalf("fallback source classification = %+v", files[0])
	}
}

func writeTrackedSourceFilesTestFile(t *testing.T, repo, rel, body string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGitForTrackedSourceFilesTest(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd, cancel := NewGitCommand(nil, args...)
	defer cancel()
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
