package types

import "testing"

func TestMutableStateAppendCompletionGateNoteDeduplicatesExactProducerNote(t *testing.T) {
	mu := NewMutableState("opaque")
	first := "selection remains unproven; keep it conditional"
	second := "another typed boundary"
	mu.AppendCompletionGateNote(first)
	mu.AppendCompletionGateNote(first)
	mu.AppendCompletionGateNote(second)
	if got, want := mu.TakeCompletionGateNote(), first+"; "+second; got != want {
		t.Fatalf("completion gate note = %q, want %q", got, want)
	}
	if got := mu.TakeCompletionGateNote(); got != "" {
		t.Fatalf("taking the note must clear text and dedupe state, got %q", got)
	}
	mu.AppendCompletionGateNote(first)
	if got := mu.TakeCompletionGateNote(); got != first {
		t.Fatalf("cleared dedupe state must allow a later run of the note, got %q", got)
	}
}

func TestMutableStateSetCompletionGateNoteResetsExactDedupeState(t *testing.T) {
	mu := NewMutableState("opaque")
	mu.AppendCompletionGateNote("old")
	mu.SetCompletionGateNote("replacement")
	mu.AppendCompletionGateNote("replacement")
	if got := mu.TakeCompletionGateNote(); got != "replacement" {
		t.Fatalf("set note should replace text and exact-dedupe state, got %q", got)
	}
}
