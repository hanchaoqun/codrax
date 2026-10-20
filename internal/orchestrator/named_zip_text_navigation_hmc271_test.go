package orchestrator

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Use the same complete raw trace and assertions as gzip. Transport format
// must not change the source universe, native query, or supplement window.
func TestHMC271NamedZIPTextNormalRunQueriesAndSupplementsExplicitWindow(t *testing.T) {
	const member = "session/capture.data"
	testNamedTextTransportNormalRun(t, func(t *testing.T) (string, []byte) {
		_, compressed := hmc268NamedGZIPText(t)
		r, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		w := zip.NewWriter(&archive)
		entry, err := w.Create(member)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(entry, r); err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "调度材料.zip")
		if err := os.WriteFile(path, archive.Bytes(), 0400); err != nil {
			t.Fatal(err)
		}
		return path, archive.Bytes()
	}, member)
}
