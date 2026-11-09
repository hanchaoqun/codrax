# §368：原生运行工作量测与只读自然结束（首代冻结失败，源码未单独发布）

本地 `main` 基线 `66c2c7ce30d46c64cde2e7f267810b91a5fe0b5a`。79 个稳定 ID，29 完整、50 开放：18 部分、28 待实施、2 待验收、1 验收中、1 持续。本代新增完整交付 **0**、发布子能力 **0**，两个实现候选；B 有限人工 PASS，A 操作员终止、整例未验收。旧失败不回签。

## 双轨与参考发现

- **缺陷轨 02.4/16.4：** 参考仓 `core/preprocess/thread_sched_ops.py` 把同线程的状态时长与频率/温度等增量观测分列，`bottleneck_scanner.py` 按实际区间取交集与并集，`wakeup_chain_index.py` 把线程实例和实际唤醒点保持独立。Codrax 不照搬 SQL：标记配对沿用已有原生扫描；状态分区保持原数，区分未量测、已量测零、已量测非零、冲突。仅同采集、同强代次、同窗/线范围/线程/原生分区、同未加滤镜口径可选更完整的整条原生凭证；冲突只撤销增量分解，不加总、不换根因资格。旧无状态的正数可读，旧零不猜。运行剩余量只称未计入已量测工作，不声称没有实际可优化工作。
- **能力轨 18.5：** 参考仓 `core/skill_executor.py` 区分执行器完成和交付/校验的返回状态。Codrax 的普通 `finish` 结束所需范围，不表示所有历史证明闭合；JSON 仅为当前本地验证器不可用解释 `accept_unverified`，不覆盖真实测试失败。已经完成批次的独立持久 verdict 不重算、不抹掉；合法且不同的下一批仍可选择。公开入口复现并重放 §367 的同一 source/proof 档案。见 [Trace RED](results/hmc_running_work_finish_20261108_v1/sealed-v1/0258-codrax-hmc368-trace-red-v1.log)、[只读 RED](results/hmc_running_work_finish_20261108_v1/sealed-v1/0278-codrax-hmc368-write-red-v1.log)。

## 双例、失败与不销账范围

- 恰好两个真实运行、各一次，原问题不改；A25/B4 个唯一已完成模型请求，1 个歧义组排除，5 对实际重叠、最长 10.018 s。[配对证据](results/hmc_running_work_finish_20261108_v1/sealed-v1/0180-codrax-hmc368-actual-request-overlap-v1.json)。
- [A 蜂鸟人工审计](results/hmc_running_work_finish_20261108_v1/sealed-v1/0147-codrax-hmc368-A-audit-v1.md)：完整用户原件的自动补齐保住用户 62 ms 窗和 3.868 ms 类校验交集；宽探索窗 6.753 ms 未被借入，未量测行不再显示零。公开实测同窗五个视图正反顺序均保 15.218/0.090/46.692 ms 和 3.868 ms 细分；自然 CLI 精确窗没有两个可互补账户，不能签到自然同窗精化。实际首稿与多轮局部修补的图绑定均未受理；重复后 [操作员终止](results/hmc_running_work_finish_20261108_v1/sealed-v1/0170-codrax-hmc368-A-operator-stop-v1.json)，OS signal=15、独立机器 verdict 缺失、整例人工未验收，未产受理图/HTML/根因旁路。记录 13 次真实同轮预算分支，首例 753,015→644,685 B，原阈值 700,000 B；另 704,472 B 原阈值拒发与强制重派仍留账。内容配额不与协议包装字节混算。
- [B 只读人工审计](results/hmc_running_work_finish_20261108_v1/sealed-v1/0172-codrax-hmc368-B-audit-v1.md)：真实规划器、独立恢复控制器自然 `verify_batch → finish`（无 disposition）。恰 1 次新执行、1 断言、0 失败；当前 `proof` 已验证，历史 `source` 仍只有 apply，工作流完成但整体证明为 `unverified/missing_terminal_verify_verdict`。三份当前报告同字节、父子归档同代匹配；源、测试、Git 未变。有限机器/人工均 PASS，正向历史、全 CLI、自然重试和上下文包不代验。
- 实际成文输入仍把 36.311 ms 唤醒前依赖候选和 13 µs 醒后竞争塞进同一「仅唤醒前、不能说明醒后延迟」说明。后续从原生竞争见证传入两方实际 TID、区间和同核重叠范围，排序族可共用但角色不能共用；不从邻近、别名或同核唤醒点推根因，不移除合法候选。图修补的旧绑定、重派预算、旧 IO/采样/权限及足证偶发误述继续独立留账。

## 全仓、封存与二进制

- 定向 12 包 520 顶层/494 子通过，race×2 6 包 82 顶层/24 子通过；蜂鸟、网易云、元宝及二进制完整原件回归通过。冻结 7,690 源输入/91 外部/22 快照。统一全仓 **97 包通过、12 无测试、1 包失败**：原零起点验收中 window_stats、thread_timeline × zh/en 四个叶断言缺所选窗口，其余 28,807 顶层/34,673 子通过。[原始全仓统计](results/hmc_running_work_finish_20261108_v1/sealed-v1/0203-codrax-hmc368-full-stats-v1.json)。原生数据仍有窗；新增可选量测和内部标识挤占固定模型注释槽，不能用定向通过或 B 自然通过放行源码。首代代码与失败均保原始收据。
- [整件二进制只读探针](results/hmc_running_work_finish_20261108_v1/sealed-v1/0183-codrax-hmc368-binary-layout-v1.log)：30,062,585 B、175,968 个物理记录，全部 12,000 样本的 ID 唯一归 attr0，369,677 个声明槽（22 样本真实零槽），尾计数字段逐记录匹配剩余字节，0 不符。这是特定原件的结构证据，不授有效分支、硬件 flag、地址/周期语义或 SPE。活跃 WAL 未达零副作用，不接生产。
- [封存索引](results/hmc_running_work_finish_20261108_v1/sealed-index-v1.json) 278 件，SHA-256 `4c814ac3d6aa479500a5797e09aa24f6970858058463661cd61aecc48864e124`；[收据](results/hmc_running_work_finish_20261108_v1/sealed-receipt-v1.json) `4f9bd45b8ebb07f27cc278f33bd03666229d74f4fa4ccd491ecc62a90f0ceeec`。前代 243 件物理复核，失败未改、原件和凭证不入 Git。
