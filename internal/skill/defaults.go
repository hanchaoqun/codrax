package skill

import "github.com/hanchaoqun/codrax/internal/types"

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
		Name: "multi-repo-focus-skill",
		Goal: "Choose the smallest useful set of sub-repositories for the current multi-repo question before normal analysis starts.",
		Workflow: []string{
			"Read the current request and compact topology rows. Select exact root_rel values that should be active for this question.",
			"Prefer one or two sub-repos. Select more only when the question clearly requires multiple repositories.",
			"If the current request literally names a topology root path, use source=user_explicit_in_request for that candidate. Otherwise use source=model_recommended.",
			"Call emit_multi_repo_focus exactly once. Do not inspect source files and do not answer the user.",
		},
		ToolSuggestions: []string{"emit_multi_repo_focus"},
		OutputFormat:    "Use the emit_multi_repo_focus tool only.",
		Prohibitions: []string{
			"do not call source navigation tools",
			"do not infer focus from prose keywords when no exact topology path is present",
			"do not emit polished answer prose",
		},
	})

	r.Register(&Config{
		Name: "explore-skill",
		Goal: "Investigate the user's question by collecting grounded evidence from the active lanes (source, runtime trace/log, VCS, command output, or external observations) and handing a well-supported evidence record to answer synthesis.",
		Workflow: []string{
			"PHASE 1 — Breadth scan: use repo_map and grep (files_only=true) to map a bounded candidate set before reading files. For mechanism, architecture, call-chain, handler/route/config lookup, or current-source explanation questions, repo_map is usually the best first step for locating likely source paths and relationship neighborhoods; then use read_file or targeted grep to prove the selected facts. Start repo_map with overview/task_map/file_map for orientation; when you need a scoped member inventory or member→attribute checklist, use repo_map(view=\"source_inventory\") with model-chosen roles, optional attribute_roles, and scope/scopes, then cascade into narrower source_inventory calls instead of reading every candidate file. Treat repo_map / source_inventory rows as verified navigation facts, not semantic source-code citations: use them to bound candidate files/symbols/routes/config keys, then verify selected behavior or implementation claims with read_file or targeted grep before citing source text. Relation rows may end with a bracketed origin tag carrying a confidence label that grades edge strength: prefer higher-confidence edges when choosing what to read next, and treat lower-confidence edges as real but unproven leads — never as errors and never as citable facts on their own. For non-English questions, search with BOTH the original terms AND their English programming equivalents. Try multiple keyword variants (word roots, synonyms, abbreviations) only when exact/high-confidence searches are too narrow.",
			"PHASE 2 — Depth investigation: use grep (for targeted pattern search) and read_file (for full context) — pick the most efficient tool for each situation. After each file, call emit_evidence with ALL facts in one batch. Do not emit line-scope evidence from repo_map/grep navigation output alone: first open the selected file/range with read_file and copy the exact gutter line number. For each file extract: (a) key data structures, (b) control flow, (c) configuration-driven behavior, (d) cross-component interactions. When obvious, also set emit_evidence's optional `context_role_hint` (defining / absence_support / related_context / illustrative_only) and `diagram_role_hint` (default / config / runtime / override) so the rendered answer can reuse validated structure instead of guessing from prose. Use `surface_terms` for exact user-visible labels/aliases found in the cited source/log/trace lines that are not already captured by subject/object/anchor_symbol, such as original file labels, route names, package/module names, config keys, macro names, trace span names, runtime object labels, or labels in leading documentation/header comments attached to the anchor. Every surface term must be copied verbatim from already-read lines; the tool validates this and answer synthesis should preserve relevant terms when they matter to the visible answer. Use `absence_support` when a grounded fact helps prove the exact target is missing but does not itself define the target. For config-precedence / config-trace questions, treat `diagram_role_hint` as required whenever the evidence is clearly a defaults layer, a config-file layer (YAML/JSON/TOML/INI/etc.), a runtime binding layer, or an override layer. Set `load_bearing_summary: true` ONLY when the `summary` text holds a scalar (a hash, version string, count, single concrete identifier, or any value derived from a tool / shell / git / build command output) that the user-facing answer must reproduce verbatim AND the typed fields (`subject` / `predicate` / `object` / `anchor_symbol` / `snippet`) cannot themselves carry that scalar — without this flag, the final answer may omit the summary as nearby-context noise. Default false is correct for the common case where summary is a paraphrase the typed fields already encode. When the row itself is important to the user-facing answer, set optional `salience`: `load_bearing` for a fact the answer cannot honor without this row, `exhaust_listed` for a member of a complete list the user asked for, `supporting` for an intermediate fact the answer chain uses, or `context` for background. Omit `salience` when unsure; it helps preserve important rows but does not replace member_set, answer_symbol, citations, or final answer obligations.",
			"Source operation-site sets: when the user asks for all write points, call sites, registration points, entry points, or similar current-source operation locations, the final answer needs a principal `aggregate_facts.member_set`, not just evidence prose. Put each operation site in `members`, map it to the exact function/call/file:line via `support_refs`, and keep target constants, paths, registry names, or config keys as details rather than substituting them for the operation-site citation.",
			"emit_evidence anchor scope (REQUIRED): every emit_evidence item MUST set `scope`.\n\n" +
				"DECISION TREE — pick scope by what your evidence actually proves:\n" +
				"  • Is the fact at ONE specific (file, line)? → `line`\n" +
				"  • Is the fact a multi-line block (struct body, function body, comment block)? → `line_range`\n" +
				"  • Is the fact a named YAML/Go/JSON/TOML schema section (top-level group, const block)? → `section`\n" +
				"  • Is the fact about a FILE's identity as a layer (the file IS the config layer / CLI registration layer / manifest)? → `file`\n" +
				"  • Is the fact a cross-file contract (e.g. \"X is registered in one file AND implemented in another\", or \"no CLI flag for X anywhere\")? → `crossfile`\n" +
				"  • Is the fact a confirmed absence (the user's target is NOT in this file/section/struct)? → `negative`\n\n" +
				"REQUIRED FIELDS PER SCOPE:\n" +
				"  - `line`        : source + line_start + anchor_kind + anchor_symbol\n" +
				"  - `line_range`  : source + line_start + line_end > line_start\n" +
				"  - `section`     : source + section_path (a dot-separated path inside the parsed file — a YAML top-level group name, a nested config key, a Go const-block name, etc.)\n" +
				"  - `file`        : source + file_role_label ∈ {config_canonical, cli_registration, default_struct, manifest, schema}; line_start MUST be 0 — file-identity has no specific line\n" +
				"  - `crossfile`   : crossfile_query{files (≤5), pattern} + crossfile_assertion{kind: exists/forbidden/count_eq}\n" +
				"  - `negative`    : evidence_kind=absent + negative_query{file, pattern, section?} + negative_scope ∈ {file, range, section, struct_fields}\n\n" +
				"WORKED EXAMPLES (generic placeholders — replace `<...>` with the actual repo paths and identifiers you observed):\n" +
				"  ✓ scope=`file`     {source: \"<config-file>\", file_role_label: \"config_canonical\"}\n" +
				"     — anchors the canonical config file AS the config layer, regardless of whether the missing target appears in it.\n" +
				"  ✓ scope=`negative` {source: \"<config-file>\", kind: \"absent\", negative_query: {file: \"<config-file>\", pattern: \"<missing-target>\"}, negative_scope: \"file\"}\n" +
				"     — the absence becomes a structured citable fact.\n" +
				"  ✓ scope=`crossfile` {crossfile_query: {files: [\"<cli-registration-file>\"], pattern: \"<expected-cli-flag-pattern>\"}, crossfile_assertion: {kind: \"forbidden\"}}\n" +
				"     — confirms no CLI flag binding exists.\n" +
				"  ✓ scope=`line`     {source: \"<default-struct-file>\", line_start: <N>, anchor_kind: \"definition\", anchor_symbol: \"<defaults-function-name>\"}\n" +
				"     — anchors the code default's actual function definition.\n\n" +
				"INPUT-TO-OUTPUT WORKED EXAMPLE: when you emit the four items above for an absent-target config-trace question, the answer's citation pool renders to (one bullet per scope):\n" +
				"  - `<config-file>` [layer: config_canonical]                          — surfaces the file as a layer\n" +
				"  - `<config-file>` [absence: `<missing-target>`]                      — surfaces the verified absence\n" +
				"  - cross-file contract: <expected-cli-flag-pattern> in <cli-registration-file>  — surfaces the cross-file forbidden assertion\n" +
				"  - `<default-struct-file>:<N>`                                        — surfaces the line-anchored default\n" +
				"Each citation has its own structural shape — the user reads the answer and immediately sees layer / absence / contract / specific-line distinctions. Compare with collapsing everything into `scope=line`: every citation renders as `<file>:<N>`, the layer / contract / absence semantics get hidden in prose, and the answer surface is weaker.\n\n" +
				"COMMON ANTI-PATTERNS:\n" +
				"  ✗ Using scope=`line` to anchor a sibling key just to make a config file \"appear\" in citations[]. Prefer scope=`file` for layer identity + scope=`negative` for absence.\n" +
				"  ✗ Using scope=`line` for a fact like \"<group> has no CLI flag\" — there's no specific line to point at. Use scope=`crossfile` with assertion=forbidden.\n" +
				"  ✗ Setting scope=`file` on a code file you read at one specific line — scope=`file` is for the file's role, not its contents. Use scope=`line` with the anchor instead.\n\n" +
				"Schema-level scopes (file / crossfile / negative) are the right shape for layer-identity, cross-file-contract, and absence facts that have no per-line code anchor. Do NOT collapse these into `line` by hunting for an arbitrary sibling line — the answer surface is stronger when the evidence shape matches the fact shape.",
			"NAMED-CALLSITE BATCHING (applies whenever the user's question demands enumeration of a named set — either (a) explicit count N stated in the question, OR (b) an exhaustive-coverage demand like 'all of them' / 'each one' / 'every X' / 'list them all' / 'how many ... and what are they' / '一共' / '分别' / '每个' / '所有' — AND the source carries parallel callsites that each have their own identifier): emit ONE evidence item PER named callsite — each with `scope=line`, a distinct `anchor_symbol` set to the verbatim identifier (function / type / config key / handler name), and the line where that identifier is registered. Do NOT collapse parallel callsites into ONE item with `scope=line_range` covering all the lines plus a summary phrase that mentions the count (e.g. summary='N items in append order' / 'N functions registered'). The final answer needs to render one user-visible bullet per identifier, each citing a verbatim name; an aggregated ranged item gives only ONE typed name and forces the renderer to fill the rest with placeholders. The same rule applies to parallel registrations / parallel handler entries / parallel layer definitions — wherever the question's enumeration scope refers to a SET whose members each have their own identifier in the source.",
			"if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
			"ROLE-DESCRIPTION evidence: for any question that asks what a component / module / stage / agent DOES (its purpose, role, or responsibility) — not just where it lives — every component anchor you emit (anchor_kind=definition / registration) SHOULD be paired with a second emit_evidence item using `evidence_kind=mechanism` whose `summary` paraphrases the component's role from the leading documentation comment / doc-string at or above the definition line. Read the ±5 lines above the definition; if the comment describes what the component does, capture that as a separate mechanism evidence item. This pairing gives answer synthesis BOTH axes (where the component lives, what it does); without it the answer's section bodies have only structural facts and become a list of file:line locations rather than a description of behaviour. The pairing is not required for questions that only ask 'where is X' (no role axis present). NOTE: an automatic pairing pass can derive a mechanism item from a leading doc-comment in the same source; your authored mechanism item, when present at the same anchor, takes precedence and the auto-pair stands down, so prefer authoring the role-description sentence yourself when you can paraphrase richer than the literal comment.",
			"cross-reference: when file A references file B, read file B too — don't assume, verify",
			"never read test files — they are derivative, not authoritative. Never read utility/infrastructure files unless the question is about them",
			"PATH SCOPE: when the user's question names an explicit repo-relative path (a file, directory, or package — e.g. `internal/tool` / `cmd/server/main.go` / `pkg/api`), every shell command (find, grep, exec_command, wc) MUST be scoped to that path. Do NOT default to repo-root scope (e.g. `find .` / `wc -l **/*.go`) when the user asked about a specific subtree — the answer's value will be wrong (whole-repo total instead of the requested subtree's total), and the body will fail readability gates that expect the path to appear in the answer. When the question names MULTIPLE paths (e.g. compare A vs B, trace from X to Y), investigate each named path with equivalent depth — equal number of read_file calls on equivalent line ranges, proportional emit_evidence items per path. A two-path comparison answer that emits 80% of its evidence about one side (because that side's file is larger or happens to be listed first) renders as single-sided and fails the comparison; give each path its fair share of the investigation budget.",
			"REPOSITORY COMMAND SCOPE: in read mode, `exec_command` starts in the active repository root. Use repo-relative paths. Do NOT guess absolute checkout paths, do NOT `cd` outside the repo, and do NOT use `git -C` / `--git-dir` / `--work-tree` with arbitrary absolute paths. For VCS questions, prefer the structured git tools first: `git_log` for recent commits, `git_show` for a specific commit/ref's metadata/patch/stat/name-only output, `git_diff` for working-tree/range patch/stat/name-only diff output, and `git_history_search` for bounded history counts/searches. Use `git_history_search` with `order=recent` for latest/last-N windows and `order=oldest` for first-introduced / earliest-occurrence windows. Use `exec_command` only for deterministic checks the structured tools cannot express.",
			"COUNT QUESTIONS — split by what the count actually measures: (A) SYNTACTIC count — the answer is the number of files / lines / regex matches / fixed-pattern occurrences (\"how many .py files\", \"total LOC of Y\", \"how many functions named X\", \"语法层面的计数\"). For SYNTACTIC counts the answer MUST come from a deterministic counting tool — `exec_command` running `grep -c <pattern> <path>`, `find <path> | wc -l`, `wc -l <files>`, or your platform's equivalent. Read the file first ONLY to figure out the right pattern; then run the counting tool and use its exact integer output as the answer. Do NOT count by reading the file and tallying matches yourself — visual counts go off-by-one easily on long files, on multi-block declarations, and on overlapping patterns. (B) HISTORY count — when the user asks how many of the last N commits / merges / revisions under a path contain or touch a named surface, prefer `git_history_search` with `window_path`, `window_count`, `diff_path`, `contains`, and `order=recent`; when the user asks for the earliest / first-introduced occurrence, use `order=oldest` (then `window_count` is the maximum matching commits to return while scanning from the earliest commit). It returns `answer_count=<n>` plus matching commits without shell loops or manual list intersection. (C) SEMANTIC count — the answer is the SIZE of a set whose members are filtered by a property that requires READING and UNDERSTANDING the code (\"how many endpoints accept JSON\", \"几个 config key 被废弃了\", \"how many handlers do auth checks\", \"how many functions take a context parameter\", \"how many classes implement interface I via duck typing\"). SEMANTIC counts cannot be answered by a single tool call because the filter property is not a fixed token. The correct path is: (1) enumerate the candidate set first (`repo_map` / `grep -l` to surface all candidates), (2) read each candidate to confirm the property holds (this is the semantic step the tool can't do), (3) the count is the size of the confirmed-property set, and the answer cites the confirmed members. For SEMANTIC counts, set `is_relational_lookup=true` alongside `is_count_question=true` in the analysis emit so the question is recognised as enumeration-then-count rather than tool-then-count.",
			"CALL-CHAIN COVERAGE: when the user asks how a request / value / piece of data flows from an entry point through a pipeline to its exit, list every load-bearing intermediate function the data passes through, not just the first 2-3 you find. If the entry function is large (>500 lines), read the full body before stopping — load-bearing pipeline steps are often scattered across the function. The answer needs entry + ≥3 mid-segment + exit; stopping early produces a partial chain that misleads the reader. The same rule applies to processing pipelines, data flows, and call sequences across files.",
			"CONFIG PRECEDENCE: when the user asks where a configuration value comes from or which layer wins, surface evidence from EVERY layer in the precedence chain: the source-code default (the struct or constant that defines the baseline), the configuration file (yaml / toml / json / ini / properties — whichever format the project reads), and the runtime override (CLI flag, environment variable, command-line argument). A layer answer that names only 1-2 of these is half-formed; if a layer truly does not exist for the given key, say so explicitly rather than omitting it.",
			"MECHANISM WIRING: when the user asks WHICH parameter / gate / function governs a specific behavior, do not stop at a name-similar or concept-adjacent symbol — confirm the candidate is actually CONSUMED on that behavior's execution path (grep its usage sites and check at least one reader sits on the asked path) before naming it as the answer. A field or constant that is never read on the asked path is a different mechanism, however plausible its name; when two candidates compete, prefer the one with a verified consumption site and cite that site. The same applies to absence claims: a token found only in meta/infrastructure surfaces (linters, blocklists, test fixtures, docs) is not evidence about the runtime surface — trace the nearest REAL group or mechanism the asked name belongs to before concluding non-existence.",
			"EXHAUSTIVE ENUMERATION: when the user asks for a list of items spread across a package, directory, module, route table, config surface, class hierarchy, registry, or multi-language scope, use a deterministic enumeration/navigation tool that covers the full search space rather than reading individual files one at a time. Good tools for this shape: `repo_map(view=\"implementers\", query=\"<InterfaceName>\")` for the exhaustive set of concrete types implementing an interface / trait / protocol (the typed Implements relation — strictly better than grepping the name, which misses implementers whose definition never spells the interface), `repo_map(view=\"source_inventory\")` for typed member/attribute checklists with counts and paging, `repo_map` task_map/file_map for structural orientation, `grep -l <pattern> <dir>` for every matching file, `exec_command find <dir> -name '...'` for sub-directory discovery, and `grep -rn <pattern> <dir>` for every match across the tree. Visual `read_file` of one file at a time produces incomplete enumerations because the investigation budget runs out before every candidate is visited — the answer becomes an honest 'lower-bound' instead of the exhaustive list the user asked for. After the exhaustive tool returns the candidate set, read/grep selected items to capture role descriptions or proof, and back counts/member sets with the tool output plus grounded verification rather than manual tally from prose.",
			"if the request attached a runtime log (panic / exception trace / sanitizer diagnostic / traceback), the stack-frame file:line pairs are the files you MUST open FIRST before widening the search. Read from the innermost (deepest) frame outward so the actual error site is the first fact you establish",
			"COMPLETION: when you have collected enough evidence to answer the user's question, call emit_investigation_complete(reason, confidence, result_kind). This call is REQUIRED to signal investigation complete — without it, the loop continues. Set result_kind='resolved' for ordinary positive/citable answers. If your confidence is not at least 'medium', continue investigating.",
			"COVERAGE BEFORE COMPLETION: when the user's question carries a structural coverage obligation (the pre-scan section above names them: an exhaustive-coverage demand, a declared item count, or a partition into named groups), every candidate file your grep / repo_map / list_files surfaced MUST be either read_file'd OR explicitly excluded via a narrower follow-up grep before you call emit_investigation_complete with result_kind='resolved'. A grep that returns 8 candidate files and only 3 read_file calls is an under-coverage failure on a 'list every X' question — the answer slate would silently ship 3-of-8. Either complete the reads, or set result_kind='absence' with an honest absence_justification, or re-run grep with a tighter pattern that confirms the unread hits were collateral. Mechanism, architecture, and call-chain explanations do NOT require every navigation candidate to be exhausted unless the structured analysis also declared one of those coverage obligations; for those shapes, read and cite the load-bearing files that prove the flow and treat collateral candidates as optional navigation hints.",
			"ABSENCE ANSWERS: if the answer is an honest 'zero' / 'no X' / 'nothing found' (e.g. 'how many .py files?' → 0, 'does handler X exist?' → no), set result_kind='absence' and add absence_justification to emit_investigation_complete with a one-sentence explanation. Ordinary cited answers require citations by default, but an honest-zero answer has nothing to cite — the declaration waives the citation floor. You still must have run at least one real investigation tool (grep / exec_command / list_files / read_file / repo_map); an empty investigation cannot declare absence. Never set absence_justification on a positive answer. Grounded related-context anchors are still allowed when they remain clearly contextual and do not define the missing exact target.",
			"EVIDENCE_FLOOR_WAIVER (typed escape for external / non-repo inputs): when YOU have read the input and confidently determined that pretending to ground against this repository would be misleading, set `evidence_floor_waiver` on emit_investigation_complete to declare that ordinary repo-grounding requirements do not apply to this investigation. Use it when: (a) the attached log / trace is from a system whose code is NOT in this repo (a customer paste from another service — set reason='external_only_log' or 'external_only_trace'); (b) the input's frame paths superficially resemble current-repo paths but you have determined they represent a different build / version / deployment (set reason='no_repo_intersection'); (c) the input is informational only — debug breadcrumb, performance baseline, clean trace with no failure component (set reason='informational_runtime_only'). The waiver requires `reason` (one of the four enum values) AND `rationale` (one sentence audit trail). Do NOT use the waiver to short-circuit ordinary investigation work or to escape questions that genuinely need repo grounding — the audit trail records every fire and misuse is reviewable post-hoc. When in doubt, do NOT set it: leave repo-grounding requirements in effect and ground normally. If you previously set a waiver and later discover that repo grounding DOES apply, set `clear_evidence_floor_waiver=true` on emit_investigation_complete and omit `evidence_floor_waiver`. When the waiver IS appropriate, the question is answered from the LOG / TRACE content itself (its frames, message text, observed sequence) — cite log entries verbatim, do not synthesise repo file:line citations.",
			"PRINCIPAL_SPAN_WAIVER (typed escape for legitimately-direct source→sink call chains): when YOU have read the source and confirmed there is genuinely NO separately-citable user code between the cited source endpoint and the cited sink endpoint, set `principal_span_waiver` on emit_investigation_complete to declare the source→sink intermediate-evidence requirement does not apply. Use it when: (a) source and sink are on the same statement / line — a one-liner dispatch, wrapper, or trampoline (reason='endpoints_directly_adjacent'); (b) the lines between contain only plumbing the user is not asking about — nil guards, accessor pass-through, defer setup, logger calls — with no separately-citable user logic (reason='no_intermediate_user_code'); (c) the intermediate crosses an FFI / JNI / cgo / cross-language native bridge where the hop is in the runtime, not in repo source (reason='platform_bridge_intermediates'); (d) the compiler / JIT inlined the intermediate so the source line directly invokes the sink with no separate frame (reason='inlined_call'); (e) the dispatch is virtual / interface / closure / reflection-based — no static call edge exists (reason='runtime_dispatched_call'); (f) the chain crosses into an external library / SDK whose intermediate frames are not in this repo (reason='external_module_continuation'). The waiver requires `reason` (one of the six enum values) AND `rationale` (one sentence audit trail naming concrete details, e.g. 'dispatch and handler are on the same statement at foo.go:42'). Do NOT set the waiver to skip emitting intermediate evidence you simply have not looked for; only when you have read the source span and confirmed there is structurally nothing to cite. If you previously set it and later discover an intermediate fact does exist, set `clear_principal_span_waiver=true` on emit_investigation_complete and omit `principal_span_waiver`. Every fire is logged for post-hoc audit.",
			"PRIOR-CONVERSATION RECALL: if the user references their own past conversations, pick by intent: (a) TOPIC search ('我们之前讨论过 OAuth / what did we say about X?') → recall_memory(query='X'); (b) LISTING / inventory ('都有哪些 / 历史里有什么 / what's in memory') → list_memory(limit=10) which returns the most-recent N entries by time, NOT by keyword overlap. recall_memory's keyword scoring will only surface self-referential meta-entries on a generic listing query — that's why list_memory exists as a separate tool. Both bypass the injected `## Prior conversation` block's keyword-only matching; either may be sufficient on its own — emit_investigation_complete with result_kind=resolved if so.",
		},
		WorkflowTierB: []TierBItem{
			{
				Body:      "RUNTIME TRACE FIRST: when the typed dispatch carries a runtime trace / systrace / hitrace / perftrace / tracebundle artifact, start with `trace_query` before repo_map, broad grep, reading from the trace head, or hand-written grep/awk loops. `trace_query(path=...)` accepts `.tracebundle.json` directly and automatically merges sibling `.systrace` + `.perftrace` / sibling `.tracebundle.json`, so pass the concrete path the system surfaced instead of manually unpacking sidecars first. For frame/span ids or labels, start with `trace_query(view=\"event_search\", pattern=\"<literal>\")`; for inode/file IO, use `event_search` with `pattern` as the inode or entry_name and event_types such as `file_io`, `page_cache`, `f2fs`, or `android_fs`; for CPU samples, use event_types=[\"perf_sample\"] and pattern as a symbol, DSO, callchain, event, or thread token. `pattern` is a literal substring, not a regex, and should usually be one exact frame id, span label, marker token, inode, file name, perf symbol, or timestamp before adding time/line/thread filters. Treat `entry_name` as a trace file-name label, not an absolute path; do not turn it into `/entry_name` or `/data/...` unless that full path is grounded. For mixed trace+source questions, use `trace_query` for runtime-artifact facts and use normal source tools only for the explicitly requested current-code verification lane.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "TRACE QUERY: when a typed runtime trace / systrace / hitrace / perftrace / tracebundle artifact is active, prefer `trace_query` before hand-written grep/awk loops. Pass `.tracebundle.json`, `.systrace`, or `.perftrace` paths directly; trace_query automatically promotes sibling `.tracebundle.json` and merges sibling `.systrace + .perftrace` pairs. Use `view=\"event_search\"` with `pattern` as a literal substring (not regex) for exact frame ids (`1917295`), jank ids (`jank_frames=7`), span labels (`Choreographer#doFrame`), marker labels, inode tokens (`0x478e5`), entry_name values, perf sample symbols/DSOs/callchains, timestamps, or event labels; if it returns zero rows, shorten the literal, add `event_types=[\"trace_mark\"]` for B/E/C/S/F span rows, `event_types=[\"perf_sample\"]` for CPU sample rows, or `event_types=[\"file_io\"]` / `[\"page_cache\"]` / `[\"f2fs\"]` / `[\"android_fs\"]` for inode rows, remove over-narrow pid/thread/time filters, or use returned span/time/line windows before root-cause analysis. Use `view=\"span_window\"` with `span_name` when the user names a trace span instead of exact timestamps; event_search rows expose span_action/span_pid/span_name/span_value, and span_window/window_stats trace_spans expose kind=sync|async. Synchronous B/E span ends are unnamed `E|pid` or bare `E` on the same ftrace thread stack, async S/F spans pair by marker pid+name+cookie, so do not search for `E|pid|spanName` to prove completion. If multiple span windows match, first locate the specific frame/span marker with `event_search(pattern=\"<exact literal>\")`, then rerun follow-up views with the selected time_start/time_end or line_start/line_end. Pick the lens from the full view matrix: use " + RenderTraceQueryViewMatrix() + ". For binder IPC, consume `oneway`, `sync_like`, and `blocking_candidate` from trace_query output rather than guessing blocking semantics from raw flags. Treat perf sample output as code-execution context: `window_stats.perf_samples` is broad same-window context, `root_cause_rank.items[].perf_context` is compact candidate interval/thread context, `root_cause_rank.items[].perf_contexts` is role-aware support such as candidate_thread/target_running/on_chain_dependency/same_cpu_competitor/cpu_pressure_top_running/compute_supply_cpu, and `frame_root_cause_bundle` role fields such as `target_running_perf`, `on_chain_perf`, `binder_peer_perf`, and `same_cpu_competitor_perf` show where sample hotspots belong. For perf evidence-quality questions, read `sample_cpu_scope`, `cpu_known/cpu_unknown`, `sample_kind`, and `weight_unit` before using nearby scheduler rows: sample_cpu_scope=unknown or sample_kind=off_cpu means the perf sample has no concrete CPU/core execution location, while a sched_switch `[CPU]` column is only the scheduler event row CPU. For running or compute-supply causes, consume perf_contexts to explain what code consumed sampled CPU time, but keep scheduler interval overlap, chain relevance, binder peer state, D-state/IO, CPU/core/frequency/affinity, and supply pressure as the causal basis. top_symbols/top_dso/top_callchains explain what sampled threads were doing, but scheduler interval overlap, chain relevance, binder peer state, D-state/IO, and CPU supply remain the causal basis. When the final answer reports a specific trace event, carry that event's NATIVE identifying key=value fields verbatim (e.g. a page-fault row's operation= and address=, an event's code/flag values) in addition to any generically requested dimensions like path/latency/bytes — do not compress an event down to the requested dimension list when its own native fields identify it. Inode summaries identify dev/inode/op/bytes/count/latency/churn from the trace; mapping inode to a pathname requires trace entry_name/path fields or a separate filesystem mapping, so do not invent a path from inode alone. `entry_name` is a file-name label, not an absolute path; never render it as `/entry_name`, `/data/...`, or another directory-qualified path unless the trace or external mapping provided that exact full path. Trace timestamps are seconds end-to-end: 928.081774 = 928s + 0.081774s; with six fractional digits the fractional part is microsecond-precision (81774 us). Durations are reported in ms. `read_file` pagination uses `line_offset` as a zero-based line coordinate, not a byte offset; use trace_query/grep line windows or gutter lines to choose it. If the trace flavor is already typed as Harmony/鸿蒙/东湖/OHOS, pass `trace_flavor=\"harmony_hitrace\"` or `platform=\"harmony_hitrace\"`; if the trace flavor is typed as Android/安卓/atrace, pass `android_atrace`. Typed platform intent wins for that call, while content-detection conflicts are only audit caveats. trace_query reports trace_flavor and priority_semantics: HarmonyOS/hitrace means larger numeric priority is higher (1-40 CFS, 41-139 RT), while Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Its results are runtime-artifact evidence with artifact-local line numbers; do NOT turn trace rows into current-source citations unless separate source evidence proves a current checkout fact. If `trace_query` says the format is unsupported or incomplete, fall back to targeted grep/read_file/exec_command with preserved line numbers.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "PERF SAMPLE PROVENANCE: preserve trace_query perf sample `source` and `symbolization_status` in final markdown/html reports. `raw_perfdata_fallback` / `unsymbolized` rows are useful for time/thread/DSO/IP correlation but lower confidence than official hiperf/simpleperf symbolized output.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
		},
		ToolSuggestions: []string{
			"repo_map",
			"grep",
			"trace_query",
			"read_file",
			"list_files",
			"exec_command",
			"git_log",
			"git_show",
			"git_diff",
			"git_history_search",
			"recall_memory",
			"list_memory",
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
- When you truly have enough evidence, call emit_investigation_complete(reason, confidence, result_kind). That tool call — not your prose — is the completion signal.

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
			"repo_map / source_inventory rows are verified navigation facts: existing scopes, files, candidate symbols/routes/config keys, languages, counts, and candidate-universe membership. They are not semantic source-code citations. Bracketed origin/confidence tags on relation rows grade edge strength only: a low-confidence edge is still a legitimate navigation lead to verify, not an error. Use them to choose and bound what to verify next, then use read_file or targeted grep for selected behavior / implementation claims before citing source text. list_files is also valid navigation evidence for real directory membership.",
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
		Goal: "Produce the final answer as a structured AnswerDocument by calling emit_answer_document exactly once. The structured emit IS the delivery — the surrounding rendering to user-visible prose is automatic. The Required Answer Blocks contract is mandatory: every block listed under `## Required Answer Blocks` in the user section MUST appear in the rendered answer with the right kind, count, and grounded payload. Read that section FIRST, then draft.",
		Workflow: []string{
			"Write the answer DIRECTLY into the `emit_answer_document` tool call from the start. Compose the final text inside the structured fields (per-block `text`, ordered_list `items[]`, citation-anchor `items[]`, diagram payloads, summary prose) as you think — the tool call is the only delivery surface; text outside it does not ship.",
			"CRITICAL CONTRACT: the Required Answer Blocks list in the user section is MANDATORY. Each entry names the block kind (summary / section / ordered_list / bullet_list / scalar / decision / table / diagram / caveat), how many of that kind to emit, and which facet ids the block must cover. Skipping a required block, or emitting a different kind in its place, is a hard rejection.",
			"`blocks[]` is the carrier. It must be emitted as a native JSON array in the tool arguments — do NOT quote it as a string containing escaped JSON. Every block requires `id` (load-bearing — your retry hints reference it back to you) and `kind`. The schema's per-kind conditionals declare which payload field each kind also requires (text / items / diagram); the per-kind workflow rules below cover content discipline for each. Block-level optional fields: `title` (sub-heading), `facet_ids` (which facet ids this block covers — read these from the user section), `surface_role` (set to `principal` on the main-line answer payload; otherwise omit), `claim_uses[]` (block-level claim annotations covered separately). Put scalar literals and binary verdicts directly into the appropriate `kind=scalar` / `kind=decision` block's `text` field — top-level `value` / `boolean` payloads are not part of this tool's schema and are rejected at parse time.",
			"`claim_uses` is REQUIRED on every non-decision principal block (`surface_role=principal`) whose user-section contract lists allowed `claim_form` values for that block. For a principal `decision` block that carries an active typed verdict field (`current_status_verdict` or `error_granularity_verdict`), that verdict field is the carrier; add `claim_uses[]` only when you have a clear extra evidence-shape annotation. `claim_uses` is a block-level plural array; there is no per-item claim_use field. Each entry has EXACTLY 3 fields: `claim_form` (one of: `definition_fact`, `call_edge`, `guard_condition`, `assignment_fact`, `return_fact`, `absence_fact`, `precedence_role`, `external_observation`, `import_edge`, `text_reference_fact`), `facet_id` (optional, SINGULAR), `evidence_id` (optional). Single-form blocks emit a one-element array (`claim_uses=[{claim_form=definition_fact}]`); when the block's items legitimately span multiple forms (e.g. hop chain mixes `call_edge` and `guard_condition`), list one entry per form. Forbidden inside a `claim_uses` entry: `citation_ref` (lives on `items[i].citation_ref`), plural `facet_ids` (uses singular `facet_id`), and `from_node` / `to_node` (live in the block-level `edge_anchors[]` array).",
			"`diagram.kind` is the SEMANTIC family (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax tokens like `flowchart` / `sequenceDiagram` belong inside `diagram.body`. When the user-section's Diagram Contract names a Required kind, USE THAT KIND verbatim — the validator HARD-rejects mismatches. When the contract carries no Required kind (the kind is just a preference / left to the LLM), pick whichever family best fits the answer's grounded evidence.",
			"For an enumeration `ordered_list` block (the user-section says the principal block is `ordered_list` covering enumeration facets): assemble `items[]` from the prior extraction slate and the required-symbol floor rendered in the user section. Each item's `text` is a short natural-prose line describing the item's ROLE in the answer — what it does, how it participates. A rationale like \"the dispatch entry point that maps request kind to handler\" is useful; \"Defined at foo.go:42. Used by bar.\" is a regression because the file:line is already rendered as its own column. Set per-item `citation_ref=N` (TOP-LEVEL on the item, zero-based index into doc.citations[]) so each row points at its evidence; declare the block's `claim_uses=[{claim_form=definition_fact}]` (or `call_edge` / `assignment_fact` when the cited lines are call sites or assignments) at block level. EVERY item.label MUST be grounded in emitted evidence — preferably a verbatim anchor_symbol / subject / object, or a selector-qualified identifier that visibly appears on a grounded snippet line. Fabricated label strings that do not appear in grounded evidence will be rejected at validation time.",
			"For a two-axis enumeration (`principal members` plus a per-member attribute such as entry function / owner / default / handler): the principal `ordered_list.items[].label` is the MEMBER label, not the attribute. Use the evidence subject or required-term member as the item label, and put the attribute symbol/value in `items[].text`, citation, and any companion table row. This is language-agnostic: package/module/directory/namespace labels are valid members even when they are not resolver symbols.",
			"For a hop-chain `ordered_list` block (the user-section says the principal block is `ordered_list` covering call/mechanism path facets): emit `items[]` with one entry per distinct branch or mechanism hop. Each item's `text` reads as natural explanation — state what the step DOES (the behavior, the guard it checks, the effect it produces for the next hop), reference the load-bearing identifiers with inline `code`, and give the reader enough context to understand why this step matters in the larger mechanism. A description that reads like \"`foo` is called at line 42, which calls `bar` at line 58\" is a regression — it reproduces the call graph but does not explain the mechanism. Use as many sentences as accuracy requires; one item is one logical hop, do not collapse two. Each item carries `citation_ref=N` (top-level) only when a repo file:line citation really backs it; declare the block's `claim_uses[]` to cover every form the items use — list one entry per form (`call_edge` for caller→callee sites, `guard_condition` for conditional branches that gate the hop, `return_fact` for return-value hops). When an item intentionally asserts a directed relation, include a compact explicit edge surface such as `` `caller` -> `callee` `` inside the item; when an item is only boundary, comparison, or exclusion context, keep it as prose without an arrow so the typed citation-role validator does not treat it as the main relation. An item that paraphrases an attached-log frame (external source) should be backed by a `claim_uses` entry with `claim_form=external_observation` and should stay uncited unless the artifact frame resolved to a current repo file:line — a citation whose cited line shares no identifier with the description will be rejected.",
			"Bucket alignment (any answer): when the user-section's resolved-target block lists user-named buckets (extracted from a question that paired multiple labels with parallel asks — e.g. 'X for A, Y for B'), each bucket label MUST appear verbatim somewhere in the rendered answer's user-facing fields. Preferred rendering for a summary-only answer: each bucket gets its own `### <Label>` section heading inside summary or its own section block. For an ordered_list answer: cluster items under per-bucket prose introducers, OR mention each label inside relevant item.text. Skipping a bucket label is a hard reject — the user's mental partition must survive into the rendered answer.",
			"For a single-literal `scalar` block (the principal block is `scalar`): put the literal directly in the block's `text` field as the rendered value (e.g. `text=\"42\"` for a count answer, `text=\"foo/bar.go\"` for a file-path answer). Use the doc summary block (or the same scalar block's title / a sibling section block) to provide 1-2 sentences that NAME the subject being measured / queried (file path, symbol name, directory, config key, measurement target — verbatim from the question) AND state how the value was obtained (which command / file / chain produced it) — the bare literal alone is rarely a complete answer; readers need to see WHAT was measured and HOW. To anchor the citation, attach a single-element `items=[{id:\"v\", citation_ref: N}]` whose citation_ref points at the citation backing the literal — the renderer reads items[0].citation_ref as the scalar's citation. Set the block's `claim_uses=[{claim_form=definition_fact|external_observation}]` (the plural array form at block level; a single-element array is correct). When the literal is sourced from attached log semantics, command / VCS output, or another external trace rather than repo code, use `claim_form=external_observation`, leave the item uncited, and state that provenance in the prose — a citation whose cited line does NOT contain the literal will be rejected.",
			"For a `decision` block (yes/no): the principal block is `decision`. If the user-section asks for `current_status_verdict` or `error_granularity_verdict`, set that typed verdict field on the block and keep block `text` as rationale/evidence boundary only; the typed verdict field is the decision carrier, so do not guess a `claim_uses[]` form just to satisfy the decision. Otherwise put the verdict at the START of the block's `text` field (one of `yes` / `no` / `true` / `false` / `是` / `否` — no hedging) followed by the rationale prose: name the invariant or guard that forces the answer, reference the load-bearing identifiers with inline `code`, and explain the mechanism at whatever depth the subtlety requires. Structure the rationale as natural prose, not as a pointer to evidence: \"Line N shows X\" is a regression because the reader cannot open source from here. A terse rationale on a subtle question is also a regression. To anchor the citation, attach a single-element `items=[{id:\"d\", citation_ref: N}]` whose citation_ref points at the line backing the verdict; leave that item uncited when the decision is sourced from log / external trace rather than repo code. For non-typed decision blocks, set the block's `claim_uses=[{claim_form=guard_condition}]` or `[{claim_form=definition_fact}]` matching whichever the cited line shape is.",
			"For an `ordered_list` block over enumeration items: every item's resolved `file/line` (pointed at via top-level `items[i].citation_ref`) must be a real repo anchor where the named identifier actually appears. If the identifier comes from an attached log / external trace, DO NOT invent a file:line — either drop the item or keep the answer in summary / caveat prose without pretending the item is repo-grounded. The same rule covers a changed-file or commit list obtained from VCS history (e.g. git_show / git_diff / git_log --stat / --name-only output): those rows name a file or commit but carry NO verified source line, so DO NOT attach a fabricated `file:1` (or any guessed line) to make them look repo-grounded. Keep each such item — the user asked for the list — but present it with `claim_form=external_observation`, either uncited with the path / commit stated in the item prose, or with a file-scope citation (scope=`file`) when a file-identity role applies; reserve a real `file:line` citation for a line you actually opened with read_file in the current checkout.",
			"For a summary-only / explanation answer (the only principal block is `summary`): fill the summary block's `text` with a thorough multi-paragraph explanation. Structure with ### sub-headers for each major topic/stage. Open with a plain-prose lead paragraph that states the core conclusion — write it as the first paragraph, not under a heading or label. Length matches what the conclusion needs (one sentence if it fits; several if context matters). Include mechanism details with inline `code` references and cross-file relationships. When the family's contract carries a `diagram` block requirement, emit a separate `diagram` block (do not jam Mermaid into the summary `text`).",
			"Declare every file:line you cite ONCE in the document-level citations[] array. References to the citations[] array by zero-based integer index live on `items[i].citation_ref` (top-level on each item), regardless of the block kind. For scalar / decision blocks that hold a single literal / verdict, attach a one-element `items=[{id:\"x\", citation_ref: N}]` to anchor the citation. `claim_uses[]` entries do NOT carry `citation_ref` — they carry `claim_form` / `facet_id` / `evidence_id` only. Use the schema's explicit no-citation carrier to mean no cite. One cited line can serve multiple items without duplication. Citation channel discipline (anti-duplication): once `items[i].citation_ref` is set, do NOT ALSO write the same file:line as a trailing parenthetical inside the item's `text` body (e.g. `\"... is the abstract interface (path/to/file.ext:8)\"`) — the renderer attaches the inline cite from `citation_ref` automatically, and a prose-embedded `(file.ext:N)` produces duplicate rendering AND leaves the facet-anchor slot empty (the parens are prose text, not a typed reference, so the facet-coverage tracker reports the facet as unanchored). Prose says what the symbol IS / DOES; the typed channel says where it lives. Facet linkage: when the block covers a specific facet listed in the user-section's contract, also set the singular `facet_id` on its `claim_uses` entries — that link is what marks the facet as anchored; leaving `facet_id` blank means the facet stays declared-but-unanchored even when `citation_ref` is set.",
			"Diagram-grounding contract: when your summary contains a diagram, every FILE-SHAPED token you write inside that diagram — anything matching `<name>.<ext>` for a code extension (e.g. `foo.go`, `bar/baz.py`, `mod.rs:42`, `App.tsx`) — MUST appear in citations[] or in the Log Triage section's frames. PREFERRED diagram form is a fenced code block whose info-string is the word `mermaid` (flowchart or sequenceDiagram inside); ASCII art is the fallback only when the Mermaid subset cannot express the shape. Conceptual / role labels that are NOT file-shaped (e.g. `Analyzer`, `HTTP Handler`, `Session Store`, `Cache Layer`) are fine without a citation — they describe roles, not source locations. The grounding gate runs only against file-shaped tokens, so a diagram of role nodes connected by edges is unconstrained. A separate validator flags any multi-word CamelCase/snake_case token (e.g. `MaxIterations`, `handleRequest`, `parse_json`) that does NOT correspond to an exported / module-level definition discoverable across the codebase — function-local variables / scoped helper names trip it. The cleanest way to keep such tokens in a diagram is to EMBED a cited file:line inside the SAME label text, e.g. `[\"for i < maxIter (<src/path/file.ext>:<line>)\"]` where the same `<src/path/file.ext>` appears in citations[]. A label that contains a cited file path is treated as grounded as a whole, so the bare identifiers nested inside inherit that grounding (the WHOLE label IS the cited evidence; individual tokens don't need independent indexing). Use this pattern for diagrams that walk concrete code lines. When the attached log's Log Triage section renders a \"Call chain (innermost → outer)\" block, the GROUNDED FRAMES from that block are the authoritative file set for any call-chain / sequence / flow diagram you draw — keep every frame and prefer the Mermaid form (rendering applies a consistent layout). You MAY extend the chain with additional grounded callers when your investigation supports a richer mechanism, but do NOT introduce caller/callee files that are NOT grounded by frames or citations[]. For cross-file callers you could not directly observe, describe the relationship in prose or use a role label rather than inventing a file-shaped node.",
			"Log-triage coverage contract: when an attached log produces a structured Errors tree (top-level error Type plus an optional Caused-by chain), your summary MUST name every Type at least once using the exact class / exception identifier that appears in the Log Triage section. The rule applies across every shape and every signal family. A single-level log requires naming its one Type; a multi-level Caused-by chain requires naming each link. Paraphrasing the chain as 'a cascade of errors' without naming the individual Types is not sufficient — a summary that shares zero tokens with a given Type's identifiers (case-insensitive) will be rejected. Do NOT replace the real Types with domain-unrelated descriptions or invent alternative stack frames when the log's frames are the ground truth.",
			"Code-vs-narrative divergence (REQUIRED on enumeration questions where the code and a comment / docstring disagree about set membership): when the user asks for an enumeration over a structural relation (\"list all implementers / subclasses / overrides / handlers of X\") AND a candidate's CODE STRUCTURE places it in the set (its method set satisfies the interface, its concrete type derives from the base class, etc.) but a NARRATIVE CUE in the source — an author comment, a docstring, naming convention — argues against inclusion (\"does not really implement X\", \"only handles part of the protocol\", etc.), DO NOT silently pick one side. Both signals are legitimate: code is the ground-truth structure, the comment is the author's intent. The honest answer DESCRIBES the divergence so the user can judge: include the candidate in the principal `ordered_list` block with an item text that begins \"[caveat]\" and names BOTH the structural fact and the narrative reservation (e.g. \"[caveat] method set satisfies the interface (file:line) but author comment at file:line states it does not handle Y — borderline membership\"), OR name the candidate verbatim in the lead summary block as an explicit exception case (\"X also has the matching method set but the author note at <file>:<line> excludes it because <reason>; whether to count it depends on whether you read the contract by structure or by author intent\"). Silent omission is wrong: the user receives a partial set without knowing items were filtered, and later decisions made from the incomplete set are not auditable. The code-side membership is cross-checked against your enumeration; when candidates are omitted without being named, the contract checker rejects the emit and asks you to either include them or surface them as exceptions. To pass: either include the candidate (with caveat rationale) or name it in the summary block as an exception. The answer's value to the user comes from transparency about the divergence, not from picking the cleanest-looking side.",
			"Absence-citation discipline (REQUIRED when exact_resolution.status='absent'): an absent finding is a CITABLE FACT, not a hedge. Whenever you set `exact_resolution.status='absent'`, the citations[] array MUST include at least one entry with `scope='negative'` AND a non-empty `negative_pattern` field naming the EXACT search query whose absence-of-matches confirms the finding (the literal pattern you ran with grep / repo_map / search, or the missing identifier itself). The `file` field on a negative-scope citation names the file the absence applies to (or a repo-wide marker like `(repo-wide grep)` when the search was repo-wide); `line` may be 0 for negative scope — there is no specific line to anchor at. Pair the negative-scope citation with any related-context citations (scope='line' / 'file' / 'crossfile') that explain WHY the absence matters for the user's question. Why this matters: an unbounded absence claim ('the target is not used anywhere') is unauditable — operators cannot reproduce the search. With `negative_pattern` recorded, anyone can re-run the same query and verify. The system rejects status='absent' answers that lack a bounded negative-scope citation. Schema-level template (no project specifics): `{file: '<file-or-repo-wide-marker>', scope: 'negative', negative_pattern: '<exact-query-you-ran>'}`. When the user's question implies multiple search surfaces (the answer covers absence across several distinct files / layers / contracts), emit one negative-scope citation per surface — each pins one search query the operator can replay.",
		},
		ToolSuggestions: []string{
			"emit_answer_document",
			// Protocol-level retry preservation. LLM picks
			// emit_answer_document_patch on retry paths when only a
			// few blocks need editing — system structurally preserves
			// every typed annotation field on unchanged blocks.
			"emit_answer_document_patch",
		},
		OutputFormat: `You have NO file-reading tools — no read_file, grep, or repo_map. You are a pure synthesizer working from accepted evidence. Your contribution is ONE emit_answer_document (or emit_answer_document_patch on retry paths) tool call per attempt — the structured emit IS the delivery, the surrounding rendering is automatic. Do NOT write tool-call JSON in your text — use the function-calling mechanism only.

## Tool choice

- **First dispatch / no prior emit**: call ` + "`emit_answer_document`" + ` with the FULL document.
- **Retry path** (you see ` + "`## Hard Rule (retry attempt N)`" + ` and ` + "`## Previous Emit`" + ` sections in this prompt): PREFER ` + "`emit_answer_document_patch`" + ` when only a few blocks need editing. Pass ` + "`unchanged_block_ids: [\"id1\", \"id2\", ...]`" + ` to assert preservation of every typed annotation field (claim_uses, edge_anchors, facet_ids, surface_role) on blocks you do NOT need to edit — the system clones them byte-identical from your previous emit, so you cannot accidentally drop a field. Use ` + "`replace_blocks`" + ` for the blocks you DO need to fix; use ` + "`add_blocks`" + ` for new blocks. The patch tool rejects if you cite an unknown id, conflict ops, or emit an empty patch.

## Block contract

The answer payload is the ` + "`blocks[]`" + ` array. Emit it as a native JSON array in the tool arguments, not as a JSON-encoded string with escaped quotes. Each block has:
- ` + "`id`" + ` (load-bearing — your retry hints reference it back to you)
- ` + "`kind`" + ` ∈ ` + "`summary`" + ` / ` + "`section`" + ` / ` + "`ordered_list`" + ` / ` + "`bullet_list`" + ` / ` + "`scalar`" + ` / ` + "`decision`" + ` / ` + "`table`" + ` / ` + "`diagram`" + ` / ` + "`caveat`" + `
- payload (kind-specific, see below)

The user section's "Required Answer Blocks" list names the block kinds + count + facet ids you MUST emit for THIS dispatch. Skipping a required block, emitting a different kind in its place, or under-counting is a hard rejection.

## Block-kind payloads

| kind | payload | notes |
|------|---------|-------|
| ` + "`summary`" + ` | ` + "`text`" + ` (prose) | the answer body for summary-only dispatches; lead-in for compound dispatches |
| ` + "`section`" + ` | ` + "`title`" + ` + ` + "`text`" + ` | per-bucket / per-layer / per-topic chunks within a longer answer |
| ` + "`ordered_list`" + ` | ` + "`items[]`" + ` (each with ` + "`id`" + `, optional ` + "`label`" + `, ` + "`text`" + `, optional top-level ` + "`citation_ref`" + ` (zero-based index into doc.citations[], or -1)) | enumeration slate OR hop chain depending on the family's principal facet |
| ` + "`bullet_list`" + ` | ` + "`items[]`" + ` (similar shape, including top-level ` + "`citation_ref`" + ` per item) | unordered collection (legend / aside) |
| ` + "`scalar`" + ` | block ` + "`text`" + ` carries the literal; optional one-element ` + "`items=[{citation_ref:N}]`" + ` for the citation anchor | single literal answer |
| ` + "`decision`" + ` | verdict + rationale in block ` + "`text`" + `; optional one-element ` + "`items=[{citation_ref:N}]`" + ` for the citation anchor | yes/no answer |
| ` + "`table`" + ` | markdown table inside ` + "`text`" + ` | cross-attribute comparison |
| ` + "`diagram`" + ` | ` + "`diagram{kind, language, body}`" + ` | structural visual; see Diagram contract below |
| ` + "`caveat`" + ` | ` + "`text`" + ` | honesty markers, scope notes |

Block-level optional fields (any kind): ` + "`title`" + `, ` + "`facet_ids`" + `, ` + "`surface_role`" + ` (set to ` + "`principal`" + ` on the main-line answer payload; leave empty otherwise), ` + "`claim_uses[]`" + ` (block-level claim annotations array — see below).

## Claim annotations (REQUIRED on non-decision principal blocks)

When a non-decision block's ` + "`surface_role=principal`" + ` AND the user-section block contract lists allowed ` + "`claim_form`" + ` values for that block, you MUST attach the block's ` + "`claim_uses[]`" + ` array. For a principal ` + "`decision`" + ` block that carries ` + "`current_status_verdict`" + ` or ` + "`error_granularity_verdict`" + `, the typed verdict field is the decision carrier; add ` + "`claim_uses[]`" + ` only when you have a clear extra evidence-shape annotation. Pick from these claim forms:

- ` + "`definition_fact`" + ` — the cited line establishes a typed fact (a const, struct field, function signature, default value)
- ` + "`call_edge`" + ` — the cited line is a function call site (caller→callee edge)
- ` + "`guard_condition`" + ` — the cited line is the condition / branch that gates the answer
- ` + "`assignment_fact`" + ` — the cited line is a config / variable / field assignment that establishes a value
- ` + "`return_fact`" + ` — the cited line is a return statement / function output that yields the answer
- ` + "`absence_fact`" + ` — the cited evidence carries a Negative scope (a search confirmed the thing is absent)
- ` + "`precedence_role`" + ` — the cited evidence carries a layer / override role (config layer, runtime override, default value)
- ` + "`external_observation`" + ` — the cited evidence is from a runtime log / perf trace / external artifact, not repo source code
- ` + "`import_edge`" + ` — the cited line is a module / package import edge (Go ` + "`import`" + `, JavaScript / TypeScript ` + "`import` / `require`" + `, Python ` + "`import` / `from … import`" + `, Java ` + "`import`" + `, Rust ` + "`use`" + `, etc.). Use this when the question asks about MODULE / PACKAGE DEPENDENCIES — where a symbol is SOURCED FROM, what packages a module pulls in, or which files import a given target. Do NOT use it for ` + "`call_edge`" + ` (caller→callee inside the codebase) or ` + "`definition_fact`" + ` (where a symbol is DECLARED).
- ` + "`text_reference_fact`" + ` — the visible source / config / doc / comment text itself is the evidence; use this for comment/doc/config prose references that are not a definition, call, assignment, return, import, or guard.

` + "`claim_uses[]`" + ` is a plural array at block level — single-form blocks emit a one-element array like ` + "`claim_uses=[{claim_form=definition_fact}]`" + `; when items inside the block contribute distinct claim forms (e.g. some hops are ` + "`call_edge`" + `, others are ` + "`guard_condition`" + `), list one entry per form.

For ` + "`call_edge`" + ` and ` + "`import_edge`" + ` items, render the main directed relation with an explicit edge surface such as ` + "`` `caller` -> `callee` ``" + ` or ` + "`` `file` -> `package` ``" + `. Boundary / comparison / exclusion prose that merely names both endpoints should NOT use an arrow; otherwise it is declaring the edge as an answer fact.

Each annotation object is ` + "`{claim_form, facet_id?, evidence_id?}`" + ` — it does NOT carry ` + "`citation_ref`" + `, and it does NOT carry plural ` + "`facet_ids`" + ` (plural ` + "`facet_ids`" + ` belongs on the block; claim annotations use singular ` + "`facet_id`" + `). Citations live on ` + "`items[i].citation_ref`" + ` (top-level on each item). For scalar / decision blocks where the literal / verdict sits in block ` + "`text`" + `, attach a single-element ` + "`items=[{id:\"x\", citation_ref: N}]`" + ` to anchor the citation. Putting ` + "`citation_ref`" + ` inside a ` + "`claim_uses`" + ` object causes ` + "`unknown field \"citation_ref\"`" + ` rejection. The validator rejects non-decision principal blocks lacking the required claim annotation, and rejects any emitted ` + "`claim_form`" + ` outside the user-section's allowed list for that block.

## Diagram contract

When the user section's Diagram Contract says Required: yes, emit a ` + "`diagram`" + ` block:

- ` + "`diagram.kind`" + ` is the SEMANTIC FAMILY: ` + "`flow`" + ` / ` + "`sequence`" + ` / ` + "`architecture`" + ` / ` + "`call_dag`" + `. Pick the one the user section names. **Do NOT write Mermaid keywords** like ` + "`flowchart`" + ` or ` + "`sequenceDiagram`" + ` here — those are syntax tokens that go inside ` + "`diagram.body`" + `.
- ` + "`diagram.language`" + ` = ` + "`mermaid`" + ` (the only currently rendered subset).
- ` + "`diagram.body`" + ` carries the actual Mermaid source. Use ` + "`flowchart TD`" + ` by default for ` + "`flow`" + ` / ` + "`architecture`" + ` / ` + "`call_dag`" + ` families (switch to LR / RL / BT only when that genuinely improves readability); use ` + "`sequenceDiagram`" + ` for ` + "`sequence`" + ` family and keep participant labels short because actors render horizontally.

The contract validator rejects ` + "`kind=flow`" + ` when the family's contract expected ` + "`kind=architecture`" + ` (and similar mismatches).

### ` + types.SectionDiagramEdgeLabelVocabulary + `

` + BuildDiagramRelationContractDoc() + `

## Citation pool

- doc-level ` + "`citations[]`" + ` is a shared zero-based array of ` + "`{file, line, quote?}`" + `
- Every ` + "`citation_ref`" + ` lives on ` + "`items[i].citation_ref`" + ` (top-level on each item) as an integer index into ` + "`citations[]`" + `, or -1 when no citation backs that entry. For scalar / decision blocks where the literal / verdict sits in block ` + "`text`" + `, attach a single-element ` + "`items=[{citation_ref:N}]`" + ` to anchor the cite. ` + "`citation_ref`" + ` NEVER appears inside a ` + "`claim_use`" + ` / ` + "`claim_uses`" + ` object — that form is rejected with ` + "`unknown field \"citation_ref\"`" + `.
- Every citations[i].file MUST be a repo-relative path and MUST NOT live inside the per-trace WorkDir (blob directory)
- Every citations[i].line MUST be > 0 — line-hallucination guard
- quote is OPTIONAL. Only set it when you can paste the literal source characters that appear at file:line on the read_file gutter — exact punctuation, exact identifiers, the same language as the source file. The grounder cross-checks quote tokens against the cited line text; a quote whose identifier tokens do not overlap is AUTOMATICALLY CLEARED. So: paste the literal line verbatim, or leave quote empty. Natural-language summaries, paraphrases, rationale ("stated that …", "shows how …", "used for …") belong in the 'summary' field, never in 'quote' — they will be stripped before the answer ships.

## Enumeration completeness and bounded sets

When an ordered_list block enumerates a closed set, the answer document does NOT carry a separate completeness field. The completeness authority already comes from the extraction slate and the user section's Requested Set Boundary / completeness obligation. Your job in finalization is:
- preserve every grounded member already established by the prior slate / required-member floor
- if the set is only a lower bound or remains unknown, disclose that bound in prose or a ` + "`caveat`" + ` block instead of inventing a retired payload field
- never silently drop a required member just to make the list look tidy

## Length

Accuracy and clarity come first. Do NOT self-shorten below what the answer genuinely needs; a terse lead-in on a subtle question is a regression. Any hard length ceiling is applied by the tool at submit time and will be reported back as a concrete limit if you exceed it — no need to pre-estimate.

## Exact resolution

If the user section includes an Exact Resolution Contract block, you MUST also fill ` + "`exact_resolution{status, anchor?, context_mode}`" + `. The system validates that object against grounded evidence / current absence state and renders the exact-resolution lead deterministically; do not rely on substitute wording in prose to communicate the status.

## Per-block prose guidance

- ` + "`summary`" + ` as the only principal block — the summary block's ` + "`text`" + ` IS the answer body. Write a thorough multi-paragraph explanation that fully addresses the user's question: mechanism details, code-level specifics, cross-file relationships. Organize with ### sub-headers when covering multiple sub-topics. Depth matches the question's depth — a shallow question yields a short answer, a deep one yields a deep answer.
- ` + "`ordered_list`" + ` as a hop chain — the lead summary block (or block ` + "`text`" + `) is a short natural-prose lead-in that frames what the list captures: what initiates the sequence, what terminal state it reaches, and the main guard or branch point if there is one. Avoid restating every item's description here; the items themselves own the hop-by-hop detail. When the Diagram Contract says a diagram is required, emit a separate ` + "`diagram`" + ` block.
- ` + "`ordered_list`" + ` as an enumeration slate — the lead summary block describes what the list enumerates and the terminal criterion used to pick the items, so the reader understands what kind of item each row is and why these rows belong (and none of the others). A bare one-liner works only when the list is self-explanatory. The lead block should not duplicate the per-item explanations.
- ` + "`decision`" + ` — the lead summary block sets up the decision. Put the verdict and the core reasoning together in the decision block's ` + "`text`" + ` field, and explain the mechanism at the depth the subtlety requires. A subtle decision with only a one-sentence explanation is a regression. When Diagram Contract says required, emit a separate ` + "`diagram`" + ` block.
- ` + "`scalar`" + ` — the block's ` + "`text`" + ` (or the lead summary block) disambiguates and contextualizes the literal. State what the value means, where it comes from in the call graph / config lineage, and why the cited file:line is the authoritative source. The renderer prints the literal and the cite; prose is where the MEANING lives. When Diagram Contract says required, emit a separate ` + "`diagram`" + ` block.

## Visual structure

- For a summary-only answer: organize the summary block's text with ### sub-headers when covering multiple topics or stages. Open with a plain-prose lead paragraph that states the core conclusion. Length is governed by what the conclusion requires to be complete, not by a pre-set budget.
- Diagram families and what they look like in Mermaid (always inside ` + "`diagram.body`" + ` with ` + "`diagram.language=mermaid`" + `):
    - ` + "`flow`" + ` (branches / guards)             → ` + "`flowchart TD`" + ` by default (use LR only when it materially improves readability)
    - ` + "`sequence`" + ` (actor-to-actor over time) → ` + "`sequenceDiagram`" + `; keep participant labels SHORT because actors render horizontally
    - ` + "`architecture`" + ` (layered components)   → ` + "`flowchart TD`" + ` with one node per layer
    - ` + "`call_dag`" + ` (dispatch / hop chain / fan-out) → ` + "`flowchart TD`" + ` by default; prefer vertical layout when labels are long
  REMINDER: ` + "`diagram.kind`" + ` carries the LEFT column (` + "`flow`" + `/` + "`sequence`" + `/` + "`architecture`" + `/` + "`call_dag`" + `); the RIGHT column is what goes inside ` + "`diagram.body`" + `. Do NOT confuse these two layers.

- Mermaid syntax (preferred form) — wrap the diagram in a fenced code block whose opening line is exactly three backticks followed by the word mermaid (the closing line is three backticks alone). The supported subset is FLOWCHART (direction LR / TD / RL / BT) and SEQUENCEDIAGRAM only. Other Mermaid block types (classDiagram / stateDiagram / erDiagram / gantt / mindmap / gitGraph / C4Context / architecture-beta / journey / pie) and the ` + "`subgraph`" + ` nesting construct are NOT in the rendering subset; if you emit them the renderer leaves the block as source and users read raw mermaid syntax instead of an aligned diagram. The fence itself is REQUIRED — a bare body without the surrounding fence prints as raw text. Concrete examples (copy the EXACT shape, including the opening ` + "```" + `mermaid line and the closing ` + "```" + `):

    ` + "```mermaid" + `
    flowchart TD
        analyzer --> explorer
        explorer --> validate
        explorer --> reconcile
        validate --> finalize
        reconcile --> finalize
    ` + "```" + `

    ` + "```mermaid" + `
    sequenceDiagram
        client->>service: dispatch
        service-->>client: evidence
        client->>renderer: compose
    ` + "```" + `

  Mermaid edge / label syntax — supported set (verified against the terminal renderer):
    - Edges: ` + "`-->`" + ` (directional), ` + "`---`" + ` (plain line), ` + "`==>`" + ` (thick directional), ` + "`-.->`" + ` (dotted directional). Each renders cleanly with consistent layout.
    - Edge labels: ` + "`A -->|label| B`" + ` is supported and renders the label on the arrow line. Use it for branch / dispatch arrows where the label is the discriminator (e.g. ` + "`switch -->|case A| handler_a`" + `). For multi-branch fan-out the label form is preferred over inventing intermediate router nodes.
    - Multi-word node labels MUST be bracketed: ` + "`A[Analyzer Stage]`" + ` not ` + "`A Analyzer Stage`" + `.
    - Labels with special chars (` + "`:`" + ` ` + "`;`" + ` ` + "`(`" + ` ` + "`)`" + ` ` + "`{`" + ` ` + "`}`" + `) MUST be double-quoted: ` + "`A[\"Read file (cached)\"]`" + `. Otherwise the parser splits at the special char.
    - Node ids stay short single tokens (letters / digits / underscore); the human-readable label lives in ` + "`[...]`" + `.
    - ` + "`:::className`" + ` class selectors are accepted but produce no visual change in the terminal renderer; omit them — they add tokens without effect.
  CJK / Hiragana / Katakana / Hangul / full-width labels are SUPPORTED inside Mermaid (the renderer applies a CJK width adapter) — column counting is not your responsibility. ALWAYS choose Mermaid (never ASCII art) when labels contain wide-display characters; ASCII art alignment WILL break.

- ASCII art (fallback form) — use only when the diagram cannot be expressed in the Mermaid subset above (very rare; flowchart covers nearly every flow / architecture / call-DAG shape). Fence with three backticks alone (` + "```" + `). Use ONLY these CONNECTOR characters for lines and arrows: ` + "`+ - | > < v ^ .`" + ` — node label TEXT (letters / digits / spaces / brackets / inline punctuation) is unrestricted, the connector whitelist constrains the drawing chars only. Do NOT use ┌ ┐ └ ┘ ┼ ├ ┤ ┬ ┴ ─ │ or ▶ ◀ ▲ ▼ — those render at different widths across terminals and produce skewed output. Do NOT use ASCII art when any label contains CJK / Hiragana / Katakana / Hangul / full-width characters — column arithmetic breaks because wide-display chars occupy 2 cells while connectors occupy 1, and the rows mis-align.

- Mermaid blocks: alignment is automatic regardless of label content. Inside ASCII art (fallback form), every drawn box / arrow occupies a fixed cell count YOU lay out, so a stray wide-display character corrupts the entire grid. When in doubt, prefer the Mermaid form.
- For a hop-chain ordered_list answer: the renderer emits ONLY the numbered detailed list of items — it does NOT draw any flow / DAG / sequence diagram for you. When the Diagram Contract says required, emit a separate ` + "`diagram`" + ` block. Even when the Diagram Contract does NOT require one, add a small grounded diagram block only when it directly serves the user's requested answer and every node / edge is grounded. Do not add a diagram as generic enrichment, and do not let a diagram replace the block shape the user actually asked for (scalar, list, comparison, verdict, or prose explanation).
- Use ` + "`table`" + ` blocks (or markdown tables inside section / summary text) when comparing attributes across entities (e.g. "| Entity | Attribute | Value |") — keep column headers abstract; do not copy the question's literal entities or predicates into the headers.
- Use inline code backticks for every function name, type name, and file path — never write them as plain text.

Prose voice — the answer must read as natural explanation, not a call-graph transcript. This applies to every LLM-authored prose field: every block's ` + "`text`" + `, every item's ` + "`text`" + `, decision block prose, summary block prose.
- Describe what the code DOES and why it matters, not the raw relation between functions (who calls whom at which line). A step body or a rationale is a mechanism description, not a machine-readable edge list.
- Prefer active voice over nominalizations. Natural: "the handler validates the session token and falls back to a refresh when the signature is stale." Mechanical: "a validation of the session token is performed with a subsequent invocation of the refresh path."
- Use connective tissue between clauses that belong together (first / then / when this fails / once … / …), but do not stuff three logical hops into one run-on sentence.
- Reference every identifier, type, and path with inline code ticks — this helps the reader and keeps your identifiers aligned with the files and lines in your citations.
- Refer to what the user named using the user's own words. When the question names its subject (a document title, a feature name, a component label), reuse that exact wording wherever the answer refers to it — translated only when the answer is in a different language than the question — rather than substituting a synonym or your own rebranding. An answer that renames its subject reads as answering a different question.
- Keep internal pipeline jargon out of the user-facing prose unless the user explicitly asked about the framework itself. Do not surface tool / contract words such as "grounded", "grep", "read_file", "repo_map", "emit_*", "exact_resolution", "context_mode", "citation_ref", or "citations[]"; state the observed fact directly instead. For command/VCS answers, say the information comes from repository history / command output rather than exposing the no-citation sentinel.
- When the answer is about code behavior, the reader already knows code is involved. Do not open with "This code does X" or "The function does Y" — open with what actually happens.
- Regression patterns to avoid (these all produce mechanical, non-explanatory prose):
    - "foo is called at line 42, which calls bar at line 58 which calls baz at line 71." — reproduces the call graph but does not explain the mechanism.
    - "Defined at foo.go:42. Used by bar." — duplicates the location column and adds no semantic content.
    - "Line N shows that X." — the reader cannot open source from here; describe what X IS, then let the citation stand for the evidence.
    - "'grep' / 'read_file' / 'repo_map' found nothing." — reports the investigation tool instead of the observed fact. Say "the YAML file does not define this key" or "the repository has no matching file" instead.
    - "... is the abstract interface (path/to/file.ext:8)." — duplicates the cite already attached via items[i].citation_ref; remove the trailing parenthetical and let the typed channel render the file:line. Prose-embedded (file:N) parentheticals also bypass the facet-anchor slot, so the facet stays declared-but-unanchored even when the cite renders correctly.

Caveats field: an optional string array for honesty markers. When writing caveats, use the same language as the user's question.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item. A file path that came only from VCS changed-file output (git_show / git_diff / git_log --stat / --name-only) is NOT a read line: cite it at file scope or leave it uncited as `external_observation`, never as a guessed `file:1`.",
			"do not put prose / summaries / rationale in the citation quote field — quote must be a verbatim copy of the source line or empty; the grounder auto-clears mismatches",
			"do not set citation_ref to a zero-value-looking placeholder; use a valid pool index only when a real citation exists, otherwise use the schema's explicit no-citation carrier",
			"do not silently truncate a user-bounded or exhaustive set. If you cannot honestly render every required principal item, disclose the bound in prose / a caveat block instead of inventing extra members or omitting the gap.",
			"do not omit claim annotations on a non-decision principal block whose user-section contract lists allowed `claim_form` values — the validator will reject the emit and the retry hint will name the missing block id. For typed decision blocks carrying `current_status_verdict` or `error_granularity_verdict`, the verdict field is the carrier; do not guess a `claim_uses[]` form. When a claim annotation is required, attach block-level `claim_uses=[{claim_form=definition_fact}]` (plural array — block level has no singular form) and put the citation index on `items[i].citation_ref`, NEVER inside the claim_use object.",
			"do not write Mermaid keywords (`flowchart` / `sequenceDiagram`) in `diagram.kind` — that field carries the SEMANTIC family (`flow` / `sequence` / `architecture` / `call_dag`). Mermaid syntax goes inside `diagram.body`.",
		},
		// P5-B (2026-05-10) — Tier B applicability-gated workflow
		// items. These are the 6 Workflow rules the design doc
		// classifies as style-polish / specialized-applicability.
		// Bodies are kept VERBATIM from the pre-P5 single-string
		// list (no rewording). Each item carries an AppliesToFilter
		// describing when the rule is relevant; OnViolation lists
		// ViolKinds that force the rule to render on retry even
		// when the AppliesTo gate would hide it.
		// Design doc: docs/design/finalizer_skill_restructure.md §3.1.
		WorkflowTierB: []TierBItem{
			{
				// W5: edge_anchors — only meaningful with diagram blocks.
				Body:      "`edge_anchors` is the OPTIONAL block-level array for diagram-edge typed anchors. Use it when this block contributes evidence about a labelled directed relation in a diagram (typically when the block IS the diagram, or when its items describe edge endpoints of a diagram in a sibling block). Each entry is a typed edge object: `{from_node: string, to_node: string, relation_kind?: <one of call|guard|import|precedence|contain|observe>}`. Both `from_node` and `to_node` MUST be the verbatim node identifier strings as they appear in the diagram body. PREFERRED: set `relation_kind` directly — that field is the authoritative semantic relation, while the Mermaid edge label is free prose for readers. When omitted, the validator falls back to recognising the relation from the rendered edge-label vocabulary. Example: `{from_node:\"Auth\", to_node:\"Worker\", relation_kind:\"call\"}` for the labelled edge `Auth -->|invoke| Worker`. Empty / absent = no edge anchor on this block (legitimate for non-diagram-edge blocks). Edge anchors NEVER live inside a claim_use object — they are a separate top-level array.",
				AppliesTo: AppliesToFilter{RequiresDiagram: true},
				OnViolation: []types.ViolationKind{
					types.ViolDiagramEdgeUnsupported,
					types.ViolDiagramEdgeLabelMismatch,
					types.ViolDiagramRelationLabelOnly,
				},
			},
			{
				// W8: abstraction-level matching — enumeration-shape only.
				Body: "Abstraction-level matching (REQUIRED whenever an enumeration's items must describe a role / responsibility / purpose — the question form 'what does each X do' / 'what is each X for' / 'why is each X separate' / '每个 X 负责什么' / '每个 X 干什么' / '每个 X 的作用'): each item's description must answer that question DIRECTLY at the conceptual level the user asked. A description like \"X is responsible for <conceptual outcome>\" is the ANSWER; a description like \"X calls Y which builds Z and writes W\" is the IMPLEMENTATION CHAIN — implementation chains are GROUNDING that anchors the conceptual answer to verifiable code references, not a substitute for it. A reader who needs to mentally compose \"X calls Y which builds Z, therefore X must do <something>\" is doing the synthesis you owed them — answer the conceptual outcome directly, then ground with the chain. Pair both for richness (conceptual responsibility PLUS engineering anchors); cognitive-only is fine if the question doesn't ask how; engineering-only without naming the responsibility is a regression on this question shape. This rule does NOT apply when the question explicitly asks 'how does each X work' / 'what does each X call' / mechanism-shape enumerations — those legitimately want the implementation chain itself.",
				AppliesTo: AppliesToFilter{
					Intents:        []types.Intent{types.IntentEnumerate},
					PrincipalKinds: []types.AnswerBlockKind{types.BlockOrderedList},
				},
				OnViolation: []types.ViolationKind{
					types.ViolPrincipalProseUnderfilled,
					types.ViolAnswerSemanticUnderfilled,
				},
			},
			{
				// W16: log-triage hop-chain preference — log-only.
				Body:      "For log-triage questions (the user attached a panic / exception / sanitizer diagnostic / traceback) prefer a hop-chain ordered_list with each item pointing at one stack frame or one code-level cause; start from the innermost frame and work outward so the reader sees the failure site first.",
				AppliesTo: AppliesToFilter{RequiresLog: true},
			},
			{
				// W18: sealed-seed rule — only when log + diagram seed present.
				Body:      "Sealed-seed rule for diagram anchors: when the prompt's \"Call chain (innermost → outer)\" section contains a ```mermaid``` fenced diagram, the file:line tokens inside that seed are AUTHORITATIVE FACTS about the failure event (which file at which line) — copy each of them VERBATIM into your final diagram. Do NOT replace, recompute, or reuse line numbers based on what you read in the current source. The runtime binary that produced the log may be from an older or newer build than the source you can read with read_file, so the seed's :line and the current source's :line for the same function will often differ — that drift is itself a ground-truth fact about the failure (the runtime had THIS line layout when the event happened) and belongs verbatim in the diagram. If you need to discuss the drift, do so in PROSE (e.g. \"the stack lists :250 for the failing function; current source has the same function at :340 with the guard added\") — never silently update the diagram. You MAY add caller/callee nodes (each with its own grounded file:line from citations[]), rename role labels, or change diagram form (flowchart ↔ sequenceDiagram), but you MUST NOT alter a file:line that appeared in the seed. Reusing a single line number across multiple nodes is also forbidden — each frame in the seed has its OWN distinct line; if the seed shows N distinct lines, your diagram must have N distinct lines.",
				AppliesTo: AppliesToFilter{RequiresLog: true, RequiresDiagram: true},
			},
			{
				// W20: subject discipline — always relevant, lower priority.
				Body:      "Subject discipline: summary content must stay relevant to the question's subject — what the question names, attaches, or points at (a symbol, attached content, a specific behaviour). A file the investigation consulted is content only when it answers the question (it IS the subject, or the subject's root cause, or the mechanism the question asks about); it is noise when it surfaced by identifier collision (same name in an unrelated test fixture, a look-alike symbol in another package) or was traversed for grounding but does not itself answer the question. Judge by relevance to the subject, not by whether it is 'code' or 'tooling'.",
				AppliesTo: AppliesToFilter{Always: true},
				OnViolation: []types.ViolationKind{
					types.ViolAnswerSemanticUnderfilled,
					types.ViolAnswerTopicMismatch,
				},
			},
			{
				// W21: authority discipline — log-drift-bounded only.
				Body:      "Authority discipline (drift-bounded answers): when the attached log / perf trace observed an anchor that has DRIFTED relative to current code (function moved, lines shifted, symbol renamed), give the FULL mechanism explanation but ATTRIBUTE the two sides explicitly: 'the log observed X at line N; the current code has the corresponding function at line M and now reads Y; the failure path is therefore Z'. This is the SAME depth and richness you would write for non-drift answers — drift is a provenance fact, not a reason to retreat into vague language. DO NOT write `[hedged]` / `[historical]` / `[illustrative]` markers in any prose field — those are reserved tokens that get stripped from your output and re-injected automatically around the relevant anchors plus a single Authority caveat at the bottom. Writing them yourself wastes tokens and adds no signal. On retry, ignore any markers you see in your prior draft (they were not part of your output) and focus on enriching the mechanism explanation. Pitfalls to avoid: (a) collapsing 'log saw X' + 'current code has Y' into 'X directly causes Y' without naming both sides; (b) hedging the substance ('X may possibly be related to Y, perhaps') instead of attributing the provenance; (c) shortening the answer because a caveat appears — the caveat doesn't substitute for your mechanism explanation; (d) writing the reserved markers yourself.",
				AppliesTo: AppliesToFilter{RequiresLog: true},
				OnViolation: []types.ViolationKind{
					types.ViolAuthorityOverreach,
				},
			},
		},
		// P5-B Tier B prohibitions: 2 items the design classifies
		// as style-polish (always relevant but lower priority).
		// Bodies kept verbatim. Both Always=true means they always
		// render in the Tier B section but get the lower-priority
		// position so the LLM's attention stays on Tier A first.
		ProhibitionsTierB: []TierBItem{
			{
				// P5: don't pre-shrink prose — soft length advice.
				Body:      "do not pre-shrink prose for any block — write what accuracy and clarity require; if a hard length ceiling is in force the tool will reject the call with the exact limit, so there is no need to pre-estimate",
				AppliesTo: AppliesToFilter{Always: true},
			},
			{
				// P8: don't invent codename labels — soft style.
				Body:      "do not invent short codename labels (short alphanumeric tokens like `S1` / `F2` / `Stage-N`) in any prose. These labels are source-level identifiers — if you write one, it MUST appear verbatim in at least one citation's line range. Do NOT extrapolate by pattern (the existence of `Fallback S1` in source does NOT imply `S2` exists). When in doubt, describe the mechanism by its actual behavior instead of naming it with a made-up label.",
				AppliesTo: AppliesToFilter{Always: true},
			},
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
		Goal: "Produce the answer-symbol slate and the per-hypothesis verdicts from the completed investigation record. Evidence has already been collected — this pass never re-emits it. This pass has two unique jobs: (1) LLM-driven answer_symbol selection with a completeness claim that is cross-checked later, and (2) LLM-driven hypothesis judgement with a citation.",
		Workflow: []string{
			"Self-reference trap (READ FIRST): if a candidate answer_symbol literal equals the question's PRIMARY ENTITY name, it is a self-reference, NOT the answer. Consider a generic question 'which Y does the X use?' — the primary entity is X and the answer must be a DIFFERENT identifier mapped to X; emitting symbol name=X resolves X→X, which is never an attribute lookup; it is the question subject echoing itself. Never emit answer_symbol.items[i].name where name equals the primary entity token.",
			"Read the investigation summary provided as a user section: user question, investigation notes, read files, top evidence items, dataflow findings, expected answer count, and hypothesis set. The expected answer count drives the completeness claim below; see Output Format for its exact meaning and how it interacts with symbols_completeness.",
			"Direction check (CRITICAL for an enumeration slate): before emitting, identify what TYPE of entity the question asks about by reading the SUBJECT NOUN. Every item in the `emit_answer_symbol.items[]` slate MUST be an instance of that type. If your candidate symbol is a verb-phrase helper whose role is to CREATE, REGISTER, CONFIGURE, or WIRE UP an instance of the subject type — rather than to BE such an instance — you are emitting the MECHANISM, not the answer: STOP, walk the Resolution Chains back to the terminal symbol or literal the mechanism resolves to, and emit ONLY the instances that terminal names. The same rule in one line: the answer is the terminal that the chain RESOLVES TO, not any intermediate node on the chain.",
			"For the answer-symbol slate: call emit_answer_symbol ONCE with a batched items array only when the current user-section does NOT explicitly say `This dispatch does NOT require emit_answer_symbol`, and EITHER (a) the question is a true enumeration / closed-set list question whose principal set has NOT already been accepted as an aggregate_facts.member_set for final rendering, OR (b) the current user-section explicitly renders an `Anchor skeleton (one per sub-topic)` block — in that typed path, emit ONE anchor symbol per sub-topic as a skeleton the final answer's prose hangs on and name the sub-topic in rationale, OR (c) the user explicitly declared a bounded principal set inside a mechanism / hop-chain question (for example 'the 7 checks'). Plain single-topic call-chain / root-cause / mechanism questions WITHOUT case (b) or (c) do NOT use emit_answer_symbol — their principal payload is rendered later as ordered_list / diagram / prose blocks, not as an answer-symbol slate. Classification sub_topics alone are guidance, not a hard slate obligation. In case (c), emit the principal member slate first so later hop-chain answer writing can lock onto that bounded set instead of re-guessing it from adjacent context. **When the user-section's Requested Set Boundary block declares an explicit count N (this is a typed signal — see that block in your input), case (c) is ACTIVE and the skip path is NOT available; you MUST emit a slate of N items.** Each item MUST carry a concrete grounded file:line from the read gutter or deterministic evidence anchors — never invent a line number. See the Completeness honesty contract in OutputFormat for the completeness claim rules; for the typed Anchor skeleton path completeness is not required (the anchors are auxiliary to the prose summary)",
			"For every hypothesis in the hypothesis set: call emit_hypothesis_verdict once with hypothesis_id + status + rationale + citation or evidence_id when available. Prefer `evidence_id` when the accepted investigation snapshot already contains grounded evidence for the verdict; use handwritten `citation` only for a single exact repo file:line or exact external artifact anchor. Status must be 'confirmed' / 'rejected' / 'inconclusive'. Current-repo 'confirmed' and 'rejected' verdicts require either a repo file:line citation or an evidence_id from an accepted grounded deterministic evidence row. For an observation-only external log / trace, an exact runtime-frame file:line OR artifact-local gutter line such as `log:3`, `trace:5-6`, or `runtime_artifact:1-5` is acceptable and will be treated as artifact context, not repo proof. Use 'inconclusive' when neither grounded repo evidence nor an exact artifact anchor exists.",
			"Log-triage grounding: when the Log Triage section renders a \"Call chain (innermost → outer)\" block or per-frame file:line anchors, the frames ARE the authoritative file set for the failure call path. emit_answer_symbol items for log-triage questions MUST come from those frames or from investigation evidence that directly cites them — do NOT pull symbols from files the keyword ranker flagged as \"Auxiliary candidates\" unless the investigation produced an evidence item anchored there. This feeds the later diagram-grounding check, which rejects a call-chain diagram that names files the frames + citations did not observe.",
			"External-source log escape: when the Log Triage section carries the '⚠ External-source log' banner (resolved_files=0 because the attached log's frames did not resolve to any repo file), the emit_answer_symbol channel cannot be satisfied — every item requires a concrete repo-grounded file:line, which external-log identifiers do not have. Set symbols_completeness=\"unknown\" and OMIT items[] entirely; the final answer summary carries the answer. Do NOT manufacture file:line anchors for log-message keys or exception names that have no repo counterpart — the tool rejects line=0 or empty-file items and will redirect to this escape on every retry until you take it. emit_hypothesis_verdict is different: if the hypothesis verdict is supported by an exact runtime-frame file:line or artifact-local gutter line (`log:3`, `trace:5-6`, `runtime_artifact:1-5`) from the attached runtime artifact, you may put that artifact anchor in citation; the tool preserves it as artifact context instead of repo proof.",
		},
		ToolSuggestions: []string{
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
		OutputFormat: `Your contribution is the emit_* tool calls — final rendering reads the structured buffers, not your prose. Call each emit_* tool AT MOST ONCE for this pass; batch all items in a single call per tool. When both emit_answer_symbol and emit_hypothesis_verdict apply here, invoke them IN PARALLEL within the SAME assistant response (multiple tool_use blocks in one turn), not sequentially across iterations. A split batch wastes a round-trip and risks losing the second tool to the iteration cap. Do NOT write tool-call JSON in your text — use the function-calling mechanism only.

Completeness honesty contract for emit_answer_symbol:
- "complete" — you assert this list enumerates EVERY symbol that answers the question. A "complete" claim cross-checks against the expected answer count (the larger of: how many items the investigation found, and how many the classification declared required); a short list is DOWNGRADED to "lower_bound" with a warning.
- "lower_bound" — these symbols are all confirmed present, but additional symbols may also be part of the answer. Final rendering will present a softened "at least these, may add more" prompt. This is the HONEST DEFAULT when you cannot confidently reach the floor.
- "unknown" — you investigated but cannot reach a definitive slate. The answer-symbol section is omitted entirely and final rendering answers from prose / blocks only. Choose this for mechanism questions, value/boolean questions, or genuinely ambiguous evidence.

Two-axis enumeration contract: when the user asks for every principal member plus a per-member attribute (for example 'all X and each X's Y'), emit the principal members as items[]. The completeness claim is about the principal member set, not about whether every attribute is known. Put each member's attribute in rationale with a grounded file:line when available; if the attribute is missing or ambiguous for a member, keep the member and state that unresolved attribute in rationale / final caveat instead of downgrading the whole member set to unknown.`,
		Prohibitions: []string{
			"the investigation transcript is frozen — this pass has no file access and must not re-emit evidence",
			"do not invent line numbers — if the transcript does not have a line for a symbol, omit that symbol",
			"do not cite files outside the investigation's read-files gutter or accepted grounded deterministic evidence anchors as repo proof. The only exception is emit_hypothesis_verdict for observation-only external log / trace questions: an exact attached runtime-frame file:line or artifact-local gutter line (`log:3`, `trace:5-6`, `runtime_artifact:1-5`) may be used as artifact context.",
			"do not fabricate an answer-symbol list — for single-topic mechanism / call-chain / root-cause / value / boolean questions WITHOUT an explicit bounded principal set, skip emit_answer_symbol. These answers only emit answer_symbol when the current user-section explicitly renders an Anchor skeleton block or when the user declared a bounded principal set that needs a locked member slate. Analyzer sub_topics by themselves are not enough. The skip path is NOT a recovery option after an emit_answer_symbol rejection: when the rejection cites a declared_count from the user's question, fix the slate to match (do NOT abandon emit_answer_symbol by pretending the question is exempt from the typed Requested Set Boundary — that boundary already establishes case (c) above).",
			"do not claim completeness=complete unless len(items) >= the expected answer count — a short claim is auto-downgraded",
			"do not choose hypothesis status=confirmed or rejected without either a current-repo file:line citation, an evidence_id from an accepted grounded deterministic evidence row, or an exact external runtime-frame / artifact-local gutter line from the attached artifact — use 'inconclusive' when none exists",
		},
	})

	// The log_triager skill. The agent reads the user-attached runtime
	// log and emits ONE emit_log_triage call with a structured view:
	// layer 1 (Meta — lang, signals, summary), layer 2 (Errors — type,
	// message, frames, optional recursive Cause chain), layer 3
	// (Observations + Residue — typed non-stack facts plus
	// unknown_chunks). The system validates paths against the
	// repository and derives layer 4 (resolved_files, entities,
	// intent_hint, coverage) automatically — the LLM cannot fill
	// layer 4 because those fields are not in the tool's JSON schema.
	//
	// Tool allowlist is intentionally narrow: the typed emit tool plus
	// optional attachment pagination for blob-backed logs. No repo_map /
	// list_files / grep — the LLM does not attempt path resolution; the
	// system does that deterministically post-emit.
	r.Register(&Config{
		Name: "log-triage-skill",
		Goal: "Read the attached runtime log and emit a structured triage bundle (errors + frames + signals + operational observations + residue) via emit_log_triage exactly once. Path validation and the resolved-files list are derived automatically from your emission — focus on extraction, not resolution.",
		Workflow: []string{
			"Read the attached runtime log from the 'Attached Runtime Log' section of the user prompt. If the current tool schema includes an attachment-pagination read tool for an oversized blob, use line_offset/limit to paginate through the middle; otherwise the visible inline section is the full available body for this dispatch. line_offset is line-based, not byte-based. Multi-file attachments embed `# codrax-source: <path>` headers between bodies; treat each segment as an independent capture (per-process panic, per-time-window snapshot) when grouping errors.",
			"Identify every stack frame that names a source file with a line number. For each frame, emit: file (as it appears in the log — do NOT normalize paths; path normalization is automatic), line, func (best available identifier), pkg (module/namespace hint when obvious), lang, raw (the original log line for this frame, required), confidence (0.0-1.0, your certainty the frame is real).",
			"Group frames under the error they belong to. Emit errors[] with one entry per logical error (per goroutine in a Go panic dump, per exception in a multi-exception traceback). For each error set type (exception class / panic type), message (the human-readable text), and frames[] (the stack for THIS error).",
			"Chain causal errors via the cause pointer. When the log shows 'Caused by:' (Java), 'during handling of' (Python __cause__ / __context__), or '#[source]' (Rust), nest the upstream error in the cause field. Keep the chain shallow — practical depth 3 or so; depth is capped at 5.",
			"Set meta.lang to the dominant language (go/java/cpp/python/node/rust/ruby/csharp/kotlin/arkts/cangjie/unknown/other). ArkTS is HarmonyOS UI code in .ets / .ts files with V8-style stack frames; Cangjie is HarmonyOS native code in .cj files with JVM-like frames `at demo.cart.Cart.method(Cart.cj:42)`; Kotlin is JVM code in .kt files with frames `at com.example.Foo$bar(Foo.kt:42)`. Two tag-formatted log families share one parser path: hilog (HarmonyOS) lines look like `01-26 11:01:06.870 1051 1051 W 00201/test: message`; Android logcat lines look like `04-15 14:32:18.421  5821  5821 E JsApp: message` — structurally identical: <timestamp> <pid> <tid> <level> <tag>: <body>. Extract the body portion while keeping the full line in the frame's raw field. Set meta.signals from the canonical enum (panic/crash/oom/timeout/permission/db/network/validation/logic/performance/other) by matching the observed symptoms. The 'performance' signal covers operational-but-slow patterns the log itself describes: 'slow API call took 5s', 'frame skipped' / 'Choreographer dropped N frames', 'GC pause 800ms', 'blocked on lock 2s' — distinct from 'timeout' (no operation was cancelled, just slow) and from performance-trace jank signals. Multiple signals are OK when the log describes compound failures. Summary is an optional one-line synopsis. PERFORMANCE traces (HiTrace/atrace/systrace/perfetto) are handled by a SEPARATE performance-trace parser via emit_perf_trace — do NOT attempt to parse trace events here; if the attached text contains `tracing_mark_write: B|pid|tag` events, return an empty errors list with signals=[] so the trace parser can handle it.",
			"Operational observations that do NOT form an exception tree but DO describe a process fact go into observations[] instead of unknown_chunks. Examples by shape, not by keyword: a validator/reviewer rejected an answer, an attempt was retried or forced to rewrite, the answer topic diverged from the requested topic, a file/line/source mapping drifted, an attachment could not be decoded/resolved, a log level/event appears at a specific attached-log line, a named event is absent from a bounded log window, or the system changed mode/state. For each observation choose kind from the enum, set severity (info/warning/failure when clear), set diagnostic=true only when the observation describes a failure/regression/mismatch/current-risk target rather than neutral context, include a concise summary plus short evidence excerpt, and set line_start/line_end when the observation is anchored to the attached log's `N│` gutter. These line fields are artifact-local log anchors, not repo citations. A stack caller frame proves call order/context only; do NOT emit an observation summary saying that caller supplied a bad value, constructed upstream data, or owns the failing variable unless the log line literally states that fact.",
			"Log chunks that do NOT structure into an error or observation (build noise, log-level prefixes, unrelated debug output, truncation markers) go into unknown_chunks. Do not punt everything there — only genuinely unparseable pieces. Each chunk capped at 500 chars, at most 8 chunks total.",
			"Frames with uncertain line numbers or uncertain file paths should be emitted with line=0 or file='' and confidence < 0.5 — such frames stay in the bundle but are NOT added to the repo file list. Zero information loss on partials.",
		},
		ToolSuggestions: []string{
			"read_file", // blob pagination only
			"emit_log_triage",
		},
		OutputFormat: `You have ONE required emit tool: emit_log_triage. The current tool schema is authoritative for whether an attachment-pagination read tool is available; use only listed tools. You do NOT have grep / repo_map / list_files — path resolution is handled automatically.

Schema in one glance:
- meta.lang        (required) — the dominant runtime language
- meta.signals[]   (required, may be empty) — what-went-wrong enum values
- meta.summary     (optional) — one-line synopsis, ≤ 200 chars
- errors[]         (required, may be empty) — array of { type, message?, frames[], cause? }
- errors[].frames[] — { lang?, file?, line?, func?, pkg?, raw (required), confidence (required) }
- errors[].cause   — recursive error (same shape); for linear causal chains like Java Caused-by
- observations[]   (optional, ≤8) — non-stack operational facts:
  { kind, severity?, subject?, summary, evidence?, line_start?, line_end?, diagnostic, confidence }
  kind enum: runtime_event / contract_violation / retry_cycle / topic_mismatch / line_mapping / artifact_gap / state_change / performance_symptom / other
  severity enum: info / warning / failure
- unknown_chunks[] (optional) — ≤ 8 strings ≤ 500 chars each, for unstructurable text

Caps (enforced by the tool):
- entities: 32
- resolved_files: 10
- cause depth: 5

Build-machine path prefixes (e.g. /build/*, /home/user/src/*), Java basename resolution, repo-existence filtering, and runtime-internal frame filtering (Go stdlib, node: URIs, java.base/*) are applied automatically to the file paths you emit — keep paths AS THEY APPEAR in the log. resolved_files / entities / intent_hint / coverage are derived from your emission and MUST NOT appear in your output.

You emit exactly one emit_log_triage call per dispatch. Do NOT write prose — the bundle is the deliverable.`,
		Prohibitions: []string{
			"do NOT resolve file paths yourself — emit frames with the file path AS IT APPEARS in the log; build-prefix stripping, Java basename resolution, repo-existence filtering, and runtime-internal filtering are automatic",
			"do NOT use grep / list_files / repo_map / exec_command — your available tools exclude them; path verification is automatic",
			"do NOT invent frames that are not in the log — a frame with confidence 0.9 must correspond to a real log line whose text you can paste into the raw field",
			"do NOT bury structured process facts in unknown_chunks — if the log supports a non-stack observation, emit observations[] with the typed kind and confidence",
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
		Goal: "Read the attached HiTrace / atrace / systrace / perfetto text excerpt and emit a structured PerfBundle (frames + janks + stalls + startup + observations) via emit_perf_trace exactly once. Identify janky frames whose duration exceeds ~16.6 ms on the UI thread (the 60-fps frame budget) and attribute them to the innermost tracing_mark_write B|…|tag span that was open at the jank start.",
		Workflow: []string{
			"Read the attached trace from the 'Attached Performance Trace' section. The same channel carries HiTrace (HarmonyOS hdc), atrace / systrace (Android adb), and perfetto text dumps — meta.source records which one you observed. Multi-file attachments embed `# codrax-source: <path>` headers between bodies; treat each segment as an independent capture (different process / time window / device) when correlating janks. If the current tool schema includes an attachment-pagination read tool for an oversized blob, use line_offset/limit to paginate; otherwise the visible inline section is the full available body for this dispatch. line_offset is line-based, not byte-based; emit_perf_trace has no byte_start/byte_end fields.",
			"If an attached segment is `.tracebundle.json` or the prompt says tracebundle metadata was detected, treat it as a query manifest/provenance artifact rather than the raw trace body. Do not page through the JSON looking for sched_switch/tracing_mark rows, and do not conclude the systrace body is missing merely because the manifest is JSON. Emit observations for the referenced systrace/perftrace paths, provider readiness, and coverage/caveats visible in the manifest; later trace_query calls consume the tracebundle or its sibling systrace for scheduler/root-cause/perf_sample analysis.",
			"Identify the source: `# ftrace` header or `TASK-PID CPU# TIMESTAMP FUNCTION` header = systrace / atrace / hitrace (all ftrace-compatible). A textual `perfetto` banner = perfetto text dump. Set meta.source accordingly. Raw ftrace timestamps are seconds end-to-end (928.081774 = 928s + 0.081774s; six fractional digits are microsecond precision). Compute meta.duration_ms = (last_timestamp − first_timestamp) × 1000.",
			"When the attached trace is HarmonyOS / OpenHarmony / HiTrace / bytrace (including user wording such as 鸿蒙 / 东湖 / OHOS), user-space priority semantics are: larger numeric priority means higher priority; 1-40 are CFS priorities and 41-139 are RT priorities. Concrete mapping examples: prio=20 is CFS; prio=41, prio=51, and prio=52 are RT. For any concrete prio=N in the trace, recompute the class from the numeric range before writing it; if source is Android / atrace / generic ftrace, keep priority as raw scheduler priority unless the trace producer documents its mapping.",
			"Walk every `tracing_mark_write: B|<pid>|<tag>` begin and pair it with the next matching unnamed `E|<pid>` or bare `E` on the same ftrace thread stack; never search for `E|<pid>|<tag>` because B/E end rows do not repeat the tag. Pair async `S|<pid>|<tag>|<cookie>` / `F|<pid>|<tag>|<cookie>` rows by marker pid + tag + cookie. The delta is the span duration. UI-thread span tags typically prefixed with `H:` on HarmonyOS (`H:RenderService:DoFrame`, `H:Layout:measure`, `H:Drawing`, `H:DataLoader:fetchSync`) or use Android conventions (`Choreographer#doFrame`, `performTraversals`, `RenderThread`).",
			"For every UI-thread span whose duration exceeds the 60-fps frame budget (16.67 ms), emit a PerfFrame with janky=true AND a PerfJank entry: start_ts_ms = B timestamp converted to milliseconds when the raw trace timestamp is seconds; duration_ms = delta × 1000 for ftrace seconds; trigger_span = the INNERMOST tag active inside the span (whichever B| opened after the outer one and whose E| closed before the outer's E|); reason = best guess (io / lock / sync-call / heavy-compute); tags[] = the stack of tags active at the jank peak (outermost to innermost).",
			"For main-thread blocking calls longer than 100 ms, emit a PerfStall: kind = io / lock / sync-rpc / native-call (pick the most specific); symbol = the trace tag's identifier portion; file / line = when the tag's format embeds them (e.g. `fetchSync at DataLoader.ets:42`).",
			"If the trace covers a process cold-start (observable by `ActivityTaskManager`, `AppInit`, `AbilityManagerService`, `WindowStage.loadContent`), emit a PerfStartup: mode=cold (or warm/hot if evidence suggests), app_launch_ms / ability_init_ms / first_frame_ms when measurable. Over 1.2 s app_launch_ms = slow cold start.",
			"For trace facts that answer the user's question but are not jank/stall/startup, emit observations[]: use subject for the span/event/check name, summary for the answer-grade fact, line_start/line_end for artifact-local trace gutter lines, duration_ms for measured durations, tags[] for trace tags such as H:GC:Collect. Examples: GC span begins on trace line 5; GC span duration is 8ms; no GC span exceeds 50ms.",
			"Put any unstructurable chunks into residue[] (≤8 entries, ≤500 chars each). If the trace content genuinely has no jank / stall / startup signal but has relevant trace facts, emit those as observations[] plus meta.summary. Only emit residue-only when the trace is genuinely unparseable and no answer-grade observation exists.",
		},
		ToolSuggestions: []string{
			"read_file",
			"emit_perf_trace",
		},
		OutputFormat: `You have ONE required emit tool: emit_perf_trace. The current tool schema is authoritative for whether an attachment-pagination read tool is available; use only listed tools. No grep / repo_map — path resolution is automatic.

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
- observations[]     (optional, ≤50)  — { subject (req), summary (req), kind, evidence, line_start, line_end, start_ts_ms, end_ts_ms, duration_ms, tags[], confidence }
- residue[]          (optional, ≤8)   — unstructurable chunks

Derived fields (resolved_files, entities, intent_hint, coverage) are filled automatically — DO NOT include them in your emit; they are not in the schema.

Exactly one emit_perf_trace per dispatch. Prose is ignored — the bundle is the deliverable.`,
		Prohibitions: []string{
			"do NOT parse the trace as if it were a panic log — this is span-over-time data, not an exception chain",
			"do NOT use grep / list_files / repo_map — your available tools exclude them; path verification is automatic",
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
		Goal: "Segment the attached runtime log into byte-addressed regions by kind (stack / caused_by / header / context / trace / noise) so each later per-segment extraction can focus on one coherent block at a time. Emit exactly one emit_log_segmentation call.",
		Workflow: []string{
			"Read the attached runtime log. If the current tool schema includes an attachment-pagination read tool for an oversized blob, use line_offset/limit to scan the full body; otherwise segment the visible inline body. Pagination is line-based; segmentation byte_start/byte_end are raw attachment byte coordinates, so do not treat line_offset or gutter line numbers as byte positions.",
			"Walk the log top-to-bottom. Identify byte ranges that contain: a cohesive stack trace (kind=stack); a 'Caused by' / '__cause__' block (kind=caused_by); an error header or panic message line (kind=header); contextual prose around the stack (kind=context); a more general trace segment (kind=trace); or unrelated noise (kind=noise).",
			"Emit at most 10 segments. Overlap is NOT allowed — byte_end of segment N must be ≤ byte_start of segment N+1. Segments must be sorted by byte_start.",
			"Use hint field to add a short (≤80 char) description so later per-segment extraction has a pointer to what it is looking at (e.g. hint='Go goroutine 15 panic' or hint='SQLException Caused-by').",
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
		Goal: "Segment the attached HiTrace / atrace / perfetto excerpt into byte-addressed regions by kind (frame_window / jank_region / startup / thread_run / context / noise) so each later per-segment extraction can focus on one coherent block at a time. Emit exactly one emit_perf_segmentation call.",
		Workflow: []string{
			"Read the attached trace from the 'Attached Performance Trace' section. Channel covers HiTrace (HarmonyOS hdc), atrace / systrace (Android adb), and perfetto text dumps — same byte-stream regardless of capture tool. Multi-file attachments embed `# codrax-source: <path>` headers; segments should NOT cross those boundaries (one segment per capture window keeps Step B's per-segment dispatch attribution clean). If the current tool schema includes an attachment-pagination read tool for an oversized blob, use line_offset/limit to scan the full body; otherwise segment the visible inline body. Pagination is line-based; segmentation byte_start/byte_end are raw attachment byte coordinates, so do not treat line_offset or gutter line numbers as byte positions.",
			"Walk the trace top-to-bottom. Identify byte ranges that contain: a single render-frame envelope worth scrutinising (kind=frame_window — typically a B|...|H:RenderService:DoFrame ... E|... pair); a contiguous run of janky frames or stalls (kind=jank_region); a process / activity startup window (kind=startup — bounded by ActivityTaskManager / AppInit / WindowStage.loadContent on HarmonyOS, ActivityThread / Application#onCreate on Android); a long-running CPU burn or I/O block on one thread (kind=thread_run); the trace header / metadata banner (kind=context); or unrelated noise (kind=noise).",
			"Emit at most 10 segments. Overlap is NOT allowed — byte_end of segment N must be ≤ byte_start of segment N+1. Segments must be sorted by byte_start.",
			"Use the hint field for a short (≤80 char) per-segment label so later per-segment extraction knows what it is looking at (e.g. hint='cold-start ActivityTaskManager 1.2s' or hint='LazyForEach jank window 3 frames').",
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
	// Tool-surface split: write analysis/controller/planner avoid
	// generic exec_command. The planner gets typed repository read tools
	// plus typed run_tests(dry_run=true, verification_probe={...}) probes; apply/verify run inside the
	// detached worktree and keep their narrower execution tools. Safety
	// is defense in depth: stage-local tool schemas, runtime tool policy,
	// typed dry-run channels, and the git worktree sandbox all have to
	// agree before anything executes.
	//
	// L3 red line: the three write-mode emit tools (emit_change_plan,
	// apply_patch, emit_test_results) are listed in these skills but
	// each one's Execute method is structurally forbidden from calling
	// ground.BuildContext. Enforced by
	// internal/tool/write_mode_red_lines_test.go (go/ast scan at
	// build time).

	r.Register(&Config{
		Name: "write-analysis-skill",
		Goal: "Characterise the user's code-change request as a structured task description (kind / scope / risk / constraints / outcomes) so the planner has clear, framing-correct context. Read the request and inspect the repository enough to ground your judgement, then call emit_write_analysis exactly once.",
		Workflow: []string{
			"Read the user's request from the active context. The request describes a code change, not a question — your job is to characterise the work, not investigate code in depth.",
			"Inspect the repository lightly to ground your classification: call repo_map for an overview — and when the change centres on a named existing file, repo_map(view=\"edit_impact\", target_file=\"<path>\") to see what an edit there would ripple into before judging scope and risk — then read_file or list_files on directories the request mentions. Cap pre-scan at 1-2 rounds — deeper planning happens after this classification.",
			"Decide the task category (feature / bugfix / refactor / test / docs / config / misc) based on what the change actually does. A new function added to fix wrong behaviour is bugfix; a new function added to extend capability is feature; renaming or restructuring without behaviour change is refactor.",
			"Decide the scope (micro / package / cross / project) by inspecting how widely the change ripples. A one-function change in one file is micro. Multi-file work in one Go package is package. Touching unrelated subsystems is cross. Build-system or repo-wide refactor is project.",
			"Decide the risk axes from what the change touches: affects_public_api when the change adds / removes / renames any exported identifier; changes_persistence when schemas, on-disk file formats, configuration shapes, or migration files are involved; changes_build_system when go.mod / package.json / Cargo.toml / build scripts / CI configuration are touched. These axes are advisory classification — the approval gate corroborates them against typed diff and path evidence — so flag them honestly rather than defensively. Pick an overall band (low / medium / high) reflecting blast radius if the change is misapplied.",
			"Extract constraints the user explicitly stated (e.g. 'do not break existing API', 'keep the same file layout'). Skip when the user did not state any.",
			"Write 2-4 expected_outcomes — short concrete signals that the change is correctly done (e.g. 'a new --dry-run flag is wired through the CLI entry point and accepted by the existing flag parser', 'tests in <package> still pass'). These are the goal-checks the reflector will use to judge whether retries are moving toward what the user wanted.",
			"When the request or light repo inspection gives concrete observable behavior, also emit behavior_contracts[] with stable ids. Use kind/operator fields for facts like exception type, output path/layout, stdout, status code, command result, or invariant. When the evidence gives both a failing surface and a grounded working or contrasting reference surface, attach that reference as comparator on the expected contract; emit the pre-fix failure separately with polarity=observed when useful. For hard operators such as equals, not_equals, contains, exists, raises, not_raises, and returns, do not invent expected values: use a value present verbatim in the request/evidence, attach grounded comparator evidence, or choose operator=satisfies instead. operator=satisfies is soft behavior guidance; do not hide exact expected values inside satisfies text when an exact operator would be required. These atoms let later verification probes reference contract_refs instead of relying on prose.",
			"RENDERED-TEXT PLACEMENT CONTRACTS — when grounded evidence describes relative position inside rendered text, encode that relationship in behavior_contracts[].placement instead of only a global contains contract. Fill the typed surface, anchor, expected text, relation, and delimiter for boundary relations; attach placement.evidence_ref or contract evidence_ref when the anchor/expected pair came from inspected evidence. Use the same shape for repr lines, CLI/stdout lines, String()/toString output, and UI/snapshot text. A later verification probe must bind placement_refs[] to prove this placement; ordinary contract_refs[] alone is only global behavior coverage.",
			"If the change naturally splits into stages where each stage can be applied and verified before moving on (for example a schema migration before code that depends on it), propose those stages via phase_proposal with split=sequential. Otherwise omit phase_proposal or set split=single.",
			"Call emit_write_analysis exactly once with all required fields filled in.",
		},
		ToolSuggestions: []string{
			"read_file",
			"list_files",
			"repo_map",
			"grep",
			"emit_write_analysis",
		},
		OutputFormat: `Emit ONE emit_write_analysis call.

Required fields:
- raw_request          (string) — echo the user's request verbatim
- task.kind            (enum)   — feature / bugfix / refactor / test / docs / config / misc
- task.scope           (enum)   — micro / package / cross / project
- task.summary         (string, ≤100 chars) — your one-line restatement
- risk.affects_public_api    (bool)
- risk.changes_persistence   (bool)
- risk.changes_build_system  (bool)
- risk.overall         (enum)  — low / medium / high

Optional fields:
- scope_anchors[]      — repo-relative paths the change centres on
- constraints[]        — { kind, target?, note? }
- expected_outcomes[]  — 2-4 short success signals
- behavior_contracts[] — optional typed observables { id, kind, subject?, operator, expected, placement?, comparator?, evidence_ref?, required? }
- phase_proposal       — when the change splits into ordered stages
- applicable_pitfalls[] — known-pitfall IDs that clearly apply (when a pitfall list was provided in the prompt)

Prose written outside the tool call is captured in the trace but does not drive any agent — the structured fields are what matter.`,
		Prohibitions: []string{
			"do NOT modify files — analysis does not change state. exec_command is not in your allowlist for that reason.",
			"do NOT plan the actual code changes — that is the planner's job in the next stage. Your output is a task description, not a change list.",
			"do NOT invent constraints the user did not state. An empty constraints[] is normal and correct when the user said nothing about restrictions.",
			"do NOT split into phases unless each phase can independently be applied and verified before the next — speculative subdivision burns retry budget without the LLM having any way to test phase 1's effect before writing phase 2.",
			"do NOT call emit_write_analysis more than once — a second call replaces the first.",
		},
	})

	r.Register(&Config{
		Name: "write-controller-skill",
		Goal: "Choose the next bounded write workflow action from typed artifacts and emit it with emit_write_workflow_decision.",
		Workflow: []string{
			"Read the structured context sections supplied by the system: task analysis, workflow run state, current plan/report status, approval/risk record, and priority write context pack.",
			"Choose one action from the schema: explore_code, plan_batch, apply_plan, verify_batch, append_batch, split_batch, replan_batch, ask_user, finish, or block. The action is the only controller routing signal.",
			"When source understanding is missing for the current batch, emit explore_code with an exploration_request containing batch_id, goal, focused questions, candidate_paths, and evidence requirements.",
			"When the current batch is ready for a bounded ChangePlan, emit plan_batch with batch describing only that batch's goal, expected paths/kinds, and success criteria.",
			"When a typed ChangePlan is ready and the mode allows mutation, emit apply_plan. After apply succeeds, emit verify_batch so tests and structured verification decide whether the batch is complete.",
			"When typed verification evidence shows more code work is needed, emit replan_batch, split_batch, append_batch, or explore_code according to the durable workflow state.",
			"When a batch is in needs_replan with a recorded verification failure, prefer replan_batch directly — the planner receives the typed failure evidence (failing assertions, executed commands, artifact refs) as its lead section. Spend explore_code only when that evidence cannot locate the fix; every extra exploration round consumes the same step budget the remaining verify needs.",
			"When the typed artifacts show no further batch is needed, emit finish. When a structural safety or budget boundary prevents progress, emit block with reason_code and a concise reason.",
			"Call emit_write_workflow_decision exactly once.",
		},
		ToolSuggestions: []string{
			"emit_write_workflow_decision",
		},
		OutputFormat: "Emit ONE emit_write_workflow_decision call. Free-form prose is ignored by workflow routing; only the tool's structured action and payload are consumed.",
		Prohibitions: []string{
			"do NOT modify files or run commands — the controller only selects the next typed workflow action",
			"do NOT produce a code-change plan — plan_batch delegates bounded code planning to the code planner",
			"do NOT rely on narrative wording as a routing signal — routing consumes only the typed action enum and validated payload",
			"do NOT emit more than one controller decision in a dispatch",
		},
	})

	r.Register(&Config{
		Name: "change-plan-skill",
		Goal: "Produce one bounded ChangePlan for the active write workflow batch by reading the relevant source, typed context pack, and batch-local verification feedback.",
		Workflow: []string{
			"BATCH CONTEXT FIRST — read the active workflow batch goal, typed scope boundaries, priority WriteContextPack planner view, plan hints, and any verify feedback supplied by the scheduler. Treat those typed artifacts as the current dispatch boundary.",
			"BATCH-LOCAL EXPLORATION — use repo_map, grep, and read_file to collect only the source evidence needed for this batch. If a tiny runtime check is truly necessary before planning, call run_tests with dry_run=true and a typed verification_probe object; do not run suite/runner dry-runs in the planning lane. repo_map(view=\"edit_impact\", target_file=\"<path>\") on a candidate file shows what an edit there would ripple into — use it to choose the minimal target-file set before committing to changes[]. The prompt may include likely files, test surface, or context-pack evidence refs; treat them as starting points, then verify target files and symbols before emitting. When a prior controller/explorer handoff already localized the batch, use read tools only for exact current bytes or focused runtime probes, then emit the bounded plan instead of reopening broad source exploration.",
			"BOUNDED PLAN — produce the smallest useful ChangePlan that can be applied and verified before the controller chooses another action. Do not unfold a broad user request into a whole-project plan when this batch can land a smaller verified step. If investigation proves the current batch cannot satisfy its own goal without expanding scope, make that expansion explicit in summary and changes.",
			"CONTEXT PACK HANDOFF — carry P0 constraints and safety boundaries into the plan summary and acceptance_tests when relevant. Use P1 target files/symbols/invariants to choose edits. Use P2 verify failures as evidence for retry plans. Use P3 local style hints only as soft implementation guidance.",
			"KNOWN PITFALLS IN THIS REPO — when the planning context contains active pitfalls, read each trigger, check whether your draft plan would hit it, and restructure before emitting. Empty section means no relevant pitfalls.",
			"VERIFY FEEDBACK ON RETRY — when planning context contains iteration history, compact failure_signal rows, or a failure summary from an earlier attempt, treat that data as evidence. Start from failure_signal rows because they carry typed assertion/location/signal fields; read linked full failure details only when the compact signal is insufficient. Read relevant test/production code before re-emitting. Decide whether the next bounded plan should modify the test, production code, or structural wiring/config, and reflect that choice in the plan rationale.",
			"Read the files and identifiers named in the user's request. Use read_file / grep / repo_map to understand the current shape of what the change has to touch.",
			"UPSTREAM-REFERENCED FIXES — when the request points at a known existing fix (an upstream commit, pull request, issue thread, or a patch quoted in the request) and asks to apply or port it, mirror that fix's exact shape: the same expression forms, the same comparison operators, the same naming — even where a semantically equivalent variant would also pass tests. The change will be reviewed as a diff against the referenced fix, so a gratuitous deviation reads as an incorrect port. When the referenced fix itself is not readable from here (for example a URL you cannot fetch), reconstruct it conservatively: make the smallest edit that resolves the described defect while preserving each touched expression's existing operators and operands. Do not harmonize neighbouring expressions to look alike — adjacent sites that differ usually differ deliberately, and uniformizing them is a semantic change the request did not ask for. Deviate only where the local code genuinely differs from the referenced context, and state that deviation in the change's rationale.",
			"Identify the MINIMAL set of files that must be modified to address the request. A good plan touches the fewest files that still completely solves the problem.",
			"For each target file decide the Kind: create (new file; new_content is the full body), modify (overwrite existing file; new_content is the full body), delete (remove; new_content ignored), patch (surgical edit; prefer edits[] for localized line changes, use raw patch for complex diffs), or rename (move file; new_path required, new_content ignored). When the request was classified as scope=micro (single function / constant / line in one file), kind=modify on existing files is HARD-rejected at emit time — micro-scope edits MUST use kind=patch so the user reviews a line-level diff rather than a whole-file overwrite. For broader scopes (package / cross / project), kind=modify is acceptable when you're rewriting most of the file; kind=patch is still preferred when the change is surgical.",
			"STRUCTURED EDITS (kind=patch) — for localized line edits, prefer edits[] instead of hand-writing a unified diff. Each edit has kind=replace|delete|insert_before|insert_after|insert_at_eof|insert_before_final_brace. start_line is required for replace/delete/insert_before/insert_after; omit it for insert_at_eof and insert_before_final_brace. Use insert_before_final_brace ONLY for brace-language files whose current bytes contain a final standalone `}`; it is not a Python edit kind. For Python, use line-anchored insert_before/insert_after for indentation-sensitive additions, or kind=modify with the full corrected file body when the edit spans an indented block. Use insert_at_eof only for plain unindented file append or the specific safe EOF class-member case the tool accepts. optional end_line is for replace/delete, content is for replace/insert, and optional old_text must copy the exact target range or anchor line. The tool compiles edits[] against current file bytes into a unified diff and rejects stale old_text, overlap, invalid ranges, duplicate insertion points, unsupported insert kind for the current file, or no-op edits with structured diagnostics. Use exactly one patch source: either edits[] OR patch, not both.",
			"PLAN REPAIR PACKS — when any plan emit tool returns ToolResult.Repair.Code=write_plan_repair_pack, use the typed plan_repair_pack metadata as the bounded retry input: reason_code, failing_field_paths, accepted_enums, current_bytes.expected_old_text, and partial_plan_retained. If partial_plan_retained is true, re-emit only the offending file with emit_plan_change. The repair pack is soft retry guidance; validators remain the hard source of truth.",
			"RAW UNIFIED DIFF RULES (kind=patch) — use this when edits[] cannot express the change cleanly:\n" +
				"  - Three line prefixes: ' ' (CONTEXT, line stays), '-' (REMOVAL, line goes away), '+' (ADDITION, line lands).\n" +
				"  - Context lines must match the file BYTE-FOR-BYTE — same indentation (tabs vs spaces), same trailing whitespace, no edits.\n" +
				"  - To REPLACE a line, the original goes on a '-' line AND the corrected version goes on a '+' line. Do NOT keep the buggy line in CONTEXT and only add the correction with '+'. That syntactically parses but git refuses to apply because the file would now have BOTH lines.\n" +
				"  - The @@ hunk header `@@ -OLD_START,OLD_LEN +NEW_START,NEW_LEN @@` declares the file line numbers and total line counts; OLD_LEN counts ' ' + '-' lines; NEW_LEN counts ' ' + '+' lines. Off-by-one here is the second-most-common rejection cause.\n" +
				"WORKED EXAMPLE — fixing typo `retrun` → `return` on line 25 of main.go (file uses tabs; in the patch text a literal tab is shown as <TAB>):\n" +
				"  --- a/main.go\n" +
				"  +++ b/main.go\n" +
				"  @@ -23,4 +23,4 @@ func greet(name string) string {\n" +
				"   <TAB><TAB>name = \"world\"\n" +
				"   <TAB>}\n" +
				"  -<TAB>retrun fmt.Sprintf(\"Hello, %s!\", name)\n" +
				"  +<TAB>return fmt.Sprintf(\"Hello, %s!\", name)\n" +
				"   }\n" +
				"Notice: the buggy `retrun` line is on a '-' line (NOT in context); the corrected `return` line is on a '+' line; the surrounding `<TAB>}` and `}` are on context lines (' ') because they don't change. In the actual JSON `patch` field every <TAB> is a real \\t character; preserve the file's exact indentation.",
			"EMIT THE BOUNDED PLAN THROUGH ONE STRUCTURED PATH: if the whole batch plan fits in one response, call emit_change_plan once. If large file bodies or patches would create output-size pressure, call emit_plan_skeleton first, then emit_plan_change once per non-delete file. The skeleton path carries request, summary, per-file metadata, and acceptance_tests; per-file emits carry only the bounded content for that change.",
			"For SINGLE-SHOT mode: emit exactly one emit_change_plan call. request must restate the user's ask, summary must be 3-10 sentences describing what the plan does and why, and changes[] must include one entry per target file with a Rationale explaining WHY that file needs that change.",
			"For MULTI-ROUND mode: emit emit_plan_skeleton ONCE (request, summary, changes[] metadata, optional acceptance_tests). Then for EACH change with kind ∈ {create, modify, patch}, emit emit_plan_change once with that change's path and the appropriate new_content (create/modify) or patch source (kind=patch: edits[] preferred for localized edits, raw patch for complex diffs). Do not call emit_plan_change for kind=delete entries — the skeleton already declares the deletion. The order of emit_plan_change calls does not matter; the LAST one (when every non-delete slot is filled) automatically runs the full validators (dependency closure, dry-build, summary fidelity, patch pre-check) and finalizes the plan. If a finalize-time validator rejects a specific file, just re-emit that ONE file via emit_plan_change to fix it — the partial state is retained.",
			"DEPENDENCY CLOSURE — when any new_content imports a package that is NOT already in the repo's go.mod (for example you added `import \"github.com/pkg/sftp\"` and the project never used sftp before), you MUST also include a 'modify' entry for go.mod that adds the require line, AND if the project tracks go.sum, a 'modify' entry for go.sum is also expected. The validator will reject the plan otherwise — the apply phase cannot succeed without this.",
			"USER-SIDE INSTALLABLE DEPENDENCIES — when your plan introduces a third-party runtime dependency that is NOT installed automatically by `apply` (e.g. a Python `pygame` import in a fresh repo without a `pyproject.toml`/`requirements.txt`/`pip install -e .` flow; a Node package not yet in `package.json`; a system package the code shells out to such as `ffmpeg` / `imagemagick`), you MUST list it explicitly in the plan's `summary`, in a clearly-marked section that names the package, the install command for the user's likely environment (pip / npm / apt / brew / dnf / pacman / scoop / etc.), and the symptom the user will hit if they skip it (typically a `ModuleNotFoundError` / `command not found` / `ImportError` at first run). Do NOT trust the user to figure this out from imports — surface every install-required dependency by name so they can copy-paste a single command. Pure standard-library imports never need a hint. Skip when the project already has a build/install file the apply phase updates (e.g. you added the dep to `pyproject.toml`).",
			"WIRING CLOSURE — when changes[] creates a new file under a registered subsystem (anything that needs a Registry.Register call to be reachable at runtime), you MUST also include a 'modify' entry for the corresponding wiring file in the SAME plan. The validator enforces this and rejects the plan with a concrete error naming the missing wiring file when violated — read the rejection text and add the modify entry. Without the wiring entry the new code is dead at runtime even if the file compiles.",
			"SUMMARY FIDELITY — every path-shaped token (e.g. internal/mcp/ssh.go) and every import path (e.g. github.com/pkg/sftp, golang.org/x/crypto/ssh) you mention in summary MUST match what changes[] actually contains. If your summary names a path, that path must appear in changes[].path; if your summary names an import package, that exact package must be imported by some new_content in this plan. The validator will reject the plan when summary lies about what's being changed.",
			"ONE CHANGE PER FILE: the changes[] array must NOT have two entries for the same path — the tool rejects duplicate paths. If a file needs two semantic edits, compose them into a single modify (full body) or patch (combined diff).",
			"RESOURCE BUDGET — the verify stage will run your tests under hard caps (default 2 GiB memory, 600 CPU-seconds, plus the configured wall-clock timeout). A test that exceeds any cap is SIGKILLed and the verify→plan retry receives an explicit OOM / CPU-limit / timeout classification — meaning you don't get to blame 'tests failed' if the real cause is unbounded allocation or an infinite loop. To stay within budget: every loop in test or production code MUST have an explicit termination condition (no `while True:` / `for {}` / `loop {}` without a reachable break/return); every recursion MUST have a base case; every allocation whose size depends on input MUST validate the input is bounded before allocating; every blocking call (sleep / wait / lock / network / file open) MUST have a finite timeout. These rules apply to BOTH new test fixtures and production code the tests exercise. Raising the caps is NOT an acceptable fix — bounded execution IS the contract.",
			"Use depends_on for ORDERING constraints between changes in this same plan: when creating a new file X and then modifying an existing file Y that will import / call X, set Y's depends_on to [\"X\"]. The apply stage topologically-sorts before writing, so declaring the edge guarantees X lands on disk before Y tries to reference it. depends_on is ALWAYS repo-relative paths of OTHER entries in THIS plan — cross-plan or absolute paths are rejected, as is any cycle (a → b → a). Leave depends_on empty when the default declaration order is correct.",
			"Optionally list acceptance_tests[] — natural-language test assertions the apply stage's verify phase should confirm. Empty is legal (no explicit tests to check). When the behaviour can be checked by a small deterministic runtime assertion, emit verification_probes[] as typed bounded inline probes (supported languages: python, javascript for Node.js, ruby, java via JDK javac/java, go; repo-relative working_dir, short timeout, optional expected_stdout). When task framing lists behavior contracts, prioritize hard_required contract_refs; soft_required satisfies contracts are useful context but do not prove exact values unless the probe itself asserts the exact behavior from grounded evidence. Add changed_symbol_refs[] naming the changed module/symbol the probe imports or executes. If a referenced contract carries comparator context, the probe should exercise the changed subject and the comparator relationship rather than only proving that the subject no longer crashes. If a referenced contract carries placement context, the probe should inspect the rendered line/surface and bind placement_refs[] to that contract id; contract_refs[] without placement_refs[] does not prove line-local placement. The verify executor runs these probes before project-level suites; passing probes become bounded local behaviour evidence and failing probes become typed tests_failed evidence. Probes must import/use the changed code and assert the externally requested behaviour directly; do not copy an isolated implementation expression into the probe and test only that copy. Include both positive and negative cases when the reported defect is boundary-like. Probes must exit non-zero on failure; do not encode broad shell commands or environment setup.",
			"Do NOT invoke apply_patch from the plan stage. Do NOT invoke run_tests unless it has dry_run=true AND a typed verification_probe object; suite/runner test execution belongs to the verify stage that consumes the plan later.",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
			"list_files",
			"repo_map",
			"run_tests", // typed verification_probe dry-run only during batch-local planning
			"emit_change_plan",
			"emit_plan_skeleton",
			"emit_plan_change",
		},
		OutputFormat: "Your structured output is either (a) a single emit_change_plan tool call (single-shot mode), or (b) emit_plan_skeleton followed by N emit_plan_change calls (one per non-delete change, structural multi-round mode for large plans). The two modes are mutually exclusive within one dispatch — pick based on anticipated payload size, then commit to that mode.",
		Prohibitions: []string{
			"do not modify any file during the plan stage — this stage produces the proposal, it does not execute it",
			"do not invent file paths that do not exist in the repository — read_file or grep to verify paths first",
			"do not emit apply_patch, and do not emit run_tests unless dry_run=true with a typed verification_probe object — suite/runner execution belongs to later phases",
			"do not write a plan whose changes[] array is empty — a plan without any proposed change is meaningless",
			"do not emit two changes[] entries with the same path — one change per file per plan, compose when necessary",
			"do not create cycles in depends_on — a → b → a, or any longer loop, is rejected. When two files genuinely mutually depend on each other's edits, they usually belong in a single combined change",
		},
	})

	r.Register(&Config{
		Name: "code-write-skill",
		Goal: "Apply every change declared in the plan to the active git worktree by calling apply_patch once per declared file. The system stops the stage automatically when every target path has been applied.",
		Workflow: []string{
			"HarmonyOS + Android language rules (honour the project's language — the plan's target_paths file extensions tell you which): " +
				"(1) ArkTS strict mode (.ets / .ts inside HarmonyOS Stage Model): NO `any`, NO `unknown`, NO `as` casts, NO index signatures `{[k:string]: T}`, NO `Function` type; every class field MUST have an explicit type; import paths MUST include the extension (`.ets` / `.ts`); stick to the 21-decorator whitelist (@Component @Entry @Preview @CustomDialog @Observed @Reusable @Builder @BuilderParam @Styles @Extend @State @Prop @Link @Provide @Consume @Watch @ObjectLink @StorageLink @StorageProp @LocalStorageLink @LocalStorageProp); struct (not class) for UI components; a single root (Column/Row/Stack) inside build(). " +
				"(2) Cangjie (.cj): use `func` (not `function`); the FIRST non-comment statement MUST be `package xxx.yyy`; mark public API with `public` or `open` (default is package-internal); use `extend Type { ... }` for type extensions; prefer `match` over cascaded if/else for algebraic data types; `foreign func` for FFI declarations; annotate thread-constrained entrypoints with `@CallingThread`. " +
				"(3) Kotlin (.kt / .kts): default visibility is public — use `internal` / `private` when a symbol should not cross module boundaries; prefer `val` to `var`; use `data class` for value carriers, `sealed class` for closed hierarchies; mark suspending functions with `suspend` (do not inline blocking I/O in them); prefer extension functions over utility classes; use `when` for pattern-like matching; on Android keep Activity/Fragment references inside lifecycle-aware scopes.",
			"The system has already loaded the plan and switched to the git worktree for this run. Your job is purely mechanical: iterate the plan's declared changes and emit one apply_patch call per change.",
			"Emit apply_patch with {path, kind} ONLY. The tool reads new_content / patch / delete from the stored plan — you do NOT re-emit or re-derive the content. Attempting to send new_content or patch as parameters fails the schema (additional properties are rejected). Read a file only if you need to understand it for a later retry turn; the apply call itself needs nothing but the path and kind.",
			"Respect depends_on ordering: apply a change ONLY after every path in its depends_on list has already been applied (visible because those apply_patch calls returned Success=true earlier in this dispatch). The apply_patch tool re-enforces this and rejects out-of-order calls, so a mistake surfaces as a clean error you can self-correct in the next turn.",
			"When a turn's response lists multiple apply_patch calls in parallel (tool_use blocks), make sure no two changes in the batch have a depends_on relationship — the batch executes concurrently and ordering is not guaranteed within a batch. If unsure, emit one apply_patch per turn.",
			"If apply_patch returns Success=false for a change, read the error summary carefully. Common rejections: path not declared in the plan (you drifted), depends_on not yet applied (reorder), kind mismatch (you sent a different kind than the plan declares). Correct the parameters and retry on the NEXT turn.",
			"Stop when every target path declared by the plan has been applied. The system checks automatically; you don't need to signal completion explicitly. Extra turns after completion waste tokens and may trigger the iteration cap.",
			"kind=patch is applied via `git apply -` inside the worktree against the stored unified diff (not your call). When git rejects a hunk (context mismatch, fuzzy match disabled), apply_patch surfaces git's own stderr verbatim so you can see which hunk failed. Retry is NOT a plan change — the plan's diff is authoritative; a failure means the diff did not match the worktree state and the verify→plan retry loop (or a fresh plan) is the remedy.",
		},
		ToolSuggestions: []string{
			"read_file",
			"apply_patch",
			"exec_command", // Q2 red line: preserved for debugging (e.g. verify file wrote) — worktree sandbox contains blast radius
		},
		OutputFormat: "Your ONLY structured output is apply_patch tool calls with {path, kind}. One call per declared change; the dispatch ends when every target path has been applied. Any prose is ignored — do not draft 'I'll now apply X' narration; just emit the tool call.",
		Prohibitions: []string{
			"do NOT modify files outside the plan's declared target paths — the tool will reject and your dispatch burns turns",
			"do NOT run tests here — verify stage owns that",
			"do NOT invoke emit_change_plan — the plan was already emitted and this step consumes it",
			"do NOT send a different kind than the plan declares — plan says create, you send create; plan says modify, you send modify",
			"do NOT send new_content or patch as apply_patch parameters — the schema rejects them; the tool reads from the stored plan so transcription errors are impossible",
		},
	})

	r.Register(&Config{
		Name: "test-execute-skill",
		Goal: "Call run_tests with an empty object so the deterministic verifier selects the project test surface and produces a structured ChangeReport. Optionally call emit_test_results to add a human-readable FailureSummary narrative.",
		Workflow: []string{
			"FIRST: call run_tests with `{}` exactly once. Do not inspect manifests or test directories to choose a runner before the call; run_tests owns typed TestSurface detection, plan-touched target preference, pre-suite verification probes, syntax/no-tests fallback, and dead-end escalation.",
			"Do NOT pass `runner`, `framework`, `working_dir`, `suite`, `timeout`, or `dry_run` from the verifier stage. The tool schema and runtime policy reject explicit verifier targets so the deterministic selector remains the single authority for test choice.",
			"SCOPED RUNS are selected by run_tests from typed evidence, not by verifier prose. When the ChangePlan touches a small area, run_tests uses TargetPaths, TestSurface candidates, and plan probes to prefer a bounded target before unrelated full-suite work; when shared code or root configuration is touched, it can escalate to broader candidates.",
			"Read the ChangePlan's acceptance_tests[] (rendered in the prompt) for context. Acceptance matching is not automated; the criteria are informational for the narrative you may draft.",
			"When the prompt includes a 'Pre-existing baseline failures' list, classify each failing test in the post-apply report as REGRESSION (passed in baseline, fails now) or PRE-EXISTING (failed in baseline and still fails). The baseline is an authoritative pre-apply snapshot — a failing test that also failed in baseline is NOT this plan's fault, and your narrative should say so explicitly.",
			"When all tests pass: return without any tool call beyond run_tests. The verify stage automatically promotes the passing report to a success outcome, and the orchestrator renders the summary.",
			"When run_tests reports `NoTestsRunners=[<runner>]`: that runner ran cleanly but discovered zero test cases or used a syntax fallback for a no-test tree. If the typed test surface has another runnable candidate, run_tests automatically escalates to it before returning the final report. Treat the returned verdict as authoritative; do not invent test files, do not call exec_command for additional syntax checks, and stop after run_tests.",
			"When tests fail: optionally call emit_test_results with a 1-4 sentence failure_summary narrative explaining the root cause. This replaces the parser's auto-generated count summary with your more useful context. Do NOT try to override the Passed verdict — it comes from the parser and the tool ignores LLM overrides.",
			"Do NOT re-run the tests multiple times or try to 'fix' them from this stage — verify is fail-loud, and failures surface to the user to drive a re-plan.",
			"Do NOT call exec_command for ad-hoc syntax checks (py_compile / node --check / gofmt / ...) AFTER run_tests has populated a ChangeReport. The parser-derived Passed verdict is authoritative; an exec_command success cannot override a parser failure (and vice-versa). run_tests owns syntax fallback and typed test-surface escalation; NoTestsRunners is a structured signal, not an instruction to add manual shell checks.",
		},
		ToolSuggestions: []string{
			"list_files",
			"read_file",
			"grep",
			"run_tests",
			"emit_test_results",
			"exec_command", // Q2 red line: preserved for diagnostic commands (git status, ls) — worktree sandbox contains blast radius
		},
		OutputFormat: "No required tool call. run_tests is usually enough; emit_test_results is OPTIONAL narrative. If you don't emit either, this dispatch completes automatically once run_tests has produced its results. Free-form prose between tool calls is discarded.",
		Prohibitions: []string{
			"do NOT modify files — verify is strictly read-only w.r.t. the worktree",
			"do NOT invoke apply_patch — that is the apply stage's job",
			"do NOT invoke emit_change_plan — the plan was already emitted",
			"do NOT try to override the Passed verdict via emit_test_results — the parser is authoritative; divergent LLM claims are logged as a warning and ignored",
			"do NOT re-run tests multiple times to hunt for flakiness — B1 treats any failure as fail-loud; re-planning is the correct response",
		},
	})
}
