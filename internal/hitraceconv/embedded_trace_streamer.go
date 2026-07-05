package hitraceconv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	embeddedTraceStreamerDir          = "embedded_trace_streamer"
	embeddedTraceStreamerManifestName = "manifest.json"
)

// embeddedTraceStreamerBundledPlatformDirs is the approved embedded
// platform set (HED-59 user ruling 2026-07-05). Each entry is a
// per-platform payload directory under embedded_trace_streamer/
// carrying its own manifest.json plus exactly one binary, and each
// platform build embeds only its own directory (see the
// embedded_trace_streamer_payload_*.go embed_streamer stubs).
// darwin is intentionally absent from the first wave: the reference
// hmtrace darwin-aarch64 asset is a mislabeled x86_64 Mach-O.
// Growing this set requires an explicit ruling; the repository guard
// and payload ratchet tests pin it.
var embeddedTraceStreamerBundledPlatformDirs = map[string]bool{
	"windows-amd64": true,
	"linux-amd64":   true,
}

type embeddedTraceStreamerManifest struct {
	SourceURL   string                          `json:"source_url"`
	UpstreamRef string                          `json:"upstream_ref"`
	Version     string                          `json:"version"`
	LicenseID   string                          `json:"license_id"`
	ApprovalRef string                          `json:"approval_ref"`
	Platforms   []embeddedTraceStreamerPlatform `json:"platforms"`
}

type embeddedTraceStreamerPlatform struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	// SizeBytes and ActualFormat record the audited on-disk truth for
	// the committed payload: exact byte size plus the verbatim
	// `file <binary>` output. The reference assets ship with
	// unreliable directory labels, so the manifest must attest what
	// the bytes actually are, not what the upstream folder claims.
	SizeBytes    int64  `json:"size_bytes"`
	ActualFormat string `json:"actual_format"`
}

type embeddedTraceStreamerResolution struct {
	Path        string
	Source      string
	Manifest    embeddedTraceStreamerManifest
	Platform    embeddedTraceStreamerPlatform
	CacheReused bool
}

// Build-tag state. Slim builds (no -tags embed_streamer) keep the
// defaults: no assets, tag disabled, discovery stays external-only and
// resolveEmbeddedTraceStreamerTool is silent. The embed_streamer
// payload stubs (embedded_trace_streamer_payload_*.go) flip
// embeddedTraceStreamerTagEnabled and either install the platform
// embed.FS or record an explicit platform-gap reason so an
// embed_streamer build on a non-bundled platform reports structured
// unavailability instead of silently degrading.
var (
	embeddedTraceStreamerAssetsFS    = func() fs.FS { return nil }
	embeddedTraceStreamerTagEnabled  = false
	embeddedTraceStreamerPlatformGap = ""
)

// resolveEmbeddedTraceStreamerTool is the lowest-priority tier of the
// trace_streamer discovery chain. It runs only after explicit option,
// CODRAX_TRACE_STREAMER, executable-directory, PATH, and known-location
// discovery all missed, so a hit on any earlier tier never triggers an
// extraction. Extraction failures are fail-loud caveats: the caller
// surfaces them as structured tool status and conversion falls back to
// the built-in parser lane exactly as when no tool was discovered.
func resolveEmbeddedTraceStreamerTool() (string, string, []string) {
	fsys := embeddedTraceStreamerAssetsFS()
	if fsys == nil {
		if !embeddedTraceStreamerTagEnabled {
			return "", "", nil
		}
		gap := strings.TrimSpace(embeddedTraceStreamerPlatformGap)
		if gap == "" {
			// Defensive-only fallback: every embed_streamer payload stub
			// either installs an assets FS or sets the platform-gap
			// message, so this branch is unreachable in real builds. The
			// zh caveat mapping (LocalizeEmbeddedTraceStreamerCaveatZh)
			// intentionally does not cover this wording; if it ever
			// surfaces it passes through in English — the desired loud
			// signal that a payload stub broke its contract.
			gap = fmt.Sprintf("embedded trace_streamer payload is enabled by the embed_streamer build tag but exposes no assets for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		return "", "", []string{gap}
	}
	resolution, err := extractEmbeddedTraceStreamer(fsys, embeddedTraceStreamerCacheRoot())
	if err != nil {
		return "", "", []string{EmbeddedTraceStreamerNotUsableMessage(err)}
	}
	return resolution.Path, resolution.Source, nil
}

// EmbeddedTraceStreamerNotUsableMessage is the single production source
// of the extraction-failure wording; its verbatim Chinese mapping lives
// in LocalizeEmbeddedTraceStreamerCaveatZh below and the lockstep pin
// exercises the pair through the real resolver output.
func EmbeddedTraceStreamerNotUsableMessage(err error) string {
	return fmt.Sprintf("embedded trace_streamer is not usable: %v", err)
}

// EmbeddedTraceStreamerPlatformGapMessage is the single production
// source of the unbundled-platform wording: the payload stub for
// non-bundled embed_streamer builds emits exactly this sentence, and
// LocalizeEmbeddedTraceStreamerCaveatZh below carries its verbatim
// Chinese mapping. English producer and Chinese mapping deliberately
// live side by side in one file, and
// TestEmbeddedTraceStreamerZhLocalizationLockstep pins that the mapping
// actually fires on this producer's output — editing the wording on
// either side alone turns that pin red.
func EmbeddedTraceStreamerPlatformGapMessage(goos, goarch string) string {
	return fmt.Sprintf(
		"embedded trace_streamer payload is enabled by the embed_streamer build tag but no binary is bundled for %s/%s; the first wave bundles linux-amd64 and windows-amd64 only, so install or configure an external trace_streamer",
		goos, goarch,
	)
}

// LocalizeEmbeddedTraceStreamerSourceZh renders embedded-tier tool
// source labels (embeddedTraceStreamerSource output, "embedded
// trace_streamer <ref>") in Chinese. Single shared authority consumed
// by both the CLI (cmd/trace_convert.go) and the REPL
// (internal/repl/messages.go); returns the input unchanged for labels
// it does not own.
func LocalizeEmbeddedTraceStreamerSourceZh(source string) string {
	trimmed := strings.TrimSpace(source)
	if !strings.Contains(strings.ToLower(trimmed), "embedded trace_streamer") {
		return trimmed
	}
	return strings.Replace(trimmed, "embedded trace_streamer", "内嵌 trace_streamer", 1)
}

// LocalizeEmbeddedTraceStreamerCaveatZh renders the embedded-tier
// discovery caveats (extraction failure, unbundled-platform gap) in
// Chinese. Single shared authority consumed by both the CLI and the
// REPL; returns the input unchanged for messages it does not own. The
// unbundled-platform arm mirrors EmbeddedTraceStreamerPlatformGapMessage
// verbatim — the lockstep pin keeps the pair honest.
func LocalizeEmbeddedTraceStreamerCaveatZh(message string) string {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "embedded trace_streamer is not usable"):
		return strings.Replace(trimmed, "embedded trace_streamer is not usable", "内嵌 trace_streamer 不可用", 1)
	case strings.Contains(lower, "embed_streamer build tag"):
		out := strings.Replace(trimmed, "embedded trace_streamer payload is enabled by the embed_streamer build tag but no binary is bundled for ", "embed_streamer 构建已启用内嵌 trace_streamer，但未内嵌 ", 1)
		return strings.Replace(out, "; the first wave bundles linux-amd64 and windows-amd64 only, so install or configure an external trace_streamer", " 平台的二进制；首批仅内嵌 linux-amd64 与 windows-amd64，请安装或配置外部 trace_streamer", 1)
	default:
		return trimmed
	}
}

func extractEmbeddedTraceStreamer(fsys fs.FS, cacheRoot string) (embeddedTraceStreamerResolution, error) {
	if fsys == nil {
		return embeddedTraceStreamerResolution{}, fmt.Errorf("embedded trace_streamer assets are not configured")
	}
	manifest, err := loadEmbeddedTraceStreamerManifestFS(fsys)
	if err != nil {
		return embeddedTraceStreamerResolution{}, err
	}
	if err := validateEmbeddedTraceStreamerManifestFields(manifest); err != nil {
		return embeddedTraceStreamerResolution{}, err
	}
	platform, ok := embeddedTraceStreamerPlatformForHost(manifest)
	if !ok {
		return embeddedTraceStreamerResolution{}, fmt.Errorf("embedded trace_streamer has no platform for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	body, cleanPath, err := readEmbeddedTraceStreamerAsset(fsys, platform)
	if err != nil {
		return embeddedTraceStreamerResolution{}, err
	}
	cacheRoot = strings.TrimSpace(cacheRoot)
	if cacheRoot == "" {
		cacheRoot = embeddedTraceStreamerCacheRoot()
	}
	target := embeddedTraceStreamerCachePath(cacheRoot, manifest, platform, cleanPath)
	if err := verifyEmbeddedTraceStreamerFileHash(target, platform.SHA256); err == nil {
		if err := chmodEmbeddedTraceStreamer(target, platform.GOOS); err != nil {
			return embeddedTraceStreamerResolution{}, err
		}
		return embeddedTraceStreamerResolution{
			Path:        target,
			Source:      embeddedTraceStreamerSource(manifest),
			Manifest:    manifest,
			Platform:    platform,
			CacheReused: true,
		}, nil
	}
	if err := writeEmbeddedTraceStreamerCache(target, body, platform.GOOS); err != nil {
		return embeddedTraceStreamerResolution{}, err
	}
	if err := verifyEmbeddedTraceStreamerFileHash(target, platform.SHA256); err != nil {
		return embeddedTraceStreamerResolution{}, err
	}
	return embeddedTraceStreamerResolution{
		Path:     target,
		Source:   embeddedTraceStreamerSource(manifest),
		Manifest: manifest,
		Platform: platform,
	}, nil
}

func loadEmbeddedTraceStreamerManifestFS(fsys fs.FS) (embeddedTraceStreamerManifest, error) {
	body, err := fs.ReadFile(fsys, embeddedTraceStreamerManifestName)
	if err != nil {
		return embeddedTraceStreamerManifest{}, fmt.Errorf("read %s: %w", embeddedTraceStreamerManifestName, err)
	}
	var manifest embeddedTraceStreamerManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return embeddedTraceStreamerManifest{}, fmt.Errorf("parse %s: %w", embeddedTraceStreamerManifestName, err)
	}
	return manifest, nil
}

func validateEmbeddedTraceStreamerManifest(root string) error {
	manifest, err := loadEmbeddedTraceStreamerManifestFS(os.DirFS(root))
	if err != nil {
		return err
	}
	if err := validateEmbeddedTraceStreamerManifestFields(manifest); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, platform := range manifest.Platforms {
		if err := validateEmbeddedTraceStreamerPlatformOnDisk(root, platform, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateEmbeddedTraceStreamerManifestFields(manifest embeddedTraceStreamerManifest) error {
	if strings.TrimSpace(manifest.SourceURL) == "" {
		return fmt.Errorf("source_url is required")
	}
	if strings.TrimSpace(manifest.UpstreamRef) == "" {
		return fmt.Errorf("upstream_ref is required")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if strings.TrimSpace(manifest.LicenseID) == "" {
		return fmt.Errorf("license_id is required")
	}
	if strings.TrimSpace(manifest.ApprovalRef) == "" {
		return fmt.Errorf("approval_ref is required")
	}
	if len(manifest.Platforms) == 0 {
		return fmt.Errorf("at least one platform binary is required")
	}
	seen := map[string]bool{}
	for _, platform := range manifest.Platforms {
		goos := strings.TrimSpace(platform.GOOS)
		goarch := strings.TrimSpace(platform.GOARCH)
		if goos == "" || goarch == "" {
			return fmt.Errorf("platform goos/goarch are required")
		}
		key := goos + "/" + goarch
		if seen[key] {
			return fmt.Errorf("duplicate embedded trace_streamer platform %s", key)
		}
		seen[key] = true
		if _, err := embeddedTraceStreamerCleanPath(platform.Path); err != nil {
			return err
		}
		if strings.TrimSpace(platform.SHA256) == "" {
			return fmt.Errorf("sha256 is required for embedded trace_streamer platform %s", key)
		}
		if platform.SizeBytes <= 0 {
			return fmt.Errorf("size_bytes is required for embedded trace_streamer platform %s", key)
		}
		if strings.TrimSpace(platform.ActualFormat) == "" {
			return fmt.Errorf("actual_format (verbatim `file` output) is required for embedded trace_streamer platform %s", key)
		}
	}
	return nil
}

func validateEmbeddedTraceStreamerPlatformOnDisk(root string, platform embeddedTraceStreamerPlatform, seen map[string]bool) error {
	goos := strings.TrimSpace(platform.GOOS)
	goarch := strings.TrimSpace(platform.GOARCH)
	if goos == "" || goarch == "" {
		return fmt.Errorf("platform goos/goarch are required")
	}
	key := goos + "/" + goarch
	if seen[key] {
		return fmt.Errorf("duplicate embedded trace_streamer platform %s", key)
	}
	seen[key] = true
	filePath, err := embeddedTraceStreamerPath(root, platform.Path)
	if err != nil {
		return err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("embedded trace_streamer binary %s is not readable: %w", platform.Path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("embedded trace_streamer binary %s is a directory", platform.Path)
	}
	if info.Size() != platform.SizeBytes {
		return fmt.Errorf("size mismatch for %s: got %d want %d", platform.Path, info.Size(), platform.SizeBytes)
	}
	// Exec-bit enforcement needs a host filesystem that has exec bits;
	// Windows hosts report synthetic modes, so the check only runs on
	// non-Windows hosts for non-Windows payloads.
	if goos != "windows" && runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return fmt.Errorf("embedded trace_streamer binary %s is not executable", platform.Path)
	}
	return verifyEmbeddedTraceStreamerFileHash(filePath, platform.SHA256)
}

func embeddedTraceStreamerPlatformForHost(manifest embeddedTraceStreamerManifest) (embeddedTraceStreamerPlatform, bool) {
	for _, platform := range manifest.Platforms {
		if strings.TrimSpace(platform.GOOS) == runtime.GOOS && strings.TrimSpace(platform.GOARCH) == runtime.GOARCH {
			return platform, true
		}
	}
	return embeddedTraceStreamerPlatform{}, false
}

func readEmbeddedTraceStreamerAsset(fsys fs.FS, platform embeddedTraceStreamerPlatform) ([]byte, string, error) {
	cleanPath, err := embeddedTraceStreamerCleanPath(platform.Path)
	if err != nil {
		return nil, "", err
	}
	info, err := fs.Stat(fsys, cleanPath)
	if err != nil {
		return nil, "", fmt.Errorf("embedded trace_streamer binary %s is not readable: %w", platform.Path, err)
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("embedded trace_streamer binary %s is a directory", platform.Path)
	}
	body, err := fs.ReadFile(fsys, cleanPath)
	if err != nil {
		return nil, "", fmt.Errorf("read embedded trace_streamer binary %s: %w", platform.Path, err)
	}
	if int64(len(body)) != platform.SizeBytes {
		return nil, "", fmt.Errorf("size mismatch for %s: got %d want %d", platform.Path, len(body), platform.SizeBytes)
	}
	got := embeddedTraceStreamerSHA256(body)
	want := strings.ToLower(strings.TrimSpace(platform.SHA256))
	if got != want {
		return nil, "", fmt.Errorf("sha256 mismatch for %s: got %s want %s", platform.Path, got, want)
	}
	return body, cleanPath, nil
}

func embeddedTraceStreamerCleanPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("embedded trace_streamer path is required")
	}
	if filepath.IsAbs(rel) || path.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("embedded trace_streamer path must be relative: %q", rel)
	}
	if strings.Contains(rel, ":") {
		return "", fmt.Errorf("embedded trace_streamer path must be relative: %q", rel)
	}
	clean := path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("embedded trace_streamer path must stay under %s: %q", embeddedTraceStreamerDir, rel)
	}
	if !fs.ValidPath(clean) {
		return "", fmt.Errorf("embedded trace_streamer path is not a valid slash path: %q", rel)
	}
	return clean, nil
}

func embeddedTraceStreamerPath(root, rel string) (string, error) {
	clean, err := embeddedTraceStreamerCleanPath(rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func verifyEmbeddedTraceStreamerFileHash(filePath, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return fmt.Errorf("sha256 is required for %s", filePath)
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	got := embeddedTraceStreamerSHA256(body)
	if got != want {
		return fmt.Errorf("sha256 mismatch for %s: got %s want %s", filePath, got, want)
	}
	return nil
}

func writeEmbeddedTraceStreamerCache(target string, body []byte, goos string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".trace_streamer-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := chmodEmbeddedTraceStreamer(tmpPath, goos); err != nil {
		return err
	}
	_ = os.Remove(target)
	return os.Rename(tmpPath, target)
}

func chmodEmbeddedTraceStreamer(filePath, goos string) error {
	if goos == "windows" {
		return nil
	}
	return os.Chmod(filePath, 0o755)
}

func embeddedTraceStreamerCachePath(cacheRoot string, manifest embeddedTraceStreamerManifest, platform embeddedTraceStreamerPlatform, cleanAssetPath string) string {
	hash := strings.ToLower(strings.TrimSpace(platform.SHA256))
	hashPrefix := hash
	if len(hashPrefix) > 16 {
		hashPrefix = hashPrefix[:16]
	}
	upstream := sanitizeEmbeddedTraceStreamerCachePart(manifest.UpstreamRef)
	if len(upstream) > 32 {
		upstream = upstream[:32]
	}
	platformDir := sanitizeEmbeddedTraceStreamerCachePart(platform.GOOS + "-" + platform.GOARCH)
	name := path.Base(cleanAssetPath)
	if strings.TrimSpace(name) == "" || name == "." || name == "/" {
		name = traceStreamerBinaryName()
	}
	return filepath.Join(cacheRoot, upstream, platformDir, hashPrefix, name)
}

// embeddedTraceStreamerConfiguredCacheRoot is the merged cache_dir
// from cmd/root.go (already anchored at runtimeAnchor per the path
// anchor rules). Empty means the CLI did not configure a cache dir, in
// which case the default matches the other ~/.codrax/cache consumers
// (repomap, env cache).
var embeddedTraceStreamerConfiguredCacheRoot string

// SetEmbeddedTraceStreamerCacheRoot points embedded trace_streamer
// extraction at <dir>/embedded-trace-streamer. Called once from
// cmd/root.go with the merged cache_dir right after
// repomap.SetCacheDir; an empty value keeps the ~/.codrax/cache
// default. Extraction stays multi-instance safe regardless of root:
// targets are keyed by upstream ref, platform, and payload hash, and
// writes go through a same-directory temp file plus rename.
func SetEmbeddedTraceStreamerCacheRoot(dir string) {
	embeddedTraceStreamerConfiguredCacheRoot = strings.TrimSpace(dir)
}

func embeddedTraceStreamerCacheRoot() string {
	if cache := strings.TrimSpace(os.Getenv("CODRAX_TRACE_STREAMER_CACHE")); cache != "" {
		return cache
	}
	if embeddedTraceStreamerConfiguredCacheRoot != "" {
		return filepath.Join(embeddedTraceStreamerConfiguredCacheRoot, "embedded-trace-streamer")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".codrax", "cache", "embedded-trace-streamer")
	}
	if cache, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cache) != "" {
		return filepath.Join(cache, "codrax", "embedded-trace-streamer")
	}
	return filepath.Join(os.TempDir(), "codrax", "embedded-trace-streamer")
}

func embeddedTraceStreamerSource(manifest embeddedTraceStreamerManifest) string {
	ref := strings.TrimSpace(manifest.UpstreamRef)
	if ref == "" {
		return "embedded trace_streamer"
	}
	return "embedded trace_streamer " + ref
}

func sanitizeEmbeddedTraceStreamerCachePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "unknown"
	}
	return out
}

func embeddedTraceStreamerSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
