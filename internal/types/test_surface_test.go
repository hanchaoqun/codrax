package types

import (
	"path/filepath"
	"testing"
)

func TestWriteTestSurfaceToFileRoundTripNormalizesSelectedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan-1.attempt-1.surface.json")
	surface := &TestSurface{Candidates: []TestSurfaceCandidate{
		{ID: "make@.", Runner: "make", WorkingDir: ".", Priority: 10, HasTestSignal: false},
		{ID: "go@.", Runner: "go", WorkingDir: ".", Priority: 20, HasTestSignal: true},
	}}
	if err := WriteTestSurfaceToFile(surface, path); err != nil {
		t.Fatalf("WriteTestSurfaceToFile: %v", err)
	}
	got, err := LoadTestSurfaceFromFile(path)
	if err != nil {
		t.Fatalf("LoadTestSurfaceFromFile: %v", err)
	}
	if got.SelectedID != "go@." {
		t.Fatalf("SelectedID = %q, want go@.", got.SelectedID)
	}
	if len(got.Candidates) != 2 || got.Candidates[0].ID != "go@." {
		t.Fatalf("surface candidates not normalized: %+v", got.Candidates)
	}
}
