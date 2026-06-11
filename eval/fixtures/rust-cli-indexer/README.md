# rust-cli-indexer — Rust CLI fixture(ripgrep 模块组织形态缩减)

设计事实:
- Matcher trait 有且仅有 2 个实现:LiteralMatcher、RegexLikeMatcher(impl 枚举题)
- 调用链:main → run → walker::collect_files → index_file → matcher.is_match(跨 main/walker/matcher 三模块)
