package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBPostvalidationTypedRecordLines(kind string, tableID int, ordinal uint64, payload []byte, chunkBytes int) []string {
	recordDigest := sha256.Sum256(payload)
	recordHash := hex.EncodeToString(recordDigest[:])
	chunks := (len(payload) + chunkBytes - 1) / chunkBytes
	lines := make([]string, 0, chunks)
	for index := 0; index < chunks; index++ {
		start := index * chunkBytes
		end := min(start+chunkBytes, len(payload))
		chunk := payload[start:end]
		chunkDigest := sha256.Sum256(chunk)
		lines = append(lines, fmt.Sprintf(
			"# codrax_trace_db_record/v1 kind=%s table_id=%d row_ordinal=%d chunk=%d chunks=%d ts_ns=1000000 payload=%s chunk_sha256=%s record_sha256=%s\n",
			kind, tableID, ordinal, index+1, chunks,
			base64.RawURLEncoding.EncodeToString(chunk),
			hex.EncodeToString(chunkDigest[:]), recordHash,
		))
	}
	return lines
}

func traceDBPostvalidationSequenceRecord(kind string, tableID int, ordinal uint64, payload string) tracequery.TraceDBRecordFields {
	digest := sha256.Sum256([]byte(payload))
	return tracequery.TraceDBRecordFields{
		Kind:         kind,
		TableID:      tableID,
		RowOrdinal:   ordinal,
		Chunk:        1,
		Chunks:       1,
		TimestampNS:  1,
		PayloadBytes: len(payload),
		RecordSHA256: hex.EncodeToString(digest[:]),
		Payload:      []byte(payload),
	}
}

func TestOwnedTraceDBTextRecordSequenceV2PinsBlocksAndRejectsMixedCarrier(t *testing.T) {
	schema := traceDBPostvalidationSequenceRecord("schema", 1, 0, "schema")
	row := traceDBPostvalidationSequenceRecord("row", 1, 1, "row")
	receipt := traceDBPostvalidationSequenceRecord("receipt", 1, 0, "receipt")
	blockEvent := func(block int, records ...tracequery.TraceDBRecordFields) tracequery.Event {
		return tracequery.Event{
			Type: tracequery.EventTraceDBRecord,
			PluginFields: &tracequery.PluginFields{
				TraceDBBlock: &tracequery.TraceDBBlockFields{
					Block:       block,
					RecordCount: len(records),
					Records:     records,
				},
			},
		}
	}
	var sequence ownedTraceDBTextRecordSequence
	if !sequence.observe(blockEvent(1, schema, row)) ||
		!sequence.observe(blockEvent(2, receipt)) ||
		!sequence.complete(2) {
		t.Fatalf("valid v2 block sequence rejected: %+v", sequence)
	}

	var skippedBlock ownedTraceDBTextRecordSequence
	if skippedBlock.observe(blockEvent(2, schema)) {
		t.Fatal("v2 sequence admitted a noncontiguous first block")
	}

	var mixed ownedTraceDBTextRecordSequence
	if !mixed.observe(blockEvent(1, schema)) ||
		mixed.observe(tracequery.Event{
			Type: tracequery.EventTraceDBRecord,
			PluginFields: &tracequery.PluginFields{
				TraceDBRecord: &row,
			},
		}) {
		t.Fatal("v2 sequence admitted a physical v1 carrier after v2 began")
	}
}

func traceDBPostvalidationKnownLine(t *testing.T, tsNS int64) string {
	t.Helper()
	row, err := prepareTraceDBRenderedRow(
		tsNS,
		0,
		"waker",
		10,
		10,
		1,
		"sched_wakeup: comm=app pid=20 prio=53 target_cpu=2",
	)
	if err != nil {
		t.Fatal(err)
	}
	return row.line + "\n"
}

func adoptTraceDBPostvalidationFixture(t *testing.T, body []byte) (sealedConversionPublicationTarget, *sealedConversionFile) {
	t.Helper()
	finalPath := filepath.Join(t.TempDir(), "capture.systrace")
	target, err := prepareSealedConversionPublicationTarget(finalPath, ".codrax-sql-postvalidation-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := target.Cleanup(); cleanupErr != nil {
			t.Errorf("cleanup postvalidation fixture: %v", cleanupErr)
		}
	})
	if err := os.WriteFile(target.StagingPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sealed.Close(); closeErr != nil {
			t.Errorf("close postvalidation fixture: %v", closeErr)
		}
	})
	return target, sealed
}

func TestTraceDBSystraceHeldPostvalidationExactContract(t *testing.T) {
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	headerLines := strings.Count(systraceHeader, "\n")
	if coverage.Error != "" || !coverage.Found || coverage.RowsRead != headerLines+1 || coverage.RowsEmitted != 1 ||
		coverage.Table != traceDBPostvalidationCoverageTable || coverage.ArtifactPath != target.FinalPath {
		t.Fatalf("held postvalidation accounting drifted: %+v", coverage)
	}
	if _, err := os.Lstat(target.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("public output appeared during held validation: %v", err)
	}
}

func TestTraceDBSystraceHeldPostvalidationClosesTypedMultiChunkRecordHash(t *testing.T) {
	standard := traceDBPostvalidationKnownLine(t, 1_000_000)
	schema := traceDBPostvalidationTypedRecordLines("schema", 1, 0, []byte(strings.Repeat("schema", 100)), 211)
	receipt := traceDBPostvalidationTypedRecordLines("receipt", 1, 0, []byte("receipt"), 211)
	validBody := []byte(systraceHeader + standard + strings.Join(schema, "") + strings.Join(receipt, ""))
	target, sealed := adoptTraceDBPostvalidationFixture(t, validBody)
	expectedRows := 1 + len(schema) + len(receipt)
	if _, coverage, err := validateSealedSystraceWithTraceQueryReceipt(
		t.Context(), sealed, target.FinalPath, expectedRows, len(schema)+len(receipt),
	); err != nil {
		t.Fatalf("valid typed multi-chunk record failed: coverage=%+v err=%v", coverage, err)
	}

	parts := strings.Fields(schema[0])
	validHash := strings.TrimPrefix(parts[len(parts)-1], "record_sha256=")
	forgedHash := strings.Repeat("0", sha256.Size*2)
	if validHash == forgedHash {
		t.Fatal("fixture unexpectedly has all-zero record hash")
	}
	forgedSchema := strings.ReplaceAll(strings.Join(schema, ""), "record_sha256="+validHash, "record_sha256="+forgedHash)
	forgedBody := []byte(systraceHeader + standard + forgedSchema + strings.Join(receipt, ""))
	forgedTarget, forgedSealed := adoptTraceDBPostvalidationFixture(t, forgedBody)
	_, coverage, err := validateSealedSystraceWithTraceQueryReceipt(
		t.Context(), forgedSealed, forgedTarget.FinalPath, expectedRows, len(schema)+len(receipt),
	)
	reason, typed := traceDBOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationEventInvalid ||
		coverage.Error != traceDBPostvalidationEventInvalid {
		t.Fatalf("forged full-record hash escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}
}

func TestTraceDBSystraceHeldPostvalidationFailsClosed(t *testing.T) {
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	regressed := traceDBPostvalidationKnownLine(t, 900_000)
	unknownRow, err := prepareTraceDBRenderedRow(1_000_000, 0, "worker", 10, 10, 1, "codrax_unknown_event: value=1")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		body         []byte
		expectedRows int
		wantReason   string
		wantWitness  bool
	}{
		{name: "header drift", body: append([]byte("!"+systraceHeader[1:]), []byte(known)...), expectedRows: 1, wantReason: traceDBPostvalidationHeaderInvalid},
		{name: "owned unknown", body: []byte(systraceHeader + unknownRow.line + "\n"), expectedRows: 1, wantReason: traceDBPostvalidationUnknownOwnedRow},
		{name: "owned unparsed", body: []byte(systraceHeader + "not an ftrace row\n"), expectedRows: 1, wantReason: traceDBPostvalidationUnparsedOwnedRow},
		{name: "clock regression", body: []byte(systraceHeader + known + regressed), expectedRows: 2, wantReason: traceDBPostvalidationClockRegression, wantWitness: true},
		{name: "count mismatch", body: []byte(systraceHeader + known), expectedRows: 2, wantReason: traceDBPostvalidationCountMismatch},
		{name: "missing lf", body: []byte(systraceHeader + strings.TrimSuffix(known, "\n")), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
		{name: "crlf", body: []byte(systraceHeader + strings.TrimSuffix(known, "\n") + "\r\n"), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
		{name: "line over cap", body: []byte(systraceHeader + strings.Repeat("x", maxTraceDBSystraceLineBytes+1) + "\n"), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, sealed := adoptTraceDBPostvalidationFixture(t, test.body)
			coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, test.expectedRows)
			reason, typed := traceDBOutputInvariantReason(err)
			if !typed || reason != test.wantReason || coverage.Error != test.wantReason {
				t.Fatalf("postvalidation reason=%q typed=%t coverage=%+v err=%v, want %q", reason, typed, coverage, err, test.wantReason)
			}
			var witness *TraceClockRegressionWitnessError
			witnessFound := errors.As(err, &witness)
			if test.wantWitness {
				headerLines := strings.Count(systraceHeader, "\n")
				if !witnessFound || witness.PreviousLine != headerLines+1 || witness.CurrentLine != headerLines+2 ||
					witness.PreviousTimestampSec != 0.001 || witness.CurrentTimestampSec != 0.0009 ||
					witness.PreviousEventType != tracequery.EventSchedWakeup || witness.CurrentEventType != tracequery.EventSchedWakeup {
					t.Fatalf("clock regression witness missing or imprecise: found=%t witness=%+v err=%v", witnessFound, witness, err)
				}
				for _, field := range []string{
					"clock_regression_previous_line=",
					"clock_regression_current_line=",
					"clock_regression_previous_event_type=sched_wakeup",
					"clock_regression_current_event_type=sched_wakeup",
				} {
					if !slices.ContainsFunc(coverage.ColumnsPresent, func(value string) bool { return strings.HasPrefix(value, field) }) {
						t.Fatalf("clock regression coverage missing %q: %+v", field, coverage)
					}
				}
			} else if witnessFound {
				t.Fatalf("non-clock failure acquired clock witness: %+v", witness)
			}
			if sealedTraceDBNormalizationFailureIsFatal(err) {
				t.Fatalf("single content failure became a multi-fault sealed DB authority error: %T %v", err, err)
			}
			if coverage.ArtifactPath != "" {
				t.Fatalf("failed postvalidation acquired receipt ArtifactPath: %+v", coverage)
			}
			if tracebundle.IsSystraceReceiptCoverage(
				coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
			) {
				t.Fatalf("failed postvalidation entered the closed receipt selector: %+v", coverage)
			}
			if strings.Contains(err.Error(), target.stagingDir.Path()) || strings.Contains(coverage.Error, target.stagingDir.Path()) {
				t.Fatalf("private staging path leaked through postvalidation: coverage=%+v err=%v", coverage, err)
			}
			if _, statErr := os.Lstat(target.FinalPath); !os.IsNotExist(statErr) {
				t.Fatalf("failed validation published a public output: %v", statErr)
			}
		})
	}
}

func TestTraceDBSystraceHeldPostvalidationPreservesCancellation(t *testing.T) {
	target, sealed := adoptTraceDBPostvalidationFixture(t, []byte(systraceHeader+traceDBPostvalidationKnownLine(t, 1_000_000)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	coverage, err := validateSealedSystraceWithTraceQuery(ctx, sealed, target.FinalPath, 1)
	if !errors.Is(err, context.Canceled) || coverage.Error != traceDBPostvalidationCanceled {
		t.Fatalf("postvalidation cancellation identity drifted: coverage=%+v err=%v", coverage, err)
	}
}

func TestTraceDBSystraceHeldPostvalidationRejectsGenerationDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("same-path mutation while a sealed Windows handle is held is covered by native generation tests")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	mutated := append([]byte(nil), body...)
	mutated[len(mutated)-2] ^= 1
	if err := os.WriteFile(target.StagingPath, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	reason, typed := traceDBOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationGenerationInvalid || coverage.Error != traceDBPostvalidationGenerationInvalid {
		t.Fatalf("generation drift escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}
}

func TestTraceDBSystraceHeldValidationPrecedesExactPublication(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	privateInfo, err := os.Stat(target.StagingPath)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	if err != nil || coverage.RowsEmitted != 1 {
		t.Fatalf("held validation failed: coverage=%+v err=%v", coverage, err)
	}
	if _, err := os.Lstat(target.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("public output existed before exact publication: %v", err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishSealedConversionFileNoReplace(context.Background(), target, sealed, ledger); err != nil {
		t.Fatal(err)
	}
	if len(ledger.created) != 1 || !ledger.created[0].authorityBound || !ledger.created[0].sealed {
		t.Fatalf("publication did not enter exact ledger authority: %+v", ledger.created)
	}
	got, err := os.ReadFile(target.FinalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("public generation was not first-visible complete: bytes=%d err=%v", len(got), err)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		publicInfo, statErr := os.Stat(target.FinalPath)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if publicInfo.Mode().Perm() != privateInfo.Mode().Perm() {
			t.Fatalf("publication permission drifted: private=%#o public=%#o", privateInfo.Mode().Perm(), publicInfo.Mode().Perm())
		}
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.releaseOwnedAuthorities(); err != nil {
		t.Fatal(err)
	}
}

func TestTraceDBSystraceExportRegistersOnlyExactPublication(t *testing.T) {
	dbPath := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	output := filepath.Join(t.TempDir(), "capture.systrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	result, err := exportTraceDBToSystraceWithLedger(context.Background(), dbPath, output, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Path != output || result.OutputBytes <= 0 || result.EventsWritten <= 0 || len(result.TraceCoverage) != 1 {
		t.Fatalf("SQL export result incomplete: %+v", result)
	}
	if len(ledger.created) != 1 || !ledger.created[0].authorityBound || !ledger.created[0].sealed {
		t.Fatalf("SQL export retained a weak/path-only publication: %+v", ledger.created)
	}
	if validation, ok := ledger.ownedTraceValidation(output); !ok || !validation.receipt.queryReady ||
		validation.receipt.kind != ownedTraceValidationSQL || validation.receipt.rows != result.EventsWritten {
		t.Fatalf("SQL export lost its same-generation validation receipt: validation=%+v ok=%t", validation, ok)
	}
	wantRead := result.EventsWritten + strings.Count(systraceHeader, "\n")
	if result.TraceCoverage[0].RowsRead != wantRead || result.TraceCoverage[0].RowsEmitted != result.EventsWritten || result.TraceCoverage[0].Error != "" {
		t.Fatalf("SQL export postvalidation coverage drifted: %+v", result.TraceCoverage)
	}
	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-sql-systrace-") {
			t.Fatalf("private SQL systrace staging survived success: %s", entry.Name())
		}
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("exact ledger rollback did not remove SQL systrace: %v", err)
	}
}
