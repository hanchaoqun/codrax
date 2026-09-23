package tracewire

import (
	"regexp"
	"testing"
)

func TestHiSysEventPrintNameCompleteGrammar(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"AA", true}, {"A0", true}, {"A_", true}, {"POWER_09", true},
		{"", false}, {"A", false}, {"0A", false}, {"_A", false},
		{"aA", false}, {"Aa", false}, {"AA ", false}, {" AA", false},
		{"AA\t", false}, {"AA\n", false}, {"AA/BB", false}, {"AA:BB", false},
		{"AA-BB", false}, {"AA.BB", false}, {"ÅA", false}, {"A字", false},
		{"ＡＡ", false}, {"AA\x00", false}, {"AA\xff", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsHiSysEventPrintName(tc.name); got != tc.want {
				t.Fatalf("IsHiSysEventPrintName(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

func TestHiSysEventPrintHeadGrammarBoundaries(t *testing.T) {
	for _, tc := range []struct {
		fields        string
		domain, event string
		want          bool
	}{
		{"AA/BB:", "AA", "BB", true},
		{"AA/BB: ", "AA", "BB", true},
		{"AA/BB:  ", "AA", "BB", true},
		{"POWER_09/THERMAL_REPORT: {\"值\":1}", "POWER_09", "THERMAL_REPORT", true},
		{"AA/BB: /CC: next", "AA", "BB", true},
		{"AA/BB: \t\n\x00\xff", "AA", "BB", true},
		{"AA/BB:\t", "", "", false},
		{"AA/BB:\n", "", "", false},
		{"AA/BB:\r", "", "", false},
		{"AA/BB:\u00a0", "", "", false},
		{"AA/BB:\u3000", "", "", false},
		{"AA/BB:x", "", "", false},
		{"AA/BB:: ", "", "", false},
		{"AA//BB: ", "", "", false},
		{"AA/BB/CC: ", "", "", false},
		{"AA/BB", "", "", false},
		{"AA/BB : ", "", "", false},
		{" AA/BB: ", "", "", false},
		{"\nAA/BB: ", "", "", false},
		{"AA/字字: ", "", "", false},
		{"AA/B: ", "", "", false},
		{"A/BB: ", "", "", false},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			domain, event, ok := ParseHiSysEventPrintHead(tc.fields)
			if ok != tc.want || domain != tc.domain || event != tc.event {
				t.Fatalf("ParseHiSysEventPrintHead(%q) = (%q, %q, %t), want (%q, %q, %t)", tc.fields, domain, event, ok, tc.domain, tc.event, tc.want)
			}
		})
	}
}

func TestHiSysEventPrintHeadLegacyRegexParity(t *testing.T) {
	// This is the former parser's complete head grammar. Captures below only
	// recover its exact domain/event; neither regex supplies source authority.
	legacy := regexp.MustCompile(`^[A-Z][A-Z0-9_]+/[A-Z][A-Z0-9_]+:( |$)`)
	legacyParts := regexp.MustCompile(`^([A-Z][A-Z0-9_]+)/([A-Z][A-Z0-9_]+):( |$)`)
	names := []string{"", "A", "AA", "A0", "A_", "POWER_09", "_A", "1A", "aA", "Aa", " AA", "AA ", "AA\t", "AA\n", "AA/BB", "AA:BB", "ÅA", "A字", "ＡＡ", "AA\x00", "AA\xff"}
	tails := []string{"", " ", "  ", " payload", " \t", " \n", " :/字\x00\xff", "\t", "\n", "\r", "\u00a0", "\u3000", ": ", "x"}
	for _, prefix := range []string{"", " ", "\n", "<hisysevent> "} {
		for _, separator := range []string{"/", "//", ":"} {
			for _, domain := range names {
				for _, event := range names {
					for _, tail := range tails {
						fields := prefix + domain + separator + event + ":" + tail
						gotDomain, gotEvent, gotOK := ParseHiSysEventPrintHead(fields)
						wantOK := legacy.MatchString(fields)
						var wantDomain, wantEvent string
						if wantOK {
							parts := legacyParts.FindStringSubmatch(fields)
							wantDomain, wantEvent = parts[1], parts[2]
						}
						if gotOK != wantOK || gotDomain != wantDomain || gotEvent != wantEvent {
							t.Fatalf("legacy mismatch for %q: got (%q,%q,%t), want (%q,%q,%t)", fields, gotDomain, gotEvent, gotOK, wantDomain, wantEvent, wantOK)
						}
					}
				}
			}
		}
	}
}
