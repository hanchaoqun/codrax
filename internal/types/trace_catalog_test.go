package types

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestTraceCatalogForkDiscoveryLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "capture"), []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := NewMutableState("query captures")
	fork := m.ForkForExploreDispatch()
	if _, err := fork.InstallTraceCatalog(c); err != nil {
		t.Fatal(err)
	}
	if len(m.TraceCatalogs()) != 0 {
		t.Fatal("fork leaked before merge")
	}
	m.MergeExploreFork(fork)
	if len(m.TraceCatalogs()) != 1 {
		t.Fatal("new discovery lost at merge")
	}
	if len(m.TraceQueryBlobRefs()) != 0 {
		t.Fatal("navigation minted payload access")
	}
	m.ResetTurnAArtifacts()
	m.MergeExploreFork(fork)
	if len(m.TraceCatalogs()) != 0 {
		t.Fatal("old fork revived discovery after turn reset")
	}
	other := NewMutableState("independent run")
	other.MergeExploreFork(fork)
	if len(other.TraceCatalogs()) != 0 {
		t.Fatal("independent run imported live catalog")
	}
}

func TestTraceCatalogIndependentForkDiscoveriesShareOnlyLiveLineage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "capture"), []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewMutableState("two objects")
	first, second := m.ForkForExploreDispatch(), m.ForkForExploreDispatch()
	for i, fork := range []*MutableState{first, second} {
		c, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		c, err = fork.InstallTraceCatalog(c)
		if err != nil {
			t.Fatal(err)
		}
		id := []string{"101", "202"}[i]
		_, err = c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: c.Snapshot().Artifacts[0].ID, Object: tracecatalog.Object{Kind: "thread", ID: id}, View: "resource_stack"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(first.TraceCatalogs()[0].Snapshot().Queries) != 1 {
		t.Fatal("query state leaked between live forks")
	}
	m.MergeExploreFork(second)
	m.MergeExploreFork(first)
	if got := m.TraceCatalogs(); len(got) != 1 || len(got[0].Snapshot().Queries) != 2 {
		t.Fatalf("independent live discoveries lost work: %+v", got)
	}
}
