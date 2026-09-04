package tracediag

import (
	"bytes"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// V11-2 (colleague_merge_audit §40.58): the LLM trace_query tool gained a
// typed OR-set `patterns` carrier (702dbf277) while the deterministic tracediag
// Step kept rejecting it as an unknown key. The script face now mirrors the
// tool face through the SAME engine normalizer (trim, case-insensitive dedupe,
// view gate, ≤ EventSearchPatternLimit, trimmed-empty rejection).
func TestParseScriptPatternsTypedORCarrier(t *testing.T) {
	for _, version := range []string{"1", "2"} {
		yamlText := "version: " + version + "\nsteps:\n  - {label: alts, view: event_search, patterns: [VerifyClass, \" jit \", verifyclass, JIT], max_lines: 20}\n"
		script, err := ParseScript([]byte(yamlText))
		if err != nil {
			t.Fatalf("v%s typed patterns must parse: %v", version, err)
		}
		if got := script.Steps[0].Patterns; !reflect.DeepEqual(got, []string{"VerifyClass", "jit"}) {
			t.Errorf("v%s Step.Patterns = %q, want trimmed + case-insensitively de-duplicated [VerifyClass jit]", version, got)
		}
	}
	tooMany := make([]string, 0, tracequery.EventSearchPatternLimit+1)
	for i := 0; i <= tracequery.EventSearchPatternLimit; i++ {
		tooMany = append(tooMany, "lit"+string(rune('a'+i)))
	}
	rejects := map[string]struct{ view, patterns, want string }{
		"wrong engine view": {"window_stats", "[VerifyClass]", "only valid for view=event_search"},
		"census view":       {ViewFormatCensus, "[VerifyClass]", "only valid for view=event_search"},
		"empty literal":     {"event_search", `[" "]`, "empty after trimming"},
		"over limit":        {"event_search", "[" + strings.Join(tooMany, ", ") + "]", "maximum is 16"},
	}
	for name, tc := range rejects {
		yamlText := "version: 1\nsteps:\n  - {label: alts, view: " + tc.view + ", patterns: " + tc.patterns + ", max_lines: 20}\n"
		_, err := ParseScript([]byte(yamlText))
		if err == nil {
			t.Errorf("%s: view=%s patterns=%s must fail loud, was accepted silently", name, tc.view, tc.patterns)
			continue
		}
		for _, want := range []string{"steps[0] (alts): patterns:", tc.want} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q must carry %q", name, err.Error(), want)
			}
		}
	}
	// Decoder-remap hint (R2' spot 5): the unknown-key rejection names the
	// step filter roster including the new field, so a mistyped key re-aims.
	_, err := ParseScript([]byte("version: 1\nsteps:\n  - {label: a, view: event_search, pattern_list: [x]}\n"))
	if err == nil || !strings.Contains(err.Error(), "pattern/patterns/event_types/trace_mark_actions") {
		t.Fatalf("decode hint must list pattern/patterns/event_types/trace_mark_actions, got %v", err)
	}
}

// stepQuery carries the validated set into the engine query (both the
// streaming and the composite-indexed event_search branches consume
// Query.Patterns) and never aliases the step's backing array.
func TestStepQueryCarriesPatternsWithoutAliasing(t *testing.T) {
	step := &Step{View: "event_search", Patterns: []string{"VerifyClass", "JIT"}}
	q := stepQuery(step, tracequery.TraceFlavorAuto)
	if !reflect.DeepEqual(q.Patterns, []string{"VerifyClass", "JIT"}) {
		t.Fatalf("Query.Patterns = %q", q.Patterns)
	}
	q.Patterns[0] = "mutated"
	if step.Patterns[0] != "VerifyClass" {
		t.Fatalf("stepQuery must copy Patterns; step mutated to %q", step.Patterns)
	}
	if q := stepQuery(&Step{View: "event_search"}, tracequery.TraceFlavorAuto); q.Patterns != nil {
		t.Fatalf("absent carrier must stay nil, got %q", q.Patterns)
	}
}

// End to end on the streaming event_search path: the report echoes the typed
// set in list form and the engine applies OR semantics (2 of 3 rows).
func TestRunPatternsTypedORCarrierEchoAndRows(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - label: alts
    view: event_search
    window: "0.9..1.1"
    patterns: [VerifyClass, JIT]
    event_types: [trace_mark]
    max_lines: 30
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	trace := "app-20 (20) [001] .... 1.000000: tracing_mark_write: B|20|VerifyClass Demo\n" +
		"app-20 (20) [001] .... 1.001000: tracing_mark_write: E|20\n" +
		"jit-30 (30) [002] .... 1.002000: tracing_mark_write: B|30|JIT compile Demo\n"
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run(patterns): failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		`patterns=["VerifyClass","JIT"]`,
		"matched=2 emitted=2",
		"B|20|VerifyClass Demo",
		"B|30|JIT compile Demo",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("patterns report missing %q\n%s", want, report)
		}
	}
	if strings.Contains(report, "E|20") {
		t.Errorf("patterns report admitted a row outside the OR set\n%s", report)
	}
}

// R2' 第 7 处 on the PARAMETER face: every yaml-tagged struct the strict
// decoder accepts is fingerprinted, and the closure walk from Script fails
// loud on a struct that is reachable but not pinned (unrecognized shape).
func TestStepParamSchemaPinned(t *testing.T) {
	types := make([]reflectTypeName, 0, len(stepParamSchemaPins))
	for typ := range stepParamSchemaPins {
		types = append(types, reflectTypeName{name: typ.PkgPath() + "." + typ.Name(), typ: typ})
	}
	sort.Slice(types, func(i, j int) bool { return types[i].name < types[j].name })
	for _, item := range types {
		got, schema := paramSchemaFingerprint(item.typ)
		want := stepParamSchemaPins[item.typ]
		if got != want {
			t.Errorf("%s parameter-face drift (review stepQuery / stepParamsEcho / decode hint / validateStep / architecture.md §13.7 / internal/tool cross-face census, then re-pin with a comment): got=%s want=%s\ncurrent_schema=%s", item.name, got, want, schema)
		}
	}
	reachable, problems := yamlDecodedStructClosure(reflect.TypeOf(Script{}))
	for _, problem := range problems {
		t.Errorf("Script yaml closure: %s", problem)
	}
	for _, typ := range reachable {
		if _, pinned := stepParamSchemaPins[typ]; !pinned {
			t.Errorf("%s is reachable from the strict-decoded Script but has no parameter-face pin; add it to stepParamSchemaPins", typ.Name())
		}
	}
	seen := map[reflect.Type]bool{}
	for _, typ := range reachable {
		seen[typ] = true
	}
	for typ := range stepParamSchemaPins {
		if !seen[typ] {
			t.Errorf("%s is pinned but not reachable from Script's yaml face (stale pin)", typ.Name())
		}
	}
}

// yamlDecodedStructClosure walks every struct yaml.v3 would decode into
// starting at root, following exactly the decoder's rules through
// YAMLFieldKey (untagged exported fields under their lowercased name,
// `yaml:"-"` skipped, `,inline` flattened) and through every container the
// decoder unwraps (pointer, slice, array, map key and element). Shapes the
// pin cannot fingerprint — an interface field (open schema) or a struct
// from another package (its fields are not this package's yaml face) — are
// reported as problems so the pin fails loud instead of walking past them
// (§40.50). The returned order is deterministic (discovery order).
func yamlDecodedStructClosure(root reflect.Type) (reachable []reflect.Type, problems []string) {
	pkg := root.PkgPath()
	seen := map[reflect.Type]bool{}
	var walk func(typ reflect.Type, path string)
	walk = func(typ reflect.Type, path string) {
		for {
			switch typ.Kind() {
			case reflect.Ptr, reflect.Slice, reflect.Array:
				typ = typ.Elem()
				continue
			case reflect.Map:
				walk(typ.Key(), path+"[key]")
				typ = typ.Elem()
				continue
			}
			break
		}
		switch typ.Kind() {
		case reflect.Interface:
			problems = append(problems, path+": interface-typed field is an open yaml schema the parameter-face pin cannot fingerprint")
			return
		case reflect.Struct:
		default:
			return
		}
		if typ.PkgPath() != pkg {
			problems = append(problems, path+": "+typ.String()+" is a struct from another package reachable through the yaml face; mirror it locally or pin it explicitly")
			return
		}
		if seen[typ] {
			return
		}
		seen[typ] = true
		reachable = append(reachable, typ)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			key, decoded, inline := YAMLFieldKey(field)
			if !decoded {
				continue
			}
			label := key
			if inline {
				label = "<inline " + field.Name + ">"
			}
			walk(field.Type, path+"."+label)
		}
	}
	walk(root, root.Name())
	return reachable, problems
}

// Self-red per shape for the closure walk (G6-tracediag #1): each shape
// below is one yaml.v3 decodes but the pre-fix walk skipped.
func TestYAMLDecodedStructClosureSelfRed(t *testing.T) {
	type nestedUntagged struct {
		Foo int
	}
	type nestedMapElem struct {
		Bar string `yaml:"bar"`
	}
	type nestedArrayElem struct {
		Baz string `yaml:"baz"`
	}
	type nestedMapKey struct {
		K string
	}
	type nestedDashed struct {
		Hidden int
	}
	type nestedInline struct {
		Qux string `yaml:"qux"`
	}
	type openField struct {
		Any interface{} `yaml:"any"`
	}
	type probeRoot struct {
		Extra    nestedUntagged           // untagged exported: decoded under "extra"
		Elems    map[string]nestedMapElem `yaml:"elems"`
		Keys     map[nestedMapKey]int     `yaml:"keys"`
		Arr      [2]nestedArrayElem       `yaml:"arr"`
		Dashed   nestedDashed             `yaml:"-"`
		Inlined  nestedInline             `yaml:",inline"`
		Foreign  tracequery.Query         `yaml:"foreign"`
		Open     openField                `yaml:"open"`
		hidden   nestedDashed             //nolint:unused — unexported, never decoded
		Untagged map[string][]nestedUntagged
	}
	_ = probeRoot{}.hidden
	reachable, problems := yamlDecodedStructClosure(reflect.TypeOf(probeRoot{}))
	names := map[string]bool{}
	for _, typ := range reachable {
		names[typ.Name()] = true
	}
	for _, want := range []string{"probeRoot", "nestedUntagged", "nestedMapElem", "nestedMapKey", "nestedArrayElem", "nestedInline", "openField"} {
		if !names[want] {
			t.Errorf("%s must be reachable through the decoder's rules; reachable=%v", want, names)
		}
	}
	if names["nestedDashed"] {
		t.Errorf("yaml:\"-\" and unexported fields are never decoded; reachable=%v", names)
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"probeRoot.open.any: interface-typed field", "probeRoot.foreign: tracequery.Query is a struct from another package"} {
		if !strings.Contains(joined, want) {
			t.Errorf("closure walk must fail loud on %q, got:\n%s", want, joined)
		}
	}
	if len(problems) != 2 {
		t.Errorf("problems=%d, want exactly the two unpinnable shapes:\n%s", len(problems), joined)
	}
	// The key rule itself: default lowercased name, tag name, dash, inline.
	for name, want := range map[string]struct {
		key             string
		decoded, inline bool
	}{
		"Extra":    {"extra", true, false},
		"Elems":    {"elems", true, false},
		"Dashed":   {"", false, false},
		"Inlined":  {"", true, true},
		"hidden":   {"", false, false},
		"Untagged": {"untagged", true, false},
	} {
		field, _ := reflect.TypeOf(probeRoot{}).FieldByName(name)
		key, decoded, inline := YAMLFieldKey(field)
		if key != want.key || decoded != want.decoded || inline != want.inline {
			t.Errorf("YAMLFieldKey(%s) = (%q,%v,%v), want (%q,%v,%v)", name, key, decoded, inline, want.key, want.decoded, want.inline)
		}
	}
}
