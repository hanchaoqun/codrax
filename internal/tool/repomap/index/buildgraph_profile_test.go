package index

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// BenchmarkBuildGraph_CrossPackageCalls guards against the call-target
// resolver regressing to a per-relation MethodIndex scan (which made
// BuildGraph ~19s on the codrax tree). The synthetic graph defines one
// hot method name across many packages and a wall of call sites that
// miss same-package resolution, exactly the shape that triggered the
// quadratic path.
func BenchmarkBuildGraph_CrossPackageCalls(b *testing.B) {
	const pkgs, callers = 200, 400
	var files []*types.FileInfo
	for p := 0; p < pkgs; p++ {
		files = append(files, &types.FileInfo{
			RelPath: fmt.Sprintf("pkg%03d/store.go", p), Language: "go", Package: fmt.Sprintf("pkg%03d", p),
			Symbols: []types.Symbol{{Name: "Handle", Receiver: "Store", Kind: "method"}},
		})
	}
	for c := 0; c < callers; c++ {
		files = append(files, &types.FileInfo{
			RelPath: fmt.Sprintf("call%03d/use.go", c), Language: "go", Package: fmt.Sprintf("call%03d", c),
			Relations: []types.Relation{{Kind: "call", To: "Handle", ToEP: types.RelationEndpoint{Name: "Handle", Receiver: "Store"}}},
		})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildGraph(".", files)
	}
}
