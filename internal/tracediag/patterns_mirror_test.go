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
	pkg := reflect.TypeOf(Script{}).PkgPath()
	seen := map[reflect.Type]bool{}
	var walk func(typ reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ.PkgPath() != pkg || seen[typ] {
			return
		}
		seen[typ] = true
		if _, pinned := stepParamSchemaPins[typ]; !pinned {
			t.Errorf("%s is reachable from the strict-decoded Script but has no parameter-face pin; add it to stepParamSchemaPins", typ.Name())
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" || field.Tag.Get("yaml") == "" {
				continue
			}
			walk(field.Type)
		}
	}
	walk(reflect.TypeOf(Script{}))
	for typ := range stepParamSchemaPins {
		if !seen[typ] {
			t.Errorf("%s is pinned but not reachable from Script's yaml face (stale pin)", typ.Name())
		}
	}
}
