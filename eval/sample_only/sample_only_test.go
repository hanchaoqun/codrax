package sample_only

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const sourceSHA = "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632"
const windowStart, windowEnd = 486489.4, 486489.6

// Explicit opt-in is only for reading the local representative capture.
// This test never calls a model: its CLI receives only /exit and every model
// client points at a local refusing endpoint that must receive zero requests.
func TestReferenceSampleOnlyBinaryOracleAndCLI239(t *testing.T) {
	source := os.Getenv("CODRAX_SAMPLE_ONLY_INPUT")
	if source == "" {
		t.Skip("set CODRAX_SAMPLE_ONLY_INPUT to the pinned test_brbe.data original")
	}
	if testing.Short() {
		t.Skip("builds actual CLI once; skipped under -short")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(source)
	if err != nil || !before.Mode().IsRegular() || before.Size() != 30062585 || digest(t, source) != sourceSHA {
		t.Fatalf("fixture must be the complete pinned 30062585-byte reference capture: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		after, err := os.Stat(source)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || digest(t, source) != sourceSHA {
			t.Errorf("original capture changed during offline preparation: %v", err)
		}
		afterEntries, err := os.ReadDir(filepath.Dir(source))
		if err != nil || !reflect.DeepEqual(entryNames(entries), entryNames(afterEntries)) {
			t.Errorf("preparation wrote beside original: %v", err)
		}
	})
	out := os.Getenv("CODRAX_SAMPLE_ONLY_ORACLE_DIR")
	if out == "" {
		out = filepath.Join(t.TempDir(), "oracle")
	}
	if !filepath.IsAbs(out) {
		t.Fatal("oracle output must be absolute")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatalf("refuse existing oracle output or missing parent: %v", err)
	}
	out, err = filepath.EvalSymlinks(out)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	material, err := traceinput.Prepare(ctx, traceinput.Options{InputPath: source, RuntimeAnchor: filepath.Join(out, "operator-only"), PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(ctx, material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 12000 || material.SourcePath() != source || material.QueryPath() == source || material.SelfContainedText() {
		t.Fatalf("full binary preparation/source separation lost: events=%d", len(idx.Events))
	}
	counts := map[tracequery.EventType]int{}
	for _, event := range idx.Events {
		counts[event.Type]++
	}
	if len(counts) != 1 || counts[tracequery.EventPerfSample] != 12000 {
		t.Fatalf("sample-only capture gained non-sample events: %+v", counts)
	}
	var preparation struct {
		SourcePath   string             `json:"source_path"`
		SourceSHA256 string             `json:"source_sha256"`
		QueryPath    string             `json:"query_path"`
		Conversion   hitraceconv.Result `json:"conversion"`
	}
	preparationPath := filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json")
	readJSON(t, preparationPath, &preparation)
	if preparation.SourcePath != source || preparation.SourceSHA256 != sourceSHA || preparation.QueryPath != material.QueryPath() || hitraceconv.QueryReadySystracePath(preparation.Conversion) != "" || hitraceconv.QueryReadyPerfTracePath(preparation.Conversion.Artifacts) == "" {
		t.Fatal("sample-only provenance lost or gained scheduler query capability")
	}
	full := tracequery.Run(idx, tracequery.Query{View: "perf_stats", Limit: 20})
	selected := tracequery.Run(idx, tracequery.Query{View: "perf_stats", TimeStart: windowStart, TimeEnd: windowEnd, Limit: 20})
	if full.PerfStats == nil || full.PerfStats.SampleCount != 12000 || selected.PerfStats == nil || selected.PerfStats.SampleCount != 1393 {
		t.Fatalf("native sample count differs from independent reference-parser oracle: full=%+v selected=%+v", full.PerfStats, selected.PerfStats)
	}
	target := tracequery.Run(idx, tracequery.Query{View: "perf_stats", TimeStart: windowStart, TimeEnd: windowEnd, PID: 1609, Limit: 20})
	if target.PerfStats == nil || target.PerfStats.SampleCount != 164 || target.PerfStats.TotalPeriod != 259 || target.PerfStats.ThreadIdentityCount != 1 || target.PerfStats.ThreadIdentityCountExact == nil || !*target.PerfStats.ThreadIdentityCountExact || target.PerfStats.ThreadIdentityUnknownSampleCount != 0 {
		t.Fatalf("target oracle lost exact sample identity/independent weight: %+v", target.PerfStats)
	}
	if len(target.PerfStats.TopSymbols) != 2 || target.PerfStats.TopSymbols[0].Symbol != "0xffffff800b829d24" || target.PerfStats.TopSymbols[0].Period != 174 || target.PerfStats.TopSymbols[1].Period != 85 || !strings.Contains(target.PerfStats.TopSymbols[1].Symbol, "SensorCallbackProxy::OnDataEvent_") {
		t.Fatal("target original IP and symbol/weight oracle changed")
	}
	for name, result := range map[string]tracequery.Result{"full": full, "window": selected, "target": target} {
		if result.WindowStats == nil || len(result.WindowStats.TopRunning) != 0 || len(result.WindowStats.RunnableTop) != 0 || result.WindowStats.CPUOccupancy != nil {
			t.Fatalf("%s acquired scheduler occupancy from samples", name)
		}
		writeJSON(t, filepath.Join(out, name+".json"), result)
	}
	writeJSON(t, filepath.Join(out, "oracle.json"), map[string]any{
		"kind": "reference_real_linux_perf_data_sample_transport_only", "source": source,
		"source_bytes": before.Size(), "source_sha256": sourceSHA, "full_samples": 12000,
		"full_start": idx.FirstTs, "full_end": idx.LastTs, "window_start": windowStart, "window_end": windowEnd,
		"window_samples": 1393, "events": counts, "query_path": material.QueryPath(),
		"target_tid": 1609, "target_comm": "poll_report", "target_samples": 164, "target_weight": 259, "target_weight_unit": "cycles",
		"preparation": preparationPath, "brbe_semantics_validated": false, "real_model_runs": 0,
		"independent_reference_parser": "core/extensions/hiperf_binary_parser.py::parse_samples; complete raw PERF_RECORD_SAMPLE inventory",
	})
	t.Logf("full=[%.9f,%.9f] samples=%d; window samples=%d identities=%d; target tid=1609 samples=%d period=%d cohorts=%d; no scheduler/BRBE authority", idx.FirstTs, idx.LastTs, full.PerfStats.SampleCount, selected.PerfStats.SampleCount, selected.PerfStats.ThreadIdentityCount, target.PerfStats.SampleCount, target.PerfStats.TotalPeriod, target.PerfStats.CohortCount)
	checkActualCLIWithoutModel(t, source, out)
}

func checkActualCLIWithoutModel(t *testing.T, source, out string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(out, "codrax-preflight")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, ".")
	build.Dir = root
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("actual CLI build failed: %v\n%s", err, data)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "no model permitted in attachment preflight", http.StatusForbidden)
	}))
	defer server.Close()
	providers := filepath.Join(out, "no-model-providers.yaml")
	providerData := fmt.Sprintf("llm:\n  default:\n    provider: openai\n    model: no-model\n    api_key: non-secret-preflight\n    base_url: %q\n", server.URL)
	writeBytes(t, providers, []byte(providerData))
	settings := filepath.Join(out, "no-model-settings.yaml")
	writeBytes(t, settings, []byte("write_enabled: false\nenv_recommend_enabled: false\nread_run_auto_resume: false\nmarkdown_preview_server: off\ntrace_attach_max_bytes: 1024\n"))
	workspace := filepath.Join(out, "cli-workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(out, "cli-logs")
	// The ordinary run.sh read argv and binary attachment path, except its
	// --request is absent so /exit tests startup without an analysis call.
	args := []string{"--providers", providers, "--repo", workspace, "--branch", "main", "--pipeline-max-steps", "15", "--log-level", "debug", "--log-dir", logs, "--htrace", source}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = workspace
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "CODRAX_SETTINGS=") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "CODRAX_SETTINGS="+settings, "NO_COLOR=1")
	command.Stdin = strings.NewReader("/exit\n")
	output, err := command.CombinedOutput()
	writeBytes(t, filepath.Join(out, "cli-preflight.out"), output)
	writeJSON(t, filepath.Join(out, "cli-preflight.json"), map[string]any{"argv": args, "stdin": "/exit\n", "provider_requests": requests.Load(), "real_model_runs": 0, "startup_error": fmt.Sprint(err)})
	if err != nil || !strings.Contains(string(output), "Goodbye!") || requests.Load() != 0 {
		t.Fatalf("actual binary attachment CLI preflight failed or called model: %v calls=%d\n%s", err, requests.Load(), output)
	}
	var childPreparation string
	if err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && entry.Name() == "preparation.json" {
			var receipt struct {
				SourcePath   string             `json:"source_path"`
				SourceSHA256 string             `json:"source_sha256"`
				QueryPath    string             `json:"query_path"`
				Conversion   hitraceconv.Result `json:"conversion"`
			}
			readJSON(t, path, &receipt)
			if receipt.SourcePath == source && receipt.SourceSHA256 == sourceSHA && hitraceconv.QueryReadySystracePath(receipt.Conversion) == "" && hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts) != "" {
				idx, err := tracequery.BuildIndex(context.Background(), receipt.QueryPath)
				if err != nil {
					return err
				}
				if len(idx.Events) != 12000 {
					return fmt.Errorf("child preparation retained %d/12000 samples", len(idx.Events))
				}
				childPreparation = path
			}
		}
		return nil
	}); err != nil || childPreparation == "" {
		t.Fatalf("actual CLI did not independently prepare complete original sample-only material: %v", err)
	}
	writeJSON(t, filepath.Join(out, "cli-preflight.json"), map[string]any{"argv": args, "stdin": "/exit\n", "provider_requests": requests.Load(), "real_model_runs": 0, "preparation": childPreparation, "native_child_samples": 12000, "model_query_unexercised": true, "answer_unexercised": true})
}

func digest(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	writeBytes(t, path, []byte(mustJSON(t, value)))
}
func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func writeBytes(t *testing.T, path string, value []byte) {
	t.Helper()
	if err := os.WriteFile(path, value, 0600); err != nil {
		t.Fatal(err)
	}
}
func readJSON(t *testing.T, path string, value any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		t.Fatal(err)
	}
}
