在 1.040 秒至 1.080 秒的时间窗口内，采集数据共记录到两条应用启动阶段记录（codrax_process_interval），均属于进程 PID 27599。两条记录的耗时均为 8ms。

此外还有 5 条 HiSys 事件（codrax_hisysevent），属于点状事件而非区间记录，展示了同一进程内 UI_READY、STAGE_READY、BOOTSTRAP 等启动里程碑的到达时刻，可作为时序参考一并列出。

**启动记录明细（1.040–1.080 秒）**

| 名称 | 开始时刻 | 结束时刻 | 耗时 | 所属进程 |
|---|---|---|---|---|
| LoadPreferences | 1.040 s | 1.048 s | 8 ms | 27599 |
| RestoreTabs | 1.064 s | 1.072 s | 8 ms | 27599 |

### 参考：同时间段 HiSys 里程碑事件（点事件）

以下事件均为时刻点记录，非起止区间，用于参考 LoadPreferences 与 RestoreTabs 之间的启动进展：

| 时刻 | 事件 | 内容 |
|---|---|---|
| 1.046 s | UI_READY (APP_LAUNCH) | scene=document, status=ok |
| 1.050 s | STAGE_READY | stage=repair, status=pending |
| 1.052 s | APP_LAUNCH (未解析事件名) | stage=draw, status=pending |
| 1.058 s | BOOTSTRAP (APP_LAUNCH) | stage=bootstrap, status=ready |