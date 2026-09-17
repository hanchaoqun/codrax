package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// The two clocks and three identities deliberately disagree. Payload metadata
// must remain searchable without becoming a header timestamp or scheduler TID.
func TestJankEventPublicTypedFilterAndNativeClock(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "jank-fields.systrace",
		traceMarkTestLine("writer", 10, 1, "B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254741093, jank_frames=2, appid=30"),
		traceMarkTestLine("writer", 10, 2, "B|20|jank_event_sync: start_ts=30, end_ts=40, jank_frames=1, appid=30"),
		traceMarkTestLine("writer", 10, 3, "B|20|jank_event_sync: start_ts=30, end_ts=40, jank_frames=8, appid=31"),
		traceMarkTestLine("writer", 10, 4, "B|20|other jank_event_sync: start_ts=30, end_ts=40, jank_frames=8, appid=30"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var q Query
	if err := json.Unmarshal([]byte(`{"View":"event_search","Limit":40,"EventFieldFilters":[{"field":"jank_frames","op":"gte","value":"2"},{"field":"appid","op":"eq","value":"30"}]}`), &q); err != nil {
		t.Fatal(err)
	}
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]Result{"indexed": Run(idx, q), "streamed": streamed} {
		if got := eventViewLines(result.Events); !reflect.DeepEqual(got, []int{1}) {
			t.Errorf("%s typed >=/appid AND filter returned %v, want [1]", name, got)
			continue
		}
		ev := result.Events[0]
		if ev.Ts != 1 || ev.PID != 10 || ev.SpanPID != 20 || !strings.Contains(ev.Raw, "appid=30") {
			t.Errorf("%s payload rewrote physical provenance: %+v", name, ev)
		}
		wire, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"jank_event"`, `"start_ts_ns":9007199254740993`, `"end_ts_ns":9007199254741093`, `"reported_duration_ns":100`, `"time_domain_status":"unverified"`} {
			if !strings.Contains(string(wire), want) {
				t.Errorf("%s lost exact native marker field %s: %s", name, want, wire)
			}
		}
		if result.EventSearchCoverage == nil || result.EventSearchCoverage.MatchedTotal != 1 {
			t.Errorf("%s census did not use the same filter: %+v", name, result.EventSearchCoverage)
		}
	}
}

func TestJankEventInvalidFieldPayloadNoZero(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "jank-invalid.systrace",
		traceMarkTestLine("writer", 10, 1, "B|20|jank_event_sync: start_ts=0, end_ts=1, jank_frames=bad, appid=30"),
		traceMarkTestLine("writer", 10, 2, "B|20|jank_event_sync: start_ts=0, end_ts=1, jank_frames=2, jank_frames=3, appid=30"),
		traceMarkTestLine("writer", 10, 3, "B|20|jank_event_sync: start_ts=0, end_ts=1, jank_frames=9223372036854775808, appid=30"),
		traceMarkTestLine("writer", 10, 4, "B|20|unrelated"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var q Query
	if err := json.Unmarshal([]byte(`{"View":"event_search","Limit":40,"EventFieldFilters":[{"field":"jank_frames","op":"ne","value":"0"}]}`), &q); err != nil {
		t.Fatal(err)
	}
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]Result{"indexed": Run(idx, q), "streamed": streamed} {
		if len(result.Events) != 0 || result.EventSearchCoverage == nil || result.EventSearchCoverage.MatchedTotal != 0 {
			t.Errorf("%s missing/invalid numeric fields matched ne=0: lines=%v coverage=%+v", name, eventViewLines(result.Events), result.EventSearchCoverage)
		}
		if !containsStringFragment(result.Caveats, "jank_event_fields_invalid=true rows=3") {
			t.Errorf("%s invalid metadata disappeared from disclosure: %v", name, result.Caveats)
		}
	}
}

func TestJankEventReportedFieldsExactGrammarAndRawPreservation(t *testing.T) {
	valid := "jank_event_sync: start_ts=0, end_ts=0, jank_frames=0, appid=0"
	for _, name := range []string{valid,
		"jank_event_sync: appid=0, end_ts=0, start_ts=0, jank_frames=0",
		"jank_event_sync: appid = 0 , end_ts = 0, start_ts = 0, jank_frames = 0 ",
		"jank_event_sync: start_ts=0, future_field=not-an-int, end_ts=0, jank_frames=0, appid=0, another=opaque",
	} {
		line := traceMarkTestLine("emitter", 10, 2, "B|20|"+name)
		ev, ok := ParseLine(17, line, newStringInterner())
		if !ok || ev.PluginFields == nil || ev.JankEvent == nil || ev.JankEvent.Values == nil {
			t.Fatalf("valid zero native fields unavailable: %+v", ev)
		}
		if ev.SpanName != name || ev.Line != 17 || ev.Ts != 2 || ev.PID != 10 || ev.SpanPID != 20 {
			t.Fatalf("typed decoration changed source marker/provenance: %+v", ev)
		}
		wire, err := json.Marshal(ev.JankEvent)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"start_ts_ns", "end_ts_ns", "jank_frames", "appid", "reported_duration_ns"} {
			if !strings.Contains(string(wire), `"`+field+`":0`) {
				t.Errorf("explicit zero missing from native fields: %s", wire)
			}
		}
		wantBytes := int64(unsafe.Sizeof(PluginFields{})) + int64(unsafe.Sizeof(JankEventFields{})) + int64(unsafe.Sizeof(JankEventValues{}))
		if got := eventSideTableBytes(&ev); got != wantBytes {
			t.Errorf("rare metadata cache bytes=%d want=%d", got, wantBytes)
		}
	}
	for _, name := range []string{
		"not_" + valid, "prefix " + valid, "Jank_event_sync: start_ts=0, end_ts=1, jank_frames=2, appid=3",
		"jank_event_sync_extra: start_ts=0, end_ts=1, jank_frames=2, appid=3",
		"jank_event_sync start_ts=0, end_ts=1, jank_frames=2, appid=3",
	} {
		ev, ok := ParseLine(1, traceMarkTestLine("emitter", 10, 1, "B|20|"+name), newStringInterner())
		if !ok || ev.SpanName != name || (ev.PluginFields != nil && ev.JankEvent != nil) {
			t.Errorf("different marker acquired jank metadata or lost raw name: %+v", ev)
		}
	}
	for _, action := range []string{"I", "S", "F"} {
		payload := action + "|20|" + valid
		if action != "I" {
			payload += "|7"
		}
		ev, ok := ParseLine(1, traceMarkTestLine("emitter", 10, 1, payload), newStringInterner())
		if !ok || (ev.PluginFields != nil && ev.JankEvent != nil) {
			t.Errorf("unproved action family %s acquired sync-jank metadata: %+v", action, ev)
		}
	}
	line := "emitter-10 (10) [000] .... 1.000000: " + valid
	if ev, ok := ParseLine(1, line, newStringInterner()); !ok || ev.PluginFields != nil || ev.Type == EventTraceMark {
		t.Fatalf("same-named raw event invented a marker type: %+v", ev)
	}
}

func TestJankEventMalformedMetadataRetainsRawButNoValues(t *testing.T) {
	for _, body := range []string{
		"", "start_ts=0, end_ts=1, jank_frames=2",
		"start_ts=0, end_ts=1, jank_frames=2, appid=3, =4",
		"start_ts=0, end_ts=1, jank_frames=2, appid=3, other",
		"start_ts=0, end_ts=1, jank_frames=2, jank_frames=3",
		"start_ts=0, end_ts=1, frames=2, appid=3",
		"start_ts=0, end_ts=1, jank_frames 2, appid=3",
		"start_ts=5, end_ts=4, jank_frames=2, appid=3",
	} {
		t.Run(body, func(t *testing.T) {
			assertInvalidJankMetadata(t, body)
		})
	}
	for _, field := range EventFieldFilterFields() {
		for _, bad := range []string{"-1", "+1", "1.0", "1e3", "NaN", "Inf", "true", "0x10", "9223372036854775808", ""} {
			parts := []string{"start_ts=0", "end_ts=1", "jank_frames=2", "appid=3"}
			for i := range parts {
				if strings.HasPrefix(parts[i], field+"=") {
					parts[i] = field + "=" + bad
				}
			}
			assertInvalidJankMetadata(t, strings.Join(parts, ", "))
		}
	}
}

func assertInvalidJankMetadata(t *testing.T, body string) {
	t.Helper()
	name := "jank_event_sync: " + body
	ev, ok := ParseLine(1, traceMarkTestLine("emitter", 10, 1, "B|20|"+name), newStringInterner())
	if !ok || ev.SpanName != name || ev.SpanAction != "B" || ev.PluginFields == nil || ev.JankEvent == nil || ev.JankEvent.Values != nil || ev.JankEvent.IssueReason == "" {
		t.Fatalf("malformed metadata lost inventory or gained values: %q => %+v", body, ev)
	}
	wire, err := json.Marshal(ev.JankEvent)
	if err != nil || strings.Contains(string(wire), `"values"`) || strings.Contains(string(wire), `"appid"`) {
		t.Fatalf("invalid metadata minted numeric JSON: %s err=%v", wire, err)
	}
	if !strings.Contains(JankEventSummary(ev), "raw marker retained") {
		t.Fatalf("invalid metadata missing textual disclosure: %q", JankEventSummary(ev))
	}
}

func TestJankEventOperatorsCombinationAndPaginationAccounting(t *testing.T) {
	var lines []string
	for i, frames := range []int{0, 1, 2, 2, 8} {
		// Two rows have identical header timestamp. They remain separate
		// physical observations rather than being deduplicated by app/count.
		ts := float64(i + 1)
		if i == 3 {
			ts = 3
		}
		lines = append(lines, traceMarkTestLine("emitter", 10, ts, "B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254740994, jank_frames="+strconv.Itoa(frames)+", appid=30"))
	}
	path := writeTraceMarkIntegrityTrace(t, "jank-census.systrace", lines...)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op   string
		want []int
	}{
		{"eq", []int{3, 4}}, {"ne", []int{1, 2, 5}}, {"gt", []int{5}},
		{"gte", []int{3, 4, 5}}, {"lt", []int{1, 2}}, {"lte", []int{1, 2, 3, 4}},
	} {
		q := Query{View: "event_search", Limit: 40, EventFieldFilters: []EventFieldFilter{{Field: "jank_frames", Op: tc.op, Value: "2"}}}
		assertJankSearchParity(t, path, idx, q, tc.want, len(tc.want))
	}
	q := Query{View: "event_search", Limit: 1, Patterns: []string{"not-present", "jank_event_sync"}, EventFieldFilters: []EventFieldFilter{{Field: "jank_frames", Op: "gte", Value: "2"}, {Field: "start_ts", Op: "eq", Value: "9007199254740993"}}}
	assertJankSearchParity(t, path, idx, q, []int{3}, 3)
	q.Limit = 3
	assertJankSearchParity(t, path, idx, q, []int{3, 4, 5}, 3)
	q.TimeStart, q.TimeEnd = 3, 3
	q.TimeStartSet, q.TimeEndSet = true, true
	assertJankSearchParity(t, path, idx, q, []int{3, 4}, 2)
	// Native timestamps are never substituted for the header clock.
	q.TimeStart, q.TimeEnd = 9007199, 9007200
	assertJankSearchParity(t, path, idx, q, nil, 0)
	q.TimeStart, q.TimeEnd, q.PID = 0, 0, 30
	q.TimeStartSet, q.TimeEndSet = false, false
	assertJankSearchParity(t, path, idx, q, nil, 0)
	q.PID = 10
	q.LineStart, q.LineEnd = 4, 4
	assertJankSearchParity(t, path, idx, q, []int{4}, 1)
	q.LineStart, q.LineEnd = 0, 0
	warm, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertJankSearchParity(t, path, warm, q, []int{3, 4, 5}, 3)
}

func assertJankSearchParity(t *testing.T, path string, idx *Index, q Query, lines []int, matched int) {
	t.Helper()
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]Result{"indexed": Run(idx, q), "streamed": streamed} {
		got := eventViewLines(result.Events)
		if len(got) != len(lines) || (len(lines) > 0 && !reflect.DeepEqual(got, lines)) {
			t.Errorf("%s filtered lines=%v want=%v; query=%+v", name, got, lines, q)
		}
		c := result.EventSearchCoverage
		if c == nil || c.MatchedTotal != matched || c.Emitted != len(lines) || !c.ScopeComplete || !c.EnumerationComplete {
			t.Errorf("%s filtered census=%+v want matched=%d emitted=%d", name, c, matched, len(lines))
		}
		if matched > len(lines) && (len(result.Compactions) != 1 || result.Compactions[0].Total != matched) {
			t.Errorf("%s missing pre-cap filtered compaction: %+v", name, result.Compactions)
		}
	}
}

func TestEventFieldFilterValueAndSharedValidation(t *testing.T) {
	for _, raw := range []string{`"0"`, `0`, `"9007199254740993"`, `9007199254740993`, `-9223372036854775808`, `"9223372036854775807"`} {
		var value EventFieldValue
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Errorf("exact JSON scalar %s rejected: %v", raw, err)
		}
		if string(value) != strings.Trim(raw, `"`) {
			t.Errorf("integer precision changed: %s => %q", raw, value)
		}
	}
	for _, raw := range []string{`1.5`, `"1.5"`, `1e3`, `"1e3"`, `"+1"`, `" 1"`, `"1 "`, `true`, `null`, `[]`, `{}`, `"NaN"`, `"Inf"`, `9223372036854775808`, `""`, `"0x10"`} {
		var value EventFieldValue
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			t.Errorf("invalid JSON scalar admitted: %s => %q", raw, value)
		}
	}
	for _, field := range EventFieldFilterFields() {
		for _, op := range EventFieldFilterOps() {
			if err := ValidateEventFieldFilters("event_search", []EventFieldFilter{{Field: field, Op: op, Value: "2"}}); err != nil {
				t.Errorf("declared field/op rejected: %s %s: %v", field, op, err)
			}
		}
	}
	for _, filters := range [][]EventFieldFilter{
		{{Field: "unknown", Op: "eq", Value: "2"}}, {{Field: "appid", Op: "EQ", Value: "2"}},
		{{Field: "appid", Op: "eq", Value: ""}}, {{Field: "appid", Op: "eq", Value: "2.0"}},
		make([]EventFieldFilter, EventFieldFilterLimit+1),
	} {
		if err := ValidateEventFieldFilters("event_search", filters); err == nil {
			t.Errorf("invalid filter admitted: %+v", filters)
		}
	}
	valid := []EventFieldFilter{{Field: "appid", Op: "eq", Value: "30"}}
	if err := ValidateEventFieldFilters("window_stats", valid); err == nil {
		t.Fatal("event-only numeric filter silently accepted on a causal/state view")
	}
	if err := ValidateEventFieldFilters("window_stats", nil); err != nil {
		t.Fatal("absent filter changed an unrelated view")
	}
	result := Run(&Index{}, Query{View: "window_stats", EventFieldFilters: valid})
	if !containsStringFragment(result.Caveats, "event_field_filters_invalid=true") || result.WindowStats != nil {
		t.Fatalf("public engine ignored invalid view/filter combination: %+v", result)
	}
}

func TestJankEventConvertedEnvelopesAndOriginalSpanSemantics(t *testing.T) {
	name := "jank_event_sync: start_ts=9007199254740993, end_ts=9007199254741093, jank_frames=2, appid=30"
	exact, err := FormatExactTraceMark(ExactTraceMark{TimestampNS: 2_000_000_000, CPU: 3, TID: 10, TGID: 10, SpanPID: 20, Action: "B", Comm: "emitter", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	unknownCPU, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{TimestampNS: 3_000_000_000, TID: 10, TGID: 10, SpanPID: 20, Action: "B", Comm: "emitter", Name: name, Reason: TraceMarkCPUReasonUnknownStart})
	if err != nil {
		t.Fatal(err)
	}
	path := writeTraceMarkIntegrityTrace(t, "jank-envelopes.systrace",
		traceMarkTestLine("emitter", 10, 1, "B|20|"+name),
		exact, unknownCPU,
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", Limit: 40, Pattern: "jank_event_sync", EventFieldFilters: []EventFieldFilter{{Field: "jank_frames", Op: "gte", Value: "2"}}}
	assertJankSearchParity(t, path, idx, q, []int{1, 2, 3}, 3)
	for _, ev := range Run(idx, q).Events {
		if ev.JankEvent == nil || ev.JankEvent.Values == nil || ev.JankEvent.Values.StartTSNS != 9007199254740993 || ev.JankEvent.Values.AppID != 30 || ev.PID != 10 || ev.SpanPID != 20 {
			t.Fatalf("converter envelope changed metadata/identity: %+v", ev)
		}
	}
	if got := idx.Events[2].TraceMarkerCPUStatus; got != TraceMarkCPUStatusUnavailable {
		t.Fatalf("native metadata invented a CPU: %q", got)
	}
	paired := writeTraceMarkIntegrityTrace(t, "jank-paired.systrace",
		traceMarkTestLine("emitter", 10, 1, "B|20|"+name),
		traceMarkTestLine("emitter", 10, 1.125, "E|20"),
	)
	pairedIndex, err := BuildIndex(context.Background(), paired)
	if err != nil {
		t.Fatal(err)
	}
	spans := Run(pairedIndex, Query{View: "span_window", SpanName: "jank_event_sync", Limit: 40}).SpanWindows
	if len(spans) != 1 || spans[0].StartTs != 1 || spans[0].EndTs != 1.125 || spans[0].DurationMs != 125 {
		t.Fatalf("reported native interval changed original B/E span semantics: %+v", spans)
	}
}

func TestJankEventCacheEpochAndClockMismatchWitnesses(t *testing.T) {
	if ParserVersion != "tracequery-v42" {
		t.Fatalf("jank typed metadata requires v42 cache epoch, got %q", ParserVersion)
	}
	cache := newTraceIndexCache(1 << 20)
	oldKey := parseCacheKey{path: "jank.trace", size: 1, modUnix: 1, version: "tracequery-v41"}
	newKey := oldKey
	newKey.version = ParserVersion
	cache.Store(oldKey, &Index{Path: oldKey.path, Events: []Event{{Type: EventTraceMark}}})
	if _, ok := cache.Load(newKey); ok {
		t.Fatal("pre-metadata parser cache reused")
	}
	for _, line := range []string{
		".ugc.aweme.lite-17267 (17267) [012] .... 13762.973139: print: B|37722|jank_event_sync: start_ts=13762824649681, end_ts=13762973126250, jank_frames=8, appid=37722",
		"com.baidu.tieba-59566 (59566) [002] .... 34579.594371: print: B|60194|jank_event_sync: start_ts=29822991976856, end_ts=29823127398574, jank_frames=8, appid=60194",
	} {
		ev, ok := ParseLine(1, line, newStringInterner())
		if !ok || ev.PluginFields == nil || ev.JankEvent == nil || ev.JankEvent.Values == nil || ev.JankEvent.TimeDomainStatus != "unverified" {
			t.Fatalf("real near/divergent clock witness lost metadata: %+v", ev)
		}
		if !strings.Contains(JankEventSummary(ev), "no header-clock alignment") {
			t.Fatal("clock proximity minted alignment")
		}
	}
}
