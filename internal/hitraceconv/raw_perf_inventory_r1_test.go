package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type r1RawPerfPublication struct {
	artifact Artifact
	decision PerfProviderDecision
	result   Result
	receipt  ownedTraceValidationReceipt
	ledger   *conversionFileLedger
}

func withR1RawPerfProviderPublication(
	t *testing.T,
	includeSample bool,
	records [][]byte,
	check func(r1RawPerfPublication),
) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	output := filepath.Join(dir, "capture.perftrace")
	if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(includeSample, records...), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(input)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
	if err != nil {
		t.Fatal(err)
	}
	artifact, _, decisions, err := maybeConvertRawPerfDataFromInputWithDecision(
		context.Background(), Options{PerfParser: "raw"}, binding, output, "",
		perfProviderStageDirectInput, false, ledger,
	)
	if err != nil {
		t.Fatalf("publish raw provider fixture: %v", err)
	}
	if artifact.Path != output || len(decisions) != 1 {
		t.Fatalf("raw provider projection is incomplete: artifact=%+v decisions=%+v", artifact, decisions)
	}
	published, ok := ledger.ownedTraceValidation(artifact.Path)
	if !ok {
		t.Fatal("raw provider publication has no owned validation receipt")
	}
	result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: decisions}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatalf("reconcile raw provider receipt: %v", err)
	}
	check(r1RawPerfPublication{
		artifact: artifact,
		decision: decisions[0],
		result:   result,
		receipt:  published.receipt,
		ledger:   ledger,
	})
}

func assertR1RawPerfProjection(
	t *testing.T,
	publication r1RawPerfPublication,
	want RawPerfCaptureCompleteness,
	wantRows int,
	wantReady bool,
) {
	t.Helper()
	receipt := publication.receipt
	if receipt.kind != ownedTraceValidationPerf || receipt.perfProfile != ownedTracePerfRaw ||
		receipt.rows != wantRows || receipt.known != wantRows ||
		receipt.authoritativeKnown != wantRows || receipt.queryReady != wantReady ||
		!receipt.hasRawCaptureCompleteness || receipt.rawCaptureCompleteness != want {
		t.Fatalf("raw receipt projection drifted:\n got=%+v\nwant census=%+v rows=%d ready=%t",
			receipt, want, wantRows, wantReady)
	}
	artifact := publication.artifact
	if artifact.Type != ArtifactPerfTrace || artifact.Perf == nil ||
		artifact.Perf.TraceQueryReady != wantReady || artifact.Perf.RawCaptureCompleteness == nil ||
		*artifact.Perf.RawCaptureCompleteness != want {
		t.Fatalf("raw artifact capability drifted: %+v", artifact)
	}
	decision := publication.decision
	if !decision.Selected || !decision.Attempted || !decision.Succeeded ||
		decision.TraceQueryReady != wantReady || decision.ArtifactPath != artifact.Path {
		t.Fatalf("raw provider decision drifted: %+v", decision)
	}
	if len(publication.result.TraceCoverage) != 1 {
		t.Fatalf("raw receipt coverage cardinality drifted: %+v", publication.result.TraceCoverage)
	}
	coverage := publication.result.TraceCoverage[0]
	if coverage.Table != "perftrace_raw_perf" || coverage.ArtifactPath != artifact.Path ||
		!coverage.Found || coverage.Error != "" || coverage.RowsEmitted != wantRows ||
		coverage.RawCaptureCompleteness == nil || *coverage.RawCaptureCompleteness != want {
		t.Fatalf("raw receipt coverage drifted: %+v", coverage)
	}
	wantQueryPath := ""
	if wantReady {
		wantQueryPath = artifact.Path
	}
	if got := QueryReadyPerfTracePath([]Artifact{artifact}); got != wantQueryPath {
		t.Fatalf("query-ready selector=%q want=%q", got, wantQueryPath)
	}
	if got := HasQueryReadyPerfTrace([]Artifact{artifact}); got != wantReady {
		t.Fatalf("query-ready presence=%t want=%t", got, wantReady)
	}
	if ready, err := hasAnalyzableStandaloneSidecar(context.Background(), []Artifact{artifact}, publication.ledger); err != nil || ready != wantReady {
		t.Fatalf("standalone selector ready=%t want=%t err=%v", ready, wantReady, err)
	}

	// Receipt values are ledger authority. Public pointer projections must be
	// independent clones, and even a returned receipt copy must not alias the
	// stored generation.
	artifact = cloneArtifact(artifact)
	coverage = cloneTraceDBCoverage(coverage)
	artifact.Perf.RawCaptureCompleteness.Profile = "forged_artifact_alias"
	coverage.RawCaptureCompleteness.Source = "forged_coverage_alias"
	receipt.rawCaptureCompleteness.Profile = "forged_receipt_copy"
	again, ok := publication.ledger.ownedTraceValidation(publication.artifact.Path)
	if !ok || !again.receipt.hasRawCaptureCompleteness || again.receipt.rawCaptureCompleteness != want {
		t.Fatalf("public/copy mutation changed ledger receipt: ok=%t receipt=%+v", ok, again.receipt)
	}
}

func TestR1RawPerfLossAndMalformedOnlyPublishExactHeaderInventory(t *testing.T) {
	tests := []struct {
		name    string
		records [][]byte
		want    RawPerfCaptureCompleteness
	}{
		{
			name: "loss_only",
			records: [][]byte{
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)),
			},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
				capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 7}
				return capture
			}(),
		},
		{
			name: "malformed_sample_only",
			records: [][]byte{
				rawPerfRecord(perfRecordSample, nil),
			},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.SampleRecords = RawPerfRecordCensus{Physical: 1, Rejected: 1}
				return capture
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "legacy.perf.data")
			legacyOutput := filepath.Join(dir, "legacy.perftrace")
			if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(false, test.records...), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), input, legacyOutput); err == nil ||
				!strings.Contains(err.Error(), "contains no supported sample records") {
				t.Fatalf("legacy direct API accepted inventory-only input: %v", err)
			}
			if _, err := os.Lstat(legacyOutput); !os.IsNotExist(err) {
				t.Fatalf("legacy direct API published inventory output: %v", err)
			}

			withR1RawPerfProviderPublication(t, false, test.records, func(publication r1RawPerfPublication) {
				body, err := os.ReadFile(publication.artifact.Path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(body, []byte(systraceHeader)) {
					t.Fatalf("inventory is not exact header-only wire: %q", body)
				}
				assertR1RawPerfProjection(t, publication, test.want, 0, false)
			})
		})
	}
}

func TestR1RawPerfZeroWithoutLossEvidenceFailsBeforePublication(t *testing.T) {
	tests := []struct {
		name    string
		records [][]byte
	}{
		{name: "clean_zero"},
		{name: "exact_zero_loss", records: [][]byte{
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 0)),
		}},
		{name: "accepted_aux_only", records: [][]byte{
			rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)),
		}},
		{name: "aux_aggregate_overflow", records: [][]byte{
			rawPerfRecord(perfRecordAux, rawPerfAuxPayload(math.MaxUint64)),
			rawPerfRecord(perfRecordAux, rawPerfAuxPayload(1)),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.perf.data")
			output := filepath.Join(dir, "capture.perftrace")
			if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(false, test.records...), 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(input)
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			ledger, err := newConversionFileLedgerForAuthority(authority)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
			if err != nil {
				t.Fatal(err)
			}
			artifact, _, decisions, convertErr := maybeConvertRawPerfDataFromInputWithDecision(
				context.Background(), Options{PerfParser: "raw"}, binding, output, "",
				perfProviderStageDirectInput, false, ledger,
			)
			if artifact.Path != "" {
				t.Fatalf("zero input without loss evidence published an artifact: artifact=%+v err=%v", artifact, convertErr)
			}
			if convertErr != nil {
				t.Fatalf("provider soft-fail escaped as a top-level error: %v", convertErr)
			}
			if len(decisions) != 1 || !decisions[0].Selected || !decisions[0].Attempted ||
				decisions[0].Succeeded || decisions[0].TraceQueryReady || decisions[0].ArtifactPath != "" ||
				strings.TrimSpace(decisions[0].Reason) == "" || strings.TrimSpace(decisions[0].Caveat) == "" {
				t.Fatalf("zero input without loss evidence lacks one explicit attempted failure decision: %+v", decisions)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("rejected zero input left public output: %v", err)
			}
			staging, err := filepath.Glob(filepath.Join(dir, ownedPerfTraceStagingPattern))
			if err != nil {
				t.Fatal(err)
			}
			if len(staging) != 0 {
				t.Fatalf("rejected zero input left private staging: %v", staging)
			}
			if _, ok := ledger.ownedTraceValidation(output); ok {
				t.Fatal("rejected zero input left a ledger receipt")
			}
		})
	}
}

func TestR1RawPerfPositiveSampleWithLossRemainsReady(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))}
	want := newRawPerfCaptureCompleteness()
	want.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	want.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	want.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 7}

	dir := t.TempDir()
	input := filepath.Join(dir, "legacy.perf.data")
	legacyOutput := filepath.Join(dir, "legacy.perftrace")
	if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(true, records...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), input, legacyOutput); err != nil {
		t.Fatalf("positive legacy direct conversion regressed: %v", err)
	}
	legacyWire, err := os.ReadFile(legacyOutput)
	if err != nil {
		t.Fatal(err)
	}

	withR1RawPerfProviderPublication(t, true, records, func(publication r1RawPerfPublication) {
		providerWire, err := os.ReadFile(publication.artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(providerWire, legacyWire) || strings.Count(string(providerWire), "perf_sample:") != 1 {
			t.Fatalf("positive+loss provider/legacy wire parity drifted:\nlegacy=%q\nprovider=%q", legacyWire, providerWire)
		}
		assertR1RawPerfProjection(t, publication, want, 1, true)
	})
}

func TestR1RawPerfInventoryBundlePreservesCensusWithoutSampleCapability(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))}
	want := newRawPerfCaptureCompleteness()
	want.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	want.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 7}

	withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
		dir := filepath.Dir(publication.artifact.Path)
		bundleBase := filepath.Join(dir, "inventory-bundle")

		manifestArtifacts, _, held, err := buildTraceBundleV2Artifacts(
			context.Background(), bundleBase+".tracebundle.json", publication.result.Artifacts, publication.ledger,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := closeHeldSealedOwnedFiles(held); err != nil {
			t.Fatal(err)
		}
		if len(manifestArtifacts) != 1 || manifestArtifacts[0].Perf == nil ||
			manifestArtifacts[0].Perf.RawCaptureCompleteness == nil {
			t.Fatalf("bundle child lost raw inventory capability: %+v", manifestArtifacts)
		}
		manifestArtifacts[0].Perf.RawCaptureCompleteness.Profile = "mutated_manifest_copy"
		if publication.artifact.Perf.RawCaptureCompleteness == nil ||
			*publication.artifact.Perf.RawCaptureCompleteness != want {
			t.Fatalf("bundle projection aliased the public Result artifact: %+v", publication.artifact.Perf)
		}

		bundle, err := writeTraceBundleWithAllCoverageAndLedger(
			context.Background(), publication.decision.InputPath, bundleBase,
			publication.result.Artifacts, nil, publication.result.ProviderDecisions, nil,
			publication.result.TraceDBCoverage, publication.result.TraceCoverage, publication.ledger,
		)
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(bundle.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(`"trace_query_ready": false`)) {
			t.Fatalf("inventory bundle omitted explicit false readiness: %s", body)
		}
		var metadata traceBundleMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata.Artifacts) != 1 || metadata.Artifacts[0].Perf == nil ||
			metadata.Artifacts[0].Perf.TraceQueryReady || metadata.Artifacts[0].Perf.RawCaptureCompleteness == nil ||
			*metadata.Artifacts[0].Perf.RawCaptureCompleteness != want ||
			len(metadata.ProviderDecisions) != 1 || metadata.ProviderDecisions[0].TraceQueryReady ||
			len(metadata.TraceCoverage) != 1 || metadata.TraceCoverage[0].RawCaptureCompleteness == nil ||
			*metadata.TraceCoverage[0].RawCaptureCompleteness != want || len(metadata.PerfClockAlignments) != 0 {
			t.Fatalf("inventory bundle projection drifted: %+v", metadata)
		}
	})
}

func TestR1RawPerfInventoryDoesNotMaskReadySiblingOrForgedClaim(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))}
	withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
		readyPath := filepath.Join(filepath.Dir(publication.artifact.Path), "ready.perftrace")
		writeOneValidatedPerfTraceForClaimTest(t, ownedTracePerfSimpleperfText, readyPath, publication.ledger)
		readyArtifact, err := newValidatedPerfTraceArtifact(
			publication.ledger, readyPath, ownedTracePerfSimpleperfText,
			perfInputLinuxPerfData, "fixture", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		artifacts := []Artifact{publication.artifact, readyArtifact}
		if got := QueryReadyPerfTracePath(artifacts); got != readyPath || !HasQueryReadyPerfTrace(artifacts) {
			t.Fatalf("inventory masked ready sibling: path=%q artifacts=%+v", got, artifacts)
		}
		if ready, err := hasAnalyzableStandaloneSidecar(context.Background(), artifacts, publication.ledger); err != nil || !ready {
			t.Fatalf("ready sibling was not analyzable: ready=%t err=%v", ready, err)
		}

		forgedInventory := cloneArtifact(publication.artifact)
		forgedInventory.Perf.RawCaptureCompleteness.LostEvents.Value++
		if ready, err := hasAnalyzableStandaloneSidecar(
			context.Background(), []Artifact{forgedInventory, readyArtifact}, publication.ledger,
		); err == nil || ready || !ownedTraceOutputHardFailure(err) {
			t.Fatalf("forged inventory was hidden by ready sibling: ready=%t err=%T %v", ready, err, err)
		}
	})
}

func TestR1RawPerfCensusTamperAndNonRawInjectionFailClosed(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))}

	t.Run("raw_receipt_missing", func(t *testing.T) {
		withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
			record := traceBundlePerfLedgerRecord(t, publication.ledger, publication.artifact.Path)
			record.traceValidation.receipt.hasRawCaptureCompleteness = false
			if _, err := validateOwnedPerfTraceArtifactClaim(publication.ledger, publication.artifact, ownedTracePerfRaw); !ownedTraceOutputHardFailure(err) {
				t.Fatalf("raw receipt without census escaped: %T %v", err, err)
			}
		})
	})

	t.Run("raw_receipt_tamper", func(t *testing.T) {
		withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
			record := traceBundlePerfLedgerRecord(t, publication.ledger, publication.artifact.Path)
			record.traceValidation.receipt.rawCaptureCompleteness.LostEvents.Value++
			if _, err := validateOwnedPerfTraceArtifactClaim(publication.ledger, publication.artifact, ownedTracePerfRaw); !ownedTraceOutputHardFailure(err) {
				t.Fatalf("tampered raw receipt escaped: %T %v", err, err)
			}
		})
	})

	for _, test := range []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "artifact_missing", mutate: func(artifact *Artifact) {
			capability := *artifact.Perf
			artifact.Perf = &capability
			artifact.Perf.RawCaptureCompleteness = nil
		}},
		{name: "artifact_tamper", mutate: func(artifact *Artifact) {
			capability := *artifact.Perf
			artifact.Perf = &capability
			capture := *capability.RawCaptureCompleteness
			artifact.Perf.RawCaptureCompleteness = &capture
			artifact.Perf.RawCaptureCompleteness.LostEvents.Value++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
				forged := publication.artifact
				test.mutate(&forged)
				if _, err := validateOwnedPerfTraceArtifactClaim(publication.ledger, forged, ownedTracePerfRaw); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("raw artifact census forgery escaped: %T %v", err, err)
				}
			})
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(*TraceDBCoverage)
	}{
		{name: "coverage_missing", mutate: func(coverage *TraceDBCoverage) {
			coverage.RawCaptureCompleteness = nil
		}},
		{name: "coverage_tamper", mutate: func(coverage *TraceDBCoverage) {
			capture := *coverage.RawCaptureCompleteness
			coverage.RawCaptureCompleteness = &capture
			coverage.RawCaptureCompleteness.LostEvents.Value++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
				result := publication.result
				result.Artifacts = append([]Artifact(nil), publication.result.Artifacts...)
				result.ProviderDecisions = append([]PerfProviderDecision(nil), publication.result.ProviderDecisions...)
				result.TraceCoverage = cloneTraceDBCoverageList(publication.result.TraceCoverage)
				test.mutate(&result.TraceCoverage[0])
				if err := reconcileResultOwnedPerfReceipts(&result, publication.ledger); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("raw coverage census forgery escaped: %T %v", err, err)
				}
			})
		})
	}

	for _, lane := range []string{"receipt", "artifact", "coverage"} {
		t.Run("nonraw_"+lane+"_injection", func(t *testing.T) {
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			artifact, decision := validatedResultPerfFixture(
				t, ledger, ownedTracePerfSimpleperfText, filepath.Join(t.TempDir(), "official.perftrace"),
			)
			capture := newRawPerfCaptureCompleteness()
			switch lane {
			case "receipt":
				record := traceBundlePerfLedgerRecord(t, ledger, artifact.Path)
				record.traceValidation.receipt.hasRawCaptureCompleteness = true
				record.traceValidation.receipt.rawCaptureCompleteness = capture
				if _, err := validateOwnedPerfTraceArtifactClaim(ledger, artifact, ownedTracePerfSimpleperfText); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("nonraw receipt accepted raw census: %T %v", err, err)
				}
			case "artifact":
				capability := *artifact.Perf
				artifact.Perf = &capability
				artifact.Perf.RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(capture)
				if _, err := validateOwnedPerfTraceArtifactClaim(ledger, artifact, ownedTracePerfSimpleperfText); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("nonraw artifact accepted raw census: %T %v", err, err)
				}
			case "coverage":
				result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{decision}}
				if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
					t.Fatal(err)
				}
				result.TraceCoverage[0].RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(capture)
				if err := reconcileResultOwnedPerfReceipts(&result, ledger); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("nonraw coverage accepted raw census: %T %v", err, err)
				}
			}
		})
	}
}
