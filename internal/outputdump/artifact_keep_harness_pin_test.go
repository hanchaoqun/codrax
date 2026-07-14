package outputdump

// artifact_keep_harness_pin_test.go — PIN-1 B7 wiring pin (§29.65 回归口,
// ARTIFACT-KEEP 第三犯, 2026-07-13).
//
// PruneDir in this package implements the retention prune that (correctly)
// keeps <root>/.codrax/output bounded — but eval sweeps run codrax with CWD
// at the repo root, so a gold sweep floods that directory and the prune then
// destroys the operator's replay/witness artifacts (three incidents, ledger
// §29.60.2). The harness-side guard is eval_archive_output_artifacts in
// eval/runner_lib.sh (cp -pn into the append-only .codrax/output_archive/
// BEFORE any run), called from eval/run.sh. The behavior test lives in
// eval/runner_lib_test.sh; this pin keeps the WIRING visible to
// `go test ./...` (the shell tests are manually triggered) — removing the
// helper or its run.sh call site reddens here.

import (
	"os"
	"strings"
	"testing"
)

func TestEvalHarnessArchivesOutputArtifactsBeforeRuns(t *testing.T) {
	lib, err := os.ReadFile("../../eval/runner_lib.sh")
	if err != nil {
		t.Fatalf("read eval/runner_lib.sh: %v", err)
	}
	for _, want := range []string{
		"eval_archive_output_artifacts() {",
		// no-clobber copy (never move): the first archived witness wins and
		// the live dump stays where the operator saved it.
		`cp -pn "$f" "$archive/"`,
		".codrax/output_archive",
	} {
		if !strings.Contains(string(lib), want) {
			t.Fatalf("eval/runner_lib.sh lost the ARTIFACT-KEEP guard piece %q — witness dumps are prune-food again (§29.60.2)", want)
		}
	}
	run, err := os.ReadFile("../../eval/run.sh")
	if err != nil {
		t.Fatalf("read eval/run.sh: %v", err)
	}
	if !strings.Contains(string(run), `eval_archive_output_artifacts "$ROOT"`) {
		t.Fatalf("eval/run.sh lost the pre-run ARTIFACT-KEEP archive call — a sweep can again destroy the only witness copy")
	}
}
