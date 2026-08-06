# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T16:59:10Z
- sweep_start_ts: 20260806-095909
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260806-095910 | answer_regex,answer_contains | none | 130s | 23 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 七个 stage、职责、条件触发和图均完整。首稿把 pipeline 顺序误声明为 typed call，精确证据门一次拒绝后改为 precedence；属于有界关系分类纠错。operator ledger 仍记 citation_quote_rewrite=3，但用户面没有再误报系统降级。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-095910 | typed_inventory_rowset,dimension_substring,answer_contains | none | 418s | 30 | read=0,repo_map=1,list=2,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | typed 事实、row-id、引用均为正确 2 extend + 2 foreign func + 8 public class；但三个 family section 先连续出现，随后三个无标题通用表连续出现，可见归属脱节，oracle 将 12 行全归到最后一个 public class。首稿 blocks string 仅部分恢复后正确拒绝，patch 成功；citation_quote_rewrite=12 仍只在 operator ledger，用户面无误导 footer。analyzer 首轮 4m54s 且 think 被截断前已达 75424 字，记为 provider/model 波动与上下文心智观察项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
