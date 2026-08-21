# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T17:33:30Z
- sweep_start_ts: 20260821-103329
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-103330 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 190s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s、四节点唤醒链、11.000ms 链上 IO 第一席、三个 1.000ms 优先级候选、实际占时/规则可消双轴、业务下钻、背景隔离、自动补采和完整 Trace 因果投影均在。模型先写“阻塞原因完全来自依赖链上游”，后文又正确披露 wakeup 不证明同步阻塞，属 B1269/B1271 已知措辞波动。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260821-103330 | log_regex,answer_regex | none | 290s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | terminal=complete、repair=0、rules=13、decisions=17、contributions=4、reconcile=pass，最终唯一输出 17,0,5。旧 required_material_scheduling 三材料误拒完全消失；B1302 获生产正证。一个跨 DAG rank 计划被系统安全拆批，未污染结果。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

1. `B1302-CUMULATIVEMATERIALLINEAGE1` 已生产转正。数据终态为 complete，6 个有界 typed batch、0 repair round；13 条规则、17 条决策、4 条贡献、
   9 个累计消费路径和 pass reconcile 全部在，最终答案长度 6 且唯一可见内容为 `17,0,5`。r818 的三份 CSV “未在终批调度”误拒没有出现。
2. B1301 的 reference scope 所有权继续正确：模型依据 typed `targets` artifact 选择 GroupA/GroupX/GroupC，系统只按已声明 reference 逐槽投影并对
   GroupX 零填充，没有回退 present-groups 的 `17,4,5`，也没有从请求/规则 prose 自行选择引用集合。
3. 数据过程中一个初始计划跨越 join/compute/final 多个依赖 rank，被现有 precise DAG guard 拆成 deferred typed queue；一个 deferred assemble 的字段
   合同也在真实 compute schema 出现后被阻断。两者均正确 fail-closed 并最终从真实 artifact 收敛，不构成 B1302 回归。总耗时仍为 290s，后续以异构
   data case 观察 schema/JSON 教学效率，不按固定轮数或时间阈值提前终止。
4. Trace 系统面通过：3 次 query 全部保留请求窗和 pid 过滤；投影按链上 11.000ms IO 第一席、三个独立 1.000ms runnable/优先级候选排序；目标
   20.000ms sleep 为实际占时症状，不直接冒充可消量；邻近 sleep/runnable 与背景 IO 活动指数均未进入主因排序。自动 critical blocking 补采和
   `fscache_page_wait_on_page_bit` 调用点边界也完整。
5. Trace 人工仅判 partial：模型首段写“阻塞原因完全来自依赖链上游”，后文同时明确优先级候选和 wakeup 不证明 app 同步阻塞等待工作完成，且目标自身
   sleep 原因未归因；“同核唤醒可减少开销”也只是未量化建议。这是已有 B1269/B1271 的模型因果措辞波动，typed 上下文与系统投影边界准确，不授权新增
   请求/答案关键词硬门或系统改写模型正文。

状态：

`r819=runner-pass-2/2,human-data-pass+trace-partial`；
`B1300=production-positive/core-closed`；
`B1301=production-positive/core-closed`；
`B1302=production-positive/core-closed`；
`data-reference-output=17,0,5+complete+repair-0`；
`B1269/B1271=model-causal-wording-repeat-partial/no-hard-prose-gate`；
`system-answer/conclusion/reference-scope-authorship=none`；
`request/model/final-prose-hard-scan=none`；
`Trace explicit-window/causal projection/auto-supplement=production-positive-r819`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/production-positive-r819`。
