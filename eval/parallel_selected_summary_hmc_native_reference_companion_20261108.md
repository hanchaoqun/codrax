# HMC364 — 原生解释适用域与附带采样树原件定位

`main` 基线 `3d5096b2223658d3b75f13aacdc31f62cc695244`，源码 `004e6174af18c010407b405960705332ef5a39c4`。逐 ID 核对 **79 项＝29 完整＋50 开放**（18 部分、28 待实施、2 待验收、1 验收中、1 持续）。未改任何任务勾选。

- **完整能力交付数：本批 0，累计 29。**
- **已交付子能力：2 项。** 已验证原生行引用附带生产者适用域；同代直接原始 perf 的常用附带采样树可复用物理记录位置。
- **剩余验收缺陷：** 样本正文继续越过执行状态/时长权限；旧网易云状态/零贡献的自然成文和本批类型化解释单元自然覆盖未验；只读缺来源、自然重试及全 CLI，原生有效分支和 SPE、旧 IO/栈/图/代表/权限仍留账。

## 两轨修复与参考设计

**共享缺陷轨（01.3/02.4/16.4/16.5/18.4）。** §363 的实际独立解释引用合法，但系统总体的闭合区间口径没有随原单元投递。原 53.97% 满深度时长占比被说成几乎一直满载；指定线程 TID36435 有 128.273 ms 就绪，不能以 D/IO 为零排除，也不能混用另一应用主线程。参考仓 `core/preprocess/parallelism_ops.py:29–72,125–177` 用同一窗内的闭合区间，分开活动并集、累计 CPU 时间、运行和就绪需求；活跃分母不能替代完整窗，也不借“未观测”证明不存在。这里保原查询量测和各自分母，把适用域放进当前原生行的交接，不移植其分母或改分类。

[公开 RED](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0201-codrax-hmc364-scope-red-v1.log) 覆盖系统计数和线程状态。由注册生产者与当前绑定提供短口径；对同一单元、完全相同的查询窗/选择/TID/口径只共享说明，独立行身份不合并。依据与原文共用原 900/1800 B，先保完整单元和边界，再给可选导航；整段放不下明确报省略，地址仍不能证明解释、算术、图边或因果。没有新增模型输入字段，没有扫问答原文或改原话。整件网易云重放旧成文单元；原字句和边界仍到 Finalizer，Extractor 明示过大而整段省略。旧 FAIL 保留，不能用重放倒签自然成文。

**有实材能力轨（09.3/17.6）。** 参考仓 `core/extensions/hiperf_binary_parser.py:230–290` 先存记录头偏移、时间、大小，后按时间逐条解码，采集器 ID 不是物理位置。旧 [公开 RED](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0154-codrax-hmc364-binary-red-v1.log) 证明四个附带视图都可返回同一原生树，却没拿到已准备的原件位置绑定。把同代绑定交给共享树构建，覆盖 `perf_stats`、`perf_timeline`、`window_stats`、`trace_perf_bundle`，与直接流式入口共用记录身份；重复查询复用准备和现有末端代次检查。不放宽查询过滤、不重转、不增加原件内容扫描，压缩外壳/脱离准备的派生文件不借定位权。物理位置仍不是样本执行状态、时长、调用或因果。

本批只读[分支槽位预检](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0155-codrax-hmc364-branch-slot-recon-v1.log)：指定窗有 5,216 个 24B 槽，两个地址字均非零、标志字低两位均为 3，尾部却有 16B、24B 两种布局。参考导入器 `core/extensions/hiperf_extender.py:191–200,373–415` 用同时间戳/TID 映射 SQLite 样本并丢弃未知周期分支；下一步先核确切生产者 ABI、端点有效性与周期未知的独立性。不能照搬可能覆盖同时间同线程样本的映射，也不能凭文件名、位模式、邻近槽或未知周期推导调用/执行关系。

## 冻结验证

- 定向 9 包、291 顶层/335 子通过；race×2 4 包、12 顶层/40 子通过。原预算重放继续通过，源码和用例冻结后停写。 [冻结](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0170-codrax-hmc364-frozen-v1.json)：7,676 输入、84 外部、8 源快照。
- 用户目录清点 75 份 ftrace。整件网易云 82,838,676B、元宝 63,659,046B 经原生查询和实际成文接缝对照；额外重放网易云旧独立解释和参考整件 30,062,585B 的四种附带树物理头。 [双整件](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0214-codrax-hmc364-whole-real-v1-receipt.json)／[旧单元整件重放](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0227-codrax-hmc364-whole-state-v1-receipt.json)／[整件附带树](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0211-codrax-hmc364-whole-binary-v1-receipt.json)。无模型检查不算第三例。
- 唯一全仓 OS0，98 包通过、12 无测试；28,800 顶层/34,665 子通过，64/14 跳过，269,046 行 JSON，无失败、重复或未终态。[终态统计](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0171-codrax-hmc364-full-stats-v1.json)。原 Trace 因果投影、闭合睡眠实例、自动补齐及现有只读/写入隔离回归未变。

## 两例真实评测及人工审计

各一次同代并行：不改原 sample-only 问句（15 步上限）和现有正向只读登记问句。机器 **2/2**；可唯一配对实际请求 A1/B5，排除两个歧义组，**1 对重叠 7.550 s**。[实际请求收据](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0150-codrax-hmc364-actual-request-overlap-v1.json)。

| 用例 | 原生、结构及有限审计 | 未通过或未触发 |
| --- | --- | --- |
| [A 完整原始 perf](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0117-codrax-hmc364-A-audit-v1.md) | 专用树及 timeline 附带树各 8 个相同物理位置，与独立 oracle 和磁盘记录头吻合，并进入实际 Finalizer。164/259；内核地址 82/174，传感器回调 82/85；四张 Markdown/HTML 表逐格同。因果投影链止 | 正文把两段约 1.5 ms 的点包络写成 CPU 时间片，且把未知样本升为 on-CPU。整答人工 **FAIL**。带事件过滤的 perf_stats 正确拒绝附带树，原生统计仍 164/259；其它附带入口未自然触发。没有结构化解释单元、图或独立根因旁路输出，不计自然验收 |
| [B 只读已有测试登记/恢复](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0143-codrax-hmc364-B-audit-v1.md) | 真实规划器及父/子控制器、独立子进程新调用；1 既有测试、1 断言、0 失败、2 命令。源/执行/工作流三层终态，当前三份报告同字节，父/子原件与链接核对；源码、测试、Git 不变。正向限定人工 **PASS** | 来源准备由公共工装提供，不是自然完整 CLI；自然重试和缺/旧来源终态的真实控制器尚未覆盖。不扩签 HMC-18.5 |

A 的实际原生边界和样本类型未知都到了成文；Explorer 的旧 `reason` 有误述，但捕获的 INIT 未见其原话，也没有使用新的结构化单元。不能认定误述由那段原话被注入造成。另一个已证可改缺口：两份树实际只差请求显示上限 6/4、实际两节点且无省略；重复展示内容为 2,980 B。后续做等价内容复用，不合并查询或改已有不同深度/省略的断言，不以去重替代语义审计。[物理位置](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0140-codrax-hmc364-A-records-v1.log)／[实际上下文与表](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0118-codrax-hmc364-A-context-v1.log)／[只读独立复核](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0145-codrax-hmc364-B-facts-v1.log)。

[封存索引](results/hmc_native_reference_companion_20261108_v1/sealed-index-v1.json) 228 件，SHA-256 `6b7c5b19d5feb190cdb1e88c0761be5612bc468b1c992b4e2bf5dab6c317e674`；[收据](results/hmc_native_reference_companion_20261108_v1/sealed-receipt-v1.json) `8f7bf985f6c2ad0cf025587710b903dc702b89d32cc0cdf822c8231d66c110b1`；[有限裁决](results/hmc_native_reference_companion_20261108_v1/sealed-v1/0149-codrax-hmc364-acceptance-v1.json) `closure=none`。[发布核对](results/hmc_native_reference_companion_20261108_v1/publication-v1.json)。大原件只留路径、大小、哈希，不入 Git；§363 的 375 件逐字节复核，原始失败未改。

## 剩余 50 项（按 ROI 高到低）

1. **P0 共享 7：** 02.4、01.3、16.4、01.2、16.5、12.5、18.4。先处理已证同内容重复与适用域冲突，保查询窗/主体/状态/单位及原查询权限；审计正文、表和关系/时序/逻辑图共用同一事实，业务名保留而节点及边身份不漂移。已足证的模型误述留账，不追同例求绿。
2. **P1 实材 6：** 09.3、18.5、03.2、14.1、18.2、08.3。先用当前原件核支持的分支 ABI 和有效端点，未知周期不补零、不以槽位冒边；轮换缺/旧来源终态的真实控制器与合法重试、全 CLI；继续按独立量尺核资源、IO 和驻留，样本不转时长。
3. **P2 跨域复用 11：** 17.6、04.3、04.5、08.1、08.5、12.2、12.3、05.2、10.2、14.2、17.7。用另一完整原件扩展业务、时间窗、关系与状态覆盖。已盘点 105 版本蜂鸟众包后台销毁后冷启动第 1 份整件（106,896,120B），有调度切换、唤醒、阻塞原因和业务标记；只是候选，须先做独立原生核验，邻近背景不升级根因。活跃 WAL 未证零副作用不进生产。
4. **P3 协议 20：** 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。实材/可核配置先行；缺测不补零，不把局部交付称完整。
5. **P4 新域 6：** 07.1、07.2、07.3、07.4、15.2、15.3。依赖实材再做完整验收；保审批、工作树和只读边界。
