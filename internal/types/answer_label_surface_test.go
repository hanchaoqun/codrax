package types

import (
	"strings"
	"testing"
)

func TestParseAnswerSourceLocationSurface_CrossLanguagePaths(t *testing.T) {
	cases := []string{
		"internal/agent/analyzer.go:1903",
		"src/native/Foo.cpp:42",
		"include/runtime/Foo.hpp:7",
		"entry/src/main/ets/pages/Index.ets:20",
		"src/main/cangjie/main.cj:3",
		"config/codrax.yaml:11",
		"`internal/tool/runner.ts:9`",
		"C:/proj/native/foo.c:12",
	}
	for _, label := range cases {
		if _, ok := ParseAnswerSourceLocationSurface(label); !ok {
			t.Fatalf("ParseAnswerSourceLocationSurface(%q) = false; want true", label)
		}
	}
}

func TestParseAnswerSourceLocationSurface_DoesNotSwallowSupportRefComposite(t *testing.T) {
	if _, ok := ParseAnswerSourceLocationSurface("explorerEvaluator @ internal/agent/explorer.go:30"); ok {
		t.Fatal("support-ref composite member must not parse as a single source location")
	}
	if _, ok := ParseAnswerSourceLocationSurface("Component | entry/src/main/ets/pages/Index.ets:20"); ok {
		t.Fatal("pipe support-ref composite member must not parse as a single source location")
	}
}

func TestParseAnswerSupportRefMemberLocation_CompositeDisplays(t *testing.T) {
	cases := []struct {
		raw   string
		label string
		file  string
		line  int
	}{
		{"explorerEvaluator @ internal/agent/explorer.go:30", "explorerEvaluator", "internal/agent/explorer.go", 30},
		{"Component | entry/src/main/ets/pages/Index.ets:20", "Component", "entry/src/main/ets/pages/Index.ets", 20},
		{"RouteHandler (src/routes/user.ts:42)", "RouteHandler", "src/routes/user.ts", 42},
	}
	for _, tc := range cases {
		label, loc, ok := ParseAnswerSupportRefMemberLocation(tc.raw)
		if !ok {
			t.Fatalf("ParseAnswerSupportRefMemberLocation(%q) = false", tc.raw)
		}
		if label != tc.label || !strings.EqualFold(loc.File, tc.file) || loc.LineStart != tc.line {
			t.Fatalf("ParseAnswerSupportRefMemberLocation(%q) = (%q, %+v), want %q %s:%d",
				tc.raw, label, loc, tc.label, tc.file, tc.line)
		}
	}
}

func TestAnswerSupportRefLabelIsGeneric(t *testing.T) {
	for _, label := range []string{"Member", "members", "`item`", "principal_member"} {
		if !AnswerSupportRefLabelIsGeneric(label) {
			t.Fatalf("AnswerSupportRefLabelIsGeneric(%q) = false, want true", label)
		}
	}
	for _, label := range []string{"Intent", "explorer", "RouteHandler"} {
		if AnswerSupportRefLabelIsGeneric(label) {
			t.Fatalf("AnswerSupportRefLabelIsGeneric(%q) = true, want false", label)
		}
	}
}

func TestAnswerCodeSurfaceAppearsInTextUsesIdentityBoundaries(t *testing.T) {
	if !AnswerCodeSurfaceAppearsInText(`return "explorer"`, "explorer") {
		t.Fatal("quoted return literal should support the exact code surface")
	}
	if AnswerCodeSurfaceAppearsInText("goroutine scheduler", "go") {
		t.Fatal("short surface must not match as a substring inside a larger identity token")
	}
	if !AnswerCodeSurfaceAppearsInText("type Foo::Bar struct {}", "Foo::Bar") {
		t.Fatal("C++/Cangjie-style qualified surface should match exactly")
	}
}

func TestAnswerSourceLocationLabelMatchesCitation(t *testing.T) {
	if !AnswerSourceLocationLabelMatchesCitation("internal/agent/analyzer.go:1903", Citation{
		File: "internal/agent/analyzer.go",
		Line: 1903,
	}) {
		t.Fatal("exact repo-relative source label should match same citation")
	}
	if !AnswerSourceLocationLabelMatchesCitation("analyzer.go:1903", Citation{
		File: "internal/agent/analyzer.go",
		Line: 1903,
	}) {
		t.Fatal("basename source label should match citation suffix")
	}
	if !AnswerSourceLocationLabelMatchesCitation("entry/src/main/ets/pages/Index.ets:20-24", Citation{
		File: "entry/src/main/ets/pages/Index.ets",
		Line: 22,
	}) {
		t.Fatal("source label line range should match citation inside range")
	}
	if AnswerSourceLocationLabelMatchesCitation("internal/agent/analyzer.go:1904", Citation{
		File: "internal/agent/analyzer.go",
		Line: 1903,
	}) {
		t.Fatal("different line must not match")
	}
	if AnswerSourceLocationLabelMatchesCitation("HTTP:200", Citation{
		File: "internal/agent/analyzer.go",
		Line: 200,
	}) {
		t.Fatal("non-path labels must not be treated as source locations")
	}
}

func TestAnswerFilePathLabelMatchesCitation(t *testing.T) {
	if !AnswerFilePathLabelMatchesCitation("internal/analysis/compiler/templates.go", Citation{
		File: "internal/analysis/compiler/templates.go",
		Line: 256,
	}) {
		t.Fatal("repo-relative file label should match a citation inside the same file")
	}
	if !AnswerFilePathLabelMatchesCitation("templates.go", Citation{
		File: "internal/analysis/compiler/templates.go",
		Line: 366,
	}) {
		t.Fatal("basename file label should match citation suffix")
	}
	if !AnswerFilePathLabelMatchesCitation("entry/src/main/ets/pages/Index.ets", Citation{
		File: "entry/src/main/ets/pages/Index.ets",
		Line: 20,
	}) {
		t.Fatal("ArkTS file label should use the shared code/config path extension registry")
	}
	if !AnswerFilePathLabelMatchesCitation("src/main/cangjie/main.cj", Citation{
		File: "src/main/cangjie/main.cj",
		Line: 3,
	}) {
		t.Fatal("Cangjie file label should use the shared code/config path extension registry")
	}
	if AnswerFilePathLabelMatchesCitation("internal/agent/analyzer.go", Citation{
		File: "internal/tool/emit_analysis.go",
		Line: 1,
	}) {
		t.Fatal("different file must not match")
	}
	if AnswerFilePathLabelMatchesCitation("CitationReq.Required", Citation{
		File: "internal/types/analysis_ir.go",
		Line: 1244,
	}) {
		t.Fatal("symbol-like labels must not be treated as file paths")
	}
}
