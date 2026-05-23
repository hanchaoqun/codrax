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
	if len(input.EvidenceItems) != 1 || input.EvidenceItems[0].Summary != "source fact" {
		t.Fatalf("compiler mutated input: %+v", input)
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
	context := findObservationRecord(t, ledger, "log:observation:1")
	if context.Role != AnswerAggregateRoleSupportingCoverage || context.GroundingPolicy != ClaimGroundingSoft {
		t.Fatalf("non-diagnostic info observation in an external error log should be support-only, got %+v", context)
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
	if logRecord.Span.LineStart != 40 || logRecord.Span.LineEnd != 43 {
		t.Fatalf("log observation should preserve artifact-local lines: %+v", logRecord)
	}
	perfRecord := findObservationRecord(t, ledger, "perf:observation:0")
	if perfRecord.Span.LineStart != 9 || perfRecord.Span.StartTsMs != 120.5 || perfRecord.Span.EndTsMs != 168.75 {
		t.Fatalf("perf observation should preserve artifact-local line/time spans: %+v", perfRecord)
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
			ToolName: "exec_command",
			Success:  true,
			Summary:  "[exec_command: $ git log --oneline -n 3]\n[exec_command: evidence_origin=vcs_metadata]\nabc123 feature\n",
			RawRef:   "/tmp/codrax/blob/exec_command-1.txt",
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
	exec := findObservationRecord(t, ledger, "tool:2#vcs_metadata")
	if exec.SourceRef.Command != "git log --oneline -n 3" ||
		exec.SourceRef.RawRef != "/tmp/codrax/blob/exec_command-1.txt" ||
		exec.SourceRef.PayloadRef != "/tmp/codrax/blob/exec_command-1.txt" {
		t.Fatalf("exec_command command/raw ref not projected: %+v", exec.SourceRef)
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
	ledger := CompileObservationLedger(input)
	assertObservationRecord(t, ledger, "tool:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "aggregate:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "evidence:turn-a", AnswerEvidenceOriginCurrentSource, ObservationSourceCurrentSource)
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
