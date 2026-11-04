package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A successful search of an actual query artifact must not become repository
// source coverage, including zero matches and the native/streamed backends.
// This checks the public producer, ledger and Known Facts receipt together.
func TestHMC344RuntimeSearchOriginPublicMatrix(t *testing.T) {
	for _, refIndex := range []int{0, 1} {
		for _, mode := range []string{"native", "streamed", "files_only", "zero"} {
			t.Run(filepath.Ext([]string{"out.txt", "out.json"}[refIndex])+"/"+mode, func(t *testing.T) {
				ctx, published, refs := b1624PublishedResult(t)
				before, _ := json.Marshal(published)
				p := map[string]any{"path": filepath.Base(refs[refIndex]), "pattern": "quoted-result-span", "fixed_string": true}
				if mode == "native" {
					p["line_start"], p["line_end"] = 1, 100000
				}
				if mode == "files_only" {
					p["files_only"] = true
				}
				if mode == "zero" {
					p["pattern"] = "literal-never-in-query-567812"
				}
				params, _ := json.Marshal(p)
				r, err := (&GrepTool{}).Execute(ctx, params)
				if err != nil || !r.Success || r.PathDiscovery == nil {
					t.Fatalf("public search failed: %v %+v", err, r)
				}
				if r.PathDiscovery.NoMatches != (mode == "zero") || r.RuntimeArtifactRead != nil || r.ReadCoverage != nil {
					t.Fatal("search fabricated a contiguous read, or lost its actual outcome")
				}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
				if len(ledger.Records) != 1 || ledger.Records[0].Origin != types.AnswerEvidenceOriginRuntimeArtifact || ledger.Records[0].SourceRef.Kind != types.ObservationSourceRuntimeArtifact || ledger.Records[0].GroundingPolicy != types.ClaimGroundingDisplayOnly || ledger.Records[0].Negative != (mode == "zero") {
					t.Fatalf("query-result search was promoted to source/negative-source proof: %+v", ledger.Records)
				}
				f := types.NewToolResultRepoFact(r, r.RawRef, .8)
				observed, ok := f.ToolObservation()
				if !ok || observed.TypedRecords != 1 || !reflect.DeepEqual(observed.Origins, []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginRuntimeArtifact}) {
					t.Fatalf("Known Facts conflicts with the actual search input: %+v", observed)
				}
				after, _ := json.Marshal(published)
				if string(before) != string(after) {
					t.Fatal("search classification changed native facts or query credentials")
				}
			})
		}
	}
}

func TestHMC344RuntimeSearchKeepsRepositoryAndLogScopesSeparate(t *testing.T) {
	ctx, _, _ := b1624PublishedResult(t)
	for _, name := range []string{"service.go", "service.log"} {
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, name), []byte("business_signal\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, pattern := range []string{"business_signal", "no_such_literal_123"} {
			params, _ := json.Marshal(map[string]any{"path": name, "pattern": pattern, "fixed_string": true})
			r, err := (&GrepTool{}).Execute(ctx, params)
			if err != nil || !r.Success {
				t.Fatalf("actual source/log search: %v %+v", err, r)
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
			want := types.AnswerEvidenceOriginCurrentSource
			if pattern != "business_signal" {
				want = types.AnswerEvidenceOriginRepoNegativeSearch
			}
			if name == "service.log" {
				want = types.AnswerEvidenceOriginRuntimeArtifact
			}
			if len(ledger.Records) != 1 || ledger.Records[0].Origin != want {
				t.Fatalf("source/log outcome mixed: name=%s pattern=%s %+v", name, pattern, ledger.Records)
			}
		}
	}
}

func TestHMC344RuntimeMissCannotJoinSourceNegativeAuthority(t *testing.T) {
	ctx, _, refs := b1624PublishedResult(t)
	params, _ := json.Marshal(map[string]any{"path": refs[1], "pattern": "no_such_literal_123", "fixed_string": true})
	r, err := (&GrepTool{}).Execute(ctx, params)
	if err != nil || !r.Success || r.PathDiscovery == nil || !r.PathDiscovery.NoMatches {
		t.Fatalf("real runtime miss absent: %v %+v", err, r)
	}
	ctx.EvidenceItems = []types.EvidenceItem{{ID: "neg-code", Kind: types.EvidenceAbsent, Scope: types.ScopeNegative,
		GroundingStatus: types.GroundingGrounded, NegativeScope: types.NegativeScopeFile,
		NegativeQuery: &types.NegativeQuery{File: "service.go", Pattern: "missing_config_key"}}}
	ctx.ToolResults = []types.ToolResult{r}
	rows := currentSourceNegativeScopeAuthorityRows(ctx)
	if len(rows) != 1 || rows[0].Producer != "verified_negative_evidence" {
		t.Fatalf("runtime search borrowed the unrelated verified code absence: %+v", rows)
	}
}
