package types

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// This is a contract-boundary fixture, not a trace parser or device-capture
// acceptance. The source witness is issued through the real current-read API;
// no test assigns the new fixed-fact seal directly.
func hmc277CurrentMeasurementInput(t *testing.T) ObservationLedgerInput {
	t.Helper()
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.systrace")
	m := NewMutableState("Display the measured facts in their original scope.")
	read := m.PrepareTraceQuerySourceRead(path)
	r := measurementProviderRecord(t)
	r.SourceRef.Path, r.SourceRef.ArtifactKind = read.Path(), "trace"
	p, ok := DecodeRuntimeMeasurementPublication(measurementProviderRecord(t))
	if !ok {
		t.Fatal("invalid existing native publication fixture")
	}
	p.Source = r.SourceRef
	p.Tables[0].Notes = []string{"request residence, not thread wait; unknown size remains unknown"}
	member := p.Tables[0].Clone()
	member.View = RuntimeMeasurementMembers
	member.Label = "Independent request members"
	member.Rows = [][]string{{"9007199254740993", "NULL"}, {"0", "unavailable"}}
	p.Tables = append(p.Tables, member)
	hmc277SetMeasurementPublication(t, &r, p)
	result := ToolResult{ToolName: "trace_query", Success: true,
		TraceQuerySourceRead: TraceQueryPhysicalSourceReadCandidate(read.Path()), Observations: []ObservationRecord{r}}
	m.StampTraceQuerySourceRead(read, &result)
	m.StampTraceQueryWindowReplay(read, &result, json.RawMessage(`{"source":"path","view":"window_stats","time_start":0,"time_end":1}`))
	if source, _, current := m.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !current || source != r.SourceRef.Path {
		t.Fatal("fixture did not acquire a real current-source replay witness")
	}
	start, end := 0.0, 1.0
	return ObservationLedgerInput{ToolResults: []ToolResult{result}, runtimeMeasurementPairConsumer: m,
		RequestModel: &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
			RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}}
}

func hmc277SetMeasurementPublication(t *testing.T, r *ObservationRecord, p RuntimeMeasurementPublication) {
	t.Helper()
	p.Source = r.SourceRef
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
}

func TestHMC277FixedMeasurementFactsSealFullPublicTable(t *testing.T) {
	input := hmc277CurrentMeasurementInput(t)
	before, err := json.Marshal(input.ToolResults)
	if err != nil {
		t.Fatal(err)
	}
	contract := BuildRuntimeMeasurementContract(input)
	if got := len(contract.Choices()); got != 2 {
		t.Fatalf("current complete native query returned %d tables, want 2", got)
	}
	for _, table := range contract.Choices() {
		if !table.IsValid() || !table.HasNativeMeasurementFacts() || !table.HasNativeFixedFacts() || table.HasNativeRawFacts() || table.HasNativeSamplingFacts() || table.HasNativeResourceFacts() {
			t.Fatal("ordinary current measurement did not acquire only its generic fixed display qualification")
		}
		clone := table.Clone()
		if !clone.HasNativeMeasurementFacts() || !reflect.DeepEqual(clone, table) {
			t.Fatal("Clone lost current private qualification or changed public values")
		}
		for name, edit := range map[string]func(*RuntimeMeasurementTable){
			"label":          func(v *RuntimeMeasurementTable) { v.Label += " changed" },
			"row":            func(v *RuntimeMeasurementTable) { v.Rows[0][0] = "invented" },
			"columns":        func(v *RuntimeMeasurementTable) { v.Columns[0] = "invented physical unit" },
			"notes":          func(v *RuntimeMeasurementTable) { v.Notes = append(v.Notes, "root cause proven") },
			"view":           func(v *RuntimeMeasurementTable) { v.View = RuntimeMeasurementTimeline },
			"observation_id": func(v *RuntimeMeasurementTable) { v.ObservationID += "-other" },
			"default":        func(v *RuntimeMeasurementTable) { v.DefaultPresentation = !v.DefaultPresentation },
			"population": func(v *RuntimeMeasurementTable) {
				v.DisplayPopulation = &RuntimeMeasurementDisplayPopulation{QueryScopeID: "invented", RowIDs: []string{"one"}, MatchedRows: 1}
			},
		} {
			t.Run(string(table.View)+"/"+name, func(t *testing.T) {
				changed := table.Clone()
				edit(&changed)
				if changed.HasNativeMeasurementFacts() || changed.HasNativeFixedFacts() || changed.IsValid() {
					t.Fatal("public table mutation retained a current fixed-fact qualification")
				}
				if !table.HasNativeMeasurementFacts() || !reflect.DeepEqual(table, clone) {
					t.Fatal("editing a clone changed the original table")
				}
			})
		}
	}
	after, err := json.Marshal(input.ToolResults)
	if err != nil || string(before) != string(after) {
		t.Fatal("display qualification mutated the native query observations")
	}
}

func TestHMC277FixedMeasurementFactsJSONCannotRetainPrivateAuthority(t *testing.T) {
	table := BuildRuntimeMeasurementContract(hmc277CurrentMeasurementInput(t)).Choices()[0]
	// Every older private marker must also disappear on a reused receiver,
	// not merely the generic seal introduced by this batch.
	table.nativeEventData, table.nativeLogDisplayPopulation, table.nativeTraceDisplayPopulation = true, true, true
	table.nativeEventSelectionKey, table.nativeFactIdentity = "private-selection", "private-fact"
	table.nativeEventScopeNoteIndex, table.nativeEventRequestExcluded, table.nativeEventTargetExcluded = 3, 4, 5
	table.coverageScope = &runtimeMeasurementCoverageScope{path: "/private"}
	table.nativeSourcePresentation = &runtimeMeasurementSourcePresentation{}
	table.nativeSamplingFactsDigest = runtimeSamplingDigest(table)
	table.nativeResourceFactsDigest = runtimeSamplingDigest(table)
	table.nativeRawFactsDigest = runtimeSamplingDigest(table)
	encoded, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	for _, reuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "reused"}[reuse], func(t *testing.T) {
			var decoded RuntimeMeasurementTable
			if reuse {
				decoded = table.Clone()
			}
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.HasNativeFixedFacts() || decoded.HasNativeMeasurementFacts() || decoded.nativeEventData || decoded.nativeLogDisplayPopulation || decoded.nativeTraceDisplayPopulation ||
				decoded.nativeEventSelectionKey != "" || decoded.nativeFactIdentity != "" || decoded.nativeEventScopeNoteIndex != 0 || decoded.nativeEventRequestExcluded != 0 || decoded.nativeEventTargetExcluded != 0 ||
				decoded.coverageScope != nil || decoded.nativeSourcePresentation != nil || decoded.nativeSamplingFactsDigest != "" || decoded.nativeResourceFactsDigest != "" || decoded.nativeRawFactsDigest != "" || decoded.nativeMeasurementFactsDigest != "" {
				t.Fatal("JSON preserved or minted a private table capability")
			}
			roundTrip, err := json.Marshal(decoded)
			if err != nil || string(roundTrip) != string(encoded) {
				t.Fatal("revoking private authority lost public table facts")
			}
		})
	}
}

func TestHMC277FixedMeasurementFactsRequireCurrentCompleteWitness(t *testing.T) {
	for _, mode := range []string{"current", "memo_only", "cancelled", "unknown_window", "line_window", "foreign_run", "no_replay", "failed", "stale_source"} {
		t.Run(mode, func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			result := &input.ToolResults[0]
			switch mode {
			case "memo_only":
				result.ReusedFromRunMemo = true
			case "cancelled":
				// Even an earlier source witness cannot authorize a cancelled
				// query's newly attached fixed display qualification.
				result.TraceViewCancellation = &TraceViewCancellation{}
			case "unknown_window", "line_window":
				r := &result.Observations[0]
				p, ok := DecodeRuntimeMeasurementPublication(*r)
				if !ok {
					t.Fatal("invalid fixture before changing scope")
				}
				if mode == "unknown_window" {
					r.SourceRef.QueryWindowKnown = false
				} else {
					r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd = true, 1, 9
				}
				hmc277SetMeasurementPublication(t, r, p)
			case "foreign_run":
				input.runtimeMeasurementPairConsumer = NewMutableState("another run")
			case "no_replay":
				result.TraceQueryWindowReplay = TraceQueryWindowReplayRef{}
			case "failed":
				result.Success = false
			case "stale_source":
				if err := os.WriteFile(result.Observations[0].SourceRef.Path, []byte("changed source generation\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			qualified := 0
			for _, table := range BuildRuntimeMeasurementContract(input).Choices() {
				if table.HasNativeMeasurementFacts() {
					qualified++
				}
			}
			want := 0
			if mode == "current" {
				want = 2
			}
			if qualified != want {
				t.Fatalf("%s qualified %d fixed tables, want %d", mode, qualified, want)
			}
		})
	}
}

func TestHMC277FixedMeasurementFactsSiblingConflictIsSticky(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_first", true: "conflict_first"}[reverse], func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			original := input.ToolResults[0]
			conflict := original
			conflict.Observations = append([]ObservationRecord(nil), original.Observations...)
			p, ok := DecodeRuntimeMeasurementPublication(conflict.Observations[0])
			if !ok || len(p.Tables) != 2 {
				t.Fatal("fixture requires two sibling views")
			}
			p.Tables[1].Rows[0][0] = "conflicting sibling value"
			hmc277SetMeasurementPublication(t, &conflict.Observations[0], p)
			input.ToolResults = []ToolResult{original, conflict, original}
			if reverse {
				input.ToolResults = []ToolResult{conflict, original, conflict}
			}
			if contract := BuildRuntimeMeasurementContract(input); len(contract.Choices()) != 0 || contract.Active() {
				t.Fatal("unchanged summary sibling escaped a whole-publication conflict")
			}
		})
	}
}

func TestHMC277FixedMeasurementFactsIdenticalMemoDoesNotEraseFreshWitness(t *testing.T) {
	for _, memoFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "current_first", true: "memo_first"}[memoFirst], func(t *testing.T) {
			input := hmc277CurrentMeasurementInput(t)
			current := input.ToolResults[0]
			memo := current
			memo.ReusedFromRunMemo = true
			input.ToolResults = []ToolResult{current, memo}
			if memoFirst {
				input.ToolResults = []ToolResult{memo, current}
			}
			tables := BuildRuntimeMeasurementContract(input).Choices()
			if len(tables) != 2 {
				t.Fatal("identical current and memo facts spuriously conflict")
			}
			for _, table := range tables {
				if !table.HasNativeMeasurementFacts() {
					t.Fatal("identical memo erased an independently current qualification")
				}
			}
		})
	}
}
