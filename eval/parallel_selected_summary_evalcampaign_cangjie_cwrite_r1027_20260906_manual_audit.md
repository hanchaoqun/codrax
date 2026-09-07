# r1027：仓颉声明清单与C原生写验证人工审计

- date: 2026-09-07T03:14:15Z
- sweep_start_ts: 20260906-201404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

快照`c05682e3dfad`，干净构建后两路并行，没有第三路/重跑求绿。机器2/2 PASS；人工最终答案/补丁均通过，但仓颉调查交接有确定性上下文缺口，不能以最终被救回掩盖过程问题。原工件/计划/报告/oracle未修改。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260906-201415 | write_apply,write_patch_oracle,answer_contains | none | 104s | 28 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 原生编译+两次运行；单行+1/-1；独立原产物四格输出正确，聚合测试不冒充逐合同凭证 |
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260906-201415 | dimension_substring,answer_contains | none | 117s | 28 | read尝试=3但成功=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=4/0,fin_reject=2,unavail=3,prune=0 | FINAL PASS / PROCESS GAP | 完整五声明在finalizer恢复；两次emit-only交接丢分类造成错误中间清单，登记B1586 |

## 仓颉：五个复合行完整，后续交接曾丢分类

- 最终工件`.codrax/output/20260906-201610.195-9030.md`，1853字节；`extend Cart@cart/Cart.cj:30`、`foreign func native_add@bridge/Bridge.cj:6`、三个public class `Bridge@bridge/Bridge.cj:15 / Cart@cart/Cart.cj:14 / App@main.cj:11`均正确，五精确引用齐全。
- package来自源码`demo.cart/demo.bridge/demo.app`声明，不由路径推断；Item是struct未混入class，ohSum未混入foreign，Cart的extend与class分开。问题是分类清单，不需要强加图。
- 首轮source_inventory已有完整1/1/3，随后两次emit-only materialization收到的Relation Dossier只剩role/type=4、function=1、名称/位置/语言/覆盖状态/package，缺精确surface family。`internal/context/builder.go::relationDossierSourceInventoryMemberExamples`未携带该分类；Known Facts又被截断，系统同时不准重新读源码。模型随后生成class4、extend0/foreign0，甚至臆造两个extend。
- Finalizer仍收到完整行及`typed_surface_family_row_counts=extend1/foreign1/public-class3`（ALL日志2400–2425），最终恢复正确。这是“后段幸存数据救回”的正证，不证明中间交接无gap。三次read_file均为不可用调用尝试，metrics计3不等于成功补读。
- 成文两次拒绝分别是section漏text、完整4 cells同时又带label/text；模型按明确schema窄修后第3次成功。JSON字符串载体安全解码未删正文，未证不可能合同。
- `public class=4`陈旧聚合附注与正确family3并存，附注并未触发这两次拒绝或改写；随B1586一起审计其权威范围。早期“entity unverified context”提示未证造成最终误判，不另升级成高危事实。

## C：精准一行改动及真实编译运行

- 原repo seed=`eec74ffc4bd9decac3c920cbffd37476c15cced2`；应用提交`6b8afe65c07d0a80f04dc4945df1c29dddc76b0f`。真实git diff只有`main.c:19 retrun buf; → return buf;`，**+1/-1**，kind=patch经structured_builder→git_apply。
- medium风险由原auto_safe门批准，scope限main.c/本batch；没有主仓合并。原Makefile/其他源码不变；plan-1/2同ID且代码/合同/probe/summary相同，仅状态推进。
- 生产`make test`耗时868.437ms，实际编译为`cc -Wall -O0 -o main main.c`，再运行`./main`、`./main codrax`均exit0。原测试将stdout丢弃，所以它证明编译与两种运行成功，不证明输出逐字符合需求。
- 本席独立运行原生成main，未重编译、未回写：无参、codrax、空字符串、Alice/Bob双参 **4/4**；stdout分别为Hello world/codrax/world及两行Alice/Bob，换行精确，stderr空、exit0。
- 首次模型试图Go probe通过os/exec调用C编译器，被已有精确语言门拒；下一轮删除不适配probe，使用原native Make验证。教学此前已明确不能绕语言门，属于正确纠偏，不是系统同时必带必拒。
- 终态complete/verified/strong；ledger3/3是2文件impact+1advisory，5个合同planning-only、required0，不能解释为5项行为都独立执行。B1575/B1578/B1572适用条件本轮未触发，不计live正证。
- 未跟踪编译产物`main`保留且未提交，系统上下文/最终交付已明确披露；运行开始前原repo已有`.gitignore`是初始化产物，不归因模型。未发现新增P1施工项。

## 下一优先项

当前243个case（read215/apply25/plan3），本批轮换非Python声明分类与原生写验证，而非重跑r1026求绿。B1586下一批优先：跨语言声明清单在emit-only交接中保留精确类别/关键属性及显示完整性，复用既有机械row-set权威；审计已完整时是否仍重复物化、陈旧模型聚合是否还发相冲突的建议。不放开全部read，不按用户词语硬门，不要求模型凭泛化type/function角色重猜声明类别。后续轮换ArkTS、C宏/条件编译和精确窗Trace回归；模型答案仍由模型负责。
