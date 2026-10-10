package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLogObservationFactsSeparateNavigationAndAuthority(t *testing.T) {
	for _, errorCount := range []int{0, 1, 2} {
		for _, evidence := range []string{"", "operation=refreshCatalog reason=source_printed_relation"} {
			obs := LogObservation{Kind: LogObservationRuntimeEvent, Subject: "business navigation hint", Summary: "model interpretation", Evidence: evidence, LineStart: 4, Diagnostic: true, Confidence: 1}
			bundle := &LogBundle{Errors: make([]LogError, errorCount), Observations: []LogObservation{obs}}
			before, _ := json.Marshal(bundle)
			fact, ok := ProjectLogObservationForFacts(bundle, obs)
			if ok != (evidence != "") || fact.Subject != "" || fact.Summary != evidence || fact.LineStart != 4 || fact.Kind != obs.Kind {
				t.Fatalf("errors=%d evidence=%q: inappropriate fact projection: %+v / %t", errorCount, evidence, fact, ok)
			}
			if errorCount < 2 {
				navigation, kept := ProjectLogObservationForReasoning(bundle, obs)
				if !kept || navigation.Subject != obs.Subject || navigation.Summary != obs.Summary {
					t.Fatalf("exploration lost its advisory business clues: %+v", navigation)
				}
			}
			if evidence == "" {
				for _, row := range CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle}).Records {
					if strings.HasPrefix(row.ID, "log:observation:") {
						t.Fatalf("summary-only observation became an answer fact: %+v", row)
					}
				}
				if bindings := logBundleClaimBindings(bundle, nil); len(bindings) != 0 {
					t.Fatalf("summary-only observation became a claim: %+v", bindings)
				}
				for _, seed := range CollectExternalObservationSeeds(bundle, nil) {
					if seed.Kind == "log_observation" {
						t.Fatalf("summary-only observation became a fact seed: %+v", seed)
					}
				}
			}
			after, _ := json.Marshal(bundle)
			if string(before) != string(after) {
				t.Fatal("projection mutated the original audit carrier")
			}
		}
	}
}
