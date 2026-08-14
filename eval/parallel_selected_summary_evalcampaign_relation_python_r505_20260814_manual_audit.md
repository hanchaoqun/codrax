# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T20:01:12Z
- sweep_start_ts: 20260814-130111
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260814-130112 | answer_regex,answer_contains | none | 170s | 25 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | B821 获生产正证：终稿一次保留 12 个 production implementer、文件位置和 12 条 `实现类型 -->|implements| LoopController`；方向与 exact anchors 一致，零成文拒绝/patch，也没有把 3 个测试实现混入主体。系统只给 copy-ready typed recipe，图与说明仍由模型提交。末段“接口实现声明”对 Go 隐式满足略显含混，但不改变成员、方向或结论，记为文案波动而不立硬门。 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260814-130112 | write_apply,write_patch_oracle | none | 209s | 24 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | B822-B825 均生效：controller 正常进入 plan/apply/verify，未再 length 截断，生产补丁修复 whole-float months/years，人工在 applied tree 跑原有 unittest 为 4/4 PASS；最终也诚实显示“未完全验证”。确定性新 GAP 是 probe 声明 `expects_baseline_failure=true`，系统只运行改后 probe 并跳过项目 suite，随后又因 `verification_probe_baseline_not_run` 把同一通过结果降为 `proof_weak`。此外计划 edit 留下先赋原值、再赋归一化值的冗余 `self.years` 行，功能无误但属模型 patch 质量波动，不做 Python/行文硬拟合。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generalized gaps and closure

1. `B821-TYPEDRELATIONLOOKUPTODISPLAYDIRECTION1`：r505 生产闭环。typed lookup 与显示方向分离后，
   模型首稿即生成全部同向真边，strict validator 零拒绝；15 语言共用载体，无 case/language 特判。
2. `B826-BOUNDEDPROBEBASELINEDIFFERENTIAL1/P0`：`expects_baseline_failure` 是 typed proof obligation，
   但默认关闭全量 baseline capture 时没有任何执行者；改后 probe 通过会先跳过 suite，再被同一 obligation
   降为 weak，形成“必须有、又不提供”的矛盾合同。最优修复不是忽略 obligation 或强制双跑全量 suite，
   而是在不可变 main snapshot 上运行同一有界 probe：只有 main 失败且 active worktree 通过才铸造
   differential authority；main 也通过或 baseline 不可用均保持 typed weak/unavailable。
3. B826 实现不读取 request、模型草稿或最终答案；不从失败文本猜 defect。只有 schema-valid
   `ExpectsBaselineFailure`、两个明确 repo root、结构化 runner status 与 before/after verdict 参与判定。
   项目套件选择、现有 proof 门和模型对 plan/patch/final report 的作者权均不放宽。

## Invariants

- 活跃流没有固定 4ms/4m 年龄降级；r505 两案均由正常 tool/finish 路径结束。
- Trace 显式时间窗、因果投影、系统自动补齐、typed on-chain-only 主因、背景 support-only 均未改动。
- 系统不扫描答案关键词，不代写模型图、计划、补丁或结论；只提供 typed recipe 与 typed proof receipt。
