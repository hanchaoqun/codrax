package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type traceBundleSystraceFixture struct {
	input    string
	output   string
	ledger   *conversionFileLedger
	artifact Artifact
	decision TraceProviderDecision
	coverage TraceDBCoverage
}

func newTraceBundleSystraceFixture(t testing.TB, rows []renderedRow) traceBundleSystraceFixture {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	artifact, decision, coverage := validatedResultBuiltinSystraceFixture(
		t, ledger, input, filepath.Join(dir, "capture.systrace"), rows,
	)
	return traceBundleSystraceFixture{
		input: input, output: artifact.Path, ledger: ledger,
		artifact: artifact, decision: decision, coverage: coverage,
	}
}

func (fixture traceBundleSystraceFixture) cleanup(t testing.TB) {
	t.Helper()
	if err := fixture.ledger.cleanup(); err != nil {
		t.Errorf("cleanup systrace bundle fixture: %v", err)
	}
}

func TestTraceBundleSystraceReceiptBoundaryRejectsOneSidedOrForgedClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*[]Artifact, *[]TraceProviderDecision, *[]TraceDBCoverage, *[]TraceDBCoverage)
	}{
		{name: "type_only", mutate: func(artifacts *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			(*artifacts)[0].Trace = nil
		}},
		{name: "artifact_bytes", mutate: func(artifacts *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			(*artifacts)[0].Bytes++
		}},
		{name: "artifact_sha", mutate: func(artifacts *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			(*artifacts)[0].SHA256 = "00"
		}},
		{name: "missing_decision", mutate: func(_ *[]Artifact, decisions *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			*decisions = nil
		}},
		{name: "duplicate_decision", mutate: func(_ *[]Artifact, decisions *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			*decisions = append(*decisions, (*decisions)[0])
		}},
		{name: "missing_coverage", mutate: func(_ *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, coverage *[]TraceDBCoverage) {
			*coverage = nil
		}},
		{name: "duplicate_coverage", mutate: func(_ *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, coverage *[]TraceDBCoverage) {
			*coverage = append(*coverage, cloneTraceDBCoverage((*coverage)[0]))
		}},
		{name: "orphan_coverage", mutate: func(_ *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, coverage *[]TraceDBCoverage) {
			(*coverage)[0].ArtifactPath += ".orphan"
		}},
		{name: "wrong_coverage_lane", mutate: func(_ *[]Artifact, _ *[]TraceProviderDecision, dbCoverage *[]TraceDBCoverage, coverage *[]TraceDBCoverage) {
			*dbCoverage = append(*dbCoverage, (*coverage)[0])
			*coverage = nil
		}},
		{name: "raw_duplicate_artifact", mutate: func(artifacts *[]Artifact, _ *[]TraceProviderDecision, _ *[]TraceDBCoverage, _ *[]TraceDBCoverage) {
			*artifacts = append(*artifacts, (*artifacts)[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTraceBundleSystraceFixture(t, []renderedRow{builtinWriterKnownRow(1_000_000, 0)})
			defer fixture.cleanup(t)
			artifacts := []Artifact{fixture.artifact}
			decisions := []TraceProviderDecision{fixture.decision}
			var dbCoverage []TraceDBCoverage
			coverage := []TraceDBCoverage{cloneTraceDBCoverage(fixture.coverage)}
			test.mutate(&artifacts, &decisions, &dbCoverage, &coverage)
			bundle, err := writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), fixture.input, fixture.output,
				artifacts, nil, nil, decisions, dbCoverage, coverage, fixture.ledger,
			)
			if err == nil || !reflect.DeepEqual(bundle, Artifact{}) || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("forged systrace bundle inputs were accepted: bundle=%+v err=%v", bundle, err)
			}
			bundlePath := traceSidecarBase(fixture.input, fixture.output) + ".tracebundle.json"
			if _, statErr := os.Lstat(bundlePath); !os.IsNotExist(statErr) {
				t.Fatalf("failed systrace receipt audit left a manifest: %v", statErr)
			}
		})
	}
}

func TestTraceBundleSystraceReceiptRewritesManifestCopiesOnly(t *testing.T) {
	fixture := newTraceBundleSystraceFixture(t, []renderedRow{builtinWriterKnownRow(1_000_000, 0)})
	defer fixture.cleanup(t)
	failure := traceProviderFailure(
		newTraceProviderDecision(
			traceProviderStageTraceBody,
			traceProviderByName(traceProviderNameTraceStreamer),
			Options{TraceEngine: traceEngineAuto},
			fixture.input,
			fixture.output,
		),
		"trace_db_normalize_failed",
		"trace_streamer DB did not produce a validated systrace",
	)
	decisions := []TraceProviderDecision{failure, fixture.decision}
	coverage := []TraceDBCoverage{{
		Family: tracebundle.SystraceReceiptFamily,
		Table:  tracebundle.SystraceReceiptTableSQL,
		Role:   tracebundle.SystraceReceiptRole,
		Found:  true, Error: traceDBPostvalidationScanFailed,
	}, cloneTraceDBCoverage(fixture.coverage)}
	wantDecisions := append([]TraceProviderDecision(nil), decisions...)
	wantCoverage := cloneTraceDBCoverageList(coverage)
	bundle, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), fixture.input, fixture.output,
		[]Artifact{fixture.artifact}, nil, nil, decisions, nil, coverage, fixture.ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bundle.Path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest traceBundleMetadata
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Systrace != "capture.systrace" || len(manifest.Artifacts) != 1 ||
		manifest.Artifacts[0].Path != "capture.systrace" || len(manifest.TraceDecisions) != 2 ||
		manifest.TraceDecisions[0].OutputPath != "capture.systrace" ||
		manifest.TraceDecisions[0].ArtifactPath != "" ||
		manifest.TraceDecisions[1].OutputPath != "capture.systrace" ||
		manifest.TraceDecisions[1].ArtifactPath != "capture.systrace" ||
		len(manifest.TraceCoverage) != 2 || manifest.TraceCoverage[0].ArtifactPath != "" ||
		manifest.TraceCoverage[1].ArtifactPath != "capture.systrace" {
		t.Fatalf("systrace manifest paths were not rewritten exactly: %+v", manifest)
	}
	if !reflect.DeepEqual(decisions, wantDecisions) || !reflect.DeepEqual(coverage, wantCoverage) {
		t.Fatalf("bundle rewrite mutated public collections: decisions=%+v coverage=%+v", decisions, coverage)
	}
}

func TestTraceBundleSystraceReceiptRejectsSameSizePublicRewrite(t *testing.T) {
	fixture := newTraceBundleSystraceFixture(t, []renderedRow{builtinWriterKnownRow(1_000_000, 0)})
	// This test deliberately invalidates the public generation. The ledger is
	// still closed, but its cleanup error is part of the expected fail-closed
	// signal rather than a fixture leak.
	defer func() { _ = fixture.ledger.cleanup() }()
	body, err := os.ReadFile(fixture.output)
	if err != nil {
		t.Fatal(err)
	}
	replacement := append([]byte(nil), body...)
	replacement[len(replacement)-2] ^= 1
	if err := os.WriteFile(fixture.output, replacement, 0o644); err != nil {
		t.Fatal(err)
	}
	forged := fixture.artifact
	forged.Bytes = int64(len(replacement))
	bundle, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), fixture.input, fixture.output,
		[]Artifact{forged}, nil, nil, []TraceProviderDecision{fixture.decision}, nil,
		[]TraceDBCoverage{fixture.coverage}, fixture.ledger,
	)
	if err == nil || !reflect.DeepEqual(bundle, Artifact{}) || !ownedTraceOutputHardFailure(err) {
		t.Fatalf("same-size public rewrite was accepted: bundle=%+v err=%v", bundle, err)
	}
}

func TestTraceBundleSystraceReceiptHeldParityRejectsCoherentForgedDigest(t *testing.T) {
	fixture := newTraceBundleSystraceFixture(t, []renderedRow{builtinWriterKnownRow(1_000_000, 0)})
	defer fixture.cleanup(t)
	index, ok := fixture.ledger.byPath[fixture.artifact.traceReceiptBindingPath]
	if !ok || index < 0 || index >= len(fixture.ledger.created) ||
		fixture.ledger.created[index].traceValidation == nil {
		t.Fatal("systrace receipt ledger record is unavailable")
	}
	forged := sha256.Sum256([]byte("not the held systrace child"))
	fixture.ledger.created[index].traceValidation.receipt.wireSHA256 = forged
	artifact := fixture.artifact
	artifact.SHA256 = hex.EncodeToString(forged[:])
	bundle, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), fixture.input, fixture.output,
		[]Artifact{artifact}, nil, nil, []TraceProviderDecision{fixture.decision}, nil,
		[]TraceDBCoverage{fixture.coverage}, fixture.ledger,
	)
	var publication *ownedTracePublicationError
	if err == nil || !reflect.DeepEqual(bundle, Artifact{}) || !ownedTraceOutputHardFailure(err) ||
		!errors.As(err, &publication) || publication.Stage != "bind_bundle_systrace_receipt" {
		t.Fatalf("held systrace parity accepted a coherently forged digest: bundle=%+v err=%v", bundle, err)
	}
}

func TestTraceBundleSystraceReceiptGateOrderingPinned(t *testing.T) {
	finalizer := sourceGenerationFunctionBody(t, "result_perf_receipt.go", "finalizeResultTraceBundleWithLedger")
	systraceResultAt := strings.Index(finalizer, "reconcileResultOwnedSystraceReceipts(result, ledger)")
	primaryParityAt := strings.Index(finalizer, "bundleOutputPath != result.OutputPath")
	perfResultAt := strings.Index(finalizer, "reconcileResultOwnedPerfReceipts(result, ledger)")
	normalizeAt := strings.Index(finalizer, "normalizeResultCollections(result)")
	bundleAt := strings.Index(finalizer, "writeTraceBundleWithAllCoverageAndLedger(")
	if systraceResultAt < 0 || primaryParityAt <= systraceResultAt || perfResultAt <= primaryParityAt ||
		normalizeAt <= perfResultAt || bundleAt <= normalizeAt {
		t.Fatalf("Result finalizer lost systrace->perf->normalize->bundle ordering:\n%s", finalizer)
	}

	writer := sourceGenerationFunctionBody(t, "standalone.go", "writeTraceBundleWithAllCoverageAndGatesAndLedgerOps")
	rawCopyAt := strings.Index(writer, "rawArtifacts := append([]Artifact(nil), artifacts...)")
	dedupeAt := strings.Index(writer, "artifacts = dedupeArtifacts(artifacts)")
	systraceAuditAt := strings.Index(writer, "auditTraceBundleOwnedSystraceReceipts(outputPath, rawArtifacts,")
	perfAuditAt := strings.Index(writer, "auditTraceBundleOwnedPerfReceipts(rawArtifacts,")
	buildAt := strings.Index(writer, "buildTraceBundleV2Artifacts(")
	systraceRewriteAt := strings.Index(writer, "rewriteTraceBundleSystraceMetadata(")
	if rawCopyAt < 0 || dedupeAt <= rawCopyAt || systraceAuditAt <= dedupeAt || perfAuditAt <= systraceAuditAt ||
		buildAt <= perfAuditAt || systraceRewriteAt <= buildAt {
		t.Fatalf("bundle writer lost raw systrace receipt audit/rewrite ordering:\n%s", writer)
	}

	builder := sourceGenerationFunctionBody(t, "standalone.go", "buildTraceBundleV2Artifacts")
	claimAt := strings.Index(builder, "validateOwnedSystraceArtifactClaim(ledger, publicArtifact, kind)")
	bindingAt := strings.Index(builder, "bindingPath = publicArtifact.traceReceiptBindingPath")
	holdAt := strings.Index(builder, "ledger.holdAndMeasureSealedOwnedPath(ctx, bindingPath)")
	parityAt := strings.Index(builder, "systraceClaim.receipt.size != measuredBytes")
	assignAt := strings.Index(builder, "out[i].Bytes = measuredBytes")
	if claimAt < 0 || bindingAt <= claimAt || holdAt <= bindingAt || parityAt <= holdAt || assignAt <= parityAt {
		t.Fatalf("bundle builder lost receipt->frozen binding->held parity->projection ordering:\n%s", builder)
	}
	if strings.Count(builder, "if os.SameFile(prior, info)") != 1 {
		t.Fatalf("bundle builder lost its physical causal-child alias guard:\n%s", builder)
	}
}

func TestFinalizeTraceBundleRejectsPrimarySelectorSplit(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	first, firstDecision, firstCoverage := validatedResultBuiltinSystraceFixture(
		t, ledger, input, filepath.Join(dir, "first.systrace"),
		[]renderedRow{builtinWriterKnownRow(1_000_000, 0)},
	)
	second, secondDecision, secondCoverage := validatedResultBuiltinSystraceFixture(
		t, ledger, input, filepath.Join(dir, "second.systrace"),
		[]renderedRow{builtinWriterKnownRow(2_000_000, 1)},
	)
	result := Result{
		InputPath: input, OutputPath: first.Path, OutputBytes: first.Bytes,
		EventsWritten: first.Trace.Rows, Artifacts: []Artifact{first, second},
		TraceDecisions: []TraceProviderDecision{firstDecision, secondDecision},
		TraceCoverage:  []TraceDBCoverage{firstCoverage, secondCoverage},
	}
	err = finalizeResultTraceBundleWithLedger(context.Background(), input, second.Path, &result, ledger)
	var publication *ownedTracePublicationError
	if err == nil || !ownedTraceOutputHardFailure(err) || !errors.As(err, &publication) ||
		publication.Stage != "finalize_result_primary_systrace" || result.BundlePath != "" {
		t.Fatalf("split Result/bundle primary was accepted: result=%+v err=%v", result, err)
	}
}

func TestTraceBundleSystracePrimaryControlsManifestAndPerfClockAuthority(t *testing.T) {
	tests := []struct {
		name            string
		inventoryFirst  bool
		primaryReady    bool
		wantPrimary     string
		wantTraceDomain string
	}{
		{
			name: "inventory_first_ready_primary", inventoryFirst: true, primaryReady: true,
			wantPrimary: "ready.systrace", wantTraceDomain: "trace_seconds",
		},
		{
			name: "ready_first_inventory_primary", inventoryFirst: false, primaryReady: false,
			wantPrimary: "inventory.systrace", wantTraceDomain: "trace_body_not_query_ready",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(input)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			inventory, inventoryDecision, inventoryCoverage := validatedResultBuiltinSystraceFixture(
				t, ledger, input, filepath.Join(dir, "inventory.systrace"), nil,
			)
			ready, readyDecision, readyCoverage := validatedResultBuiltinSystraceFixture(
				t, ledger, input, filepath.Join(dir, "ready.systrace"),
				[]renderedRow{builtinWriterKnownRow(1_000_000, 0)},
			)
			perf, perfDecision := validatedResultPerfFixture(
				t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "capture.perftrace"),
			)
			artifacts := []Artifact{ready, inventory, perf}
			decisions := []TraceProviderDecision{readyDecision, inventoryDecision}
			coverage := []TraceDBCoverage{readyCoverage, inventoryCoverage}
			if test.inventoryFirst {
				artifacts = []Artifact{inventory, ready, perf}
				decisions = []TraceProviderDecision{inventoryDecision, readyDecision}
				coverage = []TraceDBCoverage{inventoryCoverage, readyCoverage}
			}
			primary := inventory
			if test.primaryReady {
				primary = ready
			}
			result := Result{
				InputPath: input, OutputPath: primary.Path, OutputBytes: primary.Bytes,
				EventsWritten: primary.Trace.Rows, Artifacts: artifacts,
				ProviderDecisions: []PerfProviderDecision{perfDecision}, TraceDecisions: decisions,
				TraceCoverage: coverage,
			}
			if err := finalizeResultTraceBundleWithLedger(
				context.Background(), input, result.OutputPath, &result, ledger,
			); err != nil {
				t.Fatal(err)
			}
			if result.OutputPath != primary.Path || result.OutputBytes != primary.Bytes ||
				result.EventsWritten != primary.Trace.Rows {
				t.Fatalf("Result primary was mutated: %+v", result)
			}
			body, err := os.ReadFile(result.BundlePath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest traceBundleMetadata
			if err := json.Unmarshal(body, &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Systrace != test.wantPrimary || len(manifest.PerfClockAlignments) != 1 ||
				manifest.PerfClockAlignments[0].TraceTimeDomain != test.wantTraceDomain {
				t.Fatalf("primary/clock authority drifted: primary=%q alignments=%+v", manifest.Systrace, manifest.PerfClockAlignments)
			}
		})
	}
}
