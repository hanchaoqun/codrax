# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T23:16:29Z
- sweep_start_ts: 20260802-161628
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260802-161629 | log_regex,answer_regex | none | 44s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 首轮组合 awk 错误被模型识别；续轮三条独立 sysctl 均 exit=0，最终 18/18 core、137438953472 B=128 GiB、M5 Max 40-core 与原始结果一致。坏结果未取得最终权限。 |
| 1 | read_combo_log_current_code_dimensions | PASS | eval/results/read_combo_log_current_code_dimensions-20260802-161629 | log_attachment,answer_regex | log_triage | 177s | 38 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=1,prune=0 | fail | value semantics 已阻止“4 个模型/4 次重试”；但答案仍把相邻的 attempt、renderer lifecycle 串成驱动/重置/回起点，typed transition witness 缺席。日志行引用还发生 2→3、3→4 错位，部分 source 机制被升级成该历史事件已走过的链。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### `operation_system_inventory`: human PASS

1. 路由为 typed `operation/computer_operation`，没有误入 repo/read/write。
2. 第一轮 CPU/内存组合命令的 awk 逐行消费三个 sysctl 值，产出 3 组错误的
   `logical=0/memory=0.0`。command-result context 保留 exact command、exit code 和原始
   preview；模型没有因为 exit=0 就把错误值当最终真值，而是明确判定解析失败并发出续轮。
3. 第二轮 `/usr/sbin/sysctl -n hw.physicalcpu|hw.logicalcpu|hw.memsize` 分别得到
   `18 / 18 / 137438953472`。最终 128 GB 为精确二进制换算；GPU/显示信息来自成功的
   `system_profiler SPDisplaysDataType -detailLevel mini`，与 payload 一致。
4. 上下文足够且权限正确：失败的派生值、成功的原始值和补采理由都可见。未发现 production
   GAP；首轮 planner 生成脆弱 awk 属模型规划质量波动，系统的续轮纠错已覆盖，不值得硬化命令模板。

### `read_combo_log_current_code_dimensions`: runner PASS / human FAIL

1. `value_kind=stage_ordinal` 与 `attempt_ordinal` 在 Log Triager 前、structured bundle、
   Explorer context 和 compact ledger 全部可见；两个 `log:protocol:N` rows 在 94 条记录压力下
   保留。Log Triager 已正确称 `4/4` 为 finalizer stage position，最终也不再声称 4 个模型或
   4 次重试。`EVAL-B47-SEMCAL1` 的目标因此通过真实回放。
2. 但两行都只有 `transition_authority=event_local_only`。模型仍从日志邻接推出
   “timeout 驱动 renderer retry → stage 重新进入 → attempt 重置/推进 → pipeline 回到起点”，
   并把当前源码中的 retry predicates/EventAdapterRetry 当成该历史运行已经经过的完整链。
   附件没有 correlation/transition witness，当前源码也未与 exact runtime event identity/version
   联结；最多只能陈述两个事件按行序被观察到，再解释当前代码中可能的 retry 机制。
3. 最终 Hop 1 用 `runtime_artifact_log:3` 引用原始 line 2，Hop 2 用 line 4 引用原始 line 3；
   这是引用选择/回填的独立错位观察，本批不以行字面特例修复，登记 watch 后由后续异构日志
   验证是否泛化复现。
4. context 数量不是问题：峰值 76,297 tokens（38%）且已读取当前源码。缺的是显式 typed
   relation caliber，导致模型将“时序相邻”升级成“因果跳转已证”。

### GAP and disposition

- `EVAL-B47-RELSEM1/P1`：精确事件值存在，但跨事件 relation authority 没有独立、醒目的
  typed carrier。方案是仅从 producer-owned OperationalSemantics 生成
  `observed_log_line_order_only / cross_event_transition=unproven /
  typed_transition_witness=absent`，贯穿 pre-triage、structured context 与 compact ledger。
  它是 evidence/guidance fence，不扫描用户输入或模型答案，不拒绝、不删除、不替换结论。
- `EVAL-B47-LOGCITE1/P2-watch`：runtime artifact 行引用发生稳定偏移的可能性；本轮只立案观察，
  待不同日志 topology 复现后再定位 compiler/citation join，避免按 4 行 fixture 过拟合。
- Trace 显式时间窗、因果投影、系统自动补采/补齐代码路径未修改。
