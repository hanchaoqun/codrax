package tracecatalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestHMC252ManagedExportRootLazySharedAndRunIsolated(t *testing.T) {
	anchor := t.TempDir()
	r := NewManagedExportRoot(anchor)
	if p, err := r.Directory(t.Context(), false); err != nil || p != "" {
		t.Fatalf("read-only unrequested root: %q %v", p, err)
	}
	if entries, _ := os.ReadDir(anchor); len(entries) != 0 {
		t.Fatal("constructor or lookup created files")
	}
	const n = 24
	paths := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(i int) { defer wg.Done(); paths[i], errs[i] = r.Directory(t.Context(), true) }(i)
	}
	wg.Wait()
	for i := range paths {
		if errs[i] != nil || paths[i] != paths[0] || paths[i] == "" {
			t.Fatalf("concurrent namespace divergence: %q %v", paths[i], errs[i])
		}
	}
	other, err := NewManagedExportRoot(anchor).Directory(t.Context(), true)
	if err != nil || other == paths[0] {
		t.Fatalf("run namespace collision: %q %v", other, err)
	}
	if p, err := r.Directory(t.Context(), false); err != nil || p != paths[0] {
		t.Fatal("read-only lookup changed publication root", err)
	}
}

func TestHMC252ManagedExportRootRejectsUnboundAndChangedScope(t *testing.T) {
	for _, mode := range []string{"nil", "json", "relative", "missing", "canceled", "symlink_parent", "replace_run", "symlink_run"} {
		t.Run(mode, func(t *testing.T) {
			anchor := t.TempDir()
			r := NewManagedExportRoot(anchor)
			ctx := t.Context()
			switch mode {
			case "nil":
				r = nil
			case "json":
				working := t.TempDir()
				t.Chdir(working)
				t.Cleanup(func() {
					if entries, _ := os.ReadDir(working); len(entries) != 0 {
						t.Error("unbound JSON created a working-directory output location")
					}
				})
				data, err := json.Marshal(r)
				if err != nil || string(data) != "{}" {
					t.Fatalf("host scope serialized: %s %v", data, err)
				}
				r = &ManagedExportRoot{}
				if err := json.Unmarshal([]byte(`{"anchor":"`+anchor+`","run":"chosen","path":"`+anchor+`"}`), r); err != nil {
					t.Fatal(err)
				}
			case "relative":
				r = NewManagedExportRoot("relative")
			case "missing":
				r = NewManagedExportRoot(filepath.Join(anchor, "not-provisioned"))
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "symlink_parent":
				if err := os.Symlink(t.TempDir(), filepath.Join(anchor, "deliveries")); err != nil {
					t.Fatal(err)
				}
			case "replace_run", "symlink_run":
				path, err := r.Directory(ctx, true)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path, path+"-retained"); err != nil {
					t.Fatal(err)
				}
				if mode == "replace_run" {
					err = os.Mkdir(path, 0700)
				} else {
					err = os.Symlink(t.TempDir(), path)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if path, err := r.Directory(ctx, true); err == nil {
				t.Fatalf("untrusted destination accepted: %s", path)
			}
		})
	}
}
