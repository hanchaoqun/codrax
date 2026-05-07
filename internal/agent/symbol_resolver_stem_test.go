package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// fakeGraphForStem builds a minimal *repomap.Graph populated with a
// fixed set of (name → file) entries so the stem-lookup tests can
// inspect canonical mapping behaviour without spinning up a full
// scanner.
func fakeGraphForStem(symbols map[string]string) *repomap.Graph {
	g := &repomap.Graph{
		SymbolDefs: make(map[string][]*rmtypes.Symbol, len(symbols)),
		FileIndex:  make(map[string]*rmtypes.FileInfo),
	}
	for name, file := range symbols {
		sym := &rmtypes.Symbol{Name: name, File: file}
		g.SymbolDefs[name] = []*rmtypes.Symbol{sym}
		if _, ok := g.FileIndex[file]; !ok {
			g.FileIndex[file] = &rmtypes.FileInfo{
				RelPath: file, Symbols: []rmtypes.Symbol{*sym},
			}
		}
	}
	return g
}

// TestLookupSymbolStem_M1bR1Reproduction pins the m1b r1 production
// failure: user said "analyzer agent" / "finalizer agent", LLM
// extracted `AnalyzerAgent` / `FinalizerAgent` entities, but the
// repo has `analyzerEvaluator` / `finalizerEvaluator`. With Fix J,
// stripping the "Agent" role suffix → "Analyzer" / "Finalizer"
// resolves the corresponding Evaluator symbols via flat-substring.
func TestLookupSymbolStem_M1bR1Reproduction(t *testing.T) {
	g := fakeGraphForStem(map[string]string{
		"analyzerEvaluator":  "internal/agent/analyzer.go",
		"finalizerEvaluator": "internal/agent/finalizer.go",
	})
	r := newRepomapSymbolResolver(g)
	stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
	for _, ent := range []string{"AnalyzerAgent", "FinalizerAgent"} {
		hits := stemR.LookupSymbolStem(ent)
		if len(hits) == 0 {
			t.Errorf("Fix J: %q must resolve via stem (Agent suffix → Analyzer/Finalizer flat-substring); got 0 hits", ent)
		}
	}
}

// TestLookupSymbolStem_RoleSuffixVariants pins each common role
// suffix the stripper recognises.
func TestLookupSymbolStem_RoleSuffixVariants(t *testing.T) {
	cases := []struct {
		name      string
		userEntity string
		repoSym   string
		mustResolve bool
		note      string
	}{
		{"Agent", "AnalyzerAgent", "analyzerEvaluator", true, "Agent → flat-stem analyzer ⊂ analyzerevaluator"},
		{"Handler", "RequestHandler", "requestProcessor", true, "Handler → request ⊂ requestprocessor"},
		{"Manager", "ConfigManager", "configService", true, "Manager → config ⊂ configservice"},
		{"Service", "AuthService", "authImpl", true, "Service → auth ⊂ authimpl"},
		{"Worker", "JobWorker", "jobRunner", false, "Job stem too short (3 chars below stemSuffixFloor)"},
		{"WorkerLong", "BatchWorker", "batchRunner", true, "Batch ≥4 → batch ⊂ batchrunner"},
		{"Driver", "DBDriver", "dbAdapter", false, "DB stem too short (2 chars below stemSuffixFloor)"},
		{"Controller", "UserController", "userManager", true, "Controller → user ⊂ usermanager"},
		{"Dispatcher", "EventDispatcher", "eventQueue", true, "Dispatcher → event ⊂ eventqueue"},
		{"Processor", "DataProcessor", "dataPipeline", true, "Processor → data ⊂ datapipeline"},
		{"Runner", "TestRunner", "testHarness", true, "Runner → test ⊂ testharness"},
		{"Module", "AuthModule", "authPkg", true, "auth stem 4 chars + authpkg contains 'auth' ⊂ → match"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := fakeGraphForStem(map[string]string{tc.repoSym: "x.go"})
			r := newRepomapSymbolResolver(g)
			stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
			hits := stemR.LookupSymbolStem(tc.userEntity)
			gotResolved := len(hits) > 0
			if gotResolved != tc.mustResolve {
				t.Errorf("[%s] %q resolved=%v, want %v\n  note: %s",
					tc.name, tc.userEntity, gotResolved, tc.mustResolve, tc.note)
			}
		})
	}
}

// TestLookupSymbolStem_NoSuffixMatchReturnsNil — when surface has
// no recognised role suffix, return nil.
func TestLookupSymbolStem_NoSuffixMatchReturnsNil(t *testing.T) {
	g := fakeGraphForStem(map[string]string{
		"realFunctionName": "x.go",
	})
	r := newRepomapSymbolResolver(g)
	stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
	if hits := stemR.LookupSymbolStem("PlainNoun"); len(hits) != 0 {
		t.Errorf("no role suffix → nil; got %d hits", len(hits))
	}
	if hits := stemR.LookupSymbolStem("NotAnEntity"); len(hits) != 0 {
		t.Errorf("no role suffix → nil; got %d hits", len(hits))
	}
}

// TestLookupSymbolStem_StemTooShortReturnsNil — stems below
// stemSuffixFloor (4 chars) are not subject to substring scan.
func TestLookupSymbolStem_StemTooShortReturnsNil(t *testing.T) {
	g := fakeGraphForStem(map[string]string{
		"abcManager": "x.go", // contains "abc"
	})
	r := newRepomapSymbolResolver(g)
	stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
	// "AService" → strip "service" → "a" (1 char, below floor)
	if hits := stemR.LookupSymbolStem("AService"); len(hits) != 0 {
		t.Errorf("stem 'a' below floor → nil; got %d hits", len(hits))
	}
	// "ABCAgent" → strip "agent" → "abc" (3 chars, below floor)
	if hits := stemR.LookupSymbolStem("ABCAgent"); len(hits) != 0 {
		t.Errorf("stem 'abc' below floor → nil; got %d hits", len(hits))
	}
}

// TestLookupSymbolStem_NilResolverTolerated — nil receiver returns
// nil, never panics.
func TestLookupSymbolStem_NilResolverTolerated(t *testing.T) {
	var r *repomapSymbolResolver
	if hits := r.LookupSymbolStem("anything"); hits != nil {
		t.Errorf("nil receiver should return nil; got %d hits", len(hits))
	}
}

// TestLookupSymbolStem_EmptyGraphReturnsNil — no symbols → no hits.
func TestLookupSymbolStem_EmptyGraphReturnsNil(t *testing.T) {
	g := fakeGraphForStem(nil)
	r := newRepomapSymbolResolver(g)
	stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
	if hits := stemR.LookupSymbolStem("AnalyzerAgent"); len(hits) != 0 {
		t.Errorf("empty graph → nil; got %d hits", len(hits))
	}
}

// TestLookupSymbolStem_MultiHit — when stem matches multiple
// graph symbols, return up to maxSymbolResolverHits.
func TestLookupSymbolStem_MultiHit(t *testing.T) {
	g := fakeGraphForStem(map[string]string{
		"analyzerEvaluator":  "internal/agent/analyzer.go",
		"analyzerHelper":     "internal/agent/analyzer_helper.go",
		"runAnalyzer":        "internal/runner.go",
	})
	r := newRepomapSymbolResolver(g)
	stemR := r.(interface{ LookupSymbolStem(string) []normalizer.SymbolHit })
	hits := stemR.LookupSymbolStem("AnalyzerAgent")
	if len(hits) < 2 {
		t.Errorf("multi-symbol stem-match should return ≥2 hits; got %d", len(hits))
	}
}

// TestStripRoleSuffix_TableInvariants pins the role-suffix
// stripper across the canonical suffix list.
func TestStripRoleSuffix_TableInvariants(t *testing.T) {
	cases := []struct {
		in   string
		want string
		note string
	}{
		{"AnalyzerAgent", "Analyzer", "Agent suffix"},
		{"RequestHandler", "Request", "Handler suffix"},
		{"ConfigManager", "Config", "Manager suffix"},
		{"AuthService", "Auth", "Service suffix"},
		{"JobWorker", "Job", "Worker suffix"},
		{"UserController", "User", "Controller suffix"},
		{"EventDispatcher", "Event", "Dispatcher suffix"},
		{"DataProcessor", "Data", "Processor suffix"},
		{"TestRunner", "Test", "Runner suffix"},
		{"PlainNoun", "", "no role suffix → empty"},
		{"agent_handler", "agent", "snake separator trimmed"},
		{"agent-handler", "agent", "kebab separator trimmed"},
		{"", "", "empty in → empty out"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := stripRoleSuffix(tc.in)
			if got != tc.want {
				t.Errorf("stripRoleSuffix(%q) = %q, want %q\n  note: %s",
					tc.in, got, tc.want, tc.note)
			}
		})
	}
}

