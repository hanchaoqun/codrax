package preview

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SEC #28 pins: the preview server binds loopback-only by default. Preview
// pages render customer trace/repo analysis content and the auth token rides
// the URL query string, so all-interfaces exposure must be an explicit
// markdown_preview_host: "0.0.0.0" opt-in, never the default.

func TestDefaultHostIsLoopback(t *testing.T) {
	if defaultHost != "127.0.0.1" {
		t.Fatalf("defaultHost = %q, want loopback 127.0.0.1 (SEC #28: widening is an explicit config opt-in)", defaultHost)
	}
	cfg := NormalizeConfig(Config{})
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("NormalizeConfig empty host = %q, want 127.0.0.1", cfg.Host)
	}
}

func TestDefaultConfigListensOnLoopbackOnly(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "answer.md")
	if err := os.WriteFile(md, []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(Config{Mode: ModeOn}) // empty Host = default
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close(context.Background())
	url, err := srv.RegisterMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	addr := srv.addr
	srv.mu.Unlock()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("listener addr %q: %v", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("default listener bound to %q, want a loopback address", addr)
	}
	if !strings.Contains(url, "127.0.0.1") {
		t.Fatalf("default preview URL should carry the loopback host, got %q", url)
	}
}
