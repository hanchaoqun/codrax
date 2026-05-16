package preview

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerRegisterMarkdownRendersMermaidAndEscapesHTML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "answer.md")
	body := "# Answer\n\n<script>alert(1)</script>\n\n```mermaid\nflowchart TD\n  A[Start] --> B[Done]\n```\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	srv, err := NewServer(Config{Mode: ModeOn, Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(nil)

	u, err := srv.RegisterMarkdown(path)
	if err != nil {
		t.Fatalf("RegisterMarkdown: %v", err)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, `<div class="mermaid">`) ||
		!strings.Contains(html, "flowchart TD") {
		t.Fatalf("mermaid block not rendered for browser:\n%s", html)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("raw markdown HTML was not escaped:\n%s", html)
	}
	if !strings.Contains(html, "raw HTML omitted") {
		t.Fatalf("goldmark raw-HTML safety marker missing:\n%s", html)
	}
	if !strings.Contains(html, "/assets/mermaid.min.js?token=") {
		t.Fatalf("embedded mermaid asset URL missing:\n%s", html)
	}
}

func TestServerRequiresToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "answer.md")
	if err := os.WriteFile(path, []byte("# ok\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	srv, err := NewServer(Config{Mode: ModeOn, Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(nil)
	u, err := srv.RegisterMarkdown(path)
	if err != nil {
		t.Fatalf("RegisterMarkdown: %v", err)
	}
	u = strings.Split(u, "?")[0]
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET preview without token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status without token = %d, want 403", resp.StatusCode)
	}
}

func TestValidateConfig(t *testing.T) {
	if err := ValidateConfig(Config{Mode: "bogus"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
	if err := ValidateConfig(Config{Mode: ModeAuto, Port: 70000}); err == nil {
		t.Fatal("expected invalid port error")
	}
	if err := ValidateConfig(Config{}); err != nil {
		t.Fatalf("empty config should inherit defaults: %v", err)
	}
	cfg := NormalizeConfig(Config{})
	if cfg.Mode != ModeAuto || cfg.Host != defaultHost || cfg.Port != defaultPort {
		t.Fatalf("NormalizeConfig = %+v", cfg)
	}
}
