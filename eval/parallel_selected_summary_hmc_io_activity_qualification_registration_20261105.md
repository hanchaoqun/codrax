# HMC356 — I/O 活动量尺候选、只读登记终态与冻结失败

2026-11-05 本地，`main` 基线 `f9c765cdc6bfe85056a1e2a0f7214afb2abf071d`。79 个唯一任务，29 完整、50 开放（17 部分实施、29 待实施、2 待验收、1 验收中、1 持续执行）。**本批完整能力交付 0、已交付子能力 0；1 组候选留在本地，未提交源码。** 只读正向有限验收是既有能力的收据，不新增完整任务或代签其它验收项。剩余缺陷见[分轨手审](parallel_selected_summary_hmc_io_activity_qualification_registration_20261105_manual_audit.md)和下方排序。

## 共享缺陷轨：原生缺测、成文交接和离线报告

公开实际 TraceQuery→Finalizer 旧码 RED：没有可用 D/IO 等待时长的背景 I/O 活动指数，显示 `d_state=0.000ms io_wait=0.000ms`。候选在生产者分开已计入的有限正值和 `not_accounted`，保极小正值不四舍五入为零；工具、直接显示和共享覆盖行共用字段限定。原始数值 wire、聚合、分数/排行、真零的独立证据、时间窗、配对及因果权限不变。没有新模型发射字段或问答文本硬门。覆盖摘要漏带 `ValueContext` 一并补齐，限定进入同一事实行，不另设可冲突的调查结论。

参考 `core/preprocess/io_ops.py:114/482` 的重叠桶与 SQL 起点桶分别给资源概览，不是连续请求驻留或设备排队；`uninterruptible_ops.py` 和 `uninterruptible_dstate.yaml` 以原生 `thread_state` 约束状态时长和 caller。借用的是独立统计对象与适用域，保现有成功配对半开区间、全窗分母、独立起点计数，不复制桶近似、异常阈值、缺值补零或把邻近背景升级成根因。

用户完整实采实际查询→Finalizer，零模型、非裁剪：网易云 82,838,676 B / 559,592 行（SHA `12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`），元宝 63,659,046 B / 412,546 行（SHA `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`）。与 HMC355 原生快照独立逐字段比对，保数值、状态行、原件和 Trace 因果投影：两件分别保 D/IO 等待 32.283/13.978 ms、23.414/15.079 ms，未计入的块/存储延迟不冒实测零。75 份用户 ftrace 未找到原 IO 问题所需的配对协议，不代表 IO 为零；A 用原 authored 夹具，独立整数端点 oracle 对原 expected 一致。[整件验证](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0260-codrax-hmc356-whole-real-v1-receipt.json)／[同源比较](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0252-codrax-hmc356-whole-compare-v1-receipt.json)。

终版相关 5 包 74 顶层/92 子例、race×2 4 包 28/70、原因果 4 包 22/141、双整件 1/2 通过；保原四根因、闭合睡眠和链上业务线索。早期新增测试选错可选行的失败保原，修测试范围；纠正 HMC355 测试先手写预算而被 Build 重置的问题，测试实际带预算的公开入口，不放松旧断言。参数捕获未运行真实 CLI，不计第三例。

[冻结](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0167-codrax-hmc356-frozen-v1.json) 7652 输入/142 外部/10 仓外快照，SHA `ce226977558168a6e1c23272c6f5869ec9175070665a2d28d5e3a9e6b08b0afa`。唯一全仓 UTC 01:21:23.072456–01:35:43.407927，OS1、源前后核验一致；97 包 PASS/12 无测试/1 FAIL，28781 顶层 PASS/58 SKIP/1 FAIL，34635 子例 PASS/14 SKIP，268758 纯 JSON、无重复/未终态。[精确统计](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0168-codrax-hmc356-full-stats-v1.json)。

`internal/tracediag::TestEvalcaseDHIO2ChurnEndToEnd` 明确仍要求 `d_state=3.859ms io_wait=0.000ms`。这是遗漏离线报告消费者的契约迁移，不是无关环境失败。下一代保真实 D 时长、438.195 指数、两种 0.936 ms、churn 2167、inode/线程/原始事件等旧针，显式校验未计入 I/O 等待、缺测限定和禁止裸零；不动本冻结，不做第二次同代全仓。

## 完整能力轨：两例真实并行，各一次

[请求配对](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0133-codrax-hmc356-actual-request-overlap-v1.json)：A6/B11 个完成的唯一请求，歧义 0，16 对真实请求重叠，最大 31.857 秒，不拿进程重叠代替。无第三付费例。

| 例 | 原机器 | 人工结论 |
| --- | --- | --- |
| A 原自然 `trace_io_inflight_overview`，n1/15 步 | FAIL，OS0 不等于通过 | 四组24数/8 配对/13 段和原生表通过，实际 INIT 收到缺测限定；完整解释 FAIL，08.3 保留 |
| B 原 `TestNativeRegistrationLiveNaturalControllerTerminal` | PASS，原精确终态行 | 独立恢复、新执行、原件与报告引用有限 PASS；非完整 CLI/登记矩阵 |

A 原生表正确，实际投递的调查 unit 0 混同窗内发起与重叠配对、越权说队列不排队；正文还混用裁窗驻留与完整请求耗时。未投的其它错误不算实际污染。原 regex 漏匹配释义，机器 FAIL 不改，人工仍有实质失败。原有合法同口径计算、独立统计和表格不禁用。HMC355 行引用只出现教学，未实际引用（6 定义/0 引用）。

B 真实父/恢复子 controller 与 planner，保两次合法拒绝后重试；现有 `test_value.ValueTest.test_increment` 通过（1 断言/0 失败），新凭证不同于历史及源验证。子进程源/测试/HEAD/Git 不变，当前、最终与恢复报告同字节，原始尝试和绑定各自保留。`all_batches_verified` 只表示本条正向合同，不能补签缺来源负向、旧未触发及 B2–B6。

## 剩余 50 项，按 ROI 排序

下表仅调整执行顺序，不改变[统一清单](../docs/design/hmosperf_implementation_tasks_20260917.md)的退出合同。ID 省略 HMC-。

| 优先 | 数量 | 全部开放 ID | 下一步 |
| --- | ---: | --- | --- |
| P0 共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先修报告消费者；同查询/统计对象/单位绑定贯穿调查、正文、表图，区分精确地址与解释权，复核预算与重复教学 |
| P1 具备材料 | 5 | 08.3、18.5、03.2、14.1、18.2 | 有新共享证据再验旧 IO，轮换既有只读登记正负终态与实材完整能力，不为充分证据的误述反复跑同题 |
| P2 可复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 实例/资源/帧/源钟/生命周期、二进制格式和独立分母；现存 DB 只读，活跃 WAL 仍禁止生产接入 |
| P3 协议/材料 | 21 | 09.3、04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 将 09.3 有实材 BRBE 子范围提前预审；SPE 等保持协议、配置、时钟与独立 oracle 前置条件 |
| P4 新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体/Sendable、写验证，保原风险、批准及隔离 |

二进制预审（非交付）：参考 `core/extensions/hiperf_binary_parser.py:298–384`、`hiperf_extender.py:58–111,319–389` 用紧凑逐样本记录携带分支顺序、有效性及周期，再关联符号。真原件 `tests/fixtures/hiperf_brbe_001/test_brbe.data` 30,062,585 B、SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；本地独立参考解析为 12000 样本、11978 有栈、369677 分支记录、894 未知周期，非 Codrax 通过。现 Go 跳过条目；先按已证布局/边界验证有效性、样本内顺序及未知值，不猜尾部、不照搬已删旧列，不冒 SPE、硬件 IPC、实际调用、运行时长或因果权限。

## 保留、发布与边界

[索引](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-index-v1.json) 274 件，SHA `0c537a630a62be611181ab7c7a544b314cfd5bc8d2dcbc213396725320abfeae`；[封存收据](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-receipt-v1.json) SHA `5ae4e51471c4104e53cb8491843eb6e300272fdf6c2ca4a3e0178623f8326eca`；[人工决定](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0132-codrax-hmc356-acceptance-v1.json) closure=none。累计 58 代 26810 件，按代计，不做全局去重。逐件校验本代及前 355/354/353/失败 352 的物理原件；其它 53 代链头。完整大文件保大小、SHA、路径引用，不提交凭证或运行产物；Go 快照归档为 txt，避免测试扫描污染。

本代只发布五份审计/清单文档，不产生成功源码 `publication`，不把候选、失败或未触发计交付。旧采样、IO、栈、权限/图、重复查询、预算/自然引用和只读未验场景继续留账。保 Trace 显式窗、因果投影/自动补齐、合法计算/重试及链上业务线索；WAL 零副作用未证明前保持生产 NO-GO。
