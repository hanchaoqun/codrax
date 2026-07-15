package tracebundle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenDecodeValidateAndClose(t *testing.T) {
	path := writeManifest(t, `{"artifacts":[],"future":{"accepted":true}}`)
	snapshot, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	type manifest struct {
		Artifacts []struct{} `json:"artifacts"`
	}
	var decoded manifest
	if err := snapshot.Decode(&decoded); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.Artifacts == nil || len(decoded.Artifacts) != 0 {
		t.Fatalf("decoded artifacts = %#v", decoded.Artifacts)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path() != filepath.Clean(abs) {
		t.Fatalf("Path = %q, want %q", snapshot.Path(), filepath.Clean(abs))
	}
	if snapshot.Size() != int64(len(`{"artifacts":[],"future":{"accepted":true}}`)) {
		t.Fatalf("Size = %d", snapshot.Size())
	}
	if !snapshot.Identity().Strong() {
		t.Fatal("snapshot identity is not strong")
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := snapshot.Validate(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Validate after Close = %v", err)
	}
	if err := snapshot.Decode(&decoded); !errors.Is(err, ErrClosed) {
		t.Fatalf("Decode after Close = %v", err)
	}
}

func TestOpenRejectsNonRegularAndOversizedBeforeDecode(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "oversized.tracebundle.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxManifestBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestOpenAcceptsExactByteCapAndReaderRejectsCapPlusOne(t *testing.T) {
	exactPath := filepath.Join(t.TempDir(), "exact.tracebundle.json")
	data := make([]byte, MaxManifestBytes)
	copy(data, `{}`)
	for i := 2; i < len(data); i++ {
		data[i] = ' '
	}
	if err := os.WriteFile(exactPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Open(context.Background(), exactPath)
	if err != nil {
		t.Fatalf("exact cap Open: %v", err)
	}
	if snapshot.Size() != MaxManifestBytes {
		t.Fatalf("exact cap Size = %d", snapshot.Size())
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}

	overPath := filepath.Join(t.TempDir(), "held-cap-plus-one")
	file, err := os.Create(overPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxManifestBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := readManifest(context.Background(), file, MaxManifestBytes, overPath); !errors.Is(err, ErrTooLarge) {
		_ = file.Close()
		t.Fatalf("held cap+1 error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPreservesContextCancellation(t *testing.T) {
	path := writeManifest(t, `{}`)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(canceled, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error = %v", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer stop()
	if _, err := Open(expired, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired-deadline error = %v", err)
	}

	largePath := filepath.Join(t.TempDir(), "large.tracebundle.json")
	large := `{"padding":"` + strings.Repeat("x", manifestReadChunk*3) + `"}`
	if err := os.WriteFile(largePath, []byte(large), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterChecksContext{cancelAt: 5}
	if _, err := Open(ctx, largePath); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-read cancellation error = %v (checks=%d)", err, ctx.checks.Load())
	}
}

func TestOpenRunsEnvelopePreflight(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{} {}`),
		[]byte(`{"A":1,"a":2}`),
		{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
	} {
		path := filepath.Join(t.TempDir(), "invalid.tracebundle.json")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if snapshot, err := Open(context.Background(), path); !errors.Is(err, ErrInvalidManifest) || snapshot != nil {
			t.Fatalf("Open(%q) = snapshot=%v error=%v", body, snapshot, err)
		}
	}
}

func TestValidateRejectsHeldRewriteWithRestoredMtime(t *testing.T) {
	path := writeManifest(t, `{"value":"a"}`)
	snapshot, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer snapshot.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"value":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); !errors.Is(err, ErrGenerationChanged) {
		t.Fatalf("Validate after same-size rewrite = %v", err)
	}
}

func TestValidateRejectsAtomicPathReplacementAndRemoval(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		path := writeManifest(t, `{"value":"a"}`)
		snapshot, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer snapshot.Close()
		replacement := filepath.Join(filepath.Dir(path), "replacement.json")
		if err := os.WriteFile(replacement, []byte(`{"value":"a"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
		if err := snapshot.Validate(); !errors.Is(err, ErrGenerationChanged) {
			t.Fatalf("Validate after replacement = %v", err)
		}
	})

	t.Run("removal", func(t *testing.T) {
		path := writeManifest(t, `{}`)
		snapshot, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer snapshot.Close()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		err = snapshot.Validate()
		if !errors.Is(err, ErrGenerationChanged) {
			t.Fatalf("Validate after removal = %v", err)
		}
	})
}

func TestValidateRejectsSymlinkRetarget(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	link := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(first, []byte(`{"value":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"value":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	snapshot, err := Open(context.Background(), link)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer snapshot.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); !errors.Is(err, ErrGenerationChanged) {
		t.Fatalf("Validate after symlink retarget = %v", err)
	}
}

func TestDecodeReportsTypedSchemaFailure(t *testing.T) {
	path := writeManifest(t, `{"artifacts":"wrong-shape"}`)
	snapshot, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer snapshot.Close()
	var dst struct {
		Artifacts []string `json:"artifacts"`
	}
	if err := snapshot.Decode(&dst); err == nil || !strings.HasPrefix(err.Error(), "tracebundle manifest decode:") {
		t.Fatalf("Decode error = %v", err)
	}
}

type cancelAfterChecksContext struct {
	checks   atomic.Int32
	cancelAt int32
}

func (c *cancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterChecksContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterChecksContext) Value(any) any               { return nil }
func (c *cancelAfterChecksContext) Err() error {
	if c.checks.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.tracebundle.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func repeatedArray(elements int) string {
	if elements == 0 {
		return "[]"
	}
	return "[" + strings.Repeat("0,", elements-1) + "0]"
}

func objectWithMembers(members int) string {
	var builder strings.Builder
	builder.WriteByte('{')
	for i := 0; i < members; i++ {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, "%q:0", fmt.Sprintf("k%d", i))
	}
	builder.WriteByte('}')
	return builder.String()
}
