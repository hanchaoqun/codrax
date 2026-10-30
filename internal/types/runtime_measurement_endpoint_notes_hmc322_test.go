package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A real current-source/replay receipt surrounds this provider-protocol
// fixture. The private endpoint is issued only through its native publisher;
// no test creates or edits a private witness to make the positive path pass.
func endpointNotesInputHMC322(t *testing.T, inclusive bool) ObservationLedgerInput {
	t.Helper()
	input := hmc277CurrentMeasurementInput(t)
	r := &input.ToolResults[0].Observations[0]
	r.Role = AnswerAggregateRoleSupportingCoverage
	p, ok := DecodeRuntimeMeasurementPublication(*r)
	if !ok || len(p.Tables) != 2 {
		t.Fatal("HARNESS: expected two valid current native views")
	}
	p.Tables[0].Notes = []string{"scope A", "shared note", "scope A", "tail A"}
	p.Tables[1].Notes = []string{"scope B", "shared note", "tail B"}
	hmc277SetMeasurementPublication(t, r, p)
	*r = PublishRuntimeMeasurementQueryWindow(*r, 0, 1, inclusive)
	if endpointOfHMC321(*r) != runtimeMeasurementNativeQueryEnd(inclusive) {
		t.Fatal("HARNESS: native endpoint was not published")
	}
	return input
}

func TestHMC322EndpointOptionalNotesDeletionKeepsCurrentPresentation(t *testing.T) {
	for _, inclusive := range []bool{false, true} {
		for _, mode := range []string{"empty", "non_prefix", "duplicate_subset", "second_view_only"} {
			t.Run(map[bool]string{false: "excluded/", true: "included/"}[inclusive]+mode, func(t *testing.T) {
				input := endpointNotesInputHMC322(t, inclusive)
				original := input.ToolResults[0].Observations[0]
				originalWire, _ := json.Marshal(original)
				before := BuildRuntimeMeasurementContract(input).Choices()
				if len(before) != 2 {
					t.Fatal("HARNESS: real current-source contract lost a native view")
				}
				changed := rewriteRawProjectionHMC268(t, original, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
					switch mode {
					case "empty":
						p.Tables[0].Notes, p.Tables[1].Notes = nil, []string{}
					case "non_prefix":
						p.Tables[0].Notes = []string{p.Tables[0].Notes[0], p.Tables[0].Notes[3]}
						p.Tables[1].Notes = []string{p.Tables[1].Notes[1]}
					case "duplicate_subset":
						p.Tables[0].Notes = []string{p.Tables[0].Notes[0], p.Tables[0].Notes[2]}
					case "second_view_only":
						p.Tables[1].Notes = p.Tables[1].Notes[1:]
					}
				})
				p, ok := DecodeRuntimeMeasurementPublication(changed)
				if !ok {
					t.Fatal("optional-note deletion invalidated the provider protocol")
				}
				input.ToolResults[0].Observations[0] = changed
				beforeCompile, _ := json.Marshal(input.ToolResults)
				after := BuildRuntimeMeasurementContract(input).Choices()
				if len(after) != len(before) {
					t.Fatal("optional-note deletion changed the native view population")
				}
				for i, table := range after {
					want := before[i].Clone()
					// Match the existing Clone/ShortProviderNotes empty-list
					// representation; every fact and authority check stays exact.
					want.Notes = append([]string(nil), p.Tables[i].Notes...)
					wantWire, _ := json.Marshal(want)
					gotWire, _ := json.Marshal(table)
					if string(gotWire) != string(wantWire) || !table.HasNativeMeasurementFacts() || table.HasNativeRawFacts() || table.HasNativeResourceFacts() || table.HasNativeSamplingFacts() {
						t.Fatalf("fresh current display changed facts or borrowed another authority lane: view=%s facts=%t raw=%t resource=%t sampling=%t\nwant=%s\ngot=%s", table.View, table.HasNativeMeasurementFacts(), table.HasNativeRawFacts(), table.HasNativeResourceFacts(), table.HasNativeSamplingFacts(), wantWire, gotWire)
					}
					oldTitle, oldNotes, oldOwned := before[i].NativeSourcePresentation("en")
					title, notes, owned := table.NativeSourcePresentation("en")
					if !oldOwned || !owned || oldTitle != title || !reflect.DeepEqual(oldNotes, notes) {
						t.Fatal("legal deletion lost the native endpoint/source presentation")
					}
					if !reflect.DeepEqual(before[i].Notes, want.Notes) {
						if _, _, staleOwned := want.NativeSourcePresentation("en"); staleOwned || want.HasNativeMeasurementFacts() {
							t.Fatal("changed table borrowed an old full display seal")
						}
					}
					if !reflect.DeepEqual(table.Clone(), table) {
						t.Fatal("clone lost fresh current source presentation")
					}
				}
				afterCompile, _ := json.Marshal(input.ToolResults)
				afterOriginal, _ := json.Marshal(original)
				if string(beforeCompile) != string(afterCompile) || string(originalWire) != string(afterOriginal) || endpointOfHMC321(original) != runtimeMeasurementNativeQueryEnd(inclusive) {
					t.Fatal("deletion/compilation changed the immutable original or query evidence")
				}
			})
		}
	}
}

func TestHMC322EndpointNotesDeletionRejectsChangesOutsideProjection(t *testing.T) {
	for name, change := range map[string]func(*ObservationRecord, *RuntimeMeasurementPublication){
		"note_addition": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = append(p.Tables[0].Notes[:1], "new note")
		},
		"note_replacement": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{"invented boundary"}
		},
		"note_reorder": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{"tail A", "scope A"}
		},
		"note_extra_duplicate": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{"scope A", "scope A", "scope A"}
		},
		"note_from_sibling": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{p.Tables[1].Notes[0]}
		},
		"row": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Rows[0][0] = "changed" },
		"column": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Columns[0] = "device residence"
		},
		"view_swap": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0], p.Tables[1] = p.Tables[1], p.Tables[0]
		},
		"source": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.Path += ".other" },
		"query":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryScopeID += ".other" },
		"window": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryWindowEndTs = 2 },
		"target": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryTargetPID = 123 },
		"unit":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Unit = "thread wait" },
	} {
		t.Run(name, func(t *testing.T) {
			input := endpointNotesInputHMC322(t, false)
			original := input.ToolResults[0].Observations[0]
			changed := rewriteRawProjectionHMC268(t, original, func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
				p.Tables[1].Notes = p.Tables[1].Notes[:1] // a legitimate deletion cannot hide another edit
				change(r, p)
			})
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndUnknown {
				t.Fatal("non-deletion mutation retained native endpoint")
			}
			changed = FinalizeRuntimeRawMeasurement(changed)
			changed = PublishRuntimeMeasurementQueryWindow(changed, 0, 1, false)
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndUnknown || endpointOfHMC321(original) != runtimeMeasurementQueryEndExcluded {
				t.Fatal("finalization repaired changed facts or mutated the original witness")
			}
		})
	}
}

func TestHMC322EndpointNotesDeletionFinalizationIsImmutable(t *testing.T) {
	for _, order := range []string{"delete_before_decoration", "delete_after_decoration"} {
		t.Run(order, func(t *testing.T) {
			input := endpointNotesInputHMC322(t, false)
			original := input.ToolResults[0].Observations[0]
			beforeWitness := *original.runtimeRawMeasurementPublication
			shorten := func(r ObservationRecord) ObservationRecord {
				return rewriteRawProjectionHMC268(t, r, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
					p.Tables[0].Notes = []string{p.Tables[0].Notes[3]}
					p.Tables[1].Notes = nil
				})
			}
			changed := original
			if order == "delete_before_decoration" {
				changed = shorten(changed)
			}
			changed = rewriteRawProjectionHMC268(t, changed, func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
				r.SourceRef.TimeDomain, r.SourceRef.CanonicalTimeDomain, r.SourceRef.ClockAlignment = "capture", "monotonic", "calibrated"
				r.Span.Selector, r.SupportRefs = "native source decoration", []string{"current original"}
			})
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndUnknown {
				t.Fatal("source decoration bypassed native finalization")
			}
			changed = FinalizeRuntimeRawMeasurement(changed)
			if order == "delete_after_decoration" {
				changed = shorten(changed)
			}
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndExcluded || runtimeRawMeasurementValid(changed) {
				t.Fatal("legal deletion/source decoration lost endpoint or gained raw authority")
			}
			changed = FinalizeRuntimeRawMeasurement(changed)
			changed = PublishRuntimeMeasurementQueryWindow(changed, 0, 1, true)
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndExcluded || *original.runtimeRawMeasurementPublication != beforeWitness {
				t.Fatal("repeated finalization replaced an endpoint or its immutable baseline")
			}
		})
	}
}

func TestHMC322EndpointNotesDeletionKeepsUnknownAndRawBoundaries(t *testing.T) {
	for _, mode := range []string{"legacy", "fresh_json", "raw"} {
		t.Run(mode, func(t *testing.T) {
			input := endpointNotesInputHMC322(t, false)
			original := input.ToolResults[0].Observations[0]
			if mode == "legacy" {
				input = hmc277CurrentMeasurementInput(t)
				original = input.ToolResults[0].Observations[0]
			} else if mode == "fresh_json" {
				wire, err := json.Marshal(original)
				if err != nil {
					t.Fatal(err)
				}
				var restored ObservationRecord
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				original = restored
			} else {
				original = measurementProviderRecord(t)
				original.Predicate, original.Role = "measurements_observation", AnswerAggregateRoleSupportingCoverage
				original = rewriteRawProjectionHMC268(t, original, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
					p.Tables[0].Notes = []string{"raw producer scope"}
				})
				var ok bool
				original, ok = PublishRuntimeRawMeasurement(original)
				if !ok || !runtimeRawMeasurementValid(original) {
					t.Fatal("HARNESS: missing strict raw source witness")
				}
			}
			changed := rewriteRawProjectionHMC268(t, original, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
				for i := range p.Tables {
					p.Tables[i].Notes = nil
				}
			})
			changed = FinalizeRuntimeRawMeasurement(changed)
			if endpointOfHMC321(changed) != runtimeMeasurementQueryEndUnknown || mode == "raw" && runtimeRawMeasurementValid(changed) {
				t.Fatal("deletion minted unknown endpoint or weakened raw semantic notes")
			}
			if mode != "raw" {
				input.ToolResults[0].Observations[0] = changed
				choices := BuildRuntimeMeasurementContract(input).Choices()
				if len(choices) != 2 {
					t.Fatal("unknown endpoint erased otherwise valid current measurements")
				}
				for _, table := range choices {
					basis, owned := table.NativeSourceTitleBasis()
					if !owned || basis.QueryWindow != "0–1 s (endpoint inclusion unavailable)" || !table.HasNativeMeasurementFacts() || table.HasNativeRawFacts() {
						t.Fatal("legacy/JSON source title borrowed a native endpoint")
					}
				}
			}
		})
	}
}
