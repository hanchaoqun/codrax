# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T17:08:29Z
- sweep_start_ts: 20260802-100828
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | FAIL | eval/results/operation_web_manual_summary-20260802-100829 | log_regex,answer_regex | none | 59s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | command_rounds=1 | partial | runner FAIL 是 oracle 只接受连续“用户使用手册”或英文 manual/guide，而正文标题为“CODRAX 用户手册摘要”；主体包含安装、PATH、provider、启动、REPL/CLI 与 8 场景。真实不足是 evaluator 忽略 excerpt_truncated=true，仅凭 source_truncated=false 在首页后提前 complete，并把详细文档留给用户再请求。 |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260802-100829 | log_regex,answer_regex | none | 209s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data=11,repair=6,failed=9 | fail | 初始 plan 走完整 typed ledger 链后已有 decisions=5、rules=3、target contributions=2；中间 reconcile 确定性得到 2，却把早期 artifact summary 当最终 answer，与 expected=2 交叉核对，最终失败。见 EVAL-B44-RECONAUTH1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. operation 的产品答案不是 runner 所称“没有 manual/guide”：中文“用户手册摘要”与该语义
   等价。现有 oracle 将可变文案当固定术语，属于看护过硬；放宽为 `用户(使用)?手册|使用指南`
   只影响 eval 评分，不进入产品路由/答案 gate。
2. operation evaluator 收到了 `excerpt_truncated=true` 的精确上下文，仍在思考与 typed reason
   中只引用 `source_truncated=false` 并提前 complete。这是模型没有遵循已足量软指导；本轮不
   再叠加 hard gate 或答案改写，保留 variance watch。
3. data 的失败与 B44-a output-contract 修复无回归关系：intermediate publication 仍应
   freeform。真正 split-brain 是 reconcile 也拿这个 freeform 判断旧 summary 是否“可比较答案”，
   遗失了 workflow `json_only` authority。
4. 本对无 trace attachment/query；修后的 final_projection metric 为 0，跨模式量具污染已覆盖。
