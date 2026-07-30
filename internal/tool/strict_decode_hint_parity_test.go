package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// strict_decode_hint_parity_test.go — EVALFIX-2A Tripwire C. The
// MisplacedFieldHint tables are the typed ledger of KNOWN recurring
// decode near-misses; each wrong-NAME row (CanonicalName set) makes a
// repair-time promise that must have a prompt-time counterpart: the
// canonical field's schema description pre-teaches the wrong token so
// the model never emits it in the first place. The table universe is
// AST-scanned from source (never hand-copied), the test roster maps
// each table to its tool's schema, and both directions are checked so
// a NEW hint table cannot dodge the obligation silently.

// strictDecodeHintSchemaExemptions — wrong-NAME rows whose token is
// deliberately NOT pre-taught in the schema (typical ruling:
// pre-teaching the wrong token would prime the model to emit it). Key
// is "<table var>:<Field>"; rationale must name the ruling. Stale
// entries are a red test.
var strictDecodeHintSchemaExemptions = map[string]string{}

// strictDecodeHintTableRoster maps every package-level
// []MisplacedFieldHint var to its table value and the schema of the
// tool that consumes it. The AST census below keeps this roster
// honest in both directions.
var strictDecodeHintTableRoster = map[string]struct {
	hints  []MisplacedFieldHint
	schema json.RawMessage
}{
	"answerDocumentV2MisplacedHints": {answerDocumentV2MisplacedHints, (&EmitAnswerDocument{}).Parameters()},
	"emitEvidenceMisplacedHints":     {emitEvidenceMisplacedHints, (&EmitEvidence{}).Parameters()},
	"emitPerfTraceMisplacedHints":    {emitPerfTraceMisplacedHints, (&EmitPerfTrace{}).Parameters()},
	"emitAnalysisMisplacedHints":     {emitAnalysisMisplacedHints, (&EmitAnalysis{}).Parameters()},
}

func TestEveryWrongNameHintIsPreTaughtInSchemaOrExempted(t *testing.T) {
	universe := misplacedFieldHintTableVarsFromSource(t)
	if len(universe) == 0 {
		t.Fatal("no []MisplacedFieldHint tables found in source — the census scan is broken and the tripwire vacuous")
	}
	for _, name := range universe {
		if _, ok := strictDecodeHintTableRoster[name]; !ok {
			t.Errorf("hint table %q exists in source but is missing from strictDecodeHintTableRoster — add it so its wrong-NAME rows carry the schema pre-teach obligation", name)
		}
	}
	inUniverse := map[string]bool{}
	for _, name := range universe {
		inUniverse[name] = true
	}
	for name := range strictDecodeHintTableRoster {
		if !inUniverse[name] {
			t.Errorf("roster entry %q matches no package-level []MisplacedFieldHint var — remove the stale entry", name)
		}
	}
	wrongNameRows := 0
	seenExemptionKeys := map[string]bool{}
	for name, entry := range strictDecodeHintTableRoster {
		for _, hint := range entry.hints {
			if hint.CanonicalName == "" {
				continue
			}
			wrongNameRows++
			key := name + ":" + hint.Field
			seenExemptionKeys[key] = true
			description, found := schemaPropertyDescription(t, entry.schema, hint.CanonicalName)
			if !found {
				t.Errorf("hint table %s row %q names canonical field %q which has no property description in the tool schema", name, hint.Field, hint.CanonicalName)
				continue
			}
			taught := strings.Contains(description, hint.Field)
			rationale, exempted := strictDecodeHintSchemaExemptions[key]
			switch {
			case exempted && strings.TrimSpace(rationale) == "":
				t.Errorf("exemption %q has no rationale", key)
			case exempted && taught:
				t.Errorf("hint row %q is exempted AND its wrong token appears in the %q schema description — remove the stale exemption", key, hint.CanonicalName)
			case !exempted && !taught:
				t.Errorf("hint table %s row: canonical field %q schema description does not pre-teach the wrong token %q — add the disambiguation sentence to the description or declare an exemption with a rationale", name, hint.CanonicalName, hint.Field)
			}
		}
	}
	if wrongNameRows == 0 {
		t.Fatal("no wrong-NAME (CanonicalName) hint rows exist — the parity assertion is vacuous; if the last row was retired, retire this tripwire's Fatal alongside it")
	}
	for key := range strictDecodeHintSchemaExemptions {
		if !seenExemptionKeys[key] {
			t.Errorf("exemption %q matches no wrong-NAME hint row — remove the stale entry", key)
		}
	}
}

// misplacedFieldHintTableVarsFromSource AST-scans every non-test file
// in this package for package-level `var X = []MisplacedFieldHint{...}`
// declarations — the closed table universe.
func misplacedFieldHintTableVarsFromSource(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, value := range vs.Values {
					lit, ok := value.(*ast.CompositeLit)
					if !ok {
						continue
					}
					arr, ok := lit.Type.(*ast.ArrayType)
					if !ok {
						continue
					}
					ident, ok := arr.Elt.(*ast.Ident)
					if !ok || ident.Name != "MisplacedFieldHint" || i >= len(vs.Names) {
						continue
					}
					out = append(out, vs.Names[i].Name)
				}
			}
		}
	}
	return out
}

// schemaPropertyDescription walks the schema JSON for a property named
// field under any "properties" object and returns its description.
func schemaPropertyDescription(t *testing.T, schema json.RawMessage, field string) (string, bool) {
	t.Helper()
	var parsed any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("unmarshal tool schema: %v", err)
	}
	var walk func(node any) (string, bool)
	walk = func(node any) (string, bool) {
		obj, ok := node.(map[string]any)
		if !ok {
			if arr, ok := node.([]any); ok {
				for _, child := range arr {
					if desc, found := walk(child); found {
						return desc, true
					}
				}
			}
			return "", false
		}
		if props, ok := obj["properties"].(map[string]any); ok {
			if target, ok := props[field].(map[string]any); ok {
				desc, _ := target["description"].(string)
				return desc, true
			}
		}
		for _, child := range obj {
			if desc, found := walk(child); found {
				return desc, true
			}
		}
		return "", false
	}
	return walk(parsed)
}
