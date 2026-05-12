package types

import "testing"

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
