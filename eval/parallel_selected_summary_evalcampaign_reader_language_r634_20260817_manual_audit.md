# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T18:37:15Z
- sweep_start_ts: 20260817-113714
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-113715 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 主窗、链上 #1=worker-200/8.300ms、投影和背景分权均正确；模型仍在摘要/风险段/表格复制 raw cause token，把候选先写成“典型反转场景”，并给无排序席的 app-100 sleep 虚列 #2。末端映射提示不足以抵消大上下文中的重复 raw token。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-113715 | answer_regex,answer_contains,mermaid_edge_count | none | 319s | 34 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | partial | 最终图保留阶段顺序及 BusContext/Mutable 无箭头包含，源码坐标已退出可见边；关系标签仍写 raw `precedence`。首稿虚构多条状态数据流，首修又以 `MS` 代替 exact participant `Mutable`，第二次修补才通过。正文仍把若干未证读写写成事实。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

1. B997 的 prompt-only active map 已接线但生产无效：模型看到了自然语言映射，仍从更大、更重复的
   raw rank/ledger/schema 上下文复制 wire token。不得升级成答案字符串扫描、硬拒或 renderer 改写；
   下一步应让 finalizer 的读者合成面消费单一 display carrier，并减少不属于 JSON 发射所需的 raw 重复。
2. Trace 确定性投影无回归，但模型又给 `target_self_state` 虚列 #2。B995 的精确主窗 roster 已在末端，
   说明多个旧 rank 表与新 roster 并置仍增加心智负担；应按消费角色拆开 machine/audit 与 reader synthesis，
   而非关键词封锁 `#2`。
3. B998 部分生效：图不再出现 `@ file:line`，但 `precedence` 仍是内部关系枚举。业务标签教学同样被
   typed recipe 的 raw relation 名压过。关系事实和可见标签应成为同一 recipe 的两个字段。
4. 新确认 B1000：no-arrow grouping 只有结构化描述，没有 copy-ready typed skeleton；模型用短 id `MS`
   后触发 exact participant 可见性拒绝。可由 parser-owned owner/member/type 生成不带箭头的 Mermaid 片段，
   仍不选择布局外关系、不生成答案、不虚构边。
5. 两条活跃流均远超 4ms 并正常完成；无固定累计年龄降级。
