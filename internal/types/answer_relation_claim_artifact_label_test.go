package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// answer_relation_claim_artifact_label_test.go — V1-4 §40.26 ③ on the relation
// fold (§40.48 fold-in of the review): the partition key is not a dead field.
// Two same-member relation authorities compiled from two trace files stay
// distinguishable END TO END — on the authority, on the model-copyable claim
// projection, on the JSON wire, through the ledger dedupe, and in validation
// — and deleting the stamping in CompileTraceAnswerRelationAuthorities is red.

func twoArtifactSameMemberRelationSet(identicalValues bool) TraceCausalProjectionSet {
	account := func(total float64) *TraceCausalProjectionTargetStateAccount {
		return &TraceCausalProjectionTargetStateAccount{
			Subject: "UIThread-100", RunningMS: total / 2, RunnableMS: total / 4,
			SleepMS: total / 4, TotalMS: total, WindowStartTs: 10, WindowEndTs: 10 + total/1000,
		}
	}
	totalB := 40.0
	if identicalValues {
		totalB = 20.0
	}
	return TraceCausalProjectionSet{Projections: []TraceCausalProjection{
		{ArtifactLabel: "a.systrace", WindowStartTs: 10, WindowEndTs: 10.02, TargetStateAccount: account(20)},
		{ArtifactLabel: "b.systrace", WindowStartTs: 10, WindowEndTs: 10 + totalB/1000, TargetStateAccount: account(totalB)},
	}}
}

func TestAnswerRelationAuthoritiesKeepPartitionKeyEndToEnd(t *testing.T) {
	set := twoArtifactSameMemberRelationSet(false)
	if got := TraceCausalProjectionSetArtifactLabels(set); strings.Join(got, ",") != "a.systrace,b.systrace" {
		t.Fatalf("partition roster: %v", got)
	}
	authorities := CompileTraceAnswerRelationAuthorities(set)
	if len(authorities) != 2 || authorities[0].Kind != AnswerRelationAuthorityClosedPartition || authorities[1].Kind != AnswerRelationAuthorityClosedPartition {
		t.Fatalf("fixture: want two closed-partition authorities, got %+v", authorities)
	}
	if !answerRelationSameSet(authorities[0].MemberRefs, authorities[1].MemberRefs) {
		t.Fatalf("fixture: the two relations must have the same members: %+v", authorities)
	}
	if authorities[0].ArtifactLabel != "a.systrace" || authorities[1].ArtifactLabel != "b.systrace" {
		t.Fatalf("authorities must carry their projection's partition key (deleting the stamp is red): %+v", authorities)
	}
	for i, authority := range authorities {
		claim := AnswerRelationClaimForAuthority(authority)
		if claim.ArtifactLabel != authority.ArtifactLabel {
			t.Fatalf("claim projection %d must keep the partition key: %+v", i, claim)
		}
		raw, err := json.Marshal(claim)
		if err != nil || !strings.Contains(string(raw), `"artifact_label":"`+authority.ArtifactLabel+`"`) {
			t.Fatalf("wire copy %d must carry artifact_label: %s %v", i, raw, err)
		}
		if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim}, authorities, false); err != nil {
			t.Fatalf("an exact labeled copy validates: %v", err)
		}
	}
	// A copied claim whose label names a different trace file than the
	// authority it cites is a precise mismatch (typed string equality); an
	// unlabeled copy is unspecified and still validates (typed escape lane).
	wrong := AnswerRelationClaimForAuthority(authorities[0])
	wrong.ArtifactLabel = "b.systrace"
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{wrong}, authorities, false); err == nil || !strings.Contains(err.Error(), "artifact_label") {
		t.Fatalf("a relabelled copy must be rejected with the field named: %v", err)
	}
	unlabeled := AnswerRelationClaimForAuthority(authorities[0])
	unlabeled.ArtifactLabel = ""
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{unlabeled}, authorities, false); err != nil {
		t.Fatalf("an unlabeled copy is unspecified, not wrong: %v", err)
	}
	// Equality is partition-aware: the same content from two trace files is
	// two claims.
	twin := AnswerRelationClaimForAuthority(authorities[0])
	twin.ArtifactLabel = "b.systrace"
	if AnswerRelationClaimsEqual([]AnswerRelationClaim{AnswerRelationClaimForAuthority(authorities[0])}, []AnswerRelationClaim{twin}) {
		t.Fatalf("claims from two trace files must not compare equal")
	}
}

// Identical accounting in two trace files yields identical content-fingerprint
// ids; the partition key is then the ONLY thing that keeps the two apart, and
// validation resolves a labeled claim against the authority of ITS trace file.
func TestAnswerRelationAuthoritiesIdenticalContentInTwoArtifactsStayDistinguishable(t *testing.T) {
	authorities := CompileTraceAnswerRelationAuthorities(twoArtifactSameMemberRelationSet(true))
	if len(authorities) != 2 || authorities[0].ID != authorities[1].ID {
		t.Fatalf("fixture: identical content must fingerprint to one id: %+v", authorities)
	}
	if authorities[0].ArtifactLabel == authorities[1].ArtifactLabel {
		t.Fatalf("two trace files must stay distinguishable: %+v", authorities)
	}
	for _, authority := range authorities {
		claim := AnswerRelationClaimForAuthority(authority)
		if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim}, authorities, false); err != nil {
			t.Fatalf("labeled claim for %s must validate against its own authority: %v", authority.ArtifactLabel, err)
		}
	}
	stray := AnswerRelationClaimForAuthority(authorities[0])
	stray.ArtifactLabel = "c.systrace"
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{stray}, authorities, false); err == nil {
		t.Fatalf("a label naming no trace file of this fold must be rejected")
	}
}

// Ledger-fed two-ruler rows are unlabeled and dedupe by id ahead of the
// projection-compiled copy; the partition key survives that dedupe (the
// LEDGER-MERGE-1 shape) instead of being dropped with the duplicate.
func TestAnswerRelationAuthoritiesFromLedgerKeepPartitionKeyThroughDedupe(t *testing.T) {
	unlabeled := AnswerRelationAuthority{ID: "x", Kind: AnswerRelationAuthorityClosedPartition}
	labeled := AnswerRelationAuthority{ID: "x", Kind: AnswerRelationAuthorityClosedPartition, ArtifactLabel: "a.systrace"}
	got := dedupeAnswerRelationAuthorities([]AnswerRelationAuthority{unlabeled, labeled, {ID: "y", ArtifactLabel: "b.systrace"}})
	if len(got) != 2 || got[0].ID != "x" || got[0].ArtifactLabel != "a.systrace" || got[1].ArtifactLabel != "b.systrace" {
		t.Fatalf("dedupe must keep the first row and backfill its partition key: %+v", got)
	}
}
