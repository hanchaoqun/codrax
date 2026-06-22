package hitraceconv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const representativeSysFixtureDir = "testdata/representative_sys_traces"

type representativeSysTraceManifest struct {
	ID             string                         `json:"id"`
	Description    string                         `json:"description"`
	Input          string                         `json:"input"`
	TraceDB        string                         `json:"trace_db,omitempty"`
	Redistribution string                         `json:"redistribution"`
	TraceKind      string                         `json:"trace_kind"`
	Expected       representativeSysTraceExpected `json:"expected"`
}

type representativeSysTraceExpected struct {
	MinEvents     int                              `json:"min_events"`
	BuiltinParity bool                             `json:"builtin_parity"`
	Coverage      []representativeCoverageExpected `json:"coverage"`
	EventTypes    []string                         `json:"event_types"`
}

type representativeCoverageExpected struct {
	Family  string `json:"family"`
	Table   string `json:"table"`
	MinRows int    `json:"min_rows"`
}

func TestRepresentativeSysTraceFixtures(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join(representativeSysFixtureDir, "*.json"))
	if err != nil {
		t.Fatalf("glob representative sys manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Skip("no committed representative .sys fixture manifests; Batch 6C3 remains open until a redistributable fixture is added")
	}
	for _, manifestPath := range manifests {
		manifest := readRepresentativeManifest(t, manifestPath)
		t.Run(manifest.ID, func(t *testing.T) {
			runRepresentativeSysTraceFixture(t, manifest)
		})
	}
}

func readRepresentativeManifest(t *testing.T, path string) representativeSysTraceManifest {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read representative manifest %s: %v", path, err)
	}
	var manifest representativeSysTraceManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("parse representative manifest %s: %v", path, err)
	}
	if strings.TrimSpace(manifest.ID) == "" ||
		strings.TrimSpace(manifest.Input) == "" ||
		strings.TrimSpace(manifest.Redistribution) == "" ||
		strings.TrimSpace(manifest.TraceKind) == "" {
		t.Fatalf("representative manifest must declare id/input/redistribution/trace_kind: %+v", manifest)
	}
	return manifest
}

func runRepresentativeSysTraceFixture(t *testing.T, manifest representativeSysTraceManifest) {
	t.Helper()
	dir := representativeSysFixtureDir
	input := representativeFixturePath(t, dir, manifest.Input)
	sqlOutput := filepath.Join(t.TempDir(), manifest.ID+".sql.systrace")
	sql := convertRepresentativeSysTraceSQL(t, manifest, input, sqlOutput)
	assertRepresentativeTraceResult(t, "sql", sql, sqlOutput, manifest.Expected)
	if manifest.Expected.BuiltinParity {
		if manifest.TraceKind != "no_perf_sys" {
			t.Fatalf("builtin parity is only valid for no_perf_sys fixtures: %+v", manifest)
		}
		builtinOutput := filepath.Join(t.TempDir(), manifest.ID+".builtin.systrace")
		builtin, err := ConvertFile(context.Background(), Options{
			InputPath:   input,
			OutputPath:  builtinOutput,
			TraceEngine: traceEngineBuiltin,
		})
		if err != nil {
			t.Fatalf("convert representative fixture with built-in engine: %v", err)
		}
		if !hasTraceDecision(builtin.TraceDecisions, traceProviderNameBuiltinSys, true) {
			t.Fatalf("built-in representative conversion missing provider provenance: %+v", builtin.TraceDecisions)
		}
		assertRepresentativeTraceResult(t, "builtin", builtin, builtinOutput, manifest.Expected)
	}
}

func convertRepresentativeSysTraceSQL(t *testing.T, manifest representativeSysTraceManifest, input, output string) Result {
	t.Helper()
	opts := Options{
		InputPath:   input,
		OutputPath:  output,
		TraceEngine: traceEngineTraceStreamer,
		KeepTraceDB: true,
	}
	if manifest.TraceDB != "" {
		if runtime.GOOS == "windows" {
			t.Skip("representative fixture trace_db sidecar uses fake trace_streamer shell script")
		}
		dbPath := representativeFixturePath(t, representativeSysFixtureDir, manifest.TraceDB)
		traceStreamer := writeFakeTraceStreamer(t, t.TempDir(), 0)
		t.Setenv("TRACE_STREAMER_FIXTURE_DB", dbPath)
		opts.TraceStreamerPath = traceStreamer
	} else {
		traceStreamer := strings.TrimSpace(os.Getenv("CODRAX_REPRESENTATIVE_TRACE_STREAMER"))
		if traceStreamer == "" {
			t.Fatalf("representative fixture %s has no trace_db sidecar; set CODRAX_REPRESENTATIVE_TRACE_STREAMER for real trace_streamer validation", manifest.ID)
		}
		opts.TraceStreamerPath = traceStreamer
	}
	result, err := ConvertFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("convert representative fixture with SQL engine: %v", err)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, true) || !hasArtifact(result.Artifacts, ArtifactTraceDB) {
		t.Fatalf("SQL representative conversion missing provider provenance: decisions=%+v artifacts=%+v", result.TraceDecisions, result.Artifacts)
	}
	return result
}

func representativeFixturePath(t *testing.T, dir, rel string) string {
	t.Helper()
	if filepath.IsAbs(rel) {
		t.Fatalf("representative fixture path must be relative: %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		t.Fatalf("representative fixture path must stay under %s: %q", dir, rel)
	}
	path := filepath.Join(dir, clean)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("representative fixture path %s is not readable: %v", path, err)
	}
	return path
}

func assertRepresentativeTraceResult(t *testing.T, label string, result Result, output string, expected representativeSysTraceExpected) {
	t.Helper()
	if expected.MinEvents > 0 && result.EventsWritten < expected.MinEvents {
		t.Fatalf("%s representative conversion emitted too few events: result=%+v expected=%+v", label, result, expected)
	}
	if label == "sql" {
		for _, cov := range expected.Coverage {
			if !coverageHasEmitted(result.TraceDBCoverage, cov.Family, cov.Table, cov.MinRows) {
				t.Fatalf("%s representative coverage missing %s/%s >= %d: %+v", label, cov.Family, cov.Table, cov.MinRows, result.TraceDBCoverage)
			}
		}
	}
	if len(expected.EventTypes) == 0 {
		return
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("%s representative trace_query parse failed: %v", label, err)
	}
	counts := map[tracequery.EventType]int{}
	for _, ev := range idx.Events {
		counts[ev.Type]++
	}
	for _, raw := range expected.EventTypes {
		typ := tracequery.EventType(raw)
		if counts[typ] == 0 {
			t.Fatalf("%s representative output missing event type %s: counts=%+v events=%+v", label, raw, counts, idx.Events)
		}
	}
}
