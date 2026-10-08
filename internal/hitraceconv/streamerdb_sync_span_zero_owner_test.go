package hitraceconv

import (
	"fmt"
	"strings"
	"testing"
)

// Canonical IPID zero with a proven positive public PID is already supported
// by the SQLite compatibility input contract. It must not disappear from the
// shared collision index or turn an optional derived projection into lane loss.
func TestTraceDBStaticInitializeZeroOwnerKeepsOriginalAndHealthyBusiness(t *testing.T) {
	for _, sourceName := range []string{"dlopen libzero.so", "conflicting side-table name"} {
		t.Run(sourceName, func(t *testing.T) {
			rows := []string{
				"UPDATE process SET ipid=0 WHERE ipid=1",
				"UPDATE thread SET ipid=0 WHERE ipid=1",
				"INSERT INTO callstack VALUES (1,1000000,1000000,1,NULL,'dlopen libzero.so','',NULL,NULL,0)",
				"INSERT INTO callstack VALUES (2,3000000,1000000,1,NULL,'healthy-original','',NULL,NULL,0)",
			}
			baseline, before := exportTraceDBSyncSpanIntegrationFixture(t, "zero-owner-baseline", rows...)
			if requireTraceDBCoverage(t, before.Coverage, "slice", "callstack").RowsEmitted != 4 ||
				!strings.Contains(baseline, "healthy-original") {
				t.Fatal("zero-owner compatibility baseline was not admitted")
			}
			rows = append(rows, fmt.Sprintf("INSERT INTO static_initalize VALUES (1000000,2000000,'%s',0,100)", sourceName))
			body, after := exportTraceDBSyncSpanIntegrationFixture(t, "zero-owner-derived", rows...)
			original := requireTraceDBCoverage(t, after.Coverage, "slice", "callstack")
			derived := requireTraceDBCoverage(t, after.Coverage, "slice", "static_initalize")
			reason := "duplicate_callstack_projection=1"
			if sourceName != "dlopen libzero.so" {
				reason = "conflicting_derived_projection=1"
			}
			if original.RowsEmitted != 4 || derived.RowsRead != 1 || derived.RowsEmitted != 0 ||
				!strings.Contains(derived.Skipped, reason) || !strings.Contains(body, "healthy-original") ||
				!strings.Contains(body, "B|100|dlopen libzero.so") {
				t.Fatalf("zero-owner derived row lost original evidence: original=%+v derived=%+v\n%s", original, derived, body)
			}
		})
	}
}

func TestTraceDBSyncSpanSemanticKeyZeroOwnerDoesNotWeakenIdentity(t *testing.T) {
	for _, producer := range []traceDBSyncSpanProducer{traceDBSyncSpanProducerCallstack,
		traceDBSyncSpanProducerStaticInitialize, traceDBSyncSpanProducerSourceRawMarker} {
		valid := traceDBTestSyncSpanCandidate(producer, 1, 101, 100, 1000, 2000, "dlopen lib.so")
		valid.OwnerIPID = 0
		if err := validateTraceDBSyncSpanCandidate(valid); err != nil {
			t.Fatal(err)
		}
		key, ok := traceDBSyncSpanCandidateSemanticKey(valid)
		if !ok || key.OwnerIPID != 0 || key.HeaderTGID != 100 || key.CanonicalITID != 101 {
			t.Fatalf("known zero owner lost exact key: %+v %t", key, ok)
		}
		for name, mutate := range map[string]func(*traceDBSyncSpanCandidate){
			"unknown owner":             func(c *traceDBSyncSpanCandidate) { c.OwnerIPIDKnown = false },
			"negative owner":            func(c *traceDBSyncSpanCandidate) { c.OwnerIPID = -1 },
			"unknown canonical thread":  func(c *traceDBSyncSpanCandidate) { c.CanonicalITIDKnown = false },
			"idle canonical thread":     func(c *traceDBSyncSpanCandidate) { c.CanonicalITID = 0 },
			"negative canonical thread": func(c *traceDBSyncSpanCandidate) { c.CanonicalITID = -1 },
			"missing public thread":     func(c *traceDBSyncSpanCandidate) { c.HeaderTID = 0 },
			"missing public process":    func(c *traceDBSyncSpanCandidate) { c.HeaderTGID = 0 },
			"negative public process":   func(c *traceDBSyncSpanCandidate) { c.HeaderTGID = -1 },
			"unknown marker owner":      func(c *traceDBSyncSpanCandidate) { c.MarkerPIDKnown = true; c.MarkerPID = 0 },
			"reverse interval":          func(c *traceDBSyncSpanCandidate) { c.End = c.Start - 1 },
		} {
			bad := valid
			mutate(&bad)
			if _, ok := traceDBSyncSpanCandidateSemanticKey(bad); ok {
				t.Fatalf("producer=%d invalid %s gained collision identity", producer, name)
			}
		}
	}
	// A valid collision key does not authorize source-raw publication. The
	// raw marker payload must still agree with its independently proven owner.
	raw := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerSourceRawMarker, 1, 101, 100, 1000, 2000, "dlopen lib.so")
	raw.OwnerIPID = 0
	raw.StartMarkerBody = "B|200|dlopen lib.so"
	if validateTraceDBSyncSpanCandidate(raw) == nil {
		t.Fatal("zero owner bypassed raw marker owner validation")
	}
}
