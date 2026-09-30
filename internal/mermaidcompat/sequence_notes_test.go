package mermaidcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestSequenceMultilineNotesPreserveTextAndTopology(t *testing.T) {
	in := "sequenceDiagram\n    participant T as target-41\n    Note over T: 10.001–10.004s\n    S态睡眠 (3ms)\n    等待唤醒\n    W-->>T: 唤醒 10.004s\n    Note left of T: 状态\n      Runnable (0.5ms)\n    Note over T,W: 下一阶段\n    IO等待 (3ms)\n"
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{"Note over T: 10.001–10.004s<br/>S态睡眠 (3ms)<br/>等待唤醒", "Note left of T: 状态<br/>Runnable (0.5ms)", "Note over T,W: 下一阶段<br/>IO等待 (3ms)", "participant T as target-41"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if !reflect.DeepEqual(ParseEdges(in), ParseEdges(got)) ||
		!reflect.DeepEqual(SequenceParticipantReferences(in), SequenceParticipantReferences(got)) {
		t.Fatal("note repair changed topology or participants")
	}
	if again := NormalizeSourceForMarkdown(got); again != got {
		t.Fatal("note repair is not idempotent")
	}
}

func TestSequenceMultilineNotesDoNotSwallowSyntax(t *testing.T) {
	for _, next := range []string{
		"A->>B: request", "A-->>B: reply", "A->>B", "A --> B", "participant A as Worker", "actor B",
		"Note over A: next", "loop retry", "alt ok", "else no", "opt maybe", "par work", "and more",
		"critical hold", "option fail", "break stop", "rect rgb(0,0,0)", "end", "activate A", "deactivate A",
		"autonumber", "box Group", "link A: Docs @ URL", "links A: {}", "properties A: {}", "details A: {}",
		"title Work", "create participant C", "destroy C", "%% keep comment", "", "stop", "accTitle: title",
		"foo;A->>B: hidden", "unknown {", "}", "sequenceDiagram",
	} {
		t.Run(next, func(t *testing.T) {
			in := "sequenceDiagram\n    Note over A: keep\n    " + next + "\n"
			if got := NormalizeSequenceMultilineNotes(in); got != in {
				t.Fatalf("swallowed syntax: %q => %q", in, got)
			}
		})
	}
	for _, in := range []string{
		"flowchart LR\n    Note over A: keep\n    continuation\n",
		"sequenceDiagram\n    Note over A\n    continuation\n",
		"sequenceDiagram\n    Note over A:\n    continuation\n",
		"sequenceDiagram\n    Note over A: keep\n  dedented text\n",
	} {
		if got := NormalizeSequenceMultilineNotes(in); got != in {
			t.Fatalf("ambiguous source changed: %q", got)
		}
	}
}

func TestSequenceBrowserSourceDoesNotAcquireTerminalQuotes(t *testing.T) {
	in := "sequenceDiagram\n participant A as target-41\n participant B as Orchestrator.Run\n actor C as User Name\n A->>B: call\n"
	if got := NormalizeSourceForMarkdown(in); got != in {
		t.Fatalf("terminal-only label quoting leaked into shared source: %s", got)
	}
}
