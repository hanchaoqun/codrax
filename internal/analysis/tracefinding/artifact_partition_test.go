package tracefinding

// artifact_partition_test.go — V1-4 (colleague_merge_audit §40.26, 2026-09-03):
// the multi-artifact fold keeps its partition key on every public seat
// surface — candidate decision, contract identity, public sidecar wire —
// sourced from the SAME projection.ArtifactLabel the answer's per-trace
// sections wear; plus the V1-1 (§40.25) 「词面来自 tracefence 单源」 ties for
// the sidecar evidence sentence, the description caliber discipline and the
// guide census.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

func artifactPartitionSet(labels ...string) types.TraceCausalProjectionSet {
	var set types.TraceCausalProjectionSet
	for i, label := range labels {
		set.Projections = append(set.Projections, types.TraceCausalProjection{
			ArtifactPath: "traces/" + label, ArtifactLabel: label,
			WindowStartTs: 1, WindowEndTs: 1.016,
			RankedSeats: []types.TraceCausalProjectionNode{{
				EvidenceID: "E-" + string(rune('A'+i)), Subject: "RenderThread", Rank: 1, TypeToken: "scheduler_latency",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMS: 4,
			}},
		})
	}
	return set
}

func TestCompileCandidateContractKeepsArtifactPartitionKeyOnEveryCandidate(t *testing.T) {
	contract, err := CompileCandidateContract(types.ObservationLedger{}, artifactPartitionSet("a.systrace", "b.systrace"), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Candidates) != 2 || contract.Candidates[0].Decision.CandidateID == contract.Candidates[1].Decision.CandidateID {
		t.Fatalf("a same-named thread in two trace files must be two candidates: %+v", contract.Candidates)
	}
	got := map[string]bool{}
	for _, candidate := range contract.Candidates {
		if candidate.Decision.SubjectName != "RenderThread" || candidate.Decision.ArtifactLabel == "" {
			t.Fatalf("candidate lost its partition key: %+v", candidate.Decision)
		}
		if candidate.Decision.EvidenceFacts == nil || candidate.Decision.EvidenceFacts.ArtifactLabel != candidate.Decision.ArtifactLabel {
			t.Fatalf("decision and evidence facts must read the same projection label: %+v", candidate.Decision)
		}
		got[candidate.Decision.ArtifactLabel] = true
	}
	if !got["a.systrace"] || !got["b.systrace"] {
		t.Fatalf("both labels must be present: %v", got)
	}
	if !reflect.DeepEqual(contract.ArtifactLabels, []string{"a.systrace", "b.systrace"}) || !contract.MultiArtifact() {
		t.Fatalf("contract must carry the partition roster in first-appearance order: %v", contract.ArtifactLabels)
	}
	// Acceptance pin (§40.26): the contract identity binds partition
	// membership — dropping a partition changes the set id even when the
	// surviving candidates are unchanged (a label-only projection).
	labelOnly := artifactPartitionSet("a.systrace", "c.systrace")
	labelOnly.Projections[1].RankedSeats = nil
	with, err := CompileCandidateContract(types.ObservationLedger{}, labelOnly, SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	without, err := CompileCandidateContract(types.ObservationLedger{}, artifactPartitionSet("a.systrace"), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(with.Candidates) != 1 || len(without.Candidates) != 1 || with.Candidates[0].Decision.CandidateID != without.Candidates[0].Decision.CandidateID {
		t.Fatalf("fixture drift: candidates must coincide: %+v / %+v", with.Candidates, without.Candidates)
	}
	if with.CandidateSetID == without.CandidateSetID || with.FindingID == without.FindingID {
		t.Fatalf("partition membership must be part of the contract identity")
	}
	if without.MultiArtifact() || len(without.ArtifactLabels) != 1 {
		t.Fatalf("single-artifact contract drifted: %v", without.ArtifactLabels)
	}
}

func TestSidecarWireCarriesArtifactLabelAcrossArtifacts(t *testing.T) {
	contract, err := CompileCandidateContract(types.ObservationLedger{}, artifactPartitionSet("a.systrace", "b.systrace"), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*types.TraceRootCauseItemV2{
		{CandidateID: contract.Candidates[0].Decision.CandidateID},
		{CandidateID: contract.Candidates[1].Decision.CandidateID},
	}}, contract)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	wire := string(raw)
	for _, want := range []string{`"artifact_label":"a.systrace"`, `"artifact_label":"b.systrace"`} {
		if !strings.Contains(wire, want) {
			t.Fatalf("wire missing %q:\n%s", want, wire)
		}
	}
	if strings.Count(wire, `"schema_version":2`) != 1 || strings.Contains(wire, "candidate_id") {
		t.Fatalf("append-only v2 with the receipt stripped:\n%s", wire)
	}
	if len(report.RootCauses) != 2 || report.RootCauses[0].Summary != report.RootCauses[1].Summary {
		t.Fatalf("same-named seats share a summary yet both survive: %+v", report.RootCauses)
	}
}

// The partition key is system-owned on the live selector lane too (V1-4
// §40.26 ③ / §40.48 S4): the binder builds every published item from the
// frozen contract candidate and reads only candidate_id + description from
// the model's selection, so a submitted artifact_label can never move a cause
// to another trace file. (Merge note, §40.44 × §40.48: the original pin on the
// retired Required-lane Validate snapshot compare went with that lane; this is
// the same property on the lane the customer actually receives.)
func TestBindRootCauseReportSelectionKeepsSystemOwnedArtifactLabel(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E-A"}, {ID: "E-B"}}}
	contract, err := CompileCandidateContract(ledger, artifactPartitionSet("a.systrace", "b.systrace"), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	selectable := SelectableRootCauseCandidates(contract)
	if len(selectable) != 2 {
		t.Fatalf("both partitions must be selectable: %+v", selectable)
	}
	submitted := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
	for _, candidate := range selectable {
		submitted.RootCauses = append(submitted.RootCauses, &types.TraceRootCauseItemV2{
			CandidateID: candidate.Decision.CandidateID, ArtifactLabel: "moved.systrace",
		})
	}
	bound, err := BindRootCauseReportSelection(submitted, contract)
	if err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
	if len(bound.RootCauses) != len(selectable) {
		t.Fatalf("every selection must bind: %+v", bound.RootCauses)
	}
	for i, item := range bound.RootCauses {
		want := selectable[i].Decision.ArtifactLabel
		if want == "" || item.ArtifactLabel != want || item.ArtifactLabel == "moved.systrace" {
			t.Fatalf("root_causes[%d]: artifact_label must be the contract's own partition key %q, got %q", i, want, item.ArtifactLabel)
		}
	}
	raw, _ := json.Marshal(bound)
	if strings.Contains(string(raw), "moved.systrace") {
		t.Fatalf("model-authored partition key leaked to the wire:\n%s", raw)
	}
}

// V1-1 (§40.25 acceptance 「词面来自 tracefence 单源」): the sidecar evidence
// sentence is built from the SAME Table ③e row the crown face reads, and the
// window-projection sentence never calls the number 有效归因 outside its
// disclosure suffix.
func TestSidecarEvidenceSentenceWordsComeFromTracefence(t *testing.T) {
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range contract.Candidates {
		item, ok := boundRootCauseItem(candidate)
		if !ok {
			t.Fatalf("candidate not representable: %+v", candidate)
		}
		phrase, suffix, ok := tracefence.SidecarImpactCaliberPhrase(item.ImpactCaliber)
		if !ok {
			t.Fatalf("caliber %q has no tracefence row", item.ImpactCaliber)
		}
		want := candidate.Decision.SubjectName + " 在目标窗口内的" + phrase + "为 "
		if !strings.HasPrefix(item.Evidence[0], want) || !strings.Contains(item.Evidence[0], " ms"+suffix) {
			t.Fatalf("evidence sentence must be rendered from the tracefence row (%q / %q): %q", phrase, suffix, item.Evidence[0])
		}
		if item.ImpactCaliber == types.TraceImpactCaliberWindowProjection {
			stripped := strings.ReplaceAll(item.Evidence[0], suffix, "")
			if strings.Contains(stripped, tracefence.ImpactCaliberEffectiveZH) {
				t.Fatalf("a window projection must never be called %s: %q", tracefence.ImpactCaliberEffectiveZH, item.Evidence[0])
			}
		}
	}
}

// V1-1 on the SIDECAR-NARR-1 description surface (§40.48 fold-in of the
// review: a bare word substring cannot tell a claim from a denial, so the
// caliber discipline is TEACHING, never a drop). Hedged/negated CROWNCAL-
// compliant descriptions of a window-projection seat — in either face — are
// published verbatim; the binder may attach a non-dropping advisory note,
// but the selection and the description both stand.
func TestBindRootCauseReportKeepsHedgedWindowProjectionDescriptions(t *testing.T) {
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), SeatFrameCausalityAuthority{Applicable: true})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	byID := map[string]types.TraceCauseDecision{}
	for _, c := range contract.Candidates {
		byID[c.Decision.SubjectName] = c.Decision
	}
	if byID["GLThread"].Magnitude.Caliber != types.TraceImpactCaliberWindowProjection || byID["RenderThread"].Magnitude.Caliber != types.TraceImpactCaliberEffectiveAttribution {
		t.Fatalf("fixture drift: %+v", byID)
	}
	_, suffix, _ := tracefence.SidecarImpactCaliberPhrase(types.TraceImpactCaliberWindowProjection)
	for _, description := range []string{
		"GLThread 窗内投影占用约 3 ms，尚未发布" + tracefence.ImpactCaliberEffectiveZH,
		"GLThread 该值不是" + tracefence.ImpactCaliberEffectiveZH + "，仅为窗内投影",
		"GLThread 窗内投影占用约 3 ms(未发布" + tracefence.ImpactCaliberEffectiveZH + ")",
		"GLThread 窗内投影占用约 3 ms" + suffix,
		"GLThread window-projected occupancy about 3 ms; no effective " + tracefence.ImpactCaliberEffectiveEN + " was published",
		"GLThread 在窗内的" + tracefence.ImpactCaliberEffectiveZH + "约 3 ms，UI 线程因此等待",
	} {
		report, advisories, err := BindRootCauseReportSelectionWithAdvisories(&types.TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*types.TraceRootCauseItemV2{
			{CandidateID: byID["GLThread"].CandidateID, Description: description},
			{CandidateID: byID["RenderThread"].CandidateID, Description: description},
		}}, contract)
		if err != nil || len(report.RootCauses) != 2 {
			t.Fatalf("selection must stand for %q: %v %+v", description, err, report)
		}
		for i, item := range report.RootCauses {
			if item.Description != description {
				t.Fatalf("description %q on root_causes[%d] (caliber %s) must be published verbatim, got %q (advisories %v)", description, i, item.ImpactCaliber, item.Description, advisories)
			}
		}
		for _, advisory := range advisories {
			if advisory.Dropped() || strings.Contains(advisory.Reason, "dropped") {
				t.Fatalf("the caliber note is advisory only, never a drop: %+v", advisory)
			}
			if advisory.Kind != RootCauseAdvisoryNote {
				t.Fatalf("a kept description mints only the typed note kind: %+v", advisory)
			}
		}
	}
}

// The caliber discipline lives in ONE teaching sentence (schema description
// and selector context render the same function): it names the typed token
// and the Table ③e word, and it no longer threatens a drop for the word.
func TestRootCauseDescriptionTeachingCarriesCaliberDisciplineWithoutDrop(t *testing.T) {
	teaching := types.TraceRootCauseDescriptionTeaching()
	for _, want := range []string{types.TraceImpactCaliberWindowProjection, "never as " + tracefence.ImpactCaliberEffectiveZH, "internal reference"} {
		if !strings.Contains(teaching, want) {
			t.Fatalf("teaching must carry %q: %q", want, teaching)
		}
	}
	if strings.Contains(teaching, "calls a "+types.TraceImpactCaliberWindowProjection+" number") {
		t.Fatalf("teaching must not promise a word-triggered drop (noisy prose signal ⇒ soft guidance only): %q", teaching)
	}
}

// Guide census (V1-1 §40.25 / V1-4 §40.26): the customer guide spells every
// closed-set caliber token, both sidecar phrases and the artifact_label field
// verbatim — the doc is a taught surface, not free prose.
func TestRootCauseGuideSpellsClosedSetsVerbatim(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "guides", "trace_short_root_cause_implementation_zh.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(src)
	for _, caliber := range types.AllTraceImpactCalibers() {
		phrase, suffix, _ := tracefence.SidecarImpactCaliberPhrase(caliber)
		for _, want := range []string{"`" + caliber + "`", phrase + "为", suffix} {
			if !strings.Contains(guide, want) {
				t.Fatalf("guide must carry %q for caliber %s", want, caliber)
			}
		}
	}
	for _, want := range []string{"`artifact_label`", "同名"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("guide must teach %q", want)
		}
	}
}
