package types

import "testing"

func TestPerfBundleHasAuthoritativeJankVerdictFailsClosed(t *testing.T) {
	for name, bundle := range map[string]*PerfBundle{
		"unspecified jank entry": {Janks: []PerfJank{{DurationMs: 48}}},
		"pretriage jank entry": {Janks: []PerfJank{{
			DurationMs:       48,
			VerdictAuthority: PerfObservationAuthorityPreTriageModelExtraction,
		}}},
		"unspecified frame bit": {Frames: []PerfFrame{{DurationMs: 48, Janky: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			if bundle.HasAuthoritativeJankVerdict() {
				t.Fatal("untyped or model-extracted classification must not mint a hard jank verdict")
			}
		})
	}
}

func TestPerfBundleHasAuthoritativeJankVerdictAcceptsDeterministicAuthority(t *testing.T) {
	bundle := &PerfBundle{Janks: []PerfJank{{
		DurationMs:       48,
		VerdictAuthority: PerfObservationAuthorityDeterministicValidator,
	}}}
	if !bundle.HasAuthoritativeJankVerdict() {
		t.Fatal("an explicitly deterministic jank verdict should satisfy the typed authority check")
	}
}
