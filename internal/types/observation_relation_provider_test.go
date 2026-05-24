package types

import "testing"

func TestObservationRelationCandidateSource_VCSDiffCurrentSourceExactAnchor(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		currentObservationRecord("current", "internal/agent/explorer.go", 6152, "postProactiveClosureTargetSignal"),
		{
			ID:      "diff",
			Origin:  AnswerEvidenceOriginVCSDiff,
			Subject: "postProactiveClosureTargetSignal",
			SourceRef: ObservationSourceRef{
				Kind:     ObservationSourceVCSDiff,
				Commit:   "abc123",
				Pathspec: "internal/agent/explorer.go",
			},
			Span:        ObservationSpan{NewLine: 6152},
			Summary:     "diff touched the current mid-loop signal",
			RawExcerpt:  "+ postProactiveClosureTargetSignal(obs)",
			SupportRefs: []string{"internal/agent/explorer.go:6152"},
		},
	}}
	rows := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
		Sources: []string{"postProactiveClosureTargetSignal"},
		Purpose: TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("rows len=%d, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Relation != TypedRelationSourceAnchor ||
		row.Carrier != TypedRelationCarrierExternalObservation ||
		row.Precision != TypedRelationPrecisionExactEvidence ||
		row.Member.File != "internal/agent/explorer.go" ||
		row.Member.Line != 6152 {
		t.Fatalf("unexpected exact source-anchor row: %+v", row)
	}
}

func TestObservationRelationCandidateSource_RuntimeSupportRefExactAnchor(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		currentObservationRecord("current", "internal/llm/openai.go", 88, "request timeout"),
		{
			ID:      "log",
			Origin:  AnswerEvidenceOriginRuntimeArtifact,
			Subject: "request timeout",
			SourceRef: ObservationSourceRef{
				Kind:         ObservationSourceRuntimeArtifact,
				ArtifactID:   "attached_log",
				ArtifactKind: "log",
			},
			Span:        ObservationSpan{LineStart: 12, LineEnd: 12},
			SupportRefs: []string{"request timeout @ internal/llm/openai.go:88"},
			Summary:     "log observed a request timeout",
		},
	}}
	rows := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
		Sources: []string{"request timeout"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 1 || rows[0].Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("runtime support ref should produce one exact row: %+v", rows)
	}
}

func TestObservationRelationCandidateSource_ClaimOnlyIsPromptOnly(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		currentObservationRecord("current", "internal/config/runtime.go", 17, "Config"),
		{
			ID:       "mcp",
			Origin:   AnswerEvidenceOriginMCPResource,
			Subject:  "Config",
			ClaimKey: "Config",
			SourceRef: ObservationSourceRef{
				Kind:        ObservationSourceMCPResource,
				Server:      "docs",
				ResourceURI: "mcp://docs/config",
			},
			Summary: "external doc mentions Config",
		},
	}}
	prompt := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
		Sources: []string{"Config"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(prompt) != 1 || prompt[0].Precision != TypedRelationPrecisionNameOnly {
		t.Fatalf("claim-only bridge should be prompt-only name-only row: %+v", prompt)
	}
	coverage := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
		Sources: []string{"Config"},
		Purpose: TypedRelationPurposeCoverageGate,
	})
	if len(coverage) != 0 {
		t.Fatalf("name-only source-anchor rows must not be coverage-gate eligible: %+v", coverage)
	}
}

func TestObservationRelationCandidateSource_ExternalOriginsSupportExactAnchors(t *testing.T) {
	origins := []AnswerEvidenceOrigin{
		AnswerEvidenceOriginCommandMeasurement,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource,
	}
	for _, origin := range origins {
		t.Run(string(origin), func(t *testing.T) {
			ledger := ObservationLedger{Records: []ObservationRecord{
				currentObservationRecord("current", "src/main.ts", 10, "main"),
				{
					ID:      "external",
					Origin:  origin,
					Subject: "main",
					SourceRef: ObservationSourceRef{
						Kind:        ObservationSourceKindForOrigin(origin),
						RawRef:      "payload://external",
						ResourceURI: "external://resource",
					},
					SupportRefs: []string{"main @ src/main.ts:10"},
				},
			}}
			rows := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
				Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
				Sources: []string{"main"},
				Purpose: TypedRelationPurposeCoverageGate,
			})
			if len(rows) != 1 || rows[0].Precision != TypedRelationPrecisionExactEvidence {
				t.Fatalf("%s exact support ref should produce exact row: %+v", origin, rows)
			}
		})
	}
}

func TestObservationRelationCandidateSource_NegativeObservationDoesNotPromote(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		currentObservationRecord("current", "internal/foo.go", 5, "Foo"),
		{
			ID:          "negative",
			Origin:      AnswerEvidenceOriginRuntimeArtifact,
			Subject:     "Foo",
			Negative:    true,
			SourceRef:   ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "attached_log"},
			SupportRefs: []string{"Foo @ internal/foo.go:5"},
		},
	}}
	rows := ObservationRelationCandidateSource{Ledger: ledger}.TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationSourceAnchor},
		Sources: []string{"Foo"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 0 {
		t.Fatalf("negative observations must stay observation facts, not source-anchor relations: %+v", rows)
	}
}

func currentObservationRecord(id, file string, line int, key string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginCurrentSource,
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: file},
		Span:            ObservationSpan{LineStart: line, LineEnd: line},
		ClaimKey:        key,
		Subject:         key,
		AnchorKind:      AnchorDefinition,
		EvidenceKind:    EvidenceDirect,
		EvidenceScope:   ScopeLine,
		GroundingStatus: GroundingGrounded,
		Summary:         key + " source anchor",
	}
}
