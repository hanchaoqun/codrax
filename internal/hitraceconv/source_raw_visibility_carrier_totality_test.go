package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Behavioural pins for colleague_merge_audit §40.38 fold-in (F7/F9/F10/F8).
// They are written against the census/runtime entry points only, so each one
// was run RED on the untouched tree (registry without the five comment
// families, `# `-literals skipped by R2, census grammar `[a-z_]` vs runtime
// `[a-z0-9_]`, an 80-file floor instead of positive witnesses, and a bare
// event_invalid reason without row identification) before turning green.

// traceDBCarrierCensusCopy returns a mutable copy of the scanned sources.
func traceDBCarrierCensusCopy(sources map[string][]byte) map[string][]byte {
	mutated := make(map[string][]byte, len(sources))
	for name, src := range sources {
		mutated[name] = src
	}
	return mutated
}

// TestTraceDBCarrierCensusTotality (F7/F10): the census is total over BOTH
// carrier kinds in BOTH packages, and it proves its own coverage positively
// instead of by a file-count floor.
func TestTraceDBCarrierCensusTotality(t *testing.T) {
	sources := traceDBCarrierCensusSources(t)
	if violations := traceDBCarrierFamilyCensus(sources, traceDBReservedCarrierFamilies); len(violations) != 0 {
		t.Fatalf("live census is not clean:\n%s", strings.Join(violations, "\n"))
	}

	t.Run("live_comment_families_are_registered", func(t *testing.T) {
		// F7: the five comment wires the live tree emits but the registry
		// did not know about. The registry is the single source, so this is
		// the exact list — not a floor.
		registered := map[string]bool{}
		for _, family := range traceDBReservedCarrierFamilies {
			registered[family.Wire] = true
		}
		for _, wire := range []string{
			"codrax_trace_db_record/v1", "codrax_trace_db_block/v2",
			"codrax_frame_callstack/v1", "codrax_frame_gpu/v1", "codrax_perf_napi_async/v1",
		} {
			if !registered[wire] {
				t.Errorf("live comment wire %s is emitted by the tree but not registered", wire)
			}
		}
	})
	t.Run("unregistered_comment_literal_is_reported", func(t *testing.T) {
		// F7 (c): a brand-new `# codrax_<family>/v<N>` literal must be red in
		// either package — the `# ` prefix is no exemption.
		for _, key := range []string{"zz_future_comment.go", "tracequery/zz_future_comment.go"} {
			pkg := "hitraceconv"
			if strings.HasPrefix(key, "tracequery/") {
				pkg = "tracequery"
			}
			mutated := traceDBCarrierCensusCopy(sources)
			mutated[key] = []byte("package " + pkg + "\n\nconst futurePrefix = \"# codrax_totally_new_family/v1\"\n")
			violations := traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies)
			if !strings.Contains(strings.Join(violations, "\n"), "R2 "+key) {
				t.Errorf("census did not report an unregistered comment carrier literal in %s: %v", key, violations)
			}
		}
	})
	t.Run("registered_wire_declaration_is_a_positive_witness", func(t *testing.T) {
		// F10: deleting the parser file that declares the ftrace-body wire
		// (and every other declaring file) from the scanned set must be red,
		// not silently empty.
		for _, key := range []string{"tracequery/source_raw_visibility.go", "tracequery/official_sql_relations.go", "tracequery/trace_db_text_record.go"} {
			mutated := traceDBCarrierCensusCopy(sources)
			delete(mutated, key)
			if violations := traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies); len(violations) == 0 {
				t.Errorf("census stayed clean with %s removed from the scanned set", key)
			}
		}
	})
	t.Run("dropping_the_parser_package_turns_the_census_red", func(t *testing.T) {
		mutated := traceDBCarrierCensusCopy(sources)
		for key := range mutated {
			if strings.HasPrefix(key, "tracequery/") {
				delete(mutated, key)
			}
		}
		if len(mutated) == len(sources) {
			t.Fatalf("scanned set contains no parser-package files")
		}
		if violations := traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies); len(violations) == 0 {
			t.Fatalf("census stayed clean without the parser package in the scanned set")
		}
	})
	t.Run("emitter_must_reference_the_declared_wire", func(t *testing.T) {
		// A converter-side emitter that re-types the wire as its own literal
		// (the pre-fold-in streamerdb_text_fidelity.go shape) is red twice:
		// the emitter no longer references the declaration, and a second
		// literal of a registered wire outside its WireFile is a duplicate.
		const emitter = "streamerdb_text_fidelity.go"
		before := string(sources[emitter])
		after := strings.ReplaceAll(before, "tracequery.TraceDBTextBlockPrefix+\" block=", "\"# codrax_trace_db_block/v2 block=")
		if after == before {
			t.Fatalf("emitter shape drifted; self-red mutation did not apply")
		}
		mutated := traceDBCarrierCensusCopy(sources)
		mutated[emitter] = []byte(after)
		violations := strings.Join(traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies), "\n")
		if !strings.Contains(violations, "R1 comment family codrax_trace_db_block/v2") || !strings.Contains(violations, "R2 "+emitter) {
			t.Fatalf("census accepted an emitter that duplicates the wire literal instead of referencing it: %s", violations)
		}
	})
}

// TestTraceDBCarrierWireGrammarCensusMatchesRuntime (F9): for every candidate
// token the structural census sees exactly what the runtime body gate
// refuses — digit-bearing families included — because both derive from the
// one tracequery grammar.
func TestTraceDBCarrierWireGrammarCensusMatchesRuntime(t *testing.T) {
	sources := traceDBCarrierCensusSources(t)
	for _, candidate := range []struct {
		wire      string
		wantToken bool
	}{
		{wire: "codrax_family2/v1", wantToken: true},
		{wire: "codrax_zz_future_family/v1", wantToken: true},
		{wire: "codrax_f_9_x/v12", wantToken: true},
		{wire: "codrax_family/v1x", wantToken: false},
		{wire: "codrax_Family/v1", wantToken: false},
		{wire: "codrax_family/vx", wantToken: false},
		{wire: "codrax_/v1", wantToken: false},
		{wire: "codrax-family/v1", wantToken: false},
	} {
		t.Run(candidate.wire, func(t *testing.T) {
			row := fmt.Sprintf("worker-1 (1) [000] .... 2.000000: hmfs_writepage: %s semantic_authority=none", candidate.wire)
			runtimeRefuses := traceDBRowBodyCarriesCarrierSignature(row, "hmfs_writepage")
			mutated := traceDBCarrierCensusCopy(sources)
			mutated["zz_probe.go"] = []byte("package hitraceconv\n\nconst probeWire = \"" + candidate.wire + "\"\nconst probeComment = \"# " + candidate.wire + " x=1\"\n")
			violations := strings.Join(traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies), "\n")
			censusSees := strings.Contains(violations, "R2 zz_probe.go")
			if runtimeRefuses != candidate.wantToken || censusSees != candidate.wantToken {
				t.Fatalf("grammar disagreement for %q: runtime_refuses=%t census_sees=%t want=%t (%s)",
					candidate.wire, runtimeRefuses, censusSees, candidate.wantToken, violations)
			}
			// Registration follows the same grammar: a token the runtime
			// refuses can be registered, a non-token cannot.
			families := []traceDBCarrierFamily{traceDBReservedCarrierFamilies[0]}
			families[0].Wire = candidate.wire
			registration := strings.Join(traceDBCarrierFamilyCensus(sources, families), "\n")
			rejected := strings.Contains(registration, "is not a versioned codrax wire token")
			if rejected == candidate.wantToken {
				t.Fatalf("registration grammar disagrees with runtime for %q: rejected=%t want_token=%t (%s)",
					candidate.wire, rejected, candidate.wantToken, registration)
			}
		})
	}
}

// TestOwnedTraceValidationEventInvalidNamesTheRow (F8, ruled: keep the gate,
// improve the surface): a device-authored row whose body squats the reserved
// wire grammar is still refused, and the typed reason detail identifies the
// offending row — physical line, event name and the first bytes of the body —
// on the same per-row lane the clock-regression witness uses.
func TestOwnedTraceValidationEventInvalidNamesTheRow(t *testing.T) {
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	headerLines := strings.Count(systraceHeader, "\n")
	for _, tc := range []struct {
		name   string
		header string
		body   string
	}{
		// A device-authored payload squatting the reserved grammar under a
		// known-type header. (Under an unknown-type header such as a bare
		// tracing_mark_write the row is refused earlier, by the unknown-row
		// digest gate, and never reaches this arm.)
		{name: "device_payload_squats_reserved_namespace", header: "hmfs_writepage", body: "codrax_agent/v2 started"},
		{name: "future_family_under_semantic_header", header: "hmfs_writepage", body: "codrax_zz_future_family/v1 semantic_authority=none payload_b64=AAEC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := prepareTraceDBRenderedRowEnvelope(2_000_000, 1, "worker", 25827, 25827, 1, 4, 2, true,
				tc.header+": "+tc.body)
			if err != nil {
				t.Fatal(err)
			}
			bytesBody := []byte(systraceHeader + known + row.line + "\n")
			target, sealed := adoptTraceDBPostvalidationFixture(t, bytesBody)
			_, coverage, err := validateSealedSystraceWithTraceQueryReceiptAndWire(
				context.Background(), sealed, target.FinalPath, 2, 0, 0, ownedTraceTestWireDigest(t, bytesBody))
			var invariant *traceDBOutputInvariantError
			if !errors.As(err, &invariant) || invariant.Reason != traceDBPostvalidationEventInvalid || coverage.Error != traceDBPostvalidationEventInvalid {
				t.Fatalf("reserved-namespace squatter escaped the gate: coverage=%+v err=%v", coverage, err)
			}
			columns := strings.Join(coverage.ColumnsPresent, "\n")
			wantPrefix := tc.body
			if len(wantPrefix) > maxTraceEventInvalidWitnessBodyBytes {
				wantPrefix = wantPrefix[:maxTraceEventInvalidWitnessBodyBytes]
			}
			for _, want := range []string{
				fmt.Sprintf("event_invalid_line=%d", headerLines+2),
				"event_invalid_event_name=" + tc.header,
				"event_invalid_kind=carrier_signature_under_foreign_header",
				fmt.Sprintf("event_invalid_body_prefix=%q", wantPrefix),
			} {
				if !strings.Contains(columns, want) {
					t.Fatalf("event_invalid detail does not name the offending row (%q missing):\n%s\nerr=%v", want, columns, err)
				}
			}
			// The typed witness rides the provider error graph (errors.As), the
			// same way the clock-regression witness reaches the diagnostic
			// report.
			var witness *TraceEventInvalidWitnessError
			if !errors.As(err, &witness) || witness.Line != headerLines+2 || witness.EventName != tc.header ||
				witness.Kind != TraceEventInvalidCarrierSignatureUnderForeignHeader || witness.BodyPrefix != wantPrefix {
				t.Fatalf("typed error graph does not carry the row identification: witness=%+v err=%v", witness, err)
			}
			if !strings.Contains(witness.Error(), fmt.Sprintf("line=%d", headerLines+2)) || !strings.Contains(witness.Error(), tc.header) {
				t.Fatalf("witness text does not name the row: %v", witness)
			}
		})
	}
	// The event-side arm (parsed visibility carrier under a foreign header)
	// names its row through the same lane.
	format := traceDBRawVisibilityFormat("sched_migrate_task")
	payload, digest, err := traceDBSourceRawVisibilitySchemaFor(format)
	if err != nil {
		t.Fatal(err)
	}
	carrierBody, err := traceDBSourceRawVisibilityBody(format, traceDBRawVisibilityContent(format),
		&traceDBSourceRawVisibilitySchemaWire{payload: payload, digest: digest})
	if err != nil {
		t.Fatal(err)
	}
	row, err := prepareTraceDBRenderedRowEnvelope(2_000_000, 1, "worker", 25827, 25827, 1, 4, 2, true,
		"sched_migrate_task: "+carrierBody)
	if err != nil {
		t.Fatal(err)
	}
	bytesBody := []byte(systraceHeader + known + row.line + "\n")
	target, sealed := adoptTraceDBPostvalidationFixture(t, bytesBody)
	_, coverage, err := validateSealedSystraceWithTraceQueryReceiptAndWire(
		context.Background(), sealed, target.FinalPath, 2, 0, 1, ownedTraceTestWireDigest(t, bytesBody))
	if coverage.Error != traceDBPostvalidationEventInvalid {
		t.Fatalf("foreign-header carrier escaped: coverage=%+v err=%v", coverage, err)
	}
	columns := strings.Join(coverage.ColumnsPresent, "\n")
	for _, want := range []string{
		fmt.Sprintf("event_invalid_line=%d", headerLines+2),
		"event_invalid_event_name=sched_migrate_task",
		"event_invalid_event_type=" + string(tracequery.EventSourceRawVisibility),
		"event_invalid_body_prefix=\"" + tracequery.SourceRawVisibilityWire + " ",
	} {
		if !strings.Contains(columns, want) {
			t.Fatalf("foreign-header carrier detail does not name the row (%q missing):\n%s", want, columns)
		}
	}
}
