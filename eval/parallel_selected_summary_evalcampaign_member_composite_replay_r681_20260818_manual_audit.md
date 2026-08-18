# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T11:03:25Z
- sweep_start_ts: 20260818-040323
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-040325 | answer_regex,answer_contains | none | 106s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Windows/Apple/POSIX API 与 `cmd_sleep` 调用者均正确且引用对应；B1064 完整 accepted evidence 视图未再发生跨定义代次借证。该轮平台 aggregate 改为 5 个 API 文本成员，仅 `cmd_sleep` 触发 composite support，故是无回归正证而非三平台 composite 分支的完整生产 pin。可见表头仍是“项目/列2/列3/列4”，且末尾泛化“证据支持稍弱”与实际证据强度不匹配，展示质量记 partial。 |
| 2 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260818-040325 | answer_regex,answer_contains | none | 193s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 的跨行 regex 仍是假阴性：终稿确有配置与 Mermaid 两节。B1067 已获生产正证：653/678 不再重定位合并，Explorer 从 545s/27 轮降到 193s/7 轮且无 completion 死循环。答案本身仍失败：把 `cmd/root.go:198` 的注释误称为 `merge` 函数，未解释真实加载/覆盖链；已定位到 `RenderMermaidBlocks` 却未继续读实现，错误称降级逻辑未定位。深层原因是 probe 的 accepted completion 被系统当作全 DAG 完成，两个 typed sub-topic evidence 节点未调度。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. 两案均无畸形 JSON、旧稿恢复、空答案、Finalizer reject 或活跃流固定 4ms 降级。组合案耗时下降证明 B1067 已闭环，剩余缺证不是旧循环的尾效应。
2. 新确认 `B1068-MULTITOPICPROBECOMPLETIONSCOPE1=P1`。Analyzer 已用 `SubTopics` 产生两个独立调查单元，编译器也生成 `probe -> evidence_t0/evidence_t1 -> validate -> reconcile`；但首个 ready window 只有 probe。Explorer 在 probe 内提前做了部分两题调查并成功调用 `emit_investigation_complete` 后，默认 soft policy 在后续循环直接 auto-complete 3 个 explore 节点、1 个 validate 节点和 reconcile，两个主题 evidence 节点从未得到自己的 dispatch。
3. 这不是模型波动：日志同时证明模型知道配置链与 Mermaid 内部实现仍未读，系统却发送 close-ready 引导并接受全局结束。最优根修必须使用 `SubTopics + NodeEvidence + NodeExecStatus`，把“仍有必需主题 evidence 未执行”时的完成限定为当前窗口；不能扫描用户请求、模型 reason 或最终答案来猜是否完整。
4. Runner multiline oracle 继续记低优先 eval 债。产品修复不能靠强制标题、关键词或改答案来迎合该 regex。
5. 本轮没有 Trace 输入；显式窗 Trace 因果投影、自动补齐、链上-only 主因、背景 support-only、实际占用/规则可消双轴均未触碰。

## Decision

- `B1064=production-no-regression/three-platform-composite-branch-not-exercised`
- `B1067=production-closed-r681`
- `B1068=confirmed/P1/typed-DAG-scope-fix-next`
- `combo-multiline-oracle=false-negative/low-priority`
- `active-stream-fixed-4ms-degrade=forbidden/not-observed`
- `system-answer/conclusion/relation-authorship=none`
