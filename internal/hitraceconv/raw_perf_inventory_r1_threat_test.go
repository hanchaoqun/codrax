package hitraceconv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestR1ThreatRawPerfInventoryIssueFamilies(t *testing.T) {
	tests := []struct {
		name          string
		includeSample bool
		records       [][]byte
		want          RawPerfCaptureCompleteness
		wantReady     bool
	}{
		{
			name: "lost_samples_positive",
			records: [][]byte{
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(9)),
			},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostSampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
				capture.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 9}
				return capture
			}(),
		},
		{
			name: "lost_events_overflow",
			records: [][]byte{
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, math.MaxUint64)),
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(2, 1)),
			},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
				capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}
				return capture
			}(),
		},
		{
			name: "lost_samples_overflow",
			records: [][]byte{
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(math.MaxUint64)),
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(1)),
			},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostSampleRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
				capture.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}
				return capture
			}(),
		},
		{
			name:    "malformed_lost",
			records: [][]byte{rawPerfRecord(perfRecordLost, nil)},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostRecords = RawPerfRecordCensus{Physical: 1, Rejected: 1}
				capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
				return capture
			}(),
		},
		{
			name:    "malformed_lost_samples",
			records: [][]byte{rawPerfRecord(perfRecordLostSamples, nil)},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.LostSampleRecords = RawPerfRecordCensus{Physical: 1, Rejected: 1}
				capture.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
				return capture
			}(),
		},
		{
			name:    "malformed_aux",
			records: [][]byte{rawPerfRecord(perfRecordAux, nil)},
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.AuxRecords = RawPerfRecordCensus{Physical: 1, Rejected: 1}
				capture.AuxBytes = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
				return capture
			}(),
		},
		{
			name:          "positive_clean",
			includeSample: true,
			wantReady:     true,
			want: func() RawPerfCaptureCompleteness {
				capture := newRawPerfCaptureCompleteness()
				capture.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
				return capture
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withR1RawPerfProviderPublication(t, test.includeSample, test.records, func(publication r1RawPerfPublication) {
				wantRows := 0
				if test.wantReady {
					wantRows = 1
				}
				assertR1RawPerfProjection(t, publication, test.want, wantRows, test.wantReady)
			})
		})
	}
}

func TestR1ThreatRawPerfReadinessAndCoverageForgeryFailClosed(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))}
	withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
		forgedArtifact := cloneArtifact(publication.artifact)
		forgedArtifact.Perf.TraceQueryReady = true
		if _, err := validateOwnedPerfTraceArtifactClaim(publication.ledger, forgedArtifact, ownedTracePerfRaw); !ownedTraceOutputHardFailure(err) {
			t.Fatalf("inventory artifact readiness forgery escaped: %T %v", err, err)
		}

		forgedDecision := publication.result
		forgedDecision.Artifacts = cloneArtifactList(publication.result.Artifacts)
		forgedDecision.ProviderDecisions = append([]PerfProviderDecision(nil), publication.result.ProviderDecisions...)
		forgedDecision.TraceCoverage = cloneTraceDBCoverageList(publication.result.TraceCoverage)
		forgedDecision.ProviderDecisions[0].TraceQueryReady = true
		if err := reconcileResultOwnedPerfReceipts(&forgedDecision, publication.ledger); !ownedTraceOutputHardFailure(err) {
			t.Fatalf("inventory decision readiness forgery escaped: %T %v", err, err)
		}

		wrongLane := publication.result
		wrongLane.Artifacts = cloneArtifactList(publication.result.Artifacts)
		wrongLane.ProviderDecisions = append([]PerfProviderDecision(nil), publication.result.ProviderDecisions...)
		wrongLane.TraceCoverage = cloneTraceDBCoverageList(publication.result.TraceCoverage)
		wrongLane.TraceDBCoverage = []TraceDBCoverage{{
			Table: "unrelated_db_diagnostic", Found: true,
			RawCaptureCompleteness: cloneRawPerfCaptureCompleteness(publication.receipt.rawCaptureCompleteness),
		}}
		if err := reconcileResultOwnedPerfReceipts(&wrongLane, publication.ledger); !ownedTraceOutputHardFailure(err) {
			t.Fatalf("raw census escaped into trace DB coverage: %T %v", err, err)
		}

		forgedCensus := cloneArtifact(publication.artifact)
		forgedCensus.Perf.RawCaptureCompleteness.LostEvents.Value++
		if artifactDedupeKey(forgedCensus) == artifactDedupeKey(publication.artifact) {
			t.Fatal("perf artifact dedupe key hid a raw census-only drift")
		}
	})
}

func TestR1ThreatRawPerfInventoryBundlePinsRelativeChildMeasurements(t *testing.T) {
	records := [][]byte{rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(9))}
	withR1RawPerfProviderPublication(t, false, records, func(publication r1RawPerfPublication) {
		bundle, err := writeTraceBundleWithAllCoverageAndLedger(
			context.Background(), publication.decision.InputPath,
			filepath.Join(filepath.Dir(publication.artifact.Path), "measured-inventory"),
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
		var metadata traceBundleMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			t.Fatal(err)
		}
		wantPath := filepath.Base(publication.artifact.Path)
		if len(metadata.Artifacts) != 1 || metadata.Artifacts[0].Path != wantPath ||
			metadata.Artifacts[0].Bytes != publication.artifact.Bytes || metadata.Artifacts[0].SHA256 != publication.artifact.SHA256 ||
			len(metadata.ProviderDecisions) != 1 || !metadata.ProviderDecisions[0].Succeeded ||
			metadata.ProviderDecisions[0].TraceQueryReady || metadata.ProviderDecisions[0].ArtifactPath != wantPath ||
			metadata.ProviderDecisions[0].OutputPath != wantPath || len(metadata.TraceCoverage) != 1 ||
			metadata.TraceCoverage[0].ArtifactPath != wantPath {
			t.Fatalf("inventory child measurement/path projection drifted: %+v", metadata)
		}
		if publication.artifact.Path == wantPath || !filepath.IsAbs(publication.artifact.Path) ||
			publication.result.ProviderDecisions[0].ArtifactPath != publication.artifact.Path ||
			publication.result.TraceCoverage[0].ArtifactPath != publication.artifact.Path {
			t.Fatalf("bundle rewrite mutated the public Result: %+v", publication.result)
		}
	})
}

func TestR1ThreatRawPerfInventoryWriterFailureLeavesNoGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.perftrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	capture := newRawPerfCaptureCompleteness()
	capture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 7}
	residual := RawPerfCaptureResidual{Profile: rawPerfCaptureResidualProfile, Source: rawPerfCaptureResidualSource}
	admission := rawPerfTestQueryableAdmission(0)
	_, err = writeValidatedOwnedPerfTraceWithLedger(
		context.Background(), ownedPerfTraceWriteSpec{
			Profile: ownedTracePerfRaw, ExpectedRows: 0, RawCaptureCompleteness: &capture,
			RawCaptureResidual: &residual, RawSampleAdmission: &admission,
		}, path, ledger, func(io.Writer) error { return syscall.ENOSPC },
	)
	if !ownedTraceOutputHardFailure(err) || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("raw inventory writer failure lost hard identity: %T %v", err, err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed inventory writer left public output: %v", statErr)
	}
	if _, ok := ledger.ownedTraceValidation(path); ok || len(ledger.created) != 0 {
		t.Fatalf("failed inventory writer left ledger state: %+v", ledger.created)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-perftrace-output-") {
			t.Fatalf("failed inventory writer left private staging: %s", entry.Name())
		}
	}
}
