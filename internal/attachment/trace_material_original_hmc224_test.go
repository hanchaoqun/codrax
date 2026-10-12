package attachment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC224PreparedOriginalAliasRequiresSameGeneration(t *testing.T) {
	root := t.TempDir()
	original, derived, alias, other := filepath.Join(root, "original"), filepath.Join(root, "query"), filepath.Join(root, "alias"), filepath.Join(root, "other")
	body := "# tracer: nop\n"
	bindings := map[string]filegeneration.Identity{}
	for _, path := range []string{original, derived, other} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if path != other {
			bindings[path] = id
		}
	}
	m, err := BindTraceMaterial(original, derived, body, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	if !m.MatchesOriginalSource(alias) {
		t.Fatal("current original alias rejected")
	}
	if m.MatchesOriginalSource(derived) || m.MatchesOriginalSource(other) {
		t.Fatal("derived or equal bytes promoted to original input")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if m.MatchesOriginalSource(alias) {
		t.Fatal("retargeted alias accepted")
	}
	if err := os.Rename(other, original); err != nil {
		t.Fatal(err)
	}
	if m.MatchesOriginalSource(original) {
		t.Fatal("replaced source accepted")
	}
}
