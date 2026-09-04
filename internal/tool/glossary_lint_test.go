package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// llmFacingToolRoster is the single roster of every tool whose
// Description() / Parameters() the model sees. It is the ONE list the
// schema jargon gate walks (§40.52 merged the former two hand-written
// rosters — 8 emit tools in this file, 17 tools in prompt_hygiene_test
// — which between them had silently missed EmitMultiRepoFocus).
// TestToolRosterCensus pins it against the source: every type in
// internal/tool that declares `Parameters() json.RawMessage` must be
// instantiated here, and nothing here may lack that method.
var llmFacingToolRoster = []Tool{
	&ExecCommand{},
	&GrepTool{},
	&TraceQuery{},
	&ReadFile{},
	&ListFiles{},
	&GitDiff{},
	&GitShow{},
	&GitLog{},
	&GitHistorySearch{},
	&ApplyPatch{},
	&RunTests{},
	&RecallMemory{},
	&ListMemory{},
	NewProposeSubAgents(),
	&EmitAnalysis{},
	&EmitEvidence{},
	&EmitInvestigationComplete{},
	&EmitAnswerSymbol{},
	&EmitHypothesisVerdict{},
	&EmitAnswerDocument{},
	&EmitAnswerDocumentPatch{},
	&EmitLogTriage{},
	&EmitLogSegmentation{},
	&EmitPerfTrace{},
	&EmitPerfSegmentation{},
	&EmitChangePlan{},
	&EmitPlanSkeleton{},
	&EmitPlanChange{},
	&EmitTestResults{},
	&EmitWriteAnalysis{},
	&EmitWriteWorkflowDecision{},
	&EmitMultiRepoFocus{},
}

// TestNoInternalTermsInToolSchemas scans every rostered tool's
// Description() and its full Parameters() JSON — both the structured
// "description" values (walked recursively so descriptions nested
// inside properties / items / anyOf / oneOf are labelled by path) and
// the raw schema bytes (enum values, titles, anything else the model
// reads) — for glossary tokens through the shared scanner.
//
// Batch 1 policy: report-only (t.Log). Batch 2B cleaned the tool-schema
// surfaces and batch 4B promoted the gate to t.Fatal. §40.52 made it
// the single tool-schema entry over llmFacingToolRoster.
func TestNoInternalTermsInToolSchemas(t *testing.T) {
	var hits []glossarylint.Hit
	for _, tl := range llmFacingToolRoster {
		name := tl.Name()
		hits = append(hits, glossarylint.ScanText(name+".Description", tl.Description())...)
		params := tl.Parameters()
		if len(params) == 0 {
			continue
		}
		var decoded any
		if err := json.Unmarshal(params, &decoded); err != nil {
			t.Errorf("%s: Parameters() is not valid JSON: %v", name, err)
			continue
		}
		hits = append(hits, scanJSONDescriptions(name+".Parameters", "", decoded)...)
		hits = append(hits, glossarylint.ScanText(name+".Parameters[raw]", string(params))...)
	}
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoInternalTermsInToolSchemas found %d violation(s); rephrase in user-facing language or extend internal/skill/glossary.go :: InternalTermsBlocklist", len(hits))
}

// TestToolRosterCensus pins llmFacingToolRoster against the package
// source: the set of receiver types declaring `Parameters()
// json.RawMessage` in non-test files directly under internal/tool must
// equal the set of concrete types in the roster.
func TestToolRosterCensus(t *testing.T) {
	declared := parametersReceivers(t, ".")
	rostered := map[string]bool{}
	for _, tl := range llmFacingToolRoster {
		rt := reflect.TypeOf(tl)
		for rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		if rostered[rt.Name()] {
			t.Errorf("roster lists %s twice", rt.Name())
		}
		rostered[rt.Name()] = true
	}
	for name := range declared {
		if !rostered[name] {
			t.Errorf("%s declares Parameters() json.RawMessage but is missing from llmFacingToolRoster", name)
		}
	}
	for name := range rostered {
		if !declared[name] {
			t.Errorf("llmFacingToolRoster lists %s, which declares no Parameters() json.RawMessage in internal/tool", name)
		}
	}
}

// parametersReceivers returns the receiver type names of every
// `func (… *T) Parameters() json.RawMessage` in non-test files under dir.
func parametersReceivers(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Name.Name != "Parameters" || fd.Type.Results == nil || len(fd.Type.Results.List) != 1 {
				continue
			}
			sel, ok := fd.Type.Results.List[0].Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "RawMessage" {
				continue
			}
			recv := fd.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				out[id.Name] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no Parameters() json.RawMessage receivers found under %s — census walker is broken", dir)
	}
	return out
}

// scanJSONDescriptions walks a decoded JSON schema tree and scans
// every value keyed under the literal property name "description"
// against the glossary, labelling hits by their schema path.
func scanJSONDescriptions(root, path string, node any) []glossarylint.Hit {
	var out []glossarylint.Hit
	switch v := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := v[k]
			childPath := path
			if childPath == "" {
				childPath = k
			} else {
				childPath = childPath + "." + k
			}
			if k == "description" {
				if s, ok := child.(string); ok {
					out = append(out, glossarylint.ScanText(root+"."+childPath, s)...)
				}
				continue
			}
			out = append(out, scanJSONDescriptions(root, childPath, child)...)
		}
	case []any:
		for i, child := range v {
			out = append(out, scanJSONDescriptions(root, path+"["+strconv.Itoa(i)+"]", child)...)
		}
	}
	return out
}
