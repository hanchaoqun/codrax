package skill

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sourceInventoryLensRoleProse renders the single-source repo_map
// source_inventory role list (types.SourceInventoryLensRoleNames) as
// comma-joined prose for skill prompts. Same single source as the
// repo_map parameter schema enums and refusal text, so the list a
// skill teaches can never drift from what the tool accepts.
func sourceInventoryLensRoleProse() string {
	return strings.Join(types.SourceInventoryLensRoleNames(), ", ")
}

// sourceInventoryLensRoleProseBackquoted is the same single-source
// list with each role back-quoted for prompt prose that renders roles
// as inline code.
func sourceInventoryLensRoleProseBackquoted() string {
	names := types.SourceInventoryLensRoleNames()
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "`"+n+"`")
	}
	return strings.Join(quoted, ", ")
}

// mechanicalProducerChainSeparationDirective is shared by evidence collection
// and answer synthesis. It is soft guidance for typed explain turns: no gate
// parses the user's request or the answer prose, and numeric/token equality is
// explicitly not promoted into mechanical authority.
const mechanicalProducerChainSeparationDirective = "MECHANICAL PRODUCER-CHAIN SEPARATION: when explaining how a rendered status, diagnostic, generated message, configuration-derived value, or other composed output is produced, trace each visible fragment to its own producer before joining the explanation. Treat outer decoration (for example a prefix or progress ordinal), localized/status payload lookup, and retry/loop policy as separate mechanisms unless a direct call, assignment, parameter flow, or returned value connects them. Equal numbers, equal tokens, nearby constants, or appearance on the same rendered line are candidate clues only — never proof that one controls the other. Ground the visible fragment at the formatter/composer that emits it, then follow that fragment's actual inputs backward; if one producer chain was not established, state that boundary instead of borrowing a plausible constant from another subsystem."

// independentMechanismContrastDirective prevents a comparison answer from
// treating one classifier's complement as proof of a different mechanism.
// Like the producer-chain rule above, this is shared soft guidance selected
// from typed mechanism shape; it does not inspect or rewrite answer prose.
const independentMechanismContrastDirective = "INDEPENDENT MECHANISM CONTRAST: when explaining a distinction, boundary, or interaction between two or more named mechanisms, trace each side through its own producer and control path before comparing them. A predicate that classifies mechanism A proves only A's membership and branch behavior; its false branch or logical complement is not evidence for mechanism B unless source shows an explicit handoff, shared decision, or return-value flow. During exploration, use repo_map/grep only to locate each side, then open the load-bearing definition, branch, handler, or requeue path before claiming its behavior. A carrier-only enum, constant, type, schema, or event-name declaration proves identity only; it does not prove who emits or calls it, what condition triggers it, who handles it, or which control-flow action follows. A global registry or capability catalog likewise proves availability only; neither declaration nor registration proves that an item is an active member of one requested mode/path. For a current mode/path's active membership and order, prefer the narrowest mode-specific selector, dispatcher, guard list, or executed control-flow over a broader declaration/registration universe; keep broad-only members as available/background capabilities unless a grounded current-path edge admits them. Ground a behavioral mechanism at its actual producer/callsite and consumer/handler branch, not at a declaration that merely names the carrier. Carry a principal mechanism comparison as `aggregate_facts.member_set`: one `members[]` entry per compared mechanism, an index-aligned `member_notes[]` entry describing that member's own path, and one index-aligned `support_refs[]` entry to that path; never leave one mechanism only in ungrounded grouped_count dimensions. During answer synthesis, preserve only independently supported members and the proven join. If one side's path was not established, state that evidence boundary instead of describing the mechanisms as a proven binary partition."

// runtimeRuleInstantiationDirective separates a source-level rule from proof
// that one attached runtime event instantiated that rule. It is selected from
// typed log+mechanism context only; no request or answer prose is inspected.
const runtimeRuleInstantiationDirective = "RUNTIME RULE INSTANTIATION: source code that defines a predicate, threshold, classifier, advisory, or routing rule proves that the rule exists; it does not by itself prove that an attached runtime event satisfied the rule or was caused by it. Before attributing the observed event to that rule, bind every load-bearing operand from the attached artifact or another typed runtime source (for example model/provider identity, configured threshold, attempt state, branch result, and enforcement site) and reconcile any mismatch. A declaration or soft-warning predicate is not the enforcement path. If an operand is missing or contradicts the rule, present the source rule as possible context or an uninstantiated advisory and keep the observed event's cause unproven. Do not upgrade source plausibility into runtime causality."

// generatedArtifactVerificationDirective keeps generator/template changes
// honest at the artifact boundary. It remains planner guidance: verification
// chooses and runs a behavioral probe, while no source-token hard gate is
// introduced in the product runtime.
const generatedArtifactVerificationDirective = "GENERATED-ARTIFACT VERIFICATION: when a change edits a template, generator, transpiler, serializer, code emitter, or build step, verify the produced artifact rather than only scanning the producer source. Any declaration, helper, guard, import, or reference used by generated code must exist in the generated artifact's own lexical/runtime scope and in emission order. A source-token or regex check can show that text exists in the generator, but cannot prove that the generated output parses, resolves names, executes, or preserves behavior; add a bounded probe that renders/builds the artifact and then parses, imports, compiles, or executes it at the closest available boundary. If the native runtime is unavailable, use a deterministic generated-output parser/scope check and disclose that narrower boundary."

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
			"PHASE 1 — Breadth scan: use repo_map and grep (files_only=true) to map a bounded candidate set before reading files. For mechanism, architecture, call-chain, handler/route/config lookup, or current-source explanation questions, repo_map is usually the best first step for locating likely source paths and relationship neighborhoods; then use read_file or targeted grep to prove the selected facts. Start repo_map with overview/task_map/file_map for orientation; when you need a scoped member inventory or member→attribute checklist, use repo_map(view=\"source_inventory\") with model-chosen roles (roles accepts exactly: " + sourceInventoryLensRoleProse() + " — map the user's construct family to the closest listed role, e.g. a decorator/annotation inventory → roles=[\"function\"] or roles=[\"type\"] per language carrier, a config-surface checklist → roles=[\"config_key\"] with scope=\"<config-dir>\"), optional attribute_roles, and scope/scopes, then cascade into narrower source_inventory calls instead of reading every candidate file. Treat repo_map / source_inventory rows as verified navigation facts, not semantic source-code citations: use them to bound candidate files/symbols/routes/config keys, then verify selected behavior or implementation claims with read_file or targeted grep before citing source text. Relation rows may end with a bracketed origin tag carrying a confidence label that grades edge strength: prefer higher-confidence edges when choosing what to read next, and treat lower-confidence edges as real but unproven leads — never as errors and never as citable facts on their own. For non-English questions, search with BOTH the original terms AND their English programming equivalents. Try multiple keyword variants (word roots, synonyms, abbreviations) only when exact/high-confidence searches are too narrow.",
			"PHASE 2 — Depth investigation: use grep (for targeted pattern search) and read_file (for full context) — pick the most efficient tool for each situation. After each inspected source region, call emit_evidence with ALL facts in one batch. repo_map / source_inventory relationship rows and broad or files-only grep results are navigation only and cannot become line-scope evidence. A scoped targeted source grep result that prints the exact production match with file, gutter line number, and enough local context may be emitted directly when grounding accepts that location; use read_file whenever the full body, enclosing control flow, or wider context is needed. Never emit from a filename-only or relationship-map row. For each source region extract: (a) key data structures, (b) control flow, (c) configuration-driven behavior, (d) cross-component interactions. When an inspected function/method body contains a load-bearing direct invocation, emit that invocation as its own `evidence_kind=relationship`, `anchor_kind=call` item with exact caller, callee, and call-site line; do not leave the observed edge only inside a definition item's summary. This is an authoring completeness reminder, not a requirement that every definition have an incident edge: leaf bodies and definitions without an observed invocation remain valid, and unsupported edges must never be invented. When obvious, also set emit_evidence's optional `context_role_hint` (defining / absence_support / related_context / illustrative_only) and `diagram_role_hint` (default / config / runtime / override) so the rendered answer can reuse validated structure instead of guessing from prose. Use `surface_terms` for exact user-visible labels/aliases found in the cited source/log/trace lines that are not already captured by subject/object/anchor_symbol, such as original file labels, route names, package/module names, config keys, macro names, trace span names, runtime object labels, or labels in leading documentation/header comments attached to the anchor. Every surface term must be copied verbatim from already-inspected lines; the tool validates this and answer synthesis should preserve relevant terms when they matter to the visible answer. Use `absence_support` when a grounded fact helps prove the exact target is missing but does not itself define the target. For config-precedence / config-trace questions, treat `diagram_role_hint` as required whenever the evidence is clearly a defaults layer, a config-file layer (YAML/JSON/TOML/INI/etc.), a runtime binding layer, or an override layer. Set `load_bearing_summary: true` ONLY when the `summary` text holds a scalar (a hash, version string, count, single concrete identifier, or any value derived from a tool / shell / git / build command output) that the user-facing answer must reproduce verbatim AND the typed fields (`subject` / `predicate` / `object` / `anchor_symbol` / `snippet`) cannot themselves carry that scalar — without this flag, the final answer may omit the summary as nearby-context noise. Default false is correct for the common case where summary is a paraphrase the typed fields already encode. When the row itself is important to the user-facing answer, set optional `salience`: `load_bearing` for a fact the answer cannot honor without this row, `exhaust_listed` for a member of a complete list the user asked for, `supporting` for an intermediate fact the answer chain uses, or `context` for background. Omit `salience` when unsure; it helps preserve important rows but does not replace member_set, answer_symbol, citations, or final answer obligations.",
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
			"CALL-CHAIN COVERAGE: when the user asks how a request / value / piece of data flows from an entry point through a pipeline to its exit, list every load-bearing intermediate function the data passes through, not just the first 2-3 you find. If the entry function is large (>500 lines), read the full body before stopping — load-bearing pipeline steps are often scattered across the function. The answer needs entry + ≥3 mid-segment + exit; stopping early produces a partial chain that misleads the reader. The same rule applies to processing pipelines, data flows, and call sequences across files. When the typed request carries a completeness obligation for a mechanism/call path, audit each already-read load-bearing callable before completion: emit entry/early-return guards as separate `conditional` + `condition` rows, and emit every material call inside a conditional branch as its own `relationship` + `call` row. Never fold a guard and its guarded call into one item, never claim that lexical adjacency proves control ownership, and do not enumerate unrelated formatting/helper calls merely because they share the body. This rule is language-neutral and applies equally to Go, Java, Kotlin, JavaScript/TypeScript/ArkTS, C/C++, Rust, Python, Ruby, Swift, Lua, Cangjie, and other executable languages. CALL-CHAIN FRONTIER HANDOFF: several direct calls found in one caller body are a sibling-edge SET, not automatically an ordered caller→helper→helper path. Emit each load-bearing call you actually observed as its own typed call evidence row with the exact caller, callee, and callsite. If source inspection finds a reverse or shared-callee direct call that explains why the requested direction has no path, emit that real edge separately before declaring `principal_span_waiver.reason=no_directed_path`; a definition row or waiver rationale cannot substitute for the edge. A shared callee is a static graph boundary only and does not prove parallel execution, convergence, or a join. Before calling a flat sibling roster a complete chain, follow only the already-proven, load-bearing helper frontier one body at a time while that helper carries the requested value or control flow. Stop at the requested sink, a proven boundary, or an external/dynamic handoff. This is bounded semantic descent, not exhaustive traversal of every callee.",
			"CONFIG PRECEDENCE: when the user asks where a configuration value comes from or which layer wins, surface evidence from EVERY layer in the precedence chain: the source-code default (the struct or constant that defines the baseline), the configuration file (yaml / toml / json / ini / properties — whichever format the project reads), and the runtime override (CLI flag, environment variable, command-line argument). A layer answer that names only 1-2 of these is half-formed; if a layer truly does not exist for the given key, say so explicitly rather than omitting it.",
			"MECHANISM WIRING: when the user asks WHICH parameter / gate / function governs a specific behavior, do not stop at a name-similar or concept-adjacent symbol — confirm the candidate is actually CONSUMED on that behavior's execution path (grep its usage sites and check at least one reader sits on the asked path) before naming it as the answer. A field or constant that is never read on the asked path is a different mechanism, however plausible its name; when two candidates compete, prefer the one with a verified consumption site and cite that site. The same applies to absence claims: a token found only in meta/infrastructure surfaces (linters, blocklists, test fixtures, docs) is not evidence about the runtime surface — trace the nearest REAL group or mechanism the asked name belongs to before concluding non-existence.",
			"EXHAUSTIVE ENUMERATION: when the user asks for a list of items spread across a package, directory, module, route table, config surface, class hierarchy, registry, or multi-language scope, use a deterministic enumeration/navigation tool that covers the full search space rather than reading individual files one at a time. Good tools for this shape: `repo_map(view=\"implementers\", query=\"<InterfaceName>\")` for the exhaustive set of concrete types implementing an interface / trait / protocol (the typed Implements relation — strictly better than grepping the name, which misses implementers whose definition never spells the interface), `repo_map(view=\"source_inventory\")` for typed member/attribute checklists with counts and paging, `repo_map` task_map/file_map for structural orientation, `grep -l <pattern> <dir>` for every matching file, `exec_command find <dir> -name '...'` for sub-directory discovery, and `grep -rn <pattern> <dir>` for every match across the tree. Visual `read_file` of one file at a time produces incomplete enumerations because the investigation budget runs out before every candidate is visited — the answer becomes an honest 'lower-bound' instead of the exhaustive list the user asked for. After the exhaustive tool returns the candidate set, read/grep selected items to capture role descriptions or proof, and back counts/member sets with the tool output plus grounded verification rather than manual tally from prose.",
			"if the request attached a runtime log (panic / exception trace / sanitizer diagnostic / traceback), the stack-frame file:line pairs are the files you MUST open FIRST before widening the search. Read from the innermost (deepest) frame outward so the actual error site is the first fact you establish",
			"COMPLETION: when you have collected enough evidence to answer the user's question, call emit_investigation_complete(reason, confidence, result_kind). This tool call is the explicit completion signal. Without it the investigation keeps running until its budget ends, and a budget-ended investigation hands off whatever evidence it happens to have — usually a weaker answer than a deliberate completion with a stated conclusion, so prefer the explicit call. If the completion call is rejected with a named missing-work reason, do the named work or switch to the matching typed field (result_kind='absence', a typed waiver) instead of re-sending the same call unchanged. Set result_kind='resolved' for ordinary positive/citable answers. If your confidence is not at least 'medium', continue investigating.",
			"COVERAGE BEFORE COMPLETION: when the user's question carries a structural coverage obligation (the pre-scan section above names them: an exhaustive-coverage demand, a declared item count, or a partition into named groups), every candidate file your grep / repo_map / list_files surfaced MUST be either read_file'd OR explicitly excluded via a narrower follow-up grep before you call emit_investigation_complete with result_kind='resolved'. A grep that returns 8 candidate files and only 3 read_file calls is an under-coverage failure on a 'list every X' question — the answer slate would silently ship 3-of-8. Either complete the reads, or set result_kind='absence' with an honest absence_justification, or re-run grep with a tighter pattern that confirms the unread hits were collateral. Mechanism, architecture, and call-chain explanations do NOT require every navigation candidate to be exhausted unless the structured analysis also declared one of those coverage obligations; for those shapes, read and cite the load-bearing files that prove the flow and treat collateral candidates as optional navigation hints.",
			"ABSENCE ANSWERS: if the answer is an honest 'zero' / 'no X' / 'nothing found' (e.g. 'how many .py files?' → 0, 'does handler X exist?' → no), set result_kind='absence' and add absence_justification to emit_investigation_complete with a one-sentence explanation. Ordinary cited answers require citations by default, but an honest-zero answer has nothing to cite — the declaration waives the citation floor. You still must have run at least one real investigation tool (grep / exec_command / list_files / read_file / repo_map); an empty investigation cannot declare absence. Never set absence_justification on a positive answer. Grounded related-context anchors are still allowed when they remain clearly contextual and do not define the missing exact target.",
			"EVIDENCE_FLOOR_WAIVER (typed escape for external / non-repo inputs): when YOU have read the input and confidently determined that pretending to ground against this repository would be misleading, set `evidence_floor_waiver` on emit_investigation_complete to declare that ordinary repo-grounding requirements do not apply to this investigation. Use it when: (a) the attached log / trace is from a system whose code is NOT in this repo (a customer paste from another service — set reason='external_only_log' or 'external_only_trace'); (b) the input's frame paths superficially resemble current-repo paths but you have determined they represent a different build / version / deployment (set reason='no_repo_intersection'); (c) the input is informational only — debug breadcrumb, performance baseline, clean trace with no failure component (set reason='informational_runtime_only'). The waiver requires `reason` (one of the four enum values) AND `rationale` (one self-contained sentence — it activates the waiver and is shown downstream as the stated justification, so name the observed mismatch concretely). Do NOT use the waiver to short-circuit ordinary investigation work or to escape questions that genuinely need repo grounding — the audit trail records every fire and misuse is reviewable post-hoc. When in doubt, do NOT set it: leave repo-grounding requirements in effect and ground normally. If you previously set a waiver and later discover that repo grounding DOES apply, set `clear_evidence_floor_waiver=true` on emit_investigation_complete and omit `evidence_floor_waiver`. When the waiver IS appropriate, the question is answered from the LOG / TRACE content itself (its frames, message text, observed sequence) — cite log entries verbatim, do not synthesise repo file:line citations.",
			"PRINCIPAL_SPAN_WAIVER (typed escape for call-chain span/boundary conclusions): after reading the source, set `principal_span_waiver` on emit_investigation_complete only for one of these finite cases: (a) source and sink are on the same statement / line — a one-liner dispatch, wrapper, or trampoline (reason='endpoints_directly_adjacent'); (b) the lines between contain only plumbing the user is not asking about — nil guards, accessor pass-through, defer setup, logger calls — with no separately-citable user logic (reason='no_intermediate_user_code'); (c) the intermediate crosses an FFI / JNI / cgo / cross-language native bridge where the hop is in the runtime, not in repo source (reason='platform_bridge_intermediates'); (d) the compiler / JIT inlined the intermediate so the source line directly invokes the sink with no separate frame (reason='inlined_call'); (e) the dispatch is virtual / interface / closure / reflection-based — no static call edge exists (reason='runtime_dispatched_call'); (f) the chain crosses into an external library / SDK whose intermediate frames are not in this repo (reason='external_module_continuation'); (g) both requested endpoints exist but accepted typed call edges do not reach the requested sink from the source — for example the sink is a wrapper that calls the reachable sibling in the opposite direction (reason='no_directed_path'). Reasons (a)-(f) apply only after a directed source→sink path exists and waive separately-citable intermediate span evidence; they can never replace a missing directed endpoint path. Reason (g) declares the endpoint boundary: keep the nearest proven directed path, preserve the exact requested sink separately, and show reverse/parallel calls only in their real direction. The waiver requires `reason` (one of the seven enum values) AND `rationale` (one self-contained sentence naming concrete details; a blank rationale is ignored and normal gates still apply). Do NOT use a waiver for evidence you simply have not looked for. If later evidence contradicts it, set `clear_principal_span_waiver=true` and omit `principal_span_waiver`. Every fire is logged for post-hoc audit.",
			"PRIOR-CONVERSATION RECALL: if the user references their own past conversations, pick by intent: (a) TOPIC search ('我们之前讨论过 OAuth / what did we say about X?') → recall_memory(query='X'); (b) LISTING / inventory ('都有哪些 / 历史里有什么 / what's in memory') → list_memory(limit=10) which returns the most-recent N entries by time, NOT by keyword overlap. recall_memory's keyword scoring will only surface self-referential meta-entries on a generic listing query — that's why list_memory exists as a separate tool. Both bypass the injected `## Prior conversation` block's keyword-only matching; either may be sufficient on its own — emit_investigation_complete with result_kind=resolved if so.",
		},
		WorkflowTierB: []TierBItem{
			{
				Body:      "RUNTIME TRACE FIRST: when the typed dispatch carries a runtime trace / ftrace / systrace / hitrace / atrace / perftrace / tracebundle artifact, start with `trace_query` before repo_map, broad grep, reading from the trace head, or hand-written grep/awk loops. `trace_query(path=...)` accepts raw ftrace-compatible text paths such as `.ftrace`/`.trace`/`.systrace` plus `.tracebundle.json` directly, automatically promotes sibling `.tracebundle.json`, and builds a provenance-aware sibling `.systrace` + `.perftrace` composite — sibling perf samples join the shared causal timeline only when the clock domain is proven (identical time domains or an explicit calibrated mapping); an unproven sibling `.perftrace` stays clock-isolated with a typed caveat and remains directly queryable by its own path — so pass the concrete path the system surfaced instead of manually unpacking sidecars first. For frame/span ids or labels, start with `trace_query(view=\"event_search\", pattern=\"<literal>\")`; for inode/file IO, use `event_search` with `pattern` as the inode or entry_name and event_types such as `file_io`, `page_cache`, `f2fs`, or `android_fs`; for top-N / most-frequent inode IO enumeration, use `view=\"window_stats\"` and read its `top_io_inodes` section (per-inode event counts summed across threads and ordered by count, with the total group count disclosed; its latency is the single largest event plus per-thread totals, never a cross-thread latency sum); for CPU samples, use event_types=[\"perf_sample\"] and pattern as a symbol, DSO, callchain, event, or thread token. `pattern` is a literal substring, not a regex, and should usually be one exact frame id, span label, marker token, inode, file name, perf symbol, or timestamp before adding time/line/thread filters. Treat `entry_name` as a trace file-name label, not an absolute path; do not turn it into `/entry_name` or `/data/...` unless that full path is grounded. For mixed trace+source questions, use `trace_query` for runtime-artifact facts and use normal source tools only for the explicitly requested current-code verification lane.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "TRACE QUERY: when a typed runtime trace / ftrace / systrace / hitrace / atrace / perftrace / tracebundle artifact is active, prefer `trace_query` before hand-written grep/awk loops. Pass `.ftrace`, `.trace`, `.tracebundle.json`, `.systrace`, or `.perftrace` paths directly; trace_query automatically promotes sibling `.tracebundle.json` and builds a provenance-aware `.systrace + .perftrace` composite — sibling perf samples join the shared causal timeline only when the clock domain is proven (identical time domains or an explicit calibrated mapping), and an unproven sibling `.perftrace` is isolated with a typed caveat while staying directly queryable by its own path. Use `view=\"event_search\"` with `pattern` as a literal substring (not regex) for exact frame ids (`1917295`), jank ids (`jank_frames=7`), span labels (`Choreographer#doFrame`), marker labels, inode tokens (`0x478e5`), entry_name values, perf sample symbols/DSOs/callchains, timestamps, or event labels; if it returns zero rows or `parsed_events=0`, first treat that as a window/filter/timestamp diagnostic rather than proof that ftrace is unsupported: shorten the literal, add `event_types=[\"trace_mark\"]` for B/E/C/S/F span rows, `event_types=[\"perf_sample\"]` for CPU sample rows, or `event_types=[\"file_io\"]` / `[\"page_cache\"]` / `[\"f2fs\"]` / `[\"android_fs\"]` for inode rows, remove over-narrow pid/thread/time filters, verify compound timestamps such as `1s 501ms 565μs 915ns` normalized to seconds, or use returned span/time/line windows before root-cause analysis. Use `view=\"span_window\"` with `span_name` when the user names a trace span instead of exact timestamps; event_search rows expose span_action/span_pid/span_name/span_value, and span_window/window_stats trace_spans expose kind=sync|async. Synchronous B/E span ends are unnamed `E|pid` or bare `E` on the same ftrace thread stack, async S/F spans pair by marker pid+name+cookie, so do not search for `E|pid|spanName` to prove completion. If multiple span windows match, first locate the specific frame/span marker with `event_search(pattern=\"<exact literal>\")`, then rerun follow-up views with the selected time_start/time_end or line_start/line_end. Pick the lens from the full view matrix: use " + RenderTraceQueryViewMatrix() + ". For binder IPC, consume `oneway`, `sync_like`, and `blocking_candidate` from trace_query output rather than guessing blocking semantics from raw flags. Treat perf sample output as code-execution context: `window_stats.perf_samples` is broad same-window context, `root_cause_rank.items[].perf_context` is compact candidate interval/thread context, `root_cause_rank.items[].perf_contexts` is role-aware support such as candidate_thread/target_running/on_chain_dependency/same_cpu_competitor/cpu_pressure_top_running/compute_supply_cpu, and `frame_root_cause_bundle` role fields such as `target_running_perf`, `on_chain_perf`, `binder_peer_perf`, and `same_cpu_competitor_perf` show where sample hotspots belong. For perf evidence-quality questions, read `sample_cpu_scope`, `cpu_known/cpu_unknown`, `sample_kind`, and `weight_unit` before using nearby scheduler rows: sample_cpu_scope=unknown or sample_kind=off_cpu means the perf sample has no concrete CPU/core execution location, while a sched_switch `[CPU]` column is only the scheduler event row CPU. For running or compute-supply causes, consume perf_contexts to explain what code consumed sampled CPU time, but keep scheduler interval overlap, chain relevance, binder peer state, D-state/IO, CPU/core/frequency/affinity, and supply pressure as the causal basis. On wakeup_chain rows, projected_impact_ms/projected_total_ms are the selected-window or target-blocking projection; on root_cause_rank rows the projection key is projected_impact_ms (a rank observation note spelling projected_total_ms only echoes cumulative_impact_ms — one value under two names, never a second measurement), and ranking order lives on effective_impact_ms (before score), not on the projection pair; actual_impact_ms/actual_total_ms/actual_window describe the underlying scheduler state segment that may extend outside that projection — do not mix the projected and actual durations. top_symbols/top_dso/top_callchains explain what sampled threads were doing, but scheduler interval overlap, chain relevance, binder peer state, D-state/IO, and CPU supply remain the causal basis. When a wakeup chain exists, treat window_stats IO/D-state/CPU-pressure rows as background context unless root_cause_rank marks the corresponding candidate chain_relevance=on_chain/causality=on_wakeup_chain; aggregate rows such as cpu_pressure/io_pressure/supply_pressure remain supporting context and must not be promoted into the direct root-cause chain merely because their representative thread overlaps the chain. An off-chain row or aggregate pressure proves only that background load was observed: temporal overlap, the same broad IO/CPU class, or a pressure score does NOT prove a shared device/inode/lock/core, contention with the on-chain cause, or that it lengthened that cause. Unless trace_query publishes an exact shared-resource identity or typed dependency/competition edge, describe it as independent background and at most a follow-up investigation direction; never add its duration to chain rows or invent a combined total. Apply the same discipline to the investigation conclusion you hand off in `emit_investigation_complete`'s `reason`: when a rank=1 chain_relevance=on_chain cause exists, open the reason's root-cause conclusion with that cause; root causes have TWO dimensions, so beside that priced cause also carry the significant RAW-occupancy findings the pricing rules could not price — business_span_mention families (verbatim span + trio) and any on-chain row whose raw running/occupancy is large but effective-priced to zero — as new-fix-direction leads, so the final answer can speak both dimensions — but skip rank rows with tier=target_self_state (the analysis target's own wait/lock-hold/sleep symptom: keep them as the target's own state, never the root cause, and take the conclusion from the best-ranked non-symptom row — which may be the target's OWN runnable/running/IO/D-state row: those compete normally as decomposable self causes such as scheduling pressure, compute supply, IO blocking, or D-state) — and report cpu_pressure/io_pressure/supply_pressure-style aggregates only as background load with their cross-thread CPU·ms basis stated, not as the headline cause. When the final answer reports a specific trace event, carry that event's NATIVE identifying key=value fields verbatim (e.g. a page-fault row's operation= and address=, an event's code/flag values) in addition to any generically requested dimensions like path/latency/bytes — do not compress an event down to the requested dimension list when its own native fields identify it. When a result carries a `blocked_reason_census` line for a thread (total=N with per-caller ×count(Σms) shares), treat that census as the authoritative blocked_reason inventory for the window: carry all N records with their published caller symbols and iowait classification into your notes and conclusion. A blocked_reason caller is only the recorded kernel call site, not by itself a waited object, owner, device, or completed GPU/IO operation; name those only from separate typed identity or dependency evidence. Census records and scheduler-state intervals are different measures: never explain a count/Σ mismatch as rounding or precision tolerance, and never pair them one-to-one without a typed interval join. When your reading of individual raw rows disagrees with the census count or Σ, re-read the rows instead of silently dropping one. For top-N / most-frequent inode IO enumeration questions, read the window_stats `top_io_inodes` section: whole-window per-(dev,inode) totals with cross-thread event counts summed and the total group count disclosed; its latency figures are the largest single event plus per-thread totals — never a cross-thread latency sum. Inode summaries identify dev/inode/op/bytes/count/latency/churn from the trace; mapping inode to a pathname requires trace entry_name/path fields or a separate filesystem mapping, so do not invent a path from inode alone. `entry_name` is a file-name label, not an absolute path; never render it as `/entry_name`, `/data/...`, or another directory-qualified path unless the trace or external mapping provided that exact full path. Trace timestamps are seconds end-to-end: 928.081774 = 928s + 0.081774s; with six fractional digits the fractional part is microsecond-precision (81774 us). Durations are reported in ms. `read_file` pagination uses `line_offset` as a zero-based line coordinate, not a byte offset; use trace_query/grep line windows or gutter lines to choose it. If the trace flavor is already typed as Harmony/鸿蒙/东湖/OHOS, pass `trace_flavor=\"harmony_hitrace\"` or `platform=\"harmony_hitrace\"`; if the trace flavor is typed as Android/安卓/atrace, pass `android_atrace`. Typed platform intent wins for that call, while content-detection conflicts are only audit caveats. trace_query reports trace_flavor and priority_semantics: HarmonyOS/hitrace means larger numeric priority is higher (1-40 CFS, 41-159 RT; >159 system_or_kernel/raw), while Android/generic ftrace keeps raw scheduler priority and does not apply Harmony ranges. Its results are runtime-artifact evidence with artifact-local line numbers; do NOT turn trace rows into current-source citations unless separate source evidence proves a current checkout fact. If `trace_query` says the format is truly unsupported or incomplete after broad artifact-local verification, fall back to targeted grep/read_file/exec_command with preserved line numbers.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// DCS E6/F3a (ledger §23.1 ruling ③, 2026-07-08): the mention
				// obligation is a DOUBLE gate on typed fields — every retained
				// on-chain semantic family is mentioned as an optimization point
				// regardless of its primary/secondary/tertiary tier or TOP-N rank;
				// non-chain semantic rows only at background_rank<=3.
				//
				// EVOLUTION RECORD (RCM §24.7.1/§24.10 user rulings 2026-07-08,
				// real_trace_campaign_20260705.md §24.12): the mention
				// obligation moved from single-span to FAMILY caliber —
				// same-thread spans of one semantic class now arrive as ONE
				// merged rank row whose projected_impact_ms is the family's
				// window-projection TOTAL (member_count/member_roster/
				// member_max_ms carry the merged members), so "the largest
				// one" is judged on family totals and the mention carries the
				// merged count and largest member name. The background_rank<=3
				// gate is unchanged mechanically — merged rows simply occupy
				// fewer, larger board positions.
				// TEX (§28.1, 2026-07-09): texture_upload joined the semantic
				// class set — same treatment as the four compile classes.
				// EVOLUTION RECORD (B830/B831, eval r507/r508, 2026-08-15):
				// on-chain membership and semantic causality are separate typed
				// axes. Target-self deterministic work (and any future exact
				// target-wait/completion binding) may keep positive effective
				// attribution. A non-target chain-interval intersection or bare
				// host wakeup edge is relation-only: preserve its raw occupancy and
				// unconditional optimization mention, but never infer that the
				// target waited for completion or let it enter the rank ladder.
				// This is soft teaching from typed fields only; no answer prose gate.
				Body:      "TRACE SEMANTIC SPAN ROOT CAUSES: root_cause_rank and the independent trace_semantic_span channel emit the closed semantic work classes (`jit_compile`, `class_verification`, `shader_compile`, `runtime_compile`, `texture_upload`, `gc_pause`) for compile/verify/shader/texture-upload/explicit-GC-pause spans projected into the window. Same-thread spans of one semantic class arrive merged as a single family: member_count carries the merged span count, member_roster the individual span names with their durations, member_max_ms/member_min_ms the member value range, and the typed raw span/overlap fields preserve the complete occupancy account independently of effective_impact_ms. EXCEPTION — shader_compile splits by PROVEN cache outcome: span_subcategory=shader_cache_miss and span_subcategory=shader_cache_hit arrive as TWO separate same-thread families with their own totals; mention each separately (miss = real compilation, the actionable precompile/cache-warm optimization point; hit = cache-served lookup time, never a compilation cost and never grounds to advise precompilation) and never sum the two into one shader claim; plain shader subcategory makes no cache claim. Consume span_name/span_kind/span_category/span_subcategory/semantic_class, on_chain_basis, projected_impact_ms, effective_impact_ms, rank/tier, member_count/member_roster and actual_* fields. Only a semantic row with positive typed effective_impact_ms and a positive primary/secondary/tertiary rank participates in the ordinary root-cause election; when it ranks highest, name it by semantic class (for a merged row, the class word with its span count — e.g. \"Texture upload x11\" — never one member's span name) together with the published effective caliber. A non-target row with on_chain_basis=semantic_chain_interval_relation or host_wakeup_edge_pre_span is relation-only: keep its exact raw occupancy, on-chain business clue, member roster, and deterministic optimization direction, but effective_impact_ms=0/rank=0 means it is not a root-cause seat. Do not say the target slept waiting for that semantic operation, that its completion triggered the wakeup, or that it directly blocked the target unless a separate typed target-wait or semantic-completion binding establishes that mechanism; state the exact wakeup/path relation separately. Relation-only semantic rows MUST NEVER enter the Background board merely because they are unranked. If typed authority reports `causal_conclusion=unproven` or `frame_evidence_status=absent`, preserve the frame-causality boundary and do not upgrade a selected-window ranking into a proven frame cause. Independently of root-cause TOP N, the final conclusion MUST mention EVERY retained on-chain semantic family from trace_semantic_span as a deterministic optimization point together with its raw share of its own query window and, when member_count>1, its merged span count and largest member span name; these field-verified opportunities are never omitted merely because their rank row was truncated or relation-only. Only rows without on-chain overlap may carry background_rank; do not mention an off-chain semantic family in the conclusion unless background_rank<=3, and then only as a background optimization point. Generic `trace_span` / trace_mark_category rows are supporting context and are never promoted into the direct wakeup-chain cause.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// WO-P1 (SMR-1 批 S9-AWEME, smr_audit_report §②, 2026-07-12):
				// the answer-body IO type word forked across runs and against
				// the evidence face (io_latency vs io_burst_episode naming ONE
				// physical IO) — soft guidance only (正文榜 is the model's
				// prose face; 精确信号红线禁硬卡). Authority order mirrors the
				// engine's io-facet lead election (io_wait > io_latency >
				// io_burst_episode); consistency is observed via eval, never a
				// hard gate.
				Body:      "TRACE IO TYPE-WORD SINGLE SOURCE: when the answer body names an IO cause, use ONE IO type word per physical IO episode, taken from the evidence row the sentence cites (its typed `type` field): io_wait for the scheduler-wait account, io_latency for the end-to-end completion account, io_burst_episode for the burst-episode account. When one episode was measured by several calibers, lead with io_wait when present, else io_latency, else io_burst_episode; name the other calibers only as alternative measurements of the same time — never as additional causes and never added together. Keep the same type word for the same episode everywhere in one answer: the body wording must match the cited evidence row's own type word.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "PERF SAMPLE PROVENANCE: preserve trace_query perf sample `source` and `symbolization_status` in final markdown/html reports. `raw_perfdata_fallback` / `unsymbolized` rows are useful for time/thread/DSO/IP correlation but lower confidence than official hiperf/simpleperf symbolized output.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// Soft semantic boundary only. A caller name is useful navigation,
				// but its morphology cannot mint a resource/mechanism/fix claim.
				Body:      "BLOCKED-REASON CALLSITE BOUNDARY: a blocked_reason caller is only the recorded kernel call site, not by itself a waited object, owner, device, completed GPU/IO operation, subsystem mechanism, or direct fix direction. Its function-name morphology is only a code-location/search clue unless a separate typed identity or dependency field confirms the interpretation. Do not expand a symbol-shaped label into a subsystem story or fix recommendation without that separate evidence.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "TRACE PRIORITY-INVERSION AUTHORITY: `lower_priority_waker` and `lower_priority_dependency` prove only a low-priority dependency candidate; by themselves they do not prove that priority inversion occurred, do not prove that the lower-priority thread blocked the higher-priority thread, and carry no elapsed inversion impact. Only a typed `root_cause_*` row whose type is `priority_inversion_candidate` or `priority_inversion_runnable_wait` and whose `effective_impact_ms` is positive supports the wording priority-inversion candidate plus that measured impact; `priority_inversion_authority=confirmed_holder_waiter` is the separate confirmed lane. The measured mechanism is deliberately broader than same-CPU preemption: preserve the producer's typed gated composition, including a lower-priority on-chain dependency's runnable time in full and any cross-CPU weak-core/compute-supply running deficit. A causal-tree self row labelled `own·runnable` / `自身·runnable` is the focused thread's own ready-to-run wait, never the dependency thread's runnable state; claim dependency runnable impact only from that dependency's own measured typed row. When only the structural edge exists, say `low-priority dependency candidate; no priority-inversion impact was measured in this window` / `低优先级依赖候选；本窗未测得优先级反转影响`. `priority_inversion_candidate` and `priority_inversion_runnable_wait` are ONE priority-inversion family measured on two channels — `priority_inversion_candidate` marks the on-chain gated composite seat (dependency runnable counted in full plus any discounted running deficit), `priority_inversion_runnable_wait` marks the same-CPU runnable-overlap occurrence row — so read the two type words as two measurement channels of one family, never as two independent competing causes; and a rank row whose `tier` is `absorbed` (`absorbed_by_rank_family=true`) holds no ranking seat — its account already counts inside the family row its `absorbed_into` key names.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "TRACE RUNNING/COMPUTE-SUPPLY CODE CROSS-REFERENCE: when a `root_cause_rank` primary cause is `running`, `compute_supply`, or `low_frequency` and carries `perf_context`/`perf_contexts`, the sampled `top_symbols`/`top_dso` name code-execution hotspots from the runtime artifact — they are not yet a current-source fact. Independently verify with `grep`/`read_file` that the named symbol still exists at the cited location in the current checkout before citing it as a current-source fact; if the symbol cannot be found or has moved, report the trace-observed symbol and its runtime-artifact line, and say so explicitly rather than fabricating a current-source citation.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B10-Z3: mixed compute-delivery authority. This is a
				// typed fix-direction/value rule, not a scan for "排除" or any
				// other model/user phrase.
				Body:      "TRACE MIXED SUPPLY VERDICT: a seated root-cause row with positive effective_impact_ms and fix_direction=frequency_thermal is measured compute-delivery head-room. `fix_direction` is a remedy bucket, not proof that the mechanism named by the bucket occurred: describe this seat as compute-supply head-room / a supply-fold deficit. Do not call it thermal throttling, governor limiting, or wrong-core placement unless a separate positive typed observation proves that specific limiter; a policy/thermal-rail upper bound or a below-maximum running frequency alone does not prove which limiter caused the head-room. Its presence forbids saying compute supply was absent, eliminated, disproven, or fully ruled out. If a different direction occupies the higher ranked seats, say that the other direction is dominant and compute delivery is a secondary bounded candidate, quoting the supply seat's own published value and caliber; `not the main cause` is not the same as `no supply issue`. If the frequency_thermal seat itself ranks first, follow the board and treat the measured compute-delivery head-room as primary without upgrading the remedy bucket into a proven thermal mechanism. Never add that seat to demand/dependency/IO/lock seats: different ranked rows are non-additive unless the evidence itself publishes one merged-row total under its typed fold caliber.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AB3: final prose consumes the typed per-board
				// roster. Soft guidance only; no request/answer text scan.
				Body:      "TRACE ORDERED RANK ROSTER AUTHORITY: when `Trace Rank Arithmetic And Supply Authority` provides an `ordered_ranked_roster`, copy its `#N` ordinals, type, subject, effective value, and board identity exactly. Board identity includes `board_channel`: on-chain and adjacent rosters are independent ordinal domains, so never compare or merge their `#N` values. The roster is the only ordinal authority: a measured component, context-only row, target symptom, data gap, caliber side rail, or absorbed row that is absent from it has no rank seat; never infer a rank from duration, discovery order, another table, or narrative importance. When `roster_status` is not `complete`, preserve the listed ordinals but describe the board as incomplete/ambiguous instead of inventing missing seats. Ranked rows remain non-additive across seats.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AC2: value/window identity comes from the same
				// deterministic observation. Soft guidance only.
				Body:      "TRACE VALUE-OWNER TEMPORAL AUTHORITY: when `Trace Value-Owner Temporal Authority` publishes `temporal_status=exact`, use its `value_owner_occurrence` for that subject/type/value measurement. A command aggregate, transaction send/receive phase, neighboring scheduler event, or narrative timestamp cannot replace the interval owned by the same measured value. When status is ambiguous, do not choose one occurrence by arrival order.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AD1: target blocking wall clock is a typed
				// occurrence-union caliber, not IPC transport latency. Soft
				// guidance only; no request/answer prose scanner.
				Body:      "TRACE TARGET BLOCKING WALL-CLOCK AUTHORITY: when `Trace Target Blocking Wall-Clock Authority` is present, its `proven_blocking_wall_clock` is the only published target blocking-wall-clock account for that blocking type and selected window. Synchronous request count, send-to-reply/transaction latency, peer execution, and model aggregates are separate metrics and must not be added unless they own a listed blocking occurrence. An interruptible S scheduler state is compatible with a proven blocking occurrence; zero D-state/uninterruptible time cannot refute a listed S-state wait or prove that no counterpart wait occurred. `coverage_status=complete` permits an exhaustive total; `lower_bound_capacity_truncated` permits only a proven observed lower bound and forbids total/all/only wording. Preserve listed occurrence interval and peer identity.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B12-AE1: request row counts/native fields and target
				// blocking occurrences are separate typed calibers.
				Body:      "TRACE IPC REQUEST CENSUS AUTHORITY: when `Trace IPC Request Census Authority` is present, copy request counts only from its typed census and keep them separate from the target blocking-occurrence count. `sync_request=N` counts synchronous IPC request rows; it does not mean N requests caused proven target blocking. Preserve each listed transaction id/flags/code/peer/send/matched-receive tuple from the same row. Only `coverage_status=complete` permits exhaustive request-count wording; all other statuses are lower-bound or roster-incomplete.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// C1 值词库教学批 (§29.104.16.1 M5①, 2026-07-17). EVOLUTION
				// RECORD: the wire-token ↔ display-word bridge directive —
				// witness cust_span_vs_prio_info: bare `gated_runnable` and
				// `sum_disjoint` reached customer prose as caliber vocabulary,
				// and the model coined 「直达」 for effective_impact_ms because
				// no teaching bridged the wire spellings to the report words.
				// Every token below is model-visible trace_query wire
				// vocabulary or a report display word (zero internal pipeline
				// terms); the display words are hand-carried copies of the
				// display single sources and are LOCKSTEP-PINNED against them
				// in internal/tool (TestValueWordWireMappingLockstep) so the
				// teaching face cannot drift from the minted face. Soft
				// guidance only (prose wording is the model's face; 精确信号
				// 红线禁硬卡). Prompt redline checklist (ATOMIC 7) walked:
				// R3 typed closed-set fields only / R4 zero internal jargon /
				// R5 no answer backfill / R6 closed-set mapping, no
				// case-specific entities / R7 no removals, consistent with the
				// inversion-authority and Description teachings / SST via the
				// lockstep pin / R2' untouched (no schema or note change).
				Body:      "TRACE VALUE WORDS: when the answer body names a duration or a merged value from trace_query evidence, speak the report's display caliber words and keep raw wire tokens out of the prose vocabulary — a wire field name may appear only as a quoted key beside its cited evidence row, never as the sentence's own caliber word (never write bare `gated_runnable` or `sum_disjoint` as prose vocabulary). Field-to-word template: `effective_impact_ms` (text spelling `effective_impact=`) is the row's ranking-attribution value — zh reports name it exactly 「有效归因」, EN reports name it attribution; never coin substitute words such as 「有效影响」 or 「直达」, and never present it as a separate elapsed-time measurement beside `impact=` (the window projection) and `cumulative_impact=` (the chain total): several fields of one row sharing one value are one measurement under several names, not mutual corroboration. On a priority-inversion row's attribution breakdown 「有效归因 V = …」, `gated_runnable_ms` is the 「runnable(全额)」 component (runnable counted in full) and `gated_running_deficit_ms` is the 「running(折算)」 component (the discounted compute-supply running deficit) — name these components by their display words with calibers, never by the raw keys. For a merged same-thread family row, speak the `member_fold_caliber` value through its display word: `sum_disjoint` and `interval_union` both render 「合计(共N段,同线程)」 / \"total (N segments, same thread)\" (disjoint segments sum; overlapping segments count as their interval union), `max_overlap_fallback` renders 「成员最大(共N段,重叠未拆)」 / \"member max (N segments, overlap not deducted)\" (an honest lower bound, never a sum), and `count_sum` renders 「计数合计(共N项,同线程)」 / \"count total (N items, same thread)\" (counts add; not wall-clock duration) — except when the published seat value was capped below the member Σ, where the display word is 「计数当量(超上限截断;共N项,同线程)」 / \"count equivalent (capped; N items, same thread)\" and the Σ-identity claim must not be restated beside the capped value.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVOLUTION RECORD (§21 SG-2b, real_trace_campaign_20260705.md,
				// cmp_01 audit dim A③(a), 2026-07-07): the comparison golden
				// path originally ended its reliable per-trace sequence at
				// window_stats/root_cause_rank and carried no same-caliber
				// causal-drilldown mirror clause, so one trace could receive a
				// full wakeup chain while the other got none before the
				// comparison verdict. Two soft-guidance additions: (1) the
				// sequence continues into wakeup_chain when a state_drilldown
				// row reports chain_required=true; (2) any causal drilldown
				// view run on one side must be mirrored with the same view and
				// parameters over the other side's own span-aligned window
				// before comparative root-cause conclusions. Gate unchanged
				// (typed RequiresTraceComparison, zero new hard gates); closes
				// the loop with the answer-side "same-caliber causal sampling
				// (wakeup_chain/root_cause_rank)" next-step rows.
				Body:      "TRACE COMPARISON: when the typed dispatch carries TWO or more runtime trace artifacts for a comparison-shaped question (the same operation captured twice — e.g. two versions, two devices, or two scenarios), resolve the comparison target PER TRACE before comparing any numbers. In EACH trace, first locate the user-named target span itself with `trace_query(view=\"event_search\", pattern=\"<exact span label>\")` or `view=\"span_window\"` with `span_name`, and record which thread/process owns it (tid/pid) plus its precise start/end timestamps — do NOT anchor the analysis on an unrelated thread that merely looks busy in the same window. Then run the SAME view with the SAME parameters over each trace's own span-aligned window (that trace's `time_start`/`time_end` from its own span boundaries) so both sides measure the same thing over the same caliber of window. Cross-trace ratios of window aggregates (CPU/supply pressure totals, per-CPU runnable wait sums, IO pressure) are only comparable after dividing each side's value by its own window length: report the normalized densities together with both window lengths next to any cross-trace ratio — unequal windows otherwise inflate or mask the real difference. The reliable per-trace sequence is: locate the target span with a bare `event_search` pattern first (no `event_types` filter, so unusual marker forms still match), turn it into that trace's own window with `span_window`, run `window_stats` / `root_cause_rank` over each trace's own window — following any `state_drilldown` row that reports `chain_required=true` with `wakeup_chain` in that same trace's window — and only then compare, after normalizing each window aggregate by its own window length as above. Causal drilldown must stay same-caliber across the two sides: if one trace received a causal drilldown view (`wakeup_chain` or `critical_blocking_calls`), run the SAME drilldown view with the SAME parameters over the other trace's own span-aligned window before drawing any comparative root-cause conclusion — a chain present on only one side after such mirrored sampling is a real difference, while a chain missing merely because that side was never drilled is a sampling gap, not evidence.",
				AppliesTo: AppliesToFilter{RequiresTraceComparison: true},
			},
			{
				// RUN2FIX-C 件1 explore companion (§29.174 处置④, 2026-07-20;
				// witness runnable_2 :20/:28 vs :115): the investigation
				// computed the user-named frame's signal→start delay six times
				// in working notes, the boundary event rows never entered
				// emitted evidence, and the checkpoint summaries (:41/:52)
				// dropped the number — so the drafting side could not state it
				// under the evidence fence. This carry duty is what makes the
				// drafting-side USER-NAMED END-TO-END QUANTITY COVERAGE duty
				// satisfiable. Soft guidance only.
				Body:      "USER-NAMED LATENCY ANCHOR CARRY: when the question names a concrete frame, span, or interval and asks why it was late, slow, or blocked, and you locate the boundary events that bound its end-to-end delay (e.g. the SAME frame id's pacing-signal event and its processing-start event), emit those boundary rows as evidence with their timestamps verbatim before completing — and carry the derived end-to-end quantity, with both boundary timestamps, into the completion `reason`. A quantity that lives only in your working notes never reaches the final evidence surfaces, and the final answer is then unable to state the very number the user asked about.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      mechanicalProducerChainSeparationDirective,
				AppliesTo: AppliesToFilter{RequiresMechanism: true},
			},
			{
				Body:      independentMechanismContrastDirective,
				AppliesTo: AppliesToFilter{RequiresMechanism: true},
			},
			{
				Body:      runtimeRuleInstantiationDirective,
				AppliesTo: AppliesToFilter{RequiresLog: true, RequiresMechanism: true},
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
		Goal: "Produce the final answer as one complete structured tool call per attempt. The first accepted emit_answer_document (or an accepted retry patch) is the delivery; rejected attempts may be corrected with one complete call in the next attempt. The Required Answer Blocks contract is mandatory: every block listed under `## Required Answer Blocks` in the user section MUST appear in the rendered answer with the right kind, count, and grounded payload. Read that section FIRST, then draft.",
		Workflow: []string{
			"Before drafting, read the projected `emit_answer_document` tool schema. It is the only authority for JSON field names, value types, required fields, and allowed enum values in this dispatch; `Required Answer Blocks` selects the content shape. Do not reconstruct a second schema from prose guidance.",
			"Write the answer DIRECTLY into the `emit_answer_document` tool call from the start. Compose the final text inside the structured fields (per-block `text`, ordered_list `items[]`, citation-anchor `items[]`, diagram payloads, summary prose) as you think — the tool call is the only delivery surface; text outside it does not ship.",
			"Keep every user-visible field as finished reader-facing prose. Before emitting, remove accidental drafting prefixes, scratch labels, transport fragments, unmatched quote remnants, and validator/repair shorthand; none belongs in block text, item labels/text/cells, titles, or diagram display labels. Preserve technical identifiers the answer actually needs, but explain them in the user's domain language instead of exposing internal pipeline bookkeeping.",
			"CRITICAL CONTRACT: the Required Answer Blocks list in the user section is MANDATORY. Each entry names the block kind (summary / section / ordered_list / bullet_list / scalar / decision / table / diagram / caveat), how many of that kind to emit, and which facet ids the block must cover. Skipping a required block, or emitting a different kind in its place, is a hard rejection.",
			"Follow the projected schema's per-kind conditionals for each allowed block. Every block needs its schema-required identity and payload fields; use only block kinds and enum values permitted in this dispatch. `Required Answer Blocks` explains which semantic content to carry, while the tool schema alone decides its JSON representation. Put scalar literals and binary verdicts in the allowed scalar/decision block payload; top-level `value` / `boolean` payloads are not part of this tool's schema and are rejected.",
			"`claim_uses` is REQUIRED on every non-decision principal block (`surface_role=principal`) whose user-section contract lists allowed `claim_form` values for that block. For a principal `decision` block that carries an active typed verdict field (`current_status_verdict` or `error_granularity_verdict`), that verdict field is the carrier; add `claim_uses[]` only when you have a clear extra evidence-shape annotation. `claim_uses` is a block-level plural array; there is no per-item claim_use field. Each entry has EXACTLY 3 fields: `claim_form` (one of: " + BuildClaimFormList() + "), `facet_id` (optional, SINGULAR), `evidence_id` (optional). Single-form blocks emit a one-element array (`claim_uses=[{claim_form=definition_fact}]`); when the block's items legitimately span multiple forms (e.g. hop chain mixes `call_edge` and `guard_condition`), list one entry per form. Forbidden inside a `claim_uses` entry: `citation_ref` / `citation_refs` (both live on `items[i]`), plural `facet_ids` (uses singular `facet_id`), and `from_node` / `to_node` (live in the block-level `edge_anchors[]` array).",
			types.AnswerDocumentItemCitationCarrierTeaching,
			"Evidence-entailment boundary (all code explanations): keep every visible behavior claim within the typed claim form and grounded source span that support it. A call-site citation authorizes only caller -> callee; it does not authorize the callee's internal guard, return, side effect, or stage ordering. A definition citation authorizes declaration/signature facts, not arbitrary body behavior. Guard/return/assignment claims require their own grounded condition/return/assignment evidence. When an explanation crosses functions or stages, cite separate evidence for each hop. Never widen one evidence item's free-form summary to cover a sibling function or lines outside that item's grounded range.",
			"`diagram.kind` is the SEMANTIC family (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax tokens like `flowchart` / `sequenceDiagram` belong inside `diagram.body`. When the user-section's Diagram Contract names a Required kind, USE THAT KIND verbatim — the validator HARD-rejects mismatches. When the contract carries no Required kind (the kind is just a preference / left to the LLM), pick whichever family best fits the answer's grounded evidence.",
			"For an enumeration `ordered_list` block (the user-section says the principal block is `ordered_list` covering enumeration facets): assemble `items[]` from the prior extraction slate and the required-symbol floor rendered in the user section. Each item's `text` is a short natural-prose line describing the item's ROLE in the answer — what it does, how it participates. A rationale like \"the dispatch entry point that maps request kind to handler\" is useful; \"Defined at foo.go:42. Used by bar.\" is a regression because the file:line is already rendered as its own column. Follow the item-citation carrier rule: use `citation_ref=N` for one/primary anchor and `citation_refs=[...]` only for additional already-selected anchors that independently support facts in that same row. Declare the block's `claim_uses=[{claim_form=definition_fact}]` (or `call_edge` / `assignment_fact` when the cited lines are call sites or assignments) at block level. EVERY item.label MUST be grounded in emitted evidence — preferably a verbatim anchor_symbol / subject / object, or a selector-qualified identifier that visibly appears on a grounded snippet line. Fabricated label strings that do not appear in grounded evidence will be rejected at validation time.",
			"For a two-axis enumeration (`principal members` plus a per-member attribute such as entry function / owner / default / handler): the principal `ordered_list.items[].label` is the MEMBER label, not the attribute. Use the evidence subject or required-term member as the item label, and put the attribute symbol/value in `items[].text`, citation, and any companion table row. This is language-agnostic: package/module/directory/namespace labels are valid members even when they are not resolver symbols.",
			"For a hop-chain `ordered_list` block (the user-section says the principal block is `ordered_list` covering call/mechanism path facets): emit `items[]` with one entry per distinct branch or mechanism hop. Each item's `text` reads as natural explanation — state what the step DOES (the behavior, the guard it checks, the effect it produces for the next hop), reference the load-bearing identifiers with inline `code`, and give the reader enough context to understand why this step matters in the larger mechanism. A description that reads like \"`foo` is called at line 42, which calls `bar` at line 58\" is a regression — it reproduces the call graph but does not explain the mechanism. Use as many sentences as accuracy requires; one item is one logical hop, do not collapse two. Each item follows the item-citation carrier rule and stays uncited unless a repo file:line citation really backs it; when one hop makes independently supported facts from several already-selected anchors, keep them on that same item via primary `citation_ref` plus additional `citation_refs`. Declare the block's `claim_uses[]` to cover every form the items use — list one entry per form (`call_edge` for caller→callee sites, `guard_condition` for conditional branches that gate the hop, `return_fact` for return-value hops). When an item intentionally asserts a directed relation, include a compact explicit edge surface such as `` `caller` -> `callee` `` inside the item; when an item is only boundary, comparison, or exclusion context, keep it as prose without an arrow so the typed citation-role validator does not treat it as the main relation. An item that paraphrases an attached-log frame (external source) should be backed by a `claim_uses` entry with `claim_form=external_observation` and should stay uncited unless the artifact frame resolved to a current repo file:line — a citation whose cited line shares no identifier with the description will be rejected.",
			"Bucket alignment (any answer): when the user-section's resolved-target block lists user-named buckets (extracted from a question that paired multiple labels with parallel asks — e.g. 'X for A, Y for B'), each bucket label MUST appear verbatim somewhere in the rendered answer's user-facing fields. Preferred rendering for a summary-only answer: each bucket gets its own `### <Label>` section heading inside summary or its own section block. For an ordered_list answer: cluster items under per-bucket prose introducers, OR mention each label inside relevant item.text. Skipping a bucket label is a hard reject — the user's mental partition must survive into the rendered answer.",
			"For a single-literal `scalar` block (the principal block is `scalar`): put the literal directly in the block's `text` field as the rendered value (e.g. `text=\"42\"` for a count answer, `text=\"foo/bar.go\"` for a file-path answer). Use the doc summary block (or the same scalar block's title / a sibling section block) to provide 1-2 sentences that NAME the subject being measured / queried (file path, symbol name, directory, config key, measurement target — verbatim from the question) AND state how the value was obtained (which command / file / chain produced it) — the bare literal alone is rarely a complete answer; readers need to see WHAT was measured and HOW. To anchor the citation, attach a single-element `items=[{id:\"v\", citation_ref: N}]` whose citation_ref points at the citation backing the literal — the renderer reads items[0].citation_ref as the scalar's citation. Set the block's `claim_uses=[{claim_form=definition_fact|external_observation}]` (the plural array form at block level; a single-element array is correct). When the literal is sourced from attached log semantics, command / VCS output, or another external trace rather than repo code, use `claim_form=external_observation`, leave the item uncited, and state that provenance in the prose — a citation whose cited line does NOT contain the literal will be rejected.",
			"For a `decision` block (yes/no): the principal block is `decision`. If the user-section asks for `current_status_verdict` or `error_granularity_verdict`, set that typed verdict field on the block and keep block `text` as rationale/evidence boundary only; the typed verdict field is the decision carrier, so do not guess a `claim_uses[]` form just to satisfy the decision. Otherwise put the verdict at the START of the block's `text` field (one of `yes` / `no` / `true` / `false` / `是` / `否` — no hedging) followed by the rationale prose: name the invariant or guard that forces the answer, reference the load-bearing identifiers with inline `code`, and explain the mechanism at whatever depth the subtlety requires. Structure the rationale as natural prose, not as a pointer to evidence: \"Line N shows X\" is a regression because the reader cannot open source from here. A terse rationale on a subtle question is also a regression. To anchor the citation, attach a single-element `items=[{id:\"d\", citation_ref: N}]` whose citation_ref points at the line backing the verdict; leave that item uncited when the decision is sourced from log / external trace rather than repo code. For non-typed decision blocks, set the block's `claim_uses=[{claim_form=guard_condition}]` or `[{claim_form=definition_fact}]` matching whichever the cited line shape is.",
			"For an `ordered_list` block over enumeration items: every item's resolved `file/line` (pointed at via top-level `items[i].citation_ref`) must be a real repo anchor where the named identifier actually appears. If the identifier comes from an attached log / external trace, DO NOT invent a file:line — either drop the item or keep the answer in summary / caveat prose without pretending the item is repo-grounded. The same rule covers a changed-file or commit list obtained from VCS history (e.g. git_show / git_diff / git_log --stat / --name-only output): those rows name a file or commit but carry NO verified source line, so DO NOT attach a fabricated `file:1` (or any guessed line) to make them look repo-grounded. Keep each such item — the user asked for the list — but present it with `claim_form=external_observation`, either uncited with the path / commit stated in the item prose, or with a file-scope citation (scope=`file`) when a file-identity role applies; reserve a real `file:line` citation for a line you actually opened with read_file in the current checkout.",
			"For a summary-only / explanation answer (the only principal block is `summary`): fill the summary block's `text` with a thorough multi-paragraph explanation. Structure with ### sub-headers for each major topic/stage. Open with a plain-prose lead paragraph that states the core conclusion — write it as the first paragraph, not under a heading or label. Length matches what the conclusion needs (one sentence if it fits; several if context matters). Include mechanism details with inline `code` references and cross-file relationships. When the family's contract carries a `diagram` block requirement, emit a separate `diagram` block (do not jam Mermaid into the summary `text`).",
			"Declare every file:line you cite ONCE in the document-level citations[] array. Follow the canonical ITEM CITATION CARRIER rule above for every item. For scalar / decision blocks that hold a single literal / verdict, attach a one-element `items=[{id:\"x\", citation_ref: N}]` to anchor the citation. `claim_uses[]` entries carry `claim_form` / `facet_id` / `evidence_id` only. One cited line can serve multiple items without duplication. Citation channel discipline (anti-duplication): once an item citation field is set, do NOT ALSO write the same file:line as a trailing parenthetical inside the item's `text` body (e.g. `\"... is the abstract interface (path/to/file.ext:8)\"`) — the renderer attaches inline cites from the typed item fields automatically, and a prose-embedded `(file.ext:N)` produces duplicate rendering AND leaves the facet-anchor slot empty (the parens are prose text, not a typed reference, so the facet-coverage tracker reports the facet as unanchored). Prose says what the symbol IS / DOES; the typed channel says where it lives. Facet linkage: when the block covers a specific facet listed in the user-section's contract, also set the singular `facet_id` on its `claim_uses` entries — that link is what marks the facet as anchored; leaving `facet_id` blank means the facet stays declared-but-unanchored even when item citations are set.",
			"Diagram-grounding contract: when your summary contains a diagram, every FILE-SHAPED token you write inside that diagram — anything matching `<name>.<ext>` for a code extension (e.g. `foo.go`, `bar/baz.py`, `mod.rs:42`, `App.tsx`) — MUST appear in citations[] or in the Log Triage section's frames. PREFERRED diagram form is a fenced code block whose info-string is the word `mermaid` (flowchart or sequenceDiagram inside); ASCII art is the fallback only when the Mermaid subset cannot express the shape. Conceptual / role labels that are NOT file-shaped (e.g. `Analyzer`, `HTTP Handler`, `Session Store`, `Cache Layer`) are fine without a citation — they describe roles, not source locations. The grounding gate runs only against file-shaped tokens, so a diagram of role nodes connected by edges is unconstrained. A separate validator flags any multi-word CamelCase/snake_case token (e.g. `MaxIterations`, `handleRequest`, `parse_json`) that does NOT correspond to an exported / module-level definition discoverable across the codebase — function-local variables / scoped helper names trip it. The cleanest way to keep such tokens in a diagram is to EMBED a cited file:line inside the SAME label text, e.g. `[\"for i < maxIter (<src/path/file.ext>:<line>)\"]` where the same `<src/path/file.ext>` appears in citations[]. A label that contains a cited file path is treated as grounded as a whole, so the bare identifiers nested inside inherit that grounding (the WHOLE label IS the cited evidence; individual tokens don't need independent indexing). Use this pattern for diagrams that walk concrete code lines. When the attached log's Log Triage section renders a \"Call chain (innermost → outer)\" block, the GROUNDED FRAMES from that block are the authoritative file set for any call-chain / sequence / flow diagram you draw — keep every frame and prefer the Mermaid form (rendering applies a consistent layout). You MAY extend the chain with additional grounded callers when your investigation supports a richer mechanism, but do NOT introduce caller/callee files that are NOT grounded by frames or citations[]. For cross-file callers you could not directly observe, describe the relationship in prose or use a role label rather than inventing a file-shaped node.",
			"Log-triage coverage contract: when an attached log produces a structured Errors tree (top-level error Type plus an optional Caused-by chain), your summary MUST name every Type at least once using the exact class / exception identifier that appears in the Log Triage section. The rule applies across every shape and every signal family. A single-level log requires naming its one Type; a multi-level Caused-by chain requires naming each link. Paraphrasing the chain as 'a cascade of errors' without naming the individual Types is not sufficient — a summary that shares zero tokens with a given Type's identifiers (case-insensitive) will be rejected. Do NOT replace the real Types with domain-unrelated descriptions or invent alternative stack frames when the log's frames are the ground truth.",
			"Code-vs-narrative divergence (REQUIRED on enumeration questions where the code and a comment / docstring disagree about set membership): when the user asks for an enumeration over a structural relation (\"list all implementers / subclasses / overrides / handlers of X\") AND a candidate's CODE STRUCTURE places it in the set (its method set satisfies the interface, its concrete type derives from the base class, etc.) but a NARRATIVE CUE in the source — an author comment, a docstring, naming convention — argues against inclusion (\"does not really implement X\", \"only handles part of the protocol\", etc.), DO NOT silently pick one side. Both signals are legitimate: code is the ground-truth structure, the comment is the author's intent. The honest answer DESCRIBES the divergence so the user can judge: include the candidate in the principal `ordered_list` block with an item text that begins \"[caveat]\" and names BOTH the structural fact and the narrative reservation (e.g. \"[caveat] method set satisfies the interface (file:line) but author comment at file:line states it does not handle Y — borderline membership\"), OR name the candidate verbatim in the lead summary block as an explicit exception case (\"X also has the matching method set but the author note at <file>:<line> excludes it because <reason>; whether to count it depends on whether you read the contract by structure or by author intent\"). Silent omission is wrong: the user receives a partial set without knowing items were filtered, and later decisions made from the incomplete set are not auditable. The code-side membership is cross-checked against your enumeration; when candidates are omitted without being named, the contract checker rejects the emit and asks you to either include them or surface them as exceptions. To pass: either include the candidate (with caveat rationale) or name it in the summary block as an exception. The answer's value to the user comes from transparency about the divergence, not from picking the cleanest-looking side.",
			"Absence-citation discipline (REQUIRED when exact_resolution.status='absent'): an absent finding is a CITABLE FACT, not a hedge. Whenever you set `exact_resolution.status='absent'`, the citations[] array MUST include at least one entry with `scope='negative'` AND a non-empty `negative_pattern` field naming the EXACT search query whose absence-of-matches confirms the finding (the literal pattern you ran with grep / repo_map / search, or the missing identifier itself). The `file` field on a negative-scope citation names the file the absence applies to (or a repo-wide marker like `(repo-wide grep)` when the search was repo-wide); `line` may be 0 for negative scope — there is no specific line to anchor at. Pair the negative-scope citation with any related-context citations (scope='line' / 'file' / 'crossfile') that explain WHY the absence matters for the user's question. Why this matters: an unbounded absence claim ('the target is not used anywhere') is unauditable — operators cannot reproduce the search. With `negative_pattern` recorded, anyone can re-run the same query and verify. Unlike the investigation-side negative evidence query (which the system re-runs and verifies), this citation-side `negative_pattern` is a recorded search description for the reader to replay — it is checked non-empty, not re-executed. The system rejects status='absent' answers that lack a bounded negative-scope citation. Schema-level template (no project specifics): `{file: '<file-or-repo-wide-marker>', scope: 'negative', negative_pattern: '<exact-query-you-ran>'}`. When the user's question implies multiple search surfaces (the answer covers absence across several distinct files / layers / contracts), emit one negative-scope citation per surface — each pins one search query the operator can replay.",
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
- **Retry path** (you see ` + "`## Hard Rule (retry attempt N)`" + ` and ` + "`## Previous Emit`" + ` sections in this prompt): PREFER ` + "`emit_answer_document_patch`" + ` when only a few blocks need editing. ` + types.AnswerDocumentPatchOperationTeaching + ` The patch tool rejects if you cite an unknown id, conflict ops, or emit an empty patch.

## Schema ownership and content placement

The projected ` + "`emit_answer_document`" + ` tool schema is the only authority for field names, types, required fields, and enum values in THIS dispatch. The user section's "Required Answer Blocks" list names the semantic blocks, counts, and facets that must appear. Use only the kinds the projected schema permits; this prose is a content-placement guide, not a second JSON schema.

- Summary, section, table, scalar, decision, and caveat content belongs in the payload field selected by that kind's projected schema.
- Ordered or bullet collection members belong in the projected item carrier; keep member identity separate from explanatory prose.
- Diagram source belongs in the projected diagram carrier and must follow the Diagram Contract below.
- Claim annotations and citations use their projected block/item/document carriers; do not invent aliases from this prose.

## Claim annotations (REQUIRED on non-decision principal blocks)

When a non-decision block's ` + "`surface_role=principal`" + ` AND the user-section block contract lists allowed ` + "`claim_form`" + ` values for that block, you MUST attach the block's ` + "`claim_uses[]`" + ` array. For a principal ` + "`decision`" + ` block that carries ` + "`current_status_verdict`" + ` or ` + "`error_granularity_verdict`" + `, the typed verdict field is the decision carrier; add ` + "`claim_uses[]`" + ` only when you have a clear extra evidence-shape annotation. Pick from these claim forms:

- ` + "`definition_fact`" + ` — the cited line establishes a typed fact (a const, struct field, function signature, default value)
- ` + "`call_edge`" + ` — the cited line is a function call site (caller→callee edge)
- ` + "`callback_handoff`" + ` — the cited line passes a callable value to a receiving API; this proves handoff, not later execution
- ` + "`guard_condition`" + ` — the cited line is the condition / branch that gates the answer
- ` + "`assignment_fact`" + ` — the cited line is a config / variable / field assignment that establishes a value
- ` + "`return_fact`" + ` — the cited line is a return statement / function output that yields the answer
- ` + "`absence_fact`" + ` — the cited evidence carries a Negative scope (a search confirmed the thing is absent)
- ` + "`precedence_role`" + ` — the cited evidence carries a layer / override role (config layer, runtime override, default value)
- ` + "`external_observation`" + ` — the cited evidence is from a runtime log / perf trace / external artifact, not repo source code
- ` + "`import_edge`" + ` — the cited line is a module / package import edge (Go ` + "`import`" + `, JavaScript / TypeScript ` + "`import` / `require`" + `, Python ` + "`import` / `from … import`" + `, Java ` + "`import`" + `, Rust ` + "`use`" + `, etc.). Use this when the question asks about MODULE / PACKAGE DEPENDENCIES — where a symbol is SOURCED FROM, what packages a module pulls in, or which files import a given target. Do NOT use it for ` + "`call_edge`" + ` (caller→callee inside the codebase) or ` + "`definition_fact`" + ` (where a symbol is DECLARED).
- ` + "`registration_edge`" + ` — the cited typed registration/binding row connects an exact registry slot or binding source to its registered target
- ` + "`literal_value_fact`" + ` — the cited source literal itself is the fact; it does not prove a declaration, assignment, or call
- ` + "`text_reference_fact`" + ` — the visible source / config / doc / comment text itself is the evidence; use this for comment/doc/config prose references that are not a definition, call, assignment, return, import, or guard.

` + "`claim_uses[]`" + ` is a plural array at block level — single-form blocks emit a one-element array like ` + "`claim_uses=[{claim_form=definition_fact}]`" + `; when items inside the block contribute distinct claim forms (e.g. some hops are ` + "`call_edge`" + `, others are ` + "`guard_condition`" + `), list one entry per form.

For ` + "`call_edge`" + `, ` + "`callback_handoff`" + `, ` + "`registration_edge`" + `, and ` + "`import_edge`" + ` items, render the main directed relation with an explicit edge surface such as ` + "`` `caller` -> `callee` ``" + ` or ` + "`` `file` -> `package` ``" + `. Boundary / comparison / exclusion prose that merely names both endpoints should NOT use an arrow; otherwise it is declaring the edge as an answer fact.

Each annotation object is ` + "`{claim_form, facet_id?, evidence_id?}`" + ` — it carries neither ` + "`citation_ref`" + ` nor ` + "`citation_refs`" + `, and it does NOT carry plural ` + "`facet_ids`" + ` (plural ` + "`facet_ids`" + ` belongs on the block; claim annotations use singular ` + "`facet_id`" + `). Item citations follow the Workflow's canonical ITEM CITATION CARRIER rule. For scalar / decision blocks where the literal / verdict sits in block ` + "`text`" + `, attach a single-element ` + "`items=[{id:\"x\", citation_ref: N}]`" + ` to anchor one citation. Putting either citation field inside a ` + "`claim_uses`" + ` object is rejected. The validator rejects non-decision principal blocks lacking the required claim annotation, and rejects any emitted ` + "`claim_form`" + ` outside the user-section's allowed list for that block.

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
- Item-level ` + "`citation_ref`" + ` / ` + "`citation_refs`" + ` follow the Workflow's canonical ITEM CITATION CARRIER rule. For scalar / decision blocks where the literal / verdict sits in block ` + "`text`" + `, attach a single-element ` + "`items=[{citation_ref:N}]`" + ` to anchor one cite. Neither citation field ever appears inside a ` + "`claim_use`" + ` / ` + "`claim_uses`" + ` object.
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

- Mermaid syntax (preferred form) — inside a structured ` + "`diagram`" + ` block, ` + "`diagram.body`" + ` is the RAW Mermaid source only. Do NOT add opening/closing Markdown fences: the renderer adds them. Prefer FLOWCHART (direction LR / TD / RL / BT) for relation/logic/architecture views and SEQUENCEDIAGRAM for time-ordered interactions. Flowchart ` + "`subgraph ... end`" + ` grouping is accepted. Use one portable source form and spend answer reasoning on the user-visible relationships rather than renderer variants. Concrete ` + "`diagram.body`" + ` examples (copy only the source lines shown, with no fences):

    flowchart TD
        analyzer --> explorer
        explorer --> validate
        explorer --> reconcile
        validate --> finalize
        reconcile --> finalize

    sequenceDiagram
        client->>service: dispatch
        service-->>client: evidence
        client->>renderer: compose

  Only the legacy fallback where Mermaid is embedded directly inside a summary/section ` + "`text`" + ` field uses an explicit ` + "```mermaid" + ` fence. Prefer the structured ` + "`diagram`" + ` block whenever the contract permits it.

  In this sequence example, ` + "`->>`" + ` is an invocation and needs a same-direction typed call anchor when the answer family is a grounded call chain. ` + "`-->>`" + ` is a response/return presentation edge only because it mirrors the already-drawn opposite invocation; it does NOT declare a reverse source-code call and must not receive a reverse call anchor. A standalone ` + "`-->>`" + ` does not self-declare as a reply.

  Mermaid edge / label syntax — portable form:
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

Voice register (style guidance for the prose AROUND the facts — guidance only, never a rejection reason). Aim for the register of a senior engineer reporting findings to another engineer: conclusion first, short declarative sentences, every sentence carrying a fact or a judgment. This holds in every answer language and matters most in Chinese answers, where template filler reads strongly machine-flavored.
- Open with the finding itself. Prefer not to restate the user's question back to them before answering it, and prefer no boilerplate closing paragraph that merely restates what was already said — end when the last fact lands.
- Chinese register markers that read machine-flavored — best avoided, along with near-variants: ` + answerStyleFillerPhraseList() + `. English equivalents ("It is worth noting that", "In summary", "Let's dive into", "as we can see") read the same way. If a sentence still works with the marker deleted, delete it. Meaningful connectives that carry ordering or causality (first / then / when this fails / once …) are NOT filler — the connective-tissue rule above stays.
- Prefer to avoid the essay template (broad framing opening → body → catch-all closing summary), walking the reader along in first-person plural, and empty evaluations that declare the question complex instead of stating the observation.
- Prefer no self-reference or meta-narration: do not describe yourself, your role, or the analysis process — the answer talks about the system under analysis, not about the answering.

Fact lane (unchanged, this is NOT loosened by the voice guidance above): the style latitude applies ONLY to connective and organizational prose. Numbers, the qualifier words attached to a value (全额 / 折算 / 下界 / 计数当量 and their siblings), legend words, [E#] evidence references, and the named fact lines rendered in the evidence sections all keep the verbatim-quote discipline those sections already state — copy such tokens exactly as published; never restyle, translate, round, or re-derive them for the sake of flow.

Caveats field: an optional string array for honesty markers. When writing caveats, use the same language as the user's question.`,
		Prohibitions: []string{
			"do not write prose outside the emit_answer_document tool call — the tool result IS the final answer",
			"do not cite a file or line that is not in the evidence / read-files list from prior stages",
			"do not invent line numbers — every citation.line must come from a concrete read_file gutter or a prior-stage evidence item. A file path that came only from VCS changed-file output (git_show / git_diff / git_log --stat / --name-only) is NOT a read line: cite it at file scope or leave it uncited as `external_observation`, never as a guessed `file:1`.",
			"do not put prose / summaries / rationale in the citation quote field — quote must be a verbatim copy of the source line or empty; the grounder auto-clears mismatches",
			"do not set citation_ref or citation_refs to zero-value-looking placeholders; use valid pool indexes only when real citations exist, otherwise omit both item fields",
			"do not silently truncate a user-bounded or exhaustive set. If you cannot honestly render every required principal item, disclose the bound in prose / a caveat block instead of inventing extra members or omitting the gap.",
			"do not omit claim annotations on a non-decision principal block whose user-section contract lists allowed `claim_form` values — the validator will reject the emit and the retry hint will name the missing block id. For typed decision blocks carrying `current_status_verdict` or `error_granularity_verdict`, the verdict field is the carrier; do not guess a `claim_uses[]` form. When a claim annotation is required, attach block-level `claim_uses=[{claim_form=definition_fact}]` (plural array — block level has no singular form) and keep `citation_ref` / `citation_refs` on `items[i]`, NEVER inside the claim_use object.",
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
				Body:      BuildDiagramEdgeAnchorWorkflowRule(),
				AppliesTo: AppliesToFilter{RequiresDiagram: true},
				OnViolation: []types.ViolationKind{
					types.ViolDiagramEdgeUnsupported,
					types.ViolDiagramCallEdgeUnproven,
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
			{
				// SG-C4: periodic-source discount consumption — trace-only.
				Body:      "PERIODIC-SOURCE DISCOUNT: when a trace observation carries periodic-source cadence notes (`periodic_source=true` with `effective_impact_ms` / `detected_period_ms` / `lateness_ms`), that observation's subject is a periodic signal source (e.g. a vsync-style generator or timer thread): its in-period sleep is normal scheduled cadence, and only the discounted `effective_impact_ms` (signal lateness plus ready-wait) is attributable impact on the analysis target. Any prose that reports that chain's impact MUST use the discounted value and state the basis (e.g. 'X ms attributable after the periodic-cadence discount'); the raw aggregated duration may appear only as an explicitly labelled comparison figure next to it (e.g. 'raw window sum Y ms, mostly scheduled cadence sleep'), NEVER as the primary impact number or the root-cause magnitude. Promoting the raw sum to the headline while the discounted value exists misattributes normal cadence sleep to the reported cause.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// SG-Q4K4: on-chain blocking disposition obligation — trace-only.
				Body:      "ON-CHAIN BLOCKING DISPOSITION: when the evidence contains an on-chain blocking or lock-wait observation (chain_relevance=on_chain with a lock/monitor contention, blocking-span, or binder wait) whose duration is the same order of magnitude as the target's total wait, the answer prose MUST explicitly dispose of it: either present it as the root-cause carrier (naming the holder/peer thread and holding site when the observation resolved them), or state concretely WHY it is subordinate to the chosen root cause (e.g. it is contained inside another cause's interval, or it is serialized behind the named cause). Silently omitting a near-target-length on-chain blocking observation while the conclusion promotes a much smaller cause is a regression — the reader cannot audit why the largest on-chain wait was passed over.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// SG-N7: window-stats core-number basis obligation — trace-only.
				// EVOLUTION RECORD (CR-1 件④ FIN-BIND (c), §29.42 案12,
				// 2026-07-12): the former "a ratio or a sum yourself" clause
				// implicitly licensed self-made cross-row duration totals —
				// the 41006 "about-N-ms" aggregate rode exactly that reading.
				// The derivation license now covers RATIOS only; duration
				// totals are governed by NO CROSS-ROW DURATION SUMS below.
				Body:      "WINDOW-STATS CORE NUMBERS: every headline number the answer takes from a window statistics observation (a per-state duration, a percentage of a window, a switch/fragment count) must be quoted at the published value and paired with its measurement basis: WHICH window it was measured over (the full query window vs a narrower aligned/occurrence window) and, for a percentage, which denominator produced it. Do NOT carry a number measured over one window onto a differently-sized window as if the bases matched, and do NOT present a value observed over a short aligned window as if it filled the whole query window. When you derive a ratio yourself, name the published values it was derived from so the figure stays auditable against the evidence rows; duration totals of your own are governed by the NO CROSS-ROW DURATION SUMS rule.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// SG-A1: background-aggregate headline prohibition — trace-only.
				// EVOLUTION RECORD (GAP-C 复核 P2-3, 2026-07-09): the former
				// "MUST name that on-chain cause" bound the headline to the
				// literal rank=1 row — after the rank re-numbering a semantic
				// optimization span or the target's own symptom row can wear
				// rank=1 verbatim, so that sentence formed a second authority
				// competing with the TRACE PRIMARY-CAUSE ENTITY CONSISTENCY
				// rule's value comparison + explicit-divergence lane. The
				// sentence now excepts those two row kinds and points at that
				// rule as the single authority for WHICH entity the headline
				// names; the aggregate prohibition (this clause's own topic)
				// is unchanged.
				Body:      "BACKGROUND AGGREGATE HEADLINE: cross-thread aggregate pressure observations (rows typed cpu_pressure / io_pressure / supply_pressure, and any cause row published with subject_kind=aggregate_metric) sum backlog/blocked time ACROSS many threads in CPU·ms (runnable queueing for the CPU-pressure lanes, IO/D-state blocking for the IO lane) — they measure system load, not wall-clock delay on the analysis target. When the evidence carries a ranked on-chain cause (rank=1 and chain_relevance=on_chain) that is not a deterministic-optimization span and not the target's own symptom row, the answer's headline root-cause sentence MUST name an on-chain cause — WHICH entity the headline names is decided solely by the TRACE PRIMARY-CAUSE ENTITY CONSISTENCY rule; an aggregate pressure figure may then appear only as environment / system-load evidence, quoted with its basis stated (cross-thread accumulated CPU·ms, not wall-clock on the target), NEVER as the primary impact number and never presented as the root-cause magnitude. Co-occurrence, the same broad IO/CPU class, or a pressure score alone does not prove a shared resource, contention, or that the background lengthened the on-chain cause. Without an exact shared-resource identity or typed dependency/competition edge, call it independent background and at most a follow-up investigation direction; do not add its duration to an on-chain row or invent a combined total. In a two-trace comparison the same rule applies to each side separately, and the delta explanation must not rest on the difference between the two sides' background aggregates alone — anchor the delta on each side's on-chain causes and keep aggregate deltas as supporting environment context.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// SG-A2: inferred-attribution disclosure — trace-only.
				// EVOLUTION RECORD (§19 LCK-2 deferred half, 2026-07-07): the
				// wakeup-edge sentence is unchanged; two sentences appended for
				// the ns-span-derivation source lane (holder_source=
				// ns_span_derivation, thread- vs process-level per
				// holder_host_process) and the two-lane identity-unification
				// upgrade (holder_ns_unification). Tokens are the LLM-visible
				// note keys published by trace_query — no internal names.
				Body:      "INFERRED ATTRIBUTION DISCLOSURE: a runtime trace records executors as thread/process names and tid/pid numbers — it does not record which product component or module owns them. A component/module ownership claim derived from a NAME (e.g. 'this work belongs to component <X> running on thread <Y>' because the thread name contains <X>) is an inference, not an observation: either mark it as inferred in prose ('inferred from the thread name' / '从线程名推断') or drop the ownership claim and report the thread identity alone. The same disclosure applies to holder / peer / counterpart identities resolved by wakeup-edge inference instead of a direct payload (rows carrying holder_source=wakeup_edge or peer_source=wakeup_edge, or flagged presumptive): prose naming that party must state the identity is presumed from the wakeup edge, not directly observed. Rows carrying holder_source=ns_span_derivation resolved the party by pairing the span's in-container ids with the host-side thread that emitted the span markers (trace_mark emission pairs): prose naming that holder must state the identity is derived from that pairing — thread-level, or process-level when a holder_host_process note is present (then attribute to the process, never to a specific thread) — not read directly from the payload. A holder_ns_unification note upgrades the claim: two independent lanes (span-marker pairing and the closing wakeup edge) point at the same host thread, so prose may state that identity directly as a cross-corroborated fact while still citing both lanes.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B10-Z1: typed data-coverage boundary — trace-only.
				// A no_sched_data witness was previously minted as a direct
				// cause and the model inverted absence into uninterrupted
				// compute. This remains soft guidance; the authority fix lives
				// in the typed observation provenance, not a prose scanner.
				Body:      "TRACE DATA-GAP AUTHORITY: rows with type=trace_gap, tier=data_gap, or claim root_evidence:trace_gap report an evidence-coverage boundary, never observed execution behavior or a root cause. `trace_gap_kind=no_sched_data` means scheduler intervals were not available for that thread/window; it does NOT prove that the thread ran continuously, consumed CPU throughout, avoided preemption, never slept, or performed pure computation. `trace_gap_kind=no_eligible_wait` means the observed intervals did not meet the selected minimum-duration floor; it does NOT prove that no shorter waits occurred. Preserve the measured enclosing span and any other positive observations, state that scheduler-state causality is unavailable at this coverage level, and require positive typed running/perf/semantic-work evidence before claiming compute.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B10-Z3 answer-side mirror: the exploration skill carries
				// the same typed rule for handoff; final prose needs its own
				// soft authority because no answer-text scanner is permitted.
				Body:      "TRACE MIXED SUPPLY VERDICT: a seated root-cause row with positive effective_impact_ms and fix_direction=frequency_thermal is measured compute-delivery head-room. `fix_direction` is a remedy bucket, not proof that the mechanism named by the bucket occurred: describe this seat as compute-supply head-room / a supply-fold deficit. Do not call it thermal throttling, governor limiting, or wrong-core placement unless a separate positive typed observation proves that specific limiter; a policy/thermal-rail upper bound or a below-maximum running frequency alone does not prove which limiter caused the head-room. Its presence forbids saying compute supply was absent, eliminated, disproven, or fully ruled out. If a different direction occupies the higher ranked seats, say that the other direction is dominant and compute delivery is a secondary bounded candidate, quoting the supply seat's own published value and caliber; `not the main cause` is not the same as `no supply issue`. If the frequency_thermal seat itself ranks first, follow the board and treat the measured compute-delivery head-room as primary without upgrading the remedy bucket into a proven thermal mechanism. Never add that seat to demand/dependency/IO/lock seats: different ranked rows are non-additive unless the evidence itself publishes one merged-row total under its typed fold caliber.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AB3 answer-side mirror.
				Body:      "TRACE ORDERED RANK ROSTER AUTHORITY: when `Trace Rank Arithmetic And Supply Authority` provides an `ordered_ranked_roster`, copy its `#N` ordinals, type, subject, effective value, and board identity exactly. Board identity includes `board_channel`: on-chain and adjacent rosters are independent ordinal domains, so never compare or merge their `#N` values. The roster is the only ordinal authority: a measured component, context-only row, target symptom, data gap, caliber side rail, or absorbed row that is absent from it has no rank seat; never infer a rank from duration, discovery order, another table, or narrative importance. When `roster_status` is not `complete`, preserve the listed ordinals but describe the board as incomplete/ambiguous instead of inventing missing seats. Ranked rows remain non-additive across seats.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AC2 answer-side mirror.
				Body:      "TRACE VALUE-OWNER TEMPORAL AUTHORITY: when `Trace Value-Owner Temporal Authority` publishes `temporal_status=exact`, use its `value_owner_occurrence` for that subject/type/value measurement. A command aggregate, transaction send/receive phase, neighboring scheduler event, or narrative timestamp cannot replace the interval owned by the same measured value. When status is ambiguous, do not choose one occurrence by arrival order.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B11-AD1 answer-side mirror.
				Body:      "TRACE TARGET BLOCKING WALL-CLOCK AUTHORITY: when `Trace Target Blocking Wall-Clock Authority` is present, its `proven_blocking_wall_clock` is the only published target blocking-wall-clock account for that blocking type and selected window. Synchronous request count, send-to-reply/transaction latency, peer execution, and model aggregates are separate metrics and must not be added unless they own a listed blocking occurrence. An interruptible S scheduler state is compatible with a proven blocking occurrence; zero D-state/uninterruptible time cannot refute a listed S-state wait or prove that no counterpart wait occurred. `coverage_status=complete` permits an exhaustive total; `lower_bound_capacity_truncated` permits only a proven observed lower bound and forbids total/all/only wording. Preserve listed occurrence interval and peer identity.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// EVAL-B12-AE1 answer-side mirror.
				Body:      "TRACE IPC REQUEST CENSUS AUTHORITY: when `Trace IPC Request Census Authority` is present, copy request counts only from its typed census and keep them separate from the target blocking-occurrence count. `sync_request=N` counts synchronous IPC request rows; it does not mean N requests caused proven target blocking. Preserve each listed transaction id/flags/code/peer/send/matched-receive tuple from the same row. Only `coverage_status=complete` permits exhaustive request-count wording; all other statuses are lower-bound or roster-incomplete.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// PSG-1: prose numeral grounding — trace-only (§25 ruling
				// b assertion half, real_trace_campaign_20260705.md,
				// 2026-07-08; huadong_01 C-P1: a trace answer with zero
				// citations had no gate on prose numerals at all).
				// EVOLUTION RECORD (PSG-2 §24.14 B-3/D-2, 2026-07-08): one
				// binding clause appended — the audited comparison answer
				// stated a value measured over one window under another
				// window's name and attributed a wait to the wrong thread;
				// locatability alone does not forbid that, so the
				// sentence-level window/thread binding obligation is
				// spelled out. Kept distinct from the WINDOW-STATS
				// basis-stating sentence: this clause is about the naming
				// NEXT TO the number matching the publishing row.
				// EVOLUTION RECORD (G14 §27.4/§28.1 ruling, 2026-07-09;
				// opendir_79_01 five witnesses): the former same-sentence
				// "name the exact source view and time window" escape is
				// RETIRED — all five leaked numerals were exploration-phase
				// intermediates remembered from views that never entered the
				// final evidence surfaces, and the escape licensed exactly
				// that. The precise directive "exploration-phase numbers not
				// in the final evidence surfaces are prohibited" replaces
				// it. Gate unchanged: soft guidance only, no hard gate on
				// prose numerals (噪声信号只作软引导).
				// EVOLUTION RECORD (PSG-2H §29.10-2 ruling, 2026-07-10;
				// 792 回访 witness: a thread named in prose one character
				// away from the only real spelling): one thread-identity
				// spelling sentence appended — the numeral discipline
				// extends to thread identity tokens, which must be copied
				// verbatim from an evidence surface, never assembled or
				// adjusted. Detection half lives in the answer-side PSG
				// gate (same soft one-round lane + ship-time disclosure).
				// EVOLUTION RECORD (复核 P2-3, 2026-07-12): the derivation
				// license "(a sum, a ratio, a normalization)" narrowed to
				// "(a ratio or a normalization)" + an explicit pointer to NO
				// CROSS-ROW DURATION SUMS — the sum word contradicted the
				// FIN-BIND (c) rule in the same Tier B set.
				Body:      "PROSE NUMBER GROUNDING: every numeric measurement the prose states (a duration in ms, a percentage, an occurrence count) must be locatable somewhere in the report's evidence surfaces — the measured observation records (the runtime observation supplement), the projection tables, the per-file evidence index, the metric snapshot, a quoted evidence block, or a structured fact carried into the final answer. A number that cannot be located there must be removed from the prose: intermediate values read during the investigation that never entered those final evidence surfaces (a latency, an inode token, an occurrence count, a duration remembered from an earlier view) are prohibited in the body, and naming the view or window they came from does not license them — the reader cannot audit a figure no shipped evidence surface carries. Never invent a replacement number, and when you derive a value yourself (a ratio or a normalization), name the published values it was derived from so the figure stays auditable; duration totals of your own are governed by the NO CROSS-ROW DURATION SUMS rule — never add durations from different rows into a new figure. When a sentence names the time window or the thread a number belongs to, that window and thread must be the ones the number's evidence row was published under — bind each figure to its own publishing row; and when comparing across windows, normalize each side by its own window length first, then compare the normalized figures side by side. The same discipline applies to thread identities: a thread named in name-tid form must be copied verbatim from an evidence surface — never assemble, adjust, or recall a thread name or id from memory; when the exact spelling is not on any evidence surface, drop the token or use the published spelling instead.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// PSG-2: object identity assertions — trace-only (§25
				// ruling b assertion half; the audited specimen read a
				// thread NAMED after a fence-acquire operation as the
				// fence object itself and asserted a holder relation with
				// zero holder evidence rows).
				Body:      "OBJECT IDENTITY ASSERTIONS: any claim about who HOLDS, OWNS, or BLOCKS what (a lock, a fence, a buffer, any synchronization object) must be backed by an evidence row that states that relation through its typed fields (holder / blocking / contention notes). A thread's NAME is not evidence about the objects it manipulates: thread names and object names are never interchangeable — a thread named after an operation or an object (e.g. a name containing 'Fence' or 'Lock') does not hold that object by virtue of its name, and reading such a thread name as the object being held is a fabrication. When no evidence row states the holding relation, report the thread identity and the observed wait without asserting a holder or a direction of blocking.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// G13a: primary-cause entity consistency — trace-only
				// (§27.4 G13 + §28.1 ruling, real_trace_campaign_20260705.md,
				// 2026-07-09; huadong_79_01 witness): the answer prose led
				// with a periodic-source chain whose cadence-discounted
				// attribution was ~0.17ms while the ranked evidence's largest
				// attributable cause was an 11.5ms IO row — the report's own
				// rank surfaces and the prose named DIFFERENT primary causes.
				// Complements (never replaces) BACKGROUND AGGREGATE HEADLINE:
				// that clause forbids aggregates in the headline; this one
				// pins the headline's ENTITY to the ranked ordering and adds
				// the explicit-divergence lane. Soft guidance only (entity
				// naming is a noisy signal); a one-shot answer-side advisory
				// covers the same obligation.
				// EVOLUTION RECORD (HEADLINE-ELIM 件1, §29.104.14.1 witness
				// 定谳, 2026-07-16; cust_span_runnable witness: the prose
				// overturned the board's #1 — a 9.586ms deterministic self-work
				// row — for the 8.608ms #2 on the category argument 「并非由
				// 外部阻塞引起」, with zero numeric comparison, anchored by the
				// user's own pre-analysis narrative): three duties hardened —
				// the divergence declaration must QUOTE both published values
				// side by side; category arguments ("own/deterministic work is
				// not a root cause") can never demote the top-ranked row
				// (causality=self_deterministic rows compete on equal terms —
				// system semantics, taught verbatim); and a narrative inside
				// the user's request is a lead, never ranking evidence. All
				// pre-existing obligations kept verbatim (改写非重写).
				Body:      "TRACE PRIMARY-CAUSE ENTITY CONSISTENCY: the entity the prose presents as the primary / main cause must be the SAME entity the ranked root-cause evidence puts first after its own discounts and demotions — compare candidates by their attributable values as published (a periodic signal source counts at its discounted attribution, a merged family row at its combined total, rows with tier=target_self_state are the target's own symptom and never the primary cause, and rows with tier=data_gap mark data blind spots — never causes; a critical_blocking row marked absorbed_by_rank_family=true reports the SAME events as the merged family row its absorbed_into key names — count it inside that family's combined value, never as an additional separate cause). Do not promote a lower-attribution cause to the headline while a larger attributable ranked cause exists, and never demote the top-ranked cause with a category argument: 'this is the thread's own work / deterministic work, not external blocking' does not disqualify a candidate — rows published with causality=self_deterministic compete on equal terms with every other ranked candidate, rank #1 means the largest published attributable value, and the headline conclusion must face that number instead of reclassifying it away. When your synthesis genuinely concludes that a different factor is the primary cause, you MUST still name the top-ranked entity in the primary-cause discussion, state explicitly that your conclusion diverges from the ranked ordering, give the evidence basis for that divergence, and quote the two published values side by side — the top-ranked entity's attributable value and the value of the entity you name instead (e.g. 'ranked #1 <entity A> at <N> ms vs <entity B> at <M> ms'); a divergence without that numeric comparison is not a declared divergence — silently substituting a different entity as the main cause is a regression the reader cannot audit. A pre-analysis or narrative inside the user's request (e.g. '<X> is blocked by <Y>') is an investigation lead, never ranking evidence: it must not override the published value ordering of this report's ranked evidence, and when following that narrative leads you to a different primary cause, the divergence duties above apply in full, grounded in THIS report's published values.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// G13b: lock-wait site quotation — trace-only (§27.4 G13
				// second half, 2026-07-09; opendir_79_01 witness): the answer
				// named a main-thread message-queue call as the lock wait
				// point while the contention span's own text said the wait
				// happened inside an asset resource read ("blocking from
				// boolean android.content.res.AssetManager.getResourceValue
				// (...)") — a wait site inferred from thread-role common
				// sense, not from the span payload. Companion to OBJECT
				// IDENTITY ASSERTIONS (which pins the HOLDER side); this
				// clause pins the WAITER side's code site.
				Body:      "LOCK-WAIT SITE QUOTATION: when the prose names the code site where a blocked thread is waiting (the wait point / blocking point of a lock, monitor, or similar contention), that site may come ONLY from the contention span's own recorded text — quote the span's `blocking from` (or equivalent wait-site) segment verbatim, keeping the method signature and any file:line exactly as the span prints them. Never infer the call site from framework knowledge or from the thread's role (e.g. do not name a message-queue or event-loop entry as the wait point merely because the blocked thread is a main/UI thread): a wait site the span text does not state is a fabrication. When the span carries no wait-site text, report the wait and its holder evidence without naming a code site.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// G16: hop citation-assertion alignment — trace-only (§27.4
				// G16, 2026-07-09; opendir_79_01 witness): three consecutive
				// hops carried citations shifted by one position — the
				// priority-inversion hop pointed at an IO-latency row, so
				// every assertion after the shift was "backed" by the wrong
				// evidence kind. Soft guidance only (evidence-kind matching
				// is a noisy signal).
				Body:      "HOP CITATION-ASSERTION ALIGNMENT: every citation attached to a list item / hop must point at evidence of the SAME kind as that item's own assertion — an item asserting lock contention or priority inversion must not carry an IO-latency row's reference, an item asserting IO blocking must not carry a lock-contention reference, and likewise for every other evidence kind. After assembling the items, re-check the primary citation_ref and every additional citation_refs index against what the referenced evidence actually shows before emitting: an off-by-one drift after inserting, removing, or reordering items misattaches EVERY citation that follows the shift. When no same-kind evidence backs an item, leave that item uncited and state the boundary in its text instead of borrowing an adjacent item's reference.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// FIN-BIND (a): measurement-subject binding — trace-only
				// (CR-1 件④, §29.42.2② 教学先行,
				// real_trace_campaign_20260705.md, 2026-07-12; 41006 first
				// draft witness: a whole-window aggregate published for a
				// different subject was quoted inside another thread's
				// cause paragraph). Complements TRACE PRIMARY-CAUSE ENTITY
				// CONSISTENCY (headline entity) — this clause binds EVERY
				// prose number to its own published subject.
				Body:      "MEASUREMENT-SUBJECT BINDING: every duration / percentage you write in prose belongs to the SAME thread or entity its measured evidence row publishes it for. The danger shape: while explaining why one thread waited, quoting a nearby value that was published for a DIFFERENT thread, for a merged family, or for the whole window, as if it were that thread's own number. Before writing a number next to an entity name, re-check which subject the measured row publishes it under; when the value belongs to another subject or to a merged family, either name that owner explicitly beside the number or leave the number out.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseScalarUngrounded,
				},
			},
			{
				// FIN-BIND (b): root-cause board order — trace-only (CR-1
				// 件④, §29.42.2②/§29.42.3, 2026-07-12; 41006 witness: prose
				// stated its own Rank1..5 that violated the ranked ordering
				// AND its own declared sort key). Complements TRACE
				// PRIMARY-CAUSE ENTITY CONSISTENCY (which pins the headline
				// ENTITY): this clause pins the stated ORDER of the whole
				// cause list, with the same explicit-divergence lane
				// (conscious-flip: disclosed deviation always allowed).
				Body:      "ROOT-CAUSE BOARD ORDER: when the prompt carries a runtime-trace root-cause board section, state the ranked causes in THAT order — the board is the single authoritative ordering, and a prose cause list that silently uses a different order (or declares one sort key and lists values that do not follow it) contradicts the report's own evidence pages. When your combined judgment genuinely ranks the causes differently, you may present your own order — but then say explicitly that it differs from the measured ordering and what your judgment is based on; never reorder silently, and never present two different \"number one\" causes in one answer.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseLexiconBoardInconsistent,
				},
			},
			{
				// FIN-BIND (c): cross-row duration sums — trace-only (CR-1
				// 件④, §29.42 案12, 2026-07-12; 41006 witness: "约45ms" was
				// a model-side aggregate no published caliber reproduced).
				// Complements WINDOW-STATS CORE NUMBERS (which allows a
				// self-derived ratio with its inputs named): wall-clock
				// DURATIONS never sum across rows.
				Body:      "NO CROSS-ROW DURATION SUMS: never add durations from different measured rows or different threads into a new total of your own (wall-clock intervals overlap and double-count — a self-made aggregate fabricates time). This includes a prose total made from the top N root-cause rank seats: rank values are comparable attribution/head-room values, not additive parts of one duration. Quote each row's own published value, or a total the evidence itself publishes (a merged family row's combined value is such a total, and its typed fold caliber licenses only the members inside that one row). An approximate figure introduced with wording like 'about N ms' must still be one published value, not your own sum. Self-derived RATIOS stay allowed under the core-numbers rule (name the published values they came from); duration totals do not.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseScalarUngrounded,
				},
			},
			{
				// FIN-BIND (d): channel words per row — trace-only (CR-1
				// 件④, §29.42 案23, 2026-07-12; witness: prose narrated an
				// on-chain block-IO row as "background IO noise", reversing
				// its own published channel). Complements BACKGROUND
				// AGGREGATE HEADLINE (which forbids promoting background
				// rows): this is the reverse direction — never DEMOTE an
				// on-chain row to background wording.
				Body:      "CHANNEL WORDS PER ROW: describe each measured cause row with the channel its evidence publishes (chain_relevance / causality fields). A row published as on the wakeup/dependency chain is part of the causal chain — never narrate it as background noise or incidental load; a row published as background or adjacent is context — never promote it into the direct chain. When prose and the row's published channel disagree, the published channel wins; if you believe the channel assignment is wrong, say so explicitly as your own assessment instead of silently rewording the row's role.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseLexiconBoardInconsistent,
				},
			},
			{
				// FIN-BIND (e): inversion×lock coexistence — trace-only
				// (CR-1 件④ + §29.40.1 user ruling 2026-07-11: 并存披露,
				// 非降级替换). Fact statements carry BOTH facts; the repair
				// space they imply belongs to the optimization/next-step
				// surface.
				Body:      "PRIORITY-INVERSION AND LOCK-HOLD COEXISTENCE: when the evidence publishes BOTH a priority-inversion relation and a lock-holding fact for the same wait, state both facts — they coexist; neither cancels the other. Do not use the lock fact to deny the inversion wording, and do not hide the lock fact to keep the inversion story clean. The two facts define different repair spaces (a lock-held inversion can be addressed through priority inheritance or through decoupling the lock; a lock-free inversion only through priority/scheduling adjustment) — put that repair-direction reasoning in the optimization / next-step surface, and keep the fact statements themselves plain.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// FIN-BIND (f): IO-latency role words — trace-only (CR-3
				// 件⑦a, §29.50 F-9 分诊, 2026-07-12; CAL-1 冷读 F-9 witness:
				// prose swapped a thread's own sleep segment with the block
				// request's device-side latency — two near-equal values, two
				// different roles). Complements MEASUREMENT-SUBJECT BINDING
				// (which binds a number to its thread): this clause binds a
				// number to its ROLE within one IO story.
				Body:      "IO-LATENCY ROLE WORDS: a published IO-latency value is the REQUEST's own latency (from issue to completion — the device/completion side), while the waiting thread's blocked or sleeping segment over the same period is a DIFFERENT measurement with its own value; the two are usually close but never interchangeable. When prose names who initiated an IO and who completed it, keep each value with its own role — the requester's blocked time comes from the requester's row, the request latency from the IO row. Never restate one side's value as the other's, and when only one side is published, say which side it is instead of implying both.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseScalarUngrounded,
				},
			},
			{
				// FIN-BIND (g): state-duration caliber separation — trace-only
				// (CR-3 件⑦b = P6 教学半场, §29.42 P6 + §29.50 遗留 B1,
				// 2026-07-12; witness: prose listed activity-slice values
				// beside full-window totals for one thread and the set summed
				// past the window — a physically impossible partition that
				// flipped the starvation attribution). The mechanical half is
				// the deterministic conservation cross-check; this clause is
				// the drafting-side discipline.
				Body:      "STATE-DURATION CALIBER SEPARATION: one thread's scheduler states partition wall clock — for any single thread, running + runnable + sleep + uninterruptible time together can never exceed the analysis window, and no single state can exceed the window. Values measured over an activity slice (covering only part of the window) and full-window totals are DIFFERENT calibers: never list them side by side as if directly comparable, and never mix calibers inside one per-thread breakdown. The separation has a second direction: a value published for ONE segment, occurrence, or per-CPU slice (e.g. a single wait segment's duration inside an observation summary) is never the thread's full-window total for that state — when prose states how long the thread spent in a state over the window, take the number from the published full-window per-state account (the target_window_states partition), not from a segment row and not from your own reconstruction of segments. A perf-triage time_semantics value spanning the whole attached artifact is a third, artifact-global caliber: it proves timestamp units and attachment extent only, never the selected query window or any target-thread state duration, even when its endpoints are numerically close to the selected window. Before publishing a per-thread state breakdown, sanity-check that the durations can fit the window together; when they cannot, re-read which rows the values really came from instead of publishing an impossible set.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseScalarUngrounded,
				},
			},
			{
				// EVAL-B1-R12 (2026-07-30): exact target wait occurrences are
				// engine-paired evidence. The prompt-level roster explicitly
				// distinguishes complete from budget-truncated membership.
				Body:      "TARGET WAIT OCCURRENCE AUTHORITY: when a target_window_wait_occurrences record carries target_wait_occurrence_prompt=status=complete, treat its target_wait_occurrence notes as the exact engine-paired roster. Copy count, start/end, duration, state, iowait, and caller from those notes; do not rebuild intervals from adjacent sched_switch/sched_wakeup/event_search rows and do not merge or discard a listed occurrence. Cross-check target_wait_occurrence_prompt_sum_ms against the target's published wait total before writing. When prompt status is incomplete, disclose the emitted/total boundary and never claim that the visible notes enumerate all occurrences.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
				OnViolation: []types.ViolationKind{
					types.ViolProseScalarUngrounded,
				},
			},
			{
				// EVAL-B10-AA1 (2026-07-31): target_window_states is a
				// thread-local wall-clock partition, not a CPU-wide load
				// account. Keep the wording boundary soft and typed.
				Body:      "TARGET THREAD VERSUS CPU SCOPE: `target_window_states` partitions ONE target thread's wall-clock states. Its running share describes only how much of the window that target ran, and its runnable share bounds only that target's scheduler queueing. Neither value alone is CPU utilization, CPU idle capacity, or system saturation. State a CPU-wide utilization/saturation conclusion only when a separate typed per-CPU, core-class, process-domain, or system occupancy/idle/pressure account supports it; never rename target-thread running percentage as CPU utilization.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// FIN-BIND (h): trace-first disclosure on an empty trace
				// result — trace-only (CR-3 件⑦c, §29.47.7 立案 2026-07-12;
				// witness: a trace-led question whose analysis produced zero
				// root-cause findings shipped a source-code mechanism
				// narrative in their place). 判据收窄 per the ruling: mixed
				// analysis where trace observations anchor the answer stays
				// normal — the rule speaks ONLY to the empty-result shape.
				Body:      "NO SILENT SOURCE FALLBACK ON AN EMPTY TRACE RESULT: when a runtime trace is attached and the question asks about that trace, but the trace analysis produced ZERO root-cause findings, disclose that FIRST — state plainly that this trace analysis produced no root-cause findings and why (no matching observations in the window, an analysis error, or an exhausted budget), and only then offer whatever else genuinely helps. An empty trace result is not a source-code question: never let source-code mechanism narration silently take the place of the missing trace findings as the answer's principal claims. Mixed analysis stays normal — when trace observations are present and anchor the answer, explaining the implicated source code is welcome; this rule applies only when the principal claims would otherwise rest on source citations alone because the trace lane found nothing.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// ANSWERFACE-1 件1 (§29.140 G4 教学半场, 2026-07-19): census
				// consumption discipline — soft guidance only (the census is a
				// typed inventory the model consumes; dropping a record by
				// impression is a value error the drafting side must not
				// commit; no gate reads this).
				Body:      "BLOCKED-REASON CENSUS CONSUMPTION: when the evidence publishes a blocked_reason census for a thread (a `blocked_reason_census` note with a total count and per-caller ×count(Σms) shares), that census is the authoritative record inventory: the window holds exactly that many kernel blocked_reason records for that thread, with exactly those caller symbols and Σ shares. A caller symbol identifies the kernel call site recorded for the blocked_reason event; it is NOT by itself the waited resource/object, its owner, a device, a completed GPU/IO operation, a subsystem mechanism, or a direct fix direction. Its function-name morphology is only a code-location/search clue unless a separate typed field or dependency edge confirms the interpretation. Name a wait object, owner, mechanism, subsystem, or corresponding fix only when that separate typed evidence exists. Report the census's own numbers — never re-derive the count or the per-record fields from your recollection of raw rows read earlier, and never drop a record because it no longer fits the narrative (reporting one fewer record 'without a recorded reason' while the census names the full count contradicts the published inventory). The census count/Σ and a scheduler-state interval count/total are different typed measures: do not call their difference rounding or an allowed precision error, and do not pair every caller record to every D/IO interval unless a typed interval join authorizes that pairing. When your reading of individual raw rows disagrees with the census count or Σ, the census wins and the raw rows deserve a re-read — a silent drop is a wrong answer, not a simplification. The census record's selected_window note names the caliber of that count: read the total as a count over exactly the window the note names (when no analysis window was established, that window is the whole trace), and never re-dress it as an in-window count for a window the note does not name.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// ANSWERFACE-1 件6 (§29.140 叙事消费教学, 2026-07-19): the
				// report's deterministic word faces are answer material, not
				// appendix decoration — zero hard gates, prompt guidance only
				// (the audit observed six major answers consuming none of
				// them). Tokens are the LLM-visible note keys published by
				// the trace evidence — no internal names.
				//
				// FREQDIR-1 件3 (§29.149 修向③, 2026-07-19; witness 95946: the
				// answer's 修复方向 list silently dropped the published #1
				// direction and crowned another as 「提升空间最大」): the (a)
				// clause gains the missing completeness duty (every published
				// direction enters the enumeration — the semantic-span
				// never-omit precedent applied to the direction axis) PAIRED
				// in the same breath with the caliber-participation rule (a
				// discounted/折算 seat enters WITH its caliber and never joins
				// a cross-direction sum — the pair travels together so the
				// duty can never mint a mixed-caliber total), and the ranking
				// stays with the published board order (never self-crowned by
				// a cross-caliber comparison — in either direction). Still
				// zero hard gates: prompt guidance only (§29.42.4/§29.104.13
				// answer-face ownership; the appendix arm merely discloses).
				Body:      "TYPED WORD-FACE CONSUMPTION: the deterministic evidence publishes several typed word faces the narrative should actively use. (a) `fix_direction` groups ranked causes by repair direction — when the user asks what to fix, answer along those directions. Within one direction, the largest seat's published value is that direction's recoverable ceiling. Across DIFFERENT directions, do not sum seats into a guaranteed combined gain unless an exact typed additive or joint-counterfactual carrier authorizes it. This non-additivity means the joint benefit is UNPROVEN; it does NOT prove that the seats overlap, that one depends on another, or that fixing one makes another disappear. When the user asks for repair directions or improvement head-room, the direction enumeration MUST cover EVERY direction value published on ON-CHAIN seated causes (adjacent-channel rows stay conditional upper bounds and never set a direction's maximum) — a direction is never omitted because its seat's value rides a different caliber; a seat published on the discounted (折算) caliber enters the enumeration as its OWN direction entry stated together with its published caliber words, and its value never joins any wall-clock total or any guaranteed cross-direction total. State such an entry beside the other directions — never summed into them, never silently dropped, and never ranked by your own cross-caliber comparison: let the published board order speak for which direction leads. (b) Only `cross_direction_overlaps` authorizes an overlap statement: it marks two seats sharing the same physical time, so fixing either side already recovers the PUBLISHED shared part; never present that shared part as stackable and never extrapolate beyond its typed value. Pairs whose overlap falls below a significance floor (relative to the smaller seat's published value) publish no overlap sentence and leave only a `cross_direction_overlap_undisclosed` audit token: read that as 'no significant overlap', never as proof the two seats are disjoint, and never invent an overlap value for such a pair. (c) merged family rows carry member rosters (`member_roster` / `member_wall_ms` / `member_count`): the leading members are concrete business leads — name the top member spans when explaining a merged family instead of quoting only the family total. (d) chain-credential markers state HOW a row's on-chain status was proven (`chain_credential_segments` = per-segment proof; `chain_credential_envelope_level` = envelope-level only; `chain_credential_lane_demoted` = no on-chain credential, demoted lane): paraphrase attribution at the strength the credential supports — a demoted or adjacent row is a conditional upper bound ('at most', 'if causally linked'), never a proven cause. (e) `business_span_mention` rows are advisory business-side leads, never ranked causes: each carries a verbatim span name with a typed trio (occurrence count, max single duration, total). Root causes have TWO dimensions — the rule-priced eliminable board AND raw time occupancy guiding NEW fix directions — and these rows are the dedicated raw-occupancy carriers, so when business_span_mention rows are present the final conclusion MUST mention the admitted families (at least the largest by total, with its trio) as business-lens leads beside the ranked causes. Read the trio as levers — many occurrences with a small max single suggest reducing the call count or reshaping the business flow; a long max single suggests shortening one run's duration — never as a substitute for the ranked causes, and never promote a mention row into the primary-cause discussion.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// RUN2FIX-C 件1 (§29.174 处置④, 2026-07-20; witness runnable_2
				// :20/:28/:84 vs :115): the model derived the user-named frame's
				// vsync→doFrame 119.320ms delay six times during the
				// investigation and the final answer never stated it — only a
				// neighboring 22.214ms wakeup-edge latency shipped. Mechanical
				// map: the boundary pair has NO typed carrier on the finalize
				// evidence surfaces today (event rows mint no observations;
				// frame_target_resolution carries the selected span window, not
				// the signal→start delta), so this is the teaching-duty form
				// (fork (b)) — soft guidance + fact fence, zero hard gates
				// (§29.42.4/§29.104.13). The explore-skill companion
				// USER-NAMED LATENCY ANCHOR CARRY makes the duty satisfiable
				// (anchors must reach an evidence surface first).
				Body:      "USER-NAMED END-TO-END QUANTITY COVERAGE: when the question names a concrete frame, span, or interval and asks why it was late, slow, or blocked, the answer's lead must state that object's OWN end-to-end quantity — how long the named object was actually delayed or blocked (e.g. from the same frame id's pacing-signal timestamp to its processing-start timestamp) — whenever the investigation established it. Do not let a nearby smaller number silently stand in for it: one wakeup edge's latency or one wait segment's duration measures a different thing, so when you state such a number, say what it measures — it never replaces the named object's own delay. Stating the elapsed time between two published boundary timestamps of the SAME named object is a boundary difference, not a cross-row duration sum: name BOTH boundary timestamps beside the derived value (positive shape: 「信号时刻 t1 → 开始处理时刻 t2,共推迟 Δms」; negative shape: quoting only one wakeup edge's latency as if it answered the whole delay), and keep every stated anchor locatable on the report's evidence surfaces (PROSE NUMBER GROUNDING governs the numbers themselves). When the investigation computed the quantity but its boundary anchors never reached any evidence surface, say so plainly — name the boundary that is missing — instead of silently dropping the question's core quantity; never invent, estimate, or substitute a replacement number.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// RUN2FIX-C 件2 (§29.174 处置④, 2026-07-20; witness runnable_2
				// :115/:119/:120/:121): tier=primary/secondary/tertiary,
				// 「runnable_wait dominant_state=runnable」, 「d_state_or_io_wait
				// 有效归因」 and 「根因排名on-chain」 all shipped inside Chinese
				// customer prose. Coverage map: STYLE-1 (answer_style_words.go)
				// is a filler-phrase wordlist and never spoke to field
				// spellings; the wire→display-word bridge (TRACE VALUE WORDS)
				// lives on the explore skill and covers the caliber-word
				// families only; meanwhile the board feed prints `tier=` /
				// `channel=` k=v rows verbatim and the Fact lane teaches
				// verbatim copying — so the drafting face had NO directive
				// separating field spellings from reader words. Soft guidance
				// only; the example display words are hand literals per the
				// Fact-lane precedent (the published faces they mirror are
				// pinned by the projection tests).
				Body:      "READER WORDS OVER FIELD SPELLINGS: the measured evidence addresses its facts with key=value field spellings and underscore enum tokens — `tier=primary`, `dominant_state=runnable`, `chain_relevance=on_chain`, cause-type tokens such as `d_state_or_io_wait` / `runnable_wait`. Those spellings are data field names, not reader words: user-facing prose states the same fact with the report's own published display words — the words the report's board, tree, and legend already print (in a Chinese answer e.g. 链上/邻近 for the channel, published cause words such as 优先级反转候选 / 调度压力候选 / D状态/IO候选, the seat ordinal 根因排序#N, the 修向 word pair) — or with plain reader language. A field name may appear only as a quoted key beside its cited evidence row, never as the sentence's own vocabulary. The fact fence is unchanged: values, published caliber words (全额/折算/下界 and their siblings), state words the report itself prints (running / runnable / sleep / D-state), and [E#] references stay verbatim — only the field-name/enum spelling wrapped around them is replaced by the published word. Negative shape (do not ship): 「…d_state_or_io_wait有效归因10.433ms(tier=tertiary)」. Positive shape: 「…的D状态/IO候选,有效归因10.433ms,根因排序#3,修向 IO/内核/依赖 (IO / kernel / dependency)」.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// RUN2FIX-C 件3 (§29.174 处置④, 2026-07-20; witness runnable_2
				// :115-:123): the shipped answer opened with one ~1100-character
				// wall-of-text paragraph, then re-read seven board seats value
				// by value, while the core quantity was absent and the top
				// eliminable causes sat 40+ lines below. Content-organization
				// teaching only — the deterministic faces (overview/tree/table)
				// and every consistency rule (TRACE PRIMARY-CAUSE ENTITY
				// CONSISTENCY / ROOT-CAUSE BOARD ORDER) are untouched.
				Body:      "TRACE ANSWER SKELETON: organize a trace root-cause answer in four moves. ① Open with ONE quantified conclusion: the user-named object and its end-to-end quantity (per USER-NAMED END-TO-END QUANTITY COVERAGE — when the quantity was never established, its honest gap statement stands in that slot, never an invented number), followed by the strongest relationship permitted by typed causal authority. When frame/deadline causality is proven, state the causal chain; when `causal_conclusion=unproven` or `frame_evidence_status=absent`, state the selected-window bottleneck / strongest chain candidate and the missing frame-causality link instead. A `root_cause_rank` or projection title is a ranking inside the observed chain, not permission to call it the proven dropped-frame cause. Keep the opening to one or two short sentences, nothing in front of them. ② Then split the target's own account: which waiting is designed-in cooperation (e.g. sleeping for a downstream reply) and which part is the real bottleneck. ③ Then the top eliminable causes with their repair directions — a few short lines (about five at most), each with its value, its published caliber word, and its [E#]; concrete action advice follows these directions and names these entities, never generic template steps. ④ Point everything else at the report's own deterministic faces (the overview, the causal tree, the detail table): prose does NOT re-read every seat's value account row by row — the board already renders them, and a per-seat prose re-listing duplicates the authority face without adding judgment. Keep paragraphs short: split a long conclusion into short sentences or bullets — a single wall-of-text paragraph buries the conclusion the user came for.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				// RUN2FIX-C 件4 (§29.174 处置④ F4, 2026-07-20; witness
				// runnable_2 :115/:118/:122 vs :67/:509): prose stated
				// 「wakeup往来共计62次(31次+34次)」 three times — 31+34=65 (the
				// model's own investigation computed 65 correctly), while 62
				// was borrowed from the state_churn face's 62 次切换 (a
				// different measurement). Teaching arm only (F4①): the F4②
				// contract-soft-check variant was evaluated and NOT taken —
				// the witness's real surface forms carry free prose inside the
				// decomposition parens (thread names, direction words), so
				// extracting count pairs is a noisy NL parse (the CR-4
				// 2026-07-12 ruling retired exactly that from system verdict
				// surfaces; the bare A+B=C equation form is already covered by
				// the pure-arithmetic juxtaposition arm). 噪声信号只作软引导.
				Body:      "TOTALS MATCH THEIR PARTS: when prose states a total and lists its parts beside it (「共N次(A次+B次)」 and every equivalent phrasing), the total must equal the exact sum of the listed parts — do the addition before emitting, and when the published parts are what you have, the total IS their sum, never a nearby number remembered from a different measurement. Wakeup traffic totals come from the published per-direction wakeup counts (the per-pair census counts): a two-way total is their exact sum. A state-switch count (a thread's N次切换 from its state-churn statistics) counts scheduler state transitions, not wakeup exchanges — never borrow it as a wakeup total. When the listed parts cover only part of the whole, say so (「其中」 / partial wording) instead of presenting a partial list as the full decomposition. Negative shape (do not ship): 「wakeup往来共62次(唤醒31次+被唤醒34次)」 — 31+34=65, and 62 is a different measurement's number. Positive shape: 「wakeup往来共65次(唤醒31次+被唤醒34次)」.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      "WAKEUP CENSUS DIRECTION AND STATE: consume typed wakeup_edge_census rows as waker -> wakee counts. The sleep_exit/d_exit/other_exit split is the state the WAKEE LEFT when the wakeup occurred — pre-wakeup state — not a state entered after the wakeup. A wakeup makes the target runnable; later switch-in, execution, preemption, and switch-out are separate scheduler facts. Never turn sleep_exit=N into “after each wake it immediately slept”; an all/every post-wakeup claim requires a separately complete paired transition census.",
				AppliesTo: AppliesToFilter{RequiresTrace: true},
			},
			{
				Body:      mechanicalProducerChainSeparationDirective,
				AppliesTo: AppliesToFilter{RequiresMechanism: true},
			},
			{
				Body:      independentMechanismContrastDirective,
				AppliesTo: AppliesToFilter{RequiresMechanism: true},
			},
			{
				Body:      runtimeRuleInstantiationDirective,
				AppliesTo: AppliesToFilter{RequiresLog: true, RequiresMechanism: true},
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
			"Group frames under the explicit error occurrence they belong to. Emit errors[] with one entry per logical error that the artifact actually reports (for example one panic/fatal header, or each explicitly printed exception in a multi-exception traceback). In a Go goroutine dump, JVM thread dump, or native thread dump, a goroutine/thread block that only shows a concurrent stack snapshot and has no error/exception header of its own is NOT a sibling error: preserve it as an observations[] thread_snapshot with severity=info, diagnostic=false, the thread/goroutine identity, and the exact observed frame excerpt. A thread_snapshot proves only what that thread was executing when the dump was captured; it does not prove that the thread crashed, emitted the error, touched the same resource, or caused the failure. Only when separate goroutines/threads each carry their own explicit panic/error/exception header do they become peer errors. For each error set type (exception class / panic type), message (optional text copied VERBATIM from the attached log; never a synthesized explanation), and frames[] (the stack for THIS explicit error occurrence).",
			"Chain causal errors via the cause pointer only when the artifact prints an explicit exception-chain separator such as Java/Rust 'Caused by:', Python 'The above exception was the direct cause of the following exception:', or Python 'During handling of the above exception, another exception occurred:'. For every cause edge emit the sibling cause_relation={authority:'explicit_artifact_marker', marker:'<one exact separator line copied from the artifact>'}. Similar error text, adjacent timestamps, shared IDs/tags, and neighboring stacks never establish this edge; emit those explicit errors as peer errors[] and leave the cross-error relation unproven. Keep the chain shallow — practical depth 3 or so; depth is capped at 5.",
			"Set meta.lang to the dominant language (go/java/cpp/python/node/rust/ruby/csharp/kotlin/arkts/cangjie/unknown/other). ArkTS is HarmonyOS UI code in .ets / .ts files with V8-style stack frames; Cangjie is HarmonyOS native code in .cj files with JVM-like frames `at demo.cart.Cart.method(Cart.cj:42)`; Kotlin is JVM code in .kt files with frames `at com.example.Foo$bar(Foo.kt:42)`. Two tag-formatted log families share one parser path: hilog (HarmonyOS) lines look like `01-26 11:01:06.870 1051 1051 W 00201/test: message`; Android logcat lines look like `04-15 14:32:18.421  5821  5821 E JsApp: message` — structurally identical: <timestamp> <pid> <tid> <level> <tag>: <body>. Extract the body portion while keeping the full line in the frame's raw field. Set meta.signals from the canonical enum (panic/crash/oom/timeout/permission/db/network/validation/logic/performance/other) by matching the observed symptoms. The 'performance' signal covers operational-but-slow patterns the log itself describes: 'slow API call took 5s', 'frame skipped' / 'Choreographer dropped N frames', 'GC pause 800ms', 'blocked on lock 2s' — distinct from 'timeout' (no operation was cancelled, just slow) and from performance-trace jank signals. Multiple signals are OK when the log describes compound failures. Summary is an optional one-line synopsis. PERFORMANCE traces (HiTrace/atrace/systrace/perfetto) are handled by a SEPARATE performance-trace parser via emit_perf_trace — do NOT attempt to parse trace events here; if the attached text contains `tracing_mark_write: B|pid|tag` events, return an empty errors list with signals=[] so the trace parser can handle it.",
			"Operational observations that do NOT form an exception tree but DO describe a process fact go into observations[] instead of unknown_chunks. Examples by shape, not by keyword: a validator/reviewer rejected an answer, an attempt was retried or forced to rewrite, the answer topic diverged from the requested topic, a file/line/source mapping drifted, an attachment could not be decoded/resolved, a log level/event appears at a specific attached-log line, a named event is absent from a bounded log window, or the system changed mode/state. For each observation choose kind from the enum, set severity (info/warning/failure when clear), set diagnostic=true only when the observation describes a failure/regression/mismatch/current-risk target rather than neutral context, include a concise summary plus a short evidence excerpt copied VERBATIM from the attached log, and set line_start/line_end when the observation is anchored to the attached log's `N│` gutter. Omit evidence when no short exact excerpt exists. Evidence is the observed artifact text; summary is only a triager interpretation and MUST stay within what those exact lines establish. Never infer the meaning or producer of a numeric prefix, progress ordinal, attempt counter, status decoration, localized payload, or composed message from visual adjacency or equal numbers alone; record the literal fields separately and leave their code mechanism unresolved for current-source investigation. These line fields are artifact-local log anchors, not repo citations. A stack caller frame proves call order/context only; do NOT emit an observation summary saying that caller supplied a bad value, constructed upstream data, or owns the failing variable unless the log line literally states that fact.",
			"Log chunks that do NOT structure into an error or observation (build noise, log-level prefixes, unrelated debug output, truncation markers) go into unknown_chunks. Do not punt everything there — only genuinely unparseable pieces. Each chunk capped at 500 chars, at most 8 chunks total.",
			"Frames with uncertain line numbers or uncertain file paths should be emitted with line=0 or file='' and confidence < 0.5 — such frames stay in the bundle but are NOT added to the repo file list. Zero information loss on partials.",
		},
		ToolSuggestions: []string{
			"read_file", // blob pagination only
			"emit_log_triage",
		},
		OutputFormat: types.LogTriageJSONShapeFirstTeaching + `

You have ONE required emit tool: emit_log_triage. The current tool schema is authoritative for whether an attachment-pagination read tool is available; use only listed tools. You do NOT have grep / repo_map / list_files — path resolution is handled automatically.

Schema in one glance:
- meta.lang        (required) — the dominant runtime language
- meta.signals[]   (required, may be empty) — what-went-wrong enum values
- meta.summary     (optional) — one-line synopsis, ≤ 200 chars
- errors[]         (required, may be empty) — array of { type, message?, frames[], cause?, cause_relation? }
- errors[].message — optional VERBATIM substring of the attached log; omit it when no explicit message exists; put bounded interpretation in observations[].summary
- errors[].frames[] — { lang?, file?, line?, func?, pkg?, raw (required), confidence (required) }
- errors[].cause   — recursive error (same shape); only for an explicit artifact-marked linear causal chain
- errors[].cause_relation — required beside every cause: { authority: "explicit_artifact_marker", marker: <VERBATIM explicit separator line> }; omit both fields for peer/adjacent errors
- observations[]   (optional, ≤8) — non-stack operational facts and concurrent thread snapshots:
  { kind, severity?, subject?, summary, evidence? (VERBATIM attached-log excerpt), line_start?, line_end?, diagnostic, confidence }
  kind enum: runtime_event / thread_snapshot / contract_violation / retry_cycle / topic_mismatch / line_mapping / artifact_gap / state_change / performance_symptom / other
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
			"do NOT turn every goroutine/thread snapshot into an error — without its own explicit panic/error/exception header it is concurrent runtime context and belongs in observations[]; errors[].message must be copied verbatim from the attached log",
			"do NOT upgrade a composed log line's visual layout into mechanism facts — progress ordinals, retry counters, status payloads, and neighboring lines keep separate unknown producers until current source establishes their data flow",
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
		Goal: "Read the attached HiTrace / atrace / systrace / perfetto text excerpt and emit a structured PerfBundle (frames + janks + stalls + startup + observations) via emit_perf_trace exactly once. Preserve measured frame/span durations and keep frame-deadline, refresh-rate, jank, and causal classifications within the authority actually present in the artifact.",
		Workflow: []string{
			"Read the attached trace from the 'Attached Performance Trace' section. The same channel carries HiTrace (HarmonyOS hdc), atrace / systrace (Android adb), and perfetto text dumps — meta.source records which one you observed. Multi-file attachments embed `# codrax-source: <path>` headers between bodies; treat each segment as an independent capture (different process / time window / device) when correlating janks. If the current tool schema includes an attachment-pagination read tool for an oversized blob, use line_offset/limit to paginate; otherwise the visible inline section is the full available body for this dispatch. line_offset is line-based, not byte-based; emit_perf_trace has no byte_start/byte_end fields.",
			"If an attached segment is `.tracebundle.json` or the prompt says tracebundle metadata was detected, treat it as a query manifest/provenance artifact rather than the raw trace body. Do not page through the JSON looking for sched_switch/tracing_mark rows, and do not conclude the systrace body is missing merely because the manifest is JSON. Emit observations for the referenced systrace/perftrace paths, provider readiness, and coverage/caveats visible in the manifest; later trace_query calls consume the tracebundle or its sibling systrace for scheduler/root-cause/perf_sample analysis.",
			"Identify the source: `# ftrace` header or `TASK-PID CPU# TIMESTAMP FUNCTION` header = systrace / atrace / hitrace (all ftrace-compatible). A textual `perfetto` banner = perfetto text dump. Set meta.source accordingly. Raw ftrace timestamps are seconds end-to-end (928.081774 = 928s + 0.081774s; six fractional digits are microsecond precision). Compute meta.duration_ms = (last_timestamp − first_timestamp) × 1000.",
			"When the attached trace is HarmonyOS / OpenHarmony / HiTrace / bytrace (including user wording such as 鸿蒙 / 东湖 / OHOS), user-space priority semantics are: larger numeric priority means higher priority; 1-40 are CFS priorities, 41-159 are RT priorities, and values >159 are system_or_kernel/raw scheduler tokens. Concrete mapping examples: prio=20 is CFS; prio=41, prio=140, and prio=159 are RT; prio=160 and prio=301 are raw system/kernel values. For any concrete prio=N in the trace, recompute the class from the numeric range before writing it. Raw system/kernel values remain visible but must not be called RT/high-priority evidence or compared numerically for priority inversion. If source is Android / atrace / generic ftrace, keep priority as raw scheduler priority unless the trace producer documents its mapping.",
			"Keep priority-inversion semantics evidence-calibrated and broad: a lower_priority_waker/lower_priority_dependency relation alone is only a structural low-priority dependency candidate and does not prove measured inversion or blocking. A measured candidate requires a positive effective_impact_ms on a typed root_cause priority_inversion_candidate/priority_inversion_runnable_wait row; confirmed holder/waiter authority is separate. Do not reduce the mechanism to same-CPU preemption: typed impact may count the lower-priority on-chain dependency's runnable time in full plus a cross-CPU weak-core/compute-supply running deficit. A target self runnable row is the target's own ready-to-run wait, not the dependency's runnable state. Read priority_inversion_candidate and priority_inversion_runnable_wait as ONE priority-inversion family measured on two channels — candidate marks the on-chain gated composite seat, runnable_wait marks the same-CPU runnable-overlap occurrence row — never as two independent competing causes; a rank row whose tier is absorbed holds no ranking seat: its account already counts inside the family row its absorbed_into key names.",
			"Keep scheduler residency and cross-subject causality separate. A thread in S, D, or io_wait is not occupying a CPU during that interval; its wait can be the cause of that thread's own delay, but overlap with another thread's slow frame does not prove that it consumed the CPU or caused that frame. Attribute one subject's wait to another subject's frame only when a typed wakeup/IPC/lock/flow/dependency edge connects them and their published intervals are temporally compatible. Without that connector, emit the measured wait and frame as separate observations and mark their relationship unproven; never manufacture a dependency from same-CPU labels, temporal overlap, or adjacency alone.",
			"Partition scheduler transition intervals before naming them. For one target, sched_switch prev_state=S|D at t_sleep marks entry into sleep/blocking, sched_wakeup or sched_waking at t_wake marks exit from that wait into runnable, and a later sched_switch next_pid=target at t_run marks switch-in to running. t_sleep→t_wake is sleep/blocking until wake; t_wake→t_run is runnable scheduling delay; t_sleep→t_run is total non-running. Never call the pre-wakeup sleep interval or total non-running interval wakeup latency, and never call t_run the wakeup timestamp. Keep these model-extracted observations advisory until deterministic trace_query publishes the typed state account.",
			"Preserve Binder transaction direction from the emitting row and typed call_semantics. `call_semantics=reply` (or native `reply=1`) means the row's source thread is sending a reply to its destination; it is not that source issuing a synchronous request or waiting for the destination to return. Only `call_semantics=sync_request` with the typed blocking candidate supports a synchronous blocking-request description. Keep destination, receiver, reply, and any later sched_wakeup edge as separate observed relations unless the typed IPC graph connects them.",
			"Walk every `tracing_mark_write: B|<pid>|<tag>` begin and pair it with the next matching unnamed `E|<pid>` or bare `E` on the same ftrace thread stack; never search for `E|<pid>|<tag>` because B/E end rows do not repeat the tag. Pair async `S|<pid>|<tag>|<cookie>` / `F|<pid>|<tag>|<cookie>` rows by marker pid + tag + cookie. The delta is the span duration. UI-thread span tags typically prefixed with `H:` on HarmonyOS (`H:RenderService:DoFrame`, `H:Layout:measure`, `H:Drawing`, `H:DataLoader:fetchSync`) or use Android conventions (`Choreographer#doFrame`, `performTraversals`, `RenderThread`).",
			"Frame verdict authority: the current emit_perf_trace payload has no validator-owned refresh-rate/frame-deadline carrier, so it cannot prove a jank, dropped-frame, or non-dropped-frame verdict. Always emit measured frame/span durations; keep `janky` false/omitted and omit janks[]. If the artifact explicitly states a refresh rate, frame budget, or deadline, preserve that literal as an observation for later typed verification, but still leave the verdict unproven here. Put relevant measured phases in frames[] or observations[] and keep trigger/reason/tags as model-extracted navigation candidates, not causal proof, until deterministic trace_query rows verify them. Keep the measured slow interval separate from those candidates.",
			"For main-thread blocking calls longer than 100 ms, emit a PerfStall: kind = io / lock / sync-rpc / native-call (pick the most specific); symbol = the trace tag's identifier portion; file / line = when the tag's format embeds them (e.g. `fetchSync at DataLoader.ets:42`).",
			"If the trace covers a process cold-start (observable by `ActivityTaskManager`, `AppInit`, `AbilityManagerService`, `WindowStage.loadContent`), emit a PerfStartup: mode=cold (or warm/hot if evidence suggests), app_launch_ms / ability_init_ms / first_frame_ms when measurable. Over 1.2 s app_launch_ms = slow cold start.",
			"For trace facts that answer the user's question but are not jank/stall/startup, emit observations[]: use subject for the span/event/check name, summary only for a fact directly established by the cited trace lines, line_start/line_end for artifact-local trace gutter lines, duration_ms for measured durations, and tags[] for trace tags such as H:GC:Collect. Treat these rows as model-extracted navigation facts: use them to locate the relevant trace region, while deterministic trace_query results remain authoritative for numeric, scheduler-class, mechanism, and causal claims. Examples of directly observed shapes: GC span begins on trace line 5; a paired GC span duration is 8ms; no paired GC span in the bounded excerpt exceeds 50ms. Do not turn runnable time, non-running time, switching interval, or segment count into context-switch overhead, lost compute, frame-budget extrapolation, scheduler policy, wakeup mechanism, or root cause unless those claims are directly measured by typed trace evidence.",
			"Put any unstructurable chunks into residue[] (≤8 entries, ≤500 chars each). If the trace content genuinely has no jank / stall / startup signal but has relevant trace facts, emit those as observations[] plus meta.summary. Only emit residue-only when the trace is genuinely unparseable and no directly observed trace fact exists.",
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
			types.WriteAnalysisJSONShapeFirstTeaching,
			"Read the user's request from the active context. The request describes a code change, not a question — your job is to characterise the work, not investigate code in depth.",
			"Inspect the repository lightly to ground your classification: call repo_map for an overview — and when the change centres on a named existing file, repo_map(view=\"edit_impact\", target_file=\"<path>\") to see what an edit there would ripple into before judging scope and risk — then read_file or list_files on directories the request mentions. Cap pre-scan at 1-2 rounds — deeper planning happens after this classification.",
			"Decide the task category (feature / bugfix / refactor / test / docs / config / misc) based on what the change actually does. A new function added to fix wrong behaviour is bugfix; a new function added to extend capability is feature; renaming or restructuring without behaviour change is refactor.",
			"Decide the scope (micro / package / cross / project) by inspecting how widely the change ripples. A one-function change in one file is micro. Multi-file work in one Go package is package. Touching unrelated subsystems is cross. Build-system or repo-wide refactor is project.",
			"Decide the risk axes from what the change touches: affects_public_api when the change adds / removes / renames any exported identifier; changes_persistence when schemas, on-disk file formats, configuration shapes, or migration files are involved; changes_build_system when go.mod / package.json / Cargo.toml / build scripts / CI configuration are touched. These axes are advisory classification — the approval gate corroborates them against typed diff and path evidence — so flag them honestly rather than defensively. Pick an overall band (low / medium / high) reflecting the mutation's blast radius if the proposed patch is misapplied, not the severity of the pre-existing defect. A package-local bugfix that preserves public signatures and has no persistence, build-system, security, privilege, remote-execution, or irreversible-data surface is ordinarily low or medium; reserve high for genuinely broad or high-impact mutation surfaces. This is soft risk-calibration guidance only and never overrides the deterministic approval gate.",
			"Extract constraints the user explicitly stated (e.g. 'do not break existing API', 'keep the same file layout'). Skip when the user did not state any.",
			"Write 2-4 expected_outcomes — short concrete signals that the change is correctly done (e.g. 'a new --dry-run flag is wired through the CLI entry point and accepted by the existing flag parser', 'tests in <package> still pass'). These are the goal-checks the reflector will use to judge whether retries are moving toward what the user wanted.",
			// The escape-lane enumeration is single-sourced in
			// GateTeachingWriteExactContractGrounding (EVALFIX-2A):
			// the grounding gate's retry hint splices the SAME
			// constant, so the initial prompt can never teach a
			// drifted approximation of the gate's predicate.
			"When the request or light repo inspection gives concrete observable behavior, also emit behavior_contracts[] with stable ids. Use kind/operator fields for facts like exception type, output path/layout, stdout, status code, command result, or invariant. When the evidence gives both a failing surface and a grounded working or contrasting reference surface, attach that reference as comparator on the expected contract; emit the pre-fix failure separately with polarity=observed when useful. " +
				GateTeachingWriteExactContractGrounding.Text +
				" operator=satisfies is soft behavior guidance; do not hide exact expected values inside satisfies text when an exact operator would be required. These atoms let later verification probes reference contract_refs instead of relying on prose.",
			"STATE-TRANSITION CONTRACTS — when light repository inspection establishes shared mutable state or an ordered lifecycle/protocol, carry the behavior as behavior_contracts[].transition.steps[] with schema-known phases setup, action, observation, and postcondition. Preserve execution order. Include a non-initial-state sequence when setup changes the state consumed by the action; when two operations or directions mutate the same state boundary, include a cross-operation sequence instead of testing each operation only from a fresh object. Ground each step with request/test/file evidence when available, and keep inferred sequence semantics under operator=satisfies. This is soft context for planning and verification, not proof and not a hard plan gate.",
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
- behavior_contracts[] — optional typed observables { id, kind, subject?, operator, expected, transition?: { steps: [{ phase: setup|action|observation|postcondition, operation?, expected?, evidence_ref? }] }, placement?, comparator?, evidence_ref?, required? }
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
			"Choose one action from the current emit_write_workflow_decision schema's projected action enum. The action is the only controller routing signal; do not use an action absent from that dispatch's schema.",
			"When source understanding is missing for the current batch, emit explore_code with an exploration_request containing batch_id, goal, focused questions, candidate_paths, and evidence requirements.",
			"When the current batch is ready for a bounded ChangePlan, emit plan_batch with batch describing only that batch's goal, expected paths/kinds, and success criteria.",
			"When a typed ChangePlan is ready and the projected schema offers apply_plan, emit it. After apply succeeds, use verify_batch only when that action is present, so tests and structured verification decide whether the batch is complete.",
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
			types.ChangePlanJSONShapeFirstTeaching,
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
				"  - The @@ hunk header `@@ -OLD_START,OLD_LEN +NEW_START,NEW_LEN @@` declares the file line numbers and total line counts; OLD_LEN counts ' ' + '-' lines; NEW_LEN counts ' ' + '+' lines. Preserve the file's exact indentation and use real tab characters in JSON string values when the file uses tabs.",
			"EMIT THE BOUNDED PLAN THROUGH ONE STRUCTURED PATH: if the whole batch plan fits in one response, call emit_change_plan once. If large file bodies or patches would create output-size pressure, call emit_plan_skeleton first, then emit_plan_change once per non-delete file. The skeleton path carries request, summary, per-file metadata, and acceptance_tests; per-file emits carry only the bounded content for that change.",
			"For SINGLE-SHOT mode: emit exactly one emit_change_plan call. request must restate the user's ask, summary must be 3-10 sentences describing what the plan does and why, and changes[] must include one entry per target file with a Rationale explaining WHY that file needs that change.",
			"For MULTI-ROUND mode: emit emit_plan_skeleton ONCE (request, summary, changes[] metadata, optional acceptance_tests). Then for EACH change with kind ∈ {create, modify, patch}, emit emit_plan_change once with that change's path and the appropriate new_content (create/modify) or patch source (kind=patch: edits[] preferred for localized edits, raw patch for complex diffs). Do not call emit_plan_change for kind=delete entries — the skeleton already declares the deletion. The order of emit_plan_change calls does not matter; the LAST one (when every non-delete slot is filled) automatically runs the full validators (dependency closure, dry-build, summary fidelity, patch pre-check) and finalizes the plan. If a finalize-time validator rejects a specific file, just re-emit that ONE file via emit_plan_change to fix it — the partial state is retained.",
			"DEPENDENCY CLOSURE — when any new_content imports a package that is NOT already in the repo's go.mod (for example you added `import \"github.com/pkg/sftp\"` and the project never used sftp before), you MUST also include a 'modify' entry for go.mod that adds the require line, AND if the project tracks go.sum, a 'modify' entry for go.sum is also expected. The validator will reject the plan otherwise — the apply phase cannot succeed without this.",
			"USER-SIDE INSTALLABLE DEPENDENCIES — when your plan introduces a third-party runtime dependency that is NOT installed automatically by `apply` (e.g. a Python `pygame` import in a fresh repo without a `pyproject.toml`/`requirements.txt`/`pip install -e .` flow; a Node package not yet in `package.json`; a system package the code shells out to such as `ffmpeg` / `imagemagick`), you MUST list it explicitly in the plan's `summary`, in a clearly-marked section that names the package, the install command for the user's likely environment (pip / npm / apt / brew / dnf / pacman / scoop / etc.), and the symptom the user will hit if they skip it (typically a `ModuleNotFoundError` / `command not found` / `ImportError` at first run). Do NOT trust the user to figure this out from imports — surface every install-required dependency by name so they can copy-paste a single command. Pure standard-library imports never need a hint. Skip when the project already has a build/install file the apply phase updates (e.g. you added the dep to `pyproject.toml`).",
			"WIRING CLOSURE — when changes[] creates a new file under a registered subsystem (anything that needs a Registry.Register call to be reachable at runtime), you MUST also include a 'modify' entry for the corresponding wiring file in the SAME plan. The validator enforces this and rejects the plan with a concrete error naming the missing wiring file when violated — read the rejection text and add the modify entry. Without the wiring entry the new code is dead at runtime even if the file compiles.",
			"SUMMARY FIDELITY — every path-shaped token (e.g. internal/mcp/ssh.go) and every import path (e.g. github.com/pkg/sftp, golang.org/x/crypto/ssh) you mention in summary MUST match what changes[] actually contains. If your summary names a path, that path must appear in changes[].path; if your summary names an import package, that exact package must be imported by some new_content in this plan. The validator will reject the plan when summary lies about what's being changed.",
			"ONE CHANGE PER FILE: the changes[] array must NOT have two entries for the same path — the tool rejects duplicate paths. If a file needs two semantic edits, compose them into a single modify (full body) or patch (combined diff).",
			"MUTATIONS ONLY — changes[] contains files whose bytes, path, or existence will actually change. An expected invariant such as `go.mod remains unchanged`, `no generated files are touched`, or `public API stays the same` belongs in summary/acceptance_tests[], never in changes[] as an empty patch, no-op edit, or placeholder file. Preserve unchanged scope by omission plus verification, not by proposing a mutation that says it changes nothing.",
			"RESOURCE BUDGET — the verify stage will run your tests under hard caps (default 2 GiB memory, 600 CPU-seconds, plus the configured wall-clock timeout). A test that exceeds any cap is SIGKILLed and the verify→plan retry receives an explicit OOM / CPU-limit / timeout classification — meaning you don't get to blame 'tests failed' if the real cause is unbounded allocation or an infinite loop. To stay within budget: every loop in test or production code MUST have an explicit termination condition (no `while True:` / `for {}` / `loop {}` without a reachable break/return); every recursion MUST have a base case; every allocation whose size depends on input MUST validate the input is bounded before allocating; every blocking call (sleep / wait / lock / network / file open) MUST have a finite timeout. These rules apply to BOTH new test fixtures and production code the tests exercise. Raising the caps is NOT an acceptable fix — bounded execution IS the contract.",
			"Use depends_on for ORDERING constraints between changes in this same plan: when creating a new file X and then modifying an existing file Y that will import / call X, set Y's depends_on to [\"X\"]. The apply stage topologically-sorts before writing, so declaring the edge guarantees X lands on disk before Y tries to reference it. depends_on is ALWAYS repo-relative paths of OTHER entries in THIS plan — cross-plan or absolute paths are rejected, as is any cycle (a → b → a). Leave depends_on empty when the default declaration order is correct.",
			generatedArtifactVerificationDirective,
			"STATEFUL VERIFICATION — when a behavior contract carries transition.steps[], design acceptance tests or verification_probes that execute those steps in order against the changed production code. Do not replace the sequence with source-token checks, and do not test only a fresh initial object when setup intentionally creates a non-initial state. For shared cursors, lifecycle state, transactions, sessions, queues, or other multi-operation protocols, exercise an operation after another operation has already moved the shared boundary, then assert the observation and postcondition. This remains soft probe-design guidance; only executed probe/project-runner results own verification authority.",
			"PROBE DECISION FIRST — verification_probes[] are optional. When the typed test_surface advertises a native project runner for the changed language/package and the change is a local syntax/build repair, prefer acceptance_tests[] plus that verify-stage runner and omit a probe; the verifier, not the planner, establishes exact changed-path coverage by executing the runner. Add a probe only for requested behavior the advertised project surface cannot exercise, or when the controller explicitly marks a verification proof-follow-up. Never create a probe merely to reread changed source tokens, copy the changed implementation into a standalone program, or wrap the same compiler/test command.",
			"Optionally list acceptance_tests[] — natural-language test assertions the apply stage's verify phase should confirm. Empty is legal (no explicit tests to check). verification_probes[] are optional source-level programs, not command runners: emit one only when a supported inline runtime (python, javascript for Node.js, ruby, java via JDK javac/java, or go) can directly import or execute the changed production behavior. For C, C++, Rust, Cangjie, ArkTS, or any other target not directly executable through that lane, omit verification_probes[] and put the native build/test command in acceptance_tests[] for project verification; never launch an external compiler or test runner from a supported-language wrapper merely to bypass the runtime enum. Use repo-relative working_dir, a short timeout, and optional expected_stdout. When task framing lists behavior contracts, prioritize hard_required contract_refs; soft_required satisfies contracts are useful context but do not prove exact values unless the probe itself asserts the exact behavior from grounded evidence. Add changed_symbol_refs[] for the changed identity the probe imports or executes: use language-level names such as `Axis.convert` for symbols and `path:<repo-relative-file>` for a file/module. Do not put an extensionless import path in the symbol lane; the tool promotes it to path: only when it uniquely matches one changed file. A Go probe normally uses standalone `package main` code that imports the changed package. When the changed Go package is itself package main or the behavior requires an unexported symbol, instead emit a real same-package `TestX(*testing.T)` source, set working_dir to that changed package directory, and keep its package declaration equal to the changed package; the verifier mounts it through a temporary overlay and runs only that test. If a referenced contract carries comparator context, the probe should exercise the changed subject and the comparator relationship rather than only proving that the subject no longer crashes. If a referenced contract carries placement context, the probe should inspect the rendered line/surface and bind placement_refs[] to that contract id; contract_refs[] without placement_refs[] does not prove line-local placement. The verify executor runs these probes before project-level suites. A passing probe is bounded local behaviour evidence. A failing probe is an exact execution observation, but its model-authored expected comparator is not by itself authority that production code is wrong; project tests or grounded typed contracts must corroborate that comparator before source repair follows from it. Probes must import/use the changed code and assert the externally requested behaviour directly; do not copy an isolated implementation expression into the probe and test only that copy. Include both positive and negative cases when the reported defect is boundary-like; for run-length, cardinality, or threshold rules, exercise a triggering case and a nearby non-triggering case such as a singleton or just-below-boundary input. Probes must exit non-zero on failure; do not encode broad shell commands or environment setup.",
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
			"do not write an ordinary source-change plan whose changes[] array is empty. There are only two typed exceptions: a scheduler-labelled verification_proof_followup may use changes: [] together with verification_probes[] over the already-applied worktree; a replan may use changes: [] only when the emit tool accepts its typed passing-probe no-change sentinel. The tool validates both exceptions; never infer either exception from prose.",
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
