package tool

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracediag"
)

// Cross-face census (V11-2, colleague_merge_audit §40.58): the deterministic
// tracediag Step mirrors the LLM trace_query tool's event_search filter face
// so a customer script can replay any LLM-lane search. Commit 702dbf277 added
// `patterns` to the tool + engine only; nothing tripped because the only
// schema pins covered RESULT carriers. This census walks the tool's live
// JSON-schema properties against tracediag.Step's yaml tags with a closed
// two-roster decision (mirror-or-exempt) and fails loud on any property it
// does not recognize (§40.36/§40.50 census discipline).
//
// The direction tool → tracediag is the legal one (tracediag's import
// boundary forbids importing internal/tool; cmd/tracediag.go already imports
// both). INTENDED: this test goes red on every future trace_query schema
// property — the fix is a deliberate mirror-or-exempt entry with a review
// comment, never a blind roster widening.

// traceQueryTraceDiagStepMirror: tool property → tracediag Step yaml tag.
// time_start/time_end collapse into the script's single `window` token and
// the tool's inline row cap `limit` is the script's `max_lines` body cap.
var traceQueryTraceDiagStepMirror = map[string]string{
	"view":               "view",
	"pid":                "pid",
	"thread":             "thread",
	"time_start":         "window",
	"time_end":           "window",
	"line_start":         "line_start",
	"line_end":           "line_end",
	"pattern":            "pattern",
	"patterns":           "patterns",
	"event_types":        "event_types",
	"trace_mark_actions": "trace_mark_actions",
	"limit":              "max_lines",
}

// traceQueryToolOnlyParams: schema properties that by design have no script
// field — artifact selection is a CLI flag (--trace / --trace-flavor), and
// the rest belong to views tracediag drives without a per-step knob (span/
// frame discovery scope, wakeup budgets, recipe/interaction selectors,
// window_sweep bucketing, compute-supply topology).
var traceQueryToolOnlyParams = map[string]bool{
	"source":                true,
	"path":                  true,
	"trace_flavor":          true,
	"platform":              true,
	"target_scope":          true,
	"span_name":             true,
	"interaction_direction": true,
	"recipe_name":           true,
	"max_depth":             true,
	"max_branches":          true,
	"max_chain_nodes":       true,
	"via_thread":            true,
	"min_duration_ms":       true,
	"include_window_stats":  true,
	"core_topology":         true,
	"bucket_ms":             true,
}

// traceQueryTraceDiagMirrorViolations is the pure census core: every shape it
// can reject is exercised by the self-red subtests below.
func traceQueryTraceDiagMirrorViolations(props []string, stepTags map[string]bool, mirror map[string]string, exempt map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, prop := range props {
		seen[prop] = true
		tag, mirrored := mirror[prop]
		switch {
		case mirrored && exempt[prop]:
			out = append(out, "ambiguous roster: "+prop+" is both mirrored and tool-only")
		case mirrored && !stepTags[tag]:
			out = append(out, "missing step field: tool property "+prop+" is declared mirrored onto Step yaml:\""+tag+"\" but no such tag exists")
		case !mirrored && !exempt[prop]:
			out = append(out, "unrecognized tool property "+prop+": add it to traceQueryTraceDiagStepMirror (and sync tracediag Step/stepQuery/stepParamsEcho/decode hint/docs) or to traceQueryToolOnlyParams with a reason")
		}
	}
	for prop := range mirror {
		if !seen[prop] {
			out = append(out, "stale mirror entry: "+prop+" is no longer a tool property")
		}
	}
	for prop := range exempt {
		if !seen[prop] {
			out = append(out, "stale tool-only entry: "+prop+" is no longer a tool property")
		}
	}
	sort.Strings(out)
	return out
}

func traceQuerySchemaProperties(t *testing.T) []string {
	t.Helper()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) == 0 {
		t.Fatal("trace_query schema has no properties — census would be vacuous")
	}
	props := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		props = append(props, name)
	}
	sort.Strings(props)
	return props
}

// traceDiagStepYAMLTags is the set of keys the strict yaml decoder accepts
// on a Step, derived through tracediag.YAMLFieldKey (the decoder's own
// rule: untagged exported fields decode under their lowercased name) rather
// than tag presence, so an untagged field cannot slip past the mirror.
func traceDiagStepYAMLTags() map[string]bool {
	typ := reflect.TypeOf(tracediag.Step{})
	tags := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		key, decoded, inline := tracediag.YAMLFieldKey(typ.Field(i))
		if !decoded || inline {
			continue
		}
		tags[key] = true
	}
	return tags
}

func TestTraceQueryEventSearchParamsMirroredInTraceDiagStep(t *testing.T) {
	props := traceQuerySchemaProperties(t)
	tags := traceDiagStepYAMLTags()
	if len(tags) == 0 {
		t.Fatal("tracediag.Step exposes no yaml tags — census would be vacuous")
	}
	if violations := traceQueryTraceDiagMirrorViolations(props, tags, traceQueryTraceDiagStepMirror, traceQueryToolOnlyParams); len(violations) > 0 {
		t.Fatalf("trace_query ↔ tracediag Step parameter-face drift:\n  %s", strings.Join(violations, "\n  "))
	}
}

// Self-red per shape: the census core must flag every drift class it claims
// to cover, including the exact 702dbf277 shape (a new tool property that no
// roster names) against the LIVE schema and Step.
func TestTraceQueryTraceDiagMirrorCensusSelfRed(t *testing.T) {
	props := traceQuerySchemaProperties(t)
	tags := traceDiagStepYAMLTags()
	withoutPatterns := map[string]string{}
	for k, v := range traceQueryTraceDiagStepMirror {
		if k != "patterns" {
			withoutPatterns[k] = v
		}
	}
	missingTag := map[string]string{}
	for k, v := range traceQueryTraceDiagStepMirror {
		missingTag[k] = v
	}
	missingTag["patterns"] = "patterns_v2"
	staleMirror := map[string]string{}
	for k, v := range traceQueryTraceDiagStepMirror {
		staleMirror[k] = v
	}
	staleMirror["retired_param"] = "pattern"
	staleExempt := map[string]bool{}
	ambiguous := map[string]bool{}
	for k := range traceQueryToolOnlyParams {
		staleExempt[k] = true
		ambiguous[k] = true
	}
	staleExempt["retired_param"] = true
	ambiguous["patterns"] = true
	for name, tc := range map[string]struct {
		props  []string
		mirror map[string]string
		exempt map[string]bool
		want   string
	}{
		"702dbf277 shape: new property in no roster": {props, withoutPatterns, traceQueryToolOnlyParams, "unrecognized tool property patterns"},
		"mirror names a tag Step lacks":              {props, missingTag, traceQueryToolOnlyParams, "missing step field: tool property patterns"},
		"stale mirror entry":                         {props, staleMirror, traceQueryToolOnlyParams, "stale mirror entry: retired_param"},
		"stale tool-only entry":                      {props, traceQueryTraceDiagStepMirror, staleExempt, "stale tool-only entry: retired_param"},
		"property in both rosters":                   {props, traceQueryTraceDiagStepMirror, ambiguous, "ambiguous roster: patterns"},
	} {
		violations := traceQueryTraceDiagMirrorViolations(tc.props, tags, tc.mirror, tc.exempt)
		if len(violations) != 1 || !strings.Contains(violations[0], tc.want) {
			t.Errorf("%s: census must flag exactly this shape (%q), got %q", name, tc.want, violations)
		}
	}
}
