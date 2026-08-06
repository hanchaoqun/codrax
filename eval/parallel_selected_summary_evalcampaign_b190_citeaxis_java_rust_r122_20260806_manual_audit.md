# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T19:56:31Z
- sweep_start_ts: 20260806-125629
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_trait_impls | PASS | eval/results/sr_rust_trait_impls-20260806-125631 | answer_regex | none | 87s | 20 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 两个实现、`--fixed`/default 条件和实现行 17/33 均正确，runner 绿；但模型提交 main.rs 选择行后，系统按 label 重绑到 impl 行，再由只认 struct 行 7/23 的 principal coverage 判成 2 条 soft violation，最终追加“证据支持稍弱”。同一 typed 管线先选实现证明、再否定它，属于系统等价锚接线矛盾，不能以 runner PASS 签绿。 |
| 1 | sr_java_handler_impls | PASS | eval/results/sr_java_handler_impls-20260806-125631 | typed_inventory_rowset,answer_regex,answer_contains | none | 550s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S20 保持：三实现/三路径、type 角色、零 role caveat，runner 绿；但模型把相邻 `@Route` 文本拼进 class-line quote，quote normalizer 纠正后显式引用只剩 class 行，不能直接证明路径。系统又追加一份确定性 roster，造成主清单重复；550s 主要是 provider/analyzer 第二轮等待，finalizer 无重试。S21 软教学未稳定消除二轴引用弱化。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
