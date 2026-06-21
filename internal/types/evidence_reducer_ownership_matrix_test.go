package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestKnownEvidenceReducerInputClassesCoversDeclaredConstants(t *testing.T) {
	declared := declaredEvidenceReducerInputClasses(t)
	known := map[EvidenceReducerInputClass]bool{}
	for _, class := range KnownEvidenceReducerInputClasses() {
		known[class] = true
	}
	for _, class := range declared {
		if !known[class] {
			t.Fatalf("EvidenceReducerInputClass constant %q is not listed by KnownEvidenceReducerInputClasses", class)
		}
	}
	if len(known) != len(declared) {
		t.Fatalf("known reducer classes=%d, declared constants=%d; known=%v declared=%v", len(known), len(declared), known, declared)
	}
}

func TestEvidenceReducerOwnershipMatrixCoversKnownInputClasses(t *testing.T) {
	rows := EvidenceReducerOwnershipMatrix()
	seen := map[EvidenceReducerInputClass]EvidenceReducerOwnershipRow{}
	for _, row := range rows {
		if row.InputClass == "" {
			t.Fatalf("ownership row has empty input class: %+v", row)
		}
		if _, exists := seen[row.InputClass]; exists {
			t.Fatalf("duplicate ownership row for %q", row.InputClass)
		}
		seen[row.InputClass] = row
	}
	for _, class := range KnownEvidenceReducerInputClasses() {
		row, ok := seen[class]
		if !ok {
			t.Fatalf("missing ownership row for reducer input class %q", class)
		}
		if len(row.Artifacts) == 0 {
			t.Fatalf("ownership row %q must name owned artifact classes", class)
		}
	}
	if len(seen) != len(KnownEvidenceReducerInputClasses()) {
		t.Fatalf("ownership matrix has %d rows, known input classes=%d", len(seen), len(KnownEvidenceReducerInputClasses()))
	}
}

func TestEvidenceReducerOwnershipMatrixUsesClosedArtifactsAndMergeSemantics(t *testing.T) {
	for _, row := range EvidenceReducerOwnershipMatrix() {
		artifactSet := map[EvidenceReducerArtifactClass]bool{}
		for _, artifact := range row.Artifacts {
			if !ValidEvidenceReducerArtifactClass(artifact) {
				t.Fatalf("row %q has unknown artifact class %q", row.InputClass, artifact)
			}
			if artifactSet[artifact] {
				t.Fatalf("row %q duplicates artifact class %q", row.InputClass, artifact)
			}
			artifactSet[artifact] = true
			semantics, ok := row.Semantics[artifact]
			if !ok {
				t.Fatalf("row %q missing merge semantics for artifact %q", row.InputClass, artifact)
			}
			if !ValidEvidenceReducerMergeSemantics(semantics) {
				t.Fatalf("row %q artifact %q has unknown merge semantics %q", row.InputClass, artifact, semantics)
			}
		}
		for artifact := range row.Semantics {
			if !artifactSet[artifact] {
				t.Fatalf("row %q has merge semantics for unowned artifact %q", row.InputClass, artifact)
			}
		}
	}
}

func TestEvidenceReducerOwnershipForInputClassReturnsClone(t *testing.T) {
	row, ok := EvidenceReducerOwnershipForInputClass(EvidenceReducerInputReadRunSnapshotSeed)
	if !ok {
		t.Fatalf("missing read-run snapshot ownership row")
	}
	row.Artifacts[0] = EvidenceReducerArtifactClass("mutated")
	for artifact := range row.Semantics {
		delete(row.Semantics, artifact)
		break
	}
	fresh, ok := EvidenceReducerOwnershipForInputClass(EvidenceReducerInputReadRunSnapshotSeed)
	if !ok {
		t.Fatalf("missing read-run snapshot ownership row after mutation")
	}
	if !ValidEvidenceReducerArtifactClass(fresh.Artifacts[0]) {
		t.Fatalf("ownership row leaked mutable artifacts slice: %+v", fresh)
	}
	if len(fresh.Semantics) == 0 {
		t.Fatalf("ownership row leaked mutable semantics map: %+v", fresh)
	}
}

func TestEvidenceReducerOwnershipMatrixPinsKeyMergeSemantics(t *testing.T) {
	cases := []struct {
		class    EvidenceReducerInputClass
		artifact EvidenceReducerArtifactClass
		want     EvidenceReducerMergeSemantics
	}{
		{EvidenceReducerInputStageCoverageSnapshot, EvidenceReducerArtifactReadSet, EvidenceReducerMergeSnapshotReplace},
		{EvidenceReducerInputReadCoverageDelta, EvidenceReducerArtifactReadSet, EvidenceReducerMergeAdditiveUnion},
		{EvidenceReducerInputReadRunSnapshotSeed, EvidenceReducerArtifactProgressDecision, EvidenceReducerMergeLatestAuthority},
		{EvidenceReducerInputNodeArtifactDelta, EvidenceReducerArtifactNodeArtifacts, EvidenceReducerMergeNormalizedIdentity},
		{EvidenceReducerInputForkClosureDelta, EvidenceReducerArtifactForkClosure, EvidenceReducerMergeForkClosureDelta},
	}
	for _, c := range cases {
		row, ok := EvidenceReducerOwnershipForInputClass(c.class)
		if !ok {
			t.Fatalf("missing ownership row for %q", c.class)
		}
		if got := row.Semantics[c.artifact]; got != c.want {
			t.Fatalf("row %q artifact %q semantics=%q, want %q", c.class, c.artifact, got, c.want)
		}
	}
}

func declaredEvidenceReducerInputClasses(t *testing.T) []EvidenceReducerInputClass {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "evidence_round.go", nil, 0)
	if err != nil {
		t.Fatalf("parse evidence_round.go: %v", err)
	}
	var out []EvidenceReducerInputClass
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if name == nil || !strings.HasPrefix(name.Name, "EvidenceReducerInput") || name.Name == "EvidenceReducerInputClass" {
				continue
			}
			if i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote reducer input constant %s: %v", name.Name, err)
			}
			out = append(out, EvidenceReducerInputClass(value))
		}
		return true
	})
	return out
}
