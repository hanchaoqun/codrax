# IO 计时口径批次：固定双例人工审计

- 日期：2026-09-23；固定干净构建 revision：`e29e8e0e8e04`。
- 每例一次、恰好 2 并行；runner 15511 正式 exit 0，没有第三次追绿。
- 结果根：`eval/results/hmc_io_value_caliber_20260923`；机器原判保留在同名非 manual 文件中。
- 机器 2/2 PASS；完整人工 1/2 PASS。确定性回归与真实模型命中分开，不倒签任何历史 FAIL。

| 用例 | 机器 | 人工 | runner 耗时 | 结论边界 |
|---|---|---|---:|---|
| `empty_python_module_apply` | PASS | PASS（业务交付） | 191s | 仅实现文件变化，原生 4 个测试真实通过；不是原生断言只读补登记或新补证交付身份的验收 |
| `trace_query_business_marker_io_chain` | PASS | FAIL | 346s | 主链及 35/31/1ms 保留，但业务窗混账、相邻状态不可相加和缺证写成否定仍错误；新 IO 数值口径未命中 |

## 1. Trace：完整答案未通过

结果目录：`trace_query_business_marker_io_chain-20260923-010950`。审计覆盖 `run-1.principal.md` 全文、完整报告 `.codrax/output/20260923-011534.386-88705.md`（268 行）、图、旁路、原始夹具、工具过程及实际 Finalizer 上下文。过程日志为该目录 `run-1.logs/codrax-20260923-010952-000-88705.log`。runner 346s 与管线约 343s 是不同计时口径。

原始夹具 `eval/fixtures/hmosperf_business_io_chain/events.systrace`：

- OpenDocument：1.000000–1.050000，共 50ms；同一业务窗内 running 5ms、sleep 44ms、runnable 1ms。
- LoadDocumentIndex：1.004500–1.044500，共 40ms；同一业务窗内 running 8ms、sleep 31ms、runnable 1ms。
- 工作线程读请求：1.005000–1.040000，共 35ms；完成唤醒闭合的 S 态等待为 1.009010–1.040010，共 31ms；随后调度等待 1ms。
- 工作线程在 1.045000 唤醒主线程，后者在 1.046000 切入 CPU，另有 1ms 调度等待。后台写请求 47ms 没有独立的完成唤醒闭合证明，不能仅因耗时更长晋升主因。

人工失败依据：

1. principal 第 1/7–15 行把 50ms 业务窗与 0.999000–1.052000 的 53ms 查询窗混用：报运行 7ms、额外约 1ms 未归账，又说与业务窗同口径。实际附件范围为 0.999000–1.051000；更宽查询确有 1ms 未归账，但不能转移给完整 50ms 业务窗。
2. principal 第 23/25 行（完整报告第 38–40 行）说睡眠 44ms 与后续 runnable 1ms 因首尾相接而不能相加。两者是同线程同窗口互斥状态，离 CPU 总时长可计 45ms；仍不能把它们与覆盖这些等待的跨线程请求驻留重复相加。“总阻塞”是否包含 runnable 应先定义，但“首尾相接所以重复”本身错误。
3. principal 第 39 行将结束打点写作原始 `E|100|OpenDocument`，实际原文是 `E|100`。正文又将缺少后台唤醒闭合写成“未发出唤醒”，并把 35ms 请求耗时说成直接决定主线程全部阻塞；均超过已给证据。后台没有被选为主因，这一点保留。
4. 末次 patch 增加了正确的 5/44/1 与 8/31/1 业务条目，却未修正开头及时间分解的矛盾；不能只看补丁后的正确句子给整份答案 PASS。

上下文充分性：实际 Finalizer 日志 3827–3830 明确给两条业务窗完整状态账和不得替换宽窗的说明；3835–3848 同时给 35ms 请求、31ms 发起线程等待、缺少闭合不等于未唤醒。没有证据表明这部分被截断。核心数值误用不能归因于系统缺数据，也不能凭单次证明纯模型波动。

但同一上下文 3836 的系统句子 `Request residence, issuer blocking, scheduler delay, and cross-request aggregates are not additive.` 过于笼统，把本可按原生分区汇总的调度状态也包入禁加范围。它可能助长误解，列为可泛化的教学矛盾修复；不能声称它是全部答案错误的唯一原因。修复在本次 live 后进行，只用公开消息回归验收，不修改本次答案或 verdict。

图及结构：Trace 因果投影存在，采用合法文本树；方向为从目标向上游追溯，唤醒子行指向父行，没有新增反向因果边。链上业务、S 态 IO、调度等待和背景区别仍在；树中未获可排名资格的睡眠节点保持上下文。不把没有 Mermaid 当语法失败。系统图/附录仍有内部词面与较宽查询范围，继续留原父项。

新功能命中：共 17 次 trace_query，但没有 `root_cause_rank` 或 `blocking` 视图调用；本轮原生候选与投影中没有新增 `io_value_caliber`。因此不得把机器 PASS、旧 IO 双尺上下文或公开联测，宣称为新数值口径贯通的真实模型验收。

旁路 `.codrax/output/20260923-011534.386-88705.root-causes.json` 确已生成，schema_version=2、root_causes=[]、status=unavailable、reason_code=no_selectable_typed_on_chain_candidates。它诚实反映本次没有可选择的 typed 链上候选，没有从模型正文伪造根因；不能因为正文说有根因便强行补选，也不能把空旁路误记为文件生成失败。

## 2. 写模式：业务交付通过，证明增强仍开放

结果目录：`empty_python_module_apply-20260923-010950`。审计覆盖完整 run 输出、计划、报告、应用树、实际命令和测试源码。

- 交付 `5b92b40bf12fd01a4dc54f052b748f405bb55732`，基线 `01e26f4f17302618c663ff73ffbcbc2ff4e1da02`；只修改 `totals.py` 两行，定义 total 并返回 sum(values)。原测试、README 和配置保持不变，隔离工作树中交付，未自动合入评测主仓。
- apply 日志 `codrax-20260923-011212-000-89886.log` 第 674–678 行实际执行 `/usr/bin/python3` 对应的 `python3 -m unittest "tests/test_totals.py" -v`，exit 0。4 个测试方法、4 个原生断言全部通过；不是 7 个测试，也不是只有 probe 通过。report 第 4–40 行保留断言结果，第 83–95 行保留 native 命令；应用树的 `tests/test_totals.py:7–18` 是真实原生方法。
- 最终 run 输出第 175–191 行如实给工作树、4 条验证通过及自然语言验收清单不等于逐项独立证据的边界，因此业务交付人工 PASS。
- 旧空文件编辑教学真实命中：plan 日志 1330–1332 指向 patch/insert_at_eof，1350 修正、1362–1370 接受。随后控制器 1375 的路径-only 定位和 1563–1578 对零字节文件要求 enclosing owner 仍触发多余重规划，原空基线定位债不销。
- 最终计划 44–80 的 PTO 使用 pytest 风格身份，而实际 suite 是 `tests.test_totals.TotalTest`；82–114 合同均 planning_only_ungrounded，apply 日志 804 的 hard_required=0/planning_only=4。实际原生执行通过不证明这组 PTO/完整行为合同绑定已闭合。
- 本例是普通 apply，没有命中 source-free、只读 PTO 补登记、多来源 AppliedSources 或新补证身份路径。不能用这次 PASS 核销统一账本 §165 或 HMC-18.5 B2–B6。

## 3. 后续处理与范围

优先完成本批 IO 口径迁移、通用非重复计量教学及完整全仓验证；不追跑第三个 live。随后按统一账本 §165/166，优先原生断言只读登记及静态工具说明完成通道，再处理业务范围/补齐、caller 双轴、共享旁路来源与更广 IO 能力。所有旧人工失败保留；稳定任务数仍为 79 总项、14 已交付、65 开放。
