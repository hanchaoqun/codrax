# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T20:37:57Z
- sweep_start_ts: 20260818-133756
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260818-133757 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 283s | 41 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | partial | B1105b 生产正证：系统逐条观测块用读者语言保留 11ms 链上 IO、四级唤醒链、次级调度延迟、窗口与下钻建议，不再显示 `source=/causality=/chain_depth=/recommended_views=`；Trace 因果投影、自动补采、链上-only 主因、背景降格与双轴均保留，零成文拒绝，活跃流跨 4ms 正常。新确认 B1107：`root_cause_*` 非链上行虽显示「因果依据：目标自身/时间相邻/同窗背景」，行首仍统称「根因观测」，与主因链上-only 红线冲突。模型正文另有无证据的 NFS/CIFS 猜测，并将 3 段 typed 互斥可加的 1ms 既写「合计 3ms」又写「不可直接相加」；不以终稿扫描/系统代写修复。同一三视图在两个 explorer 车道重复查询 6 次，记 B1108 作类型化调用等价/缓存审计。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-133757 | answer_regex,answer_contains | none | 369s | 32 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | B1106 生产接线正证：初始 Finalizer 上下文已出现单一 `diagram_block_sibling_fields_json` 对象，同时携完整 edge anchors 和两条 participant boundaries；硬门、引导与原子载体现在同源。模型首稿仍选择性漏了 boundaries，且把非端点项混入 `principal_path_edge`，因此一次拒绝是实际的模型服从/结构组合失败，不再是 B1106 上下文缺载体。两次 patch 后最终保留正确 shared-callee 图 `buildAnalysisIR -> gate.RunWith <- gate.Run`、两条未证请求边界和 15 个中间函数。不放宽关系门，不由系统代画；后续从上下文压缩/结构示例心智负担评估，不对该 case 硬拟合。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

1. `B1106-DIAGRAMPARTICIPANTSLATESINGLEOWNER1=production-positive`：生产 prompt 已出现完整原子 sibling
   carrier；本轮重试不能再归因于系统漏发 participant slate。模型最终内容正确，但 1 reject/2 patch 仍使
   369s 用时和上下文负担偏高，继续作为异构服从/结构教学观察项，不新增终稿词面门。
2. `B1105b-TRACESYSTEMAPPENDIXREADERPROJECTION1=production-positive`：系统 appendix 的 wire enum
   泄漏已关闭，数值、坐标和 typed 因果边界没有丢失。
3. `B1107-TRACEAPPENDIXCAUSALPOSITIONLABEL1=P1/confirmed`：root-cause family 的读者行名必须优先读
   typed 因果位置；`self_wall_clock/adjacent/background/unproven` 不得仅因 ClaimKey 前缀而称根因。
4. `B1108-TRACEQUERYSEMANTICDUPLICATION1=P2/confirmed-audit-needed`：两个 explorer 车道对相同附件、
   目标和窗口各执行 window/wakeup/rank 三视图；参数只在 platform alias 和默认选项上有表面差异。应先
   统一 canonical call identity 与结果复用，不得简单删一车道或减少因果视图。
5. 两案均未出现畸形 JSON 恢复、空答案、固定 4ms 降级或系统替写模型结论；显式窗 Trace 因果投影与
   自动补齐完整保留。
