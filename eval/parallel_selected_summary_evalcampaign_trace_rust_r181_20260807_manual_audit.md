# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T18:42:28Z
- sweep_start_ts: 20260807-114226
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260807-114228 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 137s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、四次 trace_query、wakeup `net-300 -> worker-200 -> app-100`、app sleep 10.000ms、worker runnable 8.300ms、根因排序、窗内可消量和双维根因表均完整，且系统投影没有替换模型正文。但正文首句把 `priority_inversion_candidate` 直接写成“主要阻塞原因是优先级反转”，后文又称“典型反转/优先级继承”，与同稿 caveat“无 holder/waiter typed 证据，仅 validation_candidate”冲突。Prompt 已精确要求候选措辞，属于模型权限服从波动；不扫描正文做硬门、不让系统改写，跨 trace 继续观察。 |
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-114228 | answer_regex | none | 156s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | 文字与 5 条 source call citation 正确，walker 角色及 matcher runtime selection 完整；但系统 copy-ready 图把同一 Rust 函数拆成 `walker::collect_files` 与 `collect_files` 两节点，导致图上 `run -> walker::collect_files` 和 `collect_files -> walk` 断开。模型试图补一条不存在的桥后被 validator 正确拒绝，patch 只能保留断图。确认 B299：renderer 未消费 validator 已有的“typed inbound qualified endpoint + unique source-local definition”身份桥。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
