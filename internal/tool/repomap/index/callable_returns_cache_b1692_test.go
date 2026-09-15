package index

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestB1692CallableReturnsCacheRoundTripAndReaderIntegrity(t *testing.T) {
	source := []byte("package p\nfunc work() string { return \"值\" }\n")
	fi, rows := b1692ParsePublic(t, types.LangGo, string(source))
	if len(rows) != 1 {
		t.Fatal("missing actual public parser receipt")
	}
	dir := t.TempDir()
	if err := saveFileInfos(dir, "", []*types.FileInfo{fi}); err != nil {
		t.Fatal(err)
	}
	warm := LoadFileInfos(dir)
	if len(warm) != 1 || !reflect.DeepEqual(fi.CallableReturnExpressions, warm[0].CallableReturnExpressions) {
		t.Fatalf("cold/warm divergence: %+v", warm)
	}
	sym := fi.Symbols[0]
	reader := types.NewCallableReturnReader(warm[0], source)
	if got := reader.For(sym); len(got) != 1 || got[0].Expression != `"值"` {
		t.Fatalf("warm reader lost exact return: %+v", got)
	}
	got := reader.For(sym)
	got[0].Expression = "mutated"
	warm[0].CallableReturnExpressions[0].Expression = "other"
	if got := reader.For(sym); len(got) != 1 || got[0].Expression != `"值"` {
		t.Fatalf("reader is not isolated: %+v", got)
	}
	if got := types.NewCallableReturnReader(fi, append([]byte("// shifted\n"), source...)).For(sym); len(got) != 0 {
		t.Fatalf("stale graph reused current source: %+v", got)
	}
	for _, mutate := range []func(*types.Symbol){
		func(s *types.Symbol) { s.File = "other.go" }, func(s *types.Symbol) { s.Parent = "Other" }, func(s *types.Symbol) { s.Receiver = "Other" }, func(s *types.Symbol) { s.Name = "other" }, func(s *types.Symbol) { s.Line++ }, func(s *types.Symbol) { s.EndLine++ }, func(s *types.Symbol) { s.BodyStartLine++ },
	} {
		wrong := sym
		mutate(&wrong)
		if len(reader.For(wrong)) != 0 {
			t.Fatalf("foreign owner borrowed receipt: %+v", wrong)
		}
	}
	for name, mutate := range map[string]func(*types.FileInfo){
		"duplicate_owner": func(f *types.FileInfo) { f.Symbols = append(f.Symbols, f.Symbols[0]) },
		"owner":           func(f *types.FileInfo) { f.CallableReturnExpressions[0].CallableName = "other" },
		"byte":            func(f *types.FileInfo) { f.CallableReturnExpressions[0].StartByte++ },
		"line":            func(f *types.FileInfo) { f.CallableReturnExpressions[0].LineStart-- },
		"kind":            func(f *types.FileInfo) { f.CallableReturnExpressions[0].Kind = "unknown" },
		"provenance":      func(f *types.FileInfo) { f.CallableReturnExpressions[0].Provenance = "regex" },
		"expression":      func(f *types.FileInfo) { f.CallableReturnExpressions[0].Expression = `"假"` },
		"hash":            func(f *types.FileInfo) { f.Hash = "" },
	} {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(fi)
			var copy types.FileInfo
			if err := json.Unmarshal(b, &copy); err != nil {
				t.Fatal(err)
			}
			mutate(&copy)
			if len(types.NewCallableReturnReader(&copy, source).For(sym)) != 0 {
				t.Fatal("invalid receipt admitted")
			}
		})
	}
}

func TestB1692CallableReturnsCacheRejectsLegacyEpochAndMalformedReceipt(t *testing.T) {
	if cacheSchemaVersion < 8 {
		t.Fatalf("unexpected callable receipt cache epoch: %d", cacheSchemaVersion)
	}
	if cacheManifestVersionReject(7, extractorVersions) == CacheRejectNone {
		t.Fatal("cache without new carrier generation admitted")
	}
	fi, rows := b1692ParsePublic(t, types.LangGo, "package p\nfunc work() int { return 1 }\n")
	if len(rows) != 1 {
		t.Fatal("missing receipt")
	}
	if reject := validateCachedFileInfos([]*types.FileInfo{fi}); reject != CacheRejectNone {
		t.Fatalf("valid receipt rejected: %s", reject)
	}
	fi.CallableReturnExpressions[0].CallableName = "other"
	if reject := validateCachedFileInfos([]*types.FileInfo{fi}); reject != CacheRejectCorrupt {
		t.Fatalf("malformed owner not rejected: %s", reject)
	}
}
