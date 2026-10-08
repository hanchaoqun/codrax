package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// HMC §208: independently approved correction of the existing preparation
// paragraph. Reverse only these exact bytes; historical whole-description
// pins must continue to reject unrelated edits rather than accepting new SHAs.
const tracePreparationApproved = "Normal analysis runs automatically prepare supported binary file paths and supported TraceStreamer SQLite databases before querying, reusing the complete prepared material within the run; no separate model conversion call is needed. SQLite is recognized by content, not filename extension. Self-contained databases and generation-stable main/WAL inputs use a validated private read-only snapshot without invoking a converter or changing the original database; SHM is not snapshot authority. Continuous-write online consistency, rollback journals, ambiguous sidecars and arbitrary databases are unsupported. Previews are bounded but queries use the complete admitted material. Originals stay unchanged and evidence line references address the readable query material. Unsupported, malformed or inventory-only captures fail without substituting a different source. Explicit trace stdin accepts supported text or binary only after complete EOF, bounded sealing and normal preparation; incomplete streams and inline binary are unsupported. Low-level parsers remain text/bundle only; codrax trace convert --input <binary-trace-path> remains the explicit binary conversion interface."

const tracePreparationPrior = "Normal analysis runs automatically prepare supported binary file paths and closed, self-contained TraceStreamer SQLite databases before querying, reusing the complete prepared material within the run; no separate model conversion call is needed. SQLite is recognized by content, not filename extension, read from a private snapshot without invoking a converter or changing the original database. WAL/header-WAL, SHM or journal state is currently refused; use a closed self-contained export. Previews are bounded but queries use the complete admitted material. Originals stay unchanged and evidence line references address the readable query material. Unsupported, malformed or inventory-only captures fail without substituting a different source. Binary stdin/inline is not supported. Low-level parsers remain text/bundle only; codrax trace convert --input <binary-trace-path> remains the explicit binary conversion interface."

func traceQueryDescriptionBeforePreparationEvolution(t *testing.T, description string) string {
	t.Helper()
	if traceQueryInputPreparationTeaching != tracePreparationApproved || strings.Count(description, tracePreparationApproved) != 1 || strings.Contains(description, tracePreparationPrior) {
		t.Fatal("preparation teaching must be the one approved existing-slot correction")
	}
	return strings.Replace(description, tracePreparationApproved, tracePreparationPrior, 1)
}

func TestTraceQueryDescriptionPreparationOnlyEvolution(t *testing.T) {
	prior := traceQueryDescriptionBeforePreparationEvolution(t, (&TraceQuery{}).Description())
	// Entire canonical Description after the §207 terminal-window evolution.
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != "045b4bd726d2450682b2fa9c592e43a3006ed677935b36d4a703d6981d1acd38" {
		t.Fatalf("Description changed outside the exact preparation correction: %s", got)
	}
}

func TestTraceQueryPreparationTeachingBothPublicSurfaces(t *testing.T) {
	var params struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &params); err != nil {
		t.Fatal(err)
	}
	for name, surface := range map[string]string{"description": (&TraceQuery{}).Description(), "path": params.Properties["path"].Description} {
		if strings.Count(surface, tracePreparationApproved) != 1 || strings.Contains(surface, tracePreparationPrior) {
			t.Fatalf("%s lost the shared current preparation contract", name)
		}
		for _, required := range []string{"generation-stable main/WAL", "SHM is not snapshot authority", "Continuous-write online consistency", "complete EOF", "incomplete streams and inline binary are unsupported", "fail without substituting a different source"} {
			if !strings.Contains(surface, required) {
				t.Errorf("%s lacks %q", name, required)
			}
		}
	}
}
