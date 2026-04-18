package skill

// RegisterDefaults registers all built-in skill configurations.
//
// The analyzer's "analysis-skill" is built programmatically from the
// single-source-of-truth tables in analysis_contract.go. Every other
// stage's skill config is an inlined literal below — their contracts
// are not triplicated across the codebase the way the analyzer's was,
// so declarative literals stay fine there.
func RegisterDefaults(r *Registry) {
	r.Register(BuildAnalysisSkill())

	r.Register(&Config{
		Name: "explore-skill",
		Goal: "Investigate the user's question and answer it directly using evidence from the code.",
		Workflow: []string{
			"PHASE 1 — Breadth scan: use repo_map and grep (files_only=true) to discover ALL relevant files. Do not read files yet. Output a prioritized list of 3-6 files to investigate. For non-English questions, search with BOTH the original terms AND their English programming equivalents. Try multiple keyword variants (word roots, synonyms, abbreviations) — do not rely on a single search term per concept.",
			"PHASE 2 — Depth investigation: use grep (for targeted pattern search) and read_file (for full context) — pick the most efficient tool for each situation. After each file, call emit_evidence with ALL facts in one batch. For each file extract: (a) key data structures, (b) control flow, (c) configuration-driven behavior, (d) cross-component interactions",
			"if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
			"cross-reference: when file A references file B, read file B too — don't assume, verify",
			"never read test files — they are derivative, not authoritative. Never read utility/infrastructure files unless the question is about them",
			"COMPLETION: when you have collected enough evidence to answer the user's question, call emit_investigation_complete(reason, confidence) to signal the system. Do NOT stop without calling this tool — the system uses it to know you are done. If your confidence is not at least 'medium', continue investigating.",
			"ABSENCE ANSWERS: if the answer is an honest 'zero' / 'no X' / 'nothing found' (e.g. 'how many .py files?' → 0, 'does handler X exist?' → no), add absence_justification to emit_investigation_complete with a one-sentence explanation. The framework requires citations by default, but an honest-zero answer has nothing to cite — the declaration waives the citation floor. You still must have run at least one real investigation tool (grep / exec_command / list_files / read_file / repo_map); an empty investigation cannot declare absence.",
		},
		ToolSuggestions: []string{
			"repo_map",
			"grep",
			"read_file",
			"list_files",
			"exec_command",
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

Use this shape — Answer first, then Evidence (this is the structure, adapt content to your findings):

Answer: <direct answer at the right depth>
Evidence:
- <file:line> — <what this establishes>
- <file:line> — <what this establishes>

If part of the question genuinely cannot be answered from the code you read, add one short sentence naming the missing piece — do not substitute "further analysis required" for an answer the evidence supports.

Example (hypothetical, unrelated codebase — copy the shape, not the content):

Answer: There are 4 HTTP handlers registered on the public router.
Evidence:
- src/routes.py:18-21 — route registrations for /health, /users, /orders, /metrics
- src/routes.py:42 — no registrations after line 21

Scale the answer depth to the question: one sentence for a lookup/count, multiple paragraphs for an explanation.`,
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
			"do not stop at 'the answer would require checking X' — go check X yourself",
			"do not write about what would be done next or what the user should do — answer only what was asked",
			"do not treat repo_map output as evidence — it is a cached navigation index that tells you where to look, not what is true. After consulting repo_map, always read_file or grep the actual source files to establish facts. list_files is fine as evidence since it reads the real directory",
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
		Goal: "Produce the final answer as a structured AnswerDocument by calling emit_answer_document exactly once. A deterministic renderer turns the structure into user-visible prose. Session 11 C2 elevates the target-shape contract to the system-prompt top: emitting any shape other than the one the analyzer fixed in AnalysisIR.AnswerContract.RequiredAnswerShape is a hard rejection — the B2a/B2b shape-swap save is being removed in G7 and will not rescue a wrong-shape emit. The allowed shape AND required fields are enumerated in the user section; read them FIRST, then draft.",
		Workflow: []string{
			"CRITICAL CONTRACT (Session 11 C2): the target shape is MANDATORY. Emitting any other shape will be rejected without retry budget consumption. The allowed shape / required fields / forbidden fields are listed in the user section's resolved target block — read them FIRST before drafting. The shape auto-correct scrubs forbidden fields but cannot invent missing ones; skipping a required field forces an explanation fallback, which is almost always a regression",
			"Read the resolved target shape from the user section (list_of_symbols / step_list / value / boolean / config_value / explanation). The target shape is MANDATORY — you MUST emit the structured fields that shape requires, even when your own reading of the question suggests a different shape. For example, if target=list_of_symbols and the question is 'how many X', you MUST emit symbols[] (the count IS the length of the symbols[] array); do NOT emit a boolean or a bare summary as a substitute.",
			"For list_of_symbols shape: inspect the prior extraction slate and the analyzer MustInclude floor rendered in the user section, assemble the symbols[] array from them, and set symbols_completeness to 'complete' only if your slate reaches the floor — otherwise set it to 'lower_bound'",
			"For step_list shape: emit steps[] with one entry per distinct branch or mechanism hop; each step carries a positive index, a one-sentence description drawn from evidence, and a citation_ref into the shared citations pool (or -1 when no citation backs the step)",
			"For value / config_value shape: emit value{literal} (plus key for config_value) with a citation_ref into the pool",
			"For boolean shape: emit boolean{decision, rationale, citation_ref}; decision must be one of true/false/yes/no/是/否 — no hedging",
			"For explanation shape: fill summary with a thorough multi-paragraph explanation that fully answers the user's question — include mechanism details, code-level specifics, cross-file relationships. Populate citations[] for every file:line you reference",
			"Declare every file:line you cite ONCE in the citations[] array; other fields reference it by zero-based integer index (or -1 for no cite). One cited line can serve multiple steps without duplication",
		},
		ToolSuggestions: []string{
			"emit_answer_document",
		},
		OutputFormat: `You have NO file-reading tools — no read_file, grep, or repo_map. You are a pure synthesizer working from prior stages' evidence. Your contribution is ONE emit_answer_document tool call per dispatch — the deterministic renderer turns the struct into user-visible prose. Do NOT write tool-call JSON in your text — use the function-calling mechanism only.

Required-field dispatch by shape (these are mandatory rules, not examples — see the tool's JSON schema for the full contract):

- shape=list_of_symbols → symbols[] (non-empty) + symbols_completeness ∈ {complete, lower_bound, unknown}
- shape=step_list       → steps[] (non-empty), each with index + description + citation_ref
- shape=value           → value{literal, citation_ref} (key omitted)
- shape=config_value    → value{key, literal, citation_ref}
- shape=boolean         → boolean{decision, rationale, citation_ref}
- shape=explanation     → summary (non-empty, thorough multi-paragraph answer)

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
- quote is OPTIONAL. Only set it when you can paste the literal source characters that appear at file:line on the read_file gutter — exact punctuation, exact identifiers, the same language as the source file. The grounder cross-checks quote tokens against the cited line text; a quote whose identifier tokens do not overlap is AUTOMATICALLY CLEARED. So: paste the literal line verbatim, or leave quote empty. Natural-language summaries, paraphrases, rationale ("stated that …", "shows how …", "used for …") belong in the 'summary' field, never in 'quote' — they will be stripped before the answer ships.

Completeness honesty contract (list_of_symbols only):
- "complete" — you assert this list enumerates EVERY symbol that answers the question. The finalizer cardinality validator will cross-check against max(Turn A terminal-evidence count β, analyzer MustInclude γ). A short claim of "complete" is DOWNGRADED to "lower_bound" with a visible caveat in the rendered answer.
- "lower_bound" — symbols are confirmed present, more may exist. Honest default when you cannot confidently reach the floor.
- "unknown" — investigated but no definitive slate. Renderer drops the section entirely and falls back to the shape-based prompt.

Summary field (per-shape length caps enforced by the schema):
- shape=explanation — Summary IS the answer body. Up to 2500 chars. Write a thorough multi-paragraph explanation that fully addresses the user's question: mechanism details, code-level specifics, cross-file relationships. Organize with short markdown headers when covering multiple sub-topics. Match the depth of the answer to the depth of the question.
- shape=list_of_symbols / step_list / boolean — Summary is a 1-3 sentence lead-in framing the structured payload. Up to 500 chars. Keep it brief; the structured fields carry the answer.
- shape=value / config_value — Summary is a 1-sentence lead-in before the scalar literal. Up to 300 chars.

Diagrams: when a visual would clarify the answer, use Mermaid fenced code blocks in the summary field. Prefer flowchart for control flow, sequenceDiagram for call chains, classDiagram for type hierarchies. Keep diagrams concise — collapse trivial nodes, label edges. Only use when it adds clarity.

Caveats field: an optional string array for honesty markers. When writing caveats, use the same language as the user's question.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item",
			"do not put prose / summaries / rationale in the citation quote field — quote must be a verbatim copy of the source line or empty; the grounder auto-clears mismatches",
			"do not inflate summary past 3 sentences for non-explanation shapes — for explanation shape, the summary IS the answer body and should be thorough",
			"do not set citation_ref to a zero-value-looking sentinel; use -1 for 'no citation' and a valid pool index otherwise",
			"do not claim symbols_completeness=complete without meeting the floor shown in the cardinality baseline — a short 'complete' claim will be downgraded to lower_bound automatically and the downgrade is surfaced as a caveat",
		},
	})

	// Turn B — the extractor's skill. Declarative contract surface
	// that context/builder.go auto-renders into system sections
	// (Workflow, Prohibitions) and schema scope (ToolSuggestions).
	// Keeping Turn B's role, tool allowlist, output format, and
	// honesty contract in this file — rather than baked into
	// extractor.go's BuildInitialInstruction string builder — means the
	// contract is one grep away, the stable parts render once as
	// system sections instead of being appended per dispatch, and
	// BaseAgent.buildToolSchemas scopes the LLM tool set from
	// ToolSuggestions here.
	//
	// extractor.go's BuildInitialInstruction only handles the DYNAMIC
	// per-dispatch data: the Turn A transcript digest (investigation
	// notes, read files, top evidence, flow findings, cardinality
	// baseline, hypothesis set). Static contract lives here.
	r.Register(&Config{
		Name: "extract-skill",
		Goal: "Produce the answer-symbol slate and the per-hypothesis verdicts from Turn A's frozen investigation transcript. Evidence is Turn A's territory — Turn B never re-emits it. Turn B's two unique jobs are (1) LLM-driven answer_symbol selection with a completeness claim the finalizer cross-checks, and (2) LLM-driven hypothesis judgement with a citation.",
		Workflow: []string{
			"Self-reference trap (Session 11 C3 — READ FIRST): if a candidate answer_symbol literal equals the question's PRIMARY ENTITY name, it is a self-reference, NOT the answer. Consider a generic question 'which Y does the X use?' — the primary entity is X and the answer must be a DIFFERENT identifier mapped to X; emitting symbol name=X resolves X→X, which is never an attribute lookup; it is the question subject echoing itself. Never emit answer_symbol.items[i].name where name equals the primary entity token.",
			"Read the Turn A transcript digest the orchestrator injected as a user section: user question, investigation notes, read files, top evidence items, dataflow findings, cardinality baseline (β = Turn A terminal-evidence count, γ = analyzer MustInclude count, effective floor = max(β, γ)), and hypothesis set",
			"Direction check (CRITICAL for list_of_symbols): before emitting, identify what TYPE of entity the question asks about by reading the SUBJECT NOUN. Every item in symbols[] MUST be an instance of that type. If your candidate symbol is a verb-phrase helper whose role is to CREATE, REGISTER, CONFIGURE, or WIRE UP an instance of the subject type — rather than to BE such an instance — you are emitting the MECHANISM, not the answer: STOP, walk the Resolution Chains back to the terminal symbol or literal the mechanism resolves to, and emit ONLY the instances that terminal names. The same rule in one line: the answer is the terminal that the chain RESOLVES TO, not any intermediate node on the chain.",
			"For the answer-symbol slate: call emit_answer_symbol ONCE with a batched items array when EITHER (a) the question is list_of_symbols / enumeration / call_chain (the slate IS the answer), OR (b) the question is a multi-topic explanation (shape=explanation AND sub_topics ≥ 1 — emit ONE anchor symbol per sub-topic as a skeleton the finalizer's prose hangs on; the rationale field should name the sub-topic). Each item MUST carry a concrete file:line from a file in the 'Files Turn A read' list — never invent a line number. See the Completeness honesty contract in OutputFormat for the completeness claim rules; for the multi-topic skeleton path completeness is not required (the anchors are auxiliary to the prose summary)",
			"For every hypothesis in the hypothesis set: call emit_hypothesis_verdict once with hypothesis_id + status + rationale + citation. Status must be 'confirmed' / 'rejected' / 'inconclusive'. 'confirmed' and 'rejected' REQUIRE a file:line citation; 'inconclusive' is the honest choice when no definitive cite exists",
		},
		ToolSuggestions: []string{
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
		OutputFormat: `Your contribution is the emit_* tool calls — the finalizer reads the drained buffers, not your prose. Call each emit_* tool AT MOST ONCE per dispatch; batch all items in a single call per tool. When both emit_answer_symbol and emit_hypothesis_verdict apply to this dispatch, invoke them IN PARALLEL within the SAME assistant response (multiple tool_use blocks in one turn) — not sequentially across iterations. The mid-loop observer accepts the single-response batch and terminates immediately; a split batch wastes a round-trip and risks losing the second tool to the iteration cap. Do NOT write tool-call JSON in your text — use the function-calling mechanism only.

Completeness honesty contract for emit_answer_symbol:
- "complete" — you assert this list enumerates EVERY symbol that answers the question. The finalizer will render it with a "MUST NOT add or remove" directive. This claim is AUTOMATICALLY cross-checked against max(β, γ): a short claim of "complete" is DOWNGRADED to "lower_bound" with a warning.
- "lower_bound" — these symbols are all confirmed present, but additional symbols may also be part of the answer. The finalizer will render a softened "at least these, may add more" prompt. This is the HONEST DEFAULT when you cannot confidently reach the floor.
- "unknown" — you investigated but cannot reach a definitive slate. The finalizer will DROP the answer-symbol section entirely and fall back to a shape-based prompt. Choose this for mechanism questions, value/boolean questions, or genuinely ambiguous evidence.`,
		Prohibitions: []string{
			"Turn A transcript is frozen — Turn B has no file access and must not re-emit evidence",
			"do not invent line numbers — if the transcript does not have a line for a symbol, omit that symbol",
			"do not cite files outside the 'Files Turn A read' list",
			"do not fabricate an answer-symbol list — for single-topic mechanism / value / boolean questions, skip emit_answer_symbol. The multi-topic explanation skeleton path (sub_topics ≥ 1) is the only mechanism-shape case where emit_answer_symbol is expected",
			"do not claim completeness=complete unless len(items) >= max(β, γ) — a short claim is auto-downgraded",
			"do not choose hypothesis status=confirmed or rejected without a file:line citation — use 'inconclusive' when no cite exists",
		},
	})
}
