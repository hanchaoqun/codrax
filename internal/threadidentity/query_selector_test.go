package threadidentity

import "testing"

func TestQuerySelectorExplicitCoordinatesAndCanonicalNames(t *testing.T) {
	for _, tc := range []struct {
		pid      int
		raw      string
		wantPID  int
		wantName string
	}{
		{0, "document-worker-11 (tid=11)", 11, "document-worker"},
		{0, "worker-3 (tid=11)", 11, "worker-3"},
		{11, "worker-3", 11, "worker-3"},
		{0, "worker-3-11 (tid=11)", 11, "worker-3"},
		{11, "thread=worker tid=11 thread_id=11", 11, "worker"},
		{0, "worker [11] (11)", 11, "worker"},
		{0, "11", 11, ""},
		{0, "worker-11", 11, "worker"},
		{0, "worker", 0, "worker"},
		{0, "", 0, ""},
	} {
		pid, name, ok := QuerySelector(tc.pid, tc.raw)
		if !ok || pid != tc.wantPID || name != tc.wantName {
			t.Errorf("%d/%q -> %d/%q/%t", tc.pid, tc.raw, pid, name, ok)
		}
		pid2, name2, ok2 := QuerySelector(pid, name)
		if !ok2 || pid2 != pid || name2 != name {
			t.Errorf("canonical coordinates are not idempotent: %d/%q -> %d/%q", pid, name, pid2, name2)
		}
	}
	for _, tc := range []struct {
		pid int
		raw string
	}{
		{11, "worker tid=12"}, {11, "12"}, {0, "worker tid=11 pid=12"},
		{0, "worker [12] (tid=11)"}, {0, "worker tid=0"}, {0, "worker tid=4194305"},
		{-1, "worker"}, {MaxPID + 1, "worker"},
	} {
		if pid, name, ok := QuerySelector(tc.pid, tc.raw); ok {
			t.Errorf("conflicting/invalid explicit coordinates accepted: %d/%q -> %d/%q", tc.pid, tc.raw, pid, name)
		}
	}
}
