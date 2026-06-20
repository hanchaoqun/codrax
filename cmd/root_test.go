package cmd

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeCompatArgs_RewritesLegacySingleDashLongFlags(t *testing.T) {
	got := normalizeCompatArgs([]string{
		"-repo", ".",
		"-branch=main",
		"-request", "trace analyzer",
		"-pipeline-max-steps", "50",
		"-eval-disable-git-history",
		"-chitchat-classifier=true",
	})
	want := []string{
		"--repo", ".",
		"--branch=main",
		"--request", "trace analyzer",
		"--pipeline-max-steps", "50",
		"--eval-disable-git-history",
		"--chitchat-classifier=true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCompatArgs mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestEvalDisableGitHistoryFlagDefaultsOff(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("eval-disable-git-history")
	if flag == nil {
		t.Fatal("--eval-disable-git-history flag should be registered")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--eval-disable-git-history must default off, got %q", flag.DefValue)
	}
}

func TestProviderConfigErrorIsActionable(t *testing.T) {
	err := providerConfigError("/tmp/does-not-exist/providers.yaml", assertErr("providers.yaml: llm.default.provider is required"))
	msg := err.Error()
	for _, want := range []string{
		"LLM provider is not configured",
		"没有可用的模型 provider 配置",
		"/tmp/does-not-exist/providers.yaml",
		"not found",
		"llm.default.provider is required",
		"--providers /path/to/providers.yaml",
		"LLM_PROVIDER",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("provider config error missing %q:\n%s", want, msg)
		}
	}
}

func TestAnchorProviderTLSCAFilesUsesExecutableConfigAnchor(t *testing.T) {
	anchor := filepath.Join(t.TempDir(), "codrax-bin")
	absoluteCA := filepath.Join(t.TempDir(), "abs-ca.pem")
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{TLSCAFile: "cert.pem"},
			Agents: map[string]types.LLMProviderConfig{
				"analyzer":  {TLSCAFile: "certs/analyzer.pem"},
				"finalizer": {TLSCAFile: absoluteCA},
				"extractor": {},
			},
		},
	}

	anchorProviderTLSCAFiles(cfg, anchor)

	if got, want := cfg.LLM.Default.TLSCAFile, filepath.Join(anchor, "cert.pem"); got != want {
		t.Fatalf("default tls_ca_file anchored incorrectly: got %q want %q", got, want)
	}
	if got, want := cfg.LLM.Agents["analyzer"].TLSCAFile, filepath.Join(anchor, "certs/analyzer.pem"); got != want {
		t.Fatalf("agent tls_ca_file anchored incorrectly: got %q want %q", got, want)
	}
	if got := cfg.LLM.Agents["finalizer"].TLSCAFile; got != absoluteCA {
		t.Fatalf("absolute tls_ca_file should pass through: got %q want %q", got, absoluteCA)
	}
	if got := cfg.LLM.Agents["extractor"].TLSCAFile; got != "" {
		t.Fatalf("empty tls_ca_file should remain empty, got %q", got)
	}
}

func TestPostFinalizeLLMReviewersAreOptInByDefault(t *testing.T) {
	if selfConsistencyEnabled {
		t.Fatal("self-consistency reviewer must be opt-in by default; deterministic final-answer checks remain covered by pipeline_strict_answer_review_enabled")
	}
	if semanticQualityEnabled {
		t.Fatal("semantic-quality reviewer must be opt-in by default; deterministic final-answer checks remain covered by pipeline_strict_answer_review_enabled")
	}
}

func TestRuntimeStoresInstallOnOrchestrator(t *testing.T) {
	anchor := filepath.Join(t.TempDir(), ".codrax")
	stores := newRuntimeStores(anchor)
	if stores.PlanStore == nil || stores.PlanGroupStore == nil || stores.WriteWorkflowRunStore == nil || stores.OperationPendingStore == nil {
		t.Fatalf("runtime stores should all be initialized: %+v", stores)
	}
	if got, want := stores.PlanStore.PlanDir(), filepath.Join(anchor, "plans"); got != want {
		t.Fatalf("plan store dir mismatch: got %q want %q", got, want)
	}
	if got, want := stores.WriteWorkflowRunStore.WorkflowDir(), filepath.Join(anchor, "plans", "workflows"); got != want {
		t.Fatalf("workflow store dir mismatch: got %q want %q", got, want)
	}
	if got, want := stores.OperationPendingStore.Path(), filepath.Join(anchor, "operations", "pending.json"); got != want {
		t.Fatalf("operation pending path mismatch: got %q want %q", got, want)
	}

	orch := orchestrator.New(types.PipelineSettings{}, agent.NewRegistry(), skill.NewRegistry(), agent.NewSubAgentRegistry())
	installRuntimeStores(orch, stores)
	v := reflect.ValueOf(orch).Elem()
	for _, field := range []string{"planSaver", "planGroupStore", "writeWorkflowRunStore"} {
		got := v.FieldByName(field)
		if !got.IsValid() || got.IsNil() {
			t.Fatalf("orchestrator field %s should be installed", field)
		}
	}
}

func TestRuntimeReasoningGraphIDIsRuntimeScoped(t *testing.T) {
	got := runtimeReasoningGraphID(filepath.Join(t.TempDir(), ".codrax"))
	if !strings.HasPrefix(got, "runtime-.codrax-") {
		t.Fatalf("runtime graph id = %q", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestNormalizeCompatArgs_LeavesShortFlagsAndPositionalsUntouched(t *testing.T) {
	in := []string{"-r", "task", "--request", "task2", "positional", "-x"}
	got := normalizeCompatArgs(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("short flags/positionals changed:\n  got:  %#v\n  want: %#v", got, in)
	}
}

func TestNormalizeCompatArgs_StopsRewritingAfterDoubleDash(t *testing.T) {
	in := []string{"-repo", ".", "--", "-request", "literal"}
	got := normalizeCompatArgs(in)
	want := []string{"--repo", ".", "--", "-request", "literal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("double-dash stop mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestLoopPolicyFromAgentSettingsPreservesQuotaBurnGuards(t *testing.T) {
	settings := types.DefaultAgentSettings()
	settings.LoopMinInjectInterval = 1
	settings.LoopMaxContinuations = 2
	settings.LoopMaxMidLoopInjects = 3
	settings.LoopIdleStopThreshold = 4

	got := loopPolicyFromAgentSettings(settings)
	defaults := agent.DefaultLoopPolicy()
	if got.MinInjectInterval != 1 ||
		got.MaxContinuations != 2 ||
		got.MaxMidLoopInjects != 3 ||
		got.IdleStopThreshold != 4 {
		t.Fatalf("settings-backed loop fields not applied: %+v", got)
	}
	if got.IdenticalToolCallAfterSuccessStreak != defaults.IdenticalToolCallAfterSuccessStreak ||
		got.IdenticalToolCallAfterFailureStreak != defaults.IdenticalToolCallAfterFailureStreak ||
		got.IdenticalErrorStreak != defaults.IdenticalErrorStreak ||
		got.MaxPerKeyInjects != defaults.MaxPerKeyInjects {
		t.Fatalf("quota-burn guards should inherit DefaultLoopPolicy values, got %+v defaults %+v", got, defaults)
	}
}
