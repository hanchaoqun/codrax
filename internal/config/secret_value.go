package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ResolveSecretValue resolves one credential-bearing providers.yaml
// value (PIB-7, pi borrow — pi's resolve-config-value.ts, ledger
// docs/design/pi_borrow_analysis_20260729.md §3.5 item 8):
//
//	"!cmd …"        → run via the shell, trimmed stdout is the value
//	                  (lets operators source keys from keychain/vault
//	                  instead of writing plaintext into providers.yaml)
//	"$VAR" / "${VAR}" → environment variable reference (full-string
//	                  form only; an embedded "$" stays literal)
//	"$$rest"        → literal "$rest" (escape hatch)
//	anything else   → literal, returned unchanged
//
// Deliberate deviation from pi pinned in the ledger: pi re-executes
// shell commands on every request (no caching, for rotating tokens);
// codrax resolves ONCE at config load — API keys rotate rarely and a
// per-request subprocess in the hot path is not worth it.
//
// Fail-loud: a declared reference that cannot be resolved (command
// fails, variable unset) is a configuration error, not an empty
// credential — silently sending "" or the literal "$VAR" as an API key
// produces a far more confusing provider-side 401.
func ResolveSecretValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return raw, nil
	case strings.HasPrefix(value, "$$"):
		return "$" + strings.TrimPrefix(value, "$$"), nil
	case strings.HasPrefix(value, "!"):
		command := strings.TrimSpace(strings.TrimPrefix(value, "!"))
		if command == "" {
			return "", fmt.Errorf("providers config: empty !command value")
		}
		out, err := exec.Command("sh", "-c", command).Output()
		if err != nil {
			detail := ""
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				detail = ": " + strings.TrimSpace(string(ee.Stderr))
			}
			return "", fmt.Errorf("providers config: !command failed (%v)%s", err, detail)
		}
		resolved := strings.TrimSpace(string(out))
		if resolved == "" {
			return "", fmt.Errorf("providers config: !command produced empty output")
		}
		return resolved, nil
	case strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}"):
		name := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
		return resolveSecretEnv(name)
	case strings.HasPrefix(value, "$"):
		name := strings.TrimPrefix(value, "$")
		if name == "" || strings.ContainsAny(name, " \t{}$") {
			return raw, nil // not a clean full-string reference — literal
		}
		return resolveSecretEnv(name)
	}
	return raw, nil
}

func resolveSecretEnv(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("providers config: empty environment reference")
	}
	resolved, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(resolved) == "" {
		return "", fmt.Errorf("providers config: environment variable %s referenced but not set", name)
	}
	return strings.TrimSpace(resolved), nil
}

// resolveProviderSecrets rewrites every credential-bearing field in the
// decoded providers config, BEFORE per-agent inheritance: each declared
// !command runs exactly once, and merge's non-empty-overrides check
// never mistakes an unresolved "$VAR" literal for a real key.
func resolveProviderSecrets(cfg *types.ProvidersConfig) error {
	if cfg == nil {
		return nil
	}
	resolve := func(target *types.LLMProviderConfig, scope string) error {
		if target == nil {
			return nil
		}
		resolved, err := ResolveSecretValue(target.APIKey)
		if err != nil {
			return fmt.Errorf("%s api_key: %w", scope, err)
		}
		target.APIKey = resolved
		return nil
	}
	if err := resolve(&cfg.LLM.Default, "default"); err != nil {
		return err
	}
	for name := range cfg.LLM.Agents {
		agent := cfg.LLM.Agents[name]
		if err := resolve(&agent, "agent "+name); err != nil {
			return err
		}
		cfg.LLM.Agents[name] = agent
	}
	return nil
}
