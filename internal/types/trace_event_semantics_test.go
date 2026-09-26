package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTraceEventSemanticsInventoryPersistence(t *testing.T) {
	record := testTraceEventSearchInventoryRecord()
	wire, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	wire = []byte(strings.Replace(string(wire), `"raw":"original"`, `"raw":"original","semantics":{"schema_version":1,"fields":[{"key":"counter.value","type":"decimal","status":"known","value":"9007199254740993"}]}`, 1))
	var restored ObservationRecord
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if !IsValidTraceEventSearchInventoryRecord(restored) {
		t.Fatal("valid restored semantics rejected")
	}
	out, err := json.Marshal(restored)
	if err != nil || !strings.Contains(string(out), `"value":"9007199254740993"`) {
		t.Fatalf("parsed semantics were discarded or rounded during persistence: %s %v", out, err)
	}
}

func semanticTestValue(value string) *string { return &value }

func TestTraceEventSemanticsValidationAndKnownZero(t *testing.T) {
	good := &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{
		{Key: "marker.name", Type: "text", Status: "known", Value: semanticTestValue("")},
		{Key: "marker.payload_pid", Type: "int64", Status: "known", Value: semanticTestValue("0")},
		{Key: "counter.value", Type: "decimal", Status: "known", Value: semanticTestValue("9007199254740993")},
		{Key: "counter.metadata", Type: "text", Status: "unavailable", IssueReason: "not_recorded"},
	}}
	if !ValidateTraceEventSemantics(nil) || !ValidateTraceEventSemantics(good) {
		t.Fatal("legacy or known-zero rejected")
	}
	for name, mutate := range map[string]func(*TraceEventSemantics){
		"version":           func(v *TraceEventSemantics) { v.SchemaVersion++ },
		"empty":             func(v *TraceEventSemantics) { v.Fields = nil },
		"duplicate":         func(v *TraceEventSemantics) { v.Fields = append(v.Fields, v.Fields[0]) },
		"unknown_key":       func(v *TraceEventSemantics) { v.Fields[0].Key = "root_cause" },
		"wrong_type":        func(v *TraceEventSemantics) { v.Fields[1].Type = "text" },
		"invented_unit":     func(v *TraceEventSemantics) { v.Fields[2].Unit = "ms" },
		"float_integer":     func(v *TraceEventSemantics) { v.Fields[1].Value = semanticTestValue("0.0") },
		"overflow":          func(v *TraceEventSemantics) { v.Fields[1].Value = semanticTestValue("9223372036854775808") },
		"missing_zero":      func(v *TraceEventSemantics) { v.Fields[1].Value = nil },
		"negative_owner":    func(v *TraceEventSemantics) { v.Fields[1].Value = semanticTestValue("-1") },
		"nan":               func(v *TraceEventSemantics) { v.Fields[2].Value = semanticTestValue("NaN") },
		"scientific":        func(v *TraceEventSemantics) { v.Fields[2].Value = semanticTestValue("1e3") },
		"unknown_has_value": func(v *TraceEventSemantics) { v.Fields[3].Value = semanticTestValue("0") },
		"unknown_no_reason": func(v *TraceEventSemantics) { v.Fields[3].IssueReason = "" },
		"causal_status":     func(v *TraceEventSemantics) { v.Fields[0].Status = "causal" },
		"long_value": func(v *TraceEventSemantics) {
			v.Fields[0].Value = semanticTestValue(strings.Repeat("x", TraceEventSemanticValueByteLimit+1))
		},
		"bad_utf8": func(v *TraceEventSemantics) { v.Fields[0].Value = semanticTestValue(string([]byte{0xff})) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := CloneTraceEventSemantics(good)
			mutate(copy)
			if ValidateTraceEventSemantics(copy) {
				t.Fatal("invalid semantic fields admitted")
			}
		})
	}
	for _, value := range []string{"0", "-0", "+1", ".25", "-12.5", "9007199254740993.0"} {
		copy := CloneTraceEventSemantics(good)
		copy.Fields[2].Value = semanticTestValue(value)
		if !ValidateTraceEventSemantics(copy) {
			t.Fatalf("exact decimal lost %q", value)
		}
	}
}

func TestTraceEventSemanticsOmissionCloneLedgerAndAuthority(t *testing.T) {
	r := testTraceEventSearchInventoryRecord()
	r.EventSearchInventory.Rows[0].Semantics = &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{
		OmitTraceEventSemanticValue(TraceEventSemanticField{Key: "marker.name", Type: "text", Status: "known", Value: semanticTestValue(strings.Repeat("long-name", 200))}, "value_exceeds_limit"),
		{Key: "counter.value", Type: "decimal", Status: "known", Value: semanticTestValue("0")},
	}}
	if !IsValidTraceEventSearchInventoryRecord(r) {
		t.Fatal("bounded omission rejected")
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{r}}}})
	if len(ledger.Records) == 0 {
		t.Fatal("inventory lost")
	}
	wire, _ := json.Marshal(ledger)
	var restored ObservationLedger
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if !IsValidTraceEventSearchInventoryRecord(restored.Records[0]) {
		t.Fatal("restored semantics invalid")
	}
	cloned := CloneTraceEventSearchInventory(restored.Records[0].EventSearchInventory)
	cloned.Rows[0].Semantics.Fields[0].Omitted.SHA256 = "changed"
	*cloned.Rows[0].Semantics.Fields[1].Value = "999"
	if restored.Records[0].EventSearchInventory.Rows[0].Semantics.Fields[0].Omitted.SHA256 == "changed" || *r.EventSearchInventory.Rows[0].Semantics.Fields[1].Value != "0" {
		t.Fatal("clone mutated accepted fields")
	}
	without := ObservationLedger{Records: append([]ObservationRecord(nil), ledger.Records...)}
	for n := range without.Records {
		without.Records[n].EventSearchInventory = nil
	}
	if !reflect.DeepEqual(CompileTraceCausalProjectionSet(ledger), CompileTraceCausalProjectionSet(without)) {
		t.Fatal("semantics granted causal authority")
	}
	bad := r
	bad.EventSearchInventory = CloneTraceEventSearchInventory(r.EventSearchInventory)
	bad.EventSearchInventory.Rows[0].EventType = "sched_switch"
	if IsValidTraceEventSearchInventoryRecord(bad) {
		t.Fatal("cross-family fields accepted")
	}
}

func TestTraceEventSemanticsRegistryAndWholeBudget(t *testing.T) {
	all := &TraceEventSemantics{SchemaVersion: 1}
	for _, d := range traceEventSemanticDescriptors {
		got, ok := LookupTraceEventSemanticDescriptor(d.Key)
		if !ok || got != d || d.Label == "" {
			t.Fatalf("incomplete descriptor %+v", d)
		}
		value := strings.Repeat("x", TraceEventSemanticValueByteLimit)
		if d.Type != "text" {
			value = "0"
		}
		field := TraceEventSemanticField{Key: d.Key, Type: d.Type, Unit: d.Unit, Status: "known", Value: semanticTestValue(value)}
		if !traceEventSemanticFieldValueValid(d.Key, value) {
			field.Status, field.Value, field.IssueReason = "unavailable", nil, "not_recorded"
		}
		all.Fields = append(all.Fields, field)
	}
	wire, _ := json.Marshal(all)
	if len(wire) <= TraceEventSemanticsByteLimit {
		t.Fatal("fixture does not exceed whole budget")
	}
	if ValidateTraceEventSemantics(all) {
		t.Fatal("oversized envelope accepted")
	}
}
