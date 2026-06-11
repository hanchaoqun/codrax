package findings_validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Batch-6 C3: a sub-repo-prefixed path (repo-x/src/lib.rs) must verify
// against the parent root even though pathRegex anchors at src/.
func TestValidate_MultiRepoPrefixedPathVerifies(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "repo-stub-rust", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "repo-stub-rust", "src", "lib.rs"), []byte("pub fn x() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Validate("the function lives in repo-stub-rust/src/lib.rs today", root, nil)
	if len(res.Unverified) != 0 {
		t.Fatalf("prefixed existing path must verify, got unverified=%+v annotated=%q", res.Unverified, res.Annotated)
	}
	if strings.Contains(res.Annotated, "~~") {
		t.Fatalf("no annotation expected, got %q", res.Annotated)
	}

	// A genuinely missing path still annotates.
	res = Validate("see repo-stub-rust/src/missing.rs for details", root, nil)
	if len(res.Unverified) != 1 {
		t.Fatalf("missing path must stay unverified, got %+v", res.Unverified)
	}
}

// Expansion must not swallow prose: a path preceded by a space keeps
// legacy behavior, and expansion without a '/' boundary is rejected.
func TestExpandPathTokenLeft_Boundaries(t *testing.T) {
	text := "see src/lib.rs and repo-a/src/lib.rs"
	// "src/lib.rs" at index 4: preceded by space → no expansion.
	if got := expandPathTokenLeft(text, 4, 14); got != "" {
		t.Fatalf("space-preceded match must not expand, got %q", got)
	}
	idx := strings.Index(text, "repo-a/src/lib.rs")
	srcIdx := idx + len("repo-a/")
	if got := expandPathTokenLeft(text, srcIdx, idx+len("repo-a/src/lib.rs")); got != "repo-a/src/lib.rs" {
		t.Fatalf("expansion = %q, want full prefixed path", got)
	}
}
