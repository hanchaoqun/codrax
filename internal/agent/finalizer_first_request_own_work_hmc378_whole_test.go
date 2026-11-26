package agent

import (
	"os"
	"strings"
	"testing"
)

// Own-work pricing in the first finalizer request, complete original capture
// (opt-in; skipped without the variable, failing when the file is not the
// pinned original). The capture is one no evaluation case reads: Honeybird,
// device-reboot cold start 5, main thread 10566, the closed bindApplication
// window. Expected numbers are the standalone raw oracle's (the thread's own
// B/E markers and sched_switch records in the ftrace text): 129
// class-verification instances, 37.529 ms as their union, the largest
// 4.784 ms, 33.090 ms of the union on a CPU.
//
// The pin reads facts, not layout: for each fact, at least one request line
// must carry the target, the semantic class and the number together. Which
// section a line sits in and how the section is worded are free to change.
func TestHMC378WholeOriginalOwnWorkFirstRequest(t *testing.T) {
	c := hmc377Capture{"honeybird_reboot5", "CODRAX_HMC377_HONEYBIRD_REBOOT",
		"54405241db707b901057b41a5c94e69a3fc233425fdf7a1f745e43585cf248d4", 124269104, 10566, 162.372184, 163.616645}
	path := hmc377PinnedCapture(t, c)
	ctx := hmc377CaptureFinalizerContext(t, c, path)
	messages, _ := hmc377CaptureFirstRequest(t, ctx)
	var request strings.Builder
	for _, message := range messages {
		request.WriteString(message.Content)
		request.WriteString("\n")
	}
	if dir := os.Getenv("CODRAX_HMC377_FIRST_REQUEST_DUMP"); dir != "" {
		if err := os.WriteFile(dir+"/"+c.name+".own-work.request.txt", []byte(request.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(request.String(), "\n")
	carried := func(parts ...string) bool {
		for _, line := range lines {
			all := true
			for _, part := range parts {
				all = all && strings.Contains(line, part)
			}
			if all {
				return true
			}
		}
		return false
	}
	const target = "-10566"
	for _, fact := range []struct {
		name  string
		parts []string
	}{
		{"the own verification row is the union of its 129 records", []string{target, "class_verification", "37.529ms", "129 measurement records"}},
		{"the largest record is 4.784 ms", []string{target, "class_verification", "37.529ms", "record maximum 4.784 ms"}},
		{"the semantic inventory lists the family with its total and count", []string{target, "semantic_class=`class_verification`", "total=37.529ms", "occurrences=129"}},
		{"the on-CPU share of the union is a physical overlap with running time, never an addend", []string{target, "33.090ms", "semantic_member_intervals"}},
	} {
		if !carried(fact.parts...) {
			t.Errorf("first request lost a fact: %s (no line carries %q together)", fact.name, fact.parts)
		}
	}
}
