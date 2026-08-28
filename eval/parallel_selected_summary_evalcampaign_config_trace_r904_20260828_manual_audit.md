# r904 配置解释 + 显式窗 Trace 人工审计

- 基线：`main@7f59b66734527d7112b9fd52cce6ed49589a0814`
- 二进制快照：`./.codrax/tmp/codrax-selected-20260828-135748`
- 并发：严格 2 路；单例超时 1200s
- runner：2 PASS / 0 FAIL；人工裁定不继承 runner PASS

| case | runner | 人工裁定 | 耗时 | 关键结论 |
|---|---|---|---:|---|
| `trace_query_wakeup_causal_io_chain` | PASS | partial | 182s | 显式窗、完整唤醒链、链上排序、双账户、自动补齐与 Trace 因果投影完整；模型仍越权扩写页面预取/缓存与锁/优先级继承修向 |
| `qf_config_precedence` | PASS | fail | 306s | 真实 Decode/CLI 覆盖执行体已读，答案主事实正确；但 responsibility 自声明可错配，scalar/count 假冲突进入用户 caveat，8/9 引用被裁掉 |

## Trace

1. 系统合同通过：主窗固定为 `2.000..2.020s`；目标 app-100 四态完整归账；3 次 target-filtered `trace_query`；
   `threadpool-400 -> network-300 -> cookie-200 -> app-100` 唤醒链；11.000ms 链上 IO 第一席；三个独立 1.000ms
   低优先级 runnable/供给候选；实际占时与现规则可消双账户；邻近/背景未顶替主因；自动补齐和完整 `Trace 因果投影` 均在。
2. 一次 finalizer 轻量拒绝只要求补 `kind=summary,surface_role=principal` 与 typed caliber；两次 patch 均为 add-block，未删除模型正文、
   未回退旧稿，也未触发固定 4ms/4m、首字节、stall、累计流年龄或上下文比例降级。
3. 模型遵循仍 partial：typed context 明确 `fscache_page_wait_on_page_bit` 只证明内核调用点，不识别具体对象/子系统/holder，也不授权
   prefetch/cache 修向；终稿仍写“页面缓存未就绪”、建议预取/缓存预热，并从 priority candidate 扩写到锁持有者与优先级继承。继续归入
   `B1269/B1271`，不得通过请求/模型/终稿关键词扫描、硬拒或系统代写来纠正。

## 配置解释

1. B1405 获得部分生产正证：Explorer 真正读取 `LoadRuntimeSettings` 执行体并发射 `yaml.NewDecoder`、`KnownFields(true)`、
   `Decode(&s)` 的 call evidence；终稿正确给出 `50 < codrax.yaml < --pipeline-max-steps` 和最终 `SetMaxSteps`。
2. analyzer 共 5 iteration/4 次被拒后第 5 次成功：先有一个独立 `field_value_profile.target` 缺失，随后经历“高置信文件未分类”、
   “全部 navigation-only 导致维度无 owner”，最终把 `internal/config/runtime.go` 绑定到 `[1,2,3]`。这说明 schema 教学仍增加较多心智，
   但责任错误在一条 correction 中同时列出了全部文件/维度，没有逐文件串行拒绝。
3. 新 P1 `B1408-DIMENSIONOWNERSELFAUTHORITY1`：CLI 覆盖操作实际位于 `cmd/root.go`，analyzer 却把维度 3 owner 绑定到
   `runtime.go`；Explorer 后续虽发射真实 `Changed/IntVar` 行，完成门仍可由 `runtime.go:1635 Decode` 一行携带 `[1,3]` 关闭两个席。
   “typed”只保证载体形状，不证明模型自报的文件/维度语义为真。后续方案必须避免让 prescan 阶段猜测成为硬语义权威；优先研究每个独立 operation
   维度至少需要可区分的 executable evidence seat/匹配，而不是继续增加路径或语言特判。
4. `B1407-AGGREGATECOUNTSCALARCOLLISION1` 已升级为用户可见：validator 把 scalar block 的默认值 `50` 当作
   `pipeline_max_steps 优先级链操作节点` 的 `visible_count`，与 aggregate cardinality=5 比较，生成 soft violation，最终答案附加
   “部分验收检查未达到预期标准”。这是确定性 false positive，下一施工批优先修。
5. 新 P2 `B1409-BLOCKCITATIONASSOCIATION1`：finalizer 提交 9 个真实 citations，但 summary/section 没有 item/evidence carrier，
   emit-time 以 unused 为由裁掉 8 个，最终只有 scalar 默认值引用；正文虽写多个 `file:line`，用户无法从结构化引用面核对解析/覆盖链。
   修复应改进 block-level 事实与 citation 的结构化关联/教学，不应从正文正则回推引用。
6. r903 的陈腐 example advisory 本轮未复现，但这是模型本轮读取路径变化，不足以关闭 `B1396`；继续保持 open。

## 状态

`r904=runner-pass-2/2,human-trace-partial+config-fail`；
`B1405=production-partial/real-decode-read+wrong-owner-self-authority`；
`B1407=production-confirmed/user-visible-false-caveat/P1-next`；
`B1408=confirmed/P1/model-authored-owner-not-semantic-authority`；
`B1409=confirmed/P2/8-of-9-citations-pruned`；
`B1396=not-reproduced-this-run/still-open`；
`B1269/B1271=repeat-partial/model-guidance-open`；
`system-answer/conclusion/relation/wording-selection=none`；
`request/model/final-prose/path-keyword/mermaid-content-scan=none`；
`Trace explicit-window/causal projection/auto-supplement=production-positive-r904`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/production-positive-r904`。
