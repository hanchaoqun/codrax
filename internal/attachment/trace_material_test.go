package attachment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestTraceMaterialBindsAllGenerationsAndImmutablePreview(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "original"), filepath.Join(dir, "query"), filepath.Join(dir, "member")}
	bindings := make(map[string]filegeneration.Identity)
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("first generation"), 0600); err != nil {
			t.Fatal(err)
		}
		var err error
		bindings[p], err = filegeneration.FromPath(p)
		if err != nil {
			t.Fatal(err)
		}
	}
	preview := "# codrax-source: " + paths[1] + "\n# preview only\n"
	m, err := BindTraceMaterial(paths[0], paths[1], preview, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(context.Background(), preview+"changed"); err == nil {
		t.Fatal("accepted different preview")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Validate(ctx, preview); err != context.Canceled {
		t.Fatalf("cancel = %v", err)
	}
	// A same-length write with restored mtime still changes the physical generation.
	stat, err := os.Stat(paths[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[2], []byte("other generation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(paths[2], stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	bindings[paths[2]], err = filegeneration.FromPath(paths[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(context.Background(), preview); err == nil {
		t.Fatal("caller map mutation or same-size/mtime rewrite laundered changed member")
	}
}

func TestTraceMaterialCannotBeMintedBySerializedFields(t *testing.T) {
	var m TraceMaterial
	if err := json.Unmarshal([]byte(`{"sourcePath":"/x","queryPath":"/x","preview":"text","bindings":{},"selfContainedText":true,"SelfContainedText":true}`), &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(context.Background(), "text"); err == nil {
		t.Fatal("JSON minted attachment receipt")
	}
	if m.SelfContainedText() {
		t.Fatal("JSON minted complete-text snapshot permission")
	}
	var missing *TraceMaterial
	if err := missing.Validate(context.Background(), "text"); err == nil {
		t.Fatal("nil receipt accepted")
	}
}

func TestTraceMaterialCompleteTextFactoryIsExplicitAndExact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.sys")
	body := "# codrax-preview: truncated; this literal is only source text\ncomplete source\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]filegeneration.Identity{path: id}
	preview := "# codrax-source: " + path + "\n" + body
	ordinary, err := BindTraceMaterial(path, path, preview, bindings)
	if err != nil || ordinary.SelfContainedText() {
		t.Fatalf("default factory granted completeness: %v %v", ordinary, err)
	}
	complete, err := BindCompleteTextTraceMaterial(path, preview, bindings)
	if err != nil || !complete.SelfContainedText() {
		t.Fatalf("complete factory rejected exact text because of body prose: %v %v", complete, err)
	}
	if _, err := BindCompleteTextTraceMaterial(path, preview[:len(preview)-1], bindings); err == nil {
		t.Fatal("short body minted complete-text permission")
	}
	encoded, err := json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	var restored TraceMaterial
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.SelfContainedText() {
		t.Fatalf("serialization retained authority: %s %v", encoded, err)
	}
}
