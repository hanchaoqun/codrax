package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func makeGraph(symbols map[string][]*repomap.Symbol, files map[string]*repomap.FileInfo) *repomap.Graph {
	return &repomap.Graph{
		SymbolDefs: symbols,
		FileIndex:  files,
	}
}

func TestSymbolResolver_NilGraphReturnsNilAdapter(t *testing.T) {
	if r := newRepomapSymbolResolver(nil); r != nil {
		t.Fatalf("expected nil resolver for nil graph, got %T", r)
	}
}

func TestSymbolResolver_ExactNameHit(t *testing.T) {
	g := makeGraph(map[string][]*repomap.Symbol{
		"Blob": {{Name: "Blob", File: "internal/types/context.go"}},
	}, nil)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("Blob")
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Canonical != "Blob" {
		t.Fatalf("canonical = %q, want Blob", hits[0].Canonical)
	}
	if hits[0].Domain != "types" {
		t.Fatalf("domain = %q, want types", hits[0].Domain)
	}
}

func TestSymbolResolver_CaseInsensitiveFallback(t *testing.T) {
	g := makeGraph(map[string][]*repomap.Symbol{
		"Blob": {{Name: "Blob", File: "internal/types/context.go"}},
	}, nil)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("blob")
	if len(hits) != 1 {
		t.Fatalf("case-insensitive lookup should hit Blob; got %d hits", len(hits))
	}
	if hits[0].Canonical != "Blob" {
		t.Fatalf("canonical should preserve repo spelling, got %q", hits[0].Canonical)
	}
}

func TestSymbolResolver_UnderscoreNormalization(t *testing.T) {
	g := makeGraph(map[string][]*repomap.Symbol{
		"BuildAgentContext": {{Name: "BuildAgentContext", File: "internal/agent/context.go"}},
	}, nil)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("build_agent_context")
	if len(hits) != 1 {
		t.Fatalf("snake→camel should match; got %d hits", len(hits))
	}
}

func TestSymbolResolver_MultipleHitsReturnedSeparately(t *testing.T) {
	g := makeGraph(map[string][]*repomap.Symbol{
		"Config": {
			{Name: "Config", File: "internal/config/runtime.go"},
			{Name: "Config", File: "internal/tool/repomap/config.go"},
		},
	}, nil)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("Config")
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits for two definitions, got %d", len(hits))
	}
	if hits[0].Domain == hits[1].Domain {
		t.Fatalf("two definitions in different dirs should have different domains; got both %q", hits[0].Domain)
	}
}

func TestSymbolResolver_HitCapAt20(t *testing.T) {
	defs := make([]*repomap.Symbol, 0, 50)
	for i := 0; i < 50; i++ {
		defs = append(defs, &repomap.Symbol{
			Name: "Handler",
			File: "internal/agent/file" + string(rune('a'+i%26)) + ".go",
			Line: i,
		})
	}
	g := makeGraph(map[string][]*repomap.Symbol{"Handler": defs}, nil)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("Handler")
	if len(hits) > maxSymbolResolverHits {
		t.Fatalf("resolver must cap hits at %d, got %d", maxSymbolResolverHits, len(hits))
	}
}

func TestSymbolResolver_FileIndexPackageWinsOverPrefix(t *testing.T) {
	g := makeGraph(
		map[string][]*repomap.Symbol{
			"MyFunc": {{Name: "MyFunc", File: "internal/tool/repomap/foo.go"}},
		},
		map[string]*repomap.FileInfo{
			"internal/tool/repomap/foo.go": {Package: "repomap"},
		},
	)
	r := newRepomapSymbolResolver(g)
	hits := r.LookupSymbol("MyFunc")
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Domain != "repomap" {
		t.Fatalf("FileInfo.Package should take precedence over prefix table; got %q", hits[0].Domain)
	}
}

func TestSymbolResolver_PathFallbackWhenPackageMissing(t *testing.T) {
	cases := []struct {
		file   string
		domain string
	}{
		// Parent-dir rule: the immediate enclosing directory name.
		// Works identically for Go / Python / Java / Rust / TS layouts
		// without touching a prefix table.
		{"internal/agent/explorer.go", "agent"},
		{"internal/tool/repomap/tool.go", "repomap"},
		{"internal/analysis/hdp/planner.go", "hdp"},
		{"cmd/codrax/main.go", "codrax"},
		{"src/auth/login.py", "auth"},
		{"src/main/java/com/myapp/user/UserService.java", "user"},
		{"src/parser/tokens.rs", "parser"},
		{"lib/formatter/pretty.js", "formatter"},
		// Root-level file: no parent to infer from.
		{"README.md", ""},
		{"main.go", ""},
	}
	for _, c := range cases {
		g := makeGraph(map[string][]*repomap.Symbol{
			"X": {{Name: "X", File: c.file}},
		}, nil)
		r := newRepomapSymbolResolver(g)
		hits := r.LookupSymbol("X")
		if len(hits) != 1 || hits[0].Domain != c.domain {
			t.Fatalf("file=%q expected domain=%q, got hits=%+v", c.file, c.domain, hits)
		}
	}
}

func TestSymbolResolver_LookupFileSurface_ExactAndUniqueBasename(t *testing.T) {
	g := makeGraph(nil, map[string]*repomap.FileInfo{
		"internal/orchestrator/orchestrator.go": {RelPath: "internal/orchestrator/orchestrator.go", Package: "orchestrator"},
		"entry/src/main/ets/pages/Index.ets":    {RelPath: "entry/src/main/ets/pages/Index.ets", Package: "pages"},
		"pkg/foo.cj":                            {RelPath: "pkg/foo.cj", Package: "pkg"},
		"src/lib.cpp":                           {RelPath: "src/lib.cpp", Package: "src"},
	})
	r := newRepomapSymbolResolver(g).(interface {
		LookupFileSurface(string) []normalizer.SymbolHit
	})
	cases := []struct {
		query string
		want  string
	}{
		{"internal/orchestrator/orchestrator.go", "internal/orchestrator/orchestrator.go"},
		{"orchestrator.go", "internal/orchestrator/orchestrator.go"},
		{"Index.ets", "entry/src/main/ets/pages/Index.ets"},
		{"pkg/foo.cj", "pkg/foo.cj"},
		{"src/lib.cpp", "src/lib.cpp"},
	}
	for _, tc := range cases {
		hits := r.LookupFileSurface(tc.query)
		if len(hits) != 1 {
			t.Fatalf("LookupFileSurface(%q) hits=%d, want 1", tc.query, len(hits))
		}
		if hits[0].Canonical != tc.want {
			t.Fatalf("LookupFileSurface(%q) canonical=%q, want %q", tc.query, hits[0].Canonical, tc.want)
		}
	}
}

func TestSymbolResolver_LookupFileSurface_AmbiguousBasenameReturnsNil(t *testing.T) {
	g := makeGraph(nil, map[string]*repomap.FileInfo{
		"internal/a/config.go": {RelPath: "internal/a/config.go"},
		"internal/b/config.go": {RelPath: "internal/b/config.go"},
	})
	r := newRepomapSymbolResolver(g).(interface {
		LookupFileSurface(string) []normalizer.SymbolHit
	})
	if hits := r.LookupFileSurface("config.go"); len(hits) != 0 {
		t.Fatalf("ambiguous basename must not become a hard gate signal; got %+v", hits)
	}
}

func TestSymbolResolver_EmptySurface(t *testing.T) {
	g := makeGraph(map[string][]*repomap.Symbol{
		"Blob": {{Name: "Blob", File: "internal/types/context.go"}},
	}, nil)
	r := newRepomapSymbolResolver(g)
	if hits := r.LookupSymbol(""); hits != nil {
		t.Fatalf("empty surface should return nil, got %v", hits)
	}
	if hits := r.LookupSymbol("   "); hits != nil {
		t.Fatalf("whitespace-only surface should return nil, got %v", hits)
	}
}
