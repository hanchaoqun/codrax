package types

import "testing"

func TestReservedRuntimeArtifactBlobKind(t *testing.T) {
	tests := map[string]string{
		"attached_trace.txt":                "trace",
		"attached_trace-44d2a269.txt":       "trace",
		"ATTACHED_HITRACE-A1B2C3D4.TXT":     "trace",
		"/tmp/attached_atrace-00000000.txt": "trace",
		"attached_log-deadbeef.txt":         "log",
		"attached_trace-short.txt":          "",
		"attached_trace-zzzzzzzz.txt":       "",
		"some_trace-44d2a269.txt":           "",
	}
	for input, want := range tests {
		if got := ReservedRuntimeArtifactBlobKind(input); got != want {
			t.Errorf("ReservedRuntimeArtifactBlobKind(%q)=%q, want %q", input, got, want)
		}
		if got := RuntimeArtifactPathKind(input); got != want {
			t.Errorf("RuntimeArtifactPathKind(%q)=%q, want %q", input, got, want)
		}
	}
}
