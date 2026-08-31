# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T04:11:00Z
- sweep_start_ts: 20260830-211058
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-211100 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、4 次 typed trace_query、最终因果投影、链上 NetworkService 第一席、实际占时/规则可消双账户、邻近/背景隔离均在；无固定 4ms/4m 或活动流时限降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260830-211100 | answer_regex,answer_contains | none | 706s | 63 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=21,inv=5/0,fin_reject=14,unavail=0,prune=0 | pass | 最终答案含四阶段表和合法 sequenceDiagram，三条 checkout-verified precedence 均在；未降级。但 14 次拒绝暴露候选生产者未发布执行器所需的多个精确已声明 stage ID，过程判 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. Trace 人工判定通过。目标窗保持 `34579.490..34579.500s`，系统消费 4 次 typed `trace_query` 并发布最终 `Trace 因果投影`；
   `NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 为已证链，链上第一席、实际占时与规则可消量分账、确定性语义
   优化线索和邻近/背景隔离均未缩水。没有按 4ms、4m、轮次、上下文比例或活动流年龄降级。
2. read 最终答案人工判定通过、过程判定 partial。最终 sequence diagram 使用精确声明的 `StageAnalyze/StageExplore/StageExtract/StageFinalize`
   并包含三条顺序关系，表格覆盖四阶段输入、输出与主要载体；没有恢复旧稿或系统代写结论。r951 的 B1467 多词 participant 自锁未再发生，
   但本轮没有自然触发 `read mode` 的精确多词显示修补，故 B1467 只可记为确定性回归无退化，不能宣称生产正证。
3. 14 次成文拒绝中，模型 JSON 形状、whole-block 非授权操作和条件孤儿清理误用属于模型/教学噪声；其中一项是确定性系统合同缺口
   `B1468-DECLAREDSTAGECHOICESET1/P1`：同一 verified stage 同时有 `S1 as StageAnalyze` 与 `StageAnalyze as StageAnalyze` 等多个合法显式
   participant 时，候选只发布抽象 `analyze`，而执行器正确拒绝在多个声明间猜测。模型直到最后自行猜中 `StageAnalyze` 才通过。
4. `3f150be9f` 已根修 B1468：precedence lease 按 checkout-verified stage authority 发布该侧所有精确已声明 participant ID；存在精确声明时剔除
   会创建隐式重复泳道的未声明 fallback。模型仍选择使用哪个候选、是否添加关系、业务标签与布局；执行器的歧义 fail-closed 保持不变。定向测试和完整
   `go test ./internal/tool -count=1`（183.286s）通过，等待下一轮生产回放。
