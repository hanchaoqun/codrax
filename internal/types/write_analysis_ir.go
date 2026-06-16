package types

// WriteAnalysisIR is the write-mode peer to AnalysisIR. It captures
// what the user wants to do as a code-modification task, framed in
// write-task vocabulary (kind / scope / risk / constraints) rather
// than read-mode question-answering vocabulary (intent / scenario /
// complexity / answer_subject).
//
// The two IRs coexist on BusContext.Mutable. Read-mode Runs only
// populate AnalysisIR; write-mode Runs populate both because the
// existing read analyzer still runs as a classifier providing
// keywords for ranking, while the new write_analyzer stage adds
// task-shape facts the write agents (planner / coder / verifier /
// reflector) consume directly. Old AnalysisIR fields like Intent /
// Scenario are not consulted by write agents — they look up
// WriteAnalysisIR.Request.Task instead.
//
// All structural fields below are populated by the LLM through
// emit_write_analysis. Down-stream system code only validates and
// reads. No system-side keyword extraction or substring matching:
// every classification decision is the model's, gated by the
// emit-tool schema.
type WriteAnalysisIR struct {
	Request         WriteRequestModel `json:"request"`
	PhaseProposal   PhaseProposal     `json:"phase_proposal"`
	PitfallsApplied []string          `json:"pitfalls_applied,omitempty"`
	PrescanFiles    []string          `json:"prescan_files,omitempty"`
	RepoFacts       WriteRepoFacts    `json:"repo_facts"`
}

// WriteRequestModel captures the user's code-change request.
//
// Field decisions:
//   - RawRequest: stored on the IR (not just BusContext) so a serialised
//     write IR is self-contained for debug logs / replay tests.
//   - Task: the LLM's classification of what kind of work this is.
//   - Risk: the LLM's judgement, plus a system reconciliation pass that
//     tightens AffectsPublicAPI / ChangesPersistence / ChangesBuildSystem
//     when ExpectedOutcomes contradicts the LLM's claim.
//   - ScopeAnchors: directories / files the LLM identified as the centre
//     of the change. These seed the planner's "where to read first" view.
//   - Constraints: must-keep / must-not-touch the user mentioned. The LLM
//     extracts these from the request body; the planner renders them
//     verbatim into its prompt.
//   - ExpectedOutcomes: 2-4 short concrete signals that the change is
//     done correctly. The reflector reads these so its critique can
//     judge "does the plan move toward the goal" not just "did tests
//     pass".
//   - BehaviorContracts: optional typed observables that downstream plan and
//     verify artifacts can reference by ID. These replace prose parsing for
//     hard coverage checks.
type WriteRequestModel struct {
	RawRequest        string                  `json:"raw_request"`
	Task              WriteTask               `json:"task"`
	Risk              WriteRiskProfile        `json:"risk"`
	ScopeAnchors      []string                `json:"scope_anchors,omitempty"`
	Constraints       []WriteConstraint       `json:"constraints,omitempty"`
	ExpectedOutcomes  []string                `json:"expected_outcomes,omitempty"`
	BehaviorContracts []WriteBehaviorContract `json:"behavior_contracts,omitempty"`
}

// WriteTask is the LLM's category + scope + one-line restatement.
type WriteTask struct {
	Kind    WriteTaskKind `json:"kind"`
	Scope   WriteScope    `json:"scope"`
	Summary string        `json:"summary"`
}

// WriteTaskKind is the closed set of write-task categories. The
// values are LLM-facing in the emit_write_analysis schema; renaming
// any of them is a wire-format change.
type WriteTaskKind string

const (
	// WriteTaskFeature: build a new piece of functionality. Mostly
	// kind=create work.
	WriteTaskFeature WriteTaskKind = "feature"

	// WriteTaskBugfix: correct an existing wrong behaviour. Mostly
	// kind=modify / kind=patch with small surface.
	WriteTaskBugfix WriteTaskKind = "bugfix"

	// WriteTaskRefactor: change shape without changing behaviour.
	// Mix of kind=modify and (when added) kind=rename.
	WriteTaskRefactor WriteTaskKind = "refactor"

	// WriteTaskTest: add or improve test coverage. The verifier's
	// "0 tests discovered → unverified" branch checks for this kind
	// to decide whether a no-test outcome is benign.
	WriteTaskTest WriteTaskKind = "test"

	// WriteTaskDocs: documentation / comments only.
	WriteTaskDocs WriteTaskKind = "docs"

	// WriteTaskConfig: configuration files / dependency manifest
	// updates / env file changes.
	WriteTaskConfig WriteTaskKind = "config"

	// WriteTaskMisc: anything that does not fit the buckets above.
	// LLM fall-back, planner treats as an open task.
	WriteTaskMisc WriteTaskKind = "misc"
)

// IsKnownWriteTaskKind reports whether v is a canonical kind. Used
// by the validator to gate schema-acceptable values; non-canonical
// values fall back to WriteTaskMisc.
func IsKnownWriteTaskKind(v string) bool {
	switch WriteTaskKind(v) {
	case WriteTaskFeature, WriteTaskBugfix, WriteTaskRefactor,
		WriteTaskTest, WriteTaskDocs, WriteTaskConfig, WriteTaskMisc:
		return true
	}
	return false
}

// WriteScope is the LLM's call on how widely the change ripples.
// Used by the verifier to scale ERM thresholds and (post-Phase 2)
// by the scope_runner to decide whether to scope test runs.
type WriteScope string

const (
	// ScopeMicro: change is contained to one function or one
	// constant in one file.
	ScopeMicro WriteScope = "micro"

	// ScopePackage: change ripples within a single package.
	ScopePackage WriteScope = "package"

	// ScopeCross: change touches multiple unrelated components.
	ScopeCross WriteScope = "cross"

	// ScopeProject: project-wide change (build, infra, refactor
	// across the repo).
	ScopeProject WriteScope = "project"
)

// IsKnownWriteScope reports whether v is a canonical scope value.
func IsKnownWriteScope(v string) bool {
	switch WriteScope(v) {
	case ScopeMicro, ScopePackage, ScopeCross, ScopeProject:
		return true
	}
	return false
}

// WriteRiskProfile captures three boolean risk axes plus an overall
// LLM judgement. The booleans are independent: a change might affect
// build system without touching public API. Overall is a simple
// "low" / "medium" / "high" string the LLM picks; the planner shows
// it verbatim in the prompt and the verifier reads it to pick a
// stricter or laxer ERM.
//
// Distinct from the read-mode RiskLevel struct (which is a 0-5
// scalar + evidence list driven by the read-mode risk-matrix
// evaluator) because write tasks don't need a 5-level matrix here —
// the booleans already cover the axes that matter, and the overall
// label is just a roll-up.
type WriteRiskProfile struct {
	AffectsPublicAPI   bool          `json:"affects_public_api"`
	ChangesPersistence bool          `json:"changes_persistence"`
	ChangesBuildSystem bool          `json:"changes_build_system"`
	Overall            WriteRiskBand `json:"overall"`
}

// WriteRiskBand is the closed set of overall risk labels the LLM
// picks. Keep stable as wire format.
type WriteRiskBand string

const (
	RiskBandLow    WriteRiskBand = "low"
	RiskBandMedium WriteRiskBand = "medium"
	RiskBandHigh   WriteRiskBand = "high"
)

// IsKnownWriteRiskBand reports whether v is a canonical band.
func IsKnownWriteRiskBand(v string) bool {
	switch WriteRiskBand(v) {
	case RiskBandLow, RiskBandMedium, RiskBandHigh:
		return true
	}
	return false
}

// WriteConstraint is one user-stated constraint extracted from the
// request body. Kind is a short label the LLM picks ("preserve_api",
// "no_external_deps", "match_existing_style", "no_breaking_changes",
// or any other free string when none of those fit). Target names the
// scoped object (path / symbol / "*"). Note holds a short quote of
// the user's original wording for traceability.
type WriteConstraint struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
	Note   string `json:"note,omitempty"`
}

// PhaseProposal is the LLM's suggestion for whether the task should
// be split into sequential phases. Phase 1 of the dual-analyzer
// rollout records the proposal but does NOT execute it (split is
// downgraded to "single" by the validator); Phase 2 will wire it
// through to a multi-phase TaskGraph. Until then this field is
// informational and validate-only.
type PhaseProposal struct {
	Split  string      `json:"split"` // "single" | "sequential" | "parallel"
	Phases []PhaseSeed `json:"phases,omitempty"`
}

// PhaseSeed is one proposed phase. Goal is the LLM's per-phase
// objective; RoughTargetPaths are anchors the LLM expects each phase
// to touch.
type PhaseSeed struct {
	Goal             string   `json:"goal"`
	RoughTargetPaths []string `json:"rough_target_paths,omitempty"`
}

// IsKnownPhaseSplit reports whether v is a canonical split value.
func IsKnownPhaseSplit(v string) bool {
	switch v {
	case "single", "sequential", "parallel":
		return true
	}
	return false
}

// WriteRepoFacts carries deterministic facts about the repository
// that the validator computes (or copies from existing detection
// helpers) and stitches into the IR so the planner / verifier do
// not have to reprobe.
type WriteRepoFacts struct {
	PrimaryLanguage string   `json:"primary_language,omitempty"`
	Manifests       []string `json:"manifests,omitempty"`
	GitState        string   `json:"git_state,omitempty"` // "clean" / "dirty" / "bare-dir" / "has-staged" / "unknown"
	HasTests        bool     `json:"has_tests"`
	TestRunner      string   `json:"test_runner,omitempty"`
}
