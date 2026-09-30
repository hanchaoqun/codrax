package tool

import "testing"

func TestEmitRuntimeThreadIdentityNormalizesBeforeHandoff(t *testing.T) {
	confidence := .95
	for _, thread := range []string{"ui-10 (tid=10)", "ui [10]", "ui 10", "tid=10"} {
		got, why, ok := parseRuntimeTarget(emitRuntimeTargetParam{Kind: "thread", Thread: thread, Confidence: &confidence})
		if !ok || got.PID != 10 || (got.Thread != "ui" && got.Thread != "") {
			t.Fatalf("%q: %+v %s %t", thread, got, why, ok)
		}
	}
	pid := 11
	negative := -1
	for _, input := range []emitRuntimeTargetParam{
		{Kind: "thread", Thread: "ui-11 (tid=10)", Confidence: &confidence},
		{Kind: "thread", Thread: "ui (tid=10)", PID: &pid, Confidence: &confidence},
		{Kind: "thread", Thread: "ui (tid=10)", PID: &negative, Confidence: &confidence},
	} {
		if _, why, ok := parseRuntimeTarget(input); ok || why == "" {
			t.Fatal("contradiction not repairable")
		}
	}
}
