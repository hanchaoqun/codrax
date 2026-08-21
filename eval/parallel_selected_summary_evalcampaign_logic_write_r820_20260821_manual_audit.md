# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T17:40:47Z
- sweep_start_ts: 20260821-104046
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260821-104047 | write_plan,write_patch_oracle | none | 71s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划仅把 main.py:20 的 retrun 改为 return，kind=patch、精确 old_text、pending_approval；没有应用写入。Python import+greet 验证探针与三项验收一致，未扩大路径或行为。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-104047 | answer_regex,answer_contains,mermaid_edge_count | none | 871s | 65 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=19,inv=5/0,fin_reject=20,unavail=2,prune=0 | fail | 关系修补成功消费旧 lease 后只剩 BusContext participant coverage；系统要求复用精确 typed argument_flow candidate，却没有为该新代次安装 addition_ref。模型复用旧 ref 被执行器正确判 stale，连续 20 次 reject，最终只能降级显示旧草稿。B1303。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

1. 写模式为真实正例：仓库输入只有 `main.py`，ChangePlan 只有一条 `retrun -> return` 精确替换；状态为 `pending_approval`，本轮没有 apply、没有主仓改动。验证探针直接导入模块并断言 `greet("test") == "Hello, test!"`，与改动目标一致。
2. read 不是模型随机漏字段，而是确定性不可满足合同。第 3 轮原子 patch 成功删除未证边并加入三条 typed stage precedence，旧 relation lease 随成功事务被正确消费。随后 pre-emit 只报告 `BusContext / available_typed_incident_edge_not_rendered`，并发布精确 `o.busCtx -> ctxbuilder.BuildAgentContext / argument_flow` candidate；但该 participant-only reject 没有新的 relation failure，因此旧实现既不产生 addition-only relation delta，也不安装新 lease。
3. 模型第 4 轮提交了上一代出现过的 `addition_ref=ra1-9f678...`，第 17 轮又提交另一历史 `ra1-a7aa...`；执行器均正确按“not present in a live relation-repair lease”拒绝。问题在 producer/consumer 能力接线：提示要求添加、执行面却没有同代可用能力，不能通过放宽 stale-ref 校验或跳过 typed relation gate修复。
4. 最终 runner 的 `degraded_answer_checks_skipped:1` 与人工失败一致。系统只恢复了上一版草稿，明确写出“最终重试未能产出有效的 answer_document”；草稿图仍含 `Analyzer -> BusContext`、`BusContext -> Explorer/Extractor` 等未完成 typed 校验的关系，因此“有可读散文”不等于正式答案有效。
5. 次级过程观察：第二个 explorer dispatch 把累计证据扩到较宽集合，曾短暂采用测试文件证据；最终引用大体回到生产源码，但 27 个 explorer iteration、16 次 read 和 65% context 仍偏高。先作为异构重复观察，不与 B1303 混修，也不据单例增加轮数、耗时、4ms/4m 或上下文比例硬截断。

## B1303 施工与验收

- 参与者覆盖 producer 现在从同一个 typed candidate provider 同时生成 addition-only relation delta；只有精确 candidate、精确且唯一的现有 diagram carrier 都成立才发射。多图且 mismatch 未绑定 carrier 时不猜目标、不铸 ref。
- lease 支持 `failures=[] + allowed_additions!=[]` 的短生命周期形：模型仍选择 candidate 并自写可见 from/to node 与业务标签；ref 只补隐藏 relation kind、canonical identities 和 block id。既有边必须保持，未列第二条关系仍拒绝，空能力、缺失/歧义 carrier 和历史 ref 均 fail-closed。
- full/patch 两条 retry 路由均先将 producer candidate 按当前 patch base 重签，再把执行器实际持有的 `addition_ref` 放回联合 typed capsule；addition-only 教学不再要求不存在的 `failure_ref`。
- 单测覆盖 producer 同源、当前 ref 安装、原子执行并消费、未列关系拒绝、歧义多图不发能力、空/缺失 carrier 拒绝，以及混合 relation failure 旧车道不回归。相关三个包完整套件通过。

状态：

`r820=runner-pass-1/2,human-write-pass+read-system-fail`；
`B1303-PARTICIPANTADDITIONLEASE1=implemented/related-full-suite-pass/pending-production-replay`；
`participant-only-candidate=current-generation-addition-ref`；
`stale-ref=fail-closed`；`ambiguous-target=no-capability`；
`system-edge/action/visible-endpoint/wording/conclusion-selection=none`；
`request/model/final-prose/mermaid-message-fact-scan=none`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/unchanged`。
