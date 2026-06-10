package types

import "testing"

func TestNormalizeArtifactLineRefs_BoundsAndDrops(t *testing.T) {
	in := []ArtifactLineRef{
		{Source: "LOG", StartLine: 3},
		{Source: "trace", StartLine: 5, EndLine: 6},
		{Source: "file", StartLine: 1},
		{Source: "log", StartLine: 0},
		{Source: "log", StartLine: 9, EndLine: 4},
	}
	out := NormalizeArtifactLineRefs(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 surviving refs, got %+v", out)
	}
	if out[0].Source != "log" || out[0].StartLine != 3 || out[0].EndLine != 3 {
		t.Fatalf("single-line log ref normalizes to start=end, got %+v", out[0])
	}
	if out[1].Source != "trace" || out[1].EndLine != 6 {
		t.Fatalf("trace span preserved, got %+v", out[1])
	}
	if out[2].StartLine != 9 || out[2].EndLine != 9 {
		t.Fatalf("inverted span clamps to single line, got %+v", out[2])
	}
}
