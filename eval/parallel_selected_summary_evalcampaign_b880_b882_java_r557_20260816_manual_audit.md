# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T08:39:10Z
- sweep_start_ts: 20260816-013908
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260816-013910 | primary_answer | none | 83s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Typed context and final prompt both carried `AuditLog.record -> System.out.println` and kept durability/database semantics unproven, but the model still wrote “标准输出，完成审计落库”. This is a model instruction-following failure, not missing evidence. It also labels five listed methods as “5 跳” while the visible interaction graph contains side calls and a principal write path, so hop/path semantics remain imprecise. One diagram metadata repair was valid. No duplicate rejected draft or duplicate output-dimension supplement remained. Keep as a cross-model/language replay observation; do not add a prose scan/hard rewrite. |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-013910 | answer_regex,answer_contains | none | 330s | 41 | read=17,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B882 is production-positive: rejected first draft is absent and the requested-dimension supplement appears exactly once. B880 is only partial: Analyzer preserves both split-clause participants and completion refuses silent closure, while the soft repair names `cmd/root.go` first; Explorer nevertheless spends 24 iterations/4 completion calls and never reads that registration site, then the final diagram presents disconnected local fragments and unsupported narrative about finalizer ownership/selection. Runner PASS checks literals/sections only and misses the relation failure. Root gap is typed repair-plan consumption/scheduling, not missing relation vocabulary and not a reason to let the system author an edge. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
