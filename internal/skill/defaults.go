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
		Goal: "Investigate the user's question by reading code, collecting grounded evidence, and handing a well-supported evidence record to downstream synthesis.",
		Workflow: []string{
			"PHASE 1 — Breadth scan: use repo_map and grep (files_only=true) to discover ALL relevant files. Do not read files yet. Output a prioritized list of 3-6 files to investigate. For non-English questions, search with BOTH the original terms AND their English programming equivalents. Try multiple keyword variants (word roots, synonyms, abbreviations) — do not rely on a single search term per concept.",
			"PHASE 2 — Depth investigation: use grep (for targeted pattern search) and read_file (for full context) — pick the most efficient tool for each situation. After each file, call emit_evidence with ALL facts in one batch. For each file extract: (a) key data structures, (b) control flow, (c) configuration-driven behavior, (d) cross-component interactions",
			"if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
			"cross-reference: when file A references file B, read file B too — don't assume, verify",
			"never read test files — they are derivative, not authoritative. Never read utility/infrastructure files unless the question is about them",
			"if the request attached a runtime log (panic / exception trace / sanitizer diagnostic / traceback), the stack-frame file:line pairs are already your RequiredFiles — open them FIRST before widening the search. Read from the innermost (deepest) frame outward so the actual error site is the first fact you establish",
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
		// The explorer is not the final answer writer anymore. The useful
		// output at this stage is tool use plus grounded evidence. Keep the
		// contract explicit here so the LLM does not fall back to generic
		// "Answer:/Evidence:" prose that pollutes later heuristics.
		OutputFormat: `Your valuable output in this stage is tool use plus structured evidence, not polished answer prose.

Preferred behavior per turn:
- If you know the next file or search to run, call the tool directly instead of drafting an answer.
- Optional assistant text between tool calls should be 1-3 short working notes about what the last read established.
- Do NOT use narrative labels such as "Answer:", "Evidence:", "Summary:", or "Caveat:" in those notes.
- After reading a file, call emit_evidence(items=[...]) with the grounded facts you learned from that file.
- When you truly have enough evidence, call emit_investigation_complete(reason, confidence). That tool call — not your prose — is the completion signal.

Working-note examples (illustrative shape only — use generic abstract names, not literal identifiers from the code under investigation):
- "fnA" first checks a cached state, then falls back to a fresh computation when the cache is stale.
- "fnB" reprocesses only the changed inputs and merges them into the cached state.

Keep any prose brief and operational; save the final user-facing answer for later stages.`,
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
			"do not stop at 'the answer would require checking X' — go check X yourself",
			"do not write about what would be done next or what the user should do — answer only what was asked",
			"do not ask the user whether to continue investigating or what area to inspect next — decide from the evidence and use tools",
			"do not write `Answer:` / `Evidence:` / `Summary:` headings during exploration",
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
		Goal: "Produce the final answer as a structured AnswerDocument by calling emit_answer_document exactly once. A deterministic renderer turns the structure into user-visible prose. The target-shape contract is mandatory: emitting any shape other than the one declared in the user section is a hard rejection. The allowed shape AND required fields are enumerated in the user section; read them FIRST, then draft.",
		Workflow: []string{
			"Write the answer DIRECTLY into the `emit_answer_document` tool call from the start. Do not compose the answer as a plain prose paragraph first and then call the tool — compose the final text inside the `summary` field (and the shape-specific structured fields) as you think. The tool call is the only delivery surface; text outside it does not ship.",
			"CRITICAL CONTRACT: the target shape is MANDATORY. Emitting any other shape will be rejected. The allowed shape / required fields / forbidden fields are listed in the user section's resolved target block — read them FIRST before drafting. Skipping a required field forces an explanation fallback, which is almost always a regression.",
			"Read the resolved target shape from the user section (list_of_symbols / step_list / value / boolean / config_value / explanation). The target shape is MANDATORY — you MUST emit the structured fields that shape requires, even when your own reading of the question suggests a different shape. For example, if target=list_of_symbols and the question is 'how many X', you MUST emit symbols[] (the count IS the length of the symbols[] array); do NOT emit a boolean or a bare summary as a substitute.",
			"For list_of_symbols shape: inspect the prior extraction slate and the analyzer MustInclude floor rendered in the user section, assemble the symbols[] array from them, and set symbols_completeness to 'complete' only if your slate reaches the floor — otherwise set it to 'lower_bound'",
			"For step_list shape: emit steps[] with one entry per distinct branch or mechanism hop; each step carries a positive index, a clear description drawn from evidence, and a citation_ref into the shared citations pool (or -1 when no citation backs the step). Accuracy and clarity come first — describe what the step actually does with the precision the reader needs, do NOT artificially compress to a single sentence. When the step topology is non-linear (branches, parallel, DAG, fan-out), include the appropriate ASCII diagram inside `summary` yourself: the deterministic renderer emits ONLY the numbered detailed list, it does NOT auto-generate any flow diagram. A step that paraphrases an attached-log frame (external source) should set citation_ref=-1 — the literal-grounding gate rejects a citation whose cited line shares no identifier with the description.",
			"For value / config_value shape: emit value{literal} (plus key for config_value) with a citation_ref into the pool. Use `summary` to disambiguate when the literal alone is ambiguous (two layers that both name it, overloads, lineage from a chain) — the renderer prints the literal and the cite; summary should state what the value MEANS so the reader doesn't need the codebase to interpret it. When the literal is sourced from the attached log / external trace rather than repo code (e.g. a function name that appears in a panic stack but not in the repo), set citation_ref=-1 and state in summary that the answer is derived from log semantics — the literal-grounding gate rejects a citation whose cited line does NOT contain the literal, and -1 is the honest escape.",
			"For boolean shape: emit boolean{decision, rationale, citation_ref}; decision must be one of true/false/yes/no/是/否 — no hedging. The rationale field is where the reasoning lives — treat it as the heart of the answer and explain the mechanism at whatever depth the subtlety requires; a terse rationale on a subtle question is a regression. If the rationale draws on log semantics or external sources rather than repo code, set citation_ref=-1 — the literal-grounding gate rejects a cite whose line shares no identifier with the rationale.",
			"For list_of_symbols shape: every symbols[i].file/line must be a real repo anchor where the symbol's name actually appears. If the identifier comes from an attached log / external trace (e.g. a panic frame naming a function the repo does not define), DO NOT invent a file:line — either drop the item or set symbols_completeness='unknown' — the literal-grounding gate rejects a symbol whose cited line shares no identifier token with symbols[i].name.",
			"For explanation shape: fill summary with a thorough multi-paragraph explanation. Structure with ### sub-headers for each major topic/stage. Open with a plain-prose lead paragraph that states the core conclusion up front — write it as the first paragraph of the summary body, NOT as a `### TL;DR` (or similarly-named) section header, and do NOT prefix it with the literal string `TL;DR:` / `TLDR:`. Length of the lead paragraph matches what the conclusion requires (one sentence if it fits; several sentences if context matters). Include mechanism details with inline `code` references, cross-file relationships, and at least one ASCII diagram whenever the answer describes control flow, dispatch, timing between actors, architecture, or a relationship/fan-out graph — pick the shape that communicates best: (a) flow/decision diagram for branches and guards (boxes plus `──▶`, `─┬─`, `├─▶`, `└─▶`); (b) sequence diagram for actor-to-actor calls over time (vertical lanes with `│`, horizontal arrows `──▶` / `◀──` labelled with the call name); (c) architecture diagram for layered components (stacked boxes separated by `│` / `└───┘` with labelled dependency edges); (d) call-DAG diagram for one-to-many dispatch or fan-out (a root box with `├─▶` / `└─▶` fan-out to callees; do NOT arbitrarily cap depth if the answer's real fan-out has more levels). Always wrap diagrams in triple-backtick fences so monospace alignment survives, and place the diagram immediately after the lead paragraph. Populate citations[] for every file:line you reference.",
			"Declare every file:line you cite ONCE in the citations[] array; other fields reference it by zero-based integer index (or -1 for no cite). One cited line can serve multiple steps without duplication",
			"For log-triage questions (the user attached a panic / exception / sanitizer diagnostic / traceback) prefer shape=step_list with each step pointing at one stack frame or one code-level cause; start from the innermost frame and work outward so the reader sees the failure site first",
			"Diagram-grounding contract: when your summary contains a fenced code block (ASCII call-chain / sequence / architecture / flow diagram), every file name you write inside that block — e.g. `foo.go`, `bar/baz.py:42` — MUST appear in citations[] or in the Log Triage section's resolved frames. A filename inside a diagram is a concrete structural claim, not narrative prose, and the system-level diagram-grounding gate will reject the emit when it names files you did not observe. When the attached log's Log Triage section renders a \"Call chain (innermost → outer)\" block, draw any call-chain diagram in summary from those frames verbatim — do NOT introduce caller/callee files that are not in the frames. For cross-file callers you could not directly observe, describe the relationship in prose rather than inventing a diagram node.",
			"Log-triage coverage contract: when an attached log produces a structured Errors tree (top-level error Type plus optional Cause recursion chain), your summary MUST name every Type at least once using the exact class / exception identifier that appears in the Log Triage section. The rule applies across every shape and every signal family. A single-level log requires naming its one Type; a multi-level Cause chain requires naming each link. Paraphrasing the chain as 'a cascade of errors' without naming the individual Types is not sufficient — the deterministic validator rejects summaries that share zero tokens with a given Type's identifiers (case-insensitive). Do NOT replace the real Types with domain-unrelated descriptions or invent alternative stack frames when the log's frames are the ground truth.",
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
- "complete" — you assert this list enumerates EVERY symbol that answers the question. A cardinality cross-check runs against the larger of the investigation's terminal-evidence count and the analyzer's must-include count. A short claim of "complete" is DOWNGRADED to "lower_bound" with a visible caveat in the rendered answer.
- "lower_bound" — symbols are confirmed present, more may exist. Honest default when you cannot confidently reach the floor.
- "unknown" — investigated but no definitive slate. Renderer drops the section entirely and falls back to the shape-based prompt.

Summary field — accuracy and clarity come first. Do NOT self-shorten below what the answer genuinely needs; a terse lead-in on a subtle question is a regression. Any hard length ceiling is applied by the tool at submit time and will be reported back to you as a concrete limit if you exceed it — there is no need to pre-estimate.
- shape=explanation — Summary IS the answer body. Write a thorough multi-paragraph explanation that fully addresses the user's question: mechanism details, code-level specifics, cross-file relationships. Organize with ### sub-headers when covering multiple sub-topics. Depth matches the question's depth — a shallow question yields a short answer, a deep one yields a deep answer.
- shape=step_list — Summary frames the overall mechanism the numbered steps implement. State the starting point, the terminating effect, and (when the step topology is non-linear — branches, parallel, DAG, fan-out) embed the appropriate ASCII diagram here. The renderer emits only the numbered detailed list; any visual representation of step topology is YOUR responsibility in the summary text.
- shape=list_of_symbols — Summary describes what the list enumerates and the terminal criterion used to pick the symbols, so the reader understands what kind of item each row is and why these rows belong (and none of the others). A bare one-liner works only when the list is self-explanatory.
- shape=boolean — Summary sets up the decision. The core reasoning belongs in the boolean.rationale field — explain the mechanism at the depth the subtlety requires. A subtle boolean with a one-sentence rationale is a regression.
- shape=value / config_value — Summary disambiguates and contextualizes the literal. State what the value means, where it comes from in the call graph / config lineage, and why the cited file:line is the authoritative source. The renderer prints the literal and the cite; summary is where the MEANING lives.

Visual structure (IMPORTANT — users need to understand logic at a glance):
- For explanation shape: organize summary with ### sub-headers when covering multiple topics or stages. Open with a plain-prose lead paragraph that states the core conclusion up front — write it as the first paragraph of the summary body, NOT as a ### TL;DR (or similarly-named) section header, and do NOT prefix it with the literal string "TL;DR:" / "TLDR:". Length is governed by what the conclusion actually requires to be complete, not by a pre-set budget.
- Diagrams are EXPECTED (not optional) whenever the answer involves control flow, dispatch, timing, architecture, or fan-out. Pick the diagram kind that matches the question:
    - Flow/decision diagram — branches and guards. Boxes plus horizontal arrows ──▶ with ─┬─ / ├─▶ / └─▶ to split into child edges.
    - Sequence diagram — actor-to-actor calls over time. Vertical lanes drawn with │, horizontal arrows ──▶ / ◀── labelled with the call name, ordered top-to-bottom.
    - Architecture diagram — layered components. Stacked boxes separated by │ and └───┘ with labelled edges showing dependencies.
    - Call-DAG diagram — one-to-many dispatch or fan-out. A root box with ├─▶ / └─▶ fan-out to callees; grow to as many levels as the real fan-out requires — do not arbitrarily truncate.
  Always wrap diagrams in triple-backtick fences so monospace alignment survives the terminal/markdown pipeline. Place the diagram near the top of the summary (immediately after the lead paragraph) so readers see structure before prose. Plain explanatory questions without flow/dispatch/architecture content do not need a diagram.
- For step_list shape: the renderer emits ONLY a numbered detailed list of steps — it does NOT draw any flow / DAG / sequence diagram for you. When the step topology is non-linear (branches, parallel, DAG, fan-out) YOU must draw the diagram inside the summary text using the same diagram palette above. A linear 3-step chain can skip the diagram; a branching / fan-out / DAG cannot.
- Use markdown tables when comparing attributes across entities (e.g. "| Entity | Attribute | Value |") — keep column headers abstract; do not copy the question's literal entities or predicates into the headers.
- Use inline code backticks for every function name, type name, and file path — never write them as plain text.

Caveats field: an optional string array for honesty markers. When writing caveats, use the same language as the user's question.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item",
			"do not put prose / summaries / rationale in the citation quote field — quote must be a verbatim copy of the source line or empty; the grounder auto-clears mismatches",
			"do not pre-shrink summary for any shape — write what accuracy and clarity require; if a hard length ceiling is in force the tool will reject the call with the exact limit, so there is no need to pre-estimate",
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
		Goal: "Produce the answer-symbol slate and the per-hypothesis verdicts from the investigation's frozen transcript. Evidence belongs to the investigation stage — this extraction stage never re-emits it. The extraction stage has two unique jobs: (1) LLM-driven answer_symbol selection with a completeness claim that is cross-checked downstream, and (2) LLM-driven hypothesis judgement with a citation.",
		Workflow: []string{
			"Self-reference trap (READ FIRST): if a candidate answer_symbol literal equals the question's PRIMARY ENTITY name, it is a self-reference, NOT the answer. Consider a generic question 'which Y does the X use?' — the primary entity is X and the answer must be a DIFFERENT identifier mapped to X; emitting symbol name=X resolves X→X, which is never an attribute lookup; it is the question subject echoing itself. Never emit answer_symbol.items[i].name where name equals the primary entity token.",
			"Read the investigation transcript digest the orchestrator injected as a user section: user question, investigation notes, read files, top evidence items, dataflow findings, cardinality baseline (the investigation's terminal-evidence count, the analyzer's must-include count, and the effective floor = the larger of the two), and hypothesis set",
			"Direction check (CRITICAL for list_of_symbols): before emitting, identify what TYPE of entity the question asks about by reading the SUBJECT NOUN. Every item in symbols[] MUST be an instance of that type. If your candidate symbol is a verb-phrase helper whose role is to CREATE, REGISTER, CONFIGURE, or WIRE UP an instance of the subject type — rather than to BE such an instance — you are emitting the MECHANISM, not the answer: STOP, walk the Resolution Chains back to the terminal symbol or literal the mechanism resolves to, and emit ONLY the instances that terminal names. The same rule in one line: the answer is the terminal that the chain RESOLVES TO, not any intermediate node on the chain.",
			"For the answer-symbol slate: call emit_answer_symbol ONCE with a batched items array when EITHER (a) the question is list_of_symbols / enumeration / call_chain (the slate IS the answer), OR (b) the question is a multi-topic explanation (shape=explanation AND sub_topics ≥ 1 — emit ONE anchor symbol per sub-topic as a skeleton the final answer's prose hangs on; the rationale field should name the sub-topic). Each item MUST carry a concrete file:line from a file in the investigation's read-files list — never invent a line number. See the Completeness honesty contract in OutputFormat for the completeness claim rules; for the multi-topic skeleton path completeness is not required (the anchors are auxiliary to the prose summary)",
			"For every hypothesis in the hypothesis set: call emit_hypothesis_verdict once with hypothesis_id + status + rationale + citation. Status must be 'confirmed' / 'rejected' / 'inconclusive'. 'confirmed' and 'rejected' REQUIRE a file:line citation; 'inconclusive' is the honest choice when no definitive cite exists",
			"Log-triage grounding: when the Log Triage section renders a \"Call chain (innermost → outer)\" block or per-frame file:line anchors, the frames ARE the authoritative file set for the failure call path. emit_answer_symbol items for log-triage questions MUST come from those frames or from investigation evidence that directly cites them — do NOT pull symbols from files the keyword ranker flagged as \"Auxiliary candidates\" unless the investigation produced an evidence item anchored there. This feeds the finalizer's diagram-grounding gate, which rejects a call-chain diagram that names files the frames + citations did not observe.",
			"External-source log escape: when the Log Triage section carries the '⚠ External-source log' banner (resolved_files=0 because the attached log's frames did not resolve to any repo file), the emit_answer_symbol channel cannot be satisfied — every item requires a concrete repo-grounded file:line, which external-log identifiers do not have. Set symbols_completeness=\"unknown\" and OMIT items[] entirely; the summary prose in the downstream finalizer carries the answer. Do NOT manufacture file:line anchors for log-message keys or exception names that have no repo counterpart — the tool rejects line=0 or empty-file items and will redirect to this escape on every retry until you take it.",
		},
		ToolSuggestions: []string{
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
		OutputFormat: `Your contribution is the emit_* tool calls — downstream rendering reads the drained buffers, not your prose. Call each emit_* tool AT MOST ONCE per dispatch; batch all items in a single call per tool. When both emit_answer_symbol and emit_hypothesis_verdict apply to this dispatch, invoke them IN PARALLEL within the SAME assistant response (multiple tool_use blocks in one turn) — not sequentially across iterations. The mid-loop observer accepts the single-response batch and terminates immediately; a split batch wastes a round-trip and risks losing the second tool to the iteration cap. Do NOT write tool-call JSON in your text — use the function-calling mechanism only.

Completeness honesty contract for emit_answer_symbol:
- "complete" — you assert this list enumerates EVERY symbol that answers the question. Downstream rendering will present it with a "MUST NOT add or remove" directive. This claim is AUTOMATICALLY cross-checked against the effective floor (the larger of the investigation's terminal-evidence count and the analyzer's must-include count): a short claim of "complete" is DOWNGRADED to "lower_bound" with a warning.
- "lower_bound" — these symbols are all confirmed present, but additional symbols may also be part of the answer. Downstream rendering will present a softened "at least these, may add more" prompt. This is the HONEST DEFAULT when you cannot confidently reach the floor.
- "unknown" — you investigated but cannot reach a definitive slate. The answer-symbol section is DROPPED entirely and downstream rendering falls back to a shape-based prompt. Choose this for mechanism questions, value/boolean questions, or genuinely ambiguous evidence.`,
		Prohibitions: []string{
			"the investigation transcript is frozen — this extraction stage has no file access and must not re-emit evidence",
			"do not invent line numbers — if the transcript does not have a line for a symbol, omit that symbol",
			"do not cite files outside the investigation's read-files list",
			"do not fabricate an answer-symbol list — for single-topic mechanism / value / boolean questions, skip emit_answer_symbol. The multi-topic explanation skeleton path (sub_topics ≥ 1) is the only mechanism-shape case where emit_answer_symbol is expected",
			"do not claim completeness=complete unless len(items) >= the effective floor — a short claim is auto-downgraded",
			"do not choose hypothesis status=confirmed or rejected without a file:line citation — use 'inconclusive' when no cite exists",
		},
	})

	// The log_triager skill. The agent reads the user-attached runtime
	// log and emits ONE emit_log_triage call with a structured view:
	// layer 1 (Meta — lang, signals, summary), layer 2 (Errors — type,
	// message, frames, optional recursive Cause chain), layer 3
	// (Residue — unknown_chunks). The system validates paths against
	// the repository and derives layer 4 (resolved_files, entities,
	// intent_hint, coverage) automatically — the LLM cannot fill
	// layer 4 because those fields are not in the tool's JSON schema.
	//
	// Tool allowlist is intentionally narrow: read_file only, for
	// paginating large log blobs. No repo_map / list_files / grep —
	// the LLM does not attempt path resolution; the system does that
	// deterministically post-emit.
	r.Register(&Config{
		Name: "log-triage-skill",
		Goal: "Read the attached runtime log and emit a structured triage bundle (errors + frames + signals + residue) via emit_log_triage exactly once. The system validates paths and derives the resolved-files list automatically — focus on extraction, not resolution.",
		Workflow: []string{
			"Read the attached runtime log from the 'Attached Runtime Log' section of the user prompt. If the log was oversized and blobbed to attached_log.txt, use read_file with offset/limit to paginate through the middle — the head+tail preview is already visible inline.",
			"Identify every stack frame that names a source file with a line number. For each frame, emit: file (as it appears in the log, do NOT normalize paths — the system does this), line, func (best available identifier), pkg (module/namespace hint when obvious), lang, raw (the original log line for this frame, required), confidence (0.0-1.0, your certainty the frame is real).",
			"Group frames under the error they belong to. Emit errors[] with one entry per logical error (per goroutine in a Go panic dump, per exception in a multi-exception traceback). For each error set type (exception class / panic type), message (the human-readable text), and frames[] (the stack for THIS error).",
			"Chain causal errors via the cause pointer. When the log shows 'Caused by:' (Java), 'during handling of' (Python __cause__ / __context__), or '#[source]' (Rust), nest the upstream error in the cause field. Keep the chain shallow — practical depth 3 or so; the system caps at 5.",
			"Set meta.lang to the dominant language (go/java/cpp/python/node/rust/ruby/csharp/unknown/other). Set meta.signals from the canonical enum (panic/crash/oom/timeout/permission/db/network/validation/logic/other) by matching the observed symptoms. Multiple signals are OK when the log describes compound failures. Summary is an optional one-line synopsis.",
			"Log chunks that do NOT structure into an error (build noise, log-level prefixes, unrelated debug output, truncation markers) go into unknown_chunks. Do not punt everything there — only genuinely unparseable pieces. Each chunk capped at 500 chars, at most 8 chunks total.",
			"Frames with uncertain line numbers or uncertain file paths should be emitted with line=0 or file='' and confidence < 0.5 — the system will keep the frame in the bundle but will NOT add it to the repo file list. Zero information loss on partials.",
		},
		ToolSuggestions: []string{
			"read_file", // blob pagination only
			"emit_log_triage",
		},
		OutputFormat: `You have ONE emit tool: emit_log_triage. You have ONE read tool: read_file, used SOLELY for paginating the attached log blob when it was too large to inline. You do NOT have grep / repo_map / list_files — path resolution is system work, not yours.

Schema in one glance:
- meta.lang        (required) — the dominant runtime language
- meta.signals[]   (required, may be empty) — what-went-wrong enum values
- meta.summary     (optional) — one-line synopsis, ≤ 200 chars
- errors[]         (required, may be empty) — array of { type, message?, frames[], cause? }
- errors[].frames[] — { lang?, file?, line?, func?, pkg?, raw (required), confidence (required) }
- errors[].cause   — recursive error (same shape); for linear causal chains like Java Caused-by
- unknown_chunks[] (optional) — ≤ 8 strings ≤ 500 chars each, for unstructurable text

What the system does with your output:
- Strips build-machine prefixes (/build/*, /home/user/src/*) from file paths
- Resolves Java basename frames via repo glob
- Drops frames whose file does not exist inside the repository (hallucination filter)
- Filters out runtime-internal frames (Go stdlib, node: URIs, java.base/*)
- Derives resolved_files, entities, intent_hint, and coverage from your emission
- Caps entities at 32, resolved_files at 10, cause depth at 5

You emit exactly one emit_log_triage call per dispatch. Do NOT write prose — the validated bundle is the deliverable.`,
		Prohibitions: []string{
			"do NOT resolve file paths yourself — emit frames with the file path AS IT APPEARS in the log; the system does StripBuildPathPrefix, Java basename glob, os.Stat, and runtime-internal filtering",
			"do NOT use grep / list_files / repo_map / exec_command — the stage's tool allowlist explicitly excludes them; path verification is system work",
			"do NOT invent frames that are not in the log — a frame with confidence 0.9 must correspond to a real log line whose text you can paste into the raw field",
			"do NOT emit fields outside the documented schema (resolved_files, entities, intent_hint, coverage are LAYER 4 — system-derived, not LLM-emitted); the JSON schema rejects unknown fields",
			"do NOT punt the entire log to unknown_chunks — extract every stack frame you can; unknown_chunks is for genuinely unparseable noise, not a fallback for lazy extraction",
			"do NOT produce multiple emit_log_triage calls — exactly one emit per dispatch; a second call replaces the first",
		},
	})

	// The log segmentation skill for the two-step fallback. Called
	// when emit_log_triage either (a) failed schema validation, (b)
	// returned with coverage below the threshold, or (c) the log
	// exceeds the single-shot size cap. The LLM's only job is to
	// scan the log once and return byte coordinates for the regions
	// that look like stacks / traces / headers. The agent controller
	// then re-dispatches emit_log_triage per stack-shaped segment
	// and merges the partial bundles.
	r.Register(&Config{
		Name: "log-segmentation-skill",
		Goal: "Segment the attached runtime log into byte-addressed regions by kind (stack / caused_by / header / context / trace / noise) so the downstream per-segment extractor can focus on one coherent block at a time. Emit exactly one emit_log_segmentation call.",
		Workflow: []string{
			"Read the attached runtime log. If the log was blobbed, use read_file to scan the full body — segmentation needs coordinates over the complete text.",
			"Walk the log top-to-bottom. Identify byte ranges that contain: a cohesive stack trace (kind=stack); a 'Caused by' / '__cause__' block (kind=caused_by); an error header or panic message line (kind=header); contextual prose around the stack (kind=context); a more general trace segment (kind=trace); or unrelated noise (kind=noise).",
			"Emit at most 10 segments. Overlap is NOT allowed — byte_end of segment N must be ≤ byte_start of segment N+1. Segments must be sorted by byte_start.",
			"Use hint field to add a short (≤80 char) description so the downstream per-segment extractor has a pointer to what it is looking at (e.g. hint='Go goroutine 15 panic' or hint='SQLException Caused-by').",
			"The system validates byte coordinates against the log length. Segments that are reversed, zero-length, or out of bounds are silently dropped.",
		},
		ToolSuggestions: []string{
			"read_file", // for blobbed logs
			"emit_log_segmentation",
		},
		OutputFormat: `Emit ONE emit_log_segmentation call with the full segments[] list.

Schema in one glance:
- segments[] (required, up to 10 entries)
  - byte_start (required, integer ≥ 0)
  - byte_end   (required, integer > byte_start)
  - kind       (required, enum: stack | caused_by | header | context | trace | noise)
  - hint       (optional, ≤ 80 chars)

Do NOT emit any other tool call. Do NOT write prose.`,
		Prohibitions: []string{
			"do not call emit_log_triage — that is Step B, dispatched by the agent after Step A segmentation completes",
			"do not overlap segments — ranges must be disjoint and sorted",
			"do not produce more than 10 segments — coarsen if the log is granular enough to need more",
		},
	})
}
