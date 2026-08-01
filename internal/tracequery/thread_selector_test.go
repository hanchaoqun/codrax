package tracequery

import "testing"

func TestParseThreadSelectorIdentityCanonicalizesAcceptedSpellings(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		wantPID  int
		wantName string
	}{
		{"2955", 2955, ""},
		{"pid=2955", 2955, ""},
		{"thread_id:2955", 2955, ""},
		{"CompThread_0 [2955]", 2955, "CompThread_0"},
		{"CompThread_0 2955", 2955, "CompThread_0"},
		{"CompThread_0-2955", 2955, "CompThread_0"},
	} {
		pid, name, ok := ParseThreadSelectorIdentity(tc.raw)
		if !ok || pid != tc.wantPID || name != tc.wantName {
			t.Fatalf("ParseThreadSelectorIdentity(%q)=(%d,%q,%t), want (%d,%q,true)",
				tc.raw, pid, name, ok, tc.wantPID, tc.wantName)
		}
	}
	for _, raw := range []string{"", "CompThread_0", "pid=0", "100-200ms"} {
		if pid, name, ok := ParseThreadSelectorIdentity(raw); ok {
			t.Fatalf("incomplete selector %q unexpectedly parsed as (%d,%q)", raw, pid, name)
		}
	}
}
