// repl_binary_tail runs one explicitly opted-in, actual CLI REPL evaluation.
// No model adapter, runner stub or precomputed query is installed in the child.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type options struct {
	out, binary, providers, settings string
	live, prepareOnly                bool
	timeout                          time.Duration
}

func main() {
	var opts options
	flag.StringVar(&opts.out, "out", "", "new absolute evidence directory (parent must exist)")
	flag.StringVar(&opts.binary, "binary", "", "actual built codrax executable (required with -live)")
	flag.StringVar(&opts.providers, "providers", "", "provider config path; referenced, never copied (required with -live)")
	flag.StringVar(&opts.settings, "settings", "", "optional non-secret baseline runtime YAML; evaluation overrides are recorded")
	flag.BoolVar(&opts.live, "live", false, "explicitly authorize exactly one real-model REPL question")
	flag.BoolVar(&opts.prepareOnly, "prepare-only", false, "validate fixture/oracle without starting a CLI or model")
	flag.DurationVar(&opts.timeout, "timeout", 20*time.Minute, "whole child process budget; no automatic rerun")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	if opts.live == opts.prepareOnly || !filepath.IsAbs(opts.out) || strings.ContainsAny(opts.out, "\r\n") || opts.timeout <= 0 {
		return errors.New("select exactly one of -live/-prepare-only, a fresh absolute -out, and positive timeout")
	}
	var err error
	if opts.live {
		if opts.binary, err = regularAbsolute(opts.binary); err != nil {
			return fmt.Errorf("binary: %w", err)
		}
		if opts.providers, err = regularAbsolute(opts.providers); err != nil {
			return fmt.Errorf("providers: %w", err)
		}
	}
	settings, err := runtimeSettings(opts)
	if err != nil {
		return err
	}
	if err := os.Mkdir(opts.out, 0700); err != nil {
		return fmt.Errorf("refuse reused/missing-parent output directory: %w", err)
	}
	// Canonical paths keep macOS /var→/private/var provenance exact.
	opts.out, err = filepath.EvalSymlinks(opts.out)
	if err != nil {
		return err
	}
	for _, dir := range []string{"input", "operator-oracle", "workspace", "logs"} {
		if err := os.Mkdir(filepath.Join(opts.out, dir), 0700); err != nil {
			return err
		}
	}
	input := filepath.Join(opts.out, "input", "capture.sys")
	if err := os.WriteFile(input, rmqFixture(), 0400); err != nil {
		return err
	}
	oracle, err := prepareOracle(ctx, input, filepath.Join(opts.out, "operator-oracle"))
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(opts.out, "oracle.json"), oracle); err != nil {
		return err
	}
	if opts.prepareOnly {
		return writeJSON(filepath.Join(opts.out, "receipt.json"), map[string]any{"status": "fixture_validated_only", "real_model_runs": 0, "human_audit": "not_run"})
	}
	workspace := filepath.Join(opts.out, "workspace")
	// No oracle, generator, answer, or converted material in the model repo.
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Trace evaluation workspace\nNo application source is supplied.\n"), 0600); err != nil {
		return err
	}
	settingsPath := filepath.Join(opts.out, "runtime.yaml")
	if err := os.WriteFile(settingsPath, settings, 0600); err != nil {
		return err
	}
	stdin := "/htrace " + input + "\n" + question + "\n/exit\n"
	if err := os.WriteFile(filepath.Join(opts.out, "stdin.txt"), []byte(stdin), 0600); err != nil {
		return err
	}
	args := childArgs(workspace, filepath.Join(opts.out, "logs"))
	binarySHA, err := fileSHA(opts.binary)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(opts.out, "evaluation.json"), map[string]any{
		"kind": "repl_rmq_binary_tail", "binary": opts.binary, "binary_sha256": binarySHA,
		"argv": args, "question": question, "real_model_question_count": 1, "fixture": oracle.Kind,
		"stdin": "stdin.txt", "preview_limit": previewBytes, "runtime_settings_sha256": bytesSHA(settings),
		"oracle_is_not_child_input": true, "automatic_reruns": 0,
	}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, opts.binary, args...)
	cmd.Dir = workspace
	cmd.Env = childEnv(os.Environ(), settingsPath)
	cmd.Stdin = strings.NewReader(stdin)
	output, err := os.OpenFile(filepath.Join(opts.out, "run.out"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = output, output
	started := time.Now()
	runErr := cmd.Run()
	closeErr := output.Close()
	receipt := auditRun(opts.out, oracle)
	receipt.Checks["cli_exit_zero"] = runErr == nil && closeErr == nil
	receipt.ElapsedSeconds = time.Since(started).Seconds()
	if runErr != nil {
		receipt.ExecutionError = runErr.Error()
	}
	receipt.finish()
	if err := writeJSON(filepath.Join(opts.out, "receipt.json"), receipt); err != nil {
		return err
	}
	if receipt.Status != "machine_checks_pass_human_pending" {
		return fmt.Errorf("real REPL evidence checks failed; original artifacts retained at %s", opts.out)
	}
	fmt.Printf("Machine checks passed; full human audit required: %s\n", opts.out)
	return nil
}

func childArgs(workspace, logs string) []string {
	return []string{"--repo", workspace, "--branch", "main", "--mode=code", "--chitchat-classifier=false", "--pipeline-max-steps", "50", "--log-level", "debug", "--log-dir", logs, "--color", "never"}
}

func runtimeSettings(opts options) ([]byte, error) {
	settings := map[string]any{}
	if opts.settings != "" {
		data, err := os.ReadFile(opts.settings)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, &settings); err != nil {
			return nil, err
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	for key, value := range map[string]any{"trace_attach_max_bytes": previewBytes, "output_dump_enabled": true, "read_run_auto_resume": false, "write_enabled": false, "markdown_preview_server": "off", "env_recommend_enabled": false} {
		settings[key] = value
	}
	if opts.providers != "" {
		settings["providers_config"] = opts.providers
	}
	return yaml.Marshal(settings)
}

func childEnv(prior []string, settings string) []string {
	out := make([]string, 0, len(prior)+2)
	for _, entry := range prior {
		if !strings.HasPrefix(entry, "CODRAX_SETTINGS=") && !strings.HasPrefix(entry, "NO_COLOR=") {
			out = append(out, entry)
		}
	}
	return append(out, "CODRAX_SETTINGS="+settings, "NO_COLOR=1")
}

func regularAbsolute(path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "\r\n") {
		return "", errors.New("missing or multiline path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return abs, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func bytesSHA(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
