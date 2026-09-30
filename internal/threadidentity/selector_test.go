package threadidentity

import "testing"

func TestIdentitySharedSpellingsAndConflicts(t *testing.T) {
	for _, raw := range []string{"ui-10", "ui (tid=10)", "ui [10]", "ui-10 (tid=10)", "ui-10 [10]", "thread=ui thread_id:10", "ui 10"} {
		pid, name, ok := Identity(raw)
		if !ok || pid != 10 || name != "ui" {
			t.Errorf("%q -> %d/%q/%t", raw, pid, name, ok)
		}
	}
	for _, raw := range []string{"ui-11 (tid=10)", "tid=11 pid=10", "ui [11] (10)", "pid=0", "ui", "", "pid=4194305"} {
		if pid, name, ok := Identity(raw); ok {
			t.Errorf("accepted %q -> %d/%q", raw, pid, name)
		}
	}
}
