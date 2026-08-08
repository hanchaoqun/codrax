package types

import "testing"

func TestNormalizedSurfaceSymbolTailReceiverOperators(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "cxx pointer member call", raw: "sink_->write()", want: "write"},
		{name: "qualified cxx pointer member", raw: "logx::holder->flush()", want: "flush"},
		{name: "php receiver member", raw: "$registry->create()", want: "create"},
		{name: "nested member after pointer", raw: "state->handler.run()", want: "run"},
		{name: "plain arrow is not an identity", raw: "caller -> callee", want: "caller"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizedSurfaceSymbolTail(tc.raw); got != tc.want {
				t.Fatalf("NormalizedSurfaceSymbolTail(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
