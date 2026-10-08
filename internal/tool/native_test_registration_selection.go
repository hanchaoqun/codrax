package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A dispatch-local reference is input transport only. Durable plans retain
// the original exact pair, not a permission that could survive its grant.
type emitProjectTestObservation struct {
	types.ProjectTestObservation
	AssertionRef json.RawMessage `json:"assertion_ref,omitempty"`
}

func normalizeEmittedProjectTestObservations(ctx *types.BusContext, in []emitProjectTestObservation, changes []types.FileChange, registration bool) ([]types.ProjectTestObservation, string) {
	out := make([]types.ProjectTestObservation, 0, len(in))
	var choices []types.NativeTestIdentityChoice
	if registration && ctx != nil && ctx.Mutable != nil && ctx.Mode.IsWrite() && ctx.PipelineStage == types.StagePlan {
		choices = ctx.Mutable.NativeTestRegistrationIdentityChoices(repositoryReadPhysicalIdentity(ctx.RepoRoot))
	}
	for i, row := range in {
		observation := row.ProjectTestObservation
		if len(row.AssertionRef) != 0 {
			var ref string
			if json.Unmarshal(row.AssertionRef, &ref) != nil || ref == "" {
				return nil, fmt.Sprintf("project_test_observations[%d].assertion_ref must be a nonempty current identity reference", i)
			}
			var selected *types.NativeTestIdentityChoice
			for j := range choices {
				if choices[j].Ref == ref {
					if selected != nil {
						return nil, fmt.Sprintf("project_test_observations[%d].assertion_ref is ambiguous", i)
					}
					selected = &choices[j]
				}
			}
			if selected == nil {
				return nil, fmt.Sprintf("project_test_observations[%d].assertion_ref is not a current authorized identity choice; use an offered reference or supply an independently inspected exact assertion_suite/assertion_id pair", i)
			}
			if (observation.AssertionSuite != "" || observation.AssertionID != "") &&
				(observation.AssertionSuite != selected.AssertionSuite || observation.AssertionID != selected.AssertionID) {
				return nil, fmt.Sprintf("project_test_observations[%d].assertion_ref conflicts with the explicit identity pair; omit both strings or copy the selected pair exactly", i)
			}
			observation.AssertionSuite, observation.AssertionID = selected.AssertionSuite, selected.AssertionID
		}
		out = append(out, observation)
	}
	return normalizeProjectTestObservations(ctx, out, changes)
}

func (t *EmitChangePlan) ParametersFor(ctx *types.AgentContext) json.RawMessage {
	return nativeTestRegistrationSelectionSchema(t.Parameters(), ctx)
}

func (t *EmitPlanSkeleton) ParametersFor(ctx *types.AgentContext) json.RawMessage {
	return nativeTestRegistrationSelectionSchema(t.Parameters(), ctx)
}

func nativeTestRegistrationSelectionSchemaForBus(raw json.RawMessage, ctx *types.BusContext) json.RawMessage {
	if ctx == nil {
		return raw
	}
	return nativeTestRegistrationSelectionSchema(raw, &types.AgentContext{Mutable: ctx.Mutable, RepoRoot: ctx.RepoRoot, Mode: ctx.Mode, Stage: ctx.PipelineStage})
}

func nativeTestRegistrationSelectionSchema(raw json.RawMessage, ctx *types.AgentContext) json.RawMessage {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.Stage != types.StagePlan {
		return raw
	}
	choices := ctx.Mutable.NativeTestRegistrationIdentityChoices(repositoryReadPhysicalIdentity(ctx.RepoRoot))
	if len(choices) == 0 {
		return raw
	}
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return raw
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return raw
	}
	pto, ok := properties["project_test_observations"].(map[string]any)
	if !ok {
		return raw
	}
	items, ok := pto["items"].(map[string]any)
	if !ok {
		return raw
	}
	fields, ok := items["properties"].(map[string]any)
	if !ok {
		return raw
	}
	refs := make([]string, 0, len(choices))
	for _, choice := range choices {
		refs = append(refs, choice.Ref)
	}
	fields["assertion_ref"] = map[string]any{"type": "string", "enum": refs, "description": types.NativeTestRegistrationAssertionSelectionTeaching}
	items["required"] = []string{"id", "test_path", "contract_refs"}
	items["anyOf"] = []any{
		map[string]any{"required": []string{"assertion_ref"}},
		map[string]any{"required": []string{"assertion_suite", "assertion_id"}},
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return raw
	}
	return encoded
}
