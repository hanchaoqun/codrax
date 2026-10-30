package types

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

func intervalEndpointRecordHMC321(t *testing.T) ObservationRecord {
	t.Helper()
	r := measurementProviderRecord(t)
	r.Role = AnswerAggregateRoleSupportingCoverage
	return r
}

func endpointOfHMC321(r ObservationRecord) runtimeMeasurementQueryEnd {
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		return runtimeMeasurementQueryEndUnknown
	}
	return p.queryWindowEnd
}

func TestHMC321IntervalEndpointSealIsDisplayOnly(t *testing.T) {
	for _, predicate := range []string{TraceBusinessSpanPredicate, "io_inflight", "thread_timeline_observation", "cpu_frequency_timeline_observation"} {
		for _, inclusive := range []bool{false, true} {
			t.Run(predicate+map[bool]string{false: "/excluded", true: "/included"}[inclusive], func(t *testing.T) {
				r := intervalEndpointRecordHMC321(t)
				r = rewriteRawProjectionHMC268(t, r, func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
					r.Predicate = predicate
					if predicate == "thread_timeline_observation" || predicate == "cpu_frequency_timeline_observation" {
						p.Tables[0].View = RuntimeMeasurementTimeline
					}
				})
				if _, ok := DecodeRuntimeMeasurementPublication(r); !ok {
					t.Fatal("HARNESS: fixture must satisfy the unchanged predicate/view contract")
				}
				before, _ := json.Marshal(r)
				r = PublishRuntimeMeasurementQueryWindow(r, 0, 1, inclusive)
				want := runtimeMeasurementNativeQueryEnd(inclusive)
				if endpointOfHMC321(r) != want || runtimeRawMeasurementValid(r) {
					t.Fatal("native endpoint missing or generic interval acquired raw-fact authority")
				}
				if _, ok := PublishRuntimeRawMeasurement(r); ok {
					t.Fatal("endpoint seal expanded the raw producer allowlist")
				}
				r = PublishRuntimeMeasurementQueryWindow(r, 0, 1, !inclusive)
				if endpointOfHMC321(r) != want {
					t.Fatal("repeated call replaced the original native endpoint")
				}
				after, _ := json.Marshal(r)
				if string(before) != string(after) {
					t.Fatal("private display contract changed public facts or wire schema")
				}
				var restored ObservationRecord
				if err := json.Unmarshal(after, &restored); err != nil {
					t.Fatal(err)
				}
				if endpointOfHMC321(FinalizeRuntimeRawMeasurement(restored)) != runtimeMeasurementQueryEndUnknown {
					t.Fatal("JSON restored a producer endpoint")
				}
			})
		}
	}
}

func TestHMC321EndpointFinalizationOnlyDecoratesCurrentProjection(t *testing.T) {
	original := PublishRuntimeMeasurementQueryWindow(intervalEndpointRecordHMC321(t), 0, 1, false)
	for name, change := range map[string]func(*ObservationRecord, *RuntimeMeasurementPublication){
		"decoration": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.SourceRef.TimeDomain, r.SourceRef.CanonicalTimeDomain, r.SourceRef.ClockAlignment = "capture", "monotonic", "calibrated"
			r.SourceRef.ClockCalibrated = true
			offset, slope := .1, 1.0
			r.SourceRef.ClockOffsetSec, r.SourceRef.ClockSlope = &offset, &slope
			r.Span.Selector, r.SupportRefs = "source lines 1-10", []string{"current source"}
		},
		"row":    func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Rows[0][0] = "invented" },
		"column": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Columns[0] = "wrong unit" },
		"note": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{"invented boundary"}
		},
		"window":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryWindowEndTs = 2 },
		"target":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryTargetPID = 33 },
		"lines":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryLineRangeKnown = true },
		"path":    func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.Path += ".other" },
		"payload": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.PayloadRef += ".other" },
		"query":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryScopeID += ":other" },
		"span":    func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Span.EndTs = 4 },
		"role": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.Role = AnswerAggregateRolePrincipalAnswer
		},
		"authority": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.ClaimAuthority = ObservationClaimAuthorityIndependentlyProven
		},
		"value": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Value = "31" },
		"unit":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Unit = "device residence" },
	} {
		t.Run(name, func(t *testing.T) {
			r := rewriteRawProjectionHMC268(t, original, change)
			if name == "decoration" {
				r.RichNotes = append(r.RichNotes, TraceNoteKeyCapacityTruncated+"=true")
			}
			if endpointOfHMC321(r) != runtimeMeasurementQueryEndUnknown {
				t.Fatal("unfinalized mutation retained endpoint authority")
			}
			r = FinalizeRuntimeRawMeasurement(r)
			r = PublishRuntimeMeasurementQueryWindow(r, 0, 1, false)
			want := runtimeMeasurementQueryEndUnknown
			if name == "decoration" {
				want = runtimeMeasurementQueryEndExcluded
			}
			if endpointOfHMC321(r) != want || endpointOfHMC321(original) != runtimeMeasurementQueryEndExcluded {
				t.Fatal("finalizer repaired altered facts or modified the immutable original seal")
			}
		})
	}
}

func TestHMC321EndpointPublisherRequiresExactNativeInterval(t *testing.T) {
	for _, mode := range []string{"legacy", "point", "missing", "lines", "start", "end", "nan", "infinite", "role", "predicate", "no_publication", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			r := intervalEndpointRecordHMC321(t)
			start, end := 0.0, 1.0
			if mode == "point" || mode == "missing" || mode == "lines" {
				r = rewriteRawProjectionHMC268(t, r, func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
					switch mode {
					case "point":
						r.SourceRef.QueryWindowEndTs = 0
					case "missing":
						r.SourceRef.QueryWindowKnown = false
					case "lines":
						r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart = true, 1
					}
				})
			}
			switch mode {
			case "start":
				start = .1
			case "end":
				end = 2
			case "nan":
				start = math.NaN()
			case "infinite":
				end = math.Inf(1)
			case "role":
				r.Role = AnswerAggregateRolePrincipalAnswer
			case "predicate":
				r.Predicate = "measurements_observation"
			case "no_publication":
				r.RichNotes = nil
			case "duplicate":
				r.RichNotes = append(r.RichNotes, r.RichNotes[0])
			}
			if mode != "legacy" {
				r = PublishRuntimeMeasurementQueryWindow(r, start, end, false)
			}
			if r.runtimeRawMeasurementPublication != nil || endpointOfHMC321(r) != runtimeMeasurementQueryEndUnknown {
				t.Fatal("unsupported or mismatched scope acquired endpoint metadata")
			}
		})
	}
}

func TestHMC321EndpointConsumerKeepsCurrentSourceAndCloneBoundaries(t *testing.T) {
	input := hmc277CurrentMeasurementInput(t)
	r := &input.ToolResults[0].Observations[0]
	r.Role = AnswerAggregateRoleSupportingCoverage
	*r = PublishRuntimeMeasurementQueryWindow(*r, 0, 1, false)
	before, _ := json.Marshal(input.ToolResults)
	contract := BuildRuntimeMeasurementContract(input)
	if len(contract.Choices()) != 2 {
		t.Fatal("lost complete native tables")
	}
	for _, table := range contract.Choices() {
		basis, owned := table.NativeSourceTitleBasis()
		if !owned || basis.QueryWindow != "[0,1) s" || !table.HasNativeMeasurementFacts() || table.HasNativeRawFacts() {
			t.Fatal("endpoint changed ordinary publication authority or lost its current source")
		}
		if !reflect.DeepEqual(table.Clone(), table) {
			t.Fatal("immutable clone lost endpoint metadata")
		}
		wire, _ := json.Marshal(table)
		restored := table.Clone()
		if err := json.Unmarshal(wire, &restored); err != nil {
			t.Fatal(err)
		}
		if _, owned := restored.NativeSourceTitleBasis(); owned {
			t.Fatal("reused JSON receiver retained native title authority")
		}
	}
	after, _ := json.Marshal(input.ToolResults)
	if string(before) != string(after) {
		t.Fatal("compiler changed original native evidence")
	}
	if err := os.WriteFile(r.SourceRef.Path, []byte("different source generation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, table := range BuildRuntimeMeasurementContract(input).Choices() {
		if _, owned := table.NativeSourceTitleBasis(); owned || table.HasNativeMeasurementFacts() {
			t.Fatal("endpoint seal bypassed current source-generation validation")
		}
	}
}

func TestHMC321EndpointWitnessPrivateImmutableShape(t *testing.T) {
	typ := reflect.TypeOf(runtimeRawMeasurementWitness{})
	if typ.NumField() != 3 {
		t.Fatal("classify every private witness field before immutable sharing")
	}
	for i, want := range []reflect.Type{reflect.TypeOf(""), reflect.TypeOf(""), reflect.TypeOf(runtimeMeasurementQueryEndUnknown)} {
		field := typ.Field(i)
		if field.PkgPath == "" || field.Type != want {
			t.Fatal("witness acquired public or mutable state")
		}
	}
	if runtimeMeasurementQueryEndUnknown != 0 {
		t.Fatal("zero/legacy endpoint must remain unknown")
	}
}
