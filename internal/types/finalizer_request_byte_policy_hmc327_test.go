package types

import (
	"reflect"
	"testing"
)

func TestHMC327FinalizerRequestBytePolicyDefaultAndResolution(t *testing.T) {
	if got := DefaultAgentSettings().FinalizerRequestBytePolicy; got != FinalizerRequestByteEnforce {
		t.Fatalf("normal default must enforce the request-byte contract: %q", got)
	}
	for _, tc := range []struct {
		name string
		in   FinalizerRequestBytePolicy
		want FinalizerRequestBytePolicy
	}{
		{"zero", "", FinalizerRequestByteEnforce},
		{"enforce", FinalizerRequestByteEnforce, FinalizerRequestByteEnforce},
		{"explicit_legacy", FinalizerRequestByteLegacyAdvisory, FinalizerRequestByteLegacyAdvisory},
		{"invalid", "unknown", FinalizerRequestByteEnforce},
		{"case_is_not_enum", "ENFORCE", FinalizerRequestByteEnforce},
		{"whitespace_is_not_enum", " legacy_advisory ", FinalizerRequestByteEnforce},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := AgentSettings{FinalizerRequestBytePolicy: tc.in, MaxIterations: 31}
			got := ResolvedAgentSettings(input)
			if got.FinalizerRequestBytePolicy != tc.want || got.MaxIterations != 31 {
				t.Fatalf("resolution lost the explicit policy or unrelated setting: %+v", got)
			}
			if input.FinalizerRequestBytePolicy != tc.in || input.MaxToolHistoryBytes != 0 {
				t.Fatal("resolution mutated the caller's raw settings")
			}
			if again := ResolvedAgentSettings(got); !reflect.DeepEqual(got, again) {
				t.Fatal("resolved policy is not stable across composition layers")
			}
		})
	}
}

func TestHMC327UnresolvedFinalizerRequestBytePolicyRemainsExplicitlyAbsent(t *testing.T) {
	var raw AgentSettings
	_ = DefaultAgentSettings()
	_ = ResolvedAgentSettings(raw)
	if raw.FinalizerRequestBytePolicy != "" {
		t.Fatal("default construction silently opted a raw legacy composition into enforcement")
	}
	field, ok := reflect.TypeOf(raw).FieldByName("FinalizerRequestBytePolicy")
	if !ok || field.Tag.Get("yaml") != "-" {
		t.Fatal("this in-memory policy must not introduce a YAML configuration surface")
	}
}
