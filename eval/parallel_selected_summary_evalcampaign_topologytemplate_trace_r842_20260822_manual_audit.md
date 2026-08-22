# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T05:19:11Z
- sweep_start_ts: 20260821-221909
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-221911 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统投影完整且 0 次成文拒绝；模型把链上候选过强写成“主要阻塞原因/完全决定”，并猜测网络文件系统预取或缓存未命中，超出 typed 因果上限，按跨轮模型波动继续观察。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260821-221911 | answer_regex,answer_contains | none | 199s | 29 | read=14,repo_map=2,list=1,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | B1327 新模板真实发布且无 raw enum/占位符终稿泄漏；模型未采用模板，自造 send→send 自调用并一轮混用 replace_blocks 与 diagram_edge_edits，最终正文与图正确但经历 3 次拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### TypeScript relation answer

- 初始 finalizer prompt 和第二次 relation repair hint 均实际包含 `Typed topology authoring template`；8 条模板边使用
  `AUTHOR_BUSINESS_ACTION`，anchor 只带 typed identity/relation，不再把 `call` 当可见消息，也没有把模板称为可直接验收的答案。
- 模型首稿没有照抄模板，而是自写业务标签；第一拒由三个独立问题组成：缺少精确 `@app/core` 结构锚、`<br/>` 使 Mermaid 消息与
  `visible_label` 不字节一致、自造 `Send -> Send` 自调用且无 call authority。第二稿错误地给自调用补 anchor，第三稿同时提交同块
  `replace_blocks` 与 `diagram_edge_edits`，第四稿才收敛。没有一次拒绝来自 raw relation enum，故 B1327 的系统合同冲突已消失，但自然采用/低重试
  尚未得到正证。
- 终稿正确给出 `run -> ApiClient.fetchUser -> HttpTransport.send -> HttpTransport.dispatchOnce -> fetch`，另列
  `send -> sleep` 重试等待分支；`@app/core` 精确落到 `tsconfig.base.json:8 -> packages/core/src/index.ts` 并说明子包 extends。Mermaid 语法合法、
  四条正向调用有业务标签和 typed anchors，虚构自调用已删除。小缺点是导语称“5 个节点”但正文另列 sleep 分支，计数口径不够严谨。

### Explicit-window Trace answer

- 用户窗严格保持 2.000000..2.020000，3 次 target-filtered typed query；系统成文前补采仍产出 app-100 全窗状态、三跳唤醒链、
  threadpool-400 11.000ms IO 第一席、三个 1.000ms runnable/优先级候选、实际占时/规则可消双账户、链上/邻近/背景分区和完整
  `Trace 因果投影`。0 次 finalizer reject，无旧稿恢复或固定时长降级。
- 系统上下文明确发布 `target_direct_blocking_authority=not_provided`、wakeup 不等于同步 blocker、caller 只证明本席内核等待调用点且不证明资源/后端；
  模型仍写“主要阻塞原因”“app-100 睡眠完全由整条依赖链最长节点决定”，并猜测网络文件系统预取/缓存未命中与“同步读”修向，属于答案因果越界。
  r841 同一用例曾遵守相同边界，因此本轮不足以证明新确定性系统 gap；保留 `B1269/B1271` 软教学观察，不扫描/拒绝/删除/改写模型正文。
- 模型另称“唤醒链路共 4 跳”，正文实际列 3 条 wakeup edge 加目标线程状态，是线程数与边数混淆；同样按模型算术波动记录，不增加单样本硬门。
