package tool

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// WF-range 区间符 pin (用户裁定 2026-07-12, SMR-1 批同批): value-range word
// faces use the ASCII "~" (数~数 typed range token, CR-3 prose 门同义) — the
// en-dash "–" misreads as a minus in arithmetic-dense reports. Scope: ×N
// family forms + every ms value range + the legend (a~b) teaching forms.
// Time windows (s-suffixed, and the taught ".." token) and true minus signs
// stay untouched. ×N→N次 family rewording is NOT this batch (× itself 勿动).

// Source-level zero-residue grep: no display-authority emission may format an
// ms value range with the en-dash.
func TestSMR1WFRangeNoEnDashValueRangeEmission(t *testing.T) {
	valueRange := regexp.MustCompile(`%\.[0-9]f–%\.[0-9]fms|a–b`)
	for _, name := range infoContractDisplayAuthorityFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := uxg1StripComments(string(raw))
		if m := valueRange.FindString(src); m != "" {
			t.Errorf("%s: value-range en-dash residue %q (用户裁定: 值区间用 ~)", name, m)
		}
	}
}

// Render-level pin: the ×N family form and the per-instance range speak "~".
func TestSMR1WFRangeMergedFormsUseTilde(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1D1S2Projection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "3次(8.879~80.751ms)") {
		t.Fatalf("×N form must use the ~ range glyph:\n%s", fence)
	}
	// Zero residue in value-range contexts: digits–digits…ms never renders.
	if m := regexp.MustCompile(`[0-9]–[0-9.]+ms`).FindString(fence); m != "" {
		t.Fatalf("value-range en-dash residue %q:\n%s", m, fence)
	}
}
