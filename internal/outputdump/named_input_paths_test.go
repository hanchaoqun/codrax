package outputdump

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC221NamedInputPathsAreExplicitBoundedAndSourceDistinct(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a", "same file.data"), filepath.Join(root, "b", "same file.data"), filepath.Join(root, "plain")}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a trace"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// The prefix of a quoted path can itself be a real file. It was not
	// independently named and must never consume a content-probe budget slot.
	if err := os.WriteFile(filepath.Join(root, "a", "same"), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	request := "inspect '" + paths[0] + "' and `b/same file.data` and ./plain ./plain missing.data trace_query"
	if got := NamedInputPathsFromRequest(request, root, 8); !reflect.DeepEqual(got, paths) {
		t.Fatalf("sources: %q want %q", got, paths)
	}
	if got := NamedInputPathsFromRequest(request, root, 2); !reflect.DeepEqual(got, paths[:2]) {
		t.Fatalf("bounded sources: %q", got)
	}
	if got := NamedInputPathsFromRequest("trace_query runtime measurements plain", root, 8); len(got) != 0 {
		t.Fatalf("prose invented a path: %q", got)
	}
}
