package attachment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestTraceMaterialSinglePhysicalSourceIsNotPreviewCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name     string
		samePath bool
		members  int
		want     bool
	}{
		{"bounded single text", true, 1, true},
		{"converted material", false, 2, false},
		{"bundle members", true, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			query := source
			bindings := map[string]filegeneration.Identity{}
			for i := 0; i < tc.members; i++ {
				p := filepath.Join(dir, []string{"source", "query", "member"}[i])
				if err := os.WriteFile(p, []byte("full material larger than preview\n"), 0600); err != nil {
					t.Fatal(err)
				}
				id, err := filegeneration.FromPath(p)
				if err != nil {
					t.Fatal(err)
				}
				bindings[p] = id
			}
			if !tc.samePath {
				query = filepath.Join(dir, "query")
			}
			preview := "# codrax-source: " + query + "\n# bounded preview\n"
			m, err := BindTraceMaterial(source, query, preview, bindings)
			if err != nil {
				t.Fatal(err)
			}
			if m.SinglePhysicalSource() != tc.want || m.SelfContainedText() {
				t.Fatalf("origin/completeness conflated: %+v", m)
			}
			encoded, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != "{}" {
				t.Fatalf("exported private origin: %s", encoded)
			}
			var restored TraceMaterial
			if err = json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.SinglePhysicalSource() {
				t.Fatal("restored origin from JSON")
			}
			if err = os.WriteFile(source, []byte("drift"), 0600); err != nil {
				t.Fatal(err)
			}
			if m.Validate(context.Background(), preview) == nil {
				t.Fatal("origin accepted stale material")
			}
		})
	}
	var absent *TraceMaterial
	if absent.SinglePhysicalSource() {
		t.Fatal("nil origin")
	}
}
