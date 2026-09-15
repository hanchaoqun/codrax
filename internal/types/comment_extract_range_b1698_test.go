package types

import (
	"strings"
	"testing"
)

func TestB1698CommentRangeKeepsLegacyTextAndDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, path, source, text string
		definition, start, end   int
	}{
		{"go_blank_and_attribute", "x.go", "// First line.\n\n// Second line.\n\nfunc Worker() {}\n", "First line. Second line.", 5, 1, 3},
		{"rust_attribute", "x.rs", "/// First line.\n//! Second line.\n#[inline]\nfn worker() {}\n", "First line. Second line.", 4, 1, 2},
		{"java_block", "X.java", "/**\n * First line.\n * Second line.\n */\n@Route(\"/x\")\nclass X {}\n", "First line. Second line.", 6, 1, 4},
		{"java_slash_fallback", "X.java", "// First line.\n// Second line.\nclass X {}\n", "First line. Second line.", 3, 1, 2},
		{"python_single", "x.py", "def worker():\n    '''Documented action.'''\n    return 1\n", "Documented action.", 1, 2, 2},
		{"python_multi", "x.py", "def worker():\n    \"\"\"First line.\n    Second line.\n    \"\"\"\n    return 1\n", "First line. Second line.", 1, 2, 4},
		{"python_unclosed_legacy", "x.py", "def worker():\n    \"\"\"Unclosed text.\n", "Unclosed text.", 1, 2, 0},
		{"hash", "x.rb", "# First line.\n# Second line.\ndef worker\nend\n", "First line. Second line.", 3, 1, 2},
		{"html", "x.html", "<!-- First line.\nSecond line. -->\n<component />\n", "First line. Second line.", 3, 1, 2},
		{"typescript_lines", "x.ts", "// Documented action.\n@decorator\nclass Worker {}\n", "Documented action.", 3, 1, 1},
		{"javascript_block_unsupported", "x.js", "/** A block. */\nfunction worker() {}\n", "", 2, 0, 0},
		{"no_comment", "x.go", "package p\nfunc Worker() {}\n", "", 2, 0, 0},
		{"empty_comment", "X.java", "/** */\nclass X {}\n", "", 2, 0, 0},
		{"too_many_input_lines", "x.go", strings.Repeat("// A line.\n", 13) + "func Worker() {}\n", "", 14, 0, 0},
		{"too_many_input_bytes", "X.java", "/** " + strings.Repeat("x", 801) + " */\nclass X {}\n", "", 2, 0, 0},
		{"invalid_definition", "x.go", "// Note.\n", "", 0, 0, 0},
		{"definition_outside_source", "x.go", "// Note.\n", "", 9, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacyText, legacyStart := ExtractLeadingDocComment([]byte(tc.source), tc.definition, tc.path)
			text, start, end := ExtractLeadingDocCommentRange([]byte(tc.source), tc.definition, tc.path)
			if legacyText != tc.text || legacyStart != tc.start || text != tc.text || start != tc.start || end != tc.end {
				t.Fatalf("legacy=(%q,%d) range=(%q,%d,%d); want (%q,%d,%d)", legacyText, legacyStart, text, start, end, tc.text, tc.start, tc.end)
			}
		})
	}
}

func TestB1698CommentRangeDoesNotUseTruncatedSummaryForExtent(t *testing.T) {
	source := strings.Repeat("// A documented line.\n", 8) + "func Worker() {}\n"
	text, start, end := ExtractLeadingDocCommentRange([]byte(source), 9, "x.go")
	if text != strings.TrimSpace(strings.Repeat("A documented line. ", 6)) || start != 1 || end != 8 {
		t.Fatalf("summary cap must not truncate source extent: %q %d-%d", text, start, end)
	}
	source = "/** " + strings.Repeat("说明 abc ", 50) + " */\nclass Worker {}\n"
	text, start, end = ExtractLeadingDocCommentRange([]byte(source), 2, "Worker.java")
	if text == "" || start != 1 || end != 1 || len([]rune(text)) > maxOutChars+1 || !strings.Contains(text, "…") {
		t.Fatalf("rune-safe clipped summary must retain its original complete source line: %q %d-%d", text, start, end)
	}
}
