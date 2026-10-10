package criterion

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogTypeLiteralCannotBypassRuntimeEvidenceFloorWithoutWitness(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "captured.log", Data: []byte("NativeFault\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	locations, err := catalog.Locate(context.Background(), []loginput.ExcerptSelector{{Text: "NativeFault"}})
	if err != nil || len(locations) != 1 {
		t.Fatalf("locate: %+v / %v", locations, err)
	}
	proven := types.LogError{Type: "NativeFault", SourceBinding: types.NewLogSourceBindingForError(locations[0], "NativeFault", "")}
	for _, tc := range []struct {
		name  string
		error types.LogError
		want  bool
	}{
		{"diagnostic_only", types.LogError{Type: "InventedFault"}, false},
		{"source_literal", proven, true},
		{"label_mutated", types.LogError{Type: "InventedFault", SourceBinding: proven.SourceBinding}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := Env{ObservationOnlyCompletion: true, LogTriage: &types.LogBundle{Errors: []types.LogError{tc.error}}}
			for _, c := range []types.Criterion{{Kind: string(KindEvidenceCount), Expr: ">=3"}, {Kind: string(KindExtractInputReady)}} {
				if got := Eval(c, env); got.Satisfied != tc.want {
					t.Fatalf("%s satisfied=%v want %v: %s", c.Kind, got.Satisfied, tc.want, got.Detail)
				}
			}
		})
	}
}
