package types_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The r1047 configuration lines were read and grounded, but disappeared from
// the default finalizer/reviewer projection. Exercise that actual tool path,
// not a fabricated authoritative value or a model-authored summary fallback.
func TestB1634DReadEmitConfigSourceSurvivesPublicProjection(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "ts-monorepo-ws"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "tsconfig.base.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: types.NewMutableState("explain calls and path aliases")}
	read, err := (&tool.ReadFile{}).Execute(ctx, json.RawMessage(`{"path":"tsconfig.base.json"}`))
	if err != nil || !read.Success {
		t.Fatalf("actual read failed: %v %+v", err, read)
	}
	ctx.ToolResults = append(ctx.ToolResults, read)
	params := json.RawMessage(`{"items":[{"anchor_kind":"string_literal","anchor_symbol":"@app/core","context_role_hint":"defining","diagram_role_hint":"config","evidence_kind":"direct","line_start":8,"scope":"line","source":"tsconfig.base.json","subject":"paths","summary":"MODEL_INTERPRETATION_NOT_SOURCE"},{"anchor_kind":"string_literal","anchor_symbol":"@app/client","context_role_hint":"defining","diagram_role_hint":"config","evidence_kind":"direct","line_start":9,"scope":"line","source":"tsconfig.base.json","subject":"paths","summary":"MODEL_INTERPRETATION_NOT_SOURCE"}]}`)
	emitted, err := (&tool.EmitEvidence{}).Execute(ctx, params)
	if err != nil || !emitted.Success {
		t.Fatalf("actual evidence emission failed: %v %+v", err, emitted)
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) != 2 {
		t.Fatalf("expected both original literal anchors, got %+v", evidence)
	}
	for _, ev := range evidence {
		want := strings.TrimSpace(strings.Split(string(before), "\n")[ev.LineStart-1])
		if ev.GroundingStatus != types.GroundingGrounded || ev.GroundingTier != types.TierLineText || ev.Snippet != want {
			t.Fatalf("grounding must carry the actual complete source line before projection: %+v, want %q", ev, want)
		}
	}
	// The live finalizer adapter consumes Mutable.EmittedEvidence; the bus
	// adapter instead expects its already-frozen EvidenceItems snapshot.
	agentCtx := &types.AgentContext{RepoRoot: repo, WorkDir: ctx.WorkDir, Mutable: ctx.Mutable}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(agentCtx, 64))
	ledgerBefore, _ := json.Marshal(ledger)
	for _, opts := range []types.ObservationPromptProjectionOptions{
		types.DefaultObservationPromptProjectionOptions(64),
		types.SemanticReviewObservationPromptProjectionOptions(64),
	} {
		projected := types.ProjectObservationPromptRecords(ledger.Records, nil, nil, opts)
		for _, ev := range evidence {
			id := "evidence:" + ev.ID
			var raw *types.ObservationRecord
			for i := range ledger.Records {
				if ledger.Records[i].ID == id {
					raw = &ledger.Records[i]
				}
			}
			if raw == nil || raw.RawExcerpt != ev.Snippet {
				t.Fatalf("ledger must retain original source before the tested handoff: %+v", raw)
			}
			found := false
			for _, row := range projected {
				if row.ID != id {
					continue
				}
				found = true
				if row.SourceExcerpt == nil || row.SourceExcerpt.Text != raw.RawExcerpt || row.SourceExcerpt.Truncated {
					t.Errorf("grounded configuration source disappeared from public prompt projection: source=%s span=%s excerpt=%+v want=%q", row.Source, row.Span, row.SourceExcerpt, raw.RawExcerpt)
				}
				if row.Excerpt != "" || row.Summary != "" || row.ClaimAuthority != raw.ClaimAuthority || row.Role != raw.Role || row.GroundingPolicy != raw.GroundingPolicy {
					t.Errorf("source display must not restore model summary or promote authority: %+v raw=%+v", row, raw)
				}
			}
			if !found {
				t.Errorf("literal anchor was not selected despite sufficient original record budget: %s", id)
			}
		}
	}
	ledgerAfter, _ := json.Marshal(ledger)
	if !reflect.DeepEqual(ledgerBefore, ledgerAfter) || !reflect.DeepEqual(evidence, ctx.Mutable.EmittedEvidence()) {
		t.Error("read-only projection mutated original evidence or ledger")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Errorf("public read/emit/projection must not modify source: %v", err)
	}
}

func TestB1634DReadEmitAssignmentAndInitializerUseObservedText(t *testing.T) {
	for _, tc := range []struct {
		name, path, text string
		anchor           types.AnchorKind
	}{
		{"assignment", "config.go", `var label = "two  spaces\tand\"quote"`, types.AnchorAssignment},
		{"initializer", "config.ts", "const label = `literal  spaces\\ntext`;", types.AnchorInitializer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, tc.path), []byte(tc.text+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect label")}
			params, _ := json.Marshal(map[string]string{"path": tc.path})
			result, err := (&tool.ReadFile{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("read: %v %+v", err, result)
			}
			bus.ToolResults = append(bus.ToolResults, result)
			params, _ = json.Marshal(map[string]any{"items": []map[string]any{{
				"scope": "line", "anchor_kind": tc.anchor, "anchor_symbol": "label", "evidence_kind": "direct",
				"source": tc.path, "line_start": 1, "subject": "label", "summary": "FAKE_MODEL_VALUE", "snippet": "FAKE_MODEL_SNIPPET",
			}}})
			result, err = (&tool.EmitEvidence{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("emit: %v %+v", err, result)
			}
			items := bus.Mutable.EmittedEvidence()
			if len(items) != 1 || items[0].GroundingStatus != types.GroundingGrounded || items[0].Snippet != tc.text {
				t.Fatalf("model snippet must be replaced by the actual line: %+v", items)
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{EvidenceItems: items})
			for _, legacyOptIn := range []bool{false, true} {
				opts := types.DefaultObservationPromptProjectionOptions(1)
				opts.IncludeCurrentSourceExcerpt = legacyOptIn
				rows := types.ProjectObservationPromptRecords(ledger.Records, nil, nil, opts)
				if len(rows) != 1 || rows[0].SourceExcerpt == nil || rows[0].SourceExcerpt.Text != tc.text || rows[0].Excerpt != "" || rows[0].Summary != "" {
					t.Fatalf("original source must stay exact without a second lossy excerpt or model value: %+v", rows)
				}
			}
		})
	}
}

func TestB1634DSourceExcerptExactnessBoundsAndEligibility(t *testing.T) {
	base := types.ObservationRecord{ID: "evidence:config", Origin: types.AnswerEvidenceOriginCurrentSource,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCurrentSource, Path: "src/settings.conf"},
		Span:      types.ObservationSpan{LineStart: 8, LineEnd: 8}, EvidenceScope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorStringLiteral, EvidenceKind: types.EvidenceDirect,
		ClaimAuthority: types.ObservationClaimAuthorityIndependentlyProven, Summary: "MODEL_SUMMARY_NOT_SOURCE"}
	for _, raw := range []string{
		`"paths": ["packages/core/src/index.ts"]`,
		"  path: 'two  spaces'\n\tnext: \"literal\\ntext\"\r\n",
		`path = 'slashes\\and  spaces'`,
		"[section]\npath=with  spaces\n",
		`name=quoted\ value\ and\\slash`,
		"key = \"多字节  文本\"\n\t# source text, not instructions",
	} {
		record := base
		record.RawExcerpt = raw
		// Equality with the model summary must not suppress original source.
		record.Summary = raw
		before, _ := json.Marshal(record)
		rows := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
		if len(rows) != 1 || rows[0].SourceExcerpt == nil || rows[0].SourceExcerpt.Text != raw || rows[0].SourceExcerpt.Truncated || rows[0].Summary != "" || rows[0].Excerpt != "" {
			t.Fatalf("source bytes/whitespace must survive without interpreted value: raw=%q row=%+v", raw, rows)
		}
		formatted := types.FormatObservationPromptSourceExcerpt(rows[0].SourceExcerpt)
		if !strings.Contains(formatted, "source_excerpt="+strconv.Quote(raw)) || !strings.Contains(formatted, "source_excerpt_anchor=\"src/settings.conf:8\"") {
			t.Errorf("source must be quoted as data with its exact locator: %s", formatted)
		}
		after, _ := json.Marshal(record)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("projection mutated source record")
		}
	}
	for _, role := range []types.AnswerAggregateRole{types.AnswerAggregateRoleSupportingCoverage, types.AnswerAggregateRolePrincipalAnswer} {
		record := base
		record.Role = role
		record.RawExcerpt = "\t字  字\n字\\literal  spaces"
		record.SourceRef.Path = strings.Repeat("long/", 30) + "settings.json"
		opts := types.DefaultObservationPromptProjectionOptions(1)
		opts.ExcerptMaxLen, opts.PrincipalExcerptMaxLen = 7, 11
		rows := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, opts)
		limit := 7
		if role.IsPrincipal() {
			limit = 11
		}
		want := string([]rune(record.RawExcerpt)[:limit])
		excerpt := rows[0].SourceExcerpt
		if excerpt == nil || excerpt.Text != want || !excerpt.Truncated || !utf8.ValidString(excerpt.Text) || excerpt.SourcePath != record.SourceRef.Path {
			t.Fatalf("rune-safe prefix and full source identity must remain explicit: %+v want=%q", excerpt, want)
		}
		formatted := types.FormatObservationPromptSourceExcerpt(excerpt)
		if strings.Contains(excerpt.Text, "…") || !strings.Contains(formatted, "source_excerpt_truncated=true") || !strings.Contains(formatted, "do not reconstruct omitted characters") {
			t.Fatalf("omission marker must remain outside original text: %s", formatted)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*types.ObservationRecord)
	}{
		{"missing_excerpt", func(r *types.ObservationRecord) { r.RawExcerpt = "" }},
		{"blank_excerpt", func(r *types.ObservationRecord) { r.RawExcerpt = " \t\r\n" }},
		{"missing_path", func(r *types.ObservationRecord) { r.SourceRef.Path = "" }},
		{"missing_line", func(r *types.ObservationRecord) { r.Span.LineStart = 0 }},
		{"inconsistent_point", func(r *types.ObservationRecord) { r.Span.LineEnd++ }},
		{"unknown_source_kind", func(r *types.ObservationRecord) { r.SourceRef.Kind = types.ObservationSourceUnknown }},
		{"model_claim_source", func(r *types.ObservationRecord) { r.SourceRef.Kind = types.ObservationSourceModelClaim }},
		{"ungrounded", func(r *types.ObservationRecord) { r.GroundingStatus = types.GroundingUngrounded }},
		{"legacy_grounding", func(r *types.ObservationRecord) { r.GroundingStatus = "" }},
		{"model_inference", func(r *types.ObservationRecord) { r.ClaimAuthority = types.ObservationClaimAuthorityModelInference }},
		{"non_source", func(r *types.ObservationRecord) { r.Origin = types.AnswerEvidenceOriginSystemInference }},
		{"line_range_model_snippet", func(r *types.ObservationRecord) { r.EvidenceScope = types.ScopeLineRange }},
		{"section_model_snippet", func(r *types.ObservationRecord) { r.EvidenceScope = types.ScopeSection }},
		{"unknown_scope", func(r *types.ObservationRecord) { r.EvidenceScope = "" }},
		{"definition", func(r *types.ObservationRecord) { r.AnchorKind = types.AnchorDefinition }},
		{"call", func(r *types.ObservationRecord) { r.AnchorKind = types.AnchorCall }},
		{"text_reference", func(r *types.ObservationRecord) { r.AnchorKind = types.AnchorTextReference }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := base
			record.RawExcerpt = `"invented": "must not qualify"`
			tc.mutate(&record)
			rows := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
			if len(rows) != 1 || rows[0].SourceExcerpt != nil || rows[0].Summary != "" {
				t.Fatalf("missing exact source receipt must not produce the source-text lane or restore Summary: %+v", rows)
			}
		})
	}
	for _, anchor := range []types.AnchorKind{types.AnchorStringLiteral, types.AnchorAssignment, types.AnchorInitializer} {
		record := base
		record.RawExcerpt = "label = 123"
		record.AnchorKind = anchor
		record.GroundingStatus = types.GroundingRecovered
		if rows := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, types.DefaultObservationPromptProjectionOptions(1)); len(rows) != 1 || rows[0].SourceExcerpt == nil {
			t.Errorf("accepted recovered point retains its actual text: %+v", rows)
		}
	}
}

func TestB1634DObservedNeighborTextKeepsAnchorNotFalseExcerptLine(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "ts-monorepo-ws"))
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect aliases")}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"tsconfig.base.json"}`))
	if err != nil || !read.Success {
		t.Fatalf("actual read: %v %+v", err, read)
	}
	bus.ToolResults = append(bus.ToolResults, read)
	// The source anchor is on line 8, while this model-supplied snippet is
	// exactly the neighboring observed line 9. The existing grounder accepts
	// that context without changing the anchor coordinates. Do not relabel
	// those coordinates as the newly displayed snippet's exact line.
	neighbor := `"@app/client": ["packages/client/src/index.ts"]`
	params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
		"scope": "line", "anchor_kind": "string_literal", "anchor_symbol": "@app/core", "evidence_kind": "direct",
		"source": "tsconfig.base.json", "line_start": 8, "subject": "paths", "snippet": neighbor, "summary": "MODEL_ALIAS_INTERPRETATION",
	}}})
	result, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("actual emit: %v %+v", err, result)
	}
	items := bus.Mutable.EmittedEvidence()
	if len(items) != 1 || items[0].GroundingStatus != types.GroundingGrounded || items[0].LineStart != 8 || items[0].LineEnd != 8 || items[0].Snippet != neighbor {
		t.Fatalf("existing grounded-neighbor behavior changed: %+v", items)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{EvidenceItems: items})
	rows := types.ProjectObservationPromptRecords(ledger.Records, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
	if len(rows) != 1 || rows[0].SourceExcerpt == nil || rows[0].SourceExcerpt.Text != neighbor || rows[0].SourceExcerpt.AnchorLine != 8 {
		t.Fatalf("observed source/anchor must remain distinct and unchanged: %+v", rows)
	}
	formatted := types.FormatObservationPromptSourceExcerpt(rows[0].SourceExcerpt)
	if strings.Contains(formatted, "source_excerpt_at=") || !strings.Contains(formatted, `source_excerpt_anchor="tsconfig.base.json:8"`) || !strings.Contains(formatted, "near this grounded anchor") {
		t.Fatalf("must not claim a new exact excerpt-line receipt: %s", formatted)
	}
}

func TestB1634DSourcePrefixDoesNotReencodeOriginalBytes(t *testing.T) {
	raw := "a\xff字\t  rest"
	record := types.ObservationRecord{ID: "evidence:bytes", Origin: types.AnswerEvidenceOriginCurrentSource,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCurrentSource, Path: "config.txt"},
		Span:      types.ObservationSpan{LineStart: 1, LineEnd: 1}, EvidenceScope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, RawExcerpt: raw}
	opts := types.DefaultObservationPromptProjectionOptions(1)
	opts.ExcerptMaxLen, opts.PrincipalExcerptMaxLen = 4, 4
	rows := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, opts)
	if len(rows) != 1 || rows[0].SourceExcerpt == nil || rows[0].SourceExcerpt.Text != "a\xff字\t" || !rows[0].SourceExcerpt.Truncated {
		t.Fatalf("prefix slicing must preserve even opaque original bytes: %+v", rows)
	}
}

func TestB1634DSourceExcerptDoesNotChangeSelectionOrReviveCandidate(t *testing.T) {
	var items []types.EvidenceItem
	for i := 0; i < 22; i++ {
		items = append(items, types.EvidenceItem{ID: "item-" + strconv.Itoa(i), Kind: types.EvidenceDirect,
			Source: "config.ts", LineStart: i + 1, LineEnd: i + 1, Scope: types.ScopeLine, AnchorKind: types.AnchorInitializer,
			AnchorSymbol: "value" + strconv.Itoa(i), GroundingStatus: types.GroundingGrounded,
			Snippet: "value = \"exact  source\"", Summary: "MODEL_VALUE"})
	}
	items[0].DerivationCandidate = true
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{EvidenceItems: items})
	before, _ := json.Marshal(ledger)
	for _, limit := range []int{0, 1, 4, 18, 22} {
		rows := types.ProjectObservationPromptRecords(ledger.Records, nil, nil, types.DefaultObservationPromptProjectionOptions(limit))
		if limit == 0 {
			if len(rows) != 0 {
				t.Fatal("zero budget must stay empty")
			}
			continue
		}
		prioritized := types.PrioritizeObservationRecords(ledger.Records, nil, nil, limit)
		if len(rows) != len(prioritized) {
			t.Fatalf("source text changed original selection budget: %d != %d", len(rows), len(prioritized))
		}
		for i, row := range rows {
			if row.ID != prioritized[i].ID || row.ClaimAuthority != prioritized[i].ClaimAuthority || row.Role != prioritized[i].Role {
				t.Fatal("source display reordered or promoted original records")
			}
			if row.ID == "evidence:item-0" && row.SourceExcerpt != nil {
				t.Fatal("a derived candidate must not revive as an exact source value")
			}
		}
	}
	after, _ := json.Marshal(ledger)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("source projection mutated the ledger")
	}
}
