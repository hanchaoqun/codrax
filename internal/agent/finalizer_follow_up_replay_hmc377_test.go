package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377SealedFollowUp is one sealed batch-376 finalizer dispatch whose
// accepted first answer was followed by an optional follow-up that the loop
// refused locally. Initial message texts, the call arguments and the tool
// result are the logged texts; the free text of the answer (logged truncated),
// the hint, the two call ids and the tool-surface directive are padding of
// their logged sizes.
type hmc377SealedFollowUp struct {
	initial                       []llm.Message
	freeText, hint                int
	tool, params, result          string
	lanes                         []postEmitAdvisoryLaneKind
	firstMessages, firstTools     int
	followMessages, followTools   int
	pressure                      string
	refusedHard, refusedAllocated bool
	dividend                      int
	roster                        []string
}

func hmc377ParseSealedFollowUp(t *testing.T, path string) hmc377SealedFollowUp {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var (
		receipt  = regexp.MustCompile(`\[diag finalizer\] phase=prompt_byte_budget receipt=(\{.*\})\n`)
		initial  = regexp.MustCompile(`\[diag finalizer\] INIT msg role=(\w+) len=(\d+)\n`)
		pressure = regexp.MustCompile(`\[agent finalizer\] context-pressure (\w+): iter=\d+ (?:prompt=\d+/\d+ bytes )?messages=(\d+) tools=(\d+)`)
		answer   = regexp.MustCompile(`\[diag finalizer\] iter=0 ASSISTANT content_len=(\d+) tool_calls=1`)
		call     = regexp.MustCompile(`\[diag finalizer\] iter=0 phase=toolcall call\[0\] tool=(\w+) params=(.*)\n`)
		result   = regexp.MustCompile(`\[diag finalizer\] iter=0 phase=toolresult TOOLRESULT \w+ ok=true len=(\d+):\n`)
		lanes    = regexp.MustCompile(`\[diag finalizer\] post_emit_advisory lanes=\[([^\]]*)\]`)
		hint     = regexp.MustCompile(`\[diag finalizer\] iter=0 phase=midloop_inject MIDLOOP inject len=(\d+):\n`)
		roster   = regexp.MustCompile(`"candidate_id": "(candidate-[0-9a-f]+)"`)
	)
	number := func(b []byte) int { n, _ := strconv.Atoi(string(b)); return n }
	next := func(re *regexp.Regexp, from int) ([]int, [][]byte) {
		loc := re.FindSubmatchIndex(raw[from:])
		if loc == nil {
			t.Fatalf("%s: missing %s", filepath.Base(path), re)
		}
		groups := make([][]byte, len(loc)/2)
		for i := range groups {
			if loc[2*i] >= 0 {
				groups[i] = raw[from+loc[2*i] : from+loc[2*i+1]]
			}
			loc[2*i], loc[2*i+1] = loc[2*i]+from, loc[2*i+1]+from
		}
		return loc, groups
	}
	var s hmc377SealedFollowUp
	loc, groups := next(receipt, 0)
	var budget finalizerPromptBudget
	if err := json.Unmarshal(groups[1], &budget); err != nil {
		t.Fatal(err)
	}
	s.dividend = budget.RepairTeachingDividendBytes
	pos := loc[1]
	rosterStart := pos
	for range 3 {
		loc, groups = next(initial, pos)
		n := number(groups[2])
		body := raw[loc[1] : loc[1]+n]
		if !bytes.HasPrefix(raw[loc[1]+n:], []byte("\n---\n")) {
			t.Fatalf("INIT body of %d bytes is not delimited", n)
		}
		s.initial = append(s.initial, llm.Message{Role: string(groups[1]), Content: string(body)})
		pos = loc[1] + n
	}
	seen := map[string]bool{}
	for _, m := range roster.FindAllSubmatch(raw[rosterStart:pos], -1) {
		if id := string(m[1]); !seen[id] {
			seen[id] = true
			s.roster = append(s.roster, id)
		}
	}
	loc, groups = next(pressure, pos)
	s.firstMessages, s.firstTools = number(groups[2]), number(groups[3])
	loc, groups = next(answer, loc[1])
	s.freeText = number(groups[1])
	loc, groups = next(call, loc[1])
	s.tool, s.params = string(groups[1]), string(groups[2])
	loc, groups = next(result, loc[1])
	n := number(groups[1])
	s.result = string(raw[loc[1] : loc[1]+n])
	loc, groups = next(lanes, loc[1]+n)
	for _, kind := range strings.Fields(string(groups[1])) {
		s.lanes = append(s.lanes, postEmitAdvisoryLaneKind(kind))
	}
	loc, groups = next(hint, loc[1])
	s.hint = number(groups[1])
	loc, groups = next(pressure, loc[1])
	s.pressure, s.followMessages, s.followTools = string(groups[1]), number(groups[2]), number(groups[3])
	rest := raw[loc[1]:]
	s.refusedHard = bytes.Contains(rest[:min(len(rest), 4096)], []byte("skipping optional post-emit repair; preserving accepted answer"))
	s.refusedAllocated = bytes.Contains(rest[:min(len(rest), 4096)], []byte("finalizer byte allocation exhausted during optional repair; preserving accepted answer"))
	return s
}

// Replays the two refused follow-ups of batch 376 (A: Honeybird, refused at
// the hard pressure ratio; B: NetEase reboot, refused by the allocation guard)
// through the real FilterToolSchemas and repair projection.
//
//	CODRAX_HMC377_SEALED_FOLLOW_UP_LOGS=<A run-1.logs.all.log>:<B run-1.logs.all.log> go test ./internal/agent/ -run TestHMC377SealedFollowUpReplay -v
func TestHMC377SealedFollowUpReplay(t *testing.T) {
	list := os.Getenv("CODRAX_HMC377_SEALED_FOLLOW_UP_LOGS")
	if list == "" {
		t.Skip("requires the two sealed batch-376 run logs")
	}
	const allocation, window, hard = 700000, 800000, 0.9
	sealed := map[int]struct{ refused, lanes int }{642266: {707552, 1}, 675652: {781288, 1}}
	pad := func(n int) string { return strings.Repeat("x", n) }
	// The follow-up teaching is rendered for the operations the request
	// publishes (HMC377 F5); its size relative to the repair teaching is
	// measured on a built-in dispatch for that operation set.
	repair := func() *finalizerRepairTeaching {
		live := &hmc377Live{skill: traceTeachingSkill(t, "answer-document-skill")}
		live.base = NewFinalizerAgent(&Dependencies{LLM: &hmc376Adapter{window: 1_000_000}, Tools: tool.NewRegistry(), MaxIterations: 1,
			AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
		_, repair := live.base.buildInitialMessagesAndRepairTeaching(hmc377Context(false)(t), live.skill)
		return repair
	}()
	delta := func(operations []string) int {
		followUp := repair.followUpSystem(operations)
		if followUp == "" {
			t.Fatalf("the follow-up teaching for %v is longer than the repair teaching", operations)
		}
		return len(followUp) - len(repair.patchSystem)
	}
	seen := 0
	for _, path := range filepath.SplitList(list) {
		s := hmc377ParseSealedFollowUp(t, path)
		first := s.firstMessages + s.firstTools
		want, ok := sealed[first]
		if !ok {
			t.Fatalf("%s: unexpected first request of %d bytes", filepath.Base(path), first)
		}
		seen++
		t.Run(strconv.Itoa(first), func(t *testing.T) {
			if s.followMessages+s.followTools != want.refused || len(s.lanes) != want.lanes || !(s.refusedHard || s.refusedAllocated) {
				t.Fatalf("not the sealed refusal: %+v", s)
			}
			mutable := types.NewMutableState("sealed sizes and texts only")
			candidates := make([]types.TraceFindingCandidateV1, 0, len(s.roster))
			for _, id := range s.roster {
				candidates = append(candidates, types.TraceFindingCandidateV1{PrimaryEligible: true, Decision: types.TraceCauseDecision{CandidateID: id, SubjectName: "sealed",
					Token:           types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
					Magnitude:       &types.TypedMagnitude{Value: 1, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
					CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E"}}})
			}
			mutable.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true, CandidateSetID: "sealed", Candidates: candidates})
			accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "sealed accepted answer"}}}
			mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
			before, _ := json.Marshal(mutable.AnswerDocumentV2())
			ctx := &types.AgentContext{Stage: types.StageFinalize, AgentName: types.AgentFinalizer, Mutable: mutable}
			e := &answerDocumentEvaluator{requestBytePolicy: types.FinalizerRequestByteEnforce, promptBudget: &finalizerPromptBudget{RequestAllocationBytes: allocation},
				mu: mutable, postEmitAdvisoryDelivered: true, postEmitAdvisoryKinds: s.lanes}

			// The logged follow-up request, unit by unit.
			id := pad(29)
			request := append([]llm.Message(nil), s.initial...)
			request = append(request,
				llm.Message{Role: "assistant", Content: pad(s.freeText), ToolCalls: []llm.ToolCall{{ID: id, Name: s.tool, Params: json.RawMessage(s.params)}}},
				llm.Message{Role: "tool", ToolCallID: id, Content: s.result},
				llm.Message{Role: "user", Content: pad(s.hint)})
			tail := s.followMessages - llm.EstimateMessagesBytes(request)
			if tail != 1085+62 {
				t.Fatalf("the logged units do not rebuild the logged follow-up messages: %d request-only bytes", tail)
			}
			request = append(request, llm.Message{Role: "system", Content: pad(1085 - len("system"))}, llm.Message{Role: "user", Content: pad(62 - len("user"))})
			if got := llm.EstimateMessagesBytes(request); got != s.followMessages {
				t.Fatalf("rebuilt follow-up messages %d B, sealed %d B", got, s.followMessages)
			}

			// The follow-up catalog: the dispatch patch schema of this roster,
			// its description padded so that the compacted whole surface has
			// the logged size; then what the real filter publishes for the
			// delivered rows, compacted as the loop does after filtering.
			patch := llm.ToolSchema{Name: answerDocumentPatchToolName, Parameters: (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)}
			if !strings.Contains(string(patch.Parameters), s.roster[0]) || len(s.roster) < 2 {
				t.Fatalf("fixture: the dispatch schema does not offer the sealed roster %v", s.roster)
			}
			patch.Description = pad(s.followTools - llm.EstimateToolSchemasBytes(llm.CompactToolSchemas([]llm.ToolSchema{patch})))
			if len(patch.Description) <= 0 || llm.EstimateToolSchemasBytes(llm.CompactToolSchemas([]llm.ToolSchema{patch})) != s.followTools {
				t.Fatal("fixture: the sealed catalog size was not rebuilt")
			}
			tools := llm.CompactToolSchemas(e.FilterToolSchemas(ctx, []llm.ToolSchema{{Name: answerDocumentToolName, Parameters: json.RawMessage(`{}`)}, patch}))
			if len(tools) != 1 || tools[0].Name != answerDocumentPatchToolName {
				t.Fatalf("follow-up catalog: %+v", tools)
			}
			if narrow := postEmitAdvisoryPatchOperations(s.lanes) != nil; narrow != (llm.EstimateToolSchemasBytes(tools) < s.followTools) {
				t.Fatalf("the catalog projection does not follow the delivered rows: %d B", llm.EstimateToolSchemasBytes(tools))
			}
			teaching := &finalizerRepairTeaching{draftSystem: s.initial[0].Content, patchSystem: pad(len(s.initial[0].Content) - s.dividend)}
			teaching.followUpSystem = func(operations []string) string { return pad(len(teaching.patchSystem) + delta(operations)) }
			projected := teaching.project(ctx, e, request, tools)
			got := llm.EstimateRequestBytes(projected, tools)
			pressure := float64(got) / window
			if got > allocation || pressure >= hard || e.finalizerRequestByteError(ctx, projected, tools) != nil {
				t.Fatalf("planned follow-up %d B (pressure %.3f) is still refused", got, pressure)
			}
			after, _ := json.Marshal(mutable.AnswerDocumentV2())
			if string(after) != string(before) {
				t.Fatal("planning the follow-up altered the accepted answer")
			}
			for i := range s.initial {
				if i > 0 && projected[i].Content != s.initial[i].Content {
					t.Fatal("a required evidence message changed")
				}
			}
			teachingSwapped := projected[0].Content != s.initial[0].Content
			t.Logf("%s lanes=%v: sealed %d B (%s) -> planned %d B, tools %d -> %d B, teaching replaced=%t, margin %d B, pressure %.1f%%",
				filepath.Base(filepath.Dir(path)), s.lanes, want.refused, s.pressure, got, s.followTools, llm.EstimateToolSchemasBytes(tools), teachingSwapped, allocation-got, pressure*100)
		})
	}
	if seen != len(sealed) {
		t.Fatalf("replayed %d of %d sealed follow-ups", seen, len(sealed))
	}
}
