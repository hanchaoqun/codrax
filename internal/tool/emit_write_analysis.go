package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitWriteAnalysis is the write_analyzer agent's structured exit
// channel. The LLM characterises the user's code-change request and
// emits a WriteAnalysisIR via this tool. The system validates the
// emit and stores it on Mutable.SetWriteAnalysisIR — downstream
// write agents (planner / coder / verifier / reflector) read from
// there.
//
// Mirror of EmitPerfTrace's shape: ReadOnly + NonEvidenceTool because
// the emission mutates BusContext but produces no citable repo fact.
// The tool is only registered for the write_analyzer agent's tool
// allowlist (BaseAgent.toolset) so a stray call from a read-mode
// agent is structurally impossible.
type EmitWriteAnalysis struct {
	ReadOnly
	NonEvidenceTool
}

type emitWriteAnalysisParams struct {
	RawRequest         string                          `json:"raw_request"`
	Task               emitWriteAnalysisTask           `json:"task"`
	Risk               emitWriteAnalysisRisk           `json:"risk"`
	ScopeAnchors       []string                        `json:"scope_anchors,omitempty"`
	Constraints        []emitWriteAnalysisConstraint   `json:"constraints,omitempty"`
	ExpectedOutcomes   []string                        `json:"expected_outcomes,omitempty"`
	BehaviorContracts  []types.WriteBehaviorContract   `json:"behavior_contracts,omitempty"`
	PhaseProposal      *emitWriteAnalysisPhaseProposal `json:"phase_proposal,omitempty"`
	ApplicablePitfalls []string                        `json:"applicable_pitfalls,omitempty"`
}

type emitWriteAnalysisTask struct {
	Kind    string `json:"kind"`
	Scope   string `json:"scope"`
	Summary string `json:"summary"`
}

type emitWriteAnalysisRisk struct {
	AffectsPublicAPI   bool   `json:"affects_public_api"`
	ChangesPersistence bool   `json:"changes_persistence"`
	ChangesBuildSystem bool   `json:"changes_build_system"`
	Overall            string `json:"overall"`
}

type emitWriteAnalysisConstraint struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
	Note   string `json:"note,omitempty"`
}

type emitWriteAnalysisPhaseProposal struct {
	Split  string                       `json:"split"`
	Phases []emitWriteAnalysisPhaseSeed `json:"phases,omitempty"`
}

type emitWriteAnalysisPhaseSeed struct {
	Goal             string   `json:"goal"`
	RoughTargetPaths []string `json:"rough_target_paths,omitempty"`
}

// Name returns the tool's stable identifier.
func (t *EmitWriteAnalysis) Name() string { return "emit_write_analysis" }

// Description — one short sentence; strategy guidance lives in
// write-analysis-skill.
func (t *EmitWriteAnalysis) Description() string {
	return "Emits the structured write-analysis record characterising the user's code-change request: task category, scope, risk, constraints, and expected outcomes. Call EXACTLY once per dispatch. " +
		"The system validates enum values + cross-field consistency before storing the IR — invalid emits are rejected with the schema reminder so the next attempt can correct."
}

// Parameters returns the strict JSON schema.
func (t *EmitWriteAnalysis) Parameters() json.RawMessage {
	emitWriteAnalysisSchemaOnce.Do(buildEmitWriteAnalysisSchemaBytes)
	return emitWriteAnalysisSchemaCache
}

var (
	emitWriteAnalysisSchemaOnce  sync.Once
	emitWriteAnalysisSchemaCache json.RawMessage
)

func buildEmitWriteAnalysisSchemaBytes() {
	schema := buildEmitWriteAnalysisSchema()
	raw, err := json.Marshal(schema)
	if err != nil {
		emitWriteAnalysisSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_write_analysis schema build failed: %s"}`, err))
		return
	}
	emitWriteAnalysisSchemaCache = raw
}

// Execute parses + validates + stores WriteAnalysisIR on MutableState.
func (t *EmitWriteAnalysis) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_write_analysis requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	var p emitWriteAnalysisParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil, params)
	}

	// Required field checks. RawRequest must be non-empty (the LLM
	// echoes the user's request as a self-check); Task.Summary must
	// be non-empty (the planner uses it as a per-iteration title).
	if strings.TrimSpace(p.RawRequest) == "" {
		return errResult(t.Name(), "emit_write_analysis rejected: raw_request is empty"), nil
	}
	if strings.TrimSpace(p.Task.Summary) == "" {
		return errResult(t.Name(), "emit_write_analysis rejected: task.summary is empty"), nil
	}

	// Enum validation with deterministic fall-back. Unknown values
	// fall back to the safest option rather than rejecting — keeps
	// the LLM moving forward when it is creative with labels.
	taskKind := types.WriteTaskKind(strings.ToLower(strings.TrimSpace(p.Task.Kind)))
	if !types.IsKnownWriteTaskKind(string(taskKind)) {
		taskKind = types.WriteTaskMisc
	}
	scope := types.WriteScope(strings.ToLower(strings.TrimSpace(p.Task.Scope)))
	if !types.IsKnownWriteScope(string(scope)) {
		scope = types.ScopeMicro
	}
	overall := types.WriteRiskBand(strings.ToLower(strings.TrimSpace(p.Risk.Overall)))
	if !types.IsKnownWriteRiskBand(string(overall)) {
		overall = types.RiskBandLow
	}

	// Phase proposal validation. A "sequential" split with fewer than
	// 2 phases collapses to "single" — a single-phase sequential
	// proposal is a contradiction and the planner should treat it as
	// a normal one-shot plan.
	phase := types.PhaseProposal{Split: "single"}
	if p.PhaseProposal != nil {
		split := strings.ToLower(strings.TrimSpace(p.PhaseProposal.Split))
		if !types.IsKnownPhaseSplit(split) {
			split = "single"
		}
		seeds := make([]types.PhaseSeed, 0, len(p.PhaseProposal.Phases))
		for _, s := range p.PhaseProposal.Phases {
			if strings.TrimSpace(s.Goal) == "" {
				continue
			}
			seeds = append(seeds, types.PhaseSeed{
				Goal:             strings.TrimSpace(s.Goal),
				RoughTargetPaths: dedupAndTrim(s.RoughTargetPaths),
			})
		}
		if (split == "sequential" || split == "parallel") && len(seeds) < 2 {
			split = "single"
			seeds = nil
		}
		// Hard-cap phase count at the operator-tunable resolved
		// maximum (default MaxPhasesPerGroup=5; yaml override
		// `pipeline_max_phases_per_run` clamped to
		// MaxPhasesPerGroupCeil=12). Real-world multi-phase
		// tasks are 2-3 phases; >5 almost always means the LLM
		// mis-classified what should be a single phase as
		// multiple. Truncate rather than reject so a near-cap
		// emit doesn't fail the entire dispatch.
		maxPhases := types.ResolvedMaxPhases()
		if len(seeds) > maxPhases {
			logging.Warning("[emit_write_analysis] phase proposal had %d phases; capping at %d",
				len(seeds), maxPhases)
			seeds = seeds[:maxPhases]
		}
		phase.Split = split
		phase.Phases = seeds
	}

	// Constraint normalisation. Drop entries with empty Kind — the
	// LLM occasionally emits placeholder rows when no real constraint
	// applies; let those go cleanly.
	constraints := make([]types.WriteConstraint, 0, len(p.Constraints))
	for _, c := range p.Constraints {
		if strings.TrimSpace(c.Kind) == "" {
			continue
		}
		constraints = append(constraints, types.WriteConstraint{
			Kind:   strings.TrimSpace(c.Kind),
			Target: strings.TrimSpace(c.Target),
			Note:   strings.TrimSpace(c.Note),
		})
	}

	expectedOutcomes := dedupAndTrim(p.ExpectedOutcomes)
	if msg := validateWriteBehaviorContractEnums(p.BehaviorContracts); msg != "" {
		return errResult(t.Name(), msg), nil
	}
	contracts := types.NormalizeWriteBehaviorContracts(p.BehaviorContracts, expectedOutcomes)
	ir := &types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest: strings.TrimSpace(p.RawRequest),
			Task: types.WriteTask{
				Kind:    taskKind,
				Scope:   scope,
				Summary: strings.TrimSpace(p.Task.Summary),
			},
			Risk: types.WriteRiskProfile{
				AffectsPublicAPI:   p.Risk.AffectsPublicAPI,
				ChangesPersistence: p.Risk.ChangesPersistence,
				ChangesBuildSystem: p.Risk.ChangesBuildSystem,
				Overall:            overall,
			},
			ScopeAnchors:      dedupAndTrim(p.ScopeAnchors),
			Constraints:       constraints,
			ExpectedOutcomes:  expectedOutcomes,
			BehaviorContracts: contracts,
		},
		PhaseProposal:   phase,
		PitfallsApplied: dedupAndTrim(p.ApplicablePitfalls),
	}

	ctx.Mutable.SetWriteAnalysisIR(ir)

	logging.Debug("[emit_write_analysis] stored: kind=%s scope=%s overall=%s constraints=%d outcomes=%d contracts=%d phases=%s/%d",
		ir.Request.Task.Kind, ir.Request.Task.Scope, ir.Request.Risk.Overall,
		len(ir.Request.Constraints), len(ir.Request.ExpectedOutcomes), len(ir.Request.BehaviorContracts),
		ir.PhaseProposal.Split, len(ir.PhaseProposal.Phases))

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf("write analysis stored: kind=%s scope=%s risk=%s outcomes=%d contracts=%d",
			ir.Request.Task.Kind, ir.Request.Task.Scope, ir.Request.Risk.Overall,
			len(ir.Request.ExpectedOutcomes), len(ir.Request.BehaviorContracts)),
		Timestamp: time.Now(),
	}, nil
}

// dedupAndTrim returns a slice with each input string trimmed and
// duplicates removed (case-sensitive). Preserves first-occurrence
// order.
func dedupAndTrim(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		v := strings.TrimSpace(s)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func validateWriteBehaviorContractEnums(contracts []types.WriteBehaviorContract) string {
	for i, c := range contracts {
		kind := strings.ToLower(strings.TrimSpace(string(c.Kind)))
		if kind != "" && !types.IsKnownWriteBehaviorContractKind(kind) {
			return fmt.Sprintf("emit_write_analysis rejected: behavior_contracts[%d].kind=%q is unsupported; use one of observable, exception, output_path, stdout, status_code, file_layout, command_result, invariant", i, c.Kind)
		}
		operator := strings.ToLower(strings.TrimSpace(string(c.Operator)))
		if operator != "" && !types.IsKnownWriteBehaviorOperator(operator) {
			return fmt.Sprintf("emit_write_analysis rejected: behavior_contracts[%d].operator=%q is unsupported; use one of satisfies, equals, not_equals, contains, not_contains, exists, not_exists, raises, not_raises, returns", i, c.Operator)
		}
		if c.Comparator != nil {
			compOperator := strings.ToLower(strings.TrimSpace(string(c.Comparator.Operator)))
			if compOperator != "" && !types.IsKnownWriteBehaviorOperator(compOperator) {
				return fmt.Sprintf("emit_write_analysis rejected: behavior_contracts[%d].comparator.operator=%q is unsupported; use one of satisfies, equals, not_equals, contains, not_contains, exists, not_exists, raises, not_raises, returns", i, c.Comparator.Operator)
			}
			relation := strings.ToLower(strings.TrimSpace(string(c.Comparator.Relation)))
			if relation != "" && !types.IsKnownWriteBehaviorComparatorRelation(relation) {
				return fmt.Sprintf("emit_write_analysis rejected: behavior_contracts[%d].comparator.relation=%q is unsupported; use one of same_as, consistent_with, contrasts_with, regression_baseline", i, c.Comparator.Relation)
			}
		}
		polarity := strings.ToLower(strings.TrimSpace(string(c.Polarity)))
		if polarity != "" && !types.IsKnownWriteBehaviorPolarity(polarity) {
			return fmt.Sprintf("emit_write_analysis rejected: behavior_contracts[%d].polarity=%q is unsupported; use expected, forbidden, or observed", i, c.Polarity)
		}
	}
	return ""
}

func buildEmitWriteAnalysisSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"raw_request", "task", "risk"},
		"properties": map[string]any{
			"raw_request": map[string]any{
				"type":        "string",
				"description": "The user's request, echoed verbatim (or trivially trimmed) so the validator can sanity-check that you read the same input the system loaded.",
			},
			"task": map[string]any{
				"type":     "object",
				"required": []string{"kind", "scope", "summary"},
				"properties": map[string]any{
					"kind": map[string]any{
						"type":        "string",
						"enum":        []string{"feature", "bugfix", "refactor", "test", "docs", "config", "misc"},
						"description": "Which category of code change. feature = new functionality. bugfix = correct wrong behaviour. refactor = reshape without behaviour change. test = add/improve tests. docs = documentation/comments only. config = configuration/dependency manifest changes. misc = none of the above.",
					},
					"scope": map[string]any{
						"type":        "string",
						"enum":        []string{"micro", "package", "cross", "project"},
						"description": "How widely the change ripples. micro = one function in one file. package = one package's worth of files. cross = multiple unrelated components. project = repository-wide.",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "≤100 chars, your one-line restatement of what the user wants done. The planner uses this as a per-iteration title.",
					},
				},
			},
			"risk": map[string]any{
				"type":     "object",
				"required": []string{"affects_public_api", "changes_persistence", "changes_build_system", "overall"},
				"properties": map[string]any{
					"affects_public_api":   map[string]any{"type": "boolean"},
					"changes_persistence":  map[string]any{"type": "boolean"},
					"changes_build_system": map[string]any{"type": "boolean"},
					"overall": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "Roll-up risk band combining the booleans plus your judgement on blast radius.",
					},
				},
			},
			"scope_anchors": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Repo-relative paths or directory prefixes the change centres on. Up to ~10 entries.",
			},
			"constraints": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"kind"},
					"properties": map[string]any{
						"kind":   map[string]any{"type": "string", "description": "Short label like preserve_api / no_external_deps / match_existing_style. Use preserve_regression_test when the user explicitly says an existing regression test/input must be kept. Pick the closest fit; free string is fine."},
						"target": map[string]any{"type": "string", "description": "Path or symbol the constraint applies to. Use '*' when global."},
						"note":   map[string]any{"type": "string", "description": "Short quote of the user's wording, when applicable."},
					},
				},
				"description": "Must-keep / must-not-touch the user mentioned. Skip when the user did not mention any.",
			},
			"expected_outcomes": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "2-4 short concrete signals that the change is correctly done. The reflector reads these to judge whether retries are moving toward the goal.",
			},
			"behavior_contracts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Stable short id, e.g. c1 or raises-valueerror. Used by verification_probes.contract_refs.",
						},
						"kind": map[string]any{
							"type": "string",
							"enum": []string{"observable", "exception", "output_path", "stdout", "status_code", "file_layout", "command_result", "invariant"},
						},
						"subject": map[string]any{
							"type":        "string",
							"description": "Path, symbol, command, endpoint, or runtime surface this contract describes.",
						},
						"operator": map[string]any{
							"type": "string",
							"enum": []string{"satisfies", "equals", "not_equals", "contains", "not_contains", "exists", "not_exists", "raises", "not_raises", "returns"},
						},
						"polarity": map[string]any{
							"type":        "string",
							"enum":        []string{"expected", "forbidden", "observed"},
							"description": "expected means the fixed code must satisfy this behavior; forbidden means the fixed code must avoid this behavior; observed means this is pre-fix evidence or a current failure, not a completion target. Do not mark observed facts as required.",
						},
						"expected": map[string]any{
							"type":        "string",
							"description": "Exact observable value or concise behavior to preserve/satisfy.",
						},
						"comparator": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"subject": map[string]any{
									"type":        "string",
									"description": "Grounded working or contrasting reference surface for this contract, such as a nearby API call, command, or expression named by the request or light repo inspection.",
								},
								"operator": map[string]any{
									"type": "string",
									"enum": []string{"satisfies", "equals", "not_equals", "contains", "not_contains", "exists", "not_exists", "raises", "not_raises", "returns"},
								},
								"expected": map[string]any{
									"type":        "string",
									"description": "Observable value or behavior of the comparator surface.",
								},
								"relation": map[string]any{
									"type":        "string",
									"enum":        []string{"same_as", "consistent_with", "contrasts_with", "regression_baseline"},
									"description": "How the fixed subject should relate to the comparator. same_as means the fixed subject should match the comparator's observable behavior; contrasts_with means the comparator is intentionally different evidence.",
								},
								"evidence_ref": map[string]any{
									"type":        "string",
									"description": "Optional source such as issue text, runtime log, or file:line evidence supporting the comparator.",
								},
							},
							"description": "Optional typed baseline/comparator when the issue or light repo inspection gives both a failing subject and a known working/contrasting reference. Do not infer from keywords; use only grounded evidence.",
						},
						"evidence_ref": map[string]any{
							"type":        "string",
							"description": "Optional source such as issue text, log ref, or file:line evidence.",
						},
						"required": map[string]any{
							"type":        "boolean",
							"description": "True when this observable is required for task completion.",
						},
					},
					"required": []string{"id", "kind", "expected"},
				},
				"description": "Optional typed observables the plan/probes should satisfy or preserve. Prefer these when the request names exception type, output path/layout, status code, command result, stdout text, or a concrete invariant. Separate observed pre-fix failures from expected fixed behavior with polarity=observed vs polarity=expected/forbidden. For bugs that should stop raising, use operator=not_raises with polarity=expected; use operator=raises only when the fixed behavior is supposed to raise. When evidence includes a known working or contrasting reference surface, attach it as comparator so probes can verify the relationship instead of only proving no-crash. Do not infer from keywords; emit only facts grounded in the request or light repo inspection.",
			},
			"phase_proposal": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"split": map[string]any{
						"type": "string",
						"enum": []string{"single", "sequential", "parallel"},
					},
					"phases": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":     "object",
							"required": []string{"goal"},
							"properties": map[string]any{
								"goal":               map[string]any{"type": "string"},
								"rough_target_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							},
						},
					},
				},
				"description": "If the change splits naturally into stages each independently verifiable before the next, propose the stages. Otherwise omit or set split=single.",
			},
			"applicable_pitfalls": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "If known pitfalls (provided in the prompt) clearly apply, list their IDs here. Empty list when none apply or no pitfall list was provided.",
			},
		},
	}
}
