package tool_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// strict_decode_registry_test.go — structural invariant for the
// unified structured-payload repair layer (L2: per-tool strict decode).
//
// Walks every tool in the production registry and asserts its params
// decode is either strict-decode wired (DisallowUnknownFields is
// reachable from the tool's Execute method, so unknown fields become a
// typed same-dispatch rejection) or explicitly listed in the documented
// exemption table below. Onboarding is an invariant, not an audit: a
// new tool registered without strict decode and without an exemption
// entry fails this test.

// strictDecodeExemptTools documents every registry tool whose params
// decode is still lenient (unknown fields silently dropped, no typed
// ToolRepair rejection). Entries are an explicit onboarding BACKLOG for
// the repair layer, seeded 2026-06-12 when repo_map was onboarded —
// they are not approvals. Wire a tool (see trace_query / repo_map for
// the reference shape: shared structured-payload compat, then
// DisallowUnknownFields, then the typed strict-decode failure path) and
// remove its entry here.
var strictDecodeExemptTools = map[string]string{
	"git_diff":                    "lenient decode; not yet onboarded",
	"git_show":                    "lenient decode; not yet onboarded",
	"git_log":                     "lenient decode; not yet onboarded",
	"git_history_search":          "lenient decode; not yet onboarded",
	"emit_multi_repo_focus":       "lenient decode; not yet onboarded",
	"emit_analysis":               "lenient decode with bespoke local-model normalization; not yet onboarded",
	"recall_memory":               "lenient decode; not yet onboarded",
	"list_memory":                 "lenient decode; not yet onboarded",
	"propose_sub_agents":          "lenient decode; not yet onboarded",
	"emit_investigation_complete": "lenient decode; not yet onboarded",
	"emit_log_triage":             "lenient decode; not yet onboarded",
	"emit_log_segmentation":       "lenient decode; not yet onboarded",
	"emit_perf_segmentation":      "lenient decode; not yet onboarded",
}

// buildProductionToolRegistry mirrors the production registry:
// tool.RegisterDefaults plus the runtime registrations in cmd/root.go.
// TestStrictDecodeRegistryMirrorsProductionSites pins the mirror
// against the registration call sites in source so it cannot drift.
func buildProductionToolRegistry() *tool.Registry {
	r := tool.NewRegistry()
	tool.RegisterDefaults(r)
	r.Register(&repomap.RepoMapV2{})
	r.Register(tool.NewProposeSubAgents())
	r.Register(&tool.EmitEvidence{})
	r.Register(&tool.EmitInvestigationComplete{})
	r.Register(&tool.EmitAnswerSymbol{})
	r.Register(&tool.EmitHypothesisVerdict{})
	r.Register(&tool.EmitAnswerDocument{})
	r.Register(&tool.EmitAnswerDocumentPatch{})
	r.Register(&tool.EmitLogTriage{})
	r.Register(&tool.EmitLogSegmentation{})
	r.Register(&tool.EmitPerfTrace{})
	r.Register(&tool.EmitPerfSegmentation{})
	r.Register(&tool.EmitWriteAnalysis{})
	r.Register(&tool.EmitWriteWorkflowDecision{})
	return r
}

func strictDecodeTestPaths(t *testing.T) (toolDir, repomapDir, defaultsGo, rootGo string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	toolDir = filepath.Dir(file)
	repomapDir = filepath.Join(toolDir, "repomap")
	defaultsGo = filepath.Join(toolDir, "defaults.go")
	rootGo = filepath.Join(toolDir, "..", "..", "cmd", "root.go")
	return toolDir, repomapDir, defaultsGo, rootGo
}

// TestStrictDecodeRegistryMirrorsProductionSites pins the test-local
// registry mirror against the registration call sites in source: when
// cmd/root.go or RegisterDefaults gains or loses a tool, this fails so
// the mirror (and the wiring classification below) must be updated.
func TestStrictDecodeRegistryMirrorsProductionSites(t *testing.T) {
	_, _, defaultsGo, rootGo := strictDecodeTestPaths(t)
	defaultsSrc, err := os.ReadFile(defaultsGo)
	if err != nil {
		t.Fatalf("read defaults.go: %v", err)
	}
	rootSrc, err := os.ReadFile(rootGo)
	if err != nil {
		t.Fatalf("read cmd/root.go: %v", err)
	}
	sites := strings.Count(string(defaultsSrc), "r.Register(") +
		strings.Count(string(rootSrc), "toolRegistry.Register(")
	reg := buildProductionToolRegistry()
	if got := len(reg.List()); got != sites {
		t.Fatalf("registry mirror drift: %d production Register call sites, %d tools in test mirror — update buildProductionToolRegistry and classify the new tool (strict decode or exemption entry)", sites, got)
	}
}

func TestEveryRegistryToolIsStrictDecodeWiredOrExempt(t *testing.T) {
	toolDir, repomapDir, _, _ := strictDecodeTestPaths(t)
	idx := loadStrictDecodeFuncIndex(t, []string{toolDir, repomapDir})

	reg := buildProductionToolRegistry()
	names := reg.List()
	if len(names) == 0 {
		t.Fatal("empty registry")
	}
	for _, name := range names {
		tl, err := reg.Get(name)
		if err != nil {
			t.Fatalf("registry get %q: %v", name, err)
		}
		rt := reflect.TypeOf(tl)
		for rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		exec, ok := idx.execByRecv[rt.Name()]
		if !ok {
			t.Errorf("tool %q (type %s): no Execute method found in scanned sources — extend the scan dirs in this test", name, rt.Name())
			continue
		}
		wired := idx.markerReachable(exec, map[*strictDecodeFunc]bool{})
		_, exempt := strictDecodeExemptTools[name]
		switch {
		case wired && exempt:
			t.Errorf("tool %q is strict-decode wired but still listed in strictDecodeExemptTools — remove the stale exemption entry", name)
		case !wired && !exempt:
			t.Errorf("tool %q params decode is not strict-decode wired and not in the documented exemption table — onboard it to the repair layer (DisallowUnknownFields + typed rejection, see trace_query / repo_map) or add a documented exemption entry", name)
		}
	}

	registered := map[string]bool{}
	for _, name := range names {
		registered[name] = true
	}
	for name := range strictDecodeExemptTools {
		if !registered[name] {
			t.Errorf("strictDecodeExemptTools lists %q which is not a registered tool — remove the stale entry", name)
		}
	}
}

type strictDecodeFunc struct {
	name      string
	hasMarker bool
	callees   map[string]bool
}

type strictDecodeFuncIndex struct {
	byName     map[string][]*strictDecodeFunc
	execByRecv map[string]*strictDecodeFunc
}

func (idx *strictDecodeFuncIndex) markerReachable(fn *strictDecodeFunc, visited map[*strictDecodeFunc]bool) bool {
	if visited[fn] {
		return false
	}
	visited[fn] = true
	if fn.hasMarker {
		return true
	}
	for callee := range fn.callees {
		for _, target := range idx.byName[callee] {
			if idx.markerReachable(target, visited) {
				return true
			}
		}
	}
	return false
}

// loadStrictDecodeFuncIndex parses every non-test Go file in dirs and
// indexes top-level functions/methods by name, recording whether each
// body contains the strict-decode marker (DisallowUnknownFields) and
// which function names it calls, so marker reachability can follow
// in-package decode helpers (e.g. an Execute that delegates decoding).
func loadStrictDecodeFuncIndex(t *testing.T, dirs []string) *strictDecodeFuncIndex {
	t.Helper()
	idx := &strictDecodeFuncIndex{
		byName:     map[string][]*strictDecodeFunc{},
		execByRecv: map[string]*strictDecodeFunc{},
	}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			parsed, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, decl := range parsed.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				start := fset.Position(fd.Body.Pos()).Offset
				end := fset.Position(fd.Body.End()).Offset
				if start < 0 || end > len(src) || start >= end {
					continue
				}
				fn := &strictDecodeFunc{
					name:      fd.Name.Name,
					hasMarker: strings.Contains(string(src[start:end]), "DisallowUnknownFields"),
					callees:   map[string]bool{},
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch fun := call.Fun.(type) {
					case *ast.Ident:
						fn.callees[fun.Name] = true
					case *ast.SelectorExpr:
						fn.callees[fun.Sel.Name] = true
					}
					return true
				})
				idx.byName[fn.name] = append(idx.byName[fn.name], fn)
				if fd.Name.Name == "Execute" && fd.Recv != nil && len(fd.Recv.List) == 1 {
					if recv := strictDecodeRecvTypeName(fd.Recv.List[0].Type); recv != "" {
						idx.execByRecv[recv] = fn
					}
				}
			}
		}
	}
	return idx
}

func strictDecodeRecvTypeName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.StarExpr:
		return strictDecodeRecvTypeName(v.X)
	case *ast.Ident:
		return v.Name
	default:
		return ""
	}
}
