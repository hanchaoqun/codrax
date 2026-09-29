package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStartupSummaryAuthorityAcrossFactConsumers(t *testing.T) {
	for _, authority := range []PerfObservationAuthority{"", PerfObservationAuthorityPreTriageModelExtraction, "unknown_future_authority", PerfObservationAuthorityDeterministicValidator} {
		t.Run(string(authority), func(t *testing.T) {
			bundle := &PerfBundle{Meta: PerfMeta{Source: "hitrace", Summary: "unverified unrelated synopsis"}, Startup: &PerfStartup{Authority: authority, Mode: "cold", AppLaunchMs: 4321, AbilityInitMs: 876, FirstFrameMs: 543}}
			raw, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			var restored PerfBundle
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			verified := authority == PerfObservationAuthorityDeterministicValidator
			if restored.HasAuthoritativeStartup() != verified || !restored.HasStructuredObservations() || restored.Startup.AppLaunchMs != 4321 {
				t.Fatal("persistence lost audit data or upgraded authority")
			}
			var records []ObservationRecord
			compilePerfBundleObservations(&restored, func(r ObservationRecord) { records = append(records, r) })
			for _, count := range []int{len(restored.LogFrames()), len(perfBundleClaimBindings(&restored, nil)), len(collectPerfExternalObservationSeeds(&restored, nil)), len(records)} {
				if (count > 0) != verified {
					t.Fatalf("authority %q minted/lost a fact surface: count=%d", authority, count)
				}
			}
			if (LogPerfSubKindOf(EvidenceItem{Source: "startup.go"}, nil, &restored) == PerfStartupFrame) != verified || (rootCauseRequiredSubKind(RequestModel{PerfTrace: &restored}) == PerfStartupFrame) != verified {
				t.Fatal("startup obligation ignored typed authority")
			}
			for _, r := range records {
				if strings.Contains(r.Summary, "unverified unrelated") {
					t.Fatal("model synopsis promoted through verified scalar")
				}
			}
		})
	}
}
