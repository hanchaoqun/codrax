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
