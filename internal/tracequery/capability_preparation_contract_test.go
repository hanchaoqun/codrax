package tracequery

import (
	"strings"
	"testing"
)

func TestTraceCapabilityCatalogNativeMeasureContractMatchesDeliveredIntervals(t *testing.T) {
	catalog, err := TraceCapabilities(ViewCPUStateFrequency, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, format := range catalog.InputFormats {
		if format.ID != "sqlite" {
			continue
		}
		found = true
		for _, required := range []string{"preserved explicit measure intervals", "unverified meaning", "missing durations or coverage remain unknown", "Continuous-write online consistency is unsupported"} {
			if !strings.Contains(format.Limitation, required) {
				t.Errorf("SQL metadata lost %q: %+v", required, format)
			}
		}
		if strings.Contains(format.Limitation, "not yet supplied") || strings.Contains(format.Limitation, "unsupported_direct_input") {
			t.Fatal("metadata denied the delivered native interval path")
		}
	}
	if !found {
		t.Fatal("CPU interval catalog lost SQL input contract")
	}
}
