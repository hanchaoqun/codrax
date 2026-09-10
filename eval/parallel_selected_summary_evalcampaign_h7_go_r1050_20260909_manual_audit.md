# r1050 人工审计：H7 明确窗全谱 Trace / Go 精准写入

- date: 2026-09-10T01:56:16Z
- sweep_start_ts: 20260909-185614
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

冻结二进制 `bd471758e329`，构建于 `2026-09-10T01:55:33Z`。243 个 case 按受影响能力、模式轮换、来源覆盖和持久化路径选本批，严格同时 2 个，各一次；未改 case、oracle、默认步数。机器表保留原结果，人工结论独立。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | FAIL | eval/results/patch_go_typo-20260909-185616 | write_apply,write_patch_oracle,answer_contains | none | 187s | 28 | read=4,repo_map=0,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（实际代码/请求）；证明语义仍有限 | 最终镜像是 proof-only 计划，runner 错拿它检查已应用 patch；原 machine FAIL 不改 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260909-185616 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 288s | 58 | read=6,repo_map=0,list=0,trace=4,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=3 | fail（整份答案） | 投影/0.040ms 修复保留；正文单位/跨范围求和/计数错误，系统跨查询状态混算及组数量词仍有 gap |

## 1. Go：代码正确，机器失败域取错；证明身份修复获 live 正证

原 patch 计划 `plan-1789005424994848000-25872` 只改 `main.go:25` 的 `retrun` 为 `return`。日志 303–306 的 structured_builder/git_apply、checkpoint `6b2f2b9d08ee2ce581915e10ea0ed049e1e9a48d` 及交付 primary_source_plan_id 指向原 patch，不是最终 proof 计划。fixture→durable diff 精确一行，`main_test.go`、`go.mod`、README 字节未改，applied-tree 与 durable 一致。

三次真正的 `go test -json ./...`（日志 674、1057、2610）均 exit 0，原 TestGreet 的 empty/whitespace/codrax 三臂通过；另一次 Go 编译包装 probe（2608）exit 0，不称四次 probe。正式完成报告 verified，交付义务 10 covered + 1 advisory，无 open items。

最后保存的 `plan-1789005539638893000-25939` 是零变更证明计划，status=applied 且 `persistence_kind=proof_probe_only`，worktree/applied_at 保留。真实 emit（2230）→verify（2626–2629）→保存（2839）→退出清理/UpdatePlanStatusOnDisk/Load 已走通，**B676 获持久化 live 正证**。该批是 verify_only，未走 coder；本次没有实际触发新 empty-apply 拒绝臂，不能冒签。

**B1634c/P2 再确认**：唯一 machine failure 为 `no_plan_regex:"kind":[[:space:]]*"patch"`。`eval/run.sh:1660–1671` 使用最后 `$plan`，而真实应用的原计划明有 patch。需要单独的“已应用计划”oracle 范围，以 durable checkpoint/source ref 绑定，最终报告另验；不能把所有历史草稿拼起来求过针，也不为本例降低 regex。原机器 FAIL 保留。

证明边界另账：初次 PTO 把 go_build 当原生测试，准确反馈 unresolved；Python grep probe 被正确语言门拒一次。随后 Go wrapper 调 `go build -o /dev/null main.go`，只能直接证明编译通过，不能推广成任意源码字符串不存在的证明。模型虽最终说“间接”，其 probe 输出仍过强；已有教学已警告不要 wrapper，本次不是新教学缺失。手工源 diff 证明具体 typo 确已修正，但 **B1561/B1616b 的泛化行为证明语义不因此关闭**。提前 all_verified（2440）被正确要求重新 verify（2451），未跳过执行。

## 2. H7：投影保留，核心账户补齐；正文与系统显示分开判

最终 `.codrax/output/20260909-190101.450-25868.md`（127849 bytes）及 HTML（3316140 bytes）存在。139-byte `.root-causes.json` 也存在，status=unavailable / reason=`valid_model_root_cause_selection_unavailable`，如实说明没有有效的模型根因选择；不是旁路未生成，也不代表所有根因为零或因果投影缺失。

明确用户窗 `13762.791708..13763.024898`（233.190ms）和 1 份因果投影均保留，系统 frame bundle 补采仍执行。链上根因与邻近背景分开；实际 running 74.915ms / 规则折算 65.912ms 两轴、D 36.757ms、链上 IO 0.985ms、微小贡献、业务 span/语义线索未被本批删掉。

**B1636 live 正证**：日志 950/966 与 MD91/632 的 runnable=1.576ms、已归账=231.834ms，未知=1.356ms；原 131→139 行的 `13762.793064..13762.793104` 首唤醒后缀 0.040ms 已回到账户及树（MD240）。running74.915/S118.586/D36.757/IO0 不变，未知前缀未被填造。附着副本行号偏移仍与原源区分。

正文人工 fail：

- MD29 将主查询 65.912 与 source line1..400 筛选查询 1.326 加成 67.238；二者**时间窗相同但原始行范围不同**，不是可相加的两段时间窗。
- MD31–35 将 typed 650000/920000/840000/750000 kHz 写成 650/920/840/750 kHz，单位错 1000 倍。
- MD39–45 混淆 11 个物理 D 区间、4 个 CPU 汇总成员与 12 条内核原因记录；把 16.064/10.424/6.495/3.774 的组累计说成原因明细，4+4 不等于 12，3.757 与同句 2.400 也矛盾。
- MD49 的 top3≈98.5% 既无合法共同可加分母，按列值也不可复算。相关不可任意相加、单位、原因记录不等于 D 区间的指导已入模（2378/2992/3120/3233）；不能用改系统正文或关键词硬门“修好”。

**枚举审计更正**：主结果 items=33 是 12 个链上 rank1–12、5 个无 rank 的 self/data_gap/caliber 与 16 个邻近 rank1–16，不是 33 个链上席位。正文 #1–12 已覆盖已发布链上排行，不能虚报缺 #13–33；但另有 compaction 未展开 12 条链上记录，故其“所有/完整”表述仍过强。原 raw 29–42 行 compact 表的 12/33 已含所有 12 个已发链席；292–335 行全 33 compact 表虽未在 24KiB head +4KiB tail 内，不等于链榜完全未交接。

MD72 的 Playe2-V15 81.616ms 有 filtered query f6949c97 的真实来源，不是错拿别的线程值；需保留它与原 runnable230.649、不同查询和不同量尺的边界，不能凭名字相同就拼数。

## 3. 确认的系统残余与施工顺序

1. **B1638/P1：跨原始行查询的目标状态混池**。主 query b0e1e8cd 已列 8 个 S 记录和 78.630ms，filtered f6949c97（相同时间、line1..400）又给开放尾 S230.286ms；最终 MD221 加成 S308.916，连 D36.757/R1.576 形成 MD128 等待347.249ms。已有“仅纳入记录”注不能解除重叠来源混算。另 log3234 的 D36.757 配 filtered kernel census1/3.213；该 filtered 账户实际 D0。`final_decision_boundary.go` 只按 capture/target/time 对比，忽略 query/source-line 范围。该卡片已有 `unjoined_distinct_observation_domains` 和禁止差值说明，不能说它授权逐记录映射；仍缺所选查询来源配对。须从 producer QueryScopeID/full query/同结果收据定义单源范围，缺来源保独立事实不授权合并。QueryScopeID 本身还含 view/child scope，不能直接按它全局硬切而破坏合法跨 view 补齐；需要明确比较资格。禁止用短 hash、文字标签或模型 prose 推断身份。已安排公开入口 RED/方案审查，未实施值域修复。原 B1629b 是非 point grounding 重定位范围债，独立保留，不挪用编号。
2. **B1622/P2：汇总成员数被叫成物理段数**。MD227/E6 `合计(共4段,同线程)` 的 FamilyMemberCount=4 是 CPU 汇总条数；member roster 的 occurrence 为5+3+2+1=11。`answer_document_mutation_runtime_rcm.go` 及同源图例需改汇总记录量词，数值/成员数/排序不动。这个系统块在成文后发布，不能解释模型首稿错误；两者分别记账。已安排真实发布 RED 和最小双语显示批。
3. **B1639/P2：结果读取教学冲突**。已发布 raw 第61行（byte16040）让模型不要直接读 payload，而同一 StoreBlob 尾注与 B1624 正确建议 grep/read_file；日志13的 head24576 配置使旧句确实位于 tool Summary 保留路径，debug 只打印2000字不代表没有入模。schema 更把原始 Trace 行过滤叫 `result line window`。应统一原 capture 分析、已发布结果读取、两类行号三层教学；权限/registry/实际查询不改。它是否导致本轮误用 line1..400 仅是假说，不伪称因果已证。
4. **B1634c/P2** 已应用计划 oracle 域单列；B1634d/1637b 工程闭环但本轮没有精确配置值/非Trace混合关系公开图的 live 触发。下一批按来源覆盖轮换，不反复跑 H7 追绿。

## 4. 红线及验收边界

无成文重试降级、无系统改写模型块。H7 总288s不是单次活跃流超过4分钟，最长单调用66.538s；单独 actual4ms/旧4m活跃增量、keepalive、真实停滞/取消/调用方deadline工程针本轮绿，不能冒充长期单流 live。状态和数值修复不应让未知前缀、链外背景或不完整 IO 配对获得因果权。

机器1/2仅作原始观测：人工 Go 请求交付通过但证明语义残余开放，H7 整份失败但 B1636/投影/补齐获正证。新问题并入统一台账 §123.1723，各批源码/测试/审阅/提交另记，不把审计完成当修复完成。
