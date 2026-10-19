package attachment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC263PreparedQueryArtifactsAreExactPrivateAndGenerationBound(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "capture.zip"), filepath.Join(dir, "capture.tracebundle.json"), filepath.Join(dir, "events.systrace"), filepath.Join(dir, "receipt.json")}
	bindings := map[string]filegeneration.Identity{}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("first generation\n"), 0600); err != nil {
			t.Fatal(err)
		}
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	m, err := BindTraceMaterial(paths[0], paths[1], "# preview\n", bindings)
	if err != nil {
		t.Fatal(err)
	}
	if m.OwnsQueryArtifact(paths[2]) {
		t.Fatal("all generation bindings became query leaves")
	}
	selected := []string{paths[2]}
	sealed, err := m.WithQueryArtifacts(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}
	selected[0] = paths[3]
	if !sealed.OwnsQueryArtifact(paths[1]) || !sealed.OwnsQueryArtifact(paths[2]) {
		t.Fatal("admitted manifest/leaf source join lost")
	}
	for _, path := range []string{paths[0], paths[3], filepath.Join(dir, "neighbor.systrace"), "events.systrace"} {
		if sealed.OwnsQueryArtifact(path) {
			t.Fatalf("unselected source accepted: %s", path)
		}
	}
	if sealed.MatchesPath(paths[2]) || sealed.MatchesOriginalSource(paths[2]) {
		t.Fatal("leaf ownership created navigation alias")
	}
	for _, path := range []string{paths[0], filepath.Join(dir, "missing"), "events.systrace"} {
		if _, err := m.WithQueryArtifacts(t.Context(), []string{path}); err == nil {
			t.Fatalf("unbound/transport leaf accepted: %s", path)
		}
	}
	data, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	var restored TraceMaterial
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.OwnsQueryArtifact(paths[2]) {
		t.Fatal("JSON restored private source ownership")
	}
	if err := os.WriteFile(paths[2], []byte("other generation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if sealed.OwnsQueryArtifact(paths[2]) || sealed.OwnsQueryArtifact(paths[1]) {
		t.Fatal("changed leaf retained source join")
	}
	var nilMaterial *TraceMaterial
	if _, err := nilMaterial.WithQueryArtifacts(t.Context(), nil); err == nil {
		t.Fatal("nil material accepted")
	}
}
