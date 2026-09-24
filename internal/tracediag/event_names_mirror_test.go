package tracediag

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestEventNamesMirrorScriptAndRun(t *testing.T) {
	for _, version := range []string{"1", "2"} {
		script, err := ParseScript([]byte("version: " + version + "\nsteps:\n  - {label: named, view: event_search, event_names: [block_bio_queue, block_bio_queue, BLOCK_BIO_QUEUE], max_lines: 30}\n"))
		if err != nil {
			t.Fatal(err)
		}
		step := &script.Steps[0]
		want := []string{"block_bio_queue", "BLOCK_BIO_QUEUE"}
		if !reflect.DeepEqual(step.EventNames, want) {
			t.Fatalf("script folded case or lost exact names: %q", step.EventNames)
		}
		q := stepQuery(step, tracequery.TraceFlavorAuto)
		if !reflect.DeepEqual(q.EventNames, want) {
			t.Fatal("step mapping lost names")
		}
		q.EventNames[0] = "changed"
		if !reflect.DeepEqual(step.EventNames, want) {
			t.Fatal("step/query slice alias")
		}
	}
	for _, fields := range []string{
		"view: window_stats, event_names: [block_bio_queue]",
		"view: event_search, event_names: [' ']",
		"view: event_search, event_names: [" + strings.Repeat("x,", tracequery.EventSearchNameLimit) + "x]",
	} {
		_, err := ParseScript([]byte("version: 1\nsteps:\n  - {label: bad, " + fields + ", max_lines: 30}\n"))
		if err == nil || !strings.Contains(err.Error(), "event_names") {
			t.Fatalf("shared invalid-name contract ignored: %v", err)
		}
	}
	scriptPath, tracePath, _ := writeRunFixtures(t, "version: 1\nsteps:\n  - {label: named, view: event_search, event_names: [block_bio_queue], max_lines: 30}\n")
	body := "reader-40 (40) [001] .... 2.000000: block_rq_issue: 8,0 R 4096 () 100 + 8 [reader]\n" +
		"bio-50 (50) [002] .... 2.015000: block_bio_queue: 8,0 R 100 + 8 [bio]\n"
	if err := os.WriteFile(tracePath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &report)
	if err != nil || failed != 0 {
		t.Fatalf("public script failed: %v %s", err, report.String())
	}
	for _, want := range []string{`event_names=["block_bio_queue"]`, "matched=1 emitted=1", "block_bio_queue"} {
		if !strings.Contains(report.String(), want) {
			t.Errorf("report missing %q: %s", want, report.String())
		}
	}
	if strings.Contains(report.String(), "2.000000: block_rq_issue") {
		t.Fatal("raw-name query broadened into category")
	}
}
