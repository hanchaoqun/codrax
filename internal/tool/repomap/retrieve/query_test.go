package retrieve

import (
	"reflect"
	"sort"
	"testing"
)

// TestIsTestFile locks the test-vs-impl classifier used by
// queryMatchScore to damp _test.go siblings. Positive cases must
// catch every language convention repomap supports; negative cases
// must avoid false positives on implementation files that happen
// to contain "test" in their path or name.
func TestIsTestFile(t *testing.T) {
	positive := []string{
		"internal/agent/finalizer_test.go",
		"tool/repomap/resolver_go_test.go",
		"tests/foo.rs",
		"crates/app/tests/integration.rs",
		"foo/test_something.py",
		"foo/module_test.py",
		"com/acme/MyClassTest.java",
		"com/acme/MyClassTests.java",
		"src/test/kotlin/com/acme/MyServiceTest.kt",
		"spec/models/user_spec.rb",
		"Tests/AppTests/FeatureTests.swift",
		"lua/spec/router_spec.lua",
		"src/components/Button.test.tsx",
		"src/app.spec.ts",
		"src/app.test.js",
	}
	for _, p := range positive {
		if !isTestFile(p) {
			t.Errorf("isTestFile(%q) = false, want true", p)
		}
	}
	negative := []string{
		"internal/agent/tester.go",            // substring "test" but not a test file
		"internal/analysis/gate/gate.go",      // no test
		"src/testimonial/page.tsx",            // substring "test" in dir
		"internal/tool/repomap/extract_go.go", // contains "_" + "go"
		"com/acme/Tester.java",                // not *Test.java
		"foo/testing.py",                      // not test_* / *_test
		"app/bin/setup.rb",                    // executable, not test
		"lua/inspect.lua",                     // lua utility, not test
	}
	for _, p := range negative {
		if isTestFile(p) {
			t.Errorf("isTestFile(%q) = true, want false", p)
		}
	}
}

// TestTokenizeQuery locks the core behaviors expected by the
// multilingual rewrite of queryMatchScore: primary + CamelCase /
// snake / kebab / dotted decomposition, CJK bi-gram, dedup, digit
// reclassification, and mixed-script queries.
func TestTokenizeQuery(t *testing.T) {
	type expected struct {
		text   string
		kind   string
		weight float64
	}
	cases := []struct {
		label string
		query string
		want  []expected
	}{
		{
			label: "single lowercase word",
			query: "graph",
			want: []expected{
				{"graph", TokenIdentifier, 1.0},
			},
		},
		{
			label: "whitespace-separated words",
			query: "build agent context",
			want: []expected{
				{"build", TokenIdentifier, 1.0},
				{"agent", TokenIdentifier, 1.0},
				{"context", TokenIdentifier, 1.0},
			},
		},
		{
			label: "CamelCase expands to primary + subs",
			query: "BuildAgentContext",
			want: []expected{
				{"buildagentcontext", TokenIdentifier, 1.0},
				{"build", TokenIdentifier, 0.5},
				{"agent", TokenIdentifier, 0.5},
				{"context", TokenIdentifier, 0.5},
			},
		},
		{
			label: "HTTPServer uppercase-run rule",
			query: "HTTPServer",
			want: []expected{
				{"httpserver", TokenIdentifier, 1.0},
				{"http", TokenIdentifier, 0.5},
				{"server", TokenIdentifier, 0.5},
			},
		},
		{
			label: "snake_case expands to primary + subs",
			query: "propose_sub_agents",
			want: []expected{
				{"propose_sub_agents", TokenIdentifier, 1.0},
				{"propose", TokenIdentifier, 0.5},
				{"sub", TokenIdentifier, 0.5},
				{"agents", TokenIdentifier, 0.5},
			},
		},
		{
			label: "kebab-case expands to primary + subs",
			query: "kebab-case-item",
			want: []expected{
				{"kebab-case-item", TokenIdentifier, 1.0},
				{"kebab", TokenIdentifier, 0.5},
				{"case", TokenIdentifier, 0.5},
				{"item", TokenIdentifier, 0.5},
			},
		},
		{
			label: "dotted path expands to primary + subs",
			query: "graph.go",
			want: []expected{
				{"graph.go", TokenIdentifier, 1.0},
				{"graph", TokenIdentifier, 0.5},
				{"go", TokenIdentifier, 0.5},
			},
		},
		{
			label: "slashed file path expands to primary + subs",
			query: "internal/tool/repomap/graph.go",
			want: []expected{
				{"internal/tool/repomap/graph.go", TokenIdentifier, 1.0},
				{"internal", TokenIdentifier, 0.5},
				{"tool", TokenIdentifier, 0.5},
				{"repomap", TokenIdentifier, 0.5},
				{"graph", TokenIdentifier, 0.5},
				{"go", TokenIdentifier, 0.5},
			},
		},
		{
			label: "dedup on repeat",
			query: "graph graph graph",
			want: []expected{
				{"graph", TokenIdentifier, 1.0},
			},
		},
		{
			label: "all-digit reclassifies as number",
			query: "42",
			want: []expected{
				{"42", TokenNumber, 1.0},
			},
		},
		{
			label: "mixed Chinese + Latin query",
			query: "分析器 analyze",
			want: []expected{
				{"分析", TokenCJK, 1.0},
				{"析器", TokenCJK, 1.0},
				{"analyze", TokenIdentifier, 1.0},
			},
		},
		{
			label: "CJK run 4 hanzi produces 3 bigrams",
			query: "任务分析器",
			want: []expected{
				{"任务", TokenCJK, 1.0},
				{"务分", TokenCJK, 1.0},
				{"分析", TokenCJK, 1.0},
				{"析器", TokenCJK, 1.0},
			},
		},
		{
			label: "single hanzi as unigram",
			query: "分",
			want: []expected{
				{"分", TokenCJK, 1.0},
			},
		},
		{
			label: "CJK + Latin embedded without space",
			query: "有多少个agent可以调用subagent",
			want: []expected{
				{"有多", TokenCJK, 1.0},
				{"多少", TokenCJK, 1.0},
				{"少个", TokenCJK, 1.0},
				{"agent", TokenIdentifier, 1.0},
				{"可以", TokenCJK, 1.0},
				{"以调", TokenCJK, 1.0},
				{"调用", TokenCJK, 1.0},
				{"subagent", TokenIdentifier, 1.0},
			},
		},
		{
			label: "punctuation is dropped",
			query: "parse(func, x);",
			want: []expected{
				{"parse", TokenIdentifier, 1.0},
				{"func", TokenIdentifier, 1.0},
				{"x", TokenIdentifier, 1.0},
			},
		},
		{
			label: "empty query",
			query: "",
			want:  nil,
		},
		{
			label: "whitespace only",
			query: "   \t\n",
			want:  nil,
		},
		{
			label: "README eval-blind case #1",
			query: "propose_sub_agents tool",
			want: []expected{
				{"propose_sub_agents", TokenIdentifier, 1.0},
				{"propose", TokenIdentifier, 0.5},
				{"sub", TokenIdentifier, 0.5},
				{"agents", TokenIdentifier, 0.5},
				{"tool", TokenIdentifier, 1.0},
			},
		},
		{
			label: "README eval-blind case #2",
			query: "BuildAgentContext narrowed view",
			want: []expected{
				{"buildagentcontext", TokenIdentifier, 1.0},
				{"build", TokenIdentifier, 0.5},
				{"agent", TokenIdentifier, 0.5},
				{"context", TokenIdentifier, 0.5},
				{"narrowed", TokenIdentifier, 1.0},
				{"view", TokenIdentifier, 1.0},
			},
		},
		{
			label: "README eval-blind case #3 (Chinese analyzer question)",
			query: "分析器如何派生 run policy",
			want: []expected{
				{"分析", TokenCJK, 1.0},
				{"析器", TokenCJK, 1.0},
				{"器如", TokenCJK, 1.0},
				{"如何", TokenCJK, 1.0},
				{"何派", TokenCJK, 1.0},
				{"派生", TokenCJK, 1.0},
				{"run", TokenIdentifier, 1.0},
				{"policy", TokenIdentifier, 1.0},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			got := TokenizeQuery(c.query)
			if len(got) != len(c.want) {
				t.Fatalf("%q: token count = %d want %d\ngot:  %+v\nwant: %+v",
					c.query, len(got), len(c.want), got, c.want)
			}
			for i, tok := range got {
				w := c.want[i]
				if tok.Text != w.text || tok.Kind != w.kind || tok.Weight != w.weight {
					t.Errorf("token[%d] = %+v, want {%q %q %v}",
						i, tok, w.text, w.kind, w.weight)
				}
			}
		})
	}
}

// TestSplitCamel is a focused lock on the CamelCase / TitleCase
// decomposer, including the HTTPServer uppercase-run backoff rule
// and numeric segments.
func TestSplitCamel(t *testing.T) {
	cases := map[string][]string{
		"":                nil,
		"foo":             {"foo"},
		"Foo":             {"Foo"},
		"fooBar":          {"foo", "Bar"},
		"FooBar":          {"Foo", "Bar"},
		"HTTPServer":      {"HTTP", "Server"},
		"buildHTTPServer": {"build", "HTTP", "Server"},
		"HTTPSConnection": {"HTTPS", "Connection"},
		"parseJSONBody":   {"parse", "JSON", "Body"},
		"v2":              {"v2"},
		"Foo2Bar":         {"Foo", "2", "Bar"},
	}
	for in, want := range cases {
		got := splitCamel(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("splitCamel(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestSplitIdentifier locks the full identifier decomposer used
// inside TokenizeQuery: non-alnum split first, then CamelCase.
func TestSplitIdentifier(t *testing.T) {
	cases := map[string][]string{
		"":                      nil,
		"foo":                   {"foo"},
		"foo_bar":               {"foo", "bar"},
		"foo-bar":               {"foo", "bar"},
		"foo.bar":               {"foo", "bar"},
		"foo/bar":               {"foo", "bar"},
		"foo::bar":              {"foo", "bar"},
		"buildAgentContext":     {"build", "agent", "context"},
		"parse_HTTP_response":   {"parse", "http", "response"},
		"internal/tool/repomap": {"internal", "tool", "repomap"},
		"a.b.c.d":               {"a", "b", "c", "d"},
	}
	// Stable iteration for deterministic failure output.
	keys := make([]string, 0, len(cases))
	for k := range cases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, in := range keys {
		want := cases[in]
		got := splitIdentifier(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("splitIdentifier(%q) = %v, want %v", in, got, want)
		}
	}
}
