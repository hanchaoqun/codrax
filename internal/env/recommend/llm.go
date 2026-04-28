package recommend

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// llmAdapterRef is the cheap-model adapter cmd/root.go wires in.
// nil → LLM fallback short-circuits and returns no recommendations
// (caller falls through to Stage 4 DocsLink).
var (
	llmMu      sync.RWMutex
	llmAdapter llm.Adapter
)

// SetLLMAdapter wires the env_recommender adapter from cmd/root.go.
// Adapter is resolved by config.ResolveProvider("env_recommender")
// with a chitchat_classifier fallback.
func SetLLMAdapter(a llm.Adapter) {
	llmMu.Lock()
	llmAdapter = a
	llmMu.Unlock()
}

func currentLLMAdapter() llm.Adapter {
	llmMu.RLock()
	defer llmMu.RUnlock()
	return llmAdapter
}

// recommendFromLLM is Stage 3. Builds a concise environment
// digest + the diagnosis, asks the LLM to emit one
// emit_env_recommendation tool call, validates schema, returns
// recommendations. Failures (no adapter / timeout / unparsable /
// schema-invalid) silently return nil → caller falls through.
//
// Successful results write through to the disk cache so the
// second identical lookup never invokes LLM.
func recommendFromLLM(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation {
	adapter := currentLLMAdapter()
	if adapter == nil {
		return nil
	}
	metrics.llmCalls.Add(1)
	timeout := time.Duration(settings.LLMTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	systemPrompt := buildLLMSystemPrompt(settings)
	userPrompt := buildLLMUserPrompt(d, env)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	tools := []llm.ToolSchema{envRecommendToolSchema()}

	type result struct {
		resp llm.Response
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		// ctx already enforces the LLM timeout; the inner Adapter.Chat
		// honours it for HTTP-level cancellation, so a timeout fires
		// the SSE socket close immediately rather than leaking the
		// goroutine until natural completion.
		resp, err := adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
		ch <- result{resp: resp, err: err}
	}()

	select {
	case <-ctx.Done():
		metrics.llmTimeouts.Add(1)
		logging.Warning("[env/recommend/llm] timeout after %v", timeout)
		return nil
	case r := <-ch:
		if r.err != nil {
			metrics.llmErrors.Add(1)
			logging.Warning("[env/recommend/llm] chat err: %v", r.err)
			return nil
		}
		if len(r.resp.ToolCalls) == 0 {
			metrics.llmErrors.Add(1)
			logging.Debug("[env/recommend/llm] no tool_calls in response")
			return nil
		}
		recs := parseLLMRecommendations(r.resp.ToolCalls[0])
		if len(recs) > 0 {
			metrics.llmSuccess.Add(1)
			// Source-tag and write through to cache so next time
			// is free.
			storeInCache(d, env, recs, "llm")
		} else {
			metrics.llmErrors.Add(1)
		}
		return recs
	}
}

func buildLLMSystemPrompt(settings types.EnvRecommendSettings) string {
	var b strings.Builder
	b.WriteString(`You are codrax's environment-recommendation assistant. Given a structured diagnosis and an environment snapshot, emit exactly one emit_env_recommendation tool_call.

Hard rules:
- Output goes through structured fields only. NEVER answer in prose.
- strategy priority: venv_install > project_install > user_install > toolchain_bootstrap > docs_link. Use global_install ONLY when no other path exists, and ALWAYS set needs_sudo=true with a Caveat.
- Every command must start with "!" so the user can copy-paste it into the codrax prompt.
- Recommendations must reflect the operator's real environment — do not invent flags or package names.
- 2-3 candidates max. Ranked. Never duplicate.
`)
	if !settings.RecommendGlobalInstall {
		b.WriteString(`- global_install candidates are DISABLED in this configuration; do not produce them.
`)
	}
	return b.String()
}

func buildLLMUserPrompt(d types.Diagnosis, env *types.EnvFacts) string {
	envSummary := ""
	if env != nil {
		envSummary = fmt.Sprintf(
			"OS: %s / %s / %s\nShell: %s\nNetwork reachable: %t\nProject files: %s\nPath tools: %s\n",
			env.OS, env.OSFamily, env.OSVersion, env.Shell, env.Network.Reachable,
			joinKeys(env.ProjectFiles), joinKeys(env.PkgManagers))
	}
	return fmt.Sprintf(`Diagnosis:
- kind: %s
- runner: %s
- tool: %s
- source: %s
- excerpt: %s

Environment:
%s

Emit one emit_env_recommendation with 1-3 ranked recommendations.`,
		d.Kind, d.Runner, d.Tool, d.Source, d.Excerpt, envSummary)
}

func joinKeys[V any](m map[string]V) string {
	if len(m) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func envRecommendToolSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "emit_env_recommendation",
		Description: "Emit ranked install recommendations for the diagnosed environment problem.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "recommendations": {
      "type": "array",
      "minItems": 1,
      "maxItems": 3,
      "items": {
        "type": "object",
        "properties": {
          "strategy": {"type": "string", "enum": ["venv_install", "project_install", "user_install", "toolchain_bootstrap", "global_install", "docs_link"]},
          "command":  {"type": "string", "description": "Single shell command starting with !"},
          "why":      {"type": "string", "description": "Short rationale tied to the env snapshot"},
          "needs_network": {"type": "boolean"},
          "needs_sudo":    {"type": "boolean"},
          "estimated_sec": {"type": "integer"},
          "caveats":       {"type": "array", "items": {"type": "string"}}
        },
        "required": ["strategy", "command", "why"]
      }
    }
  },
  "required": ["recommendations"]
}`),
	}
}

func parseLLMRecommendations(call llm.ToolCall) []types.Recommendation {
	if call.Name != "emit_env_recommendation" {
		return nil
	}
	var p struct {
		Recommendations []struct {
			Strategy     string   `json:"strategy"`
			Command      string   `json:"command"`
			Why          string   `json:"why"`
			NeedsNetwork bool     `json:"needs_network"`
			NeedsSudo    bool     `json:"needs_sudo"`
			EstimatedSec int      `json:"estimated_sec"`
			Caveats      []string `json:"caveats"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(call.Params, &p); err != nil {
		return nil
	}
	out := make([]types.Recommendation, 0, len(p.Recommendations))
	for i, r := range p.Recommendations {
		strat, ok := parseStrategy(r.Strategy)
		if !ok {
			continue
		}
		// Reject commands that don't start with !; the system
		// prompt mandates this — anything else is suspicious.
		if !strings.HasPrefix(strings.TrimSpace(r.Command), "!") {
			continue
		}
		out = append(out, types.Recommendation{
			Strategy:     strat,
			Command:      r.Command,
			Why:          r.Why,
			NeedsNetwork: r.NeedsNetwork,
			NeedsSudo:    r.NeedsSudo,
			EstimatedSec: r.EstimatedSec,
			Caveats:      r.Caveats,
			Priority:     i + 1, // 1-indexed
		})
	}
	return out
}

func parseStrategy(s string) (types.InstallStrategy, bool) {
	switch s {
	case "venv_install":
		return types.StrategyVenvInstall, true
	case "project_install":
		return types.StrategyProjectInstall, true
	case "user_install":
		return types.StrategyUserInstall, true
	case "toolchain_bootstrap":
		return types.StrategyToolchainBootstrap, true
	case "global_install":
		return types.StrategyGlobalInstall, true
	case "docs_link":
		return types.StrategyDocsLink, true
	}
	return 0, false
}
