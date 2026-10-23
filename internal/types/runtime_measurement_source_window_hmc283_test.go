package types

import "testing"

func TestHMC283EventSelectionDisclosureKeepsAppliedScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		scan *TraceEventSearchScanScope
		want string
	}{
		{"legacy", nil, "selection unavailable (legacy producer)"},
		{"zero_not_applied", &TraceEventSearchScanScope{}, "no time or line filter"},
		{"zero_applied", &TraceEventSearchScanScope{TimeStartApplied: true}, "time >= 0 s"},
		{"line_lower", &TraceEventSearchScanScope{LineStart: 7}, "lines >= 7 (time arguments not applied)"},
		{"line_upper", &TraceEventSearchScanScope{LineEnd: 9}, "lines <= 9 (time arguments not applied)"},
		{"indexed", &TraceEventSearchScanScope{TimeStart: 2, TimeEnd: 3, TimeStartApplied: true, TimeEndApplied: true, IndexRestricted: true}, "time [2,3] s (both endpoints included); restricted to indexed candidates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeEventQuerySelection(tc.scan); got != tc.want {
				t.Fatalf("actual scan selection changed: got %q want %q", got, tc.want)
			}
		})
	}
}
