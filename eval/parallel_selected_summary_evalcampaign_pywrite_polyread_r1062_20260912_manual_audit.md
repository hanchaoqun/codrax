# r1062 人工审计：Python 症状写与跨语言调用链读

- date: 2026-09-12T08:42:47Z
- sweep_start_ts: 20260912-014247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线`3aeecaca0502`，源码批已提交推送、冻结86包全测和前后置make通过，binary built=`2026-09-12T08:42:17Z`。08:42:47Z–08:48:14Z严格并行2、每例一次，原case/oracle/CAP5/1200s未改，显式SDKROOT26.5；两例期间源码冻结。机器1 PASS/1 FAIL；人工分开核补丁业务正确性、证明完整性和答案事实。没有第三路追跑，后续显示修复不得倒签本轮结果。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260912-014247 | write_apply,write_patch_oracle | none | 263s | 28 | read=7,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial: patch pass / proof incomplete | 业务补丁正确；错误模型比较器未修，累计证明未闭；失败观察真实保留但展示预算遮蔽尾部/引用 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260912-014247 | answer_regex | none | 327s | 46 | read=7,repo_map=3,list=1,trace=0,source_lens=1 | midloop=7,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | 主链/回退已答，最大rank/顺序/导入等同编译错误；最终模型主动删图，render N/A |

## 1. Python：补丁、执行失败与交付证明分判

1. 原case仅用`(is_integer|int[(])`与`ValueError`两类源码匹配，加原write最终验证资格；人工另看双参数、整数/整浮点/非整数及原测试未被替换。外层263s，结果子目录261s，两者计时边界不同，不互相覆盖。
2. 首计划`plan-1789202703137096000-96387`新加`int | float`，本机Python3.9.6实际import TypeError；末计划`plan-1789202758728739000-96638`改用`typing.Union`，最终交付树业务正确。`relativedelta.py` SHA256=`ce72c9f4e37fa3362f0a2a04928d27280fc62daeb4eae3baa0be305513400313`。原4个测试方法源码字节保留，新增负数整浮点、years非整数与零浮点3个测试。
3. 首baseline probe用try/except显示三失败后exit0，系统明确observation_only，不授行为或空changes；不把这种观察性基线说成已修复。原normalize_probe另有比较器错误：`2020-01-01 + years=2 + months=12`实际2023-01-01，模型期望2021-01-01。后两次AssertionError是真实进程失败，但不足以证明产品错误，原生套件均7/7通过。
4. 应用日志`run-1.logs/codrax-20260912-014503-000-96638.log` L684为首失败，685–693完整给出TypeError；report是tests_failed/verification_probe_exception，同时有比较器warning与runtime_failure error，原error保护仍阻止继续suite。本批B1561未改判，原详情/引用确实进入新嵌套字段和final；生成的原输出1646B可读。
5. **B1561显示预算后继缺口确认。** controller L864–867把长traceback仅留头部，221字节正常输出路径因210显示阈值被整项省略；末`run-1.out:204–216`同样将AssertionError截成AssertionEr。完整report L74–79、final L516–521仍保原字节，非未落盘或原JSON丢失。planner L1167的既有failure_signal完整给出TypeError、L1416/1432正确诊断并在L1516修复；所以是controller/交付显示不足、下游恢复成功，不是整个信息链再次完全丢失。后继按通用头尾摘录和独立完整引用预算修复，不识别业务/异常关键词、不改变结论。
6. 最终machine FAIL=`write_final_verdict:unverified:verification_proof_failed`，final profile本轮verification passed7/strong，但累计ledger failed，保3项失败能力（旧import失败command/local_verification及当前错误比较器command）。不得为追绿抹历史真实失败。代码`verification_proof_profile.go:460–462`先处理非权威失败消解、后补累计changed-path，可能使前者看见随后才会解除的缺证；该项只有代码+成品见证，**仍为B1561/P1待公共RED候选**，不把既有精确重跑政策直接裁成bug。
7. 独立后验`.codrax/tmp/20260912-r1062-dateutil-posthoc.{py,log}`明确`not_product_proof`：Python3.9.6原生7/7通过、301项双参数/正负/零/跨年/date+datetime/非整数及非date等检查通过；原探针重放确实AssertionError。原fixture、正式源快照、答案/机器结果未改，也不回填产品proof。结论为补丁业务通过、模型检查尚错、交付证明未闭。
8. JSON首拒为micro修改需patch结构，精确修复建议后已更正，未发现同一字段必带必拒；没有成文通道，不把fin_reject=0当结构契约全部已覆盖。模型新增不兼容类型写法与错误日期期望保模型质量观察，不按特定语言字面新增硬门。

## 2. 跨语言读：核心链保留，事实与图完整性仍分别审

1. 最终`.codrax/output/20260912-014812.824-96424.{md,html}`，MD SHA256=`bcf08bdf36a3c4fbab424578142f953b6f4a52da91756d5f331b460fc73da17a`，以下坐标为该MD或`run-1.logs.all.log`。主链包含Python facade→原生`_fastlex.tokenize_bytes`→PyO3 wrapper→Rust core及ImportError回退，wrapper先from_triples再调用core亦在正文，未再称PyModule为注册主体。
2. 人工FAIL：MD28“最大rank”与lib.rs:24的`rank < r`相反；MD12–13先列list后guard，真实list仅在True分支；MD9/30将导入成功扩大为编译成功，并将各种平台不兼容都描述为回退，源码仅捕获ImportError。精确源码已入模，不能为这些总结错误系统改写正文。
3. MD26描述pyfunction wrapper却引用core注释:8，原模型选择的`ev-a375…`本来就是该行auto_pair_role_description（日志3443），不是系统改错ID。引用角色弱于声明/调用证据的风险保审计，不把本轮错误全归模型或全归系统。
4. B1664生产口径：安全锚3312起无PyModule，主体为_fastlex，3553–3560注册桥清楚为非调用。模型本轮没有提交PyModule错主体，故是同case未重现污染的正向信号，**不是撤证负臂live已验**。
5. 2次成文拒绝/3次patch：首次图锚与关系不匹配，第二次缺visible_label及注册桥；未找到相互冲突合同。模型在日志4016主动`remove_block_ids=[call-diagram]`；4019吸收同调用中已删除图上的17个冗余edit，不是系统擅删图。最终仅2条关系列表、**没有Mermaid，render=N/A**；run-1.out中间草稿的坏图不能当最终图或解析通过。
6. 独立软提示残余：日志3527 role=registration_or_binding却写“_fastlex calls wrap_pyfunction!(...)”；`types/evidence_surface_render.go:269`按AnchorCall先选calls，未使用更精确的registration Kind。typed capsule/权限仍正确，不裁为授权漏洞，也未证明它导致最后错误；按B1664显示同源后续P2审计，不能为了补图自动创造调用。
7. 原fixture与run-1.parent（排.git/.codrax）相同，源码证据充分、最大上下文46%，无预算不足证据。机器仅宽三关键词匹配，因此PASS不能抵消人工失败；不追加第三次或改oracle。

## 3. 后续与不变量

- 已推B1561原字段/落盘/传递修复得到真实触发；新显式预算缺口下一小批公共先红后绿，原r1062不重写。
- B1561累计消解顺序待公共复现；B1626请求成员多窗仍P1完整链方案在统一文档§123.1775.11，未实施；B1598单位、B1402附注域与原生工具链能力债保留。
- 本轮读/写未自然命中Trace，不能冒称显式窗投影/自动补采/根因旁路有新生产验收。上一H7单窗见证与完整回归保留；不改变链上/背景分界、根因机制或业务线索。
- 首响应600s、真实静默300s、非流600s不变。连接持续推理/工具/心跳时不按4ms或旧4分钟降级；整例1200s、caller取消等独立限制仍保。此次两例均正常结束，无超时证据。

状态：exact2-once；machine1-pass/1-fail；write-business-pass/proof-incomplete；read-human-fail；final-mermaid=absent/N-A；original-artifacts/oracles/model-answer=unchanged。
