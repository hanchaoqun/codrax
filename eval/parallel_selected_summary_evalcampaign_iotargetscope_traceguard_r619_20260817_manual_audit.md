# r619 人工审计：IO owner scope 正证与重复查询物理身份缺口

- 基线：B975/B976 + B977 owner-scope 提示 + 量纲语义 oracle。
- 执行：`real_trace_h3_iofam_one_seat` 与 `trace_query_wakeup_causal_runnable`，并发恰好 2，单次回放。
- Runner：2/2 PASS，flagged 0/2。
- 人工：H3 major-partial；Trace pass。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| real_trace_h3_iofam_one_seat | PASS | major-partial | 量纲与作用域已显著改善：终稿明确 1.347ms 是单请求墙钟、1.337ms 是目标阻塞墙钟，4 次 union=4.384ms；41.329 request·ms 明确非墙钟且不可相加；storage/inode/page-cache 均标为系统上下文或与目标无直接关系。仍有一个确定性矛盾：同一物理 request 被两次 window_stats 以不同 result ID 重复携带，authority 的 ID 去重把 6 个不同目标请求算成 7 个 witness；正文头/表也出现 7 vs 6。 |
| trace_query_wakeup_causal_runnable | PASS | pass | 链上 worker-200 runnable 8.300ms、低优先级依赖和目标 S-state 保留；完整因果投影、自动补齐与 root-only-on-chain 不受有限 IO authority 影响。 |

## 新处置

`B978-RUNTIMEFACTPHYSICALDEDUP1/P1`：有限 authority 对 exact `io_latency` 使用 producer-owned 物理身份
`artifact + endpoint family + dev + sector + len + issue_ts + complete_ts + issuer` 去重。任一字段缺失即按原 record ID
fail-open，不按摘要、数值近似或时间邻近合并。该去重只影响模型上下文重复展示和 witness count，不修改 ledger、查询结果、
因果 rank、阻塞 interval union 或答案。
