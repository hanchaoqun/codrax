package dataquery

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Answer-repair projection source selection and typed rule-declared
// output-field mapping for assemble_answer (DL-A, ledger §7.12 as-built
// ruling; specimen eval data_json_strict_ids-20260705-050016). Split
// out of action_runner.go under the DQA LOC ratchet.
//
// Projection priority ladder while the answer-repair lane is active
// (every rung consumes only typed signals; the decision table below is
// the as-built ruling — do not reorder without reopening ledger §7.12):
//
//	rung 1 — declared mapping first: when a unique, non-conflicting
//	         typed rule_coverage.output_field declaration exists AND the
//	         deterministic reconcile-group projection recomputes a JSON
//	         object whose single key equals it, that fresh recompute IS
//	         the answer. Payloads are never projected on this rung:
//	         content and lineage agree by construction.
//	rung 2 — payload fallback, only when rung 1 is unavailable.
//	         Admission (typed): custom_payload envelope, untruncated
//	         JSON sample, output contract satisfied, contract-format
//	         narrowing (JSON payloads are admissible ONLY under a
//	         json_only contract — a plain_single_line/csv_line contract
//	         never accepts a JSON payload), and the declared-schema
//	         guard (a payload contradicting a typed declaration is
//	         rejected on EVERY rank, anchor included). Ranking:
//	         freshness first (seeded round vs current invocation), then
//	         anchor > declared-key > bare-format, then later production
//	         position.
//	rung 3 — declaration present but no usable candidate: the contested
//	         lineage projection is published WITH a typed advisory trace
//	         (result ContractWarnings + artifact field); the fallback is
//	         never silent while a declaration exists.
//	rung 4 — no declaration and no candidate: fall through to the
//	         lineage projection exactly as outside the lane.
//
// The reconcile rewrite a rung-2 projection carries is coupled to
// publication: Run() publishes it only when the lane answer actually
// becomes result.Answer (a trailing custom_transform result that
// supplies its own answer keeps the pre-lane reconcile instead).

// AnswerRepairContext is the typed repair anchor consumed by
// assemble_answer source selection. Callers set Active only from typed
// evaluator fields (EvaluationHasActionableRepairTarget + normalized
// action_kind == assemble_answer); no prose is parsed on either side.
type AnswerRepairContext struct {
	Active bool
	// ActionID is the evaluator's typed action_id anchor. Within the
	// same freshness round, an artifact whose ID equals it ranks first
	// in repair source selection.
	ActionID string
}

// assembleProjectionAnswerArtifact is the typed value of the assemble
// artifact's "projection" field when the answer was projected from an
// answer-bearing payload under the repair lane. Run() consumes it to
// couple the lane's reconcile rewrite to actual answer publication.
const assembleProjectionAnswerArtifact = "answer_artifact"

// assembleDeclaredFieldUnprojectedKey is the typed artifact-field trace
// stamped on the assemble artifact when a declared output field existed
// but neither the declared mapping nor any payload candidate was usable
// (rung 3): the contested lineage projection was published under
// advisory, never silently.
const assembleDeclaredFieldUnprojectedKey = "declared_output_field_unprojected"

// answerRepairDeclaredFieldAdvisory is the rung-3 advisory published on
// result.ContractWarnings. Soft guidance by design: it steers the
// evaluator/planner toward materializing a projectable source instead
// of hard-failing a structurally valid (if contested) projection.
func answerRepairDeclaredFieldAdvisory(declaredField string) string {
	return fmt.Sprintf("assemble_answer: declared output field %q not projected: no deterministic reconcile-group mapping and no admissible answer-bearing payload candidate; contested lineage projection published under the answer-repair lane", declaredField)
}

// runAssembleAnswerFromRepairSource projects the final answer from a
// typed answer-bearing artifact while the answer-repair lane is active
// (rung 2 of the ladder above). Candidate admission and ranking consume
// only typed fields — artifact Kind/ID, the json_shape diagnostic, the
// payload sample, seeded-round freshness, the evaluator's typed
// action_id anchor, the rule-declared output field, and the output
// contract validator — never evaluator or planner prose. Returns
// ok=false to fall to the ladder's later rungs when no candidate
// qualifies. The reconcile rewrite in the returned report asserts the
// lane answer as the final answer; Run() must publish it only when this
// answer becomes result.Answer (publication coupling).
func (r ActionRunner) runAssembleAnswerFromRepairSource(action DataAction, reconcile *ReconcileReport, contract OutputContract, artifacts []DataArtifact, declaredField string) (DataArtifact, string, *ReconcileReport, bool) {
	source, answer, ok := r.selectAnswerBearingRepairSource(artifacts, contract, r.AnswerRepair.ActionID, declaredField)
	if !ok {
		return DataArtifact{}, "", nil, false
	}
	next := *reconcile
	next.ActualAnswer = LooseText(answer)
	next.ExpectedAnswer = LooseText(answer)
	next.Groups = append(withoutFinalAnswerProjectionGroups(reconcile.Groups), ReconcileGroup{
		GroupKey:   LooseText("final_answer"),
		Metric:     LooseText("projection"),
		Scope:      LooseText("final_answer"),
		Role:       LooseText("output"),
		Expected:   LooseText(answer),
		Actual:     LooseText(answer),
		Difference: LooseText("0"),
	})
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "assembled_answer")
	sourcePaths := append([]string{strings.TrimSpace(source.ID)}, assembleAnswerConsumedPaths(action, contract, assembleReferenceProjection{})...)
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionAssembleAnswer),
		SourcePaths: cleanStringListPreserveOrder(sourcePaths),
		Summary:     fmt.Sprintf("assembled final answer from answer-bearing artifact %s under the answer-repair lane", strings.TrimSpace(source.ID)),
		Fields: map[string]string{
			"group_count":            fmt.Sprintf("%d", len(reconcile.Groups)),
			"projection":             assembleProjectionAnswerArtifact,
			"repair_source_artifact": strings.TrimSpace(source.ID),
			"answer_len":             fmt.Sprintf("%d", len(answer)),
		},
	}, answer, &next, true
}

// selectAnswerBearingRepairSource picks the projection source for an
// answer repair among typed answer-bearing artifacts.
//
// Admission (all typed): the answerBearingArtifactPayload envelope
// checks, contract-format narrowing (payload candidates are JSON by
// construction, so they are admissible ONLY under a json_only output
// contract — under plain_single_line/csv_line a JSON payload would
// satisfy the line-shape validator while violating the requested output
// shape, so it never enters), the output-contract validator, and the
// declared-schema guard: when a typed rule declaration exists, a
// payload whose single object key does not equal it is rejected on
// every rank — the anchor rank included (projecting it would contradict
// the declared output schema).
//
// Ranking (typed priority order):
//
//	primary : freshness — current-invocation artifacts beat any seeded
//	          round; among seeded artifacts the newer producing round
//	          wins (SeedArtifactRounds, same-ID collisions already keep
//	          the newest copy upstream). A stale anchor-named payload
//	          therefore never suppresses a repair batch's fresh output.
//	tie 1   : anchor-named (ID == evaluator action_id) > declared-key
//	          match > bare contract-format match (the last admissible
//	          only when no typed declaration exists).
//	tie 2   : later production position wins (newest first, matching
//	          the freshness philosophy; the pre-ruling first-seen-wins
//	          tie is what let the oldest payload win permanently).
func (r ActionRunner) selectAnswerBearingRepairSource(artifacts []DataArtifact, contract OutputContract, anchorActionID, declaredField string) (DataArtifact, string, bool) {
	if contract.Normalize().Format != OutputJSONOnly {
		return DataArtifact{}, "", false
	}
	anchorActionID = strings.TrimSpace(anchorActionID)
	declaredField = strings.TrimSpace(declaredField)
	bestIdx := -1
	bestRank := 0
	bestFreshness := 0
	var bestArtifact DataArtifact
	bestAnswer := ""
	for i, artifact := range artifacts {
		payload, ok := answerBearingArtifactPayload(artifact)
		if !ok {
			continue
		}
		if err := ValidateAnswer(payload, contract); err != nil {
			continue
		}
		if declaredField != "" && !jsonObjectHasSingleKey(payload, declaredField) {
			// Declared-schema guard, every rank: a typed rule declaration
			// exists and this payload does not match it; projecting it
			// (anchor-named or not) would contradict the declared output
			// schema.
			continue
		}
		rank := 2
		switch {
		case anchorActionID != "" && strings.TrimSpace(artifact.ID) == anchorActionID:
			rank = 0
		case declaredField != "":
			rank = 1
		}
		freshness := r.artifactFreshness(i, artifact)
		better := bestIdx == -1 ||
			freshness > bestFreshness ||
			(freshness == bestFreshness && rank < bestRank) ||
			(freshness == bestFreshness && rank == bestRank && i > bestIdx)
		if better {
			bestIdx = i
			bestRank = rank
			bestFreshness = freshness
			bestArtifact = artifact
			bestAnswer = payload
		}
	}
	if bestIdx == -1 {
		return DataArtifact{}, "", false
	}
	return bestArtifact, bestAnswer, true
}

// artifactFreshness returns the typed freshness of an entry in the
// accumulated artifacts slice: entries produced by the current runner
// invocation (index at or beyond the seeded prefix) are strictly
// fresher than any seeded round; seeded entries carry the 1-based
// producing round recorded by the workflow seed merge (0 when unknown,
// i.e. oldest). Residual honesty: a current-invocation artifact that
// reuses a seeded ID is classified by its position, not the map, so it
// keeps current-round freshness.
func (r ActionRunner) artifactFreshness(index int, artifact DataArtifact) int {
	if index >= r.seedArtifactCount {
		return math.MaxInt
	}
	return r.SeedArtifactRounds[strings.TrimSpace(artifact.ID)]
}

// answerBearingArtifactPayload reports whether the artifact is a typed
// answer-bearing candidate and returns its payload. Admission is
// envelope-shaped: Kind==ArtifactKindCustomPayload, a non-empty
// json_shape diagnostic, and an untruncated single-row Sample holding
// valid JSON.
//
// Producer honesty (as-built): appendRunnerPayloadArtifact is the only
// SYSTEM producer of this kind and always stamps the constant ID
// ArtifactIDEmittedPayload — so the anchor rank (artifact ID equal to
// an evaluator action_id) is unreachable through the system producer
// and only fires for script-declared artifacts that set
// kind=custom_payload with their own IDs in result.artifacts. The kind
// is therefore NOT single-producer. The Sample payload is the
// producer's canonical JSON re-encoding (keys sorted, compacted,
// HTML-escaped by encoding/json), not the script's verbatim output
// bytes; what this projection publishes is that canonical re-encoding.
//
// Truncation is the single size-related admission blocker: the system
// producer truncates samples above 1200 bytes, so oversized payloads
// are never projectable through this lane. That residual is accepted —
// a truncated sample can no longer round-trip as JSON, and publishing a
// partial payload would be worse than falling to the later rungs.
func answerBearingArtifactPayload(artifact DataArtifact) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(artifact.Kind), ArtifactKindCustomPayload) {
		return "", false
	}
	if artifact.Fields == nil || strings.TrimSpace(artifact.Fields["json_shape"]) == "" {
		return "", false
	}
	if len(artifact.Sample) == 0 {
		return "", false
	}
	payload := strings.TrimSpace(artifact.Sample[0])
	if payload == "" || strings.HasSuffix(payload, "[truncated]") {
		return "", false
	}
	if !json.Valid([]byte(payload)) {
		return "", false
	}
	return payload, true
}

func jsonObjectHasSingleKey(payload, key string) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return false
	}
	if len(obj) != 1 {
		return false
	}
	_, ok := obj[key]
	return ok
}

// ruleDeclaredOutputField returns the single typed output field name
// declared across the rule coverage ledger, or "" when none or several
// distinct names are declared. Ambiguity yields "" on purpose: the
// deterministic field mapping must never guess between competing
// declarations, and prose-only rules (empty OutputField) contribute
// nothing (DL-A(b), ledger §7.12).
func ruleDeclaredOutputField(records []RuleCoverageRecord) string {
	declared := ""
	for _, rec := range records {
		field := strings.TrimSpace(rec.OutputField.String())
		if field == "" {
			continue
		}
		if declared != "" && declared != field {
			return ""
		}
		declared = field
	}
	return declared
}

func reconcileGroupsSingleMetricJSONList(groups []ReconcileGroup, action DataAction, valueField string, contributions []ContributionRecord, declaredField string) (string, []any, bool) {
	if len(groups) < 2 || reconcileJSONObjectExplicitKey(action) != "" {
		return "", nil, false
	}
	metric := singleReconcileMetric(groups)
	if metric == "" || !reconcileGroupsUseListProjectionOperations(groups, contributions) {
		return "", nil, false
	}
	key := pluralizeJSONFieldName(metric)
	// Typed rule-declared output schema wins over contribution-lineage
	// pluralization: this single-metric list projection emits exactly
	// one object key, so a rule-declared output field name maps onto it
	// deterministically (DL-A(b), ledger §7.12).
	if declared := strings.TrimSpace(declaredField); declared != "" {
		key = declared
	}
	if key == "" {
		return "", nil, false
	}
	values := make([]any, 0, len(groups))
	for _, group := range groups {
		for _, value := range reconcileJSONValueSlice(reconcileGroupJSONValueForAssemble(group, valueField, contributions)) {
			if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
				continue
			}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return "", nil, false
	}
	return key, values, true
}

func reconcileGroupJSONObjectKey(group ReconcileGroup, action DataAction, valueField string, groupCount int, declaredField string) string {
	if key := reconcileJSONObjectExplicitKey(action); key != "" {
		return key
	}
	// Single-group objects emit exactly one key, so a typed
	// rule-declared output field name maps onto it deterministically
	// and outranks every lineage-derived fallback below. Multi-group
	// objects keep per-group keys: one declared name cannot rename many
	// keys, so it is not applied there (no invention under ambiguity).
	if groupCount == 1 {
		if declared := strings.TrimSpace(declaredField); declared != "" {
			return declared
		}
	}
	if groupCount == 1 && !reconcileValueFieldIsStandard(valueField) {
		if key := pluralizeJSONFieldName(valueField); key != "" {
			return key
		}
	}
	if groupCount == 1 && reconcileGroupUsesSyntheticAllKey(group) && len(group.Values) > 0 {
		if key := pluralizeJSONFieldName(group.Metric.String()); key != "" {
			return key
		}
	}
	return firstNonEmptyString(strings.TrimSpace(group.GroupKey.String()), strings.TrimSpace(group.Metric.String()))
}
