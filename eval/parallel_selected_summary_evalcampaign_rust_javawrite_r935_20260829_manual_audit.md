# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T09:54:33Z
- sweep_start_ts: 20260829-025431
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260829-025433 | write_plan,write_patch_oracle | none | 84s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | The one-line `retrun -> return` plan is exact, but the planner emitted a Java probe that launches `javac Main.java` instead of directly exercising `Main`; the plan gate accepted it because the old Java coupling lexer read `Main` from the command string. |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260829-025433 | answer_regex | none | 146s | 28 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | The final answer keeps the two `run` branches separate, gives the exact call sites, and explains walker as the file-collection stage. The prior false `walk -> index_file` linearization and six-node/“six hops” arithmetic are absent. No diagram was emitted, so B1448's alias-repair production arm did not naturally trigger. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Rust read mode

- Final answer correctly represents sibling branches rather than one invented linear chain:
  `run -> walker::collect_files -> walk -> fs::read_dir` and
  `run -> index_file -> read_to_string / Matcher::is_match`.
- `walker` is accurately characterized as the recursive file-discovery stage that excludes hidden and `target` directories and returns the file list consumed by `run`.
- All listed source coordinates match the explored files. The answer contains no unsupported `walk -> index_file` edge and does not confuse node count with hop count.
- Finalizer rejected zero drafts. The model chose a prose/list answer rather than Mermaid, which is valid for this request. Consequently this run is a no-regression signal for B1448, not production proof of its `existing alias -> allowed addition -> atomic repair` path.

### Java plan mode

- The proposed source edit is minimal and correct: one replacement at `Main.java:16`, with no unrelated file or line.
- The planner prompt explicitly taught both relevant boundaries: local syntax/build repairs should normally omit an inline probe, and verification probes are source-level programs rather than compiler/test command wrappers.
- The emitted `compile_check` nevertheless calls `Runtime.getRuntime().exec("javac Main.java")`. It never imports, instantiates, or invokes `Main`; a successful run would prove only that a nested compiler command exited zero.
- The old hard coupling gate incorrectly accepted the probe because `javaTypeTokens` scanned quoted literal contents and treated `Main` inside `"javac Main.java"` as a real Java type reference. This is deterministic authority pollution, not a need for a source-keyword ban and not merely model variance.

### Resolution

- Filed and fixed as `B1449-PROBECOUPLINGLITERALAUTHORITY1` in `fcbc48a97`.
- The coupling extractors now derive module edges only from executable language carriers: stateful non-code masking for Python/Java, Go AST imports, and comment/string-aware JavaScript/Ruby import/require tokenization.
- Negative pins cover import-shaped text in Python docstrings, JavaScript/Ruby literals and comments, Java command strings, and Go raw strings. The original Java wrapper is rejected for missing changed-class coupling; legitimate direct imports/references continue to pass.
- This fix does not scan the user request, model reasoning, answer prose, or visible labels, and it does not ban subprocesses by keyword. A probe may still exercise product behavior that legitimately spawns a child process, but its typed authority must come from a real import/reference to the changed product target.
