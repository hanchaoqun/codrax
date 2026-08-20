# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T17:49:52Z
- sweep_start_ts: 20260820-104951
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-104952 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass-with-model-wording-watch | 显式 2.000..2.020s、6 次 trace_query/自动补采和 Trace 因果投影完整；threadpool-400 iowait 11.000ms 为已证链上首因，三个 runnable 1.000ms 席归调度供给，邻近 sleep/背景 IO 指数未入根因。无流式时间降级。模型导语把“目标仍在 sleep”说成 RT 线程未能优先获调度，并把 11ms IO+3ms runnable 说成构成完整 20ms，均比 typed 投影更强；保留为上下文校准观察项，不能据此改写模型答案。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-104953 | answer_regex,answer_contains | none | 1303s | 58 | read=61,repo_map=4,list=0,trace=0,source_lens=0 | midloop=31,inv=10/0,fin_reject=12,unavail=0,prune=9 | fail | 活跃流未按 4ms/4m/首字节/stall 降级；失败来自成文关系修补震荡。首轮 JSON 恢复后末块 kind 丢失，第二轮完整稿进入 typed relation 修复；之后 11 次整块 diagram patch 在修一处时误删另一条未点名已证边，或在 participant/return 合同间迁移。第 13 轮才被接受，但最终图缺失 Run→Graph、Finalize/Extractor 主段，runner 精确报 no_regex_match:(extractor|Extract)。B1249 候选同轮权限没有越权，真正残余是整块替换缺少模型声明的原子关系操作。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `B1249-RELCANDIDATETXN`：结构化许可与未列关系拒绝均按设计生效，但不能关账；模型仍需重发完整 Mermaid body 与完整 `edge_anchors`，所以一个局部修复可误删无关关系并触发下一合同。
- `B1251-RELATIONPATCHATOM1/P1`：需要模型所有权不变的原子 diagram relation patch。模型声明精确 block/edge、操作和新可见标签/节点；系统只把该声明事务化应用到上一版模型图，未触及的关系结构保持，不由系统选择、生成或改写关系与结论。
- `B1252-EXPLORERCLOSURE1/P1-observe`：read 车道三个 Explorer window 共约 93 个迭代、61 次 read、193 条 evidence，closure repair 后仍继续导航，最终成文起始上下文约 61.8k；需另审 typed completion/closure 是否缺少可执行停止信号，不能用源码或请求关键词硬截断。
- `B1253-TRACESLEEPPRIO1/P2-observe`：Trace typed 投影正确，但模型导语把睡眠期间的上游链等待表述成目标 RT 线程“未能优先获得调度”，并用 11+3ms 支撑完整 20ms。后续应校准模型上下文：目标未 runnable 前，优先级不能解释目标调度延迟；链上多席位墙钟不可直接补足窗口。只作软教学/typed 口径，不做答案扫描或系统代写。
