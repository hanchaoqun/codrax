# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T05:57:06Z
- sweep_start_ts: 20260828-225704
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-225706 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 42 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 34579.472865..34579.587805 窗、四跳唤醒链、链上根因席、实际占时/规则可消双轴、D/io_wait 与 block issue/complete、确定性语义、业务线索、代表窗、完整 Trace 因果投影和自动补齐均在；邻近/背景未进入主因，计数当量与 IO 活动指数没有再被伪装为毫秒。模型仍在开头把“优先级反转候选”概括成“显著的优先级反转卡顿/两段式阻塞”，并把仅证明调用位的 fscache/page-lock 候选写得偏具体；后文 typed 边界又正确否认锁持有与帧因果，属于 B1269/B1271 的模型遵循重复 witness，不新增正文扫描硬门或系统改写。 |
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260828-225706 | answer_regex,answer_contains | none | 222s | 38 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B1440 生产正证：最终 Mermaid 保留 6 条首稿关系，其中 analyze→explore→extract→finalize 三条主链使用 checkout-verified precedence anchor；零 identity conflict、零 relation reject、零关系删除。唯一拒绝是表格行同时携带 cells 与可见 label/text 的结构形错误，模型按精确提示一次重发后通过，随后仅补 member_set facet；未触碰图关系。可选图仍未变成必须完整图，系统没有创建、选择或改写关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. `B1440-OPTIONALSTAGEEDGEAUTHORITYDRIFT1` 获得生产正证并闭环：可选架构图可以消费已接地的阶段先后关系，而不会借此启用“必须画完整图”合同；主阶段三条关系没有再因 Stage/Agent 别名冲突被删除。
2. architecture 的一次重试是表格行 schema 形状错误，提示精确、单轮修复成功、关系块未被重写；暂不升级为新 GAP。最终 6 条关系均保留，图不再退化为孤立节点集合。
3. Trace 回放确认 B1440 不触及运行时权威域：显式时间窗、链上根因、两个分析维度、因果投影和自动补齐无回退。模型对候选机理的措辞仍偏强，继续记在 B1269/B1271 的软教学/模型遵循观察项；禁止按请求或答案词面硬拒，也禁止系统代写结论。
4. 两路都没有 JSON salvage、旧稿恢复、系统替写答案或固定 4ms/4m 活跃流降级。
