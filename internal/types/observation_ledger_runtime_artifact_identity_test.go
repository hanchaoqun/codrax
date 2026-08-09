package types

import "testing"

func TestCompileObservationLedger_PreflightAttachmentInsideRepoKeepsRuntimeOriginAcrossProducers(t *testing.T) {
	const attached = "eval/fixtures/oversized_log.txt"
	input := ObservationLedgerInput{
		RepoRoot: "/repo",
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind:    "log",
				Source:  "/repo/" + attached,
				Carrier: "attachment",
			}},
		}),
		EvidenceItems: []EvidenceItem{{
			ID:              "stack-top",
			Source:          attached,
			LineStart:       643,
			LineEnd:         643,
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			GroundingStatus: GroundingRecovered,
			Summary:         "observed stack top",
		}},
		ToolResults: []ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				ReadCoverage: &ToolReadCoverage{
					Path:      attached,
					LineStart: 636,
					LineEnd:   647,
				},
			},
			{
				ToolName: "grep",
				Success:  true,
				PathDiscovery: &ToolPathDiscovery{
					Kind:        "grep",
					Path:        "/repo/" + attached,
					ResultCount: 2,
				},
			},
		},
	}

	ledger := CompileObservationLedger(input)
	if len(ledger.Records) != 3 {
		t.Fatalf("records=%d, want 3: %+v", len(ledger.Records), ledger.Records)
	}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact {
			t.Fatalf("%s origin=%q, want runtime_artifact", record.ID, record.Origin)
		}
		if record.SourceRef.Kind != ObservationSourceRuntimeArtifact {
			t.Fatalf("%s source kind=%q, want runtime_artifact", record.ID, record.SourceRef.Kind)
		}
		if record.SourceRef.ArtifactKind != "log" || record.SourceRef.ArtifactID == "" {
			t.Fatalf("%s missing artifact identity: %+v", record.ID, record.SourceRef)
		}
		if ObservationRecordHasCurrentSourceLineSpan(record) {
			t.Fatalf("%s must not qualify as current-source citation", record.ID)
		}
	}
}

func TestCompileObservationLedger_PreflightIdentityDoesNotTaintNeighboringRepoFiles(t *testing.T) {
	const attached = "eval/fixtures/customer.txt"
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RepoRoot: "/repo",
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind:   "log",
				Source: attached,
			}},
		}),
		ToolResults: []ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				ReadCoverage: &ToolReadCoverage{
					Path:      "eval/fixtures/customer.txt",
					LineStart: 1,
					LineEnd:   2,
				},
			},
			{
				ToolName: "read_file",
				Success:  true,
				ReadCoverage: &ToolReadCoverage{
					Path:      "eval/fixtures/neighbor.txt",
					LineStart: 1,
					LineEnd:   2,
				},
			},
		},
	})
	if len(ledger.Records) != 2 {
		t.Fatalf("records=%d, want 2: %+v", len(ledger.Records), ledger.Records)
	}
	if ledger.Records[0].Origin != AnswerEvidenceOriginRuntimeArtifact {
		t.Fatalf("attached origin=%q, want runtime_artifact", ledger.Records[0].Origin)
	}
	if ledger.Records[1].Origin != AnswerEvidenceOriginCurrentSource {
		t.Fatalf("neighbor origin=%q, want current_source", ledger.Records[1].Origin)
	}
}

func TestCompileObservationLedger_NoPreflightKeepsLegacyTxtSourceOrigin(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "read_file",
			Success:  true,
			ReadCoverage: &ToolReadCoverage{
				Path:      "notes/customer.txt",
				LineStart: 1,
				LineEnd:   1,
			},
		}},
	})
	if len(ledger.Records) != 1 || ledger.Records[0].Origin != AnswerEvidenceOriginCurrentSource {
		t.Fatalf("legacy record changed without typed preflight: %+v", ledger.Records)
	}
}

func TestCompileObservationLedger_AttachedTraceBlobSharesTypedCaptureIdentity(t *testing.T) {
	const original = "/repo/eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	originalResult := typedTraceToolResult("trace_query:original#root_cause_rank:1")
	originalResult.Observations[0].SourceRef.Path = original
	originalResult.Observations[0].SourceRef.ArtifactID = "trace_query"
	originalResult.Observations[0].SupportRefs = []string{original + ":10-20"}

	blobResult := typedTraceToolResult("trace_query:supplement#root_cause_rank:1")
	blobPath := "/repo/.codrax/blob/session/attached_trace.txt"
	blobResult.Observations[0].SourceRef.Path = blobPath
	blobResult.Observations[0].SourceRef.ArtifactID = "attached_trace"
	blobResult.Observations[0].SourceRef.PayloadRef = "/blobs/supplement.json"
	blobResult.Observations[0].SourceRef.RawRef = "/blobs/supplement.txt"
	blobResult.Observations[0].Span = ObservationSpan{LineStart: 11, LineEnd: 21}
	blobResult.Observations[0].SupportRefs = []string{blobPath + ":11-21"}

	ledger := CompileObservationLedger(ObservationLedgerInput{
		RepoRoot: "/repo",
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{
				// Request-path discovery is populated before attachment discovery
				// in production. The later attachment carrier must still establish
				// the materialization relation for the same source.
				{Kind: "trace", Source: original, Carrier: "request_path"},
				{Kind: "trace", Source: original, Carrier: "attachment"},
			},
		}),
		ToolResults: []ToolResult{originalResult, blobResult},
	})
	if len(ledger.Records) != 2 {
		t.Fatalf("records=%d, want both independently addressable observations: %+v", len(ledger.Records), ledger.Records)
	}
	wantIdentity := "eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	for _, record := range ledger.Records {
		if got := record.SourceRef.CaptureIdentityPath; got != wantIdentity {
			t.Fatalf("%s capture identity=%q, want %q", record.ID, got, wantIdentity)
		}
	}
	if ledger.Records[0].SourceRef.Path != original || ledger.Records[0].Span.LineStart != 10 {
		t.Fatalf("original locator changed: %+v", ledger.Records[0])
	}
	if ledger.Records[1].SourceRef.Path != blobPath || ledger.Records[1].Span.LineStart != 11 {
		t.Fatalf("materialized locator must stay exact instead of inheriting the original file's coordinate system: %+v", ledger.Records[1])
	}
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 || set.Projections[0].ArtifactLabel != "donghu_tieba_frame.systrace" {
		t.Fatalf("one attached capture must compile one projection: %+v", set)
	}
	if relation := BuildRuntimeArtifactPairRelationAuthority(ledger); relation.Active {
		t.Fatalf("one attached capture must not mint cross-artifact authority: %+v", relation)
	}
}

func TestCompileObservationLedger_AttachedTraceAliasFailsOpenWithoutUniqueTypedBinding(t *testing.T) {
	newBlobResult := func(id, artifactID string) ToolResult {
		result := typedTraceToolResult(id)
		result.Observations[0].SourceRef.Path = "/repo/.codrax/blob/session/attached_trace.txt"
		result.Observations[0].SourceRef.ArtifactID = artifactID
		return result
	}
	cases := []struct {
		name       string
		preflight  RuntimeArtifactPreflightProfile
		artifactID string
	}{
		{
			name: "ordinary user path with producer id",
			preflight: RuntimeArtifactPreflightProfile{Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "/repo/original.systrace", Carrier: "attachment",
			}}},
			artifactID: "trace_query",
		},
		{
			name: "multiple attachment candidates",
			preflight: RuntimeArtifactPreflightProfile{Artifacts: []RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "/repo/a.systrace", Carrier: "attachment"},
				{Kind: "trace", Source: "/repo/b.systrace", Carrier: "attachment"},
			}},
			artifactID: "attached_trace",
		},
		{
			name: "inline attachment",
			preflight: RuntimeArtifactPreflightProfile{Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "(inline)", Carrier: "attachment",
			}}},
			artifactID: "attached_trace",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := CompileObservationLedger(ObservationLedgerInput{
				RepoRoot:                 "/repo",
				RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(tc.preflight),
				ToolResults:              []ToolResult{newBlobResult("trace_query:"+tc.name, tc.artifactID)},
			})
			if len(ledger.Records) != 1 {
				t.Fatalf("records=%d: %+v", len(ledger.Records), ledger.Records)
			}
			if got := ledger.Records[0].SourceRef.CaptureIdentityPath; got != "" {
				t.Fatalf("ambiguous/untyped alias must fail open, got capture identity %q", got)
			}
		})
	}
}

func TestCompileObservationLedger_PathlessPerfRowsBindOnlyToUniqueAttachedTrace(t *testing.T) {
	const attached = "/repo/customer.systrace"
	input := ObservationLedgerInput{
		RepoRoot: "/repo",
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: attached, Carrier: "attachment",
			}},
		}),
		PerfBundle: &PerfBundle{
			Meta: PerfMeta{Source: "hitrace"},
			Observations: []PerfObservation{{
				Authority: PerfObservationAuthorityPreTriageModelExtraction,
				Kind:      "span",
				Subject:   "UI span",
				Summary:   "navigation narrative",
			}},
		},
	}
	ledger := CompileObservationLedger(input)
	if len(ledger.Records) != 1 {
		t.Fatalf("records=%d, want one perf observation: %+v", len(ledger.Records), ledger.Records)
	}
	record := ledger.Records[0]
	if got := record.SourceRef.CaptureIdentityPath; got != "customer.systrace" {
		t.Fatalf("pathless perf row capture identity=%q, want %q", got, "customer.systrace")
	}
	if record.SourceRef.Path != "" {
		t.Fatalf("producer-local path must remain pathless, got %q", record.SourceRef.Path)
	}
}

func TestCompileObservationLedger_PathlessPerfRowsFailOpenForAmbiguousAttachments(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RepoRoot: "/repo",
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "/repo/a.systrace", Carrier: "attachment"},
				{Kind: "trace", Source: "/repo/b.systrace", Carrier: "attachment"},
			},
		}),
		PerfBundle: &PerfBundle{Observations: []PerfObservation{{
			Authority: PerfObservationAuthorityPreTriageModelExtraction,
			Subject:   "ambiguous",
		}}},
	})
	if len(ledger.Records) != 1 {
		t.Fatalf("records=%d: %+v", len(ledger.Records), ledger.Records)
	}
	if got := ledger.Records[0].SourceRef.CaptureIdentityPath; got != "" {
		t.Fatalf("ambiguous attachments must not bind pathless perf rows, got %q", got)
	}
}
