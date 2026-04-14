package skill

// RegisterDefaults registers all built-in skill configurations.
func RegisterDefaults(r *Registry) {
	r.Register(&Config{
		Name: "task-analysis-skill",
		Goal: "Classify the user request into a v3 RequestModel (intent, scenario, complexity, writing, keywords, entities, question_kind, answer_shape) via a single emit_analysis tool call.",
		Workflow: []string{
			"read user input and detect its language",
			"pick intent from the v3 enum (explain/trace/enumerate/root_cause/return_value/config_query/refactor/bugfix/security_audit/unknown)",
			"pick scenario from the v3 enum (architecture_explain/root_cause/security_audit/refactor_design/config_trace/performance_bottleneck/generic)",
			"pick complexity (simple/moderate/complex) from the number of files the answer likely needs",
			"set writing (true only if files may be mutated) and high_risk (true only when writing=true and change is irreversible or security-sensitive)",
			"extract entities VERBATIM from the user's text — CamelCase/snake_case only, no generic nouns",
			"generate ≥8 keywords in three rounds — core terms, compound identifiers, action synonyms",
			"pick question_kind and answer_shape from the ERM / finalizer enums",
			"call emit_analysis EXACTLY ONCE with the classified fields",
		},
		ToolSuggestions: []string{
			"emit_analysis",
		},
		OutputFormat: `Call emit_analysis EXACTLY ONCE with all required fields: intent, scenario, complexity, writing, keywords, entities, question_kind, answer_shape (high_risk optional, defaults false). ` +
			`The system synthesises the TermGraph, TaskGraph, RiskMatrix, EvidencePlan, AnswerContract, and Hypotheses deterministically from your input — do not provide them. ` +
			`Keyword generation (3 rounds): (1) Core — extract every domain noun and verb from the question in both original and identifier forms (CamelCase, snake_case). (2) Compound — cross-combine core terms into plausible multi-word identifiers (CacheStore, store_config). (3) Synonyms — for each verb add 2-3 programming synonyms (send → emit/dispatch/publish). The system auto-expands each keyword into case variants, so produce diverse STEMS rather than repeating words. ` +
			`After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log.`,
		Prohibitions: []string{
			"do not call any tool other than emit_analysis",
			"do not write prose before the emit_analysis call",
			"do not make assumptions about code structure",
			"do not start implementation",
			"do not translate or re-case entities — copy them verbatim from the user's text",
		},
	})

	r.Register(&Config{
		Name: "repo-explore-skill",
		Goal: "Investigate the user's question and answer it directly using evidence from the code.",
		Workflow: []string{
			"PHASE 1 — Breadth scan: use repo_map and grep (with files_only=true) to discover ALL relevant files. Do not read files in full yet. Classify each file by role (type definitions, core logic, configuration/rules, entry point). Output a prioritized list of 3-6 files to investigate",
			"PHASE 2 — Depth read: read each file from your list in full (no offset/limit for <500 lines). For each file extract: (a) key data structures and fields, (b) control flow and branching logic, (c) configuration-driven behavior, (d) cross-component interactions",
			"if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
			"cross-reference: when file A references file B, read file B too — don't assume, verify",
			"never read test files — they are derivative, not authoritative. Never read utility/infrastructure files unless the question is about them",
		},
		ToolSuggestions: []string{
			"repo_map",
			"grep",
			"read_file",
			"list_files",
			"exec_command",
			"todo_write",
		},
		// OutputFormat is shape-by-example, not shape-by-rule. The previous
		// abstract description ("Direct answer ... plus file:line citations")
		// failed to bind the LLM away from its training-distribution default
		// of cataloger sections. A single concrete example fixed that, but
		// over-fit in two ways: (1) the example was the trigger query's
		// answer verbatim, contaminating the very query it was supposed to
		// validate, and (2) it pinned the answer length at "one or two
		// sentences", which compresses any genuine multi-paragraph
		// explanation request into a degenerate two-sentence summary plus
		// file list.
		//
		// The fix is three concrete examples covering three answer scales
		// (lookup, count, multi-paragraph explanation), all using content
		// completely unrelated to this codebase. The principle "scale the
		// answer to the question" is stated above the examples in plain
		// English. The examples teach format flexibility (the Answer block
		// scales) without teaching specific content (the LLM can't copy any
		// of these because none of them are about this repo).
		OutputFormat: `The shape of your answer must match the shape of the question. Lead with the direct answer at the right level of detail — one sentence for a count, name, or yes/no question; multiple paragraphs for an explanation, walkthrough, or comparison. Match the answer's depth to the question's depth: do not pad a one-sentence answer into prose, and do not compress a multi-paragraph explanation into two sentences. Always ground load-bearing claims in a file:line citation.

Use this shape — Answer first, then Evidence:

Answer: <direct answer at the right depth>
Evidence:
- <file:line> — <what this establishes>
- <file:line> — <what this establishes>

If part of the question genuinely cannot be answered from the code you read, add one short sentence naming the missing piece — do not substitute "further analysis required" for an answer the evidence supports.

Below are three illustrative examples drawn from a hypothetical, unrelated codebase. They show how the Answer block scales — DO NOT copy their content, only their shape:

Example A — lookup question (one short sentence is the right depth):
Answer: The project uses version 3.12 of the language runtime.
Evidence:
- pyproject.toml:7 — python = "^3.12"

Example B — count question (one sentence + several pieces of evidence):
Answer: There are 4 HTTP handlers registered on the public router.
Evidence:
- src/routes.py:18-21 — route registrations for /health, /users, /orders, /metrics
- src/routes.py:42 — no registrations after line 21

Example C — explanation question (multi-paragraph is the right depth):
Answer: The cache is a write-through layer between the API handlers and the database. On every write the handler updates the database first and then invalidates the corresponding cache key; the next read repopulates the cache from the database. There is no read-through-on-miss path: a cache miss is served directly from the database without writing back, which is intentional so that stale data from a misbehaving writer cannot persist beyond one request.

The cache key format is "<resource>:<id>" and the default TTL is 5 minutes. Eviction is purely LRU; there is no manual flush API.

Evidence:
- lib/cache.rb:42-58 — set() invalidates the key after the DB write returns
- lib/cache.rb:78-91 — get() falls through to the DB on miss but does not write back
- lib/cache.rb:14 — DEFAULT_TTL = 300

Example D — architecture/flow question (use a Mermaid diagram when a visual clarifies relationships or sequences):
Answer: A write request flows through three layers before reaching the database:

` + "```mermaid" + `
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant CA as Cache
    participant DB as Database
    C->>H: PUT /api/v1/users/42
    H->>DB: UPDATE users SET ...
    DB-->>H: ok
    H->>CA: invalidate("users:42")
    H-->>C: 200 OK
` + "```" + `

The handler always writes to the database first. Only after the DB confirms success does it invalidate the cache key.

Evidence:
- app/handlers/user_handler.ts:87-102 — updateUser calls repo.save then cache.invalidate
- app/cache/store.ts:42-58 — invalidate deletes the key, next get repopulates from DB`,
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
			"do not stop at 'the answer would require checking X' — go check X yourself",
			"do not write about what would be done next or what the user should do — answer only what was asked",
			"do not treat repo_map output as evidence — it is a cached navigation index that tells you where to look, not what is true. After consulting repo_map, always read_file or grep the actual source files to establish facts. list_files is fine as evidence since it reads the real directory",
		},
	})

	r.Register(&Config{
		Name: "implementation-plan-skill",
		Goal: "Design a step-by-step implementation plan.",
		Workflow: []string{
			"review objective and facts",
			"identify files to modify",
			"design change sequence",
			"assess risks",
			"define verification approach",
			"estimate impact scope",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
		},
		OutputFormat: "JSON with plan containing files_to_modify, steps, risks, validation",
		Prohibitions: []string{
			"do not write code",
			"do not modify files",
		},
	})

	r.Register(&Config{
		Name: "code-implement-skill",
		Goal: "Write or modify code to implement the plan.",
		Workflow: []string{
			"review plan steps",
			"implement changes file by file",
			"generate patches",
			"verify syntax",
			"add necessary imports",
			"handle edge cases",
		},
		ToolSuggestions: []string{
			"read_file",
			"apply_patch",
			"exec_command",
			"grep",
			"todo_write",
		},
		OutputFormat: "JSON with patch, modified_files, implementation_notes",
		Prohibitions: []string{
			"do not deviate from the plan without justification",
			"do not delete unrelated code",
		},
	})

	r.Register(&Config{
		Name: "design-review-skill",
		Goal: "Review plan feasibility, architecture impact, and risks.",
		Workflow: []string{
			"check requirement alignment",
			"verify architecture integrity",
			"evaluate edge cases",
			"assess security and performance risks",
			"check for missing steps",
			"provide recommendations",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
		},
		OutputFormat: "JSON with review_result (pass/fail), issues, must_fix, suggestions",
		Prohibitions: []string{
			"do not modify the plan",
			"do not implement fixes",
		},
	})

	r.Register(&Config{
		Name: "code-review-skill",
		Goal: "Review code correctness, bugs, style, and side effects.",
		Workflow: []string{
			"check plan conformance",
			"verify logic correctness",
			"check boundary conditions",
			"review error handling",
			"assess code style",
			"check for side effects",
			"verify backwards compatibility",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
			"git_diff",
		},
		OutputFormat: "JSON with review_result (pass/fail), code_issues, must_fix",
		Prohibitions: []string{
			"do not modify code",
			"do not auto-fix issues",
		},
	})

	r.Register(&Config{
		Name: "verification-skill",
		Goal: "Verify implementation correctness through testing.",
		Workflow: []string{
			"compile the project",
			"run unit tests",
			"run linter",
			"execute smoke tests",
			"verify runtime behavior",
			"report results",
		},
		ToolSuggestions: []string{
			"exec_command",
			"run_tests",
			"read_file",
		},
		OutputFormat: "JSON with verification_result (pass/fail), logs, errors, next_action",
		Prohibitions: []string{
			"do not modify source code",
			"do not skip failing tests",
		},
	})

	r.Register(&Config{
		Name: "final-answer-skill",
		Goal: "Produce the final user-facing output for an implementation task.",
		Workflow: []string{
			"summarize all changes",
			"compile patch information",
			"write usage instructions",
			"list action steps",
			"mark tasks complete",
		},
		ToolSuggestions: []string{},
		OutputFormat: "Markdown with summary, changes, and instructions. " +
			"Use tables for structured data (file lists, config comparisons). " +
			"Use Mermaid diagrams (```mermaid) when a visual would clarify architecture, " +
			"data flow, or sequence of operations.",
		Prohibitions: []string{
			"do not introduce new changes",
			"do not omit failures or warnings",
		},
	})

	// analysis-final-answer-skill is the finalize-stage skill used when the
	// task is read-only / question-answering (Writing == false). The default
	// final-answer-skill is shaped for implementation tasks: its workflow is
	// "summarize all changes", "compile patch information", "write usage
	// instructions", "list action steps", "mark tasks complete" — verbs that
	// presuppose a code change actually happened. Forcing an analysis answer
	// through that template produces verbose templated prose with invented
	// "Action Steps" and "Tasks Completion" sections, and it dilutes precise
	// quantitative answers ("1 SubExplorer") into mushy ones ("several
	// components facilitate subagent management"). The orchestrator's
	// dispatchStage routes to this skill when determineActivePolicy()
	// returns "analysis".
	r.Register(&Config{
		Name: "analysis-final-answer-skill",
		Goal: "Answer the user's question directly using the evidence the prior stages collected, at the right level of detail for what was asked.",
		Workflow: []string{
			"read the user's original question and identify whether it asks for a fact (count/name/yes-no), an explanation (how/why/walkthrough), or a comparison",
			"read the prior stage findings (especially the explorer's stage report) and identify the specific answer the evidence supports",
			"if the explorer's stage report already contains a complete, well-structured answer to the question, adopt it directly — restructure into the required output format (Answer/Evidence) but do NOT rephrase, rewrite, or re-explain the same content in different words",
			"only add new material if the explorer's report is missing part of the answer or lacks citations; otherwise your job is formatting, not rewriting",
			"ground load-bearing claims in file:line citations from the prior stages",
			"if part of the question is unanswered, say which part and why — do not substitute templated boilerplate for an answer the evidence supports",
		},
		ToolSuggestions: []string{},
		// OutputFormat shares the same shape-by-example design as
		// repo-explore-skill above. See the comment there for the rationale
		// and the over-fitting failure mode that motivated three examples
		// instead of one. The two skills use the same Answer/Evidence
		// shape so that the explorer's stage_report can flow through the
		// finalizer with minimal rewriting.
		OutputFormat: `The shape of your answer must match the shape of the question. Lead with the direct answer at the right level of detail — one sentence for a count, name, or yes/no question; multiple paragraphs for an explanation, walkthrough, or comparison. Match the answer's depth to the question's depth: do not pad a one-sentence answer into prose, and do not compress a multi-paragraph explanation into two sentences. Always ground load-bearing claims in a file:line citation.

Use this shape — Answer first, then Evidence:

**Answer:** <direct answer at the right depth>

**Evidence:**
- <file:line> — <what this establishes>
- <file:line> — <what this establishes>

(Optional) **Caveat:** <only if part of the question genuinely cannot be answered from the gathered evidence — name the missing piece in one short sentence, do not pad>

Below are three illustrative examples drawn from a hypothetical, unrelated codebase. They show how the Answer block scales — DO NOT copy their content, only their shape:

Example A — lookup question (one short sentence is the right depth):
**Answer:** The project uses version 3.12 of the language runtime.
**Evidence:**
- pyproject.toml:7 — python = "^3.12"

Example B — count question (one sentence + several pieces of evidence):
**Answer:** There are 4 HTTP handlers registered on the public router.
**Evidence:**
- src/routes.py:18-21 — route registrations for /health, /users, /orders, /metrics
- src/routes.py:42 — no registrations after line 21

Example C — explanation question (multi-paragraph is the right depth):
**Answer:** The cache is a write-through layer between the API handlers and the database. On every write the handler updates the database first and then invalidates the corresponding cache key; the next read repopulates the cache from the database. There is no read-through-on-miss path: a cache miss is served directly from the database without writing back, which is intentional so that stale data from a misbehaving writer cannot persist beyond one request.

The cache key format is "<resource>:<id>" and the default TTL is 5 minutes. Eviction is purely LRU; there is no manual flush API.

**Evidence:**
- lib/cache.rb:42-58 — set() invalidates the key after the DB write returns
- lib/cache.rb:78-91 — get() falls through to the DB on miss but does not write back
- lib/cache.rb:14 — DEFAULT_TTL = 300

Example D — architecture/flow question (use a Mermaid diagram when a visual clarifies relationships or sequences):
**Answer:** A write request flows through three layers before reaching the database:

` + "```mermaid" + `
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant CA as Cache
    participant DB as Database
    C->>H: PUT /api/v1/users/42
    H->>DB: UPDATE users SET ...
    DB-->>H: ok
    H->>CA: invalidate("users:42")
    H-->>C: 200 OK
` + "```" + `

The handler always writes to the database first. Only after the DB confirms success does it invalidate the cache key. This guarantees that a crash between the two steps leaves stale-but-safe data in the cache rather than new data in the cache with an uncommitted DB row.

**Evidence:**
- app/handlers/user_handler.ts:87-102 — updateUser calls repo.save then cache.invalidate
- app/cache/store.ts:42-58 — invalidate deletes the key, next get repopulates from DB`,
		Prohibitions: []string{
			"do not invent next steps the user did not ask for",
			"do not write 'usage instructions' or 'action steps' for a question that asked for an answer rather than for changes",
			"do not substitute 'further investigation needed' for an answer the prior stages already established",
			"do not repeat or paraphrase content that the explorer's stage report already stated — if the explorer answered it, adopt that text as-is instead of rewriting it in your own words",
		},
	})

	// The structured-finalizer skill. Holds the complete declarative
	// contract for the emit_answer_document tool channel: shape
	// dispatch table, citation pool semantics, completeness honesty
	// contract, prohibitions. The evaluator (answerDocumentEvaluator)
	// only renders DYNAMIC per-dispatch content (resolved target
	// shape, MustInclude floor, prior extraction slate) so a shape
	// addition or prompt re-word lives here in declarative config,
	// one grep away, instead of in a Go string builder.
	r.Register(&Config{
		Name: "answer-document-skill",
		Goal: "Produce the final answer as a structured AnswerDocument by calling emit_answer_document exactly once. A deterministic renderer turns the structure into user-visible prose.",
		Workflow: []string{
			"Read the resolved target shape from the user section (list_of_symbols / step_list / value / boolean / config_value / explanation)",
			"For list_of_symbols shape: inspect the prior extraction slate and the analyzer MustInclude floor rendered in the user section, assemble the symbols[] array from them, and set symbols_completeness to 'complete' only if your slate reaches the floor — otherwise set it to 'lower_bound'",
			"For step_list shape: emit steps[] with one entry per distinct branch or mechanism hop; each step carries a positive index, a one-sentence description drawn from evidence, and a citation_ref into the shared citations pool (or -1 when no citation backs the step)",
			"For value / config_value shape: emit value{literal} (plus key for config_value) with a citation_ref into the pool",
			"For boolean shape: emit boolean{decision, rationale, citation_ref}; decision must be one of true/false/yes/no/是/否 — no hedging",
			"For explanation shape: fill summary with 1-2 sentence lead-in prose (≤500 chars), populate citations[] for any referenced file:line",
			"Declare every file:line you cite ONCE in the citations[] array; other fields reference it by zero-based integer index (or -1 for no cite). One cited line can serve multiple steps without duplication",
		},
		ToolSuggestions: []string{
			"emit_answer_document",
		},
		OutputFormat: `Produce ZERO free-form prose. Your entire contribution is ONE emit_answer_document tool call per dispatch — the finalizer reads the Mutable buffer and runs a deterministic renderer, not your assistant text.

Required-field dispatch by shape (see the tool's JSON schema for the full contract):

- shape=list_of_symbols → symbols[] (non-empty) + symbols_completeness ∈ {complete, lower_bound, unknown}
- shape=step_list       → steps[] (non-empty), each with index + description + citation_ref
- shape=value           → value{literal, citation_ref} (key omitted)
- shape=config_value    → value{key, literal, citation_ref}
- shape=boolean         → boolean{decision, rationale, citation_ref}
- shape=explanation     → summary (non-empty, ≤500 chars)

Forbidden-field rules:
- list_of_symbols forbids steps / value / boolean
- step_list forbids symbols / value / boolean
- value + config_value forbid steps / symbols / boolean
- boolean forbids steps / symbols / value
- explanation forbids steps / symbols / value / boolean

Citation pool:
- citations[] is a shared zero-based array of {file, line, quote?}
- Every citation_ref elsewhere (steps[i].citation_ref, value.citation_ref, boolean.citation_ref) is an integer index into citations[], or -1 when no citation backs that entry
- Every citations[i].file MUST be a repo-relative path and MUST NOT live inside the per-trace WorkDir (blob directory)
- Every citations[i].line MUST be > 0 — line-hallucination guard

Completeness honesty contract (list_of_symbols only):
- "complete" — you assert this list enumerates EVERY symbol that answers the question. The finalizer cardinality validator will cross-check against max(Turn A terminal-evidence count β, analyzer MustInclude γ). A short claim of "complete" is DOWNGRADED to "lower_bound" with a visible caveat in the rendered answer.
- "lower_bound" — symbols are confirmed present, more may exist. Honest default when you cannot confidently reach the floor.
- "unknown" — investigated but no definitive slate. Renderer drops the section entirely and falls back to the shape-based prompt.

Summary field (shape=explanation or optional lead-in for others):
- LLM-authored 1-2 sentence lead-in, ≤500 chars — the ONE prose escape hatch. Do not pad this into an answer body.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item",
			"do not inflate summary past 500 characters — it is a 1-2 sentence lead-in, not the answer body",
			"do not set citation_ref to a zero-value-looking sentinel; use -1 for 'no citation' and a valid pool index otherwise",
			"do not claim symbols_completeness=complete without meeting the floor shown in the cardinality baseline — a short 'complete' claim will be downgraded to lower_bound automatically and the downgrade is surfaced as a caveat",
		},
	})

	// Turn B — the extractor's skill. Declarative contract surface
	// that context/builder.go auto-renders into system sections
	// (Workflow, Prohibitions) and schema scope (ToolSuggestions).
	// Keeping Turn B's role, tool allowlist, output format, and
	// honesty contract in this file — rather than baked into
	// extractor.go's BuildInitialPrompt string builder — means the
	// contract is one grep away, the stable parts render once as
	// system sections instead of being appended per dispatch, and
	// BaseAgent.buildToolSchemas scopes the LLM tool set from
	// ToolSuggestions here.
	//
	// extractor.go's BuildInitialPrompt only handles the DYNAMIC
	// per-dispatch data: the Turn A transcript digest (investigation
	// notes, read files, top evidence, flow findings, cardinality
	// baseline, hypothesis set). Static contract lives here.
	r.Register(&Config{
		Name: "extract-skill",
		Goal: "Produce the answer-symbol slate and the per-hypothesis verdicts from Turn A's frozen investigation transcript. Evidence is Turn A's territory — Turn B never re-emits it. Turn B's two unique jobs are (1) LLM-driven answer_symbol selection with a completeness claim the finalizer cross-checks, and (2) LLM-driven hypothesis judgement with a citation.",
		Workflow: []string{
			"Read the Turn A transcript digest the orchestrator injected as a user section: user question, investigation notes, read files, top evidence items, dataflow findings, cardinality baseline (β = Turn A terminal-evidence count, γ = analyzer MustInclude count, effective floor = max(β, γ)), and hypothesis set",
			"For the answer-symbol slate (only for list_of_symbols / enumeration / call_chain questions): call emit_answer_symbol ONCE with a batched items array. Each item MUST carry a concrete file:line from a file in the 'Files Turn A read' list — never invent a line number",
			"For emit_answer_symbol, set 'completeness' to 'complete' only if len(items) >= effective floor; otherwise set it to 'lower_bound'. If the question is not a list-of-names question at all, skip emit_answer_symbol entirely (the finalizer will use the shape-based prompt)",
			"For every hypothesis in the hypothesis set: call emit_hypothesis_verdict once with hypothesis_id + status + rationale + citation. Status must be 'confirmed' / 'rejected' / 'inconclusive'. 'confirmed' and 'rejected' REQUIRE a file:line citation; 'inconclusive' is the honest choice when no definitive cite exists",
		},
		ToolSuggestions: []string{
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
		OutputFormat: `Produce ZERO free-form narrative. Your entire contribution is the emit_* tool calls — the finalizer reads the drained buffers, not your assistant text. Call each emit_* tool AT MOST ONCE per dispatch; batch all items in a single call per tool.

Completeness honesty contract for emit_answer_symbol:
- "complete" — you assert this list enumerates EVERY symbol that answers the question. The finalizer will render it with a "MUST NOT add or remove" directive. This claim is AUTOMATICALLY cross-checked against max(β, γ): a short claim of "complete" is DOWNGRADED to "lower_bound" with a warning.
- "lower_bound" — these symbols are all confirmed present, but additional symbols may also be part of the answer. The finalizer will render a softened "at least these, may add more" prompt. This is the HONEST DEFAULT when you cannot confidently reach the floor.
- "unknown" — you investigated but cannot reach a definitive slate. The finalizer will DROP the answer-symbol section entirely and fall back to a shape-based prompt. Choose this for mechanism questions, value/boolean questions, or genuinely ambiguous evidence.`,
		Prohibitions: []string{
			"do not call read_file, grep, repo_map, list_files, or exec_command — the Turn A transcript is frozen and Turn B has no file access",
			"do not call emit_evidence — evidence is Turn A's exclusive channel. Turn A already emitted evidence during Phase 1 and the orchestrator merged it into BusContext.EvidenceItems before this dispatch. Your job is to consume that evidence, not re-emit it.",
			"do not invent line numbers — if the transcript does not have a line for a symbol, omit that symbol",
			"do not cite files outside the 'Files Turn A read' list provided in the user section",
			"do not fabricate an answer-symbol list to fill the section — if the question is a mechanism / value / boolean question, skip emit_answer_symbol and let the finalizer use the shape-based prompt",
			"do not claim completeness=complete without verifying len(items) >= max(β, γ) from the cardinality baseline — a short 'complete' claim will be downgraded to lower_bound automatically and the downgrade is logged as a warning",
			"do not choose hypothesis status=confirmed or rejected without a concrete file:line citation — use 'inconclusive' when no cite exists",
		},
	})
}
