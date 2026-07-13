package hitraceconv

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestReleaseEmbeddedTraceStreamerCacheSymlinkFailsLoudWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating an unprivileged Windows symlink is environment-dependent")
	}
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	body := []byte("verified-embedded-trace-streamer")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, body)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, body, 0o444)
	cacheRoot := t.TempDir()
	target := embeddedTraceStreamerCachePath(cacheRoot, manifest, manifest.Platforms[0], binaryRel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(cacheRoot, "external-owner")
	if err := os.WriteFile(victim, body, 0o600); err != nil {
		t.Fatal(err)
	}
	victimBefore, err := os.Lstat(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, target); err != nil {
		t.Fatal(err)
	}
	linkBefore, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}

	_, err = extractEmbeddedTraceStreamer(fsys, cacheRoot)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("cache symlink did not fail loud before reuse/chmod: %v", err)
	}
	victimAfter, err := os.Lstat(victim)
	if err != nil {
		t.Fatal(err)
	}
	linkAfter, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(victimBefore, victimAfter) || victimBefore.Mode() != victimAfter.Mode() || !bytes.Equal(got, body) {
		t.Fatalf("cache symlink target changed: same_identity=%t before_mode=%s after_mode=%s got=%q", os.SameFile(victimBefore, victimAfter), victimBefore.Mode(), victimAfter.Mode(), got)
	}
	if linkAfter.Mode()&os.ModeSymlink == 0 || !os.SameFile(linkBefore, linkAfter) {
		t.Fatalf("cache symlink identity changed: before=%s after=%s", linkBefore.Mode(), linkAfter.Mode())
	}
	assertTarget, err := os.Readlink(target)
	if err != nil || assertTarget != victim {
		t.Fatalf("cache symlink changed: target=%q err=%v", assertTarget, err)
	}
	releaseAssertNoEmbeddedCacheTemps(t, filepath.Dir(target))
}

func TestReleaseEmbeddedTraceStreamerCacheReusesValidCanonicalIdentity(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cache", traceStreamerBinaryName())
	body := []byte("verified-cache-payload")
	if err := writeEmbeddedTraceStreamerCache(target, body, runtime.GOOS); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeEmbeddedTraceStreamerCache(target, body, runtime.GOOS); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("valid canonical cache target was replaced instead of reused")
	}
	releaseAssertNoEmbeddedCacheTemps(t, filepath.Dir(target))
}

// A mismatched target created after the initial absent check is an external
// racing owner. The all-platform no-replace primitive must fail without
// changing its identity, bytes, or mode.
func TestReleaseEmbeddedTraceStreamerCacheRaceNeverOverwritesMismatchedCanonical(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cache", traceStreamerBinaryName())
	body := []byte("verified-cache-payload")
	external := []byte("external-cache-owner!!")
	if len(body) != len(external) {
		t.Fatal("test fixture must use equal sizes")
	}
	originalPublish := embeddedTraceStreamerCachePublishNoReplace
	var externalBefore os.FileInfo
	embeddedTraceStreamerCachePublishNoReplace = func(stagingPath, finalPath string) (bool, error) {
		if err := os.WriteFile(finalPath, external, 0o600); err != nil {
			return false, err
		}
		info, err := os.Lstat(finalPath)
		if err != nil {
			return false, err
		}
		externalBefore = info
		return originalPublish(stagingPath, finalPath)
	}
	t.Cleanup(func() { embeddedTraceStreamerCachePublishNoReplace = originalPublish })

	err := writeEmbeddedTraceStreamerCache(target, body, runtime.GOOS)
	if err == nil {
		t.Fatal("mismatched racing cache owner was overwritten instead of failing loud")
	}
	after, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if externalBefore == nil || !os.SameFile(externalBefore, after) || externalBefore.Mode() != after.Mode() || !bytes.Equal(got, external) {
		t.Fatalf("racing cache owner changed: same_identity=%t before_mode=%v after_mode=%s got=%q want=%q err=%v",
			externalBefore != nil && os.SameFile(externalBefore, after), func() os.FileMode {
				if externalBefore == nil {
					return 0
				}
				return externalBefore.Mode()
			}(), after.Mode(), got, external, err)
	}
	releaseAssertNoEmbeddedCacheTemps(t, filepath.Dir(target))
}

func TestReleaseEmbeddedTraceStreamerCacheConcurrentPublicationHasNoMissingWindow(t *testing.T) {
	target := filepath.Join(t.TempDir(), "cache", traceStreamerBinaryName())
	body := bytes.Repeat([]byte("codrax-trace-streamer-cache\n"), 2048)
	if err := writeEmbeddedTraceStreamerCache(target, body, runtime.GOOS); err != nil {
		t.Fatalf("seed embedded cache: %v", err)
	}

	watchErr := make(chan error, 1)
	stop := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := os.ReadFile(target)
			if err != nil {
				select {
				case watchErr <- fmt.Errorf("canonical cache path became unreadable: %w", err):
				default:
				}
				return
			}
			if !bytes.Equal(got, body) {
				select {
				case watchErr <- fmt.Errorf("canonical cache exposed partial/foreign bytes: got=%d want=%d", len(got), len(body)):
				default:
				}
				return
			}
			runtime.Gosched()
		}
	}()
	<-started

	writerErr := make(chan error, 1)
	var writers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for iteration := 0; iteration < 8; iteration++ {
				if err := writeEmbeddedTraceStreamerCache(target, body, runtime.GOOS); err != nil {
					select {
					case writerErr <- err:
					default:
					}
					return
				}
			}
		}()
	}
	writers.Wait()
	close(stop)
	<-done

	select {
	case err := <-writerErr:
		t.Fatalf("concurrent embedded cache writer failed: %v", err)
	default:
	}
	select {
	case err := <-watchErr:
		t.Fatal(err)
	default:
	}
	if err := verifyEmbeddedTraceStreamerFileHash(target, embeddedTraceStreamerSHA256(body)); err != nil {
		t.Fatalf("final embedded cache hash: %v", err)
	}
	releaseAssertNoEmbeddedCacheTemps(t, filepath.Dir(target))
}

func TestReleaseEmbeddedTraceStreamerCacheConcurrentColdExtractionConverges(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("#!/bin/sh\nexit 0\n")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	cacheRoot := t.TempDir()

	type outcome struct {
		resolution embeddedTraceStreamerResolution
		err        error
	}
	outcomes := make(chan outcome, 24)
	var extractors sync.WaitGroup
	for worker := 0; worker < cap(outcomes); worker++ {
		extractors.Add(1)
		go func() {
			defer extractors.Done()
			resolution, err := extractEmbeddedTraceStreamer(fsys, cacheRoot)
			outcomes <- outcome{resolution: resolution, err: err}
		}()
	}
	extractors.Wait()
	close(outcomes)

	canonicalPath := ""
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent cold extraction failed: %v", outcome.err)
		}
		if canonicalPath == "" {
			canonicalPath = outcome.resolution.Path
		}
		if outcome.resolution.Path != canonicalPath {
			t.Fatalf("concurrent extraction diverged: got=%q want=%q", outcome.resolution.Path, canonicalPath)
		}
	}
	if canonicalPath == "" {
		t.Fatal("concurrent extraction returned no canonical path")
	}
	if err := verifyEmbeddedTraceStreamerFileHash(canonicalPath, manifest.Platforms[0].SHA256); err != nil {
		t.Fatalf("cold extraction final hash: %v", err)
	}
	releaseAssertNoEmbeddedCacheTemps(t, filepath.Dir(canonicalPath))
}

// Functional concurrency guards above exercise the publication protocol. This
// structural pin makes the no-missing-window invariant deterministic: the
// canonical target must never be deleted before no-replace publication.
func TestReleaseEmbeddedTraceStreamerCachePublisherNeverDeletesCanonicalTarget(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate release cache contract test")
	}
	production := filepath.Join(filepath.Dir(currentFile), "embedded_trace_streamer.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), production, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "writeEmbeddedTraceStreamerCache" {
			return true
		}
		found = true
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, pkgOK := selector.X.(*ast.Ident)
			arg, argOK := call.Args[0].(*ast.Ident)
			if pkgOK && argOK && pkg.Name == "os" &&
				(selector.Sel.Name == "Remove" || selector.Sel.Name == "RemoveAll") && arg.Name == "target" {
				t.Errorf("writeEmbeddedTraceStreamerCache deletes canonical target before publication via os.%s(target)", selector.Sel.Name)
			}
			return true
		})
		return false
	})
	if !found {
		t.Fatal("writeEmbeddedTraceStreamerCache production function not found")
	}
}

func releaseAssertNoEmbeddedCacheTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".trace_streamer-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("embedded cache staging files leaked: %v", matches)
	}
}
