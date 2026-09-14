# r1065 人工审计：显式窗 Trace / C 双错误码传播

- date: 2026-09-14T01:32:37Z
- sweep_start_ts: 20260913-183237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线为已推送 `95cf8b488`，clean binary revision `95cf8b48883f`、built `2026-09-14T01:32:08Z`。B1667导航、B1618来源账户及资格共同冻结后86包全测通过；提交后重新make。实盘243例（read215/apply25/plan3，62个read显式Trace）按影响、老化、模式、语言及本机可执行性选H11与libgit2 C，严格exact2各一次，read15/apply24步、CAP5、整例1200s不变。没有第三例、重刷或改原case/oracle。

机器1/2；**H11正文人工FAIL，C最终代码/原生行为PASS，但存在确定性流程自冲突和逐合同证明能力边界。** 原答案、计划、正式报告与机器失败均保留。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h11_cross_direction_overlap | FAIL | eval/results/real_trace_h11_cross_direction_overlap-20260913-183237 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 58 | read=2,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail（正文） | IO原值/投影保留；机器因旧计数词形失败，正文另有求和、单位、独立性错误 |
| 2 | github_issue_libgit2_foreach_worktree_symptom | PASS | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260913-183237 | write_apply,write_patch_oracle | none | 222s | 28 | read=11,repo_map=5,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（代码/原生行为） | 两处实现修复、原测试不动；终验后非法重新探索两次，formal verified不等于7条自然语言逐项执行证明 |

H11本体wall160s、C本体219s；表中164/222s为并行runner边界，不混用。运行日志 `.codrax/tmp/20260913-r1065-runner.log`。

## 1. H11：不是答案或投影丢失

最终 `.codrax/output/20260913-183518.557-32106.{md,html,root-causes.json}`；过程日志 `run-1.logs/codrax-20260913-183239-000-32106.log`。3次trace_query与2次read_file（均读已发布JSON结果，不是源码）。分析有一次结构拒绝后通过；成文零拒绝、一次成功局部patch。

- 明确13762.791708..13763.024898、233.190ms窗，目标Running/Runnable/Sleep/D=157.248/5.604/70.338/0ms；一份Trace因果投影与真实占用/规则可消双轴保留。绘制、提交fence、traversal/measure业务族在MD82–92，不丢业务线索，但模型自己的业务修向总结仍弱。
- **机器失败仅是旧计数词形**：原oracle要求`12\.658ms.*IO阻塞.*共47段`，MD135/197/632保留12.658ms及“共47条记录”。沿B1622计数语义迁移记测试维护债，不把它说成IO数据丢失，也不为旧oracle恢复物理次数承诺。本轮未修改oracle。
- MD73/1092明确调度器IO标记为零不排除完成闭合的响应阻塞；12.658ms不与请求驻留、S/D状态标记混尺。JIT工作2.388ms为关系未证，未被升级链上根因。旧0.114/0.043ms自身Running×IO错误交集没有恢复。
- **正文人工FAIL独立于机器词形**：MD15/27把12.658+3.602加为IO方向16.260，又在同括号加入其他线程3.598/1.354而仍称16.260；MD35却称不可相加。MD23/49将558000kHz缩成558kHz，策略上下限也少1000倍，伴随内部枚举泄漏。MD55/57/61称独立/正交或假设运行期间同时等待；MD63将最大单席误作方向上界。MD49/51还将不同尺runnable行统称自身累计。
- 这些错误已在模型首稿中出现，patch保留原summary和其他5块，只追加模型选择的工作关系说明。成文阶段已收到IO单席与额外成员、未知关系不等于独立、两种runnable尺及正确kHz值（完整日志约2043/2871–2876；系统频率证据MD567也正确）。不能归为新JSON兼容错误或断言已经证明随机波动；不新增正文关键词门，不让系统改总和/独立性/结论。
- **B1618本轮未获得最终逐账户附注正证**：MD1112–1120旧对账、拓扑和其他线程事实占满8项，另5项省略；未出现本批逐源状态行。公共真实Query多窗/双capture/筛选矩阵和全测已通过，仍不能拿本轮单窗live代销多账户自然采纳，也不扩cap为了固定样例命中。
- 根因旁路15,697B、schema2、status=available、10条；模型原生选择10项完整保留，携捕获/请求窗/查询窗/证据。JSON正规化只搬运版本/别名与恢复字符串包对象，没有代选；旁路没有被正文16.260等错误合计污染。与r1046空选择、r1019合法选择处理失败分别记账。

### B1669/P2：格式提示被当作第二份附件

CLI仅1份Trace，日志438–443的Runtime Artifact Selection却将真实文件路径与`harmony_hitrace`列为两项，铸造`multiple_trace_artifacts`并要求保留歧义。模型最终仍选择attached_trace、范围正确，本例未发生跨文件分析，不能夸大成答案失败根因。

`internal/types/runtime_artifact_selection.go:61–63`直接把AttachedHitraceSource当来源；`context.go:8383`明确字段允许格式提示；既有`runtimeArtifactAttachmentSourceIsAddressable`已区分同类非路径值。下一小批应从typed附件/preflight的唯一身份归并格式载体；只有唯一同附件凭证才合并，真实双文件、同basename异路径、inline未知来源均不得被压为一个。不能从用户/模型prose、文件短名相同或相似度猜同源。当前静态+生产上下文已确认，公共RED/实现待续。

## 2. C：补丁正确，流程与证明边界分别核验

正式保留树为 `run-1.repo/.codrax/worktrees/trace-1789349656266970000-32253`，seed `c1cbb4e`，两次应用提交`91eeb4d`→`c0f67b8`。相对seed仅repository.c两行：assignment先于`!=0`/`<0`比较；原Makefile/test_repository.c不变，保留树clean，运行repo/主fixture没有自动合入。

1. 第一次仅修callback；原生make check真实抓到lookup仍返回1，模型replan再修lookup。最终callback负数/正数原样返回、lookup负数原样返回、非负lookup成功归0，先失败优先。交付实现SHA256=`e92f6a99afc75d599c2332a66279ffd5c873c36277c4b5dbc622de93e2cb96b9`。
2. 主审独立原生后验：链接正式交付实现执行原4断言，再执行`INT_MIN/-42/-1/0/1/17/INT_MAX`×两参数49格，全部通过，组合工具回执exit0/0.988s。输出及二进制在`/tmp/codrax-r1065-native.dACEzt`，没有改交付源、tests或正式report；此为人工后验，不回填系统证明。
3. 正式final `plan-1789349724305530000-32253.final.json`为verified/strong，current report仅1个aggregate make-test/check；累计记录两份report。7条行为条款均planning_only_ungrounded，没有required逐行为义务，因此不是B1561恢复了逐断言证据。末计划模型将callback-positive错绑success_path_returns_zero，但无实际assertion receipt，不产生覆盖；最终UI明确自然语言验收不代表每项独立执行证明。不能以机器PASS销掉B1561/B1575能力债。
4. **B1122摘要旧债新增见证**：实际是lookup断言失败，apply日志1515摘要只取make首行编译命令，controller1586误叙述编译失败；planner随后获得完整失败详情后修正。底层tests_failed类型正确。按旧案补有界失败输出摘要，不新增文本判决、不改真实测试结果。

### B1668/P1：完成态与定位恢复动作自冲突

apply日志 `run-1.logs/codrax-20260913-183416-000-32253.log:3137/3364`记录两次finish→explore_code、reason=truth_ledger_weak_requires_localization。持久workflow进度先明确`explore_code rejected in complete: workflow_already_complete`，恢复后仍重新派探索，随后预算耗尽，第三次finish才结束；不是模型主动无限探索，也不是B1561逐断言缺口导致。

终态events的localization_projected可复算解释两个范围：workflow定位包含repository.c与读过的test_repository.c，owner只支持repository.c，故conflicted/localization_owner_partial、ratio0.5；final active-plan定位只含repository.c，为supported/ratio1。**不是同一范围同时得到0.5/1，也没有当时完整view快照；不冒称override时已经看到ratio1。** `loopkernel/write_localization.go`将context锚并入定位、workflow_execution_view仅在Missing/Unknown时使用plan review；`write_controller_scheduler.go:7045–7074`又允许完成态发Explore，与转换内核拒绝同动作冲突，恢复再normalize可重新铸造该动作。

下一最高优先批：统一当前/累计交付与纯导航观察的typed定位义务范围，显式保留旧批真实未闭义务；同一执行代次的决策正规化和恢复必须消费同一合法动作集，恢复后重校验，禁止Complete→不允许的Explore循环。先实际controller+transition公共RED，再覆盖真缺定位、已验证、旧批未闭、预算耗尽与合法后续批；不能简单跳过weak检查或把全部缺口改绿，不按test文件名/语言特判。当前生产证据与根因定位已记，尚未实现/新RED。

## 3. 收账与下一批

- 已推送：B1667 `b05a595c6`；B1618 `95cf8b488`。本轮不重开同题追绿，原machine结果保留。
- 顺序：B1668完成态/定位恢复合同 → B1669附件身份 → B1561/B1575真实逐合同执行能力与B1122失败摘要；B1622计数语义oracle维护另批，必须保原失败回执。B1647c图修补与B1626多窗自然采纳仍待异构命中，不用本轮无图/单窗代销。
- 无Mermaid正文，本轮图格式为N/A，不宣称导航450格公开层回归等价于15种解析器/所有图live全验收。
- adapter实际为首响应10m/中途真实静默5m/非流式10m；分析terminal调用另有显式3m阶段预算，不混作HTTP默认。两例短于4m，不能称长活跃SSE正证；本批4ms/旧4m活跃流/keepalive/真实静默/调用者取消count3和全测已过。没有因短时无可见答案降级，没有改模型正文或根因结论。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
