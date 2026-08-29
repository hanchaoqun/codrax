# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T05:36:38Z
- sweep_start_ts: 20260828-223637
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-223638 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1423/B1439 生产正证：模型上下文与终稿不再泄漏 causal/frame 原始枚举，page_cache_churn 不再被写成 7.200ms，IO 行明确是跨单位活动指数且绝对高低未定义。显式窗、5 次约束查询、四跳链、链上主席、实际占时/规则可消双轴、D/io_wait、确定性语义、业务线索、因果投影和自动补齐完整。剩余问题是模型无视精确校准写“CPU/IO 中高”、把两个重叠主席相加为 38%，并把候选优先级关系扩写成“队列占满/大幅倒置”；系统投影已明确重叠不可加，故记 B1269/B1271 与算术遵循的重复 witness，不新增 prose 硬门或系统改写。 |
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260828-223638 | answer_regex,answer_contains | none | 192s | 32 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=4,unavail=0,prune=0 | partial | 新确认 B1440：首稿 6 条 Mermaid 关系方向和正文均正确；提示已发布三条 checkout-verified 主阶段 precedence 配方。因 analyzer 将图标为 optional，校验侧没有加载同一 edge authority；两行标签里的 Agent 身份又被唯一解析后与 Stage anchor 冲突。修补器不允许新建 Agent 节点或整块替换，模型最终删除全部箭头，图退化为 7 个孤立节点。确定性合同冲突，非模型波动。已按“可选已证边权限”和“必需完整图权限”分层根修；反向边、Trace 借权及未证 pre-stage 边仍 fail-closed。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B1423 获得生产正证：模型可见上下文只给自然语言因果/帧证据边界，原始控制枚举未进入终稿。
2. B1439 的计数/跨单位语义获得生产正证；模型仍把绝对等级写成“中高”属于明确教学后的遵循波动，不能倒逼系统扫描答案原文或替模型下结论。
3. B1440 是提示与验证消费者对同一 typed stage recipe 的权限漂移。修复只让模型已选择的 checkout-verified 相邻阶段边通过；是否画图、画哪些真实边、业务标签和布局仍由模型决定。
4. 两路均没有旧稿恢复、JSON salvage、系统替写答案或固定 4ms/4m 活跃流降级。Trace 的窗口、链上根因和投影路径未被图关系修改触及。
