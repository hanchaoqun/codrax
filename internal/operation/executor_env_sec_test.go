package operation

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// SEC #29 pins: LLM-planned operation subprocesses run with a credential-
// stripped environment. Blocklist = name contains KEY/TOKEN/SECRET/PASSWORD
// (case-insensitive) plus CODRAX_SETTINGS; runtime-essential variables
// (PATH, HOME, …) pass through; explicit typed step env entries append last.

func TestOperationEnvNameIsSensitive(t *testing.T) {
	sensitive := []string{
		"OPENAI_API_KEY", "api_key", "GITHUB_TOKEN", "MyToken",
		"AWS_SECRET_ACCESS_KEY", "DB_PASSWORD", "password", "CODRAX_SETTINGS",
		"codrax_settings", "SSH_KEY_PATH", "NPM_AUTH_TOKEN",
	}
	for _, name := range sensitive {
		if !operationEnvNameIsSensitive(name) {
			t.Errorf("operationEnvNameIsSensitive(%q) = false, want true", name)
		}
	}
	benign := []string{
		"PATH", "HOME", "LANG", "TMPDIR", "SHELL", "USER", "TERM",
		"GOPATH", "SystemRoot", "PATHEXT", "PWD", "",
	}
	for _, name := range benign {
		if operationEnvNameIsSensitive(name) {
			t.Errorf("operationEnvNameIsSensitive(%q) = true, want false (runtime-essential must survive)", name)
		}
	}
}

func TestOperationSubprocessEnvStripsCredentials(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"OPENAI_API_KEY=sk-live-123",
		"GITHUB_TOKEN=ghp_456",
		"DEPLOY_SECRET=aaa",
		"DB_PASSWORD=bbb",
		"CODRAX_SETTINGS=/etc/codrax.yaml",
		"LANG=en_US.UTF-8",
	}
	step := []string{"MY_FLAG=1", "EXPLICIT_TOKEN=allowed-by-plan"}
	got := operationSubprocessEnv(parent, step)
	joined := "\n" + strings.Join(got, "\n") + "\n"
	for _, want := range []string{"PATH=/usr/bin", "HOME=/home/u", "LANG=en_US.UTF-8", "MY_FLAG=1", "EXPLICIT_TOKEN=allowed-by-plan"} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Errorf("sanitized env lost %q: %v", want, got)
		}
	}
	for _, leaked := range []string{"sk-live-123", "ghp_456", "DEPLOY_SECRET", "DB_PASSWORD", "CODRAX_SETTINGS"} {
		if strings.Contains(joined, leaked) {
			t.Errorf("sanitized env leaked %q: %v", leaked, got)
		}
	}
}

func TestExecutorSubprocessDoesNotSeeInheritedCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-based integration pin; unix only")
	}
	t.Setenv("CODRAX_SEC29_TEST_TOKEN", "leaky-value-987")
	plan := CommandOperationPlan{
		ID:     "sec29",
		Status: StatusReady,
		Steps: []CommandStep{{
			ID:        "s1",
			Shell:     `echo "token=[${CODRAX_SEC29_TEST_TOKEN:-}] path_set=[${PATH:+yes}]"`,
			TimeoutMS: 20000,
		}},
	}
	result := CommandExecutor{}.Execute(context.Background(), plan)
	if result.Status != StatusExecuted {
		t.Fatalf("execute status = %s: %+v", result.Status, result.StepResults)
	}
	out := result.OutputPreview
	if strings.Contains(out, "leaky-value-987") {
		t.Fatalf("subprocess saw the inherited *_TOKEN value: %s", out)
	}
	if !strings.Contains(out, "token=[]") {
		t.Fatalf("expected stripped token marker in output, got: %s", out)
	}
	if !strings.Contains(out, "path_set=[yes]") {
		t.Fatalf("PATH must survive the strip (runtime-essential), got: %s", out)
	}
}
