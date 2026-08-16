# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T15:18:41Z
- sweep_start_ts: 20260816-081839
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260816-081841 | write_apply,answer_regex | none | 160s | 25 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（代码）；验证边界诚实 | `_prefault` 改为存在性判断，false/0/空串回归齐全；PATH 无 Node，只有 source_static，因此 runner 的 unverified 是验证能力边界，不是修复失败。 |
| 1 | read_combo_answer_document_tools | FAIL | eval/results/read_combo_answer_document_tools-20260816-081841 | answer_regex,answer_contains | none | 980s | 57 | read=23,repo_map=1,list=0,trace=0,source_lens=0 | midloop=28,inv=9/0,fin_reject=15,unavail=0,prune=0 | fail | 最终把 `<2` 分支方向写反；关系图退化为零边节点表。emit 层接受精确 unproven 边界，外层又要求至少一条边，形成确定性合同冲突并触发回 Explore。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### github_issue_zod_prefault

1. 实际补丁正确且范围受控：`finalizeDefault` 从值真值判断改为
   `"_prefault" in result.schema`；测试分别覆盖 `false`、`0`、空字符串，保留已有 default
   不被覆盖的语义。
2. `make check` 通过，但报告对两个 TypeScript production/test path 的能力均为
   `source_static`。本机 `node` 不在 PATH，系统无法执行 JavaScript 行为探针；Controller 虽仍选择
   `all_verified`，确定性 normalizer 正确降为 `accept_unverified`，没有把静态证明冒充行为证明。
3. runner FAIL=`production_verification_source_static_only` 是诚实验证边界。次级 P2 是最终披露只说
   “本地验证能力或证明覆盖不完整”，没有把 exact unavailable runtime（Node）带到客户可见面；不应为
   eval 降低验证杆，也不应使用其他语言脚本伪装目标运行时。

### read_combo_answer_document_tools

1. B912 只在 Explore 侧部分生效：关系缺口已经诚实收敛为“两个参与者各有局部事实但缺少连接分量”，
   没再把独立 `Name()`/局部调用当作二者关系；但 Finalizer 后置合同把任务重新打回 Explore，导致
   37 个 Explorer iteration、17 个 Finalizer iteration、15 次成文拒绝、978s。
2. 确认 B913（P0 合同冲突）：第 4 轮成文工具已接受一个零边 Mermaid 图，两个
   `incident_required` 参与者均为可见断开节点且各有精确 `status=unproven` 边界；外层
   `required_diagram_edge_absent` 随即要求至少一条结构边。此时任何补边又会被 typed relation gate
   以无证据拒绝，形成“必须画边 / 禁止虚构边”的确定性循环，不是模型波动。
3. B913 根修采用同一 structured shape：只有零边图中全部 typed incident participant 均精确可见、
   每个恰有一个 `unproven` row、没有额外 row，才不再触发通用零边 violation。真实 typed relation
   是否已经存在仍由后续 evidence-aware participant validator 判定；stale/partial/invisible/unknown
   boundary 全部继续拒绝。没有扫描用户、模型思考或答案原文。
4. 确认 B914（P1 跨语言关系表达）：现有载体能表达 unary guard
   `enclosing callable -> condition`，不能表达 AST 已证明的 condition/branch 到受控 append/drop/return/
   tool-selection effect。模型要回答路由逻辑时只能在“画出业务正确但无 typed 权威的边”和“删除全部边”
   之间选择。下一批应审计所有支持语言（含 ArkTS/Cangjie）的 parser-owned control-flow/guard-effect
   carrier，复用 AST 支配/分支归属，而不是给本案例或两个工具名加例外。
5. 最终答案还把 `emitFullDocFailStreak < 2` 写成选择 patch；真实代码在 `<2` 时原样返回 schemas，
   `>=2 + patch base + patch schema present` 才过滤 full。该事实错误与 B914 的分支结果证据缺口同源，
   不能用答案关键词硬纠正或由系统代写结论。runner 的 `missing:retry` 只是次级词形 oracle，不代表
   人工正确。

## 不变量复核

- 无畸形 JSON、空答案或旧稿恢复；活跃字节流未因 4ms/fixed age 降级。
- 本批不修改 Trace 查询、显式时间窗、因果投影、自动补齐或根因选举。Trace 主因仍只能来自 typed
  on-chain 证据；邻近/背景只作支持；实际占用/业务线索与规则计价可消除量双轴不变。
- 系统没有生成、选择、补画关系，也没有修改模型结论；修复只协调两个结构校验层对同一
  model-authored typed boundary 的解释。
