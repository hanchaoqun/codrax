package tool

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func traceQueryDescriptionWithoutEventNameSuffix(t *testing.T) string {
	t.Helper()
	description := (&TraceQuery{}).Description()
	suffix := " " + traceQueryEventNameTeaching
	if strings.Count(description, traceQueryEventNameTeaching) != 1 || !strings.HasSuffix(description, suffix) {
		t.Fatal("event-name teaching must occur exactly once at the Description tail")
	}
	return strings.TrimSuffix(description, suffix)
}

func TestTraceQueryDescriptionSQLiteAndEventNameOnlyEvolution(t *testing.T) {
	// Approved SQLite correction, deliberately literal rather than deriving
	// the accepted delta from whichever production text happens to be current.
	const approvedPreparation = "Normal analysis runs automatically prepare supported binary file paths and closed, self-contained TraceStreamer SQLite databases before querying, reusing the complete prepared material within the run; no separate model conversion call is needed. SQLite is recognized by content, not filename extension, read from a private snapshot without invoking a converter or changing the original database. WAL/header-WAL, SHM or journal state is currently refused; use a closed self-contained export. Previews are bounded but queries use the complete admitted material. Originals stay unchanged and evidence line references address the readable query material. Unsupported, malformed or inventory-only captures fail without substituting a different source. Binary stdin/inline is not supported. Low-level parsers remain text/bundle only; codrax trace convert --input <binary-trace-path> remains the explicit binary conversion interface."
	const priorPreparation = "Normal analysis runs automatically prepare supported binary file paths before querying, reusing the complete prepared material within the run; no separate model conversion call is needed. Previews are bounded but queries use the complete admitted material. Originals stay unchanged and evidence line references address the readable query material. Unsupported, malformed or inventory-only captures fail without substituting a different source. Existing SQLite databases still require explicit text export; binary stdin/inline is not supported. Low-level parsers remain text/bundle only; codrax trace convert --input <binary-trace-path> remains the explicit conversion interface."
	description := traceQueryDescriptionWithoutEventNameSuffix(t)
	// Keep this historical SHA intact by reversing only the independently
	// pinned, later state-accounting guidance replacement.
	description = traceQueryDescriptionBeforeStateAccountingEvolution(t, description)
	if traceQueryInputPreparationTeaching != approvedPreparation || strings.Count(description, approvedPreparation) != 1 {
		t.Fatal("input preparation teaching differs from the one approved SQLite paragraph correction")
	}
	prior := strings.Replace(description, approvedPreparation, priorPreparation, 1)
	// SHA of the complete golden immediately before 501f02f97 / 7834230a6.
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != "afad00c25048ed09f35556a4e03a93035b31c665fab8eb2fae25363c02d2f927" {
		t.Fatalf("Description changed outside the approved SQLite paragraph and terminal event-name contract: %s", got)
	}
}
