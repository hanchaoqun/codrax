package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC251ImmutableBlobFullHashAndExactBytes(t *testing.T) {
	dir := t.TempDir()
	// A real SHA8 collision, not a mocked hash. Truncated names must not merge
	// these byte-distinct payloads or overwrite the first physical generation.
	a, b := "native payload 107400", "native payload 116764"
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	if !bytes.Equal(ha[:4], hb[:4]) || ha == hb {
		t.Fatal("fixture is not a short-hash collision")
	}
	first := StoreBlobArtifact(dir, "trace_query", "native.json", a)
	before, err := filegeneration.FromPath(first)
	if err != nil {
		t.Fatal(err)
	}
	second := StoreBlobArtifact(dir, "trace_query", "native.json", b)
	if first == "" || second == "" || first == second {
		t.Fatal("byte-distinct results share a path")
	}
	for _, item := range []struct {
		path, body string
		hash       [32]byte
	}{{first, a, ha}, {second, b, hb}} {
		data, err := os.ReadFile(item.path)
		if err != nil || string(data) != item.body || !strings.Contains(filepath.Base(item.path), hex.EncodeToString(item.hash[:])) {
			t.Fatalf("lost full content address: %s %v", item.path, err)
		}
	}
	if got := StoreBlobArtifact(dir, "trace_query", "native.json", a); got != first {
		t.Fatal("same content lost address")
	}
	after, err := filegeneration.FromPath(first)
	if err != nil || !before.SameVersion(after) {
		t.Fatal("identical content rewrote file generation")
	}
	long := StoreBlobArtifact(dir, "trace_query", strings.Repeat("name", 100)+".json", a)
	if long == "" || len(filepath.Base(long)) > 255 {
		t.Fatal("full hash broke long stem storage")
	}
}

func TestHMC251ImmutableBlobConcurrentPublish(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("完整原生结果;", 8192)
	type result struct {
		path       string
		generation filegeneration.Identity
		err        error
	}
	got := make(chan result, 24)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < cap(got); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			path := StoreBlobArtifact(dir, "trace_query", "native.json", body)
			generation, err := filegeneration.FromPath(path)
			got <- result{path, generation, err}
		}()
	}
	close(start)
	wg.Wait()
	close(got)
	var first string
	for item := range got {
		if item.err != nil || item.path == "" {
			t.Fatalf("publication: %v", item.err)
		}
		if first == "" {
			first = item.path
		}
		if item.path != first {
			t.Fatal("parallel result paths diverged")
		}
		current, err := filegeneration.FromPath(first)
		if err != nil || !item.generation.SameVersion(current) {
			t.Fatal("parallel publication changed a returned generation")
		}
	}
	data, err := os.ReadFile(first)
	if err != nil || string(data) != body {
		t.Fatal("published partial bytes")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging leaked: %v %v", entries, err)
	}
}

func TestHMC251ImmutableBlobConflictNeverRepairsOrFollows(t *testing.T) {
	for _, kind := range []string{"different_bytes", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			body := "native good bytes"
			leaf := immutableTraceResultName("native.json", body)
			path := filepath.Join(dir, leaf)
			switch kind {
			case "different_bytes":
				if err := os.WriteFile(path, []byte("native evil bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if ref := StoreBlobArtifact(dir, "trace_query", "native.json", body); ref != "" {
				t.Fatalf("conflicting object reused: %s", ref)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || !filegeneration.FromInfo(before).SameVersion(filegeneration.FromInfo(after)) {
				t.Fatal("existing object replaced")
			}
			if kind == "different_bytes" {
				data, _ := os.ReadFile(path)
				if string(data) != "native evil bytes" {
					t.Fatal("foreign bytes repaired")
				}
			}
		})
	}
}

func TestHMC251ImmutableBlobNoReplaceAtPublicationBoundary(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(stage, []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("external winner"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = immutableBlobRenameNoReplace(f, dir, "stage", "target"); err == nil {
		t.Fatal("replaced an external winner")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "external winner" {
		t.Fatal("target corrupted")
	}
}

func TestHMC251ImmutableBlobRawAndLegacyIsolation(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("original line\n", MaxInlineBytes)
	ctx := &types.BusContext{WorkDir: dir}
	preview, ref := StoreBlob(ctx, "trace_query", body)
	before, err := filegeneration.FromPath(ref)
	if err != nil {
		t.Fatal(err)
	}
	nextPreview, next := StoreBlob(ctx, "trace_query", body)
	after, err := filegeneration.FromPath(ref)
	if err != nil || ref != next || preview != nextPreview || !before.SameVersion(after) {
		t.Fatal("raw reference was rewritten")
	}
	if !strings.Contains(preview, ref) {
		t.Fatal("raw reference navigation lost")
	}
	inline, ref := StoreBlob(ctx, "trace_query", "short")
	if inline != "short" || ref != "" {
		t.Fatal("inline behavior changed")
	}
	blank := strings.Repeat(" ", MaxInlineBytes+1)
	_, ref = StoreBlob(ctx, "trace_query", blank)
	if data, err := os.ReadFile(ref); err != nil || string(data) != blank {
		t.Fatal("raw whitespace bytes were discarded")
	}
	if StoreBlobArtifact(dir, "trace_query", "blank.json", blank) != "" {
		t.Fatal("structured artifact blank rejection changed")
	}
	legacy := StoreBlobArtifact(dir, "read_file", "legacy.txt", "initial")
	if err := os.WriteFile(legacy, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	if StoreBlobArtifact(dir, "read_file", "legacy.txt", "initial") != legacy {
		t.Fatal("legacy address changed")
	}
	data, _ := os.ReadFile(legacy)
	if string(data) != "initial" {
		t.Fatal("non-trace legacy repair policy changed")
	}
}
