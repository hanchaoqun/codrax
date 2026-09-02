# r1013 跨模式人工审计

- date: 2026-09-02T01:38:29Z
- sweep_start_ts: 20260901-183829
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h3_iofam_one_seat | PASS | eval/results/real_trace_h3_iofam_one_seat-20260901-183829 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 43 | read=2,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 核心 IO 三口径正确；枚举完整性/端点转抄/隐含请求求和范围错误；精确上下文已提供，留为模型整理残余。默认空根因旁路生产正证。 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260901-183829 | write_apply,write_patch_oracle | none | 205s | 27 | read=7,repo_map=2,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只改构造函数、保留原四项测试；真实目标行为运行通过。模型错误探针保留警告但未压过项目测试，独立复验通过。 |

## H3：事实、上下文和答案分开核对

- 工件：`.codrax/output/20260901-184107.768-83809.md` 及同名 `.root-causes.json`。
- 请求是有限 IO 指标对照，不是强制根因榜；不以没有因果投影判回归。默认旁路生成 schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active。
- 正确保留单请求 1.347ms、目标闭合等待 1.337ms、4 段 S 等待并集下界 4.384ms、调度标记 IO=0、全部 S 睡眠 70.338ms；没有拿 D=0 否定 S 等待。
- 正文把“目标可见请求 6 个”升级成“共发起 6 个”；41.329 request·ms 实际只统计全局隐藏的 190 个配对，并非 198 个全部配对；第一请求 13762.872568..13762.873915 被抄成 13762.872568..13763.024898。三处均不能据机器 PASS 放过。
- finalizer 日志 Requested Runtime Fact Authority 明确给出 bounded witness count、emitted=8/total=198/hidden=190、hidden sum 和第一请求准确端点；模型原始 emit 已含错值，发布没有改写。属既有 H3 服从性残余，不是解析丢事件或错误 IO 因果匹配。
- 模型 table 遗漏 columns，渲染中性“列 1/2/3”，没有隐藏的另一组模型表头遭删除；公开 schema 有 columns。s_sleep、completion_woke_issuer 等机器字段进入可见文案也违反已有软指导。均不通过系统代写或扫描正文追加硬拒来“修好”。

## dateutil：实际改动与执行证明

- 计划 `plan-1788313262159658000-83813`；apply ref `64ec6b2441edf2481e338501f6f1af890dcdd6eb`（仅评测隔离仓）。
- relativedelta.py 的 __init__ 先拒绝非整数 float，再对整数值 float 转 int；原 test_relativedelta.py 字节未改。模型没有为通过测试修改期望。
- post_apply_verify 包含原四个 assertion，项目命令 `python3 -m unittest "test_relativedelta.py" -v` 成功；changed_path_coverage=covered、capability=target_behavior，最终 complete/verified。
- 人工在实际保留工作树重跑四项测试全绿，不以源码 regex 命中冒充目标行为执行。
- 额外模型探针错误：构造 years=1,months=1 后仍期待 2020-02-15，实际应为 2021-02-15。该比较器没有独立既有测试权威；报告保留 observed_failure/model_authored_probe_comparator_unverified，继续原测试，未把错误模型 probe 变成强制业务修补门。终稿明确自然语言验收不等于逐条独立执行证明。

## 流式/JSON 与后续

- 活跃 Reader 字节、心跳、隐藏推理、分片帧和 4ms evaluator budget 的 llm/agent 定向回归通过；不因 4ms/4m 没有可见答案而降级。真实静默与调用方明确取消仍有边界。
- 这一批无已证确定性系统缺陷，无新正文硬门，无 fixture/oracle 降杆。H3 模型残余沿原账观察，下一批转异构关系与显式窗投影。统一账见 §123.1620。

## 审计原则

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
