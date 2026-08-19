# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T22:58:59Z
- sweep_start_ts: 20260819-155858
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-155859 | log_regex,write_apply,answer_regex,answer_contains | none | 520s | 26 | read=7,repo_map=2,list=1,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | B1193 的精确 observation 文件已进入 pytest 队列，说明执行接线生效；但 pytest 不可用后，runner_missing escalation 切到 `python3 -m unittest discover -v`，丢失 `tests/test_tokenizer.py` 精确 suite，仍无法铸 assertion receipt。planner 还把 assertion_suite/id 写成方法拼接与源码断言表达式，暴露 JSON 字段教学歧义。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-155859 | answer_regex,answer_contains,mermaid_edge_count | none | 536s | 45 | read=17,repo_map=3,list=0,trace=0,source_lens=1 | midloop=10,inv=5/0,fin_reject=4,unavail=0,prune=2 | fail | Runner 只检查有图/有边而假绿。终稿把同一条 `o.busCtx -> ctxbuilder.BuildAgentContext` argument_flow typed tuple 复制授权为 `BC -> A/Ev/X/F` 四条不同可见边；唯一来源只是 extract_work.go:15 的 extractor 构造。Mutable 仍孤立并诚实标 unproven。B1192 的精确 retarget 修补分支未在本轮最终病灶触发，不能记生产闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

1. **B1193 获得半条生产正证**：`ProjectTestObservations.test_path` 确实产生了
   `python3 -m pytest tests/test_tokenizer.py ...`，不再是“声明后永不执行”。失败点已经后移到
   runner/framework 降级：pytest 缺失后创建的 unittest candidate 没有继承 typed suite，退化成
   `discover -v`。这是 B1194/P0，必须在 candidate substitution 时保持 exact target，不得从计划
   prose、测试名或输出文本猜测。
2. **读案是结构性假绿**：一条 exact argument-flow 证据被四个不同 Mermaid endpoint pair 复用，
   validator 逐条都能在证据池中找到相同 tuple，因此错误放行。应新增 typed relation tuple 与可见
   edge-pair 的单图基数约束；相同 `(relation_kind, from_identity, to_identity)` 不得映射到多个不同
   body endpoint pair。该门只读 schema 字段和 Mermaid 结构，不读取 request/label/final prose，且
   Trace runtime diagram 继续走独立 authority。
3. **B1192 不能虚报生产闭环**：已有实现与全工具测试通过，但 r746 最终失败形不是
   endpoint-retarget guidance 命中形；维持 `implemented/pending-production-trigger`。
4. **B1195/P1 JSON 教学歧义**：`assertion_id` 必须是 runner 报告的 test function/method identity，
   不能是源码 assertion expression；`assertion_suite` 是 containing suite/class/module identity，不能把
   test method 拼在 suite 后。只改 schema 教学与示例，不按语言/测试名加硬门。
5. 第三个 source-free proof follow-up 被要求删除 `project_test_observations`，随后累计 observation 义务
   从计划消失。先审计 controller-owned cumulative scope 是否应跨 proof-only sentinel 保留，再决定
   是否立 B1196；本记录不先行宣称结论。
6. 两案都未出现 4 分钟固定总龄降级。读案持续活跃生成约 536s 后仍完成；结束权仍只属于 caller
   cancel/deadline、无首字节、byte-stall、transport/decode failure。

## 修复批次

- **R746-A / P0**：阻止一条 typed relation tuple 在同一图内授权多个不同可见 endpoint pair；补单边、
  多边复制、真正不同 typed tuple 正负 pin。系统只拒绝矛盾关系，不画边、不改答案。
- **R746-B / P0**：runner_missing/framework escalation 继承 exact typed observation/impact target；歧义时
  fail-open 为非精确候选但不得铸 assertion receipt。
- **R746-C / P1**：澄清 assertion suite/id JSON 教学，并审计 proof-only cumulative obligation 持久性。
