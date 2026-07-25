package tracequery

import "testing"

func TestPerPIDIdentityNarrowingRetainsCleanOffCPUAndChurnRows(t *testing.T) {
	idx := buildTraceIndex(t, "per_pid_identity_offcpu.systrace", `
          old-900 (  900) [001] .... 0.995000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
          app-200 (  200) [000] .... 1.000000: sched_wakeup: comm=worker pid=100 prio=20 target_cpu=000
          old-900 (  900) [001] .... 1.005000: sched_switch: prev_comm=old prev_pid=900 prev_prio=20 prev_state=X ==> next_comm=swapper/1 next_pid=0 next_prio=120
        swapper-0 (    0) [000] .... 1.010000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
       worker-100 (  100) [000] .... 1.020000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
        creator-7 (    7) [001] .... 1.025000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=001
          app-200 (  200) [000] .... 1.030000: sched_wakeup: comm=worker pid=100 prio=20 target_cpu=000
        swapper-1 (    0) [001] .... 1.035000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=900 next_prio=20
        swapper-0 (    0) [000] .... 1.040000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
          new-900 (  900) [001] .... 1.045000: sched_switch: prev_comm=new prev_pid=900 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
       worker-100 (  100) [000] .... 1.050000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.06, MinDurationMs: 0.1, Limit: 20})

	if !threadDurationRowsContainPID(stats.TopRunning, 100) ||
		!threadDurationRowsContainPID(stats.RunnableTop, 100) ||
		!threadDurationRowsContainPID(stats.SleepTop, 100) {
		t.Fatalf("clean target scheduler value channels were erased: running=%+v runnable=%+v sleep=%+v", stats.TopRunning, stats.RunnableTop, stats.SleepTop)
	}
	for _, rows := range [][]ThreadDuration{stats.TopRunning, stats.RunnableTop, stats.SleepTop, stats.DStateTop, stats.IOWaitTop} {
		if threadDurationRowsContainPID(rows, 900) {
			t.Fatalf("conflicting PID leaked into scheduler value channels: %+v", rows)
		}
	}
	if !threadCPULoadRowsContainPID(stats.ThreadCPULoad, 100) {
		t.Fatalf("clean thread CPU-load input did not survive: %+v", stats.ThreadCPULoad)
	}
	for _, row := range stats.ThreadCPULoad {
		if row.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into thread CPU-load: %+v", stats.ThreadCPULoad)
		}
	}
	if !stateChurnRowsContainPID(stats.StateChurn, 100) {
		t.Fatalf("clean churn row did not survive: %+v", stats.StateChurn)
	}
	for _, row := range stats.StateChurn {
		if row.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into churn: %+v", stats.StateChurn)
		}
	}
	if len(stats.ProcessCPULoad) != 0 || stats.CPUOccupancy != nil || stats.ProcessDomainCensus != nil {
		t.Fatalf("process composites reopened before contributor-completeness proof: process=%+v occupancy=%+v census=%+v", stats.ProcessCPULoad, stats.CPUOccupancy, stats.ProcessDomainCensus)
	}
}

func TestWakeupChainSurvivesUnrelatedPIDConflict(t *testing.T) {
	idx := buildTraceIndex(t, "per_pid_identity_clean_chain.systrace", `
        swapper-0 (    0) [000] .... 0.990000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=100 next_prio=20
        swapper-1 (    0) [001] .... 0.995000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=200 next_prio=30
          old-900 (  900) [002] .... 0.998000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
       target-100 (  100) [000] .... 1.000000: sched_switch: prev_comm=target prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
          old-900 (  900) [002] .... 1.005000: sched_switch: prev_comm=old prev_pid=900 prev_prio=20 prev_state=X ==> next_comm=swapper/2 next_pid=0 next_prio=120
        waker-200 (  200) [001] .... 1.020000: sched_switch: prev_comm=waker prev_pid=200 prev_prio=30 prev_state=R ==> next_comm=swapper/1 next_pid=0 next_prio=120
        creator-7 (    7) [002] .... 1.025000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=002
        waker-200 (  200) [001] .... 1.030000: sched_wakeup: comm=target pid=100 prio=20 target_cpu=000
        swapper-0 (    0) [000] .... 1.040000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=100 next_prio=20
       target-100 (  100) [000] .... 1.050000: sched_switch: prev_comm=target prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 0.99, TimeEnd: 1.06, MinDurationMs: 0.1, MaxDepth: 4, MaxBranches: 8, Limit: 20}
	chain := BuildWakeupChain(idx, q)
	if !wakeupEdgesContainPair(chain.Edges, 200, 100) {
		t.Fatalf("unrelated PID reuse erased a strict clean wakeup edge: %+v", chain)
	}
	if containsSubstring(chain.Caveats, "wakeup_chain_fail_closed=true") ||
		!containsSubstring(chain.Caveats, "wakeup_chain_per_pid_identity=true") {
		t.Fatalf("clean target chain authority was not narrowed correctly: %+v", chain.Caveats)
	}
	for _, node := range chain.Nodes {
		if node.Thread.PID == 900 {
			t.Fatalf("unrelated conflicting PID leaked into chain nodes: %+v", chain.Nodes)
		}
	}

	rank := BuildRootCauseRank(idx, q)
	cleanRankRow := false
	for _, item := range rank.Items {
		if item.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into root-cause rank: %+v", rank.Items)
		}
		if item.Thread.PID == 100 || item.Thread.PID == 200 {
			cleanRankRow = true
		}
	}
	if !cleanRankRow {
		t.Fatalf("root-cause rank did not consume the recovered clean scheduler/chain inputs: %+v", rank)
	}
	interactions := BuildInteractionStats(idx, q)
	if !interactionRowsContainPID(interactions.Items, 200) ||
		interactionRowsContainPID(interactions.Items, 900) {
		t.Fatalf("interaction peer closure did not retain only identity-safe peers: %+v", interactions)
	}
}

func TestWakeupChainDropsConflictingDependencyBeforeNodeOrEdge(t *testing.T) {
	idx := buildTraceIndex(t, "per_pid_identity_unsafe_dependency.systrace", `
        swapper-0 (    0) [000] .... 0.990000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=100 next_prio=20
          old-900 (  900) [001] .... 0.995000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=30
       target-100 (  100) [000] .... 1.000000: sched_switch: prev_comm=target prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
          old-900 (  900) [001] .... 1.005000: sched_switch: prev_comm=old prev_pid=900 prev_prio=30 prev_state=X ==> next_comm=swapper/1 next_pid=0 next_prio=120
        creator-7 (    7) [001] .... 1.010000: sched_wakeup_new: comm=new pid=900 prio=30 target_cpu=001
          new-900 (  900) [001] .... 1.020000: sched_wakeup: comm=target pid=100 prio=20 target_cpu=000
        swapper-0 (    0) [000] .... 1.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=100 next_prio=20
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 0.99, TimeEnd: 1.04, MinDurationMs: 0.1, MaxDepth: 4, MaxBranches: 8, Limit: 20})
	if len(chain.Edges) != 0 {
		t.Fatalf("unsafe dependency minted a causal edge: %+v", chain.Edges)
	}
	for _, node := range chain.Nodes {
		if node.Thread.PID == 900 {
			t.Fatalf("unsafe dependency minted a chain node: %+v", chain.Nodes)
		}
	}
	if !containsSubstring(chain.Caveats, "thread_identity_dependency_fail_closed=true") ||
		!containsSubstring(chain.Caveats, "no missing_wakeup or trace_gap evidence was inferred") {
		t.Fatalf("dependency branch authority withdrawal was not disclosed: %+v", chain.Caveats)
	}
	for _, evidence := range chain.RootEvidence {
		if evidence.Thread.PID == 900 ||
			evidence.Type == "missing_wakeup" ||
			evidence.Type == "trace_gap" {
			t.Fatalf("unsafe dependency rejection was converted into fabricated evidence: %+v", chain.RootEvidence)
		}
	}
	interactions := BuildInteractionStats(idx, Query{PID: 100, TimeStart: 0.99, TimeEnd: 1.04, Limit: 20})
	if interactionRowsContainPID(interactions.Items, 900) ||
		!containsSubstring(interactions.Caveats, "interaction_identity_per_pid_filtered=true") {
		t.Fatalf("unsafe wakeup peer leaked through interaction stats: %+v", interactions)
	}
}

func TestIPCGraphFiltersOnlyConflictingEndpointPID(t *testing.T) {
	clean := buildTraceIndex(t, "per_pid_identity_clean_ipc.systrace", `
          old-900 (  900) [002] .... 1.000000: sched_switch: prev_comm=old prev_pid=900 prev_prio=20 prev_state=X ==> next_comm=swapper/2 next_pid=0 next_prio=120
       target-100 (  100) [000] .... 1.010000: binder_transaction: transaction=7 dest_node=1 dest_proc=200 dest_thread=200 reply=0 flags=0x0 code=0x1
       server-200 (  200) [001] .... 1.011000: binder_transaction_received: transaction_id=7
        creator-7 (    7) [002] .... 1.020000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=002
	`)
	q := Query{PID: 100, TimeStart: 0.99, TimeEnd: 1.03, Limit: 20}
	graph := BuildIPCGraph(clean, q)
	if len(graph.Edges) != 1 || graph.Edges[0].Sender.PID != 100 || graph.Edges[0].Receiver.PID != 200 {
		t.Fatalf("unrelated PID conflict erased a clean Binder edge: %+v", graph)
	}

	unsafe := buildTraceIndex(t, "per_pid_identity_unsafe_ipc.systrace", `
          old-900 (  900) [002] .... 1.000000: sched_switch: prev_comm=old prev_pid=900 prev_prio=20 prev_state=X ==> next_comm=swapper/2 next_pid=0 next_prio=120
        creator-7 (    7) [002] .... 1.005000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=002
       target-100 (  100) [000] .... 1.010000: binder_transaction: transaction=8 dest_node=1 dest_proc=900 dest_thread=900 reply=0 flags=0x0 code=0x1
          new-900 (  900) [002] .... 1.011000: binder_transaction_received: transaction_id=8
	`)
	graph = BuildIPCGraph(unsafe, q)
	if len(graph.Edges) != 0 || !containsSubstring(graph.Caveats, "ipc_identity_per_pid_filtered=true") ||
		!containsSubstring(graph.Caveats, "suppressed_pids=[900]") {
		t.Fatalf("conflicting Binder endpoint was not filtered per PID: %+v", graph)
	}
}

func threadDurationRowsContainPID(rows []ThreadDuration, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}

func threadCPULoadRowsContainPID(rows []ThreadCPULoadSummary, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}

func stateChurnRowsContainPID(rows []ThreadStateChurnSummary, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}

func wakeupEdgesContainPair(edges []WakeupEdge, wakerPID, wakeePID int) bool {
	for _, edge := range edges {
		if edge.Waker.PID == wakerPID && edge.Wakee.PID == wakeePID {
			return true
		}
	}
	return false
}

func interactionRowsContainPID(rows []InteractionSummary, pid int) bool {
	for _, row := range rows {
		if row.Peer.PID == pid {
			return true
		}
	}
	return false
}
