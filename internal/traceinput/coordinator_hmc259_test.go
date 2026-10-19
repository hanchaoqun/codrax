package traceinput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC259CoordinatorSearchOnlyNamedBinary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native search-only entry witness currently targets Darwin")
	}
	opts, source, original := coordinatorFixture(t)
	opts.RuntimeAnchor = t.TempDir()
	dir := filepath.Dir(source)
	if err := os.Chmod(dir, 0o100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("readable named input prerequisite failed: %v", err)
	}
	if _, err := os.ReadDir(dir); !os.IsPermission(err) {
		t.Skipf("host does not enforce search-only directory listing: %v", err)
	}
	c := NewCoordinator(opts)
	m, err := c.Prepare(context.Background(), source)
	if err != nil || m == nil {
		t.Fatalf("named readable binary required unnecessary directory listing: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), m.QueryPath())
	if err != nil || len(idx.Events) != 40 || idx.Events[39].WakeePID != 139 {
		t.Fatalf("complete binary tail unavailable: %v %v", idx, err)
	}
	if got, err := c.Prepare(context.Background(), source); err != nil || got != m {
		t.Fatalf("search-only input did not reuse receipt: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original input changed: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o100 {
		t.Fatalf("source permission changed: %v %v", info, err)
	}
}

func hmc259SearchOnly(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("native search-only entry witness currently targets Darwin")
	}
	if err := os.Chmod(dir, 0o100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := os.ReadDir(dir); !os.IsPermission(err) {
		t.Skipf("host does not enforce search-only directory listing: %v", err)
	}
}

func TestHMC259SearchOnlyAliasesCannotReprepareChangedCapture(t *testing.T) {
	for _, aliasFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "physical_first", true: "alias_first"}[aliasFirst], func(t *testing.T) {
			opts, physical, alias := coordinatorCaseAliasFixture(t)
			opts.RuntimeAnchor = t.TempDir()
			parent := filepath.Dir(filepath.Dir(physical))
			hmc259SearchOnly(t, parent)
			hmc259SearchOnly(t, filepath.Dir(physical))
			first, next := physical, alias
			if aliasFirst {
				first, next = alias, physical
			}
			c := NewCoordinator(opts)
			m, err := c.Prepare(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := c.Prepare(context.Background(), next); err != nil || got != m || len(c.PreparedMaterials()) != 1 {
				t.Fatalf("native spelling failed to share exact capture: %v", err)
			}
			originalInfo, err := os.Stat(physical)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(physical)
			if err != nil {
				t.Fatal(err)
			}
			// Same-size rewrite and restored mtime must not escape the current
			// receipt through another case spelling in a non-listable parent.
			body[len(body)-1] ^= 1
			writeTestFile(t, physical, body)
			if err := os.Chtimes(physical, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{first, next, filepath.Join(filepath.Dir(alias), "CaPtUrE.sYs")} {
				if got, err := c.Prepare(context.Background(), path); err == nil || got != nil || len(c.PreparedMaterials()) != 1 {
					t.Fatalf("stale generation bypassed through case alias %q: %v", path, err)
				}
			}
		})
	}
}

func TestHMC259SearchOnlySymlinkRetargetAndReadPermission(t *testing.T) {
	opts, physical, _ := coordinatorFixture(t)
	opts.RuntimeAnchor = t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias.sys")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	hmc259SearchOnly(t, filepath.Dir(physical))
	c := NewCoordinator(opts)
	if _, err := c.Prepare(context.Background(), alias); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other.sys")
	writeTestFile(t, other, realRMQFixture(2))
	if err := os.Rename(alias, alias+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Prepare(context.Background(), alias); err == nil || got != nil {
		t.Fatalf("seen alias retarget bypassed existing receipt: %v", err)
	}
	if err := os.Chmod(physical, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(physical, 0o600) })
	if _, err := os.ReadFile(physical); !os.IsPermission(err) {
		t.Skipf("host does not enforce unreadable-file permission: %v", err)
	}
	fresh := NewCoordinator(opts)
	if got, err := fresh.Prepare(context.Background(), physical); !errors.Is(err, os.ErrPermission) || got != nil || len(fresh.PreparedMaterials()) != 0 {
		t.Fatalf("native identity granted unavailable file-read permission: %v", err)
	}
}

type hmc259ResolutionContext struct {
	context.Context
	onRecheck func()
	calls     int
	err       error
}

func (c *hmc259ResolutionContext) Err() error {
	c.calls++
	if c.calls == 2 && c.onRecheck != nil {
		c.onRecheck()
	}
	if c.calls >= 2 {
		return c.err
	}
	return nil
}

func TestHMC259SearchOnlyResolutionRejectsMutationAndCancellation(t *testing.T) {
	for _, mutation := range []string{"cancellation", "file_replaced", "parent_replaced", "symlink", "second_hardlink"} {
		t.Run(mutation, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "source")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "capture.sys")
			writeTestFile(t, path, []byte("original bytes"))
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			hmc259SearchOnly(t, dir)
			ctx := &hmc259ResolutionContext{Context: context.Background()}
			ctx.onRecheck = func() {
				if mutation == "cancellation" {
					ctx.err = context.Canceled
					return
				}
				if err := os.Chmod(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				switch mutation {
				case "file_replaced", "symlink":
					if err := os.Rename(path, path+".original"); err != nil {
						t.Fatal(err)
					}
					if mutation == "symlink" {
						if err := os.Symlink(path+".original", path); err != nil {
							t.Fatal(err)
						}
					} else {
						writeTestFile(t, path, []byte("original bytes"))
					}
				case "parent_replaced":
					if err := os.Rename(dir, dir+".old"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(filepath.Join(dir+".old", "capture.sys"), path); err != nil {
						t.Fatal(err)
					}
				case "second_hardlink":
					if err := os.Link(path, path+".other"); err != nil {
						t.Fatal(err)
					}
				}
			}
			got, err := preparationSearchOnlyDirectoryEntry(ctx, dir, "capture.sys", info)
			if got != "" || err == nil {
				t.Fatalf("resolution accepted %s: %q %v", mutation, got, err)
			}
			if mutation == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation became an input-format error: %v", err)
			}
		})
	}
}

func TestHMC259SearchOnlyHardlinkAmbiguityDoesNotGrantIdentity(t *testing.T) {
	opts, first, _ := coordinatorFixture(t)
	opts.RuntimeAnchor = t.TempDir()
	second := filepath.Join(filepath.Dir(first), "independent.sys")
	if err := os.Link(first, second); err != nil {
		t.Fatal(err)
	}
	hmc259SearchOnly(t, filepath.Dir(first))
	c := NewCoordinator(opts)
	for _, path := range []string{first, second} {
		if got, err := c.Prepare(context.Background(), path); got != nil || err == nil || !strings.Contains(err.Error(), "single-link") || len(c.PreparedMaterials()) != 0 {
			t.Fatalf("uncertain hardlink spelling granted identity for %q: %v", path, err)
		}
	}
	if err := os.Chmod(filepath.Dir(first), 0o700); err != nil {
		t.Fatal(err)
	}
	a, err := c.Prepare(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Prepare(context.Background(), second)
	if err != nil || a == b || a.QueryPath() == b.QueryPath() || len(c.PreparedMaterials()) != 2 {
		t.Fatalf("listable exact hardlink entries merged or failure negatively cached: %v", err)
	}
}
