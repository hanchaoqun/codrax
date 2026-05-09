package types

import (
	"context"
	"reflect"
	"testing"
)

// TestBusContextProjection_AllTypedSignalsPropagated_ToolBusContext
// pins the contract: every field name in projectionTypedSignalFields
// MUST be readable on AgentContext (the source) and BusContext (the
// destination), AND ToolBusContext must propagate every one of them
// from a fully-populated AgentContext into the resulting BusContext.
//
// Regression target: tonight's mr_pin_isolation eval surfaced that
// buildToolBusContext silently dropped MultiGraph / TypedDenials /
// PendingSubRepos / Ctx, so the L1 active-set gate's nil-check
// short-circuited and the LLM read cross-pin sub-repo content.
//
// Adding a new typed-signal field to BusContext + AgentContext
// requires:
//  1. Adding the field name to projectionTypedSignalFields
//  2. Adding the field to ToolBusContext + SubAgentContext
//
// Skipping step 2 fails this test. Skipping step 1 fails the test on
// the next typed-signal field added — keeps the contract tight.
func TestBusContextProjection_AllTypedSignalsPropagated_ToolBusContext(t *testing.T) {
	ac := &AgentContext{}
	for _, fieldName := range projectionTypedSignalFields {
		if !setNonZeroFieldOnAgentContext(t, ac, fieldName) {
			t.Errorf("projectionTypedSignalFields lists %q but AgentContext has no such field "+
				"(or no non-zero sentinel handler for its type) — keep both ends in sync",
				fieldName)
		}
	}

	bc := ToolBusContext(ac, AgentName("test-agent"))
	if bc == nil {
		t.Fatal("ToolBusContext returned nil for non-nil AgentContext")
	}

	for _, fieldName := range projectionTypedSignalFields {
		if !nonZeroFieldOnBusContext(t, bc, fieldName) {
			t.Errorf("ToolBusContext dropped typed signal %q — buildToolBusContext narrowing "+
				"must propagate this field; the L1 active-set gate / TypedDenials sanitisation / "+
				"REPL Memory access depends on it",
				fieldName)
		}
	}

	// ActiveAgent override sanity — the helper falls back to ctx.AgentName
	// when the override is empty, otherwise uses the override.
	if bc.ActiveAgent != AgentName("test-agent") {
		t.Errorf("ToolBusContext active-agent override: got %q, want %q",
			bc.ActiveAgent, "test-agent")
	}
}

// TestBusContextProjection_AllTypedSignalsPropagated_SubAgentContext
// pins the same contract for the BusContext → AgentContext direction.
// Mutable is intentionally dropped so we don't include it in the
// typed-signal list; its absence is verified by a separate assertion.
func TestBusContextProjection_AllTypedSignalsPropagated_SubAgentContext(t *testing.T) {
	bc := &BusContext{}
	for _, fieldName := range projectionTypedSignalFields {
		if !setNonZeroFieldOnBusContext(t, bc, fieldName) {
			t.Errorf("projectionTypedSignalFields lists %q but BusContext has no such field "+
				"(or no non-zero sentinel handler for its type) — keep both ends in sync",
				fieldName)
		}
	}
	// Mutable on parent must NOT propagate (sub-agents are read-only).
	bc.Mutable = NewMutableState("test-objective")

	ac := SubAgentContext(bc, &SubAgentRequest{SubAgent: "test-sub"})
	if ac == nil {
		t.Fatal("SubAgentContext returned nil for non-nil BusContext")
	}

	for _, fieldName := range projectionTypedSignalFields {
		if !nonZeroFieldOnAgentContext(t, ac, fieldName) {
			t.Errorf("SubAgentContext dropped typed signal %q — sub-agent dispatch must "+
				"preserve all gate-relevant typed signals; the L1 active-set gate / TypedDenials "+
				"sanitisation / cancellation Ctx depends on it",
				fieldName)
		}
	}
	// Mutable drop assertion — the explicit black-list entry.
	if ac.Mutable != nil {
		t.Error("SubAgentContext leaked parent Mutable — sub-agents must not mutate parent state")
	}
}

// setNonZeroFieldOnAgentContext sets the named field on an
// AgentContext to a non-zero sentinel value. Returns false when the
// field name is unrecognised. The sentinel values match the fields'
// runtime types exactly so the read-side check (nonZeroFieldOnX)
// agrees on equality.
func setNonZeroFieldOnAgentContext(t *testing.T, ac *AgentContext, fieldName string) bool {
	t.Helper()
	switch fieldName {
	case "MultiGraph":
		ac.MultiGraph = struct{ marker string }{"test-mg"}
	case "SubRepos":
		ac.SubRepos = []SubRepoSnapshot{{Slug: "test"}}
	case "ActiveSubRepo":
		ac.ActiveSubRepo = &SubRepoSnapshot{Slug: "test-active"}
	case "PendingSubRepos":
		ac.PendingSubRepos = []string{"pending-a"}
	case "MultiRepoInactivePreviewCount":
		ac.MultiRepoInactivePreviewCount = 7
	case "TypedDenials":
		td := TypedDenialSet{}
		td.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "test/sentinel.go"})
		ac.TypedDenials = &td
	case "Memory":
		ac.Memory = nopMemoryReader{}
	case "Ctx":
		ac.Ctx = context.Background()
	case "EnvFacts":
		ac.EnvFacts = &EnvFacts{}
	case "EnvRecommendSettings":
		ac.EnvRecommendSettings = EnvRecommendSettings{Enabled: true}
	default:
		return false
	}
	return true
}

// setNonZeroFieldOnBusContext mirrors setNonZeroFieldOnAgentContext
// for BusContext. Note TypedDenials is a value (not pointer) on
// BusContext, so the sentinel uses an empty value (the test asserts
// the value SHAPE propagates, not specific contents — TypedDenials
// is shared via pointer in the AgentContext direction).
func setNonZeroFieldOnBusContext(t *testing.T, bc *BusContext, fieldName string) bool {
	t.Helper()
	switch fieldName {
	case "MultiGraph":
		bc.MultiGraph = struct{ marker string }{"test-mg"}
	case "SubRepos":
		bc.SubRepos = []SubRepoSnapshot{{Slug: "test"}}
	case "ActiveSubRepo":
		bc.ActiveSubRepo = &SubRepoSnapshot{Slug: "test-active"}
	case "PendingSubRepos":
		bc.PendingSubRepos = []string{"pending-a"}
	case "MultiRepoInactivePreviewCount":
		bc.MultiRepoInactivePreviewCount = 7
	case "TypedDenials":
		// AgentContext's TypedDenials is a pointer; the sub-agent
		// helper points it back at bus.TypedDenials. To verify the
		// pointer non-nil after projection, leave the BusContext
		// value as zero — the helper's `&bus.TypedDenials` always
		// produces a non-nil pointer regardless of the value.
	case "Memory":
		bc.Memory = nopMemoryReader{}
	case "Ctx":
		bc.Ctx = context.Background()
	case "EnvFacts":
		bc.EnvFacts = &EnvFacts{}
	case "EnvRecommendSettings":
		bc.EnvRecommendSettings = EnvRecommendSettings{Enabled: true}
	default:
		return false
	}
	return true
}

// nonZeroFieldOnBusContext reports whether the named typed-signal
// field is non-zero on the BusContext. Uses reflect for slice / map /
// pointer / interface fields where direct comparison is awkward.
func nonZeroFieldOnBusContext(t *testing.T, bc *BusContext, fieldName string) bool {
	t.Helper()
	v := reflect.ValueOf(bc).Elem().FieldByName(fieldName)
	if !v.IsValid() {
		t.Errorf("BusContext has no field %q", fieldName)
		return false
	}
	return !v.IsZero()
}

// nonZeroFieldOnAgentContext mirrors nonZeroFieldOnBusContext for
// AgentContext. Special-case TypedDenials: the helper sets it to
// `&bus.TypedDenials`, which is non-nil even when bus.TypedDenials
// is zero-value. So the pointer being non-nil is the proof of
// propagation.
func nonZeroFieldOnAgentContext(t *testing.T, ac *AgentContext, fieldName string) bool {
	t.Helper()
	v := reflect.ValueOf(ac).Elem().FieldByName(fieldName)
	if !v.IsValid() {
		t.Errorf("AgentContext has no field %q", fieldName)
		return false
	}
	return !v.IsZero()
}

// nopMemoryReader satisfies the MemoryReader interface with no-op
// behavior. Used as a sentinel non-nil value for Memory propagation
// tests.
type nopMemoryReader struct{}

func (nopMemoryReader) Search(query string, opts MemorySearchOpts) []MemoryIndexEntry {
	return nil
}
