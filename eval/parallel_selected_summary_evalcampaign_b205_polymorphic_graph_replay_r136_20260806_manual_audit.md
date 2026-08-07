# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T00:04:01Z
- sweep_start_ts: 20260806-170400
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-170401 | answer_regex,answer_contains | none | 90s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Class/registry/lookup conclusion is right, but the answer omits the executor callback-handoff boundary and the actual `TimestampMixin -> ValidationMixin -> BasePlugin` cooperative `handle` path. `runner.py` was injected as exact pre-read and counted as read coverage, yet the grounder rejected line 17 as absent from `read_file` history; callback authority was therefore lost and the diagram was deleted after two rejects. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-170401 | answer_regex,answer_contains | none | 135s | 21 | read=4,repo_map=3,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | Factory choice, injection, virtual write and `fputs` are mostly recovered in prose, but the relation layer cannot render selection/injection/virtual dispatch without pretending they are direct calls, so the model deletes the diagram after two rejects. The summary and final hop also say stdout while the cited code passes `stderr`; the generic runner oracle misses this contradiction. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- Both cases used native valid JSON: strict decode remap/carrier/element/string-recovery counters are all zero. The four failures are post-decode semantic-contract rejects, not malformed JSON recovery.
- `PREAUTH1` (P0 contract contradiction): prompt pre-read is presented as saving a `read_file` call and is entered into typed read coverage, while the grounder accepts only explicit `read_file` tool history. The same exact source is simultaneously “already read” and “not read”.
- `POLYGRAPH2` (P1 expression gap): selection, constructor injection and virtual dispatch are principal mechanism relations but lack an adequate typed diagram composition. Forcing them through `call` correctly fails; deleting the diagram loses the mechanism the user asked to understand.
- `PATHSEM1` (P1 context/answer gap): Python evidence contains all three `handle` definitions but no structured cooperative-dispatch obligation reaches the finalizer. The model collapses the MRO path to `BasePlugin.handle`.
- `FACTCHECK1` (P1): C++ citations show `stderr`, but the answer says stdout twice. Existing regex oracles test named nodes only and do not catch a conclusion/citation value contradiction.

## Batch decision

1. S35c first: make exact prompt pre-read bytes a persistent, task-scoped grounding authority; explicit `read_file` remains the newer overlay. This removes the self-contradictory contract and lets the existing cross-language callback classifier consume the source the model actually saw.
2. S36: add typed composition for runtime selection/binding/virtual dispatch and cooperative override paths without widening any of them into direct-call proof.
3. S37: add typed value-consistency guidance/checks from citable source fields (for example stdout/stderr) without scanning answer prose as a hard gate; expand human/oracle coverage through structured answer facts rather than fixture words.
