package types

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const probeGranularityTestPhrase = "not per-contract/method execution receipts or runtime coverage"

func TestVerificationProbeGranularityContextUsesOnlyAdmittedWitnessShape(t *testing.T) {
	for _, category := range []string{"probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs"} {
		for _, tc := range []struct {
			name, status, source string
			witness              WriteBehaviorWitnessKind
			want                 bool
		}{
			{"stamped", "satisfied", "verification_probe", WriteBehaviorWitnessVerificationProbe, true},
			{"legacy category", "satisfied", "verification_probe", "", true},
			{"missing", "missing", "verification_probe", "", false},
			{"failed", "failed", "verification_probe", "", false},
			{"unavailable", "unavailable", "verification_probe", "", false},
			{"advisory", "advisory", "verification_probe", "", false},
			{"other source", "satisfied", "project_test_observation", "", false},
			{"contradictory stamp", "satisfied", "verification_probe", WriteBehaviorWitnessSourceText, false},
			{"unknown stamp", "satisfied", "verification_probe", "future", false},
		} {
			t.Run(category+"/"+tc.name, func(t *testing.T) {
				record := VerificationConfidenceRecord{
					Source: tc.source, Category: category, Status: tc.status, WitnessKind: tc.witness,
					ContractRefs: []string{"one", "two"}, Detail: "original observation detail",
				}
				report := &ChangeReport{Passed: true, VerificationConfidence: []VerificationConfidenceRecord{record}}
				before, _ := json.Marshal(report)
				pack := WriteContextPackFromChangeReport(report)
				for _, consumer := range []WriteContextConsumer{WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier} {
					view := pack.View(consumer, 20)
					if len(view.Items) != 1 || view.Items[0].Kind != "verification_confidence" {
						t.Fatalf("disclosure added/removed context rows: %+v", view.Items)
					}
					item := view.Items[0]
					if strings.Contains(item.Text, probeGranularityTestPhrase) != tc.want {
						t.Fatalf("granularity disclosure eligibility=%v: %s", tc.want, item.Text)
					}
					if item.ID != writeVerificationConfidenceContextID(record) {
						t.Fatalf("stable confidence identity changed: %+v", item)
					}
				}
				after, _ := json.Marshal(report)
				if !bytes.Equal(before, after) {
					t.Fatal("presentation rewrote the stored confidence report")
				}
			})
		}
	}
	for _, category := range []string{"project_test_contract_refs", "source_contract_refs", "probe_changed_symbol", "new_category"} {
		record := VerificationConfidenceRecord{Source: "verification_probe", Category: category, Status: "satisfied", Detail: "unchanged"}
		if got := renderVerificationConfidenceContext(record); strings.Contains(got, probeGranularityTestPhrase) {
			t.Fatalf("unrelated category was promoted to admitted probe witness: %s", got)
		}
	}
}

func TestVerificationProbeGranularityContextSurvivesExistingTextBudget(t *testing.T) {
	record := VerificationConfidenceRecord{
		Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
		ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{strings.Repeat("reference", 60)},
		Detail: strings.Repeat("original observation ", 60),
	}
	text := renderVerificationConfidenceContext(record)
	if !strings.Contains(text, probeGranularityTestPhrase) || len([]rune(text)) > writeContextPackTextLen+3 {
		t.Fatalf("execution boundary was lost or text budget expanded: %s", text)
	}
	if !strings.Contains(VerificationConfidenceDisplayDetail(record), strings.TrimSpace(record.Detail)) {
		t.Fatal("unbounded detail view discarded the original observation")
	}
}

func TestVerificationProbeGranularityLedgerChangesOnlyDetail(t *testing.T) {
	for _, category := range []string{"probe_contract_refs", "probe_soft_contract_refs", "probe_placement_refs"} {
		t.Run(category, func(t *testing.T) {
			record := VerificationConfidenceRecord{
				Source: "verification_probe", Category: category, Status: "satisfied", Severity: "info",
				ContractRefs: []string{"one", "two"}, ReasonCode: "original_reason", Detail: "original detail",
				WitnessKind: WriteBehaviorWitnessVerificationProbe,
			}
			report := &ChangeReport{PlanID: "plan-display", VerificationConfidence: []VerificationConfidenceRecord{record}}
			before, _ := json.Marshal(report)
			var ledger VerificationProofLedger
			ledger.addVerificationConfidenceLedgerItems(nil, report)
			if len(ledger.Obligations) != 2 || len(ledger.Disclosures) != 0 || len(ledger.Capabilities) != 0 {
				t.Fatalf("presentation changed the existing obligation domain: %+v", ledger)
			}
			for i, item := range ledger.Obligations {
				if !strings.Contains(item.Detail, probeGranularityTestPhrase) || !strings.Contains(item.Detail, record.Detail) {
					t.Errorf("ledger lost whole-probe execution boundary: %+v", item)
				}
				want := VerificationProofLedgerItem{
					Kind: verificationProofLedgerKindFromConfidence(category), Status: VerificationProofLedgerItemCovered,
					Source: record.Source, ReportPlanID: report.PlanID, Category: category, Severity: record.Severity,
					ReasonCode: record.ReasonCode, ContractRef: record.ContractRefs[i], Detail: record.Detail,
				}
				want.ID = verificationProofLedgerItemKey(want)
				item.Detail = record.Detail
				if !reflect.DeepEqual(item, want) {
					t.Fatalf("non-display proof data changed: got=%+v want=%+v", item, want)
				}
			}
			after, _ := json.Marshal(report)
			if !bytes.Equal(before, after) {
				t.Fatal("ledger projection changed the source report")
			}
		})
	}
}

func TestVerificationProbeGranularityPreservesExactRefDischarge(t *testing.T) {
	for _, cumulative := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			plan, report := verificationProjectReasonFixture(VerificationProjectTestAssertionNotObservedReasonCode)
			refs := []string{"value"}
			if complete {
				refs = append(refs, "boundary")
			}
			record := VerificationConfidenceRecord{
				Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
				ContractRefs: refs, WitnessKind: WriteBehaviorWitnessVerificationProbe, Detail: "original observation",
			}
			var artifacts []VerificationProofArtifact
			if cumulative {
				otherPlan, otherReport := verificationProjectReasonFixture(VerificationProjectTestAssertionNotObservedReasonCode)
				otherPlan.ID, otherReport.PlanID = "plan-followup", "plan-followup"
				otherReport.VerificationConfidence = []VerificationConfidenceRecord{record}
				artifacts = []VerificationProofArtifact{{Plan: otherPlan, Report: otherReport}}
			} else {
				report.VerificationConfidence = append(report.VerificationConfidence, record)
			}
			before, _ := json.Marshal([]any{plan, report, artifacts})
			profile := BuildCumulativeVerificationProofProfile(plan, report, artifacts)
			ledger := BuildVerificationProofLedger(plan, report, artifacts)
			if complete {
				if profile.Status != VerificationProofAdequate || ledger.UncoveredCount != 0 || ledger.State != VerificationProofLedgerVerified {
					t.Fatalf("display note weakened exact-ref discharge: %+v %+v", profile, ledger)
				}
			} else if profile.Status != VerificationProofWeak || ledger.UncoveredCount == 0 || !verificationProofHasReason(profile, VerificationProjectTestAssertionNotObservedReasonCode) {
				t.Fatalf("partial proof became sufficient: %+v %+v", profile, ledger)
			}
			found := 0
			for _, item := range ledger.Obligations {
				if item.Category == "probe_contract_refs" {
					found++
					if !strings.Contains(item.Detail, probeGranularityTestPhrase) {
						t.Errorf("cumulative=%v ledger dropped execution boundary: %+v", cumulative, item)
					}
				}
			}
			if found != len(refs) {
				t.Fatalf("ref obligations changed: found=%d want=%d", found, len(refs))
			}
			after, _ := json.Marshal([]any{plan, report, artifacts})
			if !bytes.Equal(before, after) {
				t.Fatal("cumulative presentation mutated plans or reports")
			}
		}
	}
}
