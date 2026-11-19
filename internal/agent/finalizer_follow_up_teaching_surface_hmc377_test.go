package agent

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 F5: when the follow-up publishes a projected patch surface and is
// over the allocation, the lane-matched static teaching that replaces the
// draft teaching is derived from the operations that request publishes. It
// never teaches an operation the projected schema leaves out; with the whole
// surface published it stays the dispatch-time representation.

// hmc377PatchOperationUniverse is every top-level patch operation either the
// static compatibility schema or this dispatch's projection can publish.
func hmc377PatchOperationUniverse(t *testing.T, ctx *types.AgentContext) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, raw := range []json.RawMessage{(&tool.EmitAnswerDocumentPatch{}).Parameters(), (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)} {
		for _, name := range hmc377Properties(t, llm.ToolSchema{Parameters: raw}) {
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// hmc377NamedOperations returns the operations of the universe a text names
// as a whole word, backticked or not.
func hmc377NamedOperations(text string, universe []string) map[string]bool {
	named := map[string]bool{}
	for _, operation := range universe {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(operation) + `\b`).MatchString(text) {
			named[operation] = true
		}
	}
	return named
}

// Through the real finalizer loop: the selection follow-up publishes only
// its two operations and, with the provider's reasoning channel pushing it
// just past the unchanged allocation, the teaching rung replaces the draft
// teaching. The replacement names exactly the published operations.
func TestHMC377ProjectedFollowUpTeachingNamesOnlyPublishedOperations(t *testing.T) {
	wide := hmc377Run{window: 1_000_000, script: hmc377Script("")}.execute(t)
	if len(wide.adapter.messages) != 2 || !errors.Is(wide.err, hmc377Stop) && wide.err != nil {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	draft := wide.adapter.messages[1][0].Content
	window := hmc376Window(max(wide.adapter.size(0), wide.adapter.size(1)) + 512)
	allocation := hmc376Allocation(window)
	// Over the allocation by 1 KiB with the draft teaching: far less than the
	// teaching dividend, so rung 1 alone decides the request.
	reasoning := strings.Repeat("r", allocation-wide.adapter.size(1)+1024)
	tight := hmc377Run{window: window, script: hmc377Script(reasoning)}.execute(t)
	if len(tight.adapter.messages) != 2 {
		t.Fatalf("the projected follow-up was not sent: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	if tight.adapter.size(1) > allocation {
		t.Fatalf("sent follow-up %d B exceeds the allocation %d B", tight.adapter.size(1), allocation)
	}
	system := tight.adapter.messages[1][0].Content
	if system == draft || tight.adapter.messages[0][0].Content != draft {
		t.Fatal("fixture: the teaching rung must replace only the follow-up's draft teaching")
	}
	published := hmc377Properties(t, tight.adapter.tools[1][0])
	if strings.Join(published, ",") != "replace_trace_root_causes,unchanged_block_ids" {
		t.Fatalf("fixture: the selection follow-up must publish its two operations, got %v", published)
	}
	universe := hmc377PatchOperationUniverse(t, tight.live.ctx)
	named := hmc377NamedOperations(system, universe)
	for _, operation := range universe {
		want := operation == "replace_trace_root_causes" || operation == "unchanged_block_ids"
		if named[operation] != want {
			t.Errorf("the follow-up teaching names %q=%t; the request publishes %v", operation, named[operation], published)
		}
	}
	if strings.Contains(system, types.AnswerDocumentPatchOperationTeaching) {
		t.Error("the projected follow-up teaching embeds the whole-surface operation teaching")
	}
	if tight.live.ctx.Mutable.TraceRootCauseReport() == nil || hmc377Snapshot(t, tight.accepted) != hmc377Snapshot(t, tight.live.ctx.Mutable.AnswerDocumentV2()) {
		t.Fatal("the follow-up lost the selection or altered the accepted answer")
	}
}

// Every combination of post-emit lane rows, in table order, through the real
// FilterToolSchemas and the real repair projection of a built-in dispatch:
// the teaching that replaces the draft teaching names every operation the
// projected request publishes and no other; a whole-surface request keeps the
// whole-surface representation.
func TestHMC377FollowUpTeachingMatchesThePublishedOperationsForEveryLaneCombination(t *testing.T) {
	sk := traceTeachingSkill(t, "answer-document-skill")
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: hmc377Answer},
		{ID: "detail", Kind: types.BlockSection, Title: "Detail", Text: "second model block"},
	}}
	projected := 0
	for mask := 1; mask < 1<<len(postEmitAdvisoryLanes); mask++ {
		var kinds []postEmitAdvisoryLaneKind
		for i, lane := range postEmitAdvisoryLanes {
			if mask&(1<<i) != 0 {
				kinds = append(kinds, lane.kind)
			}
		}
		ctx := hmc377Context(false)(t)
		base := NewFinalizerAgent(&Dependencies{LLM: &hmc376Adapter{window: 1_000_000}, Tools: registry, MaxIterations: 1,
			AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
		messages, repair := base.buildInitialMessagesAndRepairTeaching(ctx, sk)
		e := base.eval.(*answerDocumentEvaluator)
		if repair == nil || repair.patchSystem == "" || e.promptBudget == nil {
			t.Fatal("fixture: an enforced built-in dispatch must prepare its lane teachings")
		}
		ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
		e.postEmitAdvisoryDelivered, e.postEmitAdvisoryKinds = true, kinds
		dispatch := base.buildToolSchemas(sk, ctx)
		tools := llm.CompactToolSchemas(e.FilterToolSchemas(ctx, dispatch))
		if len(tools) != 1 || tools[0].Name != answerDocumentPatchToolName {
			t.Fatalf("%v: the follow-up must offer the patch tool only: %v", kinds, tools)
		}
		var wholeSchema llm.ToolSchema
		for _, schema := range dispatch {
			if schema.Name == answerDocumentPatchToolName {
				wholeSchema = schema
			}
		}
		published, whole := hmc377Properties(t, tools[0]), hmc377Properties(t, wholeSchema)
		narrow := strings.Join(published, ",") != strings.Join(whole, ",")
		if narrow != (postEmitAdvisoryPatchOperations(kinds) != nil) {
			t.Fatalf("%v: published %v, declared %v", kinds, published, postEmitAdvisoryPatchOperations(kinds))
		}
		e.promptBudget.RequestAllocationBytes = 1 // every rung runs; only the teaching is inspected
		out := repair.project(ctx, e, messages, tools)
		system := out[0].Content
		if system == messages[0].Content {
			t.Fatalf("%v: the follow-up teaching rung did not run", kinds)
		}
		for i := 1; i < len(messages); i++ {
			if out[i].Content != messages[i].Content {
				t.Fatalf("%v: the teaching rung changed message %d", kinds, i)
			}
		}
		if !narrow {
			if !strings.Contains(system, types.AnswerDocumentPatchOperationTeaching) {
				t.Errorf("%v: a whole-surface follow-up must keep the whole-surface operation teaching", kinds)
			}
			continue
		}
		projected++
		universe := hmc377PatchOperationUniverse(t, ctx)
		named := hmc377NamedOperations(system, universe)
		offered := map[string]bool{}
		for _, operation := range published {
			offered[operation] = true
		}
		for _, operation := range universe {
			if named[operation] != offered[operation] {
				t.Errorf("%v: the follow-up teaching names %q=%t; the request publishes %v", kinds, operation, named[operation], published)
			}
		}
		if strings.Contains(system, types.AnswerDocumentPatchOperationTeaching) {
			t.Errorf("%v: the projected follow-up teaching embeds the whole-surface operation teaching", kinds)
		}
	}
	if projected != 3 {
		t.Fatalf("expected the order, the selection and the order+selection combinations to be projected, got %d", projected)
	}
}
