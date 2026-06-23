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

type embeddedTraceStreamerManifest struct {
	SourceURL   string                          `json:"source_url"`
	UpstreamRef string                          `json:"upstream_ref"`
	LicenseID   string                          `json:"license_id"`
	ApprovalRef string                          `json:"approval_ref"`
	Platforms   []embeddedTraceStreamerPlatform `json:"platforms"`
}

type embeddedTraceStreamerPlatform struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type embeddedTraceStreamerResolution struct {
	Path        string
	Source      string
	Manifest    embeddedTraceStreamerManifest
	Platform    embeddedTraceStreamerPlatform
	CacheReused bool
}

var embeddedTraceStreamerAssetsFS = func() fs.FS { return nil }

func resolveEmbeddedTraceStreamerTool() (string, string, []string) {
	fsys := embeddedTraceStreamerAssetsFS()
	if fsys == nil {
		return "", "", nil
	}
	resolution, err := extractEmbeddedTraceStreamer(fsys, embeddedTraceStreamerCacheRoot())
	if err != nil {
		return "", "", []string{fmt.Sprintf("embedded trace_streamer is not usable: %v", err)}
	}
	return resolution.Path, resolution.Source, nil
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
	if goos != "windows" && info.Mode()&0o111 == 0 {
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

func embeddedTraceStreamerCacheRoot() string {
	if cache := strings.TrimSpace(os.Getenv("CODRAX_TRACE_STREAMER_CACHE")); cache != "" {
		return cache
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
