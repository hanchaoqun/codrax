package repomap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestB1692CallableReturnColdWarmAndScopedProjectionIsolation(t *testing.T) {
	repo := t.TempDir()
	sub := filepath.Join(repo, "svc")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	source := []byte("package svc\nfunc Work() string { return \"value\" }\n")
	if err := os.WriteFile(filepath.Join(sub, "work.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	cold, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	warm, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if cold.Metadata.IndexStatus.Source != rmtypes.IndexSourceFullScan || warm.Metadata.IndexStatus.Source != rmtypes.IndexSourceCacheHit {
		t.Fatal("not actual cold/warm cache branches")
	}
	coldFile, warmFile := cold.FileIndex["svc/work.go"], warm.FileIndex["svc/work.go"]
	if coldFile == nil || warmFile == nil || len(coldFile.CallableReturnExpressions) != 1 || !reflect.DeepEqual(coldFile.CallableReturnExpressions, warmFile.CallableReturnExpressions) {
		t.Fatal("cold/warm return divergence")
	}
	projected, err := projectGraphToRoot(warm, repo, sub)
	if err != nil {
		t.Fatal(err)
	}
	pf := projected.FileIndex["work.go"]
	if pf == nil || len(rmtypes.NewCallableReturnReader(pf, source).For(pf.Symbols[0])) != 1 {
		t.Fatal("scoped identity failed to preserve return")
	}
	pf.CallableReturnExpressions[0].Expression = "mutated"
	if warmFile.CallableReturnExpressions[0].Expression != `"value"` {
		t.Fatal("scope clone aliased parent receipt slice")
	}
}
