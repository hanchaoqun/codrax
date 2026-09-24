# 受信测量绑定与写模式固定双例人工审计

- 开始：本地 2026-09-23 21:47:41；UTC 2026-09-24 04:47:41。
- 干净构建：`24d1e6805422`，构建时间 `2026-09-24T04:47:17Z`。
- 恰好 2 并行 × 1，单例预算 1800 秒；没有第三例，没有改写原始答案或追跑求绿。
- [机器汇总](parallel_selected_summary_hmc_measurement_binding_20260923.md)：机器 1/2；完整人工 **0/2**。确定性回归与正确子能力不倒签完整答案。
- 任务口径：79 唯一项 = 14 已交付 + 65 开放，重复 0。HMC-08.3 仍验收中；写模式归原 HMC-18.5 / 统一账本 §165。

| 用例 | 机器 | 人工 | 时长 | 主要结论 |
| --- | --- | --- | ---: | --- |
| `trace_query_io_inflight` | PASS | FAIL | 662s | 四组原生指标及实际受信表选择正确；窗口发起总体、歧义成员的自由文本解释错误 |
| `empty_python_module_apply` | FAIL | FAIL | 236s | 实现、修改范围、实际测试和交付身份通过；原生断言登记及行为证明没有闭合 |

## 1. IO：新绑定真实命中，不代表正文已经可信

证据目录：`eval/results/trace_query_io_inflight-20260923-214741/`。读取完整 `run-1.primary.md`、`run-1.answer-surfaces.json`、原始日志、`eval/fixtures/hmosperf_io_inflight/events.systrace` 和 `expected.json`，逐组核对。原始日志为 `run-1.logs/codrax-20260923-214751-000-33560.log`。

实际最终工具调用选择四个 `runtime_measurement` 的 `summary` 视图，ID 为 `trace_query:trace-query-result-8414713f.json#io_inflight:1..4`。不是模型手抄普通表，也不是只有单测命中。窗口为 `[1.000000,1.010000)` 秒：

| 家族 / 设备 / 操作 | 峰值请求数 | 全窗均值 | 忙碌 ms | 请求·ms | 完整配对数 | 窗内发起数 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| RQ / 8,0 / R | 2 | 1.4 | 10 | 14 | 4 | 6 |
| BIO / 8,0 / R | 1 | 1 | 10 | 10 | 1 | 1 |
| RQ / 8,0 / W | 1 | 0.6 | 6 | 6 | 2 | 2 |
| RQ / 8,1 / R | 1 | 0.4 | 4 | 4 | 1 | 1 |

四表数值、单位、物理来源和查询范围均正确。全配对族的 8 条统计不是跨层去重后的物理 IO 总数，表中也明确不能相加。请求驻留不等于线程等待，不直接授目标阻塞、响应影响或根因。原生 `members` / `timeline` 同样进入最终供给，但模型没有选择它们作为最终可见表。

### 1.1 明确不通过的事实与来源

1. 正文第 101 行称 RQ / 8,0 / R 的“窗口内 6 次发起”包含 `0.998000` 的请求。真实窗口内发起是源行 6/9/10/16/18/20；窗口前源行 3 不在这六次内。源行 3 → 11 是合法跨入请求，只有窗口内 4ms 驻留计量。正文后面的通用边界说明又说跨入请求不计窗内发起，形成内部矛盾。
2. 正文第 101/113 行称 `reader-41@1.0035` 和 `reader-41@1.008` 都因歧义排除。真正歧义是同 sector 9000 的源行 9 `reader-40@1.003` 与源行 10 `reader-41@1.0035`；源行 18 `reader-41@1.008` → 源行 23 `1.012` 是合法跨出请求，窗口内贡献 2ms。源行 20 `1.009` 才是无对应完成的另一次发起。
3. 内部字段 `unpaired_start`、`ambiguous_cohorts`、`pairing_suppressed`、`completion_woke_issuer` 仍进入用户解释；模型标题与系统表标签重复，同族长备注重复四次，英文内部统计说明占比偏高。归 HMC-16.4/16.5 展示与噪音治理，不把原文扫描替换当根本修复。

最终指令中已完整给出 RQ 四条接纳成员，预览遗漏为 0，包含 `0.998→1.004` 与 `1.008→1.012` 及各自 4ms/2ms 窗内贡献（日志 2761 附近）。因此不能把上述错误归因于本轮成员 handoff 缺失，也没有足够证据定为纯模型波动。

独立审计追溯：探索阶段在日志 1370/1379 错称 0.998 未配对，1975/1985 又错称 sector4000 未完成并重算错误值；两次探索完成被接纳。但日志 2237–2923 的最终初始上下文没有这些错误 aggregate label/value 或 reason，2688 还明确隔离 17 条预分析候选。不能据此断言探索错误直接污染最终阶段。本次两句精确错误最早出现在最终首次提交 2972，重试 3038 保留；3099 局部补丁只补 facet/title，没有制造或修正它们。

另有确定的系统教学歧义：生产表共用备注把“仅成功配对请求”写成整体范围，summary 却另列独立窗内发起次数；后置 arrivals 不等于 pair count 没有明确撤销前置总体限定。本批 live 后按指标分别说明统计对象，成员/时序也分别声明自己的总体。不宣称这就是本例错误的确定原因，不用修后的确定性回归倒签这份答案。

### 1.2 JSON、过程、图与旁路

- 7 次 TraceQuery，源码读取/目录映射/列表均为 0；2 次探索完成均被接纳，最终成文拒绝 1 次。最大上下文 82080/200000（41%），未证明为上下文容量不足。
- 初次成文把 blocks 编码为 JSON 字符串并带无效流程图；局部恢复保留合法显示附件，随后改成合法 blocks 并选中四张受信表。最终无关系图；本题未要求关系图，不把没有图单独算失败。
- 探索 `aggregate_facts` 曾编码为字符串；当前 schema 真实类型是数组，系统进行无损解码恢复。未发现此处 schema 与教学矛盾，不因一次误用再堆一套教学。
- 最终没有把在途数认定为设备瓶颈或链上根因；本夹具没有调度/唤醒证据。不能把 `completion_woke_issuer=false` 泛化成所有真实场景没有 IO 等待。
- 原生 HTML 载体含四张完整表及正确数值；本次核查标签/内容，不冒称完成浏览器视觉验收。保存文件为 `.codrax/output/20260923-215841.729-33560.md` 与 `.html`。
- 必选 `.root-causes.json` 已生成，131 字节，`schema_version=2`、空 `root_causes`、`reason_code=trace_root_cause_contract_not_active`。这是统计问题的诚实空旁路，不是文件缺失或根因选择失败。

## 2. 写模式：功能通过，证明不能误签

证据目录：`eval/results/empty_python_module_apply-20260923-214741/`。逐项读取两次计划、验证/最终报告、写入结果、交付身份、完整输出与日志，并比较实际提交差异及 fixture 字节。

### 2.1 已证通过的范围

- 种子提交 `7fdc2814504bbeba02298976e40ddccc35bd9c96` → 实施提交 `3edc0db3a45db2b0d7f42ff26d84e64ce40097d7`，只修改 `totals.py`。六行实现核心为 `return sum(values)`，满足任意精度整数、空输入与单次遍历输入。
- 原评测仓 HEAD 不变，隔离工作树 clean；README、两个既有测试文件与夹具字节相同，无配置、依赖改动。
- 原生 unittest 四项 `test_empty`、`test_large_integers`、`test_signed_integers`、`test_single_pass_iterable` 真实通过，末次日志 `…-49727.log:2875`，59.5ms、exit 0。报告五项是 4 native + 1 probe；`suite_continued` 不是另一次执行。
- 当前验证计划 `plan-1790225470088990000-49727`，源码来源计划 `plan-1790225357253402000-33575`；commit、patch effect、diff fingerprint、测试 SHA 一致。
- probe `target_execution.status=complete`，源码 SHA 与交付吻合，真实执行 `totals.py:6`。旧“补证计划没有当前源码执行身份”此次未复现，不能继续以旧原因解释本轮。
- `delivery.status=coherent`；最终 `completion.verdict=unverified`、`reason_code=verification_proof_incomplete`，最终答复诚实披露未完全验证。写例没有 Trace 合同，不套用 Trace 旁路要求。

### 2.2 未闭合原因与归属

1. **模型误写身份。** 初始声明用 pytest 风格；真实为 unittest，suite=`tests.test_totals.TotalTest`、assertion=`test_empty` 等。精确身份已在 `…-49727.log:1223–1226` 提供；补证仍写成 `python@unittest@.::tests.test_totals.TotalTest`，根目录前缀和分隔格式都错。不能记成“模型已正确更正，仅系统拒绝”。
2. **系统缺少受控只读补登记通道。** `emit_change_plan.go:385`、`emit_plan_skeleton.go:214` 拒绝无源码计划的现有测试声明，本轮日志 2428–2430 命中。删除该声明后才发出 probe-only 计划，却未替换旧错误登记。
3. **真实执行不冒充行为证明。** Python target observer 证明修改源码被执行；原生行为证明还需 exact candidate + invocation + suite/assertion。唯一硬要求 `empty-case` 因而缺合格见证。六条未闭合 ledger 是该合同与派生覆盖，不是六个独立代码故障。

控制器日志 3143 将模型 `all_verified` 调整为 `accept_unverified`，防误签有效。归原 HMC-18.5 / 统一账本 §165；这是模型误用与确定的系统恢复能力缺口共同命中，不归为纯模型波动。

下一片应由控制器发出精确只读登记权限，绑定 run/batch/root、当前一致交付及完整合同集合；消费实际测试字节读取凭证，原子登记现有断言后重新运行，以新 invocation/test SHA/交付身份验证。不得改源码或测试绕行，不以历史 PASS、相似名称、输出关键词、空测试或 skip 履行合同；持久化恢复仍须核验授权与身份。公开底座见 `write_native_binding_public_test.go`、`run_tests_existing_test_delivery_public_test.go`、`project_test_observation_existing_file_test.go`、`project_test_observation_nested_scope_test.go`、`run_tests_project_test_observation_test.go`。

## 3. 下一批排序与不变项

已修本批确定性总体说明歧义；随后提高参考仓大小、读写比例、IOPS/带宽（08.2）的优先级，复用受信绑定并接通必要的范围/量尺展示（16.4/16.5），设计见统一账本 §178.5。其次是长期影响写模式的只读登记（18.5），再推进完整调度成员/分布（08.4）、业务实例归属/比较范围（04.2）及安全 SQLite（17.7）。正文误述和展示债仍留账，不围绕两条错误无限堆提示或按原文强改。这是原 65 项的排序，不新增父项或虚减数量。

600/300/600 秒、活跃流保护、显式用户窗、因果投影、自动补齐、链上根因及 IO/业务线索保持。没有新增源码/输出关键词硬门，没有通过删除失败判据或替换原报告取得通过。
