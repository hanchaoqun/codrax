package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const transportNegativeHelperEnv = "CODRAX_REPL_TAIL_TRANSPORT_NEGATIVE_HELPER"

func TestMain(m *testing.M) {
	// Cross-platform transport negative. Intercept before m.Run parses the
	// actual CLI arguments; this helper never constructs a model client.
	if os.Getenv(transportNegativeHelperEnv) == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		fmt.Println("tail-target 424242 1.011")
		os.Exit(37)
	}
	os.Exit(m.Run())
}

func TestConstructedRMQOracleUsesCompleteProductionPreparation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "fixture-only")
	if err := run(context.Background(), options{out: out, prepareOnly: true, timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	var oracle fixtureOracle
	if !readJSON(filepath.Join(out, "oracle.json"), &oracle) || oracle.PreviewLimit != 1024 || oracle.TailInPreview || oracle.FullEvents != 12 || oracle.ExpectedTime != 1.011 || oracle.ExpectedPID != 424242 {
		t.Fatalf("unexpected deterministic oracle: %+v", oracle)
	}
	if oracle.InputSHA256 != bytesSHA(rmqFixture()) {
		t.Fatal("RMQ bytes differ from frozen generator")
	}
	var receipt map[string]any
	if !readJSON(filepath.Join(out, "receipt.json"), &receipt) || receipt["status"] != "fixture_validated_only" || receipt["real_model_runs"] != float64(0) {
		t.Fatalf("prepare-only forged a live success: %+v", receipt)
	}
	// A complete preflight result outside workspace must not be accepted as
	// evidence that the real child prepared, queried, or answered anything.
	r := auditRun(out, oracle)
	r.Checks["cli_exit_zero"] = true
	r.finish()
	if r.Status != "machine_checks_failed" || r.Checks["native_query_contains_exact_tail"] || r.Checks["child_preparation_bound_to_original"] {
		t.Fatalf("oracle leaked into model-evidence verdict: %+v", r)
	}
}

func TestExplicitOptInAndFreshDirectoryRequired(t *testing.T) {
	parent := t.TempDir()
	for _, opts := range []options{
		{out: filepath.Join(parent, "not-opted-in"), timeout: time.Minute},
		{out: filepath.Join(parent, "both"), live: true, prepareOnly: true, timeout: time.Minute},
		{out: "relative", prepareOnly: true, timeout: time.Minute},
		{out: parent, prepareOnly: true, timeout: time.Minute},
	} {
		if err := run(context.Background(), opts); err == nil {
			t.Fatalf("unsafe options admitted: %+v", opts)
		}
	}
}

func TestChildCLIArgumentsCannotSkipREPLOrAttachPreparedAnswers(t *testing.T) {
	args := childArgs("/workspace", "/logs")
	for _, arg := range args {
		if arg == "--request" || arg == "-r" || strings.HasPrefix(arg, "--htrace") || strings.HasPrefix(arg, "--atrace") {
			t.Fatalf("single-shot or pre-attached shortcut in REPL invocation: %q", args)
		}
	}
	if !strings.Contains(strings.Join(args, " "), "--pipeline-max-steps 50") {
		t.Fatal("normal request budget unexpectedly reduced")
	}
	env := childEnv([]string{"CODRAX_SETTINGS=stale", "NO_COLOR=old", "KEEP=1"}, "/new/runtime.yaml")
	if !reflect.DeepEqual(env, []string{"KEEP=1", "CODRAX_SETTINGS=/new/runtime.yaml", "NO_COLOR=1"}) {
		t.Fatalf("child inherited conflicting configuration: %q", env)
	}
	data, err := runtimeSettings(options{providers: "/outside/providers.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := yaml.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["trace_attach_max_bytes"] != 1024 || settings["providers_config"] != "/outside/providers.yaml" || settings["write_enabled"] != false || settings["read_run_auto_resume"] != false {
		t.Fatalf("unfrozen child configuration: %+v", settings)
	}
}

// This explicit transport negative uses a non-model executable that exits 37.
// It can never produce a success receipt, even if stdout names the right tuple.
func TestFailedChildKeepsOriginalEvidenceAndCannotPassFromStdout(t *testing.T) {
	parent := t.TempDir()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(transportNegativeHelperEnv, "1")
	providers := filepath.Join(parent, "unused-provider-path")
	if err := os.WriteFile(providers, []byte("not read by this negative transport executable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(parent, "failed")
	if err := run(context.Background(), options{out: out, live: true, binary: bin, providers: providers, timeout: time.Minute}); err == nil {
		t.Fatal("nonzero child with no native artifacts reported success")
	}
	var receipt runReceipt
	if !readJSON(filepath.Join(out, "receipt.json"), &receipt) || receipt.Status != "machine_checks_failed" || receipt.Checks["cli_exit_zero"] || receipt.Checks["answer_mentions_expected_tuple"] || !strings.Contains(receipt.ExecutionError, "37") {
		t.Fatalf("missing honest failure receipt: %+v", receipt)
	}
	for _, name := range []string{"run.out", "evaluation.json", "stdin.txt", "input/capture.sys", "operator-oracle/query.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("failure discarded %s: %v", name, err)
		}
	}
}
