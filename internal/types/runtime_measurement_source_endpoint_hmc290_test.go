package types

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestHMC290UnknownSourceEndpointsNeverDefaultHalfOpen(t *testing.T) {
	input := hmc277CurrentMeasurementInput(t)
	before, _ := json.Marshal(input.ToolResults)
	contract := BuildRuntimeMeasurementContract(input)
	if len(contract.Choices()) != 2 {
		t.Fatal("missing current native tables")
	}
	for _, table := range contract.Choices() {
		basis, owned := table.NativeSourceTitleBasis()
		if !owned || basis.QueryWindow != "0–1 s (endpoint inclusion unavailable)" {
			t.Errorf("unknown endpoint acquired brackets: %+v owned=%t", basis, owned)
		}
		for lang, want := range map[string]string{"zh": "实际查询窗：0–1 s（端点包含性未知）", "en": "actual query window: 0–1 s (endpoint inclusion unavailable)"} {
			_, notes, valid := table.NativeSourcePresentation(lang)
			if !valid || !strings.Contains(strings.Join(notes, "\n"), want) {
				t.Errorf("%s unknown endpoint disclosure: %v", lang, notes)
			}
		}
		clone := table.Clone()
		if !reflect.DeepEqual(clone, table) {
			t.Fatal("clone changed source presentation")
		}
		wire, _ := json.Marshal(table)
		var restored RuntimeMeasurementTable
		if err := json.Unmarshal(wire, &restored); err != nil {
			t.Fatal(err)
		}
		if _, ok := restored.NativeSourceTitleBasis(); ok {
			t.Fatal("JSON restored native title authority")
		}
	}
	after, _ := json.Marshal(input.ToolResults)
	if string(before) != string(after) {
		t.Fatal("display formatting modified original records")
	}
}

func TestHMC290SourceWindowUnknownScopeIsNotAnInterval(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source ObservationSourceRef
	}{
		{"missing", ObservationSourceRef{}},
		{"point", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1}},
		{"line_precedence", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2, QueryLineRangeKnown: true, QueryLineStart: 3}},
		{"nan", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: math.NaN(), QueryWindowEndTs: 2}},
		{"infinite", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: math.Inf(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, edge := range []runtimeMeasurementQueryEnd{runtimeMeasurementQueryEndUnknown, runtimeMeasurementQueryEndExcluded, runtimeMeasurementQueryEndIncluded} {
				w := newRuntimeMeasurementSourceWindow(tc.source, edge)
				if w.known || w.format("en") != "" || w.format("zh") != "" {
					t.Fatal("endpoint metadata invented a continuous query scope")
				}
			}
		})
	}
}

func TestHMC290SourceWindowPairAndSealIntegrity(t *testing.T) {
	for _, inclusive := range []bool{false, true} {
		name, want := "half_open", "[0,1) s"
		if inclusive {
			name, want = "closed", "[0,1] s"
		}
		t.Run(name, func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			input.RequestModel = nil
			result := input.ToolResults[0]
			r := result.Observations[0]
			p, ok := DecodeRuntimeMeasurementPublication(r)
			if !ok {
				t.Fatal("fixture has no native publication")
			}
			r.Predicate, r.ClaimKey, r.Subject, r.Object = RuntimeSamplingMeasurementPredicate, RuntimeSamplingMeasurementPredicate, "native samples", "available"
			r.Role = AnswerAggregateRoleSupportingCoverage
			r.SourceRef.QueryTargetScope, r.SourceRef.QueryTargetPID = "thread", 101
			r.RichNotes = nil
			p.Tables[1].View = RuntimeMeasurementDistribution
			r, ok = PublishRuntimeSamplingMeasurement(r, p.Tables, inclusive)
			if !ok {
				t.Fatal("producer rejected exact native tables")
			}
			publication, ok := DecodeRuntimeMeasurementPublication(r)
			if !ok || publication.queryWindowEnd != runtimeMeasurementNativeQueryEnd(inclusive) {
				t.Fatal("sealed producer boundary was not preserved")
			}
			for _, mutation := range []string{"json_record", "changed_window", "changed_notes"} {
				bad := r
				switch mutation {
				case "json_record":
					wire, _ := json.Marshal(r)
					bad = ObservationRecord{}
					if err := json.Unmarshal(wire, &bad); err != nil {
						t.Fatal(err)
					}
				case "changed_window":
					bad.SourceRef.QueryWindowEndTs = 2
				case "changed_notes":
					bad.RichNotes = append(append([]string(nil), r.RichNotes...), "query_window=[0,1]")
				}
				if _, admitted := DecodeRuntimeMeasurementPublication(bad); admitted {
					t.Errorf("%s retained producer boundary authority", mutation)
				}
			}
			wire, _ := json.Marshal(publication)
			var jsonPublication RuntimeMeasurementPublication
			if err := json.Unmarshal(wire, &jsonPublication); err != nil || jsonPublication.queryWindowEnd != runtimeMeasurementQueryEndUnknown {
				t.Fatal("publication JSON restored private endpoint metadata")
			}
			other := publication
			other.queryWindowEnd = runtimeMeasurementNativeQueryEnd(!inclusive)
			for _, pair := range [][]RuntimeMeasurementPublication{{publication, other}, {other, publication}} {
				if len(coalesceRuntimeMeasurementPublications(pair)) != 0 {
					t.Fatal("conflicting producer endpoint semantics coalesced")
				}
			}
			result.Observations[0] = r
			pair := NewNativeMeasurementPair(input.runtimeMeasurementPairConsumer, [2]NativeMeasurementPairSide{{Request: json.RawMessage(`{"source":"path","view":"perf_stats"}`), Result: result}})
			wrapper := ToolResult{ToolName: "trace_query", Success: true, RuntimeMeasurementPair: pair}
			input.ToolResults = []ToolResult{wrapper}
			before, _ := json.Marshal(wrapper)
			contract := BuildRuntimeMeasurementContract(input)
			checked, status := 0, false
			for _, table := range contract.Choices() {
				if table.ObservationID == r.ID {
					basis, owned := table.NativeSourceTitleBasis()
					if !owned || basis.QueryWindow != want || !table.HasNativeSamplingFacts() {
						t.Fatalf("pair copy-out lost native boundary: %+v owned=%t", basis, owned)
					}
					checked++
				} else {
					for _, row := range table.Rows {
						status = status || strings.Contains(strings.Join(row, " "), r.ID+" "+want)
					}
				}
			}
			if checked != 2 || !status {
				t.Fatalf("pair endpoint did not reach exact facts/status: checked=%d status=%t", checked, status)
			}
			after, _ := json.Marshal(wrapper)
			if string(before) != string(after) {
				t.Fatal("formatting modified immutable pair report")
			}
		})
	}
}

func TestHMC290AcquisitionReferenceUsesNativeEndpoint(t *testing.T) {
	for _, mode := range []string{"unknown", "closed", "half_open"} {
		t.Run(mode, func(t *testing.T) {
			r := measurementProviderRecord(t)
			p, _ := DecodeRuntimeMeasurementPublication(r)
			p.Tables[0].DefaultPresentation = true
			want := "0–1 s（端点包含性未知）"
			if mode != "unknown" {
				r.Predicate, r.ClaimKey = RuntimeResourceMeasurementPredicate, RuntimeResourceMeasurementPredicate
				r.Subject, r.Object, r.Role = "native resource stacks", "available", AnswerAggregateRoleSupportingCoverage
				r.SourceRef.QueryTargetScope, r.SourceRef.QueryTargetPID, r.SourceRef.QueryLineRangeKnown = "thread", 101, true
				members := p.Tables[0].Clone()
				members.View, members.DefaultPresentation = RuntimeMeasurementMembers, false
				r.RichNotes = nil
				var ok bool
				r, ok = PublishRuntimeResourceMeasurement(r, []RuntimeMeasurementTable{p.Tables[0], members}, mode == "closed")
				if !ok {
					t.Fatal("resource producer rejected fixture")
				}
				p, ok = DecodeRuntimeMeasurementPublication(r)
				if !ok {
					t.Fatal("resource publication rejected")
				}
				want = "[0,1) s"
				if mode == "closed" {
					want = "[0,1] s"
				}
			}
			query := tracecatalog.QueryRecord{Plan: tracecatalog.Plan{ArtifactID: "a"}, Attempts: []tracecatalog.Attempt{{Completion: tracecatalog.Completion{Outcome: tracecatalog.OutcomeSuccess, PayloadRef: p.Source.PayloadRef}}}}
			state, refs, history := runtimeMeasurementAcquisitionSide(tracecatalog.Artifact{ID: "a"}, nil, []tracecatalog.QueryRecord{query}, []RuntimeMeasurementPublication{p}, map[string]string{r.ID: "available"})
			if state != "available" || !reflect.DeepEqual(refs, []string{r.ID + " " + want}) || !reflect.DeepEqual(history, []string{"success=1"}) {
				t.Fatalf("source endpoint or execution state changed: %s %v %v", state, refs, history)
			}
			query.Attempts[0].PayloadRef = "another-query.json"
			state, refs, history = runtimeMeasurementAcquisitionSide(tracecatalog.Artifact{ID: "a"}, nil, []tracecatalog.QueryRecord{query}, []RuntimeMeasurementPublication{p}, map[string]string{r.ID: "available"})
			if state != "unavailable" || len(refs) != 0 || !reflect.DeepEqual(history, []string{"success=1", "查询已执行，但没有当前范围内可用的原生量测；不是量值为0。"}) {
				t.Fatalf("display endpoint metadata changed query identity or execution history: %s %v %v", state, refs, history)
			}
		})
	}
}
