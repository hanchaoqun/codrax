package types

import "testing"

func TestAnswerCoverageCaveatSourceNeutralTemplate(t *testing.T) {
	tmpl, ok := CaveatFamilyTemplateFor(CaveatFamilyAnswerCoverage)
	if !ok {
		t.Fatal("answer coverage must retain its registered disclosure")
	}
	for _, tc := range []struct {
		lang, got, want string
	}{
		{"zh", tmpl.ZH, "答案在某些维度的覆盖度可能不充分，建议结合相关证据进一步核对。"},
		{"en", tmpl.EN, "Coverage on some dimensions of the answer may be incomplete; cross-check the relevant evidence."},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("coverage disclosure must not choose a source or component for the user:\ngot: %s\nwant: %s", tc.got, tc.want)
			}
		})
	}
}
