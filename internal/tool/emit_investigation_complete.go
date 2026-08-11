package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tool/multipath"
	rmrelation "github.com/hanchaoqun/codrax/internal/tool/repomap/relation"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitInvestigationCompleteDowngradePrefix is the human-readable Summary prefix
// this tool writes when preCompleteContractCheck rejects a completion attempt.
// The tool returns Success=true so the LLM sees the explanation, while leaving
// MutableState.InvestigationComplete FALSE. Control-plane observers must use the
// typed MutableState flag rather than this display prefix to decide whether the
// investigation actually completed.
const EmitInvestigationCompleteDowngradePrefix = "emit_investigation_complete DOWNGRADED"

var callChainQualifiedCallRe = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:(?:\.|::)[A-Za-z_][A-Za-z0-9_]*)+\s*\(`)
var deterministicHistoryCountLineRe = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(?:answer_count|result_count|filtered_count|intersection_count|vcs_count|history_count)\s*[:=]\s*(-?\d+)(?:[^A-Za-z0-9_]|$)`)
var aggregateMemberVCSCommitHashBaseRe = regexp.MustCompile(`(?i)^[0-9a-f]{7,40}$`)

// EmitInvestigationComplete is the explorer's explicit completion
// signal. When the LLM has collected enough evidence to answer the
// user's question, it calls this tool to tell the system "move on
// to extraction and finalization". This replaces the implicit
// completion detection that relied on ShouldStop heuristics and
// soft-stop interception.
//
// The tool validates the declaration: confidence must be "high" or
// "medium" — a "low" confidence call is rejected so the LLM
// continues investigating instead of prematurely stopping.
//
// On success, the tool writes a flag on MutableState that the
// explorer's ShouldStop reads to terminate the ReAct loop, and that
// ParseOutput reads to set HasEnoughFacts.
type EmitInvestigationComplete struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitInvestigationComplete) Name() string { return "emit_investigation_complete" }

func (t *EmitInvestigationComplete) Description() string {
	return "Signal that the investigation is complete and the system should " +
		"move to the extraction and finalization stages. Call this ONCE when " +
		"you have collected enough evidence to answer the user's question. " +
		"Do NOT call this if you still have files to read or hypotheses to verify. " +
		"Requires a reason that carries the concise terminal conclusion for later " +
		"answer writing, plus a confidence level (high or medium)."
}

var (
	emitInvestigationCompleteParametersOnce   sync.Once
	emitInvestigationCompleteParametersCached json.RawMessage
)

func (t *EmitInvestigationComplete) Parameters() json.RawMessage {
	emitInvestigationCompleteParametersOnce.Do(func() {
		// §21 EMIT-2 / 维度C④: the aggregate_facts cap is single-sourced from
		// types.MaxAnswerAggregateFacts via the __AGG_FACTS_CAP__ placeholder
		// (both the maxItems constraint and the description pre-announcement),
		// so the schema promise can never drift from the validator.
		emitInvestigationCompleteParametersCached = json.RawMessage(strings.ReplaceAll(`{
		"type": "object",
		"properties": {
			"reason": {
				"type": "string",
				"description": "Concise completion conclusion for later answer writing: state what the investigation found, why it is complete, and any important scope boundary, no-hit/exclusion finding, cross-repository or cross-component distinction, or caveat that should not be lost. Do not leave the conclusion only in free-form text before the tool call. Keep counts, complete member lists, and per-bucket facts in aggregate_facts; use absence_justification for a genuine zero or not-found result. This field is preserved as context, not as a citation. For external runtime/log/trace artifacts, keep direct observations separate from inferred upstream causes: the artifact can directly prove the error message, observed operation/property, frame/span, signal, duration, and trace order. It does not by itself prove which variable/parameter/caller supplied the bad value or how upstream data was constructed; put that as a possible upstream investigation direction unless the artifact text or separately grounded current-source evidence proves it."
			},
			"confidence": {
				"type": "string",
				"enum": ["high", "medium"],
				"description": "Your confidence that the collected evidence is sufficient. 'low' is not accepted — continue investigating instead."
			},
			"result_kind": {
				"type": "string",
				"enum": ["resolved", "absence"],
				"description": "Structured terminal disposition for this investigation. Use 'resolved' for ordinary positive/citable answers. Use 'absence' only when the honest terminal answer is zero / no-such-target / nothing found and you are also providing absence_justification."
			},
			"absence_justification": {
				"type": "string",
				"description": "OPTIONAL. Set this ONLY when the answer is an honest 'zero' / 'no X' / 'nothing found' that has no direct exact-target definition to cite (e.g. 'how many .py files?' answered 0, 'does handler X exist?' answered no). A single short sentence explaining why the answer is genuinely empty. Leave unset for every non-absence answer. Grounded related-context anchors are allowed when they remain clearly contextual (for example a nearby config family, call chain, or architecture edge) and do not define the missing exact target. This is a declarative claim, not a system override: the framework still audits that at least one investigation-class tool (grep / exec_command / list_files / read_file / repo_map) ran successfully before accepting the waiver."
			},
			"aggregate_facts": {
				"type": "array",
				"maxItems": __AGG_FACTS_CAP__,
				"description": "OPTIONAL but expected for derived scalar/count answers, categorical behavior verdicts, exhaustive member enumerations, and named-mechanism comparisons. HARD CAP: at most __AGG_FACTS_CAP__ entries per call — budget them toward the highest-value facts. When a scalar comparison or multi-group investigation would exceed the cap, merge same-family per-group scalars into ONE grouped_count fact with members (one fact per metric family, not one per group or per trace side), and keep audit-only bookkeeping details in reason instead of separate audit_ledger entries. A named-mechanism comparison is different: use ONE principal member_set with one mechanism per members[] entry, its own index-aligned member_notes[] control-path description, and its own index-aligned support_refs[] evidence location. Do not place a compared mechanism only in grouped_count dimensions: dimensions have no member-specific evidence mapping, so that shape cannot prove both sides independently. A carrier-only enum, constant, type, schema, or event-name declaration proves identity only; behavioral member_notes require the actual producer/callsite and consumer/handler control path. For an attached runtime event, a source predicate proves the rule exists but not that this instance satisfied it: bind every load-bearing runtime operand or mark the rule uninstantiated. A payload over the cap is truncated by role priority (principal_answer kept first, audit_ledger dropped first) when the principal_answer facts themselves fit the cap, and rejected otherwise. Model-authored structured aggregate facts discovered during investigation: total counts, unique-set counts, per-group counts, per-user-bucket counts, exact member sets, scalar durations/latencies/frequencies, behavior outcomes, and excluded-candidate counts. Use this instead of burying aggregates, verdicts, or complete member lists only in reason prose. Count values must be non-negative integer strings with units kept in unit. Durations such as 119.227 ms, latency, percentage, frequency, and other non-integer measurements must use kind=scalar_value, not total_count. behavior_outcome/error_granularity_verdict values are stable category strings. For kind=member_set, value may be omitted when members contains a non-empty complete set; a verified empty set uses value=\"0\" and members=[]. When value is an explicit valid integer, it MUST equal len(members): a numeric mismatch is rejected because it means either the count or the purported exact roster is wrong. Schema-adjacent non-integer value text such as \"1+\" may still be repaired from exact members. Values must come from your verified tool output or structured evidence; this handoff is preserved downstream and no value is inferred from raw evidence.",
				"items": {
					"type": "object",
					"properties": {
						"kind": {
							"type": "string",
							"enum": ["total_count", "unique_count", "grouped_count", "bucket_count", "excluded_count", "scalar_value", "member_set", "negative_search", "negative_observation", "behavior_outcome", "error_granularity_verdict"],
							"description": "total_count = total principal hits or a typed pre-filter candidate pool when dimensions name that stage; unique_count = size of a distinct set such as unique files; grouped_count = count for a syntax/category/dimension group; bucket_count = count for one user-named bucket; excluded_count = non-counted candidate bucket; scalar_value = other derived scalar; member_set = exact exhaustive principal member list whose value equals len(members), including value=\"0\" with members=[] for a verified empty set; behavior_outcome = a categorical code behavior result such as accepted/rejected/partial_success/fail_fast/collect_errors; error_granularity_verdict = the categorical failure-scope verdict for item/batch/transaction behavior questions. behavior_outcome and error_granularity_verdict use value as a stable category string, not a number; put compared axes in dimensions and support in support_refs. negative_search = a verified zero-result repository search. For negative_search, set value=\"0\", unit=\"matches\", and dimensions for repo, query or pattern, scope, and searched_at; do NOT fake a file:line evidence item such as repo:0 for not-found proof. negative_observation = a verified zero-result observation over a non-repo source such as git history/diff output, an attached log, a trace, command output, or repo-map/index output. For negative_observation, set value=\"0\", unit=\"matches\", include origin/evidence_origin, target/query/pattern/predicate or a runtime carrier such as missing_signal/missing_event/missing_field, scope, and searched_at; do NOT invent repo dimensions."
						},
						"label": {
							"type": "string",
							"description": "User-visible name of this aggregate, e.g. production assignment locations, unique files, direct assignments, struct literal initializers, CLI bucket."
						},
						"value": {
							"type": "string",
							"description": "Exact value to preserve. For count kinds use a non-negative integer string such as \"0\", \"3\", or \"12\"; put words like files/locations in unit. For durations/latencies/percentages/frequencies and other non-integer measurements, use kind=scalar_value with unit such as ms, s, %, or kHz. For behavior_outcome/error_granularity_verdict use a stable category string such as \"per_item_rejection\", \"whole_batch_failure\", \"partial_success\", \"fail_fast\", or \"collect_errors\". For kind=negative_search or kind=negative_observation, value must be \"0\". For kind=member_set this may be omitted when members is the complete exact set; for grouped_count/bucket_count it may also be omitted when members is the exact group/bucket member set. total_count/unique_count require explicit value because their members can be examples."
						},
						"role": {
							"type": "string",
							"enum": ["principal_answer", "supporting_coverage", "audit_ledger"],
							"description": "Optional typed role. principal_answer means this aggregate is the answer slate/value the answer-writing step must preserve visibly. supporting_coverage means it backs another principal answer such as a scalar count or mechanism explanation and must not force duplicate visible rows. audit_ledger means it is investigation bookkeeping only. Omit when unsure; the framework infers the legacy role from the typed request shape."
						},
						"provenance": {
							"type": "string",
							"description": "Optional short structured source note for operators, e.g. emit_investigation_complete.aggregate_facts, rg_count, deterministic_count_enrichment, or command:rg. Do not include this string in the user-visible final answer."
						},
						"unit": {
							"type": "string",
							"description": "Optional unit such as locations, files, functions, packages, buckets, candidates."
						},
						"dimensions": {
							"type": "array",
							"description": "Optional typed axes that make the aggregate precise, e.g. [{name:\"scope\", value:\"production\"}, {name:\"syntax\", value:\"struct_literal\"}, {name:\"stage\", value:\"candidate_pre_filter\"}, {name:\"basis\", value:\"verified_member_set\"}]. For kind=negative_search, include repo, query or pattern, scope, and searched_at. For kind=negative_observation, include origin/evidence_origin plus target, query, pattern, predicate, or runtime carriers such as missing_signal/missing_event/missing_field; include scope for the bounded observed surface, such as a commit range, tool result, artifact id, or trace window. Use searched_at=\"current_investigation\" when no wall-clock timestamp is available. If a broad candidate search count differs from the final verified member_set, emit companion total_count/excluded_count facts with dimensions that explain the stage/basis instead of burying the discrepancy in prose.",
							"items": {
								"type": "object",
								"properties": {
									"name":  {"type": "string"},
									"value": {"type": "string"}
								},
								"required": ["name", "value"]
							}
						},
						"members": {
							"type": "array",
							"description": "Optional exact principal members backing the aggregate, such as enum/type names, file:line labels for total_count, or file paths for unique_count. For member_set this must be the complete answer member set: list EVERY member by name and keep value equal to len(members) — a count-only member_set (non-zero value with members missing) is rejected, so never hand over just the count. Use an empty array only for a verified empty set with value=\"0\" and typed negative/no-hit support. If you provide members for a count fact, the member count must equal value; omit members rather than provide samples.",
							"items": {"type": "string"}
						},
						"member_notes": {
							"type": "array",
							"description": "Optional per-member explanatory notes aligned by index with members[]. Use this only for verified role/rationale/meaning that should make the final answer less dry; it does not define member identity and must not replace support_refs or evidence. For a current-source architecture/mechanism roster used to close the investigation, either omit member_notes entirely or provide exactly one non-empty note AND one grounded support_ref for every members[] entry, all in the same order; partial responsibility rosters are rejected instead of being presented as proved. When the typed request contract requires a current-source per-member attribute table, omission is not an exit: provide one verified non-empty member_note and one same-member grounded support_ref for every row so the requested row attributes reach the final answer instead of being inferred from identity alone.",
							"items": {"type": "string"}
						},
						"excluded": {
							"type": "array",
							"description": "Optional excluded candidate labels for excluded_count or for explaining why a total is bounded.",
							"items": {"type": "string"}
						},
						"support_refs": {
							"type": "array",
							"description": "Optional evidence ids, file:line labels, command labels, or member-specific refs like \"Member: file:line\" or \"Member @ file:line\" that back this aggregate. For current-source principal member_set rows, add one already-read grounded location per members[] entry whenever members are code/source identities, routes, config keys, operation sites, or display aliases that may not exactly match an emitted evidence anchor. POSITIONAL DECISION: positional refs are exactly one ref per member, so len(support_refs)==len(members), in the same order; choose the bounded source span that proves the corresponding member_notes row, not an identity-only declaration when the note claims behavior or responsibility. Never append a second parallel batch of bare refs. Labeled refs [\"<leading-symbol>: <file>:<line>\", ...] are accepted when the label is the member's bare leading identifier and a non-positional mapping is genuinely required. Write each ref bare: no wrapping backticks or quotes, no trailing parenthetical annotation glued to the ref, and no free-prose note appended after it — explanatory text belongs in member_notes. REQUIRED for kind=member_set when ANY current-source member is shaped \"<code identifier> (<qualifier>)\" — for example \"Orchestrator (4-stage pipeline)\" or \"Gate.Run (8 checks)\" — because the decorator changes the surface text so the member never auto-resolves against an evidence anchor named just \"Orchestrator\" / \"Gate.Run\". member_notes are explanatory row notes; they do not ground or identify members and do not replace support_refs. Observation-only runtime artifacts and VCS commit members do not need repo file:line support_refs; keep them in their typed aggregate/provenance lane. Bare members can omit support_refs only when the visible member is already a verbatim emitted evidence/answer-symbol anchor or a display-only non-source item.",
							"items": {"type": "string"}
						}
					},
					"required": ["kind", "label"]
				}
			},
			"relation_claims": {
				"type": "array",
				"maxItems": 16,
				"description": "OPTIONAL; normally omit it because typed trace relation authorities are carried into final synthesis automatically. If structured metadata is useful, copy only a complete relation_claim_copy JSON object printed by trace_query. Never copy relation_diagnostic_only fields such as member_values, measured_envelope_overlap, comparison_rule, comparison_value, fix_direction, or chain_lane: they are reasoning context and are not fields in this schema. Omission does not block closure. Submitted claims are validated only against typed trace carriers; the framework does not scan or rewrite reason.",
				"items": {
					"type": "object",
					"properties": {
						"authority_id": {"type": "string"},
						"member_refs": {"type": "array", "items": {"type": "string"}},
						"physical_relation": {"type": "string", "enum": ["unresolved", "mutually_exclusive", "overlap", "contains", "contained_by"]},
						"addition": {"type": "string", "enum": ["authorized_to_published_subtotal", "forbidden"]},
						"subtotal_value": {"type": "number"},
						"subtotal_unit": {"type": "string"}
					},
					"required": ["authority_id", "member_refs", "physical_relation", "addition"]
				}
			},
			"evidence_floor_waiver": {
				"type": "object",
				"description": "OPTIONAL. The typed escape lane for declaring that ordinary repo-grounding requirements do NOT apply to this investigation. Set this when YOU have confidently determined that pretending to ground against repo code would be misleading — for example: the attached log is from a different system whose paths have no intersection with this repo (a customer-pasted panic from another service); a synthetic-looking log whose frame paths superficially match this repo but represent a different build / version / deployment; the input is informational with no failure component to ground against. A waiver is honored only when both reason and rationale are valid; incomplete or invalid waiver payloads are ignored and normal grounding gates still apply. Every accepted or ignored fire is logged so a reviewer can spot misuse. Do NOT set this to escape ordinary investigation work — only when the model's reading of the input materially conflicts with the system's default to-look-in-repo posture.",
				"properties": {
					"reason": {
						"type": "string",
						"enum": ["external_only_log", "external_only_trace", "no_repo_intersection", "informational_runtime_only"],
						"description": "external_only_log = attached log is from a system whose code is NOT in this repo (canonical: customer-pasted panic from another service). external_only_trace = same for trace. no_repo_intersection = stronger general case: any apparent path overlap is coincidental (e.g. a synthetic log whose frames happen to name current-repo paths but represent a different build). informational_runtime_only = input is debug breadcrumb / clean baseline / performance observation with no failure to ground."
					},
					"rationale": {
						"type": "string",
						"description": "One sentence explaining why repo grounding does not apply. Required for the waiver to take effect — a waiver with a blank rationale is ignored and normal grounding gates still apply. The text is carried forward and shown to the answer-writing step verbatim as the stated justification, so make it self-contained and concrete (name the observed mismatch). The reason enum, not this text, selects which requirement is relaxed; every fire is also logged for review."
					}
				}
			},
			"clear_evidence_floor_waiver": {
				"type": "boolean",
				"description": "OPTIONAL. Set true only to explicitly retract a previously declared evidence_floor_waiver after new investigation shows repo grounding DOES apply. Do not set this together with evidence_floor_waiver."
			},
			"principal_span_waiver": {
				"type": "object",
				"description": "OPTIONAL. The typed escape lane for the principal source→sink span/reachability gates. Set this only after YOU read the source and establish one finite boundary: no separately-citable intermediate user code, a runtime/external bridge, or no directed static path between the exact endpoints (in which case preserve the nearest proven path and endpoint boundary instead of inventing an edge). A waiver is honored only when both reason and rationale are valid; incomplete or invalid payloads are ignored and normal gates still apply. Every accepted or ignored fire is logged for review.",
				"properties": {
					"reason": {
						"type": "string",
						"enum": ["endpoints_directly_adjacent", "no_intermediate_user_code", "platform_bridge_intermediates", "inlined_call", "runtime_dispatched_call", "external_module_continuation", "no_directed_path"],
						"description": "endpoints_directly_adjacent = source and sink are on one dispatch/wrapper statement. no_intermediate_user_code = only plumbing lies between them. platform_bridge_intermediates = the hop is an FFI/JNI/cgo/cross-language runtime bridge. inlined_call = compiler/JIT inlining removes a separate frame. runtime_dispatched_call = interface/virtual/closure/reflection dispatch has no static edge. external_module_continuation = intermediates live outside this repo. no_directed_path = both exact endpoints were inspected but accepted typed call edges do not reach the sink from the source; before completion emit every observed direct reverse/parallel call as its own typed call evidence row, then report the nearest proven path and endpoint boundary separately."
					},
					"rationale": {
						"type": "string",
						"description": "One sentence explaining which scenario applies in concrete terms (e.g. \"dispatch and handler are on the same statement at foo.go:42\" / \"intermediate is the cgo trampoline at frame N\"). Required for the waiver to take effect — a waiver with a blank rationale is ignored and normal span gates still apply. The reason enum, not this text, selects which requirement is relaxed; every fire is logged for review."
					}
				}
			},
			"clear_principal_span_waiver": {
				"type": "boolean",
				"description": "OPTIONAL. Set true only to explicitly retract a previously declared principal_span_waiver after new investigation shows intermediate evidence DOES exist and can be emitted. Do not set this together with principal_span_waiver."
			}
			},
			"required": ["reason", "confidence", "result_kind"]
		}`, "__AGG_FACTS_CAP__", strconv.Itoa(types.MaxAnswerAggregateFacts)))
	})
	return cloneJSONRawMessage(emitInvestigationCompleteParametersCached)
}

type emitInvestigationCompleteWaiverPayload struct {
	Reason    string `json:"reason"`
	Rationale string `json:"rationale"`
}

// joinEvidenceFloorWaiverReasons returns the back-quoted enum list
// for retry-hint construction. Sourced from
// types.EvidenceFloorWaiverReasonValues so the schema description,
// validator error message, and any future surface stay byte-canonical.
func joinEvidenceFloorWaiverReasons() string {
	values := types.EvidenceFloorWaiverReasonValues()
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, "`"+string(v)+"`")
	}
	return strings.Join(parts, ", ")
}

// joinPrincipalSpanWaiverReasons returns the back-quoted enum list
// for retry-hint construction. Sourced from
// types.PrincipalSpanWaiverReasonValues so the schema enum, the
// validator error message, and the skill-prompt teaching text stay
// byte-canonical (one update site for any new reason value).
func joinPrincipalSpanWaiverReasons() string {
	values := types.PrincipalSpanWaiverReasonValues()
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, "`"+string(v)+"`")
	}
	return strings.Join(parts, ", ")
}

type emitInvestigationCompleteParams struct {
	Reason                   string                                  `json:"reason"`
	Confidence               string                                  `json:"confidence"`
	ResultKind               string                                  `json:"result_kind"`
	AbsenceJustification     string                                  `json:"absence_justification,omitempty"`
	AggregateFacts           []types.AnswerAggregateFact             `json:"aggregate_facts,omitempty"`
	RelationClaims           []types.AnswerRelationClaim             `json:"relation_claims,omitempty"`
	EvidenceFloorWaiver      *emitInvestigationCompleteWaiverPayload `json:"evidence_floor_waiver,omitempty"`
	ClearEvidenceWaiver      bool                                    `json:"clear_evidence_floor_waiver,omitempty"`
	PrincipalSpanWaiver      *emitInvestigationCompleteWaiverPayload `json:"principal_span_waiver,omitempty"`
	ClearPrincipalSpanWaiver bool                                    `json:"clear_principal_span_waiver,omitempty"`
}

type emitInvestigationCompleteRawParams struct {
	Reason                   string                                  `json:"reason"`
	Confidence               string                                  `json:"confidence"`
	ResultKind               string                                  `json:"result_kind"`
	AbsenceJustification     string                                  `json:"absence_justification,omitempty"`
	AggregateFacts           json.RawMessage                         `json:"aggregate_facts,omitempty"`
	RelationClaims           []types.AnswerRelationClaim             `json:"relation_claims,omitempty"`
	EvidenceFloorWaiver      *emitInvestigationCompleteWaiverPayload `json:"evidence_floor_waiver,omitempty"`
	ClearEvidenceWaiver      bool                                    `json:"clear_evidence_floor_waiver,omitempty"`
	PrincipalSpanWaiver      *emitInvestigationCompleteWaiverPayload `json:"principal_span_waiver,omitempty"`
	ClearPrincipalSpanWaiver bool                                    `json:"clear_principal_span_waiver,omitempty"`
}

func (p *emitInvestigationCompleteParams) UnmarshalJSON(data []byte) error {
	var raw emitInvestigationCompleteRawParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return p.loadFromRaw(raw)
}

func (p *emitInvestigationCompleteParams) loadFromRaw(raw emitInvestigationCompleteRawParams) error {
	var reasonMisplaced map[string]json.RawMessage
	if cleanReason, fields, ok := extractMisplacedCompletionFieldsFromReason(raw.Reason); ok {
		raw.Reason = cleanReason
		reasonMisplaced = fields
		if len(strings.TrimSpace(string(raw.AggregateFacts))) == 0 {
			if recoveredFacts := fields["aggregate_facts"]; len(recoveredFacts) > 0 {
				raw.AggregateFacts = recoveredFacts
			}
		}
	}
	facts, misplaced, aggregateFactsStringRecovered, err := decodeAggregateFactsPayload(raw.AggregateFacts)
	if err != nil {
		return err
	}
	if aggregateFactsStringRecovered {
		logging.Warning("[emit_investigation_complete] aggregate_facts arrived as JSON-encoded string; re-parsed losslessly")
	}
	misplaced = mergeMisplacedCompletionFields(misplaced, reasonMisplaced)
	if err := recoverMisplacedCompletionWaiverFields(&raw, misplaced); err != nil {
		return err
	}
	relationClaims := types.CloneAnswerRelationClaims(raw.RelationClaims)
	if len(relationClaims) == 0 {
		if recovered := misplaced["relation_claims"]; len(recovered) > 0 {
			if err := decodeStrictJSONValue(recovered, &relationClaims); err != nil {
				return fmt.Errorf("decode misplaced relation_claims: %w", err)
			}
		}
	}
	if raw.Reason == "" {
		raw.Reason = decodeMisplacedStringField(misplaced, "reason")
	}
	if !completionConfidenceValid(raw.Confidence) {
		raw.Confidence = decodeMisplacedStringField(misplaced, "confidence")
	}
	if !completionResultKindValid(raw.ResultKind) {
		raw.ResultKind = decodeMisplacedStringField(misplaced, "result_kind")
	}
	if raw.AbsenceJustification == "" && strings.EqualFold(strings.TrimSpace(raw.ResultKind), "absence") {
		raw.AbsenceJustification = decodeMisplacedStringField(misplaced, "absence_justification")
	}
	*p = emitInvestigationCompleteParams{
		Reason:                   raw.Reason,
		Confidence:               raw.Confidence,
		ResultKind:               raw.ResultKind,
		AbsenceJustification:     raw.AbsenceJustification,
		AggregateFacts:           facts,
		RelationClaims:           relationClaims,
		EvidenceFloorWaiver:      raw.EvidenceFloorWaiver,
		ClearEvidenceWaiver:      raw.ClearEvidenceWaiver,
		PrincipalSpanWaiver:      raw.PrincipalSpanWaiver,
		ClearPrincipalSpanWaiver: raw.ClearPrincipalSpanWaiver,
	}
	return nil
}

// recoverMisplacedCompletionWaiverFields completes the same compatibility
// lane used for string-wrapped aggregate_facts sibling tails. Some providers
// serialize the array and following schema-owned fields into one string. The
// leading array decoder already separates that tail; leaving typed waivers
// behind while recovering aggregate facts makes the completion gate ask for a
// waiver that was present in the very payload it consumed.
//
// A top-level member wins for its whole waiver family. When neither top-level
// member is present, recover both the declaration and clear flag so the normal
// mutual-exclusion and enum/rationale checks below remain the single authority.
func recoverMisplacedCompletionWaiverFields(raw *emitInvestigationCompleteRawParams, fields map[string]json.RawMessage) error {
	if raw == nil || len(fields) == 0 {
		return nil
	}
	if raw.EvidenceFloorWaiver == nil && !raw.ClearEvidenceWaiver {
		waiver, clear, err := decodeMisplacedCompletionWaiverFamily(fields, "evidence_floor_waiver", "clear_evidence_floor_waiver")
		if err != nil {
			return err
		}
		raw.EvidenceFloorWaiver = waiver
		raw.ClearEvidenceWaiver = clear
	}
	if raw.PrincipalSpanWaiver == nil && !raw.ClearPrincipalSpanWaiver {
		waiver, clear, err := decodeMisplacedCompletionWaiverFamily(fields, "principal_span_waiver", "clear_principal_span_waiver")
		if err != nil {
			return err
		}
		raw.PrincipalSpanWaiver = waiver
		raw.ClearPrincipalSpanWaiver = clear
	}
	return nil
}

func decodeMisplacedCompletionWaiverFamily(fields map[string]json.RawMessage, waiverKey, clearKey string) (*emitInvestigationCompleteWaiverPayload, bool, error) {
	var waiver *emitInvestigationCompleteWaiverPayload
	if encoded := fields[waiverKey]; len(encoded) > 0 {
		var decoded emitInvestigationCompleteWaiverPayload
		if err := decodeStrictJSONValue(encoded, &decoded); err != nil {
			return nil, false, fmt.Errorf("decode misplaced %s: %w", waiverKey, err)
		}
		waiver = &decoded
	}
	clear := false
	if encoded := fields[clearKey]; len(encoded) > 0 {
		if err := decodeStrictJSONValue(encoded, &clear); err != nil {
			return nil, false, fmt.Errorf("decode misplaced %s: %w", clearKey, err)
		}
	}
	return waiver, clear, nil
}

func decodeAggregateFactsPayload(raw json.RawMessage) ([]types.AnswerAggregateFact, map[string]json.RawMessage, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil, false, nil
	}
	var payload []byte
	var misplaced map[string]json.RawMessage
	stringRecovered := false
	if strings.HasPrefix(trimmed, `"`) {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, nil, false, err
		}
		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return nil, nil, false, nil
		}
		if repaired, changed := toolparam.RemoveTrailingCommasBeforeJSONClosers(encoded); changed {
			encoded = repaired
		}
		arrayPayload, tail, ok := splitLeadingJSONArrayWithMisplacedObjectTail(encoded)
		if !ok {
			return nil, nil, false, fmt.Errorf("aggregate_facts string must contain a JSON array")
		}
		payload = arrayPayload
		misplaced = tail
		stringRecovered = true
	} else {
		payload = raw
	}
	payload = normalizeAggregateFactsPayloadCompat(payload)
	var facts []types.AnswerAggregateFact
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&facts); err != nil {
		// EMITBURN-1 (§29.173): mark entry-array strict-decode failures with a
		// typed wrapper carrying the post-compat payload bytes, so the reject
		// builder can walk EVERY entry for the accumulated report instead of
		// stopping at Go's first decoder error. Error() text is unchanged.
		return nil, nil, false, &aggregateFactsEntryDecodeError{err: err, payload: payload}
	}
	return facts, misplaced, stringRecovered, nil
}

// aggregateFactsEntryDecodeError marks a strict-decode failure INSIDE the
// aggregate_facts entry array (as opposed to the top-level parameter decode).
// It preserves the original decoder error text and chain (Unwrap) and carries
// the post-compat payload so the reject message can re-walk entries leniently.
// Reporting-shape only: the reject decision and lane are unchanged.
type aggregateFactsEntryDecodeError struct {
	err     error
	payload []byte
}

func (e *aggregateFactsEntryDecodeError) Error() string { return e.err.Error() }

func (e *aggregateFactsEntryDecodeError) Unwrap() error { return e.err }

func normalizeAggregateFactsPayloadCompat(payload []byte) []byte {
	var rows []map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	if err := dec.Decode(&rows); err != nil {
		return payload
	}
	changed := false
	for _, row := range rows {
		if normalizeAggregateFactObjectCompat(row) {
			changed = true
		}
	}
	if !changed {
		return payload
	}
	out, err := json.Marshal(rows)
	if err != nil {
		return payload
	}
	return out
}

func normalizeAggregateFactObjectCompat(row map[string]json.RawMessage) bool {
	if len(row) == 0 {
		return false
	}
	kind := strings.TrimSpace(decodeAggregateFactCompatString(row["kind"]))
	var dims []types.AnswerAggregateDimension
	if raw := row["dimensions"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &dims)
	}
	changed := false
	for key, dimName := range aggregateFactTopLevelDimensionAliases(kind) {
		raw, ok := row[key]
		if !ok {
			continue
		}
		value, ok := aggregateFactCompatScalarString(raw)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if !aggregateFactCompatDimensionCovered(dims, dimName) {
			dims = append(dims, types.AnswerAggregateDimension{Name: dimName, Value: value})
		}
		delete(row, key)
		changed = true
	}
	if !changed {
		return false
	}
	if len(dims) > 0 {
		if raw, err := json.Marshal(dims); err == nil {
			row["dimensions"] = raw
		}
	}
	return true
}

func aggregateFactTopLevelDimensionAliases(kind string) map[string]string {
	common := map[string]string{
		"origin":          "origin",
		"evidence_origin": "evidence_origin",
		"source_origin":   "origin",
		"source":          "source_ref",
		"source_ref":      "source_ref",
		"tool":            "source_ref",
		"producer":        "source_ref",
		"target":          "target",
		"query":           "query",
		"pattern":         "pattern",
		"predicate":       "predicate",
		"scope":           "scope",
		"window":          "scope",
		"range":           "scope",
		"artifact_id":     "artifact_id",
		"trace_window":    "trace_window",
		"commit_range":    "commit_range",
		"tool_result":     "tool_result",
		"searched_at":     "searched_at",
		"searched_in":     "searched_at",
		"result_count":    "result_count",
		"count":           "result_count",
		"matches":         "result_count",
		"match_count":     "result_count",
	}
	if kind == string(types.AnswerAggregateNegativeObservation) {
		common["checked_types"] = "target"
		common["checked_type"] = "target"
		common["absent_types"] = "target"
		common["absent_type"] = "target"
		common["checked_events"] = "target"
		common["event_types"] = "target"
		common["observed_types"] = "target"
		common["missing_signal"] = "target"
		common["missing_signals"] = "target"
		common["absent_signal"] = "target"
		common["absent_signals"] = "target"
		common["missing_field"] = "target"
		common["missing_fields"] = "target"
		common["absent_field"] = "target"
		common["absent_fields"] = "target"
		common["missing_event"] = "target"
		common["missing_events"] = "target"
		common["absent_event"] = "target"
		common["absent_events"] = "target"
		common["missing_type"] = "target"
		common["missing_types"] = "target"
	}
	return common
}

func aggregateFactCompatDimensionCovered(dims []types.AnswerAggregateDimension, name string) bool {
	name = normalizeAggregateFactCompatDimensionKey(name)
	if name == "" {
		return false
	}
	for _, dim := range dims {
		if normalizeAggregateFactCompatDimensionKey(dim.Name) == name {
			return true
		}
	}
	return false
}

func normalizeAggregateFactCompatDimensionKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	switch key {
	case "window", "range", "search_scope", "observed_scope":
		return "scope"
	case "artifact", "log", "log_id", "trace", "trace_id":
		return "artifact_id"
	case "span_window", "time_window":
		return "trace_window"
	case "git_range", "revision_range", "history_window":
		return "commit_range"
	case "tool_result_id", "command_id", "command_result":
		return "tool_result"
	case "matches", "match_count", "count", "observed_count":
		return "result_count"
	case "checked_types", "checked_type",
		"absent_types", "absent_type",
		"checked_events", "event_types", "observed_types",
		"missing_signal", "missing_signals",
		"absent_signal", "absent_signals",
		"missing_field", "missing_fields",
		"absent_field", "absent_fields",
		"missing_event", "missing_events",
		"absent_event", "absent_events",
		"missing_type", "missing_types":
		return "target"
	case "source", "tool", "producer":
		return "source_ref"
	case "source_origin":
		return "origin"
	default:
		return key
	}
}

func decodeAggregateFactCompatString(raw json.RawMessage) string {
	value, ok := aggregateFactCompatScalarString(raw)
	if !ok {
		return ""
	}
	return value
}

func aggregateFactCompatScalarString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", false
	}
	return aggregateFactCompatValueString(value)
}

func aggregateFactCompatValueString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v), true
	case json.Number:
		return strings.TrimSpace(v.String()), true
	case bool:
		return strconv.FormatBool(v), true
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			part, ok := aggregateFactCompatValueString(item)
			if !ok || strings.TrimSpace(part) == "" {
				continue
			}
			parts = append(parts, strings.TrimSpace(part))
		}
		if len(parts) == 0 {
			return "", false
		}
		return strings.Join(parts, ", "), true
	default:
		return "", false
	}
}

var completionReasonFieldStartRe = regexp.MustCompile(`(?m)(?:^|[\n,;]\s*)("?(confidence|result_kind|aggregate_facts|absence_justification)"?\s*[:=])`)
var completionReasonBareKeyRe = regexp.MustCompile(`(?m)(^|[,\{\s])"?(confidence|result_kind|aggregate_facts|absence_justification)"?\s*[:=]`)
var completionReasonBareConfidenceRe = regexp.MustCompile(`(?i)("confidence"\s*:\s*)(high|medium)\b`)
var completionReasonBareResultKindRe = regexp.MustCompile(`(?i)("result_kind"\s*:\s*)(resolved|absence)\b`)

func extractMisplacedCompletionFieldsFromReason(reason string) (string, map[string]json.RawMessage, bool) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", nil, false
	}
	matches := completionReasonFieldStartRe.FindAllStringSubmatchIndex(reason, -1)
	for _, match := range matches {
		if len(match) < 6 || match[2] < 0 {
			continue
		}
		start := match[2]
		tail := strings.TrimSpace(reason[start:])
		fields, ok := decodeMisplacedCompletionReasonTail(tail)
		if !ok || !misplacedCompletionReasonFieldsAreSufficient(fields) {
			continue
		}
		clean := strings.TrimRight(strings.TrimSpace(reason[:start]), " \t\r\n,;")
		return clean, fields, true
	}
	return "", nil, false
}

func decodeMisplacedCompletionReasonTail(tail string) (map[string]json.RawMessage, bool) {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return nil, false
	}
	var candidates []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range candidates {
			if existing == s {
				return
			}
		}
		candidates = append(candidates, s)
	}
	add(tail)
	for _, candidate := range misplacedObjectTailCandidates(tail) {
		add(candidate)
	}
	for _, candidate := range candidates {
		normalized := normalizeMisplacedCompletionReasonTail(candidate)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte("{"+normalized+"}"), &fields); err == nil {
			return fields, true
		}
	}
	return nil, false
}

func normalizeMisplacedCompletionReasonTail(tail string) string {
	tail = strings.TrimSpace(tail)
	tail = completionReasonBareKeyRe.ReplaceAllString(tail, `${1}"${2}":`)
	tail = completionReasonBareConfidenceRe.ReplaceAllString(tail, `${1}"${2}"`)
	tail = completionReasonBareResultKindRe.ReplaceAllString(tail, `${1}"${2}"`)
	if repaired, changed := toolparam.RemoveTrailingCommasBeforeJSONClosers(tail); changed {
		tail = repaired
	}
	return strings.TrimSpace(tail)
}

func misplacedCompletionReasonFieldsAreSufficient(fields map[string]json.RawMessage) bool {
	if len(fields) == 0 {
		return false
	}
	score := 0
	if raw := fields["confidence"]; len(raw) > 0 {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && completionConfidenceValid(s) {
			score++
		}
	}
	if raw := fields["result_kind"]; len(raw) > 0 {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && completionResultKindValid(s) {
			score++
		}
	}
	if raw := fields["aggregate_facts"]; len(raw) > 0 {
		if _, _, _, err := decodeAggregateFactsPayload(raw); err == nil {
			score++
		}
	}
	if raw := fields["absence_justification"]; len(raw) > 0 {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
			score++
		}
	}
	return score >= 2
}

func completionConfidenceValid(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium":
		return true
	default:
		return false
	}
}

func completionResultKindValid(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "resolved", "absence":
		return true
	default:
		return false
	}
}

func mergeMisplacedCompletionFields(base, extra map[string]json.RawMessage) map[string]json.RawMessage {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]json.RawMessage, len(extra))
	}
	for key, raw := range extra {
		if len(raw) == 0 {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = raw
	}
	return base
}

func splitLeadingJSONArrayWithMisplacedObjectTail(s string) ([]byte, map[string]json.RawMessage, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !strings.HasPrefix(s, "[") {
		return nil, nil, false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, nil, false
	}
	if len(raw) == 0 || raw[0] != '[' {
		return nil, nil, false
	}
	rest := strings.TrimSpace(s[int(dec.InputOffset()):])
	if rest == "" {
		return raw, nil, true
	}
	if !strings.HasPrefix(rest, ",") {
		return nil, nil, false
	}
	var misplaced map[string]json.RawMessage
	for _, tail := range misplacedObjectTailCandidates(strings.TrimSpace(strings.TrimPrefix(rest, ","))) {
		if err := json.Unmarshal([]byte("{"+tail+"}"), &misplaced); err == nil {
			return raw, misplaced, true
		}
	}
	return nil, nil, false
}

func misplacedObjectTailCandidates(tail string) []string {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return nil
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if existing == s {
				return
			}
		}
		out = append(out, s)
	}
	add(tail)
	if repaired, changed := toolparam.RemoveTrailingCommasBeforeJSONClosers(tail); changed {
		add(repaired)
	}
	for _, candidate := range append([]string(nil), out...) {
		if trimmed, ok := trimOneDanglingObjectTailCloser(candidate); ok {
			add(trimmed)
			if repaired, changed := toolparam.RemoveTrailingCommasBeforeJSONClosers(trimmed); changed {
				add(repaired)
			}
		}
	}
	return out
}

func trimOneDanglingObjectTailCloser(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	inString := false
	escaped := false
	lastNonSpace := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case ' ', '\n', '\r', '\t':
		default:
			lastNonSpace = i
		}
	}
	if inString || lastNonSpace < 0 || s[lastNonSpace] != '}' {
		return "", false
	}
	return strings.TrimSpace(s[:lastNonSpace]), true
}

func decodeMisplacedStringField(fields map[string]json.RawMessage, name string) string {
	if len(fields) == 0 {
		return ""
	}
	raw, ok := fields[name]
	if !ok || len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func normalizeCompletionAggregateFacts(
	ctx *types.BusContext,
	resultKind string,
	raw []types.AnswerAggregateFact,
) ([]types.AnswerAggregateFact, []string, error) {
	var preNotes []string
	raw, preNotes = normalizeCompletionNegativeObservationFacts(ctx, raw)
	var compatNotes []string
	raw, compatNotes = normalizeCompletionAggregateFactCompat(ctx, raw)
	preNotes = append(preNotes, compatNotes...)
	var requestBoundCountNotes []string
	raw, requestBoundCountNotes = canonicalizeRequestBoundSourceInventoryMemberSetCounts(ctx, raw)
	preNotes = append(preNotes, requestBoundCountNotes...)
	normalized, err := types.NormalizeAnswerAggregateFacts(raw)
	if err == nil {
		// AUTOREPAIR-1 件2 (§29.175): surface the types-layer Tier2 backfills
		// by diffing the pre-validation payload against the validator output.
		preNotes = append(preNotes, aggregateFactTypesLayerBackfillNotes(raw, normalized)...)
		normalized, notes := reconcileCompletionAggregateFactsWithDeterministicCount(ctx, normalized)
		notes = append(preNotes, notes...)
		return normalized, notes, nil
	}
	optionalAggregates := completionAggregateFactsAreOptional(ctx, resultKind) &&
		!completionAggregateFactsContainLoadBearingAbsenceProof(raw)
	if len(raw) == 0 || (!optionalAggregates && !completionAggregateFactsCanDropContextualNegativeObservation(ctx, resultKind, raw)) {
		return normalized, preNotes, err
	}
	out := make([]types.AnswerAggregateFact, 0, len(raw))
	notes := append([]string(nil), preNotes...)
	droppedContextualNegative := false
	for i, fact := range raw {
		one, oneErr := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{fact})
		if oneErr != nil {
			if !optionalAggregates &&
				!completionAggregateFactIsDroppableContextualNegativeObservation(fact) &&
				!completionAggregateFactIsDroppableImpreciseRepoNegativeObservation(fact) {
				return normalized, preNotes, err
			}
			if optionalAggregates {
				notes = append(notes, fmt.Sprintf("dropped optional aggregate_facts[%d]: %v", i, oneErr))
			} else {
				notes = append(notes, fmt.Sprintf("dropped contextual aggregate_facts[%d]: %v", i, oneErr))
				droppedContextualNegative = true
			}
			continue
		}
		out = append(out, one...)
	}
	if droppedContextualNegative && !completionAggregateFactsHavePrincipalSurvivor(ctx, out) {
		return normalized, preNotes, err
	}
	if len(out) == 0 {
		if optionalAggregates {
			notes = append(notes, fmt.Sprintf("ignored optional aggregate_facts payload after validation error: %v", err))
		} else {
			notes = append(notes, fmt.Sprintf("ignored contextual aggregate_facts payload after validation error: %v", err))
		}
		return nil, notes, nil
	}
	merged, mergeErr := types.NormalizeAnswerAggregateFacts(out)
	if mergeErr != nil {
		// §21 EMIT-2 / 维度C③ (N2, 2026-07-07): a cap-class merge failure used
		// to nil the WHOLE payload and return success — 17 individually valid
		// facts silently wiped with only a notes breadcrumb (latent silent data
		// loss, static-audit find). A collection-level cap error belongs to the
		// set, not to any single entry, so the per-item salvage loop above is
		// structurally blind to it. Fix at the shared point BEFORE the
		// optional/contextual note split (covers both lanes): compact by role
		// priority to the cap and disclose, never discard everything. Non-cap
		// merge errors keep the legacy ignore path.
		var capErr *types.AggregateFactsCapExceededError
		if errors.As(mergeErr, &capErr) {
			compacted := compactCompletionAggregateFactsForRuntime(out, types.MaxAnswerAggregateFacts)
			if remerged, remergeErr := types.NormalizeAnswerAggregateFacts(compacted); remergeErr == nil {
				notes = append(notes, fmt.Sprintf("aggregate_facts payload had %d entries over the %d-entry cap; kept %d by role priority (principal_answer first, audit_ledger last) instead of discarding the payload — merge same-family facts into one grouped_count or move detail into reason to avoid truncation", len(out), types.MaxAnswerAggregateFacts, len(remerged)))
				merged, mergeErr = remerged, nil
			}
		}
	}
	if mergeErr != nil {
		if optionalAggregates {
			notes = append(notes, fmt.Sprintf("ignored optional aggregate_facts payload after merge validation error: %v", mergeErr))
		} else {
			notes = append(notes, fmt.Sprintf("ignored contextual aggregate_facts payload after merge validation error: %v", mergeErr))
		}
		return nil, notes, nil
	}
	var reconcileNotes []string
	merged, reconcileNotes = reconcileCompletionAggregateFactsWithDeterministicCount(ctx, merged)
	notes = append(notes, reconcileNotes...)
	if optionalAggregates {
		notes = append(notes, fmt.Sprintf("kept %d/%d optional aggregate_facts after dropping invalid entries", len(merged), len(raw)))
	} else {
		notes = append(notes, fmt.Sprintf("kept %d/%d aggregate_facts after dropping contextual non-principal entries", len(merged), len(raw)))
	}
	return merged, notes, nil
}

func completionAggregateFactsCanDropContextualNegativeObservation(ctx *types.BusContext, resultKind string, facts []types.AnswerAggregateFact) bool {
	if !strings.EqualFold(strings.TrimSpace(resultKind), "resolved") {
		return false
	}
	hasDroppable := false
	for _, fact := range facts {
		if completionAggregateFactIsDroppableContextualNegativeObservation(fact) ||
			completionAggregateFactIsDroppableImpreciseRepoNegativeObservation(fact) {
			hasDroppable = true
			continue
		}
		if one, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{fact}); err == nil &&
			completionAggregateFactsHavePrincipalSurvivor(ctx, one) {
			continue
		}
	}
	return hasDroppable
}

func completionAggregateFactIsDroppableContextualNegativeObservation(fact types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateNegativeObservation {
		return false
	}
	role := types.NormalizeAnswerAggregateRole(fact.Role)
	if role != types.AnswerAggregateRoleSupportingCoverage && role != types.AnswerAggregateRoleAuditLedger {
		return false
	}
	if !completionAggregateFactHasRepoSourceBoundary(fact) {
		return false
	}
	candidate := completionAggregateFactWithSyntheticOrigin(fact, string(types.AnswerEvidenceOriginRuntimeArtifact))
	if _, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{candidate}); err != nil {
		return false
	}
	return true
}

func completionAggregateFactIsDroppableImpreciseRepoNegativeObservation(fact types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateNegativeObservation {
		return false
	}
	if types.NormalizeAnswerAggregateRole(fact.Role) != types.AnswerAggregateRoleUnknown {
		return false
	}
	if !completionAggregateFactHasRepoSourceBoundary(fact) {
		return false
	}
	if !completionAggregateFactZeroResult(fact) {
		return false
	}
	dims := completionAggregateDimensionMap(fact.Dimensions)
	if dims["target"] != "" || dims["query"] != "" || dims["pattern"] != "" || dims["predicate"] != "" {
		return false
	}
	return completionAggregateFactObservedSurface(dims) != ""
}

func completionAggregateFactZeroResult(fact types.AnswerAggregateFact) bool {
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		for _, dim := range fact.Dimensions {
			if normalizeAggregateFactCompatDimensionKey(dim.Name) == "result_count" {
				value = strings.TrimSpace(dim.Value)
				break
			}
		}
	}
	if value == "" {
		value = "0"
	}
	n, err := strconv.Atoi(value)
	return err == nil && n == 0
}

func completionAggregateFactHasRepoSourceOrigin(fact types.AnswerAggregateFact) bool {
	values := []string{fact.Provenance}
	for _, dim := range fact.Dimensions {
		name := normalizeAggregateFactCompatDimensionKey(dim.Name)
		if name == "origin" || name == "evidence_origin" {
			values = append(values, dim.Value)
		}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if types.AnswerEvidenceOriginFromStructuredToken(value) == types.AnswerEvidenceOriginCurrentSource {
			return true
		}
		switch strings.ToLower(value) {
		case "read_file", "repo_map", "source_inventory":
			return true
		}
	}
	return false
}

func completionAggregateFactHasRepoSourceBoundary(fact types.AnswerAggregateFact) bool {
	if completionAggregateFactHasRepoSourceOrigin(fact) {
		return true
	}
	return completionAggregateFactObservedSurface(completionAggregateDimensionMap(fact.Dimensions)) != ""
}

func completionAggregateFactObservedSurface(dims map[string]string) string {
	for _, key := range []string{"scope", "file", "path", "source", "source_ref", "repo_path"} {
		if value := strings.TrimSpace(dims[key]); value != "" {
			return value
		}
	}
	return ""
}

func completionAggregateFactWithSyntheticOrigin(fact types.AnswerAggregateFact, origin string) types.AnswerAggregateFact {
	out := fact
	out.Provenance = origin
	dims := make([]types.AnswerAggregateDimension, 0, len(fact.Dimensions)+1)
	replaced := false
	for _, dim := range fact.Dimensions {
		name := normalizeAggregateFactCompatDimensionKey(dim.Name)
		if name == "origin" || name == "evidence_origin" {
			if !replaced {
				dims = append(dims, types.AnswerAggregateDimension{Name: "origin", Value: origin})
				replaced = true
			}
			continue
		}
		dims = append(dims, dim)
	}
	if !replaced {
		dims = append(dims, types.AnswerAggregateDimension{Name: "origin", Value: origin})
	}
	out.Dimensions = dims
	return out
}

func completionAggregateFactsHavePrincipalSurvivor(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	rm := requestModelForAggregateSupport(ctx)
	for _, fact := range facts {
		if types.AnswerAggregateFactRoleForRequest(fact, rm) == types.AnswerAggregateRolePrincipalAnswer {
			return true
		}
	}
	return false
}

func completionAggregateFactsContainLoadBearingAbsenceProof(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		if types.AnswerAggregateFactIsExactEmptyMemberSet(fact) ||
			completionAggregateNegativeFactDeclaresZero(fact) {
			return true
		}
	}
	return false
}

func completionAggregateNegativeFactDeclaresZero(fact types.AnswerAggregateFact) bool {
	switch fact.Kind {
	case types.AnswerAggregateNegativeSearch:
		if !completionAggregateFactZeroResult(fact) {
			return false
		}
		return completionAggregateDimensionValue(fact.Dimensions, "repo") != "" &&
			completionAggregateFirstDimensionValueForFact(fact, "query", "pattern") != "" &&
			completionAggregateFirstDimensionValueForFact(fact, "scope") != ""
	case types.AnswerAggregateNegativeObservation:
		if !completionAggregateFactZeroResult(fact) {
			return false
		}
		return completionAggregateFirstDimensionValueForFact(fact, "target", "query", "pattern", "predicate") != "" &&
			completionAggregateFactObservedSurface(completionAggregateDimensionMap(fact.Dimensions)) != ""
	default:
		return false
	}
}

func completionAggregateFirstDimensionValueForFact(fact types.AnswerAggregateFact, names ...string) string {
	for _, name := range names {
		if value := completionAggregateDimensionValue(fact.Dimensions, name); value != "" {
			return value
		}
	}
	return ""
}

func completionAggregateDimensionValue(dims []types.AnswerAggregateDimension, want string) string {
	want = normalizeAggregateFactCompatDimensionKey(want)
	for _, dim := range dims {
		if normalizeAggregateFactCompatDimensionKey(dim.Name) == want {
			return strings.TrimSpace(dim.Value)
		}
	}
	return ""
}

func normalizeCompletionAggregateFactCompat(ctx *types.BusContext, raw []types.AnswerAggregateFact) ([]types.AnswerAggregateFact, []string) {
	if len(raw) == 0 {
		return raw, nil
	}
	out := append([]types.AnswerAggregateFact(nil), raw...)
	var notes []string
	// A memberless non-integer value in a count kind is a measurement
	// (duration/latency/percentage/frequency/density) that was misfiled — a float
	// in a count field is a semantic error in ANY intent/scenario, so reclassify
	// it to scalar_value unconditionally rather than gating on a runtime-artifact
	// scene (which stranded in-repo root_cause turns and drove the count-kind
	// ping-pong retries). The one exception is a genuine integer-count question
	// (IsCountQuestion): there a decimal is a real user-facing error and must
	// remain a hard reject so the model corrects the count itself.
	allowMemberlessDecimalScalar := !completionAggregateFactIsIntegerCountQuestion(ctx)
	for i := range out {
		fact := &out[i]
		if completionAggregateFactValueRepairableFromMembers(*fact) {
			oldValue := strings.TrimSpace(fact.Value)
			fact.Value = strconv.Itoa(len(fact.Members))
			notes = append(notes, fmt.Sprintf("aggregate_facts[%d] %s value %q->%s from exact members", i, fact.Kind, oldValue, fact.Value))
		}
		if !allowMemberlessDecimalScalar || !completionAggregateFactDecimalCountShouldBeScalar(*fact) {
			continue
		}
		oldKind := fact.Kind
		oldValue := strings.TrimSpace(fact.Value)
		// AUTOREPAIR-1 件3② (§29.175 T2-UNIT-SUFFIX-SPLIT): a unit-suffixed
		// decimal ("10.503ms") in a memberless count kind splits into
		// value+unit on the same exact lexical boundary the reject prober
		// uses — kind→scalar_value, value→numeric prefix, unit→suffix (only
		// when unit was empty). The IsCountQuestion carve-out above and the
		// members/excluded guards inside the trigger are unchanged.
		if prefix, suffix, ok := completionAggregateFactUnitSuffixSplit(*fact); ok {
			fact.Kind = types.AnswerAggregateScalar
			fact.Value = prefix
			if strings.TrimSpace(fact.Unit) == "" {
				fact.Unit = suffix
			}
			notes = append(notes, fmt.Sprintf("aggregate_facts[%d] kind normalized %s→scalar_value;值 %q 拆为 value=%s unit=%s(计数类须整数)", i, oldKind, oldValue, fact.Value, fact.Unit))
			continue
		}
		fact.Kind = types.AnswerAggregateScalar
		notes = append(notes, fmt.Sprintf("aggregate_facts[%d] kind normalized %s→scalar_value because value=%q is a non-integer measurement; count kinds require integer values", i, oldKind, oldValue))
	}
	// §21 EMIT-2 / 维度C① (2026-07-07): the cap compaction used to be gated on
	// an origin-lane scene signal (runtime completion-landing authority), which
	// stayed closed for mixed current_source+runtime lanes and let a 17>16
	// payload burn a full retry round (specimen cust_trace_cmp_01 line 106/161)
	// — §18.A 根因1 同形 scene-gated safety net. The gate is now the payload
	// shape itself, a PRECISE signal: role-priority compaction can never drop a
	// principal_answer fact while the principal slate fits the cap, so it runs
	// unconditionally in that shape. Only a principal slate that ALONE exceeds
	// the cap stays a hard reject (routed by
	// completionAggregateFactsCapRejectRouting): the model must consolidate its
	// answer facts; the system truncates but never merges or rewrites them
	// (系统不可代替 LLM 写用户面板答案). Same pattern as the §18.A.1 NUM
	// landing ruling: unconditional self-heal + one precise carve-out.
	if limit := types.MaxAnswerAggregateFacts; len(out) > limit &&
		completionAggregateFactsWithinPrincipalBudget(out, limit) {
		before := len(out)
		out = compactCompletionAggregateFactsForRuntime(out, limit)
		notes = append(notes, fmt.Sprintf("aggregate_facts compacted from %d to %d entries (cap %d, kept by role priority: principal_answer first, audit_ledger last); preserve extra audit details in reason if needed", before, len(out), limit))
	}
	return out, notes
}

// completionAggregateFactsWithinPrincipalBudget reports whether the
// principal_answer facts alone fit the cap — the payload-shape precise signal
// (§21 EMIT-2 / 维度C①) that makes role-priority compaction lossless for the
// answer slate: while true, truncation only sheds supporting/audit context;
// once false, silent truncation would drop parts of the ANSWER, so the caller
// must keep the hard reject and route the model to consolidate instead.
func completionAggregateFactsWithinPrincipalBudget(facts []types.AnswerAggregateFact, limit int) bool {
	principal := 0
	for _, fact := range facts {
		if types.NormalizeAnswerAggregateRole(fact.Role) == types.AnswerAggregateRolePrincipalAnswer {
			principal++
		}
	}
	return principal <= limit
}

// completionAggregateFactsCapRejectRouting builds the operation routing that
// rides an aggregate_facts cap rejection (§21 EMIT-2 / 维度C②, NUM 根因2
// 孪生): the current count, the per-role tally, and the three concrete edits
// (merge same-family facts / move detail to reason / trim lowest-value roles).
// LLM-facing text: it names only schema-visible concepts (roles, kinds,
// reason), never internal mechanisms.
func completionAggregateFactsCapRejectRouting(facts []types.AnswerAggregateFact) string {
	counts := map[types.AnswerAggregateRole]int{}
	for _, fact := range facts {
		counts[types.NormalizeAnswerAggregateRole(fact.Role)]++
	}
	var tally []string
	appendRole := func(label string, role types.AnswerAggregateRole) {
		if n := counts[role]; n > 0 {
			tally = append(tally, fmt.Sprintf("%s=%d", label, n))
		}
	}
	appendRole("principal_answer", types.AnswerAggregateRolePrincipalAnswer)
	appendRole("supporting_coverage", types.AnswerAggregateRoleSupportingCoverage)
	appendRole("audit_ledger", types.AnswerAggregateRoleAuditLedger)
	appendRole("unspecified_role", types.AnswerAggregateRoleUnknown)
	return fmt.Sprintf(
		"You sent %d facts (role tally: %s); the budget is %d entries. Keep the highest-value facts: merge same-family per-group scalars into ONE grouped_count fact with members (one fact per metric family, not one per group or per trace side), move audit-only details into reason, and drop the lowest-value audit_ledger entries first. Do not drop principal_answer facts — consolidate them into fewer aggregate rows instead.",
		len(facts), strings.Join(tally, ", "), types.MaxAnswerAggregateFacts)
}

func completionAggregateFactValueRepairableFromMembers(fact types.AnswerAggregateFact) bool {
	if len(fact.Members) == 0 {
		return false
	}
	switch fact.Kind {
	case types.AnswerAggregateMemberSet,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount:
	default:
		return false
	}
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return false
	}
	n, err := strconv.Atoi(value)
	return err != nil || n < 0
}

// completionAggregateFactIsIntegerCountQuestion reports whether the request is a
// genuine integer-count question (IsCountQuestion) — the single precise typed
// signal that marks the boundary where a decimal value in a count kind is a real
// user-facing error (the model must correct the count itself) rather than a
// misfiled runtime measurement. For every other intent/scenario a memberless
// float in a count kind is reclassified to scalar_value; only this boundary keeps
// the decimal a hard reject.
func completionAggregateFactIsIntegerCountQuestion(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion
}

func completionAggregateFactDecimalCountShouldBeScalar(fact types.AnswerAggregateFact) bool {
	if !completionAggregateFactIsCountKind(fact.Kind) {
		return false
	}
	if len(fact.Members) > 0 || len(fact.Excluded) > 0 {
		return false
	}
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return false
	}
	if _, err := strconv.ParseUint(value, 10, 64); err == nil {
		return false
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return true
	}
	// AUTOREPAIR-1 件3② (§29.175): unit-suffixed decimal arm — the exact
	// lexical split rules the same misfiled-measurement semantics.
	_, _, ok := completionAggregateFactUnitSuffixSplit(fact)
	return ok
}

// completionAggregateFactUnitSuffixSplit (AUTOREPAIR-1 件3②, §29.175
// T2-UNIT-SUFFIX-SPLIT) reports whether a memberless count-kind value splits
// as <numeric-prefix><unit-suffix>: the prefix passes ParseFloat, the suffix
// is a non-empty run of letters/%/µ on the single-sourced numeric-prefix
// boundary the types-layer reject prober uses
// (types.AggregateValueNumericPrefix — exact lexical split, no similarity
// scoring), and the fact's unit slot is empty or byte-equal to the suffix. A
// conflicting unit ("s" vs suffix "ms") disqualifies the repair — the
// contradiction is the model's to resolve.
func completionAggregateFactUnitSuffixSplit(fact types.AnswerAggregateFact) (prefix, suffix string, ok bool) {
	if !completionAggregateFactIsCountKind(fact.Kind) {
		return "", "", false
	}
	if len(fact.Members) > 0 || len(fact.Excluded) > 0 {
		return "", "", false
	}
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return "", "", false
	}
	prefix = types.AggregateValueNumericPrefix(value)
	suffix = value[len(prefix):]
	if prefix == "" || suffix == "" {
		return "", "", false
	}
	if _, err := strconv.ParseFloat(prefix, 64); err != nil {
		return "", "", false
	}
	for _, r := range suffix {
		if !unicode.IsLetter(r) && r != '%' && r != 'µ' {
			return "", "", false
		}
	}
	if unit := strings.TrimSpace(fact.Unit); unit != "" && unit != suffix {
		return "", "", false
	}
	return prefix, suffix, true
}

func compactCompletionAggregateFactsForRuntime(raw []types.AnswerAggregateFact, limit int) []types.AnswerAggregateFact {
	if len(raw) <= limit || limit <= 0 {
		return raw
	}
	out := make([]types.AnswerAggregateFact, 0, limit)
	addGroup := func(match func(types.AnswerAggregateFact) bool) {
		for _, fact := range raw {
			if len(out) >= limit {
				return
			}
			if !match(fact) {
				continue
			}
			out = append(out, fact)
		}
	}
	addGroup(func(f types.AnswerAggregateFact) bool {
		return types.NormalizeAnswerAggregateRole(f.Role) == types.AnswerAggregateRolePrincipalAnswer
	})
	addGroup(func(f types.AnswerAggregateFact) bool {
		return types.NormalizeAnswerAggregateRole(f.Role) == types.AnswerAggregateRoleSupportingCoverage
	})
	addGroup(func(f types.AnswerAggregateFact) bool {
		return types.NormalizeAnswerAggregateRole(f.Role) == types.AnswerAggregateRoleUnknown
	})
	addGroup(func(f types.AnswerAggregateFact) bool {
		return types.NormalizeAnswerAggregateRole(f.Role) == types.AnswerAggregateRoleAuditLedger
	})
	if len(out) == 0 {
		return raw[:limit]
	}
	return out
}

func reconcileCompletionAggregateFactsWithDeterministicCount(
	ctx *types.BusContext,
	facts []types.AnswerAggregateFact,
) ([]types.AnswerAggregateFact, []string) {
	if len(facts) == 0 || ctx == nil || ctx.AnalysisIR == nil {
		return facts, nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion || rm.Predicates.IsHistoryLookup {
		return facts, nil
	}
	measured, ok := deterministicCountToolResultValue(ctx)
	if !ok {
		return facts, nil
	}
	type candidate struct {
		index int
		value int
	}
	var candidates []candidate
	for i, fact := range facts {
		value, ok := deterministicCountAggregateFactNumericValue(fact)
		if !ok {
			continue
		}
		if !deterministicCountAggregateFactCanReconcile(fact) {
			continue
		}
		candidates = append(candidates, candidate{index: i, value: value})
	}
	if len(candidates) != 1 {
		return facts, nil
	}
	c := candidates[0]
	if c.value == measured {
		return facts, nil
	}
	out := cloneCompletionAggregateFacts(facts)
	fact := &out[c.index]
	old := fact.Value
	fact.Value = strconv.Itoa(measured)
	fact.Dimensions = ensureDeterministicCountAggregateDimensions(fact.Dimensions)
	if strings.TrimSpace(fact.Provenance) == "" {
		fact.Provenance = "system_deterministic_count_reconcile"
	}
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		label = fmt.Sprintf("aggregate_facts[%d]", c.index)
	}
	return out, []string{fmt.Sprintf("%s.value %s->%d from deterministic command measurement", label, old, measured)}
}

func deterministicCountAggregateFactNumericValue(fact types.AnswerAggregateFact) (int, bool) {
	switch fact.Kind {
	case types.AnswerAggregateTotalCount, types.AnswerAggregateUniqueCount, types.AnswerAggregateScalar:
	default:
		return 0, false
	}
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func deterministicCountAggregateFactCanReconcile(fact types.AnswerAggregateFact) bool {
	role := types.NormalizeAnswerAggregateRole(fact.Role)
	if role != types.AnswerAggregateRoleUnknown && role != types.AnswerAggregateRolePrincipalAnswer {
		return false
	}
	if len(fact.Members) > 0 || len(fact.Excluded) > 0 {
		return false
	}
	if !deterministicCountAggregateFactHasCommandMeasurementSurface(fact) {
		return false
	}
	origins := types.AnswerAggregateFactEvidenceOrigins(fact, &types.RequestModel{
		Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true},
	})
	if len(origins) == 0 {
		return true
	}
	for _, origin := range origins {
		switch origin {
		case types.AnswerEvidenceOriginCommandMeasurement, types.AnswerEvidenceOriginCurrentSource:
			continue
		default:
			return false
		}
	}
	return true
}

func deterministicCountAggregateFactHasCommandMeasurementSurface(fact types.AnswerAggregateFact) bool {
	if types.AnswerEvidenceOriginFromStructuredToken(fact.Provenance) == types.AnswerEvidenceOriginCommandMeasurement {
		return true
	}
	for _, dim := range fact.Dimensions {
		value := strings.ToLower(strings.TrimSpace(dim.Value))
		if value == "" {
			continue
		}
		if types.AnswerEvidenceOriginFromStructuredToken(value) == types.AnswerEvidenceOriginCommandMeasurement {
			return true
		}
	}
	return false
}

func ensureDeterministicCountAggregateDimensions(in []types.AnswerAggregateDimension) []types.AnswerAggregateDimension {
	out := append([]types.AnswerAggregateDimension(nil), in...)
	has := func(name string) bool {
		for _, dim := range out {
			if strings.EqualFold(strings.TrimSpace(dim.Name), name) {
				return true
			}
		}
		return false
	}
	if !has("origin") && !has("evidence_origin") {
		out = append(out, types.AnswerAggregateDimension{Name: "origin", Value: string(types.AnswerEvidenceOriginCommandMeasurement)})
	}
	if !has("proof_source") {
		out = append(out, types.AnswerAggregateDimension{Name: "proof_source", Value: "exec_command"})
	}
	if !has("answer_axis") {
		out = append(out, types.AnswerAggregateDimension{Name: "answer_axis", Value: "count"})
	}
	return out
}

// AUTOREPAIR-1 件2 (§29.175 T2-DISCLOSE-SILENT-BACKFILLS) — Tier2 disclosure
// wording, single wording point (词面单点): exactly three formats, reused
// verbatim by every completion-layer backfill arm and by the types-layer
// dimension differ. No call site may paraphrase them.
//
//	(a) dimension backfill — the third slot names the TYPED source
//	    (provenance / excluded[0] / 附加运行时工件 / 同事实维度 / value);
//	(b) searched_at default — the wording must say the system minted only
//	    the schema-default token, never a time claim (a real timestamp
//	    assertion would be Tier3 content);
//	(c) value zeroing for negative kinds.
const (
	aggregateFactBackfillDimensionNoteFormat  = "aggregate_facts[%d] 已补注 %s=%s(由 %s 推导)"
	aggregateFactBackfillSearchedAtNoteFormat = "aggregate_facts[%d] 已补注 searched_at=current_investigation(默认值,非时间戳断言)"
	aggregateFactBackfillValueZeroNoteFormat  = "aggregate_facts[%d] 已按 result_count 归一 value=0"
	// aggregateFactKindFoldNoteFormat discloses the 件3③ lexical kind fold
	// applied by the types layer (fold happens inside the shared validator;
	// the differ surfaces it here).
	aggregateFactKindFoldNoteFormat = "aggregate_facts[%d] kind 词形归一 %q→%s"
	// aggregateFactBackfillUnitMatchesNoteFormat — RULE3-1 件5 (§29.181④,
	// 2026-07-21): the types layer silently backfills Unit:="matches" on
	// negative kinds (answer_aggregate_fact.go 两处回填); 「凡施修必披露」
	// admits zero exceptions, so the differ mints this note whenever the
	// backfill actually ran (a fact arriving WITH a unit stays note-less).
	aggregateFactBackfillUnitMatchesNoteFormat = "aggregate_facts[%d] 已补注 unit=matches(由 result_count 语义推导)"
)

// Typed-source tokens for note format (a) — closed set, one token per arm.
const (
	aggregateFactBackfillSourceProvenance      = "provenance"
	aggregateFactBackfillSourceExcludedFirst   = "excluded[0]"
	aggregateFactBackfillSourceRuntimeArtifact = "附加运行时工件"
	aggregateFactBackfillSourceOwnDimensions   = "同事实维度"
	aggregateFactBackfillSourceValueSlot       = "value"
)

func normalizeCompletionNegativeObservationFacts(ctx *types.BusContext, raw []types.AnswerAggregateFact) ([]types.AnswerAggregateFact, []string) {
	if len(raw) == 0 {
		return raw, nil
	}
	out := append([]types.AnswerAggregateFact(nil), raw...)
	var notes []string
	for i := range out {
		fact := &out[i]
		if fact.Kind != types.AnswerAggregateNegativeObservation {
			continue
		}
		dims := completionAggregateDimensionMap(fact.Dimensions)
		if dims["origin"] == "" && dims["evidence_origin"] == "" {
			switch {
			case strings.TrimSpace(fact.Provenance) != "":
				fact.Dimensions = append(fact.Dimensions, types.AnswerAggregateDimension{Name: "origin", Value: strings.TrimSpace(fact.Provenance)})
				dims["origin"] = strings.TrimSpace(fact.Provenance)
				notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "origin", dims["origin"], aggregateFactBackfillSourceProvenance))
			case ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.HasExternalOnlyRuntimeArtifact():
				fact.Dimensions = append(fact.Dimensions, types.AnswerAggregateDimension{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)})
				dims["origin"] = string(types.AnswerEvidenceOriginRuntimeArtifact)
				notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "origin", dims["origin"], aggregateFactBackfillSourceRuntimeArtifact))
			case completionAggregateFirstDimensionValue(dims, "artifact_id", "trace_window") != "" &&
				completionRuntimeNegativeObservationDefaultScope(ctx) != "":
				// EMITBURN-1 件5 (§29.173) narrow widening. In a MIXED
				// repo+artifact run a dimension-less negative_observation is
				// ambiguous between a repo-search absence (negative_search's
				// lane) and an artifact observation, so run-level attachment
				// alone must NOT stamp origin — that would launder repo
				// absences past both the origin hard arm and negative_search's
				// repo/scope discipline (authority-adjacent mislabel; origin is
				// descriptive but it also classifies the fact's lane). When the
				// fact ITSELF anchors to the artifact via artifact_id or
				// trace_window (precise fact-local signals with no repo
				// reading), origin=runtime_artifact is descriptively entailed
				// by the fact's own claim, so the convenience injection is
				// safe. tool_result/source_ref stay excluded: they also name
				// repo tools. The broad any-attached-run widening stays
				// rejected — reasoning filed in the EMITBURN-1 settle note.
				fact.Dimensions = append(fact.Dimensions, types.AnswerAggregateDimension{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)})
				dims["origin"] = string(types.AnswerEvidenceOriginRuntimeArtifact)
				notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "origin", dims["origin"], aggregateFactBackfillSourceOwnDimensions))
			}
		}
		if dims["target"] == "" && dims["query"] == "" && dims["pattern"] == "" && dims["predicate"] == "" && len(fact.Excluded) > 0 {
			if target := strings.TrimSpace(fact.Excluded[0]); target != "" {
				fact.Dimensions = append(fact.Dimensions, types.AnswerAggregateDimension{Name: "target", Value: target})
				dims["target"] = target
				notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "target", target, aggregateFactBackfillSourceExcludedFirst))
			}
		}
		if dims["scope"] == "" &&
			completionAggregateFirstDimensionValue(dims, "artifact_id", "trace_window", "commit_range", "tool_result", "source_ref") == "" {
			if scope := completionRuntimeNegativeObservationDefaultScope(ctx); scope != "" {
				fact.Dimensions = append(fact.Dimensions, types.AnswerAggregateDimension{Name: "scope", Value: scope})
				notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "scope", scope, aggregateFactBackfillSourceRuntimeArtifact))
			}
		}
	}
	return out, notes
}

// aggregateFactTypesLayerBackfillNotes (AUTOREPAIR-1 件2, §29.175) makes the
// types-layer Tier2 backfills visible without moving them: it diffs the
// payload handed to types.NormalizeAnswerAggregateFacts against its output
// and mints the single-wording-point notes for exactly the closed backfill
// set — kind lexical fold (件3③), negative-kind value zeroing, and the
// result_count / scope / searched_at dimension appends. The comparison uses
// the types layer's own kind-aware canonical dimension map, never a
// hand-copied alias table. Precision guards: notes are minted only for an
// index-aligned payload (equal lengths — the accept lane never reorders),
// and only for the closed dimension-name set, so unrelated normalization can
// never masquerade as a backfill. An already-canonical payload yields zero
// notes.
func aggregateFactTypesLayerBackfillNotes(pre, post []types.AnswerAggregateFact) []string {
	if len(pre) == 0 || len(pre) != len(post) {
		return nil
	}
	var notes []string
	for i := range post {
		rawFact := pre[i]
		fact := post[i]
		if rawFact.Kind != fact.Kind {
			if folded, ok := types.FoldAnswerAggregateKindLexical(rawFact.Kind); ok && folded == fact.Kind {
				notes = append(notes, fmt.Sprintf(aggregateFactKindFoldNoteFormat, i, string(rawFact.Kind), string(fact.Kind)))
			}
		}
		negativeKind := fact.Kind == types.AnswerAggregateNegativeSearch || fact.Kind == types.AnswerAggregateNegativeObservation
		if negativeKind && strings.TrimSpace(rawFact.Value) == "" && fact.Value == "0" {
			notes = append(notes, fmt.Sprintf(aggregateFactBackfillValueZeroNoteFormat, i))
		}
		// RULE3-1 件5 (§29.181④): the Unit:="matches" silent backfill gains
		// its disclosure note — minted exactly when the types layer filled an
		// EMPTY unit (a payload already carrying any unit stays note-less).
		if negativeKind && strings.TrimSpace(rawFact.Unit) == "" && fact.Unit == "matches" {
			notes = append(notes, fmt.Sprintf(aggregateFactBackfillUnitMatchesNoteFormat, i))
		}
		if !negativeKind {
			continue
		}
		preDims := types.AggregateFactCanonicalDimensionMapForKind(fact.Kind, rawFact.Dimensions)
		postDims := types.AggregateFactCanonicalDimensionMapForKind(fact.Kind, fact.Dimensions)
		if postDims["result_count"] != "" && preDims["result_count"] == "" {
			notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "result_count", postDims["result_count"], aggregateFactBackfillSourceValueSlot))
		}
		if postDims["scope"] != "" && preDims["scope"] == "" {
			notes = append(notes, fmt.Sprintf(aggregateFactBackfillDimensionNoteFormat, i, "scope", postDims["scope"], aggregateFactBackfillSourceOwnDimensions))
		}
		if postDims["searched_at"] != "" && preDims["searched_at"] == "" {
			notes = append(notes, fmt.Sprintf(aggregateFactBackfillSearchedAtNoteFormat, i))
		}
	}
	return notes
}

func completionAggregateDimensionMap(dims []types.AnswerAggregateDimension) map[string]string {
	out := make(map[string]string, len(dims))
	for _, dim := range dims {
		name := normalizeAggregateFactCompatDimensionKey(dim.Name)
		switch name {
		case "origin", "evidence_origin",
			"target", "query", "pattern", "predicate",
			"scope", "artifact_id", "trace_window", "commit_range", "tool_result", "source_ref",
			"file", "path", "repo_path":
			if out[name] == "" {
				out[name] = strings.TrimSpace(dim.Value)
			}
		}
	}
	return out
}

func completionAggregateFirstDimensionValue(dims map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(dims[name]); value != "" {
			return value
		}
	}
	return ""
}

func completionRuntimeNegativeObservationDefaultScope(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" {
		return "attached_runtime_trace"
	}
	if strings.TrimSpace(ctx.AttachedLog) != "" {
		return "attached_runtime_log"
	}
	if ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	switch {
	case rm.PerfTrace != nil && rm.PerfTrace.HasStructuredObservations():
		return "attached_runtime_trace"
	case rm.LogTriage != nil && rm.LogTriage.HasStructuredObservations():
		return "attached_runtime_log"
	case runtimeSourceAuthorityAllowsRuntimeCompletionLandingBool(ctx):
		return "attached_runtime_artifact"
	case rm.HasExternalOnlyRuntimeArtifact(),
		rm.HasRuntimeArtifactPathReference(),
		rm.HasRuntimeArtifactWithoutRequiredCurrentSource():
		return "attached_runtime_artifact"
	default:
		return ""
	}
}

func completionAggregateFactsAreOptional(ctx *types.BusContext, resultKind string) bool {
	if !strings.EqualFold(strings.TrimSpace(resultKind), "resolved") {
		return false
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.HasObservationOnlyRuntimeArtifact() {
		return true
	}
	if rm.Intent == types.IntentEnumerate ||
		rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.QuestionStructure().HasAnyObligation() ||
		types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm) {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		return false
	}
	if allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLanding(ctx); decided {
		return allowed
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		return true
	}
	return rm.Intent == types.IntentExplain ||
		rm.Intent == types.IntentTrace ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Scenario == types.ScenarioArchitectureExplain ||
		types.IsHistoryBackedCurrentCodeExplanation(rm)
}

func aggregateFactValueCanonicalizationNotes(raw, normalized []types.AnswerAggregateFact) []string {
	if len(raw) == 0 || len(normalized) == 0 {
		return nil
	}
	limit := len(raw)
	if len(normalized) < limit {
		limit = len(normalized)
	}
	var notes []string
	for i := 0; i < limit; i++ {
		rawFact := raw[i]
		fact := normalized[i]
		if rawFact.Kind != types.AnswerAggregateMemberSet || fact.Kind != types.AnswerAggregateMemberSet {
			if completionAggregateFactIsCountKind(rawFact.Kind) &&
				rawFact.Kind == fact.Kind &&
				len(rawFact.Members) > 0 &&
				len(fact.Members) == 0 {
				notes = append(notes, fmt.Sprintf("%s[%d].partial members omitted (value=%s len(members)=%d)",
					rawFact.Kind, i, strings.TrimSpace(rawFact.Value), len(rawFact.Members)))
			}
			continue
		}
		rawValue := strings.TrimSpace(rawFact.Value)
		if rawValue == "" || rawValue == fact.Value || len(fact.Members) == 0 {
			continue
		}
		if _, err := strconv.Atoi(rawValue); err != nil {
			continue
		}
		want := strconv.Itoa(len(fact.Members))
		if fact.Value != want {
			continue
		}
		notes = append(notes, fmt.Sprintf("member_set[%d].value %s->%s from members", i, rawValue, fact.Value))
	}
	return notes
}

func completionAggregateFactIsCountKind(kind types.AnswerAggregateKind) bool {
	switch kind {
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount:
		return true
	default:
		return false
	}
}

func decodeEmitInvestigationCompleteParamsStrict(ctx *types.BusContext, name string, params json.RawMessage, schema json.RawMessage) (emitInvestigationCompleteParams, *types.ToolResult, error) {
	normalized := applyStructuredPayloadCompat(name, params, schema)
	var raw emitInvestigationCompleteRawParams
	if _, decodeFailure, err := decodeStrictNormalizedToolParams(name, normalized, &raw, nil); err != nil {
		return emitInvestigationCompleteParams{}, decodeFailure, err
	}
	var p emitInvestigationCompleteParams
	if err := p.loadFromRaw(raw); err != nil {
		suffix := completionAggregateFactsDecodeRejectSuffix(ctx, err, schema)
		res, retErr := failStrictDecodeWithErrorMessage(name, time.Now(), err, nil, normalized, "emit_investigation_complete: ", suffix)
		return emitInvestigationCompleteParams{}, &res, retErr
	}
	return p, nil, nil
}

// completionAggregateFactsDecodeRejectSuffix (EMITBURN-1 件1+件2, §29.173)
// enriches an aggregate_facts entry-array strict-decode reject. The decode
// layer itself stays first-error by Go's decoder nature and the reject lane
// is unchanged; this suffix adds (a) the valid entry field list reflected
// from the tool schema — never hand-copied — and (b) the accumulated
// violation listing over EVERY entry (per-entry re-decode plus the full
// normalize walk of the leniently decodable payload), so one reject teaches
// every fix instead of one per retry round.
func completionAggregateFactsDecodeRejectSuffix(ctx *types.BusContext, err error, schema json.RawMessage) string {
	var entryErr *aggregateFactsEntryDecodeError
	if !errors.As(err, &entryErr) {
		return ""
	}
	var b strings.Builder
	if extractUnknownFieldName(err) != "" {
		if fields := aggregateFactsEntrySchemaFields(schema); len(fields) > 0 {
			fmt.Fprintf(&b, "; aggregate_facts entries accept only these fields: %s", strings.Join(fields, ", "))
		}
	}
	violations := collectAggregateFactsEntryDecodeViolations(entryErr.payload)
	if facts, ok := decodeAggregateFactsEntriesLenient(entryErr.payload); ok {
		violations = append(violations, completionAggregateFactsCollectViolations(ctx, facts)...)
	}
	if len(violations) > 0 {
		fmt.Fprintf(&b, "; the payload has %d violation(s) — fix ALL of them in this one re-emit: %s",
			len(violations), completionAggregateFactsViolationList(violations))
	}
	b.WriteString(completionAggregateFactsFixNotDeleteSentence)
	return b.String()
}

// collectAggregateFactsEntryDecodeViolations strict-decodes every entry of the
// aggregate_facts array individually and returns each entry's decoder error
// (still first-error PER entry — Go's decoder nature) with its index, so the
// accumulated reject can name every broken entry, not just the first one the
// whole-array decode stopped at.
func collectAggregateFactsEntryDecodeViolations(payload []byte) []string {
	var entries []json.RawMessage
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil
	}
	var out []string
	for i, entry := range entries {
		var fact types.AnswerAggregateFact
		dec := json.NewDecoder(strings.NewReader(string(entry)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&fact); err != nil {
			out = append(out, fmt.Sprintf("aggregate_facts[%d]: %v", i, RemapStrictDecodeErrorWithRaw(err, nil, entry)))
		}
	}
	return out
}

// decodeAggregateFactsEntriesLenient decodes the entry array ignoring unknown
// fields so the normalize-layer walker can still report every dimension-arm
// violation of the decodable content alongside the decode-layer violations.
func decodeAggregateFactsEntriesLenient(payload []byte) ([]types.AnswerAggregateFact, bool) {
	var facts []types.AnswerAggregateFact
	if err := json.Unmarshal(payload, &facts); err != nil {
		return nil, false
	}
	return facts, true
}

// aggregateFactsEntrySchemaFields reflects the aggregate_facts per-entry field
// names out of the tool schema (properties → aggregate_facts → items), reusing
// the declaration-order token walk of strictDecodeSchemaTopLevelFields. Any
// parse irregularity returns nil: the list is an additive hint, never a gate.
func aggregateFactsEntrySchemaFields(schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	var root struct {
		Properties struct {
			AggregateFacts struct {
				Items json.RawMessage `json:"items"`
			} `json:"aggregate_facts"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil
	}
	return strictDecodeSchemaTopLevelFields(root.Properties.AggregateFacts.Items)
}

// completionAggregateFactsFixNotDeleteSentence is the anti-deletion teaching
// appended to every aggregate_facts reject (EMITBURN-1 件4, §29.173): a
// rejected payload retains nothing, so deleting entries silently loses their
// typed values from the answer pipeline. Fix-first is the taught path;
// deletion is a named, reasoned last resort.
const completionAggregateFactsFixNotDeleteSentence = " Fix the failing entries in place rather than deleting them — rejected payloads retain nothing, so a deleted entry's typed value leaves the answer permanently; if an entry truly cannot be fixed, dropping it is a last resort and you must name the dropped entry in reason with one line on why."

// completionAggregateFactsCollectViolations runs the same emit-side pre-pass
// the accept path runs (negative-observation auto-fill, compat normalization)
// and then walks every entry and arm for the accumulated reject listing.
// Message content only — never a verdict.
func completionAggregateFactsCollectViolations(ctx *types.BusContext, raw []types.AnswerAggregateFact) []string {
	if len(raw) == 0 {
		return nil
	}
	raw, _ = normalizeCompletionNegativeObservationFacts(ctx, raw)
	raw, _ = normalizeCompletionAggregateFactCompat(ctx, raw)
	return types.CollectAnswerAggregateFactsViolations(raw)
}

// completionAggregateFactsViolationList formats the accumulated violations as
// a numbered single-line listing, capped at the first ten plus a count so a
// pathological payload cannot flood the reject message.
func completionAggregateFactsViolationList(violations []string) string {
	const maxListedAggregateViolations = 10
	shown := violations
	more := 0
	if len(shown) > maxListedAggregateViolations {
		more = len(shown) - maxListedAggregateViolations
		shown = shown[:maxListedAggregateViolations]
	}
	var b strings.Builder
	for i, violation := range shown {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "[%d] %s", i+1, violation)
	}
	if more > 0 {
		fmt.Fprintf(&b, "; ... and %d more violation(s)", more)
	}
	return b.String()
}

func (t *EmitInvestigationComplete) Execute(ctx *types.BusContext, params json.RawMessage) (result types.ToolResult, err error) {
	runtimeTimings := make([]types.ToolRuntimeTiming, 0, 6)
	defer func() {
		attachToolRuntimeTimings(&result, runtimeTimings)
	}()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   "emit_investigation_complete rejected: no mutable state (sub-agent context)",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	decodeStart := time.Now()
	p, decodeFailure, err := decodeEmitInvestigationCompleteParamsStrict(ctx, t.Name(), params, t.Parameters())
	if err != nil {
		recordToolRuntimeTiming(&runtimeTimings, "strict_decode", decodeStart, 0)
		return *decodeFailure, err
	}
	recordToolRuntimeTiming(&runtimeTimings, "strict_decode", decodeStart, 1)

	conf := strings.ToLower(strings.TrimSpace(p.Confidence))
	if conf != "high" && conf != "medium" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: fmt.Sprintf(
				"emit_investigation_complete rejected: confidence=%q is not accepted. "+
					"Only 'high' or 'medium' are valid. If you are unsure, continue "+
					"investigating — read more files, run more greps, collect more evidence.",
				p.Confidence),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	resultKind := strings.ToLower(strings.TrimSpace(p.ResultKind))
	if resultKind != "resolved" && resultKind != "absence" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: fmt.Sprintf(
				"emit_investigation_complete rejected: result_kind=%q is not accepted. "+
					"Use 'resolved' for ordinary positive/citable answers or 'absence' for honest zero / no-such-target answers.",
				p.ResultKind),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   "emit_investigation_complete rejected: reason is required",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	relationClaims, relationErr := validateCompletionRelationClaims(ctx, p.RelationClaims)
	if relationErr != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   fmt.Sprintf("emit_investigation_complete rejected: %v", relationErr),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	aggregateStart := time.Now()
	evidenceSnapshot := ctx.Mutable.EmittedEvidence()
	if repair := pendingBlockingEmitEvidenceItemValidationRepair(ctx); repair != nil {
		// A required typed relation row was locally rejected by the latest
		// emit_evidence call. Do not count a completion attempt made before that
		// exact item repair as flow no-progress: the failure is structurally
		// repairable and has not yet entered the evidence buffer. A later
		// successful emit_evidence call naturally supersedes this latch.
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   "emit_investigation_complete rejected: the latest emit_evidence call has an unresolved schema-invalid item required by the typed relation answer. " + repair.Hint,
			Repair:    attachToolJSONSurfaceMetadata(t.Name(), repair),
			Success:   true,
			Timestamp: time.Now(),
		}, nil
	}
	aggregateFacts, softAggregateNotes, err := normalizeCompletionAggregateFacts(ctx, resultKind, p.AggregateFacts)
	if err != nil {
		summary := fmt.Sprintf("emit_investigation_complete rejected: %v", err)
		// §21 EMIT-2 / 维度C② (N1, NUM 根因2 孪生): a cap rejection must teach
		// the concrete edit, not just restate the limit — same field-routing
		// discipline as the deterministic-count rejection lane. Typed error
		// detection (errors.As), never message substring matching.
		var capErr *types.AggregateFactsCapExceededError
		if errors.As(err, &capErr) {
			summary += " " + completionAggregateFactsCapRejectRouting(p.AggregateFacts)
		}
		// EMITBURN-1 件1 (§29.173): one reject lists EVERY violation across the
		// whole payload (per-entry index + arm + fix) instead of surfacing one
		// arm per retry round. Message content only — the reject verdict above
		// is untouched, and its headline stays the serial gate's first error.
		if all := completionAggregateFactsCollectViolations(ctx, p.AggregateFacts); len(all) > 1 {
			summary += fmt.Sprintf(" The payload has %d violations — fix ALL of them in this one re-emit: %s.",
				len(all), completionAggregateFactsViolationList(all))
		}
		summary += completionAggregateFactsFixNotDeleteSentence
		// AUTOREPAIR-1 件2 (§29.175, 组合合同 with EMITBURN-1 件1): when the
		// post-repair payload still rejects, the reject carries the repairs
		// already applied alongside the full violation list, so the model does
		// not re-fix what the system fixed.
		if len(softAggregateNotes) > 0 {
			summary += " | system repairs already applied (keep them in the re-emit): " + strings.Join(softAggregateNotes, "; ")
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   summary,
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	justification := strings.TrimSpace(p.AbsenceJustification)
	resultKind, justification = normalizeExactAbsenceCompletionWithEvidenceAndAggregates(ctx, resultKind, justification, evidenceSnapshot, aggregateFacts)
	aggregateFactNormalizationNotes := aggregateFactValueCanonicalizationNotes(p.AggregateFacts, aggregateFacts)
	aggregateFactNormalizationNotes = append(aggregateFactNormalizationNotes, softAggregateNotes...)
	earlyDowngradeConverged := false
	discoverySelectionMissing := resultKind == "resolved" &&
		callChainDiscoverySelectionRequired(ctx) &&
		!types.HasCallChainDiscoverySelectionEvidence(evidenceSnapshot)
	if !discoverySelectionMissing && ctx != nil && ctx.Mutable != nil {
		// A previously converged lane may outlive a later form/coverage
		// rejection. Precise current evidence upgrades that lane; do not carry
		// an obsolete "selection unproven" boundary into the accepted answer.
		ctx.Mutable.EvidenceClosure().ClearCompletionCaveat(types.DowngradeLaneCallChainDiscoverySelection)
	}
	if discoverySelectionMissing {
		queueCallChainDiscoverySelectionRepair(ctx)
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneCallChainDiscoverySelection) {
			ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: preCompleteDowngradeSummary(
					"emit_investigation_complete rejected: the discover-sink call chain has no citable typed runtime-target selection fact. Reuse an already-read selection site and emit exactly one registration, assignment/initializer, or factory return item with explicit subject/object; keep guards and calls as separate facts."),
				Repair: attachToolJSONSurfaceMetadata(t.Name(), &types.ToolRepair{
					Code:   "call_chain_discovery_selection_evidence",
					Hint:   "Emit one citable typed runtime-target selection fact from an already-read source line. " + types.CallChainDiscoverySelectionEmissionGuide,
					Fields: []string{"items[].evidence_kind", "items[].anchor_kind", "items[].subject", "items[].object", "items[].source", "items[].line_start"},
					Metadata: map[string]string{
						"repair_origin": "emit_investigation_complete.call_chain_discovery_selection",
						"lane":          string(types.DowngradeLaneCallChainDiscoverySelection),
					},
				}),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
		ctx.Mutable.AppendCompletionGateNote("runtime target selection remains unproven: no citable registration, assignment/initializer, or factory-return fact connected the discovered destination to the typed call path; the model must keep the destination conditional and may still summarize the proven static calls")
	}
	verifiedStagePrecedence := completionVerifiedReadModeStagePrecedence(ctx)
	flowOperationMissing := resultKind == "resolved" && justification == "" &&
		flowOperationEvidenceRequired(ctx) &&
		!types.HasFlowOperationEvidenceForRequest(evidenceSnapshot, ctx.AnalysisIR.RequestModel) &&
		len(verifiedStagePrecedence) == 0
	if !flowOperationMissing && ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.EvidenceClosure().ClearCompletionCaveat(types.DowngradeLaneFlowOperationCarrier)
		ctx.Mutable.EvidenceClosure().ClearRepairsByDowngradeLane(types.DowngradeLaneFlowOperationCarrier)
	}
	if flowOperationMissing {
		queueFlowOperationCarrierRepair(ctx, evidenceSnapshot)
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneFlowOperationCarrier) {
			ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: preCompleteDowngradeSummary(
					"emit_investigation_complete rejected: the typed flow investigation contains no citable operation-level transfer. Definitions and carrier-field rosters establish identities only. Do one focused source pass over the producer, transfer/merge boundary, and consumer; emit each verified directed operation, or retry completion with the transfer explicitly left unproven."),
				Repair: attachToolJSONSurfaceMetadata(t.Name(), &types.ToolRepair{
					Code:   "flow_operation_carrier_evidence",
					Hint:   types.FlowOperationEvidenceEmissionGuide,
					Fields: []string{"items[].anchor_kind", "items[].subject", "items[].object", "items[].source", "items[].line_start"},
					Metadata: map[string]string{
						"repair_origin": "emit_investigation_complete.flow_operation_carrier",
						"lane":          string(types.DowngradeLaneFlowOperationCarrier),
					},
				}),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
		ctx.Mutable.AppendCompletionGateNote("operation-level flow remains unproven: no citable call, callback, assignment/initializer, return, or precedence row established movement between source components; definitions and field rosters may still be summarized as independent context, but not as an ordered data path")
	}
	missingFlowParticipants := flowParticipantCoverageMissing(ctx, evidenceSnapshot, verifiedStagePrecedence...)
	if len(missingFlowParticipants) == 0 && ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.EvidenceClosure().ClearCompletionCaveat(types.DowngradeLaneFlowParticipantCoverage)
		ctx.Mutable.EvidenceClosure().ClearRepairsByDowngradeLane(types.DowngradeLaneFlowParticipantCoverage)
	}
	if !flowOperationMissing && len(missingFlowParticipants) > 0 {
		queueFlowParticipantCoverageRepair(ctx, missingFlowParticipants, evidenceSnapshot)
		participantBlockerKey := types.ComputeDowngradeTypedIdentifierSetKey(
			string(types.DowngradeLaneFlowParticipantCoverage), missingFlowParticipants,
		)
		if !preCompleteDowngradeConvergesWithTypedBlockerKey(ctx, types.DowngradeLaneFlowParticipantCoverage, participantBlockerKey) {
			ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: preCompleteDowngradeSummary(fmt.Sprintf(
					"emit_investigation_complete rejected: the required source-flow diagram still has no citable operation incident to typed participant(s) %v. The current operation rows may describe real auxiliary edges, but they do not cover the requested participant slate. Do one focused pass over the corresponding source operations and emit the verified directed rows; when source cannot prove a participant relation, keep that participant explicitly unproven instead of inventing a bridge.",
					missingFlowParticipants)),
				Repair: attachToolJSONSurfaceMetadata(t.Name(), &types.ToolRepair{
					Code:   "flow_participant_operation_evidence",
					Hint:   flowParticipantCoverageRepairHint(missingFlowParticipants),
					Fields: []string{"items[].anchor_kind", "items[].subject", "items[].object", "items[].source", "items[].line_start"},
					Metadata: map[string]string{
						"repair_origin": "emit_investigation_complete.flow_participant_coverage",
						"lane":          string(types.DowngradeLaneFlowParticipantCoverage),
					},
				}),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
		ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
			"required source-flow participant relation remains unproven for %v: existing grounded operation rows may still be summarized, but the model must not connect these participants or claim a complete requested flow without incident typed evidence",
			missingFlowParticipants))
	}
	ignoredEvidenceWaiver, evidenceWaiverReject := applyEvidenceFloorWaiverPayload(ctx, t.Name(), p)
	if evidenceWaiverReject != nil {
		return *evidenceWaiverReject, nil
	}
	if err := validateAggregateRequestedDecoratorAlignmentWithEvidence(ctx, aggregateFacts, evidenceSnapshot); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   fmt.Sprintf("emit_investigation_complete rejected: %v", err),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	// Establish exact typed-relation principal ownership before the scope
	// validator reads aggregate roles. Otherwise an omitted-role contextual
	// sibling can be mistaken for a second principal slate and rejected before
	// the typed authority has a chance to classify it as supporting coverage.
	aggregateFacts = markExactTypedRelationPrincipalMemberSets(ctx, aggregateFacts)
	// B18b: when a broad source_inventory analysis shape converges on an exact
	// typed relation roster, prevent an auxiliary/test implementation from
	// entering the principal member_set. This is a hard structured reject:
	// exact provider candidates + deterministic path roles + model-authored
	// aggregate members only. It never scans the request, closure, or answer
	// prose, and unlike the generic low-delta gate it cannot be converged away.
	if violations := relationPrincipalMemberSetScopeViolations(ctx, aggregateFacts); len(violations) > 0 {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: "emit_investigation_complete rejected: the principal relation member_set promotes exact typed relation rows outside the requested source scope: " +
				relationScopeViolationSummary(violations) +
				". Move these rows to excluded[] or an explicitly auxiliary aggregate; retain them in the complete audit roster, but do not count them as principal unless SourceScopeProfile selects that role.",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	aggregateFacts = sourceInventoryPrincipalRowSetLandingFacts(ctx, aggregateFacts)
	// has_per_member_table completion obligation (2026-06-12
	// sequence-table forensics): the analyzer-declared typed shape
	// makes the bounded member set part of the answer, so a resolved
	// completion must hand it over as a member_set aggregate fact —
	// the only channel the answer-side materializer and its coverage
	// gate consume. Typed escape lane (§1.6): absence_justification
	// declares the set genuinely non-enumerable. All four conjuncts
	// are typed fields; no prose is inspected.
	if ctx != nil && ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable &&
		strings.EqualFold(strings.TrimSpace(resultKind), "resolved") &&
		justification == "" &&
		!completionFactsContainMemberSet(aggregateFacts) {
		queuePrincipalMemberSetHandoffRepair(ctx, "has_per_member_table")
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLanePrincipalMemberSetHandoff) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   preCompleteDowngradeSummary(principalMemberSetHandoffDowngradeSummary("has_per_member_table")),
				Repair:    attachToolJSONSurfaceMetadata(t.Name(), principalMemberSetHandoffRepair("has_per_member_table")),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
	}
	preflightStart := time.Now()
	preflight := buildCompletionPreflightView(ctx, resultKind, justification, evidenceSnapshot, aggregateFacts, nil)
	effectiveAggregateFacts := preflight.EffectiveAggregateFacts
	structuredRelationAuthorityFacts := preflight.StructuredRelationAuthorityFacts
	recordToolRuntimeTiming(&runtimeTimings, "completion_preflight_view", preflightStart, len(preflight.Evidence))
	if resultKind == "absence" {
		var notes []string
		effectiveAggregateFacts, notes = dropUnsupportedDecoratedMemberSets(ctx, effectiveAggregateFacts, "absence handoff", false)
		aggregateFactNormalizationNotes = append(aggregateFactNormalizationNotes, notes...)
	} else if completionAggregateFactsAreOptional(ctx, resultKind) &&
		!completionAggregateFactsContainLoadBearingAbsenceProof(effectiveAggregateFacts) {
		var notes []string
		effectiveAggregateFacts, notes = dropUnsupportedDecoratedMemberSets(ctx, effectiveAggregateFacts, "optional aggregate handoff", true)
		aggregateFactNormalizationNotes = append(aggregateFactNormalizationNotes, notes...)
	}
	if len(effectiveAggregateFacts) > 0 {
		var notes []string
		effectiveAggregateFacts, notes = normalizeDecoratedMemberSetFormDebt(ctx, resultKind, effectiveAggregateFacts, evidenceSnapshot)
		aggregateFactNormalizationNotes = append(aggregateFactNormalizationNotes, notes...)
	}
	if nextKind, nextJustification, note := normalizeSourceInventoryAbsenceWithPrincipalAggregates(ctx, resultKind, justification, effectiveAggregateFacts); note != "" {
		resultKind = nextKind
		justification = nextJustification
		aggregateFactNormalizationNotes = append(aggregateFactNormalizationNotes, note)
		preflight = buildCompletionPreflightViewFromEffective(ctx, resultKind, justification, evidenceSnapshot, effectiveAggregateFacts, structuredRelationAuthorityFacts)
	}
	preflight = preflight.WithAggregateFacts(effectiveAggregateFacts, structuredRelationAuthorityFacts)
	publishSourceInventoryAdvisory(ctx, effectiveAggregateFacts, evidenceSnapshot)
	recordToolRuntimeTiming(&runtimeTimings, "aggregate_normalization", aggregateStart, len(effectiveAggregateFacts))

	// Reject the emit when a member_set carries members led by a code
	// identifier but never publishes per-member grounding. Without
	// support_refs the finalizer cannot align row item citations to
	// members and the downstream pre-emit oracle rejects every row that
	// quotes a code-shaped member (the 2026-05-16 architectural-
	// comparison loop: "Orchestrator (4阶段管道整合)" cited but no
	// support_refs → candidate_citations=[] → 4-iter retry → raw-
	// content fallback with cited=0). The enrichment pass above already
	// auto-fills support_refs from grounded evidence when member names
	// match anchor endpoints, so reaching this branch means the model
	// emitted code-shape members whose evidence does not name them
	// verbatim — repair belongs at the explorer turn, not in finalize.
	memberValidationStart := time.Now()
	if err := validateAggregateMemberSetSupportRefs(ctx, effectiveAggregateFacts, evidenceSnapshot); err != nil {
		queueCompletionFormRepair(ctx, "member_set_support_refs", err.Error())
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneCompletionForm) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   preCompleteDowngradeSummary("emit_investigation_complete rejected: " + err.Error()),
				Repair:    attachToolJSONSurfaceMetadata(t.Name(), completionFormRepair("member_set_support_refs", err.Error())),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
	}
	recordToolRuntimeTiming(&runtimeTimings, "decorator_member_validation", memberValidationStart, len(effectiveAggregateFacts))

	// Strict-decode + store principal_span_waiver (typed escape for
	// the source→sink intermediate-evidence gate). Invalid optional
	// payloads are ignored rather than honored; normal precise gates
	// below decide whether span proof is still required.
	if p.ClearPrincipalSpanWaiver && p.PrincipalSpanWaiver != nil {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: "emit_investigation_complete rejected: clear_principal_span_waiver cannot be set together with principal_span_waiver. " +
				"Either declare a new waiver or explicitly retract the existing one, not both.",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	ignoredPrincipalSpanWaiver := ""
	if p.ClearPrincipalSpanWaiver {
		ctx.Mutable.ClearPrincipalSpanWaiver()
		logging.Info("[emit_investigation_complete] principal_span_waiver explicitly cleared")
	} else if p.PrincipalSpanWaiver != nil {
		spanReason := strings.TrimSpace(p.PrincipalSpanWaiver.Reason)
		spanRationale := strings.TrimSpace(p.PrincipalSpanWaiver.Rationale)
		if spanReason == "" {
			ctx.Mutable.ClearPrincipalSpanWaiver()
			ignoredPrincipalSpanWaiver = "ignored principal_span_waiver because reason is missing; normal span gates still apply"
			logging.Warning("[emit_investigation_complete] %s", ignoredPrincipalSpanWaiver)
		} else if spanRationale == "" {
			ctx.Mutable.ClearPrincipalSpanWaiver()
			ignoredPrincipalSpanWaiver = fmt.Sprintf("ignored principal_span_waiver=%s because rationale is missing; normal span gates still apply", spanReason)
			logging.Warning("[emit_investigation_complete] %s", ignoredPrincipalSpanWaiver)
		} else {
			typedSpanReason := types.PrincipalSpanWaiverReason(spanReason)
			if !typedSpanReason.IsValid() {
				ctx.Mutable.ClearPrincipalSpanWaiver()
				ignoredPrincipalSpanWaiver = fmt.Sprintf("ignored principal_span_waiver.reason=%q because it is not accepted; normal span gates still apply", spanReason)
				logging.Warning("[emit_investigation_complete] %s", ignoredPrincipalSpanWaiver)
			} else {
				ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
					Reason:    typedSpanReason,
					Rationale: spanRationale,
				})
				logging.Info("[emit_investigation_complete] principal_span_waiver accepted: reason=%s rationale=%q",
					typedSpanReason, truncateForLog(spanRationale, 200))
			}
		}
	}

	// G1 (Plan E, 2026-05-02) — pre-complete coverage gate.
	// When the user's question carries a structural obligation
	// (CompletenessObligation.Required OR EnumerationBoundary
	// declared) AND result_kind=resolved, refuse the emit if the
	// closure's ScannedSet (files surfaced by grep / repo_map /
	// list_files) contains files NOT in ReadSet. The signal is
	// "the explorer surfaced N candidate files but only read K<N"
	// — exactly the s5a pathology where 8 LoopController
	// implementations were grep-known but only 3 read_file'd.
	//
	// Bypass conditions:
	//   - result_kind=absence: honest zero, scanning without reading
	//     is fine (you read enough to confirm absence)
	//   - no obligation declared: pre-Plan-E behaviour preserved
	//   - ScannedSet ⊆ ReadSet: every candidate covered, full pass
	//   - Tools failed to populate ScannedSet (test-mode shortcut):
	//     no candidate signal, gate skips
	// G1 (Plan E rollout 2026-05-02) was REVERTED 2026-05-02 after
	// the s1a-125430 trace surfaced a systemic over-firing problem.
	//
	// Architectural lesson: investigation-time enforcement of
	// "exhaustive coverage" needs a precise per-question candidate
	// set (e.g. files that grep matched the user's primary entity
	// during this dispatch). closure.ScannedSet is the breadth-scan
	// keyword ranker output, which surfaces files by import-count and
	// loose similarity — for s1a's "gate.Run 的 9 项检查" question the
	// ranker offered 12+ files including emit_answer_document.go and
	// agent.go (popular but unrelated). G1 forced the explorer to
	// read every one, looping into timeout.
	//
	// Replacement: enforce on the ANSWER side, not the investigation
	// side.
	//   - G2 (emit_answer_symbol) precisely rejects completeness=
	//     lower_bound under CompletenessObligation. The signal is
	//     question-precise (a single boolean flag) instead of
	//     ranker-noisy.
	//   - G3 (emit_answer_document) precisely rejects bucket-label
	//     omission. Single-bucket questions never trigger.
	//   - The explore-skill carries soft "COVERAGE BEFORE COMPLETION"
	//     guidance (E-7) — the LLM is encouraged to read every grep
	//     hit on completeness questions, but not structurally forced.
	//
	// Future option: a precise per-question candidate tracker
	// (closure-side "files with grep-matches against analyzer's
	// PrimaryEntities") could supersede the soft guidance with a
	// hard gate. Out of scope for this commit.

	// Grounding gates. Two independent floors evaluated in AND:
	//
	//   1. GroundingFloor — (grounded + recovered) / total. Blocks
	//      mostly-speculative investigations.
	//   2. Tier1Floor — grounded-via-TierLineText / total. Blocks
	//      "pure-recovery" investigations where the LLM never
	//      actually read a file; the recovery tiers filled every
	//      LineStart from the repomap graph but the finalizer
	//      grounder (stricter Tier 2) cannot re-cite the same
	//      anchors, leaving the pipeline in a loop.
	//
	// Items emitted before the gate apply cumulatively in this
	// dispatch's Mutable buffer. Honest-zero absence claims skip
	// these floors, then pass through the dedicated absence validation
	// below; otherwise absent targets can be rejected for having no
	// positive evidence to cite.
	groundingFloorStart := time.Now()
	if justification == "" {
		contract := answerExactResolutionContract(ctx)
		evidence := evidenceSnapshot
		if targets := completionPendingExactTargets(ctx, contract, evidence); len(targets) > 0 {
			if !evidenceHasAnyDefiningExactTargetProof(contract, evidence, targets) {
				// Defining-proof advisory (softened per §29.60 2026-07-13):
				// "the exact target has no grounded defining anchor yet" is a
				// proof-strength judgment, not a structural invalidity — the
				// model's completion may be right on nearby evidence (witness
				// codrax-20260713-053445: this arm burned 3 identical rounds
				// before the convergence lane let the same completion pass).
				// The typed detection (completionPendingExactTargets +
				// evidenceHasAnyDefiningExactTargetProof) is preserved
				// verbatim; the result now rides the accepted completion as a
				// gate note plus a typed caveat (§29.42.4 detect→disclose)
				// instead of delaying closure. Answer-side citation gates
				// still guard any concrete anchor the answer actually cites.
				label := "target"
				if contract != nil && strings.TrimSpace(contract.TargetLabel) != "" {
					label = strings.TrimSpace(contract.TargetLabel)
				}
				ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
					"exact-%s proof note: no grounded defining anchor names the exact %s yet — the emitted evidence supports nearby or contextual material. If the answer states where the %s is defined or asserts its exact behavior, present that part as unverified (or ground a defining anchor first)",
					label, label, label))
				if closure := ctx.Mutable.EvidenceClosure(); closure != nil {
					closure.AppendCompletionCaveat(types.CompletionCaveat{
						Lane:       types.DowngradeLaneExactResolvedDefiningProof,
						ReasonCode: "defining_proof_missing_disclosed",
						Reason:     "no grounded defining anchor names the exact target; completion proceeded with disclosure",
					})
				}
				logging.Info("[emit_investigation_complete] exact defining-proof advisory (disclose, not delay): label=%s targets=%d", label, len(targets))
			}
		}
		policy := CurrentGroundingPolicy()
		if msg, ok := groundingGateRejectWithPreflight(ctx, policy.GroundingFloor, preflight); !ok {
			if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneGroundingCitationFloor) {
				if ctx != nil && ctx.Mutable != nil {
					ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
				}
				return types.ToolResult{
					ToolName:  t.Name(),
					Summary:   preCompleteDowngradeSummary(msg),
					Repair:    attachToolJSONSurfaceMetadata(t.Name(), groundingCitationFloorRepair("grounding_floor", "Repair, re-anchor, or drop ungrounded evidence items until the typed grounding floor is satisfied; if the same blocker cannot progress, completion will proceed with a caveat.")),
					Success:   true,
					Timestamp: time.Now(),
				}, nil
			}
			earlyDowngradeConverged = true
		}
		if !earlyDowngradeConverged {
			if msg, ok := tier1GateRejectWithPreflight(ctx, policy.Tier1Floor, preflight); !ok {
				if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneGroundingCitationFloor) {
					if ctx != nil && ctx.Mutable != nil {
						ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
					}
					return types.ToolResult{
						ToolName:  t.Name(),
						Summary:   preCompleteDowngradeSummary(msg),
						Repair:    attachToolJSONSurfaceMetadata(t.Name(), groundingCitationFloorRepair("tier1_citation_floor", "Read the cited sources through read_file or equivalent typed source coverage, then re-emit grounded evidence so citation rendering can preserve the anchors.")),
						Success:   true,
						Timestamp: time.Now(),
					}, nil
				}
				earlyDowngradeConverged = true
			}
		}
	}
	recordToolRuntimeTiming(&runtimeTimings, "grounding_citation_floors", groundingFloorStart, len(evidenceSnapshot))

	if msg := completionPathDiscoveryAbsenceProofReject(ctx, resultKind, effectiveAggregateFacts); msg != "" {
		queuePathDiscoveryAbsenceRepair(ctx)
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLanePathDiscoveryAbsence) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName: t.Name(),
				Summary:  preCompleteDowngradeSummary(msg),
				Repair: attachToolJSONSurfaceMetadata(t.Name(), &types.ToolRepair{
					Code: "path_discovery_absence_proof",
					Hint: "Complete the typed file-family absence proof with recursive path discovery that includes repo-owned auxiliary/corpus trees, or with an authoritative source_inventory lens.",
					Metadata: map[string]string{
						"result_kind":   "absence",
						"repair_origin": "emit_investigation_complete.path_discovery_absence",
					},
				}),
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
		earlyDowngradeConverged = true
	}

	// Declarative absence claim. Stored on Mutable so the orchestrator
	// can waive citation-floor gates for honest-zero answers. The
	// audit (hasAnyInvestigationSuccess) still runs — an LLM cannot
	// escape by declaring absence with zero tool work.
	//
	// Absence-vs-grounded-evidence contradiction gate. The LLM has
	// previously learned to shortcut citation-floor gates by tacking
	// absence_justification onto every emit_investigation_complete
	// call. Reject the combination when the evidence buffer already
	// contains ≥1 grounded or recovered item — by definition that is
	// not a zero answer, and accepting the claim bypasses the finalize
	// citation gate for a question that DOES have file:line anchors.
	// The rejection message tells the LLM exactly what to do: drop
	// the field and re-emit. This runs BEFORE SetInvestigationComplete
	// so the LLM sees the error and corrects in the same dispatch.
	if justification != "" {
		if resultKind != "absence" {
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: "emit_investigation_complete rejected: absence_justification requires result_kind=absence. " +
					"For ordinary positive/citable answers, set result_kind=resolved and omit absence_justification.",
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
		if evidence := preflight.Evidence; preflight.GenericEvidenceTally.hasAny() &&
			!completionFactsContainPrincipalEmptyMemberSetWithTypedNoHitSupport(ctx, effectiveAggregateFacts) &&
			!allowsContextualEvidenceForAbsence(ctx, evidence) {
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: "emit_investigation_complete rejected: absence_justification is reserved for honest-zero answers " +
					"(the question genuinely has nothing to cite — e.g. 'how many .py files?' → 0, 'does handler X exist?' → no). " +
					"This investigation already recorded grounded/recovered evidence items via emit_evidence, so the answer is NOT an absence. " +
					"Remove absence_justification and re-call emit_investigation_complete with just reason + confidence.",
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
	}
	if resultKind == "absence" && justification == "" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: "emit_investigation_complete rejected: result_kind=absence requires absence_justification. " +
				"State in one short sentence what was searched and why the terminal answer is genuinely empty.",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	if resultKind == "absence" && justification != "" {
		contract := exactResolutionContractForCompletion(ctx)
		requiredFiles := types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
		if len(requiredFiles) > 0 {
			if ctx != nil && ctx.Mutable != nil {
				closure := ctx.Mutable.EvidenceClosure()
				refreshClosureReadSnapshot(ctx, closure)
				closure.DrainSatisfiedPendingReads()
				closure.DrainVerifiedUnverifiedFindings()
			}
			evidence := evidenceSnapshot
			scenario := types.ScenarioGeneric
			if ctx.AnalysisIR != nil {
				scenario = ctx.AnalysisIR.RequestModel.Scenario
			}
			if !evidenceHasGroundedRelatedContextProof(contract, scenario, evidence, requiredFiles) {
				repairKind, repairFiles := exactAbsenceContextRepairKind(ctx, contract, scenario, evidence, requiredFiles)
				configTraceMultiLayer := scenario == types.ScenarioConfigTrace &&
					contract != nil &&
					contract.TargetKind == types.SubjectConfigKey &&
					types.ConfigTraceNeedsMultiLayerClosure(requiredFiles)
				validatedRoleCount := 0
				if scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					validatedRoleCount = types.ConfigTraceValidatedDiagramRoleCount(contract, requiredFiles, evidence)
				}
				summary := fmt.Sprintf(
					"emit_investigation_complete rejected: this exact-absence answer still lacks a grounded production related-context anchor from the current same-scope candidate set. Read one of these repo_map-ranked files, emit at least one grounded related_context fact from it, then re-call emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...). Pending same-scope files: %s",
					strings.Join(repairFiles, ", "),
				)
				repairOrigin := "emit_investigation_complete.exact_absence_context"
				rationale := "exact-absence closure is still missing a grounded same-scope related-context anchor; read one of these files, emit related_context evidence, then retry emit_investigation_complete."
				hint := "Read one of the queued same-scope files, emit at least one grounded `related_context` fact from it, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
				repairCode := "exact_absence_context_anchor"
				if scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					requestedRoles := types.ConfigTraceMissingRequestedDiagramRoles(contract, requiredFiles, evidence)
					requestedRoleText := types.JoinEvidenceDiagramRoles(requestedRoles)
					if configTraceMultiLayer && validatedRoleCount > 0 {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks grounded precedence coverage for the user-requested role(s) `%s` from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit grounded related_context facts that carry explicit diagram_role_hint values so downstream answers can cite a real precedence chain across every requested role. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Read one of the queued same-scope files, emit grounded `related_context` facts with validated precedence `diagram_role_hint` values, and keep going until the requested roles `%s` each have a grounded anchor before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks enough grounded precedence coverage from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit grounded related_context facts that carry explicit diagram_role_hint values (`default`, `config`, `runtime`, or `override`) so downstream answers can cite a real multi-layer precedence chain. When the same-scope search already spans multiple files, preserve at least two validated precedence roles instead of closing on a single layer. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Read one of the queued same-scope files, emit grounded `related_context` facts with validated precedence `diagram_role_hint` values, and when the current scope already spans multiple files keep going until you have at least two validated precedence roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						repairOrigin = "emit_investigation_complete.exact_absence_precedence"
						rationale = "exact-absence config-trace closure still lacks enough grounded precedence coverage; read one of these files, emit related_context evidence with validated diagram roles, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_anchor"
					} else {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence anchor for the user-requested role(s) `%s` from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit at least one grounded related_context fact with an explicit diagram_role_hint so downstream answers can cite that requested role. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Read one of the queued same-scope files, emit at least one grounded `related_context` fact with a validated precedence `diagram_role_hint`, and keep going until the requested roles `%s` have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence-capable lineage anchor from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit at least one grounded related_context fact that carries an explicit diagram_role_hint (`default`, `config`, `runtime`, or `override`) so downstream answers can cite a real precedence anchor. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Read one of the queued same-scope files, emit at least one grounded `related_context` fact with a validated precedence `diagram_role_hint`, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						repairOrigin = "emit_investigation_complete.exact_absence_precedence"
						rationale = "exact-absence config-trace closure still lacks a grounded precedence-capable same-scope anchor; read one of these files, emit related_context evidence with a validated diagram role, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_anchor"
					}
				}
				if repairKind == types.RepairEmitEvidence {
					summary = fmt.Sprintf(
						"emit_investigation_complete rejected: this exact-absence answer still lacks a grounded same-scope related-context anchor on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read file(s), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. Same-scope files: %s",
						strings.Join(repairFiles, ", "),
					)
					rationale = "exact-absence closure is still missing a grounded same-scope related-context anchor, but the relevant file scope has already been read; stay on those anchors, emit grounded related_context evidence, then retry emit_investigation_complete."
					hint = "Do NOT read neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read same-scope file(s), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
					repairCode = "exact_absence_context_evidence"
				}
				if repairKind == types.RepairEmitEvidence && scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					requestedRoles := types.ConfigTraceMissingRequestedDiagramRoles(contract, requiredFiles, evidence)
					requestedRoleText := types.JoinEvidenceDiagramRoles(requestedRoles)
					if configTraceMultiLayer && validatedRoleCount > 0 {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks grounded precedence coverage for the user-requested role(s) `%s` on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values until each requested role has a grounded anchor, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values until the requested roles `%s` each have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks enough grounded precedence coverage on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values (`default`, `config`, `runtime`, or `override`), and when the current same-scope span already covers multiple files preserve at least two validated roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values, and when the current same-scope span already covers multiple files keep going until you have at least two validated roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						rationale = "exact-absence config-trace closure still lacks enough grounded precedence coverage, but the relevant file scope has already been read; stay on those anchors, re-emit related_context evidence with validated diagram roles, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_evidence"
					} else {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence anchor for the user-requested role(s) `%s` on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values until each requested role has a grounded anchor, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values until the requested roles `%s` each have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence-capable lineage anchor on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read file(s) with a validated precedence `diagram_role_hint` (`default`, `config`, `runtime`, or `override`), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Do NOT read neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read same-scope file(s) with a validated precedence `diagram_role_hint`, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						rationale = "exact-absence config-trace closure still lacks a grounded precedence-capable same-scope anchor, but the relevant file scope has already been read; stay on those anchors, re-emit related_context evidence with a validated diagram role, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_evidence"
					}
				}
				requiredDiagramRoles := types.JoinEvidenceDiagramRoles(types.ConfigTraceRequestedDiagramRoles(contract))
				if requiredDiagramRoles == "" {
					requiredDiagramRoles = "default,config,runtime,override"
				}
				repair := completionFileRepair(
					repairKind,
					repairCode,
					hint,
					repairFiles,
					map[string]string{
						"result_kind":            "absence",
						"context_role":           string(types.EvidenceContextRoleRelatedContext),
						"required_diagram_roles": requiredDiagramRoles,
						"repair_origin":          repairOrigin,
					},
				)
				queueCompletionRepairs(ctx, repairKind, repairFiles, rationale, repairOrigin)
				if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneExactAbsenceContext) {
					if ctx != nil && ctx.Mutable != nil {
						ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
					}
					return types.ToolResult{
						ToolName:  t.Name(),
						Summary:   preCompleteDowngradeSummary(summary),
						Repair:    attachToolJSONSurfaceMetadata(t.Name(), repair),
						Success:   true,
						Timestamp: time.Now(),
					}, nil
				}
				earlyDowngradeConverged = true
			}
		}
	}

	// CGEC E1: pre-complete contract simulation. Before flipping the
	// investigationComplete flag, simulate whether the finalizer's
	// AnswerContract would actually pass on the current evidence +
	// ReadSet snapshot. The two cheap predictive checks:
	//
	//   (a) PendingReads non-empty — the chain promotion enforcer
	//       or a previous emit_answer_document grounder reject
	//       queued forced reads. Completing now means the LLM
	//       skipped them; force a downgrade.
	//
	//   (b) Cite-eligible evidence count below the analyzer's
	//       MinCitations floor — the LLM hasn't gathered enough
	//       file:line anchors to satisfy citation_count_ge.
	//
	// Either failure causes the tool to return a downgraded result
	// (Success=true so the LLM sees the explanation but does NOT
	// flip investigationComplete). The explorer's ShouldStop sees
	// the flag still false and continues the loop.
	preCompleteStart := time.Now()
	// A requested source-code call-chain endpoint is a precise typed identity
	// and direction contract. It must not share the generic low-delta escape:
	// retry count cannot turn an absent exact endpoint/path into evidence. Run
	// the existing single-source predicate before every convergent lane. Once
	// the predicate passes, preCompleteContractCheckWithPreflight may call it
	// again harmlessly as part of its direct-call/test contract.
	if justification == "" {
		if downgrade := callChainExactEndpointReachabilityDowngradeWithEvidence(ctx, ctx.Mutable.EvidenceClosure(), preflight.Evidence); downgrade != "" {
			recordToolRuntimeTiming(&runtimeTimings, "pre_complete_call_chain_exact_endpoint", preCompleteStart, len(preflight.Evidence))
			ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
	}
	// Low-delta convergence boundary (gap 1): each gate, when it fires, checks
	// whether the model has re-attempted emit_investigation_complete with the
	// SAME lane + SAME typed blocker (pending reads / unverified findings /
	// raised repairs) and no new typed delta. After the hard threshold of such
	// no-progress re-attempts, the gate FALLS THROUGH to completion with a
	// typed caveat instead of re-issuing the identical downgrade forever — the
	// runaway "verification not stable" loop. Incidental evidence churn does not
	// reset convergence; only a genuinely changed blocker does. The else-if
	// chain preserves "first firing gate wins" and lets a converged gate skip
	// the rest and complete.
	if earlyDowngradeConverged {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_gate_chain", preCompleteStart, len(preflight.Evidence))
	} else if downgrade := currentSourceLaneCoverageDowngrade(ctx, preflight); downgrade != "" {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_current_source_lane", preCompleteStart, len(preflight.Evidence))
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneCurrentSourceLane) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
	} else if downgrade := sourceInventoryClassUniverseAbsenceDowngrade(ctx, resultKind, mergeCompletionAggregateFacts(effectiveAggregateFacts, aggregateFacts)); downgrade != "" {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_source_class_universe", preCompleteStart, len(preflight.Evidence))
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneSourceClassUniverse) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
	} else if downgrade, sourceInventoryFacts := sourceInventoryResolvedCompletionDowngradeForFacts(ctx, resultKind, effectiveAggregateFacts, aggregateFacts); downgrade != "" {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_source_inventory_completion", preCompleteStart, len(preflight.Evidence))
		if !preCompleteSourceInventoryDowngradeConverges(ctx, sourceInventoryFacts) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
	} else if downgrade := sourceInventoryLensExecutionDowngrade(ctx, effectiveAggregateFacts); downgrade != "" {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_source_inventory_lens", preCompleteStart, len(preflight.Evidence))
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneSourceInventoryLens) {
			if ctx != nil && ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true,
				Timestamp: time.Now(),
			}, nil
		}
	} else if downgrade := preCompleteContractCheckWithPreflight(ctx, justification, preflight); downgrade != "" {
		recordToolRuntimeTiming(&runtimeTimings, "pre_complete_gate_chain", preCompleteStart, len(preflight.Evidence))
		if !preCompleteDowngradeConverges(ctx, types.DowngradeLaneContractChain) {
			if ctx != nil && ctx.Mutable != nil {
				closure := ctx.Mutable.EvidenceClosure()
				closure.BumpPreCompleteDowngrades(1)
				// Session 11 F1: pre-complete downgrade is a compound
				// signal — something (missing reads, zero citations, shape
				// mismatch, unverified finds) blocked completion. The
				// downgrade message body carries the reason for the
				// operator; we record a ledger entry so F2 can aggregate
				// "closure blocked N times with same root" into a direct
				// IR patch request. Confidence 0.70 — the downgrade
				// doesn't pinpoint which IR field without more context,
				// so we leave CitationReq as the default blame and let
				// the paired Repair (always raised by preCompleteContractCheck)
				// carry the kind-specific detail.
				closure.AppendViolation(types.Violation{
					Kind:       types.ViolPreCompleteDowngrade,
					Detail:     "pre-complete simulator rejected emit_investigation_complete",
					ClusterKey: types.ComposeClusterKey("root", "CitationReq", "stage", "pre_complete"),
					Stage:      string(types.StageExplore),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "CitationReq",
						Reason:     "closure snapshot fails preflight; evidence insufficient or citations outside the already-read file set",
						Confidence: 0.70,
					},
				})
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   downgrade,
				Success:   true, // soft signal so the loop continues without surfacing a tool error
				Timestamp: time.Now(),
			}, nil
		}
	}
	recordToolRuntimeTiming(&runtimeTimings, "pre_complete_gate_chain", preCompleteStart, len(preflight.Evidence))

	stateWriteStart := time.Now()
	reason = normalizeLogSourceDriftCompletionReason(ctx, reason)
	ctx.Mutable.SetInvestigationAggregateFacts(effectiveAggregateFacts)
	ctx.Mutable.SetInvestigationRelationClaims(relationClaims)
	appendPrincipalSpanWaiverCompletionNote(ctx)
	ctx.Mutable.SetInvestigationComplete(reason)
	ctx.Mutable.SetInvestigationResultKind(resultKind)
	ctx.Mutable.RetainInvestigationAggregateFacts()
	ctx.Mutable.RetainInvestigationRelationClaims()
	// A1 anchor advisory (soft): verified-but-unconsumed anchors ride a
	// gate note into answer writing so the finalizer can disclose the
	// boundary — never a denial (anchor relevance is a judgment call).
	if ctx.AnalysisIR != nil {
		if unconsumed := types.UnconsumedAnchorObligations(
			types.CompileAnchorObligations(ctx.AnalysisIR),
			ctx.Mutable.EvidenceClosure(),
			ctx.Mutable.EmittedEvidence(),
		); len(unconsumed) > 0 {
			ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
				"analysis-verified anchors without read/evidence proof in this investigation: %s — if the answer relies on one of them, present it as unverified context",
				types.SummarizeAnchorObligations(unconsumed)))
		}
	}
	// RN-14c (§7.9, A1 soft-advisory paradigm): a scheduler-shaped trace turn
	// completing with a pinned user-focus thread but no wakeup-chain-family
	// observation targeting that thread rides a gate note into the accepted
	// completion — never a denial: chain absence can be a data fact, but the
	// answer should disclose it, and RN-14a's via_thread gives the model a
	// one-call way to connect the runnable anchor to the focus thread.
	if note := userFocusWakeupChainGapNote(ctx); note != "" {
		ctx.Mutable.AppendCompletionGateNote(note)
	}
	// R5 wakeup-chain drilldown advisory (§7.30 裁定3, softened per §29.60
	// 2026-07-13, same paradigm as RN-14c above): a census-ranked
	// chain_required=true sleep subject whose trace artifact has no
	// wakeup-chain-family observation used to hard-downgrade the completion
	// in the pre-complete chain. The detection (reconcile-first, per-artifact
	// one-shot) is unchanged; the result now rides the accepted completion as
	// a gate note plus a typed caveat instead of re-opening the tool loop.
	if note := wakeupChainDrilldownPendingAdvisory(ctx); note != "" {
		ctx.Mutable.AppendCompletionGateNote(note)
		if closure := ctx.Mutable.EvidenceClosure(); closure != nil {
			closure.AppendCompletionCaveat(types.CompletionCaveat{
				Lane:       types.DowngradeLaneWakeupChainDrilldown,
				ReasonCode: "wakeup_chain_drilldown_pending",
				Reason:     "a census-ranked sleep subject on this trace artifact has no wakeup-chain drilldown; completion proceeded with disclosure",
			})
		}
	}
	// Promote the waiver only when THIS accepted attempt declared it and
	// the declaration was not ignored — a stale pending waiver from an
	// earlier denied attempt must not arm the finalize citation-floor
	// relax for a completion that stood on repo evidence.
	ctx.Mutable.RetainEvidenceFloorWaiver(p.EvidenceFloorWaiver != nil && ignoredEvidenceWaiver == "")
	if drained := ctx.Mutable.EvidenceClosure().ClearPendingReads(); drained > 0 {
		logging.Info("[emit_investigation_complete] cleared %d stale pending forced-read directive(s) after accepted completion", drained)
	}
	if drained := ctx.Mutable.EvidenceClosure().ClearRepairs(); drained > 0 {
		logging.Info("[emit_investigation_complete] cleared %d stale repair directive(s) after accepted completion", drained)
	}
	visibleReason := reason
	if waiver := ctx.Mutable.PrincipalSpanWaiver(); waiver != nil && waiver.IsActive() &&
		waiver.Reason == types.PrincipalSpanWaiverNoDirectedPath {
		startHint, endHint := "source endpoint", "sink endpoint"
		if ctx.AnalysisIR != nil {
			if source, sink, ok := types.CallChainOrderedEndpointHints(ctx.AnalysisIR.RequestModel); ok {
				startHint, endHint = source, sink
			}
		}
		visibleReason = fmt.Sprintf(
			"typed call-chain endpoint boundary accepted: no directed path from `%s` to `%s`; keep nearest proven and reverse/parallel call edges separate",
			startHint,
			endHint,
		)
	}
	summary := fmt.Sprintf("Investigation marked complete (confidence=%s, result_kind=%s): %s", conf, resultKind, visibleReason)
	if justification != "" {
		ctx.Mutable.SetAbsenceJustification(justification)
		summary += fmt.Sprintf(" | absence_justification: %s", justification)
	}
	if len(effectiveAggregateFacts) > 0 {
		summary += fmt.Sprintf(" | aggregate_facts: %d", len(effectiveAggregateFacts))
	}
	if len(relationClaims) > 0 {
		summary += fmt.Sprintf(" | relation_claims: %d", len(relationClaims))
	}
	if len(aggregateFactNormalizationNotes) > 0 {
		summary += " | aggregate_facts normalized: " + strings.Join(aggregateFactNormalizationNotes, "; ")
	}
	if ignoredEvidenceWaiver != "" {
		summary += " | " + ignoredEvidenceWaiver
	}
	if ignoredPrincipalSpanWaiver != "" {
		summary += " | " + ignoredPrincipalSpanWaiver
	}
	if gateNote := ctx.Mutable.TakeCompletionGateNote(); gateNote != "" {
		summary += " | " + gateNote
	}
	recordToolRuntimeTiming(&runtimeTimings, "completion_state_write", stateWriteStart, len(effectiveAggregateFacts))

	return types.ToolResult{
		ToolName:  t.Name(),
		Summary:   summary,
		Success:   true,
		Timestamp: time.Now(),
	}, nil
}

func pendingBlockingEmitEvidenceItemValidationRepair(ctx *types.BusContext) *types.ToolRepair {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	results := ctx.Mutable.DispatchToolResults()
	for i := len(results) - 1; i >= 0; i-- {
		result := results[i]
		if types.CanonicalToolName(result.ToolName) != "emit_evidence" {
			continue
		}
		if result.Repair == nil || result.Repair.Code != types.ToolRepairCodeEvidenceItemValidation ||
			result.Repair.Metadata == nil || result.Repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired ||
			result.Repair.Metadata["completion_blocking"] != "true" {
			return nil
		}
		return cloneEmitInvestigationToolRepair(result.Repair)
	}
	return nil
}

func cloneEmitInvestigationToolRepair(in *types.ToolRepair) *types.ToolRepair {
	if in == nil {
		return nil
	}
	out := *in
	out.Fields = append([]string(nil), in.Fields...)
	out.Targets = append([]types.ToolRepairTarget(nil), in.Targets...)
	for i := range out.Targets {
		out.Targets[i].Lines = append([]int(nil), in.Targets[i].Lines...)
	}
	if in.Metadata != nil {
		out.Metadata = make(map[string]string, len(in.Metadata))
		for key, value := range in.Metadata {
			out.Metadata[key] = value
		}
	}
	return &out
}

func appendPrincipalSpanWaiverCompletionNote(ctx *types.BusContext) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	waiver := ctx.Mutable.PrincipalSpanWaiver()
	if waiver == nil || !waiver.IsActive() || waiver.Reason != types.PrincipalSpanWaiverNoDirectedPath {
		return
	}
	startHint, endHint := "source endpoint", "sink endpoint"
	if ctx.AnalysisIR != nil {
		if source, sink, ok := types.CallChainOrderedEndpointHints(ctx.AnalysisIR.RequestModel); ok {
			startHint = source
			endHint = sink
		}
	}
	ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
		"model-declared typed call-chain boundary: principal_span_waiver=no_directed_path for `%s` -> `%s`. The accepted call-edge graph did not establish that direction; keep the nearest proven path and endpoint boundary separate. Show a reverse/parallel relationship only when its own typed call edge is present, and do not turn endpoint definitions into a call edge.",
		startHint, endHint))
	ctx.Mutable.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLaneContractChain,
		ReasonCode: "call_chain_no_directed_path",
		Reason:     "the model declared a typed no_directed_path boundary after source inspection",
	})
}

func sourceInventoryResolvedCompletionDowngradeForFacts(
	ctx *types.BusContext,
	resultKind string,
	effective, current []types.AnswerAggregateFact,
) (string, []types.AnswerAggregateFact) {
	facts := mergeCompletionAggregateFacts(effective, current)
	return sourceInventoryResolvedCompletionDowngrade(ctx, resultKind, facts), facts
}

// preCompleteSourceInventoryDowngradeConverges bounds noisy navigation debt,
// but never force-completes a precise typed row omission. A complete exact lens
// already supplies every missing row needed for a cheap structured retry; using
// convergence here would accept a known-incomplete handoff and force the answer
// publisher to invent the omitted principal row later.
func preCompleteSourceInventoryDowngradeConverges(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if sourceInventoryResolvedCompletionPreciseCoverageGap(ctx, facts).Blocking {
		return false
	}
	return preCompleteDowngradeConverges(ctx, types.DowngradeLaneSourceInventoryCompletion)
}

func validateAggregateRequestedDecoratorAlignment(ctx *types.BusContext, facts []types.AnswerAggregateFact) error {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return validateAggregateRequestedDecoratorAlignmentWithEvidence(ctx, facts, evidence)
}

func validateAggregateRequestedDecoratorAlignmentWithEvidence(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) error {
	if ctx == nil || ctx.Mutable == nil || len(facts) == 0 {
		return nil
	}
	requested := requestedDecoratorSurfaceTerms(ctx)
	if len(requested) == 0 {
		return nil
	}
	gc := ground.BuildContext(ctx)
	if len(evidence) == 0 {
		return nil
	}
	for i, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 {
			continue
		}
		claimed := aggregateFactClaimedRequestedDecorator(fact, requested)
		if claimed == "" {
			continue
		}
		for _, member := range fact.Members {
			ev, ok := aggregateMemberEvidence(member, evidence)
			if !ok {
				continue
			}
			actual := attachedDecoratorSurfaceTermSet(ev, gc)
			if len(actual) == 0 || actual[claimed] {
				continue
			}
			actualList := make([]string, 0, len(actual))
			for term := range actual {
				actualList = append(actualList, term)
			}
			sort.Strings(actualList)
			return fmt.Errorf(
				"aggregate_facts[%d] member %q is listed under requested decorator %q, but its typed evidence at %s:%d is attached to %s; correct the member_set or move the member to a separate related-context aggregate",
				i, member, claimed, ev.Source, ev.LineStart, strings.Join(actualList, ", "))
		}
	}
	return nil
}

func aggregateFactClaimedRequestedDecorator(fact types.AnswerAggregateFact, requested map[string]bool) string {
	add := []string{fact.Label}
	for _, d := range fact.Dimensions {
		add = append(add, d.Name, d.Value)
	}
	for _, raw := range add {
		term := decoratorSurfaceTermFromLabel(raw)
		if term != "" && requested[term] {
			return term
		}
	}
	return ""
}

func aggregateMemberEvidence(member string, evidence []types.EvidenceItem) (types.EvidenceItem, bool) {
	want := aggregateMemberKey(member)
	if want == "" {
		return types.EvidenceItem{}, false
	}
	for _, ev := range evidence {
		for _, raw := range aggregateEvidenceMemberLabels(ev) {
			if aggregateMemberKey(raw) == want {
				return ev, true
			}
		}
	}
	return types.EvidenceItem{}, false
}

func aggregateEvidenceMemberLabels(ev types.EvidenceItem) []string {
	out := []string{ev.AnchorSymbol, ev.Subject, ev.Object}
	out = append(out, ev.SurfaceTerms...)
	return out
}

func aggregateMemberKey(raw string) string {
	return types.NormalizedSurfaceSymbolTail(raw)
}

// investigationCompletePolicy holds the operator-configured policy
// from codrax.yaml's agent_investigation_complete_policy. cmd/root.go
// calls SetInvestigationCompletePolicy at startup. Mirrors the same
// SetXxx pattern as SetBlobLimits / SetAnalysisLimits / SetGroundingPolicy.
//
// Default is the empty string (effectively "soft" — preCompleteContractCheck
// gates fire normally). When set to "override", the pre-complete
// gates are skipped to honor the per-task scheduler's "skip all
// criteria" mode (see orchestrator.go:454-468).
var investigationCompletePolicy string

// SetInvestigationCompletePolicy is the configuration entrypoint
// from cmd/root.go. Pass the empty string to restore default
// behavior.
func SetInvestigationCompletePolicy(policy string) {
	investigationCompletePolicy = strings.TrimSpace(policy)
}

// downgradeConvergenceHardThreshold is the number of CONSECUTIVE no-progress
// pre-complete downgrade re-attempts (same lane + same typed blocker) after
// which the convergence boundary force-completes with a typed caveat instead of
// re-issuing the identical downgrade. Default 4 is conservative: a healthy run
// completes in 1-2 attempts or makes progress (changing the blocker and
// resetting the count), so only a genuine runaway reaches the threshold.
var downgradeConvergenceHardThreshold = 3

// SetDowngradeConvergenceHardThreshold configures the low-delta convergence
// hard threshold from cmd/root.go. Non-positive values restore the default.
func SetDowngradeConvergenceHardThreshold(n int) {
	if n <= 0 {
		downgradeConvergenceHardThreshold = 3
		return
	}
	downgradeConvergenceHardThreshold = n
}

// preCompleteDowngradeConverges records a typed downgrade fingerprint for the
// given lane and reports whether the model has now re-attempted
// emit_investigation_complete with the SAME lane + SAME typed blocker enough
// consecutive times (the hard threshold) that the loop should force-complete.
// It reads only typed closure state (pending reads, unverified findings, active
// repairs) — never the downgrade Summary prose — so the boundary routes on
// precise signals. On convergence it records a typed CompletionCaveat for
// downstream answer-caveat / telemetry consumers.
func preCompleteDowngradeConverges(ctx *types.BusContext, lane types.DowngradeLane) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return false
	}
	blockerKey := types.ComputeDowngradeBlockerKey(closure.PendingReads(), closure.UnverifiedFindings(), closure.ActiveRepairs())
	return preCompleteDowngradeConvergesWithClosureAndBlockerKey(ctx, closure, lane, blockerKey)
}

// preCompleteDowngradeConvergesWithTypedBlockerKey is the narrow lane-owned
// variant for a gate that has a more precise blocker than the shared closure.
// It does not let unrelated reads/findings/repairs reset convergence while the
// gate's own typed blocker is unchanged.
func preCompleteDowngradeConvergesWithTypedBlockerKey(ctx *types.BusContext, lane types.DowngradeLane, blockerKey uint32) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return false
	}
	return preCompleteDowngradeConvergesWithClosureAndBlockerKey(ctx, closure, lane, blockerKey)
}

func preCompleteDowngradeConvergesWithClosureAndBlockerKey(ctx *types.BusContext, closure *types.EvidenceClosure, lane types.DowngradeLane, blockerKey uint32) bool {
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return false
	}
	if closure.HasCompletionCaveat(lane) {
		// Convergence is monotonic across the whole completion chain. A later
		// form/coverage failure may cause another completion attempt, but it
		// cannot reopen a lane whose typed boundary was already accepted.
		// Queue helpers run before this function, so remove only the repair that
		// belongs to this converged lane and preserve every sibling obligation.
		closure.ClearRepairsByDowngradeLane(lane)
		return true
	}
	allowLaneChurn := lane == types.DowngradeLaneCompletionForm
	exactThreshold := downgradeConvergenceHardThreshold
	laneChurnThreshold := downgradeConvergenceHardThreshold
	if lane == types.DowngradeLaneCompletionForm || lane == types.DowngradeLaneCallChainDiscoverySelection || lane == types.DowngradeLaneFlowOperationCarrier || lane == types.DowngradeLaneFlowParticipantCoverage {
		exactThreshold = 2
	}
	if lane == types.DowngradeLaneCompletionForm {
		if laneChurnThreshold < 3 {
			laneChurnThreshold = 3
		}
	}
	decision := closure.RecordDowngradeProgressDeltaWithLaneChurnThresholds(lane, blockerKey, exactThreshold, laneChurnThreshold, allowLaneChurn)
	if decision.ShouldReplan {
		return false
	}
	reason := "completed under low-delta convergence: the same blocker recurred across repeated attempts with no new grounded progress"
	if lane == types.DowngradeLaneCompletionForm && decision.Delta.HardThreshold == laneChurnThreshold && decision.Delta.HardThreshold > exactThreshold {
		reason = "completed under bounded completion-form convergence: structured landing debt recurred across repeated form-repair attempts without requiring broader exploration"
	}
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       lane,
		ReasonCode: decision.ReasonCode,
		Reason:     reason,
	})
	logging.Info("[emit_investigation_complete] low-delta convergence force-complete lane=%s after %d consecutive no-progress attempts (blocker=%d reason=%s)",
		lane, decision.Delta.Consecutive, blockerKey, decision.ReasonCode)
	return true
}

func callChainDiscoverySelectionRequired(ctx *types.BusContext) bool {
	return ctx != nil && ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.CallChainEndpointProfile.DiscoverSinkActive()
}

func queueCallChainDiscoverySelectionRepair(ctx *types.BusContext) {
	if ctx == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	closure.AddRepair(types.RepairDirective{
		Kind:          types.RepairEmitEvidence,
		Files:         completionMaterializationReadFiles(closure),
		Tools:         []string{"emit_evidence"},
		Subject:       "call_chain_discovery_selection",
		Rationale:     "a discover-sink call chain needs one citable typed registration, assignment/initializer, or factory-return fact connected to the current call path before the runtime destination can be stated as proven",
		Origin:        "emit_investigation_complete.call_chain_discovery_selection",
		DowngradeLane: types.DowngradeLaneCallChainDiscoverySelection,
		Stage:         string(types.StageExplore),
	})
}

func flowOperationEvidenceRequired(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	// A flow-shaped question over an attached runtime artifact is not a
	// current-source operation-flow investigation merely because the analyzer
	// selected AxisFlow/IntentExplain. Keep the source-operation carrier gate
	// only when the shared typed source-lane decision says current checkout
	// evidence is required. This preserves mixed trace+source requests while
	// avoiding a redundant producer/transfer/consumer source pass after typed
	// trace_query evidence has already answered an external-only flow question.
	// The decision reads runtime carriers and structured source obligations; it
	// never parses request or model prose.
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(
		types.RuntimeArtifactContextActiveFromBus(ctx),
	) {
		return false
	}
	return rm.PredicateAxis == types.AxisFlow &&
		rm.Intent != types.IntentTrace &&
		types.ResolveQuestionFamily(rm) != types.QFRootCauseTrace &&
		(rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource())
}

func completionVerifiedReadModeStagePrecedence(ctx *types.BusContext) []stageauthority.PrecedenceRelation {
	if ctx == nil || ctx.AnalysisIR == nil || (ctx.Mode != "" && ctx.Mode != types.ModeRead) {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	if !ok || !stageauthority.MatchesRequiredMainStageParticipantSlate(ctx.AnalysisIR.RequestModel, authority.Main) {
		return nil
	}
	return authority.Precedence
}

func queueFlowOperationCarrierRepair(ctx *types.BusContext, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil {
		return
	}
	files, keywords := flowOperationRepairTargets(ctx, nil, evidence)
	ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:          types.RepairExpandSearch,
		Files:         files,
		Keywords:      keywords,
		Tools:         []string{"repo_map", "grep", "read_file", "emit_evidence"},
		Subject:       "flow_operation_carrier",
		Rationale:     types.FlowOperationEvidenceEmissionGuide,
		Origin:        "emit_investigation_complete.flow_operation_carrier",
		DowngradeLane: types.DowngradeLaneFlowOperationCarrier,
		Stage:         string(types.StageExplore),
	})
}

func flowParticipantCoverageMissing(ctx *types.BusContext, evidence []types.EvidenceItem, stagePrecedence ...stageauthority.PrecedenceRelation) []string {
	if ctx == nil || ctx.AnalysisIR == nil || !flowOperationEvidenceRequired(ctx) {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.DiagramHint == nil || !rm.DiagramHint.Required || len(rm.DiagramHint.Participants) == 0 {
		return nil
	}
	operations := types.FlowOperationEvidenceForRequest(evidence, rm)
	seen := make(map[string]bool)
	missing := make([]string, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		identity := strings.TrimSpace(participant.Identity)
		key := strings.ToLower(identity)
		if identity == "" || seen[key] || participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		seen[key] = true
		covered := stageauthority.ParticipantHasIncidentPrecedence(rm, participant, stagePrecedence)
		for _, operation := range operations {
			for _, surface := range types.DiagramParticipantIdentitySurfaces(rm, participant) {
				if types.AnswerCodeIdentitySurfacesCompatible(surface, operation.Subject) ||
					types.AnswerCodeIdentitySurfacesCompatible(surface, operation.Object) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			missing = append(missing, identity)
		}
	}
	return missing
}

func flowParticipantCoverageRepairHint(missing []string) string {
	return fmt.Sprintf(
		"The required source-flow participant slate still lacks incident operation evidence for %v. %s",
		missing, types.FlowOperationEvidenceEmissionGuide)
}

func queueFlowParticipantCoverageRepair(ctx *types.BusContext, missing []string, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil || len(missing) == 0 {
		return
	}
	files, keywords := flowOperationRepairTargets(ctx, missing, evidence)
	ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:          types.RepairExpandSearch,
		Files:         files,
		Keywords:      keywords,
		Tools:         []string{"repo_map", "grep", "read_file", "emit_evidence"},
		Subject:       "flow_participants:" + strings.Join(missing, ","),
		Rationale:     flowParticipantCoverageRepairHint(missing),
		Origin:        "emit_investigation_complete.flow_participant_coverage",
		DowngradeLane: types.DowngradeLaneFlowParticipantCoverage,
		Stage:         string(types.StageExplore),
	})
}

func preCompleteDowngradeSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	const rejectedPrefix = "emit_investigation_complete rejected:"
	if strings.HasPrefix(summary, rejectedPrefix) {
		return "emit_investigation_complete DOWNGRADED: " + strings.TrimSpace(strings.TrimPrefix(summary, rejectedPrefix))
	}
	if summary == "" {
		return "emit_investigation_complete DOWNGRADED: completion is delayed by a typed pre-complete repair lane."
	}
	return "emit_investigation_complete DOWNGRADED: " + summary
}

func groundingCitationFloorRepair(code, hint string) *types.ToolRepair {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "grounding_citation_floor"
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "Improve typed evidence grounding or continue with a caveat after repeated no-progress attempts."
	}
	return &types.ToolRepair{
		Code: code,
		Hint: hint,
		Metadata: map[string]string{
			"repair_origin": "emit_investigation_complete.grounding_citation_floor",
			"lane":          string(types.DowngradeLaneGroundingCitationFloor),
		},
	}
}

// exactResolvedDefiningProofRepair / queueExactResolvedDefiningProofRepair
// were removed with the §29.60 defining-proof soften (2026-07-13): the arm no
// longer delays closure, so no retry-driving ToolRepair / RepairDirective is
// minted — the typed detection rides the accepted completion as a gate note
// plus a DowngradeLaneExactResolvedDefiningProof caveat instead.

func principalMemberSetHandoffRepair(reasonCode string) *types.ToolRepair {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "principal_member_set_handoff"
	}
	return &types.ToolRepair{
		Code: "principal_member_set_handoff",
		Hint: "Emit aggregate_facts with kind=member_set and exact members for the requested principal answer set, or switch to result_kind=absence with an absence_justification if the set is genuinely empty or not enumerable.",
		Fields: []string{
			"emit_investigation_complete.aggregate_facts[].kind=member_set",
			"emit_investigation_complete.aggregate_facts[].members",
			"emit_investigation_complete.absence_justification",
		},
		Metadata: map[string]string{
			"repair_origin": "emit_investigation_complete.principal_member_set_handoff",
			"lane":          string(types.DowngradeLanePrincipalMemberSetHandoff),
			"reason_code":   reasonCode,
		},
	}
}

func principalMemberSetHandoffDowngradeSummary(reasonCode string) string {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "principal_member_set_handoff"
	}
	return "emit_investigation_complete rejected: this question declares a precise principal member-set obligation (" + reasonCode + "), so a resolved completion must carry the complete verified member list as an aggregate_facts entry with kind=\"member_set\" (exact members; support_refs when required by the member contract). If the set genuinely cannot be enumerated from the investigation, set absence_justification explaining why instead of completing without it."
}

func completionFormRepair(reasonCode, hint string) *types.ToolRepair {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "completion_form"
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "Repair the structured completion handoff, or continue with a typed caveat after repeated no-progress attempts."
	}
	return &types.ToolRepair{
		Code: "completion_form_" + reasonCode,
		Hint: hint,
		Fields: []string{
			"emit_investigation_complete.aggregate_facts",
			"emit_investigation_complete.aggregate_facts[].members",
			"emit_investigation_complete.aggregate_facts[].support_refs",
		},
		Metadata: map[string]string{
			"repair_origin": types.RepairOriginCompletionFormPrefix + reasonCode,
			"lane":          string(types.DowngradeLaneCompletionForm),
			"reason_code":   reasonCode,
		},
	}
}

func queueCompletionFormRepair(ctx *types.BusContext, reasonCode, rationale string) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "completion_form"
	}
	rationale = strings.TrimSpace(rationale)
	if rationale == "" {
		rationale = "completion handoff needs framework-owned form repair"
	}
	closure.AddRepair(types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Subject:       "completion_form:" + reasonCode,
		Tools:         []string{"emit_investigation_complete"},
		Rationale:     rationale,
		Origin:        types.RepairOriginCompletionFormPrefix + reasonCode,
		DowngradeLane: types.DowngradeLaneCompletionForm,
		Stage:         string(types.StageExplore),
	})
}

func queuePrincipalMemberSetHandoffRepair(ctx *types.BusContext, reasonCode string) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		reasonCode = "principal_member_set_handoff"
	}
	closure.AddRepair(types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Subject:       "principal_member_set_handoff:" + reasonCode,
		Tools:         []string{"emit_investigation_complete"},
		Rationale:     "principal answer shape requires aggregate_facts.kind=member_set with exact members, or an explicit absence boundary",
		Origin:        "emit_investigation_complete.principal_member_set_handoff",
		DowngradeLane: types.DowngradeLanePrincipalMemberSetHandoff,
		Stage:         string(types.StageExplore),
	})
}

func queuePathDiscoveryAbsenceRepair(ctx *types.BusContext) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	closure.AddRepair(types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Subject:       "path_discovery_absence_proof",
		Tools:         []string{"list_files", "repo_map"},
		Rationale:     "typed file-family absence needs recursive path discovery with auxiliary/corpus coverage or an authoritative source_inventory lens before declaring a hard zero",
		Origin:        "emit_investigation_complete.path_discovery_absence",
		DowngradeLane: types.DowngradeLanePathDiscoveryAbsence,
		Stage:         string(types.StageExplore),
	})
}

// CurrentInvestigationCompletePolicy returns the active policy
// string. Used by the pre-complete simulator and by tests that need
// to assert / restore the global.
func CurrentInvestigationCompletePolicy() string {
	return investigationCompletePolicy
}

// preCompleteContractCheck is the CGEC E1 simulator. Returns an
// empty string when the LLM may proceed to mark complete, or a
// human-readable downgrade message describing exactly what is
// missing. The caller treats a non-empty return as "do NOT call
// SetInvestigationComplete; surface this message in the tool
// result so the LLM sees what to do next".
//
// Honors the agent_investigation_complete_policy setting: when
// "override", the pre-complete check is skipped because the
// orchestrator's DAG scheduler will mark every explore-type node
// done immediately on the in-flight emit_investigation_complete,
// bypassing every criterion gate. Running pre-complete gates in
// "override" mode would contradict the operator's explicit
// "skip all criteria" policy.
//
// Two predictive checks (cheap, framework-side, no LLM in the loop):
//
//	(a) PendingReads — files queued by chain promotion or a previous
//	    finalizer grounder reject. If any are still outstanding the
//	    next finalize attempt will hit the same citation drop again.
//
//	(b) Citation-floor preflight — when AnswerContract.CitationReq.
//	    Required is true, the evidence buffer must contain at least
//	    one cite-eligible item whose Source is also in ReadSet.
//	    Else citation_count_ge will fail.
//
// Absence-justified investigations (justification non-empty) are
// exempted from check (b): the absence carve-out already waives
// MinCitations, and the LLM has explicitly told the system it cannot
// cite anything.
func preCompleteContractCheck(ctx *types.BusContext, justification string, aggregateFactsOpt ...[]types.AnswerAggregateFact) string {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return preCompleteContractCheckWithEvidence(ctx, justification, evidence, aggregateFactsOpt...)
}

func preCompleteContractCheckWithEvidence(ctx *types.BusContext, justification string, evidence []types.EvidenceItem, aggregateFactsOpt ...[]types.AnswerAggregateFact) string {
	var aggregateFacts []types.AnswerAggregateFact
	if len(aggregateFactsOpt) > 0 {
		aggregateFacts = aggregateFactsOpt[0]
	}
	var structuredRelationAuthorityFacts []types.AnswerAggregateFact
	if len(aggregateFactsOpt) > 1 {
		structuredRelationAuthorityFacts = aggregateFactsOpt[1]
	}
	view := buildCompletionPreflightView(ctx, "", justification, evidence, aggregateFacts, structuredRelationAuthorityFacts)
	return preCompleteContractCheckWithPreflight(ctx, justification, view)
}

func preCompleteContractCheckWithPreflight(ctx *types.BusContext, justification string, view completionPreflightView) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	evidence := view.Evidence
	aggregateFacts := view.EffectiveAggregateFacts
	structuredRelationAuthorityFacts := view.StructuredRelationAuthorityFacts
	// Honor agent_investigation_complete_policy=override. The DAG
	// scheduler will skip all criteria when this policy is set, so
	// running the pre-complete gates would contradict operator
	// intent. soft / strict / unset all leave the gates in force.
	if investigationCompletePolicy == "override" {
		return ""
	}
	// R5 wakeup-chain drilldown (§7.30 裁定3) no longer blocks here: per
	// §29.60 (2026-07-13) the model's completion decision is terminal for
	// quality-class detections, and the arm's subject comes from the
	// whole-window census drilldown plan rather than the question's target
	// thread (witness codrax-20260713-061452: the gate held a correct
	// CompThread D-state answer hostage over an unrelated irq thread's
	// sleep row). The typed detection is preserved verbatim and now rides
	// the ACCEPTED completion as a gate note + typed caveat — see
	// wakeupChainDrilldownPendingAdvisory and its emission site next to the
	// RN-14c note before SetInvestigationComplete.
	closure := ctx.Mutable.EvidenceClosure()
	refreshClosureReadSnapshot(ctx, closure)
	// Drain PendingReads the LLM has already satisfied via its own
	// read_file calls during this dispatch. Without this, an enforcer-
	// enqueued PendingRead (primary_anchor_unread, phase1_unread, chain
	// promotion, grounder reject) re-renders in every subsequent retry
	// hint until the next window boundary, falsely signalling "still
	// unread" and burning retries on a loop the LLM cannot exit.
	if drained := closure.DrainSatisfiedPendingReads(); drained > 0 {
		logging.Debug("[CGEC] drained %d satisfied PendingRead(s) after ReadSet refresh", drained)
	}
	// Belt-and-suspenders next to the pending-read drain: the four read-
	// coverage mutators already self-clear, but this keeps the C2 gate and
	// the downgrade blocker key below evaluating post-drain state within
	// this same dispatch.
	closure.DrainVerifiedUnverifiedFindings()
	if cleared := clearUnavailablePendingReadsAfterFailedRead(ctx, closure); cleared > 0 {
		logging.Info("[CGEC] cleared %d unavailable PendingRead(s) after failed read_file proof", cleared)
	}

	// Session 12 phase1-unread gate. When the LLM calls complete on a
	// breadth-intent question (mechanism / call_chain / conditional)
	// while high-ranked pre-scan files remain unread, push those files
	// into PendingReads so the downstream PendingReads branch surfaces
	// them with a "Forced Read List" downgrade. This closes the gap
	// where R5 ghost-anchor promotion chased red-herring files because
	// the real answer-bearing file never produced a ghost anchor (the
	// LLM simply never read it, so no chain could reference it).
	//
	// requiresCrossFileCoverage further refines: a project-orientation
	// question (intent=explain, complexity=simple, no PrimaryEntities,
	// not cross-component) reads as "tour the repo, not investigate
	// a specific subsystem". The breadth-intent gates skip those
	// because there is no code-level cross-file flow to balance —
	// README + manifest + entry-point already cover the answer space.
	if justification == "" {
		raiseRequiredFileHintPendingReads(ctx, closure, evidence)
		if genericForcedReadBoundarySatisfied(ctx, aggregateFacts, evidence) {
			logging.Info("[emit_investigation_complete] generic forced-read gates bypassed by grounded model-owned completion boundary")
		} else {
			raisePrimaryAnchorPendingRead(ctx, closure)
			raisePhase1UnreadPendingReads(ctx, closure, aggregateFacts)
			applyMultiPathAnchorChecksWithEvidence(ctx, closure, evidence)
		}
		if downgrade := structuredRelationAuthorityPreCompleteDowngrade(ctx, closure, structuredRelationAuthorityFacts, evidence); downgrade != "" {
			return downgrade
		}
	}
	if label, ok := completionGroundingBypassLabel(ctx, aggregateFacts); ok {
		logging.Info("[emit_investigation_complete] multi-topic anchor backbone bypassed by %s", label)
	} else if downgrade := explanationAnchorBackboneDowngrade(ctx); downgrade != "" {
		return downgrade
	}

	pending := closure.PendingReads()

	// CGEC C2: check (e) — evidence Source falls on a file the
	// analyzer named but findings_validator could not verify. The
	// LLM's evidence pool is citing a file the framework has flagged
	// as "analyzer hallucination". Downgrade and push for grep
	// rediscovery so the LLM can disprove or confirm the file
	// actually exists with the expected symbol. Runs BEFORE the
	// PendingReads check so operator sees the root-cause signal
	// first (unverified findings are stronger evidence of bad
	// grounding than "LLM didn't read file X").
	if unverified := closure.UnverifiedFindings(); len(unverified) > 0 {
		unverifiedPaths := make(map[string]string)
		nonAdvisoryUnverified := 0
		for _, u := range unverified {
			if u.Advisory {
				// Aged out of blocking (advisory demotion) — renders as
				// soft prompt guidance only.
				continue
			}
			nonAdvisoryUnverified++
			if u.Kind == "path" {
				unverifiedPaths[u.Token] = u.Reason
			}
		}
		if len(unverifiedPaths) > 0 {
			var hits []string
			var matchedTokens []string
			seenTokens := make(map[string]bool)
			for _, ev := range evidence {
				if reason, bad := unverifiedPaths[ev.Source]; bad {
					hits = append(hits, fmt.Sprintf("%s (%s)", ev.Source, reason))
					if !seenTokens[ev.Source] {
						seenTokens[ev.Source] = true
						matchedTokens = append(matchedTokens, ev.Source)
					}
				}
			}
			if len(hits) > 0 {
				// Emit RepairExpandSearch so the retry hint surfaces
				// broaden-keyword guidance. The Rationale mentions
				// the unverified files explicitly so the LLM knows
				// which claims to disprove.
				var kws []string
				if ctx.AnalysisIR != nil {
					kws = append(kws, ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords...)
				}
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairExpandSearch,
					Keywords:  kws,
					Rationale: fmt.Sprintf("evidence cites %d file(s) the prior findings check flagged as unverified: %s — re-grep the repo to confirm the correct locations", len(hits), strings.Join(hits, "; ")),
					Origin:    "pre_complete.evidence_on_unverified_path",
				})
				logging.Info("[CGEC] C2 downgrade: evidence cites unverified path(s) count=%d", len(hits))
				// No-progress denial breaker (mirror of the citation-floor
				// breaker below). The fingerprint is built ONLY from
				// deterministic counters this handler does not mutate —
				// hits shrinks only when the model re-anchors evidence, the
				// unverified count shrinks only via verified-read clears /
				// advisory demotion, and the read set grows only on real
				// reads. The handler's own AddRepair above is deduped, so
				// it cannot churn the converger or this fingerprint.
				if ctx.Mutable != nil {
					fingerprint := fmt.Sprintf("unverified_path_evidence hits=%d uf=%d reads=%d", len(hits), nonAdvisoryUnverified, len(closure.ReadSet()))
					if streak := ctx.Mutable.RecordCompletionDenialStreak(fingerprint); streak > completionDenialBreakerMaxStreak {
						// Typed escape lane: demote the matched path
						// findings to Advisory so subsequent attempts and
						// the exact-resolution consumers stop blocking on
						// them, then let completion continue in degraded
						// form.
						demoted := closure.DemoteUnverifiedFindingsToAdvisory("path", matchedTokens...)
						logging.Warning("[emit_investigation_complete] C2 unverified-path denial breaker tripped after %d identical no-progress denials (%s); demoted %d finding(s) to advisory and accepting completion in degraded form",
							streak-1, fingerprint, demoted)
						ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
							"the evidence cites %d path(s) the findings check could not verify; completion proceeded in degraded form after %d identical no-progress attempts — present those anchors as unconfirmed, not as verified current-source citations",
							len(hits), streak-1))
						return ""
					}
				}
				var b strings.Builder
				b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — evidence cites files the prior findings check flagged as unverified.\n\n")
				b.WriteString("The following evidence sources were unable to be verified against the repo graph:\n")
				for _, h := range hits {
					b.WriteString("  - " + h + "\n")
				}
				b.WriteString("\nRe-run grep to confirm the correct file paths (the prior pass may have hallucinated them). After finding real evidence anchors, re-call emit_investigation_complete.")
				return b.String()
			}
		}
	}

	// Check (a): forced reads still outstanding.
	// CGEC D3: partition PendingRead entries by ScannedSet
	// membership. Files the explorer's pre-scan saw (or grep'd /
	// read during this run — IsScanned is lenient when ScannedSet
	// is empty) render as "Forced Read List" directing the LLM to
	// read them. Files NOT in ScannedSet render as "Suspicious
	// Anchors" — likely ghost paths the LLM should either verify
	// with grep OR ignore entirely. The two-section layout makes
	// the LLM's action different per bucket: READ the scanned
	// ones, INVESTIGATE (or reject) the unscanned ones.
	//
	// 2026-05-10: skip the entire forced-read block when the model
	// has declared evidence_floor_waiver OR the system has detected
	// an external-source log. In those scenarios, demanding repo
	// reads only forces the LLM to read unrelated files and risk
	// confabulation (the customer-reported logtri_custom failure).
	if label, ok := completionGroundingBypassLabel(ctx, aggregateFacts); ok && len(pending) > 0 {
		logging.Info("[emit_investigation_complete] forced-read pending list bypassed by %s (pending=%d)",
			label, len(pending))
		pending = nil
	}
	if len(pending) > 0 {
		blocking, advisory := partitionPendingReadsForAcceptedClosure(ctx, pending, aggregateFacts, evidence)
		if len(advisory) > 0 {
			logging.Info("[emit_investigation_complete] %d generic/advisory pending read(s) no longer block model-owned completion boundary", len(advisory))
			if cleared := closure.ClearPendingReadsMatching(advisory...); cleared > 0 {
				logging.Info("[emit_investigation_complete] cleared %d advisory pending read(s) after accepted completion boundary", cleared)
			}
			// §29.42.4 detect→disclose: the demoted coverage suggestions
			// ride the accepted completion as a bounded note instead of
			// silently vanishing — the answer should not present claims
			// about unread files as read-verified. The typed caveat lane
			// (件2, 2026-07-13) carries the same fact to the user-facing
			// answer disclosure (completionCaveatLaneSystemCaveats).
			ctx.Mutable.AppendCompletionGateNote(advisoryPendingReadCoverageNote(advisory))
			closure.AppendCompletionCaveat(types.CompletionCaveat{
				Lane:       types.DowngradeLaneForcedReadCoverage,
				ReasonCode: "coverage_reads_demoted",
				Reason:     "coverage-class read suggestions were demoted to advisory at the accepted completion",
			})
		}
		pending = blocking
	}
	if len(pending) > 0 {
		var scanned, suspicious []types.PendingRead
		for _, p := range pending {
			if closure.IsScanned(p.File) {
				scanned = append(scanned, p)
			} else {
				suspicious = append(suspicious, p)
			}
		}
		var b strings.Builder
		b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — pending forced reads block the closure.\n\n")
		max := 6
		if len(scanned) > 0 {
			b.WriteString("## Forced Read List (scanned files the LLM has not read yet)\n")
			for i, p := range scanned {
				if i >= max {
					fmt.Fprintf(&b, "  ... and %d more\n", len(scanned)-max)
					break
				}
				// A runtime log/trace in the pending list is read to VERIFY
				// the referenced rows, not to mine citations: emit_evidence
				// will (correctly) classify its rows as external
				// observations, so telling the model to read it "for
				// evidence" recreates the read→emit→skip loop. Say which
				// lane the verified rows land in instead.
				if types.RuntimeArtifactPathKind(p.File) != "" {
					fmt.Fprintf(&b, "  - %s — %s (runtime log/trace: read to verify the referenced rows; verified rows are preserved through emit_investigation_complete reason/aggregate_facts, not emit_evidence)\n", p.File, p.Rationale)
					continue
				}
				fmt.Fprintf(&b, "  - %s — %s\n", p.File, p.Rationale)
			}
			b.WriteString("\n")
		}
		if len(suspicious) > 0 {
			b.WriteString("## Suspicious Anchors (files NOT in the investigation's scanned set — possibly hallucinated paths)\n")
			for i, p := range suspicious {
				if i >= max {
					fmt.Fprintf(&b, "  ... and %d more\n", len(suspicious)-max)
					break
				}
				fmt.Fprintf(&b, "  - %s — %s\n", p.File, p.Rationale)
			}
			b.WriteString("Either grep for the real path if this is a legitimate reference, or reject any chain / citation that depends on it.\n\n")
		}
		b.WriteString("Read the scanned files (if any) and/or verify the suspicious anchors, then re-call emit_investigation_complete. Marking complete now will drop every chain anchored in them.")
		return b.String()
	}
	// Endpoint reachability and interior-span completeness are independent
	// contracts. A principal member_set can close the latter, but a roster has
	// no edge direction and must never bypass the former. Keep this check ahead
	// of the member_set shortcut so RunWith/Run siblings, reverse wrappers, and
	// unordered node lists cannot mint source -> sink authority.
	if justification == "" {
		if downgrade := callChainExactEndpointReachabilityDowngradeWithEvidence(ctx, closure, evidence); downgrade != "" {
			return downgrade
		}
		if downgrade := callChainReadParserRelationHandoffDowngrade(ctx, closure, aggregateFacts, evidence); downgrade != "" {
			return downgrade
		}
	}
	if callChainAggregateMemberSetCompletesPrincipalBoundary(ctx, aggregateFacts, evidence) {
		logging.Info("[emit_investigation_complete] call-chain closure gates satisfied by principal aggregate member_set")
	} else {
		if downgrade := callChainPrincipalSpanDowngradeWithEvidence(ctx, closure, evidence); downgrade != "" {
			return downgrade
		}
		if downgrade := callChainQualifiedIntermediateDowngradeWithEvidence(ctx, closure, evidence); downgrade != "" {
			return downgrade
		}
	}
	if justification == "" {
		if downgrade := fieldValueCountCoverageDowngradeWithEvidence(ctx, closure, evidence); downgrade != "" {
			return downgrade
		}
		if downgrade := historyCountAggregateHandoffDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := deterministicCountProofDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := principalRequiredTermHandoffDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := relationMemberSetHandoffDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := exhaustiveEnumerationMemberSetDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := changeImpactTargetStructuredHandoffDowngradeWithEvidence(ctx, closure, evidence); downgrade != "" {
			return downgrade
		}
		if downgrade := changeImpactPrincipalMemberSetDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
		if downgrade := principalSupportMaterializationDowngrade(ctx, closure, aggregateFacts); downgrade != "" {
			return downgrade
		}
	}

	// Check (b): citation-floor preflight. Requires AnalysisIR.
	if justification != "" {
		// Absence answer waives the floor by contract; bypass.
		return ""
	}
	// The explicit current-source-exclusion bypass depends only on the typed
	// request model (ExcludesCurrentSource), not on any mutable observation
	// state, so it must be honored even when Mutable is absent — a user who
	// said "don't analyze code" can never satisfy a current-source floor.
	if label, ok := explicitCurrentSourceExclusionCompletionBypassLabel(ctx); ok {
		logging.Info("[emit_investigation_complete] citation-floor bypassed by %s", label)
		return ""
	}
	// The zero-current-source-repo bypass likewise depends only on the
	// deterministic run-entry census, not on mutable observation state.
	if label, ok := zeroCurrentSourceRepoCompletionBypassLabel(ctx); ok {
		logging.Info("[emit_investigation_complete] citation-floor bypassed by %s", label)
		return ""
	}
	if ctx.Mutable != nil {
		if label, ok := completionGroundingBypassLabel(ctx, aggregateFacts); ok {
			// External-source logs/traces and model-declared waivers are
			// answered from structured origin-specific semantics, not from
			// repo file:line anchors. Builder/front-end already teach
			// downstream stages to use external-observation provenance
			// where appropriate; forcing a repo citation floor here only
			// creates pointless read-more loops against unrelated files.
			logging.Info("[emit_investigation_complete] citation-floor bypassed by %s", label)
			return ""
		}
	}
	ir := ctx.AnalysisIR
	if ir == nil {
		return ""
	}
	if !ir.AnswerContract.CitationReq.Required {
		return ""
	}
	min := ir.AnswerContract.CitationReq.MinCitations
	if min <= 0 {
		min = 1
	}
	if effective, ok := types.EffectiveCitationFloorForPrincipalMemberSets(min, aggregateFacts, &ir.RequestModel); ok {
		logging.Info("[emit_investigation_complete] citation-floor capped by principal member_set: base=%d effective=%d", min, effective)
		min = effective
	}
	if sourceInventoryMechanicalRowSetSatisfiesCitationFloor(ctx, aggregateFacts) {
		logging.Info("[emit_investigation_complete] citation-floor satisfied by exact source-inventory mechanical row-set")
		return ""
	}
	if completionFactsContainPrincipalEmptyMemberSetWithTypedNoHitSupport(ctx, aggregateFacts) {
		logging.Info("[emit_investigation_complete] citation-floor satisfied by exact empty member_set with typed no-hit support")
		return ""
	}
	readSet := closure.ReadSet()
	eligible := 0
	for _, e := range evidence {
		if e.Source == "" || e.LineStart <= 0 {
			continue
		}
		// Line-level check (post-2026-05-01 multi-path surgical-read
		// rewrite): use HasReadLine instead of HasRead so the gate
		// stays aligned with the finalizer-side grounder. Pre-fix:
		// HasRead returned true on any partial read, so a citation
		// pointing at a line OUTSIDE the surgical-read range was
		// counted as eligible by the gate but rejected by the
		// grounder downstream — the answer would ship with fewer
		// real citations than the gate believed it had cleared,
		// silently degrading shape compliance. HasReadLine has a
		// load-bearing backward-compat clause: when the file is in
		// readSet but no range records exist (legacy emitters that
		// never populated readRanges), it returns true so this
		// tightening does NOT regress callers that don't track
		// ranges.
		if len(readSet) > 0 && !closure.HasReadLine(e.Source, e.LineStart) {
			continue
		}
		eligible++
	}
	if eligible >= min {
		// CGEC E3: check (f) — when the AnswerSubject has
		// reasonable confidence and the explorer's chain ranker
		// recorded SubjectMatch scores for chain terminals, assert
		// that at least ONE chain scored above the rebind floor
		// (0.4). If every chain scores below, the investigation is
		// producing evidence about the WRONG kind of token
		// (e.g. AgentName chains when question asks for SkillName).
		// Emit RepairRebindSubject so the retry prompt tells the
		// explorer to constrain its chain production to the right
		// subject kind. Confidence gate mirrors rankChainsBySubject's
		// existing G5 trigger (conf >= 0.5 + best < 0.4). Runs
		// BEFORE the subject/shape mismatch check — rebind is the
		// more fundamental fix (shape matters only if subject is
		// right).
		const (
			rebindFloor   = 0.4
			highConfFloor = 0.5
		)
		subj := ir.RequestModel.AnswerSubject
		matches := closure.AllSubjectMatches()
		if len(matches) > 0 && subj.Confidence >= highConfFloor && subj.Kind != types.SubjectUnknown {
			var bestScore float64
			for _, v := range matches {
				if v > bestScore {
					bestScore = v
				}
			}
			if bestScore < rebindFloor {
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairRebindSubject,
					Subject:   string(subj.Kind),
					Rationale: fmt.Sprintf("pre-complete: %d chain(s) scored against expected subject %s; none above %.1f (best=%.2f)", len(matches), subj.Kind, rebindFloor, bestScore),
					Origin:    "pre_complete.subject_match_low",
				})
				logging.Info("[CGEC] E3 rebind_subject: origin=pre_complete.subject_match_low kind=%s best=%.2f floor=%.2f", subj.Kind, bestScore, rebindFloor)
			}
		}
		// CGEC B2b: eligible evidence is sufficient in quantity, but
		// check for a static mismatch between AnswerSubject (what
		// kind of literal the answer should be) and the compiled
		// AnswerSemanticView's family classification. When the
		// subject points at a code symbol (function / type /
		// interface / handler route / return value) but the family
		// resolved to QFConfigPrecedence (config-key scalar
		// contract), the family routing under-fits the user's
		// subject. Emit a repair so the retry hint surfaces the
		// conflict; does NOT downgrade the current emit because
		// evidence IS present.
		if mismatch, fromFamily, toFamily := detectSubjectViewMismatch(ir); mismatch {
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairSwapView,
				Subject:   fmt.Sprintf("from=%s,to=%s", fromFamily, toFamily),
				Rationale: fmt.Sprintf("answer subject=%s (source-code literal) but family=%s — the rendered answer should be %s instead", ir.RequestModel.AnswerSubject.Kind, fromFamily, toFamily),
				Origin:    "pre_complete.subject_view_mismatch",
			})
			logging.Info("[CGEC] B2b view_swap: origin=pre_complete.subject_view_mismatch from=%s to=%s", fromFamily, toFamily)
			// View mismatch detected at pre-complete boundary.
			// High-confidence (0.85) signal that the IR's family
			// classification disagrees with the subject.kind.
			closure.AppendViolation(types.Violation{
				Kind:       types.ViolViewSwap,
				Detail:     fmt.Sprintf("pre-complete B2b: AnswerSubject=%s vs family=%s (→ %s)", ir.RequestModel.AnswerSubject.Kind, fromFamily, toFamily),
				ClusterKey: types.ComposeClusterKey("subject", string(ir.RequestModel.AnswerSubject.Kind), "from", string(fromFamily), "to", string(toFamily), "root", "answer_subject"),
				Stage:      string(types.StageExplore),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_subject",
					Reason:     fmt.Sprintf("subject.kind=%s incompatible with family=%s at pre-complete", ir.RequestModel.AnswerSubject.Kind, fromFamily),
					Confidence: 0.85,
				},
			})
		}
		return ""
	}
	// CGEC B1c: evidence short of MinCitations. Historically this
	// always rendered an expand-search repair from analyzer keywords,
	// which is still a useful fallback when no structural target exists.
	// Prefer precise structured targets first: when the model already
	// handed off aggregate_facts.support_refs or repo-lens candidate
	// files, reading those files is lower-noise than widening grep.
	if eligible+1 < min {
		if repair, ok := citationFloorStructuredReadRepair(ctx, aggregateFacts, min-eligible); ok {
			closure.AddRepair(repair)
			logging.Info("[CGEC] B1c read_support_refs: origin=%s eligible=%d min=%d files=%d", repair.Origin, eligible, min, len(repair.Files))
		} else {
			var kws []string
			if ir.RequestModel.AnalyzerHints.Keywords != nil {
				kws = append(kws, ir.RequestModel.AnalyzerHints.Keywords...)
			}
			if len(kws) > 0 {
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairExpandSearch,
					Keywords:  kws,
					Rationale: fmt.Sprintf("evidence buffer has only %d of %d required cite-eligible items and no structured file:line repair targets — broaden grep coverage with stems / conceptual synonyms of the keywords above", eligible, min),
					Origin:    "pre_complete.citation_floor_low",
				})
				logging.Info("[CGEC] B1c expand_search: origin=pre_complete.citation_floor_low eligible=%d min=%d keywords=%d", eligible, min, len(kws))
			}
		}
	}
	// No-progress denial breaker: if this exact denial — same floor, same
	// cite-eligible count, same read-set size — has already been returned
	// completionDenialBreakerMaxStreak times consecutively, denying again
	// cannot extract new work from the model; it only reopens the
	// investigation into the same wall (trace_repl.log 2026-07-02: 37
	// identical denials, 46 minutes, no answer). Accept with an explicit
	// degraded-citation caveat instead. The fingerprint is built from
	// deterministic counters only, so any real progress between attempts
	// (a new file read, a new eligible citation, a changed floor) resets
	// the streak and the gate stays hard.
	if ctx.Mutable != nil {
		fingerprint := fmt.Sprintf("citation_floor min=%d eligible=%d reads=%d", min, eligible, len(readSet))
		if streak := ctx.Mutable.RecordCompletionDenialStreak(fingerprint); streak > completionDenialBreakerMaxStreak {
			logging.Warning("[emit_investigation_complete] citation-floor denial breaker tripped after %d identical no-progress denials (%s); accepting completion with degraded-citation caveat",
				streak-1, fingerprint)
			ctx.Mutable.AppendCompletionGateNote(fmt.Sprintf(
				"citation floor (≥%d) accepted in degraded form after %d identical no-progress attempts; the answer must present its evidence as runtime/derived observations, not current-source citations",
				min, streak-1))
			return ""
		}
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — pre-complete citation preflight failed.\n\n")
	fmt.Fprintf(&b, "The answer contract requires ≥%d citation(s) but the current evidence buffer has only %d cite-eligible item(s) (Source non-empty AND in the read-files list).\n",
		min, eligible)
	b.WriteString("Continue the investigation: emit more file:line evidence anchored in files you actually read, or read additional files first.")
	return b.String()
}

// completionDenialBreakerMaxStreak is how many CONSECUTIVE identical
// no-progress citation-floor denials are allowed before the gate accepts a
// degraded completion. Three attempts give the retry directives a fair chance
// to produce progress; anything the model was going to fix, it fixes well
// within three attempts once state actually changes between them.
const completionDenialBreakerMaxStreak = 3

func effectiveCompletionAggregateFacts(ctx *types.BusContext, current []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	current = cloneCompletionAggregateFacts(current)
	if ctx == nil || ctx.Mutable == nil {
		return current
	}
	if len(current) > 0 {
		if stable := completionMonotonicStableAggregateFacts(ctx, current); len(stable) > 0 {
			// XGAP-FIX ① (§29.104.8): the monotonic carry-forward exists so a
			// later emit that DROPS a fact keeps the earlier handoff, but for
			// ordinal-seated ranking facts the later-accepted emit owns the
			// publication surface — carrying the earlier same-label version
			// forward mints a self-contradictory verbatim obligation set
			// (witness 20260715-202022.323-89609: two 「根因排序」 facts, seat
			// #1 published with two values). Supersede is deterministic
			// bookkeeping, not a judgement on model content.
			stable = types.SupersedeOrdinalMemberSetFactsByLabel(stable, current)
			if len(stable) == 0 {
				return current
			}
			merged := append(cloneCompletionAggregateFacts(stable), current...)
			return types.MergeAnswerAggregateFacts(merged)
		}
		return current
	}
	stable := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(stable) == 0 {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			stable = ta.AcceptedAggregateFacts
		}
	}
	return cloneCompletionAggregateFacts(stable)
}

func completionMonotonicStableAggregateFacts(ctx *types.BusContext, current []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if ctx == nil || ctx.Mutable == nil || len(current) == 0 {
		return nil
	}
	stable := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(stable) == 0 {
		return nil
	}
	var out []types.AnswerAggregateFact
	for _, stableFact := range stable {
		for _, currentFact := range current {
			if completionAggregateFactsMonotonicCompatible(stableFact, currentFact) {
				out = append(out, stableFact)
				break
			}
		}
	}
	return out
}

func completionAggregateFactsMonotonicCompatible(stable, current types.AnswerAggregateFact) bool {
	if stable.Kind != types.AnswerAggregateMemberSet || current.Kind != types.AnswerAggregateMemberSet {
		return false
	}
	stableLabel := strings.TrimSpace(stable.Label)
	currentLabel := strings.TrimSpace(current.Label)
	if stableLabel == "" || currentLabel == "" || !strings.EqualFold(stableLabel, currentLabel) {
		return false
	}
	stableRole := types.AnswerAggregateFactRoleForRequest(stable, nil)
	currentRole := types.AnswerAggregateFactRoleForRequest(current, nil)
	if stableRole != currentRole {
		return false
	}
	return true
}

func effectiveCompletionAggregateFactsForValidation(ctx *types.BusContext, current []types.AnswerAggregateFact, evidence []types.EvidenceItem) []types.AnswerAggregateFact {
	effective := effectiveCompletionAggregateFacts(ctx, current)
	effective = enrichCompletionAggregateFactsWithMemberSupportWithEvidence(ctx, effective, evidence)
	effective = reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, effective, evidence)
	effective = reconcileCompletionAggregateFactsWithSourceInventory(ctx, effective, evidence)
	effective = enrichCompletionAggregateFactsWithDeterministicCount(ctx, effective)
	effective = enrichCompletionAggregateFactsWithDeterministicHistoryCount(ctx, effective)
	if ctx != nil && ctx.AnalysisIR != nil {
		effective = types.NormalizeAggregateFactRolesForRequest(effective, &ctx.AnalysisIR.RequestModel)
	}
	effective = sourceInventoryPrincipalRowSetLandingFacts(ctx, effective)
	effective = normalizeAggregateFactsForTypedExclusion(ctx, effective)
	return effective
}

func normalizeSourceInventoryAbsenceWithPrincipalAggregates(ctx *types.BusContext, resultKind, justification string, facts []types.AnswerAggregateFact) (string, string, string) {
	if !strings.EqualFold(strings.TrimSpace(resultKind), "absence") ||
		strings.TrimSpace(justification) == "" ||
		ctx == nil ||
		ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile == nil ||
		!ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active() ||
		!types.SourceInventoryRequiresRepoWideLens(ctx.AnalysisIR.RequestModel) {
		return resultKind, justification, ""
	}
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
	for _, ref := range refs {
		if len(ref.Fact.Members) == 0 {
			continue
		}
		return "resolved", "", "result_kind normalized: source-inventory absence converted to resolved because typed principal aggregate_facts contain non-empty repo-wide members"
	}
	return resultKind, justification, ""
}

func sourceInventoryPrincipalRowSetLandingFacts(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	facts = cloneCompletionAggregateFacts(facts)
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return facts
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	snapshot := sourceInventoryAuthoritySnapshotForCompletion(ctx, observation, facts)
	if len(snapshot.ProjectedPrincipalAggregates) == 0 {
		return facts
	}
	return snapshot.ProjectedPrincipalAggregates
}

func cloneCompletionAggregateFacts(in []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.AnswerAggregateFact, len(in))
	for i, fact := range in {
		out[i] = fact
		if fact.Dimensions != nil {
			out[i].Dimensions = append([]types.AnswerAggregateDimension(nil), fact.Dimensions...)
		}
		if fact.Members != nil {
			out[i].Members = append([]string(nil), fact.Members...)
		}
		if fact.MemberNotes != nil {
			out[i].MemberNotes = append([]string(nil), fact.MemberNotes...)
		}
		if fact.Excluded != nil {
			out[i].Excluded = append([]string(nil), fact.Excluded...)
		}
		if fact.SupportRefs != nil {
			out[i].SupportRefs = append([]string(nil), fact.SupportRefs...)
		}
	}
	return out
}

func mergeCompletionAggregateFacts(current, stable []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	out := cloneCompletionAggregateFacts(current)
	seen := make(map[string]int, len(out))
	for idx, fact := range out {
		if key := completionAggregateFactIdentity(fact); key != "" {
			seen[key] = idx
		}
	}
	for _, fact := range stable {
		if completionAggregateMemberSetSupersetMerged(&out, fact) {
			continue
		}
		key := completionAggregateFactIdentity(fact)
		if key != "" {
			if idx, ok := seen[key]; ok {
				if idx >= 0 && idx < len(out) && completionAggregateFactPreferred(fact, out[idx]) {
					cloned := cloneCompletionAggregateFacts([]types.AnswerAggregateFact{fact})
					if len(cloned) > 0 {
						out[idx] = cloned[0]
					}
				}
				continue
			}
		}
		cloned := cloneCompletionAggregateFacts([]types.AnswerAggregateFact{fact})
		if len(cloned) == 0 {
			continue
		}
		out = append(out, cloned[0])
		if key != "" {
			seen[key] = len(out) - 1
		}
	}
	return out
}

func completionAggregateFactIdentity(fact types.AnswerAggregateFact) string {
	return types.AnswerAggregateFactIdentity(fact)
}

func completionAggregateMemberSetSupersetMerged(out *[]types.AnswerAggregateFact, stable types.AnswerAggregateFact) bool {
	if out == nil || !completionAggregateFactCarriesMemberSet(stable) {
		return false
	}
	stableKey := completionAggregateMemberSetMergeKey(stable)
	if stableKey == "" {
		return false
	}
	for i := range *out {
		current := (*out)[i]
		if !completionAggregateFactCarriesMemberSet(current) ||
			completionAggregateMemberSetMergeKey(current) != stableKey {
			continue
		}
		switch {
		case completionAggregateMembersCoverFacts(current, stable):
			if completionAggregateFactPreferred(stable, current) {
				cloned := cloneCompletionAggregateFacts([]types.AnswerAggregateFact{stable})
				if len(cloned) > 0 {
					(*out)[i] = cloned[0]
				}
			}
			return true
		case completionAggregateMembersCoverFacts(stable, current):
			cloned := cloneCompletionAggregateFacts([]types.AnswerAggregateFact{stable})
			if len(cloned) > 0 {
				(*out)[i] = cloned[0]
			}
			return true
		default:
			return false
		}
	}
	return false
}

func completionAggregateFactPreferred(a, b types.AnswerAggregateFact) bool {
	ar := completionAggregateFactRolePriority(a)
	br := completionAggregateFactRolePriority(b)
	if ar != br {
		return ar > br
	}
	if len(a.SupportRefs) != len(b.SupportRefs) {
		return len(a.SupportRefs) > len(b.SupportRefs)
	}
	return len(a.Members) > len(b.Members)
}

func completionAggregateFactRolePriority(fact types.AnswerAggregateFact) int {
	return types.AnswerAggregateRolePriority(types.AnswerAggregateFactRoleForRequest(fact, nil))
}

func completionAggregateFactCarriesMemberSet(fact types.AnswerAggregateFact) bool {
	return fact.Kind == types.AnswerAggregateMemberSet && len(fact.Members) > 0
}

func completionAggregateMemberSetMergeKey(fact types.AnswerAggregateFact) string {
	label := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(fact.Label)), " "))
	if label == "" {
		return ""
	}
	var dims []string
	for _, dim := range fact.Dimensions {
		name := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(dim.Name)), " "))
		value := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(dim.Value)), " "))
		if name == "" && value == "" {
			continue
		}
		dims = append(dims, name+"="+value)
	}
	sort.Strings(dims)
	unit := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(fact.Unit)), " "))
	return string(fact.Kind) + "\x00" + label + "\x00" + unit + "\x00" + strings.Join(dims, "\x00")
}

func completionAggregateMembersCover(have []string, want []string) bool {
	return completionAggregateMembersCoverFacts(
		types.AnswerAggregateFact{Members: have},
		types.AnswerAggregateFact{Members: want},
	)
}

func completionAggregateMembersCoverFacts(haveFact, wantFact types.AnswerAggregateFact) bool {
	have := haveFact.Members
	want := wantFact.Members
	if len(have) == 0 || len(want) == 0 || len(have) < len(want) {
		return false
	}
	haveSet := make(map[string]bool, len(have)*2)
	for idx, member := range have {
		for _, key := range completionAggregateMemberKeysForFact(haveFact, idx, member) {
			haveSet[key] = true
		}
	}
	for idx, member := range want {
		matched := false
		for _, key := range completionAggregateMemberKeysForFact(wantFact, idx, member) {
			if haveSet[key] {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func completionAggregateMemberKeys(member string) []string {
	return completionAggregateMemberKeysForFact(types.AnswerAggregateFact{Members: []string{member}}, 0, member)
}

func completionAggregateMemberKeysForFact(fact types.AnswerAggregateFact, memberIdx int, member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(candidate string) {
		candidate = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(candidate)), " "))
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	add(member)
	for _, candidate := range types.AnswerAggregateMemberDisplayCandidates(member) {
		add(candidate)
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		add(label)
		for _, candidate := range types.AnswerAggregateMemberDisplayCandidates(label) {
			add(candidate)
		}
	}
	if loc := completionAggregateMemberLocationKey(fact, memberIdx, member); loc != "" {
		for _, key := range append([]string(nil), out...) {
			add(key + "\x00loc:" + loc)
		}
	}
	return out
}

func completionAggregateMemberLocationKey(fact types.AnswerAggregateFact, memberIdx int, member string) string {
	if surface, ok := types.ParseAnswerSourceLocationSurface(member); ok {
		return normalizeAnswerSupportLocationForCompletion(aggregateMemberStartLocationForCompletion(surface))
	}
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(member); ok &&
		strings.TrimSpace(loc.File) != "" && loc.LineStart > 0 {
		return normalizeAnswerSupportLocationForCompletion(aggregateMemberStartLocationForCompletion(loc))
	}
	if memberIdx < 0 || memberIdx >= len(fact.SupportRefs) {
		return ""
	}
	ref := strings.TrimSpace(fact.SupportRefs[memberIdx])
	if ref == "" {
		return ""
	}
	_, loc, ok := types.ParseAnswerSupportRefMemberLocation(ref)
	if !ok || strings.TrimSpace(loc.File) == "" || loc.LineStart <= 0 {
		return ""
	}
	return normalizeAnswerSupportLocationForCompletion(aggregateMemberStartLocationForCompletion(loc))
}

func aggregateMemberStartLocationForCompletion(loc types.AnswerSourceLocationSurface) string {
	file := strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`))
	if file == "" || loc.LineStart <= 0 {
		return ""
	}
	return file + ":" + strconv.Itoa(loc.LineStart)
}

func normalizeAnswerSupportLocationForCompletion(location string) string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return ""
	}
	return strings.ToLower(location)
}

func repoGroundingBypassLabel(ctx *types.BusContext) (string, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return "", false
	}
	if w := ctx.Mutable.EvidenceFloorWaiver(); w.IsActive() {
		if !runtimeArtifactGroundingBypassAllowed(ctx) {
			return "", false
		}
		return fmt.Sprintf("evidence_floor_waiver=%s", w.Reason), true
	}
	if bundle := ctx.Mutable.LogTriage(); bundle != nil && bundle.IsExternalSource() {
		if !runtimeArtifactGroundingBypassAllowed(ctx) {
			return "", false
		}
		return "system-detected external-source log", true
	}
	if perf := ctx.Mutable.PerfTrace(); perf != nil && perf.IsExternalSource() {
		if !runtimeArtifactGroundingBypassAllowed(ctx) {
			return "", false
		}
		return "system-detected external-source trace", true
	}
	return "", false
}

func completionGroundingBypassLabel(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) (string, bool) {
	if label, ok := explicitCurrentSourceExclusionCompletionBypassLabel(ctx); ok {
		return label, true
	}
	if label, ok := zeroCurrentSourceRepoCompletionBypassLabel(ctx); ok {
		return label, true
	}
	if label, ok := repoGroundingBypassLabel(ctx); ok {
		return label, true
	}
	if label, ok := traceQueryRuntimeObservationCompletionBypassLabel(ctx, aggregateFacts); ok {
		return label, true
	}
	if label, ok := originSpecificCompletionBypassLabel(ctx, aggregateFacts); ok {
		return label, true
	}
	return "", false
}

// explicitCurrentSourceExclusionCompletionBypassLabel waives the completion
// citation floor when the analyzer has explicitly excluded current source from
// the answer. ExcludesCurrentSource() is the strongest precise typed boundary
// the request model carries: current_source_mode=exclude AND
// exclusion_kind=explicit_user_boundary AND at least one verbatim SourceQuote
// (see ExternalObservationPolicy.ExcludesCurrentSource). Any exclude that lacks
// that anchored provenance — or lacks a runtime-artifact carrier — is already
// softened to allow upstream by promoteInvalidExternalObservationExcludeToAllow
// / normalizeExternalObservationPolicyForCurrentSourceExplanation, so reaching
// this point means the user genuinely instructed "don't analyze code".
//
// For such a request there is, by the user's own instruction, no current source
// to cite. The runtime evidence a trace-only investigation gathers may arrive
// via trace_query (already bypassed) OR via raw read_file/grep on the attached
// runtime artifact (NOT counted as trace_query runtime observations and NOT
// emitted as typed aggregate facts), in which case none of the other bypasses
// fire and the current-source citation floor becomes unsatisfiable. That forces
// emit_investigation_complete to be rejected every turn, reopening the
// investigation and driving the model back into reading source it was told to
// leave alone — a livelock. Keying the bypass on the precise exclusion signal
// mirrors readLocalizerTier1CurrentSourceRequired, which already treats
// ExcludesCurrentSource() as "current source not required".
func explicitCurrentSourceExclusionCompletionBypassLabel(ctx *types.BusContext) (string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return "", false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return "explicit_current_source_exclusion", true
	}
	return "", false
}

// wakeupChainDrilldownPendingAdvisory returns a one-shot ADVISORY note when
// the run's own trace_query drilldown plan marked the dominant sleep state as
// chain_required=true but no wakeup-chain-family observation was ever
// produced (R5 / §7.30 裁定3). Both inputs are deterministic typed facts from
// trace_query observations; the one-shot marker guarantees the note fires at
// most once per trace artifact, so it cannot spam.
//
// §29.60 soften (2026-07-13): this arm used to return a BLOCKING downgrade
// from the pre-complete contract chain. Two witnessed defects drove the
// demotion to an accepted-completion gate note + typed caveat:
//   - subject/intent orthogonality: the chain_required rows are minted by the
//     window-census drilldown plan (top-N sleepers of the WHOLE window,
//     query.go buildStateDrilldownPlanForTarget), so the named subject can be
//     an idle peripheral thread unrelated to the user's question
//     (codrax-20260713-061452: a correct CompThread D-state answer was held
//     over udk-irq-1-76's sleep row);
//   - coverage is artifact-scoped and subject-blind (rowWakeupCovered): ONE
//     wakeup_chain run on any thread of the artifact discharges every row, so
//     the old "Run wakeup_chain for that thread" wording overclaimed what the
//     obligation actually was.
//
// Everything below the message rendering — reconcile-first evaluation,
// sibling-window rules, per-artifact one-shot — is preserved verbatim.
//
// CHAIN-RECONCILE (账本 §18.B B-2, P1): the SAME (thread, s_sleep) subject can
// carry TWO sibling state_drilldown rows across different calls — a
// narrow-window top_sleep row (FragmentCount<4 inside that window, so it
// SURVIVES the fragmented-sleep filter ⇒ chain_required=true) AND a
// wide-window state_churn row (fragmented churn decomposition, e.g. 101
// segments ⇒ chain_required=false). Both verdicts are correct for their own
// query window; the plan dedup in query.go collapses duplicates only WITHIN
// one call, so the contradiction is inherently cross-call and only this gate
// sees it. A gate that looked only at the chain_required=true row burned the
// one-shot retry even though the model had already seen the contradicting
// sibling ("state_drilldown shows chain_required=false but I'm being told to
// drill down" — the q9 wasted round). Reconcile rule: before downgrading a
// chain_required=true row, check whether the SAME (Subject, Object=s_sleep)
// also produced a state_churn / chain_required=false sibling whose query
// window OVERLAPS the chain-required row's window ([aStart,aEnd] ∩
// [bStart,bEnd] ≠ ∅, closed intervals). If it did, the thread's sleep in THAT
// window already has a wide-window decomposition evidence face and the hard
// gate stands down — the narrow-window chain requirement stays on every SOFT
// surface (see 反例 below). The window condition keeps the reconcile honest:
// a state_churn face from a DISJOINT window (thread churned in window A,
// fragmented-slept in window B) explains nothing about the sleep in B and
// must not waive B's drilldown. When the window note (selected_window,
// fallback window) is missing or unparseable on EITHER side, the pair
// conservatively does NOT reconcile — the obligation is kept rather than
// guessed away on an absent precise signal. If no overlapping sibling exists,
// the obligation stands and the downgrade fires, now with source/window
// attribution so the model can locate the triggering row instead of
// dismissing it as stale. All-typed judgement: Subject equality,
// Object=="s_sleep", the chain_required boolean note, the source enum note
// (source=state_churn — the ONLY producer lane of a chain_required=false
// s_sleep row, stateDrilldownNeedsWakeupChainForSource; requiring it pins the
// semantic justification, wide-window decomposition evidence, against future
// producer drift), and the selected_window/window interval notes. No prose
// scan, no heuristic.
//
// 反例 evaluation ("宽窗 false 但窄窗 true 确实该逼"?): the shape exists —
// the user's jank can sit inside a narrow window whose top_sleep is one long
// segment while the wide window shows fragmented churn. Standing down the
// HARD gate is still correct there because (a) every SOFT surface survives:
// the row itself keeps chain_required=true +
// recommended_views=wakeup_chain,root_cause_rank on the wire, and the RN-14c
// user-focus advisory and the projection's recommended-chain warning lane are
// untouched; (b) two typed rows disagreeing is exactly the noisy-signal
// condition of the precise-signals red line — the gate cannot know which
// window the user cares about, so it must not spend a user-visible denial on
// that coin flip; (c) the gate is one-shot — it only ever bought ONE nudge,
// and when the model's context already holds the contradicting sibling that
// nudge is the q9 wasted burn, not a correction.
//
// CHAIN-SCOPE (账本 §21 维度A③(b), 2026-07-07): the wakeup-family exemption,
// the sibling reconcile, and the one-shot marker are all scoped PER TRACE
// ARTIFACT. The former run-global haveWakeupFamily let one artifact's chain
// exempt EVERY artifact's obligation — in the cmp_01 dual-trace comparison
// the 7.0 trace's full wakeup chain (E1-E10) structurally waived the 6.0
// trace's chain_required=true debt, the one-shot was never consumed, and the
// 6.0 tree shipped with "本轮未执行唤醒链下钻". The artifact identity is the
// SAME typed lane the per-artifact projection partitioner keys on
// (types.TraceCausalProjectionRecordArtifactIdentityWithLabel — SourceRef
// path / artifact id / locator, never prose), so the gate's per-artifact
// obligation mirrors the per-partition WakeupChainRecommendedNotRun flag the
// display face compiles. Bucketing rules, all fail-open toward the legacy
// run-global behavior when identity is absent (precise-signal absence is
// never guessed over):
//   - a wakeup-family observation WITH identity exempts only its own
//     artifact; an identity-less one keeps the legacy global reach;
//   - a chain_required row WITH identity is owed unless ITS artifact (or an
//     identity-less wakeup observation) has a wakeup face; an identity-less
//     row keeps the legacy "any wakeup anywhere" exemption;
//   - a state_churn sibling reconciles only rows of the SAME artifact scope
//     (equal keys, or either side identity-less) — same-name threads in two
//     traces can no longer cover each other;
//   - the one-shot key carries the artifact identity suffix: at most ONE
//     nudge per artifact per run (the CHAIN-RECONCILE conservative arm
//     continues; total fires stay bounded by the finite typed artifact
//     identities of the run's actual trace files, so no livelock).
func wakeupChainDrilldownPendingAdvisory(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	// Chain-required sleep rows in scan order (deterministic downgrade pick,
	// with source/window/artifact attribution) and, per subject, the query
	// windows of state_churn / chain_required=false sibling rows.
	type chainRequiredRow struct {
		artifactKey   string // typed artifact-identity partition key ("" = identity-less legacy row)
		artifactLabel string // display basename for the guidance text
		subject       string
		source        string // source= enum note of the chain_required=true row
		window        string // selected_window (fallback window) note of that row
	}
	var chainRequiredRows []chainRequiredRow
	chainRequiredSubjects := map[string]bool{}
	siblingWindowsBySubject := map[string][]wakeupChainSiblingWindow{}
	wakeupFamilyGlobal := false // identity-less wakeup observation: legacy run-global reach
	wakeupFamilyByArtifact := map[string]bool{}
	for _, result := range append(ctx.Mutable.DispatchToolResults(), ctx.ToolResults...) {
		if types.CanonicalToolName(result.ToolName) != "trace_query" {
			continue
		}
		for _, obs := range result.Observations {
			switch strings.TrimSpace(obs.Predicate) {
			case "wakeup_chain", "wakeup_chain_edge", "wakeup_causal_impact", "wakeup_causal_aggregate":
				if key, _ := types.TraceCausalProjectionRecordArtifactIdentityWithLabel(obs); key != "" {
					wakeupFamilyByArtifact[key] = true
				} else {
					wakeupFamilyGlobal = true
				}
			case "state_drilldown":
				if strings.TrimSpace(obs.Object) != "s_sleep" {
					continue
				}
				subject := strings.TrimSpace(obs.Subject)
				if subject == "" {
					continue
				}
				source := traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeySource)
				window := traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeySelectedWindow)
				if window == "" {
					window = traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyWindow)
				}
				artifactKey, artifactLabel := types.TraceCausalProjectionRecordArtifactIdentityWithLabel(obs)
				switch traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyChainRequired) {
				case "true":
					chainRequiredSubjects[subject] = true
					chainRequiredRows = append(chainRequiredRows, chainRequiredRow{
						artifactKey:   artifactKey,
						artifactLabel: artifactLabel,
						subject:       subject,
						source:        source,
						window:        window,
					})
				case "false":
					// A chain-NOT-required sibling for the SAME sleep subject —
					// its evidence face (state_churn) covers the sleep without a
					// wakeup chain, but only for its own query window AND its own
					// artifact scope; the overlap check below decides per
					// chain-required row. state_churn is the canonical producer
					// of the chain_required=false sleep row (query.go:4839 via
					// stateDrilldownNeedsWakeupChainForSource); keying on it
					// keeps the reconcile precise.
					if source == "state_churn" && window != "" {
						siblingWindowsBySubject[subject] = append(siblingWindowsBySubject[subject],
							wakeupChainSiblingWindow{artifactKey: artifactKey, window: window})
					}
				}
			}
		}
	}
	if len(chainRequiredRows) == 0 {
		return ""
	}
	// CHAIN-SCOPE wakeup exemption, per row: an artifact-scoped wakeup face
	// covers only its own artifact; identity-less signals keep the legacy
	// run-global reach on BOTH sides.
	rowWakeupCovered := func(row chainRequiredRow) bool {
		if wakeupFamilyGlobal {
			return true
		}
		if row.artifactKey == "" {
			return len(wakeupFamilyByArtifact) > 0
		}
		return wakeupFamilyByArtifact[row.artifactKey]
	}
	// Pick the first (scan-order, deterministic) chain-required row that is
	// genuinely still owed: its own artifact has no wakeup-family face, no
	// state_churn / chain_required=false sibling of the same subject AND
	// artifact scope covers an overlapping window, and its artifact's one-shot
	// nudge is unburned. Missing/unparseable windows fail toward keeping the
	// obligation.
	var advisory *chainRequiredRow
	owed := 0
	for i := range chainRequiredRows {
		row := &chainRequiredRows[i]
		if rowWakeupCovered(*row) {
			continue
		}
		if wakeupChainSiblingWindowReconciles(row.artifactKey, row.window, siblingWindowsBySubject[row.subject]) {
			continue
		}
		owed++
		oneShotKey := "wakeup_chain_drilldown_pending"
		if row.artifactKey != "" {
			// Per-artifact one-shot (§21 CHAIN-SCOPE): each artifact gets at
			// most one note per run; identity-less rows keep the legacy
			// run-global key byte-identical.
			oneShotKey += "\x00" + row.artifactKey
		}
		if !ctx.Mutable.MarkCompletionGateOneShot(oneShotKey) {
			continue
		}
		advisory = row
		break
	}
	if advisory == nil {
		if owed == 0 {
			// Every chain_required=true sleep row was either covered by its own
			// artifact's wakeup face or reconciled by a state_churn /
			// chain_required=false sibling covering an overlapping window in the
			// same artifact scope — nothing to disclose, and the one-shot
			// markers stay unburned for this run.
			logging.Info("[emit_investigation_complete] wakeup-chain advisory reconciled away: %d chain_required=true sleep subject(s) each covered by an own-artifact wakeup face or a window-overlapping state_churn chain_required=false sibling", len(chainRequiredSubjects))
		}
		return ""
	}
	advisorySubject, advisoryRow := advisory.subject, *advisory
	logging.Info("[emit_investigation_complete] one-shot wakeup-chain drilldown advisory: subject=%s source=%s window=%s artifact=%s",
		advisorySubject, advisoryRow.source, advisoryRow.window, advisoryRow.artifactLabel)
	// Attribution rides the row's own typed notes verbatim (source= / query
	// window / trace artifact) so the note names the exact state_drilldown
	// row it is talking about; legacy rows without those notes degrade to the
	// bare flag. Honesty contract (§29.60): the subject is named as a
	// whole-window census candidate — NOT as the question's target thread —
	// and the suggested view is scoped to the trace artifact (any thread in
	// the window covers it), matching the actual reconcile semantics.
	attribution := "chain_required=true"
	if source := strings.TrimSpace(advisoryRow.source); source != "" {
		attribution += ", source=" + source
	}
	if window := strings.TrimSpace(advisoryRow.window); window != "" {
		attribution += ", query window " + window
	}
	scopeSentence := "No wakeup_chain view was run this turn."
	if strings.TrimSpace(advisoryRow.artifactLabel) != "" {
		attribution += ", trace artifact " + strings.TrimSpace(advisoryRow.artifactLabel)
		scopeSentence = "No wakeup_chain view was run against that trace artifact this turn — a wakeup chain drilled on a DIFFERENT trace artifact does not cover this one."
	}
	return "wakeup-chain coverage note: the window census ranked the sleep of " + advisorySubject +
		" as a wakeup-chain drilldown candidate (" + attribution + "), and no sibling state_drilldown row of the same thread over an overlapping query window reconciles it with chain_required=false (source=state_churn). " +
		scopeSentence +
		" That subject comes from the whole-window census ranking, not necessarily from the question's target thread. " +
		"If the answer explains why that thread slept, present the upstream waker as unverified; " +
		"a bounded trace_query(view=\"wakeup_chain\") on that trace artifact would cover it if exploration continues. " +
		"This note is delivered at most once per trace artifact"
}

// advisoryPendingReadCoverageNote renders the bounded disclosure for
// coverage-class pending reads demoted at an accepted completion (§29.60
// forced-read split, 2026-07-13): the suggestions are surfaced as unread
// coverage instead of blocking closure or silently vanishing.
func advisoryPendingReadCoverageNote(advisory []types.PendingRead) string {
	if len(advisory) == 0 {
		return ""
	}
	const maxListed = 4
	var files []string
	for _, p := range advisory {
		if f := strings.TrimSpace(p.File); f != "" {
			files = append(files, f)
		}
		if len(files) == maxListed {
			break
		}
	}
	note := fmt.Sprintf("coverage note: %d suggested file(s) were not read before completion", len(advisory))
	if len(files) > 0 {
		note += " (" + strings.Join(files, ", ")
		if len(advisory) > len(files) {
			note += fmt.Sprintf(", +%d more", len(advisory)-len(files))
		}
		note += ")"
	}
	note += " — the answer stands on the evidence actually gathered; present any claim about those files as unverified"
	return note
}

// wakeupChainSiblingWindow is one state_churn / chain_required=false sibling
// face: its typed query window plus the typed artifact identity it was
// observed on (§21 CHAIN-SCOPE — "" = identity-less legacy row).
type wakeupChainSiblingWindow struct {
	artifactKey string
	window      string
}

// wakeupChainSiblingWindowReconciles reports whether ANY state_churn sibling
// in the SAME artifact scope overlaps the chain-required row's window. Both
// sides must carry a parseable typed window note (selected_window / window,
// "start..end" floats as emitted by traceQueryWindowValue); a missing or
// malformed window on either side fails toward keeping the drilldown
// obligation — precise-signal absence is never guessed over. Closed-interval
// overlap: a state_churn face whose query window merely touches the
// chain-required window still covers the boundary instant; anything disjoint
// reconciles nothing. Artifact scope (§21 CHAIN-SCOPE): a sibling observed on
// a DIFFERENT trace artifact explains nothing about this artifact's sleep —
// same-name threads in two traces must not cover each other; either side
// being identity-less keeps the legacy subject-keyed reach.
func wakeupChainSiblingWindowReconciles(rowArtifactKey, requiredWindow string, siblingWindows []wakeupChainSiblingWindow) bool {
	reqStart, reqEnd, ok := traceWindowNoteInterval(requiredWindow)
	if !ok {
		return false
	}
	for _, sibling := range siblingWindows {
		if rowArtifactKey != "" && sibling.artifactKey != "" && rowArtifactKey != sibling.artifactKey {
			continue
		}
		sibStart, sibEnd, ok := traceWindowNoteInterval(sibling.window)
		if !ok {
			continue
		}
		if reqStart <= sibEnd && sibStart <= reqEnd {
			return true
		}
	}
	return false
}

// traceWindowNoteInterval parses the canonical typed window note value
// ("<start>..<end>" floats, the exact shape traceQueryWindowValue /
// traceQuerySelectedWindowNoteValue emit) into a closed interval. Local by
// design — the projection layer's interval algebra stays in its own package;
// this gate needs only plain [a,b] ∩ [c,d] ≠ ∅. Any deviation from the
// canonical shape returns false and the caller keeps the obligation.
func traceWindowNoteInterval(value string) (float64, float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(value, "..", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, errStart := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	end, errEnd := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errStart != nil || errEnd != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
}

// userFocusWakeupChainGapNote implements RN-14c (§7.9): when the analyzer
// pinned a user-focus thread (typed RuntimeTargets lane, same H4 channel the
// RN-14b hint reuses), the turn produced scheduler-shaped trace observations
// (state_drilldown or wakeup-chain family), and NO wakeup-chain-family
// observation has the focus thread as its chain TARGET, the accepted
// completion carries a soft advisory suggesting view=wakeup_chain
// pid=<focus> — with via_thread=<runnable anchor> when a runnable-dominant
// anchor thread is visible in the typed observations. All inputs are typed
// observation fields (Predicate / Object / depth / dominant_state / path
// notes); raw prose is never consulted, and the note never blocks completion.
func userFocusWakeupChainGapNote(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	focusPID, ok := analyzerPinnedFocusThreadPID(ctx)
	if !ok {
		return ""
	}
	sawSchedulerShape := false
	haveFocusChain := false
	anchorPID := 0
	for _, result := range append(ctx.Mutable.DispatchToolResults(), ctx.ToolResults...) {
		if types.CanonicalToolName(result.ToolName) != "trace_query" {
			continue
		}
		for _, obs := range result.Observations {
			switch strings.TrimSpace(obs.Predicate) {
			case "state_drilldown", "wakeup_chain", "wakeup_chain_edge", "wakeup_causal_impact", "wakeup_causal_aggregate":
				sawSchedulerShape = true
			}
			if pid, ok := traceChainObservationTargetPID(obs); ok && pid == focusPID {
				haveFocusChain = true
			}
			if anchorPID == 0 {
				if pid, ok := traceRunnableAnchorObservationPID(obs); ok && pid != focusPID {
					anchorPID = pid
				}
			}
		}
	}
	if !sawSchedulerShape || haveFocusChain {
		return ""
	}
	note := fmt.Sprintf("user-focus thread %d has no wakeup-chain evidence yet; consider view=wakeup_chain pid=%d", focusPID, focusPID)
	if anchorPID > 0 {
		note += fmt.Sprintf(" via_thread=%d — connect the runnable anchor to the user-focus thread's wakeup chain", anchorPID)
	}
	return note
}

// traceChainObservationTargetPID extracts the chain TARGET pid carried by a
// wakeup-chain-family observation (RN-14c §7.9). Carrier fields probed from
// the producers in trace_query.go: the wakeup_chain path observation's
// Subject is the chain target label; wakeup_causal_impact rows carry the
// target as their own Subject only at depth 0 (the target's self impact);
// aggregate rows carry it as the trailing element of their typed path note
// ("dependency -> ... -> target"). Everything else has no target carrier.
//
// F-1 (统一复核 2026-07-04): the impact producer's depth note rides the
// zero-drop traceQueryTypedCount (traceQueryTypedCausalImpactRichNotes in
// trace_query.go) — ChainDepth==0 publishes NO depth= note at all, so a
// literal depth="0" can never appear on the wire. ABSENCE of the note IS the
// precise depth-0 signal (absence ⟺ ChainDepth==0, presence ⟺ depth ≥ 1;
// zero-drop semantics pinned by a mutual comment at the producer). Without
// this lane a zero-edge (flat) chain run — which emits neither a path
// observation (traceQueryWakeupChainPath returns "" on zero edges) nor
// aggregate/edge rows — left ALL four recognition lanes dead and re-injected
// the "no wakeup-chain evidence" advisory on turns that already ran the
// focus chain.
func traceChainObservationTargetPID(obs types.ObservationRecord) (int, bool) {
	switch strings.TrimSpace(obs.Predicate) {
	case "wakeup_chain":
		return traceThreadLabelTrailingPID(obs.Subject)
	case "wakeup_causal_impact":
		if traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyDepth) == "" {
			return traceThreadLabelTrailingPID(obs.Subject)
		}
		return traceChainPathTailPID(traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyPath))
	case "wakeup_causal_aggregate", "wakeup_chain_edge":
		return traceChainPathTailPID(traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyPath))
	}
	return 0, false
}

// traceRunnableAnchorObservationPID identifies a runnable-dominant anchor
// thread from typed observation rows: a state_drilldown row whose Object is
// the typed runnable state, or a depth-0 wakeup_causal_impact whose typed
// dominant_state note is runnable. F-1: depth 0 is keyed on the ABSENCE of
// the depth= note (producer zero-drop, see traceChainObservationTargetPID).
func traceRunnableAnchorObservationPID(obs types.ObservationRecord) (int, bool) {
	switch strings.TrimSpace(obs.Predicate) {
	case "state_drilldown":
		if strings.TrimSpace(obs.Object) == string(tracequery.StateRunnable) {
			return traceThreadLabelTrailingPID(obs.Subject)
		}
	case "wakeup_causal_impact":
		if traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyDepth) == "" &&
			traceObservationNoteValue(obs.RichNotes, types.TraceNoteKeyDominantState) == string(tracequery.StateRunnable) {
			return traceThreadLabelTrailingPID(obs.Subject)
		}
	}
	return 0, false
}

// traceObservationNoteValue returns the value of one "key=value" typed rich
// note, or "" when absent.
func traceObservationNoteValue(notes []string, key string) string {
	prefix := key + "="
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

// traceThreadLabelTrailingPID parses the pid out of a canonical thread label
// ("comm-pid", "pid=N", or a bare pid) by exact integer parse of the trailing
// segment — never a substring scan of prose.
func traceThreadLabelTrailingPID(label string) (int, bool) {
	label = strings.TrimSpace(label)
	if label == "" {
		return 0, false
	}
	if rest, ok := strings.CutPrefix(label, "pid="); ok {
		if pid, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && pid > 0 {
			return pid, true
		}
		return 0, false
	}
	if pid, err := strconv.Atoi(label); err == nil && pid > 0 {
		return pid, true
	}
	if idx := strings.LastIndex(label, "-"); idx >= 0 && idx < len(label)-1 {
		if pid, err := strconv.Atoi(label[idx+1:]); err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

// traceChainPathTailPID returns the pid of the final (target) element of a
// typed chain path note "a-1 -> b-2 -> c-3".
func traceChainPathTailPID(path string) (int, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, false
	}
	parts := strings.Split(path, "->")
	return traceThreadLabelTrailingPID(parts[len(parts)-1])
}

// zeroCurrentSourceRepoCompletionBypassLabel waives the completion citation
// floor when the deterministic run-entry census proved the repository contains
// no current-source files at all: every regular file is itself a runtime
// artifact (e.g. a directory holding one .sys.ftrace and nothing else). In
// that repository shape a current-source citation floor is structurally
// unsatisfiable — no sequence of tool calls can ever produce a cite-eligible
// current-source line — so keeping the floor hard cannot improve grounding; it
// can only force the reject→reopen→re-read livelock observed in the customer
// trace_repl.log (2026-07-02, 37 consecutive completion denials). Unlike
// explicitCurrentSourceExclusionCompletionBypassLabel this does not depend on
// the analyzer emitting any policy: the signal is pure environment state
// (path-shape census over the checkout), which keeps this hard gate on a
// precise deterministic input.
func zeroCurrentSourceRepoCompletionBypassLabel(ctx *types.BusContext) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		return "zero_current_source_repo", true
	}
	return "", false
}

func traceQueryRuntimeObservationCompletionBypassLabel(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) (string, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return "", false
	}
	count := ctx.Mutable.TraceQueryRuntimeObservationCount()
	if count == 0 {
		for _, result := range append(ctx.Mutable.DispatchToolResults(), ctx.ToolResults...) {
			if !result.Success || types.CanonicalToolName(result.ToolName) != "trace_query" {
				continue
			}
			for _, observation := range result.Observations {
				if observation.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
					!types.RuntimeObservationProducerIsDeterministicQuery(observation.Producer) ||
					observation.SourceRef.Kind != types.ObservationSourceRuntimeArtifact ||
					observation.GroundingPolicy != types.ClaimGroundingHard {
					continue
				}
				count++
			}
		}
	}
	if count == 0 {
		return "", false
	}
	if ctx.AnalysisIR != nil {
		rm := ctx.AnalysisIR.RequestModel
		if allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLandingWithAggregateFacts(ctx, aggregateFacts); decided {
			if !allowed {
				return "", false
			}
		} else if types.RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, ctx.TurnRouteHint) == types.RuntimeSourceRequirementPrecise {
			return "", false
		}
		// A trace_query hard observation is already a typed runtime-artifact
		// citation surface. It may come from an explicit path in the user's
		// question rather than from --htrace/--log attachment pre-stages, so do
		// not require RequestModel path heuristics to rediscover the artifact at
		// completion time. The current-source requirement check above is the
		// contract boundary for mixed trace+source questions.
		if !traceQueryRuntimeObservationBypassCompatibleWithAggregateFacts(ctx, aggregateFacts) {
			return "", false
		}
		return fmt.Sprintf("trace_query_runtime_observations=%d", count), true
	}
	if !traceQueryRuntimeObservationBypassCompatibleWithAggregateFacts(ctx, aggregateFacts) {
		return "", false
	}
	return fmt.Sprintf("trace_query_runtime_observations=%d", count), true
}

func traceQueryRuntimeObservationBypassCompatibleWithAggregateFacts(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) bool {
	if len(aggregateFacts) == 0 {
		return true
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	for _, fact := range aggregateFacts {
		explicitOrigins := aggregateFactExplicitCompletionOrigins(fact)
		for _, origin := range explicitOrigins {
			if origin == types.AnswerEvidenceOriginCurrentSource ||
				(origin != types.AnswerEvidenceOriginRuntimeArtifact && types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin)) {
				return false
			}
		}
		if aggregateMemberSetOriginRequiresCurrentSource(ctx, &rm, []types.AnswerAggregateFact{fact}) {
			origins := types.AnswerAggregateFactEvidenceOrigins(fact, &rm)
			if len(origins) == 0 || !types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins) {
				return false
			}
		}
	}
	return true
}

func aggregateFactExplicitCompletionOrigins(fact types.AnswerAggregateFact) []types.AnswerEvidenceOrigin {
	seen := map[types.AnswerEvidenceOrigin]bool{}
	var out []types.AnswerEvidenceOrigin
	add := func(raw string) {
		origin := types.AnswerEvidenceOriginFromStructuredToken(raw)
		if origin == types.AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}
	add(fact.Provenance)
	for _, dim := range fact.Dimensions {
		add(dim.Value)
	}
	for _, ref := range fact.SupportRefs {
		add(ref)
	}
	return out
}

func originSpecificCompletionBypassLabel(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) (string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return "", false
	}
	rm := ctx.AnalysisIR.RequestModel
	if allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLandingWithAggregateFacts(ctx, aggregateFacts); decided {
		if !allowed {
			return "", false
		}
	} else if types.RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, ctx.TurnRouteHint) == types.RuntimeSourceRequirementPrecise {
		return "", false
	}
	originCounts := make(map[types.AnswerEvidenceOrigin]int)
	for _, fact := range aggregateFacts {
		origins := types.AnswerAggregateFactEvidenceOrigins(fact, &rm)
		if !completionAuthorityFactOriginsCanParticipate(origins) {
			continue
		}
		for _, origin := range origins {
			if origin == types.AnswerEvidenceOriginRuntimeArtifact {
				// DELIBERATE: runtime_artifact origins never satisfy this
				// bypass, and that is not a gap to "fix". A model-declared
				// aggregate dimension is a noisy self-reported signal; letting
				// it waive the citation floor would let any trace-flavored
				// claim close the investigation. Trace-only turns have two
				// PRECISE channels instead: deterministic trace_query hard
				// observations (traceQueryRuntimeObservationCompletionBypassLabel)
				// and the environment-state census
				// (zeroCurrentSourceRepoCompletionBypassLabel), plus the typed
				// ExcludesCurrentSource request boundary. See the
				// precise-signals-for-hard-gates rule in CLAUDE.md.
				continue
			}
			if origin == types.AnswerEvidenceOriginUnknown {
				continue
			}
			originCounts[origin]++
		}
	}
	if len(originCounts) == 0 {
		return "", false
	}
	var labels []string
	for origin, count := range originCounts {
		labels = append(labels, fmt.Sprintf("%s=%d", origin, count))
	}
	sort.Strings(labels)
	return "typed external-observation completion (" + strings.Join(labels, ", ") + ")", true
}

func runtimeArtifactGroundingBypassAllowed(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return true
	}
	rm := types.RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if rm == nil {
		return true
	}
	// A runtime-artifact-only repository (deterministic census: zero
	// current-source files) can never satisfy a current-source requirement,
	// so any hard requirement below is structurally unsatisfiable there —
	// honoring it would only strand model-declared waivers and reopen the
	// investigation forever.
	if ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		return true
	}
	attachedRuntimeArtifact := runtimeArtifactContextActiveForCompletion(ctx, rm)
	if runtimeArtifactCurrentSourceHardRequirement(ctx) {
		return false
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(attachedRuntimeArtifact) {
		return true
	}
	return attachedRuntimeArtifact
}

func runtimeSourceAnswerAuthorityForCompletion(ctx *types.BusContext) types.RuntimeSourceAnswerAuthoritySnapshot {
	return types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
}

func runtimeSourceAnswerAuthorityForCompletionWithAggregateFacts(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) types.RuntimeSourceAnswerAuthoritySnapshot {
	if len(aggregateFacts) == 0 {
		return runtimeSourceAnswerAuthorityForCompletion(ctx)
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	rm := types.RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	merged := append([]types.AnswerAggregateFact{}, input.AggregateFacts...)
	merged = append(merged, originSpecificCompletionAuthorityAggregateFacts(aggregateFacts, rm)...)
	input.AggregateFacts = types.MergeAnswerAggregateFacts(merged)
	return types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.CompileObservationLedger(input))
}

func originSpecificCompletionAuthorityAggregateFacts(aggregateFacts []types.AnswerAggregateFact, rm *types.RequestModel) []types.AnswerAggregateFact {
	if len(aggregateFacts) == 0 {
		return nil
	}
	out := make([]types.AnswerAggregateFact, 0, len(aggregateFacts))
	for _, fact := range aggregateFacts {
		if completionAuthorityFactOriginsCanParticipate(types.AnswerAggregateFactEvidenceOrigins(fact, rm)) {
			out = append(out, fact)
		}
	}
	return out
}

func completionAuthorityFactOriginsCanParticipate(origins []types.AnswerEvidenceOrigin) bool {
	if len(origins) == 0 || !types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins) {
		return false
	}
	for _, origin := range origins {
		if origin == types.AnswerEvidenceOriginRepoNegativeSearch {
			return false
		}
	}
	return true
}

func runtimeSourceAuthorityAllowsRuntimeCompletionLanding(ctx *types.BusContext) (bool, bool) {
	if runtimeSourceCompletionContractRequiresCurrentSourceProof(ctx) {
		return false, true
	}
	authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
	if !runtimeSourceAuthorityAppliesToCompletionLanding(authority) {
		return false, false
	}
	return runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority), true
}

func runtimeSourceAuthorityAllowsRuntimeCompletionLandingWithAggregateFacts(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) (bool, bool) {
	if runtimeSourceCompletionContractRequiresCurrentSourceProof(ctx) {
		return false, true
	}
	authority := runtimeSourceAnswerAuthorityForCompletionWithAggregateFacts(ctx, aggregateFacts)
	if !runtimeSourceAuthorityAppliesToCompletionLanding(authority) {
		return false, false
	}
	return runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority), true
}

func runtimeSourceCompletionContractRequiresCurrentSourceProof(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	contract := &ctx.AnalysisIR.AnswerContract
	if answerContractRequiresCurrentSourceProofForRuntimeCompletion(ctx, contract) {
		return true
	}
	if !contract.CitationReq.Required {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	return types.RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, ctx.TurnRouteHint) == types.RuntimeSourceRequirementPrecise
}

func runtimeSourceAuthorityAllowsRuntimeCompletionLandingBool(ctx *types.BusContext) bool {
	allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLanding(ctx)
	return decided && allowed
}

func runtimeSourceAuthorityAppliesToCompletionLanding(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	return authority.HasRuntimeCarrier() ||
		authority.CanDowngradeToCaveat ||
		authority.CanHardBlockCompletion ||
		authority.CurrentSourceSatisfied
}

func runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func runtimeArtifactCurrentSourceHardRequirement(ctx *types.BusContext) bool {
	required, _ := runtimeArtifactCurrentSourceHardRequirementLabeled(ctx)
	return required
}

// runtimeArtifactCurrentSourceHardRequirementLabeled reports whether a typed
// current-source requirement keeps runtime-artifact grounding bypasses closed,
// plus which requirement holds. The label feeds waiver-rejection feedback so
// the model learns WHAT still demands current-source evidence instead of
// looping blind (trace_repl.log 2026-07-02: the bare rejection line was
// retried 37 times with no way to discover the blocking contract).
func runtimeArtifactCurrentSourceHardRequirementLabeled(ctx *types.BusContext) (bool, string) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false, ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if answerContractRequiresCurrentSourceProofForRuntimeCompletion(ctx, &ctx.AnalysisIR.AnswerContract) {
		return true, "the answer contract requires a current-status/current-source diagnostic"
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true, "the request asks for change impact against the current checkout"
	}
	if currentSourceLaneHasSpecificSourceSeed(ctx) {
		return true, "the request names specific current-source files that must be inspected"
	}
	authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
	if authority.CanHardBlockCompletion {
		return true, "typed runtime/source authority still requires current-source verification"
	}
	return false, ""
}

func answerContractRequiresCurrentSourceProofForRuntimeCompletion(ctx *types.BusContext, contract *types.AnswerContract) bool {
	if contract == nil {
		return false
	}
	if contract.CurrentStatusDiagnostic != nil && contract.CurrentStatusDiagnostic.Required &&
		!runtimeCompletionSurfacePlanSuppressesCurrentStatus(ctx) {
		return true
	}
	return answerContractExactResolutionRequiresCurrentSourceProof(contract)
}

func answerContractExactResolutionRequiresCurrentSourceProof(contract *types.AnswerContract) bool {
	if contract == nil || contract.ExactResolution == nil {
		return false
	}
	exact := contract.ExactResolution
	for _, target := range append([]string{exact.TargetLabel}, exact.Targets...) {
		if currentSourceCoveragePath(target) {
			return true
		}
	}
	return currentSourceCoveragePath(exact.RelatedContextScopeHint)
}

func runtimeCompletionSurfacePlanSuppressesCurrentStatus(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	return plan != nil && !plan.CurrentStatusDiagnosticRequired
}

func applyEvidenceFloorWaiverPayload(ctx *types.BusContext, toolName string, p emitInvestigationCompleteParams) (string, *types.ToolResult) {
	if ctx == nil || ctx.Mutable == nil {
		return "", nil
	}
	// Store evidence_floor_waiver before any completion gate that can
	// legitimately relax for external runtime/log/trace surfaces. The
	// same emit call may declare both aggregate_facts and the waiver;
	// consumers must see the typed waiver before deciding whether a
	// source-line support requirement applies.
	if p.ClearEvidenceWaiver && p.EvidenceFloorWaiver != nil {
		return "", &types.ToolResult{
			ToolName: toolName,
			Summary: "emit_investigation_complete rejected: clear_evidence_floor_waiver cannot be set together with evidence_floor_waiver. " +
				"Either declare a new waiver or explicitly retract the existing one, not both.",
			Success:   false,
			Timestamp: time.Now(),
		}
	}
	if p.ClearEvidenceWaiver {
		ctx.Mutable.ClearEvidenceFloorWaiver()
		logging.Info("[emit_investigation_complete] evidence_floor_waiver explicitly cleared")
		return "", nil
	}
	if p.EvidenceFloorWaiver == nil {
		return "", nil
	}

	waiverReason := strings.TrimSpace(p.EvidenceFloorWaiver.Reason)
	waiverRationale := strings.TrimSpace(p.EvidenceFloorWaiver.Rationale)
	if waiverReason == "" {
		ctx.Mutable.SetEvidenceFloorWaiver(nil)
		ignored := "ignored evidence_floor_waiver because reason is missing; normal grounding gates still apply"
		logging.Warning("[emit_investigation_complete] %s", ignored)
		return ignored, nil
	}
	if waiverRationale == "" {
		ctx.Mutable.SetEvidenceFloorWaiver(nil)
		ignored := fmt.Sprintf("ignored evidence_floor_waiver=%s because rationale is missing; normal grounding gates still apply", waiverReason)
		logging.Warning("[emit_investigation_complete] %s", ignored)
		return ignored, nil
	}
	typedReason := types.EvidenceFloorWaiverReason(waiverReason)
	if !typedReason.IsValid() {
		ctx.Mutable.SetEvidenceFloorWaiver(nil)
		ignored := fmt.Sprintf("ignored evidence_floor_waiver.reason=%q because it is not accepted; normal grounding gates still apply", waiverReason)
		logging.Warning("[emit_investigation_complete] %s", ignored)
		return ignored, nil
	}

	artifactAttached := runtimeArtifactContextActiveForCompletion(ctx, requestModelForWaiver(ctx))
	if evidenceFloorWaiverIsPureVCSHistoryMisuse(ctx, typedReason, artifactAttached) {
		ctx.Mutable.SetEvidenceFloorWaiver(nil)
		ignored := fmt.Sprintf("ignored evidence_floor_waiver=%s for pure VCS history; carry git findings through reason/aggregate_facts, not runtime-artifact waiver", typedReason)
		logging.Warning("[emit_investigation_complete] %s rationale=%q",
			ignored, truncateForLog(waiverRationale, 200))
		return ignored, nil
	}
	if !runtimeArtifactGroundingBypassAllowed(ctx) {
		ctx.Mutable.SetEvidenceFloorWaiver(nil)
		// Name the exact blocker so a rejected waiver is actionable instead
		// of a bare loop: the model must know WHICH typed requirement still
		// demands current-source evidence (or that no runtime artifact
		// context is active at all) to make progress on the next attempt.
		ignored := fmt.Sprintf("ignored evidence_floor_waiver=%s because no typed runtime artifact context is active this turn; the waiver applies to runs grounded in an attached or referenced log/trace", typedReason)
		if required, label := runtimeArtifactCurrentSourceHardRequirementLabeled(ctx); required {
			ignored = fmt.Sprintf("ignored evidence_floor_waiver=%s: %s. Keep runtime observations in reason/aggregate_facts and add current-source evidence for that requirement before closing", typedReason, label)
		}
		logging.Warning("[emit_investigation_complete] %s rationale=%q",
			ignored, truncateForLog(waiverRationale, 200))
		return ignored, nil
	}

	ctx.Mutable.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    typedReason,
		Rationale: waiverRationale,
	})
	logging.Info("[emit_investigation_complete] evidence_floor_waiver accepted: reason=%s artifact_attached=%t rationale=%q",
		typedReason, artifactAttached, truncateForLog(waiverRationale, 200))
	return "", nil
}

func runtimeArtifactContextActiveForCompletion(ctx *types.BusContext, rm *types.RequestModel) bool {
	if types.RuntimeArtifactContextActiveFromBus(ctx) {
		return true
	}
	if rm == nil {
		return false
	}
	// HasTypedRuntimeArtifactPathIdentity supersedes the policy-gated
	// HasRuntimeArtifactPathReference here on purpose: this predicate only
	// widens a RELAXATION surface (whether a model-declared waiver is
	// admissible), so the identity of an analyzer-extracted artifact path is
	// enough — demanding the LLM-emitted citation policy as well stranded
	// trace-only turns whenever the analyzer skipped the policy field.
	return rm.LogTriage != nil || rm.PerfTrace != nil || rm.HasTypedRuntimeArtifactPathIdentity()
}

func requestModelForWaiver(ctx *types.BusContext) *types.RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if ctx.Mutable != nil {
		if rm.LogTriage == nil {
			rm.LogTriage = ctx.Mutable.LogTriage()
		}
		if rm.PerfTrace == nil {
			rm.PerfTrace = ctx.Mutable.PerfTrace()
		}
	}
	return &rm
}

// Tombstone (§29.151 UPTAIL-1 件5): requestModelHasRequiredCurrentKeyCodeDimension
// lived here with zero callers — a stranded copy of the bare Required∧Role
// current_key_code word-face check. Deleted rather than revived: the live
// hard-gate face is types.RequestModel.HasRequiredCurrentKeyCodeDimensionWithPreciseAnchor
// (precise-anchor-only, 件4), and any future caller must use that form.

func historyCountAggregateHandoffDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion || !rm.Predicates.IsHistoryLookup {
		return ""
	}
	if aggregateFactsContainCountAnswer(aggregateFacts) || aggregateFactsContainDeterministicCountScalar(aggregateFacts) {
		return ""
	}
	if closure != nil {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Keywords:  dedupStringsPreserveOrder(append(append([]string{}, rm.AnalyzerHints.ExactTargets...), append(rm.AnalyzerHints.Entities, rm.AnalyzerHints.Keywords...)...)),
			Rationale: "history count needs a model-authored aggregate fact for the verified filtered result so broad command-output counts cannot be mistaken for the answer",
			Origin:    "pre_complete.history_count_aggregate_handoff",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — history count aggregate handoff is missing.\n\n")
	b.WriteString("This is a historical count question. Raw git / shell output can contain broad candidate counts, commit-message matches, and the verified filtered result at the same time; finalization must not choose among those numbers from raw output memory.\n\n")
	b.WriteString("Emit `aggregate_facts` with kind=`total_count`, `unique_count`, `scalar_value`, or `member_set` for the verified answer count. Use `dimensions` to name the history window, filter basis, and any broad candidate pool / exclusions when they differ. The value must come from your verified tool output or structured evidence, not from closure prose. If you use a single deterministic VCS command instead, its final output line must label the exact answer as `answer_count=<n>`, `intersection_count=<n>`, `filtered_count=<n>`, `result_count=<n>`, `vcs_count=<n>`, or `history_count=<n>` so the system can distinguish the final answer from broad candidate counts. Then re-call `emit_investigation_complete`.")
	return b.String()
}

func deterministicCountProofDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion {
		return ""
	}
	if aggregateFactsContainCountAnswer(aggregateFacts) || aggregateFactsContainDeterministicCountScalar(aggregateFacts) {
		return ""
	}
	if hasDeterministicCountToolResult(ctx) {
		return ""
	}
	if closure != nil {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  dedupStringsPreserveOrder(append(append([]string{}, rm.AnalyzerHints.ExactTargets...), append(rm.AnalyzerHints.Entities, rm.AnalyzerHints.Keywords...)...)),
			Rationale: "count question lacks deterministic command output; run exec_command with a counting pipeline that prints the final integer, then use that exact output as the count proof",
			Origin:    "pre_complete.deterministic_count_proof",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — deterministic count proof is missing.\n\n")
	b.WriteString("This is a count question whose answer is a derived integer. The investigation has read/evidence items, but no successful `exec_command` result with an integer count. Do not close from visual tallying of read_file or grep snippets.\n\n")
	b.WriteString("Run a deterministic counting command that prints the final integer (for example: `rg ... | <production/comment/test filter> | wc -l`, `find ... | wc -l`, `wc -l ...`, or the platform-equivalent command for the repository language), or emit `aggregate_facts` with an exact count/member set derived from verified candidate classification. Then re-call emit_investigation_complete.")
	return b.String()
}

func aggregateFactsContainCountAnswer(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		switch fact.Kind {
		case types.AnswerAggregateTotalCount, types.AnswerAggregateUniqueCount,
			types.AnswerAggregateGroupedCount, types.AnswerAggregateBucketCount,
			types.AnswerAggregateMemberSet:
			return true
		}
	}
	if aggregateFactsContainUnambiguousScalarCountAnswer(facts) {
		return true
	}
	return false
}

func aggregateFactsContainUnambiguousScalarCountAnswer(facts []types.AnswerAggregateFact) bool {
	var value int
	found := false
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar {
			continue
		}
		role := types.NormalizeAnswerAggregateRole(fact.Role)
		if role == types.AnswerAggregateRoleSupportingCoverage || role == types.AnswerAggregateRoleAuditLedger {
			continue
		}
		n, ok := parseAggregateScalarIntegerValue(fact)
		if !ok {
			continue
		}
		if found && n != value {
			return false
		}
		value = n
		found = true
	}
	return found
}

func parseAggregateScalarIntegerValue(fact types.AnswerAggregateFact) (int, bool) {
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return n, true
}

func relationMemberSetHandoffDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.RequiresRelationMemberSetHandoff(rm) &&
		!exactTypedRelationProjectionActive(ctx, aggregateFacts) {
		return ""
	}
	ok, invalid := exhaustiveEnumerationMemberSetUsable(ctx, aggregateFacts)
	relationGaps := relationMemberSetCoverageGaps(ctx, aggregateFacts)
	scopeViolations := relationPrincipalMemberSetScopeViolations(ctx, aggregateFacts)
	if ok && len(relationGaps) == 0 && len(scopeViolations) == 0 {
		return ""
	}
	if closure != nil {
		files := completionMaterializationReadFiles(closure)
		for _, gap := range relationGaps {
			files = append(files, gap.File)
		}
		for _, violation := range scopeViolations {
			files = append(files, violation.File)
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     dedupStringsPreserveOrder(files),
			Keywords:  dedupStringsPreserveOrder(append(append([]string{}, types.StructuralRelationScopeCandidates(rm)...), append(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities...)...)),
			Rationale: "relation lookup needs a typed principal member_set so finalization answers with qualifying members before mechanism explanation",
			Origin:    "pre_complete.relation_member_set",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — relation member-set handoff is missing.\n\n")
	b.WriteString("The current question is a typed relation lookup with a set/count/enumerate answer shape. Do not close with only a mechanism explanation, raw read_file memory, or closure prose; finalization needs the qualifying members as model-authored structured data.\n\n")
	if strings.TrimSpace(invalid) != "" {
		fmt.Fprintf(&b, "A `member_set` was present but is not usable as relation principal-member data: %s\n\n", invalid)
	}
	if len(relationGaps) > 0 {
		fmt.Fprintf(&b, "The current `member_set` omits grounded typed relation evidence that was already emitted and is inside the requested source scope: %s. Include these members in the principal `member_set`, or place them in `excluded[]` with an explicit reason if the user-facing answer intentionally excludes them.\n\n", relationMemberSetGapSummary(relationGaps))
	}
	if len(scopeViolations) > 0 {
		fmt.Fprintf(&b, "The current principal `member_set` promotes exact typed relation rows outside the analyzer-emitted source scope: %s. Keep those rows in the complete audit roster, but move them to `excluded[]` or an explicitly auxiliary aggregate; do not count them in the principal relation set unless `SourceScopeProfile` selects that role.\n\n", relationScopeViolationSummary(scopeViolations))
	}
	b.WriteString("Emit `aggregate_facts` with kind=`member_set`, value equal to the exact qualifying-member count, and members containing the verified relation answer set. For relation rows you may use compact surfaces such as `caller → callee`, `package: entry`, `module/import`, or a plain qualifying member when the relation target is already clear from the request and evidence. Each member must be backed by typed evidence or member-specific support_refs. Then re-call `emit_investigation_complete`.")
	return b.String()
}

type relationCoverageGap struct {
	Relation   types.TypedRelationKind
	SourceName string
	Name       string
	File       string
	Line       int
}

type relationScopeViolation struct {
	Name       string
	SourceRole types.SourcePathRole
	File       string
}

// relationPrincipalMemberSetScopeViolations prevents an exact auxiliary
// structural member from being minted as a principal answer row under the
// default production scope. It consumes only the typed relation candidate
// protocol and structured aggregate_facts members. Unknown or ambiguous roles
// fail open; raw request text, closure prose, and final-answer prose are never
// read.
func relationPrincipalMemberSetScopeViolations(ctx *types.BusContext, facts []types.AnswerAggregateFact) []relationScopeViolation {
	if ctx == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.RequiresRelationMemberSetHandoff(rm) &&
		!exactTypedRelationProjectionActive(ctx, facts) {
		return nil
	}
	candidates := relationCandidatesForRequestAllSourceRoles(ctx, rm)
	if len(candidates) == 0 {
		return nil
	}
	byMember := map[string][]types.TypedRelationMember{}
	for _, candidate := range candidates {
		member := types.NormalizeTypedRelationMemberSourceRole(candidate.Member)
		if member.SourceRole == types.SourcePathRoleUnknown {
			continue
		}
		key := aggregateMemberKey(member.Name)
		if key == "" {
			continue
		}
		byMember[key] = append(byMember[key], member)
	}
	if len(byMember) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []relationScopeViolation
	for _, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			!types.AnswerAggregateFactRoleForRequest(fact, &rm).IsPrincipal() {
			continue
		}
		for _, raw := range fact.Members {
			var matches []types.TypedRelationMember
			for _, display := range aggregateMemberDisplayCandidates(raw) {
				matches = append(matches, byMember[aggregateMemberKey(display)]...)
			}
			if len(matches) == 0 {
				continue
			}
			inScope := false
			for _, member := range matches {
				if relationSourceInRequestedScope(member.File, rm) {
					inScope = true
					break
				}
			}
			// Same-name production + auxiliary candidates are ambiguous and
			// therefore fail open. Exact all-auxiliary matches are rejected.
			if inScope {
				continue
			}
			member := matches[0]
			key := aggregateMemberKey(raw) + "|" + string(member.SourceRole) + "|" + member.File
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, relationScopeViolation{
				Name:       strings.TrimSpace(raw),
				SourceRole: member.SourceRole,
				File:       member.File,
			})
		}
	}
	return out
}

func relationScopeViolationSummary(violations []relationScopeViolation) string {
	parts := make([]string, 0, len(violations))
	for _, violation := range violations {
		part := strings.TrimSpace(violation.Name)
		if violation.SourceRole != types.SourcePathRoleUnknown {
			part += " [source_role=" + string(violation.SourceRole) + "]"
		}
		if violation.File != "" {
			part += " @ " + violation.File
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func relationMemberSetCoverageGaps(ctx *types.BusContext, facts []types.AnswerAggregateFact) []relationCoverageGap {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.HasTypedRelationMemberSetShape(rm) &&
		!exactTypedRelationProjectionActive(ctx, facts) {
		return nil
	}
	expected := relationCandidatesForRequest(ctx, rm)
	if len(expected) == 0 {
		return nil
	}
	evidenceKeys := relationGroundedCandidateEvidenceKeys(ctx, expected)
	if len(evidenceKeys) == 0 {
		return nil
	}
	var gaps []relationCoverageGap
	for _, item := range expected {
		memberKey := aggregateMemberKey(item.Member.Name)
		candidateKey := relationCandidateEvidenceKey(item)
		pairIncluded, pairExcluded := relationPrincipalMemberSetCandidateCoverage(facts, &rm, item)
		if memberKey == "" || candidateKey == "" || !evidenceKeys[candidateKey] || pairIncluded || pairExcluded {
			continue
		}
		gaps = append(gaps, relationCoverageGap{
			Relation:   item.Relation,
			SourceName: item.SourceName,
			Name:       strings.TrimSpace(item.Member.Name),
			File:       canonicalRelationSourcePath(item.Member.File),
			Line:       item.Member.Line,
		})
	}
	return gaps
}

// relationPrincipalMemberSetCandidateCoverage recognizes a model-authored
// structured relation row such as "Handler -> /route" as coverage for that
// exact directed typed candidate. The member_set field is already a typed
// structured carrier; this helper never scans request, closure, or answer
// prose. Direction and both endpoint identities must match, so an unrelated or
// reversed pair cannot satisfy the gate.
func relationPrincipalMemberSetCandidateCoverage(
	facts []types.AnswerAggregateFact,
	rm *types.RequestModel,
	candidate types.TypedRelationCandidate,
) (included bool, excluded bool) {
	for _, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			!types.AnswerAggregateFactRoleForRequest(fact, rm).IsPrincipal() {
			continue
		}
		for _, raw := range fact.Members {
			if relationAggregateMemberMatchesCandidate(raw, candidate) {
				included = true
			}
		}
		for _, raw := range fact.Excluded {
			if relationAggregateMemberMatchesCandidate(raw, candidate) {
				excluded = true
			}
		}
	}
	return included, excluded
}

func relationAggregateMemberMatchesCandidate(raw string, candidate types.TypedRelationCandidate) bool {
	memberKey := aggregateMemberKey(candidate.Member.Name)
	if memberKey == "" {
		return false
	}
	if left, right, ok := relationAggregateMemberDirectedPair(raw); ok {
		sourceKey := aggregateMemberKey(candidate.SourceName)
		return sourceKey != "" &&
			aggregateMemberKey(left) == sourceKey &&
			aggregateMemberKey(right) == memberKey
	}
	// Preserve the established plain-member lane for implementers, callers,
	// imports, and other relations whose target is already clear in the typed
	// request.
	for _, display := range aggregateMemberDisplayCandidates(raw) {
		if aggregateMemberKey(display) == memberKey {
			return true
		}
	}
	return false
}

func relationAggregateMemberDirectedPair(raw string) (left string, right string, ok bool) {
	if left, right, ok = types.AnswerAggregateMemberRelationParts(raw); ok {
		return left, right, true
	}
	// The generic aggregate display parser intentionally rejects slash-bearing
	// atoms so paths are not mistaken for compact relations. Here both sides
	// are compared against an already-typed candidate, so an explicit arrow can
	// safely carry route/file-like endpoints without broadening that parser.
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, "\n\r") {
		return "", "", false
	}
	for _, sep := range []string{"→", "->", "=>"} {
		if strings.Count(raw, sep) != 1 {
			continue
		}
		parts := strings.SplitN(raw, sep, 2)
		left, right = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if left != "" && right != "" {
			return left, right, true
		}
	}
	return "", "", false
}

func relationCandidatesForRequest(ctx *types.BusContext, rm types.RequestModel) []types.TypedRelationCandidate {
	query := types.BuildTypedRelationQuery(rm, types.TypedRelationPurposeCoverageGate, 0)
	if len(query.Kinds) == 0 || len(query.Sources) == 0 {
		return nil
	}
	if provider, ok := ctx.MultiGraph.(types.TypedRelationCandidateSource); ok && provider != nil {
		if out := relationFilterCoverageCandidates(provider.TypedRelationCandidates(query), rm); len(out) > 0 {
			return out
		}
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if out := relationFilterCoverageCandidates(rmrelation.TypedRelationCandidates(graph, query), rm); len(out) > 0 {
		return out
	}
	if out := relationFilterCoverageCandidates((types.EvidenceRelationCandidateSource{Items: relationEvidenceItemsForCoverage(ctx)}).TypedRelationCandidates(query), rm); len(out) > 0 {
		return out
	}
	if query.AllowsKind(types.TypedRelationImplements) {
		if provider, ok := ctx.MultiGraph.(types.TypedRelationImplementerSource); ok && provider != nil {
			if out := relationTypedImplementersFromProvider(provider, query.Sources, rm); len(out) > 0 {
				return out
			}
		}
		if out := relationTypedImplementersFromGraph(graph, query.Sources, rm); len(out) > 0 {
			return out
		}
	}
	return nil
}

// relationCandidatesForRequestAllSourceRoles returns the same exact typed
// structural universe before request-scope projection. It exists only to
// detect principal member_set rows that promote an auxiliary relation member;
// normal coverage still uses relationCandidatesForRequest and therefore
// requires every in-scope expected member.
func relationCandidatesForRequestAllSourceRoles(ctx *types.BusContext, rm types.RequestModel) []types.TypedRelationCandidate {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	query := types.BuildTypedRelationQuery(rm, types.TypedRelationPurposeCoverageGate, 0)
	if len(query.Kinds) == 0 || len(query.Sources) == 0 {
		return nil
	}
	if provider, ok := ctx.MultiGraph.(types.TypedRelationCandidateSource); ok && provider != nil {
		if out := relationFilterCoverageCandidatesAllSourceRoles(provider.TypedRelationCandidates(query)); len(out) > 0 {
			return out
		}
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if out := relationFilterCoverageCandidatesAllSourceRoles(rmrelation.TypedRelationCandidates(graph, query)); len(out) > 0 {
		return out
	}
	return relationFilterCoverageCandidatesAllSourceRoles((types.EvidenceRelationCandidateSource{
		Items: relationEvidenceItemsForCoverage(ctx),
	}).TypedRelationCandidates(query))
}

// exactTypedRelationProjectionActive recognizes the analyzer-drift shape where
// a request is classified as source_inventory even though the accepted
// principal member_set is exactly a typed relation roster. It is deliberately
// downstream and evidence-driven: the structured request must ask for a typed
// relation/diagram member surface, an exact provider must return candidates,
// and every principal member must match either an exact relation member or its
// exact source anchor. A generic "all types" set with unrelated rows therefore
// cannot activate this lane.
func exactTypedRelationProjectionActive(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || len(facts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !exactTypedRelationProjectionRequestShape(rm) {
		return false
	}
	candidates := relationCandidatesForRequestAllSourceRoles(ctx, rm)
	if len(candidates) == 0 {
		return false
	}
	return len(exactTypedRelationPrincipalFactIndexes(facts, rm, candidates)) > 0
}

func exactTypedRelationProjectionRequestShape(rm types.RequestModel) bool {
	if types.HasTypedRelationMemberSetShape(rm) {
		return true
	}
	if !rm.Predicates.IsCategoryEnumeration ||
		rm.DiagramHint == nil ||
		rm.DiagramHint.Kind == types.DiagramNone ||
		rm.SourceInventoryProfile == nil ||
		!rm.SourceInventoryProfile.Active() {
		return false
	}
	for _, role := range rm.SourceInventoryProfile.PrincipalTargetRoles() {
		if role == types.AnswerCandidateRoleType {
			return true
		}
	}
	return false
}

func exactTypedRelationPrincipalFactIndexes(
	facts []types.AnswerAggregateFact,
	rm types.RequestModel,
	candidates []types.TypedRelationCandidate,
) map[int]bool {
	memberKeys := map[string]bool{}
	sourceKeys := map[string]bool{}
	for _, candidate := range candidates {
		if !candidate.Precision.CoverageGateEligible() {
			continue
		}
		if key := aggregateMemberKey(candidate.Member.Name); key != "" {
			memberKeys[key] = true
		}
		if key := aggregateMemberKey(candidate.SourceName); key != "" {
			sourceKeys[key] = true
		}
	}
	if len(memberKeys) == 0 {
		return nil
	}
	out := map[int]bool{}
	for i, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			!types.AnswerAggregateFactRoleForRequest(fact, &rm).IsPrincipal() ||
			len(fact.Members) == 0 {
			continue
		}
		matchedMembers := map[string]bool{}
		allRecognized := true
		for _, raw := range fact.Members {
			recognized := false
			_, _, structuredPair := relationAggregateMemberDirectedPair(raw)
			for _, candidate := range candidates {
				if !relationAggregateMemberMatchesCandidate(raw, candidate) {
					continue
				}
				if key := relationCandidateEvidenceKey(candidate); key != "" {
					matchedMembers[key] = true
				}
				recognized = true
			}
			if structuredPair {
				if !recognized {
					allRecognized = false
					break
				}
				continue
			}
			for _, display := range aggregateMemberDisplayCandidates(raw) {
				key := aggregateMemberKey(display)
				if key == "" {
					continue
				}
				if memberKeys[key] {
					matchedMembers[key] = true
					recognized = true
				}
				if sourceKeys[key] {
					recognized = true
				}
			}
			if !recognized {
				allRecognized = false
				break
			}
		}
		if allRecognized && len(matchedMembers) > 0 {
			out[i] = true
		}
	}
	return out
}

func markExactTypedRelationPrincipalMemberSets(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if len(facts) == 0 {
		return facts
	}
	// This provenance token is reserved for the system decision below. Strip
	// any model-supplied copy first so downstream code never trusts a spoofed
	// marker.
	out := cloneCompletionAggregateFacts(facts)
	for i := range out {
		provenance := strings.ReplaceAll(out[i].Provenance, types.TypedRelationPrincipalMemberSetAggregateProvenance, "")
		provenance = strings.ReplaceAll(provenance, types.TypedRelationImplicitSiblingAggregateDemotionProvenance, "")
		provenance = strings.ReplaceAll(provenance, types.TypedExclusionExactEmptyMemberSetAggregateProvenance, "")
		out[i].Provenance = strings.Trim(provenance, " ;,")
	}
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return out
	}
	rm := ctx.AnalysisIR.RequestModel
	if !exactTypedRelationProjectionRequestShape(rm) {
		return out
	}
	candidates := relationCandidatesForRequestAllSourceRoles(ctx, rm)
	indexes := exactTypedRelationPrincipalFactIndexes(out, rm, candidates)
	if len(indexes) == 0 {
		return out
	}
	for i := range out {
		if !indexes[i] ||
			strings.Contains(out[i].Provenance, types.TypedRelationPrincipalMemberSetAggregateProvenance) {
			continue
		}
		var removed int
		out[i], removed = canonicalizeExactTypedRelationFactMembers(out[i], candidates)
		if removed > 0 {
			logging.Info("[emit_investigation_complete] canonicalized %d duplicate typed relation member row(s) onto the accepted candidate identity axis", removed)
		}
		if strings.TrimSpace(out[i].Provenance) == "" {
			out[i].Provenance = types.TypedRelationPrincipalMemberSetAggregateProvenance
		} else {
			out[i].Provenance = strings.TrimSpace(out[i].Provenance) + "; " +
				types.TypedRelationPrincipalMemberSetAggregateProvenance
		}
	}
	// An exact typed relation set owns the default principal relation axis.
	// Other model-emitted member sets are still valuable context, but an
	// omitted role must not accidentally mint a second competing principal
	// roster (and trigger multiple deterministic carrier compilers). Preserve
	// explicit role=principal_answer byte-for-byte: that is the model's typed
	// decision that the request genuinely has another principal axis.
	for i := range out {
		if indexes[i] ||
			out[i].Kind != types.AnswerAggregateMemberSet ||
			len(out[i].Members) == 0 ||
			types.NormalizeAnswerAggregateRole(out[i].Role) != types.AnswerAggregateRoleUnknown ||
			!types.AnswerAggregateFactRoleForRequest(out[i], &rm).IsPrincipal() {
			continue
		}
		out[i].Role = types.AnswerAggregateRoleSupportingCoverage
		if strings.TrimSpace(out[i].Provenance) == "" {
			out[i].Provenance = types.TypedRelationImplicitSiblingAggregateDemotionProvenance
		} else {
			out[i].Provenance = strings.TrimSpace(out[i].Provenance) + "; " +
				types.TypedRelationImplicitSiblingAggregateDemotionProvenance
		}
	}
	return out
}

func relationEvidenceItemsForCoverage(ctx *types.BusContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	var out []types.EvidenceItem
	seen := map[string]bool{}
	add := func(items []types.EvidenceItem) {
		for _, item := range items {
			key := types.StableEvidenceID(item)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	add(ctx.EvidenceItems)
	if ctx.Mutable != nil {
		add(ctx.Mutable.EmittedEvidence())
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			add(ta.EvidenceItems)
		}
	}
	return out
}

func relationTypedImplementersFromProvider(provider types.TypedRelationImplementerSource, candidates []string, rm types.RequestModel) []types.TypedRelationCandidate {
	if provider == nil {
		return nil
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		members := provider.ImplementerMembersOf(candidate)
		if len(members) == 0 {
			continue
		}
		var out []types.TypedRelationCandidate
		seen := map[string]bool{}
		for _, member := range members {
			name := strings.TrimSpace(member.Name)
			if name == "" {
				continue
			}
			file := canonicalRelationSourcePath(member.File)
			if !relationSourceInRequestedScope(file, rm) {
				continue
			}
			key := aggregateMemberKey(name) + "|" + file
			if seen[key] {
				continue
			}
			seen[key] = true
			member.Name = name
			member.File = file
			member = types.NormalizeTypedRelationMemberSourceRole(member)
			out = append(out, types.TypedRelationCandidate{
				Relation:   types.TypedRelationImplements,
				SourceName: candidate,
				Member:     member,
				Carrier:    types.TypedRelationCarrierMultiGraph,
				Precision:  types.TypedRelationPrecisionExactSymbolID,
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func relationTypedImplementersFromGraph(graph *repotypes.Graph, candidates []string, rm types.RequestModel) []types.TypedRelationCandidate {
	if graph == nil {
		return nil
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		ids := graph.ImplementersOf(candidate)
		if len(ids) == 0 {
			continue
		}
		var out []types.TypedRelationCandidate
		seen := map[string]bool{}
		for _, id := range ids {
			sym, ok := graph.SymbolByID[id]
			if !ok || sym == nil || strings.TrimSpace(sym.Name) == "" {
				continue
			}
			file := canonicalRelationSourcePath(sym.File)
			if !relationSourceInRequestedScope(file, rm) {
				continue
			}
			key := aggregateMemberKey(sym.Name) + "|" + file
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, types.TypedRelationCandidate{
				Relation:   types.TypedRelationImplements,
				SourceName: candidate,
				Member: types.TypedRelationMember{
					Name:       strings.TrimSpace(sym.Name),
					File:       file,
					Line:       sym.Line,
					Kind:       sym.Kind,
					SourceRole: types.ClassifySourcePathRole(file),
					Distance:   1,
				},
				Carrier:   types.TypedRelationCarrierGraph,
				Precision: types.TypedRelationPrecisionExactSymbolID,
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func relationFilterCoverageCandidates(candidates []types.TypedRelationCandidate, rm types.RequestModel) []types.TypedRelationCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]types.TypedRelationCandidate, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if !candidate.CoverageGateEligible() {
			continue
		}
		member := candidate.Member
		member.Name = strings.TrimSpace(member.Name)
		member.File = canonicalRelationSourcePath(member.File)
		member = types.NormalizeTypedRelationMemberSourceRole(member)
		if member.Name == "" || !relationSourceInRequestedScope(member.File, rm) {
			continue
		}
		candidate.Member = member
		key := relationCandidateEvidenceKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func relationFilterCoverageCandidatesAllSourceRoles(candidates []types.TypedRelationCandidate) []types.TypedRelationCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]types.TypedRelationCandidate, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if !candidate.CoverageGateEligible() {
			continue
		}
		member := candidate.Member
		member.Name = strings.TrimSpace(member.Name)
		member.File = canonicalRelationSourcePath(member.File)
		member = types.NormalizeTypedRelationMemberSourceRole(member)
		if member.Name == "" || member.SourceRole == types.SourcePathRoleUnknown {
			continue
		}
		candidate.Member = member
		key := relationCandidateEvidenceKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func relationGroundedCandidateEvidenceKeys(ctx *types.BusContext, expected []types.TypedRelationCandidate) map[string]bool {
	expectedByKey := map[string][]types.TypedRelationCandidate{}
	for _, item := range expected {
		if !item.CoverageGateEligible() {
			continue
		}
		key := aggregateMemberKey(item.Member.Name)
		if key == "" {
			continue
		}
		expectedByKey[key] = append(expectedByKey[key], item)
	}
	if len(expectedByKey) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, ev := range ctx.Mutable.EmittedEvidence() {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		source := canonicalRelationSourcePath(ev.Source)
		if source == "" {
			continue
		}
		for _, raw := range aggregateEvidenceMemberLabels(ev) {
			key := aggregateMemberKey(raw)
			expectedItems, ok := expectedByKey[key]
			if !ok {
				continue
			}
			for _, expectedItem := range expectedItems {
				if !relationCandidateGroundedInFile(expectedItem, source) {
					continue
				}
				if candidateKey := relationCandidateEvidenceKey(expectedItem); candidateKey != "" {
					out[candidateKey] = true
				}
			}
		}
	}
	return out
}

func relationCandidateGroundedInFile(candidate types.TypedRelationCandidate, source string) bool {
	source = canonicalRelationSourcePath(source)
	if source == "" {
		return false
	}
	for _, file := range relationCandidateGroundingFiles(candidate) {
		if source == canonicalRelationSourcePath(file) {
			return true
		}
	}
	return false
}

func relationCandidateGroundingFiles(candidate types.TypedRelationCandidate) []string {
	var out []string
	add := func(file string) {
		file = canonicalRelationSourcePath(file)
		if file == "" {
			return
		}
		for _, existing := range out {
			if existing == file {
				return
			}
		}
		out = append(out, file)
	}
	switch candidate.Relation {
	case types.TypedRelationImports, types.TypedRelationExports:
		add(candidate.SourceFile)
		add(candidate.Member.File)
	default:
		add(candidate.Member.File)
	}
	return out
}

func relationSourceInRequestedScope(source string, rm types.RequestModel) bool {
	scope := types.SourceScopeProduction
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope != "" {
		scope = rm.SourceScopeProfile.RequestedScope
	}
	return types.SourceScopeAllowsPathRole(scope, types.ClassifySourcePathRole(source))
}

func canonicalRelationSourcePath(raw string) string {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`)
	raw = strings.TrimPrefix(raw, "./")
	return strings.TrimSpace(raw)
}

func relationCandidateEvidenceKey(candidate types.TypedRelationCandidate) string {
	memberKey := aggregateMemberKey(candidate.Member.Name)
	if memberKey == "" {
		return ""
	}
	file := canonicalRelationSourcePath(candidate.Member.File)
	return string(candidate.Relation) + "|" + memberKey + "|" + file
}

func relationMemberSetGapSummary(gaps []relationCoverageGap) string {
	if len(gaps) == 0 {
		return ""
	}
	parts := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		prefix := strings.TrimSpace(string(gap.Relation))
		if gap.SourceName != "" {
			if prefix != "" {
				prefix += " "
			}
			prefix += gap.SourceName + " -> "
		} else if prefix != "" {
			prefix += " "
		}
		if gap.File != "" && gap.Line > 0 {
			parts = append(parts, fmt.Sprintf("%s%s @ %s:%d", prefix, gap.Name, gap.File, gap.Line))
			continue
		}
		if gap.File != "" {
			parts = append(parts, fmt.Sprintf("%s%s @ %s", prefix, gap.Name, gap.File))
			continue
		}
		parts = append(parts, prefix+gap.Name)
	}
	return strings.Join(parts, "; ")
}

func exhaustiveEnumerationMemberSetDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if exactTypedRelationProjectionActive(ctx, aggregateFacts) {
		// The preceding relation handoff gate owns this exact member slate.
		// Running the generic/source-inventory universe gate as a second owner
		// would re-expand it to every mechanically observed type.
		return ""
	}
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		return ""
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil || view.Family != types.QFEnumeration || !view.NeedsEnumerationSlate() {
		return ""
	}
	ok, invalid := exhaustiveEnumerationMemberSetUsable(ctx, aggregateFacts)
	countGaps := exhaustiveEnumerationPrincipalGroupedCountGaps(ctx, aggregateFacts)
	sourceInventoryAuthority := BuildSourceInventoryAnswerPreEmitAuthority(ctx, aggregateFacts)
	universeGap := sourceInventoryAuthority.CandidateUniverseGap
	duplicateLocationGap := sourceInventoryAuthority.DuplicateLocationGap
	surfaceFamilyGap := sourceInventoryAuthority.SurfaceFamilyGap
	originSpecificOnlyMemberSet := exhaustiveEnumerationHasOriginSpecificOnlyMemberSet(ctx, aggregateFacts)
	if ok && len(countGaps) == 0 && !universeGap.Blocking && !duplicateLocationGap.Blocking && !surfaceFamilyGap.Blocking {
		return ""
	}
	if strings.TrimSpace(invalid) == "" && len(countGaps) == 0 && !universeGap.Blocking && !duplicateLocationGap.Blocking && !surfaceFamilyGap.Blocking {
		if syms, claim := ctx.Mutable.EmittedAnswerSymbols(); len(syms) > 0 && claim == types.CompletenessComplete {
			return ""
		}
	}
	if closure != nil {
		repairKind := types.RepairEmitEvidence
		repairOrigin := "pre_complete.exhaustive_member_set"
		repairRationale := "exhaustive enumeration needs a typed member_set handoff or a completed answer-symbol slate so finalization does not reconstruct the list from prose"
		files := completionMaterializationReadFiles(closure)
		keywords := dedupStringsPreserveOrder(append(append([]string{}, rm.AnalyzerHints.ExactTargets...), append(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities...)...))
		subject := ""
		if types.SourceInventoryPrincipalNavigationActive(rm) {
			repairKind = types.RepairStructuredHandoff
			repairOrigin = types.RepairOriginCompletionFormPrefix + "source_inventory_member_set"
			repairRationale = "source-inventory enumeration needs a corrected aggregate_facts.member_set handoff using existing source-inventory row locations; do not reopen broad evidence collection for completion-form debt"
			files = nil
			keywords = nil
			subject = "Repair aggregate_facts.member_set from the current typed source-inventory row set. Use member-specific support_refs copied from row locations already visible in this turn."
		} else if originSpecificOnlyMemberSet {
			repairKind = types.RepairStructuredHandoff
			repairOrigin = "pre_complete.exhaustive_member_set.origin_specific"
			repairRationale = "origin-specific external observations need a corrected aggregate_facts.member_set handoff with principal_answer role, exact members, and structured origin/provenance; do not repair this by emitting current-source evidence for the external artifact"
			files = nil
			keywords = nil
			subject = "Repair aggregate_facts.member_set for the origin-specific external observation lane."
		} else if checklistFiles := sourceInventoryMemberSetRepairFiles(ctx, 12); len(checklistFiles) > 0 {
			files = dedupStringsPreserveOrder(append(files, checklistFiles...))
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      repairKind,
			Files:     files,
			Keywords:  keywords,
			Subject:   subject,
			Rationale: repairRationale,
			Origin:    repairOrigin,
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — exhaustive member-set handoff is missing.\n\n")
	b.WriteString("The current question requires an exhaustive enumeration. Do not close with the complete list only in tool-output memory, thinking text, or the closure reason.\n\n")
	if strings.TrimSpace(invalid) != "" {
		if originSpecificOnlyMemberSet {
			fmt.Fprintf(&b, "An origin-specific external-observation `member_set` was present but is not structurally usable as principal-member handoff data: %s\n\n", invalid)
		} else {
			fmt.Fprintf(&b, "A `member_set` was present but is not usable as citable principal-member data: %s\n\n", invalid)
		}
	}
	if len(countGaps) > 0 {
		fmt.Fprintf(&b, "Some principal grouped/bucket count facts declare answer categories without handing off their members: %s. Under an exhaustive enumeration request, every positive principal grouped/bucket count must either carry its exact `members[]` or be represented by a matching principal `member_set`.\n\n", strings.Join(countGaps, "; "))
	}
	if universeGap.Blocking {
		fmt.Fprintf(&b, "An exact candidate universe observed through structured navigation is not covered or excluded by your current principal `member_set`: %s. This does not mean every candidate is automatically part of the final answer, but a complete close must either include verified principal members, explicitly list excluded candidates in `excluded`, or emit a matching `excluded_count` disclosure for candidates you intentionally ruled out.\n\n", universeGap.Summary(16))
	}
	if duplicateLocationGap.Blocking {
		fmt.Fprintf(&b, "A typed source-inventory row surface appears at multiple source locations, but the current principal `member_set` covers only part of that observed duplicate-location family: %s. A complete close must include each verified location for that selected member surface or explicitly exclude the non-answer locations.\n\n", duplicateLocationGap.Summary(16))
	}
	if surfaceFamilyGap.Blocking {
		fmt.Fprintf(&b, "A typed source-inventory surface family has multiple observed principal source rows, but the current principal `member_set` covers only part of that selected family: %s. A complete close must include each verified row for the selected surface family or explicitly exclude the non-answer locations.\n\n", surfaceFamilyGap.Summary(16))
	}
	if checklist := sourceInventoryMemberSetRepairChecklist(ctx, 16); checklist != "" {
		b.WriteString(checklist)
		b.WriteString("\n\n")
	}
	if types.SourceInventoryPrincipalNavigationActive(rm) {
		b.WriteString("Repair `aggregate_facts` with kind=`member_set`, role=`principal_answer`, value equal to the exact selected source-inventory member count, and `members` containing only the requested principal source rows. Use the source-inventory row locations already visible in this turn as member-specific `support_refs` (`Member @ path/file.ext:123` or positional `path/file.ext:123`). Do not call tools that are not present in the current turn, and do not reopen broad exploration solely to satisfy completion-form debt. If a candidate cannot be grounded from the current typed source-inventory row set or already-read evidence, omit it from the principal set or disclose it as a caveat/exclusion rather than inventing support. Then re-call `emit_investigation_complete`.")
	} else if originSpecificOnlyMemberSet {
		b.WriteString("Either emit the completed `emit_answer_symbol` slate, or repair `aggregate_facts` with kind=`member_set`, role=`principal_answer`, value equal to the exact member count, and `members` containing every principal answer member copied from the origin-specific tool/resource results. Preserve structured origin/provenance dimensions such as origin=`runtime_artifact`, artifact_id/artifact_kind, payload_ref/row_set_ref, or the producer token that identifies the external observation lane. Do not call `emit_evidence` for external trace/log/document/resource rows, and do not invent repo file:line `support_refs` for artifact-local members. If the external result has artifact-local line/row/time coordinates, carry them in dimensions/supporting aggregate facts or member notes rather than converting them into current-source citations. Then re-call `emit_investigation_complete`.")
	} else {
		b.WriteString("Either emit the completed `emit_answer_symbol` slate, or include `aggregate_facts` with kind=`member_set`, value equal to the exact member count, and `members` containing every principal answer member copied from your verified search/read/command results. For a verified empty set, use value=\"0\" and members=[] together with typed zero-result support such as `negative_search` or `negative_observation`; do not use `scalar_value` as the principal set carrier. For code symbols, paths, routes, config keys, macros, spans, and source-location members, each non-empty member must be backed by typed evidence already emitted through `emit_evidence`, or by member-specific `support_refs` such as `Member @ path/file.ext:123` that point to the same grounded evidence line. Exclude related-context/helper candidates from the principal member_set. If your broad candidate search count is different from the verified member_set count, also emit companion total_count/excluded_count aggregate facts with dimensions that name the candidate stage and exclusion basis. Then re-call `emit_investigation_complete`.")
	}
	return b.String()
}

func exhaustiveEnumerationHasOriginSpecificOnlyMemberSet(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if len(facts) == 0 {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet && !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		origins := types.AnswerAggregateFactEvidenceOrigins(fact, rm)
		if types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins) {
			return true
		}
	}
	return false
}

// sourceInventoryClassUniverseAbsenceDowngrade is the LOOP-gate twin of the
// finalize-stage validateSourceInventoryExactAbsenceBound: both consume the
// same SourceInventoryAnswerPreEmitAuthority view so source-inventory absence
// closure, pre-complete repair, and final contract validation cannot diverge.
// This catches a still-open typed source-class universe before the explorer
// spends its whole budget. It is subtype-scoped by the predicate (fires only
// when the request has an active source_inventory_profile with positive source
// classes that are not complete-zero for the requested roles — so
// symbol-existence / behaviour-verdict absence is inert). It reads typed inputs
// only, never the model's absence_justification prose.
//
// RNE-C61: it declares repo_map as a required tool (RepairDirective.Tools) so the
// restricted / no-emit completion surface EXPOSES repo_map and the demand is
// satisfiable — the production-only lens that satisfied the lens-execution gate
// can leave corpus / fixture / thirdparty classes uncovered, which this catches.
func sourceInventoryClassUniverseAbsenceDowngrade(ctx *types.BusContext, resultKind string, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(resultKind), "absence") {
		return ""
	}
	authority := BuildSourceInventoryAnswerPreEmitAuthority(ctx, aggregateFacts)
	if !authority.ExactAbsenceBlocking {
		return ""
	}
	ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:      types.RepairStructuredHandoff,
		Tools:     []string{"repo_map"},
		Subject:   "Prove the requested source-class universe with a complete `repo_map(view=\"source_inventory\")` pass over the principal roles (including any repo-owned corpus / fixture / thirdparty source files), or emit unknown/caveated, before declaring exact absence.",
		Rationale: "A source-family exact absence was declared while the typed source-class universe is still open (" + authority.ExactAbsenceSummary + "). A production-source no-hit does not bound corpus / fixture / thirdparty source classes; run the bounded source_inventory lens over the requested roles, then hand off a complete zero member_set or an honest bounded absence.",
		Origin:    "pre_complete.source_class_universe",
		Stage:     string(types.StageExplore),
	})
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — source-family absence declared while the typed source-class universe is still open.\n\n")
	b.WriteString(authority.ExactAbsenceSummary + "\n\n")
	b.WriteString("Run `repo_map` with `view=\"source_inventory\"` over the requested principal roles to prove the exact source-class universe (including any repo-owned corpus / fixture / thirdparty source files), then re-emit with a complete zero `member_set` or a bounded absence. A production-source no-hit does not by itself prove cross-class absence.\n")
	return b.String()
}

func sourceInventoryResolvedCompletionDowngrade(ctx *types.BusContext, resultKind string, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(resultKind), "resolved") {
		return ""
	}
	if exactTypedRelationProjectionActive(ctx, aggregateFacts) {
		return ""
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if profile == nil || !types.SourceInventoryPrincipalAuthorityActive(ctx.AnalysisIR.RequestModel) {
		return ""
	}
	if gap := sourceInventoryResolvedCompletionPreciseCoverageGap(ctx, aggregateFacts); gap.Blocking {
		return sourceInventoryResolvedCompletionPreciseCoverageDowngrade(ctx, gap)
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	authority := sourceInventoryCompletionAuthorityForContext(ctx, observation, aggregateFacts)
	if !authority.Blocking {
		return ""
	}
	if sourceInventoryResolvedCompletionDebtIsAdvisory(ctx, authority, observation) {
		return ""
	}
	roles := sourceInventoryCompletionRepairRoleLabels(authority, observation)
	scopes := sourceInventoryCompletionRepairScopes(authority)
	repoMapPath, repoMapScopes := sourceInventoryLensExecutionRepoMapCallShape(scopes)
	nextCursor := sourceInventoryCompletionRepairCursor(authority)
	subject := fmt.Sprintf("Continue the incomplete source-inventory lens with path=%q, view=\"source_inventory\", roles=[%s], scopes=[%s], include_counts=true, and include_attributes=false.",
		repoMapPath, strings.Join(roles, ", "), sourceInventoryLensExecutionQuotedList(repoMapScopes))
	if query := sourceInventoryResolvedCompletionRepairQuery(ctx); query != "" {
		subject += " Use query=" + strconv.Quote(query) + " to target the typed requested construct/language surface."
	}
	if nextCursor != "" {
		subject += " Use cursor=" + strconv.Quote(nextCursor) + " when paging the same lens."
	}
	rationale := "A resolved source-inventory closure was attempted while the typed inventory observation is incomplete or budget-truncated. Page or narrow the source_inventory lens by the missing typed family, then hand off a complete principal member_set or an explicit bounded-incomplete scope."
	ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:                               types.RepairStructuredHandoff,
		Tools:                              []string{"repo_map"},
		Subject:                            subject,
		Rationale:                          rationale,
		Origin:                             "pre_complete.source_inventory_completion",
		Stage:                              string(types.StageExplore),
		SourceInventoryCompletionAuthority: authority,
	})
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — source-inventory result is still incomplete for a resolved exhaustive answer.\n\n")
	b.WriteString(sourceInventoryResolvedCompletionSummary(observation, profile) + "\n\n")
	b.WriteString("Continue with the typed `repo_map(view=\"source_inventory\")` follow-up route before closing as resolved. Cursor is valid only for the same lens identity; a missing source-class follow-up starts from the first page of its narrower typed family/scope. Do not replace an incomplete repo-wide inventory with a smaller fixture/support subtree unless the final handoff explicitly declares that bounded scope.\n")
	if len(repoMapScopes) > 0 || nextCursor != "" {
		b.WriteString("\nSuggested typed follow-up to target the typed requested construct/language surface: ")
		b.WriteString(fmt.Sprintf("repo_map path=%q view=\"source_inventory\" roles=[%s]", repoMapPath, strings.Join(roles, ", ")))
		if len(repoMapScopes) > 0 {
			b.WriteString(" scopes=[" + sourceInventoryLensExecutionQuotedList(repoMapScopes) + "]")
		}
		if query := sourceInventoryResolvedCompletionRepairQuery(ctx); query != "" {
			b.WriteString(" query=" + strconv.Quote(query))
		}
		if nextCursor != "" {
			b.WriteString(" cursor=" + strconv.Quote(nextCursor))
		}
		b.WriteString(".\n")
	}
	return b.String()
}

func sourceInventoryResolvedCompletionPreciseCoverageGap(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) SourceInventoryCandidateUniverseGap {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	if SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, aggregateFacts) {
		return SourceInventoryCandidateUniverseGap{}
	}
	best := BuildSourceInventoryAnswerPreEmitAuthority(ctx, aggregateFacts).BestUniverseGap
	if !best.Blocking {
		return SourceInventoryCandidateUniverseGap{}
	}
	return best
}

func sourceInventoryResolvedCompletionPreciseCoverageDowngrade(ctx *types.BusContext, gap SourceInventoryCandidateUniverseGap) string {
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
			Kind:      types.RepairStructuredHandoff,
			Tools:     []string{"emit_investigation_complete"},
			Subject:   sourceInventoryPreciseCoverageRepairSubject(gap),
			Rationale: "The typed source_inventory row-set already observed a precise principal row family that the current aggregate_facts.member_set only partially covered. Repair the structured handoff or explicitly exclude the missing rows; do not reopen broad repository exploration.",
			Origin:    "pre_complete.source_inventory_precise_coverage",
			Stage:     string(types.StageExplore),
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — source-inventory principal row-set is partially covered.\n\n")
	b.WriteString("This is an exact typed row-set mismatch, not broad pagination debt. Repair `aggregate_facts.member_set` so it includes every observed row in the same source-inventory family, or add an explicit bounded exclusion for rows that should not be counted.\n\n")
	b.WriteString("Missing precise row(s): " + gap.Summary(SourceInventoryCandidateUniverseSummaryLimit) + "\n")
	if rows := sourceInventoryPreciseCoverageMissingRows(gap); rows != "" {
		b.WriteString("Missing row locations: " + rows + "\n")
	}
	return b.String()
}

func sourceInventoryPreciseCoverageRepairSubject(gap SourceInventoryCandidateUniverseGap) string {
	summary := gap.Summary(SourceInventoryCandidateUniverseSummaryLimit)
	if summary == "" {
		return "Repair aggregate_facts.member_set for the precise source_inventory row-set coverage gap."
	}
	return "Repair aggregate_facts.member_set for precise source_inventory coverage: " + summary
}

func sourceInventoryPreciseCoverageMissingRows(gap SourceInventoryCandidateUniverseGap) string {
	if len(gap.Missing) == 0 {
		return ""
	}
	rows := make([]string, 0, min(len(gap.Missing), SourceInventoryCandidateUniverseSummaryLimit))
	for _, member := range gap.Missing {
		label := strings.TrimSpace(member.Name)
		if label == "" {
			label = strings.TrimSpace(member.Key)
		}
		if label == "" {
			label = "member"
		}
		file := strings.TrimSpace(strings.ReplaceAll(member.File, `\`, `/`))
		if file == "" {
			if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(member.SupportRef); ok {
				file = strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`))
				if member.Line <= 0 {
					member.Line = loc.LineStart
				}
			}
		}
		if file != "" && member.Line > 0 {
			rows = append(rows, fmt.Sprintf("%s @ %s:%d", label, file, member.Line))
		} else if file != "" {
			rows = append(rows, fmt.Sprintf("%s @ %s", label, file))
		} else {
			rows = append(rows, label)
		}
		if len(rows) >= SourceInventoryCandidateUniverseSummaryLimit {
			break
		}
	}
	if len(gap.Missing) > len(rows) {
		rows = append(rows, fmt.Sprintf("+%d more", len(gap.Missing)-len(rows)))
	}
	return strings.Join(rows, "; ")
}

func sourceInventoryResolvedCompletionDebtIsAdvisory(ctx *types.BusContext, authority types.SourceInventoryCompletionAuthority, observation types.SourceInventoryObservation) bool {
	authority = types.NormalizeSourceInventoryCompletionAuthority(authority)
	if !types.SourceInventoryCompletionAuthorityCanOnlyCaveat(authority) {
		return false
	}
	// The source-inventory completion authority observes navigation/tool debt:
	// pagination, candidate-budget truncation, incomplete broad roles, and
	// follow-up route suggestions. Those signals prove that the helper lens did
	// not exhaust a mechanical candidate space; they do not precisely prove that
	// the user's principal question is unanswered. Exact member-set omissions
	// are enforced by exhaustiveEnumerationMemberSetDowngrade and
	// SourceInventoryCandidateUniverseCoverageGap, where the system owns a
	// concrete candidate universe. Here we preserve a typed caveat and let the
	// model-authored completion continue instead of turning helper debt into a
	// hard loop.
	if ctx != nil && ctx.Mutable != nil {
		var parts []string
		if authority.ReasonCode != "" {
			parts = append(parts, "reason="+authority.ReasonCode)
		}
		if authority.CandidateBudgetTruncated {
			parts = append(parts, "candidate_budget_truncated")
		}
		if authority.PageIncomplete {
			parts = append(parts, "page_incomplete")
		}
		if authority.NextCursor != "" {
			parts = append(parts, "next_cursor="+authority.NextCursor)
		}
		if len(authority.Scopes) > 0 {
			parts = append(parts, "scopes="+strings.Join(authority.Scopes, ","))
		}
		if len(parts) == 0 {
			parts = append(parts, "source_inventory_navigation_debt")
		}
		ctx.Mutable.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
			Lane:       types.DowngradeLaneSourceInventoryCompletion,
			ReasonCode: "source_inventory_navigation_debt_advisory",
			Reason:     "source_inventory navigation debt is non-precise for resolved completion; " + strings.Join(parts, "; "),
		})
	}
	logging.Info("[emit_investigation_complete] treating source-inventory completion debt as advisory: reason=%s truncated=%t page_incomplete=%t scopes=%s",
		authority.ReasonCode, authority.CandidateBudgetTruncated || (observation.Execution != nil && observation.Execution.CandidateBudgetTruncated), authority.PageIncomplete, strings.Join(authority.Scopes, ","))
	return true
}

func sourceInventoryCompletionAuthorityForContext(ctx *types.BusContext, observation types.SourceInventoryObservation, aggregateFacts []types.AnswerAggregateFact) types.SourceInventoryCompletionAuthority {
	return sourceInventoryAuthoritySnapshotForCompletion(ctx, observation, aggregateFacts).CompletionAuthority
}

func sourceInventoryAuthoritySnapshotForCompletion(ctx *types.BusContext, observation types.SourceInventoryObservation, aggregateFacts []types.AnswerAggregateFact) types.SourceInventoryAuthoritySnapshot {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.SourceInventoryAuthoritySnapshot{}
	}
	acceptedExact := SourceInventoryAcceptedClosureCoversExactUniverse(ctx, aggregateFacts)
	acceptedRequested := SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, aggregateFacts)
	snapshot := types.BuildSourceInventoryAuthoritySnapshot(types.SourceInventoryAuthoritySnapshotInput{
		Observation:               observation,
		RequestModel:              ctx.AnalysisIR.RequestModel,
		ExistingAggregateFacts:    aggregateFacts,
		AcceptedExactUniverse:     acceptedExact,
		AcceptedRequestedUniverse: acceptedRequested,
		RequiredFiles:             ctx.AnalysisIR.EvidencePlan.RequiredFiles,
	})
	authority := snapshot.CompletionAuthority
	if authority.Blocking && !acceptedRequested {
		if debt := sourceInventoryRequestedUniverseFollowupDebt(ctx, observation, ctx.AnalysisIR.RequestModel, aggregateFacts); debt.IsActive() {
			authority.FollowupDebt = debt
			authority.ReasonCode = types.SourceInventoryCompletionReasonFollowupDebt
		}
	}
	if authority.FollowupDebt.IsActive() {
		debt := authority.FollowupDebt
		debt.Query.Query = sourceInventoryResolvedCompletionRepairQuery(ctx)
		authority.FollowupDebt = types.NormalizeSourceInventoryFollowupDebt(debt)
	}
	snapshot.CompletionAuthority = types.NormalizeSourceInventoryCompletionAuthority(authority)
	if snapshot.CompletionAuthority.FollowupDebt.IsActive() {
		snapshot.FollowupDebt = snapshot.CompletionAuthority.FollowupDebt
	}
	return types.NormalizeSourceInventoryAuthoritySnapshot(snapshot)
}

func sourceInventoryResolvedCompletionRequiresRepoWideAuthority(ctx *types.BusContext, observation types.SourceInventoryObservation) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !types.SourceInventoryRequiresRepoWideLens(ctx.AnalysisIR.RequestModel) {
		return false
	}
	return sourceInventoryObservationResolvedCompletionIncomplete(observation)
}

func sourceInventoryResolvedCompletionRepairScopes(ctx *types.BusContext, observation types.SourceInventoryObservation) []string {
	if ctx != nil && ctx.AnalysisIR != nil {
		if scopes := types.SourceInventoryRequestedPathScopes(ctx.AnalysisIR.RequestModel); len(scopes) > 0 {
			return scopes
		}
	}
	if sourceInventoryResolvedCompletionRequiresRepoWideAuthority(ctx, observation) {
		return []string{"."}
	}
	return append([]string(nil), observation.Scopes...)
}

func sourceInventoryResolvedCompletionRepairQuery(ctx *types.BusContext) string {
	return strings.TrimSpace(sourceInventoryAdvisorySnapshotQuery(ctx))
}

func sourceInventoryCompletionRepairCursor(authority types.SourceInventoryCompletionAuthority) string {
	debt := types.NormalizeSourceInventoryFollowupDebt(authority.FollowupDebt)
	if !debt.IsActive() || debt.ReasonCode != types.SourceInventoryFollowupDebtPagination {
		return ""
	}
	return strings.TrimSpace(debt.Query.Cursor)
}

func sourceInventoryObservationResolvedCompletionIncomplete(observation types.SourceInventoryObservation) bool {
	if !observation.Complete {
		return true
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		return true
	}
	if observation.Page != nil && (!observation.Page.Complete || strings.TrimSpace(observation.Page.NextCursor) != "") {
		return true
	}
	for _, set := range observation.Sets {
		if !set.Complete && (set.Count > 0 || set.Total > 0 || len(set.Members) > 0) {
			return true
		}
	}
	return false
}

func sourceInventoryObservationRolesForCompletion(observation types.SourceInventoryObservation) []types.AnswerCandidateRole {
	var roles []types.AnswerCandidateRole
	seen := map[types.AnswerCandidateRole]bool{}
	for _, set := range observation.Sets {
		if set.Role == types.AnswerCandidateRoleUnknown || seen[set.Role] {
			continue
		}
		seen[set.Role] = true
		roles = append(roles, set.Role)
	}
	return roles
}

func sourceInventoryObservationNextCursor(observation types.SourceInventoryObservation) string {
	if observation.Page == nil {
		return ""
	}
	return strings.TrimSpace(observation.Page.NextCursor)
}

func sourceInventoryResolvedCompletionSummary(observation types.SourceInventoryObservation, profile *types.SourceInventoryProfile) string {
	var parts []string
	if summary := types.SourceInventorySourceClassSummary(observation.SourceClasses, 8); summary != "" {
		parts = append(parts, "source_classes="+summary)
	}
	if profile != nil {
		if roles := sourceInventoryLensExecutionRoleLabels(profile.PrincipalTargetRoles()); len(roles) > 0 {
			parts = append(parts, "principal_roles="+strings.Join(roles, ","))
		}
	}
	if !observation.Complete {
		parts = append(parts, "observation_incomplete")
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		parts = append(parts, "candidate_budget_truncated")
	}
	if observation.Page != nil {
		if !observation.Page.Complete {
			parts = append(parts, "page_incomplete")
		}
		if observation.Page.NextCursor != "" {
			parts = append(parts, "next_cursor="+observation.Page.NextCursor)
		}
	}
	for _, set := range observation.Sets {
		if !set.Complete {
			parts = append(parts, fmt.Sprintf("role_%s_incomplete_count=%d", types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Count))
		}
	}
	if len(parts) == 0 {
		return "source_inventory=incomplete"
	}
	return strings.Join(parts, "; ")
}

func sourceInventoryLensExecutionDowngrade(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) string {
	if exactTypedRelationProjectionActive(ctx, aggregateFacts) {
		return ""
	}
	if SourceInventoryAcceptedClosureCoversExactUniverse(ctx, aggregateFacts) ||
		sourceInventoryLensExecutionPathHandoffComplete(ctx, aggregateFacts) {
		return ""
	}
	gap := SourceInventoryLensExecutionGapForContext(ctx)
	if !gap.Blocking {
		return ""
	}
	roles := sourceInventoryLensExecutionRoleLabels(gap.Roles)
	scopes := append([]string(nil), gap.Scopes...)
	if len(scopes) == 0 {
		scopes = []string{"."}
	}
	repoMapPath, repoMapScopes := sourceInventoryLensExecutionRepoMapCallShape(scopes)
	subject := fmt.Sprintf("Run `repo_map` with path=%q, `view=\"source_inventory\"`, roles=[%s], scopes=[%s], include_counts=true, and include_attributes=false for the broad pass.",
		repoMapPath, strings.Join(roles, ", "), sourceInventoryLensExecutionQuotedList(repoMapScopes))
	rationale := "The active `source_inventory_profile` makes this a bounded source-inventory lane. Advisory graph rows and direct `list_files` universes are navigation context only; before closing, execute the typed source_inventory lens, then hand off the model-authored principal set through aggregate_facts.member_set or an honest absence boundary. " + types.SourceInventoryMechanicalFactBoundary
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
			Kind:      types.RepairStructuredHandoff,
			Tools:     []string{"repo_map"},
			Subject:   subject,
			Rationale: rationale,
			Origin:    "pre_complete.source_inventory_lens_execution",
			Stage:     string(types.StageExplore),
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — source-inventory lens has not run.\n\n")
	b.WriteString("The current question has an active typed `source_inventory_profile`, so closure needs the executable repo-map source inventory lens before the final member/absence handoff. ")
	b.WriteString("Advisory graph rows and `list_files` direct-child universes are useful navigation context, but they do not replace the bounded `repo_map(view=\"source_inventory\")` pass for this inventory lane.\n\n")
	fmt.Fprintf(&b, "- required roles: `%s`\n", strings.Join(roles, "`, `"))
	fmt.Fprintf(&b, "- suggested repo_map path: `%s`\n", repoMapPath)
	fmt.Fprintf(&b, "- suggested scopes: `%s`\n", strings.Join(repoMapScopes, "`, `"))
	if gap.HasAdvisory {
		b.WriteString("- current state: source-inventory advisory is present\n")
	}
	if gap.HasListFiles {
		b.WriteString("- current state: list_files direct universe is present, but source_inventory lens execution is still missing\n")
	}
	b.WriteString("\nNext action: ")
	b.WriteString(subject)
	b.WriteString(" Then verify/cite selected source rows and retry `emit_investigation_complete` with the completed model-authored `aggregate_facts.member_set`, or `result_kind=\"absence\"` plus a typed absence justification if the lens confirms there are no matching members.")
	return strings.TrimSpace(b.String())
}

func sourceInventoryLensExecutionRepoMapCallShape(scopes []string) (string, []string) {
	cleaned := sourceInventoryLensExecutionCleanScopes(scopes)
	if len(cleaned) == 0 {
		return ".", []string{"."}
	}
	if len(cleaned) == 1 && sourceInventoryLensExecutionScopeLooksLikeFile(cleaned[0]) {
		dir := path.Dir(cleaned[0])
		base := path.Base(cleaned[0])
		if dir == "" || dir == "." || dir == "/" {
			return ".", []string{base}
		}
		return dir, []string{base}
	}
	return ".", cleaned
}

func sourceInventoryLensExecutionCleanScopes(scopes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		scope := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if scope == "" {
			scope = "."
		}
		if seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func sourceInventoryLensExecutionScopeLooksLikeFile(scope string) bool {
	scope = strings.Trim(strings.ReplaceAll(strings.TrimSpace(scope), `\`, `/`), "/")
	if scope == "" || scope == "." {
		return false
	}
	base := path.Base(scope)
	return base != "." && strings.TrimSpace(path.Ext(base)) != ""
}

func sourceInventoryLensExecutionPrincipalHandoffComplete(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if len(facts) == 0 {
		return false
	}
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	for _, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if !types.AnswerAggregateFactRoleForRequest(fact, rm).IsPrincipal() {
			continue
		}
		return true
	}
	return false
}

func sourceInventoryLensExecutionPathHandoffComplete(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if !sourceInventoryLensExecutionPrincipalHandoffComplete(ctx, facts) {
		return false
	}
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.RequestModel.SourceInventoryProfile == nil {
		return false
	}
	roles := ctx.AnalysisIR.RequestModel.SourceInventoryProfile.PrincipalTargetRoles()
	if len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		switch role {
		case types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigFile:
			continue
		default:
			return false
		}
	}
	return true
}

func sourceInventoryLensExecutionRoleLabels(roles []types.AnswerCandidateRole) []string {
	out := make([]string, 0, len(roles))
	seen := map[string]bool{}
	for _, role := range roles {
		label := strings.TrimSpace(string(role))
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	if len(out) == 0 {
		return []string{"function", "method", "type", "file"}
	}
	return out
}

func sourceInventoryLensExecutionQuotedList(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, strconv.Quote(value))
	}
	if len(out) == 0 {
		return strconv.Quote(".")
	}
	return strings.Join(out, ", ")
}

func sourceInventoryMemberSetRepairChecklist(ctx *types.BusContext, maxRows int) string {
	if ctx == nil || ctx.Mutable == nil || maxRows <= 0 {
		return ""
	}
	observation := ctx.Mutable.SourceInventoryObservation()
	if !observation.IsActive() {
		return ""
	}
	support := buildAggregateMemberSupportIndex(ctx)
	var b strings.Builder
	b.WriteString("Active source-inventory verification checklist (advisory only): repo-lens rows below preserve mechanical count/list shape, but they are not final evidence, not a system-authored member_set, and do not decide the user's intended answer set. If, after your own verification, these rows exactly match the principal list you intend to close, re-emit them yourself in `aggregate_facts.member_set`; rows marked `candidate_needs_verification` should be verified/read or accompanied by valid member-specific support_refs before citing.\n")
	emitted := 0
	total := 0
	for _, set := range observation.Sets {
		total += len(set.Members)
	}
	for _, set := range observation.Sets {
		if len(set.Members) == 0 || emitted >= maxRows {
			continue
		}
		fmt.Fprintf(&b, "- role `%s` %s: count=%d len(members)=%d complete=%t\n",
			set.Role, types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Count, len(set.Members), set.Complete)
		for _, member := range set.Members {
			if emitted >= maxRows {
				break
			}
			line := sourceInventoryMemberSetRepairRow(member, support)
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "  - %s\n", line)
			emitted++
		}
	}
	if total > emitted {
		fmt.Fprintf(&b, "- showing %d of %d source-inventory rows; narrow or page `repo_map(view=\"source_inventory\")` for hidden rows instead of widening blindly.", emitted, total)
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryMemberSetRepairRow(member types.SourceInventoryObservationMember, support aggregateMemberSupportIndex) string {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return ""
	}
	state := "candidate_needs_verification"
	if sourceInventoryObservationMemberHasVerifiedSource(member, support) {
		state = "verified_source_window"
	}
	if member.CoverageState == types.SourceInventoryCoverageAmbiguous {
		state = "ambiguous_candidate"
	}
	parts := []string{fmt.Sprintf("%s `%s`", state, name)}
	if member.File != "" && member.Line > 0 {
		parts = append(parts, fmt.Sprintf("@ %s:%d", member.File, member.Line))
	} else if member.File != "" {
		parts = append(parts, "@ "+member.File)
	}
	if member.Language != "" {
		parts = append(parts, "language="+member.Language)
	}
	if member.Role != "" {
		parts = append(parts, "role="+string(member.Role))
	}
	if member.SupportRef != "" {
		parts = append(parts, "support_ref=`"+member.SupportRef+"`")
	}
	if attrs := sourceInventoryMemberSetRepairAttributes(member.Attributes, support); attrs != "" {
		parts = append(parts, attrs)
	}
	return strings.Join(parts, " — ")
}

func sourceInventoryMemberSetRepairAttributes(attrs []types.SourceInventoryObservationAttribute, support aggregateMemberSupportIndex) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 3
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		state := "candidate"
		if sourceInventoryObservationAttributeHasVerifiedSource(attr, support) {
			state = "verified"
		}
		if attr.CoverageState == types.SourceInventoryCoverageAmbiguous {
			state = "ambiguous"
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "attribute"
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, attr.Line)
		}
		item := fmt.Sprintf("%s %s `%s`", state, role, name)
		if loc != "" {
			item += " @ " + loc
		}
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return ""
	}
	if len(attrs) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(attrs)-len(parts)))
	}
	return "attributes: " + strings.Join(parts, "; ")
}

func sourceInventoryObservationMemberHasVerifiedSource(member types.SourceInventoryObservationMember, support aggregateMemberSupportIndex) bool {
	if sourceInventoryObservationLocationHasVerifiedSource(member.File, member.Line, support) {
		return true
	}
	if _, loc, ok := aggregateMemberSupportRefParts(member.SupportRef); ok && sourceInventoryObservationSupportLocationVerified(loc, support) {
		return true
	}
	return aggregateMemberCoveredByEvidenceLabels(aggregateSupportLabels(member.Name, []string{member.Key}), support.byLabel)
}

func sourceInventoryObservationAttributeHasVerifiedSource(attr types.SourceInventoryObservationAttribute, support aggregateMemberSupportIndex) bool {
	if sourceInventoryObservationLocationHasVerifiedSource(attr.File, attr.Line, support) {
		return true
	}
	if _, loc, ok := aggregateMemberSupportRefParts(attr.SupportRef); ok && sourceInventoryObservationSupportLocationVerified(loc, support) {
		return true
	}
	return aggregateMemberCoveredByEvidenceLabels(aggregateSupportLabels(attr.Name, []string{attr.Key}), support.byLabel)
}

func sourceInventoryObservationLocationHasVerifiedSource(file string, line int, support aggregateMemberSupportIndex) bool {
	loc := aggregateSupportLocationKey(file, line)
	if loc == "" {
		return false
	}
	return sourceInventoryObservationSupportLocationVerified(loc, support)
}

func sourceInventoryObservationSupportLocationVerified(loc string, support aggregateMemberSupportIndex) bool {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return false
	}
	return len(support.byLocation[loc]) > 0 || len(support.toolLinesByLocation[loc]) > 0
}

func sourceInventoryMemberSetRepairFiles(ctx *types.BusContext, maxFiles int) []string {
	if ctx == nil || ctx.Mutable == nil || maxFiles <= 0 {
		return nil
	}
	observation := ctx.Mutable.SourceInventoryObservation()
	if !observation.IsActive() {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	add := func(file string) {
		file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
		if file == "" || seen[file] || !types.IsCodeOrConfigPathExtension(filepath.Ext(file)) {
			return
		}
		seen[file] = true
		files = append(files, file)
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			if len(files) >= maxFiles {
				return files
			}
			add(member.File)
			for _, attr := range member.Attributes {
				if len(files) >= maxFiles {
					return files
				}
				add(attr.File)
			}
		}
	}
	return files
}

func aggregateFactsContainMemberSet(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		if types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			types.AnswerAggregateFactRoleForRequest(fact, nil).IsPrincipal() {
			return true
		}
	}
	return false
}

func exhaustiveEnumerationMemberSetUsable(ctx *types.BusContext, facts []types.AnswerAggregateFact) (bool, string) {
	if len(facts) == 0 {
		return false, ""
	}
	support := buildAggregateMemberSupportIndex(ctx)
	sawMemberSet := false
	var invalid []string
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	for factIdx, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		sawMemberSet = true
		if !types.AnswerAggregateFactRoleForRequest(fact, rm).IsPrincipal() {
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] role=%q is not principal_answer", factIdx, fact.Role))
			continue
		}
		if len(fact.Members) == 0 {
			if types.AnswerAggregateFactIsExactEmptyMemberSet(fact) &&
				exactEmptyMemberSetHasTypedNoHitSupport(fact, facts) {
				return true, ""
			}
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] is an empty member_set without typed zero-result support", factIdx))
			continue
		}
		if aggregateMemberSetCanRelyOnOriginSpecificProvenance(ctx, fact) {
			return true, ""
		}
		if aggregateMemberSetCanRelyOnSourceInventoryPrincipalRowSet(ctx, fact) {
			return true, ""
		}
		allUsable := true
		for memberIdx, member := range fact.Members {
			if aggregateMemberSetMemberUsableAt(fact, member, memberIdx, support) {
				continue
			}
			allUsable = false
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] member %q has no typed evidence / member-specific support_ref", factIdx, strings.TrimSpace(member)))
			if len(invalid) >= 12 {
				break
			}
		}
		if allUsable {
			return true, ""
		}
	}
	if !sawMemberSet {
		return false, ""
	}
	if len(invalid) == 0 {
		return false, ""
	}
	return false, strings.Join(invalid, "; ")
}

func aggregateMemberSetCanRelyOnSourceInventoryPrincipalRowSet(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil ||
		strings.TrimSpace(fact.Provenance) != types.SourceInventoryPrincipalRowSetAggregateProvenance ||
		!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
		len(fact.Members) == 0 ||
		len(fact.SupportRefs) != len(fact.Members) {
		return false
	}
	for _, candidate := range sourceInventoryPrincipalRowSetLandingFacts(ctx, nil) {
		if strings.TrimSpace(candidate.Provenance) != types.SourceInventoryPrincipalRowSetAggregateProvenance ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(candidate) ||
			len(candidate.Members) == 0 ||
			len(candidate.SupportRefs) != len(candidate.Members) {
			continue
		}
		if aggregateMemberSetSameSystemInventoryRows(candidate, fact) {
			return true
		}
	}
	return false
}

func aggregateMemberSetSameSystemInventoryRows(a, b types.AnswerAggregateFact) bool {
	if a.Kind != b.Kind ||
		types.AnswerAggregateFactRoleForRequest(a, nil) != types.AnswerAggregateFactRoleForRequest(b, nil) ||
		!aggregateStringSlicesEqual(a.Members, b.Members) ||
		!aggregateStringSlicesEqual(a.SupportRefs, b.SupportRefs) {
		return false
	}
	return strings.TrimSpace(a.Value) == strings.TrimSpace(b.Value)
}

func aggregateStringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func exactEmptyMemberSetHasTypedNoHitSupport(fact types.AnswerAggregateFact, facts []types.AnswerAggregateFact) bool {
	if !types.AnswerAggregateFactIsExactEmptyMemberSet(fact) {
		return false
	}
	if types.AnswerAggregateFactHasTypedExclusionExactEmptyAuthority(fact) && len(fact.Excluded) > 0 {
		return true
	}
	for _, candidate := range facts {
		switch candidate.Kind {
		case types.AnswerAggregateNegativeSearch, types.AnswerAggregateNegativeObservation:
		default:
			continue
		}
		if strings.TrimSpace(candidate.Value) == "0" {
			return true
		}
	}
	return false
}

func completionFactsContainPrincipalEmptyMemberSetWithTypedNoHitSupport(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if len(facts) == 0 {
		return false
	}
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	for _, fact := range facts {
		if !types.AnswerAggregateFactRoleForRequest(fact, rm).IsPrincipal() {
			continue
		}
		if exactEmptyMemberSetHasTypedNoHitSupport(fact, facts) {
			return true
		}
	}
	return false
}

func completionPathDiscoveryAbsenceProofReject(ctx *types.BusContext, resultKind string, facts []types.AnswerAggregateFact) string {
	if !completionNeedsPathDiscoveryAbsenceProof(ctx, resultKind, facts) {
		return ""
	}
	results := completionPathDiscoveryToolResults(ctx)
	if !completionToolResultsHavePathFilteredGrepNoMatch(results) {
		return ""
	}
	if sourceInventoryToolResultsHaveSourceInventoryLens(results) {
		return ""
	}
	hasRecursiveDiscovery := false
	hasAuxiliaryDiscovery := false
	for _, result := range results {
		if !completionToolResultIsRecursivePathDiscoveryList(result) {
			continue
		}
		hasRecursiveDiscovery = true
		if result.PathDiscovery != nil && result.PathDiscovery.IncludeAuxiliary {
			hasAuxiliaryDiscovery = true
		}
	}
	if !hasRecursiveDiscovery {
		return "emit_investigation_complete rejected: repo-wide file-family absence is not proven by content grep or a non-recursive directory listing. Run `list_files` with path=\".\", recursive=true, and include/file_type matching the searched file family, or run `repo_map` with view=\"source_inventory\" when this is a typed source-inventory lane; then retry with aggregate_facts based on that typed universe."
	}
	if completionRepoHasAuxiliaryInventoryDirs(ctx) && !hasAuxiliaryDiscovery {
		return "emit_investigation_complete rejected: the repository contains repo-owned auxiliary/corpus trees, so this file-family absence proof must include them explicitly. Re-run `list_files` with path=\".\", recursive=true, matching include/file_type, and include_auxiliary=true, or run `repo_map` with view=\"source_inventory\" if the typed source-inventory lens is authoritative for this lane."
	}
	return ""
}

func completionNeedsPathDiscoveryAbsenceProof(ctx *types.BusContext, resultKind string, facts []types.AnswerAggregateFact) bool {
	if !completionTypedInventoryLikeRequest(ctx) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(resultKind), "absence") {
		return true
	}
	for _, fact := range facts {
		switch fact.Kind {
		case types.AnswerAggregateNegativeSearch:
			if strings.TrimSpace(fact.Value) == "0" {
				return true
			}
		case types.AnswerAggregateMemberSet:
			if types.AnswerAggregateFactIsExactEmptyMemberSet(fact) {
				return true
			}
		}
	}
	return false
}

func completionTypedInventoryLikeRequest(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if types.IsTypedSourceEnumerationShape(rm) {
		return true
	}
	if rm.Predicates.HasPerMemberTable {
		return true
	}
	if rm.CompletenessObligation != nil && rm.CompletenessObligation.IsActive() &&
		rm.Intent == types.IntentEnumerate &&
		!rm.Predicates.IsScalarAnswer &&
		!rm.Predicates.IsCountQuestion &&
		!rm.Predicates.IsHistoryLookup &&
		!rm.Predicates.IsDiagnosticQuestion {
		return true
	}
	return false
}

func completionPathDiscoveryToolResults(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil {
		return nil
	}
	var out []types.ToolResult
	if ctx.Mutable != nil {
		out = append(out, ctx.Mutable.DispatchToolResults()...)
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			out = append(out, ta.ToolResults...)
		}
	}
	out = append(out, ctx.ToolResults...)
	return out
}

func completionToolResultsHavePathFilteredGrepNoMatch(results []types.ToolResult) bool {
	for _, result := range results {
		if !result.Success || types.CanonicalToolName(result.ToolName) != "grep" {
			continue
		}
		pd := result.PathDiscovery
		if pd == nil || pd.Kind != types.ToolPathDiscoveryKindGrep || !pd.NoMatches {
			continue
		}
		if strings.TrimSpace(pd.Include) == "" && strings.TrimSpace(pd.FileType) == "" {
			continue
		}
		return true
	}
	return false
}

func completionToolResultIsRecursivePathDiscoveryList(result types.ToolResult) bool {
	if !result.Success || types.CanonicalToolName(result.ToolName) != "list_files" {
		return false
	}
	pd := result.PathDiscovery
	if pd == nil || pd.Kind != types.ToolPathDiscoveryKindListFiles || !pd.Recursive {
		return false
	}
	return strings.TrimSpace(pd.Include) != "" || strings.TrimSpace(pd.FileType) != ""
}

func completionRepoHasAuxiliaryInventoryDirs(ctx *types.BusContext) bool {
	root := completionRepoRoot(ctx)
	if root == "" {
		return false
	}
	count := 0
	found := false
	_ = filepath.WalkDir(root, func(pathValue string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || found {
			return nil
		}
		count++
		if count > 5000 {
			return filepath.SkipDir
		}
		name := d.Name()
		if pathValue != root && listFilesAuxiliaryDirNameAllowed(name) {
			found = true
			return filepath.SkipDir
		}
		if pathValue == root {
			return nil
		}
		if IsWindowsReservedDevicePath(name) {
			return filepath.SkipDir
		}
		if listFilesAuxiliaryDirNameAllowed(name) {
			return nil
		}
		if IsExcludedDirName(name) {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func completionRepoRoot(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	if strings.TrimSpace(ctx.RepoRoot) != "" {
		return ctx.RepoRoot
	}
	if ctx.Mutable != nil {
		return ctx.Mutable.RepoRoot()
	}
	return ""
}

func aggregateMemberSetCanRelyOnOriginSpecificProvenance(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	rm := requestModelForAggregateSupport(ctx)
	origins := types.AnswerAggregateFactEvidenceOrigins(fact, rm)
	if !types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins) {
		return false
	}
	if aggregateMemberSetOriginRequiresCurrentSource(ctx, rm, []types.AnswerAggregateFact{fact}) {
		return false
	}
	return aggregateMemberSetOriginContextAvailable(ctx, rm, origins)
}

func aggregateMemberSetOriginRequiresCurrentSource(ctx *types.BusContext, rm *types.RequestModel, aggregateFacts []types.AnswerAggregateFact) bool {
	if rm == nil {
		return false
	}
	if allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLandingWithAggregateFacts(ctx, aggregateFacts); decided {
		return !allowed
	}
	hint := types.TurnRouteHint{}
	if ctx != nil {
		hint = ctx.TurnRouteHint
	}
	return types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, hint) == types.RuntimeSourceRequirementPrecise
}

func aggregateMemberSetOriginContextAvailable(ctx *types.BusContext, rm *types.RequestModel, origins []types.AnswerEvidenceOrigin) bool {
	if len(origins) == 0 {
		return false
	}
	if aggregateMemberSetRuntimeContextAvailable(ctx, rm) {
		return true
	}
	if rm != nil {
		if rm.Predicates.IsHistoryLookup {
			for _, origin := range origins {
				if origin == types.AnswerEvidenceOriginVCSMetadata || origin == types.AnswerEvidenceOriginVCSDiff {
					return true
				}
			}
		}
		if rm.ExternalObservationPolicy != nil &&
			(rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() ||
				rm.ExternalObservationPolicy.ExcludesCurrentSource()) {
			return true
		}
		if rm.HasExternalObservationArtifactReference() {
			return true
		}
	}
	for _, origin := range origins {
		switch origin {
		case types.AnswerEvidenceOriginCommandMeasurement,
			types.AnswerEvidenceOriginRepoNegativeSearch,
			types.AnswerEvidenceOriginCrossRepoIndex,
			types.AnswerEvidenceOriginExternalDocument,
			types.AnswerEvidenceOriginWebPage,
			types.AnswerEvidenceOriginMCPResource,
			types.AnswerEvidenceOriginConnectorResource:
			return true
		}
	}
	return false
}

func aggregateMemberSetRuntimeContextAvailable(ctx *types.BusContext, rm *types.RequestModel) bool {
	if allowed, decided := runtimeSourceAuthorityAllowsRuntimeCompletionLanding(ctx); decided {
		return allowed
	}
	if rm != nil && (rm.HasExternalOnlyRuntimeArtifact() || rm.HasRuntimeArtifactWithoutRequiredCurrentSource()) {
		return true
	}
	if ctx != nil && (strings.TrimSpace(ctx.AttachedLog) != "" || strings.TrimSpace(ctx.AttachedHitrace) != "") {
		return true
	}
	if ctx != nil && ctx.Mutable != nil {
		if waiver := ctx.Mutable.EvidenceFloorWaiver(); waiver.IsActive() {
			switch waiver.Reason {
			case types.EvidenceFloorWaiverExternalLog,
				types.EvidenceFloorWaiverExternalTrace,
				types.EvidenceFloorWaiverNoRepoIntersection,
				types.EvidenceFloorWaiverInformationalRuntime:
				return true
			}
		}
	}
	return false
}

func exhaustiveEnumerationPrincipalGroupedCountGaps(ctx *types.BusContext, facts []types.AnswerAggregateFact) []string {
	if ctx == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return nil
	}
	rm := &ctx.AnalysisIR.RequestModel
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) {
		return nil
	}
	memberSetLabels := map[string]bool{}
	for _, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer {
			continue
		}
		if key := aggregateFactCategoryKey(fact); key != "" {
			memberSetLabels[key] = true
		}
	}
	var gaps []string
	for idx, fact := range facts {
		if fact.Kind != types.AnswerAggregateGroupedCount && fact.Kind != types.AnswerAggregateBucketCount {
			continue
		}
		if types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(fact.Value))
		if err != nil || n <= 0 || len(fact.Members) > 0 {
			continue
		}
		if n == 1 && len(memberSetLabels) > 0 {
			// A singleton grouped/bucket count is often a scalar boundary or
			// basis note for an already-handed-off exhaustive slate (for
			// example "one declaration block contains these members"). Forcing
			// the explorer to manufacture a synthetic singleton member turns
			// that metadata into a user-visible principal row later. Multi-item
			// grouped counts still need members or a matching member_set because
			// they can hide a missing answer category.
			continue
		}
		if key := aggregateFactCategoryKey(fact); key != "" && memberSetLabels[key] {
			continue
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = string(fact.Kind)
		}
		gaps = append(gaps, fmt.Sprintf("aggregate_facts[%d] %s %q value=%d has no members", idx, fact.Kind, label, n))
		if len(gaps) >= 8 {
			break
		}
	}
	return gaps
}

func aggregateFactCategoryKey(fact types.AnswerAggregateFact) string {
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		return ""
	}
	return strings.ToLower(label)
}

type aggregateMemberSupportIndex struct {
	byLabel                         map[string][]types.EvidenceItem
	byLocation                      map[string][]types.EvidenceItem
	byID                            map[string]types.EvidenceItem
	valueLiteralEvidence            []types.EvidenceItem
	answerSyms                      map[string]types.AnswerSymbol
	toolLinesByLocation             map[string][]string
	sourceInventoryLabelsByLocation map[string][]string
	sourceInventoryLabelsBySupport  map[string][]string
	readFileLines                   []aggregateToolLine
}

func buildAggregateMemberSupportIndex(ctx *types.BusContext) aggregateMemberSupportIndex {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
}

func buildAggregateMemberSupportIndexWithEvidence(ctx *types.BusContext, evidence []types.EvidenceItem) aggregateMemberSupportIndex {
	idx := aggregateMemberSupportIndex{
		byLabel:                         map[string][]types.EvidenceItem{},
		byLocation:                      map[string][]types.EvidenceItem{},
		byID:                            map[string]types.EvidenceItem{},
		answerSyms:                      map[string]types.AnswerSymbol{},
		toolLinesByLocation:             map[string][]string{},
		sourceInventoryLabelsByLocation: map[string][]string{},
		sourceInventoryLabelsBySupport:  map[string][]string{},
		readFileLines:                   nil,
	}
	if ctx == nil || ctx.Mutable == nil {
		return idx
	}
	for _, ev := range evidence {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if id := strings.TrimSpace(ev.ID); id != "" {
			idx.byID[id] = ev
		}
		if loc := aggregateSupportLocationKey(ev.Source, ev.LineStart); loc != "" {
			idx.byLocation[loc] = append(idx.byLocation[loc], ev)
		}
		if aggregateEvidenceCanCarryValueLiteral(ev) {
			idx.valueLiteralEvidence = append(idx.valueLiteralEvidence, ev)
		}
		for _, raw := range aggregateEvidenceMemberLabels(ev) {
			key := aggregateMemberKey(raw)
			if key == "" {
				continue
			}
			idx.byLabel[key] = append(idx.byLabel[key], ev)
		}
	}
	if syms, claim := ctx.Mutable.EmittedAnswerSymbols(); claim == types.CompletenessComplete {
		for _, sym := range syms {
			key := aggregateMemberKey(sym.Name)
			if key == "" {
				continue
			}
			idx.answerSyms[key] = sym
		}
	}
	for _, result := range aggregateSupportToolResults(ctx) {
		if !result.Success || strings.TrimSpace(result.ToolName) != "read_file" {
			continue
		}
		for _, line := range aggregateReadFileToolLinesFromResult(result) {
			loc := aggregateSupportLocationKey(line.File, line.Line)
			if loc == "" {
				continue
			}
			idx.toolLinesByLocation[loc] = append(idx.toolLinesByLocation[loc], line.Text)
			idx.readFileLines = append(idx.readFileLines, line)
		}
	}
	appendSourceInventorySupportLocations(ctx, &idx)
	return idx
}

func appendSourceInventorySupportLocations(ctx *types.BusContext, idx *aggregateMemberSupportIndex) {
	if ctx == nil || ctx.Mutable == nil || idx == nil {
		return
	}
	appendSourceInventoryObservationSupportLocations(ctx.Mutable.SourceInventoryObservation(), idx)
	if ctx.AnalysisIR == nil {
		return
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if !profile.Active() || profile.Confidence < 0.70 {
		return
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return
	}
	scopes := sourceInventoryScopes(ctx, graph, nil)
	if len(scopes) == 0 {
		return
	}
	supportBudget := sourceInventoryExecBudgetForSupport(ctx, graph)
	for _, set := range sourceInventoryCandidateSets(ctx, graph, newSourceInventoryExecutionViewWithBudget(graph, scopes, supportBudget), scopes, profile, nil, false, false, "", supportBudget) {
		if !set.complete {
			continue
		}
		for _, candidate := range set.candidates {
			if strings.TrimSpace(candidate.member) == "" {
				continue
			}
			appendSourceInventorySupportIndexRow(idx, candidate.member, candidate.key, candidate.supportRef, candidate.file, candidate.line)
			for _, attr := range candidate.attributes {
				appendSourceInventorySupportIndexRow(idx, attr.member, attr.key, attr.supportRef, attr.file, attr.line)
			}
		}
	}
	for loc, labels := range idx.sourceInventoryLabelsByLocation {
		idx.sourceInventoryLabelsByLocation[loc] = dedupStringsPreserveOrder(labels)
	}
	for ref, labels := range idx.sourceInventoryLabelsBySupport {
		idx.sourceInventoryLabelsBySupport[ref] = dedupStringsPreserveOrder(labels)
	}
}

func appendSourceInventoryObservationSupportLocations(observation types.SourceInventoryObservation, idx *aggregateMemberSupportIndex) {
	if !observation.IsActive() || idx == nil {
		return
	}
	for _, set := range observation.Sets {
		if !set.Complete {
			continue
		}
		for _, member := range set.Members {
			appendSourceInventorySupportIndexRow(idx, member.Name, member.Key, member.SupportRef, member.File, member.Line)
			for _, attr := range member.Attributes {
				appendSourceInventorySupportIndexRow(idx, attr.Name, attr.Key, attr.SupportRef, attr.File, attr.Line)
			}
		}
	}
}

func appendSourceInventorySupportIndexRow(idx *aggregateMemberSupportIndex, name, key, supportRef, file string, line int) {
	if idx == nil {
		return
	}
	labels := aggregateSupportLabels(name, []string{key})
	if len(labels) == 0 {
		return
	}
	if loc := aggregateSupportLocationKey(file, line); loc != "" {
		idx.sourceInventoryLabelsByLocation[loc] = append(idx.sourceInventoryLabelsByLocation[loc], labels...)
	}
	if ref := strings.TrimSpace(supportRef); ref != "" {
		idx.sourceInventoryLabelsBySupport[ref] = append(idx.sourceInventoryLabelsBySupport[ref], labels...)
	}
}

// memberHasDecoratedCodeIdentityBase reports whether an aggregate-
// fact member has the shape "<code identifier> (<qualifier>)" — a
// decorated code-identity surface like "Orchestrator (4阶段管道整合)".
// These members trip the finalizer's per-item citation_ref alignment
// oracle because (a) the surface text the model uses as a table /
// list row label is the FULL decorated string, and (b) the
// downstream evidence-anchor matching cannot resolve the decorated
// surface verbatim against any grounded subject / object / anchor
// symbol. Bare code-identity members (no decorator) can still get
// auto-grounded via the existing enrichment pass when an evidence
// anchor names them verbatim, so they intentionally do not trigger
// this gate.
func memberHasDecoratedCodeIdentityBase(member string) bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return false
	}
	base, _, ok := types.AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		return false
	}
	return types.IsCodeIdentitySurface(strings.TrimSpace(base))
}

// validateAggregateMemberSetSupportRefs is the emit-time gate that
// closes the schema asymmetry behind the 2026-05-16 architectural-
// comparison finalize loop. AnswerAggregateFact.SupportRefs is
// declared `omitempty` so any model emit goes through normalize even
// when SupportRefs is absent — but the finalizer's downstream
// alignment oracle DEMANDS per-member grounding to verify table /
// list row citation_refs. When the two halves disagree, every row
// quoting a decorated code-shape member gets rejected with
// candidate_citations=[]; the model has no path to fix it because
// the missing or unresolved data is upstream.
//
// Run AFTER enrichCompletionAggregateFactsWithMemberSupportWithEvidence
// so auto-fillable members get their support_refs first. The check
// is narrowed to DECORATED code-identity members because the
// enrichment pass can usually back bare members like "Intent" /
// "StageAnalyze" against verbatim evidence anchors; the decorator
// (parens + qualifier) makes verbatim matching fail and the
// finalizer has no way to recover. Reaching this validator with
// empty or unresolved SupportRefs + decorated code-shape members
// means the explorer must either attach support_refs that resolve to
// grounded member evidence or drop the decorator so the bare symbol
// can auto-resolve.
func validateAggregateMemberSetSupportRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) error {
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	var violations []string
	seenViolation := map[string]bool{}
	addViolation := func(message string) {
		message = strings.TrimSpace(message)
		if message == "" || seenViolation[message] {
			return
		}
		seenViolation[message] = true
		violations = append(violations, message)
	}
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet {
			continue
		}
		if aggregateMemberSetShadowedBySourceInventoryPrincipalRows(fact, facts) {
			continue
		}
		if aggregateNarrativeMemberSetRequiresAlignedCurrentSourceSupport(ctx, fact) {
			if len(fact.MemberNotes) != len(fact.Members) || len(fact.SupportRefs) != len(fact.Members) {
				addViolation(fmt.Sprintf(
					"aggregate_facts: current-source architecture/mechanism member_set %q uses member_notes as a completion boundary, but members/member_notes/support_refs are not exactly index-aligned (members=%d member_notes=%d support_refs=%d). Re-emit either without member_notes, or with exactly one verified non-empty member_note and one grounded support_ref per member in the same order; identity-only declarations do not prove behavioral responsibility",
					strings.TrimSpace(fact.Label), len(fact.Members), len(fact.MemberNotes), len(fact.SupportRefs),
				))
				continue
			}
			for i := range fact.Members {
				if strings.TrimSpace(fact.MemberNotes[i]) == "" || strings.TrimSpace(fact.SupportRefs[i]) == "" {
					addViolation(fmt.Sprintf(
						"aggregate_facts: current-source architecture/mechanism member_set %q has an empty member_note or support_ref at index %d for member %q. Re-emit with one verified non-empty note and one grounded ref per member in the same order, or omit member_notes entirely",
						strings.TrimSpace(fact.Label), i, strings.TrimSpace(fact.Members[i]),
					))
				}
				if strings.TrimSpace(fact.SupportRefs[i]) != "" &&
					!aggregateMemberSetSupportRefsResolveMember(fact, fact.Members[i], support) {
					addViolation(fmt.Sprintf(
						"aggregate_facts: current-source architecture/mechanism member_set %q has support_ref %q at index %d, but it does not resolve to grounded evidence for member %q. Re-emit with the already-read producer/callsite/consumer location that proves this member's responsibility; an identity-only or unrelated location cannot close the investigation",
						strings.TrimSpace(fact.Label), strings.TrimSpace(fact.SupportRefs[i]), i, strings.TrimSpace(fact.Members[i]),
					))
				}
			}
			// The narrative contract above already checks the same-member support
			// and non-empty notes required by the per-member table lane. Avoid
			// emitting two differently worded violations for the same row.
			continue
		}
		if aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(ctx, fact, evidence) {
			for i, member := range fact.Members {
				if aggregateMemberSetSupportRefsResolveMember(fact, member, support) {
					continue
				}
				ref := ""
				if i < len(fact.SupportRefs) {
					ref = strings.TrimSpace(fact.SupportRefs[i])
				}
				addViolation(fmt.Sprintf(
					"aggregate_facts: current-source per-member table member_set %q has member %q at index %d with support_ref %q, but that row does not resolve to grounded evidence for the same member. Re-emit one exact grounded evidence id or source location per members[] row in the same order; do not use a nearby function or shared file as evidence for a different member",
					strings.TrimSpace(fact.Label), strings.TrimSpace(member), i, ref,
				))
			}
			if aggregatePerMemberTableCurrentSourceSetRequiresAttributeNotes(ctx, fact, evidence) {
				if len(fact.MemberNotes) != len(fact.Members) {
					addViolation(fmt.Sprintf(
						"aggregate_facts: current-source per-member attribute table member_set %q has members=%d but member_notes=%d. Re-emit exactly one verified non-empty member_note and one same-member grounded support_ref per members[] row in the same order; an identity-only roster cannot supply the requested per-row attributes",
						strings.TrimSpace(fact.Label), len(fact.Members), len(fact.MemberNotes),
					))
				} else {
					for i, member := range fact.Members {
						if strings.TrimSpace(fact.MemberNotes[i]) != "" {
							continue
						}
						addViolation(fmt.Sprintf(
							"aggregate_facts: current-source per-member attribute table member_set %q has an empty member_note at index %d for member %q. Re-emit a verified row attribute note grounded by the aligned same-member support_ref; identity alone cannot prove the requested row attributes",
							strings.TrimSpace(fact.Label), i, strings.TrimSpace(member),
						))
					}
				}
			}
			continue
		}
		var missing []string
		var unresolved []string
		for _, member := range fact.Members {
			if decoratedAggregateMemberCanRelyOnOriginSpecificProvenance(ctx, fact, member) {
				continue
			}
			if !memberHasDecoratedCodeIdentityBase(member) {
				continue
			}
			if len(fact.SupportRefs) == 0 {
				missing = append(missing, member)
				continue
			}
			if !aggregateMemberSetSupportRefsResolveMember(fact, member, support) {
				unresolved = append(unresolved, member)
			}
		}
		if len(missing) == 0 && len(unresolved) == 0 {
			continue
		}
		problem := missing
		problemKind := "support_refs is empty"
		if len(problem) == 0 {
			problem = unresolved
			problemKind = "support_refs do not resolve to the cited member"
		}
		sample := problem
		if len(sample) > 3 {
			sample = sample[:3]
		}
		quoted := make([]string, 0, len(sample))
		for _, m := range sample {
			quoted = append(quoted, fmt.Sprintf("%q", m))
		}
		omitted := ""
		if len(problem) > len(sample) {
			omitted = fmt.Sprintf(" (+%d more)", len(problem)-len(sample))
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = "(unlabeled)"
		}
		addViolation(fmt.Sprintf(
			"aggregate_facts: member_set %q has %d member(s) shaped as "+
				"\"<code identifier> (<qualifier>)\" (%s%s) but %s. "+
				"Decorated code-shape members never auto-resolve against evidence anchors, "+
				"so downstream answer rendering cannot align row item citation_ref values "+
				"without explicit per-member grounding. Re-emit with support_refs in either "+
				"of these forms: labeled [\"<member-leading-symbol>: <file>:<line>\", …] "+
				"where the label is the member's bare leading identifier (no decorator), "+
				"or positional [\"<file>:<line>\", …] with one entry per members[] in the "+
				"same order. If you cannot ground a member, drop the decorator so the "+
				"bare symbol can auto-resolve; removing the member entirely is a last "+
				"resort and you must name the removed member in reason with one line on why.",
			label, len(problem), strings.Join(quoted, ", "), omitted, problemKind,
		))
	}
	if len(violations) == 0 {
		return nil
	}
	if len(violations) == 1 {
		return fmt.Errorf("%s", violations[0])
	}
	indexed := make([]string, 0, len(violations))
	for i, violation := range violations {
		indexed = append(indexed, fmt.Sprintf("[%d] %s", i+1, violation))
	}
	return fmt.Errorf(
		"%s The payload has %d member-row grounding violations — fix ALL of them in this one re-emit: %s",
		violations[0], len(violations), strings.Join(indexed, "; "),
	)
}

// aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport activates the
// generic row-identity contract only when all of its inputs are typed: the
// analyzer declared a per-member table, runtime/source authority requires the
// current checkout, the fact is a load-bearing member_set, and this turn has
// citable current-source evidence in a real repository. It never inspects the
// user request or answer prose. External/runtime-only rosters retain their
// origin-specific support lanes.
func aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(
	ctx *types.BusContext,
	fact types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) bool {
	if ctx == nil || ctx.AnalysisIR == nil ||
		!ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable ||
		fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 ||
		!aggregateFactCanDefineModelOwnedCompletionBoundary(fact) ||
		completionRepoRoot(ctx) == "" ||
		aggregateMemberSetCanRelyOnOriginSpecificProvenance(ctx, fact) {
		return false
	}
	authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
	if !authority.CurrentSourceRequired {
		return false
	}
	for _, item := range evidence {
		if item.IsCitable() && strings.TrimSpace(item.Source) != "" && item.LineStart > 0 {
			return true
		}
	}
	return false
}

// aggregatePerMemberTableCurrentSourceSetRequiresAttributeNotes tightens the
// same typed/current-source/load-bearing lane from row identity to row
// attribute sufficiency. AnswerIntentContract is the single typed compiler for
// this distinction: plain enumeration remains identity-only, while a declared
// per-member table requires one model-authored, evidence-backed note per row.
// No requested column label or final-answer prose is inspected here.
func aggregatePerMemberTableCurrentSourceSetRequiresAttributeNotes(
	ctx *types.BusContext,
	fact types.AnswerAggregateFact,
	evidence []types.EvidenceItem,
) bool {
	if !aggregatePerMemberTableCurrentSourceSetRequiresAlignedSupport(ctx, fact, evidence) {
		return false
	}
	contract := types.CompileAnswerIntentContractWithPreflight(
		ctx.AnalysisIR.RequestModel,
		&ctx.AnalysisIR.AnswerContract,
		ctx.RuntimeArtifactPreflight,
	)
	return contract.SourceInventoryAttributeDemand
}

// aggregateNarrativeMemberSetRequiresAlignedCurrentSourceSupport binds the
// schema's per-member responsibility teaching to its emit-time contract. It
// consumes only typed aggregate fields and typed evidence origins: no user
// request text, model reasoning, or final-answer prose participates. Runtime,
// trace, VCS, and connector rosters therefore keep their origin-specific
// evidence lanes and are not forced into repository file:line support.
func aggregateNarrativeMemberSetRequiresAlignedCurrentSourceSupport(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.MemberNotes) == 0 ||
		!aggregateFactCanDefineModelOwnedCompletionBoundary(fact) {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			return true
		}
	}
	return false
}

func aggregateMemberSetShadowedBySourceInventoryPrincipalRows(fact types.AnswerAggregateFact, facts []types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateMemberSet ||
		!strings.Contains(fact.Provenance, "demoted:shadowed_by_source_inventory_principal_row_set") {
		return false
	}
	for _, candidate := range facts {
		if candidate.Kind != types.AnswerAggregateMemberSet ||
			strings.TrimSpace(candidate.Provenance) != types.SourceInventoryPrincipalRowSetAggregateProvenance ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(candidate) ||
			len(candidate.Members) == 0 ||
			len(candidate.SupportRefs) != len(candidate.Members) {
			continue
		}
		return true
	}
	return false
}

func aggregateMemberSetSupportRefsResolveMember(fact types.AnswerAggregateFact, member string, support aggregateMemberSupportIndex) bool {
	labels := aggregateMemberDisplayCandidates(member)
	if len(labels) == 0 {
		return false
	}
	// Exact-parsed positional refs keep the ORIGINAL bidirectional
	// semantics byte-for-byte: a positional array the model actually
	// emitted binds each position to its member, and a mismatch must count
	// as unresolved so the swapped-refs repair lane can re-derive the right
	// location per member.
	if loc, ok := aggregateMemberSetPositionalSupportLocation(fact, memberIndexInAggregateFact(fact, member), labels); ok {
		receiverOperationMatch := aggregateSupportLocationMatchesReceiverOperation(loc, member, support)
		if !aggregateSupportLocationCompatibleWithMember(member, loc) && !receiverOperationMatch {
			return false
		}
		return aggregateSupportLocationMatchesMemberLabels(loc, labels, support) ||
			aggregateSupportLocationNearbyToolValueMatchesLabels(loc, labels, support) ||
			receiverOperationMatch
	}
	// SUPPREF-TOL rework (P0-A): a positional location that only exists via
	// the decoration-strip retry is consumed SUCCESS-ONLY. In the baseline
	// tree such a ref failed to parse and the member fell through to the
	// per-ref lanes; an early false here would have CUT OFF that fallback
	// and minted a baseline-accepted/now-rejected form (decorated
	// location-only positional ref plus a different ref that resolves the
	// member). Same philosophy as the aggregateMemberSetMemberUsableAt
	// positional arm: accept on match, otherwise keep falling through.
	if loc, ok := aggregateMemberSetPositionalStripRetrySupportLocation(fact, memberIndexInAggregateFact(fact, member), labels); ok {
		receiverOperationMatch := aggregateSupportLocationMatchesReceiverOperation(loc, member, support)
		if (aggregateSupportLocationCompatibleWithMember(member, loc) || receiverOperationMatch) &&
			(aggregateSupportLocationMatchesMemberLabels(loc, labels, support) ||
				aggregateSupportLocationNearbyToolValueMatchesLabels(loc, labels, support) ||
				receiverOperationMatch) {
			return true
		}
	}
	for _, ref := range fact.SupportRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		// SUPPREF-TOL: exact-match lanes try the verbatim ref first, then
		// one decoration-stripped retry (same lookup, no widened matching).
		for _, candidate := range aggregateSupportRefLookupCandidates(ref) {
			if aggregateSourceInventorySupportRefMatchesLabels(candidate, labels, support.sourceInventoryLabelsBySupport) {
				return true
			}
			if ev, ok := support.byID[candidate]; ok && aggregateEvidenceMatchesAnyLabel(ev, labels) {
				return true
			}
		}
		label, loc, ok := aggregateMemberSupportRefPartsTolerant(ref)
		if !ok {
			continue
		}
		receiverOperationMatch := aggregateSupportLocationMatchesReceiverOperation(loc, member, support)
		if (!aggregateSupportLocationCompatibleWithMember(member, loc) && !receiverOperationMatch) ||
			!aggregateSupportLabelMatchesMember(label, labels) {
			continue
		}
		refLabels := aggregateSupportLabels(label, aggregateMemberLocationSupportLabels(member, labels))
		if aggregateLocationEvidenceMatchesLabels(loc, refLabels, support.byLocation) ||
			aggregateToolLocationMatchesLabels(loc, refLabels, support.toolLinesByLocation) ||
			aggregateSourceInventoryLocationMatchesLabels(loc, refLabels, support.sourceInventoryLabelsByLocation) ||
			aggregateSupportLocationNearbyToolValueMatchesLabels(loc, refLabels, support) ||
			receiverOperationMatch {
			return true
		}
	}
	return false
}

// aggregateSupportLocationMatchesReceiverOperation resolves the syntactic
// ambiguity between a compact display relation (`caller->callee`) and a
// receiver-shaped source expression (`sink_->write()`). It never decides from
// the aggregate member alone. Acceptance requires one grounded call at the
// exact support location whose operation tail matches and whose source snippet
// contains the same receiver/operator prefix. This preserves strict relation
// file-axis checks while allowing a source-faithful member expression to bind
// its own call-site evidence.
func aggregateSupportLocationMatchesReceiverOperation(location, member string, support aggregateMemberSupportIndex) bool {
	member = strings.TrimSpace(member)
	if member == "" || strings.Contains(member, " -> ") {
		return false
	}
	arrow := strings.LastIndex(member, "->")
	if arrow <= 0 || arrow+2 >= len(member) {
		return false
	}
	prefix := member
	if open := strings.Index(prefix[arrow+2:], "("); open >= 0 {
		prefix = prefix[:arrow+2+open]
	}
	prefix = strings.TrimSpace(strings.TrimSuffix(prefix, "()"))
	if prefix == "" {
		return false
	}
	want := types.NormalizedSurfaceSymbolTail(member)
	if want == "" {
		return false
	}
	for _, ev := range support.byLocation[strings.TrimSpace(location)] {
		if ev.AnchorKind != types.AnchorCall || ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		matchedTail := false
		for _, raw := range aggregateEvidenceMemberLabels(ev) {
			if types.NormalizedSurfaceSymbolTail(raw) == want {
				matchedTail = true
				break
			}
		}
		if matchedTail && strings.Contains(strings.TrimSpace(ev.Snippet), prefix) {
			return true
		}
	}
	return false
}

// aggregateSupportRefLookupCandidates returns the verbatim ref plus its
// decoration-stripped form (when stripping changed anything) for the
// exact-match lookup lanes (evidence ids, source-inventory support refs).
// Order is load-bearing: the verbatim form always wins first, so the
// already-working surface cannot change.
func aggregateSupportRefLookupCandidates(ref string) []string {
	out := []string{ref}
	if stripped, changed := stripAggregateSupportRefDecorations(ref); changed {
		out = append(out, stripped)
	}
	return out
}

func memberIndexInAggregateFact(fact types.AnswerAggregateFact, member string) int {
	for idx, candidate := range fact.Members {
		if candidate == member {
			return idx
		}
	}
	return -1
}

func decoratedAggregateMemberCanRelyOnRuntimeArtifactProvenance(ctx *types.BusContext, fact types.AnswerAggregateFact, member string) bool {
	if !memberHasDecoratedCodeIdentityBase(member) {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	if rm == nil {
		return false
	}
	if authority := runtimeSourceAnswerAuthorityForCompletionWithAggregateFacts(ctx, []types.AnswerAggregateFact{fact}); runtimeSourceAuthorityAppliesToCompletionLanding(authority) {
		if !runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority) {
			return false
		}
		if aggregateFactHasExplicitRuntimeArtifactOrigin(fact) {
			return true
		}
		return !runtimeSourceCompletionLandingRequiresExplicitRuntimeOrigin(ctx, rm)
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return false
	}
	attachedRuntimeArtifact := aggregateSupportRuntimeArtifactContextActive(ctx)
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(attachedRuntimeArtifact) {
		return true
	}
	if attachedRuntimeArtifact && ctx != nil && ctx.Mutable != nil && runtimeArtifactGroundingBypassAllowed(ctx) {
		if waiver := ctx.Mutable.EvidenceFloorWaiver(); waiver.IsActive() {
			switch waiver.Reason {
			case types.EvidenceFloorWaiverExternalLog,
				types.EvidenceFloorWaiverExternalTrace,
				types.EvidenceFloorWaiverNoRepoIntersection,
				types.EvidenceFloorWaiverInformationalRuntime:
				return true
			}
		}
	}
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if origin == types.AnswerEvidenceOriginRuntimeArtifact {
			return true
		}
	}
	return false
}

func aggregateFactHasExplicitRuntimeArtifactOrigin(fact types.AnswerAggregateFact) bool {
	if types.AnswerEvidenceOriginFromStructuredToken(fact.Provenance) == types.AnswerEvidenceOriginRuntimeArtifact {
		return true
	}
	for _, dim := range fact.Dimensions {
		if types.AnswerEvidenceOriginFromStructuredToken(dim.Value) == types.AnswerEvidenceOriginRuntimeArtifact {
			return true
		}
	}
	for _, ref := range fact.SupportRefs {
		if aggregateSupportRefCarriesExplicitRuntimeArtifactOrigin(ref) {
			return true
		}
	}
	return false
}

// aggregateSupportRefCarriesExplicitRuntimeArtifactOrigin reports whether one
// support_refs entry explicitly declares the runtime-artifact origin. The
// dimension-token grammar keeps first say (its success surface is unchanged);
// SUPPREF-TOL then retries the SAME grammar on the decoration-stripped form,
// and finally reads the ref through the EXISTING support-ref origin grammar
// (types.AnswerEvidenceOriginFromSupportRef) — the grammar the aggregate-fact
// origin projection already applies to this very field, which recognizes
// artifact-path refs such as the reserved attached_trace.txt blob basename.
// The h9 witness ref "attached_trace.txt: wakeup_chain path" names the
// attached artifact yet was invisible here, so its fact lost the explicit
// runtime-origin standing its twin fact earned from a mere origin dimension
// token (§29.104.13).
func aggregateSupportRefCarriesExplicitRuntimeArtifactOrigin(ref string) bool {
	if types.AnswerEvidenceOriginFromStructuredToken(ref) == types.AnswerEvidenceOriginRuntimeArtifact {
		return true
	}
	if stripped, changed := stripAggregateSupportRefDecorations(ref); changed &&
		types.AnswerEvidenceOriginFromStructuredToken(stripped) == types.AnswerEvidenceOriginRuntimeArtifact {
		return true
	}
	return types.AnswerEvidenceOriginFromSupportRef(ref) == types.AnswerEvidenceOriginRuntimeArtifact
}

func runtimeSourceCompletionLandingRequiresExplicitRuntimeOrigin(ctx *types.BusContext, rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	if ctx != nil && types.RouteBackedExternalObservationRequiresCurrentSource(rm, ctx.TurnRouteHint) {
		return !runtimeCompletionAuthorityAllowsImplicitRuntimeOrigin(ctx)
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.HasTypedCurrentSourceScopeRequest() || rm.HasCurrentSourceObligationSignal() {
		return true
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	return false
}

func runtimeCompletionAuthorityAllowsImplicitRuntimeOrigin(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
	if !runtimeSourceAuthorityAppliesToCompletionLanding(authority) ||
		!runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority) ||
		authority.KeepsCurrentSourceLaneLoadBearing() {
		return false
	}
	return authority.DeterministicRuntimeQueryCount > 0 || authority.AddressableRuntimeCount > 0
}

func aggregateSupportRuntimeArtifactContextActive(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	return types.RuntimeArtifactContextActiveFromBus(ctx)
}

func decoratedAggregateMemberCanRelyOnOriginSpecificProvenance(ctx *types.BusContext, fact types.AnswerAggregateFact, member string) bool {
	if !memberHasDecoratedCodeIdentityBase(member) {
		return false
	}
	if decoratedAggregateMemberCanRelyOnVCSProvenance(ctx, fact, member) {
		return true
	}
	if decoratedAggregateMemberCanRelyOnRuntimeArtifactProvenance(ctx, fact, member) {
		return true
	}
	rm := requestModelForAggregateSupport(ctx)
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		switch origin {
		case types.AnswerEvidenceOriginCommandMeasurement,
			types.AnswerEvidenceOriginRepoNegativeSearch,
			types.AnswerEvidenceOriginCrossRepoIndex,
			types.AnswerEvidenceOriginExternalDocument,
			types.AnswerEvidenceOriginWebPage,
			types.AnswerEvidenceOriginMCPResource,
			types.AnswerEvidenceOriginConnectorResource:
			return true
		}
	}
	return false
}

func decoratedAggregateMemberCanRelyOnVCSProvenance(ctx *types.BusContext, fact types.AnswerAggregateFact, member string) bool {
	base, _, ok := types.AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		return false
	}
	base = strings.TrimSpace(base)
	if !aggregateMemberVCSCommitHashBaseRe.MatchString(base) {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if origin == types.AnswerEvidenceOriginVCSMetadata || origin == types.AnswerEvidenceOriginVCSDiff {
			return true
		}
	}
	return false
}

func requestModelForAggregateSupport(ctx *types.BusContext) *types.RequestModel {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil {
		return &ctx.AnalysisIR.RequestModel
	}
	if ctx.Mutable != nil {
		return ctx.Mutable.RequestModel()
	}
	return nil
}

func enrichCompletionAggregateFactsWithMemberSupport(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return enrichCompletionAggregateFactsWithMemberSupportWithEvidence(ctx, facts, evidence)
}

func enrichCompletionAggregateFactsWithMemberSupportWithEvidence(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []types.AnswerAggregateFact {
	if len(facts) == 0 || ctx == nil || ctx.Mutable == nil {
		return facts
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	if len(support.readFileLines) == 0 {
		if !aggregateMemberSupportIndexHasEvidenceAnchors(support) {
			return facts
		}
	}
	out := cloneCompletionAggregateFacts(facts)
	for factIdx := range out {
		if out[factIdx].Kind != types.AnswerAggregateMemberSet || len(out[factIdx].Members) == 0 {
			continue
		}
		if len(out[factIdx].SupportRefs) > 0 {
			if refs, ok := aggregateMemberSetCanonicalObservationSupportRefs(out[factIdx], support); ok {
				out[factIdx].SupportRefs = refs
				continue
			}
			if refs, ok := aggregateMemberSetRepairedExistingSupportRefs(out[factIdx], support); ok {
				out[factIdx].SupportRefs = refs
			}
			continue
		}
		if refs, ok := aggregateMemberSetUniqueEvidenceSupportRefs(out[factIdx], support); ok {
			out[factIdx].SupportRefs = refs
			continue
		}
		if refs, ok := aggregateMemberSetUniqueReadFileSupportRefs(out[factIdx], support); ok {
			out[factIdx].SupportRefs = refs
			continue
		}
	}
	return out
}

// aggregateMemberSetCanonicalObservationSupportRefs reconciles two distinct
// axes that an explorer may legitimately place in one member_set: Members is
// the answer roster, while SupportRefs may enumerate every grounded
// observation of those members.  A function can therefore have two call-site
// refs even though it occupies one answer row.  Downstream row renderers need
// exactly one positional ref per member; leaving the observation-shaped list
// intact makes the later citation normalizer associate an unrelated ref with
// a row and can cause it to append a contradictory partial roster.
//
// The reconciliation is deliberately exact and fail-open.  Each raw ref is
// tested as a one-member positional fact against the already-grounded support
// index.  We use it only when it resolves exactly one member and every member
// has such a ref.  Ambiguous or incomplete evidence preserves the model's
// original payload for the existing validation/repair lanes.  Extra
// observations remain available in the evidence ledger; this function only
// chooses the canonical citation used by each roster row.
func aggregateMemberSetCanonicalObservationSupportRefs(fact types.AnswerAggregateFact, support aggregateMemberSupportIndex) ([]string, bool) {
	if fact.Kind != types.AnswerAggregateMemberSet ||
		len(fact.Members) == 0 ||
		len(fact.SupportRefs) == 0 ||
		len(fact.SupportRefs) == len(fact.Members) {
		return nil, false
	}

	type exactObservationRef struct {
		memberIdx int
		canonical string
	}
	matches := make([]exactObservationRef, len(fact.SupportRefs))
	for refIdx, raw := range fact.SupportRefs {
		matchedMember := -1
		canonical := ""
		for memberIdx, member := range fact.Members {
			probe := fact
			probe.Members = []string{member}
			probe.MemberNotes = nil
			probe.SupportRefs = []string{raw}
			if !aggregateMemberSetSupportRefsResolveMember(probe, member, support) {
				continue
			}
			candidate := strings.TrimSpace(raw)
			if repaired, ok := aggregateMemberCanonicalExistingSupportRef(raw, member, support); ok {
				candidate = repaired
			}
			if candidate == "" || matchedMember >= 0 {
				// One observation resolving two roster members is not precise
				// enough to drive a positional citation hard contract.
				matchedMember = -2
				canonical = ""
				break
			}
			matchedMember = memberIdx
			canonical = candidate
		}
		matches[refIdx] = exactObservationRef{memberIdx: matchedMember, canonical: canonical}
	}

	refs := make([]string, len(fact.Members))
	for memberIdx := range fact.Members {
		for refIdx := range matches {
			if matches[refIdx].memberIdx == memberIdx {
				refs[memberIdx] = matches[refIdx].canonical
				break
			}
		}
		if refs[memberIdx] == "" {
			return nil, false
		}
	}

	reconciled := fact
	reconciled.SupportRefs = refs
	for memberIdx, member := range reconciled.Members {
		if !aggregateMemberSetMemberUsableAt(reconciled, member, memberIdx, support) &&
			!aggregateMemberSetSupportRefsResolveMember(reconciled, member, support) {
			return nil, false
		}
	}
	return refs, true
}

func aggregateMemberSupportIndexHasEvidenceAnchors(support aggregateMemberSupportIndex) bool {
	return len(support.byLabel) > 0 || len(support.byLocation) > 0 || len(support.byID) > 0
}

func aggregateMemberSetUniqueEvidenceSupportRefs(fact types.AnswerAggregateFact, support aggregateMemberSupportIndex) ([]string, bool) {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 || len(fact.SupportRefs) > 0 {
		return nil, false
	}
	refs := make([]string, len(fact.Members))
	for idx, member := range fact.Members {
		ref, ok := aggregateMemberUniqueEvidenceSupportRef(member, support)
		if !ok {
			return nil, false
		}
		refs[idx] = ref
	}
	return refs, true
}

func aggregateMemberUniqueEvidenceSupportRef(member string, support aggregateMemberSupportIndex) (string, bool) {
	labels := aggregateMemberDisplayCandidates(member)
	if base, _, ok := types.AnswerAggregateDecoratedLabelParts(member); ok {
		base = strings.TrimSpace(base)
		if base != "" {
			labels = append(labels, base)
			if tail := types.NormalizedSurfaceSymbolTail(base); tail != "" {
				labels = append(labels, tail)
			}
		}
	}
	labels = dedupStringsPreserveOrder(labels)
	if len(labels) == 0 {
		return "", false
	}
	seen := map[string]bool{}
	var matches []string
	for _, label := range labels {
		key := aggregateMemberKey(label)
		if key == "" {
			continue
		}
		for _, ev := range support.byLabel[key] {
			if !ev.IsCitable() {
				continue
			}
			loc := aggregateSupportLocationKey(ev.Source, ev.LineStart)
			if loc == "" || seen[loc] {
				continue
			}
			if !aggregateEvidenceMatchesAnyLabel(ev, []string{label}) {
				continue
			}
			seen[loc] = true
			matches = append(matches, loc)
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func aggregateMemberSetUniqueReadFileSupportRefs(fact types.AnswerAggregateFact, support aggregateMemberSupportIndex) ([]string, bool) {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 || len(fact.SupportRefs) > 0 {
		return nil, false
	}
	refs := make([]string, len(fact.Members))
	for idx, member := range fact.Members {
		ref, ok := aggregateMemberReadFileSupportRef(member, support)
		if !ok {
			return nil, false
		}
		loc, ok := aggregateSupportRefLocationOnly(ref)
		if !ok {
			return nil, false
		}
		refs[idx] = loc
	}
	return refs, true
}

func aggregateMemberSetRepairedExistingSupportRefs(fact types.AnswerAggregateFact, support aggregateMemberSupportIndex) ([]string, bool) {
	if fact.Kind != types.AnswerAggregateMemberSet ||
		len(fact.Members) == 0 ||
		len(fact.SupportRefs) != len(fact.Members) {
		return nil, false
	}
	refs := append([]string(nil), fact.SupportRefs...)
	changed := false
	for idx, member := range fact.Members {
		if aggregateMemberSetSupportRefsResolveMember(fact, member, support) {
			continue
		}
		if ref, ok := aggregateMemberUniqueEvidenceSupportRef(member, support); ok {
			refs[idx] = ref
			changed = true
			continue
		}
		if ref, ok := aggregateMemberReadFileSupportRef(member, support); ok {
			if loc, ok := aggregateSupportRefLocationOnly(ref); ok {
				refs[idx] = loc
			} else {
				refs[idx] = ref
			}
			changed = true
			continue
		}
		if ref, ok := aggregateMemberCanonicalExistingSupportRef(fact.SupportRefs[idx], member, support); ok {
			refs[idx] = ref
			changed = true
			continue
		}
		return nil, false
	}
	if !changed {
		return nil, false
	}
	repaired := fact
	repaired.SupportRefs = refs
	for idx, member := range repaired.Members {
		if !aggregateMemberSetMemberUsableAt(repaired, member, idx, support) &&
			!aggregateMemberSetSupportRefsResolveMember(repaired, member, support) {
			return nil, false
		}
	}
	return refs, true
}

func aggregateMemberCanonicalExistingSupportRef(raw string, member string, support aggregateMemberSupportIndex) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	_, loc, ok := aggregateMemberSupportRefPartsTolerant(raw)
	if !ok || loc == "" {
		return "", false
	}
	labels := aggregateMemberDisplayCandidates(member)
	if len(labels) == 0 || !aggregateSupportLocationCompatibleWithMember(member, loc) {
		return "", false
	}
	if aggregateSupportLocationMatchesMemberLabels(loc, labels, support) {
		return loc, true
	}
	canon, ok := aggregateCanonicalSupportLocationByUniqueSuffix(loc, labels, support)
	if !ok || !aggregateSupportLocationCompatibleWithMember(member, canon) {
		return "", false
	}
	return canon, true
}

func aggregateCanonicalSupportLocationByUniqueSuffix(location string, labels []string, support aggregateMemberSupportIndex) (string, bool) {
	file, line, ok := aggregateSupportLocationParts(location)
	if !ok {
		return "", false
	}
	seen := map[string]bool{}
	var matches []string
	consider := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		candidateFile, candidateLine, ok := aggregateSupportLocationParts(candidate)
		if !ok || candidateLine != line {
			return
		}
		if !aggregateReadFilePathMatchesQualifier(candidateFile, file) {
			return
		}
		if !aggregateSupportLocationMatchesMemberLabels(candidate, labels, support) {
			return
		}
		seen[candidate] = true
		matches = append(matches, candidate)
	}
	for candidate := range support.byLocation {
		consider(candidate)
	}
	for candidate := range support.toolLinesByLocation {
		consider(candidate)
	}
	for candidate := range support.sourceInventoryLabelsByLocation {
		consider(candidate)
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func aggregateSupportLocationParts(location string) (string, int, bool) {
	surface, ok := types.ParseAnswerSourceLocationSurface(location)
	if !ok || surface.File == "" || surface.LineStart <= 0 {
		return "", 0, false
	}
	file := strings.TrimSpace(strings.ReplaceAll(surface.File, `\`, `/`))
	if file == "" {
		return "", 0, false
	}
	return file, surface.LineStart, true
}

func aggregateSupportRefLocationOnly(ref string) (string, bool) {
	_, loc, ok := aggregateMemberSupportRefPartsTolerant(ref)
	if !ok || strings.TrimSpace(loc) == "" {
		return "", false
	}
	return loc, true
}

func aggregateSupportToolResults(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	var out []types.ToolResult
	out = append(out, ctx.Mutable.DispatchToolResults()...)
	if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
		out = append(out, ta.ToolResults...)
	}
	return out
}

var aggregateReadFileGutterPattern = regexp.MustCompile(`^\s*(\d+)\s*[│|]\s?(.*)$`)
var aggregateToolLocationPattern = regexp.MustCompile(`\b[^\s"'` + "`" + `]+:\d+\b`)
var aggregateDecoratedLineQualifierPattern = regexp.MustCompile(`(?i)^\s*(?:#|lines?|ln|l|行|第)\s*#?\s*(\d+)\s*(?:行)?\s*$`)

const aggregateMaxReadFileRawRefBytes = 2 * 1024 * 1024

type aggregateToolLine struct {
	File string
	Line int
	Text string
}

func aggregateReadFileToolLinesFromResult(result types.ToolResult) []aggregateToolLine {
	coverage := result.ReadCoverage
	if coverage == nil {
		return nil
	}
	path := strings.TrimSpace(strings.ReplaceAll(coverage.Path, `\`, `/`))
	if path == "" {
		return nil
	}
	rawRef := strings.TrimSpace(result.RawRef)
	if rawRef == "" {
		rawRef = strings.TrimSpace(coverage.RawRef)
	}
	content, ok := LoadBlobText(rawRef, aggregateMaxReadFileRawRefBytes)
	if !ok {
		return nil
	}
	lineStart := coverage.LineStart
	if lineStart <= 0 {
		lineStart = 1
	}
	lineEnd := coverage.LineEnd
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	var out []aggregateToolLine
	for _, line := range strings.Split(content, "\n") {
		m := aggregateReadFileGutterPattern.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		lineNo, err := strconv.Atoi(m[1])
		if err != nil || lineNo <= 0 {
			continue
		}
		if lineNo < lineStart || lineNo > lineEnd {
			continue
		}
		out = append(out, aggregateToolLine{
			File: path,
			Line: lineNo,
			Text: strings.TrimSpace(m[2]),
		})
	}
	return out
}

func aggregateMemberSetMemberUsable(fact types.AnswerAggregateFact, member string, support aggregateMemberSupportIndex) bool {
	return aggregateMemberSetMemberUsableAt(fact, member, -1, support)
}

func aggregateMemberSetMemberUsableAt(fact types.AnswerAggregateFact, member string, memberIdx int, support aggregateMemberSupportIndex) bool {
	labels := aggregateMemberDisplayCandidates(member)
	if len(labels) == 0 {
		return false
	}
	if !aggregateMemberNeedsTypedSupport(labels) {
		return true
	}
	if aggregateMemberCoveredByAnswerSymbol(labels, support.answerSyms) {
		return true
	}
	if aggregateMemberCoveredByEvidenceLabels(labels, support.byLabel) {
		return true
	}
	if loc, ok := aggregateMemberSetPositionalSupportLocation(fact, memberIdx, labels); ok &&
		aggregateSupportLocationCompatibleWithMember(member, loc) &&
		(aggregateSupportLocationMatchesMemberLabels(loc, labels, support) ||
			aggregateSupportLocationNearbyToolValueMatchesLabels(loc, labels, support)) {
		return true
	}
	// SUPPREF-TOL: decoration-stripped positional supplement, success-only
	// like every arm of this oracle.
	if loc, ok := aggregateMemberSetPositionalStripRetrySupportLocation(fact, memberIdx, labels); ok &&
		aggregateSupportLocationCompatibleWithMember(member, loc) &&
		(aggregateSupportLocationMatchesMemberLabels(loc, labels, support) ||
			aggregateSupportLocationNearbyToolValueMatchesLabels(loc, labels, support)) {
		return true
	}
	if label, loc, ok := aggregateMemberSupportRefParts(member); ok {
		if !aggregateSupportLocationCompatibleWithMember(member, loc) {
			return false
		}
		refLabels := aggregateSupportLabels(label, aggregateMemberLocationSupportLabels(member, labels))
		if aggregateLocationEvidenceMatchesLabels(loc, refLabels, support.byLocation) ||
			aggregateToolLocationMatchesLabels(loc, refLabels, support.toolLinesByLocation) {
			return true
		}
	}
	for _, ref := range fact.SupportRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		// SUPPREF-TOL: verbatim first, one decoration-stripped retry after.
		for _, candidate := range aggregateSupportRefLookupCandidates(ref) {
			if aggregateSourceInventorySupportRefMatchesLabels(candidate, labels, support.sourceInventoryLabelsBySupport) {
				return true
			}
			if ev, ok := support.byID[candidate]; ok && aggregateEvidenceMatchesAnyLabel(ev, labels) {
				return true
			}
		}
		label, loc, ok := aggregateMemberSupportRefPartsTolerant(ref)
		if !ok {
			continue
		}
		if !aggregateSupportLocationCompatibleWithMember(member, loc) {
			continue
		}
		if aggregateSupportLocationMatchesMemberLabels(loc, labels, support) {
			return true
		}
		if aggregateSupportLabelMatchesMember(label, labels) &&
			aggregateSupportLocationVerified(loc, support) &&
			aggregateMemberValueLiteralCoveredByEvidence(labels, support) {
			return true
		}
		if !aggregateSupportLabelMatchesMember(label, labels) {
			continue
		}
		refLabels := aggregateSupportLabels(label, aggregateMemberLocationSupportLabels(member, labels))
		if aggregateLocationEvidenceMatchesLabels(loc, refLabels, support.byLocation) ||
			aggregateToolLocationMatchesLabels(loc, refLabels, support.toolLinesByLocation) ||
			aggregateSupportLocationNearbyToolValueMatchesLabels(loc, refLabels, support) {
			return true
		}
	}
	return false
}

func aggregateMemberSetPositionalSupportLocation(fact types.AnswerAggregateFact, memberIdx int, labels []string) (string, bool) {
	if memberIdx < 0 || memberIdx >= len(fact.Members) || len(fact.SupportRefs) != len(fact.Members) {
		return "", false
	}
	raw := strings.TrimSpace(fact.SupportRefs[memberIdx])
	if raw == "" {
		return "", false
	}
	// EXACT parse only, deliberately (SUPPREF-TOL rework): the resolve lane
	// attaches STRICT accountability to exact-parsed positional refs — each
	// position must back its own member or the member counts unresolved so
	// the swapped-refs repair can re-derive the right location. Routing the
	// strip retry through here would launder that accountability. The
	// decoration-stripped supplement lives in
	// aggregateMemberSetPositionalStripRetrySupportLocation and is consumed
	// success-only.
	label, loc, ok := aggregateMemberSupportRefParts(raw)
	if !ok || loc == "" {
		return "", false
	}
	// Positional support_refs are a schema-level association only for
	// location-only refs. If the model supplied any label, keep the stricter
	// label+location checks below so a mislabeled or generic-labeled ref cannot
	// be accepted just because it occupies the same array index.
	if strings.TrimSpace(label) != "" {
		return "", false
	}
	if !aggregateSupportLabelMatchesMember(label, labels) {
		return "", false
	}
	return loc, true
}

// aggregateMemberSetPositionalStripRetrySupportLocation is the SUPPREF-TOL
// tolerant supplement to the exact positional lane: it only speaks when the
// verbatim ref does NOT exact-parse and the decoration-stripped form is a
// legal label-less location. Callers must consume it SUCCESS-ONLY (accept on
// match, otherwise fall through to the per-ref lanes) — a strip-derived
// positional location carries none of the exact lane's strict positional
// accountability, and letting it early-return false minted the P0-A
// baseline-accepted/now-rejected form (decorated location-only positional
// ref cutting off a resolving labeled ref).
func aggregateMemberSetPositionalStripRetrySupportLocation(fact types.AnswerAggregateFact, memberIdx int, labels []string) (string, bool) {
	if memberIdx < 0 || memberIdx >= len(fact.Members) || len(fact.SupportRefs) != len(fact.Members) {
		return "", false
	}
	raw := strings.TrimSpace(fact.SupportRefs[memberIdx])
	if raw == "" {
		return "", false
	}
	if _, _, ok := aggregateMemberSupportRefParts(raw); ok {
		return "", false
	}
	label, loc, ok := aggregateMemberSupportRefPartsTolerant(raw)
	if !ok || loc == "" || strings.TrimSpace(label) != "" {
		return "", false
	}
	if !aggregateSupportLabelMatchesMember(label, labels) {
		return "", false
	}
	return loc, true
}

func aggregateSupportLocationVerified(loc string, support aggregateMemberSupportIndex) bool {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return false
	}
	return len(support.byLocation[loc]) > 0 ||
		len(support.toolLinesByLocation[loc]) > 0 ||
		len(support.sourceInventoryLabelsByLocation[loc]) > 0
}

func aggregateSupportLocationMatchesMemberLabels(location string, labels []string, support aggregateMemberSupportIndex) bool {
	if location == "" || len(labels) == 0 {
		return false
	}
	memberLabels := aggregateSupportLabels("", labels)
	return aggregateLocationEvidenceMatchesLabels(location, memberLabels, support.byLocation) ||
		aggregateToolLocationMatchesLabels(location, memberLabels, support.toolLinesByLocation) ||
		aggregateSourceInventoryLocationMatchesLabels(location, memberLabels, support.sourceInventoryLabelsByLocation)
}

func aggregateSourceInventoryLocationMatchesLabels(location string, labels []string, byLocation map[string][]string) bool {
	if location == "" || len(labels) == 0 {
		return false
	}
	for _, label := range byLocation[location] {
		if aggregateSupportLabelMatchesMember(label, labels) {
			return true
		}
	}
	return false
}

func aggregateSourceInventorySupportRefMatchesLabels(ref string, labels []string, bySupport map[string][]string) bool {
	if strings.TrimSpace(ref) == "" || len(labels) == 0 {
		return false
	}
	for _, label := range bySupport[strings.TrimSpace(ref)] {
		if aggregateSupportLabelMatchesMember(label, labels) {
			return true
		}
	}
	return false
}

func aggregateMemberReadFileSupportRef(member string, support aggregateMemberSupportIndex) (string, bool) {
	if ref, ok := aggregateMemberEmbeddedLocationReadFileSupportRef(member, support); ok {
		return ref, true
	}
	if ref, ok := aggregateDecoratedMemberReadFileSupportRef(member, support); ok {
		return ref, true
	}
	if _, _, isRelation := types.AnswerAggregateMemberRelationParts(member); !isRelation {
		if ref, ok := aggregateBareMemberReadFileSupportRef(member, support); ok {
			return ref, true
		}
	}
	left, _, ok := types.AnswerAggregateMemberRelationParts(member)
	if !ok || strings.TrimSpace(left) == "" {
		return "", false
	}
	labels := aggregateRelationRightSupportLabels(member)
	if len(labels) == 0 {
		return "", false
	}
	for _, line := range support.readFileLines {
		if !aggregateReadFilePathMatchesRelationLeft(line.File, left) {
			continue
		}
		if !aggregateReadFileLineSupportsLabels(line.Text, labels) {
			continue
		}
		loc := aggregateSupportLocationKey(line.File, line.Line)
		if loc == "" {
			continue
		}
		return "Member @ " + loc, true
	}
	return "", false
}

func aggregateMemberEmbeddedLocationReadFileSupportRef(member string, support aggregateMemberSupportIndex) (string, bool) {
	for _, loc := range aggregateMemberEmbeddedSourceLocations(member) {
		canon, ok := aggregateCanonicalSupportLocationByUniqueSuffixNoLabel(loc, support)
		if !ok {
			continue
		}
		return "Member @ " + canon, true
	}
	return "", false
}

func aggregateMemberEmbeddedSourceLocations(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range aggregateToolLocationPattern.FindAllString(member, -1) {
		raw = strings.Trim(strings.TrimSpace(raw), "`'\".,;，；。)")
		if raw == "" {
			continue
		}
		surface, ok := types.ParseAnswerSourceLocationSurface(raw)
		if !ok || strings.TrimSpace(surface.File) == "" || surface.LineStart <= 0 {
			continue
		}
		loc := aggregateSupportLocationKey(surface.File, surface.LineStart)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		out = append(out, loc)
	}
	return out
}

func aggregateCanonicalSupportLocationByUniqueSuffixNoLabel(location string, support aggregateMemberSupportIndex) (string, bool) {
	file, line, ok := aggregateSupportLocationParts(location)
	if !ok {
		return "", false
	}
	seen := map[string]bool{}
	var matches []string
	consider := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		candidateFile, candidateLine, ok := aggregateSupportLocationParts(candidate)
		if !ok || candidateLine != line {
			return
		}
		if !aggregateReadFilePathMatchesQualifier(candidateFile, file) {
			return
		}
		seen[candidate] = true
		matches = append(matches, candidate)
	}
	for candidate := range support.byLocation {
		consider(candidate)
	}
	for candidate := range support.toolLinesByLocation {
		consider(candidate)
	}
	for candidate := range support.sourceInventoryLabelsByLocation {
		consider(candidate)
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func aggregateBareMemberReadFileSupportRef(member string, support aggregateMemberSupportIndex) (string, bool) {
	labels := aggregateMemberDisplayCandidates(member)
	if len(labels) == 0 {
		return "", false
	}
	var matches []string
	seen := map[string]bool{}
	for _, line := range support.readFileLines {
		if !aggregateToolLineSupportsLabels(line.Text, labels) {
			continue
		}
		loc := aggregateSupportLocationKey(line.File, line.Line)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		matches = append(matches, loc)
	}
	if len(matches) != 1 {
		return "", false
	}
	return "Member @ " + matches[0], true
}

func aggregateDecoratedMemberReadFileSupportRef(member string, support aggregateMemberSupportIndex) (string, bool) {
	base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		return "", false
	}
	base = strings.TrimSpace(base)
	if base == "" || !types.IsCodeIdentitySurface(base) {
		return "", false
	}
	labels := aggregateMemberDisplayCandidates(base)
	if tail := types.NormalizedSurfaceSymbolTail(base); tail != "" {
		labels = append(labels, tail)
	}
	labels = dedupStringsPreserveOrder(labels)
	if ref, ok := aggregateDecoratedLocationQualifierSupportRef(base, qualifier, labels, support); ok {
		return ref, true
	}
	lineNo, ok := aggregateDecoratedLineQualifierNumber(qualifier)
	if !ok {
		return aggregateDecoratedBaseOnlyReadFileSupportRef(base, labels, support)
	}
	var matches []string
	seen := map[string]bool{}
	for _, line := range support.readFileLines {
		if line.Line != lineNo {
			continue
		}
		if !aggregateToolLineSupportsLabels(line.Text, labels) {
			continue
		}
		loc := aggregateSupportLocationKey(line.File, line.Line)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		matches = append(matches, loc)
	}
	if len(matches) != 1 {
		return "", false
	}
	return base + ": " + matches[0], true
}

func aggregateDecoratedBaseOnlyReadFileSupportRef(base string, labels []string, support aggregateMemberSupportIndex) (string, bool) {
	base = strings.TrimSpace(base)
	if base == "" || !types.IsCodeIdentitySurface(base) || len(labels) == 0 {
		return "", false
	}
	var matches []string
	seen := map[string]bool{}
	for _, line := range support.readFileLines {
		if !aggregateToolLineSupportsLabels(line.Text, labels) {
			continue
		}
		loc := aggregateSupportLocationKey(line.File, line.Line)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		matches = append(matches, loc)
	}
	if len(matches) != 1 {
		return "", false
	}
	return base + ": " + matches[0], true
}

func aggregateDecoratedLocationQualifierSupportRef(base, qualifier string, labels []string, support aggregateMemberSupportIndex) (string, bool) {
	surface, ok := types.ParseAnswerSourceLocationSurface(qualifier)
	if !ok || surface.File == "" || surface.LineStart <= 0 {
		return "", false
	}
	var matches []string
	seen := map[string]bool{}
	for _, line := range support.readFileLines {
		if line.Line != surface.LineStart {
			continue
		}
		if !aggregateReadFilePathMatchesQualifier(line.File, surface.File) {
			continue
		}
		if !aggregateToolLineSupportsLabels(line.Text, labels) {
			continue
		}
		loc := aggregateSupportLocationKey(line.File, line.Line)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		matches = append(matches, loc)
	}
	if len(matches) != 1 {
		return "", false
	}
	return base + ": " + matches[0], true
}

func aggregateReadFilePathMatchesQualifier(readFile, qualifierFile string) bool {
	readFile = strings.ToLower(strings.Trim(strings.TrimSpace(strings.ReplaceAll(readFile, `\`, `/`)), "/"))
	qualifierFile = strings.ToLower(strings.Trim(strings.TrimSpace(strings.ReplaceAll(qualifierFile, `\`, `/`)), "/"))
	if readFile == "" || qualifierFile == "" {
		return false
	}
	if readFile == qualifierFile {
		return true
	}
	if strings.Contains(qualifierFile, "/") {
		return strings.HasSuffix(readFile, "/"+qualifierFile)
	}
	return path.Base(readFile) == qualifierFile
}

func aggregateDecoratedLineQualifierNumber(qualifier string) (int, bool) {
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		return 0, false
	}
	m := aggregateDecoratedLineQualifierPattern.FindStringSubmatch(qualifier)
	if len(m) != 2 {
		return 0, false
	}
	line, err := strconv.Atoi(m[1])
	if err != nil || line <= 0 {
		return 0, false
	}
	return line, true
}

func aggregateRelationRightSupportLabels(member string) []string {
	labels := aggregateMemberDisplayCandidates(member)
	var out []string
	for _, label := range labels {
		_, right, ok := types.AnswerAggregateMemberRelationParts(label)
		if !ok {
			continue
		}
		right = strings.TrimSpace(right)
		if right == "" {
			continue
		}
		out = append(out, right)
		if tail := types.NormalizedSurfaceSymbolTail(right); tail != "" {
			out = append(out, tail)
		}
	}
	return dedupStringsPreserveOrder(out)
}

func aggregateMemberLocationSupportLabels(member string, labels []string) []string {
	out := append([]string(nil), labels...)
	out = append(out, aggregateRelationRightSupportLabels(member)...)
	return dedupStringsPreserveOrder(out)
}

func aggregateSupportLocationCompatibleWithMember(member string, location string) bool {
	left, _, ok := types.AnswerAggregateMemberRelationParts(member)
	if !ok || !types.IsCodeIdentitySurface(left) {
		return true
	}
	file := strings.TrimSpace(location)
	if idx := strings.LastIndex(file, ":"); idx >= 0 {
		file = file[:idx]
	}
	if file == "" {
		return true
	}
	return aggregateReadFilePathMatchesRelationLeft(file, left)
}

func aggregateReadFilePathMatchesRelationLeft(file, left string) bool {
	file = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)))
	if file == "" {
		return false
	}
	for _, candidate := range aggregateRelationLeftPathCandidates(left) {
		if candidate == "" {
			continue
		}
		if aggregatePathContainsSegmentSequence(file, candidate) {
			return true
		}
	}
	return false
}

func aggregateRelationLeftPathCandidates(left string) []string {
	left = strings.ToLower(strings.Trim(strings.TrimSpace(left), "`'\" "))
	if left == "" {
		return nil
	}
	var out []string
	add := func(v string) {
		v = strings.Trim(strings.TrimSpace(strings.ReplaceAll(v, `\`, `/`)), "/")
		if v != "" {
			out = append(out, v)
		}
	}
	add(left)
	add(strings.ReplaceAll(left, "::", "/"))
	add(strings.ReplaceAll(left, ".", "/"))
	add(strings.TrimPrefix(strings.ReplaceAll(left, "@", ""), "/"))
	if tail := types.NormalizedSurfaceSymbolTail(left); tail != "" && tail != left {
		add(tail)
	}
	return dedupStringsPreserveOrder(out)
}

func aggregatePathContainsSegmentSequence(file, candidate string) bool {
	candidate = strings.Trim(strings.TrimSpace(candidate), "/")
	if candidate == "" {
		return false
	}
	if strings.Contains(candidate, "/") {
		return strings.Contains("/"+file+"/", "/"+candidate+"/")
	}
	for _, segment := range strings.Split(file, "/") {
		base := strings.TrimSuffix(segment, path.Ext(segment))
		if segment == candidate || base == candidate {
			return true
		}
	}
	return false
}

func aggregateReadFileLineSupportsLabels(line string, labels []string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || aggregateReadFileLineIsComment(trimmed) {
		return false
	}
	if !aggregateReadFileLineLooksDefining(trimmed) {
		return false
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || types.AnswerSupportRefLabelIsGeneric(label) {
			continue
		}
		if types.AnswerCodeSurfaceAppearsInText(trimmed, label) {
			return true
		}
	}
	return false
}

func aggregateReadFileLineIsComment(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "--")
}

func aggregateReadFileLineLooksDefining(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"func ", "def ", "fn ", "function ", "fun ",
		"class ", "interface ", "enum ", "struct ", "trait ",
		"type ", "const ", "var ", "let ",
		"public ", "private ", "protected ", "export ",
		"impl ", "module ", "service ", "message ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(line, "(") && strings.Contains(line, "{")
}

func aggregateMemberDisplayCandidates(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	out := []string{member}
	out = append(out, types.AnswerAggregateMemberDisplayCandidates(member)...)
	if base, _, ok := types.AnswerAggregateDecoratedLabelParts(member); ok {
		base = strings.TrimSpace(base)
		if base != "" && types.IsCodeIdentitySurface(base) {
			out = append(out, base)
			if tail := types.NormalizedSurfaceSymbolTail(base); tail != "" {
				out = append(out, tail)
			}
		}
	}
	if base, ok := aggregateDecoratedSourceSupportBase(member); ok {
		out = append(out, base)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			prefix := strings.TrimSpace(member[:idx])
			if prefix != "" {
				out = append(out, prefix)
			}
		}
	}
	return dedupStringsPreserveOrder(out)
}

func aggregateDecoratedSourceSupportBase(member string) (string, bool) {
	base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		return "", false
	}
	base = strings.TrimSpace(base)
	if base == "" || !types.IsCodeIdentitySurface(base) {
		return "", false
	}
	if _, ok := aggregateDecoratedLineQualifierNumber(qualifier); ok {
		return base, true
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(qualifier); ok && surface.File != "" && surface.LineStart > 0 {
		return base, true
	}
	return "", false
}

func aggregateMemberNeedsTypedSupport(labels []string) bool {
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if types.IsCodeIdentitySurface(label) {
			return true
		}
		if _, ok := types.ParseAnswerSourceLocationSurface(label); ok {
			return true
		}
		if _, ok := types.ParseAnswerFilePathSurface(label); ok {
			return true
		}
	}
	return false
}

func aggregateMemberCoveredByAnswerSymbol(labels []string, syms map[string]types.AnswerSymbol) bool {
	if len(syms) == 0 {
		return false
	}
	for _, label := range labels {
		if _, ok := syms[aggregateMemberKey(label)]; ok {
			return true
		}
	}
	return false
}

func aggregateMemberCoveredByEvidenceLabels(labels []string, byLabel map[string][]types.EvidenceItem) bool {
	for _, label := range labels {
		key := aggregateMemberKey(label)
		if key == "" {
			continue
		}
		if len(byLabel[key]) > 0 {
			return true
		}
	}
	return false
}

func aggregateMemberSupportRefParts(raw string) (label string, location string, ok bool) {
	if label, loc, parsed := aggregateColonSupportRefMemberLocation(raw); parsed {
		return label, aggregateSupportLocationKey(loc.File, loc.LineStart), true
	}
	label, loc, parsed := types.ParseAnswerSupportRefMemberLocation(raw)
	if !parsed {
		return "", "", false
	}
	return label, aggregateSupportLocationKey(loc.File, loc.LineStart), true
}

// stripAggregateSupportRefDecorations peels model-authored DECORATIONS off a
// support_refs entry and reports whether anything was removed. SUPPREF-TOL
// (§29.104.13): decorated support_refs used to poison the whole
// emit_investigation_complete handoff — the h9 witness emit#4 (run
// 20260715-201207, keva-1 3.429ms + the correct conversion basis) was
// wholesale DOWNGRADED because one carried-forward ref wore an annotation the
// ref parsers cannot see through. The closed decoration set below is measured
// from the eval-log census, deliberately narrow (宁窄勿宽):
//
//   - outer whitespace;
//   - one symmetric wrapping backtick / double-quote / single-quote pair;
//   - one TRAILING parenthesized annotation — ASCII " (…)" (whitespace before
//     the opener required) or fullwidth "（…）" (CJK typography attaches
//     directly), inner and base both non-empty; census witnesses:
//     "attached_trace.txt:603 (caller=fscache_page_get_an+0xd0c/0x1b44)" and
//     "runtime_artifact:ae9bd2562e129fa7 (wakeup counts census)";
//   - one trailing colon-prose annotation "<artifact-path>: <prose>" — ONLY
//     when the head parses as a runtime-artifact path via the existing
//     types.RuntimeArtifactPathKind parser AND the tail does NOT itself parse
//     as a source location (a parseable tail means the legal labeled form
//     "label: file:line"; ambiguous shapes are never stripped — 剥后多义按失败
//     处理); census witness: "attached_trace.txt: wakeup_chain path".
//
// Ordinal prefixes and CJK 行-suffix line forms were NOT observed as
// support_refs decorations in the census and are deliberately excluded. The
// helper is a PREPROCESSOR: callers must retry the UNCHANGED existing parsers
// / lookup lanes on the stripped form, so a stripped ref behaves exactly like
// its directly-emitted bare form — never better (semantic equivalence), and a
// ref that still fails after stripping keeps today's behavior byte-identical
// (fail-open).
func stripAggregateSupportRefDecorations(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	out := trimmed
	for {
		// K2 stop rule: peel ONE decoration layer at a time and stop as
		// soon as the exposed core parses through the unchanged existing
		// ref parser. Without this, the fixpoint loop stripped THROUGH
		// legal cores — "`Foo (src/foo.go:10)`" lost its backticks and
		// then its payload-bearing paren group, leaving bare "Foo"
		// (fail-open turned into a mis-parse). One layer per iteration +
		// parse-stop makes over-stripping structurally impossible.
		if _, _, ok := aggregateMemberSupportRefParts(out); ok {
			break
		}
		next := strings.TrimSpace(stripAggregateSupportRefOneDecorationLayer(out))
		if next == out || next == "" {
			break
		}
		out = next
	}
	if out == "" || out == trimmed {
		return trimmed, false
	}
	return out, true
}

func stripAggregateSupportRefOneDecorationLayer(s string) string {
	if next := stripAggregateSupportRefSymmetricWrap(s); next != s {
		return next
	}
	if next := stripAggregateSupportRefTrailingParenAnnotation(s); next != s {
		return next
	}
	return stripAggregateSupportRefArtifactColonAnnotation(s)
}

func stripAggregateSupportRefSymmetricWrap(s string) string {
	for _, quote := range []string{"`", `"`, "'"} {
		if len(s) > 2*len(quote) && strings.HasPrefix(s, quote) && strings.HasSuffix(s, quote) {
			inner := strings.TrimSpace(s[len(quote) : len(s)-len(quote)])
			if inner != "" && !strings.Contains(inner, quote) {
				return inner
			}
		}
	}
	return s
}

func stripAggregateSupportRefTrailingParenAnnotation(s string) string {
	if base, ok := aggregateSupportRefTrailingGroupBase(s, " (", ")"); ok {
		return base
	}
	if base, ok := aggregateSupportRefTrailingGroupBase(s, "（", "）"); ok {
		return base
	}
	return s
}

func aggregateSupportRefTrailingGroupBase(s, open, close string) (string, bool) {
	if !strings.HasSuffix(s, close) {
		return "", false
	}
	idx := strings.LastIndex(s, open)
	if idx <= 0 {
		return "", false
	}
	base := strings.TrimSpace(s[:idx])
	inner := strings.TrimSpace(s[idx+len(open) : len(s)-len(close)])
	if base == "" || inner == "" {
		return "", false
	}
	return base, true
}

func stripAggregateSupportRefArtifactColonAnnotation(s string) string {
	idx := strings.Index(s, ": ")
	if idx <= 0 {
		return s
	}
	head := strings.TrimSpace(s[:idx])
	tail := strings.TrimSpace(s[idx+2:])
	if head == "" || tail == "" {
		return s
	}
	if types.RuntimeArtifactPathKind(head) == "" {
		return s
	}
	// Ambiguity guard: a tail that parses as a source location means the
	// whole string is the legal labeled form "label: file:line" — never a
	// decoration. Refuse to strip instead of guessing.
	if _, ok := types.ParseAnswerSourceLocationSurface(tail); ok {
		return s
	}
	return head
}

// aggregateMemberSupportRefPartsTolerant is the SUPPREF-TOL retry wrapper
// around aggregateMemberSupportRefParts: exact parse first (the successfully
// parsing surface is untouched), then one decoration-strip pass and a retry of
// the SAME parser. Used by the support_refs resolution / repair / positional
// lanes only; member-surface parsing and threshold counters deliberately stay
// on the exact parser.
func aggregateMemberSupportRefPartsTolerant(raw string) (label string, location string, ok bool) {
	if label, loc, parsed := aggregateMemberSupportRefParts(raw); parsed {
		return label, loc, true
	}
	stripped, changed := stripAggregateSupportRefDecorations(raw)
	if !changed {
		return "", "", false
	}
	return aggregateMemberSupportRefParts(stripped)
}

func aggregateColonSupportRefMemberLocation(raw string) (label string, location types.AnswerSourceLocationSurface, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", types.AnswerSourceLocationSurface{}, false
	}
	for _, sep := range []string{": ", ":\t"} {
		idx := strings.Index(raw, sep)
		if idx <= 0 || idx+len(sep) >= len(raw) {
			continue
		}
		label := strings.TrimSpace(raw[:idx])
		location := strings.TrimSpace(raw[idx+len(sep):])
		if label == "" || location == "" {
			continue
		}
		if surface, parsed := types.ParseAnswerSourceLocationSurface(location); parsed {
			return label, surface, true
		}
	}
	return "", types.AnswerSourceLocationSurface{}, false
}

func aggregateSupportLocationFromSurface(raw string) string {
	if surface, ok := types.ParseAnswerSourceLocationSurface(raw); ok {
		return aggregateSupportLocationKey(surface.File, surface.LineStart)
	}
	return ""
}

func aggregateSupportLocationKey(file string, line int) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if file == "" || line <= 0 {
		return ""
	}
	return file + ":" + fmt.Sprintf("%d", line)
}

func aggregateSupportLabelMatchesMember(label string, memberLabels []string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return true
	}
	if types.AnswerSupportRefLabelIsGeneric(label) {
		return true
	}
	want := aggregateMemberKey(label)
	if want == "" {
		return false
	}
	for _, member := range memberLabels {
		if aggregateMemberKey(member) == want {
			return true
		}
	}
	return false
}

func aggregateSupportLabels(label string, fallback []string) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return fallback
	}
	return dedupStringsPreserveOrder(append([]string{label}, fallback...))
}

func aggregateLocationEvidenceMatchesLabels(location string, labels []string, byLocation map[string][]types.EvidenceItem) bool {
	if location == "" {
		return false
	}
	for _, ev := range byLocation[location] {
		if aggregateEvidenceMatchesAnyLabel(ev, labels) {
			return true
		}
		if aggregateEvidenceTextContainsAnyLabel(ev, labels) {
			return true
		}
		if aggregateEvidenceValueLiteralContainsAnyLabel(ev, labels) {
			return true
		}
	}
	return false
}

func aggregateToolLocationMatchesLabels(location string, labels []string, byLocation map[string][]string) bool {
	if location == "" || len(labels) == 0 {
		return false
	}
	lines := byLocation[location]
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if aggregateToolLineSupportsLabels(line, labels) {
			return true
		}
	}
	return false
}

func aggregateSupportLocationNearbyToolValueMatchesLabels(location string, labels []string, support aggregateMemberSupportIndex) bool {
	file, line, ok := aggregateSupportLocationParts(location)
	if !ok || len(labels) == 0 || len(support.readFileLines) == 0 {
		return false
	}
	for _, toolLine := range support.readFileLines {
		if toolLine.Line < line || toolLine.Line > line+4 {
			continue
		}
		if !aggregateReadFilePathMatchesQualifier(toolLine.File, file) {
			continue
		}
		if aggregateQuotedValueTextContainsAnyLabel(toolLine.Text, labels) {
			return true
		}
	}
	return false
}

func aggregateToolLineSupportsLabels(line string, labels []string) bool {
	line = strings.TrimSpace(line)
	if line == "" || aggregateReadFileLineIsComment(line) {
		return false
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if types.AnswerSupportRefLabelIsGeneric(label) {
			continue
		}
		if types.AnswerCodeSurfaceAppearsInText(line, label) {
			return true
		}
	}
	return false
}

func aggregateEvidenceTextContainsAnyLabel(ev types.EvidenceItem, labels []string) bool {
	text := aggregateEvidenceMemberSupportText(ev)
	if text == "" {
		return false
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || types.AnswerSupportRefLabelIsGeneric(label) || !types.IsCodeIdentitySurface(label) {
			continue
		}
		if types.AnswerCodeSurfaceAppearsInText(text, label) {
			return true
		}
	}
	return false
}

func aggregateEvidenceMemberSupportText(ev types.EvidenceItem) string {
	text := strings.TrimSpace(ev.Snippet)
	if ev.LoadBearingSummary {
		text = strings.TrimSpace(text + "\n" + ev.Summary)
	}
	return text
}

func aggregateMemberValueLiteralCoveredByEvidence(labels []string, support aggregateMemberSupportIndex) bool {
	for _, ev := range support.valueLiteralEvidence {
		if aggregateEvidenceValueLiteralContainsAnyLabel(ev, labels) {
			return true
		}
	}
	return false
}

func aggregateEvidenceCanCarryValueLiteral(ev types.EvidenceItem) bool {
	if ev.GroundingStatus == types.GroundingUngrounded {
		return false
	}
	if strings.TrimSpace(ev.Snippet) != "" {
		return true
	}
	if strings.TrimSpace(ev.Summary) == "" {
		return false
	}
	if ev.LoadBearingSummary {
		return true
	}
	switch ev.AnchorKind {
	case types.AnchorReturn, types.AnchorAssignment, types.AnchorInitializer, types.AnchorStringLiteral:
		return true
	default:
		return false
	}
}

func aggregateEvidenceValueLiteralContainsAnyLabel(ev types.EvidenceItem, labels []string) bool {
	if ev.GroundingStatus == types.GroundingUngrounded {
		return false
	}
	if aggregateQuotedValueTextContainsAnyLabel(ev.Snippet, labels) {
		return true
	}
	if ev.LoadBearingSummary || ev.AnchorKind == types.AnchorReturn || ev.AnchorKind == types.AnchorAssignment ||
		ev.AnchorKind == types.AnchorInitializer || ev.AnchorKind == types.AnchorStringLiteral {
		return aggregateQuotedValueTextContainsAnyLabel(ev.Summary, labels)
	}
	return false
}

func aggregateQuotedValueTextContainsAnyLabel(text string, labels []string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || types.AnswerSupportRefLabelIsGeneric(label) {
			continue
		}
		for _, quote := range []string{`"`, `'`, "`"} {
			if strings.Contains(text, quote+label+quote) {
				return true
			}
			if strings.Contains(strings.ToLower(text), quote+strings.ToLower(label)+quote) {
				return true
			}
		}
	}
	return false
}

func aggregateEvidenceMatchesAnyLabel(ev types.EvidenceItem, labels []string) bool {
	for _, raw := range aggregateEvidenceMemberLabels(ev) {
		evKey := aggregateMemberKey(raw)
		if evKey == "" {
			continue
		}
		for _, label := range labels {
			if evKey == aggregateMemberKey(label) {
				return true
			}
		}
	}
	return false
}

func changeImpactPrincipalMemberSetDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	profile := rm.ChangeImpactProfile
	if profile == nil || !profile.Active() {
		return ""
	}
	switch profile.RequestedOutput {
	case types.ImpactOutputFiles, types.ImpactOutputSites:
	default:
		return ""
	}
	ok, invalid := changeImpactAggregatePrincipalMemberSetUsable(profile, aggregateFacts)
	if ok {
		return ""
	}
	if closure != nil {
		keywords := dedupStringsPreserveOrder(append(
			append([]string{strings.TrimSpace(profile.Target)}, rm.AnalyzerHints.ExactTargets...),
			append(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities...)...,
		))
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     completionMaterializationReadFiles(closure),
			Keywords:  keywords,
			Rationale: "change-impact file/site answers need a model-emitted aggregate_facts member_set so principal members do not get reconstructed from prose or nearby context",
			Origin:    "pre_complete.change_impact_member_set",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — change-impact principal member_set handoff is missing.\n\n")
	fmt.Fprintf(&b, "The active change-impact profile requests `%s` as the principal answer surface. Do not close with the affected files/sites only in thinking text, raw grep output, read-file memory, or the closure reason.\n\n", profile.RequestedOutput)
	if strings.TrimSpace(invalid) != "" {
		fmt.Fprintf(&b, "A `member_set` was present but is not usable as citable source-location principal data: %s\n\n", invalid)
	}
	b.WriteString("Include `aggregate_facts` with kind=`member_set` and `members` containing the complete affected principal set copied from verified search/read/command results. For requested_output=files, members may be file paths only when `support_refs` includes a citable file:line anchor for each file; file:line members are also accepted. For requested_output=sites, each member must be a file:line surface. Then re-call `emit_investigation_complete`.")
	return b.String()
}

func changeImpactAggregatePrincipalMemberSetUsable(profile *types.ChangeImpactProfile, facts []types.AnswerAggregateFact) (bool, string) {
	if profile == nil || !profile.Active() {
		return true, ""
	}
	sawMemberSet := false
	var invalid []string
	for factIdx, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet {
			continue
		}
		sawMemberSet = true
		if !types.AnswerAggregateFactRoleForRequest(fact, nil).IsPrincipal() {
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] role=%q is not principal_answer", factIdx, fact.Role))
			continue
		}
		if len(fact.Members) == 0 {
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] has no members", factIdx))
			continue
		}
		allUsable := true
		for _, member := range fact.Members {
			if changeImpactAggregatePrincipalMemberUsable(profile, fact, member) {
				continue
			}
			allUsable = false
			invalid = append(invalid, fmt.Sprintf("aggregate_facts[%d] member %q has no citable source surface for requested_output=%s", factIdx, member, profile.RequestedOutput))
		}
		if allUsable {
			return true, ""
		}
	}
	if !sawMemberSet {
		return false, ""
	}
	return false, strings.Join(invalid, "; ")
}

func changeImpactAggregatePrincipalMemberUsable(profile *types.ChangeImpactProfile, fact types.AnswerAggregateFact, member string) bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return false
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(member); ok && surface.File != "" && surface.LineStart > 0 {
		return true
	}
	if profile.RequestedOutput != types.ImpactOutputFiles {
		return false
	}
	file, ok := types.ParseAnswerFilePathSurface(member)
	if !ok || file == "" {
		return false
	}
	for _, raw := range fact.SupportRefs {
		surface, ok := types.ParseAnswerSourceLocationSurface(raw)
		if !ok || surface.File == "" || surface.LineStart <= 0 {
			continue
		}
		if normalizeAnswerLocationFileForChangeImpact(surface.File) == normalizeAnswerLocationFileForChangeImpact(file) {
			return true
		}
	}
	return false
}

func normalizeAnswerLocationFileForChangeImpact(raw string) string {
	return strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "`'\" ")
}

func changeImpactTargetStructuredHandoffDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure) string {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return changeImpactTargetStructuredHandoffDowngradeWithEvidence(ctx, closure, evidence)
}

func changeImpactTargetStructuredHandoffDowngradeWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	profile := ctx.AnalysisIR.RequestModel.ChangeImpactProfile
	if profile == nil || !profile.Active() || !profile.RequestedBroadAffectedSites() {
		return ""
	}
	gc := ground.BuildContext(ctx)
	sourceText := func(item types.EvidenceItem) string {
		return changeImpactReadTextForEvidence(gc, item)
	}
	gaps := types.ChangeImpactPrincipalTargetSurfaceGaps(profile, evidence, sourceText)
	if len(gaps) == 0 {
		return ""
	}
	files := make([]string, 0, len(gaps))
	for _, item := range gaps {
		if file := strings.TrimSpace(item.Source); file != "" {
			files = append(files, file)
		}
	}
	files = dedupStringsPreserveOrder(files)
	if closure != nil {
		rm := ctx.AnalysisIR.RequestModel
		keywords := dedupStringsPreserveOrder(append(
			append([]string{strings.TrimSpace(profile.Target)}, rm.AnalyzerHints.ExactTargets...),
			append(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities...)...,
		))
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     files,
			Keywords:  keywords,
			Rationale: "change-impact affected-site evidence names the target on the cited source line, but the model did not carry that target through structured evidence fields",
			Origin:    "pre_complete.change_impact_target_surface",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — change-impact target handoff is under-structured.\n\n")
	fmt.Fprintf(&b, "The active change-impact target is `%s`. The cited source line(s) below name that target, but the corresponding `emit_evidence` item(s) do not carry it in structured fields such as `anchor_symbol`, `subject`, `object`, `condition`, `snippet`, or `surface_terms`. Summary prose alone is not a principal-member handoff.\n\n", strings.TrimSpace(profile.Target))
	b.WriteString("Re-emit corrected grounded evidence for these already-read anchor lines, preserving the actual source line in `snippet` and putting the target/owner-qualified member path into a structured field. Then re-call `emit_investigation_complete`.\n")
	limit := minInt(len(gaps), 8)
	for i := 0; i < limit; i++ {
		item := gaps[i]
		fmt.Fprintf(&b, "  - %s — %s\n", supportLocationForCompletion(item), strings.TrimSpace(changeImpactReadTextForEvidence(gc, item)))
	}
	if len(gaps) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(gaps)-limit)
	}
	return b.String()
}

func changeImpactReadTextForEvidence(gc *ground.Context, item types.EvidenceItem) string {
	if gc == nil || strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return ""
	}
	lines := gc.LineIndex[item.Source]
	if len(lines) == 0 {
		return ""
	}
	start := item.LineStart
	end := item.LineEnd
	if end < start {
		end = start
	}
	if end-start > 6 {
		end = start + 6
	}
	var parts []string
	for line := start; line <= end; line++ {
		if text := strings.TrimSpace(lines[line]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func supportLocationForCompletion(item types.EvidenceItem) string {
	source := strings.TrimSpace(item.Source)
	if source == "" {
		return "-"
	}
	if item.LineStart > 0 {
		return fmt.Sprintf("%s:%d", source, item.LineStart)
	}
	return source
}

func principalSupportMaterializationDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if !principalSupportMaterializationRequired(ctx, view, aggregateFacts) {
		return ""
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil {
		return ""
	}
	if len(types.PrincipalSupportEvidenceItemsForFamily(view.Family, plan)) > 0 {
		return ""
	}
	if syms, _ := ctx.Mutable.EmittedAnswerSymbols(); len(syms) > 0 {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if closure != nil {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     completionMaterializationReadFiles(closure),
			Keywords:  dedupStringsPreserveOrder(append(append([]string{}, rm.AnalyzerHints.ExactTargets...), append(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities...)...)),
			Rationale: "the investigation has citable material, but none has been emitted into the typed principal support lane for this answer family",
			Origin:    "pre_complete.principal_support_materialization",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — typed principal evidence handoff is missing.\n\n")
	fmt.Fprintf(&b, "The resolved answer family is `%s`, which renders principal user-visible claims from typed support lanes. The current evidence buffer may contain raw reads or nearby context, but after facet binding it has 0 citable principal-support item(s).\n\n", view.Family)
	b.WriteString("Do not close from raw read_file / repo_map memory or closure prose alone. Stay on the already-read answer-bearing lines when possible, emit `emit_evidence` items whose `anchor_kind` and `anchor_symbol` point at the exact principal member / scalar / component / layer lines, then re-call `emit_investigation_complete`. If the answer is genuinely a derived count or scalar, carry it through `aggregate_facts` with the verified value instead of burying it in `reason` prose.")
	return b.String()
}

func principalRequiredTermHandoffDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.HasBoundedCategoryEnumerationMembers(rm) {
		return ""
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil || !view.NeedsEnumerationSlate() {
		return ""
	}
	terms := completionPrincipalHandoffTerms(ctx.AnalysisIR.AnswerContract, rm)
	if len(terms) == 0 {
		return ""
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil {
		return ""
	}
	support := completionPrincipalEvidencePool(ctx, types.PrincipalSupportEvidenceItemsForFamily(view.Family, plan))
	syms, _ := ctx.Mutable.EmittedAnswerSymbols()
	missing := completionMissingPrincipalHandoffTerms(terms, support, syms, aggregateFacts)
	if len(missing) == 0 {
		return ""
	}
	rmHints := rm.AnalyzerHints
	if closure != nil {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     completionMaterializationReadFiles(closure),
			Keywords:  completionMissingTermKeywords(missing, rmHints),
			Rationale: fmt.Sprintf("bounded enumeration has %d hard required term(s) without typed citable principal handoff", len(missing)),
			Origin:    "pre_complete.principal_required_term_handoff",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — required principal members lack typed handoff.\n\n")
	fmt.Fprintf(&b, "The current answer is a bounded enumeration with %d required answer term(s), but %d term(s) have no matching typed AnswerSymbol or citable principal-support evidence item yet.\n\n", len(terms), len(missing))
	b.WriteString("Do not close from raw grep / read_file prose, prior reasoning, or a count-only closure. Emit one grounded `emit_evidence` item per missing member using the exact source/log/trace label as `anchor_symbol`, `subject`, `object`, or `surface_terms`, with a real file:line citation from an already-read source when possible; or emit the completed symbol slate before retrying `emit_investigation_complete`.\n\n")
	b.WriteString("First missing terms:\n")
	limit := minInt(len(missing), 12)
	for i := 0; i < limit; i++ {
		fmt.Fprintf(&b, "  - %s (%s)\n", missing[i].Text, missing[i].Kind)
	}
	if len(missing) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(missing)-limit)
	}
	return b.String()
}

func completionPrincipalHandoffTerms(contract types.AnswerContract, rm types.RequestModel) []types.ContractTerm {
	all := types.NormalizedMustIncludeTerms(contract)
	if len(all) == 0 {
		return nil
	}
	out := make([]types.ContractTerm, 0, len(all))
	for _, term := range all {
		if completionAnalyzerEntityTermIsSoftForStructuredEnumeration(term, rm) {
			continue
		}
		if completionContractTermNeedsPrincipalHandoff(term) {
			out = append(out, term)
		}
	}
	return out
}

func completionAnalyzerEntityTermIsSoftForStructuredEnumeration(term types.ContractTerm, rm types.RequestModel) bool {
	if term.Source != types.ContractTermSourceAnalyzerEntity {
		return false
	}
	if !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	if rm.Predicates.IsRelationalLookup || rm.Predicates.IsScalarAnswer {
		return false
	}
	if rm.QuestionStructure().HasAnyObligation() {
		return true
	}
	kind := term.Kind
	if !kind.IsValid() {
		kind = types.InferContractTermKind(term.Text)
	}
	if kind == types.ContractTermFileStem {
		return true
	}
	if len(rm.SubTopics) >= 2 {
		return true
	}
	if len(rm.QuestionStructure().Buckets) >= 2 {
		return true
	}
	return false
}

func completionPrincipalEvidencePool(ctx *types.BusContext, support []types.EvidenceItem) []types.EvidenceItem {
	var out []types.EvidenceItem
	out = append(out, support...)
	if ctx != nil && ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			out = append(out, ta.EvidenceItems...)
		}
		out = append(out, ctx.Mutable.EmittedEvidence()...)
	}
	if len(out) < 2 {
		return out
	}
	seen := make(map[string]bool, len(out))
	deduped := out[:0]
	for _, item := range out {
		key := strings.ToLower(strings.TrimSpace(item.ID)) + "\x00" +
			strings.ToLower(strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))) + "\x00" +
			strconv.Itoa(item.LineStart) + "\x00" +
			strings.ToLower(strings.TrimSpace(item.AnchorSymbol)) + "\x00" +
			strings.ToLower(strings.TrimSpace(item.Subject)) + "\x00" +
			strings.ToLower(strings.TrimSpace(item.Object))
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, item)
	}
	return deduped
}

func completionContractTermNeedsPrincipalHandoff(term types.ContractTerm) bool {
	text := strings.TrimSpace(term.Text)
	if text == "" {
		return false
	}
	kind := term.Kind
	if !kind.IsValid() {
		kind = types.InferContractTermKind(text)
	}
	switch kind {
	case types.ContractTermSymbol, types.ContractTermToolName, types.ContractTermFileStem:
		return true
	default:
		return false
	}
}

func completionMissingPrincipalHandoffTerms(
	terms []types.ContractTerm,
	support []types.EvidenceItem,
	syms []types.AnswerSymbol,
	aggregateFacts []types.AnswerAggregateFact,
) []types.ContractTerm {
	if len(terms) == 0 {
		return nil
	}
	var missing []types.ContractTerm
	for _, term := range terms {
		if completionTermCoveredByAnswerSymbol(term, syms) ||
			completionTermCoveredBySupportEvidence(term, support) ||
			completionTermCoveredByAggregateFacts(term, aggregateFacts) {
			continue
		}
		if term.Source == types.ContractTermSourceAnalyzerEntity &&
			aggregateFactsContainMemberSet(aggregateFacts) {
			continue
		}
		missing = append(missing, term)
	}
	return missing
}

func completionTermCoveredByAggregateFacts(term types.ContractTerm, facts []types.AnswerAggregateFact) bool {
	needle := completionTermFold(term.Text)
	if needle == "" {
		return false
	}
	for _, fact := range facts {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		if !types.AnswerAggregateFactRoleForRequest(fact, nil).IsPrincipal() {
			continue
		}
		for _, member := range fact.Members {
			if completionAggregateMemberContainsTerm(member, needle) {
				return true
			}
		}
	}
	return false
}

func completionAggregateMemberContainsTerm(member, foldedNeedle string) bool {
	member = completionTermFold(member)
	if member == "" || foldedNeedle == "" {
		return false
	}
	if member == foldedNeedle {
		return true
	}
	for _, sep := range []string{" @ ", "\t", " | ", ":", " "} {
		if idx := strings.Index(member, sep); idx > 0 && strings.TrimSpace(member[:idx]) == foldedNeedle {
			return true
		}
	}
	return false
}

func completionTermCoveredByAnswerSymbol(term types.ContractTerm, syms []types.AnswerSymbol) bool {
	needle := completionTermFold(term.Text)
	if needle == "" {
		return false
	}
	for _, sym := range syms {
		if completionTermFold(sym.Name) == needle {
			return true
		}
	}
	return false
}

func completionTermCoveredBySupportEvidence(term types.ContractTerm, support []types.EvidenceItem) bool {
	needle := completionTermFold(term.Text)
	if needle == "" {
		return false
	}
	for _, item := range support {
		if item.GroundingStatus == types.GroundingUngrounded ||
			strings.TrimSpace(item.Source) == "" ||
			item.LineStart <= 0 {
			continue
		}
		for _, surface := range completionEvidenceStructuredSurfaces(item, term.Kind) {
			if completionTermFold(surface) == needle {
				return true
			}
		}
	}
	return false
}

func completionEvidenceStructuredSurfaces(item types.EvidenceItem, kind types.ContractTermKind) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		out = append(out, s)
	}
	for _, s := range []string{
		item.AnchorSymbol,
		item.OwnerSymbol,
		item.Subject,
		item.Object,
	} {
		add(s)
	}
	for _, term := range item.SurfaceTerms {
		add(term)
	}
	if kind == types.ContractTermFileStem {
		src := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		add(src)
		add(path.Dir(src))
		add(path.Base(src))
		ext := path.Ext(src)
		if ext != "" {
			add(strings.TrimSuffix(path.Base(src), ext))
		}
	}
	return dedupStringsPreserveOrder(out)
}

func completionTermFold(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if s == "" {
		return ""
	}
	return strings.ToLower(s)
}

func completionMissingTermKeywords(missing []types.ContractTerm, hints types.AnalyzerHints) []string {
	raw := make([]string, 0, len(missing)+len(hints.ExactTargets)+len(hints.PrimaryEntities)+len(hints.Entities))
	for _, term := range missing {
		raw = append(raw, term.Text)
	}
	raw = append(raw, hints.ExactTargets...)
	raw = append(raw, hints.PrimaryEntities...)
	raw = append(raw, hints.Entities...)
	return dedupStringsPreserveOrder(raw)
}

func principalSupportMaterializationRequired(ctx *types.BusContext, view *types.AnswerSemanticView, aggregateFacts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || view == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Predicates.IsHistoryLookup {
		return false
	}
	if w := ctx.Mutable.EvidenceFloorWaiver(); w.IsActive() {
		return false
	}
	if bundle := ctx.Mutable.LogTriage(); bundle != nil && bundle.IsExternalSource() {
		return false
	}
	if perf := ctx.Mutable.PerfTrace(); perf != nil && perf.IsExternalSource() {
		return false
	}
	if aggregateFactsCanCarryPrincipalAnswer(rm, view.Family, aggregateFacts) {
		return false
	}
	switch view.Family {
	case types.QFConfigPrecedence, types.QFRoleLookup, types.QFEnumeration, types.QFArchitecture:
		return true
	default:
		return false
	}
}

func aggregateFactsCanCarryPrincipalAnswer(rm types.RequestModel, family types.QuestionFamily, facts []types.AnswerAggregateFact) bool {
	if len(facts) == 0 {
		return false
	}
	for _, fact := range facts {
		switch fact.Kind {
		case types.AnswerAggregateScalar:
			if !types.AnswerAggregateFactRoleForRequest(fact, &rm).IsPrincipal() {
				continue
			}
			if family == types.QFConfigPrecedence || family == types.QFRoleLookup {
				return true
			}
		case types.AnswerAggregateMemberSet:
			if !types.AnswerAggregateFactRoleForRequest(fact, &rm).IsPrincipal() {
				continue
			}
			if family == types.QFEnumeration {
				return true
			}
		case types.AnswerAggregateTotalCount, types.AnswerAggregateUniqueCount,
			types.AnswerAggregateGroupedCount, types.AnswerAggregateBucketCount,
			types.AnswerAggregateExcluded:
			if !types.AnswerAggregateFactRoleForRequest(fact, &rm).IsPrincipal() {
				continue
			}
			if rm.Predicates.IsCountQuestion {
				return true
			}
		}
	}
	return false
}

func completionMaterializationReadFiles(closure *types.EvidenceClosure) []string {
	if closure == nil {
		return nil
	}
	readSet := closure.ReadSet()
	if len(readSet) == 0 {
		return nil
	}
	files := make([]string, 0, len(readSet))
	for file := range readSet {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file != "" {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	if len(files) > 6 {
		files = files[:6]
	}
	return files
}

func citationFloorStructuredReadRepair(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, needed int) (types.RepairDirective, bool) {
	if sourceInventoryMechanicalRowSetSatisfiesCitationFloor(ctx, aggregateFacts) {
		return types.RepairDirective{}, false
	}
	files := citationFloorStructuredReadFiles(ctx, aggregateFacts, needed)
	if len(files) == 0 {
		return types.RepairDirective{}, false
	}
	return types.RepairDirective{
		Kind:      types.RepairReadFile,
		Files:     files,
		Rationale: "citation floor is low, but the current structured handoff already names candidate file:line support refs; read these exact source files before broadening search",
		Origin:    "pre_complete.citation_floor_support_refs",
		Stage:     string(types.StageExplore),
	}, true
}

func citationFloorStructuredReadFiles(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, needed int) []string {
	limit := needed
	if limit < 1 {
		limit = 1
	}
	if limit < 2 {
		limit = 2
	}
	if limit > types.RequiredFileHintCoverageMax {
		limit = types.RequiredFileHintCoverageMax
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		if len(out) >= limit {
			return
		}
		for _, candidate := range citationFloorSupportRefFileCandidates(ctx, raw) {
			file, ok := citationFloorResolveRepoFile(ctx, candidate)
			if !ok || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, file)
			return
		}
	}
	for _, raw := range citationFloorAggregateSupportRefFiles(aggregateFacts) {
		add(raw)
	}
	if len(out) >= limit {
		return out
	}
	for _, raw := range citationFloorSourceInventoryCandidateFiles(ctx) {
		add(raw)
	}
	return out
}

func citationFloorAggregateSupportRefFiles(aggregateFacts []types.AnswerAggregateFact) []string {
	var principal []string
	var fallback []string
	for _, fact := range aggregateFacts {
		if len(fact.SupportRefs) == 0 {
			continue
		}
		target := &fallback
		if fact.Role == types.AnswerAggregateRolePrincipalAnswer {
			target = &principal
		}
		for _, ref := range fact.SupportRefs {
			file, _, ok := citationFloorSupportRefLocation(ref)
			if !ok {
				continue
			}
			*target = append(*target, file)
		}
	}
	return dedupStringsPreserveOrder(append(principal, fallback...))
}

func citationFloorSupportRefLocation(raw string) (string, int, bool) {
	_, loc, ok := aggregateMemberSupportRefParts(raw)
	if !ok {
		loc = aggregateSupportLocationFromSurface(raw)
	}
	if loc == "" {
		return "", 0, false
	}
	return parseAggregateSupportLocationKey(loc)
}

func citationFloorSourceInventoryCandidateFiles(ctx *types.BusContext) []string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	observation := ctx.Mutable.SourceInventoryObservation()
	if !observation.IsActive() {
		return nil
	}
	var files []string
	for _, set := range observation.Sets {
		if !set.Complete {
			continue
		}
		for _, member := range set.Members {
			if file := strings.TrimSpace(member.File); file != "" {
				files = append(files, file)
			}
			for _, attr := range member.Attributes {
				if file := strings.TrimSpace(attr.File); file != "" {
					files = append(files, file)
				}
			}
		}
	}
	return dedupStringsPreserveOrder(files)
}

func citationFloorSupportRefFileCandidates(ctx *types.BusContext, file string) []string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if file == "" {
		return nil
	}
	candidates := []string{file}
	for _, scope := range citationFloorStructuredScopePrefixes(ctx) {
		if scope == "" || strings.HasPrefix(file, scope+"/") {
			continue
		}
		candidates = append(candidates, path.Join(scope, file))
	}
	return dedupStringsPreserveOrder(candidates)
}

func citationFloorStructuredScopePrefixes(ctx *types.BusContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	var prefixes []string
	add := func(raw string) {
		if dir, ok := citationFloorResolveRepoDir(ctx, raw); ok {
			prefixes = append(prefixes, dir)
		}
	}
	hints := ctx.AnalysisIR.RequestModel.AnalyzerHints
	for _, group := range [][]string{
		hints.ExactTargets,
		hints.MentionedEntities,
		hints.PrimaryEntities,
		hints.Entities,
	} {
		for _, raw := range group {
			add(raw)
		}
	}
	if ctx.Mutable != nil {
		for _, result := range aggregateSupportToolResults(ctx) {
			add(sourceInventoryListFilesToolScope(ctx, result))
		}
		observation := ctx.Mutable.SourceInventoryObservation()
		if observation.IsActive() {
			for _, scope := range observation.Scopes {
				add(scope)
			}
		}
	}
	return dedupStringsPreserveOrder(prefixes)
}

func citationFloorResolveRepoDir(ctx *types.BusContext, raw string) (string, bool) {
	canon := citationFloorCanonicalRepoPath(ctx, raw)
	if canon == "" {
		return "", false
	}
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return canon, true
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, filepath.FromSlash(canon)))
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return canon, true
	}
	dir := path.Dir(canon)
	if dir == "." || dir == "/" {
		return "", false
	}
	return dir, true
}

func citationFloorResolveRepoFile(ctx *types.BusContext, raw string) (string, bool) {
	canon := citationFloorCanonicalRepoPath(ctx, raw)
	if canon == "" {
		return "", false
	}
	qualified, ok := types.QualifyForcedReadSeedPath(ctx, canon)
	if !ok {
		return "", false
	}
	canon = citationFloorCanonicalRepoPath(ctx, qualified)
	if canon == "" {
		return "", false
	}
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return canon, true
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, filepath.FromSlash(canon)))
	if err != nil || info.IsDir() {
		return "", false
	}
	return canon, true
}

func citationFloorCanonicalRepoPath(ctx *types.BusContext, raw string) string {
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	canon := types.CanonicalRequiredFileHintPath(raw, repoRoot)
	canon = strings.Trim(strings.ReplaceAll(canon, `\`, `/`), "/")
	if canon == "" || canon == ".." || strings.HasPrefix(canon, "../") || path.IsAbs(canon) {
		return ""
	}
	return canon
}

func enrichCompletionAggregateFactsWithDeterministicCount(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if ctx == nil || ctx.AnalysisIR == nil {
		return facts
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion || rm.Predicates.IsHistoryLookup {
		return facts
	}
	if aggregateFactsContainCountAnswer(facts) || aggregateFactsContainDeterministicCountScalar(facts) {
		return facts
	}
	value, ok := deterministicCountToolResultValue(ctx)
	if !ok {
		return facts
	}
	return appendDeterministicCountAggregateFact(facts, value, deterministicCountAggregateLabel(ctx, false), "system_deterministic_count_enrichment", "exec_command", nil)
}

func enrichCompletionAggregateFactsWithDeterministicHistoryCount(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFact {
	if ctx == nil || ctx.AnalysisIR == nil {
		return facts
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion || !rm.Predicates.IsHistoryLookup {
		return facts
	}
	if aggregateFactsContainCountAnswer(facts) || aggregateFactsContainDeterministicCountScalar(facts) {
		return facts
	}
	value, proofSource, ok := deterministicHistoryCountToolResult(ctx)
	if !ok {
		return facts
	}
	return appendDeterministicCountAggregateFact(facts, value, deterministicCountAggregateLabel(ctx, true), "system_deterministic_history_count_enrichment", proofSource, []types.AnswerAggregateDimension{
		{Name: "measurement_kind", Value: "vcs_history_count"},
	})
}

func deterministicCountAggregateLabel(ctx *types.BusContext, history bool) string {
	zh := true
	if ctx != nil && ctx.AnalysisIR != nil {
		lang := strings.ToLower(strings.TrimSpace(ctx.AnalysisIR.RequestModel.Language))
		zh = lang == "" || strings.HasPrefix(lang, "zh")
	}
	if zh {
		if history {
			return "系统验证的历史计数结果"
		}
		return "系统验证的计数结果"
	}
	if history {
		return "system-verified history count result"
	}
	return "system-verified count result"
}

func appendDeterministicCountAggregateFact(facts []types.AnswerAggregateFact, value int, label string, provenance string, proofSource string, extraDims []types.AnswerAggregateDimension) []types.AnswerAggregateFact {
	out := cloneCompletionAggregateFacts(facts)
	if strings.TrimSpace(proofSource) == "" {
		proofSource = "exec_command"
	}
	dims := []types.AnswerAggregateDimension{
		{Name: "origin", Value: deterministicCountAggregateOrigin(proofSource)},
		{Name: "proof_source", Value: proofSource},
		{Name: "answer_axis", Value: "count"},
	}
	dims = append(dims, extraDims...)
	out = append(out, types.AnswerAggregateFact{
		Kind:       types.AnswerAggregateScalar,
		Label:      label,
		Value:      strconv.Itoa(value),
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: provenance,
		Dimensions: dims,
	})
	return out
}

func deterministicCountAggregateOrigin(proofSource string) string {
	switch strings.ToLower(strings.TrimSpace(proofSource)) {
	case "git_history_search", "git_log", "git_show", "exec_command_git_history":
		return string(types.AnswerEvidenceOriginVCSMetadata)
	case "git_diff":
		return string(types.AnswerEvidenceOriginVCSDiff)
	default:
		return string(types.AnswerEvidenceOriginCommandMeasurement)
	}
}

func evidenceFloorWaiverIsPureVCSHistoryMisuse(ctx *types.BusContext, _ types.EvidenceFloorWaiverReason, artifactAttached bool) bool {
	if artifactAttached || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.RequestModel.Predicates.IsHistoryLookup
}

func aggregateFactsContainDeterministicCountScalar(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar {
			continue
		}
		for _, dim := range fact.Dimensions {
			if strings.EqualFold(strings.TrimSpace(dim.Name), "answer_axis") &&
				strings.EqualFold(strings.TrimSpace(dim.Value), "count") {
				return true
			}
		}
	}
	return false
}

func hasDeterministicCountToolResult(ctx *types.BusContext) bool {
	_, ok := deterministicCountToolResultValue(ctx)
	return ok
}

func deterministicCountToolResultValue(ctx *types.BusContext) (int, bool) {
	var value int
	found := false
	for _, tr := range deterministicCountToolResults(ctx) {
		if !tr.Success {
			continue
		}
		measurement := tr.CommandMeasurement
		if measurement == nil ||
			measurement.Kind != types.ToolCommandMeasurementKindCount ||
			measurement.History ||
			measurement.Origin != types.AnswerEvidenceOriginCommandMeasurement {
			continue
		}
		n := measurement.Value
		if found && n != value {
			return 0, false
		}
		value = n
		found = true
	}
	return value, found
}

func deterministicHistoryCountToolResultValue(ctx *types.BusContext) (int, bool) {
	value, _, ok := deterministicHistoryCountToolResult(ctx)
	return value, ok
}

func deterministicHistoryCountToolResult(ctx *types.BusContext) (int, string, bool) {
	var value int
	var source string
	found := false
	for _, tr := range deterministicCountToolResults(ctx) {
		if !tr.Success {
			continue
		}
		measurement := tr.CommandMeasurement
		if measurement == nil ||
			measurement.Kind != types.ToolCommandMeasurementKindCount ||
			!measurement.History {
			continue
		}
		candidateSource := strings.TrimSpace(measurement.ProofSource)
		if candidateSource == "" {
			candidateSource = tr.ToolName
		}
		switch candidateSource {
		case "exec_command_git_history", "git_history_search", "git_log", "git_show":
		default:
			continue
		}
		n := measurement.Value
		if found && n != value {
			return 0, "", false
		}
		value = n
		source = candidateSource
		found = true
	}
	return value, source, found
}

func deterministicHistoryCountProofInteger(summary string) (int, bool) {
	for _, line := range deterministicCountPayloadLines(summary) {
		match := deterministicHistoryCountLineRe.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func deterministicCountPayloadLines(summary string) []string {
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	rawLines := strings.Split(summary, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[exec_command:") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func deterministicCountToolResults(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil {
		return nil
	}
	var out []types.ToolResult
	out = append(out, ctx.ToolResults...)
	if ctx.Mutable != nil {
		out = append(out, ctx.Mutable.DispatchToolResults()...)
	}
	return out
}

type fieldValueCountTarget struct {
	Full    string
	Owner   string
	Field   string
	Literal string
}

type fieldValueCountCandidate struct {
	File string
	Line int
	Text string
}

func fieldValueCountCoverageDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure) string {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return fieldValueCountCoverageDowngradeWithEvidence(ctx, closure, evidence)
}

func fieldValueCountCoverageDowngradeWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) string {
	target, ok := fieldValueCountTargetFromContext(ctx)
	if !ok {
		return ""
	}
	candidates := scanRepoForFieldValueCountCandidates(ctx.RepoRoot, target)
	if len(candidates) == 0 {
		return ""
	}
	var missing []fieldValueCountCandidate
	for _, c := range candidates {
		if fieldValueCountCandidateCovered(c, closure, evidence) {
			continue
		}
		missing = append(missing, c)
	}
	if len(missing) == 0 {
		return ""
	}
	if closure != nil {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  dedupFieldValueSearchKeywords(target),
			Rationale: fmt.Sprintf("field/value count for %s=%s has %d production candidate line(s) not yet read or evidenced; broaden beyond selector assignment syntax to aggregate/object/designated-initializer forms", target.Full, target.Literal, len(missing)),
			Origin:    "pre_complete.field_value_count_coverage",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — field/value count coverage is incomplete.\n\n")
	fmt.Fprintf(&b, "The request is a typed count/scalar lookup for `%s = %s`. Direct selector searches only cover one syntax family; production code may also set the same field through aggregate/object/designated-initializer forms such as `%s: %s`.\n\n",
		target.Full, target.Literal, target.Field, target.Literal)
	b.WriteString("Read or emit evidence for these production candidate line(s) before closing:\n")
	const maxMissing = 8
	for i, c := range missing {
		if i >= maxMissing {
			fmt.Fprintf(&b, "  ... and %d more candidate line(s)\n", len(missing)-maxMissing)
			break
		}
		fmt.Fprintf(&b, "  - %s:%d — %s\n", c.File, c.Line, c.Text)
	}
	b.WriteString("\nUse a deterministic broader search over both the full target and the leaf field/value surface, then classify every candidate as production assignment, non-production, comment, or unrelated before re-calling emit_investigation_complete.")
	return b.String()
}

func fieldValueCountTargetFromContext(ctx *types.BusContext) (fieldValueCountTarget, bool) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return fieldValueCountTarget{}, false
	}
	profile := ctx.AnalysisIR.RequestModel.FieldValueProfile
	if profile == nil || !profile.Active() {
		return fieldValueCountTarget{}, false
	}
	full, owner, field := profile.Target, profile.Owner, profile.Field
	if full == "" || owner == "" || field == "" {
		var ok bool
		full, owner, field, ok = types.ParseFieldValueTarget(profile.Target)
		if !ok {
			return fieldValueCountTarget{}, false
		}
	}
	literal := strings.TrimSpace(profile.Literal)
	if literal == "" {
		return fieldValueCountTarget{}, false
	}
	return fieldValueCountTarget{
		Full:    full,
		Owner:   owner,
		Field:   field,
		Literal: literal,
	}, true
}

func scanRepoForFieldValueCountCandidates(repoRoot string, target fieldValueCountTarget) []fieldValueCountCandidate {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || target.Field == "" || target.Literal == "" {
		return nil
	}
	linePattern := regexp.MustCompile(`(^|[^A-Za-z0-9_])\.?` + regexp.QuoteMeta(target.Field) + `\s*(?::|=|=>)\s*` + regexp.QuoteMeta(target.Literal) + `([^A-Za-z0-9_]|$)`)
	var out []fieldValueCountCandidate
	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relOK := fieldValueRelPath(repoRoot, path)
		if d.IsDir() {
			if !relOK || rel == "." {
				return nil
			}
			if fieldValueSkipDir(rel, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !relOK || !fieldValueProductionSourcePath(rel) {
			return nil
		}
		content, readErr := width.ReadFileBounded(path, 0)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
		for i, line := range lines {
			if fieldValueCommentOnlyLine(line) || !linePattern.MatchString(line) {
				continue
			}
			if !fieldValueOwnerNearby(lines, i, target) {
				continue
			}
			out = append(out, fieldValueCountCandidate{
				File: rel,
				Line: i + 1,
				Text: strings.TrimSpace(line),
			})
			if len(out) >= 64 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File == out[j].File {
			return out[i].Line < out[j].Line
		}
		return out[i].File < out[j].File
	})
	return out
}

func fieldValueRelPath(repoRoot, path string) (string, bool) {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		rel = "."
	}
	return rel, true
}

func fieldValueSkipDir(rel, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if DirNameMatchesExcludePattern(name, ExcludeDirPatternsAnyLevel) {
		return true
	}
	parts := strings.Split(strings.Trim(rel, "/"), "/")
	return len(parts) == 1 && ExcludeDirsRootOnlySet[parts[0]]
}

func fieldValueProductionSourcePath(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(rel)))
	if lower == "" || types.LooksLikeTestFilePath(lower) {
		return false
	}
	if strings.Contains(lower, "/testdata/") || strings.HasPrefix(lower, "testdata/") ||
		strings.Contains(lower, "/fixtures/") || strings.HasPrefix(lower, "fixtures/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(lower))
	switch ext {
	case ".go", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx",
		".m", ".mm", ".java", ".kt", ".kts", ".js", ".jsx", ".mjs", ".cjs",
		".ts", ".tsx", ".ets", ".py", ".rb", ".rs", ".swift", ".lua", ".cs",
		".php", ".scala", ".cj", ".json", ".yaml", ".yml", ".toml":
		return true
	default:
		return false
	}
}

func fieldValueCommentOnlyLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" ||
		strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "--")
}

func fieldValueOwnerNearby(lines []string, index int, target fieldValueCountTarget) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	if strings.Contains(lines[index], target.Full) || strings.Contains(lines[index], target.Owner) {
		return true
	}
	start := index - 4
	if start < 0 {
		start = 0
	}
	end := index + 2
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for i := start; i <= end; i++ {
		if fieldValueCommentOnlyLine(lines[i]) {
			continue
		}
		if strings.Contains(lines[i], target.Owner) || strings.Contains(lines[i], target.Full) {
			return true
		}
	}
	return false
}

func fieldValueCountCandidateCovered(c fieldValueCountCandidate, closure *types.EvidenceClosure, evidence []types.EvidenceItem) bool {
	if closure != nil && closure.HasReadLine(c.File, c.Line) {
		return true
	}
	for _, ev := range evidence {
		if filepath.ToSlash(strings.TrimSpace(ev.Source)) != c.File {
			continue
		}
		start := ev.LineStart
		end := ev.LineEnd
		if end <= 0 {
			end = start
		}
		if start > 0 && c.Line >= start && c.Line <= end {
			return true
		}
	}
	return false
}

func dedupFieldValueSearchKeywords(target fieldValueCountTarget) []string {
	return dedupStringsPreserveOrder([]string{target.Full, target.Owner, target.Field, target.Literal})
}

func dedupStringsPreserveOrder(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func explanationAnchorBackboneDowngrade(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if ctx.Mutable != nil && ctx.Mutable.WriteExplorationRequest() != nil {
		return ""
	}
	if !types.IRRequiresAnchorSkeleton(ctx.AnalysisIR) {
		return ""
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil {
		return ""
	}
	anchors := plan.ExplanationAnchorBackbone
	missing := plan.ExplanationAnchorMissingTopics
	if len(missing) == 0 {
		return ""
	}
	if ctx.Mutable != nil {
		keywords := make([]string, 0, len(ctx.AnalysisIR.RequestModel.SubTopics))
		for _, topic := range ctx.AnalysisIR.RequestModel.SubTopics {
			keywords = append(keywords, topic.Entities...)
		}
		ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  keywords,
			Rationale: fmt.Sprintf("multi-topic explanation still lacks one grounded anchor per sub-topic (%d/%d covered); read the exact owner/definition line for each missing sub-topic before re-calling emit_investigation_complete", len(anchors), len(ctx.AnalysisIR.RequestModel.SubTopics)),
			Origin:    "pre_complete.explanation_anchor_skeleton",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — multi-topic explanation still lacks one grounded anchor per sub-topic.\n\n")
	total := len(anchors) + len(missing)
	if total == 0 {
		total = len(ctx.AnalysisIR.RequestModel.SubTopics)
	}
	fmt.Fprintf(&b, "Current grounded anchor coverage: %d / %d sub-topics.\n", len(anchors), total)
	b.WriteString("Missing sub-topics:\n")
	for _, topic := range missing {
		fmt.Fprintf(&b, "  - %s\n", topic)
	}
	b.WriteString("\nRead the exact definition/owner line for the missing sub-topic(s), emit grounded evidence from those lines, then re-call emit_investigation_complete.")
	return b.String()
}

func capabilitySurfacePlan(ctx *types.BusContext) *types.AnswerSurfacePlan {
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil || plan.CapabilitySurface == nil {
		return nil
	}
	return plan
}

func capabilitySurfaceUnreadAuthorityFiles(plan *types.AnswerSurfacePlan, closure *types.EvidenceClosure, repoRoot string) []string {
	if plan == nil || closure == nil || len(plan.CapabilityAuthorityFiles) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(plan.CapabilityAuthorityFiles))
	var unread []string
	for _, raw := range plan.CapabilityAuthorityFiles {
		canon := phase1UnreadCanonPath(raw, repoRoot)
		if canon == "" || seen[canon] || closure.HasRead(canon) {
			continue
		}
		seen[canon] = true
		unread = append(unread, canon)
	}
	return unread
}

func refreshClosureReadSnapshot(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return
	}
	// P4-cross-sub-repo (Sc 6): wire ground.CrossRepoOracle.
	if oracleSource, ok := ctx.MultiGraph.(interface {
		Oracle() types.SymbolOracle
	}); ok && oracleSource != nil {
		ground.SetCrossRepoOracle(oracleSource.Oracle())
	} else {
		ground.SetCrossRepoOracle(nil)
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
		// Even with no ground LineIndex we still want to refresh the
		// per-file ranges in case the dispatch buffer carries reads
		// whose banners weren't ingested by ground (e.g. malformed
		// path canonicalisation). The coverage refresh below is its
		// own canonical walker; nil-safe on empty history.
		ground.RefreshClosureCoverage(ctx, closure)
		return
	}
	readSet := closure.ReadSet()
	if len(readSet) == 0 {
		readSet = make(map[string]bool, len(gc.LineIndex))
	}
	changed := false
	for file := range gc.LineIndex {
		if file == "" || readSet[file] || closure.HasRead(file) {
			continue
		}
		readSet[file] = true
		changed = true
	}
	if changed {
		closure.IngestEvidenceReducerInput(types.EvidenceReducerInput{
			Class:          types.EvidenceReducerInputStageCoverageSnapshot,
			ReadSet:        readSet,
			ReplaceReadSet: true,
		}, ctx.RepoRoot)
	}
	// Sync per-file ranges + totals from the same live history. Every
	// downstream consumer of MergedReadLines / CoverageRatio /
	// HasFullyRead — including the multi-path symbol-anchored gate
	// (applyMultiPathAnchorChecks → multipath.EvaluateAnchor's
	// HasFullyRead bypass) and any other per-file coverage check —
	// would otherwise read a snapshot frozen at the END of the
	// previous dispatch's ParseOutput, so a mid-dispatch read_file
	// is invisible to the next emit_investigation_complete attempt.
	// Historical failure mode this fix targets: pre-2026-05-01 the
	// "covered 645 / 4368 lines (15%)" hint repeated across four
	// iterations while the LLM had read past the suggested range
	// three times.
	ground.RefreshClosureCoverage(ctx, closure)
}

func currentSourceForcedReadGatesApply(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return true
	}
	// Source-inventory completion can still open the phase1 gate even when
	// repo-wide prescan hints are only soft. The actual pending-read queue below
	// still requires a precise source-inventory scope, so broad ranker noise
	// cannot become a hard completion blocker here.
	if types.SourceInventoryRequiredFileCoverageShape(ctx.AnalysisIR.RequestModel) {
		return true
	}
	if authority := runtimeSourceAnswerAuthorityForCompletion(ctx); runtimeSourceAuthorityAppliesToCompletionLanding(authority) {
		if authority.CanHardBlockCompletion {
			return true
		}
		if authority.AllowsProceedWithoutAdditionalCurrentSourceRead() {
			return false
		}
	}
	if runtimeArtifactGroundingBypassAllowed(ctx) {
		return false
	}
	rm := types.RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if !types.RuntimeSourceRequestHasExternalObservationCarrier(rm, ctx.TurnRouteHint) {
		return true
	}
	return types.RuntimeSourceRequestCurrentSourceRequirementPrecision(
		rm,
		ctx.TurnRouteHint,
	) == types.RuntimeSourceRequirementPrecise
}

func currentSourceLaneCoverageDowngrade(ctx *types.BusContext, preflight completionPreflightView) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	fatalZeroWitness := false
	if currentSourceLaneRuntimeArtifactCarrier(ctx) {
		authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
		fatalZeroWitness = zeroWitnessExplicitSourceDemandFatalClass(ctx, preflight, authority)
		if !fatalZeroWitness && authority.Active && authority.NeedsCurrentSourceEvidence && !authority.CanHardBlockCompletion {
			recordSoftCurrentSourceCompletionCaveat(ctx, authority)
			return ""
		}
	}
	if !fatalZeroWitness {
		if !currentSourceForcedReadGatesApply(ctx) || !currentSourceLaneRuntimeArtifactCarrier(ctx) {
			return ""
		}
		if currentSourceLaneHasCoverage(ctx, preflight) {
			return ""
		}
	} else {
		logging.Info("[emit_investigation_complete] zero-witness explicit-source-demand fatal class: external-artifact waiver/soft-caveat step-aside does not cover the current-source lane; issuing bounded source push-back")
	}
	if ctx.Mutable != nil {
		rm := ctx.AnalysisIR.RequestModel
		ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  dedupStringsPreserveOrder(append(append([]string{}, rm.AnalyzerHints.ExactTargets...), append(rm.AnalyzerHints.Entities, rm.AnalyzerHints.Keywords...)...)),
			Rationale: "typed current-source lane is required but no current-source evidence or read coverage exists; locate and read the implementation owner before completion",
			Origin:    "pre_complete.current_source_lane_coverage",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — typed current-source lane is required but no current-source evidence/read coverage exists.\n\n")
	if fatalZeroWitness {
		if quote := types.RuntimeSourceAuthorityRequestModelFromBusContext(ctx).FirstVerbatimCurrentSourceDemandQuote(); quote != "" {
			fmt.Fprintf(&b, "The request explicitly asks for a current-source explanation (%q). ", quote)
		}
		b.WriteString("External log/trace observations — including an accepted evidence_floor_waiver — cover only the artifact lane; they cannot discharge that explicit current-source demand while zero source files have been read. ")
	}
	b.WriteString("The runtime/external observation lane can be preserved as context, but it cannot close this investigation by itself. ")
	b.WriteString("Run a focused source localization step (`repo_map`, files-only search, or `read_file` on a bounded owner path), then emit grounded current-source evidence before retrying `emit_investigation_complete`.\n")
	return b.String()
}

// zeroWitnessExplicitSourceDemandFatalClass is the EVALFIX-2G fatal-class
// conjunction (eval/boundary_regression_adjudication_20260730.md §5.1): a run
// must NOT complete source-optional when ALL of these PRECISE signals conjoin:
//
//  1. the unified route+request authority has already classified the
//     current-source lane as required (so an explicit artifact-only
//     current_source=optional route cannot be bypassed here);
//  2. the typed CurrentSourceExplanationProfile is Active AND carries a
//     verbatim current-request quote (schema-validated boolean + verbatim
//     substring — the same predicate family EVALFIX-1's validateExactTargets
//     uses; no prose regex scanning at gate time);
//  3. an external-observation carrier is present — established by the caller's
//     currentSourceLaneRuntimeArtifactCarrier guard (the same typed carrier
//     lane the evidence_floor_waiver / system-detected external-source
//     bypasses serve);
//  4. the deterministic current-source witness count is zero (proof-lane 口径:
//     read_file coverage, non-log evidence anchors, and ledger current-source
//     records; model prose never counts).
//
// When the conjunction holds, the soft-caveat and forced-read-gate step-asides
// in currentSourceLaneCoverageDowngrade stand down and the EXISTING bounded
// push-back fires instead: RepairExpandSearch + localization teaching, bounded
// by the DowngradeLaneCurrentSourceLane low-delta convergence — after
// downgradeConvergenceHardThreshold no-progress attempts the lane falls
// through and completes with a typed caveat (disclose-and-proceed), so this is
// a bounded reopen, never an unbounded hard wall (typed escape preserved).
//
// 完成门权属模型: only this zero-witness fatal class may gate; every non-fatal
// shape keeps today's behavior unchanged (soft obligations still downgrade to
// a caveat). The external-source waiver keeps exempting the ARTIFACT lane's
// repo-grounding (citation floor) — this conjunction merely stops the waiver
// from silencing the current-source lane too (adjudication §5.2): once one
// deterministic source witness lands, signal 3 breaks and the waiver's
// artifact-lane exemption completes the run exactly as before.
func zeroWitnessExplicitSourceDemandFatalClass(ctx *types.BusContext, preflight completionPreflightView, authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	// A zero-current-source repo can never satisfy the demand; pushing back
	// would only burn the bounded retries on a structurally unsatisfiable
	// obligation (mirrors runtimeArtifactGroundingBypassAllowed).
	if ctx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		return false
	}
	rm := types.RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if rm == nil {
		return false
	}
	// The user's explicit "don't analyze code" boundary always wins over the
	// explanation-profile demand (same precedence the precision classifier
	// runtimeSourceAuthorityPreciseCurrentSourceRequirement applies).
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	// Authority is the single route+request compiler. In particular, an
	// explicit external-observation route with current_source=optional must
	// not be reopened solely because the analyzer copied an artifact-scope
	// phrase into CurrentSourceExplanationProfile. Route-required mixed turns
	// and precise path/line obligations still publish CurrentSourceRequired.
	if !authority.CurrentSourceRequired {
		return false
	}
	// Signal 2: typed Active profile with a verbatim request quote.
	if !rm.CurrentSourceExplanationHasVerbatimRequestQuote() {
		return false
	}
	// Signal 4: zero deterministic current-source witnesses — both the lane's
	// own coverage walk (read_file coverage + non-log evidence anchors) and
	// the ledger-backed typed satisfaction count must be empty.
	if currentSourceLaneHasCoverage(ctx, preflight) {
		return false
	}
	return !authority.CurrentSourceSatisfied
}

func recordSoftCurrentSourceCompletionCaveat(ctx *types.BusContext, authority types.RuntimeSourceAnswerAuthoritySnapshot) {
	if ctx == nil || ctx.Mutable == nil || ctx.Mutable.EvidenceClosure() == nil {
		return
	}
	if !authority.CanDowngradeToCaveat {
		return
	}
	ctx.Mutable.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLaneCurrentSourceLane,
		ReasonCode: types.ProgressReasonConverged,
		Reason:     "soft current-source obligation was not backed by a precise source anchor; completed with runtime observation caveat instead of reopening broad source exploration",
	})
}

func currentSourceLaneRuntimeArtifactCarrier(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if authority := runtimeSourceAnswerAuthorityForCompletion(ctx); runtimeSourceAuthorityAppliesToCompletionLanding(authority) {
		if runtimeSourceAuthorityHasRuntimeCarrier(authority) {
			return true
		}
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.HasExternalOnlyRuntimeArtifact() || rm.HasRuntimeArtifactPathReference() {
		return true
	}
	if ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil && bundle.IsExternalSource() {
			return true
		}
		if perf := ctx.Mutable.PerfTrace(); perf != nil && perf.IsExternalSource() {
			return true
		}
	}
	return false
}

func runtimeSourceAuthorityHasRuntimeCarrier(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.HasRuntimeCarrier()
}

func currentSourceLaneHasSpecificSourceSeed(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	for _, hint := range ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints {
		if currentSourceCoveragePath(hint.Path) {
			return true
		}
	}
	if ctx.Mutable != nil {
		for _, ranked := range ctx.Mutable.Phase1Ranking() {
			if ranked.ExactEntityRank <= 0 {
				continue
			}
			if currentSourceCoveragePath(ranked.Path) {
				return true
			}
		}
	}
	return false
}

func currentSourceLaneHasCoverage(ctx *types.BusContext, preflight completionPreflightView) bool {
	for _, ev := range preflight.Evidence {
		if currentSourceCoverageEvidenceItem(ev) {
			return true
		}
	}
	if ctx == nil {
		return false
	}
	for _, ev := range ctx.EvidenceItems {
		if currentSourceCoverageEvidenceItem(ev) {
			return true
		}
	}
	if ctx.Mutable != nil {
		for _, p := range ctx.Mutable.EvidenceClosure().CanonicalReadFiles() {
			if currentSourceCoveragePath(p) {
				return true
			}
		}
		if turnA := ctx.Mutable.TurnAArtifacts(); turnA != nil {
			for _, p := range turnA.ReadFiles {
				if currentSourceCoveragePath(p) {
					return true
				}
			}
		}
	}
	return false
}

func currentSourceCoverageEvidenceItem(ev types.EvidenceItem) bool {
	if ev.Origin == types.ClaimOriginLog ||
		ev.Origin == types.ClaimOriginPerf {
		return false
	}
	return currentSourceCoveragePath(ev.Source)
}

func currentSourceCoveragePath(raw string) bool {
	path := strings.TrimSpace(filepath.ToSlash(raw))
	if path == "" || types.LooksLikeRuntimeArtifactPath(path) {
		return false
	}
	if !types.HasCodeOrConfigPathSuffix(path) {
		return false
	}
	return true
}

func forcedReadSeedIsRuntimeArtifact(path string) bool {
	return types.LooksLikeRuntimeArtifactPath(path)
}

func raisePrimaryAnchorPendingRead(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil || ctx.AnalysisIR == nil {
		return
	}
	if historyBackedCurrentCodeSkipsGenericForcedReadGates(ctx) {
		logging.Info("[CGEC] primary_anchor_unread: skipped for history-backed current-code explanation")
		return
	}
	if !currentSourceForcedReadGatesApply(ctx) {
		logging.Info("[CGEC] primary_anchor_unread: skipped current-source forced read decision=%s",
			ctx.AnalysisIR.RequestModel.CurrentSourceLaneDecision())
		return
	}
	if capabilitySurfacePlan(ctx) != nil {
		return
	}
	// Orientation skip — a "what does this project do?" question
	// should not be blocked on "primary anchor unread"; the very
	// concept of a primary anchor implies a specific code subject
	// the user pinned to, which orientation questions explicitly
	// lack (Predicates clean + len(PrimaryEntities)==0). Mirrors
	// the same predicate applyMultiPathAnchorChecks uses.
	if types.IsProjectOrientationQuestion(ctx.AnalysisIR.RequestModel) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind), "mechanism") {
		return
	}
	for _, ranked := range ctx.Mutable.Phase1Ranking() {
		if ranked.ExactEntityRank <= 0 {
			continue
		}
		if forcedReadSeedIsRuntimeArtifact(ranked.Path) {
			logging.Info("[CGEC] primary_anchor_unread: skipping runtime artifact seed file=%s", ranked.Path)
			continue
		}
		canon, ok := qualifyForcedReadSeedPath(ctx, ranked.Path)
		if !ok {
			logging.Warning("[CGEC] primary_anchor_unread: skipping phantom path file=%s (no match in any active sub-repo)",
				canonicaliseAnchorPath(ranked.Path, ctx.RepoRoot))
			continue
		}
		if closure.HasRead(canon) {
			continue
		}
		if forcedReadSeedUnavailableAfterAttempt(ctx, canon) {
			logging.Warning("[CGEC] primary_anchor_unread: skipping unavailable path file=%s after failed read_file proof", canon)
			continue
		}
		closure.AddPendingRead(types.PendingRead{
			File:      canon,
			Rationale: "Exact entity anchor remains unread — mechanism answers must read the anchor implementation file before completion",
			Origin:    "pre_complete.primary_anchor",
		})
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Files:     []string{canon},
			Rationale: "Read the exact-entity anchor file before re-calling emit_investigation_complete",
			Origin:    "pre_complete.primary_anchor",
		})
		logging.Info("[CGEC] primary_anchor_unread: queued forced-read file=%s", canon)
		return
	}
}

func raiseRequiredFileHintPendingReads(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.AnalysisIR == nil || closure == nil {
		return
	}
	if !types.RequiredFileHintCurrentSourceCoverageApplies(ctx.AnalysisIR.RequestModel) {
		return
	}
	maxUnread := types.RequiredFileHintCoverageMaxForRequest(ctx.AnalysisIR.RequestModel)
	var unread []string
	for _, hint := range ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		canon, ok := types.QualifyForcedReadSeedPath(ctx, hint.Path)
		if !ok {
			logging.Warning("[CGEC] required_file_hint_unread: skipping phantom path file=%s",
				types.CanonicalRequiredFileHintPath(hint.Path, ctx.RepoRoot))
			continue
		}
		if closure.HasRead(canon) || groundedEvidenceCoversFile(evidence, canon, ctx.RepoRoot) {
			continue
		}
		if forcedReadSeedUnavailableAfterAttempt(ctx, canon) {
			logging.Warning("[CGEC] required_file_hint_unread: skipping unavailable path file=%s after failed read_file proof", canon)
			continue
		}
		unread = append(unread, canon)
		if len(unread) >= maxUnread {
			break
		}
	}
	if len(unread) == 0 {
		return
	}
	for _, file := range unread {
		closure.AddPendingRead(types.PendingRead{
			File:      file,
			Rationale: "High-confidence current-source file from request analysis remains unread — mixed artifact/history + current-code answers must verify this source before completion",
			Origin:    "required_file_hint_unread",
		})
		logging.Info("[CGEC] required_file_hint_unread: queued forced-read file=%s", file)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairExpandSearch,
		Files:     unread,
		Rationale: fmt.Sprintf("%d high-confidence current-source required file(s) remain unread; inspect them before re-calling emit_investigation_complete", len(unread)),
		Origin:    "pre_complete.required_file_hint_unread",
	})
}

func groundedEvidenceCoversFile(evidence []types.EvidenceItem, file, repoRoot string) bool {
	file = types.CanonicalRequiredFileHintPath(file, repoRoot)
	if file == "" {
		return false
	}
	for _, ev := range evidence {
		if ev.GroundingStatus == types.GroundingUngrounded || ev.LineStart <= 0 {
			continue
		}
		source := types.CanonicalRequiredFileHintPath(ev.Source, repoRoot)
		if source == file {
			return true
		}
	}
	return false
}

// applyMultiPathAnchorChecks is the symbol-anchored verification the
// 2026-05-01 rewrite installed in place of raiseMultiPathCoverageParity.
// For each primary-anchor file (Phase1Ranking entry with
// ExactEntityRank > 0) it runs internal/tool/multipath.EvaluateAnchor
// and translates the resulting Decision into closure mutations:
//
//   - ActionSkip — no-op.
//   - ActionDemandSurgicalRead — AddPendingRead + AddRepair, both
//     carrying the engine's surgical Rationale that names the
//     uncovered symbol slivers and includes a copy-pasteable
//     read_file invocation.
//   - ActionAdvisoryHint — AddRepair with kind RepairExpandSearch,
//     intentionally NO PendingRead so emit_investigation_complete is
//     not blocked. The LLM may complete if it judges the file
//     unrelated.
//
// Cross-gate skips mirror the predecessor:
//   - capabilitySurfacePlan present → skip (capability surface owns
//     its own anchor set).
//   - IsProjectOrientationQuestion(RequestModel) → skip ("what does
//     this project do?" has no cross-component depth obligation).
//   - requiresCrossFileCoverage(kind, anchorCount) false → skip.
//   - Fewer than 2 primary anchors → skip (single-subject questions
//     have no parity target).
func applyMultiPathAnchorChecks(ctx *types.BusContext, closure *types.EvidenceClosure) {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	applyMultiPathAnchorChecksWithEvidence(ctx, closure, evidence)
}

func applyMultiPathAnchorChecksWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.Mutable == nil || closure == nil || ctx.AnalysisIR == nil {
		return
	}
	if historyBackedCurrentCodeSkipsGenericForcedReadGates(ctx) {
		logging.Info("[CGEC] multi_path_anchor: skipped for history-backed current-code explanation")
		return
	}
	if capabilitySurfacePlan(ctx) != nil {
		return
	}
	rm := ctx.AnalysisIR.RequestModel
	if types.IsProjectOrientationQuestion(rm) {
		return
	}
	kind := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	ranked := ctx.Mutable.Phase1Ranking()
	if len(ranked) == 0 {
		return
	}

	// Collect canonical anchor file list (dedup by canonical path).
	// Forced-read seeds may arrive as bare sub-repo-relative paths
	// while read_file history is keyed by active sub-repo prefix.
	// Qualify at seed time so every downstream gate sees the same key.
	repoRoot := ctx.RepoRoot
	anchors := make([]string, 0, len(ranked))
	seen := make(map[string]bool, len(ranked))
	for _, rf := range ranked {
		if rf.ExactEntityRank <= 0 {
			continue
		}
		canon, ok := qualifyForcedReadSeedPath(ctx, rf.Path)
		if !ok {
			logging.Warning("[CGEC] multi_path_anchor: skipping phantom path file=%s (no match in any active sub-repo)",
				canonicaliseAnchorPath(rf.Path, repoRoot))
			continue
		}
		if canon == "" || seen[canon] {
			continue
		}
		seen[canon] = true
		anchors = append(anchors, canon)
	}
	if !requiresCrossFileCoverage(kind, len(anchors)) {
		return
	}
	if len(anchors) < 2 {
		return
	}

	// Resolve config + symbol oracle + keyword anchors once per check.
	limits := CurrentAnalysisLimits()
	cfg := multipath.Config{
		MinGroundedPerAnchor: limits.MultiPathMinGroundedPerAnchor,
		SymbolContextLines:   limits.MultiPathSymbolContextLines,
		KeywordContextLines:  limits.MultiPathKeywordContextLines,
		SmallFileThreshold:   limits.MultiPathSmallFileThreshold,
		RepoRoot:             repoRoot,
	}
	oracle := repomapSymbolOracle(ctx)
	entities := unionPrimaryAndDerivedEntities(rm.AnalyzerHints)
	keywordAnchorMap := multipath.ExtractKeywordAnchorsCapped(
		multiPathToolHistory(ctx), entities, repoRoot,
		limits.MultiPathMaxKeywordAnchorsPerFile,
	)

	for _, file := range anchors {
		dec := multipath.EvaluateAnchor(file, closure, evidence, oracle, entities, keywordAnchorMap[file], cfg)
		switch dec.Action {
		case multipath.ActionSkip:
			logging.Debug("[CGEC] multi_path_anchor: file=%s signal=%s action=skip reason=%q",
				file, dec.Signal, dec.Reason)
		case multipath.ActionDemandSurgicalRead:
			if forcedReadSeedUnavailableAfterAttempt(ctx, file) {
				logging.Warning("[CGEC] multi_path_anchor: skipping unavailable path file=%s after failed read_file proof", file)
				continue
			}
			if !callChainMultiPathDemandShouldBlock(rm, file, dec, evidence) {
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairExpandSearch,
					Files:     []string{file},
					Rationale: dec.Rationale,
					Origin:    "pre_complete.multi_path_anchor",
				})
				logging.Info("[CGEC] multi_path_anchor: file=%s signal=%s action=advisory_hint reason=%q",
					file, dec.Signal, "call-chain symbol demand is not an endpoint or model-emitted evidence surface")
				continue
			}
			// Surgical Demand from the engine flows into the
			// PendingRead's LineRanges. runForcedReads (the Lazy
			// Auto-Read recovery in cgec_enforcers.go) honours these
			// to issue surgical read_file calls instead of pulling
			// the whole file when the LLM stalls — so the demand
			// stays bounded both in the primary path (LLM reads the
			// rationale-named ranges) and the recovery path
			// (orchestrator forced-reads the same ranges).
			closure.AddPendingRead(types.PendingRead{
				File:       file,
				Rationale:  dec.Rationale,
				Origin:     "pre_complete.multi_path_anchor",
				LineRanges: append([]types.LineRange(nil), dec.Demand...),
			})
			logging.Info("[CGEC] multi_path_anchor: file=%s signal=%s action=demand_surgical_read symbols=%v missing_ranges=%d",
				file, dec.Signal, dec.Symbols, len(dec.Demand))
		case multipath.ActionAdvisoryHint:
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairExpandSearch,
				Files:     []string{file},
				Rationale: dec.Rationale,
				Origin:    "pre_complete.multi_path_anchor",
			})
			logging.Info("[CGEC] multi_path_anchor: file=%s signal=%s action=advisory_hint (non-blocking)",
				file, dec.Signal)
		}
	}
}

func callChainMultiPathDemandShouldBlock(rm types.RequestModel, file string, dec multipath.Decision, evidence []types.EvidenceItem) bool {
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain {
		return true
	}
	if dec.Signal != multipath.SignalSymbolDemand || len(dec.Symbols) == 0 {
		return true
	}
	startHint, endHint, ok := callChainPrincipalEndpointHints(rm)
	if !ok {
		return true
	}
	var hardSurfaces []string
	hardSurfaces = append(hardSurfaces, startHint, endHint)
	fileKey := canonicalCallChainSource(file)
	for _, item := range evidence {
		if canonicalCallChainSource(item.Source) != fileKey {
			continue
		}
		hardSurfaces = append(hardSurfaces, item.Subject, item.Object, item.AnchorSymbol, item.OwnerSymbol)
	}
	for _, symbol := range dec.Symbols {
		if callChainSymbolMatchesAnySurface(symbol, hardSurfaces) {
			return true
		}
	}
	return false
}

func callChainSymbolMatchesAnySurface(symbol string, surfaces []string) bool {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return false
	}
	symKey := callChainSymbolMatchKey(symbol)
	if symKey == "" {
		return false
	}
	for _, surface := range surfaces {
		if callChainSymbolMatchesSurfaceKey(symKey, symbol, surface) {
			return true
		}
	}
	return false
}

func callChainSymbolMatchesSurfaceKey(symKey, symbol, surface string) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	surfaceKey := callChainSymbolMatchKey(surface)
	if surfaceKey == "" {
		return false
	}
	if strings.EqualFold(symbol, surface) || symKey == surfaceKey {
		return true
	}
	symLeaf := callChainSymbolLeaf(symKey)
	surfaceLeaf := callChainSymbolLeaf(surfaceKey)
	return symLeaf != "" && surfaceLeaf != "" && symLeaf == surfaceLeaf
}

func callChainSymbolMatchKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "()")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(raw, "\\", "/"))
}

func callChainSymbolLeaf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, sep := range []string{"::", ".", "#"} {
		if idx := strings.LastIndex(raw, sep); idx >= 0 && idx+len(sep) < len(raw) {
			raw = raw[idx+len(sep):]
		}
	}
	return strings.TrimSpace(raw)
}

// canonicaliseAnchorPath collapses an anchor path to repo-relative
// POSIX form via ground.CanonicalRepoRelative when repoRoot is set,
// degrading to a slash-form cleanup when not. Mirrors the canonicaliser
// inside multipath / inside ground.coverage so every gate-related
// path key converges on the same shape across Linux / Windows / macOS.
func canonicaliseAnchorPath(p, repoRoot string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	if repoRoot != "" {
		return ground.CanonicalRepoRelative(p, repoRoot)
	}
	cleaned := strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(cleaned, "./")
}

func qualifyForcedReadSeedPath(ctx *types.BusContext, raw string) (string, bool) {
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	qualified, ok := types.QualifyForcedReadSeedPath(ctx, raw)
	if !ok {
		return "", false
	}
	qualified = canonicaliseAnchorPath(qualified, repoRoot)
	if qualified == "" {
		return "", false
	}
	return qualified, true
}

func forcedReadSeedUnavailableAfterAttempt(ctx *types.BusContext, canon string) bool {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return false
	}
	canon = phase1UnreadCanonPath(canon, ctx.RepoRoot)
	if canon == "" || !forcedReadSeedHadFailedReadAttempt(ctx, canon) {
		return false
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, filepath.FromSlash(canon)))
	return err != nil || info.IsDir()
}

func clearUnavailablePendingReadsAfterFailedRead(ctx *types.BusContext, closure *types.EvidenceClosure) int {
	if ctx == nil || closure == nil {
		return 0
	}
	cleared := 0
	for _, pending := range closure.PendingReads() {
		if forcedReadSeedUnavailableAfterAttempt(ctx, pending.File) {
			closure.ClearPendingReadFor(pending.File)
			cleared++
		}
	}
	return cleared
}

func forcedReadSeedHadFailedReadAttempt(ctx *types.BusContext, canon string) bool {
	if ctx == nil {
		return false
	}
	canon = phase1UnreadCanonPath(canon, ctx.RepoRoot)
	if canon == "" {
		return false
	}
	absNeedle := ""
	if strings.TrimSpace(ctx.RepoRoot) != "" {
		absNeedle = filepath.ToSlash(filepath.Join(ctx.RepoRoot, filepath.FromSlash(canon)))
	}
	for _, result := range aggregateSupportToolResults(ctx) {
		if result.Success || types.CanonicalToolName(result.ToolName) != "read_file" {
			continue
		}
		if readFilePathRepairMatchesForcedRead(ctx, result.Repair, canon, absNeedle) {
			return true
		}
	}
	return false
}

func readFilePathRepairMatchesForcedRead(ctx *types.BusContext, repair *types.ToolRepair, canon, absNeedle string) bool {
	if repair == nil {
		return false
	}
	switch strings.TrimSpace(repair.Code) {
	case types.ToolRepairCodeReadFilePathMissing, types.ToolRepairCodeReadFilePathIsDirectory:
	default:
		return false
	}
	if readFilePathRepairMetadataMatches(ctx, repair.Metadata, "path", canon, absNeedle) {
		return true
	}
	if readFilePathRepairMetadataMatches(ctx, repair.Metadata, "resolved_path", canon, absNeedle) {
		return true
	}
	for _, target := range repair.Targets {
		if readFileRepairPathMatches(ctx, target.File, canon, absNeedle) {
			return true
		}
	}
	return false
}

func readFilePathRepairMetadataMatches(ctx *types.BusContext, metadata map[string]string, key, canon, absNeedle string) bool {
	if metadata == nil {
		return false
	}
	return readFileRepairPathMatches(ctx, metadata[key], canon, absNeedle)
}

func readFileRepairPathMatches(ctx *types.BusContext, raw, canon, absNeedle string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || canon == "" {
		return false
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	normalized := phase1UnreadCanonPath(raw, repoRoot)
	if normalized == canon {
		return true
	}
	rawSlash := strings.ReplaceAll(raw, `\`, `/`)
	if absNeedle != "" && rawSlash == absNeedle {
		return true
	}
	return false
}

// repomapSymbolOracle wraps the per-Run repomap Graph (when one is
// attached) into the multipath.SymbolOracle interface. Returns nil
// when no graph is available — the engine then disables Signal 1 and
// drops to Signal 2 / 3 cleanly.
func repomapSymbolOracle(ctx *types.BusContext) multipath.SymbolOracle {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	g, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || g == nil {
		return nil
	}
	return repomapOracleAdapter{graph: g}
}

type repomapOracleAdapter struct {
	graph *repotypes.Graph
}

func (a repomapOracleAdapter) SymbolsInFile(file string) []multipath.SymbolDef {
	if a.graph == nil {
		return nil
	}
	syms := a.graph.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}
	out := make([]multipath.SymbolDef, 0, len(syms))
	for _, s := range syms {
		out = append(out, multipath.SymbolDef{
			Name:    s.Name,
			Line:    s.Line,
			EndLine: s.EndLine,
		})
	}
	return out
}

// multiPathToolHistory aggregates the same three-source tool history
// ground.BuildContext consumes (TurnA artefacts → ctx.ToolResults →
// in-dispatch buffer). multipath.ExtractKeywordAnchors walks this
// to find grep matches the LLM produced earlier in the Run; passing
// the same composite history that everything else reads keeps the
// gate consistent with what the LLM was actually shown.
//
// nil-tolerant on every input branch.
func multiPathToolHistory(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil {
		return nil
	}
	var history []types.ToolResult
	if ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && len(ta.ToolResults) > 0 {
			history = append(history, ta.ToolResults...)
		}
	}
	if len(ctx.ToolResults) > 0 {
		history = append(history, ctx.ToolResults...)
	}
	if ctx.Mutable != nil {
		if disp := ctx.Mutable.DispatchToolResults(); len(disp) > 0 {
			history = append(history, disp...)
		}
	}
	return history
}

// unionPrimaryAndDerivedEntities merges the analyzer's PrimaryEntities
// + Entities + MentionedEntities + DerivedEntities into a single
// dedup'd lower-cased slice. The engine matches case-insensitively
// against this set when picking question-related symbols, so passing
// the union (rather than just PrimaryEntities) lets sub-topic
// derivations and log-augmented entities also count.
func unionPrimaryAndDerivedEntities(h types.AnalyzerHints) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	push := func(list []string) {
		for _, e := range list {
			key := strings.ToLower(strings.TrimSpace(e))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, e)
		}
	}
	push(h.PrimaryEntities)
	push(h.MentionedEntities)
	push(h.Entities)
	push(h.DerivedEntities)
	return out
}

// nextReadOffset suggests the read_file offset (0-based, matching
// the tool's schema) the LLM should pass to continue WITHOUT re-
// reading already-covered ranges. Strategy: take the largest End
// of any merged 1-based read range and return that value as the
// 0-based offset (End is 1-based; the next unread 1-based line is
// End+1, whose 0-based offset is End). When the file has no
// recorded reads (fresh anchor) returns 0 (start of file). The
// offset is a hint, not a constraint — the LLM may pass a
// different offset to reach a specific section. Helps the LLM
// avoid the "I already read 1-100 so let me re-read 1-100" loop
// the production trace exposed.
func nextReadOffset(ranges []types.LineRange) int {
	maxEnd := 0
	for _, r := range ranges {
		if r.End > maxEnd {
			maxEnd = r.End
		}
	}
	if maxEnd <= 0 {
		return 0
	}
	return maxEnd
}

// raisePhase1UnreadPendingReads is the session-12 CGEC gate that
// catches the "LLM declares complete while ignoring high-ranked pre-
// scan files" failure mode. The R5 ghost-anchor promotion covers the
// opposite half — files the LLM's chains REFERENCE but didn't read —
// and says nothing about files the LLM simply skipped. This gate
// closes that gap by treating the keyword-search ranking itself as
// evidence of "these files are likely answer-bearing; verify they
// were read before complete".
//
// Triggering conditions (all must hold):
//   - analysisLimits.Phase1UnreadTopK > 0 (gate enabled)
//   - ctx.AnalysisIR is non-nil (classification available)
//   - the declared RequirementKind structurally benefits from
//     cross-file coverage. mechanism / call_chain / conditional always
//     do; config_mapping joins them when multiple primary anchors are
//     present (for example code default + runtime overlay)
//   - Phase1Ranking has at least Phase1UnreadTopK entries
//   - top-K minus ReadSet has at least Phase1UnreadMinUnread entries
//
// When a repo graph and a concrete ReadSet focus are available, the
// gate narrows the unread list to hard anchors only. Generic graph
// adjacency remains a navigation signal for the explorer, but is too
// broad to become a completion blocker on its own (same-package
// interface/type references otherwise pull sibling implementations into
// the Forced Read List).
//
// When it fires: append PendingReads for each unread top-K file (with
// origin="phase1_unread" so trace inspection can tell this gate from
// chain-promotion D2) and raise a RepairExpandSearch directive. The
// downstream PendingReads branch of preCompleteContractCheck then
// renders the downgrade message.
func raisePhase1UnreadPendingReads(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) {
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return
	}
	if ctx.AnalysisIR == nil {
		return
	}
	if historyBackedCurrentCodeSkipsGenericForcedReadGates(ctx) {
		logging.Info("[CGEC] phase1_unread: skipped for history-backed current-code explanation")
		return
	}
	if !currentSourceForcedReadGatesApply(ctx) {
		logging.Info("[CGEC] phase1_unread: skipped current-source forced read decision=%s",
			ctx.AnalysisIR.RequestModel.CurrentSourceLaneDecision())
		return
	}
	// Orientation skip — orientation questions don't carry a
	// breadth-intent obligation to read the keyword-search top-K.
	// Same predicate as raisePrimaryAnchorPendingRead +
	// applyMultiPathAnchorChecks for cross-gate consistency.
	if types.IsProjectOrientationQuestion(ctx.AnalysisIR.RequestModel) {
		return
	}
	// T2.1: once-per-pipeline latch. The first firing queues the
	// unread top-K files + raises a RepairExpandSearch directive;
	// the LLM has now been told. A second firing would list the same
	// (or a subset of the same) files without surfacing any new
	// information the first fire did not, while paying the cost of
	// an extra explorer redispatch. Stall soft/hard detection still
	// catches the "LLM keeps declaring complete without progress"
	// case via fingerprint convergence.
	if closure.Phase1UnreadFired() {
		return
	}
	if plan := capabilitySurfacePlan(ctx); plan != nil {
		unread := capabilitySurfaceUnreadAuthorityFiles(plan, closure, ctx.RepoRoot)
		if len(unread) == 0 {
			return
		}
		for _, file := range unread {
			closure.AddPendingRead(types.PendingRead{
				File:      file,
				Rationale: "Capability-surface authority file remains unread — inspect the canonical stage binding / skill contract / tool exposure source before declaring completion",
				Origin:    "phase1_unread",
			})
			logging.Info("[CGEC] phase1_unread: queued capability-authority file=%s", file)
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Files:     unread,
			Rationale: fmt.Sprintf("%d capability-surface authority file(s) remain unread; inspect them before re-calling emit_investigation_complete", len(unread)),
			Origin:    "pre_complete.phase1_unread",
		})
		closure.MarkPhase1UnreadFired()
		return
	}
	limits := CurrentAnalysisLimits()
	if limits.Phase1UnreadTopK <= 0 {
		return
	}
	kind := types.NormalizeRequirementKind(ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind)
	ranking := ctx.Mutable.Phase1Ranking()
	if len(ranking) == 0 {
		return
	}
	sourceInventoryCoverage := sourceInventoryPhase1CoverageApplies(ctx)
	filter := newPhase1UnreadFilter(ctx, closure, aggregateFacts)
	if sourceInventoryCoverage && !filter.sourceInventoryStrictScope {
		logging.Info("[CGEC] phase1_unread: skipped source-inventory generic ranker files without a precise completed scope")
		return
	}
	if !sourceInventoryCoverage && !requiresCrossFileCoverage(kind, countPrimaryAnchorFiles(ranking)) {
		return
	}
	topK := limits.Phase1UnreadTopK
	if topK > len(ranking) {
		topK = len(ranking)
	}
	minUnread := limits.Phase1UnreadMinUnread
	if minUnread < 1 {
		minUnread = 1
	}
	type unreadRankedFile struct {
		rank int
		file types.Phase1RankedFile
	}
	var unread []unreadRankedFile
	for i := 0; i < topK; i++ {
		f := ranking[i]
		if forcedReadSeedIsRuntimeArtifact(f.Path) {
			logging.Info("[CGEC] phase1_unread: skipping runtime artifact seed file=%s", f.Path)
			continue
		}
		canon := phase1UnreadCanonPath(f.Path, ctx.RepoRoot)
		if canon == "" {
			continue
		}
		// Multi-repo qualification: in multi-repo posture the ranker
		// returns sub-repo-relative bare paths (e.g.
		// "packages/opencode/src/tool/apply_patch.ts") whose canonical
		// key differs from the LLM's actual read of the FS-real
		// "opencode/packages/opencode/src/tool/apply_patch.ts". Resolve
		// against the active set so HasRead / DrainSatisfiedPendingReads
		// key on the same canonical form the read_file gate produces.
		// docs/design/post_phase2a_forensic_followups.md §2.1.B.
		qualified, qok := qualifyForcedReadSeedPath(ctx, canon)
		if !qok {
			logging.Warning("[CGEC] phase1_unread: skipping phantom path file=%s (no match in any active sub-repo)", canon)
			continue
		}
		if qualified != canon {
			logging.Info("[CGEC] phase1_unread: auto-qualified bare path %s → %s", canon, qualified)
			canon = qualified
		}
		if forcedReadSeedUnavailableAfterAttempt(ctx, canon) {
			logging.Warning("[CGEC] phase1_unread: skipping unavailable path file=%s after failed read_file proof", canon)
			continue
		}
		if closure.HasRead(canon) {
			continue
		}
		if filter.enabled && !filter.hasMandatoryReadSignal(f, canon) {
			logging.Debug("[CGEC] phase1_unread: skip non-mandatory ranked file=%s after read focus", canon)
			continue
		}
		f.Path = canon
		unread = append(unread, unreadRankedFile{rank: i + 1, file: f})
	}
	if len(unread) < minUnread {
		return
	}

	files := make([]string, 0, len(unread))
	for _, item := range unread {
		f := item.file
		rationale := fmt.Sprintf(
			"Top-%d pre-scan ranked file (score=%.1f) remains unread — breadth-intent question (kind=%s) needs cross-component evidence",
			item.rank, f.Score, kind,
		)
		closure.AddPendingRead(types.PendingRead{
			File:      f.Path,
			Rationale: rationale,
			Origin:    "phase1_unread",
		})
		files = append(files, f.Path)
		logging.Info("[CGEC] phase1_unread: queued forced-read file=%s score=%.1f kind=%s", f.Path, f.Score, kind)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairExpandSearch,
		Files:     files,
		Rationale: fmt.Sprintf("%d of the top-%d keyword-search ranked files were not read before emit_investigation_complete — open them before re-declaring complete", len(unread), topK),
		Origin:    "pre_complete.phase1_unread",
	})
	// T2.1: set the latch after the gate has produced its output, so
	// a nil/zero-queue early return above does NOT burn the latch.
	closure.MarkPhase1UnreadFired()
}

type phase1UnreadFilter struct {
	enabled                    bool
	repoRoot                   string
	graph                      *repotypes.Graph
	readFiles                  []string
	requiredFileSet            map[string]bool
	sourceInventoryStrictScope bool
	sourceInventoryScopeSet    map[string]bool
}

func newPhase1UnreadFilter(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact) phase1UnreadFilter {
	var out phase1UnreadFilter
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return out
	}
	out.repoRoot = ctx.RepoRoot
	out.requiredFileSet = phase1UnreadRequiredFileSet(ctx)
	out.sourceInventoryScopeSet, out.sourceInventoryStrictScope = phase1UnreadSourceInventoryStrictScopeSet(ctx, aggregateFacts)
	if out.sourceInventoryStrictScope {
		out.enabled = true
	}
	if g, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph); ok && g != nil && len(g.FileIndex) > 0 {
		out.graph = g
	}
	for file := range closure.ReadSet() {
		if canon := phase1UnreadCanonPath(file, ctx.RepoRoot); canon != "" {
			out.readFiles = append(out.readFiles, canon)
		}
	}
	sort.Strings(out.readFiles)
	if out.graph == nil || len(out.readFiles) == 0 {
		return out
	}
	for _, file := range out.readFiles {
		if _, ok := out.graph.FileIndex[file]; ok {
			out.enabled = true
			break
		}
	}
	return out
}

func (f phase1UnreadFilter) hasMandatoryReadSignal(ranked types.Phase1RankedFile, file string) bool {
	file = phase1UnreadCanonPath(file, f.repoRoot)
	if file == "" {
		return false
	}
	if f.sourceInventoryStrictScope {
		return f.sourceInventoryScopeSet != nil && f.sourceInventoryScopeSet[file]
	}
	if f.requiredFileSet != nil && f.requiredFileSet[file] {
		return true
	}
	return ranked.ExactEntityRank > 0
}

func phase1UnreadSourceInventoryStrictScopeSet(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) (map[string]bool, bool) {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return nil, false
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if !types.SourceInventoryLensExecuted(observation) || !sourceInventoryObservationCompleteForPhase1Scope(observation) {
		return nil, false
	}
	files := types.PrincipalSourceInventoryMemberSetScopeFiles(aggregateFacts, &ctx.AnalysisIR.RequestModel)
	if len(files) == 0 {
		files = types.PrincipalSourceInventoryMemberSetScopeFiles(ctx.Mutable.StableInvestigationAggregateFacts(), &ctx.AnalysisIR.RequestModel)
	}
	if len(files) == 0 {
		files = types.BoundedSourceEnumerationScopeFiles(ctx.AnalysisIR.RequestModel, ctx.AnalysisIR.EvidencePlan.RequiredFiles, ctx.RepoRoot)
	}
	if len(files) == 0 || types.BoundedSourceEnumerationCommonScope(files) == "" {
		return nil, false
	}
	set := make(map[string]bool, len(files))
	for _, file := range files {
		canon := phase1UnreadCanonPath(file, ctx.RepoRoot)
		if canon != "" {
			set[canon] = true
		}
	}
	if len(set) == 0 {
		return nil, false
	}
	return set, true
}

func sourceInventoryObservationCompleteForPhase1Scope(observation types.SourceInventoryObservation) bool {
	if observation.Complete {
		return true
	}
	if observation.Page != nil && observation.Page.Complete {
		return true
	}
	if types.SourceInventorySourceClassesComplete(observation.SourceClasses) {
		return true
	}
	return false
}

func phase1UnreadRequiredFileSet(ctx *types.BusContext) map[string]bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	add := func(set map[string]bool, raw string) map[string]bool {
		canon := phase1UnreadCanonPath(raw, ctx.RepoRoot)
		if canon == "" {
			return set
		}
		if set == nil {
			set = make(map[string]bool)
		}
		set[canon] = true
		return set
	}
	var set map[string]bool
	for _, file := range ctx.AnalysisIR.EvidencePlan.RequiredFiles {
		set = add(set, file)
	}
	for _, hint := range ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		set = add(set, hint.Path)
	}
	return set
}

func phase1UnreadCanonPath(path, repoRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = ground.CanonicalRepoRelative(path, repoRoot)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

// qualifyForcedReadPathForMultiRepo resolves a forced-read seed path
// against the multi-repo active set so the queued entry matches the
// canonical form the LLM's later read_file produces. Returns (qualified,
// true) when the path resolves to a unique active sub-repo on disk;
// returns (canon, false) when the path is a phantom (no match in any
// active sub-repo) so the caller drops the seed rather than queue an
// entry that can never drain.
//
// Single-repo / no-MultiGraph callers pass through byte-identical (the
// gate short-circuits at IsSingle). Already-prefixed and absolute paths
// pass through ResolveActiveSetPath untouched via its sub-repo-prefix
// branch + absolute-path branch.
//
// Ambiguous bare paths (matching ≥ 2 active sub-repos on disk) are
// returned as (canon, false) — i.e. dropped — because the seed cannot
// pick the right sub-repo without LLM intent. The phase1_unread gate
// has a once-per-pipeline latch (T2.1) so a dropped entry does not
// re-fire; the LLM continues its investigation and the path will be
// surfaced by ordinary keyword_search ranking with the prefix attached.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.B — the
// pre-scanner produced bare "packages/opencode/src/tool/apply_patch.ts"
// in a codrax + opencode multi-repo; the queue's canonicalisation
// keyed it differently from the LLM's FS-real
// "opencode/packages/opencode/src/tool/apply_patch.ts", queue never
// drained → 10× DOWNGRADED + 5 min wasted before the LLM gave up.
func qualifyForcedReadPathForMultiRepo(ctx *types.BusContext, p string) (string, bool) {
	if ctx == nil || ctx.MultiGraph == nil {
		return p, true
	}
	gater, isGater := ctx.MultiGraph.(types.MultiRepoActiveSetGater)
	if !isGater || gater == nil {
		return p, true
	}
	gate := gater.ResolveActiveSetPath(ctx, "phase1_unread_seed", p, func(abs string) bool {
		info, err := os.Stat(abs)
		return err == nil && !info.IsDir()
	})
	if !gate.Allowed {
		return p, false
	}
	if gate.ResolvedPath == "" {
		return p, true
	}
	return gate.ResolvedPath, true
}

type callChainPrincipalSpanDemand struct {
	source       string
	startHint    string
	endHint      string
	startLine    int
	endLine      int
	lateStart    int
	demandStart  int
	demandEnd    int
	lastInterior int
}

// callChainPrincipalSpanDowngrade protects the principal source→sink
// trace from closing while the support lane still has a large, typed
// evidence hole immediately before the sink. The gate is deliberately
// language-neutral: it reads only analyzer-emitted endpoints and
// structured EvidenceItem fields (Source/LineStart/Subject/Object/
// AnchorSymbol/OwnerSymbol), never Go-specific AST knowledge or raw
// prose keyword scans.
func callChainPrincipalSpanDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure) string {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return callChainPrincipalSpanDowngradeWithEvidence(ctx, closure, evidence)
}

type callChainDirectedPathStatus struct {
	StartResolved  bool
	EndResolved    bool
	StartAmbiguous bool
	EndAmbiguous   bool
	Path           []string
}

// callChainExactEndpointReachabilityDowngradeWithEvidence keeps endpoint
// identity separate from directed call authority. Definitions, read-window
// hits, and prefix siblings such as RunWith/Run can orient investigation, but
// only accepted ClaimCallEdge rows can establish source -> sink reachability.
// The graph is language-neutral: subjects/objects may use '.', '::', or '#'.
// Runtime Trace causal projections never enter this source-code contract.
func callChainExactEndpointReachabilityDowngradeWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || closure == nil ||
		!completionDirectedCallChainEndpointShape(ctx) {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain || types.IsProjectOrientationQuestion(rm) {
		return ""
	}
	startHint, endHint, ok := types.CallChainOrderedEndpointHints(rm)
	if !ok {
		return ""
	}
	status, edgeCount := callChainDirectedPathStatusForEvidence(evidence, startHint, endHint)
	waiver := ctx.Mutable.PrincipalSpanWaiver()
	if len(status.Path) > 0 {
		if waiver != nil && waiver.IsActive() && waiver.Reason == types.PrincipalSpanWaiverNoDirectedPath {
			return EmitInvestigationCompleteDowngradePrefix + " — principal_span_waiver=no_directed_path contradicts the accepted typed call graph.\n\n" +
				"A same-direction typed call path now reaches the requested sink. Set clear_principal_span_waiver=true and retry; keep the path model-authored from the accepted call-edge rows."
		}
		return ""
	}
	if waiver != nil && waiver.IsActive() && waiver.Reason == types.PrincipalSpanWaiverNoDirectedPath {
		existence := types.AnalyzeCallChainEndpointExistence(evidence, startHint, endHint)
		if !existence.StartProven || !existence.EndProven || existence.StartAmbiguous || existence.EndAmbiguous {
			var missing []string
			if !existence.StartProven || existence.StartAmbiguous {
				missing = append(missing, startHint)
			}
			if !existence.EndProven || existence.EndAmbiguous {
				missing = append(missing, endHint)
			}
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairEmitEvidence,
				Keywords:  append([]string(nil), missing...),
				Tools:     []string{"repo_map", "grep", "read_file", "emit_evidence"},
				Rationale: "no_directed_path requires citable current-source existence proof for both exact requested endpoints; locate and read each unresolved endpoint, then emit its grounded definition or real call edge before retrying",
				Origin:    "pre_complete.call_chain_no_path_endpoint_existence",
				Stage:     string(types.StageExplore),
			})
			return fmt.Sprintf("%s — principal_span_waiver=no_directed_path lacks exact endpoint existence proof.\n\nUnresolved or ambiguous endpoint(s): `%s`. Before declaring a no-path boundary, locate and read each exact endpoint and emit a grounded current-source definition or real call edge. A prefix sibling such as RunWith for Run, recovered anchor, runtime artifact, or waiver rationale cannot prove endpoint existence.",
				EmitInvestigationCompleteDowngradePrefix, strings.Join(missing, "`, `"))
		}
		return ""
	}
	if waiver != nil && waiver.IsActive() {
		return fmt.Sprintf("%s — principal_span_waiver=%s cannot waive a missing directed endpoint path.\n\nThe accepted typed call-edge graph does not reach `%s` from `%s`. This waiver reason applies only after a source-to-sink path exists and the remaining question is whether separately-citable intermediate user code is available. Emit the missing same-direction call edge(s), or retry with principal_span_waiver.reason=no_directed_path and a concrete boundary rationale. Do not use endpoint adjacency to substitute a reachable sibling for the requested sink.",
			EmitInvestigationCompleteDowngradePrefix, waiver.Reason, endHint, startHint)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairEmitEvidence,
		Keywords:  []string{startHint, endHint},
		Tools:     []string{"repo_map", "grep", "read_file", "emit_evidence"},
		Rationale: "exact source-to-sink call-chain completion lacks a directed path in accepted typed call-edge evidence; emit the missing same-direction edge(s), or declare the typed no_directed_path boundary after source inspection",
		Origin:    "pre_complete.call_chain_exact_endpoint_reachability",
		Stage:     string(types.StageExplore),
	})
	resolution := "both endpoint identities are absent from the accepted call-edge graph"
	switch {
	case status.StartAmbiguous || status.EndAmbiguous:
		resolution = "one or both short endpoint identities resolve to multiple qualified graph nodes, and not every candidate participates in a same-direction source-to-sink path"
	case status.StartResolved && status.EndResolved:
		resolution = "both endpoints occur in the typed call-edge graph, but the sink is not reachable from the source"
	case status.StartResolved:
		resolution = "the source occurs in the typed call-edge graph, but the exact sink has no call-edge node"
	case status.EndResolved:
		resolution = "the exact sink occurs in the typed call-edge graph, but the source has no call-edge node"
	}
	return fmt.Sprintf("%s — exact call-chain endpoint is not directionally proven.\n\nThe accepted source-code graph contains %d typed call edge(s); %s for `%s` -> `%s`. A symbol definition, a nearby helper, or a prefix sibling such as RunWith for Run cannot prove that direction. Emit grounded same-direction call-edge evidence if the path exists. If source inspection establishes that no such directed path exists, retry with principal_span_waiver.reason=no_directed_path and a concrete rationale, then present the nearest proven path and the endpoint boundary separately instead of inventing an edge.",
		EmitInvestigationCompleteDowngradePrefix, edgeCount, resolution, startHint, endHint)
}

func callChainDirectedPathStatusForEvidence(evidence []types.EvidenceItem, startHint, endHint string) (callChainDirectedPathStatus, int) {
	analysis := types.AnalyzeCallChainEvidenceGraph(evidence, startHint, endHint)
	status := callChainDirectedPathStatus{
		StartResolved: analysis.StartResolved, EndResolved: analysis.EndResolved,
		StartAmbiguous: analysis.StartAmbiguous, EndAmbiguous: analysis.EndAmbiguous,
	}
	if len(analysis.DirectedPath) > 0 {
		status.Path = []string{analysis.DirectedPath[0].From}
		for _, edge := range analysis.DirectedPath {
			status.Path = append(status.Path, edge.To)
		}
	}
	return status, analysis.EdgeCount
}

func callChainPrincipalSpanDowngradeWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) string {
	repair, demand, alreadyRead, ok := callChainPrincipalSpanRepairWithEvidence(ctx, closure, evidence)
	if !ok {
		return ""
	}
	closure.AddRepair(repair)

	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — call-chain principal span is not closure-complete.\n\n")
	lineRange := repair.LineRanges[0]
	fmt.Fprintf(&b, "The structured evidence proves the source endpoint `%s` at %s:%d and the sink endpoint `%s` at %s:%d, but the principal support lane would jump from line %d to the sink without any model-emitted intermediate evidence in %s:%d-%d.\n\n",
		demand.startHint, demand.source, demand.startLine,
		demand.endHint, demand.source, demand.endLine,
		demand.lastInterior, demand.source, lineRange.Start, lineRange.End)
	if alreadyRead {
		b.WriteString("That range is already in read coverage, so do NOT widen scope yet. Re-emit grounded `emit_evidence` items for the intermediate call / handoff / boundary facts from the already-read span, then retry `emit_investigation_complete`.")
	} else {
		b.WriteString("Read that source span, then emit grounded `emit_evidence` items for the intermediate call / handoff / boundary facts before retrying `emit_investigation_complete`.")
	}
	return b.String()
}

func callChainPrincipalSpanRepairWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) (types.RepairDirective, callChainPrincipalSpanDemand, bool, bool) {
	if ctx == nil || ctx.Mutable == nil || closure == nil || ctx.AnalysisIR == nil {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	// §1.6 typed escape: the model can declare via principal_span_waiver
	// that the source→sink intermediate-evidence requirement does not
	// apply (endpoints adjacent / platform bridge / inlined / runtime
	// dispatch / external module / pure plumbing). The waiver is
	// validated up-stream so reaching this point with IsActive()==true
	// already means the reason is in the typed enum and the rationale
	// is non-empty. Bypass the gate so the LLM is not forced to invent
	// intermediate evidence; the audit log line records the reason for
	// post-hoc review.
	if waiver := ctx.Mutable.PrincipalSpanWaiver(); waiver.IsActive() {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentTrace && types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	if types.IsProjectOrientationQuestion(rm) {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	startHint, endHint, ok := callChainPrincipalEndpointHints(rm)
	if !ok {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	demand, ok := callChainPrincipalSpanDemandForEvidence(evidence, startHint, endHint)
	if !ok {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	lineRange := types.LineRange{Start: demand.demandStart, End: demand.demandEnd}
	if lineRange.End < lineRange.Start {
		return types.RepairDirective{}, callChainPrincipalSpanDemand{}, false, false
	}
	alreadyRead := callChainDemandRangeFullyRead(closure, demand.source, lineRange)
	rationale := fmt.Sprintf(
		"source-to-sink call-chain closure has no structured principal evidence in %s:%d-%d between %s and %s; emit grounded intermediate evidence before closing",
		demand.source, lineRange.Start, lineRange.End, demand.startHint, demand.endHint,
	)
	repairKind := types.RepairReadFile
	if alreadyRead {
		repairKind = types.RepairEmitEvidence
		rationale = fmt.Sprintf(
			"already-read source-to-sink span %s:%d-%d still has no structured principal evidence between %s and %s; stay on this span and emit grounded intermediate evidence before closing",
			demand.source, lineRange.Start, lineRange.End, demand.startHint, demand.endHint,
		)
	}
	return types.RepairDirective{
		Kind:       repairKind,
		Files:      []string{demand.source},
		Rationale:  rationale,
		Origin:     "pre_complete.call_chain_principal_span",
		LineRanges: []types.LineRange{lineRange},
		Stage:      string(types.StageExplore),
	}, demand, alreadyRead, true
}

// QueueProactiveCallChainClosureRepairs exposes the call-chain principal-span
// closure target before the model attempts emit_investigation_complete. It
// reuses the exact typed oracle that the completion boundary already enforces:
// trace/call-chain RequestModel traits, analyzer endpoint hints, structured
// EvidenceItem rows, read coverage, and principal_span_waiver. The function
// queues only repair directives; it never materializes evidence or rewrites the
// model's answer.
func QueueProactiveCallChainClosureRepairs(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return false
	}
	refreshClosureReadSnapshot(ctx, closure)
	evidence := ctx.Mutable.EmittedEvidence()
	if callChainAggregateMemberSetCompletesPrincipalBoundary(ctx, effectiveCompletionAggregateFacts(ctx, nil), evidence) {
		return false
	}
	repair, _, _, ok := callChainPrincipalSpanRepairWithEvidence(ctx, closure, evidence)
	if !ok {
		return false
	}
	closure.AddRepair(repair)
	return true
}

type callChainPrincipalSpanContext struct {
	source    string
	startHint string
	endHint   string
	startLine int
	endLine   int
	items     []types.EvidenceItem
}

type callChainMissingQualifiedCall struct {
	Name string
	Line int
	Text string
}

type callChainReadParserRelationGap struct {
	Source string
	Line   int
	Caller string
	Callee string
}

// callChainReadParserRelationHandoffDowngrade prevents a model-owned
// principal call-chain roster from closing over parser-authored calls that the
// investigation already read but never carried into typed evidence.  The join
// is deliberately narrow: both endpoints must resolve uniquely to distinct
// members of one grounded principal member_set, the relation must come from an
// AST-grade parser, and its exact callsite line must be in this run's read
// closure.  The system only asks Explorer to emit the missing edge; it neither
// mints evidence nor writes the eventual call-chain conclusion.
func callChainReadParserRelationHandoffDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, aggregateFacts []types.AnswerAggregateFact, evidence []types.EvidenceItem) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || closure == nil || len(aggregateFacts) == 0 {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if (rm.Intent != types.IntentTrace && types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain) ||
		types.IsProjectOrientationQuestion(rm) || types.RuntimeArtifactContextActiveFromBus(ctx) {
		return ""
	}
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil || len(graph.FileIndex) == 0 {
		return ""
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(aggregateFacts, &rm)
	if len(refs) == 0 {
		refs = types.PrincipalAggregateMemberSetFactRefs(aggregateFacts)
	}
	var memberSets [][]string
	for _, ref := range refs {
		fact := ref.Fact
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) < 2 ||
			!aggregateMemberSetAllMembersUsable(fact, support) {
			continue
		}
		memberSets = append(memberSets, append([]string(nil), fact.Members...))
	}
	if len(memberSets) == 0 {
		return ""
	}

	files := make([]string, 0, len(graph.FileIndex))
	for file := range graph.FileIndex {
		files = append(files, file)
	}
	sort.Strings(files)
	const maxGaps = 8
	gaps := make([]callChainReadParserRelationGap, 0, maxGaps)
	seen := make(map[string]bool)
	for _, file := range files {
		fi := graph.FileIndex[file]
		if fi == nil {
			continue
		}
		fileSource := canonicalRelationSourcePath(fi.RelPath)
		if fileSource == "" {
			fileSource = canonicalRelationSourcePath(file)
		}
		if fileSource == "" || !closure.HasRead(fileSource) {
			continue
		}
		for i := range fi.Relations {
			rel := &fi.Relations[i]
			if rel.Kind != "call" || rel.Line <= 0 ||
				(rel.Provenance != repotypes.ProvenanceTreeSitter && rel.Provenance != repotypes.ProvenanceCangjieParser) {
				continue
			}
			source := canonicalRelationSourcePath(rel.File)
			if source == "" {
				source = canonicalRelationSourcePath(fi.RelPath)
			}
			if source == "" || !closure.HasReadLine(source, rel.Line) {
				continue
			}
			caller := qualifiedEvidenceSymbolNameInFile(fi, enclosingCallableSymbol(fi, rel.Line))
			callee := callRelationTargetName(graph, fi, rel)
			if caller == "" || callee == "" {
				continue
			}
			matched := false
			for _, members := range memberSets {
				callerIndex, callerOK := callChainUniqueMemberEndpointIndex(members, caller)
				calleeIndex, calleeOK := callChainUniqueMemberEndpointIndex(members, callee)
				if callerOK && calleeOK && callerIndex != calleeIndex {
					matched = true
					break
				}
			}
			if !matched || callChainTypedEvidenceContainsExactParserRelation(evidence, source, rel.Line, caller, callee) {
				continue
			}
			key := strings.ToLower(fmt.Sprintf("%s:%d:%s:%s", source, rel.Line, caller, callee))
			if seen[key] {
				continue
			}
			seen[key] = true
			gaps = append(gaps, callChainReadParserRelationGap{Source: source, Line: rel.Line, Caller: caller, Callee: callee})
			if len(gaps) >= maxGaps {
				break
			}
		}
		if len(gaps) >= maxGaps {
			break
		}
	}
	if len(gaps) == 0 {
		return ""
	}
	files = files[:0]
	keywords := make([]string, 0, 2*len(gaps))
	for _, gap := range gaps {
		files = append(files, gap.Source)
		keywords = append(keywords, gap.Caller, gap.Callee)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairEmitEvidence,
		Files:     dedupStringsPreserveOrder(files),
		Keywords:  dedupStringsPreserveOrder(keywords),
		Rationale: "grounded principal call-chain members contain AST-proven calls on already-read lines that are missing from typed call-edge evidence",
		Origin:    "pre_complete.call_chain_read_parser_relation_handoff",
		Stage:     string(types.StageExplore),
	})
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — already-read parser call relations lack typed handoff.\n\n")
	b.WriteString("A grounded principal call-chain member_set contains both endpoints of the AST-authored calls below, and their exact callsite lines are already in this run's read closure, but no equivalent citable call-edge EvidenceItem was emitted. Re-emit each load-bearing edge with evidence_kind=relationship, anchor_kind=call, exact caller/callee, source, and line_start; then retry. Definitions, member notes, read_file memory, and completion reason prose do not replace the edge.\n\n")
	for _, gap := range gaps {
		fmt.Fprintf(&b, "  - %s:%d `%s` -> `%s`\n", gap.Source, gap.Line, gap.Caller, gap.Callee)
	}
	return b.String()
}

func callChainUniqueMemberEndpointIndex(members []string, endpoint string) (int, bool) {
	match := -1
	for i, member := range members {
		base := types.NormalizedSurfaceSymbolTail(member)
		if !types.CallChainEndpointCompatible(member, endpoint) &&
			(base == "" || !types.CallChainEndpointCompatible(base, endpoint)) {
			continue
		}
		if match >= 0 {
			return -1, false
		}
		match = i
	}
	return match, match >= 0
}

func callChainTypedEvidenceContainsExactParserRelation(evidence []types.EvidenceItem, source string, line int, caller, callee string) bool {
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimCallEdge ||
			canonicalRelationSourcePath(item.Source) != source || item.LineStart != line {
			continue
		}
		from := strings.TrimSpace(item.OwnerSymbol)
		if from == "" {
			from = strings.TrimSpace(item.Subject)
		}
		to := strings.TrimSpace(item.Object)
		if to == "" {
			to = strings.TrimSpace(item.AnchorSymbol)
		}
		if types.CallChainEndpointCompatible(from, caller) && types.CallChainEndpointCompatible(to, callee) {
			return true
		}
	}
	return false
}

// callChainQualifiedIntermediateDowngrade closes the gap that the coarse
// tail-span gate intentionally leaves open: when the model has already read a
// same-file source→sink span and emitted adjacent call-chain anchors, a small
// gap between those anchors may still contain load-bearing qualified calls
// that never made it into typed evidence. The detector is structural and
// language-neutral: it reads only read_file gutter text plus typed evidence
// file/line fields, extracts explicit qualified call forms (`pkg.Fn(`,
// `ns::Fn(`), and asks the model to re-emit them as evidence. It never turns
// those candidates into answer facts by itself.
func callChainQualifiedIntermediateDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure) string {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return callChainQualifiedIntermediateDowngradeWithEvidence(ctx, closure, evidence)
}

func callChainQualifiedIntermediateDowngradeWithEvidence(ctx *types.BusContext, closure *types.EvidenceClosure, evidence []types.EvidenceItem) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if waiver := ctx.Mutable.PrincipalSpanWaiver(); waiver.IsActive() {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentTrace && types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain {
		return ""
	}
	if types.IsProjectOrientationQuestion(rm) {
		return ""
	}
	startHint, endHint, ok := callChainPrincipalEndpointHints(rm)
	if !ok {
		return ""
	}
	span, ok := callChainPrincipalSpanContextForEvidence(evidence, startHint, endHint)
	if !ok {
		return ""
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
		return ""
	}
	lines := gc.LineIndex[span.source]
	if len(lines) == 0 {
		return ""
	}
	missing := callChainMissingQualifiedIntermediateCalls(span, lines)
	if len(missing) == 0 {
		return ""
	}
	if callChainSpanHasDenseStructuredCoverage(span) {
		if closure != nil {
			callChainAddQualifiedIntermediateAdvisoryRepair(closure, span, missing)
		}
		logging.Info("[emit_investigation_complete] call-chain qualified intermediates kept advisory: dense structured coverage already present source=%s items=%d missing_candidates=%d",
			span.source, len(span.items), len(missing))
		return ""
	}
	if closure != nil {
		callChainAddQualifiedIntermediateAdvisoryRepair(closure, span, missing)
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — call-chain qualified intermediates lack typed handoff.\n\n")
	fmt.Fprintf(&b, "The `%s` → `%s` span in %s is already represented by typed endpoint evidence, but the already-read gutter contains qualified intermediate call candidates that are not present in `emit_evidence` yet. Do not close from read_file memory or thinking prose: re-emit grounded `emit_evidence` items for the load-bearing calls below, preserving their exact source lines in `snippet`, then retry `emit_investigation_complete`.\n\n",
		span.startHint, span.endHint, span.source)
	for _, call := range missing {
		fmt.Fprintf(&b, "  - %s:%d `%s` — %s\n", span.source, call.Line, call.Name, strings.TrimSpace(call.Text))
	}
	return b.String()
}

func callChainSpanHasDenseStructuredCoverage(span callChainPrincipalSpanContext) bool {
	if len(span.items) < 8 {
		return false
	}
	seenLines := map[int]bool{}
	for _, item := range span.items {
		if !callChainPrincipalSpanEvidenceEligible(item) || item.LineStart <= 0 {
			continue
		}
		seenLines[item.LineStart] = true
	}
	return len(seenLines) >= 8
}

func callChainAddQualifiedIntermediateAdvisoryRepair(closure *types.EvidenceClosure, span callChainPrincipalSpanContext, missing []callChainMissingQualifiedCall) {
	if closure == nil || len(missing) == 0 {
		return
	}
	keywords := make([]string, 0, len(missing)+2)
	keywords = append(keywords, span.startHint, span.endHint)
	for _, call := range missing {
		keywords = append(keywords, call.Name)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairEmitEvidence,
		Files:     []string{span.source},
		Keywords:  dedupStringsPreserveOrder(keywords),
		Rationale: "already-read source-to-sink call-chain span contains qualified call candidates that were not carried through typed evidence",
		Origin:    "pre_complete.call_chain_qualified_intermediate",
		Advisory:  true,
		Stage:     string(types.StageExplore),
	})
}

func callChainAggregateMemberSetCompletesPrincipalBoundary(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, evidence []types.EvidenceItem) bool {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentTrace && types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain {
		return false
	}
	if types.IsProjectOrientationQuestion(rm) {
		return false
	}
	startHint, endHint, ok := callChainPrincipalEndpointHints(rm)
	if !ok {
		return false
	}
	span, ok := callChainPrincipalSpanContextForEvidence(evidence, startHint, endHint)
	if !ok {
		return false
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(aggregateFacts, &rm)
	if len(refs) == 0 {
		refs = types.PrincipalAggregateMemberSetFactRefs(aggregateFacts)
	}
	var candidateFacts []types.AnswerAggregateFact
	for _, ref := range refs {
		candidateFacts = append(candidateFacts, ref.Fact)
	}
	if len(candidateFacts) == 0 {
		for _, fact := range aggregateFacts {
			if fact.Kind == types.AnswerAggregateMemberSet && types.NormalizeAnswerAggregateRole(fact.Role).IsPrincipal() {
				candidateFacts = append(candidateFacts, fact)
			}
		}
	}
	for _, fact := range candidateFacts {
		if len(fact.Members) < 2 {
			continue
		}
		if !aggregateMemberSetAllMembersUsable(fact, support) {
			continue
		}
		if !aggregateMemberSetHasSupportInsideCallChainSpan(fact, span) {
			continue
		}
		return true
	}
	return false
}

func narrativePrincipalMemberSetCompletesBoundary(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, evidence []types.EvidenceItem) bool {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil || view.Family != types.QFArchitecture {
		return false
	}
	if !types.IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(aggregateFacts, &rm)
	if len(refs) == 0 {
		return false
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	for _, ref := range refs {
		fact := ref.Fact
		if len(fact.Members) < 2 {
			continue
		}
		if aggregateMemberSetAllMembersUsable(fact, support) {
			return true
		}
	}
	return false
}

func genericForcedReadBoundarySatisfied(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, evidence []types.EvidenceItem) bool {
	if narrativePrincipalMemberSetCompletesBoundary(ctx, aggregateFacts, evidence) {
		return true
	}
	if relationMemberSetCompletesGenericForcedReadBoundary(ctx, aggregateFacts) {
		return true
	}
	if sourceInventoryMemberSetCompletesGenericForcedReadBoundary(ctx, aggregateFacts) {
		return true
	}
	if diagnosticMechanismGroundedEvidenceCompletesForcedReadBoundary(ctx, evidence) {
		return true
	}
	if genericForcedReadBoundarySatisfiedByGroundedEvidence(ctx, evidence) {
		return true
	}
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !genericForcedReadBoundaryCanUseModelPrincipalSet(rm) {
		return false
	}
	support := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
	for _, fact := range aggregateFacts {
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) < 2 {
			continue
		}
		if !aggregateFactCanDefineModelOwnedCompletionBoundary(fact) {
			continue
		}
		if aggregateMemberSetAllMembersUsable(fact, support) {
			return true
		}
	}
	return false
}

func diagnosticMechanismGroundedEvidenceCompletesForcedReadBoundary(ctx *types.BusContext, evidence []types.EvidenceItem) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(evidence) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentExplain && rm.Intent != types.IntentRootCause {
		return false
	}
	if !rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	switch types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case types.ReqMechanism, types.ReqConditional, types.ReqCallChain:
	default:
		return false
	}
	seenItems := make(map[string]bool)
	seenFiles := make(map[string]bool)
	for _, item := range evidence {
		if !evidenceItemSupportsGenericForcedReadBoundary(item) {
			continue
		}
		if !currentSourceCoverageEvidenceItem(item) {
			continue
		}
		if !diagnosticMechanismEvidenceKindSupportsBoundary(item.Kind) {
			continue
		}
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		key := fmt.Sprintf("%s:%d", source, item.LineStart)
		if seenItems[key] {
			continue
		}
		seenItems[key] = true
		seenFiles[source] = true
	}
	minItems, minFiles := diagnosticMechanismGroundedBoundaryThresholds(ctx)
	return len(seenItems) >= minItems && len(seenFiles) >= minFiles
}

func diagnosticMechanismEvidenceKindSupportsBoundary(kind types.EvidenceKind) bool {
	switch kind {
	case types.EvidenceMechanism,
		types.EvidenceRelationship,
		types.EvidenceConditional,
		types.EvidenceConcrete,
		types.EvidenceDirect,
		types.EvidenceRegistration:
		return true
	default:
		return false
	}
}

func diagnosticMechanismGroundedBoundaryThresholds(ctx *types.BusContext) (minItems int, minFiles int) {
	minItems, minFiles = 3, 2
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.AnswerContract.CitationReq.Required {
		if want := ctx.AnalysisIR.AnswerContract.CitationReq.MinCitations; want > minItems {
			minItems = want
		}
	}
	if minItems < 1 {
		minItems = 1
	}
	return minItems, minFiles
}

func sourceInventoryMemberSetCompletesGenericForcedReadBoundary(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.SourceInventoryPrincipalAuthorityActive(rm) {
		return false
	}
	return SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, aggregateFacts) ||
		SourceInventoryAcceptedClosureCoversExactUniverse(ctx, aggregateFacts)
}

func sourceInventoryMechanicalRowSetSatisfiesCitationFloor(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if rm.SourceInventoryProfile.RequestsSourceText() {
		return false
	}
	if gap := sourceInventoryResolvedCompletionPreciseCoverageGap(ctx, aggregateFacts); gap.Blocking {
		return false
	}
	return SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, aggregateFacts) ||
		SourceInventoryAcceptedClosureCoversExactUniverse(ctx, aggregateFacts)
}

func relationMemberSetCompletesGenericForcedReadBoundary(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.RequiresRelationMemberSetHandoff(rm) &&
		!exactTypedRelationProjectionActive(ctx, aggregateFacts) {
		return false
	}
	ok, _ := exhaustiveEnumerationMemberSetUsable(ctx, aggregateFacts)
	if !ok {
		return false
	}
	return len(relationMemberSetCoverageGaps(ctx, aggregateFacts)) == 0 &&
		len(relationPrincipalMemberSetScopeViolations(ctx, aggregateFacts)) == 0
}

func genericForcedReadBoundarySatisfiedByGroundedEvidence(ctx *types.BusContext, evidence []types.EvidenceItem) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(evidence) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !genericForcedReadBoundaryCanUseModelPrincipalSet(rm) {
		return false
	}
	minItems, minFiles := genericForcedReadGroundedBoundaryThresholds(ctx, rm)
	seenItems := make(map[string]bool)
	seenFiles := make(map[string]bool)
	for _, item := range evidence {
		if !evidenceItemSupportsGenericForcedReadBoundary(item) {
			continue
		}
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" || item.LineStart <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", source, item.LineStart)
		if seenItems[key] {
			continue
		}
		seenItems[key] = true
		seenFiles[source] = true
	}
	return len(seenItems) >= minItems && len(seenFiles) >= minFiles
}

func evidenceItemSupportsGenericForcedReadBoundary(item types.EvidenceItem) bool {
	if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		return false
	}
	if item.GroundingStatus != types.GroundingGrounded {
		return false
	}
	if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return false
	}
	return true
}

func genericForcedReadGroundedBoundaryThresholds(ctx *types.BusContext, rm types.RequestModel) (minItems int, minFiles int) {
	minItems = 2
	minFiles = 1
	if types.IsArchitectureNarrativeExplanation(rm) ||
		rm.Predicates.IsCrossComponent ||
		rm.Complexity == types.ComplexityComplex {
		minItems = 3
		minFiles = 2
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.AnswerContract.CitationReq.Required {
		if want := ctx.AnalysisIR.AnswerContract.CitationReq.MinCitations; want > minItems {
			minItems = want
		}
	}
	if minItems < 1 {
		minItems = 1
	}
	if minFiles < 1 {
		minFiles = 1
	}
	return minItems, minFiles
}

func genericForcedReadBoundaryCanUseModelPrincipalSet(rm types.RequestModel) bool {
	if types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm) {
		return false
	}
	if types.SourceInventoryPrincipalAuthorityActive(rm) &&
		(rm.Intent == types.IntentEnumerate || rm.Predicates.IsCategoryEnumeration || rm.QuestionStructure().HasAnyObligation()) {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	kind := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if rm.Intent != types.IntentExplain && !(kind == types.ReqCallChain && rm.Intent == types.IntentTrace) {
		return false
	}
	if types.IsArchitectureNarrativeExplanation(rm) || types.IsSingleTopicMechanismExplanation(rm) {
		return true
	}
	switch kind {
	case types.ReqMechanism, types.ReqConditional, types.ReqRegistration, types.ReqCallChain:
		return rm.Scenario == "" || rm.Scenario == types.ScenarioArchitectureExplain
	default:
		return false
	}
}

// completionExactCallChainEndpointShape reports the 件3 typed shape: the
// question declares exact targets AND rides the call predicate axis (the
// history+current explicit source→sink trace family). For that shape the
// phase1-ranked files are endpoint-proof candidates, so their pending reads
// stay citation-class (blocking) instead of demoting as breadth coverage.
// Both inputs are precise typed analyzer fields; no prose is inspected.
func completionExactCallChainEndpointShape(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	return len(rm.AnalyzerHints.ExactTargets) > 0 && rm.PredicateAxis == types.AxisCall
}

// completionDirectedCallChainEndpointShape selects the precise typed shapes
// for which source-to-sink reachability is a hard completion fact. Explicit
// Only CallChainEndpointProfile is directional authority. ExactTargets and
// entity lanes remain unordered identity sets even when they contain exactly
// two values; using their mention order would turn a schema-valid answer into
// a false reverse-path hard rejection. No raw request, model reasoning,
// completion reason, or final-answer prose is inspected.
//
// Keep this separate from completionExactCallChainEndpointShape: the latter
// also promotes phase-1 file reads to blocking and must retain its stronger
// explicit-target requirement.
func completionDirectedCallChainEndpointShape(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	_, _, ok := types.CallChainOrderedEndpointHints(ctx.AnalysisIR.RequestModel)
	return ok
}

// pendingReadIsPhase1Family reports whether a pending read came from the
// phase1 pre-scan ranking lane (the endpoint-candidate producer for the 件3
// shape above). Typed origin routing only.
func pendingReadIsPhase1Family(p types.PendingRead) bool {
	switch types.NormalizePendingReadOrigin(p.Origin) {
	case "phase1_unread", "pre_complete.phase1_unread":
		return true
	}
	return false
}

func partitionPendingReadsForAcceptedClosure(ctx *types.BusContext, pending []types.PendingRead, aggregateFacts []types.AnswerAggregateFact, evidence []types.EvidenceItem) ([]types.PendingRead, []types.PendingRead) {
	if len(pending) == 0 {
		return nil, nil
	}
	modelBoundary := genericForcedReadBoundarySatisfied(ctx, aggregateFacts, evidence)
	principalSpanWaived := false
	if ctx != nil && ctx.Mutable != nil {
		principalSpanWaived = ctx.Mutable.PrincipalSpanWaiver().IsActive()
	}
	blocking := make([]types.PendingRead, 0, len(pending))
	advisory := make([]types.PendingRead, 0)
	for _, p := range pending {
		if principalSpanWaived && types.NormalizePendingReadOrigin(p.Origin) == "pre_complete.call_chain_principal_span" {
			advisory = append(advisory, p)
			continue
		}
		if sourceInventoryAcceptedClosureDemotesPendingRead(ctx, aggregateFacts, p) {
			advisory = append(advisory, p)
			continue
		}
		if modelBoundary && mixedRuntimeCurrentSourceAcceptedBoundaryDemotesRequiredFilePendingRead(ctx, p) {
			advisory = append(advisory, p)
			continue
		}
		// 件3 (2026-07-13, 复核回收): the explicit-endpoint call-chain shape
		// (typed ExactTargets declared AND PredicateAxis==call) promotes its
		// phase1-family reads to citation class — the ranked files ARE the
		// endpoint-proof candidates there, not breadth suggestions.
		if completionExactCallChainEndpointShape(ctx) && pendingReadIsPhase1Family(p) {
			blocking = append(blocking, p)
			continue
		}
		// Coverage-class origins (§29.60 split, 2026-07-13): breadth /
		// navigation / ranker-sourced reads (phase1_unread, primary_anchor,
		// multi_path_anchor, chain_promotion.*) demote to advisory on EVERY
		// completion attempt — the model's completion call IS the boundary;
		// these reads suggest coverage, they do not support a citation the
		// answer makes. Citation-supporting and unknown origins (grounder
		// rejects, required-file obligations, empty origin) keep blocking —
		// the conservative default of PendingReadBlocksAcceptedClosure is
		// unchanged. Previously this demotion additionally required the
		// grounded model-owned boundary (genericForcedReadBoundarySatisfied),
		// which still gates the required-file overflow arm above.
		if !types.PendingReadBlocksAcceptedClosure(p) || types.IsGenericForcedReadOrigin(p.Origin) {
			advisory = append(advisory, p)
			continue
		}
		blocking = append(blocking, p)
	}
	return blocking, advisory
}

func mixedRuntimeCurrentSourceAcceptedBoundaryDemotesRequiredFilePendingRead(ctx *types.BusContext, pending types.PendingRead) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	origin := types.NormalizePendingReadOrigin(pending.Origin)
	if origin != "required_file_hint_unread" && origin != "pre_dispatch.required_file_hint_unread" {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
	if !mixedRuntimeCurrentSourceRequiredFileOverflowBoundaryApplies(ctx, rm, authority) {
		return false
	}
	cap := mixedRuntimeCurrentSourceRequiredFileOverflowCap(rm, authority)
	if cap <= 0 {
		return false
	}
	return highConfidenceRequiredFileHintCount(ctx, rm) > cap
}

func mixedRuntimeCurrentSourceRequiredFileOverflowBoundaryApplies(ctx *types.BusContext, rm types.RequestModel, authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if len(rm.AnalyzerHints.RequiredFileHints) == 0 ||
		!runtimeSourceAuthorityAppliesToCompletionLanding(authority) ||
		!runtimeSourceAuthorityHasRuntimeCarrier(authority) {
		return false
	}
	if authority.CanHardBlockCompletion &&
		!runtimeSourceAuthorityHardBlockComesOnlyFromRequiredFileHints(ctx, rm, authority) {
		return false
	}
	return authority.CurrentSourceRequired ||
		authority.CanDowngradeToCaveat ||
		authority.CanUseRuntimeOnlyWithCaveat ||
		authority.RuntimeObservationCount > 0
}

func mixedRuntimeCurrentSourceRequiredFileOverflowCap(rm types.RequestModel, authority types.RuntimeSourceAnswerAuthoritySnapshot) int {
	if runtimeSourceAuthorityAppliesToCompletionLanding(authority) && runtimeSourceAuthorityHasRuntimeCarrier(authority) {
		return types.MixedRuntimeCurrentSourceRequiredFileHintCoverageMax
	}
	return types.RequiredFileHintCoverageMaxForRequest(rm)
}

func runtimeSourceAuthorityHardBlockComesOnlyFromRequiredFileHints(ctx *types.BusContext, rm types.RequestModel, authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if ctx == nil || ctx.AnalysisIR == nil || !authority.CanHardBlockCompletion || len(rm.AnalyzerHints.RequiredFileHints) == 0 {
		return false
	}
	withoutHints := rm
	withoutHints.AnalyzerHints.RequiredFileHints = nil
	// Counterfactual sibling view: identical exported state except for
	// an AnalysisIR whose RequestModel has the required-file hints
	// stripped, so the authority snapshot can be rechecked without the
	// hints' contribution. ShallowClone, never `*ctx` (go vet
	// copylocks): BusContext carries sync.Mutex-guarded answer-surface
	// caches, and the clone's fresh empty cache is also semantically
	// right here — the cache key includes the AnalysisIR pointer, which
	// this view swaps, so an inherited entry could never legally hit.
	clonedCtx := ctx.ShallowClone()
	clonedIR := *ctx.AnalysisIR
	clonedIR.RequestModel = withoutHints
	clonedCtx.AnalysisIR = &clonedIR
	rechecked := runtimeSourceAnswerAuthorityForCompletion(clonedCtx)
	return runtimeSourceAuthorityAppliesToCompletionLanding(rechecked) &&
		runtimeSourceAuthorityHasRuntimeCarrier(rechecked) &&
		!rechecked.CanHardBlockCompletion
}

func highConfidenceRequiredFileHintCount(ctx *types.BusContext, rm types.RequestModel) int {
	count := 0
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		if _, ok := types.QualifyForcedReadSeedPath(ctx, hint.Path); !ok {
			continue
		}
		count++
	}
	return count
}

func sourceInventoryAcceptedClosureDemotesPendingRead(ctx *types.BusContext, aggregateFacts []types.AnswerAggregateFact, pending types.PendingRead) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(aggregateFacts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, aggregateFacts) &&
		!SourceInventoryAcceptedClosureCoversExactUniverse(ctx, aggregateFacts) {
		return false
	}
	if sourceInventoryMechanicalRequiredFilePendingReadCanBecomeAdvisory(rm, pending) {
		return true
	}
	principalFiles := types.PrincipalSourceInventoryMemberSetSourceFiles(aggregateFacts, &rm)
	if len(principalFiles) == 0 {
		return false
	}
	pendingFile := sourceInventoryAcceptedClosurePendingReadFileKey(pending.File, ctx.RepoRoot)
	if pendingFile == "" {
		return false
	}
	for _, file := range principalFiles {
		if sourceInventoryAcceptedClosurePendingReadFileKey(file, ctx.RepoRoot) == pendingFile {
			return false
		}
	}
	return true
}

func sourceInventoryMechanicalRequiredFilePendingReadCanBecomeAdvisory(rm types.RequestModel, pending types.PendingRead) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	origin := types.NormalizePendingReadOrigin(pending.Origin)
	if origin != "required_file_hint_unread" && origin != "pre_dispatch.required_file_hint_unread" {
		return false
	}
	if rm.SourceInventoryProfile.RequestsSourceText() {
		return false
	}
	return true
}

func sourceInventoryAcceptedClosurePendingReadFileKey(file, repoRoot string) string {
	key := types.CanonicalRequiredFileHintPath(file, repoRoot)
	if key == "" {
		return ""
	}
	return strings.ToLower(key)
}

func aggregateFactCanDefineModelOwnedCompletionBoundary(fact types.AnswerAggregateFact) bool {
	if types.NormalizeAnswerAggregateRole(fact.Role) == types.AnswerAggregateRolePrincipalAnswer {
		return true
	}
	prov := strings.ToLower(strings.TrimSpace(fact.Provenance))
	return strings.Contains(prov, "demoted:mechanism_narrative_support_member_set") ||
		strings.Contains(prov, "demoted:architecture_narrative_support_member_set")
}

func aggregateMemberSetAllMembersUsable(fact types.AnswerAggregateFact, support aggregateMemberSupportIndex) bool {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 {
		return false
	}
	for memberIdx, member := range fact.Members {
		if !aggregateMemberSetMemberUsableAt(fact, member, memberIdx, support) {
			return false
		}
	}
	return true
}

func aggregateMemberSetHasSupportInsideCallChainSpan(fact types.AnswerAggregateFact, span callChainPrincipalSpanContext) bool {
	source := canonicalCallChainSource(span.source)
	if source == "" || span.endLine <= span.startLine {
		return false
	}
	seen := map[int]bool{}
	for _, ref := range fact.SupportRefs {
		_, loc, ok := aggregateMemberSupportRefParts(ref)
		if !ok {
			continue
		}
		file, line, ok := parseAggregateSupportLocationKey(loc)
		if !ok || canonicalCallChainSource(file) != source {
			continue
		}
		if line <= span.startLine || line > span.endLine {
			continue
		}
		seen[line] = true
	}
	// Two citable points inside the source→sink span are enough to
	// prove the model emitted a structured principal path boundary.
	// The exact members remain model-owned; this only prevents the
	// generic span oracle from expanding the requested set to every
	// nearby helper call found in read_file gutters.
	return len(seen) >= 2
}

func parseAggregateSupportLocationKey(loc string) (string, int, bool) {
	loc = strings.TrimSpace(strings.ReplaceAll(loc, `\`, `/`))
	if loc == "" {
		return "", 0, false
	}
	idx := strings.LastIndex(loc, ":")
	if idx <= 0 || idx == len(loc)-1 {
		return "", 0, false
	}
	line, err := strconv.Atoi(strings.TrimSpace(loc[idx+1:]))
	if err != nil || line <= 0 {
		return "", 0, false
	}
	file := strings.TrimSpace(loc[:idx])
	if file == "" {
		return "", 0, false
	}
	return file, line, true
}

func callChainPrincipalSpanContextForEvidence(evidence []types.EvidenceItem, startHint, endHint string) (callChainPrincipalSpanContext, bool) {
	if len(evidence) == 0 {
		return callChainPrincipalSpanContext{}, false
	}
	bySource := make(map[string][]types.EvidenceItem)
	for _, item := range evidence {
		if !callChainPrincipalSpanEvidenceEligible(item) {
			continue
		}
		source := canonicalCallChainSource(item.Source)
		if source == "" {
			continue
		}
		item.Source = source
		bySource[source] = append(bySource[source], item)
	}
	for source, items := range bySource {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].LineStart == items[j].LineStart {
				return strings.TrimSpace(items[i].AnchorSymbol) < strings.TrimSpace(items[j].AnchorSymbol)
			}
			return items[i].LineStart < items[j].LineStart
		})
		startIdx := -1
		for i, item := range items {
			if callChainEvidenceMatchesEndpoint(item, startHint) {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			continue
		}
		endIdx := -1
		for i := len(items) - 1; i > startIdx; i-- {
			if callChainEvidenceMatchesEndpoint(items[i], endHint) {
				endIdx = i
				break
			}
		}
		if endIdx <= startIdx {
			continue
		}
		spanItems := append([]types.EvidenceItem(nil), items[startIdx:endIdx+1]...)
		return callChainPrincipalSpanContext{
			source:    source,
			startHint: startHint,
			endHint:   endHint,
			startLine: spanItems[0].LineStart,
			endLine:   spanItems[len(spanItems)-1].LineStart,
			items:     spanItems,
		}, true
	}
	return callChainPrincipalSpanContext{}, false
}

func callChainMissingQualifiedIntermediateCalls(span callChainPrincipalSpanContext, lines map[int]string) []callChainMissingQualifiedCall {
	if len(span.items) < 2 || len(lines) == 0 {
		return nil
	}
	const maxGapLines = 100
	const maxMissingCalls = 8
	covered := map[string]bool{}
	for _, item := range span.items {
		for _, term := range callChainEvidenceStructuredTerms(item) {
			if term == "" {
				continue
			}
			covered[strings.ToLower(term)] = true
			if tail := callChainEndpointTail(term); tail != "" {
				covered[strings.ToLower(tail)] = true
			}
		}
	}
	seen := map[string]bool{}
	var out []callChainMissingQualifiedCall
	for i := 0; i < len(span.items)-1; i++ {
		prevLine := span.items[i].LineStart
		nextLine := span.items[i+1].LineStart
		if prevLine <= 0 || nextLine <= prevLine+1 || nextLine-prevLine-1 > maxGapLines {
			continue
		}
		for line := prevLine + 1; line < nextLine; line++ {
			text := lines[line]
			for _, name := range callChainQualifiedCallCandidatesFromLine(text) {
				key := strings.ToLower(name)
				if covered[key] || covered[strings.ToLower(callChainEndpointTail(name))] || seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, callChainMissingQualifiedCall{Name: name, Line: line, Text: text})
				if len(out) >= maxMissingCalls {
					return out
				}
			}
		}
	}
	return out
}

func callChainEvidenceStructuredTerms(item types.EvidenceItem) []string {
	out := []string{
		item.Subject,
		item.Object,
		item.AnchorSymbol,
		item.OwnerSymbol,
	}
	out = append(out, item.SurfaceTerms...)
	return out
}

func callChainQualifiedCallCandidatesFromLine(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
		return nil
	}
	if strings.HasPrefix(line, "return ") || strings.HasPrefix(line, "throw ") || strings.HasPrefix(line, "raise ") {
		return nil
	}
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	matches := callChainQualifiedCallRe.FindAllString(line, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, match := range matches {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(match), "("))
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func callChainPrincipalEndpointHints(rm types.RequestModel) (string, string, bool) {
	return types.CallChainOrderedEndpointHints(rm)
}

func callChainEndpointHintLooksLikePath(raw string) bool {
	s := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") {
		return true
	}
	return types.HasCodeOrConfigPathSuffix(strings.ToLower(s))
}

func callChainPrincipalSpanDemandForEvidence(evidence []types.EvidenceItem, startHint, endHint string) (callChainPrincipalSpanDemand, bool) {
	if len(evidence) == 0 {
		return callChainPrincipalSpanDemand{}, false
	}
	bySource := make(map[string][]types.EvidenceItem)
	for _, item := range evidence {
		if !callChainPrincipalSpanEvidenceEligible(item) {
			continue
		}
		source := canonicalCallChainSource(item.Source)
		if source == "" {
			continue
		}
		item.Source = source
		bySource[source] = append(bySource[source], item)
	}
	for source, items := range bySource {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].LineStart == items[j].LineStart {
				return strings.TrimSpace(items[i].AnchorSymbol) < strings.TrimSpace(items[j].AnchorSymbol)
			}
			return items[i].LineStart < items[j].LineStart
		})
		var startItem types.EvidenceItem
		startLine := 0
		for _, item := range items {
			if callChainEvidenceMatchesEndpoint(item, startHint) {
				startItem = item
				startLine = item.LineStart
				break
			}
		}
		if startLine <= 0 {
			continue
		}
		var endItem types.EvidenceItem
		endLine := 0
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			if item.LineStart <= startLine {
				continue
			}
			if callChainEvidenceMatchesEndpoint(item, endHint) {
				endItem = item
				endLine = item.LineStart
				break
			}
		}
		if endLine <= startLine {
			continue
		}
		if callChainSpanEndpointPairIsStaticBinding(startItem, endItem) {
			continue
		}
		span := endLine - startLine
		// Default read_file lazy-limit override is 100 lines; add a
		// small buffer so this hard gate only fires on a genuinely
		// multi-window source-to-sink span, not compact straight-line
		// helpers where no intermediate hop is expected.
		const minSpanForLateCoverageGate = 120
		if span < minSpanForLateCoverageGate {
			continue
		}
		lateStart := startLine + (span * 3 / 4)
		if lateStart <= startLine {
			continue
		}
		hasLateInterior := false
		lastInterior := startLine
		for _, item := range items {
			if item.LineStart <= startLine || item.LineStart >= endLine {
				continue
			}
			if item.LineStart > lastInterior {
				lastInterior = item.LineStart
			}
			if item.LineStart >= lateStart {
				hasLateInterior = true
			}
		}
		if hasLateInterior {
			if gapStart, gapEnd, gapPrev, ok := callChainPrincipalTailGapDemand(items, startLine, endLine, lateStart); ok {
				return callChainPrincipalSpanDemand{
					source:       source,
					startHint:    startHint,
					endHint:      endHint,
					startLine:    startLine,
					endLine:      endLine,
					lateStart:    lateStart,
					demandStart:  gapStart,
					demandEnd:    gapEnd,
					lastInterior: gapPrev,
				}, true
			}
			continue
		}
		return callChainPrincipalSpanDemand{
			source:       source,
			startHint:    startHint,
			endHint:      endHint,
			startLine:    startLine,
			endLine:      endLine,
			lateStart:    lateStart,
			demandStart:  lateStart,
			demandEnd:    endLine - 1,
			lastInterior: lastInterior,
		}, true
	}
	return callChainPrincipalSpanDemand{}, false
}

func callChainPrincipalTailGapDemand(items []types.EvidenceItem, startLine, endLine, lateStart int) (int, int, int, bool) {
	const minPrincipalTailGap = 120
	if endLine <= startLine || lateStart <= startLine {
		return 0, 0, 0, false
	}
	prev := startLine
	for _, item := range items {
		line := item.LineStart
		if line <= startLine || line >= endLine {
			continue
		}
		if line > prev {
			gapStart := prev + 1
			if gapStart < lateStart {
				gapStart = lateStart
			}
			gapEnd := line - 1
			if gapEnd >= gapStart && gapEnd-gapStart+1 >= minPrincipalTailGap {
				return gapStart, gapEnd, prev, true
			}
		}
		if line > prev {
			prev = line
		}
	}
	gapStart := prev + 1
	if gapStart < lateStart {
		gapStart = lateStart
	}
	gapEnd := endLine - 1
	if gapEnd >= gapStart && gapEnd-gapStart+1 >= minPrincipalTailGap {
		return gapStart, gapEnd, prev, true
	}
	return 0, 0, 0, false
}

func callChainSpanEndpointPairIsStaticBinding(startItem, endItem types.EvidenceItem) bool {
	return callChainSpanEndpointIsStaticBinding(startItem) || callChainSpanEndpointIsStaticBinding(endItem)
}

func callChainSpanEndpointIsStaticBinding(item types.EvidenceItem) bool {
	if item.Kind == types.EvidenceRegistration {
		return true
	}
	if item.AnchorKind == types.AnchorInitializer || item.AnchorKind == types.AnchorAssignment {
		return true
	}
	pred := strings.ToLower(strings.TrimSpace(item.Predicate))
	return pred == "registers" || pred == "registered_by" || pred == "implements"
}

func callChainPrincipalSpanEvidenceEligible(item types.EvidenceItem) bool {
	if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return false
	}
	if item.GroundingStatus == types.GroundingUngrounded {
		return false
	}
	switch item.Kind {
	case types.EvidenceDirect, types.EvidenceConditional, types.EvidenceMechanism, types.EvidenceRelationship, types.EvidenceRegistration:
		return true
	default:
		return false
	}
}

func canonicalCallChainSource(raw string) string {
	source := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	source = strings.TrimPrefix(source, "./")
	if source == "." {
		return ""
	}
	return source
}

func callChainEvidenceMatchesEndpoint(item types.EvidenceItem, endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false
	}
	for _, candidate := range []string{
		item.Subject,
		item.Object,
		item.AnchorSymbol,
		item.OwnerSymbol,
	} {
		if callChainCodeTermMatches(candidate, endpoint) {
			return true
		}
	}
	for _, term := range item.SurfaceTerms {
		if callChainCodeTermMatches(term, endpoint) {
			return true
		}
	}
	return false
}

func callChainCodeTermMatches(candidate, endpoint string) bool {
	candidate = strings.TrimSpace(candidate)
	endpoint = strings.TrimSpace(endpoint)
	if candidate == "" || endpoint == "" {
		return false
	}
	return types.AnswerCodeIdentitySurfacesCompatible(candidate, endpoint)
}

func callChainEndpointTail(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "::", "."))
	if idx := strings.LastIndex(s, "."); idx >= 0 && idx+1 < len(s) {
		return s[idx+1:]
	}
	return s
}

func callChainDemandRangeFullyRead(closure *types.EvidenceClosure, source string, demand types.LineRange) bool {
	if closure == nil || source == "" || demand.Start <= 0 || demand.End < demand.Start {
		return false
	}
	if !closure.HasRead(source) {
		return false
	}
	ranges := closure.ReadRanges(source)
	if len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if r.Start <= demand.Start && r.End >= demand.End {
			return true
		}
	}
	return false
}

// requiresCrossFileCoverage reports whether the question kind should
// pay the cost of structural cross-file coverage guards. Pure breadth
// intents always qualify. Config-mapping questions are conditional:
// only multi-anchor cases need parity/unread protection.
func requiresCrossFileCoverage(k types.RequirementKind, primaryAnchors int) bool {
	switch k {
	case types.ReqMechanism, types.ReqCallChain, types.ReqConditional:
		return true
	case types.ReqConfigMapping:
		return primaryAnchors >= 2
	}
	return false
}

func sourceInventoryPhase1CoverageApplies(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	return types.HasBoundedSourceEnumerationScope(rm, ctx.AnalysisIR.EvidencePlan.RequiredFiles, ctx.RepoRoot)
}

func historyBackedCurrentCodeSkipsGenericForcedReadGates(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return types.IsHistoryBackedCurrentCodeExplanation(ctx.AnalysisIR.RequestModel)
}

func countPrimaryAnchorFiles(ranked []types.Phase1RankedFile) int {
	if len(ranked) == 0 {
		return 0
	}
	seen := make(map[string]bool, len(ranked))
	count := 0
	for _, rf := range ranked {
		if rf.ExactEntityRank <= 0 {
			continue
		}
		path := strings.TrimPrefix(strings.TrimSpace(rf.Path), "./")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		count++
	}
	return count
}

// detectSubjectViewMismatch returns true when the AnswerSubject is a
// source-code literal kind (function_name, type_name, interface,
// handler_route, return_value) but the compiled AnswerSemanticView's
// principal facet is the config-precedence family — i.e. the
// contract expects a config-key scalar but the subject points at a
// code symbol. The repair signal tells downstream that the family
// classification under-fits the user's actual subject.
//
// Returns (true, currentFamily, suggestedFamily) so the caller can
// emit a structured Repair with provenance metadata. fromFamily is
// always QFConfigPrecedence; toFamily is QFRoleLookup which is the
// closest scalar family for a code-symbol subject.
func detectSubjectViewMismatch(ir *types.AnalysisIR) (bool, types.QuestionFamily, types.QuestionFamily) {
	if ir == nil {
		return false, "", ""
	}
	subj := ir.RequestModel.AnswerSubject.Kind
	switch subj {
	case types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectReturnValue:
	default:
		return false, "", ""
	}
	family := types.ResolveQuestionFamily(ir.RequestModel)
	if family != types.QFConfigPrecedence {
		return false, "", ""
	}
	return true, family, types.QFRoleLookup
}

// evidenceTally breaks an evidence slice into grounding-status
// counts + the sub-slices a caller typically needs for rendering
// repair hints. Populated once by tallyEvidence and consumed by all
// three gate checks (groundingGateReject, tier1GateReject) + the
// hasGroundedOrRecovered predicate below, so the dispatch rules for
// GroundingStatus / GroundingTier live in one place.
//
// Legacy items with empty GroundingStatus (pre-session-5 concrete_value
// scans) count as Tier-1-grounded — they are deterministic facts, not
// LLM claims, and should NOT push either floor down.
type evidenceTally struct {
	total           int
	tier1           int
	tier2Grounded   int // GroundingGrounded but GroundingTier != TierLineText
	recovered       int
	ungrounded      int
	ungroundedItems []types.EvidenceItem
	tier2Items      []types.EvidenceItem
	recoveredItems  []types.EvidenceItem
}

// completionPreflightView is the typed, per-attempt snapshot shared by
// emit_investigation_complete's terminal gates. It intentionally carries
// only structured artifacts already accepted by earlier decode/normalize
// layers; hard gates must not inspect model prose to decide routing.
type completionPreflightView struct {
	Evidence                         []types.EvidenceItem
	EffectiveAggregateFacts          []types.AnswerAggregateFact
	StructuredRelationAuthorityFacts []types.AnswerAggregateFact
	GenericEvidenceTally             evidenceTally
	Tier1EvidenceTally               evidenceTally
}

func buildCompletionPreflightView(
	ctx *types.BusContext,
	resultKind string,
	justification string,
	evidence []types.EvidenceItem,
	aggregateFacts []types.AnswerAggregateFact,
	structuredRelationAuthorityFacts []types.AnswerAggregateFact,
) completionPreflightView {
	effective := effectiveCompletionAggregateFactsForValidation(ctx, aggregateFacts, evidence)
	relationFacts := effective
	if structuredRelationAuthorityFacts != nil {
		relationFacts = effectiveCompletionAggregateFactsForValidation(ctx, structuredRelationAuthorityFacts, evidence)
	}
	return buildCompletionPreflightViewFromEffective(ctx, resultKind, justification, evidence, effective, relationFacts)
}

func buildCompletionPreflightViewFromEffective(
	ctx *types.BusContext,
	resultKind string,
	justification string,
	evidence []types.EvidenceItem,
	effectiveAggregateFacts []types.AnswerAggregateFact,
	structuredRelationAuthorityFacts []types.AnswerAggregateFact,
) completionPreflightView {
	evidence = cloneCompletionEvidence(evidence)
	effectiveAggregateFacts = cloneCompletionAggregateFacts(effectiveAggregateFacts)
	if structuredRelationAuthorityFacts == nil {
		structuredRelationAuthorityFacts = effectiveAggregateFacts
	}
	structuredRelationAuthorityFacts = cloneCompletionAggregateFacts(structuredRelationAuthorityFacts)
	return completionPreflightView{
		Evidence:                         evidence,
		EffectiveAggregateFacts:          effectiveAggregateFacts,
		StructuredRelationAuthorityFacts: structuredRelationAuthorityFacts,
		GenericEvidenceTally:             completionGenericEvidenceTally(evidence),
		Tier1EvidenceTally:               completionTier1EvidenceTally(ctx, resultKind, justification, evidence),
	}
}

func (v completionPreflightView) WithAggregateFacts(
	effectiveAggregateFacts []types.AnswerAggregateFact,
	structuredRelationAuthorityFacts []types.AnswerAggregateFact,
) completionPreflightView {
	v.EffectiveAggregateFacts = cloneCompletionAggregateFacts(effectiveAggregateFacts)
	if structuredRelationAuthorityFacts == nil {
		structuredRelationAuthorityFacts = v.EffectiveAggregateFacts
	}
	v.StructuredRelationAuthorityFacts = cloneCompletionAggregateFacts(structuredRelationAuthorityFacts)
	return v
}

func cloneCompletionEvidence(in []types.EvidenceItem) []types.EvidenceItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.EvidenceItem, len(in))
	copy(out, in)
	return out
}

func completionGenericEvidenceTally(evidence []types.EvidenceItem) evidenceTally {
	return tallyEvidence(evidence, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
}

func completionTier1EvidenceTally(ctx *types.BusContext, resultKind string, justification string, evidence []types.EvidenceItem) evidenceTally {
	contract := exactResolutionContractForCompletion(ctx)
	scenario := types.ScenarioGeneric
	if ctx != nil && ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	stableAbsent := resultKind == "absence" && strings.TrimSpace(justification) != ""
	var mutable *types.MutableState
	if ctx != nil {
		mutable = ctx.Mutable
	}
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, mutable)
	rm := types.RequestModel{}
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = ctx.AnalysisIR.RequestModel
	}
	return tallyEvidence(evidence, contract, scenario, stableAbsent, requiredFiles, rm)
}

// tallyEvidence classifies each item in the evidence buffer and
// returns the populated tally. Single source of truth for how the
// pipeline counts grounding outcomes.
func tallyEvidence(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) evidenceTally {
	var t evidenceTally
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		t.total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			if e.GroundingTier == types.TierLineText {
				t.tier1++
			} else {
				t.tier2Grounded++
				t.tier2Items = append(t.tier2Items, e)
			}
		case types.GroundingRecovered:
			t.recovered++
			t.recoveredItems = append(t.recoveredItems, e)
		case types.GroundingUngrounded:
			t.ungrounded++
			t.ungroundedItems = append(t.ungroundedItems, e)
		default:
			t.tier1++
		}
	}
	return t
}

// acceptedTotal is (grounded + recovered) — the "at least it lined up
// somewhere" set used by the existing grounded-ratio gate.
func (t evidenceTally) acceptedTotal() int { return t.tier1 + t.tier2Grounded + t.recovered }

// hasAny reports whether at least one item is grounded or recovered.
// Powers the absence-vs-grounded contradiction gate.
func (t evidenceTally) hasAny() bool { return t.acceptedTotal() > 0 }

type Tier1RepairTarget struct {
	File  string
	Lines []int
}

func toolRepairTargetsForFiles(kind types.RepairKind, files []string) []types.ToolRepairTarget {
	if len(files) == 0 {
		return nil
	}
	action := strings.TrimSpace(string(kind))
	if action == "" {
		action = string(types.RepairReadFile)
	}
	seen := make(map[string]bool, len(files))
	out := make([]types.ToolRepairTarget, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, types.ToolRepairTarget{
			File:   file,
			Action: action,
		})
	}
	return out
}

func queueCompletionRepairs(ctx *types.BusContext, kind types.RepairKind, files []string, rationale, origin string) {
	if ctx == nil || ctx.Mutable == nil || len(files) == 0 {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	files = append([]string(nil), files...)
	for i := range files {
		files[i] = strings.TrimSpace(strings.ReplaceAll(files[i], `\`, `/`))
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      kind,
		Files:     files,
		Rationale: rationale,
		Origin:    origin,
	})
}

func completionFileRepair(kind types.RepairKind, code, hint string, files []string, metadata map[string]string) *types.ToolRepair {
	targets := toolRepairTargetsForFiles(kind, files)
	if code == "" && hint == "" && len(targets) == 0 && len(metadata) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code:     code,
		Hint:     hint,
		Targets:  targets,
		Metadata: metadata,
	}
}

func exactAbsenceContextRepairKind(ctx *types.BusContext, contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, requiredFiles []string) (types.RepairKind, []string) {
	files := dedupeCanonicalFiles(requiredFiles)
	if len(files) == 0 {
		return types.RepairReadFile, nil
	}
	if ctx == nil || ctx.Mutable == nil {
		return types.RepairReadFile, files
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return types.RepairReadFile, files
	}
	refreshClosureReadSnapshot(ctx, closure)
	var unread []string
	var materialize []string
	for _, file := range files {
		if !closure.HasRead(file) {
			unread = append(unread, file)
			continue
		}
		if evidenceNeedsMaterializationFromFile(contract, scenario, items, file, files) || closure.HasFullyRead(file) {
			materialize = append(materialize, file)
		}
	}
	if len(materialize) > 0 {
		// Prefer staying on already-read same-scope anchors before
		// widening to unread siblings. This keeps exact-absence
		// closure repairs focused on the current grounded branch and
		// avoids repo_map-ranked detours when the missing proof can
		// still be materialized from files already in read coverage.
		return types.RepairEmitEvidence, materialize
	}
	if len(unread) > 0 {
		return types.RepairReadFile, unread
	}
	return types.RepairReadFile, files
}

func evidenceNeedsMaterializationFromFile(contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, file string, requiredFiles []string) bool {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if contract == nil || file == "" || len(items) == 0 {
		return false
	}
	for _, item := range items {
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source != file {
			continue
		}
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		if types.ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
		if types.ExactResolutionAnswerContextAnchorAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
		if types.ExactResolutionContextSurfaceRelevant(contract, item) {
			return true
		}
	}
	return false
}

func dedupeCanonicalFiles(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func BuildTier1RepairTargets(repoRoot string, items []types.EvidenceItem) []Tier1RepairTarget {
	if len(items) == 0 {
		return nil
	}
	byFile := make(map[string]map[int]bool)
	for _, it := range items {
		file := ground.CanonicalRepoRelative(it.Source, repoRoot)
		if file == "" {
			continue
		}
		if byFile[file] == nil {
			byFile[file] = make(map[int]bool)
		}
		if it.LineStart > 0 {
			byFile[file][it.LineStart] = true
		}
	}
	if len(byFile) == 0 {
		return nil
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)
	out := make([]Tier1RepairTarget, 0, len(files))
	for _, file := range files {
		target := Tier1RepairTarget{File: file}
		for line := range byFile[file] {
			target.Lines = append(target.Lines, line)
		}
		sort.Ints(target.Lines)
		out = append(out, target)
	}
	return out
}

func buildTier1RepairTargets(ctx *types.BusContext, items []types.EvidenceItem) []Tier1RepairTarget {
	if ctx == nil {
		return nil
	}
	return BuildTier1RepairTargets(ctx.RepoRoot, items)
}

func Tier1LineList(lines []int, max int) string {
	if len(lines) == 0 {
		return ""
	}
	if max <= 0 || max > len(lines) {
		max = len(lines)
	}
	var parts []string
	for _, line := range lines[:max] {
		parts = append(parts, fmt.Sprintf("%d", line))
	}
	if len(lines) > max {
		parts = append(parts, fmt.Sprintf("+%d more", len(lines)-max))
	}
	return strings.Join(parts, ", ")
}

func tier1LineList(lines []int, max int) string {
	return Tier1LineList(lines, max)
}

func queueTier1ReadRepairs(ctx *types.BusContext, targets []Tier1RepairTarget) {
	if ctx == nil || ctx.Mutable == nil || len(targets) == 0 {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	for _, target := range targets {
		rationale := "Line-text grounding floor: read_file this source to convert non-line_text evidence into grounded evidence."
		if lines := tier1LineList(target.Lines, 4); lines != "" {
			rationale = fmt.Sprintf("Line-text grounding floor: read_file %s near lines %s to convert non-line_text evidence into grounded evidence.", target.File, lines)
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairReadFile,
			Files:     []string{target.File},
			Rationale: rationale,
			Origin:    "emit_investigation_complete.tier1_floor",
		})
	}
}

// groundingGateReject returns (message, ok). When ok=false, the
// returned message describes the gate miss and lists the ungrounded
// items with concrete repair options. When ok=true, the gate passed
// or was disabled (floor == 0).
func groundingGateReject(ctx *types.BusContext, floor float64) (string, bool) {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return groundingGateRejectWithEvidence(ctx, floor, evidence)
}

func groundingGateRejectWithEvidence(ctx *types.BusContext, floor float64, evidence []types.EvidenceItem) (string, bool) {
	view := buildCompletionPreflightViewFromEffective(ctx, "", "", evidence, nil, nil)
	return groundingGateRejectWithPreflight(ctx, floor, view)
}

func groundingGateRejectWithPreflight(ctx *types.BusContext, floor float64, view completionPreflightView) (string, bool) {
	if floor <= 0 {
		return "", true
	}
	evidence := view.Evidence
	if len(evidence) == 0 {
		// No emit_evidence calls at all — tool-only investigation is
		// still legitimate (exec_command one-shot, grep-only answer
		// for simple list questions). Accept.
		return "", true
	}
	t := view.GenericEvidenceTally
	if t.total == 0 {
		return "", true
	}
	ratio := float64(t.acceptedTotal()) / float64(t.total)
	if ratio >= floor {
		return "", true
	}
	leads := t.ungroundedItems
	grounded := t.tier1 + t.tier2Grounded
	recovered := t.recovered
	total := t.total
	var b strings.Builder
	fmt.Fprintf(&b,
		"emit_investigation_complete rejected: grounding ratio %.0f%% (%d grounded + %d recovered / %d total) < floor %.0f%%.\n\n",
		ratio*100, grounded, recovered, total, floor*100)
	b.WriteString("Ungrounded items cannot be emitted as citations:\n")
	maxList := 10
	for i, it := range leads {
		if i >= maxList {
			fmt.Fprintf(&b, "  ... and %d more\n", len(leads)-maxList)
			break
		}
		note := strings.TrimSpace(it.GroundingNote)
		if note == "" {
			note = "no tier accepted the citation"
		}
		anchor := it.AnchorSymbol
		if anchor == "" {
			anchor = "-"
		}
		fmt.Fprintf(&b, "  [%d] %s @ %s:%d (anchor_kind=%s, anchor_symbol=%s) — %s\n",
			i+1, it.Kind, it.Source, it.LineStart, it.AnchorKind, anchor, note)
	}
	b.WriteString("\nRepair options per item:\n")
	b.WriteString("  (A) call read_file on the source near the cited line so the line-text grounding pass can validate.\n")
	b.WriteString("  (B) re-emit with a different anchor_symbol — the identifier the grounder should find on that line.\n")
	b.WriteString("  (C) if you provided no snippet, add one (1-2 lines of actual code) so the snippet_fuzzy recovery tier can re-anchor.\n")
	b.WriteString("  (D) drop the item entirely if it was speculative — emit_evidence rejects of speculation do not hurt the investigation.\n")
	return b.String(), false
}

// tier1GateReject rejects emit_investigation_complete when the
// fraction of Tier-1-proven items (GroundingStatus=Grounded AND
// GroundingTier=TierLineText) falls below floor. Zero floor disables
// the gate (session-7 backward compat). Powers "don't let the LLM
// complete an investigation it never actually read" — the finalizer
// grounder's stricter Tier 2 would drop every citation anyway and
// stall the pipeline.
func tier1GateReject(ctx *types.BusContext, floor float64, resultKind, justification string) (string, bool) {
	var evidence []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return tier1GateRejectWithEvidence(ctx, floor, resultKind, justification, evidence)
}

func tier1GateRejectWithEvidence(ctx *types.BusContext, floor float64, resultKind, justification string, evidence []types.EvidenceItem) (string, bool) {
	view := buildCompletionPreflightViewFromEffective(ctx, resultKind, justification, evidence, nil, nil)
	return tier1GateRejectWithPreflight(ctx, floor, view)
}

func tier1GateRejectWithPreflight(ctx *types.BusContext, floor float64, view completionPreflightView) (string, bool) {
	if floor <= 0 {
		return "", true
	}
	evidence := view.Evidence
	if len(evidence) == 0 {
		return "", true
	}
	t := view.Tier1EvidenceTally
	if t.total == 0 {
		return "", true
	}
	ratio := float64(t.tier1) / float64(t.total)
	if ratio >= floor {
		return "", true
	}
	targets := buildTier1RepairTargets(ctx, append(append([]types.EvidenceItem(nil), t.tier2Items...), t.recoveredItems...))
	queueTier1ReadRepairs(ctx, targets)
	var b strings.Builder
	fmt.Fprintf(&b,
		"emit_investigation_complete rejected: line-text-grounded ratio %.0f%% (%d grounded-via-line_text / %d total) < floor %.0f%%.\n\n",
		ratio*100, t.tier1, t.total, floor*100)
	b.WriteString("Citation grounding at answer-render time is stricter than at evidence-emit time: if the cited sources were never read via read_file, the rendered answer will drop those citations and the pipeline will reject the result.\n\n")
	if len(targets) > 0 {
		b.WriteString("Repair: structured read_file repairs have been queued for the sources below. Read them, emit grounded evidence, then retry completion.\n")
	} else {
		b.WriteString("Repair: call read_file on the sources below so the line-text grounding pass can re-validate them.\n")
	}
	maxList := 6
	for i, target := range targets {
		if i >= maxList {
			fmt.Fprintf(&b, "  ... and %d more non-line_text sources\n", len(targets)-maxList)
			break
		}
		lines := tier1LineList(target.Lines, 4)
		if lines == "" {
			fmt.Fprintf(&b, "  [%d] %s — read_file %s and re-emit grounded evidence\n", i+1, target.File, target.File)
			continue
		}
		fmt.Fprintf(&b, "  [%d] %s — read_file near lines %s, then re-emit grounded evidence\n",
			i+1, target.File, lines)
	}
	if len(targets) == 0 {
		for i, it := range t.recoveredItems {
			if i >= maxList {
				fmt.Fprintf(&b, "  ... and %d more recovered-only items\n", len(t.recoveredItems)-maxList)
				break
			}
			anchor := it.AnchorSymbol
			if anchor == "" {
				anchor = "-"
			}
			fmt.Fprintf(&b, "  [%d] %s @ %s:%d (anchor_kind=%s, anchor_symbol=%s) — read_file %s near line %d to convert Recovered → Grounded\n",
				i+1, it.Kind, it.Source, it.LineStart, it.AnchorKind, anchor, it.Source, it.LineStart)
		}
	}
	return b.String(), false
}

// hasGroundedOrRecovered reports whether the evidence buffer contains
// at least one item whose grounder verdict is grounded or recovered.
// Drives the absence-vs-grounded contradiction gate in Execute.
func hasGroundedOrRecovered(items []types.EvidenceItem) bool {
	return completionGenericEvidenceTally(items).hasAny()
}

func allowsContextualEvidenceForAbsence(ctx *types.BusContext, evidence []types.EvidenceItem) bool {
	if groundedEvidenceIsContextOnly(evidence) {
		return true
	}
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil || !contract.AllowAbsence {
		return false
	}
	scenario := types.ScenarioGeneric
	if ctx != nil && ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	var requiredFiles []string
	if ctx != nil && ctx.Mutable != nil {
		requiredFiles = types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
	}
	targets := exactAbsencePendingTargets(ctx)
	if len(targets) == 0 {
		targets = append(targets, contract.Targets...)
	}
	if types.ExactResolutionAbsenceClosureReady(contract, scenario, targets, evidence, requiredFiles) {
		return true
	}
	// Once the upstream exact-target lane has already established a
	// pending "not found" target, absence closure may still be valid
	// even if the human-readable completion fields do not repeat the
	// literal on every line. In that state, supporting evidence is
	// allowed as long as no defining proof for the exact target was
	// emitted. The reason / absence_justification prose is explanatory
	// only and must not decide this gate.
	return len(targets) > 0 && !evidenceHasAnyDefiningExactTargetProof(contract, evidence, targets)
}

func groundedEvidenceIsContextOnly(items []types.EvidenceItem) bool {
	sawGrounded := false
	for _, item := range items {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		sawGrounded = true
		switch item.ContextRole {
		case types.EvidenceContextRoleAbsenceSupport,
			types.EvidenceContextRoleRelatedContext,
			types.EvidenceContextRoleIllustrativeOnly:
			continue
		default:
			return false
		}
	}
	return sawGrounded
}

func exactAbsencePendingTargets(ctx *types.BusContext) []string {
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil {
		return nil
	}
	unverified := unverifiedFindingsForCompletion(ctx)
	return types.ExactResolutionPendingTargets(contract, unverified)
}

func completionPendingExactTargets(ctx *types.BusContext, contract *types.ExactResolutionContract, evidence []types.EvidenceItem) []string {
	if contract == nil {
		return nil
	}
	if pending := exactAbsencePendingTargets(ctx); len(pending) > 0 {
		return pending
	}
	if ctx != nil && ctx.Mutable != nil &&
		strings.EqualFold(ctx.Mutable.StableInvestigationResultKind(), "absence") &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" {
		return nil
	}
	if evidenceHasAnyDefiningExactTargetProof(contract, evidence, contract.Targets) {
		return nil
	}
	return append([]string(nil), contract.Targets...)
}

func unverifiedFindingsForCompletion(ctx *types.BusContext) []types.UnverifiedFinding {
	var out []types.UnverifiedFinding
	if ctx == nil {
		return nil
	}
	if ctx.Mutable != nil {
		out = append(out, ctx.Mutable.EvidenceClosure().UnverifiedFindings()...)
	}
	return dedupeUnverifiedFindings(out)
}

func dedupeUnverifiedFindings(in []types.UnverifiedFinding) []types.UnverifiedFinding {
	seen := make(map[string]bool)
	var out []types.UnverifiedFinding
	for _, f := range in {
		token := strings.TrimSpace(f.Token)
		if token == "" {
			continue
		}
		kind := strings.TrimSpace(f.Kind)
		if kind == "" {
			kind = "symbol"
		}
		key := kind + "\x00" + token
		if seen[key] {
			continue
		}
		seen[key] = true
		f.Token = token
		f.Kind = kind
		out = append(out, f)
	}
	return out
}

func exactResolutionContractForCompletion(ctx *types.BusContext) *types.ExactResolutionContract {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	if c := answerExactResolutionContract(ctx); c != nil {
		return c
	}
	return types.BuildExactResolutionContract(ctx.AnalysisIR.RequestModel)
}

func normalizeExactAbsenceCompletionWithEvidenceAndAggregates(ctx *types.BusContext, resultKind, justification string, evidence []types.EvidenceItem, aggregateFacts []types.AnswerAggregateFact) (string, string) {
	resultKind, justification = normalizeExactAbsenceCompletionWithEvidence(ctx, resultKind, justification, evidence)
	if justification != "" || (resultKind != "absence" && resultKind != "resolved") {
		return resultKind, justification
	}
	if ctx == nil || ctx.Mutable == nil {
		return resultKind, justification
	}
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil || !contract.AllowAbsence || len(contract.Targets) == 0 {
		return resultKind, justification
	}
	if evidenceHasAnyDefiningExactTargetProof(contract, evidence, contract.Targets) {
		return resultKind, justification
	}
	if !aggregateFactsSupportExactAbsence(contract, aggregateFacts) {
		return resultKind, justification
	}
	auto := types.ExactResolutionAutoAbsenceJustification(contract)
	if auto == "" {
		return resultKind, justification
	}
	logging.Info("[emit_investigation_complete] synthesized exact-absence justification from negative_search aggregate (targets=%v)", contract.Targets)
	return "absence", auto
}

func normalizeExactAbsenceCompletionWithEvidence(ctx *types.BusContext, resultKind, justification string, evidence []types.EvidenceItem) (string, string) {
	if ctx == nil || ctx.Mutable == nil {
		return resultKind, justification
	}
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil || !contract.AllowAbsence || len(contract.Targets) == 0 {
		return resultKind, justification
	}
	if justification != "" && resultKind == "absence" {
		return resultKind, justification
	}
	scenario := types.ScenarioGeneric
	if ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
	if resultKind == "resolved" &&
		justification == "" &&
		strings.EqualFold(ctx.Mutable.StableInvestigationResultKind(), "absence") &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" &&
		!evidenceHasAnyDefiningExactTargetProof(contract, evidence, contract.Targets) {
		logging.Info("[emit_investigation_complete] preserving prior exact-absence closure over resolved retry without new defining proof (targets=%v)", contract.Targets)
		return "absence", ctx.Mutable.StableAbsenceJustification()
	}
	if !types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, requiredFiles) {
		return resultKind, justification
	}
	auto := types.ExactResolutionAutoAbsenceJustification(contract)
	if auto == "" {
		return resultKind, justification
	}
	if resultKind == "absence" && justification == "" {
		logging.Info("[emit_investigation_complete] synthesized exact-absence justification from structured evidence (targets=%v)", contract.Targets)
		return "absence", auto
	}
	if resultKind == "resolved" && justification == "" {
		logging.Info("[emit_investigation_complete] normalized result_kind resolved -> absence from structured exact-absence evidence (targets=%v)", contract.Targets)
		return "absence", auto
	}
	return resultKind, justification
}

func aggregateFactsSupportExactAbsence(contract *types.ExactResolutionContract, facts []types.AnswerAggregateFact) bool {
	if contract == nil || len(contract.Targets) == 0 || len(facts) == 0 {
		return false
	}
	for _, target := range contract.Targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if !aggregateFactsContainNegativeSearchForTarget(facts, target) {
			return false
		}
	}
	return true
}

func aggregateFactsContainNegativeSearchForTarget(facts []types.AnswerAggregateFact, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateNegativeSearch || strings.TrimSpace(fact.Value) != "0" {
			continue
		}
		fields := []string{fact.Label, fact.Provenance, fact.Unit}
		for _, dim := range fact.Dimensions {
			name := strings.ToLower(strings.TrimSpace(dim.Name))
			switch name {
			case "target", "query", "pattern", "literal", "symbol", "config_key", "path", "scope":
				fields = append(fields, dim.Value)
			}
		}
		for _, field := range fields {
			if strings.Contains(strings.ToLower(strings.TrimSpace(field)), target) {
				return true
			}
		}
	}
	return false
}

func dropUnsupportedDecoratedMemberSets(ctx *types.BusContext, facts []types.AnswerAggregateFact, contextLabel string, includePrincipal bool) ([]types.AnswerAggregateFact, []string) {
	if len(facts) == 0 {
		return facts, nil
	}
	contextLabel = strings.TrimSpace(contextLabel)
	if contextLabel == "" {
		contextLabel = "aggregate handoff"
	}
	out := make([]types.AnswerAggregateFact, 0, len(facts))
	var notes []string
	for _, fact := range facts {
		role := types.NormalizeAnswerAggregateRole(fact.Role)
		if fact.Kind != types.AnswerAggregateMemberSet ||
			role == types.AnswerAggregateRoleUnknown ||
			(role.IsPrincipal() && !includePrincipal) ||
			len(fact.SupportRefs) > 0 ||
			!aggregateMemberSetHasUnsupportedDecoratedCodeMembersForContext(ctx, fact) {
			out = append(out, fact)
			continue
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = "(unlabeled)"
		}
		notes = append(notes, fmt.Sprintf("dropped %s member_set %q without support_refs from %s", role, label, contextLabel))
	}
	return out, notes
}

func normalizeDecoratedMemberSetFormDebt(ctx *types.BusContext, resultKind string, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) ([]types.AnswerAggregateFact, []string) {
	if len(facts) == 0 {
		return facts, nil
	}
	out := cloneCompletionAggregateFacts(facts)
	var notes []string
	changedAny := false
	var lazySupport *aggregateMemberSupportIndex
	supportIndex := func() aggregateMemberSupportIndex {
		if lazySupport == nil {
			idx := buildAggregateMemberSupportIndexWithEvidence(ctx, evidence)
			lazySupport = &idx
		}
		return *lazySupport
	}
	for i := range out {
		fact := &out[i]
		if fact.Kind != types.AnswerAggregateMemberSet ||
			len(fact.Members) == 0 {
			continue
		}
		// SUPPREF-TOL (§29.104.13): support_refs used to block this repair
		// lane by mere PRESENCE. The h9 witness fact carried exactly one
		// decorated prose ref ("attached_trace.txt: wakeup_chain path") that
		// no parser or lookup lane can consume — strictly less grounding
		// than emitting no refs at all, yet it disabled the bare-surface
		// repair and the whole emit was DOWNGRADED. Only refs that are
		// actually CONSUMABLE (verbatim or decoration-stripped: evidence-id
		// hit, source-inventory hit, or a parseable location) keep blocking,
		// because those may carry load-bearing member associations the
		// rewrite could desync. Junk refs stay on the fact (lossless) but no
		// longer veto the repair.
		if len(fact.SupportRefs) > 0 &&
			aggregateFactAnySupportRefConsumable(fact.SupportRefs, supportIndex()) {
			continue
		}
		if !decoratedMemberSetFormRepairEligible(ctx, resultKind, *fact) {
			continue
		}
		memberNotes := append([]string(nil), fact.MemberNotes...)
		for len(memberNotes) < len(fact.Members) {
			memberNotes = append(memberNotes, "")
		}
		changed := false
		for j, member := range fact.Members {
			if decoratedAggregateMemberCanRelyOnOriginSpecificProvenance(ctx, *fact, member) {
				continue
			}
			base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(member)
			base = strings.TrimSpace(base)
			qualifier = strings.TrimSpace(qualifier)
			if !ok || base == "" || !types.IsCodeIdentitySurface(base) {
				continue
			}
			fact.Members[j] = base
			if qualifier != "" && strings.TrimSpace(memberNotes[j]) == "" {
				memberNotes[j] = qualifier
			}
			changed = true
		}
		if !changed {
			continue
		}
		fact.MemberNotes = memberNotes
		fact.Provenance = appendCompletionAggregateProvenance(fact.Provenance, "form_repair:decorated_member_base")
		notes = append(notes, fmt.Sprintf("normalized member_set %q decorated member labels into bare member surfaces with member_notes", completionFirstNonEmptyString(fact.Label, "(unlabeled)")))
		changedAny = true
	}
	if !changedAny {
		return facts, nil
	}
	normalized, err := types.NormalizeAnswerAggregateFacts(out)
	if err != nil {
		notes = append(notes, fmt.Sprintf("ignored decorated member form repair after validation error: %v", err))
		return facts, notes
	}
	return normalized, notes
}

func decoratedMemberSetFormRepairEligible(ctx *types.BusContext, resultKind string, fact types.AnswerAggregateFact) bool {
	if !strings.EqualFold(strings.TrimSpace(resultKind), "resolved") {
		return false
	}
	rm := requestModelForAggregateSupport(ctx)
	// SUPPREF-TOL (§29.104.13): under a typed explicit user exclude boundary
	// ("只分析这份 trace，不分析代码" — CSP #63 chokepoint precedent in
	// AnswerAggregateFactEvidenceOrigins), a current-source grounding
	// requirement is semantically impossible for this run, so it must not
	// veto the bare-surface form repair. In the h9 witness the requirement
	// bit came from blob reads of the engine's own trace-query result
	// polluting the current-source census (CSP #63 family, root fix owned
	// there); honoring it here left the decorated members with no
	// non-downgrade landing in an exclude run. The user boundary is the
	// stronger precise signal and outranks the derived authority bit for
	// THIS repair lane only — proof lanes and the authority snapshot itself
	// are untouched.
	excludesCurrentSource := rm != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()
	if !excludesCurrentSource && aggregateMemberSetOriginRequiresCurrentSource(ctx, rm, []types.AnswerAggregateFact{fact}) {
		return false
	}
	if authority := runtimeSourceAnswerAuthorityForCompletion(ctx); runtimeSourceAuthorityAppliesToCompletionLanding(authority) &&
		runtimeSourceAuthorityAllowsRuntimeCompletionLandingSnapshot(authority) &&
		runtimeSourceCompletionLandingRequiresExplicitRuntimeOrigin(ctx, rm) &&
		!aggregateFactHasExplicitRuntimeArtifactOrigin(fact) {
		return false
	}
	origins := types.AnswerAggregateFactEvidenceOrigins(fact, rm)
	for _, origin := range origins {
		if !types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			continue
		}
		// SUPPREF-TOL (§29.104.13): the runtime-artifact origin only blocks
		// this repair when the origin-specific bypass will actually protect
		// the fact's decorated members at the support_refs gate. In the h9
		// witness state the runtime landing snapshot was disallowed
		// (current-source lane load-bearing), so the gate refused the bypass
		// while THIS pre-filter still assumed it — the fact fell between the
		// two halves and the emit was wholesale DOWNGRADED. When the bypass
		// holds, repair stays blocked exactly as before (the per-member loop
		// would no-op anyway); when it does not, the bare-surface repair is
		// the fact's only non-downgrade landing. Other origin-specific
		// origins (VCS, command, external documents, …) keep today's block.
		if origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			!aggregateFactOriginLanesProtectAllDecoratedMembers(ctx, fact) {
			continue
		}
		return false
	}
	return true
}

// aggregateFactOriginLanesProtectAllDecoratedMembers reports whether every
// decorated code-identity member of the fact would be bypassed by
// decoratedAggregateMemberCanRelyOnOriginSpecificProvenance at the
// support_refs gate. A fact with no decorated member returns true: there is
// nothing for the form repair to rewrite, so blocking it preserves today's
// no-op path.
func aggregateFactOriginLanesProtectAllDecoratedMembers(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	for _, member := range fact.Members {
		if !memberHasDecoratedCodeIdentityBase(member) {
			continue
		}
		if !decoratedAggregateMemberCanRelyOnOriginSpecificProvenance(ctx, fact, member) {
			return false
		}
	}
	return true
}

// aggregateFactAnySupportRefConsumable reports whether at least one
// support_refs entry can actually be consumed by the resolution lanes —
// verbatim or after decoration stripping: an emitted-evidence id, a
// source-inventory support ref, or a parseable file:line location. Refs that
// fail all three even after stripping cannot ground any member and must not
// veto the bare-surface form repair.
func aggregateFactAnySupportRefConsumable(refs []string, support aggregateMemberSupportIndex) bool {
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		for _, candidate := range aggregateSupportRefLookupCandidates(ref) {
			if _, ok := support.byID[candidate]; ok {
				return true
			}
			if len(support.sourceInventoryLabelsBySupport[candidate]) > 0 {
				return true
			}
		}
		if _, loc, ok := aggregateMemberSupportRefPartsTolerant(ref); ok && strings.TrimSpace(loc) != "" {
			return true
		}
	}
	return false
}

func completionFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func appendCompletionAggregateProvenance(current, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return current
	}
	if current == "" {
		return addition
	}
	for _, part := range strings.Split(current, ";") {
		if strings.TrimSpace(part) == addition {
			return current
		}
	}
	return current + "; " + addition
}

func aggregateMemberSetHasUnsupportedDecoratedCodeMembers(fact types.AnswerAggregateFact) bool {
	return aggregateMemberSetHasUnsupportedDecoratedCodeMembersForContext(nil, fact)
}

func aggregateMemberSetHasUnsupportedDecoratedCodeMembersForContext(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if fact.Kind != types.AnswerAggregateMemberSet || len(fact.SupportRefs) > 0 {
		return false
	}
	for _, member := range fact.Members {
		if !memberHasDecoratedCodeIdentityBase(member) {
			continue
		}
		if decoratedAggregateMemberCanRelyOnOriginSpecificProvenance(ctx, fact, member) {
			continue
		}
		return true
	}
	return false
}

func evidenceHasAnyDefiningExactTargetProof(contract *types.ExactResolutionContract, items []types.EvidenceItem, targets []string) bool {
	if contract == nil || len(items) == 0 {
		return false
	}
	for _, it := range items {
		switch it.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		if it.ContextRole == types.EvidenceContextRoleIllustrativeOnly ||
			it.ContextRole == types.EvidenceContextRoleAbsenceSupport ||
			it.ContextRole == types.EvidenceContextRoleRelatedContext {
			continue
		}
		if strings.TrimSpace(string(it.AnchorKind)) == "" {
			continue
		}
		if !evidenceProofAnchorsAnyListedExactTarget(contract, it, targets) {
			continue
		}
		if types.IsNegativeEvidencePredicate(it.Predicate) {
			continue
		}
		if !types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, it.Source) {
			continue
		}
		return true
	}
	return false
}

func evidenceProofAnchorsAnyListedExactTarget(contract *types.ExactResolutionContract, item types.EvidenceItem, targets []string) bool {
	for _, target := range targets {
		if types.ExactResolutionTextMentionsTarget(contract, item.AnchorSymbol, target) ||
			types.ExactResolutionTextMentionsTarget(contract, item.Object, target) {
			return true
		}
		if strings.TrimSpace(item.AnchorSymbol) == "" &&
			types.ExactResolutionTextMentionsTarget(contract, item.Subject, target) {
			return true
		}
	}
	return false
}

func evidenceDirectlyAnchorsAnyListedExactTarget(contract *types.ExactResolutionContract, item types.EvidenceItem, targets []string) bool {
	for _, target := range targets {
		if types.ExactResolutionTextMentionsTarget(contract, item.AnchorSymbol, target) ||
			types.ExactResolutionTextMentionsTarget(contract, item.Object, target) {
			return true
		}
		if strings.TrimSpace(item.AnchorSymbol) == "" &&
			types.ExactResolutionTextMentionsTarget(contract, item.Subject, target) {
			return true
		}
	}
	return false
}

func evidenceHasGroundedRelatedContextProof(contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, requiredFiles []string) bool {
	if contract == nil || len(items) == 0 {
		return false
	}
	for _, item := range items {
		if types.ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
	}
	if scenario == types.ScenarioConfigTrace && contract.TargetKind == types.SubjectConfigKey {
		return false
	}
	for _, item := range items {
		if types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, item, requiredFiles) {
			return true
		}
	}
	return false
}

// G1 (Plan E rollout 2026-05-02) was implemented here as
// computePreCompleteCoverageGap and reverted the same day after the
// s1a regression — see the comment block above the resultKind check
// for the architectural lesson. Function deleted to keep the file
// honest about what's enforced and what's not.

// completionFactsContainMemberSet reports whether any aggregate fact
// hands over an exact member set (kind=member_set with at least one
// member) — the per-member-table completion obligation's carrier.
func completionFactsContainMemberSet(facts []types.AnswerAggregateFact) bool {
	for _, f := range facts {
		if strings.EqualFold(strings.TrimSpace(string(f.Kind)), "member_set") && len(f.Members) > 0 {
			return true
		}
	}
	return false
}
