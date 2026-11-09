# §370：优先级见证同源与完整 CLI 复验（冻结失败）

本地 `main` 基线 `6ff8c0a7084fe983b932dc3df3e45a8c98df5eb7`，候选 `d7eef3ca4db3a484bfca77323ea4ffe71678717a` 只留本地，远端只发审计。仍为 **79 唯一 / 29 完整 / 50 开放**，本批完整新增 0、已推子能力新增 0。§368–369 的原失败和正负只读恢复限定通过不回签。

## 两轨结果

- **原生优先级见证（02.4/16.4/12.5）**：参考仓 `core/preprocess/wakeup_chain_index.py` 把物理唤醒点/两端线程分开，`thread_sched_ops.py` 独立量测就绪、运行及补充维度；借鉴这一分层，不移植查询字节。公开入口原先把醒前低优先级依赖供给和醒后就绪／运行相交混成一项教学，[原公开 RED](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0250-codrax-hmc370-priority-red-v2.log)。候选在同一原生重叠见证携带两方 TID、连续 CPU、半开相交和各自物理行；只有实录唤醒打开该就绪段才给出醒后定位。不是汇总延长、排名变动、锁关系或新图箭头，未证背景不升根因；自动补齐、显式窗及原生业务证据不变。成文和选择旁路可共用，非唤醒切出、异源/异主体/异常时间负控过。
- **A 蜂鸟原件**（106,896,120B）：真实成文准确分开 36.311ms 醒前依赖和 13µs 醒后同核重叠；见证为 TID 64156 与 64176、CPU4、L25227 唤醒、L25229 上 CPU，主窗 62ms 的 15.218/0.090/46.692ms 状态和 3.868ms 确定性运行保留。[人工审计](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0190-codrax-hmc370-A-audit-v1.md)。四次实际成文请求后，图本地节点与原生参与者绑定仍失败，下一必要修补 704359B/700000B 未发，强制成文 811875B 亦未发。机器/人工 FAIL，无受理正文、图、HTML，旁路显式不可用；本次自然结束，不是 §368 操作员停止。
- **B 完整 CLI 只读登记验收（18.5）**：从未修复 Python 夹具真实走 analyzer/controller/planner/apply/verify，交付仅把 increment 改为 `value + 1`，已有测试和 setup.py 未改，夹具 HEAD 保留；启动只给夹具 .gitignore 加运行目录规则。已有原生测试每次 3 断言、0 失败，不能把探针/语法结果合算成原用例。实际允许只读登记的后续 planner 同时收到多处 `verification_probe_required=true`。它选择零变更探针、零登记，独立登记分支未命中；报告虽有测试 PASS，durable 明确 `complete/unverified/verification_proof_incomplete`，机器/人工 FAIL。后继以现行私有授权/合同在摘要、清单、探索入口共用一次证明方式投影；探针专属、真执行、历史范围、隔离、审批不变。[人工审计](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0212-codrax-hmc370-B-audit-v1.md)。

## 冻结与证据

- 初始定向抓到旧图例文字保护，修图例保留 runnable 调度等待/运行算力供给两分支，不改断言。末版定向 6 包 235 顶层/391 子通过，race×2 5 包14/18；蜂鸟、网易云、元宝及二进制整件 8/2 通过。冻结 7,693 源输入/51 外部/19 快照，SHA-256 `b23a1c2e34a9bb0ad56bda39c259d10d7f304409f254009ca24f1f517c8f8bbc`。
- 唯一全仓 exit 1：95 包通过/12 无测试/3 包失败，28,805 顶层/34,677 子通过。三个既有非事件 schema 保护发现旧 rank JSON 出现新字段；保留归属及显示消费两项普查发现新增字段未登记。下一代保既有 rank JSON，见证只走本代保留及成文展示，给真实所有者登记，不重签本代。[原始全仓统计](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0238-codrax-hmc370-full-stats-v1.json)。
- 两例真实各一次，唯一完成 A15/B31，无歧义，33 对实际重叠、最长17.960秒。[重叠证据](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0229-codrax-hmc370-actual-request-overlap-v1.json)。封存 281 件；[索引](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-index-v1.json) `53eefb6d43c25f4a64c902d8c3580906f963b407885564f9cf85106628cf4e61`；[收据](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-receipt-v1.json) `f85f890ab82f0150eda34eeb2d15749bdd36e4bb9b815e544ce7b42711f6c7c6`。§369 原始 301 件逐份核 SHA，失败及用户原件不入 Git。
- **二进制 09.3**：[完整原件容器/原始位复核](results/hmc_priority_overlap_cli_registration_20261108_v1/sealed-v1/0231-codrax-hmc370-binary-features-v1.log)：11 个 feature extent 无重叠；attr+56=0、+72=11。369677 个槽中，低两位为 1/2 的 12/41 个各有一个端点字为零，369624 个低两位 3、两端非零。参考 `hiperf_binary_parser.py` 面向特定采集按槽解码、`brbe_ops.py` 按同采样相邻序号拼基本块；本证据只读、仅证该原件原始数值，不采用参考 attr0/偏移启发式推广 ABI，不证有效边、周期、IPC、地址代次或 SPE。活跃 WAL 未证零副作用，不接生产。

ROI 沿用[统一 50 项分层](parallel_selected_summary_hmc_state_account_budget_20261108.md#回归封存和剩余顺序)。先收口旧排名 wire/字段归属和写 planner 多面教学；再推进同一草稿图绑定、必要修补的整轮与重派预算、业务墙钟和内部诊断隔离。旧 IO、采样、栈、代表选择、权限覆盖仍保原账。
