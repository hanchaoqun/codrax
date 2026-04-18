package findings_validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// TestValidate_ExistingPath_Unchanged: a real repo path stays as-is.
func TestValidate_ExistingPath_Unchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal/agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal/agent/explorer.go"), []byte("package agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := "The function lives in internal/agent/explorer.go and works."
	got := Validate(in, dir, nil)
	if got.Annotated != in {
		t.Errorf("existing path was annotated:\n  got: %q\n want: %q", got.Annotated, in)
	}
	if len(got.Unverified) != 0 {
		t.Errorf("expected no unverified findings, got %v", got.Unverified)
	}
}

// TestValidate_MissingPath_Annotated: a non-existent path gets the
// strikethrough + warning tag.
func TestValidate_MissingPath_Annotated(t *testing.T) {
	dir := t.TempDir()
	in := "See internal/context/builder_test.go for the helper."
	got := Validate(in, dir, nil)
	if !strings.Contains(got.Annotated, "~~internal/context/builder_test.go~~") {
		t.Errorf("expected strikethrough on missing path:\n%s", got.Annotated)
	}
	if !strings.Contains(got.Annotated, "unverified") {
		t.Errorf("expected unverified label, got: %s", got.Annotated)
	}
	if len(got.Unverified) != 1 || got.Unverified[0].Kind != "path" {
		t.Errorf("expected 1 path miss, got %v", got.Unverified)
	}
}

// TestValidate_ChineseInput_ChineseLabel: CJK input switches the
// warning label to Chinese.
func TestValidate_ChineseInput_ChineseLabel(t *testing.T) {
	dir := t.TempDir()
	in := "在 internal/context/builder_test.go 中定义了辅助函数。"
	got := Validate(in, dir, nil)
	if !strings.Contains(got.Annotated, "未验证") {
		t.Errorf("expected Chinese label, got: %s", got.Annotated)
	}
}

// TestValidate_ExistingSymbol_Unchanged: backticked symbol that is
// in the graph passes through.
func TestValidate_ExistingSymbol_Unchanged(t *testing.T) {
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{
			"BuildPromptContext": {&repomap.Symbol{Name: "BuildPromptContext", Kind: "function"}},
		},
	}
	in := "Call `BuildPromptContext` to assemble the prompt."
	got := Validate(in, "", graph)
	if got.Annotated != in {
		t.Errorf("existing symbol was annotated:\n  got: %q\n want: %q", got.Annotated, in)
	}
}

// TestValidate_MissingSymbol_Annotated: backticked symbol missing
// from graph gets the warning.
func TestValidate_MissingSymbol_Annotated(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}}
	in := "Inspect `explorerSkill` for details."
	got := Validate(in, "", graph)
	if !strings.Contains(got.Annotated, "~~`explorerSkill`~~") {
		t.Errorf("expected strikethrough on missing symbol:\n%s", got.Annotated)
	}
	if len(got.Unverified) != 1 || got.Unverified[0].Token != "explorerSkill" {
		t.Errorf("expected 1 symbol miss for explorerSkill, got %v", got.Unverified)
	}
}

// TestValidate_PlainEnglishWordInBackticks_Skipped: lowercase
// English words wrapped in backticks are NOT validated as
// identifiers (too noisy).
func TestValidate_PlainEnglishWordInBackticks_Skipped(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}}
	in := "This is `important` to know."
	got := Validate(in, "", graph)
	if got.Annotated != in {
		t.Errorf("plain word should not be validated as symbol:\n  got: %q", got.Annotated)
	}
}

// TestValidate_BugRegression: the original analyzer hallucination
// from the bug trace gets caught.
func TestValidate_BugRegression(t *testing.T) {
	graph := &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}}
	dir := t.TempDir() // empty repo — no internal/context/builder_test.go
	in := "在 internal/context/builder_test.go 文件中，有一个名为 `explorerSkill` 的函数可能与 Explorer agent 的默认技能相关。"
	got := Validate(in, dir, graph)
	if !strings.Contains(got.Annotated, "~~internal/context/builder_test.go~~") {
		t.Errorf("expected hallucinated path annotated:\n%s", got.Annotated)
	}
	if !strings.Contains(got.Annotated, "~~`explorerSkill`~~") {
		t.Errorf("expected hallucinated symbol annotated:\n%s", got.Annotated)
	}
	if len(got.Unverified) != 2 {
		t.Errorf("expected 2 misses (path + symbol), got %d: %v", len(got.Unverified), got.Unverified)
	}
}

// Empty input passes through.
func TestValidate_EmptyInput(t *testing.T) {
	got := Validate("", "/tmp", nil)
	if got.Annotated != "" {
		t.Errorf("empty input should pass through, got %q", got.Annotated)
	}
}

// Both checks disabled (no repoRoot, no graph) is a no-op.
func TestValidate_BothChecksDisabled_Identity(t *testing.T) {
	in := "internal/agent/explorer.go and `Symbol` here."
	got := Validate(in, "", nil)
	if got.Annotated != in {
		t.Errorf("disabled validator must be identity, got %q", got.Annotated)
	}
}
