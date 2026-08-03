package textfmt

import "testing"

func TestNormalizeAttachedArtifactText(t *testing.T) {
	got := NormalizeAttachedArtifactText("a\r\nb\rc\n")
	if got != "a\nb\nc\n" {
		t.Fatalf("normalized artifact text = %q", got)
	}
}
