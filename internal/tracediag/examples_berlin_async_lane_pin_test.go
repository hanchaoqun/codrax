package tracediag

import (
	"path/filepath"
	"regexp"
	"testing"
)

// SEC #30 pin: the berlin pairing witness's async lanes must stay ANCHORED
// (`S|<pid>|` / `F|<pid>|`), never the bare substrings "S|"/"F|". The engine
// pattern is case-insensitive substring over payload fields with no prefix
// anchoring, so a bare "S|" matches any trace_mark payload containing
// "…s|…" (counters like C|1252|H:VSync-rs|0, B| spans with names ending in
// s) and crowds the true async rows out of the 100-line cap — the witness
// then round-trips useless. The pid-segment form is the only precise lane
// available under substring semantics: async payloads are S|pid|name|cookie,
// and "s|<pid>|" occurs nowhere else in a payload.
func TestBerlinWitnessAsyncLanesAreAnchored(t *testing.T) {
	script, err := LoadScript(filepath.Join("..", "..", "examples", "tracediag", "collect_berlin_pairing_witness.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	anchored := regexp.MustCompile(`^[SF]\|\d+\|$`)
	found := map[string]bool{}
	for _, step := range script.Steps {
		switch step.Label {
		case "raw_async_starts", "raw_async_finishes":
			found[step.Label] = true
			if step.Pattern == "S|" || step.Pattern == "F|" {
				t.Errorf("%s regressed to the bare substring pattern %q (cap crowd-out, SEC #30)", step.Label, step.Pattern)
			}
			if !anchored.MatchString(step.Pattern) {
				t.Errorf("%s pattern %q is not the anchored async lane form S|<pid>| / F|<pid>|", step.Label, step.Pattern)
			}
		}
	}
	for _, label := range []string{"raw_async_starts", "raw_async_finishes"} {
		if !found[label] {
			t.Errorf("berlin witness lost its %s step", label)
		}
	}
}
