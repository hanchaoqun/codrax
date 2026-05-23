package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRelationCandidateGroundingFiles_ImportEdgeUsesObservedImporterFile(t *testing.T) {
	candidate := types.TypedRelationCandidate{
		Relation:   types.TypedRelationImports,
		SourceName: "cmd/root.go",
		SourceFile: "cmd/root.go",
		Member: types.TypedRelationMember{
			Name: "internal/tool/tool.go",
			File: "internal/tool/tool.go",
		},
		Precision: types.TypedRelationPrecisionExactFile,
	}
	if !relationCandidateGroundedInFile(candidate, "cmd/root.go") {
		t.Fatal("import-edge grounding should accept evidence from the observed importer file")
	}
	if !relationCandidateGroundedInFile(candidate, "internal/tool/tool.go") {
		t.Fatal("import-edge grounding should still accept evidence from the member file")
	}
	if relationCandidateGroundedInFile(candidate, "unrelated.go") {
		t.Fatal("import-edge grounding must not accept unrelated files")
	}
}

func TestRelationCandidateGroundingFiles_DefaultUsesMemberFileOnly(t *testing.T) {
	candidate := types.TypedRelationCandidate{
		Relation:   types.TypedRelationImplements,
		SourceName: "Looper",
		SourceFile: "iface.go",
		Member: types.TypedRelationMember{
			Name: "impl",
			File: "impl.go",
		},
		Precision: types.TypedRelationPrecisionExactSymbolID,
	}
	if relationCandidateGroundedInFile(candidate, "iface.go") {
		t.Fatal("non-import relation grounding must not switch to source file")
	}
	if !relationCandidateGroundedInFile(candidate, "impl.go") {
		t.Fatal("non-import relation grounding should use member file")
	}
}
