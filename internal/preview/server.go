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

	// SEC #28 (2026-07-10): default bind is loopback-only. Preview pages
	// render customer trace/repo analysis content, and the auth token rides
	// the URL query string (browser history / chat paste / proxy logs), so
	// LAN exposure must be an explicit operator decision: set
	// codrax.yaml :: markdown_preview_host: "0.0.0.0" to widen.
	defaultHost        = "127.0.0.1"
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
// are intentionally useful: Mode defaults to auto, Host to loopback
// (127.0.0.1; set 0.0.0.0 explicitly for LAN exposure), and Port to an
// OS-selected high port.
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
	urlHost  string
	urlPort  string
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
	s.urlHost = ""
	s.urlPort = ""
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
	s.urlHost, s.urlPort = displayHostPort(s.cfg.Host, s.addr)
	s.server = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	// Capture the *http.Server locally: Close nils s.server under the lock,
	// and a goroutine that re-reads the field can otherwise panic on a
	// nil Serve when Close wins the race before Serve starts.
	server := s.server
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Warning("[markdown_preview] server stopped: %v", err)
		}
	}()
	logging.Info("[markdown_preview] listening on %s display_host=%s", s.addr, s.urlHost)
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
		Lang:       markdownDocumentLanguage(data),
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
	host, port := s.urlHost, s.urlPort
	if host == "" || port == "" {
		host, port = displayHostPort(s.cfg.Host, s.addr)
	}
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

var reachableDisplayHost = defaultReachableDisplayHost

func displayHostPort(configuredHost, listenerAddr string) (string, string) {
	host, port, err := net.SplitHostPort(listenerAddr)
	if err != nil {
		return "127.0.0.1", "0"
	}
	displayHost := strings.TrimSpace(configuredHost)
	if !isWildcardHost(displayHost) {
		return strings.Trim(displayHost, "[]"), port
	}
	if candidate := strings.TrimSpace(reachableDisplayHost()); candidate != "" {
		return strings.Trim(candidate, "[]"), port
	}
	if !isWildcardHost(host) {
		return strings.Trim(host, "[]"), port
	}
	displayHost = "127.0.0.1"
	return displayHost, port
}

func isWildcardHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	return host == "" || host == "0.0.0.0" || host == "::"
}

func defaultReachableDisplayHost() string {
	if ip := outboundDisplayIP(); ip != "" {
		return ip
	}
	if ip := interfaceDisplayIP(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

func outboundDisplayIP() string {
	conn, err := net.DialTimeout("udp4", "8.8.8.8:80", 150*time.Millisecond)
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return displayableIPString(addr.IP)
}

func interfaceDisplayIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	best := ""
	bestScore := 0
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			score := displayIPScore(ip)
			if score > bestScore {
				bestScore = score
				best = ip.String()
			}
		}
	}
	return best
}

func ipFromAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

func displayableIPString(ip net.IP) string {
	if displayIPScore(ip) == 0 {
		return ""
	}
	return ip.String()
}

func displayIPScore(ip net.IP) int {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return 0
	}
	if v4 := ip.To4(); v4 != nil {
		if ip.IsPrivate() {
			return 80
		}
		return 100
	}
	if ip.IsPrivate() {
		return 60
	}
	if ip.IsGlobalUnicast() {
		return 70
	}
	return 0
}

type pageArgs struct {
	Title      string
	BodyHTML   string
	RawURL     string
	MermaidURL string
	MermaidJS  string
	Lang       string
}

func renderHTMLPage(a pageArgs) string {
	title := html.EscapeString(a.Title)
	lang := "zh-CN"
	previewLabel := "Codrax 报告预览"
	rawLabel := "原始 Markdown"
	standaloneLabel := "自包含 HTML"
	if strings.EqualFold(strings.TrimSpace(a.Lang), "en") {
		lang = "en"
		previewLabel = "Codrax Report Preview"
		rawLabel = "Raw Markdown"
		standaloneLabel = "Self-contained HTML"
	}
	rawURL := html.EscapeString(a.RawURL)
	mermaidURL := html.EscapeString(a.MermaidURL)
	rawLink := `<span>` + standaloneLabel + `</span>`
	if rawURL != "" {
		rawLink = `<a href="` + rawURL + `">` + rawLabel + `</a>`
	}
	mermaidLoader := ""
	if strings.TrimSpace(a.MermaidJS) != "" {
		mermaidLoader = "<script>\n" + safeInlineScript(a.MermaidJS) + "\n</script>\n"
	} else if mermaidURL != "" {
		mermaidLoader = `<script src="` + mermaidURL + `"></script>` + "\n"
	}
	return `<!doctype html>
<html lang="` + lang + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
<style>
:root {
  color-scheme: light dark;
  --fg: #202124; --muted: #667085; --line: #d0d7de; --code: #f6f8fa; --bg: #ffffff;
  --error-bg: #fff5f5; --error-fg: #b42318;
  --link: #2563eb; --focus: #2563eb;
  --rank-1-fg: #7c2d12; --rank-1-bg: #ffedd5;
  --rank-2-fg: #1e40af; --rank-2-bg: #dbeafe;
  --rank-3-fg: #5b21b6; --rank-3-bg: #ede9fe;
  --rank-4-fg: #14532d; --rank-4-bg: #dcfce7;
  --rank-5-fg: #9d174d; --rank-5-bg: #fce7f3;
  /* UXG-0 D2 (§29.36.2): ◇ adjacent-channel ordinals share the chip shape but
     wear ONE neutral slate-blue pair — a per-rank palette would invite the
     forbidden cross-channel comparison with the root-cause seat colors. */
  --rank-adjacent-fg: #334155; --rank-adjacent-bg: #e2e8f0;
  --action-fg: #166534; --action-bg: #ecfdf3; --action-border: #86efac;
  --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, "PingFang SC", "HarmonyOS Sans SC", "Microsoft YaHei UI", "Microsoft YaHei", "Noto Sans SC", "Noto Sans CJK SC", "Source Han Sans SC", "WenQuanYi Micro Hei", Arial, sans-serif;
  --font-mono: "Sarasa Mono SC", "Noto Sans Mono CJK SC", "Source Han Mono SC", ui-monospace, "Cascadia Mono", "Cascadia Code", SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Microsoft YaHei UI", "Microsoft YaHei", monospace;
  --font-symbols: "Apple Symbols", "Segoe UI Symbol", "Noto Sans Symbols 2", "Symbola", "Arial Unicode MS", sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root { --fg: #e6edf3; --muted: #9aa4b2; --line: #30363d; --code: #161b22; --bg: #0d1117; --error-bg: #2a1212; --error-fg: #ffb4a8; --link: #79c0ff; --focus: #60a5fa; --rank-1-fg: #fed7aa; --rank-1-bg: #7c2d12; --rank-2-fg: #bfdbfe; --rank-2-bg: #1e3a8a; --rank-3-fg: #ddd6fe; --rank-3-bg: #4c1d95; --rank-4-fg: #bbf7d0; --rank-4-bg: #14532d; --rank-5-fg: #fbcfe8; --rank-5-bg: #831843; --rank-adjacent-fg: #cbd5e1; --rank-adjacent-bg: #334155; --action-fg: #bbf7d0; --action-bg: #123524; --action-border: #22c55e; }
}
/* CJK-heavy report body: the stack must name CJK faces explicitly —
   without them Windows browsers fall back to SimSun for the zh text,
   which renders cramped (customer 2026-07-09: 字间距过紧难读). Latin
   faces stay first so mixed zh/en prose keeps platform Latin glyphs. */
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg); font: 16px/1.78 var(--font-sans); }
main { width: 100%; max-width: 980px; min-width: 0; margin: 0 auto; padding: 32px 20px 64px; letter-spacing: .02em; }
p, li, blockquote, th, td { overflow-wrap: anywhere; }
li { margin: .3em 0; }
.topbar { display: flex; justify-content: space-between; gap: 16px; align-items: center; margin-bottom: 24px; color: var(--muted); font-size: 13px; }
a { color: var(--link); text-decoration: none; }
a:hover { text-decoration: underline; }
h1, h2, h3 { line-height: 1.25; margin: 1.5em 0 .55em; overflow-wrap: anywhere; }
h1:first-child { margin-top: 0; }
/* Runtime reports are decision-first chapters. A light H2 divider makes the
   scan path explicit without turning every section into a card dashboard. */
h2 { margin-top: 2em; padding-bottom: .34em; border-bottom: 1px solid var(--line); }
/* Causal-projection trees rely on a per-character grid: letter-spacing
   must stay 0 here (a constant per-char pad breaks the CJK 2:1 width
   ratio the bars are aligned against), and the CJK fallbacks keep tree
   glyphs out of SimSun on Windows. */
pre, code { font-family: var(--font-mono); letter-spacing: 0; }
pre { max-width: 100%; overflow: auto; overscroll-behavior-x: contain; -webkit-overflow-scrolling: touch; padding: 14px 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--code); line-height: 1.6; }
code { background: var(--code); border-radius: 4px; padding: .1em .3em; }
pre code { display: block; background: transparent; padding: 0; overflow-wrap: normal; word-break: normal; }
/* The causal tree is a calibrated character grid, not ordinary source code.
   Prefer CJK-capable mono faces, suppress ligatures/colored emoji substitution,
   and render it one step smaller than prose.  Horizontal scrolling preserves
   the causal columns on narrow screens; the renderer already caps every row. */
pre.trace-projection-tree { font-size: .8125rem; line-height: 1.52; white-space: pre; overflow-x: auto; overflow-y: hidden; scrollbar-gutter: stable; tab-size: 2; font-kerning: none; font-variant-ligatures: none; font-variant-emoji: text; font-feature-settings: "liga" 0, "calt" 0; }
pre.trace-projection-tree code { font: inherit; line-height: inherit; white-space: inherit; font-kerning: inherit; font-variant-ligatures: inherit; font-variant-emoji: inherit; font-feature-settings: inherit; }
pre.trace-projection-tree .trace-cell { display: inline-block; width: 1ch; min-width: 1ch; height: 1em; line-height: 1em; vertical-align: baseline; white-space: pre; }
pre.trace-projection-tree .trace-cell-0 { width: 0; min-width: 0; }
pre.trace-projection-tree .trace-cell-2 { width: 2ch; min-width: 2ch; }
/* v5 P0 mark-envelope model (design causal_tree_v5_design_20260711.md
   重-1/§C.3/§D-5, user ruling 2026-07-11).
   EVOLUTION RECORD (supersedes UXR-1 §29.36⑤'s as-built form and the UXG-0
   D4 overflow-budget patch; ledger real_trace_campaign_20260705.md
   §29.36⑤/§29.38): the §29.36⑤ "glyph 尺寸/行高归一" goal is now honored by
   GEOMETRY, not tuning — the per-ICON optical scale family (1.10 / 1.00 /
   1.15, whose own comment had already drifted to "~1.22x") and the
   icon-cell overflow spill accounting are RETIRED as a family. (The chip's
   .95 below is NOT this family's subject — see the T-6 note at its rule.)
   A state mark and its generator-guaranteed companion space fuse into ONE
   2ch inline-grid slot; textContent keeps both bytes, columns never move.
   Sufficiency is arithmetic, not calibration: cell 1ch~=7.2px; line box
   13px x 1.52 ~= 19.76px (勘正 fix round 2026-07-11: as-built line-height
   is 1.52, not the design draft's 1.55 — the arithmetic holds either way);
   envelope 2ch ~= 14.4px >= max measured symbol ink 13.8px; ink height
   <= 13.8px < line box, so overflow:visible can never touch a neighboring
   row's ink. No explicit slot height, no vertical-align magic number — the
   baseline is inherited from the inline box (if a measured offset ever
   needs correcting, use the derived form -(lineBox-1em)/2 and write out the
   derivation; bare constants are forbidden). A mark WITHOUT a companion
   space (the circled mark inside the 链止 keep-mark) keeps a 1ch slot with its
   overflowing ink track centered — place-items centers the item only
   within its grid AREA, and Chromium sizes the auto track to the ink, so
   the wider-than-1ch track needs justify-content to sit symmetrically
   (真机 -0.78px per side) — still overflow:visible, spill stays
   unclipped. */
pre.trace-projection-tree .trace-slot { display: inline-grid; width: 2ch; min-width: 2ch; place-items: center start; overflow: visible; vertical-align: baseline; white-space: pre; }
pre.trace-projection-tree .trace-slot-1 { width: 1ch; min-width: 1ch; place-items: center; justify-content: center; }
/* CAL-1 件⑥b (2026-07-12): the neutral mark's ink is the smallest in the
   directory and reads faint at 13px. The audited bigger-ink replacements
   (the bullet / filled-circle family) are ALL East-Asian-Ambiguous (width 2
   in EAW context) and disqualified by the dual-context width-1 rule, so the
   HTML face compensates optically INSIDE the envelope: ink-only font-size
   raise, grid geometry untouched (the slot keeps its 2ch/1ch track), still
   overflow:visible. Arithmetic, not calibration (修复轮 P3-3 勘正): em box
   13px*1.2 = 15.6px < line box 19.76px (no line growth); the mark's ink
   budget ~0.6em of the enlarged box = 0.6*15.6 = 9.36px < the 14.4px
   envelope — spill impossible. */
pre.trace-projection-tree .trace-icon-neutral .trace-ink { font-size: 1.2em; }
pre.trace-projection-tree .trace-ink { line-height: 1; font-family: var(--font-symbols); font-synthesis: none; font-variant-emoji: text; }
/* Run segmentation (v5 §C.3 C-9): pure-ASCII runs collapse into ONE span
   pinned to their whole width (mono ASCII metrics are reliable; ~5x fewer
   DOM nodes). Box-drawing rails (.trace-rail) and bar blocks (.trace-bar)
   stay per-rune 1ch .trace-cell cells — fallback fonts draw the U+2500
   series full-width, so run-level pinning would overlap ink; █▒░ block
   glyphs are full-cell ink by design (the one physically-correct 1ch mark
   class). CJK/mixed runes keep per-rune 1/2ch cells. */
pre.trace-projection-tree .trace-run { display: inline-block; height: 1em; line-height: 1em; vertical-align: baseline; white-space: pre; }
/* 档1 decorations (v5 §C.3 T-5) — pure CSS/attribute, zero byte changes:
   per-line hover wash; ◇/▒ stanza heads pinned against the fence's own
   HORIZONTAL scroll (the pre is the scroll container, so left:0 keeps the
   section word readable while the columns scroll); E# tokens as in-page
   anchors to the paired detail/evidence entries. */
pre.trace-projection-tree .trace-line:hover { background: rgba(127, 127, 127, .14); }
pre.trace-projection-tree .trace-stanza-head { position: sticky; left: 0; display: inline-block; background: var(--code); }
pre.trace-projection-tree a.trace-eref { display: inline-block; height: 1em; line-height: 1em; vertical-align: baseline; white-space: pre; color: inherit; text-decoration: underline dotted; text-underline-offset: .18em; }
pre.trace-projection-tree a.trace-eref:hover { color: var(--link); text-decoration-style: solid; }
/* Ordinal chips consume exactly the same two grid cells as their Markdown
   tokens (#1..#5 after a channel word — pure ASCII, no clipping risk).
   Color adds a scan target; the rounded colored pill is the chip's meaning
   surface and must not bleed past its own grid cells onto neighboring text.
   EVOLUTION RECORD (CAL-1 件⑥a, 2026-07-12 — T-6 fulfilled ahead of the P2a
   constant-4 mark-field batch): the TOP-5 badge chip form (1ch cell with
   hidden overflow plus its inner-glyph .95 ink shrink) is RETIRED. UXG-0 D5 already put the badge's companion space in the fence
   bytes, so the badge now rides the same v5 P0 2ch inline-grid envelope as
   the state marks (.trace-slot .trace-rank-pill below): colored pill
   stretched to the full 2ch, overflow visible, dingbat ink complete —
   geometry, not tuning (the scale family stays retired; do not resurrect
   scale for badges either). */
pre.trace-projection-tree .trace-rank-ordinal { display: inline-block; height: 1em; line-height: 1em; vertical-align: baseline; white-space: pre; overflow: hidden; border-radius: .22em; font-weight: 750; text-align: center; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
pre.trace-projection-tree .trace-rank-width-1 { width: 1ch; min-width: 1ch; }
pre.trace-projection-tree .trace-rank-width-2 { width: 2ch; min-width: 2ch; }
pre.trace-projection-tree .trace-rank-pill { border-radius: .22em; font-weight: 750; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
pre.trace-projection-tree .trace-rank-1 { color: var(--rank-1-fg); background: var(--rank-1-bg); }
pre.trace-projection-tree .trace-rank-2 { color: var(--rank-2-fg); background: var(--rank-2-bg); }
pre.trace-projection-tree .trace-rank-3 { color: var(--rank-3-fg); background: var(--rank-3-bg); }
pre.trace-projection-tree .trace-rank-4 { color: var(--rank-4-fg); background: var(--rank-4-bg); }
pre.trace-projection-tree .trace-rank-5 { color: var(--rank-5-fg); background: var(--rank-5-bg); }
pre.trace-projection-tree .trace-rank-adjacent { color: var(--rank-adjacent-fg); background: var(--rank-adjacent-bg); }
pre.trace-projection-tree .trace-action-token { display: inline-block; height: 1em; line-height: 1em; vertical-align: baseline; overflow: hidden; border-radius: .18em; color: var(--action-fg); background: var(--action-bg); font-weight: 750; text-align: center; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
pre.trace-projection-tree .trace-action-width-6 { width: 6ch; min-width: 6ch; }
pre.trace-projection-tree .trace-action-width-18 { width: 18ch; min-width: 18ch; }
pre.trace-projection-tree:focus-visible { outline: 3px solid var(--focus); outline-offset: 2px; }
table { border-collapse: collapse; width: max-content; max-width: 100%; overflow: auto; overscroll-behavior-x: contain; display: block; font-size: .925em; line-height: 1.52; }
th, td { border: 1px solid var(--line); padding: 6px 10px; }
th { background: var(--code); font-weight: 650; }
blockquote { border-left: 4px solid var(--line); margin-left: 0; padding-left: 16px; color: var(--muted); }
img, svg { max-width: 100%; height: auto; }
/* Lossless trace audit surfaces remain complete but visually subordinate to
   conclusion/root-cause/optimization chapters. Per-node blocks flow into two
   balanced columns on wide screens; a node's paragraph/list stays together. */
section.trace-projection-detail,
section.trace-projection-evidence { margin-top: 2.5em; padding: 14px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--code); font-size: .86rem; line-height: 1.55; }
/* Action surface: visible before the lossless audit appendix, but restrained
   enough that TOP rank chips remain the strongest scan anchors. */
section.trace-action-optimization { margin: 1.45em 0 2em; padding: 12px 14px 14px; border: 1px solid var(--action-border); border-left-width: 4px; border-radius: 9px; background: var(--action-bg); }
section.trace-action-optimization > h2 { margin: 0 0 .72em; padding-bottom: .4em; border-bottom-color: var(--action-border); color: var(--action-fg); font-size: 1.24rem; }
section.trace-action-optimization th:first-child,
section.trace-action-optimization td:first-child { color: var(--action-fg); font-weight: 700; }
section.trace-projection-detail { column-count: 2; column-gap: 28px; column-rule: 1px solid var(--line); }
section.trace-projection-detail > h2,
section.trace-projection-evidence > h2 { margin: 0 0 .75em; padding-bottom: .45em; border-bottom: 1px solid var(--line); font-size: 1.18rem; column-span: all; }
/* Keep each per-node E# heading attached to the first attribute block instead
   of stranding it at the foot of the left audit column. */
section.trace-projection-detail > h3 { break-after: avoid-column; break-inside: avoid-column; }
section.trace-projection-detail > p { break-after: avoid-column; }
section.trace-projection-detail > ul,
section.trace-projection-detail > ol,
section.trace-projection-detail > table,
section.trace-projection-detail > pre { break-before: avoid-column; break-inside: avoid-column; }
section.trace-projection-detail li,
section.trace-projection-evidence li { margin: .2em 0; }
/* §29.9 aux-reference appendix (the tracefence.AuxRefMarkers blocks, markdown_auxfold.go):
   reference blocks relocate to the document-end 「阅读参考」 appendix;
   the in-place pointer lines and the whole appendix render small and
   muted, visibly subordinate to the main body. */
.aux { font-size: .85em; color: var(--muted); }
section.aux { margin-top: 3em; border-top: 1px solid var(--line); padding-top: 12px; }
section.aux h2 { font-size: 1em; color: var(--muted); margin: 0 0 .5em; font-weight: 600; }
.aux-src { margin: 1.2em 0 .2em; opacity: .85; font-style: italic; }
/* Mermaid measures label boxes in a detached container that never sees
   main's letter-spacing; any inherited tracking renders labels wider than
   the measured node box and clips the last glyph. Keep spacing at normal
   inside the diagram so rendered width matches measured width. */
.mermaid { margin: 18px 0; overflow: auto; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); letter-spacing: normal; }
.mermaid[data-render-error="true"] { border-color: #d92d20; background: var(--error-bg); }
.mermaid-error-title { margin: 0 0 10px; color: var(--error-fg); font-weight: 600; }
.mermaid[data-render-error="true"] pre { margin: 0; background: var(--code); }
@media (max-width: 640px) {
  body { font-size: 15px; line-height: 1.72; }
  main { padding: 20px 12px 44px; }
  .topbar { flex-wrap: wrap; gap: 6px 12px; margin-bottom: 18px; }
  pre { padding: 11px 12px; }
  pre.trace-projection-tree { font-size: 12px; line-height: 1.48; }
  th, td { padding: 5px 8px; }
  section.trace-projection-detail { column-count: 1; }
  section.trace-projection-detail,
  section.trace-projection-evidence { padding: 11px 12px; font-size: 12.5px; }
}
@page { margin: 12mm; }
@media print {
  :root { color-scheme: light; --fg: #111; --muted: #555; --line: #aaa; --code: #f7f7f7; --bg: #fff; --rank-1-fg: #7c2d12; --rank-1-bg: #ffedd5; --rank-2-fg: #1e40af; --rank-2-bg: #dbeafe; --rank-3-fg: #5b21b6; --rank-3-bg: #ede9fe; --rank-4-fg: #14532d; --rank-4-bg: #dcfce7; --rank-5-fg: #9d174d; --rank-5-bg: #fce7f3; --rank-adjacent-fg: #334155; --rank-adjacent-bg: #e2e8f0; --action-fg: #166534; --action-bg: #ecfdf3; --action-border: #86efac; }
  body { background: #fff; color: #111; font: 10.5pt/1.55 var(--font-sans); }
  main { max-width: none; padding: 0; letter-spacing: 0; }
  .topbar { display: none; }
  a { color: inherit; text-decoration: underline; }
  h1, h2, h3 { break-after: avoid-page; page-break-after: avoid; }
  pre:not(.trace-projection-tree) { white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; }
  pre.trace-projection-tree { font-size: 7.5pt; line-height: 1.4; overflow: visible; scrollbar-gutter: auto; border-color: #aaa; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
  table { display: table; width: 100%; max-width: 100%; overflow: visible; table-layout: fixed; font-size: 8.5pt; }
  th, td { overflow-wrap: anywhere; word-break: break-word; }
  .mermaid { break-inside: avoid-page; page-break-inside: avoid; }
  section.trace-projection-detail,
  section.trace-projection-evidence { font-size: 8pt; line-height: 1.42; }
  section.trace-projection-detail { column-count: 2; column-gap: 8mm; }
}
</style>
</head>
<body>
<main>
<div class="topbar"><span>` + previewLabel + `</span>` + rawLink + `</div>
` + a.BodyHTML + `
</main>
` + mermaidLoader + `<script>
function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, function (ch) {
    return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[ch];
  });
}
if (window.mermaid) {
  window.mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: "default" });
  document.querySelectorAll(".mermaid").forEach(function (node, index) {
    var source = node.textContent || "";
    window.mermaid.render("codrax-mermaid-" + index, source).then(function (result) {
      node.innerHTML = result.svg;
      node.removeAttribute("data-render-error");
    }).catch(function () {
      node.setAttribute("data-render-error", "true");
      node.innerHTML = '<p class="mermaid-error-title">Mermaid render failed; raw source is preserved below.</p><pre><code>' + escapeHTML(source) + '</code></pre>';
    });
  });
}
</script>
</body>
</html>`
}

func safeInlineScript(js string) string {
	return strings.NewReplacer(
		"</script", `<\/script`,
		"</SCRIPT", `<\/SCRIPT`,
		"</Script", `<\/Script`,
	).Replace(js)
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
