package tracecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHMC225IndependentOriginalBinding(t *testing.T) {
	dir := t.TempDir()
	a, b, alias := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "a-link")
	putFile(t, a, "a")
	putFile(t, b, "b")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	c, err := DiscoverIndependentPair(context.Background(), [2]string{alias, b})
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []*Catalog{c, c.Clone()} {
		artifact, bound, err := current.IndependentArtifactForOriginal(context.Background(), alias)
		if !bound || err != nil || artifact.ID == "" {
			t.Fatalf("live alias lost: %+v %v %v", artifact, bound, err)
		}
		if _, bound, _ := current.IndependentArtifactForOriginal(context.Background(), a); bound {
			t.Fatal("unrequested physical spelling minted role")
		}
	}
	raw, _ := json.Marshal(c.Snapshot())
	var restored Catalog
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, bound, _ := restored.IndependentArtifactForOriginal(context.Background(), alias); bound {
		t.Fatal("JSON restored original binding")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, alias); err != nil {
		t.Fatal(err)
	}
	if _, bound, err := c.IndependentArtifactForOriginal(context.Background(), alias); !bound || !errors.Is(err, ErrStale) {
		t.Fatalf("retargeted alias not stale: %v %v", bound, err)
	}
	if _, bound, err := c.IndependentArtifactForOriginal(context.Background(), b); !bound || err != nil {
		t.Fatal("one stale side invalidated healthy side")
	}
}

func TestHMC225IndependentMissingAndCancellation(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "missing"), filepath.Join(dir, "healthy")
	putFile(t, b, "capture")
	c, err := DiscoverIndependentPair(context.Background(), [2]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if _, bound, err := c.IndependentArtifactForOriginal(context.Background(), a); !bound || !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing side lost its exact declared identity")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, bound, err := c.IndependentArtifactForOriginal(ctx, b); !bound || !errors.Is(err, context.Canceled) {
		t.Fatal("global cancellation absorbed")
	}
}
