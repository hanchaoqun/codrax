package preview

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

const (
	ModeAuto = "auto"
	ModeOn   = "on"
	ModeOff  = "off"

	defaultHost        = "0.0.0.0"
	defaultPort        = 0
	maxMarkdownBytes   = 32 << 20
	readHeaderTimeout  = 5 * time.Second
	shutdownGrace      = 2 * time.Second
	mermaidAssetPath   = "assets/mermaid.min.js"
	mermaidAssetURL    = "/assets/mermaid.min.js"
	defaultContentType = "text/html; charset=utf-8"
)

//go:embed assets/mermaid.min.js assets/MERMAID_LICENSE
var assets embed.FS

// Config controls the in-process markdown preview server. Empty values
// are intentionally useful: Mode defaults to auto, Host to all
// interfaces, and Port to an OS-selected high port.
type Config struct {
	Mode string
	Host string
	Port int
}

// NormalizeConfig returns a config with process defaults filled in. It
// does not validate; callers that surface operator errors should call
// ValidateConfig after resolving YAML.
func NormalizeConfig(cfg Config) Config {
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = ModeAuto
	}
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	return cfg
}

// ValidateConfig rejects values that would produce surprising network
// behaviour. Port 0 is valid and means "ask the OS for a free port".
func ValidateConfig(cfg Config) error {
	cfg = NormalizeConfig(cfg)
	switch cfg.Mode {
	case ModeAuto, ModeOn, ModeOff:
	default:
		return fmt.Errorf("markdown_preview_server must be one of %q, %q, %q; got %q", ModeAuto, ModeOn, ModeOff, cfg.Mode)
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("markdown_preview_port must be in [0, 65535]; got %d", cfg.Port)
	}
	return nil
}

// Enabled reports whether the server may start. ModeAuto starts lazily
// only after a markdown file is registered.
func (cfg Config) Enabled() bool {
	return NormalizeConfig(cfg).Mode != ModeOff
}

// Server serves registered markdown files as rendered HTML. It is a
// best-effort UX helper: errors are returned to the caller so they can
// be logged without affecting the primary answer path.
type Server struct {
	cfg   Config
	token string

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	addr     string
	pathToID map[string]string
	idToPath map[string]string
}

// NewServer constructs a preview server. The listener is opened lazily
// on the first RegisterMarkdown call so REPL sessions that never produce
// an output file do not claim a port.
func NewServer(cfg Config) (*Server, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	cfg = NormalizeConfig(cfg)
	token, err := randomURLToken(24)
	if err != nil {
		return nil, fmt.Errorf("generate preview token: %w", err)
	}
	return &Server{
		cfg:      cfg,
		token:    token,
		pathToID: make(map[string]string),
		idToPath: make(map[string]string),
	}, nil
}

// RegisterMarkdown allows one markdown file to be previewed and returns
// a tokenized URL. The path must already exist and be a regular .md file.
func (s *Server) RegisterMarkdown(path string) (string, error) {
	if s == nil || !s.cfg.Enabled() {
		return "", nil
	}
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolve markdown path: %w", err)
	}
	abs = filepath.Clean(abs)
	if !strings.EqualFold(filepath.Ext(abs), ".md") {
		return "", fmt.Errorf("preview only serves .md files: %s", abs)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat markdown path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("preview path is not a regular file: %s", abs)
	}
	if info.Size() > maxMarkdownBytes {
		return "", fmt.Errorf("markdown preview file is too large: %d bytes", info.Size())
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureStartedLocked(); err != nil {
		return "", err
	}
	id := s.pathToID[abs]
	if id == "" {
		id, err = randomHexID(10)
		if err != nil {
			return "", fmt.Errorf("generate preview id: %w", err)
		}
		s.pathToID[abs] = id
		s.idToPath[id] = abs
	}
	return s.previewURLLocked(id), nil
}

// Close shuts the listener down. It is safe to call more than once.
func (s *Server) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
	}
	return server.Shutdown(ctx)
}

func (s *Server) ensureStartedLocked() error {
	if s.listener != nil {
		return nil
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("start markdown preview server on %s: %w", addr, err)
	}
	s.listener = ln
	s.addr = ln.Addr().String()
	s.server = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Warning("[markdown_preview] server stopped: %v", err)
		}
	}()
	logging.Info("[markdown_preview] listening on %s", s.addr)
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.tokenOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == mermaidAssetURL:
		s.serveMermaidAsset(w, r)
	case strings.HasPrefix(r.URL.Path, "/raw/"):
		s.serveRaw(w, r, strings.TrimPrefix(r.URL.Path, "/raw/"))
	case strings.HasPrefix(r.URL.Path, "/preview/"):
		s.servePreview(w, r, strings.TrimPrefix(r.URL.Path, "/preview/"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) tokenOK(r *http.Request) bool {
	return r.URL.Query().Get("token") == s.token
}

func (s *Server) serveMermaidAsset(w http.ResponseWriter, r *http.Request) {
	data, err := assets.ReadFile(mermaidAssetPath)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *Server) serveRaw(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := s.pathForID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) servePreview(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := s.pathForID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if info.Size() > maxMarkdownBytes {
		http.Error(w, "markdown file too large for preview", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "read markdown failed", http.StatusInternalServerError)
		return
	}
	body, err := RenderMarkdownHTML(data)
	if err != nil {
		http.Error(w, "render markdown failed", http.StatusInternalServerError)
		return
	}
	page := renderHTMLPage(pageArgs{
		Title:      filepath.Base(path),
		BodyHTML:   body,
		RawURL:     s.routeURL("/raw/"+id, r.URL.Query().Get("token")),
		MermaidURL: s.routeURL(mermaidAssetURL, r.URL.Query().Get("token")),
	})
	w.Header().Set("Content-Type", defaultContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data: http: https:; style-src 'unsafe-inline'; script-src 'self' 'unsafe-inline';")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(page))
	}
}

func (s *Server) pathForID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, `\`) {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.idToPath[id]
	return path, ok
}

func (s *Server) previewURLLocked(id string) string {
	return s.routeURL("/preview/"+id, s.token)
}

func (s *Server) routeURL(path, token string) string {
	host, port := displayHostPort(s.cfg.Host, s.addr)
	u := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   path,
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func displayHostPort(configuredHost, listenerAddr string) (string, string) {
	host, port, err := net.SplitHostPort(listenerAddr)
	if err != nil {
		return "127.0.0.1", "0"
	}
	displayHost := strings.TrimSpace(configuredHost)
	if displayHost == "" || displayHost == "0.0.0.0" || displayHost == "::" || displayHost == "[::]" {
		displayHost = "127.0.0.1"
	}
	if host != "" && configuredHost == "" {
		displayHost = "127.0.0.1"
	}
	return displayHost, port
}

type pageArgs struct {
	Title      string
	BodyHTML   string
	RawURL     string
	MermaidURL string
}

func renderHTMLPage(a pageArgs) string {
	title := html.EscapeString(a.Title)
	rawURL := html.EscapeString(a.RawURL)
	mermaidURL := html.EscapeString(a.MermaidURL)
	return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
<style>
:root { color-scheme: light dark; --fg: #202124; --muted: #667085; --line: #d0d7de; --code: #f6f8fa; --bg: #ffffff; }
@media (prefers-color-scheme: dark) {
  :root { --fg: #e6edf3; --muted: #9aa4b2; --line: #30363d; --code: #161b22; --bg: #0d1117; }
}
body { margin: 0; background: var(--bg); color: var(--fg); font: 16px/1.62 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
main { max-width: 980px; margin: 0 auto; padding: 32px 20px 64px; }
.topbar { display: flex; justify-content: space-between; gap: 16px; align-items: center; margin-bottom: 24px; color: var(--muted); font-size: 13px; }
a { color: #2563eb; text-decoration: none; }
a:hover { text-decoration: underline; }
h1, h2, h3 { line-height: 1.25; margin: 1.5em 0 .55em; }
h1:first-child { margin-top: 0; }
pre, code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace; }
pre { overflow: auto; padding: 14px 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--code); }
code { background: var(--code); border-radius: 4px; padding: .1em .3em; }
pre code { background: transparent; padding: 0; }
table { border-collapse: collapse; width: max-content; max-width: 100%; overflow: auto; display: block; }
th, td { border: 1px solid var(--line); padding: 6px 10px; }
blockquote { border-left: 4px solid var(--line); margin-left: 0; padding-left: 16px; color: var(--muted); }
.mermaid { margin: 18px 0; overflow: auto; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
</style>
</head>
<body>
<main>
<div class="topbar"><span>Codrax Markdown Preview</span><a href="` + rawURL + `">Raw Markdown</a></div>
` + a.BodyHTML + `
</main>
<script src="` + mermaidURL + `"></script>
<script>
if (window.mermaid) {
  window.mermaid.initialize({ startOnLoad: true, securityLevel: "strict", theme: "default" });
}
</script>
</body>
</html>`
}

func randomURLToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomHexID(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
