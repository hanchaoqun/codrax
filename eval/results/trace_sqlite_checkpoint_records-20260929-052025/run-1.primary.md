在 1.040s 到 1.080s 这段 40ms 窗口内，trace 共记录到 2 条完整的应用启动间隔（codrax_process_interval），均属于同一进程 PID 27599。

### 启动记录明细

- **LoadPreferences**：起止 1.040s → 1.048s，耗时 8ms，所属进程 PID 27599。
- **RestoreTabs**：起止 1.064s → 1.072s，耗时 8ms，所属进程 PID 27599。

### 附加即时事件

同一窗口内还散落着 5 条单点 hi_sysevent 事件（1.046s UI_READY、1.050s STAGE_READY、1.052s draw、1.058s bootstrap），它们仅为时间戳标记，不构成起止区间，故未计入启动记录清单。