# tree-sitter-cangjie

仓颉(Cangjie) 编程语言 1.0.0 LTS (cjnative) 自维护 tree-sitter grammar。

**状态**：PR-(-1).b 调研期 —— 仅 corpus 种子；grammar.js / parser.c 在 PR-0c 中实现。

## 设计基线

- **目标**：仓颉 1.0.0 LTS（cjnative；公开版本，cangjie-lang.cn 官网释出）
- **基础**：从零起 grammar；surface 与 Rust/Swift/Java 都不同（`func` 而非 `function`、`package_clause` 必含、`extend` / `match` / trait-like `interface`）
- **fallback**：codrax 主体 3 层（cangjie → regex → path-only）；无近邻 grammar 可降级
- **包路径来源**：必须从首行 `package xxx.yyy` 读取，禁止从文件路径推断（红线 L-Cangjie-2）
- **编译产物排除**：`.cjo` / `target/` / `.cangjie-cache/` 不进 repomap（红线 L-Cangjie-1）

## 文件布局（最终态，PR-0c 完成时）

```
.
├── grammar.js              # tree-sitter grammar DSL
├── src/
│   ├── parser.c            # tree-sitter generate 产物（checkin）
│   ├── scanner.c
│   ├── grammar.json
│   └── tree_sitter/
├── corpus/
│   ├── sources/*.cj        # 原始公开样本（PR-(-1).b 调研期产物）
│   └── *.txt               # tree-sitter test 标准格式（PR-0c 写）
├── package.json
├── binding.go              # cgo Go binding
└── README.md
```

## Corpus 来源（公开资料）

- cangjie-lang.cn（官网 + Playground 示例）
- docs.cangjie-lang.cn/cjnative/user_manual/
- gitee.com/openharmony/third_party_cangjie
- 仓颉官方文档示例（package / func / class / extend / match / generic / foreign）

每个 .cj 文件首行注释标 source URL。

## 关键节点（grammar surface）

- `package_clause`：首行 `package xxx.yyy`
- `import_declaration`：`import xxx.yyy` / `import xxx.{a, b}` / `import xxx as y`
- `function_declaration`：`func name(p: T): R { }`，含 modifier
- `class_declaration` / `struct_declaration` / `interface_declaration` / `enum_declaration`
- `extend_declaration`：扩展类型（含 `extend Trait for Type`）
- `pattern_match`：`match (x) { case A => ...; case _ => ... }`
- `operator_overload`：`operator func +(...) { }`
- `foreign_block`：`foreign func ...` FFI

## Modifier 关键字

`public` / `private` / `protected` / `internal` / `open` / `static` / `operator` / `sealed` / `abstract` / `foreign` / `override` / `redef` / `mut` / `const` / `unsafe`
