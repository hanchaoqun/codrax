package tracequery

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestPairingWindowDiscoveryLateF2FSScopePoisonFallsBackToRetainedSCSI(t *testing.T) {
	path := writeWindowDiscoveryTrace(t,
		"io-40 (40) [003] .... 1.000000: f2fs_sync_file_enter: "+f2fsSyncEnterBody+"\n"+
			"io-40 (40) [003] .... 1.001000: f2fs_sync_file_exit: "+f2fsSyncExitBody+"\n"+
			"io-41 (41) [003] .... 1.002000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\n"+
			"io-41 (41) [003] .... 1.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\n"+
			"io-40 (40) [003] .... 1.004000: f2fs_direct_IO_enter: "+strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1))
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityComplete {
		t.Fatalf("late unknown-key F2FS endpoint did not fail discovery identity closed: %+v", result)
	}
	if len(result.Windows) != 1 || result.Windows[0].CoreLineStart != 3 || result.Windows[0].CoreLineEnd != 4 {
		t.Fatalf("late F2FS poison erased the independently retained SCSI fallback: %+v", result)
	}
	if len(result.Families) != 1 || result.Families[0].InvalidIdentityCount != 1 {
		t.Fatalf("parse-success malformed endpoint was not handled exactly once by the event observer: %+v", result.Families)
	}
	for _, candidate := range result.Candidates {
		if candidate.FirstLine == 1 || candidate.LastLine == 2 {
			t.Fatalf("poisoned F2FS schema candidate remained publishable: %+v", result.Candidates)
		}
	}
}

func TestPairingWindowDiscoveryParserRejectedF2FSEndpointCannotBeDeletedToBridgePair(t *testing.T) {
	path := writeWindowDiscoveryTrace(t,
		"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [bad] .... 1.001000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: "+f2fsDIOExitBody)
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityComplete || result.ParseComplete || len(result.Windows) != 0 || len(result.Candidates) != 0 {
		t.Fatalf("parser-rejected exact F2FS endpoint was deleted and the surrounding pair was rescued: %+v", result)
	}
	if len(result.Families) != 1 || result.Families[0].InvalidIdentityCount != 1 {
		t.Fatalf("parser-rejected F2FS endpoint was not counted once in the typed family: %+v", result.Families)
	}
	if !containsSubstring(result.Caveats, "physical_pairing_audit_fail_closed=true; failures=1 capped=false") {
		t.Fatalf("parser-rejected physical endpoint barrier was not disclosed: %+v", result.Caveats)
	}
}

func TestPairingWindowDiscoveryParserRejectedUnknownF2FSKeyPoisonsOnlyF2FSScope(t *testing.T) {
	path := writeWindowDiscoveryTrace(t,
		"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [bad] .... 1.001000: f2fs_direct_IO_enter: "+strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1)+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: "+f2fsDIOExitBody+"\n"+
			"io-41 (41) [003] .... 1.003000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\n"+
			"io-41 (41) [003] .... 1.004000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096")
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityComplete || result.ParseComplete || len(result.Windows) != 1 || result.Windows[0].CoreLineStart != 4 || result.Windows[0].CoreLineEnd != 5 {
		t.Fatalf("parser-rejected unknown F2FS key crossed scope or rescued the F2FS pair: %+v", result)
	}
	for _, candidate := range result.Candidates {
		if candidate.FirstLine <= 3 {
			t.Fatalf("parser-rejected unknown F2FS key left a scope-poisoned candidate: %+v", result.Candidates)
		}
	}
}

func TestPairingWindowDiscoverySchemaPoolIsBoundedAndStable(t *testing.T) {
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	discovery := newPairingWindowDiscovery(req, "/trace/schema-pool.systrace")
	for line := windowDiscoveryCandidatePoolLimit + 1; line >= 1; line-- {
		discovery.retainSchema(&WindowDiscoveryCandidate{
			Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", FirstLine: line, LastLine: line,
			CollectionComplete: true, FitsSingleWindow: true, IdentityFingerprint: strconv.Itoa(line), laneKey: "lane-" + strconv.Itoa(line),
		})
	}
	if !discovery.poolTruncated || len(discovery.schema) != windowDiscoveryCandidatePoolLimit {
		t.Fatalf("schema candidate reservoir lost its hard bound: truncated=%t retained=%d", discovery.poolTruncated, len(discovery.schema))
	}
	for index, candidate := range discovery.schema {
		if candidate.FirstLine != index+1 {
			t.Fatalf("bounded schema reservoir is not stable/top-ranked at %d: %+v", index, discovery.schema)
		}
	}

	reserved := newPairingWindowDiscovery(req, "/trace/schema-scope-seat.systrace")
	for line := 1; line <= windowDiscoveryCandidatePoolLimit; line++ {
		reserved.retainSchema(&WindowDiscoveryCandidate{
			Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", FirstLine: line, LastLine: line,
			CollectionComplete: true, FitsSingleWindow: true, IdentityFingerprint: strconv.Itoa(line),
			laneKey: "f2fs-lane-" + strconv.Itoa(line), storageScope: "f2fs",
		})
	}
	reserved.retainSchema(&WindowDiscoveryCandidate{
		Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", FirstLine: 10_000, LastLine: 10_000,
		CollectionComplete: true, FitsSingleWindow: true, IdentityFingerprint: "scsi-fallback",
	})
	reserved.invalidateDiscoveryCandidates("", "f2fs")
	if len(reserved.schema) != 1 || reserved.schema[0].IdentityFingerprint != "scsi-fallback" {
		t.Fatalf("bounded schema reservoir did not reserve an independent generic-storage fallback: %+v", reserved.schema)
	}

	fair := newPairingWindowDiscovery(req, "/trace/schema-lane-fairness.systrace")
	for line := 1; line <= windowDiscoveryCandidatePoolLimit*2; line++ {
		fair.retainSchema(&WindowDiscoveryCandidate{
			Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", FirstLine: line, LastLine: line,
			CollectionComplete: true, FitsSingleWindow: true, IdentityFingerprint: "hot", laneKey: "hot", storageScope: "f2fs",
		})
	}
	fair.retainSchema(&WindowDiscoveryCandidate{
		Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", FirstLine: 100_000, LastLine: 100_000,
		CollectionComplete: true, FitsSingleWindow: true, IdentityFingerprint: "clean", laneKey: "clean", storageScope: "f2fs",
	})
	if len(fair.schema) != 2 {
		t.Fatalf("repeat cohorts from one lane consumed schema reservoir seats: %+v", fair.schema)
	}
	fair.invalidateDiscoveryCandidates("hot", "")
	if len(fair.schema) != 1 || fair.schema[0].laneKey != "clean" {
		t.Fatalf("late exact-lane poison erased a clean lane in the same F2FS scope: %+v", fair.schema)
	}
}

func TestPairingWindowDiscoveryBudgetStopMakesPrefixCandidatesDiagnosticOnly(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		"io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]",
		"irq-2 (2) [003] .... 1.001000: block_rq_complete: 8,0 R () 123 + 8 [0]",
		"io-40 (40) [003] .... 1.002000: block_rq_issue: 8,0 R 4096 () 999 + 8 [io]",
		"io-40 (40) [bad] .... 1.003000: f2fs_direct_IO_enter: " + f2fsDIOEnter510Body,
	}, "\n"))
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage}
	req.EndpointLimit = 2
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.IdentityComplete || !result.BudgetStopped || len(result.Windows) != 0 || result.ScannedLineCount != 3 {
		t.Fatalf("budget-stopped prefix candidate remained publishable: %+v", result)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("budget-stopped prefix schema should remain as one diagnostic candidate: %+v", result.Candidates)
	}
	candidate := result.Candidates[0]
	const blocked = "discovery_budget_exceeded_before_full_file_completion"
	if candidate.CollectionComplete || candidate.FitsSingleWindow || candidate.Selected || candidate.RequiredWindowCount != 0 ||
		candidate.CollectionBlockedReason != blocked || candidate.SelectionReason != "not_collectible:"+blocked {
		t.Fatalf("budget-stopped candidate diagnostic contract drifted: %+v", candidate)
	}
	if !containsSubstring(result.Caveats, "no candidate window was published") {
		t.Fatalf("budget-stop publication denial was not disclosed: %+v", result.Caveats)
	}
}

func TestPairingWindowDiscoveryQuarantineBudgetEscalatesWithProvenance(t *testing.T) {
	newDiscovery := func(families []WindowDiscoveryFamily, budget int) *pairingWindowDiscovery {
		req := pairingDiscoveryRequest(.9, 1.1)
		req.Families = families
		req.ActiveLaneLimit = budget
		return newPairingWindowDiscovery(req, "/trace/quarantine-budget.systrace")
	}

	t.Run("f2fs scope preserves SCSI and other scope", func(t *testing.T) {
		discovery := newDiscovery([]WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}, 2)
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "scsi-clean", IdentityFingerprint: "scsi-clean"})
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "f2fs-clean", storageScope: "f2fs", IdentityFingerprint: "f2fs-clean"})
		discovery.lanes["f2fs-active"] = &pairingDiscoveryLane{family: WindowDiscoveryFamilyStorage, key: "f2fs-active", storageScope: "f2fs"}
		discovery.lanes["scsi-active"] = &pairingDiscoveryLane{family: WindowDiscoveryFamilyStorage, key: "scsi-active"}
		discovery.poisonDiscoveryScope(WindowDiscoveryFamilyStorage, "mmc")
		repeated := pairingDiscoveryEndpoint{family: WindowDiscoveryFamilyStorage, keyKnown: true, key: "f2fs-bad-0", scope: "f2fs"}
		for repeat := 0; repeat < 10_000; repeat++ {
			discovery.rejectEndpoint(repeated)
		}
		if len(discovery.poisonedLanes) != 1 || discovery.quarantineEscalations != 0 {
			t.Fatalf("identical F2FS poison consumed repeated budget or escalated: %+v", discovery)
		}
		for index := 1; index < 100; index++ {
			discovery.rejectEndpoint(pairingDiscoveryEndpoint{
				family: WindowDiscoveryFamilyStorage, keyKnown: true, key: "f2fs-bad-" + strconv.Itoa(index), scope: "f2fs",
			})
			if len(discovery.poisonedLanes) > discovery.quarantineBudget() {
				t.Fatalf("exact-lane quarantine exceeded its hard bound at %d: %d", index, len(discovery.poisonedLanes))
			}
		}
		if !discovery.identityIncomplete || discovery.quarantineEscalations != 1 || !discovery.poisonedScopes["f2fs"] || !discovery.poisonedScopes["mmc"] ||
			discovery.poisonedFamilies[WindowDiscoveryFamilyStorage] || len(discovery.poisonedLanes) != 0 {
			t.Fatalf("F2FS overflow did not promote exactly to its scope: %+v", discovery)
		}
		if len(discovery.schema) != 1 || discovery.schema[0].laneKey != "scsi-clean" {
			t.Fatalf("F2FS scope escalation damaged clean SCSI candidate: %+v", discovery.schema)
		}
		if discovery.lanes["f2fs-active"] != nil || discovery.lanes["scsi-active"] == nil {
			t.Fatalf("F2FS scope escalation did not release only subordinate active lanes: %+v", discovery.lanes)
		}
		for repeat := 0; repeat < 10_000; repeat++ {
			discovery.poisonDiscoveryScope(WindowDiscoveryFamilyStorage, "f2fs")
		}
	})

	t.Run("block family preserves storage", func(t *testing.T) {
		discovery := newDiscovery([]WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage}, 2)
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "scsi-clean", IdentityFingerprint: "scsi-clean"})
		discovery.lanes["block-active"] = &pairingDiscoveryLane{family: WindowDiscoveryFamilyBlock, key: "block-active"}
		discovery.lanes["storage-active"] = &pairingDiscoveryLane{family: WindowDiscoveryFamilyStorage, key: "storage-active"}
		discovery.poisonDiscoveryScope(WindowDiscoveryFamilyStorage, "f2fs")
		for index := 0; index < 100; index++ {
			discovery.rejectEndpoint(pairingDiscoveryEndpoint{
				family: WindowDiscoveryFamilyBlock, keyKnown: true, key: "block-bad-" + strconv.Itoa(index),
			})
			if len(discovery.poisonedLanes) > discovery.quarantineBudget() {
				t.Fatalf("Block exact-lane quarantine exceeded its hard bound at %d: %d", index, len(discovery.poisonedLanes))
			}
		}
		if discovery.quarantineEscalations != 1 || !discovery.poisonedFamilies[WindowDiscoveryFamilyBlock] ||
			discovery.poisonedFamilies[WindowDiscoveryFamilyStorage] || !discovery.poisonedScopes["f2fs"] || len(discovery.poisonedLanes) != 0 {
			t.Fatalf("Block overflow crossed family or cleared independent scope poison: %+v", discovery)
		}
		if len(discovery.schema) != 1 || discovery.schema[0].laneKey != "scsi-clean" {
			t.Fatalf("Block family escalation damaged storage candidate: %+v", discovery.schema)
		}
		if discovery.lanes["block-active"] != nil || discovery.lanes["storage-active"] == nil {
			t.Fatalf("Block family escalation did not release only subordinate active lanes: %+v", discovery.lanes)
		}
		for repeat := 0; repeat < 10_000; repeat++ {
			discovery.poisonDiscoveryFamily(WindowDiscoveryFamilyBlock)
		}
	})

	t.Run("rollback recovers scope and unresolved fails family closed", func(t *testing.T) {
		discovery := newDiscovery([]WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}, 1)
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "scsi-clean", IdentityFingerprint: "scsi-clean"})
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "f2fs-a", storageScope: "f2fs", IdentityFingerprint: "f2fs-a"})
		discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "f2fs-b", storageScope: "f2fs", IdentityFingerprint: "f2fs-b"})
		discovery.markDiscoveryLaneRollback("f2fs-a", WindowDiscoveryFamilyStorage)
		rollbackMarked := false
		for _, candidate := range discovery.schema {
			if candidate.laneKey == "f2fs-a" && candidate.CollectionBlockedReason == "same_lane_timestamp_rollback" {
				rollbackMarked = true
			}
		}
		if len(discovery.poisonedLanes) != 1 || !rollbackMarked {
			t.Fatalf("first rollback did not retain one bounded diagnostic poison: lanes=%+v schema=%+v", discovery.poisonedLanes, discovery.schema)
		}
		for repeat := 0; repeat < 10_000; repeat++ {
			discovery.rejectEndpoint(pairingDiscoveryEndpoint{family: WindowDiscoveryFamilyStorage, keyKnown: true, key: "f2fs-a", scope: "f2fs"})
		}
		if record := discovery.poisonedLanes["f2fs-a"]; !record.stateCleared {
			t.Fatalf("rollback poison did not perform exactly one later exact-state cleanup: %+v", record)
		}
		discovery.markDiscoveryLaneRollback("f2fs-b", WindowDiscoveryFamilyStorage)
		if discovery.quarantineEscalations != 1 || !discovery.poisonedScopes["f2fs"] || len(discovery.poisonedLanes) != 0 ||
			len(discovery.schema) != 1 || discovery.schema[0].laneKey != "scsi-clean" {
			t.Fatalf("rollback overflow did not promote to recovered F2FS scope: %+v", discovery)
		}

		unresolved := newDiscovery([]WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage}, 1)
		unresolved.poisonDiscoveryScope(WindowDiscoveryFamilyStorage, "mmc")
		unresolved.markDiscoveryLaneRollback("missing", WindowDiscoveryFamilyBlock)
		if !unresolved.poisonedFamilies[WindowDiscoveryFamilyBlock] || !unresolved.poisonedScopes["mmc"] || len(unresolved.poisonedLanes) != 0 {
			t.Fatalf("unresolved rollback guessed provenance or cleared unrelated poison: %+v", unresolved)
		}
	})
}

func TestPairingWindowDiscoveryQuarantineEscalationPublishesTypedCaveat(t *testing.T) {
	malformed := strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775808", 1)
	lines := []string{
		"io-41 (41) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096",
		"io-41 (41) [003] .... 1.001000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096",
	}
	for index, inode := range []string{"0x9", "0xa", "0xb"} {
		body := strings.Replace(malformed, "ino=0x9", "ino="+inode, 1)
		lines = append(lines, "io-40 (40) [003] .... 1.00"+strconv.Itoa(index+2)+"000: f2fs_direct_IO_enter: "+body)
	}
	path := writeWindowDiscoveryTrace(t, strings.Join(lines, "\n"))
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	req.ActiveLaneLimit = 2
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.IdentityComplete || result.BudgetStopped || len(result.Windows) != 1 || result.Windows[0].CoreLineStart != 1 || result.Windows[0].CoreLineEnd != 2 {
		t.Fatalf("F2FS quarantine escalation crossed into clean SCSI publication: %+v", result)
	}
	if !containsSubstring(result.Caveats, "pairing_quarantine_budget_escalated=true; budget=2 escalations=1 retained_exact_lanes=0 poisoned_scopes=1 poisoned_families=0") {
		t.Fatalf("quarantine escalation typed caveat missing: %+v", result.Caveats)
	}
}

func TestPairingWindowDiscoveryBlockQuarantineEscalationPreservesStorage(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		"io-41 (41) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096",
		"io-41 (41) [003] .... 1.001000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096",
		"io-40 (40) [003] .... 1.002000: block_rq_issue: 8,0 R 0 () 1 + 0 [io]",
		"io-40 (40) [003] .... 1.003000: block_rq_issue: 8,0 R 0 () 2 + 0 [io]",
		"io-40 (40) [003] .... 1.004000: block_rq_issue: 8,0 R 0 () 3 + 0 [io]",
	}, "\n"))
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage}
	req.ActiveLaneLimit = 2
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.IdentityComplete || result.BudgetStopped || len(result.Windows) != 1 ||
		result.Windows[0].Family != WindowDiscoveryFamilyStorage || result.Windows[0].CoreLineStart != 1 || result.Windows[0].CoreLineEnd != 2 {
		t.Fatalf("Block quarantine escalation crossed into clean storage publication: %+v", result)
	}
	if !containsSubstring(result.Caveats, "pairing_quarantine_budget_escalated=true; budget=2 escalations=1 retained_exact_lanes=0 poisoned_scopes=0 poisoned_families=1") {
		t.Fatalf("Block family escalation typed caveat missing: %+v", result.Caveats)
	}
}

func TestPairingWindowDiscoveryCappedPhysicalAuditPoisonsOnlyRequestedFamily(t *testing.T) {
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage}
	discovery := newPairingWindowDiscovery(req, "/trace/capped-audit.systrace")
	discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyBlock, Kind: "schema_probe", laneKey: "block"})
	discovery.retainSchema(&WindowDiscoveryCandidate{Family: WindowDiscoveryFamilyStorage, Kind: "schema_probe", laneKey: "storage"})
	discovery.consumeDurationPairingAudit(&Index{durationOrderFailuresCapped: map[durationOrderFamily]bool{durationOrderStorage: true}})
	if !discovery.auditCapped || !discovery.identityIncomplete || !discovery.poisonedFamilies[WindowDiscoveryFamilyStorage] {
		t.Fatalf("capped storage physical audit did not fail the requested family closed: %+v", discovery)
	}
	if len(discovery.schema) != 1 || discovery.schema[0].Family != WindowDiscoveryFamilyBlock {
		t.Fatalf("capped storage audit crossed into the independent block family: %+v", discovery.schema)
	}
}
