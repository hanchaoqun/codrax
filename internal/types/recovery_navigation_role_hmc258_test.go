package types

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// Producer fixture validates a complete physical snapshot. Tool-package roots
// additionally exercise the public read_file/resume admission and consumers.
func recoveryRoleProducer258(t *testing.T) (*MutableState, string, RecoveryNavigationRead) {
	t.Helper()
	root := t.TempDir()
	c, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "settings.json")
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	m := NewMutableState("current recovery")
	candidate := m.PrepareRecoveryNavigationRead(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tracecatalog.DecodeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	return m, path, candidate
}

func TestHMC258RecoveryNavigationSourceClaimContextAndCachedPools(t *testing.T) {
	m, path, candidate := recoveryRoleProducer258(t)
	root := filepath.Dir(path)
	if err := os.WriteFile(filepath.Join(root, "workers.json"), []byte(`{"workers":8}`), 0600); err != nil {
		t.Fatal(err)
	}
	var items []EvidenceItem
	var members []SourceInventoryObservationMember
	for _, name := range []string{"settings.json", "workers.json"} {
		items = append(items, EvidenceItem{Source: name, Subject: "workers", LineStart: 1, LineEnd: 1, Kind: EvidenceDirect, Scope: ScopeLine, GroundingStatus: GroundingGrounded})
		members = append(members, SourceInventoryObservationMember{Name: "workers", File: name, Line: 1, CoverageState: SourceInventoryCoverageObserved})
	}
	input := ObservationLedgerInput{RepoRoot: root, recoveryNavigationConsumer: m, EvidenceItems: items,
		SourceInventoryObservation: SourceInventoryObservation{Active: true, Sets: []SourceInventoryObservationSet{{Members: members}}},
		ToolResults:                []ToolResult{{ToolName: "read_file", Success: true, ReadCoverage: &ToolReadCoverage{Path: "settings.json", LineStart: 1, LineEnd: 1, TotalLines: 1}}}}
	// Compile first: stale copied source claims must also be requalified once
	// complete validated reading establishes actual navigation use.
	input.aggregateSourceClaims = compileAggregateSourceClaimContext(input)
	if len(input.aggregateSourceClaims.items) != 2 {
		t.Fatal("fixture omitted old source claims")
	}
	if !m.RecordValidatedRecoveryNavigation(context.Background(), candidate) {
		t.Fatal("producer registration failed")
	}
	for _, source := range []ObservationLedgerInput{input, {RepoRoot: root, recoveryNavigationConsumer: m, EvidenceItems: items, ToolResults: input.ToolResults, SourceInventoryObservation: input.SourceInventoryObservation}} {
		claims := compileAggregateSourceClaimContext(source)
		if len(claims.items) != 1 || claims.items[0].Source != "workers.json" || len(claims.inventory) != 1 || claims.inventory[0].path != "workers.json" {
			t.Fatalf("cached/direct member pool bypassed purpose: %+v", claims)
		}
		for _, witness := range claims.coordinates.witnesses {
			if witness.path == "settings.json" {
				t.Fatal("old read coverage restored source support")
			}
		}
		if _, ok := source.currentSourceSupportWitnesses().bindLocation(AnswerSourceLocationSurface{File: "settings.json", LineStart: 1}); ok {
			t.Fatal("direct witness bypass")
		}
	}
	// Internal source-kind round trip carries no private producer registry and
	// is not a current-source or runtime measurement by either authority face.
	ledger := CompileObservationLedger(input)
	var nav ObservationRecord
	for _, record := range ledger.Records {
		if record.SourceRef.Kind == ObservationSourceRecoveryNavigation {
			nav = record
			break
		}
	}
	if nav.SourceRef.Path == "" || nav.Origin != AnswerEvidenceOriginExternalDocument {
		t.Fatal("historical original disappeared")
	}
	if len(externalObservationSufficiencyCandidates([]ObservationRecord{nav})) != 0 {
		t.Fatal("historical fields satisfied current external observation")
	}
	b, _ := json.Marshal(nav)
	var restored ObservationRecord
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.SourceRef.Kind != ObservationSourceRecoveryNavigation || restored.Value != nav.Value || restored.ClaimKey != nav.ClaimKey {
		t.Fatal("kind/field roundtrip changed")
	}
	if NewMutableState("new task").IsRecoveryNavigationMaterial(root, path) {
		t.Fatal("JSON/history restored private role")
	}
	// A write consumer keeps ordinary source authority despite prior navigation.
	bus := &BusContext{RepoRoot: root, Mode: ModeApply, Mutable: m, EvidenceItems: items}
	if claims := compileAggregateSourceClaimContext(ObservationLedgerInputFromBusContext(bus, 0)); len(claims.items) != 2 {
		t.Fatal("write source pool lost same-file inspection")
	}
}

func TestHMC258RecoveryNavigationConcurrentResetRevokesFork(t *testing.T) {
	m, path, candidate := recoveryRoleProducer258(t)
	if !m.RecordValidatedRecoveryNavigation(context.Background(), candidate) {
		t.Fatal("register")
	}
	fork := m.ForkForExploreDispatch()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				fork.IsRecoveryNavigationMaterial(filepath.Dir(path), path)
				fork.RecordValidatedRecoveryNavigation(context.Background(), candidate)
			}
		}()
	}
	m.ResetTurnAArtifacts()
	wg.Wait()
	if fork.IsRecoveryNavigationMaterial(filepath.Dir(path), path) || m.IsRecoveryNavigationMaterial(filepath.Dir(path), path) || fork.RecordValidatedRecoveryNavigation(context.Background(), candidate) {
		t.Fatal("old concurrent producer revived revoked role")
	}
	// New complete producer qualification is possible in the fresh epoch.
	fresh := m.PrepareRecoveryNavigationRead(path)
	if !m.RecordValidatedRecoveryNavigation(context.Background(), fresh) {
		t.Fatal("reset poisoned new epoch")
	}
}

func TestHMC258RecoveryNavigationAnswerIntentUsesNamedRuntimeDomain(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		rm := hmc257NamedRuntimeRequest()
		if mixed {
			rm.AnalyzerHints.ExactTargets = []string{"src/owner.go"}
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionSourceLocation
		}
		got := CompileAnswerIntentContract(rm, nil)
		if !got.HasOrigin(AnswerEvidenceOriginRuntimeArtifact) || got.HasOrigin(AnswerEvidenceOriginCurrentSource) != mixed {
			t.Fatalf("named domain source/runtime mismatch mixed=%v: %+v", mixed, got)
		}
	}
	source := CompileAnswerIntentContract(RequestModel{Intent: IntentConfigQuery, AnalyzerHints: AnalyzerHints{ExactTargets: []string{"settings.json"}}}, nil)
	if !source.HasOrigin(AnswerEvidenceOriginCurrentSource) || source.HasOrigin(AnswerEvidenceOriginRuntimeArtifact) {
		t.Fatal("ordinary source request was reclassified")
	}
}
