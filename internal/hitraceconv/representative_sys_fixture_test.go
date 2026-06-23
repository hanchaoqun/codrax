package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	InputSHA256    string                         `json:"input_sha256"`
	TraceDB        string                         `json:"trace_db,omitempty"`
	TraceDBSHA256  string                         `json:"trace_db_sha256,omitempty"`
	CaptureClass   string                         `json:"capture_class"`
	Redistribution string                         `json:"redistribution"`
	ApprovalRef    string                         `json:"approval_ref"`
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

const (
	representativeCaptureClassReal = "redistributable_real_capture"

	representativeRedistributionPublic           = "public"
	representativeRedistributionApprovedInternal = "approved_internal"
	representativeRedistributionApprovedCustomer = "approved_customer"
)

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

func TestRepresentativeSysTraceManifestAuthority(t *testing.T) {
	dir := t.TempDir()
	inputBody := []byte("representative sys bytes")
	dbBody := []byte("representative trace db bytes")
	if err := os.WriteFile(filepath.Join(dir, "capture.sys"), inputBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.trace.db"), dbBody, 0o644); err != nil {
		t.Fatal(err)
	}
	base := representativeSysTraceManifest{
		ID:             "vendor-no-perf-001",
		Input:          "capture.sys",
		InputSHA256:    representativeTestSHA256(inputBody),
		TraceDB:        "capture.trace.db",
		TraceDBSHA256:  representativeTestSHA256(dbBody),
		CaptureClass:   representativeCaptureClassReal,
		Redistribution: representativeRedistributionApprovedInternal,
		ApprovalRef:    "internal-approval-123",
		TraceKind:      "no_perf_sys",
		Expected: representativeSysTraceExpected{
			MinEvents:     1,
			BuiltinParity: true,
		},
	}
	if err := validateRepresentativeManifestAuthority(base, dir); err != nil {
		t.Fatalf("valid representative manifest rejected: %v", err)
	}
	tests := []struct {
		name string
		edit func(*representativeSysTraceManifest)
		want string
	}{
		{
			name: "missing capture class",
			edit: func(m *representativeSysTraceManifest) {
				m.CaptureClass = ""
			},
			want: "capture_class",
		},
		{
			name: "synthetic capture class",
			edit: func(m *representativeSysTraceManifest) {
				m.CaptureClass = "synthetic_fixture"
			},
			want: "capture_class",
		},
		{
			name: "unapproved redistribution",
			edit: func(m *representativeSysTraceManifest) {
				m.Redistribution = "private_customer_capture"
			},
			want: "redistribution",
		},
		{
			name: "missing approval ref",
			edit: func(m *representativeSysTraceManifest) {
				m.ApprovalRef = ""
			},
			want: "approval_ref",
		},
		{
			name: "missing input hash",
			edit: func(m *representativeSysTraceManifest) {
				m.InputSHA256 = ""
			},
			want: "input_sha256",
		},
		{
			name: "bad input hash",
			edit: func(m *representativeSysTraceManifest) {
				m.InputSHA256 = strings.Repeat("0", 64)
			},
			want: "input_sha256 mismatch",
		},
		{
			name: "missing trace db hash",
			edit: func(m *representativeSysTraceManifest) {
				m.TraceDBSHA256 = ""
			},
			want: "trace_db_sha256",
		},
		{
			name: "bad trace db hash",
			edit: func(m *representativeSysTraceManifest) {
				m.TraceDBSHA256 = strings.Repeat("1", 64)
			},
			want: "trace_db_sha256 mismatch",
		},
		{
			name: "absolute input path",
			edit: func(m *representativeSysTraceManifest) {
				m.Input = filepath.Join(dir, "capture.sys")
			},
			want: "must be relative",
		},
		{
			name: "escaping input path",
			edit: func(m *representativeSysTraceManifest) {
				m.Input = "../capture.sys"
			},
			want: "must stay under",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := base
			tt.edit(&manifest)
			err := validateRepresentativeManifestAuthority(manifest, dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func representativeTestSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
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
	if err := validateRepresentativeManifestAuthority(manifest, filepath.Dir(path)); err != nil {
		t.Fatalf("representative manifest %s failed authority validation: %v", path, err)
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
	path, err := representativeFixturePathChecked(dir, rel)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func representativeFixturePathChecked(dir, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("representative fixture path must be relative: %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("representative fixture path must stay under %s: %q", dir, rel)
	}
	path := filepath.Join(dir, clean)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("representative fixture path %s is not readable: %w", path, err)
	}
	return path, nil
}

func validateRepresentativeManifestAuthority(manifest representativeSysTraceManifest, dir string) error {
	if strings.TrimSpace(manifest.CaptureClass) != representativeCaptureClassReal {
		return fmt.Errorf("capture_class must be %q for representative retirement evidence", representativeCaptureClassReal)
	}
	switch strings.TrimSpace(manifest.Redistribution) {
	case representativeRedistributionPublic, representativeRedistributionApprovedInternal, representativeRedistributionApprovedCustomer:
	default:
		return fmt.Errorf("redistribution must be one of %q, %q, or %q", representativeRedistributionPublic, representativeRedistributionApprovedInternal, representativeRedistributionApprovedCustomer)
	}
	if strings.TrimSpace(manifest.ApprovalRef) == "" {
		return fmt.Errorf("approval_ref is required for representative retirement evidence")
	}
	if err := verifyRepresentativeFileHash(dir, manifest.Input, manifest.InputSHA256, "input_sha256"); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.TraceDB) != "" {
		if err := verifyRepresentativeFileHash(dir, manifest.TraceDB, manifest.TraceDBSHA256, "trace_db_sha256"); err != nil {
			return err
		}
	}
	return nil
}

func verifyRepresentativeFileHash(dir, rel, want, field string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return fmt.Errorf("%s is required", field)
	}
	path, err := representativeFixturePathChecked(dir, rel)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read representative fixture %s: %w", path, err)
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("%s mismatch for %s: got %s want %s", field, rel, got, want)
	}
	return nil
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
