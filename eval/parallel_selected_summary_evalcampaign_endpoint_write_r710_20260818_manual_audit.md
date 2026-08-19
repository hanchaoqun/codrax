# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T00:42:04Z
- sweep_start_ts: 20260818-174202
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-174204 | answer_regex,answer_contains,mermaid_edge_count | none | 396s | 49 | read=24,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=8/0,fin_reject=2,unavail=0,prune=1 | fail | 四阶段 precedence 正确，但题目要求的 BusContext/Mutable 与各阶段数据流未画出；两次 typed 完备性拒绝后，终稿退化为 `BusContext -> BuildAgentContext -> bus.Mutable.Objective`，该局部方法调用不能回答共享状态如何流经四阶段。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-174204 | log_regex,write_apply,answer_regex,answer_contains | none | 618s | 25 | read=10,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | 最终 `make check` 真实通过，但模型把基线回归 oracle 从五换行必须折叠为单个 `[300]` 改成 `[300,300,10]`；实现与被改写测试自洽，却违反用户要求。首次 verify 正确报红，replan 错把修改测试当修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Read / diagram

- B1119 生效：探索从 r709 的 995s、53 dispatch、全函数兄弟调用扩散降至 396s、2 dispatch；未再出现参数短名冒充执行的旧形。
- B1120 的端点侧约束没有被绕过：终稿 `o.busCtx -> BuildAgentContext` 与 `BuildAgentContext -> bus.Mutable.Objective` 的可见方向未把另一 requested participant 冒名映射到非 incident 端点。
- 新 gap 是 typed 关系载体自身不足。`BuildAgentContext` 的 keyed composite literal `Mutable: bus.Mutable` 已产生 `assignment_fact`，但目标端点仅为裸 `Mutable`，丢失构造对象 `AgentContext.Mutable`；relation capsule 因而把它分在独立组件，无法桥接到 agent 阶段。
- participant repair candidate 固定截前 3 条时，Mutable 收到的都是 `Objective()`、`EmittedAnswerSymbols()` 等局部 call，未优先给出与 `AxisFlow` 更匹配的 assignment/data-flow。硬门随后要求“已有 typed incident 必须画一条”，模型只好选择语义较弱的 Objective 调用，删掉第一稿中更接近问题、但未获 typed anchor 的阶段到 Mutable 数据边。
- 最优根修不是放宽图证据门，也不是系统代画：先保真 keyed/composite initializer 的接收者身份，再按请求 relation axis 对已证候选做稳定优先级排序；模型仍选择是否画图和如何下结论。

### Write / verification

- write analyzer 把明确的“existing five-newline odd-run regression input is intentional; do not reduce or remove it”改写成“输入保留、期望值按实际行为更新”，没有发出已有 `preserve_regression_test target=tests/test_tokenizer.py` typed constraint。B1115 下游 critic 因此没有保护 authority，首版计划在 apply 前未被拦截。
- 基线测试的完整 oracle 是五个换行对应单个 rank token `[300]`，与用户的“a consecutive run ... collapse to one rank token”一致。首计划改成 `[300,10]` 后真实 `make check` 报 `[...300,300,10] != [...300,10]`；第二计划继续把期望改成 `[...300,300,10]`，最终项目套件虽绿但属于 self-fulfilling verification。
- B1117 的 typed handoff 正确保留 `Lists differ` 左右操作数为 unlabeled；然而 Controller/Planner 的自由推理仍多次把两侧口头标成 expected/actual，证明该提示只能防系统伪铸角色，不能替模型决定正确 oracle。
- B1118 的最终安全目标得到生产正证：replan 后确实再次执行 `make check`，旧失败没有被局部静态检查清空；但本轮没有“局部 verification probe 先通过”的前提，因此不是 `suite_continued reason=cumulative_verification_scope` 精确分支正证。
- Make composite parser 将嵌套 unittest 输出压成 `make-test`，FailureDetail 只保留 `AssertionError: Lists differ...`，丢了 traceback file:line；因此既有 `preserve_failed_test_assertion` 后置保护也无法定位测试文件。这是独立 P1 诊断保真 gap，但即使补回也只能保护首计划修改后的 `[300,10]`，不能替代分析阶段的原始回归保护。
- Planner 曾一次把 `changes[]` 发成 JSON 字符串，JSON-shape-first 合同 fail-loud 后下一轮恢复 native array；没有空答案或降级，暂记单次模型载体波动，不以 case 专门兼容。

### Priority

1. **P0 B1121**：加强 write-analysis typed carrier 教学/pin：用户声明现有回归测试、输入或 fixture 为 intentional/must-keep 时，必须发 exact-path `preserve_regression_test`，且不得在 expected outcome 中擅自授权改 oracle。
2. **P1 B1122**：保留 Make 包裹的下游测试失败位置/断言身份，避免 composite row 把已存在的 post-failure test-contract critic 置盲。
3. **P1 B1123**：复合字面量/keyed initializer 的赋值目标携带构造对象/声明类型身份，形成 `bus.Mutable -> AgentContext.Mutable` 这类真正 data-flow endpoint。
4. **P1 B1124**：participant candidate 对 flow/assignment/call 等按最终 typed relation axis 稳定排序；只改变候选 guidance 顺序，不创造或选择关系。

Active stream 未观察到固定 4ms 降级。该批不涉及 Trace 查询、显式窗、因果投影、自动补齐、链上-only 主因或实际占时/规则可消双轴。
