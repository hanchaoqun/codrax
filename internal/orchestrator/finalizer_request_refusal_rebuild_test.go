package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A first answer request that was refused before sending is rebuilt from the
// same inputs by the forced dispatch and refused again. The run stops at the
// one refusal.
func TestRefusedFirstAnswerRequestIsNotRebuiltFromUnchangedInputs(t *testing.T) {
	run := runRefusalScenario(t, refusalRunOptions{})
	if run.finalizerCalls != 1 {
		t.Errorf("answer stage dispatched %d times for one refused state, want 1", run.finalizerCalls)
	}
	if got := run.bus.Mutable.Result(); got != "" {
		t.Errorf("no answer may be authored for a refused request: %q", got)
	}
	if run.bus.TaskState.LastError == "" {
		t.Error("a run without an answer must end with a failure notice")
	}
	if !run.bus.Mutable.FinalizerRequestRefusal().NothingSent() {
		t.Errorf("the refusal still stands at the end of the run: %+v", run.bus.Mutable.FinalizerRequestRefusal())
	}
	if strings.Contains(run.log, "would be rebuilt from different inputs") {
		t.Errorf("nothing changed between the refusal and the forced path:\n%s", lastLogLines(run.log, 12))
	}
}

// The forced path drains pending forced reads before it dispatches. A drained
// read changes what the answer request is built from, so the dispatch runs.
func TestRefusedFirstAnswerRequestIsRebuiltAfterADrainedForcedRead(t *testing.T) {
	run := runRefusalScenario(t, refusalRunOptions{
		repoFiles: lateReadRepo,
		finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
			if call == 1 {
				pendingLateRead(ctx)
			}
			return nil, refusedAnswerRequest()
		},
	})
	if !strings.Contains(run.log, "force-finalize: drained 1 pending forced read(s)") {
		t.Fatalf("fixture must drain one forced read before the forced dispatch:\n%s", lastLogLines(run.log, 20))
	}
	if run.finalizerCalls != 2 {
		t.Fatalf("a drained forced read changes the inputs; answer stage dispatched %d times, want 2", run.finalizerCalls)
	}
	if !strings.Contains(run.log, "would be rebuilt from different inputs (") || !strings.Contains(run.log, "bus.ToolResults") {
		t.Errorf("the log must name what changed since the refusal:\n%s", lastLogLines(run.log, 20))
	}
}

// The forced path can run the extract stage first. Its ingested output is new
// input for the answer request, so the forced dispatch runs.
func TestRefusedFirstAnswerRequestIsRebuiltAfterExtractIngestion(t *testing.T) {
	run := runRefusalScenario(t, refusalRunOptions{
		requireExtract: true,
		extractor: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
			if call == 1 {
				return nil, errors.New("extract stage could not run before the answer")
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				AnswerSymbols: []types.AnswerSymbol{{
					Name: "Source", File: "src.go", Line: 1, Rationale: "extracted on the forced path",
				}},
				AnswerSymbolCompleteness: types.CompletenessLowerBound,
			}, nil
		},
	})
	if run.extractorCalls != 2 {
		t.Fatalf("fixture must run the extract stage on the forced path; extractor calls = %d\n%s", run.extractorCalls, lastLogLines(run.log, 30))
	}
	if run.finalizerCalls != 2 {
		t.Fatalf("extract ingestion changes the inputs; answer stage dispatched %d times, want 2", run.finalizerCalls)
	}
	if !strings.Contains(run.log, "would be rebuilt from different inputs (") || !strings.Contains(run.log, "bus.AnswerSymbols") {
		t.Errorf("the log must name the ingested carrier as the change:\n%s", lastLogLines(run.log, 12))
	}
}

// heldMemoryService stands for the memory store's read service: an object
// whose own state the sample cannot walk.
type heldMemoryService struct{ recalled sync.Map }

func (*heldMemoryService) Search(string, types.MemorySearchOpts) []types.MemoryIndexEntry {
	return nil
}

// A run holds more than typed data on the state its answer request is built
// from: the multi-repository index and the memory service on the bus, the
// repository graph and its symbol index on the mutable state.
func repositoryGraph(repo string) *rmtypes.Graph {
	file := &rmtypes.FileInfo{RelPath: "src.go", Language: "go", Package: "src", Symbols: []rmtypes.Symbol{
		{Name: "Source", Kind: "function", File: "src.go", Line: 3, EndLine: 3, Exported: true},
	}}
	return &rmtypes.Graph{
		Root:       repo,
		Files:      []*rmtypes.FileInfo{file},
		FileIndex:  map[string]*rmtypes.FileInfo{"src.go": file},
		SymbolDefs: map[string][]*rmtypes.Symbol{"Source": {&file.Symbols[0]}},
		Metadata:   rmtypes.Metadata{FileCount: 1},
	}
}

func repositoryMultiGraph(t *testing.T, graph *rmtypes.Graph) *multigraph.MultiGraph {
	t.Helper()
	mg, err := multigraph.New(multigraph.Config{
		Topology: singleRepoWarmupTopology(graph.Root),
		Build:    func(string, string) (*rmtypes.Graph, error) { return graph, nil },
		Cap:      1,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	return mg
}

// The refused request is still recognised in a run that holds its repository
// index and services: they are compared by identity, not walked.
func TestRefusedFirstAnswerRequestIsNotRebuiltWhileTheRunHoldsItsRepositoryIndex(t *testing.T) {
	var graph *rmtypes.Graph
	run := runRefusalScenario(t, refusalRunOptions{
		configure: func(o *Orchestrator, repo string) {
			graph = repositoryGraph(repo)
			mg := repositoryMultiGraph(t, graph)
			o.SetMultiGraphProvider(func() any { return mg })
			o.SetMemoryReader(&heldMemoryService{})
		},
		explorer: func(ctx *types.AgentContext) {
			ctx.Mutable.SetSearchGraph(graph)
			ctx.Mutable.SetSymbolOracle(repomap.NewSymbolOracle(graph))
		},
	})
	if run.bus.MultiGraph == nil || run.bus.Memory == nil || run.bus.Mutable.SearchGraph() == nil || run.bus.Mutable.SymbolOracle() == nil {
		t.Fatal("fixture: the run must hold its repository index and services")
	}
	record := run.bus.Mutable.FinalizerRequestRefusal()
	if !record.NothingSent() || record.InputDigest == "" {
		t.Fatalf("a state that holds services is still sampled: %+v\n%s", record, lastLogLines(run.log, 12))
	}
	if run.finalizerCalls != 1 {
		t.Errorf("answer stage dispatched %d times for one refused state, want 1", run.finalizerCalls)
	}
}

// What an index keeps for itself is not an input; another index in its place
// is. The scheduler's decision follows both.
func TestRepositoryIndexIsComparedByIdentityAcrossARefusal(t *testing.T) {
	readLog := captureOperatorLog(t)
	graph := repositoryGraph(t.TempDir())
	bus := &types.BusContext{
		Mutable:    types.NewMutableState("question"),
		MultiGraph: repositoryMultiGraph(t, graph),
		Memory:     &heldMemoryService{},
	}
	bus.Mutable.SetSearchGraph(graph)
	bus.Mutable.SetSymbolOracle(repomap.NewSymbolOracle(graph))
	o := &Orchestrator{busCtx: bus, emit: func(render.Event) {}}
	o.recordFinalizeDispatchResult(refusedAnswerRequest())
	if record := finalizeRefusalOf(bus); !record.NothingSent() || record.InputDigest == "" {
		t.Fatalf("a state that holds services is still sampled: %+v", record)
	}

	// The first lookup makes the graph memoise a table inside itself.
	if found, _ := bus.Mutable.SymbolOracle().SymbolExistsFlat("Source"); !found {
		t.Fatal("fixture: the symbol index must answer through the graph")
	}
	if o.repeatedAnswerRequestRefusal() == nil {
		t.Fatal("a lookup through the same index must not make the refused request look new")
	}

	bus.MultiGraph = repositoryMultiGraph(t, graph)
	if err := o.repeatedAnswerRequestRefusal(); err != nil {
		t.Fatalf("another index in place of the first is a different input and must dispatch, got %v", err)
	}
	if log := readLog(); !strings.Contains(log, "would be rebuilt from different inputs (bus.MultiGraph)") {
		t.Errorf("the log must name the replaced index:\n%s", lastLogLines(log, 4))
	}
}

// A refused repair or follow-up request says nothing about a fresh first
// request, and neither does a dispatch that failed for another reason: the
// forced dispatch runs even though nothing changed in between.
func TestOnlyARefusalThatSentNothingReplacesTheForcedDispatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first error
	}{
		{"repair request refused", &agent.AnswerContextByteBudgetError{ActualBytes: 777916, AllocatedBytes: 700000, RequestsSent: 1}},
		{"dispatch failed otherwise", errors.New("provider closed connection after the answer prompt")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runRefusalScenario(t, refusalRunOptions{
				finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
					if call == 1 {
						return nil, tc.first
					}
					return nil, refusedAnswerRequest()
				},
			})
			if run.finalizerCalls != 2 {
				t.Fatalf("the forced dispatch must run; answer stage dispatched %d times, want 2", run.finalizerCalls)
			}
			if strings.Contains(run.log, "would be rebuilt from different inputs") {
				t.Errorf("no first-request refusal stood, so nothing was compared:\n%s", lastLogLines(run.log, 12))
			}
		})
	}
}

// When the inputs changed and the rebuilt request is answered, the run ends
// with that answer and no refusal stands.
func TestForcedDispatchAfterARefusalDeliversItsAnswer(t *testing.T) {
	run := runRefusalScenario(t, refusalRunOptions{
		repoFiles: lateReadRepo,
		finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
			if call == 1 {
				pendingLateRead(ctx)
				return nil, refusedAnswerRequest()
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Source 在 src.go:1 定义。"}, nil
		},
	})
	if run.finalizerCalls != 2 {
		t.Fatalf("answer stage dispatched %d times, want 2", run.finalizerCalls)
	}
	if got := run.bus.Mutable.Result(); !strings.Contains(got, "Source") {
		t.Fatalf("the forced dispatch's answer must be delivered: %q", got)
	}
	if run.bus.TaskState.LastError != "" || run.bus.Mutable.FinalizerRequestRefusal() != nil {
		t.Errorf("an answered run carries no refusal: error=%q record=%+v", run.bus.TaskState.LastError, run.bus.Mutable.FinalizerRequestRefusal())
	}
}

// The decision itself, on the scheduler's helper: only a refusal that sent
// nothing, over inputs that were sampled and are unchanged, replaces the
// forced dispatch. The standing refusal is then reported as the typed error.
func TestRepeatedAnswerRequestRefusalRequiresReadableUnchangedInputs(t *testing.T) {
	newRun := func(bus *types.BusContext) *Orchestrator {
		return &Orchestrator{busCtx: bus, emit: func(render.Event) {}}
	}
	wrapped := fmt.Errorf("agent finalizer execution: %w", refusedAnswerRequest())

	o := newRun(&types.BusContext{Mutable: types.NewMutableState("question")})
	if o.answerRequestRefusedUnsent() || o.repeatedAnswerRequestRefusal() != nil {
		t.Fatal("a run without a refusal has nothing to repeat")
	}
	o.recordFinalizeDispatchResult(wrapped)
	if !o.answerRequestRefusedUnsent() {
		t.Fatal("the wrapped typed error must be recorded as a refusal with nothing sent")
	}
	var standing *agent.AnswerContextByteBudgetError
	if err := o.repeatedAnswerRequestRefusal(); !errors.As(err, &standing) || standing.ActualBytes != 742912 || standing.AllocatedBytes != 700000 || standing.RequestsSent != 0 {
		t.Fatalf("unchanged inputs: the standing refusal is reported as the typed error, got %v", err)
	}
	o.busCtx.EvidenceItems = append(o.busCtx.EvidenceItems, types.EvidenceItem{ID: "late"})
	if err := o.repeatedAnswerRequestRefusal(); err != nil {
		t.Fatalf("changed inputs must dispatch, got %v", err)
	}

	// A refused repair request is not a standing first-request refusal, even
	// over unchanged inputs.
	repair := newRun(&types.BusContext{Mutable: types.NewMutableState("question")})
	repair.recordFinalizeDispatchResult(&agent.AnswerContextByteBudgetError{ActualBytes: 777916, AllocatedBytes: 700000, RequestsSent: 1})
	if repair.answerRequestRefusedUnsent() || repair.repeatedAnswerRequestRefusal() != nil {
		t.Fatal("a refused repair request must not replace a fresh forced dispatch")
	}

	// A state the sample cannot read is treated as changed, at either end.
	opaque := newRun(unsampleableBus())
	opaque.recordFinalizeDispatchResult(wrapped)
	if !opaque.answerRequestRefusedUnsent() || opaque.repeatedAnswerRequestRefusal() != nil {
		t.Fatal("an unsampled state must never suppress the forced dispatch")
	}
	late := newRun(&types.BusContext{Mutable: types.NewMutableState("question")})
	late.recordFinalizeDispatchResult(wrapped)
	makeUnsampleable(late.busCtx)
	if late.repeatedAnswerRequestRefusal() != nil {
		t.Fatal("a state that became unreadable must not suppress the forced dispatch")
	}

	// The dispatch stamp is an input too.
	stamped := newRun(&types.BusContext{Mutable: types.NewMutableState("question")})
	stamped.recordFinalizeDispatchResult(wrapped)
	stamped.emitStageRetryAttempt = 1
	if stamped.repeatedAnswerRequestRefusal() != nil {
		t.Fatal("a different dispatch stamp is a different request")
	}

	// A dispatch that was not refused clears the record.
	o.recordFinalizeDispatchResult(nil)
	if o.answerRequestRefusedUnsent() || o.busCtx.Mutable.FinalizerRequestRefusal() != nil {
		t.Fatal("the record describes the last answer dispatch only")
	}
}

func TestChangedAnswerInputsAreDescribedForTheOperator(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("question")}
	readable := types.SampleAnswerRequestInputs(bus, types.AnswerDispatchStamp{})
	if got := describeChangedAnswerInputs([]string{"bus.ToolResults", "state.dispatchToolResults"}, readable); got != "bus.ToolResults,state.dispatchToolResults" {
		t.Errorf("changed carriers are listed by name: %q", got)
	}
	many := make([]string, 14)
	for i := range many {
		many[i] = fmt.Sprintf("bus.F%02d", i)
	}
	if got := describeChangedAnswerInputs(many, readable); !strings.HasSuffix(got, "bus.F11,…") || strings.Contains(got, "bus.F12") {
		t.Errorf("a long list is cut after twelve names: %q", got)
	}
	if got := describeChangedAnswerInputs(nil, readable); got != "refused state not sampled" {
		t.Errorf("no named change means the refused state had no digest: %q", got)
	}
	makeUnsampleable(bus)
	unreadable := types.SampleAnswerRequestInputs(bus, types.AnswerDispatchStamp{})
	if got := describeChangedAnswerInputs(nil, unreadable); !strings.HasPrefix(got, "state not sampled: ") {
		t.Errorf("an unreadable state is reported as such: %q", got)
	}
}
