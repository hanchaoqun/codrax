package textfmt

import "testing"

func TestLineGutter(t *testing.T) {
	got := LineGutter([]string{"alpha", "", "gamma"}, 7)
	want := "     7│ alpha\n     8│ \n     9│ gamma"
	if got != want {
		t.Fatalf("LineGutter() = %q, want %q", got, want)
	}
}
