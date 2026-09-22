package tracefinding

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRootCauseWaitBucketLabelsKeepRawSplitAndEliminabilityBoundary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		d, io float64
	}{
		{"D_IO", 0, 7},
		{"non_IO_D", 3, 0},
		{"mixed", 3, 7},
	} {
		for _, token := range []string{"d_state_or_io_wait", "fragmented_d_state_or_io_wait"} {
			t.Run(tc.name+"/"+token, func(t *testing.T) {
				decision := types.TraceCauseDecision{
					Token: types.TraceCausalTokenSnapshot{Token: token},
					Magnitude: &types.TypedMagnitude{Value: tc.d + tc.io, Unit: "ms", Caliber: types.TraceImpactCaliberEffectiveAttribution,
						Components: &types.TraceMagnitudeComponents{DStateMS: tc.d, IOWaitMS: tc.io}},
				}
				before, _ := json.Marshal(decision)
				for _, lang := range []string{"zh", "en"} {
					got := RootCauseValueDescriptionForLanguage(decision, lang)
					label, boundary := "non-IO D-state", "not a promise of directly eliminable time"
					if lang == "zh" {
						label, boundary = "非 IO D-state", "不是可直接消除的承诺"
					}
					for _, want := range []string{fmt.Sprintf("%s %.3f ms", label, tc.d), fmt.Sprintf("I/O wait %.3f ms", tc.io), boundary} {
						if lang == "zh" {
							want = strings.ReplaceAll(want, "I/O wait", "I/O 等待")
						}
						if !strings.Contains(got, want) {
							t.Errorf("%s composition uses a physical D-total caption for the raw bucket, or changed a boundary: missing %q in %s", lang, want, got)
						}
					}
				}
				after, _ := json.Marshal(decision)
				if string(before) != string(after) {
					t.Fatal("wording changed decision facts, amount, or category token")
				}
			})
		}
	}
}
