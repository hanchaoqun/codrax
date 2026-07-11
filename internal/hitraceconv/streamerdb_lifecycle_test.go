package hitraceconv

import (
	"reflect"
	"testing"
)

func traceDBLifecycleFixtureIndex() traceDBThreadIndex {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 10, Name: "old-owner"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 20, Name: "new-owner"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "old-name"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "new-name"}
	buildTraceDBThreadSecondaryIndexes(&index)
	return index
}

func TestTraceDBLifecycleCreationCutAndBoundarySemantics(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	if !builder.addCreation(2, 100) {
		t.Fatal("valid creation rejected")
	}
	lifecycle := builder.finalize()
	wantCuts := []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, wantCuts) {
		t.Fatalf("cuts=%+v, want %+v", lifecycle.ByTID[42].Cuts, wantCuts)
	}
	for _, test := range []struct {
		name string
		itid int64
		ts   int64
		want bool
	}{
		{name: "old before cut", itid: 1, ts: 99, want: true},
		{name: "old at cut", itid: 1, ts: 100},
		{name: "old after cut", itid: 1, ts: 101},
		{name: "new before observed creation may be prior same-itid generation", itid: 2, ts: 99, want: true},
		{name: "new at left-closed cut", itid: 2, ts: 100, want: true},
		{name: "new after cut", itid: 2, ts: 101, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := traceDBLifecycleThreadPointAllows(lifecycle, identities, test.itid, test.ts); got != test.want {
				t.Fatalf("point allows=%t, want %t", got, test.want)
			}
		})
	}
	if !traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 50, 100) {
		t.Fatal("half-open old interval ending at cut was rejected")
	}
	if traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 50, 101) {
		t.Fatal("half-open old interval crossed a cut")
	}
	if traceDBLifecycleThreadClosedEndpointAllows(lifecycle, identities, 1, 50, 100) {
		t.Fatal("closing endpoint at reused public identity cut was accepted")
	}
	if !traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 2, 100, 110) {
		t.Fatal("new interval starting at its left-closed cut was rejected")
	}
}

func TestTraceDBLifecycleAcceptsMaxInt64Timestamp(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	maxTimestamp := int64(^uint64(0) >> 1)
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addCreation(2, maxTimestamp)
	lifecycle := builder.finalize()
	want := []traceDBLifecycleBoundary{{TS: maxTimestamp, NewITID: 2, NewIPID: 2}}
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) ||
		!traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, maxTimestamp) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, maxTimestamp) {
		t.Fatalf("max-int64 lifecycle event was mistaken for an empty sentinel: %+v", lifecycle.ByTID[42])
	}
}

func TestTraceDBLifecycleTerminalUsesStrictlyLaterEarliestActivityAcrossSources(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	if !builder.addTerminal(1, 50, "X") {
		t.Fatal("valid terminal rejected")
	}
	firstSource := builder.newActivityCursor()
	if !firstSource.observe(1, 80) || !firstSource.observe(1, 50) {
		t.Fatal("valid first-source activity rejected")
	}
	secondSource := builder.newActivityCursor()
	if !secondSource.observe(1, 60) {
		t.Fatal("valid earlier second-source activity rejected")
	}
	lifecycle := builder.finalize()
	want := []traceDBLifecycleBoundary{{TS: 60, NewITID: 1, NewIPID: 1}}
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) {
		t.Fatalf("terminal restart cuts=%+v, want %+v", lifecycle.ByTID[42].Cuts, want)
	}
	if traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 55, 65) {
		t.Fatal("same-ITID reuse failed to cut a crossing source interval")
	}
	if !traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 60) {
		t.Fatal("same-ITID restart point was rejected")
	}
}

func TestTraceDBLifecycleTerminalClosesGenerationWithoutRestart(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addTerminal(1, 50, "X")
	lifecycle := builder.finalize()
	if !reflect.DeepEqual(lifecycle.ByTID[42].Terminals, []traceDBLifecycleBoundary{{TS: 50, NewITID: 1, NewIPID: 1}}) {
		t.Fatalf("terminal missing from immutable lane: %+v", lifecycle.ByTID[42])
	}
	if !traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 50) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 51) {
		t.Fatal("terminal exact point/dead-gap semantics are incorrect")
	}
	if !traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 40, 50) ||
		traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 40, 51) {
		t.Fatal("thread-owned interval crossed an exact terminal")
	}
	if !traceDBLifecycleThreadClosedEndpointAllows(lifecycle, identities, 1, 40, 50) {
		t.Fatal("old closing endpoint at its exact terminal was rejected")
	}
}

func TestTraceDBLifecycleClosedEndpointMustMatchTerminalSubject(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addTerminal(2, 50, "X")
	lifecycle := builder.finalize()
	if traceDBLifecycleThreadClosedEndpointAllows(lifecycle, identities, 1, 40, 50) ||
		!traceDBLifecycleThreadClosedEndpointAllows(lifecycle, identities, 2, 40, 50) {
		t.Fatal("closed endpoint did not require the terminal's exact subject")
	}
}

func TestTraceDBLifecycleRepeatedTerminalIsNotItselfARebirth(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addTerminal(1, 50, "X")
	builder.addTerminal(1, 70, "X")
	cursor := builder.newActivityCursor()
	cursor.observe(1, 70)
	cursor.observe(1, 80)
	lifecycle := builder.finalize()
	want := []traceDBLifecycleBoundary{{TS: 80, NewITID: 1, NewIPID: 1}}
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) {
		t.Fatalf("repeated terminal was treated as activity: cuts=%+v want=%+v", lifecycle.ByTID[42].Cuts, want)
	}
}

func TestTraceDBLifecycleEqualTerminalActivitiesNeverBecomeRestarts(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()

	t.Run("different subject at exact terminal", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addTerminal(1, 60, "X")
		cursor := builder.newActivityCursor()
		cursor.observe(2, 60)
		lifecycle := builder.finalize()
		lane := lifecycle.ByTID[42]
		if len(lane.Cuts) != 0 || len(lane.PoisonPoints) != 0 || len(lane.UnknownStarts) != 0 {
			t.Fatalf("different-subject activity at terminal became a transition: %+v", lane)
		}
	})

	t.Run("conflicted terminal cannot mutate prior proposal", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		prior := builder.lane(42).terminalsByTS[50]
		builder.addTerminal(1, 60, "X")
		builder.addTerminal(1, 60, "Z")
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		cursor.observe(2, 60)
		if prior.RestartKnown {
			t.Fatalf("equal conflicted terminal updated prior restart: %+v", prior)
		}
		lane := builder.finalize().ByTID[42]
		if !reflect.DeepEqual(lane.PoisonPoints, []int64{60}) || !reflect.DeepEqual(lane.UnknownStarts, []int64{60}) {
			t.Fatalf("conflicted terminal lost its generation invalidation: %+v", lane)
		}
	})

	t.Run("strictly later different subject remains valid", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 60, "X")
		cursor := builder.newActivityCursor()
		cursor.observe(2, 61)
		lane := builder.finalize().ByTID[42]
		want := []traceDBLifecycleBoundary{{TS: 61, NewITID: 2, NewIPID: 2}}
		if !reflect.DeepEqual(lane.Cuts, want) || len(lane.PoisonPoints) != 0 || len(lane.UnknownStarts) != 0 {
			t.Fatalf("strictly later different-subject restart was over-suppressed: %+v", lane)
		}
	})
}

func TestTraceDBLifecycleActivityOrderDoesNotChangeEarliestRestart(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	build := func(order []int64) traceDBLifecycleIndex {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		cursor := builder.newActivityCursor()
		for _, timestamp := range order {
			cursor.observe(1, timestamp)
		}
		return builder.finalize()
	}
	forward := build([]int64{60, 70, 80})
	reverse := build([]int64{80, 70, 60})
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("activity physical order changed lifecycle:\nforward=%+v\nreverse=%+v", forward, reverse)
	}
}

func TestTraceDBLifecycleEarlierActivitySupersedesLaterAmbiguousCandidate(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	build := func(activities []traceDBLifecycleBoundary) traceDBLifecycleIndex {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		cursor := builder.newActivityCursor()
		for _, activity := range activities {
			cursor.observe(activity.NewITID, activity.TS)
		}
		return builder.finalize()
	}
	lateConflictFirst := build([]traceDBLifecycleBoundary{{TS: 60, NewITID: 1}, {TS: 60, NewITID: 2}, {TS: 55, NewITID: 1}})
	earlyCandidateFirst := build([]traceDBLifecycleBoundary{{TS: 55, NewITID: 1}, {TS: 60, NewITID: 1}, {TS: 60, NewITID: 2}})
	want := []traceDBLifecycleBoundary{{TS: 55, NewITID: 1, NewIPID: 1}}
	if !reflect.DeepEqual(lateConflictFirst, earlyCandidateFirst) ||
		!reflect.DeepEqual(lateConflictFirst.ByTID[42].Cuts, want) ||
		len(lateConflictFirst.ByTID[42].PoisonPoints) != 0 {
		t.Fatalf("activity permutation changed the immutable lifecycle:\nlate-first=%+v\nearly-first=%+v", lateConflictFirst, earlyCandidateFirst)
	}
}

func TestTraceDBLifecycleLaterTerminalCreatesFreshRestartProof(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()

	t.Run("blocked dead anchor", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 10, "X")
		builder.addPoisonForITID(1, 15)
		builder.addTerminal(1, 30, "X")
		cursor := builder.newActivityCursor()
		cursor.observe(1, 40)
		lifecycle := builder.finalize()
		want := []traceDBLifecycleBoundary{{TS: 40, NewITID: 1, NewIPID: 1}}
		if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) ||
			traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 35) ||
			!traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 40) {
			t.Fatalf("later terminal did not replace the blocked dead anchor: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("unknown requires direct recovery", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addCreation(1, 10)
		builder.addCreation(2, 10)
		builder.addTerminal(1, 20, "X")
		cursor := builder.newActivityCursor()
		cursor.observe(2, 30)
		builder.addCreation(2, 40)
		lifecycle := builder.finalize()
		want := []traceDBLifecycleBoundary{{TS: 40, NewITID: 2, NewIPID: 2}}
		if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) ||
			traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 25) ||
			traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 30) ||
			!traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 40) {
			t.Fatalf("Unknown recovered without an independent direct begin: %+v", lifecycle.ByTID[42])
		}
	})
}

func TestTraceDBLifecycleConflictsAndPoisonCannotBeSkipped(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()

	t.Run("conflicting creation", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addCreation(1, 100)
		builder.addCreation(2, 100)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{100}) {
			t.Fatalf("conflicting creation survived: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("conflicting creation remains unknown until trusted cut", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addCreation(1, 100)
		builder.addCreation(2, 100)
		builder.addCreation(2, 120)
		lifecycle := builder.finalize()
		if traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 101) ||
			traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 101) ||
			!traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 120) {
			t.Fatal("conflicted cut did not create an unknown segment or later cut failed to restore it")
		}
	})

	t.Run("conflicting terminal", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addTerminal(1, 50, "Z")
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{50}) {
			t.Fatalf("conflicting terminal inferred a cut: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("conflicting earliest activity", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		one := builder.newActivityCursor()
		two := builder.newActivityCursor()
		one.observe(1, 60)
		two.observe(2, 60)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{60}) {
			t.Fatalf("ambiguous restart inferred a cut: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("poison between terminal and later activity", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addPoisonForITID(1, 55)
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{55}) {
			t.Fatalf("later activity jumped a poisoned point: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("global poison between terminal and later activity", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addGlobalPoison(55)
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 {
			t.Fatalf("later activity jumped a global poisoned point: %+v", lifecycle)
		}
	})

	t.Run("direct creation supersedes a later inferred restart", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addCreation(2, 55)
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		want := []traceDBLifecycleBoundary{{TS: 55, NewITID: 2, NewIPID: 2}}
		if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) {
			t.Fatalf("later inferred cut survived earlier direct creation: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("direct creation at candidate must converge", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addCreation(2, 60)
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{60}) {
			t.Fatalf("same-time direct/inferred subject conflict was not poisoned: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("direct creation at candidate coalesces", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addCreation(1, 60)
		cursor := builder.newActivityCursor()
		cursor.observe(1, 60)
		lifecycle := builder.finalize()
		want := []traceDBLifecycleBoundary{{TS: 60, NewITID: 1, NewIPID: 1}}
		if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, want) || len(lifecycle.ByTID[42].PoisonPoints) != 0 {
			t.Fatalf("same-time convergent direct/inferred cut did not coalesce: %+v", lifecycle.ByTID[42])
		}
	})

	t.Run("global poison at direct creation", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addCreation(2, 100)
		builder.addGlobalPoison(100)
		lifecycle := builder.finalize()
		if len(lifecycle.ByTID[42].Cuts) != 0 || traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 101) {
			t.Fatalf("global poison failed to invalidate same-time direct cut: %+v", lifecycle)
		}
	})

	t.Run("proposal conflict after latest terminal stays unknown", func(t *testing.T) {
		builder := newTraceDBLifecycleBuilder(identities)
		builder.addTerminal(1, 50, "X")
		builder.addTerminal(1, 60, "X")
		cursor := builder.newActivityCursor()
		cursor.observe(1, 61)
		cursor.observe(2, 61)
		cursor.observe(1, 70)
		lifecycle := builder.finalize()
		lane := lifecycle.ByTID[42]
		if len(lane.Cuts) != 0 || !reflect.DeepEqual(lane.PoisonPoints, []int64{61}) ||
			!reflect.DeepEqual(lane.UnknownStarts, []int64{61}) || traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 70) {
			t.Fatalf("conflicted post-terminal candidate minted a later cut: %+v", lane)
		}
	})
}

func TestTraceDBLifecycleRejectsStaleTerminalSubjectAndPseudoITID(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addCreation(2, 40)
	builder.addTerminal(1, 50, "X")
	cursor := builder.newActivityCursor()
	cursor.observe(1, 60)
	lifecycle := builder.finalize()
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, []traceDBLifecycleBoundary{{TS: 40, NewITID: 2, NewIPID: 2}}) ||
		!reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{50}) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 60) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 60) {
		t.Fatalf("stale terminal subject reopened a generation: %+v", lifecycle.ByTID[42])
	}
	if traceDBLifecycleThreadPointAllows(lifecycle, identities, 0, 10) ||
		traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 0, 10, 20) {
		t.Fatal("generic hard lifecycle primitive accepted pseudo ITID 0")
	}
}

func TestTraceDBLifecyclePoisonTaintAndRangePredicates(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addPoisonForITID(1, 20)
	builder.addGlobalPoison(30)
	lifecycle := builder.finalize()
	if traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 20) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 30) {
		t.Fatal("poisoned point was accepted")
	}
	if traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 10, 21) ||
		traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 2, 29, 31) {
		t.Fatal("source interval crossed a poison barrier")
	}
	if !traceDBLifecycleThreadSourceIntervalAllows(lifecycle, identities, 1, 21, 29) {
		t.Fatal("interval outside poison barriers was rejected")
	}

	tainted := newTraceDBLifecycleBuilder(identities)
	tainted.taintITID(1)
	lifecycle = tainted.finalize()
	if traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 100) ||
		traceDBLifecycleThreadPointAllows(lifecycle, identities, 2, 100) {
		t.Fatal("same public-TID lane escaped taint")
	}

	global := newTraceDBLifecycleBuilder(identities)
	global.taintGlobal()
	lifecycle = global.finalize()
	if traceDBLifecycleThreadPointAllows(lifecycle, identities, 1, 100) {
		t.Fatal("global lifetime taint was ignored")
	}
}

func TestTraceDBLifecycleOnlyUniqueMainThreadProjectsProcessGeneration(t *testing.T) {
	identities := newTraceDBThreadIndex(0, true)
	identities.Processes[10] = traceDBProcess{IPID: 10, PID: 100, Name: "new-process"}
	identities.Processes[20] = traceDBProcess{IPID: 20, PID: 100, Name: "old-process"}
	identities.ByITID[10] = traceDBThread{ITID: 10, TID: 100, IPID: 10, Name: "new-main", IsMainThread: true}
	identities.ByITID[11] = traceDBThread{ITID: 11, TID: 101, IPID: 10, Name: "worker"}
	identities.ByITID[20] = traceDBThread{ITID: 20, TID: 100, IPID: 20, Name: "old-main", IsMainThread: true}
	buildTraceDBThreadSecondaryIndexes(&identities)

	builder := newTraceDBLifecycleBuilder(identities)
	builder.addCreation(10, 50)
	builder.addCreation(11, 60)
	lifecycle := builder.finalize()
	if !reflect.DeepEqual(lifecycle.ByPID[100].Cuts, []traceDBLifecycleBoundary{{TS: 50, NewITID: 10, NewIPID: 10}}) {
		t.Fatalf("process cuts=%+v", lifecycle.ByPID[100].Cuts)
	}
	if traceDBLifecycleProcessPointAllows(lifecycle, identities, 20, 50) ||
		!traceDBLifecycleProcessPointAllows(lifecycle, identities, 10, 50) {
		t.Fatal("process generation boundary did not select the new IPID")
	}
	if !traceDBLifecycleProcessSourceIntervalAllows(lifecycle, identities, 20, 40, 50) ||
		traceDBLifecycleProcessSourceIntervalAllows(lifecycle, identities, 20, 40, 51) {
		t.Fatal("process half-open interval boundary is incorrect")
	}
	if traceDBLifecycleProcessClosedEndpointAllows(lifecycle, identities, 20, 40, 50) {
		t.Fatal("process closing endpoint crossed a generation cut")
	}
	if cuts := lifecycle.ByPID[100].Cuts; len(cuts) != 1 {
		t.Fatalf("worker creation polluted process generation: %+v", cuts)
	}
}

func TestTraceDBLifecycleMainTIDWorkerReuseTombstonesOldProcess(t *testing.T) {
	identities := newTraceDBThreadIndex(0, true)
	identities.Processes[10] = traceDBProcess{IPID: 10, PID: 100, Name: "old-main-process"}
	identities.Processes[20] = traceDBProcess{IPID: 20, PID: 20, Name: "worker-owner"}
	identities.Processes[30] = traceDBProcess{IPID: 30, PID: 100, Name: "new-main-process"}
	identities.ByITID[10] = traceDBThread{ITID: 10, TID: 100, IPID: 10, IsMainThread: true}
	identities.ByITID[20] = traceDBThread{ITID: 20, TID: 100, IPID: 20}
	identities.ByITID[30] = traceDBThread{ITID: 30, TID: 100, IPID: 30, IsMainThread: true}
	buildTraceDBThreadSecondaryIndexes(&identities)
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addCreation(10, 10)
	builder.addCreation(20, 20)
	builder.addCreation(30, 30)
	lifecycle := builder.finalize()
	want := []traceDBLifecycleBoundary{
		{TS: 10, NewITID: 10, NewIPID: 10},
		{TS: 20, NewITID: 20, NewIPID: 20},
		{TS: 30, NewITID: 30, NewIPID: 30},
	}
	if !reflect.DeepEqual(lifecycle.ByPID[100].Cuts, want) {
		t.Fatalf("main PID lane lost a worker-reuse tombstone: %+v", lifecycle.ByPID[100])
	}
	if !traceDBLifecycleProcessPointAllows(lifecycle, identities, 10, 15) ||
		traceDBLifecycleProcessPointAllows(lifecycle, identities, 10, 25) ||
		!traceDBLifecycleProcessPointAllows(lifecycle, identities, 30, 35) {
		t.Fatal("main→worker→main process lifetime selection is incorrect")
	}
}

func TestTraceDBLifecycleProcessClosedEndpointMatchesTerminal(t *testing.T) {
	identities := newTraceDBThreadIndex(0, true)
	identities.Processes[10] = traceDBProcess{IPID: 10, PID: 100}
	identities.ByITID[10] = traceDBThread{ITID: 10, TID: 100, IPID: 10, IsMainThread: true}
	buildTraceDBThreadSecondaryIndexes(&identities)
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addTerminal(10, 50, "X")
	lifecycle := builder.finalize()
	if !traceDBLifecycleProcessSourceIntervalAllows(lifecycle, identities, 10, 40, 50) ||
		!traceDBLifecycleProcessClosedEndpointAllows(lifecycle, identities, 10, 40, 50) ||
		traceDBLifecycleProcessSourceIntervalAllows(lifecycle, identities, 10, 40, 51) {
		t.Fatal("process terminal half-open/closed endpoint semantics diverged")
	}
}

func TestTraceDBLifecycleActivityCursorIsBoundedByTerminals(t *testing.T) {
	identities := traceDBLifecycleFixtureIndex()
	builder := newTraceDBLifecycleBuilder(identities)
	builder.addTerminal(1, 5, "X")
	cursor := builder.newActivityCursor()
	for timestamp := int64(6); timestamp < 100006; timestamp++ {
		if !cursor.observe(1, timestamp) {
			t.Fatalf("activity %d rejected", timestamp)
		}
	}
	if len(builder.lane(42).terminals) != 1 {
		t.Fatalf("activity cursor retained per-row state: terminals=%d", len(builder.lane(42).terminals))
	}
	lifecycle := builder.finalize()
	if !reflect.DeepEqual(lifecycle.ByTID[42].Cuts, []traceDBLifecycleBoundary{{TS: 6, NewITID: 1, NewIPID: 1}}) {
		t.Fatalf("large activity stream chose wrong restart: %+v", lifecycle.ByTID[42])
	}
}
