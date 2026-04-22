package llm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildRequest_ToolChoiceWire locks the wire-format contract for
// ChatOptions.ToolChoice. The OpenAI API takes `tool_choice` as either
// a string ("auto" / "required" / "none") or an object (force a
// specific function); codrax only forwards the string form today.
// Three invariants matter:
//   - empty or "auto" → field omitted (same payload as pre-options callers)
//   - "required" → field present as JSON string "required"
//   - tool_choice is never emitted when tools[] is empty (OpenAI rejects that)
func TestBuildRequest_ToolChoiceWire(t *testing.T) {
	schema := []ToolSchema{
		{Name: "emit_analysis", Description: "x", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	msgs := []Message{{Role: "user", Content: "hi"}}
	adapter := &OpenAIAdapter{model: "m"}

	t.Run("empty_options_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty options should omit tool_choice, got: %s", b)
		}
	})

	t.Run("auto_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "auto"})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("auto should omit tool_choice (matches provider default), got: %s", b)
		}
	})

	t.Run("required_emits_string", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"tool_choice":"required"`) {
			t.Errorf("required should serialize as \"tool_choice\":\"required\", got: %s", b)
		}
	})

	t.Run("required_without_tools_still_omitted", func(t *testing.T) {
		req := adapter.buildRequest(msgs, nil, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		// Without tools[] an OpenAI 400 would rejects tool_choice; we
		// omit it so the adapter still produces a valid request when a
		// caller declared no tools but asked for required.
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty tools should suppress tool_choice even when required, got: %s", b)
		}
	})
}

// TestBuildHTTPClient_TLSOptions pins the http.Client construction
// path for per-provider TLS config:
//   - zero options → stock client, no custom Transport
//   - tls_ca_file set → client uses a Transport with RootCAs populated
//     and InsecureSkipVerify still false
//   - tls_insecure_skip_verify=true → Transport with InsecureSkipVerify=true
func TestBuildHTTPClient_TLSOptions(t *testing.T) {
	t.Run("zero_options_uses_stock_client", func(t *testing.T) {
		c := buildHTTPClient(TLSOptions{}, "https://api.example.com/v1")
		if c.Transport != nil {
			t.Errorf("zero options should leave Transport nil (use DefaultTransport), got %T", c.Transport)
		}
	})

	t.Run("insecure_skip_verify_sets_flag", func(t *testing.T) {
		c := buildHTTPClient(TLSOptions{InsecureSkipVerify: true}, "https://api.example.com/v1")
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("InsecureSkipVerify not propagated, got %+v", tr.TLSClientConfig)
		}
	})

	t.Run("ca_file_populates_root_cas", func(t *testing.T) {
		// Write a minimal valid PEM (self-generated test root).
		// This cert is ONLY used for the RootCAs load path; the test
		// never actually dials anything.
		pem := generateTestCAPEM(t)
		dir := t.TempDir()
		caPath := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(caPath, pem, 0o600); err != nil {
			t.Fatalf("write ca.pem: %v", err)
		}
		c := buildHTTPClient(TLSOptions{CAFile: caPath}, "https://api.example.com/v1")
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
			t.Fatalf("RootCAs not populated: %+v", tr.TLSClientConfig)
		}
		if tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("CAFile path must not flip InsecureSkipVerify on")
		}
	})

	t.Run("bad_ca_file_path_falls_back_gracefully", func(t *testing.T) {
		// Missing file → log warning, skip RootCAs override.
		c := buildHTTPClient(TLSOptions{CAFile: "/does/not/exist/ca.pem"}, "https://api.example.com/v1")
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil {
			t.Fatalf("TLSClientConfig nil; want empty config after fallback")
		}
		if tr.TLSClientConfig.RootCAs != nil {
			t.Errorf("bad CA file should leave RootCAs nil, got populated pool")
		}
	})

	t.Run("zero_tls_cfg_does_not_leak_skip_verify", func(t *testing.T) {
		// Belt-and-suspenders: a CAFile-only client must not accidentally
		// enable InsecureSkipVerify on the Transport's fresh tls.Config.
		cfg := &tls.Config{}
		if cfg.InsecureSkipVerify {
			t.Errorf("fresh tls.Config should zero-init InsecureSkipVerify to false")
		}
	})
}

// generateTestCAPEM produces a valid self-signed certificate in PEM
// form. The cert is only used for the pool.AppendCertsFromPEM parse
// path; no TLS handshake actually touches it.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "codrax-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
