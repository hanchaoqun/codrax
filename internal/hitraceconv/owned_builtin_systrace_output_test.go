package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func builtinWriterTestRow(tsNS uint64, seq int, body string, provenance builtinRowProvenance) renderedRow {
	return renderedRow{
		tsNS:              tsNS,
		seq:               seq,
		line:              traceDBFormatLine("worker", 7, 7, 1, int64(tsNS), 0, 0, body),
		builtinProvenance: provenance,
	}
}

func builtinWriterKnownRow(tsNS uint64, seq int) renderedRow {
	return builtinWriterTestRow(tsNS, seq,
		"sched_wakeup: comm=app pid=7 prio=20 target_cpu=001", builtinRowProvenanceNone)
}

func builtinWriterOpaqueRow(tsNS uint64, seq int, body string) renderedRow {
	return builtinWriterTestRow(tsNS, seq, "print: "+body, builtinRowProvenanceOpaqueMarkerAdvisory)
}

func builtinWriterHeaderOnlyRow(tsNS uint64, seq int) renderedRow {
	return builtinWriterTestRow(tsNS, seq, "", builtinRowProvenanceIntentionalHeaderOnly)
}

func builtinWriterExpectedDigest(rows []renderedRow, match func(renderedRow) bool) ownedTraceRowDigest {
	var builder ownedTraceRowDigestBuilder
	headerLines := strings.Count(systraceHeader, "\n")
	for index, row := range rows {
		if match(row) {
			builder.add(headerLines+index+1, row.line)
		}
	}
	return builder.finish()
}

func TestOwnedBuiltinSystraceRowWriterClosedProfileMatrix(t *testing.T) {
	known := builtinWriterKnownRow(1_000_000, 0)
	headerOnly := builtinWriterHeaderOnlyRow(2_000_000, 1)
	opaqueUnknown := builtinWriterOpaqueRow(3_000_000, 2, "customer opaque payload")
	opaquePluginKnown := builtinWriterOpaqueRow(4_000_000, 3, "[I][XPower] battery stats flushed")
	tests := []struct {
		name                          string
		rows                          []renderedRow
		known, advisory, unknown, raw int
	}{
		{name: "zero rows"},
		{name: "known only", rows: []renderedRow{known}, known: 1},
		{name: "header only", rows: []renderedRow{headerOnly}, raw: 1},
		{name: "opaque unknown only", rows: []renderedRow{opaqueUnknown}, advisory: 1, unknown: 1},
		{name: "opaque plugin known only", rows: []renderedRow{opaquePluginKnown}, known: 1, advisory: 1},
		{name: "mixed authoritative known opaque and header only", rows: []renderedRow{known, opaqueUnknown, headerOnly}, known: 1, advisory: 1, unknown: 1, raw: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wire bytes.Buffer
			profile, err := writeOwnedBuiltinSystraceRows(context.Background(), &wire, test.rows)
			if err != nil {
				t.Fatalf("write rows: %v", err)
			}
			wantWire := systraceHeader
			for _, row := range test.rows {
				wantWire += row.line + "\n"
			}
			if wire.String() != wantWire {
				t.Fatalf("wire bytes differ:\n got %q\nwant %q", wire.String(), wantWire)
			}
			if profile.Kind != ownedTraceValidationBuiltin || profile.CoverageTable != tracebundle.SystraceReceiptTableBuiltin ||
				!profile.AllowZeroRows || profile.ExpectedRows != len(test.rows) || profile.ExpectedKnown != test.known ||
				profile.ExpectedAdvisory.Rows != test.advisory || profile.ExpectedUnknown.Rows != test.unknown ||
				profile.ExpectedUnparsed.Rows != test.raw || profile.ExpectedWire.Valid {
				t.Fatalf("profile drifted: %+v", profile)
			}
			wantAdvisory := builtinWriterExpectedDigest(test.rows, func(row renderedRow) bool {
				return row.builtinProvenance == builtinRowProvenanceOpaqueMarkerAdvisory
			})
			wantUnknown := builtinWriterExpectedDigest(test.rows, func(row renderedRow) bool {
				if row.builtinProvenance != builtinRowProvenanceOpaqueMarkerAdvisory {
					return false
				}
				event, parsed, parseErr := parseOwnedSystraceRow(1, row.line)
				return parseErr == nil && parsed && event.Type == tracequery.EventUnknown
			})
			wantUnparsed := builtinWriterExpectedDigest(test.rows, func(row renderedRow) bool {
				return row.builtinProvenance == builtinRowProvenanceIntentionalHeaderOnly
			})
			if !ownedTraceRowDigestEqual(wantAdvisory, profile.ExpectedAdvisory) ||
				!ownedTraceRowDigestEqual(wantUnknown, profile.ExpectedUnknown) ||
				!ownedTraceRowDigestEqual(wantUnparsed, profile.ExpectedUnparsed) {
				t.Fatalf("exact exceptional-row digests drifted: profile=%+v", profile)
			}
		})
	}
}

func TestOwnedBuiltinSystraceRowWriterDigestsFinalSortedCoordinates(t *testing.T) {
	rows := []renderedRow{
		builtinWriterOpaqueRow(4_000_000, 4, "late opaque"),
		builtinWriterHeaderOnlyRow(3_000_000, 3),
		builtinWriterOpaqueRow(2_000_000, 2, "[I][XPower] plugin context"),
		builtinWriterKnownRow(1_000_000, 1),
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].tsNS == rows[j].tsNS {
			return rows[i].seq < rows[j].seq
		}
		return rows[i].tsNS < rows[j].tsNS
	})
	var wire bytes.Buffer
	profile, err := writeOwnedBuiltinSystraceRows(context.Background(), &wire, rows)
	if err != nil {
		t.Fatal(err)
	}
	wantAdvisory := builtinWriterExpectedDigest(rows, func(row renderedRow) bool {
		return row.builtinProvenance == builtinRowProvenanceOpaqueMarkerAdvisory
	})
	wantUnknown := builtinWriterExpectedDigest(rows, func(row renderedRow) bool {
		return strings.Contains(row.line, "late opaque")
	})
	wantUnparsed := builtinWriterExpectedDigest(rows, func(row renderedRow) bool {
		return row.builtinProvenance == builtinRowProvenanceIntentionalHeaderOnly
	})
	if !ownedTraceRowDigestEqual(wantAdvisory, profile.ExpectedAdvisory) ||
		!ownedTraceRowDigestEqual(wantUnknown, profile.ExpectedUnknown) ||
		!ownedTraceRowDigestEqual(wantUnparsed, profile.ExpectedUnparsed) {
		t.Fatalf("digest did not bind final physical coordinates: %+v", profile)
	}
	body := wire.String()
	last := -1
	for _, token := range []string{"sched_wakeup:", "plugin context", "late opaque"} {
		at := strings.Index(body, token)
		if at < 0 || at <= last {
			t.Fatalf("final wire order drifted at %q: %s", token, body)
		}
		last = at
	}
}

func TestOwnedBuiltinSystraceRowWriterRejectsForgedProvenance(t *testing.T) {
	tests := []struct {
		name   string
		row    renderedRow
		reason string
	}{
		{name: "unmarked opaque unknown", row: builtinWriterTestRow(1, 1, "print: opaque", builtinRowProvenanceNone), reason: traceDBPostvalidationUnknownOwnedRow},
		{name: "forged advisory on known scheduler", row: func() renderedRow {
			row := builtinWriterKnownRow(1, 1)
			row.builtinProvenance = builtinRowProvenanceOpaqueMarkerAdvisory
			return row
		}(), reason: traceDBPostvalidationUnknownOwnedRow},
		{name: "non print unknown advisory", row: builtinWriterTestRow(1, 1, "vendor_unknown: value=1", builtinRowProvenanceOpaqueMarkerAdvisory), reason: traceDBPostvalidationUnknownOwnedRow},
		{name: "trace mark mislabeled advisory", row: builtinWriterTestRow(1, 1, "print: B|7|Frame", builtinRowProvenanceOpaqueMarkerAdvisory), reason: traceDBPostvalidationUnknownOwnedRow},
		{name: "garbage mislabeled header only", row: renderedRow{tsNS: 1, seq: 1, line: "not a trace header", builtinProvenance: builtinRowProvenanceIntentionalHeaderOnly}, reason: traceDBPostvalidationUnparsedOwnedRow},
		{name: "open provenance value", row: builtinWriterTestRow(1, 1, "print: opaque", builtinRowProvenance(255)), reason: traceDBPostvalidationUnparsedOwnedRow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wire bytes.Buffer
			_, err := writeOwnedBuiltinSystraceRows(context.Background(), &wire, []renderedRow{test.row})
			reason, _, typed := ownedTraceOutputInvariantReason(err)
			if !typed || reason != test.reason {
				t.Fatalf("forgery was not a typed hard reject: reason=%q err=%v", reason, err)
			}
		})
	}
}

func TestOwnedBuiltinSystracePublicationReceiptMatrix(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	known := builtinWriterKnownRow(1_000_000, 0)
	headerOnly := builtinWriterHeaderOnlyRow(2_000_000, 1)
	opaqueUnknown := builtinWriterOpaqueRow(3_000_000, 2, "opaque customer payload")
	opaquePluginKnown := builtinWriterOpaqueRow(4_000_000, 3, "[I][XPower] battery stats flushed")
	tests := []struct {
		name                                                string
		rows                                                []renderedRow
		known, authoritative, advisory, unknown, headerOnly int
		ready                                               bool
	}{
		{name: "zero rows"},
		{name: "known only", rows: []renderedRow{known}, known: 1, authoritative: 1, ready: true},
		{name: "header only", rows: []renderedRow{headerOnly}, headerOnly: 1},
		{name: "opaque unknown only", rows: []renderedRow{opaqueUnknown}, advisory: 1, unknown: 1},
		{name: "opaque plugin known only", rows: []renderedRow{opaquePluginKnown}, known: 1, advisory: 1},
		{name: "mixed authoritative known opaque and header", rows: []renderedRow{known, opaqueUnknown, headerOnly}, known: 1, authoritative: 1, advisory: 1, unknown: 1, headerOnly: 1, ready: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "builtin.systrace")
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ledger.cleanup() })
			publication, err := writeValidatedOwnedBuiltinSystraceWithLedger(context.Background(), path, test.rows, ledger)
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			artifact := publication.Artifact
			capability := artifact.Trace
			if artifact.Type != ArtifactSystrace || artifact.Path != path || artifact.Converter != converterVersion || capability == nil ||
				capability.ProviderKind != traceProviderKindBuiltinSys || capability.ProviderName != traceProviderNameBuiltinSys ||
				capability.OutputFormat != ownedSystraceOutputFormat || capability.ValidationProfile != string(ownedTraceValidationBuiltin) ||
				capability.Rows != len(test.rows) || capability.Known != test.known || capability.AuthoritativeKnown != test.authoritative ||
				capability.AdvisoryRows != test.advisory || capability.IntentionalUnknown != test.unknown ||
				capability.IntentionalHeaderOnly != test.headerOnly || capability.TraceQueryReady != test.ready {
				t.Fatalf("receipt capability drifted: artifact=%+v capability=%+v", artifact, capability)
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			sum := sha256.Sum256(body)
			if artifact.Bytes != int64(len(body)) || artifact.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatalf("artifact byte receipt drifted: artifact=%+v bytes=%d", artifact, len(body))
			}
			coverage := publication.TraceCoverage
			headerLines := strings.Count(systraceHeader, "\n")
			if coverage.Family != tracebundle.SystraceReceiptFamily || coverage.Table != tracebundle.SystraceReceiptTableBuiltin ||
				coverage.Role != tracebundle.SystraceReceiptRole || coverage.ArtifactPath != artifact.Path || !coverage.Found || coverage.Error != "" ||
				coverage.RowsRead != headerLines+len(test.rows) || coverage.RowsEmitted != test.known+test.unknown {
				t.Fatalf("receipt coverage drifted: %+v", coverage)
			}
			published, ok := ledger.ownedTraceValidation(artifact.traceReceiptBindingPath)
			if !ok || published.artifactPath != artifact.Path || published.receipt.rows != len(test.rows) ||
				published.receipt.known != test.known || published.receipt.authoritativeKnown != test.authoritative ||
				published.receipt.advisory != test.advisory || published.receipt.unknown != test.unknown ||
				published.receipt.unparsed != test.headerOnly || published.receipt.queryReady != test.ready {
				t.Fatalf("ledger receipt drifted: published=%+v ok=%t", published, ok)
			}
			decision, decisionErr := traceProviderPublished(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys),
					Options{TraceEngine: traceEngineBuiltin}, "input.sys", artifact.Path), artifact, ledger,
			)
			if decisionErr != nil || !decision.Selected || !decision.Attempted || !decision.Succeeded ||
				decision.ArtifactPath != artifact.Path || decision.TraceQueryReady != test.ready {
				t.Fatalf("provider decision/receipt parity drifted: decision=%+v err=%v", decision, decisionErr)
			}
		})
	}
}

func TestOwnedBuiltinSystracePublicationRefusesCollisionAndCleansPrivateState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collision.systrace")
	original := []byte("customer-owned-generation\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.cleanup() })
	_, err = writeValidatedOwnedBuiltinSystraceWithLedger(context.Background(), path, []renderedRow{builtinWriterKnownRow(1, 1)}, ledger)
	var publication *ownedTracePublicationError
	if err == nil || !errors.As(err, &publication) || publication == nil || !ownedTraceOutputHardFailure(err) {
		t.Fatalf("collision was not a typed hard failure: %T %v", err, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("collision overwrote customer generation: got=%q err=%v", got, readErr)
	}
	if len(ledger.created) != 0 {
		t.Fatalf("collision entered publication ledger: %+v", ledger.created)
	}
	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), strings.TrimSuffix(ownedBuiltinSystraceStagingPattern, "*")) {
			t.Fatalf("private builtin staging residue survived: %s", entry.Name())
		}
	}
}

func TestOwnedBuiltinSystracePublicationPreCanceledLeavesNoState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "canceled.systrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.cleanup() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	publication, err := writeValidatedOwnedBuiltinSystraceWithLedger(
		ctx, path, []renderedRow{builtinWriterKnownRow(1, 1)}, ledger,
	)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(publication, builtinSystracePublication{}) {
		t.Fatalf("pre-cancel identity/result drifted: publication=%+v err=%v", publication, err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("pre-canceled writer left a public output: %v", statErr)
	}
	if len(ledger.created) != 0 {
		t.Fatalf("pre-canceled writer entered publication ledger: %+v", ledger.created)
	}
	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	if len(entries) != 0 {
		t.Fatalf("pre-canceled writer left private state: %+v", entries)
	}
}

func TestSharedSorterRejectsBuiltinOnlyProvenanceAndRenderedRowBudgetStaysFlat(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(renderedRow{}) != 88 {
		t.Fatalf("64-bit renderedRow size=%d want=88", unsafe.Sizeof(renderedRow{}))
	}
	for _, row := range []renderedRow{
		builtinWriterOpaqueRow(1, 1, "opaque"),
		builtinWriterHeaderOnlyRow(1, 1),
	} {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		err = sink.add(row)
		_ = sink.cleanup()
		var invariant *traceDBOutputInvariantError
		if !errors.As(err, &invariant) || invariant == nil || invariant.Reason != "builtin_row_provenance_forbidden_in_shared_sorter" {
			t.Fatalf("shared sorter silently compacted builtin provenance: %T %v", err, err)
		}
	}
}

func TestConvertFileBuiltinMixedReceiptResultParity(t *testing.T) {
	printFormat := strings.Join(syntheticFormatBlock("print", 501, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned long", "ip", 8, 8, false),
		syntheticField("char", "buf", 16, 0, false),
	}), "\n")
	opaque := directMarkerCStringFixture("print", []byte("customer opaque payload"), true).content
	formats := strings.Join([]string{syntheticEventFormat(), printFormat, syntheticUnsupportedEventFormat()}, "\n")
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formats))
	writeSegment(&capture, segmentCmdlines, []byte("36379 app\n100 marker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("36379 36379\n100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 10, OffsetNS: 0, Content: syntheticWakeupContent(10)},
		{EventID: 501, OffsetNS: 1_000, Content: opaque},
		{EventID: 20, OffsetNS: 2_000, Content: syntheticWakeupContent(20)},
	}))
	dir := t.TempDir()
	input := filepath.Join(dir, "mixed.sys")
	output := filepath.Join(dir, "mixed.systrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert mixed receipt fixture: %v", err)
	}
	var artifact *Artifact
	for index := range result.Artifacts {
		if result.Artifacts[index].Type == ArtifactSystrace && result.Artifacts[index].Path == output {
			artifact = &result.Artifacts[index]
			break
		}
	}
	if artifact == nil || artifact.Trace == nil {
		t.Fatalf("receipt-backed builtin artifact absent: %+v", result.Artifacts)
	}
	capability := artifact.Trace
	if result.OutputPath != artifact.Path || result.OutputBytes != artifact.Bytes || result.EventsWritten != capability.Rows ||
		result.UnknownEventCount != capability.IntentionalHeaderOnly || capability.Rows != 3 || capability.Known != 1 ||
		capability.AuthoritativeKnown != 1 || capability.AdvisoryRows != 1 || capability.IntentionalUnknown != 1 ||
		capability.IntentionalHeaderOnly != 1 || !capability.TraceQueryReady {
		t.Fatalf("Result/artifact/capability parity drifted: result=%+v artifact=%+v", result, artifact)
	}
	coverageFound := false
	for _, coverage := range result.TraceCoverage {
		if coverage.Table == tracebundle.SystraceReceiptTableBuiltin && coverage.ArtifactPath == artifact.Path {
			coverageFound = coverage.Family == tracebundle.SystraceReceiptFamily && coverage.Role == tracebundle.SystraceReceiptRole &&
				coverage.RowsEmitted == 2 && coverage.Error == ""
		}
	}
	decisionFound := false
	for _, decision := range result.TraceDecisions {
		if decision.ProviderName == traceProviderNameBuiltinSys {
			decisionFound = decision.Succeeded && decision.TraceQueryReady == capability.TraceQueryReady &&
				decision.ArtifactPath == artifact.Path
		}
	}
	if !coverageFound || !decisionFound {
		t.Fatalf("Result lost receipt coverage/decision parity: coverage=%+v decisions=%+v", result.TraceCoverage, result.TraceDecisions)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "header-only") || !strings.Contains(caveats, "opaque advisory") {
		t.Fatalf("exceptional row disclosures missing: %s", caveats)
	}
}
