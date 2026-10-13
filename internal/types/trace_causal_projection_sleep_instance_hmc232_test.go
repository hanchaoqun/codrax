package types

import (
	"encoding/json"
	"fmt"
	"testing"
)

func hmc232SleepInstance(t *testing.T, source string, branch int, start, end float64, first, last int) (TraceCausalProjectionNode, traceCausalProjectionWakeupEdge) {
	t.Helper()
	ref := ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: source, PayloadRef: source + ".json", QueryScopeID: source + ":query", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 10,
		TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", ClockAlignment: "identity", QueryTargetPID: 200, QueryTargetScope: "thread"}
	record := ObservationRecord{ID: fmt.Sprintf("sleep-%s-%d", source, branch), Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "wakeup_causal_impact", Subject: "worker-200", Object: "s_sleep", Unit: "ms", SourceRef: ref, ObservedAt: "generation-1",
		Span:               ObservationSpan{StartTs: start, EndTs: end, LineStart: first, LineEnd: last},
		RichNotes:          []string{"causality=on_wakeup_chain", "dominant_state=s_sleep", fmt.Sprintf("chain_branch=%d", branch), "state_account_key=" + TraceSchedulerStateAccountIdentity(200, "s_sleep", start, end, [][2]float64{{start, end}}, (end-start)*1000), "state_account_complete=true", fmt.Sprintf("sleep=%.3f", (end-start)*1000)},
		MeasurementSources: &TraceSchedulerMeasurementSources{Domains: []TraceSchedulerMeasurementDomain{{Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 200, WindowStartTs: start, WindowEndTs: end, PartitionID: "partition"}}}}
	f := RuntimeWakeupEvent{IndexPath: source, From: "child", To: "parent", Waker: RuntimeWakeupThread{PID: 80, Comm: "irq", TGID: 2}, Wakee: RuntimeWakeupThread{PID: 200, Comm: "worker", TGID: 100}, Timestamp: end, Line: last, Branch: branch}
	b, _ := json.Marshal(f)
	edgeRecord := ObservationRecord{ID: fmt.Sprintf("wake-%s-%d", source, branch), Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Role: AnswerAggregateRoleSupportingCoverage, ProvenanceLane: ObservationProvenanceObservedDirectCause,
		Predicate: "wakeup_chain_edge", ClaimKey: "wakeup_chain_edge:irq-80->worker-200", Subject: "irq-80", Object: "worker-200", Unit: "ms", SourceRef: ref, ObservedAt: record.ObservedAt,
		Span: ObservationSpan{StartTs: end, EndTs: end, LineStart: last, LineEnd: last}, SupportRefs: []string{fmt.Sprintf("%s:%d", source, last)}, RichNotes: []string{TraceNoteKeyWakeupEventInstance + "=" + string(b)}}
	edge, ok := traceCausalProjectionWakeupEdgeFromRecord(edgeRecord)
	if !ok || edge.Instance == nil {
		t.Fatal("test needs a fully decoded native event")
	}
	return traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record), edge
}

func hmc232Attach(node *TraceCausalProjectionNode, edges ...traceCausalProjectionWakeupEdge) {
	traceCausalProjectionAttachSleepDrilldownTarget(node, traceCausalProjectionUniqueDrilldownTargets(edges, nil), edges)
}

func TestHMC232SleepInstanceRepeatedWaits(t *testing.T) {
	for _, differentWaker := range []bool{false, true} {
		t.Run(fmt.Sprintf("different_waker_%t", differentWaker), func(t *testing.T) {
			a, ea := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
			b, eb := hmc232SleepInstance(t, "/trace", 2, 4, 5, 30, 40)
			if differentWaker {
				eb.Waker = "other-81"
				eb.Instance.Waker = RuntimeWakeupThread{PID: 81, Comm: "other"}
			}
			for i, pair := range []struct {
				node *TraceCausalProjectionNode
				edge traceCausalProjectionWakeupEdge
			}{{&a, ea}, {&b, eb}} {
				hmc232Attach(pair.node, ea, eb)
				if !pair.node.DrilldownWakeupPointKnown || pair.node.DrilldownWakeupTs != pair.edge.WakeupTs || pair.node.DrilldownWakeupLine != pair.edge.WakeupLine || pair.node.DrilldownEvidenceID != pair.edge.EvidenceID || pair.node.DrilldownTarget != pair.edge.Waker {
					t.Fatalf("wait %d lost its own closure: %+v", i, pair.node)
				}
			}
		})
	}
}

func TestHMC232SleepInstanceExactReceiptAndPoint(t *testing.T) {
	mutations := map[string]func(*TraceCausalProjectionNode, *traceCausalProjectionWakeupEdge){
		"different_source": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) {
			e.SourceRef.Path = "/other"
			e.Instance.IndexPath = "/other"
		},
		"different_query": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) {
			e.SourceRef.QueryScopeID = "other-query"
		},
		"different_payload": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) {
			e.SourceRef.PayloadRef = "other.json"
		},
		"different_generation": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) { e.ObservedAt = "generation-2" },
		"different_window": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) {
			e.SourceRef.QueryWindowEndTs = 11
		},
		"different_line_selection": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) { e.SourceRef.QueryLineStart = 2 },
		"different_clock": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) {
			e.SourceRef.CanonicalTimeDomain = "other-clock"
		},
		"different_branch": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) { e.Instance.Branch = 2 },
		"missing_branch":   func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.ChainBranch = 0 },
		"different_thread": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) {
			n.MeasurementOrigins[0].MeasurementSources.Domains[0].TargetTID = 201
		},
		"earlier_wait": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.StartTs = 4; n.EndTs = 5 },
		"same_time_other_physical_range": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) {
			n.LineStart = 21
			n.LineEnd = 30
		},
		"missing_physical_end": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.LineEnd = 0 },
		"missing_native_event": func(_ *TraceCausalProjectionNode, e *traceCausalProjectionWakeupEdge) { e.Instance = nil },
		"missing_inventory":    func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.StateAccountComplete = false },
		"missing_partition": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) {
			n.MeasurementOrigins[0].MeasurementSources = nil
		},
		"unknown_contributor": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) {
			n.MeasurementOrigins = append(n.MeasurementOrigins, TraceSchedulerMeasurementOrigin{})
		},
		"mixed_state_envelope": func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.RunningMS = 1 },
		"multiple_wait_fold":   func(n *TraceCausalProjectionNode, _ *traceCausalProjectionWakeupEdge) { n.MergedCount = 2 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			n, e := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
			mutate(&n, &e)
			hmc232Attach(&n, e)
			if n.DrilldownTarget != "irq-80" {
				t.Fatal("missing instance must still permit thread navigation")
			}
			if n.DrilldownWakeupPointKnown || n.DrilldownWakeupTs != 0 || n.DrilldownWakeupLine != 0 {
				t.Fatalf("unproved instance acquired closure: %s", name)
			}
		})
	}
}

func TestHMC232SleepInstanceSameCoordinatesDifferentSources(t *testing.T) {
	a, ea := hmc232SleepInstance(t, "/source-a", 1, 2, 3, 10, 20)
	b, eb := hmc232SleepInstance(t, "/source-b", 1, 2, 3, 10, 20)
	for _, pair := range []struct {
		node *TraceCausalProjectionNode
		id   string
	}{{&a, ea.EvidenceID}, {&b, eb.EvidenceID}} {
		hmc232Attach(pair.node, ea, eb)
		if !pair.node.DrilldownWakeupPointKnown || pair.node.DrilldownEvidenceID != pair.id {
			t.Fatal("exact point cited another capture's event")
		}
	}
	a.MeasurementOrigins = append(a.MeasurementOrigins, b.MeasurementOrigins...)
	a.DrilldownWakeupPointKnown = false
	a.DrilldownWakeupTs = 0
	a.DrilldownWakeupLine = 0
	hmc232Attach(&a, ea, eb)
	if a.DrilldownWakeupPointKnown {
		t.Fatal("mixed captures fabricated one closure")
	}
}

func TestHMC232SleepInstanceConflictingEventTuple(t *testing.T) {
	for _, field := range []string{"from", "to", "segment", "line", "waker_generation"} {
		t.Run(field, func(t *testing.T) {
			n, e := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
			other := e
			copyEvent := *e.Instance
			other.Instance = &copyEvent
			switch field {
			case "from":
				other.Instance.From = "other-child"
			case "to":
				other.Instance.To = "other-parent"
			case "segment":
				other.Instance.Segment = 2
			case "line":
				other.Instance.Line = 19
				other.WakeupLine = 19
			case "waker_generation":
				other.Instance.Waker.TGID = 3
			}
			hmc232Attach(&n, e, other)
			if n.DrilldownWakeupPointKnown {
				t.Fatalf("conflicting %s minted a closure", field)
			}
		})
	}
}

func TestHMC232SleepInstanceRepeatedPublicationLocalBranch(t *testing.T) {
	for _, sameBranch := range []bool{true, false} {
		t.Run(fmt.Sprintf("same_local_branch_%t", sameBranch), func(t *testing.T) {
			n, e := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
			other := e
			f := *e.Instance
			other.Instance = &f
			other.SourceRef.QueryScopeID = "second-query"
			other.SourceRef.PayloadRef = "second.json"
			other.EvidenceID = "second-event"
			origin := CloneTraceSchedulerMeasurementOrigins(n.MeasurementOrigins)[0]
			origin.SourceRef = other.SourceRef
			n.MeasurementOrigins = append(n.MeasurementOrigins, origin)
			if !sameBranch {
				other.Instance.Branch = 2
			}
			hmc232Attach(&n, e, other)
			if n.DrilldownWakeupPointKnown != sameBranch {
				t.Fatal("local branch identity was borrowed across queries, or duplicate proof lost")
			}
			if n.DrilldownTarget != "irq-80" || n.ChainRelevance != "on_chain" || len(n.MeasurementOrigins) != 2 {
				t.Fatal("withholding point must preserve navigation, chain and all receipts")
			}
		})
	}
}

func TestHMC232SleepInstanceExactInventoryNotRoundedTotals(t *testing.T) {
	n, e := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
	// The non-sleep gap is 100ns; both accounts round to 1000.000ms.
	n.StateAccountKey = TraceSchedulerStateAccountIdentity(200, "s_sleep", 2, 3, [][2]float64{{2, 2.5}, {2.5000001, 3}}, 999.9999)
	hmc232Attach(&n, e)
	if n.DrilldownWakeupPointKnown {
		t.Fatal("multi-wait inventory acquired the last member's closing point")
	}
	n, e = hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
	n.StateOccupancyPresence |= traceStateOccupancyRunnable
	hmc232Attach(&n, e)
	if n.DrilldownWakeupPointKnown {
		t.Fatal("published zero-rounding non-sleep key was treated as absence")
	}
}

func TestHMC232SleepInstanceRepeatedPublicationClockConflict(t *testing.T) {
	n, e := hmc232SleepInstance(t, "/trace", 1, 2, 3, 10, 20)
	other := e
	other.SourceRef.QueryScopeID = "second-query"
	other.SourceRef.CanonicalTimeDomain = "different-clock"
	origin := CloneTraceSchedulerMeasurementOrigins(n.MeasurementOrigins)[0]
	origin.SourceRef = other.SourceRef
	n.MeasurementOrigins = append(n.MeasurementOrigins, origin)
	hmc232Attach(&n, e, other)
	if n.DrilldownWakeupPointKnown || n.DrilldownTarget != "irq-80" {
		t.Fatal("cross-clock duplicate may navigate, not publish one closure")
	}
}

func TestHMC232StateAccountV2HistoricalBytes(t *testing.T) {
	got := TraceSchedulerStateAccountIdentity(200, "s_sleep", .999, 1.051, [][2]float64{{1.00901, 1.04001}}, 31)
	const want = "state_account:v2:833b03a3d10bed200d085bfebc13ccfb"
	if got != want {
		t.Fatalf("native business fixture v2 key changed: %s", got)
	}
	if narrowed := TraceSchedulerStateAccountIdentity(200, "s_sleep", 1.00901, 1.04001, [][2]float64{{1.00901, 1.04001}}, 31); narrowed != want {
		t.Fatal("view window entered physical inventory identity")
	}
}
