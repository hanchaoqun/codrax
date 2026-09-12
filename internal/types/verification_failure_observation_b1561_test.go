package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func b1561FailureReport(t *testing.T) *ChangeReport {
	t.Helper()
	var report ChangeReport
	if err := json.Unmarshal([]byte(`{"plan_id":"p-current","passed":true,"test_results":[{"assertion_id":"native-pass","passed":true}],"verification_diagnostics":[{"source":"pre_suite_verification_probe","category":"probe_comparator_authority","reason_code":"model_authored_probe_comparator_unverified","runner":"verification_probe","outcome":"observed_failure","failure_observations":[{"assertion_id":"model-check","suite":"verification_probe/python","failure_detail":"assert expected == actual\nexpected: [1,  2]","output_ref":"/outputs/current  probe.txt"}]}]}`), &report); err != nil {
		t.Fatal(err)
	}
	return &report
}

func TestB1561FailureObservationMergeIsExactAndDetached(t *testing.T) {
	base := VerificationFailureObservation{AssertionID: "A", Suite: "S", FailureDetail: "a  b\n\t", OutputRef: "/Case Output"}
	variants := []VerificationFailureObservation{base, base, base, base}
	variants[0].AssertionID = "a"
	variants[1].Suite = "s"
	variants[2].FailureDetail = "a b\n\t"
	variants[3].OutputRef = "/case Output"
	input := append([]VerificationFailureObservation{base}, variants...)
	want := append([]VerificationFailureObservation(nil), input...)
	got := MergeVerificationFailureObservations(input, []VerificationFailureObservation{base})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge changed exact tuple/order: %+v", got)
	}
	got[0].FailureDetail = "changed"
	if !reflect.DeepEqual(input, want) || MergeVerificationFailureObservations() != nil {
		t.Error("merge aliased input or invented an observation")
	}
}

func TestB1561FailureObservationCollectionUsesOnlyTypedDiagnosticLane(t *testing.T) {
	report := b1561FailureReport(t)
	base := report.VerificationDiagnostics[0]
	want := base.FailureObservations
	for _, mutate := range []func(*VerificationDiagnostic){
		func(d *VerificationDiagnostic) { d.Category = "other" },
		func(d *VerificationDiagnostic) { d.ReasonCode = "other" },
		func(d *VerificationDiagnostic) { d.Runner = "go" },
		func(d *VerificationDiagnostic) { d.Outcome = "passed" },
	} {
		d := base
		mutate(&d)
		negative := &ChangeReport{VerificationDiagnostics: []VerificationDiagnostic{d}, TestResults: []TestResult{{Passed: false, FailureDetail: "do not infer this row"}}}
		if got := CurrentReportFailureObservations(negative); len(got) != 0 {
			t.Fatalf("unrelated diagnostic/raw result gained a model-probe observation: %+v", got)
		}
	}
	for _, passed := range []bool{false, true} {
		report.Passed = passed
		report.VerificationDiagnostics = []VerificationDiagnostic{base, base}
		if got := CurrentReportFailureObservations(report); !reflect.DeepEqual(got, want) {
			t.Fatalf("passed=%v lost typed observation or duplicated it: %+v", passed, got)
		}
	}
	if CurrentReportFailureObservations(nil) != nil {
		t.Fatal("nil report invented observations")
	}
	legacy, _ := json.Marshal(VerificationDiagnostic{Category: "probe_comparator_authority"})
	if bytes.Contains(legacy, []byte("failure_observations")) {
		t.Fatal("new optional field changed legacy JSON")
	}
}

func TestB1561FailureObservationDisplayIsBoundedAndExplicit(t *testing.T) {
	rows := []VerificationFailureObservation{{AssertionID: "exact  ID", Suite: "suite", FailureDetail: "actual\n  expected"},
		{AssertionID: strings.Repeat("字\n", 500), Suite: strings.Repeat("suite", 300), FailureDetail: strings.Repeat("\t\n字  ", 400), OutputRef: strings.Repeat("/long  ref", 100)}}
	for i := 2; i < 6; i++ {
		rows = append(rows, VerificationFailureObservation{AssertionID: fmt.Sprint(i), FailureDetail: "tail", OutputRef: "/outputs/ref"})
	}
	before := append([]VerificationFailureObservation(nil), rows...)
	for _, chinese := range []bool{false, true} {
		got := RenderVerificationFailureObservations(rows, chinese)
		for _, line := range strings.Split(got, "\n") {
			if !utf8.ValidString(line) || utf8.RuneCountInString(line) > writeContextPackTextLen {
				t.Errorf("bounded line invalid or over budget (%d): %q", utf8.RuneCountInString(line), line)
			}
		}
		for _, want := range []string{`exact  ID`, `actual\n  expected`} {
			if !strings.Contains(got, want) {
				t.Errorf("lost exact excerpt/missing-ref boundary %q: %s", want, got)
			}
		}
		if chinese {
			for _, want := range []string{"省略2项", "已截断", "完整引用保留于原始报告", "不等于已证实产品缺陷", "完整输出=不可用"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing Chinese disclosure %q: %s", want, got)
				}
			}
			for _, forbidden := range []string{"assertion_id", "failure_detail", "output_ref", "unavailable", "探针"} {
				if strings.Contains(got, forbidden) {
					t.Errorf("Chinese display leaked internal wording %q", forbidden)
				}
			}
		} else {
			for _, want := range []string{"omitted=2", "truncated; omitted", "complete field remains in report JSON", "not prove a product defect", "output_ref=unavailable"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing English disclosure %q: %s", want, got)
				}
			}
		}
		if strings.Contains(got, "/long  ref") {
			t.Error("long ref became a misleading partial actionable path")
		}
	}
	if !reflect.DeepEqual(rows, before) || RenderVerificationFailureObservations(nil, false) != "" {
		t.Error("renderer mutated raw observations or invented an empty warning")
	}
}

func TestB1561AtomicFailureContextSurvivesCapScopeAndReplay(t *testing.T) {
	report := b1561FailureReport(t)
	pack := WriteContextPackFromChangeReport(report).WithScope("batch", "slice")
	var atom WriteContextItem
	for _, item := range pack.Items {
		if item.Kind == "verification_failure_observation" {
			atom = item
		}
	}
	if atom.ID == "" || atom.SourceID != report.PlanID || atom.BatchID != "batch" || atom.SliceID != "slice" || atom.Text != RenderVerificationFailureObservations(CurrentReportFailureObservations(report), false) {
		t.Fatalf("observation was not one complete exact scoped item: %+v", atom)
	}
	crowded := WriteContextPack{PackID: "crowded", BatchID: "batch"}
	for i := 0; i < writeContextPackMaxItems; i++ {
		crowded.Items = append(crowded.Items, WriteContextItem{ID: fmt.Sprint(i), Kind: "background", Priority: WriteContextP3, Text: "background"})
	}
	crowded.Items = append(crowded.Items, atom)
	crowded = NormalizeWriteContextPack(crowded)
	if len(crowded.Items) != writeContextPackMaxItems {
		t.Fatal("pack cap changed")
	}
	for _, scope := range []struct {
		batch, slice string
		want         bool
	}{{"batch", "slice", true}, {"other", "slice", false}, {"batch", "other", false}} {
		found := false
		for _, item := range crowded.ViewForScope(WriteConsumerPlanner, 100, scope.batch, scope.slice).Items {
			if item.Kind == atom.Kind {
				found = item.Text == atom.Text
			}
		}
		if found != scope.want {
			t.Errorf("scope %+v crossed or lost atomic observation", scope)
		}
	}
	newReport := b1561FailureReport(t)
	newReport.VerificationDiagnostics[0].FailureObservations[0].FailureDetail = "new current observation"
	latest := WriteContextPackFromChangeReport(newReport).WithScope("batch", "slice")
	merged := MergeWriteContextPacks("batch", "", pack, latest, latest)
	count := 0
	for _, item := range merged.Items {
		if item.Kind == atom.Kind {
			count++
			if !strings.Contains(item.Text, "new current observation") || strings.Contains(item.Text, "expected: [1,") {
				t.Error("same-plan merge combined old/new observation pieces")
			}
		}
	}
	if count != 1 {
		t.Fatalf("replay did not retain exactly one atomic group: %d", count)
	}
	if len(verificationFailureObservationContextItems(&ChangeReport{PlanID: report.PlanID, Passed: true})) != 0 {
		t.Error("empty current report emitted a fabricated observation")
	}
	// Empty later input does not relabel an existing persisted pack item as
	// current. Consumers with a current report must prefer it even when empty.
	historical := MergeWriteContextPacks("batch", "", pack, WriteContextPackFromChangeReport(&ChangeReport{PlanID: report.PlanID, Passed: true}).WithScope("batch", "slice"))
	var preserved bool
	for _, item := range historical.Items {
		if item.Kind == atom.Kind {
			preserved = item.Text == atom.Text
		}
	}
	if !preserved {
		t.Error("empty report destroyed original historical observation")
	}
}

func TestB1561FinalObservationPersistenceAndClone(t *testing.T) {
	report := b1561FailureReport(t)
	want := CurrentReportFailureObservations(report)
	final := BuildWriteFinalReport(WriteFinalReportInput{Report: report})
	path := filepath.Join(t.TempDir(), "final.json")
	if err := WriteFinalReportToFile(&final, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWriteFinalReportFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Verification.FailureObservations, want) {
		t.Fatal("durable final report lost full observation bytes")
	}
	final.Verification.FailureObservations[0].FailureDetail = "mutated projection"
	if !reflect.DeepEqual(CurrentReportFailureObservations(report), want) {
		t.Fatal("final summary aliased original diagnostic rows")
	}
}

func TestB1561FailureHandoffClonesObservationWithoutChangingAuthority(t *testing.T) {
	report := b1561FailureReport(t)
	report.Passed = false
	report.FailureKind = FailureKindTestsFailed
	before, _ := json.Marshal(report)
	handoff := BuildVerifyFailureHandoff(report, "batch", 1, "", "")
	if handoff == nil || len(handoff.Diagnostics) != 1 || len(handoff.Diagnostics[0].FailureObservations) != 1 {
		t.Fatalf("failed report lost existing diagnostic lane: %+v", handoff)
	}
	handoff.Diagnostics[0].FailureObservations[0].FailureDetail = "mutated handoff"
	after, _ := json.Marshal(report)
	if !bytes.Equal(before, after) {
		t.Fatal("nested handoff slice aliased source report")
	}
}

func TestB1561CurrentFailureObservationSurvivesPublicContextAndFinalReport(t *testing.T) {
	report := b1561FailureReport(t)
	before, _ := json.Marshal(report)
	passed, total := report.Score()
	profile := BuildVerificationProofProfile(nil, report)
	pack := WriteContextPackFromChangeReport(report)
	for _, consumer := range []WriteContextConsumer{WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier} {
		var texts []string
		for _, item := range pack.View(consumer, 100).Items {
			if item.Kind == "verification_failure_observation" {
				texts = append(texts, item.Text)
			}
		}
		joined := strings.Join(texts, "\n")
		for _, want := range []string{"model-check", "expected: [1,  2]", "/outputs/current  probe.txt", "not prove a product defect"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s lost independent observed failure %q: %s", consumer, want, joined)
			}
		}
	}
	for _, item := range pack.View(WriteConsumerApproval, 100).Items {
		if item.Kind == "verification_failure_observation" {
			t.Error("failure observation became an approval input")
		}
	}
	final := BuildWriteFinalReport(WriteFinalReportInput{Report: report})
	encoded, _ := json.Marshal(final.Verification)
	if !bytes.Contains(encoded, []byte(`"failure_observations"`)) || !bytes.Contains(encoded, []byte("model-check")) {
		t.Errorf("final summary lost independent failure observation: %s", encoded)
	}
	if !final.Verification.Passed || final.Verification.TestCount != total || final.Verification.PassedCount != passed {
		t.Error("display changed the original verification verdict/counts")
	}
	if BuildVerifyFailureHandoff(report, "batch", 1, "", "") != nil {
		t.Error("observation changed the passed-report handoff branch")
	}
	after, _ := json.Marshal(report)
	afterProfile, _ := json.Marshal(BuildVerificationProofProfile(nil, report))
	beforeProfile, _ := json.Marshal(profile)
	if !bytes.Equal(before, after) || !bytes.Equal(beforeProfile, afterProfile) {
		t.Error("display changed original report/proof bytes")
	}
}
