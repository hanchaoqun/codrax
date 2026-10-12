# HMC §226：无窗双侧入口与原生时序展示人工审计

固定构建 `b78918cc915a`，2026-10-11 20:45:55 开始；两例各一次、并行 2，单例上限 1200 秒。runner 正式 exit 0 不代表两例全绿。未追加第三例或重跑，问题与四份原始 fixture 哈希均保持不变。后续同批确定性补救不能倒签这两份答案。

| 用例 | 机器 | 完整人工 | 主要结果 |
| --- | --- | --- | --- |
| `trace_dual_measurement_unbounded_partial` | PASS，103s | **FAIL** | 无窗双侧声明、真实准备、坏侧隔离和健康侧取数均命中；最终手抄表漏未知时间记录且损失数值精度 |
| `trace_dual_source_window_timeline` | FAIL，外层415s（子进程413s） | **FAIL** | 真正建立逐侧窗口并取到正确时序；修图报错索引错误导致反复修错项，最终降级出厂的旧稿仍有错时间、单位及比例 |

## 1. 无窗双侧原始量测

原件目录：`eval/results/hmc_native_timeline_20261011/trace_dual_measurement_unbounded_partial-20261011-204555/`；进程日志 `run-1.logs/codrax-20261011-204557-000-80343.log`；工具原件 `.codrax/blob/20261011-204557-000-80343/`。

- 有效 `emit_analysis` 包含无端点的 baseline/current comparison 和合法原文锚点，后续真实查询空基线失败、健康 current 成功。日志约 851/859/920 行分别为失败侧、健康 measurements、补充 event_search 结果。上批“comparison 丢失后坏侧中止整轮”的自然入口失败本次未复现；不能因此签整个 10.1 父能力。
- `trace-query-result-45433835.json` 含 9 条有时间量测，`unpositioned_rows=1`；`trace-query-result-08e97796.json` 的原始事件保留全部 10 条。未知时间行原 RowID=9、filter=10、原值整数 55、时间 SQL NULL、原持续 50000000ns。finalizer 日志约 1678 行的 event inventory `display_rows[0]` 已有可读 `plugin.value=55`、`source.start_ns` unavailable/null、`source_time_known=false`。**不是输入/工具或整个事实交接丢失该字段**；原生 measurements 成员表尚未承载这个无法定位的补充记录。
- 最终接受的 `run-1.principal.md:3` 声称 10 条，表却只有 9 条；第 7 行 filter10 计 5（原始为 6），漏 55。第 20 行把 `250000000.25` 改写为 `2.5×10⁸`；第 22 行把 `9007199254740995` 改写为 `9.007×10¹⁵`，不再是精确原值。最终 `emit_answer_document` 只有 summary，没有原生选择器。
- 当前 RequestModel 已有 `scope=bounded_fact_set` 与 `fact_families=[other_observed_value]`，但没有可选 `requested_answer_dimensions`，旧默认展示入口因此关闭。这是共享适用性接缝，不应让模型或用户为同一事实域重复声明。原生表与逐侧获取状态未显示，手写副本继而失真。
- 保留的正确部分：四个 filter 没有按同名合并，NULL 与文本 `"0"` 区分，最后 4.5s 起点的原始 50ms 未被改为零，未知单位没有猜成 Hz，基线为空并准备失败如实报告。9 条已定位总体、1 条未定位补充总体不能混称某个完整时间窗内的 10 条。

同批最小补救挂 10.1/16.4/18.4：已有 typed 有限事实域可触发原生默认展示，明确独立维度仍优先；将已返回、合法但无法定位的原始量测单列 native members 表，保 RowID/filter/精确存储类与原值，不给时间、成员总体或因果权限。显式请求窗内不能把扫描范围当作该记录发生时间。测试通过也不改变本例人工 FAIL。

## 2. 双侧独立时间窗与时序图

原件目录：`eval/results/hmc_native_timeline_20261011/trace_dual_source_window_timeline-20261011-204555/`；日志 `run-1.logs/codrax-20261011-204557-000-80344.log`；工具原件 `.codrax/blob/20261011-204557-000-80344/`；实际出厂报告 `.codrax/output/20261011-205248.600-80344.md`。

- 第 5 次 `emit_analysis` 最终接受真实 comparison，params `tool-call_function_s55xnff3l1bi_1-emit_analysis-params-f1349a96.json`、result `...-cbaef6ab.txt`。此前引用、重复 TID 角色等拒绝未隐藏。与上批只走 pooled windows 不同，本次逐源通路确实命中。
- 工具取数正确：基线 waker201@1.002 → 上CPU1.006，可运行等待 4ms；当前 waker301@4.004 → 上CPU4.012，可运行等待 8ms。窗口内运行分别 4/8ms，睡眠 2/4ms。专用 CPU 结果 `f57ff5d7`/`64088361` 与线程结果 `d1f31ef2`/`3d53aaf2` 保留各自来源和窗口；频率各两段，基线 1GHz→2GHz@1.005，当前 1.5GHz→750MHz@4.010；两侧 idle 均为 60%。对侧窗口干扰的 waker701/801 未混入主答案。
- 本批新线程区间、频率区间原生表已在 finalizer 交接中准确发布。唤醒事件、实际上 CPU 时间和完整频率区间不是工具无数据；结束见证/选窗边界也没有被改写成额外事件。图端点显示名称补救不能替非法关系背书。
- 第 7–9 次 patch 出现相互反向提示，实际是 `diagram_participant_edits` 先拆 visibility 与 orphan 两个 slice 后使用局部索引：真实坏项是原数组 `[1]` 的 `retain_as_context` 缺 `block_id/participant_id`，错误却指向 `[0]` 的 `ensure_visible`。模型按错误位置增加字段后，又收到 ensure_visible 应删除这些字段的正确形状提示，于是修错项循环。确定性根因是**错误坐标丢失**，不是两个 action 的 schema 本身同时要求互斥字段，也不能定性为模型波动。
- 共 10 次拒绝、9 次 patch，未产生有效接受稿，最终走降级恢复首稿。机器据 `degraded_answer_checks_skipped:1` 判 FAIL。报告第 24 行误称基线 idle 40%；第 30 行把 750MHz 写成 750kHz，并把整个后半段说成 idle；第 34 行误称两侧运行相同 4ms、idle 40%→60%；图第 47 行把基线上CPU写成 4.006s（应 1.006s）。这些不是仅存在于已丢弃中间稿，而是实际出厂的正文/图错误。
- 报告第 67–68 行诚实披露未通过结构化校验，但“本次无适用板块”也说明原生时序表没有进入恢复稿；追加事实对照只是附带频率摘要，不能替代完整频率驻留区间或纠正文中结论。正文部分正确时间及最后的不可直接比较说明不能抵消上述错误。

同批先修通用 participant 编辑坐标，在混合动作中保原始索引，公开负控验证模型按诊断定位真正坏项。降级出口同时复用正常出口的原生默认表和当前受信凭证，保降级状态，不把草稿升级为接受稿。未通过关系校验的图载体仍可能进入恢复展示，这一独立接缝保 16.4/12.5/18.4 高优先级留案；本批不为单位、比例或时间误述新增关键词扫描硬门，也不增加第三次 live 求绿。

## 3. 记账与验收边界

仍为 **79 个唯一任务 = 20 个完整能力已交付 + 59 个开放任务**，重复 ID 为 0。本批完整父项新增 0；原生线程/频率时序与可读图端点、同批有限事实默认展示/未定位记录/诊断坐标均按子能力记录。五个稳定验收父项 03.2/04.2/08.3/08.4/18.2 与本批两份 FAIL 交叉记账，不相加，也不因输入已改善销掉成文失败。无写例、无只读登记自然分支，不能补签相关历史 FAIL。完整验证、发布及失败原件归档见统一账本 §226。
