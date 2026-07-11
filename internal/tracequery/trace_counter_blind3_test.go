package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

func TestTraceCounterWireSchemaStandardHarmonyCarvedAndGlobal(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		owner       int
		scope       string
		counter     string
		value       float64
		trailingTag string
		level       string
		tagBits     string
	}{
		{
			name:  "standard atrace",
			line:  `writer-11 (11) [000] .... 1.000000: tracing_mark_write: C|42|Heap size (KB)|157028`,
			owner: 42, scope: traceCounterOwnerPayloadProcess, counter: "Heap size (KB)", value: 157028,
		},
		{
			name:  "harmony tag",
			line:  `rs-11 (11) [000] .... 1.000000: print: C|1252|H:VSync-rs|0|I38`,
			owner: 1252, scope: traceCounterOwnerPayloadProcess, counter: "H:VSync-rs", value: 0, trailingTag: "I38", level: "I", tagBits: "38",
		},
		{
			name:  "production name with pipe separated unit",
			line:  `SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455`,
			owner: 23106, scope: traceCounterOwnerPayloadProcess, counter: "Heap size |(KB)", value: 205455,
		},
		{
			name:  "production unit name with harmony tag",
			line:  `SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455|I38`,
			owner: 23106, scope: traceCounterOwnerPayloadProcess, counter: "Heap size |(KB)", value: 205455, trailingTag: "I38", level: "I", tagBits: "38",
		},
		{
			name:  "address carved harmony",
			line:  `rs-11 (11) [000] .... 1.000000: print: 0x0: 1252|H:DVSyncRateManagerPeriod|16.67|M0538`,
			owner: 1252, scope: traceCounterOwnerPayloadProcess, counter: "H:DVSyncRateManagerPeriod", value: 16.67, trailingTag: "M0538", level: "M", tagBits: "0538",
		},
		{
			name:  "explicit global",
			line:  `kernel-0 (0) [000] .... 1.000000: print: C|0|cpu_total_load|70.0`,
			owner: 0, scope: traceCounterOwnerGlobal, counter: "cpu_total_load", value: 70,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(1, tc.line, newStringInterner())
			if !ok || ev.Type != EventTraceMark || ev.SpanAction != "C" {
				t.Fatalf("counter row not recognised: ok=%v event=%+v", ok, ev)
			}
			sample := parseTraceCounterSample(ev)
			if !sample.identityOK || !sample.numericValid || sample.issueReason != "" {
				t.Fatalf("valid counter rejected: %+v", sample)
			}
			if sample.ownerPID != tc.owner || sample.ownerScope != tc.scope || sample.name != tc.counter || sample.numericValue != tc.value || sample.metadataRaw != tc.trailingTag || sample.outputLevel != tc.level || sample.tagBits != tc.tagBits {
				t.Fatalf("parsed sample=%+v want owner=%d scope=%s name=%q value=%g tag=%q", sample, tc.owner, tc.scope, tc.counter, tc.value, tc.trailingTag)
			}
		})
	}
}

func TestTraceCounterHarmonyMetadataAndOwnerClosedDomain(t *testing.T) {
	for _, metadata := range []string{"D00", "I38", "C3062", "M0538", "D0001", "M0501", "M0105"} {
		payload := "C|2147483647|depth|1|" + metadata
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok {
			t.Fatalf("valid metadata row rejected: %s", metadata)
		}
		sample := parseTraceCounterSample(ev)
		if !sample.identityOK || !sample.numericValid || sample.metadataRaw != metadata || sample.ownerPID != 2147483647 {
			t.Fatalf("valid metadata/owner rejected: %s => %+v", metadata, sample)
		}
	}
	for _, metadata := range []string{"I1", "I123", "I3863", "M3830", "D0005", "M0500", "I0105", "I05", "C0538", "X38"} {
		payload := "C|42|depth|1|" + metadata
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok {
			t.Fatalf("inventory row disappeared: %s", metadata)
		}
		sample := parseTraceCounterSample(ev)
		if sample.numericValid || sample.metadataRaw != "" {
			t.Fatalf("invalid metadata was peeled or minted a value: %s => %+v", metadata, sample)
		}
	}
	for _, owner := range []string{"2147483648", "-1", "owner", " 42 "} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, "C|"+owner+"|depth|1"), newStringInterner())
		if !ok || parseTraceCounterSample(ev).identityOK {
			t.Fatalf("out-of-domain owner admitted: %s => %+v", owner, ev)
		}
	}
	for _, payload := range []string{"C|42|depth| 1", "C|42|depth| 1 |I38", "C|42|depth|1| I38"} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok || parseTraceCounterSample(ev).numericValid {
			t.Fatalf("whitespace-padded scalar/metadata admitted: %q => %+v", payload, ev)
		}
	}
	if parsed := parseTraceMarkValidated("E|42|D0005"); parsed.invalidAction != traceMarkActionE || parsed.invalidReason != traceMarkReasonInvalidEndTag {
		t.Fatalf("unreachable OH metadata became a duration endpoint: %+v", parsed)
	}
	if parsed := parseTraceMarkValidated("E|42|I05"); parsed.invalidAction != traceMarkActionE || parsed.invalidReason != traceMarkReasonInvalidEndTag {
		t.Fatalf("commercial tag bits with non-commercial level became an endpoint: %+v", parsed)
	}
	if parsed := parseTraceMarkValidated("E|42|M0500"); parsed.invalidAction != traceMarkActionE || parsed.invalidReason != traceMarkReasonInvalidEndTag {
		t.Fatalf("unreachable commercial/always ordering became an endpoint: %+v", parsed)
	}
	for _, payload := range []string{"C|42|   |1", "C|42| | |1"} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		sample := parseTraceCounterSample(ev)
		if !ok || sample.identityOK || sample.issueReason != "empty_counter_name" {
			t.Fatalf("all-space opaque name minted identity: %q => %+v", payload, sample)
		}
	}
}

func TestTraceCounterFullPayloadAuthoritySurvivesFieldTextClamp(t *testing.T) {
	name := strings.Repeat("segment|", 45) + "tail"
	interner := newStringInterner()
	first, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, "C|42|"+name+"|7|I38"), interner)
	if !ok || len(first.FieldText) != 300 || first.SpanName != name || first.SpanValue != "7" || first.PluginFields == nil || first.PluginFields.Counter == nil || !first.PluginFields.Counter.Parsed {
		t.Fatalf("full counter payload did not survive display clamp: %+v", first)
	}
	wantSideBytes := int64(unsafe.Sizeof(PluginFields{})) + int64(unsafe.Sizeof(TraceCounterFields{}))
	if got := eventSideTableBytes(&first); got != wantSideBytes {
		t.Fatalf("nested counter side-table cache accounting=%d want=%d", got, wantSideBytes)
	}
	sample := parseTraceCounterSample(first)
	if !sample.identityOK || !sample.numericValid || sample.name != name || sample.metadataRaw != "I38" {
		t.Fatalf("typed side-table parity failed: %+v", sample)
	}
	body, err := json.Marshal(first)
	if err != nil || !strings.Contains(string(body), strconv.Quote(name)) {
		t.Fatalf("Event JSON retained the left/truncated interpretation: err=%v json=%s", err, body)
	}
	idx := &Index{Events: []Event{first}}
	if hits := EventSearch(idx, Query{Pattern: "segment|segment|tail", Limit: 10}); len(hits) != 1 || hits[0].SpanName != name {
		t.Fatalf("event_search diverged from typed counter name: %+v", hits)
	}

	second, ok := ParseLine(2, traceMarkTestLine("writer", 10, 2, "C|42|"+name+"|9|I38"), interner)
	if !ok {
		t.Fatal("second long counter row rejected")
	}
	idx.Events = append(idx.Events, second)
	deltas, quality := computeCounterDeltas(idx, Query{}, 8)
	if len(deltas) != 1 || deltas[0].Name != name || deltas[0].Delta != 2 || quality == nil || quality.NumericRows != 2 {
		t.Fatalf("delta consumer reparsed the clamped payload: deltas=%+v quality=%+v", deltas, quality)
	}

	overCap := "C|42|" + strings.Repeat("x", traceCounterPayloadMaxBytes) + "|1"
	tooLong, ok := ParseLine(3, traceMarkTestLine("writer", 10, 3, overCap), interner)
	if !ok || tooLong.SpanAction != "C" {
		t.Fatalf("over-cap row must remain counter inventory: ok=%t event=%+v", ok, tooLong)
	}
	tooLongSample := parseTraceCounterSample(tooLong)
	if tooLongSample.identityOK || tooLongSample.issueReason != "counter_payload_too_long" {
		t.Fatalf("over-cap payload minted typed identity: %+v", tooLongSample)
	}
	prefix, suffix := "C|42|", "|1"
	exactCap := prefix + strings.Repeat("c", traceCounterPayloadMaxBytes-len(prefix)-len(suffix)) + suffix
	if len(exactCap) != traceCounterPayloadMaxBytes {
		t.Fatalf("test fixture cap length=%d", len(exactCap))
	}
	exactCapEvent, ok := ParseLine(5, traceMarkTestLine("writer", 10, 5, exactCap), interner)
	if !ok || parseTraceCounterSample(exactCapEvent).issueReason != "counter_payload_too_long" {
		t.Fatalf("writer truncation boundary was admitted: %+v", parseTraceCounterSample(exactCapEvent))
	}
	belowCap := prefix + strings.Repeat("b", traceCounterPayloadMaxBytes-1-len(prefix)-len(suffix)) + suffix
	belowCapEvent, ok := ParseLine(6, traceMarkTestLine("writer", 10, 6, belowCap), interner)
	if !ok || !parseTraceCounterSample(belowCapEvent).numericValid {
		t.Fatalf("complete payload below protocol cap rejected: %+v", parseTraceCounterSample(belowCapEvent))
	}
	trimmedAtCapEvent, ok := ParseLine(7, traceMarkTestLine("writer", 10, 7, belowCap+" "), interner)
	if !ok || parseTraceCounterSample(trimmedAtCapEvent).issueReason != "counter_payload_too_long" {
		t.Fatalf("raw 1024-byte record lost its truncation signal after trim: %+v", parseTraceCounterSample(trimmedAtCapEvent))
	}
	carvedBodyPrefix, carvedBodySuffix := "42|", "|1"
	carvedBody := carvedBodyPrefix + strings.Repeat("r", traceCounterPayloadMaxBytes-3-len(carvedBodyPrefix)-len(carvedBodySuffix)) + carvedBodySuffix
	carvedAtCap, ok := ParseLine(8, traceMarkTestLine("writer", 10, 8, "0x0: "+carvedBody+" "), interner)
	if !ok || parseTraceCounterSample(carvedAtCap).issueReason != "counter_payload_too_long" {
		t.Fatalf("carved raw cap lost its action-restoration/truncation signal: %+v", parseTraceCounterSample(carvedAtCap))
	}

	attackName := strings.Repeat("n", 310)
	attack, ok := ParseLine(4, traceMarkTestLine("writer", 10, 4, "C|42|"+attackName+"|1|junk"), interner)
	if !ok || len(attack.FieldText) != 300 {
		t.Fatalf("post-clamp attack row disappeared: ok=%t event=%+v", ok, attack)
	}
	attackSample := parseTraceCounterSample(attack)
	if !attackSample.identityOK || attackSample.numericValid || attackSample.valueRaw != "junk" {
		t.Fatalf("bounded FieldText revalidated a poisoned full payload: %+v", attackSample)
	}
}

func TestTraceCounterOpaqueNameWhitespaceIsExactIdentity(t *testing.T) {
	idx := buildTraceIndex(t, "counter-name-space.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|depth|1`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|depth|3`,
		`writer-10 (10) [000] .... 1.200000: print: C|42| depth |10`,
		`writer-10 (10) [000] .... 1.300000: print: C|42| depth |14`,
	}, "\n")+"\n")
	deltas, quality := computeCounterDeltas(idx, Query{}, 8)
	if len(deltas) != 2 || quality == nil || quality.TotalSeries != 2 {
		t.Fatalf("opaque name whitespace merged identities: deltas=%+v quality=%+v", deltas, quality)
	}
	seen := map[string]float64{}
	for _, row := range deltas {
		seen[row.Name] = row.Delta
	}
	if seen["depth"] != 2 || seen[" depth "] != 4 {
		t.Fatalf("opaque names were normalized: %+v", seen)
	}
}

func TestTraceCounterCompatibilityInventorySharesTypedParserWithDeltas(t *testing.T) {
	idx := buildTraceIndex(t, "counter-inventory-parity.systrace", strings.Join([]string{
		// Exact production pipe-unit witness plus a second sample for delta.
		`SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455`,
		`SharedPreferenc-64221 (63993) [008] .... 1.010000: print: C|23106|Heap size |(KB)|205460`,
		// Same grammar with an exact trailing Harmony tag.
		`SharedPreferenc-64221 (63993) [008] .... 1.020000: print: C|23106|Tagged heap |(KB)|7|M0538`,
		`SharedPreferenc-64221 (63993) [008] .... 1.030000: print: C|23106|Tagged heap |(KB)|9|M0538`,
		// Non-numeric and invalid rows remain legacy inventory, while the typed
		// quality face explains why they cannot mint counter_deltas.
		`SharedPreferenc-64221 (63993) [008] .... 1.040000: print: C|23106|Pending |(count)|NaN|I38`,
		`SharedPreferenc-64221 (63993) [008] .... 1.050000: print: C|bad|queue_depth|17`,
		`SharedPreferenc-64221 (63993) [008] .... 1.060000: print: C|23106|bad_extra|1|not-a-track`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if stats.CounterQuality == nil || stats.CounterQuality.Rows != 7 || stats.CounterQuality.InvalidRows != 1 || stats.CounterQuality.NonNumericRows != 2 {
		t.Fatalf("counter quality did not retain invalid/non-numeric rows: %+v", stats.CounterQuality)
	}
	if len(stats.TraceCounters) != 7 {
		t.Fatalf("compatibility inventory lost a physical counter shape: %+v", stats.TraceCounters)
	}
	findInventory := func(name, value, tag string) *TraceCounterSummary {
		for i := range stats.TraceCounters {
			row := &stats.TraceCounters[i]
			if row.Name == name && row.Value == value && row.TrailingTag == tag {
				return row
			}
		}
		return nil
	}
	findDelta := func(name, tag string) *TraceCounterDeltaSummary {
		for i := range stats.CounterDeltas {
			row := &stats.CounterDeltas[i]
			if row.Name == name && row.TrailingTag == tag {
				return row
			}
		}
		return nil
	}

	production := findInventory("Heap size |(KB)", "205455", "")
	productionDelta := findDelta("Heap size |(KB)", "")
	if production == nil || productionDelta == nil {
		t.Fatalf("production pipe-unit row diverged across inventory/delta: inventory=%+v deltas=%+v", stats.TraceCounters, stats.CounterDeltas)
	}
	if production.OwnerPID != productionDelta.OwnerPID || production.OwnerScope != productionDelta.OwnerScope || production.OwnerRaw != "23106" || productionDelta.First != 205455 {
		t.Fatalf("production owner/value parity mismatch: inventory=%+v delta=%+v", production, productionDelta)
	}
	tagged := findInventory("Tagged heap |(KB)", "7", "M0538")
	taggedDelta := findDelta("Tagged heap |(KB)", "M0538")
	if tagged == nil || taggedDelta == nil || tagged.OwnerPID != taggedDelta.OwnerPID || tagged.OwnerScope != taggedDelta.OwnerScope || taggedDelta.First != 7 {
		t.Fatalf("tagged pipe-unit parity mismatch: inventory=%+v delta=%+v", tagged, taggedDelta)
	}
	if row := findInventory("Pending |(count)", "NaN", "I38"); row == nil || row.OwnerPID != 23106 || row.OwnerScope != traceCounterOwnerPayloadProcess {
		t.Fatalf("non-numeric typed identity disappeared from compatibility inventory: %+v", stats.TraceCounters)
	}
	if row := findInventory("queue_depth", "17", ""); row == nil || row.OwnerRaw != "bad" {
		t.Fatalf("invalid-owner row disappeared from compatibility inventory: %+v", stats.TraceCounters)
	}
	if row := findInventory("bad_extra|1", "not-a-track", ""); row == nil || row.OwnerRaw != "23106" {
		t.Fatalf("right-delimited non-numeric row lost its bounded inventory fields: %+v", stats.TraceCounters)
	}
	if !containsSubstring(stats.Caveats, "trace_counter_quality_degraded=true") {
		t.Fatalf("invalid/non-numeric inventory was not paired with quality disclosure: %v", stats.Caveats)
	}
}

func TestTraceCounterDeltaUsesPayloadOwnerNameAndPhysicalSourceIdentity(t *testing.T) {
	idx := buildTraceIndex(t, "counter-identity.systrace", strings.Join([]string{
		`emitter-a-10 (10) [000] .... 1.000000: print: C|100|queue_depth|1|I38`,
		`emitter-b-11 (11) [001] .... 1.100000: print: C|100|queue_depth|4|I38`,
		`emitter-a-10 (10) [000] .... 1.200000: print: C|200|queue_depth|10|I35`,
		`emitter-a-10 (10) [000] .... 1.300000: print: C|200|queue_depth|7|I35`,
		`emitter-a-10 (10) [000] .... 1.400000: print: C|100|render_depth|20|M0538`,
		`emitter-b-11 (11) [001] .... 1.500000: print: C|100|render_depth|25|M0538`,
		`emitter-a-10 (10) [000] .... 1.600000: print: C|0|queue_depth|30`,
		`emitter-b-11 (11) [001] .... 1.700000: print: C|0|queue_depth|32`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.8})
	if len(stats.CounterDeltas) != 4 {
		t.Fatalf("payload owner/tag identities were merged or split by emitter: %+v", stats.CounterDeltas)
	}
	if stats.CounterQuality == nil || stats.CounterQuality.Rows != 8 || stats.CounterQuality.TotalSeries != 4 || stats.CounterQuality.PublishedSeries != 4 {
		t.Fatalf("counter quality inventory mismatch: %+v", stats.CounterQuality)
	}
	find := func(scope string, owner int, name string) *TraceCounterDeltaSummary {
		for i := range stats.CounterDeltas {
			row := &stats.CounterDeltas[i]
			if row.OwnerScope == scope && row.OwnerPID == owner && row.Name == name {
				return row
			}
		}
		return nil
	}
	owner100 := find(traceCounterOwnerPayloadProcess, 100, "queue_depth")
	if owner100 == nil || owner100.Samples != 2 || owner100.First != 1 || owner100.Last != 4 || owner100.Delta != 3 {
		t.Fatalf("cross-emitter payload series not joined exactly: %+v", owner100)
	}
	if owner100.Thread.PID != 10 || owner100.OwnerPID != 100 || owner100.TrailingTag != "I38" || owner100.OutputLevel != "I" || owner100.TagBits != "38" || owner100.MetadataStatus != "stable" {
		t.Fatalf("owner, emitter, or metadata provenance was conflated: %+v", owner100)
	}
	if row := find(traceCounterOwnerPayloadProcess, 200, "queue_depth"); row == nil || row.Delta != -3 {
		t.Fatalf("second payload owner was merged with owner=100: %+v", row)
	}
	if row := find(traceCounterOwnerPayloadProcess, 100, "render_depth"); row == nil || row.Delta != 5 || row.TrailingTag != "M0538" {
		t.Fatalf("second logical name or metadata provenance was lost: %+v", row)
	}
	if row := find(traceCounterOwnerGlobal, 0, "queue_depth"); row == nil || row.Delta != 2 || row.MetadataStatus != "absent" {
		t.Fatalf("explicit pid=0 global series missing: %+v", row)
	}
	for _, row := range stats.CounterDeltas {
		if row.Baseline != traceCounterBaselineInWindowFirstSample || row.UnitStatus != traceCounterUnitUnknown {
			t.Fatalf("baseline/unit policy not explicit: %+v", row)
		}
		if row.SourcePath == "" || row.FirstLocalLine <= 0 || row.LastLocalLine <= 0 {
			t.Fatalf("physical provenance missing: %+v", row)
		}
	}
}

func TestTraceCounterMetadataChangeDoesNotSplitLogicalIdentityAndFailsClosed(t *testing.T) {
	idx := buildTraceIndex(t, "counter-metadata-change.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|queue_depth|1|I38`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|queue_depth|4|M0538`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{})
	if len(stats.CounterDeltas) != 0 || stats.CounterQuality == nil || stats.CounterQuality.TotalSeries != 1 || stats.CounterQuality.DerivedInvalidSeries != 1 || stats.CounterQuality.SuppressedSeries != 1 {
		t.Fatalf("metadata variation split or published one logical series: deltas=%+v quality=%+v", stats.CounterDeltas, stats.CounterQuality)
	}
	found := false
	for _, issue := range stats.CounterQuality.Issues {
		found = found || issue.Reason == "counter_metadata_changed"
	}
	if !found {
		t.Fatalf("metadata variation was not disclosed: %+v", stats.CounterQuality)
	}
}

func TestTraceCounterUnknownUnitAndInvalidRowsFailLoudBounded(t *testing.T) {
	lines := []string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|Heap size (KB)|100`,
		`writer-10 (10) [000] .... 1.010000: print: C|42|Heap size (KB)|NaN`,
		`writer-10 (10) [000] .... 1.020000: print: C|42|Heap size (KB)|120`,
		`writer-10 (10) [000] .... 1.030000: print: C|bad|queue_depth|1`,
		`writer-10 (10) [000] .... 1.040000: print: C|43||1`,
		`writer-10 (10) [000] .... 1.050000: print: C|44|queue_depth|Inf`,
		`writer-10 (10) [000] .... 1.060000: print: C|45|queue_depth|1|not-a-track`,
	}
	// More than the per-reason witness cap: counts remain complete while the
	// retained samples stay bounded.
	for i := 0; i < 5; i++ {
		lines = append(lines, `writer-10 (10) [000] .... 1.070000: print: C|46|bad_numeric|NaN`)
	}
	idx := buildTraceIndex(t, "counter-invalid.systrace", strings.Join(lines, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.2})

	if len(stats.CounterDeltas) != 0 {
		t.Fatalf("series with invalid endpoints/members minted a delta: %+v", stats.CounterDeltas)
	}
	if len(stats.TraceCounters) == 0 {
		t.Fatal("compatibility TraceCounters inventory must retain invalid/non-numeric rows")
	}
	quality := stats.CounterQuality
	if quality == nil || quality.Rows != len(lines) || quality.InvalidRows != 2 || quality.NonNumericRows != 8 || quality.SuppressedSeries != 4 {
		t.Fatalf("typed invalid inventory mismatch: %+v", quality)
	}
	if !containsSubstring(stats.Caveats, "trace_counter_quality_degraded=true") {
		t.Fatalf("invalid counters were fail-silent: %v", stats.Caveats)
	}
	var nonDecimal *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "non_decimal_or_non_finite_value" {
			nonDecimal = &quality.Issues[i]
		}
	}
	if nonDecimal == nil || nonDecimal.Count != 8 || len(nonDecimal.Samples) != traceCounterIssueSampleCap {
		t.Fatalf("issue count/sample cap mismatch: %+v", nonDecimal)
	}
}

func TestTraceCounterRightDelimitedOpaqueNamesAndUnknownTails(t *testing.T) {
	valid := map[string]string{
		"C|42|queue_depth|fabricated|1": "queue_depth|fabricated",
		"C|42|queue_depth|(KB|1":        "queue_depth|(KB",
		"C|42|queue_depth||inner|1":     "queue_depth||inner",
	}
	for payload, wantName := range valid {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok || ev.SpanAction != "C" {
			t.Fatalf("counter inventory row missing for %q: %+v", payload, ev)
		}
		sample := parseTraceCounterSample(ev)
		if !sample.identityOK || !sample.numericValid || sample.name != wantName || sample.numericValue != 1 {
			t.Fatalf("right-delimited opaque name rejected for %q: %+v", payload, sample)
		}
	}

	for _, payload := range []string{
		"C|42|queue_depth|1|not-a-metadata-token",
		"C|42|queue_depth|1|I1",
		"C|42|queue_depth|1|X99",
		"C|42|queue_depth|1|I38|M0538",
		"C|42|queue_depth|1|I38|junk",
		"C|42|queue_depth|",
	} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok || ev.SpanAction != "C" {
			t.Fatalf("counter inventory row missing for %q: %+v", payload, ev)
		}
		sample := parseTraceCounterSample(ev)
		if sample.identityOK && sample.numericValid {
			t.Fatalf("unknown/double tail minted a numeric series for %q: %+v", payload, sample)
		}
	}
}

func TestTraceCounterBaselineDoesNotGuessWindowHeadCarry(t *testing.T) {
	idx := buildTraceIndex(t, "counter-window-baseline.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 0.900000: print: C|42|Heap size (KB)|90`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|Heap size (KB)|100`,
		`writer-10 (10) [000] .... 1.200000: print: C|42|Heap size (KB)|130`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.3})
	if len(stats.CounterDeltas) != 1 {
		t.Fatalf("window counter missing: %+v", stats.CounterDeltas)
	}
	row := stats.CounterDeltas[0]
	if row.First != 100 || row.Last != 130 || row.Delta != 30 || row.Baseline != traceCounterBaselineInWindowFirstSample {
		t.Fatalf("pre-window sample was silently treated as carry-in: %+v", row)
	}
	if row.UnitStatus != traceCounterUnitUnknown {
		t.Fatalf("unit was guessed from '(KB)' name text: %+v", row)
	}
}

func TestTraceCounterPayloadLaneRollbackSurvivesEmitterAndHostLifecycleChanges(t *testing.T) {
	idx := buildTraceIndex(t, "counter-payload-rollback.systrace", strings.Join([]string{
		`writer-a-500 (500) [000] .... 2.000000: print: C|42|queue_depth|7|I38`,
		`creator-7 (7) [001] .... 2.100000: sched_wakeup_new: comm=host42 pid=42 prio=20 target_cpu=001`,
		`writer-b-501 (501) [002] .... 1.500000: print: C|42|queue_depth|3|I38`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 2.2})
	if len(stats.CounterDeltas) != 0 || !containsSubstring(stats.Caveats, "family=trace_counter_delta") {
		t.Fatalf("canonical/emitter/lifecycle changes hid a physical payload-lane rollback: deltas=%+v caveats=%v", stats.CounterDeltas, stats.Caveats)
	}
}

func TestTraceCounterFloat64PrecisionCollisionFailsClosed(t *testing.T) {
	idx := buildTraceIndex(t, "counter-precision.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|exact_limit|9007199254740992`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|exact_limit|9007199254740993`,
		`writer-10 (10) [000] .... 1.200000: print: C|43|safe_decimal|12345678901234.5`,
		`writer-10 (10) [000] .... 1.300000: print: C|43|safe_decimal|12345678901235.5`,
		`writer-10 (10) [000] .... 1.400000: print: C|44|unsafe_decimal|123456789012345.6`,
		`writer-10 (10) [000] .... 1.500000: print: C|45|unsafe_integer_delta|-9007199254740992`,
		`writer-10 (10) [000] .... 1.600000: print: C|45|unsafe_integer_delta|9007199254740991`,
		`writer-10 (10) [000] .... 1.700000: print: C|46|exact_decimal_delta|90071992547409.1`,
		`writer-10 (10) [000] .... 1.800000: print: C|46|exact_decimal_delta|90071992547409.2`,
	}, "\n")+"\n")
	deltas, quality := computeCounterDeltas(idx, Query{}, 8)
	if quality == nil || quality.NonNumericRows != 2 || quality.DerivedInvalidSeries != 1 || quality.SuppressedSeries != 3 {
		t.Fatalf("precision-unsafe rows were not suppressed: deltas=%+v quality=%+v", deltas, quality)
	}
	if len(deltas) != 2 {
		t.Fatalf("safe decimal compatibility lanes changed: %+v", deltas)
	}
	gotDeltas := map[string]float64{}
	for _, row := range deltas {
		gotDeltas[row.Name] = row.Delta
	}
	if gotDeltas["safe_decimal"] != 1 || gotDeltas["exact_decimal_delta"] != 0.1 {
		t.Fatalf("decimal delta was computed from rounded endpoints: %+v", gotDeltas)
	}
	var issue, derivedIssue *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "numeric_precision_unsafe" {
			issue = &quality.Issues[i]
		}
		if quality.Issues[i].Reason == "derived_delta_precision_unsafe" {
			derivedIssue = &quality.Issues[i]
		}
	}
	if issue == nil || issue.Count != 2 || derivedIssue == nil || derivedIssue.Count != 1 {
		t.Fatalf("precision loss was not disclosed exactly: %+v", quality)
	}
}

func TestTraceCounterTopNOrdersByExactDecimalDelta(t *testing.T) {
	idx := buildTraceIndex(t, "counter-exact-rank.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|a|0.00000000000000123456789012346`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|a|1`,
		`writer-10 (10) [000] .... 1.200000: print: C|42|z|0.00000000000000123456789012345`,
		`writer-10 (10) [000] .... 1.300000: print: C|42|z|1`,
	}, "\n")+"\n")
	deltas, quality := computeCounterDeltas(idx, Query{}, 1)
	if len(deltas) != 1 || deltas[0].Name != "z" || quality == nil || quality.TotalSeries != 2 || quality.TruncatedSeries != 1 {
		t.Fatalf("rounded float tie selected the smaller exact delta: deltas=%+v quality=%+v", deltas, quality)
	}
}

func TestTraceCounterSeriesBudgetFailsClosedWithoutIncompleteTopN(t *testing.T) {
	const overflow = 7
	events := make([]Event, 0, traceCounterSeriesBudget+overflow)
	for i := 0; i < traceCounterSeriesBudget+overflow; i++ {
		name := "counter_" + strconv.Itoa(i)
		value := strconv.Itoa(i)
		events = append(events, Event{
			Line: i + 1, Ts: 1 + float64(i)/100000, Type: EventTraceMark,
			SpanAction: "C", SpanPID: 42, SpanName: name, SpanValue: value,
			FieldText: "C|42|" + name + "|" + value, PID: 10, Comm: "writer",
		})
	}
	idx := &Index{Events: events}
	stats := ComputeWindowStats(idx, Query{})
	if len(stats.CounterDeltas) != 0 {
		t.Fatalf("series-budget overflow published top-N from an incomplete universe: %+v", stats.CounterDeltas)
	}
	quality := stats.CounterQuality
	if quality == nil || !quality.SeriesBudgetExceeded || quality.SeriesBudget != traceCounterSeriesBudget || quality.OverflowRows != overflow {
		t.Fatalf("series budget overflow not typed exactly: %+v", quality)
	}
	if quality.TotalSeries != traceCounterSeriesBudget || quality.TotalSeriesStatus != "lower_bound" || quality.PublishedSeries != 0 || quality.SuppressedSeries != traceCounterSeriesBudget {
		t.Fatalf("over-budget series accounting is not fail-closed: %+v", quality)
	}
	var budgetIssue *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "series_budget_exceeded" {
			budgetIssue = &quality.Issues[i]
		}
	}
	if budgetIssue == nil || budgetIssue.Count != overflow || len(budgetIssue.Samples) != traceCounterIssueSampleCap {
		t.Fatalf("budget issue count/witness cap mismatch: %+v", budgetIssue)
	}
	if len(stats.TraceCounters) == 0 {
		t.Fatal("series budget must not suppress the compatibility TraceCounters inventory")
	}
	if !containsSubstring(stats.Caveats, "budget_exceeded=true") {
		t.Fatalf("series budget fail-close was not disclosed: %v", stats.Caveats)
	}
}

func TestTraceCounterColdWarmWindowAndCompositeProvenance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter-window.systrace")
	body := strings.Join([]string{
		`writer-10 (10) [000] .... 0.900000: print: C|42|queue_depth|90|I38`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|queue_depth|100|I38`,
		`writer-11 (11) [001] .... 1.200000: print: C|42|queue_depth|130|I38`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeStart: 1.0, TimeEndSet: true, TimeEnd: 1.3}
	q := Query{TimeStart: 1.0, TimeEnd: 1.3}

	resetAnchorCaches()
	if _, err := BuildIndex(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	warmStats := ComputeWindowStats(warm, q)

	resetAnchorCaches()
	cold, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	coldStats := ComputeWindowStats(cold, q)
	if !reflect.DeepEqual(coldStats.CounterDeltas, warmStats.CounterDeltas) || !reflect.DeepEqual(coldStats.CounterQuality, warmStats.CounterQuality) {
		t.Fatalf("cold/warm counter semantics diverged:\ncold=%+v quality=%+v\nwarm=%+v quality=%+v", coldStats.CounterDeltas, coldStats.CounterQuality, warmStats.CounterDeltas, warmStats.CounterQuality)
	}

	// A composite's virtual-coordinate resolver is part of the series identity.
	// Two causally admitted source ledgers carrying the same payload token must
	// remain two series even after canonical timestamp sorting.
	composite := &Index{
		Path: "bundle.tracebundle.json",
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: filepath.Join(dir, "a.systrace"), TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", LocalLineCount: 2, CausalCompatible: true},
			{SourcePath: filepath.Join(dir, "b.systrace"), TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", VirtualLineBase: 100, LocalLineCount: 2, CausalCompatible: true},
		},
		Events: []Event{
			{Line: 1, Ts: 1.0, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "1", FieldText: "C|42|queue_depth|1", PID: 10},
			{Line: 101, Ts: 1.05, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "10", FieldText: "C|42|queue_depth|10", PID: 20},
			{Line: 2, Ts: 1.1, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "3", FieldText: "C|42|queue_depth|3", PID: 11},
			{Line: 102, Ts: 1.15, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "20", FieldText: "C|42|queue_depth|20", PID: 21},
		},
	}
	deltas, quality := computeCounterDeltas(composite, Query{TimeStart: 0.9, TimeEnd: 1.2}, 8)
	if len(deltas) != 2 || quality == nil || quality.TotalSeries != 2 {
		t.Fatalf("composite sources were merged by lookalike payload identity: deltas=%+v quality=%+v", deltas, quality)
	}
	if deltas[0].SourcePath == deltas[1].SourcePath {
		t.Fatalf("physical provenance absent from identity: %+v", deltas)
	}
}
