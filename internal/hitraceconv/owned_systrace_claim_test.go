package hitraceconv

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func publishOwnedSQLSystraceClaimFixture(t *testing.T) (*conversionFileLedger, string, publishedOwnedTraceValidation) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	receipt, _, err := validateSealedSystraceWithTraceQueryReceipt(
		context.Background(), sealed, target.finalBindingPath, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := ledger.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup SQL systrace claim fixture: %v", cleanupErr)
		}
	})
	if err := publishValidatedOwnedTraceOutputNoReplace(context.Background(), target, sealed, receipt, ledger); err != nil {
		t.Fatal(err)
	}
	published, ok := ledger.ownedTraceValidation(target.finalBindingPath)
	if !ok {
		t.Fatal("published SQL systrace receipt is unavailable")
	}
	return ledger, target.finalBindingPath, published
}

func TestClosedSQLSystraceReceiptProjectsExactArtifactAndDecision(t *testing.T) {
	ledger, path, published := publishOwnedSQLSystraceClaimFixture(t)
	caveats := []string{"generated from trace_streamer SQLite DB rows"}
	artifact, err := newValidatedSystraceArtifact(ledger, path, ownedTraceValidationSQL, caveats)
	if err != nil {
		t.Fatal(err)
	}
	wantSHA := hex.EncodeToString(published.receipt.wireSHA256[:])
	if artifact.Type != ArtifactSystrace || artifact.Path != path ||
		artifact.Bytes != published.receipt.size || artifact.SHA256 != wantSHA ||
		artifact.Converter != traceStreamerConverter || !reflect.DeepEqual(artifact.Caveats, caveats) {
		t.Fatalf("SQL artifact did not project the exact receipt/profile: artifact=%+v receipt=%+v", artifact, published.receipt)
	}
	if artifact.Trace == nil {
		t.Fatal("SQL artifact omitted its receipt-derived trace capability")
	}
	wantCapability := TraceArtifactCapability{
		ProviderKind:          traceProviderKindOfficialDB,
		ProviderName:          traceProviderNameTraceStreamer,
		OutputFormat:          ownedSystraceOutputFormat,
		ValidationProfile:     string(ownedTraceValidationSQL),
		Rows:                  published.receipt.rows,
		Known:                 published.receipt.known,
		AuthoritativeKnown:    published.receipt.authoritativeKnown,
		AdvisoryRows:          0,
		IntentionalUnknown:    0,
		IntentionalHeaderOnly: 0,
		TraceQueryReady:       true,
	}
	if *artifact.Trace != wantCapability || wantCapability.Rows != 1 || wantCapability.Known != 1 {
		t.Fatalf("SQL artifact capability did not exactly project the strict-known receipt: got=%+v want=%+v", *artifact.Trace, wantCapability)
	}
	if published.receipt.coverage.ArtifactPath != path ||
		published.receipt.coverage.Family != tracebundle.SystraceReceiptFamily ||
		published.receipt.coverage.Table != tracebundle.SystraceReceiptTableSQL ||
		published.receipt.coverage.Role != tracebundle.SystraceReceiptRole ||
		!tracebundle.IsSystraceReceiptCoverage(
			published.receipt.coverage.Family,
			published.receipt.coverage.Table,
			published.receipt.coverage.Role,
			published.receipt.coverage.ArtifactPath,
		) {
		t.Fatalf("SQL receipt lost its exact public artifact coverage tuple: path=%q coverage=%+v", path, published.receipt.coverage)
	}
	claim, err := validateOwnedSystraceArtifactClaim(ledger, artifact, ownedTraceValidationSQL)
	if err != nil {
		t.Fatal(err)
	}
	if !claim.publishedIdentity.SameVersion(published.publishedIdentity) ||
		claim.receipt.size != published.receipt.size || claim.receipt.wireSHA256 != published.receipt.wireSHA256 {
		t.Fatalf("artifact claim changed its public generation: claim=%+v published=%+v", claim, published)
	}

	base := newTraceProviderDecision(
		traceProviderStageTraceBody,
		traceProviderByName(traceProviderNameTraceStreamer),
		Options{TraceEngine: traceEngineTraceStreamer},
		"capture.sys",
		path,
	)
	if base.Selected || base.Attempted || base.Succeeded || base.TraceQueryReady || base.ArtifactPath != "" {
		t.Fatalf("base trace decision was not fail-closed: %+v", base)
	}
	decision, err := traceProviderPublished(base, artifact, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Selected || !decision.Attempted || !decision.Succeeded || !decision.TraceQueryReady ||
		decision.ArtifactPath != path || decision.OutputPath != path ||
		decision.ProviderKind != traceProviderKindOfficialDB || decision.ProviderName != traceProviderNameTraceStreamer ||
		decision.Reason != "" {
		t.Fatalf("SQL decision did not project the exact receipt/profile: %+v", decision)
	}

	if kind, ok := ownedTraceSystraceKindForProvider(traceProviderNameTraceStreamer); !ok || kind != ownedTraceValidationSQL {
		t.Fatalf("closed SQL provider reverse mapping = (%q,%t)", kind, ok)
	}
	for _, provider := range []string{"", " trace_streamer_db", "trace_streamer", "TRACE_STREAMER_DB"} {
		if kind, ok := ownedTraceSystraceKindForProvider(provider); ok || kind != "" {
			t.Fatalf("non-exact provider %q gained systrace kind (%q,%t)", provider, kind, ok)
		}
	}
}

func TestSQLSystraceClaimsFailHardWithoutExactReceiptProfile(t *testing.T) {
	ledger, path, _ := publishOwnedSQLSystraceClaimFixture(t)
	artifact, err := newValidatedSystraceArtifact(ledger, path, ownedTraceValidationSQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := newTraceProviderDecision(
		traceProviderStageTraceBody,
		traceProviderByName(traceProviderNameTraceStreamer),
		Options{TraceEngine: traceEngineTraceStreamer},
		"capture.sys",
		path,
	)

	emptyLedger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer emptyLedger.cleanup()
	assertHard := func(name string, err error) {
		t.Helper()
		if err == nil || !ownedTraceOutputHardFailure(err) {
			t.Fatalf("%s did not fail at the hard receipt boundary: %T %v", name, err, err)
		}
	}
	_, err = validatedOwnedSystraceClaim(emptyLedger, path, ownedTraceValidationSQL)
	assertHard("missing receipt", err)
	_, err = validatedOwnedSystraceClaim(ledger, path, ownedTraceValidationBuiltin)
	assertHard("wrong receipt kind", err)
	_, err = newValidatedSystraceArtifact(ledger, path, ownedTraceValidationPerf, nil)
	assertHard("non-systrace kind", err)

	mutations := []struct {
		name   string
		mutate func(*Artifact)
	}{
		{"type", func(item *Artifact) { item.Type = ArtifactTraceBundle }},
		{"path", func(item *Artifact) { item.Path += ".other" }},
		{"bytes", func(item *Artifact) { item.Bytes++ }},
		{"sha", func(item *Artifact) { item.SHA256 = strings.Repeat("0", 64) }},
		{"converter", func(item *Artifact) { item.Converter = converterVersion }},
		{"data_type", func(item *Artifact) { item.DataType = 1 }},
		{"plugin_name", func(item *Artifact) { item.PluginName = "forged" }},
		{"plugin_version", func(item *Artifact) { item.PluginVersion = "1" }},
		{"source_offset", func(item *Artifact) { item.SourceOffset = 1 }},
		{"source_bytes", func(item *Artifact) { item.SourceBytes = 1 }},
	}
	for _, test := range mutations {
		t.Run("artifact_"+test.name, func(t *testing.T) {
			forged := artifact
			test.mutate(&forged)
			_, err := validateOwnedSystraceArtifactClaim(ledger, forged, ownedTraceValidationSQL)
			assertHard("forged artifact "+test.name, err)
			_, err = traceProviderPublished(base, forged, ledger)
			assertHard("published forged artifact "+test.name, err)
		})
	}

	capabilityMutations := []struct {
		name   string
		mutate func(*TraceArtifactCapability)
	}{
		{"provider_kind", func(item *TraceArtifactCapability) { item.ProviderKind = traceProviderKindBuiltin }},
		{"provider_name", func(item *TraceArtifactCapability) { item.ProviderName = traceProviderNameBuiltinModern }},
		{"output_format", func(item *TraceArtifactCapability) { item.OutputFormat = ArtifactTraceDB }},
		{"validation_profile", func(item *TraceArtifactCapability) { item.ValidationProfile = string(ownedTraceValidationBuiltin) }},
		{"rows", func(item *TraceArtifactCapability) { item.Rows++ }},
		{"known", func(item *TraceArtifactCapability) { item.Known++ }},
		{"authoritative_known", func(item *TraceArtifactCapability) { item.AuthoritativeKnown++ }},
		{"advisory_rows", func(item *TraceArtifactCapability) { item.AdvisoryRows++ }},
		{"intentional_unknown", func(item *TraceArtifactCapability) { item.IntentionalUnknown++ }},
		{"intentional_header_only", func(item *TraceArtifactCapability) { item.IntentionalHeaderOnly++ }},
		{"trace_query_ready", func(item *TraceArtifactCapability) { item.TraceQueryReady = false }},
	}
	for _, test := range capabilityMutations {
		t.Run("capability_"+test.name, func(t *testing.T) {
			forged := artifact
			capability := *artifact.Trace
			test.mutate(&capability)
			forged.Trace = &capability
			_, err := validateOwnedSystraceArtifactClaim(ledger, forged, ownedTraceValidationSQL)
			assertHard("forged capability "+test.name, err)
			_, err = traceProviderPublished(base, forged, ledger)
			assertHard("published forged capability "+test.name, err)
		})
	}

	missingCapability := artifact
	missingCapability.Trace = nil
	_, err = validateOwnedSystraceArtifactClaim(ledger, missingCapability, ownedTraceValidationSQL)
	assertHard("missing trace capability", err)
	_, err = traceProviderPublished(base, missingCapability, ledger)
	assertHard("published missing trace capability", err)
	wrongLane := artifact
	wrongLane.Perf = &PerfArtifactCapability{}
	_, err = validateOwnedSystraceArtifactClaim(ledger, wrongLane, ownedTraceValidationSQL)
	assertHard("systrace carrying perf capability", err)

	for _, test := range []struct {
		name   string
		mutate func(*TraceProviderDecision)
	}{
		{"provider_name", func(item *TraceProviderDecision) { item.ProviderName = traceProviderNameBuiltinSys }},
		{"provider_kind", func(item *TraceProviderDecision) { item.ProviderKind = traceProviderKindBuiltin }},
		{"output_path", func(item *TraceProviderDecision) { item.OutputPath += ".other" }},
	} {
		t.Run("decision_"+test.name, func(t *testing.T) {
			forged := base
			test.mutate(&forged)
			_, err := traceProviderPublished(forged, artifact, ledger)
			assertHard("forged decision "+test.name, err)
		})
	}

	// A type/path-only Artifact is descriptive metadata, not a capability.
	// It must not acquire bytes, digest, profile or readiness from its own
	// assertion; only the receipt factory may construct the public claim.
	typeOnly := Artifact{Type: ArtifactSystrace, Path: path}
	_, err = validateOwnedSystraceArtifactClaim(ledger, typeOnly, ownedTraceValidationSQL)
	assertHard("type-only artifact", err)
	_, err = traceProviderPublished(base, typeOnly, ledger)
	assertHard("type-only publication", err)

	if len(ledger.created) != 1 || ledger.created[0].traceValidation == nil {
		t.Fatalf("SQL claim fixture lost its sole ledger receipt: %+v", ledger.created)
	}
	originalCoverage := ledger.created[0].traceValidation.receipt.coverage
	for _, test := range []struct {
		name   string
		mutate func(*TraceDBCoverage)
	}{
		{name: "padded_path", mutate: func(item *TraceDBCoverage) { item.ArtifactPath = " " + path }},
		{name: "wrong_path", mutate: func(item *TraceDBCoverage) { item.ArtifactPath = path + ".other" }},
		{name: "wrong_family", mutate: func(item *TraceDBCoverage) { item.Family = "trace_db" }},
		{name: "wrong_role", mutate: func(item *TraceDBCoverage) { item.Role = "database_inventory" }},
		{name: "future_table", mutate: func(item *TraceDBCoverage) { item.Table = "tracequery_build_index_v2" }},
		{name: "builtin_table", mutate: func(item *TraceDBCoverage) { item.Table = tracebundle.SystraceReceiptTableBuiltin }},
	} {
		t.Run("receipt_coverage_"+test.name, func(t *testing.T) {
			coverage := originalCoverage
			test.mutate(&coverage)
			ledger.created[0].traceValidation.receipt.coverage = coverage
			defer func() { ledger.created[0].traceValidation.receipt.coverage = originalCoverage }()
			_, err := validatedOwnedSystraceClaim(ledger, path, ownedTraceValidationSQL)
			assertHard("forged receipt coverage "+test.name, err)
		})
	}
}

func TestClosedSQLSystraceClaimKeepsFrozenBindingAcrossCWD(t *testing.T) {
	const helper = "CODRAX_SQL_RECEIPT_CWD_HELPER"
	if os.Getenv(helper) == "1" {
		preparedAt := os.Getenv("CODRAX_SQL_RECEIPT_PREPARED_AT")
		movedTo := os.Getenv("CODRAX_SQL_RECEIPT_MOVED_TO")
		if err := os.Chdir(preparedAt); err != nil {
			t.Fatal(err)
		}
		target, err := prepareSealedConversionPublicationTarget("relative.systrace", ".codrax-relative-sql-claim-*")
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
		receipt, _, err := validateSealedSystraceWithTraceQueryReceipt(
			context.Background(), sealed, target.finalBindingPath, 1,
		)
		if err != nil {
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
		if err := os.Chdir(movedTo); err != nil {
			t.Fatal(err)
		}
		artifact, err := newValidatedSystraceArtifact(
			ledger, target.finalBindingPath, ownedTraceValidationSQL, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Path != "relative.systrace" || artifact.traceReceiptBindingPath != target.finalBindingPath ||
			artifact.traceReceiptArtifactPath != artifact.Path || !filepath.IsAbs(artifact.traceReceiptBindingPath) {
			t.Fatalf("frozen/display path split drifted: %+v binding=%q display=%q", artifact, artifact.traceReceiptBindingPath, artifact.traceReceiptArtifactPath)
		}
		if _, err := validateOwnedSystraceArtifactClaim(ledger, artifact, ownedTraceValidationSQL); err != nil {
			t.Fatal(err)
		}
		return
	}
	preparedAt := t.TempDir()
	movedTo := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestClosedSQLSystraceClaimKeepsFrozenBindingAcrossCWD$")
	cmd.Env = append(os.Environ(),
		helper+"=1",
		"CODRAX_SQL_RECEIPT_PREPARED_AT="+preparedAt,
		"CODRAX_SQL_RECEIPT_MOVED_TO="+movedTo,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("relative systrace receipt subprocess: %v\n%s", err, output)
	}
}

func TestReceiptBackedSystraceDedupeKeyPreservesWholeClaim(t *testing.T) {
	ledger, path, _ := publishOwnedSQLSystraceClaimFixture(t)
	artifact, err := newValidatedSystraceArtifact(ledger, path, ownedTraceValidationSQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if artifactDedupeKey(artifact) != artifactDedupeKey(artifact) {
		t.Fatal("identical receipt artifact has an unstable dedupe key")
	}
	inventory := Artifact{Type: ArtifactSystrace, Path: artifact.Path, Converter: artifact.Converter}
	if artifactDedupeKey(artifact) == artifactDedupeKey(inventory) {
		t.Fatal("receipt-backed systrace coalesced with type-only inventory")
	}
	forged := artifact
	forged.Bytes++
	if artifactDedupeKey(artifact) == artifactDedupeKey(forged) {
		t.Fatal("receipt byte drift was hidden by artifact dedupe")
	}
	wrongLane := artifact
	wrongLane.Perf = &PerfArtifactCapability{}
	if artifactDedupeKey(artifact) == artifactDedupeKey(wrongLane) {
		t.Fatal("receipt-backed systrace coalesced with a forged perf capability")
	}
}

func TestSQLSystraceProductionConsumesReceiptFactoriesOnly(t *testing.T) {
	exporter := sourceGenerationFunctionBody(t, "streamerdb_export.go", "exportTraceDBToSystraceFromOpenWithLedger")
	for _, required := range []string{
		"newValidatedSystraceArtifact(",
		"ownedTraceValidationSQL",
	} {
		if !strings.Contains(exporter, required) {
			t.Fatalf("SQL exporter lost receipt projection %q:\n%s", required, exporter)
		}
	}
	for _, forbidden := range []string{
		"Artifact{",
		"ArtifactSystrace",
		"traceProviderSuccess(",
	} {
		if strings.Contains(exporter, forbidden) {
			t.Fatalf("SQL exporter regained direct systrace mint %q:\n%s", forbidden, exporter)
		}
	}
	factoryAt := strings.Index(exporter, "newValidatedSystraceArtifact(")
	if factoryAt < 0 {
		t.Fatal("SQL exporter has no receipt Artifact factory")
	}
	factoryCall := exporter[factoryAt:]
	assertSourceGenerationOrder(t, factoryCall,
		"ledger,",
		"target.finalBindingPath,",
		"ownedTraceValidationSQL,",
	)

	provider := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "runTraceStreamerExport")
	if !strings.Contains(provider, "traceProviderPublished(") {
		t.Fatalf("SQL provider does not consume the published receipt:\n%s", provider)
	}
	if strings.Contains(provider, "traceProviderSuccess(") {
		t.Fatalf("SQL provider regained Artifact-type readiness mint:\n%s", provider)
	}
}

func TestOwnedSystracePublicationFailureIsFatalToTraceStreamerFallback(t *testing.T) {
	err := newOwnedTracePublicationError("consume_public_receipt", "out.systrace", errors.New("receipt drift"))
	if !sealedTraceDBNormalizationFailureIsFatal(err) {
		t.Fatalf("owned systrace publication failure was demoted to provider fallback: %T %v", err, err)
	}
	provider := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "runTraceStreamerExport")
	claimAt := strings.Index(provider, "success, err := traceProviderPublished(")
	if claimAt < 0 || !strings.Contains(provider[claimAt:], "return traceStreamerExportResult{}, err") {
		t.Fatalf("SQL receipt projection failure no longer returns before provider success:\n%s", provider)
	}
	auto := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "maybeRunTraceStreamerAuto")
	if !strings.Contains(auto, "return runTraceStreamerExport(") {
		t.Fatalf("auto provider no longer propagates SQL hard errors directly:\n%s", auto)
	}
}

func TestLegacySystraceTypeReadinessMintIsAbsentAndInventoryFailsClosed(t *testing.T) {
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
		if strings.Contains(string(body), "traceProviderSuccess(") {
			t.Fatalf("production file %s regained the legacy Artifact-type readiness mint", name)
		}
		if strings.Contains(string(body), "TraceQueryReady: true") ||
			strings.Contains(string(body), "TraceQueryReady = true") {
			t.Fatalf("production file %s regained a static trace-query readiness mint", name)
		}
		if name != "owned_systrace_claim.go" && strings.Contains(string(body), "TraceArtifactCapability{") {
			t.Fatalf("production file %s regained a second trace capability constructor", name)
		}
	}

	inventory := sourceGenerationFunctionBody(t, "trace_provider.go", "traceProviderInventoryPublished")
	if !strings.Contains(inventory, "decision.TraceQueryReady = false") {
		t.Fatalf("temporary inventory publisher no longer fails closed:\n%s", inventory)
	}
	for _, forbidden := range []string{
		"decision.TraceQueryReady = true",
		"artifact.Type == ArtifactSystrace",
		"artifact.Type != ArtifactSystrace",
	} {
		if strings.Contains(inventory, forbidden) {
			t.Fatalf("temporary inventory publisher regained readiness inference %q:\n%s", forbidden, inventory)
		}
	}

	// Enumerate every production TraceQueryReady write by AST. Static false is
	// always fail-closed; the only positive RHS values are exact ledger receipt
	// selectors in the dedicated perf/systrace claim consumers.
	renderExpr := func(fset *token.FileSet, expr ast.Expr) string {
		var buffer bytes.Buffer
		if err := printer.Fprint(&buffer, fset, expr); err != nil {
			t.Fatalf("render TraceQueryReady expression: %v", err)
		}
		return buffer.String()
	}
	allowedReceiptRHS := func(fileName, rhs string) bool {
		switch rhs {
		case "false":
			return true
		case "receipt.queryReady":
			return fileName == "owned_systrace_claim.go"
		case "published.receipt.queryReady":
			return fileName == "owned_systrace_claim.go" ||
				fileName == "owned_perftrace_claim.go" || fileName == "perf_provider.go"
		default:
			return false
		}
	}
	positiveWrites := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				for index, left := range typed.Lhs {
					selector, ok := left.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "TraceQueryReady" || index >= len(typed.Rhs) {
						continue
					}
					rhs := renderExpr(fset, typed.Rhs[index])
					if !allowedReceiptRHS(name, rhs) {
						t.Fatalf("production file %s writes TraceQueryReady from unproved RHS %q", name, rhs)
					}
					if rhs != "false" {
						positiveWrites[name+"|"+rhs]++
					}
				}
			case *ast.KeyValueExpr:
				key, ok := typed.Key.(*ast.Ident)
				if !ok || key.Name != "TraceQueryReady" {
					return true
				}
				rhs := renderExpr(fset, typed.Value)
				if !allowedReceiptRHS(name, rhs) {
					t.Fatalf("production file %s literals TraceQueryReady from unproved RHS %q", name, rhs)
				}
				if rhs != "false" {
					positiveWrites[name+"|"+rhs]++
				}
			}
			return true
		})
	}
	wantPositiveWrites := map[string]int{
		"owned_systrace_claim.go|receipt.queryReady":            1,
		"owned_systrace_claim.go|published.receipt.queryReady":  1,
		"owned_perftrace_claim.go|published.receipt.queryReady": 2,
		"perf_provider.go|published.receipt.queryReady":         1,
	}
	if !reflect.DeepEqual(positiveWrites, wantPositiveWrites) {
		t.Fatalf("receipt-derived readiness write set drifted: got=%v want=%v", positiveWrites, wantPositiveWrites)
	}
}
