package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// These are report-consumer fixtures, not a claim of native execution.
func TestExistingTestReceiptNativeInvocationIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangeReport)
		want string
	}{
		{"legacy", func(*ChangeReport) {}, "satisfied"},
		{"same_invocation", func(r *ChangeReport) { r.ExecutedCommands[0].InvocationID = "a"; r.TestResults[0].InvocationID = "a" }, "satisfied"},
		{"other_invocation", func(r *ChangeReport) { r.ExecutedCommands[0].InvocationID = "a"; r.TestResults[0].InvocationID = "b" }, "missing"},
		{"missing_result_identity", func(r *ChangeReport) { r.ExecutedCommands[0].InvocationID = "a" }, "missing"},
		{"missing_command_identity", func(r *ChangeReport) { r.TestResults[0].InvocationID = "a" }, "missing"},
		{"legacy_pair_in_mixed_report", func(r *ChangeReport) {
			r.ExecutedCommands = append(r.ExecutedCommands, ExecutedCommand{InvocationID: "other"})
		}, "missing"},
		{"legacy_pair_with_identified_other_result", func(r *ChangeReport) { r.TestResults = append(r.TestResults, TestResult{InvocationID: "other"}) }, "missing"},
		{"duplicate_command_identity", func(r *ChangeReport) {
			r.ExecutedCommands[0].InvocationID, r.TestResults[0].InvocationID = "a", "a"
			r.ExecutedCommands = append(r.ExecutedCommands, r.ExecutedCommands[0])
		}, "missing"},
		{"same_identity_later_row", func(r *ChangeReport) {
			r.ExecutedCommands[0].InvocationID, r.TestResults[0].InvocationID = "a", "b"
			own := r.TestResults[0]
			own.InvocationID = "a"
			r.TestResults = append(r.TestResults, own)
		}, "satisfied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, report := existingIntentConsumerFixture()
			tc.edit(report)
			assertExistingIntentConsumerStatuses(t, plan, report, tc.want)
		})
	}
}

func TestNativeInvocationFieldsJSONCompatibility(t *testing.T) {
	legacy := &ChangeReport{ExecutedCommands: []ExecutedCommand{{Runner: "go"}}, TestResults: []TestResult{{Suite: "example", AssertionID: "TestValue"}}}
	wire, err := json.Marshal(legacy)
	if err != nil || strings.Contains(string(wire), "invocation_id") {
		t.Fatalf("legacy JSON changed: %s, %v", wire, err)
	}
	legacy.ExecutedCommands[0].InvocationID, legacy.TestResults[0].InvocationID = "opaque\"identity", "opaque\"identity"
	digest := ExistingTestAssertionDigest(legacy.TestResults[0])
	copyRow := legacy.TestResults[0]
	copyRow.InvocationID = ""
	if ExistingTestAssertionDigest(copyRow) != digest {
		t.Fatal("invocation identity changed the existing assertion digest protocol")
	}
	wire, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var restored ChangeReport
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, &restored) {
		t.Fatal("JSON round trip changed invocation/result identity")
	}
}

func TestNativeTestInvocationIndex(t *testing.T) {
	for _, tc := range []struct {
		name       string
		commands   []string
		results    []string
		wantOwners [][]int
	}{
		{"legacy", []string{"", ""}, []string{"", ""}, [][]int{{0, 1}, {0, 1}}},
		{"one_command_many_suites", []string{"a"}, []string{"a", "a", "a"}, [][]int{{0, 1, 2}}},
		{"two_calls_same_candidate", []string{"a", "b"}, []string{"b", "a", "b"}, [][]int{{1}, {0, 2}}},
		{"empty_edges", []string{"a", ""}, []string{"", "a"}, [][]int{{1}, nil}},
		{"only_result_identified", []string{""}, []string{"a", ""}, [][]int{nil}},
		{"only_command_identified", []string{"a", ""}, []string{""}, [][]int{nil, nil}},
		{"duplicate_does_not_poison_other_id", []string{"a", "b", "a"}, []string{"a", "b"}, [][]int{nil, {1}, nil}},
		{"third_duplicate_stays_invalid", []string{"a", "a", "a"}, []string{"a"}, [][]int{nil, nil, nil}},
		{"byte_exact_opaque_identity", []string{"a", " a", "go@dir::a"}, []string{"a ", " a", "a", "go@dir::a"}, [][]int{{2}, {1}, {3}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &ChangeReport{}
			for _, id := range tc.commands {
				report.ExecutedCommands = append(report.ExecutedCommands, ExecutedCommand{InvocationID: id})
			}
			for _, id := range tc.results {
				report.TestResults = append(report.TestResults, TestResult{InvocationID: id})
			}
			before, _ := json.Marshal(report)
			index := NewNativeTestInvocationIndex(report)
			for ci := range tc.commands {
				for ri := range tc.results {
					want := false
					for _, owner := range tc.wantOwners[ci] {
						want = want || owner == ri
					}
					if got := index.Matches(ci, ri); got != want {
						t.Errorf("Matches(%d,%d)=%t, want %t", ci, ri, got, want)
					}
				}
			}
			for _, pair := range [][2]int{{-1, 0}, {0, -1}, {len(tc.commands), 0}, {0, len(tc.results)}} {
				if index.Matches(pair[0], pair[1]) {
					t.Errorf("out-of-bounds match: %v", pair)
				}
			}
			after, _ := json.Marshal(report)
			if string(before) != string(after) {
				t.Fatal("index mutated its report")
			}
			// The index owns its ID snapshot, not mutable report slices.
			old := index.Matches(0, 0)
			report.ExecutedCommands[0].InvocationID, report.TestResults[0].InvocationID = "replacement", "replacement"
			if index.Matches(0, 0) != old {
				t.Fatal("report mutation changed the existing index")
			}
		})
	}
	var absent *NativeTestInvocationIndex
	if absent.Matches(0, 0) || NewNativeTestInvocationIndex(nil).Matches(0, 0) {
		t.Fatal("nil report/index created a match")
	}
}

func TestExistingTestReceiptNativeInvocationFailureAndMultiplicity(t *testing.T) {
	for _, same := range []bool{false, true} {
		plan, report := existingIntentConsumerFixture()
		report.ExecutedCommands[0].InvocationID = "actual"
		report.ExecutedCommands[0].ExitCode = 1
		report.TestResults[0].InvocationID = "other"
		if same {
			report.TestResults[0].InvocationID = "actual"
		}
		report.TestResults[0].Passed = false
		report.ExistingTestExecutions[0].FailedAssertionCount = 1
		report.ExistingTestExecutions[0].AssertionDigests = []string{ExistingTestAssertionDigest(report.TestResults[0])}
		want := "missing"
		if same {
			want = "failed"
		}
		assertExistingIntentConsumerStatuses(t, plan, report, want)
	}
	plan, report := existingIntentConsumerFixture()
	report.ExecutedCommands[0].InvocationID, report.TestResults[0].InvocationID = "actual", "actual"
	other := report.TestResults[0]
	other.InvocationID = "other"
	report.TestResults = append(report.TestResults, other)
	report.ExistingTestExecutions[0].AssertionCount = 2
	digest := report.ExistingTestExecutions[0].AssertionDigests[0]
	report.ExistingTestExecutions[0].AssertionDigests = []string{digest, digest}
	assertExistingIntentConsumerStatuses(t, plan, report, "missing")
}
