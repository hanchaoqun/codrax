package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFormatLogTriageStructured_NilBundle_EmptyString pins the skip
// signal: nil bundle produces no output so callers append no section.
func TestFormatLogTriageStructured_NilBundle_EmptyString(t *testing.T) {
	if got := formatLogTriageStructured(nil, nil); got != "" {
		t.Errorf("nil bundle: got %q, want empty", got)
	}
}

// TestFormatLogTriageStructured_EmptyBundle_EmptyString verifies a
// bundle with no Lang + no Errors + no Residue is treated the same
// as nil — no section rendered.
func TestFormatLogTriageStructured_EmptyBundle_EmptyString(t *testing.T) {
	got := formatLogTriageStructured(&types.LogBundle{}, nil)
	if got != "" {
		t.Errorf("empty bundle: got %q, want empty", got)
	}
}

// TestFormatLogTriageStructured_MetaRendered verifies the Meta block
// surfaces Lang / Signals / Coverage / IntentHint while omitting the
// triager's auxiliary free-form Summary from downstream prompts.
func TestFormatLogTriageStructured_MetaRendered(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "java",
			Signals: []types.LogSignal{types.SignalPanic, types.SignalDB},
			Summary: "database connection refused",
		},
		Errors:     []types.LogError{{Type: "RuntimeException"}},
		Coverage:   0.92,
		IntentHint: types.IntentRootCause,
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"Language: java",
		"Signals: panic, db",
		"Coverage: 0.92",
		"Intent hint: root_cause",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in render:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Summary: database connection refused") {
		t.Fatalf("structured log prompt must not surface auxiliary summary prose:\n%s", got)
	}
}

func TestFormatLogTriageStructured_IncludesRuntimeTupleDiscipline(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{{
			Type: "nil pointer dereference",
			Frames: []types.LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 320,
				Func: "(*analyzerEvaluator).ParseOutput",
				Raw:  "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput(0x0, 0x0, 0x0)",
			}},
		}},
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"Stack-frame argument annotations attached by the runtime artifact's panic / exception / traceback dumper",
		"Do NOT map their positional values to a specific receiver, source parameter, caller-side provenance, or exact downstream branch",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in render:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ParseOutput(0x0, 0x0, 0x0)") {
		t.Fatalf("structured prompt should redact runtime tuple payloads in raw frame surfaces:\n%s", got)
	}
	if !strings.Contains(got, "ParseOutput(...)") {
		t.Fatalf("structured prompt should preserve the frame shape while redacting tuple payloads:\n%s", got)
	}
}

// TestFormatLogTriageStructured_CauseChainNested pins the key fix
// this renderer shipped for: multi-level Cause chains render with
// nested "caused by" markers so the LLM does not need to re-parse
// the tree from raw text. Java / Python / Rust exception wrapping
// all land here.
func TestFormatLogTriageStructured_CauseChainNested(t *testing.T) {
	inner := &types.LogError{
		Type:    "IOException",
		Message: "Connection refused: /10.0.0.5:5432",
	}
	middle := &types.LogError{
		Type:    "NullPointerException",
		Message: "user is null",
		Cause:   inner,
	}
	outer := types.LogError{
		Type:    "RuntimeException",
		Message: "request processing failed",
		Cause:   middle,
	}
	bundle := &types.LogBundle{
		Meta:   types.LogMeta{Lang: "java"},
		Errors: []types.LogError{outer},
	}
	got := formatLogTriageStructured(bundle, nil)
	// All three exception types must appear.
	for _, want := range []string{"RuntimeException", "NullPointerException", "IOException"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing exception %q:\n%s", want, got)
		}
	}
	// Two nesting indicators for depth-1 and depth-2 causes.
	if strings.Count(got, "caused by") < 2 {
		t.Errorf("expected ≥ 2 'caused by' markers for 3-level chain:\n%s", got)
	}
}

// TestFormatLogTriageStructured_ResolvedFramesFlaggedForCitation
// pins the audit-friendly provenance contract: frames with verified
// File+Line get the ★ resolved marker so the LLM knows to cite;
// unresolved frames render WITHOUT the marker and WITHOUT a
// file:line column so the LLM cannot accidentally cite a hallucinated
// path.
func TestFormatLogTriageStructured_ResolvedFramesFlaggedForCitation(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{{
			Type: "runtime error",
			Frames: []types.LogFrame{
				// Verified — File set + Line > 0.
				{
					File:       "internal/agent/analyzer.go",
					Line:       42,
					Func:       "main.crashy",
					Raw:        "	internal/agent/analyzer.go:42 +0x1e",
					Confidence: 0.95,
				},
				// Unresolved — File cleared (hallucination filter).
				{
					Func:       "main.ghost",
					Raw:        "	/fake/path/nope.go:99 +0x1e",
					Confidence: 0.4,
				},
			},
		}},
	}
	got := formatLogTriageStructured(bundle, nil)

	// Resolved frame: ★ marker + file:line quoted.
	if !strings.Contains(got, "★ resolved") {
		t.Error("resolved frame missing ★ marker")
	}
	if !strings.Contains(got, "internal/agent/analyzer.go:42") {
		t.Error("resolved frame missing file:line")
	}

	// Unresolved frame: NO ★ marker, NO file:line column.
	// The string "nope.go:99" must NOT appear — only the raw field
	// carries it but that's in `raw:` not as a citation anchor.
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ghost") && strings.Contains(line, "★") {
			t.Errorf("unresolved frame got ★ marker (would mislead citation): %s", line)
		}
	}
	// "(unresolved)" tag should appear for the ghost frame.
	if !strings.Contains(got, "(unresolved)") {
		t.Error("unresolved frame missing (unresolved) tag")
	}
}

// TestFormatLogTriageStructured_RawFieldRendered confirms the audit
// contract that every frame carries its Raw text into the prompt so
// an auditor can cross-check the LLM's downstream claims against
// what the log literally said.
func TestFormatLogTriageStructured_RawFieldRendered(t *testing.T) {
	rawText := "main.crashy()\n\t/src/main.go:42 +0x1e"
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File:       "main.go",
				Line:       42,
				Raw:        rawText,
				Confidence: 0.9,
			}},
		}},
	}
	got := formatLogTriageStructured(bundle, nil)
	if !strings.Contains(got, "raw:") {
		t.Error("Raw field not rendered under `raw:` label")
	}
	// Raw text is quoted verbatim (or truncated with marker).
	// The first token should be in the output.
	if !strings.Contains(got, "main.crashy") {
		t.Errorf("Raw text not in output:\n%s", got)
	}
}

// TestFormatLogTriageStructured_ProvenanceLegendPresent pins the
// audit anchor: the bottom legend explicitly tells the LLM which
// frames are citeable and which are not. Without this, an LLM
// reading the section would have to infer the ★ meaning.
func TestFormatLogTriageStructured_ProvenanceLegendPresent(t *testing.T) {
	bundle := &types.LogBundle{
		Meta:   types.LogMeta{Lang: "go"},
		Errors: []types.LogError{{Type: "X"}},
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"Provenance legend",
		"safe to cite as `file:line`",
		"do NOT cite",
		"Cross-check",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("provenance legend missing %q", want)
		}
	}
}

// TestFormatLogTriageStructured_ParallelSnapshotsLabelled confirms
// the renderer distinguishes parallel error snapshots (e.g. Go
// goroutine dump with multiple goroutines panicking on the same
// shared resource) from a single error.
func TestFormatLogTriageStructured_ParallelSnapshotsLabelled(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{
			{Type: "panic: goroutine 15"},
			{Type: "panic: goroutine 87"},
			{Type: "panic: goroutine 120"},
		},
	}
	got := formatLogTriageStructured(bundle, nil)
	if !strings.Contains(got, "3 parallel snapshots") {
		t.Errorf("parallel snapshots not labelled:\n%s", got)
	}
}

// TestFormatLogTriageStructured_UnknownChunksRendered verifies the
// Residue block surfaces unstructured chunks with an explicit
// "NOT citeable" marker, preserving zero-information-loss while
// keeping the LLM from accidentally citing them.
func TestFormatLogTriageStructured_UnknownChunksRendered(t *testing.T) {
	bundle := &types.LogBundle{
		Meta:    types.LogMeta{Lang: "go"},
		Errors:  []types.LogError{{Type: "X"}},
		Residue: types.LogResidue{UnknownChunks: []string{"strange boilerplate", "unparseable fragment"}},
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"Unstructured residue",
		"NOT citeable",
		"strange boilerplate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("residue block missing %q", want)
		}
	}
}

func TestFormatLogTriageStructured_OperationalObservationsRendered(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "unknown"},
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationLineMapping,
			Severity:   types.LogObservationFailure,
			Subject:    "line-number handling",
			Summary:    "the log observed a line-number handling bug and a later answer retry",
			Evidence:   "semantic reviewer reported requested line-number topic was not addressed",
			Diagnostic: true,
			Confidence: 0.9,
		}},
		IntentHint: types.IntentRootCause,
		Coverage:   1,
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"Operational observations",
		"kind=line_mapping",
		"diagnostic=true",
		"line-number handling",
		"two lanes: what the log observed, and what current code evidence proves now",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation render missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Unstructured residue") {
		t.Fatalf("observation-only bundle should not be rendered as residue:\n%s", got)
	}
}

// TestBuildPromptContext_LogTriageSection_RenderedForExplorer pins
// the plumbing for downstream consumer agents: when a bundle is on
// AgentContext.LogTriage, the explorer (and by symmetry extractor /
// finalizer / analyzer) gets the structured section in its prompt.
func TestBuildPromptContext_LogTriageSection_RenderedForExplorer(t *testing.T) {
	bundle := &types.LogBundle{
		Meta:   types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{Type: "runtime error"}},
	}
	ac := &types.AgentContext{
		AgentName:   types.AgentExplorer,
		Stage:       types.StageExplore,
		Objective:   "analyze this crash",
		LogTriage:   bundle,
		AttachedLog: "panic: test",
		Preferences: []string{},
	}
	sk := &skill.Config{Name: "explore-skill", Goal: "investigate"}
	pc := BuildPromptContext(ac, sk)
	found := false
	for _, section := range pc.UserSections {
		if section.Title == "Log Triage — Validated Extraction" {
			found = true
			if !strings.Contains(section.Content, "Language: go") {
				t.Errorf("section rendered but content looks wrong: %s", section.Content)
			}
			break
		}
	}
	if !found {
		titles := make([]string, len(pc.UserSections))
		for i, s := range pc.UserSections {
			titles[i] = s.Title
		}
		t.Errorf("explorer did not get Log Triage Validated Extraction section; titles: %v", titles)
	}
}

// TestBuildPromptContext_LogTriageSection_SkippedForLogTriager pins
// the producer-skip gate: the log_triager agent is the producer of
// the bundle, so giving it the structured view back would be a
// confusing echo — skip for that agent.
func TestBuildPromptContext_LogTriageSection_SkippedForLogTriager(t *testing.T) {
	bundle := &types.LogBundle{
		Meta:   types.LogMeta{Lang: "go"},
		Errors: []types.LogError{{Type: "runtime error"}},
	}
	ac := &types.AgentContext{
		AgentName: types.AgentLogTriager,
		Stage:     types.StageLogTriage,
		Objective: "triage this log",
		LogTriage: bundle,
	}
	sk := &skill.Config{Name: "log-triage-skill", Goal: "triage"}
	pc := BuildPromptContext(ac, sk)
	for _, section := range pc.UserSections {
		if section.Title == "Log Triage — Validated Extraction" {
			t.Fatal("log_triager agent got Log Triage section (producer should not receive its own output back)")
		}
	}
}

// TestFormatLogTriageStructured_ExternalSourceDirective_FiresWhenResolvedZeroErrorsPositive
// pins the session-22 front-loaded directive: when the LLM extracted
// at least one Error from the log but NO frame resolved to a repo
// file, the structured view must open with an "External-source
// log" block telling the LLM upfront that repo files cannot ground
// the literals. The customer-trace pattern (Python traceback into
// a Go repo) lives here.
//
// Directive surfaces BEFORE the Meta block so every agent — not
// just the finalizer — sees it at iter 0 and avoids burning
// explore rounds fishing for ungroundable citations.
func TestFormatLogTriageStructured_ExternalSourceDirective_FiresWhenResolvedZeroErrorsPositive(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "python",
			Signals: []types.LogSignal{types.SignalValidation},
		},
		Errors:        []types.LogError{{Type: "KeyError", Message: "'database'"}},
		ResolvedFiles: nil, // ZERO resolved — frames outside this repo
	}
	got := formatLogTriageStructured(bundle, nil)
	for _, want := range []string{
		"External-source log",
		"resolved_files=0",
		"citation_ref=-1",
		"literal-grounding gate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("directive missing %q in render:\n%s", want, got)
		}
	}
	// The directive must come BEFORE the Meta block so iter 0 sees
	// it immediately. Language line is the first Meta entry.
	directiveIdx := strings.Index(got, "External-source log")
	metaIdx := strings.Index(got, "Language:")
	if directiveIdx < 0 || metaIdx < 0 || directiveIdx > metaIdx {
		t.Errorf("External-source directive must appear before Meta block (directive at %d, Meta at %d):\n%s",
			directiveIdx, metaIdx, got)
	}
	for _, forbidden := range []string{"steps[]", "emit_answer_document.symbols"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("external-source directive must not mention retired V1/V1.5 channels %q:\n%s", forbidden, got)
		}
	}
}

// TestFormatLogTriageStructured_ExternalSourceDirective_NoFireWhenResolvedNonEmpty
// is the negative control: when frames DID resolve to repo files,
// the directive must NOT fire — repo-code grounding is viable, the
// LLM should pursue it normally.
func TestFormatLogTriageStructured_ExternalSourceDirective_NoFireWhenResolvedNonEmpty(t *testing.T) {
	bundle := &types.LogBundle{
		Meta:          types.LogMeta{Lang: "go"},
		Errors:        []types.LogError{{Type: "panic", Message: "runtime error"}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
	}
	got := formatLogTriageStructured(bundle, nil)
	if strings.Contains(got, "External-source log") {
		t.Errorf("directive must not fire when at least one frame resolves to the repo:\n%s", got)
	}
}

// TestFormatLogTriageStructured_ExternalSourceDirective_NoFireWhenErrorsEmpty
// is the second negative control: the "degraded" case (log that
// parsed into no errors at all) must not tag the session as
// external-source — it is a different pathology with its own
// answer-path (graceful fall-through, no log-semantic answer).
func TestFormatLogTriageStructured_ExternalSourceDirective_NoFireWhenErrorsEmpty(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "unknown"},
		Residue: types.LogResidue{
			UnknownChunks: []string{"lorem ipsum dolor sit amet"},
		},
	}
	got := formatLogTriageStructured(bundle, nil)
	if strings.Contains(got, "External-source log") {
		t.Errorf("directive must not fire on degraded (zero-errors) bundles:\n%s", got)
	}
}

// TestBuildPromptContext_LogTriageSection_OmittedWhenNil confirms
// the "no bundle → no section" invariant: agents get nothing when
// no log was attached or the stage degraded.
func TestBuildPromptContext_LogTriageSection_OmittedWhenNil(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "normal question",
		LogTriage: nil, // no bundle
	}
	sk := &skill.Config{Name: "explore-skill", Goal: "investigate"}
	pc := BuildPromptContext(ac, sk)
	for _, section := range pc.UserSections {
		if section.Title == "Log Triage — Validated Extraction" {
			t.Fatal("unexpected Log Triage section when bundle is nil")
		}
	}
}

// Session-22 F3.1: when Signals include panic / crash AND the primary
// error has ≥2 resolved frames, a "### Call chain (innermost → outer)"
// block must render below the Errors tree with each frame tagged as
// innermost / caller / outermost, so the finalizer draws its call-DAG
// diagram from the frames verbatim instead of inventing a caller from
// the ranker's Auxiliary candidates.
func TestFormatLogTriageStructured_RendersCallChainForPanic(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalPanic, types.SignalCrash},
		},
		Errors: []types.LogError{
			{
				Type:    "nil pointer dereference",
				Message: "runtime error: invalid memory address or nil pointer dereference",
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR", Confidence: 1.0},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "(*analyzerEvaluator).ParseOutput", Confidence: 1.0},
				},
			},
		},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		Coverage:      1.0,
	}
	got := formatLogTriageStructured(bundle, nil)
	if !strings.Contains(got, "### Call chain (innermost → outer)") {
		t.Fatalf("panic+crash bundle with 2 resolved frames must render Call chain block; got:\n%s", got)
	}
	// Call-chain block now emits Mermaid (was bare-fence ASCII
	// "innermost failure: …" / "↑ caller:" prose). Assert on the
	// Mermaid markers + grounded labels which survive the format
	// change.
	for _, want := range []string{
		"```mermaid",
		"flowchart",
		"internal/agent/analyzer.go:250",
		"internal/agent/analyzer.go:320",
		"-->",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mermaid call-chain block missing %q; got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "diagram grounding gate") {
		t.Errorf("block must reference the diagram grounding gate so LLM knows the downstream contract; got:\n%s", got)
	}
}

// Session-22 F3.1 (negative): non-crash bundles (oom / timeout /
// validation / ...) do NOT get the Call chain block — the frames may
// be a middleware chain rather than a failure call path.
func TestFormatLogTriageStructured_SkipsCallChainForNonCrashSignals(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "python",
			Signals: []types.LogSignal{types.SignalValidation},
		},
		Errors: []types.LogError{
			{
				Type: "ValidationError",
				Frames: []types.LogFrame{
					{File: "a.py", Line: 10, Func: "validate", Confidence: 1.0},
					{File: "b.py", Line: 20, Func: "parse", Confidence: 1.0},
				},
			},
		},
		ResolvedFiles: []string{"a.py", "b.py"},
	}
	got := formatLogTriageStructured(bundle, nil)
	if strings.Contains(got, "### Call chain") {
		t.Errorf("non-crash signals must skip Call chain block; got:\n%s", got)
	}
}

// Session-22 F3.1: a panic bundle with only one resolved frame yields
// no Call chain — the chain needs at least a caller and a callee to
// carry meaning.
func TestFormatLogTriageStructured_SkipsCallChainWithSingleFrame(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{
			{
				Type: "panic",
				Frames: []types.LogFrame{
					{File: "a.go", Line: 5, Func: "main", Confidence: 1.0},
				},
			},
		},
		ResolvedFiles: []string{"a.go"},
	}
	got := formatLogTriageStructured(bundle, nil)
	if strings.Contains(got, "### Call chain") {
		t.Errorf("single-frame panic must skip Call chain block; got:\n%s", got)
	}
}
