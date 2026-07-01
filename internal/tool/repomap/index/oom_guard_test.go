package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestHashFileStreamsIdenticalToWholeFileHash pins that the streaming hashFile
// produces byte-identical output to the previous whole-file
// contentHash(os.ReadFile(...)) implementation, so switching to streaming does
// not invalidate existing cache hashes.
func TestHashFileStreamsIdenticalToWholeFileHash(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content []byte
	}{
		{"empty", []byte("")},
		{"small", []byte("package main\nfunc main() {}\n")},
		// A few MiB, larger than io.Copy's internal buffer so multiple chunks
		// are hashed — the case that would differ if streaming were wrong.
		{"multichunk", []byte(strings.Repeat("abcdefghij0123456789", 300000))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name+".txt")
			if err := os.WriteFile(path, c.content, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := hashFile(path)
			if err != nil {
				t.Fatalf("hashFile error: %v", err)
			}
			want := contentHash(c.content)
			if got != want {
				t.Fatalf("streaming hash %q != whole-file hash %q", got, want)
			}
		})
	}
}

// TestParseOneFileSkipsOversizeFileWithoutReadingWhole confirms that a file
// larger than maxParseSourceBytes is stream-hashed but not deep-parsed, so a
// pathologically large file cannot OOM the parse stage or produce symbols.
func TestParseOneFileSkipsOversizeFileWithoutReadingWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.go")
	// One byte over the cap is enough to exercise the guard without allocating
	// hundreds of MB in the test.
	big := make([]byte, maxParseSourceBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := FileEntry{
		RelPath:  "huge.go",
		AbsPath:  path,
		Language: types.LangGo,
		Size:     int64(len(big)),
	}
	fi := parseOneFile(entry)
	if fi == nil {
		t.Fatal("parseOneFile returned nil for oversize file")
	}
	if fi.Hash == "" {
		t.Fatalf("oversize file must still be hashed for change detection: %+v", fi)
	}
	if want := hashFileMust(t, path); fi.Hash != want {
		t.Fatalf("oversize file hash %q != streamed hash %q", fi.Hash, want)
	}
	if len(fi.Symbols) != 0 {
		t.Fatalf("oversize file must not be deep-parsed into symbols, got %d", len(fi.Symbols))
	}
	if fi.Size != entry.Size || fi.RelPath != entry.RelPath || fi.Language != entry.Language {
		t.Fatalf("oversize file must keep identity metadata: %+v", fi)
	}
}

// TestParseOneFileStillParsesFileAtOrBelowCap ensures the guard does not
// suppress symbol extraction for a normal (sub-cap) source file.
func TestParseOneFileStillParsesFileAtOrBelowCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.go")
	src := "package main\n\nfunc Alpha() {}\nfunc Beta() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := FileEntry{
		RelPath:  "small.go",
		AbsPath:  path,
		Language: types.LangGo,
		Size:     int64(len(src)),
	}
	// Use the real ParseFiles entry so the tree-sitter pipeline runs.
	infos := ParseFiles([]FileEntry{entry}, dir)
	if len(infos) != 1 || infos[0] == nil || infos[0].Hash == "" {
		t.Fatalf("normal file should parse and hash: %+v", infos)
	}
	if len(infos[0].Symbols) == 0 {
		t.Fatalf("normal sub-cap source file should still yield symbols: %+v", infos[0])
	}
}

func hashFileMust(t *testing.T, path string) string {
	t.Helper()
	h, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile error: %v", err)
	}
	return h
}
