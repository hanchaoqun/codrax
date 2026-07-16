package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func ownedTraceTestWireDigest(t *testing.T, body []byte) ownedTraceWireDigest {
	t.Helper()
	hasher := newOwnedTraceWireHasher()
	if _, err := hasher.Write(body); err != nil {
		t.Fatal(err)
	}
	result := hasher.finish()
	if !result.Valid || result.Bytes != int64(len(body)) {
		t.Fatalf("wire digest accounting drifted: %+v", result)
	}
	return result
}

func ownedTraceTestRowDigest(line int, text string) ownedTraceRowDigest {
	var builder ownedTraceRowDigestBuilder
	builder.add(line, text)
	return builder.finish()
}

func ownedTraceTestHeaderOnlyLine(t *testing.T) string {
	t.Helper()
	known := strings.TrimSuffix(traceDBPostvalidationKnownLine(t, 1_000_000), "\n")
	cut := strings.Index(known, "sched_wakeup:")
	if cut < 0 {
		t.Fatalf("known fixture lacks event body: %q", known)
	}
	return known[:cut]
}

func TestOwnedTraceValidationBuiltinAndProfilerExceptionalRowsAreExact(t *testing.T) {
	headerLines := strings.Count(systraceHeader, "\n")
	t.Run("builtin fixed header inventory", func(t *testing.T) {
		body := []byte(systraceHeader)
		target, sealed := adoptTraceDBPostvalidationFixture(t, body)
		receipt, coverage, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, ownedTraceValidationProfile{
			Kind: ownedTraceValidationBuiltin, CoverageTable: "builtin_systrace", ExpectedWire: ownedTraceTestWireDigest(t, body), AllowZeroRows: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if receipt.queryReady || receipt.rows != 0 || coverage.RowsRead != headerLines || coverage.RowsEmitted != 0 {
			t.Fatalf("fixed-header inventory receipt drifted: receipt=%+v coverage=%+v", receipt, coverage)
		}
	})

	t.Run("builtin intentional header only", func(t *testing.T) {
		headerOnly := ownedTraceTestHeaderOnlyLine(t)
		body := []byte(systraceHeader + headerOnly + "\n")
		target, sealed := adoptTraceDBPostvalidationFixture(t, body)
		profile := ownedTraceValidationProfile{
			Kind:             ownedTraceValidationBuiltin,
			CoverageTable:    "builtin_systrace",
			ExpectedRows:     1,
			ExpectedUnparsed: ownedTraceTestRowDigest(headerLines+1, headerOnly),
			ExpectedWire:     ownedTraceTestWireDigest(t, body),
			AllowZeroRows:    true,
		}
		receipt, coverage, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, profile)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.queryReady || receipt.known != 0 || receipt.unparsed != 1 || coverage.RowsEmitted != 0 || coverage.RowsRead != headerLines+1 {
			t.Fatalf("builtin inventory receipt drifted: receipt=%+v coverage=%+v", receipt, coverage)
		}

		forged := profile
		forged.ExpectedUnparsed = ownedTraceTestRowDigest(headerLines+1, headerOnly+"forged")
		_, forgedCoverage, forgedErr := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, forged)
		reason, _, typed := ownedTraceOutputInvariantReason(forgedErr)
		if !typed || reason != traceDBPostvalidationUnparsedOwnedRow || forgedCoverage.Error != reason {
			t.Fatalf("forged intentional-unparsed ledger escaped: reason=%q coverage=%+v err=%v", reason, forgedCoverage, forgedErr)
		}

		wireDrift := profile
		wireDrift.ExpectedWire.SHA256[0] ^= 0xff
		_, wireCoverage, wireErr := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, wireDrift)
		reason, _, typed = ownedTraceOutputInvariantReason(wireErr)
		if !typed || reason != traceDBPostvalidationWireMismatch || wireCoverage.Error != reason {
			t.Fatalf("writer/validator wire digest drift escaped: reason=%q coverage=%+v err=%v", reason, wireCoverage, wireErr)
		}
	})

	t.Run("profiler advisory unknown", func(t *testing.T) {
		row, err := prepareTraceDBRenderedRow(1_000_000, 0, "worker", 10, 10, 1, "codrax_unknown_event: value=1")
		if err != nil {
			t.Fatal(err)
		}
		body := []byte(systraceHeader + row.line + "\n")
		target, sealed := adoptTraceDBPostvalidationFixture(t, body)
		profile := ownedTraceValidationProfile{
			Kind:            ownedTraceValidationProfiler,
			CoverageTable:   "profiler_systrace",
			ExpectedRows:    1,
			ExpectedUnknown: ownedTraceTestRowDigest(headerLines+1, row.line),
			ExpectedWire:    ownedTraceTestWireDigest(t, body),
		}
		receipt, coverage, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, profile)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.queryReady || receipt.unknown != 1 || receipt.unparsed != 0 || coverage.RowsEmitted != 1 {
			t.Fatalf("profiler advisory receipt drifted: receipt=%+v coverage=%+v", receipt, coverage)
		}

		structuredClaim := profile
		structuredClaim.ExpectedKnown = 1
		structuredClaim.ExpectedUnknown = ownedTraceRowDigest{}
		_, rejectedCoverage, rejectedErr := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, structuredClaim)
		reason, _, typed := ownedTraceOutputInvariantReason(rejectedErr)
		if !typed || reason != traceDBPostvalidationUnknownOwnedRow || rejectedCoverage.Error != reason {
			t.Fatalf("structured unknown row escaped: reason=%q coverage=%+v err=%v", reason, rejectedCoverage, rejectedErr)
		}
	})
}

func TestOwnedTraceValidationPerfProfileRequiresExactSemanticRow(t *testing.T) {
	bodyText, err := tracewire.BuildPerfSampleBody(tracewire.PerfSampleRow{
		Layout:              tracewire.PerfSampleLayoutBase,
		CPU:                 1,
		CPUKnown:            true,
		PID:                 10,
		TID:                 10,
		ThreadComm:          "worker",
		SampleWeight:        7,
		Event:               "cpu-cycles",
		Symbol:              "DoWork",
		DSO:                 "libwork.so",
		Source:              tracewire.PerfSampleSourceSimpleperfReportSample,
		SymbolizationStatus: tracewire.PerfSymbolizationSymbolized,
		Clock:               tracewire.PerfSampleClockRecord,
		ClockConfidence:     tracewire.PerfClockConfidenceAssumed,
		CallchainStatus:     tracewire.PerfCallchainStatusMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := prepareTraceDBRenderedRow(1_000_000, 0, "worker", 10, 10, 1, bodyText)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(systraceHeader + row.line + "\n")
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	profile := ownedTraceValidationProfile{
		Kind:                 ownedTraceValidationPerf,
		PerfProfile:          ownedTracePerfSimpleperfText,
		ExpectedRows:         1,
		ExpectedKnown:        1,
		ExpectedWire:         ownedTraceTestWireDigest(t, body),
		RequiredEventType:    tracequery.EventPerfSample,
		RequiredPerfSource:   string(tracewire.PerfSampleSourceSimpleperfReportSample),
		RequiredPerfClock:    string(tracewire.PerfSampleClockRecord),
		RequirePerfIntegrity: true,
	}
	receipt, _, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.queryReady || receipt.known != 1 {
		t.Fatalf("valid perf receipt did not become query-ready: %+v", receipt)
	}

	misdeclaredTuple := profile
	misdeclaredTuple.RequiredPerfSource = string(tracewire.PerfSampleSourceHiperfProto)
	_, coverage, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, misdeclaredTuple)
	reason, _, typed := ownedTraceOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationCountMismatch || coverage.Error != reason {
		t.Fatalf("perf profile/source tuple drift escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}

	wrongProfile := profile
	wrongProfile.PerfProfile = ownedTracePerfHiperfProto
	wrongProfile.RequiredPerfSource = string(tracewire.PerfSampleSourceHiperfProto)
	wrongProfile.RequiredPerfClock = string(tracewire.PerfSampleClockMonotonicRaw)
	_, coverage, err = validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, wrongProfile)
	reason, _, typed = ownedTraceOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationEventInvalid || coverage.Error != reason {
		t.Fatalf("perf wire/profile source drift escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}

	nonPerfBody := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 2_000_000))
	nonPerfTarget, nonPerfSealed := adoptTraceDBPostvalidationFixture(t, nonPerfBody)
	nonPerfProfile := profile
	nonPerfProfile.ExpectedWire = ownedTraceTestWireDigest(t, nonPerfBody)
	_, coverage, err = validateOwnedTraceOutput(context.Background(), nonPerfSealed, nonPerfTarget.FinalPath, nonPerfProfile)
	reason, _, typed = ownedTraceOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationEventTypeMismatch || coverage.Error != reason {
		t.Fatalf("known non-perf row escaped perf profile: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}
}

func TestValidatedOwnedTracePublisherBindsReceiptAndRejectsSourceSwap(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	receipt, _, err := validateOwnedTraceOutput(context.Background(), sealed, target.FinalPath, ownedTraceValidationProfile{
		Kind: ownedTraceValidationSQL, CoverageTable: traceDBPostvalidationCoverageTable, ExpectedRows: 1, ExpectedKnown: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.cleanup() })
	if err := publishValidatedOwnedTraceOutputNoReplace(context.Background(), target, sealed, receipt, ledger); err != nil {
		t.Fatal(err)
	}
	published, ok := ledger.ownedTraceValidation(target.FinalPath)
	if !ok || !published.receipt.queryReady || published.receipt.kind != ownedTraceValidationSQL ||
		!published.publishedIdentity.SameVersion(ledger.created[0].sealedIdentity) {
		t.Fatalf("validated public receipt is not bound to the ledger generation: published=%+v ledger=%+v", published, ledger.created)
	}
	got, err := os.ReadFile(target.FinalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("validated publication bytes drifted: got=%d err=%v", len(got), err)
	}

	otherBody := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 2_000_000))
	otherTarget, otherSealed := adoptTraceDBPostvalidationFixture(t, otherBody)
	otherLedger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherLedger.cleanup() })
	err = publishValidatedOwnedTraceOutputNoReplace(context.Background(), otherTarget, otherSealed, receipt, otherLedger)
	if err == nil || !strings.Contains(err.Error(), "does not bind the publication source generation") {
		t.Fatalf("validation-A/publication-B substitution was accepted: %v", err)
	}
	if _, statErr := os.Lstat(otherTarget.FinalPath); !os.IsNotExist(statErr) {
		t.Fatalf("source-swap failure published a public output: %v", statErr)
	}

	forgedTarget, forgedSealed := adoptTraceDBPostvalidationFixture(t, body)
	forgedLedger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = forgedLedger.cleanup() })
	forgedReceipt := receipt
	forgedReceipt.sourceIdentity = forgedSealed.identity
	forgedReceipt.wireSHA256[0] ^= 0xff
	err = publishValidatedOwnedTraceOutputNoReplace(context.Background(), forgedTarget, forgedSealed, forgedReceipt, forgedLedger)
	if err == nil || !strings.Contains(err.Error(), "differs from its held validation receipt") {
		t.Fatalf("public snapshot digest mismatch was accepted: %v", err)
	}
	if len(forgedLedger.created) != 0 {
		t.Fatalf("digest-mismatched publication entered the ledger: %+v", forgedLedger.created)
	}
	if _, statErr := os.Lstat(forgedTarget.FinalPath); !os.IsNotExist(statErr) {
		t.Fatalf("digest-mismatched publication survived rollback: %v", statErr)
	}
}

func TestOwnedTraceValidationSingleMintingAuthorityStructure(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if name != "trace_validation.go" && strings.Contains(text, "ownedTraceValidationReceipt{") {
			t.Fatalf("production source %s can mint an opaque validation receipt", name)
		}
		if name != "trace_validation.go" && strings.Contains(text, ".recordOwnedTraceValidation(") {
			t.Fatalf("production source %s bypasses the validated publication throat", name)
		}
	}

	validator := sourceGenerationFunctionBody(t, "trace_validation.go", "validateOwnedTraceOutput")
	for _, required := range []string{
		"source.Validate()",
		"source.withOpenFile",
		"tracequery.StreamScanHeldFileWithLineObserver",
		"postScanGenerationErr := source.Validate()",
		"receipt = ownedTraceValidationReceipt{",
	} {
		if !strings.Contains(validator, required) {
			t.Fatalf("owned validator lost required authority %q:\n%s", required, validator)
		}
	}
	for _, forbidden := range []string{"tracequery.BuildIndex", "os.Open(", "os.OpenFile("} {
		if strings.Contains(validator, forbidden) {
			t.Fatalf("owned validator regained a path/retaining authority %q:\n%s", forbidden, validator)
		}
	}

	publisher := sourceGenerationFunctionBody(t, "trace_validation.go", "publishValidatedOwnedTraceOutputNoReplace")
	assertSourceGenerationOrder(t, publisher,
		"bindingPath := strings.TrimSpace(target.finalBindingPath)",
		"artifactPath := target.FinalPath",
		"validateOwnedTraceReceiptSource(source, receipt)",
		"publishSealedConversionFileNoReplaceWithValidation(",
		"validatePublishedOwnedTraceReceipt(ctx, publication, receipt)",
		"ledger.recordOwnedTraceValidation(bindingPath, artifactPath, receipt)",
	)
	if !strings.Contains(publisher, "ledger.removeOwnedPath(bindingPath)") {
		t.Fatalf("receipt-binding failure no longer rolls back the public generation:\n%s", publisher)
	}
}

func TestValidatedOwnedTracePublisherFreezesRelativeBindingAcrossCWD(t *testing.T) {
	const helperEnv = "CODRAX_TEST_OWNED_TRACE_RELATIVE_BINDING"
	if os.Getenv(helperEnv) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestValidatedOwnedTracePublisherFreezesRelativeBindingAcrossCWD$")
		command.Env = append(os.Environ(), helperEnv+"=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("relative-binding subprocess: %v\n%s", err, output)
		}
		return
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}

	root := t.TempDir()
	preparedAt := filepath.Join(root, "prepared")
	movedTo := filepath.Join(root, "moved")
	if err := os.Mkdir(preparedAt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(movedTo, 0o700); err != nil {
		t.Fatal(err)
	}
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalCWD) }()
	if err := os.Chdir(preparedAt); err != nil {
		t.Fatal(err)
	}
	target, err := prepareSealedConversionPublicationTarget("relative.systrace", ".codrax-relative-receipt-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Cleanup()
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	if err := os.WriteFile(target.StagingPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	defer sealed.Close()
	receipt, _, err := validateOwnedTraceOutput(context.Background(), sealed, target.finalBindingPath, ownedTraceValidationProfile{
		Kind: ownedTraceValidationSQL, CoverageTable: traceDBPostvalidationCoverageTable, ExpectedRows: 1, ExpectedKnown: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(movedTo); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	if err := publishValidatedOwnedTraceOutputNoReplace(context.Background(), target, sealed, receipt, ledger); err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.ownedTraceValidation(target.finalBindingPath); !ok {
		t.Fatalf("receipt was not attached to frozen binding %q", target.finalBindingPath)
	}
	if got, err := os.ReadFile(filepath.Join(preparedAt, "relative.systrace")); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("relative publication moved with CWD: bytes=%d err=%v", len(got), err)
	}
	if _, err := os.Lstat(filepath.Join(movedTo, "relative.systrace")); !os.IsNotExist(err) {
		t.Fatalf("current CWD received a spurious publication: %v", err)
	}
}
