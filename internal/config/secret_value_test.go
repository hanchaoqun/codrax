package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PIB-7 (ledger docs/design/pi_borrow_analysis_20260729.md §3.5 item
// 8): providers.yaml credential values support !command and $VAR
// references, resolved once at load, fail-loud on unresolvable
// references.

func TestResolveSecretValue_Forms(t *testing.T) {
	t.Setenv("CODRAX_TEST_KEY", "sk-from-env")

	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"literal", "sk-plain-key", "sk-plain-key", false},
		{"empty stays empty for env fallback", "", "", false},
		{"env dollar", "$CODRAX_TEST_KEY", "sk-from-env", false},
		{"env braces", "${CODRAX_TEST_KEY}", "sk-from-env", false},
		{"env unset fails loud", "$CODRAX_TEST_UNSET_XYZ", "", true},
		{"command", "!printf sk-from-cmd", "sk-from-cmd", false},
		{"command failure fails loud", "!false", "", true},
		{"command empty output fails loud", "!true", "", true},
		{"dollar escape", "$$literal-dollar", "$literal-dollar", false},
		{"embedded dollar stays literal", "pre$FIX-key", "pre$FIX-key", false},
	}
	for _, tc := range cases {
		got, err := ResolveSecretValue(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error, got %q", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLoadProviders_ResolvesSecretsBeforeInheritance pins the load-time
// integration: default + per-agent api_key references resolve once at
// load, so ResolveProvider inheritance always sees real values.
func TestLoadProviders_ResolvesSecretsBeforeInheritance(t *testing.T) {
	t.Setenv("CODRAX_TEST_DEFAULT_KEY", "sk-default")
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	yaml := `
llm:
  default:
    provider: openai
    model: m1
    api_key: $CODRAX_TEST_DEFAULT_KEY
  agents:
    analyzer:
      api_key: "!printf sk-analyzer"
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	if cfg.LLM.Default.APIKey != "sk-default" {
		t.Errorf("default api_key = %q, want resolved env value", cfg.LLM.Default.APIKey)
	}
	if got := cfg.LLM.Agents["analyzer"].APIKey; got != "sk-analyzer" {
		t.Errorf("agent api_key = %q, want resolved command output", got)
	}

	// Unresolvable reference → load fails loud with the scope named.
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("llm:\n  default:\n    api_key: $CODRAX_TEST_UNSET_XYZ\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProviders(bad); err == nil || !strings.Contains(err.Error(), "default api_key") {
		t.Errorf("unresolvable reference must fail loud naming the scope; got %v", err)
	}
}
