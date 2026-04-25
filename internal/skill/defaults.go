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
			"PATH SCOPE: when the user's question names an explicit repo-relative path (a file, directory, or package — e.g. `internal/tool` / `cmd/server/main.go` / `pkg/api`), every shell command (find, grep, exec_command, wc) MUST be scoped to that path. Do NOT default to repo-root scope (e.g. `find .` / `wc -l **/*.go`) when the user asked about a specific subtree — the answer's value will be wrong (whole-repo total instead of the requested subtree's total), and the body will fail readability gates that expect the path to appear in the answer. When the question names MULTIPLE paths (e.g. compare A vs B, trace from X to Y), investigate each named path with equivalent depth — equal number of read_file calls on equivalent line ranges, proportional emit_evidence items per path. A two-path comparison answer that emits 80% of its evidence about one side (because that side's file is larger or happens to be listed first) renders as a single-sided answer downstream and fails the comparison; give each path its fair share of the investigation budget.",
			"if the request attached a runtime log (panic / exception trace / sanitizer diagnostic / traceback), the stack-frame file:line pairs are the files you MUST open FIRST before widening the search. Read from the innermost (deepest) frame outward so the actual error site is the first fact you establish",
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
			"do not invent short codename labels (short alphanumeric tokens like `S1` / `F2` / `Stage-N`) in evidence summaries unless the exact token literally appears in the cited line range. These labels are source-level identifiers — verify by reading the anchored lines, do NOT extrapolate from an existing label by pattern (seeing `Fallback S1` in the code does NOT imply `S2`, `S3` exist)",
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
			"For list_of_symbols shape: inspect the prior extraction slate and the required-symbol floor rendered in the user section, and assemble the symbols[] array from them. Each symbol's `rationale` is a short natural-prose line describing the symbol's ROLE in the mechanism — what it does, how it participates in the answer. A rationale like \"the dispatch entry point that maps request kind to handler\" is useful; \"Defined at foo.go:42. Used by bar.\" is a regression because file:line is already rendered as its own column. Set symbols_completeness per the Completeness honesty contract in Output Format.",
			"For step_list shape: emit steps[] with one entry per distinct branch or mechanism hop. Each step's `description` reads as natural explanation — state what the step DOES (the behavior, the guard it checks, the effect it produces for the next hop), reference the load-bearing identifiers with inline `code`, and give the reader enough context to understand why this step matters in the larger mechanism. A description that reads like \"`foo` is called at line 42, which calls `bar` at line 58\" is a regression — it reproduces the call graph but does not explain the mechanism. Use as many sentences as accuracy requires; one step is one logical hop, do not collapse two. Every step carries a positive index and a citation_ref into the shared citations pool (or -1 when no citation backs the step). If the dynamic Diagram Contract says a diagram is required, include the appropriate ASCII diagram inside `summary` yourself — the renderer emits only the numbered list, and linear call chains still count. A step that paraphrases an attached-log frame (external source) should set citation_ref=-1 — a citation whose cited line shares no identifier with the description will be rejected.",
			"For value / config_value shape: emit value{literal} (plus key for config_value) with a citation_ref into the pool. ALWAYS fill `summary` with 1-2 sentences that NAME the subject being measured / queried (file path, symbol name, directory, config key, measurement target — verbatim from the question) AND state how the value was obtained (which command / file / chain produced it). The bare literal alone is rarely a complete answer — readers need to see WHAT was measured and HOW so they can act on or audit the value; an answer that renders as just \"值为 `<literal>`.\" with empty summary is a regression. If the dynamic Diagram Contract says a diagram is required, `summary` must also carry that grounded diagram even for scalar answers. When the literal is sourced from the attached log / external trace rather than repo code (e.g. a function name that appears in a panic stack but not in the repo), set citation_ref=-1 and state in summary that the answer is derived from log semantics — a citation whose cited line does NOT contain the literal will be rejected, and -1 is the honest escape.",
			"For boolean shape: emit boolean{decision, rationale, citation_ref}; decision must be one of true/false/yes/no/是/否 — no hedging. The `rationale` carries the reasoning — name the invariant or guard that forces the answer, reference the load-bearing identifiers with inline `code`, and explain the mechanism at whatever depth the subtlety requires. Structure the rationale as natural prose, not as a pointer to evidence: a rationale that reads \"Line N shows X\" is a regression because the reader cannot open source from here. A terse rationale on a subtle question is also a regression. If the dynamic Diagram Contract says a diagram is required, `summary` must also carry that grounded diagram even for yes/no answers. If the rationale draws on log semantics or external sources rather than repo code, set citation_ref=-1 — a cite whose line shares no identifier with the rationale will be rejected.",
			"For list_of_symbols shape: every symbols[i].file/line must be a real repo anchor where the symbol's name actually appears. If the identifier comes from an attached log / external trace (e.g. a panic frame naming a function the repo does not define), DO NOT invent a file:line — either drop the item or set symbols_completeness='unknown' — a symbol whose cited line shares no identifier token with symbols[i].name will be rejected.",
			"For explanation shape: fill summary with a thorough multi-paragraph explanation. Structure with ### sub-headers for each major topic/stage. Open with a plain-prose lead paragraph that states the core conclusion up front — write it as the first paragraph of the summary body, NOT as a `### TL;DR` (or similarly-named) section header, and do NOT prefix it with the literal string `TL;DR:` / `TLDR:`. Length of the lead paragraph matches what the conclusion requires (one sentence if it fits; several sentences if context matters). Include mechanism details with inline `code` references, cross-file relationships, and at least one ASCII diagram whenever the answer describes control flow, dispatch, timing between actors, architecture, or a relationship/fan-out graph, OR whenever the dynamic Diagram Contract says a diagram is required — pick the shape that communicates best: (a) flow/decision diagram for branches and guards (boxes plus `──▶`, `─┬─`, `├─▶`, `└─▶`); (b) sequence diagram for actor-to-actor calls over time (vertical lanes with `│`, horizontal arrows `──▶` / `◀──` labelled with the call name); (c) architecture diagram for layered components (stacked boxes separated by `│` / `└───┘` with labelled dependency edges); (d) call-DAG diagram for one-to-many dispatch or fan-out (a root box with `├─▶` / `└─▶` fan-out to callees; do NOT arbitrarily cap depth if the answer's real fan-out has more levels). Always wrap diagrams in triple-backtick fences so monospace alignment survives, and place the diagram immediately after the lead paragraph. Populate citations[] for every file:line you reference.",
			"Declare every file:line you cite ONCE in the citations[] array; other fields reference it by zero-based integer index (or -1 for no cite). One cited line can serve multiple steps without duplication",
			"For log-triage questions (the user attached a panic / exception / sanitizer diagnostic / traceback) prefer shape=step_list with each step pointing at one stack frame or one code-level cause; start from the innermost frame and work outward so the reader sees the failure site first",
			"Diagram-grounding contract: when your summary contains a fenced code block (ASCII call-chain / sequence / architecture / flow diagram), every file name you write inside that block — e.g. `foo.go`, `bar/baz.py:42` — MUST appear in citations[] or in the Log Triage section's frames. A filename inside a diagram is a concrete structural claim, not narrative prose, and an emit that names files you did not observe will be rejected. When the attached log's Log Triage section renders a \"Call chain (innermost → outer)\" block, draw any call-chain diagram in summary from those frames verbatim — do NOT introduce caller/callee files that are not in the frames. For cross-file callers you could not directly observe, describe the relationship in prose rather than inventing a diagram node.",
			"Log-triage coverage contract: when an attached log produces a structured Errors tree (top-level error Type plus an optional Caused-by chain), your summary MUST name every Type at least once using the exact class / exception identifier that appears in the Log Triage section. The rule applies across every shape and every signal family. A single-level log requires naming its one Type; a multi-level Caused-by chain requires naming each link. Paraphrasing the chain as 'a cascade of errors' without naming the individual Types is not sufficient — a summary that shares zero tokens with a given Type's identifiers (case-insensitive) will be rejected. Do NOT replace the real Types with domain-unrelated descriptions or invent alternative stack frames when the log's frames are the ground truth.",
			"Subject discipline: summary content must stay relevant to the question's subject — what the question names, attaches, or points at (a symbol, attached content, a specific behaviour). A file the investigation consulted is content only when it answers the question (it IS the subject, or the subject's root cause, or the mechanism the question asks about); it is noise when it surfaced by identifier collision (same name in an unrelated test fixture, a look-alike symbol in another package) or was traversed for grounding but does not itself answer the question. Judge by relevance to the subject, not by whether it is 'code' or 'tooling'.",
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
- "complete" — you assert this list enumerates EVERY symbol that answers the question. Your list is cross-checked against the expected answer count (the larger of: how many items the investigation found, and how many the classification declared required). A short claim of "complete" is DOWNGRADED to "lower_bound" with a visible caveat in the rendered answer.
- "lower_bound" — symbols are confirmed present, more may exist. Honest default when you cannot confidently reach the floor.
- "unknown" — investigated but no definitive slate. Renderer drops the section entirely and falls back to the shape-based prompt.

Summary field — accuracy and clarity come first. Do NOT self-shorten below what the answer genuinely needs; a terse lead-in on a subtle question is a regression. Any hard length ceiling is applied by the tool at submit time and will be reported back to you as a concrete limit if you exceed it — there is no need to pre-estimate. If the user section includes a Diagram Contract block with Required: yes, summary MUST contain at least one grounded triple-backtick diagram regardless of shape.
- shape=explanation — Summary IS the answer body. Write a thorough multi-paragraph explanation that fully addresses the user's question: mechanism details, code-level specifics, cross-file relationships. Organize with ### sub-headers when covering multiple sub-topics. Depth matches the question's depth — a shallow question yields a short answer, a deep one yields a deep answer.
- shape=step_list — Summary is a short natural-prose lead-in that frames what the step list captures: what initiates the sequence, what terminal state it reaches, and the main guard or branch point if there is one. Avoid restating every step's description here; the numbered list below owns the hop-by-hop detail, summary is the connective narrative. When the Diagram Contract says a diagram is required, embed the appropriate ASCII diagram after the prose — the renderer emits only the numbered list and any visual structure is your responsibility, including linear call chains.
- shape=list_of_symbols — Summary describes what the list enumerates and the terminal criterion used to pick the symbols, so the reader understands what kind of item each row is and why these rows belong (and none of the others). A bare one-liner works only when the list is self-explanatory. Summary should not duplicate the per-row rationale column — it frames the list, the rationale column explains each row's role.
- shape=boolean — Summary sets up the decision. The core reasoning belongs in the boolean.rationale field — explain the mechanism at the depth the subtlety requires. A subtle boolean with a one-sentence rationale is a regression. When Diagram Contract says required, summary must also carry the grounded diagram.
- shape=value / config_value — Summary disambiguates and contextualizes the literal. State what the value means, where it comes from in the call graph / config lineage, and why the cited file:line is the authoritative source. The renderer prints the literal and the cite; summary is where the MEANING lives. When Diagram Contract says required, summary must also carry the grounded diagram.

Visual structure (IMPORTANT — users need to understand logic at a glance):
- For explanation shape: organize summary with ### sub-headers when covering multiple topics or stages. Open with a plain-prose lead paragraph that states the core conclusion up front — write it as the first paragraph of the summary body, NOT as a ### TL;DR (or similarly-named) section header, and do NOT prefix it with the literal string "TL;DR:" / "TLDR:". Length is governed by what the conclusion actually requires to be complete, not by a pre-set budget.
- Diagrams are REQUIRED whenever the user section's Diagram Contract says Required: yes, regardless of shape. More generally, diagrams are EXPECTED whenever the answer involves control flow, dispatch, timing, architecture, or fan-out. Pick the diagram kind that matches the question:
    - Flow/decision diagram — branches and guards. Boxes plus horizontal arrows ──▶ with ─┬─ / ├─▶ / └─▶ to split into child edges.
    - Sequence diagram — actor-to-actor calls over time. Vertical lanes drawn with │, horizontal arrows ──▶ / ◀── labelled with the call name, ordered top-to-bottom.
    - Architecture diagram — layered components. Stacked boxes separated by │ and └───┘ with labelled edges showing dependencies.
    - Call-DAG diagram — one-to-many dispatch or fan-out. A root box with ├─▶ / └─▶ fan-out to callees; grow to as many levels as the real fan-out requires — do not arbitrarily truncate.
  Always wrap diagrams in triple-backtick fences so monospace alignment survives the terminal/markdown pipeline. Place the diagram near the top of the summary (immediately after the lead paragraph) so readers see structure before prose. Plain explanatory questions without flow/dispatch/architecture content do not need a diagram unless the Diagram Contract explicitly requires one.
- For step_list shape: the renderer emits ONLY a numbered detailed list of steps — it does NOT draw any flow / DAG / sequence diagram for you. When the Diagram Contract says required, YOU must draw the diagram inside the summary text using the same diagram palette above; do not skip linear call chains.
- Use markdown tables when comparing attributes across entities (e.g. "| Entity | Attribute | Value |") — keep column headers abstract; do not copy the question's literal entities or predicates into the headers.
- Use inline code backticks for every function name, type name, and file path — never write them as plain text.

Prose voice — the answer must read as natural explanation, not a call-graph transcript. This applies to every LLM-authored prose field: summary, steps[].description, boolean.rationale, symbols[].rationale.
- Describe what the code DOES and why it matters, not the raw relation between functions (who calls whom at which line). A step body or a rationale is a mechanism description, not a machine-readable edge list.
- Prefer active voice over nominalizations. Natural: "the handler validates the session token and falls back to a refresh when the signature is stale." Mechanical: "a validation of the session token is performed with a subsequent invocation of the refresh path."
- Use connective tissue between clauses that belong together (first / then / when this fails / once … / …), but do not stuff three logical hops into one run-on sentence.
- Reference every identifier, type, and path with inline code ticks — this helps the reader and keeps your identifiers aligned with the files and lines in your citations.
- When the answer is about code behavior, the reader already knows code is involved. Do not open with "This code does X" or "The function does Y" — open with what actually happens.
- Regression patterns to avoid (these all produce mechanical, non-explanatory prose):
    - "foo is called at line 42, which calls bar at line 58 which calls baz at line 71." — reproduces the call graph but does not explain the mechanism.
    - "Defined at foo.go:42. Used by bar." — duplicates the location column and adds no semantic content.
    - "Line N shows that X." — the reader cannot open source from here; describe what X IS, then let the citation stand for the evidence.

Caveats field: an optional string array for honesty markers. When writing caveats, use the same language as the user's question.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item",
			"do not put prose / summaries / rationale in the citation quote field — quote must be a verbatim copy of the source line or empty; the grounder auto-clears mismatches",
			"do not pre-shrink summary for any shape — write what accuracy and clarity require; if a hard length ceiling is in force the tool will reject the call with the exact limit, so there is no need to pre-estimate",
			"do not set citation_ref to a zero-value-looking sentinel; use -1 for 'no citation' and a valid pool index otherwise",
			"do not claim symbols_completeness=complete without meeting the expected answer count shown in the user section — a short 'complete' claim will be downgraded to lower_bound automatically and the downgrade is surfaced as a caveat",
			"do not invent short codename labels (short alphanumeric tokens like `S1` / `F2` / `Stage-N`) in summary or step descriptions. These labels are source-level identifiers — if you write one, it MUST appear verbatim in at least one citation's line range. Do NOT extrapolate by pattern (the existence of `Fallback S1` in source does NOT imply `S2` exists). When in doubt, describe the mechanism by its actual behavior instead of naming it with a made-up label.",
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
			"Read the investigation summary provided as a user section: user question, investigation notes, read files, top evidence items, dataflow findings, expected answer count, and hypothesis set. The expected answer count drives the completeness claim below; see Output Format for its exact meaning and how it interacts with symbols_completeness.",
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
- "complete" — you assert this list enumerates EVERY symbol that answers the question. A "complete" claim cross-checks against the expected answer count (the larger of: how many items the investigation found, and how many the classification declared required); a short list is DOWNGRADED to "lower_bound" with a warning.
- "lower_bound" — these symbols are all confirmed present, but additional symbols may also be part of the answer. Downstream rendering will present a softened "at least these, may add more" prompt. This is the HONEST DEFAULT when you cannot confidently reach the floor.
- "unknown" — you investigated but cannot reach a definitive slate. The answer-symbol section is DROPPED entirely and downstream rendering falls back to a shape-based prompt. Choose this for mechanism questions, value/boolean questions, or genuinely ambiguous evidence.`,
		Prohibitions: []string{
			"the investigation transcript is frozen — this extraction stage has no file access and must not re-emit evidence",
			"do not invent line numbers — if the transcript does not have a line for a symbol, omit that symbol",
			"do not cite files outside the investigation's read-files list",
			"do not fabricate an answer-symbol list — for single-topic mechanism / value / boolean questions, skip emit_answer_symbol. The multi-topic explanation skeleton path (sub_topics ≥ 1) is the only mechanism-shape case where emit_answer_symbol is expected",
			"do not claim completeness=complete unless len(items) >= the expected answer count — a short claim is auto-downgraded",
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
		Goal: "Read the attached runtime log and emit a structured triage bundle (errors + frames + signals + residue) via emit_log_triage exactly once. Path validation and the resolved-files list are derived automatically from your emission — focus on extraction, not resolution.",
		Workflow: []string{
			"Read the attached runtime log from the 'Attached Runtime Log' section of the user prompt. If the log was oversized and blobbed to attached_log.txt, use read_file with offset/limit to paginate through the middle — the head+tail preview is already visible inline. Multi-file attachments embed `# codrax-source: <path>` headers between bodies; treat each segment as an independent capture (per-process panic, per-time-window snapshot) when grouping errors.",
			"Identify every stack frame that names a source file with a line number. For each frame, emit: file (as it appears in the log — do NOT normalize paths; path normalization is automatic), line, func (best available identifier), pkg (module/namespace hint when obvious), lang, raw (the original log line for this frame, required), confidence (0.0-1.0, your certainty the frame is real).",
			"Group frames under the error they belong to. Emit errors[] with one entry per logical error (per goroutine in a Go panic dump, per exception in a multi-exception traceback). For each error set type (exception class / panic type), message (the human-readable text), and frames[] (the stack for THIS error).",
			"Chain causal errors via the cause pointer. When the log shows 'Caused by:' (Java), 'during handling of' (Python __cause__ / __context__), or '#[source]' (Rust), nest the upstream error in the cause field. Keep the chain shallow — practical depth 3 or so; depth is capped at 5.",
			"Set meta.lang to the dominant language (go/java/cpp/python/node/rust/ruby/csharp/kotlin/arkts/cangjie/unknown/other). ArkTS is HarmonyOS UI code in .ets / .ts files with V8-style stack frames; Cangjie is HarmonyOS native code in .cj files with JVM-like frames `at demo.cart.Cart.method(Cart.cj:42)`; Kotlin is JVM code in .kt files with frames `at com.example.Foo$bar(Foo.kt:42)`. Two tag-formatted log families share one parser path: hilog (HarmonyOS) lines look like `01-26 11:01:06.870 1051 1051 W 00201/test: message`; Android logcat lines look like `04-15 14:32:18.421  5821  5821 E JsApp: message` — structurally identical: <timestamp> <pid> <tid> <level> <tag>: <body>. Extract the body portion while keeping the full line in the frame's raw field. Set meta.signals from the canonical enum (panic/crash/oom/timeout/permission/db/network/validation/logic/other) by matching the observed symptoms. Multiple signals are OK when the log describes compound failures. Summary is an optional one-line synopsis. PERFORMANCE traces (HiTrace/atrace/systrace/perfetto) are handled by a SEPARATE perf_triage stage via emit_perf_trace — do NOT attempt to parse trace events here; if the attached text contains `tracing_mark_write: B|pid|tag` events, return an empty errors list with signals=[] and let the perf_triager handle it.",
			"Log chunks that do NOT structure into an error (build noise, log-level prefixes, unrelated debug output, truncation markers) go into unknown_chunks. Do not punt everything there — only genuinely unparseable pieces. Each chunk capped at 500 chars, at most 8 chunks total.",
			"Frames with uncertain line numbers or uncertain file paths should be emitted with line=0 or file='' and confidence < 0.5 — such frames stay in the bundle but are NOT added to the repo file list. Zero information loss on partials.",
		},
		ToolSuggestions: []string{
			"read_file", // blob pagination only
			"emit_log_triage",
		},
		OutputFormat: `You have ONE emit tool: emit_log_triage. You have ONE read tool: read_file, used SOLELY for paginating the attached log blob when it was too large to inline. You do NOT have grep / repo_map / list_files — path resolution is handled automatically.

Schema in one glance:
- meta.lang        (required) — the dominant runtime language
- meta.signals[]   (required, may be empty) — what-went-wrong enum values
- meta.summary     (optional) — one-line synopsis, ≤ 200 chars
- errors[]         (required, may be empty) — array of { type, message?, frames[], cause? }
- errors[].frames[] — { lang?, file?, line?, func?, pkg?, raw (required), confidence (required) }
- errors[].cause   — recursive error (same shape); for linear causal chains like Java Caused-by
- unknown_chunks[] (optional) — ≤ 8 strings ≤ 500 chars each, for unstructurable text

Caps (enforced by the tool):
- entities: 32
- resolved_files: 10
- cause depth: 5

Build-machine path prefixes (e.g. /build/*, /home/user/src/*), Java basename resolution, repo-existence filtering, and runtime-internal frame filtering (Go stdlib, node: URIs, java.base/*) are applied automatically to the file paths you emit — keep paths AS THEY APPEAR in the log. resolved_files / entities / intent_hint / coverage are derived from your emission and MUST NOT appear in your output.

You emit exactly one emit_log_triage call per dispatch. Do NOT write prose — the bundle is the deliverable.`,
		Prohibitions: []string{
			"do NOT resolve file paths yourself — emit frames with the file path AS IT APPEARS in the log; build-prefix stripping, Java basename resolution, repo-existence filtering, and runtime-internal filtering are automatic",
			"do NOT use grep / list_files / repo_map / exec_command — the stage's tool allowlist explicitly excludes them; path verification is automatic",
			"do NOT invent frames that are not in the log — a frame with confidence 0.9 must correspond to a real log line whose text you can paste into the raw field",
			"do NOT emit fields outside the documented schema (resolved_files, entities, intent_hint, coverage are derived automatically from your emission, not LLM-emitted); the JSON schema rejects unknown fields",
			"do NOT punt the entire log to unknown_chunks — extract every stack frame you can; unknown_chunks is for genuinely unparseable noise, not a fallback for lazy extraction",
			"do NOT produce multiple emit_log_triage calls — exactly one emit per dispatch; a second call replaces the first",
		},
	})

	// The perf_triage skill. The agent reads the user-attached
	// HiTrace / Android-systrace / perfetto excerpt and emits ONE
	// emit_perf_trace call with a structured PerfBundle (jank spans +
	// main-thread stalls + cold-start timing + frames). Unlike
	// log_triage there is no recursive cause chain — performance data
	// is span-centric. The system derives Layer-4 (ResolvedFiles /
	// Entities / IntentHint / Coverage) automatically.
	r.Register(&Config{
		Name: "perf-triage-skill",
		Goal: "Read the attached HiTrace / atrace / systrace / perfetto text excerpt and emit a structured PerfBundle (frames + janks + stalls + startup) via emit_perf_trace exactly once. Identify janky frames whose duration exceeds ~16.6 ms on the UI thread (the 60-fps frame budget) and attribute them to the innermost tracing_mark_write B|…|tag span that was open at the jank start.",
		Workflow: []string{
			"Read the attached trace from the 'Attached Performance Trace' section. The same channel carries HiTrace (HarmonyOS hdc), atrace / systrace (Android adb), and perfetto text dumps — meta.source records which one you observed. Multi-file attachments embed `# codrax-source: <path>` headers between bodies; treat each segment as an independent capture (different process / time window / device) when correlating janks. If the trace was blobbed (large), use read_file to paginate — head+tail is inline.",
			"Identify the source: `# ftrace` header or `TASK-PID CPU# TIMESTAMP FUNCTION` header = systrace / atrace / hitrace (all ftrace-compatible). A textual `perfetto` banner = perfetto text dump. Set meta.source accordingly. meta.duration_ms = last_timestamp − first_timestamp (milliseconds).",
			"Walk every `tracing_mark_write: B|<pid>|<tag>` begin and pair it with its matching `E|<pid>` end. The delta is the span duration. UI-thread span tags typically prefixed with `H:` on HarmonyOS (`H:RenderService:DoFrame`, `H:Layout:measure`, `H:Drawing`, `H:DataLoader:fetchSync`) or use Android conventions (`Choreographer#doFrame`, `performTraversals`, `RenderThread`).",
			"For every UI-thread span whose duration exceeds the 60-fps frame budget (16.67 ms), emit a PerfFrame with janky=true AND a PerfJank entry: start_ts_ms = B timestamp; duration_ms = delta; trigger_span = the INNERMOST tag active inside the span (whichever B| opened after the outer one and whose E| closed before the outer's E|); reason = best guess (io / lock / sync-call / heavy-compute); tags[] = the stack of tags active at the jank peak (outermost to innermost).",
			"For main-thread blocking calls longer than 100 ms, emit a PerfStall: kind = io / lock / sync-rpc / native-call (pick the most specific); symbol = the trace tag's identifier portion; file / line = when the tag's format embeds them (e.g. `fetchSync at DataLoader.ets:42`).",
			"If the trace covers a process cold-start (observable by `ActivityTaskManager`, `AppInit`, `AbilityManagerService`, `WindowStage.loadContent`), emit a PerfStartup: mode=cold (or warm/hot if evidence suggests), app_launch_ms / ability_init_ms / first_frame_ms when measurable. Over 1.2 s app_launch_ms = slow cold start.",
			"Put any unstructurable chunks into residue[] (≤8 entries, ≤500 chars each). If the trace content genuinely has no jank / stall / startup signal, emit with frames=[] janks=[] stalls=[] AND a meta.summary explaining why — the validator then treats the emission as 'nothing to report' rather than failure.",
		},
		ToolSuggestions: []string{
			"read_file",
			"emit_perf_trace",
		},
		OutputFormat: `You have ONE emit tool: emit_perf_trace. Read-only pagination tool: read_file (for large trace blobs). No grep / repo_map — path resolution is automatic.

Schema in one glance:
- meta.source        (required) — one of: hitrace / atrace / systrace / perfetto / unknown
- meta.duration_ms   (optional) — total trace span
- meta.app_pid       (optional) — dominant foreground PID
- meta.signals[]     (optional, enum) — jank / cold-start-slow / main-thread-stall / io-block / gc-pause / render-miss
- meta.summary       (optional, ≤200 chars)
- frames[]           (optional, ≤200) — { duration_ms (req), frame_no, ts_ms, phase, janky }
- janks[]            (optional, ≤50)  — { start_ts_ms, duration_ms (req), trigger_span, reason, tags[] }
- stalls[]           (optional, ≤50)  — { start_ts_ms, duration_ms (req), kind, symbol, file, line }
- startup            (optional)       — { mode (req: cold/warm/hot), app_launch_ms, ability_init_ms, first_frame_ms }
- residue[]          (optional, ≤8)   — unstructurable chunks

Layer-4 fields (resolved_files, entities, intent_hint, coverage) are derived automatically — DO NOT include them in your emit; they are not in the schema.

Exactly one emit_perf_trace per dispatch. Prose is ignored — the bundle is the deliverable.`,
		Prohibitions: []string{
			"do NOT parse the trace as if it were a panic log — this is span-over-time data, not an exception chain",
			"do NOT use grep / list_files / repo_map — the stage's tool allowlist excludes them; the perf_triager is structurally identical to log_triager in that respect",
			"do NOT emit resolved_files / entities / intent_hint / coverage — those are derived automatically from your emission",
			"do NOT fabricate spans that are not in the trace — every PerfJank entry must correspond to a B|...|tag / E|pid pair observable in the attached trace body",
			"do NOT produce multiple emit_perf_trace calls — exactly one emit per dispatch; a second call replaces the first",
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

	// The perf-segmentation skill mirrors log-segmentation for the
	// HiTrace / atrace / systrace / perfetto channel. Step A of the
	// perf two-step fallback. Registered alongside log-segmentation
	// so the perf_triager's two-step controller can resolve it by
	// name from the same skill registry.
	r.Register(&Config{
		Name: "perf-segmentation-skill",
		Goal: "Segment the attached HiTrace / atrace / perfetto excerpt into byte-addressed regions by kind (frame_window / jank_region / startup / thread_run / context / noise) so the downstream per-segment extractor can focus on one coherent block at a time. Emit exactly one emit_perf_segmentation call.",
		Workflow: []string{
			"Read the attached trace from the 'Attached Performance Trace' section. Channel covers HiTrace (HarmonyOS hdc), atrace / systrace (Android adb), and perfetto text dumps — same byte-stream regardless of capture tool. Multi-file attachments embed `# codrax-source: <path>` headers; segments should NOT cross those boundaries (one segment per capture window keeps Step B's per-segment dispatch attribution clean). If the trace was blobbed (large), use read_file to scan the full body — segmentation needs coordinates over the complete text.",
			"Walk the trace top-to-bottom. Identify byte ranges that contain: a single render-frame envelope worth scrutinising (kind=frame_window — typically a B|...|H:RenderService:DoFrame ... E|... pair); a contiguous run of janky frames or stalls (kind=jank_region); a process / activity startup window (kind=startup — bounded by ActivityTaskManager / AppInit / WindowStage.loadContent on HarmonyOS, ActivityThread / Application#onCreate on Android); a long-running CPU burn or I/O block on one thread (kind=thread_run); the trace header / metadata banner (kind=context); or unrelated noise (kind=noise).",
			"Emit at most 10 segments. Overlap is NOT allowed — byte_end of segment N must be ≤ byte_start of segment N+1. Segments must be sorted by byte_start.",
			"Use the hint field for a short (≤80 char) per-segment label so the downstream per-segment extractor knows what it is looking at (e.g. hint='cold-start ActivityTaskManager 1.2s' or hint='LazyForEach jank window 3 frames').",
			"The system validates byte coordinates against the trace length. Segments that are reversed, zero-length, or out of bounds are silently dropped. context + noise segments are still recorded for diagnostics but Step B skips them.",
		},
		ToolSuggestions: []string{
			"read_file", // for blobbed traces
			"emit_perf_segmentation",
		},
		OutputFormat: `Emit ONE emit_perf_segmentation call with the full segments[] list.

Schema in one glance:
- segments[] (required, up to 10 entries)
  - byte_start (required, integer ≥ 0)
  - byte_end   (required, integer > byte_start)
  - kind       (required, enum: frame_window | jank_region | startup | thread_run | context | noise)
  - hint       (optional, ≤ 80 chars)

Do NOT emit any other tool call. Do NOT write prose.`,
		Prohibitions: []string{
			"do not call emit_perf_trace — that is Step B, dispatched by the agent after Step A segmentation completes",
			"do not overlap segments — ranges must be disjoint and sorted",
			"do not produce more than 10 segments — coarsen the granularity if the trace has more candidate regions",
			"do not invent kinds outside the enum — the schema rejects unknown values",
		},
	})

	// ── B0 write-mode skills ───────────────────────────────────
	//
	// Three skills paired with the planner / coder / verifier agents.
	// Registered AFTER the 5 read-mode skills so their presence in the
	// registry cannot perturb read-mode skill resolution (no shared
	// name; explicit lookup by stage → skill name in topology.go).
	//
	// L6 red line: each write skill intentionally keeps exec_command
	// in its ToolSuggestions. Safety comes from the git worktree
	// sandbox (Day 2 landed): plan / apply / verify stages run inside
	// a detached worktree so a runaway rm -rf hits the worktree copy,
	// never the user's main checkout. Stripping exec_command would
	// break the flexibility promise (Q2) without adding security
	// — worktree isolation is the safety surface, not skill
	// allowlisting.
	//
	// L3 red line: the three write-mode emit tools (emit_change_plan,
	// apply_patch, emit_test_results) are listed in these skills but
	// each one's Execute method is structurally forbidden from calling
	// ground.BuildContext. Enforced by
	// internal/tool/write_mode_red_lines_test.go (go/ast scan at
	// build time).

	r.Register(&Config{
		Name: "change-plan-skill",
		Goal: "Produce a concrete ChangePlan for the user's requested code change by reading the relevant source and emitting emit_change_plan exactly once.",
		Workflow: []string{
			"Read the files and identifiers named in the user's request. Use read_file / grep / repo_map to understand the current shape of what the change has to touch.",
			"Identify the MINIMAL set of files that must be modified to address the request. A good plan touches the fewest files that still completely solves the problem.",
			"For each target file decide the Kind: create (new file; new_content is the full body), modify (overwrite existing file; new_content is the full body), delete (remove; new_content ignored), or patch (surgical edit via a unified diff; fill the patch field with the diff text and leave new_content empty). Prefer patch for small/surgical edits so git apply validates hunks against the real file; prefer modify when you're rewriting most of the file.",
			"Emit exactly one emit_change_plan call. request must restate the user's ask, summary must be 3-10 sentences describing what the plan does and why, and changes[] must include one entry per target file with a Rationale explaining WHY that file needs that change.",
			"DEPENDENCY CLOSURE — when any new_content imports a package that is NOT already in the repo's go.mod (for example you added `import \"github.com/pkg/sftp\"` and the project never used sftp before), you MUST also include a 'modify' entry for go.mod that adds the require line, AND if the project tracks go.sum, a 'modify' entry for go.sum is also expected. The deps-closure validator (V1) will reject the plan otherwise — apply phase cannot succeed without this.",
			"WIRING CLOSURE — when changes[] creates a new file under any of these registered subsystems, you MUST also include a 'modify' entry for the corresponding wiring file in the SAME plan: internal/mcp/* → cmd/root.go (mcp.Registry.Register call); internal/skill/* → internal/skill/defaults.go (skill.Registry.Register call); internal/tool/* → cmd/root.go (tool registration); internal/agent/* → cmd/root.go (agent.Registry.Register call). Without the wiring entry the new code is dead — the runtime never instantiates it. The wiring-closure validator (V3) will reject otherwise.",
			"SUMMARY FIDELITY — every path-shaped token (e.g. internal/mcp/ssh.go) and every import path (e.g. github.com/pkg/sftp, golang.org/x/crypto/ssh) you mention in summary MUST match what changes[] actually contains. If your summary names a path, that path must appear in changes[].path; if your summary names an import package, that exact package must be imported by some new_content in this plan. The consistency validator (V4) will reject the plan when summary lies about what's being changed.",
			"ONE CHANGE PER FILE: the changes[] array must NOT have two entries for the same path — the tool rejects duplicate paths. If a file needs two semantic edits, compose them into a single modify (full body) or patch (combined diff).",
			"Use depends_on for ORDERING constraints between changes in this same plan: when creating a new file X and then modifying an existing file Y that will import / call X, set Y's depends_on to [\"X\"]. The apply stage topologically-sorts before writing, so declaring the edge guarantees X lands on disk before Y tries to reference it. depends_on is ALWAYS repo-relative paths of OTHER entries in THIS plan — cross-plan or absolute paths are rejected, as is any cycle (a → b → a). Leave depends_on empty when the default declaration order is correct.",
			"Optionally list acceptance_tests[] — natural-language test assertions the apply stage's verify phase should confirm. Empty is legal (no explicit tests to check).",
			"Do NOT invoke apply_patch or run_tests from the plan stage — those belong to the apply / verify stages that consume the plan later.",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
			"list_files",
			"repo_map",
			"exec_command", // Q2 red line: kept deliberately; worktree sandbox guards runaway commands
			"emit_change_plan",
		},
		OutputFormat: "Your only structured output is the single emit_change_plan tool call. Any prose around the call is discarded — compose the plan body directly inside the tool's summary / changes[] fields. Call the tool once; a second call overwrites the first.",
		Prohibitions: []string{
			"do not modify any file during the plan stage — this stage produces the proposal, it does not execute it",
			"do not invent file paths that do not exist in the repository — read_file or grep to verify paths first",
			"do not emit apply_patch or run_tests — those are for downstream stages",
			"do not write a plan whose changes[] array is empty — a plan without any proposed change is meaningless",
			"do not emit two changes[] entries with the same path — one change per file per plan, compose when necessary",
			"do not create cycles in depends_on — a → b → a, or any longer loop, is rejected. When two files genuinely mutually depend on each other's edits, they usually belong in a single combined change",
		},
	})

	r.Register(&Config{
		Name: "code-write-skill",
		Goal: "Apply every ChangeUnit from the installed ChangePlan to the active git worktree by calling apply_patch once per unit. The evaluator's ShouldStop fires when WriteClosure.AppliedSet ⊇ plan.TargetPaths.",
		Workflow: []string{
			"HarmonyOS + Android language rules (honour the project's language — the ChangePlan's target_paths file extensions tell you which): " +
				"(1) ArkTS strict mode (.ets / .ts inside HarmonyOS Stage Model): NO `any`, NO `unknown`, NO `as` casts, NO index signatures `{[k:string]: T}`, NO `Function` type; every class field MUST have an explicit type; import paths MUST include the extension (`.ets` / `.ts`); stick to the 21-decorator whitelist (@Component @Entry @Preview @CustomDialog @Observed @Reusable @Builder @BuilderParam @Styles @Extend @State @Prop @Link @Provide @Consume @Watch @ObjectLink @StorageLink @StorageProp @LocalStorageLink @LocalStorageProp); struct (not class) for UI components; a single root (Column/Row/Stack) inside build(). " +
				"(2) Cangjie (.cj): use `func` (not `function`); the FIRST non-comment statement MUST be `package xxx.yyy`; mark public API with `public` or `open` (default is package-internal); use `extend Type { ... }` for type extensions; prefer `match` over cascaded if/else for algebraic data types; `foreign func` for FFI declarations; annotate thread-constrained entrypoints with `@CallingThread`. " +
				"(3) Kotlin (.kt / .kts): default visibility is public — use `internal` / `private` when a symbol should not cross module boundaries; prefer `val` to `var`; use `data class` for value carriers, `sealed class` for closed hierarchies; mark suspending functions with `suspend` (do not inline blocking I/O in them); prefer extension functions over utility classes; use `when` for pattern-like matching; on Android keep Activity/Fragment references inside lifecycle-aware scopes.",
			"The orchestrator has already loaded the ChangePlan onto Mutable and swapped ctx.RepoRoot to a git worktree checkout. Your job is purely mechanical: iterate plan.changes[] and emit one apply_patch call per ChangeUnit.",
			"Emit apply_patch with {path, kind} ONLY. The tool reads new_content / patch / delete directly from the plan on Mutable — you do NOT re-emit or re-derive the content. Attempting to send new_content or patch as parameters fails the schema (DisallowUnknownFields). Read a file only if you need to understand it for a later retry turn; the apply call itself needs nothing but the ChangeUnit's path and kind.",
			"Respect depends_on ordering: apply a unit ONLY after every path in its depends_on list is already in AppliedSet (visible because those apply_patch calls returned Success=true earlier in this dispatch). The apply_patch tool re-enforces W1b and rejects out-of-order calls, so a mistake surfaces as a clean error you can self-correct in the next turn.",
			"When a turn's response lists multiple apply_patch calls in parallel (tool_use blocks), make sure no two units in the batch have a depends_on relationship — the batch executes concurrently and ordering is not guaranteed within a batch. If unsure, emit one apply_patch per turn.",
			"If apply_patch returns Success=false for a unit, read the error summary carefully. Common rejections: W1 (path not in plan.TargetPaths — you drifted), W1b (depends_on not yet applied — reorder), kind mismatch (you sent a different kind than plan declares). Correct the parameters and retry on the NEXT turn.",
			"Stop when every plan.target_paths entry is present in AppliedSet. The evaluator checks automatically; you don't need to signal completion explicitly. Extra turns after completion waste tokens and may trigger the iteration cap.",
			"kind=patch is applied via `git apply -` inside the worktree against the plan's unified diff (sourced from Mutable, not your call). When git rejects a hunk (context mismatch, fuzzy match disabled), apply_patch surfaces git's own stderr verbatim so you can see which hunk failed. Retry is NOT a plan change — the plan's diff is authoritative; a failure means the planner's diff did not match the worktree state and the verify→plan retry loop (or a fresh plan) is the remedy.",
		},
		ToolSuggestions: []string{
			"read_file",
			"apply_patch",
			"exec_command", // Q2 red line: preserved for debugging (e.g. verify file wrote) — worktree sandbox contains blast radius
		},
		OutputFormat: "Your ONLY structured output is apply_patch tool calls with {path, kind}. One call per ChangeUnit; the evaluator stops when every plan.target_paths entry is applied. Any prose is ignored — do not draft 'I'll now apply X' narration; just emit the tool call.",
		Prohibitions: []string{
			"do NOT modify files outside plan.target_paths — W1 will reject and your dispatch burns turns",
			"do NOT run tests here — verify stage owns that",
			"do NOT invoke emit_change_plan — the plan was already emitted upstream and this stage consumes it",
			"do NOT send a different kind than the ChangeUnit declares — plan says create, you send create; plan says modify, you send modify",
			"do NOT send new_content or patch as apply_patch parameters — schema rejects them; the tool hydrates from the plan on Mutable so transcription errors are impossible",
		},
	})

	r.Register(&Config{
		Name: "test-execute-skill",
		Goal: "Run the project's test suite inside the active worktree and produce a structured ChangeReport. run_tests is deterministic — it auto-detects the runner (Go / Node / Python / Rust), executes the command, parses the output, and installs Mutable.ChangeReport. Your role is optional: call emit_test_results to add a human-readable FailureSummary narrative when failures need context beyond aggregate counts.",
		Workflow: []string{
			"Call run_tests with empty suite (run all tests) or a language-appropriate selector. The tool auto-detects the runner from project manifests (go.mod / package.json / pyproject.toml / pytest.ini / Cargo.toml) and uses the corresponding command + parser. One call is usually enough — the full suite outcome lands on Mutable.ChangeReport.",
			"Read the ChangePlan's acceptance_tests[] (rendered in the prompt) for context. Acceptance matching is not automated; the criteria are informational for the narrative you may draft.",
			"When the prompt includes a 'Pre-existing baseline failures' list, classify each failing test in the post-apply report as REGRESSION (passed in baseline, fails now) or PRE-EXISTING (failed in baseline and still fails). The baseline is an authoritative pre-apply snapshot — a failing test that also failed in baseline is NOT this plan's fault, and your narrative should say so explicitly.",
			"When all tests pass: return without any tool call. The verify stage automatically promotes the passing report to a success outcome, and the orchestrator renders the summary.",
			"When tests fail: optionally call emit_test_results with a 1-4 sentence failure_summary narrative explaining the root cause (e.g. 'all failures are in the new handler's error path — the edge-case test expected a wrapped error but got the raw one'). This replaces the parser's auto-generated count summary with your more useful context. Do NOT try to override the Passed verdict — it comes from the parser and the tool ignores LLM overrides.",
			"Do NOT re-run the tests multiple times or try to 'fix' them from this stage — B1 policy is fail-loud, and verify failures surface to the user to drive a re-plan. Apply/verify does NOT auto-retry in B1.",
		},
		ToolSuggestions: []string{
			"read_file",
			"run_tests",
			"emit_test_results",
			"exec_command", // Q2 red line: preserved for diagnostic commands (git status, ls) — worktree sandbox contains blast radius
		},
		OutputFormat: "No required tool call. run_tests is usually enough; emit_test_results is OPTIONAL narrative. If you don't emit either, the evaluator's ShouldStop fires on ChangeReport presence (which run_tests installs synchronously). Free-form prose between tool calls is discarded.",
		Prohibitions: []string{
			"do NOT modify files — verify is strictly read-only w.r.t. the worktree",
			"do NOT invoke apply_patch — that is the apply stage's job",
			"do NOT invoke emit_change_plan — the plan was emitted upstream",
			"do NOT try to override the Passed verdict via emit_test_results — the parser is authoritative; divergent LLM claims are logged as a warning and ignored",
			"do NOT re-run tests multiple times to hunt for flakiness — B1 treats any failure as fail-loud; re-planning is the correct response",
		},
	})
}
