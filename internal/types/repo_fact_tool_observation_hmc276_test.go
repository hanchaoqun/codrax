package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHMC276RepoFactToolObservationCopyAndRestore(t *testing.T) {
	r := ToolResult{ToolName: "exec_command", Summary: "7", RawRef: "/results/count.txt", Success: true,
		CommandMeasurement: &ToolCommandMeasurement{Kind: "count", Value: 7, Command: "awk 'BEGIN {print 7}'"}}
	f := NewToolResultRepoFact(r, "tool:exec_command", .8)
	want, ok := f.ToolObservation()
	if !ok || want.TypedRecords != 1 || len(want.Origins) != 1 || want.Origins[0] != AnswerEvidenceOriginCommandMeasurement {
		t.Fatalf("typed origin missing: %+v / %t", want, ok)
	}
	duplicate := r
	duplicate.Observations = []ObservationRecord{{ID: "tool:0#command_measurement", Origin: AnswerEvidenceOriginCommandMeasurement}}
	if display, _ := NewToolResultRepoFact(duplicate, f.Source, .8).ToolObservation(); display.TypedRecords != 1 {
		t.Fatalf("one exact producer/carrier record counted twice: %+v", display)
	}
	copy := f
	copy.Confidence = .001 // This field remains soft metadata, never a gate.
	got, ok := copy.ToolObservation()
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatal("ordinary copy lost immutable association or confidence became a gate")
	}
	got.Origins[0] = AnswerEvidenceOriginCurrentSource
	got.GroundingPolicies[0] = ClaimGroundingDisplayOnly
	again, _ := f.ToolObservation()
	if !reflect.DeepEqual(again, want) {
		t.Fatal("getter exposed mutable provenance")
	}
	r.CommandMeasurement.Origin = AnswerEvidenceOriginCurrentSource
	if again, _ := f.ToolObservation(); !reflect.DeepEqual(again, want) {
		t.Fatal("producer mutation rewrote a published snapshot")
	}
	for name, change := range map[string]func(*RepoFact){
		"tool":   func(f *RepoFact) { f.Key = "read_file" },
		"value":  func(f *RepoFact) { f.Value = "8" },
		"source": func(f *RepoFact) { f.Source = "other" },
		"ref":    func(f *RepoFact) { f.EvidenceRef = "/results/other.txt" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := f
			change(&copy)
			if _, ok := copy.ToolObservation(); ok {
				t.Fatal("changed fact retained producer association")
			}
		})
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "typed_records") || strings.Contains(string(data), "toolObservation") {
		t.Fatal("private provenance serialized")
	}
	for _, reused := range []bool{false, true} {
		restored := RepoFact{}
		if reused {
			restored = f
		}
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if _, ok := restored.ToolObservation(); ok || restored.Value != f.Value || restored.EvidenceRef != f.EvidenceRef {
			t.Fatalf("restore minted provenance or lost audit bytes: reused=%t %+v", reused, restored)
		}
	}
	legacy := RepoFact{Key: f.Key, Value: f.Value, Source: f.Source, EvidenceRef: f.EvidenceRef, Confidence: f.Confidence}
	if _, ok := legacy.ToolObservation(); ok {
		t.Fatal("identical public values reconstructed private association")
	}
	r.Success = false
	if _, ok := NewToolResultRepoFact(r, f.Source, .8).ToolObservation(); ok {
		t.Fatal("failed result became captured success")
	}
}
