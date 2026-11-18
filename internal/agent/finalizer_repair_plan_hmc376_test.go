package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The repair frame is planned inside the allocation, only under the enforce
// policy. It restricts optional previews and expanded equivalent forms; it
// never restricts a required unit or the dispatch guard.
func TestHMC376PlannedRepairFrameReceipt(t *testing.T) {
	const enforce, advisory = types.FinalizerRequestByteEnforce, types.FinalizerRequestByteLegacyAdvisory
	teaching := func(draft, patch int) *finalizerRepairTeaching {
		return &finalizerRepairTeaching{draftSystem: strings.Repeat("d", draft), patchSystem: strings.Repeat("p", patch)}
	}
	for _, c := range []struct {
		name                        string
		policy                      types.FinalizerRequestBytePolicy
		teaching                    *finalizerRepairTeaching
		envelope, dividend, reserve int
	}{
		{"dividend covers the envelope", enforce, teaching(118486, 10156), 100000, 108330, 0},
		{"dividend below the envelope", enforce, teaching(60000, 10000), 100000, 50000, 50000},
		{"teaching not replaceable", enforce, &finalizerRepairTeaching{draftSystem: strings.Repeat("d", 118486)}, 100000, 0, 100000},
		{"no teaching", enforce, nil, 100000, 0, 100000},
		{"advisory policy has no refusal to plan for", advisory, teaching(60000, 10000), 0, 0, 0},
	} {
		p := &finalizerPromptBudget{Policy: c.policy, WindowDerivedBytes: 800000}
		p.planRepairFrame(c.teaching)
		if p.RepairFrameEnvelopeBytes != c.envelope || p.RepairTeachingDividendBytes != c.dividend || p.RepairFrameReserveBytes != c.reserve || p.RepairFrameShortfallBytes != 0 {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
	var unknownWindow *finalizerPromptBudget
	unknownWindow.planRepairFrame(teaching(2, 1)) // an unknown window has no budget and must not panic

	p := &finalizerPromptBudget{Policy: enforce, WindowDerivedBytes: 800000, RequestAllocationBytes: 700000, OptionalTargetBytes: 650000,
		StaticBytes: 100000, SchemaBytes: 100000, TailBytes: 62}
	fixed := p.StaticBytes + p.SchemaBytes + p.TailBytes + len("user")
	if !p.optionalFits(650000-fixed) || p.optionalFits(650000-fixed+1) {
		t.Fatal("without a reserve the optional target alone decides, as before")
	}
	p.planRepairFrame(teaching(60000, 10000))
	if p.RepairFrameReserveBytes != 50000 || !p.optionalFits(650000-fixed) || p.optionalFits(650000-fixed+1) {
		t.Fatal("a reserve that already fits under the optional target must not change it")
	}
	p.planRepairFrame(nil)
	if p.RepairFrameReserveBytes != 100000 || !p.optionalFits(600000-fixed) || p.optionalFits(600000-fixed+1) {
		t.Fatal("optional previews must leave the planned repair reserve free")
	}
	if !p.requiredFits(700000-fixed) || p.requiredFits(700000-fixed+1) {
		t.Fatal("required units are held to the allocation only, never to the reserve")
	}
	mutable := types.NewMutableState("question")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mutable}
	e := &answerDocumentEvaluator{requestBytePolicy: enforce, promptBudget: p, mu: mutable}
	atAllocation := []llm.Message{{Role: "user", Content: strings.Repeat("r", 700000-len("user"))}}
	var refused *AnswerContextByteBudgetError
	if e.finalizerRequestByteError(ctx, atAllocation, nil) != nil {
		t.Fatal("the dispatch guard charged the repair reserve to a request")
	}
	atAllocation[0].Content += "r"
	if err := e.finalizerRequestByteError(ctx, atAllocation, nil); !errors.As(err, &refused) || refused.ActualBytes != 700001 || refused.AllocatedBytes != 700000 {
		t.Fatalf("the allocation itself changed: %v", err)
	}

	// Equivalent forms are lossless, so they yield to the reserve; a required
	// section is published whole and only recorded as a shortfall.
	const required, expanded, compact = "required unit that is never dropped", "expanded equivalent form of one repeated section", "compact form"
	publish := func(allocation func(full int) int, reserve int) (string, *finalizerPromptBudget) {
		var b strings.Builder
		trace := newAnswerDocDynamicTrace(nil)
		trace.promptBudget = &finalizerPromptBudget{Policy: enforce, DynamicAllocatedBytes: 1 << 30}
		trace.appendSection(&b, "required", func() string { return required })
		trace.appendEquivalentSection(&b, "equivalent", func(bool) finalizerTextProjection {
			return finalizerTextProjection{expanded: expanded, compact: compact}
		})
		trace.promptBudget.DynamicAllocatedBytes, trace.promptBudget.RepairFrameReserveBytes = allocation(b.Len()), reserve
		return trace.finishBudgeted(b.String()), trace.promptBudget
	}
	exact := func(full int) int { return full }
	full, plan := publish(exact, 0)
	if !strings.Contains(full, expanded) || plan.EquivalentGroups != 0 || plan.RepairFrameShortfallBytes != 0 {
		t.Fatalf("fixture: an exact fit without a reserve publishes the expanded form: %+v", plan)
	}
	saved := len(expanded) - len(compact)
	planned, plan := publish(exact, saved)
	if strings.Contains(planned, expanded) || !strings.Contains(planned, compact) || !strings.Contains(planned, required) ||
		plan.EquivalentGroups != 1 || plan.DynamicPublishedBytes != len(full)-saved || plan.RepairFrameShortfallBytes != 0 {
		t.Fatalf("the equivalent form did not yield to the planned reserve: %+v", plan)
	}
	inside, plan := publish(exact, len(full))
	if inside != planned || plan.RepairFrameShortfallBytes != len(planned) {
		t.Fatalf("required units inside the reserve must be published and recorded: %q %+v", inside, plan)
	}
	if over, plan := publish(func(full int) int { return full - saved - 1 }, 7); over != planned || plan.RepairFrameShortfallBytes != 7 {
		t.Fatalf("the shortfall is bounded by the reserve; an over-allocation is the guard's finding: %+v", plan)
	}

	// Public entry: the dividend is the exact byte difference between the
	// first-draft teaching and the lane-matched teaching that replaces it.
	stop := errors.New("second request reached the adapter")
	script := func(n int, state *types.MutableState) (llm.Response, error) {
		if n != 1 {
			return llm.Response{}, stop
		}
		state.SetLastRejectedAnswerDocumentV2(hmc376Draft())
		return llm.Response{Content: hmc376Notes("ONLY", 1700), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
	}
	wide := hmc376Run{window: 1_000_000, iterations: 2, script: script}.execute(t)
	window := hmc376Window(wide.adapter.size(0) + 7000)
	tight := hmc376Run{window: window, iterations: 2, script: script}.execute(t)
	if !errors.Is(wide.err, stop) || !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 2 {
		t.Fatalf("fixture: %v / %v", wide.err, tight.err)
	}
	dividend := len(tight.adapter.messages[0][0].Content) - len(tight.adapter.messages[1][0].Content)
	if dividend <= 0 || len(wide.adapter.messages[1][0].Content) != len(wide.adapter.messages[0][0].Content) {
		t.Fatalf("fixture must replace the teaching only under pressure: dividend=%d", dividend)
	}
	for name, got := range map[string]struct {
		budget *finalizerPromptBudget
		window int
	}{"roomy": {wide.budget, 1_000_000}, "tight": {tight.budget, window}} {
		b := got.budget
		if b == nil || b.RepairFrameEnvelopeBytes != got.window*types.BytesPerToken/8 || b.RepairFrameEnvelopeBytes != b.SafetyReserveBytes ||
			b.RepairTeachingDividendBytes != dividend || b.RepairFrameReserveBytes != max(0, b.RepairFrameEnvelopeBytes-dividend) {
			t.Errorf("%s plan: %+v (dividend %d)", name, b, dividend)
		}
	}
	receipt, _ := json.Marshal(tight.budget)
	for _, key := range []string{`"repair_frame_envelope_bytes":`, `"repair_teaching_dividend_bytes":`, `"repair_frame_reserve_bytes":`, `"repair_frame_shortfall_bytes":`,
		`"safety_reserve_bytes":`, `"request_allocation_bytes":`, `"dynamic_published_bytes":`} {
		if !strings.Contains(string(receipt), key) {
			t.Errorf("receipt lost or lacks %s", key)
		}
	}
	legacy := hmc376Run{window: window, iterations: 2, policy: advisory, script: script}.execute(t)
	if b := legacy.budget; b == nil || b.RepairFrameEnvelopeBytes != 0 || b.RepairTeachingDividendBytes != 0 || b.RepairFrameReserveBytes != 0 || b.RepairFrameShortfallBytes != 0 {
		t.Fatalf("the advisory policy planned a repair frame: %+v", b)
	}
}

type hmc376SealedRound struct {
	tool                               string
	freeText, params, result, hint     int
	requestMessages, requestTools      int
	requested, answered, replacedBytes int
}

type hmc376SealedDispatch struct {
	initial []int
	rounds  []*hmc376SealedRound
	refused int
}

func hmc376ParseSealedLog(t *testing.T, path string) []*hmc376SealedDispatch {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var (
		receipt  = []byte("[diag finalizer] phase=prompt_byte_budget receipt=")
		initial  = regexp.MustCompile(`\[diag finalizer\] INIT msg role=\w+ len=(\d+)`)
		pressure = regexp.MustCompile(`\[agent finalizer\] context-pressure \w+: iter=(\d+) prompt=\d+/\d+ bytes messages=(\d+) tools=(\d+)`)
		answer   = regexp.MustCompile(`\[diag finalizer\] iter=(\d+) ASSISTANT content_len=(\d+)`)
		call     = regexp.MustCompile(`\[diag finalizer\] iter=(\d+) phase=toolcall call\[0\] tool=(\w+) params=(.*)$`)
		result   = regexp.MustCompile(`\[diag finalizer\] iter=(\d+) phase=toolresult TOOLRESULT \w+ ok=\w+ len=(\d+)`)
		hint     = regexp.MustCompile(`\[diag finalizer\] iter=(\d+) phase=midloop_inject MIDLOOP inject len=(\d+)`)
		swapped  = regexp.MustCompile(`phase=repair_prompt_budget draft_request_bytes=\d+ repair_request_bytes=\d+ allocation_bytes=\d+ replaced_system_bytes=(\d+)`)
		refusal  = regexp.MustCompile(`(?:DAG finalize failed|forced finalize failed)[^:]*: .*messages and tool schemas=(\d+) bytes`)
	)
	number := func(b []byte) int { n, _ := strconv.Atoi(string(b)); return n }
	var dispatches []*hmc376SealedDispatch
	var current *hmc376SealedDispatch
	pendingSwap := 0
	round := func(index []byte) *hmc376SealedRound {
		for len(current.rounds) <= number(index) {
			current.rounds = append(current.rounds, &hmc376SealedRound{})
		}
		return current.rounds[number(index)]
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if bytes.Contains(line, receipt) {
			current = &hmc376SealedDispatch{}
			dispatches = append(dispatches, current)
			continue
		}
		if current == nil {
			continue
		}
		if m := initial.FindSubmatch(line); m != nil {
			current.initial = append(current.initial, number(m[1]))
		} else if m := swapped.FindSubmatch(line); m != nil {
			pendingSwap = number(m[1])
		} else if m := pressure.FindSubmatch(line); m != nil {
			r := round(m[1])
			r.requestMessages, r.requestTools, r.requested, r.replacedBytes = number(m[2]), number(m[3]), 1, pendingSwap
			pendingSwap = 0
		} else if m := answer.FindSubmatch(line); m != nil {
			r := round(m[1])
			r.freeText, r.answered = number(m[2]), 1
		} else if m := call.FindSubmatch(line); m != nil {
			r := round(m[1])
			r.tool, r.params = string(m[2]), len(m[3])
		} else if m := result.FindSubmatch(line); m != nil {
			round(m[1]).result = number(m[2])
		} else if m := hint.FindSubmatch(line); m != nil {
			round(m[1]).hint = number(m[2])
		} else if m := refusal.FindSubmatch(line); m != nil && current.refused == 0 {
			current.refused = number(m[1])
		}
	}
	return dispatches
}

// Replays the refused requests of four sealed runs from their logged byte
// sizes. Nothing but sizes and tool names is read from the logs; the messages
// are padding of the logged lengths, sent through the real projection.
//
//	CODRAX_HMC376_SEALED_LOGS=<log>:<log>:<log>:<log> go test ./internal/agent/ -run TestHMC376SealedRepairNumbersReplay -v
func TestHMC376SealedRepairNumbersReplay(t *testing.T) {
	list := os.Getenv("CODRAX_HMC376_SEALED_LOGS")
	if list == "" {
		t.Skip("requires the sealed finalizer logs of the four refused runs")
	}
	const allocation, hardDirectiveMax = 700000, 1024
	// Keyed by the size of the run's first sent request, as logged.
	sealed := map[int]struct {
		repairRefused, repairSent, forcedRefused, forcedSent int
	}{
		677804: {777916, 668639, 826610, 675386},
		596276: {704472, 567419, 0, 0},
		690429: {704359, 662999, 811875, 688626},
		684328: {714886, 680417, 825190, 682525},
	}
	pad := func(n int) string { return strings.Repeat("x", n) }
	seen := map[int]bool{}
	for _, path := range filepath.SplitList(list) {
		dispatches := hmc376ParseSealedLog(t, path)
		if len(dispatches) < 2 || len(dispatches[0].initial) != 3 || len(dispatches[0].rounds) < 2 {
			t.Fatalf("%s: not a sealed finalizer log with a repair and a forced dispatch", filepath.Base(path))
		}
		first, forced := dispatches[0], dispatches[1]
		firstRequest := first.rounds[0].requestMessages + first.rounds[0].requestTools
		want, known := sealed[firstRequest]
		if !known || seen[firstRequest] {
			t.Fatalf("%s: unexpected or repeated first request of %d bytes", filepath.Base(path), firstRequest)
		}
		seen[firstRequest] = true
		t.Run(strconv.Itoa(firstRequest), func(t *testing.T) {
			// Dividend: the logged size of the replaced teaching (the oldest of
			// these runs predates the replacement and was refused before it).
			dividend := 108330
			for _, r := range first.rounds {
				if r.replacedBytes != 0 && r.replacedBytes != dividend {
					t.Fatalf("logged teaching replacement %d", r.replacedBytes)
				}
			}
			last := len(first.rounds) - 1
			refusedRound := first.rounds[last]
			if refusedRound.requested != 1 || refusedRound.answered != 0 || first.refused != want.repairRefused {
				t.Fatalf("sealed repair refusal: %d (round %d)", first.refused, last)
			}
			mutable := types.NewMutableState("sealed sizes only")
			ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mutable}
			e := &answerDocumentEvaluator{requestBytePolicy: types.FinalizerRequestByteEnforce, promptBudget: &finalizerPromptBudget{RequestAllocationBytes: allocation}, mu: mutable, preferPatchNext: true}
			teaching := &finalizerRepairTeaching{draftSystem: pad(first.initial[0]), patchSystem: pad(first.initial[0] - dividend)}
			messages := []llm.Message{{Role: "system", Content: teaching.draftSystem}, {Role: "user", Content: pad(first.initial[1])}, {Role: "user", Content: pad(first.initial[2])}}
			superseded, latest := 0, 0
			for n, r := range first.rounds[:last] {
				if r.answered != 1 || !isAnswerDocumentTool(r.tool) {
					t.Fatalf("round %d is not one answer attempt: %+v", n, r)
				}
				id := pad(26) + strconv.Itoa(100+n) // 29 bytes, as issued by the sealed provider
				start, corrected := len(messages), -1
				messages = append(messages, llm.Message{Role: "assistant", Content: pad(r.freeText), ToolCalls: []llm.ToolCall{{ID: id, Name: r.tool, Params: json.RawMessage(pad(r.params))}}},
					llm.Message{Role: "tool", ToolCallID: id, Content: pad(r.result)})
				if r.hint > 0 {
					corrected = len(messages)
					messages = append(messages, llm.Message{Role: "user", Content: pad(r.hint)})
				}
				teaching.history.observe(mutable, messages, start, corrected, []types.ToolResult{{ToolName: r.tool, Repair: &types.ToolRepair{}}})
				superseded, latest = superseded+latest, r.freeText
			}
			// Whatever the logged request carried beyond these units is its
			// request-only tail (the tool-surface directive and the language nudge).
			// The logged message size is after any teaching replacement.
			teachingBytes := first.initial[0]
			if refusedRound.replacedBytes != 0 {
				teachingBytes -= dividend
			}
			tail := refusedRound.requestMessages - (llm.EstimateMessagesBytes(messages) - first.initial[0] + teachingBytes)
			if tail < 1100 || tail > 1200 {
				t.Fatalf("the logged sizes do not rebuild the logged request: unexplained %d bytes", tail)
			}
			request := append(append([]llm.Message(nil), messages...), llm.Message{Role: "system", Content: pad(tail - len("system"))})
			tools := []llm.ToolSchema{{Name: hmc376Patch, Parameters: json.RawMessage(pad(refusedRound.requestTools - len(hmc376Patch)))}}
			// The sealed refusal is the logged request: after the teaching
			// replacement where that build had it, plus the pressure directive
			// a request above the hard ratio received.
			swappedOnly := llm.EstimateRequestBytes(request, tools) - dividend
			logged := swappedOnly
			if refusedRound.replacedBytes == 0 {
				logged += dividend
			}
			if logged != refusedRound.requestMessages+refusedRound.requestTools || want.repairRefused < logged || want.repairRefused-logged > hardDirectiveMax {
				t.Fatalf("rebuilt request is %d bytes, sealed refusal %d", logged, want.repairRefused)
			}
			projected := teaching.project(ctx, e, request, tools)
			got := llm.EstimateRequestBytes(projected, tools)
			expected := swappedOnly
			if swappedOnly > allocation {
				expected -= superseded
			}
			if got != expected || got != want.repairSent || got > allocation || e.finalizerRequestByteError(ctx, projected, tools) != nil {
				t.Fatalf("repair request after the ladder: %d bytes (expected %d, sealed arithmetic %d)", got, expected, want.repairSent)
			}
			kept := 0
			for i, message := range projected {
				if message.Role != "assistant" {
					continue
				}
				if _, result, ok := hmc376CallMessage(projected, message.ToolCalls[0].ID); !ok || result.ToolCallID != message.ToolCalls[0].ID || len(message.ToolCalls[0].Params) != first.rounds[kept].params {
					t.Fatalf("round trip %d was not kept whole", kept)
				}
				if kept == last-1 && len(projected[i].Content) != latest {
					t.Fatal("the latest attempt lost its free text although the request already fit")
				}
				kept++
			}
			if len(projected) != len(request) || kept != last {
				t.Fatalf("round trips kept: %d of %d", kept, last)
			}
			t.Logf("repair: sealed refusal %d -> %d bytes (margin %d, %d attempts kept)", want.repairRefused, got, allocation-got, last)

			if want.forcedRefused == 0 {
				return
			}
			// The forced dispatch published both catalogs: the complete answer
			// catalog of the first request plus the patch catalog of the refused round.
			forcedRound := forced.rounds[0]
			fullCatalog, patchCatalog := first.rounds[0].requestTools, refusedRound.requestTools
			if forcedRound.requestTools != fullCatalog+patchCatalog || forced.refused != want.forcedRefused ||
				forced.refused-forcedRound.requestMessages-forcedRound.requestTools > hardDirectiveMax {
				t.Fatalf("sealed forced refusal: %+v refused=%d", forcedRound, forced.refused)
			}
			mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			dispatch := &answerDocumentEvaluator{mu: mutable}
			if !dispatch.answerPatchOperandHidden(ctx) {
				t.Fatal("a new dispatch with a retained draft must publish the complete answer catalog alone")
			}
			if single := forcedRound.requestMessages + fullCatalog; single != want.forcedSent || single > allocation || firstRequest > allocation {
				t.Fatalf("forced dispatch with one catalog: %d bytes", single)
			}
			t.Logf("forced: sealed refusal %d -> %d bytes with one catalog (margin %d)", want.forcedRefused, want.forcedSent, allocation-want.forcedSent)
		})
	}
	if len(seen) != len(sealed) {
		t.Fatalf("replayed %d of %d sealed runs", len(seen), len(sealed))
	}
}
