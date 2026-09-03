package types

// answer_relation_claim_identity_test.go — §40.48 review fold-in (G-sidecar-
// artifact #0 ruling): the identity of a relation authority is the pair
// (id, artifact_label), everywhere. Identical accounting in two trace files
// fingerprints to ONE content id and is TWO authorities; the handoff face
// (CompileTraceAnswerRelationAuthorities — the exact call in
// agent/answer_document_trace_decision_handoff.go) and the validation face
// (CompileTraceAnswerRelationAuthoritiesFromLedger — the exact slate both real
// validators compile in tool/emit_investigation_complete_relation_claims.go
// and tool/answer_document_relation_claim_validation.go) must agree on that
// count, so an exact copy of EITHER trace file's printed relation_claim_copy
// validates end to end.
//
// EVOLUTION RECORD (§40.48 review, 2026-09-03): dedupeAnswerRelationAuthorities
// previously kept the FIRST row per bare id and only backfilled an EMPTY
// label, so the validator slate silently dropped the second trace file's
// labeled row while the un-deduped handoff face still printed both copies and
// taught "copy a claim with its artifact_label unchanged". An exact b.systrace
// copy was then rejected with the self-contradicting hint `this typed
// authority was compiled from trace file "a.systrace" — copy the label
// unchanged or omit it`, and copying both taught claims was rejected as
// `duplicates authority_id` (the validator seen-set was keyed on the bare id).
// TestTwoArtifactIdenticalAccountingClaimCopiesValidateEndToEnd and
// TestDedupeAnswerRelationAuthoritiesKeysOnAuthorityIdentity were RED against
// that code (verified with a scratch copy of the pre-fix file via
// go test -overlay); the single-artifact pin below was green before and after
// — it exists to hold the unchanged behaviour still.

import (
	"strings"
	"testing"
)

// twoArtifactIdenticalAccountingLedger builds a production-shaped ledger:
// per trace file one root_cause_primary anchor (resolves the projection
// window) plus one target_window_states record with byte-identical five-lane
// accounting, each carrying the typed SourceRef artifact identity lane.
func twoArtifactIdenticalAccountingLedger(paths ...string) ObservationLedger {
	var records []ObservationRecord
	for i, path := range paths {
		prefix := string(rune('a' + i))
		source := ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact, Path: path,
			ArtifactID: traceCausalProjectionArtifactBasename(path), ArtifactKind: "trace",
		}
		anchor := rn12Obs(prefix+"-root", "root_cause_primary", "root_cause_primary:x",
			"shadowhook-64305", "runnable_wait", "26.392", 26.392, 3, 9,
			"rank=1", "tier=primary", "chain_relevance=on_chain",
			"causality=on_wakeup_chain", "chain_depth=1", "dominant_state=runnable",
			cov4AnchorWindow)
		state := cov4StateRecord(prefix+"-state", "ease.app-63993", cov4AnchorWindow, 151.382)
		anchor.SourceRef, state.SourceRef = source, source
		records = append(records, anchor, state)
	}
	return ObservationLedger{Records: records}
}

func closedPartitionAuthorities(in []AnswerRelationAuthority) []AnswerRelationAuthority {
	var out []AnswerRelationAuthority
	for _, authority := range in {
		if authority.Kind == AnswerRelationAuthorityClosedPartition {
			out = append(out, authority)
		}
	}
	return out
}

func TestTwoArtifactIdenticalAccountingClaimCopiesValidateEndToEnd(t *testing.T) {
	ledger := twoArtifactIdenticalAccountingLedger("/tmp/a.systrace", "/tmp/b.systrace")
	set := CompileTraceCausalProjectionSet(ledger)
	if got := strings.Join(TraceCausalProjectionSetArtifactLabels(set), ","); got != "a.systrace,b.systrace" {
		t.Fatalf("fixture: want two labeled partitions, got %q", got)
	}

	// Handoff face: the exact compile the trace decision handoff prints
	// relation_claim_copy objects from.
	handoff := closedPartitionAuthorities(CompileTraceAnswerRelationAuthorities(set))
	if len(handoff) != 2 || handoff[0].ID != handoff[1].ID {
		t.Fatalf("fixture: identical accounting must yield two same-id authorities on the handoff face: %+v", handoff)
	}
	if handoff[0].ArtifactLabel != "a.systrace" || handoff[1].ArtifactLabel != "b.systrace" {
		t.Fatalf("handoff authorities must keep their trace files: %+v", handoff)
	}

	// Validation face: the exact slate both real validators compile. The two
	// faces must agree on the authority identity roster — the second trace
	// file's row survives the dedupe.
	validator := CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	identities := func(in []AnswerRelationAuthority) []string {
		var out []string
		for _, authority := range in {
			out = append(out, authority.ID+"\x00"+authority.ArtifactLabel)
		}
		return out
	}
	if !answerRelationSameSet(identities(handoff), identities(closedPartitionAuthorities(validator))) {
		t.Fatalf("handoff face and validation face disagree on authority identities:\nhandoff=%+v\nvalidator=%+v", handoff, validator)
	}

	// Both printed claim copies, copied verbatim as taught, validate together.
	claimA := AnswerRelationClaimForAuthority(handoff[0])
	claimB := AnswerRelationClaimForAuthority(handoff[1])
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claimA, claimB}, validator, false); err != nil {
		t.Fatalf("exact handoff copies from both trace files must validate end to end: %v", err)
	}

	// Claiming the SAME trace file's authority twice is still a duplicate,
	// named precisely.
	err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claimA, claimB, claimB}, validator, false)
	if err == nil || !strings.Contains(err.Error(), "duplicates authority_id") || !strings.Contains(err.Error(), `"b.systrace"`) {
		t.Fatalf("a per-trace-file duplicate must be rejected naming its trace file: %v", err)
	}

	// An unlabeled copy is unspecified and resolves to the first authority
	// under the id; pairing it with that authority's labeled copy is one
	// authority claimed twice, not two artifacts.
	unlabeled := claimA
	unlabeled.ArtifactLabel = ""
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claimA, unlabeled}, validator, false); err == nil ||
		!strings.Contains(err.Error(), "duplicates authority_id") {
		t.Fatalf("an unlabeled copy of an already-claimed authority must count as a duplicate: %v", err)
	}

	// Closure is judged per identity: acknowledging one trace file's
	// partition does not close the other's.
	err = ValidateAnswerRelationClaims([]AnswerRelationClaim{claimA}, validator, true)
	if err == nil || !strings.Contains(err.Error(), "missing required model-authored relation claim") ||
		!strings.Contains(err.Error(), `from trace file "b.systrace"`) {
		t.Fatalf("closure must require the second trace file's own claim: %v", err)
	}
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claimA, claimB}, validator, true); err != nil {
		t.Fatalf("both trace files acknowledged must close: %v", err)
	}
}

// Single-artifact behaviour is unchanged by the identity dedupe: one
// authority (carrying its V1-4 partition label), labeled and unlabeled copies
// both validate, and a repeat is a duplicate.
func TestSingleArtifactRelationAuthoritySlateUnchangedByIdentityDedupe(t *testing.T) {
	ledger := twoArtifactIdenticalAccountingLedger("/tmp/a.systrace")
	validator := closedPartitionAuthorities(CompileTraceAnswerRelationAuthoritiesFromLedger(ledger))
	if len(validator) != 1 || validator[0].ArtifactLabel != "a.systrace" {
		t.Fatalf("a single-artifact ledger compiles ONE closed-partition authority carrying its V1-4 label: %+v", validator)
	}
	claim := AnswerRelationClaimForAuthority(validator[0])
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim}, validator, true); err != nil {
		t.Fatalf("the exact copy must validate and close: %v", err)
	}
	unlabeled := claim
	unlabeled.ArtifactLabel = ""
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{unlabeled}, validator, true); err != nil {
		t.Fatalf("an unlabeled copy is unspecified, not wrong, and still closes: %v", err)
	}
	err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim, claim}, validator, false)
	if err == nil || !strings.Contains(err.Error(), "duplicates authority_id") {
		t.Fatalf("a repeated claim is still a duplicate: %v", err)
	}
}

// The dedupe itself keys on the (id, artifact_label) identity: two labeled
// same-id rows with distinct labels are two authorities; a same-label copy
// collapses; an unlabeled row is unspecified and is absorbed once the id is
// represented (the backfill direction is pinned by
// TestAnswerRelationAuthoritiesFromLedgerKeepPartitionKeyThroughDedupe).
func TestDedupeAnswerRelationAuthoritiesKeysOnAuthorityIdentity(t *testing.T) {
	got := dedupeAnswerRelationAuthorities([]AnswerRelationAuthority{
		{ID: "x", ArtifactLabel: "a.systrace"},
		{ID: "x", ArtifactLabel: "b.systrace"},
		{ID: "x", ArtifactLabel: "a.systrace"},
		{ID: "x"},
	})
	if len(got) != 2 || got[0].ArtifactLabel != "a.systrace" || got[1].ArtifactLabel != "b.systrace" {
		t.Fatalf("identity dedupe must keep one row per (id, artifact_label): %+v", got)
	}
}
