# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T04:22:45Z
- sweep_start_ts: 20260828-212244
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | logtri_oversized | PASS | eval/results/logtri_oversized-20260828-212245 | log_attachment | log_triage | 166s | 26 | read=8,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=5/0,fin_reject=0,unavail=1,prune=0 | partial | 附件内发出点和调用者定位正确，但可选的当前仓验证被错误升级为全局“证据不足”裁决，答案自相矛盾。 |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260828-212245 | typed_inventory_rowset,dimension_substring,answer_contains | none | 448s | 37 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=7,inv=4/0,fin_reject=6,unavail=1,prune=0 | partial | 2/2/8 三组事实和引用均正确；系统因短路径证据未绑定唯一 typed 行而把 public class 错降为补充，连续拒绝 6 次。runner 还对合法列表形使用了表格字面 oracle。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 1. `logtri_oversized`

### 人工结论

- 附件事实正确：最深栈帧 `main.crashy()` 是 panic 直接发出点，`main.main()` 是调用者；最终答案引用了
  `eval/fixtures/oversized_log.txt:643` 与 `:645`，且诚实披露当前仓找不到对应实现。
- 主答案却同时写出“证据不足—无法判断”。这不是附件证据不足，而只是“无法验证当前 checkout 是否仍含相同实现”；它不能抹掉已经由附件证明的历史发出点。
- runner PASS 只证明附件引用命中，未覆盖该语义自冲突，因此人工判定为 partial。

### 系统机制

1. typed 路由已经给出 `source=artifact`、`current_source_evidence_mode=optional`。
2. analyzer 仍发射 `DiagnosticProfile.CurrentVersionCheck=true`，把只问附件事实的任务升级成当前版本状态检查。
3. finalizer 据此强制 `current_status_verdict=still_present|fixed|not_enough_evidence`；模型只能选择
   `not_enough_evidence`，该局部状态又被渲染成全局“结论”。

这属于跨阶段 typed authority 未对齐，不是模型对日志事实的推理波动。

### B1438-ARTIFACTOPTIONALCURRENTSTATUS1（P1）

- 当 typed 路由明确为 artifact + current source optional，且不存在独立的 typed current-source explanation obligation 时，
  analyzer 不得单方面把 `CurrentVersionCheck` 升级为必答合同。
- 最优修向是在分析 IR 接纳处对 typed 信号做交叉归一：清除该错误升级并记录 warning；真正询问“当前是否仍存在”的任务由
  `current_source_evidence_mode=required` 或有效的 `CurrentSourceExplanationProfile` 保留当前状态合同。
- 禁止扫描用户原文、模型原文或最终答案；不得影响 artifact 内根因分析、Trace 窗口、链上根因和自动补齐。

## 2. `cangjie_repomap`

### 人工结论

- 模型实际找到并逐项引用了 `extend=2`、`foreign func=2`、`public class=8`，文件、行号与 package 均正确。
- runner 的 `missing_inventory_group` 还包含一个测试口径问题：它要求 `| extend ` / `| foreign func` 之类表格字面，而合法项目列表同样回答了问题。该 oracle 应修正，产品不得为表格字面过拟合。
- 更严重的是系统把 public class 错标为 `out_of_requested_universe`，连续 6 次拒绝正确 principal carrier，最终迫使模型把 8 个明确请求的类降成“补充”，并泄漏 `source inventory principal row set`、`source_inventory_row_id` 等内部术语。

### 系统机制

1. analyzer 的 typed 请求明确包含三组 `SourceQuotes=[extend 块, foreign func 声明, public class]`。
2. explorer 已提交三组精确模型 aggregate facts 和逐成员 support refs；public class 的 typed rows 也带正确
   `SurfaceFamily=public class`。
3. `sourceInventoryCanonicalizePrincipalFactMemberIdentities` 只接受完整路径精确相等。模型的结构化 support ref 使用
   `Bridge.cj:15` 等 basename+line，虽然在 typed row set 中可唯一解析，却未被绑定。
4. 投影因此只承认另一路已带完整身份的 4 行 synthetic principal set，并把 public class 等模型事实 shadow/demote；后续 requested-universe 又以这个不完整系统集合为准，形成自证循环。

### B1437-SHORTSUPPORTTYPEDROWIDENTITY1（P1，优先）

- 身份绑定顺序应为：完整路径精确匹配优先；失败后允许结构化短路径按路径段后缀 + 精确行号 + 兼容成员标签匹配 typed principal rows。
- 只有结果唯一（或重复行归一到同一 principal row key）才可绑定；同 basename、同行且存在多个不同 typed key 时继续 fail-closed，绝不猜测。
- 该修复只补隐藏 typed identity，不创建、删除或改写模型的成员集合、结论和分组；不使用语言名或单个 Cangjie case 分支。

## 3. 冻结顺序与不变量

1. 先修 B1437：它当前直接触发 6 次硬拒绝并错误改写 principal/audit 权限，ROI 最高。
2. 再修 B1438：保持附件事实为主答案，当前仓验证仅在 typed required 时成为必答裁决。
3. runner 表格字面 oracle 单独作为 eval 基建修复，禁止反向硬化产品输出格式。
4. 全批不改 Trace 显式窗、typed 自动补齐、因果投影、链上根因选举和双账户；背景证据仍仅为支持/排查方向。

## 4. 施工结果

- B1437 已实现：短 support path 只在 typed 路径段后缀、精确行号和双标签共同指向唯一 principal key 时升级为完整身份；歧义路径保持原字节并走既有 fail-closed。与 row-set 完全不相交的独立 typed 选择事实不再污染覆盖等式，相交 superset 仍拒绝。
- B1438 已实现：external-observation optional 路由下，孤立 `CurrentVersionCheck` 不能再生成 mandatory current-status verdict；共享 typed authority 证明仍有独立 current-source obligation 或路由为 required 时，旗标保持。
- Cangjie eval oracle 已删除表格管道 marker，列表与表格统一按 typed 行 token、所属 section 和精确基数验收；新增 2/2/8 heading-scoped list 回归。
- 验证：受影响包、source-inventory LOC 收敛钉、`eval/runner_lib_test.sh`、完整 `go test ./... -count=1`、CGO release-tag `make` 全部通过。等待从新提交构建不可变二进制做生产回放。
