# r1021 人工审计：主干 Trace / C++ 写模式

- date: 2026-09-06T07:01:05Z
- sweep_start_ts: 20260906-000101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `224a6b7d5`，干净二进制快照，两路并行。已读完整控制日志、结构化 full/patch 工件、写模式终验和 applied diff；原机器结果 0/2 保留，不因离线修复重签。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260906-000106 | write_apply,answer_regex | none | 146s | 28 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 加法前拓宽和传参修向正确，测试未改；两次聚合命令通过但断言级凭证缺失，终验诚实 unverified。B1561 再现 |
| 1 | real_trace_h11_cross_direction_overlap | FAIL | eval/results/real_trace_h11_cross_direction_overlap-20260906-000106 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 51 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=3,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | 10块/5个ID歧义草稿按ID排序panic，整份答案/因果投影未落盘。强制根因旁路仅产生unavailable兜底。P0 B1565 |

## 证据与判定

- Trace 会话 `20260906-000110-000-60534` 的 full emit 参数保留两套不同内容的 summary/section，ID 重复。最初仅以“至多一个 summary”引导；后续两次对同 ID 同时 remove+replace 被正确拒绝，第三次 reorder 用唯一 ID 列表命中缺前置身份校验的数组越界。问题是系统把不可寻址的草稿宣称为 patch 基线，不能仅归咎模型畸形 JSON。
- 首次成文上下文约 45%，批次最高 51%；Trace 精确状态、58.320ms 供给折算、12.658ms/47段 IO 阻塞、链上低优先级依赖方候选与业务 span 等事实已在工具/名册供给，未到预算上限。草稿仍有折算口径/上下界/内部枚举措辞误读，暂不增加正文硬门或系统代写。
- 没有可人工验收的最终 Trace 正文/图。`.codrax/output/20260906-000421.819-60534.root-causes.json` 132 字节 unavailable 兜底证明必选旁路仍尝试落盘，但不补足丢失的模型答案。P0 B1565 应先修并复放。
- 写模式 applied commit `1f857bb458a1bc4f7cbd1144652261d2c7056b4f`：`render_year(int)` 改为 `long long`，`year_offset + 1900` 改为加法前 `static_cast<long long>`。2+/2-，未删改原测试；项目命令两次成功仍无 required 断言 receipt。没有自动合入用户主干。不能用模型的“未测试”措辞代替实际命令记录，也不能用聚合成功推导断言级证明。
- 本轮无流式 4ms/4min 无正文截断。调用方取消/deadline 与无活动 watchdog 仍应保留；B1565 是独立 patch 结构崩溃。
