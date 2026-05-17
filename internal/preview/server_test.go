package preview

import (
	"io"
	"net"
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
	if !strings.Contains(html, "startOnLoad: false") ||
		!strings.Contains(html, "Mermaid render failed; raw source is preserved below.") {
		t.Fatalf("preview page must manually render mermaid with clean fallback:\n%s", html)
	}
}

func TestServerRegisterMarkdownAllowsClassDiagramInBrowserMermaid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "class.md")
	body := "# Classes\n\n```mermaid classDiagram\n  class Animal\n  Animal <|-- Duck\n```\n"
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
		!strings.Contains(html, "classDiagram") ||
		!strings.Contains(html, "Animal &lt;|-- Duck") {
		t.Fatalf("classDiagram was not handed to browser Mermaid:\n%s", html)
	}
	if !strings.Contains(html, "/assets/mermaid.min.js?token=") ||
		!strings.Contains(html, "window.mermaid.render") {
		t.Fatalf("browser Mermaid render path missing:\n%s", html)
	}
	if strings.Contains(html, "Mermaid 子集 [classDiagram]") {
		t.Fatalf("preview server must not use terminal subset fallback:\n%s", html)
	}
}

func TestRenderMarkdownHTMLNormalizesBareSubgraphTitles(t *testing.T) {
	body := []byte("```mermaid\nflowchart TD\n  subgraph Explorer System\n    A --> B\n  end\n  subgraph AlreadyValid[Already Valid]\n    C --> D\n  end\n```\n")
	got, err := RenderMarkdownHTML(body)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if !strings.Contains(got, `subgraph Explorer_System_2 [Explorer System]`) {
		t.Fatalf("bare subgraph title was not normalized:\n%s", got)
	}
	if !strings.Contains(got, `subgraph AlreadyValid[Already Valid]`) {
		t.Fatalf("already valid subgraph title should be preserved:\n%s", got)
	}
}

func TestRenderMarkdownHTMLNormalizesSequenceStop(t *testing.T) {
	body := []byte("```mermaid\nsequenceDiagram\n    participant Explorer as Explorer\n    participant Runtime as Runtime\n    Runtime-->>Explorer: Original Output\n    stop\n```\n")
	got, err := RenderMarkdownHTML(body)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if strings.Contains(got, "\n    stop\n") {
		t.Fatalf("bare sequence stop survived:\n%s", got)
	}
	if !strings.Contains(got, "Note over Explorer,Runtime: stop") {
		t.Fatalf("sequence stop note missing:\n%s", got)
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

func TestDisplayHostPortUsesConfiguredSpecificHost(t *testing.T) {
	host, port := displayHostPort("203.0.113.10", "0.0.0.0:49152")
	if host != "203.0.113.10" || port != "49152" {
		t.Fatalf("displayHostPort explicit host = %s:%s", host, port)
	}
}

func TestDisplayHostPortWildcardUsesReachableHost(t *testing.T) {
	old := reachableDisplayHost
	reachableDisplayHost = func() string { return "198.51.100.25" }
	defer func() { reachableDisplayHost = old }()

	host, port := displayHostPort("0.0.0.0", "0.0.0.0:49152")
	if host != "198.51.100.25" || port != "49152" {
		t.Fatalf("displayHostPort wildcard = %s:%s", host, port)
	}
}

func TestDisplayIPScorePrefersPublicIPv4(t *testing.T) {
	cases := []struct {
		name string
		ip   net.IP
		want int
	}{
		{"loopback", net.ParseIP("127.0.0.1"), 0},
		{"private ipv4", net.ParseIP("192.168.1.8"), 80},
		{"public ipv4", net.ParseIP("8.8.8.8"), 100},
		{"private ipv6", net.ParseIP("fd00::1"), 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := displayIPScore(c.ip); got != c.want {
				t.Fatalf("displayIPScore(%s)=%d, want %d", c.ip, got, c.want)
			}
		})
	}
}
