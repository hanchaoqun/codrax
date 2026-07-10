package ground

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SEC #26 必修② pin (grounding lane): the ground file reader consults the
// shared sensitive-config authority — a model-supplied evidence file naming
// the live credential store is never read into grounding context; the error
// surfaces through the existing Ungrounded-note lane. A normal repo file
// still reads.
func TestReadRepoFileRefusesCredentialFile(t *testing.T) {
	repo := t.TempDir()
	providers := filepath.Join(repo, "providers.yaml")
	if err := os.WriteFile(providers, []byte("api_key: fake-ground-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	types.SetSensitiveConfigFilePaths([]string{providers})
	t.Cleanup(func() { types.SetSensitiveConfigFilePaths(nil) })

	gc := &Context{RepoRoot: repo}
	body, err := readRepoFile(gc, "providers.yaml")
	if err == nil {
		t.Fatalf("credential file read must fail, got body %q", string(body))
	}
	if strings.Contains(err.Error(), repo) {
		t.Fatalf("refusal error must not echo the resolved path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("refusal error must carry the generic caveat, got: %v", err)
	}

	body, err = readRepoFile(gc, "main.go")
	if err != nil || !strings.Contains(string(body), "package main") {
		t.Fatalf("normal repo file must still read: body=%q err=%v", string(body), err)
	}
}
