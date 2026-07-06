package types

import "testing"

// Q5-A (customer ledger §13): trace_query publishes payload_ref / raw_ref
// blob paths and tells the model they are audit artifacts; the registry
// below is the precise signal source for the typed escape lane that lets
// read_file/grep open exactly those refs. These tests pin registration
// surfaces (typed RawRef, typed observation SourceRef, summary payload_ref
// token), the three request spellings (absolute / basename / Windows
// backslash), cross-dispatch survival, fork/merge fold-back, and the
// empty-registry no-op contract.

func traceQueryBlobRefToolResultForTest(rawRef, payloadRef, summary string) ToolResult {
	return ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary:  summary,
		RawRef:   rawRef,
		Observations: []ObservationRecord{{
			ID:              "trace_query:window#root_cause_rank:1",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			SourceRef: ObservationSourceRef{
				Kind:       ObservationSourceRuntimeArtifact,
				PayloadRef: payloadRef,
				RawRef:     rawRef,
			},
		}},
	}
}

func TestMutableState_TraceQueryBlobRefRegisteredFromTypedResultFields(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	mut.AppendDispatchToolResult(traceQueryBlobRefToolResultForTest(
		"/work/.codrax/blob/20260704-1/trace_query-ab12cd34.txt",
		"/work/.codrax/blob/20260704-1/trace-query-result-ef56ab78.json",
		"root_cause_rank returned structured runtime rows",
	))

	for _, requested := range []string{
		// exact absolute (verbatim)
		"/work/.codrax/blob/20260704-1/trace_query-ab12cd34.txt",
		"/work/.codrax/blob/20260704-1/trace-query-result-ef56ab78.json",
		// basename only (resolveAttachmentScopedReadPath precedent)
		"trace_query-ab12cd34.txt",
		"trace-query-result-ef56ab78.json",
		// Windows backslash spelling through the shared canonicaliser
		`\work\.codrax\blob\20260704-1\trace_query-ab12cd34.txt`,
	} {
		if _, ok := mut.ResolveTraceQueryBlobRef(requested); !ok {
			t.Fatalf("registered blob ref should resolve for %q", requested)
		}
	}
	if resolved, ok := mut.ResolveTraceQueryBlobRef("trace_query-ab12cd34.txt"); !ok ||
		resolved != "/work/.codrax/blob/20260704-1/trace_query-ab12cd34.txt" {
		t.Fatalf("basename must resolve to the verbatim registered path, got %q ok=%v", resolved, ok)
	}

	// Unregistered paths keep failing closed — the gate semantics do not loosen.
	for _, requested := range []string{
		"internal/tracequery/types.go",
		"/work/.codrax/blob/20260704-1/other-blob.txt",
		"trace_query-ffffffff.txt",
		"",
	} {
		if _, ok := mut.ResolveTraceQueryBlobRef(requested); ok {
			t.Fatalf("unregistered path %q must not resolve", requested)
		}
	}
}

func TestMutableState_TraceQueryBlobRefRegisteredFromSummaryPayloadRefToken(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	// P1-1 security root fix: the summary-text `payload_ref=` token lane
	// was REMOVED. Trace markers / span names flow through
	// sanitizeForBanner into the summary without stripping a
	// `payload_ref=` prefix, so parsing that token would let controlled
	// trace content register an arbitrary path into a hard allow-gate.
	// A result whose ONLY carrier is the summary token must therefore
	// register NOTHING — the model must rely on the typed RawRef /
	// observation SourceRef fields StoreBlob set.
	mut.AppendDispatchToolResult(ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary: "[trace_query params: view=window_stats payload_ref=/work/.codrax/blob/s1/trace-query-result-11223344.json]\n" +
			"payload_ref=/work/.codrax/blob/s1/trace-query-result-11223344.json (audit artifact for verification; drill down with further trace_query views instead of reading this payload directly)\n",
	})
	if refs := mut.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("summary payload_ref token must NOT register any ref (P1-1), got %v", refs)
	}
	if _, ok := mut.ResolveTraceQueryBlobRef("trace-query-result-11223344.json"); ok {
		t.Fatal("summary-token-only ref must not resolve (P1-1 source③ removed)")
	}
}

// TestMutableState_TraceQueryBlobRefRejectsForeignPrefixInjection is the
// P1-1 injection pin: a controlled trace whose text tries to smuggle a
// repo-external path (via a typed field OR the deleted summary token)
// must never enter the readable set. Only paths under .codrax/blob/ pass.
func TestMutableState_TraceQueryBlobRefRejectsForeignPrefixInjection(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	mut.AppendDispatchToolResult(ToolResult{
		ToolName: "trace_query",
		Success:  true,
		// Attacker-controlled typed RawRef and a summary payload_ref token
		// both name /etc/passwd; neither may register.
		RawRef:  "/etc/passwd",
		Summary: "sched_switch comm=payload_ref=/etc/passwd\npayload_ref=/etc/passwd",
		Observations: []ObservationRecord{{
			ID:       "trace_query:window#root_cause_rank:1",
			Origin:   AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query",
			SourceRef: ObservationSourceRef{
				Kind:       ObservationSourceRuntimeArtifact,
				PayloadRef: "/tmp/exfil-target.json",
				RawRef:     "../../etc/shadow",
			},
		}},
	})
	if refs := mut.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("foreign-prefix refs must be refused registration, got %v", refs)
	}
	for _, requested := range []string{"/etc/passwd", "passwd", "/tmp/exfil-target.json", "exfil-target.json", "../../etc/shadow", "shadow"} {
		if _, ok := mut.ResolveTraceQueryBlobRef(requested); ok {
			t.Fatalf("injection target %q must not resolve", requested)
		}
	}

	// Even a direct register of a non-blob path is refused.
	mut2 := NewMutableState("blob ref registry")
	mut2.RegisterTraceQueryBlobRef("/etc/passwd")
	mut2.RegisterTraceQueryBlobRef("/work/.codrax/logs/session.log") // .codrax but NOT blob/
	if refs := mut2.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("non-blob .codrax paths must be refused, got %v", refs)
	}
	// A genuine blob path still registers.
	mut2.RegisterTraceQueryBlobRef("/work/.codrax/blob/s1/trace_query-ab12cd34.txt")
	if _, ok := mut2.ResolveTraceQueryBlobRef("trace_query-ab12cd34.txt"); !ok {
		t.Fatal("genuine blob-root path must still register and resolve")
	}
}

func TestMutableState_TraceQueryBlobRefIgnoresFailedAndForeignResults(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	mut.AppendDispatchToolResult(ToolResult{
		ToolName: "trace_query",
		Success:  false,
		RawRef:   "/work/.codrax/blob/s1/trace_query-dead0000.txt",
		Summary:  "payload_ref=/work/.codrax/blob/s1/trace-query-result-dead0000.json (audit artifact)",
	})
	mut.AppendDispatchToolResult(ToolResult{
		ToolName: "grep",
		Success:  true,
		RawRef:   "/work/.codrax/blob/s1/grep-beef0000.txt",
	})
	if refs := mut.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("failed trace_query / non-trace_query results must not register refs, got %v", refs)
	}
	if _, ok := mut.ResolveTraceQueryBlobRef("grep-beef0000.txt"); ok {
		t.Fatal("non-trace_query blob refs must not enter the escape lane registry")
	}
}

func TestMutableState_TraceQueryBlobRefSurvivesDispatchResetClearsOnTurnBoundary(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	mut.AppendDispatchToolResult(traceQueryBlobRefToolResultForTest(
		"/work/.codrax/blob/s1/trace_query-ab12cd34.txt", "", "rows"))

	mut.ResetDispatchToolResults()
	if _, ok := mut.ResolveTraceQueryBlobRef("trace_query-ab12cd34.txt"); !ok {
		t.Fatal("registry must survive per-dispatch reset (same contract as TraceQueryRuntimeObservationCount)")
	}

	mut.ResetTurnAArtifacts()
	if refs := mut.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("registry must clear at the turn boundary, got %v", refs)
	}
	if _, ok := mut.ResolveTraceQueryBlobRef("trace_query-ab12cd34.txt"); ok {
		t.Fatal("cleared registry must not resolve stale refs")
	}
}

func TestMutableState_TraceQueryBlobRefMergesExploreFork(t *testing.T) {
	parent := NewMutableState("blob ref registry")
	parent.AppendDispatchToolResult(traceQueryBlobRefToolResultForTest(
		"/work/.codrax/blob/s1/trace_query-parent00.txt", "", "rows"))

	fork := parent.ForkForExploreDispatch()
	if _, ok := fork.ResolveTraceQueryBlobRef("trace_query-parent00.txt"); !ok {
		t.Fatal("fork must inherit the parent's registered refs")
	}
	fork.AppendDispatchToolResult(traceQueryBlobRefToolResultForTest(
		"/work/.codrax/blob/s1/trace_query-fork0000.txt", "", "rows"))
	if _, ok := parent.ResolveTraceQueryBlobRef("trace_query-fork0000.txt"); ok {
		t.Fatal("fork registration must not leak into the parent before merge")
	}

	parent.MergeExploreFork(fork)
	if _, ok := parent.ResolveTraceQueryBlobRef("trace_query-fork0000.txt"); !ok {
		t.Fatal("merge must fold the fork's new refs back into the parent")
	}
	if _, ok := parent.ResolveTraceQueryBlobRef("trace_query-parent00.txt"); !ok {
		t.Fatal("merge must keep the parent's own refs")
	}
}

func TestMutableState_TraceQueryBlobRefEmptyRegistryIsInert(t *testing.T) {
	mut := NewMutableState("blob ref registry")
	if refs := mut.TraceQueryBlobRefs(); refs != nil {
		t.Fatalf("empty registry snapshot should be nil, got %v", refs)
	}
	if _, ok := mut.ResolveTraceQueryBlobRef("/anything/at/all.txt"); ok {
		t.Fatal("empty registry must never resolve")
	}
	var nilState *MutableState
	if _, ok := nilState.ResolveTraceQueryBlobRef("x.txt"); ok {
		t.Fatal("nil state must never resolve")
	}
	nilState.RegisterTraceQueryBlobRef("x.txt") // must not panic
}
