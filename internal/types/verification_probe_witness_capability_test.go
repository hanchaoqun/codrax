package types

import "testing"

func TestVerificationProbeWitnessCapabilityMatchesReceiptProducer(t *testing.T) {
	for _, tc := range []struct {
		language string
		want     bool
	}{
		{"", true}, {"python", true}, {"py", true}, {" PYTHON ", true},
		{"javascript", false}, {"ruby", false}, {"java", false}, {"go", false},
	} {
		t.Run(tc.language, func(t *testing.T) {
			got := VerificationProbeUsesExecutionOnlyWitness(tc.language)
			if got != tc.want {
				t.Fatalf("execution-only=%v want %v", got, tc.want)
			}
			resolution := ResolveVerificationProbeTargetExecution(nil, VerificationProbe{Language: tc.language}, nil)
			if resolution.Applies != got {
				t.Fatal("dispatch capability disagrees with the executor-owned receipt boundary")
			}
		})
	}
}
