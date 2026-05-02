package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestUpsertHedgePrefix_PreservesLegitimateParenthetical: round-7
// load-bearing fix. Pre-round-7, ceiling-upgrade on text that began
// with a legitimate parenthetical (e.g. "(in older Go) X happens")
// would have stripped the user's parenthetical as if it were a
// stale system reason. Now stripping is system-match only — user
// parentheticals are preserved.
func TestUpsertHedgePrefix_PreservesLegitimateParenthetical(t *testing.T) {
	// Stale state: text already prefixed with the WEAKER ceiling, plus
	// a USER-WRITTEN parenthetical right after the marker.
	stale := hedgeMarkerConditional + " (in older Go versions) the channel was unbuffered"
	out := upsertHedgePrefix(stale, hedgeMarkerHistorical,
		hedgeMarkerHistorical+" (historical observation)")
	if !strings.HasPrefix(out, hedgeMarkerHistorical) {
		t.Errorf("upgrade did not happen: %q", out)
	}
	if !strings.Contains(out, "(in older Go versions)") {
		t.Errorf("legitimate parenthetical stripped: %q", out)
	}
	if !strings.Contains(out, "the channel was unbuffered") {
		t.Errorf("user content lost: %q", out)
	}
}

// TestUpsertHedgePrefix_StripsSystemTerseReasonOnUpgrade: the dual
// case — stale prefix WITH a real system terse reason. The system
// reason MUST be stripped on upgrade so the new marker's reason
// doesn't double up.
func TestUpsertHedgePrefix_StripsSystemTerseReasonOnUpgrade(t *testing.T) {
	stale := hedgeMarkerConditional + " (log-derived; code drifted) X is at line 10"
	out := upsertHedgePrefix(stale, hedgeMarkerHistorical,
		hedgeMarkerHistorical+" (historical observation)")
	if !strings.HasPrefix(out, hedgeMarkerHistorical+" (historical observation)") {
		t.Errorf("upgraded prefix not at start: %q", out)
	}
	// Stale system reason must NOT survive.
	if strings.Contains(out, "log-derived; code drifted") {
		t.Errorf("stale system reason retained: %q", out)
	}
}

// TestStripLeadingMarkerAndReason_PreservesLegitimateParenthetical:
// same guarantee for the strip helper used by StripAuthorityArtifacts.
func TestStripLeadingMarkerAndReason_PreservesLegitimateParenthetical(t *testing.T) {
	line := hedgeMarkerHistorical + " (legacy code path) X happens"
	out := stripLeadingMarkerAndReason(line)
	if strings.Contains(out, hedgeMarkerHistorical) {
		t.Errorf("marker not stripped: %q", out)
	}
	if !strings.Contains(out, "(legacy code path)") {
		t.Errorf("legitimate parenthetical stripped: %q", out)
	}
}

// TestStripInlineMarkerWithReason_PreservesLegitimateParenthetical:
// mid-line variant of the same guarantee. User writes "X [hedged]
// (legacy code) happens" — only the marker should be stripped.
func TestStripInlineMarkerWithReason_PreservesLegitimateParenthetical(t *testing.T) {
	line := "the handler X " + hedgeMarkerConditional + " (legacy lookup path) returns Y"
	out := stripInlineMarkerWithReason(line, hedgeMarkerConditional)
	if strings.Contains(out, hedgeMarkerConditional) {
		t.Errorf("marker not stripped: %q", out)
	}
	if !strings.Contains(out, "(legacy lookup path)") {
		t.Errorf("legitimate parenthetical stripped: %q", out)
	}
}

// TestKnownSystemTerseReasons_CoversEveryCeilingLanguageCombo:
// drift between shortHedgeReasonFor's output and the strip helper's
// match list would silently leak. Pin the contract: every reason
// that can be produced by shortHedgeReasonFor must be in the strip
// list, and vice versa.
func TestKnownSystemTerseReasons_CoversEveryCeilingLanguageCombo(t *testing.T) {
	// Build the set of reasons shortHedgeReasonFor produces across
	// every ceiling × language combo.
	produced := make(map[string]bool)
	for _, c := range []types.AuthorityCeiling{
		types.AuthorityConditional,
		types.AuthorityHistorical,
		types.AuthorityIllustrative,
	} {
		for _, l := range []answerDocLang{answerDocLangEN, answerDocLangZH} {
			r := shortHedgeReasonFor(c, l)
			if r == "" {
				t.Errorf("shortHedgeReasonFor(%q, %v) empty", c, l)
				continue
			}
			produced[r] = true
		}
	}
	// Every produced reason must be in the strip list.
	stripList := make(map[string]bool)
	for _, r := range knownSystemTerseReasons() {
		stripList[r] = true
	}
	for r := range produced {
		if !stripList[r] {
			t.Errorf("produced reason %q is NOT in knownSystemTerseReasons", r)
		}
	}
	// Every strip-list entry must be producible (no dead entries).
	for r := range stripList {
		if !produced[r] {
			t.Errorf("strip-list entry %q is NOT produced by any ceiling/lang combo (dead)", r)
		}
	}
}
