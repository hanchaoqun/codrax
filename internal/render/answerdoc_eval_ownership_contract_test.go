package render

import (
	"os"
	"strings"
	"testing"
)

// The previous boundary-title registry was internally consistent but could
// not cover interleaved/prepended system blocks. Its replacement consumes the
// ownership receipt. Rendering/appendix behavior is exercised in surfaces_test.
func TestEvalPrimaryUsesFinalRenderOwnershipInsteadOfHeadingGuesses(t *testing.T) {
	raw, err := os.ReadFile("../../eval/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	runner := string(raw)
	if !strings.Contains(runner, `eval_load_answer_surfaces "$log" "$OUTDIR/run-$i"`) ||
		!strings.Contains(runner, `elif eval_requires_answer_surfaces; then`) {
		t.Fatal("scoped eval must require a bound render receipt")
	}
	for _, obsolete := range []string{"scope_primary_stdout()", "scope_principal_stdout()"} {
		if strings.Contains(runner, obsolete) {
			t.Fatalf("title-scanned fallback returned: %s", obsolete)
		}
	}
}
