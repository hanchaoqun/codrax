package tracequery

import "testing"

func TestProcessMeasureProtocolNavigationUsesFullSelectedInventory(t *testing.T) {
	unknown := processMeasureTestRecord(1, 1000000000, 1000000000, 30, 100)
	known := processMeasureTestRecord(2, 1000000000, 1000000000, 60, 100)
	known.Name = "H:PreferredFrameRate"
	idx := renderingFixture(t, processMeasureTestText(t, unknown, known))
	p := processMeasureTestRun(t, idx, Query{TimeStart: 1, TimeStartSet: true, TimeEnd: 2, Limit: 1})
	if len(p.Rows) != 1 || p.Rows[0].Record.Name != unknown.Name || len(p.AvailableDerivedViews) != 1 || p.AvailableDerivedViews[0] != ViewPreferredFrameRate {
		t.Fatalf("navigation was derived from bounded display, not complete scan: %+v", p)
	}
	for _, name := range []string{"PreferredFrameRate", "H:PreferredFrameRateExtra", "H:preferredframerate", "H:PreferredFrameRate "} {
		known.Name = name
		if got := ProcessMeasureProtocolView(known); got != "" {
			t.Fatalf("similar name got native protocol: %q => %s", name, got)
		}
	}
	known.Name, known.NameKnown = "H:PreferredFrameRate", false
	if got := ProcessMeasureProtocolView(known); got != "" {
		t.Fatal("unknown name got protocol")
	}
}
