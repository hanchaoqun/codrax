package traceinput

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func hmc263SelectionFixture(t *testing.T) (Options, string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, item := range []struct {
		name  string
		pages int
	}{{"业务/A.data", 7}, {"业务/B", 3}} {
		f, err := w.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(realRMQFixture(item.pages)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "captures.zip")
	writeTestFile(t, name, buf.Bytes())
	return Options{RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 640}, name, buf.Bytes()
}

func hmc263Select(t *testing.T, c *Coordinator, path, member string) *attachment.TraceMaterial {
	t.Helper()
	m, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: member})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHMC263ArchiveSelectionSharedPreparationAndColdWarmIdentity(t *testing.T) {
	opts, path, _ := hmc263SelectionFixture(t)
	opts.InputPath, opts.ArchiveMember = path, "业务/A.data"
	direct, err := Prepare(t.Context(), opts)
	if err != nil || direct.ArchiveMember() != opts.ArchiveMember || !direct.ExplicitArchiveSelection() {
		t.Fatalf("direct selection: %v %v", direct, err)
	}
	var calls atomic.Int32
	c := newCoordinator(opts, func(ctx context.Context, o hitraceconv.Options) (hitraceconv.Result, error) {
		calls.Add(1)
		return hitraceconv.PrepareFile(ctx, o)
	})
	a, b := hmc263Select(t, c, path, "业务/A.data"), hmc263Select(t, c, path, "业务/B")
	if a == b || a.QueryPath() == b.QueryPath() || calls.Load() != 2 || len(c.PreparedMaterials()) != 2 {
		t.Fatal("member identity collapsed")
	}
	for _, row := range []struct {
		m      *attachment.TraceMaterial
		member string
		count  int
	}{{a, "业务/A.data", 7}, {b, "业务/B", 3}} {
		m := hmc263Select(t, c, path, row.member)
		query, err := c.Prepare(t.Context(), m.QueryPath())
		if m != row.m || query != m || err != nil || calls.Load() != 2 {
			t.Fatal("cold/warm selection changed")
		}
		idx, err := tracequery.BuildIndex(t.Context(), m.QueryPath())
		if err != nil || len(idx.Events) != row.count {
			t.Fatalf("wrong complete selected population: %v %v", idx, err)
		}
		if m.MatchesPath(path) || m.MatchesOriginalSource(path) || !m.MatchesPath(m.QueryPath()) || !m.MatchesSelection(path, row.member) || m.MatchesSelection(m.QueryPath(), row.member) {
			t.Fatal("source/query selection alias escaped")
		}
		if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: m.QueryPath(), ArchiveMember: row.member}); err == nil {
			t.Fatal("derived path accepted selector")
		}
	}
	if _, err := c.Prepare(t.Context(), path); err == nil {
		t.Fatal("an earlier selection resolved unspecified ambiguity")
	}
	alias := filepath.Join(filepath.Dir(path), "alias.zip")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if got := hmc263Select(t, c, alias, "业务/B"); got != b {
		t.Fatal("member symlink alias lost receipt")
	}
}

func TestHMC263ArchiveSelectionOriginalAndDerivedInvalidationAreIndependent(t *testing.T) {
	for _, change := range []string{"derived", "same_bytes_new_original", "retarget_alias", "stale_new_alias"} {
		t.Run(change, func(t *testing.T) {
			opts, path, body := hmc263SelectionFixture(t)
			alias := filepath.Join(filepath.Dir(path), "alias.zip")
			if err := os.Symlink(path, alias); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			c := newCoordinator(opts, func(ctx context.Context, o hitraceconv.Options) (hitraceconv.Result, error) {
				calls.Add(1)
				return hitraceconv.PrepareFile(ctx, o)
			})
			a := hmc263Select(t, c, alias, "业务/A.data")
			switch change {
			case "derived":
				writeTestFile(t, a.QueryPath(), []byte("broken derived output"))
				b := hmc263Select(t, c, path, "业务/B")
				if b == nil || calls.Load() != 2 {
					t.Fatal("damaged A blocked healthy B")
				}
				if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "业务/A.data"}); err == nil {
					t.Fatal("damaged A regained authority")
				}
				return
			case "same_bytes_new_original", "stale_new_alias":
				replacement := filepath.Join(filepath.Dir(path), "new.zip")
				writeTestFile(t, replacement, body)
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
				if change == "stale_new_alias" {
					unseen := filepath.Join(filepath.Dir(path), "new-alias.zip")
					if err := os.Symlink(path, unseen); err != nil {
						t.Fatal(err)
					}
					if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: unseen, ArchiveMember: "业务/B"}); err == nil {
						t.Fatal("new alias accepted stale original")
					}
					writeTestFile(t, replacement, body)
					if err := os.Remove(unseen); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(replacement, unseen); err != nil {
						t.Fatal(err)
					}
					path = unseen
				}
			case "retarget_alias":
				other := filepath.Join(filepath.Dir(path), "other.zip")
				writeTestFile(t, other, body)
				if err := os.Remove(alias); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, alias); err != nil {
					t.Fatal(err)
				}
				path = alias
			}
			for _, member := range []string{"业务/A.data", "业务/B", ""} {
				if m, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: member}); err == nil || m != nil || calls.Load() != 1 {
					t.Fatalf("member change bypassed source: %s %v calls=%d", member, err, calls.Load())
				}
			}
		})
	}
}

func TestHMC263ArchiveSelectionRejectsNonZIPAndInvalidCoordinates(t *testing.T) {
	for _, body := range [][]byte{[]byte("# text trace\n"), realRMQFixture(1), []byte("{\"schema\":\"tracebundle-v2\"}")} {
		path := filepath.Join(t.TempDir(), "capture.zip")
		writeTestFile(t, path, body)
		var calls atomic.Int32
		_, err := prepare(t.Context(), Options{InputPath: path, ArchiveMember: "a.data"}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
			calls.Add(1)
			return hitraceconv.Result{}, nil
		})
		if err == nil || calls.Load() != 0 {
			t.Fatal("selector on non-ZIP was ignored")
		}
	}
	opts, path, _ := hmc263SelectionFixture(t)
	c := NewCoordinator(opts)
	for _, member := range []string{" ../A.data", "../A.data", "业务/./A.data", "业务/A.data/", "业务\\A.data", "业务/A.data\x00"} {
		if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: member}); err == nil {
			t.Fatalf("invalid selector accepted: %q", member)
		}
	}
}

func TestHMC263ArchiveSelectionFailureAndCancellationDoNotPoisonSibling(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed_member", true: "canceled_member"}[cancel], func(t *testing.T) {
			opts, path, _ := hmc263SelectionFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			c := newCoordinator(opts, func(ctx context.Context, o hitraceconv.Options) (hitraceconv.Result, error) {
				calls.Add(1)
				if o.ArchiveMember == "业务/A.data" {
					close(entered)
					<-release
					if cancel {
						return hitraceconv.Result{}, ctx.Err()
					}
					return hitraceconv.Result{}, errors.New("A failed")
				}
				return hitraceconv.PrepareFile(ctx, o)
			})
			ctx, stop := context.WithCancel(t.Context())
			defer stop()
			aDone := make(chan error, 1)
			go func() {
				_, err := c.PrepareSelection(ctx, attachment.TraceSelection{Path: path, ArchiveMember: "业务/A.data"})
				aDone <- err
			}()
			<-entered
			var wg sync.WaitGroup
			results := make([]*attachment.TraceMaterial, 8)
			errs := make([]error, 8)
			for i := range results {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					results[i], errs[i] = c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "业务/B"})
				}(i)
			}
			if cancel {
				stop()
			}
			close(release)
			wg.Wait()
			if err := <-aDone; err == nil {
				t.Fatal("A unexpectedly succeeded")
			}
			for i, m := range results {
				if errs[i] != nil || m == nil || m != results[0] {
					t.Fatalf("B inherited A failure: %v", errs[i])
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("sibling singleflight lost: %d", calls.Load())
			}
		})
	}
}
