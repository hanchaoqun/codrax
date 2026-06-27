package types

import (
	"strings"
	"testing"
)

func TestAssessExternalObservationSufficiency_MCPLineRowsSufficientWhenSourceOptional(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call:lookup_trace_fact",
			Success:     true,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			MIMEType:    "application/vnd.codrax.observation+json",
			Observations: []MCPTypedObservation{
				{Summary: "target sleeps", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, LineEnd: 7, Selector: "pid=4242 event=sched_switch"},
				{Summary: "helper wakes target", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12, LineEnd: 12, Selector: "pid=4242 event=sched_wakeup waker=helper"},
			},
		}},
	})
	got := AssessExternalObservationSufficiency(ledger.Records, nil, TurnRouteHint{
		Route:  "repo",
		Source: "external_tool",
	})
	if !got.Status.Sufficient() {
		t.Fatalf("typed MCP rows should be sufficient when source is optional, got %+v", got)
	}
	if got.RecordCount != 2 || got.AddressableCount != 2 {
		t.Fatalf("counts = %+v, want two addressable rows", got)
	}
	if len(got.Origins) != 1 || got.Origins[0] != AnswerEvidenceOriginMCPResource {
		t.Fatalf("origins = %+v, want MCP", got.Origins)
	}
}

func TestAssessExternalObservationSufficiency_BlockedByCurrentSourceProfile(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call:lookup_trace_fact",
			Success:     true,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Observations: []MCPTypedObservation{{
				Summary:   "helper wakes target",
				LineStart: 12,
				LineEnd:   12,
				Selector:  "waker=helper",
			}},
		}},
	})
	rm := &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []CurrentSourceExplanationMode{
				CurrentSourceExplanationExplainCurrentMechanism,
			},
			SourceQuotes: []string{"current checkout"},
			Confidence:   0.9,
		},
	}
	got := AssessExternalObservationSufficiency(ledger.Records, rm, TurnRouteHint{
		Route:  "repo",
		Source: "external_tool",
	})
	if got.Status != ExternalObservationSufficiencyBlockedByCurrentSource {
		t.Fatalf("current-source profile must block external-only sufficiency, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_BlockedByCurrentKeyCodeDimensionWithArtifactCitationPolicy(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		LogBundle: &LogBundle{
			Observations: []LogObservation{{
				Kind:       LogObservationRuntimeEvent,
				Subject:    "finalizer",
				Summary:    "attached log observed a finalizer retry",
				LineStart:  1,
				LineEnd:    4,
				Confidence: 0.95,
			}},
		},
	})
	rm := &RequestModel{
		LogTriage: &LogBundle{
			Observations: []LogObservation{{
				Kind:       LogObservationRuntimeEvent,
				Subject:    "finalizer",
				Summary:    "attached log observed a finalizer retry",
				LineStart:  1,
				LineEnd:    4,
				Confidence: 0.95,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"do not cite artifact lines as source"},
			Confidence:           0.9,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "current key code",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}
	got := AssessExternalObservationSufficiency(ledger.Records, rm, TurnRouteHint{
		Route:  "repo",
		Source: "artifact",
	})
	if got.Status != ExternalObservationSufficiencyBlockedByCurrentSource {
		t.Fatalf("current_key_code dimension must block external-only sufficiency even with artifact-only citation policy, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_BlockedByTypedSourceScope(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		PerfBundle: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		},
	})
	rm := &RequestModel{
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceAllow,
			Confidence:           0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
	}
	got := AssessExternalObservationSufficiency(ledger.Records, rm, TurnRouteHint{
		Route:  "repo",
		Source: "artifact",
	})
	if got.Status != ExternalObservationSufficiencyBlockedByCurrentSource {
		t.Fatalf("typed source scope must block external-only sufficiency, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_BlockedBySourceScopeQuote(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		LogBundle: &LogBundle{
			Observations: []LogObservation{{
				Kind:      LogObservationRetryCycle,
				Subject:   "finalizer timeout",
				Summary:   "runtime log shows retry after timeout",
				LineStart: 2,
			}},
		},
	})
	rm := &RequestModel{
		LogTriage: &LogBundle{
			Observations: []LogObservation{{
				Kind:      LogObservationRetryCycle,
				Subject:   "finalizer timeout",
				Summary:   "runtime log shows retry after timeout",
				LineStart: 2,
			}},
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeAll,
			SourceQuotes:   []string{"请结合当前源码解释"},
			Confidence:     0.8,
		},
	}
	got := AssessExternalObservationSufficiency(ledger.Records, rm, TurnRouteHint{
		Route:           "repo",
		Source:          "artifact",
		NeedsRepoAccess: true,
	})
	if got.Status != ExternalObservationSufficiencyBlockedByCurrentSource {
		t.Fatalf("runtime artifact plus source-scope quote must block external-only sufficiency, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_RouteHintOverridesDefaultSourceRequirement(t *testing.T) {
	records := []ObservationRecord{{
		ID:     "row",
		Origin: AnswerEvidenceOriginMCPResource,
		SourceRef: ObservationSourceRef{
			Kind:        ObservationSourceMCPResource,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
		},
		Span:    ObservationSpan{LineStart: 7, LineEnd: 7},
		Summary: "typed external line fact",
	}}
	got := AssessExternalObservationSufficiency(records, &RequestModel{}, TurnRouteHint{
		Route:      "repo",
		Source:     "external_tool",
		Confidence: 0.8,
	})
	if !got.Status.Sufficient() {
		t.Fatalf("typed external route should make default source lane optional, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_RouteBackedRepoArtifactBlocksRuntimeOnlyCompletion(t *testing.T) {
	records := []ObservationRecord{{
		ID:     "log:error:0",
		Origin: AnswerEvidenceOriginRuntimeArtifact,
		SourceRef: ObservationSourceRef{
			Kind:       ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_log",
		},
		Span:    ObservationSpan{LineStart: 2, LineEnd: 3},
		Summary: "first_byte_timeout exceeded after 40s",
	}}
	got := AssessExternalObservationSufficiency(records, &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
	}, TurnRouteHint{
		Route:           "repo",
		Source:          "artifact",
		NeedsRepoAccess: true,
		Confidence:      0.9,
	})
	if got.Status != ExternalObservationSufficiencyBlockedByCurrentSource {
		t.Fatalf("route-backed repo artifact should block runtime-only sufficiency, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_ExplicitSourceExclusionOverridesRouteBackedRepoArtifact(t *testing.T) {
	records := []ObservationRecord{{
		ID:     "log:error:0",
		Origin: AnswerEvidenceOriginRuntimeArtifact,
		SourceRef: ObservationSourceRef{
			Kind:       ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_log",
		},
		Span:    ObservationSpan{LineStart: 2, LineEnd: 3},
		Summary: "first_byte_timeout exceeded after 40s",
	}}
	got := AssessExternalObservationSufficiency(records, &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"只分析日志"},
			Confidence:           0.9,
		},
	}, TurnRouteHint{
		Route:           "repo",
		Source:          "artifact",
		NeedsRepoAccess: true,
		Confidence:      0.9,
	})
	if !got.Status.Sufficient() {
		t.Fatalf("explicit current-source exclusion should preserve runtime-only sufficiency, got %+v", got)
	}
}

func TestAssessExternalObservationSufficiency_TraceQueryRuntimeRecordSufficient(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Summary: strings.Join([]string{
				"[trace_query params: view=root_cause_rank source=attached_trace path=/tmp/a.systrace origin=runtime_artifact artifact_id=attached_trace artifact_kind=trace payload_ref=/tmp/trace-query.json]",
				"## Root cause rank",
				"- rank=1 tier=primary type=scheduler_latency thread=com.app-42 impact=107.900ms score=86.320 confidence=0.88 lines=110-120 source=scheduler_latency_stats — runnable wait dominated the frame window",
			}, "\n"),
		}},
	})
	got := AssessExternalObservationSufficiency(ledger.Records, &RequestModel{}, TurnRouteHint{
		Route:      "repo",
		Source:     "external_tool",
		Confidence: 0.8,
	})
	if !got.Status.Sufficient() {
		t.Fatalf("small trace_query runtime records should be sufficient under external-observation-first route, got %+v", got)
	}
	if len(got.Origins) != 1 || got.Origins[0] != AnswerEvidenceOriginRuntimeArtifact {
		t.Fatalf("origins = %+v, want runtime artifact", got.Origins)
	}
}

func TestAssessExternalObservationSufficiency_BroadExternalRowsNotDirectlySufficient(t *testing.T) {
	var records []ObservationRecord
	for i := 0; i < externalObservationSufficiencyMaxDirectRecords+1; i++ {
		records = append(records, ObservationRecord{
			ID:     "row",
			Origin: AnswerEvidenceOriginMCPResource,
			SourceRef: ObservationSourceRef{
				Kind:        ObservationSourceMCPResource,
				ResourceURI: "mcp://fixture/large",
			},
			Span:    ObservationSpan{LineStart: i + 1, LineEnd: i + 1},
			Summary: "typed row",
		})
	}
	got := AssessExternalObservationSufficiency(records, nil, TurnRouteHint{
		Route:  "repo",
		Source: "external_tool",
	})
	if got.Status.Sufficient() {
		t.Fatalf("broad external row set should continue normal investigation, got %+v", got)
	}
	if got.RecordCount != externalObservationSufficiencyMaxDirectRecords+1 {
		t.Fatalf("record_count=%d, want broad count", got.RecordCount)
	}
}

func TestAssessExternalObservationSufficiency_NotEligibleForPlainSourceTurn(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName: "fixture",
			Method:     "tools/call",
			Success:    true,
			Observations: []MCPTypedObservation{{
				Summary:   "helper wakes target",
				LineStart: 12,
				LineEnd:   12,
				Selector:  "waker=helper",
			}},
		}},
	})
	got := AssessExternalObservationSufficiency(ledger.Records, nil, TurnRouteHint{
		Route:  "repo",
		Source: "repo",
	})
	if got.Status.Sufficient() {
		t.Fatalf("plain repo turn must not be satisfied by incidental external rows, got %+v", got)
	}
}
