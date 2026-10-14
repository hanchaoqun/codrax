package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise main -> cmd.Execute -> CLI mode parsing -> the public REPL with
// precisely the live harness argv. Only /exit is supplied; no model response
// is synthesized. A local rejecting endpoint detects any unexpected request.
func TestActualCLIExitOnlyUsesHarnessArgumentsWithoutModelCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the actual CLI once; skipped under -short")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "codrax")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("actual CLI build: %v\n%s", err, output)
	}
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "exit-only regression must never call a model", http.StatusForbidden)
	}))
	defer endpoint.Close()
	for _, invalidMode := range []bool{false, true} {
		name := "harness_argv"
		if invalidMode {
			name = "internal_read_mode_is_not_a_cli_mode"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			workspace, logs := filepath.Join(dir, "workspace"), filepath.Join(dir, "logs")
			if err := os.Mkdir(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			providers := filepath.Join(dir, "providers.yaml")
			config := fmt.Sprintf("llm:\n  default:\n    provider: openai\n    api_key: exit-only-non-secret\n    model: exit-only\n    base_url: %q\n", endpoint.URL)
			if err := os.WriteFile(providers, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := runtimeSettings(options{providers: providers})
			if err != nil {
				t.Fatal(err)
			}
			settingsPath := filepath.Join(dir, "runtime.yaml")
			if err := os.WriteFile(settingsPath, settings, 0600); err != nil {
				t.Fatal(err)
			}
			args := childArgs(workspace, logs)
			if invalidMode {
				for i, arg := range args {
					if strings.HasPrefix(arg, "--mode=") {
						args[i] = "--mode=read"
					}
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, args...)
			command.Dir = workspace
			command.Env = childEnv(os.Environ(), settingsPath)
			command.Stdin = strings.NewReader("/exit\n")
			output, runErr := command.CombinedOutput()
			if invalidMode {
				if runErr == nil || !strings.Contains(string(output), `unknown mode "read"`) || strings.Contains(string(output), "Goodbye!") {
					t.Fatalf("invalid-mode negative bypassed real CLI parsing: %v\n%s", runErr, output)
				}
			} else if runErr != nil || !strings.Contains(string(output), "Goodbye!") {
				t.Fatalf("harness argv cannot enter/exit actual REPL: %v\n%s", runErr, output)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("exit-only CLI contacted model endpoint %d times", got)
			}
			paths, err := filepath.Glob(filepath.Join(logs, "*.log"))
			if err != nil || len(paths) == 0 {
				t.Fatalf("actual CLI emitted no inspectable logs: %v", err)
			}
			for _, path := range paths {
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(body), "phase=llm_request") || strings.Contains(string(body), "phase=toolcall") || strings.Contains(string(body), "[repl] final answer") {
					t.Fatalf("exit-only CLI unexpectedly entered an answer pipeline: %s", body)
				}
			}
		})
	}
}
