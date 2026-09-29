## 1.040–1.080s 区间启动记录与系统事件

在 1.040–1.080 秒区间内，trace 共记录了 2 个启动阶段和 4 条系统事件，均归属于 PID 27599 的进程。

### 启动记录（进程区间）

两个 AppStartup 区间均为进程级标记，不记录具体执行线程，信息如下：

1. **LoadPreferences**：1.040000 s → 1.048000 s，耗时 8 ms，所属进程 PID 27599。
2. **RestoreTabs**：1.064000 s → 1.072000 s，耗时 8 ms，所属进程 PID 27599。

### 同期系统事件

同一时间段内共捕获 4 条 hi_sysevent，涵盖 APP_LAUNCH 域的 UI 就绪、阶段状态和 bootstrap 完成等事件：

| 时间（s） | 事件域 | 事件名称 | 内容 | 进程/线程 |
|---|---|---|---|---|
| 1.046 | APP_LAUNCH | UI_READY | scene=document status=ok | PID 27599 |
| 1.050 | STAGE_READY（内部） | STAGE_READY | stage=repair status=pending | TID 27599 |
| 1.052 | APP_LAUNCH | APP_LAUNCH | stage=draw status=pending | TID 27599 |
| 1.058 | APP_LAUNCH | BOOTSTRAP | stage=bootstrap status=ready | PID 27599 |

从事件时序可见：LoadPreferences 在 1.048 s 结束后，APP_LAUNCH/UI_READY 于 1.046 s 已发出（提前于区间结束），随后 draw 阶段进入 pending，bootstrap 在 1.058 s 就绪。RestoreTabs 在 1.064 s 开始，并在 1.072 s 结束，与前面的事件序列有一定重叠。

> 本次分析基于用户指定范围 1.040000–1.080000 秒内的确定性 trace 查询结果。启动区间（AppStartup）和系统事件（hi_sysevent）均来自 attached trace 的 event_search 视图，进程/线程标识源自 trace 解析结果，不关联当前代码仓库文件。