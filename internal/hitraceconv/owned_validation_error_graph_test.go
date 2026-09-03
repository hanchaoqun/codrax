package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// ownedValidationEventInvalidFixturePath is the checked-in profiler container
// whose ONLY defect is a device-authored text row squatting the reserved
// carrier grammar under a known header. It is the one fixture in the tree
// that trips tracequery_postvalidation_event_invalid through the REAL
// ConvertFile error graph (profiler text lane → owned-output postvalidation);
// cmd/trace_convert_diagnostic_test.go reads the same file so the diagnostic
// report pin walks that graph instead of a hand-built error.
//
// No SQL/builtin/profiler producer can mint such a row from its own inputs
// (bodies are canonically re-rendered), which is why the fixture is a text
// frame: the profiler text lane republishes accepted ftrace text verbatim.
const ownedValidationEventInvalidFixturePath = "testdata/owned_validation/event_invalid_squatter.htrace"

// ownedValidationEventInvalidFixtureRow is the refused row's body; the
// witness must show these producer bytes.
const ownedValidationEventInvalidFixtureRow = "codrax_agent/v2 started"

func ownedValidationEventInvalidFixtureBytes() []byte {
	lines := []string{
		traceDBFormatLine("worker", 40, 40, 2, 5_000_000_000, 0, 0, "sched_wakeup: comm=app pid=20 prio=53 target_cpu=2"),
		traceDBFormatLine("worker", 40, 40, 2, 5_001_000_000, 0, 0, "hmfs_writepage: "+ownedValidationEventInvalidFixtureRow),
	}
	messages := make([][]byte, 0, len(lines))
	for _, line := range lines {
		messages = append(messages, syntheticProfilerPluginData("bytrace_plugin", []byte(line+"\n")))
	}
	return syntheticProfilerTraceFile(messages...)
}

// TestOwnedValidationEventInvalidFixtureIsBuiltByTheRealContainerBuilder binds
// the checked-in fixture bytes to the in-package container builder so the
// cross-package golden cannot drift from the producer it stands for. Moving
// it is deliberate: set CODRAX_UPDATE_OWNED_VALIDATION_FIXTURE=1, rerun, and
// leave an EVOLUTION RECORD next to the constants above.
func TestOwnedValidationEventInvalidFixtureIsBuiltByTheRealContainerBuilder(t *testing.T) {
	want := ownedValidationEventInvalidFixtureBytes()
	if os.Getenv("CODRAX_UPDATE_OWNED_VALIDATION_FIXTURE") == "1" {
		if err := os.MkdirAll(filepath.Dir(ownedValidationEventInvalidFixturePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ownedValidationEventInvalidFixturePath, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(ownedValidationEventInvalidFixturePath)
	if err != nil {
		t.Fatalf("read owned-validation fixture: %v (regenerate deliberately with CODRAX_UPDATE_OWNED_VALIDATION_FIXTURE=1)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("checked-in fixture %s (%d bytes) no longer matches the real profiler container builder (%d bytes); regenerate deliberately with CODRAX_UPDATE_OWNED_VALIDATION_FIXTURE=1 and record the evolution",
			ownedValidationEventInvalidFixturePath, len(got), len(want))
	}
}

func requireEventInvalidFixtureWitness(t *testing.T, err error) *TraceEventInvalidWitnessError {
	t.Helper()
	if err == nil {
		t.Fatal("conversion of the squatter fixture succeeded; the owned-output census did not fire")
	}
	headerLines := strings.Count(systraceHeader, "\n")
	var witness *TraceEventInvalidWitnessError
	if !errors.As(err, &witness) || witness == nil {
		t.Fatalf("real conversion error graph dropped the event_invalid witness: %T %v", err, err)
	}
	if witness.Kind != TraceEventInvalidCarrierSignatureUnderForeignHeader || witness.Line != headerLines+2 ||
		witness.EventName != "hmfs_writepage" || witness.EventType != tracequery.EventFilesystem ||
		witness.BodyPrefix != ownedValidationEventInvalidFixtureRow {
		t.Fatalf("witness does not name the fixture row: %+v", witness)
	}
	if reason, _, ok := ownedTraceOutputInvariantReason(err); !ok || reason != traceDBPostvalidationEventInvalid {
		t.Fatalf("typed reason lost on the way out of ConvertFile: reason=%q ok=%t err=%v", reason, ok, err)
	}
	// The configured tool path is customer-facing; private staging directories
	// (every `.codrax-*` staging pattern) are not.
	if strings.Contains(err.Error(), ".codrax-") {
		t.Fatalf("private staging path leaked through the conversion error: %v", err)
	}
	return witness
}

// TestConvertFileCarriesEventInvalidWitnessThroughTheRealErrorGraph (§40.43
// F-carrier-2 I): the typed row witness minted by the owned-output validator
// must survive every wrapper on the REAL conversion path — the sealed
// publication throat, the profiler lane, ConvertFile's ledger cleanup join,
// and (auto engine) the trace_streamer→builtin fallback error — so that a
// future wrapper without Unwrap turns this pin red instead of silently
// dropping the diagnostic report's typed_error_event_invalid line.
func TestConvertFileCarriesEventInvalidWitnessThroughTheRealErrorGraph(t *testing.T) {
	fixture, err := os.ReadFile(ownedValidationEventInvalidFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("builtin_engine", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "capture.htrace")
		output := filepath.Join(dir, "capture.systrace")
		if err := os.WriteFile(input, fixture, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
		})
		requireEventInvalidFixtureWitness(t, err)
		if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
			t.Fatalf("refused artifact was published: %v", statErr)
		}
	})
	t.Run("auto_engine_after_trace_streamer_failure", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("fake trace_streamer shell fixture uses /bin/sh")
		}
		dir := t.TempDir()
		input := filepath.Join(dir, "capture.htrace")
		output := filepath.Join(dir, "capture.systrace")
		if err := os.WriteFile(input, fixture, 0o600); err != nil {
			t.Fatal(err)
		}
		traceStreamer := writeFakeTraceStreamer(t, dir, 7)
		_, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, TraceStreamerPath: traceStreamer,
		})
		requireEventInvalidFixtureWitness(t, err)
		var fallback *TraceProviderFallbackError
		if !errors.As(err, &fallback) || fallback.Fallback == nil {
			t.Fatalf("auto engine did not wrap the builtin refusal in the fallback error graph: %T %v", err, err)
		}
	})
}

// TestTraceStreamerLaneErrorGraphCarriesRowWitnesses (§40.43 F-carrier-2 I):
// the SQL lane cannot mint an event_invalid or clock-regression row from any
// DB fixture (bodies are canonically rendered, rows are sorted), so its
// wrappers are driven here in production order with the REAL functions:
// validator → traceDBOutputInvariantError → export Cause →
// redactTraceStreamerExportResult (with and without a private path to
// redact) → traceStreamerExportFailureError → traceProviderFallbackFailure.
// Both typed witnesses must remain reachable by errors.As at every step.
func TestTraceStreamerLaneErrorGraphCarriesRowWitnesses(t *testing.T) {
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	squatter, err := prepareTraceDBRenderedRowEnvelope(2_000_000, 1, "worker", 25827, 25827, 1, 4, 2, true,
		"hmfs_writepage: "+ownedValidationEventInvalidFixtureRow)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		body   []byte
		reason string
		check  func(t *testing.T, err error)
	}{
		{
			name:   "event_invalid",
			body:   []byte(systraceHeader + known + squatter.line + "\n"),
			reason: traceDBPostvalidationEventInvalid,
			check: func(t *testing.T, err error) {
				var witness *TraceEventInvalidWitnessError
				if !errors.As(err, &witness) || witness.Kind != TraceEventInvalidCarrierSignatureUnderForeignHeader ||
					witness.EventName != "hmfs_writepage" || witness.BodyPrefix != ownedValidationEventInvalidFixtureRow {
					t.Fatalf("event_invalid witness unreachable: %+v err=%v", witness, err)
				}
			},
		},
		{
			name:   "clock_regression",
			body:   []byte(systraceHeader + known + traceDBPostvalidationKnownLine(t, 900_000)),
			reason: traceDBPostvalidationClockRegression,
			check: func(t *testing.T, err error) {
				var witness *TraceClockRegressionWitnessError
				headerLines := strings.Count(systraceHeader, "\n")
				if !errors.As(err, &witness) || witness.PreviousLine != headerLines+1 || witness.CurrentLine != headerLines+2 {
					t.Fatalf("clock regression witness unreachable: %+v err=%v", witness, err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, sealed := adoptTraceDBPostvalidationFixture(t, tc.body)
			stagingDir := target.stagingDir.Path()
			_, coverage, validationErr := validateSealedSystraceWithTraceQueryReceiptAndWire(
				context.Background(), sealed, target.FinalPath, 2, 0, 0, ownedTraceTestWireDigest(t, tc.body))
			if reason, ok := traceDBOutputInvariantReason(validationErr); !ok || reason != tc.reason || coverage.Error != tc.reason {
				t.Fatalf("validator did not refuse with %q: coverage=%+v err=%v", tc.reason, coverage, validationErr)
			}
			tc.check(t, validationErr)
			for _, cause := range []struct {
				name string
				err  error
			}{
				{name: "bare_validation_error", err: validationErr},
				// A cause whose text carries the private staging path is
				// redacted into traceStreamerPrivateError; the typed graph
				// must survive that wrapper too.
				{name: "path_carrying_cause", err: fmt.Errorf("normalize %s: %w", filepath.Join(stagingDir, "capture.systrace"), validationErr)},
			} {
				t.Run(cause.name, func(t *testing.T) {
					export := traceStreamerExportResult{
						Decision: TraceProviderDecision{
							ProviderName: traceProviderNameTraceStreamer, Attempted: true,
							Reason: "trace_db_normalize_failed", Caveat: "normalize failed: " + cause.err.Error(),
						},
						Ran: true, FailureStage: "trace_db_normalize", FailureCode: "trace_db_normalize_failed",
						Cause: cause.err,
					}
					redactTraceStreamerExportResult(&export, stagingDir)
					lane := traceProviderLanePlan{Source: "test", Path: filepath.Join(stagingDir, "..", "trace_streamer")}
					failure := traceStreamerExportFailureError(export, lane)
					var provider *TraceProviderFailureError
					if !errors.As(failure, &provider) || provider.Cause == nil {
						t.Fatalf("export failure is not the typed provider error: %T %v", failure, failure)
					}
					tc.check(t, failure)
					if strings.Contains(failure.Error(), stagingDir) {
						t.Fatalf("private staging path leaked through the provider error: %v", failure)
					}
					fallback := traceProviderFallbackFailure(
						traceProviderPlan{RequestedEngine: traceEngineAuto, OrderedEngines: []string{traceEngineTraceStreamer, traceEngineBuiltin}},
						export, errors.New("builtin lane failed too"))
					var fallbackErr *TraceProviderFallbackError
					if !errors.As(fallback, &fallbackErr) {
						t.Fatalf("auto fallback did not build the fallback error: %T %v", fallback, fallback)
					}
					tc.check(t, fallback)
				})
			}
		})
	}
}

// TestConversionErrorTypesThatWrapACauseUnwrapIt (§40.43 F-carrier-2 I,
// structural half): every error type in this package that holds a wrapped
// error must expose it through Unwrap, otherwise errors.As in the diagnostic
// report stops at that type and the typed witness lines vanish while the
// hand-reachable pins stay green. The census is positive: it must see the
// known members of the conversion error graph, so an empty scan is red.
func TestConversionErrorTypesThatWrapACauseUnwrapIt(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	type errorType struct {
		file        string
		wrapsError  bool
		isError     bool
		unwrapArity string
	}
	census := map[string]*errorType{}
	entry := func(name string) *errorType {
		if census[name] == nil {
			census[name] = &errorType{}
		}
		return census[name]
	}
	receiverName := func(expr ast.Expr) string {
		switch recv := expr.(type) {
		case *ast.StarExpr:
			if ident, ok := recv.X.(*ast.Ident); ok {
				return ident.Name
			}
		case *ast.Ident:
			return recv.Name
		}
		return ""
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						structType, ok := typeSpec.Type.(*ast.StructType)
						if !ok {
							continue
						}
						for _, field := range structType.Fields.List {
							if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
								info := entry(typeSpec.Name.Name)
								info.file = filepath.Base(path)
								info.wrapsError = true
							}
						}
					}
				case *ast.FuncDecl:
					if d.Recv == nil || len(d.Recv.List) == 0 {
						continue
					}
					name := receiverName(d.Recv.List[0].Type)
					if name == "" {
						continue
					}
					switch d.Name.Name {
					case "Error":
						entry(name).isError = true
					case "Unwrap":
						arity := "invalid"
						if d.Type.Results != nil && len(d.Type.Results.List) == 1 {
							switch result := d.Type.Results.List[0].Type.(type) {
							case *ast.Ident:
								if result.Name == "error" {
									arity = "error"
								}
							case *ast.ArrayType:
								if elem, ok := result.Elt.(*ast.Ident); ok && elem.Name == "error" && result.Len == nil {
									arity = "[]error"
								}
							}
						}
						entry(name).unwrapArity = arity
					}
				}
			}
		}
	}
	var violations, members []string
	for name, info := range census {
		if !info.isError || !info.wrapsError {
			continue
		}
		members = append(members, name)
		if info.unwrapArity != "error" && info.unwrapArity != "[]error" {
			violations = append(violations, fmt.Sprintf("%s (%s) wraps an error but has no Unwrap() error / Unwrap() []error", name, info.file))
		}
	}
	sort.Strings(violations)
	sort.Strings(members)
	if len(violations) != 0 {
		t.Fatalf("conversion error types that would drop typed witnesses from errors.As:\n%s", strings.Join(violations, "\n"))
	}
	for _, required := range []string{
		"ownedTraceOutputInvariantError", "traceDBOutputInvariantError", "traceStreamerPrivateError",
		"TraceProviderFailureError", "TraceProviderFallbackError", "ownedTracePublicationError",
	} {
		if !slices.Contains(members, required) {
			t.Fatalf("census did not see conversion error graph member %s: %v", required, members)
		}
	}
}
