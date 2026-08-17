# r623 人工审计：IO 跨阶段量尺闭环，旧 oracle 误报布局失败

- 基线：`main@b1eb11121`
- 严格并发：恰好 2 个案例
- Runner：`1 PASS / 1 FAIL`
- 人工：Trace `pass`；H3 `partial`

| case | runner | 人工 | 审计结论 |
|---|---|---|---|
| `trace_query_wakeup_causal_runnable` | PASS | pass | 显式窗、4 次 typed 查询、唤醒链、链上 8.300ms 调度等待、因果投影与自动补齐均保留；无固定 4ms 流降级。 |
| `real_trace_h3_iofam_one_seat` | FAIL | partial | 终稿已明确 1.347ms 是单请求设备端墙钟、1.337ms 是对应目标 S-state 阻塞墙钟、4 次 union≥4.384ms、scheduler D/io_wait=0 不否定 S wait，41.329 request·ms 非墙钟且不可相加。Runner 失败仅因旧正则要求数字与“墙钟”同一物理行。残余是两段请求数分布写反、把 global 198 写成“issued”以及若干 wire 词进入正文。 |

## 判定

`B981-IOCLOCKSCOPECROSSSTAGE1` 获生产正证。`trace_query` 首次文本面已携带两把 scope；explorer 中途仍一度把
`single_request_elapsed_wall_clock_not_target_blocking` 错括注为“非墙钟”，但 finalizer 最终按 producer/final typed
authority 恢复正确量纲，未再形成 r622 的用户可见矛盾。该中途波动不应通过答案硬门或系统改写结论解决。

H3 oracle 改为分别钉：三个物理数值必须出现；正文必须存在“单请求/设备端墙钟”和“目标线程阻塞墙钟”两类语义；
41.329 必须明确非墙钟/不可相加。它不再强制数值与分类词处于同一 Markdown 行，也不指定表格布局。

`B982-RUNTIMEFACTREADERFACE1` 确认但不在本批硬做：终稿仍复制 `request_residence`、
`completion_closed_issuer_blocked`、`target_wait_occurrence_prompt`、`io_latency_coverage` 等控制名。最优方向是给所有有限
runtime fact family 建共享 typed reader-label 层，同时保留机器审计载荷；不得扫描或删除模型答案原文，也不得用固定中文替换模型结论。

状态：

- `B981-IOCLOCKSCOPECROSSSTAGE1=production-positive-r623`
- `H3-oracle=line-layout-coupling-retired/semantic-rulers-pinned`
- `B982-RUNTIMEFACTREADERFACE1=confirmed/open-general-registry`
- `Trace explicit-window causal projection/auto-supplement=pass-r623`
- Trace root=`typed-on-chain-only`; adjacent/background=`support-only`
- `system-answer/conclusion-authorship=none`
- `active-stream-4ms-degrade=forbidden/not-observed`
