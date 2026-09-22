package tracediag

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestB1713ParseScriptEventFieldFilters(t *testing.T) {
	for _, version := range []int{1, 2} {
		yamlText := fmt.Sprintf(`version: %d
steps:
  - label: jank
    view: event_search
    event_field_filters:
      - {field: jank_frames, op: gte, value: 2}
      - {field: appid, op: eq, value: "30"}
      - {field: start_ts, op: gte, value: 9007199254740993}
`, version)
		script, err := ParseScript([]byte(yamlText))
		if err != nil {
			t.Fatalf("v%d typed field filters rejected: %v", version, err)
		}
		echo := stepParamsEcho(&script.Steps[0])
		for _, want := range []string{`event_field_filters=[`, `"field":"jank_frames"`, `"op":"gte"`, `"value":"2"`, `"value":"9007199254740993"`} {
			if !strings.Contains(echo, want) {
				t.Errorf("v%d exact filter echo lacks %q: %s", version, want, echo)
			}
		}
	}
}

func TestB1713FieldFiltersRejectInvalidYAMLAndSemanticContracts(t *testing.T) {
	cases := map[string]string{
		"unknown_field":     `[{field: arbitrary, op: gte, value: 2}]`,
		"unknown_operator":  `[{field: jank_frames, op: greater, value: 2}]`,
		"unknown_key":       `[{field: jank_frames, op: gte, value: 2, typo: 1}]`,
		"missing_value":     `[{field: jank_frames, op: gte}]`,
		"empty_value":       `[{field: jank_frames, op: gte, value: ""}]`,
		"float":             `[{field: jank_frames, op: gte, value: 2.0}]`,
		"float_tag_integer": `[{field: jank_frames, op: gte, value: !!float 2}]`,
		"float_tag_large":   `[{field: start_ts, op: gte, value: !!float 9007199254740993}]`,
		"exponent":          `[{field: jank_frames, op: gte, value: 2e0}]`,
		"hex":               `[{field: jank_frames, op: gte, value: 0x2}]`,
		"octal":             `[{field: jank_frames, op: gte, value: 0o2}]`,
		"bool":              `[{field: jank_frames, op: gte, value: true}]`,
		"null":              `[{field: jank_frames, op: gte, value: null}]`,
		"sequence":          `[{field: jank_frames, op: gte, value: [2]}]`,
		"mapping":           `[{field: jank_frames, op: gte, value: {v: 2}}]`,
		"overflow":          `[{field: jank_frames, op: gte, value: 9223372036854775808}]`,
	}
	cases["over_limit"] = "[" + strings.Repeat("{field: jank_frames, op: gte, value: 2},", tracequery.EventFieldFilterLimit) + "{field: jank_frames, op: gte, value: 2}]"
	for name, filters := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseScript([]byte("version: 1\nsteps:\n  - label: reject\n    view: event_search\n    event_field_filters: " + filters + "\n"))
			if err == nil {
				t.Fatalf("invalid filter accepted: %s", filters)
			}
		})
	}
	for _, view := range []string{"window_stats", ViewFormatCensus} {
		_, err := ParseScript([]byte("version: 1\nsteps:\n  - label: reject\n    view: " + view + "\n    event_field_filters: [{field: jank_frames, op: gte, value: 2}]\n"))
		if err == nil || !strings.Contains(err.Error(), "only valid for view=event_search") {
			t.Errorf("non-event view %s did not reject through shared engine validator: %v", view, err)
		}
	}
}

func TestB1713StepQueryCopiesEventFieldFilters(t *testing.T) {
	step := &Step{View: "event_search", EventFieldFilters: []EventFieldFilter{{Field: "start_ts", Op: "gte", Value: "9007199254740993"}}}
	query := stepQuery(step, tracequery.TraceFlavorAuto)
	if len(query.EventFieldFilters) != 1 || query.EventFieldFilters[0].Value != "9007199254740993" {
		t.Fatalf("exact native filter missing: %+v", query.EventFieldFilters)
	}
	query.EventFieldFilters[0].Value = "0"
	if step.EventFieldFilters[0].Value != "9007199254740993" {
		t.Fatal("engine query aliases v2 step's predicate list")
	}
	if absent := stepQuery(&Step{View: "event_search"}, tracequery.TraceFlavorAuto); absent.EventFieldFilters != nil {
		t.Fatal("absent filter must preserve nil")
	}
}

func TestB1713StepSchemaEvolutionAddsOnlyEventFieldFilters(t *testing.T) {
	_, filterSchema := paramSchemaFingerprint(reflect.TypeOf(EventFieldFilter{}))
	if filterSchema != "Field|string|field;Op|string|op;Value|tracediag.EventFieldValue|value" {
		t.Fatalf("local YAML scalar migration changed the closed predicate face: %s", filterSchema)
	}
	_, schema := paramSchemaFingerprint(reflect.TypeOf(Step{}))
	var prior []string
	added := 0
	for _, field := range strings.Split(schema, ";") {
		if field == "EventFieldFilters|[]tracediag.EventFieldFilter|event_field_filters" {
			added++
			continue
		}
		prior = append(prior, field)
	}
	sum := sha256.Sum256([]byte(strings.Join(prior, ";")))
	if added != 1 || hex.EncodeToString(sum[:]) != "77f2f21c1f75a0e0babddb8f9981850ba79bbfbb17f278e1a88fb94eb23ca54e" {
		t.Fatalf("unrelated step schema changed: %s", schema)
	}
}

func TestB1713RunJankFieldFiltersKeepsNativeClockAndIdentity(t *testing.T) {
	scriptPath, tracePath, _ := writeRunFixtures(t, `version: 1
steps:
  - label: selected_jank
    view: event_search
    window: "0.9..1.4"
    event_types: [trace_mark]
    event_field_filters:
      - {field: jank_frames, op: gte, value: 2}
      - {field: appid, op: eq, value: 30}
    max_lines: 30
`)
	trace := "writer-10 (10) [001] .... 1.000000: tracing_mark_write: B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254741093, jank_frames=2, appid=30\n" +
		"writer-10 (10) [001] .... 1.100000: tracing_mark_write: B|20|jank_event_sync: start_ts=30, end_ts=40, jank_frames=1, appid=30\n" +
		"writer-10 (10) [001] .... 1.200000: tracing_mark_write: B|20|jank_event_sync: start_ts=30, end_ts=40, jank_frames=8, appid=31\n" +
		"writer-10 (10) [001] .... 1.300000: tracing_mark_write: B|20|other jank_event_sync: start_ts=30, end_ts=40, jank_frames=8, appid=30\n"
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &output)
	if err != nil || failed != 0 {
		t.Fatalf("Run failed=%d err=%v\n%s", failed, err, output.String())
	}
	report := output.String()
	for _, want := range []string{"matched=1 emitted=1", "line=1 ts=1.000000", "writer-10 (10)", "B|20|jank_event_sync", "start_ts=9007199254740993", "end_ts=9007199254741093", "appid=30", "start_ts_ns=9007199254740993", "reported_duration_ns=100", "native_time_domain=source_trace_clock", "appid is not a scheduler TID"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks exact native payload/header separation %q:\n%s", want, report)
		}
	}
	for _, absent := range []string{"line=2 ts=", "line=3 ts=", "line=4 ts="} {
		if strings.Contains(report, absent) {
			t.Errorf("AND predicate admitted rejected raw row %q:\n%s", absent, report)
		}
	}
}

func TestB1713EventRowKeepsTypedNativeFieldsAheadOfClampedRawTail(t *testing.T) {
	event := tracequery.EventView{Event: tracequery.Event{
		Line: 7, Ts: 1.25, Type: tracequery.EventTraceMark,
		PluginFields: &tracequery.PluginFields{JankEvent: &tracequery.JankEventFields{
			TimeDomainStatus: "unverified",
			Values:           &tracequery.JankEventValues{StartTSNS: 9007199254740993, EndTSNS: 9007199254741093, ReportedDurationNS: 100},
		}},
	}, Raw: strings.Repeat("x", maxRenderedTokenBytes+100)}
	row := renderEventRow(event)
	for _, want := range []string{"ts=1.250000", "start_ts_ns=9007199254740993", "end_ts_ns=9007199254741093", "jank_frames=0", "appid=0", "reported_duration_ns=100", "native_time_domain=unverified"} {
		if !strings.Contains(row, want) {
			t.Errorf("clamped raw tail displaced exact typed metadata %q: %s", want, row)
		}
	}
	if strings.Contains(row, event.Raw) {
		t.Fatal("raw row token bound was bypassed")
	}
	ordinary := tracequery.EventView{Event: tracequery.Event{Line: 7, Ts: 1.25, Type: tracequery.EventSchedWakeup}, Raw: "original"}
	if got := renderEventRow(ordinary); got != "- line=7 ts=1.250000 type=sched_wakeup | original" {
		t.Fatalf("sparse metadata changed an unrelated event: %s", got)
	}
}

func TestB1713RunJankFieldFiltersRetainsMatchedAndVisibleCounts(t *testing.T) {
	scriptPath, tracePath, _ := writeRunFixtures(t, `version: 1
steps:
  - label: bounded_jank
    view: event_search
    event_field_filters:
      - {field: jank_frames, op: gte, value: 2}
      - {field: appid, op: eq, value: 30}
    max_lines: 5
`)
	var trace strings.Builder
	for i := 0; i < 12; i++ {
		frames := 2
		if i%2 != 0 {
			frames = 1
		}
		fmt.Fprintf(&trace, "writer-10 (10) [001] .... 1.%06d: tracing_mark_write: B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254741093, jank_frames=%d, appid=30\n", i, frames)
	}
	if err := os.WriteFile(tracePath, []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &output)
	if err != nil || failed != 0 {
		t.Fatalf("Run failed=%d err=%v\n%s", failed, err, output.String())
	}
	report := output.String()
	if !strings.Contains(report, "matched_total=6") || !strings.Contains(report, "compacted=true") || !strings.Contains(report, "统计完成不代表匹配记录已全部返回或展示") {
		t.Fatalf("filtered census or visible truncation disclosure missing:\n%s", report)
	}
	shown := strings.Count(report, "- line=")
	if shown < 1 || shown >= 6 || !strings.Contains(report, fmt.Sprintf("matched=6 emitted=%d", shown)) {
		t.Fatalf("visible/total filtered row counts disagree (shown=%d):\n%s", shown, report)
	}
}
