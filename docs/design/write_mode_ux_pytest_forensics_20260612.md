# 用户实测 UX 取证(pytest-40842ba6 日志)+ s11b lock-in 翻案

## s11b 取证定案(假设翻案)
"精确目标已命中"横幅源自 `render/answerdoc.go:410`,渲染的是 **finalizer 模型自己 emit 的 typed `exact_resolution` 字段**(emit_answer_document_v2.go:40)——非系统早期锁定。真形态:finalizer **同文自我矛盾**(typed 字段宣告 exact_match=transientRetryBudget,prose 否认该机制与所问相关)。归 L4 self-consistency reviewer 扩展候选(既有 backlog):精确检查 = exact_resolution.anchor 与 body 主张机制的一致性(typed 对 typed,可避开关键词匹配:anchor 符号是否出现在答案的 citation 集且未被 absence/disclaimer typed 字段标记)。

## 四个用户实测问题(空仓 pytest,贪吃蛇请求)

1. **写任务落入 artifact_generation**(log 85206):read REPL 下"用python写一个贪吃蛇游戏"被 turn_policy 路由 `route=operation operation_kind=artifact_generation`。L2 红线禁止 classifier 选 write(正确),**gap 是无引导**。设计:turn_policy 在 `operation_kind=artifact_generation && target=file_artifact && confidence≥高` 时(全 typed 信号),在答案面板附一行"如需走可审计的写管线(plan→apply→verify、可回退 diff),用 `/mode plan` 重新发起"——软引导,不自动切换,L2 保持。
2. **空仓写模式强制 flag**(log 86198:39):非 git 目录写模式要求 `--auto-init-repo` / yaml `write_auto_init_repo`。设计:REPL 交互态改为**当场询问**("目标目录不是 git 仓库,写模式需要先 git init。现在初始化?y/N"),替代 flag 仪式;CLI 单发保持显式 flag(无人值守不可静默改状态)。实施点:orchestrator.go 该日志的 gate 处区分 REPL/单发 + repl 提问通道。
3. **空仓 repomap ✗ 刷屏**(已修):`TotalFiles==0`(typed)时渲染中性"未发现可解析源文件(空仓库),已跳过索引",不再 ✗ 失败 ×6。
4. **plan 面板无下一步**(已修):`renderChangePlanSummary` 尾部加一行 `/plan show · /approve · /reject` 双语提示。自动 approve 不做默认(approval_policy=auto_safe 已存在,用户可配置;面板提示先解决心智负担)。

## 状态
- [x] #3 #4 已修(`5bd36903`)。
- [x] #1 路由软引导(2026-06-12):`operationWritePipelineGuidance` 三 typed 信号门(operation_kind=artifact_generation + target_surface=file_artifact + confidence≥0.7 named const),双语单行,接 operationDispatch 制品 lane + operationUnavailableMsg 两个面;REPL-only by construction(单发 CLI 走 commandOperationPlanMarkdown 不同 builder)。
- [x] #2 REPL auto-init 询问(2026-06-12):`preRunBareDirConsent` 挂 dispatch spinner 前,write 模式 + NeedsInit + 交互态当场 y/N;**单 consent 覆盖双层**(init + 空目录 scaffold,`worktree.DirIsEffectivelyEmpty` 上移为 canonical 探针)否则空目录场景 consent 后仍死端第二道墙;/approve lane 同步扩展 scaffold 层;脚本态/单发保持 flag 仪式 fail-loud;per-Run defer-restore 防泄漏。
- [x] s11b → L4(2026-06-12):落在确定性 V2 oracle 链(非 opt-in reviewer——那默认关),`validateExactResolutionGrounding` typed-vs-typed:exact/alias anchor ∩ {citation enclosing_function, evidence anchors, edge endpoints}(排除 reviewer 侧 regex 行提取噪声源),negative_pattern 同符号 = 同文自證自反;空池 no-op(集合侧缺失是噪声);`ViolExactResolutionUngrounded` SoftByDefault+Promotable,LocusFinalizer。
- [x] trace_query 余 6 案:5 首跑 PASS + inode_event_search spec 否定词表第 7 例(答案"无法直接…对应…路径"实质正确)拓宽后对既有输出验证 PASS。
- [ ] read_combo 21 案扫批进行中(本 session)。
