## 启动记录总览

在 1.040s–1.080s 窗口内，进程 PID 27599 共记录到 2 段启动阶段（AppStartup 区间）和 5 条系统事件（HiSysEvent），均来自同一个进程。

**启动阶段记录（Process Interval）**

| 项目 | 阶段名称 | 开始时刻 | 结束时刻 | 耗时 | 所属进程 PID |
|---|---|---|---|---|---|
| LoadPreferences | LoadPreferences | 1.040000 s | 1.048000 s | 8 ms | 27599 |
| RestoreTabs | RestoreTabs | 1.064000 s | 1.072000 s | 8 ms | 27599 |

**系统事件记录（HiSysEvent）**

| 项目 | 时刻 | 事件类型 | 内容 | 来源线程 TID |
|---|---|---|---|---|
| UI_READY | 1.046000 s | UI_READY | scene=document status=ok | 27599 |
| STAGE_READY | 1.050000 s | STAGE_READY | stage=repair status=pending | 27599 |
| APP_LAUNCH | 1.052000 s | APP_LAUNCH（事件名未解析） | stage=draw status=pending | 27599 |
| BOOTSTRAP | 1.058000 s | BOOTSTRAP | stage=bootstrap status=ready | 27599 |

> 以上事件全部提取自attached trace，属于运行时观测而非当前仓库源码。事件内容中 APP_LAUNCH 的 event_name 在跟踪中标记为未解析引用（unresolved_reference），BOOTSTRAP 事件在查询范围内最早出现于 1.058s。