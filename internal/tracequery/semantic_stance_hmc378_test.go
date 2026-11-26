package tracequery

import "testing"

// The stance of a classified marker — does the recording thread execute the
// work, or is it parked until someone else's work ends — is a property of the
// marker name. Whichever layer of the classifier owns the class (a built-in
// arm, a vocabulary row, an operator pattern), the name has one stance, and
// every consumer reads that one decision.

type hmc378StanceCase struct {
	name  string
	class string
	owner traceSpanSemanticOwner
	waits bool
}

// The matrix: every name the engine classifies has exactly one stance, and
// the two predicates the lanes read are each other's negation on it.
func TestHMC378StanceMatrixIsOwnerIndependent(t *testing.T) {
	const builtIn, vocabulary = traceSpanSemanticOwnerBuiltIn, traceSpanSemanticOwnerPlatformVocabulary
	for _, c := range []hmc378StanceCase{
		// A vocabulary row whose stance is "waits" owns the name.
		{"GC: Wait For Completion Alloc", "gc_pause", vocabulary, true},
		{"GC: Wait For Completion Background", "gc_pause", vocabulary, true},
		{"H:SuspendTime::WaitGCFinished", "gc_pause", vocabulary, true},
		// The built-in collection arm owns the name through one of its
		// compound shapes; the shape itself says the thread waits.
		{"WaitForGcToComplete", "gc_pause", builtIn, true},
		{"H:WaitForGcToComplete", "gc_pause", builtIn, true},
		{"wait_for_gc_to_complete", "gc_pause", builtIn, true},
		{"Wait For GC To Complete", "gc_pause", builtIn, true},
		{"WaitForGcToComplete (kGcCauseForAlloc)", "gc_pause", builtIn, true},
		// The built-in collection arm owns the name because of a word of
		// its tail; the operation head is the vocabulary's wait row. One
		// marker family must not have two stances by tail word.
		{"GC: Wait For Completion collector", "gc_pause", builtIn, true},
		{"GC: Wait For Completion Concurrent mark", "gc_pause", builtIn, true},
		{"GC: Wait For Completion Full", "gc_pause", builtIn, true},
		{"GC: Wait For Completion Young", "gc_pause", builtIn, true},
		// Compound shapes with no wait in them, and every other built-in
		// arm: the thread executes the work.
		{"SuspendAllForGC", "gc_pause", builtIn, false},
		{"suspend_all_for_gc", "gc_pause", builtIn, false},
		{"CollectGarbage", "gc_pause", builtIn, false},
		{"collect garbage", "gc_pause", builtIn, false},
		{"GC", "gc_pause", builtIn, false},
		{"GC pause", "gc_pause", builtIn, false},
		{"GC: Young collection", "gc_pause", builtIn, false},
		{"Garbage Collection", "gc_pause", builtIn, false},
		{"VerifyClass com.example.Foo", "class_verification", builtIn, false},
		{"JIT compiling void pkg.Entry.run() (kind=Baseline)", "jit_compile", builtIn, false},
		{"H:Ark runtime compile", "runtime_compile", builtIn, false},
		{"H:Load/Compile Shaders", "shader_compile", builtIn, false},
		{"Texture upload(15283) 512x194", "texture_upload", builtIn, false},
		// Vocabulary rows whose stance is "executes".
		{"H:FullGC::RunPhases;GCReason2", "gc_pause", vocabulary, false},
		{"H:PartialGC::RunPhases;GCReason1", "gc_pause", vocabulary, false},
		{"H:DDGR CreateShaderModules pipelineKey = 1", "shader_compile", vocabulary, false},
		// A typed source speaks only for the family the classifier gave the
		// name. A class-verification marker whose head happens to be a
		// collection wait's keeps the stance of class verification: neither
		// the vocabulary's wait row nor the collection arm's wait shape is
		// about it.
		{"GC: Wait For Completion VerifyClass com.example.Foo", "class_verification", builtIn, false},
		{"WaitForGcToComplete VerifyClass com.example.Foo", "class_verification", builtIn, false},
	} {
		work, owner, ok := traceSpanSemanticWorkClassWithOwner(c.name)
		if !ok || work.SemanticClass != c.class || owner != c.owner {
			t.Errorf("%q: class=%q owner=%d ok=%v, want %q owned by %d", c.name, work.SemanticClass, owner, ok, c.class, c.owner)
			continue
		}
		if got := semanticSpanRecordsWait(c.name); got != c.waits {
			t.Errorf("%q (%s): records a wait = %v, want %v", c.name, c.class, got, c.waits)
		}
		if semanticSpanAdmitsHostEdgeLane(c.name) == semanticSpanRecordsWait(c.name) {
			t.Errorf("%q: the host-edge bar and the stance disagree (admits=%v, records a wait=%v)",
				c.name, semanticSpanAdmitsHostEdgeLane(c.name), semanticSpanRecordsWait(c.name))
		}
	}
	// A name with no class has no stance: it is no wait, and the host-edge
	// bar (which only ever refuses a recorded wait) does not apply to it.
	for _, name := range []string{
		"", "Choreographer#doFrame", "GCLockerMetrics", "Background concurrent copying GC",
		"H:GC::WaitFinished", "WaitForGcToCompleteLocked", "waitforgc",
	} {
		if class := traceSpanSemanticClass(name); class != "" {
			t.Errorf("%q: must stay unclassified, got %q", name, class)
		}
		if semanticSpanRecordsWait(name) || !semanticSpanAdmitsHostEdgeLane(name) {
			t.Errorf("%q: an unclassified name records no wait and is not barred", name)
		}
	}
}

// An operator pattern never changes a stance. A name the vocabulary's wait row
// owns still records a wait when a pattern classifies it too, and a name only
// a pattern classifies executes (a pattern carries no stance).
func TestHMC378OperatorPatternDoesNotChangeStance(t *testing.T) {
	SetSemanticSpanPatterns([]SemanticSpanPattern{
		{SemanticClass: "runtime_compile", Contains: []string{"wait for completion"}},
		{SemanticClass: "gc_pause", Contains: []string{"vendor heap wait"}},
	})
	t.Cleanup(func() { SetSemanticSpanPatterns(nil) })
	for _, c := range []hmc378StanceCase{
		{"GC: Wait For Completion Alloc", "gc_pause", traceSpanSemanticOwnerPlatformVocabulary, true},
		{"vendor heap wait 3", "gc_pause", traceSpanSemanticOwnerOperatorPattern, false},
		{"task wait for completion", "runtime_compile", traceSpanSemanticOwnerOperatorPattern, false},
		{"WaitForGcToComplete", "gc_pause", traceSpanSemanticOwnerBuiltIn, true},
	} {
		work, owner, ok := traceSpanSemanticWorkClassWithOwner(c.name)
		if !ok || work.SemanticClass != c.class || owner != c.owner {
			t.Fatalf("%q: class=%q owner=%d ok=%v, want %q owned by %d", c.name, work.SemanticClass, owner, ok, c.class, c.owner)
		}
		if got := semanticSpanRecordsWait(c.name); got != c.waits {
			t.Errorf("%q: records a wait = %v, want %v", c.name, got, c.waits)
		}
		if semanticSpanAdmitsHostEdgeLane(c.name) == semanticSpanRecordsWait(c.name) {
			t.Errorf("%q: the host-edge bar and the stance disagree", c.name)
		}
	}
	// The pattern does move the name into the built-in seat pool, as before:
	// seat bookkeeping and stance are separate questions.
	if semanticSpanMintedByPlatformVocabulary("GC: Wait For Completion Alloc") {
		t.Fatal("a name an operator pattern also classifies sits in the built-in seat pool")
	}
}

// The stance decision itself: every classified name has exactly one of the two
// stances and an unclassified name has none. A built-in name whose head only
// matches a vocabulary row that states no stance (a nested step, a related
// operation) is work its thread executes: such a row describes no unit of the
// family's work, so it has nothing to say about the recording thread.
func TestHMC378StanceDecisionIsTotalOnClassifiedNames(t *testing.T) {
	SetSemanticSpanPatterns([]SemanticSpanPattern{{SemanticClass: "gc_pause", Contains: []string{"vendor heap wait"}}})
	t.Cleanup(func() { SetSemanticSpanPatterns(nil) })
	for _, c := range []struct {
		name  string
		owner traceSpanSemanticOwner
		want  semanticVocabStance
	}{
		{"GC: Wait For Completion Alloc", traceSpanSemanticOwnerPlatformVocabulary, semanticVocabWaits},
		{"H:FullGC::RunPhases;GCReason2", traceSpanSemanticOwnerPlatformVocabulary, semanticVocabExecutes},
		{"WaitForGcToComplete", traceSpanSemanticOwnerBuiltIn, semanticVocabWaits},
		{"GC: Wait For Completion collector", traceSpanSemanticOwnerBuiltIn, semanticVocabWaits},
		{"SuspendAllForGC", traceSpanSemanticOwnerBuiltIn, semanticVocabExecutes},
		{"VerifyClass com.example.Foo", traceSpanSemanticOwnerBuiltIn, semanticVocabExecutes},
		// The collection arm owns the name; its head is the vocabulary's
		// step namespace "gc::", a row without a stance.
		{"H:GC::Mark", traceSpanSemanticOwnerBuiltIn, semanticVocabExecutes},
		// The shader arm owns the name; its head is an executes row of the
		// same family.
		{"H:DDGR CreateShaderModules compile", traceSpanSemanticOwnerBuiltIn, semanticVocabExecutes},
		{"vendor heap wait 3", traceSpanSemanticOwnerOperatorPattern, semanticVocabExecutes},
		{"Choreographer#doFrame", traceSpanSemanticOwnerNone, 0},
		{"H:GC::WaitFinished", traceSpanSemanticOwnerNone, 0},
		{"", traceSpanSemanticOwnerNone, 0},
	} {
		if _, owner, _ := traceSpanSemanticWorkClassWithOwner(c.name); owner != c.owner {
			t.Errorf("%q: owner %d, want %d", c.name, owner, c.owner)
			continue
		}
		if got := traceSpanSemanticStance(c.name); got != c.want {
			t.Errorf("%q: stance %d, want %d", c.name, got, c.want)
		}
	}
}
