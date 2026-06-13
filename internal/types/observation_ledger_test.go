package types

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestObservationLedgerExternalOriginsAreValidAndNonSource(t *testing.T) {
	for _, tc := range []struct {
		origin AnswerEvidenceOrigin
		source ObservationSourceKind
	}{
		{AnswerEvidenceOriginExternalDocument, ObservationSourceExternalDocument},
		{AnswerEvidenceOriginWebPage, ObservationSourceWebPage},
		{AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource},
		{AnswerEvidenceOriginConnectorResource, ObservationSourceConnector},
	} {
		if !tc.origin.IsValid() {
			t.Fatalf("origin %q should be valid", tc.origin)
		}
		if got := ObservationSourceKindForOrigin(tc.origin); got != tc.source {
			t.Fatalf("source kind for %q = %q, want %q", tc.origin, got, tc.source)
		}
		if got := AnswerClaimBindingGroundingPolicy(tc.origin, AnswerAggregateRolePrincipalAnswer); got != ClaimGroundingRepairable {
			t.Fatalf("principal grounding policy for %q = %q, want repairable", tc.origin, got)
		}
		if got := AnswerClaimBindingGroundingPolicy(tc.origin, AnswerAggregateRoleSupportingCoverage); got != ClaimGroundingSoft {
			t.Fatalf("support grounding policy for %q = %q, want soft", tc.origin, got)
		}
	}
}

func observationLedgerTestContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestAnswerEvidenceOriginCarriesOriginSpecificSupport(t *testing.T) {
	for _, origin := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCommandMeasurement,
		AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource,
	} {
		if !AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			t.Fatalf("origin %q should be origin-specific support", origin)
		}
	}
	for _, origin := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginUnknown,
		AnswerEvidenceOriginCurrentSource,
		AnswerEvidenceOriginSystemInference,
	} {
		if AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			t.Fatalf("origin %q should not be origin-specific non-source support", origin)
		}
	}
}

func TestAnswerAggregateFactEvidenceOrigins_ExternalResourceTokens(t *testing.T) {
	facts := []struct {
		token string
		want  AnswerEvidenceOrigin
	}{
		{"external_document", AnswerEvidenceOriginExternalDocument},
		{"web_page", AnswerEvidenceOriginWebPage},
		{"mcp_resource", AnswerEvidenceOriginMCPResource},
		{"connector_resource", AnswerEvidenceOriginConnectorResource},
	}
	for _, tc := range facts {
		fact := AnswerAggregateFact{
			Kind:  AnswerAggregateScalar,
			Label: "external fact",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "origin",
				Value: tc.token,
			}},
		}
		got := AnswerAggregateFactEvidenceOrigins(fact, nil)
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("origin token %q projected to %+v, want %q", tc.token, got, tc.want)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_NegativeObservationAllowsExternalOrigins(t *testing.T) {
	for _, origin := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource,
	} {
		facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "external no-hit",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(origin)},
				{Name: "target", Value: "MissingThing"},
				{Name: "scope", Value: "bounded external resource"},
				{Name: "result_count", Value: "0"},
			},
		}})
		if err != nil {
			t.Fatalf("negative_observation should allow origin %q: %v", origin, err)
		}
		if len(facts) != 1 {
			t.Fatalf("facts len for %q = %d, want 1", origin, len(facts))
		}
		got := AnswerAggregateFactEvidenceOrigins(facts[0], nil)
		if len(got) != 1 || got[0] != origin {
			t.Fatalf("normalized origin for %q = %+v", origin, got)
		}
	}
}

func TestCompileObservationLedger_CompilesExistingCarriers(t *testing.T) {
	input := ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:        "e1",
			Source:    "a.go",
			LineStart: 7,
			LineEnd:   8,
			Summary:   "source fact",
			Salience:  SalienceLoadBearing,
		}},
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "history no-hit",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
				{Name: "target", Value: "RemovedFeature"},
				{Name: "scope", Value: "HEAD~10..HEAD"},
				{Name: "result_count", Value: "0"},
			},
		}},
		ToolResults: []ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata count=1]\nabc123 latest feature",
		}},
		LogBundle: &LogBundle{Observations: []LogObservation{{
			Kind:      LogObservationRetryCycle,
			Subject:   "finalizer",
			Summary:   "finalizer retried",
			LineStart: 12,
		}}},
		PerfBundle: &PerfBundle{Observations: []PerfObservation{{
			Kind:       "gc",
			Subject:    "GC span",
			Summary:    "GC lasted 8ms",
			LineStart:  5,
			DurationMs: 8,
		}}},
		MCPResponses: []MCPResponse{{
			ServerName: "obs",
			Method:     "read_resource",
			Success:    true,
			Summary:    "cluster status is green",
		}},
	}
	ledger := CompileObservationLedger(input)
	if ledger.Empty() {
		t.Fatal("ledger should contain compiled records")
	}
	assertObservationRecord(t, ledger, "evidence:e1", AnswerEvidenceOriginCurrentSource, ObservationSourceCurrentSource)
	assertObservationRecord(t, ledger, "aggregate:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "tool:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "log:observation:0", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact)
	assertObservationRecord(t, ledger, "perf:observation:0", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact)
	assertObservationRecord(t, ledger, "mcp:0", AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource)
	if got := findObservationRecord(t, ledger, "evidence:e1"); got.GroundingPolicy != ClaimGroundingHard || got.Span.LineStart != 7 || got.Span.LineEnd != 8 {
		t.Fatalf("source evidence record did not preserve hard policy/span: %+v", got)
	}
	if got := findObservationRecord(t, ledger, "aggregate:0#vcs_metadata"); !got.Negative || got.ResultCount == nil || *got.ResultCount != 0 || got.Scope != "HEAD~10..HEAD" {
		t.Fatalf("negative aggregate record not preserved: %+v", got)
	}
	if got := findObservationRecord(t, ledger, "tool:0#vcs_metadata"); got.Summary != "abc123 latest feature" {
		t.Fatalf("tool ledger summary should skip typed banner line, got: %+v", got)
	}
	if got := findObservationRecord(t, ledger, "log:observation:0"); got.SourceRef.ArtifactID != "attached_log" || got.Span.LineStart != 12 {
		t.Fatalf("log observation should preserve artifact-local identity and line: %+v", got)
	}
	if got := findObservationRecord(t, ledger, "perf:observation:0"); got.SourceRef.ArtifactID != "attached_trace" || got.Span.LineStart != 5 {
		t.Fatalf("perf observation should preserve artifact-local identity and line: %+v", got)
	}
	if len(input.EvidenceItems) != 1 || input.EvidenceItems[0].Summary != "source fact" {
		t.Fatalf("compiler mutated input: %+v", input)
	}
}

func TestCompileObservationLedger_DemotesPerfPreTriageWhenTraceQueryExists(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:       AnswerAggregateScalar,
			Label:      "app-20 runnable total",
			Value:      "5.000",
			Unit:       "ms",
			Role:       AnswerAggregateRolePrincipalAnswer,
			Provenance: "trace_query:/tmp/trace.systrace#state_churn",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "artifact_id", Value: "attached_trace"},
				{Name: "artifact_kind", Value: "trace"},
				{Name: "target", Value: "app-20"},
				{Name: "predicate", Value: "state_churn"},
			},
		}},
		PerfBundle: &PerfBundle{Observations: []PerfObservation{{
			Kind:       "state_churn",
			Subject:    "app-20",
			Summary:    "pre-triage runnable total was estimated before trace_query",
			LineStart:  7,
			DurationMs: 3,
		}}},
	})
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		t.Fatal("ledger should expose deterministic runtime query presence")
	}
	trace := findObservationRecord(t, ledger, "aggregate:0#runtime_artifact")
	if trace.Producer != "trace_query:/tmp/trace.systrace#state_churn" ||
		trace.Role != AnswerAggregateRolePrincipalAnswer ||
		trace.GroundingPolicy != ClaimGroundingRepairable {
		t.Fatalf("trace_query aggregate should remain principal runtime support: %+v", trace)
	}
	perf := findObservationRecord(t, ledger, "perf:observation:0")
	if perf.Role != AnswerAggregateRoleSupportingCoverage || perf.GroundingPolicy != ClaimGroundingSoft {
		t.Fatalf("perf pre-triage should be demoted to supporting/soft when trace_query exists: %+v", perf)
	}
	if !observationLedgerTestContainsString(perf.RichNotes, "advisory_pretriage; deterministic_runtime_query_present=true") {
		t.Fatalf("perf pre-triage demotion should preserve an advisory note: %+v", perf.RichNotes)
	}
}

func TestCompileObservationLedger_TraceQueryRootCauseRankBecomesPrioritizedRuntimeRecords(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Summary: strings.Join([]string{
				"[trace_query params: view=recipe source=attached_trace path=/tmp/a.systrace origin=runtime_artifact artifact_id=attached_trace artifact_kind=trace payload_ref=/tmp/trace-query.json]",
				"# Trace Query: recipe",
				"source=/tmp/a.systrace lines=100 parsed_events=20 timestamp_unit=seconds selected_window=1.000000..1.200000 seconds",
				"## Root cause rank",
				"- rank=1 tier=primary type=scheduler_latency thread=com.app-42 impact=107.900ms target_impact=120.000ms score=86.320 confidence=0.88 lines=110-120 source=scheduler_latency_stats causality=on_wakeup_chain chain_depth=1 — com.app-42 runnable wait dominated the frame window",
				"- rank=2 tier=secondary type=cpu_pressure thread= impact=51.500ms score=30.900 confidence=0.74 lines=130-150 source=window_stats — cpu=10 had high runnable pressure",
				"## Wakeup chain",
				"- causal_impact thread=worker-21 depth=1 causality=on_wakeup_chain dominant_state=runnable impact=8.250ms total=9.000ms target_impact=12.500ms fragments=3 switches=2 max_segment=4.000ms p95_segment=4.000ms running=0.000ms runnable=8.250ms sleep=0.750ms d_state=0.000ms io_wait=0.000ms prio=20/ohos_cfs target_prio=52/ohos_rt priority_relation=lower_priority_dependency priority_inversion_candidate=true lines=80-88 — worker runnable dependency dominated the wakeup chain",
				"- root_evidence=binder_wait thread=binder:1-7 duration=31.800ms lines=90-99 confidence=0.86 — synchronous binder wait delayed the target",
				"## Window stats",
				"- bio_resource op=R path=/data/app/base.db thread=app-20 count=1 total_latency=2.500ms max_latency=2.500ms bytes=4096 line=3 example=op=R path=/data/app/base.db latency_us=2500 bytes=4096",
				"  callstack=BioRead>Submit",
				"- filesystem_resource op=read path=/data/app/base.db thread=app-20 count=1 total_latency=3.500ms max_latency=3.500ms bytes=1024 line=4 example=syscall=read path=/data/app/base.db duration_ms=3.5 bytes=1024",
				"- page_fault_resource op=major path=0x1234 thread=app-20 count=1 total_latency=0.150ms max_latency=0.150ms bytes=4096 line=5 example=operation=major address=0x1234 duration_us=150 size=4096",
				"- plugin_event kind=ability_monitor domain=AAFWK event=AbilityStart metric=latency_ms value=12.5 category=foreground thread=app-20 count=1 line=6 example=domain=AAFWK event_name=AbilityStart metric=latency_ms value=12.5 category=foreground",
				"- plugin_event kind=xpower domain=xpower event=xpower_cpu metric=CPU value=73 category=foreground thread=xpower-30 count=1 line=7 example=component=CPU energy=8.2 usage=73 scene=foreground",
				"- plugin_event kind=hi_sysevent domain=POWER event=THERMAL_REPORT metric=STAT value=hot category=MINOR thread=hisys-40 count=1 line=8 example=domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR",
				"- state_churn com.app-42 dominant_state=runnable impact=5.000ms total=8.000ms fragments=12 switches=11 max_segment=0.500ms p95_segment=0.500ms running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms confidence=0.83 lines=111-119 — com.app-42 had frequent state switching; dominant_state=runnable impact=5.000ms total=8.000ms fragments=12 switches=11 max_segment=0.500ms p95_segment=0.500ms totals running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms; next_step=inspect same-CPU pressure",
				"- file_io inode=0xb9b8e dev=260:136 name=foo.db op=write thread=com.app-42 count=3 bytes=12288 total_latency=1.500ms max_latency=1.000ms offsets=0..8192 lines=200-204 — inode=0xb9b8e dev=260:136 op=write count=3 bytes=12288 thread=com.app-42 name=foo.db",
				"- page_cache inode=0xb9b8e dev=260:136 thread=com.app-42 adds=2 deletes=1 churn=3 bytes=0 offsets=0..8192 lines=205-207 — inode=0xb9b8e dev=260:136 page-cache adds=2 deletes=1 churn=3 thread=com.app-42",
				"- storage_latency layer=scsi event=scsi_dispatch_cmd dev=12,80 op=read thread=com.app-42 count=2 paired=1 unpaired_start=0 unpaired_done=0 max_latency=2.000ms avg_latency=2.000ms bytes=8192 lines=208-209 — layer=scsi event=scsi_dispatch_cmd dev=12,80 op=read count=2 paired=1 max_latency=2.000ms",
				"- io_pressure signal=scheduler_iowait_with_storage_latency score=12.000 block_max=3.000ms storage_max=2.000ms file_bytes=12288 file_events=3 page_cache_churn=3 iowait_blocked=1 d_state=4.000ms top_inode=0xb9b8e top_dev=260:136 top_name=foo.db lines=200-209 — io pressure signal=scheduler_iowait_with_storage_latency score=12.000 block_max=3.000ms storage_max=2.000ms file_bytes=12288 file_events=3 page_cache_churn=3 iowait_blocked=1 d_state=4.000ms top_inode=0xb9b8e",
				"## Critical blocking calls",
				"- blocking type=monitor thread=binder:1-7 peer=com.app-42 duration=78.700ms lines=160-170 confidence=0.80 — monitor lock contention overlapped the frame",
			}, "\n"),
		}},
	})

	primary := findObservationRecord(t, ledger, "tool:0#trace_query:root_cause_rank:1")
	if primary.Role != AnswerAggregateRolePrincipalAnswer ||
		primary.ProvenanceLane != ObservationProvenanceObservedDirectCause ||
		primary.Predicate != "root_cause_primary" ||
		primary.Object != "scheduler_latency" ||
		primary.Value != "107.900" ||
		primary.Unit != "ms" ||
		primary.Span.LineStart != 110 ||
		primary.Span.LineEnd != 120 ||
		primary.Confidence != 0.88 {
		t.Fatalf("trace_query primary root cause record lost priority/line metadata: %+v", primary)
	}
	if !observationLedgerTestContainsString(primary.RichNotes, "rank=1") ||
		!observationLedgerTestContainsString(primary.RichNotes, "tier=primary") ||
		!observationLedgerTestContainsString(primary.RichNotes, "score=86.320") ||
		!observationLedgerTestContainsString(primary.RichNotes, "target_impact_ms=120.000") ||
		!observationLedgerTestContainsString(primary.RichNotes, "causality=on_wakeup_chain") ||
		!observationLedgerTestContainsString(primary.RichNotes, "chain_depth=1") {
		t.Fatalf("trace_query primary root cause should preserve rank/tier/score notes: %+v", primary.RichNotes)
	}
	causal := findObservationRecord(t, ledger, "tool:0#trace_query:wakeup_causal_impact:1")
	if causal.Role != AnswerAggregateRoleSupportingCoverage ||
		causal.Predicate != "wakeup_causal_impact" ||
		causal.Subject != "worker-21" ||
		causal.Object != "runnable" ||
		causal.Value != "8.250" ||
		causal.Unit != "ms" ||
		causal.Span.LineStart != 80 ||
		!observationLedgerTestContainsString(causal.RichNotes, "priority_inversion_candidate=true") ||
		!observationLedgerTestContainsString(causal.RichNotes, "target_impact=12.500ms") {
		t.Fatalf("trace_query causal impact should survive as supporting runtime observation: %+v", causal)
	}
	root := findObservationRecord(t, ledger, "tool:0#trace_query:root_evidence:1")
	if root.Role != AnswerAggregateRoleSupportingCoverage || root.Predicate != "binder_wait" || root.Span.LineStart != 90 {
		t.Fatalf("trace_query root evidence should survive as supporting runtime observation: %+v", root)
	}
	blocking := findObservationRecord(t, ledger, "tool:0#trace_query:critical_blocking:1")
	if blocking.Predicate != "critical_blocking" || blocking.Object != "monitor" || blocking.Value != "78.700" {
		t.Fatalf("trace_query critical blocking should survive as supporting runtime observation: %+v", blocking)
	}
	bio := findObservationRecord(t, ledger, "tool:0#trace_query:bio_resource:1")
	if bio.Predicate != "bio_resource" ||
		bio.Subject != "/data/app/base.db" ||
		bio.Object != "R" ||
		bio.Value != "2.500" ||
		bio.Unit != "ms" ||
		bio.Span.LineStart != 3 ||
		bio.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		bio.GroundingPolicy != ClaimGroundingSoft ||
		!observationLedgerTestContainsString(bio.RichNotes, "bytes=4096") ||
		!observationLedgerTestContainsString(bio.RichNotes, "callstack=BioRead>Submit") {
		t.Fatalf("trace_query bio_resource should survive as runtime observation: %+v", bio)
	}
	filesystem := findObservationRecord(t, ledger, "tool:0#trace_query:filesystem_resource:1")
	if filesystem.Predicate != "filesystem_resource" ||
		filesystem.Subject != "/data/app/base.db" ||
		filesystem.Object != "read" ||
		filesystem.Value != "3.500" ||
		!observationLedgerTestContainsString(filesystem.RichNotes, "bytes=1024") {
		t.Fatalf("trace_query filesystem_resource should survive as runtime observation: %+v", filesystem)
	}
	pageFault := findObservationRecord(t, ledger, "tool:0#trace_query:page_fault_resource:1")
	if pageFault.Predicate != "page_fault_resource" ||
		pageFault.Subject != "0x1234" ||
		pageFault.Object != "major" ||
		pageFault.Value != "0.150" ||
		!observationLedgerTestContainsString(pageFault.RichNotes, "bytes=4096") {
		t.Fatalf("trace_query page_fault_resource should survive as runtime observation: %+v", pageFault)
	}
	ability := findObservationRecord(t, ledger, "tool:0#trace_query:plugin_event:1")
	if ability.Predicate != "ability_monitor" ||
		ability.Subject != "AAFWK" ||
		ability.Object != "AbilityStart" ||
		ability.Value != "12.5" ||
		ability.Span.LineStart != 6 ||
		!observationLedgerTestContainsString(ability.RichNotes, "metric=latency_ms") {
		t.Fatalf("trace_query ability plugin event should survive as runtime observation: %+v", ability)
	}
	xpower := findObservationRecord(t, ledger, "tool:0#trace_query:plugin_event:2")
	if xpower.Predicate != "xpower" ||
		xpower.Subject != "xpower" ||
		xpower.Object != "xpower_cpu" ||
		xpower.Value != "73" ||
		!observationLedgerTestContainsString(xpower.RichNotes, "category=foreground") {
		t.Fatalf("trace_query xpower plugin event should survive as runtime observation: %+v", xpower)
	}
	hiSys := findObservationRecord(t, ledger, "tool:0#trace_query:plugin_event:3")
	if hiSys.Predicate != "hi_sysevent" ||
		hiSys.Subject != "POWER" ||
		hiSys.Object != "THERMAL_REPORT" ||
		hiSys.Value != "hot" ||
		!observationLedgerTestContainsString(hiSys.RichNotes, "metric=STAT") {
		t.Fatalf("trace_query hi_sysevent plugin event should survive as runtime observation: %+v", hiSys)
	}
	churn := findObservationRecord(t, ledger, "tool:0#trace_query:state_churn:1")
	if churn.Predicate != "state_churn" ||
		churn.Object != "runnable" ||
		churn.Subject != "com.app-42" ||
		churn.Value != "5.000" ||
		churn.Unit != "ms" ||
		churn.Span.LineStart != 111 ||
		churn.Span.LineEnd != 119 ||
		churn.Confidence != 0.83 ||
		!observationLedgerTestContainsString(churn.RichNotes, "fragments=12") ||
		!observationLedgerTestContainsString(churn.RichNotes, "max_segment=0.500ms") {
		t.Fatalf("trace_query state_churn should survive as supporting runtime observation: %+v", churn)
	}
	fileIO := findObservationRecord(t, ledger, "tool:0#trace_query:file_io:1")
	if fileIO.Predicate != "file_io_by_inode" ||
		fileIO.Subject != "foo.db" ||
		fileIO.Object != "write" ||
		fileIO.Value != "12288" ||
		fileIO.Unit != "bytes" ||
		fileIO.Span.LineStart != 200 ||
		!observationLedgerTestContainsString(fileIO.RichNotes, "inode=0xb9b8e") {
		t.Fatalf("trace_query file_io should survive as runtime observation: %+v", fileIO)
	}
	pageCache := findObservationRecord(t, ledger, "tool:0#trace_query:page_cache:1")
	if pageCache.Predicate != "page_cache_by_inode" ||
		pageCache.Value != "3" ||
		pageCache.Unit != "events" ||
		!observationLedgerTestContainsString(pageCache.RichNotes, "deletes=1") {
		t.Fatalf("trace_query page_cache should survive as runtime observation: %+v", pageCache)
	}
	storage := findObservationRecord(t, ledger, "tool:0#trace_query:storage_latency:1")
	if storage.Predicate != "storage_latency_by_layer" ||
		storage.Subject != "scsi" ||
		storage.Value != "2.000" ||
		storage.Unit != "ms" {
		t.Fatalf("trace_query storage_latency should survive as runtime observation: %+v", storage)
	}
	pressure := findObservationRecord(t, ledger, "tool:0#trace_query:io_pressure:1")
	if pressure.Predicate != "scheduler_iowait_with_storage_latency" ||
		pressure.Object != "0xb9b8e" ||
		pressure.Value != "12.000" ||
		pressure.Unit != "score" {
		t.Fatalf("trace_query io_pressure should survive as runtime observation: %+v", pressure)
	}

	rm := RequestModel{PerfTrace: &PerfBundle{Janks: []PerfJank{{TriggerSpan: "frame 7"}}}}
	promptRecords := ProjectObservationPromptRecords(ledger.Records, &rm, nil, DefaultObservationPromptProjectionOptions(4))
	if len(promptRecords) == 0 || promptRecords[0].ID != "tool:0#trace_query:root_cause_rank:1" {
		t.Fatalf("principal root cause rank should be prompt-prioritized, got %+v", promptRecords)
	}
	if promptRecords[0].Producer != "trace_query" {
		t.Fatalf("trace_query producer should survive prompt projection, got %+v", promptRecords[0])
	}
}

func TestCompileObservationLedger_CategoricalAggregateIsNotCount(t *testing.T) {
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateErrorGranularity,
		Label: "failure scope verdict",
		Value: "per_item_rejection",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{
			{Name: "target", Value: "emit_evidence invalid item"},
			{Name: "predicate", Value: "failure_scope"},
		},
		SupportRefs: []string{"internal/tool/emit_evidence.go:560"},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts: %v", err)
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: facts})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if got.Value != "per_item_rejection" {
		t.Fatalf("categorical value should be preserved, got %+v", got)
	}
	if got.ResultCount != nil {
		t.Fatalf("categorical aggregate must not become a numeric count: %+v", got)
	}
	if len(got.SupportRefs) != 1 || got.SupportRefs[0] != "internal/tool/emit_evidence.go:560" {
		t.Fatalf("support refs should be preserved: %+v", got.SupportRefs)
	}
}

func TestCompileObservationLedger_SourceInventoryObservation(t *testing.T) {
	rowSets := map[string]string{}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		SourceInventoryObservation: SourceInventoryObservation{
			Active:     true,
			Complete:   true,
			Scopes:     []string{"src"},
			Provenance: []string{"repo_lens:tool_query", "repomap_graph"},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Count:    999,
				Members: []SourceInventoryObservationMember{{
					Name:          "Run",
					Key:           "Run",
					SupportRef:    "Run: src/run.py:7",
					Role:          AnswerCandidateRoleFunction,
					File:          "src/run.py",
					Line:          7,
					Language:      "python",
					CoverageState: SourceInventoryCoverageObserved,
					Attributes: []SourceInventoryObservationAttribute{{
						Name:          "build",
						Key:           "build",
						SupportRef:    "build: src/run.py:11",
						Role:          AnswerCandidateRoleFunction,
						File:          "src/run.py",
						Line:          11,
						CoverageState: SourceInventoryCoverageAmbiguous,
						Ambiguity:     "one_of_many_candidate_attributes",
					}},
				}},
			}},
		},
		RowSetWriter: func(name, content string) string {
			rowSets[name] = content
			return "rows://" + name
		},
	})
	set := findObservationRecord(t, ledger, "source_inventory:set:0")
	if set.ResultCount == nil || *set.ResultCount != 1 {
		t.Fatalf("source inventory count must normalize to len(members), got %+v", set)
	}
	member := findObservationRecord(t, ledger, "source_inventory:0:0")
	if member.SourceRef.Path != "src/run.py" || member.Span.LineStart != 7 ||
		member.AnchorKind != AnchorDefinition || member.GroundingPolicy != ClaimGroundingRepairable {
		t.Fatalf("member observation should preserve exact mechanical location: %+v", member)
	}
	attr := findObservationRecord(t, ledger, "source_inventory:0:0:attr:0")
	if attr.Object != "build" || len(attr.RichNotes) == 0 ||
		!strings.Contains(strings.Join(attr.RichNotes, " "), "one_of_many_candidate_attributes") {
		t.Fatalf("attribute ambiguity should be preserved: %+v", attr)
	}
	if len(rowSets) != 0 {
		t.Fatalf("single-row source inventory should not create row set refs: %+v", rowSets)
	}
}

func TestCompileObservationLedger_AggregateRichNotesPreferMemberNotes(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "Kind 常量",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"KindSymbolPresent", "KindNoCallSites"},
			MemberNotes: []string{
				"KindSymbolPresent 用于符号存在性判定，检查目标符号是否能在当前证据中解析。",
				"KindNoCallSites 用于调用点缺失判定，表达没有发现调用关系的负向条件。",
			},
			SupportRefs: []string{
				"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
				"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
			},
		}},
	})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if len(got.RichNotes) < 2 {
		t.Fatalf("rich member notes should be preserved in the ledger: %+v", got)
	}
	if got.RichNotes[0] != "KindSymbolPresent 用于符号存在性判定，检查目标符号是否能在当前证据中解析。" {
		t.Fatalf("member_notes should outrank dry member names, got: %+v", got.RichNotes)
	}
	if got.RichNotes[1] != "KindNoCallSites 用于调用点缺失判定，表达没有发现调用关系的负向条件。" {
		t.Fatalf("second member note lost: %+v", got.RichNotes)
	}
	if got.RichNotes[2] != "KindSymbolPresent" {
		t.Fatalf("members should remain as fallback notes after rich notes, got: %+v", got.RichNotes)
	}
}

func TestCompileObservationLedger_MergesDuplicateSourceAnchorsRichSummaries(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{
			{
				ID:           "eval-main",
				Kind:         EvidenceDirect,
				Source:       "internal/analysis/criterion/eval.go",
				LineStart:    15,
				AnchorKind:   AnchorDefinition,
				AnchorSymbol: "Eval",
				Summary:      "Eval 对单个 Criterion 求值。",
				SurfaceTerms: []string{"Eval"},
				Salience:     SalienceLoadBearing,
			},
			{
				ID:           "eval-unknown",
				Kind:         EvidenceDirect,
				Source:       "internal/analysis/criterion/eval.go",
				LineStart:    15,
				AnchorKind:   AnchorDefinition,
				AnchorSymbol: "Eval",
				Summary:      "未知 Kind 返回 Result.UnknownKind。",
				SurfaceTerms: []string{"Result.UnknownKind"},
			},
			{
				ID:           "eval-call",
				Kind:         EvidenceDirect,
				Source:       "internal/analysis/criterion/eval.go",
				LineStart:    15,
				AnchorKind:   AnchorCall,
				AnchorSymbol: "Eval",
				Predicate:    "calls",
				Object:       "registry.Lookup",
				Summary:      "Eval 会查询注册表。",
			},
		},
	})
	if len(ledger.Records) != 2 {
		t.Fatalf("same-anchor summaries should merge while distinct claims remain separate: %+v", ledger.Records)
	}
	got := findObservationRecord(t, ledger, "evidence:eval-main")
	if got.Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("merged record should preserve stronger role, got %+v", got.Role)
	}
	for _, want := range []string{
		"Eval 对单个 Criterion 求值。",
		"未知 Kind 返回 Result.UnknownKind。",
	} {
		if !containsObservationString(got.RichNotes, want) {
			t.Fatalf("merged rich notes missing %q: %+v", want, got.RichNotes)
		}
	}
	for _, want := range []string{"Eval", "Result.UnknownKind"} {
		if !containsObservationString(got.SupportRefs, want) {
			t.Fatalf("merged support refs missing %q: %+v", want, got.SupportRefs)
		}
	}
	if findObservationRecord(t, ledger, "evidence:eval-call").Object != "registry.Lookup" {
		t.Fatalf("distinct call claim should not be merged away: %+v", ledger.Records)
	}
}

func TestCompileObservationLedger_MixedDiffAndCurrentSourceStaySeparate(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:        "current",
			Source:    "internal/current.go",
			LineStart: 21,
			Summary:   "current implementation still calls Apply",
			Salience:  SalienceLoadBearing,
		}},
		ToolResults: []ToolResult{{
			ToolName: "git_diff",
			Success:  true,
			Summary:  "[git_diff: evidence_origin=vcs_diff ref=HEAD~1]\n- old path\n+ new path",
		}},
	})

	current := findObservationRecord(t, ledger, "evidence:current")
	diff := findObservationRecord(t, ledger, "tool:0#vcs_diff")
	if current.Origin != AnswerEvidenceOriginCurrentSource || current.SourceRef.Path != "internal/current.go" {
		t.Fatalf("current-source record drifted: %+v", current)
	}
	if diff.Origin != AnswerEvidenceOriginVCSDiff || diff.SourceRef.Kind != ObservationSourceVCSDiff {
		t.Fatalf("diff record drifted: %+v", diff)
	}
	if diff.GroundingPolicy == ClaimGroundingHard {
		t.Fatalf("diff support record must not become current-source hard gate: %+v", diff)
	}
}

func TestCompileObservationLedger_CurrentSourceHardRequiresExactLineSpan(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:              "ungrounded",
			Source:          "internal/current.go",
			LineStart:       0,
			Summary:         "load-bearing but not line-addressable",
			Salience:        SalienceLoadBearing,
			GroundingStatus: GroundingGrounded,
		}},
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "current source aggregate without support",
			Value: "1",
			Role:  AnswerAggregateRolePrincipalAnswer,
		}},
	})

	ev := findObservationRecord(t, ledger, "evidence:ungrounded")
	if ObservationRecordHasCurrentSourceLineSpan(ev) {
		t.Fatalf("line-start 0 evidence must not be citation-eligible: %+v", ev)
	}
	if ev.GroundingPolicy == ClaimGroundingHard {
		t.Fatalf("current-source ledger record without exact span must downgrade from hard: %+v", ev)
	}
	agg := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if agg.GroundingPolicy == ClaimGroundingHard {
		t.Fatalf("current-source aggregate without source support must not become hard citation pressure: %+v", agg)
	}
}

func TestCompileObservationLedger_ExternalErrorInfoObservationIsSupportOnly(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		LogBundle: &LogBundle{
			Errors: []LogError{{
				Type:    "TypeError",
				Message: "Cannot read property name of undefined",
				Frames: []LogFrame{{
					Raw: "at UserCard.build (entry/src/main/ets/UserCard.ets:42)",
				}},
			}},
			Observations: []LogObservation{{
				Kind:       LogObservationRuntimeEvent,
				Subject:    "UserCard.build",
				Summary:    "UserCard.build 读取 undefined.name 时崩溃",
				Diagnostic: true,
				Severity:   LogObservationFailure,
				LineStart:  42,
			}, {
				Kind:      LogObservationRuntimeEvent,
				Subject:   "IndexPage.build",
				Summary:   "IndexPage.build 位于调用栈上",
				Severity:  LogObservationInfo,
				LineStart: 128,
			}},
		},
	})

	diagnostic := findObservationRecord(t, ledger, "log:observation:0")
	if diagnostic.Role != AnswerAggregateRolePrincipalAnswer || diagnostic.GroundingPolicy != ClaimGroundingRepairable {
		t.Fatalf("diagnostic runtime observation should remain principal repairable evidence, got %+v", diagnostic)
	}
	if diagnostic.ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("diagnostic runtime observation should carry observed_direct_cause lane, got %+v", diagnostic)
	}
	context := findObservationRecord(t, ledger, "log:observation:1")
	if context.Role != AnswerAggregateRoleSupportingCoverage || context.GroundingPolicy != ClaimGroundingSoft {
		t.Fatalf("non-diagnostic info observation in an external error log should be support-only, got %+v", context)
	}
	if context.ProvenanceLane != ObservationProvenanceArtifactSpan {
		t.Fatalf("non-diagnostic log line should carry artifact_span lane, got %+v", context)
	}
	errRecord := findObservationRecord(t, ledger, "log:error:0")
	if errRecord.ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("log error should carry observed_direct_cause lane, got %+v", errRecord)
	}
	if len(errRecord.RichNotes) == 0 || !strings.Contains(errRecord.RichNotes[0], "artifact-local") {
		t.Fatalf("log error should warn that stack refs are artifact-local support, got %+v", errRecord.RichNotes)
	}
}

func TestCompileObservationLedger_RuntimeArtifactProvenanceLanes(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateScalar,
			Label: "possible upstream explanation",
			Value: "candidate",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "runtime_lane", Value: string(ObservationProvenanceInferredUpstreamPossibility)},
				{Name: "artifact_id", Value: "attached_log"},
			},
		}},
		PerfBundle: &PerfBundle{
			Frames: []PerfFrame{{
				DurationMs: 24.5,
				TsMs:       100,
			}, {
				FrameNo:    7,
				DurationMs: 31,
				TsMs:       140,
			}},
			Janks: []PerfJank{{
				StartTsMs:   200,
				DurationMs:  120,
				TriggerSpan: "RenderPipeline.Draw",
				Reason:      "io",
			}},
			Stalls: []PerfStall{{
				StartTsMs:  220,
				DurationMs: 110,
				Kind:       "lock",
				Symbol:     "WaitFence",
			}},
		},
	})

	agg := findObservationRecord(t, ledger, "aggregate:0#runtime_artifact")
	if agg.ProvenanceLane != ObservationProvenanceInferredUpstreamPossibility {
		t.Fatalf("aggregate runtime_lane should survive as typed provenance lane, got %+v", agg)
	}
	frame0 := findObservationRecord(t, ledger, "perf:frame:0")
	if frame0.ProvenanceLane != ObservationProvenanceArtifactSpan {
		t.Fatalf("frame sample should carry artifact_span lane, got %+v", frame0)
	}
	for _, forbidden := range []string{frame0.ClaimKey, frame0.Subject, frame0.Summary} {
		if strings.Contains(forbidden, "frame 0") || strings.Contains(forbidden, "frame:0") {
			t.Fatalf("frame without artifact ordinal must not expose zero-based frame label: %+v", frame0)
		}
	}
	if frame0.Span.StartTsMs != 100 || frame0.Span.EndTsMs != 124.5 {
		t.Fatalf("frame sample should preserve trace time span, got %+v", frame0.Span)
	}
	frame7 := findObservationRecord(t, ledger, "perf:frame:1")
	if !strings.Contains(frame7.Summary, "frame 7") {
		t.Fatalf("explicit artifact frame ordinal should be preserved, got %+v", frame7)
	}
	jank := findObservationRecord(t, ledger, "perf:jank:0")
	if jank.ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("typed jank reason should carry observed_direct_cause lane, got %+v", jank)
	}
	stall := findObservationRecord(t, ledger, "perf:stall:0")
	if stall.ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("typed stall symbol should carry observed_direct_cause lane, got %+v", stall)
	}
}

func TestCompileObservationLedger_MergesDuplicateRuntimeLanesWithoutLosingStrongerLane(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateScalar,
			Label: "runtime symptom",
			Value: "TypeError",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "artifact_id", Value: "attached_log"},
				{Name: "line_start", Value: "42"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "runtime symptom",
			Value: "TypeError",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "runtime_lane", Value: string(ObservationProvenanceObservedDirectCause)},
				{Name: "artifact_id", Value: "attached_log"},
				{Name: "line_start", Value: "42"},
			},
		},
	}})
	if len(ledger.Records) != 1 {
		t.Fatalf("same runtime observation should merge even when one record adds a lane: %+v", ledger.Records)
	}
	if ledger.Records[0].ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("merged runtime observation should preserve stronger lane, got %+v", ledger.Records[0])
	}
}

func TestObservationRecordCurrentSourceSpanDistinguishesExternalCoordinates(t *testing.T) {
	logRecord := ObservationRecord{
		Origin:    AnswerEvidenceOriginRuntimeArtifact,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: "log"},
		Span:      ObservationSpan{LineStart: 42},
	}
	if ObservationRecordHasCurrentSourceLineSpan(logRecord) || ObservationRecordHasStrongCurrentSourceAnchor(logRecord) {
		t.Fatalf("artifact-local log lines must not become current-source citation anchors: %+v", logRecord)
	}
	currentRecord := ObservationRecord{
		Origin: AnswerEvidenceOriginCurrentSource,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceCurrentSource,
			Path: "internal/service.go",
		},
		Span:            ObservationSpan{LineStart: 42},
		AnchorKind:      AnchorAssignment,
		EvidenceScope:   ScopeLine,
		GroundingStatus: GroundingRecovered,
	}
	if !ObservationRecordHasCurrentSourceLineSpan(currentRecord) || !ObservationRecordHasStrongCurrentSourceAnchor(currentRecord) {
		t.Fatalf("recovered current-source assignment should remain exact enough for current-code ranking: %+v", currentRecord)
	}
	currentRecord.GroundingStatus = GroundingUngrounded
	if ObservationRecordHasCurrentSourceLineSpan(currentRecord) {
		t.Fatalf("ungrounded current-source lead must not be citation-eligible: %+v", currentRecord)
	}
}

func TestCompileObservationLedger_MixedPositiveAndNegativeExternalFactsStayTargetScoped(t *testing.T) {
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateScalar,
			Label: "matched incident",
			Value: "INC-7",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginMCPResource)},
				{Name: "target", Value: "open incident"},
				{Name: "scope", Value: "resource://obs/incidents"},
			},
		},
		{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "no fatal alert",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginMCPResource)},
				{Name: "target", Value: "fatal alert"},
				{Name: "scope", Value: "resource://obs/incidents"},
				{Name: "result_count", Value: "0"},
			},
		},
	})
	if err != nil {
		t.Fatalf("facts should normalize: %v", err)
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: facts})

	positive := findObservationRecord(t, ledger, "aggregate:0#mcp_resource")
	negative := findObservationRecord(t, ledger, "aggregate:1#mcp_resource")
	if positive.Negative {
		t.Fatalf("positive fact became negative: %+v", positive)
	}
	if !negative.Negative || negative.Subject != "fatal alert" {
		t.Fatalf("negative fact lost target scope: %+v", negative)
	}
	if positive.Subject == negative.Subject {
		t.Fatalf("positive and negative facts collapsed to one target: %+v / %+v", positive, negative)
	}
}

func TestObservationLedgerTypeLayerDoesNotImportGroundingTool(t *testing.T) {
	for _, file := range []string{
		"observation_ledger.go",
		"observation_ledger_context.go",
		"answer_evidence_origin.go",
		"answer_claim_binding.go",
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(body), "internal/tool/ground") {
			t.Fatalf("%s must not import or depend on internal/tool/ground; external observations cannot enter repo grounding", file)
		}
	}
}

func TestCompileObservationLedger_PreservesExternalPagingRefsAndLocalSpans(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateScalar,
			Label: "line total",
			Value: "70693",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginCommandMeasurement)},
				{Name: "command", Value: "find internal/tool -name '*.go' | xargs wc -l"},
				{Name: "raw_ref", Value: "/tmp/codrax/blob/exec_command-1234.txt"},
			},
		}, {
			Kind:  AnswerAggregateMemberSet,
			Label: "external row set",
			Value: "2",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginCommandMeasurement)},
				{Name: "payload_ref", Value: "blob://payload/exec-command-full.txt"},
				{Name: "row_set_ref", Value: "blob://rows/exec-command-rows.jsonl"},
				{Name: "page_ref", Value: "blob://payload/exec-command-full.txt?page=1"},
			},
		}},
		ToolResults: []ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata count=20]\nabc123 feature summary",
			RawRef:   "/tmp/codrax/blob/git_log-5678.txt",
		}},
		LogBundle: &LogBundle{Observations: []LogObservation{{
			Kind:      LogObservationRuntimeEvent,
			Subject:   "panic frame",
			Summary:   "panic at frame",
			LineStart: 40,
			LineEnd:   43,
		}}},
		PerfBundle: &PerfBundle{Observations: []PerfObservation{{
			Kind:      "jank",
			Subject:   "render stall",
			Summary:   "render stall in trace",
			LineStart: 9,
			StartTsMs: 120.5,
			EndTsMs:   168.75,
		}}},
	})

	measurement := findObservationRecord(t, ledger, "aggregate:0#command_measurement")
	if measurement.SourceRef.RawRef != "/tmp/codrax/blob/exec_command-1234.txt" ||
		measurement.SourceRef.PayloadRef != "/tmp/codrax/blob/exec_command-1234.txt" ||
		measurement.SourceRef.Command == "" {
		t.Fatalf("command measurement should preserve blob paging ref and command: %+v", measurement)
	}
	rowSet := findObservationRecord(t, ledger, "aggregate:1#command_measurement")
	if rowSet.SourceRef.PayloadRef != "blob://payload/exec-command-full.txt" ||
		rowSet.SourceRef.RowSetRef != "blob://rows/exec-command-rows.jsonl" ||
		rowSet.SourceRef.PageRef != "blob://payload/exec-command-full.txt?page=1" {
		t.Fatalf("typed payload/row/page refs should be preserved: %+v", rowSet.SourceRef)
	}
	gitRecord := findObservationRecord(t, ledger, "tool:0#vcs_metadata")
	if gitRecord.SourceRef.RawRef != "/tmp/codrax/blob/git_log-5678.txt" {
		t.Fatalf("git tool observation should preserve blob paging ref: %+v", gitRecord)
	}
	logRecord := findObservationRecord(t, ledger, "log:observation:0")
	if logRecord.SourceRef.ArtifactID != "attached_log" || logRecord.Span.LineStart != 40 || logRecord.Span.LineEnd != 43 {
		t.Fatalf("log observation should preserve artifact-local lines: %+v", logRecord)
	}
	perfRecord := findObservationRecord(t, ledger, "perf:observation:0")
	if perfRecord.SourceRef.ArtifactID != "attached_trace" ||
		perfRecord.Span.LineStart != 9 ||
		perfRecord.Span.StartTsMs != 120.5 ||
		perfRecord.Span.EndTsMs != 168.75 {
		t.Fatalf("perf observation should preserve artifact-local line/time spans: %+v", perfRecord)
	}
}

func TestCompileObservationLedger_AutoCreatesLargeAggregateRowSetRef(t *testing.T) {
	members := make([]string, 0, observationRowSetMinRows)
	notes := make([]string, 0, observationRowSetMinRows)
	refs := make([]string, 0, observationRowSetMinRows)
	for i := 0; i < observationRowSetMinRows; i++ {
		members = append(members, fmt.Sprintf("Kind%02d", i))
		notes = append(notes, fmt.Sprintf("Kind%02d 的中文说明", i))
		refs = append(refs, fmt.Sprintf("Kind%02d: internal/types/kind.go:%d", i, i+10))
	}
	var seenName, seenContent string
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind members",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     members,
			MemberNotes: notes,
			SupportRefs: refs,
		}},
		RowSetWriter: func(name, content string) string {
			seenName = name
			seenContent = content
			return "blob://rows/" + name
		},
	})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if got.SourceRef.RowSetRef != "blob://rows/aggregate-000-current_source-row-set.jsonl" {
		t.Fatalf("row_set_ref not attached: %+v", got.SourceRef)
	}
	if seenName != "aggregate-000-current_source-row-set.jsonl" {
		t.Fatalf("unexpected row-set artifact name %q", seenName)
	}
	for _, want := range []string{
		`"member":"Kind00"`,
		`"support_ref":"Kind00: internal/types/kind.go:10"`,
		`"note":"Kind00 的中文说明"`,
		`"label":"Kind members"`,
	} {
		if !strings.Contains(seenContent, want) {
			t.Fatalf("row-set content missing %q:\n%s", want, seenContent)
		}
	}
}

func TestCompileObservationLedger_SmallAggregateDoesNotCreateRowSetRef(t *testing.T) {
	called := false
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "small",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"A", "B"},
		}},
		RowSetWriter: func(name, content string) string {
			called = true
			return "blob://rows/" + name
		},
	})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if called || got.SourceRef.RowSetRef != "" {
		t.Fatalf("small aggregate should stay inline-only, called=%v ref=%q", called, got.SourceRef.RowSetRef)
	}
}

func TestCompileObservationLedger_RowSetSupportRefsAvoidIndexMismatches(t *testing.T) {
	members := make([]string, 0, observationRowSetMinRows)
	notes := make([]string, 0, observationRowSetMinRows)
	for i := 0; i < observationRowSetMinRows; i++ {
		members = append(members, fmt.Sprintf("Kind%02d", i))
		notes = append(notes, fmt.Sprintf("Kind%02d note", i))
	}
	var seenContent string
	CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind members",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     members,
			MemberNotes: notes,
			SupportRefs: []string{"Kind07: internal/types/kind.go:70"},
		}},
		RowSetWriter: func(name, content string) string {
			seenContent = content
			return "blob://rows/" + name
		},
	})
	if strings.Contains(seenContent, `"member":"Kind00","support_ref"`) {
		t.Fatalf("short support_refs must not be attached by index:\n%s", seenContent)
	}
	if !strings.Contains(seenContent, `"member":"Kind07","support_ref":"Kind07: internal/types/kind.go:70"`) {
		t.Fatalf("label-matched support_ref should be preserved:\n%s", seenContent)
	}
}

func TestCompileObservationLedger_AutoCreatesLargeExcludedRowSetRef(t *testing.T) {
	excluded := make([]string, 0, observationRowSetMinRows)
	refs := make([]string, 0, observationRowSetMinRows)
	for i := 0; i < observationRowSetMinRows; i++ {
		excluded = append(excluded, fmt.Sprintf("legacy-%02d", i))
		refs = append(refs, fmt.Sprintf("legacy-%02d: internal/types/legacy.go:%d", i, i+20))
	}
	var seenContent string
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateExcluded,
			Label:       "excluded legacy constants",
			Value:       fmt.Sprintf("%d", len(excluded)),
			Role:        AnswerAggregateRoleSupportingCoverage,
			Excluded:    excluded,
			SupportRefs: refs,
		}},
		RowSetWriter: func(name, content string) string {
			seenContent = content
			return "blob://rows/" + name
		},
	})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if got.SourceRef.RowSetRef != "blob://rows/aggregate-000-current_source-row-set.jsonl" {
		t.Fatalf("row_set_ref not attached for exact excluded set: %+v", got.SourceRef)
	}
	for _, want := range []string{
		`"member":"legacy-00"`,
		`"support_ref":"legacy-00: internal/types/legacy.go:20"`,
		`"label":"excluded legacy constants"`,
		`"kind":"excluded_count"`,
	} {
		if !strings.Contains(seenContent, want) {
			t.Fatalf("excluded row-set content missing %q:\n%s", want, seenContent)
		}
	}
}

func TestCompileObservationLedger_PartialExcludedSetDoesNotCreateRowSetRef(t *testing.T) {
	called := false
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:     AnswerAggregateExcluded,
			Label:    "partial excluded",
			Value:    "3",
			Excluded: []string{"legacy-00", "legacy-01"},
		}},
		RowSetWriter: func(name, content string) string {
			called = true
			return "blob://rows/" + name
		},
	})
	got := findObservationRecord(t, ledger, "aggregate:0#current_source")
	if called || got.SourceRef.RowSetRef != "" {
		t.Fatalf("partial excluded set should stay inline-only, called=%v ref=%q", called, got.SourceRef.RowSetRef)
	}
}

func TestCompileObservationLedger_ProjectsToolBannerCoordinates(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{
		{
			ToolName: "git_show",
			Success:  true,
			Summary:  "[git_show: repo_path=. ref=abc123 path=internal/tool format=medium no_patch=false stat=false name_only=false evidence_origin=vcs_metadata diff_origin=vcs_diff]\ncommit abc123\n",
			RawRef:   "/tmp/codrax/blob/git_show-1.txt",
		},
		{
			ToolName: "git_history_search",
			Success:  true,
			Summary: "[git_history_search: window_path=internal/orchestrator window_count=20 order=recent diff_path=internal/orchestrator contains=runTaskGraph evidence_origin=vcs_metadata]\n" +
				"window_size=20\nanswer_count=0\nmatched_commits:\n- none\n",
			RawRef: "/tmp/codrax/blob/git_history_search-1.txt",
		},
		{
			ToolName: "git_log",
			Success:  true,
			Summary: "[git_log: pathspec=internal/tool ref=HEAD count=1 format=medium stat=true name_only=false merges_only=true no_merges=false first_parent=true evidence_origin=vcs_metadata]\n" +
				"exact_changed_paths: copy these exact paths; do not expand abbreviated --stat paths containing `...`.\n" +
				"exact_path commit=abc123 path=\"docs/design/answer_surface_safety_batch_20260523.md\"\n\n" +
				"commit abc123\n",
			RawRef: "/tmp/codrax/blob/git_log-merge.txt",
		},
		{
			ToolName: "exec_command",
			Success:  true,
			Summary:  "[exec_command: $ git log --oneline -n 3]\n[exec_command: evidence_origin=vcs_metadata]\nabc123 feature\n",
			RawRef:   "/tmp/codrax/blob/exec_command-1.txt",
		},
		{
			ToolName: "trace_query",
			Success:  true,
			Summary: "[trace_query params: view=wakeup_chain source=path path=record.systrace origin=runtime_artifact artifact_id=trace_query artifact_kind=trace line_start=1102623 line_end=1139184 time_start=2942.124416 time_end=2942.260210 payload_ref=blob://payload/trace-query-result.json]\n" +
				"# Trace Query: wakeup_chain\n\n- root_evidence=runnable thread=com.tencent.mm-36379 duration=135.794ms lines=1102623-1139184 confidence=0.80\n",
			RawRef: "/tmp/codrax/blob/trace-query-summary.txt",
		},
	}})
	showMetadata := findObservationRecord(t, ledger, "tool:0#vcs_metadata")
	if showMetadata.SourceRef.Commit != "abc123" ||
		showMetadata.SourceRef.Pathspec != "internal/tool" ||
		showMetadata.SourceRef.RawRef != "/tmp/codrax/blob/git_show-1.txt" ||
		showMetadata.SourceRef.PayloadRef != "/tmp/codrax/blob/git_show-1.txt" {
		t.Fatalf("git_show metadata coordinates not projected: %+v", showMetadata.SourceRef)
	}
	showDiff := findObservationRecord(t, ledger, "tool:0#vcs_diff")
	if showDiff.SourceRef.Commit != "abc123" || showDiff.SourceRef.Pathspec != "internal/tool" {
		t.Fatalf("git_show diff coordinates not projected: %+v", showDiff.SourceRef)
	}
	history := findObservationRecord(t, ledger, "tool:1#vcs_metadata")
	if history.SourceRef.Pathspec != "internal/orchestrator" ||
		history.SourceRef.Range != "order=recent window_count=20" ||
		history.ResultCount == nil ||
		*history.ResultCount != 0 ||
		history.ClaimKey != "runTaskGraph" {
		t.Fatalf("git_history_search coordinates/result count not projected: record=%+v", history)
	}
	log := findObservationRecord(t, ledger, "tool:2#vcs_metadata")
	if log.SourceRef.Pathspec != "internal/tool" ||
		log.SourceRef.Range != "ref=HEAD count=1 first_parent=true merges_only=true" ||
		log.SourceRef.RawRef != "/tmp/codrax/blob/git_log-merge.txt" ||
		log.SourceRef.PayloadRef != "/tmp/codrax/blob/git_log-merge.txt" {
		t.Fatalf("git_log filter coordinates not projected: %+v", log.SourceRef)
	}
	if log.Summary != "commit abc123" ||
		len(log.RichNotes) == 0 ||
		!strings.Contains(strings.Join(log.RichNotes, "\n"), "answer_surface_safety_batch_20260523.md") {
		t.Fatalf("git_log exact detail should be notes while summary stays commit line: summary=%q notes=%v", log.Summary, log.RichNotes)
	}
	exec := findObservationRecord(t, ledger, "tool:3#vcs_metadata")
	if exec.SourceRef.Command != "git log --oneline -n 3" ||
		exec.SourceRef.RawRef != "/tmp/codrax/blob/exec_command-1.txt" ||
		exec.SourceRef.PayloadRef != "/tmp/codrax/blob/exec_command-1.txt" {
		t.Fatalf("exec_command command/raw ref not projected: %+v", exec.SourceRef)
	}
	trace := findObservationRecord(t, ledger, "tool:4#runtime_artifact")
	if trace.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		trace.SourceRef.Path != "record.systrace" ||
		trace.SourceRef.ArtifactID != "trace_query" ||
		trace.SourceRef.ArtifactKind != "trace" ||
		trace.SourceRef.PayloadRef != "blob://payload/trace-query-result.json" ||
		trace.SourceRef.RawRef != "/tmp/codrax/blob/trace-query-summary.txt" ||
		trace.Span.LineStart != 1102623 ||
		trace.Span.LineEnd != 1139184 ||
		trace.Span.StartTs != 2942.124416 ||
		trace.Span.EndTs != 2942.260210 ||
		ObservationRecordHasCurrentSourceLineSpan(trace) {
		t.Fatalf("trace_query runtime coordinates not projected as external span: %+v", trace)
	}
}

func TestCompileObservationLedger_ProjectsAggregateExternalCoordinates(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{
		{
			Kind:       AnswerAggregateNegativeObservation,
			Label:      "git history no-hit",
			Value:      "0",
			Unit:       "matches",
			Role:       AnswerAggregateRolePrincipalAnswer,
			Provenance: "git_log",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
				{Name: "tool_result", Value: "git_log[0]"},
				{Name: "commit_range", Value: "ref=HEAD count=10"},
				{Name: "pathspec", Value: "internal/tool"},
				{Name: "query", Value: "runTaskGraph"},
				{Name: "payload_ref", Value: "blob://payload/git-log.txt"},
			},
		},
		{
			Kind:       AnswerAggregateNegativeObservation,
			Label:      "attached log no-hit",
			Value:      "0",
			Unit:       "matches",
			Role:       AnswerAggregateRolePrincipalAnswer,
			Provenance: "log_triage",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "artifact_id", Value: "attached_log"},
				{Name: "artifact_kind", Value: "log"},
				{Name: "line_start", Value: "40"},
				{Name: "line_end", Value: "43"},
				{Name: "pattern", Value: "panic"},
				{Name: "payload_ref", Value: "blob://payload/log.txt"},
				{Name: "page_ref", Value: "blob://payload/log.txt?page=2"},
			},
		},
		{
			Kind:       AnswerAggregateScalar,
			Label:      "command output row",
			Value:      "70693",
			Unit:       "lines",
			Role:       AnswerAggregateRolePrincipalAnswer,
			Provenance: "exec_command",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginCommandMeasurement)},
				{Name: "tool_result", Value: "exec_command[2]"},
				{Name: "command", Value: "wc -l"},
				{Name: "row", Value: "1"},
				{Name: "payload_ref", Value: "blob://payload/wc.txt"},
			},
		},
	}})
	git := findObservationRecord(t, ledger, "aggregate:0#vcs_metadata")
	if git.SourceRef.Kind != ObservationSourceVCSMetadata ||
		git.SourceRef.ToolCallID != "git_log[0]" ||
		git.SourceRef.Range != "ref=HEAD count=10" ||
		git.SourceRef.Pathspec != "internal/tool" ||
		git.SourceRef.PayloadRef != "blob://payload/git-log.txt" ||
		git.ClaimKey != "runTaskGraph" ||
		git.ResultCount == nil ||
		*git.ResultCount != 0 {
		t.Fatalf("git aggregate coordinates not projected: %+v", git)
	}
	log := findObservationRecord(t, ledger, "aggregate:1#runtime_artifact")
	if log.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		log.SourceRef.ArtifactID != "attached_log" ||
		log.SourceRef.ArtifactKind != "log" ||
		log.SourceRef.PayloadRef != "blob://payload/log.txt" ||
		log.SourceRef.PageRef != "blob://payload/log.txt?page=2" ||
		log.Span.LineStart != 40 ||
		log.Span.LineEnd != 43 ||
		ObservationRecordHasCurrentSourceLineSpan(log) {
		t.Fatalf("runtime artifact aggregate coordinates not projected as external span: %+v", log)
	}
	cmd := findObservationRecord(t, ledger, "aggregate:2#command_measurement")
	if cmd.SourceRef.Kind != ObservationSourceCommand ||
		cmd.SourceRef.ToolCallID != "exec_command[2]" ||
		cmd.SourceRef.Command != "wc -l" ||
		cmd.Span.Row != 1 ||
		cmd.SourceRef.PayloadRef != "blob://payload/wc.txt" {
		t.Fatalf("command aggregate coordinates not projected: %+v", cmd)
	}
}

func TestCompileObservationLedger_AggregateExternalOriginsPreserveSourceRefsAndLocalSpans(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateScalar,
			Label: "external design paragraph",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginExternalDocument)},
				{Name: "resource_uri", Value: "drive://doc/123"},
				{Name: "page_ref", Value: "page=2"},
				{Name: "paragraph", Value: "7"},
				{Name: "payload_ref", Value: "blob://payload/design-doc.txt"},
				{Name: "mime_type", Value: "text/markdown"},
				{Name: "target", Value: "release-note"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "web API contract",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginWebPage)},
				{Name: "url", Value: "https://example.test/spec"},
				{Name: "fetched_at", Value: "2026-05-23T10:00:00Z"},
				{Name: "selector", Value: "#api-contract"},
				{Name: "text_start", Value: "10"},
				{Name: "text_end", Value: "42"},
				{Name: "payload_ref", Value: "blob://payload/web-page.html"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "MCP resource row",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginMCPResource)},
				{Name: "server", Value: "docs"},
				{Name: "resource_uri", Value: "mcp://docs/spec"},
				{Name: "json_pointer", Value: "/items/0/title"},
				{Name: "row_set_ref", Value: "blob://rows/mcp.jsonl"},
				{Name: "payload_ref", Value: "blob://payload/mcp.json"},
				{Name: "mime_type", Value: "application/json"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "connector issue row",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginConnectorResource)},
				{Name: "connector", Value: "jira"},
				{Name: "resource_uri", Value: "jira://ISSUE-7"},
				{Name: "row", Value: "2"},
				{Name: "payload_ref", Value: "blob://payload/jira.json"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "cross repo index node",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginCrossRepoIndex)},
				{Name: "repo", Value: "tools"},
				{Name: "path", Value: "tools/processor.py"},
				{Name: "payload_ref", Value: "blob://payload/cross-repo-index.json"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "git metadata commit",
			Value: "abc123",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
				{Name: "repo", Value: "."},
				{Name: "commit", Value: "abc123"},
				{Name: "commit_range", Value: "HEAD~10..HEAD"},
				{Name: "pathspec", Value: "internal/tool"},
				{Name: "payload_ref", Value: "blob://payload/git-log.txt"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "git diff hunk",
			Value: "scheduler hook changed",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginVCSDiff)},
				{Name: "repo", Value: "."},
				{Name: "commit", Value: "abc123"},
				{Name: "diff_path", Value: "internal/scheduler.go"},
				{Name: "old_line", Value: "12"},
				{Name: "new_line", Value: "18"},
				{Name: "hunk_header", Value: "@@ -12 +18 @@"},
				{Name: "payload_ref", Value: "blob://payload/git-diff.patch"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "command row",
			Value: "70693",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginCommandMeasurement)},
				{Name: "command", Value: "wc -l"},
				{Name: "row", Value: "1"},
				{Name: "payload_ref", Value: "blob://payload/wc.txt"},
				{Name: "row_set_ref", Value: "blob://rows/wc.jsonl"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "log line",
			Value: "panic",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "artifact_id", Value: "attached_log"},
				{Name: "artifact_kind", Value: "log"},
				{Name: "line_start", Value: "40"},
				{Name: "line_end", Value: "43"},
				{Name: "payload_ref", Value: "blob://payload/log.txt"},
			},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "trace window",
			Value: "120.5ms",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginRuntimeArtifact)},
				{Name: "artifact_id", Value: "attached_trace"},
				{Name: "artifact_kind", Value: "trace"},
				{Name: "start_ts_ms", Value: "120.5"},
				{Name: "end_ts_ms", Value: "168.75"},
				{Name: "payload_ref", Value: "blob://payload/trace.txt"},
			},
		},
	}})

	checks := []struct {
		id     string
		origin AnswerEvidenceOrigin
		source ObservationSourceKind
		check  func(ObservationRecord) bool
	}{
		{"aggregate:0#external_document", AnswerEvidenceOriginExternalDocument, ObservationSourceExternalDocument, func(r ObservationRecord) bool {
			return r.SourceRef.ResourceURI == "drive://doc/123" && r.SourceRef.PageRef == "page=2" && r.SourceRef.MIMEType == "text/markdown" && r.Span.Paragraph == 7
		}},
		{"aggregate:1#web_page", AnswerEvidenceOriginWebPage, ObservationSourceWebPage, func(r ObservationRecord) bool {
			return r.SourceRef.URL == "https://example.test/spec" && r.SourceRef.FetchedAt == "2026-05-23T10:00:00Z" && r.Span.Selector == "#api-contract" && r.Span.TextStart == 10 && r.Span.TextEnd == 42
		}},
		{"aggregate:2#mcp_resource", AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource, func(r ObservationRecord) bool {
			return r.SourceRef.Server == "docs" && r.SourceRef.ResourceURI == "mcp://docs/spec" && r.SourceRef.RowSetRef == "blob://rows/mcp.jsonl" && r.SourceRef.MIMEType == "application/json" && r.Span.JSONPointer == "/items/0/title"
		}},
		{"aggregate:3#connector_resource", AnswerEvidenceOriginConnectorResource, ObservationSourceConnector, func(r ObservationRecord) bool {
			return r.SourceRef.Connector == "jira" && r.SourceRef.ResourceURI == "jira://ISSUE-7" && r.Span.Row == 2
		}},
		{"aggregate:4#cross_repo_index", AnswerEvidenceOriginCrossRepoIndex, ObservationSourceCrossRepoIndex, func(r ObservationRecord) bool {
			return r.SourceRef.Repo == "tools" && r.SourceRef.Path == "tools/processor.py" && r.SourceRef.PayloadRef == "blob://payload/cross-repo-index.json"
		}},
		{"aggregate:5#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata, func(r ObservationRecord) bool {
			return r.SourceRef.Commit == "abc123" && r.SourceRef.Range == "HEAD~10..HEAD" && r.SourceRef.Pathspec == "internal/tool"
		}},
		{"aggregate:6#vcs_diff", AnswerEvidenceOriginVCSDiff, ObservationSourceVCSDiff, func(r ObservationRecord) bool {
			return r.SourceRef.Pathspec == "internal/scheduler.go" && r.Span.OldLine == 12 && r.Span.NewLine == 18 && r.Span.HunkHeader == "@@ -12 +18 @@"
		}},
		{"aggregate:7#command_measurement", AnswerEvidenceOriginCommandMeasurement, ObservationSourceCommand, func(r ObservationRecord) bool {
			return r.SourceRef.Command == "wc -l" && r.SourceRef.RowSetRef == "blob://rows/wc.jsonl" && r.Span.Row == 1
		}},
		{"aggregate:8#runtime_artifact", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact, func(r ObservationRecord) bool {
			return r.SourceRef.ArtifactID == "attached_log" && r.SourceRef.ArtifactKind == "log" && r.Span.LineStart == 40 && r.Span.LineEnd == 43
		}},
		{"aggregate:9#runtime_artifact", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact, func(r ObservationRecord) bool {
			return r.SourceRef.ArtifactID == "attached_trace" && r.SourceRef.ArtifactKind == "trace" && r.Span.StartTsMs == 120.5 && r.Span.EndTsMs == 168.75
		}},
	}
	for _, tc := range checks {
		record := findObservationRecord(t, ledger, tc.id)
		if record.Origin != tc.origin || record.SourceRef.Kind != tc.source {
			t.Fatalf("%s origin/source drifted: %+v", tc.id, record)
		}
		if !tc.check(record) {
			t.Fatalf("%s lost origin-local source/span details: %+v", tc.id, record)
		}
		if tc.origin != AnswerEvidenceOriginCurrentSource &&
			(ObservationRecordHasCurrentSourceLineSpan(record) || ObservationRecordHasStrongCurrentSourceAnchor(record)) {
			t.Fatalf("%s external observation became current-source citation-eligible: %+v", tc.id, record)
		}
		if !AnswerEvidenceOriginCarriesOriginSpecificSupport(tc.origin) {
			t.Fatalf("%s origin should be origin-specific support: %s", tc.id, tc.origin)
		}
	}
}

func TestFormatObservationSourceRef_LabelsExternalPayloadRefs(t *testing.T) {
	got := FormatObservationSourceRef(ObservationSourceRef{
		Kind:       ObservationSourceCommand,
		Command:    "git log --oneline -n 10",
		ToolCallID: "exec_command[0]",
		RawRef:     "blob://payload/git-log-full.txt",
		PayloadRef: "blob://payload/git-log-full.txt",
		RowSetRef:  "blob://rows/git-log-rows.jsonl",
		PageRef:    "blob://payload/git-log-full.txt?page=1",
		FetchedAt:  "2026-05-23T10:00:00Z",
		MIMEType:   "text/plain",
	}, 90)
	for _, want := range []string{
		"kind=command",
		"command=git log --oneline -n 10",
		"tool_call_id=exec_command[0]",
		"payload_ref=blob://payload/git-log-full.txt",
		"row_set_ref=blob://rows/git-log-rows.jsonl",
		"page_ref=blob://payload/git-log-full.txt?page=1",
		"fetched_at=2026-05-23T10:00:00Z",
		"mime_type=text/plain",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted source ref missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "raw_ref=") {
		t.Fatalf("raw_ref should not duplicate payload_ref in formatted source: %s", got)
	}
}

func TestPrioritizeObservationRecords_MixedHistoryAndCurrentCodeKeepsExactSourceFirst(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:      "tool:0#vcs_metadata",
			Origin:  AnswerEvidenceOriginVCSMetadata,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "commit abc123 changed scheduler behavior",
		},
		{
			ID:     "evidence:current",
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: "internal/scheduler.go",
			},
			Span:            ObservationSpan{LineStart: 42},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "current scheduler entrypoint still exists",
		},
	}
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		ChangeImpactProfile: &ChangeImpactProfile{IsChangeImpact: true},
	}
	got := PrioritizeObservationRecords(records, &rm, nil, 2)
	if len(got) != 2 || got[0].ID != "evidence:current" || got[1].ID != "tool:0#vcs_metadata" {
		t.Fatalf("mixed history+current-code ranking should keep exact current source first, got %+v", got)
	}
}

func TestPrioritizeObservationRecords_MixedHistoryBudgetPreservesEachRequestedOrigin(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:      "tool:0#vcs_metadata",
			Origin:  AnswerEvidenceOriginVCSMetadata,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "latest commit explains the feature",
		},
		{
			ID:      "tool:1#vcs_diff",
			Origin:  AnswerEvidenceOriginVCSDiff,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "diff hunk shows the historical change",
		},
	}
	for i := 0; i < 6; i++ {
		records = append(records, ObservationRecord{
			ID:     fmt.Sprintf("evidence:current:%d", i),
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: fmt.Sprintf("internal/current_%d.go", i),
			},
			Span:            ObservationSpan{LineStart: 10 + i},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "current source detail",
		})
	}
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		ChangeImpactProfile: &ChangeImpactProfile{IsChangeImpact: true},
	}
	got := PrioritizeObservationRecords(records, &rm, nil, 3)
	if len(got) != 3 {
		t.Fatalf("got %d records, want 3: %+v", len(got), got)
	}
	seen := map[AnswerEvidenceOrigin]bool{}
	for _, record := range got {
		seen[record.Origin] = true
	}
	for _, want := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginCurrentSource,
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
	} {
		if !seen[want] {
			t.Fatalf("mixed-origin budget dropped requested origin %s: %+v", want, got)
		}
	}
	if got[0].Origin != AnswerEvidenceOriginCurrentSource {
		t.Fatalf("exact current-source evidence should remain the first mixed-origin record, got %+v", got)
	}
}

func TestPrioritizeObservationRecords_ExternalOnlyHistoryDoesNotLetIncidentalSourceDominate(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:     "evidence:incidental",
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRolePrincipalAnswer,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: "internal/noise.go",
			},
			Span:            ObservationSpan{LineStart: 7},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "incidental source read",
		},
		{
			ID:      "tool:0#vcs_metadata",
			Origin:  AnswerEvidenceOriginVCSMetadata,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "latest commit added feature",
		},
	}
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
	}
	got := PrioritizeObservationRecords(records, &rm, nil, 2)
	if len(got) != 2 || got[0].ID != "tool:0#vcs_metadata" {
		t.Fatalf("external-only history ranking should prefer requested VCS observation, got %+v", got)
	}
}

func TestPrioritizeObservationRecords_PrincipalAggregateSurvivesSourceEvidenceBudget(t *testing.T) {
	records := []ObservationRecord{{
		ID:      "aggregate:0#current_source",
		Origin:  AnswerEvidenceOriginCurrentSource,
		Role:    AnswerAggregateRolePrincipalAnswer,
		Summary: "headers handed off by exploration",
		RichNotes: []string{
			"X-Auth-Token comes from loadW3Token",
			"app-id is CodeAgent2.0",
		},
		SupportRefs: []string{"X-Auth-Token @ api.ts:10"},
	}}
	for i := 0; i < 24; i++ {
		records = append(records, ObservationRecord{
			ID:     fmt.Sprintf("evidence:current:%d", i),
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: fmt.Sprintf("internal/current_%d.go", i),
			},
			Span:            ObservationSpan{LineStart: 10 + i},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "current source detail",
		})
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	got := PrioritizeObservationRecords(records, &rm, nil, 4)
	foundAggregate := false
	for _, record := range got {
		if record.ID == "aggregate:0#current_source" {
			foundAggregate = true
			break
		}
	}
	if !foundAggregate {
		t.Fatalf("principal aggregate handoff should survive a tight source-evidence budget, got %+v", got)
	}
}

func TestObservationLedgerInputFromContexts_PrefersAcceptedTurnAToolResults(t *testing.T) {
	mut := NewMutableState("history")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		ToolResults: []ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata]\naccepted turn-a result",
		}},
		EvidenceItems: []EvidenceItem{{
			ID:        "turn-a",
			Source:    "a.go",
			LineStart: 3,
			Summary:   "accepted evidence",
		}},
		SourceInventoryObservation: SourceInventoryObservation{
			Active: true,
			Sets: []SourceInventoryObservationSet{{
				Role:  AnswerCandidateRoleFile,
				Count: 1,
				Members: []SourceInventoryObservationMember{{
					Name:       "a.go",
					Key:        "a.go",
					SupportRef: "a.go",
					Role:       AnswerCandidateRoleFile,
					File:       "a.go",
				}},
			}},
		},
	})
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateScalar,
		Label: "latest feature",
		Value: "cache reuse",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	bus := &BusContext{
		Mutable: mut,
		ToolResults: []ToolResult{{
			ToolName: "grep",
			Success:  true,
			Summary:  "[grep: evidence_origin=current_source]\npre-scan noise",
		}},
	}

	input := ObservationLedgerInputFromBusContext(bus, 64)
	if len(input.ToolResults) != 1 || input.ToolResults[0].ToolName != "git_log" {
		t.Fatalf("bus input should prefer accepted Turn A tool results over full bus history: %+v", input.ToolResults)
	}
	if !input.SourceInventoryObservation.IsActive() {
		t.Fatalf("bus input should carry source inventory observation: %+v", input.SourceInventoryObservation)
	}
	ledger := CompileObservationLedger(input)
	assertObservationRecord(t, ledger, "tool:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "aggregate:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "evidence:turn-a", AnswerEvidenceOriginCurrentSource, ObservationSourceCurrentSource)
	assertObservationRecord(t, ledger, "source_inventory:0:0", AnswerEvidenceOriginCurrentSource, ObservationSourceCurrentSource)
}

func TestObservationLedgerInputFromContexts_MergesTurnAAcceptedAggregateFacts(t *testing.T) {
	mut := NewMutableState("headers")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		AcceptedAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "model headers",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"X-Auth-Token", "app-id"},
			MemberNotes: []string{
				"X-Auth-Token comes from loadW3Token",
				"app-id is fixed to CodeAgent2.0",
			},
			SupportRefs: []string{
				"X-Auth-Token @ api.ts:10",
				"app-id @ api.ts:12",
			},
		}},
	})
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	for name, input := range map[string]ObservationLedgerInput{
		"agent": ObservationLedgerInputFromAgentContext(&AgentContext{
			Mutable:    mut,
			AnalysisIR: &AnalysisIR{RequestModel: rm},
		}, 64),
		"bus": ObservationLedgerInputFromBusContext(&BusContext{
			Mutable:    mut,
			AnalysisIR: &AnalysisIR{RequestModel: rm},
		}, 64),
	} {
		ledger := CompileObservationLedger(input)
		record := findObservationRecord(t, ledger, "aggregate:0#current_source")
		if record.Role != AnswerAggregateRolePrincipalAnswer {
			t.Fatalf("%s aggregate role = %q, want principal: %+v", name, record.Role, record)
		}
		notes := strings.Join(record.RichNotes, "\n")
		if !strings.Contains(notes, "loadW3Token") || !strings.Contains(notes, "CodeAgent2.0") {
			t.Fatalf("%s aggregate rich notes lost TurnA member details: %+v", name, record.RichNotes)
		}
		if len(record.SupportRefs) != 2 {
			t.Fatalf("%s aggregate support refs lost: %+v", name, record.SupportRefs)
		}
	}
}

func TestObservationLedgerInputFromAgentContext_CarriesMCPAndRuntimeBundles(t *testing.T) {
	bundle := &LogBundle{Observations: []LogObservation{{
		Kind:      LogObservationRuntimeEvent,
		Subject:   "runtime event",
		Summary:   "runtime event observed",
		LineStart: 8,
	}}}
	ctx := &AgentContext{
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{LogTriage: bundle}},
		MCPResponses: []MCPResponse{{
			ServerName: "docs",
			Method:     "read_resource",
			Success:    true,
			Summary:    "external doc note",
			RawRef:     "mcp://docs/note",
		}},
	}

	ledger := CompileObservationLedger(ObservationLedgerInputFromAgentContext(ctx, 64))
	assertObservationRecord(t, ledger, "log:observation:0", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact)
	assertObservationRecord(t, ledger, "mcp:0", AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource)
	if got := findObservationRecord(t, ledger, "mcp:0"); got.SourceRef.RawRef != "mcp://docs/note" {
		t.Fatalf("MCP raw ref should survive context input compilation: %+v", got)
	}
}

func TestCompileObservationLedger_MCPResponseTypedCoordinates(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName:  "docs",
			Method:      "read_resource",
			Success:     true,
			Summary:     "spec row confirms the contract",
			RawRef:      "mcp://docs/spec#raw",
			PayloadRef:  "blob://payload/mcp-spec.json",
			RowSetRef:   "blob://rows/mcp-spec.jsonl",
			PageRef:     "blob://payload/mcp-spec.json?page=1",
			ResourceURI: "mcp://docs/spec",
			MIMEType:    "application/json",
			JSONPointer: "/items/0/title",
			Selector:    "$.items[0]",
			Row:         3,
			LineStart:   12,
			LineEnd:     13,
		}},
	})
	got := findObservationRecord(t, ledger, "mcp:0")
	if got.SourceRef.Kind != ObservationSourceMCPResource ||
		got.SourceRef.Server != "docs" ||
		got.SourceRef.RawRef != "mcp://docs/spec#raw" ||
		got.SourceRef.PayloadRef != "blob://payload/mcp-spec.json" ||
		got.SourceRef.RowSetRef != "blob://rows/mcp-spec.jsonl" ||
		got.SourceRef.PageRef != "blob://payload/mcp-spec.json?page=1" ||
		got.SourceRef.ResourceURI != "mcp://docs/spec" ||
		got.SourceRef.MIMEType != "application/json" ||
		got.Span.JSONPointer != "/items/0/title" ||
		got.Span.Selector != "$.items[0]" ||
		got.Span.Row != 3 ||
		got.Span.LineStart != 12 ||
		got.Span.LineEnd != 13 {
		t.Fatalf("MCP typed coordinates were not preserved: %+v", got)
	}
	if ObservationRecordHasCurrentSourceLineSpan(got) || ObservationRecordHasStrongCurrentSourceAnchor(got) {
		t.Fatalf("MCP typed coordinates must not become current-source citation anchors: %+v", got)
	}
}

func TestCompileObservationLedger_MCPResponseTypedObservationRows(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName:  "trace",
			Method:      "tools/call:wakeup_chain",
			Success:     true,
			Summary:     "parent summary",
			PayloadRef:  "blob://payload/full.json",
			ResourceURI: "mcp://trace/run",
			MIMEType:    "application/vnd.codrax.observation+json",
			Observations: []MCPTypedObservation{{
				Summary:   "sleep entry",
				LineStart: 1102717,
			}, {
				Summary:     "wakeup",
				ResourceURI: "mcp://trace/override",
				LineStart:   1139180,
				LineEnd:     1139180,
				Selector:    "pid=36379",
			}},
		}},
	})
	first := findObservationRecord(t, ledger, "mcp:0:0")
	if first.SourceRef.Kind != ObservationSourceMCPResource ||
		first.SourceRef.ResourceURI != "mcp://trace/run" ||
		first.SourceRef.PayloadRef != "blob://payload/full.json" ||
		first.Span.LineStart != 1102717 ||
		first.Summary != "sleep entry" {
		t.Fatalf("first typed MCP row did not inherit parent coordinates: %+v", first)
	}
	second := findObservationRecord(t, ledger, "mcp:0:1")
	if second.SourceRef.ResourceURI != "mcp://trace/override" ||
		second.Span.LineStart != 1139180 ||
		second.Span.LineEnd != 1139180 ||
		second.Span.Selector != "pid=36379" {
		t.Fatalf("second typed MCP row did not preserve row coordinates: %+v", second)
	}
	if ObservationRecordHasCurrentSourceLineSpan(first) || ObservationRecordHasStrongCurrentSourceAnchor(second) {
		t.Fatalf("typed MCP rows must remain external observations: first=%+v second=%+v", first, second)
	}
}

func assertObservationRecord(t *testing.T, ledger ObservationLedger, id string, origin AnswerEvidenceOrigin, source ObservationSourceKind) {
	t.Helper()
	record := findObservationRecord(t, ledger, id)
	if record.Origin != origin {
		t.Fatalf("%s origin = %q, want %q", id, record.Origin, origin)
	}
	if record.SourceRef.Kind != source {
		t.Fatalf("%s source kind = %q, want %q", id, record.SourceRef.Kind, source)
	}
}

func findObservationRecord(t *testing.T, ledger ObservationLedger, id string) ObservationRecord {
	t.Helper()
	for _, record := range ledger.Records {
		if record.ID == id {
			return record
		}
	}
	t.Fatalf("record %q not found in %+v", id, ledger.Records)
	return ObservationRecord{}
}

func containsObservationString(values []string, want string) bool {
	want = strings.Join(strings.Fields(strings.TrimSpace(want)), " ")
	for _, value := range values {
		if strings.Join(strings.Fields(strings.TrimSpace(value)), " ") == want {
			return true
		}
	}
	return false
}
