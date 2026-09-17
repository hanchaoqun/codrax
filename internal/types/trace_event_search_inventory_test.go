package types

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testTraceEventSearchInventoryRecord() ObservationRecord {
	return ObservationRecord{
		ID: "inventory-q", Producer: "trace_query", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Role: AnswerAggregateRoleSupportingCoverage, GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane: ObservationProvenanceArtifactSpan, ClaimAuthority: ObservationClaimAuthorityDirectObservation,
		Predicate: TraceEventSearchInventoryPredicate, ObservedAt: "2026-09-16T12:00:00Z",
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture.trace", PayloadRef: "/result.json", QueryScopeID: "query-one"},
		EventSearchInventory: &TraceEventSearchInventory{
			SchemaVersion: 1, QueryScopeID: "query-one",
			Query:        TraceEventSearchInventoryQuery{View: "event_search", EventFieldFilters: []TraceEventSearchInventoryFilter{{Field: "jank_frames", Op: "gte", Value: "2"}}},
			Coverage:     TraceEventSearchInventoryCoverage{ScopeKind: "artifact", ScopeComplete: true, MatchedTotal: 1, Emitted: 1, EnumerationComplete: true},
			RowsComplete: true,
			Rows: []TraceEventSearchInventoryRow{{Line: 1, EventType: "trace_mark", EmitterTID: 101, MarkerPID: 201, Raw: "original",
				JankEvent: &TraceEventSearchJankEvent{TimeDomainStatus: "unverified", Values: &TraceEventSearchJankValues{StartTSNS: 9007199254740993, EndTSNS: 9007199254740995, JankFrames: 2, AppID: 620, ReportedDurationNS: 2}}}},
			Caveats: []string{"verbatim disclosure"},
		},
	}
}

func TestTraceEventSearchInventoryValidationAndExactJSON(t *testing.T) {
	good := testTraceEventSearchInventoryRecord()
	if !IsValidTraceEventSearchInventoryRecord(good) {
		t.Fatal("valid receipt rejected")
	}
	suffixed := good
	suffixed.Producer = "trace_query:run2"
	if !IsValidTraceEventSearchInventoryRecord(suffixed) {
		t.Fatal("existing run-qualified producer rejected")
	}
	for name, mutate := range map[string]func(*ObservationRecord){
		"model_origin":    func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginSystemInference },
		"model_producer":  func(r *ObservationRecord) { r.Producer = "aggregate_facts" },
		"model_claim":     func(r *ObservationRecord) { r.ClaimAuthority = ObservationClaimAuthorityModelInference },
		"soft_policy":     func(r *ObservationRecord) { r.GroundingPolicy = ClaimGroundingSoft },
		"wrong_role":      func(r *ObservationRecord) { r.Role = AnswerAggregateRolePrincipalAnswer },
		"causal_lane":     func(r *ObservationRecord) { r.ProvenanceLane = ObservationProvenanceObservedDirectCause },
		"wrong_predicate": func(r *ObservationRecord) { r.Predicate = "root_cause" },
		"missing_source":  func(r *ObservationRecord) { r.SourceRef.Path = "" },
		"missing_query":   func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		"wrong_query":     func(r *ObservationRecord) { r.EventSearchInventory.QueryScopeID = "other" },
		"missing_payload": func(r *ObservationRecord) { r.SourceRef.PayloadRef = "" },
		"version":         func(r *ObservationRecord) { r.EventSearchInventory.SchemaVersion++ },
		"count":           func(r *ObservationRecord) { r.EventSearchInventory.Coverage.MatchedTotal = 0 },
		"omitted":         func(r *ObservationRecord) { r.EventSearchInventory.HandoffRowsOmitted = 1 },
		"false_complete":  func(r *ObservationRecord) { r.EventSearchInventory.Coverage.EnumerationComplete = false },
		"nan_window":      func(r *ObservationRecord) { r.EventSearchInventory.Coverage.ScopeTimeStart = math.NaN() },
		"fake_clock":      func(r *ObservationRecord) { r.EventSearchInventory.Rows[0].JankEvent.TimeDomainStatus = "aligned" },
		"fake_duration":   func(r *ObservationRecord) { r.EventSearchInventory.Rows[0].JankEvent.Values.ReportedDurationNS = 3 },
		"float_filter":    func(r *ObservationRecord) { r.EventSearchInventory.Query.EventFieldFilters[0].Value = "2.0" },
	} {
		t.Run(name, func(t *testing.T) {
			r := good
			r.EventSearchInventory = CloneTraceEventSearchInventory(good.EventSearchInventory)
			mutate(&r)
			if IsValidTraceEventSearchInventoryRecord(r) {
				t.Fatalf("invalid %s admitted", name)
			}
		})
	}
	raw, err := json.Marshal(good)
	if err != nil || !strings.Contains(string(raw), `"start_ts_ns":"9007199254740993"`) {
		t.Fatalf("lossy wire %s %v", raw, err)
	}
	var decoded ObservationRecord
	if err := json.Unmarshal(raw, &decoded); err != nil || !IsValidTraceEventSearchInventoryRecord(decoded) {
		t.Fatalf("roundtrip failed %v", err)
	}
}

func TestTraceEventSearchInventoryLedgerCloneAndNoCausalPromotion(t *testing.T) {
	a := testTraceEventSearchInventoryRecord()
	b := a
	b.ID = "inventory-q2"
	b.SourceRef.QueryScopeID = "query-two"
	b.EventSearchInventory = CloneTraceEventSearchInventory(a.EventSearchInventory)
	b.EventSearchInventory.QueryScopeID = "query-two"
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{a, b}}}})
	var rows []ObservationRecord
	for _, r := range ledger.Records {
		if IsValidTraceEventSearchInventoryRecord(r) {
			rows = append(rows, r)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("query merge lost identity: %+v", ledger)
	}
	rows[0].EventSearchInventory.Rows[0].JankEvent.Values.AppID = 999
	rows[0].EventSearchInventory.Query.EventFieldFilters[0].Value = "999"
	rows[0].EventSearchInventory.Caveats[0] = "changed"
	if a.EventSearchInventory.Rows[0].JankEvent.Values.AppID != 620 || a.EventSearchInventory.Query.EventFieldFilters[0].Value != "2" || a.EventSearchInventory.Caveats[0] != "verbatim disclosure" {
		t.Fatal("ledger mutated producer receipt")
	}
	withoutInventory := ObservationLedger{Records: append([]ObservationRecord(nil), ledger.Records...)}
	for n := range withoutInventory.Records {
		withoutInventory.Records[n].EventSearchInventory = nil
	}
	if set := CompileTraceCausalProjectionSet(ledger); !reflect.DeepEqual(set, CompileTraceCausalProjectionSet(withoutInventory)) {
		t.Fatalf("inventory became causal projection: %+v", set)
	}
}
