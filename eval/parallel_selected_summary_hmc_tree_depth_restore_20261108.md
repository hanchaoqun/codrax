# HMC365 — 采样树等价展示与只读缺来源恢复

`main` 基线 `15c81321ab28ae8ad0b32fb44c0aeee4b21364ea`，源码 `39e3cfab2dee4e855b74b354679cd098f890603b`。逐 ID 核账 **79＝29 完整＋50 开放**（18 部分、28 待实施、2 待验收、1 验收中、1 持续），未改勾选。

- **完整能力交付数：本批 0，累计 29。**
- **已交付子能力：生产 1 项。** 不影响实际节点的显示深度可复用同一采样树展示，不复用查询身份。另新增缺来源终态的真实只读恢复验收，有限终态通过，不计新生产能力。
- **剩余验收缺陷：** 蜂鸟众包首稿图主体、修补预算和 JIT 操作分类；只读控制器把工作进度读成来源验证；旧采样执行/时间解释、IO、栈、代表、权限、完整图/CLI、真实二进制 ABI/有效边及 SPE 均留账。

## 两轨与参考仓发现

**已证缺陷（01.3/09.4/16.5/18.4）。** §364 原 Finalizer 重复收到同样的 2,980B 树内容，两查询只差未约束实际输出的显示深度。参考 `core/preprocess/hiperf_ops.py:183–228,405–475` 将原始节点/累积权重与深度、子节点展示裁剪分开；不移植按名字合并 DSO 或展示噪声过滤。本项目只在当前来源、线程代次、查询窗、分母、内容、物理引用相等，完整保留节点且深度严格有余量时，复用显示内容。两条查询保各自地址和请求深度，实际裁剪、根选择、权重/来源变化及同地址冲突不借资格。有限引用省略继续保留。

[公开 RED](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0207-codrax-hmc365-sampling-red-v1.log) 在真实成文 adapter 入口收到两份。原 30,062,585B 完整二进制重放 §364 两查询，修后只展示一次，保 164 记录、259 cycles、8 个原件头位置，旧不同深度和缺边界断言不放宽。无关键词硬门、无查询/因果资格变化，也不把去重写成旧正文误述修复。

**能力验收轨（18.5）。** 新增独立 opt-in 的缺来源终态恢复：工装经真实应用凭证和受限计划分派准备，真实规划器只读登记，恢复子进程的真实控制器决定执行。旧正向、脚本负向与重试验收保留；生产核准/工作树/终态判定不改。当前通过与来源终态缺失是两条独立事实。

**有实材的二进制后继（09.3）。** [只读原件对照](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0196-codrax-hmc365-reference-branch-parity-v1.log) 用参考 `hiperf_binary_parser.py:188–219,230–290,298–378` 的解析器逐物理头比对独立原字节：164 条的 5,216 槽、地址字、序号和低位相符；参考读偏移 56 得分支属性 0，原件偏移 72 为 11，且其槽解码并不要求该属性。不能把这种参考同源吻合当作生产 ABI 门；先核准确版本与原字段，端点有效性和周期未知分别表示，仍不签有效分支边、SPE、运行状态、时长或因果。

## 冻结验证

- 定向 9 包 437 顶层/520 子通过，覆盖关系、时序、逻辑边、状态/语义及只读/自动补齐；race×2 5 包 16 顶层/30 子通过。既有预算和 §364 整件解释重放保持通过。 [冻结](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0172-codrax-hmc365-frozen-v1.json)：7,679 输入、88 外部、8 快照，随后停写源码。
- 用户基线目录 75 份；实际新增整件蜂鸟众包 106,896,120B，对照原事件行、公开时序、成文/图/旁路；旧网易云、元宝整件原生到成文及 30MB 二进制原件保持复验。 [新整件公开查询](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0220-codrax-hmc365-whole-honeybird-v1-receipt.json)／[二进制重放](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0217-codrax-hmc365-whole-binary-v1-receipt.json)／[旧双整件](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0223-codrax-hmc365-whole-real-v1-receipt.json)。定向独立检查不是第三例模型评测。
- 唯一全仓 OS0，98 包通过、12 无测试，28,801 顶层/34,666 子通过、67/14 跳过，269,072 JSON 行，无失败、重复或未终态。 [正式统计](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0173-codrax-hmc365-full-stats-v1.json)。

## 两例真实评测及原始人工审计

新自然蜂鸟众包问句 15 步上限、独立只读恢复用例，各一次。实际唯一配对 A13/B4，**5 对重叠、最长 19.299 s**、歧义 0。机器 **1/2**、整例人工 **0/2**；单项通过单独记。 [并行收据](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0156-codrax-hmc365-actual-request-overlap-v1.json)。

| 用例 | 有限通过 | 失败或未触发 |
| --- | --- | --- |
| [A 蜂鸟众包启动时序](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0120-codrax-hmc365-A-audit-v1.md) | 窗 [11343.248,11343.310)，TID64156 的十段与独立整数微秒逐行相符：运行 15.218、就绪 0.090、休眠 46.692 ms；原始唤醒 64176→64156，13µs 后上 CPU。显式根因旁路正确标不可用 | 图用 MAIN_IN/MAIN_OUT 两个角色名代替精确主体，首稿被拒；初始 677,804B，修补 777,916/826,610B 都超 700,000B 且未发。无已发布正文、表、图或根因。独立单元未引用原生行，§364 的紧邻限定自然分支未触发 |
| [B 只读缺来源终态](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0145-codrax-hmc365-B-audit-v1.md) | 真实规划器、真实恢复子控制器；1 次当前既有测试/1 断言/0 失败，前检与真实调用分开。源保持仅应用，当前证明通过、总体未验证；三份当前报告同字节，原件链接/源码/测试/Git 核对 | 控制器实际只看见来源 `state=complete`，模型提交 `all_verified`；聚合器仍守 `missing_terminal_verify_verdict`。整例人工失败。不冒自然父控制器、自然重试、全 CLI 或全部上下文包耐久 |

**A 的范围与语义。** 独立原件核实 `JitMemoryRegion::Initialize` 只证明 0.372ms 初始化，精确原生分类却是 `jit_compile`，而图/估算沿用了编译说法。生产者 `traceSpanLooksLikeJITCompile` 允许原生标记的 `jit` 单独升类，归 `02.4/16.4`；后续分语义领域和实际操作，不扫用户或模型原文，不丢原业务片段、真实类校验/JIT/着色器/纹理/GC 及链上真实供给。唤醒边不解释进入休眠的条件；相关邻近片段不得代替链。 [原始行](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0178-codrax-hmc365-honeybird-oracle-v1.log)／[独立时序](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0182-codrax-hmc365-honeybird-state-oracle-v1.log)／[实际查询与预算](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0122-codrax-hmc365-A-facts-v1.log)／[显式旁路](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0056-run-1.root-causes.json)。

**B 的交接缺口。** 两个生命周期已结束的行不能替代各自验证终态。真实提示有“当前报告并非累计证明”的一般边界，却未携带来源行的缺证状态。后续由同一类型化终态判定给每批摘要和合法结束方式提供信息；不为错句加过滤，不把当前通过降成失败，也不允许它补签来源。 [模型实际请求及决定](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0146-codrax-hmc365-B-context-v1.log)／[恢复独立复核](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0151-codrax-hmc365-B-facts-v2.log)。

[封存索引](results/hmc_tree_depth_restore_20261108_v1/sealed-index-v1.json) 237 件，SHA-256 `76ea09daa4fc36f57418576361c1590253298654996dce3f56d5b15e3d3d5378`；[封存收据](results/hmc_tree_depth_restore_20261108_v1/sealed-receipt-v1.json) `6becff12ebd3895df96dd1780068d62d873d9d2d17d15acbc4701dbf36053282`；[有限裁决](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0155-codrax-hmc365-acceptance-v1.json) `closure=none`；[发布核对](results/hmc_tree_depth_restore_20261108_v1/publication-v1.json)。前代 228 件物理核对，原 FAIL 不改。整件大源仅路径/大小/哈希，不入 Git。

## 剩余 50 项（ROI 高到低）

1. **P0 共享 8：** 16.5、12.5、18.5、16.4、02.4、01.3、01.2、18.4。先公开重放两条未发送修补请求，在原预算用共享主体/候选和精确修补范围保完整事实、合法时序及关系；并统一控制器的进度/验证两轴，分语义领域与实际动作。图的业务名可读、事件和线程身份不互替，语义充分的旧波动不追第三例。
2. **P1 实材 5：** 09.3、03.2、14.1、18.2、08.3。二进制先核生产者 ABI、属性与同物理记录的有效端点和未知周期，再交付支持子路径；保原采样/IO/资源计量与来源，缺测不补零。
3. **P2 跨域复用 11：** 17.6、04.3、04.5、08.1、08.5、12.2、12.3、05.2、10.2、14.2、17.7。轮换整份真实 ftrace，继续统一业务/窗/状态与三类图，链外只作背景；活跃 WAL 未证零副作用不接生产。
4. **P3 协议 20：** 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。取真实协议和可核配置，再分批完成，不凭局部结论销账。
5. **P4 新域 6：** 07.1、07.2、07.3、07.4、15.2、15.3。依赖实材；保原审批、工作树与只读边界。
